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
)

func TestV2WorkspaceSubmissionGovernanceWithPostgres(t *testing.T) {
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
	project, err := service.CreateProject(ctx, admin, app.CreateProjectInput{BrandName: "V2 Brand", ProductName: "V2 Product"}, "req-v2-project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.CreateConnectSession(ctx, admin, project.ID, "req-v2-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := service.ConnectDevice(ctx, app.ConnectDeviceInput{ConnectKey: connect.PlaintextConnectKey, Hostname: "v2-postgres", Platform: "darwin", Arch: "arm64", Version: "test"})
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
	binding, err = service.RegisterWorkspace(ctx, workspaceActor, binding, "workspace_marketing_video", "2.0.0", []string{"codex"}, "req-v2-register")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.DeviceActor(ctx, connected.DeviceToken); err != nil {
		t.Fatalf("workspace registration invalidated the optional device credential: %v", err)
	}

	firstBundle := postgresSubmissionBundle(t, project.ID, binding.ID, "postgres-v2-first", "fact-1")
	firstRevision, err := service.CreateSubmission(ctx, workspaceActor, binding, firstBundle, "req-v2-first")
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
	var immutableObjects, submittedObjects any
	if decodeErr := json.Unmarshal(immutable.Objects, &immutableObjects); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if decodeErr := json.Unmarshal(firstRevision.Objects, &submittedObjects); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if immutable.Message != firstRevision.Message || !reflect.DeepEqual(immutableObjects, submittedObjects) {
		t.Fatalf("immutable revision changed: revision=%#v", immutable)
	}

	changed, err := service.RequestSubmissionChanges(ctx, admin, firstRevision.ID, "补充事实适用边界", "/0/scope", "req-v2-changes")
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

	secondBundle := postgresSubmissionBundle(t, project.ID, binding.ID, "postgres-v2-second", "fact-2")
	secondRevision, err := service.CreateSubmission(ctx, workspaceActor, binding, secondBundle, "req-v2-second")
	if err != nil {
		t.Fatal(err)
	}
	approval, err := service.ApproveSubmission(ctx, admin, secondRevision.ID, "事实与来源披露已审核", "req-v2-approve")
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

	scriptBundle := postgresScriptSubmissionBundle(t, project.ID, binding.ID, "postgres-script-v1", "postgres-script")
	scriptRevision, err := service.CreateSubmission(ctx, workspaceActor, binding, scriptBundle, "req-script-publish")
	if err != nil {
		t.Fatal(err)
	}
	internalApproval, err := service.ApproveSubmission(ctx, admin, scriptRevision.ID, "script internal approval", "req-script-internal")
	if err != nil || internalApproval.ApprovedSnapshot != nil || internalApproval.Submission.Status != "internally_approved" {
		t.Fatalf("script internal approval did not stop before snapshot creation: %#v err=%v", internalApproval, err)
	}
	grant, err := service.CreateReviewGrant(ctx, admin, scriptRevision.ID, "postgres-client@example.com", "req-script-grant")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.VerifyReviewGrant(ctx, grant.PlaintextToken, grant.PlaintextOTP); err != nil {
		t.Fatal(err)
	}
	clientApproval, err := service.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "client approved", "", "req-script-client")
	if err != nil || clientApproval.ApprovedSnapshot == nil {
		t.Fatalf("client approval did not atomically create snapshot: %#v err=%v", clientApproval, err)
	}
	scriptSnapshot := *clientApproval.ApprovedSnapshot
	persistedScriptSnapshot, err := store.ApprovedSnapshot(ctx, admin.TenantID, scriptSnapshot.ID)
	if err != nil || persistedScriptSnapshot.CreatedBy != "client:postgres-client@example.com" || persistedScriptSnapshot.Origin != "current" {
		t.Fatalf("client snapshot was not persisted with current lineage: %#v err=%v", persistedScriptSnapshot, err)
	}
	delivery, err := service.CreateDeliveryPackage(ctx, admin, scriptSnapshot.ID, "postgres-script", "req-delivery")
	if err != nil || len(delivery.Manifest) != 3 {
		t.Fatalf("delivery package transaction failed: %#v err=%v", delivery, err)
	}
	persistedDelivery, err := store.DeliveryPackage(ctx, admin.TenantID, delivery.ID)
	if err != nil || len(persistedDelivery.Manifest) != 3 || persistedDelivery.ApprovedSnapshotIDs[0] != scriptSnapshot.ID {
		t.Fatalf("delivery relations were not persisted: %#v err=%v", persistedDelivery, err)
	}
	performance, err := service.ImportPerformanceObservations(ctx, admin, app.ImportPerformanceInput{ProjectID: project.ID, SourceName: "postgres-results.csv", SourceFormat: "csv", Observations: []app.CreateObservationInput{{RowNumber: 2, ApprovedSnapshotID: scriptSnapshot.ID, Platform: "douyin", AccountAlias: "brand-main", PublishedAt: time.Now().UTC().Add(-24 * time.Hour), WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"impressions": 1000}, Currency: "CNY", Spend: 100, GMV: 300, IssueCategory: "creative"}}}, "req-results")
	if err != nil || performance.Observations[0].ApprovedSnapshotID != scriptSnapshot.ID {
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

func postgresScriptSubmissionBundle(t *testing.T, projectID, workspaceID, idempotencyKey, scriptID string) domain.SubmissionBundle {
	t.Helper()
	pkg := localworkspace.ScriptPackageV2{ID: scriptID, Kind: "script_package", Status: "review_ready", SchemaVersion: "2.0", Deliverability: "review_ready", ProjectID: projectID, ScriptID: scriptID, Title: "PostgreSQL script", Channel: "douyin", DurationMS: 15000, AspectRatio: "9:16", Shots: []localworkspace.ScriptShotV2{}, Citations: []localworkspace.ScriptCitationV2{}, AssetRequirements: []localworkspace.ScriptAssetRequirement{}, BlockedReasons: []localworkspace.ScriptBlockedReason{}, MissingInputs: []string{}}
	objects, err := json.Marshal([]localworkspace.ScriptPackageV2{pkg})
	if err != nil {
		t.Fatal(err)
	}
	bundle := domain.SubmissionBundle{BundleVersion: "1.0", SchemaVersion: localworkspace.ScriptPackageV2Schema, SubmissionType: "script", ProjectID: projectID, WorkspaceID: workspaceID, Objects: objects, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{{Name: "script-lint", Status: "passed"}}}, IdempotencyKey: idempotencyKey}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func postgresSubmissionBundle(t *testing.T, projectID, workspaceID, idempotencyKey, factID string) domain.SubmissionBundle {
	t.Helper()
	bundle := domain.SubmissionBundle{
		BundleVersion: "1.0", SchemaVersion: "contentcloud.knowledge/2.0", SubmissionType: "knowledge", ProjectID: projectID, WorkspaceID: workspaceID,
		LocalRunSummary:   domain.LocalRunSummary{Stage: "publish_preflight", Checks: []domain.LocalRunCheck{{Name: "knowledge-lint", Status: "passed"}}},
		Objects:           json.RawMessage(fmt.Sprintf(`[{"id":%q,"kind":"fact","status":"verified","risk_level":"low"}]`, factID)),
		SourceDisclosures: []domain.SourceDisclosure{{SourceRef: "source-" + factID, Level: "metadata_only", SHA256: strings.Repeat("a", 64)}},
		Artifacts:         []domain.SubmissionArtifact{}, Message: "PostgreSQL V2 integration", IdempotencyKey: idempotencyKey,
	}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	return bundle
}
