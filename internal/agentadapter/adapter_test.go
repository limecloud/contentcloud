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

func TestAutomationStrategiesMatchAvailableRegistryCapabilities(t *testing.T) {
	for _, client := range Clients() {
		_, implemented := automationFactories[client.ID]
		available := client.CapabilityStatus(CapabilityLocalAutomation) == SupportAvailable
		if implemented != available {
			t.Fatalf("automation strategy drift for %s: implemented=%t available=%t", client.ID, implemented, available)
		}
	}
	adapter, err := Select("claude")
	if err != nil || adapter.Kind() != "claude-code" {
		t.Fatalf("legacy Claude alias was not normalized: adapter=%#v err=%v", adapter, err)
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

func TestSelectionAndHandoffFailClosedForUnsupportedInputs(t *testing.T) {
	for _, client := range []string{"unknown-agent", "cursor"} {
		_, err := Select(client)
		var domainError *domain.Error
		if !errors.As(err, &domainError) {
			t.Fatalf("%s selection did not return a domain error: %v", client, err)
		}
		if client == "unknown-agent" && domainError.Code != "AGENT_CLIENT_INVALID" {
			t.Fatalf("unknown client error = %s", domainError.Code)
		}
		if client == "cursor" && domainError.Code != "AGENT_CLIENT_CAPABILITY_UNAVAILABLE" {
			t.Fatalf("planned client error = %s", domainError.Code)
		}
	}
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

func TestDecodeClaudeStructuredOutput(t *testing.T) {
	pkg := domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []domain.KnowledgeCandidate{}, Warnings: []string{"missing source"}}
	body, _ := json.Marshal(map[string]any{"structured_output": pkg})
	output, err := decodeClaudeOutput(body)
	if err != nil {
		t.Fatal(err)
	}
	var got domain.KnowledgeExtractionPackage
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != "1.0" || len(got.Warnings) != 1 {
		t.Fatalf("unexpected package: %#v", got)
	}
}

func TestAgentEnvironmentDoesNotInheritUnrelatedSecret(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "provider-key")
	t.Setenv("CONTENTCLOUD_TEST_SECRET", "do-not-inherit")
	t.Setenv("CONTENTCLOUD_DEVICE_TOKEN", "dt_do-not-inherit")
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_do-not-inherit")
	t.Setenv("CONTENTCLOUD_RUN_TOKEN", "rt_do-not-inherit")
	providerInherited := false
	for _, value := range agentEnvironment("codex") {
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

func TestAutomationArgumentsUsePreauthorizedFullAccess(t *testing.T) {
	codex := strings.Join(codexRunArguments("/tmp/attempt", "/tmp/attempt/result.json"), " ")
	if !strings.Contains(codex, "--dangerously-bypass-approvals-and-sandbox") || strings.Contains(codex, "read-only") {
		t.Fatalf("Codex automation arguments are not full access: %s", codex)
	}
	claude := strings.Join(claudeRunArguments([]byte(`{"type":"object"}`), "prompt"), " ")
	if !strings.Contains(claude, "--permission-mode bypassPermissions") || strings.Contains(claude, "--tools ") || strings.Contains(claude, "--safe-mode") {
		t.Fatalf("Claude automation arguments disable autonomous execution: %s", claude)
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
