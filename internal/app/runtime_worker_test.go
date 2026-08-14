package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestRuntimeWorkerFenceOwnerAndTerminalProtocol(t *testing.T) {
	store := memory.New()
	service := New(store, nil)
	session, err := service.Register(t.Context(), "runtime-worker@example.com", "long-enough-password", "Worker", "Runtime Worker")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sop := remoteWorkerTestSOP(actor.TenantID, "worker-sop", nil)
	started := startRemoteWorkerTestJob(t, store, service, actor, project, sop, "worker-task-1", "sha256:"+strings.Repeat("b", 64), "runtime-policy/test", 0)
	worker := Actor{UserID: actor.UserID, TenantID: actor.TenantID, Type: "worker"}
	handle, err := service.PrepareRuntimeWorker(t.Context(), worker, RuntimeWorkerPrepareInput{JobRunID: started.Job.ID, HarnessKind: "fake", Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, MaxParallelSessions: 128}, Role: "worker", ExecutionProfileID: "profile-test", MaxTokens: 1024, BudgetMinor: 100})
	if err != nil {
		t.Fatal(err)
	}
	if handle.Attempt.LeaseOwner != "worker:"+actor.UserID || handle.Attempt.FenceToken == "" {
		t.Fatalf("worker lease is not bound to the authenticated owner: %#v", handle.Attempt)
	}
	if _, err := service.HeartbeatRuntimeWorker(t.Context(), worker, RuntimeWorkerHeartbeatInput{AttemptID: handle.Attempt.ID, FenceToken: "wrong"}); err == nil {
		t.Fatal("stale fence token must be rejected")
	}
	if _, err := service.ActivateRuntimeWorker(t.Context(), worker, RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: agentadapter.AgentSessionRef{TenantID: "other-tenant", HarnessKind: "fake", SessionID: "session-worker-mismatch"}}); !hasAppDomainCode(err, "RUNTIME_SESSION_TENANT_MISMATCH") {
		t.Fatalf("client-supplied session tenant was not rejected: %v", err)
	}
	active, err := service.ActivateRuntimeWorker(t.Context(), worker, RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: agentadapter.AgentSessionRef{TenantID: actor.TenantID, HarnessKind: "fake", SessionID: "session-worker-1"}})
	if err != nil {
		t.Fatal(err)
	}
	event := agentadapter.AgentEvent{Type: "item.completed", Session: agentadapter.AgentSessionRef{TenantID: actor.TenantID, HarnessKind: "fake", SessionID: "session-worker-1"}, Data: json.RawMessage(`{"item_type":"tool_call","status":"completed"}`), OccurredAt: time.Now().UTC()}
	if err := service.RecordRuntimeWorkerEvent(t.Context(), worker, RuntimeWorkerEventInput{AttemptID: active.Attempt.ID, FenceToken: active.Attempt.FenceToken, Event: event}); err != nil {
		t.Fatalf("current fenced Harness event was rejected: %v", err)
	}
	if err := service.RecordRuntimeWorkerEvent(t.Context(), worker, RuntimeWorkerEventInput{AttemptID: active.Attempt.ID, FenceToken: active.Attempt.FenceToken, Event: event}); err != nil {
		t.Fatalf("identical fenced Harness event replay was rejected: %v", err)
	}
	if err := service.RecordRuntimeWorkerEvent(t.Context(), worker, RuntimeWorkerEventInput{AttemptID: active.Attempt.ID, FenceToken: "stale", Event: event}); err == nil {
		t.Fatal("stale Harness event fence must be rejected")
	}
	events, err := service.Runtime().Events(t.Context(), actor.TenantID, started.Job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	attemptEventCount := 0
	for _, recorded := range events {
		if recorded.Type == "attempt.event" {
			attemptEventCount++
		}
	}
	if attemptEventCount != 1 {
		t.Fatalf("identical Harness event replay created duplicates: %d", attemptEventCount)
	}
	if last := events[len(events)-1]; last.Type != "attempt.event" || last.Payload["data_digest"] == "" || strings.Contains(string(last.Payload["data_digest"].(string)), "tool_call") {
		t.Fatalf("Harness event was not reduced to a durable digest: %#v", last)
	}
	if err := service.validateRuntimeBusinessResult(t.Context(), worker, started.Job.ID, json.RawMessage(`{}`)); err == nil {
		t.Fatal("unregistered business types must not accept an unowned structured result")
	}
	if _, err := service.FinalizeRuntimeWorker(t.Context(), worker, RuntimeWorkerFinalizeInput{AttemptID: active.Attempt.ID, FenceToken: active.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, OutputRefs: []string{"runtime-result:worker-controlled.json"}, SafeSummary: map[string]any{"status": "forged"}}, "worker-test-forged-ref"); err == nil {
		t.Fatal("worker must not forge the server-owned runtime-result namespace")
	}
	finalized, err := service.FinalizeRuntimeWorker(t.Context(), worker, RuntimeWorkerFinalizeInput{AttemptID: active.Attempt.ID, FenceToken: active.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, SafeSummary: map[string]any{"status": "ok"}}, "worker-test")
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Handle.Attempt.State != domain.RuntimeAttemptSucceeded || finalized.Job.State != domain.JobRunCompleted {
		t.Fatalf("worker protocol did not converge Runtime state: %#v %#v", finalized.Handle.Attempt, finalized.Job)
	}
	if _, err := service.FinalizeRuntimeWorker(t.Context(), worker, RuntimeWorkerFinalizeInput{AttemptID: active.Attempt.ID, FenceToken: active.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, SafeSummary: map[string]any{"status": "ok"}}, "worker-test-retry"); err != nil {
		t.Fatalf("identical terminal worker retry must be idempotent: %v", err)
	}
	if _, err := service.FinalizeRuntimeWorker(t.Context(), worker, RuntimeWorkerFinalizeInput{AttemptID: active.Attempt.ID, FenceToken: "wrong", State: domain.RuntimeAttemptSucceeded, SafeSummary: map[string]any{"status": "ok"}}, "worker-test-wrong-fence"); err == nil {
		t.Fatal("terminal worker retry must verify the original fence digest")
	}
	if _, err := service.FinalizeRuntimeWorker(t.Context(), worker, RuntimeWorkerFinalizeInput{AttemptID: active.Attempt.ID, FenceToken: active.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, SafeSummary: map[string]any{"status": "changed"}}, "worker-test"); err == nil {
		t.Fatal("terminal RuntimeAttempt must reject a different result")
	}
}

func TestPrepareNextRuntimeWorkerUsesTenantPriorityAndSkipsPausedJobs(t *testing.T) {
	store := memory.New()
	service := New(store, nil)
	session, err := service.Register(t.Context(), "runtime-fairness@example.com", "long-enough-password", "Worker", "Runtime Fairness")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sop := remoteWorkerTestSOP(actor.TenantID, "fair-sop", nil)
	high := startRemoteWorkerTestJob(t, store, service, actor, project, sop, "fair-high", "sha256:"+strings.Repeat("e", 64), "runtime-policy/test", 100)
	time.Sleep(time.Millisecond)
	low := startRemoteWorkerTestJob(t, store, service, actor, project, sop, "fair-low", "sha256:"+strings.Repeat("e", 64), "runtime-policy/test", 1)
	worker := Actor{UserID: actor.UserID, TenantID: actor.TenantID, Type: "worker"}
	prepare := RuntimeWorkerPrepareNextInput{RuntimeWorkerPrepareInput: RuntimeWorkerPrepareInput{HarnessKind: "fake", Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, MaxParallelSessions: 128}, Role: "worker", ExecutionProfileID: "profile-test", MaxTokens: 1024, BudgetMinor: 100}}
	handle, err := service.PrepareNextRuntimeWorker(t.Context(), worker, prepare)
	if err != nil {
		t.Fatal(err)
	}
	if handle.Attempt.JobRunID != high.Job.ID {
		t.Fatalf("tenant scheduler ignored Runtime priority: got %s want %s (newer low job %s)", handle.Attempt.JobRunID, high.Job.ID, low.Job.ID)
	}
	if _, err := service.Runtime().Pause(t.Context(), actor.TenantID, high.Job.ID, "user", actor.UserID); err != nil {
		t.Fatal(err)
	}
	// The first node is already leased, so create another higher-priority job
	// and pause it before prepare_next selects a new candidate.
	paused := startRemoteWorkerTestJob(t, store, service, actor, project, sop, "fair-paused", "sha256:"+strings.Repeat("e", 64), "runtime-policy/test", 200)
	if _, err := service.Runtime().Pause(t.Context(), actor.TenantID, paused.Job.ID, "user", actor.UserID); err != nil {
		t.Fatal(err)
	}
	handle, err = service.PrepareNextRuntimeWorker(t.Context(), worker, prepare)
	if err != nil {
		t.Fatal(err)
	}
	if handle.Attempt.JobRunID != low.Job.ID {
		t.Fatalf("tenant scheduler selected a paused or non-ready job: got %s want %s", handle.Attempt.JobRunID, low.Job.ID)
	}
}

func TestRuntimeDeviceProjectScopeRestrictsImplicitAndExplicitPrepare(t *testing.T) {
	store := memory.New()
	service := New(store, nil)
	session, err := service.Register(t.Context(), "runtime-project-scope@example.com", "long-enough-password", "Worker", "Runtime Project Scope")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	allowedProject, err := service.CreateProject(t.Context(), actor, CreateProjectInput{BrandName: "Allowed", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	deniedProject, err := service.CreateProject(t.Context(), actor, CreateProjectInput{BrandName: "Denied", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sop := remoteWorkerTestSOP(actor.TenantID, "scope-sop", nil)
	denied := startRemoteWorkerTestJob(t, store, service, actor, deniedProject, sop, "scope-denied", "sha256:"+strings.Repeat("2", 64), "runtime-policy/test", 100)
	allowed := startRemoteWorkerTestJob(t, store, service, actor, allowedProject, sop, "scope-allowed", "sha256:"+strings.Repeat("2", 64), "runtime-policy/test", 1)
	device := Actor{UserID: actor.UserID, TenantID: actor.TenantID, Type: "device", DeviceID: "device-scope", ProjectIDs: []string{allowedProject.ID}}
	prepare := RuntimeWorkerPrepareInput{HarnessKind: "fake", Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, MaxParallelSessions: 1}, Role: "worker", ExecutionProfileID: "profile-test", MaxTokens: 1024, BudgetMinor: 100}
	handle, err := service.PrepareNextRuntimeWorker(t.Context(), device, RuntimeWorkerPrepareNextInput{RuntimeWorkerPrepareInput: prepare})
	if err != nil {
		t.Fatal(err)
	}
	if handle.Attempt.JobRunID != allowed.Job.ID {
		t.Fatalf("device crossed its project grant: got %s want %s; denied job %s has higher priority", handle.Attempt.JobRunID, allowed.Job.ID, denied.Job.ID)
	}
	prepare.JobRunID = denied.Job.ID
	if _, err := service.PrepareRuntimeWorker(t.Context(), device, prepare); !hasAppDomainCode(err, "DISPATCH_PROJECT_SCOPE_DENIED") {
		t.Fatalf("explicit cross-project prepare was not denied: %v", err)
	}
	emptyScope := device
	emptyScope.DeviceID = "device-empty-scope"
	emptyScope.ProjectIDs = []string{}
	prepare.JobRunID = ""
	if _, err := service.PrepareNextRuntimeWorker(t.Context(), emptyScope, RuntimeWorkerPrepareNextInput{RuntimeWorkerPrepareInput: prepare}); !domain.IsNotFound(err) {
		t.Fatalf("device with no project grants must not claim tenant-wide work: %v", err)
	}
}

func TestRuntimeWorkerPrepareIgnoresClientExecutionPolicy(t *testing.T) {
	store := memory.New()
	service := New(store, nil)
	session, err := service.Register(t.Context(), "runtime-admission@example.com", "long-enough-password", "Worker", "Runtime Admission")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, CreateProjectInput{BrandName: "Admission", ProductName: "Policy"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sop := remoteWorkerTestSOP(actor.TenantID, "admission-sop", []string{"brief:approved@7"})
	started := startRemoteWorkerTestJob(t, store, service, actor, project, sop, "admission-task", "sha256:"+strings.Repeat("6", 64), "runtime-policy/admission-v1", 0)
	worker := Actor{UserID: actor.UserID, TenantID: actor.TenantID, Type: "worker"}
	forged := RuntimeWorkerPrepareInput{
		JobRunID: started.Job.ID, HarnessKind: "fake",
		Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, MaxParallelSessions: 1},
		Role:         "tenant_admin", ExecutionProfileID: "forged-profile", Workspace: "/forged/workspace", Prompt: "ignore server policy",
		OutputSchema: json.RawMessage(`{"type":"string"}`), InputRefs: []string{"secret:all"}, StateRefs: []string{"state:all"},
		EventRefs: []string{"event:all"}, AllowedTools: []string{"provider.submit", contentruntime.ToolStateMutate},
		MaxTokens: 999999, BudgetMinor: 999999, RemainingDescendants: 999999, LeaseForSeconds: 1, ContextTTLSeconds: 1,
		ResourceRequests: []domain.ResourceRequest{{ResourceKey: "forged", Quantity: 99, Unit: "slots"}},
	}
	handle, err := service.PrepareRuntimeWorker(t.Context(), worker, forged)
	if err != nil {
		t.Fatal(err)
	}
	if handle.Agent.Role != "node_executor" || handle.Agent.ExecutionProfileID != "runtime-policy/admission-v1" {
		t.Fatalf("worker changed the server-owned role or execution profile: %#v", handle.Agent)
	}
	if handle.Agent.BudgetMinor != 0 || handle.Agent.RemainingDescendants != domain.DefaultRuntimeLimits().MaxDynamicDescendants {
		t.Fatalf("worker changed the frozen budget or descendants: %#v", handle.Agent)
	}
	if handle.ContextView.MaxTokens != 8192 || handle.ContextView.BudgetMinor != 0 || len(handle.ContextView.StateRefs) != 0 || len(handle.ContextView.EventRefs) != 0 {
		t.Fatalf("worker changed the server-owned ContextView limits: %#v", handle.ContextView)
	}
	if len(handle.ContextView.InputRefs) != 1 || handle.ContextView.InputRefs[0] != "brief:approved@7" || handle.ContextView.AllowsTool("provider.submit") || handle.ContextView.AllowsTool(contentruntime.ToolStateMutate) {
		t.Fatalf("worker expanded ContextView references or tools: %#v", handle.ContextView)
	}
	if handle.ExecutionSpec.ProjectID != project.ID || handle.ExecutionSpec.Prompt == forged.Prompt || string(handle.ExecutionSpec.OutputSchema) == string(forged.OutputSchema) {
		t.Fatalf("worker changed server-owned execution instructions: %#v", handle.ExecutionSpec)
	}
	if handle.Attempt.LeaseExpiresAt == nil || handle.Attempt.LeaseExpiresAt.Sub(handle.Attempt.CreatedAt) < contentruntime.DefaultNodeLeaseDuration-time.Second {
		t.Fatalf("worker shortened the server-owned lease: %#v", handle.Attempt)
	}
	reservations, err := store.ResourceReservations(t.Context(), actor.TenantID, handle.Attempt.ID)
	if err != nil || len(reservations) != 0 {
		t.Fatalf("worker injected resource reservations: %#v err=%v", reservations, err)
	}
}

func TestRuntimeWorkerWorkspaceAdmissionFailsClosedBeforeAttemptCreation(t *testing.T) {
	tests := []struct {
		name     string
		wantCode string
		mutate   func([]domain.DaemonWorkspaceObservation) []domain.DaemonWorkspaceObservation
	}{
		{name: "environment drift", wantCode: "RUNTIME_ENVIRONMENT_DRIFT", mutate: func(values []domain.DaemonWorkspaceObservation) []domain.DaemonWorkspaceObservation {
			values[0].EnvironmentDeclaration = "sha256:" + strings.Repeat("9", 64)
			return values
		}},
		{name: "plugin drift", wantCode: "RUNTIME_ENVIRONMENT_DRIFT", mutate: func(values []domain.DaemonWorkspaceObservation) []domain.DaemonWorkspaceObservation {
			values[0].PluginDeclaration = "sha256:" + strings.Repeat("9", 64)
			return values
		}},
		{name: "skill drift", wantCode: "RUNTIME_ENVIRONMENT_DRIFT", mutate: func(values []domain.DaemonWorkspaceObservation) []domain.DaemonWorkspaceObservation {
			values[0].SkillDeclaration = "sha256:" + strings.Repeat("9", 64)
			return values
		}},
		{name: "mcp drift", wantCode: "RUNTIME_ENVIRONMENT_DRIFT", mutate: func(values []domain.DaemonWorkspaceObservation) []domain.DaemonWorkspaceObservation {
			values[0].MCPDeclaration = "sha256:" + strings.Repeat("9", 64)
			return values
		}},
		{name: "workspace drift", wantCode: "RUNTIME_ENVIRONMENT_DRIFT", mutate: func(values []domain.DaemonWorkspaceObservation) []domain.DaemonWorkspaceObservation {
			values[0].WorkspaceDeclaration = "sha256:" + strings.Repeat("9", 64)
			return values
		}},
		{name: "workspace not ready", wantCode: "RUNTIME_ENVIRONMENT_NOT_READY", mutate: func(values []domain.DaemonWorkspaceObservation) []domain.DaemonWorkspaceObservation {
			values[0].Status, values[0].Reason = "repair_required", "skill_drift"
			return values
		}},
		{name: "ambiguous project workspaces", wantCode: "RUNTIME_WORKSPACE_AMBIGUOUS", mutate: func(values []domain.DaemonWorkspaceObservation) []domain.DaemonWorkspaceObservation {
			second := values[0]
			second.WorkspaceID = "workspace-2"
			return append(values, second)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, service, actor, project, deviceActor, instanceID, binding := runtimeWorkspaceAdmissionFixture(t)
			started := startRemoteWorkerBoundJob(t, store, service, actor, project, remoteWorkerTestSOP(actor.TenantID, "workspace-admission", nil), binding, "workspace-admission-"+strings.ReplaceAll(test.name, " ", "-"))
			observations := []domain.DaemonWorkspaceObservation{{
				WorkspaceID: "workspace-1", ProjectID: project.ID, Status: "ready", Reason: "local_components_observed", Generation: "sha256:" + strings.Repeat("7", 64),
				EnvironmentDeclaration: binding.EnvironmentDigest, PluginDeclaration: binding.PluginDigest, SkillDeclaration: binding.SkillDigest,
				MCPDeclaration: binding.MCPDigest, WorkspaceDeclaration: binding.WorkspaceDigest, ObservedAt: time.Now().UTC(),
			}}
			observations = test.mutate(observations)
			instance, err := store.DaemonInstance(t.Context(), actor.TenantID, instanceID)
			if err != nil {
				t.Fatal(err)
			}
			instance.Capabilities = map[string]any{"workspace_observations": observations}
			instance.ReportSequence++
			instance.LastSeenAt = time.Now().UTC()
			if err := store.SaveDaemonInstance(t.Context(), instance); err != nil {
				t.Fatal(err)
			}
			_, err = service.PrepareRuntimeWorker(t.Context(), deviceActor, RuntimeWorkerPrepareInput{
				JobRunID: started.Job.ID, DaemonInstanceID: instanceID, HarnessKind: "fake",
				Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, SandboxProfile: "fake", MaxParallelSessions: 1},
			})
			if !hasAppDomainCode(err, test.wantCode) {
				t.Fatalf("workspace admission error = %v, want %s", err, test.wantCode)
			}
			attempts, attemptsErr := service.Runtime().Attempts(t.Context(), actor.TenantID, started.Job.ID)
			if attemptsErr != nil || len(attempts) != 0 {
				t.Fatalf("failed workspace admission created RuntimeAttempt rows: %#v err=%v", attempts, attemptsErr)
			}
		})
	}
}

func runtimeWorkspaceAdmissionFixture(t *testing.T) (*memory.Store, *Service, Actor, domain.Project, Actor, string, domain.ExecutionBindingSnapshot) {
	t.Helper()
	store := memory.New()
	service := New(store, nil)
	session, err := service.Register(t.Context(), "workspace-admission@example.com", "long-enough-password", "Worker", "Workspace Admission")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, CreateProjectInput{BrandName: "Admission", ProductName: "Workspace"}, "")
	if err != nil {
		t.Fatal(err)
	}
	device := domain.Device{ID: domain.NewID(), TenantID: actor.TenantID, OwnerUserID: actor.UserID, DisplayName: "Admission Device", ProjectIDs: []string{project.ID}, LastSeenAt: time.Now().UTC()}
	if err := store.SaveDevice(t.Context(), device); err != nil {
		t.Fatal(err)
	}
	instanceID := domain.NewID()
	if err := store.SaveDaemonInstance(t.Context(), domain.DaemonInstance{
		ID: instanceID, TenantID: actor.TenantID, DeviceID: device.ID, ConnectionEpoch: 1, ReportSequence: 1,
		Version: "test", State: "connected", Capabilities: map[string]any{}, ActiveAttempts: []string{},
		StartedAt: time.Now().UTC().Add(-time.Minute), LastSeenAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	binding := domain.ExecutionBindingSnapshot{
		TenantID: actor.TenantID, SchemaVersion: domain.ExecutionBindingSnapshotSchema,
		ProfileID: "contentcloud.video-production", ProfileVersion: "1", RuntimePolicyID: "runtime-policy/workspace-admission",
		HarnessKinds: []string{"fake"}, AllowedTools: []string{}, SandboxProfile: "any", IsolationProfile: "workspace", EgressPolicy: "declared",
		DataClassification: "internal", MaxTokens: 8192, MaxDurationSeconds: 3600, MaxDynamicDescendants: domain.DefaultRuntimeLimits().MaxDynamicDescendants,
		FallbackPolicy: "none", WorkspaceTemplateID: "workspace-marketing-video", CreatedAt: time.Now().UTC(),
		EnvironmentDigest: "sha256:" + strings.Repeat("1", 64), PluginDigest: "sha256:" + strings.Repeat("2", 64),
		SkillDigest: "sha256:" + strings.Repeat("3", 64), MCPDigest: "sha256:" + strings.Repeat("4", 64), WorkspaceDigest: "sha256:" + strings.Repeat("5", 64),
	}
	return store, service, actor, project, Actor{UserID: actor.UserID, TenantID: actor.TenantID, Type: "device", DeviceID: device.ID, ProjectIDs: []string{project.ID}}, instanceID, binding
}

func startRemoteWorkerBoundJob(t *testing.T, store *memory.Store, service *Service, actor Actor, project domain.Project, sop domain.SOPVersion, binding domain.ExecutionBindingSnapshot, key string) contentruntime.StartResult {
	t.Helper()
	manifestHash := domain.TokenHash("runtime-worker-bound-test:" + key)
	snapshot := domain.ContextSnapshot{
		ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, BuilderVersion: "runtime-worker-test/1.0",
		SchemaVersion: domain.TaskContractSchema, Sources: []domain.ContractSource{}, InputVersions: map[string]string{}, ManifestHash: manifestHash, CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateSnapshot(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	started, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{
		TenantID: actor.TenantID, ProjectID: project.ID, WorkTaskID: key, BusinessType: "worker.test", InputSnapshotID: snapshot.ID,
		SOP: sop, ExecutionBinding: &binding, InputDigest: "sha256:" + manifestHash, RuntimePolicyID: binding.RuntimePolicyID,
		ContractMajor: 1, CreatedBy: actor.UserID, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	return started
}

func remoteWorkerTestSOP(tenantID, id string, inputRefs []string) domain.SOPVersion {
	return domain.SOPVersion{
		ID: id + "-v1", TenantID: tenantID, SOPID: id, Version: 1, SchemaVersion: domain.SOPSchemaVersion,
		Name: id, Status: "published", DefaultExecutionMode: "agent", Digest: "sha256:" + strings.Repeat("5", 64),
		Stages: []domain.StageDefinition{{
			ID: "execute", Name: "Execute", Order: 10, InputRefs: append([]string(nil), inputRefs...),
			OutputSchema: domain.KnowledgeCandidatesSchema, RequiredCapabilities: []string{domain.KnowledgeExtractCapability},
			ExecutionModes: []string{"agent"},
		}},
	}
}

func startRemoteWorkerTestJob(t *testing.T, store *memory.Store, service *Service, actor Actor, project domain.Project, sop domain.SOPVersion, key, bindingDigest, policyID string, priority int) contentruntime.StartResult {
	t.Helper()
	manifestHash := domain.TokenHash("runtime-worker-test:" + key)
	snapshot := domain.ContextSnapshot{
		ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, BuilderVersion: "runtime-worker-test/1.0",
		SchemaVersion: domain.TaskContractSchema, Sources: []domain.ContractSource{}, InputVersions: map[string]string{},
		ManifestHash: manifestHash, CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateSnapshot(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	started, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{
		TenantID: actor.TenantID, ProjectID: project.ID, WorkTaskID: key, BusinessType: "worker.test", InputSnapshotID: snapshot.ID,
		SOP: sop, BindingDigest: bindingDigest, InputDigest: "sha256:" + manifestHash, RuntimePolicyID: policyID,
		ContractMajor: 1, Priority: priority, CreatedBy: actor.UserID, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	return started
}

func hasAppDomainCode(err error, code string) bool {
	value, ok := err.(*domain.Error)
	return ok && value.Code == code
}

func TestRuntimeBusinessResultRefAndBackoffAreBounded(t *testing.T) {
	digest := strings.Repeat("a", 64)
	ref := "runtime-result:runtime/results/tenant-1/attempt-1/" + digest + ".json"
	key, parsedDigest, err := parseRuntimeBusinessResultRef(ref, "tenant-1", "attempt-1")
	if err != nil || key == "" || parsedDigest != digest {
		t.Fatalf("valid scoped Runtime result ref was rejected: key=%q digest=%q err=%v", key, parsedDigest, err)
	}
	if _, _, err := parseRuntimeBusinessResultRef(ref, "tenant-2", "attempt-1"); err == nil {
		t.Fatal("Runtime result ref must not cross tenant scope")
	}
	if _, _, err := parseRuntimeBusinessResultRef("runtime-result:runtime/results/tenant-1/attempt-1/not-a-digest.json", "tenant-1", "attempt-1"); err == nil {
		t.Fatal("Runtime result ref must contain a SHA-256 digest")
	}
	if got := runtimeBusinessResultBackoff(100); got != 5*time.Minute {
		t.Fatalf("business result retry backoff must be capped: %s", got)
	}
}
