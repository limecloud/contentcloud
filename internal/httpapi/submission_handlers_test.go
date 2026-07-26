package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
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
