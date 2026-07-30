package httpapi_test

import (
	"bytes"
	"encoding/json"
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

func TestDevBootstrapAndDashboard(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
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
	service := app.New(memory.New(), slog.Default())
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
			service := app.New(memory.New(), slog.Default())
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
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
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
	service := app.New(memory.New(), slog.Default(), app.WithPlatformAdminEmails("demo@contentcloud.local"))
	targetSession, err := service.Register(t.Context(), "customer@example.com", "long-enough-password", "Customer", "Customer Tenant")
	if err != nil {
		t.Fatal(err)
	}
	targetActor, _, err := service.SessionActor(t.Context(), targetSession.ID)
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
	response.Body.Close()
	overview := callBFF[domain.PlatformOverview](t, client, http.MethodGet, server.URL+"/api/v1/admin/dashboard", nil)
	if overview.Counts.Tenants != 2 || overview.Counts.Users != 2 {
		t.Fatalf("unexpected platform overview %#v", overview.Counts)
	}
	capabilityTenant := callBFF[domain.PlatformTenant](t, client, http.MethodPut, server.URL+"/api/v1/admin/tenants/"+targetActor.TenantID+"/content-capabilities/"+domain.ContentTypeWeChatArticle, map[string]bool{"enabled": true})
	if len(capabilityTenant.ContentTypes) != 2 || capabilityTenant.ContentTypes[1] != domain.ContentTypeWeChatArticle {
		t.Fatalf("unexpected tenant content capabilities %#v", capabilityTenant.ContentTypes)
	}
	tenant := callBFF[domain.Tenant](t, client, http.MethodPatch, server.URL+"/api/v1/admin/tenants/"+targetActor.TenantID, map[string]string{"status": "suspended"})
	if tenant.Status != "suspended" {
		t.Fatalf("unexpected tenant status %#v", tenant)
	}
	if _, _, err := service.SessionActor(t.Context(), targetSession.ID); err == nil {
		t.Fatal("tenant status endpoint did not revoke active sessions")
	}
}

func TestBFFTeamProjectAndConnectionOperations(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
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

	members := callBFF[[]app.MemberView](t, client, http.MethodGet, server.URL+"/api/bff/team/members", nil)
	if len(members) != 1 || members[0].Membership.Role != "tenant_admin" {
		t.Fatalf("unexpected members %#v", members)
	}
	invite := callBFF[domain.MembershipInvite](t, client, http.MethodPost, server.URL+"/api/bff/team/invites", map[string]string{"email": "new-member@example.com", "role": "editor"})
	if invite.PlaintextToken == "" || invite.Status != "pending" {
		t.Fatalf("unexpected invitation %#v", invite)
	}
	invite = callBFF[domain.MembershipInvite](t, client, http.MethodPost, server.URL+"/api/bff/team/invites/"+invite.ID+"/revoke", map[string]any{})
	if invite.Status != "revoked" {
		t.Fatalf("invite was not revoked %#v", invite)
	}

	template := callBFF[domain.ProjectTemplate](t, client, http.MethodPost, server.URL+"/api/bff/project-templates", app.CreateProjectTemplateInput{Name: "抖音验证", Channel: "douyin", StageObjective: "验证主卖点"})
	if template.Name != "抖音验证" {
		t.Fatalf("unexpected template %#v", template)
	}
	created := callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", app.CreateProjectInput{BrandName: "测试品牌", ProductName: "测试产品", TemplateID: template.ID})
	if created.ID == "" {
		t.Fatalf("project was not created: %#v", created)
	}
	projects := callBFF[[]domain.Project](t, client, http.MethodGet, server.URL+"/api/bff/projects", nil)
	if len(projects) != 1 {
		t.Fatalf("unexpected projects %#v", projects)
	}
	project := projects[0]
	project = callBFF[domain.Project](t, client, http.MethodPatch, server.URL+"/api/bff/projects/"+project.ID, map[string]any{"row_version": project.RowVersion, "owner_name": "项目负责人"})
	if project.OwnerName != "项目负责人" || project.RowVersion != projects[0].RowVersion+1 {
		t.Fatalf("unexpected project update %#v", project)
	}
	connect := callBFF[domain.ConnectSession](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/connect-sessions", map[string]any{})
	if connect.ID == "" || connect.State != "waiting_for_computer" || connect.Progress != nil {
		t.Fatalf("unexpected connect session %#v", connect)
	}
	connect = callBFF[domain.ConnectSession](t, client, http.MethodPost, server.URL+"/api/bff/connect-sessions/"+connect.ID+"/cancel", map[string]any{})
	if connect.State != "canceled" {
		t.Fatalf("connect session was not canceled %#v", connect)
	}
	project = callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/archive", map[string]any{"row_version": project.RowVersion})
	if project.Status != "archived" {
		t.Fatalf("project was not archived %#v", project)
	}
	project = callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/restore", map[string]any{"row_version": project.RowVersion})
	if project.Status != "active" {
		t.Fatalf("project was not restored %#v", project)
	}
}

func TestLegacyBusinessWriteRoutesAreNotExposed(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "legacy-routes@example.com", "long-enough-password", "Owner", "V3 Tenant")
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
		t.Fatalf("%s %s failed: status=%d error=%#v", method, target, response.StatusCode, envelope.Error)
	}
	return envelope.Data
}
