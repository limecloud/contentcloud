package postgres

import (
	"context"
	"errors"
	"strings"

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
		if err := tx.QueryRow(ctx, `SELECT count(*),min(created_at) FROM runtime_outbox_receipts WHERE tenant_id=$1 AND subscriber=$2 AND delivered_at IS NULL`, tenantID, domain.RuntimeOutboxSubscriberProjection).Scan(&result.Pending, &result.OldestPending); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT max(projected_at) FROM runtime_projection_snapshots WHERE tenant_id=$1`, tenantID).Scan(&result.LastProjectedAt)
	})
	return result, err
}

func (s *Store) RuntimeOutboxStats(ctx context.Context, tenantID, subscriber string) (domain.RuntimeOutboxStats, error) {
	subscriber = strings.TrimSpace(subscriber)
	if subscriber == "" {
		return domain.RuntimeOutboxStats{}, domain.Invalid("OUTBOX_SUBSCRIBER_REQUIRED", "outbox 统计需要订阅者")
	}
	result := domain.RuntimeOutboxStats{TenantID: tenantID, Subscriber: subscriber}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*),min(m.created_at) FROM runtime_outbox_receipts r JOIN runtime_outbox m ON m.tenant_id=r.tenant_id AND m.id=r.message_id WHERE r.tenant_id=$1 AND r.subscriber=$2 AND r.delivered_at IS NULL`, tenantID, subscriber).Scan(&result.Pending, &result.OldestPending)
	})
	return result, err
}

func (s *Store) SaveRuntimeMaintenanceHeartbeat(ctx context.Context, value domain.RuntimeMaintenanceHeartbeat) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_maintenance_heartbeats(tenant_id,kind,worker_id,state,last_started_at,last_success_at,last_error_code,version,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,1,$8) ON CONFLICT (tenant_id,kind) DO UPDATE SET worker_id=EXCLUDED.worker_id,state=EXCLUDED.state,last_started_at=EXCLUDED.last_started_at,last_success_at=COALESCE(EXCLUDED.last_success_at,runtime_maintenance_heartbeats.last_success_at),last_error_code=EXCLUDED.last_error_code,version=runtime_maintenance_heartbeats.version+1,updated_at=EXCLUDED.updated_at WHERE runtime_maintenance_heartbeats.updated_at<=EXCLUDED.updated_at`, value.TenantID, value.Kind, value.WorkerID, value.State, value.LastStartedAt, value.LastSuccessAt, value.LastErrorCode, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) RuntimeMaintenanceHeartbeat(ctx context.Context, tenantID, kind string) (domain.RuntimeMaintenanceHeartbeat, error) {
	var result domain.RuntimeMaintenanceHeartbeat
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT tenant_id,kind,worker_id,state,last_started_at,last_success_at,last_error_code,version,updated_at FROM runtime_maintenance_heartbeats WHERE tenant_id=$1 AND kind=$2`, tenantID, kind).Scan(&result.TenantID, &result.Kind, &result.WorkerID, &result.State, &result.LastStartedAt, &result.LastSuccessAt, &result.LastErrorCode, &result.Version, &result.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Runtime 运维心跳")
		}
		return err
	})
	return result, err
}
