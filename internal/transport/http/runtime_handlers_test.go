package httpapi_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/testsupport"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
)

func TestRuntimeCleanupDiagnosticsBFFIsTenantScopedAndRetryable(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), nil, application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	if len(bootstrap.Projects) == 0 {
		t.Fatal("bootstrap did not return a project")
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), sessionIDFromJar(t, jar, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	diagnostic := contentruntime.RuntimeCleanupDiagnostic{
		ID: "cleanup-http-1", TenantID: actor.TenantID, ProjectID: bootstrap.Projects[0].ID, TaskID: "task-http-1", RequestID: "request-http-1",
		ManifestDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ObjectKey:      "media/" + actor.TenantID + "/final/http.mp4", CauseCode: "FINAL_RENDER_STORE_FAILED", CauseSummary: "最终成片事实写入失败",
		Status: contentruntime.RuntimeCleanupPending, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	if err := store.CreateRuntimeCleanupDiagnostic(t.Context(), diagnostic); err != nil {
		t.Fatal(err)
	}
	items := callBFF[[]contentruntime.RuntimeCleanupDiagnostic](t, client, http.MethodGet, server.URL+"/api/bff/runtime/cleanup-diagnostics?status=pending", nil)
	if len(items) != 1 || items[0].ID != diagnostic.ID {
		t.Fatalf("cleanup diagnostic list = %#v", items)
	}
	detail := callBFF[contentruntime.RuntimeCleanupDiagnostic](t, client, http.MethodGet, server.URL+"/api/bff/runtime/cleanup-diagnostics/"+diagnostic.ID, nil)
	if detail.ObjectKey != diagnostic.ObjectKey || detail.TenantID != actor.TenantID {
		t.Fatalf("cleanup diagnostic detail = %#v", detail)
	}
	retry := callBFF[application.RuntimeCleanupDiagnosticSummary](t, client, http.MethodPost, server.URL+"/api/bff/runtime/cleanup-diagnostics/"+diagnostic.ID+"/retry", nil)
	if retry.Diagnostic.Status != contentruntime.RuntimeCleanupNotFound || !retry.DeleteAttempted {
		t.Fatalf("cleanup diagnostic retry = %#v", retry)
	}
}

func TestRuntimeCleanupDiagnosticsBFFRejectsCrossTenantReadAndRetry(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), nil)
	ownerSession, err := service.Identity.Register(t.Context(), "cleanup-owner@example.com", "long-enough-password", "Cleanup Owner", "Cleanup Owner Tenant")
	if err != nil {
		t.Fatal(err)
	}
	ownerActor, _, err := service.Identity.SessionActor(t.Context(), ownerSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), ownerActor, application.CreateProjectInput{BrandName: "Cleanup Brand", ProductName: "Cleanup Product"}, "cleanup-project")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	diagnostic := contentruntime.RuntimeCleanupDiagnostic{
		ID: "cleanup-cross-tenant", TenantID: ownerActor.TenantID, ProjectID: project.ID, TaskID: "task-cross-tenant", RequestID: "request-cross-tenant",
		ManifestDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ObjectKey:      "media/" + ownerActor.TenantID + "/final/cross-tenant.mp4", CauseCode: "FINAL_RENDER_STORE_FAILED", CauseSummary: "最终成片事实写入失败",
		Status: contentruntime.RuntimeCleanupPending, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	if err := store.CreateRuntimeCleanupDiagnostic(t.Context(), diagnostic); err != nil {
		t.Fatal(err)
	}

	foreignSession, err := service.Identity.Register(t.Context(), "cleanup-foreign@example.com", "long-enough-password", "Foreign Operator", "Foreign Tenant")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, nil, false, "").Handler())
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	foreignJar, _ := cookiejar.New(nil)
	foreignJar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: foreignSession.ID, Path: "/"}})
	foreignClient := &http.Client{Jar: foreignJar}

	listed := callBFF[[]contentruntime.RuntimeCleanupDiagnostic](t, foreignClient, http.MethodGet, server.URL+"/api/bff/runtime/cleanup-diagnostics?status=pending", nil)
	if len(listed) != 0 {
		t.Fatalf("cross-tenant cleanup list leaked diagnostics: %#v", listed)
	}
	for _, target := range []string{
		server.URL + "/api/bff/runtime/cleanup-diagnostics/" + diagnostic.ID,
		server.URL + "/api/bff/runtime/cleanup-diagnostics/" + diagnostic.ID + "/retry",
	} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
		if strings.HasSuffix(target, "/retry") {
			request, err = http.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader("{}"))
		}
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := foreignClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant cleanup endpoint %s returned %d, want 404: %s", target, response.StatusCode, body)
		}
		if strings.Contains(string(body), diagnostic.ID) || strings.Contains(string(body), diagnostic.ObjectKey) || strings.Contains(string(body), diagnostic.ManifestDigest) {
			t.Fatalf("cross-tenant cleanup endpoint leaked diagnostic data: %s", body)
		}
	}
}

func TestRuntimeCleanupDiagnosticsBFFRejectsDeviceToken(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), nil)
	session, err := service.Identity.Register(t.Context(), "cleanup-device@example.com", "long-enough-password", "Device Owner", "Device Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Device Brand", ProductName: "Device Product"}, "cleanup-device-project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.Workspace.CreateConnectSession(t.Context(), actor, project.ID, "cleanup-device-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, application.ConnectDeviceInput{Hostname: "cleanup-device", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	diagnostic := contentruntime.RuntimeCleanupDiagnostic{
		ID: "cleanup-device-token", TenantID: actor.TenantID, ProjectID: project.ID, TaskID: "task-device-token", RequestID: "request-device-token",
		ManifestDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ObjectKey:      "media/" + actor.TenantID + "/final/device-token.mp4", CauseCode: "FINAL_RENDER_STORE_FAILED", CauseSummary: "最终成片事实写入失败",
		Status: contentruntime.RuntimeCleanupPending, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	if err := store.CreateRuntimeCleanupDiagnostic(t.Context(), diagnostic); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(service, nil, false, "").Handler())
	defer server.Close()
	client := &http.Client{}
	requests := []struct {
		method string
		target string
	}{
		{method: http.MethodGet, target: server.URL + "/api/bff/runtime/cleanup-diagnostics?status=pending"},
		{method: http.MethodGet, target: server.URL + "/api/bff/runtime/cleanup-diagnostics/" + diagnostic.ID},
		{method: http.MethodPost, target: server.URL + "/api/bff/runtime/cleanup-diagnostics/" + diagnostic.ID + "/retry"},
	}
	for _, item := range requests {
		request, err := http.NewRequestWithContext(t.Context(), item.method, item.target, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+connected.DeviceToken)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("device token reached cleanup BFF %s: status=%d body=%s", item.target, response.StatusCode, body)
		}
		if strings.Contains(string(body), diagnostic.ID) || strings.Contains(string(body), diagnostic.ObjectKey) || strings.Contains(string(body), diagnostic.ManifestDigest) {
			t.Fatalf("device token response leaked cleanup diagnostic data: %s", body)
		}
	}

	persisted, err := store.RuntimeCleanupDiagnostic(t.Context(), actor.TenantID, diagnostic.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != contentruntime.RuntimeCleanupPending || persisted.Version != diagnostic.Version || persisted.AttemptCount != 0 {
		t.Fatalf("device token changed cleanup diagnostic: %#v", persisted)
	}
}

func TestRuntimeExplorerBFFShowsOperationsProjection(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), nil, application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	task := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "Runtime 观察任务", Goal: "验证运营后台能够观察执行实例"})
	list := callBFF[application.RuntimeJobList](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	job := runtimeJobForTask(t, list, task.Task.ID)
	detail := callBFF[application.RuntimeJobDetail](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+job.ID, nil)
	if detail.Summary.PlanDigest == "" || detail.Summary.BindingDigest == "" || detail.Summary.InputDigest == "" || detail.Summary.RuntimePolicyID == "" || detail.Summary.RootJobRunID != detail.Summary.ID || detail.Summary.ContractMajor < 1 || len(detail.Nodes) == 0 || len(detail.Events) == 0 {
		t.Fatalf("runtime detail is incomplete: %#v", detail)
	}
	if !containsStringValue(detail.Summary.AllowedActions, "replay") || !containsStringValue(detail.Summary.AllowedActions, "cancel") {
		t.Fatalf("runtime detail did not return server-authorized actions: %#v", detail.Summary.AllowedActions)
	}
	if detail.Agents == nil {
		t.Fatal("runtime detail must expose an initialized agent projection")
	}
	if _, ok := detail.Events[0].Payload["token"]; ok {
		t.Fatal("runtime event payload leaked a token field")
	}
	nodePage := callBFF[application.RuntimeExplorerPage[application.RuntimeNodeView]](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+job.ID+"/nodes?limit=1", nil)
	if len(nodePage.Items) != 1 || (len(detail.Nodes) > 1 && nodePage.NextAfter != 1) {
		t.Fatalf("runtime node pagination is invalid: %#v", nodePage)
	}
	events := callBFF[[]application.RuntimeEventView](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+job.ID+"/events?limit=1", nil)
	if len(events) != 1 {
		t.Fatalf("runtime event limit was not applied: %#v", events)
	}
	callBFF[application.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/runtime/jobs/"+job.ID+"/refresh", nil)
}

func TestRuntimePauseBFFKeepsBusinessProjectionAndRequiresExplicitResume(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), nil, application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	task := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{
		ExperienceID: bootstrap.Experiences[0].ID,
		ProjectID:    bootstrap.Projects[0].ID,
		Title:        "Runtime 暂停控制",
		Goal:         "验证暂停不会被刷新自动恢复",
	})
	list := callBFF[application.RuntimeJobList](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	jobID := runtimeJobForTask(t, list, task.Task.ID).ID
	initial := callBFF[application.RuntimeJobDetail](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+jobID, nil)
	if initial.Summary.CustomerName == "" || initial.Summary.ProjectName == "" || initial.Summary.ProductName == "" || initial.Summary.CurrentStepName == "" {
		t.Fatalf("business projection is incomplete: %#v", initial.Summary)
	}
	if initial.Summary.TotalSteps <= 0 || initial.Summary.CompletedSteps < 0 || initial.Summary.CompletedSteps > initial.Summary.TotalSteps || initial.Summary.RecommendedAction == "" {
		t.Fatalf("business progress projection is invalid: %#v", initial.Summary)
	}
	if initial.Attempts == nil || initial.Gates == nil || initial.StateCollections == nil {
		t.Fatalf("runtime detail collections must be initialized: attempts=%#v gates=%#v state=%#v", initial.Attempts, initial.Gates, initial.StateCollections)
	}
	if !containsStringValue(initial.Summary.AllowedActions, "pause") {
		t.Fatalf("running job must authorize pause: %#v", initial.Summary.AllowedActions)
	}

	paused := callBFF[application.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/runtime/jobs/"+jobID+"/pause", nil)
	if paused.Summary.State != contentruntime.JobRunPaused || !containsStringValue(paused.Summary.AllowedActions, "resume") || containsStringValue(paused.Summary.AllowedActions, "pause") {
		t.Fatalf("pause did not change authorized actions: state=%s actions=%#v", paused.Summary.State, paused.Summary.AllowedActions)
	}
	if paused.Summary.BlockingReason == "" || paused.Summary.RecommendedAction == "" {
		t.Fatalf("paused job must expose business guidance: %#v", paused.Summary)
	}

	refreshed := callBFF[application.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/runtime/jobs/"+jobID+"/refresh", nil)
	if refreshed.Summary.State != contentruntime.JobRunPaused || !containsStringValue(refreshed.Summary.AllowedActions, "resume") {
		t.Fatalf("refresh must preserve paused state: state=%s actions=%#v", refreshed.Summary.State, refreshed.Summary.AllowedActions)
	}

	resumed := callBFF[application.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/runtime/jobs/"+jobID+"/resume", nil)
	if resumed.Summary.State != contentruntime.JobRunRunning || !containsStringValue(resumed.Summary.AllowedActions, "pause") || containsStringValue(resumed.Summary.AllowedActions, "resume") {
		t.Fatalf("resume did not restore running actions: state=%s actions=%#v", resumed.Summary.State, resumed.Summary.AllowedActions)
	}
}

func TestRuntimeExplorerProjectsAgentContextWithoutSessionReference(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), nil, application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	task := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "Runtime Agent 观察", Goal: "验证智能体投影脱敏"})
	list := callBFF[application.RuntimeJobList](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	job := runtimeJobForTask(t, list, task.Task.ID)
	initial := callBFF[application.RuntimeJobDetail](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+job.ID, nil)
	actor, _, err := service.Identity.SessionActor(t.Context(), sessionIDFromJar(t, jar, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC()
	view, err := service.Runtime.Runtime().CreateContextView(t.Context(), contentruntime.ContextViewInput{TenantID: actor.TenantID, JobRunID: job.ID, NodeRunID: initial.Nodes[0].ID, AttemptID: "attempt-http-agent", AllowedTools: []string{"state.get"}, MaxTokens: 2048, BudgetMinor: 50, CreatedAt: created, ExpiresAt: created.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Runtime.Runtime().CreateAgentInstance(t.Context(), contentruntime.AgentInstanceInput{TenantID: actor.TenantID, JobRunID: job.ID, NodeRunID: initial.Nodes[0].ID, Role: "supervisor", HarnessKind: "fake", SessionRef: "opaque-session-token", ExecutionProfileID: "profile-http", ContextViewID: view.ID, RemainingDescendants: 1, BudgetMinor: 50}); err != nil {
		t.Fatal(err)
	}
	detail := callBFF[application.RuntimeJobDetail](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+job.ID, nil)
	if len(detail.Agents) != 1 || !detail.Agents[0].SessionBound || detail.Agents[0].ContextView.AllowedTools[0] != "state.get" {
		t.Fatalf("agent projection is incomplete: %#v", detail.Agents)
	}
	response, err := client.Get(server.URL + "/api/bff/runtime/jobs/" + job.ID)
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

func TestRuntimeRecoveryBFFForksFromCheckpointAndReplaysDurableEvents(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), nil, application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	task := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "Runtime 恢复", Goal: "验证检查点 Fork 和只读 Replay"})
	list := callBFF[application.RuntimeJobList](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	job := runtimeJobForTask(t, list, task.Task.ID)
	actor, _, err := service.Identity.SessionActor(t.Context(), sessionIDFromJar(t, jar, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	source := callBFF[application.RuntimeJobDetail](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+job.ID, nil)
	checkpoint, err := service.Runtime.Runtime().Checkpoint(t.Context(), actor.TenantID, source.Summary.ID, source.Nodes[0].NodeKey, []string{"state:brief"}, []string{"output:brief"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled := callBFF[application.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/runtime/jobs/"+source.Summary.ID+"/cancel", nil)
	if len(cancelled.Checkpoints) != 1 || !containsStringValue(cancelled.Checkpoints[0].AllowedActions, "fork") || cancelled.Checkpoints[0].BlockedReason != "" {
		t.Fatalf("server did not authorize the safe checkpoint fork: %#v", cancelled.Checkpoints)
	}
	forked := callBFF[application.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/runtime/checkpoints/"+checkpoint.ID+"/fork", map[string]string{"idempotency_key": "fork-" + checkpoint.ID})
	if forked.Summary.ID == source.Summary.ID || forked.Summary.WorkTaskID != source.Summary.WorkTaskID {
		t.Fatalf("checkpoint fork did not create the expected execution: %#v", forked.Summary)
	}
	if forked.Summary.RootJobRunID != source.Summary.ID || forked.Summary.BindingDigest != source.Summary.BindingDigest || forked.Summary.InputDigest != source.Summary.InputDigest || forked.Summary.RuntimePolicyID != source.Summary.RuntimePolicyID {
		t.Fatalf("checkpoint fork did not retain the frozen execution identity: %#v", forked.Summary)
	}
	replayed := callBFF[application.RuntimeReplayResult](t, client, http.MethodPost, server.URL+"/api/bff/runtime/jobs/"+source.Summary.ID+"/replay", nil)
	if replayed.JobRunID != source.Summary.ID || replayed.EventCount == 0 || replayed.ExternalCalls != 0 || !replayed.ProjectionRebuilt || replayed.IntegrityStatus != "verified" {
		t.Fatalf("replay must only expose durable events: %#v", replayed)
	}
}

func containsStringValue(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestRuntimeRecoveryBFFStartsUnknownEffectReconciliation(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), nil, application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	task := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "未知副作用对账", Goal: "验证不会盲目重试"})
	list := callBFF[application.RuntimeJobList](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	job := runtimeJobForTask(t, list, task.Task.ID)
	detail := callBFF[application.RuntimeJobDetail](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+job.ID, nil)
	actor, _, err := service.Identity.SessionActor(t.Context(), sessionIDFromJar(t, jar, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	effect, err := service.Runtime.Runtime().RegisterEffect(t.Context(), contentruntime.ExternalEffect{TenantID: actor.TenantID, JobRunID: detail.Summary.ID, NodeRunID: detail.Nodes[0].ID, Kind: "media.generate", IdempotencyKey: "unknown-effect", RequestDigest: "sha256:request", Currency: "CNY", SafeSummary: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	effect, err = service.Runtime.Runtime().ReconcileEffect(t.Context(), actor.TenantID, effect.ID, contentruntime.EffectUnknown, "", "", "TIMEOUT", effect.Version)
	if err != nil {
		t.Fatal(err)
	}
	reconciled := callBFF[application.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/runtime/effects/"+effect.ID+"/reconcile", map[string]int{"expected_version": effect.Version})
	if len(reconciled.Effects) != 1 || reconciled.Effects[0].State != contentruntime.EffectReconciling {
		t.Fatalf("unknown effect was not moved to reconciling: %#v", reconciled.Effects)
	}
}

func TestRuntimeDynamicGraphBFFKeepsRuntimeAsAuthority(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), nil, application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	task := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "动态图运营", Goal: "验证动态图和 Fanout 仍由 Runtime 统一编排"})
	list := callBFF[application.RuntimeJobList](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	job := runtimeJobForTask(t, list, task.Task.ID)
	initial := callBFF[application.RuntimeJobDetail](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+job.ID, nil)
	if initial.Plan.GraphVersion < 1 || len(initial.Plan.Nodes) == 0 || len(initial.Plan.Edges) == 0 || initial.FanoutSets == nil {
		t.Fatalf("dynamic runtime projection is incomplete: %#v", initial.Plan)
	}
	mapKey := initial.Plan.Nodes[0].Key
	joinKey := initial.Plan.Nodes[len(initial.Plan.Nodes)-1].Key
	patched := callBFF[application.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/runtime/jobs/"+job.ID+"/graph-patches", map[string]any{
		"expected_graph_version": initial.Plan.GraphVersion,
		"idempotency_key":        "http-graph-patch-1",
		"reason":                 "为已确认集合追加一条候选处理步骤",
		"add_nodes":              []map[string]any{{"key": "dynamic:candidate:1", "kind": "stage", "name": "动态候选", "depends_on": []string{mapKey}, "output_schema": "contentcloud.dynamic_candidate/1.0", "retry_max_attempts": 1}},
	})
	if patched.Plan.GraphVersion != initial.Plan.GraphVersion+1 || len(patched.Plan.Nodes) != len(initial.Plan.Nodes)+1 {
		t.Fatalf("graph patch was not projected from Runtime: before=%d after=%d nodes=%d", initial.Plan.GraphVersion, patched.Plan.GraphVersion, len(patched.Plan.Nodes))
	}
	fanout := callBFF[application.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/runtime/jobs/"+job.ID+"/fanout-sets", map[string]any{
		"map_node_key": mapKey, "join_node_key": joinKey, "generation": 1, "idempotency_key": "http-fanout-1", "reason": "按冻结项目集合展开",
		"join_policy":   map[string]any{"strategy": "best_effort", "zero_member_policy": "fail"},
		"node_template": map[string]any{"kind": "fanout_item", "name": "集合项目", "output_schema": "contentcloud.fanout_item/1.0", "retry_max_attempts": 1},
		"items":         []map[string]any{{"item_key": "item:a", "item_digest": "sha256:item-a"}, {"item_key": "item:b", "item_digest": "sha256:item-b"}},
	})
	if len(fanout.FanoutSets) != 1 || fanout.FanoutSets[0].MemberCount != 2 || len(fanout.FanoutSets[0].Members) != 2 {
		t.Fatalf("fanout set was not projected with frozen membership: %#v", fanout.FanoutSets)
	}
	joined := callBFF[application.RuntimeJobDetail](t, client, http.MethodPost, server.URL+"/api/bff/runtime/fanout-sets/"+fanout.FanoutSets[0].ID+"/join", nil)
	if len(joined.FanoutSets) != 1 || joined.FanoutSets[0].Status == "open" {
		t.Fatalf("fanout join did not return Runtime status: %#v", joined.FanoutSets)
	}
}

func TestRuntimeDynamicGraphBFFRejectsCrossTenantMutation(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), nil, application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	ownerJar, _ := cookiejar.New(nil)
	ownerClient := &http.Client{Jar: ownerJar}
	bootstrap := mustStudioBootstrap(t, ownerClient, server.URL)
	task := callBFF[application.StudioTaskView](t, ownerClient, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "租户隔离动态图", Goal: "确认其他租户不能改写执行图"})
	list := callBFF[application.RuntimeJobList](t, ownerClient, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	job := runtimeJobForTask(t, list, task.Task.ID)
	foreignSession, err := service.Identity.Register(t.Context(), "runtime-foreign@example.com", "long-enough-password", "Foreign Operator", "Foreign Tenant")
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	foreignJar, _ := cookiejar.New(nil)
	foreignJar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: foreignSession.ID, Path: "/"}})
	foreignClient := &http.Client{Jar: foreignJar}
	body := strings.NewReader(`{"expected_graph_version":1,"idempotency_key":"cross-tenant-patch","reason":"不应成功","add_nodes":[]}`)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/bff/runtime/jobs/"+job.ID+"/graph-patches", body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := foreignClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusNotFound || strings.Contains(string(responseBody), job.ID) {
		t.Fatalf("cross-tenant graph patch leaked or mutated a runtime job: status=%d body=%s", response.StatusCode, responseBody)
	}
}

func TestRuntimeEventsStreamResumesFromLastEventID(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), nil, application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, nil, true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap := mustStudioBootstrap(t, client, server.URL)
	task := callBFF[application.StudioTaskView](t, client, http.MethodPost, server.URL+"/api/studio/tasks", application.StudioCreateTaskInput{ExperienceID: bootstrap.Experiences[0].ID, ProjectID: bootstrap.Projects[0].ID, Title: "Runtime SSE", Goal: "验证事件增量"})
	list := callBFF[application.RuntimeJobList](t, client, http.MethodGet, server.URL+"/api/bff/runtime/jobs", nil)
	job := runtimeJobForTask(t, list, task.Task.ID)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/bff/runtime/jobs/"+job.ID+"/events/stream", nil)
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

func runtimeJobForTask(t *testing.T, list application.RuntimeJobList, workTaskID string) application.RuntimeJobSummary {
	t.Helper()
	for _, item := range list.Items {
		if item.WorkTaskID == workTaskID {
			return item
		}
	}
	t.Fatalf("runtime job was not created for work task %q: %#v", workTaskID, list)
	return application.RuntimeJobSummary{}
}
