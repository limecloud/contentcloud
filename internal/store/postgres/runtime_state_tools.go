package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const runtimeStateCollectionSelect = `SELECT tenant_id,id,job_run_id,collection_key,scope,schema_id,schema_revision,consistency,writer_node_key,max_record_bytes,max_records,retention_policy,read_policy,write_policy,revision,watermark,created_at,updated_at FROM runtime_state_collections`
const runtimeStateRecordSelect = `SELECT tenant_id,id,collection_id,key,value,artifact_ref,schema_revision,version,digest,created_by,updated_by,created_at,updated_at FROM runtime_state_records`
const runtimeToolCallSelect = `SELECT tenant_id,id,job_run_id,node_run_id,attempt_id,agent_instance_id,tool_name,schema_version,request_digest,safe_request,safe_result,result_digest,state,error_code,started_at,finished_at,version,created_at,updated_at FROM runtime_tool_calls`

func scanStateCollection(row pgx.Row) (domain.StateCollection, error) {
	var value domain.StateCollection
	var readPolicy, writePolicy []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.CollectionKey, &value.Scope, &value.SchemaID, &value.SchemaRevision, &value.Consistency, &value.WriterNodeKey, &value.MaxRecordBytes, &value.MaxRecords, &value.RetentionPolicy, &readPolicy, &writePolicy, &value.Revision, &value.Watermark, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.ReadPolicy, err = decodeJSON[[]string](readPolicy)
	}
	if err == nil {
		value.WritePolicy, err = decodeJSON[[]string](writePolicy)
	}
	if value.ReadPolicy == nil {
		value.ReadPolicy = []string{}
	}
	if value.WritePolicy == nil {
		value.WritePolicy = []string{}
	}
	return value, dbError(err)
}

func scanStateRecord(row pgx.Row) (domain.StateRecord, error) {
	var value domain.StateRecord
	var body []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.CollectionID, &value.Key, &body, &value.ArtifactRef, &value.SchemaRevision, &value.Version, &value.Digest, &value.CreatedBy, &value.UpdatedBy, &value.CreatedAt, &value.UpdatedAt)
	if err == nil && len(body) > 0 {
		value.Value, err = decodeJSON[map[string]any](body)
	}
	if value.Value == nil && value.ArtifactRef == "" {
		value.Value = map[string]any{}
	}
	return value, dbError(err)
}

func scanToolCall(row pgx.Row) (domain.ToolCall, error) {
	var value domain.ToolCall
	var safeRequest, safeResult []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.NodeRunID, &value.AttemptID, &value.AgentInstanceID, &value.ToolName, &value.SchemaVersion, &value.RequestDigest, &safeRequest, &safeResult, &value.ResultDigest, &value.State, &value.ErrorCode, &value.StartedAt, &value.FinishedAt, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.SafeRequest, err = decodeJSON[map[string]any](safeRequest)
	}
	if err == nil {
		value.SafeResult, err = decodeJSON[map[string]any](safeResult)
	}
	if value.SafeRequest == nil {
		value.SafeRequest = map[string]any{}
	}
	return value, dbError(err)
}

func (s *Store) CreateStateCollection(ctx context.Context, collection domain.StateCollection) error {
	if err := collection.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, collection.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_state_collections(tenant_id,id,job_run_id,collection_key,scope,schema_id,schema_revision,consistency,writer_node_key,max_record_bytes,max_records,retention_policy,read_policy,write_policy,revision,watermark,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, collection.TenantID, collection.ID, collection.JobRunID, collection.CollectionKey, collection.Scope, collection.SchemaID, collection.SchemaRevision, collection.Consistency, collection.WriterNodeKey, collection.MaxRecordBytes, collection.MaxRecords, collection.RetentionPolicy, jsonArrayValue(collection.ReadPolicy), jsonArrayValue(collection.WritePolicy), collection.Revision, collection.Watermark, collection.CreatedAt, collection.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) StateCollection(ctx context.Context, tenantID, id string) (domain.StateCollection, error) {
	var result domain.StateCollection
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanStateCollection(tx.QueryRow(ctx, runtimeStateCollectionSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("状态集合")
		}
		return err
	})
	return result, err
}

func (s *Store) StateCollections(ctx context.Context, tenantID, jobID string) ([]domain.StateCollection, error) {
	result := []domain.StateCollection{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeStateCollectionSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if jobID != "" {
			query += ` AND job_run_id=$2`
			args = append(args, jobID)
		}
		query += ` ORDER BY collection_key`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanStateCollection(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) StateRecord(ctx context.Context, tenantID, id string) (domain.StateRecord, error) {
	var result domain.StateRecord
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanStateRecord(tx.QueryRow(ctx, runtimeStateRecordSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("状态记录")
		}
		return err
	})
	return result, err
}

func (s *Store) StateRecords(ctx context.Context, tenantID, collectionID string) ([]domain.StateRecord, error) {
	result := []domain.StateRecord{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeStateRecordSelect+` WHERE tenant_id=$1 AND collection_id=$2 ORDER BY key`, tenantID, collectionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanStateRecord(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) ApplyStateRecordCAS(ctx context.Context, record domain.StateRecord, expectedVersion int) (domain.StateRecord, error) {
	if err := record.Validate(); err != nil {
		return record, err
	}
	var result domain.StateRecord
	err := s.withTenant(ctx, record.TenantID, func(tx pgx.Tx) error {
		var err error
		result, err = applyStateRecordTx(ctx, tx, record, expectedVersion)
		return err
	})
	return result, err
}

func applyStateRecordTx(ctx context.Context, tx pgx.Tx, record domain.StateRecord, expectedVersion int) (domain.StateRecord, error) {
	var result domain.StateRecord
	collection, err := scanStateCollection(tx.QueryRow(ctx, runtimeStateCollectionSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, record.TenantID, record.CollectionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return result, domain.NotFound("状态集合")
	}
	if err != nil {
		return result, err
	}
	if record.TenantID != collection.TenantID {
		return result, domain.Invalid("STATE_RECORD_SCOPE_INVALID", "状态记录与集合不属于同一租户")
	}
	if record.SchemaRevision != collection.SchemaRevision {
		return result, domain.Conflict("STATE_SCHEMA_REVISION_CONFLICT", "状态记录 SchemaRevision 与集合已发布版本不一致")
	}
	current, currentErr := scanStateRecord(tx.QueryRow(ctx, runtimeStateRecordSelect+` WHERE tenant_id=$1 AND collection_id=$2 AND key=$3 FOR UPDATE`, record.TenantID, record.CollectionID, record.Key))
	exists := currentErr == nil
	if currentErr != nil && !errors.Is(currentErr, pgx.ErrNoRows) {
		return result, currentErr
	}
	if (expectedVersion == 0 && exists) || (exists && current.Version != expectedVersion) || (!exists && expectedVersion != 0) {
		if exists {
			result = current
		}
		return result, domain.Conflict("STATE_RECORD_VERSION_CONFLICT", "状态记录版本已变化")
	}
	if collection.Consistency == "append_only" && exists {
		return current, domain.Policy("STATE_APPEND_ONLY_UPDATE_FORBIDDEN", "append_only 集合不允许覆盖既有记录", "使用新的记录键追加一条记录")
	}
	if !exists {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM runtime_state_records WHERE tenant_id=$1 AND collection_id=$2`, record.TenantID, record.CollectionID).Scan(&count); err != nil {
			return result, err
		}
		if count >= collection.MaxRecords {
			return result, domain.Policy("STATE_COLLECTION_RECORD_LIMIT", "状态集合已达到最大记录数", "清理过期记录或提高已发布集合上限")
		}
	}
	if record.Value != nil && len(jsonValue(record.Value)) > collection.MaxRecordBytes {
		return result, domain.Policy("STATE_RECORD_TOO_LARGE", "状态记录超过集合大小限制", "减少记录内容或改用受控 Artifact 引用")
	}
	if !exists {
		record.Version = 1
		_, err = tx.Exec(ctx, `INSERT INTO runtime_state_records(tenant_id,id,collection_id,key,value,artifact_ref,schema_revision,version,digest,created_by,updated_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, record.TenantID, record.ID, record.CollectionID, record.Key, runtimeNullableJSON(record.Value), record.ArtifactRef, record.SchemaRevision, record.Version, record.Digest, record.CreatedBy, record.UpdatedBy, record.CreatedAt, record.UpdatedAt)
	} else {
		record.Version = current.Version + 1
		_, err = tx.Exec(ctx, `UPDATE runtime_state_records SET value=$4,artifact_ref=$5,schema_revision=$6,version=$7,digest=$8,updated_by=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2 AND version=$3`, record.TenantID, current.ID, expectedVersion, runtimeNullableJSON(record.Value), record.ArtifactRef, record.SchemaRevision, record.Version, record.Digest, record.UpdatedBy, record.UpdatedAt)
	}
	if err != nil {
		return result, dbError(err)
	}
	collection.Revision++
	collection.Watermark++
	if _, err := tx.Exec(ctx, `UPDATE runtime_state_collections SET revision=$3,watermark=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2`, collection.TenantID, collection.ID, collection.Revision, collection.Watermark, record.UpdatedAt); err != nil {
		return result, dbError(err)
	}
	result = record
	return result, nil
}

func runtimeNullableJSON(value map[string]any) any {
	if value == nil {
		return nil
	}
	return jsonValue(value)
}

func (s *Store) ApplyStateRecordCommand(ctx context.Context, record domain.StateRecord, expectedVersion int, event domain.JobEvent) (domain.StateRecord, error) {
	if err := record.Validate(); err != nil {
		return record, err
	}
	var result domain.StateRecord
	err := s.withTenantCommand(ctx, record.TenantID, "runtime.apply_state_record", func(tx pgx.Tx) error {
		if err := validateRuntimeEventTx(ctx, tx, event); err != nil {
			return err
		}
		var err error
		result, err = applyStateRecordTx(ctx, tx, record, expectedVersion)
		if err != nil {
			return err
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return result, err
}

func (s *Store) ApplyFencedStateRecordCommand(ctx context.Context, record domain.StateRecord, expectedVersion int, attemptID, fenceToken string, now time.Time, event domain.JobEvent) (domain.StateRecord, error) {
	if err := record.Validate(); err != nil {
		return record, err
	}
	var result domain.StateRecord
	err := s.withTenantCommand(ctx, record.TenantID, "runtime.apply_fenced_state_record", func(tx pgx.Tx) error {
		if err := validateAttemptFenceTx(ctx, tx, record.TenantID, attemptID, fenceToken, now); err != nil {
			return err
		}
		if err := validateRuntimeEventTx(ctx, tx, event); err != nil {
			return err
		}
		var err error
		result, err = applyStateRecordTx(ctx, tx, record, expectedVersion)
		if err != nil {
			return err
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return result, err
}

func (s *Store) RegisterToolCallCommand(ctx context.Context, call domain.ToolCall, event domain.JobEvent) (domain.ToolCall, error) {
	if err := call.Validate(); err != nil {
		return call, err
	}
	err := s.withTenantCommand(ctx, call.TenantID, "runtime.register_tool_call", func(tx pgx.Tx) error {
		if err := validateToolCallTx(ctx, tx, call); err != nil {
			return err
		}
		if err := validateRuntimeEventTx(ctx, tx, event); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_tool_calls(tenant_id,id,job_run_id,node_run_id,attempt_id,agent_instance_id,tool_name,schema_version,request_digest,safe_request,safe_result,result_digest,state,error_code,started_at,finished_at,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, call.TenantID, call.ID, call.JobRunID, call.NodeRunID, call.AttemptID, call.AgentInstanceID, call.ToolName, call.SchemaVersion, call.RequestDigest, jsonValue(call.SafeRequest), jsonValue(call.SafeResult), call.ResultDigest, call.State, call.ErrorCode, call.StartedAt, call.FinishedAt, call.Version, call.CreatedAt, call.UpdatedAt); err != nil {
			return dbError(err)
		}
		_, err := appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return call, err
}

func (s *Store) RegisterFencedToolCallCommand(ctx context.Context, call domain.ToolCall, fenceToken string, now time.Time, event domain.JobEvent) (domain.ToolCall, error) {
	if err := call.Validate(); err != nil {
		return call, err
	}
	err := s.withTenantCommand(ctx, call.TenantID, "runtime.register_fenced_tool_call", func(tx pgx.Tx) error {
		if err := validateAttemptFenceTx(ctx, tx, call.TenantID, call.AttemptID, fenceToken, now); err != nil {
			return err
		}
		if err := validateToolCallTx(ctx, tx, call); err != nil {
			return err
		}
		if err := validateRuntimeEventTx(ctx, tx, event); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_tool_calls(tenant_id,id,job_run_id,node_run_id,attempt_id,agent_instance_id,tool_name,schema_version,request_digest,safe_request,safe_result,result_digest,state,error_code,started_at,finished_at,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, call.TenantID, call.ID, call.JobRunID, call.NodeRunID, call.AttemptID, call.AgentInstanceID, call.ToolName, call.SchemaVersion, call.RequestDigest, jsonValue(call.SafeRequest), jsonValue(call.SafeResult), call.ResultDigest, call.State, call.ErrorCode, call.StartedAt, call.FinishedAt, call.Version, call.CreatedAt, call.UpdatedAt); err != nil {
			return dbError(err)
		}
		_, err := appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return call, err
}

func (s *Store) ApplyToolCallTransitionCommand(ctx context.Context, next domain.ToolCall, expectedVersion int, event domain.JobEvent) (domain.ToolCall, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	err := s.withTenantCommand(ctx, next.TenantID, "runtime.apply_tool_call_transition", func(tx pgx.Tx) error {
		if err := validateRuntimeEventTx(ctx, tx, event); err != nil {
			return err
		}
		current, err := scanToolCall(tx.QueryRow(ctx, runtimeToolCallSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("ToolCall")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 {
			return domain.Conflict("TOOL_CALL_VERSION_CONFLICT", "ToolCall 已被更新")
		}
		if current.State == domain.ToolCallSucceeded || current.State == domain.ToolCallFailed || current.State == domain.ToolCallUnknown {
			return domain.Conflict("TOOL_CALL_TERMINAL", "终态 ToolCall 不能原地修改")
		}
		if err := validateToolCallTx(ctx, tx, current); err != nil {
			return err
		}
		updated, err := tx.Exec(ctx, `UPDATE runtime_tool_calls SET safe_result=$3,result_digest=$4,state=$5,error_code=$6,started_at=$7,finished_at=$8,version=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2 AND version=$11`, next.TenantID, next.ID, jsonValue(next.SafeResult), next.ResultDigest, next.State, next.ErrorCode, next.StartedAt, next.FinishedAt, next.Version, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return domain.Conflict("TOOL_CALL_VERSION_CONFLICT", "ToolCall 已被更新")
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return next, err
}

func (s *Store) ApplyFencedToolCallTransitionCommand(ctx context.Context, next domain.ToolCall, expectedVersion int, fenceToken string, now time.Time, event domain.JobEvent) (domain.ToolCall, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	err := s.withTenantCommand(ctx, next.TenantID, "runtime.apply_fenced_tool_call_transition", func(tx pgx.Tx) error {
		if err := validateAttemptFenceTx(ctx, tx, next.TenantID, next.AttemptID, fenceToken, now); err != nil {
			return err
		}
		if err := validateRuntimeEventTx(ctx, tx, event); err != nil {
			return err
		}
		current, err := scanToolCall(tx.QueryRow(ctx, runtimeToolCallSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("ToolCall")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 {
			return domain.Conflict("TOOL_CALL_VERSION_CONFLICT", "ToolCall 已被更新")
		}
		if current.State == domain.ToolCallSucceeded || current.State == domain.ToolCallFailed || current.State == domain.ToolCallUnknown {
			return domain.Conflict("TOOL_CALL_TERMINAL", "终态 ToolCall 不能原地修改")
		}
		if err := validateToolCallTx(ctx, tx, current); err != nil {
			return err
		}
		updated, err := tx.Exec(ctx, `UPDATE runtime_tool_calls SET safe_result=$3,result_digest=$4,state=$5,error_code=$6,started_at=$7,finished_at=$8,version=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2 AND version=$11`, next.TenantID, next.ID, jsonValue(next.SafeResult), next.ResultDigest, next.State, next.ErrorCode, next.StartedAt, next.FinishedAt, next.Version, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return domain.Conflict("TOOL_CALL_VERSION_CONFLICT", "ToolCall 已被更新")
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return next, err
}

func validateRuntimeEventTx(ctx context.Context, tx pgx.Tx, event domain.JobEvent) error {
	if event.ID == "" || event.TenantID == "" || event.JobRunID == "" || event.Type == "" || event.ActorType == "" || event.OccurredAt.IsZero() {
		return domain.Invalid("JOB_EVENT_INVALID", "JobEvent 缺少执行实例、类型或执行者")
	}
	if event.Sequence == 0 {
		return nil
	}
	var count int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0) FROM runtime_job_events WHERE tenant_id=$1 AND job_run_id=$2`, event.TenantID, event.JobRunID).Scan(&count); err != nil {
		return err
	}
	if event.Sequence != count+1 {
		return domain.Conflict("JOB_EVENT_SEQUENCE_CONFLICT", "JobEvent 序号必须连续")
	}
	return nil
}

func (s *Store) CreateToolCall(ctx context.Context, call domain.ToolCall) error {
	if err := call.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, call.TenantID, func(tx pgx.Tx) error {
		if err := validateToolCallTx(ctx, tx, call); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO runtime_tool_calls(tenant_id,id,job_run_id,node_run_id,attempt_id,agent_instance_id,tool_name,schema_version,request_digest,safe_request,safe_result,result_digest,state,error_code,started_at,finished_at,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, call.TenantID, call.ID, call.JobRunID, call.NodeRunID, call.AttemptID, call.AgentInstanceID, call.ToolName, call.SchemaVersion, call.RequestDigest, jsonValue(call.SafeRequest), jsonValue(call.SafeResult), call.ResultDigest, call.State, call.ErrorCode, call.StartedAt, call.FinishedAt, call.Version, call.CreatedAt, call.UpdatedAt)
		return dbError(err)
	})
}

func validateToolCallTx(ctx context.Context, tx pgx.Tx, call domain.ToolCall) error {
	var jobRunID, nodeRunID, agentInstanceID, contextViewID, attemptState string
	var nodeJobRunID, agentJobRunID, agentNodeRunID, agentContextViewID, agentState string
	var viewJobRunID, viewNodeRunID, viewAttemptID string
	var allowedTools []byte
	err := tx.QueryRow(ctx, `SELECT a.job_run_id,a.node_run_id,a.agent_instance_id,a.context_view_id,a.state,
		n.job_run_id,ag.job_run_id,ag.node_run_id,ag.context_view_id,ag.state,
		cv.job_run_id,cv.node_run_id,cv.attempt_id,cv.allowed_tools
		FROM runtime_attempts a
		JOIN runtime_node_runs n ON n.tenant_id=a.tenant_id AND n.id=a.node_run_id
		JOIN runtime_agent_instances ag ON ag.tenant_id=a.tenant_id AND ag.id=a.agent_instance_id
		JOIN runtime_context_views cv ON cv.tenant_id=a.tenant_id AND cv.id=a.context_view_id
		WHERE a.tenant_id=$1 AND a.id=$2`, call.TenantID, call.AttemptID).Scan(&jobRunID, &nodeRunID, &agentInstanceID, &contextViewID, &attemptState, &nodeJobRunID, &agentJobRunID, &agentNodeRunID, &agentContextViewID, &agentState, &viewJobRunID, &viewNodeRunID, &viewAttemptID, &allowedTools)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotFound("RuntimeAttempt")
	}
	if err != nil {
		return dbError(err)
	}
	tools, err := decodeJSON[[]string](allowedTools)
	if err != nil {
		return err
	}
	validScope := jobRunID == call.JobRunID && nodeRunID == call.NodeRunID && agentInstanceID == call.AgentInstanceID && contextViewID != "" && nodeJobRunID == call.JobRunID && agentJobRunID == call.JobRunID && agentNodeRunID == call.NodeRunID && agentContextViewID == contextViewID && viewJobRunID == call.JobRunID && viewNodeRunID == call.NodeRunID && viewAttemptID == call.AttemptID
	if !validScope {
		return domain.Invalid("TOOL_CALL_SCOPE_INVALID", "ToolCall 必须绑定同一 JobRun、NodeRun、Attempt、Agent 和 ContextView")
	}
	if attemptState != domain.RuntimeAttemptPrepared && attemptState != domain.RuntimeAttemptRunning {
		return domain.Conflict("TOOL_CALL_ATTEMPT_NOT_ACTIVE", "只有准备中或运行中的 Attempt 可以创建或推进 ToolCall")
	}
	if agentState != domain.AgentRunnable && agentState != domain.AgentActive {
		return domain.Conflict("TOOL_CALL_AGENT_NOT_ACTIVE", "只有可运行或活动中的 AgentInstance 可以创建或推进 ToolCall")
	}
	for _, allowed := range tools {
		if strings.TrimSpace(allowed) == call.ToolName {
			return nil
		}
	}
	return domain.Policy("TOOL_CALL_NOT_ALLOWED", "当前 ContextView 未授权该工具", "仅调用 AllowedTools 中的工具")
}

func (s *Store) ToolCall(ctx context.Context, tenantID, id string) (domain.ToolCall, error) {
	var result domain.ToolCall
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanToolCall(tx.QueryRow(ctx, runtimeToolCallSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("ToolCall")
		}
		return err
	})
	return result, err
}

func (s *Store) ToolCallByIdempotencyKey(ctx context.Context, tenantID, attemptID, toolName, idempotencyKey string) (domain.ToolCall, error) {
	var result domain.ToolCall
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanToolCall(tx.QueryRow(ctx, runtimeToolCallSelect+` WHERE tenant_id=$1 AND attempt_id=$2 AND tool_name=$3 AND safe_request->>'idempotency_key'=$4`, tenantID, attemptID, toolName, idempotencyKey))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("ToolCall")
		}
		return err
	})
	return result, err
}

func (s *Store) ToolCalls(ctx context.Context, tenantID, attemptID string) ([]domain.ToolCall, error) {
	result := []domain.ToolCall{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeToolCallSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if strings.TrimSpace(attemptID) != "" {
			query += ` AND attempt_id=$2`
			args = append(args, attemptID)
		}
		query += ` ORDER BY created_at,id`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanToolCall(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) ApplyToolCallTransition(ctx context.Context, next domain.ToolCall, expectedVersion int) (domain.ToolCall, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	var result domain.ToolCall
	err := s.withTenant(ctx, next.TenantID, func(tx pgx.Tx) error {
		current, err := scanToolCall(tx.QueryRow(ctx, runtimeToolCallSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, next.TenantID, next.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("ToolCall")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || next.Version != expectedVersion+1 {
			return domain.Conflict("TOOL_CALL_VERSION_CONFLICT", "ToolCall 已被更新")
		}
		if current.State == domain.ToolCallSucceeded || current.State == domain.ToolCallFailed || current.State == domain.ToolCallUnknown {
			return domain.Conflict("TOOL_CALL_TERMINAL", "终态 ToolCall 不能原地修改")
		}
		updated, err := tx.Exec(ctx, `UPDATE runtime_tool_calls SET safe_result=$3,result_digest=$4,state=$5,error_code=$6,started_at=$7,finished_at=$8,version=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2 AND version=$11`, next.TenantID, next.ID, jsonValue(next.SafeResult), next.ResultDigest, next.State, next.ErrorCode, next.StartedAt, next.FinishedAt, next.Version, next.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return domain.Conflict("TOOL_CALL_VERSION_CONFLICT", "ToolCall 已被更新")
		}
		result = next
		return nil
	})
	return result, err
}
