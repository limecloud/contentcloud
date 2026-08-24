package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	mediapipeline "github.com/limecloud/contentcloud/internal/integration/provider/media"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/work"
)

func TestCreateMediaGenerationBatchRejectsEmptyInputWithoutWrites(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), nil)
	_, err := service.Delivery.CreateMediaGenerationBatch(t.Context(), application.Actor{Role: "tenant_admin"}, "missing-task", application.CreateMediaGenerationBatchInput{}, "")
	if err == nil || !hasFaultCode(err, "MEDIA_BATCH_SIZE_INVALID") {
		t.Fatalf("expected empty batch validation error, got %v", err)
	}
	jobs, err := store.MediaGenerationJobs(t.Context(), "", "missing-task")
	if err != nil || len(jobs) != 0 {
		t.Fatalf("empty batch created jobs: %#v err=%v", jobs, err)
	}
}

func TestCreateMediaGenerationBatchAdmissionMatrix(t *testing.T) {
	t.Run("confirmation required does not write", func(t *testing.T) {
		fixture := newMediaBatchFixture(t, batchQuoteAdapter{})
		input := fixture.jobInput("quote-confirmation", 5)
		result, err := fixture.service.Delivery.CreateMediaGenerationBatch(fixture.ctx, fixture.actor, fixture.taskID, application.CreateMediaGenerationBatchInput{Jobs: []application.CreateMediaGenerationJobInput{input}}, "")
		if err != nil || result.Decision != "confirmation_required" || result.EstimatedCostMinor != 10 || len(result.Jobs) != 0 {
			t.Fatalf("unexpected confirmation result: %#v err=%v", result, err)
		}
		assertNoMediaJobs(t, fixture.store, fixture.actor.TenantID, fixture.taskID)
	})

	t.Run("blocked batch does not partially write", func(t *testing.T) {
		fixture := newMediaBatchFixture(t, batchQuoteAdapter{})
		valid := fixture.jobInput("blocked-valid", 5)
		invalid := fixture.jobInput("blocked-invalid", 5)
		invalid.StoryboardSnapshotID = "missing-snapshot"
		result, err := fixture.service.Delivery.CreateMediaGenerationBatch(fixture.ctx, fixture.actor, fixture.taskID, application.CreateMediaGenerationBatchInput{Jobs: []application.CreateMediaGenerationJobInput{valid, invalid}}, "")
		if err != nil || result.Decision != "blocked" || len(result.Blockers) != 1 || result.Blockers[0].Index != 1 {
			t.Fatalf("unexpected blocked result: %#v err=%v", result, err)
		}
		assertNoMediaJobs(t, fixture.store, fixture.actor.TenantID, fixture.taskID)
	})

	t.Run("currency mismatch blocks the whole batch", func(t *testing.T) {
		fixture := newMediaBatchFixture(t, batchQuoteAdapter{})
		first := fixture.jobInput("currency-cny", 5)
		second := fixture.jobInput("currency-usd", 6)
		result, err := fixture.service.Delivery.CreateMediaGenerationBatch(fixture.ctx, fixture.actor, fixture.taskID, application.CreateMediaGenerationBatchInput{Jobs: []application.CreateMediaGenerationJobInput{first, second}, ConfirmCost: true}, "")
		if err != nil || result.Decision != "blocked" || len(result.Blockers) != 1 || result.Blockers[0].Code != "MEDIA_BATCH_CURRENCY_MISMATCH" {
			t.Fatalf("unexpected currency result: %#v err=%v", result, err)
		}
		assertNoMediaJobs(t, fixture.store, fixture.actor.TenantID, fixture.taskID)
	})

	t.Run("confirmed batch queues every item", func(t *testing.T) {
		fixture := newMediaBatchFixture(t, batchQuoteAdapter{})
		result, err := fixture.service.Delivery.CreateMediaGenerationBatch(fixture.ctx, fixture.actor, fixture.taskID, application.CreateMediaGenerationBatchInput{Jobs: []application.CreateMediaGenerationJobInput{fixture.jobInput("queued-one", 5), fixture.jobInput("queued-two", 5)}, ConfirmCost: true}, "")
		if err != nil || result.Decision != "admitted" || len(result.Jobs) != 2 || result.EstimatedCostMinor != 20 {
			t.Fatalf("unexpected admitted result: %#v err=%v", result, err)
		}
		for _, job := range result.Jobs {
			if job.State != deliverydomain.MediaJobQueued || job.ErrorCode != "" || job.ErrorDetailSafe != "" {
				t.Fatalf("confirmed job was not queued cleanly: %#v", job)
			}
		}
	})

	t.Run("idempotency conflict is atomic", func(t *testing.T) {
		fixture := newMediaBatchFixture(t, batchQuoteAdapter{})
		input := fixture.jobInput("same-key", 5)
		first, err := fixture.service.Delivery.CreateMediaGenerationBatch(fixture.ctx, fixture.actor, fixture.taskID, application.CreateMediaGenerationBatchInput{Jobs: []application.CreateMediaGenerationJobInput{input}, ConfirmCost: true}, "")
		if err != nil || first.Decision != "admitted" || len(first.Jobs) != 1 {
			t.Fatalf("first batch was not admitted: %#v err=%v", first, err)
		}
		_, err = fixture.service.Delivery.CreateMediaGenerationBatch(fixture.ctx, fixture.actor, fixture.taskID, application.CreateMediaGenerationBatchInput{Jobs: []application.CreateMediaGenerationJobInput{input}, ConfirmCost: true}, "")
		if err == nil || !hasFaultCode(err, "MEDIA_JOB_IDEMPOTENCY_CONFLICT") {
			t.Fatalf("expected idempotency conflict, got %v", err)
		}
		jobs, err := fixture.store.MediaGenerationJobs(fixture.ctx, fixture.actor.TenantID, fixture.taskID)
		if err != nil || len(jobs) != 1 || jobs[0].IdempotencyKey != "same-key" {
			t.Fatalf("idempotency retry partially wrote jobs: %#v err=%v", jobs, err)
		}
	})
}

type mediaBatchFixture struct {
	ctx        context.Context
	service    *application.Application
	store      *memory.Store
	actor      application.Actor
	taskID     string
	snapshotID string
}

func newMediaBatchFixture(t *testing.T, adapter mediapipeline.Adapter) mediaBatchFixture {
	t.Helper()
	ctx, service, store, actor, binding := v3ContentFixture(t)
	service = application.New(application.DependenciesFrom(store), nil, application.WithMediaProviderAdapter("fake", adapter))
	// Reuse the V3 review path so the media job is always tied to a real approved snapshot.
	revision := publishV3ContentItem(t, ctx, service, binding, "media-batch-content", "media-batch-content")
	if _, err := service.Review.ApproveSubmission(ctx, actor, revision.ID, "internal", "media-batch-internal"); err != nil {
		t.Fatal(err)
	}
	grant, err := service.Review.CreateReviewGrant(ctx, actor, revision.ID, "media-batch@example.com", "media-batch-grant")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review.VerifyReviewGrant(ctx, grant.PlaintextToken, grant.PlaintextOTP); err != nil {
		t.Fatal(err)
	}
	decision, err := service.Review.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "client", "", "media-batch-client")
	if err != nil || decision.ApprovedSnapshot == nil {
		t.Fatalf("approved snapshot fixture failed: %#v err=%v", decision, err)
	}
	now := time.Now().UTC()
	taskID := idgen.New()
	if err := store.CreateWorkTask(ctx, work.WorkTask{ID: taskID, TenantID: actor.TenantID, ProjectID: binding.ProjectID, SOPID: "marketing-video", SOPVersion: 1, Title: "媒体批量准入测试", ContentType: identitydomain.ContentTypeMarketingVideo, Priority: "normal", RiskProfile: "low", Status: work.TaskStatusRunning, CurrentStageID: "generation", CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateStageRun(ctx, work.StageRun{ID: idgen.New(), TenantID: actor.TenantID, TaskID: taskID, StageID: "generation", Status: work.StageRunStatusRunning, ExecutionMode: "manual", StartedAt: &now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return mediaBatchFixture{ctx: ctx, service: service, store: store, actor: actor, taskID: taskID, snapshotID: decision.ApprovedSnapshot.ID}
}

func (f mediaBatchFixture) jobInput(key string, duration int) application.CreateMediaGenerationJobInput {
	return application.CreateMediaGenerationJobInput{StoryboardSnapshotID: f.snapshotID, ProviderID: "fake", ProfileVersion: "1.0.0", Mode: "text_to_video", AspectRatio: "9:16", DurationSeconds: duration, IdempotencyKey: key}
}

func assertNoMediaJobs(t *testing.T, store *memory.Store, tenantID, taskID string) {
	t.Helper()
	jobs, err := store.MediaGenerationJobs(t.Context(), tenantID, taskID)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("batch wrote jobs despite not being admitted: %#v err=%v", jobs, err)
	}
}

func hasFaultCode(err error, code string) bool {
	var value *fault.Error
	return errors.As(err, &value) && value.Code == code
}

type batchQuoteAdapter struct{}

func (batchQuoteAdapter) Validate(mediapipeline.Request, deliverydomain.ProviderProfile) error {
	return nil
}

func (batchQuoteAdapter) Estimate(request mediapipeline.Request, _ deliverydomain.ProviderProfile) (mediapipeline.Estimate, error) {
	if request.DurationSeconds == 6 {
		return mediapipeline.Estimate{CostMinor: 10, Currency: "USD"}, nil
	}
	return mediapipeline.Estimate{CostMinor: 10, Currency: "CNY"}, nil
}

func (batchQuoteAdapter) Submit(context.Context, mediapipeline.Request, deliverydomain.ProviderProfile) (mediapipeline.Submission, error) {
	return mediapipeline.Submission{ExternalJobID: "batch-external"}, nil
}

func (batchQuoteAdapter) Status(context.Context, string, deliverydomain.ProviderProfile) (mediapipeline.Status, error) {
	return mediapipeline.Status{State: "succeeded"}, nil
}

func (batchQuoteAdapter) Cancel(context.Context, string, deliverydomain.ProviderProfile) error {
	return nil
}

func (batchQuoteAdapter) Download(context.Context, string, deliverydomain.ProviderProfile) (mediapipeline.Download, error) {
	return mediapipeline.Download{}, fault.Invalid("BATCH_TEST_DOWNLOAD_UNEXPECTED", "批量准入测试不应下载媒体")
}
