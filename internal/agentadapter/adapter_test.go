package agentadapter

import (
	"encoding/json"
	"testing"

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
	for _, value := range agentEnvironment("codex") {
		if value == "CONTENTCLOUD_TEST_SECRET=do-not-inherit" {
			t.Fatal("unrelated environment secret inherited")
		}
	}
}
