package runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestRuntimeDispatchCompletesWithPiAgentHTTPHarness(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/capabilities":
			_ = json.NewEncoder(w).Encode(agentadapter.HarnessCapabilities{Kind: "pi", Events: true, Resume: true, StructuredOutput: true, MaxParallelSessions: 2})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"session_id": "pi-session-1", "state": "active"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/pi-session-1/events":
			_ = json.NewEncoder(w).Encode(map[string]any{"events": []map[string]any{{"id": "event-1", "type": "result.completed", "data": map[string]any{"output_refs": []string{"artifact:pi-result"}, "output_digest": "sha256:pi-result", "safe_summary": map[string]any{"executor": "pi", "api_token": "must-redact"}, "used_cost_minor": 7}, "occurred_at": time.Now().UTC()}}, "cursor": "event-1", "terminal": true, "state": "completed"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	harness, err := agentadapter.NewRemoteHTTPHarness(agentadapter.RemoteHTTPHarnessConfig{Kind: "pi", Endpoint: server.URL, HTTPClient: server.Client(), PollInterval: time.Millisecond, AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	registry := agentadapter.NewHarnessRegistry()
	if err := registry.Register("pi", harness); err != nil {
		t.Fatal(err)
	}
	repo := memory.New()
	service := NewWithHarnessRegistry(repo, time.Now, registry)
	started, err := service.Start(t.Context(), testStartInput("task-pi", domain.NewID()))
	if err != nil {
		t.Fatal(err)
	}
	input := dispatchInput(started.Job.ID)
	input.HarnessKind = "pi"
	input.ExecutionProfileID = "profile-pi-http"
	input.Workspace = writeRuntimeHarnessWorkspace(t)
	dispatched, err := service.DispatchNext(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if dispatched.Handle.Attempt.State != domain.RuntimeAttemptSucceeded || dispatched.Handle.Attempt.HarnessKind != "pi" || dispatched.Handle.Agent.HarnessKind != "pi" || dispatched.Handle.Node.OutputDigest != "sha256:pi-result" {
		t.Fatalf("Pi Agent did not complete the existing Runtime protocol: %#v", dispatched.Handle)
	}
	if dispatched.Handle.Attempt.SafeSummary["api_token"] != "[redacted]" || dispatched.Handle.Agent.UsedCostMinor != 7 {
		t.Fatalf("Pi receipt cost or redaction was lost: %#v", dispatched.Handle.Attempt)
	}
}

func writeRuntimeHarnessWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "attempt")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	contract := domain.TaskContract{ContractVersion: "1.0", ContractID: "contract-pi", RunID: "run-pi", TaskType: "test", Project: domain.Project{ID: "project-1"}, Capability: domain.Capability{ID: "content.compose"}}
	contractBody, _ := json.Marshal(contract)
	for name, body := range map[string][]byte{"contract.json": contractBody, "output.schema.json": []byte(`{"type":"object"}`), "SKILL.md": []byte("# Pi Agent Test\n")} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o400); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
