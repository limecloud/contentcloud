package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const runtimeProjectionRebuildSelect = `SELECT tenant_id,id,job_run_id,mode,status,event_count,last_sequence,external_calls,integrity_status,error_code,started_at,finished_at,version FROM runtime_projection_rebuild_runs`

func scanRuntimeProjectionRebuild(row pgx.Row) (domain.RuntimeProjectionRebuildRun, error) {
	var value domain.RuntimeProjectionRebuildRun
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.Mode, &value.Status, &value.EventCount, &value.LastSequence, &value.ExternalCalls, &value.IntegrityStatus, &value.ErrorCode, &value.StartedAt, &value.FinishedAt, &value.Version)
	return value, err
}

func (s *Store) CreateRuntimeProjectionRebuild(ctx context.Context, run domain.RuntimeProjectionRebuildRun) error {
	if err := run.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, run.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_projection_rebuild_runs(tenant_id,id,job_run_id,mode,status,event_count,last_sequence,external_calls,integrity_status,error_code,started_at,finished_at,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, run.TenantID, run.ID, run.JobRunID, run.Mode, run.Status, run.EventCount, run.LastSequence, run.ExternalCalls, run.IntegrityStatus, run.ErrorCode, run.StartedAt, run.FinishedAt, run.Version)
		return dbError(err)
	})
}

func (s *Store) UpdateRuntimeProjectionRebuild(ctx context.Context, run domain.RuntimeProjectionRebuildRun, expectedVersion int) error {
	if err := run.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, run.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE runtime_projection_rebuild_runs SET status=$3,event_count=$4,last_sequence=$5,external_calls=$6,integrity_status=$7,error_code=$8,finished_at=$9,version=$10 WHERE tenant_id=$1 AND id=$2 AND version=$11`, run.TenantID, run.ID, run.Status, run.EventCount, run.LastSequence, run.ExternalCalls, run.IntegrityStatus, run.ErrorCode, run.FinishedAt, run.Version, expectedVersion)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return domain.Conflict("RUNTIME_PROJECTION_REBUILD_VERSION_CONFLICT", "投影重建运行事实已被更新")
		}
		return nil
	})
}

func (s *Store) RuntimeProjectionRebuilds(ctx context.Context, tenantID, jobID string) ([]domain.RuntimeProjectionRebuildRun, error) {
	result := []domain.RuntimeProjectionRebuildRun{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeProjectionRebuildSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if jobID != "" {
			query += ` AND job_run_id=$2`
			args = append(args, jobID)
		}
		query += ` ORDER BY started_at DESC,id`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeProjectionRebuild(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return result, domain.NotFound("投影重建运行事实")
	}
	return result, err
}
