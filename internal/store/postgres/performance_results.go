package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

const performanceObservationSelect = `SELECT id,tenant_id,project_id,COALESCE(import_batch_id::text,''),COALESCE(row_number,0),approved_snapshot_id,platform,account_alias,published_at,window_hours,sample_status,metrics,currency,spend,gmv,roi,COALESCE(dedup_key,''),issue_category,notes,created_at FROM performance_observations`

func (s *Store) CreatePerformanceImportBatch(ctx context.Context, batch domain.PerformanceImportBatch, observations []domain.PerformanceObservation) error {
	return s.withTenant(ctx, batch.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO performance_import_batches(id,tenant_id,project_id,source_name,source_format,source_sha256,currency,row_count,imported_count,status,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, batch.ID, batch.TenantID, batch.ProjectID, batch.SourceName, batch.SourceFormat, batch.SourceSHA256, batch.Currency, batch.RowCount, batch.ImportedCount, batch.Status, batch.CreatedBy, batch.CreatedAt)
		if err != nil {
			return dbError(err)
		}
		for _, v := range observations {
			_, err = tx.Exec(ctx, `INSERT INTO performance_observations(id,tenant_id,project_id,import_batch_id,row_number,approved_snapshot_id,platform,account_alias,published_at,window_hours,sample_status,metrics,currency,spend,gmv,roi,dedup_key,issue_category,notes,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`, v.ID, v.TenantID, v.ProjectID, v.ImportBatchID, v.RowNumber, v.ApprovedSnapshotID, v.Platform, v.AccountAlias, v.PublishedAt, v.WindowHours, v.SampleStatus, jsonValue(v.Metrics), v.Currency, v.Spend, v.GMV, v.ROI, v.DedupKey, v.IssueCategory, v.Notes, v.CreatedAt)
			if err != nil {
				return dbError(err)
			}
		}
		return nil
	})
}

func (s *Store) PerformanceImportBatches(ctx context.Context, tenantID, projectID string) ([]domain.PerformanceImportBatch, error) {
	out := []domain.PerformanceImportBatch{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id,tenant_id,project_id,source_name,source_format,source_sha256,currency,row_count,imported_count,status,created_by,created_at FROM performance_import_batches WHERE tenant_id=$1 AND project_id=$2 ORDER BY created_at DESC`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.PerformanceImportBatch
			if err := rows.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.SourceName, &v.SourceFormat, &v.SourceSHA256, &v.Currency, &v.RowCount, &v.ImportedCount, &v.Status, &v.CreatedBy, &v.CreatedAt); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) PerformanceImportBatch(ctx context.Context, tenantID, id string) (domain.PerformanceImportBatch, error) {
	var result domain.PerformanceImportBatch
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id,tenant_id,project_id,source_name,source_format,source_sha256,currency,row_count,imported_count,status,created_by,created_at FROM performance_import_batches WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&result.ID, &result.TenantID, &result.ProjectID, &result.SourceName, &result.SourceFormat, &result.SourceSHA256, &result.Currency, &result.RowCount, &result.ImportedCount, &result.Status, &result.CreatedBy, &result.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("结果导入批次")
		}
		return err
	})
	return result, err
}

func (s *Store) ExistingPerformanceDedupKeys(ctx context.Context, tenantID, projectID string, keys []string) (map[string]string, error) {
	out := map[string]string{}
	if len(keys) == 0 {
		return out, nil
	}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT dedup_key,id FROM performance_observations WHERE tenant_id=$1 AND project_id=$2 AND dedup_key=ANY($3::text[])`, tenantID, projectID, keys)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key, id string
			if err := rows.Scan(&key, &id); err != nil {
				return err
			}
			out[key] = id
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) PerformanceObservation(ctx context.Context, tenantID, id string) (domain.PerformanceObservation, error) {
	var result domain.PerformanceObservation
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanPerformanceObservation(tx.QueryRow(ctx, performanceObservationSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("结果观察")
		}
		return err
	})
	return result, err
}

func (s *Store) PerformanceObservations(ctx context.Context, tenantID, projectID string) ([]domain.PerformanceObservation, error) {
	out := []domain.PerformanceObservation{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, performanceObservationSelect+` WHERE tenant_id=$1 AND project_id=$2 ORDER BY published_at DESC, row_number`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanPerformanceObservation(rows)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func scanPerformanceObservation(row pgx.Row) (domain.PerformanceObservation, error) {
	var v domain.PerformanceObservation
	var metrics []byte
	err := row.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.ImportBatchID, &v.RowNumber, &v.ApprovedSnapshotID, &v.Platform, &v.AccountAlias, &v.PublishedAt, &v.WindowHours, &v.SampleStatus, &metrics, &v.Currency, &v.Spend, &v.GMV, &v.ROI, &v.DedupKey, &v.IssueCategory, &v.Notes, &v.CreatedAt)
	if err != nil {
		return v, err
	}
	v.Metrics, err = decodeJSON[map[string]float64](metrics)
	return v, err
}

func (s *Store) CreateRatingDecision(ctx context.Context, v domain.RatingDecision) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO rating_decisions(id,tenant_id,project_id,subject_type,subject_id,observation_ids,rating,reason,next_action,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, v.ID, v.TenantID, v.ProjectID, v.SubjectType, v.SubjectID, jsonArrayValue(v.ObservationIDs), v.Rating, v.Reason, v.NextAction, v.CreatedBy, v.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) RatingDecisions(ctx context.Context, tenantID, projectID string) ([]domain.RatingDecision, error) {
	out := []domain.RatingDecision{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id,tenant_id,project_id,subject_type,subject_id,observation_ids,rating,reason,next_action,created_by,created_at FROM rating_decisions WHERE tenant_id=$1 AND project_id=$2 ORDER BY created_at DESC`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.RatingDecision
			var observationIDs []byte
			if err := rows.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.SubjectType, &v.SubjectID, &observationIDs, &v.Rating, &v.Reason, &v.NextAction, &v.CreatedBy, &v.CreatedAt); err != nil {
				return err
			}
			v.ObservationIDs, err = decodeJSON[[]string](observationIDs)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}
