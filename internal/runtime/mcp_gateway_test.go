package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
)

func activeGatewayFixture(t *testing.T) (*RuntimeMCPGateway, DispatchHandle, domain.StateCollection) {
	t.Helper()
	fake := agentadapter.NewFakeHarness()
	service, _, started := newDispatchRuntime(t, fake, time.Now)
	collection := stateCollectionForTest(started, started.Plan.Nodes[0].Key, "brief", "cas_map", 10)
	publishStateSchemaForTest(t, service, collection)
	if err := service.CreateStateCollection(t.Context(), collection); err != nil {
		t.Fatal(err)
	}
	input := dispatchInput(started.Job.ID)
	input.StateRefs = []string{collection.ID}
	input.AllowedTools = []string{ToolStateQuery, ToolStateMutate, ToolEffectPrepare}
	handle, err := service.PrepareRemoteDispatch(t.Context(), DispatchInput{
		TenantID: started.Job.TenantID, JobRunID: started.Job.ID, Owner: "worker-1", HarnessKind: "fake", Role: "node_executor", ExecutionProfileID: "profile-fake", StateRefs: input.StateRefs, AllowedTools: input.AllowedTools, MaxTokens: 1024, LeaseFor: time.Minute,
	}, mapCapabilities(fake))
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := fake.Start(t.Context(), agentadapter.StartAgentRequest{TenantID: started.Job.TenantID, JobRunID: handle.Attempt.JobRunID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID})
	if err != nil {
		t.Fatal(err)
	}
	active, err := service.ActivateDispatch(t.Context(), handle, session)
	if err != nil {
		t.Fatal(err)
	}
	return NewRuntimeMCPGateway(service), active, collection
}

func mapCapabilities(fake *agentadapter.FakeHarness) agentadapter.HarnessCapabilities {
	capabilities, _ := fake.Detect(nil)
	return capabilities
}

func TestRuntimeMCPGatewayBindsToolCallToFenceAndContext(t *testing.T) {
	gateway, handle, collection := activeGatewayFixture(t)
	request := GatewayRequest{TenantID: handle.Attempt.TenantID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, ToolName: ToolStateMutate, RequestID: "mcp-state-1", Arguments: map[string]any{"collection": collection.ID, "key": "topic", "value": map[string]any{"value": "春日"}}}
	first, err := gateway.Call(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ToolCall.State != domain.ToolCallSucceeded || first.Result["record"] == nil {
		t.Fatalf("MCP state mutation did not produce a terminal ToolCall: %#v", first)
	}
	replayed, err := gateway.Call(t.Context(), request)
	if err != nil || !replayed.Replayed || replayed.ToolCall.ID != first.ToolCall.ID || replayed.Result["record"] == nil {
		t.Fatalf("MCP idempotency replay was not stable: first=%#v replay=%#v err=%v", first, replayed, err)
	}
	query, err := gateway.Call(t.Context(), GatewayRequest{TenantID: request.TenantID, AttemptID: request.AttemptID, FenceToken: request.FenceToken, ToolName: ToolStateQuery, RequestID: "mcp-state-query-1", Arguments: map[string]any{"collection": collection.CollectionKey}})
	if err != nil {
		t.Fatal(err)
	}
	records, ok := query.Result["records"].([]domain.StateRecord)
	if !ok || len(records) != 1 || records[0].Key != "topic" {
		t.Fatalf("MCP state query returned unexpected records: %#v", query.Result)
	}
	if _, err := gateway.Call(t.Context(), GatewayRequest{TenantID: request.TenantID, AttemptID: request.AttemptID, FenceToken: "stale-fence", ToolName: ToolStateQuery, RequestID: "mcp-stale", Arguments: map[string]any{"collection": collection.ID}}); !hasDomainCode(err, "MCP_GATEWAY_FENCE_STALE") {
		t.Fatalf("stale MCP fence was accepted: %v", err)
	}
	if _, err := gateway.Call(t.Context(), GatewayRequest{TenantID: request.TenantID, AttemptID: request.AttemptID, FenceToken: request.FenceToken, ToolName: "provider.submit", RequestID: "mcp-unauthorized", Arguments: map[string]any{}}); !hasDomainCode(err, "MCP_GATEWAY_TOOL_NOT_ALLOWED") {
		t.Fatalf("unallowlisted MCP tool was accepted: %v", err)
	}
}

func TestRuntimeMCPGatewayTokenIsAttemptScopedAndRevokedAtTerminal(t *testing.T) {
	gateway, handle, collection := activeGatewayFixture(t)
	if !strings.HasPrefix(handle.GatewayToken, "rtg_") || handle.Attempt.GatewayTokenHash != domain.TokenHash(handle.GatewayToken) {
		t.Fatalf("prepare did not return the Attempt token matching the persisted hash")
	}
	response, err := gateway.CallWithToken(t.Context(), handle.GatewayToken, GatewayTokenRequest{ToolName: ToolStateQuery, RequestID: "rtg-query", Arguments: map[string]any{"collection": collection.ID}})
	if err != nil || response.ToolCall.State != domain.ToolCallSucceeded {
		t.Fatalf("valid Attempt token call failed: response=%#v err=%v", response, err)
	}
	if _, err := gateway.CallWithToken(t.Context(), "rtg_invalid", GatewayTokenRequest{ToolName: ToolStateQuery}); !hasDomainCode(err, "RUNTIME_GATEWAY_TOKEN_INVALID") {
		t.Fatalf("invalid token was accepted: %v", err)
	}
	if _, err := gateway.service.FinalizeDispatch(t.Context(), handle, DispatchOutcome{State: domain.RuntimeAttemptFailed, ErrorCode: "TEST_TERMINAL"}); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.CallWithToken(t.Context(), handle.GatewayToken, GatewayTokenRequest{ToolName: ToolStateQuery, RequestID: "rtg-after-terminal", Arguments: map[string]any{"collection": collection.ID}}); !hasDomainCode(err, "RUNTIME_GATEWAY_TOKEN_INVALID") {
		t.Fatalf("terminal Attempt token remained usable: %v", err)
	}
}

func TestRuntimeMCPGatewayEffectPreparationIsAttemptScoped(t *testing.T) {
	gateway, handle, _ := activeGatewayFixture(t)
	response, err := gateway.Call(t.Context(), GatewayRequest{TenantID: handle.Attempt.TenantID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, ToolName: ToolEffectPrepare, RequestID: "mcp-effect-1", Arguments: map[string]any{"kind": "provider.submit", "request_digest": "sha256:" + repeatGatewayHex(64, 'a'), "cost_minor": 100, "safe_summary": map[string]any{"provider": "fake"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.ToolCall.State != domain.ToolCallSucceeded || response.Result["effect_id"] == "" {
		t.Fatalf("effect.prepare did not persist an Effect: %#v", response)
	}
	if replay, err := gateway.Call(t.Context(), GatewayRequest{TenantID: handle.Attempt.TenantID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, ToolName: ToolEffectPrepare, RequestID: "mcp-effect-1", Arguments: map[string]any{"kind": "provider.submit", "request_digest": "sha256:" + repeatGatewayHex(64, 'a'), "cost_minor": 100, "safe_summary": map[string]any{"provider": "fake"}}}); err != nil || !replay.Replayed || replay.Result["effect_id"] != response.Result["effect_id"] {
		t.Fatalf("effect.prepare replay was not idempotent: %#v err=%v", replay, err)
	}
}

func TestRuntimeMCPGatewayFailedReplayPreservesTerminalError(t *testing.T) {
	gateway, handle, _ := activeGatewayFixture(t)
	request := GatewayRequest{TenantID: handle.Attempt.TenantID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, ToolName: ToolEffectPrepare, RequestID: "mcp-effect-invalid", Arguments: map[string]any{"kind": "provider.submit", "request_digest": "sha256:" + repeatGatewayHex(64, 'c'), "cost_minor": -1}}
	first, err := gateway.Call(t.Context(), request)
	if !hasDomainCode(err, "MCP_GATEWAY_EFFECT_COST_INVALID") || first.ToolCall.State != domain.ToolCallFailed {
		t.Fatalf("invalid effect did not persist the expected failed ToolCall: response=%#v err=%v", first, err)
	}
	replayed, err := gateway.Call(t.Context(), request)
	if !hasDomainCode(err, "MCP_GATEWAY_EFFECT_COST_INVALID") || !replayed.Replayed || replayed.ToolCall.ID != first.ToolCall.ID || replayed.ToolCall.State != domain.ToolCallFailed {
		t.Fatalf("failed MCP replay changed terminal semantics: first=%#v replay=%#v err=%v", first, replayed, err)
	}
}

func TestRuntimeMCPGatewayCommandsRecheckFenceInsideCommandStore(t *testing.T) {
	gateway, handle, collection := activeGatewayFixture(t)
	service := gateway.service
	record := stateRecordForTest(collection, "stale", "node:"+handle.Node.NodeKey)
	if _, err := service.StateRecordCASForAttempt(t.Context(), record, 0, handle.Attempt.ID, "stale-fence"); !hasDomainCode(err, "MCP_GATEWAY_FENCE_STALE") {
		t.Fatalf("state command accepted a stale fence: %v", err)
	}
	if _, err := service.RegisterEffectForAttempt(t.Context(), domain.ExternalEffect{TenantID: handle.Attempt.TenantID, JobRunID: handle.Attempt.JobRunID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID, Kind: "provider.submit", IdempotencyKey: "stale-effect", RequestDigest: "sha256:" + repeatGatewayHex(64, 'b'), Currency: "CNY", SafeSummary: map[string]any{}}, "stale-fence"); !hasDomainCode(err, "MCP_GATEWAY_FENCE_STALE") {
		t.Fatalf("effect command accepted a stale fence: %v", err)
	}
	if records, err := service.StateRecords(t.Context(), handle.Attempt.TenantID, collection.ID); err != nil || len(records) != 0 {
		t.Fatalf("stale fenced command changed state: %#v err=%v", records, err)
	}
	if effects, err := service.Effects(t.Context(), handle.Attempt.TenantID, handle.Attempt.JobRunID); err != nil || len(effects) != 0 {
		t.Fatalf("stale fenced command registered an Effect: %#v err=%v", effects, err)
	}
}

func repeatGatewayHex(count int, value byte) string {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return string(result)
}
