package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

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
	sop := domain.SOPVersion{ID: "rls-runtime-sop-v1", TenantID: a.TenantID, SOPID: "rls-runtime-sop", Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: "RLS Runtime", Status: "published", DefaultExecutionMode: "agent", Stages: []domain.StageDefinition{{ID: "execute", Name: "执行", Order: 10, OutputSchema: "contentcloud.rls/1.0", ExecutionModes: []string{"agent"}}}}
	started, err := service.Runtime().Start(ctx, contentruntime.StartInput{TenantID: a.TenantID, ProjectID: project.ID, WorkTaskID: "rls-runtime-" + suffix, BusinessType: "rls.test", SOP: sop, BindingDigest: "sha256:" + strings.Repeat("a", 64), InputDigest: "sha256:" + strings.Repeat("b", 64), RuntimePolicyID: "runtime-policy/rls", ContractMajor: 1, CreatedBy: a.UserID, IdempotencyKey: "rls-runtime-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := service.PrepareRuntimeWorker(ctx, deviceActor, app.RuntimeWorkerPrepareInput{JobRunID: started.Job.ID, HarnessKind: "fake", Role: "worker", ExecutionProfileID: "rls-test", MaxTokens: 512})
	if err != nil {
		t.Fatal(err)
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
}
