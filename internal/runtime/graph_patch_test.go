package runtime_test

import (
	"testing"
	"time"

	. "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

func TestApplyGraphPatchOnlyAppendsNewDownstreamNodes(t *testing.T) {
	plan, err := NewCompiler(DefaultRuntimeLimits()).CompileSOP(testSOP(), "tenant-1", "user-1", fixedRuntimeTime())
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyGraphPatch(plan, 1, GraphPatch{ExpectedGraphVersion: 1, IdempotencyKey: "fanout-1", Reason: "为新受众创建脚本候选", AddNodes: []JobPlanNode{{Key: "audience:1", Kind: "stage", Name: "受众一脚本", OutputSchema: "contentcloud.script/1.0", DependsOn: []string{"stage:sources"}, RetryMaxAttempts: 1}}, CancelPendingNodeKeys: []string{"stage:delivery"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.GraphVersion != 2 || result.Plan.Digest == plan.Digest || len(result.Plan.Nodes) != len(plan.Nodes)+1 || len(result.CancelPendingNodeKeys) != 1 {
		t.Fatalf("unexpected graph patch result: %#v", result)
	}
}

func TestPatchGraphPersistsRevisionNodesAndEventAtomically(t *testing.T) {
	repo := memory.New()
	service := New(repo, fixedRuntimeTime)
	started, err := service.Start(t.Context(), testStartInput("task-graph", "graph-job"))
	if err != nil {
		t.Fatal(err)
	}
	patch := GraphPatch{
		ExpectedGraphVersion: 1,
		IdempotencyKey:       "expand-audiences",
		Reason:               "为已确认受众生成候选",
		AddNodes: []JobPlanNode{{
			Key: "audience:1", Kind: "stage", Name: "受众一脚本", OutputSchema: "contentcloud.script/1.0",
			DependsOn: []string{"stage:sources"}, RetryMaxAttempts: 1,
		}},
	}
	result, err := service.PatchGraph(t.Context(), "tenant-1", started.Job.ID, "supervisor-1", patch)
	if err != nil {
		t.Fatal(err)
	}
	if result.Plan.BaseRevisionID != started.Plan.ID || result.Plan.GraphVersion != 2 || result.Plan.PatchKey != patch.IdempotencyKey {
		t.Fatalf("graph revision metadata was not frozen: %#v", result.Plan)
	}
	job, err := service.Job(t.Context(), "tenant-1", started.Job.ID)
	if err != nil || job.PlanRevisionID != result.Plan.ID || job.PlanDigest != result.Plan.Digest {
		t.Fatalf("job did not advance to the graph revision: %#v err=%v", job, err)
	}
	nodes, err := service.Nodes(t.Context(), "tenant-1", started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, node := range nodes {
		if node.NodeKey == "audience:1" {
			found = node.State == NodePending
		}
	}
	if !found {
		t.Fatalf("graph patch did not persist its NodeRun: %#v", nodes)
	}
	events, err := service.Events(t.Context(), "tenant-1", started.Job.ID, 0)
	if err != nil || events[len(events)-1].Type != "graph.patched" {
		t.Fatalf("graph patch event was not committed: %#v err=%v", events, err)
	}
	if _, err := service.PatchGraph(t.Context(), "tenant-1", started.Job.ID, "supervisor-1", patch); err == nil {
		t.Fatal("stale graph patch unexpectedly succeeded")
	}
}

func TestApplyGraphPatchRejectsExistingDependencyMutationAndCycles(t *testing.T) {
	plan, err := NewCompiler(DefaultRuntimeLimits()).CompileSOP(testSOP(), "tenant-1", "user-1", fixedRuntimeTime())
	if err != nil {
		t.Fatal(err)
	}
	_, err = ApplyGraphPatch(plan, 1, GraphPatch{ExpectedGraphVersion: 1, IdempotencyKey: "invalid-existing", Reason: "修改已有节点", AddEdges: []JobPlanEdge{{From: "stage:delivery", To: "stage:sources"}}})
	if err == nil {
		t.Fatal("existing node dependency mutation unexpectedly accepted")
	}
	_, err = ApplyGraphPatch(plan, 1, GraphPatch{ExpectedGraphVersion: 1, IdempotencyKey: "invalid-cycle", Reason: "创建环", AddNodes: []JobPlanNode{{Key: "cycle", Kind: "stage", Name: "环", OutputSchema: "contentcloud.test/1.0", DependsOn: []string{"cycle"}}}})
	if err == nil {
		t.Fatal("cycle graph patch unexpectedly accepted")
	}
	_, err = ApplyGraphPatch(plan, 2, GraphPatch{ExpectedGraphVersion: 1, IdempotencyKey: "stale", Reason: "旧版本"})
	if err == nil {
		t.Fatal("stale graph version unexpectedly accepted")
	}
}

func fixedRuntimeTime() time.Time {
	return time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
}
