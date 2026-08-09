package runtime

import (
	"context"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type YieldDispatchInput struct {
	Reason        string
	WaitRefs      []string
	SafeSummary   map[string]any
	UsedCostMinor int64
}

type YieldDispatchResult struct {
	Yield  domain.RuntimeYield `json:"yield"`
	Handle DispatchHandle      `json:"handle"`
	Job    domain.JobRun       `json:"job"`
}

func (s *Service) RuntimeYield(ctx context.Context, tenantID, id string) (domain.RuntimeYield, error) {
	return s.repo.RuntimeYield(ctx, tenantID, id)
}

func (s *Service) RuntimeYields(ctx context.Context, tenantID, jobID string) ([]domain.RuntimeYield, error) {
	return s.repo.RuntimeYields(ctx, tenantID, jobID)
}

// YieldDispatch ends the current Attempt, releases its lease and reservations,
// and leaves the logical AgentInstance waiting on durable identities.
func (s *Service) YieldDispatch(ctx context.Context, handle DispatchHandle, input YieldDispatchInput) (YieldDispatchResult, error) {
	now := s.now().UTC()
	node, attempt, agent := handle.Node, handle.Attempt, handle.Agent
	if node.State != domain.NodeRunning || attempt.State != domain.RuntimeAttemptRunning || agent.State != domain.AgentActive {
		return YieldDispatchResult{Handle: handle}, domain.Conflict("DISPATCH_NOT_YIELDABLE", "只有当前运行中的调度尝试可以让出执行权")
	}
	if attempt.LeaseExpiresAt == nil || node.LeaseExpiresAt == nil || !attempt.LeaseExpiresAt.After(now) || !node.LeaseExpiresAt.After(now) || attempt.FenceToken == "" || attempt.FenceToken != node.FenceToken {
		return YieldDispatchResult{Handle: handle}, domain.Conflict("DISPATCH_LEASE_STALE", "让出请求的调度租约已经过期")
	}
	input.Reason = strings.TrimSpace(input.Reason)
	input.WaitRefs = sortedRefs(input.WaitRefs)
	if len(input.WaitRefs) == 0 {
		return YieldDispatchResult{Handle: handle}, domain.Invalid("RUNTIME_YIELD_REFS_REQUIRED", "让出执行权必须冻结至少一个等待引用")
	}
	nodeState, agentState := "", ""
	switch input.Reason {
	case domain.YieldWaitChildren:
		nodeState, agentState = domain.NodeWaitingChildren, domain.AgentWaitingChildren
	case domain.YieldWaitHuman:
		nodeState, agentState = domain.NodeWaitingHuman, domain.AgentWaitingGate
	case domain.YieldWaitEffect:
		nodeState, agentState = domain.NodeWaitingExternal, domain.AgentWaitingEffect
	default:
		return YieldDispatchResult{Handle: handle}, domain.Invalid("RUNTIME_YIELD_REASON_INVALID", "让出执行权的等待原因无效")
	}
	if err := s.validateYieldRefs(ctx, node, input.Reason, input.WaitRefs, false); err != nil {
		return YieldDispatchResult{Handle: handle}, err
	}
	if input.UsedCostMinor < agent.UsedCostMinor || input.UsedCostMinor > agent.BudgetMinor {
		return YieldDispatchResult{Handle: handle}, domain.Policy("AGENT_INSTANCE_USAGE_INVALID", "AgentInstance 用量不能回退或超过预算", "重新核对累计用量")
	}
	if input.SafeSummary == nil {
		input.SafeSummary = map[string]any{}
	}
	input.SafeSummary = sanitizeSafeSummary(input.SafeSummary)
	digest, err := domain.CanonicalHash(struct {
		AttemptID string
		Reason    string
		WaitRefs  []string
		Summary   map[string]any
	}{attempt.ID, input.Reason, input.WaitRefs, input.SafeSummary})
	if err != nil {
		return YieldDispatchResult{Handle: handle}, err
	}
	expectedNodeVersion, expectedAttemptVersion, expectedAgentVersion := node.Version, attempt.Version, agent.Version
	if err := node.Transition(nodeState); err != nil {
		return YieldDispatchResult{Handle: handle}, err
	}
	if err := attempt.Transition(domain.RuntimeAttemptYielded); err != nil {
		return YieldDispatchResult{Handle: handle}, err
	}
	if err := agent.Transition(agentState); err != nil {
		return YieldDispatchResult{Handle: handle}, err
	}
	fenceToken := attempt.FenceToken
	node.State, node.LeaseOwner, node.FenceToken, node.LeaseExpiresAt = nodeState, "", "", nil
	node.Version++
	node.UpdatedAt = now
	attempt.State = domain.RuntimeAttemptYielded
	attempt.ResultDigest = "sha256:" + digest
	attempt.SafeSummary = input.SafeSummary
	attempt.LeaseOwner, attempt.FenceToken, attempt.LeaseExpiresAt = "", "", nil
	attempt.FinishedAt = &now
	attempt.Version++
	attempt.UpdatedAt = now
	agent.State = agentState
	agent.UsedCostMinor = input.UsedCostMinor
	agent.Version++
	agent.UpdatedAt = now
	yielded := domain.RuntimeYield{ID: domain.NewID(), TenantID: node.TenantID, JobRunID: node.JobRunID, NodeRunID: node.ID, AttemptID: attempt.ID, AgentInstanceID: agent.ID, Reason: input.Reason, WaitRefs: input.WaitRefs, State: domain.RuntimeYieldOpen, Version: 1, YieldedAt: now, CreatedAt: now, UpdatedAt: now}
	event := domain.JobEvent{ID: domain.NewID(), TenantID: node.TenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "attempt.yielded", ActorType: "worker", ActorID: handle.Attempt.LeaseOwner, IdempotencyKey: attempt.ID + ":yielded", Payload: map[string]any{"attempt_id": attempt.ID, "yield_id": yielded.ID, "reason": yielded.Reason, "wait_refs": yielded.WaitRefs, "yield_digest": attempt.ResultDigest}, OccurredAt: now}
	yielded, node, attempt, agent, err = s.repo.YieldDispatch(ctx, yielded, node, expectedNodeVersion, attempt, expectedAttemptVersion, agent, expectedAgentVersion, fenceToken, event)
	if err != nil {
		return YieldDispatchResult{Handle: handle}, err
	}
	handle.Node, handle.Attempt, handle.Agent = node, attempt, agent
	job, err := s.Refresh(ctx, node.TenantID, node.JobRunID)
	return YieldDispatchResult{Yield: yielded, Handle: handle, Job: job}, err
}

// ResumeYield resolves one durable wait. The node becomes ready; the scheduler
// creates a new Attempt, optionally resuming the preserved harness session.
func (s *Service) ResumeYield(ctx context.Context, tenantID, yieldID, resumeKey, actorID string) (domain.RuntimeYield, error) {
	resumeKey = strings.TrimSpace(resumeKey)
	if resumeKey == "" {
		return domain.RuntimeYield{}, domain.Invalid("RUNTIME_YIELD_RESUME_KEY_REQUIRED", "恢复 RuntimeYield 必须提供幂等键")
	}
	yielded, err := s.repo.RuntimeYield(ctx, tenantID, yieldID)
	if err != nil {
		return yielded, err
	}
	if yielded.State == domain.RuntimeYieldResolved {
		if yielded.ResumeKey == resumeKey {
			return yielded, nil
		}
		return yielded, domain.Conflict("RUNTIME_YIELD_ALREADY_RESOLVED", "RuntimeYield 已由其他恢复请求处理")
	}
	node, err := s.repo.NodeRun(ctx, tenantID, yielded.NodeRunID)
	if err != nil {
		return yielded, err
	}
	agent, err := s.repo.AgentInstance(ctx, tenantID, yielded.AgentInstanceID)
	if err != nil {
		return yielded, err
	}
	if err := s.validateYieldRefs(ctx, node, yielded.Reason, yielded.WaitRefs, true); err != nil {
		return yielded, err
	}
	expectedYieldVersion, expectedNodeVersion, expectedAgentVersion := yielded.Version, node.Version, agent.Version
	if err := node.Transition(domain.NodeReady); err != nil {
		return yielded, err
	}
	if err := agent.Transition(domain.AgentRunnable); err != nil {
		return yielded, err
	}
	now := s.now().UTC()
	yielded.State, yielded.ResumeKey, yielded.ResolvedAt = domain.RuntimeYieldResolved, resumeKey, &now
	yielded.Version++
	yielded.UpdatedAt = now
	node.State = domain.NodeReady
	node.Version++
	node.UpdatedAt = now
	agent.State = domain.AgentRunnable
	agent.Version++
	agent.UpdatedAt = now
	yielded, _, _, err = s.repo.ResolveRuntimeYield(ctx, yielded, expectedYieldVersion, node, expectedNodeVersion, agent, expectedAgentVersion, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: yielded.JobRunID, NodeKey: node.NodeKey, Type: "attempt.resume_ready", ActorType: "runtime", ActorID: strings.TrimSpace(actorID), IdempotencyKey: "yield-resume:" + resumeKey, Payload: map[string]any{"yield_id": yielded.ID, "attempt_id": yielded.AttemptID, "reason": yielded.Reason}, OccurredAt: now})
	return yielded, err
}

func (s *Service) validateYieldRefs(ctx context.Context, node domain.NodeRun, reason string, refs []string, requireReady bool) error {
	for _, ref := range refs {
		switch reason {
		case domain.YieldWaitChildren:
			child, err := s.repo.NodeRun(ctx, node.TenantID, ref)
			if err != nil {
				return err
			}
			if child.JobRunID != node.JobRunID || child.ID == node.ID {
				return domain.Invalid("RUNTIME_YIELD_CHILD_SCOPE_INVALID", "等待的子节点不属于当前执行实例")
			}
			if requireReady && child.State != domain.NodeSucceeded && child.State != domain.NodeSkipped {
				if child.State == domain.NodeFailed || child.State == domain.NodeBlocked || child.State == domain.NodeCancelled {
					return domain.Conflict("RUNTIME_YIELD_CHILD_FAILED", "等待的子节点已经失败，不能恢复父节点")
				}
				return domain.Conflict("RUNTIME_YIELD_NOT_READY", "等待的子节点尚未完成")
			}
		case domain.YieldWaitEffect:
			effect, err := s.repo.Effect(ctx, node.TenantID, ref)
			if err != nil {
				return err
			}
			if effect.JobRunID != node.JobRunID || effect.NodeRunID != node.ID {
				return domain.Invalid("RUNTIME_YIELD_EFFECT_SCOPE_INVALID", "等待的外部操作不属于当前执行节点")
			}
			if requireReady && effect.State != domain.EffectSucceeded {
				if effect.State == domain.EffectFailed || effect.State == domain.EffectManual {
					return domain.Conflict("RUNTIME_YIELD_EFFECT_FAILED", "等待的外部操作未成功，不能自动恢复节点")
				}
				return domain.Conflict("RUNTIME_YIELD_NOT_READY", "等待的外部操作尚未收敛")
			}
		case domain.YieldWaitHuman:
			if strings.TrimSpace(ref) == "" {
				return domain.Invalid("RUNTIME_YIELD_HUMAN_REF_INVALID", "人工等待引用不能为空")
			}
		}
	}
	return nil
}
