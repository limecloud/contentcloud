package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateDeliveryPackage(ctx context.Context, value domain.DeliveryPackage, artifacts []domain.Artifact) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO delivery_packages(id,tenant_id,project_id,content_item_id,status,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, value.ID, value.TenantID, value.ProjectID, value.ContentItemID, value.Status, value.CreatedBy, value.CreatedAt); err != nil {
			return dbError(err)
		}
		for position, snapshotID := range value.ApprovedSnapshotIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO delivery_package_snapshots(tenant_id,delivery_package_id,approved_snapshot_id,position) VALUES($1,$2,$3,$4)`, value.TenantID, value.ID, snapshotID, position); err != nil {
				return dbError(err)
			}
		}
		for position, artifact := range artifacts {
			if err := insertArtifact(ctx, tx, artifact); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO delivery_package_artifacts(tenant_id,delivery_package_id,artifact_id,position) VALUES($1,$2,$3,$4)`, value.TenantID, value.ID, artifact.ID, position); err != nil {
				return dbError(err)
			}
		}
		return nil
	})
}

func (s *Store) DeliveryPackages(ctx context.Context, tenantID, projectID string) ([]domain.DeliveryPackage, error) {
	values := []domain.DeliveryPackage{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id,tenant_id,project_id,content_item_id,status,created_by,created_at FROM delivery_packages WHERE tenant_id=$1 AND ($2='' OR project_id::text=$2) ORDER BY created_at DESC`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value domain.DeliveryPackage
			if err := rows.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.ContentItemID, &value.Status, &value.CreatedBy, &value.CreatedAt); err != nil {
				return err
			}
			values = append(values, value)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for index := range values {
			if err := loadDeliveryPackageRelations(ctx, tx, &values[index]); err != nil {
				return err
			}
		}
		return nil
	})
	return values, err
}

func (s *Store) DeliveryPackage(ctx context.Context, tenantID, id string) (domain.DeliveryPackage, error) {
	var value domain.DeliveryPackage
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id,tenant_id,project_id,content_item_id,status,created_by,created_at FROM delivery_packages WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.ContentItemID, &value.Status, &value.CreatedBy, &value.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("DeliveryPackage")
		}
		if err != nil {
			return err
		}
		return loadDeliveryPackageRelations(ctx, tx, &value)
	})
	return value, err
}

func loadDeliveryPackageRelations(ctx context.Context, tx pgx.Tx, value *domain.DeliveryPackage) error {
	snapshotRows, err := tx.Query(ctx, `SELECT approved_snapshot_id FROM delivery_package_snapshots WHERE tenant_id=$1 AND delivery_package_id=$2 ORDER BY position`, value.TenantID, value.ID)
	if err != nil {
		return err
	}
	defer snapshotRows.Close()
	value.ApprovedSnapshotIDs = []string{}
	for snapshotRows.Next() {
		var id string
		if err := snapshotRows.Scan(&id); err != nil {
			return err
		}
		value.ApprovedSnapshotIDs = append(value.ApprovedSnapshotIDs, id)
	}
	if err := snapshotRows.Err(); err != nil {
		return err
	}
	snapshotRows.Close()
	artifactRows, err := tx.Query(ctx, artifactSelect+` WHERE tenant_id=$1 AND id IN (SELECT artifact_id FROM delivery_package_artifacts WHERE tenant_id=$1 AND delivery_package_id=$2) ORDER BY (SELECT position FROM delivery_package_artifacts WHERE delivery_package_id=$2 AND artifact_id=artifacts.id)`, value.TenantID, value.ID)
	if err != nil {
		return err
	}
	defer artifactRows.Close()
	value.Manifest = []domain.Artifact{}
	for artifactRows.Next() {
		artifact, err := scanArtifact(artifactRows)
		if err != nil {
			return err
		}
		value.Manifest = append(value.Manifest, artifact)
	}
	return artifactRows.Err()
}
