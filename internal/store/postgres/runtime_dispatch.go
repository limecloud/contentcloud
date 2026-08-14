package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const runtimeAttemptSelect = `SELECT tenant_id,id,job_run_id,node_run_id,agent_instance_id,context_view_id,attempt_no,harness_kind,capabilities,session_ref,state,lease_owner,fence_token,gateway_token_hash,gateway_expires_at,lease_expires_at,output_refs,result_digest,safe_summary,error_code,version,created_at,started_at,finished_at,updated_at FROM runtime_attempts`

func validateAttemptFenceTx(ctx context.Context, tx pgx.Tx, tenantID, attemptID, fenceToken string, now time.Time) error {
	attempt, err := scanRuntimeAttempt(tx.QueryRow(ctx, runtimeAttemptSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, attemptID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotFound("RuntimeAttempt")
	}
	if err != nil {
		return err
	}
	if attempt.State != domain.RuntimeAttemptRunning || strings.TrimSpace(fenceToken) == "" || attempt.FenceToken != fenceToken || attempt.LeaseExpiresAt == nil || !attempt.LeaseExpiresAt.After(now) {
		return domain.Conflict("MCP_GATEWAY_FENCE_STALE", "MCP 调用的 Attempt fence 或租约已失效")
	}
	return nil
}

func (s *Store) NextReadyNode(ctx context.Context, tenantID, jobID string, allowedProjectIDs []string) (domain.NodeRun, error) {
	var result domain.NodeRun
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeNodeSelect + ` WHERE tenant_id=$1 AND state='ready'
			AND EXISTS (SELECT 1 FROM runtime_job_runs eligible_job
				WHERE eligible_job.tenant_id=runtime_node_runs.tenant_id
				  AND eligible_job.id=runtime_node_runs.job_run_id
				  AND eligible_job.state NOT IN ('paused','completed','failed','cancelled','rejected'))`
		args := []any{tenantID}
		if strings.TrimSpace(jobID) != "" {
			query += ` AND job_run_id=$2`
			args = append(args, jobID)
		}
		if allowedProjectIDs != nil {
			query += ` AND EXISTS (SELECT 1 FROM runtime_job_runs scoped_job
				WHERE scoped_job.tenant_id=runtime_node_runs.tenant_id
				  AND scoped_job.id=runtime_node_runs.job_run_id
				  AND scoped_job.project_id = ANY($` + strconv.Itoa(len(args)+1) + `::uuid[]))`
			args = append(args, allowedProjectIDs)
		}
		query += ` ORDER BY ((SELECT priority FROM runtime_job_runs j WHERE j.tenant_id=runtime_node_runs.tenant_id AND j.id=runtime_node_runs.job_run_id) + floor(EXTRACT(EPOCH FROM (now()-updated_at))/60)) DESC, updated_at,created_at,id LIMIT 1`
		value, err := scanRuntimeNode(tx.QueryRow(ctx, query, args...))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("可调度的执行节点")
		}
		return err
	})
	return result, err
}

func (s *Store) AgentInstanceForNode(ctx context.Context, tenantID, nodeID string) (domain.AgentInstance, error) {
	var result domain.AgentInstance
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeAgent(tx.QueryRow(ctx, runtimeAgentSelect+` WHERE tenant_id=$1 AND node_run_id=$2 AND parent_agent_instance_id IS NULL ORDER BY created_at,id LIMIT 1`, tenantID, nodeID))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("节点 AgentInstance")
		}
		return err
	})
	return result, err
}

func scanRuntimeAttempt(row pgx.Row) (domain.RuntimeAttempt, error) {
	var value domain.RuntimeAttempt
	var capabilities, outputs, summary []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.NodeRunID, &value.AgentInstanceID, &value.ContextViewID, &value.AttemptNo, &value.HarnessKind, &capabilities, &value.SessionRef, &value.State, &value.LeaseOwner, &value.FenceToken, &value.GatewayTokenHash, &value.GatewayExpiresAt, &value.LeaseExpiresAt, &outputs, &value.ResultDigest, &summary, &value.ErrorCode, &value.Version, &value.CreatedAt, &value.StartedAt, &value.FinishedAt, &value.UpdatedAt)
	if err == nil {
		value.Capabilities, err = decodeJSON[map[string]any](capabilities)
	}
	if err == nil {
		value.OutputRefs, err = decodeJSON[[]string](outputs)
	}
	if err == nil {
		value.SafeSummary, err = decodeJSON[map[string]any](summary)
	}
	if value.Capabilities == nil {
		value.Capabilities = map[string]any{}
	}
	if value.OutputRefs == nil {
		value.OutputRefs = []string{}
	}
	if value.SafeSummary == nil {
		value.SafeSummary = map[string]any{}
	}
	return value, dbError(err)
}

func (s *Store) RuntimeAttempt(ctx context.Context, tenantID, id string) (domain.RuntimeAttempt, error) {
	var result domain.RuntimeAttempt
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeAttempt(tx.QueryRow(ctx, runtimeAttemptSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("RuntimeAttempt")
		}
		return err
	})
	return result, err
}

func (s *Store) RuntimeAttemptByGatewayTokenHash(ctx context.Context, tokenHash string) (domain.RuntimeAttempt, error) {
	var tenantID, attemptID string
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id,attempt_id FROM contentcloud_lookup_runtime_gateway_token($1)`, tokenHash).Scan(&tenantID, &attemptID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RuntimeAttempt{}, domain.NotFound("Runtime Gateway 凭据")
		}
		return domain.RuntimeAttempt{}, err
	}
	return s.RuntimeAttempt(ctx, tenantID, attemptID)
}

func (s *Store) RuntimeAttempts(ctx context.Context, tenantID, jobID string) ([]domain.RuntimeAttempt, error) {
	result := []domain.RuntimeAttempt{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeAttemptSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if strings.TrimSpace(jobID) != "" {
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
			value, err := scanRuntimeAttempt(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) PrepareDispatch(ctx context.Context, node domain.NodeRun, expectedNodeVersion int, attempt domain.RuntimeAttempt, view domain.ContextView, agent domain.AgentInstance, createAgent bool, expectedAgentVersion int, reservations []domain.ResourceReservation, event domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error) {
	if err := node.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := attempt.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if len(attempt.GatewayTokenHash) != 64 || attempt.GatewayExpiresAt == nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.Invalid("DISPATCH_GATEWAY_CREDENTIAL_INVALID", "RuntimeAttempt 必须持有哈希化的短期 Gateway 凭据")
	}
	if err := view.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := agent.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	err := s.withTenantCommand(ctx, node.TenantID, "runtime.prepare_dispatch", func(tx pgx.Tx) error {
		currentNode, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, node.TenantID, node.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("执行节点")
		}
		if err != nil {
			return err
		}
		if currentNode.Version != expectedNodeVersion || currentNode.State != domain.NodeReady {
			return domain.Conflict("NODE_DISPATCH_CONFLICT", "执行节点已经被其他执行者领取")
		}
		if node.Version != expectedNodeVersion+1 || node.State != domain.NodeLeased || node.AttemptCount != currentNode.AttemptCount+1 || node.AttemptCount != attempt.AttemptNo || node.TenantID != attempt.TenantID || node.TenantID != view.TenantID || node.TenantID != agent.TenantID || node.JobRunID != attempt.JobRunID || node.JobRunID != view.JobRunID || node.JobRunID != agent.JobRunID || node.ID != attempt.NodeRunID || node.ID != view.NodeRunID || node.ID != agent.NodeRunID || view.AttemptID != attempt.ID || attempt.ContextViewID != view.ID || attempt.AgentInstanceID != agent.ID || agent.ContextViewID != view.ID {
			return domain.Invalid("DISPATCH_PREPARE_INVALID", "待准备的调度对象版本、范围或租约不一致")
		}
		if node.FenceToken == "" || attempt.FenceToken == "" || node.FenceToken != attempt.FenceToken {
			return domain.Invalid("DISPATCH_FENCE_INVALID", "节点与 RuntimeAttempt 必须共享不可猜的围栏令牌")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_context_views(tenant_id,id,job_run_id,node_run_id,attempt_id,schema_version,input_refs,state_refs,event_refs,allowed_tools,max_tokens,budget_minor,digest,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, view.TenantID, view.ID, view.JobRunID, view.NodeRunID, view.AttemptID, view.SchemaVersion, jsonArrayValue(view.InputRefs), jsonArrayValue(view.StateRefs), jsonArrayValue(view.EventRefs), jsonArrayValue(view.AllowedTools), view.MaxTokens, view.BudgetMinor, view.Digest, view.CreatedAt, view.ExpiresAt); err != nil {
			return dbError(err)
		}
		if createAgent {
			if expectedAgentVersion != 0 {
				return domain.Conflict("AGENT_INSTANCE_EXISTS", "AgentInstance 已存在")
			}
			if _, err := tx.Exec(ctx, `INSERT INTO runtime_agent_instances(tenant_id,id,job_run_id,node_run_id,parent_agent_instance_id,role,harness_kind,session_ref,execution_profile_id,context_view_id,state,depth,remaining_descendants,budget_minor,used_cost_minor,version,created_at,updated_at) VALUES($1,$2,$3,$4,NULL,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, agent.TenantID, agent.ID, agent.JobRunID, agent.NodeRunID, agent.Role, agent.HarnessKind, agent.SessionRef, agent.ExecutionProfileID, agent.ContextViewID, agent.State, agent.Depth, agent.RemainingDescendants, agent.BudgetMinor, agent.UsedCostMinor, agent.Version, agent.CreatedAt, agent.UpdatedAt); err != nil {
				return dbError(err)
			}
		} else {
			currentAgent, err := scanRuntimeAgent(tx.QueryRow(ctx, runtimeAgentSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, agent.TenantID, agent.ID))
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("AgentInstance")
			}
			if err != nil {
				return err
			}
			if currentAgent.Version != expectedAgentVersion || agent.Version != expectedAgentVersion+1 || currentAgent.JobRunID != agent.JobRunID || currentAgent.NodeRunID != agent.NodeRunID || currentAgent.ParentAgentInstanceID != "" || currentAgent.HarnessKind != agent.HarnessKind || currentAgent.ExecutionProfileID != agent.ExecutionProfileID {
				return domain.Conflict("AGENT_INSTANCE_DISPATCH_CONFLICT", "节点 AgentInstance 已被更新或执行配置发生变化")
			}
			if err := currentAgent.Transition(domain.AgentRunnable); err != nil {
				return err
			}
			result, err := tx.Exec(ctx, `UPDATE runtime_agent_instances SET session_ref=$3,context_view_id=$4,state=$5,budget_minor=$6,used_cost_minor=$7,version=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2 AND version=$10`, agent.TenantID, agent.ID, agent.SessionRef, agent.ContextViewID, agent.State, agent.BudgetMinor, agent.UsedCostMinor, agent.Version, agent.UpdatedAt, expectedAgentVersion)
			if err != nil {
				return dbError(err)
			}
			if result.RowsAffected() != 1 {
				return domain.Conflict("AGENT_INSTANCE_DISPATCH_CONFLICT", "节点 AgentInstance 已被更新")
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_attempts(tenant_id,id,job_run_id,node_run_id,agent_instance_id,context_view_id,attempt_no,harness_kind,capabilities,session_ref,state,lease_owner,fence_token,gateway_token_hash,gateway_expires_at,lease_expires_at,output_refs,result_digest,safe_summary,error_code,version,created_at,started_at,finished_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)`, attempt.TenantID, attempt.ID, attempt.JobRunID, attempt.NodeRunID, attempt.AgentInstanceID, attempt.ContextViewID, attempt.AttemptNo, attempt.HarnessKind, jsonValue(attempt.Capabilities), attempt.SessionRef, attempt.State, attempt.LeaseOwner, attempt.FenceToken, attempt.GatewayTokenHash, attempt.GatewayExpiresAt, attempt.LeaseExpiresAt, jsonArrayValue(attempt.OutputRefs), attempt.ResultDigest, jsonValue(attempt.SafeSummary), attempt.ErrorCode, attempt.Version, attempt.CreatedAt, attempt.StartedAt, attempt.FinishedAt, attempt.UpdatedAt); err != nil {
			return dbError(err)
		}
		if err := reserveResourcesTx(ctx, tx, reservations); err != nil {
			return err
		}
		updated, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$3,attempt_count=$4,lease_owner=$5,fence_token=$6,lease_expires_at=$7,version=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2 AND version=$10 AND state='ready'`, node.TenantID, node.ID, node.State, node.AttemptCount, node.LeaseOwner, node.FenceToken, node.LeaseExpiresAt, node.Version, node.UpdatedAt, expectedNodeVersion)
		if err != nil {
			return dbError(err)
		}
		if updated.RowsAffected() != 1 {
			return domain.Conflict("NODE_DISPATCH_CONFLICT", "执行节点已经被其他执行者领取")
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return node, attempt, agent, err
}

func (s *Store) ActivateDispatch(ctx context.Context, node domain.NodeRun, expectedNodeVersion int, attempt domain.RuntimeAttempt, expectedAttemptVersion int, agent domain.AgentInstance, expectedAgentVersion int, event domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error) {
	if err := node.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := attempt.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := agent.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	err := s.withTenantCommand(ctx, node.TenantID, "runtime.activate_dispatch", func(tx pgx.Tx) error {
		currentNode, currentAttempt, currentAgent, err := lockDispatchState(ctx, tx, node.TenantID, node.ID, attempt.ID, agent.ID)
		if err != nil {
			return err
		}
		if currentNode.Version != expectedNodeVersion || currentAttempt.Version != expectedAttemptVersion || currentAgent.Version != expectedAgentVersion {
			return domain.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被其他执行者更新")
		}
		if currentNode.State != domain.NodeLeased || currentAttempt.State != domain.RuntimeAttemptPrepared || currentAgent.State != domain.AgentRunnable || currentNode.LeaseOwner != currentAttempt.LeaseOwner || currentAttempt.LeaseOwner != event.ActorID || currentNode.FenceToken == "" || currentNode.FenceToken != currentAttempt.FenceToken || currentNode.FenceToken != node.FenceToken || currentAttempt.LeaseExpiresAt == nil || !currentAttempt.LeaseExpiresAt.After(attempt.UpdatedAt) {
			return domain.Conflict("DISPATCH_LEASE_STALE", "调度租约无效、已过期或不属于当前执行者")
		}
		if currentAttempt.NodeRunID != currentNode.ID || currentAttempt.AgentInstanceID != currentAgent.ID || currentAttempt.ContextViewID != currentAgent.ContextViewID {
			return domain.Conflict("DISPATCH_SCOPE_INVALID", "调度对象不属于同一 Node、Attempt 或 Agent")
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
		if node.Version != expectedNodeVersion+1 || attempt.Version != expectedAttemptVersion+1 || agent.Version != expectedAgentVersion+1 || attempt.SessionRef == "" || agent.SessionRef != attempt.SessionRef {
			return domain.Invalid("DISPATCH_ACTIVATE_INVALID", "激活后的调度状态不一致")
		}
		if err := updateDispatchStateTx(ctx, tx, node, expectedNodeVersion, attempt, expectedAttemptVersion, agent, expectedAgentVersion); err != nil {
			return err
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return node, attempt, agent, err
}

func (s *Store) HeartbeatDispatch(ctx context.Context, tenantID, attemptID, owner, fenceToken string, expectedNodeVersion, expectedAttemptVersion int, now time.Time, leaseFor time.Duration) (domain.NodeRun, domain.RuntimeAttempt, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(fenceToken) == "" || leaseFor <= 0 {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.Invalid("DISPATCH_HEARTBEAT_INVALID", "调度心跳需要执行者和正数时长")
	}
	var node domain.NodeRun
	var attempt domain.RuntimeAttempt
	err := s.withTenantCommand(ctx, tenantID, "runtime.heartbeat_dispatch", func(tx pgx.Tx) error {
		currentAttempt, err := scanRuntimeAttempt(tx.QueryRow(ctx, runtimeAttemptSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, attemptID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("RuntimeAttempt")
		}
		if err != nil {
			return err
		}
		currentNode, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, currentAttempt.NodeRunID))
		if err != nil {
			return err
		}
		currentAttempt, err = scanRuntimeAttempt(tx.QueryRow(ctx, runtimeAttemptSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, attemptID))
		if err != nil {
			return err
		}
		if currentNode.Version != expectedNodeVersion || currentAttempt.Version != expectedAttemptVersion {
			return domain.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被更新，请重新读取")
		}
		if currentNode.State != domain.NodeRunning || currentAttempt.State != domain.RuntimeAttemptRunning || currentNode.LeaseOwner != owner || currentAttempt.LeaseOwner != owner || currentNode.FenceToken != fenceToken || currentAttempt.FenceToken != fenceToken || currentNode.LeaseExpiresAt == nil || currentAttempt.LeaseExpiresAt == nil || !currentNode.LeaseExpiresAt.After(now) || !currentAttempt.LeaseExpiresAt.After(now) {
			return domain.Conflict("DISPATCH_LEASE_STALE", "调度租约无效、已过期或不属于当前执行者")
		}
		expires := now.Add(leaseFor)
		currentNode.LeaseExpiresAt = &expires
		currentNode.Version++
		currentNode.UpdatedAt = now
		currentAttempt.LeaseExpiresAt = &expires
		currentAttempt.Version++
		currentAttempt.UpdatedAt = now
		nodeResult, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET lease_expires_at=$3,version=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2 AND version=$6 AND state='running' AND lease_owner=$7 AND fence_token=$8`, tenantID, currentNode.ID, currentNode.LeaseExpiresAt, currentNode.Version, currentNode.UpdatedAt, expectedNodeVersion, owner, fenceToken)
		if err != nil {
			return dbError(err)
		}
		attemptResult, err := tx.Exec(ctx, `UPDATE runtime_attempts SET lease_expires_at=$3,version=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2 AND version=$6 AND state='running' AND lease_owner=$7 AND fence_token=$8`, tenantID, currentAttempt.ID, currentAttempt.LeaseExpiresAt, currentAttempt.Version, currentAttempt.UpdatedAt, expectedAttemptVersion, owner, fenceToken)
		if err != nil {
			return dbError(err)
		}
		if nodeResult.RowsAffected() != 1 || attemptResult.RowsAffected() != 1 {
			return domain.Conflict("DISPATCH_LEASE_STALE", "调度租约已经失效，请重新领取")
		}
		if _, err := tx.Exec(ctx, `UPDATE runtime_resource_reservations SET expires_at=$3,updated_at=$4 WHERE tenant_id=$1 AND attempt_id=$2 AND state='held' AND fence_token=$5`, tenantID, currentAttempt.ID, expires, now, fenceToken); err != nil {
			return dbError(err)
		}
		node, attempt = currentNode, currentAttempt
		return nil
	})
	return node, attempt, err
}

func (s *Store) FinalizeDispatch(ctx context.Context, node domain.NodeRun, expectedNodeVersion int, attempt domain.RuntimeAttempt, expectedAttemptVersion int, agent domain.AgentInstance, expectedAgentVersion int, fenceToken string, event domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error) {
	if err := node.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := attempt.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	if err := agent.Validate(); err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	resultNode, resultAttempt, resultAgent := node, attempt, agent
	err := s.withTenantCommand(ctx, node.TenantID, "runtime.finalize_dispatch", func(tx pgx.Tx) error {
		currentNode, currentAttempt, currentAgent, err := lockDispatchState(ctx, tx, node.TenantID, node.ID, attempt.ID, agent.ID)
		if err != nil {
			return err
		}
		if currentAttempt.Terminal() {
			if currentAttempt.State == attempt.State && currentAttempt.ResultDigest == attempt.ResultDigest {
				resultNode, resultAttempt, resultAgent = currentNode, currentAttempt, currentAgent
				return nil
			}
			return domain.Conflict("RUNTIME_ATTEMPT_RESULT_CONFLICT", "RuntimeAttempt 已收到不同的终态结果")
		}
		if currentNode.Version != expectedNodeVersion || currentAttempt.Version != expectedAttemptVersion || currentAgent.Version != expectedAgentVersion {
			return domain.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被其他执行者更新")
		}
		if currentAttempt.NodeRunID != currentNode.ID || currentAttempt.AgentInstanceID != currentAgent.ID || currentAttempt.ContextViewID != currentAgent.ContextViewID {
			return domain.Conflict("DISPATCH_SCOPE_INVALID", "调度对象不属于同一 Node、Attempt 或 Agent")
		}
		if currentNode.LeaseOwner == "" || currentNode.LeaseOwner != currentAttempt.LeaseOwner || currentAttempt.LeaseOwner != event.ActorID || strings.TrimSpace(fenceToken) == "" || currentNode.FenceToken != fenceToken || currentAttempt.FenceToken != fenceToken || currentNode.LeaseExpiresAt == nil || currentAttempt.LeaseExpiresAt == nil || !currentNode.LeaseExpiresAt.After(event.OccurredAt) || !currentAttempt.LeaseExpiresAt.After(event.OccurredAt) {
			return domain.Conflict("DISPATCH_LEASE_STALE", "终态结果不属于当前调度租约")
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
		if node.Version != expectedNodeVersion+1 || attempt.Version != expectedAttemptVersion+1 || agent.Version != expectedAgentVersion+1 {
			return domain.Invalid("DISPATCH_FINALIZE_INVALID", "终态调度对象的版本不一致")
		}
		if err := updateDispatchStateTx(ctx, tx, node, expectedNodeVersion, attempt, expectedAttemptVersion, agent, expectedAgentVersion); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE runtime_resource_reservations SET state='consumed',fence_token='',expires_at=NULL,released_at=$3,updated_at=$3 WHERE tenant_id=$1 AND attempt_id=$2 AND state='held' AND fence_token=$4`, attempt.TenantID, attempt.ID, event.OccurredAt, fenceToken); err != nil {
			return dbError(err)
		}
		_, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return resultNode, resultAttempt, resultAgent, err
}

func lockDispatchState(ctx context.Context, tx pgx.Tx, tenantID, nodeID, attemptID, agentID string) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error) {
	node, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, nodeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.NotFound("执行节点")
	}
	if err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	attempt, err := scanRuntimeAttempt(tx.QueryRow(ctx, runtimeAttemptSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, attemptID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.NotFound("RuntimeAttempt")
	}
	if err != nil {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, err
	}
	agent, err := scanRuntimeAgent(tx.QueryRow(ctx, runtimeAgentSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, agentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NodeRun{}, domain.RuntimeAttempt{}, domain.AgentInstance{}, domain.NotFound("AgentInstance")
	}
	return node, attempt, agent, err
}

func updateDispatchStateTx(ctx context.Context, tx pgx.Tx, node domain.NodeRun, expectedNodeVersion int, attempt domain.RuntimeAttempt, expectedAttemptVersion int, agent domain.AgentInstance, expectedAgentVersion int) error {
	nodeResult, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$3,output_refs=$4,output_digest=$5,error_code=$6,lease_owner=$7,fence_token=$8,lease_expires_at=$9,version=$10,updated_at=$11 WHERE tenant_id=$1 AND id=$2 AND version=$12`, node.TenantID, node.ID, node.State, jsonArrayValue(node.OutputRefs), node.OutputDigest, node.ErrorCode, node.LeaseOwner, node.FenceToken, node.LeaseExpiresAt, node.Version, node.UpdatedAt, expectedNodeVersion)
	if err != nil {
		return dbError(err)
	}
	attemptResult, err := tx.Exec(ctx, `UPDATE runtime_attempts SET session_ref=$3,state=$4,lease_owner=$5,fence_token=$6,lease_expires_at=$7,output_refs=$8,result_digest=$9,safe_summary=$10,error_code=$11,version=$12,started_at=$13,finished_at=$14,updated_at=$15 WHERE tenant_id=$1 AND id=$2 AND version=$16`, attempt.TenantID, attempt.ID, attempt.SessionRef, attempt.State, attempt.LeaseOwner, attempt.FenceToken, attempt.LeaseExpiresAt, jsonArrayValue(attempt.OutputRefs), attempt.ResultDigest, jsonValue(attempt.SafeSummary), attempt.ErrorCode, attempt.Version, attempt.StartedAt, attempt.FinishedAt, attempt.UpdatedAt, expectedAttemptVersion)
	if err != nil {
		return dbError(err)
	}
	agentResult, err := tx.Exec(ctx, `UPDATE runtime_agent_instances SET session_ref=$3,context_view_id=$4,state=$5,remaining_descendants=$6,budget_minor=$7,used_cost_minor=$8,version=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2 AND version=$11`, agent.TenantID, agent.ID, agent.SessionRef, agent.ContextViewID, agent.State, agent.RemainingDescendants, agent.BudgetMinor, agent.UsedCostMinor, agent.Version, agent.UpdatedAt, expectedAgentVersion)
	if err != nil {
		return dbError(err)
	}
	if nodeResult.RowsAffected() != 1 || attemptResult.RowsAffected() != 1 || agentResult.RowsAffected() != 1 {
		return domain.Conflict("DISPATCH_VERSION_CONFLICT", "调度状态已经被其他执行者更新")
	}
	return nil
}

func appendRuntimeEventTx(ctx context.Context, tx pgx.Tx, event domain.JobEvent) (domain.JobEvent, error) {
	if event.ID == "" || event.TenantID == "" || event.JobRunID == "" || strings.TrimSpace(event.Type) == "" || strings.TrimSpace(event.ActorType) == "" || event.OccurredAt.IsZero() {
		return event, domain.Invalid("JOB_EVENT_INVALID", "JobEvent 缺少执行实例、类型或执行者")
	}
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	var lockedJobID string
	if err := tx.QueryRow(ctx, `SELECT id FROM runtime_job_runs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, event.TenantID, event.JobRunID).Scan(&lockedJobID); err != nil {
		return event, dbError(err)
	}
	if event.IdempotencyKey != "" {
		existing, err := scanRuntimeEvent(tx.QueryRow(ctx, `SELECT tenant_id,id,job_run_id,sequence,type,node_key,actor_type,actor_id,correlation_id,idempotency_key,payload,occurred_at FROM runtime_job_events WHERE tenant_id=$1 AND job_run_id=$2 AND idempotency_key=$3`, event.TenantID, event.JobRunID, event.IdempotencyKey))
		if err == nil {
			if err := enqueueRuntimeOutboxTx(ctx, tx, existing); err != nil {
				return existing, err
			}
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return event, err
		}
	}
	var nextSequence int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM runtime_job_events WHERE tenant_id=$1 AND job_run_id=$2`, event.TenantID, event.JobRunID).Scan(&nextSequence); err != nil {
		return event, err
	}
	if event.Sequence != 0 && event.Sequence != nextSequence {
		return event, domain.Conflict("JOB_EVENT_SEQUENCE_CONFLICT", "JobEvent 序号必须连续")
	}
	event.Sequence = nextSequence
	_, err := tx.Exec(ctx, `INSERT INTO runtime_job_events(tenant_id,id,job_run_id,sequence,type,node_key,actor_type,actor_id,correlation_id,idempotency_key,payload,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, event.TenantID, event.ID, event.JobRunID, event.Sequence, event.Type, event.NodeKey, event.ActorType, event.ActorID, event.CorrelationID, event.IdempotencyKey, jsonValue(event.Payload), event.OccurredAt)
	if err != nil {
		return event, dbError(err)
	}
	if err := enqueueRuntimeOutboxTx(ctx, tx, event); err != nil {
		return event, err
	}
	return event, nil
}

func enqueueRuntimeOutboxTx(ctx context.Context, tx pgx.Tx, event domain.JobEvent) error {
	if _, err := tx.Exec(ctx, `INSERT INTO runtime_outbox(tenant_id,id,event_id,schema_version,topic,aggregate_id,payload,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (tenant_id,event_id) DO NOTHING`, event.TenantID, event.ID, event.ID, domain.RuntimeEventSchema, "runtime.job_event", event.JobRunID, jsonValue(map[string]any{"event_id": event.ID, "job_run_id": event.JobRunID, "sequence": event.Sequence, "type": event.Type, "payload": event.Payload}), event.OccurredAt); err != nil {
		return dbError(err)
	}
	for _, subscriber := range domain.RuntimeOutboxSubscribers(event.Type) {
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_outbox_receipts(tenant_id,message_id,subscriber,next_attempt_at,created_at) VALUES($1,$2,$3,$4,$4) ON CONFLICT (tenant_id,message_id,subscriber) DO NOTHING`, event.TenantID, event.ID, subscriber, event.OccurredAt); err != nil {
			return dbError(err)
		}
	}
	return nil
}
