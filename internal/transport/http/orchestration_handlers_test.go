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

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestOrchestrationBFFVerticalSlice(t *testing.T) {
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

	admin := callBFF[catalogdomain.AdminWorkOSView](t, client, http.MethodGet, server.URL+"/api/bff/admin/work-os", nil)
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
	createdEnvironment := callBFF[catalogdomain.Environment](t, client, http.MethodPost, server.URL+"/api/bff/admin/environments", application.SaveEnvironmentInput{Name: "审核环境", Slug: "review", Status: "paused", DefaultSOPID: defaultSOPID, DefaultSOPVersion: 1})
	if createdEnvironment.ID == "" || createdEnvironment.Status != "paused" {
		t.Fatalf("environment create did not persist: %#v", createdEnvironment)
	}
	createdSOP := callBFF[catalogdomain.SOPSummary](t, client, http.MethodPost, server.URL+"/api/bff/admin/sops", application.CreateSOPInput{Name: "后台文章流程", ContentTypes: []string{identitydomain.ContentTypeWeChatArticle}})
	if createdSOP.Definition.ID == "" || len(createdSOP.Versions) != 1 || createdSOP.Versions[0].Status != "draft" {
		t.Fatalf("SOP create did not return draft: %#v", createdSOP)
	}
	lint := callBFF[application.SOPLintReport](t, client, http.MethodGet, server.URL+"/api/bff/admin/sops/"+createdSOP.Definition.ID+"/versions/1/lint", nil)
	if !lint.Valid {
		t.Fatalf("default SOP draft should pass structural lint: %#v", lint)
	}
	environment := admin.Environments[0]
	environment.Status = "paused"
	updatedEnvironment := callBFF[catalogdomain.Environment](t, client, http.MethodPatch, server.URL+"/api/bff/admin/environments/"+environment.ID, map[string]any{"status": "paused"})
	if updatedEnvironment.Status != "paused" {
		t.Fatalf("environment update did not persist: %#v", updatedEnvironment)
	}
	callBFF[catalogdomain.Environment](t, client, http.MethodPatch, server.URL+"/api/bff/admin/environments/"+environment.ID, map[string]any{"status": "active"})

	project := callBFF[workspacedomain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", application.CreateProjectInput{BrandName: "后台品牌", ProductName: "后台产品", ContentType: identitydomain.ContentTypeVideoScript})
	projectSOP := callBFF[map[string]any](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/sop", nil)
	if projectSOP["binding"] == nil || projectSOP["sop"] == nil {
		t.Fatalf("project SOP response is incomplete: %#v", projectSOP)
	}
	task := callBFFWithHeaders[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks", application.CreateWorkTaskInput{ProjectID: project.ID, Title: "后台创建任务", InputRefs: []string{"brief:local"}}, map[string]string{"Idempotency-Key": "http-task-1"})
	if task.Task.ID == "" || task.Task.SOPDigest == "" {
		t.Fatalf("task response is incomplete: %#v", task)
	}
	listed := callBFF[[]work.WorkTask](t, client, http.MethodGet, server.URL+"/api/bff/tasks?project_id="+project.ID, nil)
	if len(listed) != 1 || listed[0].ID != task.Task.ID {
		t.Fatalf("task list did not return created task: %#v", listed)
	}
	fetched := callBFF[application.WorkTaskView](t, client, http.MethodGet, server.URL+"/api/bff/tasks/"+task.Task.ID, nil)
	if fetched.Task.SOPDigest != task.Task.SOPDigest || len(fetched.StageRuns) != 1 {
		t.Fatalf("task detail lost pinned SOP or stage run: %#v", fetched)
	}
	replayed := callBFFWithHeaders[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks", application.CreateWorkTaskInput{ProjectID: project.ID, Title: "后台创建任务", InputRefs: []string{"brief:local"}}, map[string]string{"Idempotency-Key": "http-task-1"})
	if replayed.Task.ID != task.Task.ID {
		t.Fatalf("HTTP Idempotency-Key did not return the original task: first=%s replay=%s", task.Task.ID, replayed.Task.ID)
	}
	conversation := callBFF[work.ConversationImport](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/conversation-imports", application.CreateConversationImportInput{ClientID: "codex", NodeID: "node-http", Purpose: "task_handoff", RequestedScope: work.ConversationScopeSummary, AttachAs: work.ConversationAttachTaskInput, IdempotencyKey: "http-import-1"})
	if conversation.Status != work.ConversationImportAwaitingConfirmation || conversation.AdapterID != "codex@0.1.0" {
		t.Fatalf("conversation import request was not created: %#v", conversation)
	}
	content := []work.ConversationContent{{Kind: "summary", Text: "HTTP 客户端已完成脱敏摘要。"}}
	digest, err := stablehash.Sum(content)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	bundle := work.ConversationBundle{SchemaVersion: work.ConversationBundleSchema, BundleID: "http-bundle-1", ImportID: conversation.ID, Client: work.ConversationClient{ID: "codex", ClientVersion: "1.0.0", AdapterVersion: "0.1.0", NodeID: "node-http"}, Source: work.ConversationSource{Format: "codex.events-jsonl/v1", SessionRef: "hmac:http-session-reference"}, Purpose: "task_handoff", Scope: work.ConversationScope{Mode: work.ConversationScopeSummary}, Target: work.ConversationTarget{TaskID: task.Task.ID}, Content: content, Redaction: work.ConversationRedaction{Applied: true, PolicyDigest: "sha256:" + strings.Repeat("c", 64)}, Consent: work.ConversationConsent{ConfirmedAt: now}, ContentDigest: "sha256:" + digest, ExportedAt: now.Add(time.Second)}
	uploaded := callBFF[work.ConversationImport](t, client, http.MethodPost, server.URL+"/api/bff/conversation-imports/"+conversation.ID+"/bundle", bundle)
	if uploaded.Status != work.ConversationImportUploaded || uploaded.Bundle == nil {
		t.Fatalf("conversation bundle was not accepted: %#v", uploaded)
	}
}

func TestOperationsExecutorBFFUsesRegisteredDevices(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	session, err := service.Identity.Register(t.Context(), "executor-bff@example.com", "long-enough-password", "执行端运营", "执行端客户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	device := workspacedomain.Device{ID: idgen.New(), TenantID: actor.TenantID, OwnerUserID: actor.UserID, DisplayName: "内容工作站", Hostname: "content.local", Platform: "darwin", Arch: "arm64", Version: "0.21.0", Capabilities: []catalogdomain.Capability{{ID: "script_generation", Version: "1.0.0", Kind: "business_capability"}}, ProjectIDs: []string{}, LastSeenAt: now}
	if err := store.SaveDevice(t.Context(), device); err != nil {
		t.Fatal(err)
	}
	instance := workspacedomain.DaemonInstance{ID: idgen.New(), TenantID: actor.TenantID, DeviceID: device.ID, ConnectionEpoch: 1, ReportSequence: 1, Version: device.Version, State: "connected", Capabilities: map[string]any{"environment_status": "ready"}, StartedAt: now.Add(-time.Minute), LastSeenAt: now}
	if err := store.SaveDaemonInstance(t.Context(), instance); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}

	directory := callBFF[application.OperationsExecutorDirectory](t, client, http.MethodGet, server.URL+"/api/bff/operations/executors", nil)
	if len(directory.Executors) != 1 || directory.Executors[0].ID != device.ID || directory.Executors[0].Status != "online" {
		t.Fatalf("operations executor directory did not return registered device facts: %#v", directory)
	}
	detail := callBFF[application.OperationsExecutor](t, client, http.MethodGet, server.URL+"/api/bff/operations/executors/"+device.ID, nil)
	if detail.DisplayName != "内容工作站" || detail.Hostname != "content.local" || len(detail.Capabilities) != 1 {
		t.Fatalf("operations executor detail is incomplete: %#v", detail)
	}
}

func TestOperationsSkillBFFReturnsExplicitUnconfiguredDirectory(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("skill-bff@example.com"))
	session, err := service.Identity.Register(t.Context(), "skill-bff@example.com", "long-enough-password", "技能包运营", "技能包客户")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}

	directory := callBFF[application.OperationsSkillDirectory](t, client, http.MethodGet, server.URL+"/api/bff/operations/skills", nil)
	if directory.Configured || directory.Skills == nil || len(directory.Skills) != 0 {
		t.Fatalf("unconfigured skill BFF must return an explicit empty directory: %#v", directory)
	}
}

func TestTaskGovernanceBFFActions(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
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
	project := callBFF[workspacedomain.Project](t, jarClient, http.MethodPost, server.URL+"/api/bff/projects", application.CreateProjectInput{BrandName: "治理品牌", ProductName: "治理产品", ContentType: identitydomain.ContentTypeVideoScript})
	task := callBFF[application.WorkTaskView](t, jarClient, http.MethodPost, server.URL+"/api/bff/tasks", application.CreateWorkTaskInput{ProjectID: project.ID, Title: "治理链路", ContentType: identitydomain.ContentTypeVideoScript, InputRefs: []string{"brief:api"}})
	task = callBFF[application.WorkTaskView](t, jarClient, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/actions", application.TaskActionInput{Action: "start"})
	if task.Task.Status != work.TaskStatusRunning || len(task.Runs) != 1 {
		t.Fatalf("start action was not persisted: %#v", task)
	}
	stage := task.StageRuns[0]
	task = callBFF[application.WorkTaskView](t, jarClient, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/stages/"+stage.StageID+"/report", application.StageReportInput{StageRunID: stage.ID, Status: work.StageRunStatusCompleted, OutputRefs: []string{"local://output"}, Checks: map[string]any{"passed": true}})
	if task.Task.Status != work.TaskStatusReady {
		t.Fatalf("stage report should advance task: %#v", task.Task)
	}
}

func TestMarketingVideoBFFScriptApprovalCreatesContentSnapshot(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	session, err := service.Identity.Register(t.Context(), "marketing-http@example.com", "long-enough-password", "营销负责人", "营销团队")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetTenantContentCapability(t.Context(), identitydomain.TenantContentCapability{TenantID: actor.TenantID, ContentType: identitydomain.ContentTypeMarketingVideo, Enabled: true, UpdatedBy: actor.UserID, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}
	project := callBFF[workspacedomain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", application.CreateProjectInput{BrandName: "金陵古都", ProductName: "金陵古都香", Channel: "douyin", ContentType: identitydomain.ContentTypeMarketingVideo})
	now := time.Now().UTC()
	sourceID := idgen.New()
	sourceRevision := sourcedomain.SourceRevision{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: project.ID, SourceID: sourceID, FileName: "jinling-gudu.md", ObjectKey: "sources/jinling-gudu.md", SHA256: strings.Repeat("1", 64), ByteSize: 32, DeclaredMIME: "text/markdown", DetectedMIME: "text/markdown", ProcessingStatus: "ready", UploadedBy: actor.UserID, CreatedAt: now}
	if err := store.CreateSource(t.Context(), sourcedomain.Source{ID: sourceID, TenantID: actor.TenantID, ProjectID: project.ID, Name: "金陵古都参考资料", SourceType: "document", Status: "ready", RevisionCount: 1, LatestRevision: sourceRevision.ID, CreatedAt: now}, sourceRevision); err != nil {
		t.Fatal(err)
	}
	knowledgeObject := sourcedomain.KnowledgeObject{ID: "fact:jinling-http", TenantID: actor.TenantID, ProjectID: project.ID, ObjectType: "FactAssertion", Layer: "product", Version: 1, Status: "approved", Title: "金陵文化表达", Statement: "仅使用已核验的南京历史文化表达。", Payload: map[string]any{"scope": "brand_story"}, AllowedChannels: []string{"douyin"}, EvidenceRefs: []string{"evidence:jinling-http"}, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	knowledgeObject.Digest, err = knowledgeObject.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateKnowledgeObject(t.Context(), knowledgeObject); err != nil {
		t.Fatal(err)
	}
	pack := sourcedomain.KnowledgePack{ID: "pack:jinling-http", TenantID: actor.TenantID, ProjectID: project.ID, Name: "金陵知识包", Purpose: "marketing_video", Version: 1, Status: "published", ObjectRefs: []sourcedomain.KnowledgePackObjectRef{{ObjectID: knowledgeObject.ID, Version: 1}}, QueryPolicy: sourcedomain.DefaultKnowledgeQueryPolicy(), CreatedBy: actor.UserID, PublishedBy: actor.UserID, CreatedAt: now, PublishedAt: &now}
	pack.Digest, err = pack.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateKnowledgePack(t.Context(), pack); err != nil {
		t.Fatal(err)
	}
	knowledgeSnapshot, err := sourcedomain.BuildKnowledgeSnapshot(pack, []sourcedomain.KnowledgeObject{knowledgeObject}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateKnowledgeSnapshot(t.Context(), knowledgeSnapshot); err != nil {
		t.Fatal(err)
	}

	task := callBFF[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks", application.CreateWorkTaskInput{ProjectID: project.ID, Title: "金陵古都香剧本", ContentType: identitydomain.ContentTypeMarketingVideo, InputRefs: []string{"brief:jinling-gudu"}})
	task = callBFF[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/actions", application.TaskActionInput{Action: "start"})
	sourceRun := task.StageRuns[0]
	task = callBFF[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/stages/sources/report", application.StageReportInput{StageRunID: sourceRun.ID, Status: work.StageRunStatusCompleted, Outputs: []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputSourceRevision, ObjectID: sourceRevision.ID, Role: catalogdomain.StageOutputRolePrimary}}, Checks: map[string]any{"source.registered": true}})
	task = callBFF[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/actions", application.TaskActionInput{Action: "start"})
	knowledgeRun := currentHTTPStageRun(t, task, "knowledge")
	task = callBFF[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/stages/knowledge/report", application.StageReportInput{StageRunID: knowledgeRun.ID, Status: work.StageRunStatusCompleted, Outputs: []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputKnowledgeSnapshot, ObjectID: knowledgeSnapshot.ID, Role: catalogdomain.StageOutputRolePrimary}}, Checks: map[string]any{"claim.references": true, "rights.references": true}})
	task = callBFF[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/actions", application.TaskActionInput{Action: "start"})
	scriptBody := json.RawMessage(`{"title":"金陵古都香｜别把南京放回抽屉","scenes":[{"scene":1,"duration_seconds":4,"visual":"明城墙晨光","voiceover":"把一座城的气息，带出抽屉。"}]}`)
	script := callBFF[reviewdomain.TaskRevision](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/revisions", application.CreateTaskRevisionInput{ContentType: identitydomain.ContentTypeMarketingVideo, Content: scriptBody, KnowledgeSnapshotIDs: []string{knowledgeSnapshot.ID}})
	task = callBFF[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/stages/script/report", application.StageReportInput{StageRunID: currentHTTPStageRun(t, task, "script").ID, Status: work.StageRunStatusCompleted, Outputs: []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputSubmissionRevision, ObjectID: script.ID, ObjectVersion: script.RevisionNo, Role: catalogdomain.StageOutputRolePrimary}}, Checks: map[string]any{"content.schema": true, "claim.references": true}})
	gate := findHTTPGate(t, task, "script_review")
	task = callBFF[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks/"+task.Task.ID+"/gates/"+gate.ID+"/decide", application.GateDecisionInput{Decision: "approved", Reason: "Web 剧本审核通过"})
	if task.Task.CurrentStageID != "storyboard" {
		t.Fatalf("script approval did not advance to storyboard: %#v", task.Task)
	}
	snapshots := callBFF[[]reviewdomain.ApprovedSnapshot](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/approved-snapshots?type=content_batch", nil)
	if len(snapshots) != 1 || snapshots[0].SubmissionType != "content_batch" || snapshots[0].ContentHash != script.ContentHash {
		t.Fatalf("Web script approval did not create content_batch snapshot: %#v", snapshots)
	}
}

func currentHTTPStageRun(t *testing.T, task application.WorkTaskView, stageID string) work.StageRun {
	t.Helper()
	for _, run := range task.StageRuns {
		if run.StageID == stageID {
			return run
		}
	}
	t.Fatalf("stage run %s not found: %#v", stageID, task.StageRuns)
	return work.StageRun{}
}

func findHTTPGate(t *testing.T, task application.WorkTaskView, gateID string) reviewdomain.GateEvaluation {
	t.Helper()
	for _, gate := range task.Gates {
		if gate.GateID == gateID && gate.Status == reviewdomain.GateEvaluationPending {
			return gate
		}
	}
	t.Fatalf("pending gate %s not found: %#v", gateID, task.Gates)
	return reviewdomain.GateEvaluation{}
}

func TestSOPGovernanceBFFActions(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.Body.Close()

	admin := callBFF[catalogdomain.AdminWorkOSView](t, client, http.MethodGet, server.URL+"/api/bff/admin/work-os", nil)
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

	draft := callBFF[catalogdomain.SOPVersion](t, client, http.MethodPost, server.URL+"/api/bff/admin/sops/"+sopID+"/versions", map[string]any{"source_version": 1})
	draft.Description = "资料与知识建设升级"
	updated := callBFF[catalogdomain.SOPVersion](t, client, http.MethodPatch, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/"+strconv.Itoa(draft.Version), application.SaveSOPVersionInput{Description: draft.Description, Name: draft.Name, ContentTypes: draft.ContentTypes, Stages: draft.Stages, Gates: draft.Gates, DefaultExecutionMode: draft.DefaultExecutionMode})
	if updated.Status != "draft" || updated.Description != draft.Description {
		t.Fatalf("SOP draft update did not persist: %#v", updated)
	}
	published := callBFF[catalogdomain.SOPVersion](t, client, http.MethodPost, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/"+strconv.Itoa(draft.Version)+"/publish", map[string]any{})
	if published.Status != "published" || published.Digest == "" {
		t.Fatalf("SOP draft was not published: %#v", published)
	}
	preview := callBFF[application.SOPVersionPreview](t, client, http.MethodGet, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/"+strconv.Itoa(published.Version)+"/preview?environment_id="+admin.Environments[0].ID, nil)
	if preview.SOP.Version != published.Version || !preview.Lint.Valid || len(preview.Environments) != 1 {
		t.Fatalf("SOP preview response is incomplete: %#v", preview)
	}
	if preview.SelectedEnvironmentID != admin.Environments[0].ID || !preview.Publishable || len(preview.RequiredCapabilities) != 0 || !preview.Environments[0].Ready {
		t.Fatalf("preview should preserve the honest no-capability binding fact: %#v", preview)
	}
	diff := callBFF[application.SOPVersionDiff](t, client, http.MethodGet, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/1/diff/"+strconv.Itoa(published.Version), nil)
	if diff.Same || len(diff.Changes) == 0 {
		t.Fatalf("SOP diff did not expose the change: %#v", diff)
	}
	environment := callBFF[catalogdomain.Environment](t, client, http.MethodPatch, server.URL+"/api/bff/admin/environments/"+admin.Environments[0].ID, map[string]any{"default_sop_id": sopID, "default_sop_version": published.Version})
	project := callBFF[workspacedomain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", application.CreateProjectInput{BrandName: "升级验证品牌", ProductName: "升级验证产品", ContentType: identitydomain.ContentTypeVideoScript})
	callBFF[map[string]any](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/sop", nil)
	task := callBFF[application.WorkTaskView](t, client, http.MethodPost, server.URL+"/api/bff/tasks", application.CreateWorkTaskInput{ProjectID: project.ID, Title: "升级影响任务", ContentType: identitydomain.ContentTypeVideoScript})
	impact := callBFF[application.SOPVersionImpact](t, client, http.MethodGet, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/"+strconv.Itoa(published.Version)+"/impact", nil)
	if impact.Counts["environments"] != 1 || impact.Counts["projects"] != 1 || impact.Counts["tasks"] != 1 {
		t.Fatalf("SOP impact did not include bindings: %#v", impact)
	}
	rollback := callBFF[application.SOPRollbackResult](t, client, http.MethodPost, server.URL+"/api/bff/admin/sops/"+sopID+"/rollback", map[string]any{"target_version": 1})
	if rollback.TargetVersion != 1 || rollback.Version.Version <= published.Version || rollback.ReboundEnvironments != 1 || rollback.ReboundProjects != 1 {
		t.Fatalf("SOP rollback did not rebind future work: %#v", rollback)
	}
	if task.Task.SOPVersion != published.Version || environment.DefaultSOPVersion != published.Version {
		t.Fatalf("rollback response rewrote caller-side history unexpectedly: task=%#v environment=%#v", task.Task, environment)
	}
	callBFF[map[string]any](t, client, http.MethodPost, server.URL+"/api/bff/admin/sops/"+sopID+"/versions/"+strconv.Itoa(published.Version)+"/retire", map[string]any{})
}
