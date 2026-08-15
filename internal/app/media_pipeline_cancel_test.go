package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/mediapipeline"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

type cancelTrackingProvider struct {
	cancelErr error
	calls     int
	status    string
}

func (p *cancelTrackingProvider) Validate(mediapipeline.Request, domain.ProviderProfile) error {
	return nil
}

func (p *cancelTrackingProvider) Estimate(mediapipeline.Request, domain.ProviderProfile) (mediapipeline.Estimate, error) {
	return mediapipeline.Estimate{CostMinor: 1, Currency: "CNY"}, nil
}

func (p *cancelTrackingProvider) Submit(context.Context, mediapipeline.Request, domain.ProviderProfile) (mediapipeline.Submission, error) {
	return mediapipeline.Submission{ExternalJobID: "external-1"}, nil
}

func (p *cancelTrackingProvider) Status(context.Context, string, domain.ProviderProfile) (mediapipeline.Status, error) {
	state := p.status
	if state == "" {
		state = "running"
	}
	return mediapipeline.Status{State: state}, nil
}

func (p *cancelTrackingProvider) Cancel(context.Context, string, domain.ProviderProfile) error {
	p.calls++
	return p.cancelErr
}

func (p *cancelTrackingProvider) Download(context.Context, string, domain.ProviderProfile) (mediapipeline.Download, error) {
	return mediapipeline.Download{}, errors.New("not used")
}

func TestCancelMediaGenerationJobCallsProviderBeforeLocalTerminalState(t *testing.T) {
	tests := []struct {
		name        string
		cancelErr   error
		expectState string
		expectCode  string
	}{
		{name: "success", expectState: domain.MediaJobCancelled},
		{name: "unknown", cancelErr: errors.New("timeout"), expectState: domain.MediaJobAwaitingExternal, expectCode: "PROVIDER_CANCEL_UNKNOWN"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			store := memory.New()
			now := time.Now().UTC()
			profile := domain.ProviderProfile{ProviderID: "seedance", Version: "1.0.0", Digest: "sha256:" + "a" + strings.Repeat("0", 63), AdapterVersion: "test/1", Model: "model", Region: "global", Modes: []string{"image_to_video"}, InputMediaTypes: []string{"image/png"}, OutputMediaType: "video/mp4", DataRetention: "ephemeral", Pricing: map[string]any{"currency": "CNY", "per_job_minor": 1}, Status: "published", VerifiedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
			if err := store.CreateProviderProfile(ctx, profile); err != nil {
				t.Fatal(err)
			}
			provider := &cancelTrackingProvider{cancelErr: test.cancelErr}
			service := app.New(store, nil, app.WithMediaProviderAdapter("seedance", provider))
			job := domain.MediaGenerationJob{ID: "job-1", TenantID: "tenant-1", ProjectID: "project-1", TaskID: "task-1", StageRunID: "stage-1", StoryboardSnapshotID: "snapshot-1", PromptPackageArtifactID: "prompt-1", ProviderID: "seedance", ProfileVersion: profile.Version, ProfileDigest: profile.Digest, Model: profile.Model, Mode: "image_to_video", AspectRatio: "9:16", DurationSeconds: 5, State: domain.MediaJobGenerating, IdempotencyKey: "job-key", Currency: "CNY", AttemptCount: 1, MaxAttempts: 3, RowVersion: 1, CreatedBy: "user-1", CreatedAt: now, UpdatedAt: now}
			if err := store.CreateMediaGenerationJob(ctx, job); err != nil {
				t.Fatal(err)
			}
			attempt := domain.ProviderAttempt{ID: "attempt-1", TenantID: job.TenantID, ProjectID: job.ProjectID, GenerationJobID: job.ID, AttemptNumber: 1, ProviderID: job.ProviderID, RequestDigest: "sha256:" + strings.Repeat("b", 64), ExternalJobID: "external-1", ProviderState: "submitted", SafeRequestSummary: map[string]any{}, SafeResponseSummary: map[string]any{}, DisclosureManifest: map[string]any{}, Currency: "CNY", CreatedAt: now, UpdatedAt: now}
			if err := store.CreateProviderAttempt(ctx, attempt); err != nil {
				t.Fatal(err)
			}
			actor := app.Actor{UserID: "user-1", TenantID: job.TenantID, Role: "tenant_admin"}
			result, err := service.CancelMediaGenerationJob(ctx, actor, job.ID, app.MediaJobDecisionInput{ExpectedVersion: 1}, "cancel-1")
			if test.cancelErr == nil && err != nil {
				t.Fatal(err)
			}
			if test.cancelErr != nil && !containsCode(err, test.expectCode) {
				t.Fatalf("error = %v", err)
			}
			if provider.calls != 1 || result.State != test.expectState {
				t.Fatalf("calls=%d result=%#v", provider.calls, result)
			}
			stored, err := store.MediaGenerationJob(ctx, job.TenantID, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.State != test.expectState || stored.ErrorCode != test.expectCode {
				t.Fatalf("stored=%#v", stored)
			}
			if test.cancelErr != nil {
				provider.status = "cancelled"
				if err := service.ProcessMediaGenerationJob(ctx, job.TenantID, job.ID); err != nil {
					t.Fatal(err)
				}
				stored, err = store.MediaGenerationJob(ctx, job.TenantID, job.ID)
				if err != nil || stored.State != domain.MediaJobCancelled || stored.ErrorCode != "" {
					t.Fatalf("reconciled=%#v err=%v", stored, err)
				}
			}
		})
	}
}

func TestReconcileMediaGenerationSubmitBindsExternalIDWithoutResubmitting(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	now := time.Now().UTC()
	profile := domain.ProviderProfile{ProviderID: "seedance", Version: "1.0.0", Digest: "sha256:" + "a" + strings.Repeat("0", 63), AdapterVersion: "test/1", Model: "model", Region: "global", Modes: []string{"image_to_video"}, InputMediaTypes: []string{"image/png"}, OutputMediaType: "video/mp4", DataRetention: "ephemeral", Pricing: map[string]any{"currency": "CNY", "per_job_minor": 1}, Status: "published", VerifiedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	if err := store.CreateProviderProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	provider := &cancelTrackingProvider{status: "running"}
	service := app.New(store, nil, app.WithMediaProviderAdapter("seedance", provider))
	job := domain.MediaGenerationJob{ID: "job-reconcile", TenantID: "tenant-1", ProjectID: "project-1", TaskID: "task-1", StageRunID: "stage-1", StoryboardSnapshotID: "snapshot-1", PromptPackageArtifactID: "prompt-1", ProviderID: "seedance", ProfileVersion: profile.Version, ProfileDigest: profile.Digest, Model: profile.Model, Mode: "image_to_video", AspectRatio: "9:16", DurationSeconds: 5, State: domain.MediaJobAwaitingExternal, IdempotencyKey: "job-reconcile-key", Currency: "CNY", AttemptCount: 1, MaxAttempts: 3, RowVersion: 1, CreatedBy: "user-1", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateMediaGenerationJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	attempt := domain.ProviderAttempt{ID: "attempt-reconcile", TenantID: job.TenantID, ProjectID: job.ProjectID, GenerationJobID: job.ID, AttemptNumber: 1, ProviderID: job.ProviderID, RequestDigest: "sha256:" + strings.Repeat("b", 64), ProviderState: "unknown", SafeRequestSummary: map[string]any{}, SafeResponseSummary: map[string]any{}, DisclosureManifest: map[string]any{}, Currency: "CNY", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateProviderAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	result, err := service.ReconcileMediaGenerationSubmit(ctx, app.Actor{UserID: "user-1", TenantID: job.TenantID, Role: "tenant_admin"}, job.ID, app.MediaJobSubmitReconciliationInput{ExpectedVersion: 1, ExternalJobID: "external-reconciled"}, "reconcile-1")
	if err != nil || result.RowVersion != 2 {
		t.Fatalf("reconciliation result=%#v err=%v", result, err)
	}
	attempts, err := store.ProviderAttempts(ctx, job.TenantID, job.ID)
	if err != nil || len(attempts) != 1 || attempts[0].ExternalJobID != "external-reconciled" || attempts[0].ProviderState != "reconciliation_pending" {
		t.Fatalf("reconciled attempt=%#v err=%v", attempts, err)
	}
	if provider.calls != 0 {
		t.Fatalf("reconciliation must not call provider cancel or submit, calls=%d", provider.calls)
	}
}

func containsCode(err error, code string) bool {
	var value *domain.Error
	return errors.As(err, &value) && value.Code == code
}
