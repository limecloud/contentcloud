package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const environmentSelect = `SELECT tenant_id,id,name,slug,status,manifest_digest,default_sop_id,default_sop_version,capabilities,created_at,updated_at FROM environments`
const sopDefinitionSelect = `SELECT tenant_id,id,name,description,content_types,current_version,template_key,built_in,source_ref,created_by,created_at,updated_at FROM sop_definitions`
const sopVersionSelect = `SELECT tenant_id,id,sop_id,version,schema_version,name,description,content_types,stages,gates,default_execution_mode,digest,status,created_by,published_by,created_at,published_at FROM sop_versions`
const workTaskSelect = `SELECT tenant_id,id,project_id,environment_id,sop_id,sop_version,sop_digest,title,intent,content_type,input_refs,requested_output,assignee_user_id,priority,due_at,risk_profile,idempotency_key,status,current_stage_id,next_action,created_by,created_at,updated_at FROM work_tasks`
const stageRunSelect = `SELECT tenant_id,id,task_id,stage_id,status,execution_mode,input_refs,output_refs,started_at,completed_at,updated_at FROM stage_runs`
const conversationImportSelect = `SELECT tenant_id,id,project_id,task_id,COALESCE(stage_run_id,''),node_id,client_id,adapter_version,adapter_id,purpose,requested_scope,attach_as,retention_days,status,idempotency_key,COALESCE(bundle::text,''),expires_at,created_by,created_at,updated_at,cancelled_at,uploaded_at FROM conversation_imports`

func scanEnvironment(row pgx.Row) (domain.Environment, error) {
	var value domain.Environment
	var capabilities []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.Name, &value.Slug, &value.Status, &value.ManifestDigest, &value.DefaultSOPID, &value.DefaultSOPVersion, &capabilities, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.Capabilities, err = decodeJSON[[]domain.EnvironmentCapability](capabilities)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateEnvironment(ctx context.Context, value domain.Environment) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO environments(tenant_id,id,name,slug,status,manifest_digest,default_sop_id,default_sop_version,capabilities,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, value.TenantID, value.ID, value.Name, value.Slug, value.Status, value.ManifestDigest, value.DefaultSOPID, value.DefaultSOPVersion, jsonArrayValue(value.Capabilities), value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) Environments(ctx context.Context, tenantID string) ([]domain.Environment, error) {
	result := []domain.Environment{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, environmentSelect+` WHERE tenant_id=$1 ORDER BY created_at`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanEnvironment(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) Environment(ctx context.Context, tenantID, id string) (domain.Environment, error) {
	var result domain.Environment
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanEnvironment(tx.QueryRow(ctx, environmentSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Environment")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveEnvironment(ctx context.Context, value domain.Environment) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE environments SET name=$3,slug=$4,status=$5,manifest_digest=$6,default_sop_id=$7,default_sop_version=$8,capabilities=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.Name, value.Slug, value.Status, value.ManifestDigest, value.DefaultSOPID, value.DefaultSOPVersion, jsonArrayValue(value.Capabilities), value.UpdatedAt)
		if err == nil && command.RowsAffected() == 0 {
			return domain.NotFound("Environment")
		}
		return dbError(err)
	})
}

func scanSOPDefinition(row pgx.Row) (domain.SOPDefinition, error) {
	var value domain.SOPDefinition
	var contentTypes []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.Name, &value.Description, &contentTypes, &value.CurrentVersion, &value.TemplateKey, &value.BuiltIn, &value.SourceRef, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.ContentTypes, err = decodeJSON[[]string](contentTypes)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func scanSOPVersion(row pgx.Row) (domain.SOPVersion, error) {
	var value domain.SOPVersion
	var contentTypes, stages, gates []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.SOPID, &value.Version, &value.SchemaVersion, &value.Name, &value.Description, &contentTypes, &stages, &gates, &value.DefaultExecutionMode, &value.Digest, &value.Status, &value.CreatedBy, &value.PublishedBy, &value.CreatedAt, &value.PublishedAt)
	if err == nil {
		value.ContentTypes, err = decodeJSON[[]string](contentTypes)
	}
	if err == nil {
		value.Stages, err = decodeJSON[[]domain.StageDefinition](stages)
	}
	if err == nil {
		value.Gates, err = decodeJSON[[]domain.GateDefinition](gates)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateSOP(ctx context.Context, definition domain.SOPDefinition, version domain.SOPVersion) error {
	if err := version.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, definition.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO sop_definitions(tenant_id,id,name,description,content_types,current_version,template_key,built_in,source_ref,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, definition.TenantID, definition.ID, definition.Name, definition.Description, jsonArrayValue(definition.ContentTypes), definition.CurrentVersion, definition.TemplateKey, definition.BuiltIn, definition.SourceRef, definition.CreatedBy, definition.CreatedAt, definition.UpdatedAt); err != nil {
			return dbError(err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO sop_versions(tenant_id,id,sop_id,version,schema_version,name,description,content_types,stages,gates,default_execution_mode,digest,status,created_by,published_by,created_at,published_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, version.TenantID, version.ID, version.SOPID, version.Version, version.SchemaVersion, version.Name, version.Description, jsonArrayValue(version.ContentTypes), jsonArrayValue(version.Stages), jsonArrayValue(version.Gates), version.DefaultExecutionMode, version.Digest, version.Status, version.CreatedBy, version.PublishedBy, version.CreatedAt, version.PublishedAt)
		return dbError(err)
	})
}

func (s *Store) SaveSOPDefinition(ctx context.Context, value domain.SOPDefinition) error {
	if value.ID == "" || value.TenantID == "" || value.Name == "" {
		return domain.Invalid("SOP_INVALID", "SOP 定义缺少 ID、租户或名称")
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE sop_definitions SET name=$3,description=$4,content_types=$5,current_version=$6,template_key=$7,built_in=$8,source_ref=$9,updated_at=$10 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.Name, value.Description, jsonArrayValue(value.ContentTypes), value.CurrentVersion, value.TemplateKey, value.BuiltIn, value.SourceRef, value.UpdatedAt)
		if err == nil && command.RowsAffected() == 0 {
			return domain.NotFound("SOP")
		}
		return dbError(err)
	})
}

func (s *Store) CreateSOPVersion(ctx context.Context, version domain.SOPVersion) error {
	if err := version.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, version.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO sop_versions(tenant_id,id,sop_id,version,schema_version,name,description,content_types,stages,gates,default_execution_mode,digest,status,created_by,published_by,created_at,published_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, version.TenantID, version.ID, version.SOPID, version.Version, version.SchemaVersion, version.Name, version.Description, jsonArrayValue(version.ContentTypes), jsonArrayValue(version.Stages), jsonArrayValue(version.Gates), version.DefaultExecutionMode, version.Digest, version.Status, version.CreatedBy, version.PublishedBy, version.CreatedAt, version.PublishedAt)
		return dbError(err)
	})
}

func (s *Store) SOPs(ctx context.Context, tenantID string) ([]domain.SOPSummary, error) {
	result := []domain.SOPSummary{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sopDefinitionSelect+` WHERE tenant_id=$1 ORDER BY updated_at DESC`, tenantID)
		if err != nil {
			return err
		}
		definitions := []domain.SOPDefinition{}
		for rows.Next() {
			definition, err := scanSOPDefinition(rows)
			if err != nil {
				rows.Close()
				return err
			}
			definitions = append(definitions, definition)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		versions, err := tx.Query(ctx, sopVersionSelect+` WHERE tenant_id=$1 ORDER BY sop_id,version DESC`, tenantID)
		if err != nil {
			return err
		}
		defer versions.Close()
		bySOP := map[string][]domain.SOPVersion{}
		for versions.Next() {
			version, err := scanSOPVersion(versions)
			if err != nil {
				return err
			}
			bySOP[version.SOPID] = append(bySOP[version.SOPID], version)
		}
		if err := versions.Err(); err != nil {
			return err
		}
		for _, definition := range definitions {
			result = append(result, domain.SOPSummary{Definition: definition, Versions: bySOP[definition.ID]})
		}
		return nil
	})
	return result, err
}

func (s *Store) SOP(ctx context.Context, tenantID, id string) (domain.SOPSummary, error) {
	result := domain.SOPSummary{Versions: []domain.SOPVersion{}}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		definition, err := scanSOPDefinition(tx.QueryRow(ctx, sopDefinitionSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		if err != nil {
			return err
		}
		result.Definition = definition
		rows, err := tx.Query(ctx, sopVersionSelect+` WHERE tenant_id=$1 AND sop_id=$2 ORDER BY version DESC`, tenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			version, err := scanSOPVersion(rows)
			if err != nil {
				return err
			}
			result.Versions = append(result.Versions, version)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) SaveSOPVersion(ctx context.Context, value domain.SOPVersion) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE sop_versions SET name=$4,description=$5,content_types=$6,stages=$7,gates=$8,default_execution_mode=$9,digest=$10 WHERE tenant_id=$1 AND sop_id=$2 AND version=$3 AND status='draft'`, value.TenantID, value.SOPID, value.Version, value.Name, value.Description, jsonArrayValue(value.ContentTypes), jsonArrayValue(value.Stages), jsonArrayValue(value.Gates), value.DefaultExecutionMode, value.Digest)
		if err == nil && command.RowsAffected() == 0 {
			return domain.Conflict("SOP_VERSION_IMMUTABLE", "SOP 草稿不存在或版本已发布")
		}
		return dbError(err)
	})
}

func (s *Store) PublishSOPVersion(ctx context.Context, tenantID, sopID string, version int, publishedBy string, publishedAt time.Time) (domain.SOPVersion, error) {
	var result domain.SOPVersion
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanSOPVersion(tx.QueryRow(ctx, sopVersionSelect+` WHERE tenant_id=$1 AND sop_id=$2 AND version=$3 FOR UPDATE`, tenantID, sopID, version))
		if err != nil {
			return err
		}
		if err := value.Validate(); err != nil {
			return err
		}
		digest, err := value.ContentDigest()
		if err != nil {
			return err
		}
		value.Digest = "sha256:" + digest
		value.Status, value.PublishedBy, value.PublishedAt = "published", publishedBy, &publishedAt
		if _, err := tx.Exec(ctx, `UPDATE sop_versions SET digest=$4,status='published',published_by=$5,published_at=$6 WHERE tenant_id=$1 AND sop_id=$2 AND version=$3`, tenantID, sopID, version, value.Digest, publishedBy, publishedAt); err != nil {
			return dbError(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE sop_definitions SET current_version=$3,updated_at=$4 WHERE tenant_id=$1 AND id=$2`, tenantID, sopID, version, publishedAt); err != nil {
			return dbError(err)
		}
		result = value
		return nil
	})
	return result, err
}

func (s *Store) RetireSOPVersion(ctx context.Context, tenantID, sopID string, version int, retiredAt time.Time) error {
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM sop_versions WHERE tenant_id=$1 AND sop_id=$2 AND version=$3 FOR UPDATE`, tenantID, sopID, version).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("SOP 版本")
			}
			return dbError(err)
		}
		if status != "published" {
			return domain.Conflict("SOP_VERSION_STATE_INVALID", "只有已发布 SOP 版本可以退休")
		}
		var current int
		if err := tx.QueryRow(ctx, `SELECT current_version FROM sop_definitions WHERE tenant_id=$1 AND id=$2`, tenantID, sopID).Scan(&current); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("SOP")
			}
			return dbError(err)
		}
		if current == version {
			return domain.Policy("SOP_CURRENT_VERSION_CANNOT_RETIRE", "当前生效版本不能直接退休", "先发布另一个版本或执行回滚")
		}
		command, err := tx.Exec(ctx, `UPDATE sop_versions SET status='retired' WHERE tenant_id=$1 AND sop_id=$2 AND version=$3 AND status='published'`, tenantID, sopID, version)
		if err == nil && command.RowsAffected() == 0 {
			return domain.Conflict("SOP_VERSION_STATE_INVALID", "SOP 版本状态已变化")
		}
		if err != nil {
			return dbError(err)
		}
		_, err = tx.Exec(ctx, `UPDATE sop_definitions SET updated_at=$3 WHERE tenant_id=$1 AND id=$2`, tenantID, sopID, retiredAt)
		return dbError(err)
	})
}

func (s *Store) SaveProjectSOPBinding(ctx context.Context, value domain.ProjectSOPBinding) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO project_sop_bindings(tenant_id,project_id,environment_id,sop_id,sop_version,sop_digest,bound_by,bound_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (tenant_id,project_id) DO UPDATE SET environment_id=EXCLUDED.environment_id,sop_id=EXCLUDED.sop_id,sop_version=EXCLUDED.sop_version,sop_digest=EXCLUDED.sop_digest,bound_by=EXCLUDED.bound_by,bound_at=EXCLUDED.bound_at`, value.TenantID, value.ProjectID, value.EnvironmentID, value.SOPID, value.SOPVersion, value.SOPDigest, value.BoundBy, value.BoundAt)
		return dbError(err)
	})
}

func (s *Store) ProjectSOPBinding(ctx context.Context, tenantID, projectID string) (domain.ProjectSOPBinding, error) {
	var result domain.ProjectSOPBinding
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT tenant_id,project_id,environment_id,sop_id,sop_version,sop_digest,bound_by,bound_at FROM project_sop_bindings WHERE tenant_id=$1 AND project_id=$2`, tenantID, projectID).Scan(&result.TenantID, &result.ProjectID, &result.EnvironmentID, &result.SOPID, &result.SOPVersion, &result.SOPDigest, &result.BoundBy, &result.BoundAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("项目 SOP")
		}
		return dbError(err)
	})
	return result, err
}

func (s *Store) ProjectSOPBindings(ctx context.Context, tenantID string) ([]domain.ProjectSOPBinding, error) {
	result := []domain.ProjectSOPBinding{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT tenant_id,project_id,environment_id,sop_id,sop_version,sop_digest,bound_by,bound_at FROM project_sop_bindings WHERE tenant_id=$1 ORDER BY bound_at,project_id`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value domain.ProjectSOPBinding
			if err := rows.Scan(&value.TenantID, &value.ProjectID, &value.EnvironmentID, &value.SOPID, &value.SOPVersion, &value.SOPDigest, &value.BoundBy, &value.BoundAt); err != nil {
				return dbError(err)
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func scanWorkTask(row pgx.Row) (domain.WorkTask, error) {
	var value domain.WorkTask
	var inputRefs, requestedOutput []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.EnvironmentID, &value.SOPID, &value.SOPVersion, &value.SOPDigest, &value.Title, &value.Intent, &value.ContentType, &inputRefs, &requestedOutput, &value.AssigneeUserID, &value.Priority, &value.DueAt, &value.RiskProfile, &value.IdempotencyKey, &value.Status, &value.CurrentStageID, &value.NextAction, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.InputRefs, err = decodeJSON[[]string](inputRefs)
	}
	if err == nil {
		value.RequestedOutput, err = decodeJSON[map[string]any](requestedOutput)
	}
	return value, dbError(err)
}

func (s *Store) CreateWorkTask(ctx context.Context, value domain.WorkTask) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO work_tasks(tenant_id,id,project_id,environment_id,sop_id,sop_version,sop_digest,title,intent,content_type,input_refs,requested_output,assignee_user_id,priority,due_at,risk_profile,idempotency_key,status,current_stage_id,next_action,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`, value.TenantID, value.ID, value.ProjectID, value.EnvironmentID, value.SOPID, value.SOPVersion, value.SOPDigest, value.Title, value.Intent, value.ContentType, jsonArrayValue(value.InputRefs), jsonValue(value.RequestedOutput), value.AssigneeUserID, value.Priority, value.DueAt, value.RiskProfile, value.IdempotencyKey, value.Status, value.CurrentStageID, value.NextAction, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) WorkTaskByIdempotencyKey(ctx context.Context, tenantID, key string) (domain.WorkTask, error) {
	var result domain.WorkTask
	if key == "" {
		return result, domain.NotFound("任务")
	}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanWorkTask(tx.QueryRow(ctx, workTaskSelect+` WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("任务")
		}
		return err
	})
	return result, err
}

func (s *Store) WorkTasks(ctx context.Context, tenantID, projectID string) ([]domain.WorkTask, error) {
	result := []domain.WorkTask{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := workTaskSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if projectID != "" {
			query += ` AND project_id=$2`
			args = append(args, projectID)
		}
		query += ` ORDER BY updated_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanWorkTask(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func scanConversationImport(row pgx.Row) (domain.ConversationImport, error) {
	var value domain.ConversationImport
	var bundleText string
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.TaskID, &value.StageRunID, &value.NodeID, &value.ClientID, &value.AdapterVersion, &value.AdapterID, &value.Purpose, &value.RequestedScope, &value.AttachAs, &value.RetentionDays, &value.Status, &value.IdempotencyKey, &bundleText, &value.ExpiresAt, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt, &value.CancelledAt, &value.UploadedAt)
	if err == nil && bundleText != "" {
		bundle, decodeErr := decodeJSON[domain.ConversationBundle]([]byte(bundleText))
		err = decodeErr
		if err == nil {
			value.Bundle = &bundle
		}
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateConversationImport(ctx context.Context, value domain.ConversationImport) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO conversation_imports(tenant_id,id,project_id,task_id,stage_run_id,node_id,client_id,adapter_version,adapter_id,purpose,requested_scope,attach_as,retention_days,status,idempotency_key,bundle,expires_at,created_by,created_at,updated_at,cancelled_at,uploaded_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`, value.TenantID, value.ID, value.ProjectID, value.TaskID, nullable(value.StageRunID), value.NodeID, value.ClientID, value.AdapterVersion, value.AdapterID, value.Purpose, value.RequestedScope, value.AttachAs, value.RetentionDays, value.Status, value.IdempotencyKey, nullableJSONBundle(value.Bundle), value.ExpiresAt, value.CreatedBy, value.CreatedAt, value.UpdatedAt, value.CancelledAt, value.UploadedAt)
		return dbError(err)
	})
}

func (s *Store) ConversationImport(ctx context.Context, tenantID, id string) (domain.ConversationImport, error) {
	var result domain.ConversationImport
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanConversationImport(tx.QueryRow(ctx, conversationImportSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("对话导入请求")
		}
		return err
	})
	return result, err
}

func (s *Store) ConversationImportByIdempotencyKey(ctx context.Context, tenantID, key string) (domain.ConversationImport, error) {
	var result domain.ConversationImport
	if key == "" {
		return result, domain.NotFound("对话导入请求")
	}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanConversationImport(tx.QueryRow(ctx, conversationImportSelect+` WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("对话导入请求")
		}
		return err
	})
	return result, err
}

func (s *Store) ConversationImportsForTask(ctx context.Context, tenantID, taskID string) ([]domain.ConversationImport, error) {
	result := []domain.ConversationImport{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, conversationImportSelect+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY created_at`, tenantID, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, scanErr := scanConversationImport(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) SaveConversationImport(ctx context.Context, value domain.ConversationImport) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE conversation_imports SET status=$3,bundle=$4,updated_at=$5,cancelled_at=$6,uploaded_at=$7 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.Status, nullableJSONBundle(value.Bundle), value.UpdatedAt, value.CancelledAt, value.UploadedAt)
		if err == nil && command.RowsAffected() == 0 {
			return domain.NotFound("对话导入请求")
		}
		return dbError(err)
	})
}

func nullableJSONBundle(value *domain.ConversationBundle) any {
	if value == nil {
		return nil
	}
	return jsonValue(value)
}

func (s *Store) WorkTask(ctx context.Context, tenantID, id string) (domain.WorkTask, error) {
	var result domain.WorkTask
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanWorkTask(tx.QueryRow(ctx, workTaskSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		return err
	})
	return result, err
}

func (s *Store) SaveWorkTask(ctx context.Context, value domain.WorkTask) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE work_tasks SET input_refs=$3,requested_output=$4,assignee_user_id=$5,priority=$6,due_at=$7,risk_profile=$8,status=$9,current_stage_id=$10,next_action=$11,updated_at=$12 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, jsonArrayValue(value.InputRefs), jsonValue(value.RequestedOutput), value.AssigneeUserID, value.Priority, value.DueAt, value.RiskProfile, value.Status, value.CurrentStageID, value.NextAction, value.UpdatedAt)
		if err == nil && command.RowsAffected() == 0 {
			return domain.NotFound("任务")
		}
		return dbError(err)
	})
}

func scanStageRun(row pgx.Row) (domain.StageRun, error) {
	var value domain.StageRun
	var inputs, outputs []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.TaskID, &value.StageID, &value.Status, &value.ExecutionMode, &inputs, &outputs, &value.StartedAt, &value.CompletedAt, &value.UpdatedAt)
	if err == nil {
		value.InputRefs, err = decodeJSON[[]string](inputs)
	}
	if err == nil {
		value.OutputRefs, err = decodeJSON[[]string](outputs)
	}
	return value, dbError(err)
}

func (s *Store) StageRuns(ctx context.Context, tenantID, taskID string) ([]domain.StageRun, error) {
	result := []domain.StageRun{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, stageRunSelect+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY updated_at`, tenantID, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanStageRun(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		outputs, err := taskStageOutputsTx(ctx, tx, tenantID, taskID)
		if err != nil {
			return err
		}
		byRun := map[string][]domain.TaskStageOutput{}
		for _, output := range outputs {
			byRun[output.StageRunID] = append(byRun[output.StageRunID], output)
		}
		for index := range result {
			result[index].Outputs = byRun[result[index].ID]
			if result[index].Outputs == nil {
				result[index].Outputs = []domain.TaskStageOutput{}
			}
		}
		return nil
	})
	return result, err
}

func (s *Store) CreateStageRun(ctx context.Context, value domain.StageRun) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO stage_runs(tenant_id,id,task_id,stage_id,status,execution_mode,input_refs,output_refs,started_at,completed_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, value.TenantID, value.ID, value.TaskID, value.StageID, value.Status, value.ExecutionMode, jsonArrayValue(value.InputRefs), jsonArrayValue(value.OutputRefs), value.StartedAt, value.CompletedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) SaveStageRun(ctx context.Context, value domain.StageRun) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE stage_runs SET status=$3,execution_mode=$4,input_refs=$5,output_refs=$6,started_at=$7,completed_at=$8,updated_at=$9 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.Status, value.ExecutionMode, jsonArrayValue(value.InputRefs), jsonArrayValue(value.OutputRefs), value.StartedAt, value.CompletedAt, value.UpdatedAt)
		if err == nil && command.RowsAffected() == 0 {
			return domain.NotFound("StageRun")
		}
		return dbError(err)
	})
}
