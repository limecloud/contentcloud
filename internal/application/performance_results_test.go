package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/persistence/memory"

	"github.com/limecloud/contentcloud/internal/application"
	performancedomain "github.com/limecloud/contentcloud/internal/performance"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestPerformanceImportIsAtomicAndRejectsMixedCurrency(t *testing.T) {
	ctx, service, store, actor, project, snapshot := performanceFixture(t)
	input := validPerformanceImport(project.ID, snapshot.ID)
	second := input.Observations[0]
	second.RowNumber = 3
	second.AccountAlias = "second-account"
	second.Currency = "USD"
	input.Observations = append(input.Observations, second)

	_, err := service.Performance.ImportPerformanceObservations(ctx, actor, input, "req-mixed-currency")
	var domainErr *fault.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "RESULT_IMPORT_REJECTED" {
		t.Fatalf("expected structured batch rejection, got %v", err)
	}
	details, ok := domainErr.Details.(map[string]any)
	if !ok || len(details["row_errors"].([]performancedomain.PerformanceImportRowError)) == 0 {
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
	ctx, service, store, actor, project, snapshot := performanceFixture(t)
	input := validPerformanceImport(project.ID, snapshot.ID)

	result, err := service.Performance.ImportPerformanceObservations(ctx, actor, input, "req-import")
	if err != nil {
		t.Fatal(err)
	}
	if result.Batch.ImportedCount != 1 || result.Observations[0].ROI == nil || *result.Observations[0].ROI != 3 {
		t.Fatalf("unexpected imported result: %#v", result)
	}
	if result.Observations[0].ImportBatchID != result.Batch.ID || result.Batch.Currency != "CNY" {
		t.Fatalf("batch lineage or currency missing: %#v", result)
	}

	_, err = service.Performance.ImportPerformanceObservations(ctx, actor, input, "req-duplicate")
	var domainErr *fault.Error
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
	ctx, service, store, actor, project, snapshot := performanceFixture(t)
	imported, err := service.Performance.ImportPerformanceObservations(ctx, actor, validPerformanceImport(project.ID, snapshot.ID), "req-import")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Performance.CreateRatingDecision(ctx, actor, application.CreateRatingDecisionInput{
		ProjectID:      project.ID,
		SubjectType:    "approved_snapshot",
		SubjectID:      snapshot.ID,
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
	unchanged, err := store.ApprovedSnapshot(ctx, actor.TenantID, snapshot.ID)
	if err != nil || unchanged.ContentHash != snapshot.ContentHash {
		t.Fatalf("rating decision mutated the approved snapshot: %#v, %v", unchanged, err)
	}
	decisions, _ := store.RatingDecisions(ctx, actor.TenantID, project.ID)
	if len(decisions) != 1 || decisions[0].ID != result.Decision.ID {
		t.Fatalf("rating decision not persisted: %#v", decisions)
	}
}

func TestPerformanceImportDryRunDoesNotPersist(t *testing.T) {
	ctx, service, store, actor, project, snapshot := performanceFixture(t)
	input := validPerformanceImport(project.ID, snapshot.ID)
	input.DryRun = true
	result, err := service.Performance.ImportPerformanceObservations(ctx, actor, input, "req-dry-run")
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

func performanceFixture(t *testing.T) (context.Context, *application.Application, *memory.Store, application.Actor, workspacedomain.Project, reviewdomain.ApprovedSnapshot) {
	t.Helper()
	ctx, service, store, actor, binding := v3ContentFixture(t)
	revision := publishV3ContentItem(t, ctx, service, binding, "content-item-performance", "publish-performance")
	if _, err := service.Review.ApproveSubmission(ctx, actor, revision.ID, "internal review passed", "internal-approve"); err != nil {
		t.Fatal(err)
	}
	grant, err := service.Review.CreateReviewGrant(ctx, actor, revision.ID, "client@example.com", "create-client-review")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review.VerifyReviewGrant(ctx, grant.PlaintextToken, grant.PlaintextOTP); err != nil {
		t.Fatal(err)
	}
	decision, err := service.Review.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "client approved", "", "client-approve")
	if err != nil || decision.ApprovedSnapshot == nil {
		t.Fatalf("approved snapshot was not created: %#v, %v", decision, err)
	}
	project, err := service.Workspace.Project(ctx, actor, binding.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, service, store, actor, project, *decision.ApprovedSnapshot
}

func validPerformanceImport(projectID, approvedSnapshotID string) application.ImportPerformanceInput {
	return application.ImportPerformanceInput{
		ProjectID:    projectID,
		SourceName:   "results.csv",
		SourceFormat: "csv",
		Observations: []application.CreateObservationInput{{
			RowNumber:          2,
			ProjectID:          projectID,
			ApprovedSnapshotID: approvedSnapshotID,
			Platform:           "douyin",
			AccountAlias:       "brand-main",
			PublishedAt:        time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
			WindowHours:        24,
			SampleStatus:       "seed_candidate",
			Metrics:            map[string]float64{"impressions": 12000, "completion_rate": 0.42},
			Currency:           "CNY",
			Spend:              100,
			GMV:                300,
			IssueCategory:      "creative",
			Notes:              "人工复盘",
		}},
	}
}
