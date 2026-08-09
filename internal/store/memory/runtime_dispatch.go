package memory

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) NextReadyNode(_ context.Context, tenantID, jobID string) (domain.NodeRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var selected domain.NodeRun
	selectedScore := int64(-1)
	now := time.Now()
	for _, candidate := range s.runtimeNodes {
		if candidate.TenantID != tenantID || (jobID != "" && candidate.JobRunID != jobID) || candidate.State != domain.NodeReady {
			continue
		}
		job, ok := s.runtimeJobs[runtimePlanKey(candidate.TenantID, candidate.JobRunID)]
		if !ok || job.State == domain.JobRunPaused || job.State == domain.JobRunCompleted || job.State == domain.JobRunFailed || job.State == domain.JobRunCancelled || job.State == domain.JobRunRejected {
			continue
		}
		priority := int64(job.Priority)
		score := priority + int64(now.Sub(candidate.UpdatedAt)/time.Minute)
		if selected.ID == "" || score > selectedScore || (score == selectedScore && (candidate.UpdatedAt.Before(selected.UpdatedAt) || (candidate.UpdatedAt.Equal(selected.UpdatedAt) && candidate.ID < selected.ID))) {
			selected = candidate
			selectedScore = score
		}
	}
	if selected.ID == "" {
		return domain.NodeRun{}, domain.NotFound("可调度的执行节点")
	}
	return selected, nil
}

func (s *Store) AgentInstanceForNode(_ context.Context, tenantID, nodeID string) (domain.AgentInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var selected domain.AgentInstance
	for _, candidate := range s.runtimeAgents {
		if candidate.TenantID != tenantID || candidate.NodeRunID != nodeID || candidate.ParentAgentInstanceID != "" {
			continue
		}
		if selected.ID == "" || candidate.CreatedAt.Before(selected.CreatedAt) || (candidate.CreatedAt.Equal(selected.CreatedAt) && candidate.ID < selected.ID) {
			selected = candidate
		}
	}
	if selected.ID == "" {
		return domain.AgentInstance{}, domain.NotFound("节点 AgentInstance")
	}
	return selected, nil
}

func (s *Store) RuntimeAttempt(_ context.Context, tenantID, id string) (domain.RuntimeAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeAttempts[runtimePlanKey(tenantID, id)]
	if !ok {
		return domain.RuntimeAttempt{}, domain.NotFound("RuntimeAttempt")
	}
	return value, nil
}

func (s *Store) RuntimeAttempts(_ context.Context, tenantID, jobID string) ([]domain.RuntimeAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.RuntimeAttempt{}
	for _, value := range s.runtimeAttempts {
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

func (s *Store) PrepareDispatch(_ context.Context, node domain.NodeRun, expectedNodeVersion int, attempt domain.RuntimeAttempt, view domain.ContextView, agent domain.AgentInstance, createAgent bool, expectedAgentVersion int, reservations []domain.ResourceReservation, event domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error) {
	if err := node.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := attempt.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := view.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := agent.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	currentNode, ok := s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)]
	if !ok {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.NotFound("执行节点")
	}
	if currentNode.Version != expectedNodeVersion || currentNode.State != domain.NodeReady {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("NODE_DISPATCH_CONFLICT", "执行节点已经被其他执行者领取")
	}
	if node.FenceToken == "" || attempt.FenceToken == "" || node.FenceToken != attempt.FenceToken {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Invalid("DISPATCH_FENCE_INVALID", "节点与 RuntimeAttempt 必须共享不可猜的围栏令牌")
	}
	if node.Version != expectedNodeVersion+1 || node.State != domain.NodeLeased || node.AttemptCount != currentNode.AttemptCount+1 || node.AttemptCount != attempt.AttemptNo || node.LeaseOwner != attempt.LeaseOwner || !sameTimePointer(node.LeaseExpiresAt, attempt.LeaseExpiresAt) {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Invalid("DISPATCH_PREPARE_INVALID", "待准备的节点与 RuntimeAttempt 租约不一致")
	}
	if !sameDispatchScope(node, attempt, view, agent) || view.AttemptID != attempt.ID || attempt.ContextViewID != view.ID || attempt.AgentInstanceID != agent.ID || agent.ContextViewID != view.ID || agent.State != domain.AgentRunnable {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Invalid("DISPATCH_SCOPE_INVALID", "调度对象不属于同一 JobRun、NodeRun 或 Attempt")
	}
	if _, exists := s.runtimeAttempts[runtimePlanKey(attempt.TenantID, attempt.ID)]; exists {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("RUNTIME_ATTEMPT_EXISTS", "RuntimeAttempt 已存在")
	}
	for _, existing := range s.runtimeAttempts {
		if existing.TenantID == attempt.TenantID && existing.NodeRunID == attempt.NodeRunID && existing.AttemptNo == attempt.AttemptNo {
			return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("RUNTIME_ATTEMPT_NUMBER_EXISTS", "节点执行尝试序号已存在")
		}
	}
	if err := validateNewContextViewLocked(s, view); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	agentKey := runtimePlanKey(agent.TenantID, agent.ID)
	if createAgent {
		if _, exists := s.runtimeAgents[agentKey]; exists || expectedAgentVersion != 0 {
			return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("AGENT_INSTANCE_EXISTS", "AgentInstance 已存在")
		}
	} else {
		currentAgent, exists := s.runtimeAgents[agentKey]
		if !exists {
			return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.NotFound("AgentInstance")
		}
		if currentAgent.Version != expectedAgentVersion || agent.Version != expectedAgentVersion+1 || currentAgent.JobRunID != agent.JobRunID || currentAgent.NodeRunID != agent.NodeRunID || currentAgent.ParentAgentInstanceID != "" || currentAgent.HarnessKind != agent.HarnessKind || currentAgent.ExecutionProfileID != agent.ExecutionProfileID {
			return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("AGENT_INSTANCE_DISPATCH_CONFLICT", "节点 AgentInstance 已被更新或执行配置发生变化")
		}
		if err := currentAgent.Transition(domain.AgentRunnable); err != nil {
			return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
		}
	}
	for _, existing := range s.runtimeAgents {
		if existing.TenantID == agent.TenantID && existing.ContextViewID == agent.ContextViewID && existing.ID != agent.ID {
			return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("AGENT_INSTANCE_CONTEXT_EXISTS", "ContextView 已绑定 AgentInstance")
		}
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := validateResourceReservationsLocked(s, reservations); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}

	s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
	s.runtimeContextViews[runtimePlanKey(view.TenantID, view.ID)] = view
	s.runtimeAgents[agentKey] = agent
	s.runtimeAttempts[runtimePlanKey(attempt.TenantID, attempt.ID)] = attempt
	for _, reservation := range reservations {
		s.runtimeReservations[runtimePlanKey(reservation.TenantID, reservation.ID)] = reservation
	}
	event = appendRuntimeEventLocked(s, event)
	return node, attempt, agent, nil
}

func (s *Store) ActivateDispatch(_ context.Context, node domain.NodeRun, expectedNodeVersion int, attempt domain.RuntimeAttempt, expectedAttemptVersion int, agent domain.AgentInstance, expectedAgentVersion int, event domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error) {
	if err := node.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := attempt.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := agent.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentNode, currentAttempt, currentAgent, err := s.dispatchStateLocked(node.TenantID, node.ID, attempt.ID, agent.ID)
	if err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if currentNode.Version != expectedNodeVersion || currentAttempt.Version != expectedAttemptVersion || currentAgent.Version != expectedAgentVersion {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被其他执行者更新")
	}
	if currentNode.State != domain.NodeLeased || currentAttempt.State != domain.RuntimeAttemptPrepared || currentAgent.State != domain.AgentRunnable || currentNode.LeaseOwner != currentAttempt.LeaseOwner || currentAttempt.LeaseOwner != event.ActorID || currentNode.FenceToken == "" || currentNode.FenceToken != currentAttempt.FenceToken || currentNode.FenceToken != node.FenceToken || currentAttempt.LeaseExpiresAt == nil || !currentAttempt.LeaseExpiresAt.After(attempt.UpdatedAt) {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("DISPATCH_LEASE_STALE", "调度租约无效、已过期或不属于当前执行者")
	}
	if err := currentNode.Transition(node.State); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := currentAttempt.Transition(attempt.State); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := currentAgent.Transition(agent.State); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if !sameDispatchScope(node, attempt, domain.ContextView{TenantID: node.TenantID, JobRunID: node.JobRunID, NodeRunID: node.ID}, agent) || attempt.SessionRef == "" || agent.SessionRef != attempt.SessionRef || node.Version != expectedNodeVersion+1 || attempt.Version != expectedAttemptVersion+1 || agent.Version != expectedAgentVersion+1 {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Invalid("DISPATCH_ACTIVATE_INVALID", "激活后的调度状态不一致")
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
	s.runtimeAttempts[runtimePlanKey(attempt.TenantID, attempt.ID)] = attempt
	s.runtimeAgents[runtimePlanKey(agent.TenantID, agent.ID)] = agent
	appendRuntimeEventLocked(s, event)
	return node, attempt, agent, nil
}

func (s *Store) HeartbeatDispatch(_ context.Context, tenantID, attemptID, owner, fenceToken string, expectedNodeVersion, expectedAttemptVersion int, now time.Time, leaseFor time.Duration) (domain.NodeRun, domain.RuntimeAttempt, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(fenceToken) == "" || leaseFor <= 0 {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.Invalid("DISPATCH_HEARTBEAT_INVALID", "调度心跳需要执行者和正数时长")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.runtimeAttempts[runtimePlanKey(tenantID, attemptID)]
	if !ok {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.NotFound("RuntimeAttempt")
	}
	node, ok := s.runtimeNodes[runtimeNodeKey(tenantID, attempt.NodeRunID)]
	if !ok {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.NotFound("执行节点")
	}
	if node.Version != expectedNodeVersion || attempt.Version != expectedAttemptVersion {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被更新，请重新读取")
	}
	if node.State != domain.NodeRunning || attempt.State != domain.RuntimeAttemptRunning || node.LeaseOwner != owner || attempt.LeaseOwner != owner || node.FenceToken != fenceToken || attempt.FenceToken != fenceToken || node.LeaseExpiresAt == nil || attempt.LeaseExpiresAt == nil || !node.LeaseExpiresAt.After(now) || !attempt.LeaseExpiresAt.After(now) {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.Conflict("DISPATCH_LEASE_STALE", "调度租约无效、已过期或不属于当前执行者")
	}
	expires := now.Add(leaseFor)
	node.LeaseExpiresAt = &expires
	node.Version++
	node.UpdatedAt = now
	attempt.LeaseExpiresAt = &expires
	attempt.Version++
	attempt.UpdatedAt = now
	s.runtimeNodes[runtimeNodeKey(tenantID, node.ID)] = node
	s.runtimeAttempts[runtimePlanKey(tenantID, attempt.ID)] = attempt
	for key, reservation := range s.runtimeReservations {
		if reservation.TenantID == tenantID && reservation.AttemptID == attempt.ID && reservation.State == domain.ReservationHeld && reservation.FenceToken == fenceToken {
			reservation.ExpiresAt = &expires
			reservation.UpdatedAt = now
			s.runtimeReservations[key] = reservation
		}
	}
	return node, attempt, nil
}

func (s *Store) FinalizeDispatch(_ context.Context, node domain.NodeRun, expectedNodeVersion int, attempt domain.RuntimeAttempt, expectedAttemptVersion int, agent domain.AgentInstance, expectedAgentVersion int, fenceToken string, event domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error) {
	if err := node.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := attempt.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := agent.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentNode, currentAttempt, currentAgent, err := s.dispatchStateLocked(node.TenantID, node.ID, attempt.ID, agent.ID)
	if err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if currentAttempt.Terminal() {
		if currentAttempt.State == attempt.State && currentAttempt.ResultDigest == attempt.ResultDigest {
			return currentNode, currentAttempt, currentAgent, nil
		}
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("RUNTIME_ATTEMPT_RESULT_CONFLICT", "RuntimeAttempt 已收到不同的终态结果")
	}
	if currentNode.Version != expectedNodeVersion || currentAttempt.Version != expectedAttemptVersion || currentAgent.Version != expectedAgentVersion {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被其他执行者更新")
	}
	if currentNode.LeaseOwner == "" || currentNode.LeaseOwner != currentAttempt.LeaseOwner || currentAttempt.LeaseOwner != event.ActorID || strings.TrimSpace(fenceToken) == "" || currentNode.FenceToken != fenceToken || currentAttempt.FenceToken != fenceToken || currentNode.LeaseExpiresAt == nil || currentAttempt.LeaseExpiresAt == nil || !currentNode.LeaseExpiresAt.After(event.OccurredAt) || !currentAttempt.LeaseExpiresAt.After(event.OccurredAt) {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Conflict("DISPATCH_LEASE_STALE", "终态结果不属于当前调度租约")
	}
	if err := currentNode.Transition(node.State); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := currentAttempt.Transition(attempt.State); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := currentAgent.Transition(agent.State); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if node.Version != expectedNodeVersion+1 || attempt.Version != expectedAttemptVersion+1 || agent.Version != expectedAgentVersion+1 || !sameDispatchScope(node, attempt, domain.ContextView{TenantID: node.TenantID, JobRunID: node.JobRunID, NodeRunID: node.ID}, agent) {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Invalid("DISPATCH_FINALIZE_INVALID", "终态调度对象的版本或范围不一致")
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
	s.runtimeAttempts[runtimePlanKey(attempt.TenantID, attempt.ID)] = attempt
	s.runtimeAgents[runtimePlanKey(agent.TenantID, agent.ID)] = agent
	for key, reservation := range s.runtimeReservations {
		if reservation.TenantID == attempt.TenantID && reservation.AttemptID == attempt.ID && reservation.State == domain.ReservationHeld {
			reservation.State = domain.ReservationConsumed
			reservation.FenceToken = ""
			reservation.ExpiresAt = nil
			finished := event.OccurredAt
			reservation.ReleasedAt = &finished
			reservation.UpdatedAt = event.OccurredAt
			s.runtimeReservations[key] = reservation
		}
	}
	appendRuntimeEventLocked(s, event)
	return node, attempt, agent, nil
}

func (s *Store) dispatchStateLocked(tenantID, nodeID, attemptID, agentID string) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error) {
	node, ok := s.runtimeNodes[runtimeNodeKey(tenantID, nodeID)]
	if !ok {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.NotFound("执行节点")
	}
	attempt, ok := s.runtimeAttempts[runtimePlanKey(tenantID, attemptID)]
	if !ok {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.NotFound("RuntimeAttempt")
	}
	agent, ok := s.runtimeAgents[runtimePlanKey(tenantID, agentID)]
	if !ok {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.NotFound("AgentInstance")
	}
	return node, attempt, agent, nil
}

func sameDispatchScope(node domain.NodeRun, attempt domain.RuntimeAttempt, view domain.ContextView, agent domain.AgentInstance) bool {
	return node.TenantID == attempt.TenantID && node.TenantID == view.TenantID && node.TenantID == agent.TenantID &&
		node.JobRunID == attempt.JobRunID && node.JobRunID == view.JobRunID && node.JobRunID == agent.JobRunID &&
		node.ID == attempt.NodeRunID && node.ID == view.NodeRunID && node.ID == agent.NodeRunID
}

func sameTimePointer(left, right *time.Time) bool {
	return left != nil && right != nil && left.Equal(*right)
}

func validateNewContextViewLocked(s *Store, view domain.ContextView) error {
	key := runtimePlanKey(view.TenantID, view.ID)
	if _, ok := s.runtimeContextViews[key]; ok {
		return domain.Conflict("CONTEXT_VIEW_EXISTS", "ContextView 已存在")
	}
	for _, existing := range s.runtimeContextViews {
		if existing.TenantID == view.TenantID && existing.Digest == view.Digest {
			return domain.Conflict("CONTEXT_VIEW_DIGEST_EXISTS", "相同摘要的 ContextView 已存在")
		}
		if existing.TenantID == view.TenantID && existing.NodeRunID == view.NodeRunID && existing.AttemptID == view.AttemptID {
			return domain.Conflict("CONTEXT_VIEW_ATTEMPT_EXISTS", "执行尝试已经绑定 ContextView")
		}
	}
	return nil
}

func validateRuntimeEventLocked(s *Store, event domain.JobEvent) error {
	if err := validateRuntimeEventFields(event); err != nil {
		return err
	}
	if job, ok := s.runtimeJobs[runtimePlanKey(event.TenantID, event.JobRunID)]; !ok || job.TenantID != event.TenantID {
		return domain.NotFound("执行实例")
	}
	return nil
}

func validateRuntimeEventFields(event domain.JobEvent) error {
	if event.ID == "" || event.TenantID == "" || event.JobRunID == "" || event.Type == "" || event.ActorType == "" || event.OccurredAt.IsZero() {
		return domain.Invalid("JOB_EVENT_INVALID", "JobEvent 缺少执行实例、类型或执行者")
	}
	return nil
}

func appendRuntimeEventLocked(s *Store, event domain.JobEvent) domain.JobEvent {
	events := s.runtimeEvents[event.JobRunID]
	event.Sequence = int64(len(events) + 1)
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	s.runtimeEvents[event.JobRunID] = append(events, event)
	outboxID := runtimePlanKey(event.TenantID, event.ID)
	if _, exists := s.runtimeOutbox[outboxID]; !exists {
		s.runtimeOutbox[outboxID] = domain.RuntimeOutboxMessage{
			ID: event.ID, TenantID: event.TenantID, EventID: event.ID,
			SchemaVersion: domain.RuntimeEventSchema, Topic: "runtime.job_event",
			AggregateID:   event.JobRunID,
			Payload:       map[string]any{"event_id": event.ID, "job_run_id": event.JobRunID, "sequence": event.Sequence, "type": event.Type, "payload": event.Payload},
			NextAttemptAt: event.OccurredAt, CreatedAt: event.OccurredAt,
		}
	}
	return event
}
