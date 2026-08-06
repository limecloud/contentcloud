package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) WorkTaskRuns(ctx context.Context, tenantID, taskID string) ([]domain.TaskRun, error) {
	result := []domain.TaskRun{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runSelect+` WHERE tenant_id=$1 AND work_task_id=$2 ORDER BY created_at`, tenantID, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, scanErr := scanRun(rows)
			if scanErr != nil {
				return scanErr
			}
			value.RunTokenHash = ""
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

const gateEvaluationSelect = `SELECT tenant_id,id,project_id,task_id,stage_run_id,gate_id,gate_mode,status,revision_id,input_refs,checks,decision,reason,decided_by,decided_at,expires_at,created_at,updated_at FROM task_gate_evaluations`

func scanGateEvaluation(row pgx.Row) (domain.GateEvaluation, error) {
	var value domain.GateEvaluation
	var inputs, checks []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.TaskID, &value.StageRunID, &value.GateID, &value.GateMode, &value.Status, &value.RevisionID, &inputs, &checks, &value.Decision, &value.Reason, &value.DecidedBy, &value.DecidedAt, &value.ExpiresAt, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.InputRefs, err = decodeJSON[[]string](inputs)
	}
	if err == nil {
		value.Checks, err = decodeJSON[map[string]any](checks)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateGateEvaluation(ctx context.Context, value domain.GateEvaluation) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO task_gate_evaluations(tenant_id,id,project_id,task_id,stage_run_id,gate_id,gate_mode,status,revision_id,input_refs,checks,decision,reason,decided_by,decided_at,expires_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, value.TenantID, value.ID, value.ProjectID, value.TaskID, value.StageRunID, value.GateID, value.GateMode, value.Status, value.RevisionID, jsonArrayValue(value.InputRefs), jsonValue(value.Checks), value.Decision, value.Reason, value.DecidedBy, value.DecidedAt, value.ExpiresAt, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) GateEvaluations(ctx context.Context, tenantID, taskID string) ([]domain.GateEvaluation, error) {
	result := []domain.GateEvaluation{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, gateEvaluationSelect+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY created_at`, tenantID, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, scanErr := scanGateEvaluation(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) GateEvaluation(ctx context.Context, tenantID, id string) (domain.GateEvaluation, error) {
	var result domain.GateEvaluation
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanGateEvaluation(tx.QueryRow(ctx, gateEvaluationSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("审核门评估")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveGateEvaluation(ctx context.Context, value domain.GateEvaluation) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE task_gate_evaluations SET status=$3,revision_id=$4,input_refs=$5,checks=$6,decision=$7,reason=$8,decided_by=$9,decided_at=$10,expires_at=$11,updated_at=$12 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.Status, value.RevisionID, jsonArrayValue(value.InputRefs), jsonValue(value.Checks), value.Decision, value.Reason, value.DecidedBy, value.DecidedAt, value.ExpiresAt, value.UpdatedAt)
		if err == nil && command.RowsAffected() == 0 {
			return domain.NotFound("审核门评估")
		}
		return dbError(err)
	})
}

const taskRevisionSelect = `SELECT tenant_id,id,project_id,task_id,revision_no,content_type,schema_version,content,content_hash,sop_digest,knowledge_snapshot_ids,evidence_summary,rights_summary,status,submitted_by,submitted_at,created_at FROM task_revisions`

func scanTaskRevision(row pgx.Row) (domain.TaskRevision, error) {
	var value domain.TaskRevision
	var content, snapshots, evidence, rights []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.TaskID, &value.RevisionNo, &value.ContentType, &value.SchemaVersion, &content, &value.ContentHash, &value.SOPDigest, &snapshots, &evidence, &rights, &value.Status, &value.SubmittedBy, &value.SubmittedAt, &value.CreatedAt)
	if err == nil {
		value.Content = append([]byte{}, content...)
		value.KnowledgeSnapshotIDs, err = decodeJSON[[]string](snapshots)
	}
	if err == nil {
		value.EvidenceSummary, err = decodeJSON[map[string]any](evidence)
	}
	if err == nil {
		value.RightsSummary, err = decodeJSON[map[string]any](rights)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateTaskRevision(ctx context.Context, value domain.TaskRevision) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO task_revisions(tenant_id,id,project_id,task_id,revision_no,content_type,schema_version,content,content_hash,sop_digest,knowledge_snapshot_ids,evidence_summary,rights_summary,status,submitted_by,submitted_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, value.TenantID, value.ID, value.ProjectID, value.TaskID, value.RevisionNo, value.ContentType, value.SchemaVersion, value.Content, value.ContentHash, value.SOPDigest, jsonArrayValue(value.KnowledgeSnapshotIDs), jsonValue(value.EvidenceSummary), jsonValue(value.RightsSummary), value.Status, value.SubmittedBy, value.SubmittedAt, value.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) TaskRevisions(ctx context.Context, tenantID, taskID string) ([]domain.TaskRevision, error) {
	result := []domain.TaskRevision{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, taskRevisionSelect+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY revision_no`, tenantID, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, scanErr := scanTaskRevision(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) TaskRevision(ctx context.Context, tenantID, id string) (domain.TaskRevision, error) {
	var result domain.TaskRevision
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanTaskRevision(tx.QueryRow(ctx, taskRevisionSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("任务修订版本")
		}
		return err
	})
	return result, err
}

const taskDeliverySelect = `SELECT tenant_id,id,project_id,task_id,revision_id,destination,status,manifest,COALESCE(delivery_package_id::text,''),integrity_status,delivery_digest,delivered_by,delivered_at,error_code,created_at,updated_at FROM task_deliveries`

func scanTaskDelivery(row pgx.Row) (domain.TaskDelivery, error) {
	var value domain.TaskDelivery
	var manifest []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.TaskID, &value.RevisionID, &value.Destination, &value.Status, &manifest, &value.DeliveryPackageID, &value.IntegrityStatus, &value.DeliveryDigest, &value.DeliveredBy, &value.DeliveredAt, &value.ErrorCode, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.Manifest, err = decodeJSON[[]string](manifest)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateTaskDelivery(ctx context.Context, value domain.TaskDelivery) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO task_deliveries(tenant_id,id,project_id,task_id,revision_id,destination,status,manifest,delivery_package_id,integrity_status,delivery_digest,delivered_by,delivered_at,error_code,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, value.TenantID, value.ID, value.ProjectID, value.TaskID, value.RevisionID, value.Destination, value.Status, jsonArrayValue(value.Manifest), nullable(value.DeliveryPackageID), value.IntegrityStatus, value.DeliveryDigest, value.DeliveredBy, value.DeliveredAt, value.ErrorCode, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) TaskDeliveries(ctx context.Context, tenantID, taskID string) ([]domain.TaskDelivery, error) {
	result := []domain.TaskDelivery{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, taskDeliverySelect+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY created_at`, tenantID, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, scanErr := scanTaskDelivery(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) TaskDelivery(ctx context.Context, tenantID, id string) (domain.TaskDelivery, error) {
	var result domain.TaskDelivery
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanTaskDelivery(tx.QueryRow(ctx, taskDeliverySelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("任务交付")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveTaskDelivery(ctx context.Context, value domain.TaskDelivery) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE task_deliveries SET status=$3,manifest=$4,delivery_package_id=$5,integrity_status=$6,delivery_digest=$7,delivered_by=$8,delivered_at=$9,error_code=$10,updated_at=$11 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.Status, jsonArrayValue(value.Manifest), nullable(value.DeliveryPackageID), value.IntegrityStatus, value.DeliveryDigest, value.DeliveredBy, value.DeliveredAt, value.ErrorCode, value.UpdatedAt)
		if err == nil && command.RowsAffected() == 0 {
			return domain.NotFound("任务交付")
		}
		return dbError(err)
	})
}
