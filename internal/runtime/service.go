package runtime

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
)

type Service struct {
	repo      Repository
	now       func() time.Time
	compiler  Compiler
	harnesses *agentadapter.HarnessRegistry
}

const DefaultNodeLeaseDuration = 5 * time.Minute

func New(repo Repository, now func() time.Time) *Service {
	return NewWithHarnessRegistry(repo, now, nil)
}

func NewWithHarnessRegistry(repo Repository, now func() time.Time, harnesses *agentadapter.HarnessRegistry) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now, compiler: NewCompiler(domain.DefaultRuntimeLimits()), harnesses: harnesses}
}

func (s *Service) Repository() Repository { return s.repo }

type StartInput struct {
	TenantID       string
	ProjectID      string
	WorkTaskID     string
	SOP            domain.SOPVersion
	Priority       int
	CreatedBy      string
	IdempotencyKey string
	CorrelationID  string
}

type StartResult struct {
	Plan  domain.JobPlanRevision `json:"plan"`
	Job   domain.JobRun          `json:"job"`
	Nodes []domain.NodeRun       `json:"nodes"`
}

func (s *Service) Start(ctx context.Context, input StartInput) (StartResult, error) {
	if s == nil || s.repo == nil {
		return StartResult{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.WorkTaskID) == "" {
		return StartResult{}, domain.Invalid("JOB_RUN_INPUT_INVALID", "创建执行实例缺少租户或任务")
	}
	if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
		if existing, err := s.repo.JobRunByIdempotencyKey(ctx, input.TenantID, key); err == nil {
			plan, planErr := s.repo.Plan(ctx, input.TenantID, existing.PlanRevisionID)
			if planErr != nil {
				return StartResult{}, planErr
			}
			nodes, nodeErr := s.repo.NodeRuns(ctx, input.TenantID, existing.ID)
			if nodeErr != nil {
				return StartResult{}, nodeErr
			}
			return StartResult{Plan: plan, Job: existing, Nodes: nodes}, nil
		} else if !domain.IsNotFound(err) {
			return StartResult{}, err
		}
	}
	now := s.now().UTC()
	plan, err := s.compiler.CompileSOP(input.SOP, input.TenantID, input.CreatedBy, now)
	if err != nil {
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
				err = nil
				break
			}
		}
		if err != nil {
			return StartResult{}, err
		}
	}
	state := domain.JobRunCreated
	job := domain.JobRun{ID: domain.NewID(), TenantID: input.TenantID, ProjectID: input.ProjectID, WorkTaskID: input.WorkTaskID, PlanRevisionID: plan.ID, PlanDigest: plan.Digest, IdempotencyKey: strings.TrimSpace(input.IdempotencyKey), State: state, Priority: input.Priority, Version: 1, CreatedBy: input.CreatedBy, CreatedAt: now, UpdatedAt: now}
	nodes := make([]domain.NodeRun, 0, len(plan.Nodes))
	for _, node := range plan.Nodes {
		nodes = append(nodes, domain.NodeRun{ID: domain.NewID(), TenantID: input.TenantID, JobRunID: job.ID, NodeKey: node.Key, State: domain.NodePending, OutputRefs: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now})
	}
	event := domain.JobEvent{ID: domain.NewID(), TenantID: input.TenantID, JobRunID: job.ID, Sequence: 1, Type: "job.created", ActorType: "user", ActorID: input.CreatedBy, CorrelationID: input.CorrelationID, IdempotencyKey: input.IdempotencyKey, Payload: map[string]any{"plan_digest": plan.Digest, "node_count": len(nodes)}, OccurredAt: now}
	if err := s.repo.CreateJobBundle(ctx, job, nodes, event); err != nil {
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

func (s *Service) Job(ctx context.Context, tenantID, id string) (domain.JobRun, error) {
	return s.repo.JobRun(ctx, tenantID, id)
}
func (s *Service) Jobs(ctx context.Context, tenantID, taskID string) ([]domain.JobRun, error) {
	return s.repo.JobRuns(ctx, tenantID, taskID)
}
func (s *Service) Plan(ctx context.Context, tenantID, id string) (domain.JobPlanRevision, error) {
	return s.repo.Plan(ctx, tenantID, id)
}
func (s *Service) Nodes(ctx context.Context, tenantID, jobID string) ([]domain.NodeRun, error) {
	return s.repo.NodeRuns(ctx, tenantID, jobID)
}

// ClaimNode is the scheduler/worker boundary for the new Runtime graph. The
// repository performs the atomic claim; the service only records a diagnostic
// event after the claim succeeds.
func (s *Service) ClaimNode(ctx context.Context, tenantID, jobID, owner string, leaseFor time.Duration) (domain.NodeRun, error) {
	if s == nil || s.repo == nil {
		return domain.NodeRun{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if leaseFor <= 0 {
		leaseFor = DefaultNodeLeaseDuration
	}
	node, err := s.repo.ClaimReadyNode(ctx, tenantID, jobID, owner, s.now().UTC(), leaseFor)
	if err != nil {
		return domain.NodeRun{}, err
	}
	_, eventErr := s.repo.AppendJobEvent(ctx, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "node.leased", ActorType: "scheduler", ActorID: strings.TrimSpace(owner), Payload: map[string]any{"attempt_count": node.AttemptCount, "lease_expires_at": node.LeaseExpiresAt}, OccurredAt: s.now().UTC()})
	if eventErr != nil {
		return node, eventErr
	}
	return node, nil
}

// HeartbeatNode renews a lease and promotes the first heartbeat from leased to
// running. Version and owner checks make late workers fail closed.
func (s *Service) HeartbeatNode(ctx context.Context, tenantID, nodeID, owner string, expectedVersion int, leaseFor time.Duration) (domain.NodeRun, error) {
	if s == nil || s.repo == nil {
		return domain.NodeRun{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if leaseFor <= 0 {
		leaseFor = DefaultNodeLeaseDuration
	}
	node, err := s.repo.HeartbeatNode(ctx, tenantID, nodeID, owner, expectedVersion, s.now().UTC(), leaseFor)
	if err != nil {
		return domain.NodeRun{}, err
	}
	_, eventErr := s.repo.AppendJobEvent(ctx, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "node.heartbeat", ActorType: "worker", ActorID: strings.TrimSpace(owner), Payload: map[string]any{"state": node.State, "lease_expires_at": node.LeaseExpiresAt}, OccurredAt: s.now().UTC()})
	if eventErr != nil {
		return node, eventErr
	}
	return node, nil
}

// ExpireNodeLeases is safe to call from a periodic scheduler tick. Expired
// nodes are first marked by the repository and then re-evaluated by Refresh so
// dependency readiness and the JobRun projection advance together.
func (s *Service) ExpireNodeLeases(ctx context.Context, tenantID string, now time.Time) error {
	if s == nil || s.repo == nil {
		return domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
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
func (s *Service) Events(ctx context.Context, tenantID, jobID string, after int64) ([]domain.JobEvent, error) {
	return s.repo.JobEvents(ctx, tenantID, jobID, after)
}
func (s *Service) Effects(ctx context.Context, tenantID, jobID string) ([]domain.ExternalEffect, error) {
	return s.repo.Effects(ctx, tenantID, jobID)
}
func (s *Service) Checkpoints(ctx context.Context, tenantID, jobID string) ([]domain.Checkpoint, error) {
	return s.repo.Checkpoints(ctx, tenantID, jobID)
}

func (s *Service) Refresh(ctx context.Context, tenantID, jobID string) (domain.JobRun, error) {
	job, err := s.repo.JobRun(ctx, tenantID, jobID)
	if err != nil {
		return domain.JobRun{}, err
	}
	return s.refresh(ctx, job)
}

func (s *Service) Resume(ctx context.Context, tenantID, jobID, actorType, actorID string) (domain.JobRun, error) {
	job, err := s.repo.JobRun(ctx, tenantID, jobID)
	if err != nil {
		return domain.JobRun{}, err
	}
	if job.State != domain.JobRunPaused {
		return domain.JobRun{}, domain.Conflict("JOB_RUN_NOT_RESUMABLE", "只有已暂停的执行实例可以恢复")
	}
	if err := job.Transition(domain.JobRunRunning); err != nil {
		return domain.JobRun{}, err
	}
	job.State = domain.JobRunRunning
	job.Version++
	job.UpdatedAt = s.now().UTC()
	if err := s.repo.SaveJobRun(ctx, job, job.Version-1); err != nil {
		return domain.JobRun{}, err
	}
	if _, err := s.repo.AppendJobEvent(ctx, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: jobID, Type: "job.resumed", ActorType: actorType, ActorID: actorID, OccurredAt: s.now().UTC()}); err != nil {
		return domain.JobRun{}, err
	}
	return s.refresh(ctx, job)
}

func (s *Service) refresh(ctx context.Context, job domain.JobRun) (domain.JobRun, error) {
	if job.State == domain.JobRunCompleted || job.State == domain.JobRunFailed || job.State == domain.JobRunCancelled || job.State == domain.JobRunRejected {
		return job, nil
	}
	plan, err := s.repo.Plan(ctx, job.TenantID, job.PlanRevisionID)
	if err != nil {
		return job, err
	}
	nodes, err := s.repo.NodeRuns(ctx, job.TenantID, job.ID)
	if err != nil {
		return job, err
	}
	byKey := map[string]domain.NodeRun{}
	for _, node := range nodes {
		byKey[node.NodeKey] = node
	}
	changed := false
	for _, spec := range plan.Nodes {
		node := byKey[spec.Key]
		if node.State != domain.NodePending && node.State != domain.NodeRetryableFailed && node.State != domain.NodeLeaseExpired && node.State != domain.NodeWaitingResource {
			continue
		}
		allSucceeded, blocked := true, false
		for _, dep := range spec.DependsOn {
			dependency, ok := byKey[dep]
			if !ok || dependency.State != domain.NodeSucceeded {
				allSucceeded = false
			}
			if ok && (dependency.State == domain.NodeFailed || dependency.State == domain.NodeBlocked || dependency.State == domain.NodeCancelled) {
				blocked = true
			}
		}
		next := node.State
		if blocked {
			next = domain.NodeBlocked
		} else if allSucceeded {
			next = domain.NodeReady
		}
		if next != node.State {
			if err := node.Transition(next); err != nil {
				return job, err
			}
			node.State = next
			node.Version++
			node.UpdatedAt = s.now().UTC()
			if err := s.repo.SaveNodeRun(ctx, node, node.Version-1); err != nil {
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
		case domain.NodeReady:
			ready++
		case domain.NodeLeased, domain.NodeRunning, domain.NodeWaitingExternal:
			active++
		case domain.NodeWaitingHuman:
			waiting++
		case domain.NodeFailed, domain.NodeBlocked:
			failed++
		case domain.NodeSucceeded, domain.NodeSkipped, domain.NodeCancelled:
			terminal++
		}
	}
	next := job.State
	switch {
	case job.State == domain.JobRunCreated:
		next = domain.JobRunAdmitted
	case failed > 0:
		next = domain.JobRunFailed
	case terminal == len(nodes) && len(nodes) > 0:
		next = domain.JobRunCompleted
	case waiting > 0:
		next = domain.JobRunWaitingHuman
	case active > 0 || ready > 0:
		next = domain.JobRunRunning
	}
	if next != job.State {
		if err := job.Transition(next); err != nil {
			return job, err
		}
		job.State = next
		job.Version++
		job.UpdatedAt = s.now().UTC()
		if err := s.repo.SaveJobRun(ctx, job, job.Version-1); err != nil {
			return job, err
		}
		_, _ = s.repo.AppendJobEvent(ctx, domain.JobEvent{ID: domain.NewID(), TenantID: job.TenantID, JobRunID: job.ID, Type: "job." + next, ActorType: "runtime", Payload: map[string]any{"ready": ready, "active": active, "waiting_human": waiting, "failed": failed}, OccurredAt: s.now().UTC()})
	}
	return job, nil
}

func (s *Service) TransitionNode(ctx context.Context, tenantID, nodeID, next, actorType, actorID string, expectedVersion int) (domain.NodeRun, error) {
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
	if err := s.repo.SaveNodeRun(ctx, node, expectedVersion); err != nil {
		return node, err
	}
	_, err = s.repo.AppendJobEvent(ctx, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "node." + next, ActorType: actorType, ActorID: actorID, Payload: map[string]any{"node_id": node.ID}, OccurredAt: s.now().UTC()})
	if err != nil {
		return node, err
	}
	_, _ = s.Refresh(ctx, tenantID, node.JobRunID)
	return node, nil
}

func (s *Service) CompleteNode(ctx context.Context, tenantID, nodeID string, outputRefs []string, outputDigest, actorType, actorID string, expectedVersion int) (domain.NodeRun, error) {
	node, err := s.repo.NodeRun(ctx, tenantID, nodeID)
	if err != nil {
		return node, err
	}
	if node.State != domain.NodeRunning && node.State != domain.NodeWaitingExternal && node.State != domain.NodeWaitingHuman {
		return node, domain.Conflict("NODE_RESULT_NOT_ACCEPTED", "只有执行中的节点可以接收结果")
	}
	if err := node.Transition(domain.NodeSucceeded); err != nil {
		return node, err
	}
	node.State = domain.NodeSucceeded
	node.OutputRefs = append([]string{}, outputRefs...)
	node.OutputDigest = outputDigest
	node.LeaseOwner = ""
	node.LeaseExpiresAt = nil
	node.Version++
	node.UpdatedAt = s.now().UTC()
	if err := s.repo.SaveNodeRun(ctx, node, expectedVersion); err != nil {
		return node, err
	}
	_, err = s.repo.AppendJobEvent(ctx, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "node.succeeded", ActorType: actorType, ActorID: actorID, Payload: map[string]any{"output_count": len(outputRefs), "output_digest": outputDigest}, OccurredAt: s.now().UTC()})
	if err != nil {
		return node, err
	}
	_, _ = s.Refresh(ctx, tenantID, node.JobRunID)
	return node, nil
}

func (s *Service) FailNode(ctx context.Context, tenantID, nodeID, errorCode string, retryable bool, actorType, actorID string, expectedVersion int) (domain.NodeRun, error) {
	node, err := s.repo.NodeRun(ctx, tenantID, nodeID)
	if err != nil {
		return node, err
	}
	if node.State != domain.NodeRunning && node.State != domain.NodeWaitingExternal {
		return node, domain.Conflict("NODE_FAILURE_NOT_ACCEPTED", "只有执行中的节点可以记录失败")
	}
	planJob, err := s.repo.JobRun(ctx, tenantID, node.JobRunID)
	if err != nil {
		return node, err
	}
	plan, err := s.repo.Plan(ctx, tenantID, planJob.PlanRevisionID)
	if err != nil {
		return node, err
	}
	maxAttempts := domain.DefaultRuntimeLimits().MaxAttemptsPerNode
	for _, spec := range plan.Nodes {
		if spec.Key == node.NodeKey && spec.RetryMaxAttempts > 0 {
			maxAttempts = spec.RetryMaxAttempts
		}
	}
	next := domain.NodeFailed
	if retryable && node.AttemptCount < maxAttempts {
		next = domain.NodeRetryableFailed
	}
	if err := node.Transition(next); err != nil {
		return node, err
	}
	node.State = next
	node.ErrorCode = strings.TrimSpace(errorCode)
	node.Version++
	node.UpdatedAt = s.now().UTC()
	node.LeaseOwner = ""
	node.LeaseExpiresAt = nil
	if err := s.repo.SaveNodeRun(ctx, node, expectedVersion); err != nil {
		return node, err
	}
	_, err = s.repo.AppendJobEvent(ctx, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: node.JobRunID, NodeKey: node.NodeKey, Type: "node." + next, ActorType: actorType, ActorID: actorID, Payload: map[string]any{"error_code": node.ErrorCode, "retryable": retryable}, OccurredAt: s.now().UTC()})
	if err != nil {
		return node, err
	}
	_, _ = s.Refresh(ctx, tenantID, node.JobRunID)
	return node, nil
}

func (s *Service) Cancel(ctx context.Context, tenantID, jobID, actorType, actorID string) (domain.JobRun, error) {
	job, err := s.repo.JobRun(ctx, tenantID, jobID)
	if err != nil {
		return job, err
	}
	if err := job.Transition(domain.JobRunCancelled); err != nil {
		return job, err
	}
	job.State = domain.JobRunCancelled
	job.Version++
	job.UpdatedAt = s.now().UTC()
	if err := s.repo.SaveJobRun(ctx, job, job.Version-1); err != nil {
		return job, err
	}
	nodes, err := s.repo.NodeRuns(ctx, tenantID, jobID)
	if err != nil {
		return job, err
	}
	for _, node := range nodes {
		if node.State != domain.NodeSucceeded && node.State != domain.NodeFailed && node.State != domain.NodeCancelled && node.State != domain.NodeSkipped {
			if node.Transition(domain.NodeCancelled) == nil {
				node.State = domain.NodeCancelled
				node.Version++
				node.UpdatedAt = s.now().UTC()
				_ = s.repo.SaveNodeRun(ctx, node, node.Version-1)
			}
		}
	}
	_, _ = s.repo.AppendJobEvent(ctx, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: jobID, Type: "job.cancelled", ActorType: actorType, ActorID: actorID, OccurredAt: s.now().UTC()})
	return job, nil
}

func (s *Service) Checkpoint(ctx context.Context, tenantID, jobID, nodeKey string, stateRefs, outputRefs []string) (domain.Checkpoint, error) {
	job, err := s.repo.JobRun(ctx, tenantID, jobID)
	if err != nil {
		return domain.Checkpoint{}, err
	}
	plan, err := s.repo.Plan(ctx, tenantID, job.PlanRevisionID)
	if err != nil {
		return domain.Checkpoint{}, err
	}
	nodes, err := s.repo.NodeRuns(ctx, tenantID, jobID)
	if err != nil {
		return domain.Checkpoint{}, err
	}
	completed := []string{}
	for _, node := range nodes {
		if node.State == domain.NodeSucceeded || node.State == domain.NodeSkipped {
			completed = append(completed, node.NodeKey)
		}
	}
	sort.Strings(completed)
	now := s.now().UTC()
	checkpoint := domain.Checkpoint{ID: domain.NewID(), TenantID: tenantID, JobRunID: jobID, NodeKey: nodeKey, PlanDigest: plan.Digest, StateRefs: append([]string{}, stateRefs...), OutputRefs: append([]string{}, outputRefs...), CompletedNodes: completed, CreatedAt: now}
	digest, err := domain.CanonicalHash(struct {
		Job, Plan, Node          string
		Completed, State, Output []string
	}{job.ID, plan.Digest, nodeKey, completed, stateRefs, outputRefs})
	if err != nil {
		return checkpoint, err
	}
	checkpoint.Digest = "sha256:" + digest
	if err := s.repo.CreateCheckpoint(ctx, checkpoint); err != nil {
		return checkpoint, err
	}
	return checkpoint, nil
}

func (s *Service) MutateState(ctx context.Context, tenantID, jobID string, mutation domain.StateMutation, actorType, actorID string) (domain.RuntimeState, error) {
	if strings.TrimSpace(mutation.Collection) == "" || strings.TrimSpace(mutation.IdempotencyKey) == "" {
		return domain.RuntimeState{}, domain.Invalid("RUNTIME_STATE_MUTATION_INVALID", "状态变更需要集合名和幂等键")
	}
	state, err := s.repo.RuntimeState(ctx, tenantID, jobID, mutation.Collection)
	if domain.IsNotFound(err) {
		state = domain.RuntimeState{ID: domain.NewID(), TenantID: tenantID, JobRunID: jobID, Collection: mutation.Collection, SchemaVersion: domain.RuntimeStateSchema, Revision: 0, Values: map[string]any{}, UpdatedAt: s.now().UTC()}
		err = nil
	}
	if err != nil {
		return domain.RuntimeState{}, err
	}
	if mutation.ExpectedRevision != state.Revision {
		return domain.RuntimeState{}, domain.Conflict("RUNTIME_STATE_CAS_CONFLICT", "运行状态已经更新，请重新读取后再提交")
	}
	if state.Values == nil {
		state.Values = map[string]any{}
	}
	for key, value := range mutation.Set {
		state.Values[key] = value
	}
	for key, values := range mutation.Append {
		current, _ := state.Values[key].([]any)
		state.Values[key] = append(current, values...)
	}
	state.Revision++
	state.UpdatedAt = s.now().UTC()
	if err := s.repo.SaveRuntimeStateCAS(ctx, state, mutation.ExpectedRevision, mutation.IdempotencyKey); err != nil {
		return domain.RuntimeState{}, err
	}
	_, _ = s.repo.AppendJobEvent(ctx, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: jobID, Type: "state.mutated", ActorType: actorType, ActorID: actorID, Payload: map[string]any{"collection": mutation.Collection, "revision": state.Revision}, OccurredAt: s.now().UTC()})
	return state, nil
}

func (s *Service) RegisterEffect(ctx context.Context, effect domain.ExternalEffect) (domain.ExternalEffect, error) {
	if existing, err := s.repo.EffectByIdempotencyKey(ctx, effect.TenantID, effect.IdempotencyKey); err == nil {
		return existing, nil
	} else if !domain.IsNotFound(err) {
		return effect, err
	}
	if effect.ID == "" {
		effect.ID = domain.NewID()
	}
	if effect.State == "" {
		effect.State = domain.EffectRegistered
	}
	if effect.Version < 1 {
		effect.Version = 1
	}
	if effect.CreatedAt.IsZero() {
		effect.CreatedAt = s.now().UTC()
	}
	effect.UpdatedAt = effect.CreatedAt
	if err := s.repo.CreateEffect(ctx, effect); err != nil {
		return effect, err
	}
	return effect, nil
}

func (s *Service) ReconcileEffect(ctx context.Context, tenantID, effectID, next, externalID, responseDigest, errorCode string, expectedVersion int) (domain.ExternalEffect, error) {
	effects, err := s.repo.Effects(ctx, tenantID, "")
	if err != nil {
		return domain.ExternalEffect{}, err
	}
	var effect domain.ExternalEffect
	for _, candidate := range effects {
		if candidate.ID == effectID {
			effect = candidate
			break
		}
	}
	if effect.ID == "" {
		return effect, domain.NotFound("外部操作")
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
	if err := s.repo.SaveEffect(ctx, effect, expectedVersion); err != nil {
		return effect, err
	}
	return effect, nil
}
