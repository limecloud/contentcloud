package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestOrchestrationBFFVerticalSlice(t *testing.T) {
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

	admin := callBFF[domain.AdminWorkOSView](t, client, http.MethodGet, server.URL+"/api/bff/admin/work-os", nil)
	if len(admin.Environments) != 1 || len(admin.SOPs) != 5 {
		t.Fatalf("unexpected admin data: %#v", admin)
	}
	defaultSOPID := ""
	for _, summary := range admin.SOPs {
		if summary.Definition.TemplateKey == "marketing_video_production" {
			defaultSOPID = summary.Definition.ID
			break
		}
	}
	if defaultSOPID == "" || admin.Environments[0].DefaultSOPID != defaultSOPID {
		t.Fatalf("marketing video built-in SOP was not selected as the default: %#v", admin)
	}
	createdEnvironment := callBFF[domain.Environment](t, client, http.MethodPost, server.URL+"/api/bff/admin/environments", app.SaveEnvironmentInput{Name: "审核环境", Slug: "review", Status: "paused", DefaultSOPID: defaultSOPID, DefaultSOPVersion: 1})
	if createdEnvironment.ID == "" || createdEnvironment.Status != "paused" {
		t.Fatalf("environment create did not persist: %#v", createdEnvironment)
	}
	createdSOP := callBFF[domain.SOPSummary](t, client, http.MethodPost, server.URL+"/api/bff/admin/sops", app.CreateSOPInput{Name: "后台文章流程", ContentTypes: []string{domain.ContentTypeWeChatArticle}})
	if createdSOP.Definition.ID == "" || len(createdSOP.Versions) != 1 || createdSOP.Versions[0].Status != "draft" {
		t.Fatalf("SOP create did not return draft: %#v", createdSOP)
	}
	lint := callBFF[app.SOPLintReport](t, client, http.MethodGet, server.URL+"/api/bff/admin/sops/"+createdSOP.Definition.ID+"/versions/1/lint", nil)
	if !lint.Valid {
		t.Fatalf("default SOP draft should pass structural lint: %#v", lint)
	}
	environment := admin.Environments[0]
	environment.Status = "paused"
	updatedEnvironment := callBFF[domain.Environment](t, client, http.MethodPatch, server.URL+"/api/bff/admin/environments/"+environment.ID, map[string]any{"status": "paused"})
	if updatedEnvironment.Status != "paused" {
		t.Fatalf("environment update did not persist: %#v", updatedEnvironment)
	}
	callBFF[domain.Environment](t, client, http.MethodPatch, server.URL+"/api/bff/admin/environments/"+environment.ID, map[string]any{"status": "active"})

	project := callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", app.CreateProjectInput{BrandName: "后台品牌", ProductName: "后台产品", ContentType: domain.ContentTypeVideoScript})
	projectSOP := callBFF[map[string]any](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/sop", nil)
	if projectSOP["binding"] == nil || projectSOP["sop"] == nil {
		t.Fatalf("project SOP response is incomplete: %#v", projectSOP)
	}
	task := callBFFWithHeaders[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks", app.CreateWorkTaskInput{ProjectID: project.ID, Title: "后台创建任务", InputRefs: []string{"brief:local"}}, map[string]string{"Idempotency-Key": "http-task-1"})
	if task.Task.ID == "" || task.Task.SOPDigest == "" {
		t.Fatalf("task response is incomplete: %#v", task)
	}
	listed := callBFF[[]domain.WorkTask](t, client, http.MethodGet, server.URL+"/api/bff/tasks?project_id="+project.ID, nil)
	if len(listed) != 1 || listed[0].ID != task.Task.ID {
		t.Fatalf("task list did not return created task: %#v", listed)
	}
	fetched := callBFF[app.WorkTaskView](t, client, http.MethodGet, server.URL+"/api/bff/tasks/"+task.Task.ID, nil)
	if fetched.Task.SOPDigest != task.Task.SOPDigest || len(fetched.StageRuns) != 1 {
		t.Fatalf("task detail lost pinned SOP or stage run: %#v", fetched)
	}
	replayed := callBFFWithHeaders[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks", app.CreateWorkTaskInput{ProjectID: project.ID, Title: "后台创建任务", InputRefs: []string{"brief:local"}}, map[string]string{"Idempotency-Key": "http-task-1"})
	if replayed.Task.ID != task.Task.ID {
		t.Fatalf("HTTP Idempotency-Key did not return the original task: first=%s replay=%s", task.Task.ID, replayed.Task.ID)
	}
	conversation := callBFF[domain.ConversationImport](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/conversation-imports", app.CreateConversationImportInput{ClientID: "codex", NodeID: "node-http", Purpose: "task_handoff", RequestedScope: domain.ConversationScopeSummary, AttachAs: domain.ConversationAttachTaskInput, IdempotencyKey: "http-import-1"})
	if conversation.Status != domain.ConversationImportAwaitingConfirmation || conversation.AdapterID != "codex@0.1.0" {
		t.Fatalf("conversation import request was not created: %#v", conversation)
	}
	content := []domain.ConversationContent{{Kind: "summary", Text: "HTTP 客户端已完成脱敏摘要。"}}
	digest, err := domain.CanonicalHash(content)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	bundle := domain.ConversationBundle{SchemaVersion: domain.ConversationBundleSchema, BundleID: "http-bundle-1", ImportID: conversation.ID, Client: domain.ConversationClient{ID: "codex", ClientVersion: "1.0.0", AdapterVersion: "0.1.0", NodeID: "node-http"}, Source: domain.ConversationSource{Format: "codex.events-jsonl/v1", SessionRef: "hmac:http-session-reference"}, Purpose: "task_handoff", Scope: domain.ConversationScope{Mode: domain.ConversationScopeSummary}, Target: domain.ConversationTarget{TaskID: task.Task.ID}, Content: content, Redaction: domain.ConversationRedaction{Applied: true, PolicyDigest: "sha256:" + strings.Repeat("c", 64)}, Consent: domain.ConversationConsent{ConfirmedAt: now}, ContentDigest: "sha256:" + digest, ExportedAt: now.Add(time.Second)}
	uploaded := callBFF[domain.ConversationImport](t, client, http.MethodPost, server.URL+"/api/bff/conversation-imports/"+conversation.ID+"/bundle", bundle)
	if uploaded.Status != domain.ConversationImportUploaded || uploaded.Bundle == nil {
		t.Fatalf("conversation bundle was not accepted: %#v", uploaded)
	}
}

func TestTaskGovernanceBFFActions(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	client := &http.Client{}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	jarClient := &http.Client{Jar: func() http.CookieJar { jar, _ := cookiejar.New(nil); return jar }()}
	bootstrap, err := jarClient.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.Body.Close()
	project := callBFF[domain.Project](t, jarClient, http.MethodPost, server.URL+"/api/bff/projects", app.CreateProjectInput{BrandName: "治理品牌", ProductName: "治理产品", ContentType: domain.ContentTypeVideoScript})
	task := callBFF[app.WorkTaskView](t, jarClient, http.MethodPost, server.URL+"/api/bff/tasks", app.CreateWorkTaskInput{ProjectID: project.ID, Title: "治理链路", ContentType: domain.ContentTypeVideoScript, InputRefs: []string{"brief:api"}})
	task = callBFF[app.WorkTaskView](t, jarClient, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/actions", app.TaskActionInput{Action: "start"})
	if task.Task.Status != domain.TaskStatusRunning || len(task.Runs) != 1 {
		t.Fatalf("start action was not persisted: %#v", task)
	}
	stage := task.StageRuns[0]
	task = callBFF[app.WorkTaskView](t, jarClient, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/stages/"+stage.StageID+"/report", app.StageReportInput{StageRunID: stage.ID, Status: domain.StageRunStatusCompleted, OutputRefs: []string{"local://output"}, Checks: map[string]any{"passed": true}})
	if task.Task.Status != domain.TaskStatusReady {
		t.Fatalf("stage report should advance task: %#v", task.Task)
	}
}

func TestMarketingVideoBFFScriptApprovalCreatesContentSnapshot(t *testing.T) {
	store := memory.New()
	service := app.New(store, slog.Default())
	session, err := service.Register(t.Context(), "marketing-http@example.com", "long-enough-password", "营销负责人", "营销团队")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetTenantContentCapability(t.Context(), domain.TenantContentCapability{TenantID: actor.TenantID, ContentType: domain.ContentTypeMarketingVideo, Enabled: true, UpdatedBy: actor.UserID, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}
	project := callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", app.CreateProjectInput{BrandName: "金陵古都", ProductName: "金陵古都香", Channel: "douyin", ContentType: domain.ContentTypeMarketingVideo})
	now := time.Now().UTC()
	sourceID := domain.NewID()
	sourceRevision := domain.SourceRevision{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, SourceID: sourceID, FileName: "jinling-gudu.md", ObjectKey: "sources/jinling-gudu.md", SHA256: strings.Repeat("1", 64), ByteSize: 32, DeclaredMIME: "text/markdown", DetectedMIME: "text/markdown", ProcessingStatus: "ready", UploadedBy: actor.UserID, CreatedAt: now}
	if err := store.CreateSource(t.Context(), domain.Source{ID: sourceID, TenantID: actor.TenantID, ProjectID: project.ID, Name: "金陵古都参考资料", SourceType: "document", Status: "ready", RevisionCount: 1, LatestRevision: sourceRevision.ID, CreatedAt: now}, sourceRevision); err != nil {
		t.Fatal(err)
	}
	knowledgeObject := domain.KnowledgeObject{ID: "fact:jinling-http", TenantID: actor.TenantID, ProjectID: project.ID, ObjectType: "FactAssertion", Layer: "product", Version: 1, Status: "approved", Title: "金陵文化表达", Statement: "仅使用已核验的南京历史文化表达。", Payload: map[string]any{"scope": "brand_story"}, AllowedChannels: []string{"douyin"}, EvidenceRefs: []string{"evidence:jinling-http"}, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	knowledgeObject.Digest, err = knowledgeObject.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateKnowledgeObject(t.Context(), knowledgeObject); err != nil {
		t.Fatal(err)
	}
	pack := domain.KnowledgePack{ID: "pack:jinling-http", TenantID: actor.TenantID, ProjectID: project.ID, Name: "金陵知识包", Purpose: "marketing_video", Version: 1, Status: "published", ObjectRefs: []domain.KnowledgePackObjectRef{{ObjectID: knowledgeObject.ID, Version: 1}}, QueryPolicy: domain.DefaultKnowledgeQueryPolicy(), CreatedBy: actor.UserID, PublishedBy: actor.UserID, CreatedAt: now, PublishedAt: &now}
	pack.Digest, err = pack.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateKnowledgePack(t.Context(), pack); err != nil {
		t.Fatal(err)
	}
	knowledgeSnapshot, err := domain.BuildKnowledgeSnapshot(pack, []domain.KnowledgeObject{knowledgeObject}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateKnowledgeSnapshot(t.Context(), knowledgeSnapshot); err != nil {
		t.Fatal(err)
	}

	task := callBFF[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks", app.CreateWorkTaskInput{ProjectID: project.ID, Title: "金陵古都香剧本", ContentType: domain.ContentTypeMarketingVideo, InputRefs: []string{"brief:jinling-gudu"}})
	task = callBFF[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/actions", app.TaskActionInput{Action: "start"})
	sourceRun := task.StageRuns[0]
	task = callBFF[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/stages/sources/report", app.StageReportInput{StageRunID: sourceRun.ID, Status: domain.StageRunStatusCompleted, Outputs: []domain.TaskStageOutput{{OutputType: domain.StageOutputSourceRevision, ObjectID: sourceRevision.ID, Role: domain.StageOutputRolePrimary}}, Checks: map[string]any{"source.registered": true}})
	task = callBFF[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/actions", app.TaskActionInput{Action: "start"})
	knowledgeRun := currentHTTPStageRun(t, task, "knowledge")
	task = callBFF[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/stages/knowledge/report", app.StageReportInput{StageRunID: knowledgeRun.ID, Status: domain.StageRunStatusCompleted, Outputs: []domain.TaskStageOutput{{OutputType: domain.StageOutputKnowledgeSnapshot, ObjectID: knowledgeSnapshot.ID, Role: domain.StageOutputRolePrimary}}, Checks: map[string]any{"claim.references": true, "rights.references": true}})
	task = callBFF[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/actions", app.TaskActionInput{Action: "start"})
	scriptBody := json.RawMessage(`{"title":"金陵古都香｜别把南京放回抽屉","scenes":[{"scene":1,"duration_seconds":4,"visual":"明城墙晨光","voiceover":"把一座城的气息，带出抽屉。"}]}`)
	script := callBFF[domain.TaskRevision](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/revisions", app.CreateTaskRevisionInput{ContentType: domain.ContentTypeMarketingVideo, Content: scriptBody, KnowledgeSnapshotIDs: []string{knowledgeSnapshot.ID}})
	task = callBFF[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/stages/script/report", app.StageReportInput{StageRunID: currentHTTPStageRun(t, task, "script").ID, Status: domain.StageRunStatusCompleted, Outputs: []domain.TaskStageOutput{{OutputType: domain.StageOutputSubmissionRevision, ObjectID: script.ID, ObjectVersion: script.RevisionNo, Role: domain.StageOutputRolePrimary}}, Checks: map[string]any{"content.schema": true, "claim.references": true}})
	gate := findHTTPGate(t, task, "script_review")
	task = callBFF[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/gates/"+gate.ID+"/decide", app.GateDecisionInput{Decision: "approved", Reason: "Web 剧本审核通过"})
	if task.Task.CurrentStageID != "storyboard" {
		t.Fatalf("script approval did not advance to storyboard: %#v", task.Task)
	}
	snapshots := callBFF[[]domain.ApprovedSnapshot](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/approved-snapshots?type=content_batch", nil)
	if len(snapshots) != 1 || snapshots[0].SubmissionType != "content_batch" || snapshots[0].ContentHash != script.ContentHash {
		t.Fatalf("Web script approval did not create content_batch snapshot: %#v", snapshots)
	}
}

func currentHTTPStageRun(t *testing.T, task app.WorkTaskView, stageID string) domain.StageRun {
	t.Helper()
	for _, run := range task.StageRuns {
		if run.StageID == stageID {
			return run
		}
	}
	t.Fatalf("stage run %s not found: %#v", stageID, task.StageRuns)
	return domain.StageRun{}
}

func findHTTPGate(t *testing.T, task app.WorkTaskView, gateID string) domain.GateEvaluation {
	t.Helper()
	for _, gate := range task.Gates {
		if gate.GateID == gateID && gate.Status == domain.GateEvaluationPending {
			return gate
		}
	}
	t.Fatalf("pending gate %s not found: %#v", gateID, task.Gates)
	return domain.GateEvaluation{}
}

func TestSOPGovernanceBFFActions(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.Body.Close()

	admin := callBFF[domain.AdminWorkOSView](t, client, http.MethodGet, server.URL+"/api/bff/admin/work-os", nil)
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

	draft := callBFF[domain.SOPVersion](t, client, http.MethodPost, server.URL+"/api/bff/admin/sops/"+sopID+"/versions", map[string]any{"source_version": 1})
	draft.Description = "资料与知识建设升级"
	updated := callBFF[domain.SOPVersion](t, client, http.MethodPatch, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/"+strconv.Itoa(draft.Version), app.SaveSOPVersionInput{Description: draft.Description, Name: draft.Name, ContentTypes: draft.ContentTypes, Stages: draft.Stages, Gates: draft.Gates, DefaultExecutionMode: draft.DefaultExecutionMode})
	if updated.Status != "draft" || updated.Description != draft.Description {
		t.Fatalf("SOP draft update did not persist: %#v", updated)
	}
	published := callBFF[domain.SOPVersion](t, client, http.MethodPost, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/"+strconv.Itoa(draft.Version)+"/publish", map[string]any{})
	if published.Status != "published" || published.Digest == "" {
		t.Fatalf("SOP draft was not published: %#v", published)
	}
	diff := callBFF[app.SOPVersionDiff](t, client, http.MethodGet, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/1/diff/"+strconv.Itoa(published.Version), nil)
	if diff.Same || len(diff.Changes) == 0 {
		t.Fatalf("SOP diff did not expose the change: %#v", diff)
	}
	environment := callBFF[domain.Environment](t, client, http.MethodPatch, server.URL+"/api/bff/admin/environments/"+admin.Environments[0].ID, map[string]any{"default_sop_id": sopID, "default_sop_version": published.Version})
	project := callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", app.CreateProjectInput{BrandName: "升级验证品牌", ProductName: "升级验证产品", ContentType: domain.ContentTypeVideoScript})
	callBFF[map[string]any](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/sop", nil)
	task := callBFF[app.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks", app.CreateWorkTaskInput{ProjectID: project.ID, Title: "升级影响任务", ContentType: domain.ContentTypeVideoScript})
	impact := callBFF[app.SOPVersionImpact](t, client, http.MethodGet, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/"+strconv.Itoa(published.Version)+"/impact", nil)
	if impact.Counts["environments"] != 1 || impact.Counts["projects"] != 1 || impact.Counts["tasks"] != 1 {
		t.Fatalf("SOP impact did not include bindings: %#v", impact)
	}
	rollback := callBFF[app.SOPRollbackResult](t, client, http.MethodPost, server.URL+"/api/bff/admin/sops/"+sopID+"/rollback", map[string]any{"target_version": 1})
	if rollback.TargetVersion != 1 || rollback.Version.Version <= published.Version || rollback.ReboundEnvironments != 1 || rollback.ReboundProjects != 1 {
		t.Fatalf("SOP rollback did not rebind future work: %#v", rollback)
	}
	if task.Task.SOPVersion != published.Version || environment.DefaultSOPVersion != published.Version {
		t.Fatalf("rollback response rewrote caller-side history unexpectedly: task=%#v environment=%#v", task.Task, environment)
	}
	callBFF[map[string]any](t, client, http.MethodPost, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/"+strconv.Itoa(published.Version)+"/retire", map[string]any{})
}
