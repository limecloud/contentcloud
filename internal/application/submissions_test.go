package application_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"

	"github.com/limecloud/contentcloud/internal/application"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
)

func TestWorkspaceSubmissionApprovalDoesNotStartRuntimeRun(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(ctx, "submission@example.com", "long-enough-password", "Owner", "Agency")
	if err != nil {
		t.Fatal(err)
	}
	admin, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, admin, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "req-project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.Workspace.CreateConnectSession(ctx, admin, project.ID, "req-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(ctx, service, admin, connect, application.ConnectDeviceInput{Hostname: "local", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.Workspace.WorkspaceActor(ctx, connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	if binding.CredentialHash != "" {
		t.Fatal("workspace authentication leaked the credential hash")
	}
	binding, err = service.Review.RegisterWorkspace(ctx, workspaceActor, binding, "workspace_marketing_video", "2.0.0", []string{"codex-plugin"}, "req-register")
	if err != nil || binding.CredentialHash != "" {
		t.Fatalf("workspace registration failed or leaked token hash: %#v %v", binding, err)
	}
	if len(binding.Targets) != 1 || binding.Targets[0] != "codex" {
		t.Fatalf("legacy distribution target was not normalized: %#v", binding.Targets)
	}
	if _, _, err := service.Workspace.DeviceActor(ctx, connected.DeviceToken); err != nil {
		t.Fatalf("workspace registration must not invalidate the optional Device Credential: %v", err)
	}
	bundle := reviewdomain.SubmissionBundle{
		BundleVersion: "3.0", SubmissionType: "knowledge", ProjectID: project.ID, WorkspaceID: connected.WorkspaceID, BaseSnapshotIDs: []string{}, EnvironmentDigest: submissionEnvironmentDigest,
		Objects: []reviewdomain.SubmissionObjectRef{mustSubmissionObject(t, "fact-1", "Fact", "30-knowledge/pages/facts/fact-1.json", map[string]any{"id": "fact-1", "kind": "fact", "status": "verified", "risk_level": "low"})}, SourceDisclosures: []reviewdomain.SourceDisclosure{}, Artifacts: []reviewdomain.SubmissionArtifact{}, LocalRunSummary: reviewdomain.LocalRunSummary{Checks: []reviewdomain.LocalRunCheck{{Name: "knowledge-lint", Status: "passed"}}}, IdempotencyKey: "knowledge-v1",
	}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.Review.CreateSubmission(ctx, workspaceActor, binding, bundle, "req-publish")
	if err != nil {
		t.Fatal(err)
	}
	if revision.Artifacts == nil {
		t.Fatal("empty artifact manifest must remain a JSON array")
	}
	replayed, err := service.Review.CreateSubmission(ctx, workspaceActor, binding, bundle, "req-publish-retry")
	if err != nil || replayed.ID != revision.ID {
		t.Fatalf("idempotent retry failed: %#v %v", replayed, err)
	}
	approval, err := service.Review.ApproveSubmission(ctx, admin, revision.ID, "facts and citations reviewed", "req-approve")
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
	bundle.Objects = []reviewdomain.SubmissionObjectRef{mustSubmissionObject(t, "fact-2", "Fact", "30-knowledge/pages/facts/fact-2.json", map[string]any{"id": "fact-2", "kind": "fact", "status": "verified", "risk_level": "low"})}
	bundle.IdempotencyKey = "knowledge-v2"
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	secondRevision, err := service.Review.CreateSubmission(ctx, workspaceActor, binding, bundle, "req-publish-v2")
	if err != nil {
		t.Fatal(err)
	}
	returned, err := service.Review.RequestSubmissionChanges(ctx, admin, secondRevision.ID, "补充事实生效范围", "/0/scope", "req-changes")
	if err != nil || returned.Status != "changes_requested" {
		t.Fatalf("request changes failed: %#v %v", returned, err)
	}
	feedback, err := service.Review.WorkspaceFeedback(ctx, workspaceActor, binding)
	if err != nil || len(feedback) != 1 || feedback[0].Comments[0].JSONPointer != "/0/scope" {
		t.Fatalf("workspace feedback missing: %#v %v", feedback, err)
	}
	delta, err := service.Review.WorkspaceDecisions(ctx, workspaceActor, binding)
	if err != nil || len(delta.Decisions) != 2 {
		t.Fatalf("workspace decision delta missing approval/change request: %#v %v", delta, err)
	}
	runs, err := service.Runtime.Runs(ctx, admin, project.ID)
	if err != nil || len(runs) != 0 {
		t.Fatalf("ordinary local publish must not start RuntimeRun: %#v %v", runs, err)
	}
}

func TestEvidenceLimitedSubmissionCannotBeRemotelyApproved(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, _ := service.Identity.Register(ctx, "risk@example.com", "long-enough-password", "Owner", "Agency")
	admin, _, _ := service.Identity.SessionActor(ctx, session.ID)
	project, _ := service.Workspace.CreateProject(ctx, admin, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	connect, _ := service.Workspace.CreateConnectSession(ctx, admin, project.ID, "")
	connected, _ := testsupport.ConnectBootstrap(ctx, service, admin, connect, application.ConnectDeviceInput{Hostname: "local"})
	workspaceActor, binding, _ := service.Workspace.WorkspaceActor(ctx, connected.WorkspaceToken)
	bundle := reviewdomain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "knowledge", ProjectID: project.ID, WorkspaceID: binding.ID, BaseSnapshotIDs: []string{}, EnvironmentDigest: submissionEnvironmentDigest, Objects: []reviewdomain.SubmissionObjectRef{mustSubmissionObject(t, "claim-1", "Claim", "30-knowledge/pages/claims/claim-1.json", map[string]any{"id": "claim-1", "kind": "claim", "status": "approved", "risk_level": "high"})}, SourceDisclosures: []reviewdomain.SourceDisclosure{{SourceRef: "source-1", Level: "metadata_only", SHA256: strings.Repeat("a", 64)}}, Artifacts: []reviewdomain.SubmissionArtifact{}, LocalRunSummary: reviewdomain.LocalRunSummary{Checks: []reviewdomain.LocalRunCheck{}}, IdempotencyKey: "risk-v1"}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.Review.CreateSubmission(ctx, workspaceActor, binding, bundle, "")
	if err != nil || !revision.EvidenceLimited {
		t.Fatalf("expected evidence-limited revision: %#v %v", revision, err)
	}
	_, err = service.Review.ApproveSubmission(ctx, admin, revision.ID, "approve", "")
	assertDomainCode(t, err, "EVIDENCE_LEVEL_INSUFFICIENT")
}

const submissionEnvironmentDigest = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

func mustSubmissionObject(t *testing.T, id, objectType, path string, content any) reviewdomain.SubmissionObjectRef {
	t.Helper()
	value, err := reviewdomain.NewSubmissionObjectRef(id, objectType, 1, path, content)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
