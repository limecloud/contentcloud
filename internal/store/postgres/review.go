package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateReviewCycle(ctx context.Context, cycle domain.ReviewCycle) (domain.ReviewCycle, error) {
	err := s.withTenant(ctx, cycle.TenantID, func(tx pgx.Tx) error {
		var locked bool
		if cycle.SubjectType != "submission_revision" {
			return domain.Invalid("REVIEW_SUBJECT_TYPE_INVALID", "审核周期不支持该主题类型（subject_type）")
		}
		if err := tx.QueryRow(ctx, `SELECT true FROM submission_revisions WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, cycle.TenantID, cycle.SubjectID).Scan(&locked); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("审核对象")
			}
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(cycle_number),0)+1 FROM review_cycles WHERE tenant_id=$1 AND subject_id=$2`, cycle.TenantID, cycle.SubjectID).Scan(&cycle.CycleNumber); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO review_cycles(id,tenant_id,project_id,subject_type,subject_id,cycle_number,status,conclusion,assignee_user_id,opened_by,decided_by,opened_at,decided_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, cycle.ID, cycle.TenantID, cycle.ProjectID, cycle.SubjectType, cycle.SubjectID, cycle.CycleNumber, cycle.Status, cycle.Conclusion, cycle.AssigneeUserID, cycle.OpenedBy, cycle.DecidedBy, cycle.OpenedAt, cycle.DecidedAt, cycle.CreatedAt)
		return dbError(err)
	})
	return cycle, err
}

func scanReviewCycle(row pgx.Row) (domain.ReviewCycle, error) {
	var cycle domain.ReviewCycle
	err := row.Scan(&cycle.ID, &cycle.TenantID, &cycle.ProjectID, &cycle.SubjectType, &cycle.SubjectID, &cycle.CycleNumber, &cycle.Status, &cycle.Conclusion, &cycle.AssigneeUserID, &cycle.OpenedBy, &cycle.DecidedBy, &cycle.OpenedAt, &cycle.DecidedAt, &cycle.CreatedAt)
	return cycle, err
}

const reviewCycleSelect = `SELECT id,tenant_id,project_id,subject_type,subject_id,cycle_number,status,conclusion,assignee_user_id,opened_by,decided_by,opened_at,decided_at,created_at FROM review_cycles`

func (s *Store) ReviewCycles(ctx context.Context, tenantID, subjectID string) ([]domain.ReviewCycle, error) {
	result := []domain.ReviewCycle{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, reviewCycleSelect+` WHERE tenant_id=$1 AND subject_id=$2 ORDER BY cycle_number DESC`, tenantID, subjectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			cycle, err := scanReviewCycle(rows)
			if err != nil {
				return err
			}
			result = append(result, cycle)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) SaveReviewCycle(ctx context.Context, cycle domain.ReviewCycle) error {
	return s.withTenant(ctx, cycle.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE review_cycles SET status=$3,conclusion=$4,assignee_user_id=$5,decided_by=$6,decided_at=$7 WHERE tenant_id=$1 AND id=$2`, cycle.TenantID, cycle.ID, cycle.Status, cycle.Conclusion, cycle.AssigneeUserID, cycle.DecidedBy, cycle.DecidedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("审核周期")
		}
		return nil
	})
}

func (s *Store) CreateReviewComment(ctx context.Context, v domain.ReviewComment) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO review_comments(id,tenant_id,project_id,review_cycle_id,subject_type,subject_id,carried_from_comment_id,shot_id,json_pointer,body,visibility,author_id,resolved_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, v.ID, v.TenantID, v.ProjectID, nullable(v.ReviewCycleID), v.SubjectType, v.SubjectID, nullable(v.CarriedFromID), v.ShotID, v.JSONPointer, v.Body, v.Visibility, v.AuthorID, v.ResolvedAt, v.CreatedAt)
		return dbError(err)
	})
}

func scanReviewComment(row pgx.Row) (domain.ReviewComment, error) {
	var v domain.ReviewComment
	err := row.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.ReviewCycleID, &v.SubjectType, &v.SubjectID, &v.CarriedFromID, &v.ShotID, &v.JSONPointer, &v.Body, &v.Visibility, &v.AuthorID, &v.ResolvedAt, &v.CreatedAt)
	return v, err
}

const reviewCommentSelect = `SELECT id,tenant_id,project_id,COALESCE(review_cycle_id::text,''),subject_type,subject_id,COALESCE(carried_from_comment_id::text,''),shot_id,json_pointer,body,visibility,author_id,resolved_at,created_at FROM review_comments`

func (s *Store) ReviewComments(ctx context.Context, tenantID, subjectID string) ([]domain.ReviewComment, error) {
	out := []domain.ReviewComment{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := reviewCommentSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if subjectID != "" {
			query += ` AND subject_id=$2`
			args = append(args, subjectID)
		}
		query += ` ORDER BY created_at`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanReviewComment(rows)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}
func (s *Store) ReviewComment(ctx context.Context, tenantID, id string) (domain.ReviewComment, error) {
	var result domain.ReviewComment
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanReviewComment(tx.QueryRow(ctx, reviewCommentSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("审核批注")
		}
		return err
	})
	return result, err
}
func (s *Store) SaveReviewComment(ctx context.Context, v domain.ReviewComment) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE review_comments SET body=$3,visibility=$4,resolved_at=$5 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, v.Body, v.Visibility, v.ResolvedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("审核批注")
		}
		return nil
	})
}

func (s *Store) CreateReviewGrant(ctx context.Context, v domain.ReviewGrant) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO review_grants(id,tenant_id,project_id,subject_type,subject_id,subject_hash,reviewer_email,token_hash,otp_hash,expires_at,verified_at,revoked_at,decision_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, v.ID, v.TenantID, v.ProjectID, v.SubjectType, v.SubjectID, v.SubjectHash, v.ReviewerEmail, v.TokenHash, v.OTPHash, v.ExpiresAt, v.VerifiedAt, v.RevokedAt, v.DecisionAt, v.CreatedAt)
		return dbError(err)
	})
}
func scanReviewGrant(row pgx.Row) (domain.ReviewGrant, error) {
	var v domain.ReviewGrant
	err := row.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.SubjectType, &v.SubjectID, &v.SubjectHash, &v.ReviewerEmail, &v.TokenHash, &v.OTPHash, &v.ExpiresAt, &v.VerifiedAt, &v.RevokedAt, &v.DecisionAt, &v.CreatedAt)
	return v, err
}

const reviewGrantSelect = `SELECT id,tenant_id,project_id,subject_type,subject_id,subject_hash,reviewer_email,token_hash,otp_hash,expires_at,verified_at,revoked_at,decision_at,created_at FROM review_grants`

func (s *Store) ReviewGrant(ctx context.Context, tenantID, id string) (domain.ReviewGrant, error) {
	var result domain.ReviewGrant
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		grant, err := scanReviewGrant(tx.QueryRow(ctx, reviewGrantSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = grant
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("客户审批授权")
		}
		return err
	})
	return result, err
}

func (s *Store) ReviewGrants(ctx context.Context, tenantID, subjectID string) ([]domain.ReviewGrant, error) {
	result := []domain.ReviewGrant{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, reviewGrantSelect+` WHERE tenant_id=$1 AND subject_id=$2 ORDER BY created_at DESC`, tenantID, subjectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			grant, err := scanReviewGrant(rows)
			if err != nil {
				return err
			}
			grant.TokenHash = ""
			grant.OTPHash = ""
			result = append(result, grant)
		}
		return rows.Err()
	})
	return result, err
}
func (s *Store) ReviewGrantByTokenHash(ctx context.Context, hash string) (domain.ReviewGrant, error) {
	var tenantID, grantID string
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id,grant_id FROM contentcloud_lookup_review_token($1)`, hash).Scan(&tenantID, &grantID); err != nil {
		return domain.ReviewGrant{}, domain.NotFound("客户审批授权")
	}
	var result domain.ReviewGrant
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanReviewGrant(tx.QueryRow(ctx, reviewGrantSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, grantID))
		result = v
		return dbError(err)
	})
	return result, err
}
func (s *Store) MarkReviewGrantVerified(ctx context.Context, tenantID, id string, verifiedAt time.Time) error {
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE review_grants SET verified_at=COALESCE(verified_at,$3) WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL AND decision_at IS NULL AND expires_at>$3`, tenantID, id, verifiedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.Conflict("REVIEW_GRANT_STATE_INVALID", "客户审批授权已撤销、已完成或已过期")
		}
		return nil
	})
}

func (s *Store) RevokeReviewGrant(ctx context.Context, tenantID, id string, revokedAt time.Time) error {
	return s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE review_grants SET revoked_at=COALESCE(revoked_at,$3) WHERE tenant_id=$1 AND id=$2 AND decision_at IS NULL`, tenantID, id, revokedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.Conflict("REVIEW_GRANT_STATE_INVALID", "客户审批授权不存在或已完成")
		}
		return nil
	})
}

func (s *Store) CreateArtifact(ctx context.Context, v domain.Artifact) error {
	return s.withTenant(ctx, v.TenantID, func(tx pgx.Tx) error {
		return insertArtifact(ctx, tx, v)
	})
}

func insertArtifact(ctx context.Context, tx pgx.Tx, value domain.Artifact) error {
	if value.ApprovedSnapshotID == "" {
		return domain.Invalid("ARTIFACT_SNAPSHOT_REQUIRED", "成果文件必须绑定批准快照")
	}
	var snapshotExists bool
	if err := tx.QueryRow(ctx, `SELECT true FROM approved_snapshots WHERE tenant_id=$1 AND project_id=$2 AND id=$3`, value.TenantID, value.ProjectID, value.ApprovedSnapshotID).Scan(&snapshotExists); errors.Is(err, pgx.ErrNoRows) {
		return domain.NotFound("批准快照")
	} else if err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO artifacts(id,tenant_id,project_id,approved_snapshot_id,kind,capability_id,capability_version,capability_digest,schema_id,media_type,file_name,sha256,byte_size,object_key,visibility,retention_class,purpose,metadata,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, value.ID, value.TenantID, value.ProjectID, value.ApprovedSnapshotID, value.Kind, value.CapabilityID, value.CapabilityVersion, value.CapabilityDigest, value.SchemaID, value.MediaType, value.FileName, value.SHA256, value.ByteSize, value.ObjectKey, value.Visibility, value.RetentionClass, value.Purpose, jsonValue(value.Metadata), value.CreatedAt)
	return dbError(err)
}
func scanArtifact(row pgx.Row) (domain.Artifact, error) {
	var v domain.Artifact
	var metadata []byte
	err := row.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.ApprovedSnapshotID, &v.Kind, &v.CapabilityID, &v.CapabilityVersion, &v.CapabilityDigest, &v.SchemaID, &v.MediaType, &v.FileName, &v.SHA256, &v.ByteSize, &v.ObjectKey, &v.Visibility, &v.RetentionClass, &v.Purpose, &metadata, &v.CreatedAt)
	if err == nil {
		v.Metadata, err = decodeJSON[map[string]any](metadata)
	}
	return v, err
}

const artifactSelect = `SELECT id,tenant_id,project_id,approved_snapshot_id,kind,capability_id,capability_version,capability_digest,schema_id,media_type,file_name,sha256,byte_size,object_key,visibility,retention_class,purpose,metadata,created_at FROM artifacts`

func (s *Store) ArtifactsByApprovedSnapshot(ctx context.Context, tenantID, snapshotID string) ([]domain.Artifact, error) {
	out := []domain.Artifact{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, artifactSelect+` WHERE tenant_id=$1 AND approved_snapshot_id=$2 ORDER BY created_at DESC`, tenantID, snapshotID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanArtifact(rows)
			if err != nil {
				return err
			}
			out = append(out, value)
		}
		return rows.Err()
	})
	return out, err
}
func (s *Store) Artifact(ctx context.Context, tenantID, id string) (domain.Artifact, error) {
	var result domain.Artifact
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		v, err := scanArtifact(tx.QueryRow(ctx, artifactSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = v
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("产物")
		}
		return err
	})
	return result, err
}
