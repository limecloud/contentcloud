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
	if err != nil || !caps.Events || !caps.Resume || caps.Kind != "claude" {
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

func testClaudeStreamHarness(argsPath string) *claudeStreamHarness {
	return &claudeStreamHarness{binary: os.Args[0], prefixArgs: []string{"-test.run=TestClaudeHarnessHelperProcess", "--"}, extraEnv: []string{"CLAUDE_HARNESS_HELPER=1", "CLAUDE_HARNESS_ARGS_PATH=" + argsPath}, sessions: map[string]*claudeStreamSession{}, detect: func(context.Context) error { return nil }}
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
	encoder := json.NewEncoder(os.Stdout)
	_ = encoder.Encode(map[string]any{"type": "system", "subtype": "init", "session_id": sessionID})
	_ = encoder.Encode(map[string]any{"type": "assistant", "session_id": sessionID, "message": "private transcript"})
	_ = encoder.Encode(map[string]any{"type": "result", "subtype": "success", "session_id": sessionID, "result": `{"schema_version":"1.0"}`})
}
