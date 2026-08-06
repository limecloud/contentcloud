package runtime

import (
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func startAgentRuntime(t *testing.T) (*Service, *memory.Store, StartResult, domain.NodeRun, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	repo := memory.New()
	service := New(repo, func() time.Time { return now })
	started, err := service.Start(t.Context(), StartInput{TenantID: "tenant-1", ProjectID: "project-1", WorkTaskID: "task-agent", SOP: testSOP(), CreatedBy: "user-1", IdempotencyKey: "agent-job"})
	if err != nil {
		t.Fatal(err)
	}
	if len(started.Nodes) == 0 {
		t.Fatal("runtime did not create nodes")
	}
	return service, repo, started, started.Nodes[0], now
}

func createAgentContext(t *testing.T, service *Service, started StartResult, node domain.NodeRun, now time.Time, attemptID string, tools []string, budget int64) domain.ContextView {
	t.Helper()
	view, err := service.CreateContextView(t.Context(), ContextViewInput{
		TenantID:     started.Job.TenantID,
		JobRunID:     started.Job.ID,
		NodeRunID:    node.ID,
		AttemptID:    attemptID,
		InputRefs:    []string{"asset:source-1"},
		StateRefs:    []string{"brief@1"},
		AllowedTools: tools,
		MaxTokens:    4096,
		BudgetMinor:  budget,
		CreatedAt:    now,
		ExpiresAt:    now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestContextViewAndAgentInstancePersistence(t *testing.T) {
	service, repo, started, node, now := startAgentRuntime(t)
	view := createAgentContext(t, service, started, node, now, "attempt-root", []string{"artifact.resolve", "state.get"}, 500)

	storedView, err := repo.ContextView(t.Context(), "tenant-1", view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedView.Digest != view.Digest || storedView.JobRunID != started.Job.ID {
		t.Fatalf("unexpected stored ContextView: %#v", storedView)
	}
	views, err := service.ContextViews(t.Context(), "tenant-1", started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].ID != view.ID {
		t.Fatalf("unexpected ContextView list: %#v", views)
	}

	agent, err := service.CreateAgentInstance(t.Context(), AgentInstanceInput{
		TenantID:             "tenant-1",
		JobRunID:             started.Job.ID,
		NodeRunID:            node.ID,
		Role:                 "supervisor",
		HarnessKind:          "fake",
		ExecutionProfileID:   "profile-root",
		ContextViewID:        view.ID,
		RemainingDescendants: 3,
		BudgetMinor:          500,
	})
	if err != nil {
		t.Fatal(err)
	}
	agents, err := service.AgentInstances(t.Context(), "tenant-1", started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].ID != agent.ID || agents[0].Depth != 0 {
		t.Fatalf("unexpected AgentInstance list: %#v", agents)
	}

	transitioned, err := service.TransitionAgentInstance(t.Context(), "tenant-1", agent.ID, domain.AgentRunnable, "session-1", 20, agent.Version)
	if err != nil {
		t.Fatal(err)
	}
	if transitioned.Version != 2 || transitioned.UsedCostMinor != 20 {
		t.Fatalf("unexpected AgentInstance transition: %#v", transitioned)
	}
	if err := repo.SaveAgentInstance(t.Context(), transitioned, agent.Version); err == nil {
		t.Fatal("stale AgentInstance version unexpectedly updated")
	}
}

func TestChildAgentCannotEscalateBudgetOrTools(t *testing.T) {
	service, _, started, node, now := startAgentRuntime(t)
	parentView := createAgentContext(t, service, started, node, now, "attempt-parent", []string{"artifact.resolve", "state.get"}, 100)
	parent, err := service.CreateAgentInstance(t.Context(), AgentInstanceInput{
		TenantID:             "tenant-1",
		JobRunID:             started.Job.ID,
		NodeRunID:            node.ID,
		Role:                 "supervisor",
		HarnessKind:          "fake",
		ExecutionProfileID:   "profile-parent",
		ContextViewID:        parentView.ID,
		RemainingDescendants: 2,
		BudgetMinor:          100,
	})
	if err != nil {
		t.Fatal(err)
	}

	budgetView := createAgentContext(t, service, started, node, now, "attempt-budget", []string{"state.get"}, 101)
	_, err = service.CreateAgentInstance(t.Context(), AgentInstanceInput{
		TenantID:              "tenant-1",
		JobRunID:              started.Job.ID,
		NodeRunID:             node.ID,
		ParentAgentInstanceID: parent.ID,
		Role:                  "researcher",
		HarnessKind:           "fake",
		ExecutionProfileID:    "profile-child",
		ContextViewID:         budgetView.ID,
		BudgetMinor:           101,
	})
	if err == nil {
		t.Fatal("child AgentInstance unexpectedly exceeded parent budget")
	}

	toolView := createAgentContext(t, service, started, node, now, "attempt-tools", []string{"state.get", "web.search"}, 50)
	_, err = service.CreateAgentInstance(t.Context(), AgentInstanceInput{
		TenantID:              "tenant-1",
		JobRunID:              started.Job.ID,
		NodeRunID:             node.ID,
		ParentAgentInstanceID: parent.ID,
		Role:                  "researcher",
		HarnessKind:           "fake",
		ExecutionProfileID:    "profile-child",
		ContextViewID:         toolView.ID,
		BudgetMinor:           50,
	})
	if err == nil {
		t.Fatal("child AgentInstance unexpectedly expanded parent tool permissions")
	}
}

func TestChildAgentCreationConsumesParentDescendantQuota(t *testing.T) {
	service, _, started, node, now := startAgentRuntime(t)
	parentView := createAgentContext(t, service, started, node, now, "attempt-quota-parent", []string{"state.get"}, 100)
	parent, err := service.CreateAgentInstance(t.Context(), AgentInstanceInput{
		TenantID:             "tenant-1",
		JobRunID:             started.Job.ID,
		NodeRunID:            node.ID,
		Role:                 "supervisor",
		HarnessKind:          "fake",
		ExecutionProfileID:   "profile-parent",
		ContextViewID:        parentView.ID,
		RemainingDescendants: 2,
		BudgetMinor:          100,
	})
	if err != nil {
		t.Fatal(err)
	}
	childView := createAgentContext(t, service, started, node, now, "attempt-quota-child", []string{"state.get"}, 50)
	child, err := service.CreateAgentInstance(t.Context(), AgentInstanceInput{
		TenantID:              "tenant-1",
		JobRunID:              started.Job.ID,
		NodeRunID:             node.ID,
		ParentAgentInstanceID: parent.ID,
		Role:                  "researcher",
		HarnessKind:           "fake",
		ExecutionProfileID:    "profile-child",
		ContextViewID:         childView.ID,
		RemainingDescendants:  1,
		BudgetMinor:           50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.Depth != 1 {
		t.Fatalf("unexpected child depth: %d", child.Depth)
	}
	updatedParent, err := service.AgentInstance(t.Context(), "tenant-1", parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedParent.RemainingDescendants != 0 || updatedParent.Version != parent.Version+1 {
		t.Fatalf("parent quota was not reserved atomically: %#v", updatedParent)
	}

	secondView := createAgentContext(t, service, started, node, now, "attempt-quota-second", []string{"state.get"}, 10)
	_, err = service.CreateAgentInstance(t.Context(), AgentInstanceInput{
		TenantID:              "tenant-1",
		JobRunID:              started.Job.ID,
		NodeRunID:             node.ID,
		ParentAgentInstanceID: parent.ID,
		Role:                  "researcher",
		HarnessKind:           "fake",
		ExecutionProfileID:    "profile-child",
		ContextViewID:         secondView.ID,
		BudgetMinor:           10,
	})
	if err == nil {
		t.Fatal("exhausted parent unexpectedly created another child")
	}
}
