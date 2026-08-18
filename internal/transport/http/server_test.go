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
	"sync"
	"testing"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestDevBootstrapAndDashboard(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	resp, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status %d", resp.StatusCode)
	}
	resp, err = client.Get(server.URL + "/api/bff/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var value struct {
		OK   bool `json:"ok"`
		Data struct {
			Projects []any `json:"projects"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	if !value.OK || len(value.Data.Projects) != 0 {
		t.Fatalf("unexpected dashboard %#v", value)
	}
}

func TestBFFRequiresSession(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/api/bff/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestSessionCookieSecurityFollowsRequestScheme(t *testing.T) {
	tests := []struct {
		name           string
		forwardedProto string
		expectedSecure bool
	}{
		{name: "plain HTTP", expectedSecure: false},
		{name: "HTTPS reverse proxy", forwardedProto: "https", expectedSecure: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
			server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
			defer server.Close()
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/auth/register", strings.NewReader(`{"email":"cookie@example.com","password":"long-enough-password","display_name":"Cookie User","tenant_name":"Cookie Tenant"}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			if test.forwardedProto != "" {
				request.Header.Set("X-Forwarded-Proto", test.forwardedProto)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("registration status %d", response.StatusCode)
			}
			cookies := response.Cookies()
			if len(cookies) != 1 || cookies[0].Name != "cc_session" {
				t.Fatalf("unexpected session cookies %#v", cookies)
			}
			if cookies[0].Secure != test.expectedSecure {
				t.Fatalf("Secure=%v, want %v", cookies[0].Secure, test.expectedSecure)
			}
		})
	}
}

func TestPlatformAdminEndpointsRequireExplicitGrant(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("development bootstrap status=%d body=%s", response.StatusCode, body)
	}
	response.Body.Close()
	response, err = client.Get(server.URL + "/api/v1/admin/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected explicit platform grant to be required, got %d", response.StatusCode)
	}
}

func TestPlatformAdminOverviewAndTenantStatusEndpoint(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("demo@contentcloud.local"))
	targetSession, err := service.Identity.Register(t.Context(), "customer@example.com", "long-enough-password", "Customer", "Customer Tenant")
	if err != nil {
		t.Fatal(err)
	}
	targetActor, _, err := service.Identity.SessionActor(t.Context(), targetSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("development bootstrap status=%d body=%s", response.StatusCode, body)
	}
	var bootstrap struct {
		OK   bool `json:"ok"`
		Data struct {
			MarketingVideoFixture application.MarketingVideoDemoFixtureResult `json:"marketing_video_fixture"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&bootstrap); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if !bootstrap.OK || bootstrap.Data.MarketingVideoFixture.Project.KnowledgeReady != 4 {
		t.Fatalf("initial development bootstrap knowledge projection is incomplete: %#v", bootstrap)
	}
	executors := callBFF[application.OperationsExecutorDirectory](t, client, http.MethodGet, server.URL+"/api/bff/operations/executors", nil)
	if len(executors.Executors) != 1 {
		t.Fatalf("development bootstrap did not create an executor current-state fixture: %#v", executors)
	}
	executor := executors.Executors[0]
	if executor.PresenceStatus != "online" || executor.EnvironmentStatus != "ready" || executor.EnvironmentReason != "development_fixture" || executor.RuntimeStatus != "healthy" || executor.RuntimeReason != "development_fixture" {
		t.Fatalf("development executor health axes are incomplete: %#v", executor)
	}
	if executor.DaemonInstanceID == "" || executor.ConnectionEpoch != 1 || len(executor.Runtimes) != 2 || !executor.Runtimes[0].Selected || executor.Runtimes[0].Kind != "codex" || executor.Runtimes[0].Version != "codex fixture" || executor.Runtimes[1].Kind != "claude" || executor.Runtimes[1].ErrorCode != "CLAUDE_AUTH_REQUIRED" {
		t.Fatalf("development executor Runtime inventory is incomplete: %#v", executor)
	}
	if len(executor.Workspaces) != 1 || executor.Workspaces[0].ProjectID != bootstrap.Data.MarketingVideoFixture.Project.ID || executor.Workspaces[0].WorkspaceID == "" || executor.Workspaces[0].Status != "ready" || executor.Workspaces[0].Generation != "sha256:development-workspace-generation" {
		t.Fatalf("development executor Workspace inventory is incomplete: %#v", executor.Workspaces)
	}
	demoProjects := callBFF[[]workspacedomain.Project](t, client, http.MethodGet, server.URL+"/api/bff/projects", nil)
	if len(demoProjects) != 1 || demoProjects[0].ContentType != identitydomain.ContentTypeMarketingVideo || demoProjects[0].ConnectedDevices != 1 || demoProjects[0].KnowledgeReady != 4 {
		t.Fatalf("development bootstrap did not create the marketing video project: %#v", demoProjects)
	}
	dashboard := callBFF[application.Dashboard](t, client, http.MethodGet, server.URL+"/api/bff/dashboard", nil)
	if dashboard.Counts["knowledge_ready"] != 4 || len(dashboard.Projects) != 1 || dashboard.Projects[0].KnowledgeReady != 4 {
		t.Fatalf("development dashboard knowledge projection is incomplete: %#v", dashboard)
	}
	demoTasks := callBFF[[]work.WorkTask](t, client, http.MethodGet, server.URL+"/api/bff/tasks?project_id="+demoProjects[0].ID, nil)
	if len(demoTasks) != 1 || demoTasks[0].Status != work.TaskStatusDelivered {
		t.Fatalf("development bootstrap did not deliver the marketing-video task: %#v", demoTasks)
	}
	demoTask := callBFF[application.WorkTaskView](t, client, http.MethodGet, server.URL+"/api/bff/tasks/"+demoTasks[0].ID, nil)
	if len(demoTask.SourceRevisions) != 1 || len(demoTask.KnowledgeSnapshots) != 1 || len(demoTask.KnowledgeSnapshots[0].Objects) != 4 || len(demoTask.Revisions) != 1 || len(demoTask.MediaJobs) != 1 || len(demoTask.ProviderAttempts) != 1 || len(demoTask.MediaReviews) != 3 || len(demoTask.DeliveryPackages) != 1 || len(demoTask.Deliveries) != 1 {
		t.Fatalf("development marketing-video fixture is incomplete: %#v", demoTask)
	}
	response, err = client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	refreshedExecutors := callBFF[application.OperationsExecutorDirectory](t, client, http.MethodGet, server.URL+"/api/bff/operations/executors", nil)
	if len(refreshedExecutors.Executors) != 1 || refreshedExecutors.Executors[0].DaemonInstanceID != executor.DaemonInstanceID || refreshedExecutors.Executors[0].PresenceStatus != "online" {
		t.Fatalf("development executor current-state fixture is not idempotent: %#v", refreshedExecutors)
	}
	demoProjects = callBFF[[]workspacedomain.Project](t, client, http.MethodGet, server.URL+"/api/bff/projects", nil)
	demoTasks = callBFF[[]work.WorkTask](t, client, http.MethodGet, server.URL+"/api/bff/tasks?project_id="+demoProjects[0].ID, nil)
	if len(demoProjects) != 1 || len(demoTasks) != 1 || demoTasks[0].ID != demoTask.Task.ID {
		t.Fatalf("development marketing-video fixture is not idempotent: projects=%#v tasks=%#v", demoProjects, demoTasks)
	}
	overview := callBFF[identitydomain.PlatformOverview](t, client, http.MethodGet, server.URL+"/api/v1/admin/dashboard", nil)
	if overview.Counts.Tenants != 2 || overview.Counts.Users != 2 {
		t.Fatalf("unexpected platform overview %#v", overview.Counts)
	}
	capabilityTenant := callBFF[identitydomain.PlatformTenant](t, client, http.MethodPut, server.URL+"/api/v1/admin/tenants/"+targetActor.TenantID+"/content-capabilities/"+identitydomain.ContentTypeWeChatArticle, map[string]bool{"enabled": true})
	if len(capabilityTenant.ContentTypes) != 2 || capabilityTenant.ContentTypes[1] != identitydomain.ContentTypeWeChatArticle {
		t.Fatalf("unexpected tenant content capabilities %#v", capabilityTenant.ContentTypes)
	}
	tenant := callBFF[identitydomain.Tenant](t, client, http.MethodPatch, server.URL+"/api/v1/admin/tenants/"+targetActor.TenantID, map[string]string{"status": "suspended"})
	if tenant.Status != "suspended" {
		t.Fatalf("unexpected tenant status %#v", tenant)
	}
	if _, _, err := service.Identity.SessionActor(t.Context(), targetSession.ID); err == nil {
		t.Fatal("tenant status endpoint did not revoke active sessions")
	}
}

func TestDevelopmentBootstrapIsConcurrentIdempotent(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()

	const workers = 6
	start := make(chan struct{})
	results := make(chan struct {
		status int
		err    error
	}, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			response, err := http.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
			if err != nil {
				results <- struct {
					status int
					err    error
				}{err: err}
				return
			}
			response.Body.Close()
			results <- struct {
				status int
				err    error
			}{status: response.StatusCode}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			t.Fatalf("development bootstrap failed: %v", result.err)
		}
		if result.status != http.StatusOK {
			t.Fatalf("development bootstrap status=%d", result.status)
		}
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("development bootstrap status=%d", response.StatusCode)
	}
	projects := callBFF[[]workspacedomain.Project](t, client, http.MethodGet, server.URL+"/api/bff/projects", nil)
	if len(projects) != 1 || projects[0].ConnectedDevices != 1 {
		t.Fatalf("concurrent bootstrap created %d projects: %#v", len(projects), projects)
	}
	tasks := callBFF[[]work.WorkTask](t, client, http.MethodGet, server.URL+"/api/bff/tasks?project_id="+projects[0].ID, nil)
	if len(tasks) != 1 || tasks[0].Status != work.TaskStatusDelivered {
		t.Fatalf("concurrent bootstrap created unexpected tasks: %#v", tasks)
	}
}

func TestBFFTeamProjectAndConnectionOperations(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status %d", response.StatusCode)
	}

	members := callBFF[[]application.MemberView](t, client, http.MethodGet, server.URL+"/api/bff/team/members", nil)
	if len(members) != 1 || members[0].Membership.Role != "tenant_admin" {
		t.Fatalf("unexpected members %#v", members)
	}
	invite := callBFF[identitydomain.MembershipInvite](t, client, http.MethodPost, server.URL+"/api/bff/team/invites", map[string]string{"email": "new-member@example.com", "role": "editor"})
	if invite.PlaintextToken == "" || invite.Status != "pending" {
		t.Fatalf("unexpected invitation %#v", invite)
	}
	invite = callBFF[identitydomain.MembershipInvite](t, client, http.MethodPost, server.URL+"/api/bff/team/invites/"+invite.ID+"/revoke", map[string]any{})
	if invite.Status != "revoked" {
		t.Fatalf("invite was not revoked %#v", invite)
	}

	builtinTemplates := callBFF[[]workspacedomain.ProjectTemplate](t, client, http.MethodGet, server.URL+"/api/bff/project-templates", nil)
	if len(builtinTemplates) != 3 {
		t.Fatalf("expected builtin project templates, got %#v", builtinTemplates)
	}
	template := callBFF[workspacedomain.ProjectTemplate](t, client, http.MethodPost, server.URL+"/api/bff/project-templates", application.CreateProjectTemplateInput{Name: "抖音验证", Channel: "douyin", StageObjective: "验证主卖点"})
	if template.Name != "抖音验证" {
		t.Fatalf("unexpected template %#v", template)
	}
	created := callBFF[workspacedomain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", application.CreateProjectInput{BrandName: "测试品牌", ProductName: "测试产品", TemplateID: template.ID})
	if created.ID == "" {
		t.Fatalf("project was not created: %#v", created)
	}
	projects := callBFF[[]workspacedomain.Project](t, client, http.MethodGet, server.URL+"/api/bff/projects", nil)
	if len(projects) != 1 {
		t.Fatalf("unexpected projects %#v", projects)
	}
	project := projects[0]
	project = callBFF[workspacedomain.Project](t, client, http.MethodPatch, server.URL+"/api/bff/projects/"+project.ID, map[string]any{"row_version": project.RowVersion, "owner_name": "项目负责人"})
	if project.OwnerName != "项目负责人" || project.RowVersion != projects[0].RowVersion+1 {
		t.Fatalf("unexpected project update %#v", project)
	}
	connect := callBFF[workspacedomain.ConnectSession](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/connect-sessions", map[string]any{})
	if connect.ID == "" || connect.State != "waiting_for_computer" || connect.Progress != nil {
		t.Fatalf("unexpected connect session %#v", connect)
	}
	connect = callBFF[workspacedomain.ConnectSession](t, client, http.MethodPost, server.URL+"/api/bff/connect-sessions/"+connect.ID+"/cancel", map[string]any{})
	if connect.State != "canceled" {
		t.Fatalf("connect session was not canceled %#v", connect)
	}
	project = callBFF[workspacedomain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/archive", map[string]any{"row_version": project.RowVersion})
	if project.Status != "archived" {
		t.Fatalf("project was not archived %#v", project)
	}
	project = callBFF[workspacedomain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/restore", map[string]any{"row_version": project.RowVersion})
	if project.Status != "active" {
		t.Fatalf("project was not restored %#v", project)
	}
}

func TestLegacyBusinessWriteRoutesAreNotExposed(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(t.Context(), "legacy-routes@example.com", "long-enough-password", "Owner", "V3 Tenant")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}

	paths := []string{
		"/api/bff/projects/project-1/sources",
		"/api/bff/projects/project-1/assets",
		"/api/bff/projects/project-1/briefs",
		"/api/bff/projects/project-1/scripts",
		"/api/bff/projects/project-1/results",
		"/api/bff/projects/project-1/rating-decisions",
		"/api/bff/approved-snapshots/snapshot-1/exports",
		"/api/bff/approved-snapshots/snapshot-1/delivery-packages",
	}
	for _, route := range paths {
		request, requestErr := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+route, strings.NewReader("{}"))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		request.Header.Set("Content-Type", "application/json")
		response, requestErr := client.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("legacy route %s returned %d, want 404", route, response.StatusCode)
		}
	}
}

func callBFF[T any](t *testing.T, client *http.Client, method, target string, input any) T {
	return callBFFWithHeaders[T](t, client, method, target, input, nil)
}

func callBFFWithHeaders[T any](t *testing.T, client *http.Client, method, target string, input any, headers map[string]string) T {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		OK    bool         `json:"ok"`
		Data  T            `json:"data"`
		Error *fault.Error `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !envelope.OK {
		t.Fatalf("%s %s failed: status=%d error=%#v", method, target, response.StatusCode, envelope.Error)
	}
	return envelope.Data
}
