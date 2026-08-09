package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const runtimeOutboxSelect = `SELECT tenant_id,id,event_id,schema_version,topic,aggregate_id,payload,attempts,next_attempt_at,locked_by,locked_until,delivered_at,last_error,created_at FROM runtime_outbox`

func scanRuntimeOutbox(row pgx.Row) (domain.RuntimeOutboxMessage, error) {
	var value domain.RuntimeOutboxMessage
	var payload []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.EventID, &value.SchemaVersion, &value.Topic, &value.AggregateID, &payload, &value.Attempts, &value.NextAttemptAt, &value.LockedBy, &value.LockedUntil, &value.DeliveredAt, &value.LastError, &value.CreatedAt)
	if err == nil {
		value.Payload, err = decodeJSON[map[string]any](payload)
	}
	if value.Payload == nil {
		value.Payload = map[string]any{}
	}
	return value, dbError(err)
}

func (s *Store) ClaimReadyNodeCommand(ctx context.Context, tenantID, jobID, owner string, now time.Time, leaseFor time.Duration, event domain.JobEvent) (domain.NodeRun, error) {
	if strings.TrimSpace(owner) == "" || leaseFor <= 0 {
		return domain.NodeRun{}, domain.Invalid("NODE_LEASE_INVALID", "节点租约需要执行者和正数时长")
	}
	var result domain.NodeRun
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeNodeSelect + ` WHERE tenant_id=$1 AND state='ready'`
		args := []any{tenantID}
		if strings.TrimSpace(jobID) != "" {
			query += ` AND job_run_id=$2`
			args = append(args, jobID)
		}
		query += ` ORDER BY ((SELECT priority FROM runtime_job_runs j WHERE j.tenant_id=runtime_node_runs.tenant_id AND j.id=runtime_node_runs.job_run_id) + floor(EXTRACT(EPOCH FROM (now()-updated_at))/60)) DESC, updated_at,created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`
		node, err := scanRuntimeNode(tx.QueryRow(ctx, query, args...))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("可领取的执行节点")
		}
		if err != nil {
			return err
		}
		if err := node.Transition(domain.NodeLeased); err != nil {
			return err
		}
		expires := now.Add(leaseFor)
		fenceToken, _, err := domain.NewOpaqueToken("rtf_", 24)
		if err != nil {
			return err
		}
		node.State, node.AttemptCount, node.LeaseOwner = domain.NodeLeased, node.AttemptCount+1, strings.TrimSpace(owner)
		node.FenceToken = fenceToken
		node.LeaseExpiresAt, node.Version, node.UpdatedAt = &expires, node.Version+1, now
		updated, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$3,attempt_count=$4,lease_owner=$5,fence_token=$6,lease_expires_at=$7,version=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2 AND version=$10`, tenantID, node.ID, node.State, node.AttemptCount, node.LeaseOwner, node.FenceToken, node.LeaseExpiresAt, node.Version, node.UpdatedAt, node.Version-1)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return domain.Conflict("NODE_RUN_VERSION_CONFLICT", "执行节点已被更新，请重新领取")
		}
		event.TenantID, event.JobRunID, event.NodeKey, event.ActorID = tenantID, node.JobRunID, node.NodeKey, strings.TrimSpace(owner)
		if event.Payload == nil {
			event.Payload = map[string]any{}
		}
		event.Payload["attempt_count"], event.Payload["lease_expires_at"] = node.AttemptCount, node.LeaseExpiresAt
		if _, err := appendRuntimeEventTx(ctx, tx, event); err != nil {
			return err
		}
		result = node
		return nil
	})
	return result, err
}

func (s *Store) HeartbeatNodeCommand(ctx context.Context, tenantID, nodeID, owner string, expectedVersion int, now time.Time, leaseFor time.Duration, event domain.JobEvent) (domain.NodeRun, error) {
	if strings.TrimSpace(owner) == "" || leaseFor <= 0 {
		return domain.NodeRun{}, domain.Invalid("NODE_HEARTBEAT_INVALID", "节点心跳需要执行者和正数时长")
	}
	var result domain.NodeRun
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		node, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, nodeID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("执行节点")
		}
		if err != nil {
			return err
		}
		if node.Version != expectedVersion || node.LeaseOwner != strings.TrimSpace(owner) || node.LeaseExpiresAt == nil || !node.LeaseExpiresAt.After(now) || (node.State != domain.NodeLeased && node.State != domain.NodeRunning) {
			return domain.Conflict("NODE_LEASE_STALE", "节点租约无效、已过期或不属于当前执行者")
		}
		if node.State == domain.NodeLeased {
			if err := node.Transition(domain.NodeRunning); err != nil {
				return err
			}
			node.State = domain.NodeRunning
		}
		expires := now.Add(leaseFor)
		node.LeaseExpiresAt, node.Version, node.UpdatedAt = &expires, node.Version+1, now
		updated, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$3,lease_expires_at=$4,version=$5,updated_at=$6 WHERE tenant_id=$1 AND id=$2 AND version=$7 AND lease_owner=$8 AND fence_token <> ''`, tenantID, node.ID, node.State, node.LeaseExpiresAt, node.Version, node.UpdatedAt, expectedVersion, owner)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return domain.Conflict("NODE_LEASE_STALE", "节点租约已经失效，请重新领取")
		}
		event.TenantID, event.JobRunID, event.NodeKey, event.ActorID = tenantID, node.JobRunID, node.NodeKey, strings.TrimSpace(owner)
		if event.Payload == nil {
			event.Payload = map[string]any{}
		}
		event.Payload["state"], event.Payload["lease_expires_at"] = node.State, node.LeaseExpiresAt
		if _, err := appendRuntimeEventTx(ctx, tx, event); err != nil {
			return err
		}
		result = node
		return nil
	})
	return result, err
}

func (s *Store) ApplyJobTransition(ctx context.Context, next domain.JobRun, expectedVersion int, event domain.JobEvent) (domain.JobRun, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	err := s.withTenant(ctx, next.TenantID, func(tx pgx.Tx) error {
		current, err := scanRuntimeJob(tx.QueryRow(ctx, runtimeJobSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("执行实例")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 {
			return domain.Conflict("JOB_RUN_VERSION_CONFLICT", "执行实例已被更新，请重新读取")
		}
		if err := current.Transition(next.State); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE runtime_job_runs SET source_job_run_id=$3,checkpoint_id=$4,state=$5,priority=$6,version=$7,error_code=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2 AND version=$10`, next.TenantID, next.ID, next.SourceJobRunID, next.CheckpointID, next.State, next.Priority, next.Version, next.ErrorCode, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return domain.Conflict("JOB_RUN_VERSION_CONFLICT", "执行实例已被更新，请重新读取")
		}
		event.TenantID, event.JobRunID = next.TenantID, next.ID
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return next, err
}

func (s *Store) ApplyGraphPatchCommand(ctx context.Context, next domain.JobRun, expectedVersion int, plan domain.JobPlanRevision, addedNodes []domain.NodeRun, cancelNodeKeys []string, event domain.JobEvent) (domain.JobRun, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	if err := plan.Validate(); err != nil {
		return next, err
	}
	for _, node := range addedNodes {
		if err := node.Validate(); err != nil {
			return next, err
		}
		if node.TenantID != next.TenantID || node.JobRunID != next.ID {
			return next, domain.Invalid("NODE_RUN_SCOPE_INVALID", "GraphPatch 新节点不属于当前执行实例")
		}
	}
	err := s.withTenant(ctx, next.TenantID, func(tx pgx.Tx) error {
		current, err := scanRuntimeJob(tx.QueryRow(ctx, runtimeJobSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("执行实例")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 || current.PlanRevisionID != plan.BaseRevisionID || next.PlanRevisionID != plan.ID || next.PlanDigest != plan.Digest {
			return domain.Conflict("GRAPH_VERSION_CONFLICT", "执行图版本已经变化，请重新读取后再提交")
		}
		switch current.State {
		case domain.JobRunCompleted, domain.JobRunFailed, domain.JobRunCancelled, domain.JobRunRejected:
			return domain.Conflict("GRAPH_PATCH_JOB_TERMINAL", "终态执行实例不能修改执行图")
		}
		if err := insertRuntimePlanTx(ctx, tx, plan); err != nil {
			return err
		}
		for _, node := range addedNodes {
			if _, err := tx.Exec(ctx, `INSERT INTO runtime_node_runs(tenant_id,id,job_run_id,node_key,state,attempt_count,output_refs,output_digest,error_code,lease_owner,fence_token,lease_expires_at,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, node.TenantID, node.ID, node.JobRunID, node.NodeKey, node.State, node.AttemptCount, jsonArrayValue(node.OutputRefs), node.OutputDigest, node.ErrorCode, node.LeaseOwner, node.FenceToken, node.LeaseExpiresAt, node.Version, node.CreatedAt, node.UpdatedAt); err != nil {
				return dbError(err)
			}
		}
		for _, nodeKey := range cancelNodeKeys {
			var node domain.NodeRun
			node, err = scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND job_run_id=$2 AND node_key=$3 FOR UPDATE`, next.TenantID, next.ID, nodeKey))
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("GraphPatch 待取消节点")
			}
			if err != nil {
				return err
			}
			if node.State != domain.NodePending && node.State != domain.NodeReady && node.State != domain.NodeWaitingResource {
				return domain.Conflict("GRAPH_PATCH_CANCEL_CONFLICT", "GraphPatch 只能取消尚未执行的节点")
			}
			if _, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$4,version=version+1,updated_at=$5 WHERE tenant_id=$1 AND job_run_id=$2 AND node_key=$3`, next.TenantID, next.ID, nodeKey, domain.NodeCancelled, event.OccurredAt); err != nil {
				return dbError(err)
			}
		}
		updated, err := tx.Exec(ctx, `UPDATE runtime_job_runs SET plan_revision_id=$3,plan_digest=$4,state=$5,priority=$6,version=$7,error_code=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2 AND version=$10`, next.TenantID, next.ID, next.PlanRevisionID, next.PlanDigest, next.State, next.Priority, next.Version, next.ErrorCode, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return domain.Conflict("GRAPH_VERSION_CONFLICT", "执行图版本已经变化，请重新读取后再提交")
		}
		event.TenantID, event.JobRunID = next.TenantID, next.ID
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return next, err
}

func (s *Store) ApplyNodeTransition(ctx context.Context, next domain.NodeRun, expectedVersion int, event domain.JobEvent) (domain.NodeRun, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	err := s.withTenant(ctx, next.TenantID, func(tx pgx.Tx) error {
		current, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("执行节点")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 {
			return domain.Conflict("NODE_RUN_VERSION_CONFLICT", "执行节点已被更新，请重新读取")
		}
		if current.JobRunID != next.JobRunID {
			return domain.Invalid("NODE_RUN_SCOPE_INVALID", "执行节点不属于当前执行实例")
		}
		if err := current.Transition(next.State); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$3,attempt_count=$4,output_refs=$5,output_digest=$6,error_code=$7,lease_owner=$8,fence_token=$9,lease_expires_at=$10,version=$11,updated_at=$12 WHERE tenant_id=$1 AND id=$2 AND version=$13`, next.TenantID, next.ID, next.State, next.AttemptCount, jsonArrayValue(next.OutputRefs), next.OutputDigest, next.ErrorCode, next.LeaseOwner, next.FenceToken, next.LeaseExpiresAt, next.Version, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return domain.Conflict("NODE_RUN_VERSION_CONFLICT", "执行节点已被更新，请重新读取")
		}
		event.TenantID, event.JobRunID, event.NodeKey = next.TenantID, next.JobRunID, next.NodeKey
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return next, err
}

func (s *Store) ApplyStateMutation(ctx context.Context, tenantID, jobID string, mutation domain.StateMutation, event domain.JobEvent) (domain.RuntimeState, error) {
	if strings.TrimSpace(mutation.Collection) == "" || strings.TrimSpace(mutation.IdempotencyKey) == "" {
		return domain.RuntimeState{}, domain.Invalid("RUNTIME_STATE_MUTATION_INVALID", "状态变更需要集合名和幂等键")
	}
	var state domain.RuntimeState
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var err error
		state, err = scanRuntimeState(tx.QueryRow(ctx, runtimeStateSelect+` WHERE tenant_id=$1 AND job_run_id=$2 AND collection=$3 FOR UPDATE`, tenantID, jobID, mutation.Collection))
		if errors.Is(err, pgx.ErrNoRows) {
			state = domain.RuntimeState{ID: domain.NewID(), TenantID: tenantID, JobRunID: jobID, Collection: mutation.Collection, SchemaVersion: domain.RuntimeStateSchema, Values: map[string]any{}, Revision: 0, UpdatedAt: event.OccurredAt}
			err = nil
		}
		if err != nil {
			return err
		}
		var marker int
		err = tx.QueryRow(ctx, `INSERT INTO runtime_state_mutations(tenant_id,job_run_id,collection,idempotency_key) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING RETURNING 1`, tenantID, jobID, mutation.Collection, mutation.IdempotencyKey).Scan(&marker)
		if errors.Is(err, pgx.ErrNoRows) {
			state, err = scanRuntimeState(tx.QueryRow(ctx, runtimeStateSelect+` WHERE tenant_id=$1 AND job_run_id=$2 AND collection=$3`, tenantID, jobID, mutation.Collection))
			return err
		}
		if err != nil {
			return dbError(err)
		}
		if state.Revision != mutation.ExpectedRevision {
			return domain.Conflict("RUNTIME_STATE_CAS_CONFLICT", "运行状态已经更新，请重新读取")
		}
		if state.Values == nil {
			state.Values = map[string]any{}
		}
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
		if state.ID == "" {
			state.ID = domain.NewID()
		}
		_, err = tx.Exec(ctx, `INSERT INTO runtime_states(tenant_id,id,job_run_id,collection,schema_version,revision,values,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(tenant_id,job_run_id,collection) DO UPDATE SET schema_version=EXCLUDED.schema_version,revision=EXCLUDED.revision,values=EXCLUDED.values,updated_at=EXCLUDED.updated_at WHERE runtime_states.revision=$9`, tenantID, state.ID, jobID, mutation.Collection, state.SchemaVersion, state.Revision, jsonValue(state.Values), state.UpdatedAt, mutation.ExpectedRevision)
		if err != nil {
			return dbError(err)
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return state, err
}

func (s *Store) RegisterEffectCommand(ctx context.Context, effect domain.ExternalEffect, event domain.JobEvent) (domain.ExternalEffect, error) {
	if effect.ID == "" || effect.TenantID == "" || effect.JobRunID == "" || effect.Kind == "" || effect.IdempotencyKey == "" || effect.RequestDigest == "" || effect.Version < 1 || effect.CreatedAt.IsZero() || effect.UpdatedAt.IsZero() {
		return effect, domain.Invalid("EFFECT_INVALID", "外部操作缺少执行引用、幂等键或请求摘要")
	}
	err := s.withTenant(ctx, effect.TenantID, func(tx pgx.Tx) error {
		existing, err := scanRuntimeEffect(tx.QueryRow(ctx, runtimeEffectSelect+` WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, effect.TenantID, effect.IdempotencyKey))
		if err == nil {
			effect = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_effects(tenant_id,id,job_run_id,node_run_id,attempt_id,resource_reservation_id,kind,idempotency_key,state,external_id,request_digest,response_digest,cost_minor,currency,safe_summary,error_code,version,created_at,updated_at) VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, effect.TenantID, effect.ID, effect.JobRunID, effect.NodeRunID, effect.AttemptID, effect.ResourceReservationID, effect.Kind, effect.IdempotencyKey, effect.State, effect.ExternalID, effect.RequestDigest, effect.ResponseDigest, effect.CostMinor, effect.Currency, jsonValue(effect.SafeSummary), effect.ErrorCode, effect.Version, effect.CreatedAt, effect.UpdatedAt); err != nil {
			return dbError(err)
		}
		event.TenantID, event.JobRunID = effect.TenantID, effect.JobRunID
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return effect, err
}

func (s *Store) ApplyEffectTransition(ctx context.Context, next domain.ExternalEffect, expectedVersion int, event domain.JobEvent) (domain.ExternalEffect, error) {
	err := s.withTenant(ctx, next.TenantID, func(tx pgx.Tx) error {
		current, err := scanRuntimeEffect(tx.QueryRow(ctx, runtimeEffectSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("外部操作")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 {
			return domain.Conflict("EFFECT_VERSION_CONFLICT", "外部操作已被更新，请重新读取")
		}
		if err := current.Transition(next.State); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE runtime_effects SET state=$3,external_id=$4,response_digest=$5,error_code=$6,version=$7,updated_at=$8 WHERE tenant_id=$1 AND id=$2 AND version=$9`, next.TenantID, next.ID, next.State, next.ExternalID, next.ResponseDigest, next.ErrorCode, next.Version, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return domain.Conflict("EFFECT_VERSION_CONFLICT", "外部操作已被更新，请重新读取")
		}
		event.TenantID, event.JobRunID = next.TenantID, next.JobRunID
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return next, err
}

func (s *Store) RuntimeOutboxMessages(ctx context.Context, tenantID string, now time.Time, limit int) ([]domain.RuntimeOutboxMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	result := []domain.RuntimeOutboxMessage{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeOutboxSelect+` WHERE tenant_id=$1 AND delivered_at IS NULL AND next_attempt_at<=$2 AND (locked_until IS NULL OR locked_until<=$2) ORDER BY next_attempt_at,id LIMIT $3`, tenantID, now, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeOutbox(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) ClaimRuntimeOutbox(ctx context.Context, tenantID, consumer string, now time.Time, leaseFor time.Duration, limit int) ([]domain.RuntimeOutboxMessage, error) {
	consumer = strings.TrimSpace(consumer)
	if consumer == "" || leaseFor <= 0 {
		return nil, domain.Invalid("OUTBOX_CLAIM_INVALID", "outbox 认领需要消费者和正数租约")
	}
	if limit <= 0 {
		limit = 100
	}
	lockedUntil := now.Add(leaseFor)
	result := []domain.RuntimeOutboxMessage{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeOutboxSelect+` WHERE tenant_id=$1 AND delivered_at IS NULL AND next_attempt_at<=$2 AND (locked_until IS NULL OR locked_until<=$2) ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT $3`, tenantID, now, limit)
		if err != nil {
			return dbError(err)
		}
		defer rows.Close()
		candidates := []domain.RuntimeOutboxMessage{}
		for rows.Next() {
			value, err := scanRuntimeOutbox(rows)
			if err != nil {
				return err
			}
			candidates = append(candidates, value)
		}
		if err := rows.Err(); err != nil {
			return dbError(err)
		}
		for _, candidate := range candidates {
			updated, err := tx.Exec(ctx, `UPDATE runtime_outbox SET locked_by=$3,locked_until=$4,attempts=attempts+1 WHERE tenant_id=$1 AND id=$2 AND delivered_at IS NULL AND (locked_until IS NULL OR locked_until<=$5)`, tenantID, candidate.ID, consumer, lockedUntil, now)
			if err != nil {
				return dbError(err)
			}
			if updated.RowsAffected() != 1 {
				continue
			}
			candidate.LockedBy = consumer
			candidate.LockedUntil = &lockedUntil
			candidate.Attempts++
			result = append(result, candidate)
		}
		return nil
	})
	return result, err
}

func (s *Store) AckRuntimeOutbox(ctx context.Context, tenantID, messageID, consumer string, deliveredAt time.Time) error {
	consumer = strings.TrimSpace(consumer)
	if consumer == "" || deliveredAt.IsZero() {
		return domain.Invalid("OUTBOX_ACK_INVALID", "outbox 确认需要消费者和确认时间")
	}
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE runtime_outbox SET delivered_at=$4,locked_by='',locked_until=NULL,last_error='' WHERE tenant_id=$1 AND id=$2 AND delivered_at IS NULL AND locked_by=$3 AND locked_until>$4`, tenantID, messageID, consumer, deliveredAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return domain.Conflict("OUTBOX_LEASE_STALE", "outbox 消息不属于当前消费者或租约已过期")
		}
		return nil
	})
}

func (s *Store) RetryRuntimeOutbox(ctx context.Context, tenantID, messageID, consumer string, now, nextAttemptAt time.Time, lastError string) error {
	consumer = strings.TrimSpace(consumer)
	if consumer == "" || now.IsZero() || nextAttemptAt.IsZero() || strings.TrimSpace(lastError) == "" {
		return domain.Invalid("OUTBOX_RETRY_INVALID", "outbox 重试需要消费者、时间和错误原因")
	}
	if nextAttemptAt.Before(now) {
		return domain.Invalid("OUTBOX_RETRY_INVALID", "outbox 下次尝试时间不能早于当前时间")
	}
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE runtime_outbox SET next_attempt_at=$4,locked_by='',locked_until=NULL,last_error=$5 WHERE tenant_id=$1 AND id=$2 AND delivered_at IS NULL AND locked_by=$3 AND locked_until>$6`, tenantID, messageID, consumer, nextAttemptAt, strings.TrimSpace(lastError), now)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return domain.Conflict("OUTBOX_LEASE_STALE", "outbox 消息不属于当前消费者或租约已过期")
		}
		return nil
	})
}
