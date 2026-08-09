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
	NodeWaitingChildren = "waiting_children"
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

	ToolCallProposed   = "proposed"
	ToolCallAuthorized = "authorized"
	ToolCallRunning    = "running"
	ToolCallSucceeded  = "succeeded"
	ToolCallFailed     = "failed"
	ToolCallUnknown    = "unknown"

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
	RuntimeAttemptYielded         = "yielded"
	RuntimeAttemptSucceeded       = "succeeded"
	RuntimeAttemptRetryableFailed = "retryable_failed"
	RuntimeAttemptFailed          = "failed"
	RuntimeAttemptCancelled       = "cancelled"
	RuntimeAttemptExpired         = "expired"

	ReservationHeld     = "held"
	ReservationConsumed = "consumed"
	ReservationReleased = "released"
	ReservationExpired  = "expired"
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

const (
	FanoutOpen      = "open"
	FanoutClosed    = "closed"
	FanoutSucceeded = "succeeded"
	FanoutFailed    = "failed"

	FanoutMemberPending   = "pending"
	FanoutMemberRunning   = "running"
	FanoutMemberSucceeded = "succeeded"
	FanoutMemberFailed    = "failed"
	FanoutMemberCancelled = "cancelled"
	FanoutMemberSkipped   = "skipped"

	JoinAll        = "all"
	JoinMinSuccess = "min_success"
	JoinQuorum     = "quorum"
	JoinBestEffort = "best_effort"
	JoinFailFast   = "fail_fast"

	ZeroMemberFail         = "fail"
	ZeroMemberSucceedEmpty = "succeed_empty"
	QuorumWaitAllTerminal  = "wait_all_terminal"
	QuorumCancelPending    = "cancel_pending"
)

// JoinPolicy is frozen with a FanoutSet. Runtime never guesses a completion
// rule from the current number of rows.
type JoinPolicy struct {
	Strategy         string `json:"strategy"`
	MinSuccess       int    `json:"min_success,omitempty"`
	QuorumPercent    int    `json:"quorum_percent,omitempty"`
	ZeroMemberPolicy string `json:"zero_member_policy"`
	QuorumStopPolicy string `json:"quorum_stop_policy,omitempty"`
}

func NormalizeJoinPolicy(policy JoinPolicy) JoinPolicy {
	if policy.ZeroMemberPolicy == "" {
		policy.ZeroMemberPolicy = ZeroMemberFail
	}
	return policy
}

func (p JoinPolicy) Validate() error {
	p = NormalizeJoinPolicy(p)
	switch p.Strategy {
	case JoinAll, JoinMinSuccess, JoinQuorum, JoinBestEffort, JoinFailFast:
	default:
		return Invalid("FANOUT_JOIN_POLICY_INVALID", "FanoutSet 汇聚策略无效")
	}
	if p.Strategy == JoinMinSuccess && p.MinSuccess < 1 {
		return Invalid("FANOUT_JOIN_POLICY_INVALID", "min_success 必须大于零")
	}
	if p.Strategy == JoinQuorum && (p.QuorumPercent < 1 || p.QuorumPercent > 100) {
		return Invalid("FANOUT_JOIN_POLICY_INVALID", "quorum 百分比必须在 1 到 100 之间")
	}
	if p.ZeroMemberPolicy != ZeroMemberFail && p.ZeroMemberPolicy != ZeroMemberSucceedEmpty {
		return Invalid("FANOUT_JOIN_POLICY_INVALID", "zero_member_policy 无效")
	}
	if p.Strategy == JoinQuorum {
		if p.QuorumStopPolicy == "" {
			return Invalid("FANOUT_JOIN_POLICY_INVALID", "quorum 必须声明停止策略")
		}
		if p.QuorumStopPolicy != QuorumWaitAllTerminal && p.QuorumStopPolicy != QuorumCancelPending {
			return Invalid("FANOUT_JOIN_POLICY_INVALID", "quorum_stop_policy 无效")
		}
	}
	return nil
}

type FanoutSet struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	JobRunID         string     `json:"job_run_id"`
	MapNodeKey       string     `json:"map_node_key"`
	JoinNodeKey      string     `json:"join_node_key"`
	SourceCollection string     `json:"source_collection"`
	SourceRevision   int        `json:"source_revision"`
	SourceWatermark  int64      `json:"source_watermark"`
	Generation       int        `json:"generation"`
	IdempotencyKey   string     `json:"idempotency_key"`
	MembershipDigest string     `json:"membership_digest"`
	RequestDigest    string     `json:"request_digest"`
	MemberCount      int        `json:"member_count"`
	JoinPolicy       JoinPolicy `json:"join_policy"`
	Status           string     `json:"status"`
	Version          int        `json:"version"`
	ClosedAt         *time.Time `json:"closed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (set FanoutSet) Validate() error {
	if set.ID == "" || set.TenantID == "" || set.JobRunID == "" || strings.TrimSpace(set.MapNodeKey) == "" || strings.TrimSpace(set.JoinNodeKey) == "" || strings.TrimSpace(set.IdempotencyKey) == "" || set.Generation < 1 || set.MemberCount < 0 || set.Version < 1 || set.CreatedAt.IsZero() || set.UpdatedAt.IsZero() {
		return Invalid("FANOUT_SET_INVALID", "FanoutSet 缺少执行范围、节点、代次或幂等键")
	}
	if len(set.MembershipDigest) != 71 || !strings.HasPrefix(set.MembershipDigest, "sha256:") || len(set.RequestDigest) != 71 || !strings.HasPrefix(set.RequestDigest, "sha256:") {
		return Invalid("FANOUT_SET_DIGEST_INVALID", "FanoutSet 缺少成员集合或请求摘要")
	}
	if err := NormalizeJoinPolicy(set.JoinPolicy).Validate(); err != nil {
		return err
	}
	switch set.Status {
	case FanoutOpen:
		if set.ClosedAt != nil {
			return Invalid("FANOUT_SET_STATE_INVALID", "开放中的 FanoutSet 不能有封存时间")
		}
	case FanoutClosed, FanoutSucceeded, FanoutFailed:
		if set.ClosedAt == nil {
			return Invalid("FANOUT_SET_STATE_INVALID", "已封存 FanoutSet 必须记录封存时间")
		}
	default:
		return Invalid("FANOUT_SET_STATE_INVALID", "FanoutSet 状态无效")
	}
	return nil
}

type FanoutMember struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	FanoutSetID  string    `json:"fanout_set_id"`
	MemberKey    string    `json:"member_key"`
	ItemKey      string    `json:"item_key"`
	ItemDigest   string    `json:"item_digest"`
	Generation   int       `json:"generation"`
	NodeRunID    string    `json:"node_run_id"`
	State        string    `json:"state"`
	OutputRefs   []string  `json:"output_refs"`
	OutputDigest string    `json:"output_digest,omitempty"`
	ErrorCode    string    `json:"error_code,omitempty"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func DeterministicFanoutMemberKey(jobRunID, mapNodeKey, fanoutSetID, itemKey string, generation int) (string, error) {
	if strings.TrimSpace(jobRunID) == "" || strings.TrimSpace(mapNodeKey) == "" || strings.TrimSpace(fanoutSetID) == "" || strings.TrimSpace(itemKey) == "" || generation < 1 {
		return "", Invalid("FANOUT_MEMBER_KEY_INVALID", "Fanout 成员键缺少执行范围、节点、集合、项目键或代次")
	}
	digest, err := CanonicalHash(struct {
		JobRunID    string `json:"job_run_id"`
		MapNodeKey  string `json:"map_node_key"`
		FanoutSetID string `json:"fanout_set_id"`
		ItemKey     string `json:"item_key"`
		Generation  int    `json:"generation"`
	}{jobRunID, mapNodeKey, fanoutSetID, itemKey, generation})
	if err != nil {
		return "", err
	}
	return "fanout:" + digest, nil
}

func (member FanoutMember) Validate() error {
	if member.ID == "" || member.TenantID == "" || member.FanoutSetID == "" || member.MemberKey == "" || member.ItemKey == "" || member.ItemDigest == "" || member.NodeRunID == "" || member.Generation < 1 || member.Version < 1 || member.CreatedAt.IsZero() || member.UpdatedAt.IsZero() {
		return Invalid("FANOUT_MEMBER_INVALID", "FanoutMember 缺少集合、成员键、输入摘要或执行节点")
	}
	switch member.State {
	case FanoutMemberPending, FanoutMemberRunning, FanoutMemberSucceeded, FanoutMemberFailed, FanoutMemberCancelled, FanoutMemberSkipped:
	default:
		return Invalid("FANOUT_MEMBER_STATE_INVALID", "FanoutMember 状态无效")
	}
	if member.OutputRefs == nil {
		return Invalid("FANOUT_MEMBER_OUTPUT_INVALID", "FanoutMember 输出引用必须显式为空数组或包含引用")
	}
	return nil
}

type JoinDecision struct {
	Status           string
	Terminal         bool
	Successful       int
	RequiredSuccess  int
	CancelMemberKeys []string
}

// EvaluateJoin is a pure, deterministic reducer over one closed FanoutSet.
// It intentionally consumes the frozen members, never a live source query.
func EvaluateJoin(set FanoutSet, members []FanoutMember) (JoinDecision, error) {
	set.JoinPolicy = NormalizeJoinPolicy(set.JoinPolicy)
	if err := set.JoinPolicy.Validate(); err != nil {
		return JoinDecision{}, err
	}
	if set.Status == FanoutOpen {
		return JoinDecision{}, Conflict("FANOUT_SET_NOT_CLOSED", "FanoutSet 尚未封存，不能汇聚")
	}
	if len(members) != set.MemberCount {
		return JoinDecision{}, Conflict("FANOUT_MEMBER_COUNT_MISMATCH", "FanoutSet 成员数量与封存快照不一致")
	}
	if len(members) == 0 {
		if set.JoinPolicy.ZeroMemberPolicy == ZeroMemberSucceedEmpty {
			return JoinDecision{Status: FanoutSucceeded, Terminal: true}, nil
		}
		return JoinDecision{Status: FanoutFailed, Terminal: true}, nil
	}
	successful, terminal, failed := 0, 0, 0
	cancel := []string{}
	for _, member := range members {
		switch member.State {
		case FanoutMemberSucceeded:
			successful++
		case FanoutMemberFailed:
			failed++
			terminal++
		case FanoutMemberCancelled, FanoutMemberSkipped:
			terminal++
		case FanoutMemberPending, FanoutMemberRunning:
		default:
			return JoinDecision{}, Invalid("FANOUT_MEMBER_STATE_INVALID", "FanoutSet 包含未知成员状态")
		}
		if member.State == FanoutMemberSucceeded {
			terminal++
		}
	}
	decision := JoinDecision{Successful: successful, CancelMemberKeys: cancel}
	required := 0
	switch set.JoinPolicy.Strategy {
	case JoinAll:
		required = len(members)
		if failed > 0 {
			decision.Status, decision.Terminal = FanoutFailed, true
		} else if terminal == len(members) {
			if successful == len(members) {
				decision.Status, decision.Terminal = FanoutSucceeded, true
			} else {
				decision.Status, decision.Terminal = FanoutFailed, true
			}
		}
	case JoinMinSuccess:
		required = set.JoinPolicy.MinSuccess
		if successful+len(members)-terminal < required {
			decision.Status, decision.Terminal = FanoutFailed, true
		} else if terminal == len(members) {
			decision.Status, decision.Terminal = FanoutSucceeded, successful >= required
		}
	case JoinQuorum:
		required = (len(members)*set.JoinPolicy.QuorumPercent + 99) / 100
		if successful+len(members)-terminal < required {
			decision.Status, decision.Terminal = FanoutFailed, true
		} else if successful >= required && set.JoinPolicy.QuorumStopPolicy == QuorumCancelPending {
			for _, member := range members {
				if member.State == FanoutMemberPending {
					decision.CancelMemberKeys = append(decision.CancelMemberKeys, member.MemberKey)
				}
			}
			if terminal == len(members) || terminal+len(decision.CancelMemberKeys) == len(members) {
				decision.Status, decision.Terminal = FanoutSucceeded, true
			}
		} else if terminal == len(members) {
			decision.Status, decision.Terminal = FanoutSucceeded, successful >= required
		}
	case JoinBestEffort:
		if terminal == len(members) {
			if successful > 0 {
				decision.Status, decision.Terminal = FanoutSucceeded, true
			} else {
				decision.Status, decision.Terminal = FanoutFailed, true
			}
		}
	case JoinFailFast:
		if failed > 0 {
			for _, member := range members {
				if member.State == FanoutMemberPending {
					decision.CancelMemberKeys = append(decision.CancelMemberKeys, member.MemberKey)
				}
			}
			decision.Status, decision.Terminal = FanoutFailed, true
		} else if terminal == len(members) {
			if successful == len(members) {
				decision.Status, decision.Terminal = FanoutSucceeded, true
			} else {
				decision.Status, decision.Terminal = FanoutFailed, true
			}
		}
	}
	decision.RequiredSuccess = required
	return decision, nil
}

// JobPlanRevision is immutable once a JobRun references it.
type JobPlanRevision struct {
	ID             string                `json:"id"`
	TenantID       string                `json:"tenant_id"`
	BaseRevisionID string                `json:"base_revision_id,omitempty"`
	GraphVersion   int                   `json:"graph_version"`
	PatchKey       string                `json:"patch_key,omitempty"`
	PatchReason    string                `json:"patch_reason,omitempty"`
	SOPID          string                `json:"sop_id"`
	SOPVersion     int                   `json:"sop_version"`
	SOPDigest      string                `json:"sop_digest"`
	SchemaVersion  string                `json:"schema_version"`
	Digest         string                `json:"digest"`
	Nodes          []JobPlanNode         `json:"nodes"`
	Edges          []JobPlanEdge         `json:"edges"`
	CustomerSteps  []JobPlanCustomerStep `json:"customer_steps"`
	Limits         RuntimeLimits         `json:"limits"`
	CompiledAt     time.Time             `json:"compiled_at"`
	CompiledBy     string                `json:"compiled_by"`
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
	if p.ID == "" || p.TenantID == "" || p.SOPID == "" || p.SOPVersion < 1 || p.SchemaVersion != JobPlanSchema || p.Digest == "" || p.GraphVersion < 1 {
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
	ID                  string    `json:"id"`
	TenantID            string    `json:"tenant_id"`
	ProjectID           string    `json:"project_id"`
	WorkTaskID          string    `json:"work_task_id"`
	BusinessType        string    `json:"business_type,omitempty"`
	InputSnapshotID     string    `json:"input_snapshot_id,omitempty"`
	BusinessOutputCount int       `json:"business_output_count,omitempty"`
	PlanRevisionID      string    `json:"plan_revision_id"`
	PlanDigest          string    `json:"plan_digest"`
	BindingDigest       string    `json:"binding_digest"`
	InputDigest         string    `json:"input_digest"`
	RuntimePolicyID     string    `json:"runtime_policy_id"`
	ContractMajor       int       `json:"contract_major"`
	ContractMinor       int       `json:"contract_minor"`
	RootJobRunID        string    `json:"root_job_run_id"`
	SourceJobRunID      string    `json:"source_job_run_id,omitempty"`
	CheckpointID        string    `json:"checkpoint_id,omitempty"`
	IdempotencyKey      string    `json:"idempotency_key,omitempty"`
	State               string    `json:"state"`
	Priority            int       `json:"priority"`
	Version             int       `json:"version"`
	ErrorCode           string    `json:"error_code,omitempty"`
	CreatedBy           string    `json:"created_by"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
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

// RuntimeAttempt is the authoritative execution-attempt model.
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
	FenceToken      string         `json:"fence_token,omitempty"`
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
		if strings.TrimSpace(attempt.LeaseOwner) == "" || strings.TrimSpace(attempt.FenceToken) == "" || attempt.LeaseExpiresAt == nil || !attempt.LeaseExpiresAt.After(attempt.UpdatedAt) || attempt.FinishedAt != nil {
			return Invalid("RUNTIME_ATTEMPT_LEASE_INVALID", "运行中的 RuntimeAttempt 缺少有效租约")
		}
	} else if attempt.LeaseOwner != "" || attempt.FenceToken != "" || attempt.LeaseExpiresAt != nil || attempt.FinishedAt == nil {
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
	case RuntimeAttemptPrepared, RuntimeAttemptRunning, RuntimeAttemptYielded, RuntimeAttemptSucceeded, RuntimeAttemptRetryableFailed, RuntimeAttemptFailed, RuntimeAttemptCancelled, RuntimeAttemptExpired:
		return true
	}
	return false
}

func (attempt RuntimeAttempt) Terminal() bool {
	switch attempt.State {
	case RuntimeAttemptYielded, RuntimeAttemptSucceeded, RuntimeAttemptRetryableFailed, RuntimeAttemptFailed, RuntimeAttemptCancelled, RuntimeAttemptExpired:
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
			RuntimeAttemptYielded: true, RuntimeAttemptSucceeded: true, RuntimeAttemptRetryableFailed: true, RuntimeAttemptFailed: true,
			RuntimeAttemptCancelled: true, RuntimeAttemptExpired: true,
		},
	}
	if !allowed[attempt.State][next] {
		return Conflict("RUNTIME_ATTEMPT_TRANSITION_INVALID", fmt.Sprintf("RuntimeAttempt 不能从 %s 转为 %s", attempt.State, next))
	}
	return nil
}

func (r JobRun) Validate() error {
	if r.ID == "" || r.TenantID == "" || r.ProjectID == "" || r.WorkTaskID == "" || r.PlanRevisionID == "" || r.PlanDigest == "" || r.BindingDigest == "" || r.InputDigest == "" || r.RuntimePolicyID == "" || r.ContractMajor < 1 || r.ContractMinor < 0 || r.RootJobRunID == "" || r.State == "" || r.Version < 1 {
		return Invalid("JOB_RUN_INVALID", "JobRun 缺少任务、计划或状态")
	}
	if !validSHA256Digest(r.PlanDigest) || !validSHA256Digest(r.BindingDigest) || !validSHA256Digest(r.InputDigest) {
		return Invalid("JOB_RUN_DIGEST_INVALID", "JobRun 的计划、执行绑定或输入摘要无效")
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
		JobRunAdmitted:     {JobRunRunning: true, JobRunWaitingHuman: true, JobRunPaused: true, JobRunCompleted: true, JobRunFailed: true, JobRunRejected: true, JobRunCancelled: true},
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
	FenceToken     string     `json:"fence_token,omitempty"`
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
	if n.State == NodeLeased || n.State == NodeRunning {
		if strings.TrimSpace(n.LeaseOwner) == "" || strings.TrimSpace(n.FenceToken) == "" || n.LeaseExpiresAt == nil || !n.LeaseExpiresAt.After(n.UpdatedAt) {
			return Invalid("NODE_RUN_LEASE_INVALID", "运行中的 NodeRun 缺少有效租约或围栏")
		}
	} else if n.LeaseOwner != "" || n.FenceToken != "" || n.LeaseExpiresAt != nil {
		return Invalid("NODE_RUN_LEASE_INVALID", "非运行中的 NodeRun 不能持有租约或围栏")
	}
	return nil
}

func validNodeState(state string) bool {
	switch state {
	case NodePending, NodeReady, NodeWaitingResource, NodeLeased, NodeRunning, NodeWaitingChildren, NodeWaitingExternal, NodeWaitingHuman, NodeSucceeded, NodeRetryableFailed, NodeFailed, NodeBlocked, NodeSkipped, NodeCancelled, NodeLeaseExpired:
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
		NodeRunning:         {NodeSucceeded: true, NodeRetryableFailed: true, NodeFailed: true, NodeWaitingChildren: true, NodeWaitingExternal: true, NodeWaitingHuman: true, NodeCancelled: true, NodeLeaseExpired: true},
		NodeWaitingChildren: {NodeReady: true, NodeFailed: true, NodeCancelled: true},
		NodeWaitingExternal: {NodeReady: true, NodeSucceeded: true, NodeFailed: true, NodeRetryableFailed: true, NodeCancelled: true},
		NodeWaitingHuman:    {NodeReady: true, NodeSucceeded: true, NodeRetryableFailed: true, NodeFailed: true, NodeCancelled: true},
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

type RuntimeExplorerView struct {
	TenantID      string    `json:"tenant_id"`
	JobRunID      string    `json:"job_run_id"`
	Job           JobRun    `json:"job"`
	Nodes         []NodeRun `json:"nodes"`
	LastEventSeq  int64     `json:"last_event_sequence"`
	ProjectedAt   time.Time `json:"projected_at"`
	SourceEventID string    `json:"source_event_id"`
}

type RuntimeProjectionStats struct {
	TenantID        string     `json:"tenant_id"`
	Pending         int        `json:"pending"`
	OldestPending   *time.Time `json:"oldest_pending,omitempty"`
	LastProjectedAt *time.Time `json:"last_projected_at,omitempty"`
}

type RuntimeProjectionRebuildRun struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	JobRunID        string     `json:"job_run_id"`
	Mode            string     `json:"mode"`
	Status          string     `json:"status"`
	EventCount      int        `json:"event_count"`
	LastSequence    int64      `json:"last_sequence"`
	ExternalCalls   int        `json:"external_calls"`
	IntegrityStatus string     `json:"integrity_status"`
	ErrorCode       string     `json:"error_code,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	Version         int        `json:"version"`
}

func (run RuntimeProjectionRebuildRun) Validate() error {
	if run.ID == "" || run.TenantID == "" || run.JobRunID == "" || run.StartedAt.IsZero() || run.Version < 1 {
		return Invalid("RUNTIME_PROJECTION_REBUILD_INVALID", "投影重建运行事实缺少身份或版本")
	}
	if run.Mode != "rebuild" && run.Mode != "dry_run" {
		return Invalid("RUNTIME_PROJECTION_REBUILD_MODE_INVALID", "投影重建模式无效")
	}
	switch run.Status {
	case "running":
		if run.FinishedAt != nil {
			return Invalid("RUNTIME_PROJECTION_REBUILD_RUNNING_INVALID", "运行中的投影重建不能包含结束时间")
		}
	case "completed", "failed":
		if run.FinishedAt == nil {
			return Invalid("RUNTIME_PROJECTION_REBUILD_FINISHED_INVALID", "已结束的投影重建必须记录结束时间")
		}
	default:
		return Invalid("RUNTIME_PROJECTION_REBUILD_STATUS_INVALID", "投影重建状态无效")
	}
	if run.ExternalCalls != 0 {
		return Invalid("RUNTIME_PROJECTION_EXTERNAL_CALLS", "投影重建不允许外部调用")
	}
	return nil
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

type ResourceRequest struct {
	ResourceKey string `json:"resource_key"`
	Quantity    int64  `json:"quantity"`
	Unit        string `json:"unit"`
}

type ResourceQuota struct {
	TenantID    string    `json:"tenant_id"`
	ResourceKey string    `json:"resource_key"`
	Capacity    int64     `json:"capacity"`
	Unit        string    `json:"unit"`
	Version     int       `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (quota ResourceQuota) Validate() error {
	if strings.TrimSpace(quota.TenantID) == "" || strings.TrimSpace(quota.ResourceKey) == "" || quota.Capacity < 0 || strings.TrimSpace(quota.Unit) == "" || quota.Version < 1 || quota.UpdatedAt.IsZero() {
		return Invalid("RESOURCE_QUOTA_INVALID", "资源配额缺少资源键、容量、单位或版本")
	}
	return nil
}

type ResourceReservation struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	JobRunID       string     `json:"job_run_id"`
	NodeRunID      string     `json:"node_run_id"`
	AttemptID      string     `json:"attempt_id"`
	ResourceKey    string     `json:"resource_key"`
	Quantity       int64      `json:"quantity"`
	Unit           string     `json:"unit"`
	State          string     `json:"state"`
	FenceToken     string     `json:"fence_token,omitempty"`
	IdempotencyKey string     `json:"idempotency_key"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	ReleasedAt     *time.Time `json:"released_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (reservation ResourceReservation) Validate() error {
	if reservation.ID == "" || reservation.TenantID == "" || reservation.JobRunID == "" || reservation.NodeRunID == "" || reservation.AttemptID == "" || strings.TrimSpace(reservation.ResourceKey) == "" || reservation.Quantity <= 0 || strings.TrimSpace(reservation.Unit) == "" || reservation.IdempotencyKey == "" || reservation.CreatedAt.IsZero() || reservation.UpdatedAt.IsZero() {
		return Invalid("RESOURCE_RESERVATION_INVALID", "资源预留缺少执行范围、资源键、数量或幂等键")
	}
	switch reservation.State {
	case ReservationHeld:
		if reservation.FenceToken == "" || reservation.ExpiresAt == nil || !reservation.ExpiresAt.After(reservation.UpdatedAt) || reservation.ReleasedAt != nil {
			return Invalid("RESOURCE_RESERVATION_LEASE_INVALID", "持有中的资源预留缺少有效围栏或有效期")
		}
	case ReservationConsumed, ReservationReleased, ReservationExpired:
		if reservation.FenceToken != "" || reservation.ExpiresAt != nil || reservation.ReleasedAt == nil {
			return Invalid("RESOURCE_RESERVATION_TERMINAL_INVALID", "终态资源预留必须释放围栏并记录结束时间")
		}
	default:
		return Invalid("RESOURCE_RESERVATION_STATE_INVALID", "资源预留状态无效")
	}
	return nil
}

type Checkpoint struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	JobRunID            string         `json:"job_run_id"`
	NodeKey             string         `json:"node_key"`
	PlanDigest          string         `json:"plan_digest"`
	StateRefs           []string       `json:"state_refs"`
	StateWatermarks     map[string]int `json:"state_watermarks"`
	OutputRefs          []string       `json:"output_refs"`
	CompletedNodes      []string       `json:"completed_nodes"`
	EventCursor         int64          `json:"event_cursor"`
	SideEffectWatermark string         `json:"side_effect_watermark,omitempty"`
	ParentCheckpointID  string         `json:"parent_checkpoint_id,omitempty"`
	Digest              string         `json:"digest"`
	CreatedAt           time.Time      `json:"created_at"`
}

type StateCollection struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	JobRunID        string    `json:"job_run_id"`
	CollectionKey   string    `json:"collection_key"`
	Scope           string    `json:"scope"`
	SchemaID        string    `json:"schema_id"`
	SchemaRevision  int       `json:"schema_revision"`
	Consistency     string    `json:"consistency"`
	WriterNodeKey   string    `json:"writer_node_key,omitempty"`
	MaxRecordBytes  int       `json:"max_record_bytes"`
	MaxRecords      int       `json:"max_records"`
	RetentionPolicy string    `json:"retention_policy"`
	ReadPolicy      []string  `json:"read_policy"`
	WritePolicy     []string  `json:"write_policy"`
	Revision        int       `json:"revision"`
	Watermark       int64     `json:"watermark"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (collection StateCollection) Validate() error {
	if collection.ID == "" || collection.TenantID == "" || collection.JobRunID == "" || collection.CollectionKey == "" || collection.SchemaID == "" || collection.SchemaRevision < 1 || collection.MaxRecordBytes <= 0 || collection.MaxRecords <= 0 || collection.Revision < 0 || collection.UpdatedAt.IsZero() {
		return Invalid("STATE_COLLECTION_INVALID", "状态集合缺少范围、Schema、大小或版本约束")
	}
	switch collection.Scope {
	case "job", "branch", "node_private":
	default:
		return Invalid("STATE_COLLECTION_SCOPE_INVALID", "状态集合范围无效")
	}
	switch collection.Consistency {
	case "single_writer", "append_only", "cas_map", "reducer_owned":
	default:
		return Invalid("STATE_COLLECTION_CONSISTENCY_INVALID", "状态集合一致性策略无效")
	}
	if (collection.Consistency == "single_writer" || collection.Consistency == "reducer_owned" || collection.Scope == "node_private") && strings.TrimSpace(collection.WriterNodeKey) == "" {
		return Invalid("STATE_COLLECTION_WRITER_REQUIRED", "单写入者、归并写入者或节点私有集合必须声明 WriterNodeKey")
	}
	return nil
}

type StateRecord struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	CollectionID   string         `json:"collection_id"`
	Key            string         `json:"key"`
	Value          map[string]any `json:"value,omitempty"`
	ArtifactRef    string         `json:"artifact_ref,omitempty"`
	SchemaRevision int            `json:"schema_revision"`
	Version        int            `json:"version"`
	Digest         string         `json:"digest"`
	CreatedBy      string         `json:"created_by"`
	UpdatedBy      string         `json:"updated_by"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func (record StateRecord) Validate() error {
	if record.ID == "" || record.TenantID == "" || record.CollectionID == "" || record.Key == "" || record.SchemaRevision < 1 || record.Version < 1 || record.Digest == "" || strings.TrimSpace(record.CreatedBy) == "" || strings.TrimSpace(record.UpdatedBy) == "" || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() || (record.Value == nil && record.ArtifactRef == "") {
		return Invalid("STATE_RECORD_INVALID", "状态记录缺少集合、键、值引用或版本摘要")
	}
	if record.Value != nil && record.ArtifactRef != "" {
		return Invalid("STATE_RECORD_VALUE_INVALID", "状态记录不能同时保存内嵌值和大对象引用")
	}
	return nil
}

type ToolCall struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	JobRunID        string         `json:"job_run_id"`
	NodeRunID       string         `json:"node_run_id"`
	AttemptID       string         `json:"attempt_id"`
	AgentInstanceID string         `json:"agent_instance_id"`
	ToolName        string         `json:"tool_name"`
	SchemaVersion   string         `json:"schema_version"`
	RequestDigest   string         `json:"request_digest"`
	SafeRequest     map[string]any `json:"safe_request"`
	ResultDigest    string         `json:"result_digest,omitempty"`
	State           string         `json:"state"`
	ErrorCode       string         `json:"error_code,omitempty"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
	Version         int            `json:"version"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (view ContextView) AllowsTool(toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	for _, allowed := range view.AllowedTools {
		if strings.TrimSpace(allowed) == toolName {
			return true
		}
	}
	return false
}

func (call ToolCall) Validate() error {
	if call.ID == "" || call.TenantID == "" || call.JobRunID == "" || call.NodeRunID == "" || call.AttemptID == "" || call.AgentInstanceID == "" || call.ToolName == "" || call.SchemaVersion == "" || call.RequestDigest == "" || call.State == "" || call.Version < 1 || call.CreatedAt.IsZero() || call.UpdatedAt.IsZero() {
		return Invalid("TOOL_CALL_INVALID", "ToolCall 缺少执行范围、工具契约或请求摘要")
	}
	switch call.State {
	case ToolCallProposed, ToolCallAuthorized, ToolCallRunning, ToolCallSucceeded, ToolCallFailed, ToolCallUnknown:
	default:
		return Invalid("TOOL_CALL_STATE_INVALID", "ToolCall 状态无效")
	}
	return nil
}

type ExternalEffect struct {
	ID                    string         `json:"id"`
	TenantID              string         `json:"tenant_id"`
	JobRunID              string         `json:"job_run_id"`
	NodeRunID             string         `json:"node_run_id"`
	AttemptID             string         `json:"attempt_id,omitempty"`
	ResourceReservationID string         `json:"resource_reservation_id,omitempty"`
	Kind                  string         `json:"kind"`
	IdempotencyKey        string         `json:"idempotency_key"`
	State                 string         `json:"state"`
	ExternalID            string         `json:"external_id,omitempty"`
	RequestDigest         string         `json:"request_digest"`
	ResponseDigest        string         `json:"response_digest,omitempty"`
	CostMinor             int64          `json:"cost_minor"`
	Currency              string         `json:"currency"`
	SafeSummary           map[string]any `json:"safe_summary"`
	ErrorCode             string         `json:"error_code,omitempty"`
	Version               int            `json:"version"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
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
