package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const runtimeProviderInboxSelect = `SELECT tenant_id,id,job_run_id,provider_id,message_id,received_digest,external_id,effect_id,provider_state,response_digest,cost_minor,currency,safe_payload,state,error_code,received_at,processed_at,version,created_at,updated_at FROM runtime_provider_inbox`
const runtimeProviderReconSelect = `SELECT tenant_id,id,job_run_id,effect_id,provider_id,external_id,request_key,observed_state,response_digest,expected_minor,observed_minor,currency,reason,status,safe_summary,resolved_at,created_at,updated_at,version FROM runtime_provider_reconciliations`
const runtimeProviderBillSelect = `SELECT tenant_id,id,job_run_id,provider_id,bill_id,external_id,effect_id,bill_digest,amount_minor,currency,status,observed_at,created_at,updated_at,version FROM runtime_provider_bills`

func scanRuntimeProviderInbox(row pgx.Row) (domain.ProviderInboxMessage, error) {
	var value domain.ProviderInboxMessage
	var payload []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.ProviderID, &value.MessageID, &value.ReceivedDigest, &value.ExternalID, &value.EffectID, &value.ProviderState, &value.ResponseDigest, &value.CostMinor, &value.Currency, &payload, &value.State, &value.ErrorCode, &value.ReceivedAt, &value.ProcessedAt, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.SafePayload, err = decodeJSON[map[string]any](payload)
	}
	value.NormalizeCollections()
	return value, err
}

func scanRuntimeProviderRecon(row pgx.Row) (domain.ProviderReconciliation, error) {
	var value domain.ProviderReconciliation
	var summary []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.EffectID, &value.ProviderID, &value.ExternalID, &value.RequestKey, &value.ObservedState, &value.ResponseDigest, &value.ExpectedMinor, &value.ObservedMinor, &value.Currency, &value.Reason, &value.Status, &summary, &value.ResolvedAt, &value.CreatedAt, &value.UpdatedAt, &value.Version)
	if err == nil {
		value.SafeSummary, err = decodeJSON[map[string]any](summary)
	}
	value.NormalizeCollections()
	return value, err
}

func scanRuntimeProviderBill(row pgx.Row) (domain.ProviderBillRecord, error) {
	var value domain.ProviderBillRecord
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.ProviderID, &value.BillID, &value.ExternalID, &value.EffectID, &value.BillDigest, &value.AmountMinor, &value.Currency, &value.Status, &value.ObservedAt, &value.CreatedAt, &value.UpdatedAt, &value.Version)
	return value, err
}

func (s *Store) ProviderInboxMessage(ctx context.Context, tenantID, id string) (domain.ProviderInboxMessage, error) {
	var result domain.ProviderInboxMessage
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeProviderInbox(tx.QueryRow(ctx, runtimeProviderInboxSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Provider 回调消息")
		}
		return err
	})
	return result, err
}

func (s *Store) ProviderInboxMessages(ctx context.Context, tenantID, jobID string) ([]domain.ProviderInboxMessage, error) {
	result := []domain.ProviderInboxMessage{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeProviderInboxSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if jobID != "" {
			query += ` AND job_run_id=$2`
			args = append(args, jobID)
		}
		query += ` ORDER BY received_at,id`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeProviderInbox(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) ProviderReconciliations(ctx context.Context, tenantID, effectID string) ([]domain.ProviderReconciliation, error) {
	result := []domain.ProviderReconciliation{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeProviderReconSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if effectID != "" {
			query += ` AND effect_id=$2`
			args = append(args, effectID)
		}
		query += ` ORDER BY created_at,id`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeProviderRecon(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) ProviderReconciliation(ctx context.Context, tenantID, id string) (domain.ProviderReconciliation, error) {
	var result domain.ProviderReconciliation
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeProviderRecon(tx.QueryRow(ctx, runtimeProviderReconSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Provider 对账")
		}
		return err
	})
	return result, err
}

func (s *Store) ProviderBillRecords(ctx context.Context, tenantID, effectID string) ([]domain.ProviderBillRecord, error) {
	result := []domain.ProviderBillRecord{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeProviderBillSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if effectID != "" {
			query += ` AND effect_id=$2`
			args = append(args, effectID)
		}
		query += ` ORDER BY observed_at,id`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeProviderBill(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) ReceiveProviderInboxCommand(ctx context.Context, message domain.ProviderInboxMessage, effect *domain.ExternalEffect, expectedEffectVersion int, reconciliation *domain.ProviderReconciliation, event domain.JobEvent) (domain.ProviderInboxMessage, domain.ExternalEffect, error) {
	message.NormalizeCollections()
	if err := message.Validate(); err != nil {
		return message, domain.ExternalEffect{}, err
	}
	var resultEffect domain.ExternalEffect
	err := s.withTenant(ctx, message.TenantID, func(tx pgx.Tx) error {
		existing, err := scanRuntimeProviderInbox(tx.QueryRow(ctx, runtimeProviderInboxSelect+` WHERE tenant_id=$1 AND provider_id=$2 AND message_id=$3 FOR UPDATE`, message.TenantID, message.ProviderID, message.MessageID))
		if err == nil {
			if existing.ReceivedDigest != message.ReceivedDigest {
				return domain.Conflict("PROVIDER_INBOX_DIGEST_CONFLICT", "相同 Provider 消息 ID 的内容摘要不一致")
			}
			message = existing
			if existing.EffectID != "" {
				value, effectErr := scanRuntimeEffect(tx.QueryRow(ctx, runtimeEffectSelect+` WHERE tenant_id=$1 AND id=$2`, existing.TenantID, existing.EffectID))
				if effectErr != nil {
					return effectErr
				}
				resultEffect = value
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if event.TenantID != message.TenantID || event.JobRunID != message.JobRunID {
			return domain.Invalid("JOB_EVENT_SCOPE_INVALID", "Provider 回调事件不属于当前执行实例")
		}
		if event.ID == "" || event.Type == "" || event.ActorType == "" || event.OccurredAt.IsZero() {
			return domain.Invalid("JOB_EVENT_INVALID", "JobEvent 缺少类型或执行者")
		}
		if effect != nil {
			current, err := scanRuntimeEffect(tx.QueryRow(ctx, runtimeEffectSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, effect.TenantID, effect.ID))
			if err != nil {
				if domain.IsNotFound(err) || errors.Is(err, pgx.ErrNoRows) {
					return domain.NotFound("外部操作")
				}
				return err
			}
			noEffectChange := current.Version == expectedEffectVersion && effect.Version == current.Version && runtimeProviderEffectEqual(current, *effect)
			if !noEffectChange && (current.Version != expectedEffectVersion || effect.Version != expectedEffectVersion+1) {
				return domain.Conflict("EFFECT_VERSION_CONFLICT", "外部操作已被更新，请重新读取")
			}
			if !noEffectChange {
				if err := current.Transition(effect.State); err != nil {
					return err
				}
			}
			if effect.TenantID != message.TenantID || effect.JobRunID != message.JobRunID || effect.ExternalID != message.ExternalID {
				return domain.Invalid("PROVIDER_INBOX_EFFECT_SCOPE_INVALID", "Provider 回调与外部操作范围不一致")
			}
			message.EffectID = effect.ID
			resultEffect = *effect
		}
		if reconciliation != nil {
			reconciliation.NormalizeCollections()
			if err := reconciliation.Validate(); err != nil {
				return err
			}
			if reconciliation.TenantID != message.TenantID || reconciliation.JobRunID != message.JobRunID {
				return domain.Invalid("PROVIDER_RECONCILIATION_SCOPE_INVALID", "Provider 对账不属于当前执行实例")
			}
			if _, err := tx.Exec(ctx, `INSERT INTO runtime_provider_reconciliations(tenant_id,id,job_run_id,effect_id,provider_id,external_id,request_key,observed_state,response_digest,expected_minor,observed_minor,currency,reason,status,safe_summary,resolved_at,created_at,updated_at,version) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) ON CONFLICT (tenant_id,request_key) DO NOTHING`, reconciliation.TenantID, reconciliation.ID, reconciliation.JobRunID, reconciliation.EffectID, reconciliation.ProviderID, reconciliation.ExternalID, reconciliation.RequestKey, reconciliation.ObservedState, reconciliation.ResponseDigest, reconciliation.ExpectedMinor, reconciliation.ObservedMinor, reconciliation.Currency, reconciliation.Reason, reconciliation.Status, jsonValue(reconciliation.SafeSummary), reconciliation.ResolvedAt, reconciliation.CreatedAt, reconciliation.UpdatedAt, reconciliation.Version); err != nil {
				return dbError(err)
			}
		}
		if effect != nil {
			message.State = domain.ProviderInboxApplied
			processed := event.OccurredAt
			message.ProcessedAt = &processed
			if effect.Version != expectedEffectVersion {
				result, err := tx.Exec(ctx, `UPDATE runtime_effects SET state=$3,external_id=$4,response_digest=$5,cost_minor=$6,currency=$7,error_code=$8,version=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2 AND version=$11`, effect.TenantID, effect.ID, effect.State, effect.ExternalID, effect.ResponseDigest, effect.CostMinor, effect.Currency, effect.ErrorCode, effect.Version, effect.UpdatedAt, expectedEffectVersion)
				if err != nil {
					return dbError(err)
				}
				if result.RowsAffected() != 1 {
					return domain.Conflict("EFFECT_VERSION_CONFLICT", "外部操作已被更新，请重新读取")
				}
			}
		} else {
			message.State = domain.ProviderInboxPending
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_provider_inbox(tenant_id,id,job_run_id,provider_id,message_id,received_digest,external_id,effect_id,provider_state,response_digest,cost_minor,currency,safe_payload,state,error_code,received_at,processed_at,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`, message.TenantID, message.ID, message.JobRunID, message.ProviderID, message.MessageID, message.ReceivedDigest, message.ExternalID, message.EffectID, message.ProviderState, message.ResponseDigest, message.CostMinor, message.Currency, jsonValue(message.SafePayload), message.State, message.ErrorCode, message.ReceivedAt, message.ProcessedAt, message.Version, message.CreatedAt, message.UpdatedAt); err != nil {
			return dbError(err)
		}
		event.TenantID, event.JobRunID = message.TenantID, message.JobRunID
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return message, resultEffect, err
}

func runtimeProviderEffectEqual(left, right domain.ExternalEffect) bool {
	return left.ID == right.ID && left.TenantID == right.TenantID && left.JobRunID == right.JobRunID &&
		left.NodeRunID == right.NodeRunID && left.AttemptID == right.AttemptID && left.ResourceReservationID == right.ResourceReservationID &&
		left.Kind == right.Kind && left.IdempotencyKey == right.IdempotencyKey && left.State == right.State &&
		left.ExternalID == right.ExternalID && left.RequestDigest == right.RequestDigest && left.ResponseDigest == right.ResponseDigest &&
		left.CostMinor == right.CostMinor && left.Currency == right.Currency && left.ErrorCode == right.ErrorCode && left.Version == right.Version
}

func (s *Store) RecordProviderBillCommand(ctx context.Context, bill domain.ProviderBillRecord, reconciliation *domain.ProviderReconciliation, event domain.JobEvent) (domain.ProviderBillRecord, error) {
	if err := bill.Validate(); err != nil {
		return bill, err
	}
	err := s.withTenant(ctx, bill.TenantID, func(tx pgx.Tx) error {
		existing, err := scanRuntimeProviderBill(tx.QueryRow(ctx, runtimeProviderBillSelect+` WHERE tenant_id=$1 AND provider_id=$2 AND bill_id=$3 FOR UPDATE`, bill.TenantID, bill.ProviderID, bill.BillID))
		if err == nil {
			if existing.BillDigest != bill.BillDigest {
				return domain.Conflict("PROVIDER_BILL_DIGEST_CONFLICT", "相同账单 ID 的内容摘要不一致")
			}
			bill = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if event.TenantID != bill.TenantID || event.JobRunID != bill.JobRunID {
			return domain.Invalid("JOB_EVENT_SCOPE_INVALID", "Provider 账单事件不属于当前执行实例")
		}
		if reconciliation != nil {
			reconciliation.NormalizeCollections()
			if err := reconciliation.Validate(); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO runtime_provider_reconciliations(tenant_id,id,job_run_id,effect_id,provider_id,external_id,request_key,observed_state,response_digest,expected_minor,observed_minor,currency,reason,status,safe_summary,resolved_at,created_at,updated_at,version) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) ON CONFLICT (tenant_id,request_key) DO NOTHING`, reconciliation.TenantID, reconciliation.ID, reconciliation.JobRunID, reconciliation.EffectID, reconciliation.ProviderID, reconciliation.ExternalID, reconciliation.RequestKey, reconciliation.ObservedState, reconciliation.ResponseDigest, reconciliation.ExpectedMinor, reconciliation.ObservedMinor, reconciliation.Currency, reconciliation.Reason, reconciliation.Status, jsonValue(reconciliation.SafeSummary), reconciliation.ResolvedAt, reconciliation.CreatedAt, reconciliation.UpdatedAt, reconciliation.Version); err != nil {
				return dbError(err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_provider_bills(tenant_id,id,job_run_id,provider_id,bill_id,external_id,effect_id,bill_digest,amount_minor,currency,status,observed_at,created_at,updated_at,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, bill.TenantID, bill.ID, bill.JobRunID, bill.ProviderID, bill.BillID, bill.ExternalID, bill.EffectID, bill.BillDigest, bill.AmountMinor, bill.Currency, bill.Status, bill.ObservedAt, bill.CreatedAt, bill.UpdatedAt, bill.Version); err != nil {
			return dbError(err)
		}
		event.TenantID, event.JobRunID = bill.TenantID, bill.JobRunID
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return bill, err
}

func (s *Store) ResolveProviderReconciliationCommand(ctx context.Context, reconciliation domain.ProviderReconciliation, effect domain.ExternalEffect, expectedEffectVersion int, event domain.JobEvent) (domain.ProviderReconciliation, domain.ExternalEffect, error) {
	reconciliation.NormalizeCollections()
	if err := reconciliation.Validate(); err != nil {
		return reconciliation, effect, err
	}
	var resultRecon domain.ProviderReconciliation
	var resultEffect domain.ExternalEffect
	err := s.withTenant(ctx, reconciliation.TenantID, func(tx pgx.Tx) error {
		currentRecon, err := scanRuntimeProviderRecon(tx.QueryRow(ctx, runtimeProviderReconSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, reconciliation.TenantID, reconciliation.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Provider 对账")
		}
		if err != nil {
			return err
		}
		if currentRecon.Version != reconciliation.Version-1 {
			return domain.Conflict("PROVIDER_RECONCILIATION_VERSION_CONFLICT", "Provider 对账已被更新，请重新读取")
		}
		currentEffect, err := scanRuntimeEffect(tx.QueryRow(ctx, runtimeEffectSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, effect.TenantID, effect.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("外部操作")
		}
		if err != nil {
			return err
		}
		if currentEffect.Version != expectedEffectVersion || effect.Version != expectedEffectVersion+1 {
			return domain.Conflict("EFFECT_VERSION_CONFLICT", "外部操作已被更新，请重新读取")
		}
		if effect.State != currentEffect.State {
			if err := currentEffect.Transition(effect.State); err != nil {
				return err
			}
		}
		if event.TenantID != effect.TenantID || event.JobRunID != effect.JobRunID || event.ID == "" || event.Type == "" || event.ActorType == "" || event.OccurredAt.IsZero() {
			return domain.Invalid("JOB_EVENT_INVALID", "Provider 对账事件不完整或范围不一致")
		}
		if _, err := tx.Exec(ctx, `UPDATE runtime_provider_reconciliations SET status=$3,reason=$4,safe_summary=$5,resolved_at=$6,updated_at=$7,version=$8 WHERE tenant_id=$1 AND id=$2 AND version=$9`, reconciliation.TenantID, reconciliation.ID, reconciliation.Status, reconciliation.Reason, jsonValue(reconciliation.SafeSummary), reconciliation.ResolvedAt, reconciliation.UpdatedAt, reconciliation.Version, currentRecon.Version); err != nil {
			return dbError(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE runtime_effects SET state=$3,external_id=$4,response_digest=$5,cost_minor=$6,currency=$7,error_code=$8,version=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2 AND version=$11`, effect.TenantID, effect.ID, effect.State, effect.ExternalID, effect.ResponseDigest, effect.CostMinor, effect.Currency, effect.ErrorCode, effect.Version, effect.UpdatedAt, expectedEffectVersion); err != nil {
			return dbError(err)
		}
		event.TenantID, event.JobRunID = effect.TenantID, effect.JobRunID
		if _, err := appendRuntimeEventTx(ctx, tx, event); err != nil {
			return err
		}
		resultRecon, resultEffect = reconciliation, effect
		return nil
	})
	return resultRecon, resultEffect, err
}
