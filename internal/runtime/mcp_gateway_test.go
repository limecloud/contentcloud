package runtime_test

import (
	"context"
	"strings"
	"testing"
	"time"

	. "github.com/limecloud/contentcloud/internal/runtime"

	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

type gatewayFixture struct {
	gateway    *RuntimeMCPGateway
	service    *Service
	repository *memory.Store
	handle     DispatchHandle
	collection StateCollection
}

func activeGatewayFixture(t *testing.T) gatewayFixture {
	t.Helper()
	fake := agentadapter.NewFakeHarness()
	service, repository, started := newDispatchRuntime(t, fake, time.Now)
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
	return gatewayFixture{
		gateway: NewRuntimeMCPGateway(service), service: service, repository: repository,
		handle: active, collection: collection,
	}
}

func mapCapabilities(fake *agentadapter.FakeHarness) agentadapter.HarnessCapabilities {
	capabilities, _ := fake.Detect(nil)
	return capabilities
}

func TestRuntimeMCPGatewayBindsToolCallToFenceAndContext(t *testing.T) {
	fixture := activeGatewayFixture(t)
	gateway, handle, collection := fixture.gateway, fixture.handle, fixture.collection
	request := GatewayRequest{TenantID: handle.Attempt.TenantID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, ToolName: ToolStateMutate, RequestID: "mcp-state-1", Arguments: map[string]any{"collection": collection.ID, "key": "topic", "value": map[string]any{"value": "春日"}}}
	first, err := gateway.Call(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ToolCall.State != ToolCallSucceeded || first.Result["record"] == nil {
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
	records, ok := query.Result["records"].([]StateRecord)
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

func TestRuntimeMCPGatewayDoesNotReexecutePersistedRunningCall(t *testing.T) {
	fixture := activeGatewayFixture(t)
	handle, collection := fixture.handle, fixture.collection
	request := GatewayRequest{TenantID: handle.Attempt.TenantID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, ToolName: ToolStateQuery, RequestID: "mcp-running", Arguments: map[string]any{"collection": collection.ID}}
	idempotencyKey := request.RequestID
	requestDigest, err := stablehash.Sum(struct {
		ToolName       string         `json:"tool_name"`
		Arguments      map[string]any `json:"arguments"`
		IdempotencyKey string         `json:"idempotency_key"`
	}{request.ToolName, request.Arguments, idempotencyKey})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	call := ToolCall{
		ID: idgen.New(), TenantID: request.TenantID, JobRunID: handle.Attempt.JobRunID,
		NodeRunID: handle.Attempt.NodeRunID, AttemptID: handle.Attempt.ID, AgentInstanceID: handle.Attempt.AgentInstanceID,
		ToolName: request.ToolName, SchemaVersion: "contentcloud.tool/state.query/1", RequestDigest: "sha256:" + requestDigest,
		SafeRequest: map[string]any{"idempotency_key": idempotencyKey, "collection": collection.ID}, State: ToolCallRunning,
		StartedAt: &now, Version: 3, CreatedAt: now, UpdatedAt: now,
	}
	gateway := NewRuntimeMCPGateway(New(&runningToolCallRepository{Store: fixture.repository, call: call}, time.Now))
	response, err := gateway.Call(t.Context(), request)
	if !hasDomainCode(err, "MCP_GATEWAY_TOOL_CALL_IN_PROGRESS") || response.ToolCall.ID != call.ID || response.ToolCall.State != ToolCallRunning {
		t.Fatalf("persisted running ToolCall was not fenced: response=%#v err=%v", response, err)
	}
}

func TestRuntimeMCPGatewayTokenIsAttemptScopedAndRevokedAtTerminal(t *testing.T) {
	fixture := activeGatewayFixture(t)
	gateway, service, handle, collection := fixture.gateway, fixture.service, fixture.handle, fixture.collection
	if !strings.HasPrefix(handle.GatewayToken, "rtg_") || handle.Attempt.GatewayTokenHash != idgen.TokenHash(handle.GatewayToken) {
		t.Fatalf("prepare did not return the Attempt token matching the persisted hash")
	}
	response, err := gateway.CallWithToken(t.Context(), handle.GatewayToken, GatewayTokenRequest{ToolName: ToolStateQuery, RequestID: "rtg-query", Arguments: map[string]any{"collection": collection.ID}})
	if err != nil || response.ToolCall.State != ToolCallSucceeded {
		t.Fatalf("valid Attempt token call failed: response=%#v err=%v", response, err)
	}
	if _, err := gateway.CallWithToken(t.Context(), "rtg_invalid", GatewayTokenRequest{ToolName: ToolStateQuery}); !hasDomainCode(err, "RUNTIME_GATEWAY_TOKEN_INVALID") {
		t.Fatalf("invalid token was accepted: %v", err)
	}
	if _, err := service.FinalizeDispatch(t.Context(), handle, DispatchOutcome{State: RuntimeAttemptFailed, ErrorCode: "TEST_TERMINAL"}); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.CallWithToken(t.Context(), handle.GatewayToken, GatewayTokenRequest{ToolName: ToolStateQuery, RequestID: "rtg-after-terminal", Arguments: map[string]any{"collection": collection.ID}}); !hasDomainCode(err, "RUNTIME_GATEWAY_TOKEN_INVALID") {
		t.Fatalf("terminal Attempt token remained usable: %v", err)
	}
}

func TestRuntimeMCPGatewayEffectPreparationIsAttemptScoped(t *testing.T) {
	fixture := activeGatewayFixture(t)
	gateway, handle := fixture.gateway, fixture.handle
	response, err := gateway.Call(t.Context(), GatewayRequest{TenantID: handle.Attempt.TenantID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, ToolName: ToolEffectPrepare, RequestID: "mcp-effect-1", Arguments: map[string]any{"kind": "provider.submit", "request_digest": "sha256:" + repeatGatewayHex(64, 'a'), "cost_minor": 100, "safe_summary": map[string]any{"provider": "fake"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.ToolCall.State != ToolCallSucceeded || response.Result["effect_id"] == "" {
		t.Fatalf("effect.prepare did not persist an Effect: %#v", response)
	}
	if replay, err := gateway.Call(t.Context(), GatewayRequest{TenantID: handle.Attempt.TenantID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, ToolName: ToolEffectPrepare, RequestID: "mcp-effect-1", Arguments: map[string]any{"kind": "provider.submit", "request_digest": "sha256:" + repeatGatewayHex(64, 'a'), "cost_minor": 100, "safe_summary": map[string]any{"provider": "fake"}}}); err != nil || !replay.Replayed || replay.Result["effect_id"] != response.Result["effect_id"] {
		t.Fatalf("effect.prepare replay was not idempotent: %#v err=%v", replay, err)
	}
}

func TestRuntimeMCPGatewayFailedReplayPreservesTerminalError(t *testing.T) {
	fixture := activeGatewayFixture(t)
	gateway, handle := fixture.gateway, fixture.handle
	request := GatewayRequest{TenantID: handle.Attempt.TenantID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, ToolName: ToolEffectPrepare, RequestID: "mcp-effect-invalid", Arguments: map[string]any{"kind": "provider.submit", "request_digest": "sha256:" + repeatGatewayHex(64, 'c'), "cost_minor": -1}}
	first, err := gateway.Call(t.Context(), request)
	if !hasDomainCode(err, "MCP_GATEWAY_EFFECT_COST_INVALID") || first.ToolCall.State != ToolCallFailed {
		t.Fatalf("invalid effect did not persist the expected failed ToolCall: response=%#v err=%v", first, err)
	}
	replayed, err := gateway.Call(t.Context(), request)
	if !hasDomainCode(err, "MCP_GATEWAY_EFFECT_COST_INVALID") || !replayed.Replayed || replayed.ToolCall.ID != first.ToolCall.ID || replayed.ToolCall.State != ToolCallFailed {
		t.Fatalf("failed MCP replay changed terminal semantics: first=%#v replay=%#v err=%v", first, replayed, err)
	}
}

func TestRuntimeMCPGatewayCommandsRecheckFenceInsideCommandStore(t *testing.T) {
	fixture := activeGatewayFixture(t)
	service, handle, collection := fixture.service, fixture.handle, fixture.collection
	record := stateRecordForTest(collection, "stale", "node:"+handle.Node.NodeKey)
	if _, err := service.StateRecordCASForAttempt(t.Context(), record, 0, handle.Attempt.ID, "stale-fence"); !hasDomainCode(err, "MCP_GATEWAY_FENCE_STALE") {
		t.Fatalf("state command accepted a stale fence: %v", err)
	}
	if _, err := service.RegisterEffectForAttempt(t.Context(), ExternalEffect{TenantID: handle.Attempt.TenantID, JobRunID: handle.Attempt.JobRunID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID, Kind: "provider.submit", IdempotencyKey: "stale-effect", RequestDigest: "sha256:" + repeatGatewayHex(64, 'b'), Currency: "CNY", SafeSummary: map[string]any{}}, "stale-fence"); !hasDomainCode(err, "MCP_GATEWAY_FENCE_STALE") {
		t.Fatalf("effect command accepted a stale fence: %v", err)
	}
	if records, err := service.StateRecords(t.Context(), handle.Attempt.TenantID, collection.ID); err != nil || len(records) != 0 {
		t.Fatalf("stale fenced command changed state: %#v err=%v", records, err)
	}
	if effects, err := service.Effects(t.Context(), handle.Attempt.TenantID, handle.Attempt.JobRunID); err != nil || len(effects) != 0 {
		t.Fatalf("stale fenced command registered an Effect: %#v err=%v", effects, err)
	}
}

type runningToolCallRepository struct {
	*memory.Store
	call ToolCall
}

func (r *runningToolCallRepository) ToolCallByIdempotencyKey(context.Context, string, string, string, string) (ToolCall, error) {
	return r.call, nil
}

func repeatGatewayHex(count int, value byte) string {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return string(result)
}
