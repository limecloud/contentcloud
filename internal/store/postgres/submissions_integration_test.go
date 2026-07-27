package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
	storepg "github.com/limecloud/contentcloud/internal/store/postgres"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestV3WorkspaceSubmissionGovernanceWithPostgres(t *testing.T) {
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
	service := app.New(store, slog.Default())
	suffix := domain.NewID()
	session, err := service.Register(ctx, fmt.Sprintf("submission-%s@example.com", suffix), "long-enough-password", "Submission Owner", "Submission Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	admin, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, admin, app.CreateProjectInput{BrandName: "V3 Brand", ProductName: "V3 Product"}, "req-v3-project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.CreateConnectSession(ctx, admin, project.ID, "req-v3-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(ctx, service, admin, connect, app.ConnectDeviceInput{Hostname: "v3-postgres", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.WorkspaceActor(ctx, connected.WorkspaceToken)
	if err != nil {
		t.Fatalf("workspace credential lookup failed: %v", err)
	}
	if workspaceActor.WorkspaceID != connected.WorkspaceID || binding.ProjectID != project.ID || binding.CredentialHash != "" {
		t.Fatalf("unexpected workspace credential projection: actor=%#v binding=%#v", workspaceActor, binding)
	}
	binding, err = service.RegisterWorkspace(ctx, workspaceActor, binding, "workspace_marketing_agent", "3.0.0", []string{"codex"}, "req-v3-register")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.DeviceActor(ctx, connected.DeviceToken); err != nil {
		t.Fatalf("workspace registration invalidated the optional device credential: %v", err)
	}

	firstBundle := postgresSubmissionBundle(t, project.ID, binding.ID, "postgres-v3-first", "fact-1")
	firstRevision, err := service.CreateSubmission(ctx, workspaceActor, binding, firstBundle, "req-v3-first")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `UPDATE submission_revisions SET message='tampered' WHERE id=$1`, firstRevision.ID); err == nil {
		t.Fatal("submission revision update unexpectedly bypassed the immutability trigger")
	}
	immutable, err := store.SubmissionRevision(ctx, admin.TenantID, firstRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if immutable.Message != firstRevision.Message || !semanticJSONEqual(t, immutable.Objects, firstRevision.Objects) {
		t.Fatalf("immutable revision changed: revision=%#v", immutable)
	}

	changed, err := service.RequestSubmissionChanges(ctx, admin, firstRevision.ID, "补充事实适用边界", "/0/scope", "req-v3-changes")
	if err != nil || changed.Status != "changes_requested" {
		t.Fatalf("request changes transaction failed: submission=%#v err=%v", changed, err)
	}
	comments, err := store.ReviewComments(ctx, admin.TenantID, firstRevision.ID)
	if err != nil || len(comments) != 1 || comments[0].JSONPointer != "/0/scope" {
		t.Fatalf("review comment was not committed with the state transition: comments=%#v err=%v", comments, err)
	}
	decisions, err := store.Approvals(ctx, admin.TenantID, firstRevision.ID)
	if err != nil || len(decisions) != 1 || decisions[0].Decision != "request_changes" {
		t.Fatalf("change decision was not committed with the state transition: decisions=%#v err=%v", decisions, err)
	}

	secondBundle := postgresSubmissionBundle(t, project.ID, binding.ID, "postgres-v3-second", "fact-2")
	secondRevision, err := service.CreateSubmission(ctx, workspaceActor, binding, secondBundle, "req-v3-second")
	if err != nil {
		t.Fatal(err)
	}
	approval, err := service.ApproveSubmission(ctx, admin, secondRevision.ID, "事实与来源披露已审核", "req-v3-approve")
	if err != nil {
		t.Fatal(err)
	}
	if approval.ApprovedSnapshot == nil {
		t.Fatal("knowledge approval must create an ApprovedSnapshot")
	}
	snapshot := *approval.ApprovedSnapshot
	persistedSnapshot, err := store.ApprovedSnapshot(ctx, admin.TenantID, snapshot.ID)
	if err != nil || persistedSnapshot.SubmissionRevisionID != secondRevision.ID || persistedSnapshot.ContentHash != secondRevision.ContentHash {
		t.Fatalf("approval snapshot transaction failed: snapshot=%#v err=%v", persistedSnapshot, err)
	}
	persistedSubmission, err := store.Submission(ctx, admin.TenantID, secondRevision.SubmissionID)
	if err != nil || persistedSubmission.Status != "approved" || persistedSubmission.CurrentRevisionID != secondRevision.ID {
		t.Fatalf("approved submission state was not committed atomically: submission=%#v err=%v", persistedSubmission, err)
	}
	staleSubmission := persistedSubmission
	staleSubmission.Status = "changes_requested"
	staleSubmission.UpdatedAt = time.Now().UTC()
	staleDecision := domain.ApprovalDecision{ID: domain.NewID(), TenantID: admin.TenantID, ProjectID: project.ID, SubjectType: "submission_revision", SubjectID: secondRevision.ID, SubjectHash: secondRevision.ContentHash, DecisionStage: "internal", ActorID: admin.UserID, Decision: "request_changes", PreviousState: "submitted", ResultingState: "changes_requested", CreatedAt: staleSubmission.UpdatedAt}
	staleComment := domain.ReviewComment{ID: domain.NewID(), TenantID: admin.TenantID, ProjectID: project.ID, SubjectType: "submission_revision", SubjectID: secondRevision.ID, Body: "stale decision", Visibility: "internal", AuthorID: admin.UserID, CreatedAt: staleSubmission.UpdatedAt}
	if err := store.RequestSubmissionChanges(ctx, staleSubmission, staleDecision, staleComment); err == nil {
		t.Fatal("stale state transition unexpectedly overwrote an approved submission")
	}
	stillApproved, err := store.Submission(ctx, admin.TenantID, secondRevision.SubmissionID)
	if err != nil || stillApproved.Status != "approved" {
		t.Fatalf("stale transition changed approved submission: submission=%#v err=%v", stillApproved, err)
	}
	staleComments, err := store.ReviewComments(ctx, admin.TenantID, secondRevision.ID)
	if err != nil || len(staleComments) != 0 {
		t.Fatalf("stale transition persisted review comment: comments=%#v err=%v", staleComments, err)
	}

	contentBundle := postgresContentSubmissionBundle(t, project.ID, binding.ID, "postgres-content-v1", "postgres-content-item")
	contentRevision, err := service.CreateSubmission(ctx, workspaceActor, binding, contentBundle, "req-content-publish")
	if err != nil {
		t.Fatal(err)
	}
	internalApproval, err := service.ApproveSubmission(ctx, admin, contentRevision.ID, "content internal approval", "req-content-internal")
	if err != nil || internalApproval.ApprovedSnapshot != nil || internalApproval.Submission.Status != "internally_approved" {
		t.Fatalf("content internal approval did not stop before snapshot creation: %#v err=%v", internalApproval, err)
	}
	grant, err := service.CreateReviewGrant(ctx, admin, contentRevision.ID, "postgres-client@example.com", "req-content-grant")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.VerifyReviewGrant(ctx, grant.PlaintextToken, grant.PlaintextOTP); err != nil {
		t.Fatal(err)
	}
	clientApproval, err := service.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "client approved", "", "req-content-client")
	if err != nil || clientApproval.ApprovedSnapshot == nil {
		t.Fatalf("client approval did not atomically create snapshot: %#v err=%v", clientApproval, err)
	}
	contentSnapshot := *clientApproval.ApprovedSnapshot
	persistedContentSnapshot, err := store.ApprovedSnapshot(ctx, admin.TenantID, contentSnapshot.ID)
	if err != nil || persistedContentSnapshot.CreatedBy != "client:postgres-client@example.com" {
		t.Fatalf("client snapshot was not persisted with current lineage: %#v err=%v", persistedContentSnapshot, err)
	}
	delivery, err := service.CreateDeliveryPackage(ctx, admin, contentSnapshot.ID, "postgres-content-item", "req-delivery")
	if err != nil || delivery.ContentItemID != "postgres-content-item" || len(delivery.Manifest) != 3 {
		t.Fatalf("delivery package transaction failed: %#v err=%v", delivery, err)
	}
	persistedDelivery, err := store.DeliveryPackage(ctx, admin.TenantID, delivery.ID)
	if err != nil || persistedDelivery.ContentItemID != "postgres-content-item" || len(persistedDelivery.Manifest) != 3 || persistedDelivery.ApprovedSnapshotIDs[0] != contentSnapshot.ID {
		t.Fatalf("delivery relations were not persisted: %#v err=%v", persistedDelivery, err)
	}
	performance, err := service.ImportPerformanceObservations(ctx, admin, app.ImportPerformanceInput{ProjectID: project.ID, SourceName: "postgres-results.csv", SourceFormat: "csv", Observations: []app.CreateObservationInput{{RowNumber: 2, ApprovedSnapshotID: contentSnapshot.ID, Platform: "douyin", AccountAlias: "brand-main", PublishedAt: time.Now().UTC().Add(-24 * time.Hour), WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"impressions": 1000}, Currency: "CNY", Spend: 100, GMV: 300, IssueCategory: "creative"}}}, "req-results")
	if err != nil || performance.Observations[0].ApprovedSnapshotID != contentSnapshot.ID {
		t.Fatalf("performance observation did not bind the snapshot: %#v err=%v", performance, err)
	}

	otherSession, err := service.Register(ctx, fmt.Sprintf("submission-other-%s@example.com", suffix), "long-enough-password", "Other Owner", "Other Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	otherActor, _, err := service.SessionActor(ctx, otherSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id',$1,true)`, otherActor.TenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE contentcloud_runtime`); err != nil {
		t.Fatal(err)
	}
	for table, id := range map[string]string{
		"workspace_bindings":   binding.ID,
		"submissions":          persistedSubmission.ID,
		"submission_revisions": secondRevision.ID,
		"source_disclosures":   secondRevision.SourceDisclosures[0].ID,
		"approved_snapshots":   snapshot.ID,
	} {
		var count int
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE id=$1", table)
		if err := tx.QueryRow(ctx, query, id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("tenant B saw tenant A %s row through RLS", table)
		}
	}
}

func postgresContentSubmissionBundle(t *testing.T, projectID, workspaceID, idempotencyKey, contentItemID string) domain.SubmissionBundle {
	t.Helper()
	pkg := localworkspace.ContentItem{ID: contentItemID, Type: "content_item", Status: "review_ready", SchemaVersion: localworkspace.ContentItemSchema, Deliverability: "review_ready", ProjectID: projectID, ContentID: contentItemID, Title: "PostgreSQL content item", Channel: "douyin", DurationMS: 15000, AspectRatio: "9:16", Shots: []localworkspace.ContentShot{}, Citations: []localworkspace.ContentCitation{}, AssetRequirements: []localworkspace.ContentAssetRequirement{}, BlockedReasons: []localworkspace.ContentBlockedReason{}, MissingInputs: []string{}}
	object, err := domain.NewSubmissionObjectRef(contentItemID, "content_item", 1, "50-production/batches/postgres/"+contentItemID+".json", pkg)
	if err != nil {
		t.Fatal(err)
	}
	bundle := domain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "content_batch", ProjectID: projectID, WorkspaceID: workspaceID, BaseSnapshotIDs: []string{}, EnvironmentDigest: postgresSubmissionEnvironmentDigest, Objects: []domain.SubmissionObjectRef{object}, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{{Name: "content-item-lint", Status: "passed"}}}, IdempotencyKey: idempotencyKey}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func postgresSubmissionBundle(t *testing.T, projectID, workspaceID, idempotencyKey, factID string) domain.SubmissionBundle {
	t.Helper()
	bundle := domain.SubmissionBundle{
		BundleVersion: "3.0", SubmissionType: "knowledge", ProjectID: projectID, WorkspaceID: workspaceID, BaseSnapshotIDs: []string{}, EnvironmentDigest: postgresSubmissionEnvironmentDigest,
		LocalRunSummary:   domain.LocalRunSummary{Stage: "publish_preflight", Checks: []domain.LocalRunCheck{{Name: "knowledge-lint", Status: "passed"}}},
		Objects:           []domain.SubmissionObjectRef{postgresSubmissionObject(t, factID, "Fact", "30-knowledge/pages/facts/"+factID+".json", map[string]any{"id": factID, "kind": "fact", "status": "verified", "risk_level": "low"})},
		SourceDisclosures: []domain.SourceDisclosure{{SourceRef: "source-" + factID, Level: "metadata_only", SHA256: strings.Repeat("a", 64)}},
		Artifacts:         []domain.SubmissionArtifact{}, Message: "PostgreSQL V3 integration", IdempotencyKey: idempotencyKey,
	}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	return bundle
}

const postgresSubmissionEnvironmentDigest = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

func postgresSubmissionObject(t *testing.T, id, objectType, path string, content any) domain.SubmissionObjectRef {
	t.Helper()
	value, err := domain.NewSubmissionObjectRef(id, objectType, 1, path, content)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func semanticJSONEqual(t *testing.T, left, right any) bool {
	t.Helper()
	decode := func(value any) any {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	return reflect.DeepEqual(decode(left), decode(right))
}
