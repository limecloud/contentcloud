package postgres

import (
	"context"
	"errors"
	"strconv"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/jackc/pgx/v5"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

const runtimeCleanupDiagnosticSelect = `SELECT id,tenant_id,project_id,task_id,request_id,manifest_digest,object_key,cause_code,cause_summary,cleanup_error,status,attempt_count,next_retry_at,created_at,updated_at,version FROM runtime_cleanup_diagnostics`

func scanRuntimeCleanupDiagnostic(row pgx.Row) (contentruntime.RuntimeCleanupDiagnostic, error) {
	var value contentruntime.RuntimeCleanupDiagnostic
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.TaskID, &value.RequestID, &value.ManifestDigest, &value.ObjectKey, &value.CauseCode, &value.CauseSummary, &value.CleanupError, &value.Status, &value.AttemptCount, &value.NextRetryAt, &value.CreatedAt, &value.UpdatedAt, &value.Version)
	return value, err
}

func (s *Store) CreateRuntimeCleanupDiagnostic(ctx context.Context, value contentruntime.RuntimeCleanupDiagnostic) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_cleanup_diagnostics(id,tenant_id,project_id,task_id,request_id,manifest_digest,object_key,cause_code,cause_summary,cleanup_error,status,attempt_count,next_retry_at,created_at,updated_at,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, value.ID, value.TenantID, value.ProjectID, value.TaskID, value.RequestID, value.ManifestDigest, value.ObjectKey, value.CauseCode, value.CauseSummary, value.CleanupError, value.Status, value.AttemptCount, value.NextRetryAt, value.CreatedAt, value.UpdatedAt, value.Version)
		return dbError(err)
	})
}

func (s *Store) UpdateRuntimeCleanupDiagnostic(ctx context.Context, value contentruntime.RuntimeCleanupDiagnostic, expectedVersion int) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		current, err := scanRuntimeCleanupDiagnostic(tx.QueryRow(ctx, runtimeCleanupDiagnosticSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, value.TenantID, value.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("Blob 清理诊断")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || value.Version != expectedVersion+1 {
			return fault.Conflict("RUNTIME_CLEANUP_DIAGNOSTIC_VERSION_CONFLICT", "Blob 清理诊断已被更新")
		}
		if err := value.ValidateTransition(current); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE runtime_cleanup_diagnostics SET cleanup_error=$3,status=$4,attempt_count=$5,next_retry_at=$6,updated_at=$7,version=$8 WHERE tenant_id=$1 AND id=$2 AND version=$9`, value.TenantID, value.ID, value.CleanupError, value.Status, value.AttemptCount, value.NextRetryAt, value.UpdatedAt, value.Version, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return fault.Conflict("RUNTIME_CLEANUP_DIAGNOSTIC_VERSION_CONFLICT", "Blob 清理诊断已被更新")
		}
		return nil
	})
}

func (s *Store) RuntimeCleanupDiagnostic(ctx context.Context, tenantID, id string) (contentruntime.RuntimeCleanupDiagnostic, error) {
	var value contentruntime.RuntimeCleanupDiagnostic
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var scanErr error
		value, scanErr = scanRuntimeCleanupDiagnostic(tx.QueryRow(ctx, runtimeCleanupDiagnosticSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return fault.NotFound("Blob 清理诊断")
		}
		return scanErr
	})
	return value, err
}

func (s *Store) RuntimeCleanupDiagnostics(ctx context.Context, tenantID, status string, limit int) ([]contentruntime.RuntimeCleanupDiagnostic, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	result := make([]contentruntime.RuntimeCleanupDiagnostic, 0)
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeCleanupDiagnosticSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if status != "" {
			query += ` AND status=$2`
			args = append(args, status)
		}
		query += ` ORDER BY updated_at ASC,id LIMIT $` + strconv.Itoa(len(args)+1)
		args = append(args, limit)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value contentruntime.RuntimeCleanupDiagnostic
			if err := rows.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.TaskID, &value.RequestID, &value.ManifestDigest, &value.ObjectKey, &value.CauseCode, &value.CauseSummary, &value.CleanupError, &value.Status, &value.AttemptCount, &value.NextRetryAt, &value.CreatedAt, &value.UpdatedAt, &value.Version); err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}
