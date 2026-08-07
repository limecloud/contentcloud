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
	if !bootstrap.Projects[0].ExecutionClientConnected || bootstrap.Projects[0].ConnectedClientCount == 0 {
		t.Fatalf("customer bootstrap did not expose the connected execution client state: %#v", bootstrap.Projects[0])
	}
	if got, want := strings.Join(bootstrap.Experiences[0].StepTitles, ","), "灵感采集,人物原型,营销剧本,视频分镜,候选成片,交付准备"; got != want {
		t.Fatalf("customer experience leaked runtime stage names: got %q want %q", got, want)
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

func TestCustomerStudioAssetSurfaceSeparatesWorkspaceMaterialsAndCreativeResults(t *testing.T) {
	service := app.New(memory.New(), slog.Default(), app.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	clientJar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: clientJar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)

	surface := callBFF[app.CustomerAssetSurface](t, client, http.MethodGet, server.URL+"/api/studio/assets", nil)
	if len(surface.CreativeResults.Items) == 0 {
		t.Fatal("development fixture should expose generated customer assets")
	}
	if len(surface.Workspace.Materials) == 0 || len(surface.Workspace.Folders) == 0 {
		t.Fatalf("development fixture should expose an explicit workspace material: %#v", surface.Workspace)
	}
	initialMaterialCount, initialFolderCount := len(surface.Workspace.Materials), len(surface.Workspace.Folders)
	allowedResultTypes := map[string]bool{"persona": true, "script": true, "storyboard": true, "image": true, "video": true}
	foundResultTypes := map[string]bool{}
	for _, item := range surface.CreativeResults.Items {
		if !allowedResultTypes[item.ResultType] {
			t.Fatalf("customer result exposed non-result type %q: %#v", item.ResultType, item)
		}
		if item.TaskID == "" || item.TaskTitle == "" {
			t.Fatalf("customer asset lost its originating task: %#v", item)
		}
		foundResultTypes[item.ResultType] = true
	}
	for _, required := range []string{"persona", "script", "storyboard", "image", "video"} {
		if !foundResultTypes[required] {
			t.Fatalf("development fixture did not expose %s result assets: %#v", required, foundResultTypes)
		}
	}
	raw := getStudioRaw(t, client, server.URL+"/api/studio/assets")
	for _, forbidden := range []string{`"tenant_id"`, `"source_revision_id"`, `"object_key"`, `"kind":"source"`, `"kind":"knowledge"`, `source_revision:`, `"rights_record"`, `"source_type":"manual_inspiration"`} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("customer asset surface leaked an internal or governance object %s: %s", forbidden, raw)
		}
	}

	folder := callBFF[app.WorkspaceFolderItem](t, client, http.MethodPost, server.URL+"/api/studio/asset-folders", app.CreateWorkspaceFolderInput{ProjectID: bootstrap.Projects[0].ID, Name: "品牌资料"})
	material := callMultipartBFF[app.WorkspaceMaterialItem](t, client, server.URL+"/api/studio/materials", "brand-brief.txt", []byte("品牌语气与产品卖点"), map[string]string{"project_id": bootstrap.Projects[0].ID, "folder_ref": folder.Ref, "file_type": "text/plain"})
	if material.MaterialKind != domain.WorkspaceMaterialDocument || material.FolderRef != folder.Ref || material.ProjectID != bootstrap.Projects[0].ID {
		t.Fatalf("unexpected workspace material projection: %#v", material)
	}
	withMaterial := callBFF[app.CustomerAssetSurface](t, client, http.MethodGet, server.URL+"/api/studio/assets", nil)
	if len(withMaterial.Workspace.Materials) != initialMaterialCount+1 || len(withMaterial.Workspace.Folders) != initialFolderCount+1 || len(withMaterial.CreativeResults.Items) != len(surface.CreativeResults.Items) {
		t.Fatalf("workspace material changed the creative result projection: %#v", withMaterial)
	}
	task := callBFF[app.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", app.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "资料加入创作", Goal: "验证固定版本工作区资料可以加入任务", AssetRefs: []string{}, MaterialRefs: []string{}})
	callBFF[app.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks/"+task.Task.ID+"/materials", app.StudioAttachMaterialsInput{MaterialRefs: []string{material.Ref}})
	recent := callBFF[app.CustomerAssetSurface](t, client, http.MethodGet, server.URL+"/api/studio/assets", nil)
	if len(recent.Recent.Materials) == 0 || recent.Recent.Materials[0].Ref != material.Ref || recent.Recent.Materials[0].LastUsedAt == nil {
		t.Fatalf("attached workspace material did not enter the recent projection: %#v", recent.Recent.Materials)
	}
}

func TestCustomerStudioInspirationUsesProjectReferenceBoundary(t *testing.T) {
	service := app.New(memory.New(), slog.Default(), app.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	task := callBFF[app.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", app.StudioCreateTaskInput{
		ExperienceID: bootstrap.Experiences[0].ID,
		ProjectID:    bootstrap.Projects[0].ID,
		Title:        "项目参考边界回归",
		Goal:         "验证灵感不进入结果资产库",
		AssetRefs:    []string{},
	})

	withReference := callBFF[app.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks/"+task.Task.ID+"/inspirations", app.StudioAddInspirationInput{
		Title:                  "保留的观察",
		Body:                   "只作为后续任务的项目参考。",
		KeepAsProjectReference: true,
	})
	var projectReference *app.StudioInspiration
	for index := range withReference.Inspirations {
		if withReference.Inspirations[index].Title == "保留的观察" {
			projectReference = &withReference.Inspirations[index]
			break
		}
	}
	if projectReference == nil || !projectReference.SavedAsProjectReference {
		t.Fatalf("project reference flag was not projected: %#v", withReference.Inspirations)
	}
	rawTask := getStudioRaw(t, client, server.URL+"/api/studio/tasks/"+task.Task.ID)
	if !strings.Contains(rawTask, `"saved_as_project_reference":true`) || strings.Contains(rawTask, `"saved_for_reuse"`) {
		t.Fatalf("customer task exposed the wrong inspiration contract: %s", rawTask)
	}
	rawAssets := getStudioRaw(t, client, server.URL+"/api/studio/assets")
	if strings.Contains(rawAssets, "保留的观察") || strings.Contains(rawAssets, `"saved_for_reuse"`) {
		t.Fatalf("project reference leaked into result asset catalog: %s", rawAssets)
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

func TestCustomerStudioExecutionClientHTTPFlow(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "studio-connect@example.com", "long-enough-password", "Studio Connect", "Studio Connect Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "连接品牌", ProductName: "连接产品", Channel: "douyin"}, "studio-connect-project")
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}

	catalog := callBFF[struct {
		Clients []struct {
			ID        string `json:"id"`
			Available bool   `json:"available"`
		} `json:"clients"`
	}](t, client, http.MethodGet, server.URL+"/api/studio/execution-clients", nil)
	if len(catalog.Clients) == 0 || catalog.Clients[0].ID != "codex" || !catalog.Clients[0].Available {
		t.Fatalf("unexpected execution client catalog: %#v", catalog)
	}
	for _, client := range catalog.Clients {
		if client.ID != "codex" && client.Available {
			t.Fatalf("customer bootstrap exposed an unsupported connection client: %#v", client)
		}
	}

	connect := callBFF[app.StudioConnectSession](t, client, http.MethodPost, server.URL+"/api/studio/projects/"+project.ID+"/connect-sessions", map[string]any{})
	if connect.ProjectID != project.ID || connect.Status != "waiting_for_computer" {
		t.Fatalf("unexpected created connection session: %#v", connect)
	}
	waiting := callBFF[app.StudioConnectSession](t, client, http.MethodGet, server.URL+"/api/studio/connect-sessions/"+connect.ID, nil)
	if waiting.Status != "waiting_for_computer" || waiting.RequiresConfirmation {
		t.Fatalf("unexpected initial connection status: %#v", waiting)
	}

	started := callDispatch[app.StartBootstrapAuthorizationResult](t, client, server.URL, "", "bootstrap.authorization.start", app.StartBootstrapAuthorizationInput{
		SessionID: connect.ID, CodeChallenge: strings.Repeat("a", 43), Platform: "darwin", Arch: "arm64", CLIVersion: "test",
	})
	confirmation := callBFF[app.StudioConnectSession](t, client, http.MethodGet, server.URL+"/api/studio/connect-sessions/"+connect.ID, nil)
	if confirmation.Status != "confirmation_required" || !confirmation.RequiresConfirmation || confirmation.VerificationCode == "" {
		t.Fatalf("client confirmation was not projected: %#v", confirmation)
	}
	approved := callBFF[app.StudioConnectSession](t, client, http.MethodPost, server.URL+"/api/studio/connect-sessions/"+connect.ID+"/approve", map[string]any{})
	if approved.Status != "connecting" || approved.RequiresConfirmation {
		t.Fatalf("approved connection status = %#v", approved)
	}
	if started.AttemptID == "" {
		t.Fatal("bootstrap authorization did not return an attempt")
	}

	cancelSession := callBFF[app.StudioConnectSession](t, client, http.MethodPost, server.URL+"/api/studio/projects/"+project.ID+"/connect-sessions", map[string]any{})
	canceled := callBFF[app.StudioConnectSession](t, client, http.MethodPost, server.URL+"/api/studio/connect-sessions/"+cancelSession.ID+"/cancel", map[string]any{})
	if canceled.Status != "canceled" || canceled.ProjectID != project.ID {
		t.Fatalf("canceled connection status = %#v", canceled)
	}
}

func TestCustomerStudioTaskRequiresExecutionClient(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "studio-task-gate@example.com", "long-enough-password", "Studio Task Gate", "Studio Task Gate Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "未连接品牌", ProductName: "未连接产品", Channel: "douyin"}, "studio-task-gate-project")
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}
	status, code := callStudioError(t, client, http.MethodPost, server.URL+"/api/studio/tasks", app.StudioCreateTaskInput{
		ExperienceID: "not-used-before-connection", ProjectID: project.ID, Title: "未连接任务", Goal: "验证连接门禁",
	})
	if status != http.StatusConflict || code != "STUDIO_EXECUTION_CLIENT_REQUIRED" {
		t.Fatalf("unconnected task creation status=%d code=%q, want 409/STUDIO_EXECUTION_CLIENT_REQUIRED", status, code)
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
	createInvite, err := service.CreateMembershipInvite(t.Context(), adminActor, "studio-editor@example.com", "editor", "studio-editor")
	if err != nil {
		t.Fatal(err)
	}
	editorSession, err := service.RegisterWithInvite(t.Context(), "studio-editor@example.com", "long-enough-password", "内容编辑", createInvite.PlaintextToken)
	if err != nil {
		t.Fatal(err)
	}
	editorJar, _ := cookiejar.New(nil)
	editorJar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: editorSession.ID, Path: "/"}})
	editorClient := &http.Client{Jar: editorJar}
	editorBootstrap := callBFF[app.StudioBootstrap](t, editorClient, http.MethodGet, server.URL+"/api/studio/bootstrap", nil)
	if !editorBootstrap.Session.CanCreate || !editorBootstrap.Session.CanConnectClient {
		t.Fatalf("editor should be able to start and connect customer work: %#v", editorBootstrap.Session)
	}
	connect := callBFF[app.StudioConnectSession](t, editorClient, http.MethodPost, server.URL+"/api/studio/projects/"+bootstrap.Projects[0].ID+"/connect-sessions", map[string]any{})
	if connect.Status != "waiting_for_computer" {
		t.Fatalf("editor could not create customer connection: %#v", connect)
	}
	canceled := callBFF[app.StudioConnectSession](t, editorClient, http.MethodPost, server.URL+"/api/studio/connect-sessions/"+connect.ID+"/cancel", map[string]any{})
	if canceled.Status != "canceled" {
		t.Fatalf("editor could not cancel customer connection: %#v", canceled)
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

func callStudioError(t *testing.T, client *http.Client, method, target string, input any) (int, string) {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), method, target, strings.NewReader(string(body)))
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
		Error *domain.Error `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Error == nil {
		t.Fatalf("expected studio error, got %#v", envelope)
	}
	return response.StatusCode, envelope.Error.Code
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
