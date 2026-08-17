package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/integration/pluginbuiltin"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
	"github.com/limecloud/contentcloud/internal/integration/pluginidentity"
	"github.com/limecloud/contentcloud/internal/localworkspace"
	"github.com/limecloud/contentcloud/internal/workbench"
)

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
	for _, name := range []string{"workspace.status", "workspace.doctor", "workspace.conversation-context", "workspace.project-brief.save", "workspace.approved.list", "workspace.approved.show", "mcp.status", "mcp.serve"} {
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

func TestWorkspaceProjectBriefCommandAdvancesBusinessFlow(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := (&Root{stdout: &stdout, stderr: &stderr}).command()
	command.SetArgs([]string{
		"--json", "workspace", "project-brief", "save", "--directory", root,
		"--client", "客户 A", "--brand", "品牌 A", "--product-or-service", "产品 A",
		"--objective", "验证内容方向", "--channel", "抖音", "--audience", "新客户",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("project brief command failed: %v; stderr=%s", err, stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Onboarding localworkspace.WorkspaceOnboarding `json:"onboarding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || envelope.Data.Onboarding.State != localworkspace.OnboardingNeedsSourceIntake {
		t.Fatalf("unexpected project brief command response: err=%v output=%s", err, stdout.String())
	}
}

func TestWorkspaceMemoryCommandsRebuildQueryStatusAndClearDryRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-memory-cli", ProjectID: "project-memory-cli", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "10-context", "project.yaml"), []byte("客户需要提高年轻白领的复购。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		var stdout, stderr bytes.Buffer
		command := (&Root{stdout: &stdout, stderr: &stderr, now: func() time.Time { return time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC) }}).command()
		command.SetArgs(append([]string{"--json"}, args...))
		if err := command.Execute(); err != nil {
			t.Fatalf("command %v failed: %v; stderr=%s", args, err, stderr.String())
		}
		return stdout.String()
	}
	var rebuilt struct {
		OK   bool `json:"ok"`
		Data struct {
			State string `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(run("workspace", "memory", "rebuild", "--directory", root)), &rebuilt); err != nil || !rebuilt.OK || rebuilt.Data.State != localworkspace.MemoryStateReady {
		t.Fatalf("unexpected rebuild response: err=%v payload=%+v", err, rebuilt)
	}
	var queried struct {
		OK   bool `json:"ok"`
		Data struct {
			Backend    string `json:"backend"`
			Candidates []any  `json:"candidates"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(run("workspace", "memory", "query", "--directory", root, "--query", "年轻白领")), &queried); err != nil || !queried.OK || queried.Data.Backend != localworkspace.MemoryBackendSQLiteFTS5 || len(queried.Data.Candidates) == 0 {
		t.Fatalf("unexpected query response: err=%v payload=%+v", err, queried)
	}
	if payload := run("workspace", "memory", "status", "--directory", root); !strings.Contains(payload, `"state":"ready"`) {
		t.Fatalf("status response is not ready: %s", payload)
	}
	if payload := run("workspace", "memory", "clear", "--directory", root, "--dry-run"); !strings.Contains(payload, `"would_clear":true`) {
		t.Fatalf("clear dry-run did not report cache: %s", payload)
	}
}

func TestWorkspaceMemoryRememberCommandBindsLocalSource(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-memory-remember", ProjectID: "project-memory-remember", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "40-work", "focus.md"), []byte("当前焦点：验证候选受众反馈。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := (&Root{stdout: &stdout, stderr: &stderr, now: func() time.Time { return time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC) }}).command()
	command.SetArgs([]string{"--json", "workspace", "memory", "remember", "--directory", root, "--id", "memr_cli", "--kind", "working", "--source-ref", "40-work/focus.md", "--summary", "当前任务优先验证候选受众反馈。"})
	if err := command.Execute(); err != nil {
		t.Fatalf("memory remember command failed: %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"memory_id":"memr_cli"`) || !strings.Contains(stdout.String(), `"record_ref":"40-work/memory/records/memr_cli.json"`) {
		t.Fatalf("unexpected memory remember response: %s", stdout.String())
	}
}

func TestWorkspaceFixtureApplyMaterializesExternalPackage(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "jinling-gudu")
	fixture := filepath.Join("..", "..", "fixtures", "v3", "jinling-gudu.json")
	var stdout, stderr bytes.Buffer
	command := (&Root{stdout: &stdout, stderr: &stderr}).command()
	command.SetArgs([]string{"--json", "workspace", "fixture", "apply", fixture, "--directory", directory, "--project-id", "project-fixture", "--workspace-id", "workspace-fixture", "--target", "none"})
	if err := command.Execute(); err != nil {
		t.Fatalf("fixture apply failed: %v; stderr=%s", err, stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Workspace struct {
				SourceCount int `json:"source_count"`
			} `json:"workspace"`
			ContentBatch struct {
				Batch struct {
					Status string `json:"status"`
				} `json:"batch"`
			} `json:"content_batch"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || envelope.Data.Workspace.SourceCount != 20 || envelope.Data.ContentBatch.Batch.Status != "blocked" {
		t.Fatalf("unexpected fixture apply envelope: %v %s", err, stdout.String())
	}
	if commandSchemas()["workspace.fixture.apply"] == nil {
		t.Fatal("workspace.fixture.apply is missing from CLI contract")
	}
}

func TestMCPListsAndCallsWorkspaceTools(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", ServerURL: "http://localhost:8080", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	r := &Root{mcpCWD: root, now: func() time.Time { return now }}
	initializeParams, _ := json.Marshal(map[string]any{"protocolVersion": "2025-06-18"})
	initialized := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("0"), Method: "initialize", Params: initializeParams})
	initializeResult, ok := initialized.Result.(map[string]any)
	if initialized.Error != nil || !ok || initializeResult["protocolVersion"] != "2025-06-18" || initializeResult["instructions"] == "" {
		t.Fatalf("unexpected initialize response: %#v", initialized)
	}
	capabilities, _ := initializeResult["capabilities"].(map[string]any)
	if capabilities["resources"] == nil || capabilities["tools"] == nil {
		t.Fatalf("initialize capabilities missing tools/resources: %#v", capabilities)
	}
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
	for _, name := range []string{"contentcloud_open_studio_view", "workspace_context", "workspace_view", "workspace_open_workbench", "workspace_workbench_status", "workspace_close_workbench", "workspace_proposal_prepare", "workspace_proposal_apply", "memory_status", "memory_rebuild", "memory_remember", "memory_consolidate", "memory_promote", "memory_extract", "memory_remote_query", "memory_query", "workspace_project_brief", "environment_execution_plan", "environment_prepare_plan", "environment_prepare_apply", "workspace_status", "workspace_doctor", "source_register", "source_list", "source_ingest", "source_verify", "local_run_init", "local_run_show", "local_run_record", "local_run_check", "local_run_advance", "local_run_fail", "local_run_resume", "local_run_claim", "local_run_takeover", "local_run_renew", "local_run_release", "handoff_create_ready", "handoff_list_ready", "handoff_accept", "handoff_complete", "handoff_supersede", "knowledge_import", "knowledge_lint", "knowledge_query", "knowledge_diagnose", "knowledge_pack", "brief_lint", "content_batch_init", "content_item_lint", "content_batch_lint", "content_batch_finalize", "content_item_diff", "delivery_export", "article_brief_lint", "article_batch_create", "article_item_lint", "article_batch_lint", "article_batch_finalize", "article_item_diff", "wechat_package_export", "wechat_package_lint", "publish_preflight", "publish_apply", "submission_status", "review_feedback_list", "review_feedback_pull", "review_feedback_inbox", "approved_snapshot_list", "approved_snapshot_pull", "approved_snapshot_inbox", "approved_snapshot_show"} {
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
	statusLink := mcpProjectViewURLForTest(t, result)
	if statusLink.Path != "/studio" || statusLink.Query().Get("project") != "project-1" {
		t.Fatalf("workspace_status returned an unexpected project link: %s", statusLink)
	}
	doctorResult := callMCPToolForTest(t, r, "workspace_doctor", map[string]any{})
	doctorLink := mcpProjectViewURLForTest(t, doctorResult)
	if doctorLink.Path != "/studio/connect" || doctorLink.Query().Get("project") != "project-1" {
		t.Fatalf("workspace_doctor returned an unexpected setup link: %s", doctorLink)
	}
	params, _ = json.Marshal(map[string]any{"name": "workspace_context", "arguments": map[string]any{}})
	contextCall := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "tools/call", Params: params})
	contextResult, ok := contextCall.Result.(map[string]any)
	if contextCall.Error != nil || !ok || contextResult["isError"] != false {
		t.Fatalf("conversation context tool failed: error=%+v result=%#v", contextCall.Error, contextCall.Result)
	}
	resourceList := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("5"), Method: "resources/list"})
	resources, ok := resourceList.Result.(map[string]any)
	if resourceList.Error != nil || !ok || len(resources["resources"].([]map[string]any)) != 2 {
		t.Fatalf("unexpected resource list: %#v", resourceList)
	}
	templateList := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("5.1"), Method: "resources/templates/list"})
	templates, ok := templateList.Result.(map[string]any)
	if templateList.Error != nil || !ok || len(templates["resourceTemplates"].([]map[string]any)) != 1 {
		t.Fatalf("unexpected resource template list: %#v", templateList)
	}
	for _, template := range templates["resourceTemplates"].([]map[string]any) {
		uri, _ := template["uriTemplate"].(string)
		if !strings.Contains(uri, "digest={sha256}") {
			t.Fatalf("resource template is not digest-bound: %#v", template)
		}
	}
	resourceParams, _ := json.Marshal(map[string]any{"uri": "contentcloud://workspace/conversation-context"})
	resourceRead := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("6"), Method: "resources/read", Params: resourceParams})
	resourceResult, ok := resourceRead.Result.(map[string]any)
	if resourceRead.Error != nil || !ok {
		t.Fatalf("conversation context resource failed: %#v", resourceRead)
	}
	contents := resourceResult["contents"].([]map[string]string)
	toolJSON, err := json.Marshal(contextResult["structuredContent"])
	if err != nil {
		t.Fatal(err)
	}
	var toolValue, resourceValue any
	if err := json.Unmarshal(toolJSON, &toolValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(contents[0]["text"]), &resourceValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(toolValue, resourceValue) {
		t.Fatalf("tool/resource schema drift:\ntool=%s\nresource=%s", toolJSON, contents[0]["text"])
	}
	briefResult := callMCPToolForTest(t, r, "workspace_project_brief", map[string]any{
		"client": "客户 A", "brand": "品牌 A", "product_or_service": "产品 A", "objective": "验证内容方向",
		"channels": []string{"抖音"}, "audience": "新客户", "confirm": true,
	})
	briefValue, ok := briefResult["structuredContent"].(map[string]any)
	if !ok || briefResult["isError"] != false || briefValue["business_files_modified"] != true {
		t.Fatalf("project brief tool failed: %#v", briefResult)
	}
	onboarding, ok := briefValue["onboarding"].(localworkspace.WorkspaceOnboarding)
	if !ok || onboarding.State != localworkspace.OnboardingNeedsSourceIntake || onboarding.NextStep.ID != "source_intake" {
		t.Fatalf("project brief tool did not advance onboarding: %#v", briefValue)
	}
	pack := filepath.Join(root, "30-knowledge", "packs", "mcp-knowledge.json")
	if err := os.WriteFile(pack, []byte(`[{"id":"fact-1","kind":"fact","status":"verified"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	params, _ = json.Marshal(map[string]any{"name": "publish_preflight", "arguments": map[string]any{"directory": root, "submission_type": "knowledge"}})
	call = r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "tools/call", Params: params})
	result, ok = call.Result.(map[string]any)
	if call.Error != nil || !ok || result["isError"] != false {
		t.Fatalf("publish preflight tool failed: error=%+v result=%#v", call.Error, call.Result)
	}
	basePath := filepath.Join(root, "40-work", "base-script.json")
	candidatePath := filepath.Join(root, "40-work", "candidate-script.json")
	if err := os.WriteFile(basePath, []byte(`{"id":"script-version:1","title":"原标题"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidatePath, []byte(`{"id":"script-version:2","based_on_version_id":"script-version:1","change_summary":"调整标题","title":"新标题"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	params, _ = json.Marshal(map[string]any{"name": "content_item_diff", "arguments": map[string]any{"directory": root, "baseline_file": "40-work/base-script.json", "candidate_file": "40-work/candidate-script.json", "allowed_paths": []string{"/title"}}})
	call = r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "tools/call", Params: params})
	result, ok = call.Result.(map[string]any)
	if call.Error != nil || !ok || result["isError"] != false {
		t.Fatalf("script diff MCP tool failed: error=%+v result=%#v", call.Error, call.Result)
	}
}

func TestMCPBindsExplicitWorkspaceForSubsequentResourceReads(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-bind", WorkspaceID: "workspace-bind", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	ref := "50-production/local.md"
	body := []byte("bound to the customer workspace\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), body, 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Root{mcpCWD: filepath.Join(t.TempDir(), "plugin-root"), now: func() time.Time { return time.Date(2026, 8, 14, 5, 0, 0, 0, time.UTC) }}
	viewResult := callMCPToolForTest(t, r, "workspace_view", map[string]any{"directory": root, "view": "file", "ref": ref})
	view := viewResult["structuredContent"].(localworkspace.WorkspaceView)
	params, _ := json.Marshal(map[string]any{"uri": view.Resources[0].URI})
	read := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "resources/read", Params: params})
	if read.Error != nil {
		t.Fatalf("bound resource read failed: %#v", read.Error)
	}
	otherRoot := filepath.Join(t.TempDir(), "other")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: otherRoot, ProjectID: "project-other", WorkspaceID: "workspace-other", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	_, err := r.resolveMCPWorkspace(otherRoot)
	if err == nil {
		t.Fatal("MCP session accepted a second workspace")
	}
	assertCLIErrorCode(t, err, "MCP_WORKSPACE_SESSION_CONFLICT")
}

func TestMCPLocalRunToolsDriveMarketingRunAndRecovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-mcp-marketing", ProjectID: "project-mcp-marketing", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 15, 14, 0, 0, 0, time.UTC)
	r := &Root{mcpCWD: root, now: func() time.Time { return now }}

	initialized := callMCPToolForTest(t, r, "local_run_init", map[string]any{"run_id": "run-marketing-mcp", "intent": "intent:content"})
	run := initialized["structuredContent"].(localworkspace.LocalRunContext)
	claimed := callMCPToolForTest(t, r, "local_run_claim", map[string]any{
		"run_id": run.RunID, "owner_kind": "agent", "owner_id": "marketing-mcp", "expected_revision": run.ContextRevision,
	})
	claim := claimed["structuredContent"].(localworkspace.RunClaim)

	checked := callMCPToolForTest(t, r, "local_run_check", map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "expected_revision": claim.ContextRevision,
		"name": "kb-lint", "status": "passed", "detail": "营销知识门禁通过",
	})
	run = checked["structuredContent"].(localworkspace.LocalRunContext)
	advanced := callMCPToolForTest(t, r, "local_run_advance", map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "expected_revision": run.ContextRevision, "stage": "query",
	})
	run = advanced["structuredContent"].(localworkspace.LocalRunContext)
	recorded := callMCPToolForTest(t, r, "local_run_record", map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "expected_revision": run.ContextRevision, "eligible_ids": []string{"knowledge:eligible-1"},
	})
	run = recorded["structuredContent"].(localworkspace.LocalRunContext)
	advanced = callMCPToolForTest(t, r, "local_run_advance", map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "expected_revision": run.ContextRevision, "stage": "compile",
	})
	run = advanced["structuredContent"].(localworkspace.LocalRunContext)
	recorded = callMCPToolForTest(t, r, "local_run_record", map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "expected_revision": run.ContextRevision, "output_paths": []string{"50-production/marketing-draft.json"},
	})
	run = recorded["structuredContent"].(localworkspace.LocalRunContext)
	advanced = callMCPToolForTest(t, r, "local_run_advance", map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "expected_revision": run.ContextRevision, "stage": "output-lint",
	})
	run = advanced["structuredContent"].(localworkspace.LocalRunContext)
	checked = callMCPToolForTest(t, r, "local_run_check", map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "expected_revision": run.ContextRevision,
		"name": "content-lint", "status": "passed",
	})
	run = checked["structuredContent"].(localworkspace.LocalRunContext)
	completed := callMCPToolForTest(t, r, "local_run_advance", map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "expected_revision": run.ContextRevision, "stage": "done",
	})
	run = completed["structuredContent"].(localworkspace.LocalRunContext)
	if run.Stage != "done" || run.Status != "completed" || len(run.EligibleIDs) != 1 || len(run.OutputPaths) != 1 {
		t.Fatalf("marketing MCP run did not complete: %#v", run)
	}

	failedInit := callMCPToolForTest(t, r, "local_run_init", map[string]any{"run_id": "run-marketing-recovery", "intent": "intent:content"})
	failedRun := failedInit["structuredContent"].(localworkspace.LocalRunContext)
	failedClaimResult := callMCPToolForTest(t, r, "local_run_claim", map[string]any{
		"run_id": failedRun.RunID, "owner_kind": "agent", "owner_id": "marketing-recovery", "expected_revision": failedRun.ContextRevision,
	})
	failedClaim := failedClaimResult["structuredContent"].(localworkspace.RunClaim)
	failed := callMCPToolForTest(t, r, "local_run_fail", map[string]any{
		"run_id": failedRun.RunID, "claim_token": failedClaim.Token, "expected_revision": failedClaim.ContextRevision, "findings": []string{"缺少品牌权利证明"},
	})
	failedRun = failed["structuredContent"].(localworkspace.LocalRunContext)
	resumed := callMCPToolForTest(t, r, "local_run_resume", map[string]any{
		"run_id": failedRun.RunID, "claim_token": failedClaim.Token, "expected_revision": failedRun.ContextRevision,
	})
	resumedRun := resumed["structuredContent"].(localworkspace.LocalRunContext)
	if resumedRun.Status != "active" || len(resumedRun.Findings) != 1 {
		t.Fatalf("marketing MCP recovery did not resume original run: %#v", resumedRun)
	}
}

func TestMCPAppsNegotiationExposesOnlyTheAppResource(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-app", WorkspaceID: "workspace-app", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	r := &Root{mcpCWD: root}

	initializeParams, _ := json.Marshal(map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities": map[string]any{
			"extensions": map[string]any{
				mcpAppsExtensionID: map[string]any{"mimeTypes": []string{mcpAppsMIMEType}},
			},
		},
	})
	initialized := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("0"), Method: "initialize", Params: initializeParams})
	if initialized.Error != nil || !r.mcpAppsEnabled() {
		t.Fatalf("MCP Apps capability was not negotiated: %#v", initialized)
	}

	toolList := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/list"})
	listed := toolList.Result.(map[string]any)
	var appTool map[string]any
	for _, tool := range listed["tools"].([]map[string]any) {
		if tool["name"] == "workspace_open_workbench" {
			appTool = tool
			break
		}
	}
	if appTool == nil {
		t.Fatal("workspace_open_workbench is missing")
	}
	meta, ok := appTool["_meta"].(map[string]any)
	uiMeta, uiOK := meta["ui"].(map[string]any)
	if !ok || !uiOK || uiMeta["resourceUri"] != mcpAppsResourceURI {
		t.Fatalf("MCP Apps metadata is missing: %#v", appTool)
	}

	resourceList := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "resources/list"})
	resources := resourceList.Result.(map[string]any)["resources"].([]map[string]any)
	if len(resources) != 3 {
		t.Fatalf("unexpected negotiated resource count: %#v", resources)
	}
	appResource := resources[2]
	if appResource["uri"] != mcpAppsResourceURI || appResource["mimeType"] != mcpAppsMIMEType {
		t.Fatalf("unexpected app resource listing: %#v", appResource)
	}

	params, _ := json.Marshal(map[string]any{"uri": mcpAppsResourceURI})
	read := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "resources/read", Params: params})
	if read.Error != nil {
		t.Fatalf("MCP App resource read failed: %#v", read.Error)
	}
	contents := read.Result.(map[string]any)["contents"].([]map[string]any)
	if len(contents) != 1 || contents[0]["mimeType"] != mcpAppsMIMEType {
		t.Fatalf("unexpected app resource content: %#v", contents)
	}
	html, ok := contents[0]["text"].(string)
	if !ok || !strings.Contains(html, "ui/initialize") || strings.Contains(html, root) {
		t.Fatalf("app resource is not self-contained: %q", html)
	}
}

func TestMCPAppsFallbackHidesMetadataAndRejectsAppResource(t *testing.T) {
	r := &Root{}
	initializeParams, _ := json.Marshal(map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{
		"extensions": map[string]any{mcpAppsExtensionID: map[string]any{"mimeTypes": []string{"text/html"}}},
	}})
	initialized := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("0"), Method: "initialize", Params: initializeParams})
	if initialized.Error != nil || r.mcpAppsEnabled() {
		t.Fatalf("unsupported host unexpectedly negotiated MCP Apps: %#v", initialized)
	}
	toolList := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/list"})
	for _, tool := range toolList.Result.(map[string]any)["tools"].([]map[string]any) {
		if tool["name"] == "workspace_open_workbench" && tool["_meta"] != nil {
			t.Fatalf("fallback tool leaked MCP Apps metadata: %#v", tool)
		}
	}
	resources := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "resources/list"}).Result.(map[string]any)["resources"].([]map[string]any)
	if len(resources) != 2 {
		t.Fatalf("fallback resource list changed: %#v", resources)
	}
	params, _ := json.Marshal(map[string]any{"uri": mcpAppsResourceURI})
	read := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "resources/read", Params: params})
	if read.Error == nil || read.Error.Code != -32001 || read.Error.Message != "当前 MCP 会话未协商 MCP Apps，不能读取本地工作台 App Resource" {
		t.Fatalf("unsupported host did not reject app resource: %#v", read)
	}
}

func TestMCPRootsRequestBindsSingleRootAndHandlesChangeNotification(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: workspaceRoot, ProjectID: "project-roots", WorkspaceID: "workspace-roots", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	pluginRoot := filepath.Join(t.TempDir(), "plugin")
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	r := &Root{mcpCWD: pluginRoot, stdout: outWriter}
	done := make(chan error, 1)
	go func() { done <- r.serveMCP(t.Context(), inReader); _ = outWriter.Close() }()

	write := func(value any) {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(inWriter, string(body)+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	out := bufio.NewReader(outReader)
	read := func() map[string]any {
		line, err := out.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var message map[string]any
		if err := json.Unmarshal(line, &message); err != nil {
			t.Fatal(err)
		}
		return message
	}
	write(map[string]any{"jsonrpc": "2.0", "id": "init", "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{"roots": map[string]any{"listChanged": true}},
	}})
	if message := read(); message["id"] != "init" {
		t.Fatalf("unexpected initialize response: %#v", message)
	}
	write(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	rootRequest := read()
	if rootRequest["method"] != "roots/list" {
		t.Fatalf("server did not request roots/list: %#v", rootRequest)
	}
	write(map[string]any{"jsonrpc": "2.0", "id": rootRequest["id"], "result": map[string]any{"roots": []map[string]any{{"uri": (&url.URL{Scheme: "file", Path: workspaceRoot}).String(), "name": "项目"}}}})
	write(map[string]any{"jsonrpc": "2.0", "id": "context", "method": "tools/call", "params": map[string]any{"name": "workspace_context", "arguments": map[string]any{}}})
	contextResponse := read()
	contextJSON, _ := json.Marshal(contextResponse)
	if contextResponse["id"] != "context" || !strings.Contains(string(contextJSON), "project-roots") {
		t.Fatalf("roots did not bind workspace context: %#v", contextResponse)
	}
	write(map[string]any{"jsonrpc": "2.0", "method": "notifications/roots/list_changed"})
	changedRequest := read()
	if changedRequest["method"] != "roots/list" || changedRequest["id"] == rootRequest["id"] {
		t.Fatalf("roots change did not create a new request: first=%#v second=%#v", rootRequest, changedRequest)
	}
	write(map[string]any{"jsonrpc": "2.0", "id": changedRequest["id"], "result": map[string]any{"roots": []map[string]any{{"uri": (&url.URL{Scheme: "file", Path: workspaceRoot}).String()}}}})
	if err := inWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestMCPRootsRequireExplicitSelectionForMultipleRoots(t *testing.T) {
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	for _, root := range []string{first, second} {
		if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: root, WorkspaceID: root, CLIVersion: "test", Target: "none"}); err != nil {
			t.Fatal(err)
		}
	}
	r := &Root{mcpCWD: filepath.Join(t.TempDir(), "plugin")}
	params, _ := json.Marshal(map[string]any{"roots": []map[string]any{
		{"uri": (&url.URL{Scheme: "file", Path: first}).String()},
		{"uri": (&url.URL{Scheme: "file", Path: second}).String()},
	}})
	r.applyMCPRoots(params)
	if _, err := r.resolveMCPWorkspace(""); err == nil {
		t.Fatal("multiple MCP roots were guessed without explicit selection")
	} else {
		assertCLIErrorCode(t, err, "MCP_ROOT_SELECTION_REQUIRED")
	}
	resolved, err := r.resolveMCPWorkspace(second)
	canonicalSecond, evalErr := filepath.EvalSymlinks(second)
	if err != nil || evalErr != nil || resolved != canonicalSecond {
		t.Fatalf("explicit MCP root selection failed: root=%q err=%v", resolved, err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: outside, ProjectID: "project-outside", WorkspaceID: "workspace-outside", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	outsideRoot := &Root{mcpCWD: filepath.Join(t.TempDir(), "plugin-outside")}
	outsideRoot.applyMCPRoots(params)
	if _, err := outsideRoot.resolveMCPWorkspace(outside); err == nil {
		t.Fatal("explicit directory outside MCP roots was accepted")
	} else {
		assertCLIErrorCode(t, err, "MCP_ROOT_OUTSIDE_DECLARED_ROOTS")
	}
}

func TestMCPOpenProjectViewReturnsTrustedResourceLink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: "https://content.example.com", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	r := &Root{mcpCWD: root}

	var definition map[string]any
	for _, tool := range mcpTools() {
		if tool["name"] == "contentcloud_open_studio_view" {
			definition = tool
			break
		}
	}
	if definition == nil {
		t.Fatal("contentcloud_open_studio_view definition is missing")
	}
	annotations := definition["annotations"].(map[string]any)
	if annotations["readOnlyHint"] != true || annotations["destructiveHint"] != false || annotations["idempotentHint"] != true || annotations["openWorldHint"] != true {
		t.Fatalf("unexpected navigation annotations: %#v", annotations)
	}
	schema := definition["inputSchema"].(map[string]any)
	properties := schema["properties"].(map[string]any)
	if properties["url"] != nil || properties["host"] != nil || schema["additionalProperties"] != false {
		t.Fatalf("navigation schema exposes an unsafe target: %#v", schema)
	}

	result := callMCPToolForTest(t, r, "contentcloud_open_studio_view", map[string]any{"view": "home"})
	contents, ok := result["content"].([]map[string]any)
	if !ok || len(contents) != 2 || contents[1]["type"] != "resource_link" || contents[1]["mimeType"] != "text/html" {
		t.Fatalf("unexpected resource link content: %#v", result["content"])
	}
	if contents[1]["uri"] != "https://content.example.com/studio?project=project-1" {
		t.Fatalf("unexpected project URL: %#v", contents[1])
	}
	structured, ok := result["structuredContent"].(mcpProjectViewResult)
	if !ok || structured.ProjectID != "project-1" || structured.View != "home" || structured.Focus != nil || structured.BrowserHandoff.URL != contents[1]["uri"] || !structured.BrowserHandoff.Required {
		t.Fatalf("unexpected structured navigation result: %#v", result["structuredContent"])
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), root) || strings.Contains(string(body), "workspace-1") {
		t.Fatalf("navigation result leaked local or workspace-only state: %s", body)
	}

	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	review := callMCPToolForTest(t, r, "contentcloud_open_studio_view", map[string]any{"view": "tasks", "focus": map[string]any{"kind": "submission_revision", "id": "revision-1", "digest": digest}})
	reviewContent := review["content"].([]map[string]any)
	parsed, err := url.Parse(reviewContent[1]["uri"].(string))
	if err != nil || parsed.Query().Get("focus_kind") != "submission_revision" || parsed.Query().Get("focus_id") != "revision-1" || parsed.Query().Get("expected_digest") != digest {
		t.Fatalf("unexpected focused review URL: %#v error=%v", reviewContent[1], err)
	}
}

func TestMCPOpenProjectViewRejectsUnsafeInputs(t *testing.T) {
	trustedRoot := filepath.Join(t.TempDir(), "trusted")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: trustedRoot, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: "https://content.example.com", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	r := &Root{mcpCWD: trustedRoot}
	tests := []struct {
		name      string
		arguments map[string]any
		code      string
	}{
		{name: "arbitrary url", arguments: map[string]any{"view": "home", "url": "https://evil.example"}, code: "MCP_PARAMS_INVALID"},
		{name: "arbitrary host", arguments: map[string]any{"view": "home", "host": "evil.example"}, code: "MCP_PARAMS_INVALID"},
		{name: "unknown view", arguments: map[string]any{"view": "unknown"}, code: "PROJECT_VIEW_INVALID"},
		{name: "wrong focus kind", arguments: map[string]any{"view": "tasks", "focus": map[string]any{"kind": "bootstrap_attempt", "id": "attempt-1"}}, code: "PROJECT_FOCUS_INVALID"},
		{name: "unknown focus field", arguments: map[string]any{"view": "connect", "focus": map[string]any{"kind": "environment_health", "id": "doctor-1", "path": "/tmp/private"}}, code: "MCP_PARAMS_INVALID"},
		{name: "revision digest required", arguments: map[string]any{"view": "tasks", "focus": map[string]any{"kind": "submission_revision", "id": "revision-1"}}, code: "PROJECT_FOCUS_DIGEST_REQUIRED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := callMCPToolExpectErrorForTest(t, r, "contentcloud_open_studio_view", test.arguments)
			if got := mcpToolErrorCodeForTest(t, result); got != test.code {
				t.Fatalf("unexpected error code: got=%q want=%q result=%#v", got, test.code, result)
			}
		})
	}

	untrustedRoot := filepath.Join(t.TempDir(), "untrusted")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: untrustedRoot, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: "http://content.example.com", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	result := callMCPToolExpectErrorForTest(t, &Root{mcpCWD: untrustedRoot}, "contentcloud_open_studio_view", map[string]any{"view": "home"})
	if got := mcpToolErrorCodeForTest(t, result); got != "WEB_TARGET_UNTRUSTED" {
		t.Fatalf("unexpected untrusted target error: %q %#v", got, result)
	}

	result = callMCPToolExpectErrorForTest(t, &Root{mcpCWD: t.TempDir()}, "contentcloud_open_studio_view", map[string]any{"view": "home"})
	if got := mcpToolErrorCodeForTest(t, result); got != "WORKSPACE_NOT_BOUND" {
		t.Fatalf("unexpected workspace resolution error: %q %#v", got, result)
	}
}

func TestMCPServeSerializesProjectViewResourceLink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: "https://content.example.com", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"contentcloud_open_studio_view","arguments":{"view":"home"}}}` + "\n"
	var output bytes.Buffer
	r := &Root{mcpCWD: root, stdout: &output}
	if err := r.serveMCP(t.Context(), strings.NewReader(request)); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result struct {
			Content []struct {
				Type     string `json:"type"`
				URI      string `json:"uri"`
				MimeType string `json:"mimeType"`
			} `json:"content"`
			StructuredContent struct {
				ProjectID      string `json:"project_id"`
				View           string `json:"view"`
				BrowserHandoff struct {
					URL           string `json:"url"`
					BrowserAction string `json:"browserAction"`
				} `json:"browserHandoff"`
			} `json:"structuredContent"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid MCP JSON response: %v output=%s", err, output.String())
	}
	if response.Result.IsError || len(response.Result.Content) != 2 || response.Result.Content[1].Type != "resource_link" || response.Result.Content[1].MimeType != "text/html" {
		t.Fatalf("unexpected serialized MCP content: %#v", response.Result)
	}
	link := response.Result.Content[1].URI
	if link != "https://content.example.com/studio?project=project-1" || response.Result.StructuredContent.ProjectID != "project-1" || response.Result.StructuredContent.View != "home" || response.Result.StructuredContent.BrowserHandoff.URL != link || response.Result.StructuredContent.BrowserHandoff.BrowserAction != "navigate" {
		t.Fatalf("unexpected serialized navigation contract: %#v", response.Result)
	}
	if strings.Contains(output.String(), root) || strings.Contains(output.String(), "workspace-1") {
		t.Fatalf("serialized MCP response leaked local state: %s", output.String())
	}
}

func TestMCPServeSerializesAttachedWorkspaceToolResourceLink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: "https://content.example.com", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workspace_status","arguments":{}}}` + "\n"
	var output bytes.Buffer
	if err := (&Root{mcpCWD: root, stdout: &output}).serveMCP(t.Context(), strings.NewReader(request)); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result struct {
			Content []struct {
				Type     string `json:"type"`
				URI      string `json:"uri"`
				MimeType string `json:"mimeType"`
			} `json:"content"`
			StructuredContent struct {
				Initialized bool `json:"initialized"`
				Binding     struct {
					ProjectID string `json:"project_id"`
				} `json:"binding"`
			} `json:"structuredContent"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid MCP JSON response: %v output=%s", err, output.String())
	}
	if response.Result.IsError || len(response.Result.Content) != 2 || response.Result.Content[1].Type != "resource_link" || response.Result.Content[1].MimeType != "text/html" {
		t.Fatalf("unexpected serialized workspace status content: %#v", response.Result)
	}
	if response.Result.Content[1].URI != "https://content.example.com/studio?project=project-1" || !response.Result.StructuredContent.Initialized || response.Result.StructuredContent.Binding.ProjectID != "project-1" {
		t.Fatalf("workspace status data or attached link changed during serialization: %#v", response.Result)
	}
}

func TestMCPServeUsesInjectedWorkspaceRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-injected", WorkspaceID: "workspace-injected", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(contentCloudWorkspaceRootEnvironment, root)
	request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workspace_context","arguments":{}}}` + "\n"
	var output bytes.Buffer
	r := &Root{stdout: &output}
	if err := r.serveMCP(t.Context(), strings.NewReader(request)); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if r.mcpCWD != root || r.mcpWorkspaceRoot != canonicalRoot {
		t.Fatalf("MCP did not bind the injected workspace root: cwd=%q root=%q", r.mcpCWD, r.mcpWorkspaceRoot)
	}
	var response struct {
		Result struct {
			StructuredContent struct {
				ProjectID string `json:"project_id"`
			} `json:"structuredContent"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid MCP JSON response: %v output=%s", err, output.String())
	}
	if response.Result.IsError || response.Result.StructuredContent.ProjectID != "project-injected" {
		t.Fatalf("workspace_context did not use the injected root: %#v", response.Result)
	}
}

func TestMCPWorkspaceToolLinkFailureDoesNotReverseBusinessSuccess(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: "http://content.example.com", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	result := callMCPToolForTest(t, &Root{mcpCWD: root}, "workspace_status", map[string]any{})
	if _, ok := result["structuredContent"].(localworkspace.Status); !ok || result["isError"] != false {
		t.Fatalf("link construction failure changed the business result: %#v", result)
	}
	content, ok := result["content"].([]map[string]string)
	if !ok || len(content) != 1 || content[0]["type"] != "text" {
		t.Fatalf("untrusted binding unexpectedly produced a resource link: %#v", result["content"])
	}
}

func TestMCPWorkspaceViewResourcesStayLocal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-view", WorkspaceID: "workspace-view", ServerURL: "https://content.example.com", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	ref := "50-production/脚本 候选.md"
	body := []byte("# 脚本候选\n\n只在本地展示。\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), body, 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Root{mcpCWD: root, now: func() time.Time { return time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC) }}
	viewResult := callMCPToolForTest(t, r, "workspace_view", map[string]any{"view": "content_item", "ref": ref})
	view, ok := viewResult["structuredContent"].(localworkspace.WorkspaceView)
	if !ok || view.View.Text != string(body) || view.ObservedDigest == "" || len(view.Resources) != 1 {
		t.Fatalf("unexpected workspace view result: %#v", viewResult)
	}
	serialized, err := json.Marshal(viewResult)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), root) || strings.Contains(string(serialized), "https://content.example.com") {
		t.Fatalf("workspace view leaked local root or cloud URL: %s", serialized)
	}
	viewContent, ok := viewResult["content"].([]map[string]any)
	if !ok || len(viewContent) != 2 || viewContent[1]["type"] != "resource_link" || viewContent[1]["uri"] != view.Resources[0].URI {
		t.Fatalf("workspace view is missing its standard MCP resource_link: %#v", viewResult["content"])
	}
	fileResourceParams, _ := json.Marshal(map[string]any{"uri": view.Resources[0].URI})
	fileResource := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "resources/read", Params: fileResourceParams})
	filePayload, ok := fileResource.Result.(map[string]any)
	if fileResource.Error != nil || !ok {
		t.Fatalf("workspace file resource failed: %#v", fileResource)
	}
	fileContents := filePayload["contents"].([]map[string]string)
	if len(fileContents) != 1 || fileContents[0]["text"] != string(body) || fileContents[0]["blob"] != "" {
		t.Fatalf("unexpected text resource payload: %#v", fileContents)
	}

	pngRef := "50-production/source.png"
	pngBody := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(pngRef)), pngBody, 0o600); err != nil {
		t.Fatal(err)
	}
	pngResult := callMCPToolForTest(t, r, "workspace_view", map[string]any{"view": "file", "ref": pngRef})
	pngView := pngResult["structuredContent"].(localworkspace.WorkspaceView)
	pngParams, _ := json.Marshal(map[string]any{"uri": pngView.Resources[0].URI})
	pngRead := r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "resources/read", Params: pngParams})
	pngPayload := pngRead.Result.(map[string]any)
	pngContents := pngPayload["contents"].([]map[string]string)
	if len(pngContents) != 1 || pngContents[0]["blob"] == "" || pngContents[0]["text"] != "" {
		t.Fatalf("binary resource did not use MCP blob: %#v", pngContents)
	}
}

func TestMCPWorkbenchKeepsBrowserHandoffPrivateAndClosesCleanly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-workbench", WorkspaceID: "workspace-workbench", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	ref := "50-production/local.md"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), []byte("local workbench\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Root{mcpCWD: root}
	t.Cleanup(func() { _ = r.localWorkbenchManager().Close() })

	opened := callMCPToolForTest(t, r, "workspace_open_workbench", map[string]any{"view": "file", "ref": ref})
	descriptor, ok := opened["structuredContent"].(workbench.Descriptor)
	if !ok || descriptor.WorkbenchID == "" || descriptor.View != "file" || descriptor.Ref != ref || descriptor.Fallback.ObservedDigest == "" {
		t.Fatalf("unexpected public workbench descriptor: %#v", opened)
	}
	publicBody, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicBody), root) || strings.Contains(string(publicBody), "127.0.0.1") || strings.Contains(string(publicBody), "handoff=") {
		t.Fatalf("public workbench descriptor leaked private handoff state: %s", publicBody)
	}
	meta, ok := opened["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("workbench result is missing private MCP metadata: %#v", opened)
	}
	private, ok := meta["run.zhongcao.contentcloud/browserHandoff"].(workbench.PrivateHandoff)
	if !ok || private.WorkbenchID != descriptor.WorkbenchID || private.Origin == "" || !strings.HasPrefix(private.URL, private.Origin+"/#handoff=") {
		t.Fatalf("unexpected private browser handoff: %#v", meta)
	}

	statusResult := callMCPToolForTest(t, r, "workspace_workbench_status", map[string]any{})
	status, ok := statusResult["structuredContent"].(workbench.Status)
	if !ok || status.WorkbenchID != descriptor.WorkbenchID || status.State != "ready" {
		t.Fatalf("unexpected workbench status: %#v", statusResult)
	}
	closed := callMCPToolForTest(t, r, "workspace_close_workbench", map[string]any{})
	closedValue, ok := closed["structuredContent"].(map[string]any)
	if !ok || closedValue["closed"] != true {
		t.Fatalf("unexpected workbench close result: %#v", closed)
	}
	afterClose := callMCPToolExpectErrorForTest(t, r, "workspace_workbench_status", map[string]any{})
	if got := mcpToolErrorCodeForTest(t, afterClose); got != "RESOURCE_NOT_FOUND" {
		t.Fatalf("closed workbench remained visible: %q %#v", got, afterClose)
	}
}

func TestMCPWorkspaceProposalUsesSameKernelAndIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-proposal", WorkspaceID: "workspace-proposal", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	run, err := localworkspace.InitLocalRun(localworkspace.InitLocalRunOptions{Root: root, RunID: "run-mcp-proposal", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	ref := "50-production/draft.md"
	path := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Root{mcpCWD: root, now: func() time.Time { return now }}
	claimResult := callMCPToolForTest(t, r, "local_run_claim", map[string]any{
		"run_id": run.RunID, "owner_kind": "agent", "owner_id": "conversation-proposal", "expected_revision": run.ContextRevision,
	})
	claim, ok := claimResult["structuredContent"].(localworkspace.RunClaim)
	if !ok || claim.Token == "" || claim.Epoch == 0 {
		t.Fatalf("unexpected proposal claim: %#v", claimResult)
	}
	viewResult := callMCPToolForTest(t, r, "workspace_view", map[string]any{"view": "file", "ref": ref, "run_id": run.RunID, "expected_context_revision": run.ContextRevision})
	view := viewResult["structuredContent"].(localworkspace.WorkspaceView)
	prepareArguments := map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "owner_kind": claim.OwnerKind, "owner_id": claim.OwnerID, "owner_epoch": claim.Epoch,
		"expected_context_revision": run.ContextRevision, "typed_action": "workspace_file.replace", "ref": ref,
		"expected_digest": view.ObservedDigest, "content": "after\n", "idempotency_key": "mcp-prepare-001",
	}
	prepared := callMCPToolForTest(t, r, "workspace_proposal_prepare", prepareArguments)
	proposal, ok := prepared["structuredContent"].(localworkspace.WorkspaceProposal)
	if !ok || proposal.ProposalID == "" || proposal.OwnerEpoch != claim.Epoch {
		t.Fatalf("unexpected MCP Proposal: %#v", prepared)
	}
	replayed := callMCPToolForTest(t, r, "workspace_proposal_prepare", prepareArguments)
	if replayedProposal := replayed["structuredContent"].(localworkspace.WorkspaceProposal); replayedProposal.ProposalID != proposal.ProposalID {
		t.Fatalf("idempotent MCP prepare created another Proposal: first=%s replay=%s", proposal.ProposalID, replayedProposal.ProposalID)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "before\n" {
		t.Fatalf("MCP prepare changed the workspace: body=%q err=%v", body, err)
	}
	applyArguments := map[string]any{
		"proposal_id": proposal.ProposalID, "claim_token": claim.Token, "owner_kind": claim.OwnerKind, "owner_id": claim.OwnerID, "owner_epoch": claim.Epoch,
		"expected_context_revision": run.ContextRevision, "idempotency_key": "mcp-apply-001", "confirm": true,
	}
	applied := callMCPToolForTest(t, r, "workspace_proposal_apply", applyArguments)
	applyResult, ok := applied["structuredContent"].(localworkspace.WorkspaceProposalApplyResult)
	if !ok || !applyResult.Applied || applyResult.ContextRevision != run.ContextRevision+1 {
		t.Fatalf("unexpected MCP Apply: %#v", applied)
	}
	replayedApply := callMCPToolForTest(t, r, "workspace_proposal_apply", applyArguments)
	if replayedResult := replayedApply["structuredContent"].(localworkspace.WorkspaceProposalApplyResult); replayedResult.ProposalID != applyResult.ProposalID {
		t.Fatalf("idempotent MCP Apply changed result: first=%#v replay=%#v", applyResult, replayedResult)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "after\n" {
		t.Fatalf("MCP Apply did not persist the exact draft: body=%q err=%v", body, err)
	}
	unknown := callMCPToolExpectErrorForTest(t, r, "workspace_proposal_prepare", map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "owner_kind": claim.OwnerKind, "owner_id": claim.OwnerID, "owner_epoch": claim.Epoch,
		"expected_context_revision": applyResult.ContextRevision, "typed_action": "workspace_file.replace", "ref": ref,
		"expected_digest": applyResult.Outputs[0].Digest, "content": "other\n", "idempotency_key": "mcp-prepare-002", "url": "https://evil.example",
	})
	if got := mcpToolErrorCodeForTest(t, unknown); got != "MCP_PARAMS_INVALID" {
		t.Fatalf("proposal tool accepted an unknown field: %q %#v", got, unknown)
	}
}

func TestMCPWorkspaceViewRejectsUnknownFieldsAndStaleDigest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-view", WorkspaceID: "workspace-view", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	ref := "50-production/item.json"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), []byte(`{"id":"item-1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Root{mcpCWD: root}
	unknown := callMCPToolExpectErrorForTest(t, r, "workspace_view", map[string]any{"view": "file", "ref": ref, "url": "https://evil.example"})
	if got := mcpToolErrorCodeForTest(t, unknown); got != "MCP_PARAMS_INVALID" {
		t.Fatalf("unknown workspace_view field was accepted: %q %#v", got, unknown)
	}
	stale := callMCPToolExpectErrorForTest(t, r, "workspace_view", map[string]any{"view": "file", "ref": ref, "expected_digest": "sha256:" + strings.Repeat("0", 64)})
	if got := mcpToolErrorCodeForTest(t, stale); got != "WORKSPACE_VIEW_STALE" {
		t.Fatalf("stale workspace view was accepted: %q %#v", got, stale)
	}
	removedRender := callMCPToolExpectErrorForTest(t, r, "workspace_render", map[string]any{"ref": ref})
	if got := mcpToolErrorCodeForTest(t, removedRender); got != "RESOURCE_NOT_FOUND" {
		t.Fatalf("removed workspace_render tool is still callable: %q %#v", got, removedRender)
	}
}

func TestMCPServeSerializesLocalWorkspaceResourceLinks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-view", WorkspaceID: "workspace-view", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	ref := "50-production/local.md"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), []byte("local view\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workspace_view","arguments":{"view":"file","ref":"50-production/local.md"}}}` + "\n"
	var output bytes.Buffer
	if err := (&Root{mcpCWD: root, stdout: &output}).serveMCP(t.Context(), strings.NewReader(request)); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result struct {
			Content []struct {
				Type     string `json:"type"`
				URI      string `json:"uri"`
				MimeType string `json:"mimeType"`
			} `json:"content"`
			StructuredContent struct {
				ObservedDigest string `json:"observed_digest"`
			} `json:"structuredContent"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("invalid MCP JSON response: %v output=%s", err, output.String())
	}
	if response.Result.IsError || len(response.Result.Content) != 2 || response.Result.Content[1].Type != "resource_link" || response.Result.Content[1].MimeType != "text/markdown" || !strings.HasPrefix(response.Result.Content[1].URI, "contentcloud://workspace/files/") || response.Result.StructuredContent.ObservedDigest == "" {
		t.Fatalf("unexpected serialized local resource contract: %#v", response.Result)
	}
	if strings.Contains(output.String(), root) {
		t.Fatalf("serialized local view leaked workspace root: %s", output.String())
	}
}

func mcpToolErrorCodeForTest(t *testing.T, result map[string]any) string {
	t.Helper()
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("tool error is missing structuredContent: %#v", result)
	}
	domainError, ok := structured["error"].(*domain.Error)
	if !ok {
		t.Fatalf("tool error is missing domain error: %#v", structured)
	}
	return domainError.Code
}

func TestMCPEnvironmentExecutionPlanUsesVerifiedOfflineState(t *testing.T) {
	now := time.Date(2026, 7, 27, 4, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", ServerURL: "https://content.example.com", CLIVersion: Version, Target: "codex-plugin", Now: now}); err != nil {
		t.Fatal(err)
	}
	manifest, manifestVerifier, registry, registryVerifier := bootstrapEnvironmentFixture(t, now)
	if _, err := localworkspace.StoreEnvironmentRegistry(root, registry, registryVerifier); err != nil {
		t.Fatal(err)
	}
	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: pluginidentity.VideoProductionVersion, Digest: manifest.Distribution.Plugins[0].Digest, Installed: true}}
	if _, err := localworkspace.StoreEnvironment(root, manifest, installed, manifestVerifier, now); err != nil {
		t.Fatal(err)
	}
	runtime := &Root{mcpCWD: root, now: func() time.Time { return now.Add(time.Minute) }, manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier)}
	contextResult := callMCPToolForTest(t, runtime, "workspace_context", map[string]any{})
	context, ok := contextResult["structuredContent"].(localworkspace.WorkspaceConversationContext)
	if !ok || len(context.ContentTypes) != 1 || context.ContentTypes[0] != domain.ContentTypeVideoScript {
		t.Fatalf("workspace context did not expose verified content types: %#v", contextResult)
	}
	arguments := map[string]any{"run_id": "run-1", "intent": "extract verified project knowledge", "required_capabilities": []string{domain.KnowledgeExtractCapability}, "input_refs": []string{"20-sources/source-registry.json"}}
	first := callMCPToolForTest(t, runtime, "environment_execution_plan", arguments)
	second := callMCPToolForTest(t, runtime, "environment_execution_plan", arguments)
	firstPlan, ok := first["structuredContent"].(environment.LocalExecutionPlan)
	if !ok || firstPlan.State != "ready" || !strings.HasPrefix(firstPlan.PlanID, "lep_") || len(firstPlan.Preparation) != 0 {
		t.Fatalf("local execution plan = %#v", first)
	}
	secondPlan, ok := second["structuredContent"].(environment.LocalExecutionPlan)
	if !ok || secondPlan.PlanID != firstPlan.PlanID {
		t.Fatalf("local execution plan is not deterministic: first=%#v second=%#v", first, second)
	}
	denied := callMCPToolExpectErrorForTest(t, runtime, "article_brief_lint", map[string]any{"file": "50-production/briefs/article.json"})
	if code := mcpToolErrorCodeForTest(t, denied); code != "CONTENT_TYPE_NOT_ENABLED" {
		t.Fatalf("disabled WeChat MCP tool returned %s", code)
	}
}

func TestMCPEnvironmentPreparationRequiresExactConfirmationAndReachesReady(t *testing.T) {
	now := time.Date(2026, 7, 27, 4, 30, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", ServerURL: "https://content.example.com", CLIVersion: Version, Target: "codex-plugin", Now: now}); err != nil {
		t.Fatal(err)
	}
	manifest, manifestVerifier, registry, registryVerifier := bootstrapEnvironmentFixture(t, now)
	if _, err := localworkspace.StoreEnvironmentRegistry(root, registry, registryVerifier); err != nil {
		t.Fatal(err)
	}
	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: pluginidentity.VideoProductionVersion, Digest: manifest.Distribution.Plugins[0].Digest, Installed: true}}
	if _, err := localworkspace.StoreEnvironment(root, manifest, installed, manifestVerifier, now); err != nil {
		t.Fatal(err)
	}
	runner := &bootstrapRunner{responses: successfulTaskPackResponses()}
	runtime := &Root{mcpCWD: root, now: func() time.Time { return now.Add(time.Minute) }, pluginRunner: runner, pluginRuntimeHook: testPluginRuntimeHook(t, pluginhost.StatusAbsent), manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier)}
	arguments := map[string]any{"run_id": "run-pack-1", "intent": "generate visual storytelling assets", "required_capabilities": []string{"contentcloud.asset.generate"}, "input_refs": []string{"50-production/briefs/brief.json"}}
	planned := callMCPToolForTest(t, runtime, "environment_prepare_plan", arguments)
	preparation, ok := planned["structuredContent"].(environment.PreparationPlan)
	if !ok || preparation.State != "ready" || len(preparation.Actions) != 1 || preparation.Actions[0].Plugin.ID != "contentcloud-visual-storytelling" || !preparation.RequiresConfirmation || !preparation.RequiresNewChat {
		t.Fatalf("preparation plan = %#v", planned)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("preparation plan invoked Codex: %#v", runner.calls)
	}

	unconfirmed := cloneStringAnyMap(arguments)
	unconfirmed["preparation_id"] = preparation.PreparationID
	unconfirmed["accept"] = false
	callMCPToolExpectErrorForTest(t, runtime, "environment_prepare_apply", unconfirmed)
	stale := cloneStringAnyMap(arguments)
	stale["preparation_id"] = "epp_" + strings.Repeat("0", 64)
	stale["accept"] = true
	callMCPToolExpectErrorForTest(t, runtime, "environment_prepare_apply", stale)
	if len(runner.calls) != 0 {
		t.Fatalf("unconfirmed or stale preparation invoked Codex: %#v", runner.calls)
	}

	confirmed := cloneStringAnyMap(arguments)
	confirmed["preparation_id"] = preparation.PreparationID
	confirmed["accept"] = true
	applied := callMCPToolExpectErrorForTest(t, runtime, "environment_prepare_apply", confirmed)
	if code := mcpToolErrorCodeForTest(t, applied); code != "ENVIRONMENT_PLUGIN_ARTIFACT_UNAVAILABLE" {
		t.Fatalf("unpublished task Pack was not blocked by standard artifact boundary: %s", code)
	}
	state, err := localworkspace.LoadEnvironment(root, manifestVerifier, now.Add(2*time.Minute))
	if err != nil || len(state.Lock.Plugins) != 1 || state.Lock.Plugins[0].ID != "contentcloud-video-production" {
		t.Fatalf("blocked preparation changed environment.lock = %#v, err = %v", state, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".contentcloud", "environment-preparation.lock")); !os.IsNotExist(err) {
		t.Fatalf("preparation lease remains after success: %v", err)
	}
}

func TestEnvironmentPreparationLoadsMarketingPackForCodexAndClaude(t *testing.T) {
	now := time.Date(2026, 8, 15, 13, 0, 0, 0, time.UTC)
	for _, host := range []pluginhost.HostID{pluginhost.HostCodex, pluginhost.HostClaude} {
		t.Run(string(host), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "project")
			target := string(host) + "-plugin"
			if host == pluginhost.HostClaude {
				// Claude customer bootstrap remains capability-gated. This fixture
				// represents a workspace already registered to the Claude host.
				target = "none"
			}
			if _, err := localworkspace.Initialize(localworkspace.InitOptions{
				Root: root, WorkspaceID: "workspace-marketing-" + string(host), ProjectID: "project-1", ServerURL: "https://content.example.com", CLIVersion: Version, Target: target, Now: now,
			}); err != nil {
				t.Fatal(err)
			}
			if host == pluginhost.HostClaude {
				lockPath := filepath.Join(root, ".contentcloud", "template.lock")
				body, readErr := os.ReadFile(lockPath)
				if readErr != nil {
					t.Fatal(readErr)
				}
				var lock localworkspace.TemplateLock
				if decodeErr := json.Unmarshal(body, &lock); decodeErr != nil {
					t.Fatal(decodeErr)
				}
				lock.Targets = []string{"claude-plugin"}
				body, encodeErr := json.MarshalIndent(lock, "", "  ")
				if encodeErr != nil {
					t.Fatal(encodeErr)
				}
				if writeErr := os.WriteFile(lockPath, append(body, '\n'), 0o600); writeErr != nil {
					t.Fatal(writeErr)
				}
			}
			marketingPackage, err := pluginbuiltin.Load(t.TempDir(), pluginidentity.Marketing, pluginidentity.MarketingVersion)
			if err != nil {
				t.Fatal(err)
			}
			manifest, manifestVerifier, registry, registryVerifier := bootstrapEnvironmentFixtureWithTaskPack(t, now, bootstrapTaskPackFixture{
				ID: pluginidentity.Marketing, Version: pluginidentity.MarketingVersion, Digest: marketingPackage.Digest, Capability: "contentcloud.marketing.content-orchestration",
			})
			if _, err := localworkspace.StoreEnvironmentRegistry(root, registry, registryVerifier); err != nil {
				t.Fatal(err)
			}
			sceneDigest := ""
			for _, plugin := range manifest.Distribution.Plugins {
				if plugin.ID == pluginidentity.VideoProduction {
					sceneDigest = plugin.Digest
				}
			}
			if sceneDigest == "" {
				t.Fatal("marketing manifest is missing the core scene plugin")
			}
			installed := []environment.LockedPlugin{{ID: pluginidentity.VideoProduction, Kind: "scene_plugin", Version: pluginidentity.VideoProductionVersion, Digest: sceneDigest, Installed: true}}
			if _, err := localworkspace.StoreEnvironment(root, manifest, installed, manifestVerifier, now); err != nil {
				t.Fatal(err)
			}

			requested := []string{}
			runtime := &Root{
				mcpCWD: root, now: func() time.Time { return now.Add(time.Minute) },
				manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier),
				pluginRuntimeHook: func(hostName, pluginID, version string) (*hostPluginRuntime, error) {
					requested = append(requested, hostName+":"+pluginID+"@"+version)
					if hostName != string(host) {
						t.Fatalf("environment selected host %q, want %q", hostName, host)
					}
					pkg, loadErr := pluginbuiltin.Load(t.TempDir(), pluginID, version)
					if loadErr != nil {
						return nil, loadErr
					}
					store, storeErr := pluginhost.NewStore(t.TempDir())
					if storeErr != nil {
						return nil, storeErr
					}
					native := &testBootstrapHost{status: pluginhost.StatusAbsent, hostID: host}
					adapter, adapterErr := pluginhost.New(native, store)
					if adapterErr != nil {
						return nil, adapterErr
					}
					return &hostPluginRuntime{Adapter: adapter, Package: pkg, HostID: host}, nil
				},
			}
			input := environmentPreparationInput{Directory: root, RunID: "run-marketing-" + string(host), Intent: "compile marketing content", Capabilities: []string{"contentcloud.marketing.content-orchestration"}, InputRefs: []string{"50-production/briefs/brief.json"}}
			_, _, preparation, err := runtime.resolveEnvironmentPreparation(input)
			if err != nil {
				t.Fatal(err)
			}
			if preparation.State != "ready" || len(preparation.Actions) != 1 || preparation.Actions[0].Plugin.ID != pluginidentity.Marketing {
				t.Fatalf("marketing preparation = %#v", preparation)
			}
			result, err := runtime.applyEnvironmentPreparation(t.Context(), input, preparation.PreparationID, true)
			if err != nil {
				t.Fatal(err)
			}
			if len(requested) != 1 || requested[0] != string(host)+":"+pluginidentity.Marketing+"@"+pluginidentity.MarketingVersion {
				t.Fatalf("bundled runtime selection = %#v", requested)
			}
			if len(result.InstalledPacks) != 1 || result.InstalledPacks[0].Plugin.ID != pluginidentity.Marketing || result.ExecutionPlan.State != "ready" {
				t.Fatalf("marketing apply result = %#v", result)
			}
			state, err := localworkspace.LoadEnvironment(root, manifestVerifier, now.Add(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			foundMarketing := false
			for _, plugin := range state.Lock.Plugins {
				if plugin.ID == pluginidentity.Marketing && plugin.Installed && plugin.Digest == marketingPackage.Digest {
					foundMarketing = true
				}
			}
			if !foundMarketing || !result.Handoff.RequiresNewChat {
				t.Fatalf("marketing lock/handoff = lock=%#v handoff=%#v", state.Lock, result.Handoff)
			}
		})
	}
}

func TestWorkspacePrepareCLIPlanAndApplyUseTheSameDeterministicPlan(t *testing.T) {
	now := time.Date(2026, 7, 27, 4, 45, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", ServerURL: "https://content.example.com", CLIVersion: Version, Target: "codex-plugin", Now: now}); err != nil {
		t.Fatal(err)
	}
	manifest, manifestVerifier, registry, registryVerifier := bootstrapEnvironmentFixture(t, now)
	if _, err := localworkspace.StoreEnvironmentRegistry(root, registry, registryVerifier); err != nil {
		t.Fatal(err)
	}
	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: pluginidentity.VideoProductionVersion, Digest: manifest.Distribution.Plugins[0].Digest, Installed: true}}
	if _, err := localworkspace.StoreEnvironment(root, manifest, installed, manifestVerifier, now); err != nil {
		t.Fatal(err)
	}

	var planOutput, planErrors bytes.Buffer
	planner := &Root{stdout: &planOutput, stderr: &planErrors, now: func() time.Time { return now.Add(time.Minute) }, manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier)}
	planCommand := planner.command()
	planCommand.SetArgs([]string{"--json", "workspace", "prepare", "plan", "--directory", root, "--run", "run-cli-pack", "--intent", "generate visual storytelling assets", "--capability", "contentcloud.asset.generate", "--input", "50-production/briefs/brief.json"})
	if err := planCommand.Execute(); err != nil {
		t.Fatalf("workspace prepare plan failed: %v; stderr=%s", err, planErrors.String())
	}
	var planEnvelope struct {
		OK   bool                        `json:"ok"`
		Data environment.PreparationPlan `json:"data"`
	}
	if err := json.Unmarshal(planOutput.Bytes(), &planEnvelope); err != nil || !planEnvelope.OK || planEnvelope.Data.State != "ready" {
		t.Fatalf("workspace prepare plan output: err=%v output=%s", err, planOutput.String())
	}

	runner := &bootstrapRunner{responses: successfulTaskPackResponses()}
	var applyOutput, applyErrors bytes.Buffer
	applier := &Root{stdout: &applyOutput, stderr: &applyErrors, pluginRunner: runner, pluginRuntimeHook: testPluginRuntimeHook(t, pluginhost.StatusAbsent), now: func() time.Time { return now.Add(time.Minute) }, manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier)}
	applyCommand := applier.command()
	applyCommand.SetArgs([]string{"--json", "workspace", "prepare", "apply", "--directory", root, "--run", "run-cli-pack", "--intent", "generate visual storytelling assets", "--capability", "contentcloud.asset.generate", "--input", "50-production/briefs/brief.json", "--preparation-id", planEnvelope.Data.PreparationID, "--accept"})
	if err := applyCommand.Execute(); err == nil || !strings.Contains(err.Error(), "标准插件包未随当前 CLI 发布") {
		t.Fatalf("workspace prepare apply should block unpublished standard artifact: %v; stderr=%s", err, applyErrors.String())
	}
}

func TestEnvironmentPreparationFailureRollsBackOnlyTheNewPack(t *testing.T) {
	now := time.Date(2026, 7, 27, 5, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", ServerURL: "https://content.example.com", CLIVersion: Version, Target: "codex-plugin", Now: now}); err != nil {
		t.Fatal(err)
	}
	manifest, manifestVerifier, registry, registryVerifier := bootstrapEnvironmentFixture(t, now)
	if _, err := localworkspace.StoreEnvironmentRegistry(root, registry, registryVerifier); err != nil {
		t.Fatal(err)
	}
	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: pluginidentity.VideoProductionVersion, Digest: manifest.Distribution.Plugins[0].Digest, Installed: true}}
	if _, err := localworkspace.StoreEnvironment(root, manifest, installed, manifestVerifier, now); err != nil {
		t.Fatal(err)
	}
	currentMarketplace := `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"v0.27.0"}}]}`
	missingPack := `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.27.0","installed":true,"enabled":true}],"available":[]}`
	runner := &bootstrapRunner{responses: []bootstrapRunnerResponse{
		{stdout: currentMarketplace}, {stdout: missingPack},
		{stdout: currentMarketplace}, {stdout: missingPack},
		{stdout: `{"pluginId":"contentcloud-visual-storytelling@contentcloud","name":"contentcloud-visual-storytelling","marketplaceName":"contentcloud","version":"1.2.0","installedPath":"/tmp/visual-storytelling"}`},
		{stdout: currentMarketplace}, {stdout: missingPack},
		{stdout: `{"removed":true}`},
	}}
	runtime := &Root{mcpCWD: root, pluginRunner: runner, pluginRuntimeHook: testPluginRuntimeHook(t, pluginhost.StatusAbsent), now: func() time.Time { return now.Add(time.Minute) }, manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier)}
	input := environmentPreparationInput{RunID: "run-pack-failure", Intent: "generate visual storytelling assets", Capabilities: []string{"contentcloud.asset.generate"}, InputRefs: []string{"50-production/briefs/brief.json"}}
	_, _, preparation, err := runtime.resolveEnvironmentPreparation(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.applyEnvironmentPreparation(t.Context(), input, preparation.PreparationID, true); err == nil {
		t.Fatal("Pack validation failure unexpectedly succeeded")
	}
	state, err := localworkspace.LoadEnvironment(root, manifestVerifier, now.Add(2*time.Minute))
	if err != nil || len(state.Lock.Plugins) != 1 || state.Lock.Plugins[0].ID != "contentcloud-video-production" {
		t.Fatalf("failed preparation changed environment.lock: state=%#v err=%v", state, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".contentcloud", "environment-preparation.lock")); !os.IsNotExist(err) {
		t.Fatalf("failed preparation retained lease: %v", err)
	}
}

func successfulTaskPackResponses() []bootstrapRunnerResponse {
	currentMarketplace := `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"v0.27.0"}}]}`
	missingPack := `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.27.0","installed":true,"enabled":true}],"available":[]}`
	currentPack := `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.27.0","installed":true,"enabled":true},{"pluginId":"contentcloud-visual-storytelling@contentcloud","name":"contentcloud-visual-storytelling","marketplaceName":"contentcloud","version":"1.2.0","installed":true,"enabled":true}],"available":[]}`
	return []bootstrapRunnerResponse{
		{stdout: currentMarketplace}, {stdout: missingPack},
		{stdout: currentMarketplace}, {stdout: missingPack},
		{stdout: `{"pluginId":"contentcloud-visual-storytelling@contentcloud","name":"contentcloud-visual-storytelling","marketplaceName":"contentcloud","version":"1.2.0","installedPath":"/tmp/visual-storytelling"}`},
		{stdout: currentMarketplace}, {stdout: currentPack},
	}
}

func cloneStringAnyMap(value map[string]any) map[string]any {
	copy := make(map[string]any, len(value))
	for key, item := range value {
		copy[key] = item
	}
	return copy
}

func TestMCPPublishApplyRequiresExactConfirmationBeforeCloudWrite(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		var payload struct {
			Command string                  `json:"command"`
			Params  domain.SubmissionBundle `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Command != "submission.create" {
			t.Fatalf("unexpected command: %s", payload.Command)
		}
		revision := domain.SubmissionRevision{ID: "revision-1", SubmissionID: "submission-1", ProjectID: "project-1", WorkspaceID: "workspace-1", ContentHash: payload.Params.ContentHash, IdempotencyKey: payload.Params.IdempotencyKey}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"ok": true, "command": payload.Command, "data": revision}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: server.URL, CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	pack := filepath.Join(root, "30-knowledge", "packs", "knowledge.json")
	if err := os.WriteFile(pack, []byte(`[{"id":"fact-1","kind":"fact","status":"verified"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_test_workspace")
	r := &Root{mcpCWD: root}
	preflightResult := callMCPToolForTest(t, r, "publish_preflight", map[string]any{"submission_type": "knowledge"})
	preflightEnvelope, ok := preflightResult["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected preflight result: %#v", preflightResult)
	}
	preflight, ok := preflightEnvelope["preflight"].(publishPreflight)
	if !ok || preflight.PlanID == "" || !preflight.RequiresConfirm {
		t.Fatalf("unexpected publish preflight: %#v", preflightEnvelope)
	}

	for _, arguments := range []map[string]any{
		{"submission_type": "knowledge", "plan_id": preflight.PlanID, "accept": false},
		{"submission_type": "knowledge", "plan_id": "pp_" + strings.Repeat("0", 64), "accept": true},
	} {
		result := callMCPToolExpectErrorForTest(t, r, "publish_apply", arguments)
		if result["isError"] != true {
			t.Fatalf("publish_apply unexpectedly succeeded: %#v", result)
		}
	}
	if requests != 0 {
		t.Fatalf("unconfirmed publish performed %d cloud writes", requests)
	}

	result := callMCPToolForTest(t, r, "publish_apply", map[string]any{"submission_type": "knowledge", "plan_id": preflight.PlanID, "accept": true})
	value, ok := result["structuredContent"].(map[string]any)
	if !ok || value["cloud_write"] != true {
		t.Fatalf("confirmed publish did not return a cloud write: %#v", result)
	}
	publishLink := mcpProjectViewURLForTest(t, result)
	if publishLink.Path != "/studio/tasks" || publishLink.Query().Get("project") != "project-1" || publishLink.Query().Get("focus_kind") != "submission_revision" || publishLink.Query().Get("focus_id") != "revision-1" || publishLink.Query().Get("expected_digest") != preflight.ContentHash {
		t.Fatalf("publish_apply did not attach the exact created revision: %s", publishLink)
	}
	if requests != 1 {
		t.Fatalf("confirmed publish performed %d cloud writes", requests)
	}
	status, err := localworkspace.LoadStatus(root)
	if err != nil || status.Sync.Published["knowledge"].SubmissionRevisionID != "revision-1" {
		t.Fatalf("published revision was not checkpointed locally: status=%#v err=%v", status.Sync, err)
	}
}

func TestMCPFeedbackPullCreatesImmutableInboxForNewConversation(t *testing.T) {
	pulls := 0
	now := time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload struct {
			Command string `json:"command"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Command != "feedback.workspace-list" {
			t.Fatalf("unexpected command: %s", payload.Command)
		}
		pulls++
		comments := []domain.ReviewComment{{ID: "comment-1", Body: "补充证据", CreatedAt: now}}
		createdAt := now
		if pulls == 2 {
			createdAt = now.Add(time.Minute)
			comments = append(comments, domain.ReviewComment{ID: "comment-2", Body: "收紧 CTA", CreatedAt: createdAt})
		}
		feedback := []domain.ReviewFeedbackBundle{{BundleVersion: "1.0", SubmissionID: "submission-1", SubmissionRevisionID: "revision-1", SubjectHash: "sha256:" + strings.Repeat("a", 64), Comments: comments, CreatedAt: createdAt}}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"ok": true, "command": payload.Command, "data": feedback}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: server.URL, CLIVersion: "test", Target: "none", Now: now}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_test_workspace")
	cloudConversation := &Root{mcpCWD: root, now: func() time.Time { return now }}
	firstPull := callMCPToolForTest(t, cloudConversation, "review_feedback_pull", map[string]any{})
	feedbackLink := mcpProjectViewURLForTest(t, firstPull)
	if feedbackLink.Path != "/studio/tasks" || feedbackLink.Query().Get("project") != "project-1" || feedbackLink.Query().Get("focus_kind") != "submission_revision" || feedbackLink.Query().Get("focus_id") != "revision-1" || feedbackLink.Query().Get("expected_digest") != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("review_feedback_pull did not attach its unique revision: %s", feedbackLink)
	}
	cloudConversation.now = func() time.Time { return now.Add(time.Minute) }
	callMCPToolForTest(t, cloudConversation, "review_feedback_pull", map[string]any{})

	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "")
	offlineConversation := &Root{mcpCWD: root, now: func() time.Time { return now.Add(2 * time.Minute) }}
	inboxResult := callMCPToolForTest(t, offlineConversation, "review_feedback_inbox", map[string]any{})
	inbox, ok := inboxResult["structuredContent"].(map[string]any)
	items, itemsOK := inbox["feedback"].([]localworkspace.ReviewFeedbackInboxItem)
	if !ok || !itemsOK || inbox["offline"] != true || len(items) != 2 || items[0].ID == items[1].ID {
		t.Fatalf("new conversation could not read immutable feedback revisions: %#v", inboxResult)
	}
	contextResult := callMCPToolForTest(t, offlineConversation, "workspace_context", map[string]any{})
	conversation, ok := contextResult["structuredContent"].(localworkspace.WorkspaceConversationContext)
	if !ok || conversation.ReviewInboxCount != 2 || !conversation.Offline {
		t.Fatalf("conversation context did not expose feedback inbox count: %#v", contextResult)
	}
	if pulls != 2 {
		t.Fatalf("offline reads unexpectedly contacted the cloud: pulls=%d", pulls)
	}
}

func TestMCPApprovedSnapshotPullSupportsOfflineCrossConversationRead(t *testing.T) {
	cloudReads := 0
	now := time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC)
	snapshots := []domain.ApprovedSnapshot{
		approvedSnapshotForMCPTest(t, "snapshot-2", "revision-2", "fact-2", now.Add(time.Minute)),
		approvedSnapshotForMCPTest(t, "snapshot-1", "revision-1", "fact-1", now),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload struct {
			Command string `json:"command"`
			Params  struct {
				ID string `json:"id"`
			} `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		var data any
		switch payload.Command {
		case "snapshot.workspace-list":
			data = snapshots
		case "snapshot.workspace-show":
			for _, snapshot := range snapshots {
				if snapshot.ID == payload.Params.ID {
					data = snapshot
					break
				}
			}
		default:
			t.Fatalf("unexpected command: %s", payload.Command)
		}
		cloudReads++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"ok": true, "command": payload.Command, "data": data}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: server.URL, CLIVersion: "test", Target: "none", Now: now}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_test_workspace")
	conversationA := &Root{mcpCWD: root, now: func() time.Time { return now.Add(2 * time.Minute) }}
	pulled := callMCPToolForTest(t, conversationA, "approved_snapshot_pull", map[string]any{"submission_type": "knowledge"})
	pullValue, ok := pulled["structuredContent"].(map[string]any)
	if !ok || pullValue["count"] != 2 {
		t.Fatalf("unexpected approved snapshot pull: %#v", pulled)
	}
	batchLink := mcpProjectViewURLForTest(t, pulled)
	if batchLink.Path != "/studio/knowledge" || batchLink.Query().Get("project") != "project-1" || batchLink.Query().Get("focus_kind") != "" {
		t.Fatalf("batch snapshot pull must not select an arbitrary snapshot: %s", batchLink)
	}
	exactPull := callMCPToolForTest(t, conversationA, "approved_snapshot_pull", map[string]any{"snapshot_id": "snapshot-1"})
	exactLink := mcpProjectViewURLForTest(t, exactPull)
	if exactLink.Path != "/studio/knowledge" || exactLink.Query().Get("project") != "project-1" || exactLink.Query().Get("focus_kind") != "snapshot" || exactLink.Query().Get("focus_id") != "snapshot-1" || exactLink.Query().Get("expected_digest") != snapshots[1].ContentHash {
		t.Fatalf("exact snapshot pull did not attach the selected immutable snapshot: %s", exactLink)
	}

	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "")
	conversationB := &Root{mcpCWD: root}
	inboxResult := callMCPToolForTest(t, conversationB, "approved_snapshot_inbox", map[string]any{"submission_type": "knowledge"})
	inbox, ok := inboxResult["structuredContent"].(map[string]any)
	items, itemsOK := inbox["snapshots"].([]localworkspace.ApprovedSnapshotCacheSummary)
	if !ok || !itemsOK || inbox["offline"] != true || len(items) != 2 || items[0].ID != "snapshot-2" || items[1].ID != "snapshot-1" {
		t.Fatalf("conversation B could not list immutable approved versions: %#v", inboxResult)
	}
	showB := callMCPToolForTest(t, conversationB, "approved_snapshot_show", map[string]any{"snapshot_id": "snapshot-1"})
	recordB := showB["structuredContent"].(map[string]any)["record"].(localworkspace.ApprovedSnapshotCacheRecord)

	conversationC := &Root{mcpCWD: root}
	showC := callMCPToolForTest(t, conversationC, "approved_snapshot_show", map[string]any{"snapshot_id": "snapshot-1"})
	recordC := showC["structuredContent"].(map[string]any)["record"].(localworkspace.ApprovedSnapshotCacheRecord)
	if recordB.Summary.SHA256 == "" || recordB.Summary.SHA256 != recordC.Summary.SHA256 || recordB.Snapshot.SubmissionRevisionID != "revision-1" {
		t.Fatalf("conversations did not share the same verified snapshot: B=%#v C=%#v", recordB, recordC)
	}
	if cloudReads != 2 {
		t.Fatalf("offline approved reads unexpectedly contacted the cloud: reads=%d", cloudReads)
	}
}

func TestWorkspaceApprovedCommandsReadCacheWithoutCredential(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	now := time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC)
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", CLIVersion: "test", Target: "none", Now: now}); err != nil {
		t.Fatal(err)
	}
	snapshot := approvedSnapshotForMCPTest(t, "snapshot-cli", "revision-cli", "fact-cli", now)
	if _, err := localworkspace.StoreApprovedSnapshot(root, snapshot, now); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "")
	for _, args := range [][]string{
		{"--json", "workspace", "approved", "list", "--directory", root, "--type", "knowledge"},
		{"--json", "workspace", "approved", "show", snapshot.ID, "--directory", root},
	} {
		var stdout, stderr bytes.Buffer
		command := (&Root{stdout: &stdout, stderr: &stderr}).command()
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("offline approved command %v failed: %v stderr=%s", args, err, stderr.String())
		}
		var envelope struct {
			OK bool `json:"ok"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK {
			t.Fatalf("unexpected offline approved output: err=%v output=%s", err, stdout.String())
		}
	}
}

func approvedSnapshotForMCPTest(t *testing.T, snapshotID, revisionID, objectID string, createdAt time.Time) domain.ApprovedSnapshot {
	t.Helper()
	canonical, err := json.Marshal(map[string]any{
		"schema_version":  "contentcloud.knowledge/2.0",
		"submission_type": "knowledge",
		"objects":         []map[string]any{{"id": objectID, "kind": "fact", "status": "approved"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return domain.ApprovedSnapshot{
		ID: snapshotID, ProjectID: "project-1", WorkspaceID: "workspace-1", SubmissionID: "submission-1", SubmissionRevisionID: revisionID,
		SubmissionType: "knowledge", SchemaVersion: "contentcloud.knowledge/2.0", ContentHash: "sha256:" + strings.Repeat("a", 64), SubjectHash: "sha256:" + strings.Repeat("a", 64),
		CanonicalContent: canonical, EligibleIDs: []string{objectID}, CreatedAt: createdAt,
	}
}

func TestMCPRunsCrossConversationHandoffLifecycle(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "codex-plugin", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 27, 7, 0, 0, 0, time.UTC)
	run, err := localworkspace.InitLocalRun(localworkspace.InitLocalRunOptions{Root: root, RunID: "run-mcp-handoff", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := filepath.Join(root, "50-production", "batches", "checkpoint.json")
	if err := os.WriteFile(checkpoint, []byte("{\"version\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Root{mcpCWD: root, now: func() time.Time { return now }}
	claimResult := callMCPToolForTest(t, r, "local_run_claim", map[string]any{"run_id": run.RunID, "owner_kind": "agent", "owner_id": "conversation-a", "expected_revision": run.ContextRevision})
	claim, ok := claimResult["structuredContent"].(localworkspace.RunClaim)
	if !ok || claim.Token == "" {
		t.Fatalf("unexpected claim result: %#v", claimResult)
	}
	handoffResult := callMCPToolForTest(t, r, "handoff_create_ready", map[string]any{
		"handoff_id":         "handoff-mcp",
		"run_id":             run.RunID,
		"claim_token":        claim.Token,
		"expected_revision":  run.ContextRevision,
		"next_capability_id": "contentcloud.marketing-video-script",
		"next_action":        "continue the script",
		"input_paths":        []string{"50-production/batches/checkpoint.json"},
	})
	handoff, ok := handoffResult["structuredContent"].(localworkspace.HandoffRecord)
	if !ok || handoff.Status != "ready" {
		t.Fatalf("unexpected handoff result: %#v", handoffResult)
	}
	contextResult := callMCPToolForTest(t, r, "workspace_context", map[string]any{})
	conversation, ok := contextResult["structuredContent"].(localworkspace.WorkspaceConversationContext)
	if !ok || len(conversation.ReadyHandoffs) != 1 || conversation.ReadyHandoffs[0].HandoffID != handoff.HandoffID {
		t.Fatalf("handoff missing from conversation context: %#v", contextResult)
	}
	acceptResult := callMCPToolForTest(t, r, "handoff_accept", map[string]any{"handoff_id": handoff.HandoffID, "owner_kind": "agent", "owner_id": "conversation-b"})
	accepted, ok := acceptResult["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected accept result: %#v", acceptResult)
	}
	acceptedClaim, ok := accepted["claim"].(localworkspace.RunClaim)
	if !ok || acceptedClaim.Token == "" {
		t.Fatalf("accept did not return claim: %#v", accepted)
	}
	completeResult := callMCPToolForTest(t, r, "handoff_complete", map[string]any{"handoff_id": handoff.HandoffID, "claim_token": acceptedClaim.Token})
	completed, ok := completeResult["structuredContent"].(localworkspace.HandoffRecord)
	if !ok || completed.Status != "completed" {
		t.Fatalf("unexpected completed handoff: %#v", completeResult)
	}
	claimStatus, err := localworkspace.RunClaimStatus(root, run.RunID, now)
	if err != nil || claimStatus.Claimed {
		t.Fatalf("handoff completion did not release claim: err=%v status=%+v", err, claimStatus)
	}
}

func callMCPToolForTest(t *testing.T, root *Root, name string, arguments map[string]any) map[string]any {
	t.Helper()
	params, err := json.Marshal(map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		t.Fatal(err)
	}
	response := root.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("99"), Method: "tools/call", Params: params})
	result, ok := response.Result.(map[string]any)
	if response.Error != nil || !ok || result["isError"] != false {
		t.Fatalf("MCP tool %s failed: error=%+v result=%#v", name, response.Error, response.Result)
	}
	return result
}

func TestMCPMemoryToolsUseLocalScopeAndDoNotUploadFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-mcp-memory", ProjectID: "project-mcp-memory", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "30-knowledge", "pages", "mcp-fact.md"), []byte("MCP 记忆候选：品牌需要保持年轻白领语气。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	r := &Root{mcpCWD: root, now: func() time.Time { return now }}
	remembered := callMCPToolForTest(t, r, "memory_remember", map[string]any{"memory_id": "memr_mcp", "kind": "knowledge", "source_ref": "30-knowledge/pages/mcp-fact.md", "summary": "品牌语气优先保持年轻白领可读。", "formed_by": "workbuddy/test"})
	if report, ok := remembered["structuredContent"].(localworkspace.MemoryRememberReport); !ok || report.Record.MemoryID != "memr_mcp" {
		t.Fatalf("unexpected memory remember result: %#v", remembered)
	}
	rebuilt := callMCPToolForTest(t, r, "memory_rebuild", map[string]any{})
	if report, ok := rebuilt["structuredContent"].(localworkspace.MemoryRebuildReport); !ok || report.State != localworkspace.MemoryStateReady {
		t.Fatalf("unexpected memory rebuild result: %#v", rebuilt)
	}
	queried := callMCPToolForTest(t, r, "memory_query", map[string]any{"query": "年轻白领", "limit": 5, "max_chars": 2000})
	result, ok := queried["structuredContent"].(localworkspace.MemoryQueryResult)
	if !ok || result.Scope.ProjectID != "project-mcp-memory" || len(result.Candidates) == 0 || result.Candidates[0].SourceRef != "30-knowledge/pages/mcp-fact.md" {
		t.Fatalf("unexpected memory query result: %#v", queried)
	}
	status := callMCPToolForTest(t, r, "memory_status", map[string]any{})
	if value, ok := status["structuredContent"].(localworkspace.MemoryProjectionStatus); !ok || value.State != localworkspace.MemoryStateReady {
		t.Fatalf("unexpected memory status result: %#v", status)
	}
}

func TestMCPMemoryRemoteToolsUseExplicitAdapterAndRevalidateSource(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-mcp-remote", ProjectID: "project-mcp-remote", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "40-work", "remote.md"), []byte("远程抽取来源：记录年轻白领偏好。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var sourceDigest string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer mcp-remote-token" {
			http.Error(writer, "missing token", http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/memory/extract":
			var input localworkspace.MemoryRemoteExtractRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.SourceRef != "40-work/remote.md" || input.Content == "" {
				http.Error(writer, "bad extract request", http.StatusBadRequest)
				return
			}
			sourceDigest = input.SourceDigest
			_ = json.NewEncoder(writer).Encode(map[string]any{"candidates": []localworkspace.MemoryExtractedCandidate{{MemoryID: "mcp-remote-memory", Kind: "working", ClaimKey: "audience:young", Summary: "年轻白领偏好清晰直接的表达。"}}})
		case "/v1/memory/query":
			var input localworkspace.MemoryRemoteQueryRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.Scope.ProjectID != "project-mcp-remote" || input.Query != "年轻白领" {
				http.Error(writer, "bad query request", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"candidates": []localworkspace.MemoryCandidate{{SchemaVersion: localworkspace.MemoryEntrySchema, MemoryID: "mcp-remote-memory", Kind: "working", Scope: input.Scope, SourceRef: "40-work/remote.md", SourceDigest: sourceDigest, Summary: "年轻白领偏好清晰直接的表达。", Trust: "memory_candidate", Status: "active", FormedBy: "remote/test", ObservedAt: time.Now().UTC()}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("MCP_MEMORY_TOKEN", "mcp-remote-token")
	r := &Root{mcpCWD: root, now: func() time.Time { return time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC) }}
	extracted := callMCPToolForTest(t, r, "memory_extract", map[string]any{"endpoint": server.URL, "provider": "test", "token_env": "MCP_MEMORY_TOKEN", "source_refs": []string{"40-work/remote.md"}, "allow_private_networks": true, "allow_http": true})
	extraction, ok := extracted["structuredContent"].(localworkspace.MemoryExtractionReport)
	if !ok || extraction.CandidateCount != 1 || len(extraction.Remembered) != 1 || sourceDigest == "" {
		t.Fatalf("unexpected MCP memory extraction: %#v", extracted)
	}
	remote := callMCPToolForTest(t, r, "memory_remote_query", map[string]any{"endpoint": server.URL, "provider": "test", "token_env": "MCP_MEMORY_TOKEN", "query": "年轻白领", "limit": 5, "max_chars": 2000, "allow_private_networks": true, "allow_http": true})
	result, ok := remote["structuredContent"].(localworkspace.MemoryQueryResult)
	if !ok || result.Backend != localworkspace.MemoryRemoteBackendPrefix+"test" || len(result.Candidates) != 1 || result.Candidates[0].MemoryID != "mcp-remote-memory" {
		t.Fatalf("unexpected MCP remote memory query: %#v", remote)
	}
}

func mcpProjectViewURLForTest(t *testing.T, result map[string]any) *url.URL {
	t.Helper()
	content, ok := result["content"].([]map[string]any)
	if !ok || len(content) != 2 || content[1]["type"] != "resource_link" || content[1]["mimeType"] != "text/html" {
		t.Fatalf("tool result is missing its project resource link: %#v", result)
	}
	uri, ok := content[1]["uri"].(string)
	if !ok {
		t.Fatalf("tool resource link URI is invalid: %#v", content[1])
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("tool resource link URI is invalid: %v", err)
	}
	return parsed
}

func TestMCPProjectViewTargetSelectionDoesNotInventObjectPrecision(t *testing.T) {
	digestA := "sha256:" + strings.Repeat("a", 64)
	digestB := "sha256:" + strings.Repeat("b", 64)

	multipleFeedback := reviewFeedbackProjectView([]domain.ReviewFeedbackBundle{
		{SubmissionRevisionID: "revision-1", SubjectHash: digestA},
		{SubmissionRevisionID: "revision-2", SubjectHash: digestB},
	})
	if multipleFeedback.View != "tasks" || multipleFeedback.Focus != nil {
		t.Fatalf("multiple feedback revisions must use the generic review view: %#v", multipleFeedback)
	}

	invalidCurrentRevision := submissionStatusProjectView(app.SubmissionDetails{
		Submission: domain.Submission{CurrentRevisionID: "revision-1"},
		Revisions:  []domain.SubmissionRevision{{ID: "revision-1", ContentHash: "incomplete"}},
	})
	if invalidCurrentRevision.View != "tasks" || invalidCurrentRevision.Focus != nil {
		t.Fatalf("invalid current revision digest must not produce a focused link: %#v", invalidCurrentRevision)
	}

	multipleSnapshots := approvedSnapshotsProjectView([]domain.ApprovedSnapshot{
		{ID: "snapshot-1", ContentHash: digestA},
		{ID: "snapshot-2", ContentHash: digestB},
	})
	if multipleSnapshots.View != "deliveries" || multipleSnapshots.Focus != nil {
		t.Fatalf("multiple snapshots must use the generic delivery view: %#v", multipleSnapshots)
	}
}

func callMCPToolExpectErrorForTest(t *testing.T, root *Root, name string, arguments map[string]any) map[string]any {
	t.Helper()
	params, err := json.Marshal(map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		t.Fatal(err)
	}
	response := root.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("98"), Method: "tools/call", Params: params})
	result, ok := response.Result.(map[string]any)
	if response.Error != nil || !ok || result["isError"] != true {
		t.Fatalf("MCP tool %s did not return the expected tool error: error=%+v result=%#v", name, response.Error, response.Result)
	}
	return result
}

func TestPublishKnowledgeDryRunNeedsNoWorkspaceCredential(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: "http://localhost:8080", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	pack := filepath.Join(root, "30-knowledge", "packs", "knowledge.json")
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
	digest := "sha256:" + strings.Repeat("b", 64)
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
			data = map[string]any{"submission": map[string]any{"id": "submission-1", "status": "submitted", "current_revision_id": "revision-1"}, "revisions": []map[string]any{{"id": "revision-1", "content_hash": digest}}, "comments": []any{}}
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
		if index == 0 {
			link := mcpProjectViewURLForTest(t, result)
			if link.Path != "/studio/tasks" || link.Query().Get("project") != "project-1" || link.Query().Get("focus_id") != "revision-1" || link.Query().Get("expected_digest") != digest {
				t.Fatalf("submission_status did not attach the current immutable revision: %s", link)
			}
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
