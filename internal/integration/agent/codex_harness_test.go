package agentadapter

import (
	"context"
	"encoding/json"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexHarnessStreamsSafeEventsAndStructuredResult(t *testing.T) {
	threadID := "019c-test-codex-thread"
	argsPath := filepath.Join(t.TempDir(), "args.jsonl")
	harness := testCodexExecHarness(threadID, "success", argsPath)
	capabilities, err := harness.Detect(t.Context())
	if err != nil || !capabilities.Resume || !capabilities.Events || !capabilities.MCPStdio || capabilities.Kind != "codex" {
		t.Fatalf("unexpected Codex capabilities: %#v err=%v", capabilities, err)
	}

	ref, stream, err := harness.Start(t.Context(), StartAgentRequest{
		TenantID: "tenant-1", NodeRunID: "node-1", AttemptID: "attempt-1",
		Workspace: writeCodexHarnessWorkspace(t), Prompt: "只处理冻结契约",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.SessionID != threadID || ref.HarnessKind != "codex" || ref.TenantID != "tenant-1" {
		t.Fatalf("Codex thread identity was not preserved: %#v", ref)
	}
	events := collectHarnessEvents(t, stream)
	if len(events) < 5 || events[0].Type != "session.started" || events[len(events)-1].Type != "result.completed" {
		t.Fatalf("unexpected Codex event sequence: %#v", events)
	}
	for _, event := range events {
		if strings.Contains(string(event.Data), "private transcript") {
			t.Fatalf("raw Codex transcript escaped the harness boundary: %#v", event)
		}
	}
	var result map[string]any
	if err := json.Unmarshal(events[len(events)-1].Data, &result); err != nil || result["schema_version"] != "1.0" {
		t.Fatalf("structured result was not forwarded: data=%s err=%v", events[len(events)-1].Data, err)
	}
	status, err := harness.Inspect(t.Context(), ref)
	if err != nil || status.State != "completed" {
		t.Fatalf("unexpected completed status: %#v err=%v", status, err)
	}

	invocations := readCodexHarnessInvocations(t, argsPath)
	if len(invocations) != 1 || containsArg(invocations[0], "--ephemeral") || !containsArg(invocations[0], "--json") || !containsArg(invocations[0], "--approve-for-me") {
		t.Fatalf("Codex start did not use the durable JSONL protocol: %#v", invocations)
	}
}

func TestCodexHarnessResumesThreadAcrossAdapterInstances(t *testing.T) {
	threadID := "019c-test-resumable-thread"
	argsPath := filepath.Join(t.TempDir(), "args.jsonl")
	workspace := writeCodexHarnessWorkspace(t)
	first := testCodexExecHarness(threadID, "success", argsPath)
	ref, stream, err := first.Start(t.Context(), StartAgentRequest{TenantID: "tenant-1", NodeRunID: "node-1", AttemptID: "attempt-1", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	_ = collectHarnessEvents(t, stream)

	second := testCodexExecHarness(threadID, "success", argsPath)
	resumed, err := second.Resume(t.Context(), ResumeAgentRequest{TenantID: "tenant-1", Session: ref, Workspace: workspace, Prompt: "继续"})
	if err != nil {
		t.Fatal(err)
	}
	events := collectHarnessEvents(t, resumed)
	if len(events) == 0 || events[0].Type != "session.resumed" || events[len(events)-1].Type != "result.completed" {
		t.Fatalf("unexpected resumed events: %#v", events)
	}
	invocations := readCodexHarnessInvocations(t, argsPath)
	if len(invocations) != 2 || !adjacentArgs(invocations[1], "resume", threadID) || containsArg(invocations[1], "--ephemeral") {
		t.Fatalf("Codex resume arguments did not preserve the thread: %#v", invocations)
	}
}

func TestCodexHarnessRegistersAttemptScopedRuntimeMCPBeforeResume(t *testing.T) {
	threadID := "019c-test-runtime-gateway-thread"
	argsPath := filepath.Join(t.TempDir(), "args.jsonl")
	workspace := writeCodexHarnessWorkspace(t)
	gateway := RuntimeGatewayConfig{URL: "https://content.example/api/v1/runtime/mcp/call", Token: "rtg_codex", AllowedTools: []string{"runtime.state.query"}}
	harness := testCodexExecHarness(threadID, "success", argsPath)
	ref, stream, err := harness.Start(t.Context(), StartAgentRequest{TenantID: "tenant-1", NodeRunID: "node-1", AttemptID: "attempt-1", Workspace: workspace, RuntimeGateway: gateway})
	if err != nil {
		t.Fatal(err)
	}
	_ = collectHarnessEvents(t, stream)
	harness = testCodexExecHarness(threadID, "success", argsPath)
	resumed, err := harness.Resume(t.Context(), ResumeAgentRequest{TenantID: "tenant-1", Session: ref, Workspace: workspace, RuntimeGateway: gateway})
	if err != nil {
		t.Fatal(err)
	}
	_ = collectHarnessEvents(t, resumed)
	invocations := readCodexHarnessInvocations(t, argsPath)
	if len(invocations) != 2 {
		t.Fatalf("Codex invocation count = %d", len(invocations))
	}
	for _, args := range invocations {
		joined := strings.Join(args, "\n")
		for _, required := range []string{"mcp_servers.contentcloud-runtime.command=", `mcp_servers.contentcloud-runtime.args=["mcp","runtime-serve"]`, "CONTENTCLOUD_RUNTIME_GATEWAY_TOKEN"} {
			if !strings.Contains(joined, required) {
				t.Fatalf("Codex Runtime MCP argument missing %q: %#v", required, args)
			}
		}
	}
	resumeIndex := indexOfArg(invocations[1], "resume")
	lastConfigIndex := -1
	for index, arg := range invocations[1] {
		if arg == "-c" {
			lastConfigIndex = index
		}
	}
	if resumeIndex < 0 || lastConfigIndex < 0 || lastConfigIndex > resumeIndex || !adjacentArgs(invocations[1], "resume", threadID) {
		t.Fatalf("Codex resume must receive Runtime MCP config as exec-level flags: %#v", invocations[1])
	}
}

func TestCodexHarnessFailsClosedOnTenantMismatchAndHostFailure(t *testing.T) {
	workspace := writeCodexHarnessWorkspace(t)
	threadID := "019c-test-failed-thread"
	harness := testCodexExecHarness(threadID, "turn_failed", filepath.Join(t.TempDir(), "args.jsonl"))
	if _, err := harness.Resume(t.Context(), ResumeAgentRequest{TenantID: "tenant-2", Session: AgentSessionRef{TenantID: "tenant-1", HarnessKind: "codex", SessionID: threadID}, Workspace: workspace}); !containsDomainCode(err, "AGENT_SESSION_TENANT_MISMATCH") {
		t.Fatalf("cross-tenant resume was not rejected: %v", err)
	}
	ref, stream, err := harness.Start(t.Context(), StartAgentRequest{TenantID: "tenant-1", NodeRunID: "node-1", AttemptID: "attempt-1", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	events := collectHarnessEvents(t, stream)
	last := events[len(events)-1]
	if last.Type != "session.failed" || last.ErrorCode != "CODEX_TURN_FAILED" || strings.Contains(string(last.Data), "provider secret") {
		t.Fatalf("Codex failure was not reduced to a stable safe event: %#v", last)
	}
	status, err := harness.Inspect(t.Context(), ref)
	if err != nil || status.State != "failed" || status.ErrorCode != "CODEX_TURN_FAILED" {
		t.Fatalf("unexpected failed status: %#v err=%v", status, err)
	}
}

func TestCodexStructuredFailureClassifiesAuthAndRateLimitWithoutMessage(t *testing.T) {
	for _, test := range []struct {
		message string
		want    string
	}{
		{message: `{"message":"HTTP 429 quota exceeded"}`, want: "CODEX_RATE_LIMITED"},
		{message: `{"message":"authentication failed"}`, want: "CODEX_AUTH_REQUIRED"},
	} {
		if got := codexFailureCode(codexJSONEvent{Type: "turn.failed", Error: json.RawMessage(test.message)}); got != test.want {
			t.Fatalf("codex structured failure %q = %q, want %q", test.message, got, test.want)
		}
	}
}

func TestCodexHarnessBoundsFirstStructuredEventHandshake(t *testing.T) {
	harness := testCodexExecHarness("019c-test-timeout-thread", "handshake_hang", filepath.Join(t.TempDir(), "args.jsonl"))
	harness.handshakeTimeout = 50 * time.Millisecond
	started := time.Now()
	_, _, err := harness.Start(t.Context(), StartAgentRequest{TenantID: "tenant-1", NodeRunID: "node-1", AttemptID: "attempt-1", Workspace: writeCodexHarnessWorkspace(t)})
	if !containsDomainCode(err, "CODEX_HANDSHAKE_TIMEOUT") {
		t.Fatalf("Codex handshake timeout = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("Codex handshake cleanup took %s", elapsed)
	}
}

func testCodexExecHarness(threadID, mode, argsPath string) *codexExecHarness {
	return &codexExecHarness{
		binary: os.Args[0], prefixArgs: []string{"-test.run=TestCodexHarnessHelperProcess", "--"},
		extraEnv: []string{"CODEX_HARNESS_HELPER=1", "CODEX_HARNESS_THREAD_ID=" + threadID, "CODEX_HARNESS_MODE=" + mode, "CODEX_HARNESS_ARGS_PATH=" + argsPath},
		detect:   func(context.Context) (string, error) { return "test", nil }, sessions: map[string]*codexExecSession{},
	}
}

func writeCodexHarnessWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "attempt")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	contract := sourcedomain.TaskContract{
		ContractVersion: "1.0", ContractID: "contract-1", RunID: "run-1", TaskType: "test",
		Project: workspacedomain.Project{ID: "project-1"}, Capability: catalogdomain.Capability{ID: "capability-1"},
	}
	contractBody, _ := json.Marshal(contract)
	for name, body := range map[string][]byte{
		"contract.json": contractBody, "output.schema.json": []byte(`{"type":"object"}`), "SKILL.md": []byte("# Test\n"),
	} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o400); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func collectHarnessEvents(t *testing.T, stream EventStream) []AgentEvent {
	t.Helper()
	defer stream.Close()
	result := []AgentEvent{}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event, ok := <-stream.Events():
			if !ok {
				return result
			}
			result = append(result, event)
		case <-deadline.C:
			t.Fatalf("timed out waiting for Codex Harness events: %#v", result)
		}
	}
}

func readCodexHarnessInvocations(t *testing.T, path string) [][]string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := [][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		var args []string
		if err := json.Unmarshal([]byte(line), &args); err != nil {
			t.Fatal(err)
		}
		result = append(result, args)
	}
	return result
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func adjacentArgs(args []string, first, second string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return true
		}
	}
	return false
}

func TestCodexHarnessHelperProcess(t *testing.T) {
	if os.Getenv("CODEX_HARNESS_HELPER") != "1" {
		return
	}
	args := os.Args
	if separator := indexOfArg(args, "--"); separator >= 0 {
		args = args[separator+1:]
	}
	if path := os.Getenv("CODEX_HARNESS_ARGS_PATH"); path != "" {
		body, _ := json.Marshal(args)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			os.Exit(21)
		}
		_, _ = file.Write(append(body, '\n'))
		_ = file.Close()
	}
	threadID := os.Getenv("CODEX_HARNESS_THREAD_ID")
	if os.Getenv("CODEX_HARNESS_MODE") == "handshake_hang" {
		time.Sleep(30 * time.Second)
		os.Exit(22)
	}
	_, _ = os.Stdout.WriteString(`{"type":"thread.started","thread_id":"` + threadID + `"}` + "\n")
	_, _ = os.Stdout.WriteString(`{"type":"turn.started"}` + "\n")
	if os.Getenv("CODEX_HARNESS_MODE") == "turn_failed" {
		_, _ = os.Stdout.WriteString(`{"type":"turn.failed","error":{"message":"provider secret"}}` + "\n")
		os.Exit(0)
	}
	_, _ = os.Stdout.WriteString(`{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"private transcript"}}` + "\n")
	_, _ = os.Stdout.WriteString(`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":2,"cache_write_input_tokens":1,"output_tokens":4,"reasoning_output_tokens":1}}` + "\n")
	if outputPath := argValue(args, "--output-last-message"); outputPath != "" {
		_ = os.WriteFile(outputPath, []byte(`{"schema_version":"1.0","ok":true}`), 0o600)
	}
	os.Exit(0)
}

func indexOfArg(args []string, target string) int {
	for index, arg := range args {
		if arg == target {
			return index
		}
	}
	return -1
}

func argValue(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}
