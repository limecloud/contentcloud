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

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/localworkspace"
	"github.com/limecloud/contentcloud/internal/mediapipeline"
	"github.com/limecloud/contentcloud/internal/store/memory"
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
	job := domain.MediaGenerationJob{
		ID: fixture.jobID, TenantID: fixture.tenantID, ProjectID: fixture.projectID, TaskID: fixture.taskID,
		StageRunID: domain.NewID(), StoryboardSnapshotID: fixture.snapshotID, ProviderID: "modelark-seedance25", ProfileVersion: "1.0.0",
		ProfileDigest: "sha256:" + strings.Repeat("d", 64), Model: "dreamina-seedance-2-5-260628", Mode: "image_to_video", AspectRatio: "9:16",
		DurationSeconds: 5, State: domain.MediaJobAwaitingExternal, IdempotencyKey: "reconcile-http-job", Currency: "CNY", MaxAttempts: 3,
		RowVersion: 1, CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now,
	}
	if err := fixture.store.CreateMediaGenerationJob(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	attempt := domain.ProviderAttempt{ID: domain.NewID(), TenantID: fixture.tenantID, ProjectID: fixture.projectID, GenerationJobID: job.ID, AttemptNumber: 1, ProviderID: job.ProviderID, RequestDigest: "sha256:" + strings.Repeat("e", 64), ProviderState: "unknown", EstimatedCostMinor: 1, Currency: "CNY", CreatedAt: now, UpdatedAt: now}
	if err := fixture.store.CreateProviderAttempt(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(fixture.service, slog.Default(), false, "").Handler())
	defer server.Close()
	client := fixture.client(server.URL)
	value := callBFF[domain.MediaGenerationJob](t, client, http.MethodPost, server.URL+"/api/bff/media-jobs/"+job.ID+"/reconcile-submit", app.MediaJobSubmitReconciliationInput{ExpectedVersion: 1, ExternalJobID: "ark-task-123"})
	if value.RowVersion != 2 || value.State != domain.MediaJobAwaitingExternal {
		t.Fatalf("submit reconciliation changed unexpected job state: %#v", value)
	}
	attempts, err := fixture.store.ProviderAttempts(t.Context(), fixture.tenantID, job.ID)
	if err != nil || len(attempts) != 1 || attempts[0].ExternalJobID != "ark-task-123" || attempts[0].ProviderState != "reconciliation_pending" {
		t.Fatalf("submit reconciliation did not bind external task ID: attempts=%#v err=%v", attempts, err)
	}
}

type seedanceHTTPFixture struct {
	service    *app.Service
	store      *memory.Store
	actor      app.Actor
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
	service := app.New(store, slog.Default())
	session, err := service.Register(ctx, "seedance-http@example.com", "long-enough-password", "视频负责人", "视频团队")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "测试品牌", ProductName: "测试产品", Channel: "douyin", ContentType: domain.ContentTypeMarketingVideo}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetTenantContentCapability(ctx, domain.TenantContentCapability{TenantID: actor.TenantID, ContentType: domain.ContentTypeMarketingVideo, Enabled: true, UpdatedBy: actor.UserID, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateWorkTask(ctx, actor, app.CreateWorkTaskInput{ProjectID: project.ID, Title: "Seedance HTTP 测试", ContentType: domain.ContentTypeMarketingVideo, InputRefs: []string{"brief:test"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	// The endpoint is stage-scoped; the fixture starts after storyboard approval
	// so the test can focus on the HTTP contract rather than replaying all stages.
	now := time.Now().UTC()
	if err := store.CreateWorkspaceBinding(ctx, domain.WorkspaceBinding{ID: task.Task.ID, TenantID: actor.TenantID, ProjectID: project.ID, OwnerUserID: actor.UserID, TemplateID: localworkspace.TemplateID, TemplateVersion: localworkspace.TemplateVersion, Targets: []string{"web"}, CredentialHash: "seedance-http-credential-" + task.Task.ID, Status: "active", InitializedAt: now, LastSeenAt: now}); err != nil {
		t.Fatal(err)
	}
	task.Task.CurrentStageID = "storyboard"
	task.Task.Status = domain.TaskStatusRunning
	if err := store.SaveWorkTask(ctx, task.Task); err != nil {
		t.Fatal(err)
	}

	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	assetHash := mediapipeline.SHA256(png)
	storyboard := domain.StoryboardPackage{
		ID: "storyboard:" + task.Task.ID, Type: "storyboard_package", SchemaVersion: domain.StoryboardPackageSchema, ProjectID: project.ID,
		ApprovedSnapshotID: "snapshot-pending", ContentItemID: "content-item:" + task.Task.ID,
		GeneratorCapability: domain.CapabilityRef{ID: "contentcloud.storyboard.generator", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)},
		Status:              "review_ready", ReviewSheetArtifactID: "asset-review-sheet", SourceDigest: "sha256:" + strings.Repeat("b", 64),
		RightsRefs: []string{"rights:test"},
		Shots:      []domain.StoryboardShot{{ShotID: "shot-1", StartMS: 0, EndMS: 4000, Role: "hero", FirstFrameArtifactID: "asset-first-frame", ImagePromptZH: "测试产品首帧", PlanB: "保持主体构图", NegativeConstraints: []string{"无文字"}, AcceptanceCriteria: []string{"首帧稳定"}}},
		Assets:     []domain.StoryboardAsset{{ID: "asset-first-frame", Role: "first_frame", ShotID: "shot-1", Path: "first-frame.png", MediaType: "image/png", SHA256: assetHash, ByteSize: int64(len(png)), RightsRefs: []string{"rights:test"}}, {ID: "asset-review-sheet", Role: "review_sheet", Path: "review-sheet.png", MediaType: "image/png", SHA256: assetHash, ByteSize: int64(len(png)), RightsRefs: []string{"rights:test"}}},
	}
	storyboard.LockedDigest, err = storyboard.ComputedLockedDigest()
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.WorkspaceBinding(ctx, actor.TenantID, task.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	object, err := domain.NewSubmissionObjectRef(storyboard.ID, "storyboard_package", 1, "storyboard/"+storyboard.ID+".json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	now = time.Now().UTC()
	submissionID, revisionID := domain.NewID(), domain.NewID()
	submission := domain.Submission{ID: submissionID, TenantID: actor.TenantID, ProjectID: project.ID, WorkspaceID: workspace.ID, SubmissionType: "storyboard", Status: "submitted", CurrentRevisionID: revisionID, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	revision := domain.SubmissionRevision{ID: revisionID, TenantID: actor.TenantID, ProjectID: project.ID, WorkspaceID: workspace.ID, SubmissionID: submissionID, RevisionNo: 1, SchemaVersion: domain.SubmissionSchemaVersion("storyboard"), ContentHash: object.Digest, EnvironmentDigest: task.Task.SOPDigest, Objects: []domain.SubmissionObjectRef{object}, IdempotencyKey: "seedance-http-submission", CreatedBy: actor.UserID, CreatedAt: now}
	if err := store.CreateSubmissionRevision(ctx, submission, revision, nil, domain.ReviewCycle{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, SubjectType: "submission_revision", SubjectID: revisionID, Status: "open", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	snapshotID := domain.NewID()
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
	object, err = domain.NewSubmissionObjectRef(storyboard.ID, "storyboard_package", 1, "storyboard/"+storyboard.ID+".json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err = json.Marshal(map[string]any{"objects": []json.RawMessage{object.Content}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := domain.ApprovedSnapshot{ID: snapshotID, TenantID: actor.TenantID, ProjectID: project.ID, WorkspaceID: workspace.ID, SubmissionID: submissionID, SubmissionRevisionID: revisionID, SubmissionType: "storyboard", SchemaVersion: domain.SubmissionSchemaVersion("storyboard"), ContentHash: object.Digest, SubjectHash: object.Digest, CanonicalContent: canonical, EligibleIDs: []string{storyboard.ID}, DecisionID: domain.NewID(), CreatedBy: actor.UserID, CreatedAt: now}
	decision := domain.ApprovalDecision{ID: snapshot.DecisionID, TenantID: actor.TenantID, ProjectID: project.ID, SubjectType: "submission_revision", SubjectID: revisionID, SubjectHash: object.Digest, DecisionStage: "internal", ActorID: actor.UserID, Decision: "approve", PreviousState: "submitted", ResultingState: "approved", CreatedAt: now}
	// The package must reference the final snapshot ID and locked digest.
	storyboardSnapshotBody := canonical
	var snapshotEnvelope struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(storyboardSnapshotBody, &snapshotEnvelope); err != nil || len(snapshotEnvelope.Objects) != 1 {
		t.Fatal("storyboard snapshot object missing")
	}
	var locked domain.StoryboardPackage
	if err := json.Unmarshal(snapshotEnvelope.Objects[0], &locked); err != nil {
		t.Fatal(err)
	}
	prompt := domain.SeedancePromptPackage{ID: "prompt-package:" + task.Task.ID, Type: "seedance_prompt_package", SchemaVersion: domain.SeedancePromptPackageSchema, StoryboardSnapshotID: snapshotID, StoryboardPackageID: locked.ID, StoryboardLockedDigest: locked.LockedDigest, Provider: "seedance", ProviderProfileVersion: "1.0.0", AdapterCapability: domain.CapabilityRef{ID: "contentcloud.seedance-execution", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("c", 64)}, Mode: "all_reference", Settings: domain.SeedanceSettings{AspectRatio: "9:16", DurationSeconds: 5, Sound: "environment_only"}, UploadManifest: []domain.SeedanceUpload{{Reference: "@图片1", ArtifactID: "asset-first-frame", File: "first-frame.png", Purpose: "first_frame", SHA256: assetHash}}, Segments: []domain.SeedanceSegment{{ID: "segment-1", Order: 1, StartMS: 0, EndMS: 4000, PromptZH: "@图片1 镜头向前推进", AcceptanceCriteria: []string{"首帧稳定"}}}, Validation: domain.SeedanceValidation{ReferencesChecked: true, LimitsChecked: true, RightsChecked: true, OfferChecked: true, DigestChecked: true}, Status: "validated"}
	promptBody, err := json.Marshal(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApproveSubmissionRevision(ctx, submission, snapshot, decision); err != nil {
		t.Fatal(err)
	}
	return seedanceHTTPFixture{service: service, store: store, actor: actor, userID: actor.UserID, tenantID: actor.TenantID, projectID: project.ID, taskID: task.Task.ID, snapshotID: snapshotID, jobID: domain.NewID(), promptBody: promptBody, sessionID: session.ID}
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

func postSeedancePromptPackage(t *testing.T, client *http.Client, target, snapshotID string, body []byte) domain.Artifact {
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
		OK    bool            `json:"ok"`
		Data  domain.Artifact `json:"data"`
		Error *domain.Error   `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !envelope.OK {
		t.Fatalf("prompt package upload failed: status=%d error=%#v", response.StatusCode, envelope.Error)
	}
	return envelope.Data
}
