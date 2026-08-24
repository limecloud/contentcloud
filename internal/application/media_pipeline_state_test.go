package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	mediapipeline "github.com/limecloud/contentcloud/internal/integration/provider/media"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

type statusRecoveryProvider struct {
	statusCalls int
	status      []string
	statusErr   error
	actualMinor int64
}

func (p *statusRecoveryProvider) Validate(mediapipeline.Request, deliverydomain.ProviderProfile) error {
	return nil
}

func (p *statusRecoveryProvider) Estimate(mediapipeline.Request, deliverydomain.ProviderProfile) (mediapipeline.Estimate, error) {
	return mediapipeline.Estimate{CostMinor: 1, Currency: "CNY"}, nil
}

func (p *statusRecoveryProvider) Submit(context.Context, mediapipeline.Request, deliverydomain.ProviderProfile) (mediapipeline.Submission, error) {
	return mediapipeline.Submission{}, errors.New("submit must not be called during status recovery")
}

func (p *statusRecoveryProvider) Status(context.Context, string, deliverydomain.ProviderProfile) (mediapipeline.Status, error) {
	p.statusCalls++
	if p.statusCalls == 1 && p.statusErr != nil {
		return mediapipeline.Status{}, p.statusErr
	}
	state := "running"
	if index := p.statusCalls - 1; index >= 0 && index < len(p.status) {
		state = p.status[index]
	}
	return mediapipeline.Status{State: state, ActualMinor: p.actualMinor}, nil
}

func (p *statusRecoveryProvider) Cancel(context.Context, string, deliverydomain.ProviderProfile) error {
	return nil
}

func (p *statusRecoveryProvider) Download(context.Context, string, deliverydomain.ProviderProfile) (mediapipeline.Download, error) {
	return mediapipeline.Download{}, errors.New("download must not be called while provider is running")
}

func TestProviderStatusUnknownRecoversByPollingWithoutResubmit(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	now := time.Now().UTC()
	profile := deliverydomain.ProviderProfile{
		ProviderID:      "state-recovery",
		Version:         "1.0.0",
		Digest:          "sha256:" + strings.Repeat("a", 64),
		AdapterVersion:  "test/1",
		Model:           "model",
		Region:          "global",
		Modes:           []string{"image_to_video"},
		InputMediaTypes: []string{"image/png"},
		OutputMediaType: "video/mp4",
		DataRetention:   "ephemeral",
		Pricing:         map[string]any{"currency": "CNY", "per_job_minor": 1},
		Status:          "published",
		VerifiedAt:      now.Add(-time.Minute),
		ExpiresAt:       now.Add(time.Hour),
	}
	if err := store.CreateProviderProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	provider := &statusRecoveryProvider{statusErr: errors.New("provider status timeout"), status: []string{"running"}}
	service := application.New(application.DependenciesFrom(store), nil, application.WithMediaProviderAdapter(profile.ProviderID, provider))
	job := deliverydomain.MediaGenerationJob{
		ID: "job-status-recovery", TenantID: "tenant-1", ProjectID: "project-1", TaskID: "task-1", StageRunID: "stage-1",
		StoryboardSnapshotID: "snapshot-1", PromptPackageArtifactID: "prompt-1", ProviderID: profile.ProviderID,
		ProfileVersion: profile.Version, ProfileDigest: profile.Digest, Model: profile.Model, Mode: "image_to_video",
		AspectRatio: "9:16", DurationSeconds: 5, State: deliverydomain.MediaJobAwaitingExternal, IdempotencyKey: "status-recovery-key",
		Currency: "CNY", AttemptCount: 1, MaxAttempts: 3, RowVersion: 1, CreatedBy: "user-1", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateMediaGenerationJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	attempt := deliverydomain.ProviderAttempt{
		ID: "attempt-status-recovery", TenantID: job.TenantID, ProjectID: job.ProjectID, GenerationJobID: job.ID,
		AttemptNumber: 1, ProviderID: job.ProviderID, ExternalJobID: "external-status-recovery", ProviderState: "submitted",
		RequestDigest: "sha256:" + strings.Repeat("b", 64), SafeRequestSummary: map[string]any{}, SafeResponseSummary: map[string]any{},
		DisclosureManifest: map[string]any{}, Currency: "CNY", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateProviderAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}

	firstErr := service.Delivery.ProcessMediaGenerationJob(ctx, job.TenantID, job.ID)
	if firstErr == nil {
		t.Fatal("status timeout must be surfaced to the worker")
	}
	stored, err := store.MediaGenerationJob(ctx, job.TenantID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != deliverydomain.MediaJobAwaitingExternal || stored.ErrorCode != "PROVIDER_STATUS_UNKNOWN" {
		t.Fatalf("status timeout must remain reconcilable: %#v", stored)
	}
	attempts, err := store.ProviderAttempts(ctx, job.TenantID, job.ID)
	if err != nil || len(attempts) != 1 || attempts[0].ProviderState != "unknown" || attempts[0].ExternalJobID != attempt.ExternalJobID || attempts[0].NextPollAt == nil || attempts[0].LastPolledAt == nil {
		t.Fatalf("status timeout evidence missing: attempts=%#v err=%v", attempts, err)
	}

	if err := service.Delivery.ProcessMediaGenerationJob(ctx, job.TenantID, job.ID); err != nil {
		t.Fatal(err)
	}
	stored, err = store.MediaGenerationJob(ctx, job.TenantID, job.ID)
	if err != nil || stored.State != deliverydomain.MediaJobAwaitingExternal {
		t.Fatalf("running provider task must remain scheduled: job=%#v err=%v", stored, err)
	}
	attempts, err = store.ProviderAttempts(ctx, job.TenantID, job.ID)
	if err != nil || len(attempts) != 1 || attempts[0].ProviderState != "running" || attempts[0].NextPollAt == nil {
		t.Fatalf("running status evidence missing: attempts=%#v err=%v", attempts, err)
	}
	if provider.statusCalls != 2 {
		t.Fatalf("expected one failed status query and one recovery poll, calls=%d", provider.statusCalls)
	}
}

func TestProviderStatusUnknownRecoversToTerminalStatesWithoutDuplicateAttempt(t *testing.T) {
	tests := []struct {
		name            string
		providerState   string
		cancelRequested bool
		expectJobState  string
		expectError     string
	}{
		{name: "failed", providerState: "failed", expectJobState: deliverydomain.MediaJobFailed, expectError: "PROVIDER_JOB_FAILED"},
		{name: "cancelled_after_cancel_request", providerState: "cancelled", cancelRequested: true, expectJobState: deliverydomain.MediaJobCancelled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			store := memory.New()
			now := time.Now().UTC()
			profile := deliverydomain.ProviderProfile{
				ProviderID: "state-terminal-recovery-" + test.name,
				Version:    "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), AdapterVersion: "test/1", Model: "model", Region: "global",
				Modes: []string{"image_to_video"}, InputMediaTypes: []string{"image/png"}, OutputMediaType: "video/mp4", DataRetention: "ephemeral",
				Pricing: map[string]any{"currency": "CNY", "per_job_minor": 1}, Status: "published", VerifiedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
			}
			if err := store.CreateProviderProfile(ctx, profile); err != nil {
				t.Fatal(err)
			}
			provider := &statusRecoveryProvider{statusErr: errors.New("provider status timeout"), status: []string{"ignored", test.providerState}, actualMinor: 9}
			service := application.New(application.DependenciesFrom(store), nil, application.WithMediaProviderAdapter(profile.ProviderID, provider))
			job := deliverydomain.MediaGenerationJob{
				ID: "job-status-terminal-" + test.name, TenantID: "tenant-1", ProjectID: "project-1", TaskID: "task-1", StageRunID: "stage-1",
				StoryboardSnapshotID: "snapshot-1", PromptPackageArtifactID: "prompt-1", ProviderID: profile.ProviderID, ProfileVersion: profile.Version,
				ProfileDigest: profile.Digest, Model: profile.Model, Mode: "image_to_video", AspectRatio: "9:16", DurationSeconds: 5,
				State: deliverydomain.MediaJobAwaitingExternal, IdempotencyKey: "status-terminal-" + test.name, Currency: "CNY", AttemptCount: 1,
				MaxAttempts: 3, RowVersion: 1, CreatedBy: "user-1", CreatedAt: now, UpdatedAt: now,
			}
			if test.cancelRequested {
				cancelledAt := now.Add(time.Minute)
				job.CancelRequestedAt = &cancelledAt
			}
			if err := store.CreateMediaGenerationJob(ctx, job); err != nil {
				t.Fatal(err)
			}
			attempt := deliverydomain.ProviderAttempt{
				ID: "attempt-status-terminal-" + test.name, TenantID: job.TenantID, ProjectID: job.ProjectID, GenerationJobID: job.ID,
				AttemptNumber: 1, ProviderID: job.ProviderID, ExternalJobID: "external-status-terminal-" + test.name, ProviderState: "submitted",
				RequestDigest: "sha256:" + strings.Repeat("b", 64), SafeRequestSummary: map[string]any{}, SafeResponseSummary: map[string]any{},
				DisclosureManifest: map[string]any{}, Currency: "CNY", CreatedAt: now, UpdatedAt: now,
			}
			if err := store.CreateProviderAttempt(ctx, attempt); err != nil {
				t.Fatal(err)
			}

			if err := service.Delivery.ProcessMediaGenerationJob(ctx, job.TenantID, job.ID); err == nil {
				t.Fatal("status timeout must be surfaced to the worker")
			}
			if err := service.Delivery.ProcessMediaGenerationJob(ctx, job.TenantID, job.ID); test.expectError == "" && err != nil {
				t.Fatal(err)
			} else if test.expectError != "" && !containsCode(err, test.expectError) {
				t.Fatalf("expected %s, got %v", test.expectError, err)
			}

			stored, err := store.MediaGenerationJob(ctx, job.TenantID, job.ID)
			if err != nil || stored.State != test.expectJobState || stored.ActualCostMinor != 9 {
				t.Fatalf("terminal recovery job=%#v err=%v", stored, err)
			}
			attempts, err := store.ProviderAttempts(ctx, job.TenantID, job.ID)
			if err != nil || len(attempts) != 1 || attempts[0].ActualCostMinor != 9 || attempts[0].ExternalJobID != attempt.ExternalJobID {
				t.Fatalf("terminal recovery attempts=%#v err=%v", attempts, err)
			}
			if provider.statusCalls != 2 {
				t.Fatalf("expected one unknown and one recovery poll, calls=%d", provider.statusCalls)
			}
		})
	}
}
