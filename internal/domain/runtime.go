package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Runtime schema identifiers are versioned independently from the customer
// projection. A plan or result is never interpreted by guessing its shape.
const (
	JobPlanSchema       = "contentcloud.job-plan/1.0"
	NodeExecutionSchema = "contentcloud.node-execution/1.0"
	NodeResultSchema    = "contentcloud.node-result/1.0"
	RuntimeStateSchema  = "contentcloud.runtime-state/1.0"
	ContextViewSchema   = "contentcloud.context-view/1.0"
	RuntimeEventSchema  = "contentcloud.runtime-event/1.0"

	JobRunCreated      = "created"
	JobRunAdmitted     = "admitted"
	JobRunRunning      = "running"
	JobRunWaitingHuman = "waiting_human"
	JobRunPaused       = "paused"
	JobRunCompleted    = "completed"
	JobRunFailed       = "failed"
	JobRunCancelled    = "cancelled"
	JobRunRejected     = "rejected"

	NodePending         = "pending"
	NodeReady           = "ready"
	NodeWaitingResource = "waiting_resource"
	NodeLeased          = "leased"
	NodeRunning         = "running"
	NodeWaitingExternal = "waiting_external"
	NodeWaitingHuman    = "waiting_human"
	NodeSucceeded       = "succeeded"
	NodeRetryableFailed = "retryable_failed"
	NodeFailed          = "failed"
	NodeBlocked         = "blocked"
	NodeSkipped         = "skipped"
	NodeCancelled       = "cancelled"
	NodeLeaseExpired    = "lease_expired"

	EffectRegistered   = "registered"
	EffectSubmitted    = "submitted"
	EffectAcknowledged = "acknowledged"
	EffectSucceeded    = "succeeded"
	EffectFailed       = "failed"
	EffectUnknown      = "unknown"
	EffectReconciling  = "reconciling"
	EffectManual       = "manual_action"

	AgentCreated         = "created"
	AgentRunnable        = "runnable"
	AgentActive          = "active"
	AgentWaitingChildren = "waiting_children"
	AgentWaitingGate     = "waiting_gate"
	AgentWaitingEffect   = "waiting_effect"
	AgentCompleted       = "completed"
	AgentFailed          = "failed"
	AgentCanceling       = "canceling"
	AgentCancelled       = "cancelled"

	RuntimeAttemptPrepared        = "prepared"
	RuntimeAttemptRunning         = "running"
	RuntimeAttemptSucceeded       = "succeeded"
	RuntimeAttemptRetryableFailed = "retryable_failed"
	RuntimeAttemptFailed          = "failed"
	RuntimeAttemptCancelled       = "cancelled"
	RuntimeAttemptExpired         = "expired"
)

type RuntimeLimits struct {
	MaxNodes              int   `json:"max_nodes"`
	MaxDepth              int   `json:"max_depth"`
	MaxDynamicDescendants int   `json:"max_dynamic_descendants"`
	MaxConcurrentNodes    int   `json:"max_concurrent_nodes"`
	MaxAttemptsPerNode    int   `json:"max_attempts_per_node"`
	MaxCostMinor          int64 `json:"max_cost_minor"`
}

func DefaultRuntimeLimits() RuntimeLimits {
	return RuntimeLimits{MaxNodes: 100, MaxDepth: 32, MaxDynamicDescendants: 100, MaxConcurrentNodes: 20, MaxAttemptsPerNode: 3, MaxCostMinor: 0}
}

type JobPlanNode struct {
	Key                  string   `json:"key"`
	Kind                 string   `json:"kind"`
	StageID              string   `json:"stage_id,omitempty"`
	GateID               string   `json:"gate_id,omitempty"`
	Name                 string   `json:"name"`
	DependsOn            []string `json:"depends_on"`
	InputRefs            []string `json:"input_refs"`
	OutputSchema         string   `json:"output_schema"`
	RequiredCapabilities []string `json:"required_capabilities"`
	ExecutionModes       []string `json:"execution_modes"`
	CustomerStepID       string   `json:"customer_step_id,omitempty"`
	SideEffectClass      string   `json:"side_effect_class"`
	RetryMaxAttempts     int      `json:"retry_max_attempts"`
}

type JobPlanEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type JobPlanCustomerStep struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	NodeKeys []string `json:"node_keys"`
}

// JobPlanRevision is immutable once a JobRun references it.
type JobPlanRevision struct {
	ID            string                `json:"id"`
	TenantID      string                `json:"tenant_id"`
	SOPID         string                `json:"sop_id"`
	SOPVersion    int                   `json:"sop_version"`
	SOPDigest     string                `json:"sop_digest"`
	SchemaVersion string                `json:"schema_version"`
	Digest        string                `json:"digest"`
	Nodes         []JobPlanNode         `json:"nodes"`
	Edges         []JobPlanEdge         `json:"edges"`
	CustomerSteps []JobPlanCustomerStep `json:"customer_steps"`
	Limits        RuntimeLimits         `json:"limits"`
	CompiledAt    time.Time             `json:"compiled_at"`
	CompiledBy    string                `json:"compiled_by"`
}

func (p *JobPlanRevision) NormalizeCollections() {
	if p.Nodes == nil {
		p.Nodes = []JobPlanNode{}
	}
	if p.Edges == nil {
		p.Edges = []JobPlanEdge{}
	}
	if p.CustomerSteps == nil {
		p.CustomerSteps = []JobPlanCustomerStep{}
	}
	for i := range p.Nodes {
		if p.Nodes[i].DependsOn == nil {
			p.Nodes[i].DependsOn = []string{}
		}
		if p.Nodes[i].InputRefs == nil {
			p.Nodes[i].InputRefs = []string{}
		}
		if p.Nodes[i].RequiredCapabilities == nil {
			p.Nodes[i].RequiredCapabilities = []string{}
		}
		if p.Nodes[i].ExecutionModes == nil {
			p.Nodes[i].ExecutionModes = []string{}
		}
	}
}

func (p JobPlanRevision) Validate() error {
	if p.ID == "" || p.TenantID == "" || p.SOPID == "" || p.SOPVersion < 1 || p.SchemaVersion != JobPlanSchema || p.Digest == "" {
		return Invalid("JOB_PLAN_INVALID", "执行计划缺少租户、流程版本、Schema 或摘要")
	}
	p.NormalizeCollections()
	if len(p.Nodes) == 0 || len(p.Nodes) > p.Limits.MaxNodes {
		return Invalid("JOB_PLAN_NODE_LIMIT", "执行计划节点数量不在允许范围内")
	}
	seen := map[string]bool{}
	for _, node := range p.Nodes {
		if strings.TrimSpace(node.Key) == "" || seen[node.Key] || strings.TrimSpace(node.Name) == "" || strings.TrimSpace(node.OutputSchema) == "" {
			return Invalid("JOB_PLAN_NODE_INVALID", "执行计划节点必须有唯一 Key、名称和输出 Schema")
		}
		seen[node.Key] = true
	}
	for _, edge := range p.Edges {
		if !seen[edge.From] || !seen[edge.To] || edge.From == edge.To {
			return Invalid("JOB_PLAN_EDGE_INVALID", "执行计划边引用无效或形成自环")
		}
	}
	if _, err := validateAcyclic(p.Nodes, p.Edges); err != nil {
		return err
	}
	return nil
}

func validateAcyclic(nodes []JobPlanNode, edges []JobPlanEdge) (int, error) {
	indegree := map[string]int{}
	adj := map[string][]string{}
	for _, n := range nodes {
		indegree[n.Key] = 0
	}
	for _, e := range edges {
		indegree[e.To]++
		adj[e.From] = append(adj[e.From], e.To)
	}
	queue := []string{}
	for key, degree := range indegree {
		if degree == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)
	count := 0
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		count++
		for _, next := range adj[key] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if count != len(nodes) {
		return count, Invalid("JOB_PLAN_CYCLE", "执行计划不能包含环")
	}
	return count, nil
}

type JobRun struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	WorkTaskID     string    `json:"work_task_id"`
	PlanRevisionID string    `json:"plan_revision_id"`
	PlanDigest     string    `json:"plan_digest"`
	SourceJobRunID string    `json:"source_job_run_id,omitempty"`
	CheckpointID   string    `json:"checkpoint_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	State          string    `json:"state"`
	Priority       int       `json:"priority"`
	Version        int       `json:"version"`
	ErrorCode      string    `json:"error_code,omitempty"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ContextView is an immutable, reference-only view of the inputs available to
// one execution attempt. It contains no source正文、secret or complete transcript.
type ContextView struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	JobRunID      string    `json:"job_run_id"`
	NodeRunID     string    `json:"node_run_id"`
	AttemptID     string    `json:"attempt_id"`
	SchemaVersion string    `json:"schema_version"`
	InputRefs     []string  `json:"input_refs"`
	StateRefs     []string  `json:"state_refs"`
	EventRefs     []string  `json:"event_refs"`
	AllowedTools  []string  `json:"allowed_tools"`
	MaxTokens     int       `json:"max_tokens"`
	BudgetMinor   int64     `json:"budget_minor"`
	Digest        string    `json:"digest"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (view ContextView) Validate() error {
	if view.ID == "" || view.TenantID == "" || view.JobRunID == "" || view.NodeRunID == "" || view.AttemptID == "" || view.SchemaVersion != ContextViewSchema || view.Digest == "" || view.MaxTokens <= 0 || view.BudgetMinor < 0 || view.ExpiresAt.IsZero() || view.CreatedAt.IsZero() || !view.ExpiresAt.After(view.CreatedAt) {
		return Invalid("CONTEXT_VIEW_INVALID", "ContextView 缺少执行引用、预算或有效期")
	}
	return nil
}

type AgentInstance struct {
	ID                    string    `json:"id"`
	TenantID              string    `json:"tenant_id"`
	JobRunID              string    `json:"job_run_id"`
	NodeRunID             string    `json:"node_run_id"`
	ParentAgentInstanceID string    `json:"parent_agent_instance_id,omitempty"`
	Role                  string    `json:"role"`
	HarnessKind           string    `json:"harness_kind"`
	SessionRef            string    `json:"session_ref,omitempty"`
	ExecutionProfileID    string    `json:"execution_profile_id"`
	ContextViewID         string    `json:"context_view_id"`
	State                 string    `json:"state"`
	Depth                 int       `json:"depth"`
	RemainingDescendants  int       `json:"remaining_descendants"`
	BudgetMinor           int64     `json:"budget_minor"`
	UsedCostMinor         int64     `json:"used_cost_minor"`
	Version               int       `json:"version"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (agent AgentInstance) Validate() error {
	if agent.ID == "" || agent.TenantID == "" || agent.JobRunID == "" || agent.NodeRunID == "" || agent.Role == "" || agent.HarnessKind == "" || agent.ExecutionProfileID == "" || agent.ContextViewID == "" || agent.State == "" || agent.Depth < 0 || agent.RemainingDescendants < 0 || agent.BudgetMinor < 0 || agent.UsedCostMinor < 0 || agent.UsedCostMinor > agent.BudgetMinor || agent.Version < 1 {
		return Invalid("AGENT_INSTANCE_INVALID", "AgentInstance 缺少身份、权限或预算约束")
	}
	if !validAgentState(agent.State) {
		return Invalid("AGENT_INSTANCE_STATE_INVALID", "AgentInstance 状态无效")
	}
	return nil
}

func validAgentState(state string) bool {
	switch state {
	case AgentCreated, AgentRunnable, AgentActive, AgentWaitingChildren, AgentWaitingGate, AgentWaitingEffect, AgentCompleted, AgentFailed, AgentCanceling, AgentCancelled:
		return true
	}
	return false
}

func (agent AgentInstance) Transition(next string) error {
	if !validAgentState(next) {
		return Invalid("AGENT_INSTANCE_STATE_INVALID", "AgentInstance 状态无效")
	}
	if agent.State == next {
		return nil
	}
	if agent.State == AgentCompleted || agent.State == AgentFailed || agent.State == AgentCancelled {
		return Conflict("AGENT_INSTANCE_TERMINAL", "终态 AgentInstance 不能原地恢复")
	}
	allowed := map[string]map[string]bool{
		AgentCreated:         {AgentRunnable: true, AgentCanceling: true},
		AgentRunnable:        {AgentActive: true, AgentFailed: true, AgentCanceling: true, AgentCancelled: true},
		AgentActive:          {AgentRunnable: true, AgentWaitingChildren: true, AgentWaitingGate: true, AgentWaitingEffect: true, AgentCompleted: true, AgentFailed: true, AgentCanceling: true, AgentCancelled: true},
		AgentWaitingChildren: {AgentRunnable: true, AgentCanceling: true, AgentFailed: true},
		AgentWaitingGate:     {AgentRunnable: true, AgentCanceling: true, AgentFailed: true},
		AgentWaitingEffect:   {AgentRunnable: true, AgentCanceling: true, AgentFailed: true},
		AgentCanceling:       {AgentCancelled: true},
	}
	if !allowed[agent.State][next] {
		return Conflict("AGENT_INSTANCE_TRANSITION_INVALID", fmt.Sprintf("AgentInstance 不能从 %s 转为 %s", agent.State, next))
	}
	return nil
}

// RuntimeAttempt is the authoritative execution-attempt model for the V8
// Runtime. The legacy RunAttempt remains scoped to the V7 TaskRun path.
type RuntimeAttempt struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	JobRunID        string         `json:"job_run_id"`
	NodeRunID       string         `json:"node_run_id"`
	AgentInstanceID string         `json:"agent_instance_id"`
	ContextViewID   string         `json:"context_view_id"`
	AttemptNo       int            `json:"attempt_no"`
	HarnessKind     string         `json:"harness_kind"`
	Capabilities    map[string]any `json:"capabilities"`
	SessionRef      string         `json:"session_ref,omitempty"`
	State           string         `json:"state"`
	LeaseOwner      string         `json:"lease_owner,omitempty"`
	LeaseExpiresAt  *time.Time     `json:"lease_expires_at,omitempty"`
	OutputRefs      []string       `json:"output_refs"`
	ResultDigest    string         `json:"result_digest,omitempty"`
	SafeSummary     map[string]any `json:"safe_summary"`
	ErrorCode       string         `json:"error_code,omitempty"`
	Version         int            `json:"version"`
	CreatedAt       time.Time      `json:"created_at"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (attempt RuntimeAttempt) Validate() error {
	if attempt.ID == "" || attempt.TenantID == "" || attempt.JobRunID == "" || attempt.NodeRunID == "" || attempt.AgentInstanceID == "" || attempt.ContextViewID == "" || attempt.AttemptNo < 1 || attempt.HarnessKind == "" || attempt.State == "" || attempt.Version < 1 || attempt.CreatedAt.IsZero() || attempt.UpdatedAt.IsZero() {
		return Invalid("RUNTIME_ATTEMPT_INVALID", "RuntimeAttempt 缺少执行范围、适配器或版本信息")
	}
	if !validRuntimeAttemptState(attempt.State) {
		return Invalid("RUNTIME_ATTEMPT_STATE_INVALID", "RuntimeAttempt 状态无效")
	}
	if attempt.State == RuntimeAttemptPrepared || attempt.State == RuntimeAttemptRunning {
		if strings.TrimSpace(attempt.LeaseOwner) == "" || attempt.LeaseExpiresAt == nil || !attempt.LeaseExpiresAt.After(attempt.UpdatedAt) || attempt.FinishedAt != nil {
			return Invalid("RUNTIME_ATTEMPT_LEASE_INVALID", "运行中的 RuntimeAttempt 缺少有效租约")
		}
	} else if attempt.LeaseOwner != "" || attempt.LeaseExpiresAt != nil || attempt.FinishedAt == nil {
		return Invalid("RUNTIME_ATTEMPT_TERMINAL_INVALID", "终态 RuntimeAttempt 必须释放租约并记录完成时间")
	}
	if attempt.State == RuntimeAttemptRunning && (attempt.SessionRef == "" || attempt.StartedAt == nil) {
		return Invalid("RUNTIME_ATTEMPT_SESSION_INVALID", "运行中的 RuntimeAttempt 缺少会话引用或启动时间")
	}
	if attempt.State == RuntimeAttemptSucceeded && attempt.ResultDigest == "" {
		return Invalid("RUNTIME_ATTEMPT_RESULT_INVALID", "成功的 RuntimeAttempt 缺少结果摘要")
	}
	return nil
}

func validRuntimeAttemptState(state string) bool {
	switch state {
	case RuntimeAttemptPrepared, RuntimeAttemptRunning, RuntimeAttemptSucceeded, RuntimeAttemptRetryableFailed, RuntimeAttemptFailed, RuntimeAttemptCancelled, RuntimeAttemptExpired:
		return true
	}
	return false
}

func (attempt RuntimeAttempt) Terminal() bool {
	switch attempt.State {
	case RuntimeAttemptSucceeded, RuntimeAttemptRetryableFailed, RuntimeAttemptFailed, RuntimeAttemptCancelled, RuntimeAttemptExpired:
		return true
	}
	return false
}

func (attempt RuntimeAttempt) Transition(next string) error {
	if !validRuntimeAttemptState(next) {
		return Invalid("RUNTIME_ATTEMPT_STATE_INVALID", "RuntimeAttempt 状态无效")
	}
	if attempt.State == next {
		return nil
	}
	if attempt.Terminal() {
		return Conflict("RUNTIME_ATTEMPT_TERMINAL", "终态 RuntimeAttempt 不能原地恢复")
	}
	allowed := map[string]map[string]bool{
		RuntimeAttemptPrepared: {
			RuntimeAttemptRunning: true, RuntimeAttemptRetryableFailed: true, RuntimeAttemptFailed: true,
			RuntimeAttemptCancelled: true, RuntimeAttemptExpired: true,
		},
		RuntimeAttemptRunning: {
			RuntimeAttemptSucceeded: true, RuntimeAttemptRetryableFailed: true, RuntimeAttemptFailed: true,
			RuntimeAttemptCancelled: true, RuntimeAttemptExpired: true,
		},
	}
	if !allowed[attempt.State][next] {
		return Conflict("RUNTIME_ATTEMPT_TRANSITION_INVALID", fmt.Sprintf("RuntimeAttempt 不能从 %s 转为 %s", attempt.State, next))
	}
	return nil
}

func (r JobRun) Validate() error {
	if r.ID == "" || r.TenantID == "" || r.ProjectID == "" || r.WorkTaskID == "" || r.PlanRevisionID == "" || r.State == "" || r.Version < 1 {
		return Invalid("JOB_RUN_INVALID", "JobRun 缺少任务、计划或状态")
	}
	if !validJobState(r.State) {
		return Invalid("JOB_RUN_STATE_INVALID", "JobRun 状态无效")
	}
	return nil
}

func validJobState(state string) bool {
	switch state {
	case JobRunCreated, JobRunAdmitted, JobRunRunning, JobRunWaitingHuman, JobRunPaused, JobRunCompleted, JobRunFailed, JobRunCancelled, JobRunRejected:
		return true
	}
	return false
}

func (r JobRun) Transition(next string) error {
	if !validJobState(next) {
		return Invalid("JOB_RUN_STATE_INVALID", "JobRun 状态无效")
	}
	if r.State == next {
		return nil
	}
	if r.State == JobRunCompleted || r.State == JobRunFailed || r.State == JobRunCancelled || r.State == JobRunRejected {
		return Conflict("JOB_RUN_TERMINAL", "终态 JobRun 不能原地恢复")
	}
	allowed := map[string]map[string]bool{
		JobRunCreated:      {JobRunAdmitted: true, JobRunRejected: true, JobRunCancelled: true},
		JobRunAdmitted:     {JobRunRunning: true, JobRunPaused: true, JobRunRejected: true, JobRunCancelled: true},
		JobRunRunning:      {JobRunWaitingHuman: true, JobRunPaused: true, JobRunCompleted: true, JobRunFailed: true, JobRunCancelled: true},
		JobRunWaitingHuman: {JobRunRunning: true, JobRunPaused: true, JobRunCancelled: true, JobRunFailed: true},
		JobRunPaused:       {JobRunRunning: true, JobRunCancelled: true},
	}
	if !allowed[r.State][next] {
		return Conflict("JOB_RUN_TRANSITION_INVALID", fmt.Sprintf("JobRun 不能从 %s 转为 %s", r.State, next))
	}
	return nil
}

type NodeRun struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	JobRunID       string     `json:"job_run_id"`
	NodeKey        string     `json:"node_key"`
	State          string     `json:"state"`
	AttemptCount   int        `json:"attempt_count"`
	OutputRefs     []string   `json:"output_refs"`
	OutputDigest   string     `json:"output_digest,omitempty"`
	ErrorCode      string     `json:"error_code,omitempty"`
	LeaseOwner     string     `json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (n NodeRun) Validate() error {
	if n.ID == "" || n.TenantID == "" || n.JobRunID == "" || n.NodeKey == "" || n.State == "" || n.Version < 1 {
		return Invalid("NODE_RUN_INVALID", "NodeRun 缺少执行实例、节点或状态")
	}
	if !validNodeState(n.State) {
		return Invalid("NODE_RUN_STATE_INVALID", "NodeRun 状态无效")
	}
	return nil
}

func validNodeState(state string) bool {
	switch state {
	case NodePending, NodeReady, NodeWaitingResource, NodeLeased, NodeRunning, NodeWaitingExternal, NodeWaitingHuman, NodeSucceeded, NodeRetryableFailed, NodeFailed, NodeBlocked, NodeSkipped, NodeCancelled, NodeLeaseExpired:
		return true
	}
	return false
}

func (n NodeRun) Transition(next string) error {
	if !validNodeState(next) {
		return Invalid("NODE_RUN_STATE_INVALID", "NodeRun 状态无效")
	}
	if n.State == next {
		return nil
	}
	if n.State == NodeSucceeded || n.State == NodeFailed || n.State == NodeSkipped || n.State == NodeCancelled {
		return Conflict("NODE_RUN_TERMINAL", "终态 NodeRun 不能原地恢复")
	}
	allowed := map[string]map[string]bool{
		NodePending:         {NodeReady: true, NodeBlocked: true, NodeCancelled: true, NodeSkipped: true},
		NodeReady:           {NodeLeased: true, NodeWaitingResource: true, NodeBlocked: true, NodeCancelled: true},
		NodeWaitingResource: {NodeReady: true, NodeBlocked: true, NodeCancelled: true},
		NodeLeased:          {NodeRunning: true, NodeRetryableFailed: true, NodeFailed: true, NodeLeaseExpired: true, NodeCancelled: true},
		NodeRunning:         {NodeSucceeded: true, NodeRetryableFailed: true, NodeFailed: true, NodeWaitingExternal: true, NodeWaitingHuman: true, NodeCancelled: true, NodeLeaseExpired: true},
		NodeWaitingExternal: {NodeSucceeded: true, NodeFailed: true, NodeRetryableFailed: true, NodeCancelled: true},
		NodeWaitingHuman:    {NodeSucceeded: true, NodeRetryableFailed: true, NodeFailed: true, NodeCancelled: true},
		NodeRetryableFailed: {NodeReady: true, NodeFailed: true, NodeCancelled: true},
		NodeLeaseExpired:    {NodeReady: true, NodeFailed: true, NodeCancelled: true},
	}
	if !allowed[n.State][next] {
		return Conflict("NODE_RUN_TRANSITION_INVALID", fmt.Sprintf("NodeRun 不能从 %s 转为 %s", n.State, next))
	}
	return nil
}

type JobEvent struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	JobRunID       string         `json:"job_run_id"`
	Sequence       int64          `json:"sequence"`
	Type           string         `json:"type"`
	NodeKey        string         `json:"node_key,omitempty"`
	ActorType      string         `json:"actor_type"`
	ActorID        string         `json:"actor_id,omitempty"`
	CorrelationID  string         `json:"correlation_id,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Payload        map[string]any `json:"payload"`
	OccurredAt     time.Time      `json:"occurred_at"`
}

// RuntimeOutboxMessage is the durable delivery record paired with a JobEvent.
// It is written in the same transaction as the authoritative snapshot and event.
type RuntimeOutboxMessage struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	EventID       string         `json:"event_id"`
	SchemaVersion string         `json:"schema_version"`
	Topic         string         `json:"topic"`
	AggregateID   string         `json:"aggregate_id"`
	Payload       map[string]any `json:"payload"`
	Attempts      int            `json:"attempts"`
	NextAttemptAt time.Time      `json:"next_attempt_at"`
	LockedBy      string         `json:"locked_by,omitempty"`
	LockedUntil   *time.Time     `json:"locked_until,omitempty"`
	DeliveredAt   *time.Time     `json:"delivered_at,omitempty"`
	LastError     string         `json:"last_error,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}

type RuntimeState struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	JobRunID      string         `json:"job_run_id"`
	Collection    string         `json:"collection"`
	SchemaVersion string         `json:"schema_version"`
	Revision      int            `json:"revision"`
	Values        map[string]any `json:"values"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type StateMutation struct {
	Collection       string           `json:"collection"`
	ExpectedRevision int              `json:"expected_revision"`
	Set              map[string]any   `json:"set"`
	Append           map[string][]any `json:"append"`
	IdempotencyKey   string           `json:"idempotency_key"`
}

type Checkpoint struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	JobRunID       string    `json:"job_run_id"`
	NodeKey        string    `json:"node_key"`
	PlanDigest     string    `json:"plan_digest"`
	StateRefs      []string  `json:"state_refs"`
	OutputRefs     []string  `json:"output_refs"`
	CompletedNodes []string  `json:"completed_nodes"`
	Digest         string    `json:"digest"`
	CreatedAt      time.Time `json:"created_at"`
}

type ExternalEffect struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	JobRunID       string         `json:"job_run_id"`
	NodeRunID      string         `json:"node_run_id"`
	Kind           string         `json:"kind"`
	IdempotencyKey string         `json:"idempotency_key"`
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

func (e ExternalEffect) Transition(next string) error {
	valid := map[string]bool{EffectRegistered: true, EffectSubmitted: true, EffectAcknowledged: true, EffectSucceeded: true, EffectFailed: true, EffectUnknown: true, EffectReconciling: true, EffectManual: true}
	if !valid[next] {
		return Invalid("EFFECT_STATE_INVALID", "外部操作状态无效")
	}
	if e.State == EffectSucceeded || e.State == EffectFailed || e.State == EffectManual {
		return Conflict("EFFECT_TERMINAL", "终态外部操作不能原地恢复")
	}
	allowed := map[string]map[string]bool{
		EffectRegistered:   {EffectSubmitted: true, EffectUnknown: true, EffectFailed: true},
		EffectSubmitted:    {EffectAcknowledged: true, EffectUnknown: true, EffectFailed: true},
		EffectAcknowledged: {EffectSucceeded: true, EffectFailed: true, EffectUnknown: true},
		EffectUnknown:      {EffectReconciling: true, EffectManual: true},
		EffectReconciling:  {EffectSucceeded: true, EffectFailed: true, EffectManual: true},
	}
	if !allowed[e.State][next] {
		return Conflict("EFFECT_TRANSITION_INVALID", fmt.Sprintf("外部操作不能从 %s 转为 %s", e.State, next))
	}
	return nil
}
