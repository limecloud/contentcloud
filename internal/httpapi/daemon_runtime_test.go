package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestDaemonPollEnforcesMinimumVersionWithoutLeasing(t *testing.T) {
	service, actor, connected, project := connectedDaemonTestRuntime(t, app.WithDaemonVersionPolicy("0.10.0", "0.20.0", "https://content.example.com/downloads"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()

	poll := dispatchDevice[app.DaemonPollResponse](t, server.URL, connected.DeviceToken, "daemon.poll", map[string]any{
		"capabilities": []domain.Capability{}, "environments": []app.AutomationEnvironmentClaim{}, "daemon_version": "0.9.0", "wait_ms": 25000,
	})
	if poll.Leased || !poll.Runtime.UpdateRequired || !poll.Runtime.UpdateAvailable || poll.Runtime.MinimumVersion != "0.10.0" || poll.Runtime.LatestVersion != "0.20.0" || poll.Runtime.UpdateURL == "" {
		t.Fatalf("unexpected version-gated poll response: %#v", poll)
	}
	devices, err := service.Devices(t.Context(), actor, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Version != "0.9.0" {
		t.Fatalf("version gate did not persist daemon version: %#v", devices)
	}
}

func TestDaemonPollHonorsShortLongPollDeadline(t *testing.T) {
	service, _, connected, _ := connectedDaemonTestRuntime(t)
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()

	started := time.Now()
	poll := dispatchDevice[app.DaemonPollResponse](t, server.URL, connected.DeviceToken, "daemon.poll", map[string]any{
		"capabilities": []domain.Capability{}, "environments": []app.AutomationEnvironmentClaim{}, "daemon_version": "0.19.0", "wait_ms": 60,
	})
	elapsed := time.Since(started)
	if poll.Leased || poll.PollAfterMS != 5000 {
		t.Fatalf("unexpected idle poll response: %#v", poll)
	}
	if elapsed < 45*time.Millisecond || elapsed > 350*time.Millisecond {
		t.Fatalf("long poll elapsed %s, expected deadline-bound wait", elapsed)
	}
}

func TestRunProgressStreamResumesAfterLastEventID(t *testing.T) {
	store := memory.New()
	service := app.New(store, slog.Default())
	session, err := service.Register(t.Context(), "sse@example.com", "long-enough-password", "SSE User", "SSE Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	run := domain.TaskRun{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, State: "running", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateRun(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	for sequence := 1; sequence <= 2; sequence++ {
		if _, err := store.AppendRunProgress(t.Context(), domain.RunProgressEvent{TenantID: actor.TenantID, ProjectID: project.ID, RunID: run.ID, AttemptID: "attempt-1", DeviceID: "device-1", Sequence: sequence, Phase: "executing", Step: sequence, Label: "step", OccurredAt: now.Add(time.Duration(sequence) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	client := sessionHTTPClient(t, server.URL, session.ID)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/bff/runs/"+run.ID+"/progress/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Last-Event-ID", "1")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("unexpected SSE response: status=%d content-type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}

	reader := bufio.NewReader(response.Body)
	lines := []string{}
	for len(lines) < 12 {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatal(readErr)
		}
		lines = append(lines, line)
		if strings.Contains(line, `"sequence":2`) {
			break
		}
	}
	cancel()
	body := strings.Join(lines, "")
	if !strings.Contains(body, "id: 2\n") || !strings.Contains(body, `"sequence":2`) || strings.Contains(body, `"sequence":1`) {
		t.Fatalf("SSE resume payload = %q", body)
	}
}

func connectedDaemonTestRuntime(t *testing.T, options ...app.Option) (*app.Service, app.Actor, app.ConnectDeviceResult, domain.Project) {
	t.Helper()
	service := app.New(memory.New(), slog.Default(), options...)
	session, err := service.Register(t.Context(), "daemon@example.com", "long-enough-password", "Daemon User", "Daemon Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.CreateConnectSession(t.Context(), actor, project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, app.ConnectDeviceInput{Hostname: "daemon.local", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return service, actor, connected, project
}

func dispatchDevice[T any](t *testing.T, serverURL, token, command string, params any) T {
	t.Helper()
	body, err := json.Marshal(map[string]any{"command": command, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, serverURL+"/api/v1/cli/dispatch", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
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

func sessionHTTPClient(t *testing.T, serverURL, sessionID string) *http.Client {
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
	return &http.Client{Jar: jar}
}
