package httpapi_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

type codexHandoffResponse struct {
	SchemaVersion string `json:"schema_version"`
	Kind          string `json:"kind"`
	ProjectID     string `json:"project_id"`
	Target        struct {
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Digest string `json:"digest"`
	} `json:"target"`
	PluginID                   string   `json:"plugin_id"`
	PluginVersion              string   `json:"plugin_version"`
	RequiresNewChat            bool     `json:"requires_new_chat"`
	RequiresWorkspaceSelection bool     `json:"requires_workspace_selection"`
	LaunchURL                  string   `json:"launch_url"`
	Prompt                     string   `json:"prompt"`
	Steps                      []string `json:"steps"`
	FallbackURL                string   `json:"fallback_url"`
}

func TestProjectCodexHandoffRequiresBoundWorkspaceAndOmitsPrivateData(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "handoff@example.com", "long-enough-password", "Owner", "Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "PRIVATE_BRAND", ProductName: "PRIVATE_PRODUCT"}, "")
	if err != nil {
		t.Fatal(err)
	}
	server, client := codexHandoffServer(t, service, session.ID)

	response := codexHandoffRequest(t, client, server.URL+"/api/bff/projects/"+project.ID+"/codex-handoff")
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("unbound project handoff status=%d, want 409", response.StatusCode)
	}
	if code := codexHandoffErrorCode(t, response); code != "CODEX_HANDOFF_WORKSPACE_REQUIRED" {
		t.Fatalf("unexpected unbound error %q", code)
	}

	connect, err := service.CreateConnectSession(t.Context(), actor, project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, app.ConnectDeviceInput{Hostname: "PRIVATE_HOST", Platform: "darwin", Arch: "arm64", Version: "0.8.0"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.WorkspaceActor(t.Context(), connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RegisterWorkspace(t.Context(), workspaceActor, binding, "workspace_marketing_agent", "3.0.0", []string{"codex"}, ""); err != nil {
		t.Fatal(err)
	}
	handoff, raw := readCodexHandoff(t, client, server.URL+"/api/bff/projects/"+project.ID+"/codex-handoff")
	assertProjectCodexHandoff(t, handoff, project.ID)
	for _, privateValue := range []string{"PRIVATE_BRAND", "PRIVATE_PRODUCT", "PRIVATE_HOST", "/Users/", "workspace_token", "connect_key", "originUrl"} {
		if strings.Contains(raw, privateValue) {
			t.Fatalf("handoff leaked private value %q: %s", privateValue, raw)
		}
	}
}

func TestReviewFeedbackCodexHandoffIsProjectScopedAndReadOnly(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "feedback-handoff@example.com", "long-enough-password", "Reviewer", "Agency")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "PRIVATE_FEEDBACK_BRAND", ProductName: "PRIVATE_FEEDBACK_PRODUCT"}, "")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.CreateConnectSession(t.Context(), actor, project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, app.ConnectDeviceInput{Hostname: "local", Platform: "darwin", Arch: "arm64", Version: "0.8.0"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.WorkspaceActor(t.Context(), connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	binding, err = service.RegisterWorkspace(t.Context(), workspaceActor, binding, "workspace_marketing_agent", "3.0.0", []string{"codex"}, "")
	if err != nil {
		t.Fatal(err)
	}
	bundle := domain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "knowledge", ProjectID: project.ID, WorkspaceID: binding.ID, BaseSnapshotIDs: []string{}, EnvironmentDigest: httpSubmissionEnvironmentDigest, Objects: []domain.SubmissionObjectRef{mustHTTPSubmissionObject(t, "fact-handoff", "Fact", "30-knowledge/pages/facts/fact-handoff.json", map[string]any{"id": "fact-handoff", "kind": "fact", "status": "verified"})}, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{}}, IdempotencyKey: "codex-handoff-feedback-v1"}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.CreateSubmission(t.Context(), workspaceActor, binding, bundle, "")
	if err != nil {
		t.Fatal(err)
	}
	server, client := codexHandoffServer(t, service, session.ID)
	target := server.URL + "/api/bff/projects/" + project.ID + "/submission-revisions/" + revision.ID + "/codex-handoff"

	response := codexHandoffRequest(t, client, target)
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict || codexHandoffErrorCode(t, response) != "CODEX_HANDOFF_FEEDBACK_REQUIRED" {
		t.Fatalf("revision without feedback must not create handoff: status=%d", response.StatusCode)
	}
	const privateComment = "PRIVATE_REVIEW_COMMENT: replace the customer claim"
	if _, err := service.RequestSubmissionChanges(t.Context(), actor, revision.ID, privateComment, "/0/status", ""); err != nil {
		t.Fatal(err)
	}
	handoff, raw := readCodexHandoff(t, client, target)
	if handoff.Kind != "review_feedback" || handoff.ProjectID != project.ID || handoff.Target.Kind != "submission_revision" || handoff.Target.ID != revision.ID {
		t.Fatalf("unexpected review handoff: %#v", handoff)
	}
	wantDigest := "sha256:" + strings.TrimPrefix(strings.ToLower(revision.ContentHash), "sha256:")
	if handoff.Target.Digest != wantDigest || !strings.Contains(handoff.Prompt, revision.ID) || !strings.Contains(handoff.Prompt, wantDigest) {
		t.Fatalf("review handoff lost immutable target: %#v", handoff)
	}
	if !strings.Contains(handoff.Prompt, "review_feedback_list") || strings.Contains(handoff.Prompt, "review_feedback_pull") || strings.Contains(handoff.Prompt, "local_run_claim") {
		t.Fatalf("review handoff must remain read-only: %q", handoff.Prompt)
	}
	for _, privateValue := range []string{"PRIVATE_FEEDBACK_BRAND", "PRIVATE_FEEDBACK_PRODUCT", privateComment, "/0/status", "/Users/", "originUrl"} {
		if strings.Contains(raw, privateValue) {
			t.Fatalf("review handoff leaked private value %q: %s", privateValue, raw)
		}
	}

	otherProject, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Other", ProductName: "Project"}, "")
	if err != nil {
		t.Fatal(err)
	}
	otherConnect, err := service.CreateConnectSession(t.Context(), actor, otherProject.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testsupport.ConnectBootstrap(t.Context(), service, actor, otherConnect, app.ConnectDeviceInput{Hostname: "other", Platform: "darwin", Arch: "arm64", Version: "0.8.0"}); err != nil {
		t.Fatal(err)
	}
	crossProject := codexHandoffRequest(t, client, server.URL+"/api/bff/projects/"+otherProject.ID+"/submission-revisions/"+revision.ID+"/codex-handoff")
	defer crossProject.Body.Close()
	if crossProject.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-project revision handoff status=%d, want 404", crossProject.StatusCode)
	}

	foreignSession, err := service.Register(t.Context(), "foreign-handoff@example.com", "long-enough-password", "Foreign", "Other Tenant")
	if err != nil {
		t.Fatal(err)
	}
	_, foreignClient := codexHandoffServerForURL(t, server.URL, foreignSession.ID)
	crossTenant := codexHandoffRequest(t, foreignClient, target)
	defer crossTenant.Body.Close()
	if crossTenant.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-tenant revision handoff status=%d, want 404", crossTenant.StatusCode)
	}
}

func assertProjectCodexHandoff(t *testing.T, handoff codexHandoffResponse, projectID string) {
	t.Helper()
	if handoff.SchemaVersion != "contentcloud.codex-handoff/1.0" || handoff.Kind != "project" || handoff.ProjectID != projectID || handoff.Target.Kind != "project" || handoff.Target.ID != projectID {
		t.Fatalf("unexpected project handoff: %#v", handoff)
	}
	if handoff.PluginID != "contentcloud-video-production@contentcloud" || handoff.PluginVersion != "0.10.0" || !handoff.RequiresNewChat || !handoff.RequiresWorkspaceSelection || handoff.FallbackURL != "/codex" || len(handoff.Steps) != 3 {
		t.Fatalf("project handoff gates are incomplete: %#v", handoff)
	}
	parsed, err := url.Parse(handoff.LaunchURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "codex" || parsed.Host != "new" || parsed.Path != "" || len(parsed.Query()) != 1 || parsed.Query().Get("prompt") != handoff.Prompt {
		t.Fatalf("unsafe Codex deep link %q", handoff.LaunchURL)
	}
	if parsed.Query().Has("path") || parsed.Query().Has("originUrl") || !strings.Contains(handoff.Prompt, "workspace_context") || !strings.Contains(handoff.Prompt, projectID) || !strings.Contains(handoff.Prompt, "plugin://"+handoff.PluginID) {
		t.Fatalf("project handoff prompt or query is incomplete: %#v", handoff)
	}
}

func codexHandoffServer(t *testing.T, service *app.Service, sessionID string) (*httptest.Server, *http.Client) {
	t.Helper()
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	t.Cleanup(server.Close)
	_, client := codexHandoffServerForURL(t, server.URL, sessionID)
	return server, client
}

func codexHandoffServerForURL(t *testing.T, serverURL, sessionID string) (*url.URL, *http.Client) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: sessionID, Path: "/"}})
	return baseURL, &http.Client{Jar: jar}
}

func codexHandoffRequest(t *testing.T, client *http.Client, target string) *http.Response {
	t.Helper()
	response, err := client.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readCodexHandoff(t *testing.T, client *http.Client, target string) (codexHandoffResponse, string) {
	t.Helper()
	response := codexHandoffRequest(t, client, target)
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("handoff status=%d body=%s", response.StatusCode, raw)
	}
	var envelope struct {
		OK   bool                 `json:"ok"`
		Data codexHandoffResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK {
		t.Fatalf("handoff failed: %s", raw)
	}
	return envelope.Data, string(raw)
}

func codexHandoffErrorCode(t *testing.T, response *http.Response) string {
	t.Helper()
	var envelope struct {
		Error *domain.Error `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error == nil {
		t.Fatal("expected domain error")
	}
	return envelope.Error.Code
}
