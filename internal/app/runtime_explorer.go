package app

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

// RuntimeJobSummary is an operations-only projection. It intentionally does
// not expose RuntimeState, input bodies, local paths, or provider credentials.
type RuntimeJobSummary struct {
	ID              string         `json:"id"`
	WorkTaskID      string         `json:"work_task_id"`
	TaskTitle       string         `json:"task_title"`
	ProjectID       string         `json:"project_id"`
	ProjectName     string         `json:"project_name"`
	State           string         `json:"state"`
	PlanDigest      string         `json:"plan_digest"`
	Priority        int            `json:"priority"`
	ErrorCode       string         `json:"error_code,omitempty"`
	NodeCount       int            `json:"node_count"`
	NodeStates      map[string]int `json:"node_states"`
	EffectCount     int            `json:"effect_count"`
	CheckpointCount int            `json:"checkpoint_count"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type RuntimeJobList struct {
	Items       []RuntimeJobSummary `json:"items"`
	NextAfter   int                 `json:"next_after,omitempty"`
	GeneratedAt time.Time           `json:"generated_at"`
}

type RuntimeNodeView struct {
	ID             string    `json:"id"`
	NodeKey        string    `json:"node_key"`
	Name           string    `json:"name"`
	Kind           string    `json:"kind"`
	CustomerStepID string    `json:"customer_step_id,omitempty"`
	State          string    `json:"state"`
	AttemptCount   int       `json:"attempt_count"`
	OutputDigest   string    `json:"output_digest,omitempty"`
	ErrorCode      string    `json:"error_code,omitempty"`
	LeaseOwner     string    `json:"lease_owner,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type RuntimeEventView struct {
	ID         string         `json:"id"`
	Sequence   int64          `json:"sequence"`
	Type       string         `json:"type"`
	NodeKey    string         `json:"node_key,omitempty"`
	ActorType  string         `json:"actor_type"`
	Payload    map[string]any `json:"payload"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type RuntimeEffectView struct {
	ID             string         `json:"id"`
	NodeRunID      string         `json:"node_run_id"`
	Kind           string         `json:"kind"`
	State          string         `json:"state"`
	ExternalID     string         `json:"external_id,omitempty"`
	RequestDigest  string         `json:"request_digest"`
	ResponseDigest string         `json:"response_digest,omitempty"`
	CostMinor      int64          `json:"cost_minor"`
	Currency       string         `json:"currency"`
	SafeSummary    map[string]any `json:"safe_summary"`
	ErrorCode      string         `json:"error_code,omitempty"`
	Version        int            `json:"version"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type RuntimeCheckpointView struct {
	ID             string    `json:"id"`
	NodeKey        string    `json:"node_key"`
	PlanDigest     string    `json:"plan_digest"`
	StateRefCount  int       `json:"state_ref_count"`
	OutputRefCount int       `json:"output_ref_count"`
	CompletedNodes []string  `json:"completed_nodes"`
	Digest         string    `json:"digest"`
	CreatedAt      time.Time `json:"created_at"`
}

type RuntimeContextView struct {
	ID            string    `json:"id"`
	NodeRunID     string    `json:"node_run_id"`
	AttemptID     string    `json:"attempt_id"`
	SchemaVersion string    `json:"schema_version"`
	InputRefCount int       `json:"input_ref_count"`
	StateRefCount int       `json:"state_ref_count"`
	EventRefCount int       `json:"event_ref_count"`
	AllowedTools  []string  `json:"allowed_tools"`
	MaxTokens     int       `json:"max_tokens"`
	BudgetMinor   int64     `json:"budget_minor"`
	Digest        string    `json:"digest"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type RuntimeAgentView struct {
	ID                    string             `json:"id"`
	NodeRunID             string             `json:"node_run_id"`
	ParentAgentInstanceID string             `json:"parent_agent_instance_id,omitempty"`
	Role                  string             `json:"role"`
	HarnessKind           string             `json:"harness_kind"`
	ExecutionProfileID    string             `json:"execution_profile_id"`
	ContextViewID         string             `json:"context_view_id"`
	SessionBound          bool               `json:"session_bound"`
	State                 string             `json:"state"`
	Depth                 int                `json:"depth"`
	RemainingDescendants  int                `json:"remaining_descendants"`
	BudgetMinor           int64              `json:"budget_minor"`
	UsedCostMinor         int64              `json:"used_cost_minor"`
	Version               int                `json:"version"`
	ContextView           RuntimeContextView `json:"context_view"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

type RuntimePlanView struct {
	ID            string                       `json:"id"`
	SOPID         string                       `json:"sop_id"`
	SOPVersion    int                          `json:"sop_version"`
	SOPDigest     string                       `json:"sop_digest"`
	SchemaVersion string                       `json:"schema_version"`
	Digest        string                       `json:"digest"`
	CustomerSteps []domain.JobPlanCustomerStep `json:"customer_steps"`
	CompiledAt    time.Time                    `json:"compiled_at"`
}

type RuntimeJobDetail struct {
	Summary     RuntimeJobSummary       `json:"summary"`
	Plan        RuntimePlanView         `json:"plan"`
	Nodes       []RuntimeNodeView       `json:"nodes"`
	Events      []RuntimeEventView      `json:"events"`
	Effects     []RuntimeEffectView     `json:"effects"`
	Checkpoints []RuntimeCheckpointView `json:"checkpoints"`
	Agents      []RuntimeAgentView      `json:"agents"`
	GeneratedAt time.Time               `json:"generated_at"`
}

func (s *Service) RuntimeJobs(ctx context.Context, actor Actor, projectID, state string, after, limit int) (RuntimeJobList, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobList{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobList{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if after < 0 {
		after = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	jobs, err := s.runtimeService.Jobs(ctx, actor.TenantID, "")
	if err != nil {
		return RuntimeJobList{}, err
	}
	filtered := make([]domain.JobRun, 0, len(jobs))
	for _, job := range jobs {
		if projectID != "" && job.ProjectID != projectID {
			continue
		}
		if state != "" && job.State != state {
			continue
		}
		filtered = append(filtered, job)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt) })
	if after > len(filtered) {
		after = len(filtered)
	}
	end := after + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	result := RuntimeJobList{Items: []RuntimeJobSummary{}, GeneratedAt: s.now().UTC()}
	for _, job := range filtered[after:end] {
		summary, summaryErr := s.runtimeJobSummary(ctx, actor, job)
		if summaryErr != nil {
			return RuntimeJobList{}, summaryErr
		}
		result.Items = append(result.Items, summary)
	}
	if end < len(filtered) {
		result.NextAfter = end
	}
	return result, nil
}

func (s *Service) RuntimeJobDetail(ctx context.Context, actor Actor, jobID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	job, err := s.runtimeService.Job(ctx, actor.TenantID, jobID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	plan, err := s.runtimeService.Plan(ctx, actor.TenantID, job.PlanRevisionID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	nodes, err := s.runtimeService.Nodes(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	events, err := s.runtimeService.Events(ctx, actor.TenantID, job.ID, 0)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	effects, err := s.runtimeService.Effects(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	checkpoints, err := s.runtimeService.Checkpoints(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	agents, err := s.runtimeService.AgentInstances(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	contextViews, err := s.runtimeService.ContextViews(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	summary, err := s.runtimeJobSummary(ctx, actor, job)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	nodeSpecs := map[string]domain.JobPlanNode{}
	for _, node := range plan.Nodes {
		nodeSpecs[node.Key] = node
	}
	result := RuntimeJobDetail{Summary: summary, Plan: RuntimePlanView{ID: plan.ID, SOPID: plan.SOPID, SOPVersion: plan.SOPVersion, SOPDigest: plan.SOPDigest, SchemaVersion: plan.SchemaVersion, Digest: plan.Digest, CustomerSteps: plan.CustomerSteps, CompiledAt: plan.CompiledAt}, Nodes: []RuntimeNodeView{}, Events: []RuntimeEventView{}, Effects: []RuntimeEffectView{}, Checkpoints: []RuntimeCheckpointView{}, Agents: []RuntimeAgentView{}, GeneratedAt: s.now().UTC()}
	for _, node := range nodes {
		spec := nodeSpecs[node.NodeKey]
		result.Nodes = append(result.Nodes, RuntimeNodeView{ID: node.ID, NodeKey: node.NodeKey, Name: spec.Name, Kind: spec.Kind, CustomerStepID: spec.CustomerStepID, State: node.State, AttemptCount: node.AttemptCount, OutputDigest: node.OutputDigest, ErrorCode: node.ErrorCode, LeaseOwner: node.LeaseOwner, UpdatedAt: node.UpdatedAt})
	}
	for _, event := range events {
		result.Events = append(result.Events, RuntimeEventView{ID: event.ID, Sequence: event.Sequence, Type: event.Type, NodeKey: event.NodeKey, ActorType: event.ActorType, Payload: sanitizeRuntimeMap(event.Payload), OccurredAt: event.OccurredAt})
	}
	for _, effect := range effects {
		result.Effects = append(result.Effects, RuntimeEffectView{ID: effect.ID, NodeRunID: effect.NodeRunID, Kind: effect.Kind, State: effect.State, ExternalID: effect.ExternalID, RequestDigest: effect.RequestDigest, ResponseDigest: effect.ResponseDigest, CostMinor: effect.CostMinor, Currency: effect.Currency, SafeSummary: sanitizeRuntimeMap(effect.SafeSummary), ErrorCode: effect.ErrorCode, Version: effect.Version, CreatedAt: effect.CreatedAt, UpdatedAt: effect.UpdatedAt})
	}
	for _, checkpoint := range checkpoints {
		result.Checkpoints = append(result.Checkpoints, RuntimeCheckpointView{ID: checkpoint.ID, NodeKey: checkpoint.NodeKey, PlanDigest: checkpoint.PlanDigest, StateRefCount: len(checkpoint.StateRefs), OutputRefCount: len(checkpoint.OutputRefs), CompletedNodes: append([]string{}, checkpoint.CompletedNodes...), Digest: checkpoint.Digest, CreatedAt: checkpoint.CreatedAt})
	}
	contextViewByID := make(map[string]domain.ContextView, len(contextViews))
	for _, view := range contextViews {
		contextViewByID[view.ID] = view
	}
	for _, agent := range agents {
		view, ok := contextViewByID[agent.ContextViewID]
		if !ok {
			return RuntimeJobDetail{}, domain.NotFound("AgentInstance ContextView")
		}
		result.Agents = append(result.Agents, RuntimeAgentView{
			ID: agent.ID, NodeRunID: agent.NodeRunID, ParentAgentInstanceID: agent.ParentAgentInstanceID,
			Role: agent.Role, HarnessKind: agent.HarnessKind, ExecutionProfileID: agent.ExecutionProfileID,
			ContextViewID: agent.ContextViewID, SessionBound: strings.TrimSpace(agent.SessionRef) != "",
			State: agent.State, Depth: agent.Depth, RemainingDescendants: agent.RemainingDescendants,
			BudgetMinor: agent.BudgetMinor, UsedCostMinor: agent.UsedCostMinor, Version: agent.Version,
			ContextView: RuntimeContextView{ID: view.ID, NodeRunID: view.NodeRunID, AttemptID: view.AttemptID, SchemaVersion: view.SchemaVersion, InputRefCount: len(view.InputRefs), StateRefCount: len(view.StateRefs), EventRefCount: len(view.EventRefs), AllowedTools: append([]string{}, view.AllowedTools...), MaxTokens: view.MaxTokens, BudgetMinor: view.BudgetMinor, Digest: view.Digest, CreatedAt: view.CreatedAt, ExpiresAt: view.ExpiresAt},
			CreatedAt:   agent.CreatedAt, UpdatedAt: agent.UpdatedAt,
		})
	}
	return result, nil
}

func (s *Service) RuntimeJobEvents(ctx context.Context, actor Actor, jobID string, after int64) ([]RuntimeEventView, error) {
	detail, err := s.RuntimeJobDetail(ctx, actor, jobID)
	if err != nil {
		return nil, err
	}
	if after <= 0 {
		return detail.Events, nil
	}
	result := make([]RuntimeEventView, 0, len(detail.Events))
	for _, event := range detail.Events {
		if event.Sequence > after {
			result = append(result, event)
		}
	}
	return result, nil
}

func (s *Service) RefreshRuntimeJob(ctx context.Context, actor Actor, jobID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if _, err := s.runtimeService.Refresh(ctx, actor.TenantID, jobID); err != nil {
		return RuntimeJobDetail{}, err
	}
	return s.RuntimeJobDetail(ctx, actor, jobID)
}

func (s *Service) CancelRuntimeJob(ctx context.Context, actor Actor, jobID, requestID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if _, err := s.runtimeService.Cancel(ctx, actor.TenantID, jobID, "user", actor.UserID); err != nil {
		return RuntimeJobDetail{}, err
	}
	s.audit(ctx, actor, "", "runtime.cancelled", "job_run", jobID, requestID, nil)
	return s.RuntimeJobDetail(ctx, actor, jobID)
}

func (s *Service) ResumeRuntimeJob(ctx context.Context, actor Actor, jobID, requestID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if _, err := s.runtimeService.Resume(ctx, actor.TenantID, jobID, "user", actor.UserID); err != nil {
		return RuntimeJobDetail{}, err
	}
	s.audit(ctx, actor, "", "runtime.resumed", "job_run", jobID, requestID, nil)
	return s.RuntimeJobDetail(ctx, actor, jobID)
}

func (s *Service) runtimeJobSummary(ctx context.Context, actor Actor, job domain.JobRun) (RuntimeJobSummary, error) {
	nodes, err := s.runtimeService.Nodes(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobSummary{}, err
	}
	effects, err := s.runtimeService.Effects(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobSummary{}, err
	}
	checkpoints, err := s.runtimeService.Checkpoints(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobSummary{}, err
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, job.WorkTaskID)
	if err != nil {
		return RuntimeJobSummary{}, err
	}
	project, err := s.store.Project(ctx, actor.TenantID, job.ProjectID)
	if err != nil {
		return RuntimeJobSummary{}, err
	}
	states := map[string]int{}
	for _, node := range nodes {
		states[node.State]++
	}
	return RuntimeJobSummary{ID: job.ID, WorkTaskID: job.WorkTaskID, TaskTitle: task.Title, ProjectID: job.ProjectID, ProjectName: project.BrandName, State: job.State, PlanDigest: job.PlanDigest, Priority: job.Priority, ErrorCode: job.ErrorCode, NodeCount: len(nodes), NodeStates: states, EffectCount: len(effects), CheckpointCount: len(checkpoints), CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt}, nil
}

func requireRuntimeOperator(actor Actor) error {
	if actor.PlatformAdmin || containsString([]string{"tenant_admin", "project_manager"}, actor.Role) {
		return nil
	}
	return domain.Policy("RUNTIME_ROLE_DENIED", "当前角色不能查看运行时控制台", "请联系项目管理员或平台运营人员授权")
}

func sanitizeRuntimeMap(input map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range input {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "credential") || strings.Contains(lower, "password") || strings.Contains(lower, "api_key") || strings.Contains(lower, "absolute_path") || lower == "path" {
			continue
		}
		switch nested := value.(type) {
		case map[string]any:
			result[key] = sanitizeRuntimeMap(nested)
		case []any:
			cleaned := make([]any, 0, len(nested))
			for _, item := range nested {
				if mapValue, ok := item.(map[string]any); ok {
					cleaned = append(cleaned, sanitizeRuntimeMap(mapValue))
				} else {
					cleaned = append(cleaned, item)
				}
			}
			result[key] = cleaned
		default:
			result[key] = value
		}
	}
	return result
}
