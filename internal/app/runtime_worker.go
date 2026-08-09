package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

// RuntimeWorkerPrepareInput is the wire contract for a remote worker. The
// server chooses the lease owner from the authenticated device; owner and
// fence fields are never accepted from the request body.
type RuntimeWorkerPrepareInput struct {
	JobRunID             string                           `json:"job_run_id"`
	HarnessKind          string                           `json:"harness_kind"`
	Capabilities         agentadapter.HarnessCapabilities `json:"capabilities"`
	Role                 string                           `json:"role"`
	ExecutionProfileID   string                           `json:"execution_profile_id"`
	Workspace            string                           `json:"workspace,omitempty"`
	Prompt               string                           `json:"prompt,omitempty"`
	OutputSchema         json.RawMessage                  `json:"output_schema,omitempty"`
	InputRefs            []string                         `json:"input_refs,omitempty"`
	StateRefs            []string                         `json:"state_refs,omitempty"`
	EventRefs            []string                         `json:"event_refs,omitempty"`
	AllowedTools         []string                         `json:"allowed_tools,omitempty"`
	MaxTokens            int                              `json:"max_tokens"`
	BudgetMinor          int64                            `json:"budget_minor"`
	RemainingDescendants int                              `json:"remaining_descendants"`
	LeaseForSeconds      int                              `json:"lease_for_seconds,omitempty"`
	ContextTTLSeconds    int                              `json:"context_ttl_seconds,omitempty"`
	ResourceRequests     []domain.ResourceRequest         `json:"resource_requests,omitempty"`
}

type RuntimeWorkerActivateInput struct {
	AttemptID  string                       `json:"attempt_id"`
	FenceToken string                       `json:"fence_token"`
	Session    agentadapter.AgentSessionRef `json:"session"`
}

type RuntimeWorkerPrepareNextInput struct {
	RuntimeWorkerPrepareInput
}

type RuntimeWorkerHeartbeatInput struct {
	AttemptID  string `json:"attempt_id"`
	FenceToken string `json:"fence_token"`
}

type RuntimeWorkerEventInput struct {
	AttemptID  string                  `json:"attempt_id"`
	FenceToken string                  `json:"fence_token"`
	Event      agentadapter.AgentEvent `json:"event"`
}

type RuntimeWorkerFinalizeInput struct {
	AttemptID       string          `json:"attempt_id"`
	FenceToken      string          `json:"fence_token"`
	State           string          `json:"state"`
	OutputRefs      []string        `json:"output_refs,omitempty"`
	OutputDigest    string          `json:"output_digest,omitempty"`
	ResultDigest    string          `json:"result_digest,omitempty"`
	SafeSummary     map[string]any  `json:"safe_summary,omitempty"`
	ErrorCode       string          `json:"error_code,omitempty"`
	UsedCostMinor   int64           `json:"used_cost_minor"`
	BusinessPayload json.RawMessage `json:"business_payload,omitempty"`
}

type RuntimeMCPCallInput struct {
	AttemptID  string         `json:"attempt_id"`
	FenceToken string         `json:"fence_token"`
	ToolName   string         `json:"tool_name"`
	RequestID  string         `json:"request_id"`
	Arguments  map[string]any `json:"arguments"`
}

type RuntimeWorkerResult struct {
	Handle            contentruntime.DispatchHandle `json:"handle"`
	Job               domain.JobRun                 `json:"job"`
	BusinessResultRef string                        `json:"business_result_ref,omitempty"`
}

func (s *Service) PrepareRuntimeWorker(ctx context.Context, actor Actor, input RuntimeWorkerPrepareInput) (contentruntime.DispatchHandle, error) {
	if err := requireRuntimeWorker(actor); err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	if s.runtimeService == nil {
		return contentruntime.DispatchHandle{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	leaseFor := secondsDuration(input.LeaseForSeconds)
	contextTTL := secondsDuration(input.ContextTTLSeconds)
	owner := runtimeWorkerOwner(actor)
	handle, err := s.runtimeService.PrepareRemoteDispatch(ctx, contentruntime.DispatchInput{
		TenantID: actor.TenantID, JobRunID: input.JobRunID, Owner: owner,
		HarnessKind: strings.TrimSpace(input.HarnessKind), Role: strings.TrimSpace(input.Role),
		ExecutionProfileID: strings.TrimSpace(input.ExecutionProfileID), Workspace: input.Workspace,
		Prompt: input.Prompt, OutputSchema: append(json.RawMessage(nil), input.OutputSchema...),
		InputRefs: append([]string(nil), input.InputRefs...), StateRefs: append([]string(nil), input.StateRefs...),
		EventRefs: append([]string(nil), input.EventRefs...), AllowedTools: append([]string(nil), input.AllowedTools...),
		MaxTokens: input.MaxTokens, BudgetMinor: input.BudgetMinor, RemainingDescendants: input.RemainingDescendants,
		LeaseFor: leaseFor, ContextTTL: contextTTL, ResourceRequests: append([]domain.ResourceRequest(nil), input.ResourceRequests...),
	}, input.Capabilities)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	return handle, nil
}

// CallRuntimeMCP is the only App-facing MCP path. Device authentication
// supplies the tenant; the worker still has to present the Attempt fence and
// the Runtime gateway re-checks every scope before executing a tool.
func (s *Service) CallRuntimeMCP(ctx context.Context, actor Actor, input RuntimeMCPCallInput) (contentruntime.GatewayResponse, error) {
	if err := requireRuntimeWorker(actor); err != nil {
		return contentruntime.GatewayResponse{}, err
	}
	if s.runtimeService == nil {
		return contentruntime.GatewayResponse{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	return contentruntime.NewRuntimeMCPGateway(s.runtimeService).Call(ctx, contentruntime.GatewayRequest{TenantID: actor.TenantID, AttemptID: input.AttemptID, FenceToken: input.FenceToken, ToolName: input.ToolName, RequestID: input.RequestID, Arguments: input.Arguments})
}

func (s *Service) PrepareNextRuntimeWorker(ctx context.Context, actor Actor, input RuntimeWorkerPrepareNextInput) (contentruntime.DispatchHandle, error) {
	candidate := input.RuntimeWorkerPrepareInput
	candidate.JobRunID = ""
	return s.PrepareRuntimeWorker(ctx, actor, candidate)
}

func (s *Service) ActivateRuntimeWorker(ctx context.Context, actor Actor, input RuntimeWorkerActivateInput) (contentruntime.DispatchHandle, error) {
	handle, err := s.workerHandle(ctx, actor, input.AttemptID, input.FenceToken)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	return s.runtimeService.ActivateDispatch(ctx, handle, input.Session)
}

func (s *Service) HeartbeatRuntimeWorker(ctx context.Context, actor Actor, input RuntimeWorkerHeartbeatInput) (contentruntime.DispatchHandle, error) {
	handle, err := s.workerHandle(ctx, actor, input.AttemptID, input.FenceToken)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	return s.runtimeService.HeartbeatDispatch(ctx, handle)
}

func (s *Service) RecordRuntimeWorkerEvent(ctx context.Context, actor Actor, input RuntimeWorkerEventInput) error {
	handle, err := s.workerHandle(ctx, actor, input.AttemptID, input.FenceToken)
	if err != nil {
		return err
	}
	return s.runtimeService.RecordHarnessEvent(ctx, handle, input.Event)
}

func (s *Service) FinalizeRuntimeWorker(ctx context.Context, actor Actor, input RuntimeWorkerFinalizeInput, requestID string) (RuntimeWorkerResult, error) {
	handle, err := s.workerFinalizeHandle(ctx, actor, input.AttemptID, input.FenceToken)
	if err != nil {
		return RuntimeWorkerResult{}, err
	}
	outcome := contentruntime.DispatchOutcome{State: input.State, OutputRefs: append([]string(nil), input.OutputRefs...), OutputDigest: strings.TrimSpace(input.OutputDigest), ResultDigest: strings.TrimSpace(input.ResultDigest), SafeSummary: input.SafeSummary, ErrorCode: strings.TrimSpace(input.ErrorCode), UsedCostMinor: input.UsedCostMinor}
	for _, ref := range outcome.OutputRefs {
		if strings.HasPrefix(strings.TrimSpace(ref), "runtime-result:") {
			return RuntimeWorkerResult{}, domain.Invalid("RUNTIME_BUSINESS_RESULT_REF_RESERVED", "runtime-result 引用只能由服务端从已校验的业务 payload 生成")
		}
	}
	resultRef := ""
	businessDigest := ""
	var businessErr error
	if len(input.BusinessPayload) > 0 {
		resultRef, businessDigest, err = s.persistRuntimeBusinessResult(ctx, actor.TenantID, input.AttemptID, input.BusinessPayload)
		if err != nil {
			return RuntimeWorkerResult{}, err
		}
		outcome.OutputRefs = append(outcome.OutputRefs, resultRef)
		if outcome.OutputDigest != "" && outcome.OutputDigest != businessDigest {
			return RuntimeWorkerResult{}, domain.Conflict("RUNTIME_BUSINESS_RESULT_DIGEST_CONFLICT", "worker 提交的 output digest 与结构化业务结果不一致")
		}
		outcome.OutputDigest = businessDigest
		if outcome.State == domain.RuntimeAttemptSucceeded {
			businessErr = s.validateRuntimeBusinessResult(ctx, actor, handle.Attempt.JobRunID, input.BusinessPayload)
		}
	} else if input.State == domain.RuntimeAttemptSucceeded {
		if job, jobErr := s.runtimeService.Job(ctx, actor.TenantID, handle.Attempt.JobRunID); jobErr != nil {
			businessErr = jobErr
		} else if job.BusinessType == "knowledge_extract" {
			businessErr = domain.Invalid("RUNTIME_BUSINESS_RESULT_REQUIRED", "知识提取成功必须交接结构化业务结果")
		}
	}
	if businessErr != nil {
		outcome.State = domain.RuntimeAttemptFailed
		outcome.ErrorCode = "RUNTIME_BUSINESS_RESULT_INVALID"
		outcome.OutputRefs = nil
		outcome.OutputDigest = ""
	}
	finalized, err := s.runtimeService.FinalizeDispatch(ctx, handle, outcome)
	if err != nil {
		return RuntimeWorkerResult{}, err
	}
	if businessErr != nil {
		return RuntimeWorkerResult{Handle: finalized.Handle, Job: finalized.Job, BusinessResultRef: resultRef}, businessErr
	}
	return RuntimeWorkerResult{Handle: finalized.Handle, Job: finalized.Job, BusinessResultRef: resultRef}, nil
}

func (s *Service) validateRuntimeBusinessResult(ctx context.Context, actor Actor, jobID string, payload json.RawMessage) error {
	run, pkg, handled, err := s.runtimeKnowledgePackage(ctx, actor, jobID, payload)
	if err != nil {
		return err
	}
	if !handled {
		return domain.Invalid("RUNTIME_BUSINESS_RESULT_UNSUPPORTED", "当前任务类型没有注册结构化业务结果契约")
	}
	_, _, err = s.validateKnowledgePackage(ctx, actor, run, pkg)
	return err
}

func (s *Service) runtimeKnowledgePackage(ctx context.Context, actor Actor, jobID string, payload json.RawMessage) (domain.RuntimeRun, domain.KnowledgeExtractionPackage, bool, error) {
	job, err := s.runtimeService.Job(ctx, actor.TenantID, jobID)
	if err != nil {
		return domain.RuntimeRun{}, domain.KnowledgeExtractionPackage{}, false, err
	}
	if job.BusinessType != "knowledge_extract" {
		return domain.RuntimeRun{}, domain.KnowledgeExtractionPackage{}, false, nil
	}
	var pkg domain.KnowledgeExtractionPackage
	if err := decodeStrict(payload, &pkg); err != nil {
		return domain.RuntimeRun{}, domain.KnowledgeExtractionPackage{}, true, domain.Invalid("RUNTIME_BUSINESS_RESULT_INVALID", "知识候选业务结果不符合严格 JSON 契约")
	}
	run, err := s.projectRuntimeRun(ctx, job)
	if err != nil {
		return domain.RuntimeRun{}, domain.KnowledgeExtractionPackage{}, true, err
	}
	return run, pkg, true, nil
}

func (s *Service) workerHandle(ctx context.Context, actor Actor, attemptID, fenceToken string) (contentruntime.DispatchHandle, error) {
	handle, err := s.loadRuntimeWorkerHandle(ctx, actor, attemptID)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	if handle.Attempt.LeaseOwner != runtimeWorkerOwner(actor) {
		return contentruntime.DispatchHandle{}, domain.Policy("RUNTIME_WORKER_OWNER_MISMATCH", "执行尝试不属于当前工作器", "使用领取该执行尝试的设备凭据")
	}
	if strings.TrimSpace(fenceToken) == "" || fenceToken != handle.Attempt.FenceToken {
		return contentruntime.DispatchHandle{}, domain.Conflict("RUNTIME_FENCE_TOKEN_INVALID", "Runtime fence token 无效或已过期")
	}
	return handle, nil
}

func (s *Service) workerFinalizeHandle(ctx context.Context, actor Actor, attemptID, fenceToken string) (contentruntime.DispatchHandle, error) {
	handle, err := s.loadRuntimeWorkerHandle(ctx, actor, attemptID)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	if !handle.Attempt.Terminal() {
		if handle.Attempt.LeaseOwner != runtimeWorkerOwner(actor) {
			return contentruntime.DispatchHandle{}, domain.Policy("RUNTIME_WORKER_OWNER_MISMATCH", "执行尝试不属于当前工作器", "使用领取该执行尝试的设备凭据")
		}
		if strings.TrimSpace(fenceToken) == "" || fenceToken != handle.Attempt.FenceToken {
			return contentruntime.DispatchHandle{}, domain.Conflict("RUNTIME_FENCE_TOKEN_INVALID", "Runtime fence token 无效或已过期")
		}
		return handle, nil
	}
	if strings.TrimSpace(fenceToken) == "" {
		return contentruntime.DispatchHandle{}, domain.Conflict("RUNTIME_FENCE_TOKEN_INVALID", "Runtime fence token 无效或已过期")
	}
	events, err := s.runtimeService.Events(ctx, actor.TenantID, handle.Attempt.JobRunID, 0)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	wantedOwner := runtimeWorkerOwner(actor)
	wantedFence := contentruntime.FenceTokenDigest(fenceToken)
	for _, event := range events {
		attempt, _ := event.Payload["attempt_id"].(string)
		fence, _ := event.Payload["fence_digest"].(string)
		if attempt == handle.Attempt.ID && event.ActorType == "worker" && event.ActorID == wantedOwner && fence == wantedFence {
			return handle, nil
		}
	}
	return contentruntime.DispatchHandle{}, domain.Policy("RUNTIME_WORKER_OWNER_MISMATCH", "执行尝试不属于当前工作器或 fence 已失效", "使用完成该执行尝试的设备凭据和 fence token")
}

func (s *Service) loadRuntimeWorkerHandle(ctx context.Context, actor Actor, attemptID string) (contentruntime.DispatchHandle, error) {
	if err := requireRuntimeWorker(actor); err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	if s.runtimeService == nil {
		return contentruntime.DispatchHandle{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	handle, err := s.runtimeService.LoadDispatchHandle(ctx, actor.TenantID, strings.TrimSpace(attemptID))
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	return handle, nil
}

func (s *Service) persistRuntimeBusinessResult(ctx context.Context, tenantID, attemptID string, payload json.RawMessage) (string, string, error) {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return "", "", domain.Invalid("RUNTIME_BUSINESS_RESULT_INVALID", "业务结果必须是合法 JSON")
	}
	digest, err := domain.CanonicalHash(value)
	if err != nil {
		return "", "", err
	}
	if len(payload) > 4*1024*1024 {
		return "", "", domain.Policy("RUNTIME_BUSINESS_RESULT_TOO_LARGE", "业务结果超过 Runtime 单次交接上限", "将正文存入业务对象或拆分为受控输出引用")
	}
	key := fmt.Sprintf("runtime/results/%s/%s/%s.json", tenantID, attemptID, digest)
	if err := s.blobs.Put(ctx, key, payload); err != nil {
		return "", "", err
	}
	return "runtime-result:" + key, "sha256:" + digest, nil
}

func requireRuntimeWorker(actor Actor) error {
	if actor.Type != "device" && actor.Type != "worker" {
		return domain.Policy("RUNTIME_WORKER_AUTH_REQUIRED", "Runtime worker 协议只接受设备或工作器凭据", "使用已授权的执行设备凭据")
	}
	if strings.TrimSpace(actor.TenantID) == "" {
		return domain.Invalid("RUNTIME_WORKER_TENANT_REQUIRED", "Runtime worker 缺少租户范围")
	}
	return nil
}

func runtimeWorkerOwner(actor Actor) string {
	if strings.TrimSpace(actor.DeviceID) != "" {
		return "device:" + strings.TrimSpace(actor.DeviceID)
	}
	if strings.TrimSpace(actor.UserID) != "" {
		return "worker:" + strings.TrimSpace(actor.UserID)
	}
	return "worker:unknown"
}

func secondsDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
