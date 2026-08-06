package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
)

type DispatchInput struct {
	TenantID             string
	JobRunID             string
	Owner                string
	HarnessKind          string
	Role                 string
	ExecutionProfileID   string
	Workspace            string
	Prompt               string
	OutputSchema         json.RawMessage
	InputRefs            []string
	StateRefs            []string
	EventRefs            []string
	AllowedTools         []string
	MaxTokens            int
	BudgetMinor          int64
	RemainingDescendants int
	LeaseFor             time.Duration
	ContextTTL           time.Duration
}

type DispatchHandle struct {
	Node         domain.NodeRun                   `json:"node"`
	Attempt      domain.RuntimeAttempt            `json:"attempt"`
	ContextView  domain.ContextView               `json:"context_view"`
	Agent        domain.AgentInstance             `json:"agent"`
	Capabilities agentadapter.HarnessCapabilities `json:"capabilities"`
	LeaseFor     time.Duration                    `json:"-"`
}

type DispatchOutcome struct {
	State         string
	OutputRefs    []string
	OutputDigest  string
	ResultDigest  string
	SafeSummary   map[string]any
	ErrorCode     string
	UsedCostMinor int64
}

type DispatchResult struct {
	Handle DispatchHandle `json:"handle"`
	Job    domain.JobRun  `json:"job"`
}

type harnessResult struct {
	OutputRefs    []string       `json:"output_refs"`
	OutputDigest  string         `json:"output_digest"`
	SafeSummary   map[string]any `json:"safe_summary"`
	UsedCostMinor int64          `json:"used_cost_minor"`
}

func (s *Service) Attempt(ctx context.Context, tenantID, id string) (domain.RuntimeAttempt, error) {
	return s.repo.RuntimeAttempt(ctx, tenantID, id)
}

func (s *Service) Attempts(ctx context.Context, tenantID, jobID string) ([]domain.RuntimeAttempt, error) {
	return s.repo.RuntimeAttempts(ctx, tenantID, jobID)
}

// PrepareDispatch is phase one of the dispatch protocol. No external harness
// call occurs until the leased node, immutable ContextView, logical Agent and
// RuntimeAttempt are committed together.
func (s *Service) PrepareDispatch(ctx context.Context, input DispatchInput) (DispatchHandle, error) {
	if s == nil || s.repo == nil {
		return DispatchHandle{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if s.harnesses == nil {
		return DispatchHandle{}, domain.Policy("AGENT_HARNESS_REGISTRY_UNAVAILABLE", "Runtime 尚未注入智能体执行适配器注册表", "联系平台运营人员检查 Runtime 启动配置")
	}
	_, capabilities, err := s.harnesses.Resolve(ctx, input.HarnessKind)
	if err != nil {
		return DispatchHandle{}, err
	}
	return s.prepareDispatchWithRetry(ctx, input, capabilities)
}

func (s *Service) prepareDispatchWithRetry(ctx context.Context, input DispatchInput, capabilities agentadapter.HarnessCapabilities) (DispatchHandle, error) {
	for {
		handle, err := s.prepareDispatch(ctx, input, capabilities)
		if err == nil || !hasDomainCode(err, "NODE_DISPATCH_CONFLICT") {
			return handle, err
		}
		if ctx.Err() != nil {
			return DispatchHandle{}, ctx.Err()
		}
	}
}

func (s *Service) prepareDispatch(ctx context.Context, input DispatchInput, capabilities agentadapter.HarnessCapabilities) (DispatchHandle, error) {
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.JobRunID = strings.TrimSpace(input.JobRunID)
	input.Owner = strings.TrimSpace(input.Owner)
	input.HarnessKind = strings.TrimSpace(capabilities.Kind)
	input.Role = strings.TrimSpace(input.Role)
	input.ExecutionProfileID = strings.TrimSpace(input.ExecutionProfileID)
	if input.TenantID == "" || input.Owner == "" || input.HarnessKind == "" || input.Role == "" || input.ExecutionProfileID == "" {
		return DispatchHandle{}, domain.Invalid("DISPATCH_INPUT_INVALID", "调度请求缺少租户、执行者、适配器、角色或执行配置")
	}
	if !capabilities.Events || !capabilities.StructuredOutput {
		return DispatchHandle{}, domain.Policy("AGENT_HARNESS_CAPABILITY_MISSING", "当前适配器不支持 Runtime 所需的结构化事件和结果", "选择支持结构化事件的 Harness")
	}
	if input.MaxTokens <= 0 || input.BudgetMinor < 0 || input.RemainingDescendants < 0 {
		return DispatchHandle{}, domain.Invalid("DISPATCH_POLICY_INVALID", "调度请求的上下文、预算或派生额度无效")
	}
	if input.LeaseFor <= 0 {
		input.LeaseFor = DefaultNodeLeaseDuration
	}
	if input.ContextTTL <= 0 {
		input.ContextTTL = time.Hour
	}
	if input.ContextTTL < input.LeaseFor {
		input.ContextTTL = input.LeaseFor
	}
	node, err := s.repo.NextReadyNode(ctx, input.TenantID, input.JobRunID)
	if err != nil {
		return DispatchHandle{}, err
	}
	now := s.now().UTC()
	expires := now.Add(input.LeaseFor)
	attemptID := domain.NewID()
	view, err := BuildContextView(ContextViewInput{
		TenantID: input.TenantID, JobRunID: node.JobRunID, NodeRunID: node.ID, AttemptID: attemptID,
		InputRefs: input.InputRefs, StateRefs: input.StateRefs, EventRefs: input.EventRefs, AllowedTools: input.AllowedTools,
		MaxTokens: input.MaxTokens, BudgetMinor: input.BudgetMinor, CreatedAt: now, ExpiresAt: now.Add(input.ContextTTL),
	})
	if err != nil {
		return DispatchHandle{}, err
	}
	agent, agentErr := s.repo.AgentInstanceForNode(ctx, input.TenantID, node.ID)
	createAgent := domain.IsNotFound(agentErr)
	if agentErr != nil && !createAgent {
		return DispatchHandle{}, agentErr
	}
	expectedAgentVersion := 0
	if createAgent {
		agent = domain.AgentInstance{
			ID: domain.NewID(), TenantID: input.TenantID, JobRunID: node.JobRunID, NodeRunID: node.ID,
			Role: input.Role, HarnessKind: input.HarnessKind, ExecutionProfileID: input.ExecutionProfileID,
			ContextViewID: view.ID, State: domain.AgentRunnable, RemainingDescendants: input.RemainingDescendants,
			BudgetMinor: input.BudgetMinor, UsedCostMinor: 0, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	} else {
		if agent.State != domain.AgentRunnable || agent.HarnessKind != input.HarnessKind || agent.ExecutionProfileID != input.ExecutionProfileID {
			return DispatchHandle{}, domain.Conflict("AGENT_INSTANCE_NOT_REUSABLE", "节点 AgentInstance 尚未让出执行权或执行配置发生变化")
		}
		expectedAgentVersion = agent.Version
		agent.ContextViewID = view.ID
		agent.SessionRef = ""
		agent.Version++
		agent.UpdatedAt = now
	}
	capabilitySnapshot := harnessCapabilitiesMap(capabilities)
	attempt := domain.RuntimeAttempt{
		ID: attemptID, TenantID: input.TenantID, JobRunID: node.JobRunID, NodeRunID: node.ID,
		AgentInstanceID: agent.ID, ContextViewID: view.ID, AttemptNo: node.AttemptCount + 1,
		HarnessKind: input.HarnessKind, Capabilities: capabilitySnapshot, State: domain.RuntimeAttemptPrepared,
		LeaseOwner: input.Owner, LeaseExpiresAt: &expires, OutputRefs: []string{}, SafeSummary: map[string]any{},
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	expectedNodeVersion := node.Version
	if err := node.Transition(domain.NodeLeased); err != nil {
		return DispatchHandle{}, err
	}
	node.State = domain.NodeLeased
	node.AttemptCount++
	node.LeaseOwner = input.Owner
	node.LeaseExpiresAt = &expires
	node.Version++
	node.UpdatedAt = now
	event := domain.JobEvent{
		ID: domain.NewID(), TenantID: input.TenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey,
		Type: "attempt.prepared", ActorType: "scheduler", ActorID: input.Owner,
		IdempotencyKey: attempt.ID + ":prepared",
		Payload:        map[string]any{"attempt_id": attempt.ID, "attempt_no": attempt.AttemptNo, "harness_kind": attempt.HarnessKind, "context_digest": view.Digest}, OccurredAt: now,
	}
	node, attempt, agent, err = s.repo.PrepareDispatch(ctx, node, expectedNodeVersion, attempt, view, agent, createAgent, expectedAgentVersion, event)
	if err != nil {
		return DispatchHandle{}, err
	}
	return DispatchHandle{Node: node, Attempt: attempt, ContextView: view, Agent: agent, Capabilities: capabilities, LeaseFor: input.LeaseFor}, nil
}

func (s *Service) ActivateDispatch(ctx context.Context, handle DispatchHandle, session agentadapter.AgentSessionRef) (DispatchHandle, error) {
	if session.SessionID == "" || len(session.SessionID) > 1024 || strings.TrimSpace(session.HarnessKind) != handle.Attempt.HarnessKind {
		return handle, domain.Invalid("DISPATCH_SESSION_INVALID", "Harness 会话引用与 RuntimeAttempt 不一致")
	}
	encodedSession, err := json.Marshal(session)
	if err != nil {
		return handle, err
	}
	now := s.now().UTC()
	expires := now.Add(handle.LeaseFor)
	node, attempt, agent := handle.Node, handle.Attempt, handle.Agent
	expectedNodeVersion, expectedAttemptVersion, expectedAgentVersion := node.Version, attempt.Version, agent.Version
	if err := node.Transition(domain.NodeRunning); err != nil {
		return handle, err
	}
	if err := attempt.Transition(domain.RuntimeAttemptRunning); err != nil {
		return handle, err
	}
	if err := agent.Transition(domain.AgentActive); err != nil {
		return handle, err
	}
	node.State = domain.NodeRunning
	node.LeaseExpiresAt = &expires
	node.Version++
	node.UpdatedAt = now
	attempt.State = domain.RuntimeAttemptRunning
	attempt.SessionRef = string(encodedSession)
	attempt.LeaseExpiresAt = &expires
	attempt.StartedAt = &now
	attempt.Version++
	attempt.UpdatedAt = now
	agent.State = domain.AgentActive
	agent.SessionRef = string(encodedSession)
	agent.Version++
	agent.UpdatedAt = now
	event := domain.JobEvent{ID: domain.NewID(), TenantID: node.TenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "attempt.running", ActorType: "worker", ActorID: attempt.LeaseOwner, IdempotencyKey: attempt.ID + ":running", Payload: map[string]any{"attempt_id": attempt.ID, "harness_kind": attempt.HarnessKind}, OccurredAt: now}
	node, attempt, agent, err = s.repo.ActivateDispatch(ctx, node, expectedNodeVersion, attempt, expectedAttemptVersion, agent, expectedAgentVersion, event)
	if err != nil {
		return handle, err
	}
	handle.Node, handle.Attempt, handle.Agent = node, attempt, agent
	return handle, nil
}

func (s *Service) HeartbeatDispatch(ctx context.Context, handle DispatchHandle) (DispatchHandle, error) {
	node, attempt, err := s.repo.HeartbeatDispatch(ctx, handle.Attempt.TenantID, handle.Attempt.ID, handle.Attempt.LeaseOwner, handle.Node.Version, handle.Attempt.Version, s.now().UTC(), handle.LeaseFor)
	if err != nil {
		return handle, err
	}
	handle.Node, handle.Attempt = node, attempt
	return handle, nil
}

func (s *Service) FinalizeDispatch(ctx context.Context, handle DispatchHandle, outcome DispatchOutcome) (DispatchResult, error) {
	now := s.now().UTC()
	node, attempt, agent := handle.Node, handle.Attempt, handle.Agent
	if outcome.SafeSummary == nil {
		outcome.SafeSummary = map[string]any{}
	}
	outcome.SafeSummary = sanitizeSafeSummary(outcome.SafeSummary)
	outcome.OutputRefs = sortedRefs(outcome.OutputRefs)
	if outcome.State == "" {
		outcome.State = domain.RuntimeAttemptSucceeded
	}
	if outcome.State == domain.RuntimeAttemptRetryableFailed {
		maxAttempts, err := s.maxAttemptsForNode(ctx, node)
		if err != nil {
			return DispatchResult{Handle: handle}, err
		}
		if attempt.AttemptNo >= maxAttempts {
			outcome.State = domain.RuntimeAttemptFailed
		}
	}
	if outcome.State != domain.RuntimeAttemptSucceeded && outcome.State != domain.RuntimeAttemptRetryableFailed && outcome.State != domain.RuntimeAttemptFailed && outcome.State != domain.RuntimeAttemptCancelled {
		return DispatchResult{Handle: handle}, domain.Invalid("DISPATCH_OUTCOME_INVALID", "RuntimeAttempt 终态结果无效")
	}
	if outcome.State == domain.RuntimeAttemptSucceeded {
		outcome.ErrorCode = ""
	} else {
		outcome.ErrorCode = safeErrorCode(outcome.ErrorCode, "DISPATCH_FAILED")
	}
	if outcome.UsedCostMinor < agent.UsedCostMinor || outcome.UsedCostMinor > agent.BudgetMinor {
		return DispatchResult{Handle: handle}, domain.Policy("AGENT_INSTANCE_USAGE_INVALID", "AgentInstance 用量不能回退或超过预算", "重新核对累计用量")
	}
	if outcome.ResultDigest == "" {
		digest, err := domain.CanonicalHash(struct {
			State        string
			OutputRefs   []string
			OutputDigest string
			SafeSummary  map[string]any
			ErrorCode    string
		}{outcome.State, outcome.OutputRefs, outcome.OutputDigest, outcome.SafeSummary, strings.TrimSpace(outcome.ErrorCode)})
		if err != nil {
			return DispatchResult{Handle: handle}, err
		}
		outcome.ResultDigest = "sha256:" + digest
	}
	if attempt.Terminal() {
		storedAttempt, err := s.repo.RuntimeAttempt(ctx, attempt.TenantID, attempt.ID)
		if err != nil {
			return DispatchResult{Handle: handle}, err
		}
		if !storedAttempt.Terminal() || storedAttempt.State != outcome.State || storedAttempt.ResultDigest != outcome.ResultDigest {
			return DispatchResult{Handle: handle}, domain.Conflict("RUNTIME_ATTEMPT_RESULT_CONFLICT", "RuntimeAttempt 已收到不同的终态结果")
		}
		storedNode, err := s.repo.NodeRun(ctx, node.TenantID, node.ID)
		if err != nil {
			return DispatchResult{Handle: handle}, err
		}
		storedAgent, err := s.repo.AgentInstance(ctx, agent.TenantID, agent.ID)
		if err != nil {
			return DispatchResult{Handle: handle}, err
		}
		handle.Node, handle.Attempt, handle.Agent = storedNode, storedAttempt, storedAgent
		job, err := s.Refresh(ctx, node.TenantID, node.JobRunID)
		return DispatchResult{Handle: handle, Job: job}, err
	}
	expectedNodeVersion, expectedAttemptVersion, expectedAgentVersion := node.Version, attempt.Version, agent.Version
	nodeState, agentState := domain.NodeSucceeded, domain.AgentCompleted
	switch outcome.State {
	case domain.RuntimeAttemptRetryableFailed:
		nodeState, agentState = domain.NodeRetryableFailed, domain.AgentRunnable
	case domain.RuntimeAttemptFailed:
		nodeState, agentState = domain.NodeFailed, domain.AgentFailed
	case domain.RuntimeAttemptCancelled:
		nodeState, agentState = domain.NodeCancelled, domain.AgentCancelled
	}
	if err := node.Transition(nodeState); err != nil {
		return DispatchResult{Handle: handle}, err
	}
	if err := attempt.Transition(outcome.State); err != nil {
		return DispatchResult{Handle: handle}, err
	}
	if err := agent.Transition(agentState); err != nil {
		return DispatchResult{Handle: handle}, err
	}
	node.State = nodeState
	node.OutputRefs = append([]string(nil), outcome.OutputRefs...)
	node.OutputDigest = strings.TrimSpace(outcome.OutputDigest)
	if node.State == domain.NodeSucceeded && node.OutputDigest == "" {
		node.OutputDigest = outcome.ResultDigest
	}
	node.ErrorCode = strings.TrimSpace(outcome.ErrorCode)
	node.LeaseOwner = ""
	node.LeaseExpiresAt = nil
	node.Version++
	node.UpdatedAt = now
	attempt.State = outcome.State
	attempt.OutputRefs = append([]string(nil), outcome.OutputRefs...)
	attempt.ResultDigest = outcome.ResultDigest
	attempt.SafeSummary = outcome.SafeSummary
	attempt.ErrorCode = strings.TrimSpace(outcome.ErrorCode)
	attempt.LeaseOwner = ""
	attempt.LeaseExpiresAt = nil
	attempt.FinishedAt = &now
	attempt.Version++
	attempt.UpdatedAt = now
	agent.State = agentState
	agent.UsedCostMinor = outcome.UsedCostMinor
	agent.Version++
	agent.UpdatedAt = now
	event := domain.JobEvent{ID: domain.NewID(), TenantID: node.TenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "attempt." + outcome.State, ActorType: "worker", ActorID: handle.Attempt.LeaseOwner, IdempotencyKey: attempt.ID + ":terminal:" + outcome.ResultDigest, Payload: map[string]any{"attempt_id": attempt.ID, "result_digest": outcome.ResultDigest, "output_count": len(outcome.OutputRefs), "error_code": attempt.ErrorCode}, OccurredAt: now}
	node, attempt, agent, err := s.repo.FinalizeDispatch(ctx, node, expectedNodeVersion, attempt, expectedAttemptVersion, agent, expectedAgentVersion, event)
	if err != nil {
		return DispatchResult{Handle: handle}, err
	}
	handle.Node, handle.Attempt, handle.Agent = node, attempt, agent
	job, err := s.Refresh(ctx, node.TenantID, node.JobRunID)
	return DispatchResult{Handle: handle, Job: job}, err
}

// DispatchNext executes the full protocol for one ready node. Harness calls are
// intentionally outside repository transactions; a crash between phases is
// recovered by the shared Node/Attempt lease.
func (s *Service) DispatchNext(ctx context.Context, input DispatchInput) (DispatchResult, error) {
	if s == nil || s.harnesses == nil {
		return DispatchResult{}, domain.Policy("AGENT_HARNESS_REGISTRY_UNAVAILABLE", "Runtime 尚未注入智能体执行适配器注册表", "联系平台运营人员检查 Runtime 启动配置")
	}
	harness, capabilities, err := s.harnesses.Resolve(ctx, input.HarnessKind)
	if err != nil {
		return DispatchResult{}, err
	}
	handle, err := s.prepareDispatchWithRetry(ctx, input, capabilities)
	if err != nil {
		return DispatchResult{}, err
	}
	session, stream, err := harness.Start(ctx, agentadapter.StartAgentRequest{JobRunID: handle.Node.JobRunID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID, Workspace: input.Workspace, Prompt: input.Prompt, OutputSchema: input.OutputSchema, ContextDigest: handle.ContextView.Digest})
	if err != nil {
		return s.FinalizeDispatch(ctx, handle, DispatchOutcome{State: domain.RuntimeAttemptRetryableFailed, ErrorCode: errorCode(err, "HARNESS_START_FAILED"), UsedCostMinor: handle.Agent.UsedCostMinor})
	}
	if stream == nil {
		return s.FinalizeDispatch(ctx, handle, DispatchOutcome{State: domain.RuntimeAttemptRetryableFailed, ErrorCode: "HARNESS_STREAM_MISSING", UsedCostMinor: handle.Agent.UsedCostMinor})
	}
	defer stream.Close()
	handle, err = s.ActivateDispatch(ctx, handle, session)
	if err != nil {
		_ = harness.Interrupt(context.WithoutCancel(ctx), session)
		cleanup, cleanupErr := s.FinalizeDispatch(context.WithoutCancel(ctx), handle, DispatchOutcome{State: domain.RuntimeAttemptRetryableFailed, ErrorCode: "DISPATCH_ACTIVATION_FAILED", UsedCostMinor: handle.Agent.UsedCostMinor})
		if cleanupErr != nil {
			return cleanup, errors.Join(err, cleanupErr)
		}
		return cleanup, err
	}
	heartbeatEvery := handle.LeaseFor / 3
	if heartbeatEvery < 10*time.Millisecond {
		heartbeatEvery = 10 * time.Millisecond
	}
	heartbeat := time.NewTicker(heartbeatEvery)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			cleanupContext := context.WithoutCancel(ctx)
			_ = harness.Interrupt(cleanupContext, session)
			result, finalizeErr := s.FinalizeDispatch(cleanupContext, handle, DispatchOutcome{State: domain.RuntimeAttemptRetryableFailed, ErrorCode: "DISPATCH_CONTEXT_CANCELLED", UsedCostMinor: handle.Agent.UsedCostMinor})
			if finalizeErr != nil {
				return result, errors.Join(ctx.Err(), finalizeErr)
			}
			return result, ctx.Err()
		case <-heartbeat.C:
			handle, err = s.HeartbeatDispatch(ctx, handle)
			if err != nil {
				_ = harness.Interrupt(context.WithoutCancel(ctx), session)
				return DispatchResult{Handle: handle}, err
			}
		case event, ok := <-stream.Events():
			if !ok {
				return s.FinalizeDispatch(ctx, handle, DispatchOutcome{State: domain.RuntimeAttemptRetryableFailed, ErrorCode: "HARNESS_STREAM_CLOSED", UsedCostMinor: handle.Agent.UsedCostMinor})
			}
			if event.Session != session {
				return s.FinalizeDispatch(ctx, handle, DispatchOutcome{State: domain.RuntimeAttemptFailed, ErrorCode: "HARNESS_SESSION_MISMATCH", UsedCostMinor: handle.Agent.UsedCostMinor})
			}
			switch event.Type {
			case "result.completed":
				outcome, parseErr := outcomeFromHarnessResult(event.Data)
				if parseErr != nil {
					return s.FinalizeDispatch(ctx, handle, DispatchOutcome{State: domain.RuntimeAttemptRetryableFailed, ErrorCode: "HARNESS_RESULT_INVALID", UsedCostMinor: handle.Agent.UsedCostMinor})
				}
				return s.FinalizeDispatch(ctx, handle, outcome)
			case "session.failed":
				return s.FinalizeDispatch(ctx, handle, DispatchOutcome{State: domain.RuntimeAttemptRetryableFailed, ErrorCode: safeErrorCode(event.ErrorCode, "HARNESS_SESSION_FAILED"), UsedCostMinor: handle.Agent.UsedCostMinor})
			case "session.interrupted":
				return s.FinalizeDispatch(ctx, handle, DispatchOutcome{State: domain.RuntimeAttemptRetryableFailed, ErrorCode: "HARNESS_SESSION_INTERRUPTED", UsedCostMinor: handle.Agent.UsedCostMinor})
			default:
				if err := s.recordHarnessEvent(ctx, handle, event); err != nil {
					return DispatchResult{Handle: handle}, err
				}
			}
		}
	}
}

func outcomeFromHarnessResult(data json.RawMessage) (DispatchOutcome, error) {
	var result harnessResult
	if len(data) == 0 || json.Unmarshal(data, &result) != nil {
		return DispatchOutcome{}, domain.Invalid("HARNESS_RESULT_INVALID", "Harness 返回的结构化结果无效")
	}
	var canonical any
	if err := json.Unmarshal(data, &canonical); err != nil {
		return DispatchOutcome{}, err
	}
	digest, err := domain.CanonicalHash(canonical)
	if err != nil {
		return DispatchOutcome{}, err
	}
	return DispatchOutcome{State: domain.RuntimeAttemptSucceeded, OutputRefs: result.OutputRefs, OutputDigest: result.OutputDigest, ResultDigest: "sha256:" + digest, SafeSummary: sanitizeSafeSummary(result.SafeSummary), UsedCostMinor: result.UsedCostMinor}, nil
}

func (s *Service) recordHarnessEvent(ctx context.Context, handle DispatchHandle, event agentadapter.AgentEvent) error {
	dataDigest := ""
	if len(event.Data) > 0 {
		var value any
		if json.Unmarshal(event.Data, &value) == nil {
			if digest, err := domain.CanonicalHash(value); err == nil {
				dataDigest = "sha256:" + digest
			}
		}
	}
	_, err := s.repo.AppendJobEvent(ctx, domain.JobEvent{ID: domain.NewID(), TenantID: handle.Node.TenantID, JobRunID: handle.Node.JobRunID, NodeKey: handle.Node.NodeKey, Type: "attempt.event", ActorType: "harness", ActorID: handle.Attempt.HarnessKind, Payload: map[string]any{"attempt_id": handle.Attempt.ID, "event_type": safeEventType(event.Type), "data_digest": dataDigest, "error_code": safeErrorCode(event.ErrorCode, "")}, OccurredAt: s.now().UTC()})
	return err
}

func (s *Service) maxAttemptsForNode(ctx context.Context, node domain.NodeRun) (int, error) {
	job, err := s.repo.JobRun(ctx, node.TenantID, node.JobRunID)
	if err != nil {
		return 0, err
	}
	plan, err := s.repo.Plan(ctx, node.TenantID, job.PlanRevisionID)
	if err != nil {
		return 0, err
	}
	maxAttempts := plan.Limits.MaxAttemptsPerNode
	if maxAttempts <= 0 {
		maxAttempts = domain.DefaultRuntimeLimits().MaxAttemptsPerNode
	}
	for _, spec := range plan.Nodes {
		if spec.Key == node.NodeKey && spec.RetryMaxAttempts > 0 {
			return spec.RetryMaxAttempts, nil
		}
	}
	return maxAttempts, nil
}

func harnessCapabilitiesMap(capabilities agentadapter.HarnessCapabilities) map[string]any {
	return map[string]any{
		"kind": capabilities.Kind, "events": capabilities.Events, "resume": capabilities.Resume,
		"fork": capabilities.Fork, "mcp_stdio": capabilities.MCPStdio, "mcp_http": capabilities.MCPHTTP,
		"structured_output": capabilities.StructuredOutput, "sandbox_profile": capabilities.SandboxProfile,
		"max_parallel_sessions": capabilities.MaxParallelSessions, "transcript_export": capabilities.TranscriptExport,
	}
}

func errorCode(err error, fallback string) string {
	var domainError *domain.Error
	if errors.As(err, &domainError) && strings.TrimSpace(domainError.Code) != "" {
		return safeErrorCode(domainError.Code, fallback)
	}
	return fallback
}

func hasDomainCode(err error, code string) bool {
	var domainError *domain.Error
	return errors.As(err, &domainError) && domainError.Code == code
}

func sanitizeSafeSummary(input map[string]any) map[string]any {
	return sanitizeSummaryMap(input, 0)
}

func sanitizeSummaryMap(input map[string]any, depth int) map[string]any {
	if input == nil || depth >= 4 {
		return map[string]any{}
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 64 {
		keys = keys[:64]
	}
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		lower := strings.ToLower(strings.TrimSpace(key))
		if lower == "" {
			continue
		}
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "credential") || strings.Contains(lower, "password") || strings.Contains(lower, "api_key") || strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "absolute_path") || lower == "path" {
			result[key] = "[redacted]"
			continue
		}
		result[key] = sanitizeSummaryValue(input[key], depth+1)
	}
	return result
}

func sanitizeSummaryValue(value any, depth int) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeSummaryMap(typed, depth)
	case []any:
		limit := len(typed)
		if limit > 64 {
			limit = 64
		}
		result := make([]any, 0, limit)
		for _, item := range typed[:limit] {
			result = append(result, sanitizeSummaryValue(item, depth+1))
		}
		return result
	case string:
		runes := []rune(typed)
		if len(runes) > 512 {
			return string(runes[:512])
		}
		return typed
	case nil, bool, float64, float32, int, int32, int64, uint, uint32, uint64:
		return typed
	default:
		return "[unsupported]"
	}
}

func safeErrorCode(value, fallback string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	if len(value) > 64 {
		return fallback
	}
	for _, char := range value {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' {
			return fallback
		}
	}
	return value
}

func safeEventType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 64 {
		return "unknown"
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '.' && char != '-' {
			return "unknown"
		}
	}
	return value
}
