package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/localworkspace"
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
	for _, name := range []string{"workspace.status", "workspace.doctor", "workspace.conversation-context", "workspace.approved.list", "workspace.approved.show", "mcp.status", "mcp.serve"} {
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
	for _, name := range []string{"contentcloud_workspace_conversation_context", "contentcloud_workspace_status", "environment_execution_plan", "environment_prepare_plan", "environment_prepare_apply", "workspace_status", "workspace_doctor", "source_register", "source_list", "source_ingest", "source_verify", "local_run_init", "local_run_show", "local_run_claim", "local_run_renew", "local_run_release", "handoff_create_ready", "handoff_list_ready", "handoff_accept", "handoff_complete", "handoff_supersede", "knowledge_import_candidates", "knowledge_lint", "knowledge_query", "knowledge_diagnose", "knowledge_pack", "brief_lint", "creative_batch_init", "script_lint", "creative_batch_lint", "creative_batch_finalize", "script_diff", "script_export", "publish_preflight", "publish_apply", "submission_status", "review_feedback_list", "review_feedback_pull", "review_feedback_inbox", "approved_snapshot_list", "approved_snapshot_pull", "approved_snapshot_inbox", "approved_snapshot_show"} {
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
	params, _ = json.Marshal(map[string]any{"name": "contentcloud_workspace_conversation_context", "arguments": map[string]any{}})
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
	basePath := filepath.Join(root, "work", "base-script.json")
	candidatePath := filepath.Join(root, "work", "candidate-script.json")
	if err := os.WriteFile(basePath, []byte(`{"id":"script-version:1","title":"原标题"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidatePath, []byte(`{"id":"script-version:2","based_on_version_id":"script-version:1","change_summary":"调整标题","title":"新标题"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	params, _ = json.Marshal(map[string]any{"name": "script_diff", "arguments": map[string]any{"directory": root, "baseline_file": "work/base-script.json", "candidate_file": "work/candidate-script.json", "allowed_paths": []string{"/title"}}})
	call = r.handleMCPRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "tools/call", Params: params})
	result, ok = call.Result.(map[string]any)
	if call.Error != nil || !ok || result["isError"] != false {
		t.Fatalf("script diff MCP tool failed: error=%+v result=%#v", call.Error, call.Result)
	}
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
	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: Version, Digest: manifest.Distribution.Plugins[0].Digest, Installed: true}}
	if _, err := localworkspace.StoreEnvironment(root, manifest, installed, manifestVerifier, now); err != nil {
		t.Fatal(err)
	}
	runtime := &Root{mcpCWD: root, now: func() time.Time { return now.Add(time.Minute) }, manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier)}
	arguments := map[string]any{"run_id": "run-1", "intent": "generate marketing video script", "required_capabilities": []string{domain.ScriptCapability}, "input_refs": []string{"outputs/briefs/brief.json"}}
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
	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: Version, Digest: manifest.Distribution.Plugins[0].Digest, Installed: true}}
	if _, err := localworkspace.StoreEnvironment(root, manifest, installed, manifestVerifier, now); err != nil {
		t.Fatal(err)
	}
	runner := &bootstrapRunner{responses: successfulTaskPackResponses()}
	runtime := &Root{mcpCWD: root, now: func() time.Time { return now.Add(time.Minute) }, codexRunner: runner, manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier)}
	arguments := map[string]any{"run_id": "run-pack-1", "intent": "generate visual storytelling assets", "required_capabilities": []string{"contentcloud.asset.generate"}, "input_refs": []string{"outputs/briefs/brief.json"}}
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
	applied := callMCPToolForTest(t, runtime, "environment_prepare_apply", confirmed)
	result, ok := applied["structuredContent"].(environmentPreparationApplyResult)
	if !ok || result.ExecutionPlan.State != "ready" || len(result.ExecutionPlan.Preparation) != 0 || len(result.InstalledPacks) != 1 || !result.InstalledPacks[0].Applied || result.BusinessFilesModified {
		t.Fatalf("preparation apply = %#v", applied)
	}
	if !result.Doctor.OK || !result.Handoff.RequiresNewChat || !strings.HasPrefix(result.Handoff.DeepLink, "codex://new?") || !strings.Contains(result.Handoff.RecoveryPrompt, "contentcloud-video-production@contentcloud") {
		t.Fatalf("preparation handoff = %#v", result.Handoff)
	}
	state, err := localworkspace.LoadEnvironment(root, manifestVerifier, now.Add(2*time.Minute))
	if err != nil || len(state.Lock.Plugins) != 2 || state.Lock.Plugins[1].ID != "contentcloud-visual-storytelling" {
		t.Fatalf("prepared environment = %#v, err = %v", state, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".contentcloud", "environment-preparation.lock")); !os.IsNotExist(err) {
		t.Fatalf("preparation lease remains after success: %v", err)
	}
	mutations := 0
	for _, call := range runner.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "plugin marketplace add") || strings.Contains(joined, "plugin remove") {
			t.Fatalf("task Pack preparation changed Marketplace or removed a plugin: %#v", runner.calls)
		}
		if joined == "codex plugin add contentcloud-visual-storytelling@contentcloud --json" {
			mutations++
		}
	}
	if mutations != 1 || len(runner.responses) != 0 {
		t.Fatalf("unexpected Codex transaction: mutations=%d remaining=%d calls=%#v", mutations, len(runner.responses), runner.calls)
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
	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: Version, Digest: manifest.Distribution.Plugins[0].Digest, Installed: true}}
	if _, err := localworkspace.StoreEnvironment(root, manifest, installed, manifestVerifier, now); err != nil {
		t.Fatal(err)
	}

	var planOutput, planErrors bytes.Buffer
	planner := &Root{stdout: &planOutput, stderr: &planErrors, now: func() time.Time { return now.Add(time.Minute) }, manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier)}
	planCommand := planner.command()
	planCommand.SetArgs([]string{"--json", "workspace", "prepare", "plan", "--directory", root, "--run", "run-cli-pack", "--intent", "generate visual storytelling assets", "--capability", "contentcloud.asset.generate", "--input", "outputs/briefs/brief.json"})
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
	applier := &Root{stdout: &applyOutput, stderr: &applyErrors, codexRunner: runner, now: func() time.Time { return now.Add(time.Minute) }, manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier)}
	applyCommand := applier.command()
	applyCommand.SetArgs([]string{"--json", "workspace", "prepare", "apply", "--directory", root, "--run", "run-cli-pack", "--intent", "generate visual storytelling assets", "--capability", "contentcloud.asset.generate", "--input", "outputs/briefs/brief.json", "--preparation-id", planEnvelope.Data.PreparationID, "--accept"})
	if err := applyCommand.Execute(); err != nil {
		t.Fatalf("workspace prepare apply failed: %v; stderr=%s", err, applyErrors.String())
	}
	var applyEnvelope struct {
		OK   bool                              `json:"ok"`
		Data environmentPreparationApplyResult `json:"data"`
	}
	if err := json.Unmarshal(applyOutput.Bytes(), &applyEnvelope); err != nil || !applyEnvelope.OK || applyEnvelope.Data.PreparationID != planEnvelope.Data.PreparationID || applyEnvelope.Data.ExecutionPlan.State != "ready" || !applyEnvelope.Data.Doctor.OK {
		t.Fatalf("workspace prepare apply output: err=%v output=%s", err, applyOutput.String())
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
	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: Version, Digest: manifest.Distribution.Plugins[0].Digest, Installed: true}}
	if _, err := localworkspace.StoreEnvironment(root, manifest, installed, manifestVerifier, now); err != nil {
		t.Fatal(err)
	}
	currentMarketplace := `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"v0.6.0"}}]}`
	missingPack := `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.6.0","installed":true,"enabled":true}],"available":[]}`
	runner := &bootstrapRunner{responses: []bootstrapRunnerResponse{
		{stdout: currentMarketplace}, {stdout: missingPack},
		{stdout: currentMarketplace}, {stdout: missingPack},
		{stdout: `{"pluginId":"contentcloud-visual-storytelling@contentcloud","name":"contentcloud-visual-storytelling","marketplaceName":"contentcloud","version":"1.2.0","installedPath":"/tmp/visual-storytelling"}`},
		{stdout: currentMarketplace}, {stdout: missingPack},
		{stdout: `{"removed":true}`},
	}}
	runtime := &Root{mcpCWD: root, codexRunner: runner, now: func() time.Time { return now.Add(time.Minute) }, manifestVerifierHook: fixedManifestVerifier(manifestVerifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier)}
	input := environmentPreparationInput{RunID: "run-pack-failure", Intent: "generate visual storytelling assets", Capabilities: []string{"contentcloud.asset.generate"}, InputRefs: []string{"outputs/briefs/brief.json"}}
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
	pluginRemovals := 0
	for _, call := range runner.calls {
		joined := strings.Join(call, " ")
		if joined == "codex plugin remove contentcloud-visual-storytelling@contentcloud --json" {
			pluginRemovals++
		}
		if strings.Contains(joined, "plugin marketplace remove") || strings.Contains(joined, "contentcloud-video-production@contentcloud") && strings.Contains(joined, "plugin remove") {
			t.Fatalf("failed preparation removed an unowned component: %#v", runner.calls)
		}
	}
	if pluginRemovals != 1 || len(runner.responses) != 0 {
		t.Fatalf("failed preparation rollback = removals %d remaining %d calls=%#v", pluginRemovals, len(runner.responses), runner.calls)
	}
	if _, err := os.Stat(filepath.Join(root, ".contentcloud", "environment-preparation.lock")); !os.IsNotExist(err) {
		t.Fatalf("failed preparation retained lease: %v", err)
	}
}

func successfulTaskPackResponses() []bootstrapRunnerResponse {
	currentMarketplace := `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"v0.6.0"}}]}`
	missingPack := `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.6.0","installed":true,"enabled":true}],"available":[]}`
	currentPack := `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.6.0","installed":true,"enabled":true},{"pluginId":"contentcloud-visual-storytelling@contentcloud","name":"contentcloud-visual-storytelling","marketplaceName":"contentcloud","version":"1.2.0","installed":true,"enabled":true}],"available":[]}`
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
	pack := filepath.Join(root, "knowledge", "packs", "knowledge.json")
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
	callMCPToolForTest(t, cloudConversation, "review_feedback_pull", map[string]any{})
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
	contextResult := callMCPToolForTest(t, offlineConversation, "contentcloud_workspace_conversation_context", map[string]any{})
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
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Command != "snapshot.workspace-list" {
			t.Fatalf("unexpected command: %s", payload.Command)
		}
		cloudReads++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"ok": true, "command": payload.Command, "data": snapshots}); err != nil {
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
	if cloudReads != 1 {
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
	run, err := localworkspace.InitLocalRun(localworkspace.InitLocalRunOptions{Root: root, RunID: "run-mcp-handoff", Intent: "content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := filepath.Join(root, "outputs", "scripts", "checkpoint.json")
	if err := os.WriteFile(checkpoint, []byte("{\"version\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Root{mcpCWD: root, now: func() time.Time { return now }}
	claimResult := callMCPToolForTest(t, r, "local_run_claim", map[string]any{"run_id": run.RunID, "owner": "conversation-a", "expected_revision": run.ContextRevision})
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
		"input_paths":        []string{"outputs/scripts/checkpoint.json"},
	})
	handoff, ok := handoffResult["structuredContent"].(localworkspace.HandoffRecord)
	if !ok || handoff.Status != "ready" {
		t.Fatalf("unexpected handoff result: %#v", handoffResult)
	}
	contextResult := callMCPToolForTest(t, r, "contentcloud_workspace_conversation_context", map[string]any{})
	conversation, ok := contextResult["structuredContent"].(localworkspace.WorkspaceConversationContext)
	if !ok || len(conversation.ReadyHandoffs) != 1 || conversation.ReadyHandoffs[0].HandoffID != handoff.HandoffID {
		t.Fatalf("handoff missing from conversation context: %#v", contextResult)
	}
	acceptResult := callMCPToolForTest(t, r, "handoff_accept", map[string]any{"handoff_id": handoff.HandoffID, "owner": "conversation-b"})
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
