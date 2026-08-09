package worker

import (
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestProcessRuntimeEventsReapsExpiredAttemptAndRecordsHealth(t *testing.T) {
	store := memory.New()
	service := app.New(store, nil)
	session, err := service.Register(t.Context(), "runtime-maintenance@example.com", "long-enough-password", "Runtime Worker", "Runtime Maintenance")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Runtime", ProductName: "Maintenance"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sop := domain.SOPVersion{ID: "runtime-maintenance-sop-v1", TenantID: actor.TenantID, SOPID: "runtime-maintenance-sop", Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: "Runtime Maintenance", Status: "published", DefaultExecutionMode: "agent", Digest: "sha256:" + strings.Repeat("a", 64), Stages: []domain.StageDefinition{{ID: "execute", Name: "Execute", Order: 10, OutputSchema: "contentcloud.runtime-maintenance/1.0", ExecutionModes: []string{"agent"}}}}
	started, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: actor.TenantID, ProjectID: project.ID, WorkTaskID: "runtime-maintenance-task", BusinessType: "runtime.maintenance", SOP: sop, BindingDigest: "sha256:" + strings.Repeat("b", 64), InputDigest: "sha256:" + strings.Repeat("c", 64), RuntimePolicyID: "runtime-policy/maintenance", ContractMajor: 1, CreatedBy: actor.UserID, IdempotencyKey: "runtime-maintenance-job"})
	if err != nil {
		t.Fatal(err)
	}
	workerActor := actor
	workerActor.Type = "worker"
	handle, err := service.PrepareRuntimeWorker(t.Context(), workerActor, app.RuntimeWorkerPrepareInput{JobRunID: started.Job.ID, HarnessKind: "fake", Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, MaxParallelSessions: 128}, Role: "worker", ExecutionProfileID: "profile-test", MaxTokens: 512, LeaseForSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	run, err := ProcessRuntimeEvents(t.Context(), store, service, "runtime-maintenance-worker", 50)
	if err != nil {
		t.Fatal(err)
	}
	if run.ReaperTenants != 1 {
		t.Fatalf("Runtime worker did not run the reaper: %#v", run)
	}
	attempt, err := store.RuntimeAttempt(t.Context(), actor.TenantID, handle.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.State != domain.RuntimeAttemptExpired {
		t.Fatalf("expired RuntimeAttempt was not reaped: %#v", attempt)
	}
	for _, kind := range []string{domain.RuntimeMaintenanceReaper, domain.RuntimeMaintenanceDelivery} {
		heartbeat, err := store.RuntimeMaintenanceHeartbeat(t.Context(), actor.TenantID, kind)
		if err != nil || heartbeat.State != "succeeded" || heartbeat.LastSuccessAt == nil {
			t.Fatalf("Runtime maintenance heartbeat %s was not successful: %#v err=%v", kind, heartbeat, err)
		}
	}
}
