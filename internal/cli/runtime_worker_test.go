package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

func TestRuntimeWorkerUsesHarnessSessionAndRenewsLeaseUntilResult(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	fake.QueueScript(agentadapter.FakeHarnessScript{Events: []agentadapter.FakeHarnessScriptEvent{{
		Type: "result.completed", Delay: 90 * time.Millisecond,
		Data: json.RawMessage(`{"output_refs":["artifact:1"],"output_digest":"sha256:result","safe_summary":{"done":true},"used_cost_minor":4}`),
	}}})
	registry := agentadapter.NewHarnessRegistry()
	if err := registry.Register("fake", fake); err != nil {
		t.Fatal(err)
	}

	leaseExpiry := time.Now().UTC().Add(120 * time.Millisecond)
	handle := contentruntime.DispatchHandle{
		Node:         domain.NodeRun{ID: "node-1"},
		Attempt:      domain.RuntimeAttempt{ID: "attempt-1", TenantID: "tenant-1", JobRunID: "job-1", HarnessKind: "fake", State: domain.RuntimeAttemptPrepared, FenceToken: "fence-1", LeaseExpiresAt: &leaseExpiry},
		ContextView:  domain.ContextView{Digest: "sha256:context"},
		Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, Resume: true, StructuredOutput: true, MaxParallelSessions: 128},
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
			writeRuntimeWorkerResponse(t, writer, envelope.Command, handle)
		case "runtime.worker.heartbeat":
			heartbeats++
			next := time.Now().UTC().Add(120 * time.Millisecond)
			handle.Attempt.LeaseExpiresAt = &next
			writeRuntimeWorkerResponse(t, writer, envelope.Command, handle)
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
	result, err := root.runRuntimeWorker(t.Context(), apiclient.New(server.URL, "device-token"), runtimeWorkerRunOptions{HarnessKind: "fake", Role: "worker", Profile: "test", Workspace: t.TempDir()}, false)
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

func TestRuntimeWorkerFinalizeInputSeparatesBusinessPayloadFromExecutionEnvelope(t *testing.T) {
	handle := contentruntime.DispatchHandle{Attempt: domain.RuntimeAttempt{ID: "attempt-1", FenceToken: "fence-1"}}
	business := json.RawMessage(`{"schema_version":"1.0","candidates":[],"warnings":[]}`)
	input, err := runtimeWorkerFinalizeInput(handle, "codex", business)
	if err != nil || string(input.BusinessPayload) != string(business) || len(input.OutputRefs) != 0 {
		t.Fatalf("business result was not preserved: input=%#v err=%v", input, err)
	}
}

func writeRuntimeWorkerResponse(t *testing.T, writer http.ResponseWriter, command string, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"ok": true, "command": command, "request_id": "request-test", "data": value, "meta": map[string]any{}}); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
