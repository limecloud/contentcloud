package runtime

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
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

func (s *Service) ReceiveProviderCallback(ctx context.Context, input ProviderCallbackInput) (ProviderInboxMessage, ExternalEffect, error) {
	if s == nil || s.repo == nil {
		return ProviderInboxMessage{}, ExternalEffect{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.ProviderID) == "" || strings.TrimSpace(input.MessageID) == "" || strings.TrimSpace(input.ExternalID) == "" || strings.TrimSpace(input.ProviderState) == "" {
		return ProviderInboxMessage{}, ExternalEffect{}, fault.Invalid("PROVIDER_CALLBACK_INVALID", "Provider 回调缺少租户、消息或外部任务身份")
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
	digest, err := stablehash.Sum(callbackDigestPayload)
	if err != nil {
		return ProviderInboxMessage{}, ExternalEffect{}, err
	}
	if input.ReceivedDigest == "" {
		input.ReceivedDigest = "sha256:" + digest
	}
	if input.ResponseDigest == "" {
		responseDigest, digestErr := stablehash.Sum(struct {
			ProviderID    string         `json:"provider_id"`
			ExternalID    string         `json:"external_id"`
			ProviderState string         `json:"provider_state"`
			CostMinor     int64          `json:"cost_minor"`
			Currency      string         `json:"currency"`
			Payload       map[string]any `json:"payload"`
		}{input.ProviderID, input.ExternalID, input.ProviderState, input.CostMinor, input.Currency, input.SafePayload})
		if digestErr != nil {
			return ProviderInboxMessage{}, ExternalEffect{}, digestErr
		}
		input.ResponseDigest = "sha256:" + responseDigest
	}
	var effect *ExternalEffect
	existingEffect, effectErr := ExternalEffect{}, error(nil)
	if strings.TrimSpace(input.EffectID) != "" {
		existingEffect, effectErr = s.repo.Effect(ctx, input.TenantID, input.EffectID)
	} else {
		existingEffect, effectErr = s.repo.EffectByExternalID(ctx, input.TenantID, input.ProviderID, input.ExternalID)
	}
	if effectErr == nil {
		input.JobRunID = existingEffect.JobRunID
		effectValue := existingEffect
		effect = &effectValue
	} else if !fault.IsNotFound(effectErr) {
		return ProviderInboxMessage{}, ExternalEffect{}, effectErr
	}
	if strings.TrimSpace(input.JobRunID) == "" {
		return ProviderInboxMessage{}, ExternalEffect{}, fault.Invalid("PROVIDER_CALLBACK_SCOPE_REQUIRED", "无法匹配本地外部操作时必须提供 Runtime JobRun 作用域")
	}
	message := ProviderInboxMessage{ID: idgen.New(), TenantID: input.TenantID, JobRunID: input.JobRunID, ProviderID: input.ProviderID, MessageID: input.MessageID, ReceivedDigest: input.ReceivedDigest, ExternalID: input.ExternalID, ProviderState: input.ProviderState, ResponseDigest: input.ResponseDigest, CostMinor: input.CostMinor, Currency: input.Currency, SafePayload: input.SafePayload, State: ProviderInboxReceived, Version: 1, ReceivedAt: input.ReceivedAt, CreatedAt: input.ReceivedAt, UpdatedAt: input.ReceivedAt, ErrorCode: input.ErrorCode}
	var reconciliation *ProviderReconciliation
	if effect != nil {
		target := providerEffectTarget(effect.State, input.ProviderState)
		if effect.State == EffectUnknown && (target == EffectSucceeded || target == EffectFailed) {
			target = EffectReconciling
		}
		next := *effect
		next.State, next.ExternalID, next.ResponseDigest, next.CostMinor, next.Currency, next.ErrorCode = target, input.ExternalID, input.ResponseDigest, input.CostMinor, input.Currency, input.ErrorCode
		if target != effect.State {
			next.Version++
		}
		next.UpdatedAt = input.ReceivedAt
		effect = &next
		if target == EffectReconciling {
			reconciliation = newProviderReconciliation(message, next, input.ProviderState, ProviderReconPending, "外部结果在本地请求超时后到达，等待授权收敛")
		}
	} else {
		reconciliation = &ProviderReconciliation{ID: idgen.New(), TenantID: message.TenantID, JobRunID: message.JobRunID, ProviderID: message.ProviderID, ExternalID: message.ExternalID, RequestKey: "provider-inbox:" + message.ProviderID + ":" + message.MessageID, ObservedState: message.ProviderState, ResponseDigest: message.ResponseDigest, ExpectedMinor: 0, ObservedMinor: message.CostMinor, Currency: message.Currency, Status: ProviderReconPending, SafeSummary: map[string]any{"message_id": message.MessageID}, CreatedAt: input.ReceivedAt, UpdatedAt: input.ReceivedAt, Version: 1}
	}
	commands, err := s.commands()
	if err != nil {
		return message, ExternalEffect{}, err
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
	return commands.ReceiveProviderInboxCommand(ctx, message, effect, expectedEffectVersion, reconciliation, JobEvent{ID: idgen.New(), TenantID: message.TenantID, JobRunID: message.JobRunID, Type: "provider.inbox.received", ActorType: "provider", ActorID: message.ProviderID, IdempotencyKey: "provider-inbox:" + message.ProviderID + ":" + message.MessageID, Payload: map[string]any{"provider_id": message.ProviderID, "external_id": message.ExternalID, "provider_state": message.ProviderState}, OccurredAt: input.ReceivedAt})
}

func providerEffectTarget(currentState, providerState string) string {
	switch strings.ToLower(strings.TrimSpace(providerState)) {
	case "succeeded", "success", "completed", "complete":
		return EffectSucceeded
	case "failed", "failure", "cancelled", "canceled":
		return EffectFailed
	case "submitted", "queued", "accepted":
		if currentState == EffectRegistered {
			return EffectSubmitted
		}
		return EffectAcknowledged
	case "unknown", "timeout":
		return EffectUnknown
	default:
		return currentState
	}
}

func newProviderReconciliation(message ProviderInboxMessage, effect ExternalEffect, observedState, status, reason string) *ProviderReconciliation {
	return &ProviderReconciliation{ID: idgen.New(), TenantID: message.TenantID, JobRunID: message.JobRunID, EffectID: effect.ID, ProviderID: message.ProviderID, ExternalID: message.ExternalID, RequestKey: "provider-reconcile:" + message.ProviderID + ":" + message.MessageID, ObservedState: observedState, ResponseDigest: message.ResponseDigest, ExpectedMinor: effect.CostMinor, ObservedMinor: message.CostMinor, Currency: message.Currency, Reason: reason, Status: status, SafeSummary: map[string]any{"message_id": message.MessageID}, CreatedAt: message.ReceivedAt, UpdatedAt: message.ReceivedAt, Version: 1}
}

func (s *Service) ResolveProviderReconciliation(ctx context.Context, tenantID, reconciliationID, nextState, actorID string) (ProviderReconciliation, ExternalEffect, error) {
	reconciliation, err := s.repo.ProviderReconciliation(ctx, tenantID, reconciliationID)
	if err != nil {
		return reconciliation, ExternalEffect{}, err
	}
	if reconciliation.EffectID == "" {
		return reconciliation, ExternalEffect{}, fault.Policy("PROVIDER_EFFECT_REQUIRED", "当前对账没有可收敛的本地外部操作", "先补录 Provider 与本地操作的关联")
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
	reconciliation.Status = ProviderReconMatched
	if nextState == EffectManual {
		reconciliation.Status = ProviderReconManual
	}
	reconciliation.ResolvedAt, reconciliation.UpdatedAt = &now, now
	commands, err := s.commands()
	if err != nil {
		return reconciliation, effect, err
	}
	return commands.ResolveProviderReconciliationCommand(ctx, reconciliation, effect, effect.Version-1, JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: effect.JobRunID, Type: "provider.reconciliation.resolved", ActorType: "operator", ActorID: strings.TrimSpace(actorID), IdempotencyKey: "provider-reconcile-resolve:" + reconciliation.ID + ":" + nextState, Payload: map[string]any{"reconciliation_id": reconciliation.ID, "effect_id": effect.ID, "state": nextState}, OccurredAt: now})
}

func (s *Service) RecordProviderBill(ctx context.Context, input ProviderBillInput) (ProviderBillRecord, error) {
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.ProviderID) == "" || strings.TrimSpace(input.BillID) == "" || strings.TrimSpace(input.ExternalID) == "" {
		return ProviderBillRecord{}, fault.Invalid("PROVIDER_BILL_INPUT_INVALID", "Provider 账单缺少租户、服务商、账单或外部任务身份")
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = s.now().UTC()
	}
	if input.Currency == "" {
		input.Currency = "CNY"
	}
	if input.BillDigest == "" {
		digest, err := stablehash.Sum(struct {
			ProviderID string `json:"provider_id"`
			BillID     string `json:"bill_id"`
			ExternalID string `json:"external_id"`
			Amount     int64  `json:"amount_minor"`
			Currency   string `json:"currency"`
		}{input.ProviderID, input.BillID, input.ExternalID, input.AmountMinor, input.Currency})
		if err != nil {
			return ProviderBillRecord{}, err
		}
		input.BillDigest = "sha256:" + digest
	}
	effect, effectErr := ExternalEffect{}, error(nil)
	if input.EffectID != "" {
		effect, effectErr = s.repo.Effect(ctx, input.TenantID, input.EffectID)
	} else {
		effect, effectErr = s.repo.EffectByExternalID(ctx, input.TenantID, input.ProviderID, input.ExternalID)
	}
	if effectErr != nil && !fault.IsNotFound(effectErr) {
		return ProviderBillRecord{}, effectErr
	}
	bill := ProviderBillRecord{ID: idgen.New(), TenantID: input.TenantID, JobRunID: input.JobRunID, ProviderID: input.ProviderID, BillID: input.BillID, ExternalID: input.ExternalID, EffectID: input.EffectID, BillDigest: input.BillDigest, AmountMinor: input.AmountMinor, Currency: input.Currency, Status: ProviderBillUnmatched, ObservedAt: input.ObservedAt, CreatedAt: input.ObservedAt, UpdatedAt: input.ObservedAt, Version: 1}
	var reconciliation *ProviderReconciliation
	if effectErr == nil {
		bill.JobRunID, bill.EffectID = effect.JobRunID, effect.ID
		bill.Status = ProviderBillDisputed
		if effect.Currency == input.Currency && effect.CostMinor == input.AmountMinor {
			bill.Status = ProviderBillMatched
		}
		reconStatus := ProviderReconCostMismatch
		if bill.Status == ProviderBillMatched {
			reconStatus = ProviderReconMatched
		}
		reconciliation = &ProviderReconciliation{ID: idgen.New(), TenantID: input.TenantID, JobRunID: effect.JobRunID, EffectID: effect.ID, ProviderID: input.ProviderID, ExternalID: input.ExternalID, RequestKey: "provider-bill:" + input.ProviderID + ":" + input.BillID, ObservedState: effect.State, ResponseDigest: input.BillDigest, ExpectedMinor: effect.CostMinor, ObservedMinor: input.AmountMinor, Currency: input.Currency, Status: reconStatus, Reason: "账单与 Runtime Effect 费用对账", SafeSummary: map[string]any{"bill_id": input.BillID}, CreatedAt: input.ObservedAt, UpdatedAt: input.ObservedAt, Version: 1}
	} else {
		reconciliation = &ProviderReconciliation{ID: idgen.New(), TenantID: input.TenantID, JobRunID: input.JobRunID, ProviderID: input.ProviderID, ExternalID: input.ExternalID, RequestKey: "provider-bill:" + input.ProviderID + ":" + input.BillID, ObservedState: "bill_only", ResponseDigest: input.BillDigest, ExpectedMinor: 0, ObservedMinor: input.AmountMinor, Currency: input.Currency, Status: ProviderReconPending, Reason: "账单到达时没有匹配的 Runtime Effect", SafeSummary: map[string]any{"bill_id": input.BillID}, CreatedAt: input.ObservedAt, UpdatedAt: input.ObservedAt, Version: 1}
	}
	commands, err := s.commands()
	if err != nil {
		return bill, err
	}
	_, err = commands.RecordProviderBillCommand(ctx, bill, reconciliation, JobEvent{ID: idgen.New(), TenantID: bill.TenantID, JobRunID: bill.JobRunID, Type: "provider.bill.recorded", ActorType: "provider", ActorID: bill.ProviderID, IdempotencyKey: "provider-bill:" + bill.ProviderID + ":" + bill.BillID, Payload: map[string]any{"bill_id": bill.BillID, "status": bill.Status, "amount_minor": bill.AmountMinor}, OccurredAt: bill.ObservedAt})
	return bill, err
}
