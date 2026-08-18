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

const runtimePlanSelect = `SELECT tenant_id,id,COALESCE(base_revision_id,''),graph_version,patch_key,patch_reason,sop_id,sop_version,sop_digest,schema_version,digest,customer_steps,limits,compiled_at,compiled_by FROM runtime_plan_revisions`
const runtimeJobSelect = `SELECT tenant_id,id,project_id,work_task_id,business_type,input_snapshot_id,business_output_count,plan_revision_id,plan_digest,binding_digest,input_digest,runtime_policy_id,contract_major,contract_minor,root_job_run_id,source_job_run_id,checkpoint_id,idempotency_key,state,priority,version,error_code,created_by,created_at,updated_at FROM runtime_job_runs`
const runtimeNodeSelect = `SELECT tenant_id,id,job_run_id,node_key,state,attempt_count,output_refs,output_digest,error_code,lease_owner,fence_token,lease_expires_at,version,created_at,updated_at FROM runtime_node_runs`
const runtimeContextViewSelect = `SELECT tenant_id,id,job_run_id,node_run_id,attempt_id,schema_version,input_refs,state_refs,event_refs,allowed_tools,max_tokens,budget_minor,digest,created_at,expires_at FROM runtime_context_views`
const runtimeAgentSelect = `SELECT tenant_id,id,job_run_id,node_run_id,COALESCE(parent_agent_instance_id,''),role,harness_kind,session_ref,execution_profile_id,context_view_id,state,depth,remaining_descendants,budget_minor,used_cost_minor,version,created_at,updated_at FROM runtime_agent_instances`
const runtimeStateSelect = `SELECT tenant_id,id,job_run_id,collection,schema_version,revision,values,updated_at FROM runtime_states`
const runtimeCheckpointSelect = `SELECT tenant_id,id,job_run_id,node_key,plan_digest,state_refs,state_watermarks,output_refs,completed_nodes,event_cursor,side_effect_watermark,parent_checkpoint_id,digest,created_at FROM runtime_checkpoints`
const runtimeEffectSelect = `SELECT tenant_id,id,job_run_id,node_run_id,COALESCE(attempt_id,''),COALESCE(resource_reservation_id,''),kind,idempotency_key,state,external_id,request_digest,response_digest,cost_minor,currency,safe_summary,error_code,version,created_at,updated_at FROM runtime_effects`

func scanRuntimePlan(row pgx.Row) (contentruntime.JobPlanRevision, error) {
	var value contentruntime.JobPlanRevision
	var steps, limits []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.BaseRevisionID, &value.GraphVersion, &value.PatchKey, &value.PatchReason, &value.SOPID, &value.SOPVersion, &value.SOPDigest, &value.SchemaVersion, &value.Digest, &steps, &limits, &value.CompiledAt, &value.CompiledBy)
	if err == nil {
		value.CustomerSteps, err = decodeJSON[[]contentruntime.JobPlanCustomerStep](steps)
	}
	if err == nil {
		value.Limits, err = decodeJSON[contentruntime.RuntimeLimits](limits)
	}
	value.NormalizeCollections()
	return value, err
}

func scanRuntimePlanNode(row pgx.Row) (contentruntime.JobPlanNode, error) {
	var value contentruntime.JobPlanNode
	var inputRefs, capabilities, modes []byte
	err := row.Scan(&value.Key, &value.Kind, &value.StageID, &value.GateID, &value.Name, &inputRefs, &value.OutputSchema, &capabilities, &modes, &value.CustomerStepID, &value.SideEffectClass, &value.RetryMaxAttempts)
	if err == nil {
		value.InputRefs, err = decodeJSON[[]string](inputRefs)
	}
	if err == nil {
		value.RequiredCapabilities, err = decodeJSON[[]string](capabilities)
	}
	if err == nil {
		value.ExecutionModes, err = decodeJSON[[]string](modes)
	}
	if value.DependsOn == nil {
		value.DependsOn = []string{}
	}
	if value.InputRefs == nil {
		value.InputRefs = []string{}
	}
	if value.RequiredCapabilities == nil {
		value.RequiredCapabilities = []string{}
	}
	if value.ExecutionModes == nil {
		value.ExecutionModes = []string{}
	}
	return value, dbError(err)
}

func loadRuntimePlanGraph(ctx context.Context, tx pgx.Tx, value *contentruntime.JobPlanRevision) error {
	rows, err := tx.Query(ctx, `SELECT node_key,kind,stage_id,gate_id,name,input_refs,output_schema,required_capabilities,execution_modes,customer_step_id,side_effect_class,retry_max_attempts FROM runtime_plan_nodes WHERE tenant_id=$1 AND revision_id=$2 ORDER BY node_key`, value.TenantID, value.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		node, scanErr := scanRuntimePlanNode(rows)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		value.Nodes = append(value.Nodes, node)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	edges, err := tx.Query(ctx, `SELECT from_key,to_key FROM runtime_plan_edges WHERE tenant_id=$1 AND revision_id=$2 ORDER BY from_key,to_key`, value.TenantID, value.ID)
	if err != nil {
		return err
	}
	for edges.Next() {
		var edge contentruntime.JobPlanEdge
		if err := edges.Scan(&edge.From, &edge.To); err != nil {
			edges.Close()
			return err
		}
		value.Edges = append(value.Edges, edge)
		for index := range value.Nodes {
			if value.Nodes[index].Key == edge.To {
				value.Nodes[index].DependsOn = append(value.Nodes[index].DependsOn, edge.From)
			}
		}
	}
	if err := edges.Err(); err != nil {
		edges.Close()
		return err
	}
	edges.Close()
	value.NormalizeCollections()
	return nil
}

func (s *Store) CreatePlan(ctx context.Context, value contentruntime.JobPlanRevision) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		return insertRuntimePlanTx(ctx, tx, value)
	})
}

func insertRuntimePlanTx(ctx context.Context, tx pgx.Tx, value contentruntime.JobPlanRevision) error {
	baseRevisionID := any(nil)
	if value.BaseRevisionID != "" {
		baseRevisionID = value.BaseRevisionID
	}
	_, err := tx.Exec(ctx, `INSERT INTO runtime_plan_revisions(tenant_id,id,base_revision_id,graph_version,patch_key,patch_reason,sop_id,sop_version,sop_digest,schema_version,digest,customer_steps,limits,compiled_at,compiled_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, value.TenantID, value.ID, baseRevisionID, value.GraphVersion, value.PatchKey, value.PatchReason, value.SOPID, value.SOPVersion, value.SOPDigest, value.SchemaVersion, value.Digest, jsonArrayValue(value.CustomerSteps), jsonValue(value.Limits), value.CompiledAt, value.CompiledBy)
	if err != nil {
		return dbError(err)
	}
	for _, node := range value.Nodes {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_plan_nodes(tenant_id,revision_id,node_key,kind,stage_id,gate_id,name,input_refs,output_schema,required_capabilities,execution_modes,customer_step_id,side_effect_class,retry_max_attempts) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, value.TenantID, value.ID, node.Key, node.Kind, node.StageID, node.GateID, node.Name, jsonArrayValue(node.InputRefs), node.OutputSchema, jsonArrayValue(node.RequiredCapabilities), jsonArrayValue(node.ExecutionModes), node.CustomerStepID, node.SideEffectClass, node.RetryMaxAttempts)
		if err != nil {
			return dbError(err)
		}
	}
	for _, edge := range value.Edges {
		if _, err := tx.Exec(ctx, `INSERT INTO runtime_plan_edges(tenant_id,revision_id,from_key,to_key) VALUES($1,$2,$3,$4)`, value.TenantID, value.ID, edge.From, edge.To); err != nil {
			return dbError(err)
		}
	}
	return nil
}

func (s *Store) Plan(ctx context.Context, tenantID, id string) (contentruntime.JobPlanRevision, error) {
	var result contentruntime.JobPlanRevision
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimePlan(tx.QueryRow(ctx, runtimePlanSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("执行计划")
		}
		if err != nil {
			return err
		}
		if err := loadRuntimePlanGraph(ctx, tx, &result); err != nil {
			return err
		}
		return nil
	})
	return result, err
}

func (s *Store) Plans(ctx context.Context, tenantID string) ([]contentruntime.JobPlanRevision, error) {
	result := []contentruntime.JobPlanRevision{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimePlanSelect+` WHERE tenant_id=$1 ORDER BY compiled_at DESC`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimePlan(rows)
			if err != nil {
				rows.Close()
				return err
			}
			result = append(result, value)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for index := range result {
			if err := loadRuntimePlanGraph(ctx, tx, &result[index]); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func (s *Store) CreateJobBundle(ctx context.Context, job contentruntime.JobRun, nodes []contentruntime.NodeRun, event contentruntime.JobEvent) error {
	if err := job.Validate(); err != nil {
		return err
	}
	return s.withTenantCommand(ctx, job.TenantID, "runtime.create_job_bundle", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_job_runs(tenant_id,id,project_id,work_task_id,business_type,input_snapshot_id,business_output_count,plan_revision_id,plan_digest,binding_digest,input_digest,runtime_policy_id,contract_major,contract_minor,root_job_run_id,source_job_run_id,checkpoint_id,idempotency_key,state,priority,version,error_code,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)`, job.TenantID, job.ID, job.ProjectID, job.WorkTaskID, job.BusinessType, job.InputSnapshotID, job.BusinessOutputCount, job.PlanRevisionID, job.PlanDigest, job.BindingDigest, job.InputDigest, job.RuntimePolicyID, job.ContractMajor, job.ContractMinor, job.RootJobRunID, job.SourceJobRunID, job.CheckpointID, job.IdempotencyKey, job.State, job.Priority, job.Version, job.ErrorCode, job.CreatedBy, job.CreatedAt, job.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		for _, node := range nodes {
			if err := node.Validate(); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO runtime_node_runs(tenant_id,id,job_run_id,node_key,state,attempt_count,output_refs,output_digest,error_code,lease_owner,fence_token,lease_expires_at,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, node.TenantID, node.ID, node.JobRunID, node.NodeKey, node.State, node.AttemptCount, jsonArrayValue(node.OutputRefs), node.OutputDigest, node.ErrorCode, node.LeaseOwner, node.FenceToken, node.LeaseExpiresAt, node.Version, node.CreatedAt, node.UpdatedAt)
			if err != nil {
				return dbError(err)
			}
		}
		if event.Sequence != 1 {
			return fault.Invalid("JOB_EVENT_SEQUENCE_INVALID", "初始 JobEvent 序号必须为 1")
		}
		if _, err := appendRuntimeEventTx(ctx, tx, event); err != nil {
			return err
		}
		return nil
	})
}

func scanRuntimeJob(row pgx.Row) (contentruntime.JobRun, error) {
	var value contentruntime.JobRun
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.WorkTaskID, &value.BusinessType, &value.InputSnapshotID, &value.BusinessOutputCount, &value.PlanRevisionID, &value.PlanDigest, &value.BindingDigest, &value.InputDigest, &value.RuntimePolicyID, &value.ContractMajor, &value.ContractMinor, &value.RootJobRunID, &value.SourceJobRunID, &value.CheckpointID, &value.IdempotencyKey, &value.State, &value.Priority, &value.Version, &value.ErrorCode, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	return value, dbError(err)
}

func (s *Store) JobRun(ctx context.Context, tenantID, id string) (contentruntime.JobRun, error) {
	var result contentruntime.JobRun
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeJob(tx.QueryRow(ctx, runtimeJobSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("执行实例")
		}
		return err
	})
	return result, err
}

func (s *Store) JobRunByIdempotencyKey(ctx context.Context, tenantID, key string) (contentruntime.JobRun, error) {
	var result contentruntime.JobRun
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeJob(tx.QueryRow(ctx, runtimeJobSelect+` WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("执行实例")
		}
		return err
	})
	return result, err
}

func (s *Store) JobRuns(ctx context.Context, tenantID, taskID string) ([]contentruntime.JobRun, error) {
	result := []contentruntime.JobRun{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeJobSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if taskID != "" {
			query += ` AND work_task_id=$2`
			args = append(args, taskID)
		}
		query += ` ORDER BY created_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeJob(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) JobRunsPage(ctx context.Context, tenantID, projectID, state string, after, limit int) ([]contentruntime.JobRun, bool, error) {
	if after < 0 || limit < 1 {
		return nil, false, fault.Invalid("RUNTIME_PAGE_INVALID", "Runtime 分页参数无效")
	}
	result := []contentruntime.JobRun{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeJobSelect+` WHERE tenant_id=$1 AND (NULLIF($2,'')::uuid IS NULL OR project_id=NULLIF($2,'')::uuid) AND ($3='' OR state=$3) ORDER BY updated_at DESC,id DESC OFFSET $4 LIMIT $5`, tenantID, projectID, state, after, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeJob(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func scanRuntimeNode(row pgx.Row) (contentruntime.NodeRun, error) {
	var value contentruntime.NodeRun
	var outputs []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.NodeKey, &value.State, &value.AttemptCount, &outputs, &value.OutputDigest, &value.ErrorCode, &value.LeaseOwner, &value.FenceToken, &value.LeaseExpiresAt, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.OutputRefs, err = decodeJSON[[]string](outputs)
	}
	if value.OutputRefs == nil {
		value.OutputRefs = []string{}
	}
	return value, dbError(err)
}

func (s *Store) NodeRuns(ctx context.Context, tenantID, jobID string) ([]contentruntime.NodeRun, error) {
	result := []contentruntime.NodeRun{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND job_run_id=$2 ORDER BY created_at`, tenantID, jobID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeNode(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) NodeRunsPage(ctx context.Context, tenantID, jobID string, after, limit int) ([]contentruntime.NodeRun, bool, error) {
	if after < 0 || limit < 1 {
		return nil, false, fault.Invalid("RUNTIME_PAGE_INVALID", "Runtime 分页参数无效")
	}
	result := []contentruntime.NodeRun{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND job_run_id=$2 ORDER BY created_at,id OFFSET $3 LIMIT $4`, tenantID, jobID, after, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeNode(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func (s *Store) NodeRun(ctx context.Context, tenantID, id string) (contentruntime.NodeRun, error) {
	var result contentruntime.NodeRun
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("执行节点")
		}
		return err
	})
	return result, err
}

func scanRuntimeContextView(row pgx.Row) (contentruntime.ContextView, error) {
	var value contentruntime.ContextView
	var inputs, states, events, tools []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.NodeRunID, &value.AttemptID, &value.SchemaVersion, &inputs, &states, &events, &tools, &value.MaxTokens, &value.BudgetMinor, &value.Digest, &value.CreatedAt, &value.ExpiresAt)
	if err == nil {
		value.InputRefs, err = decodeJSON[[]string](inputs)
	}
	if err == nil {
		value.StateRefs, err = decodeJSON[[]string](states)
	}
	if err == nil {
		value.EventRefs, err = decodeJSON[[]string](events)
	}
	if err == nil {
		value.AllowedTools, err = decodeJSON[[]string](tools)
	}
	if value.InputRefs == nil {
		value.InputRefs = []string{}
	}
	if value.StateRefs == nil {
		value.StateRefs = []string{}
	}
	if value.EventRefs == nil {
		value.EventRefs = []string{}
	}
	if value.AllowedTools == nil {
		value.AllowedTools = []string{}
	}
	return value, dbError(err)
}

func (s *Store) CreateContextView(ctx context.Context, value contentruntime.ContextView) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_context_views(tenant_id,id,job_run_id,node_run_id,attempt_id,schema_version,input_refs,state_refs,event_refs,allowed_tools,max_tokens,budget_minor,digest,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, value.TenantID, value.ID, value.JobRunID, value.NodeRunID, value.AttemptID, value.SchemaVersion, jsonArrayValue(value.InputRefs), jsonArrayValue(value.StateRefs), jsonArrayValue(value.EventRefs), jsonArrayValue(value.AllowedTools), value.MaxTokens, value.BudgetMinor, value.Digest, value.CreatedAt, value.ExpiresAt)
		return dbError(err)
	})
}

func (s *Store) ContextView(ctx context.Context, tenantID, id string) (contentruntime.ContextView, error) {
	var result contentruntime.ContextView
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeContextView(tx.QueryRow(ctx, runtimeContextViewSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("ContextView")
		}
		return err
	})
	return result, err
}

func (s *Store) ContextViews(ctx context.Context, tenantID, jobID string) ([]contentruntime.ContextView, error) {
	result := []contentruntime.ContextView{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeContextViewSelect + ` WHERE tenant_id=$1`
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
			value, err := scanRuntimeContextView(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func scanRuntimeAgent(row pgx.Row) (contentruntime.AgentInstance, error) {
	var value contentruntime.AgentInstance
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.NodeRunID, &value.ParentAgentInstanceID, &value.Role, &value.HarnessKind, &value.SessionRef, &value.ExecutionProfileID, &value.ContextViewID, &value.State, &value.Depth, &value.RemainingDescendants, &value.BudgetMinor, &value.UsedCostMinor, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	return value, dbError(err)
}

func (s *Store) CreateAgentInstance(ctx context.Context, value contentruntime.AgentInstance) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		if value.ParentAgentInstanceID != "" {
			allocation := 1 + value.RemainingDescendants
			result, err := tx.Exec(ctx, `UPDATE runtime_agent_instances AS parent
				SET remaining_descendants=parent.remaining_descendants-$6,version=parent.version+1,updated_at=$7
				FROM runtime_context_views AS parent_view,runtime_context_views AS child_view
				WHERE parent.tenant_id=$1 AND parent.id=$2 AND parent.job_run_id=$3 AND parent.depth+1=$4
				  AND parent.remaining_descendants >= $6
				  AND parent.budget_minor-parent.used_cost_minor >= $5
				  AND parent_view.tenant_id=parent.tenant_id AND parent_view.id=parent.context_view_id
				  AND child_view.tenant_id=$1 AND child_view.id=$8
				  AND child_view.allowed_tools <@ parent_view.allowed_tools`, value.TenantID, value.ParentAgentInstanceID, value.JobRunID, value.Depth, value.BudgetMinor, allocation, value.CreatedAt, value.ContextViewID)
			if err != nil {
				return dbError(err)
			}
			if result.RowsAffected() != 1 {
				return fault.Conflict("AGENT_INSTANCE_PARENT_CONFLICT", "父 AgentInstance 的范围、权限、预算或派生额度已经变化")
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO runtime_agent_instances(tenant_id,id,job_run_id,node_run_id,parent_agent_instance_id,role,harness_kind,session_ref,execution_profile_id,context_view_id,state,depth,remaining_descendants,budget_minor,used_cost_minor,version,created_at,updated_at) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, value.TenantID, value.ID, value.JobRunID, value.NodeRunID, value.ParentAgentInstanceID, value.Role, value.HarnessKind, value.SessionRef, value.ExecutionProfileID, value.ContextViewID, value.State, value.Depth, value.RemainingDescendants, value.BudgetMinor, value.UsedCostMinor, value.Version, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) AgentInstance(ctx context.Context, tenantID, id string) (contentruntime.AgentInstance, error) {
	var result contentruntime.AgentInstance
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeAgent(tx.QueryRow(ctx, runtimeAgentSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("AgentInstance")
		}
		return err
	})
	return result, err
}

func (s *Store) AgentInstances(ctx context.Context, tenantID, jobID string) ([]contentruntime.AgentInstance, error) {
	result := []contentruntime.AgentInstance{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeAgentSelect + ` WHERE tenant_id=$1`
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
			value, err := scanRuntimeAgent(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) SaveAgentInstance(ctx context.Context, value contentruntime.AgentInstance, expectedVersion int) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE runtime_agent_instances SET session_ref=$3,context_view_id=$4,state=$5,remaining_descendants=$6,budget_minor=$7,used_cost_minor=$8,version=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2 AND version=$11`, value.TenantID, value.ID, value.SessionRef, value.ContextViewID, value.State, value.RemainingDescendants, value.BudgetMinor, value.UsedCostMinor, value.Version, value.UpdatedAt, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.Conflict("AGENT_INSTANCE_VERSION_CONFLICT", "AgentInstance 已被更新，请重新读取")
		}
		return nil
	})
}

func scanRuntimeEvent(row pgx.Row) (contentruntime.JobEvent, error) {
	var value contentruntime.JobEvent
	var payload []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.Sequence, &value.Type, &value.NodeKey, &value.ActorType, &value.ActorID, &value.CorrelationID, &value.IdempotencyKey, &payload, &value.OccurredAt)
	if err == nil {
		value.Payload, err = decodeJSON[map[string]any](payload)
	}
	if value.Payload == nil {
		value.Payload = map[string]any{}
	}
	return value, err
}

func (s *Store) AppendRuntimeEvent(ctx context.Context, event contentruntime.JobEvent) (contentruntime.JobEvent, error) {
	result := event
	err := s.withTenantCommand(ctx, event.TenantID, "runtime.append_event", func(tx pgx.Tx) error {
		var err error
		result, err = appendRuntimeEventTx(ctx, tx, event)
		return err
	})
	return result, err
}

func (s *Store) JobEvents(ctx context.Context, tenantID, jobID string, after int64) ([]contentruntime.JobEvent, error) {
	result := []contentruntime.JobEvent{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT tenant_id,id,job_run_id,sequence,type,node_key,actor_type,actor_id,correlation_id,idempotency_key,payload,occurred_at FROM runtime_job_events WHERE tenant_id=$1 AND job_run_id=$2 AND sequence>$3 ORDER BY sequence`, tenantID, jobID, after)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeEvent(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) JobEventsPage(ctx context.Context, tenantID, jobID string, after int64, limit int) ([]contentruntime.JobEvent, error) {
	if limit < 1 {
		return nil, fault.Invalid("RUNTIME_PAGE_INVALID", "Runtime 分页参数无效")
	}
	result := []contentruntime.JobEvent{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT tenant_id,id,job_run_id,sequence,type,node_key,actor_type,actor_id,correlation_id,idempotency_key,payload,occurred_at FROM runtime_job_events WHERE tenant_id=$1 AND job_run_id=$2 AND sequence>$3 ORDER BY sequence LIMIT $4`, tenantID, jobID, after, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeEvent(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func scanRuntimeState(row pgx.Row) (contentruntime.RuntimeState, error) {
	var value contentruntime.RuntimeState
	var values []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.Collection, &value.SchemaVersion, &value.Revision, &values, &value.UpdatedAt)
	if err == nil {
		value.Values, err = decodeJSON[map[string]any](values)
	}
	if value.Values == nil {
		value.Values = map[string]any{}
	}
	return value, dbError(err)
}
func (s *Store) RuntimeState(ctx context.Context, tenantID, jobID, collection string) (contentruntime.RuntimeState, error) {
	var result contentruntime.RuntimeState
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeState(tx.QueryRow(ctx, runtimeStateSelect+` WHERE tenant_id=$1 AND job_run_id=$2 AND collection=$3`, tenantID, jobID, collection))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("运行状态")
		}
		return err
	})
	return result, err
}
func (s *Store) CreateCheckpoint(ctx context.Context, value contentruntime.Checkpoint) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_checkpoints(tenant_id,id,job_run_id,node_key,plan_digest,state_refs,state_watermarks,output_refs,completed_nodes,event_cursor,side_effect_watermark,parent_checkpoint_id,digest,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, value.TenantID, value.ID, value.JobRunID, value.NodeKey, value.PlanDigest, jsonArrayValue(value.StateRefs), jsonValue(value.StateWatermarks), jsonArrayValue(value.OutputRefs), jsonArrayValue(value.CompletedNodes), value.EventCursor, value.SideEffectWatermark, value.ParentCheckpointID, value.Digest, value.CreatedAt)
		return dbError(err)
	})
}
func scanRuntimeCheckpoint(row pgx.Row) (contentruntime.Checkpoint, error) {
	var value contentruntime.Checkpoint
	var states, watermarks, outputs, completed []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.NodeKey, &value.PlanDigest, &states, &watermarks, &outputs, &completed, &value.EventCursor, &value.SideEffectWatermark, &value.ParentCheckpointID, &value.Digest, &value.CreatedAt)
	if err == nil {
		value.StateRefs, err = decodeJSON[[]string](states)
	}
	if err == nil {
		value.StateWatermarks, err = decodeJSON[map[string]int](watermarks)
	}
	if err == nil {
		value.OutputRefs, err = decodeJSON[[]string](outputs)
	}
	if err == nil {
		value.CompletedNodes, err = decodeJSON[[]string](completed)
	}
	return value, dbError(err)
}
func (s *Store) Checkpoint(ctx context.Context, tenantID, id string) (contentruntime.Checkpoint, error) {
	var result contentruntime.Checkpoint
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeCheckpoint(tx.QueryRow(ctx, runtimeCheckpointSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("检查点")
		}
		return err
	})
	return result, err
}
func (s *Store) Checkpoints(ctx context.Context, tenantID, jobID string) ([]contentruntime.Checkpoint, error) {
	result := []contentruntime.Checkpoint{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeCheckpointSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if jobID != "" {
			query += ` AND job_run_id=$2`
			args = append(args, jobID)
		}
		query += ` ORDER BY created_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeCheckpoint(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) CheckpointsPage(ctx context.Context, tenantID, jobID string, after, limit int) ([]contentruntime.Checkpoint, bool, error) {
	if after < 0 || limit < 1 {
		return nil, false, fault.Invalid("RUNTIME_PAGE_INVALID", "Runtime 分页参数无效")
	}
	result := []contentruntime.Checkpoint{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeCheckpointSelect+` WHERE tenant_id=$1 AND job_run_id=$2 ORDER BY created_at DESC,id DESC OFFSET $3 LIMIT $4`, tenantID, jobID, after, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeCheckpoint(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func scanRuntimeEffect(row pgx.Row) (contentruntime.ExternalEffect, error) {
	var value contentruntime.ExternalEffect
	var summary []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.NodeRunID, &value.AttemptID, &value.ResourceReservationID, &value.Kind, &value.IdempotencyKey, &value.State, &value.ExternalID, &value.RequestDigest, &value.ResponseDigest, &value.CostMinor, &value.Currency, &summary, &value.ErrorCode, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.SafeSummary, err = decodeJSON[map[string]any](summary)
	}
	if value.SafeSummary == nil {
		value.SafeSummary = map[string]any{}
	}
	return value, dbError(err)
}
func (s *Store) Effect(ctx context.Context, tenantID, id string) (contentruntime.ExternalEffect, error) {
	var result contentruntime.ExternalEffect
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeEffect(tx.QueryRow(ctx, runtimeEffectSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("外部操作")
		}
		return err
	})
	return result, err
}
func (s *Store) EffectByIdempotencyKey(ctx context.Context, tenantID, key string) (contentruntime.ExternalEffect, error) {
	var result contentruntime.ExternalEffect
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeEffect(tx.QueryRow(ctx, runtimeEffectSelect+` WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("外部操作")
		}
		return err
	})
	return result, err
}

func (s *Store) EffectByExternalID(ctx context.Context, tenantID, providerID, externalID string) (contentruntime.ExternalEffect, error) {
	var result contentruntime.ExternalEffect
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeEffectSelect + ` WHERE tenant_id=$1 AND external_id=$2`
		args := []any{tenantID, externalID}
		if providerID != "" {
			query += ` AND safe_summary->>'provider_id'=$3`
			args = append(args, providerID)
		}
		value, err := scanRuntimeEffect(tx.QueryRow(ctx, query, args...))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("外部操作")
		}
		return err
	})
	return result, err
}
func (s *Store) Effects(ctx context.Context, tenantID, jobID string) ([]contentruntime.ExternalEffect, error) {
	result := []contentruntime.ExternalEffect{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeEffectSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if jobID != "" {
			query += ` AND job_run_id=$2`
			args = append(args, jobID)
		}
		query += ` ORDER BY created_at`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeEffect(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) EffectsPage(ctx context.Context, tenantID, jobID string, after, limit int) ([]contentruntime.ExternalEffect, bool, error) {
	if after < 0 || limit < 1 {
		return nil, false, fault.Invalid("RUNTIME_PAGE_INVALID", "Runtime 分页参数无效")
	}
	result := []contentruntime.ExternalEffect{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeEffectSelect+` WHERE tenant_id=$1 AND job_run_id=$2 ORDER BY created_at,id OFFSET $3 LIMIT $4`, tenantID, jobID, after, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeEffect(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}
func (s *Store) ExpireNodeLeases(ctx context.Context, tenantID string, now time.Time) error {
	return s.withTenantCommand(ctx, tenantID, "runtime.expire_leases", func(tx pgx.Tx) error {
		type expiredCandidate struct {
			AttemptID string
			NodeID    string
			AgentID   string
		}
		expiryEvents := []contentruntime.JobEvent{}
		rows, err := tx.Query(ctx, `SELECT id,node_run_id,agent_instance_id FROM runtime_attempts WHERE tenant_id=$1 AND state IN ('prepared','running') AND lease_expires_at<=$2 ORDER BY node_run_id,id`, tenantID, now)
		if err != nil {
			return err
		}
		candidates := []expiredCandidate{}
		for rows.Next() {
			var candidate expiredCandidate
			if err := rows.Scan(&candidate.AttemptID, &candidate.NodeID, &candidate.AgentID); err != nil {
				rows.Close()
				return err
			}
			candidates = append(candidates, candidate)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, candidate := range candidates {
			node, err := scanRuntimeNode(tx.QueryRow(ctx, runtimeNodeSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, candidate.NodeID))
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			attempt, err := scanRuntimeAttempt(tx.QueryRow(ctx, runtimeAttemptSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, candidate.AttemptID))
			if err != nil {
				return err
			}
			if attempt.LeaseExpiresAt == nil || attempt.LeaseExpiresAt.After(now) || (attempt.State != contentruntime.RuntimeAttemptPrepared && attempt.State != contentruntime.RuntimeAttemptRunning) {
				continue
			}
			expiredOwner := attempt.LeaseOwner
			attempt.State = contentruntime.RuntimeAttemptExpired
			attempt.ErrorCode = "DISPATCH_LEASE_EXPIRED"
			attempt.LeaseOwner = ""
			attempt.LeaseExpiresAt = nil
			attempt.FinishedAt = &now
			attempt.Version++
			attempt.UpdatedAt = now
			if _, err := tx.Exec(ctx, `UPDATE runtime_attempts SET state=$3,error_code=$4,lease_owner='',fence_token='',lease_expires_at=NULL,version=$5,finished_at=$6,updated_at=$6 WHERE tenant_id=$1 AND id=$2`, tenantID, attempt.ID, attempt.State, attempt.ErrorCode, attempt.Version, now); err != nil {
				return dbError(err)
			}
			if _, err := tx.Exec(ctx, `UPDATE runtime_resource_reservations SET state='expired',fence_token='',expires_at=NULL,released_at=$3,updated_at=$3 WHERE tenant_id=$1 AND attempt_id=$2 AND state='held'`, tenantID, attempt.ID, now); err != nil {
				return dbError(err)
			}
			if node.LeaseOwner == expiredOwner && node.LeaseExpiresAt != nil && !node.LeaseExpiresAt.After(now) && (node.State == contentruntime.NodeLeased || node.State == contentruntime.NodeRunning) {
				node.State = contentruntime.NodeLeaseExpired
				node.ErrorCode = "DISPATCH_LEASE_EXPIRED"
				node.LeaseOwner = ""
				node.LeaseExpiresAt = nil
				node.Version++
				node.UpdatedAt = now
				if _, err := tx.Exec(ctx, `UPDATE runtime_node_runs SET state=$3,error_code=$4,lease_owner='',fence_token='',lease_expires_at=NULL,version=$5,updated_at=$6 WHERE tenant_id=$1 AND id=$2`, tenantID, node.ID, node.State, node.ErrorCode, node.Version, now); err != nil {
					return dbError(err)
				}
			}
			agent, err := scanRuntimeAgent(tx.QueryRow(ctx, runtimeAgentSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, candidate.AgentID))
			if err != nil {
				return err
			}
			if agent.State == contentruntime.AgentActive {
				agent.State = contentruntime.AgentRunnable
				agent.Version++
				agent.UpdatedAt = now
				if _, err := tx.Exec(ctx, `UPDATE runtime_agent_instances SET state=$3,version=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2`, tenantID, agent.ID, agent.State, agent.Version, now); err != nil {
					return dbError(err)
				}
			}
			expiryEvents = append(expiryEvents, contentruntime.JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: attempt.JobRunID, NodeKey: node.NodeKey, Type: "attempt.expired", ActorType: "runtime", ActorID: expiredOwner, IdempotencyKey: attempt.ID + ":expired", Payload: map[string]any{"attempt_id": attempt.ID, "error_code": attempt.ErrorCode}, OccurredAt: now})
		}
		// Node-only claims made before PrepareDispatch have no RuntimeAttempt;
		// expire them with the same lease rule until dispatch is fully unified.
		_, err = tx.Exec(ctx, `UPDATE runtime_node_runs AS node SET state='lease_expired',error_code='DISPATCH_LEASE_EXPIRED',lease_owner='',fence_token='',lease_expires_at=NULL,version=node.version+1,updated_at=$2 WHERE node.tenant_id=$1 AND node.state IN ('leased','running') AND node.lease_expires_at<=$2 AND NOT EXISTS (SELECT 1 FROM runtime_attempts AS attempt WHERE attempt.tenant_id=node.tenant_id AND attempt.node_run_id=node.id AND attempt.state IN ('prepared','running'))`, tenantID, now)
		if err != nil {
			return dbError(err)
		}
		for _, event := range expiryEvents {
			if _, err := appendRuntimeEventTx(ctx, tx, event); err != nil {
				return err
			}
		}
		return nil
	})
}
