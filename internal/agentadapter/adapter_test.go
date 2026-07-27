package agentadapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/automationworkspace"
	"github.com/limecloud/contentcloud/internal/domain"
)

func TestDecodeClaudeStructuredOutput(t *testing.T) {
	pkg := domain.ScriptPackage{SchemaVersion: "1.1", Deliverability: "blocked", BlockedReasons: []domain.BlockReason{{Code: "missing", Message: "missing", NextAction: "review"}}}
	body, _ := json.Marshal(map[string]any{"structured_output": pkg})
	output, err := decodeClaudeOutput(body)
	if err != nil {
		t.Fatal(err)
	}
	var got domain.ScriptPackage
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != "1.1" || got.Deliverability != "blocked" {
		t.Fatalf("unexpected package: %#v", got)
	}
}

func TestAgentEnvironmentDoesNotInheritUnrelatedSecret(t *testing.T) {
	t.Setenv("CONTENTCLOUD_TEST_SECRET", "do-not-inherit")
	t.Setenv("CONTENTCLOUD_DEVICE_TOKEN", "dt_do-not-inherit")
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_do-not-inherit")
	t.Setenv("CONTENTCLOUD_RUN_TOKEN", "rt_do-not-inherit")
	for _, value := range agentEnvironment("codex") {
		if strings.Contains(value, "do-not-inherit") {
			t.Fatalf("ContentCloud or unrelated secret inherited: %s", value)
		}
	}
}

func TestAdapterLoadsOnlyFrozenAutomationWorkspaceResources(t *testing.T) {
	now := time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC)
	contract := domain.TaskContract{
		ContractVersion: "1.0", ContractID: "snapshot-1", RunID: "run-1", TaskType: "script_generate",
		Project: domain.Project{ID: "project-1"}, InputSnapshotID: "snapshot-1", OutputSchema: domain.ScriptPackageSchema,
		Capability: domain.Capability{ID: domain.ScriptCapability, Version: "1.1.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.ScriptPackageSchema, Digest: "sha256:" + strings.Repeat("a", 64), LocalOnly: true},
	}
	workspace, err := automationworkspace.Begin(automationworkspace.Options{
		BaseDir: filepath.Join(t.TempDir(), "automation"), AttemptID: "attempt-1", RunID: "run-1", ProjectID: "project-1",
		Contract: contract, OutputSchema: []byte(`{"type":"object"}`), Skill: []byte("# Test Skill\n"), Now: now, ExpiresAt: now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	directory, loaded, schema, skill, err := loadWorkspace(workspace.Root)
	if err != nil || directory != workspace.Root || loaded.RunID != contract.RunID || string(schema) != `{"type":"object"}` || string(skill) != "# Test Skill\n" {
		t.Fatalf("loaded workspace: directory=%s contract=%#v schema=%s skill=%s err=%v", directory, loaded, schema, skill, err)
	}
	contractPath := filepath.Join(workspace.Root, "contract.json")
	if err := os.Chmod(contractPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := loadWorkspace(workspace.Root); err == nil {
		t.Fatal("writable frozen contract unexpectedly accepted")
	}
}
