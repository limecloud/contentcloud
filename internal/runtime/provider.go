package runtime

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type ProviderCallbackInput struct {
	TenantID       string
	JobRunID       string
	EffectID       string
	ProviderID     string
	MessageID      string
	ExternalID     string
	ProviderState  string
	ReceivedDigest string
	ResponseDigest string
	CostMinor      int64
	Currency       string
	SafePayload    map[string]any
	ErrorCode      string
	ReceivedAt     time.Time
}

type ProviderBillInput struct {
	TenantID    string
	JobRunID    string
	EffectID    string
	ProviderID  string
	BillID      string
	ExternalID  string
	BillDigest  string
	AmountMinor int64
	Currency    string
	ObservedAt  time.Time
}

func (s *Service) ReceiveProviderCallback(ctx context.Context, input ProviderCallbackInput) (domain.ProviderInboxMessage, domain.ExternalEffect, error) {
	if s == nil || s.repo == nil {
		return domain.ProviderInboxMessage{}, domain.ExternalEffect{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.ProviderID) == "" || strings.TrimSpace(input.MessageID) == "" || strings.TrimSpace(input.ExternalID) == "" || strings.TrimSpace(input.ProviderState) == "" {
		return domain.ProviderInboxMessage{}, domain.ExternalEffect{}, domain.Invalid("PROVIDER_CALLBACK_INVALID", "Provider 回调缺少租户、消息或外部任务身份")
	}
	if input.Currency == "" {
		input.Currency = "CNY"
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.ReceivedAt.IsZero() {
		input.ReceivedAt = s.now().UTC()
	}
	if input.SafePayload == nil {
		input.SafePayload = map[string]any{}
	}
	callbackDigestPayload := struct {
		ProviderID    string         `json:"provider_id"`
		MessageID     string         `json:"message_id"`
		ExternalID    string         `json:"external_id"`
		ProviderState string         `json:"provider_state"`
		CostMinor     int64          `json:"cost_minor"`
		Currency      string         `json:"currency"`
		Payload       map[string]any `json:"payload"`
	}{input.ProviderID, input.MessageID, input.ExternalID, input.ProviderState, input.CostMinor, input.Currency, input.SafePayload}
	digest, err := domain.CanonicalHash(callbackDigestPayload)
	if err != nil {
		return domain.ProviderInboxMessage{}, domain.ExternalEffect{}, err
	}
	if input.ReceivedDigest == "" {
		input.ReceivedDigest = "sha256:" + digest
	}
	if input.ResponseDigest == "" {
		responseDigest, digestErr := domain.CanonicalHash(struct {
			ProviderID    string         `json:"provider_id"`
			ExternalID    string         `json:"external_id"`
			ProviderState string         `json:"provider_state"`
			CostMinor     int64          `json:"cost_minor"`
			Currency      string         `json:"currency"`
			Payload       map[string]any `json:"payload"`
		}{input.ProviderID, input.ExternalID, input.ProviderState, input.CostMinor, input.Currency, input.SafePayload})
		if digestErr != nil {
			return domain.ProviderInboxMessage{}, domain.ExternalEffect{}, digestErr
		}
		input.ResponseDigest = "sha256:" + responseDigest
	}
	var effect *domain.ExternalEffect
	existingEffect, effectErr := domain.ExternalEffect{}, error(nil)
	if strings.TrimSpace(input.EffectID) != "" {
		existingEffect, effectErr = s.repo.Effect(ctx, input.TenantID, input.EffectID)
	} else {
		existingEffect, effectErr = s.repo.EffectByExternalID(ctx, input.TenantID, input.ProviderID, input.ExternalID)
	}
	if effectErr == nil {
		input.JobRunID = existingEffect.JobRunID
		effectValue := existingEffect
		effect = &effectValue
	} else if !domain.IsNotFound(effectErr) {
		return domain.ProviderInboxMessage{}, domain.ExternalEffect{}, effectErr
	}
	if strings.TrimSpace(input.JobRunID) == "" {
		return domain.ProviderInboxMessage{}, domain.ExternalEffect{}, domain.Invalid("PROVIDER_CALLBACK_SCOPE_REQUIRED", "无法匹配本地外部操作时必须提供 Runtime JobRun 作用域")
	}
	message := domain.ProviderInboxMessage{ID: domain.NewID(), TenantID: input.TenantID, JobRunID: input.JobRunID, ProviderID: input.ProviderID, MessageID: input.MessageID, ReceivedDigest: input.ReceivedDigest, ExternalID: input.ExternalID, ProviderState: input.ProviderState, ResponseDigest: input.ResponseDigest, CostMinor: input.CostMinor, Currency: input.Currency, SafePayload: input.SafePayload, State: domain.ProviderInboxReceived, Version: 1, ReceivedAt: input.ReceivedAt, CreatedAt: input.ReceivedAt, UpdatedAt: input.ReceivedAt, ErrorCode: input.ErrorCode}
	var reconciliation *domain.ProviderReconciliation
	if effect != nil {
		target := providerEffectTarget(effect.State, input.ProviderState)
		if effect.State == domain.EffectUnknown && (target == domain.EffectSucceeded || target == domain.EffectFailed) {
			target = domain.EffectReconciling
		}
		next := *effect
		next.State, next.ExternalID, next.ResponseDigest, next.CostMinor, next.Currency, next.ErrorCode = target, input.ExternalID, input.ResponseDigest, input.CostMinor, input.Currency, input.ErrorCode
		if target != effect.State {
			next.Version++
		}
		next.UpdatedAt = input.ReceivedAt
		effect = &next
		if target == domain.EffectReconciling {
			reconciliation = newProviderReconciliation(message, next, input.ProviderState, domain.ProviderReconPending, "外部结果在本地请求超时后到达，等待授权收敛")
		}
	} else {
		reconciliation = &domain.ProviderReconciliation{ID: domain.NewID(), TenantID: message.TenantID, JobRunID: message.JobRunID, ProviderID: message.ProviderID, ExternalID: message.ExternalID, RequestKey: "provider-inbox:" + message.ProviderID + ":" + message.MessageID, ObservedState: message.ProviderState, ResponseDigest: message.ResponseDigest, ExpectedMinor: 0, ObservedMinor: message.CostMinor, Currency: message.Currency, Status: domain.ProviderReconPending, SafeSummary: map[string]any{"message_id": message.MessageID}, CreatedAt: input.ReceivedAt, UpdatedAt: input.ReceivedAt, Version: 1}
	}
	commands, err := s.commands()
	if err != nil {
		return message, domain.ExternalEffect{}, err
	}
	expectedEffectVersion := 0
	if effect != nil {
		expectedEffectVersion = effect.Version
		if effect.State != existingEffect.State || effect.Version != existingEffect.Version {
			expectedEffectVersion = effect.Version - 1
		}
	}
	// A terminal duplicate is accepted as an applied inbox fact without
	// attempting an illegal terminal-to-terminal transition.
	if effect != nil && effect.State == existingEffect.State && effect.Version == existingEffect.Version {
		expectedEffectVersion = effect.Version
	}
	return commands.ReceiveProviderInboxCommand(ctx, message, effect, expectedEffectVersion, reconciliation, domain.JobEvent{ID: domain.NewID(), TenantID: message.TenantID, JobRunID: message.JobRunID, Type: "provider.inbox.received", ActorType: "provider", ActorID: message.ProviderID, IdempotencyKey: "provider-inbox:" + message.ProviderID + ":" + message.MessageID, Payload: map[string]any{"provider_id": message.ProviderID, "external_id": message.ExternalID, "provider_state": message.ProviderState}, OccurredAt: input.ReceivedAt})
}

func providerEffectTarget(currentState, providerState string) string {
	switch strings.ToLower(strings.TrimSpace(providerState)) {
	case "succeeded", "success", "completed", "complete":
		return domain.EffectSucceeded
	case "failed", "failure", "cancelled", "canceled":
		return domain.EffectFailed
	case "submitted", "queued", "accepted":
		if currentState == domain.EffectRegistered {
			return domain.EffectSubmitted
		}
		return domain.EffectAcknowledged
	case "unknown", "timeout":
		return domain.EffectUnknown
	default:
		return currentState
	}
}

func newProviderReconciliation(message domain.ProviderInboxMessage, effect domain.ExternalEffect, observedState, status, reason string) *domain.ProviderReconciliation {
	return &domain.ProviderReconciliation{ID: domain.NewID(), TenantID: message.TenantID, JobRunID: message.JobRunID, EffectID: effect.ID, ProviderID: message.ProviderID, ExternalID: message.ExternalID, RequestKey: "provider-reconcile:" + message.ProviderID + ":" + message.MessageID, ObservedState: observedState, ResponseDigest: message.ResponseDigest, ExpectedMinor: effect.CostMinor, ObservedMinor: message.CostMinor, Currency: message.Currency, Reason: reason, Status: status, SafeSummary: map[string]any{"message_id": message.MessageID}, CreatedAt: message.ReceivedAt, UpdatedAt: message.ReceivedAt, Version: 1}
}

func (s *Service) ResolveProviderReconciliation(ctx context.Context, tenantID, reconciliationID, nextState, actorID string) (domain.ProviderReconciliation, domain.ExternalEffect, error) {
	reconciliation, err := s.repo.ProviderReconciliation(ctx, tenantID, reconciliationID)
	if err != nil {
		return reconciliation, domain.ExternalEffect{}, err
	}
	if reconciliation.EffectID == "" {
		return reconciliation, domain.ExternalEffect{}, domain.Policy("PROVIDER_EFFECT_REQUIRED", "当前对账没有可收敛的本地外部操作", "先补录 Provider 与本地操作的关联")
	}
	effect, err := s.repo.Effect(ctx, tenantID, reconciliation.EffectID)
	if err != nil {
		return reconciliation, effect, err
	}
	if err := effect.Transition(nextState); err != nil {
		return reconciliation, effect, err
	}
	now := s.now().UTC()
	effect.State, effect.Version, effect.UpdatedAt = nextState, effect.Version+1, now
	reconciliation.Version++
	reconciliation.Status = domain.ProviderReconMatched
	if nextState == domain.EffectManual {
		reconciliation.Status = domain.ProviderReconManual
	}
	reconciliation.ResolvedAt, reconciliation.UpdatedAt = &now, now
	commands, err := s.commands()
	if err != nil {
		return reconciliation, effect, err
	}
	return commands.ResolveProviderReconciliationCommand(ctx, reconciliation, effect, effect.Version-1, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: effect.JobRunID, Type: "provider.reconciliation.resolved", ActorType: "operator", ActorID: strings.TrimSpace(actorID), IdempotencyKey: "provider-reconcile-resolve:" + reconciliation.ID + ":" + nextState, Payload: map[string]any{"reconciliation_id": reconciliation.ID, "effect_id": effect.ID, "state": nextState}, OccurredAt: now})
}

func (s *Service) RecordProviderBill(ctx context.Context, input ProviderBillInput) (domain.ProviderBillRecord, error) {
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.ProviderID) == "" || strings.TrimSpace(input.BillID) == "" || strings.TrimSpace(input.ExternalID) == "" {
		return domain.ProviderBillRecord{}, domain.Invalid("PROVIDER_BILL_INPUT_INVALID", "Provider 账单缺少租户、服务商、账单或外部任务身份")
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = s.now().UTC()
	}
	if input.Currency == "" {
		input.Currency = "CNY"
	}
	if input.BillDigest == "" {
		digest, err := domain.CanonicalHash(struct {
			ProviderID string `json:"provider_id"`
			BillID     string `json:"bill_id"`
			ExternalID string `json:"external_id"`
			Amount     int64  `json:"amount_minor"`
			Currency   string `json:"currency"`
		}{input.ProviderID, input.BillID, input.ExternalID, input.AmountMinor, input.Currency})
		if err != nil {
			return domain.ProviderBillRecord{}, err
		}
		input.BillDigest = "sha256:" + digest
	}
	effect, effectErr := domain.ExternalEffect{}, error(nil)
	if input.EffectID != "" {
		effect, effectErr = s.repo.Effect(ctx, input.TenantID, input.EffectID)
	} else {
		effect, effectErr = s.repo.EffectByExternalID(ctx, input.TenantID, input.ProviderID, input.ExternalID)
	}
	if effectErr != nil && !domain.IsNotFound(effectErr) {
		return domain.ProviderBillRecord{}, effectErr
	}
	bill := domain.ProviderBillRecord{ID: domain.NewID(), TenantID: input.TenantID, JobRunID: input.JobRunID, ProviderID: input.ProviderID, BillID: input.BillID, ExternalID: input.ExternalID, EffectID: input.EffectID, BillDigest: input.BillDigest, AmountMinor: input.AmountMinor, Currency: input.Currency, Status: domain.ProviderBillUnmatched, ObservedAt: input.ObservedAt, CreatedAt: input.ObservedAt, UpdatedAt: input.ObservedAt, Version: 1}
	var reconciliation *domain.ProviderReconciliation
	if effectErr == nil {
		bill.JobRunID, bill.EffectID = effect.JobRunID, effect.ID
		bill.Status = domain.ProviderBillDisputed
		if effect.Currency == input.Currency && effect.CostMinor == input.AmountMinor {
			bill.Status = domain.ProviderBillMatched
		}
		reconStatus := domain.ProviderReconCostMismatch
		if bill.Status == domain.ProviderBillMatched {
			reconStatus = domain.ProviderReconMatched
		}
		reconciliation = &domain.ProviderReconciliation{ID: domain.NewID(), TenantID: input.TenantID, JobRunID: effect.JobRunID, EffectID: effect.ID, ProviderID: input.ProviderID, ExternalID: input.ExternalID, RequestKey: "provider-bill:" + input.ProviderID + ":" + input.BillID, ObservedState: effect.State, ResponseDigest: input.BillDigest, ExpectedMinor: effect.CostMinor, ObservedMinor: input.AmountMinor, Currency: input.Currency, Status: reconStatus, Reason: "账单与 Runtime Effect 费用对账", SafeSummary: map[string]any{"bill_id": input.BillID}, CreatedAt: input.ObservedAt, UpdatedAt: input.ObservedAt, Version: 1}
	} else {
		reconciliation = &domain.ProviderReconciliation{ID: domain.NewID(), TenantID: input.TenantID, JobRunID: input.JobRunID, ProviderID: input.ProviderID, ExternalID: input.ExternalID, RequestKey: "provider-bill:" + input.ProviderID + ":" + input.BillID, ObservedState: "bill_only", ResponseDigest: input.BillDigest, ExpectedMinor: 0, ObservedMinor: input.AmountMinor, Currency: input.Currency, Status: domain.ProviderReconPending, Reason: "账单到达时没有匹配的 Runtime Effect", SafeSummary: map[string]any{"bill_id": input.BillID}, CreatedAt: input.ObservedAt, UpdatedAt: input.ObservedAt, Version: 1}
	}
	commands, err := s.commands()
	if err != nil {
		return bill, err
	}
	_, err = commands.RecordProviderBillCommand(ctx, bill, reconciliation, domain.JobEvent{ID: domain.NewID(), TenantID: bill.TenantID, JobRunID: bill.JobRunID, Type: "provider.bill.recorded", ActorType: "provider", ActorID: bill.ProviderID, IdempotencyKey: "provider-bill:" + bill.ProviderID + ":" + bill.BillID, Payload: map[string]any{"bill_id": bill.BillID, "status": bill.Status, "amount_minor": bill.AmountMinor}, OccurredAt: bill.ObservedAt})
	return bill, err
}
