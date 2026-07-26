package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestV2ScriptApprovalDeliveryAndLearningChain(t *testing.T) {
	ctx, service, store, actor, binding := v2ScriptFixture(t)
	revision := publishV2Script(t, ctx, service, binding, "script-v2", "publish-v1")

	if _, err := service.CreateReviewGrant(ctx, actor, revision.ID, "client@example.com", "grant-before-internal"); err == nil {
		t.Fatal("customer review must not start before internal approval")
	}
	internal, err := service.ApproveSubmission(ctx, actor, revision.ID, "internal review passed", "internal-approve")
	if err != nil {
		t.Fatal(err)
	}
	if internal.ApprovedSnapshot != nil || internal.Submission.Status != "internally_approved" || internal.Decision.DecisionStage != "internal" {
		t.Fatalf("script internal approval created a premature snapshot: %#v", internal)
	}

	grant, err := service.CreateReviewGrant(ctx, actor, revision.ID, "client@example.com", "grant")
	if err != nil {
		t.Fatal(err)
	}
	if grant.SubjectType != "submission_revision" || grant.SubjectID != revision.ID || grant.SubjectHash != revision.ContentHash {
		t.Fatalf("grant is not bound to the immutable revision: %#v", grant)
	}
	status, err := service.SubmissionReviewStatus(ctx, actor, revision.ID)
	if err != nil || status.Submission.Status != "client_review" || status.Revision.ID != revision.ID || len(status.Grants) != 1 || status.Grants[0].ID != grant.ID {
		t.Fatalf("revision review status is incomplete: %#v err=%v", status, err)
	}
	unverified, err := service.ReviewProjection(ctx, grant.PlaintextToken)
	if err != nil || unverified.Verified || unverified.Submission != nil {
		t.Fatalf("unverified projection leaked content: %#v err=%v", unverified, err)
	}
	if _, err := service.VerifyReviewGrant(ctx, grant.PlaintextToken, "000000"); err == nil {
		t.Fatal("wrong OTP must fail")
	}
	verified, err := service.VerifyReviewGrant(ctx, grant.PlaintextToken, grant.PlaintextOTP)
	if err != nil || !verified.Verified || verified.Submission == nil || verified.Submission.SubmissionRevisionID != revision.ID {
		t.Fatalf("verified projection is incomplete: %#v err=%v", verified, err)
	}

	clientDecision, err := service.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "client approved", "", "client-approve")
	if err != nil {
		t.Fatal(err)
	}
	if clientDecision.Status != "approved" || clientDecision.ApprovedSnapshot == nil {
		t.Fatalf("client approval did not create a snapshot: %#v", clientDecision)
	}
	snapshot := *clientDecision.ApprovedSnapshot
	if snapshot.SubmissionRevisionID != revision.ID || snapshot.ContentHash != revision.ContentHash || snapshot.Origin != "current" || snapshot.CreatedBy != "client:client@example.com" {
		t.Fatalf("snapshot lineage is incomplete: %#v", snapshot)
	}
	if _, err := service.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "duplicate", "", "duplicate"); err == nil {
		t.Fatal("a review grant must be single-use")
	}
	decisions, err := store.Approvals(ctx, actor.TenantID, revision.ID)
	if err != nil || len(decisions) != 2 || decisions[0].DecisionStage != "client" || decisions[1].DecisionStage != "internal" {
		t.Fatalf("internal/client decisions were not preserved on one revision: %#v err=%v", decisions, err)
	}

	delivery, err := service.CreateDeliveryPackage(ctx, actor, snapshot.ID, "script-v2", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	if delivery.Status != "ready" || len(delivery.Manifest) != 3 || len(delivery.ApprovedSnapshotIDs) != 1 || delivery.ApprovedSnapshotIDs[0] != snapshot.ID {
		t.Fatalf("delivery package is incomplete: %#v", delivery)
	}
	formats := map[string]bool{}
	for _, artifact := range delivery.Manifest {
		formats[artifact.Metadata["format"].(string)] = true
		if artifact.ApprovedSnapshotID != snapshot.ID || artifact.Metadata["revision_hash"] != revision.ContentHash {
			t.Fatalf("artifact is not derived from the approved snapshot: %#v", artifact)
		}
		_, body, err := service.ArtifactBytes(ctx, actor, artifact.ID)
		if err != nil {
			t.Fatal(err)
		}
		if artifact.Metadata["format"] == "xlsx" {
			if _, err := zip.NewReader(bytes.NewReader(body), int64(len(body))); err != nil {
				t.Fatalf("xlsx delivery is not a valid archive: %v", err)
			}
		}
	}
	if !formats["json"] || !formats["markdown"] || !formats["xlsx"] {
		t.Fatalf("delivery formats are incomplete: %#v", formats)
	}

	now := time.Now().UTC().Add(-24 * time.Hour)
	imported, err := service.ImportPerformanceObservations(ctx, actor, app.ImportPerformanceInput{ProjectID: binding.ProjectID, SourceName: "results.csv", SourceFormat: "csv", Observations: []app.CreateObservationInput{{RowNumber: 2, ApprovedSnapshotID: snapshot.ID, Platform: "douyin", AccountAlias: "brand-main", PublishedAt: now, WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"impressions": 1000}, Currency: "CNY", Spend: 100, GMV: 300, IssueCategory: "creative"}}}, "results")
	if err != nil {
		t.Fatal(err)
	}
	if imported.Observations[0].ApprovedSnapshotID != snapshot.ID || imported.Observations[0].ScriptVersionID != "" {
		t.Fatalf("performance observation used the legacy version source: %#v", imported.Observations[0])
	}
	if _, err := service.CreateRatingDecision(ctx, actor, app.CreateRatingDecisionInput{ProjectID: binding.ProjectID, SubjectType: "approved_snapshot", SubjectID: snapshot.ID, ObservationIDs: []string{imported.Observations[0].ID}, Rating: "seed_candidate", Reason: "validated outcome", NextAction: "create one controlled variant"}, "rating"); err != nil {
		t.Fatal(err)
	}
	graph, err := service.ProjectLineage(ctx, actor, binding.ProjectID, app.LineageQuery{FocusType: "approved_snapshot", FocusID: snapshot.ID, Direction: "downstream"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"delivery_package:" + delivery.ID, "performance_observation:" + imported.Observations[0].ID} {
		if !lineageHasNode(graph, key) {
			t.Fatalf("lineage is missing %s: %#v", key, graph)
		}
	}
}

func TestV2NewRevisionRevokesPendingClientGrant(t *testing.T) {
	ctx, service, store, actor, binding := v2ScriptFixture(t)
	first := publishV2Script(t, ctx, service, binding, "script-first", "publish-first")
	if _, err := service.ApproveSubmission(ctx, actor, first.ID, "internal approved", "approve-first"); err != nil {
		t.Fatal(err)
	}
	grant, err := service.CreateReviewGrant(ctx, actor, first.ID, "client@example.com", "grant-first")
	if err != nil {
		t.Fatal(err)
	}
	second := publishV2Script(t, ctx, service, binding, "script-second", "publish-second")
	stored, err := store.ReviewGrant(ctx, actor.TenantID, grant.ID)
	if err != nil || stored.RevokedAt == nil {
		t.Fatalf("new revision did not revoke the previous grant: %#v err=%v", stored, err)
	}
	if _, err := service.ReviewProjection(ctx, grant.PlaintextToken); err == nil {
		t.Fatal("revoked grant remained usable")
	}
	if _, err := service.CreateReviewGrant(ctx, actor, second.ID, "client@example.com", "grant-second-too-early"); err == nil {
		t.Fatal("new revision inherited internal approval from the previous revision")
	}
}

func v2ScriptFixture(t *testing.T) (context.Context, *app.Service, *memory.Store, app.Actor, domain.WorkspaceBinding) {
	t.Helper()
	ctx := context.Background()
	store := memory.New()
	service := app.New(store, slog.Default())
	session, err := service.Register(ctx, "v2-full-chain@example.com", "long-enough-password", "Owner", "Agency")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.CreateConnectSession(ctx, actor, project.ID, "connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := service.ConnectDevice(ctx, app.ConnectDeviceInput{ConnectKey: connect.PlaintextConnectKey, Hostname: "local"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.WorkspaceActor(ctx, connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	binding, err = service.RegisterWorkspace(ctx, workspaceActor, binding, "workspace_marketing_video", "2.0.0", []string{"codex"}, "register")
	if err != nil {
		t.Fatal(err)
	}
	return ctx, service, store, actor, binding
}

func publishV2Script(t *testing.T, ctx context.Context, service *app.Service, binding domain.WorkspaceBinding, scriptID, idempotencyKey string) domain.SubmissionRevision {
	t.Helper()
	workspaceActor := app.Actor{TenantID: binding.TenantID, WorkspaceID: binding.ID, Type: "workspace", Role: "workspace"}
	pkg := localworkspace.ScriptPackageV2{ID: scriptID, Kind: "script_package", Status: "review_ready", SchemaVersion: "2.0", Deliverability: "review_ready", ProjectID: binding.ProjectID, ScriptID: scriptID, Title: "Approved script", Channel: "douyin", DurationMS: 15000, AspectRatio: "9:16", Shots: []localworkspace.ScriptShotV2{}, Citations: []localworkspace.ScriptCitationV2{}, AssetRequirements: []localworkspace.ScriptAssetRequirement{}, BlockedReasons: []localworkspace.ScriptBlockedReason{}, MissingInputs: []string{}}
	body, err := json.Marshal([]localworkspace.ScriptPackageV2{pkg})
	if err != nil {
		t.Fatal(err)
	}
	bundle := domain.SubmissionBundle{BundleVersion: "1.0", SchemaVersion: localworkspace.ScriptPackageV2Schema, SubmissionType: "script", ProjectID: binding.ProjectID, WorkspaceID: binding.ID, Objects: body, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{{Name: "script-lint", Status: "passed"}}}, IdempotencyKey: idempotencyKey}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.CreateSubmission(ctx, workspaceActor, binding, bundle, idempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func lineageHasNode(graph domain.LineageGraph, key string) bool {
	for _, node := range graph.Nodes {
		if node.Key == key {
			return true
		}
	}
	return false
}
