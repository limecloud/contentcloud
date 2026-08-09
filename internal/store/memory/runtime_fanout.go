package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/domain"
)

func runtimeFanoutKey(tenantID, id string) string { return tenantID + ":" + id }
func runtimeFanoutMemberKey(tenantID, setID, memberKey string) string {
	return tenantID + ":" + setID + ":" + memberKey
}

func (s *Store) FanoutSet(_ context.Context, tenantID, id string) (domain.FanoutSet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeFanoutSets[runtimeFanoutKey(tenantID, id)]
	if !ok {
		return domain.FanoutSet{}, domain.NotFound("FanoutSet")
	}
	return value, nil
}

func (s *Store) FanoutSetByIdempotencyKey(_ context.Context, tenantID, jobID, key string) (domain.FanoutSet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.runtimeFanoutSets {
		if value.TenantID == tenantID && value.JobRunID == jobID && value.IdempotencyKey == key {
			return value, nil
		}
	}
	return domain.FanoutSet{}, domain.NotFound("FanoutSet")
}

func (s *Store) FanoutSets(_ context.Context, tenantID, jobID string) ([]domain.FanoutSet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.FanoutSet{}
	for _, value := range s.runtimeFanoutSets {
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

func (s *Store) FanoutMembers(_ context.Context, tenantID, setID string) ([]domain.FanoutMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.FanoutMember{}
	for _, value := range s.runtimeFanoutMembers {
		if value.TenantID == tenantID && value.FanoutSetID == setID {
			copy := value
			copy.OutputRefs = append([]string{}, value.OutputRefs...)
			result = append(result, copy)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].MemberKey < result[j].MemberKey })
	return result, nil
}

func (s *Store) CreateFanoutSetCommand(_ context.Context, nextJob domain.JobRun, expectedJobVersion int, plan domain.JobPlanRevision, set domain.FanoutSet, members []domain.FanoutMember, nodes []domain.NodeRun, event domain.JobEvent) (domain.JobRun, error) {
	set.JoinPolicy = domain.NormalizeJoinPolicy(set.JoinPolicy)
	if err := nextJob.Validate(); err != nil {
		return nextJob, err
	}
	if err := plan.Validate(); err != nil {
		return nextJob, err
	}
	if err := set.Validate(); err != nil {
		return nextJob, err
	}
	if set.Status != domain.FanoutOpen && set.Status != domain.FanoutClosed || set.MemberCount != len(members) {
		return nextJob, domain.Invalid("FANOUT_SET_CREATE_INVALID", "FanoutSet 创建时必须开放或已封存且成员数量一致")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	jobKey := runtimePlanKey(nextJob.TenantID, nextJob.ID)
	currentJob, ok := s.runtimeJobs[jobKey]
	if !ok {
		return nextJob, domain.NotFound("执行实例")
	}
	if currentJob.Version != expectedJobVersion || nextJob.Version != expectedJobVersion+1 || currentJob.PlanRevisionID != plan.BaseRevisionID || nextJob.PlanRevisionID != plan.ID || nextJob.PlanDigest != plan.Digest {
		return nextJob, domain.Conflict("GRAPH_VERSION_CONFLICT", "执行图版本已经变化，请重新读取")
	}
	if _, ok := s.runtimePlans[runtimePlanKey(plan.TenantID, plan.ID)]; ok {
		return nextJob, domain.Conflict("JOB_PLAN_EXISTS", "执行计划版本已存在")
	}
	if _, ok := s.runtimeFanoutSets[runtimeFanoutKey(set.TenantID, set.ID)]; ok {
		return nextJob, domain.Conflict("FANOUT_SET_EXISTS", "FanoutSet 已存在")
	}
	for _, existing := range s.runtimeFanoutSets {
		if existing.TenantID == set.TenantID && existing.JobRunID == set.JobRunID && existing.IdempotencyKey == set.IdempotencyKey {
			return nextJob, domain.Conflict("FANOUT_SET_IDEMPOTENCY", "相同幂等键的 FanoutSet 已存在")
		}
	}
	if event.TenantID != set.TenantID || event.JobRunID != set.JobRunID {
		return nextJob, domain.Invalid("JOB_EVENT_SCOPE_INVALID", "FanoutSet 事件不属于当前执行实例")
	}
	if err := validateRuntimeEventFields(event); err != nil {
		return nextJob, err
	}
	seen := map[string]bool{}
	for _, member := range members {
		if err := member.Validate(); err != nil {
			return nextJob, err
		}
		if member.TenantID != set.TenantID || member.FanoutSetID != set.ID || member.Generation != set.Generation || seen[member.MemberKey] {
			return nextJob, domain.Invalid("FANOUT_MEMBER_SCOPE_INVALID", "FanoutSet 成员范围或唯一键无效")
		}
		seen[member.MemberKey] = true
	}
	for _, node := range nodes {
		if err := node.Validate(); err != nil {
			return nextJob, err
		}
		if node.TenantID != set.TenantID || node.JobRunID != set.JobRunID {
			return nextJob, domain.Invalid("NODE_RUN_SCOPE_INVALID", "FanoutSet 子节点不属于当前执行实例")
		}
		if _, exists := s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)]; exists {
			return nextJob, domain.Conflict("NODE_RUN_EXISTS", "FanoutSet 子节点已经存在")
		}
	}
	for _, member := range members {
		if _, exists := s.runtimeFanoutMembers[runtimeFanoutMemberKey(member.TenantID, member.FanoutSetID, member.MemberKey)]; exists {
			return nextJob, domain.Conflict("FANOUT_MEMBER_EXISTS", "FanoutSet 成员已经存在")
		}
	}
	s.runtimePlans[runtimePlanKey(plan.TenantID, plan.ID)] = plan
	s.runtimeJobs[jobKey] = nextJob
	s.runtimeFanoutSets[runtimeFanoutKey(set.TenantID, set.ID)] = set
	for _, member := range members {
		key := runtimeFanoutMemberKey(member.TenantID, member.FanoutSetID, member.MemberKey)
		s.runtimeFanoutMembers[key] = member
	}
	for _, node := range nodes {
		s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
	}
	appendRuntimeEventLocked(s, event)
	return nextJob, nil
}

func (s *Store) ApplyFanoutJoinCommand(_ context.Context, set domain.FanoutSet, expectedVersion int, members []domain.FanoutMember, cancelMemberKeys []string, event domain.JobEvent) (domain.FanoutSet, error) {
	if err := set.Validate(); err != nil {
		return set, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeFanoutKey(set.TenantID, set.ID)
	current, ok := s.runtimeFanoutSets[key]
	if !ok {
		return set, domain.NotFound("FanoutSet")
	}
	if current.Version != expectedVersion || set.Version != expectedVersion+1 || set.ID != current.ID || set.TenantID != current.TenantID {
		return set, domain.Conflict("FANOUT_SET_VERSION_CONFLICT", "FanoutSet 已被更新，请重新读取")
	}
	if len(members) != current.MemberCount {
		return set, domain.Conflict("FANOUT_MEMBER_COUNT_MISMATCH", "FanoutSet 成员数量与封存快照不一致")
	}
	if event.TenantID != current.TenantID || event.JobRunID != current.JobRunID {
		return set, domain.Invalid("JOB_EVENT_SCOPE_INVALID", "FanoutSet 事件不属于当前执行实例")
	}
	if err := validateCommandEvent(s, event); err != nil {
		return set, err
	}
	for _, member := range members {
		if err := member.Validate(); err != nil {
			return set, err
		}
		memberKey := runtimeFanoutMemberKey(member.TenantID, member.FanoutSetID, member.MemberKey)
		if member.TenantID != current.TenantID || member.FanoutSetID != current.ID {
			return set, domain.Invalid("FANOUT_MEMBER_SCOPE_INVALID", "FanoutSet 成员范围无效")
		}
		stored, exists := s.runtimeFanoutMembers[memberKey]
		if !exists {
			return set, domain.NotFound("FanoutSet 成员")
		}
		if member.Version != stored.Version && member.Version != stored.Version+1 {
			return set, domain.Conflict("FANOUT_MEMBER_VERSION_CONFLICT", "FanoutSet 成员已被更新，请重新读取")
		}
	}
	for _, member := range members {
		memberKey := runtimeFanoutMemberKey(member.TenantID, member.FanoutSetID, member.MemberKey)
		s.runtimeFanoutMembers[memberKey] = member
	}
	for _, memberKey := range cancelMemberKeys {
		found := false
		for key, member := range s.runtimeFanoutMembers {
			if member.TenantID == current.TenantID && member.FanoutSetID == current.ID && member.MemberKey == memberKey && member.State == domain.FanoutMemberPending {
				found = true
				node, nodeOK := s.runtimeNodes[runtimeNodeKey(member.TenantID, member.NodeRunID)]
				if !nodeOK {
					return set, domain.NotFound("FanoutSet 子节点")
				}
				if node.State != domain.NodePending && node.State != domain.NodeReady {
					return set, domain.Conflict("FANOUT_CANCEL_CONFLICT", "只能取消尚未领取的 Fanout 子节点")
				}
				node.State = domain.NodeCancelled
				node.Version++
				node.UpdatedAt = event.OccurredAt
				node.LeaseOwner, node.FenceToken, node.LeaseExpiresAt = "", "", nil
				s.runtimeNodes[runtimeNodeKey(node.TenantID, node.ID)] = node
				member.State = domain.FanoutMemberCancelled
				member.Version++
				member.UpdatedAt = event.OccurredAt
				s.runtimeFanoutMembers[key] = member
			}
		}
		if !found {
			return set, domain.NotFound("FanoutSet 成员")
		}
	}
	s.runtimeFanoutSets[key] = set
	appendRuntimeEventLocked(s, event)
	return set, nil
}
