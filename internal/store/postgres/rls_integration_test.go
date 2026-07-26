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
	storepg "github.com/limecloud/contentcloud/internal/store/postgres"
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
	connected, err := service.ConnectDevice(ctx, app.ConnectDeviceInput{ConnectKey: connect.PlaintextConnectKey, Hostname: "rls-local", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
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
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	capability := domain.Capability{ID: domain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, Digest: "sha256:rls-test", LocalOnly: true}
	lease, err := service.Poll(ctx, deviceActor, device, []domain.Capability{capability})
	if err != nil {
		t.Fatal(err)
	}
	logical := domain.Script{ID: domain.NewID(), TenantID: a.TenantID, ProjectID: project.ID, Title: "RLS script", CreatedAt: now}
	version := domain.ScriptVersion{ID: domain.NewID(), TenantID: a.TenantID, ProjectID: project.ID, RunID: run.ID, ChangeType: "initial", InvariantFields: []string{}, ChangedFields: []string{}, Status: "review_ready", InputSnapshotID: snapshot.ID, ContentHash: "rls-script-" + suffix, Package: domain.ScriptPackage{SchemaVersion: "1.1", Title: "RLS script"}, Validation: domain.ValidationReport{Valid: true}, CreatedAt: now}
	version, err = store.CreateScript(ctx, logical, version)
	if err != nil {
		t.Fatal(err)
	}
	reviewCycle := domain.ReviewCycle{ID: domain.NewID(), TenantID: a.TenantID, ProjectID: project.ID, SubjectType: "script_version", SubjectID: version.ID, Status: "open", OpenedBy: a.UserID, OpenedAt: now, CreatedAt: now}
	reviewCycle, err = store.CreateReviewCycle(ctx, reviewCycle)
	if err != nil {
		t.Fatal(err)
	}
	artifact := domain.Artifact{ID: domain.NewID(), TenantID: a.TenantID, ProjectID: project.ID, ScriptVersionID: version.ID, Kind: "extension", CapabilityID: domain.ArtifactExportCapability, CapabilityVersion: "1.0.0", CapabilityDigest: "rls-artifact", SchemaID: "opaque/1.0", MediaType: "application/octet-stream", FileName: "private.bin", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ByteSize: 10, Visibility: "internal", RetentionClass: "project", Purpose: "primary", SourceDeviceID: device.ID, ValidationStatus: "valid", PresentationTier: "metadata_only", Metadata: map[string]any{}, CreatedAt: now}
	if err := store.CreateArtifact(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	openRequest := domain.ArtifactOpenRequest{ID: domain.NewID(), TenantID: a.TenantID, ProjectID: project.ID, ArtifactID: artifact.ID, DeviceID: device.ID, RequestedBy: a.UserID, State: "pending", ExpiresAt: now.Add(time.Minute), CreatedAt: now}
	if err := store.CreateArtifactOpenRequest(ctx, openRequest); err != nil {
		t.Fatal(err)
	}
	performance, err := service.ImportPerformanceObservations(ctx, a, app.ImportPerformanceInput{ProjectID: project.ID, SourceName: "rls-results.csv", SourceFormat: "csv", Observations: []app.CreateObservationInput{{RowNumber: 2, ScriptVersionID: version.ID, Platform: "douyin", AccountAlias: "rls-account", PublishedAt: now.Add(-time.Hour), WindowHours: 24, SampleStatus: "insufficient_sample", Metrics: map[string]float64{"impressions": 10}}}}, "req-rls-results")
	if err != nil {
		t.Fatal(err)
	}
	rating, err := service.CreateRatingDecision(ctx, a, app.CreateRatingDecisionInput{ProjectID: project.ID, SubjectType: "script_version", SubjectID: version.ID, ObservationIDs: []string{performance.Observations[0].ID}, Rating: "insufficient_sample", Reason: "样本不足", NextAction: "继续收集观察"}, "req-rls-rating")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
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
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM performance_import_batches WHERE id=$1`, performance.Batch.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A performance batch through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM performance_observations WHERE id=$1`, performance.Observations[0].ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A performance observation through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM rating_decisions WHERE id=$1`, rating.Decision.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A rating decision through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM scripts WHERE id=$1`, logical.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A logical script through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM review_cycles WHERE id=$1`, reviewCycle.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A review cycle through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM artifacts WHERE id=$1`, artifact.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A artifact through RLS: count=%d", count)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM artifact_open_requests WHERE id=$1`, openRequest.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tenant B saw tenant A artifact open request through RLS: count=%d", count)
	}
}
