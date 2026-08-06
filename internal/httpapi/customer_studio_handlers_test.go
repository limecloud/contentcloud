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
)

func TestCustomerStudioProjectionAndTenantIsolation(t *testing.T) {
	service := app.New(memory.New(), slog.Default(), app.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	clientJar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: clientJar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	if len(bootstrap.Experiences) != 1 || len(bootstrap.Projects) == 0 {
		t.Fatalf("expected one published customer experience, got %#v", bootstrap)
	}
	if bootstrap.Experiences[0].ProjectIDs[0] != bootstrap.Projects[0].ID {
		t.Fatalf("experience is not bound to returned project: %#v", bootstrap.Experiences[0])
	}

	raw := getStudioRaw(t, client, server.URL+"/api/studio/bootstrap")
	for _, forbidden := range []string{`"tenant_id"`, `"sop_id"`, `"sop_digest"`, `"environment_id"`, `"stage_runs"`, `"executor_kind"`, `"capability_id"`, `"checks"`} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("customer bootstrap leaked internal field %s: %s", forbidden, raw)
		}
	}

	input := app.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "客户 Studio 幂等任务", Goal: "验证客户只看到业务目标和可复用资产", AssetRefs: []string{}}
	first := callBFFWithHeaders[app.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", input, map[string]string{"Idempotency-Key": "studio-idempotent-1"})
	second := callBFFWithHeaders[app.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", input, map[string]string{"Idempotency-Key": "studio-idempotent-1"})
	if first.Task.ID == "" || first.Task.ID != second.Task.ID {
		t.Fatalf("studio create is not idempotent: first=%s second=%s", first.Task.ID, second.Task.ID)
	}
	rawTask := getStudioRaw(t, client, server.URL+"/api/studio/tasks/"+first.Task.ID)
	for _, forbidden := range []string{`"tenant_id"`, `"sop_id"`, `"sop_digest"`, `"environment_id"`, `"stage_runs"`, `"runs"`, `"executor_kind"`, `"capability_id"`, `"checks"`} {
		if strings.Contains(rawTask, forbidden) {
			t.Fatalf("customer task leaked internal field %s: %s", forbidden, rawTask)
		}
	}

	foreignSession, err := service.Register(t.Context(), "studio-foreign@example.com", "long-enough-password", "外部客户", "外部团队")
	if err != nil {
		t.Fatal(err)
	}
	foreignJar, _ := cookiejar.New(nil)
	foreignBase, _ := url.Parse(server.URL)
	foreignJar.SetCookies(foreignBase, []*http.Cookie{{Name: "cc_session", Value: foreignSession.ID, Path: "/"}})
	foreignClient := &http.Client{Jar: foreignJar}
	response, err := foreignClient.Get(server.URL + "/api/studio/tasks/" + first.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign tenant could read customer task, status=%d", response.StatusCode)
	}
}

func TestCustomerStudioExperienceRequiresTenantCapability(t *testing.T) {
	service := app.New(memory.New(), slog.Default(), app.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	actor, _, err := service.SessionActor(t.Context(), sessionIDFromJar(t, jar, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdatePlatformTenantContentCapability(t.Context(), actor, actor.TenantID, domain.ContentTypeMarketingVideo, false, "studio-disable"); err != nil {
		t.Fatal(err)
	}
	withoutCapability := callBFF[app.StudioBootstrap](t, client, http.MethodGet, server.URL+"/api/studio/bootstrap", nil)
	if len(withoutCapability.Experiences) != 0 {
		t.Fatalf("disabled tenant capability still exposed experiences: %#v", withoutCapability.Experiences)
	}
	if len(bootstrap.Experiences) == 0 {
		t.Fatal("fixture did not create a published experience")
	}
}

func TestCustomerStudioHonorsCustomerRole(t *testing.T) {
	service := app.New(memory.New(), slog.Default(), app.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}
	bootstrap := mustStudioBootstrap(t, adminClient, server.URL)
	adminActor, _, err := service.SessionActor(t.Context(), sessionIDFromJar(t, adminJar, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	invite, err := service.CreateMembershipInvite(t.Context(), adminActor, "studio-viewer@example.com", "viewer", "studio-viewer")
	if err != nil {
		t.Fatal(err)
	}
	viewerSession, err := service.RegisterWithInvite(t.Context(), "studio-viewer@example.com", "long-enough-password", "只读客户", invite.PlaintextToken)
	if err != nil {
		t.Fatal(err)
	}
	viewerJar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	viewerJar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: viewerSession.ID, Path: "/"}})
	viewerClient := &http.Client{Jar: viewerJar}
	viewerBootstrap := callBFF[app.StudioBootstrap](t, viewerClient, http.MethodGet, server.URL+"/api/studio/bootstrap", nil)
	if viewerBootstrap.Session.CanCreate {
		t.Fatalf("viewer unexpectedly received create permission: %#v", viewerBootstrap.Session)
	}
	payload := `{"experience_id":"` + bootstrap.Experiences[0].ID + `","project_id":"` + bootstrap.Projects[0].ID + `","title":"只读不应创建","goal":"验证角色边界","inspiration":"","asset_refs":[]}`
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/studio/tasks", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := viewerClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer create status=%d, want 403", response.StatusCode)
	}
}

func mustStudioBootstrap(t *testing.T, client *http.Client, baseURL string) app.StudioBootstrap {
	t.Helper()
	response, err := client.Post(baseURL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("development bootstrap status=%d", response.StatusCode)
	}
	return callBFF[app.StudioBootstrap](t, client, http.MethodGet, baseURL+"/api/studio/bootstrap", nil)
}

func getStudioRaw(t *testing.T, client *http.Client, target string) string {
	t.Helper()
	response, err := client.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %d: %s", target, response.StatusCode, body)
	}
	var envelope struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || !envelope.OK {
		t.Fatalf("invalid studio envelope: %s", body)
	}
	return string(body)
}

func sessionIDFromJar(t *testing.T, jar http.CookieJar, baseURL string) string {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range jar.Cookies(parsed) {
		if cookie.Name == "cc_session" {
			return cookie.Value
		}
	}
	t.Fatal("session cookie missing")
	return ""
}
