package worker

import (
	"strings"
	"testing"
	"time"

	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
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
	for _, kind := range []string{contentruntime.RuntimeMaintenanceReaper, contentruntime.RuntimeMaintenanceDelivery} {
		heartbeat, err := store.RuntimeMaintenanceHeartbeat(t.Context(), actor.TenantID, kind)
		if err != nil || heartbeat.State != "succeeded" || heartbeat.LastSuccessAt == nil {
			t.Fatalf("Runtime maintenance heartbeat %s was not successful: %#v err=%v", kind, heartbeat, err)
		}
	}
}
