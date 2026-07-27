package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
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
	deviceActor, device, err := service.DeviceActor(ctx, connected.DeviceToken)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	snapshot := domain.ContextSnapshot{ID: domain.NewID(), TenantID: a.TenantID, ProjectID: project.ID, BuilderVersion: "test", SchemaVersion: "1.0", InputVersions: map[string]string{}, ManifestHash: "rls-test", CreatedAt: now}
	if err := store.CreateSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	run := domain.TaskRun{ID: domain.NewID(), TenantID: a.TenantID, ProjectID: project.ID, InputSnapshotID: snapshot.ID, IdempotencyKey: "rls-run-" + suffix, TaskType: "knowledge_extract", CapabilityID: domain.KnowledgeExtractCapability, CapabilityVersion: "1.0.0", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, OutputCount: 1, DeliveryProfiles: []string{"text"}, State: "queued", CreatedAt: now, UpdatedAt: now}
	bundle := environment.CreativeExecutionBundle{SchemaVersion: environment.ExecutionBundleSchemaVersion, BundleID: "ceb_" + domain.NewID(), ProjectID: project.ID, Subject: environment.ExecutionSubject{Type: "context_snapshot", ID: snapshot.ID, Digest: snapshot.ManifestHash}, IssuedAt: now, ExpiresAt: now.Add(time.Hour), Digest: "sha256:rls-bundle"}
	if err := store.CreateRunWithBundle(ctx, run, bundle); err != nil {
		t.Fatal(err)
	}
	capability := domain.Capability{ID: domain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, Digest: "sha256:rls-test", LocalOnly: true}
	lease, err := service.Poll(ctx, deviceActor, device, []domain.Capability{capability})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var runtimeCanUpdate, runtimeCanDelete bool
	if err := conn.QueryRow(ctx, `SELECT has_table_privilege('contentcloud_runtime','creative_execution_bundles','UPDATE'), has_table_privilege('contentcloud_runtime','creative_execution_bundles','DELETE')`).Scan(&runtimeCanUpdate, &runtimeCanDelete); err != nil {
		t.Fatal(err)
	}
	if runtimeCanUpdate || runtimeCanDelete {
		t.Fatalf("runtime role can mutate CreativeExecutionBundle: update=%t delete=%t", runtimeCanUpdate, runtimeCanDelete)
	}
	if _, err := conn.Exec(ctx, `UPDATE creative_execution_bundles SET digest='tampered' WHERE run_id=$1`, run.ID); err == nil {
		t.Fatal("CreativeExecutionBundle update unexpectedly bypassed the immutability trigger")
	}
	if _, err := conn.Exec(ctx, `DELETE FROM creative_execution_bundles WHERE run_id=$1`, run.ID); err == nil {
		t.Fatal("CreativeExecutionBundle delete unexpectedly bypassed the immutability trigger")
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
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM run_attempts WHERE id=$1`, lease.Attempt.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A run attempt through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM bootstrap_diagnostics WHERE id=$1`, firstDiagnostic.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A bootstrap diagnostic through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM creative_execution_bundles WHERE run_id=$1`, run.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A CreativeExecutionBundle through RLS: count=%d", count)
	}
}
