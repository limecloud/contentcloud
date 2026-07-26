package postgres_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	storepg "github.com/limecloud/contentcloud/internal/store/postgres"
)

func TestPerformanceImportTransactionWithPostgres(t *testing.T) {
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
	session, err := service.Register(ctx, fmt.Sprintf("results-%s@example.com", suffix), "long-enough-password", "Results Strategist", "Results Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Results Brand", ProductName: "Results Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	snapshot := domain.ContextSnapshot{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, BuilderVersion: "test", SchemaVersion: "1.0", InputVersions: map[string]string{}, ManifestHash: "results-" + suffix, CreatedAt: now}
	if err := store.CreateSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	run := domain.TaskRun{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, InputSnapshotID: snapshot.ID, IdempotencyKey: "results-run-" + suffix, TaskType: "script_generate", CapabilityID: domain.ScriptCapability, CapabilityVersion: "1.0.0", InputSchema: domain.TaskContractSchema, OutputSchema: domain.ScriptPackageSchema, OutputCount: 1, DeliveryProfiles: []string{"canonical_json"}, State: "queued", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	logical := domain.Script{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, Title: "Results script", CreatedAt: now}
	script, err := store.CreateScript(ctx, logical, domain.ScriptVersion{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, RunID: run.ID, ChangeType: "initial", InvariantFields: []string{}, ChangedFields: []string{}, Status: "approved", InputSnapshotID: snapshot.ID, ContentHash: "results-script-" + suffix, Package: domain.ScriptPackage{SchemaVersion: "1.1", Title: "Results script"}, Validation: domain.ValidationReport{Valid: true}, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.ImportPerformanceObservations(ctx, actor, app.ImportPerformanceInput{ProjectID: project.ID, SourceName: "results.csv", SourceFormat: "csv", Observations: []app.CreateObservationInput{{RowNumber: 2, ScriptVersionID: script.ID, Platform: "douyin", AccountAlias: "brand-main", PublishedAt: now.Add(-24 * time.Hour), WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"impressions": 1000}, Currency: "CNY", Spend: 100, GMV: 250, IssueCategory: "creative"}}}, "req-results")
	if err != nil {
		t.Fatal(err)
	}
	if result.Observations[0].ROI == nil || *result.Observations[0].ROI != 2.5 {
		t.Fatalf("server ROI was not persisted: %#v", result)
	}
	details, err := service.PerformanceImportDetails(ctx, actor, result.Batch.ID)
	if err != nil || len(details.Observations) != 1 || details.Observations[0].DedupKey == "" {
		t.Fatalf("import lineage did not round-trip: %#v, %v", details, err)
	}

	duplicateBatch := domain.PerformanceImportBatch{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, SourceName: "duplicate.csv", SourceFormat: "csv", SourceSHA256: domain.TokenHash("duplicate-" + suffix), RowCount: 2, ImportedCount: 2, Status: "imported", CreatedBy: actor.UserID, CreatedAt: now}
	first := details.Observations[0]
	first.ID, first.ImportBatchID, first.RowNumber = domain.NewID(), duplicateBatch.ID, 2
	second := first
	second.ID, second.RowNumber = domain.NewID(), 3
	if err := store.CreatePerformanceImportBatch(ctx, duplicateBatch, []domain.PerformanceObservation{first, second}); err == nil {
		t.Fatal("duplicate observations should fail the transaction")
	}
	if _, err := store.PerformanceImportBatch(ctx, actor.TenantID, duplicateBatch.ID); err == nil {
		t.Fatal("failed observation insert left a partial batch")
	}
	observations, err := store.PerformanceObservations(ctx, actor.TenantID, project.ID)
	if err != nil || len(observations) != 1 {
		t.Fatalf("failed transaction changed observations: %#v, %v", observations, err)
	}

	rating, err := service.CreateRatingDecision(ctx, actor, app.CreateRatingDecisionInput{ProjectID: project.ID, SubjectType: "script_version", SubjectID: script.ID, ObservationIDs: []string{result.Observations[0].ID}, Rating: "seed_candidate", Reason: "人工判断该结果可进入裂变", NextAction: "建立单变量变体"}, "req-rating")
	if err != nil || rating.Decision.ID == "" {
		t.Fatalf("rating decision failed: %#v, %v", rating, err)
	}
}
