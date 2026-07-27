package httpapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
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
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, app.ConnectDeviceInput{Hostname: "local"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, _ := service.WorkspaceActor(t.Context(), connected.WorkspaceToken)
	bundle := domain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "knowledge", ProjectID: project.ID, WorkspaceID: binding.ID, BaseSnapshotIDs: []string{}, EnvironmentDigest: httpSubmissionEnvironmentDigest, Objects: []domain.SubmissionObjectRef{mustHTTPSubmissionObject(t, "fact-1", "Fact", "30-knowledge/pages/facts/fact-1.json", map[string]any{"id": "fact-1", "kind": "fact", "status": "verified"})}, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{}}, IdempotencyKey: "bff-v1"}
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
	if len(details.Revisions) != 1 || !reflect.DeepEqual(details.Revisions[0].Objects, revision.Objects) {
		t.Fatalf("unexpected submission details: %#v", details)
	}
	focused := callBFF[app.SubmissionRevisionView](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/submission-revisions/"+revision.ID, nil)
	if focused.Submission.ID != listed[0].ID || focused.Revision.ID != revision.ID || focused.Revision.ContentHash != revision.ContentHash || len(focused.Comments) != 0 {
		t.Fatalf("unexpected focused revision: %#v", focused)
	}
	otherProject, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Other", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(server.URL + "/api/bff/projects/" + otherProject.ID + "/submission-revisions/" + revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-project revision lookup must be hidden: status=%d", response.StatusCode)
	}
	foreignSession, err := service.Register(t.Context(), "foreign-submission@example.com", "long-enough-password", "Foreign", "Foreign Tenant")
	if err != nil {
		t.Fatal(err)
	}
	foreignJar, _ := cookiejar.New(nil)
	foreignJar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: foreignSession.ID, Path: "/"}})
	foreignResponse, err := (&http.Client{Jar: foreignJar}).Get(server.URL + "/api/bff/projects/" + project.ID + "/submission-revisions/" + revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	foreignBody, readErr := io.ReadAll(foreignResponse.Body)
	foreignResponse.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if foreignResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-tenant revision lookup must be hidden: status=%d", foreignResponse.StatusCode)
	}
	for _, privateValue := range []string{project.ID, revision.ID, project.BrandName, project.ProductName} {
		if strings.Contains(string(foreignBody), privateValue) {
			t.Fatalf("cross-tenant response leaked %q: %s", privateValue, foreignBody)
		}
	}
	returned := callBFF[domain.Submission](t, client, http.MethodPost, server.URL+"/api/bff/submission-revisions/"+revision.ID+"/request-changes", map[string]string{"reason": "补充范围", "json_pointer": "/0/scope"})
	if returned.Status != "changes_requested" {
		t.Fatalf("unexpected returned state: %#v", returned)
	}
	persisted, err := service.SubmissionDetails(t.Context(), actor, returned.ID)
	if err != nil || !reflect.DeepEqual(persisted.Revisions[0].Objects, revision.Objects) || len(persisted.Comments) != 1 {
		t.Fatalf("BFF review mutated content or lost comment: %#v %v", persisted, err)
	}
}

func TestV3ContentBatchBFFCompletesClientApprovalChain(t *testing.T) {
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
	connected, err := testsupport.ConnectBootstrap(ctx, service, actor, connect, app.ConnectDeviceInput{Hostname: "local"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.WorkspaceActor(ctx, connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	contentBatch := map[string]any{
		"schema_version":          "contentcloud.content-batch/3.0",
		"id":                      "content-batch:bff:v1",
		"intent_id":               "intent:bff",
		"brief_ref":               "brief:bff:v1",
		"knowledge_snapshot_refs": []string{"snapshot:knowledge:v1"},
		"status":                  "review_ready",
		"publishable":             true,
		"content_item_refs":       []string{"content-item:bff:v1"},
		"blocked_reasons":         []string{},
		"checks":                  []map[string]string{{"name": "schema", "status": "passed"}, {"name": "knowledge_eligibility", "status": "passed"}},
	}
	bundle := domain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "content_batch", ProjectID: project.ID, WorkspaceID: binding.ID, BaseSnapshotIDs: []string{}, EnvironmentDigest: httpSubmissionEnvironmentDigest, Objects: []domain.SubmissionObjectRef{mustHTTPSubmissionObject(t, "content-batch:bff:v1", "content_batch", "50-production/batches/bff/manifest.yaml", contentBatch)}, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{{Name: "content-batch-lint", Status: "passed"}}}, IdempotencyKey: "bff-content-batch-v3"}
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
		t.Fatalf("verified V3 projection is incomplete: %#v", verified)
	}
	decision := callBFF[app.ReviewDecisionResult](t, client, http.MethodPost, server.URL+"/api/review/"+grant.PlaintextToken+"/decision", map[string]string{"decision": "approve", "reason": "client approved"})
	if decision.ApprovedSnapshot == nil || decision.Status != "approved" {
		t.Fatalf("client decision did not create an ApprovedSnapshot: %#v", decision)
	}
	snapshot := *decision.ApprovedSnapshot
	if snapshot.SubmissionRevisionID != revision.ID || snapshot.ContentHash != revision.ContentHash {
		t.Fatalf("snapshot lost revision lineage: %#v", snapshot)
	}
	listed := callBFF[[]domain.ApprovedSnapshot](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/approved-snapshots?type=content_batch", nil)
	if len(listed) != 1 || listed[0].ID != snapshot.ID {
		t.Fatalf("snapshot list mismatch: %#v", listed)
	}
	projection := callBFF[domain.ProjectProjection](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/projection", nil)
	if projection.Sections["creative"].Status != "ready" || len(projection.Snapshots) != 1 || projection.Snapshots[0].ID != snapshot.ID {
		t.Fatalf("project projection did not consume the approved snapshot: %#v", projection)
	}
}

const httpSubmissionEnvironmentDigest = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

func mustHTTPSubmissionObject(t *testing.T, id, objectType, path string, content any) domain.SubmissionObjectRef {
	t.Helper()
	value, err := domain.NewSubmissionObjectRef(id, objectType, 1, path, content)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
