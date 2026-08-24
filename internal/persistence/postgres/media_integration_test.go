package postgres_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	storepg "github.com/limecloud/contentcloud/internal/persistence/postgres"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/testsupport"
	"github.com/limecloud/contentcloud/internal/work"
)

// TestMediaPipelinePersistenceWithPostgres is intentionally skipped unless a
// dedicated integration database is supplied. It must never exercise a
// developer or production database by accident.
func TestMediaPipelinePersistenceWithPostgres(t *testing.T) {
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

	service := application.New(application.DependenciesFrom(store), slog.Default())
	suffix := idgen.New()
	session, err := service.Identity.Register(ctx, fmt.Sprintf("media-pg-%s@example.com", suffix), "long-enough-password", "Media Owner", "Media Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, task, stageRun := createPostgresMediaTask(t, ctx, store, service, actor, suffix)

	now := time.Now().UTC().Truncate(time.Microsecond)
	providerID := "postgres-media-" + suffix
	profile := deliverydomain.ProviderProfile{
		ProviderID: providerID, Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64),
		AdapterVersion: "test/1.0.0", Model: "test-video", Region: "test",
		Modes: []string{"text_to_video", "image_to_video"}, InputMediaTypes: []string{"image/png"},
		OutputMediaType: "video/mp4", DataRetention: "ephemeral", Pricing: map[string]any{"currency": "CNY", "per_job_minor": 12},
		Status: "published", VerifiedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
	}
	if err := store.CreateProviderProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	persistedProfile, err := store.ProviderProfile(ctx, profile.ProviderID, profile.Version)
	if err != nil || persistedProfile.Digest != profile.Digest || persistedProfile.Model != profile.Model {
		t.Fatalf("provider profile did not round-trip: profile=%#v err=%v", persistedProfile, err)
	}

	binding := deliverydomain.ProviderBinding{
		TenantID: actor.TenantID, ProviderID: providerID, ProfileVersion: profile.Version,
		State: "active", CredentialRef: "secret://integration/media", EgressPolicy: "provider-only",
		MonthlyBudgetMinor: 1000, MaxJobCostMinor: 100, MaxConcurrency: 2, MaxRetries: 2,
		UpdatedBy: actor.UserID, UpdatedAt: now,
	}
	if err := store.SaveProviderBinding(ctx, binding); err != nil {
		t.Fatal(err)
	}
	persistedBinding, err := store.ProviderBinding(ctx, actor.TenantID, providerID)
	if err != nil || persistedBinding.CredentialRef != binding.CredentialRef || persistedBinding.MaxRetries != binding.MaxRetries {
		t.Fatalf("provider binding did not round-trip: binding=%#v err=%v", persistedBinding, err)
	}

	job := deliverydomain.MediaGenerationJob{
		ID: idgen.New(), TenantID: actor.TenantID, ProjectID: project.ID, TaskID: task.ID, StageRunID: stageRun.ID,
		StoryboardSnapshotID: "snapshot:" + suffix, ProviderID: providerID, ProfileVersion: profile.Version,
		ProfileDigest: profile.Digest, Model: profile.Model, Mode: "text_to_video", AspectRatio: "9:16",
		DurationSeconds: 8, State: deliverydomain.MediaJobQueued, IdempotencyKey: "media-job-" + suffix,
		Currency: "CNY", EstimatedCostMinor: 12, MaxAttempts: 3, RowVersion: 1, CreatedBy: actor.UserID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateMediaGenerationJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	duplicate := job
	duplicate.ID = idgen.New()
	if err := store.CreateMediaGenerationJob(ctx, duplicate); err == nil {
		t.Fatal("duplicate media job idempotency key unexpectedly succeeded")
	}
	persistedJob, err := store.MediaGenerationJob(ctx, actor.TenantID, job.ID)
	if err != nil || persistedJob.IdempotencyKey != job.IdempotencyKey || persistedJob.RowVersion != 1 {
		t.Fatalf("media job did not round-trip: job=%#v err=%v", persistedJob, err)
	}

	firstAttempt := deliverydomain.ProviderAttempt{
		ID: idgen.New(), TenantID: actor.TenantID, ProjectID: project.ID, GenerationJobID: job.ID,
		AttemptNumber: 1, ProviderID: providerID, RequestDigest: "sha256:" + strings.Repeat("b", 64),
		ExternalJobID: "external-" + suffix, ProviderState: "unknown", SafeRequestSummary: map[string]any{"mode": job.Mode},
		SafeResponseSummary: map[string]any{}, DisclosureManifest: map[string]any{}, Currency: "CNY",
		EstimatedCostMinor: 12, ErrorCode: "PROVIDER_STATUS_UNKNOWN", ErrorDetailSafe: "status timeout",
		CreatedAt: now, UpdatedAt: now,
	}
	lastPolled := now.Add(time.Minute)
	nextPoll := now.Add(2 * time.Minute)
	firstAttempt.LastPolledAt = &lastPolled
	firstAttempt.NextPollAt = &nextPoll
	if err := store.CreateProviderAttempt(ctx, firstAttempt); err != nil {
		t.Fatal(err)
	}
	reloadedAttempts, err := store.ProviderAttempts(ctx, actor.TenantID, job.ID)
	if err != nil || len(reloadedAttempts) != 1 || reloadedAttempts[0].ExternalJobID != firstAttempt.ExternalJobID || reloadedAttempts[0].NextPollAt == nil || reloadedAttempts[0].ErrorCode != firstAttempt.ErrorCode {
		t.Fatalf("unknown attempt recovery fields were not persisted: attempts=%#v err=%v", reloadedAttempts, err)
	}

	// Recovery updates the existing attempt. A reload must not require a new
	// attempt or a second external submission fact.
	recoveredAttempt := reloadedAttempts[0]
	recoveredAttempt.ProviderState = "running"
	recoveredAttempt.ErrorCode = ""
	recoveredAttempt.ErrorDetailSafe = ""
	recoveredAttempt.UpdatedAt = now.Add(3 * time.Minute)
	if err := store.SaveProviderAttempt(ctx, recoveredAttempt); err != nil {
		t.Fatal(err)
	}
	reloadedAttempts, err = store.ProviderAttempts(ctx, actor.TenantID, job.ID)
	if err != nil || len(reloadedAttempts) != 1 || reloadedAttempts[0].ProviderState != "running" {
		t.Fatalf("unknown attempt recovery created or lost a fact: attempts=%#v err=%v", reloadedAttempts, err)
	}

	// Exercise the CAS path through every valid terminal transition and keep
	// the job and attempt on the same actual-cost amount.
	transitions := []string{deliverydomain.MediaJobSubmitting, deliverydomain.MediaJobSubmitted, deliverydomain.MediaJobGenerating, deliverydomain.MediaJobDownloading, deliverydomain.MediaJobValidating, deliverydomain.MediaJobSucceeded}
	for _, state := range transitions {
		persistedJob.State = state
		persistedJob.UpdatedAt = now.Add(4 * time.Minute)
		if state == deliverydomain.MediaJobSucceeded {
			persistedJob.ActualCostMinor = 12
		}
		if err := store.SaveMediaGenerationJob(ctx, persistedJob, persistedJob.RowVersion); err != nil {
			t.Fatalf("transition to %s failed: %v", state, err)
		}
		persistedJob.RowVersion++
	}
	stale := persistedJob
	stale.State = deliverydomain.MediaJobSucceeded
	if err := store.SaveMediaGenerationJob(ctx, stale, persistedJob.RowVersion-1); err == nil {
		t.Fatal("stale media job version unexpectedly overwrote a newer row")
	}
	reloadedJob, err := store.MediaGenerationJob(ctx, actor.TenantID, job.ID)
	if err != nil || reloadedJob.State != deliverydomain.MediaJobSucceeded || reloadedJob.ActualCostMinor != 12 {
		t.Fatalf("terminal media job cost was not persisted: job=%#v err=%v", reloadedJob, err)
	}
	reloadedAttempts, err = store.ProviderAttempts(ctx, actor.TenantID, job.ID)
	if err != nil || len(reloadedAttempts) != 1 {
		t.Fatalf("attempt cardinality changed after recovery: attempts=%#v err=%v", reloadedAttempts, err)
	}
	reloadedAttempts[0].ActualCostMinor = reloadedJob.ActualCostMinor
	reloadedAttempts[0].CompletedAt = ptrTime(now.Add(5 * time.Minute))
	reloadedAttempts[0].ProviderState = "succeeded"
	reloadedAttempts[0].UpdatedAt = now.Add(5 * time.Minute)
	if err := store.SaveProviderAttempt(ctx, reloadedAttempts[0]); err != nil {
		t.Fatal(err)
	}
	finalAttempts, err := store.ProviderAttempts(ctx, actor.TenantID, job.ID)
	if err != nil || len(finalAttempts) != 1 || finalAttempts[0].ActualCostMinor != reloadedJob.ActualCostMinor {
		t.Fatalf("job and attempt actual cost diverged: job=%#v attempts=%#v err=%v", reloadedJob, finalAttempts, err)
	}

	usage, err := store.MediaProviderUsage(ctx, actor.TenantID, providerID, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil || usage.ActualCostMinor != 12 || usage.CommittedCostMinor != 12 {
		t.Fatalf("provider usage did not aggregate terminal cost: usage=%#v err=%v", usage, err)
	}

	otherSession, err := service.Identity.Register(ctx, fmt.Sprintf("media-pg-other-%s@example.com", suffix), "long-enough-password", "Other", "Other Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	otherActor, _, err := service.Identity.SessionActor(ctx, otherSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ProviderBinding(ctx, otherActor.TenantID, providerID); err == nil {
		t.Fatal("tenant B read tenant A provider binding through Store")
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
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE contentcloud_runtime`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id',$1,true)`, otherActor.TenantID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM media_generation_jobs WHERE tenant_id=$1 AND id=$2`, actor.TenantID, job.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("tenant B saw tenant A media job through RLS")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO provider_bindings(tenant_id,provider_id,profile_version,state,credential_ref,egress_policy,max_concurrency,max_retries,updated_by,updated_at) VALUES($1,$2,$3,'active','secret://cross-tenant','provider-only',1,0,'cross-tenant',now())`, actor.TenantID, providerID, profile.Version); err == nil {
		t.Fatal("tenant B inserted a tenant A provider binding through RLS")
	}
}

// TestPostgresFinalRenderAtomicityWithPostgres is guarded by a dedicated
// integration database and never falls back to a developer or production DB.
func TestPostgresFinalRenderAtomicityWithPostgres(t *testing.T) {
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

	service := application.New(application.DependenciesFrom(store), slog.Default())
	suffix := idgen.New()
	session, err := service.Identity.Register(ctx, fmt.Sprintf("final-render-%s@example.com", suffix), "long-enough-password", "Final Render Owner", "Final Render Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, task, _ := createPostgresMediaTask(t, ctx, store, service, actor, suffix)

	connect, err := service.Workspace.CreateConnectSession(ctx, actor, project.ID, "final-render-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(ctx, service, actor, connect, application.ConnectDeviceInput{Hostname: "final-render-pg", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.Workspace.WorkspaceActor(ctx, connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	binding, err = service.Review.RegisterWorkspace(ctx, workspaceActor, binding, "workspace_marketing_video", "3.0.0", []string{"codex"}, "final-render-register")
	if err != nil {
		t.Fatal(err)
	}
	bundle := postgresSubmissionBundle(t, project.ID, binding.ID, "final-render-snapshot-"+suffix, "final-render-fact-"+suffix)
	revision, err := service.Review.CreateSubmission(ctx, workspaceActor, binding, bundle, "final-render-submit")
	if err != nil {
		t.Fatal(err)
	}
	approval, err := service.Review.ApproveSubmission(ctx, actor, revision.ID, "final render snapshot", "final-render-approve")
	if err != nil || approval.ApprovedSnapshot == nil {
		t.Fatalf("approved snapshot setup failed: snapshot=%#v err=%v", approval.ApprovedSnapshot, err)
	}
	snapshot := *approval.ApprovedSnapshot
	now := time.Now().UTC().Truncate(time.Microsecond)
	digest := "sha256:" + strings.Repeat("d", 64)
	artifact := deliverydomain.Artifact{
		ID: idgen.New(), TenantID: actor.TenantID, ProjectID: project.ID, ApprovedSnapshotID: snapshot.ID,
		Kind: "final_render", CapabilityID: "contentcloud.media.final-render", CapabilityVersion: "1.0.0", CapabilityDigest: digest,
		SchemaID: "contentcloud.final-render/1.0", MediaType: "video/mp4", FileName: "final.mp4", SHA256: digest,
		ByteSize: 4, ObjectKey: "media/final-render/" + suffix + "/final.mp4", Visibility: "client", RetentionClass: "audit", Purpose: "final_video",
		Metadata: map[string]any{"render_manifest_digest": digest}, CreatedAt: now,
	}
	review := deliverydomain.MediaReview{
		ID: idgen.New(), TenantID: actor.TenantID, ProjectID: project.ID, TaskID: task.ID, SubjectArtifactID: artifact.ID,
		SubjectDigest: digest, ReviewKind: deliverydomain.MediaReviewFinal, Status: deliverydomain.MediaReviewPending,
		Checks: map[string]any{}, RowVersion: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateFinalRender(ctx, artifact, review); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Artifact(ctx, actor.TenantID, artifact.ID); err != nil {
		t.Fatalf("final render artifact was not committed: %v", err)
	}
	persistedReview, err := store.MediaReview(ctx, actor.TenantID, review.ID)
	if err != nil || persistedReview.SubjectArtifactID != artifact.ID {
		t.Fatalf("final render review was not committed with artifact: review=%#v err=%v", persistedReview, err)
	}

	rolledBackArtifact := artifact
	rolledBackArtifact.ID = idgen.New()
	rolledBackArtifact.ObjectKey = "media/final-render/" + suffix + "/rolled-back.mp4"
	rolledBackReview := review
	rolledBackReview.SubjectArtifactID = rolledBackArtifact.ID
	if err := store.CreateFinalRender(ctx, rolledBackArtifact, rolledBackReview); err == nil {
		t.Fatal("duplicate review ID unexpectedly committed a second artifact")
	}
	if _, err := store.Artifact(ctx, actor.TenantID, rolledBackArtifact.ID); err == nil {
		t.Fatal("artifact remained after final review insert failed; transaction was not atomic")
	}

	otherSession, err := service.Identity.Register(ctx, fmt.Sprintf("final-render-other-%s@example.com", suffix), "long-enough-password", "Other", "Other Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	otherActor, _, err := service.Identity.SessionActor(ctx, otherSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Artifact(ctx, otherActor.TenantID, artifact.ID); err == nil {
		t.Fatal("tenant B read tenant A final render artifact")
	}
}

func createPostgresMediaTask(t *testing.T, ctx context.Context, store *storepg.Store, service *application.Application, actor application.Actor, suffix string) (project struct{ ID string }, task work.WorkTask, stageRun work.StageRun) {
	t.Helper()
	if err := store.SetTenantContentCapability(ctx, identitydomain.TenantContentCapability{
		TenantID:    actor.TenantID,
		ContentType: identitydomain.ContentTypeMarketingVideo,
		Enabled:     true,
		UpdatedBy:   actor.UserID,
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	createdProject, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Media Brand " + suffix, ProductName: "Media Product", ContentType: identitydomain.ContentTypeMarketingVideo, Channel: "douyin"}, "")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := service.Work.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var sop catalogdomain.SOPVersion
	for _, summary := range admin.SOPs {
		if summary.Definition.TemplateKey != "marketing_video_production" {
			continue
		}
		for _, version := range summary.Versions {
			if version.Status == "published" {
				sop = version
				break
			}
		}
	}
	if sop.ID == "" {
		t.Fatal("marketing video SOP was not provisioned")
	}
	environment, err := service.Work.CreateEnvironment(ctx, actor, application.SaveEnvironmentInput{Name: "Media Environment " + suffix, Slug: "media-" + suffix, Status: "active", DefaultSOPID: sop.SOPID, DefaultSOPVersion: sop.Version}, "")
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.Work.CreateWorkTask(ctx, actor, application.CreateWorkTaskInput{ProjectID: createdProject.ID, EnvironmentID: environment.ID, Title: "Media persistence", ContentType: identitydomain.ContentTypeMarketingVideo, InputRefs: []string{"brief:media"}, IdempotencyKey: "task-" + suffix}, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	stageRun = work.StageRun{ID: idgen.New(), TenantID: actor.TenantID, TaskID: view.Task.ID, StageID: "generation", Status: work.StageRunStatusRunning, ExecutionMode: "manual", StartedAt: &now, UpdatedAt: now}
	if err := store.CreateStageRun(ctx, stageRun); err != nil {
		t.Fatal(err)
	}
	return struct{ ID string }{ID: createdProject.ID}, view.Task, stageRun
}

func ptrTime(value time.Time) *time.Time { return &value }
