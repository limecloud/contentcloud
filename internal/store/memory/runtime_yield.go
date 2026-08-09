package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) RuntimeYield(_ context.Context, tenantID, id string) (domain.RuntimeYield, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeYields[runtimePlanKey(tenantID, id)]
	if !ok {
		return domain.RuntimeYield{}, domain.NotFound("RuntimeYield")
	}
	value.WaitRefs = append([]string(nil), value.WaitRefs...)
	return value, nil
}

func (s *Store) RuntimeYields(_ context.Context, tenantID, jobID string) ([]domain.RuntimeYield, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.RuntimeYield{}
	for _, value := range s.runtimeYields {
		if value.TenantID == tenantID && (jobID == "" || value.JobRunID == jobID) {
			value.WaitRefs = append([]string(nil), value.WaitRefs...)
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].YieldedAt.Equal(result[j].YieldedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].YieldedAt.Before(result[j].YieldedAt)
	})
	return result, nil
}

func (s *Store) YieldDispatch(_ context.Context, yielded domain.RuntimeYield, node domain.NodeRun, expectedNodeVersion int, attempt domain.RuntimeAttempt, expectedAttemptVersion int, agent domain.AgentInstance, expectedAgentVersion int, fenceToken string, event domain.JobEvent) (domain.RuntimeYield, domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error) {
	if err := yielded.Validate(); err != nil {
		return yielded, node, attempt, agent, err
	}
	if err := node.Validate(); err != nil {
		return yielded, node, attempt, agent, err
	}
	if err := attempt.Validate(); err != nil {
		return yielded, node, attempt, agent, err
	}
	if err := agent.Validate(); err != nil {
		return yielded, node, attempt, agent, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentNode, currentAttempt, currentAgent, err := s.dispatchStateLocked(node.TenantID, node.ID, attempt.ID, agent.ID)
	if err != nil {
		return yielded, node, attempt, agent, err
	}
	if existing, ok := s.runtimeYields[runtimePlanKey(yielded.TenantID, yielded.ID)]; ok {
		return existing, currentNode, currentAttempt, currentAgent, nil
	}
	if currentNode.Version != expectedNodeVersion || currentAttempt.Version != expectedAttemptVersion || currentAgent.Version != expectedAgentVersion {
		return yielded, node, attempt, agent, domain.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被其他执行者更新")
	}
	if currentNode.State != domain.NodeRunning || currentAttempt.State != domain.RuntimeAttemptRunning || currentAgent.State != domain.AgentActive || currentNode.LeaseOwner != currentAttempt.LeaseOwner || currentAttempt.LeaseOwner != event.ActorID || fenceToken == "" || currentNode.FenceToken != fenceToken || currentAttempt.FenceToken != fenceToken {
		return yielded, node, attempt, agent, domain.Conflict("DISPATCH_LEASE_STALE", "让出请求不属于当前调度租约")
	}
	if yielded.TenantID != node.TenantID || yielded.JobRunID != node.JobRunID || yielded.NodeRunID != node.ID || yielded.AttemptID != attempt.ID || yielded.AgentInstanceID != agent.ID {
		return yielded, node, attempt, agent, domain.Invalid("RUNTIME_YIELD_SCOPE_INVALID", "RuntimeYield 不属于当前调度范围")
	}
	if node.Version != expectedNodeVersion+1 || attempt.Version != expectedAttemptVersion+1 || agent.Version != expectedAgentVersion+1 {
		return yielded, node, attempt, agent, domain.Invalid("RUNTIME_YIELD_VERSION_INVALID", "让出后的调度对象版本无效")
	}
	if err := currentNode.Transition(node.State); err != nil {
		return yielded, node, attempt, agent, err
	}
	if err := currentAttempt.Transition(attempt.State); err != nil {
		return yielded, node, attempt, agent, err
	}
	if err := currentAgent.Transition(agent.State); err != nil {
		return yielded, node, attempt, agent, err
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return yielded, node, attempt, agent, err
	}
	s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
	s.runtimeAttempts[runtimePlanKey(attempt.TenantID, attempt.ID)] = attempt
	s.runtimeAgents[runtimePlanKey(agent.TenantID, agent.ID)] = agent
	s.runtimeYields[runtimePlanKey(yielded.TenantID, yielded.ID)] = yielded
	for key, reservation := range s.runtimeReservations {
		if reservation.TenantID == attempt.TenantID && reservation.AttemptID == attempt.ID && reservation.State == domain.ReservationHeld && reservation.FenceToken == fenceToken {
			reservation.State = domain.ReservationReleased
			reservation.FenceToken = ""
			reservation.ExpiresAt = nil
			releasedAt := event.OccurredAt
			reservation.ReleasedAt = &releasedAt
			reservation.UpdatedAt = releasedAt
			s.runtimeReservations[key] = reservation
		}
	}
	appendRuntimeEventLocked(s, event)
	return yielded, node, attempt, agent, nil
}

func (s *Store) ResolveRuntimeYield(_ context.Context, yielded domain.RuntimeYield, expectedYieldVersion int, node domain.NodeRun, expectedNodeVersion int, agent domain.AgentInstance, expectedAgentVersion int, event domain.JobEvent) (domain.RuntimeYield, domain.NodeRun, domain.AgentInstance, error) {
	if err := yielded.Validate(); err != nil {
		return yielded, node, agent, err
	}
	if err := node.Validate(); err != nil {
		return yielded, node, agent, err
	}
	if err := agent.Validate(); err != nil {
		return yielded, node, agent, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentYield, ok := s.runtimeYields[runtimePlanKey(yielded.TenantID, yielded.ID)]
	if !ok {
		return yielded, node, agent, domain.NotFound("RuntimeYield")
	}
	if currentYield.State == domain.RuntimeYieldResolved {
		if currentYield.ResumeKey == yielded.ResumeKey {
			return currentYield, s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)], s.runtimeAgents[runtimePlanKey(agent.TenantID, agent.ID)], nil
		}
		return yielded, node, agent, domain.Conflict("RUNTIME_YIELD_ALREADY_RESOLVED", "RuntimeYield 已由其他恢复请求处理")
	}
	currentNode, nodeOK := s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)]
	currentAgent, agentOK := s.runtimeAgents[runtimePlanKey(agent.TenantID, agent.ID)]
	if !nodeOK || !agentOK {
		return yielded, node, agent, domain.NotFound("RuntimeYield 调度对象")
	}
	if currentYield.Version != expectedYieldVersion || currentNode.Version != expectedNodeVersion || currentAgent.Version != expectedAgentVersion || yielded.Version != expectedYieldVersion+1 || node.Version != expectedNodeVersion+1 || agent.Version != expectedAgentVersion+1 {
		return yielded, node, agent, domain.Conflict("RUNTIME_YIELD_VERSION_CONFLICT", "RuntimeYield 已被更新，请重新读取")
	}
	if err := currentNode.Transition(node.State); err != nil {
		return yielded, node, agent, err
	}
	if err := currentAgent.Transition(agent.State); err != nil {
		return yielded, node, agent, err
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return yielded, node, agent, err
	}
	s.runtimeYields[runtimePlanKey(yielded.TenantID, yielded.ID)] = yielded
	s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
	s.runtimeAgents[runtimePlanKey(agent.TenantID, agent.ID)] = agent
	appendRuntimeEventLocked(s, event)
	return yielded, node, agent, nil
}
