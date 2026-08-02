package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestAgentClientCatalogExposesPlannedClientsByCapability(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "catalog@example.com", "long-enough-password", "Owner", "Tenant")
	if err != nil {
		t.Fatal(err)
	}
	server, client := codexHandoffServer(t, service, session.ID)
	response := codexHandoffRequest(t, client, server.URL+"/api/bff/agent-clients")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("catalog status=%d", response.StatusCode)
	}
	var envelope struct {
		Data struct {
			SchemaVersion string                          `json:"schema_version"`
			Clients       []agentadapter.ClientDefinition `json:"clients"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.SchemaVersion != "contentcloud.agent-client-catalog/1.0" || len(envelope.Data.Clients) != 6 {
		t.Fatalf("unexpected catalog: %#v", envelope.Data)
	}
	for _, client := range envelope.Data.Clients {
		want := agentadapter.SupportPlanned
		if client.ID == agentadapter.ClientCodex {
			want = agentadapter.SupportAvailable
		}
		if got := client.CapabilityStatus(agentadapter.CapabilityInteractiveHandoff); got != want {
			t.Fatalf("interactive handoff status for %s=%s, want %s", client.ID, got, want)
		}
	}
}

func TestGenericAgentHandoffUsesStrategyAndRejectsPlannedClient(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "agent-handoff@example.com", "long-enough-password", "Owner", "Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Private", ProductName: "Private"}, "")
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
	if _, err := service.RegisterWorkspace(t.Context(), workspaceActor, binding, "workspace_marketing_agent", "3.0.0", []string{"codex-plugin"}, ""); err != nil {
		t.Fatal(err)
	}
	server, client := codexHandoffServer(t, service, session.ID)
	base := server.URL + "/api/bff/projects/" + project.ID + "/agent-handoff"

	response := codexHandoffRequest(t, client, base+"?client=codex")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Codex agent handoff status=%d", response.StatusCode)
	}
	var envelope struct {
		Data agentadapter.Handoff `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.SchemaVersion != agentadapter.HandoffSchemaVersion || envelope.Data.Client.ID != agentadapter.ClientCodex || envelope.Data.Integration.Kind != "plugin" || !strings.HasPrefix(envelope.Data.Launch.URL, "codex://new?") {
		t.Fatalf("unexpected generic handoff: %#v", envelope.Data)
	}

	planned := codexHandoffRequest(t, client, base+"?client=cursor")
	defer planned.Body.Close()
	if planned.StatusCode != http.StatusForbidden || codexHandoffErrorCode(t, planned) != "AGENT_CLIENT_CAPABILITY_UNAVAILABLE" {
		t.Fatalf("planned client was not rejected: status=%d", planned.StatusCode)
	}
	extraQuery := codexHandoffRequest(t, client, base+"?client=codex&path=private")
	defer extraQuery.Body.Close()
	if extraQuery.StatusCode != http.StatusBadRequest || codexHandoffErrorCode(t, extraQuery) != "AGENT_CLIENT_REQUIRED" {
		t.Fatalf("extra handoff query was not rejected: status=%d", extraQuery.StatusCode)
	}
}

func TestGenericReviewFeedbackHandoffBindsRevisionAndTenant(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "agent-review@example.com", "long-enough-password", "Owner", "Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Private", ProductName: "Review"}, "")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.CreateConnectSession(t.Context(), actor, project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, app.ConnectDeviceInput{Hostname: "local", Platform: "darwin", Arch: "arm64", Version: "0.15.0"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor, binding, err := service.WorkspaceActor(t.Context(), connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	binding, err = service.RegisterWorkspace(t.Context(), workspaceActor, binding, "workspace_marketing_agent", "3.0.0", []string{"codex-plugin"}, "")
	if err != nil {
		t.Fatal(err)
	}
	bundle := domain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "knowledge", ProjectID: project.ID, WorkspaceID: binding.ID, BaseSnapshotIDs: []string{}, EnvironmentDigest: httpSubmissionEnvironmentDigest, Objects: []domain.SubmissionObjectRef{mustHTTPSubmissionObject(t, "fact-agent-review", "Fact", "30-knowledge/pages/facts/fact-agent-review.json", map[string]any{"id": "fact-agent-review", "kind": "fact", "status": "verified"})}, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{}}, IdempotencyKey: "agent-review-feedback-v1"}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.CreateSubmission(t.Context(), workspaceActor, binding, bundle, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RequestSubmissionChanges(t.Context(), actor, revision.ID, "revise the claim", "/0/status", ""); err != nil {
		t.Fatal(err)
	}
	server, client := codexHandoffServer(t, service, session.ID)
	base := server.URL + "/api/bff/projects/" + project.ID + "/submission-revisions/" + revision.ID + "/agent-handoff"

	response := codexHandoffRequest(t, client, base+"?client=codex")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("review handoff status=%d", response.StatusCode)
	}
	var envelope struct {
		Data agentadapter.Handoff `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	wantDigest := "sha256:" + strings.TrimPrefix(strings.ToLower(revision.ContentHash), "sha256:")
	if envelope.Data.Kind != "review_feedback" || envelope.Data.Target.ID != revision.ID || envelope.Data.Target.Digest != wantDigest || !strings.Contains(envelope.Data.Prompt, "review_feedback_list") {
		t.Fatalf("review handoff lost immutable target: %#v", envelope.Data)
	}

	planned := codexHandoffRequest(t, client, base+"?client=cursor")
	defer planned.Body.Close()
	if planned.StatusCode != http.StatusForbidden || codexHandoffErrorCode(t, planned) != "AGENT_CLIENT_CAPABILITY_UNAVAILABLE" {
		t.Fatalf("planned review client was not rejected: status=%d", planned.StatusCode)
	}
	foreignSession, err := service.Register(t.Context(), "foreign-agent-review@example.com", "long-enough-password", "Foreign", "Other Tenant")
	if err != nil {
		t.Fatal(err)
	}
	_, foreignClient := codexHandoffServerForURL(t, server.URL, foreignSession.ID)
	foreign := codexHandoffRequest(t, foreignClient, base+"?client=codex")
	defer foreign.Body.Close()
	if foreign.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-tenant review handoff status=%d, want 404", foreign.StatusCode)
	}
}
