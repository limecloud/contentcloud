package agentadapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRemoteHTTPHarnessRunsPiAgentStructuredSessionAndInspects(t *testing.T) {
	var eventPolls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/capabilities":
			_ = json.NewEncoder(w).Encode(HarnessCapabilities{Kind: "pi", Events: true, Resume: true, StructuredOutput: true, MaxParallelSessions: 4})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			_ = json.NewEncoder(w).Encode(remoteSessionResponse{SessionID: "remote-session-1", State: "active"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/remote-session-1/events":
			if eventPolls.Add(1) == 1 {
				_ = json.NewEncoder(w).Encode(remoteEventsResponse{Events: []remoteEvent{{Type: "result.completed", Data: json.RawMessage(`{"schema_version":"1.0"}`)}}, Terminal: true, State: "completed"})
			} else {
				_ = json.NewEncoder(w).Encode(remoteEventsResponse{Terminal: true, State: "completed"})
			}
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/remote-session-1":
			_ = json.NewEncoder(w).Encode(remoteSessionResponse{SessionID: "remote-session-1", State: "completed"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	harness, err := NewRemoteHTTPHarness(RemoteHTTPHarnessConfig{Kind: "pi", Endpoint: server.URL, Token: "test-token", HTTPClient: server.Client(), PollInterval: time.Millisecond, AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	caps, err := harness.Detect(t.Context())
	if err != nil || !caps.Events || caps.Kind != "pi" {
		t.Fatalf("unexpected capabilities %#v err=%v", caps, err)
	}
	ref, stream, err := harness.Start(t.Context(), StartAgentRequest{TenantID: "tenant-1", JobRunID: "job-1", NodeRunID: "node-1", AttemptID: "attempt-1", ContextDigest: "sha256:context", Workspace: writeCodexHarnessWorkspace(t), Prompt: "生成候选"})
	if err != nil {
		t.Fatal(err)
	}
	if ref.HarnessKind != "pi" {
		t.Fatalf("Pi Agent session lost its provider-neutral harness kind: %#v", ref)
	}
	events := collectHarnessEvents(t, stream)
	if len(events) < 2 || events[0].Type != "session.started" || events[len(events)-1].Type != "result.completed" {
		t.Fatalf("unexpected remote events %#v", events)
	}
	if !strings.Contains(string(events[len(events)-1].Data), "schema_version") {
		t.Fatalf("structured result missing: %#v", events[len(events)-1])
	}
	status, err := harness.Inspect(t.Context(), ref)
	if err != nil || status.State != "completed" {
		t.Fatalf("unexpected status %#v err=%v", status, err)
	}
}

func TestRemoteHTTPHarnessReportsUnknownOnPollFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/capabilities":
			_ = json.NewEncoder(w).Encode(HarnessCapabilities{Kind: "remote-http", Events: true, Resume: true, StructuredOutput: true, MaxParallelSessions: 1})
		case "/v1/sessions":
			_ = json.NewEncoder(w).Encode(remoteSessionResponse{SessionID: "remote-session-unknown", State: "active"})
		default:
			http.Error(w, "down", http.StatusBadGateway)
		}
	}))
	defer server.Close()
	harness, err := NewRemoteHTTPHarness(RemoteHTTPHarnessConfig{Endpoint: server.URL, HTTPClient: server.Client(), PollInterval: time.Millisecond, AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	_, stream, err := harness.Start(t.Context(), StartAgentRequest{TenantID: "tenant-1", JobRunID: "job-1", NodeRunID: "node-1", AttemptID: "attempt-1", ContextDigest: "sha256:context", Workspace: writeCodexHarnessWorkspace(t)})
	if err != nil {
		t.Fatal(err)
	}
	events := collectHarnessEvents(t, stream)
	if len(events) < 2 || events[len(events)-1].Type != "session.unknown" || events[len(events)-1].ErrorCode != "REMOTE_AGENT_EVENT_POLL_FAILED" {
		t.Fatalf("unexpected unknown events %#v", events)
	}
}
