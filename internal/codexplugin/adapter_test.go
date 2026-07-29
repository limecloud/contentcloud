package codexplugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type fakeResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

type fakeRunner struct {
	responses []fakeResponse
	calls     [][]string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (CommandResult, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if len(f.responses) == 0 {
		return CommandResult{}, errors.New("unexpected command")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return CommandResult{Stdout: []byte(response.stdout), Stderr: []byte(response.stderr), ExitCode: response.exitCode}, response.err
}

func TestPlanIsReadOnlyAndPinsMarketplaceAndPlugin(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{stdout: `{"marketplaces":[]}`},
		{stdout: `{"installed":[],"available":[]}`},
	}}
	adapter := mustAdapter(t, DefaultSpec("0.8.0"), runner)
	plan, err := adapter.Plan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "ready" || !plan.RequiresConfirmation || len(plan.Actions) != 2 {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	wantMarketplace := []string{"codex", "plugin", "marketplace", "add", "limecloud/contentcloud", "--ref", "v0.8.0", "--json"}
	if !reflect.DeepEqual(plan.Actions[0].Arguments, wantMarketplace[1:]) {
		t.Fatalf("marketplace action is not pinned: %#v", plan.Actions[0])
	}
	if len(runner.calls) != 2 || runner.calls[0][1] != "plugin" || runner.calls[1][1] != "plugin" {
		t.Fatalf("plan ran a mutation: %#v", runner.calls)
	}
}

func TestDetectClassifiesCurrentOutdatedAndBroken(t *testing.T) {
	tests := []struct {
		name        string
		marketplace string
		plugin      string
		want        string
	}{
		{
			name:        "current",
			marketplace: `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"https://github.com/limecloud/contentcloud.git","ref":"v0.8.0"}}]}`,
			plugin:      `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.8.0","installed":true,"enabled":true}],"available":[]}`,
			want:        "current",
		},
		{
			name:        "outdated ref",
			marketplace: `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"main"}}]}`,
			plugin:      `{"installed":[],"available":[]}`,
			want:        "outdated",
		},
		{
			name:        "disabled plugin",
			marketplace: `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"v0.8.0"}}]}`,
			plugin:      `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.8.0","installed":true,"enabled":false}],"available":[]}`,
			want:        "broken",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{responses: []fakeResponse{{stdout: test.marketplace}, {stdout: test.plugin}}}
			adapter := mustAdapter(t, DefaultSpec("0.8.0"), runner)
			state, err := adapter.Detect(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if state.Status != test.want {
				t.Fatalf("status=%s, want %s; state=%#v", state.Status, test.want, state)
			}
		})
	}
}

func TestApplyRequiresConfirmation(t *testing.T) {
	responses := []fakeResponse{
		{stdout: `{"marketplaces":[]}`}, {stdout: `{"installed":[],"available":[]}`},
		{stdout: `{"marketplaces":[]}`}, {stdout: `{"installed":[],"available":[]}`},
	}
	runner := &fakeRunner{responses: responses}
	adapter := mustAdapter(t, DefaultSpec("0.8.0"), runner)
	plan, err := adapter.Plan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Apply(t.Context(), plan, false); err == nil {
		t.Fatal("apply without confirmation must fail")
	}
	if len(runner.calls) != 4 {
		t.Fatalf("apply ran a mutation without confirmation: %#v", runner.calls)
	}
}

func TestApplyInstallsAndValidates(t *testing.T) {
	missingMarketplace := `{"marketplaces":[]}`
	missingPlugin := `{"installed":[],"available":[]}`
	currentMarketplace := `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"v0.8.0"}}]}`
	currentPlugin := `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.8.0","installed":true,"enabled":true}],"available":[]}`
	runner := &fakeRunner{responses: []fakeResponse{
		{stdout: missingMarketplace}, {stdout: missingPlugin},
		{stdout: missingMarketplace}, {stdout: missingPlugin},
		{stdout: `{"marketplaceName":"contentcloud","installedRoot":"/tmp/cache","alreadyAdded":false}`},
		{stdout: `{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.8.0","installedPath":"/tmp/plugin"}`},
		{stdout: currentMarketplace}, {stdout: currentPlugin},
	}}
	adapter := mustAdapter(t, DefaultSpec("0.8.0"), runner)
	plan, err := adapter.Plan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Apply(t.Context(), plan, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || !result.Receipt.MarketplaceAdded || !result.Receipt.PluginAdded || result.State.Status != "current" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(runner.responses) != 0 {
		t.Fatalf("unused responses: %d", len(runner.responses))
	}
}

func TestApplyRollsBackOnlyMarketplaceAddedByThisRun(t *testing.T) {
	missingMarketplace := `{"marketplaces":[]}`
	missingPlugin := `{"installed":[],"available":[]}`
	runner := &fakeRunner{responses: []fakeResponse{
		{stdout: missingMarketplace}, {stdout: missingPlugin},
		{stdout: missingMarketplace}, {stdout: missingPlugin},
		{stdout: `{"marketplaceName":"contentcloud","installedRoot":"/tmp/cache","alreadyAdded":false}`},
		{stderr: "plugin unavailable", exitCode: 1},
		{stdout: `{"marketplaceName":"contentcloud","installedRoot":null}`},
	}}
	adapter := mustAdapter(t, DefaultSpec("0.8.0"), runner)
	plan, err := adapter.Plan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Apply(t.Context(), plan, true)
	if err == nil {
		t.Fatal("plugin failure must be returned")
	}
	if !result.Receipt.MarketplaceAdded || result.Receipt.PluginAdded || len(result.RollbackErrors) != 0 {
		t.Fatalf("unexpected rollback result: %#v", result)
	}
	last := runner.calls[len(runner.calls)-1]
	if strings.Join(last, " ") != "codex plugin marketplace remove contentcloud --json" {
		t.Fatalf("unexpected rollback call: %#v", last)
	}
	for _, call := range runner.calls {
		if len(call) > 2 && call[1] == "plugin" && call[2] == "remove" {
			t.Fatalf("rollback removed a plugin that this run did not install: %#v", runner.calls)
		}
	}
}

func TestApplyRollsBackActualPluginIdentityReturnedByCodex(t *testing.T) {
	missingMarketplace := `{"marketplaces":[]}`
	missingPlugin := `{"installed":[],"available":[]}`
	runner := &fakeRunner{responses: []fakeResponse{
		{stdout: missingMarketplace}, {stdout: missingPlugin},
		{stdout: missingMarketplace}, {stdout: missingPlugin},
		{stdout: `{"marketplaceName":"contentcloud","installedRoot":"/tmp/cache","alreadyAdded":false}`},
		{stdout: `{"pluginId":"unexpected-plugin@contentcloud","name":"unexpected-plugin","marketplaceName":"contentcloud","version":"0.8.0","installedPath":"/tmp/unexpected"}`},
		{stdout: `{"removed":true}`},
		{stdout: `{"removed":true}`},
	}}
	adapter := mustAdapter(t, DefaultSpec("0.8.0"), runner)
	plan, err := adapter.Plan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Apply(t.Context(), plan, true)
	if err == nil {
		t.Fatal("mismatched plugin identity must fail")
	}
	if result.Receipt.AddedPluginID != "unexpected-plugin@contentcloud" || !result.Receipt.PluginAdded || len(result.RollbackErrors) != 0 {
		t.Fatalf("unexpected receipt: %#v", result)
	}
	wantSuffix := [][]string{
		{"codex", "plugin", "remove", "unexpected-plugin@contentcloud", "--json"},
		{"codex", "plugin", "marketplace", "remove", "contentcloud", "--json"},
	}
	if !reflect.DeepEqual(runner.calls[len(runner.calls)-2:], wantSuffix) {
		t.Fatalf("rollback did not remove the identities created by this run: %#v", runner.calls)
	}
}

func TestApplyRollsBackActualMarketplaceIdentityReturnedByCodex(t *testing.T) {
	missingMarketplace := `{"marketplaces":[]}`
	missingPlugin := `{"installed":[],"available":[]}`
	runner := &fakeRunner{responses: []fakeResponse{
		{stdout: missingMarketplace}, {stdout: missingPlugin},
		{stdout: missingMarketplace}, {stdout: missingPlugin},
		{stdout: `{"marketplaceName":"unexpected-marketplace","installedRoot":"/tmp/cache","alreadyAdded":false}`},
		{stdout: `{"removed":true}`},
	}}
	adapter := mustAdapter(t, DefaultSpec("0.8.0"), runner)
	plan, err := adapter.Plan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Apply(t.Context(), plan, true)
	if err == nil {
		t.Fatal("mismatched marketplace identity must fail")
	}
	if result.Receipt.AddedMarketplaceName != "unexpected-marketplace" || !result.Receipt.MarketplaceAdded || len(result.RollbackErrors) != 0 {
		t.Fatalf("unexpected receipt: %#v", result)
	}
	last := runner.calls[len(runner.calls)-1]
	if strings.Join(last, " ") != "codex plugin marketplace remove unexpected-marketplace --json" {
		t.Fatalf("rollback did not remove the marketplace created by this run: %#v", runner.calls)
	}
}

func TestBlockedPlanNeverReplacesExistingInstall(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{stdout: `{"marketplaces":[{"name":"contentcloud","root":"/tmp/other","marketplaceSource":{"sourceType":"git","source":"someone/else","ref":"main"}}]}`},
		{stdout: `{"installed":[],"available":[]}`},
	}}
	adapter := mustAdapter(t, DefaultSpec("0.8.0"), runner)
	plan, err := adapter.Plan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "blocked" || len(plan.Actions) != 0 || len(plan.BlockingReasons) == 0 {
		t.Fatalf("unexpected blocked plan: %#v", plan)
	}
}

func TestNewChatDeepLinkContainsWorkspaceAndPluginMention(t *testing.T) {
	spec := DefaultSpec("0.8.0")
	prompt := RecoveryPrompt(spec)
	link, err := NewChatDeepLink(spec, t.TempDir(), prompt)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := urlParse(link)
	if err != nil {
		t.Fatal(err)
	}
	if parsed["path"] == "" || parsed["prompt"] != prompt || !strings.Contains(parsed["prompt"], "plugin://contentcloud-video-production@contentcloud") || !strings.Contains(parsed["prompt"], "workspace_context") || strings.Contains(parsed["prompt"], "contentcloud_workspace_conversation_context") {
		t.Fatalf("unexpected deep link: %s", link)
	}
}

func TestNewChatDeepLinkRejectsNonCanonicalRecoveryPrompt(t *testing.T) {
	spec := DefaultSpec("0.8.0")
	if _, err := NewChatDeepLink(spec, t.TempDir(), "continue without a plugin mention"); err == nil {
		t.Fatal("new-chat link accepted a non-canonical recovery prompt")
	}
}

func TestLaunchNewChatFallsBackToWorkspaceCommand(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{stderr: "URL scheme unavailable", exitCode: 1},
		{stdout: "opened"},
	}}
	adapter := mustAdapter(t, DefaultSpec("0.8.0"), runner)
	adapter.GOOS = "darwin"
	workspace := t.TempDir()
	result := adapter.LaunchNewChat(t.Context(), workspace)
	if !result.Opened || result.Method != "workspace" || result.Error != "" || result.WorkspacePath != workspace {
		t.Fatalf("unexpected fallback result: %#v", result)
	}
	if len(runner.calls) != 2 || runner.calls[0][0] != "open" || !strings.HasPrefix(runner.calls[0][1], "codex://new?") || !reflect.DeepEqual(runner.calls[1], []string{"codex", "app", workspace}) {
		t.Fatalf("unexpected launch calls: %#v", runner.calls)
	}
}

func TestLaunchNewChatReportsBothLaunchFailures(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{stderr: "URL scheme unavailable", exitCode: 1},
		{stderr: "desktop app unavailable", exitCode: 2},
	}}
	adapter := mustAdapter(t, DefaultSpec("0.8.0"), runner)
	adapter.GOOS = "darwin"
	result := adapter.LaunchNewChat(t.Context(), t.TempDir())
	if result.Opened || !strings.Contains(result.Error, "open exited with 1") || !strings.Contains(result.Error, "codex app exited with 2") {
		t.Fatalf("unexpected launch failure: %#v", result)
	}
}

func mustAdapter(t *testing.T, spec Spec, runner CommandRunner) *Adapter {
	t.Helper()
	adapter, err := New(spec, runner)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func urlParse(value string) (map[string]string, error) {
	var payload struct {
		URL string `json:"url"`
	}
	encoded, _ := json.Marshal(map[string]string{"url": value})
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(payload.URL)
	if err != nil {
		return nil, err
	}
	return map[string]string{"path": parsed.Query().Get("path"), "prompt": parsed.Query().Get("prompt")}, nil
}
