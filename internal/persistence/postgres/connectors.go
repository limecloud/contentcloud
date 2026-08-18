package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/integration/connector"
)

const connectorBindingSelect = `SELECT id,tenant_id,project_id,connector_id,authorization_ref,region,status,cursor,created_by,created_at,updated_at FROM connector_bindings`

func scanConnectorBinding(row pgx.Row) (connector.Binding, error) {
	var value connector.Binding
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.ConnectorID, &value.AuthorizationRef, &value.Region, &value.Status, &value.Cursor, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func (s *Store) CreateBinding(ctx context.Context, value connector.Binding) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO connector_bindings(id,tenant_id,project_id,connector_id,authorization_ref,region,status,cursor,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, value.ID, value.TenantID, value.ProjectID, value.ConnectorID, value.AuthorizationRef, value.Region, value.Status, value.Cursor, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) Bindings(ctx context.Context, tenantID, projectID string) ([]connector.Binding, error) {
	values := []connector.Binding{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, connectorBindingSelect+` WHERE tenant_id=$1 AND ($2='' OR project_id::text=$2) ORDER BY created_at`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanConnectorBinding(rows)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}

func (s *Store) Binding(ctx context.Context, tenantID, id string) (connector.Binding, error) {
	var value connector.Binding
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var err error
		value, err = scanConnectorBinding(tx.QueryRow(ctx, connectorBindingSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("Connector 绑定")
		}
		return err
	})
	return value, err
}

func (s *Store) AcquireSyncLease(ctx context.Context, tenantID, bindingID string, lease connector.SyncLease) error {
	if lease.Owner == "" || !lease.ExpiresAt.After(time.Now().UTC()) {
		return fault.Invalid("CONNECTOR_SYNC_LEASE_INVALID", "Connector 同步租约缺少所有者或有效期")
	}
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE connector_bindings SET sync_lease_owner=$3,sync_lease_expires_at=$4 WHERE tenant_id=$1 AND id=$2 AND (sync_lease_owner='' OR sync_lease_expires_at<=now() OR sync_lease_owner=$3)`, tenantID, bindingID, lease.Owner, lease.ExpiresAt)
		if err != nil {
			return dbError(err)
		}
		if command.RowsAffected() == 1 {
			return nil
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connector_bindings WHERE tenant_id=$1 AND id=$2)`, tenantID, bindingID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fault.NotFound("Connector 绑定")
		}
		return fault.Conflict("CONNECTOR_SYNC_IN_PROGRESS", "Connector 绑定正在同步")
	})
}

func (s *Store) ReleaseSyncLease(ctx context.Context, tenantID, bindingID, owner string) error {
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE connector_bindings SET sync_lease_owner='',sync_lease_expires_at='epoch'::timestamptz WHERE tenant_id=$1 AND id=$2 AND sync_lease_owner=$3`, tenantID, bindingID, owner)
		return dbError(err)
	})
}

const connectorRecordSelect = `SELECT tenant_id,project_id,binding_id,external_id,external_version,COALESCE(source_id::text,''),COALESCE(revision_id::text,''),digest,source_url,deleted,deleted_at,rights,metadata,observed_at FROM connector_record_mappings`

func scanConnectorRecord(row pgx.Row) (connector.RecordMapping, error) {
	var value connector.RecordMapping
	var rights, metadata []byte
	err := row.Scan(&value.TenantID, &value.ProjectID, &value.BindingID, &value.ExternalID, &value.ExternalVersion, &value.SourceID, &value.RevisionID, &value.Digest, &value.SourceURL, &value.Deleted, &value.DeletedAt, &rights, &metadata, &value.ObservedAt)
	if err == nil {
		value.Rights, err = decodeJSON[map[string]any](rights)
	}
	if err == nil {
		value.Metadata, err = decodeJSON[map[string]any](metadata)
	}
	value.NormalizeCollections()
	return value, err
}

func (s *Store) Record(ctx context.Context, tenantID, bindingID, externalID string) (connector.RecordMapping, error) {
	var value connector.RecordMapping
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var err error
		value, err = scanConnectorRecord(tx.QueryRow(ctx, connectorRecordSelect+` WHERE tenant_id=$1 AND binding_id=$2 AND external_id=$3`, tenantID, bindingID, externalID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("Connector 记录映射")
		}
		return err
	})
	return value, err
}

func (s *Store) SaveRecord(ctx context.Context, value connector.RecordMapping) error {
	value.NormalizeCollections()
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO connector_record_mappings(tenant_id,project_id,binding_id,external_id,external_version,source_id,revision_id,digest,source_url,deleted,deleted_at,rights,metadata,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (tenant_id,binding_id,external_id) DO UPDATE SET external_version=EXCLUDED.external_version,source_id=EXCLUDED.source_id,revision_id=EXCLUDED.revision_id,digest=EXCLUDED.digest,source_url=EXCLUDED.source_url,deleted=EXCLUDED.deleted,deleted_at=EXCLUDED.deleted_at,rights=EXCLUDED.rights,metadata=EXCLUDED.metadata,observed_at=EXCLUDED.observed_at`, value.TenantID, value.ProjectID, value.BindingID, value.ExternalID, value.ExternalVersion, nullable(value.SourceID), nullable(value.RevisionID), value.Digest, value.SourceURL, value.Deleted, value.DeletedAt, jsonValue(value.Rights), jsonValue(value.Metadata), value.ObservedAt)
		return dbError(err)
	})
}

func (s *Store) ActiveRecordsForSource(ctx context.Context, tenantID, sourceID string) ([]connector.RecordMapping, error) {
	values := []connector.RecordMapping{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, connectorRecordSelect+` WHERE tenant_id=$1 AND source_id=$2 AND deleted=false ORDER BY observed_at`, tenantID, sourceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanConnectorRecord(rows)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}

func (s *Store) CommitReceipt(ctx context.Context, binding connector.Binding, expectedCursor, leaseOwner string, receipt connector.SyncReceipt) error {
	return s.withTenant(ctx, binding.TenantID, func(tx pgx.Tx) error {
		var cursor, currentLeaseOwner string
		var leaseExpiresAt time.Time
		if err := tx.QueryRow(ctx, `SELECT cursor,sync_lease_owner,sync_lease_expires_at FROM connector_bindings WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, binding.TenantID, binding.ID).Scan(&cursor, &currentLeaseOwner, &leaseExpiresAt); errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("Connector 绑定")
		} else if err != nil {
			return err
		}
		if cursor != expectedCursor {
			return fault.Conflict("CONNECTOR_CURSOR_CONFLICT", "Connector 游标已被另一同步推进")
		}
		if currentLeaseOwner != leaseOwner || !leaseExpiresAt.After(time.Now().UTC()) {
			return fault.Conflict("CONNECTOR_SYNC_LEASE_LOST", "Connector 同步租约已失效")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO connector_sync_receipts(tenant_id,id,project_id,binding_id,connector_id,previous_cursor,next_cursor,records,upsert_count,tombstone_count,has_more,digest,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, receipt.TenantID, receipt.ID, receipt.ProjectID, receipt.BindingID, receipt.ConnectorID, receipt.PreviousCursor, receipt.NextCursor, jsonArrayValue(receipt.Records), receipt.UpsertCount, receipt.TombstoneCount, receipt.HasMore, receipt.Digest, receipt.ObservedAt); err != nil {
			return dbError(err)
		}
		command, err := tx.Exec(ctx, `UPDATE connector_bindings SET cursor=$3,updated_at=$4,sync_lease_owner='',sync_lease_expires_at='epoch'::timestamptz WHERE tenant_id=$1 AND id=$2 AND sync_lease_owner=$5`, binding.TenantID, binding.ID, receipt.NextCursor, receipt.ObservedAt, leaseOwner)
		if err == nil && command.RowsAffected() != 1 {
			return fault.NotFound("Connector 绑定")
		}
		return dbError(err)
	})
}

func (s *Store) Receipts(ctx context.Context, tenantID, bindingID string) ([]connector.SyncReceipt, error) {
	values := []connector.SyncReceipt{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id,tenant_id,project_id,binding_id,connector_id,previous_cursor,next_cursor,records,upsert_count,tombstone_count,has_more,digest,observed_at FROM connector_sync_receipts WHERE tenant_id=$1 AND ($2='' OR binding_id=$2) ORDER BY observed_at DESC`, tenantID, bindingID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value connector.SyncReceipt
			var records []byte
			if err := rows.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.BindingID, &value.ConnectorID, &value.PreviousCursor, &value.NextCursor, &records, &value.UpsertCount, &value.TombstoneCount, &value.HasMore, &value.Digest, &value.ObservedAt); err != nil {
				return err
			}
			value.SchemaVersion = "contentcloud.connector-sync/1.0"
			value.Records, err = decodeJSON[[]connector.Record](records)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}
