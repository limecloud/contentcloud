package codex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/integration/plugin"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
)

type fakeResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

type fakeCall struct {
	name string
	args []string
	env  map[string]string
}

type fakeRunner struct {
	responses []fakeResponse
	calls     []fakeCall
}

func (f *fakeRunner) Run(_ context.Context, command Command) (CommandResult, error) {
	f.calls = append(f.calls, fakeCall{name: command.Name, args: append([]string(nil), command.Args...), env: command.Env})
	if len(f.responses) == 0 {
		return CommandResult{}, errors.New("unexpected command: " + strings.Join(command.Args, " "))
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return CommandResult{Stdout: []byte(response.stdout), Stderr: []byte(response.stderr), ExitCode: response.exitCode}, response.err
}

func TestCapabilitiesFollowVerifiedCodexVersion(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{stdout: "codex-cli 0.146.0\n"}}}
	host := newTestHost(t, runner)

	capabilities, err := host.Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if capabilities.Skills || capabilities.MCPStdio {
		t.Fatalf("old Codex version reported Agent Plugins support: %#v", capabilities)
	}
	if capabilities.PluginDirectoryInstall || capabilities.AtomicInstall || !capabilities.Rollback || !capabilities.NewSessionRequired {
		t.Fatalf("unexpected native capabilities: %#v", capabilities)
	}
}

func TestDetectReadyUsesCodexNativeStorePaths(t *testing.T) {
	runner := &fakeRunner{}
	host := newTestHost(t, runner)
	target, packageRoot := readyTarget(t, host)
	runner.responses = readyResponses(host, target, packageRoot)

	state, err := host.Detect(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != pluginhost.StatusReady || state.Release == nil || state.Release.Digest != target.Release.Digest {
		t.Fatalf("unexpected ready state: %#v", state)
	}
	for _, item := range state.Components {
		if item.Status != pluginhost.StatusReady {
			t.Fatalf("component not ready: %#v", item)
		}
	}
	if got := runner.calls[0].args; !reflect.DeepEqual(got, []string{"--version"}) {
		t.Fatalf("capability detection did not query Codex version: %#v", runner.calls)
	}
	for _, call := range runner.calls {
		if call.env["CODEX_HOME"] != host.config.CodexHome {
			t.Fatalf("Codex command escaped isolated home: %#v", call)
		}
	}
}

func TestDetectBlocksSameNamedUnmanagedMarketplace(t *testing.T) {
	runner := &fakeRunner{}
	host := newTestHost(t, runner)
	target := testTarget()
	runner.responses = []fakeResponse{
		{stdout: "codex-cli 0.147.0\n"},
		{stdout: marketplaceJSON(t, hMarketplace(host, filepath.Join(t.TempDir(), "foreign")))},
		{stdout: pluginListJSON(nil)},
	}

	state, err := host.Detect(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != pluginhost.StatusBlocked {
		t.Fatalf("unmanaged marketplace should be blocked without takeover: %#v", state)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("blocked detection should remain read-only: %#v", runner.calls)
	}
}

func TestApplyBuildsLocalProjectionAndUsesCodexPluginStore(t *testing.T) {
	runner := &fakeRunner{}
	host := newTestHost(t, runner)
	target := testTarget()
	packageRoot := filepath.Join(host.config.ProjectionRoot, "packages", target.Release.PluginID, target.Release.Version)
	if err := os.MkdirAll(packageRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	packageRoot, err := canonicalPath(packageRoot)
	if err != nil {
		t.Fatal(err)
	}
	installedRoot := host.expectedInstalledPluginRoot(target.Release)
	dataRoot := filepath.Join(host.config.CodexHome, "plugins", "data", "agent-plugins", strings.Repeat("a", 64))
	runner.responses = []fakeResponse{
		{stdout: marketplaceJSON(t)},
		{stdout: pluginListJSON(nil)},
		{stdout: jsonValue(t, map[string]any{"marketplaceName": host.config.MarketplaceName, "installedRoot": host.config.ProjectionRoot, "alreadyAdded": false})},
		{stdout: jsonValue(t, map[string]any{"pluginId": host.pluginID(target.Release.PluginID), "name": target.Release.PluginID, "marketplaceName": host.config.MarketplaceName, "version": target.Release.Version, "installedPath": installedRoot})},
		{stdout: "codex-cli 0.147.0\n"},
		{stdout: marketplaceJSON(t, hMarketplace(host, host.config.ProjectionRoot))},
		{stdout: pluginListJSON(&pluginListItem{PluginID: host.pluginID(target.Release.PluginID), Name: target.Release.PluginID, MarketplaceName: host.config.MarketplaceName, Version: target.Release.Version, Installed: true, Enabled: true, Source: pluginSource{Source: "local", Path: packageRoot}})},
		{stdout: mcpListJSON(t, target.MCP[0].Name, installedRoot, dataRoot)},
	}

	change, installed, err := host.Apply(t.Context(), pluginhost.NativeApply{Target: target, PackageRoot: packageRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(change.Data) == 0 || len(installed) != len(target.Skills)+len(target.MCP) {
		t.Fatalf("unexpected native install result: change=%s components=%#v", change.Data, installed)
	}
	manifest, _, err := host.readProjection()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Plugins) != 1 || manifest.Plugins[0].Name != target.Release.PluginID || manifest.Plugins[0].Source.Path != "./packages/example-plugin/1.2.3" {
		t.Fatalf("unexpected Codex marketplace projection: %#v", manifest)
	}
	wantMutations := [][]string{
		{"plugin", "marketplace", "add", host.config.ProjectionRoot, "--json"},
		{"plugin", "add", host.pluginID(target.Release.PluginID), "--json"},
	}
	for _, wanted := range wantMutations {
		if !hasCall(runner.calls, wanted) {
			t.Fatalf("missing native Codex command %#v in %#v", wanted, runner.calls)
		}
	}
}

func TestApplyFailureRollsBackOnlyNativeStateCreatedByRun(t *testing.T) {
	runner := &fakeRunner{}
	host := newTestHost(t, runner)
	target := testTarget()
	packageRoot := filepath.Join(host.config.ProjectionRoot, "packages", target.Release.PluginID, target.Release.Version)
	if err := os.MkdirAll(packageRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	packageRoot, err := canonicalPath(packageRoot)
	if err != nil {
		t.Fatal(err)
	}
	runner.responses = []fakeResponse{
		{stdout: marketplaceJSON(t)},
		{stdout: pluginListJSON(nil)},
		{stdout: jsonValue(t, map[string]any{"marketplaceName": host.config.MarketplaceName, "installedRoot": host.config.ProjectionRoot, "alreadyAdded": false})},
		{stderr: "plugin unavailable", exitCode: 1},
		{stdout: `{}`},
	}

	change, _, err := host.Apply(t.Context(), pluginhost.NativeApply{Target: target, PackageRoot: packageRoot})
	if err == nil {
		t.Fatal("plugin install failure was ignored")
	}
	if err := host.Rollback(t.Context(), change); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := host.readProjection()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Plugins) != 0 {
		t.Fatalf("failed install left plugin in projection: %#v", manifest)
	}
	if got := runner.calls[len(runner.calls)-1].args; !reflect.DeepEqual(got, []string{"plugin", "marketplace", "remove", host.config.MarketplaceName, "--json"}) {
		t.Fatalf("rollback did not remove marketplace created by run: %#v", runner.calls)
	}
}

func TestRemoveAndRollbackRestoreProjectionAndPlugin(t *testing.T) {
	runner := &fakeRunner{}
	host := newTestHost(t, runner)
	target, packageRoot := readyTarget(t, host)
	currentMarketplace := hMarketplace(host, host.config.ProjectionRoot)
	currentPlugin := &pluginListItem{PluginID: host.pluginID(target.Release.PluginID), Name: target.Release.PluginID, MarketplaceName: host.config.MarketplaceName, Version: target.Release.Version, Installed: true, Enabled: true, Source: pluginSource{Source: "local", Path: packageRoot}}
	runner.responses = []fakeResponse{
		{stdout: marketplaceJSON(t, currentMarketplace)},
		{stdout: pluginListJSON(currentPlugin)},
		{stdout: `{}`},
		{stdout: `{}`},
		{stdout: "codex-cli 0.147.0\n"},
		{stdout: marketplaceJSON(t)},
		{stdout: pluginListJSON(nil)},
		{stdout: jsonValue(t, map[string]any{"marketplaceName": host.config.MarketplaceName, "installedRoot": host.config.ProjectionRoot, "alreadyAdded": false})},
		{stdout: jsonValue(t, map[string]any{"pluginId": host.pluginID(target.Release.PluginID), "name": target.Release.PluginID, "marketplaceName": host.config.MarketplaceName, "version": target.Release.Version, "installedPath": host.expectedInstalledPluginRoot(target.Release)})},
	}

	change, err := host.Remove(t.Context(), pluginhost.NativeRemove{Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Rollback(t.Context(), change); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := host.readProjection()
	if err != nil {
		t.Fatal(err)
	}
	restoredRoot, err := host.resolvePluginSource(manifest.Plugins[0].Source.Path)
	if len(manifest.Plugins) != 1 || err != nil || restoredRoot != packageRoot {
		t.Fatalf("rollback did not restore projection: %#v", manifest)
	}
	if got := runner.calls[len(runner.calls)-1].args; !reflect.DeepEqual(got, []string{"plugin", "add", host.pluginID(target.Release.PluginID), "--json"}) {
		t.Fatalf("rollback did not restore previous plugin: %#v", runner.calls)
	}
}

func TestProjectionPreservesOtherManagedPlugins(t *testing.T) {
	host := newTestHost(t, &fakeRunner{})
	firstRoot := filepath.Join(host.config.ProjectionRoot, "packages", "first")
	secondRoot := filepath.Join(host.config.ProjectionRoot, "packages", "second")
	if _, err := host.upsertProjection("z-plugin", firstRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := host.upsertProjection("a-plugin", secondRoot); err != nil {
		t.Fatal(err)
	}
	if _, _, err := host.removeProjection("z-plugin"); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := host.readProjection()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Plugins) != 1 || manifest.Plugins[0].Name != "a-plugin" {
		t.Fatalf("projection update lost unrelated plugin: %#v", manifest)
	}
}

func TestRealCodexAgentPluginLifecycle(t *testing.T) {
	if os.Getenv("CONTENTCLOUD_CODEX_PLUGIN_SMOKE") != "1" {
		t.Skip("set CONTENTCLOUD_CODEX_PLUGIN_SMOKE=1 to run against the installed Codex CLI")
	}
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", ".."))
	pkg, err := plugin.Load(filepath.Join(repositoryRoot, "plugins", "contentcloud-video-production"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := pluginhost.NewStore(filepath.Join(root, "contentcloud"))
	if err != nil {
		t.Fatal(err)
	}
	host, err := New(Config{
		CodexHome:      filepath.Join(root, "codex"),
		ProjectionRoot: store.Root,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := pluginhost.New(host, store)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.Plan(t.Context(), pkg, "install")
	if err != nil {
		t.Fatalf("plan failed: %#v", err)
	}
	receipt, err := adapter.Apply(t.Context(), pkg, plan, true)
	if err != nil {
		t.Fatalf("apply failed: %#v", err)
	}
	if receipt.Status != pluginhost.StatusReady {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	state, err := adapter.Inspect(t.Context(), pkg)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != pluginhost.StatusReady {
		t.Fatalf("real Codex did not activate Agent Plugin: %#v", state)
	}
	removePlan, err := adapter.Plan(t.Context(), pkg, "remove")
	if err != nil {
		t.Fatal(err)
	}
	removed, err := adapter.Remove(t.Context(), pkg, removePlan, true)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Status != pluginhost.StatusRemoved {
		t.Fatalf("unexpected removal receipt: %#v", removed)
	}
}

func newTestHost(t *testing.T, runner CommandRunner) *Host {
	t.Helper()
	root := t.TempDir()
	host, err := New(Config{
		Binary:          "codex",
		CodexHome:       filepath.Join(root, "codex-home"),
		MarketplaceName: "contentcloud",
		ProjectionRoot:  filepath.Join(root, "projection"),
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func testTarget() pluginhost.HostTarget {
	return pluginhost.HostTarget{
		Release: pluginhost.ReleaseRef{PluginID: "example-plugin", Version: "1.2.3", Digest: "sha256:" + strings.Repeat("a", 64)},
		Skills:  []string{"example-skill"},
		MCP:     []pluginhost.MCPTarget{{Name: "example-mcp", Type: "stdio"}},
	}
}

func readyTarget(t *testing.T, host *Host) (pluginhost.HostTarget, string) {
	t.Helper()
	target := testTarget()
	packageRoot := filepath.Join(host.config.ProjectionRoot, "packages", target.Release.PluginID, target.Release.Version)
	if err := os.MkdirAll(packageRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	packageRoot, err := canonicalPath(packageRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.upsertProjection(target.Release.PluginID, packageRoot); err != nil {
		t.Fatal(err)
	}
	return target, packageRoot
}

func readyResponses(host *Host, target pluginhost.HostTarget, packageRoot string) []fakeResponse {
	installedRoot := host.expectedInstalledPluginRoot(target.Release)
	dataRoot := filepath.Join(host.config.CodexHome, "plugins", "data", "agent-plugins", strings.Repeat("d", 64))
	return []fakeResponse{
		{stdout: "codex-cli 0.147.0\n"},
		{stdout: marketplaceJSON(nil, hMarketplace(host, host.config.ProjectionRoot))},
		{stdout: pluginListJSON(&pluginListItem{PluginID: host.pluginID(target.Release.PluginID), Name: target.Release.PluginID, MarketplaceName: host.config.MarketplaceName, Version: target.Release.Version, Installed: true, Enabled: true, Source: pluginSource{Source: "local", Path: packageRoot}})},
		{stdout: mcpListJSON(nil, target.MCP[0].Name, installedRoot, dataRoot)},
	}
}

func hMarketplace(host *Host, root string) marketplaceListItem {
	return marketplaceListItem{Name: host.config.MarketplaceName, Root: root, MarketplaceSource: marketplaceSource{SourceType: "local", Source: root}}
}

func marketplaceJSON(t *testing.T, items ...marketplaceListItem) string {
	if items == nil {
		items = []marketplaceListItem{}
	}
	return jsonValue(t, marketplaceListResponse{Marketplaces: items})
}

func pluginListJSON(item *pluginListItem) string {
	items := []pluginListItem{}
	if item != nil {
		items = append(items, *item)
	}
	body, _ := json.Marshal(pluginListResponse{Installed: items, Available: []pluginListItem{}})
	return string(body)
}

func mcpListJSON(t *testing.T, name, pluginRoot, dataRoot string) string {
	return jsonValue(t, []mcpListItem{{
		Name:    name,
		Enabled: true,
		Transport: mcpTransport{
			Type: "stdio",
			Env:  map[string]string{"PLUGIN_ROOT": pluginRoot, "PLUGIN_DATA": dataRoot},
			CWD:  pluginRoot,
		},
	}})
}

func jsonValue(t *testing.T, value any) string {
	if t != nil {
		t.Helper()
	}
	body, err := json.Marshal(value)
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		panic(err)
	}
	return string(body)
}

func hasCall(calls []fakeCall, wanted []string) bool {
	for _, call := range calls {
		if reflect.DeepEqual(call.args, wanted) {
			return true
		}
	}
	return false
}
