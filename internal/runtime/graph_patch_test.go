package runtime

import (
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestApplyGraphPatchOnlyAppendsNewDownstreamNodes(t *testing.T) {
	plan, err := NewCompiler(domain.DefaultRuntimeLimits()).CompileSOP(testSOP(), "tenant-1", "user-1", fixedRuntimeTime())
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyGraphPatch(plan, 1, GraphPatch{ExpectedGraphVersion: 1, IdempotencyKey: "fanout-1", Reason: "为新受众创建脚本候选", AddNodes: []domain.JobPlanNode{{Key: "audience:1", Kind: "stage", Name: "受众一脚本", OutputSchema: "contentcloud.script/1.0", DependsOn: []string{"stage:sources"}, RetryMaxAttempts: 1}}, CancelPendingNodeKeys: []string{"stage:delivery"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.GraphVersion != 2 || result.Plan.Digest == plan.Digest || len(result.Plan.Nodes) != len(plan.Nodes)+1 || len(result.CancelPendingNodeKeys) != 1 {
		t.Fatalf("unexpected graph patch result: %#v", result)
	}
}

func TestApplyGraphPatchRejectsExistingDependencyMutationAndCycles(t *testing.T) {
	plan, err := NewCompiler(domain.DefaultRuntimeLimits()).CompileSOP(testSOP(), "tenant-1", "user-1", fixedRuntimeTime())
	if err != nil {
		t.Fatal(err)
	}
	_, err = ApplyGraphPatch(plan, 1, GraphPatch{ExpectedGraphVersion: 1, IdempotencyKey: "invalid-existing", Reason: "修改已有节点", AddEdges: []domain.JobPlanEdge{{From: "stage:delivery", To: "stage:sources"}}})
	if err == nil {
		t.Fatal("existing node dependency mutation unexpectedly accepted")
	}
	_, err = ApplyGraphPatch(plan, 1, GraphPatch{ExpectedGraphVersion: 1, IdempotencyKey: "invalid-cycle", Reason: "创建环", AddNodes: []domain.JobPlanNode{{Key: "cycle", Kind: "stage", Name: "环", OutputSchema: "contentcloud.test/1.0", DependsOn: []string{"cycle"}}}})
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
