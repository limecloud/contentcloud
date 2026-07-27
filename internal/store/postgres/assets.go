package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const assetSelect = `SELECT id,tenant_id,project_id,name,asset_type,source_revision_id,usage_mode,status,created_by,created_at,updated_at FROM assets`

func scanAsset(row pgx.Row) (domain.Asset, error) {
	var value domain.Asset
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.Name, &value.AssetType, &value.SourceRevisionID, &value.UsageMode, &value.Status, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func (s *Store) CreateAsset(ctx context.Context, value domain.Asset) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO assets(id,tenant_id,project_id,name,asset_type,source_revision_id,usage_mode,status,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, value.ID, value.TenantID, value.ProjectID, value.Name, value.AssetType, value.SourceRevisionID, value.UsageMode, value.Status, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) Assets(ctx context.Context, tenantID, projectID string) ([]domain.Asset, error) {
	items := []domain.Asset{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, assetSelect+` WHERE tenant_id=$1 AND project_id=$2 ORDER BY created_at DESC`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanAsset(rows)
			if err != nil {
				return err
			}
			items = append(items, value)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Store) Asset(ctx context.Context, tenantID, id string) (domain.Asset, error) {
	var result domain.Asset
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanAsset(tx.QueryRow(ctx, assetSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("素材")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveAsset(ctx context.Context, value domain.Asset) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE assets SET name=$3,asset_type=$4,usage_mode=$5,status=$6,updated_at=$7 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.Name, value.AssetType, value.UsageMode, value.Status, value.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("素材")
		}
		return nil
	})
}

const rightsSelect = `SELECT id,tenant_id,project_id,asset_id,rights_holder,rights_type,territories,channels,valid_from,valid_until,proof_source_revision_id,restrictions,status,COALESCE(reviewed_by::text,''),reviewed_at,row_version,created_at,updated_at FROM rights_records`

func scanRightsRecord(row pgx.Row) (domain.RightsRecord, error) {
	var value domain.RightsRecord
	var territories, channels, restrictions []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.AssetID, &value.RightsHolder, &value.RightsType, &territories, &channels, &value.ValidFrom, &value.ValidUntil, &value.ProofSourceRevisionID, &restrictions, &value.Status, &value.ReviewedBy, &value.ReviewedAt, &value.RowVersion, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.Territories, err = decodeJSON[[]string](territories)
	}
	if err == nil {
		value.Channels, err = decodeJSON[[]string](channels)
	}
	if err == nil {
		value.Restrictions, err = decodeJSON[[]string](restrictions)
	}
	return value, err
}

func (s *Store) CreateRightsRecord(ctx context.Context, value domain.RightsRecord) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO rights_records(id,tenant_id,project_id,asset_id,rights_holder,rights_type,territories,channels,valid_from,valid_until,proof_source_revision_id,restrictions,status,reviewed_by,reviewed_at,row_version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, value.ID, value.TenantID, value.ProjectID, value.AssetID, value.RightsHolder, value.RightsType, jsonArrayValue(value.Territories), jsonArrayValue(value.Channels), value.ValidFrom, value.ValidUntil, value.ProofSourceRevisionID, jsonArrayValue(value.Restrictions), value.Status, nullable(value.ReviewedBy), value.ReviewedAt, value.RowVersion, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) RightsRecords(ctx context.Context, tenantID, assetID string) ([]domain.RightsRecord, error) {
	items := []domain.RightsRecord{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := rightsSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if assetID != "" {
			query += ` AND asset_id=$2`
			args = append(args, assetID)
		}
		query += ` ORDER BY created_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRightsRecord(rows)
			if err != nil {
				return err
			}
			items = append(items, value)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Store) RightsRecord(ctx context.Context, tenantID, id string) (domain.RightsRecord, error) {
	var result domain.RightsRecord
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRightsRecord(tx.QueryRow(ctx, rightsSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("权利记录")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveRightsRecord(ctx context.Context, value domain.RightsRecord) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE rights_records SET rights_holder=$3,rights_type=$4,territories=$5,channels=$6,valid_from=$7,valid_until=$8,proof_source_revision_id=$9,restrictions=$10,status=$11,reviewed_by=$12,reviewed_at=$13,row_version=$14,updated_at=$15 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.RightsHolder, value.RightsType, jsonArrayValue(value.Territories), jsonArrayValue(value.Channels), value.ValidFrom, value.ValidUntil, value.ProofSourceRevisionID, jsonArrayValue(value.Restrictions), value.Status, nullable(value.ReviewedBy), value.ReviewedAt, value.RowVersion, value.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("权利记录")
		}
		return nil
	})
}
