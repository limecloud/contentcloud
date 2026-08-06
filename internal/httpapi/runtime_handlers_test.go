package httpapi_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/httpapi"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestRuntimeExplorerBFFShowsOperationsProjection(t *testing.T) {
	service := app.New(memory.New(), nil, app.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	task := callBFF[app.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", app.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "Runtime 观察任务", Goal: "验证运营后台能够观察执行实例"})
	list := callBFF[app.RuntimeJobList](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	if len(list.Items) != 1 || list.Items[0].WorkTaskID != task.Task.ID {
		t.Fatalf("runtime job was not created for studio task: %#v", list)
	}
	detail := callBFF[app.RuntimeJobDetail](t, client, http.MethodGet, server.URL+"/api/bff/admin/runtime/jobs/"+list.Items[0].ID, nil)
	if detail.Summary.PlanDigest == "" || len(detail.Nodes) == 0 || len(detail.Events) == 0 {
		t.Fatalf("runtime detail is incomplete: %#v", detail)
	}
	if detail.Agents == nil {
		t.Fatal("runtime detail must expose an initialized agent projection")
	}
	if _, ok := detail.Events[0].Payload["token"]; ok {
		t.Fatal("runtime event payload leaked a token field")
	}
	callBFF[app.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/admin/runtime/jobs/"+list.Items[0].ID+"/refresh", nil)
}

func TestRuntimeExplorerProjectsAgentContextWithoutSessionReference(t *testing.T) {
	service := app.New(memory.New(), nil, app.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	_ = callBFF[app.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", app.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "Runtime Agent 观察", Goal: "验证智能体投影脱敏"})
	list := callBFF[app.RuntimeJobList](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	initial := callBFF[app.RuntimeJobDetail](t, client, http.MethodGet, server.URL+"/api/bff/admin/runtime/jobs/"+list.Items[0].ID, nil)
	actor, _, err := service.SessionActor(t.Context(), sessionIDFromJar(t, jar, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC()
	view, err := service.Runtime().CreateContextView(t.Context(), contentruntime.ContextViewInput{TenantID: actor.TenantID, JobRunID: list.Items[0].ID, NodeRunID: initial.Nodes[0].ID, AttemptID: "attempt-http-agent", AllowedTools: []string{"state.get"}, MaxTokens: 2048, BudgetMinor: 50, CreatedAt: created, ExpiresAt: created.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Runtime().CreateAgentInstance(t.Context(), contentruntime.AgentInstanceInput{TenantID: actor.TenantID, JobRunID: list.Items[0].ID, NodeRunID: initial.Nodes[0].ID, Role: "supervisor", HarnessKind: "fake", SessionRef: "opaque-session-token", ExecutionProfileID: "profile-http", ContextViewID: view.ID, RemainingDescendants: 1, BudgetMinor: 50}); err != nil {
		t.Fatal(err)
	}
	detail := callBFF[app.RuntimeJobDetail](t, client, http.MethodGet, server.URL+"/api/bff/admin/runtime/jobs/"+list.Items[0].ID, nil)
	if len(detail.Agents) != 1 || !detail.Agents[0].SessionBound || detail.Agents[0].ContextView.AllowedTools[0] != "state.get" {
		t.Fatalf("agent projection is incomplete: %#v", detail.Agents)
	}
	response, err := client.Get(server.URL + "/api/bff/admin/runtime/jobs/" + list.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "opaque-session-token") || strings.Contains(string(raw), "session_ref") {
		t.Fatalf("runtime projection leaked an opaque session reference: %s", raw)
	}
}

func TestRuntimeEventsStreamResumesFromLastEventID(t *testing.T) {
	service := app.New(memory.New(), nil, app.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	_ = callBFF[app.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", app.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "Runtime SSE", Goal: "验证事件增量"})
	list := callBFF[app.RuntimeJobList](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	if len(list.Items) != 1 {
		t.Fatalf("runtime job was not created: %#v", list)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+list.Items[0].ID+"/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Last-Event-ID", "1")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("unexpected SSE response: status=%d content-type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(response.Body)
	body := strings.Builder{}
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatal(readErr)
		}
		body.WriteString(line)
		if strings.Contains(line, "job.admitted") {
			break
		}
	}
	cancel()
	if !strings.Contains(body.String(), "id: 2\n") || strings.Contains(body.String(), "job.created") {
		t.Fatalf("unexpected resumed runtime SSE payload: %q", body.String())
	}
}
