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

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
)

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
