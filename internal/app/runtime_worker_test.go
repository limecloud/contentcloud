package app

import (
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestRuntimeWorkerFenceOwnerAndTerminalProtocol(t *testing.T) {
	store := memory.New()
	service := New(store, nil)
	session, err := service.Register(t.Context(), "runtime-worker@example.com", "long-enough-password", "Worker", "Runtime Worker")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sop := domain.SOPVersion{ID: "worker-sop-v1", TenantID: actor.TenantID, SOPID: "worker-sop", Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: "Worker SOP", Status: "published", DefaultExecutionMode: "agent", Digest: "sha256:" + strings.Repeat("a", 64), Stages: []domain.StageDefinition{{ID: "generate", Name: "Generate", Order: 10, OutputSchema: "contentcloud.worker-output/1.0", ExecutionModes: []string{"agent"}}}}
	started, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: actor.TenantID, ProjectID: project.ID, WorkTaskID: "worker-task-1", BusinessType: "worker.test", SOP: sop, BindingDigest: "sha256:" + strings.Repeat("b", 64), InputDigest: "sha256:" + strings.Repeat("c", 64), RuntimePolicyID: "runtime-policy/test", ContractMajor: 1, CreatedBy: actor.UserID, IdempotencyKey: "worker-task-1"})
	if err != nil {
		t.Fatal(err)
	}
	worker := Actor{UserID: actor.UserID, TenantID: actor.TenantID, Type: "worker"}
	handle, err := service.PrepareRuntimeWorker(t.Context(), worker, RuntimeWorkerPrepareInput{JobRunID: started.Job.ID, HarnessKind: "fake", Role: "worker", ExecutionProfileID: "profile-test", MaxTokens: 1024, BudgetMinor: 100})
	if err != nil {
		t.Fatal(err)
	}
	if handle.Attempt.LeaseOwner != "worker:"+actor.UserID || handle.Attempt.FenceToken == "" {
		t.Fatalf("worker lease is not bound to the authenticated owner: %#v", handle.Attempt)
	}
	if _, err := service.HeartbeatRuntimeWorker(t.Context(), worker, RuntimeWorkerHeartbeatInput{AttemptID: handle.Attempt.ID, FenceToken: "wrong"}); err == nil {
		t.Fatal("stale fence token must be rejected")
	}
	active, err := service.ActivateRuntimeWorker(t.Context(), worker, RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: agentadapter.AgentSessionRef{TenantID: actor.TenantID, HarnessKind: "fake", SessionID: "session-worker-1"}})
	if err != nil {
		t.Fatal(err)
	}
	finalized, err := service.FinalizeRuntimeWorker(t.Context(), worker, RuntimeWorkerFinalizeInput{AttemptID: active.Attempt.ID, FenceToken: active.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, SafeSummary: map[string]any{"status": "ok"}}, "worker-test")
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Handle.Attempt.State != domain.RuntimeAttemptSucceeded || finalized.Job.State != domain.JobRunCompleted {
		t.Fatalf("worker protocol did not converge Runtime state: %#v %#v", finalized.Handle.Attempt, finalized.Job)
	}
	if _, err := service.FinalizeRuntimeWorker(t.Context(), worker, RuntimeWorkerFinalizeInput{AttemptID: active.Attempt.ID, FenceToken: active.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, SafeSummary: map[string]any{"status": "ok"}}, "worker-test-retry"); err != nil {
		t.Fatalf("identical terminal worker retry must be idempotent: %v", err)
	}
	if _, err := service.FinalizeRuntimeWorker(t.Context(), worker, RuntimeWorkerFinalizeInput{AttemptID: active.Attempt.ID, FenceToken: "wrong", State: domain.RuntimeAttemptSucceeded, SafeSummary: map[string]any{"status": "ok"}}, "worker-test-wrong-fence"); err == nil {
		t.Fatal("terminal worker retry must verify the original fence digest")
	}
	if _, err := service.FinalizeRuntimeWorker(t.Context(), worker, RuntimeWorkerFinalizeInput{AttemptID: active.Attempt.ID, FenceToken: active.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, SafeSummary: map[string]any{"status": "changed"}}, "worker-test"); err == nil {
		t.Fatal("terminal RuntimeAttempt must reject a different result")
	}
}

func TestPrepareNextRuntimeWorkerUsesTenantPriorityAndSkipsPausedJobs(t *testing.T) {
	store := memory.New()
	service := New(store, nil)
	session, err := service.Register(t.Context(), "runtime-fairness@example.com", "long-enough-password", "Worker", "Runtime Fairness")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sop := domain.SOPVersion{ID: "fair-sop-v1", TenantID: actor.TenantID, SOPID: "fair-sop", Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: "Fair SOP", Status: "published", DefaultExecutionMode: "agent", Digest: "sha256:" + strings.Repeat("d", 64), Stages: []domain.StageDefinition{{ID: "generate", Name: "Generate", Order: 10, OutputSchema: "contentcloud.worker-output/1.0", ExecutionModes: []string{"agent"}}}}
	high, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: actor.TenantID, ProjectID: project.ID, WorkTaskID: "fair-high", BusinessType: "worker.test", SOP: sop, BindingDigest: "sha256:" + strings.Repeat("e", 64), InputDigest: "sha256:" + strings.Repeat("f", 64), RuntimePolicyID: "runtime-policy/test", ContractMajor: 1, Priority: 100, CreatedBy: actor.UserID, IdempotencyKey: "fair-high"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	low, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: actor.TenantID, ProjectID: project.ID, WorkTaskID: "fair-low", BusinessType: "worker.test", SOP: sop, BindingDigest: "sha256:" + strings.Repeat("e", 64), InputDigest: "sha256:" + strings.Repeat("f", 64), RuntimePolicyID: "runtime-policy/test", ContractMajor: 1, Priority: 1, CreatedBy: actor.UserID, IdempotencyKey: "fair-low"})
	if err != nil {
		t.Fatal(err)
	}
	worker := Actor{UserID: actor.UserID, TenantID: actor.TenantID, Type: "worker"}
	prepare := RuntimeWorkerPrepareNextInput{RuntimeWorkerPrepareInput: RuntimeWorkerPrepareInput{HarnessKind: "fake", Role: "worker", ExecutionProfileID: "profile-test", MaxTokens: 1024, BudgetMinor: 100}}
	handle, err := service.PrepareNextRuntimeWorker(t.Context(), worker, prepare)
	if err != nil {
		t.Fatal(err)
	}
	if handle.Attempt.JobRunID != high.Job.ID {
		t.Fatalf("tenant scheduler ignored Runtime priority: got %s want %s (newer low job %s)", handle.Attempt.JobRunID, high.Job.ID, low.Job.ID)
	}
	if _, err := service.Runtime().Pause(t.Context(), actor.TenantID, high.Job.ID, "user", actor.UserID); err != nil {
		t.Fatal(err)
	}
	// The first node is already leased, so create another higher-priority job
	// and pause it before prepare_next selects a new candidate.
	paused, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: actor.TenantID, ProjectID: project.ID, WorkTaskID: "fair-paused", BusinessType: "worker.test", SOP: sop, BindingDigest: "sha256:" + strings.Repeat("e", 64), InputDigest: "sha256:" + strings.Repeat("f", 64), RuntimePolicyID: "runtime-policy/test", ContractMajor: 1, Priority: 200, CreatedBy: actor.UserID, IdempotencyKey: "fair-paused"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Runtime().Pause(t.Context(), actor.TenantID, paused.Job.ID, "user", actor.UserID); err != nil {
		t.Fatal(err)
	}
	handle, err = service.PrepareNextRuntimeWorker(t.Context(), worker, prepare)
	if err != nil {
		t.Fatal(err)
	}
	if handle.Attempt.JobRunID != low.Job.ID {
		t.Fatalf("tenant scheduler selected a paused or non-ready job: got %s want %s", handle.Attempt.JobRunID, low.Job.ID)
	}
}
