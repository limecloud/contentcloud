package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateRunAttempt(ctx context.Context, attempt domain.RunAttempt) error {
	return s.withTenant(ctx, attempt.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO run_attempts(id,tenant_id,project_id,run_id,device_id,state,capability_id,capability_version,capability_digest,input_schema,output_schema,token_hash,lease_expires_at,heartbeat_at,started_at,finished_at,exit_code,failure_class,usage,transcript_summary,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`, attempt.ID, attempt.TenantID, attempt.ProjectID, attempt.RunID, attempt.DeviceID, attempt.State, attempt.CapabilityID, attempt.CapabilityVersion, attempt.CapabilityDigest, attempt.InputSchema, attempt.OutputSchema, attempt.TokenHash, attempt.LeaseExpiresAt, attempt.HeartbeatAt, attempt.StartedAt, attempt.FinishedAt, attempt.ExitCode, attempt.FailureClass, jsonValue(attempt.Usage), attempt.TranscriptSummary, attempt.CreatedAt)
		return dbError(err)
	})
}

func scanRunAttempt(row pgx.Row) (domain.RunAttempt, error) {
	var attempt domain.RunAttempt
	var usage []byte
	err := row.Scan(&attempt.ID, &attempt.TenantID, &attempt.ProjectID, &attempt.RunID, &attempt.DeviceID, &attempt.State, &attempt.CapabilityID, &attempt.CapabilityVersion, &attempt.CapabilityDigest, &attempt.InputSchema, &attempt.OutputSchema, &attempt.TokenHash, &attempt.LeaseExpiresAt, &attempt.HeartbeatAt, &attempt.StartedAt, &attempt.FinishedAt, &attempt.ExitCode, &attempt.FailureClass, &usage, &attempt.TranscriptSummary, &attempt.CreatedAt)
	if err == nil {
		attempt.Usage, err = decodeJSON[map[string]any](usage)
	}
	return attempt, err
}

const runAttemptSelect = `SELECT id,tenant_id,project_id,run_id,device_id,state,capability_id,capability_version,capability_digest,input_schema,output_schema,token_hash,lease_expires_at,heartbeat_at,started_at,finished_at,exit_code,failure_class,usage,transcript_summary,created_at FROM run_attempts`

func (s *Store) RunAttempt(ctx context.Context, tenantID, id string) (domain.RunAttempt, error) {
	var result domain.RunAttempt
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		attempt, err := scanRunAttempt(tx.QueryRow(ctx, runAttemptSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = attempt
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("任务执行尝试")
		}
		return err
	})
	return result, err
}

func (s *Store) RunAttempts(ctx context.Context, tenantID, runID string) ([]domain.RunAttempt, error) {
	result := []domain.RunAttempt{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runAttemptSelect+` WHERE tenant_id=$1 AND run_id=$2 ORDER BY created_at`, tenantID, runID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			attempt, err := scanRunAttempt(rows)
			if err != nil {
				return err
			}
			attempt.TokenHash = ""
			result = append(result, attempt)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) SaveRunAttempt(ctx context.Context, attempt domain.RunAttempt) error {
	return s.withTenant(ctx, attempt.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE run_attempts SET state=$3,token_hash=$4,lease_expires_at=$5,heartbeat_at=$6,started_at=$7,finished_at=$8,exit_code=$9,failure_class=$10,usage=$11,transcript_summary=$12 WHERE tenant_id=$1 AND id=$2`, attempt.TenantID, attempt.ID, attempt.State, attempt.TokenHash, attempt.LeaseExpiresAt, attempt.HeartbeatAt, attempt.StartedAt, attempt.FinishedAt, attempt.ExitCode, attempt.FailureClass, jsonValue(attempt.Usage), attempt.TranscriptSummary)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("任务执行尝试")
		}
		return nil
	})
}

func (s *Store) ExpireRunAttempts(ctx context.Context, tenantID string, now time.Time) error {
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `WITH expired AS (
  UPDATE run_attempts SET state='expired',failure_class='lease_expired',finished_at=$2
  WHERE tenant_id=$1 AND state IN ('leased','running') AND lease_expires_at<$2
  RETURNING id,run_id
)
UPDATE task_runs r SET
  state=CASE WHEN r.attempt_count>=3 THEN 'failed' ELSE 'queued' END,
  error_code=CASE WHEN r.attempt_count>=3 THEN 'RUN_ATTEMPTS_EXHAUSTED' ELSE '' END,
  active_attempt_id=NULL,lease_device_id=NULL,lease_expires_at=NULL,run_token_hash='',updated_at=$2
FROM expired e WHERE r.tenant_id=$1 AND r.id=e.run_id AND r.active_attempt_id=e.id`, tenantID, now)
		return err
	})
}
