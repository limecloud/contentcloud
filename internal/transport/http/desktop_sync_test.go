package httpapi_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"github.com/limecloud/contentcloud/internal/testsupport"
	apiclient "github.com/limecloud/contentcloud/internal/transport/client"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestDesktopWorkspaceRevisionDispatchUsesDeviceCASAndIdempotency(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(t.Context(), "desktop-sync@example.com", "long-enough-password", "Owner", "Desktop Sync")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "desktop-project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.Workspace.CreateConnectSession(t.Context(), actor, project.ID, "desktop-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, application.ConnectDeviceInput{Hostname: "desktop", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	t.Cleanup(server.Close)
	client := apiclient.New(server.URL, connected.DeviceToken)
	firstFile := uploadDesktopWorkspaceFile(t, client, connected.WorkspaceID, project.ID, "40-work/draft.md", []byte("first revision"), "upload-1")
	firstFiles := []workspacedomain.WorkspaceRevisionFile{firstFile}
	firstInput := application.PublishWorkspaceRevisionInput{WorkspaceID: connected.WorkspaceID, ProjectID: project.ID, BaseRevision: "0", ContentDigest: workspacedomain.WorkspaceContentDigest(firstFiles), Files: firstFiles, ClientMutationID: "mutation-1", IdempotencyKey: "idem-1"}
	var first workspacedomain.WorkspaceRevision
	if err := client.Dispatch(t.Context(), "desktop.workspace.publish", firstInput, &first); err != nil {
		t.Fatal(err)
	}
	var replay workspacedomain.WorkspaceRevision
	if err := client.Dispatch(t.Context(), "desktop.workspace.publish", firstInput, &replay); err != nil || replay.ID != first.ID || replay.RevisionNo != 1 {
		t.Fatalf("idempotent replay = %#v err=%v", replay, err)
	}
	secondInput := firstInput
	secondFile := uploadDesktopWorkspaceFile(t, client, connected.WorkspaceID, project.ID, "40-work/draft.md", []byte("second revision"), "upload-2")
	secondInput.Files = []workspacedomain.WorkspaceRevisionFile{secondFile}
	secondInput.BaseRevision, secondInput.ContentDigest, secondInput.ClientMutationID, secondInput.IdempotencyKey = first.ID, workspacedomain.WorkspaceContentDigest(secondInput.Files), "mutation-2", "idem-2"
	var second workspacedomain.WorkspaceRevision
	if err := client.Dispatch(t.Context(), "desktop.workspace.publish", secondInput, &second); err != nil || second.RevisionNo != 2 {
		t.Fatalf("second revision = %#v err=%v", second, err)
	}
	stale := secondInput
	staleFile := uploadDesktopWorkspaceFile(t, client, connected.WorkspaceID, project.ID, "40-work/draft.md", []byte("stale revision"), "upload-3")
	stale.Files = []workspacedomain.WorkspaceRevisionFile{staleFile}
	stale.ContentDigest, stale.ClientMutationID, stale.IdempotencyKey = workspacedomain.WorkspaceContentDigest(stale.Files), "mutation-3", "idem-3"
	err = client.Dispatch(t.Context(), "desktop.workspace.publish", stale, &workspacedomain.WorkspaceRevision{})
	var domainErr *fault.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "WORKSPACE_REVISION_STALE" {
		t.Fatalf("expected stale conflict, got %v", err)
	}
	var latest workspacedomain.WorkspaceRevision
	if err := client.Dispatch(t.Context(), "desktop.workspace.latest", map[string]string{"workspace_id": connected.WorkspaceID, "project_id": project.ID}, &latest); err != nil || latest.ID != second.ID {
		t.Fatalf("latest revision = %#v err=%v", latest, err)
	}
	var events application.WorkspaceRevisionEvents
	if err := client.Dispatch(t.Context(), "desktop.workspace.events", map[string]any{"workspace_id": connected.WorkspaceID, "project_id": project.ID, "after": 0, "limit": 100}, &events); err != nil || events.Gap || len(events.Events) != 2 || events.NextCursor != 2 {
		t.Fatalf("workspace revision events = %#v err=%v", events, err)
	}
}

func TestDesktopWorkspaceUploadResumesAndRejectsDigestMismatch(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(t.Context(), "desktop-upload@example.com", "long-enough-password", "Owner", "Desktop Upload")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, _ := service.Identity.SessionActor(t.Context(), session.ID)
	project, _ := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "desktop-upload-project")
	connect, _ := service.Workspace.CreateConnectSession(t.Context(), actor, project.ID, "desktop-upload-connect")
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, application.ConnectDeviceInput{Hostname: "desktop", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	t.Cleanup(server.Close)
	client := apiclient.New(server.URL, connected.DeviceToken)
	data := make([]byte, workspacedomain.WorkspaceUploadChunkSize+3)
	for index := range data {
		data[index] = byte(index % 251)
	}
	fileDigest := sha256Digest(data)
	startInput := application.StartWorkspaceUploadInput{WorkspaceID: connected.WorkspaceID, ProjectID: project.ID, Ref: "50-production/video.bin", ContentDigest: fileDigest, ByteSize: int64(len(data)), IdempotencyKey: "resumable-upload"}
	var started application.WorkspaceUploadStartResult
	if err := client.Dispatch(t.Context(), "desktop.upload.start", startInput, &started); err != nil || started.Session.PartCount != 2 {
		t.Fatalf("start = %#v err=%v", started, err)
	}
	firstPart := data[:workspacedomain.WorkspaceUploadChunkSize]
	if err := client.Dispatch(t.Context(), "desktop.upload.part", application.UploadWorkspacePartInput{SessionID: started.Session.ID, PartNo: 0, Digest: sha256Digest(firstPart), Data: firstPart}, &application.WorkspaceUploadPartResult{}); err != nil {
		t.Fatal(err)
	}
	var resumed application.WorkspaceUploadStartResult
	if err := client.Dispatch(t.Context(), "desktop.upload.start", startInput, &resumed); err != nil || len(resumed.ConfirmedParts) != 1 || resumed.ConfirmedParts[0] != 0 {
		t.Fatalf("resume = %#v err=%v", resumed, err)
	}
	lastPart := data[workspacedomain.WorkspaceUploadChunkSize:]
	err = client.Dispatch(t.Context(), "desktop.upload.part", application.UploadWorkspacePartInput{SessionID: started.Session.ID, PartNo: 1, Digest: "sha256:" + repeatedDigest("0"), Data: lastPart}, &application.WorkspaceUploadPartResult{})
	var domainErr *fault.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "WORKSPACE_UPLOAD_PART_DIGEST_MISMATCH" {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
	if err := client.Dispatch(t.Context(), "desktop.upload.part", application.UploadWorkspacePartInput{SessionID: started.Session.ID, PartNo: 1, Digest: sha256Digest(lastPart), Data: lastPart}, &application.WorkspaceUploadPartResult{}); err != nil {
		t.Fatal(err)
	}
	var object workspacedomain.WorkspaceObject
	if err := client.Dispatch(t.Context(), "desktop.upload.finalize", application.FinalizeWorkspaceUploadInput{SessionID: started.Session.ID}, &object); err != nil || object.ContentDigest != fileDigest || object.ByteSize != int64(len(data)) {
		t.Fatalf("finalize = %#v err=%v", object, err)
	}
	missingFile := workspacedomain.WorkspaceRevisionFile{Ref: "40-work/missing.md", Digest: "sha256:" + repeatedDigest("f"), ByteSize: 1}
	err = client.Dispatch(t.Context(), "desktop.workspace.publish", application.PublishWorkspaceRevisionInput{WorkspaceID: connected.WorkspaceID, ProjectID: project.ID, BaseRevision: "0", Files: []workspacedomain.WorkspaceRevisionFile{missingFile}, ContentDigest: workspacedomain.WorkspaceContentDigest([]workspacedomain.WorkspaceRevisionFile{missingFile}), ClientMutationID: "missing", IdempotencyKey: "missing"}, &workspacedomain.WorkspaceRevision{})
	if !errors.As(err, &domainErr) || domainErr.Code != "WORKSPACE_FILE_OBJECT_MISSING" {
		t.Fatalf("expected missing object conflict, got %v", err)
	}
}

func TestDesktopReviewDispatchEnforcesCurrentRevisionAndDeviceProjectScope(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(t.Context(), "desktop-review-http@example.com", "long-enough-password", "Owner", "Desktop Review HTTP")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "desktop-review-http-project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.Workspace.CreateConnectSession(t.Context(), actor, project.ID, "desktop-review-http-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, application.ConnectDeviceInput{Hostname: "desktop-review", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.Workspace.WorkspaceActor(t.Context(), connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	object, err := reviewdomain.NewSubmissionObjectRef("fact-http-1", "Fact", 1, "30-knowledge/pages/fact-http-1.json", map[string]any{"id": "fact-http-1", "kind": "fact", "status": "verified", "risk_level": "low"})
	if err != nil {
		t.Fatal(err)
	}
	bundle := reviewdomain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "knowledge", ProjectID: project.ID, WorkspaceID: binding.ID, Objects: []reviewdomain.SubmissionObjectRef{object}, LocalRunSummary: reviewdomain.LocalRunSummary{Checks: []reviewdomain.LocalRunCheck{{Name: "lint", Status: "passed"}}}, EnvironmentDigest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", IdempotencyKey: "desktop-review-http-v1"}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	first, err := service.Review.CreateSubmission(t.Context(), workspaceActor, binding, bundle, "desktop-review-http-publish")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	t.Cleanup(server.Close)
	client := apiclient.New(server.URL, connected.DeviceToken)

	var inbox application.DesktopReviewInbox
	if err := client.Dispatch(t.Context(), "desktop.review.inbox", map[string]any{"project_id": project.ID}, &inbox); err != nil || len(inbox.Items) != 1 || inbox.Items[0].Revision.ID != first.ID {
		t.Fatalf("review inbox = %#v err=%v", inbox, err)
	}
	var detail application.DesktopReviewRevisionDetail
	if err := client.Dispatch(t.Context(), "desktop.review.show", map[string]any{"project_id": project.ID, "revision_id": first.ID}, &detail); err != nil || detail.Revision.ID != first.ID || len(detail.Diffs) != 1 {
		t.Fatalf("review detail = %#v err=%v", detail, err)
	}
	var comment reviewdomain.ReviewComment
	if err := client.Dispatch(t.Context(), "desktop.review.comment", application.DesktopReviewCommentInput{ProjectID: project.ID, RevisionID: first.ID, Body: "请确认来源", JSONPointer: "/objects/0"}, &comment); err != nil || comment.ID == "" {
		t.Fatalf("review comment = %#v err=%v", comment, err)
	}
	if _, err := service.Review.ResolveReviewComment(t.Context(), actor, comment.ID, "desktop-review-http-resolve"); err != nil {
		t.Fatal(err)
	}
	var approval application.SubmissionApprovalResult
	if err := client.Dispatch(t.Context(), "desktop.review.approve", application.DesktopReviewDecisionInput{ProjectID: project.ID, RevisionID: first.ID, Reason: "已核验"}, &approval); err != nil || approval.Decision.Decision != "approve" {
		t.Fatalf("review approval = %#v err=%v", approval, err)
	}

	secondBundle := bundle
	secondBundle.IdempotencyKey = "desktop-review-http-v2"
	secondObject, err := reviewdomain.NewSubmissionObjectRef("fact-http-1", "Fact", 2, "30-knowledge/pages/fact-http-1.json", map[string]any{"id": "fact-http-1", "kind": "fact", "status": "verified", "risk_level": "low", "value": "updated"})
	if err != nil {
		t.Fatal(err)
	}
	secondBundle.Objects = []reviewdomain.SubmissionObjectRef{secondObject}
	if err := secondBundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	second, err := service.Review.CreateSubmission(t.Context(), workspaceActor, binding, secondBundle, "desktop-review-http-publish-2")
	if err != nil {
		t.Fatal(err)
	}
	var rejected reviewdomain.Submission
	if err := client.Dispatch(t.Context(), "desktop.review.reject", application.DesktopReviewDecisionInput{ProjectID: project.ID, RevisionID: second.ID, Reason: "证据不完整"}, &rejected); err != nil || rejected.Status != "rejected" {
		t.Fatalf("review rejection = %#v err=%v", rejected, err)
	}
	thirdBundle := secondBundle
	thirdBundle.IdempotencyKey = "desktop-review-http-v3"
	thirdObject, err := reviewdomain.NewSubmissionObjectRef("fact-http-1", "Fact", 3, "30-knowledge/pages/fact-http-1.json", map[string]any{"id": "fact-http-1", "kind": "fact", "status": "verified", "risk_level": "low", "value": "third"})
	if err != nil {
		t.Fatal(err)
	}
	thirdBundle.Objects = []reviewdomain.SubmissionObjectRef{thirdObject}
	if err := thirdBundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	third, err := service.Review.CreateSubmission(t.Context(), workspaceActor, binding, thirdBundle, "desktop-review-http-publish-3")
	if err != nil {
		t.Fatal(err)
	}
	var changesRequested reviewdomain.Submission
	if err := client.Dispatch(t.Context(), "desktop.review.request_changes", application.DesktopReviewDecisionInput{ProjectID: project.ID, RevisionID: third.ID, Reason: "补充适用范围", JSONPointer: "/objects/0"}, &changesRequested); err != nil || changesRequested.Status != "changes_requested" {
		t.Fatalf("review changes request = %#v err=%v", changesRequested, err)
	}
	var staleErr *fault.Error
	err = client.Dispatch(t.Context(), "desktop.review.comment", application.DesktopReviewCommentInput{ProjectID: project.ID, RevisionID: first.ID, Body: "旧版本"}, &reviewdomain.ReviewComment{})
	if !errors.As(err, &staleErr) || staleErr.Code != "SUBMISSION_REVISION_STALE" {
		t.Fatalf("stale review command error = %v", err)
	}

	secondProject, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Brand 2", ProductName: "Product 2"}, "desktop-review-http-project-2")
	if err != nil {
		t.Fatal(err)
	}
	secondConnect, err := service.Workspace.CreateConnectSession(t.Context(), actor, secondProject.ID, "desktop-review-http-connect-2")
	if err != nil {
		t.Fatal(err)
	}
	secondDevice, err := testsupport.ConnectBootstrap(t.Context(), service, actor, secondConnect, application.ConnectDeviceInput{Hostname: "desktop-review-2", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	permissionClient := apiclient.New(server.URL, secondDevice.DeviceToken)
	err = permissionClient.Dispatch(t.Context(), "desktop.review.inbox", map[string]any{"project_id": project.ID}, &application.DesktopReviewInbox{})
	if !errors.As(err, &staleErr) || staleErr.Code != "DEVICE_PROJECT_ACCESS_DENIED" {
		t.Fatalf("cross-project review error = %v", err)
	}
}

func uploadDesktopWorkspaceFile(t *testing.T, client *apiclient.Client, workspaceID, projectID, ref string, data []byte, idempotencyKey string) workspacedomain.WorkspaceRevisionFile {
	t.Helper()
	digest := sha256Digest(data)
	var started application.WorkspaceUploadStartResult
	if err := client.Dispatch(t.Context(), "desktop.upload.start", application.StartWorkspaceUploadInput{WorkspaceID: workspaceID, ProjectID: projectID, Ref: ref, ContentDigest: digest, ByteSize: int64(len(data)), IdempotencyKey: idempotencyKey}, &started); err != nil {
		t.Fatal(err)
	}
	for partNo, offset := 0, 0; offset < len(data); partNo, offset = partNo+1, offset+workspacedomain.WorkspaceUploadChunkSize {
		end := min(offset+workspacedomain.WorkspaceUploadChunkSize, len(data))
		part := data[offset:end]
		if err := client.Dispatch(t.Context(), "desktop.upload.part", application.UploadWorkspacePartInput{SessionID: started.Session.ID, PartNo: partNo, Digest: sha256Digest(part), Data: part}, &application.WorkspaceUploadPartResult{}); err != nil {
			t.Fatal(err)
		}
	}
	var object workspacedomain.WorkspaceObject
	if err := client.Dispatch(t.Context(), "desktop.upload.finalize", application.FinalizeWorkspaceUploadInput{SessionID: started.Session.ID}, &object); err != nil {
		t.Fatal(err)
	}
	return workspacedomain.WorkspaceRevisionFile{Ref: ref, Digest: digest, ByteSize: int64(len(data))}
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func repeatedDigest(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
