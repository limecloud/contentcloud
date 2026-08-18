package httpapi_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	"github.com/limecloud/contentcloud/internal/application"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	mediapipeline "github.com/limecloud/contentcloud/internal/integration/provider/media"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestSeedancePromptPackageUploadBFF(t *testing.T) {
	fixture := newSeedanceHTTPFixture(t)
	server := httptest.NewServer(httpapi.New(fixture.service, slog.Default(), false, "").Handler())
	defer server.Close()
	client := fixture.client(server.URL)

	artifact := postSeedancePromptPackage(t, client, server.URL+"/api/bff/tasks/"+fixture.taskID+"/seedance-prompt-package", fixture.snapshotID, fixture.promptBody)
	if artifact.Kind != "prompt_package" || artifact.ApprovedSnapshotID != fixture.snapshotID || artifact.MediaType != "application/json" {
		t.Fatalf("prompt package upload returned an incomplete Artifact: %#v", artifact)
	}
	stored, err := fixture.store.Artifact(t.Context(), fixture.tenantID, artifact.ID)
	if err != nil || stored.SHA256 != mediapipeline.SHA256(fixture.promptBody) {
		t.Fatalf("prompt package upload did not persist content digest: artifact=%#v err=%v", stored, err)
	}
}

func TestSeedanceSubmitReconciliationBFF(t *testing.T) {
	fixture := newSeedanceHTTPFixture(t)
	now := time.Now().UTC()
	job := deliverydomain.MediaGenerationJob{
		ID: fixture.jobID, TenantID: fixture.tenantID, ProjectID: fixture.projectID, TaskID: fixture.taskID,
		StageRunID: idgen.New(), StoryboardSnapshotID: fixture.snapshotID, ProviderID: "modelark-seedance25", ProfileVersion: "1.0.0",
		ProfileDigest: "sha256:" + strings.Repeat("d", 64), Model: "dreamina-seedance-2-5-260628", Mode: "image_to_video", AspectRatio: "9:16",
		DurationSeconds: 5, State: deliverydomain.MediaJobAwaitingExternal, IdempotencyKey: "reconcile-http-job", Currency: "CNY", MaxAttempts: 3,
		RowVersion: 1, CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now,
	}
	if err := fixture.store.CreateMediaGenerationJob(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	attempt := deliverydomain.ProviderAttempt{ID: idgen.New(), TenantID: fixture.tenantID, ProjectID: fixture.projectID, GenerationJobID: job.ID, AttemptNumber: 1, ProviderID: job.ProviderID, RequestDigest: "sha256:" + strings.Repeat("e", 64), ProviderState: "unknown", EstimatedCostMinor: 1, Currency: "CNY", CreatedAt: now, UpdatedAt: now}
	if err := fixture.store.CreateProviderAttempt(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(fixture.service, slog.Default(), false, "").Handler())
	defer server.Close()
	client := fixture.client(server.URL)
	value := callBFF[deliverydomain.MediaGenerationJob](t, client, http.MethodPost, server.URL+"/api/bff/media-jobs/"+job.ID+"/reconcile-submit", application.MediaJobSubmitReconciliationInput{ExpectedVersion: 1, ExternalJobID: "ark-task-123"})
	if value.RowVersion != 2 || value.State != deliverydomain.MediaJobAwaitingExternal {
		t.Fatalf("submit reconciliation changed unexpected job state: %#v", value)
	}
	attempts, err := fixture.store.ProviderAttempts(t.Context(), fixture.tenantID, job.ID)
	if err != nil || len(attempts) != 1 || attempts[0].ExternalJobID != "ark-task-123" || attempts[0].ProviderState != "reconciliation_pending" {
		t.Fatalf("submit reconciliation did not bind external task ID: attempts=%#v err=%v", attempts, err)
	}
}

type seedanceHTTPFixture struct {
	service    *application.Application
	store      *memory.Store
	actor      application.Actor
	userID     string
	tenantID   string
	projectID  string
	taskID     string
	snapshotID string
	jobID      string
	promptBody []byte
	sessionID  string
}

func newSeedanceHTTPFixture(t *testing.T) seedanceHTTPFixture {
	t.Helper()
	ctx := t.Context()
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	session, err := service.Identity.Register(ctx, "seedance-http@example.com", "long-enough-password", "视频负责人", "视频团队")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "测试品牌", ProductName: "测试产品", Channel: "douyin", ContentType: identitydomain.ContentTypeMarketingVideo}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetTenantContentCapability(ctx, identitydomain.TenantContentCapability{TenantID: actor.TenantID, ContentType: identitydomain.ContentTypeMarketingVideo, Enabled: true, UpdatedBy: actor.UserID, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	task, err := service.Work.CreateWorkTask(ctx, actor, application.CreateWorkTaskInput{ProjectID: project.ID, Title: "Seedance HTTP 测试", ContentType: identitydomain.ContentTypeMarketingVideo, InputRefs: []string{"brief:test"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	// The endpoint is stage-scoped; the fixture starts after storyboard approval
	// so the test can focus on the HTTP contract rather than replaying all stages.
	now := time.Now().UTC()
	if err := store.CreateWorkspaceBinding(ctx, workspacedomain.WorkspaceBinding{ID: task.Task.ID, TenantID: actor.TenantID, ProjectID: project.ID, OwnerUserID: actor.UserID, TemplateID: localworkspace.TemplateID, TemplateVersion: localworkspace.TemplateVersion, Targets: []string{"web"}, CredentialHash: "seedance-http-credential-" + task.Task.ID, Status: "active", InitializedAt: now, LastSeenAt: now}); err != nil {
		t.Fatal(err)
	}
	task.Task.CurrentStageID = "storyboard"
	task.Task.Status = work.TaskStatusRunning
	if err := store.SaveWorkTask(ctx, task.Task); err != nil {
		t.Fatal(err)
	}

	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	assetHash := mediapipeline.SHA256(png)
	storyboard := work.StoryboardPackage{
		ID: "storyboard:" + task.Task.ID, Type: "storyboard_package", SchemaVersion: work.StoryboardPackageSchema, ProjectID: project.ID,
		ApprovedSnapshotID: "snapshot-pending", ContentItemID: "content-item:" + task.Task.ID,
		GeneratorCapability: work.CapabilityRef{ID: "contentcloud.storyboard.generator", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)},
		Status:              "review_ready", ReviewSheetArtifactID: "asset-review-sheet", SourceDigest: "sha256:" + strings.Repeat("b", 64),
		RightsRefs: []string{"rights:test"},
		Shots:      []work.StoryboardShot{{ShotID: "shot-1", StartMS: 0, EndMS: 4000, Role: "hero", FirstFrameArtifactID: "asset-first-frame", ImagePromptZH: "测试产品首帧", PlanB: "保持主体构图", NegativeConstraints: []string{"无文字"}, AcceptanceCriteria: []string{"首帧稳定"}}},
		Assets:     []work.StoryboardAsset{{ID: "asset-first-frame", Role: "first_frame", ShotID: "shot-1", Path: "first-frame.png", MediaType: "image/png", SHA256: assetHash, ByteSize: int64(len(png)), RightsRefs: []string{"rights:test"}}, {ID: "asset-review-sheet", Role: "review_sheet", Path: "review-sheet.png", MediaType: "image/png", SHA256: assetHash, ByteSize: int64(len(png)), RightsRefs: []string{"rights:test"}}},
	}
	storyboard.LockedDigest, err = storyboard.ComputedLockedDigest()
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.WorkspaceBinding(ctx, actor.TenantID, task.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	object, err := reviewdomain.NewSubmissionObjectRef(storyboard.ID, "storyboard_package", 1, "storyboard/"+storyboard.ID+".json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	now = time.Now().UTC()
	submissionID, revisionID := idgen.New(), idgen.New()
	submission := reviewdomain.Submission{ID: submissionID, TenantID: actor.TenantID, ProjectID: project.ID, WorkspaceID: workspace.ID, SubmissionType: "storyboard", Status: "submitted", CurrentRevisionID: revisionID, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	revision := reviewdomain.SubmissionRevision{ID: revisionID, TenantID: actor.TenantID, ProjectID: project.ID, WorkspaceID: workspace.ID, SubmissionID: submissionID, RevisionNo: 1, SchemaVersion: reviewdomain.SubmissionSchemaVersion("storyboard"), ContentHash: object.Digest, EnvironmentDigest: task.Task.SOPDigest, Objects: []reviewdomain.SubmissionObjectRef{object}, IdempotencyKey: "seedance-http-submission", CreatedBy: actor.UserID, CreatedAt: now}
	if err := store.CreateSubmissionRevision(ctx, submission, revision, nil, reviewdomain.ReviewCycle{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: project.ID, SubjectType: "submission_revision", SubjectID: revisionID, Status: "open", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	snapshotID := idgen.New()
	storyboard.ApprovedSnapshotID = snapshotID
	canonical, err := json.Marshal(map[string]any{"objects": []json.RawMessage{object.Content}})
	if err != nil {
		t.Fatal(err)
	}
	// Recompute the locked digest after binding the real approved snapshot ID.
	storyboard.LockedDigest, err = storyboard.ComputedLockedDigest()
	if err != nil {
		t.Fatal(err)
	}
	object, err = reviewdomain.NewSubmissionObjectRef(storyboard.ID, "storyboard_package", 1, "storyboard/"+storyboard.ID+".json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err = json.Marshal(map[string]any{"objects": []json.RawMessage{object.Content}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := reviewdomain.ApprovedSnapshot{ID: snapshotID, TenantID: actor.TenantID, ProjectID: project.ID, WorkspaceID: workspace.ID, SubmissionID: submissionID, SubmissionRevisionID: revisionID, SubmissionType: "storyboard", SchemaVersion: reviewdomain.SubmissionSchemaVersion("storyboard"), ContentHash: object.Digest, SubjectHash: object.Digest, CanonicalContent: canonical, EligibleIDs: []string{storyboard.ID}, DecisionID: idgen.New(), CreatedBy: actor.UserID, CreatedAt: now}
	decision := reviewdomain.ApprovalDecision{ID: snapshot.DecisionID, TenantID: actor.TenantID, ProjectID: project.ID, SubjectType: "submission_revision", SubjectID: revisionID, SubjectHash: object.Digest, DecisionStage: "internal", ActorID: actor.UserID, Decision: "approve", PreviousState: "submitted", ResultingState: "approved", CreatedAt: now}
	// The package must reference the final snapshot ID and locked digest.
	storyboardSnapshotBody := canonical
	var snapshotEnvelope struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(storyboardSnapshotBody, &snapshotEnvelope); err != nil || len(snapshotEnvelope.Objects) != 1 {
		t.Fatal("storyboard snapshot object missing")
	}
	var locked work.StoryboardPackage
	if err := json.Unmarshal(snapshotEnvelope.Objects[0], &locked); err != nil {
		t.Fatal(err)
	}
	prompt := work.SeedancePromptPackage{ID: "prompt-package:" + task.Task.ID, Type: "seedance_prompt_package", SchemaVersion: work.SeedancePromptPackageSchema, StoryboardSnapshotID: snapshotID, StoryboardPackageID: locked.ID, StoryboardLockedDigest: locked.LockedDigest, Provider: "seedance", ProviderProfileVersion: "1.0.0", AdapterCapability: work.CapabilityRef{ID: "contentcloud.seedance-execution", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("c", 64)}, Mode: "all_reference", Settings: work.SeedanceSettings{AspectRatio: "9:16", DurationSeconds: 5, Sound: "environment_only"}, UploadManifest: []work.SeedanceUpload{{Reference: "@图片1", ArtifactID: "asset-first-frame", File: "first-frame.png", Purpose: "first_frame", SHA256: assetHash}}, Segments: []work.SeedanceSegment{{ID: "segment-1", Order: 1, StartMS: 0, EndMS: 4000, PromptZH: "@图片1 镜头向前推进", AcceptanceCriteria: []string{"首帧稳定"}}}, Validation: work.SeedanceValidation{ReferencesChecked: true, LimitsChecked: true, RightsChecked: true, OfferChecked: true, DigestChecked: true}, Status: "validated"}
	promptBody, err := json.Marshal(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApproveSubmissionRevision(ctx, submission, snapshot, decision); err != nil {
		t.Fatal(err)
	}
	return seedanceHTTPFixture{service: service, store: store, actor: actor, userID: actor.UserID, tenantID: actor.TenantID, projectID: project.ID, taskID: task.Task.ID, snapshotID: snapshotID, jobID: idgen.New(), promptBody: promptBody, sessionID: session.ID}
}

func (f seedanceHTTPFixture) client(serverURL string) *http.Client {
	jar, _ := cookiejar.New(nil)
	base, _ := http.NewRequest(http.MethodGet, serverURL, nil)
	jar.SetCookies(base.URL, []*http.Cookie{{Name: "cc_session", Value: f.cookieSessionID(), Path: "/"}})
	return &http.Client{Jar: jar}
}

func (f seedanceHTTPFixture) cookieSessionID() string {
	return f.sessionID
}

func postSeedancePromptPackage(t *testing.T, client *http.Client, target, snapshotID string, body []byte) deliverydomain.Artifact {
	t.Helper()
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	if err := writer.WriteField("snapshot_id", snapshotID); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "prompt-package.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target, &payload)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		OK    bool                    `json:"ok"`
		Data  deliverydomain.Artifact `json:"data"`
		Error *fault.Error            `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !envelope.OK {
		t.Fatalf("prompt package upload failed: status=%d error=%#v", response.StatusCode, envelope.Error)
	}
	return envelope.Data
}
