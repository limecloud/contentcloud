package app_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestPerformanceImportIsAtomicAndRejectsMixedCurrency(t *testing.T) {
	ctx, service, store, actor, project, script := performanceFixture(t)
	input := validPerformanceImport(project.ID, script.ID)
	second := input.Observations[0]
	second.RowNumber = 3
	second.AccountAlias = "second-account"
	second.Currency = "USD"
	input.Observations = append(input.Observations, second)

	_, err := service.ImportPerformanceObservations(ctx, actor, input, "req-mixed-currency")
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "RESULT_IMPORT_REJECTED" {
		t.Fatalf("expected structured batch rejection, got %v", err)
	}
	details, ok := domainErr.Details.(map[string]any)
	if !ok || len(details["row_errors"].([]domain.PerformanceImportRowError)) == 0 {
		t.Fatalf("expected row errors, got %#v", domainErr.Details)
	}
	observations, err := store.PerformanceObservations(ctx, actor.TenantID, project.ID)
	if err != nil || len(observations) != 0 {
		t.Fatalf("rejected batch wrote observations: %#v, %v", observations, err)
	}
	batches, err := store.PerformanceImportBatches(ctx, actor.TenantID, project.ID)
	if err != nil || len(batches) != 0 {
		t.Fatalf("rejected batch was persisted: %#v, %v", batches, err)
	}
}

func TestPerformanceImportComputesROIAndRejectsDuplicate(t *testing.T) {
	ctx, service, store, actor, project, script := performanceFixture(t)
	input := validPerformanceImport(project.ID, script.ID)

	result, err := service.ImportPerformanceObservations(ctx, actor, input, "req-import")
	if err != nil {
		t.Fatal(err)
	}
	if result.Batch.ImportedCount != 1 || result.Observations[0].ROI == nil || *result.Observations[0].ROI != 3 {
		t.Fatalf("unexpected imported result: %#v", result)
	}
	if result.Observations[0].ImportBatchID != result.Batch.ID || result.Batch.Currency != "CNY" {
		t.Fatalf("batch lineage or currency missing: %#v", result)
	}

	_, err = service.ImportPerformanceObservations(ctx, actor, input, "req-duplicate")
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "RESULT_IMPORT_REJECTED" {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
	observations, _ := store.PerformanceObservations(ctx, actor.TenantID, project.ID)
	batches, _ := store.PerformanceImportBatches(ctx, actor.TenantID, project.ID)
	if len(observations) != 1 || len(batches) != 1 {
		t.Fatalf("duplicate changed immutable history: observations=%d batches=%d", len(observations), len(batches))
	}
}

func TestRatingDecisionIsManualAndDoesNotMutateSubject(t *testing.T) {
	ctx, service, store, actor, project, script := performanceFixture(t)
	imported, err := service.ImportPerformanceObservations(ctx, actor, validPerformanceImport(project.ID, script.ID), "req-import")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.CreateRatingDecision(ctx, actor, app.CreateRatingDecisionInput{
		ProjectID:      project.ID,
		SubjectType:    "script_version",
		SubjectID:      script.ID,
		ObservationIDs: []string{imported.Observations[0].ID},
		Rating:         "seed_candidate",
		Reason:         "完播与成交指标达到本轮人工判断阈值",
		NextAction:     "创建单变量钩子变体",
	}, "req-rating")
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Rating != "seed_candidate" || result.Decision.CreatedBy != actor.UserID {
		t.Fatalf("unexpected decision: %#v", result.Decision)
	}
	unchanged, err := store.Script(ctx, actor.TenantID, script.ID)
	if err != nil || unchanged.Status != "approved" {
		t.Fatalf("rating decision mutated the script: %#v, %v", unchanged, err)
	}
	decisions, _ := store.RatingDecisions(ctx, actor.TenantID, project.ID)
	if len(decisions) != 1 || decisions[0].ID != result.Decision.ID {
		t.Fatalf("rating decision not persisted: %#v", decisions)
	}
}

func TestPerformanceImportDryRunDoesNotPersist(t *testing.T) {
	ctx, service, store, actor, project, script := performanceFixture(t)
	input := validPerformanceImport(project.ID, script.ID)
	input.DryRun = true
	result, err := service.ImportPerformanceObservations(ctx, actor, input, "req-dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.Batch.Status != "validated" || result.Batch.ID != "" || result.Observations[0].ID != "" {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
	observations, _ := store.PerformanceObservations(ctx, actor.TenantID, project.ID)
	if len(observations) != 0 {
		t.Fatalf("dry-run persisted observations: %#v", observations)
	}
}

func performanceFixture(t *testing.T) (context.Context, *app.Service, *memory.Store, app.Actor, domain.Project, domain.ScriptVersion) {
	t.Helper()
	ctx := context.Background()
	store := memory.New()
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	project := domain.Project{ID: domain.NewID(), TenantID: domain.NewID(), BrandName: "Brand", ProductName: "Product", Status: "active", RowVersion: 1, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	logical := domain.Script{ID: domain.NewID(), TenantID: project.TenantID, ProjectID: project.ID, Title: "Script", CreatedAt: now}
	script, err := store.CreateScript(ctx, logical, domain.ScriptVersion{ID: domain.NewID(), TenantID: project.TenantID, ProjectID: project.ID, Status: "approved", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	actor := app.Actor{UserID: domain.NewID(), TenantID: project.TenantID, Role: "strategist", Type: "user"}
	return ctx, app.New(store, slog.Default()), store, actor, project, script
}

func validPerformanceImport(projectID, scriptID string) app.ImportPerformanceInput {
	return app.ImportPerformanceInput{
		ProjectID:    projectID,
		SourceName:   "results.csv",
		SourceFormat: "csv",
		Observations: []app.CreateObservationInput{{
			RowNumber:       2,
			ProjectID:       projectID,
			ScriptVersionID: scriptID,
			Platform:        "douyin",
			AccountAlias:    "brand-main",
			PublishedAt:     time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
			WindowHours:     24,
			SampleStatus:    "seed_candidate",
			Metrics:         map[string]float64{"impressions": 12000, "completion_rate": 0.42},
			Currency:        "CNY",
			Spend:           100,
			GMV:             300,
			IssueCategory:   "creative",
			Notes:           "人工复盘",
		}},
	}
}
