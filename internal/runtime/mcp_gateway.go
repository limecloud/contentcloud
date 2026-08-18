package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

const (
	ToolStateGet      = "state.get"
	ToolStateQuery    = "state.query"
	ToolStateMutate   = "state.mutate"
	ToolChildList     = "child.list"
	ToolEffectPrepare = "effect.prepare"
	ToolEffectStatus  = "effect.status"
	maxGatewayArgs    = 64 << 10
	maxGatewayResult  = 256 << 10
)

type GatewayRequest struct {
	TenantID   string         `json:"tenant_id"`
	AttemptID  string         `json:"attempt_id"`
	FenceToken string         `json:"fence_token"`
	ToolName   string         `json:"tool_name"`
	RequestID  string         `json:"request_id"`
	Arguments  map[string]any `json:"arguments"`
}

// GatewayTokenRequest is the untrusted HTTP/MCP input. Attempt identity,
// tenant scope and fence are resolved exclusively from the hashed rtg_ token.
type GatewayTokenRequest struct {
	ToolName  string         `json:"tool_name"`
	RequestID string         `json:"request_id"`
	Arguments map[string]any `json:"arguments"`
}

type GatewayResponse struct {
	ToolCall ToolCall       `json:"tool_call"`
	Result   map[string]any `json:"result,omitempty"`
	Replayed bool           `json:"replayed,omitempty"`
}

type gatewayContext struct {
	attempt RuntimeAttempt
	node    NodeRun
	agent   AgentInstance
	view    ContextView
}

// RuntimeMCPGateway is the server-owned MCP tool boundary. Harnesses may
// expose this service through stdio or Streamable HTTP, but they never receive
// a database handle or a broad Store interface.
type RuntimeMCPGateway struct {
	service *Service
}

func NewRuntimeMCPGateway(service *Service) *RuntimeMCPGateway {
	return &RuntimeMCPGateway{service: service}
}

func (g *RuntimeMCPGateway) CallWithToken(ctx context.Context, token string, request GatewayTokenRequest) (GatewayResponse, error) {
	if g == nil || g.service == nil || g.service.repo == nil {
		return GatewayResponse{}, fault.Policy("MCP_GATEWAY_UNAVAILABLE", "Runtime MCP Gateway 尚未配置", "联系平台运营人员启用 Runtime Gateway")
	}
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "rtg_") || len(token) < 32 {
		return GatewayResponse{}, fault.E("authentication", "runtime_gateway", "RUNTIME_GATEWAY_TOKEN_INVALID", "Runtime Gateway 凭据无效", 3)
	}
	attempt, err := g.service.repo.RuntimeAttemptByGatewayTokenHash(ctx, idgen.TokenHash(token))
	if err != nil {
		return GatewayResponse{}, fault.E("authentication", "runtime_gateway", "RUNTIME_GATEWAY_TOKEN_INVALID", "Runtime Gateway 凭据无效", 3)
	}
	now := g.service.now().UTC()
	if attempt.GatewayTokenHash != idgen.TokenHash(token) || attempt.GatewayExpiresAt == nil || !attempt.GatewayExpiresAt.After(now) || (attempt.State != RuntimeAttemptPrepared && attempt.State != RuntimeAttemptRunning) || attempt.LeaseExpiresAt == nil || !attempt.LeaseExpiresAt.After(now) {
		return GatewayResponse{}, fault.E("authentication", "runtime_gateway", "RUNTIME_GATEWAY_TOKEN_INVALID", "Runtime Gateway 凭据无效或已过期", 3)
	}
	if attempt.State == RuntimeAttemptPrepared {
		err := fault.Conflict("MCP_GATEWAY_NOT_ACTIVE", "Runtime Attempt 尚未完成 Agent 会话激活")
		err.Retryable = true
		err.Hint = "等待 worker 完成 activate 后重试"
		return GatewayResponse{}, err
	}
	return g.Call(ctx, GatewayRequest{
		TenantID: attempt.TenantID, AttemptID: attempt.ID, FenceToken: attempt.FenceToken,
		ToolName: request.ToolName, RequestID: request.RequestID, Arguments: request.Arguments,
	})
}

func (g *RuntimeMCPGateway) Call(ctx context.Context, request GatewayRequest) (GatewayResponse, error) {
	if g == nil || g.service == nil || g.service.repo == nil {
		return GatewayResponse{}, fault.Policy("MCP_GATEWAY_UNAVAILABLE", "Runtime MCP Gateway 尚未配置", "联系平台运营人员启用 Runtime Gateway")
	}
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.AttemptID = strings.TrimSpace(request.AttemptID)
	request.FenceToken = strings.TrimSpace(request.FenceToken)
	request.ToolName = strings.TrimSpace(request.ToolName)
	request.RequestID = strings.TrimSpace(request.RequestID)
	if request.TenantID == "" || request.AttemptID == "" || request.FenceToken == "" || request.ToolName == "" {
		return GatewayResponse{}, fault.Invalid("MCP_GATEWAY_INPUT_INVALID", "MCP 调用缺少租户、Attempt、fence 或工具名")
	}
	if request.Arguments == nil {
		request.Arguments = map[string]any{}
	}
	if body, err := json.Marshal(request.Arguments); err != nil || len(body) > maxGatewayArgs {
		return GatewayResponse{}, fault.Policy("MCP_GATEWAY_ARGUMENTS_TOO_LARGE", "MCP 工具参数超过大小限制", "使用 ArtifactRef 或缩小参数")
	}
	state, err := g.authorize(ctx, request)
	if err != nil {
		return GatewayResponse{}, err
	}
	idempotencyKey, err := gatewayIdempotencyKey(request)
	if err != nil {
		return GatewayResponse{}, err
	}
	requestDigest, err := stablehash.Sum(struct {
		ToolName       string         `json:"tool_name"`
		Arguments      map[string]any `json:"arguments"`
		IdempotencyKey string         `json:"idempotency_key"`
	}{request.ToolName, request.Arguments, idempotencyKey})
	if err != nil {
		return GatewayResponse{}, err
	}
	requestDigest = "sha256:" + requestDigest
	previous, lookupErr := g.findExisting(ctx, request.TenantID, state.attempt.ID, request.ToolName, idempotencyKey)
	hasPrevious := lookupErr == nil
	if lookupErr != nil && !fault.IsNotFound(lookupErr) {
		return GatewayResponse{}, lookupErr
	}
	if hasPrevious {
		if previous.RequestDigest != requestDigest {
			return GatewayResponse{}, fault.Conflict("MCP_GATEWAY_IDEMPOTENCY_MISMATCH", "MCP 幂等键已用于不同的工具参数")
		}
		if previous.State == ToolCallSucceeded || previous.State == ToolCallFailed || previous.State == ToolCallUnknown {
			return gatewayTerminalReplay(previous)
		}
	}
	now := g.service.now().UTC()
	call := previous
	if !hasPrevious {
		call = ToolCall{
			ID: idgen.New(), TenantID: request.TenantID, JobRunID: state.attempt.JobRunID,
			NodeRunID: state.attempt.NodeRunID, AttemptID: state.attempt.ID, AgentInstanceID: state.attempt.AgentInstanceID,
			ToolName: request.ToolName, SchemaVersion: gatewayToolSchema(request.ToolName), RequestDigest: requestDigest,
			SafeRequest: gatewaySafeRequest(request.ToolName, request.Arguments, idempotencyKey), State: ToolCallProposed, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := g.service.CreateFencedToolCall(ctx, call, request.FenceToken); err != nil {
			concurrent, concurrentErr := g.findExisting(ctx, request.TenantID, state.attempt.ID, request.ToolName, idempotencyKey)
			if concurrentErr != nil {
				return GatewayResponse{}, err
			}
			if concurrent.RequestDigest != requestDigest {
				return GatewayResponse{}, fault.Conflict("MCP_GATEWAY_IDEMPOTENCY_MISMATCH", "MCP 幂等键已用于不同的工具参数")
			}
			call = concurrent
			if call.State == ToolCallSucceeded || call.State == ToolCallFailed || call.State == ToolCallUnknown {
				return gatewayTerminalReplay(call)
			}
		}
	}
	claimedRunning := false
	if call.State == ToolCallProposed {
		authorized := call
		authorized.State = ToolCallAuthorized
		authorized.Version++
		authorized.UpdatedAt = g.service.now().UTC()
		call, err = g.service.TransitionFencedToolCall(ctx, authorized, call.Version, request.FenceToken)
		if err != nil {
			return GatewayResponse{}, err
		}
	}
	if call.State == ToolCallAuthorized {
		running := call
		running.State = ToolCallRunning
		started := g.service.now().UTC()
		running.StartedAt = &started
		running.Version++
		running.UpdatedAt = started
		claimed, transitionErr := g.service.TransitionFencedToolCall(ctx, running, call.Version, request.FenceToken)
		if transitionErr != nil {
			// A competing request may have won the authorized -> running CAS.
			// Never execute again merely because the row is already running.
			current, lookupErr := g.findExisting(ctx, request.TenantID, state.attempt.ID, request.ToolName, idempotencyKey)
			if lookupErr == nil && (current.State == ToolCallSucceeded || current.State == ToolCallFailed || current.State == ToolCallUnknown) {
				return gatewayTerminalReplay(current)
			}
			conflict := fault.Conflict("MCP_GATEWAY_TOOL_CALL_IN_PROGRESS", "相同幂等请求正在执行")
			conflict.Retryable = true
			return GatewayResponse{ToolCall: current}, conflict
		}
		call = claimed
		claimedRunning = true
	} else if call.State != ToolCallRunning {
		return GatewayResponse{}, fault.Conflict("MCP_GATEWAY_TOOL_CALL_STATE_INVALID", "MCP ToolCall 当前状态不能恢复执行")
	}
	if call.State == ToolCallRunning && !claimedRunning && hasPrevious {
		// A persisted running call has no proof that this process owns the
		// external execution. Reconciliation, rather than blind replay, must
		// decide whether it can be resumed.
		conflict := fault.Conflict("MCP_GATEWAY_TOOL_CALL_IN_PROGRESS", "相同幂等请求正在执行")
		conflict.Retryable = true
		return GatewayResponse{ToolCall: call}, conflict
	}
	result, execErr := g.execute(ctx, state, request.ToolName, request.Arguments, idempotencyKey)
	if execErr != nil {
		failed := call
		failed.State = ToolCallFailed
		failed.ErrorCode = gatewayErrorCode(execErr)
		failed.Version++
		failed.UpdatedAt = g.service.now().UTC()
		if terminal, transitionErr := g.service.TransitionFencedToolCall(ctx, failed, call.Version, request.FenceToken); transitionErr == nil {
			call = terminal
		}
		return GatewayResponse{ToolCall: call}, execErr
	}
	if body, marshalErr := json.Marshal(result); marshalErr != nil || len(body) > maxGatewayResult {
		failed := call
		failed.State = ToolCallFailed
		failed.ErrorCode = "MCP_GATEWAY_RESULT_TOO_LARGE"
		failed.Version++
		failed.UpdatedAt = g.service.now().UTC()
		if terminal, transitionErr := g.service.TransitionFencedToolCall(ctx, failed, call.Version, request.FenceToken); transitionErr == nil {
			call = terminal
		}
		return GatewayResponse{ToolCall: call}, fault.Policy("MCP_GATEWAY_RESULT_TOO_LARGE", "MCP 工具结果超过大小限制", "使用分页或 ArtifactRef")
	}
	resultDigest, err := stablehash.Sum(result)
	if err != nil {
		return GatewayResponse{}, err
	}
	succeeded := call
	succeeded.State = ToolCallSucceeded
	succeeded.SafeResult = cloneGatewayResult(result)
	succeeded.ResultDigest = "sha256:" + resultDigest
	succeeded.Version++
	succeeded.UpdatedAt = g.service.now().UTC()
	call, err = g.service.TransitionFencedToolCall(ctx, succeeded, call.Version, request.FenceToken)
	if err != nil {
		return GatewayResponse{}, err
	}
	return GatewayResponse{ToolCall: call, Result: result}, nil
}

func (g *RuntimeMCPGateway) authorize(ctx context.Context, request GatewayRequest) (gatewayContext, error) {
	repo := g.service.repo
	attempt, err := repo.RuntimeAttempt(ctx, request.TenantID, request.AttemptID)
	if err != nil {
		return gatewayContext{}, err
	}
	node, err := repo.NodeRun(ctx, request.TenantID, attempt.NodeRunID)
	if err != nil {
		return gatewayContext{}, err
	}
	agent, err := repo.AgentInstance(ctx, request.TenantID, attempt.AgentInstanceID)
	if err != nil {
		return gatewayContext{}, err
	}
	view, err := repo.ContextView(ctx, request.TenantID, attempt.ContextViewID)
	if err != nil {
		return gatewayContext{}, err
	}
	now := g.service.now().UTC()
	if attempt.State != RuntimeAttemptRunning || attempt.FenceToken != request.FenceToken || attempt.LeaseExpiresAt == nil || !attempt.LeaseExpiresAt.After(now) {
		return gatewayContext{}, fault.Conflict("MCP_GATEWAY_FENCE_STALE", "MCP 调用的 Attempt fence 或租约已失效")
	}
	if attempt.TenantID != request.TenantID || node.TenantID != request.TenantID || node.JobRunID != attempt.JobRunID || agent.TenantID != request.TenantID || agent.JobRunID != attempt.JobRunID || agent.NodeRunID != node.ID || agent.ContextViewID != view.ID || view.TenantID != request.TenantID || view.JobRunID != attempt.JobRunID || view.NodeRunID != node.ID || view.AttemptID != attempt.ID {
		return gatewayContext{}, fault.Invalid("MCP_GATEWAY_SCOPE_INVALID", "MCP 调用没有绑定同一租户、Job、Node、Attempt、Agent 和 ContextView")
	}
	if agent.State != AgentActive {
		return gatewayContext{}, fault.Conflict("MCP_GATEWAY_AGENT_NOT_ACTIVE", "只有活动中的 AgentInstance 可以调用 MCP 工具")
	}
	if !view.ExpiresAt.After(now) {
		return gatewayContext{}, fault.Policy("MCP_GATEWAY_CONTEXT_EXPIRED", "MCP 调用的 ContextView 已过期", "重新创建执行尝试和 ContextView")
	}
	if !view.AllowsTool(request.ToolName) {
		return gatewayContext{}, fault.Policy("MCP_GATEWAY_TOOL_NOT_ALLOWED", "当前 Attempt 未授权该 MCP 工具", "仅调用 ContextView AllowedTools 中的工具")
	}
	if gatewayToolSchema(request.ToolName) == "" {
		return gatewayContext{}, fault.Invalid("MCP_GATEWAY_TOOL_UNSUPPORTED", "Runtime MCP Gateway 不支持该工具")
	}
	return gatewayContext{attempt: attempt, node: node, agent: agent, view: view}, nil
}

func (g *RuntimeMCPGateway) execute(ctx context.Context, state gatewayContext, toolName string, arguments map[string]any, idempotencyKey string) (map[string]any, error) {
	switch toolName {
	case ToolStateGet, ToolStateQuery:
		return g.queryState(ctx, state, arguments)
	case ToolStateMutate:
		return g.mutateState(ctx, state, arguments, idempotencyKey)
	case ToolEffectPrepare:
		return g.prepareEffect(ctx, state, arguments, idempotencyKey)
	case ToolChildList:
		return g.listChildren(ctx, state)
	case ToolEffectStatus:
		return g.effectStatus(ctx, state, arguments)
	default:
		return nil, fault.Invalid("MCP_GATEWAY_TOOL_UNSUPPORTED", "Runtime MCP Gateway 不支持该工具")
	}
}

func (g *RuntimeMCPGateway) queryState(ctx context.Context, state gatewayContext, arguments map[string]any) (map[string]any, error) {
	collection, err := g.resolveCollection(ctx, state, stringArgument(arguments, "collection"))
	if err != nil {
		return nil, err
	}
	if !stateRefAllowed(state.view, collection) {
		return nil, fault.Policy("MCP_GATEWAY_STATE_NOT_ALLOWED", "ContextView 未授权读取该状态集合", "仅读取 ContextView StateRefs 中的集合")
	}
	records, err := g.service.repo.StateRecords(ctx, state.attempt.TenantID, collection.ID)
	if err != nil {
		return nil, err
	}
	key := stringArgument(arguments, "key")
	afterKey := stringArgument(arguments, "after_key")
	limit := intArgument(arguments, "limit", 50)
	if limit < 1 || limit > 100 {
		return nil, fault.Invalid("MCP_GATEWAY_LIMIT_INVALID", "state.query 的 limit 必须在 1 到 100 之间")
	}
	result := make([]StateRecord, 0, limit)
	for _, record := range records {
		if key != "" && record.Key != key || afterKey != "" && record.Key <= afterKey {
			continue
		}
		result = append(result, record)
		if len(result) == limit {
			break
		}
	}
	return map[string]any{"collection_id": collection.ID, "collection": collection.CollectionKey, "revision": collection.Revision, "watermark": collection.Watermark, "records": result}, nil
}

func (g *RuntimeMCPGateway) mutateState(ctx context.Context, state gatewayContext, arguments map[string]any, idempotencyKey string) (map[string]any, error) {
	collection, err := g.resolveCollection(ctx, state, stringArgument(arguments, "collection"))
	if err != nil {
		return nil, err
	}
	if !stateRefAllowed(state.view, collection) {
		return nil, fault.Policy("MCP_GATEWAY_STATE_NOT_ALLOWED", "ContextView 未授权写入该状态集合", "仅写入 ContextView StateRefs 中的集合")
	}
	key := stringArgument(arguments, "key")
	if key == "" {
		return nil, fault.Invalid("MCP_GATEWAY_STATE_KEY_REQUIRED", "state.mutate 需要记录键")
	}
	value, _ := arguments["value"].(map[string]any)
	artifactRef := stringArgument(arguments, "artifact_ref")
	if value == nil && artifactRef == "" {
		return nil, fault.Invalid("MCP_GATEWAY_STATE_VALUE_REQUIRED", "state.mutate 需要对象值或 ArtifactRef")
	}
	expectedVersion := intArgument(arguments, "expected_version", 0)
	actor := "node:" + state.node.NodeKey
	desiredDigest := "sha256:" + mustCanonicalHash(struct {
		CollectionID string         `json:"collection_id"`
		Key          string         `json:"key"`
		Value        map[string]any `json:"value,omitempty"`
		ArtifactRef  string         `json:"artifact_ref,omitempty"`
	}{collection.ID, key, value, artifactRef})
	createdAt := g.service.now().UTC()
	recordID, createdBy := idgen.New(), actor
	records, err := g.service.repo.StateRecords(ctx, state.attempt.TenantID, collection.ID)
	if err != nil {
		return nil, err
	}
	for _, existing := range records {
		if existing.Key != key {
			continue
		}
		if existing.Version == expectedVersion+1 && existing.Digest == desiredDigest {
			return map[string]any{"record": existing, "idempotency_key": idempotencyKey}, nil
		}
		recordID, createdBy, createdAt = existing.ID, existing.CreatedBy, existing.CreatedAt
		break
	}
	record := StateRecord{ID: recordID, TenantID: state.attempt.TenantID, CollectionID: collection.ID, Key: key, Value: value, ArtifactRef: artifactRef, SchemaRevision: collection.SchemaRevision, Digest: desiredDigest, CreatedBy: createdBy, UpdatedBy: actor, Version: expectedVersion + 1, CreatedAt: createdAt, UpdatedAt: g.service.now().UTC()}
	stored, err := g.service.StateRecordCASForAttempt(ctx, record, expectedVersion, state.attempt.ID, state.attempt.FenceToken)
	if err != nil {
		return nil, err
	}
	return map[string]any{"record": stored, "idempotency_key": idempotencyKey}, nil
}

func (g *RuntimeMCPGateway) prepareEffect(ctx context.Context, state gatewayContext, arguments map[string]any, idempotencyKey string) (map[string]any, error) {
	kind := stringArgument(arguments, "kind")
	requestDigest := stringArgument(arguments, "request_digest")
	if kind == "" || requestDigest == "" {
		return nil, fault.Invalid("MCP_GATEWAY_EFFECT_INPUT_INVALID", "effect.prepare 需要 kind 和 request_digest")
	}
	costMinor := int64Argument(arguments, "cost_minor", 0)
	if costMinor < 0 {
		return nil, fault.Invalid("MCP_GATEWAY_EFFECT_COST_INVALID", "effect.prepare 的费用不能为负数")
	}
	currency := stringArgument(arguments, "currency")
	if currency == "" {
		currency = "CNY"
	}
	summary, _ := arguments["safe_summary"].(map[string]any)
	if summary == nil {
		summary = map[string]any{}
	}
	effect, err := g.service.RegisterEffectForAttempt(ctx, ExternalEffect{TenantID: state.attempt.TenantID, JobRunID: state.attempt.JobRunID, NodeRunID: state.attempt.NodeRunID, AttemptID: state.attempt.ID, Kind: kind, IdempotencyKey: idempotencyKey, RequestDigest: requestDigest, CostMinor: costMinor, Currency: currency, SafeSummary: summary}, state.attempt.FenceToken)
	if err != nil {
		return nil, err
	}
	return map[string]any{"effect_id": effect.ID, "state": effect.State, "request_digest": effect.RequestDigest}, nil
}

func (g *RuntimeMCPGateway) listChildren(ctx context.Context, state gatewayContext) (map[string]any, error) {
	job, err := g.service.repo.JobRun(ctx, state.attempt.TenantID, state.attempt.JobRunID)
	if err != nil {
		return nil, err
	}
	plan, err := g.service.repo.Plan(ctx, state.attempt.TenantID, job.PlanRevisionID)
	if err != nil {
		return nil, err
	}
	nodes, err := g.service.repo.NodeRuns(ctx, state.attempt.TenantID, state.attempt.JobRunID)
	if err != nil {
		return nil, err
	}
	children := make([]NodeRun, 0)
	for _, spec := range plan.Nodes {
		if !contains(spec.DependsOn, state.node.NodeKey) {
			continue
		}
		for _, node := range nodes {
			if node.NodeKey == spec.Key {
				children = append(children, node)
				break
			}
		}
	}
	return map[string]any{"nodes": children}, nil
}

func (g *RuntimeMCPGateway) effectStatus(ctx context.Context, state gatewayContext, arguments map[string]any) (map[string]any, error) {
	effectID := stringArgument(arguments, "effect_id")
	if effectID == "" {
		return nil, fault.Invalid("MCP_GATEWAY_EFFECT_ID_REQUIRED", "effect.status 需要 effect_id")
	}
	effect, err := g.service.repo.Effect(ctx, state.attempt.TenantID, effectID)
	if err != nil {
		return nil, err
	}
	if effect.AttemptID != state.attempt.ID || effect.JobRunID != state.attempt.JobRunID || effect.NodeRunID != state.node.ID {
		return nil, fault.Policy("MCP_GATEWAY_EFFECT_SCOPE_INVALID", "外部操作不属于当前 Attempt", "只能查询当前 Attempt 声明的外部操作")
	}
	return map[string]any{"effect_id": effect.ID, "state": effect.State, "external_id": effect.ExternalID, "response_digest": effect.ResponseDigest, "safe_summary": effect.SafeSummary, "error_code": effect.ErrorCode}, nil
}

func (g *RuntimeMCPGateway) resolveCollection(ctx context.Context, state gatewayContext, reference string) (StateCollection, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return StateCollection{}, fault.Invalid("MCP_GATEWAY_COLLECTION_REQUIRED", "状态工具需要 collection")
	}
	collections, err := g.service.repo.StateCollections(ctx, state.attempt.TenantID, state.attempt.JobRunID)
	if err != nil {
		return StateCollection{}, err
	}
	for _, collection := range collections {
		if collection.ID == reference || collection.CollectionKey == reference {
			return collection, nil
		}
	}
	return StateCollection{}, fault.NotFound("状态集合")
}

func stateRefAllowed(view ContextView, collection StateCollection) bool {
	for _, ref := range view.StateRefs {
		if strings.TrimSpace(ref) == collection.ID || strings.TrimSpace(ref) == collection.CollectionKey {
			return true
		}
	}
	return false
}

func (g *RuntimeMCPGateway) findExisting(ctx context.Context, tenantID, attemptID, toolName, idempotencyKey string) (ToolCall, error) {
	return g.service.repo.ToolCallByIdempotencyKey(ctx, tenantID, attemptID, toolName, idempotencyKey)
}

func gatewayTerminalReplay(call ToolCall) (GatewayResponse, error) {
	response := GatewayResponse{ToolCall: call, Replayed: true, Result: cloneGatewayResult(call.SafeResult)}
	switch call.State {
	case ToolCallSucceeded:
		return response, nil
	case ToolCallFailed:
		code := strings.TrimSpace(call.ErrorCode)
		if code == "" {
			code = "MCP_GATEWAY_TOOL_CALL_FAILED"
		}
		return response, fault.Conflict(code, "MCP 工具调用此前已失败；相同幂等请求不会重复执行")
	case ToolCallUnknown:
		code := strings.TrimSpace(call.ErrorCode)
		if code == "" {
			code = "MCP_GATEWAY_RESULT_UNKNOWN"
		}
		return response, fault.Conflict(code, "MCP 工具调用结果尚未确认；相同幂等请求不会重复执行")
	default:
		return GatewayResponse{}, fault.Conflict("MCP_GATEWAY_TOOL_CALL_STATE_INVALID", "MCP ToolCall 尚未进入可重放终态")
	}
}

func cloneGatewayResult(result map[string]any) map[string]any {
	if result == nil {
		return nil
	}
	body, err := json.Marshal(result)
	if err != nil {
		return nil
	}
	var clone map[string]any
	if err := json.Unmarshal(body, &clone); err != nil {
		return nil
	}
	return clone
}

func gatewayIdempotencyKey(request GatewayRequest) (string, error) {
	key := stringArgument(request.Arguments, "idempotency_key")
	if key == "" {
		key = request.RequestID
	}
	if key == "" {
		return "", fault.Invalid("MCP_GATEWAY_IDEMPOTENCY_REQUIRED", "MCP 调用需要 request_id 或 idempotency_key")
	}
	if len(key) > 200 {
		return "", fault.Invalid("MCP_GATEWAY_IDEMPOTENCY_INVALID", "MCP 幂等键过长")
	}
	return key, nil
}

func gatewayToolSchema(toolName string) string {
	switch toolName {
	case ToolStateGet:
		return "contentcloud.tool/state.get/1"
	case ToolStateQuery:
		return "contentcloud.tool/state.query/1"
	case ToolStateMutate:
		return "contentcloud.tool/state.mutate/1"
	case ToolChildList:
		return "contentcloud.tool/child.list/1"
	case ToolEffectPrepare:
		return "contentcloud.tool/effect.prepare/1"
	case ToolEffectStatus:
		return "contentcloud.tool/effect.status/1"
	default:
		return ""
	}
}

func gatewaySafeRequest(toolName string, arguments map[string]any, idempotencyKey string) map[string]any {
	result := map[string]any{"idempotency_key": idempotencyKey}
	for _, key := range []string{"collection", "key", "after_key", "expected_version", "kind", "request_digest", "currency", "cost_minor"} {
		if value, ok := arguments[key]; ok {
			result[key] = value
		}
	}
	if value, ok := arguments["value"]; ok {
		result["value_digest"] = "sha256:" + mustCanonicalHash(value)
	}
	return result
}

func stringArgument(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return strings.TrimSpace(value)
}

func intArgument(arguments map[string]any, key string, fallback int) int {
	switch value := arguments[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := strconv.Atoi(value.String())
		return parsed
	default:
		return fallback
	}
}

func int64Argument(arguments map[string]any, key string, fallback int64) int64 {
	switch value := arguments[key].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case json.Number:
		parsed, _ := strconv.ParseInt(value.String(), 10, 64)
		return parsed
	default:
		return fallback
	}
}

func mustCanonicalHash(value any) string {
	digest, err := stablehash.Sum(value)
	if err != nil {
		return "invalid"
	}
	return digest
}

func gatewayErrorCode(err error) string {
	var domainErr *fault.Error
	if errors.As(err, &domainErr) {
		return domainErr.Code
	}
	return "MCP_GATEWAY_EXECUTION_FAILED"
}
