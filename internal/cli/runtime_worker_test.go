package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localconfig"
	"github.com/limecloud/contentcloud/internal/localworkspace"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

func TestRuntimeWorkerUsesHarnessSessionAndRenewsLeaseUntilResult(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	registry := agentadapter.NewHarnessRegistry()
	if err := registry.Register("fake", fake); err != nil {
		t.Fatal(err)
	}

	leaseExpiry := time.Now().UTC().Add(300 * time.Millisecond)
	handle := contentruntime.DispatchHandle{
		Node:          domain.NodeRun{ID: "node-1"},
		Attempt:       domain.RuntimeAttempt{ID: "attempt-1", TenantID: "tenant-1", JobRunID: "job-1", HarnessKind: "fake", State: domain.RuntimeAttemptPrepared, FenceToken: "fence-1", LeaseExpiresAt: &leaseExpiry},
		ContextView:   domain.ContextView{Digest: "sha256:context"},
		Capabilities:  agentadapter.HarnessCapabilities{Kind: "fake", Events: true, Resume: true, StructuredOutput: true, MaxParallelSessions: 128},
		ExecutionSpec: testRuntimeExecutionSpec("project-1", "job-1"),
	}
	heartbeats := 0
	recordedEvents := 0
	var activated app.RuntimeWorkerActivateInput
	var finalized app.RuntimeWorkerFinalizeInput
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var envelope struct {
			Command string          `json:"command"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
			t.Errorf("decode dispatch: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		switch envelope.Command {
		case "runtime.worker.prepare_next":
			var input app.RuntimeWorkerPrepareNextInput
			_ = json.Unmarshal(envelope.Params, &input)
			if input.HarnessKind != "fake" || input.Capabilities.Kind != "fake" || !input.Capabilities.Events || !input.Capabilities.StructuredOutput {
				t.Errorf("worker did not send the detected capability snapshot: %#v", input.Capabilities)
			}
			writeRuntimeWorkerResponse(t, writer, envelope.Command, handle)
		case "runtime.worker.activate":
			_ = json.Unmarshal(envelope.Params, &activated)
			handle.Attempt.State = domain.RuntimeAttemptRunning
			next := time.Now().UTC().Add(300 * time.Millisecond)
			handle.Attempt.LeaseExpiresAt = &next
			writeRuntimeWorkerResponse(t, writer, envelope.Command, handle)
		case "runtime.worker.heartbeat":
			heartbeats++
			next := time.Now().UTC().Add(300 * time.Millisecond)
			handle.Attempt.LeaseExpiresAt = &next
			writeRuntimeWorkerResponse(t, writer, envelope.Command, handle)
			if heartbeats == 2 {
				if err := fake.Complete(activated.Session, map[string]any{"output_refs": []string{"artifact:1"}, "output_digest": "sha256:result", "safe_summary": map[string]any{"done": true}, "used_cost_minor": 4}); err != nil {
					t.Errorf("complete fake harness after second heartbeat: %v", err)
				}
			}
		case "runtime.worker.event":
			recordedEvents++
			var input app.RuntimeWorkerEventInput
			_ = json.Unmarshal(envelope.Params, &input)
			if input.Event.Session.SessionID == "" || input.Event.Type == "result.completed" {
				t.Errorf("worker sent an invalid progress event: %#v", input.Event)
			}
			writeRuntimeWorkerResponse(t, writer, envelope.Command, map[string]any{"recorded": true})
		case "runtime.worker.finalize":
			_ = json.Unmarshal(envelope.Params, &finalized)
			handle.Attempt.State = finalized.State
			writeRuntimeWorkerResponse(t, writer, envelope.Command, app.RuntimeWorkerResult{Handle: handle, Job: domain.JobRun{ID: "job-1"}})
		default:
			t.Errorf("unexpected dispatch command: %s", envelope.Command)
			writer.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	root := &Root{runtimeHarnesses: registry}
	result, err := root.runRuntimeWorker(t.Context(), apiclient.New(server.URL, "device-token"), runtimeWorkerRunOptions{HarnessKind: "fake", Workspace: t.TempDir()}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result["state"] != domain.RuntimeAttemptSucceeded || heartbeats < 2 || recordedEvents == 0 {
		t.Fatalf("worker did not keep the lease and event stream alive until completion: result=%#v heartbeats=%d events=%d", result, heartbeats, recordedEvents)
	}
	if activated.Session.SessionID == "" || strings.HasPrefix(activated.Session.SessionID, "runtime-worker:") || strings.HasPrefix(activated.Session.SessionID, "fixture:") {
		t.Fatalf("worker activated a synthetic session instead of the Harness session: %#v", activated.Session)
	}
	if finalized.State != domain.RuntimeAttemptSucceeded || len(finalized.OutputRefs) != 1 || finalized.OutputRefs[0] != "artifact:1" || finalized.OutputDigest != "sha256:result" || finalized.UsedCostMinor != 4 || len(finalized.BusinessPayload) != 0 {
		t.Fatalf("worker did not translate the Harness result envelope: %#v", finalized)
	}
}

func TestRuntimeWorkerUsesAttemptWorkspaceAndCleansItAfterFinalization(t *testing.T) {
	harness := &workspaceCaptureHarness{}
	registry := agentadapter.NewHarnessRegistry()
	if err := registry.Register("workspace-capture", harness); err != nil {
		t.Fatal(err)
	}
	interactiveRoot := t.TempDir()
	leaseExpiry := time.Now().UTC().Add(time.Second)
	handle := contentruntime.DispatchHandle{
		Node:        domain.NodeRun{ID: "node-isolated"},
		Attempt:     domain.RuntimeAttempt{ID: domain.NewID(), TenantID: "tenant-1", JobRunID: "job-isolated", HarnessKind: "workspace-capture", State: domain.RuntimeAttemptPrepared, FenceToken: "fence-isolated", LeaseExpiresAt: &leaseExpiry},
		ContextView: domain.ContextView{Digest: "sha256:context"}, Capabilities: agentadapter.HarnessCapabilities{Kind: "workspace-capture", Events: true, StructuredOutput: true, MaxParallelSessions: 1},
		ExecutionSpec: testRuntimeExecutionSpec("project-isolated", "job-isolated"),
	}
	server := runtimeWorkerHandleServer(t, &handle, nil)
	defer server.Close()

	root := &Root{runtimeHarnesses: registry, stderr: &strings.Builder{}}
	result, err := root.runRuntimeWorker(t.Context(), apiclient.New(server.URL, "device-token"), runtimeWorkerRunOptions{HarnessKind: "workspace-capture", Workspace: interactiveRoot}, false)
	if err != nil || result["state"] != domain.RuntimeAttemptSucceeded {
		t.Fatalf("isolated worker result=%#v err=%v", result, err)
	}
	captured := harness.workspace()
	if captured == "" || filepath.Clean(captured) == filepath.Clean(interactiveRoot) {
		t.Fatalf("Harness received the interactive workspace instead of an Attempt workspace: %q", captured)
	}
	if relative, relErr := filepath.Rel(interactiveRoot, captured); relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("Attempt workspace overlaps the interactive workspace: %q", captured)
	}
	if !harness.sawFrozenResources() {
		t.Fatal("Harness did not receive the frozen contract, schema, skill, and lease resources")
	}
	if _, statErr := os.Stat(captured); !os.IsNotExist(statErr) {
		t.Fatalf("Attempt workspace remains after terminal finalization: %v", statErr)
	}
}

func TestRuntimeWorkerFinalizesRetryableWhenLocalLeaseDrifts(t *testing.T) {
	harness := &leaseDriftHarness{}
	registry := agentadapter.NewHarnessRegistry()
	if err := registry.Register("lease-drift", harness); err != nil {
		t.Fatal(err)
	}
	leaseExpiry := time.Now().UTC().Add(180 * time.Millisecond)
	handle := contentruntime.DispatchHandle{
		Node:        domain.NodeRun{ID: "node-lease-drift"},
		Attempt:     domain.RuntimeAttempt{ID: domain.NewID(), TenantID: "tenant-1", JobRunID: "job-lease-drift", HarnessKind: "lease-drift", State: domain.RuntimeAttemptPrepared, FenceToken: "fence-lease-drift", LeaseExpiresAt: &leaseExpiry},
		ContextView: domain.ContextView{Digest: "sha256:context"}, Capabilities: agentadapter.HarnessCapabilities{Kind: "lease-drift", Events: true, StructuredOutput: true, MaxParallelSessions: 1},
		ExecutionSpec: testRuntimeExecutionSpec("project-lease-drift", "job-lease-drift"),
	}
	var finalized app.RuntimeWorkerFinalizeInput
	server := runtimeWorkerHandleServer(t, &handle, &finalized)
	defer server.Close()

	root := &Root{runtimeHarnesses: registry, stderr: &strings.Builder{}}
	_, err := root.runRuntimeWorker(t.Context(), apiclient.New(server.URL, "device-token"), runtimeWorkerRunOptions{HarnessKind: "lease-drift"}, false)
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "AUTOMATION_WORKSPACE_LEASE_CHANGED" {
		t.Fatalf("local lease drift error = %v", err)
	}
	if finalized.State != domain.RuntimeAttemptRetryableFailed || finalized.ErrorCode != "AUTOMATION_WORKSPACE_LEASE_CHANGED" {
		t.Fatalf("local lease drift finalize = %#v", finalized)
	}
	if captured := harness.workspace(); captured == "" {
		t.Fatal("lease drift Harness did not receive an Attempt workspace")
	} else if _, statErr := os.Stat(captured); !os.IsNotExist(statErr) {
		t.Fatalf("lease-drift Attempt workspace remains after finalize: %v", statErr)
	}
}

func TestResolveRuntimeGatewayURLKeepsAttemptCredentialOnServerOrigin(t *testing.T) {
	if got, err := resolveRuntimeGatewayURL("https://content.example/base", "/api/v1/runtime/mcp/call"); err != nil || got != "https://content.example/base/api/v1/runtime/mcp/call" {
		t.Fatalf("relative Gateway URL = %q err=%v", got, err)
	}
	if _, err := resolveRuntimeGatewayURL("https://content.example/base", "https://evil.example/runtime"); err == nil {
		t.Fatal("cross-origin Runtime Gateway URL was accepted")
	}
	if got, err := resolveRuntimeGatewayURL("https://content.example", "https://content.example/api/v1/runtime/mcp/call"); err != nil || got == "" {
		t.Fatalf("same-origin absolute Gateway URL rejected: %q err=%v", got, err)
	}
	if _, err := resolveRuntimeGatewayURL("https://content.example", "/api/v1/runtime/mcp/call?token=leak"); err == nil {
		t.Fatal("Runtime Gateway URL with query parameters was accepted")
	}
}

func TestRuntimeDaemonCapabilitiesPreserveDetectedHarnessContract(t *testing.T) {
	capabilities := runtimeDaemonCapabilities(agentadapter.HarnessCapabilities{
		Kind: "codex", Events: true, Resume: true, MCPStdio: true,
		StructuredOutput: true, SandboxProfile: "workspace_write_auto_approval",
		MaxParallelSessions: 8,
	})
	for key, wanted := range map[string]any{
		"harness_kind": "codex", "events": true, "resume": true, "mcp_stdio": true,
		"structured_output": true, "sandbox_profile": "workspace_write_auto_approval",
		"max_parallel_sessions": 8,
	} {
		if got := capabilities[key]; got != wanted {
			t.Fatalf("daemon capability %s = %#v, want %#v", key, got, wanted)
		}
	}
}

func TestRuntimeDaemonCapabilitiesReportHealthyAndUnhealthyInventory(t *testing.T) {
	registry := agentadapter.NewHarnessRegistry()
	if err := registry.Register("codex", &probeHarness{kind: "codex", version: "codex 1.2.3"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("claude", &probeHarness{kind: "claude", err: domain.Policy("CLAUDE_AUTH_REQUIRED", "not logged in", "login")}); err != nil {
		t.Fatal(err)
	}
	root := &Root{runtimeHarnesses: registry}
	capabilities := root.runtimeDaemonCapabilities(t.Context(), agentadapter.HarnessCapabilities{Kind: "codex"}, false, true)
	if capabilities["runtime_status"] != "healthy" || capabilities["harness_version"] != "codex 1.2.3" {
		t.Fatalf("selected runtime health = %#v", capabilities)
	}
	probes, ok := capabilities["runtimes"].([]agentadapter.HarnessProbe)
	byKind := map[string]agentadapter.HarnessProbe{}
	for _, probe := range probes {
		byKind[probe.Kind] = probe
	}
	if !ok || len(probes) != 2 || byKind["codex"].Status != "healthy" || byKind["codex"].Version != "codex 1.2.3" || byKind["claude"].Status != "unhealthy" || byKind["claude"].ErrorCode != "CLAUDE_AUTH_REQUIRED" {
		t.Fatalf("runtime inventory = %#v", capabilities["runtimes"])
	}
}

type probeHarness struct {
	kind    string
	version string
	err     error
}

func (h *probeHarness) Detect(context.Context) (agentadapter.HarnessCapabilities, error) {
	if h.err != nil {
		return agentadapter.HarnessCapabilities{}, h.err
	}
	return agentadapter.HarnessCapabilities{Kind: h.kind, Version: h.version, Events: true, StructuredOutput: true}, nil
}
func (h *probeHarness) Start(context.Context, agentadapter.StartAgentRequest) (agentadapter.AgentSessionRef, agentadapter.EventStream, error) {
	return agentadapter.AgentSessionRef{}, nil, errors.New("not implemented")
}
func (h *probeHarness) Resume(context.Context, agentadapter.ResumeAgentRequest) (agentadapter.EventStream, error) {
	return nil, errors.New("not implemented")
}
func (h *probeHarness) Interrupt(context.Context, agentadapter.AgentSessionRef) error { return nil }
func (h *probeHarness) Inspect(context.Context, agentadapter.AgentSessionRef) (agentadapter.AgentSessionStatus, error) {
	return agentadapter.AgentSessionStatus{}, errors.New("not implemented")
}

func TestRuntimeWorkerFinalizeInputSeparatesBusinessPayloadFromExecutionEnvelope(t *testing.T) {
	handle := contentruntime.DispatchHandle{Attempt: domain.RuntimeAttempt{ID: "attempt-1", FenceToken: "fence-1"}}
	business := json.RawMessage(`{"schema_version":"1.0","candidates":[],"warnings":[]}`)
	input, err := runtimeWorkerFinalizeInput(handle, "codex", business)
	if err != nil || string(input.BusinessPayload) != string(business) || len(input.OutputRefs) != 0 {
		t.Fatalf("business result was not preserved: input=%#v err=%v", input, err)
	}
}

func TestRuntimeWorkerOnceTreatsResourceNotFoundAsEmptyQueue(t *testing.T) {
	prepareCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		prepareCalls++
		if request.URL.Path != "/api/v1/cli/dispatch" {
			t.Errorf("unexpected dispatch path: %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"ok":         false,
			"command":    "runtime.worker.prepare_next",
			"request_id": "request-empty-queue",
			"error":      &domain.Error{Type: "not_found", Subtype: "resource", Code: "RESOURCE_NOT_FOUND", Message: "当前没有可领取任务"},
		})
	}))
	defer server.Close()

	root := &Root{}
	result, err := root.runRuntimeWorker(t.Context(), apiclient.New(server.URL, "device-token"), runtimeWorkerRunOptions{Fixture: true}, true)
	if err != nil {
		t.Fatalf("--once must treat RESOURCE_NOT_FOUND as an empty queue: %v", err)
	}
	if leased, ok := result["leased"].(bool); !ok || leased {
		t.Fatalf("empty queue result must report leased=false: %#v", result)
	}
	if prepareCalls != 1 {
		t.Fatalf("expected exactly one prepare_next request, got %d", prepareCalls)
	}
}

func TestRuntimeWorkerRetriesTransientHeartbeatWithoutInterruptingHarness(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	fake.QueueScript(agentadapter.FakeHarnessScript{Events: []agentadapter.FakeHarnessScriptEvent{{
		Type: "result.completed", Delay: 700 * time.Millisecond,
		Data: json.RawMessage(`{"output_refs":["artifact:1"],"output_digest":"sha256:result"}`),
	}}})
	registry := agentadapter.NewHarnessRegistry()
	if err := registry.Register("fake", fake); err != nil {
		t.Fatal(err)
	}
	leaseExpiry := time.Now().UTC().Add(2 * time.Second)
	handle := contentruntime.DispatchHandle{Node: domain.NodeRun{ID: "node-1"}, Attempt: domain.RuntimeAttempt{ID: "attempt-1", TenantID: "tenant-1", JobRunID: "job-1", HarnessKind: "fake", State: domain.RuntimeAttemptPrepared, FenceToken: "fence-1", LeaseExpiresAt: &leaseExpiry}, ContextView: domain.ContextView{Digest: "sha256:context"}, Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true}, ExecutionSpec: testRuntimeExecutionSpec("project-1", "job-1")}
	heartbeats := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var envelope struct {
			Command string `json:"command"`
		}
		_ = json.NewDecoder(request.Body).Decode(&envelope)
		switch envelope.Command {
		case "runtime.worker.prepare_next", "runtime.worker.activate":
			writeRuntimeWorkerResponse(t, writer, envelope.Command, handle)
		case "runtime.worker.heartbeat":
			heartbeats++
			if heartbeats == 1 {
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(map[string]any{"ok": false, "command": envelope.Command, "error": &domain.Error{Type: "network", Code: "UPSTREAM_TEMPORARY", Message: "temporary", Retryable: true}})
				return
			}
			next := time.Now().UTC().Add(2 * time.Second)
			handle.Attempt.LeaseExpiresAt = &next
			writeRuntimeWorkerResponse(t, writer, envelope.Command, handle)
		case "runtime.worker.event":
			writeRuntimeWorkerResponse(t, writer, envelope.Command, map[string]any{"recorded": true})
		case "runtime.worker.finalize":
			handle.Attempt.State = domain.RuntimeAttemptSucceeded
			writeRuntimeWorkerResponse(t, writer, envelope.Command, app.RuntimeWorkerResult{Handle: handle})
		}
	}))
	defer server.Close()
	root := &Root{runtimeHarnesses: registry, stderr: &strings.Builder{}}
	result, err := root.runRuntimeWorker(t.Context(), apiclient.New(server.URL, "device-token"), runtimeWorkerRunOptions{HarnessKind: "fake"}, false)
	if err != nil || result["state"] != domain.RuntimeAttemptSucceeded || heartbeats < 2 {
		t.Fatalf("transient heartbeat failure interrupted the worker: result=%#v heartbeats=%d err=%v", result, heartbeats, err)
	}
}

func TestRuntimeWorkerRetriesPendingEventBeforeSuccessfulFinalize(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if calls == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeRuntimeWorkerResponse(t, writer, "runtime.worker.event", map[string]any{"recorded": true})
	}))
	defer server.Close()

	leaseExpiry := time.Now().UTC().Add(2 * time.Second)
	handle := contentruntime.DispatchHandle{Attempt: domain.RuntimeAttempt{ID: "attempt-1", FenceToken: "fence-1", LeaseExpiresAt: &leaseExpiry}}
	pending := []agentadapter.AgentEvent{{Type: "item.completed", Session: agentadapter.AgentSessionRef{HarnessKind: "fake", SessionID: "session-1"}, OccurredAt: time.Now().UTC()}}
	root := &Root{stderr: &strings.Builder{}}
	remaining, err := root.retryRuntimeEvents(t.Context(), apiclient.New(server.URL, "device-token"), handle, pending)
	if err != nil || len(remaining) != 0 || calls != 2 {
		t.Fatalf("pending event retry = remaining=%d calls=%d err=%v", len(remaining), calls, err)
	}
}

func TestRuntimeWorkerInterruptsHarnessAfterStructuredProgressTimeout(t *testing.T) {
	stalled := newStalledHarness()
	registry := agentadapter.NewHarnessRegistry()
	if err := registry.Register("stalled", stalled); err != nil {
		t.Fatal(err)
	}
	leaseExpiry := time.Now().UTC().Add(time.Second)
	handle := contentruntime.DispatchHandle{Node: domain.NodeRun{ID: "node-1"}, Attempt: domain.RuntimeAttempt{ID: "attempt-1", TenantID: "tenant-1", JobRunID: "job-1", HarnessKind: "stalled", State: domain.RuntimeAttemptPrepared, FenceToken: "fence-1", LeaseExpiresAt: &leaseExpiry}, ContextView: domain.ContextView{Digest: "sha256:context"}, Capabilities: agentadapter.HarnessCapabilities{Kind: "stalled", Events: true, StructuredOutput: true}, ExecutionSpec: testRuntimeExecutionSpec("project-1", "job-1")}
	var finalized app.RuntimeWorkerFinalizeInput
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var envelope struct {
			Command string          `json:"command"`
			Params  json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(request.Body).Decode(&envelope)
		switch envelope.Command {
		case "runtime.worker.prepare_next", "runtime.worker.activate", "runtime.worker.heartbeat":
			writeRuntimeWorkerResponse(t, writer, envelope.Command, handle)
		case "runtime.worker.event":
			writeRuntimeWorkerResponse(t, writer, envelope.Command, map[string]any{"recorded": true})
		case "runtime.worker.finalize":
			_ = json.Unmarshal(envelope.Params, &finalized)
			handle.Attempt.State = finalized.State
			writeRuntimeWorkerResponse(t, writer, envelope.Command, app.RuntimeWorkerResult{Handle: handle})
		}
	}))
	defer server.Close()
	root := &Root{runtimeHarnesses: registry, stderr: &strings.Builder{}}
	_, err := root.runRuntimeWorker(t.Context(), apiclient.New(server.URL, "device-token"), runtimeWorkerRunOptions{HarnessKind: "stalled", IdleTimeout: 30 * time.Millisecond}, false)
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "HARNESS_PROGRESS_TIMEOUT" {
		t.Fatalf("worker progress timeout = %v", err)
	}
	if finalized.State != domain.RuntimeAttemptRetryableFailed || finalized.ErrorCode != "HARNESS_PROGRESS_TIMEOUT" {
		t.Fatalf("timeout finalize = %#v", finalized)
	}
	if !stalled.wasInterrupted() {
		t.Fatal("progress timeout did not interrupt the stalled Harness")
	}
}

func TestRuntimeEventAdvancesProgressIgnoresClaudeInternalFrames(t *testing.T) {
	ref := agentadapter.AgentSessionRef{HarnessKind: "claude", SessionID: "session-1"}
	for _, eventType := range []string{"system", "unknown"} {
		event := agentadapter.AgentEvent{Type: "session.progress", Session: ref, Data: json.RawMessage(`{"provider":"claude","event_type":"` + eventType + `"}`)}
		if runtimeEventAdvancesProgress(event) {
			t.Fatalf("Claude %s frame advanced the progress watchdog", eventType)
		}
	}
	for _, event := range []agentadapter.AgentEvent{
		{Type: "session.progress", Session: ref, Data: json.RawMessage(`{"provider":"claude","event_type":"message"}`)},
		{Type: "item.completed", Session: ref},
		{Type: "session.progress", Session: ref, Data: json.RawMessage(`{"provider":"remote"}`)},
	} {
		if !runtimeEventAdvancesProgress(event) {
			t.Fatalf("meaningful event did not advance progress: %#v", event)
		}
	}
}

func testRuntimeExecutionSpec(projectID, jobID string) contentruntime.RemoteExecutionSpec {
	return contentruntime.RemoteExecutionSpec{
		ProjectID: projectID, JobRunID: jobID, OutputSchema: json.RawMessage(`{"type":"object"}`), Skill: "# Test Skill\n",
		TaskContract: domain.TaskContract{
			ContractVersion: "1.0", ContractID: "contract-" + jobID, RunID: jobID, TaskType: "test",
			Project: domain.Project{ID: projectID}, OutputSchema: domain.KnowledgeCandidatesSchema,
			Capability: domain.Capability{ID: domain.KnowledgeExtractCapability, Digest: "sha256:" + strings.Repeat("a", 64)},
		},
	}
}

func runtimeWorkerHandleServer(t *testing.T, handle *contentruntime.DispatchHandle, finalized *app.RuntimeWorkerFinalizeInput) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var envelope struct {
			Command string          `json:"command"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
			t.Errorf("decode Runtime worker request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		switch envelope.Command {
		case "runtime.worker.prepare_next":
			writeRuntimeWorkerResponse(t, writer, envelope.Command, *handle)
		case "runtime.worker.activate":
			handle.Attempt.State = domain.RuntimeAttemptRunning
			next := time.Now().UTC().Add(180 * time.Millisecond)
			handle.Attempt.LeaseExpiresAt = &next
			writeRuntimeWorkerResponse(t, writer, envelope.Command, *handle)
		case "runtime.worker.heartbeat":
			next := time.Now().UTC().Add(180 * time.Millisecond)
			handle.Attempt.LeaseExpiresAt = &next
			writeRuntimeWorkerResponse(t, writer, envelope.Command, *handle)
		case "runtime.worker.event":
			writeRuntimeWorkerResponse(t, writer, envelope.Command, map[string]any{"recorded": true})
		case "runtime.worker.finalize":
			var input app.RuntimeWorkerFinalizeInput
			_ = json.Unmarshal(envelope.Params, &input)
			if finalized != nil {
				*finalized = input
			}
			handle.Attempt.State = input.State
			writeRuntimeWorkerResponse(t, writer, envelope.Command, app.RuntimeWorkerResult{Handle: *handle, Job: domain.JobRun{ID: handle.Attempt.JobRunID}})
		default:
			t.Errorf("unexpected Runtime worker command: %s", envelope.Command)
			writer.WriteHeader(http.StatusBadRequest)
		}
	}))
}

type captureEventStream struct {
	events chan agentadapter.AgentEvent
	once   sync.Once
}

func newCaptureEventStream() *captureEventStream {
	return &captureEventStream{events: make(chan agentadapter.AgentEvent, 2)}
}

func (stream *captureEventStream) Events() <-chan agentadapter.AgentEvent { return stream.events }
func (stream *captureEventStream) Close() error {
	stream.once.Do(func() { close(stream.events) })
	return nil
}

type workspaceCaptureHarness struct {
	mu        sync.Mutex
	root      string
	resources bool
}

func (harness *workspaceCaptureHarness) Detect(context.Context) (agentadapter.HarnessCapabilities, error) {
	return agentadapter.HarnessCapabilities{Kind: "workspace-capture", Events: true, StructuredOutput: true, MaxParallelSessions: 1}, nil
}

func (harness *workspaceCaptureHarness) Start(_ context.Context, request agentadapter.StartAgentRequest) (agentadapter.AgentSessionRef, agentadapter.EventStream, error) {
	harness.mu.Lock()
	harness.root = request.Workspace
	harness.resources = true
	for _, name := range []string{"lease.json", "contract.json", "output.schema.json", "SKILL.md"} {
		if _, err := os.Stat(filepath.Join(request.Workspace, name)); err != nil {
			harness.resources = false
		}
	}
	harness.mu.Unlock()
	ref := agentadapter.AgentSessionRef{TenantID: request.TenantID, HarnessKind: "workspace-capture", SessionID: "workspace-capture-session"}
	stream := newCaptureEventStream()
	stream.events <- agentadapter.AgentEvent{Type: "result.completed", Session: ref, Data: json.RawMessage(`{"output_refs":[],"safe_summary":{"isolated":true}}`), OccurredAt: time.Now().UTC()}
	return ref, stream, nil
}

func (harness *workspaceCaptureHarness) Resume(context.Context, agentadapter.ResumeAgentRequest) (agentadapter.EventStream, error) {
	return nil, errors.New("resume is not supported")
}
func (harness *workspaceCaptureHarness) Interrupt(context.Context, agentadapter.AgentSessionRef) error {
	return nil
}
func (harness *workspaceCaptureHarness) Inspect(context.Context, agentadapter.AgentSessionRef) (agentadapter.AgentSessionStatus, error) {
	return agentadapter.AgentSessionStatus{}, nil
}
func (harness *workspaceCaptureHarness) workspace() string {
	harness.mu.Lock()
	defer harness.mu.Unlock()
	return harness.root
}
func (harness *workspaceCaptureHarness) sawFrozenResources() bool {
	harness.mu.Lock()
	defer harness.mu.Unlock()
	return harness.resources
}

type leaseDriftHarness struct {
	mu     sync.Mutex
	root   string
	stream *captureEventStream
}

func (harness *leaseDriftHarness) Detect(context.Context) (agentadapter.HarnessCapabilities, error) {
	return agentadapter.HarnessCapabilities{Kind: "lease-drift", Events: true, StructuredOutput: true, MaxParallelSessions: 1}, nil
}

func (harness *leaseDriftHarness) Start(_ context.Context, request agentadapter.StartAgentRequest) (agentadapter.AgentSessionRef, agentadapter.EventStream, error) {
	if err := os.WriteFile(filepath.Join(request.Workspace, "lease.json"), []byte("{}\n"), 0o600); err != nil {
		return agentadapter.AgentSessionRef{}, nil, err
	}
	ref := agentadapter.AgentSessionRef{TenantID: request.TenantID, HarnessKind: "lease-drift", SessionID: "lease-drift-session"}
	stream := newCaptureEventStream()
	stream.events <- agentadapter.AgentEvent{Type: "session.started", Session: ref, OccurredAt: time.Now().UTC()}
	harness.mu.Lock()
	harness.root, harness.stream = request.Workspace, stream
	harness.mu.Unlock()
	return ref, stream, nil
}

func (harness *leaseDriftHarness) Resume(context.Context, agentadapter.ResumeAgentRequest) (agentadapter.EventStream, error) {
	return nil, errors.New("resume is not supported")
}
func (harness *leaseDriftHarness) Interrupt(context.Context, agentadapter.AgentSessionRef) error {
	harness.mu.Lock()
	stream := harness.stream
	harness.mu.Unlock()
	if stream != nil {
		return stream.Close()
	}
	return nil
}
func (harness *leaseDriftHarness) Inspect(context.Context, agentadapter.AgentSessionRef) (agentadapter.AgentSessionStatus, error) {
	return agentadapter.AgentSessionStatus{}, nil
}
func (harness *leaseDriftHarness) workspace() string {
	harness.mu.Lock()
	defer harness.mu.Unlock()
	return harness.root
}

type stalledHarness struct {
	mu          sync.Mutex
	interrupted bool
	stream      *stalledEventStream
}

type stalledEventStream struct {
	events chan agentadapter.AgentEvent
	once   sync.Once
}

func newStalledHarness() *stalledHarness {
	return &stalledHarness{stream: &stalledEventStream{events: make(chan agentadapter.AgentEvent, 1)}}
}

func (h *stalledHarness) Detect(context.Context) (agentadapter.HarnessCapabilities, error) {
	return agentadapter.HarnessCapabilities{Kind: "stalled", Events: true, StructuredOutput: true}, nil
}

func (h *stalledHarness) Start(_ context.Context, request agentadapter.StartAgentRequest) (agentadapter.AgentSessionRef, agentadapter.EventStream, error) {
	ref := agentadapter.AgentSessionRef{TenantID: request.TenantID, HarnessKind: "stalled", SessionID: "stalled-session"}
	h.stream.events <- agentadapter.AgentEvent{Type: "session.started", Session: ref, OccurredAt: time.Now().UTC()}
	return ref, h.stream, nil
}

func (h *stalledHarness) Resume(context.Context, agentadapter.ResumeAgentRequest) (agentadapter.EventStream, error) {
	return h.stream, nil
}

func (h *stalledHarness) Interrupt(context.Context, agentadapter.AgentSessionRef) error {
	h.mu.Lock()
	h.interrupted = true
	h.mu.Unlock()
	return h.stream.Close()
}

func (h *stalledHarness) Inspect(_ context.Context, ref agentadapter.AgentSessionRef) (agentadapter.AgentSessionStatus, error) {
	return agentadapter.AgentSessionStatus{Session: ref, State: "active"}, nil
}

func (h *stalledHarness) wasInterrupted() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.interrupted
}

func (s *stalledEventStream) Events() <-chan agentadapter.AgentEvent { return s.events }

func (s *stalledEventStream) Close() error {
	s.once.Do(func() { close(s.events) })
	return nil
}

func TestDaemonBackoffIsBoundedAndJittered(t *testing.T) {
	if got := daemonBackoffDelay(0, 0); got != 500*time.Millisecond {
		t.Fatalf("initial backoff = %s", got)
	}
	if got := daemonBackoffDelay(20, 1); got != 37500*time.Millisecond {
		t.Fatalf("bounded jittered backoff = %s", got)
	}
	if got := runtimeDispatchRetryDelay(20, 1); got != 6250*time.Millisecond {
		t.Fatalf("bounded dispatch retry = %s", got)
	}
}

func TestRuntimeControlURLAndReconnectBackoff(t *testing.T) {
	url, err := runtimeControlURL("https://content.example/base/")
	if err != nil || url != "wss://content.example/base/api/v1/runtime/worker/control" {
		t.Fatalf("control URL = %q err=%v", url, err)
	}
	if got := runtimeWakeReconnectDelay(20, 1); got != 37500*time.Millisecond {
		t.Fatalf("bounded reconnect delay = %s", got)
	}
}

func TestRuntimeWakeClientCoalescesDuplicateAvailabilityFrames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close(websocket.StatusNormalClosure, "")
		_, body, err := connection.Read(request.Context())
		if err != nil {
			return
		}
		var syncFrame runtimeWakeFrame
		if json.Unmarshal(body, &syncFrame) != nil || syncFrame.Type != "control.sync_state" {
			return
		}
		ready, _ := json.Marshal(runtimeWakeFrame{Type: "control.ready", DaemonInstanceID: syncFrame.DaemonInstanceID, ConnectionEpoch: syncFrame.ConnectionEpoch, ReportSequence: syncFrame.ReportSequence})
		_ = connection.Write(request.Context(), websocket.MessageText, ready)
		for i := 0; i < 3; i++ {
			_ = connection.Write(request.Context(), websocket.MessageText, []byte(`{"type":"runtime.available"}`))
		}
		<-request.Context().Done()
	}))
	defer server.Close()
	wake := make(chan struct{}, 1)
	for i := 0; i < 3; i++ {
		signalRuntimeWake(wake)
	}
	if len(wake) != 1 {
		t.Fatalf("coalesced wake count = %d", len(wake))
	}
	<-wake
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- readRuntimeWakeConnection(ctx, server.URL, "device-token", wake, nil) }()
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("wake client did not surface an availability frame")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("wake client did not stop after cancellation")
	}
}

func TestRuntimeWakeClientSyncsActiveAttemptsOnStateChange(t *testing.T) {
	reported := make(chan runtimeWakeFrame, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close(websocket.StatusNormalClosure, "")
		_, body, err := connection.Read(request.Context())
		if err != nil {
			return
		}
		var initial runtimeWakeFrame
		if json.Unmarshal(body, &initial) != nil {
			return
		}
		ready, _ := json.Marshal(runtimeWakeFrame{Type: "control.ready", DaemonInstanceID: initial.DaemonInstanceID, ConnectionEpoch: initial.ConnectionEpoch, ReportSequence: initial.ReportSequence})
		if connection.Write(request.Context(), websocket.MessageText, ready) != nil {
			return
		}
		_, body, err = connection.Read(request.Context())
		if err != nil {
			return
		}
		var update runtimeWakeFrame
		if json.Unmarshal(body, &update) == nil {
			reported <- update
		}
		<-request.Context().Done()
	}))
	defer server.Close()
	state := newRuntimeWakeClientState("test", nil)
	accepted := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- readRuntimeWakeConnectionWithState(ctx, server.URL, "device-token", make(chan struct{}, 1), func() { signalRuntimeWake(accepted) }, state)
	}()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("control channel did not become ready")
	}
	state.setAttempt("attempt-1", true)
	select {
	case frame := <-reported:
		if frame.ReportSequence != 2 || len(frame.ActiveAttempts) != 1 || frame.ActiveAttempts[0] != "attempt-1" {
			t.Fatalf("state-change current-state frame = %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("active Attempt change was not synchronized immediately")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("wake client did not stop after cancellation")
	}
}

func TestRuntimeWakeClientSendsRedactedWorkspaceInventoryInInitialFrame(t *testing.T) {
	reported := make(chan runtimeWakeFrame, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.CloseNow()
		_, body, err := connection.Read(request.Context())
		if err != nil {
			return
		}
		var frame runtimeWakeFrame
		if json.Unmarshal(body, &frame) != nil {
			return
		}
		reported <- frame
		ready, _ := json.Marshal(runtimeWakeFrame{Type: "control.ready", DaemonInstanceID: frame.DaemonInstanceID, ConnectionEpoch: frame.ConnectionEpoch, ReportSequence: frame.ReportSequence})
		_ = connection.Write(request.Context(), websocket.MessageText, ready)
		<-request.Context().Done()
	}))
	defer server.Close()

	privateRoot := filepath.Join(t.TempDir(), "customer-private-workspace")
	state := newRuntimeWakeClientState("test", map[string]any{"runtime_status": "healthy"})
	state.setWorkspaceObservations([]domain.DaemonWorkspaceObservation{{
		WorkspaceID: "workspace-1", ProjectID: "project-1", Status: "ready", Reason: "local_components_observed",
		Generation: "sha256:generation", EnvironmentDeclaration: "sha256:environment", PluginDeclaration: "sha256:plugin",
		SkillDeclaration: "sha256:skill", MCPDeclaration: "sha256:mcp", WorkspaceDeclaration: "sha256:workspace", ObservedAt: time.Now().UTC(),
	}})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- readRuntimeWakeConnectionWithState(ctx, server.URL, "device-token", make(chan struct{}, 1), nil, state)
	}()
	select {
	case frame := <-reported:
		body, _ := json.Marshal(frame)
		if len(frame.WorkspaceObservations) != 1 || frame.WorkspaceObservations[0].WorkspaceID != "workspace-1" || frame.WorkspaceObservations[0].Generation != "sha256:generation" {
			t.Fatalf("initial workspace current-state = %#v", frame)
		}
		if strings.Contains(string(body), privateRoot) || strings.Contains(string(body), "customer-private-workspace") {
			t.Fatalf("initial current-state leaked an absolute workspace path: %s", body)
		}
	case <-time.After(time.Second):
		t.Fatal("control client did not send workspace inventory in its initial frame")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("workspace inventory client did not stop after cancellation")
	}
}

func TestObserveDaemonWorkspacesNeverSerializesLocalRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "customer-private-workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	observations := observeDaemonWorkspaces([]localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: root}})
	if len(observations) != 1 || observations[0].WorkspaceID != "workspace-1" || observations[0].ProjectID != "project-1" {
		t.Fatalf("workspace observations = %#v", observations)
	}
	body, err := json.Marshal(observations)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), root) || strings.Contains(string(body), "customer-private-workspace") {
		t.Fatalf("workspace observations leaked local root: %s", body)
	}
}

func TestRuntimeWakeClientResendsWorkspaceInventoryWhenGenerationChanges(t *testing.T) {
	reported := make(chan runtimeWakeFrame, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.CloseNow()
		_, body, err := connection.Read(request.Context())
		if err != nil {
			return
		}
		var initial runtimeWakeFrame
		if json.Unmarshal(body, &initial) != nil {
			return
		}
		ready, _ := json.Marshal(runtimeWakeFrame{Type: "control.ready", DaemonInstanceID: initial.DaemonInstanceID, ConnectionEpoch: initial.ConnectionEpoch, ReportSequence: initial.ReportSequence})
		if connection.Write(request.Context(), websocket.MessageText, ready) != nil {
			return
		}
		for {
			_, body, err = connection.Read(request.Context())
			if err != nil {
				return
			}
			var update runtimeWakeFrame
			if json.Unmarshal(body, &update) == nil && len(update.WorkspaceObservations) == 1 && update.WorkspaceObservations[0].Generation == "sha256:second" {
				reported <- update
				break
			}
		}
		<-request.Context().Done()
	}))
	defer server.Close()

	state := newRuntimeWakeClientState("test", map[string]any{"runtime_status": "healthy"})
	state.setWorkspaceObservations([]domain.DaemonWorkspaceObservation{{WorkspaceID: "workspace-1", ProjectID: "project-1", Status: "ready", Reason: "local_components_observed", Generation: "sha256:first", ObservedAt: time.Now().UTC()}})
	accepted := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- readRuntimeWakeConnectionWithState(ctx, server.URL, "device-token", make(chan struct{}, 1), func() { signalRuntimeWake(accepted) }, state)
	}()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("control channel did not accept the initial workspace inventory")
	}
	state.setWorkspaceObservations([]domain.DaemonWorkspaceObservation{{WorkspaceID: "workspace-1", ProjectID: "project-1", Status: "repair_required", Reason: "skill_drift", Generation: "sha256:second", ObservedAt: time.Now().UTC()}})
	select {
	case frame := <-reported:
		if frame.ReportSequence < 2 || len(frame.WorkspaceObservations) != 1 || frame.WorkspaceObservations[0].Generation != "sha256:second" || frame.WorkspaceObservations[0].Status != "repair_required" {
			t.Fatalf("workspace state-change frame = %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("workspace generation change was not synchronized immediately")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("workspace generation client did not stop after cancellation")
	}
}

func TestRuntimeWakeClientReconnectsWithFullStateAndNewEpoch(t *testing.T) {
	reported := make(chan runtimeWakeFrame, 2)
	var connections int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.CloseNow()
		connections++
		_, body, err := connection.Read(request.Context())
		if err != nil {
			return
		}
		var frame runtimeWakeFrame
		if json.Unmarshal(body, &frame) != nil {
			return
		}
		reported <- frame
		ready, _ := json.Marshal(runtimeWakeFrame{Type: "control.ready", DaemonInstanceID: frame.DaemonInstanceID, ConnectionEpoch: frame.ConnectionEpoch, ReportSequence: frame.ReportSequence})
		if connection.Write(request.Context(), websocket.MessageText, ready) != nil {
			return
		}
		if connections == 1 {
			_ = connection.Close(websocket.StatusGoingAway, "test reconnect")
			return
		}
		<-request.Context().Done()
	}))
	defer server.Close()

	state := newRuntimeWakeClientState("test", map[string]any{"runtime_status": "healthy"})
	state.setAttempt("attempt-reconnect", true)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		runRuntimeWakeClientWithState(ctx, server.URL, "device-token", make(chan struct{}, 1), &strings.Builder{}, nil, state, nil)
		close(done)
	}()

	var first, second runtimeWakeFrame
	select {
	case first = <-reported:
	case <-time.After(2 * time.Second):
		t.Fatal("control client did not send its initial current-state")
	}
	select {
	case second = <-reported:
	case <-time.After(4 * time.Second):
		t.Fatal("control client did not reconnect and resynchronize")
	}
	if first.DaemonInstanceID == "" || second.DaemonInstanceID != first.DaemonInstanceID {
		t.Fatalf("reconnect changed DaemonInstance identity: first=%#v second=%#v", first, second)
	}
	if first.ConnectionEpoch != 1 || second.ConnectionEpoch != 2 || first.ReportSequence != 1 || second.ReportSequence != 1 {
		t.Fatalf("reconnect did not start a new fenced epoch: first=%#v second=%#v", first, second)
	}
	for _, frame := range []runtimeWakeFrame{first, second} {
		if len(frame.ActiveAttempts) != 1 || frame.ActiveAttempts[0] != "attempt-reconnect" || frame.Capabilities["runtime_status"] != "healthy" {
			t.Fatalf("reconnect did not send full current-state: %#v", frame)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("control client did not stop after reconnect test cancellation")
	}
}

func TestRuntimeWakeClientStateGatesNewWorkWhenSelectedHarnessIsUnavailable(t *testing.T) {
	state := newRuntimeWakeClientState("test", map[string]any{"runtime_status": "healthy"})
	if !state.runtimeAvailable() {
		t.Fatal("healthy selected Harness did not admit new work")
	}
	state.setCapabilities(map[string]any{"runtime_status": "unavailable", "runtime_reason": "selected_harness_unavailable"})
	if state.runtimeAvailable() {
		t.Fatal("unavailable selected Harness still admitted new work")
	}
}

func TestRuntimeWakeClientStopsAfterCredentialRejection(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	observations := make(chan runtimeWakeObservation, 8)
	done := make(chan struct{})
	go func() {
		runRuntimeWakeClient(t.Context(), server.URL, "expired-device-token", make(chan struct{}, 1), &strings.Builder{}, func(observation runtimeWakeObservation) {
			observations <- observation
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("wake client retried a rejected credential")
	}
	if requests != 1 {
		t.Fatalf("rejected credential handshake count = %d", requests)
	}
	last := runtimeWakeObservation{}
	for len(observations) > 0 {
		last = <-observations
	}
	if last.State != "auth_rejected" || last.ErrorCode != "DEVICE_AUTH_REJECTED" {
		t.Fatalf("terminal control observation = %#v", last)
	}
}

func TestRuntimeWakeConnectionRequiresAValidControlFrameBeforeAcceptance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			return
		}
		_ = connection.Close(websocket.StatusGoingAway, "edge closed after upgrade")
	}))
	defer server.Close()
	accepted := false
	err := readRuntimeWakeConnection(t.Context(), server.URL, "device-token", make(chan struct{}, 1), func() { accepted = true })
	if err == nil {
		t.Fatal("upgrade-only connection unexpectedly succeeded")
	}
	if accepted {
		t.Fatal("upgrade-only connection reset reconnect backoff before a valid control frame")
	}
}

func writeRuntimeWorkerResponse(t *testing.T, writer http.ResponseWriter, command string, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"ok": true, "command": command, "request_id": "request-test", "data": value, "meta": map[string]any{}}); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
