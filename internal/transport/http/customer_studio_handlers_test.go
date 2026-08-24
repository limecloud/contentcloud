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

	workbenchdomain "github.com/limecloud/contentcloud/internal/experience/workbench"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/testsupport"

	"github.com/limecloud/contentcloud/internal/persistence/memory"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestCustomerStudioProjectionAndTenantIsolation(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("demo@contentcloud.local"))
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
	if got, want := bootstrap.Experiences[0].Workbench.PluginID, "contentcloud-workbench-marketing-video"; got != want {
		t.Fatalf("customer bootstrap returned workbench plugin %q want %q", got, want)
	}
	if bootstrap.Experiences[0].Workbench.Digest == "" || bootstrap.Experiences[0].Workbench.Layout != "stage-canvas-context" {
		t.Fatalf("customer bootstrap returned incomplete workbench contract: %#v", bootstrap.Experiences[0].Workbench)
	}

	raw := getStudioRaw(t, client, server.URL+"/api/studio/bootstrap")
	for _, forbidden := range []string{`"tenant_id"`, `"sop_id"`, `"sop_digest"`, `"environment_id"`, `"stage_runs"`, `"executor_kind"`, `"capability_id"`, `"checks"`} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("customer bootstrap leaked internal field %s: %s", forbidden, raw)
		}
	}

	input := application.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "客户 Studio 幂等任务", Goal: "验证客户只看到业务目标和可复用资产", AssetRefs: []string{}}
	first := callBFFWithHeaders[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", input, map[string]string{"Idempotency-Key": "studio-idempotent-1"})
	second := callBFFWithHeaders[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", input, map[string]string{"Idempotency-Key": "studio-idempotent-1"})
	if first.Task.ID == "" || first.Task.ID != second.Task.ID {
		t.Fatalf("studio create is not idempotent: first=%s second=%s", first.Task.ID, second.Task.ID)
	}
	if first.Pipeline.StageCount == 0 || first.Pipeline.ExecutionCount == 0 {
		t.Fatalf("studio task did not expose the shared platform pipeline projection: %#v", first.Pipeline)
	}
	rawTask := getStudioRaw(t, client, server.URL+"/api/studio/tasks/"+first.Task.ID)
	for _, forbidden := range []string{`"tenant_id"`, `"sop_id"`, `"sop_digest"`, `"environment_id"`, `"stage_runs"`, `"runs"`, `"executor_kind"`, `"capability_id"`, `"checks"`} {
		if strings.Contains(rawTask, forbidden) {
			t.Fatalf("customer task leaked internal field %s: %s", forbidden, rawTask)
		}
	}

	foreignSession, err := service.Identity.Register(t.Context(), "studio-foreign@example.com", "long-enough-password", "外部客户", "外部团队")
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

func TestCustomerStudioUsesTenantScopedWorkbenchDeclaration(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default(), application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	actor, _, err := service.Identity.SessionActor(t.Context(), sessionIDFromJar(t, jar, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	var base workbenchdomain.Entry
	for _, candidate := range workbenchdomain.DefaultRegistry().Entries() {
		if candidate.Manifest.ID == "contentcloud-workbench-marketing-video" {
			base = candidate
			break
		}
	}
	if base.Manifest.ID == "" {
		t.Fatal("marketing video first-party workbench is missing")
	}
	base.Manifest.ID = "tenant-video-workbench"
	base.Manifest.Version = "2.0.0"
	base.Manifest.UI.Stages[0].Label = "客户目标"
	base.Manifest.UI.Stages[0].Outcome = "固定客户目标"
	base.Manifest.UI.Stages[1].Label = "客户交付"
	base.Manifest.UI.Stages[1].Outcome = "固定客户交付"
	base.Manifest.UI.Panels = []workbenchdomain.Panel{{ID: "customer-flow", Title: "客户流程", Detail: "客户自己的工作语言", Tone: "source", Icon: "folder", StageIDs: []string{base.Manifest.UI.Stages[0].ID, base.Manifest.UI.Stages[1].ID}, Target: "start", ActionLabel: "开始客户流程"}}
	created, err := service.Operations.RegisterWorkbench(t.Context(), application.Actor{PlatformAdmin: true, UserID: actor.UserID}, application.RegisterWorkbenchInput{Manifest: base.Manifest, TemplateAliases: base.TemplateAliases, TenantIDs: []string{actor.TenantID}}, "custom-workbench")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), application.Actor{PlatformAdmin: true, UserID: actor.UserID}, created.Manifest.ID, created.Manifest.Version, application.UpdateWorkbenchStateInput{Status: "published", TenantIDs: []string{actor.TenantID}}, "publish-custom-workbench"); err != nil {
		t.Fatal(err)
	}
	updated := callBFF[application.StudioBootstrap](t, client, http.MethodGet, server.URL+"/api/studio/bootstrap", nil)
	if len(updated.Experiences) != len(bootstrap.Experiences) || updated.Experiences[0].Workbench.PluginID != "tenant-video-workbench" {
		t.Fatalf("tenant scoped workbench was not projected: %#v", updated.Experiences)
	}
	if updated.Experiences[0].Workbench.Stages[0].Label != "客户目标" || len(updated.Experiences[0].Workbench.Panels) != 1 {
		t.Fatalf("custom workbench declaration was lost at the customer boundary: %#v", updated.Experiences[0].Workbench)
	}
	task := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{
		ExperienceID: updated.Experiences[0].ID,
		ProjectID:    updated.Projects[0].ID,
		Title:        "固定客户工作台版本",
		Goal:         "验证工作台撤回后历史任务仍使用创建时的界面声明",
	})
	if task.Workbench.PluginID != created.Manifest.ID || task.Workbench.Version != created.Manifest.Version || task.Workbench.Digest != created.Digest {
		t.Fatalf("created task did not pin the resolved workbench: %#v", task.Workbench)
	}
	if _, err := service.Operations.UpdateWorkbenchState(t.Context(), application.Actor{PlatformAdmin: true, UserID: actor.UserID}, created.Manifest.ID, created.Manifest.Version, application.UpdateWorkbenchStateInput{Status: "retired", TenantIDs: []string{actor.TenantID}}, "retire-custom-workbench"); err != nil {
		t.Fatal(err)
	}
	pinned := callBFF[application.StudioTaskView](t, client, http.MethodGet, server.URL+"/api/studio/tasks/"+task.Task.ID, nil)
	if pinned.Workbench.PluginID != created.Manifest.ID || pinned.Workbench.Version != created.Manifest.Version || pinned.Workbench.Digest != created.Digest {
		t.Fatalf("retiring a workbench changed the historical task surface: %#v", pinned.Workbench)
	}
}

func TestCustomerStudioBusinessWorkbenchesUseSharedTaskAndRuntime(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	session, err := service.Identity.Register(t.Context(), "multi-workbench@example.com", "long-enough-password", "业务负责人", "多业务租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, contentType := range []string{identitydomain.ContentTypeWeChatArticle, identitydomain.ContentTypeCommerce, identitydomain.ContentTypeSerializedNovel} {
		if err := store.SetTenantContentCapability(t.Context(), identitydomain.TenantContentCapability{TenantID: actor.TenantID, ContentType: contentType, Enabled: true, UpdatedBy: actor.UserID}); err != nil {
			t.Fatal(err)
		}
	}

	articleProject, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "文章品牌", ProductName: "内容专栏", ContentType: identitydomain.ContentTypeWeChatArticle, Channel: "wechat_official_account"}, "multi-workbench-article-project")
	if err != nil {
		t.Fatal(err)
	}
	commerceProject, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "商品品牌", ProductName: "低糖点心", ContentType: identitydomain.ContentTypeCommerce, Channel: "douyin"}, "multi-workbench-commerce-project")
	if err != nil {
		t.Fatal(err)
	}
	novelProject, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "长篇故事", ProductName: "雾城连载", ContentType: identitydomain.ContentTypeSerializedNovel, Channel: "web_novel"}, "multi-workbench-novel-project")
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range []workspacedomain.Project{articleProject, commerceProject, novelProject} {
		connect, err := service.Workspace.CreateConnectSession(t.Context(), actor, project.ID, "multi-workbench-connect-"+project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, application.ConnectDeviceInput{Hostname: "multi-workbench-" + project.ID, Platform: "darwin", Arch: "arm64", Version: "test"}); err != nil {
			t.Fatal(err)
		}
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}
	bootstrap := callBFF[application.StudioBootstrap](t, client, http.MethodGet, server.URL+"/api/studio/bootstrap", nil)

	experiences := map[string]application.StudioExperience{}
	for _, experience := range bootstrap.Experiences {
		experiences[experience.Workbench.PluginID] = experience
	}
	articleExperience := experiences["contentcloud-workbench-article"]
	commerceExperience := experiences["contentcloud-workbench-commerce"]
	novelExperience := experiences["contentcloud-workbench-serialized-novel"]
	if articleExperience.ID == "" || articleExperience.Workbench.Layout != "article-editor" || !containsStringValue(articleExperience.ProjectIDs, articleProject.ID) {
		t.Fatalf("article workbench did not resolve through its built-in SOP alias: %#v", articleExperience)
	}
	if commerceExperience.ID == "" || commerceExperience.Workbench.Layout != "product-variants" || !containsStringValue(commerceExperience.ProjectIDs, commerceProject.ID) {
		t.Fatalf("commerce workbench did not resolve through its built-in SOP: %#v", commerceExperience)
	}
	if novelExperience.ID == "" || novelExperience.Workbench.Layout != "novel-editor" || !containsStringValue(novelExperience.ProjectIDs, novelProject.ID) {
		t.Fatalf("serialized novel workbench did not resolve through the shared built-in SOP: %#v", novelExperience)
	}

	articleTask := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{
		ExperienceID: articleExperience.ID, ProjectID: articleProject.ID, Title: "公众号文章任务", Goal: "形成有来源的公众号文章",
		BusinessBrief: application.StudioBusinessBrief{Audience: "新客户", Channel: "公众号", Tone: "可信", Keywords: []string{"成分", "使用方法"}}, IdempotencyKey: "multi-workbench-article-task",
	})
	commerceTask := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{
		ExperienceID: commerceExperience.ID, ProjectID: commerceProject.ID, Title: "电商内容任务", Goal: "生成遵守商品事实的渠道内容变体",
		BusinessBrief: application.StudioBusinessBrief{Channel: "抖音", TargetAudience: "控糖人群", ProductFacts: map[string]string{"净含量": "500g"}, OfferPoints: []string{"低糖", "独立包装"}}, IdempotencyKey: "multi-workbench-commerce-task",
	})
	novelTask := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{
		ExperienceID: novelExperience.ID, ProjectID: novelProject.ID, Title: "连载小说章节任务", Goal: "在既定 Canon 下完成下一章并通过连续性校验",
		BusinessBrief: application.StudioBusinessBrief{Audience: "悬疑读者", Channel: "连载平台", Tone: "克制悬疑", Keywords: []string{"雾城", "失踪案", "双时间线"}}, IdempotencyKey: "multi-workbench-novel-task",
	})

	for _, item := range []struct {
		view          application.StudioTaskView
		contentType   string
		pluginID      string
		layout        string
		expectedSteps int
		expectedNodes int
	}{
		{view: articleTask, contentType: identitydomain.ContentTypeWeChatArticle, pluginID: "contentcloud-workbench-article", layout: "article-editor", expectedSteps: 5, expectedNodes: 6},
		{view: commerceTask, contentType: identitydomain.ContentTypeCommerce, pluginID: "contentcloud-workbench-commerce", layout: "product-variants", expectedSteps: 5, expectedNodes: 7},
		{view: novelTask, contentType: identitydomain.ContentTypeSerializedNovel, pluginID: "contentcloud-workbench-serialized-novel", layout: "novel-editor", expectedSteps: 7, expectedNodes: 11},
	} {
		if item.view.Task.ContentType != item.contentType || item.view.Workbench.PluginID != item.pluginID || item.view.Workbench.Layout != item.layout {
			t.Fatalf("business workbench task lost its pinned surface: %#v", item.view)
		}
		if item.view.Pipeline.ExecutionCount != 1 || item.view.Pipeline.StageCount != len(item.view.Steps) || len(item.view.Steps) != item.expectedSteps {
			t.Fatalf("business workbench did not use the shared task/runtime pipeline: %#v", item.view.Pipeline)
		}
		stored, err := service.Work.WorkTask(t.Context(), actor, item.view.Task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := stored.Task.RequestedOutput["business_brief"].(map[string]any); !ok {
			t.Fatalf("business brief was not frozen in WorkTask.RequestedOutput: %#v", stored.Task.RequestedOutput)
		}
		if _, ok := stored.Task.RequestedOutput["workbench"].(map[string]any); !ok {
			t.Fatalf("workbench reference was not frozen in WorkTask.RequestedOutput: %#v", stored.Task.RequestedOutput)
		}
	}

	jobs, err := service.Runtime.RuntimeJobs(t.Context(), actor, "", "", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	jobByTask := map[string]application.RuntimeJobSummary{}
	for _, job := range jobs.Items {
		jobByTask[job.WorkTaskID] = job
	}
	for _, item := range []struct {
		taskID        string
		expectedNodes int
	}{
		{taskID: articleTask.Task.ID, expectedNodes: 6},
		{taskID: commerceTask.Task.ID, expectedNodes: 7},
		{taskID: novelTask.Task.ID, expectedNodes: 11},
	} {
		job := jobByTask[item.taskID]
		if job.ID == "" || job.RuntimePolicyID != "runtime-policy/customer-studio-v1" || job.ContractMajor != 1 || job.NodeCount != item.expectedNodes {
			t.Fatalf("task %s did not enter the shared Runtime contract: %#v", item.taskID, job)
		}
	}
}

func TestCustomerStudioAssetSurfaceSeparatesWorkspaceMaterialsAndCreativeResults(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	clientJar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: clientJar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)

	surface := callBFF[application.CustomerAssetSurface](t, client, http.MethodGet, server.URL+"/api/studio/assets", nil)
	if len(surface.CreativeResults.Items) == 0 {
		t.Fatal("development fixture should expose generated customer assets")
	}
	if len(surface.Workspace.Materials) == 0 || len(surface.Workspace.Folders) == 0 {
		t.Fatalf("development fixture should expose an explicit workspace material: %#v", surface.Workspace)
	}
	initialMaterialCount, initialFolderCount := len(surface.Workspace.Materials), len(surface.Workspace.Folders)
	allowedResultTypes := map[string]bool{"persona": true, "script": true, "storyboard": true, "image": true, "video": true}
	foundResultTypes := map[string]bool{}
	sampleByType := map[string]application.StudioAssetItem{}
	for _, item := range surface.CreativeResults.Items {
		if !allowedResultTypes[item.ResultType] {
			t.Fatalf("customer result exposed non-result type %q: %#v", item.ResultType, item)
		}
		if item.TaskID == "" || item.TaskTitle == "" {
			t.Fatalf("customer asset lost its originating task: %#v", item)
		}
		foundResultTypes[item.ResultType] = true
		if _, exists := sampleByType[item.ResultType]; !exists {
			sampleByType[item.ResultType] = item
		}
	}
	for _, required := range []string{"persona", "script", "storyboard", "image", "video"} {
		if !foundResultTypes[required] {
			t.Fatalf("development fixture did not expose %s result assets: %#v", required, foundResultTypes)
		}
	}
	for _, resultType := range []string{"persona", "script", "storyboard", "image", "video"} {
		item := sampleByType[resultType]
		resultID := strings.TrimPrefix(item.Ref, "result:")
		detailPath := server.URL + "/api/studio/assets/results/" + url.PathEscape(resultID) + "?task_id=" + url.QueryEscape(item.TaskID)
		detail := callBFF[application.StudioCreativeResultDetail](t, client, http.MethodGet, detailPath, nil)
		if detail.Item.Ref != item.Ref || detail.ContentFormat != resultType || !detail.ReadOnly {
			t.Fatalf("unexpected %s result detail: %#v", resultType, detail)
		}
		if (resultType == "persona" || resultType == "script" || resultType == "storyboard") && string(detail.Content) == "{}" {
			t.Fatalf("%s detail did not expose renderer content: %#v", resultType, detail)
		}
		rawDetail := getStudioRaw(t, client, detailPath)
		for _, forbidden := range []string{`"tenant_id"`, `"object_key"`, `"source_revision_id"`, `"created_by"`, `"evidence_refs"`, `"rights_refs"`} {
			if strings.Contains(rawDetail, forbidden) {
				t.Fatalf("customer %s result detail leaked %s: %s", resultType, forbidden, rawDetail)
			}
		}
	}
	raw := getStudioRaw(t, client, server.URL+"/api/studio/assets")
	for _, forbidden := range []string{`"tenant_id"`, `"source_revision_id"`, `"object_key"`, `"kind":"source"`, `"kind":"knowledge"`, `source_revision:`, `"rights_record"`, `"source_type":"manual_inspiration"`} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("customer asset surface leaked an internal or governance object %s: %s", forbidden, raw)
		}
	}

	folder := callBFF[application.WorkspaceFolderItem](t, client, http.MethodPost, server.URL+"/api/studio/asset-folders", application.CreateWorkspaceFolderInput{ProjectID: bootstrap.Projects[0].ID, Name: "品牌资料"})
	material := callMultipartBFF[application.WorkspaceMaterialItem](t, client, server.URL+"/api/studio/materials", "brand-brief.txt", []byte("品牌语气与产品卖点"), map[string]string{"project_id": bootstrap.Projects[0].ID, "folder_ref": folder.Ref, "file_type": "text/plain"})
	if material.MaterialKind != workspacedomain.WorkspaceMaterialDocument || material.FolderRef != folder.Ref || material.ProjectID != bootstrap.Projects[0].ID {
		t.Fatalf("unexpected workspace material projection: %#v", material)
	}
	withMaterial := callBFF[application.CustomerAssetSurface](t, client, http.MethodGet, server.URL+"/api/studio/assets", nil)
	if len(withMaterial.Workspace.Materials) != initialMaterialCount+1 || len(withMaterial.Workspace.Folders) != initialFolderCount+1 || len(withMaterial.CreativeResults.Items) != len(surface.CreativeResults.Items) {
		t.Fatalf("workspace material changed the creative result projection: %#v", withMaterial)
	}
	task := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "资料加入创作", Goal: "验证固定版本工作区资料可以加入任务", AssetRefs: []string{}, MaterialRefs: []string{}})
	callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks/"+task.Task.ID+"/materials", application.StudioAttachMaterialsInput{MaterialRefs: []string{material.Ref}})
	recent := callBFF[application.CustomerAssetSurface](t, client, http.MethodGet, server.URL+"/api/studio/assets", nil)
	if len(recent.Recent.Materials) == 0 || recent.Recent.Materials[0].Ref != material.Ref || recent.Recent.Materials[0].LastUsedAt == nil {
		t.Fatalf("attached workspace material did not enter the recent projection: %#v", recent.Recent.Materials)
	}
}

func TestCustomerStudioInspirationUsesProjectReferenceBoundary(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	task := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{
		ExperienceID: bootstrap.Experiences[0].ID,
		ProjectID:    bootstrap.Projects[0].ID,
		Title:        "项目参考边界回归",
		Goal:         "验证灵感不进入结果资产库",
		AssetRefs:    []string{},
	})

	withReference := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks/"+task.Task.ID+"/inspirations", application.StudioAddInspirationInput{
		Title:                  "保留的观察",
		Body:                   "只作为后续任务的项目参考。",
		KeepAsProjectReference: true,
	})
	var projectReference *application.StudioInspiration
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
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	actor, _, err := service.Identity.SessionActor(t.Context(), sessionIDFromJar(t, jar, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Identity.UpdatePlatformTenantContentCapability(t.Context(), actor, actor.TenantID, identitydomain.ContentTypeMarketingVideo, false, "studio-disable"); err != nil {
		t.Fatal(err)
	}
	withoutCapability := callBFF[application.StudioBootstrap](t, client, http.MethodGet, server.URL+"/api/studio/bootstrap", nil)
	if len(withoutCapability.Experiences) != 0 {
		t.Fatalf("disabled tenant capability still exposed experiences: %#v", withoutCapability.Experiences)
	}
	if len(bootstrap.Experiences) == 0 {
		t.Fatal("fixture did not create a published experience")
	}
}

func TestCustomerStudioExecutionClientHTTPFlow(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(t.Context(), "studio-connect@example.com", "long-enough-password", "Studio Connect", "Studio Connect Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "连接品牌", ProductName: "连接产品", Channel: "douyin"}, "studio-connect-project")
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
	availableClients := map[string]bool{}
	for _, client := range catalog.Clients {
		if client.Available {
			availableClients[client.ID] = true
		}
		if client.ID != "codex" && client.ID != "claude-code" && client.Available {
			t.Fatalf("customer bootstrap exposed an unsupported connection client: %#v", client)
		}
	}
	if !availableClients["codex"] || !availableClients["claude-code"] {
		t.Fatalf("customer bootstrap did not expose the supported host clients: %#v", catalog)
	}

	connect := callBFF[application.StudioConnectSession](t, client, http.MethodPost, server.URL+"/api/studio/projects/"+project.ID+"/connect-sessions", map[string]any{})
	if connect.ProjectID != project.ID || connect.Status != "waiting_for_computer" {
		t.Fatalf("unexpected created connection session: %#v", connect)
	}
	waiting := callBFF[application.StudioConnectSession](t, client, http.MethodGet, server.URL+"/api/studio/connect-sessions/"+connect.ID, nil)
	if waiting.Status != "waiting_for_computer" || waiting.RequiresConfirmation {
		t.Fatalf("unexpected initial connection status: %#v", waiting)
	}

	started := callDispatch[application.StartBootstrapAuthorizationResult](t, client, server.URL, "", "bootstrap.authorization.start", application.StartBootstrapAuthorizationInput{
		SessionID: connect.ID, CodeChallenge: strings.Repeat("a", 43), Platform: "darwin", Arch: "arm64", CLIVersion: "test",
	})
	confirmation := callBFF[application.StudioConnectSession](t, client, http.MethodGet, server.URL+"/api/studio/connect-sessions/"+connect.ID, nil)
	if confirmation.Status != "confirmation_required" || !confirmation.RequiresConfirmation || confirmation.VerificationCode == "" {
		t.Fatalf("client confirmation was not projected: %#v", confirmation)
	}
	approved := callBFF[application.StudioConnectSession](t, client, http.MethodPost, server.URL+"/api/studio/connect-sessions/"+connect.ID+"/approve", map[string]any{})
	if approved.Status != "connecting" || approved.RequiresConfirmation {
		t.Fatalf("approved connection status = %#v", approved)
	}
	if started.AttemptID == "" {
		t.Fatal("bootstrap authorization did not return an attempt")
	}

	cancelSession := callBFF[application.StudioConnectSession](t, client, http.MethodPost, server.URL+"/api/studio/projects/"+project.ID+"/connect-sessions", map[string]any{})
	canceled := callBFF[application.StudioConnectSession](t, client, http.MethodPost, server.URL+"/api/studio/connect-sessions/"+cancelSession.ID+"/cancel", map[string]any{})
	if canceled.Status != "canceled" || canceled.ProjectID != project.ID {
		t.Fatalf("canceled connection status = %#v", canceled)
	}
}

func TestCustomerStudioTaskRequiresExecutionClient(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(t.Context(), "studio-task-gate@example.com", "long-enough-password", "Studio Task Gate", "Studio Task Gate Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "未连接品牌", ProductName: "未连接产品", Channel: "douyin"}, "studio-task-gate-project")
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}
	status, code := callStudioError(t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{
		ExperienceID: "not-used-before-connection", ProjectID: project.ID, Title: "未连接任务", Goal: "验证连接门禁",
	})
	if status != http.StatusConflict || code != "STUDIO_EXECUTION_CLIENT_REQUIRED" {
		t.Fatalf("unconnected task creation status=%d code=%q, want 409/STUDIO_EXECUTION_CLIENT_REQUIRED", status, code)
	}
}

func TestCustomerStudioHonorsCustomerRole(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}
	bootstrap := mustStudioBootstrap(t, adminClient, server.URL)
	adminActor, _, err := service.Identity.SessionActor(t.Context(), sessionIDFromJar(t, adminJar, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	invite, err := service.Identity.CreateMembershipInvite(t.Context(), adminActor, "studio-viewer@example.com", "viewer", "studio-viewer")
	if err != nil {
		t.Fatal(err)
	}
	viewerSession, err := service.Identity.RegisterWithInvite(t.Context(), "studio-viewer@example.com", "long-enough-password", "只读客户", invite.PlaintextToken)
	if err != nil {
		t.Fatal(err)
	}
	viewerJar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	viewerJar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: viewerSession.ID, Path: "/"}})
	viewerClient := &http.Client{Jar: viewerJar}
	viewerBootstrap := callBFF[application.StudioBootstrap](t, viewerClient, http.MethodGet, server.URL+"/api/studio/bootstrap", nil)
	if viewerBootstrap.Session.CanCreate {
		t.Fatalf("viewer unexpectedly received create permission: %#v", viewerBootstrap.Session)
	}
	createInvite, err := service.Identity.CreateMembershipInvite(t.Context(), adminActor, "studio-editor@example.com", "editor", "studio-editor")
	if err != nil {
		t.Fatal(err)
	}
	editorSession, err := service.Identity.RegisterWithInvite(t.Context(), "studio-editor@example.com", "long-enough-password", "内容编辑", createInvite.PlaintextToken)
	if err != nil {
		t.Fatal(err)
	}
	editorJar, _ := cookiejar.New(nil)
	editorJar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: editorSession.ID, Path: "/"}})
	editorClient := &http.Client{Jar: editorJar}
	editorBootstrap := callBFF[application.StudioBootstrap](t, editorClient, http.MethodGet, server.URL+"/api/studio/bootstrap", nil)
	if !editorBootstrap.Session.CanCreate || !editorBootstrap.Session.CanConnectClient {
		t.Fatalf("editor should be able to start and connect customer work: %#v", editorBootstrap.Session)
	}
	connect := callBFF[application.StudioConnectSession](t, editorClient, http.MethodPost, server.URL+"/api/studio/projects/"+bootstrap.Projects[0].ID+"/connect-sessions", map[string]any{})
	if connect.Status != "waiting_for_computer" {
		t.Fatalf("editor could not create customer connection: %#v", connect)
	}
	canceled := callBFF[application.StudioConnectSession](t, editorClient, http.MethodPost, server.URL+"/api/studio/connect-sessions/"+connect.ID+"/cancel", map[string]any{})
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

func mustStudioBootstrap(t *testing.T, client *http.Client, baseURL string) application.StudioBootstrap {
	t.Helper()
	response, err := client.Post(baseURL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("development bootstrap status=%d", response.StatusCode)
	}
	return callBFF[application.StudioBootstrap](t, client, http.MethodGet, baseURL+"/api/studio/bootstrap", nil)
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
		OK    bool         `json:"ok"`
		Error *fault.Error `json:"error"`
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
