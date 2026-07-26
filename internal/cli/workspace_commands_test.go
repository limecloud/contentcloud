package cli

import (
	"bytes"
	"encoding/json"
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
}

func TestMCPListsAndCallsWorkspaceTools(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", ServerURL: "http://localhost:8080", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	r := &Root{}
	list := r.handleMCPRequest(mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/list"})
	if list.Error != nil {
		t.Fatalf("list tools failed: %+v", list.Error)
	}
	params, _ := json.Marshal(map[string]any{"name": "workspace_status", "arguments": map[string]any{"directory": root}})
	call := r.handleMCPRequest(mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "tools/call", Params: params})
	if call.Error != nil {
		t.Fatalf("call tool failed: %+v", call.Error)
	}
	result, ok := call.Result.(map[string]any)
	if !ok || result["isError"] != false {
		t.Fatalf("unexpected tool result: %#v", call.Result)
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
