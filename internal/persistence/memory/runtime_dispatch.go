package memory

import (
	"context"
	"sort"
	"strings"
	"time"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

func validateAttemptFenceLocked(s *Store, tenantID, attemptID, fenceToken string, now time.Time) error {
	attempt, ok := s.runtimeAttempts[runtimePlanKey(tenantID, attemptID)]
	if !ok {
		return fault.NotFound("RuntimeAttempt")
	}
	if attempt.State != contentruntime.RuntimeAttemptRunning || strings.TrimSpace(fenceToken) == "" || attempt.FenceToken != fenceToken || attempt.LeaseExpiresAt == nil || !attempt.LeaseExpiresAt.After(now) {
		return fault.Conflict("MCP_GATEWAY_FENCE_STALE", "MCP 调用的 Attempt fence 或租约已失效")
	}
	return nil
}

func (s *Store) NextReadyNode(_ context.Context, tenantID, jobID string, allowedProjectIDs []string) (contentruntime.NodeRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var selected contentruntime.NodeRun
	selectedScore := int64(-1)
	now := time.Now()
	for _, candidate := range s.runtimeNodes {
		if candidate.TenantID != tenantID || (jobID != "" && candidate.JobRunID != jobID) || candidate.State != contentruntime.NodeReady {
			continue
		}
		job, ok := s.runtimeJobs[runtimePlanKey(candidate.TenantID, candidate.JobRunID)]
		if !ok || job.State == contentruntime.JobRunPaused || job.State == contentruntime.JobRunCompleted || job.State == contentruntime.JobRunFailed || job.State == contentruntime.JobRunCancelled || job.State == contentruntime.JobRunRejected {
			continue
		}
		if allowedProjectIDs != nil && !containsString(allowedProjectIDs, job.ProjectID) {
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
		return contentruntime.NodeRun{}, fault.NotFound("可调度的执行节点")
	}
	return selected, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func (s *Store) AgentInstanceForNode(_ context.Context, tenantID, nodeID string) (contentruntime.AgentInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var selected contentruntime.AgentInstance
	for _, candidate := range s.runtimeAgents {
		if candidate.TenantID != tenantID || candidate.NodeRunID != nodeID || candidate.ParentAgentInstanceID != "" {
			continue
		}
		if selected.ID == "" || candidate.CreatedAt.Before(selected.CreatedAt) || (candidate.CreatedAt.Equal(selected.CreatedAt) && candidate.ID < selected.ID) {
			selected = candidate
		}
	}
	if selected.ID == "" {
		return contentruntime.AgentInstance{}, fault.NotFound("节点 AgentInstance")
	}
	return selected, nil
}

func (s *Store) RuntimeAttempt(_ context.Context, tenantID, id string) (contentruntime.RuntimeAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeAttempts[runtimePlanKey(tenantID, id)]
	if !ok {
		return contentruntime.RuntimeAttempt{}, fault.NotFound("RuntimeAttempt")
	}
	return value, nil
}

func (s *Store) RuntimeAttemptByGatewayTokenHash(_ context.Context, tokenHash string) (contentruntime.RuntimeAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, attempt := range s.runtimeAttempts {
		if attempt.GatewayTokenHash == tokenHash {
			return attempt, nil
		}
	}
	return contentruntime.RuntimeAttempt{}, fault.NotFound("Runtime Gateway 凭据")
}

func (s *Store) RuntimeAttempts(_ context.Context, tenantID, jobID string) ([]contentruntime.RuntimeAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []contentruntime.RuntimeAttempt{}
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

func (s *Store) PrepareDispatch(_ context.Context, node contentruntime.NodeRun, expectedNodeVersion int, attempt contentruntime.RuntimeAttempt, view contentruntime.ContextView, agent contentruntime.AgentInstance, createAgent bool, expectedAgentVersion int, reservations []contentruntime.ResourceReservation, event contentruntime.JobEvent) (contentruntime.NodeRun, contentruntime.RuntimeAttempt, contentruntime.AgentInstance, error) {
	if err := node.Validate(); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := attempt.Validate(); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := view.Validate(); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := agent.Validate(); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	currentNode, ok := s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)]
	if !ok {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.NotFound("执行节点")
	}
	if currentNode.Version != expectedNodeVersion || currentNode.State != contentruntime.NodeReady {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("NODE_DISPATCH_CONFLICT", "执行节点已经被其他执行者领取")
	}
	if node.FenceToken == "" || attempt.FenceToken == "" || node.FenceToken != attempt.FenceToken {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Invalid("DISPATCH_FENCE_INVALID", "节点与 RuntimeAttempt 必须共享不可猜的围栏令牌")
	}
	if len(attempt.GatewayTokenHash) != 64 || attempt.GatewayExpiresAt == nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Invalid("DISPATCH_GATEWAY_CREDENTIAL_INVALID", "RuntimeAttempt 必须持有哈希化的短期 Gateway 凭据")
	}
	if node.Version != expectedNodeVersion+1 || node.State != contentruntime.NodeLeased || node.AttemptCount != currentNode.AttemptCount+1 || node.AttemptCount != attempt.AttemptNo || node.LeaseOwner != attempt.LeaseOwner || !sameTimePointer(node.LeaseExpiresAt, attempt.LeaseExpiresAt) {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Invalid("DISPATCH_PREPARE_INVALID", "待准备的节点与 RuntimeAttempt 租约不一致")
	}
	if !sameDispatchScope(node, attempt, view, agent) || view.AttemptID != attempt.ID || attempt.ContextViewID != view.ID || attempt.AgentInstanceID != agent.ID || agent.ContextViewID != view.ID || agent.State != contentruntime.AgentRunnable {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Invalid("DISPATCH_SCOPE_INVALID", "调度对象不属于同一 JobRun、NodeRun 或 Attempt")
	}
	if _, exists := s.runtimeAttempts[runtimePlanKey(attempt.TenantID, attempt.ID)]; exists {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("RUNTIME_ATTEMPT_EXISTS", "RuntimeAttempt 已存在")
	}
	for _, existing := range s.runtimeAttempts {
		if existing.TenantID == attempt.TenantID && existing.NodeRunID == attempt.NodeRunID && existing.AttemptNo == attempt.AttemptNo {
			return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("RUNTIME_ATTEMPT_NUMBER_EXISTS", "节点执行尝试序号已存在")
		}
	}
	if err := validateNewContextViewLocked(s, view); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	agentKey := runtimePlanKey(agent.TenantID, agent.ID)
	if createAgent {
		if _, exists := s.runtimeAgents[agentKey]; exists || expectedAgentVersion != 0 {
			return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("AGENT_INSTANCE_EXISTS", "AgentInstance 已存在")
		}
	} else {
		currentAgent, exists := s.runtimeAgents[agentKey]
		if !exists {
			return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.NotFound("AgentInstance")
		}
		if currentAgent.Version != expectedAgentVersion || agent.Version != expectedAgentVersion+1 || currentAgent.JobRunID != agent.JobRunID || currentAgent.NodeRunID != agent.NodeRunID || currentAgent.ParentAgentInstanceID != "" || currentAgent.HarnessKind != agent.HarnessKind || currentAgent.ExecutionProfileID != agent.ExecutionProfileID {
			return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("AGENT_INSTANCE_DISPATCH_CONFLICT", "节点 AgentInstance 已被更新或执行配置发生变化")
		}
		if err := currentAgent.Transition(contentruntime.AgentRunnable); err != nil {
			return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
		}
	}
	for _, existing := range s.runtimeAgents {
		if existing.TenantID == agent.TenantID && existing.ContextViewID == agent.ContextViewID && existing.ID != agent.ID {
			return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("AGENT_INSTANCE_CONTEXT_EXISTS", "ContextView 已绑定 AgentInstance")
		}
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := validateResourceReservationsLocked(s, reservations); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
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

func (s *Store) ActivateDispatch(_ context.Context, node contentruntime.NodeRun, expectedNodeVersion int, attempt contentruntime.RuntimeAttempt, expectedAttemptVersion int, agent contentruntime.AgentInstance, expectedAgentVersion int, event contentruntime.JobEvent) (contentruntime.NodeRun, contentruntime.RuntimeAttempt, contentruntime.AgentInstance, error) {
	if err := node.Validate(); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := attempt.Validate(); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := agent.Validate(); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentNode, currentAttempt, currentAgent, err := s.dispatchStateLocked(node.TenantID, node.ID, attempt.ID, agent.ID)
	if err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if currentNode.Version != expectedNodeVersion || currentAttempt.Version != expectedAttemptVersion || currentAgent.Version != expectedAgentVersion {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被其他执行者更新")
	}
	if currentNode.State != contentruntime.NodeLeased || currentAttempt.State != contentruntime.RuntimeAttemptPrepared || currentAgent.State != contentruntime.AgentRunnable || currentNode.LeaseOwner != currentAttempt.LeaseOwner || currentAttempt.LeaseOwner != event.ActorID || currentNode.FenceToken == "" || currentNode.FenceToken != currentAttempt.FenceToken || currentNode.FenceToken != node.FenceToken || currentAttempt.LeaseExpiresAt == nil || !currentAttempt.LeaseExpiresAt.After(attempt.UpdatedAt) {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("DISPATCH_LEASE_STALE", "调度租约无效、已过期或不属于当前执行者")
	}
	if err := currentNode.Transition(node.State); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := currentAttempt.Transition(attempt.State); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := currentAgent.Transition(agent.State); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if !sameDispatchScope(node, attempt, contentruntime.ContextView{TenantID: node.TenantID, JobRunID: node.JobRunID, NodeRunID: node.ID}, agent) || attempt.SessionRef == "" || agent.SessionRef != attempt.SessionRef || node.Version != expectedNodeVersion+1 || attempt.Version != expectedAttemptVersion+1 || agent.Version != expectedAgentVersion+1 {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Invalid("DISPATCH_ACTIVATE_INVALID", "激活后的调度状态不一致")
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
	s.runtimeAttempts[runtimePlanKey(attempt.TenantID, attempt.ID)] = attempt
	s.runtimeAgents[runtimePlanKey(agent.TenantID, agent.ID)] = agent
	appendRuntimeEventLocked(s, event)
	return node, attempt, agent, nil
}

func (s *Store) HeartbeatDispatch(_ context.Context, tenantID, attemptID, owner, fenceToken string, expectedNodeVersion, expectedAttemptVersion int, now time.Time, leaseFor time.Duration) (contentruntime.NodeRun, contentruntime.RuntimeAttempt, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(fenceToken) == "" || leaseFor <= 0 {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, fault.Invalid("DISPATCH_HEARTBEAT_INVALID", "调度心跳需要执行者和正数时长")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.runtimeAttempts[runtimePlanKey(tenantID, attemptID)]
	if !ok {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, fault.NotFound("RuntimeAttempt")
	}
	node, ok := s.runtimeNodes[runtimeNodeKey(tenantID, attempt.NodeRunID)]
	if !ok {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, fault.NotFound("执行节点")
	}
	if node.Version != expectedNodeVersion || attempt.Version != expectedAttemptVersion {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, fault.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被更新，请重新读取")
	}
	if node.State != contentruntime.NodeRunning || attempt.State != contentruntime.RuntimeAttemptRunning || node.LeaseOwner != owner || attempt.LeaseOwner != owner || node.FenceToken != fenceToken || attempt.FenceToken != fenceToken || node.LeaseExpiresAt == nil || attempt.LeaseExpiresAt == nil || !node.LeaseExpiresAt.After(now) || !attempt.LeaseExpiresAt.After(now) {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, fault.Conflict("DISPATCH_LEASE_STALE", "调度租约无效、已过期或不属于当前执行者")
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
		if reservation.TenantID == tenantID && reservation.AttemptID == attempt.ID && reservation.State == contentruntime.ReservationHeld && reservation.FenceToken == fenceToken {
			reservation.ExpiresAt = &expires
			reservation.UpdatedAt = now
			s.runtimeReservations[key] = reservation
		}
	}
	return node, attempt, nil
}

func (s *Store) FinalizeDispatch(_ context.Context, node contentruntime.NodeRun, expectedNodeVersion int, attempt contentruntime.RuntimeAttempt, expectedAttemptVersion int, agent contentruntime.AgentInstance, expectedAgentVersion int, fenceToken string, event contentruntime.JobEvent) (contentruntime.NodeRun, contentruntime.RuntimeAttempt, contentruntime.AgentInstance, error) {
	if err := node.Validate(); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := attempt.Validate(); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := agent.Validate(); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentNode, currentAttempt, currentAgent, err := s.dispatchStateLocked(node.TenantID, node.ID, attempt.ID, agent.ID)
	if err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if currentAttempt.Terminal() {
		if currentAttempt.State == attempt.State && currentAttempt.ResultDigest == attempt.ResultDigest {
			return currentNode, currentAttempt, currentAgent, nil
		}
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("RUNTIME_ATTEMPT_RESULT_CONFLICT", "RuntimeAttempt 已收到不同的终态结果")
	}
	if currentNode.Version != expectedNodeVersion || currentAttempt.Version != expectedAttemptVersion || currentAgent.Version != expectedAgentVersion {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被其他执行者更新")
	}
	if currentNode.LeaseOwner == "" || currentNode.LeaseOwner != currentAttempt.LeaseOwner || currentAttempt.LeaseOwner != event.ActorID || strings.TrimSpace(fenceToken) == "" || currentNode.FenceToken != fenceToken || currentAttempt.FenceToken != fenceToken || currentNode.LeaseExpiresAt == nil || currentAttempt.LeaseExpiresAt == nil || !currentNode.LeaseExpiresAt.After(event.OccurredAt) || !currentAttempt.LeaseExpiresAt.After(event.OccurredAt) {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Conflict("DISPATCH_LEASE_STALE", "终态结果不属于当前调度租约")
	}
	if err := currentNode.Transition(node.State); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := currentAttempt.Transition(attempt.State); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if err := currentAgent.Transition(agent.State); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	if node.Version != expectedNodeVersion+1 || attempt.Version != expectedAttemptVersion+1 || agent.Version != expectedAgentVersion+1 || !sameDispatchScope(node, attempt, contentruntime.ContextView{TenantID: node.TenantID, JobRunID: node.JobRunID, NodeRunID: node.ID}, agent) {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.Invalid("DISPATCH_FINALIZE_INVALID", "终态调度对象的版本或范围不一致")
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, err
	}
	s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
	s.runtimeAttempts[runtimePlanKey(attempt.TenantID, attempt.ID)] = attempt
	s.runtimeAgents[runtimePlanKey(agent.TenantID, agent.ID)] = agent
	for key, reservation := range s.runtimeReservations {
		if reservation.TenantID == attempt.TenantID && reservation.AttemptID == attempt.ID && reservation.State == contentruntime.ReservationHeld {
			reservation.State = contentruntime.ReservationConsumed
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

func (s *Store) dispatchStateLocked(tenantID, nodeID, attemptID, agentID string) (contentruntime.NodeRun, contentruntime.RuntimeAttempt, contentruntime.AgentInstance, error) {
	node, ok := s.runtimeNodes[runtimeNodeKey(tenantID, nodeID)]
	if !ok {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.NotFound("执行节点")
	}
	attempt, ok := s.runtimeAttempts[runtimePlanKey(tenantID, attemptID)]
	if !ok {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.NotFound("RuntimeAttempt")
	}
	agent, ok := s.runtimeAgents[runtimePlanKey(tenantID, agentID)]
	if !ok {
		return contentruntime.NodeRun{}, contentruntime.RuntimeAttempt{}, contentruntime.AgentInstance{}, fault.NotFound("AgentInstance")
	}
	return node, attempt, agent, nil
}

func sameDispatchScope(node contentruntime.NodeRun, attempt contentruntime.RuntimeAttempt, view contentruntime.ContextView, agent contentruntime.AgentInstance) bool {
	return node.TenantID == attempt.TenantID && node.TenantID == view.TenantID && node.TenantID == agent.TenantID &&
		node.JobRunID == attempt.JobRunID && node.JobRunID == view.JobRunID && node.JobRunID == agent.JobRunID &&
		node.ID == attempt.NodeRunID && node.ID == view.NodeRunID && node.ID == agent.NodeRunID
}

func sameTimePointer(left, right *time.Time) bool {
	return left != nil && right != nil && left.Equal(*right)
}

func validateNewContextViewLocked(s *Store, view contentruntime.ContextView) error {
	key := runtimePlanKey(view.TenantID, view.ID)
	if _, ok := s.runtimeContextViews[key]; ok {
		return fault.Conflict("CONTEXT_VIEW_EXISTS", "ContextView 已存在")
	}
	for _, existing := range s.runtimeContextViews {
		if existing.TenantID == view.TenantID && existing.Digest == view.Digest {
			return fault.Conflict("CONTEXT_VIEW_DIGEST_EXISTS", "相同摘要的 ContextView 已存在")
		}
		if existing.TenantID == view.TenantID && existing.NodeRunID == view.NodeRunID && existing.AttemptID == view.AttemptID {
			return fault.Conflict("CONTEXT_VIEW_ATTEMPT_EXISTS", "执行尝试已经绑定 ContextView")
		}
	}
	return nil
}

func validateRuntimeEventLocked(s *Store, event contentruntime.JobEvent) error {
	if err := validateRuntimeEventFields(event); err != nil {
		return err
	}
	if job, ok := s.runtimeJobs[runtimePlanKey(event.TenantID, event.JobRunID)]; !ok || job.TenantID != event.TenantID {
		return fault.NotFound("执行实例")
	}
	return nil
}

func validateRuntimeEventFields(event contentruntime.JobEvent) error {
	if event.ID == "" || event.TenantID == "" || event.JobRunID == "" || event.Type == "" || event.ActorType == "" || event.OccurredAt.IsZero() {
		return fault.Invalid("JOB_EVENT_INVALID", "JobEvent 缺少执行实例、类型或执行者")
	}
	return nil
}

func appendRuntimeEventLocked(s *Store, event contentruntime.JobEvent) contentruntime.JobEvent {
	events := s.runtimeEvents[event.JobRunID]
	event.Sequence = int64(len(events) + 1)
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	s.runtimeEvents[event.JobRunID] = append(events, event)
	enqueueRuntimeOutboxLocked(s, event)
	return event
}

func enqueueRuntimeOutboxLocked(s *Store, event contentruntime.JobEvent) {
	outboxID := runtimePlanKey(event.TenantID, event.ID)
	if _, exists := s.runtimeOutbox[outboxID]; !exists {
		s.runtimeOutbox[outboxID] = contentruntime.RuntimeOutboxMessage{
			ID: event.ID, TenantID: event.TenantID, EventID: event.ID,
			SchemaVersion: contentruntime.RuntimeEventSchema, Topic: "runtime.job_event",
			AggregateID: event.JobRunID,
			Payload:     map[string]any{"event_id": event.ID, "job_run_id": event.JobRunID, "sequence": event.Sequence, "type": event.Type, "payload": event.Payload},
			CreatedAt:   event.OccurredAt,
		}
	}
	for _, subscriber := range contentruntime.RuntimeOutboxSubscribers(event.Type) {
		key := runtimeOutboxReceiptKey(event.TenantID, event.ID, subscriber)
		if _, exists := s.runtimeOutboxReceipts[key]; !exists {
			s.runtimeOutboxReceipts[key] = runtimeOutboxReceipt{TenantID: event.TenantID, MessageID: event.ID, Subscriber: subscriber, NextAttemptAt: event.OccurredAt, CreatedAt: event.OccurredAt}
		}
	}
}

func runtimeOutboxReceiptKey(tenantID, messageID, subscriber string) string {
	return tenantID + ":" + messageID + ":" + subscriber
}
