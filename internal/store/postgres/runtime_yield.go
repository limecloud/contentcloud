package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const runtimeYieldSelect = `SELECT tenant_id,id,job_run_id,node_run_id,attempt_id,agent_instance_id,reason,wait_refs,state,resume_key,version,yielded_at,resolved_at,created_at,updated_at FROM runtime_yields`

func scanRuntimeYield(row pgx.Row) (domain.RuntimeYield, error) {
	var value domain.RuntimeYield
	var refs []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.NodeRunID, &value.AttemptID, &value.AgentInstanceID, &value.Reason, &refs, &value.State, &value.ResumeKey, &value.Version, &value.YieldedAt, &value.ResolvedAt, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.WaitRefs, err = decodeJSON[[]string](refs)
	}
	return value, err
}

func (s *Store) RuntimeYield(ctx context.Context, tenantID, id string) (domain.RuntimeYield, error) {
	var result domain.RuntimeYield
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeYield(tx.QueryRow(ctx, runtimeYieldSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("RuntimeYield")
		}
		return err
	})
	return result, err
}

func (s *Store) RuntimeYields(ctx context.Context, tenantID, jobID string) ([]domain.RuntimeYield, error) {
	result := []domain.RuntimeYield{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeYieldSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if jobID != "" {
			query += ` AND job_run_id=$2`
			args = append(args, jobID)
		}
		query += ` ORDER BY yielded_at,id`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeYield(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) YieldDispatch(ctx context.Context, yielded domain.RuntimeYield, node domain.NodeRun, expectedNodeVersion int, attempt domain.RuntimeAttempt, expectedAttemptVersion int, agent domain.AgentInstance, expectedAgentVersion int, fenceToken string, event domain.JobEvent) (domain.RuntimeYield, domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error) {
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
	err := s.withTenant(ctx, yielded.TenantID, func(tx pgx.Tx) error {
		if existing, err := scanRuntimeYield(tx.QueryRow(ctx, runtimeYieldSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, yielded.TenantID, yielded.ID)); err == nil {
			yielded = existing
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		currentNode, currentAttempt, currentAgent, err := lockDispatchState(ctx, tx, node.TenantID, node.ID, attempt.ID, agent.ID)
		if err != nil {
			return err
		}
		if currentNode.Version != expectedNodeVersion || currentAttempt.Version != expectedAttemptVersion || currentAgent.Version != expectedAgentVersion {
			return domain.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被其他执行者更新")
		}
		if currentNode.State != domain.NodeRunning || currentAttempt.State != domain.RuntimeAttemptRunning || currentAgent.State != domain.AgentActive || currentNode.LeaseOwner != currentAttempt.LeaseOwner || currentAttempt.LeaseOwner != event.ActorID || fenceToken == "" || currentNode.FenceToken != fenceToken || currentAttempt.FenceToken != fenceToken {
			return domain.Conflict("DISPATCH_LEASE_STALE", "让出请求不属于当前调度租约")
		}
		if yielded.TenantID != node.TenantID || yielded.JobRunID != node.JobRunID || yielded.NodeRunID != node.ID || yielded.AttemptID != attempt.ID || yielded.AgentInstanceID != agent.ID {
			return domain.Invalid("RUNTIME_YIELD_SCOPE_INVALID", "RuntimeYield 不属于当前调度范围")
		}
		if node.Version != expectedNodeVersion+1 || attempt.Version != expectedAttemptVersion+1 || agent.Version != expectedAgentVersion+1 {
			return domain.Invalid("RUNTIME_YIELD_VERSION_INVALID", "让出后的调度对象版本无效")
		}
		if err := currentNode.Transition(node.State); err != nil {
			return err
		}
		if err := currentAttempt.Transition(attempt.State); err != nil {
			return err
		}
		if err := currentAgent.Transition(agent.State); err != nil {
			return err
		}
		if err := updateDispatchStateTx(ctx, tx, node, expectedNodeVersion, attempt, expectedAttemptVersion, agent, expectedAgentVersion); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_yields(tenant_id,id,job_run_id,node_run_id,attempt_id,agent_instance_id,reason,wait_refs,state,resume_key,version,yielded_at,resolved_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, yielded.TenantID, yielded.ID, yielded.JobRunID, yielded.NodeRunID, yielded.AttemptID, yielded.AgentInstanceID, yielded.Reason, jsonArrayValue(yielded.WaitRefs), yielded.State, yielded.ResumeKey, yielded.Version, yielded.YieldedAt, yielded.ResolvedAt, yielded.CreatedAt, yielded.UpdatedAt); err != nil {
			return dbError(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE runtime_resource_reservations SET state='released',fence_token='',expires_at=NULL,released_at=$3,updated_at=$3 WHERE tenant_id=$1 AND attempt_id=$2 AND state='held' AND fence_token=$4`, attempt.TenantID, attempt.ID, event.OccurredAt, fenceToken); err != nil {
			return dbError(err)
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return yielded, node, attempt, agent, err
}

func (s *Store) ResolveRuntimeYield(ctx context.Context, yielded domain.RuntimeYield, expectedYieldVersion int, node domain.NodeRun, expectedNodeVersion int, agent domain.AgentInstance, expectedAgentVersion int, event domain.JobEvent) (domain.RuntimeYield, domain.NodeRun, domain.AgentInstance, error) {
	if err := yielded.Validate(); err != nil {
		return yielded, node, agent, err
	}
	err := s.withTenant(ctx, yielded.TenantID, func(tx pgx.Tx) error {
		currentYield, err := scanRuntimeYield(tx.QueryRow(ctx, runtimeYieldSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, yielded.TenantID, yielded.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("RuntimeYield")
		}
		if err != nil {
			return err
		}
		if currentYield.State == domain.RuntimeYieldResolved {
			if currentYield.ResumeKey != yielded.ResumeKey {
				return domain.Conflict("RUNTIME_YIELD_ALREADY_RESOLVED", "RuntimeYield 已由其他恢复请求处理")
			}
			yielded = currentYield
			return nil
		}
		currentNode, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, node.TenantID, node.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("执行节点")
		}
		if err != nil {
			return err
		}
		currentAgent, err := scanRuntimeAgent(tx.QueryRow(ctx, runtimeAgentSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, agent.TenantID, agent.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("AgentInstance")
		}
		if err != nil {
			return err
		}
		if currentYield.Version != expectedYieldVersion || currentNode.Version != expectedNodeVersion || currentAgent.Version != expectedAgentVersion || yielded.Version != expectedYieldVersion+1 || node.Version != expectedNodeVersion+1 || agent.Version != expectedAgentVersion+1 {
			return domain.Conflict("RUNTIME_YIELD_VERSION_CONFLICT", "RuntimeYield 已被更新，请重新读取")
		}
		if err := currentNode.Transition(node.State); err != nil {
			return err
		}
		if err := currentAgent.Transition(agent.State); err != nil {
			return err
		}
		nodeResult, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$3,lease_owner='',fence_token='',lease_expires_at=NULL,version=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2 AND version=$6`, node.TenantID, node.ID, node.State, node.Version, node.UpdatedAt, expectedNodeVersion)
		if err != nil {
			return dbError(err)
		}
		agentResult, err := tx.Exec(ctx, `UPDATE runtime_agent_instances SET state=$3,version=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2 AND version=$6`, agent.TenantID, agent.ID, agent.State, agent.Version, agent.UpdatedAt, expectedAgentVersion)
		if err != nil {
			return dbError(err)
		}
		yieldResult, err := tx.Exec(ctx, `UPDATE runtime_yields SET state=$3,resume_key=$4,version=$5,resolved_at=$6,updated_at=$7 WHERE tenant_id=$1 AND id=$2 AND version=$8`, yielded.TenantID, yielded.ID, yielded.State, yielded.ResumeKey, yielded.Version, yielded.ResolvedAt, yielded.UpdatedAt, expectedYieldVersion)
		if err != nil {
			return dbError(err)
		}
		if nodeResult.RowsAffected() != 1 || agentResult.RowsAffected() != 1 || yieldResult.RowsAffected() != 1 {
			return domain.Conflict("RUNTIME_YIELD_VERSION_CONFLICT", "RuntimeYield 已被更新，请重新读取")
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return yielded, node, agent, err
}
