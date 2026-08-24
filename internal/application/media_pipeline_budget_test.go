package application

import (
	"strings"
	"testing"
	"time"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

func TestMediaProviderUsageAndMonthlyBudgetGate(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	app := New(DependenciesFrom(store), nil)
	now := time.Now().UTC()
	const tenantID = "tenant-budget"
	const providerID = "provider-budget"
	if err := store.CreateProviderProfile(ctx, deliverydomain.ProviderProfile{
		ProviderID: providerID, Version: "1.0.0", Digest: "sha256:" + strings.Repeat("b", 64), AdapterVersion: "budget/1.0.0",
		Model: "budget-model", Region: "local", Modes: []string{"text_to_video"}, OutputMediaType: "video/mp4",
		DataRetention: "ephemeral", Status: "published", VerifiedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProviderBinding(ctx, deliverydomain.ProviderBinding{
		TenantID: tenantID, ProviderID: providerID, ProfileVersion: "1.0.0", State: "active",
		CredentialRef: "secret://provider-budget", EgressPolicy: "provider-only",
		MonthlyBudgetMinor: 100, MaxJobCostMinor: 100, MaxConcurrency: 2, UpdatedBy: "test", UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, job := range []deliverydomain.MediaGenerationJob{
		budgetFixtureJob("budget-queued", tenantID, providerID, deliverydomain.MediaJobQueued, 80, 0, now),
		budgetFixtureJob("budget-succeeded", tenantID, providerID, deliverydomain.MediaJobSucceeded, 50, 10, now),
	} {
		if err := store.CreateMediaGenerationJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	usage, err := store.MediaProviderUsage(ctx, tenantID, providerID, monthStart, monthStart.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if usage.ActualCostMinor != 10 || usage.EstimatedCostMinor != 80 || usage.CommittedCostMinor != 90 || usage.ActiveJobs != 1 {
		t.Fatalf("unexpected provider usage: %#v", usage)
	}
	blockers := app.Delivery.mediaBudgetBlockers(ctx, tenantID, []deliverydomain.MediaGenerationJob{
		budgetFixtureJob("budget-request", tenantID, providerID, deliverydomain.MediaJobQueued, 20, 0, now),
	})
	if len(blockers) != 1 || blockers[0].Code != "MEDIA_BATCH_MONTHLY_BUDGET_EXCEEDED" {
		t.Fatalf("expected monthly budget blocker, got %#v", blockers)
	}

	if err := store.SaveProviderBinding(ctx, deliverydomain.ProviderBinding{
		TenantID: tenantID, ProviderID: providerID, ProfileVersion: "1.0.0", State: "active",
		CredentialRef: "secret://provider-budget", EgressPolicy: "provider-only",
		MonthlyBudgetMinor: 200, MaxJobCostMinor: 100, MaxConcurrency: 2, UpdatedBy: "test", UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if blockers = app.Delivery.mediaBudgetBlockers(ctx, tenantID, []deliverydomain.MediaGenerationJob{
		budgetFixtureJob("budget-request-allowed", tenantID, providerID, deliverydomain.MediaJobQueued, 20, 0, now),
	}); len(blockers) != 0 {
		t.Fatalf("unexpected budget blocker after raising limit: %#v", blockers)
	}
}

func TestMediaProviderConcurrencyGate(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	app := New(DependenciesFrom(store), nil)
	now := time.Now().UTC()
	const tenantID = "tenant-concurrency"
	const providerID = "provider-concurrency"
	if err := store.CreateProviderProfile(ctx, deliverydomain.ProviderProfile{
		ProviderID: providerID, Version: "1.0.0", Digest: "sha256:" + strings.Repeat("c", 64), AdapterVersion: "concurrency/1.0.0",
		Model: "concurrency-model", Region: "local", Modes: []string{"text_to_video"}, OutputMediaType: "video/mp4",
		DataRetention: "ephemeral", Status: "published", VerifiedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProviderBinding(ctx, deliverydomain.ProviderBinding{
		TenantID: tenantID, ProviderID: providerID, ProfileVersion: "1.0.0", State: "active",
		CredentialRef: "secret://provider-concurrency", EgressPolicy: "provider-only",
		MonthlyBudgetMinor: 0, MaxJobCostMinor: 100, MaxConcurrency: 2, UpdatedBy: "test", UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateMediaGenerationJob(ctx, budgetFixtureJob("concurrency-active", tenantID, providerID, deliverydomain.MediaJobGenerating, 1, 0, now)); err != nil {
		t.Fatal(err)
	}
	jobs := []deliverydomain.MediaGenerationJob{
		budgetFixtureJob("concurrency-request-1", tenantID, providerID, deliverydomain.MediaJobQueued, 1, 0, now),
		budgetFixtureJob("concurrency-request-2", tenantID, providerID, deliverydomain.MediaJobQueued, 1, 0, now),
	}
	blockers := app.Delivery.mediaConcurrencyBlockers(ctx, tenantID, jobs)
	if len(blockers) != 1 || blockers[0].Code != "MEDIA_BATCH_CONCURRENCY_EXCEEDED" {
		t.Fatalf("expected concurrency blocker, got %#v", blockers)
	}
	blockers = app.Delivery.mediaConcurrencyBlockers(ctx, tenantID, jobs[:1])
	if len(blockers) != 0 {
		t.Fatalf("expected exact concurrency boundary to pass, got %#v", blockers)
	}
}

func budgetFixtureJob(id, tenantID, providerID, state string, estimated, actual int64, createdAt time.Time) deliverydomain.MediaGenerationJob {
	return deliverydomain.MediaGenerationJob{
		ID: id, TenantID: tenantID, ProjectID: "project-budget", TaskID: "task-budget", StageRunID: "stage-budget",
		StoryboardSnapshotID: "snapshot-budget", ProviderID: providerID, ProfileVersion: "1.0.0",
		ProfileDigest: "sha256:" + strings.Repeat("a", 64), Model: "budget-model", Mode: "text_to_video",
		AspectRatio: "9:16", DurationSeconds: 5, State: state, IdempotencyKey: id, EstimatedCostMinor: estimated,
		ActualCostMinor: actual, Currency: "CNY", MaxAttempts: 1, RowVersion: 1, CreatedBy: "test", CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}
