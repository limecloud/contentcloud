package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/domain"
)

func providerInboxKey(tenantID, providerID, messageID string) string {
	return tenantID + ":" + providerID + ":" + messageID
}
func providerReconKey(tenantID, requestKey string) string { return tenantID + ":" + requestKey }
func providerBillKey(tenantID, providerID, billID string) string {
	return tenantID + ":" + providerID + ":" + billID
}

func (s *Store) ProviderInboxMessage(_ context.Context, tenantID, id string) (domain.ProviderInboxMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.runtimeProviderInbox {
		if value.TenantID == tenantID && value.ID == id {
			return value, nil
		}
	}
	return domain.ProviderInboxMessage{}, domain.NotFound("Provider 回调消息")
}

func (s *Store) ProviderInboxMessages(_ context.Context, tenantID, jobID string) ([]domain.ProviderInboxMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ProviderInboxMessage{}
	for _, value := range s.runtimeProviderInbox {
		if value.TenantID == tenantID && (jobID == "" || value.JobRunID == jobID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ReceivedAt.Equal(result[j].ReceivedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].ReceivedAt.Before(result[j].ReceivedAt)
	})
	return result, nil
}

func (s *Store) ProviderReconciliations(_ context.Context, tenantID, effectID string) ([]domain.ProviderReconciliation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ProviderReconciliation{}
	for _, value := range s.runtimeProviderRecons {
		if value.TenantID == tenantID && (effectID == "" || value.EffectID == effectID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) ProviderReconciliation(_ context.Context, tenantID, id string) (domain.ProviderReconciliation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.runtimeProviderRecons {
		if value.TenantID == tenantID && value.ID == id {
			return value, nil
		}
	}
	return domain.ProviderReconciliation{}, domain.NotFound("Provider 对账")
}

func (s *Store) ProviderBillRecords(_ context.Context, tenantID, effectID string) ([]domain.ProviderBillRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ProviderBillRecord{}
	for _, value := range s.runtimeProviderBills {
		if value.TenantID == tenantID && (effectID == "" || value.EffectID == effectID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ObservedAt.Before(result[j].ObservedAt) })
	return result, nil
}

func (s *Store) ReceiveProviderInboxCommand(_ context.Context, message domain.ProviderInboxMessage, effect *domain.ExternalEffect, expectedEffectVersion int, reconciliation *domain.ProviderReconciliation, event domain.JobEvent) (domain.ProviderInboxMessage, domain.ExternalEffect, error) {
	message.NormalizeCollections()
	if err := message.Validate(); err != nil {
		return message, domain.ExternalEffect{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := providerInboxKey(message.TenantID, message.ProviderID, message.MessageID)
	if existing, ok := s.runtimeProviderInbox[key]; ok {
		if existing.ReceivedDigest != message.ReceivedDigest {
			return existing, domain.ExternalEffect{}, domain.Conflict("PROVIDER_INBOX_DIGEST_CONFLICT", "相同 Provider 消息 ID 的内容摘要不一致")
		}
		if existing.EffectID != "" {
			return existing, s.runtimeEffects[runtimeEffectKey(existing.TenantID, existing.EffectID)], nil
		}
		return existing, domain.ExternalEffect{}, nil
	}
	if event.TenantID != message.TenantID || event.JobRunID != message.JobRunID {
		return message, domain.ExternalEffect{}, domain.Invalid("JOB_EVENT_SCOPE_INVALID", "Provider 回调事件不属于当前执行实例")
	}
	if err := validateCommandEvent(s, event); err != nil {
		return message, domain.ExternalEffect{}, err
	}
	resultEffect := domain.ExternalEffect{}
	if effect != nil {
		current, ok := s.runtimeEffects[runtimeEffectKey(effect.TenantID, effect.ID)]
		if !ok {
			return message, resultEffect, domain.NotFound("外部操作")
		}
		noEffectChange := current.Version == expectedEffectVersion && effect.Version == current.Version && providerEffectEqual(current, *effect)
		if !noEffectChange && (current.Version != expectedEffectVersion || effect.Version != expectedEffectVersion+1) {
			return message, resultEffect, domain.Conflict("EFFECT_VERSION_CONFLICT", "外部操作已被更新，请重新读取")
		}
		if !noEffectChange {
			if err := current.Transition(effect.State); err != nil {
				return message, resultEffect, err
			}
		}
		if effect.TenantID != message.TenantID || effect.JobRunID != message.JobRunID || effect.ExternalID != message.ExternalID {
			return message, resultEffect, domain.Invalid("PROVIDER_INBOX_EFFECT_SCOPE_INVALID", "Provider 回调与外部操作范围不一致")
		}
		message.EffectID = effect.ID
		resultEffect = *effect
	}
	if reconciliation != nil {
		reconciliation.NormalizeCollections()
		if err := reconciliation.Validate(); err != nil {
			return message, resultEffect, err
		}
		if reconciliation.TenantID != message.TenantID || reconciliation.JobRunID != message.JobRunID {
			return message, resultEffect, domain.Invalid("PROVIDER_RECONCILIATION_SCOPE_INVALID", "Provider 对账不属于当前执行实例")
		}
		s.runtimeProviderRecons[providerReconKey(reconciliation.TenantID, reconciliation.RequestKey)] = *reconciliation
	}
	if effect != nil {
		message.State = domain.ProviderInboxApplied
		processed := event.OccurredAt
		message.ProcessedAt = &processed
		s.runtimeEffects[runtimeEffectKey(effect.TenantID, effect.ID)] = *effect
	} else {
		message.State = domain.ProviderInboxPending
	}
	s.runtimeProviderInbox[key] = message
	appendRuntimeEventLocked(s, event)
	return message, resultEffect, nil
}

func (s *Store) ReceiveAgentInboxCommand(_ context.Context, message domain.ProviderInboxMessage, event domain.JobEvent) (domain.ProviderInboxMessage, error) {
	message.NormalizeCollections()
	if err := message.Validate(); err != nil {
		return message, err
	}
	if message.State != domain.ProviderInboxReceived || message.EffectID != "" || message.ProcessedAt != nil {
		return message, domain.Invalid("AGENT_INBOX_STATE_INVALID", "Agent 回调首次入站必须处于 received 状态且不能绑定外部操作")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := providerInboxKey(message.TenantID, message.ProviderID, message.MessageID)
	if existing, ok := s.runtimeProviderInbox[key]; ok {
		if existing.ReceivedDigest != message.ReceivedDigest {
			return existing, domain.Conflict("PROVIDER_INBOX_DIGEST_CONFLICT", "相同 Agent 回调消息 ID 的内容摘要不一致")
		}
		return existing, nil
	}
	if event.TenantID != message.TenantID || event.JobRunID != message.JobRunID {
		return message, domain.Invalid("JOB_EVENT_SCOPE_INVALID", "Agent 回调事件不属于当前执行实例")
	}
	if err := validateCommandEvent(s, event); err != nil {
		return message, err
	}
	s.runtimeProviderInbox[key] = message
	appendRuntimeEventLocked(s, event)
	return message, nil
}

func (s *Store) CompleteAgentInboxCommand(_ context.Context, message domain.ProviderInboxMessage, expectedVersion int, event domain.JobEvent) (domain.ProviderInboxMessage, error) {
	message.NormalizeCollections()
	if err := message.Validate(); err != nil {
		return message, err
	}
	if message.State != domain.ProviderInboxApplied && message.State != domain.ProviderInboxFailed {
		return message, domain.Invalid("AGENT_INBOX_STATE_INVALID", "Agent 回调只能收敛为 applied 或 failed")
	}
	if message.EffectID != "" || message.ProcessedAt == nil || message.Version != expectedVersion+1 {
		return message, domain.Invalid("AGENT_INBOX_COMPLETION_INVALID", "Agent 回调完成状态缺少处理时间或版本不连续")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := providerInboxKey(message.TenantID, message.ProviderID, message.MessageID)
	current, ok := s.runtimeProviderInbox[key]
	if !ok {
		return message, domain.NotFound("Agent 回调消息")
	}
	if current.ReceivedDigest != message.ReceivedDigest || current.ID != message.ID {
		return current, domain.Conflict("PROVIDER_INBOX_DIGEST_CONFLICT", "Agent 回调完成状态与入站事实不一致")
	}
	if current.State == message.State && current.Version == message.Version {
		return current, nil
	}
	if current.State != domain.ProviderInboxReceived || current.Version != expectedVersion {
		return current, domain.Conflict("AGENT_INBOX_VERSION_CONFLICT", "Agent 回调已被其他处理器更新")
	}
	if event.TenantID != message.TenantID || event.JobRunID != message.JobRunID {
		return message, domain.Invalid("JOB_EVENT_SCOPE_INVALID", "Agent 回调完成事件不属于当前执行实例")
	}
	if err := validateCommandEvent(s, event); err != nil {
		return message, err
	}
	s.runtimeProviderInbox[key] = message
	appendRuntimeEventLocked(s, event)
	return message, nil
}

func providerEffectEqual(left, right domain.ExternalEffect) bool {
	return left.ID == right.ID && left.TenantID == right.TenantID && left.JobRunID == right.JobRunID &&
		left.NodeRunID == right.NodeRunID && left.AttemptID == right.AttemptID && left.ResourceReservationID == right.ResourceReservationID &&
		left.Kind == right.Kind && left.IdempotencyKey == right.IdempotencyKey && left.State == right.State &&
		left.ExternalID == right.ExternalID && left.RequestDigest == right.RequestDigest && left.ResponseDigest == right.ResponseDigest &&
		left.CostMinor == right.CostMinor && left.Currency == right.Currency && left.ErrorCode == right.ErrorCode && left.Version == right.Version
}

func (s *Store) RecordProviderBillCommand(_ context.Context, bill domain.ProviderBillRecord, reconciliation *domain.ProviderReconciliation, event domain.JobEvent) (domain.ProviderBillRecord, error) {
	if err := bill.Validate(); err != nil {
		return bill, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := providerBillKey(bill.TenantID, bill.ProviderID, bill.BillID)
	if existing, ok := s.runtimeProviderBills[key]; ok {
		if existing.BillDigest != bill.BillDigest {
			return existing, domain.Conflict("PROVIDER_BILL_DIGEST_CONFLICT", "相同账单 ID 的内容摘要不一致")
		}
		return existing, nil
	}
	if event.TenantID != bill.TenantID || event.JobRunID != bill.JobRunID {
		return bill, domain.Invalid("JOB_EVENT_SCOPE_INVALID", "Provider 账单事件不属于当前执行实例")
	}
	if err := validateCommandEvent(s, event); err != nil {
		return bill, err
	}
	if reconciliation != nil {
		reconciliation.NormalizeCollections()
		if err := reconciliation.Validate(); err != nil {
			return bill, err
		}
		s.runtimeProviderRecons[providerReconKey(reconciliation.TenantID, reconciliation.RequestKey)] = *reconciliation
	}
	s.runtimeProviderBills[key] = bill
	appendRuntimeEventLocked(s, event)
	return bill, nil
}

func (s *Store) ResolveProviderReconciliationCommand(_ context.Context, reconciliation domain.ProviderReconciliation, effect domain.ExternalEffect, expectedEffectVersion int, event domain.JobEvent) (domain.ProviderReconciliation, domain.ExternalEffect, error) {
	reconciliation.NormalizeCollections()
	if err := reconciliation.Validate(); err != nil {
		return reconciliation, effect, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	currentRecon, ok := s.runtimeProviderRecons[providerReconKey(reconciliation.TenantID, reconciliation.RequestKey)]
	if !ok {
		return reconciliation, effect, domain.NotFound("Provider 对账")
	}
	if currentRecon.Version != reconciliation.Version-1 {
		return reconciliation, effect, domain.Conflict("PROVIDER_RECONCILIATION_VERSION_CONFLICT", "Provider 对账已被更新，请重新读取")
	}
	currentEffect, ok := s.runtimeEffects[runtimeEffectKey(effect.TenantID, effect.ID)]
	if !ok {
		return reconciliation, effect, domain.NotFound("外部操作")
	}
	if currentEffect.Version != expectedEffectVersion || effect.Version != expectedEffectVersion+1 {
		return reconciliation, effect, domain.Conflict("EFFECT_VERSION_CONFLICT", "外部操作已被更新，请重新读取")
	}
	if effect.State != currentEffect.State {
		if err := currentEffect.Transition(effect.State); err != nil {
			return reconciliation, effect, err
		}
	}
	if event.TenantID != effect.TenantID || event.JobRunID != effect.JobRunID {
		return reconciliation, effect, domain.Invalid("JOB_EVENT_SCOPE_INVALID", "Provider 对账事件不属于当前执行实例")
	}
	if err := validateCommandEvent(s, event); err != nil {
		return reconciliation, effect, err
	}
	s.runtimeProviderRecons[providerReconKey(reconciliation.TenantID, reconciliation.RequestKey)] = reconciliation
	s.runtimeEffects[runtimeEffectKey(effect.TenantID, effect.ID)] = effect
	appendRuntimeEventLocked(s, event)
	return reconciliation, effect, nil
}
