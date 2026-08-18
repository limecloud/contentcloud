package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"

	// RuntimeJobSummary is an operations-only projection. It intentionally does
	// not expose RuntimeState, input bodies, local paths, or provider credentials.
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
)

type RuntimeJobSummary struct {
	ID                string          `json:"id"`
	WorkTaskID        string          `json:"work_task_id"`
	BusinessType      string          `json:"business_type,omitempty"`
	TaskTitle         string          `json:"task_title"`
	CustomerName      string          `json:"customer_name"`
	ProjectID         string          `json:"project_id"`
	ProjectName       string          `json:"project_name"`
	ProductName       string          `json:"product_name"`
	ProductVersion    int             `json:"product_version"`
	CurrentStepID     string          `json:"current_step_id,omitempty"`
	CurrentStepName   string          `json:"current_step_name,omitempty"`
	CompletedSteps    int             `json:"completed_steps"`
	TotalSteps        int             `json:"total_steps"`
	TaskStatus        string          `json:"task_status"`
	TaskNextAction    string          `json:"task_next_action,omitempty"`
	State             string          `json:"state"`
	StatusSince       time.Time       `json:"status_since"`
	BlockingReason    string          `json:"blocking_reason,omitempty"`
	RecommendedAction string          `json:"recommended_action,omitempty"`
	Cost              RuntimeCostView `json:"cost"`
	PlanDigest        string          `json:"plan_digest"`
	BindingDigest     string          `json:"binding_digest"`
	InputDigest       string          `json:"input_digest"`
	RuntimePolicyID   string          `json:"runtime_policy_id"`
	ContractMajor     int             `json:"contract_major"`
	ContractMinor     int             `json:"contract_minor"`
	RootJobRunID      string          `json:"root_job_run_id"`
	SourceJobRunID    string          `json:"source_job_run_id,omitempty"`
	CheckpointID      string          `json:"checkpoint_id,omitempty"`
	Priority          int             `json:"priority"`
	ErrorCode         string          `json:"error_code,omitempty"`
	AllowedActions    []string        `json:"allowed_actions"`
	NodeCount         int             `json:"node_count"`
	NodeStates        map[string]int  `json:"node_states"`
	EffectCount       int             `json:"effect_count"`
	CheckpointCount   int             `json:"checkpoint_count"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type RuntimeCostView struct {
	Status      string `json:"status"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency,omitempty"`
	EffectCount int    `json:"effect_count"`
}

type RuntimeJobList struct {
	Items       []RuntimeJobSummary `json:"items"`
	NextAfter   int                 `json:"next_after,omitempty"`
	GeneratedAt time.Time           `json:"generated_at"`
}

type RuntimeExplorerPage[T any] struct {
	Items       []T       `json:"items"`
	NextAfter   int       `json:"next_after,omitempty"`
	GeneratedAt time.Time `json:"generated_at"`
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
	AllowedActions []string       `json:"allowed_actions"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type RuntimeProviderAttemptView struct {
	ID                 string         `json:"id"`
	GenerationJobID    string         `json:"generation_job_id"`
	AttemptNumber      int            `json:"attempt_number"`
	ProviderID         string         `json:"provider_id"`
	RuntimeEffectID    string         `json:"runtime_effect_id,omitempty"`
	ExternalJobID      string         `json:"external_job_id,omitempty"`
	ProviderState      string         `json:"provider_state"`
	HTTPStatus         int            `json:"http_status,omitempty"`
	ProviderRequestID  string         `json:"provider_request_id,omitempty"`
	EstimatedCostMinor int64          `json:"estimated_cost_minor"`
	ActualCostMinor    int64          `json:"actual_cost_minor"`
	Currency           string         `json:"currency"`
	LastPolledAt       *time.Time     `json:"last_polled_at,omitempty"`
	NextPollAt         *time.Time     `json:"next_poll_at,omitempty"`
	ErrorCode          string         `json:"error_code,omitempty"`
	SafeResponse       map[string]any `json:"safe_response"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type RuntimeProviderBillView struct {
	ID          string    `json:"id"`
	ProviderID  string    `json:"provider_id"`
	BillID      string    `json:"bill_id"`
	ExternalID  string    `json:"external_id"`
	EffectID    string    `json:"effect_id,omitempty"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	ObservedAt  time.Time `json:"observed_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type RuntimeProviderReconciliationView struct {
	ID            string         `json:"id"`
	EffectID      string         `json:"effect_id,omitempty"`
	ProviderID    string         `json:"provider_id"`
	ExternalID    string         `json:"external_id"`
	ObservedState string         `json:"observed_state"`
	ExpectedMinor int64          `json:"expected_minor"`
	ObservedMinor int64          `json:"observed_minor"`
	Currency      string         `json:"currency"`
	Reason        string         `json:"reason,omitempty"`
	Status        string         `json:"status"`
	SafeSummary   map[string]any `json:"safe_summary"`
	ResolvedAt    *time.Time     `json:"resolved_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type RuntimeAttemptView struct {
	ID             string         `json:"id"`
	NodeRunID      string         `json:"node_run_id"`
	AttemptNo      int            `json:"attempt_no"`
	HarnessKind    string         `json:"harness_kind"`
	State          string         `json:"state"`
	ExecutorRef    string         `json:"executor_ref,omitempty"`
	SessionBound   bool           `json:"session_bound"`
	LeaseExpiresAt *time.Time     `json:"lease_expires_at,omitempty"`
	ResultDigest   string         `json:"result_digest,omitempty"`
	SafeSummary    map[string]any `json:"safe_summary"`
	ErrorCode      string         `json:"error_code,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type RuntimeGateView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Mode      string    `json:"mode"`
	State     string    `json:"state"`
	Decision  string    `json:"decision,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RuntimeStateCollectionView struct {
	ID             string    `json:"id"`
	CollectionKey  string    `json:"collection_key"`
	Scope          string    `json:"scope"`
	SchemaID       string    `json:"schema_id"`
	SchemaRevision int       `json:"schema_revision"`
	Consistency    string    `json:"consistency"`
	WriterNodeKey  string    `json:"writer_node_key,omitempty"`
	RecordCount    int       `json:"record_count"`
	Revision       int       `json:"revision"`
	Watermark      int64     `json:"watermark"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type RuntimeCheckpointView struct {
	ID             string    `json:"id"`
	NodeKey        string    `json:"node_key"`
	PlanDigest     string    `json:"plan_digest"`
	StateRefCount  int       `json:"state_ref_count"`
	OutputRefCount int       `json:"output_ref_count"`
	CompletedNodes []string  `json:"completed_nodes"`
	Digest         string    `json:"digest"`
	AllowedActions []string  `json:"allowed_actions"`
	BlockedReason  string    `json:"blocked_reason,omitempty"`
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
	ID            string                               `json:"id"`
	SOPID         string                               `json:"sop_id"`
	SOPVersion    int                                  `json:"sop_version"`
	SOPDigest     string                               `json:"sop_digest"`
	SchemaVersion string                               `json:"schema_version"`
	Digest        string                               `json:"digest"`
	Edges         []RuntimePlanEdgeView                `json:"edges"`
	CustomerSteps []contentruntime.JobPlanCustomerStep `json:"customer_steps"`
	CompiledAt    time.Time                            `json:"compiled_at"`
}

type RuntimePlanEdgeView struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type RuntimeStateRecordView struct {
	ID             string         `json:"id"`
	CollectionID   string         `json:"collection_id"`
	Key            string         `json:"key"`
	Value          map[string]any `json:"value,omitempty"`
	ArtifactRef    string         `json:"artifact_ref,omitempty"`
	SchemaRevision int            `json:"schema_revision"`
	Version        int            `json:"version"`
	Digest         string         `json:"digest"`
	UpdatedBy      string         `json:"updated_by"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type RuntimeJobDetail struct {
	Summary                 RuntimeJobSummary                   `json:"summary"`
	Plan                    RuntimePlanView                     `json:"plan"`
	Nodes                   []RuntimeNodeView                   `json:"nodes"`
	Attempts                []RuntimeAttemptView                `json:"attempts"`
	Events                  []RuntimeEventView                  `json:"events"`
	Effects                 []RuntimeEffectView                 `json:"effects"`
	ProviderAttempts        []RuntimeProviderAttemptView        `json:"provider_attempts"`
	ProviderBills           []RuntimeProviderBillView           `json:"provider_bills"`
	ProviderReconciliations []RuntimeProviderReconciliationView `json:"provider_reconciliations"`
	Checkpoints             []RuntimeCheckpointView             `json:"checkpoints"`
	Agents                  []RuntimeAgentView                  `json:"agents"`
	Gates                   []RuntimeGateView                   `json:"gates"`
	StateCollections        []RuntimeStateCollectionView        `json:"state_collections"`
	StateRecords            []RuntimeStateRecordView            `json:"state_records"`
	GeneratedAt             time.Time                           `json:"generated_at"`
}

// RuntimeReplayResult is a read-only execution audit result. Replay never
// invokes a harness or provider; it only counts durable events after a cursor.
type RuntimeReplayResult struct {
	TenantID          string `json:"tenant_id"`
	JobRunID          string `json:"job_run_id"`
	RebuildRunID      string `json:"rebuild_run_id"`
	DryRun            bool   `json:"dry_run"`
	EventCount        int    `json:"event_count"`
	LastSequence      int64  `json:"last_sequence"`
	ExternalCalls     int    `json:"external_calls"`
	ProjectionRebuilt bool   `json:"projection_rebuilt"`
	IntegrityStatus   string `json:"integrity_status"`
}

func (s *RuntimeService) RuntimeJobs(ctx context.Context, actor Actor, projectID, state string, after, limit int) (RuntimeJobList, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobList{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobList{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	after, limit = normalizeRuntimePage(after, limit)
	jobs, hasMore, err := s.runtimeService.JobsPage(ctx, actor.TenantID, projectID, state, after, limit)
	if err != nil {
		return RuntimeJobList{}, err
	}
	result := RuntimeJobList{Items: []RuntimeJobSummary{}, GeneratedAt: s.now().UTC()}
	for _, job := range jobs {
		summary, summaryErr := s.runtimeJobSummary(ctx, actor, job)
		if summaryErr != nil {
			return RuntimeJobList{}, summaryErr
		}
		result.Items = append(result.Items, summary)
	}
	if hasMore {
		result.NextAfter = after + len(result.Items)
	}
	return result, nil
}

func (s *RuntimeService) RuntimeJobDetail(ctx context.Context, actor Actor, jobID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
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
	attempts, err := s.runtimeService.Attempts(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	providerAttempts, err := s.delivery.ProviderAttemptsByRuntimeJob(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	providerBills, err := s.runtimeService.Repository().ProviderBillRecords(ctx, actor.TenantID, "")
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	providerReconciliations, err := s.runtimeService.Repository().ProviderReconciliations(ctx, actor.TenantID, "")
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	stateCollections, err := s.runtimeService.StateCollections(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	contextViews, err := s.runtimeService.ContextViews(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	taskContext, err := s.runtimeTaskContext(ctx, actor, job)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	summary, err := s.runtimeJobSummaryForContext(ctx, actor, job, plan, nodes, effects, checkpoints, taskContext)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	planEdges := make([]RuntimePlanEdgeView, 0, len(plan.Edges))
	for _, edge := range plan.Edges {
		planEdges = append(planEdges, RuntimePlanEdgeView{From: edge.From, To: edge.To})
	}
	result := RuntimeJobDetail{Summary: summary, Plan: RuntimePlanView{ID: plan.ID, SOPID: plan.SOPID, SOPVersion: plan.SOPVersion, SOPDigest: plan.SOPDigest, SchemaVersion: plan.SchemaVersion, Digest: plan.Digest, Edges: planEdges, CustomerSteps: plan.CustomerSteps, CompiledAt: plan.CompiledAt}, Nodes: []RuntimeNodeView{}, Attempts: []RuntimeAttemptView{}, Events: []RuntimeEventView{}, Effects: []RuntimeEffectView{}, ProviderAttempts: []RuntimeProviderAttemptView{}, ProviderBills: []RuntimeProviderBillView{}, ProviderReconciliations: []RuntimeProviderReconciliationView{}, Checkpoints: []RuntimeCheckpointView{}, Agents: []RuntimeAgentView{}, Gates: []RuntimeGateView{}, StateCollections: []RuntimeStateCollectionView{}, StateRecords: []RuntimeStateRecordView{}, GeneratedAt: s.now().UTC()}
	result.Nodes = append(result.Nodes, runtimeNodeViews(plan, nodes)...)
	for _, attempt := range attempts {
		result.Attempts = append(result.Attempts, RuntimeAttemptView{
			ID: attempt.ID, NodeRunID: attempt.NodeRunID, AttemptNo: attempt.AttemptNo, HarnessKind: attempt.HarnessKind,
			State: attempt.State, ExecutorRef: attempt.LeaseOwner, SessionBound: strings.TrimSpace(attempt.SessionRef) != "",
			LeaseExpiresAt: attempt.LeaseExpiresAt, ResultDigest: attempt.ResultDigest, SafeSummary: sanitizeRuntimeMap(attempt.SafeSummary),
			ErrorCode: attempt.ErrorCode, CreatedAt: attempt.CreatedAt, StartedAt: attempt.StartedAt, FinishedAt: attempt.FinishedAt, UpdatedAt: attempt.UpdatedAt,
		})
	}
	for _, attempt := range providerAttempts {
		result.ProviderAttempts = append(result.ProviderAttempts, RuntimeProviderAttemptView{ID: attempt.ID, GenerationJobID: attempt.GenerationJobID, AttemptNumber: attempt.AttemptNumber, ProviderID: attempt.ProviderID, RuntimeEffectID: attempt.RuntimeEffectID, ExternalJobID: attempt.ExternalJobID, ProviderState: attempt.ProviderState, HTTPStatus: attempt.HTTPStatus, ProviderRequestID: attempt.ProviderRequestID, EstimatedCostMinor: attempt.EstimatedCostMinor, ActualCostMinor: attempt.ActualCostMinor, Currency: attempt.Currency, LastPolledAt: attempt.LastPolledAt, NextPollAt: attempt.NextPollAt, ErrorCode: attempt.ErrorCode, SafeResponse: sanitizeRuntimeMap(attempt.SafeResponseSummary), CreatedAt: attempt.CreatedAt, UpdatedAt: attempt.UpdatedAt})
	}
	for _, bill := range providerBills {
		if bill.JobRunID != job.ID {
			continue
		}
		result.ProviderBills = append(result.ProviderBills, RuntimeProviderBillView{ID: bill.ID, ProviderID: bill.ProviderID, BillID: bill.BillID, ExternalID: bill.ExternalID, EffectID: bill.EffectID, AmountMinor: bill.AmountMinor, Currency: bill.Currency, Status: bill.Status, ObservedAt: bill.ObservedAt, CreatedAt: bill.CreatedAt})
	}
	for _, reconciliation := range providerReconciliations {
		if reconciliation.JobRunID != job.ID {
			continue
		}
		result.ProviderReconciliations = append(result.ProviderReconciliations, RuntimeProviderReconciliationView{ID: reconciliation.ID, EffectID: reconciliation.EffectID, ProviderID: reconciliation.ProviderID, ExternalID: reconciliation.ExternalID, ObservedState: reconciliation.ObservedState, ExpectedMinor: reconciliation.ExpectedMinor, ObservedMinor: reconciliation.ObservedMinor, Currency: reconciliation.Currency, Reason: reconciliation.Reason, Status: reconciliation.Status, SafeSummary: sanitizeRuntimeMap(reconciliation.SafeSummary), ResolvedAt: reconciliation.ResolvedAt, CreatedAt: reconciliation.CreatedAt, UpdatedAt: reconciliation.UpdatedAt})
	}
	for _, event := range events {
		result.Events = append(result.Events, RuntimeEventView{ID: event.ID, Sequence: event.Sequence, Type: event.Type, NodeKey: event.NodeKey, ActorType: event.ActorType, Payload: sanitizeRuntimeMap(event.Payload), OccurredAt: event.OccurredAt})
	}
	result.Effects = append(result.Effects, runtimeEffectViews(effects)...)
	result.Checkpoints = append(result.Checkpoints, runtimeCheckpointViews(job, plan, checkpoints, nodes, effects)...)
	for _, gate := range taskContext.gates {
		name := gate.GateID
		mode := gate.GateMode
		if definition, ok := taskContext.gateDefinitions[gate.GateID]; ok {
			name = definition.Name
			mode = definition.Mode
		}
		result.Gates = append(result.Gates, RuntimeGateView{ID: gate.ID, Name: name, Mode: mode, State: gate.Status, Decision: gate.Decision, Reason: gate.Reason, UpdatedAt: gate.UpdatedAt})
	}
	for _, collection := range stateCollections {
		records, recordErr := s.runtimeService.StateRecords(ctx, actor.TenantID, collection.ID)
		if recordErr != nil {
			return RuntimeJobDetail{}, recordErr
		}
		result.StateCollections = append(result.StateCollections, RuntimeStateCollectionView{ID: collection.ID, CollectionKey: collection.CollectionKey, Scope: collection.Scope, SchemaID: collection.SchemaID, SchemaRevision: collection.SchemaRevision, Consistency: collection.Consistency, WriterNodeKey: collection.WriterNodeKey, RecordCount: len(records), Revision: collection.Revision, Watermark: collection.Watermark, UpdatedAt: collection.UpdatedAt})
		for _, record := range records {
			value, artifactRef := runtimeStateRecordValue(record)
			result.StateRecords = append(result.StateRecords, RuntimeStateRecordView{ID: record.ID, CollectionID: record.CollectionID, Key: record.Key, Value: value, ArtifactRef: artifactRef, SchemaRevision: record.SchemaRevision, Version: record.Version, Digest: record.Digest, UpdatedBy: record.UpdatedBy, UpdatedAt: record.UpdatedAt})
		}
	}
	contextViewByID := make(map[string]contentruntime.ContextView, len(contextViews))
	for _, view := range contextViews {
		contextViewByID[view.ID] = view
	}
	for _, agent := range agents {
		view, ok := contextViewByID[agent.ContextViewID]
		if !ok {
			return RuntimeJobDetail{}, fault.NotFound("AgentInstance ContextView")
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

func (s *RuntimeService) RuntimeJobNodesPage(ctx context.Context, actor Actor, jobID string, after, limit int) (RuntimeExplorerPage[RuntimeNodeView], error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeExplorerPage[RuntimeNodeView]{}, err
	}
	if s.runtimeService == nil {
		return RuntimeExplorerPage[RuntimeNodeView]{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	job, err := s.runtimeService.Job(ctx, actor.TenantID, jobID)
	if err != nil {
		return RuntimeExplorerPage[RuntimeNodeView]{}, err
	}
	plan, err := s.runtimeService.Plan(ctx, actor.TenantID, job.PlanRevisionID)
	if err != nil {
		return RuntimeExplorerPage[RuntimeNodeView]{}, err
	}
	after, limit = normalizeRuntimePage(after, limit)
	nodes, hasMore, err := s.runtimeService.NodesPage(ctx, actor.TenantID, job.ID, after, limit)
	if err != nil {
		return RuntimeExplorerPage[RuntimeNodeView]{}, err
	}
	result := RuntimeExplorerPage[RuntimeNodeView]{Items: runtimeNodeViews(plan, nodes), GeneratedAt: s.now().UTC()}
	if hasMore {
		result.NextAfter = after + len(result.Items)
	}
	return result, nil
}

func (s *RuntimeService) RuntimeJobEffectsPage(ctx context.Context, actor Actor, jobID string, after, limit int) (RuntimeExplorerPage[RuntimeEffectView], error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeExplorerPage[RuntimeEffectView]{}, err
	}
	if s.runtimeService == nil {
		return RuntimeExplorerPage[RuntimeEffectView]{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	job, err := s.runtimeService.Job(ctx, actor.TenantID, jobID)
	if err != nil {
		return RuntimeExplorerPage[RuntimeEffectView]{}, err
	}
	after, limit = normalizeRuntimePage(after, limit)
	effects, hasMore, err := s.runtimeService.EffectsPage(ctx, actor.TenantID, job.ID, after, limit)
	if err != nil {
		return RuntimeExplorerPage[RuntimeEffectView]{}, err
	}
	result := RuntimeExplorerPage[RuntimeEffectView]{Items: runtimeEffectViews(effects), GeneratedAt: s.now().UTC()}
	if hasMore {
		result.NextAfter = after + len(result.Items)
	}
	return result, nil
}

func (s *RuntimeService) RuntimeJobCheckpointsPage(ctx context.Context, actor Actor, jobID string, after, limit int) (RuntimeExplorerPage[RuntimeCheckpointView], error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeExplorerPage[RuntimeCheckpointView]{}, err
	}
	if s.runtimeService == nil {
		return RuntimeExplorerPage[RuntimeCheckpointView]{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	job, err := s.runtimeService.Job(ctx, actor.TenantID, jobID)
	if err != nil {
		return RuntimeExplorerPage[RuntimeCheckpointView]{}, err
	}
	plan, err := s.runtimeService.Plan(ctx, actor.TenantID, job.PlanRevisionID)
	if err != nil {
		return RuntimeExplorerPage[RuntimeCheckpointView]{}, err
	}
	nodes, err := s.runtimeService.Nodes(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeExplorerPage[RuntimeCheckpointView]{}, err
	}
	effects, err := s.runtimeService.Effects(ctx, actor.TenantID, job.ID)
	if err != nil {
		return RuntimeExplorerPage[RuntimeCheckpointView]{}, err
	}
	after, limit = normalizeRuntimePage(after, limit)
	checkpoints, hasMore, err := s.runtimeService.CheckpointsPage(ctx, actor.TenantID, job.ID, after, limit)
	if err != nil {
		return RuntimeExplorerPage[RuntimeCheckpointView]{}, err
	}
	result := RuntimeExplorerPage[RuntimeCheckpointView]{Items: runtimeCheckpointViews(job, plan, checkpoints, nodes, effects), GeneratedAt: s.now().UTC()}
	if hasMore {
		result.NextAfter = after + len(result.Items)
	}
	return result, nil
}

func normalizeRuntimePage(after, limit int) (int, int) {
	if after < 0 {
		after = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return after, limit
}

func runtimeNodeViews(plan contentruntime.JobPlanRevision, nodes []contentruntime.NodeRun) []RuntimeNodeView {
	nodeSpecs := make(map[string]contentruntime.JobPlanNode, len(plan.Nodes))
	for _, node := range plan.Nodes {
		nodeSpecs[node.Key] = node
	}
	result := make([]RuntimeNodeView, 0, len(nodes))
	for _, node := range nodes {
		spec := nodeSpecs[node.NodeKey]
		result = append(result, RuntimeNodeView{ID: node.ID, NodeKey: node.NodeKey, Name: spec.Name, Kind: spec.Kind, CustomerStepID: spec.CustomerStepID, State: node.State, AttemptCount: node.AttemptCount, OutputDigest: node.OutputDigest, ErrorCode: node.ErrorCode, LeaseOwner: node.LeaseOwner, UpdatedAt: node.UpdatedAt})
	}
	return result
}

func runtimeEffectViews(effects []contentruntime.ExternalEffect) []RuntimeEffectView {
	result := make([]RuntimeEffectView, 0, len(effects))
	for _, effect := range effects {
		allowedActions := []string{}
		if effect.State == contentruntime.EffectUnknown {
			allowedActions = append(allowedActions, "reconcile")
		}
		result = append(result, RuntimeEffectView{ID: effect.ID, NodeRunID: effect.NodeRunID, Kind: effect.Kind, State: effect.State, ExternalID: effect.ExternalID, RequestDigest: effect.RequestDigest, ResponseDigest: effect.ResponseDigest, CostMinor: effect.CostMinor, Currency: effect.Currency, SafeSummary: sanitizeRuntimeMap(effect.SafeSummary), ErrorCode: effect.ErrorCode, Version: effect.Version, AllowedActions: allowedActions, CreatedAt: effect.CreatedAt, UpdatedAt: effect.UpdatedAt})
	}
	return result
}

func runtimeCheckpointViews(job contentruntime.JobRun, plan contentruntime.JobPlanRevision, checkpoints []contentruntime.Checkpoint, nodes []contentruntime.NodeRun, effects []contentruntime.ExternalEffect) []RuntimeCheckpointView {
	result := make([]RuntimeCheckpointView, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		allowedActions, blockedReason := runtimeCheckpointActions(job, plan, checkpoint, nodes, effects)
		result = append(result, RuntimeCheckpointView{ID: checkpoint.ID, NodeKey: checkpoint.NodeKey, PlanDigest: checkpoint.PlanDigest, StateRefCount: len(checkpoint.StateRefs), OutputRefCount: len(checkpoint.OutputRefs), CompletedNodes: append([]string{}, checkpoint.CompletedNodes...), Digest: checkpoint.Digest, AllowedActions: allowedActions, BlockedReason: blockedReason, CreatedAt: checkpoint.CreatedAt})
	}
	return result
}

func (s *RuntimeService) RuntimeJobEvents(ctx context.Context, actor Actor, jobID string, after int64, limits ...int) ([]RuntimeEventView, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return nil, err
	}
	if s.runtimeService == nil {
		return nil, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if _, err := s.runtimeService.Job(ctx, actor.TenantID, jobID); err != nil {
		return nil, err
	}
	limit := 100
	if len(limits) > 0 && limits[0] > 0 && limits[0] <= 500 {
		limit = limits[0]
	}
	events, err := s.runtimeService.EventsPage(ctx, actor.TenantID, jobID, after, limit)
	if err != nil {
		return nil, err
	}
	result := make([]RuntimeEventView, 0, len(events))
	for _, event := range events {
		result = append(result, RuntimeEventView{ID: event.ID, Sequence: event.Sequence, Type: event.Type, NodeKey: event.NodeKey, ActorType: event.ActorType, Payload: sanitizeRuntimeMap(event.Payload), OccurredAt: event.OccurredAt})
	}
	return result, nil
}

func (s *RuntimeService) RefreshRuntimeJob(ctx context.Context, actor Actor, jobID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if _, err := s.runtimeService.Refresh(ctx, actor.TenantID, jobID); err != nil {
		return RuntimeJobDetail{}, err
	}
	return s.RuntimeJobDetail(ctx, actor, jobID)
}

func (s *RuntimeService) CancelRuntimeJob(ctx context.Context, actor Actor, jobID, requestID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if _, err := s.runtimeService.Cancel(ctx, actor.TenantID, jobID, "user", actor.UserID); err != nil {
		return RuntimeJobDetail{}, err
	}
	s.audit(ctx, actor, "", "runtime.cancelled", "job_run", jobID, requestID, nil)
	return s.RuntimeJobDetail(ctx, actor, jobID)
}

func (s *RuntimeService) PauseRuntimeJob(ctx context.Context, actor Actor, jobID, requestID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if _, err := s.runtimeService.Pause(ctx, actor.TenantID, jobID, "user", actor.UserID); err != nil {
		return RuntimeJobDetail{}, err
	}
	s.audit(ctx, actor, "", "runtime.paused", "job_run", jobID, requestID, nil)
	return s.RuntimeJobDetail(ctx, actor, jobID)
}

func (s *RuntimeService) ResumeRuntimeJob(ctx context.Context, actor Actor, jobID, requestID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if _, err := s.runtimeService.Resume(ctx, actor.TenantID, jobID, "user", actor.UserID); err != nil {
		return RuntimeJobDetail{}, err
	}
	s.audit(ctx, actor, "", "runtime.resumed", "job_run", jobID, requestID, nil)
	return s.RuntimeJobDetail(ctx, actor, jobID)
}

// ForkRuntimeCheckpoint creates a new JobRun from one immutable checkpoint.
// The caller must provide an idempotency key so a repeated operator action
// cannot create duplicate execution instances.
func (s *RuntimeService) ForkRuntimeCheckpoint(ctx context.Context, actor Actor, checkpointID, idempotencyKey, requestID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	checkpointID = strings.TrimSpace(checkpointID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if checkpointID == "" || idempotencyKey == "" {
		return RuntimeJobDetail{}, fault.Invalid("RUNTIME_FORK_INPUT_INVALID", "Fork 需要检查点和幂等键")
	}
	started, err := s.runtimeService.Fork(ctx, actor.TenantID, checkpointID, actor.UserID, idempotencyKey)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	s.audit(ctx, actor, "", "runtime.forked", "job_run", started.Job.ID, requestID, map[string]any{
		"source_job_run_id": started.Job.SourceJobRunID,
		"checkpoint_id":     checkpointID,
	})
	return s.RuntimeJobDetail(ctx, actor, started.Job.ID)
}

// ReplayRuntimeJob validates durable events and rebuilds the derived explorer
// projection. It is separate from execution and performs no external calls.
func (s *RuntimeService) ReplayRuntimeJob(ctx context.Context, actor Actor, jobID string, after int64) (RuntimeReplayResult, error) {
	return s.ReplayRuntimeJobWithOptions(ctx, actor, jobID, after, false)
}

func (s *RuntimeService) ReplayRuntimeJobWithOptions(ctx context.Context, actor Actor, jobID string, after int64, dryRun bool) (RuntimeReplayResult, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeReplayResult{}, err
	}
	if s.runtimeService == nil {
		return RuntimeReplayResult{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if _, err := s.runtimeService.Job(ctx, actor.TenantID, jobID); err != nil {
		return RuntimeReplayResult{}, err
	}
	if after < 0 {
		after = 0
	}
	result, err := s.runtimeService.ReplayWithOptions(ctx, actor.TenantID, jobID, after, dryRun)
	if err != nil {
		return RuntimeReplayResult{}, err
	}
	return RuntimeReplayResult{TenantID: result.TenantID, JobRunID: result.JobRunID, RebuildRunID: result.RebuildRunID, DryRun: result.DryRun, EventCount: result.EventCount, LastSequence: result.LastSequence, ExternalCalls: result.ExternalCalls, ProjectionRebuilt: result.ProjectionRebuilt, IntegrityStatus: result.IntegrityStatus}, nil
}

// BeginRuntimeEffectReconciliation is the only automatic operation available
// for an unknown external effect. It moves the effect to reconciling and
// never submits a provider request.
func (s *RuntimeService) BeginRuntimeEffectReconciliation(ctx context.Context, actor Actor, effectID string, expectedVersion int, requestID string) (RuntimeJobDetail, error) {
	if err := requireRuntimeOperator(actor); err != nil {
		return RuntimeJobDetail{}, err
	}
	if s.runtimeService == nil {
		return RuntimeJobDetail{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	effectID = strings.TrimSpace(effectID)
	if effectID == "" || expectedVersion < 1 {
		return RuntimeJobDetail{}, fault.Invalid("RUNTIME_EFFECT_RECONCILIATION_INVALID", "对账需要外部操作和有效版本号")
	}
	current, err := s.runtimeService.Effect(ctx, actor.TenantID, effectID)
	if err != nil {
		return RuntimeJobDetail{}, err
	}
	if _, err := s.runtimeService.BeginEffectReconciliation(ctx, actor.TenantID, effectID, expectedVersion); err != nil {
		return RuntimeJobDetail{}, err
	}
	s.audit(ctx, actor, "", "runtime.effect_reconciliation_started", "external_effect", effectID, requestID, map[string]any{"job_run_id": current.JobRunID})
	return s.RuntimeJobDetail(ctx, actor, current.JobRunID)
}

type runtimeTaskContext struct {
	task            work.WorkTask
	project         workspacedomain.Project
	environment     catalogdomain.Environment
	sopDefinition   catalogdomain.SOPDefinition
	sop             catalogdomain.SOPVersion
	stageRuns       []work.StageRun
	gates           []reviewdomain.GateEvaluation
	gateDefinitions map[string]catalogdomain.GateDefinition
}

func (s *RuntimeService) runtimeTaskContext(ctx context.Context, actor Actor, job contentruntime.JobRun) (runtimeTaskContext, error) {
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, job.WorkTaskID)
	if err != nil {
		if fault.IsNotFound(err) && job.BusinessType == "knowledge_extract" {
			project, projectErr := s.workspace.Project(ctx, actor.TenantID, job.ProjectID)
			if projectErr != nil {
				return runtimeTaskContext{}, projectErr
			}
			sop := knowledgeExtractionSOP()
			return runtimeTaskContext{
				task:    work.WorkTask{ID: job.WorkTaskID, TenantID: job.TenantID, ProjectID: job.ProjectID, SOPID: sop.SOPID, SOPVersion: sop.Version, SOPDigest: sop.Digest, Title: "知识候选提取", Intent: "从已接受证据生成可审核知识候选", ContentType: "knowledge_extract", Status: runtimeRunState(job.State), CurrentStageID: "knowledge_extract", NextAction: "等待 Runtime worker 提交结构化候选"},
				project: project, environment: catalogdomain.Environment{ID: "runtime-knowledge", TenantID: job.TenantID, Name: "知识提取 Runtime", Slug: "knowledge-extract", Status: "active"}, sopDefinition: catalogdomain.SOPDefinition{ID: sop.SOPID, TenantID: job.TenantID, Name: sop.Name, CurrentVersion: sop.Version}, sop: sop,
				stageRuns: []work.StageRun{}, gates: []reviewdomain.GateEvaluation{}, gateDefinitions: map[string]catalogdomain.GateDefinition{},
			}, nil
		}
		return runtimeTaskContext{}, err
	}
	project, err := s.workspace.Project(ctx, actor.TenantID, job.ProjectID)
	if err != nil {
		return runtimeTaskContext{}, err
	}
	environment, err := s.catalog.Environment(ctx, actor.TenantID, task.EnvironmentID)
	if err != nil {
		return runtimeTaskContext{}, err
	}
	summary, err := s.catalog.SOP(ctx, actor.TenantID, task.SOPID)
	if err != nil {
		return runtimeTaskContext{}, err
	}
	var sop catalogdomain.SOPVersion
	for _, candidate := range summary.Versions {
		if candidate.Version == task.SOPVersion {
			sop = candidate
			break
		}
	}
	if sop.ID == "" {
		return runtimeTaskContext{}, fault.NotFound("流程规范版本")
	}
	stageRuns, err := s.tasks.StageRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return runtimeTaskContext{}, err
	}
	gates, err := s.delivery.GateEvaluations(ctx, actor.TenantID, task.ID)
	if err != nil {
		return runtimeTaskContext{}, err
	}
	gateDefinitions := make(map[string]catalogdomain.GateDefinition, len(sop.Gates))
	for _, definition := range sop.Gates {
		gateDefinitions[definition.ID] = definition
	}
	return runtimeTaskContext{task: task, project: project, environment: environment, sopDefinition: summary.Definition, sop: sop, stageRuns: stageRuns, gates: gates, gateDefinitions: gateDefinitions}, nil
}

func (s *RuntimeService) runtimeJobSummary(ctx context.Context, actor Actor, job contentruntime.JobRun) (RuntimeJobSummary, error) {
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
	plan, err := s.runtimeService.Plan(ctx, actor.TenantID, job.PlanRevisionID)
	if err != nil {
		return RuntimeJobSummary{}, err
	}
	taskContext, err := s.runtimeTaskContext(ctx, actor, job)
	if err != nil {
		return RuntimeJobSummary{}, err
	}
	return s.runtimeJobSummaryForContext(ctx, actor, job, plan, nodes, effects, checkpoints, taskContext)
}

func (s *RuntimeService) runtimeJobSummaryForContext(_ context.Context, _ Actor, job contentruntime.JobRun, plan contentruntime.JobPlanRevision, nodes []contentruntime.NodeRun, effects []contentruntime.ExternalEffect, checkpoints []contentruntime.Checkpoint, taskContext runtimeTaskContext) (RuntimeJobSummary, error) {
	states := map[string]int{}
	for _, node := range nodes {
		states[node.State]++
	}
	nodeByKey := make(map[string]contentruntime.NodeRun, len(nodes))
	for _, node := range nodes {
		nodeByKey[node.NodeKey] = node
	}
	completedNode := func(node contentruntime.NodeRun) bool {
		return node.State == contentruntime.NodeSucceeded || node.State == contentruntime.NodeSkipped
	}
	completedSteps := 0
	totalSteps := len(plan.CustomerSteps)
	if totalSteps == 0 {
		totalSteps = len(plan.Nodes)
		for _, node := range nodes {
			if completedNode(node) {
				completedSteps++
			}
		}
	} else {
		for _, step := range plan.CustomerSteps {
			allDone := len(step.NodeKeys) > 0
			for _, key := range step.NodeKeys {
				node, ok := nodeByKey[key]
				if !ok || !completedNode(node) {
					allDone = false
					break
				}
			}
			if allDone {
				completedSteps++
			}
		}
	}
	currentStepID, currentStepName := runtimeCurrentStep(taskContext, plan, nodes)
	cost := runtimeCost(effects)
	blockingReason, recommendedAction := runtimeBusinessGuidance(job, taskContext, effects, nodes)
	return RuntimeJobSummary{
		ID: job.ID, WorkTaskID: job.WorkTaskID, BusinessType: job.BusinessType, TaskTitle: taskContext.task.Title, CustomerName: taskContext.environment.Name,
		ProjectID: job.ProjectID, ProjectName: taskContext.project.BrandName, ProductName: taskContext.sopDefinition.Name, ProductVersion: taskContext.task.SOPVersion,
		CurrentStepID: currentStepID, CurrentStepName: currentStepName, CompletedSteps: completedSteps, TotalSteps: totalSteps,
		TaskStatus: taskContext.task.Status, TaskNextAction: taskContext.task.NextAction, State: job.State, StatusSince: job.UpdatedAt,
		BlockingReason: blockingReason, RecommendedAction: recommendedAction, Cost: cost,
		PlanDigest: job.PlanDigest, BindingDigest: job.BindingDigest, InputDigest: job.InputDigest, RuntimePolicyID: job.RuntimePolicyID,
		ContractMajor: job.ContractMajor, ContractMinor: job.ContractMinor, RootJobRunID: job.RootJobRunID, SourceJobRunID: job.SourceJobRunID,
		CheckpointID: job.CheckpointID, Priority: job.Priority, ErrorCode: job.ErrorCode, AllowedActions: runtimeJobActions(job), NodeCount: len(nodes),
		NodeStates: states, EffectCount: len(effects), CheckpointCount: len(checkpoints), CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}, nil
}

func runtimeCurrentStep(taskContext runtimeTaskContext, plan contentruntime.JobPlanRevision, nodes []contentruntime.NodeRun) (string, string) {
	stepNames := make(map[string]string, len(taskContext.sop.Stages))
	for _, stage := range taskContext.sop.Stages {
		stepNames[stage.ID] = stage.Name
	}
	if id := strings.TrimSpace(taskContext.task.CurrentStageID); id != "" {
		if name := stepNames[id]; name != "" {
			return id, name
		}
		for _, step := range plan.CustomerSteps {
			if step.ID == id {
				return step.ID, step.Title
			}
		}
		return id, id
	}
	for _, node := range nodes {
		if node.State != contentruntime.NodeRunning && node.State != contentruntime.NodeLeased && node.State != contentruntime.NodeWaitingHuman && node.State != contentruntime.NodeWaitingExternal && node.State != contentruntime.NodeReady {
			continue
		}
		for _, spec := range plan.Nodes {
			if spec.Key != node.NodeKey {
				continue
			}
			if spec.CustomerStepID != "" {
				for _, step := range plan.CustomerSteps {
					if step.ID == spec.CustomerStepID {
						return step.ID, step.Title
					}
				}
			}
			return spec.Key, spec.Name
		}
	}
	return "", ""
}

func runtimeCost(effects []contentruntime.ExternalEffect) RuntimeCostView {
	view := RuntimeCostView{Status: "not_recorded"}
	currency := ""
	for _, effect := range effects {
		if effect.CostMinor <= 0 {
			continue
		}
		view.EffectCount++
		if currency == "" {
			currency = effect.Currency
		} else if currency != effect.Currency {
			view.Status = "mixed_currency"
		}
		view.AmountMinor += effect.CostMinor
	}
	if view.EffectCount > 0 && view.Status != "mixed_currency" {
		view.Status = "recorded"
		view.Currency = currency
	}
	return view
}

func runtimeBusinessGuidance(job contentruntime.JobRun, taskContext runtimeTaskContext, effects []contentruntime.ExternalEffect, nodes []contentruntime.NodeRun) (string, string) {
	for _, effect := range effects {
		if effect.State == contentruntime.EffectUnknown || effect.State == contentruntime.EffectReconciling {
			return "外部请求结果尚未确认", "先完成外部结果核对，再决定任务是否继续"
		}
	}
	for _, gate := range taskContext.gates {
		if gate.Status == reviewdomain.GateEvaluationPending {
			return "等待客户或审核人员确认", "联系当前确认人完成待处理决定"
		}
	}
	for _, node := range nodes {
		if node.State == contentruntime.NodeFailed || node.State == contentruntime.NodeBlocked {
			return "当前步骤未完成，执行出现异常", "检查失败步骤；必要时从安全检查点创建新的执行分支"
		}
	}
	switch job.State {
	case contentruntime.JobRunPaused:
		return "任务已暂停后续处理", "确认阻断原因已解除后恢复处理"
	case contentruntime.JobRunCancelled:
		return "任务已取消", "无需继续执行；如需重做请创建新的任务运行"
	case contentruntime.JobRunCompleted:
		return "任务已完成", "无需处理"
	case contentruntime.JobRunFailed:
		return "任务运行失败", "检查失败步骤和运行事件，再选择安全恢复方式"
	case contentruntime.JobRunWaitingHuman:
		return "任务正在等待人工确认", "联系当前确认人完成决定"
	default:
		if taskContext.task.NextAction != "" {
			return "任务正在按计划处理", taskContext.task.NextAction
		}
		return "任务正在按计划处理", "继续观察当前任务"
	}
}

func runtimeJobActions(job contentruntime.JobRun) []string {
	actions := []string{"replay", "refresh"}
	if job.State == contentruntime.JobRunPaused {
		actions = append(actions, "resume")
	}
	switch job.State {
	case contentruntime.JobRunCompleted, contentruntime.JobRunFailed, contentruntime.JobRunCancelled, contentruntime.JobRunRejected:
	default:
		if job.State != contentruntime.JobRunPaused {
			actions = append(actions, "pause")
		}
		actions = append(actions, "cancel")
	}
	return actions
}

func runtimeCheckpointActions(job contentruntime.JobRun, plan contentruntime.JobPlanRevision, checkpoint contentruntime.Checkpoint, nodes []contentruntime.NodeRun, effects []contentruntime.ExternalEffect) ([]string, string) {
	switch job.State {
	case contentruntime.JobRunPaused, contentruntime.JobRunCompleted, contentruntime.JobRunFailed, contentruntime.JobRunCancelled, contentruntime.JobRunRejected:
	default:
		return []string{}, "先暂停或结束源执行实例"
	}
	if checkpoint.PlanDigest != plan.Digest || job.PlanDigest != plan.Digest {
		return []string{}, "检查点与执行计划摘要不一致"
	}
	if len(checkpoint.StateWatermarks) > 0 {
		return []string{}, "检查点包含当前版本无法安全继承的共享状态水位"
	}
	for _, effect := range effects {
		if effect.State == contentruntime.EffectUnknown || effect.State == contentruntime.EffectReconciling {
			return []string{}, "先完成结果不明的外部副作用对账"
		}
	}
	nodeByKey := make(map[string]contentruntime.NodeRun, len(nodes))
	for _, node := range nodes {
		nodeByKey[node.NodeKey] = node
	}
	for _, key := range checkpoint.CompletedNodes {
		node, ok := nodeByKey[key]
		if !ok || (node.State != contentruntime.NodeSucceeded && node.State != contentruntime.NodeSkipped) {
			return []string{}, "检查点记录的已完成节点与源执行实例不一致"
		}
	}
	return []string{"fork"}, ""
}

func requireRuntimeOperator(actor Actor) error {
	if actor.PlatformAdmin || containsString([]string{"tenant_admin", "project_manager"}, actor.Role) {
		return nil
	}
	return fault.Policy("RUNTIME_ROLE_DENIED", "当前角色不能查看运行时控制台", "请联系项目管理员或平台运营人员授权")
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

func runtimeStateRecordValue(record contentruntime.StateRecord) (map[string]any, string) {
	if record.ArtifactRef != "" {
		return nil, record.ArtifactRef
	}
	if record.Value == nil {
		return map[string]any{}, ""
	}
	body, err := json.Marshal(record.Value)
	if err != nil || len(body) > 32<<10 {
		return map[string]any{"redacted": "value_too_large", "digest": record.Digest}, ""
	}
	return sanitizeRuntimeMap(record.Value), ""
}
