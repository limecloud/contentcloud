package memory

import (
	"context"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func runtimePlanKey(tenantID, id string) string { return tenantID + ":" + id }
func runtimeNodeKey(tenantID, id string) string { return tenantID + ":" + id }
func runtimeStateKey(tenantID, jobID, collection string) string {
	return tenantID + ":" + jobID + ":" + collection
}
func runtimeEffectKey(tenantID, id string) string { return tenantID + ":" + id }

func (s *Store) CreatePlan(_ context.Context, plan domain.JobPlanRevision) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimePlanKey(plan.TenantID, plan.ID)
	if _, ok := s.runtimePlans[key]; ok {
		return domain.Conflict("JOB_PLAN_EXISTS", "执行计划版本已存在")
	}
	for _, candidate := range s.runtimePlans {
		if candidate.TenantID == plan.TenantID && candidate.Digest == plan.Digest {
			return domain.Conflict("JOB_PLAN_DIGEST_EXISTS", "相同摘要的执行计划版本已存在")
		}
	}
	s.runtimePlans[key] = plan
	return nil
}

func (s *Store) Plan(_ context.Context, tenantID, id string) (domain.JobPlanRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimePlans[runtimePlanKey(tenantID, id)]
	if !ok {
		return value, domain.NotFound("执行计划")
	}
	return value, nil
}

func (s *Store) Plans(_ context.Context, tenantID string) ([]domain.JobPlanRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.JobPlanRevision{}
	for _, value := range s.runtimePlans {
		if value.TenantID == tenantID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CompiledAt.After(result[j].CompiledAt) })
	return result, nil
}

func (s *Store) CreateJobBundle(_ context.Context, job domain.JobRun, nodes []domain.NodeRun, event domain.JobEvent) error {
	if err := job.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if job.IdempotencyKey != "" {
		for _, candidate := range s.runtimeJobs {
			if candidate.TenantID == job.TenantID && candidate.IdempotencyKey == job.IdempotencyKey {
				return domain.Conflict("JOB_RUN_IDEMPOTENCY", "相同幂等键的执行实例已存在")
			}
		}
	}
	if _, ok := s.runtimeJobs[runtimePlanKey(job.TenantID, job.ID)]; ok {
		return domain.Conflict("JOB_RUN_EXISTS", "执行实例已存在")
	}
	for _, node := range nodes {
		if err := node.Validate(); err != nil {
			return err
		}
		if node.JobRunID != job.ID || node.TenantID != job.TenantID {
			return domain.Invalid("NODE_RUN_SCOPE_INVALID", "NodeRun 不属于当前 JobRun")
		}
	}
	if event.Sequence != 1 || event.JobRunID != job.ID || event.TenantID != job.TenantID {
		return domain.Invalid("JOB_EVENT_SEQUENCE_INVALID", "初始 JobEvent 序号必须为 1")
	}
	if err := validateRuntimeEventFields(event); err != nil {
		return err
	}
	s.runtimeJobs[runtimePlanKey(job.TenantID, job.ID)] = job
	for _, node := range nodes {
		s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
	}
	appendRuntimeEventLocked(s, event)
	return nil
}

func (s *Store) JobRun(_ context.Context, tenantID, id string) (domain.JobRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeJobs[runtimePlanKey(tenantID, id)]
	if !ok {
		return value, domain.NotFound("执行实例")
	}
	return value, nil
}
func (s *Store) JobRunByIdempotencyKey(_ context.Context, tenantID, key string) (domain.JobRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.runtimeJobs {
		if value.TenantID == tenantID && value.IdempotencyKey == key {
			return value, nil
		}
	}
	return domain.JobRun{}, domain.NotFound("执行实例")
}
func (s *Store) JobRuns(_ context.Context, tenantID, taskID string) ([]domain.JobRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.JobRun{}
	for _, value := range s.runtimeJobs {
		if value.TenantID == tenantID && (taskID == "" || value.WorkTaskID == taskID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}
func (s *Store) JobRunsPage(_ context.Context, tenantID, projectID, state string, after, limit int) ([]domain.JobRun, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.JobRun{}
	for _, value := range s.runtimeJobs {
		if value.TenantID == tenantID && (projectID == "" || value.ProjectID == projectID) && (state == "" || value.State == state) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return memoryRuntimePage(result, after, limit)
}
func (s *Store) NodeRuns(_ context.Context, tenantID, jobID string) ([]domain.NodeRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.NodeRun{}
	for _, value := range s.runtimeNodes {
		if value.TenantID == tenantID && (jobID == "" || value.JobRunID == jobID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}
func (s *Store) NodeRunsPage(_ context.Context, tenantID, jobID string, after, limit int) ([]domain.NodeRun, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.NodeRun{}
	for _, value := range s.runtimeNodes {
		if value.TenantID == tenantID && value.JobRunID == jobID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return memoryRuntimePage(result, after, limit)
}
func (s *Store) NodeRun(_ context.Context, tenantID, id string) (domain.NodeRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeNodes[runtimeNodeKey(tenantID, id)]
	if !ok {
		return value, domain.NotFound("执行节点")
	}
	return value, nil
}
func (s *Store) CreateContextView(_ context.Context, value domain.ContextView) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimePlanKey(value.TenantID, value.ID)
	if _, ok := s.runtimeContextViews[key]; ok {
		return domain.Conflict("CONTEXT_VIEW_EXISTS", "ContextView 已存在")
	}
	for _, existing := range s.runtimeContextViews {
		if existing.TenantID == value.TenantID && existing.Digest == value.Digest {
			return domain.Conflict("CONTEXT_VIEW_DIGEST_EXISTS", "相同摘要的 ContextView 已存在")
		}
		if existing.TenantID == value.TenantID && existing.NodeRunID == value.NodeRunID && existing.AttemptID == value.AttemptID {
			return domain.Conflict("CONTEXT_VIEW_ATTEMPT_EXISTS", "执行尝试已经绑定 ContextView")
		}
	}
	s.runtimeContextViews[key] = value
	return nil
}

func (s *Store) ContextView(_ context.Context, tenantID, id string) (domain.ContextView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeContextViews[runtimePlanKey(tenantID, id)]
	if !ok {
		return domain.ContextView{}, domain.NotFound("ContextView")
	}
	return value, nil
}

func (s *Store) ContextViews(_ context.Context, tenantID, jobID string) ([]domain.ContextView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ContextView{}
	for _, value := range s.runtimeContextViews {
		if value.TenantID == tenantID && (jobID == "" || value.JobRunID == jobID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (s *Store) CreateAgentInstance(_ context.Context, value domain.AgentInstance) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimePlanKey(value.TenantID, value.ID)
	if _, ok := s.runtimeAgents[key]; ok {
		return domain.Conflict("AGENT_INSTANCE_EXISTS", "AgentInstance 已存在")
	}
	for _, existing := range s.runtimeAgents {
		if existing.TenantID == value.TenantID && existing.ContextViewID == value.ContextViewID {
			return domain.Conflict("AGENT_INSTANCE_CONTEXT_EXISTS", "ContextView 已绑定 AgentInstance")
		}
	}
	if value.ParentAgentInstanceID != "" {
		parentKey := runtimePlanKey(value.TenantID, value.ParentAgentInstanceID)
		parent, ok := s.runtimeAgents[parentKey]
		if !ok {
			return domain.NotFound("父 AgentInstance")
		}
		parentView, parentViewOK := s.runtimeContextViews[runtimePlanKey(value.TenantID, parent.ContextViewID)]
		childView, childViewOK := s.runtimeContextViews[runtimePlanKey(value.TenantID, value.ContextViewID)]
		allocation := 1 + value.RemainingDescendants
		if parent.JobRunID != value.JobRunID || value.Depth != parent.Depth+1 || parent.RemainingDescendants < allocation || value.BudgetMinor > parent.BudgetMinor-parent.UsedCostMinor || !parentViewOK || !childViewOK || !runtimeStringSubset(childView.AllowedTools, parentView.AllowedTools) {
			return domain.Conflict("AGENT_INSTANCE_PARENT_CONFLICT", "父 AgentInstance 的范围、权限、预算或派生额度已经变化")
		}
		parent.RemainingDescendants -= allocation
		parent.Version++
		parent.UpdatedAt = value.CreatedAt
		s.runtimeAgents[parentKey] = parent
	}
	s.runtimeAgents[key] = value
	return nil
}

func runtimeStringSubset(candidate, allowed []string) bool {
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

func (s *Store) AgentInstance(_ context.Context, tenantID, id string) (domain.AgentInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeAgents[runtimePlanKey(tenantID, id)]
	if !ok {
		return domain.AgentInstance{}, domain.NotFound("AgentInstance")
	}
	return value, nil
}

func (s *Store) AgentInstances(_ context.Context, tenantID, jobID string) ([]domain.AgentInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.AgentInstance{}
	for _, value := range s.runtimeAgents {
		if value.TenantID == tenantID && (jobID == "" || value.JobRunID == jobID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (s *Store) SaveAgentInstance(_ context.Context, value domain.AgentInstance, expectedVersion int) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimePlanKey(value.TenantID, value.ID)
	old, ok := s.runtimeAgents[key]
	if !ok {
		return domain.NotFound("AgentInstance")
	}
	if old.Version != expectedVersion {
		return domain.Conflict("AGENT_INSTANCE_VERSION_CONFLICT", "AgentInstance 已被更新，请重新读取")
	}
	s.runtimeAgents[key] = value
	return nil
}

func (s *Store) AppendRuntimeEvent(_ context.Context, event domain.JobEvent) (domain.JobEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.runtimeEvents[event.JobRunID]
	if event.IdempotencyKey != "" {
		for _, existing := range events {
			if existing.IdempotencyKey == event.IdempotencyKey {
				enqueueRuntimeOutboxLocked(s, existing)
				return existing, nil
			}
		}
	}
	if event.Sequence != 0 && event.Sequence != int64(len(events)+1) {
		return event, domain.Conflict("JOB_EVENT_SEQUENCE_CONFLICT", "JobEvent 序号必须连续")
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return event, err
	}
	return appendRuntimeEventLocked(s, event), nil
}
func (s *Store) JobEvents(_ context.Context, tenantID, jobID string, after int64) ([]domain.JobEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if job, ok := s.runtimeJobs[runtimePlanKey(tenantID, jobID)]; !ok || job.TenantID != tenantID {
		return nil, domain.NotFound("执行实例")
	}
	result := []domain.JobEvent{}
	for _, event := range s.runtimeEvents[jobID] {
		if event.Sequence > after {
			result = append(result, event)
		}
	}
	return result, nil
}
func (s *Store) JobEventsPage(_ context.Context, tenantID, jobID string, after int64, limit int) ([]domain.JobEvent, error) {
	if limit < 1 {
		return nil, domain.Invalid("RUNTIME_PAGE_INVALID", "Runtime 分页参数无效")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if job, ok := s.runtimeJobs[runtimePlanKey(tenantID, jobID)]; !ok || job.TenantID != tenantID {
		return nil, domain.NotFound("执行实例")
	}
	result := make([]domain.JobEvent, 0, limit)
	for _, event := range s.runtimeEvents[jobID] {
		if event.Sequence <= after {
			continue
		}
		result = append(result, event)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (s *Store) RuntimeState(_ context.Context, tenantID, jobID, collection string) (domain.RuntimeState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeStates[runtimeStateKey(tenantID, jobID, collection)]
	if !ok {
		return value, domain.NotFound("运行状态")
	}
	return value, nil
}
func (s *Store) CreateCheckpoint(_ context.Context, value domain.Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value.ID == "" || value.TenantID == "" || value.JobRunID == "" || value.Digest == "" {
		return domain.Invalid("CHECKPOINT_INVALID", "检查点缺少执行实例或摘要")
	}
	key := runtimePlanKey(value.TenantID, value.ID)
	if _, ok := s.runtimeCheckpoints[key]; ok {
		return domain.Conflict("CHECKPOINT_EXISTS", "检查点已存在")
	}
	s.runtimeCheckpoints[key] = value
	return nil
}
func (s *Store) Checkpoint(_ context.Context, tenantID, id string) (domain.Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeCheckpoints[runtimePlanKey(tenantID, id)]
	if !ok {
		return domain.Checkpoint{}, domain.NotFound("检查点")
	}
	return value, nil
}
func (s *Store) Checkpoints(_ context.Context, tenantID, jobID string) ([]domain.Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.Checkpoint{}
	for _, value := range s.runtimeCheckpoints {
		if value.TenantID == tenantID && (jobID == "" || value.JobRunID == jobID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}
func (s *Store) CheckpointsPage(_ context.Context, tenantID, jobID string, after, limit int) ([]domain.Checkpoint, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.Checkpoint{}
	for _, value := range s.runtimeCheckpoints {
		if value.TenantID == tenantID && value.JobRunID == jobID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return memoryRuntimePage(result, after, limit)
}

func (s *Store) Effect(_ context.Context, tenantID, id string) (domain.ExternalEffect, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeEffects[runtimeEffectKey(tenantID, id)]
	if !ok {
		return domain.ExternalEffect{}, domain.NotFound("外部操作")
	}
	return value, nil
}

func (s *Store) EffectByIdempotencyKey(_ context.Context, tenantID, key string) (domain.ExternalEffect, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.runtimeEffects {
		if value.TenantID == tenantID && value.IdempotencyKey == key {
			return value, nil
		}
	}
	return domain.ExternalEffect{}, domain.NotFound("外部操作")
}

func (s *Store) EffectByExternalID(_ context.Context, tenantID, providerID, externalID string) (domain.ExternalEffect, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.runtimeEffects {
		if value.TenantID == tenantID && value.ExternalID == externalID && (providerID == "" || value.Kind == providerID || value.SafeSummary["provider_id"] == providerID) {
			return value, nil
		}
	}
	return domain.ExternalEffect{}, domain.NotFound("外部操作")
}
func (s *Store) Effects(_ context.Context, tenantID, jobID string) ([]domain.ExternalEffect, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ExternalEffect{}
	for _, value := range s.runtimeEffects {
		if value.TenantID == tenantID && (jobID == "" || value.JobRunID == jobID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}
func (s *Store) EffectsPage(_ context.Context, tenantID, jobID string, after, limit int) ([]domain.ExternalEffect, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ExternalEffect{}
	for _, value := range s.runtimeEffects {
		if value.TenantID == tenantID && value.JobRunID == jobID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return memoryRuntimePage(result, after, limit)
}

func memoryRuntimePage[T any](values []T, after, limit int) ([]T, bool, error) {
	if after < 0 || limit < 1 {
		return nil, false, domain.Invalid("RUNTIME_PAGE_INVALID", "Runtime 分页参数无效")
	}
	if after >= len(values) {
		return []T{}, false, nil
	}
	end := after + limit
	if end >= len(values) {
		return append([]T(nil), values[after:]...), false, nil
	}
	return append([]T(nil), values[after:end]...), true, nil
}
func (s *Store) ExpireNodeLeases(_ context.Context, tenantID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for attemptKey, attempt := range s.runtimeAttempts {
		if attempt.TenantID != tenantID || attempt.LeaseExpiresAt == nil || attempt.LeaseExpiresAt.After(now) || (attempt.State != domain.RuntimeAttemptPrepared && attempt.State != domain.RuntimeAttemptRunning) {
			continue
		}
		attempt.State = domain.RuntimeAttemptExpired
		attempt.ErrorCode = "DISPATCH_LEASE_EXPIRED"
		attempt.LeaseOwner = ""
		attempt.FenceToken = ""
		attempt.LeaseExpiresAt = nil
		attempt.FinishedAt = &now
		attempt.Version++
		attempt.UpdatedAt = now
		s.runtimeAttempts[attemptKey] = attempt
		for reservationKey, reservation := range s.runtimeReservations {
			if reservation.TenantID == tenantID && reservation.AttemptID == attempt.ID && reservation.State == domain.ReservationHeld {
				reservation.State = domain.ReservationExpired
				reservation.FenceToken = ""
				reservation.ExpiresAt = nil
				reservation.ReleasedAt = &now
				reservation.UpdatedAt = now
				s.runtimeReservations[reservationKey] = reservation
			}
		}
		if agent, ok := s.runtimeAgents[runtimePlanKey(tenantID, attempt.AgentInstanceID)]; ok && agent.State == domain.AgentActive {
			agent.State = domain.AgentRunnable
			agent.Version++
			agent.UpdatedAt = now
			s.runtimeAgents[runtimePlanKey(tenantID, agent.ID)] = agent
		}
		appendRuntimeEventLocked(s, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: attempt.JobRunID, Type: "attempt.expired", ActorType: "runtime", Payload: map[string]any{"attempt_id": attempt.ID, "error_code": attempt.ErrorCode}, OccurredAt: now})
	}
	for key, node := range s.runtimeNodes {
		if node.TenantID == tenantID && node.LeaseExpiresAt != nil && !node.LeaseExpiresAt.After(now) && (node.State == domain.NodeLeased || node.State == domain.NodeRunning) {
			node.State = domain.NodeLeaseExpired
			node.ErrorCode = "DISPATCH_LEASE_EXPIRED"
			node.LeaseOwner = ""
			node.FenceToken = ""
			node.LeaseExpiresAt = nil
			node.Version++
			node.UpdatedAt = now
			s.runtimeNodes[key] = node
		}
	}
	return nil
}
