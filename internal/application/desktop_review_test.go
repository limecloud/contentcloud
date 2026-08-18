package application_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestDesktopReviewReadModelAndDecision(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(ctx, "desktop-review@example.com", "long-enough-password", "Owner", "Desktop Review")
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, user, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "desktop-review-project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.Workspace.CreateConnectSession(ctx, user, project.ID, "desktop-review-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(ctx, service, user, connect, application.ConnectDeviceInput{Hostname: "desktop", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.Workspace.WorkspaceActor(ctx, connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	object, err := reviewdomain.NewSubmissionObjectRef("fact-1", "Fact", 1, "30-knowledge/pages/fact-1.json", map[string]any{"id": "fact-1", "kind": "fact", "status": "verified", "risk_level": "low"})
	if err != nil {
		t.Fatal(err)
	}
	bundle := reviewdomain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "knowledge", ProjectID: project.ID, WorkspaceID: binding.ID, Objects: []reviewdomain.SubmissionObjectRef{object}, LocalRunSummary: reviewdomain.LocalRunSummary{Checks: []reviewdomain.LocalRunCheck{{Name: "lint", Status: "passed"}}}, EnvironmentDigest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", IdempotencyKey: "desktop-review-v1"}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.Review.CreateSubmission(ctx, workspaceActor, binding, bundle, "desktop-review-publish")
	if err != nil {
		t.Fatal(err)
	}
	deviceActor, _, err := service.Workspace.DeviceActor(ctx, connected.DeviceToken)
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := service.Review.DesktopReviewInbox(ctx, deviceActor, project.ID)
	if err != nil || len(inbox.Items) != 1 || len(inbox.Items[0].AllowedActions) != 4 {
		t.Fatalf("unexpected desktop review inbox: %#v %v", inbox, err)
	}
	detail, err := service.Review.DesktopReviewRevision(ctx, deviceActor, project.ID, revision.ID)
	if err != nil || len(detail.Diffs) != 1 || detail.Diffs[0].Change != "added" {
		t.Fatalf("unexpected desktop revision detail: %#v %v", detail, err)
	}
	comment, err := service.Review.AddDesktopReviewComment(ctx, deviceActor, application.DesktopReviewCommentInput{ProjectID: project.ID, RevisionID: revision.ID, Body: "请确认事实范围", JSONPointer: "/0"}, "desktop-review-comment")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review.ResolveReviewComment(ctx, deviceActor, comment.ID, "desktop-review-resolve"); err != nil {
		t.Fatal(err)
	}
	approval, err := service.Review.DesktopApprove(ctx, deviceActor, application.DesktopReviewDecisionInput{ProjectID: project.ID, RevisionID: revision.ID, Reason: "已完成来源核验"}, "desktop-review-approve")
	if err != nil || approval.Decision.Decision != "approve" {
		t.Fatalf("desktop approval failed: %#v %v", approval, err)
	}
	projection, err := service.Review.DesktopProjectProjection(ctx, deviceActor, project.ID)
	if err != nil || projection.RuntimeState != "succeeded" || projection.LifecycleState != "draft" || projection.ReviewState != "approved" {
		t.Fatalf("unexpected runtime/review projection: %#v %v", projection, err)
	}
	if _, err := service.Review.DesktopReviewInbox(ctx, deviceActor, "unknown-project"); err == nil {
		t.Fatal("unknown project must be rejected")
	}
}

func TestDesktopProjectProjectionReflectsGovernedDelivery(t *testing.T) {
	ctx, service, _, actor, binding := v3ContentFixture(t)
	revision := publishV3ContentItem(t, ctx, service, binding, "desktop-delivery-item", "desktop-delivery-publish")
	if _, err := service.Review.ApproveSubmission(ctx, actor, revision.ID, "内审通过", "desktop-delivery-internal"); err != nil {
		t.Fatal(err)
	}
	grant, err := service.Review.CreateReviewGrant(ctx, actor, revision.ID, "desktop-client@example.com", "desktop-delivery-grant")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review.VerifyReviewGrant(ctx, grant.PlaintextToken, grant.PlaintextOTP); err != nil {
		t.Fatal(err)
	}
	decision, err := service.Review.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "客户批准", "", "desktop-delivery-client")
	if err != nil || decision.ApprovedSnapshot == nil {
		t.Fatalf("client approval = %#v err=%v", decision, err)
	}
	if _, err := service.Review.CreateDeliveryPackage(ctx, actor, decision.ApprovedSnapshot.ID, "desktop-delivery-item", "desktop-delivery-package"); err != nil {
		t.Fatal(err)
	}
	projection, err := service.Review.DesktopProjectProjection(ctx, actor, binding.ProjectID)
	if err != nil || projection.LifecycleState != "delivered" || projection.ReviewState != "approved" {
		t.Fatalf("governed delivery projection = %#v err=%v", projection, err)
	}
}
