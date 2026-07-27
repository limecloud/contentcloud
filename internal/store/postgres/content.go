package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateSource(ctx context.Context, source domain.Source, revision domain.SourceRevision) error {
	return s.withTenant(ctx, source.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO sources(id,tenant_id,project_id,name,source_type,created_at) VALUES($1,$2,$3,$4,$5,$6)`, source.ID, source.TenantID, source.ProjectID, source.Name, source.SourceType, source.CreatedAt); err != nil {
			return dbError(err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO source_revisions(id,tenant_id,project_id,source_id,file_name,object_key,sha256,byte_size,declared_mime,detected_mime,processing_status,parser_version,error_code,supersedes_id,uploaded_by,effective_from,effective_to,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, revision.ID, revision.TenantID, revision.ProjectID, revision.SourceID, revision.FileName, revision.ObjectKey, revision.SHA256, revision.ByteSize, revision.DeclaredMIME, nullable(revision.DetectedMIME), revision.ProcessingStatus, nullable(revision.ParserVersion), revision.ErrorCode, nullable(revision.SupersedesID), nullable(revision.UploadedBy), revision.EffectiveFrom, revision.EffectiveTo, revision.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) CreateSourceRevision(ctx context.Context, revision domain.SourceRevision) error {
	return s.withTenant(ctx, revision.TenantID, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sources WHERE tenant_id=$1 AND project_id=$2 AND id=$3)`, revision.TenantID, revision.ProjectID, revision.SourceID).Scan(&exists); err != nil || !exists {
			return domain.NotFound("来源")
		}
		_, err := tx.Exec(ctx, `INSERT INTO source_revisions(id,tenant_id,project_id,source_id,file_name,object_key,sha256,byte_size,declared_mime,detected_mime,processing_status,parser_version,error_code,supersedes_id,uploaded_by,effective_from,effective_to,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, revision.ID, revision.TenantID, revision.ProjectID, revision.SourceID, revision.FileName, revision.ObjectKey, revision.SHA256, revision.ByteSize, revision.DeclaredMIME, nullable(revision.DetectedMIME), revision.ProcessingStatus, nullable(revision.ParserVersion), revision.ErrorCode, nullable(revision.SupersedesID), nullable(revision.UploadedBy), revision.EffectiveFrom, revision.EffectiveTo, revision.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) Sources(ctx context.Context, tenantID, projectID string) ([]domain.Source, error) {
	out := []domain.Source{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT s.id,s.tenant_id,s.project_id,s.name,s.source_type,COALESCE(latest.processing_status,'pending'),count(r.id),COALESCE(latest.id::text,''),s.created_at
FROM sources s LEFT JOIN source_revisions r ON r.source_id=s.id LEFT JOIN LATERAL (SELECT id,processing_status FROM source_revisions lr WHERE lr.source_id=s.id ORDER BY lr.created_at DESC LIMIT 1) latest ON true
WHERE s.tenant_id=$1 AND s.project_id=$2 GROUP BY s.id,latest.id,latest.processing_status ORDER BY s.created_at DESC`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.Source
			if err := rows.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.Name, &v.SourceType, &v.Status, &v.RevisionCount, &v.LatestRevision, &v.CreatedAt); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) Source(ctx context.Context, tenantID, id string) (domain.Source, error) {
	var v domain.Source
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT s.id,s.tenant_id,s.project_id,s.name,s.source_type,COALESCE(latest.processing_status,'pending'),(SELECT count(*) FROM source_revisions r WHERE r.source_id=s.id),COALESCE(latest.id::text,''),s.created_at
FROM sources s LEFT JOIN LATERAL (SELECT id,processing_status FROM source_revisions lr WHERE lr.source_id=s.id ORDER BY lr.created_at DESC LIMIT 1) latest ON true WHERE s.tenant_id=$1 AND s.id=$2`, tenantID, id).Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.Name, &v.SourceType, &v.Status, &v.RevisionCount, &v.LatestRevision, &v.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("来源")
		}
		return err
	})
	return v, err
}

func (s *Store) SourceRevisions(ctx context.Context, tenantID, sourceID string) ([]domain.SourceRevision, error) {
	out := []domain.SourceRevision{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, revisionSelect+` WHERE tenant_id=$1 AND source_id=$2 ORDER BY created_at DESC`, tenantID, sourceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanRevision(rows)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func scanRevision(row pgx.Row) (domain.SourceRevision, error) {
	var v domain.SourceRevision
	err := row.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.SourceID, &v.FileName, &v.ObjectKey, &v.SHA256, &v.ByteSize, &v.DeclaredMIME, &v.DetectedMIME, &v.ProcessingStatus, &v.ParserVersion, &v.ErrorCode, &v.SupersedesID, &v.UploadedBy, &v.EffectiveFrom, &v.EffectiveTo, &v.CreatedAt)
	return v, err
}

const revisionSelect = `SELECT id,tenant_id,project_id,source_id,file_name,object_key,sha256,byte_size,declared_mime,COALESCE(detected_mime,''),processing_status,COALESCE(parser_version,''),error_code,COALESCE(supersedes_id::text,''),COALESCE(uploaded_by::text,''),effective_from,effective_to,created_at FROM source_revisions`

func (s *Store) SourceRevision(ctx context.Context, tenantID, id string) (domain.SourceRevision, error) {
	var result domain.SourceRevision
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanRevision(tx.QueryRow(ctx, revisionSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("来源版本")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveSourceRevision(ctx context.Context, v domain.SourceRevision) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE source_revisions SET detected_mime=$3,processing_status=$4,parser_version=$5,error_code=$6 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, nullable(v.DetectedMIME), v.ProcessingStatus, nullable(v.ParserVersion), v.ErrorCode)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("来源版本")
		}
		return nil
	})
}

func (s *Store) PendingSourceRevisions(ctx context.Context, limit int) ([]domain.SourceRevision, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `SELECT tenant_id,revision_id FROM contentcloud_pending_source_revisions($1)`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type ref struct{ tenantID, revisionID string }
	refs := []ref{}
	for rows.Next() {
		var value ref
		if err := rows.Scan(&value.tenantID, &value.revisionID); err != nil {
			return nil, err
		}
		refs = append(refs, value)
	}
	out := []domain.SourceRevision{}
	for _, value := range refs {
		revision, err := s.SourceRevision(ctx, value.tenantID, value.revisionID)
		if err != nil {
			return nil, err
		}
		out = append(out, revision)
	}
	return out, rows.Err()
}

func (s *Store) ClaimSourceRevision(ctx context.Context, tenantID, id string) (domain.SourceRevision, bool, error) {
	var result domain.SourceRevision
	claimed := false
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `UPDATE source_revisions SET processing_status='processing' WHERE tenant_id=$1 AND id=$2 AND processing_status='pending' RETURNING id,tenant_id,project_id,source_id,file_name,object_key,sha256,byte_size,declared_mime,COALESCE(detected_mime,''),processing_status,COALESCE(parser_version,''),error_code,COALESCE(supersedes_id::text,''),COALESCE(uploaded_by::text,''),effective_from,effective_to,created_at`, tenantID, id)
		v, err := scanRevision(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		result, claimed = v, true
		return nil
	})
	return result, claimed, err
}

func (s *Store) CreateEvidence(ctx context.Context, v domain.EvidenceSpan) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO evidence_spans(id,tenant_id,project_id,revision_id,locator_kind,locator,quote_text,quote_hash,ocr_confidence,review_status,reviewed_by,reviewed_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, v.ID, v.TenantID, v.ProjectID, v.RevisionID, v.LocatorKind, jsonValue(v.Locator), v.QuoteText, v.QuoteHash, v.OCRConfidence, v.ReviewStatus, nullable(v.ReviewedBy), v.ReviewedAt, v.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) Evidence(ctx context.Context, tenantID, revisionID string) ([]domain.EvidenceSpan, error) {
	out := []domain.EvidenceSpan{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id,tenant_id,project_id,revision_id,locator_kind,locator,quote_text,quote_hash,ocr_confidence,review_status,COALESCE(reviewed_by::text,''),reviewed_at,created_at FROM evidence_spans WHERE tenant_id=$1 AND revision_id=$2 ORDER BY created_at`, tenantID, revisionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.EvidenceSpan
			var locator []byte
			if err := rows.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.RevisionID, &v.LocatorKind, &locator, &v.QuoteText, &v.QuoteHash, &v.OCRConfidence, &v.ReviewStatus, &v.ReviewedBy, &v.ReviewedAt, &v.CreatedAt); err != nil {
				return err
			}
			v.Locator, err = decodeJSON[map[string]any](locator)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) EvidenceSpan(ctx context.Context, tenantID, id string) (domain.EvidenceSpan, error) {
	var result domain.EvidenceSpan
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var locator []byte
		var v domain.EvidenceSpan
		err := tx.QueryRow(ctx, `SELECT id,tenant_id,project_id,revision_id,locator_kind,locator,quote_text,quote_hash,ocr_confidence,review_status,COALESCE(reviewed_by::text,''),reviewed_at,created_at FROM evidence_spans WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.RevisionID, &v.LocatorKind, &locator, &v.QuoteText, &v.QuoteHash, &v.OCRConfidence, &v.ReviewStatus, &v.ReviewedBy, &v.ReviewedAt, &v.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("证据片段")
		}
		if err != nil {
			return err
		}
		v.Locator, err = decodeJSON[map[string]any](locator)
		result = v
		return err
	})
	return result, err
}

func (s *Store) SaveEvidence(ctx context.Context, v domain.EvidenceSpan) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE evidence_spans SET review_status=$3,reviewed_by=$4,reviewed_at=$5 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.ReviewStatus, nullable(v.ReviewedBy), v.ReviewedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("证据片段")
		}
		return nil
	})
}

func (s *Store) CreateKnowledge(ctx context.Context, v domain.KnowledgeItem) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO knowledge_items(id,tenant_id,project_id,kind,title,statement,subject,predicate,typed_value,scope,status,risk_level,allowed_channels,evidence,forbidden_extensions,depends_on_fact_ids,valid_from,valid_until,expires_at,approved_by,approved_at,origin_run_id,decision_required,row_version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)`, v.ID, v.TenantID, v.ProjectID, v.Kind, v.Title, v.Statement, v.Subject, v.Predicate, jsonValue(v.Value), jsonValue(v.Scope), v.Status, v.RiskLevel, jsonValue(v.AllowedChannels), jsonValue(v.Evidence), jsonValue(v.ForbiddenExtensions), jsonValue(v.DependsOnFactIDs), v.ValidFrom, v.ValidUntil, v.ExpiresAt, nullable(v.ApprovedBy), v.ApprovedAt, nullable(v.OriginRunID), v.DecisionRequired, v.RowVersion, v.CreatedAt, v.UpdatedAt)
		return dbError(err)
	})
}

func scanKnowledge(row pgx.Row) (domain.KnowledgeItem, error) {
	var v domain.KnowledgeItem
	var value, scope, channels, evidence, forbidden, dependencies []byte
	err := row.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.Kind, &v.Title, &v.Statement, &v.Subject, &v.Predicate, &value, &scope, &v.Status, &v.RiskLevel, &channels, &evidence, &forbidden, &dependencies, &v.ValidFrom, &v.ValidUntil, &v.ExpiresAt, &v.ApprovedBy, &v.ApprovedAt, &v.OriginRunID, &v.DecisionRequired, &v.RowVersion, &v.CreatedAt, &v.UpdatedAt)
	if err == nil {
		v.Value, err = decodeJSON[domain.TypedValue](value)
	}
	if err == nil {
		v.Scope, err = decodeJSON[domain.KnowledgeScope](scope)
	}
	if err == nil {
		v.AllowedChannels, err = decodeJSON[[]string](channels)
	}
	if err == nil {
		v.Evidence, err = decodeJSON[[]domain.EvidenceRef](evidence)
	}
	if err == nil {
		v.ForbiddenExtensions, err = decodeJSON[[]string](forbidden)
	}
	if err == nil {
		v.DependsOnFactIDs, err = decodeJSON[[]string](dependencies)
	}
	return v, err
}

const knowledgeSelect = `SELECT id,tenant_id,project_id,kind,title,statement,subject,predicate,typed_value,scope,status,risk_level,allowed_channels,evidence,forbidden_extensions,depends_on_fact_ids,valid_from,valid_until,expires_at,COALESCE(approved_by::text,''),approved_at,COALESCE(origin_run_id::text,''),decision_required,row_version,created_at,updated_at FROM knowledge_items`

func (s *Store) Knowledge(ctx context.Context, tenantID, projectID string) ([]domain.KnowledgeItem, error) {
	out := []domain.KnowledgeItem{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, knowledgeSelect+` WHERE tenant_id=$1 AND project_id=$2 ORDER BY created_at`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanKnowledge(rows)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) KnowledgeItem(ctx context.Context, tenantID, id string) (domain.KnowledgeItem, error) {
	var result domain.KnowledgeItem
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanKnowledge(tx.QueryRow(ctx, knowledgeSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("知识项")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveKnowledge(ctx context.Context, v domain.KnowledgeItem) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE knowledge_items SET kind=$3,title=$4,statement=$5,subject=$6,predicate=$7,typed_value=$8,scope=$9,status=$10,risk_level=$11,allowed_channels=$12,evidence=$13,forbidden_extensions=$14,depends_on_fact_ids=$15,valid_from=$16,valid_until=$17,expires_at=$18,approved_by=$19,approved_at=$20,decision_required=$21,row_version=$22,updated_at=$23 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.Kind, v.Title, v.Statement, v.Subject, v.Predicate, jsonValue(v.Value), jsonValue(v.Scope), v.Status, v.RiskLevel, jsonValue(v.AllowedChannels), jsonValue(v.Evidence), jsonValue(v.ForbiddenExtensions), jsonValue(v.DependsOnFactIDs), v.ValidFrom, v.ValidUntil, v.ExpiresAt, nullable(v.ApprovedBy), v.ApprovedAt, v.DecisionRequired, v.RowVersion, v.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("知识项")
		}
		return nil
	})
}
