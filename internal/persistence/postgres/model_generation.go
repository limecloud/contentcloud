package postgres

import (
	"context"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/jackc/pgx/v5"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
)

func (s *Store) CreateModelGenerationReceipt(ctx context.Context, value deliverydomain.ModelGenerationReceipt) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO model_generation_receipts(tenant_id,id,project_id,task_id,task_revision_id,provider_id,provider,model,request_id,request_digest,response_digest,input_tokens,output_tokens,total_tokens,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, value.TenantID, value.ID, value.ProjectID, value.TaskID, value.TaskRevisionID, value.ProviderID, value.Provider, value.Model, value.RequestID, value.RequestDigest, value.ResponseDigest, value.InputTokens, value.OutputTokens, value.TotalTokens, value.CreatedBy, value.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) CreateModelGeneratedRevision(ctx context.Context, revision reviewdomain.TaskRevision, receipt deliverydomain.ModelGenerationReceipt) error {
	revision.NormalizeCollections()
	if err := revision.Validate(); err != nil {
		return err
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	if receipt.TaskRevisionID != revision.ID || receipt.TaskID != revision.TaskID || receipt.TenantID != revision.TenantID {
		return fault.Invalid("MODEL_GENERATION_SCOPE_MISMATCH", "模型回执与候选修订作用域不一致")
	}
	return s.withTenant(ctx, revision.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO task_revisions(tenant_id,id,project_id,task_id,revision_no,content_type,schema_version,content,content_hash,sop_digest,knowledge_snapshot_ids,evidence_summary,rights_summary,status,submitted_by,submitted_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, revision.TenantID, revision.ID, revision.ProjectID, revision.TaskID, revision.RevisionNo, revision.ContentType, revision.SchemaVersion, revision.Content, revision.ContentHash, revision.SOPDigest, jsonArrayValue(revision.KnowledgeSnapshotIDs), jsonValue(revision.EvidenceSummary), jsonValue(revision.RightsSummary), revision.Status, revision.SubmittedBy, revision.SubmittedAt, revision.CreatedAt); err != nil {
			return dbError(err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO model_generation_receipts(tenant_id,id,project_id,task_id,task_revision_id,provider_id,provider,model,request_id,request_digest,response_digest,input_tokens,output_tokens,total_tokens,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, receipt.TenantID, receipt.ID, receipt.ProjectID, receipt.TaskID, receipt.TaskRevisionID, receipt.ProviderID, receipt.Provider, receipt.Model, receipt.RequestID, receipt.RequestDigest, receipt.ResponseDigest, receipt.InputTokens, receipt.OutputTokens, receipt.TotalTokens, receipt.CreatedBy, receipt.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) ModelGenerationReceipts(ctx context.Context, tenantID, taskID string) ([]deliverydomain.ModelGenerationReceipt, error) {
	values := []deliverydomain.ModelGenerationReceipt{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT tenant_id,id,project_id,task_id,task_revision_id,provider_id,provider,model,request_id,request_digest,response_digest,input_tokens,output_tokens,total_tokens,created_by,created_at FROM model_generation_receipts WHERE tenant_id=$1 AND ($2='' OR task_id=$2) ORDER BY created_at`, tenantID, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value deliverydomain.ModelGenerationReceipt
			if err := rows.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.TaskID, &value.TaskRevisionID, &value.ProviderID, &value.Provider, &value.Model, &value.RequestID, &value.RequestDigest, &value.ResponseDigest, &value.InputTokens, &value.OutputTokens, &value.TotalTokens, &value.CreatedBy, &value.CreatedAt); err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}
