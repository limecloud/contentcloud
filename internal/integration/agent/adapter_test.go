package agentadapter

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestClassifyProcessErrorUsesStableCodeWithoutLeakingStderr(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		stderr    string
		wantCode  string
		retryable bool
	}{
		{name: "auth", kind: "codex", stderr: "Unauthorized: Bearer rtg_private-token", wantCode: "CODEX_AUTH_REQUIRED", retryable: false},
		{name: "rate", kind: "claude", stderr: "HTTP 429 quota exceeded for sk-private", wantCode: "CLAUDE_RATE_LIMITED", retryable: true},
		{name: "network", kind: "codex", stderr: "connection reset by peer", wantCode: "CODEX_NETWORK_UNAVAILABLE", retryable: true},
		{name: "permission", kind: "claude", stderr: "permission denied: /Users/private/workspace", wantCode: "CLAUDE_PERMISSION_DENIED", retryable: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := classifyProcessError(test.kind, errors.New("process failed"), test.stderr)
			var domainErr *fault.Error
			if !errors.As(err, &domainErr) || domainErr.Code != test.wantCode || domainErr.Retryable != test.retryable {
				t.Fatalf("classified error = %#v", err)
			}
			if strings.Contains(err.Error(), "rtg_") || strings.Contains(err.Error(), "sk-") || strings.Contains(strings.ToLower(err.Error()), "/users/") {
				t.Fatalf("process stderr leaked through error: %v", err)
			}
		})
	}
	cancelled := classifyProcessError("codex", context.Canceled, "Bearer private")
	var domainErr *fault.Error
	if !errors.As(cancelled, &domainErr) || domainErr.Code != "AGENT_CANCELED" || !domainErr.Retryable {
		t.Fatalf("cancel classification = %#v", cancelled)
	}
}

func TestLimitedBufferTruncatesWithoutFailingChildProcessWrite(t *testing.T) {
	var buffer limitedBuffer
	body := []byte(strings.Repeat("x", maxAgentOutput+1024))
	n, err := buffer.Write(body)
	if err != nil || n != len(body) || !buffer.over || len(buffer.Bytes()) != maxAgentOutput {
		t.Fatalf("limited buffer write: n=%d err=%v over=%t stored=%d", n, err, buffer.over, len(buffer.Bytes()))
	}
}

func TestClientRegistryResolvesAliasesAndPlannedCapabilities(t *testing.T) {
	claude, ok := Lookup(" Claude ")
	if !ok || claude.ID != ClientClaudeCode || claude.CapabilityStatus(CapabilityLocalAutomation) != SupportAvailable {
		t.Fatalf("unexpected Claude registry entry: %#v", claude)
	}
	openClaw, ok := Lookup("open-claw")
	if !ok || openClaw.ID != ClientOpenClaw || openClaw.CapabilityStatus(CapabilityInteractiveHandoff) != SupportPlanned {
		t.Fatalf("unexpected OpenClaw registry entry: %#v", openClaw)
	}
	_, err := RequireCapability("cursor", CapabilityInteractiveHandoff)
	var domainError *fault.Error
	if !errors.As(err, &domainError) || domainError.Code != "AGENT_CLIENT_CAPABILITY_UNAVAILABLE" {
		t.Fatalf("planned capability was not rejected explicitly: %v", err)
	}
}

func TestHarnessKindsNormalizeToCanonicalNames(t *testing.T) {
	if got := normalizeHarnessKind("claude-code"); got != "claude" {
		t.Fatalf("Claude alias normalized to %q, want claude", got)
	}
	if got := normalizeHarnessKind("codex"); got != "codex" {
		t.Fatalf("Codex kind normalized to %q, want codex", got)
	}
}

func TestHandoffStrategiesMatchAvailableRegistryCapabilities(t *testing.T) {
	for _, client := range Clients() {
		_, implemented := handoffFactories[client.ID]
		available := client.CapabilityStatus(CapabilityInteractiveHandoff) == SupportAvailable
		if implemented != available {
			t.Fatalf("handoff strategy drift for %s: implemented=%t available=%t", client.ID, implemented, available)
		}
	}
	adapter, err := SelectHandoff("codex", "0.8.0")
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := adapter.Build(HandoffRequest{Kind: "project", ProjectID: "project-1", Target: HandoffTarget{Kind: "project", ID: "project-1"}})
	if err != nil || handoff.Client.ID != ClientCodex || handoff.Launch.Mode != "deep_link" || !strings.HasPrefix(handoff.Launch.URL, "codex://new?") {
		t.Fatalf("unexpected Codex handoff: %#v err=%v", handoff, err)
	}
}

func TestHandoffFailsClosedForUnsupportedInput(t *testing.T) {
	adapter, err := SelectHandoff("codex", "0.10.0")
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Build(HandoffRequest{Kind: "unsupported", ProjectID: "project-1", Target: HandoffTarget{Kind: "project", ID: "project-1"}})
	var domainError *fault.Error
	if !errors.As(err, &domainError) || domainError.Code != "AGENT_HANDOFF_KIND_INVALID" {
		t.Fatalf("invalid handoff kind was not rejected: %v", err)
	}
}

func TestAgentEnvironmentDoesNotInheritUnrelatedSecret(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "provider-key")
	t.Setenv("CONTENTCLOUD_TEST_SECRET", "do-not-inherit")
	t.Setenv("CONTENTCLOUD_DEVICE_TOKEN", "dt_do-not-inherit")
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_do-not-inherit")
	t.Setenv("CONTENTCLOUD_RUN_TOKEN", "rt_do-not-inherit")
	providerInherited := false
	for _, value := range agentEnvironment("claude") {
		if strings.Contains(value, "do-not-inherit") {
			t.Fatalf("ContentCloud control-plane secret inherited: %s", value)
		}
		if value == "OPENAI_API_KEY=provider-key" {
			providerInherited = true
		}
	}
	if !providerInherited {
		t.Fatal("provider environment was not inherited by the automation agent")
	}
}

func TestRuntimeGatewayEnvironmentExposesOnlyAttemptCredential(t *testing.T) {
	t.Setenv("CONTENTCLOUD_DEVICE_TOKEN", "dt_must-not-leak")
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_must-not-leak")
	t.Setenv("CONTENTCLOUD_RUN_TOKEN", "rt_must-not-leak")
	base := agentEnvironment("codex")
	gateway, err := runtimeGatewayEnvironment(RuntimeGatewayConfig{
		URL:          "https://content.example/api/v1/runtime/mcp/call",
		Token:        "rtg_attempt-only",
		AllowedTools: []string{"runtime.state.query", "runtime.state.mutate"},
	})
	if err != nil {
		t.Fatal(err)
	}
	combined := strings.Join(append(base, gateway...), "\n")
	for _, forbidden := range []string{"dt_must-not-leak", "wt_must-not-leak", "rt_must-not-leak", "fence_token"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("control-plane authority leaked to the Agent environment: %s", forbidden)
		}
	}
	for _, required := range []string{
		"CONTENTCLOUD_RUNTIME_GATEWAY_URL=https://content.example/api/v1/runtime/mcp/call",
		"CONTENTCLOUD_RUNTIME_GATEWAY_TOKEN=rtg_attempt-only",
		`CONTENTCLOUD_RUNTIME_GATEWAY_TOOLS=["runtime.state.query","runtime.state.mutate"]`,
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("Attempt Gateway environment missing %q: %s", required, combined)
		}
	}
}

func TestAutomationPromptAllowsToolsWithoutInteractiveApproval(t *testing.T) {
	prompt := agentPrompt(sourcedomain.TaskContract{RunID: "run-1"}, []byte("# Test Skill"))
	for _, required := range []string{"不要请求交互确认", "本机工具", "Shell", "网络能力", "执行尝试的工作目录"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("automation prompt missing %q: %s", required, prompt)
		}
	}
	for _, forbidden := range []string{"不得调用网络", "不得执行 Shell"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("automation prompt still contains obsolete restriction %q", forbidden)
		}
	}
}

func TestAdapterLoadsOnlyFrozenAutomationWorkspaceResources(t *testing.T) {
	contract := sourcedomain.TaskContract{
		ContractVersion: "1.0", ContractID: "snapshot-1", RunID: "run-1", TaskType: "knowledge_extract",
		Project: workspacedomain.Project{ID: "project-1"}, Sources: []sourcedomain.ContractSource{}, InputSnapshotID: "snapshot-1", OutputSchema: sourcedomain.KnowledgeCandidatesSchema,
		Capability: catalogdomain.Capability{ID: sourcedomain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: sourcedomain.TaskContractSchema, OutputSchema: sourcedomain.KnowledgeCandidatesSchema, Digest: "sha256:" + strings.Repeat("a", 64), LocalOnly: true},
	}
	root := filepath.Join(t.TempDir(), "attempt-1")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	contractBody, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{
		"contract.json":      contractBody,
		"output.schema.json": []byte(`{"type":"object"}`),
		"SKILL.md":           []byte("# Test Skill\n"),
	} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o400); err != nil {
			t.Fatal(err)
		}
	}
	directory, loaded, schema, skill, err := loadWorkspace(root)
	if err != nil || directory != root || loaded.RunID != contract.RunID || string(schema) != `{"type":"object"}` || string(skill) != "# Test Skill\n" {
		t.Fatalf("loaded workspace: directory=%s contract=%#v schema=%s skill=%s err=%v", directory, loaded, schema, skill, err)
	}
	contractPath := filepath.Join(root, "contract.json")
	if err := os.Chmod(contractPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := loadWorkspace(root); err == nil {
		t.Fatal("writable frozen contract unexpectedly accepted")
	}
}
