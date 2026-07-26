package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/localworkspace"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestSubmissionBFFReviewDoesNotEditRevisionContent(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "submission-bff@example.com", "long-enough-password", "Reviewer", "Agency")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, _ := service.SessionActor(t.Context(), session.ID)
	project, _ := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	connect, _ := service.CreateConnectSession(t.Context(), actor, project.ID, "")
	connected, err := service.ConnectDevice(t.Context(), app.ConnectDeviceInput{ConnectKey: connect.PlaintextConnectKey, Hostname: "local"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, _ := service.WorkspaceActor(t.Context(), connected.WorkspaceToken)
	bundle := domain.SubmissionBundle{BundleVersion: "1.0", SchemaVersion: "contentcloud.knowledge/2.0", SubmissionType: "knowledge", ProjectID: project.ID, WorkspaceID: binding.ID, Objects: json.RawMessage(`[{"id":"fact-1","kind":"fact","status":"verified"}]`), SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{}}, IdempotencyKey: "bff-v1"}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.CreateSubmission(t.Context(), workspaceActor, binding, bundle, "")
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}

	listed := callBFF[[]domain.Submission](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/submissions", nil)
	if len(listed) != 1 || listed[0].CurrentRevisionID != revision.ID {
		t.Fatalf("unexpected submission list: %#v", listed)
	}
	details := callBFF[app.SubmissionDetails](t, client, http.MethodGet, server.URL+"/api/bff/submissions/"+listed[0].ID, nil)
	if len(details.Revisions) != 1 || string(details.Revisions[0].Objects) != string(revision.Objects) {
		t.Fatalf("unexpected submission details: %#v", details)
	}
	returned := callBFF[domain.Submission](t, client, http.MethodPost, server.URL+"/api/bff/submission-revisions/"+revision.ID+"/request-changes", map[string]string{"reason": "补充范围", "json_pointer": "/0/scope"})
	if returned.Status != "changes_requested" {
		t.Fatalf("unexpected returned state: %#v", returned)
	}
	persisted, err := service.SubmissionDetails(t.Context(), actor, returned.ID)
	if err != nil || string(persisted.Revisions[0].Objects) != string(revision.Objects) || len(persisted.Comments) != 1 {
		t.Fatalf("BFF review mutated content or lost comment: %#v %v", persisted, err)
	}
}

func TestV2SubmissionBFFCompletesClientDeliveryAndResultChain(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "v2-bff@example.com", "long-enough-password", "Owner", "Agency")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: "douyin"}, "project")
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
	pkg := localworkspace.ScriptPackageV2{ID: "script-bff", Kind: "script_package", Status: "review_ready", SchemaVersion: "2.0", Deliverability: "review_ready", ProjectID: project.ID, ScriptID: "script-bff", Title: "BFF approved script", Channel: "douyin", DurationMS: 15000, AspectRatio: "9:16", Shots: []localworkspace.ScriptShotV2{}, Citations: []localworkspace.ScriptCitationV2{}, AssetRequirements: []localworkspace.ScriptAssetRequirement{}, BlockedReasons: []localworkspace.ScriptBlockedReason{}, MissingInputs: []string{}}
	objects, err := json.Marshal([]localworkspace.ScriptPackageV2{pkg})
	if err != nil {
		t.Fatal(err)
	}
	bundle := domain.SubmissionBundle{BundleVersion: "1.0", SchemaVersion: localworkspace.ScriptPackageV2Schema, SubmissionType: "script", ProjectID: project.ID, WorkspaceID: binding.ID, Objects: objects, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{{Name: "script-lint", Status: "passed"}}}, IdempotencyKey: "bff-script"}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.CreateSubmission(ctx, workspaceActor, binding, bundle, "publish")
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}

	internal := callBFF[app.SubmissionApprovalResult](t, client, http.MethodPost, server.URL+"/api/bff/submission-revisions/"+revision.ID+"/approve", map[string]string{"reason": "internal approved"})
	if internal.ApprovedSnapshot != nil || internal.Submission.Status != "internally_approved" || internal.Decision.DecisionStage != "internal" {
		t.Fatalf("internal approval did not stop before client approval: %#v", internal)
	}
	grant := callBFF[domain.ReviewGrant](t, client, http.MethodPost, server.URL+"/api/bff/submission-revisions/"+revision.ID+"/review-grants", map[string]string{"reviewer_email": "client@example.com"})
	if grant.SubjectType != "submission_revision" || grant.SubjectID != revision.ID || grant.PlaintextToken == "" || grant.PlaintextOTP == "" {
		t.Fatalf("revision grant response is incomplete: %#v", grant)
	}
	grants := callBFF[[]domain.ReviewGrant](t, client, http.MethodGet, server.URL+"/api/bff/submission-revisions/"+revision.ID+"/review-grants", nil)
	if len(grants) != 1 || grants[0].PlaintextToken != "" || grants[0].PlaintextOTP != "" {
		t.Fatalf("grant history omitted the grant or leaked secrets: %#v", grants)
	}
	unverified := callBFF[app.ReviewProjection](t, client, http.MethodGet, server.URL+"/api/review/"+grant.PlaintextToken+"/projection", nil)
	if unverified.Verified || unverified.Submission != nil {
		t.Fatalf("unverified projection leaked revision content: %#v", unverified)
	}
	verified := callBFF[app.ReviewProjection](t, client, http.MethodPost, server.URL+"/api/review/"+grant.PlaintextToken+"/verify", map[string]string{"otp": grant.PlaintextOTP})
	if !verified.Verified || verified.Submission == nil || verified.Submission.SubmissionRevisionID != revision.ID || verified.Submission.SubjectHash != revision.ContentHash {
		t.Fatalf("verified V2 projection is incomplete: %#v", verified)
	}
	decision := callBFF[app.ReviewDecisionResult](t, client, http.MethodPost, server.URL+"/api/review/"+grant.PlaintextToken+"/decision", map[string]string{"decision": "approve", "reason": "client approved"})
	if decision.ApprovedSnapshot == nil || decision.Status != "approved" {
		t.Fatalf("client decision did not create an ApprovedSnapshot: %#v", decision)
	}
	snapshot := *decision.ApprovedSnapshot
	if snapshot.SubmissionRevisionID != revision.ID || snapshot.ContentHash != revision.ContentHash || snapshot.Origin != "current" {
		t.Fatalf("snapshot lost revision lineage: %#v", snapshot)
	}
	listed := callBFF[[]domain.ApprovedSnapshot](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/approved-snapshots?type=script", nil)
	if len(listed) != 1 || listed[0].ID != snapshot.ID {
		t.Fatalf("snapshot list mismatch: %#v", listed)
	}
	artifact := callBFF[domain.Artifact](t, client, http.MethodPost, server.URL+"/api/bff/approved-snapshots/"+snapshot.ID+"/exports", map[string]string{"format": "json"})
	if artifact.ApprovedSnapshotID != snapshot.ID || artifact.Metadata["revision_hash"] != revision.ContentHash {
		t.Fatalf("snapshot export lineage is incomplete: %#v", artifact)
	}
	delivery := callBFF[domain.DeliveryPackage](t, client, http.MethodPost, server.URL+"/api/bff/approved-snapshots/"+snapshot.ID+"/delivery-packages", map[string]any{})
	if len(delivery.Manifest) != 3 || len(delivery.ApprovedSnapshotIDs) != 1 || delivery.ApprovedSnapshotIDs[0] != snapshot.ID {
		t.Fatalf("delivery package is incomplete: %#v", delivery)
	}
	deliveries := callBFF[[]domain.DeliveryPackage](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/delivery-packages", nil)
	if len(deliveries) != 1 || len(deliveries[0].Manifest) != 3 {
		t.Fatalf("delivery list mismatch: %#v", deliveries)
	}
	performance := callBFF[app.ImportPerformanceResult](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/results", app.CreateObservationInput{ApprovedSnapshotID: snapshot.ID, Platform: "douyin", AccountAlias: "brand-main", PublishedAt: time.Now().UTC().Add(-24 * time.Hour), WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"impressions": 1000}, Currency: "CNY", Spend: 100, GMV: 300, IssueCategory: "creative"})
	if len(performance.Observations) != 1 || performance.Observations[0].ApprovedSnapshotID != snapshot.ID || performance.Observations[0].ScriptVersionID != "" {
		t.Fatalf("performance observation did not bind the snapshot: %#v", performance)
	}
}
