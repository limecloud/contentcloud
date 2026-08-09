package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const runtimeFanoutSetSelect = `SELECT tenant_id,id,job_run_id,map_node_key,join_node_key,source_collection,source_revision,source_watermark,generation,idempotency_key,membership_digest,request_digest,member_count,join_strategy,min_success,quorum_percent,zero_member_policy,quorum_stop_policy,status,version,closed_at,created_at,updated_at FROM runtime_fanout_sets`
const runtimeFanoutMemberSelect = `SELECT tenant_id,id,fanout_set_id,member_key,item_key,item_digest,generation,node_run_id,state,output_refs,output_digest,error_code,version,created_at,updated_at FROM runtime_fanout_members`

func scanRuntimeFanoutSet(row pgx.Row) (domain.FanoutSet, error) {
	var value domain.FanoutSet
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.MapNodeKey, &value.JoinNodeKey, &value.SourceCollection, &value.SourceRevision, &value.SourceWatermark, &value.Generation, &value.IdempotencyKey, &value.MembershipDigest, &value.RequestDigest, &value.MemberCount, &value.JoinPolicy.Strategy, &value.JoinPolicy.MinSuccess, &value.JoinPolicy.QuorumPercent, &value.JoinPolicy.ZeroMemberPolicy, &value.JoinPolicy.QuorumStopPolicy, &value.Status, &value.Version, &value.ClosedAt, &value.CreatedAt, &value.UpdatedAt)
	return value, dbError(err)
}

func scanRuntimeFanoutMember(row pgx.Row) (domain.FanoutMember, error) {
	var value domain.FanoutMember
	var outputRefs []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.FanoutSetID, &value.MemberKey, &value.ItemKey, &value.ItemDigest, &value.Generation, &value.NodeRunID, &value.State, &outputRefs, &value.OutputDigest, &value.ErrorCode, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.OutputRefs, err = decodeJSON[[]string](outputRefs)
	}
	if value.OutputRefs == nil {
		value.OutputRefs = []string{}
	}
	return value, dbError(err)
}

func (s *Store) FanoutSet(ctx context.Context, tenantID, id string) (domain.FanoutSet, error) {
	var result domain.FanoutSet
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeFanoutSet(tx.QueryRow(ctx, runtimeFanoutSetSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("FanoutSet")
		}
		return err
	})
	return result, err
}

func (s *Store) FanoutSetByIdempotencyKey(ctx context.Context, tenantID, jobID, idempotencyKey string) (domain.FanoutSet, error) {
	var result domain.FanoutSet
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeFanoutSet(tx.QueryRow(ctx, runtimeFanoutSetSelect+` WHERE tenant_id=$1 AND job_run_id=$2 AND idempotency_key=$3`, tenantID, jobID, idempotencyKey))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("FanoutSet")
		}
		return err
	})
	return result, err
}

func (s *Store) FanoutSets(ctx context.Context, tenantID, jobID string) ([]domain.FanoutSet, error) {
	result := []domain.FanoutSet{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeFanoutSetSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if jobID != "" {
			query += ` AND job_run_id=$2`
			args = append(args, jobID)
		}
		query += ` ORDER BY created_at,id`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeFanoutSet(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) FanoutMembers(ctx context.Context, tenantID, setID string) ([]domain.FanoutMember, error) {
	result := []domain.FanoutMember{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeFanoutMemberSelect+` WHERE tenant_id=$1 AND fanout_set_id=$2 ORDER BY member_key`, tenantID, setID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeFanoutMember(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(result) == 0 {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runtime_fanout_sets WHERE tenant_id=$1 AND id=$2)`, tenantID, setID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return domain.NotFound("FanoutSet")
			}
		}
		return nil
	})
	return result, err
}

func (s *Store) CreateFanoutSetCommand(ctx context.Context, nextJob domain.JobRun, expectedJobVersion int, plan domain.JobPlanRevision, set domain.FanoutSet, members []domain.FanoutMember, nodes []domain.NodeRun, event domain.JobEvent) (domain.JobRun, error) {
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
	for _, member := range members {
		if err := member.Validate(); err != nil {
			return nextJob, err
		}
	}
	for _, node := range nodes {
		if err := node.Validate(); err != nil {
			return nextJob, err
		}
	}
	err := s.withTenant(ctx, set.TenantID, func(tx pgx.Tx) error {
		currentJob, err := scanRuntimeJob(tx.QueryRow(ctx, runtimeJobSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, nextJob.TenantID, nextJob.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("执行实例")
		}
		if err != nil {
			return err
		}
		if currentJob.Version != expectedJobVersion || nextJob.Version != expectedJobVersion+1 || currentJob.PlanRevisionID != plan.BaseRevisionID || nextJob.PlanRevisionID != plan.ID || nextJob.PlanDigest != plan.Digest {
			return domain.Conflict("GRAPH_VERSION_CONFLICT", "执行图版本已经变化，请重新读取")
		}
		if err := insertRuntimePlanTx(ctx, tx, plan); err != nil {
			return err
		}
		for _, node := range nodes {
			if node.TenantID != set.TenantID || node.JobRunID != set.JobRunID {
				return domain.Invalid("NODE_RUN_SCOPE_INVALID", "FanoutSet 子节点不属于当前执行实例")
			}
			if _, err := tx.Exec(ctx, `INSERT INTO runtime_node_runs(tenant_id,id,job_run_id,node_key,state,attempt_count,output_refs,output_digest,error_code,lease_owner,fence_token,lease_expires_at,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, node.TenantID, node.ID, node.JobRunID, node.NodeKey, node.State, node.AttemptCount, jsonArrayValue(node.OutputRefs), node.OutputDigest, node.ErrorCode, node.LeaseOwner, node.FenceToken, node.LeaseExpiresAt, node.Version, node.CreatedAt, node.UpdatedAt); err != nil {
				return dbError(err)
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO runtime_fanout_sets(tenant_id,id,job_run_id,map_node_key,join_node_key,source_collection,source_revision,source_watermark,generation,idempotency_key,membership_digest,request_digest,member_count,join_strategy,min_success,quorum_percent,zero_member_policy,quorum_stop_policy,status,version,closed_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`, set.TenantID, set.ID, set.JobRunID, set.MapNodeKey, set.JoinNodeKey, set.SourceCollection, set.SourceRevision, set.SourceWatermark, set.Generation, set.IdempotencyKey, set.MembershipDigest, set.RequestDigest, set.MemberCount, set.JoinPolicy.Strategy, set.JoinPolicy.MinSuccess, set.JoinPolicy.QuorumPercent, set.JoinPolicy.ZeroMemberPolicy, set.JoinPolicy.QuorumStopPolicy, set.Status, set.Version, set.ClosedAt, set.CreatedAt, set.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		for _, member := range members {
			if _, err := tx.Exec(ctx, `INSERT INTO runtime_fanout_members(tenant_id,id,fanout_set_id,member_key,item_key,item_digest,generation,node_run_id,state,output_refs,output_digest,error_code,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, member.TenantID, member.ID, member.FanoutSetID, member.MemberKey, member.ItemKey, member.ItemDigest, member.Generation, member.NodeRunID, member.State, jsonArrayValue(member.OutputRefs), member.OutputDigest, member.ErrorCode, member.Version, member.CreatedAt, member.UpdatedAt); err != nil {
				return dbError(err)
			}
		}
		updated, err := tx.Exec(ctx, `UPDATE runtime_job_runs SET plan_revision_id=$3,plan_digest=$4,version=$5,updated_at=$6 WHERE tenant_id=$1 AND id=$2 AND version=$7`, nextJob.TenantID, nextJob.ID, nextJob.PlanRevisionID, nextJob.PlanDigest, nextJob.Version, nextJob.UpdatedAt, expectedJobVersion)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return domain.Conflict("GRAPH_VERSION_CONFLICT", "执行图版本已经变化，请重新读取")
		}
		if event.TenantID, event.JobRunID = set.TenantID, set.JobRunID; event.Payload == nil {
			event.Payload = map[string]any{}
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return nextJob, err
}

func (s *Store) ApplyFanoutJoinCommand(ctx context.Context, set domain.FanoutSet, expectedVersion int, members []domain.FanoutMember, cancelMemberKeys []string, event domain.JobEvent) (domain.FanoutSet, error) {
	if err := set.Validate(); err != nil {
		return set, err
	}
	var result domain.FanoutSet
	err := s.withTenant(ctx, set.TenantID, func(tx pgx.Tx) error {
		current, err := scanRuntimeFanoutSet(tx.QueryRow(ctx, runtimeFanoutSetSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, set.TenantID, set.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("FanoutSet")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || set.Version != expectedVersion+1 {
			return domain.Conflict("FANOUT_SET_VERSION_CONFLICT", "FanoutSet 已被更新，请重新读取")
		}
		if len(members) != current.MemberCount {
			return domain.Conflict("FANOUT_MEMBER_COUNT_MISMATCH", "FanoutSet 成员数量与封存快照不一致")
		}
		event.TenantID, event.JobRunID = current.TenantID, current.JobRunID
		if event.ID == "" || event.TenantID == "" || event.JobRunID == "" || event.Type == "" || event.ActorType == "" || event.OccurredAt.IsZero() {
			return domain.Invalid("JOB_EVENT_INVALID", "JobEvent 缺少执行实例、类型或执行者")
		}
		for _, member := range members {
			if err := member.Validate(); err != nil {
				return err
			}
			stored, err := scanRuntimeFanoutMember(tx.QueryRow(ctx, runtimeFanoutMemberSelect+` WHERE tenant_id=$1 AND fanout_set_id=$2 AND member_key=$3 FOR UPDATE`, current.TenantID, current.ID, member.MemberKey))
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("FanoutSet 成员")
			}
			if err != nil {
				return err
			}
			if member.TenantID != current.TenantID || member.FanoutSetID != current.ID || (member.Version != stored.Version && member.Version != stored.Version+1) {
				return domain.Conflict("FANOUT_MEMBER_VERSION_CONFLICT", "FanoutSet 成员已被更新，请重新读取")
			}
			if member.Version == stored.Version {
				continue
			}
			updated, err := tx.Exec(ctx, `UPDATE runtime_fanout_members SET state=$4,output_refs=$5,output_digest=$6,error_code=$7,version=$8,updated_at=$9 WHERE tenant_id=$1 AND fanout_set_id=$2 AND member_key=$3 AND version=$10`, member.TenantID, member.FanoutSetID, member.MemberKey, member.State, jsonArrayValue(member.OutputRefs), member.OutputDigest, member.ErrorCode, member.Version, member.UpdatedAt, stored.Version)
			if err != nil {
				return dbError(err)
			}
			if updated.RowsAffected() != 1 {
				return domain.NotFound("FanoutSet 成员")
			}
		}
		for _, memberKey := range cancelMemberKeys {
			member, err := scanRuntimeFanoutMember(tx.QueryRow(ctx, runtimeFanoutMemberSelect+` WHERE tenant_id=$1 AND fanout_set_id=$2 AND member_key=$3 FOR UPDATE`, current.TenantID, current.ID, memberKey))
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("FanoutSet 成员")
			}
			if err != nil {
				return err
			}
			if member.State != domain.FanoutMemberPending {
				continue
			}
			node, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, current.TenantID, member.NodeRunID))
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("FanoutSet 子节点")
			}
			if err != nil {
				return err
			}
			if node.JobRunID != current.JobRunID || (node.State != domain.NodePending && node.State != domain.NodeReady) {
				return domain.Conflict("FANOUT_CANCEL_CONFLICT", "只能取消尚未领取的 Fanout 子节点")
			}
			if _, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state='cancelled',version=version+1,lease_owner='',fence_token='',lease_expires_at=NULL,updated_at=$3 WHERE tenant_id=$1 AND id=$2 AND version=$4`, current.TenantID, node.ID, event.OccurredAt, node.Version); err != nil {
				return dbError(err)
			}
			if _, err := tx.Exec(ctx, `UPDATE runtime_fanout_members SET state='cancelled',version=version+1,updated_at=$3 WHERE tenant_id=$1 AND fanout_set_id=$2 AND member_key=$4 AND version=$5`, current.TenantID, current.ID, event.OccurredAt, memberKey, member.Version); err != nil {
				return dbError(err)
			}
		}
		updated, err := tx.Exec(ctx, `UPDATE runtime_fanout_sets SET status=$3,version=$4,closed_at=$5,updated_at=$6 WHERE tenant_id=$1 AND id=$2 AND version=$7`, set.TenantID, set.ID, set.Status, set.Version, set.ClosedAt, set.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return domain.Conflict("FANOUT_SET_VERSION_CONFLICT", "FanoutSet 已被更新，请重新读取")
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		result = set
		return err
	})
	return result, err
}
