package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const workspaceFolderSelect = `SELECT id,tenant_id,project_id,COALESCE(parent_id::text,''),name,created_by,created_at,updated_at FROM workspace_folders`

func scanWorkspaceFolder(row pgx.Row) (domain.WorkspaceFolder, error) {
	var value domain.WorkspaceFolder
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.ParentID, &value.Name, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func (s *Store) CreateWorkspaceFolder(ctx context.Context, value domain.WorkspaceFolder) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO workspace_folders(id,tenant_id,project_id,parent_id,name,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, value.ID, value.TenantID, value.ProjectID, nullable(value.ParentID), value.Name, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) WorkspaceFolders(ctx context.Context, tenantID, projectID string) ([]domain.WorkspaceFolder, error) {
	items := []domain.WorkspaceFolder{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := workspaceFolderSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if projectID != "" {
			query += ` AND project_id=$2`
			args = append(args, projectID)
		}
		query += ` ORDER BY name ASC,created_at ASC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanWorkspaceFolder(rows)
			if err != nil {
				return err
			}
			items = append(items, value)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Store) WorkspaceFolder(ctx context.Context, tenantID, id string) (domain.WorkspaceFolder, error) {
	var value domain.WorkspaceFolder
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		loaded, err := scanWorkspaceFolder(tx.QueryRow(ctx, workspaceFolderSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		value = loaded
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("文件夹")
		}
		return err
	})
	return value, err
}

const workspaceMaterialSelect = `SELECT id,tenant_id,project_id,COALESCE(folder_id::text,''),source_id,source_revision_id,material_kind,origin,usage,title,created_by,last_used_at,created_at,updated_at FROM workspace_materials`

func scanWorkspaceMaterial(row pgx.Row) (domain.WorkspaceMaterial, error) {
	var value domain.WorkspaceMaterial
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.FolderID, &value.SourceID, &value.SourceRevisionID, &value.MaterialKind, &value.Origin, &value.Usage, &value.Title, &value.CreatedBy, &value.LastUsedAt, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func (s *Store) CreateWorkspaceMaterial(ctx context.Context, value domain.WorkspaceMaterial) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO workspace_materials(id,tenant_id,project_id,folder_id,source_id,source_revision_id,material_kind,origin,usage,title,created_by,last_used_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, value.ID, value.TenantID, value.ProjectID, nullable(value.FolderID), value.SourceID, value.SourceRevisionID, value.MaterialKind, value.Origin, value.Usage, value.Title, value.CreatedBy, value.LastUsedAt, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) WorkspaceMaterials(ctx context.Context, tenantID, projectID string) ([]domain.WorkspaceMaterial, error) {
	items := []domain.WorkspaceMaterial{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := workspaceMaterialSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if projectID != "" {
			query += ` AND project_id=$2`
			args = append(args, projectID)
		}
		query += ` ORDER BY updated_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanWorkspaceMaterial(rows)
			if err != nil {
				return err
			}
			items = append(items, value)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Store) WorkspaceMaterial(ctx context.Context, tenantID, id string) (domain.WorkspaceMaterial, error) {
	var value domain.WorkspaceMaterial
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		loaded, err := scanWorkspaceMaterial(tx.QueryRow(ctx, workspaceMaterialSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		value = loaded
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("工作区资料")
		}
		return err
	})
	return value, err
}

func (s *Store) SaveWorkspaceMaterial(ctx context.Context, value domain.WorkspaceMaterial) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE workspace_materials SET folder_id=$3,usage=$4,title=$5,last_used_at=$6,updated_at=$7 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, nullable(value.FolderID), value.Usage, value.Title, value.LastUsedAt, value.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("工作区资料")
		}
		return nil
	})
}
