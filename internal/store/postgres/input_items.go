package postgres

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const inputItemSelect = `SELECT tenant_id,id,COALESCE(project_id::text,''),source_type,title,summary,body,source_ref,source_digest,disclosure,status,target_task_id,assignee_user_id,missing_fields,metadata,idempotency_key,row_version,created_by,created_at,updated_at FROM input_items`

func scanInputItem(row pgx.Row) (domain.InputItem, error) {
	var value domain.InputItem
	var missingFields, metadata []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.SourceType, &value.Title, &value.Summary, &value.Body, &value.SourceRef, &value.SourceDigest, &value.Disclosure, &value.Status, &value.TargetTaskID, &value.AssigneeUserID, &missingFields, &metadata, &value.IdempotencyKey, &value.RowVersion, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.MissingFields, err = decodeJSON[[]string](missingFields)
	}
	if err == nil {
		value.Metadata, err = decodeJSON[map[string]any](metadata)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateInputItem(ctx context.Context, value domain.InputItem) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO input_items(tenant_id,id,project_id,source_type,title,summary,body,source_ref,source_digest,disclosure,status,target_task_id,assignee_user_id,missing_fields,metadata,idempotency_key,row_version,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`, value.TenantID, value.ID, nullable(value.ProjectID), value.SourceType, value.Title, value.Summary, value.Body, value.SourceRef, value.SourceDigest, value.Disclosure, value.Status, value.TargetTaskID, value.AssigneeUserID, jsonArrayValue(value.MissingFields), jsonValue(value.Metadata), value.IdempotencyKey, value.RowVersion, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) InputItems(ctx context.Context, tenantID, projectID, status, assigneeUserID string) ([]domain.InputItem, error) {
	result := []domain.InputItem{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := inputItemSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if projectID != "" {
			query += ` AND project_id=$2`
			args = append(args, projectID)
		}
		if status != "" {
			query += ` AND status=$` + strconv.Itoa(len(args)+1)
			args = append(args, status)
		}
		if assigneeUserID != "" {
			query += ` AND assignee_user_id=$` + strconv.Itoa(len(args)+1)
			args = append(args, assigneeUserID)
		}
		query += ` ORDER BY updated_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, scanErr := scanInputItem(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) InputItem(ctx context.Context, tenantID, id string) (domain.InputItem, error) {
	var result domain.InputItem
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanInputItem(tx.QueryRow(ctx, inputItemSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("输入收集记录")
		}
		return err
	})
	return result, err
}

func (s *Store) InputItemByIdempotencyKey(ctx context.Context, tenantID, key string) (domain.InputItem, error) {
	var result domain.InputItem
	if key == "" {
		return result, domain.NotFound("输入收集记录")
	}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanInputItem(tx.QueryRow(ctx, inputItemSelect+` WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("输入收集记录")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveInputItem(ctx context.Context, value domain.InputItem, expectedVersion int) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE input_items SET project_id=$3,source_type=$4,title=$5,summary=$6,body=$7,source_ref=$8,source_digest=$9,disclosure=$10,status=$11,target_task_id=$12,assignee_user_id=$13,missing_fields=$14,metadata=$15,row_version=$16,updated_at=$17 WHERE tenant_id=$1 AND id=$2 AND row_version=$18`, value.TenantID, value.ID, nullable(value.ProjectID), value.SourceType, value.Title, value.Summary, value.Body, value.SourceRef, value.SourceDigest, value.Disclosure, value.Status, value.TargetTaskID, value.AssigneeUserID, jsonArrayValue(value.MissingFields), jsonValue(value.Metadata), value.RowVersion, value.UpdatedAt, expectedVersion)
		if err == nil && command.RowsAffected() == 0 {
			return domain.Conflict("INPUT_ITEM_VERSION_CONFLICT", "输入收集记录已被其他人更新，请刷新后重试")
		}
		return dbError(err)
	})
}
