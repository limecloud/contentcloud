package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateSnapshot(ctx context.Context, value domain.ContextSnapshot) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO context_snapshots(id,tenant_id,project_id,builder_version,schema_version,payload,manifest_hash,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, value.ID, value.TenantID, value.ProjectID, value.BuilderVersion, value.SchemaVersion, jsonValue(value), value.ManifestHash, value.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) Snapshot(ctx context.Context, tenantID, id string) (domain.ContextSnapshot, error) {
	var result domain.ContextSnapshot
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var body []byte
		if err := tx.QueryRow(ctx, `SELECT payload FROM context_snapshots WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&body); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("上下文快照")
			}
			return err
		}
		value, err := decodeJSON[domain.ContextSnapshot](body)
		result = value
		return err
	})
	return result, err
}
