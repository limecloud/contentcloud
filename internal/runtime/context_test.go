package runtime

import (
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestBuildContextViewIsReferenceOnlyAndDeterministic(t *testing.T) {
	created := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	input := ContextViewInput{TenantID: "tenant-1", JobRunID: "job-1", NodeRunID: "node-1", AttemptID: "attempt-1", InputRefs: []string{"asset:2", "asset:1", "asset:1"}, StateRefs: []string{"brief@2"}, EventRefs: []string{"event:3"}, AllowedTools: []string{"state.get", "artifact.resolve", "state.get"}, MaxTokens: 4096, BudgetMinor: 120, CreatedAt: created, ExpiresAt: created.Add(5 * time.Minute)}
	first, err := BuildContextView(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildContextView(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || first.ID == second.ID {
		t.Fatalf("context digest/id contract broken: first=%#v second=%#v", first, second)
	}
	if len(first.InputRefs) != 2 || first.InputRefs[0] != "asset:1" || len(first.AllowedTools) != 2 {
		t.Fatalf("references were not normalized: %#v", first)
	}
	if err := first.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentInstanceStateAndBudgetAreBounded(t *testing.T) {
	agent := domain.AgentInstance{ID: "agent-1", TenantID: "tenant-1", JobRunID: "job-1", NodeRunID: "node-1", Role: "researcher", HarnessKind: "fake", ExecutionProfileID: "profile-1", ContextViewID: "view-1", State: domain.AgentCreated, Depth: 1, RemainingDescendants: 2, BudgetMinor: 100, UsedCostMinor: 0, Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := agent.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, next := range []string{domain.AgentRunnable, domain.AgentActive, domain.AgentWaitingChildren, domain.AgentRunnable, domain.AgentActive, domain.AgentCompleted} {
		if err := agent.Transition(next); err != nil {
			t.Fatalf("transition %s: %v", next, err)
		}
		agent.State = next
	}
	if err := agent.Transition(domain.AgentActive); err == nil {
		t.Fatal("terminal AgentInstance unexpectedly resumed")
	}
	agent.UsedCostMinor = 101
	if err := agent.Validate(); err == nil {
		t.Fatal("budget overrun unexpectedly validated")
	}
}
