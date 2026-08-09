package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) SaveRuntimeExplorer(ctx context.Context, view domain.RuntimeExplorerView) error {
	if view.TenantID == "" || view.JobRunID == "" || view.ProjectedAt.IsZero() {
		return domain.Invalid("RUNTIME_PROJECTION_INVALID", "Runtime Explorer 投影缺少执行实例或时间")
	}
	return s.withTenant(ctx, view.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `INSERT INTO runtime_projection_snapshots(tenant_id,job_run_id,job,nodes,last_event_sequence,source_event_id,projected_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (tenant_id,job_run_id) DO UPDATE SET job=EXCLUDED.job,nodes=EXCLUDED.nodes,last_event_sequence=EXCLUDED.last_event_sequence,source_event_id=EXCLUDED.source_event_id,projected_at=EXCLUDED.projected_at WHERE runtime_projection_snapshots.last_event_sequence <= EXCLUDED.last_event_sequence`, view.TenantID, view.JobRunID, jsonValue(view.Job), jsonArrayValue(view.Nodes), view.LastEventSeq, view.SourceEventID, view.ProjectedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() != 1 {
			return domain.Conflict("RUNTIME_PROJECTION_STALE", "投影事件序号不能倒退")
		}
		return nil
	})
}

func (s *Store) RuntimeExplorer(ctx context.Context, tenantID, jobID string) (domain.RuntimeExplorerView, error) {
	var result domain.RuntimeExplorerView
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var jobBody, nodesBody []byte
		err := tx.QueryRow(ctx, `SELECT tenant_id,job_run_id,job,nodes,last_event_sequence,source_event_id,projected_at FROM runtime_projection_snapshots WHERE tenant_id=$1 AND job_run_id=$2`, tenantID, jobID).Scan(&result.TenantID, &result.JobRunID, &jobBody, &nodesBody, &result.LastEventSeq, &result.SourceEventID, &result.ProjectedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Runtime Explorer 投影")
		}
		if err != nil {
			return err
		}
		result.Job, err = decodeJSON[domain.JobRun](jobBody)
		if err == nil {
			result.Nodes, err = decodeJSON[[]domain.NodeRun](nodesBody)
		}
		return dbError(err)
	})
	return result, err
}

func (s *Store) RuntimeProjectionStats(ctx context.Context, tenantID string) (domain.RuntimeProjectionStats, error) {
	result := domain.RuntimeProjectionStats{TenantID: tenantID}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*),min(created_at) FROM runtime_outbox WHERE tenant_id=$1 AND delivered_at IS NULL`, tenantID).Scan(&result.Pending, &result.OldestPending); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT max(projected_at) FROM runtime_projection_snapshots WHERE tenant_id=$1`, tenantID).Scan(&result.LastProjectedAt)
	})
	return result, err
}
