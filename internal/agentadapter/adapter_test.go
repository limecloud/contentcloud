package agentadapter

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
)

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
	var domainError *domain.Error
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
	var domainError *domain.Error
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

func TestAutomationPromptAllowsToolsWithoutInteractiveApproval(t *testing.T) {
	prompt := agentPrompt(domain.TaskContract{RunID: "run-1"}, []byte("# Test Skill"))
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
	contract := domain.TaskContract{
		ContractVersion: "1.0", ContractID: "snapshot-1", RunID: "run-1", TaskType: "knowledge_extract",
		Project: domain.Project{ID: "project-1"}, Sources: []domain.ContractSource{}, InputSnapshotID: "snapshot-1", OutputSchema: domain.KnowledgeCandidatesSchema,
		Capability: domain.Capability{ID: domain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, Digest: "sha256:" + strings.Repeat("a", 64), LocalOnly: true},
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
