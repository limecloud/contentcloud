package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const runtimeResourceQuotaSelect = `SELECT tenant_id,resource_key,capacity,unit,version,updated_at FROM runtime_resource_quotas`
const runtimeReservationSelect = `SELECT tenant_id,id,job_run_id,node_run_id,attempt_id,resource_key,quantity,unit,state,fence_token,idempotency_key,expires_at,released_at,created_at,updated_at FROM runtime_resource_reservations`

func scanResourceQuota(row pgx.Row) (domain.ResourceQuota, error) {
	var value domain.ResourceQuota
	err := row.Scan(&value.TenantID, &value.ResourceKey, &value.Capacity, &value.Unit, &value.Version, &value.UpdatedAt)
	return value, dbError(err)
}

func scanResourceReservation(row pgx.Row) (domain.ResourceReservation, error) {
	var value domain.ResourceReservation
	err := row.Scan(&value.TenantID, &value.ID, &value.JobRunID, &value.NodeRunID, &value.AttemptID, &value.ResourceKey, &value.Quantity, &value.Unit, &value.State, &value.FenceToken, &value.IdempotencyKey, &value.ExpiresAt, &value.ReleasedAt, &value.CreatedAt, &value.UpdatedAt)
	return value, dbError(err)
}

func (s *Store) SaveResourceQuota(ctx context.Context, quota domain.ResourceQuota) error {
	if err := quota.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, quota.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `INSERT INTO runtime_resource_quotas(tenant_id,resource_key,capacity,unit,version,updated_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (tenant_id,resource_key) DO UPDATE SET capacity=EXCLUDED.capacity,unit=EXCLUDED.unit,version=EXCLUDED.version,updated_at=EXCLUDED.updated_at WHERE runtime_resource_quotas.version=$7`, quota.TenantID, quota.ResourceKey, quota.Capacity, quota.Unit, quota.Version, quota.UpdatedAt, quota.Version-1)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return domain.Conflict("RESOURCE_QUOTA_VERSION_CONFLICT", "资源配额已被更新")
		}
		return nil
	})
}

func (s *Store) ResourceQuotas(ctx context.Context, tenantID string) ([]domain.ResourceQuota, error) {
	result := []domain.ResourceQuota{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, runtimeResourceQuotaSelect+` WHERE tenant_id=$1 ORDER BY resource_key`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanResourceQuota(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) ResourceReservations(ctx context.Context, tenantID, jobID string) ([]domain.ResourceReservation, error) {
	result := []domain.ResourceReservation{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeReservationSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if jobID != "" {
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
			value, err := scanResourceReservation(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func reserveResourcesTx(ctx context.Context, tx pgx.Tx, reservations []domain.ResourceReservation) error {
	if len(reservations) == 0 {
		return nil
	}
	type requestTotal struct {
		TenantID, ResourceKey, Unit string
		Quantity                    int64
	}
	totals := map[string]requestTotal{}
	for _, reservation := range reservations {
		if err := reservation.Validate(); err != nil {
			return err
		}
		key := reservation.TenantID + ":" + reservation.ResourceKey
		total := totals[key]
		total.TenantID, total.ResourceKey, total.Unit = reservation.TenantID, reservation.ResourceKey, reservation.Unit
		total.Quantity += reservation.Quantity
		totals[key] = total
	}
	for _, total := range totals {
		var capacity int64
		var unit string
		err := tx.QueryRow(ctx, `SELECT capacity,unit FROM runtime_resource_quotas WHERE tenant_id=$1 AND resource_key=$2 FOR UPDATE`, total.TenantID, total.ResourceKey).Scan(&capacity, &unit)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return dbError(err)
		}
		if unit != total.Unit {
			return domain.Invalid("RESOURCE_UNIT_MISMATCH", "资源预留单位与配额不一致")
		}
		var used int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity),0) FROM runtime_resource_reservations WHERE tenant_id=$1 AND resource_key=$2 AND state='held'`, total.TenantID, total.ResourceKey).Scan(&used); err != nil {
			return err
		}
		if used+total.Quantity > capacity {
			return domain.Conflict("RESOURCE_QUOTA_EXCEEDED", "资源配额不足，拒绝超卖")
		}
	}
	for _, reservation := range reservations {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_resource_reservations(tenant_id,id,job_run_id,node_run_id,attempt_id,resource_key,quantity,unit,state,fence_token,idempotency_key,expires_at,released_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, reservation.TenantID, reservation.ID, reservation.JobRunID, reservation.NodeRunID, reservation.AttemptID, reservation.ResourceKey, reservation.Quantity, reservation.Unit, reservation.State, reservation.FenceToken, reservation.IdempotencyKey, reservation.ExpiresAt, reservation.ReleasedAt, reservation.CreatedAt, reservation.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
	}
	return nil
}
