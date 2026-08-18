package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
)

func TestNewRuntimeMCPClientFailsClosedWithoutValidToolAuthorization(t *testing.T) {
	t.Setenv("CONTENTCLOUD_RUNTIME_GATEWAY_URL", "http://127.0.0.1/runtime/mcp/call")
	t.Setenv("CONTENTCLOUD_RUNTIME_GATEWAY_TOKEN", "rtg_test")
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "missing"},
		{name: "invalid JSON", value: "not-json"},
		{name: "JSON null", value: "null"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CONTENTCLOUD_RUNTIME_GATEWAY_TOOLS", test.value)
			if _, err := newRuntimeMCPClient(); err == nil {
				t.Fatal("expected invalid Runtime Gateway tool authorization to fail closed")
			}
		})
	}
}

func TestRuntimeMCPClientEmptyAuthorizationExposesAndCallsNoTools(t *testing.T) {
	t.Setenv("CONTENTCLOUD_RUNTIME_GATEWAY_URL", "http://127.0.0.1/runtime/mcp/call")
	t.Setenv("CONTENTCLOUD_RUNTIME_GATEWAY_TOKEN", "rtg_test")
	t.Setenv("CONTENTCLOUD_RUNTIME_GATEWAY_TOOLS", "[]")
	client, err := newRuntimeMCPClient()
	if err != nil {
		t.Fatal(err)
	}
	if tools := client.tools(); len(tools) != 0 {
		t.Fatalf("empty authorization exposed tools: %#v", tools)
	}
	if _, err := client.call(context.Background(), contentruntime.ToolChildList, nil); err == nil {
		t.Fatal("empty authorization allowed a Runtime MCP call")
	}
}

func TestRuntimeMCPStdioProcessCallsAttemptHTTPGateway(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	started, err := service.Runtime.Runtime().Start(t.Context(), contentruntime.StartInput{
		TenantID: "tenant-mcp-stdio", ProjectID: "project-mcp-stdio", WorkTaskID: "mcp-stdio-task", BusinessType: "mcp.stdio.test",
		SOP: runtimeMCPTestSOP(), BindingDigest: "sha256:" + strings.Repeat("a", 64), InputDigest: "sha256:" + strings.Repeat("b", 64),
		RuntimePolicyID: "runtime-policy/mcp-stdio", ContractMajor: 1, CreatedBy: "user-mcp-stdio", IdempotencyKey: "mcp-stdio-job",
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := service.Runtime.Runtime().PrepareRemoteDispatch(t.Context(), contentruntime.DispatchInput{
		TenantID: started.Job.TenantID, JobRunID: started.Job.ID, Owner: "device:mcp-stdio", HarnessKind: "fake", Role: "writer",
		ExecutionProfileID: "profile-mcp-stdio", AllowedTools: []string{contentruntime.ToolChildList}, MaxTokens: 1024, BudgetMinor: 10,
		RemainingDescendants: 1, LeaseFor: time.Minute,
	}, agentadapter.HarnessCapabilities{Kind: "fake", Events: true, Resume: true, MCPStdio: true, StructuredOutput: true, MaxParallelSessions: 1})
	if err != nil {
		t.Fatal(err)
	}
	handle, err = service.Runtime.Runtime().ActivateDispatch(t.Context(), handle, agentadapter.AgentSessionRef{TenantID: started.Job.TenantID, HarnessKind: "fake", SessionID: "mcp-stdio-session"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()

	requests := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"child.list","arguments":{}}}`,
	}, "\n") + "\n"
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=TestRuntimeMCPCLIHelperProcess", "--")
	command.Env = runtimeMCPHelperEnvironment(map[string]string{
		"CONTENTCLOUD_RUNTIME_MCP_HELPER":    "1",
		"CONTENTCLOUD_RUNTIME_GATEWAY_URL":   server.URL + "/api/v1/runtime/mcp/call",
		"CONTENTCLOUD_RUNTIME_GATEWAY_TOKEN": handle.GatewayToken,
		"CONTENTCLOUD_RUNTIME_GATEWAY_TOOLS": `["child.list"]`,
	})
	command.Stdin = strings.NewReader(requests)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("Runtime MCP stdio process failed: %v stderr=%s", err, stderr.String())
	}
	decoder := json.NewDecoder(&stdout)
	responses := make([]mcpResponse, 0, 3)
	for decoder.More() {
		var response mcpResponse
		if err := decoder.Decode(&response); err != nil {
			t.Fatalf("decode Runtime MCP stdio response: %v output=%s", err, stdout.String())
		}
		responses = append(responses, response)
	}
	if len(responses) != 3 || responses[0].Error != nil || responses[1].Error != nil || responses[2].Error != nil {
		t.Fatalf("unexpected Runtime MCP responses: %#v stderr=%s", responses, stderr.String())
	}
	list, ok := responses[1].Result.(map[string]any)
	if !ok {
		t.Fatalf("tools/list result type = %T", responses[1].Result)
	}
	tools, _ := list["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("Attempt tool allowlist was not enforced by stdio process: %#v", list)
	}
	call, ok := responses[2].Result.(map[string]any)
	if !ok || call["isError"] != false {
		t.Fatalf("tools/call result = %#v", responses[2].Result)
	}
	content, _ := call["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("tools/call content = %#v", call)
	}
	item, _ := content[0].(map[string]any)
	var gateway contentruntime.GatewayResponse
	if err := json.Unmarshal([]byte(fmt.Sprint(item["text"])), &gateway); err != nil || gateway.ToolCall.State != contentruntime.ToolCallSucceeded {
		t.Fatalf("stdio process did not return the HTTP Gateway result: gateway=%#v err=%v", gateway, err)
	}
}

func TestRuntimeMCPCLIHelperProcess(t *testing.T) {
	if os.Getenv("CONTENTCLOUD_RUNTIME_MCP_HELPER") != "1" {
		return
	}
	root := &Root{stdout: os.Stdout, stderr: os.Stderr}
	command := root.command()
	command.SetArgs([]string{"mcp", "runtime-serve"})
	command.SetIn(os.Stdin)
	if err := command.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func runtimeMCPHelperEnvironment(values map[string]string) []string {
	blocked := map[string]struct{}{}
	for key := range values {
		blocked[key] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+len(values))
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if _, exists := blocked[key]; !exists {
			environment = append(environment, value)
		}
	}
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func runtimeMCPTestSOP() catalogdomain.SOPVersion {
	return catalogdomain.SOPVersion{ID: "mcp-stdio-sop-v1", TenantID: "tenant-mcp-stdio", SOPID: "mcp-stdio-sop", Version: 1, SchemaVersion: catalogdomain.SOPSchemaVersion, Name: "MCP stdio", Status: "published", DefaultExecutionMode: "agent", Stages: []catalogdomain.StageDefinition{{ID: "write", Name: "Write", Order: 10, OutputSchema: "contentcloud.mcp-stdio/1.0", ExecutionModes: []string{"agent"}}}}
}
