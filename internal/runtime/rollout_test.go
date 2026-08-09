package runtime

import (
	"testing"

	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestRuntimeRolloutPolicyBlocksNewAdmissionOutsideCanary(t *testing.T) {
	repo := memory.New()
	service := New(repo, fixedRuntimeTime)
	input := testStartInput("rollout-task", "rollout-job")
	service.SetRolloutPolicy(RolloutPolicy{AdmissionEnabled: false, DynamicGraphEnabled: false})
	if _, err := service.Start(t.Context(), input); !hasDomainCode(err, "RUNTIME_ADMISSION_DISABLED") {
		t.Fatalf("disabled Runtime admission accepted a new JobRun: %v", err)
	}
	service.SetRolloutPolicy(RolloutPolicy{AdmissionEnabled: true, DynamicGraphEnabled: true, TenantIDs: []string{"tenant-canary"}})
	if _, err := service.Start(t.Context(), input); !hasDomainCode(err, "RUNTIME_ADMISSION_DISABLED") {
		t.Fatalf("non-canary tenant entered Runtime: %v", err)
	}
	input.TenantID = "tenant-canary"
	if _, err := service.Start(t.Context(), input); err != nil {
		t.Fatalf("canary tenant was rejected: %v", err)
	}
}

func TestRuntimeRolloutPolicyBlocksNewGraphMutations(t *testing.T) {
	repo := memory.New()
	service := New(repo, fixedRuntimeTime)
	started, err := service.Start(t.Context(), testStartInput("rollout-graph-task", "rollout-graph-job"))
	if err != nil {
		t.Fatal(err)
	}
	service.SetRolloutPolicy(RolloutPolicy{AdmissionEnabled: true, DynamicGraphEnabled: false})
	if _, err := service.PatchGraph(t.Context(), "tenant-1", started.Job.ID, "operator", GraphPatch{ExpectedGraphVersion: started.Plan.GraphVersion, IdempotencyKey: "rollout-patch", Reason: "must be blocked"}); !hasDomainCode(err, "RUNTIME_DYNAMIC_GRAPH_DISABLED") {
		t.Fatalf("disabled dynamic graph accepted a patch: %v", err)
	}
}
