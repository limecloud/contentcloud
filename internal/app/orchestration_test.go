package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestOrchestrationDefaultsAndTaskPinPublishedSOP(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "orchestration-owner@example.com", "long-enough-password", "流程负责人", "内容团队")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "品牌", ProductName: "产品", Channel: "douyin"}, "")
	if err != nil {
		t.Fatal(err)
	}

	admin, err := service.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(admin.Environments) != 1 || len(admin.SOPs) != 4 || len(admin.Gates) != 3 {
		t.Fatalf("defaults were not materialized: %#v", admin)
	}
	if !strings.HasPrefix(admin.Environments[0].ManifestDigest, "sha256:") || len(admin.Environments[0].ManifestDigest) != len("sha256:")+64 {
		t.Fatalf("environment digest is not a sha256 digest: %q", admin.Environments[0].ManifestDigest)
	}
	for _, gate := range admin.Gates {
		if gate.Mode != domain.GateModeRequiredCheck || !gate.Blocking {
			t.Fatalf("built-in deterministic Gate must block on failure: %#v", gate)
		}
	}

	binding, sop, err := service.ProjectSOP(ctx, actor, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateWorkTask(ctx, actor, app.CreateWorkTaskInput{ProjectID: project.ID, Title: "生成新品脚本", ContentType: "video_script", InputRefs: []string{"brief:1"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if task.Task.SOPID != binding.SOPID || task.Task.SOPVersion != sop.Version || task.Task.SOPDigest != sop.Digest {
		t.Fatalf("task did not pin project SOP: %#v / %#v", task.Task, binding)
	}
	if len(task.StageRuns) != 1 || task.StageRuns[0].StageID != sop.Stages[0].ID {
		t.Fatalf("task stage run was not initialized: %#v", task.StageRuns)
	}
}

func TestOrchestrationDraftPublishAndTenantIsolation(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "orchestration-admin@example.com", "long-enough-password", "管理员", "租户 A")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := service.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var sopID string
	for _, summary := range admin.SOPs {
		if summary.Definition.TemplateKey == "content_research" {
			sopID = summary.Definition.ID
			break
		}
	}
	if sopID == "" {
		t.Fatal("content research built-in SOP was not provisioned")
	}
	draft, err := service.CreateSOPDraft(ctx, actor, sopID, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if draft.Status != "draft" || draft.Version != 2 || draft.Digest != "" {
		t.Fatalf("unexpected draft: %#v", draft)
	}
	draft.Gates[0].Mode = domain.GateModeNone
	draft.Gates[0].Blocking = false
	updated, err := service.SaveSOPVersion(ctx, actor, sopID, draft.Version, app.SaveSOPVersionInput{Name: draft.Name, Description: draft.Description, ContentTypes: draft.ContentTypes, Stages: draft.Stages, Gates: draft.Gates, DefaultExecutionMode: draft.DefaultExecutionMode}, "")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Digest != "" || updated.Gates[0].Mode != domain.GateModeNone {
		t.Fatalf("draft save did not preserve editable state: %#v", updated)
	}
	published, err := service.PublishSOP(ctx, actor, sopID, draft.Version, "")
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != "published" || !strings.HasPrefix(published.Digest, "sha256:") || len(published.Digest) != len("sha256:")+64 {
		t.Fatalf("unexpected published SOP: %#v", published)
	}
	if _, err := service.SaveSOPVersion(ctx, actor, sopID, draft.Version, app.SaveSOPVersionInput{Name: "非法修改", Stages: published.Stages, Gates: published.Gates}, ""); err == nil || !domain.IsNotFound(err) && !strings.Contains(err.Error(), "不可") {
		t.Fatalf("published SOP should be immutable, got %v", err)
	}

	foreignSession, err := service.Register(ctx, "orchestration-foreign@example.com", "long-enough-password", "外部用户", "租户 B")
	if err != nil {
		t.Fatal(err)
	}
	foreignActor, _, err := service.SessionActor(ctx, foreignSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveEnvironment(ctx, foreignActor, admin.Environments[0].ID, app.SaveEnvironmentInput{Name: "越权"}, ""); err == nil || !domain.IsNotFound(err) {
		t.Fatalf("cross-tenant environment access must be hidden, got %v", err)
	}
}

func TestBuiltinSOPsAreTenantScopedAndIdempotent(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	firstSession, err := service.Register(ctx, "builtin-first@example.com", "long-enough-password", "第一租户", "第一租户")
	if err != nil {
		t.Fatal(err)
	}
	firstActor, _, err := service.SessionActor(ctx, firstSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.AdminWorkOS(ctx, firstActor)
	if err != nil {
		t.Fatal(err)
	}
	firstAgain, err := service.AdminWorkOS(ctx, firstActor)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.SOPs) != 4 || len(firstAgain.SOPs) != 4 {
		t.Fatalf("built-in SOP installation is not idempotent: first=%d again=%d", len(first.SOPs), len(firstAgain.SOPs))
	}
	for _, summary := range firstAgain.SOPs {
		if len(summary.Versions) != 1 || summary.Versions[0].Version != 1 || !summary.Definition.BuiltIn {
			t.Fatalf("repeated installation changed built-in versions: %#v", summary)
		}
	}

	secondSession, err := service.Register(ctx, "builtin-second@example.com", "long-enough-password", "第二租户", "第二租户")
	if err != nil {
		t.Fatal(err)
	}
	secondActor, _, err := service.SessionActor(ctx, secondSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.AdminWorkOS(ctx, secondActor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.SOPs) != 4 || len(second.Environments) != 1 {
		t.Fatalf("built-in SOPs leaked across tenant storage: %#v", second)
	}
	for _, summary := range second.SOPs {
		if summary.Definition.TenantID != secondActor.TenantID {
			t.Fatalf("SOP returned outside tenant scope: %#v", summary.Definition)
		}
	}
}

func TestLegacyShortVideoSOPUpgradesWithoutRebindingEnvironment(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "builtin-legacy@example.com", "long-enough-password", "旧流程租户", "旧流程租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := service.CreateSOP(ctx, actor, app.CreateSOPInput{
		Name:         "短视频生产",
		ContentTypes: []string{domain.ContentTypeVideoScript},
		Stages: []domain.StageDefinition{
			{ID: "brief", Name: "需求 Brief", Order: 10, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local"}},
			{ID: "knowledge", Name: "知识", Order: 20, OutputSchema: domain.KnowledgeSnapshotSchema, ExecutionModes: []string{"local"}},
			{ID: "draft", Name: "脚本", Order: 30, OutputSchema: "contentcloud.video_script/1.0", ExecutionModes: []string{"local"}},
			{ID: "delivery", Name: "交付", Order: 40, OutputSchema: "contentcloud.delivery/1.0", ExecutionModes: []string{"local"}},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	legacyVersion, err := service.PublishSOP(ctx, actor, legacy.Definition.ID, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	legacyEnvironment, err := service.CreateEnvironment(ctx, actor, app.SaveEnvironmentInput{Name: "旧默认环境", Slug: "legacy", Status: "active", DefaultSOPID: legacy.Definition.ID, DefaultSOPVersion: legacyVersion.Version}, "")
	if err != nil {
		t.Fatal(err)
	}

	admin, err := service.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var upgraded *domain.SOPSummary
	for index := range admin.SOPs {
		if admin.SOPs[index].Definition.ID == legacy.Definition.ID {
			upgraded = &admin.SOPs[index]
			break
		}
	}
	if upgraded == nil || !upgraded.Definition.BuiltIn || upgraded.Definition.TemplateKey != "short_video_production" {
		t.Fatalf("legacy SOP was not adopted as the built-in template: %#v", upgraded)
	}
	if len(upgraded.Versions) != 2 || upgraded.Versions[0].Version != 2 || upgraded.Versions[1].Version != 1 || upgraded.Versions[0].Status != "published" {
		t.Fatalf("legacy SOP did not receive an additive upgrade: %#v", upgraded.Versions)
	}
	for _, environment := range admin.Environments {
		if environment.ID == legacyEnvironment.ID && (environment.DefaultSOPID != legacy.Definition.ID || environment.DefaultSOPVersion != 1) {
			t.Fatalf("legacy environment was silently rebound: %#v", environment)
		}
	}
}

func TestExistingProjectGetsNewSOPBindingOnFirstWorkOSAccess(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "legacy-project@example.com", "long-enough-password", "旧项目用户", "旧项目租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "历史品牌", ProductName: "历史产品", Channel: "douyin"}, "")
	if err != nil {
		t.Fatal(err)
	}
	binding, version, err := service.ProjectSOP(ctx, actor, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.ProjectID != project.ID || binding.SOPID == "" || binding.SOPVersion < 1 || version.Status != "published" {
		t.Fatalf("existing project was not attached to a published SOP: binding=%#v version=%#v", binding, version)
	}
	if version.Name != "短视频生产" {
		t.Fatalf("existing project should use the default short-video template: %#v", version)
	}
	again, sameVersion, err := service.ProjectSOP(ctx, actor, project.ID)
	if err != nil || again.SOPDigest != binding.SOPDigest || sameVersion.Digest != version.Digest {
		t.Fatalf("repeated project binding was not stable: first=%#v second=%#v err=%v", binding, again, err)
	}
}

func TestKnownBuiltinIDRepairsMetadataBeforeUpgrade(t *testing.T) {
	ctx := t.Context()
	st := memory.New()
	service := app.New(st, nil)
	session, err := service.Register(ctx, "builtin-known-id@example.com", "long-enough-password", "旧内置用户", "旧内置租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	sopID := "builtin-sop-short-video"
	definition := domain.SOPDefinition{ID: sopID, TenantID: actor.TenantID, Name: "短视频生产", ContentTypes: []string{domain.ContentTypeVideoScript}, CurrentVersion: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	version := domain.SOPVersion{ID: "old-builtin-short-video-v1", TenantID: actor.TenantID, SOPID: sopID, Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: definition.Name, ContentTypes: definition.ContentTypes, Stages: []domain.StageDefinition{
		{ID: "brief", Name: "需求 Brief", Order: 10, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local"}},
		{ID: "knowledge", Name: "知识", Order: 20, OutputSchema: domain.KnowledgeSnapshotSchema, ExecutionModes: []string{"local"}},
		{ID: "draft", Name: "脚本", Order: 30, OutputSchema: "contentcloud.video_script/1.0", ExecutionModes: []string{"local"}},
		{ID: "delivery", Name: "交付", Order: 40, OutputSchema: "contentcloud.delivery/1.0", ExecutionModes: []string{"local"}},
	}, DefaultExecutionMode: "local", Status: "published", CreatedBy: actor.UserID, PublishedBy: actor.UserID, CreatedAt: now, PublishedAt: &now}
	digest, err := version.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	version.Digest = "sha256:" + digest
	if err := st.CreateSOP(ctx, definition, version); err != nil {
		t.Fatal(err)
	}

	admin, err := service.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range admin.SOPs {
		if summary.Definition.ID != sopID {
			continue
		}
		if !summary.Definition.BuiltIn || summary.Definition.TemplateKey != "short_video_production" || summary.Definition.SourceRef == "" {
			t.Fatalf("known built-in ID did not repair metadata: %#v", summary.Definition)
		}
		if len(summary.Versions) != 2 || summary.Versions[0].Version != 2 || summary.Versions[0].Status != "published" {
			t.Fatalf("known built-in ID did not receive additive upgrade: %#v", summary.Versions)
		}
		return
	}
	t.Fatalf("known built-in SOP was not returned: %s", sopID)
}

func TestSOPDiffImpactRollbackAndRetire(t *testing.T) {
	ctx := t.Context()
	st := memory.New()
	service := app.New(st, nil)
	session, err := service.Register(ctx, "sop-governance@example.com", "long-enough-password", "流程负责人", "流程治理租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "治理品牌", ProductName: "治理产品"}, "")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := service.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var sopID string
	for _, summary := range admin.SOPs {
		if summary.Definition.TemplateKey == "content_research" {
			sopID = summary.Definition.ID
			break
		}
	}
	if sopID == "" {
		t.Fatal("content research SOP not found")
	}
	draft, err := service.CreateSOPDraft(ctx, actor, sopID, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	draft.Description = "升级后的资料与知识建设流程"
	if _, err := service.SaveSOPVersion(ctx, actor, sopID, draft.Version, app.SaveSOPVersionInput{Name: draft.Name, Description: draft.Description, ContentTypes: draft.ContentTypes, Stages: draft.Stages, Gates: draft.Gates, DefaultExecutionMode: draft.DefaultExecutionMode}, ""); err != nil {
		t.Fatal(err)
	}
	versionTwo, err := service.PublishSOP(ctx, actor, sopID, draft.Version, "")
	if err != nil {
		t.Fatal(err)
	}
	diff, err := service.SOPVersionDiff(ctx, actor, sopID, 1, versionTwo.Version)
	if err != nil || diff.Same || len(diff.Changes) == 0 {
		t.Fatalf("SOP diff did not expose the description change: diff=%#v err=%v", diff, err)
	}
	environment, err := service.CreateEnvironment(ctx, actor, app.SaveEnvironmentInput{Name: "治理环境", Slug: "governance", Status: "active", DefaultSOPID: sopID, DefaultSOPVersion: versionTwo.Version}, "")
	if err != nil {
		t.Fatal(err)
	}
	digest := versionTwo.Digest
	if err := st.SaveProjectSOPBinding(ctx, domain.ProjectSOPBinding{TenantID: actor.TenantID, ProjectID: project.ID, EnvironmentID: environment.ID, SOPID: sopID, SOPVersion: versionTwo.Version, SOPDigest: digest, BoundBy: actor.UserID, BoundAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateWorkTask(ctx, actor, app.CreateWorkTaskInput{ProjectID: project.ID, Title: "固定版本的治理任务", ContentType: domain.ContentTypeVideoScript, InputRefs: []string{"brief:governance"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	impact, err := service.SOPVersionImpact(ctx, actor, sopID, versionTwo.Version)
	if err != nil || impact.Counts["environments"] != 1 || impact.Counts["projects"] != 1 || impact.Counts["tasks"] != 1 {
		t.Fatalf("SOP impact is incomplete: impact=%#v err=%v", impact, err)
	}
	rollback, err := service.RollbackSOPVersion(ctx, actor, sopID, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if rollback.Version.Version != versionTwo.Version+1 || rollback.TargetVersion != 1 || rollback.ReboundEnvironments != 1 || rollback.ReboundProjects != 1 {
		t.Fatalf("rollback did not publish and rebind future work: %#v", rollback)
	}
	rolledEnvironment, err := st.Environment(ctx, actor.TenantID, environment.ID)
	if err != nil || rolledEnvironment.DefaultSOPVersion != rollback.Version.Version {
		t.Fatalf("environment was not rebound by explicit rollback: %#v err=%v", rolledEnvironment, err)
	}
	binding, err := st.ProjectSOPBinding(ctx, actor.TenantID, project.ID)
	if err != nil || binding.SOPVersion != rollback.Version.Version {
		t.Fatalf("project binding was not rebound by explicit rollback: %#v err=%v", binding, err)
	}
	if task.Task.SOPVersion != versionTwo.Version {
		t.Fatalf("rollback rewrote historical task pin: %#v", task.Task)
	}
	if err := service.RetireSOPVersion(ctx, actor, sopID, versionTwo.Version, ""); err != nil {
		t.Fatalf("non-current SOP version should be retireable: %v", err)
	}
}

func TestCustomShortVideoSOPIsNotMisclassifiedAsBuiltin(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "builtin-custom@example.com", "long-enough-password", "自定义租户", "自定义租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	custom, err := service.CreateSOP(ctx, actor, app.CreateSOPInput{
		Name:         "短视频生产",
		ContentTypes: []string{domain.ContentTypeVideoScript},
		Stages: []domain.StageDefinition{
			{ID: "brief", Name: "需求", Order: 10, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local"}},
			{ID: "knowledge", Name: "知识", Order: 20, OutputSchema: domain.KnowledgeSnapshotSchema, ExecutionModes: []string{"local"}},
			{ID: "draft", Name: "脚本", Order: 30, OutputSchema: "contentcloud.video_script/1.0", ExecutionModes: []string{"local"}},
			{ID: "quality", Name: "质量", Order: 40, OutputSchema: "contentcloud.quality_report/1.0", ExecutionModes: []string{"local"}},
			{ID: "delivery", Name: "交付", Order: 50, OutputSchema: "contentcloud.delivery/1.0", ExecutionModes: []string{"local"}},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishSOP(ctx, actor, custom.Definition.ID, 1, ""); err != nil {
		t.Fatal(err)
	}
	admin, err := service.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range admin.SOPs {
		if summary.Definition.ID == custom.Definition.ID && (summary.Definition.BuiltIn || summary.Definition.TemplateKey != "") {
			t.Fatalf("same-name custom SOP was misclassified: %#v", summary.Definition)
		}
	}
}

func TestOrchestrationAdminCanCreateEnvironmentAndLintSOP(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "orchestration-builder@example.com", "long-enough-password", "流程管理员", "内容租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := service.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	createdEnvironment, err := service.CreateEnvironment(ctx, actor, app.SaveEnvironmentInput{Name: "审核环境", Slug: "review", Status: "paused", DefaultSOPID: admin.SOPs[0].Definition.ID, DefaultSOPVersion: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	if createdEnvironment.Slug != "review" || createdEnvironment.Status != "paused" || createdEnvironment.DefaultSOPID == "" {
		t.Fatalf("unexpected environment: %#v", createdEnvironment)
	}
	if _, err := service.CreateEnvironment(ctx, actor, app.SaveEnvironmentInput{Name: "无流程环境", Slug: "missing-sop", Status: "active"}, ""); err == nil || !strings.Contains(err.Error(), "必须绑定已发布 SOP") {
		t.Fatalf("environment without SOP should be rejected, got %v", err)
	}

	createdSOP, err := service.CreateSOP(ctx, actor, app.CreateSOPInput{
		Name:         "文章交付",
		ContentTypes: []string{domain.ContentTypeWeChatArticle},
		Stages:       []domain.StageDefinition{{ID: "draft", Name: "文章草稿", Order: 10, OutputSchema: "contentcloud.article/1.0", ExecutionModes: []string{"local"}, GateIDs: []string{"approval"}}},
		Gates:        []domain.GateDefinition{{ID: "approval", Name: "客户确认", Mode: domain.GateModeRequired, Blocking: false}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.LintSOPVersion(ctx, actor, createdSOP.Definition.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || len(report.Errors) == 0 {
		t.Fatalf("invalid required gate should fail lint: %#v", report)
	}
	createdSOP.Versions[0].Gates[0].Blocking = true
	if _, err := service.SaveSOPVersion(ctx, actor, createdSOP.Definition.ID, 1, app.SaveSOPVersionInput{Name: createdSOP.Versions[0].Name, ContentTypes: createdSOP.Versions[0].ContentTypes, Stages: createdSOP.Versions[0].Stages, Gates: createdSOP.Versions[0].Gates, DefaultExecutionMode: "local"}, ""); err != nil {
		t.Fatal(err)
	}
	report, err = service.LintSOPVersion(ctx, actor, createdSOP.Definition.ID, 1)
	if err != nil || !report.Valid {
		t.Fatalf("corrected SOP should pass lint: report=%#v err=%v", report, err)
	}
}

func TestProjectSOPBindingIsExplicitAndDoesNotRewriteHistoricalTasks(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "project-sop-binding@example.com", "long-enough-password", "项目负责人", "绑定租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "绑定品牌", ProductName: "绑定产品"}, "")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := service.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var article domain.SOPVersion
	for _, summary := range admin.SOPs {
		if summary.Definition.TemplateKey != "article_collaboration" {
			continue
		}
		for _, version := range summary.Versions {
			if version.Status == "published" {
				article = version
				break
			}
		}
	}
	if article.ID == "" {
		t.Fatal("article built-in SOP was not provisioned")
	}
	oldTask, err := service.CreateWorkTask(ctx, actor, app.CreateWorkTaskInput{ProjectID: project.ID, Title: "历史短视频任务", ContentType: domain.ContentTypeVideoScript, InputRefs: []string{"brief:old"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	articleEnvironment, err := service.CreateEnvironment(ctx, actor, app.SaveEnvironmentInput{Name: "文章环境", Slug: "article", Status: "active", DefaultSOPID: article.SOPID, DefaultSOPVersion: article.Version}, "")
	if err != nil {
		t.Fatal(err)
	}
	bound, err := service.BindProjectSOP(ctx, actor, project.ID, app.BindProjectSOPInput{EnvironmentID: articleEnvironment.ID, SOPID: article.SOPID, SOPVersion: article.Version}, "")
	if err != nil {
		t.Fatal(err)
	}
	if bound.Binding.EnvironmentID != articleEnvironment.ID || bound.Binding.SOPDigest != article.Digest || bound.Previous == nil {
		t.Fatalf("explicit project binding did not return current and previous bindings: %#v", bound)
	}
	newTask, err := service.CreateWorkTask(ctx, actor, app.CreateWorkTaskInput{ProjectID: project.ID, Title: "新文章任务", ContentType: domain.ContentTypeWeChatArticle, InputRefs: []string{"brief:new"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if newTask.Task.EnvironmentID != articleEnvironment.ID || newTask.Task.SOPID != article.SOPID || newTask.Task.SOPVersion != article.Version {
		t.Fatalf("new task did not use explicit project binding: %#v", newTask.Task)
	}
	if oldTask.Task.EnvironmentID == newTask.Task.EnvironmentID || oldTask.Task.SOPID == newTask.Task.SOPID {
		t.Fatalf("historical task was rewritten by project rebind: old=%#v new=%#v", oldTask.Task, newTask.Task)
	}
}

func TestTaskSOPOverrideCannotBypassEnvironmentConfiguration(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "task-sop-policy@example.com", "long-enough-password", "项目负责人", "SOP 策略租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "策略品牌", ProductName: "策略产品"}, "")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := service.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var research domain.SOPVersion
	var defaultEnvironment domain.Environment
	for _, summary := range admin.SOPs {
		if summary.Definition.TemplateKey == "content_research" {
			for _, version := range summary.Versions {
				if version.Status == "published" {
					research = version
				}
			}
		}
	}
	defaultEnvironment = admin.Environments[0]
	if research.ID == "" {
		t.Fatal("research built-in SOP was not provisioned")
	}
	if _, err := service.CreateWorkTask(ctx, actor, app.CreateWorkTaskInput{ProjectID: project.ID, SOPID: research.SOPID, SOPVersion: research.Version, Title: "绕过环境的任务", ContentType: domain.ContentTypeVideoScript, InputRefs: []string{"brief:blocked"}}, ""); err == nil || !strings.Contains(err.Error(), "只能使用 Project 绑定或 Environment 配置") {
		t.Fatalf("arbitrary SOP override should be rejected: %v", err)
	}
	if defaultEnvironment.DefaultSOPID == research.SOPID && defaultEnvironment.DefaultSOPVersion == research.Version {
		t.Fatal("test setup accidentally selected research as default")
	}
}

func TestCreateWorkTaskIdempotencyReturnsOriginalOrConflicts(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "task-idempotency@example.com", "long-enough-password", "项目负责人", "任务幂等租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "幂等品牌", ProductName: "幂等产品"}, "")
	if err != nil {
		t.Fatal(err)
	}
	input := app.CreateWorkTaskInput{ProjectID: project.ID, Title: "幂等任务", ContentType: domain.ContentTypeVideoScript, InputRefs: []string{"brief:idempotent"}, IdempotencyKey: "task-create-001"}
	first, err := service.CreateWorkTask(ctx, actor, input, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateWorkTask(ctx, actor, input, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Task.ID != second.Task.ID {
		t.Fatalf("idempotent retry created a second task: first=%s second=%s", first.Task.ID, second.Task.ID)
	}
	input.Title = "不同参数"
	if _, err := service.CreateWorkTask(ctx, actor, input, ""); err == nil || !strings.Contains(err.Error(), "相同 Idempotency-Key 已用于不同任务参数") {
		t.Fatalf("idempotency key reuse with different parameters should conflict: %v", err)
	}
}
