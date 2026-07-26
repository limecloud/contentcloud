package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/limecloud/contentcloud/internal/localworkspace"
)

func TestInitDryRunDoesNotConsumeCodeOrWriteFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "customer-project")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	var stdout, stderr bytes.Buffer
	command := (&Root{stdout: &stdout, stderr: &stderr}).command()
	command.SetArgs([]string{"--json", "init", "--connect", "cck_not_consumed", "--target", "codex", "--dry-run", root})
	if err := command.Execute(); err != nil {
		t.Fatalf("init dry-run failed: %v; stderr=%s", err, stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			DryRun bool                    `json:"dry_run"`
			Plan   localworkspace.InitPlan `json:"plan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v; output=%s", err, stdout.String())
	}
	if !envelope.OK || !envelope.Data.DryRun || envelope.Data.Plan.WouldUpload || envelope.Data.Plan.WouldDaemon {
		t.Fatalf("unexpected dry-run: %s", stdout.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("dry-run created target directory: %v", err)
	}
}

func TestWorkspaceCommandsAndMCPUseLocalState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", ServerURL: "http://localhost:8080", CLIVersion: "test", Target: "codex"}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--json", "workspace", "status", root},
		{"--json", "workspace", "doctor", "--offline", root},
		{"--json", "mcp", "status", root},
	} {
		var stdout, stderr bytes.Buffer
		command := (&Root{stdout: &stdout, stderr: &stderr}).command()
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("command %v failed: %v; stderr=%s", args, err, stderr.String())
		}
		var envelope struct {
			OK bool `json:"ok"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK {
			t.Fatalf("unexpected output for %v: %v %s", args, err, stdout.String())
		}
	}
	for _, name := range []string{"init", "workspace.status", "workspace.doctor", "mcp.status", "mcp.serve"} {
		if commandSchemas()[name] == nil {
			t.Fatalf("command schema %q is missing", name)
		}
	}
	for _, name := range []string{"pull.feedback", "pull.decisions", "pull.approved", "submission.list", "submission.show", "submission.status"} {
		schema, ok := commandSchemas()[name].(map[string]any)
		if !ok || schema["auth"] != "workspace" {
			t.Fatalf("command schema %q must require workspace auth: %#v", name, schema)
		}
	}
}

func TestMCPListsAndCallsWorkspaceTools(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", ServerURL: "http://localhost:8080", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	r := &Root{}
	list := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/list"})
	if list.Error != nil {
		t.Fatalf("list tools failed: %+v", list.Error)
	}
	listed, ok := list.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected tool list: %#v", list.Result)
	}
	tools, ok := listed["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("unexpected tools payload: %#v", listed)
	}
	names := map[string]bool{}
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		names[name] = true
	}
	for _, name := range []string{"workspace_status", "workspace_doctor", "publish_preflight", "submission_status", "review_feedback_list", "approved_snapshot_list"} {
		if !names[name] {
			t.Fatalf("MCP tool %q is missing: %#v", name, tools)
		}
	}
	params, _ := json.Marshal(map[string]any{"name": "workspace_status", "arguments": map[string]any{"directory": root}})
	call := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "tools/call", Params: params})
	if call.Error != nil {
		t.Fatalf("call tool failed: %+v", call.Error)
	}
	result, ok := call.Result.(map[string]any)
	if !ok || result["isError"] != false {
		t.Fatalf("unexpected tool result: %#v", call.Result)
	}
	pack := filepath.Join(root, "knowledge", "packs", "mcp-knowledge.json")
	if err := os.WriteFile(pack, []byte(`[{"id":"fact-1","kind":"fact","status":"verified"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	params, _ = json.Marshal(map[string]any{"name": "publish_preflight", "arguments": map[string]any{"directory": root, "submission_type": "knowledge"}})
	call = r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "tools/call", Params: params})
	result, ok = call.Result.(map[string]any)
	if call.Error != nil || !ok || result["isError"] != false {
		t.Fatalf("publish preflight tool failed: error=%+v result=%#v", call.Error, call.Result)
	}
}

func TestPublishKnowledgeDryRunNeedsNoWorkspaceCredential(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: "http://localhost:8080", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	pack := filepath.Join(root, "knowledge", "packs", "knowledge.json")
	if err := os.WriteFile(pack, []byte(`[{"id":"fact-1","kind":"fact","status":"verified"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	var stdout, stderr bytes.Buffer
	command := (&Root{stdout: &stdout, stderr: &stderr}).command()
	command.SetArgs([]string{"--json", "publish", "knowledge", "--dry-run"})
	if err := command.Execute(); err != nil {
		t.Fatalf("publish dry-run failed: %v; stderr=%s", err, stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			DryRun    bool `json:"dry_run"`
			Preflight struct {
				ObjectCount    int  `json:"object_count"`
				RawFilesUpload bool `json:"raw_files_upload"`
			} `json:"preflight"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || !envelope.Data.DryRun || envelope.Data.Preflight.ObjectCount != 1 || envelope.Data.Preflight.RawFilesUpload {
		t.Fatalf("unexpected publish preflight: %v %s", err, stdout.String())
	}
}

func TestMCPCloudReadsUseWorkspaceCredentialAndCLIDispatch(t *testing.T) {
	commands := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/cli/dispatch" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer wt_test_workspace" {
			t.Fatalf("workspace credential missing: %q", request.Header.Get("Authorization"))
		}
		var payload struct {
			Command string `json:"command"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, payload.Command)
		var data any
		switch payload.Command {
		case "submission.workspace-show":
			data = map[string]any{"submission": map[string]any{"id": "submission-1", "status": "submitted", "current_revision_id": "revision-1"}, "revisions": []map[string]any{{"id": "revision-1"}}, "comments": []any{}}
		case "feedback.workspace-list":
			data = []map[string]any{{"bundle_version": "1.0", "submission_id": "submission-1", "submission_revision_id": "revision-1", "subject_hash": "abc", "comments": []any{}}}
		case "snapshot.workspace-list":
			data = []map[string]any{{"id": "snapshot-1", "submission_type": "knowledge"}}
		default:
			t.Fatalf("unexpected CLI dispatch command: %s", payload.Command)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"ok": true, "command": payload.Command, "data": data}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: server.URL, CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_test_workspace")
	r := &Root{}
	for index, input := range []map[string]any{
		{"name": "submission_status", "arguments": map[string]any{"directory": root, "submission_id": "submission-1"}},
		{"name": "review_feedback_list", "arguments": map[string]any{"directory": root}},
		{"name": "approved_snapshot_list", "arguments": map[string]any{"directory": root, "submission_type": "knowledge"}},
	} {
		params, _ := json.Marshal(input)
		response := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params})
		result, ok := response.Result.(map[string]any)
		if response.Error != nil || !ok || result["isError"] != false {
			t.Fatalf("MCP cloud read %d failed: error=%+v result=%#v", index, response.Error, response.Result)
		}
	}
	expected := []string{"submission.workspace-show", "feedback.workspace-list", "snapshot.workspace-list"}
	if len(commands) != len(expected) {
		t.Fatalf("unexpected dispatch commands: %#v", commands)
	}
	for index := range expected {
		if commands[index] != expected[index] {
			t.Fatalf("unexpected dispatch commands: %#v", commands)
		}
	}
}
