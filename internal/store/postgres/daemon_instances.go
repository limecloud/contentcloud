package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/limecloud/contentcloud/internal/domain"
)

const daemonInstanceSelect = `SELECT id,tenant_id,device_id,connection_epoch,report_sequence,COALESCE(pid,0),daemon_version,state,capabilities,active_attempts,started_at,last_seen_at,stopped_at FROM daemon_instances`

func scanDaemonInstance(row pgx.Row) (domain.DaemonInstance, error) {
	var value domain.DaemonInstance
	var capabilities, attempts []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.DeviceID, &value.ConnectionEpoch, &value.ReportSequence, &value.PID, &value.Version, &value.State, &capabilities, &attempts, &value.StartedAt, &value.LastSeenAt, &value.StoppedAt)
	if err == nil {
		value.Capabilities, err = decodeJSON[map[string]any](capabilities)
	}
	if err == nil {
		value.ActiveAttempts, err = decodeJSON[[]string](attempts)
	}
	return value, err
}

func (s *Store) SaveDaemonInstance(ctx context.Context, value domain.DaemonInstance) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		var lockedDeviceID string
		if err := tx.QueryRow(ctx, `SELECT id FROM devices WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL FOR UPDATE`, value.TenantID, value.DeviceID).Scan(&lockedDeviceID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("设备")
			}
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM daemon_instances WHERE tenant_id=$1 AND id=$2)`, value.TenantID, value.ID).Scan(&exists); err != nil {
			return err
		}
		if !exists && value.ConnectionEpoch == 1 && value.ReportSequence == 1 && value.State != "stopped" {
			if _, err := tx.Exec(ctx, `UPDATE daemon_instances SET state='stopped',stopped_at=$3 WHERE tenant_id=$1 AND device_id=$2 AND state<>'stopped'`, value.TenantID, value.DeviceID, value.LastSeenAt); err != nil {
				return err
			}
		}
		result, err := tx.Exec(ctx, `INSERT INTO daemon_instances(id,tenant_id,device_id,connection_epoch,report_sequence,pid,daemon_version,state,capabilities,active_attempts,started_at,last_seen_at,stopped_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT (id) DO UPDATE SET connection_epoch=EXCLUDED.connection_epoch,report_sequence=EXCLUDED.report_sequence,pid=EXCLUDED.pid,daemon_version=EXCLUDED.daemon_version,state=EXCLUDED.state,capabilities=EXCLUDED.capabilities,active_attempts=EXCLUDED.active_attempts,last_seen_at=EXCLUDED.last_seen_at,stopped_at=EXCLUDED.stopped_at WHERE daemon_instances.tenant_id=EXCLUDED.tenant_id AND daemon_instances.device_id=EXCLUDED.device_id AND (EXCLUDED.connection_epoch>daemon_instances.connection_epoch OR (EXCLUDED.connection_epoch=daemon_instances.connection_epoch AND EXCLUDED.report_sequence>daemon_instances.report_sequence AND (daemon_instances.state<>'stopped' OR EXCLUDED.state='stopped'))) AND NOT EXISTS (SELECT 1 FROM daemon_instances other WHERE other.tenant_id=daemon_instances.tenant_id AND other.device_id=daemon_instances.device_id AND other.id<>daemon_instances.id AND other.state<>'stopped')`, value.ID, value.TenantID, value.DeviceID, value.ConnectionEpoch, value.ReportSequence, nullableInt(value.PID), value.Version, value.State, jsonValue(value.Capabilities), jsonArrayValue(value.ActiveAttempts), value.StartedAt, value.LastSeenAt, value.StoppedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.Conflict("DAEMON_INSTANCE_REPORT_STALE", "DaemonInstance 状态报告已过期")
		}
		return nil
	})
}

func (s *Store) DaemonInstance(ctx context.Context, tenantID, id string) (domain.DaemonInstance, error) {
	var result domain.DaemonInstance
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanDaemonInstance(tx.QueryRow(ctx, daemonInstanceSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("DaemonInstance")
		}
		result = value
		return err
	})
	return result, err
}

func (s *Store) DaemonInstances(ctx context.Context, tenantID, deviceID string) ([]domain.DaemonInstance, error) {
	values := []domain.DaemonInstance{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := daemonInstanceSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if deviceID != "" {
			query += ` AND device_id=$2`
			args = append(args, deviceID)
		}
		query += ` ORDER BY last_seen_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanDaemonInstance(rows)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
