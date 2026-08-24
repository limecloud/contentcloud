package worker

import (
	"errors"
	"strings"
	"testing"
	"time"

	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
)

func TestProcessRuntimeEventsReapsExpiredAttemptAndRecordsHealth(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), nil)
	session, err := service.Identity.Register(t.Context(), "runtime-maintenance@example.com", "long-enough-password", "Runtime Worker", "Runtime Maintenance")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Runtime", ProductName: "Maintenance"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sop := catalogdomain.SOPVersion{ID: "runtime-maintenance-sop-v1", TenantID: actor.TenantID, SOPID: "runtime-maintenance-sop", Version: 1, SchemaVersion: catalogdomain.SOPSchemaVersion, Name: "Runtime Maintenance", Status: "published", DefaultExecutionMode: "agent", Digest: "sha256:" + strings.Repeat("a", 64), Stages: []catalogdomain.StageDefinition{{ID: "execute", Name: "Execute", Order: 10, OutputSchema: "contentcloud.runtime-maintenance/1.0", ExecutionModes: []string{"agent"}}}}
	started, err := service.Runtime.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: actor.TenantID, ProjectID: project.ID, WorkTaskID: "runtime-maintenance-task", BusinessType: "runtime.maintenance", SOP: sop, BindingDigest: "sha256:" + strings.Repeat("b", 64), InputDigest: "sha256:" + strings.Repeat("c", 64), RuntimePolicyID: "runtime-policy/maintenance", ContractMajor: 1, CreatedBy: actor.UserID, IdempotencyKey: "runtime-maintenance-job"})
	if err != nil {
		t.Fatal(err)
	}
	workerActor := actor
	workerActor.Type = "worker"
	handle, err := service.Runtime.Runtime().PrepareRemoteDispatch(t.Context(), contentruntime.DispatchInput{
		TenantID: actor.TenantID, JobRunID: started.Job.ID, Owner: "worker:" + actor.UserID,
		HarnessKind: "fake", Role: "node_executor", ExecutionProfileID: "runtime-policy/maintenance:fake:stage",
		MaxTokens: 512, LeaseFor: time.Second,
	}, agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, MaxParallelSessions: 128})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	dependencies := application.DependenciesFrom(store)
	run, err := ProcessRuntimeEvents(t.Context(), dependencies.Identity, dependencies.Runtime, service, "runtime-maintenance-worker", 50)
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
	if attempt.State != contentruntime.RuntimeAttemptExpired {
		t.Fatalf("expired RuntimeAttempt was not reaped: %#v", attempt)
	}
	for _, kind := range []string{contentruntime.RuntimeMaintenanceReaper, contentruntime.RuntimeMaintenanceDelivery, contentruntime.RuntimeMaintenanceCleanup} {
		heartbeat, err := store.RuntimeMaintenanceHeartbeat(t.Context(), actor.TenantID, kind)
		if err != nil || heartbeat.State != "succeeded" || heartbeat.LastSuccessAt == nil {
			t.Fatalf("Runtime maintenance heartbeat %s was not successful: %#v err=%v", kind, heartbeat, err)
		}
	}
}

func TestProcessRuntimeEventsReconcilesCleanupDiagnostics(t *testing.T) {
	store := memory.New()
	blobs := blob.NewMemory()
	service := application.NewWithBlob(application.DependenciesFrom(store), nil, blobs)
	session, err := service.Identity.Register(t.Context(), "runtime-cleanup-worker@example.com", "long-enough-password", "Cleanup Worker", "Cleanup Team")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	diagnostic := contentruntime.RuntimeCleanupDiagnostic{
		ID: "worker-cleanup-diagnostic", TenantID: actor.TenantID, ProjectID: "project-cleanup", TaskID: "task-cleanup", RequestID: "request-cleanup",
		ManifestDigest: "sha256:" + strings.Repeat("a", 64), ObjectKey: "media/cleanup-worker/final.mp4", CauseCode: "FINAL_RENDER_STORE_FAILED", CauseSummary: "最终产物写入失败",
		Status: contentruntime.RuntimeCleanupPending, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	if err := store.CreateRuntimeCleanupDiagnostic(t.Context(), diagnostic); err != nil {
		t.Fatal(err)
	}
	if err := blobs.Put(t.Context(), diagnostic.ObjectKey, []byte("rendered")); err != nil {
		t.Fatal(err)
	}

	run, err := ProcessRuntimeEvents(t.Context(), application.DependenciesFrom(store).Identity, application.DependenciesFrom(store).Runtime, service, "cleanup-worker", 50)
	if err != nil {
		t.Fatal(err)
	}
	if run.CleanupScanned != 1 || run.CleanupClaimed != 1 || run.CleanupCleaned != 1 || run.CleanupFailed != 0 {
		t.Fatalf("cleanup diagnostics were not reconciled: %#v", run)
	}
	final, err := store.RuntimeCleanupDiagnostic(t.Context(), actor.TenantID, diagnostic.ID)
	if err != nil || final.Status != contentruntime.RuntimeCleanupCleaned {
		t.Fatalf("cleanup diagnostic did not converge: %#v err=%v", final, err)
	}
	if _, err := blobs.Get(t.Context(), diagnostic.ObjectKey); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("cleanup worker left the Blob object: %v", err)
	}
	heartbeat, err := store.RuntimeMaintenanceHeartbeat(t.Context(), actor.TenantID, contentruntime.RuntimeMaintenanceCleanup)
	if err != nil || heartbeat.State != "succeeded" || heartbeat.LastSuccessAt == nil {
		t.Fatalf("cleanup worker heartbeat missing: %#v err=%v", heartbeat, err)
	}
}
