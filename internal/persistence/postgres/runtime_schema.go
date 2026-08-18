package postgres

import (
	"context"
	"errors"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/jackc/pgx/v5"
)

const runtimeSchemaSelect = `SELECT tenant_id,schema_id,revision,status,compatibility,definition,digest,retention_policy,retain_until,created_by,created_at,published_at,retired_at,version FROM runtime_schemas`

func scanRuntimeSchema(row pgx.Row) (contentruntime.RuntimeSchema, error) {
	var schema contentruntime.RuntimeSchema
	var definition []byte
	err := row.Scan(&schema.TenantID, &schema.SchemaID, &schema.Revision, &schema.Status, &schema.Compatibility, &definition, &schema.Digest, &schema.RetentionPolicy, &schema.RetainUntil, &schema.CreatedBy, &schema.CreatedAt, &schema.PublishedAt, &schema.RetiredAt, &schema.Version)
	if err != nil {
		return schema, dbError(err)
	}
	schema.Definition, err = decodeJSON[map[string]any](definition)
	if err != nil {
		return schema, err
	}
	return schema, nil
}

func (s *Store) CreateRuntimeSchema(ctx context.Context, schema contentruntime.RuntimeSchema) error {
	if err := schema.Validate(); err != nil {
		return err
	}
	return s.withTenantCommand(ctx, schema.TenantID, "runtime.create_schema", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_schemas(tenant_id,schema_id,revision,status,compatibility,definition,digest,retention_policy,retain_until,created_by,created_at,published_at,retired_at,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, schema.TenantID, schema.SchemaID, schema.Revision, schema.Status, schema.Compatibility, jsonValue(schema.Definition), schema.Digest, schema.RetentionPolicy, schema.RetainUntil, schema.CreatedBy, schema.CreatedAt, schema.PublishedAt, schema.RetiredAt, schema.Version)
		return dbError(err)
	})
}

func (s *Store) RuntimeSchema(ctx context.Context, tenantID, schemaID string, revision int) (contentruntime.RuntimeSchema, error) {
	var result contentruntime.RuntimeSchema
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanRuntimeSchema(tx.QueryRow(ctx, runtimeSchemaSelect+` WHERE tenant_id=$1 AND schema_id=$2 AND revision=$3`, tenantID, schemaID, revision))
		result = value
		return err
	})
	return result, err
}

func (s *Store) RuntimeSchemas(ctx context.Context, tenantID, schemaID string) ([]contentruntime.RuntimeSchema, error) {
	result := []contentruntime.RuntimeSchema{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := runtimeSchemaSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if schemaID != "" {
			query += ` AND schema_id=$2`
			args = append(args, schemaID)
		}
		query += ` ORDER BY schema_id,revision DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanRuntimeSchema(rows)
			if err != nil {
				return err
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) PublishRuntimeSchema(ctx context.Context, next contentruntime.RuntimeSchema, expectedVersion int) (contentruntime.RuntimeSchema, error) {
	return s.transitionRuntimeSchema(ctx, next, expectedVersion, "draft", "published")
}

func (s *Store) RetireRuntimeSchema(ctx context.Context, next contentruntime.RuntimeSchema, expectedVersion int) (contentruntime.RuntimeSchema, error) {
	return s.transitionRuntimeSchema(ctx, next, expectedVersion, "published", "retired")
}

func (s *Store) transitionRuntimeSchema(ctx context.Context, next contentruntime.RuntimeSchema, expectedVersion int, expectedStatus, nextStatus string) (contentruntime.RuntimeSchema, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	var result contentruntime.RuntimeSchema
	err := s.withTenantCommand(ctx, next.TenantID, "runtime."+nextStatus+"_schema", func(tx pgx.Tx) error {
		current, err := scanRuntimeSchema(tx.QueryRow(ctx, runtimeSchemaSelect+` WHERE tenant_id=$1 AND schema_id=$2 AND revision=$3 FOR UPDATE`, next.TenantID, next.SchemaID, next.Revision))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("Runtime Schema")
		}
		if err != nil {
			return err
		}
		if current.Version != expectedVersion || current.Status != expectedStatus || next.Version != expectedVersion+1 || next.Status != nextStatus {
			return fault.Conflict("RUNTIME_SCHEMA_VERSION_CONFLICT", "Runtime Schema 已被更新")
		}
		_, err = tx.Exec(ctx, `UPDATE runtime_schemas SET status=$4,retain_until=$5,published_at=$6,retired_at=$7,version=$8 WHERE tenant_id=$1 AND schema_id=$2 AND revision=$3 AND version=$9 AND status=$10`, next.TenantID, next.SchemaID, next.Revision, next.Status, next.RetainUntil, next.PublishedAt, next.RetiredAt, next.Version, expectedVersion, expectedStatus)
		if err != nil {
			return dbError(err)
		}
		result = next
		return nil
	})
	return result, err
}
