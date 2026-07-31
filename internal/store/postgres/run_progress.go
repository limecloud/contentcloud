package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func scanRunProgress(row pgx.Row) (domain.RunProgressEvent, error) {
	var event domain.RunProgressEvent
	err := row.Scan(&event.Cursor, &event.TenantID, &event.ProjectID, &event.RunID, &event.AttemptID, &event.DeviceID, &event.Sequence, &event.Phase, &event.Step, &event.Label, &event.OccurredAt)
	return event, err
}

const runProgressSelect = `SELECT cursor,tenant_id,project_id,run_id,attempt_id,device_id,sequence,phase,step,label,occurred_at FROM run_progress_events`

func (s *Store) AppendRunProgress(ctx context.Context, event domain.RunProgressEvent) (domain.RunProgressEvent, error) {
	var stored domain.RunProgressEvent
	err := s.withTenant(ctx, event.TenantID, func(tx pgx.Tx) error {
		value, err := scanRunProgress(tx.QueryRow(ctx, `INSERT INTO run_progress_events(tenant_id,project_id,run_id,attempt_id,device_id,sequence,phase,step,label,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(attempt_id,sequence) DO UPDATE SET attempt_id=run_progress_events.attempt_id RETURNING cursor,tenant_id,project_id,run_id,attempt_id,device_id,sequence,phase,step,label,occurred_at`, event.TenantID, event.ProjectID, event.RunID, event.AttemptID, event.DeviceID, event.Sequence, event.Phase, event.Step, event.Label, event.OccurredAt))
		stored = value
		return err
	})
	return stored, err
}

func (s *Store) RunProgress(ctx context.Context, tenantID, runID string, after int64) ([]domain.RunProgressEvent, error) {
	result := []domain.RunProgressEvent{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runProgressSelect+` WHERE tenant_id=$1 AND run_id=$2 AND cursor>$3 ORDER BY cursor LIMIT 500`, tenantID, runID, after)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			event, scanErr := scanRunProgress(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, event)
		}
		return rows.Err()
	})
	return result, err
}
