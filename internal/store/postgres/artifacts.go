package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const artifactOpenRequestSelect = `SELECT id,tenant_id,project_id,artifact_id,device_id,requested_by,state,reason,expires_at,accepted_at,completed_at,created_at FROM artifact_open_requests`

func scanArtifactOpenRequest(row pgx.Row) (domain.ArtifactOpenRequest, error) {
	var value domain.ArtifactOpenRequest
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.ArtifactID, &value.DeviceID, &value.RequestedBy, &value.State, &value.Reason, &value.ExpiresAt, &value.AcceptedAt, &value.CompletedAt, &value.CreatedAt)
	return value, err
}

func (s *Store) CreateArtifactOpenRequest(ctx context.Context, value domain.ArtifactOpenRequest) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO artifact_open_requests(id,tenant_id,project_id,artifact_id,device_id,requested_by,state,reason,expires_at,accepted_at,completed_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, value.ID, value.TenantID, value.ProjectID, value.ArtifactID, value.DeviceID, value.RequestedBy, value.State, value.Reason, value.ExpiresAt, value.AcceptedAt, value.CompletedAt, value.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) ArtifactOpenRequest(ctx context.Context, tenantID, id string) (domain.ArtifactOpenRequest, error) {
	var result domain.ArtifactOpenRequest
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanArtifactOpenRequest(tx.QueryRow(ctx, artifactOpenRequestSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Artifact 打开请求")
		}
		return err
	})
	return result, err
}

func (s *Store) PendingArtifactOpenRequests(ctx context.Context, tenantID, deviceID string, now time.Time, limit int) ([]domain.ArtifactOpenRequest, error) {
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	values := []domain.ArtifactOpenRequest{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, artifactOpenRequestSelect+` WHERE tenant_id=$1 AND device_id=$2 AND state='pending' AND expires_at>$3 ORDER BY created_at LIMIT $4`, tenantID, deviceID, now, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, scanErr := scanArtifactOpenRequest(rows)
			if scanErr != nil {
				return scanErr
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}

func (s *Store) SaveArtifactOpenRequest(ctx context.Context, value domain.ArtifactOpenRequest) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE artifact_open_requests SET state=$3,reason=$4,accepted_at=$5,completed_at=$6 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.State, value.Reason, value.AcceptedAt, value.CompletedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("Artifact 打开请求")
		}
		return nil
	})
}

func (s *Store) ExpireArtifactOpenRequests(ctx context.Context, tenantID string, now time.Time) error {
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE artifact_open_requests SET state='expired',completed_at=$2 WHERE tenant_id=$1 AND state IN ('pending','accepted') AND expires_at<=$2`, tenantID, now)
		return dbError(err)
	})
}
