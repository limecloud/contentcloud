package postgres

import (
	"context"
	"errors"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/jackc/pgx/v5"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

func (s *Store) CreateSnapshot(ctx context.Context, value sourcedomain.ContextSnapshot) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO context_snapshots(id,tenant_id,project_id,builder_version,schema_version,payload,manifest_hash,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, value.ID, value.TenantID, value.ProjectID, value.BuilderVersion, value.SchemaVersion, jsonValue(value), value.ManifestHash, value.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) Snapshot(ctx context.Context, tenantID, id string) (sourcedomain.ContextSnapshot, error) {
	var result sourcedomain.ContextSnapshot
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var body []byte
		if err := tx.QueryRow(ctx, `SELECT payload FROM context_snapshots WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&body); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fault.NotFound("上下文快照")
			}
			return err
		}
		value, err := decodeJSON[sourcedomain.ContextSnapshot](body)
		result = value
		return err
	})
	return result, err
}
