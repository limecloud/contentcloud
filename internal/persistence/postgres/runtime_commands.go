package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	"github.com/jackc/pgx/v5"
)

func (s *Store) AppendFencedRuntimeEvent(ctx context.Context, tenantID, attemptID, owner, fenceToken string, now time.Time, event contentruntime.JobEvent) (contentruntime.JobEvent, error) {
	result := event
	err := s.withTenantCommand(ctx, tenantID, "runtime.append_fenced_event", func(tx pgx.Tx) error {
		var jobRunID, nodeKey, state, storedOwner, storedFence, harnessKind string
		var leaseExpiresAt *time.Time
		err := tx.QueryRow(ctx, `SELECT attempt.job_run_id,node.node_key,attempt.state,attempt.lease_owner,attempt.fence_token,attempt.lease_expires_at,attempt.harness_kind
			FROM runtime_attempts attempt
			JOIN runtime_node_runs node ON node.tenant_id=attempt.tenant_id AND node.id=attempt.node_run_id
			WHERE attempt.tenant_id=$1 AND attempt.id=$2
			FOR UPDATE OF attempt`, tenantID, attemptID).Scan(&jobRunID, &nodeKey, &state, &storedOwner, &storedFence, &leaseExpiresAt, &harnessKind)
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("RuntimeAttempt")
		}
		if err != nil {
			return err
		}
		if state != contentruntime.RuntimeAttemptRunning || storedOwner != strings.TrimSpace(owner) || storedFence == "" || storedFence != fenceToken || leaseExpiresAt == nil || !leaseExpiresAt.After(now) {
			return fault.Conflict("DISPATCH_FENCE_STALE", "Harness 事件的执行围栏无效或已过期")
		}
		event.TenantID, event.JobRunID, event.NodeKey, event.ActorType, event.ActorID = tenantID, jobRunID, nodeKey, "harness", harnessKind
		result, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return result, dbError(err)
}

const runtimeOutboxSelect = `SELECT m.tenant_id,m.id,m.event_id,m.schema_version,m.topic,m.aggregate_id,m.payload,r.subscriber,r.attempts,r.next_attempt_at,r.locked_by,r.locked_until,r.delivered_at,r.last_error,m.created_at FROM runtime_outbox m JOIN runtime_outbox_receipts r ON r.tenant_id=m.tenant_id AND r.message_id=m.id`

func scanRuntimeOutbox(row pgx.Row) (contentruntime.RuntimeOutboxMessage, error) {
	var value contentruntime.RuntimeOutboxMessage
	var payload []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.EventID, &value.SchemaVersion, &value.Topic, &value.AggregateID, &payload, &value.Subscriber, &value.Attempts, &value.NextAttemptAt, &value.LockedBy, &value.LockedUntil, &value.DeliveredAt, &value.LastError, &value.CreatedAt)
	if err == nil {
		value.Payload, err = decodeJSON[map[string]any](payload)
	}
	if value.Payload == nil {
		value.Payload = map[string]any{}
	}
	return value, dbError(err)
}

func (s *Store) ClaimReadyNodeCommand(ctx context.Context, tenantID, jobID, owner string, now time.Time, leaseFor time.Duration, event contentruntime.JobEvent) (contentruntime.NodeRun, error) {
	if strings.TrimSpace(owner) == "" || leaseFor <= 0 {
		return contentruntime.NodeRun{}, fault.Invalid("NODE_LEASE_INVALID", "节点租约需要执行者和正数时长")
	}
	var result contentruntime.NodeRun
	err := s.withTenantCommand(ctx, tenantID, "runtime.claim_ready_node", func(tx pgx.Tx) error {
		query := runtimeNodeSelect + ` WHERE tenant_id=$1 AND state='ready'`
		args := []any{tenantID}
		if strings.TrimSpace(jobID) != "" {
			query += ` AND job_run_id=$2`
			args = append(args, jobID)
		}
		query += ` ORDER BY ((SELECT priority FROM runtime_job_runs j WHERE j.tenant_id=runtime_node_runs.tenant_id AND j.id=runtime_node_runs.job_run_id) + floor(EXTRACT(EPOCH FROM (now()-updated_at))/60)) DESC, updated_at,created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`
		node, err := scanRuntimeNode(tx.QueryRow(ctx, query, args...))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("可领取的执行节点")
		}
		if err != nil {
			return err
		}
		if err := node.Transition(contentruntime.NodeLeased); err != nil {
			return err
		}
		expires := now.Add(leaseFor)
		fenceToken, _, err := idgen.NewOpaqueToken("rtf_", 24)
		if err != nil {
			return err
		}
		node.State, node.AttemptCount, node.LeaseOwner = contentruntime.NodeLeased, node.AttemptCount+1, strings.TrimSpace(owner)
		node.FenceToken = fenceToken
		node.LeaseExpiresAt, node.Version, node.UpdatedAt = &expires, node.Version+1, now
		updated, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$3,attempt_count=$4,lease_owner=$5,fence_token=$6,lease_expires_at=$7,version=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2 AND version=$10`, tenantID, node.ID, node.State, node.AttemptCount, node.LeaseOwner, node.FenceToken, node.LeaseExpiresAt, node.Version, node.UpdatedAt, node.Version-1)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return fault.Conflict("NODE_RUN_VERSION_CONFLICT", "执行节点已被更新，请重新领取")
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

func (s *Store) HeartbeatNodeCommand(ctx context.Context, tenantID, nodeID, owner string, expectedVersion int, now time.Time, leaseFor time.Duration, event contentruntime.JobEvent) (contentruntime.NodeRun, error) {
	if strings.TrimSpace(owner) == "" || leaseFor <= 0 {
		return contentruntime.NodeRun{}, fault.Invalid("NODE_HEARTBEAT_INVALID", "节点心跳需要执行者和正数时长")
	}
	var result contentruntime.NodeRun
	err := s.withTenantCommand(ctx, tenantID, "runtime.heartbeat_node", func(tx pgx.Tx) error {
		node, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, nodeID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("执行节点")
		}
		if err != nil {
			return err
		}
		if node.Version != expectedVersion || node.LeaseOwner != strings.TrimSpace(owner) || node.LeaseExpiresAt == nil || !node.LeaseExpiresAt.After(now) || (node.State != contentruntime.NodeLeased && node.State != contentruntime.NodeRunning) {
			return fault.Conflict("NODE_LEASE_STALE", "节点租约无效、已过期或不属于当前执行者")
		}
		if node.State == contentruntime.NodeLeased {
			if err := node.Transition(contentruntime.NodeRunning); err != nil {
				return err
			}
			node.State = contentruntime.NodeRunning
		}
		expires := now.Add(leaseFor)
		node.LeaseExpiresAt, node.Version, node.UpdatedAt = &expires, node.Version+1, now
		updated, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$3,lease_expires_at=$4,version=$5,updated_at=$6 WHERE tenant_id=$1 AND id=$2 AND version=$7 AND lease_owner=$8 AND fence_token <> ''`, tenantID, node.ID, node.State, node.LeaseExpiresAt, node.Version, node.UpdatedAt, expectedVersion, owner)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return fault.Conflict("NODE_LEASE_STALE", "节点租约已经失效，请重新领取")
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

func (s *Store) ApplyJobTransition(ctx context.Context, next contentruntime.JobRun, expectedVersion int, event contentruntime.JobEvent) (contentruntime.JobRun, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	err := s.withTenantCommand(ctx, next.TenantID, "runtime.apply_job_transition", func(tx pgx.Tx) error {
		current, err := scanRuntimeJob(tx.QueryRow(ctx, runtimeJobSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("执行实例")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 {
			return fault.Conflict("JOB_RUN_VERSION_CONFLICT", "执行实例已被更新，请重新读取")
		}
		if err := current.Transition(next.State); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE runtime_job_runs SET source_job_run_id=$3,checkpoint_id=$4,state=$5,priority=$6,version=$7,error_code=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2 AND version=$10`, next.TenantID, next.ID, next.SourceJobRunID, next.CheckpointID, next.State, next.Priority, next.Version, next.ErrorCode, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return fault.Conflict("JOB_RUN_VERSION_CONFLICT", "执行实例已被更新，请重新读取")
		}
		event.TenantID, event.JobRunID = next.TenantID, next.ID
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return next, err
}

func (s *Store) ApplyGraphPatchCommand(ctx context.Context, next contentruntime.JobRun, expectedVersion int, plan contentruntime.JobPlanRevision, addedNodes []contentruntime.NodeRun, cancelNodeKeys []string, event contentruntime.JobEvent) (contentruntime.JobRun, error) {
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
			return next, fault.Invalid("NODE_RUN_SCOPE_INVALID", "GraphPatch 新节点不属于当前执行实例")
		}
	}
	err := s.withTenantCommand(ctx, next.TenantID, "runtime.apply_graph_patch", func(tx pgx.Tx) error {
		current, err := scanRuntimeJob(tx.QueryRow(ctx, runtimeJobSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("执行实例")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 || current.PlanRevisionID != plan.BaseRevisionID || next.PlanRevisionID != plan.ID || next.PlanDigest != plan.Digest {
			return fault.Conflict("GRAPH_VERSION_CONFLICT", "执行图版本已经变化，请重新读取后再提交")
		}
		switch current.State {
		case contentruntime.JobRunCompleted, contentruntime.JobRunFailed, contentruntime.JobRunCancelled, contentruntime.JobRunRejected:
			return fault.Conflict("GRAPH_PATCH_JOB_TERMINAL", "终态执行实例不能修改执行图")
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
			var node contentruntime.NodeRun
			node, err = scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND job_run_id=$2 AND node_key=$3 FOR UPDATE`, next.TenantID, next.ID, nodeKey))
			if errors.Is(err, pgx.ErrNoRows) {
				return fault.NotFound("GraphPatch 待取消节点")
			}
			if err != nil {
				return err
			}
			if node.State != contentruntime.NodePending && node.State != contentruntime.NodeReady && node.State != contentruntime.NodeWaitingResource {
				return fault.Conflict("GRAPH_PATCH_CANCEL_CONFLICT", "GraphPatch 只能取消尚未执行的节点")
			}
			if _, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$4,version=version+1,updated_at=$5 WHERE tenant_id=$1 AND job_run_id=$2 AND node_key=$3`, next.TenantID, next.ID, nodeKey, contentruntime.NodeCancelled, event.OccurredAt); err != nil {
				return dbError(err)
			}
		}
		updated, err := tx.Exec(ctx, `UPDATE runtime_job_runs SET plan_revision_id=$3,plan_digest=$4,state=$5,priority=$6,version=$7,error_code=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2 AND version=$10`, next.TenantID, next.ID, next.PlanRevisionID, next.PlanDigest, next.State, next.Priority, next.Version, next.ErrorCode, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return fault.Conflict("GRAPH_VERSION_CONFLICT", "执行图版本已经变化，请重新读取后再提交")
		}
		event.TenantID, event.JobRunID = next.TenantID, next.ID
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return next, err
}

func (s *Store) ApplyNodeTransition(ctx context.Context, next contentruntime.NodeRun, expectedVersion int, event contentruntime.JobEvent) (contentruntime.NodeRun, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	err := s.withTenantCommand(ctx, next.TenantID, "runtime.apply_node_transition", func(tx pgx.Tx) error {
		current, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("执行节点")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 {
			return fault.Conflict("NODE_RUN_VERSION_CONFLICT", "执行节点已被更新，请重新读取")
		}
		if current.JobRunID != next.JobRunID {
			return fault.Invalid("NODE_RUN_SCOPE_INVALID", "执行节点不属于当前执行实例")
		}
		if err := current.Transition(next.State); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$3,attempt_count=$4,output_refs=$5,output_digest=$6,error_code=$7,lease_owner=$8,fence_token=$9,lease_expires_at=$10,version=$11,updated_at=$12 WHERE tenant_id=$1 AND id=$2 AND version=$13`, next.TenantID, next.ID, next.State, next.AttemptCount, jsonArrayValue(next.OutputRefs), next.OutputDigest, next.ErrorCode, next.LeaseOwner, next.FenceToken, next.LeaseExpiresAt, next.Version, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return fault.Conflict("NODE_RUN_VERSION_CONFLICT", "执行节点已被更新，请重新读取")
		}
		event.TenantID, event.JobRunID, event.NodeKey = next.TenantID, next.JobRunID, next.NodeKey
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return next, err
}

func (s *Store) ApplyStateMutation(ctx context.Context, tenantID, jobID string, mutation contentruntime.StateMutation, event contentruntime.JobEvent) (contentruntime.RuntimeState, error) {
	if strings.TrimSpace(mutation.Collection) == "" || strings.TrimSpace(mutation.IdempotencyKey) == "" {
		return contentruntime.RuntimeState{}, fault.Invalid("RUNTIME_STATE_MUTATION_INVALID", "状态变更需要集合名和幂等键")
	}
	var state contentruntime.RuntimeState
	err := s.withTenantCommand(ctx, tenantID, "runtime.apply_state_mutation", func(tx pgx.Tx) error {
		var err error
		state, err = scanRuntimeState(tx.QueryRow(ctx, runtimeStateSelect+` WHERE tenant_id=$1 AND job_run_id=$2 AND collection=$3 FOR UPDATE`, tenantID, jobID, mutation.Collection))
		if errors.Is(err, pgx.ErrNoRows) {
			state = contentruntime.RuntimeState{ID: idgen.New(), TenantID: tenantID, JobRunID: jobID, Collection: mutation.Collection, SchemaVersion: contentruntime.RuntimeStateSchema, Values: map[string]any{}, Revision: 0, UpdatedAt: event.OccurredAt}
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
			return fault.Conflict("RUNTIME_STATE_CAS_CONFLICT", "运行状态已经更新，请重新读取")
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
			state.ID = idgen.New()
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

func (s *Store) RegisterEffectCommand(ctx context.Context, effect contentruntime.ExternalEffect, event contentruntime.JobEvent) (contentruntime.ExternalEffect, error) {
	if effect.ID == "" || effect.TenantID == "" || effect.JobRunID == "" || effect.Kind == "" || effect.IdempotencyKey == "" || effect.RequestDigest == "" || effect.Version < 1 || effect.CreatedAt.IsZero() || effect.UpdatedAt.IsZero() {
		return effect, fault.Invalid("EFFECT_INVALID", "外部操作缺少执行引用、幂等键或请求摘要")
	}
	err := s.withTenantCommand(ctx, effect.TenantID, "runtime.register_effect", func(tx pgx.Tx) error {
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

func (s *Store) RegisterFencedEffectCommand(ctx context.Context, effect contentruntime.ExternalEffect, fenceToken string, now time.Time, event contentruntime.JobEvent) (contentruntime.ExternalEffect, error) {
	if effect.ID == "" || effect.TenantID == "" || effect.JobRunID == "" || effect.AttemptID == "" || effect.Kind == "" || effect.IdempotencyKey == "" || effect.RequestDigest == "" || effect.Version < 1 || effect.CreatedAt.IsZero() || effect.UpdatedAt.IsZero() {
		return effect, fault.Invalid("EFFECT_INVALID", "外部操作缺少 Attempt、幂等键或请求摘要")
	}
	err := s.withTenantCommand(ctx, effect.TenantID, "runtime.register_fenced_effect", func(tx pgx.Tx) error {
		if err := validateAttemptFenceTx(ctx, tx, effect.TenantID, effect.AttemptID, fenceToken, now); err != nil {
			return err
		}
		existing, err := scanRuntimeEffect(tx.QueryRow(ctx, runtimeEffectSelect+` WHERE tenant_id=$1 AND idempotency_key=$2 FOR UPDATE`, effect.TenantID, effect.IdempotencyKey))
		if err == nil {
			if existing.AttemptID != effect.AttemptID || existing.RequestDigest != effect.RequestDigest {
				return fault.Conflict("EFFECT_IDEMPOTENCY_MISMATCH", "Effect 幂等键已用于不同 Attempt 或请求摘要")
			}
			effect = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_effects(tenant_id,id,job_run_id,node_run_id,attempt_id,resource_reservation_id,kind,idempotency_key,state,external_id,request_digest,response_digest,cost_minor,currency,safe_summary,error_code,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, effect.TenantID, effect.ID, effect.JobRunID, effect.NodeRunID, effect.AttemptID, effect.ResourceReservationID, effect.Kind, effect.IdempotencyKey, effect.State, effect.ExternalID, effect.RequestDigest, effect.ResponseDigest, effect.CostMinor, effect.Currency, jsonValue(effect.SafeSummary), effect.ErrorCode, effect.Version, effect.CreatedAt, effect.UpdatedAt); err != nil {
			return dbError(err)
		}
		event.TenantID, event.JobRunID = effect.TenantID, effect.JobRunID
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return effect, err
}

func (s *Store) ApplyEffectTransition(ctx context.Context, next contentruntime.ExternalEffect, expectedVersion int, event contentruntime.JobEvent) (contentruntime.ExternalEffect, error) {
	err := s.withTenantCommand(ctx, next.TenantID, "runtime.apply_effect_transition", func(tx pgx.Tx) error {
		current, err := scanRuntimeEffect(tx.QueryRow(ctx, runtimeEffectSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("外部操作")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 {
			return fault.Conflict("EFFECT_VERSION_CONFLICT", "外部操作已被更新，请重新读取")
		}
		if err := current.Transition(next.State); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE runtime_effects SET state=$3,external_id=$4,response_digest=$5,error_code=$6,version=$7,updated_at=$8 WHERE tenant_id=$1 AND id=$2 AND version=$9`, next.TenantID, next.ID, next.State, next.ExternalID, next.ResponseDigest, next.ErrorCode, next.Version, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return fault.Conflict("EFFECT_VERSION_CONFLICT", "外部操作已被更新，请重新读取")
		}
		event.TenantID, event.JobRunID = next.TenantID, next.JobRunID
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return next, err
}

func (s *Store) RuntimeOutboxMessages(ctx context.Context, tenantID, subscriber string, now time.Time, limit int) ([]contentruntime.RuntimeOutboxMessage, error) {
	subscriber = strings.TrimSpace(subscriber)
	if subscriber == "" {
		return nil, fault.Invalid("OUTBOX_SUBSCRIBER_REQUIRED", "outbox 查询需要订阅者")
	}
	if limit <= 0 {
		limit = 100
	}
	result := []contentruntime.RuntimeOutboxMessage{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeOutboxSelect+` WHERE m.tenant_id=$1 AND r.subscriber=$2 AND r.delivered_at IS NULL AND r.next_attempt_at<=$3 AND (r.locked_until IS NULL OR r.locked_until<=$3) ORDER BY r.next_attempt_at,m.id LIMIT $4`, tenantID, subscriber, now, limit)
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

func (s *Store) ClaimRuntimeOutbox(ctx context.Context, tenantID, subscriber, worker string, now time.Time, leaseFor time.Duration, limit int) ([]contentruntime.RuntimeOutboxMessage, error) {
	subscriber, worker = strings.TrimSpace(subscriber), strings.TrimSpace(worker)
	if subscriber == "" || worker == "" || leaseFor <= 0 {
		return nil, fault.Invalid("OUTBOX_CLAIM_INVALID", "outbox 认领需要订阅者、工作器和正数租约")
	}
	if limit <= 0 {
		limit = 100
	}
	lockedUntil := now.Add(leaseFor)
	result := []contentruntime.RuntimeOutboxMessage{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeOutboxSelect+` WHERE m.tenant_id=$1 AND r.subscriber=$2 AND r.delivered_at IS NULL AND r.next_attempt_at<=$3 AND (r.locked_until IS NULL OR r.locked_until<=$3) ORDER BY r.next_attempt_at,m.id FOR UPDATE OF r SKIP LOCKED LIMIT $4`, tenantID, subscriber, now, limit)
		if err != nil {
			return dbError(err)
		}
		defer rows.Close()
		candidates := []contentruntime.RuntimeOutboxMessage{}
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
			updated, err := tx.Exec(ctx, `UPDATE runtime_outbox_receipts SET locked_by=$4,locked_until=$5,attempts=attempts+1 WHERE tenant_id=$1 AND message_id=$2 AND subscriber=$3 AND delivered_at IS NULL AND (locked_until IS NULL OR locked_until<=$6)`, tenantID, candidate.ID, subscriber, worker, lockedUntil, now)
			if err != nil {
				return dbError(err)
			}
			if updated.RowsAffected() != 1 {
				continue
			}
			candidate.LockedBy = worker
			candidate.LockedUntil = &lockedUntil
			candidate.Attempts++
			result = append(result, candidate)
		}
		return nil
	})
	return result, err
}

func (s *Store) AckRuntimeOutbox(ctx context.Context, tenantID, messageID, subscriber, worker string, deliveredAt time.Time) error {
	subscriber, worker = strings.TrimSpace(subscriber), strings.TrimSpace(worker)
	if subscriber == "" || worker == "" || deliveredAt.IsZero() {
		return fault.Invalid("OUTBOX_ACK_INVALID", "outbox 确认需要订阅者、工作器和确认时间")
	}
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE runtime_outbox_receipts SET delivered_at=$5,locked_by='',locked_until=NULL,last_error='' WHERE tenant_id=$1 AND message_id=$2 AND subscriber=$3 AND delivered_at IS NULL AND locked_by=$4 AND locked_until>$5`, tenantID, messageID, subscriber, worker, deliveredAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return fault.Conflict("OUTBOX_LEASE_STALE", "outbox 消息不属于当前消费者或租约已过期")
		}
		return nil
	})
}

func (s *Store) RetryRuntimeOutbox(ctx context.Context, tenantID, messageID, subscriber, worker string, now, nextAttemptAt time.Time, lastError string) error {
	subscriber, worker = strings.TrimSpace(subscriber), strings.TrimSpace(worker)
	if subscriber == "" || worker == "" || now.IsZero() || nextAttemptAt.IsZero() || strings.TrimSpace(lastError) == "" {
		return fault.Invalid("OUTBOX_RETRY_INVALID", "outbox 重试需要订阅者、工作器、时间和错误原因")
	}
	if nextAttemptAt.Before(now) {
		return fault.Invalid("OUTBOX_RETRY_INVALID", "outbox 下次尝试时间不能早于当前时间")
	}
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE runtime_outbox_receipts SET next_attempt_at=$5,locked_by='',locked_until=NULL,last_error=$6 WHERE tenant_id=$1 AND message_id=$2 AND subscriber=$3 AND delivered_at IS NULL AND locked_by=$4 AND locked_until>$7`, tenantID, messageID, subscriber, worker, nextAttemptAt, strings.TrimSpace(lastError), now)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return fault.Conflict("OUTBOX_LEASE_STALE", "outbox 消息不属于当前消费者或租约已过期")
		}
		return nil
	})
}
