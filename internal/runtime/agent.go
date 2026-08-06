package runtime

import (
	"context"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

// AgentInstanceInput contains caller-selected execution policy. Runtime-owned
// lifecycle fields such as ID, depth, state, version and timestamps are derived
// by the service.
type AgentInstanceInput struct {
	TenantID              string
	JobRunID              string
	NodeRunID             string
	ParentAgentInstanceID string
	Role                  string
	HarnessKind           string
	SessionRef            string
	ExecutionProfileID    string
	ContextViewID         string
	RemainingDescendants  int
	BudgetMinor           int64
}

func (s *Service) CreateContextView(ctx context.Context, input ContextViewInput) (domain.ContextView, error) {
	if s == nil || s.repo == nil {
		return domain.ContextView{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.JobRunID = strings.TrimSpace(input.JobRunID)
	input.NodeRunID = strings.TrimSpace(input.NodeRunID)
	if input.CreatedAt.IsZero() {
		input.CreatedAt = s.now().UTC()
	}
	job, err := s.repo.JobRun(ctx, input.TenantID, input.JobRunID)
	if err != nil {
		return domain.ContextView{}, err
	}
	node, err := s.repo.NodeRun(ctx, input.TenantID, input.NodeRunID)
	if err != nil {
		return domain.ContextView{}, err
	}
	if job.TenantID != input.TenantID || node.TenantID != input.TenantID || node.JobRunID != job.ID {
		return domain.ContextView{}, domain.Invalid("CONTEXT_VIEW_SCOPE_INVALID", "ContextView 引用的 JobRun 与 NodeRun 不属于同一执行范围")
	}
	view, err := BuildContextView(input)
	if err != nil {
		return domain.ContextView{}, err
	}
	if err := s.repo.CreateContextView(ctx, view); err != nil {
		return domain.ContextView{}, err
	}
	return view, nil
}

func (s *Service) CreateAgentInstance(ctx context.Context, input AgentInstanceInput) (domain.AgentInstance, error) {
	if s == nil || s.repo == nil {
		return domain.AgentInstance{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.JobRunID = strings.TrimSpace(input.JobRunID)
	input.NodeRunID = strings.TrimSpace(input.NodeRunID)
	input.ParentAgentInstanceID = strings.TrimSpace(input.ParentAgentInstanceID)
	input.Role = strings.TrimSpace(input.Role)
	input.HarnessKind = strings.TrimSpace(input.HarnessKind)
	input.ExecutionProfileID = strings.TrimSpace(input.ExecutionProfileID)
	input.ContextViewID = strings.TrimSpace(input.ContextViewID)
	if input.RemainingDescendants < 0 || input.BudgetMinor < 0 {
		return domain.AgentInstance{}, domain.Invalid("AGENT_INSTANCE_POLICY_INVALID", "AgentInstance 派生数量或预算不能为负数")
	}

	job, err := s.repo.JobRun(ctx, input.TenantID, input.JobRunID)
	if err != nil {
		return domain.AgentInstance{}, err
	}
	node, err := s.repo.NodeRun(ctx, input.TenantID, input.NodeRunID)
	if err != nil {
		return domain.AgentInstance{}, err
	}
	view, err := s.repo.ContextView(ctx, input.TenantID, input.ContextViewID)
	if err != nil {
		return domain.AgentInstance{}, err
	}
	if job.TenantID != input.TenantID || node.TenantID != input.TenantID || node.JobRunID != job.ID || view.TenantID != input.TenantID || view.JobRunID != job.ID || view.NodeRunID != node.ID {
		return domain.AgentInstance{}, domain.Invalid("AGENT_INSTANCE_SCOPE_INVALID", "AgentInstance、ContextView、JobRun 与 NodeRun 必须属于同一执行范围")
	}
	if !view.ExpiresAt.After(s.now().UTC()) {
		return domain.AgentInstance{}, domain.Policy("CONTEXT_VIEW_EXPIRED", "ContextView 已过期，不能创建 AgentInstance", "重新生成本次执行的 ContextView")
	}
	if input.BudgetMinor > view.BudgetMinor {
		return domain.AgentInstance{}, domain.Policy("AGENT_INSTANCE_BUDGET_ESCALATION", "AgentInstance 预算不能超过 ContextView 预算", "缩小预算后重试")
	}

	plan, err := s.repo.Plan(ctx, input.TenantID, job.PlanRevisionID)
	if err != nil {
		return domain.AgentInstance{}, err
	}
	depth := 0
	if input.RemainingDescendants > plan.Limits.MaxDynamicDescendants {
		return domain.AgentInstance{}, domain.Policy("AGENT_INSTANCE_DESCENDANT_LIMIT", "AgentInstance 派生数量超过执行计划上限", "缩小派生数量后重试")
	}
	if plan.Limits.MaxCostMinor > 0 && input.BudgetMinor > plan.Limits.MaxCostMinor {
		return domain.AgentInstance{}, domain.Policy("AGENT_INSTANCE_BUDGET_LIMIT", "AgentInstance 预算超过执行计划上限", "缩小预算后重试")
	}
	if input.ParentAgentInstanceID != "" {
		parent, err := s.repo.AgentInstance(ctx, input.TenantID, input.ParentAgentInstanceID)
		if err != nil {
			return domain.AgentInstance{}, err
		}
		if parent.TenantID != input.TenantID || parent.JobRunID != job.ID {
			return domain.AgentInstance{}, domain.Invalid("AGENT_INSTANCE_PARENT_SCOPE_INVALID", "父子 AgentInstance 必须属于同一 JobRun")
		}
		depth = parent.Depth + 1
		if depth > plan.Limits.MaxDepth {
			return domain.AgentInstance{}, domain.Policy("AGENT_INSTANCE_DEPTH_LIMIT", "子 AgentInstance 深度超过执行计划上限", "减少智能体嵌套层级")
		}
		if parent.RemainingDescendants < 1 || input.RemainingDescendants > parent.RemainingDescendants-1 {
			return domain.AgentInstance{}, domain.Policy("AGENT_INSTANCE_DESCENDANT_ESCALATION", "子 AgentInstance 派生能力超过父级剩余额度", "缩小子级派生数量后重试")
		}
		remainingBudget := parent.BudgetMinor - parent.UsedCostMinor
		if input.BudgetMinor > remainingBudget {
			return domain.AgentInstance{}, domain.Policy("AGENT_INSTANCE_BUDGET_ESCALATION", "子 AgentInstance 预算超过父级剩余预算", "缩小子级预算后重试")
		}
		parentView, err := s.repo.ContextView(ctx, input.TenantID, parent.ContextViewID)
		if err != nil {
			return domain.AgentInstance{}, err
		}
		if !isStringSubset(view.AllowedTools, parentView.AllowedTools) {
			return domain.AgentInstance{}, domain.Policy("AGENT_INSTANCE_TOOL_ESCALATION", "子 AgentInstance 不能扩大父级工具权限", "移除父级未授权的工具后重试")
		}
	}

	now := s.now().UTC()
	agent := domain.AgentInstance{
		ID:                    domain.NewID(),
		TenantID:              input.TenantID,
		JobRunID:              input.JobRunID,
		NodeRunID:             input.NodeRunID,
		ParentAgentInstanceID: input.ParentAgentInstanceID,
		Role:                  input.Role,
		HarnessKind:           input.HarnessKind,
		SessionRef:            strings.TrimSpace(input.SessionRef),
		ExecutionProfileID:    input.ExecutionProfileID,
		ContextViewID:         view.ID,
		State:                 domain.AgentCreated,
		Depth:                 depth,
		RemainingDescendants:  input.RemainingDescendants,
		BudgetMinor:           input.BudgetMinor,
		Version:               1,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := agent.Validate(); err != nil {
		return domain.AgentInstance{}, err
	}
	if err := s.repo.CreateAgentInstance(ctx, agent); err != nil {
		return domain.AgentInstance{}, err
	}
	return agent, nil
}

func (s *Service) AgentInstance(ctx context.Context, tenantID, id string) (domain.AgentInstance, error) {
	return s.repo.AgentInstance(ctx, tenantID, id)
}

func (s *Service) AgentInstances(ctx context.Context, tenantID, jobID string) ([]domain.AgentInstance, error) {
	return s.repo.AgentInstances(ctx, tenantID, jobID)
}

func (s *Service) ContextView(ctx context.Context, tenantID, id string) (domain.ContextView, error) {
	return s.repo.ContextView(ctx, tenantID, id)
}

func (s *Service) ContextViews(ctx context.Context, tenantID, jobID string) ([]domain.ContextView, error) {
	return s.repo.ContextViews(ctx, tenantID, jobID)
}

func (s *Service) TransitionAgentInstance(ctx context.Context, tenantID, id, next, sessionRef string, usedCostMinor int64, expectedVersion int) (domain.AgentInstance, error) {
	agent, err := s.repo.AgentInstance(ctx, tenantID, id)
	if err != nil {
		return domain.AgentInstance{}, err
	}
	if agent.Version != expectedVersion {
		return domain.AgentInstance{}, domain.Conflict("AGENT_INSTANCE_VERSION_CONFLICT", "AgentInstance 已被更新，请重新读取")
	}
	if usedCostMinor < agent.UsedCostMinor || usedCostMinor > agent.BudgetMinor {
		return domain.AgentInstance{}, domain.Policy("AGENT_INSTANCE_USAGE_INVALID", "AgentInstance 用量不能回退或超过预算", "重新核对累计用量")
	}
	if err := agent.Transition(next); err != nil {
		return domain.AgentInstance{}, err
	}
	agent.State = next
	agent.SessionRef = strings.TrimSpace(sessionRef)
	agent.UsedCostMinor = usedCostMinor
	agent.Version++
	agent.UpdatedAt = s.now().UTC()
	if err := s.repo.SaveAgentInstance(ctx, agent, expectedVersion); err != nil {
		return domain.AgentInstance{}, err
	}
	return agent, nil
}

func isStringSubset(candidate, allowed []string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	for _, value := range candidate {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}
