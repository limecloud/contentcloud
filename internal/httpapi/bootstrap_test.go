package httpapi_test

import (
	"bytes"
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
)

func TestBootstrapDocumentIsPublicAndAgentReady(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/api/bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status = %d, want 200", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}
	document := string(body)
	for _, required := range []string{"connect-key", "@limecloud/contentcloud@latest", "init .", "workspace doctor", "must not upload existing files"} {
		if !strings.Contains(document, required) {
			t.Fatalf("bootstrap document is missing %q", required)
		}
	}
	if strings.HasPrefix(document, "---") {
		t.Fatal("bootstrap document must not be parsed as a project Skill")
	}
}

func TestConnectSessionHTTPStateTracksWorkspaceInitialization(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "http-connect@example.com", "long-enough-password", "HTTP Connect", "HTTP Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: "douyin"}, "http-connect-project")
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}

	connect := callBFF[domain.ConnectSession](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/connect-sessions", map[string]any{})
	device := callDispatch[app.ConnectDeviceResult](t, client, server.URL, "", "device.connect", app.ConnectDeviceInput{ConnectKey: connect.PlaintextConnectKey, Hostname: "http-connect-mac", Platform: "darwin", Arch: "arm64", Version: "test"})
	status := callBFF[domain.ConnectSession](t, client, http.MethodGet, server.URL+"/api/bff/connect-sessions/"+connect.ID, nil)
	if status.State != "verifying" {
		t.Fatalf("HTTP state after device connection = %q, want verifying", status.State)
	}

	callDispatch[domain.WorkspaceBinding](t, client, server.URL, device.WorkspaceToken, "workspace.register", map[string]any{"template_id": "workspace_marketing_video", "template_version": "2.0.0", "targets": []string{"codex"}})
	status = callBFF[domain.ConnectSession](t, client, http.MethodGet, server.URL+"/api/bff/connect-sessions/"+connect.ID, nil)
	if status.State != "connected" {
		t.Fatalf("HTTP state after workspace registration = %q, want connected", status.State)
	}
}

func callDispatch[T any](t *testing.T, client *http.Client, serverURL, token, command string, params any) T {
	t.Helper()
	requestBody, err := json.Marshal(map[string]any{"command": command, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, serverURL+"/api/v1/cli/dispatch", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		OK    bool          `json:"ok"`
		Data  T             `json:"data"`
		Error *domain.Error `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !envelope.OK {
		t.Fatalf("dispatch %s failed: status=%d error=%#v", command, response.StatusCode, envelope.Error)
	}
	return envelope.Data
}
