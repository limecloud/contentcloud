package app_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestWorkspaceSubmissionApprovalCreatesImmutableSnapshotWithoutTaskRun(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "submission@example.com", "long-enough-password", "Owner", "Agency")
	if err != nil {
		t.Fatal(err)
	}
	admin, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, admin, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "req-project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.CreateConnectSession(ctx, admin, project.ID, "req-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(ctx, service, admin, connect, app.ConnectDeviceInput{Hostname: "local", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.WorkspaceActor(ctx, connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	if binding.CredentialHash != "" {
		t.Fatal("workspace authentication leaked the credential hash")
	}
	binding, err = service.RegisterWorkspace(ctx, workspaceActor, binding, "workspace_marketing_video", "2.0.0", []string{"codex"}, "req-register")
	if err != nil || binding.CredentialHash != "" {
		t.Fatalf("workspace registration failed or leaked token hash: %#v %v", binding, err)
	}
	if _, _, err := service.DeviceActor(ctx, connected.DeviceToken); err != nil {
		t.Fatalf("workspace registration must not invalidate the optional Device Credential: %v", err)
	}
	bundle := domain.SubmissionBundle{
		BundleVersion: "1.0", SchemaVersion: "contentcloud.knowledge/2.0", SubmissionType: "knowledge", ProjectID: project.ID, WorkspaceID: connected.WorkspaceID,
		Objects: json.RawMessage(`[{"id":"fact-1","kind":"fact","status":"verified","risk_level":"low"}]`), SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{{Name: "knowledge-lint", Status: "passed"}}}, IdempotencyKey: "knowledge-v1",
	}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.CreateSubmission(ctx, workspaceActor, binding, bundle, "req-publish")
	if err != nil {
		t.Fatal(err)
	}
	if revision.Artifacts == nil {
		t.Fatal("empty artifact manifest must remain a JSON array")
	}
	replayed, err := service.CreateSubmission(ctx, workspaceActor, binding, bundle, "req-publish-retry")
	if err != nil || replayed.ID != revision.ID {
		t.Fatalf("idempotent retry failed: %#v %v", replayed, err)
	}
	approval, err := service.ApproveSubmission(ctx, admin, revision.ID, "facts and citations reviewed", "req-approve")
	if err != nil {
		t.Fatal(err)
	}
	if approval.ApprovedSnapshot == nil {
		t.Fatal("knowledge approval must create an ApprovedSnapshot")
	}
	snapshot := *approval.ApprovedSnapshot
	if snapshot.SubmissionRevisionID != revision.ID || snapshot.ContentHash != revision.ContentHash || len(snapshot.EligibleIDs) != 1 || snapshot.EligibleIDs[0] != "fact-1" {
		t.Fatalf("unexpected approved snapshot: %#v", snapshot)
	}
	bundle.Objects = json.RawMessage(`[{"id":"fact-2","kind":"fact","status":"verified","risk_level":"low"}]`)
	bundle.IdempotencyKey = "knowledge-v2"
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	secondRevision, err := service.CreateSubmission(ctx, workspaceActor, binding, bundle, "req-publish-v2")
	if err != nil {
		t.Fatal(err)
	}
	returned, err := service.RequestSubmissionChanges(ctx, admin, secondRevision.ID, "补充事实生效范围", "/0/scope", "req-changes")
	if err != nil || returned.Status != "changes_requested" {
		t.Fatalf("request changes failed: %#v %v", returned, err)
	}
	feedback, err := service.WorkspaceFeedback(ctx, workspaceActor, binding)
	if err != nil || len(feedback) != 1 || feedback[0].Comments[0].JSONPointer != "/0/scope" {
		t.Fatalf("workspace feedback missing: %#v %v", feedback, err)
	}
	delta, err := service.WorkspaceDecisions(ctx, workspaceActor, binding)
	if err != nil || len(delta.Decisions) != 2 {
		t.Fatalf("workspace decision delta missing approval/change request: %#v %v", delta, err)
	}
	runs, err := service.Runs(ctx, admin, project.ID)
	if err != nil || len(runs) != 0 {
		t.Fatalf("ordinary local publish must not create TaskRun: %#v %v", runs, err)
	}
}

func TestEvidenceLimitedSubmissionCannotBeRemotelyApproved(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, _ := service.Register(ctx, "risk@example.com", "long-enough-password", "Owner", "Agency")
	admin, _, _ := service.SessionActor(ctx, session.ID)
	project, _ := service.CreateProject(ctx, admin, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	connect, _ := service.CreateConnectSession(ctx, admin, project.ID, "")
	connected, _ := testsupport.ConnectBootstrap(ctx, service, admin, connect, app.ConnectDeviceInput{Hostname: "local"})
	workspaceActor, binding, _ := service.WorkspaceActor(ctx, connected.WorkspaceToken)
	bundle := domain.SubmissionBundle{BundleVersion: "1.0", SchemaVersion: "contentcloud.knowledge/2.0", SubmissionType: "knowledge", ProjectID: project.ID, WorkspaceID: binding.ID, Objects: json.RawMessage(`[{"id":"claim-1","kind":"claim","status":"approved","risk_level":"high"}]`), SourceDisclosures: []domain.SourceDisclosure{{SourceRef: "source-1", Level: "metadata_only", SHA256: strings.Repeat("a", 64)}}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{}}, IdempotencyKey: "risk-v1"}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.CreateSubmission(ctx, workspaceActor, binding, bundle, "")
	if err != nil || !revision.EvidenceLimited {
		t.Fatalf("expected evidence-limited revision: %#v %v", revision, err)
	}
	_, err = service.ApproveSubmission(ctx, admin, revision.ID, "approve", "")
	assertDomainCode(t, err, "EVIDENCE_LEVEL_INSUFFICIENT")
}
