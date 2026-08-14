package agentadapter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaudeHarnessStreamsSafeEventsAndResumesAcrossInstances(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "claude-args.jsonl")
	workspace := writeCodexHarnessWorkspace(t)
	first := testClaudeStreamHarness(argsPath)
	caps, err := first.Detect(t.Context())
	if err != nil || !caps.Events || !caps.Resume || !caps.MCPStdio || caps.Kind != "claude" {
		t.Fatalf("unexpected Claude capabilities: %#v err=%v", caps, err)
	}
	ref, stream, err := first.Start(t.Context(), StartAgentRequest{TenantID: "tenant-1", NodeRunID: "node-1", AttemptID: "attempt-1", Workspace: workspace, Prompt: "开始"})
	if err != nil {
		t.Fatal(err)
	}
	events := collectClaudeHarnessEvents(t, stream)
	if len(events) < 3 || events[0].Type != "session.started" || events[len(events)-1].Type != "result.completed" {
		t.Fatalf("unexpected Claude event sequence: %#v", events)
	}
	if strings.Contains(string(events[1].Data), "private transcript") {
		t.Fatalf("Claude transcript escaped the structured event boundary: %#v", events[1])
	}
	if ref.SessionID == "" || ref.HarnessKind != "claude" {
		t.Fatalf("Claude session identity missing: %#v", ref)
	}
	second := testClaudeStreamHarness(argsPath)
	resumed, err := second.Resume(t.Context(), ResumeAgentRequest{TenantID: "tenant-1", Session: ref, Workspace: workspace, Prompt: "继续"})
	if err != nil {
		t.Fatal(err)
	}
	resumedEvents := collectClaudeHarnessEvents(t, resumed)
	if len(resumedEvents) < 2 || resumedEvents[0].Type != "session.resumed" || resumedEvents[len(resumedEvents)-1].Type != "result.completed" {
		t.Fatalf("unexpected Claude resume event sequence: %#v", resumedEvents)
	}
}

func TestClaudeHarnessUsesStrictAttemptScopedRuntimeMCPConfig(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "claude-runtime-args.jsonl")
	harness := testClaudeStreamHarness(argsPath)
	_, stream, err := harness.Start(t.Context(), StartAgentRequest{
		TenantID: "tenant-1", NodeRunID: "node-1", AttemptID: "attempt-1", Workspace: writeCodexHarnessWorkspace(t),
		RuntimeGateway: RuntimeGatewayConfig{URL: "https://content.example/api/v1/runtime/mcp/call", Token: "rtg_claude", AllowedTools: []string{"runtime.state.query"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = collectClaudeHarnessEvents(t, stream)
	body, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(body))), &args); err != nil {
		t.Fatal(err)
	}
	if !containsArg(args, "--strict-mcp-config") {
		t.Fatalf("Claude Runtime MCP config was not strict: %#v", args)
	}
	config := argValue(args, "--mcp-config")
	if !strings.Contains(config, `"contentcloud-runtime"`) || !strings.Contains(config, `"runtime-serve"`) {
		t.Fatalf("Claude Runtime MCP config missing stdio shim: %s", config)
	}
}

func TestClaudeStructuredFailureClassifiesAuthAndRateLimitWithoutMessage(t *testing.T) {
	for _, test := range []struct {
		message string
		want    string
	}{
		{message: "HTTP 429 too many requests", want: "CLAUDE_RATE_LIMITED"},
		{message: "invalid api key", want: "CLAUDE_AUTH_REQUIRED"},
	} {
		got, terminal := projectClaudeEvent(AgentSessionRef{HarnessKind: "claude", SessionID: "session-1"}, claudeJSONEvent{Type: "result", Subtype: "error_during_execution", Error: test.message})
		if got.ErrorCode != test.want || !terminal {
			t.Fatalf("claude structured failure %q = %#v terminal=%t, want %q", test.message, got, terminal, test.want)
		}
	}
}

func TestClaudeIsErrorCannotBeAcceptedAsSuccessfulResult(t *testing.T) {
	event, terminal := projectClaudeEvent(AgentSessionRef{HarnessKind: "claude", SessionID: "session-1"}, claudeJSONEvent{Type: "result", Subtype: "success", IsError: true, Result: json.RawMessage(`{"schema_version":"1.0"}`)})
	if !terminal || event.Type != "session.failed" || event.ErrorCode != "CLAUDE_RESULT_FAILED" {
		t.Fatalf("Claude is_error result was accepted: event=%#v terminal=%t", event, terminal)
	}
}

func testClaudeStreamHarness(argsPath string) *claudeStreamHarness {
	return &claudeStreamHarness{binary: os.Args[0], prefixArgs: []string{"-test.run=TestClaudeHarnessHelperProcess", "--"}, extraEnv: []string{"CLAUDE_HARNESS_HELPER=1", "CLAUDE_HARNESS_ARGS_PATH=" + argsPath}, sessions: map[string]*claudeStreamSession{}, detect: func(context.Context) (string, error) { return "test", nil }}
}

func TestClaudeHarnessBoundsFirstStructuredEventHandshake(t *testing.T) {
	harness := testClaudeStreamHarness(filepath.Join(t.TempDir(), "claude-args.jsonl"))
	harness.handshakeTimeout = 50 * time.Millisecond
	harness.extraEnv = append(harness.extraEnv, "CLAUDE_HARNESS_MODE=handshake_hang")
	started := time.Now()
	_, _, err := harness.Start(t.Context(), StartAgentRequest{TenantID: "tenant-1", NodeRunID: "node-1", AttemptID: "attempt-1", Workspace: writeCodexHarnessWorkspace(t)})
	if !containsDomainCode(err, "CLAUDE_HANDSHAKE_TIMEOUT") {
		t.Fatalf("Claude handshake timeout = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("Claude handshake cleanup took %s", elapsed)
	}
}

func collectClaudeHarnessEvents(t *testing.T, stream EventStream) []AgentEvent {
	t.Helper()
	defer stream.Close()
	events := []AgentEvent{}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event, ok := <-stream.Events():
			if !ok {
				return events
			}
			events = append(events, event)
		case <-deadline.C:
			t.Fatalf("timed out waiting for Claude events: %#v", events)
		}
	}
}

func TestClaudeHarnessHelperProcess(t *testing.T) {
	if os.Getenv("CLAUDE_HARNESS_HELPER") != "1" {
		return
	}
	args := os.Args
	for index, value := range args {
		if value == "--" {
			args = args[index+1:]
			break
		}
	}
	if path := os.Getenv("CLAUDE_HARNESS_ARGS_PATH"); path != "" {
		body, _ := json.Marshal(args)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.Write(append(body, '\n'))
		_ = file.Close()
	}
	sessionID := ""
	for index, value := range args {
		if value == "--resume" && index+1 < len(args) {
			sessionID = args[index+1]
		}
		if value == "--session-id" && index+1 < len(args) {
			sessionID = args[index+1]
		}
	}
	if sessionID == "" {
		t.Fatal("missing Claude session argument")
	}
	if os.Getenv("CLAUDE_HARNESS_MODE") == "handshake_hang" {
		time.Sleep(30 * time.Second)
		os.Exit(22)
	}
	encoder := json.NewEncoder(os.Stdout)
	_ = encoder.Encode(map[string]any{"type": "system", "subtype": "init", "session_id": sessionID})
	_ = encoder.Encode(map[string]any{"type": "assistant", "session_id": sessionID, "message": "private transcript"})
	_ = encoder.Encode(map[string]any{"type": "result", "subtype": "success", "session_id": sessionID, "result": `{"schema_version":"1.0"}`})
}
