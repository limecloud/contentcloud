package runtime

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
)

type Service struct {
	repo           Repository
	now            func() time.Time
	compiler       Compiler
	harnesses      *agentadapter.HarnessRegistry
	rollout        RolloutPolicy
	rolloutTenants map[string]struct{}
	available      func(string)
}

const DefaultNodeLeaseDuration = 5 * time.Minute

func New(repo Repository, now func() time.Time) *Service {
	return NewWithHarnessRegistry(repo, now, nil)
}

func NewWithHarnessRegistry(repo Repository, now func() time.Time, harnesses *agentadapter.HarnessRegistry) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now, compiler: NewCompiler(DefaultRuntimeLimits()), harnesses: harnesses, rollout: DefaultRolloutPolicy()}
}

func (s *Service) Repository() Repository { return s.repo }

type RolloutPolicy struct {
	AdmissionEnabled    bool
	DynamicGraphEnabled bool
	TenantIDs           []string
}

func DefaultRolloutPolicy() RolloutPolicy {
	return RolloutPolicy{AdmissionEnabled: true, DynamicGraphEnabled: true}
}

func (s *Service) SetRolloutPolicy(policy RolloutPolicy) {
	if s == nil {
		return
	}
	s.rollout = policy
	s.rolloutTenants = map[string]struct{}{}
	for _, tenantID := range policy.TenantIDs {
		if tenantID = strings.TrimSpace(tenantID); tenantID != "" {
			s.rolloutTenants[tenantID] = struct{}{}
		}
	}
}

// SetAvailableNotifier installs a best-effort hint for remote workers. Runtime
// state remains authoritative in the repository; callers must still use the
// fenced Prepare protocol after receiving a notification.
func (s *Service) SetAvailableNotifier(notify func(tenantID string)) {
	if s != nil {
		s.available = notify
	}
}

func (s *Service) notifyAvailable(tenantID string) {
	if s.available != nil {
		s.available(tenantID)
	}
}

func (s *Service) rolloutTenantAllowed(tenantID string) bool {
	if len(s.rolloutTenants) == 0 {
		return true
	}
	_, ok := s.rolloutTenants[strings.TrimSpace(tenantID)]
	return ok
}

func (s *Service) requireAdmission(tenantID string) error {
	if !s.rollout.AdmissionEnabled || !s.rolloutTenantAllowed(tenantID) {
		return fault.Policy("RUNTIME_ADMISSION_DISABLED", "Runtime 新执行准入当前已关闭", "等待平台运营完成 Canary 或事故恢复后重试")
	}
	return nil
}

func (s *Service) requireDynamicGraph(tenantID string) error {
	if !s.rollout.DynamicGraphEnabled || !s.rolloutTenantAllowed(tenantID) {
		return fault.Policy("RUNTIME_DYNAMIC_GRAPH_DISABLED", "Runtime 动态执行图变更当前已关闭", "保留现有执行图并等待平台运营恢复动态能力")
	}
	return nil
}

func (s *Service) commands() (RuntimeCommandStore, error) {
	return s.repo, nil
}

type StartInput struct {
	TenantID            string
	ProjectID           string
	WorkTaskID          string
	BusinessType        string
	InputSnapshotID     string
	BusinessOutputCount int
	SOP                 catalogdomain.SOPVersion
	ExecutionBinding    *ExecutionBindingSnapshot
	// BindingDigest is the compatibility input for callers that have not yet
	// adopted a structured ExecutionBindingSnapshot.
	BindingDigest   string
	InputDigest     string
	RuntimePolicyID string
	ContractMajor   int
	ContractMinor   int
	Priority        int
	CreatedBy       string
	IdempotencyKey  string
	CorrelationID   string
}

const (
	DefaultRuntimePolicyID = "runtime.default/1"
	RuntimeContractMajor   = 1
	RuntimeContractMinor   = 0
)

type StartResult struct {
	Plan  JobPlanRevision `json:"plan"`
	Job   JobRun          `json:"job"`
	Nodes []NodeRun       `json:"nodes"`
}

func (s *Service) Start(ctx context.Context, input StartInput) (StartResult, error) {
	if s == nil || s.repo == nil {
		return StartResult{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(input.WorkTaskID) == "" {
		return StartResult{}, fault.Invalid("JOB_RUN_INPUT_INVALID", "创建执行实例缺少租户、项目或任务")
	}
	now := s.now().UTC()
	plan, err := s.compiler.CompileSOP(input.SOP, input.TenantID, input.CreatedBy, now)
	if err != nil {
		return StartResult{}, err
	}
	binding, err := resolveExecutionBinding(input, plan, now)
	if err != nil {
		return StartResult{}, err
	}
	jobID := idgen.New()
	businessType := strings.TrimSpace(input.BusinessType)
	if businessType == "" {
		businessType = "runtime.job"
	}
	job := JobRun{ID: jobID, TenantID: input.TenantID, ProjectID: input.ProjectID, WorkTaskID: input.WorkTaskID, BusinessType: businessType, InputSnapshotID: strings.TrimSpace(input.InputSnapshotID), BusinessOutputCount: input.BusinessOutputCount, PlanRevisionID: plan.ID, PlanDigest: plan.Digest, BindingDigest: binding.Digest, InputDigest: strings.TrimSpace(input.InputDigest), RuntimePolicyID: binding.RuntimePolicyID, ContractMajor: input.ContractMajor, ContractMinor: input.ContractMinor, RootJobRunID: jobID, IdempotencyKey: strings.TrimSpace(input.IdempotencyKey), State: JobRunCreated, Priority: input.Priority, Version: 1, CreatedBy: input.CreatedBy, CreatedAt: now, UpdatedAt: now}
	if err := job.Validate(); err != nil {
		return StartResult{}, err
	}
	if key := job.IdempotencyKey; key != "" {
		if existing, lookupErr := s.repo.JobRunByIdempotencyKey(ctx, input.TenantID, key); lookupErr == nil {
			return s.loadIdempotentStart(ctx, existing, job)
		} else if !fault.IsNotFound(lookupErr) {
			return StartResult{}, lookupErr
		}
	}
	if err := s.requireAdmission(input.TenantID); err != nil {
		return StartResult{}, err
	}
	if err := s.ensureExecutionBindingSnapshot(ctx, binding); err != nil {
		return StartResult{}, err
	}
	if err := s.repo.CreatePlan(ctx, plan); err != nil {
		// A plan is immutable and can be shared by retries. If another request
		// already created the same digest, use the existing revision.
		plans, listErr := s.repo.Plans(ctx, input.TenantID)
		if listErr != nil {
			return StartResult{}, err
		}
		for _, candidate := range plans {
			if candidate.Digest == plan.Digest {
				plan = candidate
				job.PlanRevisionID = candidate.ID
				err = nil
				break
			}
		}
		if err != nil {
			return StartResult{}, err
		}
	}
	nodes := make([]NodeRun, 0, len(plan.Nodes))
	for _, node := range plan.Nodes {
		nodes = append(nodes, NodeRun{ID: idgen.New(), TenantID: input.TenantID, JobRunID: job.ID, NodeKey: node.Key, State: NodePending, OutputRefs: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now})
	}
	event := JobEvent{ID: idgen.New(), TenantID: input.TenantID, JobRunID: job.ID, Sequence: 1, Type: "job.created", ActorType: "user", ActorID: input.CreatedBy, CorrelationID: input.CorrelationID, IdempotencyKey: input.IdempotencyKey, Payload: map[string]any{"plan_digest": plan.Digest, "binding_digest": job.BindingDigest, "input_digest": job.InputDigest, "runtime_policy_id": job.RuntimePolicyID, "contract_major": job.ContractMajor, "contract_minor": job.ContractMinor, "node_count": len(nodes)}, OccurredAt: now}
	if err := s.repo.CreateJobBundle(ctx, job, nodes, event); err != nil {
		if job.IdempotencyKey != "" {
			if existing, lookupErr := s.repo.JobRunByIdempotencyKey(ctx, input.TenantID, job.IdempotencyKey); lookupErr == nil {
				return s.loadIdempotentStart(ctx, existing, job)
			}
		}
		return StartResult{}, err
	}
	if _, err := s.refresh(ctx, job); err != nil {
		return StartResult{}, err
	}
	job, err = s.repo.JobRun(ctx, input.TenantID, job.ID)
	if err != nil {
		return StartResult{}, err
	}
	nodes, err = s.repo.NodeRuns(ctx, input.TenantID, job.ID)
	if err != nil {
		return StartResult{}, err
	}
	return StartResult{Plan: plan, Job: job, Nodes: nodes}, nil
}

func resolveExecutionBinding(input StartInput, plan JobPlanRevision, now time.Time) (ExecutionBindingSnapshot, error) {
	policyID := strings.TrimSpace(input.RuntimePolicyID)
	if input.ExecutionBinding == nil {
		binding := ExecutionBindingSnapshot{
			TenantID: input.TenantID, Digest: strings.TrimSpace(input.BindingDigest),
			SchemaVersion: ExecutionBindingSnapshotSchema,
			ProfileID:     defaultExecutionBindingValue(policyID, "runtime.legacy"), ProfileVersion: "legacy",
			RuntimePolicyID: defaultExecutionBindingValue(policyID, DefaultRuntimePolicyID),
			HarnessKinds:    []string{}, AllowedTools: []string{ToolChildList, ToolEffectStatus, ToolStateGet, ToolStateQuery},
			SandboxProfile: "legacy", IsolationProfile: "legacy", EgressPolicy: "legacy", DataClassification: "internal",
			MaxTokens: 8192, MaxDurationSeconds: 3600, MaxCostMinor: plan.Limits.MaxCostMinor,
			MaxDynamicDescendants: plan.Limits.MaxDynamicDescendants, FallbackPolicy: "none", Legacy: true, CreatedAt: now,
		}
		if err := binding.Validate(); err != nil {
			return ExecutionBindingSnapshot{}, err
		}
		return binding, nil
	}
	binding := *input.ExecutionBinding
	if tenantID := strings.TrimSpace(binding.TenantID); tenantID != "" && tenantID != strings.TrimSpace(input.TenantID) {
		return ExecutionBindingSnapshot{}, fault.Invalid("EXECUTION_BINDING_TENANT_MISMATCH", "ExecutionBindingSnapshot 不属于当前租户")
	}
	binding.TenantID = strings.TrimSpace(input.TenantID)
	if binding.SchemaVersion == "" {
		binding.SchemaVersion = ExecutionBindingSnapshotSchema
	}
	if binding.SchemaVersion != ExecutionBindingSnapshotSchema {
		return ExecutionBindingSnapshot{}, fault.Invalid("EXECUTION_BINDING_SCHEMA_UNSUPPORTED", "ExecutionBindingSnapshot Schema 版本不受支持")
	}
	if policyID != "" && strings.TrimSpace(binding.RuntimePolicyID) != policyID {
		return ExecutionBindingSnapshot{}, fault.Invalid("EXECUTION_BINDING_POLICY_MISMATCH", "ExecutionBindingSnapshot 与 RuntimePolicy 不一致")
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	binding.CreatedAt = binding.CreatedAt.UTC()
	binding.Legacy = false
	binding.NormalizeCollections()
	digest, err := binding.ContentDigest()
	if err != nil {
		return ExecutionBindingSnapshot{}, err
	}
	if supplied := strings.TrimSpace(input.BindingDigest); supplied != "" && supplied != digest {
		return ExecutionBindingSnapshot{}, fault.Conflict("EXECUTION_BINDING_DIGEST_MISMATCH", "调用方摘要与结构化 ExecutionBindingSnapshot 不一致")
	}
	binding.Digest = digest
	if err := binding.Validate(); err != nil {
		return ExecutionBindingSnapshot{}, err
	}
	return binding, nil
}

func defaultExecutionBindingValue(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func (s *Service) ensureExecutionBindingSnapshot(ctx context.Context, binding ExecutionBindingSnapshot) error {
	if err := s.repo.CreateExecutionBindingSnapshot(ctx, binding); err == nil {
		return nil
	} else {
		existing, lookupErr := s.repo.ExecutionBindingSnapshot(ctx, binding.TenantID, binding.Digest)
		if lookupErr != nil {
			return err
		}
		existingDigest, existingErr := existing.ContentDigest()
		requestedDigest, requestedErr := binding.ContentDigest()
		if existingErr != nil || requestedErr != nil || existingDigest != requestedDigest {
			return fault.Conflict("EXECUTION_BINDING_SNAPSHOT_CONFLICT", "相同摘要已绑定不同的执行策略快照")
		}
		return nil
	}
}

func (s *Service) loadIdempotentStart(ctx context.Context, existing, requested JobRun) (StartResult, error) {
	if !sameStartAdmission(existing, requested) {
		return StartResult{}, fault.Conflict("JOB_RUN_IDEMPOTENCY_MISMATCH", "幂等键已用于不同的执行准入快照")
	}
	plan, err := s.repo.Plan(ctx, existing.TenantID, existing.PlanRevisionID)
	if err != nil {
		return StartResult{}, err
	}
	nodes, err := s.repo.NodeRuns(ctx, existing.TenantID, existing.ID)
	if err != nil {
		return StartResult{}, err
	}
	return StartResult{Plan: plan, Job: existing, Nodes: nodes}, nil
}

func sameStartAdmission(existing, requested JobRun) bool {
	return existing.ProjectID == requested.ProjectID &&
		existing.WorkTaskID == requested.WorkTaskID &&
		existing.PlanDigest == requested.PlanDigest &&
		existing.BindingDigest == requested.BindingDigest &&
		existing.InputDigest == requested.InputDigest &&
		existing.RuntimePolicyID == requested.RuntimePolicyID &&
		existing.ContractMajor == requested.ContractMajor &&
		existing.ContractMinor == requested.ContractMinor &&
		existing.BusinessType == requested.BusinessType &&
		existing.InputSnapshotID == requested.InputSnapshotID &&
		existing.BusinessOutputCount == requested.BusinessOutputCount &&
		existing.Priority == requested.Priority &&
		existing.SourceJobRunID == "" && existing.CheckpointID == ""
}

func (s *Service) Job(ctx context.Context, tenantID, id string) (JobRun, error) {
	return s.repo.JobRun(ctx, tenantID, id)
}
func (s *Service) JobByIdempotencyKey(ctx context.Context, tenantID, key string) (JobRun, error) {
	return s.repo.JobRunByIdempotencyKey(ctx, tenantID, key)
}
func (s *Service) Jobs(ctx context.Context, tenantID, taskID string) ([]JobRun, error) {
	return s.repo.JobRuns(ctx, tenantID, taskID)
}
func (s *Service) JobsPage(ctx context.Context, tenantID, projectID, state string, after, limit int) ([]JobRun, bool, error) {
	return s.repo.JobRunsPage(ctx, tenantID, projectID, state, after, limit)
}
func (s *Service) Plan(ctx context.Context, tenantID, id string) (JobPlanRevision, error) {
	return s.repo.Plan(ctx, tenantID, id)
}
func (s *Service) Nodes(ctx context.Context, tenantID, jobID string) ([]NodeRun, error) {
	return s.repo.NodeRuns(ctx, tenantID, jobID)
}
func (s *Service) NodesPage(ctx context.Context, tenantID, jobID string, after, limit int) ([]NodeRun, bool, error) {
	return s.repo.NodeRunsPage(ctx, tenantID, jobID, after, limit)
}

// ClaimNode is the scheduler/worker boundary for the new Runtime graph. The
// repository atomically claims the node and records its event/outbox message.
func (s *Service) ClaimNode(ctx context.Context, tenantID, jobID, owner string, leaseFor time.Duration) (NodeRun, error) {
	if s == nil || s.repo == nil {
		return NodeRun{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if leaseFor <= 0 {
		leaseFor = DefaultNodeLeaseDuration
	}
	commands, err := s.commands()
	if err != nil {
		return NodeRun{}, err
	}
	now := s.now().UTC()
	return commands.ClaimReadyNodeCommand(ctx, tenantID, jobID, owner, now, leaseFor, JobEvent{ID: idgen.New(), TenantID: tenantID, Type: "node.leased", ActorType: "scheduler", ActorID: strings.TrimSpace(owner), Payload: map[string]any{}, OccurredAt: now})
}

// HeartbeatNode renews a lease and promotes the first heartbeat from leased to
// running. Version and owner checks make late workers fail closed.
func (s *Service) HeartbeatNode(ctx context.Context, tenantID, nodeID, owner string, expectedVersion int, leaseFor time.Duration) (NodeRun, error) {
	if s == nil || s.repo == nil {
		return NodeRun{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if leaseFor <= 0 {
		leaseFor = DefaultNodeLeaseDuration
	}
	commands, err := s.commands()
	if err != nil {
		return NodeRun{}, err
	}
	now := s.now().UTC()
	return commands.HeartbeatNodeCommand(ctx, tenantID, nodeID, owner, expectedVersion, now, leaseFor, JobEvent{ID: idgen.New(), TenantID: tenantID, Type: "node.heartbeat", ActorType: "worker", ActorID: strings.TrimSpace(owner), Payload: map[string]any{}, OccurredAt: now})
}

// ExpireNodeLeases is safe to call from a periodic scheduler tick. Expired
// nodes are first marked by the repository and then re-evaluated by Refresh so
// dependency readiness and the JobRun projection advance together.
func (s *Service) ExpireNodeLeases(ctx context.Context, tenantID string, now time.Time) error {
	if s == nil || s.repo == nil {
		return fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if err := s.repo.ExpireNodeLeases(ctx, tenantID, now.UTC()); err != nil {
		return err
	}
	jobs, err := s.repo.JobRuns(ctx, tenantID, "")
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if _, err := s.refresh(ctx, job); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Events(ctx context.Context, tenantID, jobID string, after int64) ([]JobEvent, error) {
	return s.repo.JobEvents(ctx, tenantID, jobID, after)
}
func (s *Service) EventsPage(ctx context.Context, tenantID, jobID string, after int64, limit int) ([]JobEvent, error) {
	return s.repo.JobEventsPage(ctx, tenantID, jobID, after, limit)
}
func (s *Service) Effects(ctx context.Context, tenantID, jobID string) ([]ExternalEffect, error) {
	return s.repo.Effects(ctx, tenantID, jobID)
}
func (s *Service) EffectsPage(ctx context.Context, tenantID, jobID string, after, limit int) ([]ExternalEffect, bool, error) {
	return s.repo.EffectsPage(ctx, tenantID, jobID, after, limit)
}
func (s *Service) Checkpoints(ctx context.Context, tenantID, jobID string) ([]Checkpoint, error) {
	return s.repo.Checkpoints(ctx, tenantID, jobID)
}
func (s *Service) CheckpointsPage(ctx context.Context, tenantID, jobID string, after, limit int) ([]Checkpoint, bool, error) {
	return s.repo.CheckpointsPage(ctx, tenantID, jobID, after, limit)
}
func (s *Service) CheckpointByID(ctx context.Context, tenantID, checkpointID string) (Checkpoint, error) {
	return s.repo.Checkpoint(ctx, tenantID, checkpointID)
}
func (s *Service) Effect(ctx context.Context, tenantID, effectID string) (ExternalEffect, error) {
	return s.repo.Effect(ctx, tenantID, effectID)
}

func (s *Service) CreateStateCollection(ctx context.Context, collection StateCollection) error {
	if s == nil || s.repo == nil {
		return fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	collection.TenantID = strings.TrimSpace(collection.TenantID)
	collection.JobRunID = strings.TrimSpace(collection.JobRunID)
	collection.CollectionKey = strings.TrimSpace(collection.CollectionKey)
	collection.WriterNodeKey = strings.TrimSpace(collection.WriterNodeKey)
	if collection.CreatedAt.IsZero() {
		collection.CreatedAt = s.now().UTC()
	}
	if collection.UpdatedAt.IsZero() {
		collection.UpdatedAt = collection.CreatedAt
	}
	if collection.ReadPolicy == nil {
		collection.ReadPolicy = []string{}
	}
	if collection.WritePolicy == nil {
		collection.WritePolicy = []string{}
	}
	if err := collection.Validate(); err != nil {
		return err
	}
	job, err := s.repo.JobRun(ctx, collection.TenantID, collection.JobRunID)
	if err != nil {
		return err
	}
	if job.TenantID != collection.TenantID || job.ID != collection.JobRunID {
		return fault.Invalid("STATE_COLLECTION_SCOPE_INVALID", "状态集合必须属于当前 JobRun")
	}
	schema, schemaErr := s.repo.RuntimeSchema(ctx, collection.TenantID, collection.SchemaID, collection.SchemaRevision)
	if schemaErr != nil {
		if fault.IsNotFound(schemaErr) {
			return fault.Policy("STATE_SCHEMA_NOT_PUBLISHED", "状态集合引用的 Runtime Schema 尚未发布", "先通过 Runtime Schema Registry 发布固定版本")
		}
		return schemaErr
	}
	if schema.Status != "published" {
		return fault.Policy("STATE_SCHEMA_NOT_PUBLISHED", "状态集合只能引用 published Runtime Schema", "发布 Schema 后再创建状态集合")
	}
	if collection.Scope == "node_private" || collection.WriterNodeKey != "" {
		if collection.WriterNodeKey == "" {
			return fault.Invalid("STATE_COLLECTION_WRITER_REQUIRED", "节点私有或单写入者集合必须声明 WriterNodeKey")
		}
		nodes, err := s.repo.NodeRuns(ctx, collection.TenantID, collection.JobRunID)
		if err != nil {
			return err
		}
		found := false
		for _, node := range nodes {
			if node.NodeKey == collection.WriterNodeKey {
				found = true
				break
			}
		}
		if !found {
			return fault.Invalid("STATE_COLLECTION_WRITER_SCOPE_INVALID", "WriterNodeKey 不属于当前 JobRun")
		}
	}
	return s.repo.CreateStateCollection(ctx, collection)
}

func (s *Service) StateCollections(ctx context.Context, tenantID, jobID string) ([]StateCollection, error) {
	return s.repo.StateCollections(ctx, tenantID, jobID)
}

func (s *Service) StateRecords(ctx context.Context, tenantID, collectionID string) ([]StateRecord, error) {
	return s.repo.StateRecords(ctx, tenantID, collectionID)
}

func (s *Service) StateRecordCAS(ctx context.Context, record StateRecord, expectedVersion int) (StateRecord, error) {
	return s.stateRecordCAS(ctx, record, expectedVersion, "", "")
}

func (s *Service) StateRecordCASForAttempt(ctx context.Context, record StateRecord, expectedVersion int, attemptID, fenceToken string) (StateRecord, error) {
	if strings.TrimSpace(attemptID) == "" || strings.TrimSpace(fenceToken) == "" {
		return record, fault.Invalid("MCP_GATEWAY_FENCE_REQUIRED", "Attempt 状态写入需要 attempt_id 和 fence_token")
	}
	return s.stateRecordCAS(ctx, record, expectedVersion, strings.TrimSpace(attemptID), strings.TrimSpace(fenceToken))
}

func (s *Service) stateRecordCAS(ctx context.Context, record StateRecord, expectedVersion int, attemptID, fenceToken string) (StateRecord, error) {
	if s == nil || s.repo == nil {
		return record, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	collection, err := s.repo.StateCollection(ctx, strings.TrimSpace(record.TenantID), strings.TrimSpace(record.CollectionID))
	if err != nil {
		return record, err
	}
	if collection.TenantID != record.TenantID {
		return record, fault.Invalid("STATE_RECORD_SCOPE_INVALID", "状态记录与集合不属于同一租户")
	}
	if record.SchemaRevision != collection.SchemaRevision {
		return record, fault.Conflict("STATE_SCHEMA_REVISION_CONFLICT", "状态记录 SchemaRevision 与集合已发布版本不一致")
	}
	if expectedVersion < 0 {
		return record, fault.Invalid("STATE_RECORD_VERSION_INVALID", "状态记录期望版本不能为负数")
	}
	if collection.Consistency == "append_only" && expectedVersion != 0 {
		return record, fault.Policy("STATE_APPEND_ONLY_UPDATE_FORBIDDEN", "append_only 集合不允许覆盖既有记录", "使用新的记录键追加一条记录")
	}
	if strings.TrimSpace(record.UpdatedBy) == "" {
		record.UpdatedBy = strings.TrimSpace(record.CreatedBy)
	}
	if strings.TrimSpace(record.CreatedBy) == "" {
		record.CreatedBy = record.UpdatedBy
	}
	if !stateWriteAllowed(collection, record.UpdatedBy) {
		return record, fault.Policy("STATE_WRITE_FORBIDDEN", "当前执行者没有写入该状态集合的权限", "使用集合声明的写入者或写入策略")
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = s.now().UTC()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = record.UpdatedAt
	}
	now := s.now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = now
	}
	actor := strings.TrimSpace(record.UpdatedBy)
	if actor == "" {
		actor = "runtime"
	}
	eventType := "state.record.updated"
	if expectedVersion == 0 {
		eventType = "state.record.created"
	}
	event := JobEvent{ID: idgen.New(), TenantID: record.TenantID, JobRunID: collection.JobRunID, Type: eventType, ActorType: "runtime", ActorID: actor, IdempotencyKey: "state-record:" + record.ID + ":" + strconv.Itoa(expectedVersion), Payload: map[string]any{"collection_id": record.CollectionID, "key": record.Key, "version": expectedVersion + 1}, OccurredAt: record.UpdatedAt}
	if attemptID != "" {
		return s.repo.ApplyFencedStateRecordCommand(ctx, record, expectedVersion, attemptID, fenceToken, now, event)
	}
	return s.repo.ApplyStateRecordCommand(ctx, record, expectedVersion, event)
}

func (s *Service) CreateToolCall(ctx context.Context, call ToolCall) error {
	return s.createToolCall(ctx, call, "")
}

func (s *Service) CreateFencedToolCall(ctx context.Context, call ToolCall, fenceToken string) error {
	if strings.TrimSpace(fenceToken) == "" {
		return fault.Invalid("MCP_GATEWAY_FENCE_REQUIRED", "ToolCall 需要 Attempt fence_token")
	}
	return s.createToolCall(ctx, call, strings.TrimSpace(fenceToken))
}

func (s *Service) createToolCall(ctx context.Context, call ToolCall, fenceToken string) error {
	if s == nil || s.repo == nil {
		return fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	call.TenantID = strings.TrimSpace(call.TenantID)
	call.JobRunID = strings.TrimSpace(call.JobRunID)
	call.NodeRunID = strings.TrimSpace(call.NodeRunID)
	call.AttemptID = strings.TrimSpace(call.AttemptID)
	call.AgentInstanceID = strings.TrimSpace(call.AgentInstanceID)
	call.ToolName = strings.TrimSpace(call.ToolName)
	now := s.now().UTC()
	if call.CreatedAt.IsZero() {
		call.CreatedAt = now
	}
	if call.UpdatedAt.IsZero() {
		call.UpdatedAt = call.CreatedAt
	}
	if call.State == "" {
		call.State = ToolCallProposed
	}
	if call.Version < 1 {
		call.Version = 1
	}
	if err := authorizeToolCall(ctx, s.repo, call, now); err != nil {
		return err
	}
	event := JobEvent{ID: idgen.New(), TenantID: call.TenantID, JobRunID: call.JobRunID, NodeKey: "", Type: "tool_call." + call.State, ActorType: "runtime", ActorID: call.AgentInstanceID, IdempotencyKey: "tool-call:" + call.ID + ":created", Payload: map[string]any{"tool_call_id": call.ID, "tool_name": call.ToolName}, OccurredAt: call.CreatedAt}
	if fenceToken != "" {
		_, err := s.repo.RegisterFencedToolCallCommand(ctx, call, fenceToken, now, event)
		return err
	}
	_, err := s.repo.RegisterToolCallCommand(ctx, call, event)
	return err
}

func (s *Service) ToolCalls(ctx context.Context, tenantID, attemptID string) ([]ToolCall, error) {
	return s.repo.ToolCalls(ctx, tenantID, attemptID)
}

func (s *Service) TransitionToolCall(ctx context.Context, next ToolCall, expectedVersion int) (ToolCall, error) {
	return s.transitionToolCall(ctx, next, expectedVersion, "")
}

func (s *Service) TransitionFencedToolCall(ctx context.Context, next ToolCall, expectedVersion int, fenceToken string) (ToolCall, error) {
	if strings.TrimSpace(fenceToken) == "" {
		return next, fault.Invalid("MCP_GATEWAY_FENCE_REQUIRED", "ToolCall 状态变更需要 Attempt fence_token")
	}
	return s.transitionToolCall(ctx, next, expectedVersion, strings.TrimSpace(fenceToken))
}

func (s *Service) transitionToolCall(ctx context.Context, next ToolCall, expectedVersion int, fenceToken string) (ToolCall, error) {
	if s == nil || s.repo == nil {
		return next, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	current, err := s.repo.ToolCall(ctx, strings.TrimSpace(next.TenantID), strings.TrimSpace(next.ID))
	if err != nil {
		return next, err
	}
	if expectedVersion != current.Version || next.Version != expectedVersion+1 {
		return current, fault.Conflict("TOOL_CALL_VERSION_CONFLICT", "ToolCall 已被更新")
	}
	if next.TenantID != current.TenantID || next.JobRunID != current.JobRunID || next.NodeRunID != current.NodeRunID || next.AttemptID != current.AttemptID || next.AgentInstanceID != current.AgentInstanceID || next.ToolName != current.ToolName || next.SchemaVersion != current.SchemaVersion || next.RequestDigest != current.RequestDigest {
		return current, fault.Invalid("TOOL_CALL_SCOPE_IMMUTABLE", "ToolCall 的执行范围和请求摘要不能被修改")
	}
	if err := authorizeToolCall(ctx, s.repo, current, s.now().UTC()); err != nil {
		return current, err
	}
	event := JobEvent{ID: idgen.New(), TenantID: next.TenantID, JobRunID: next.JobRunID, NodeKey: "", Type: "tool_call." + next.State, ActorType: "runtime", ActorID: next.AgentInstanceID, IdempotencyKey: "tool-call:" + next.ID + ":" + strconv.Itoa(next.Version), Payload: map[string]any{"tool_call_id": next.ID, "state": next.State}, OccurredAt: next.UpdatedAt}
	if fenceToken != "" {
		return s.repo.ApplyFencedToolCallTransitionCommand(ctx, next, expectedVersion, fenceToken, s.now().UTC(), event)
	}
	return s.repo.ApplyToolCallTransitionCommand(ctx, next, expectedVersion, event)
}

func stateWriteAllowed(collection StateCollection, actor string) bool {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return false
	}
	writerMatch := collection.WriterNodeKey != "" && (actor == collection.WriterNodeKey || actor == "node:"+collection.WriterNodeKey || actor == "node/"+collection.WriterNodeKey)
	if collection.Scope == "node_private" || collection.Consistency == "single_writer" || collection.Consistency == "reducer_owned" {
		if !writerMatch {
			return false
		}
		if len(collection.WritePolicy) == 0 {
			return true
		}
	}
	for _, allowed := range collection.WritePolicy {
		if strings.TrimSpace(allowed) == actor {
			return true
		}
	}
	return false
}

func authorizeToolCall(ctx context.Context, repo Repository, call ToolCall, now time.Time) error {
	job, err := repo.JobRun(ctx, call.TenantID, call.JobRunID)
	if err != nil {
		return err
	}
	node, err := repo.NodeRun(ctx, call.TenantID, call.NodeRunID)
	if err != nil {
		return err
	}
	attempt, err := repo.RuntimeAttempt(ctx, call.TenantID, call.AttemptID)
	if err != nil {
		return err
	}
	agent, err := repo.AgentInstance(ctx, call.TenantID, call.AgentInstanceID)
	if err != nil {
		return err
	}
	view, err := repo.ContextView(ctx, call.TenantID, attempt.ContextViewID)
	if err != nil {
		return err
	}
	if job.ID != call.JobRunID || node.JobRunID != call.JobRunID || attempt.JobRunID != call.JobRunID || attempt.NodeRunID != call.NodeRunID || attempt.AgentInstanceID != call.AgentInstanceID || agent.JobRunID != call.JobRunID || agent.NodeRunID != call.NodeRunID || agent.ContextViewID != view.ID || view.JobRunID != call.JobRunID || view.NodeRunID != call.NodeRunID || view.AttemptID != call.AttemptID {
		return fault.Invalid("TOOL_CALL_SCOPE_INVALID", "ToolCall 必须绑定同一 JobRun、NodeRun、Attempt、Agent 和 ContextView")
	}
	if attempt.State != RuntimeAttemptPrepared && attempt.State != RuntimeAttemptRunning {
		return fault.Conflict("TOOL_CALL_ATTEMPT_NOT_ACTIVE", "只有准备中或运行中的 Attempt 可以创建或推进 ToolCall")
	}
	if agent.State != AgentRunnable && agent.State != AgentActive {
		return fault.Conflict("TOOL_CALL_AGENT_NOT_ACTIVE", "只有可运行或活动中的 AgentInstance 可以创建或推进 ToolCall")
	}
	if !view.ExpiresAt.After(now) {
		return fault.Policy("TOOL_CALL_CONTEXT_EXPIRED", "ToolCall 使用的 ContextView 已过期", "重新创建执行尝试和 ContextView")
	}
	if !view.AllowsTool(call.ToolName) {
		return fault.Policy("TOOL_CALL_NOT_ALLOWED", "当前 ContextView 未授权该工具", "仅调用 AllowedTools 中的工具")
	}
	return nil
}

func (s *Service) Refresh(ctx context.Context, tenantID, jobID string) (JobRun, error) {
	job, err := s.repo.JobRun(ctx, tenantID, jobID)
	if err != nil {
		return JobRun{}, err
	}
	return s.refresh(ctx, job)
}

func (s *Service) Resume(ctx context.Context, tenantID, jobID, actorType, actorID string) (JobRun, error) {
	job, err := s.repo.JobRun(ctx, tenantID, jobID)
	if err != nil {
		return JobRun{}, err
	}
	if job.State != JobRunPaused {
		return JobRun{}, fault.Conflict("JOB_RUN_NOT_RESUMABLE", "只有已暂停的执行实例可以恢复")
	}
	if err := job.Transition(JobRunRunning); err != nil {
		return JobRun{}, err
	}
	job.State = JobRunRunning
	job.Version++
	job.UpdatedAt = s.now().UTC()
	commands, err := s.commands()
	if err != nil {
		return JobRun{}, err
	}
	if _, err := commands.ApplyJobTransition(ctx, job, job.Version-1, JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: jobID, Type: "job.resumed", ActorType: actorType, ActorID: actorID, Payload: map[string]any{}, OccurredAt: s.now().UTC()}); err != nil {
		return JobRun{}, err
	}
	return s.refresh(ctx, job)
}

// Pause stops scheduling new nodes while preserving the current execution
// history and any active leases. Active attempts may finish and will not
// reopen the paused JobRun until an operator explicitly resumes it.
func (s *Service) Pause(ctx context.Context, tenantID, jobID, actorType, actorID string) (JobRun, error) {
	job, err := s.repo.JobRun(ctx, tenantID, jobID)
	if err != nil {
		return JobRun{}, err
	}
	if err := job.Transition(JobRunPaused); err != nil {
		return JobRun{}, err
	}
	job.State = JobRunPaused
	job.Version++
	job.UpdatedAt = s.now().UTC()
	commands, err := s.commands()
	if err != nil {
		return JobRun{}, err
	}
	if _, err := commands.ApplyJobTransition(ctx, job, job.Version-1, JobEvent{
		ID: idgen.New(), TenantID: tenantID, JobRunID: jobID, Type: "job.paused",
		ActorType: actorType, ActorID: actorID, Payload: map[string]any{}, OccurredAt: job.UpdatedAt,
	}); err != nil {
		return JobRun{}, err
	}
	return job, nil
}

func (s *Service) refresh(ctx context.Context, job JobRun) (JobRun, error) {
	if job.State == JobRunPaused || job.State == JobRunCompleted || job.State == JobRunFailed || job.State == JobRunCancelled || job.State == JobRunRejected {
		return job, nil
	}
	plan, err := s.repo.Plan(ctx, job.TenantID, job.PlanRevisionID)
	if err != nil {
		return job, err
	}
	if err := s.refreshFanoutJoins(ctx, job); err != nil {
		return job, err
	}
	fanoutSets, err := s.repo.FanoutSets(ctx, job.TenantID, job.ID)
	if err != nil {
		return job, err
	}
	fanoutsByJoin := map[string][]FanoutSet{}
	for _, fanout := range fanoutSets {
		fanoutsByJoin[fanout.JoinNodeKey] = append(fanoutsByJoin[fanout.JoinNodeKey], fanout)
	}
	nodes, err := s.repo.NodeRuns(ctx, job.TenantID, job.ID)
	if err != nil {
		return job, err
	}
	byKey := map[string]NodeRun{}
	for _, node := range nodes {
		byKey[node.NodeKey] = node
	}
	changed := false
	for _, spec := range plan.Nodes {
		node := byKey[spec.Key]
		if node.State != NodePending && node.State != NodeRetryableFailed && node.State != NodeLeaseExpired && node.State != NodeWaitingResource {
			continue
		}
		allSucceeded, blocked := true, false
		fanoutWaiting := false
		for _, fanout := range fanoutsByJoin[spec.Key] {
			if fanout.Status == FanoutFailed {
				blocked = true
				continue
			}
			if fanout.Status != FanoutSucceeded {
				fanoutWaiting = true
			}
		}
		for _, dep := range spec.DependsOn {
			dependency, ok := byKey[dep]
			if !ok || (dependency.State != NodeSucceeded && dependency.State != NodeSkipped) {
				allSucceeded = false
			}
			if ok && (dependency.State == NodeFailed || dependency.State == NodeBlocked || dependency.State == NodeCancelled) {
				blocked = true
			}
		}
		next := node.State
		if blocked {
			next = NodeBlocked
		} else if allSucceeded && !fanoutWaiting {
			next = NodeReady
		}
		if next != node.State {
			if err := node.Transition(next); err != nil {
				return job, err
			}
			node.State = next
			node.Version++
			node.UpdatedAt = s.now().UTC()
			commands, err := s.commands()
			if err != nil {
				return job, err
			}
			if _, err := commands.ApplyNodeTransition(ctx, node, node.Version-1, JobEvent{ID: idgen.New(), TenantID: job.TenantID, JobRunID: job.ID, NodeKey: node.NodeKey, Type: "node." + next, ActorType: "runtime", Payload: map[string]any{}, OccurredAt: node.UpdatedAt}); err != nil {
				return job, err
			}
			byKey[node.NodeKey] = node
			changed = true
		}
	}
	if changed {
		nodes, _ = s.repo.NodeRuns(ctx, job.TenantID, job.ID)
	}
	ready, active, waiting, failed, terminal := 0, 0, 0, 0, 0
	for _, node := range nodes {
		switch node.State {
		case NodeReady:
			ready++
		case NodeLeased, NodeRunning, NodeWaitingChildren, NodeWaitingExternal:
			active++
		case NodeWaitingHuman:
			waiting++
		case NodeFailed, NodeBlocked:
			failed++
		case NodeSucceeded, NodeSkipped, NodeCancelled:
			terminal++
		}
	}
	next := job.State
	switch {
	case job.State == JobRunCreated:
		next = JobRunAdmitted
	case failed > 0:
		next = JobRunFailed
	case terminal == len(nodes) && len(nodes) > 0:
		next = JobRunCompleted
	case waiting > 0:
		next = JobRunWaitingHuman
	case active > 0 || ready > 0:
		next = JobRunRunning
	}
	if next != job.State {
		if err := job.Transition(next); err != nil {
			return job, err
		}
		job.State = next
		job.Version++
		job.UpdatedAt = s.now().UTC()
		commands, err := s.commands()
		if err != nil {
			return job, err
		}
		if _, err := commands.ApplyJobTransition(ctx, job, job.Version-1, JobEvent{ID: idgen.New(), TenantID: job.TenantID, JobRunID: job.ID, Type: "job." + next, ActorType: "runtime", Payload: map[string]any{"ready": ready, "active": active, "waiting_human": waiting, "failed": failed}, OccurredAt: job.UpdatedAt}); err != nil {
			return job, err
		}
	}
	if ready > 0 {
		s.notifyAvailable(job.TenantID)
	}
	return job, nil
}

func (s *Service) refreshFanoutJoins(ctx context.Context, job JobRun) error {
	sets, err := s.repo.FanoutSets(ctx, job.TenantID, job.ID)
	if err != nil {
		return err
	}
	for _, set := range sets {
		if set.Status != FanoutClosed {
			continue
		}
		if _, err := s.JoinFanoutSet(ctx, job.TenantID, set.ID, "runtime.join"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) TransitionNode(ctx context.Context, tenantID, nodeID, next, actorType, actorID string, expectedVersion int) (NodeRun, error) {
	node, err := s.repo.NodeRun(ctx, tenantID, nodeID)
	if err != nil {
		return node, err
	}
	if err := node.Transition(next); err != nil {
		return node, err
	}
	node.State = next
	node.Version++
	node.UpdatedAt = s.now().UTC()
	if next == NodeLeased || next == NodeRunning {
		if node.LeaseOwner == "" {
			node.LeaseOwner = strings.TrimSpace(actorID)
		}
		if node.FenceToken == "" {
			fenceToken, _, tokenErr := idgen.NewOpaqueToken("rtf_", 24)
			if tokenErr != nil {
				return node, tokenErr
			}
			node.FenceToken = fenceToken
		}
		if node.LeaseExpiresAt == nil {
			expires := node.UpdatedAt.Add(DefaultNodeLeaseDuration)
			node.LeaseExpiresAt = &expires
		}
	} else {
		node.LeaseOwner = ""
		node.FenceToken = ""
		node.LeaseExpiresAt = nil
	}
	commands, err := s.commands()
	if err != nil {
		return node, err
	}
	if _, err = commands.ApplyNodeTransition(ctx, node, expectedVersion, JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "node." + next, ActorType: actorType, ActorID: actorID, Payload: map[string]any{"node_id": node.ID}, OccurredAt: node.UpdatedAt}); err != nil {
		return node, err
	}
	_, _ = s.Refresh(ctx, tenantID, node.JobRunID)
	return node, nil
}

func (s *Service) CompleteNode(ctx context.Context, tenantID, nodeID string, outputRefs []string, outputDigest, actorType, actorID string, expectedVersion int) (NodeRun, error) {
	node, err := s.repo.NodeRun(ctx, tenantID, nodeID)
	if err != nil {
		return node, err
	}
	if node.State != NodeRunning && node.State != NodeWaitingExternal && node.State != NodeWaitingHuman {
		return node, fault.Conflict("NODE_RESULT_NOT_ACCEPTED", "只有执行中的节点可以接收结果")
	}
	if err := node.Transition(NodeSucceeded); err != nil {
		return node, err
	}
	node.State = NodeSucceeded
	node.OutputRefs = append([]string{}, outputRefs...)
	node.OutputDigest = outputDigest
	node.LeaseOwner = ""
	node.FenceToken = ""
	node.LeaseExpiresAt = nil
	node.Version++
	node.UpdatedAt = s.now().UTC()
	commands, err := s.commands()
	if err != nil {
		return node, err
	}
	if _, err = commands.ApplyNodeTransition(ctx, node, expectedVersion, JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "node.succeeded", ActorType: actorType, ActorID: actorID, Payload: map[string]any{"output_count": len(outputRefs), "output_digest": outputDigest}, OccurredAt: node.UpdatedAt}); err != nil {
		return node, err
	}
	_, _ = s.Refresh(ctx, tenantID, node.JobRunID)
	return node, nil
}

func (s *Service) FailNode(ctx context.Context, tenantID, nodeID, errorCode string, retryable bool, actorType, actorID string, expectedVersion int) (NodeRun, error) {
	node, err := s.repo.NodeRun(ctx, tenantID, nodeID)
	if err != nil {
		return node, err
	}
	if node.State != NodeRunning && node.State != NodeWaitingExternal {
		return node, fault.Conflict("NODE_FAILURE_NOT_ACCEPTED", "只有执行中的节点可以记录失败")
	}
	planJob, err := s.repo.JobRun(ctx, tenantID, node.JobRunID)
	if err != nil {
		return node, err
	}
	plan, err := s.repo.Plan(ctx, tenantID, planJob.PlanRevisionID)
	if err != nil {
		return node, err
	}
	maxAttempts := DefaultRuntimeLimits().MaxAttemptsPerNode
	for _, spec := range plan.Nodes {
		if spec.Key == node.NodeKey && spec.RetryMaxAttempts > 0 {
			maxAttempts = spec.RetryMaxAttempts
		}
	}
	next := NodeFailed
	if retryable && node.AttemptCount < maxAttempts {
		next = NodeRetryableFailed
	}
	if err := node.Transition(next); err != nil {
		return node, err
	}
	node.State = next
	node.ErrorCode = strings.TrimSpace(errorCode)
	node.Version++
	node.UpdatedAt = s.now().UTC()
	node.LeaseOwner = ""
	node.FenceToken = ""
	node.LeaseExpiresAt = nil
	commands, err := s.commands()
	if err != nil {
		return node, err
	}
	if _, err = commands.ApplyNodeTransition(ctx, node, expectedVersion, JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "node." + next, ActorType: actorType, ActorID: actorID, Payload: map[string]any{"error_code": node.ErrorCode, "retryable": retryable}, OccurredAt: node.UpdatedAt}); err != nil {
		return node, err
	}
	_, _ = s.Refresh(ctx, tenantID, node.JobRunID)
	return node, nil
}

func (s *Service) Cancel(ctx context.Context, tenantID, jobID, actorType, actorID string) (JobRun, error) {
	job, err := s.repo.JobRun(ctx, tenantID, jobID)
	if err != nil {
		return job, err
	}
	if err := job.Transition(JobRunCancelled); err != nil {
		return job, err
	}
	job.State = JobRunCancelled
	job.Version++
	job.UpdatedAt = s.now().UTC()
	commands, err := s.commands()
	if err != nil {
		return job, err
	}
	if _, err := commands.ApplyJobTransition(ctx, job, job.Version-1, JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: jobID, Type: "job.cancelled", ActorType: actorType, ActorID: actorID, Payload: map[string]any{}, OccurredAt: job.UpdatedAt}); err != nil {
		return job, err
	}
	nodes, err := s.repo.NodeRuns(ctx, tenantID, jobID)
	if err != nil {
		return job, err
	}
	for _, node := range nodes {
		if node.State != NodeSucceeded && node.State != NodeFailed && node.State != NodeCancelled && node.State != NodeSkipped {
			if node.Transition(NodeCancelled) == nil {
				node.State = NodeCancelled
				node.LeaseOwner = ""
				node.FenceToken = ""
				node.LeaseExpiresAt = nil
				node.Version++
				node.UpdatedAt = s.now().UTC()
				_, _ = commands.ApplyNodeTransition(ctx, node, node.Version-1, JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: jobID, NodeKey: node.NodeKey, Type: "node.cancelled", ActorType: actorType, ActorID: actorID, Payload: map[string]any{}, OccurredAt: node.UpdatedAt})
			}
		}
	}
	return job, nil
}

func (s *Service) Checkpoint(ctx context.Context, tenantID, jobID, nodeKey string, stateRefs, outputRefs []string) (Checkpoint, error) {
	job, err := s.repo.JobRun(ctx, tenantID, jobID)
	if err != nil {
		return Checkpoint{}, err
	}
	plan, err := s.repo.Plan(ctx, tenantID, job.PlanRevisionID)
	if err != nil {
		return Checkpoint{}, err
	}
	nodes, err := s.repo.NodeRuns(ctx, tenantID, jobID)
	if err != nil {
		return Checkpoint{}, err
	}
	completed := []string{}
	for _, node := range nodes {
		if node.State == NodeSucceeded || node.State == NodeSkipped {
			completed = append(completed, node.NodeKey)
		}
	}
	sort.Strings(completed)
	events, err := s.repo.JobEvents(ctx, tenantID, jobID, 0)
	if err != nil {
		return Checkpoint{}, err
	}
	collections, err := s.repo.StateCollections(ctx, tenantID, jobID)
	if err != nil {
		return Checkpoint{}, err
	}
	watermarks := map[string]int{}
	for _, collection := range collections {
		watermarks[collection.CollectionKey] = collection.Revision
	}
	now := s.now().UTC()
	checkpoint := Checkpoint{ID: idgen.New(), TenantID: tenantID, JobRunID: jobID, NodeKey: nodeKey, PlanDigest: plan.Digest, StateRefs: append([]string{}, stateRefs...), StateWatermarks: watermarks, OutputRefs: append([]string{}, outputRefs...), CompletedNodes: completed, EventCursor: int64(len(events)), CreatedAt: now}
	digest, err := stablehash.Sum(struct {
		Job, Plan, Node          string
		Completed, State, Output []string
		EventCursor              int64
		StateWatermarks          map[string]int
	}{job.ID, plan.Digest, nodeKey, completed, stateRefs, outputRefs, checkpoint.EventCursor, checkpoint.StateWatermarks})
	if err != nil {
		return checkpoint, err
	}
	checkpoint.Digest = "sha256:" + digest
	if err := s.repo.CreateCheckpoint(ctx, checkpoint); err != nil {
		return checkpoint, err
	}
	return checkpoint, nil
}

// Fork creates a new execution instance from an immutable checkpoint. It does
// not copy mutable state or execute any node; downstream workers must claim
// the new pending nodes through the normal Runtime dispatch path.
func (s *Service) Fork(ctx context.Context, tenantID, checkpointID, createdBy, idempotencyKey string) (StartResult, error) {
	checkpoint, err := s.repo.Checkpoint(ctx, tenantID, checkpointID)
	if err != nil {
		return StartResult{}, err
	}
	source, err := s.repo.JobRun(ctx, tenantID, checkpoint.JobRunID)
	if err != nil {
		return StartResult{}, err
	}
	if key := strings.TrimSpace(idempotencyKey); key != "" {
		if existing, lookupErr := s.repo.JobRunByIdempotencyKey(ctx, tenantID, key); lookupErr == nil {
			if existing.CheckpointID != checkpoint.ID || existing.SourceJobRunID != source.ID {
				return StartResult{}, fault.Conflict("JOB_RUN_IDEMPOTENCY_MISMATCH", "幂等键已用于其他执行分支")
			}
			plan, planErr := s.repo.Plan(ctx, tenantID, existing.PlanRevisionID)
			if planErr != nil {
				return StartResult{}, planErr
			}
			nodes, nodeErr := s.repo.NodeRuns(ctx, tenantID, existing.ID)
			if nodeErr != nil {
				return StartResult{}, nodeErr
			}
			return StartResult{Plan: plan, Job: existing, Nodes: nodes}, nil
		} else if !fault.IsNotFound(lookupErr) {
			return StartResult{}, lookupErr
		}
	}
	if err := s.requireAdmission(tenantID); err != nil {
		return StartResult{}, err
	}
	plan, err := s.repo.Plan(ctx, tenantID, source.PlanRevisionID)
	if err != nil {
		return StartResult{}, err
	}
	if checkpoint.PlanDigest != plan.Digest || source.PlanDigest != plan.Digest {
		return StartResult{}, fault.Conflict("RUNTIME_CHECKPOINT_PLAN_DRIFT", "检查点与源执行实例的计划摘要不一致")
	}
	switch source.State {
	case JobRunPaused, JobRunCompleted, JobRunFailed, JobRunCancelled, JobRunRejected:
	default:
		return StartResult{}, fault.Policy("RUNTIME_FORK_SOURCE_ACTIVE", "源执行实例仍在活动，不能同时创建执行分支", "先暂停或结束源执行实例，再从检查点创建分支")
	}
	if len(checkpoint.StateWatermarks) > 0 {
		return StartResult{}, fault.Policy("RUNTIME_FORK_STATE_INHERITANCE_UNAVAILABLE", "检查点包含共享状态水位，当前版本不能安全创建分支", "完成按水位冻结的状态继承后再创建执行分支")
	}
	effects, err := s.repo.Effects(ctx, tenantID, source.ID)
	if err != nil {
		return StartResult{}, err
	}
	for _, effect := range effects {
		if effect.State == EffectUnknown || effect.State == EffectReconciling {
			return StartResult{}, fault.Policy("RUNTIME_FORK_EFFECT_UNRESOLVED", "源执行实例仍有结果不明的外部副作用", "先完成外部副作用对账，再从检查点创建分支")
		}
	}
	sourceNodes, err := s.repo.NodeRuns(ctx, tenantID, source.ID)
	if err != nil {
		return StartResult{}, err
	}
	sourceByKey := make(map[string]NodeRun, len(sourceNodes))
	for _, node := range sourceNodes {
		sourceByKey[node.NodeKey] = node
	}
	completed := make(map[string]struct{}, len(checkpoint.CompletedNodes))
	for _, key := range checkpoint.CompletedNodes {
		node, ok := sourceByKey[key]
		if !ok || (node.State != NodeSucceeded && node.State != NodeSkipped) {
			return StartResult{}, fault.Conflict("RUNTIME_CHECKPOINT_NODE_DRIFT", "检查点记录的已完成节点与源执行实例不一致")
		}
		completed[key] = struct{}{}
	}
	now := s.now().UTC()
	job := JobRun{ID: idgen.New(), TenantID: tenantID, ProjectID: source.ProjectID, WorkTaskID: source.WorkTaskID, PlanRevisionID: plan.ID, PlanDigest: plan.Digest, BindingDigest: source.BindingDigest, InputDigest: source.InputDigest, RuntimePolicyID: source.RuntimePolicyID, ContractMajor: source.ContractMajor, ContractMinor: source.ContractMinor, RootJobRunID: source.RootJobRunID, SourceJobRunID: source.ID, CheckpointID: checkpoint.ID, IdempotencyKey: strings.TrimSpace(idempotencyKey), State: JobRunCreated, Priority: source.Priority, Version: 1, CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now}
	if job.RootJobRunID == "" {
		job.RootJobRunID = source.ID
	}
	nodes := make([]NodeRun, 0, len(plan.Nodes))
	for _, spec := range plan.Nodes {
		node := NodeRun{ID: idgen.New(), TenantID: tenantID, JobRunID: job.ID, NodeKey: spec.Key, State: NodePending, OutputRefs: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now}
		if _, ok := completed[spec.Key]; ok {
			sourceNode := sourceByKey[spec.Key]
			node.State = sourceNode.State
			node.OutputRefs = append([]string{}, sourceNode.OutputRefs...)
			node.OutputDigest = sourceNode.OutputDigest
		}
		nodes = append(nodes, node)
	}
	event := JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: job.ID, Sequence: 1, Type: "job.forked", ActorType: "user", ActorID: createdBy, IdempotencyKey: strings.TrimSpace(idempotencyKey), Payload: map[string]any{"source_job_run_id": source.ID, "checkpoint_id": checkpoint.ID, "plan_digest": plan.Digest, "reused_completed_nodes": len(completed), "reused_output_refs": len(checkpoint.OutputRefs)}, OccurredAt: now}
	if err := s.repo.CreateJobBundle(ctx, job, nodes, event); err != nil {
		return StartResult{}, err
	}
	if _, err := s.refresh(ctx, job); err != nil {
		return StartResult{}, err
	}
	job, err = s.repo.JobRun(ctx, tenantID, job.ID)
	if err != nil {
		return StartResult{}, err
	}
	nodes, err = s.repo.NodeRuns(ctx, tenantID, job.ID)
	if err != nil {
		return StartResult{}, err
	}
	return StartResult{Plan: plan, Job: job, Nodes: nodes}, nil
}

type ReplayResult struct {
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

// Replay rebuilds only the derived Runtime Explorer projection from durable
// facts. It has no harness/provider dependency and reports zero external calls
// as an invariant.
func (s *Service) Replay(ctx context.Context, tenantID, jobID string, after int64) (ReplayResult, error) {
	return s.ReplayWithOptions(ctx, tenantID, jobID, after, false)
}

func (s *Service) ReplayWithOptions(ctx context.Context, tenantID, jobID string, after int64, dryRun bool) (result ReplayResult, err error) {
	mode := "rebuild"
	if dryRun {
		mode = "dry_run"
	}
	startedAt := s.now().UTC()
	run := RuntimeProjectionRebuildRun{ID: idgen.New(), TenantID: tenantID, JobRunID: jobID, Mode: mode, Status: "running", IntegrityStatus: "running", StartedAt: startedAt, Version: 1}
	if err = s.repo.CreateRuntimeProjectionRebuild(ctx, run); err != nil {
		return ReplayResult{}, err
	}
	result = ReplayResult{TenantID: tenantID, JobRunID: jobID, RebuildRunID: run.ID, DryRun: dryRun}
	finish := func(status, integrity, errorCode string) {
		now := s.now().UTC()
		run.Status, run.IntegrityStatus, run.ErrorCode, run.FinishedAt, run.Version = status, integrity, errorCode, &now, run.Version+1
		run.EventCount, run.LastSequence, run.ExternalCalls = result.EventCount, result.LastSequence, result.ExternalCalls
		if updateErr := s.repo.UpdateRuntimeProjectionRebuild(ctx, run, run.Version-1); err == nil && updateErr != nil {
			err = updateErr
		}
	}
	defer func() {
		if err != nil && run.Status == "running" {
			finish("failed", "failed", errorCode(err, "RUNTIME_REPLAY_FAILED"))
		}
	}()
	job, err := s.repo.JobRun(ctx, tenantID, jobID)
	if err != nil {
		return result, err
	}
	plan, err := s.repo.Plan(ctx, tenantID, job.PlanRevisionID)
	if err != nil {
		return result, err
	}
	if job.PlanDigest != plan.Digest {
		return result, fault.Conflict("RUNTIME_REPLAY_PLAN_DRIFT", "执行实例与不可变计划的摘要不一致")
	}
	nodes, err := s.repo.NodeRuns(ctx, tenantID, jobID)
	if err != nil {
		return result, err
	}
	planNodes := make(map[string]struct{}, len(plan.Nodes))
	for _, spec := range plan.Nodes {
		planNodes[spec.Key] = struct{}{}
	}
	seenNodes := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if _, ok := planNodes[node.NodeKey]; !ok {
			return result, fault.Conflict("RUNTIME_REPLAY_NODE_DRIFT", "执行节点不属于不可变计划")
		}
		if _, exists := seenNodes[node.NodeKey]; exists {
			return result, fault.Conflict("RUNTIME_REPLAY_NODE_DUPLICATE", "执行实例包含重复节点")
		}
		seenNodes[node.NodeKey] = struct{}{}
	}
	if len(seenNodes) != len(planNodes) {
		return result, fault.Conflict("RUNTIME_REPLAY_NODE_MISSING", "执行实例缺少计划节点")
	}
	events, err := s.repo.JobEvents(ctx, tenantID, jobID, 0)
	if err != nil {
		return result, err
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) {
			return result, fault.Conflict("RUNTIME_REPLAY_EVENT_GAP", "运行事件序列不连续，已停止重放")
		}
	}
	lastSequence := int64(0)
	sourceEventID := ""
	if len(events) > 0 {
		lastSequence = events[len(events)-1].Sequence
		sourceEventID = events[len(events)-1].ID
	}
	if !dryRun {
		if err := s.repo.SaveRuntimeExplorer(ctx, RuntimeExplorerView{TenantID: tenantID, JobRunID: jobID, Job: job, Nodes: nodes, LastEventSeq: lastSequence, ProjectedAt: s.now().UTC(), SourceEventID: sourceEventID}); err != nil {
			return result, err
		}
	}
	eventCount := 0
	for _, event := range events {
		if event.Sequence > after {
			eventCount++
		}
	}
	result.EventCount, result.LastSequence, result.ExternalCalls, result.ProjectionRebuilt, result.IntegrityStatus = eventCount, lastSequence, 0, !dryRun, "verified"
	finish("completed", "verified", "")
	return result, err
}

func (s *Service) MutateState(ctx context.Context, tenantID, jobID string, mutation StateMutation, actorType, actorID string) (RuntimeState, error) {
	if strings.TrimSpace(mutation.Collection) == "" || strings.TrimSpace(mutation.IdempotencyKey) == "" {
		return RuntimeState{}, fault.Invalid("RUNTIME_STATE_MUTATION_INVALID", "状态变更需要集合名和幂等键")
	}
	commands, err := s.commands()
	if err != nil {
		return RuntimeState{}, err
	}
	now := s.now().UTC()
	return commands.ApplyStateMutation(ctx, tenantID, jobID, mutation, JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: jobID, Type: "state.mutated", ActorType: actorType, ActorID: actorID, Payload: map[string]any{"collection": mutation.Collection}, OccurredAt: now})
}

func (s *Service) RegisterEffect(ctx context.Context, effect ExternalEffect) (ExternalEffect, error) {
	return s.registerEffect(ctx, effect, "")
}

func (s *Service) RegisterEffectForAttempt(ctx context.Context, effect ExternalEffect, fenceToken string) (ExternalEffect, error) {
	if strings.TrimSpace(effect.AttemptID) == "" || strings.TrimSpace(fenceToken) == "" {
		return effect, fault.Invalid("MCP_GATEWAY_FENCE_REQUIRED", "Effect 需要 Attempt 和 fence_token")
	}
	return s.registerEffect(ctx, effect, strings.TrimSpace(fenceToken))
}

func (s *Service) registerEffect(ctx context.Context, effect ExternalEffect, fenceToken string) (ExternalEffect, error) {
	if existing, err := s.repo.EffectByIdempotencyKey(ctx, effect.TenantID, effect.IdempotencyKey); err == nil {
		return existing, nil
	} else if !fault.IsNotFound(err) {
		return effect, err
	}
	if effect.ID == "" {
		effect.ID = idgen.New()
	}
	if effect.State == "" {
		effect.State = EffectRegistered
	}
	if effect.Version < 1 {
		effect.Version = 1
	}
	if effect.AttemptID == "" && effect.NodeRunID != "" {
		if attempts, attemptErr := s.repo.RuntimeAttempts(ctx, effect.TenantID, effect.JobRunID); attemptErr == nil {
			for index := len(attempts) - 1; index >= 0; index-- {
				if attempts[index].NodeRunID == effect.NodeRunID && !attempts[index].Terminal() {
					effect.AttemptID = attempts[index].ID
					break
				}
			}
		}
	}
	if effect.CreatedAt.IsZero() {
		effect.CreatedAt = s.now().UTC()
	}
	effect.UpdatedAt = effect.CreatedAt
	commands, err := s.commands()
	if err != nil {
		return effect, err
	}
	event := JobEvent{ID: idgen.New(), TenantID: effect.TenantID, JobRunID: effect.JobRunID, Type: "effect.registered", ActorType: "runtime", Payload: map[string]any{"kind": effect.Kind, "idempotency_key": effect.IdempotencyKey}, OccurredAt: effect.CreatedAt}
	if fenceToken != "" {
		return commands.RegisterFencedEffectCommand(ctx, effect, fenceToken, effect.CreatedAt, event)
	}
	return commands.RegisterEffectCommand(ctx, effect, event)
}

func (s *Service) ReconcileEffect(ctx context.Context, tenantID, effectID, next, externalID, responseDigest, errorCode string, expectedVersion int) (ExternalEffect, error) {
	effect, err := s.repo.Effect(ctx, tenantID, effectID)
	if err != nil {
		return ExternalEffect{}, err
	}
	if err := effect.Transition(next); err != nil {
		return effect, err
	}
	effect.State = next
	effect.ExternalID = externalID
	effect.ResponseDigest = responseDigest
	effect.ErrorCode = errorCode
	effect.Version++
	effect.UpdatedAt = s.now().UTC()
	commands, err := s.commands()
	if err != nil {
		return effect, err
	}
	return commands.ApplyEffectTransition(ctx, effect, expectedVersion, JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: effect.JobRunID, NodeKey: "", Type: "effect." + next, ActorType: "reconciler", Payload: map[string]any{"effect_id": effect.ID, "state": next}, OccurredAt: effect.UpdatedAt})
}

// BeginEffectReconciliation is the only automatic action for an unknown
// effect. It changes the durable state to reconciling and never submits a new
// provider request.
func (s *Service) BeginEffectReconciliation(ctx context.Context, tenantID, effectID string, expectedVersion int) (ExternalEffect, error) {
	effect, err := s.ReconcileEffect(ctx, tenantID, effectID, EffectReconciling, "", "", "", expectedVersion)
	if err != nil {
		return effect, err
	}
	return effect, nil
}
