package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	storepg "github.com/limecloud/contentcloud/internal/store/postgres"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestRuntimeRoleEnforcesTenantRLS(t *testing.T) {
	databaseURL := os.Getenv("CONTENTCLOUD_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTENTCLOUD_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := storepg.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service := app.New(store, slog.Default())
	suffix := domain.NewID()
	aSession, err := service.Register(ctx, fmt.Sprintf("rls-a-%s@example.com", suffix), "long-enough-password", "A", "RLS A "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	bSession, err := service.Register(ctx, fmt.Sprintf("rls-b-%s@example.com", suffix), "long-enough-password", "B", "RLS B "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	a, _, _ := service.SessionActor(ctx, aSession.ID)
	b, _, _ := service.SessionActor(ctx, bSession.ID)
	invite, err := service.CreateMembershipInvite(ctx, a, fmt.Sprintf("rls-b-%s@example.com", suffix), "reviewer", "")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	acceptErrors := make([]error, 2)
	var acceptWait sync.WaitGroup
	for index := range acceptErrors {
		acceptWait.Add(1)
		go func(index int) {
			defer acceptWait.Done()
			<-start
			_, acceptErrors[index] = service.AcceptMembershipInvite(ctx, b, invite.PlaintextToken, "")
		}(index)
	}
	close(start)
	acceptWait.Wait()
	acceptSuccesses := 0
	for _, acceptErr := range acceptErrors {
		if acceptErr == nil {
			acceptSuccesses++
			continue
		}
		var domainErr *domain.Error
		if !errors.As(acceptErr, &domainErr) || domainErr.Code != "INVITE_INVALID" {
			t.Fatalf("unexpected concurrent invite error: %v", acceptErr)
		}
	}
	if acceptSuccesses != 1 {
		t.Fatalf("PostgreSQL invite must be accepted exactly once, got %d successes", acceptSuccesses)
	}
	project, err := service.CreateProject(ctx, a, app.CreateProjectInput{BrandName: "Same", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.CreateConnectSession(ctx, a, project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(ctx, service, a, connect, app.ConnectDeviceInput{Hostname: "rls-local", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.WorkspaceActor(ctx, connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	diagnosticSummary := domain.BootstrapDiagnosticSummary{SchemaVersion: domain.BootstrapSchemaVersion, AttemptID: connected.BootstrapAttemptID, Platform: "darwin", Arch: "arm64", Versions: map[string]string{"contentcloud_cli": "test"}, Checks: []domain.BootstrapDiagnosticCheck{{CheckID: "runtime.node.version", Status: "passed"}}}
	firstDiagnostic, err := service.UploadBootstrapDiagnostic(ctx, workspaceActor, binding, diagnosticSummary)
	if err != nil {
		t.Fatal(err)
	}
	replayedDiagnostic, err := service.UploadBootstrapDiagnostic(ctx, workspaceActor, binding, diagnosticSummary)
	if err != nil || replayedDiagnostic.ID != firstDiagnostic.ID || !replayedDiagnostic.CreatedAt.Equal(firstDiagnostic.CreatedAt) {
		t.Fatalf("PostgreSQL diagnostic replay was not idempotent: first=%#v replayed=%#v error=%v", firstDiagnostic, replayedDiagnostic, err)
	}
	deviceActor, _, err := service.DeviceActor(ctx, connected.DeviceToken)
	if err != nil {
		t.Fatal(err)
	}
	daemonInstanceID := domain.NewID()
	_, err = service.ReportDaemonInstance(ctx, deviceActor, app.DaemonInstanceReportInput{
		ID: daemonInstanceID, ConnectionEpoch: 1, ReportSequence: 1, PID: 42,
		Version: "test", State: "connected", StartedAt: time.Now().UTC().Add(-time.Minute),
		WorkspaceObservations: []domain.DaemonWorkspaceObservation{{
			WorkspaceID: "rls-workspace-" + suffix, ProjectID: project.ID, Status: "ready",
			Reason: "integration_test", Generation: "sha256:" + strings.Repeat("c", 64), ObservedAt: time.Now().UTC(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestHash := domain.TokenHash("rls-input-" + suffix)
	inputSnapshot := domain.ContextSnapshot{
		ID: domain.NewID(), TenantID: a.TenantID, ProjectID: project.ID, BuilderVersion: "rls-test/1.0",
		SchemaVersion: domain.TaskContractSchema, Sources: []domain.ContractSource{}, InputVersions: map[string]string{},
		ManifestHash: manifestHash, CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateSnapshot(ctx, inputSnapshot); err != nil {
		t.Fatal(err)
	}
	sop := domain.SOPVersion{ID: "rls-runtime-sop-v1", TenantID: a.TenantID, SOPID: "rls-runtime-sop", Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: "RLS Runtime", Status: "published", DefaultExecutionMode: "agent", Stages: []domain.StageDefinition{{ID: "execute", Name: "执行", Order: 10, OutputSchema: domain.KnowledgeCandidatesSchema, RequiredCapabilities: []string{domain.KnowledgeExtractCapability}, ExecutionModes: []string{"agent"}}}}
	started, err := service.Runtime().Start(ctx, contentruntime.StartInput{TenantID: a.TenantID, ProjectID: project.ID, WorkTaskID: "rls-runtime-" + suffix, BusinessType: "rls.test", InputSnapshotID: inputSnapshot.ID, SOP: sop, BindingDigest: "sha256:" + strings.Repeat("a", 64), InputDigest: "sha256:" + manifestHash, RuntimePolicyID: "runtime-policy/rls", ContractMajor: 1, CreatedBy: a.UserID, IdempotencyKey: "rls-runtime-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := service.PrepareRuntimeWorker(ctx, deviceActor, app.RuntimeWorkerPrepareInput{JobRunID: started.Job.ID, DaemonInstanceID: daemonInstanceID, HarnessKind: "fake", Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, MaxParallelSessions: 128}, Role: "worker", ExecutionProfileID: "rls-test", MaxTokens: 512})
	if err != nil {
		t.Fatal(err)
	}
	sessionRef := agentadapter.AgentSessionRef{TenantID: a.TenantID, HarnessKind: "fake", SessionID: "rls-session-" + suffix}
	handle, err = service.ActivateRuntimeWorker(ctx, deviceActor, app.RuntimeWorkerActivateInput{DaemonInstanceID: daemonInstanceID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: sessionRef})
	if err != nil {
		t.Fatal(err)
	}
	harnessEvent := agentadapter.AgentEvent{Type: "turn.started", Session: sessionRef, Data: json.RawMessage(`{"phase":"started"}`), OccurredAt: time.Now().UTC()}
	if err := service.RecordRuntimeWorkerEvent(ctx, deviceActor, app.RuntimeWorkerEventInput{DaemonInstanceID: daemonInstanceID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Event: harnessEvent}); err != nil {
		t.Fatalf("PostgreSQL fenced Harness event failed: %v", err)
	}
	if err := service.RecordRuntimeWorkerEvent(ctx, deviceActor, app.RuntimeWorkerEventInput{DaemonInstanceID: daemonInstanceID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Event: harnessEvent}); err != nil {
		t.Fatalf("PostgreSQL fenced Harness event replay failed: %v", err)
	}
	runtimeEvents, err := service.Runtime().Events(ctx, a.TenantID, started.Job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	attemptEventCount := 0
	for _, event := range runtimeEvents {
		if event.Type == "attempt.event" {
			attemptEventCount++
		}
	}
	if attemptEventCount != 1 {
		t.Fatalf("PostgreSQL fenced Harness event replay created duplicates: %d", attemptEventCount)
	}
	if err := service.RecordRuntimeWorkerEvent(ctx, deviceActor, app.RuntimeWorkerEventInput{DaemonInstanceID: daemonInstanceID, AttemptID: handle.Attempt.ID, FenceToken: "stale", Event: agentadapter.AgentEvent{Type: "turn.started", Session: sessionRef, OccurredAt: time.Now().UTC()}}); err == nil {
		t.Fatal("PostgreSQL accepted a stale Harness event fence")
	}
	maintenanceNow := time.Now().UTC()
	rebuildID := domain.NewID()
	if err := store.CreateRuntimeProjectionRebuild(ctx, domain.RuntimeProjectionRebuildRun{ID: rebuildID, TenantID: a.TenantID, JobRunID: started.Job.ID, Mode: "dry_run", Status: "running", IntegrityStatus: "pending", StartedAt: maintenanceNow, Version: 1}); err != nil {
		t.Fatal("PostgreSQL could not create projection rebuild fact: ", err)
	}
	if err := store.SaveRuntimeMaintenanceHeartbeat(ctx, domain.RuntimeMaintenanceHeartbeat{TenantID: a.TenantID, Kind: domain.RuntimeMaintenanceReaper, WorkerID: "reaper-a", State: "running", LastStartedAt: maintenanceNow, Version: 1, UpdatedAt: maintenanceNow}); err != nil {
		t.Fatal("PostgreSQL could not create maintenance heartbeat: ", err)
	}
	runtimeSchema, err := service.Runtime().CreateRuntimeSchema(ctx, contentruntime.RuntimeSchemaInput{TenantID: a.TenantID, SchemaID: "contentcloud.rls-state", Revision: 1, Definition: map[string]any{"type": "object"}, RetentionPolicy: "30d", CreatedBy: a.UserID})
	if err != nil {
		t.Fatal("PostgreSQL could not create Runtime Schema: ", err)
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var attemptCanDelete, eventCanUpdate, eventCanDelete bool
	if err := conn.QueryRow(ctx, `SELECT has_table_privilege('contentcloud_runtime','runtime_attempts','DELETE'), has_table_privilege('contentcloud_runtime','runtime_job_events','UPDATE'), has_table_privilege('contentcloud_runtime','runtime_job_events','DELETE')`).Scan(&attemptCanDelete, &eventCanUpdate, &eventCanDelete); err != nil {
		t.Fatal(err)
	}
	if attemptCanDelete || eventCanUpdate || eventCanDelete {
		t.Fatalf("runtime role can mutate append-only Runtime facts: attempt_delete=%t event_update=%t event_delete=%t", attemptCanDelete, eventCanUpdate, eventCanDelete)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id',$1,true)`, b.TenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE contentcloud_runtime`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM brand_projects WHERE id=$1`, project.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A project through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM runtime_attempts WHERE id=$1`, handle.Attempt.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A RuntimeAttempt through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM bootstrap_diagnostics WHERE id=$1`, firstDiagnostic.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A bootstrap diagnostic through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM runtime_job_runs WHERE id=$1`, started.Job.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A JobRun through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM runtime_projection_rebuild_runs WHERE id=$1`, rebuildID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A projection rebuild through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM runtime_maintenance_heartbeats WHERE tenant_id=$1 AND kind=$2`, a.TenantID, domain.RuntimeMaintenanceReaper).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A maintenance heartbeat through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM runtime_schemas WHERE schema_id=$1`, runtimeSchema.SchemaID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A Runtime Schema through RLS: count=%d", count)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runtime_projection_rebuild_runs(tenant_id,id,job_run_id,mode,status,event_count,last_sequence,external_calls,integrity_status,error_code,started_at,version) VALUES($1,$2,$3,'dry_run','running',0,0,0,'cross-tenant','',$4,1)`, a.TenantID, domain.NewID(), started.Job.ID, time.Now().UTC()); err == nil {
		t.Fatal("tenant B inserted a tenant A projection rebuild through RLS")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runtime_maintenance_heartbeats(tenant_id,kind,worker_id,state,last_started_at,version,updated_at) VALUES($1,'runtime_delivery','cross-tenant','running',$2,1,$2)`, a.TenantID, time.Now().UTC()); err == nil {
		t.Fatal("tenant B inserted a tenant A maintenance heartbeat through RLS")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runtime_schemas(tenant_id,schema_id,revision,status,compatibility,definition,digest,retention_policy,created_by,created_at,version) VALUES($1,'contentcloud.cross-tenant',1,'draft','backward','{}',$2,'job','cross-tenant',$3,1)`, a.TenantID, "sha256:"+strings.Repeat("d", 64), time.Now().UTC()); err == nil {
		t.Fatal("tenant B inserted a tenant A Runtime Schema through RLS")
	}
}
