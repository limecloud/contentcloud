package memory

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func validateCommandEvent(s *Store, event domain.JobEvent) error {
	if event.ID == "" || event.TenantID == "" || event.JobRunID == "" || event.Type == "" || event.ActorType == "" || event.OccurredAt.IsZero() {
		return domain.Invalid("JOB_EVENT_INVALID", "JobEvent 缺少执行实例、类型或执行者")
	}
	if job, ok := s.runtimeJobs[runtimePlanKey(event.TenantID, event.JobRunID)]; !ok || job.TenantID != event.TenantID {
		return domain.NotFound("执行实例")
	}
	return nil
}

func (s *Store) AppendFencedRuntimeEvent(_ context.Context, tenantID, attemptID, owner, fenceToken string, now time.Time, event domain.JobEvent) (domain.JobEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.runtimeAttempts[runtimePlanKey(tenantID, attemptID)]
	if !ok {
		return event, domain.NotFound("RuntimeAttempt")
	}
	if attempt.State != domain.RuntimeAttemptRunning || attempt.LeaseOwner != strings.TrimSpace(owner) || attempt.FenceToken == "" || attempt.FenceToken != fenceToken || attempt.LeaseExpiresAt == nil || !attempt.LeaseExpiresAt.After(now) {
		return event, domain.Conflict("DISPATCH_FENCE_STALE", "Harness 事件的执行围栏无效或已过期")
	}
	event.TenantID, event.JobRunID, event.ActorType, event.ActorID = tenantID, attempt.JobRunID, "harness", attempt.HarnessKind
	if err := validateCommandEvent(s, event); err != nil {
		return event, err
	}
	for _, existing := range s.runtimeEvents[event.JobRunID] {
		if event.IdempotencyKey != "" && existing.IdempotencyKey == event.IdempotencyKey {
			enqueueRuntimeOutboxLocked(s, existing)
			return existing, nil
		}
	}
	return appendRuntimeEventLocked(s, event), nil
}

func (s *Store) ClaimReadyNodeCommand(_ context.Context, tenantID, jobID, owner string, now time.Time, leaseFor time.Duration, event domain.JobEvent) (domain.NodeRun, error) {
	if strings.TrimSpace(owner) == "" || leaseFor <= 0 {
		return domain.NodeRun{}, domain.Invalid("NODE_LEASE_INVALID", "节点租约需要执行者和正数时长")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var selected domain.NodeRun
	selectedScore := int64(-1)
	clock := now
	for _, candidate := range s.runtimeNodes {
		if candidate.TenantID != tenantID || candidate.State != domain.NodeReady || (jobID != "" && candidate.JobRunID != jobID) {
			continue
		}
		priority := int64(0)
		if job, ok := s.runtimeJobs[runtimePlanKey(candidate.TenantID, candidate.JobRunID)]; ok {
			priority = int64(job.Priority)
		}
		score := priority + int64(clock.Sub(candidate.UpdatedAt)/time.Minute)
		if selected.ID == "" || score > selectedScore || (score == selectedScore && (candidate.UpdatedAt.Before(selected.UpdatedAt) || (candidate.UpdatedAt.Equal(selected.UpdatedAt) && candidate.ID < selected.ID))) {
			selected = candidate
			selectedScore = score
		}
	}
	if selected.ID == "" {
		return selected, domain.NotFound("可领取的执行节点")
	}
	if err := selected.Transition(domain.NodeLeased); err != nil {
		return selected, err
	}
	expires := now.Add(leaseFor)
	fenceToken, _, err := domain.NewOpaqueToken("rtf_", 24)
	if err != nil {
		return domain.NodeRun{}, err
	}
	selected.State = domain.NodeLeased
	selected.AttemptCount++
	selected.LeaseOwner = strings.TrimSpace(owner)
	selected.FenceToken = fenceToken
	selected.LeaseExpiresAt = &expires
	selected.Version++
	selected.UpdatedAt = now
	event.TenantID, event.JobRunID, event.NodeKey, event.ActorID = tenantID, selected.JobRunID, selected.NodeKey, strings.TrimSpace(owner)
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	event.Payload["attempt_count"], event.Payload["lease_expires_at"] = selected.AttemptCount, selected.LeaseExpiresAt
	if err := validateCommandEvent(s, event); err != nil {
		return domain.NodeRun{}, err
	}
	s.runtimeNodes[runtimeNodeKey(tenantID, selected.ID)] = selected
	appendRuntimeEventLocked(s, event)
	return selected, nil
}

func (s *Store) HeartbeatNodeCommand(_ context.Context, tenantID, nodeID, owner string, expectedVersion int, now time.Time, leaseFor time.Duration, event domain.JobEvent) (domain.NodeRun, error) {
	if strings.TrimSpace(owner) == "" || leaseFor <= 0 {
		return domain.NodeRun{}, domain.Invalid("NODE_HEARTBEAT_INVALID", "节点心跳需要执行者和正数时长")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	node, ok := s.runtimeNodes[runtimeNodeKey(tenantID, nodeID)]
	if !ok {
		return node, domain.NotFound("执行节点")
	}
	if node.Version != expectedVersion || node.LeaseOwner != strings.TrimSpace(owner) || node.LeaseExpiresAt == nil || !node.LeaseExpiresAt.After(now) || (node.State != domain.NodeLeased && node.State != domain.NodeRunning) {
		return node, domain.Conflict("NODE_LEASE_STALE", "节点租约无效、已过期或不属于当前执行者")
	}
	if node.State == domain.NodeLeased {
		if err := node.Transition(domain.NodeRunning); err != nil {
			return node, err
		}
		node.State = domain.NodeRunning
	}
	expires := now.Add(leaseFor)
	node.LeaseExpiresAt, node.Version, node.UpdatedAt = &expires, node.Version+1, now
	event.TenantID, event.JobRunID, event.NodeKey, event.ActorID = tenantID, node.JobRunID, node.NodeKey, strings.TrimSpace(owner)
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	event.Payload["state"], event.Payload["lease_expires_at"] = node.State, node.LeaseExpiresAt
	if err := validateCommandEvent(s, event); err != nil {
		return domain.NodeRun{}, err
	}
	s.runtimeNodes[runtimeNodeKey(tenantID, node.ID)] = node
	appendRuntimeEventLocked(s, event)
	return node, nil
}

func (s *Store) ApplyJobTransition(_ context.Context, next domain.JobRun, expectedVersion int, event domain.JobEvent) (domain.JobRun, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimePlanKey(next.TenantID, next.ID)
	current, ok := s.runtimeJobs[key]
	if !ok {
		return next, domain.NotFound("执行实例")
	}
	if current.Version != expectedVersion || next.Version != expectedVersion+1 {
		return next, domain.Conflict("JOB_RUN_VERSION_CONFLICT", "执行实例已被更新，请重新读取")
	}
	if err := current.Transition(next.State); err != nil {
		return next, err
	}
	event.TenantID, event.JobRunID = next.TenantID, next.ID
	if err := validateCommandEvent(s, event); err != nil {
		return next, err
	}
	s.runtimeJobs[key] = next
	appendRuntimeEventLocked(s, event)
	return next, nil
}

func (s *Store) ApplyGraphPatchCommand(_ context.Context, next domain.JobRun, expectedVersion int, plan domain.JobPlanRevision, addedNodes []domain.NodeRun, cancelNodeKeys []string, event domain.JobEvent) (domain.JobRun, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	if err := plan.Validate(); err != nil {
		return next, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	jobKey := runtimePlanKey(next.TenantID, next.ID)
	current, ok := s.runtimeJobs[jobKey]
	if !ok {
		return next, domain.NotFound("执行实例")
	}
	if current.Version != expectedVersion || next.Version != expectedVersion+1 || current.PlanRevisionID != plan.BaseRevisionID || next.PlanRevisionID != plan.ID || next.PlanDigest != plan.Digest {
		return next, domain.Conflict("GRAPH_VERSION_CONFLICT", "执行图版本已经变化，请重新读取后再提交")
	}
	planKey := runtimePlanKey(plan.TenantID, plan.ID)
	if _, exists := s.runtimePlans[planKey]; exists {
		return next, domain.Conflict("JOB_PLAN_EXISTS", "执行计划版本已存在")
	}
	for _, existing := range s.runtimePlans {
		if existing.TenantID == plan.TenantID && (existing.Digest == plan.Digest || (plan.PatchKey != "" && existing.PatchKey == plan.PatchKey)) {
			return next, domain.Conflict("GRAPH_PATCH_EXISTS", "执行图变更已存在")
		}
	}
	for _, node := range addedNodes {
		if err := node.Validate(); err != nil {
			return next, err
		}
		if node.TenantID != next.TenantID || node.JobRunID != next.ID {
			return next, domain.Invalid("NODE_RUN_SCOPE_INVALID", "GraphPatch 新节点不属于当前执行实例")
		}
		if _, exists := s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)]; exists {
			return next, domain.Conflict("NODE_RUN_EXISTS", "GraphPatch 新节点已经存在")
		}
	}
	for _, key := range cancelNodeKeys {
		found := false
		for _, node := range s.runtimeNodes {
			if node.TenantID != next.TenantID || node.JobRunID != next.ID || node.NodeKey != key {
				continue
			}
			found = true
			if node.State != domain.NodePending && node.State != domain.NodeReady && node.State != domain.NodeWaitingResource {
				return next, domain.Conflict("GRAPH_PATCH_CANCEL_CONFLICT", "GraphPatch 只能取消尚未执行的节点")
			}
		}
		if !found {
			return next, domain.NotFound("GraphPatch 待取消节点")
		}
	}
	event.TenantID, event.JobRunID = next.TenantID, next.ID
	if err := validateCommandEvent(s, event); err != nil {
		return next, err
	}
	s.runtimePlans[planKey] = plan
	for _, node := range addedNodes {
		s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
	}
	for nodeKey, node := range s.runtimeNodes {
		if node.TenantID != next.TenantID || node.JobRunID != next.ID {
			continue
		}
		for _, cancelKey := range cancelNodeKeys {
			if node.NodeKey == cancelKey {
				node.State = domain.NodeCancelled
				node.Version++
				node.UpdatedAt = event.OccurredAt
				s.runtimeNodes[nodeKey] = node
			}
		}
	}
	s.runtimeJobs[jobKey] = next
	appendRuntimeEventLocked(s, event)
	return next, nil
}

func (s *Store) ApplyNodeTransition(_ context.Context, next domain.NodeRun, expectedVersion int, event domain.JobEvent) (domain.NodeRun, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeNodeKey(next.TenantID, next.ID)
	current, ok := s.runtimeNodes[key]
	if !ok {
		return next, domain.NotFound("执行节点")
	}
	if current.Version != expectedVersion || next.Version != expectedVersion+1 {
		return next, domain.Conflict("NODE_RUN_VERSION_CONFLICT", "执行节点已被更新，请重新读取")
	}
	if err := current.Transition(next.State); err != nil {
		return next, err
	}
	if current.JobRunID != next.JobRunID {
		return next, domain.Invalid("NODE_RUN_SCOPE_INVALID", "执行节点不属于当前执行实例")
	}
	event.TenantID, event.JobRunID, event.NodeKey = next.TenantID, next.JobRunID, next.NodeKey
	if err := validateCommandEvent(s, event); err != nil {
		return next, err
	}
	s.runtimeNodes[key] = next
	appendRuntimeEventLocked(s, event)
	return next, nil
}

func cloneStateValues(values map[string]any) map[string]any {
	clone := map[string]any{}
	for key, value := range values {
		if list, ok := value.([]any); ok {
			clone[key] = append([]any{}, list...)
			continue
		}
		clone[key] = value
	}
	return clone
}

func (s *Store) ApplyStateMutation(_ context.Context, tenantID, jobID string, mutation domain.StateMutation, event domain.JobEvent) (domain.RuntimeState, error) {
	if strings.TrimSpace(mutation.Collection) == "" || strings.TrimSpace(mutation.IdempotencyKey) == "" {
		return domain.RuntimeState{}, domain.Invalid("RUNTIME_STATE_MUTATION_INVALID", "状态变更需要集合名和幂等键")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeStateKey(tenantID, jobID, mutation.Collection)
	mutationKey := key + ":" + strings.TrimSpace(mutation.IdempotencyKey)
	if _, exists := s.runtimeStateMutations[mutationKey]; exists {
		return s.runtimeStates[key], nil
	}
	state, exists := s.runtimeStates[key]
	if !exists {
		if mutation.ExpectedRevision != 0 {
			return state, domain.Conflict("RUNTIME_STATE_CAS_CONFLICT", "运行状态版本不匹配")
		}
		state = domain.RuntimeState{ID: domain.NewID(), TenantID: tenantID, JobRunID: jobID, Collection: mutation.Collection, SchemaVersion: domain.RuntimeStateSchema, Values: map[string]any{}, UpdatedAt: event.OccurredAt}
	}
	if state.Revision != mutation.ExpectedRevision {
		return state, domain.Conflict("RUNTIME_STATE_CAS_CONFLICT", "运行状态已经更新，请重新读取")
	}
	state.Values = cloneStateValues(state.Values)
	for key, value := range mutation.Set {
		state.Values[key] = value
	}
	for key, values := range mutation.Append {
		current, _ := state.Values[key].([]any)
		state.Values[key] = append(current, values...)
	}
	state.Revision++
	state.UpdatedAt = event.OccurredAt
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	event.Payload["revision"] = state.Revision
	event.TenantID, event.JobRunID = tenantID, jobID
	if err := validateCommandEvent(s, event); err != nil {
		return state, err
	}
	s.runtimeStates[key] = state
	s.runtimeStateMutations[mutationKey] = "applied"
	appendRuntimeEventLocked(s, event)
	return state, nil
}

func (s *Store) RegisterEffectCommand(_ context.Context, effect domain.ExternalEffect, event domain.JobEvent) (domain.ExternalEffect, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runtimeEffects {
		if existing.TenantID == effect.TenantID && existing.IdempotencyKey == effect.IdempotencyKey {
			return existing, nil
		}
	}
	if effect.ID == "" || effect.TenantID == "" || effect.JobRunID == "" || effect.Kind == "" || effect.IdempotencyKey == "" || effect.RequestDigest == "" || effect.Version < 1 || effect.CreatedAt.IsZero() || effect.UpdatedAt.IsZero() {
		return effect, domain.Invalid("EFFECT_INVALID", "外部操作缺少执行引用、幂等键或请求摘要")
	}
	event.TenantID, event.JobRunID, event.NodeKey = effect.TenantID, effect.JobRunID, ""
	if err := validateCommandEvent(s, event); err != nil {
		return effect, err
	}
	s.runtimeEffects[runtimeEffectKey(effect.TenantID, effect.ID)] = effect
	appendRuntimeEventLocked(s, event)
	return effect, nil
}

func (s *Store) RegisterFencedEffectCommand(_ context.Context, effect domain.ExternalEffect, fenceToken string, now time.Time, event domain.JobEvent) (domain.ExternalEffect, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if effect.ID == "" || effect.TenantID == "" || effect.JobRunID == "" || effect.AttemptID == "" || effect.Kind == "" || effect.IdempotencyKey == "" || effect.RequestDigest == "" || effect.Version < 1 || effect.CreatedAt.IsZero() || effect.UpdatedAt.IsZero() {
		return effect, domain.Invalid("EFFECT_INVALID", "外部操作缺少 Attempt、幂等键或请求摘要")
	}
	if err := validateAttemptFenceLocked(s, effect.TenantID, effect.AttemptID, fenceToken, now); err != nil {
		return effect, err
	}
	for _, existing := range s.runtimeEffects {
		if existing.TenantID != effect.TenantID || existing.IdempotencyKey != effect.IdempotencyKey {
			continue
		}
		if existing.AttemptID != effect.AttemptID || existing.RequestDigest != effect.RequestDigest {
			return effect, domain.Conflict("EFFECT_IDEMPOTENCY_MISMATCH", "Effect 幂等键已用于不同 Attempt 或请求摘要")
		}
		return existing, nil
	}
	event.TenantID, event.JobRunID, event.NodeKey = effect.TenantID, effect.JobRunID, ""
	if err := validateCommandEvent(s, event); err != nil {
		return effect, err
	}
	s.runtimeEffects[runtimeEffectKey(effect.TenantID, effect.ID)] = effect
	appendRuntimeEventLocked(s, event)
	return effect, nil
}

func (s *Store) ApplyEffectTransition(_ context.Context, next domain.ExternalEffect, expectedVersion int, event domain.JobEvent) (domain.ExternalEffect, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeEffectKey(next.TenantID, next.ID)
	current, ok := s.runtimeEffects[key]
	if !ok {
		return next, domain.NotFound("外部操作")
	}
	if current.Version != expectedVersion || next.Version != expectedVersion+1 {
		return next, domain.Conflict("EFFECT_VERSION_CONFLICT", "外部操作已被更新，请重新读取")
	}
	if err := current.Transition(next.State); err != nil {
		return next, err
	}
	event.TenantID, event.JobRunID = next.TenantID, next.JobRunID
	if err := validateCommandEvent(s, event); err != nil {
		return next, err
	}
	s.runtimeEffects[key] = next
	appendRuntimeEventLocked(s, event)
	return next, nil
}

func (s *Store) RuntimeOutboxMessages(_ context.Context, tenantID, subscriber string, now time.Time, limit int) ([]domain.RuntimeOutboxMessage, error) {
	subscriber = strings.TrimSpace(subscriber)
	if subscriber == "" {
		return nil, domain.Invalid("OUTBOX_SUBSCRIBER_REQUIRED", "outbox 查询需要订阅者")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	result := []domain.RuntimeOutboxMessage{}
	for _, receipt := range s.runtimeOutboxReceipts {
		if receipt.TenantID != tenantID || receipt.Subscriber != subscriber || receipt.DeliveredAt != nil || receipt.NextAttemptAt.After(now) || (receipt.LockedUntil != nil && receipt.LockedUntil.After(now)) {
			continue
		}
		message, ok := s.runtimeOutbox[runtimePlanKey(tenantID, receipt.MessageID)]
		if ok {
			result = append(result, joinRuntimeOutboxReceipt(message, receipt))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].NextAttemptAt.Equal(result[j].NextAttemptAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].NextAttemptAt.Before(result[j].NextAttemptAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *Store) ClaimRuntimeOutbox(_ context.Context, tenantID, subscriber, worker string, now time.Time, leaseFor time.Duration, limit int) ([]domain.RuntimeOutboxMessage, error) {
	subscriber, worker = strings.TrimSpace(subscriber), strings.TrimSpace(worker)
	if subscriber == "" || worker == "" || leaseFor <= 0 {
		return nil, domain.Invalid("OUTBOX_CLAIM_INVALID", "outbox 认领需要订阅者、工作器和正数租约")
	}
	if limit <= 0 {
		limit = 100
	}
	lockedUntil := now.Add(leaseFor)
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []domain.RuntimeOutboxMessage{}
	for _, receipt := range s.runtimeOutboxReceipts {
		if receipt.TenantID != tenantID || receipt.Subscriber != subscriber || receipt.DeliveredAt != nil || receipt.NextAttemptAt.After(now) || (receipt.LockedUntil != nil && receipt.LockedUntil.After(now)) {
			continue
		}
		message, ok := s.runtimeOutbox[runtimePlanKey(tenantID, receipt.MessageID)]
		if ok {
			result = append(result, joinRuntimeOutboxReceipt(message, receipt))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].NextAttemptAt.Equal(result[j].NextAttemptAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].NextAttemptAt.Before(result[j].NextAttemptAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	for index := range result {
		key := runtimeOutboxReceiptKey(tenantID, result[index].ID, subscriber)
		receipt := s.runtimeOutboxReceipts[key]
		receipt.LockedBy, receipt.LockedUntil, receipt.Attempts = worker, &lockedUntil, receipt.Attempts+1
		s.runtimeOutboxReceipts[key] = receipt
		result[index] = joinRuntimeOutboxReceipt(s.runtimeOutbox[runtimePlanKey(tenantID, receipt.MessageID)], receipt)
	}
	return result, nil
}

func (s *Store) AckRuntimeOutbox(_ context.Context, tenantID, messageID, subscriber, worker string, deliveredAt time.Time) error {
	subscriber, worker = strings.TrimSpace(subscriber), strings.TrimSpace(worker)
	if subscriber == "" || worker == "" || deliveredAt.IsZero() {
		return domain.Invalid("OUTBOX_ACK_INVALID", "outbox 确认需要订阅者、工作器和确认时间")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeOutboxReceiptKey(tenantID, messageID, subscriber)
	receipt, ok := s.runtimeOutboxReceipts[key]
	if !ok || receipt.DeliveredAt != nil || receipt.LockedBy != worker || receipt.LockedUntil == nil || !receipt.LockedUntil.After(deliveredAt) {
		return domain.Conflict("OUTBOX_LEASE_STALE", "outbox 消息不属于当前消费者或租约已过期")
	}
	receipt.DeliveredAt = &deliveredAt
	receipt.LockedBy, receipt.LockedUntil, receipt.LastError = "", nil, ""
	s.runtimeOutboxReceipts[key] = receipt
	return nil
}

func (s *Store) RetryRuntimeOutbox(_ context.Context, tenantID, messageID, subscriber, worker string, now, nextAttemptAt time.Time, lastError string) error {
	subscriber, worker = strings.TrimSpace(subscriber), strings.TrimSpace(worker)
	if subscriber == "" || worker == "" || now.IsZero() || nextAttemptAt.IsZero() || strings.TrimSpace(lastError) == "" {
		return domain.Invalid("OUTBOX_RETRY_INVALID", "outbox 重试需要订阅者、工作器、时间和错误原因")
	}
	if nextAttemptAt.Before(now) {
		return domain.Invalid("OUTBOX_RETRY_INVALID", "outbox 下次尝试时间不能早于当前时间")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeOutboxReceiptKey(tenantID, messageID, subscriber)
	receipt, ok := s.runtimeOutboxReceipts[key]
	if !ok || receipt.DeliveredAt != nil || receipt.LockedBy != worker || receipt.LockedUntil == nil || !receipt.LockedUntil.After(now) {
		return domain.Conflict("OUTBOX_LEASE_STALE", "outbox 消息不属于当前消费者或租约已过期")
	}
	receipt.NextAttemptAt, receipt.LockedBy, receipt.LockedUntil, receipt.LastError = nextAttemptAt, "", nil, strings.TrimSpace(lastError)
	s.runtimeOutboxReceipts[key] = receipt
	return nil
}

func joinRuntimeOutboxReceipt(message domain.RuntimeOutboxMessage, receipt runtimeOutboxReceipt) domain.RuntimeOutboxMessage {
	message.Subscriber = receipt.Subscriber
	message.Attempts = receipt.Attempts
	message.NextAttemptAt = receipt.NextAttemptAt
	message.LockedBy = receipt.LockedBy
	message.LockedUntil = receipt.LockedUntil
	message.DeliveredAt = receipt.DeliveredAt
	message.LastError = receipt.LastError
	return message
}
