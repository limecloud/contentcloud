package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/jackc/pgx/v5"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func (s *Store) CreateWorkspaceBinding(ctx context.Context, value workspacedomain.WorkspaceBinding) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO workspace_bindings(id,tenant_id,project_id,device_id,owner_user_id,template_id,template_version,targets,credential_hash,status,initialized_at,last_seen_at,revoked_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, value.ID, value.TenantID, value.ProjectID, nullable(value.DeviceID), value.OwnerUserID, value.TemplateID, value.TemplateVersion, jsonArrayValue(value.Targets), value.CredentialHash, value.Status, value.InitializedAt, value.LastSeenAt, value.RevokedAt)
		return dbError(err)
	})
}

func scanWorkspaceBinding(row pgx.Row) (workspacedomain.WorkspaceBinding, error) {
	var value workspacedomain.WorkspaceBinding
	var targets []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.DeviceID, &value.OwnerUserID, &value.TemplateID, &value.TemplateVersion, &targets, &value.CredentialHash, &value.Status, &value.InitializedAt, &value.LastSeenAt, &value.RevokedAt)
	if err == nil {
		err = json.Unmarshal(targets, &value.Targets)
	}
	return value, err
}

const workspaceBindingSelect = `SELECT id,tenant_id,project_id,COALESCE(device_id::text,''),owner_user_id,template_id,template_version,targets,credential_hash,status,initialized_at,last_seen_at,revoked_at FROM workspace_bindings`

func (s *Store) WorkspaceBindingByTokenHash(ctx context.Context, hash string) (workspacedomain.WorkspaceBinding, error) {
	var tenantID, workspaceID string
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id,workspace_id FROM contentcloud_lookup_workspace_token($1)`, hash).Scan(&tenantID, &workspaceID); err != nil {
		return workspacedomain.WorkspaceBinding{}, fault.NotFound("工作区凭据")
	}
	var result workspacedomain.WorkspaceBinding
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanWorkspaceBinding(tx.QueryRow(ctx, workspaceBindingSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, workspaceID))
		result = value
		return dbError(err)
	})
	return result, err
}

func (s *Store) WorkspaceBinding(ctx context.Context, tenantID, id string) (workspacedomain.WorkspaceBinding, error) {
	var result workspacedomain.WorkspaceBinding
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanWorkspaceBinding(tx.QueryRow(ctx, workspaceBindingSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("工作区绑定")
		}
		return err
	})
	result.CredentialHash = ""
	return result, err
}

func (s *Store) SaveWorkspaceBinding(ctx context.Context, value workspacedomain.WorkspaceBinding) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE workspace_bindings SET template_id=$3,template_version=$4,targets=$5,status=$6,last_seen_at=$7,revoked_at=$8 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.TemplateID, value.TemplateVersion, jsonArrayValue(value.Targets), value.Status, value.LastSeenAt, value.RevokedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.NotFound("工作区绑定")
		}
		return nil
	})
}

func (s *Store) CreateSubmissionRevision(ctx context.Context, submission reviewdomain.Submission, revision reviewdomain.SubmissionRevision, disclosures []reviewdomain.SourceDisclosure, cycle reviewdomain.ReviewCycle) error {
	return s.withTenant(ctx, submission.TenantID, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM submissions WHERE tenant_id=$1 AND id=$2)`, submission.TenantID, submission.ID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec(ctx, `INSERT INTO submissions(id,tenant_id,project_id,workspace_id,submission_type,status,current_revision_id,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,NULL,$7,$8,$9)`, submission.ID, submission.TenantID, submission.ProjectID, submission.WorkspaceID, submission.SubmissionType, "preparing", submission.CreatedBy, submission.CreatedAt, submission.UpdatedAt); err != nil {
				return dbError(err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO submission_revisions(id,tenant_id,project_id,workspace_id,submission_id,revision_no,schema_version,content_hash,base_snapshot_ids,environment_digest,local_run_summary,objects,artifacts,message,idempotency_key,evidence_limited,created_by,created_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, revision.ID, revision.TenantID, revision.ProjectID, revision.WorkspaceID, revision.SubmissionID, revision.RevisionNo, revision.SchemaVersion, revision.ContentHash, jsonArrayValue(revision.BaseSnapshotIDs), revision.EnvironmentDigest, jsonValue(revision.LocalRunSummary), jsonArrayValue(revision.Objects), jsonArrayValue(revision.Artifacts), revision.Message, revision.IdempotencyKey, revision.EvidenceLimited, revision.CreatedBy, revision.CreatedAt); err != nil {
			return dbError(err)
		}
		for _, disclosure := range disclosures {
			if _, err := tx.Exec(ctx, `INSERT INTO source_disclosures(id,tenant_id,project_id,submission_revision_id,source_ref,disclosure_level,sha256,byte_size,evidence_pack,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, disclosure.ID, revision.TenantID, revision.ProjectID, revision.ID, disclosure.SourceRef, disclosure.Level, disclosure.SHA256, disclosure.ByteSize, nullableJSON(disclosure.EvidencePack), disclosure.CreatedAt); err != nil {
				return dbError(err)
			}
		}
		cycle.CycleNumber = 1
		if _, err := tx.Exec(ctx, `INSERT INTO review_cycles(id,tenant_id,project_id,subject_type,subject_id,cycle_number,status,conclusion,assignee_user_id,opened_by,decided_by,opened_at,decided_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, cycle.ID, cycle.TenantID, cycle.ProjectID, cycle.SubjectType, cycle.SubjectID, cycle.CycleNumber, cycle.Status, cycle.Conclusion, cycle.AssigneeUserID, cycle.OpenedBy, cycle.DecidedBy, cycle.OpenedAt, cycle.DecidedAt, cycle.CreatedAt); err != nil {
			return dbError(err)
		}
		result, err := tx.Exec(ctx, `UPDATE submissions SET status=$3,current_revision_id=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2`, submission.TenantID, submission.ID, submission.Status, submission.CurrentRevisionID, submission.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.NotFound("提交")
		}
		if _, err := tx.Exec(ctx, `UPDATE review_grants SET revoked_at=$3
				WHERE tenant_id=$1 AND subject_type='submission_revision' AND revoked_at IS NULL AND decision_at IS NULL
				AND subject_id IN (SELECT id FROM submission_revisions WHERE tenant_id=$1 AND submission_id=$2 AND id<>$4)`, submission.TenantID, submission.ID, revision.CreatedAt, revision.ID); err != nil {
			return dbError(err)
		}
		return nil
	})
}

func scanSubmission(row pgx.Row) (reviewdomain.Submission, error) {
	var value reviewdomain.Submission
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.WorkspaceID, &value.SubmissionType, &value.Status, &value.CurrentRevisionID, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

const submissionSelect = `SELECT id,tenant_id,project_id,workspace_id,submission_type,status,COALESCE(current_revision_id::text,''),created_by,created_at,updated_at FROM submissions`

func (s *Store) SubmissionByWorkspaceType(ctx context.Context, tenantID, projectID, workspaceID, submissionType string) (reviewdomain.Submission, error) {
	var result reviewdomain.Submission
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanSubmission(tx.QueryRow(ctx, submissionSelect+` WHERE tenant_id=$1 AND project_id=$2 AND workspace_id=$3 AND submission_type=$4`, tenantID, projectID, workspaceID, submissionType))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("提交")
		}
		return err
	})
	return result, err
}

func (s *Store) Submissions(ctx context.Context, tenantID, projectID string) ([]reviewdomain.Submission, error) {
	values := []reviewdomain.Submission{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := submissionSelect + ` WHERE tenant_id=$1`
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
			value, err := scanSubmission(rows)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}

func (s *Store) Submission(ctx context.Context, tenantID, id string) (reviewdomain.Submission, error) {
	var result reviewdomain.Submission
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanSubmission(tx.QueryRow(ctx, submissionSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("提交")
		}
		return err
	})
	return result, err
}

func scanSubmissionRevision(row pgx.Row) (reviewdomain.SubmissionRevision, error) {
	var value reviewdomain.SubmissionRevision
	var baseSnapshots, localRun, objects, artifacts []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.WorkspaceID, &value.SubmissionID, &value.RevisionNo, &value.SchemaVersion, &value.ContentHash, &baseSnapshots, &value.EnvironmentDigest, &localRun, &objects, &artifacts, &value.Message, &value.IdempotencyKey, &value.EvidenceLimited, &value.CreatedBy, &value.CreatedAt)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(baseSnapshots, &value.BaseSnapshotIDs); err != nil {
		return value, err
	}
	if err := json.Unmarshal(objects, &value.Objects); err != nil {
		return value, err
	}
	if err := json.Unmarshal(localRun, &value.LocalRunSummary); err != nil {
		return value, err
	}
	if err := json.Unmarshal(artifacts, &value.Artifacts); err != nil {
		return value, err
	}
	return value, nil
}

const submissionRevisionSelect = `SELECT id,tenant_id,project_id,workspace_id,submission_id,revision_no,schema_version,content_hash,base_snapshot_ids,environment_digest,local_run_summary,objects,artifacts,message,idempotency_key,evidence_limited,created_by,created_at FROM submission_revisions`

func (s *Store) SubmissionRevision(ctx context.Context, tenantID, id string) (reviewdomain.SubmissionRevision, error) {
	var result reviewdomain.SubmissionRevision
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanSubmissionRevision(tx.QueryRow(ctx, submissionRevisionSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("提交修订版本")
		}
		if err != nil {
			return err
		}
		value.SourceDisclosures, err = sourceDisclosures(ctx, tx, tenantID, id)
		result = value
		return err
	})
	return result, err
}

func (s *Store) SubmissionRevisions(ctx context.Context, tenantID, submissionID string) ([]reviewdomain.SubmissionRevision, error) {
	values := []reviewdomain.SubmissionRevision{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, submissionRevisionSelect+` WHERE tenant_id=$1 AND submission_id=$2 ORDER BY revision_no DESC`, tenantID, submissionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanSubmissionRevision(rows)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		for index := range values {
			values[index].SourceDisclosures, err = sourceDisclosures(ctx, tx, tenantID, values[index].ID)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return values, err
}

func sourceDisclosures(ctx context.Context, tx pgx.Tx, tenantID, revisionID string) ([]reviewdomain.SourceDisclosure, error) {
	rows, err := tx.Query(ctx, `SELECT id,tenant_id,project_id,submission_revision_id,source_ref,disclosure_level,sha256,byte_size,COALESCE(evidence_pack,'null'::jsonb),created_at FROM source_disclosures WHERE tenant_id=$1 AND submission_revision_id=$2 ORDER BY source_ref`, tenantID, revisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []reviewdomain.SourceDisclosure{}
	for rows.Next() {
		var value reviewdomain.SourceDisclosure
		var evidence []byte
		if err := rows.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.SubmissionRevisionID, &value.SourceRef, &value.Level, &value.SHA256, &value.ByteSize, &evidence, &value.CreatedAt); err != nil {
			return nil, err
		}
		if string(evidence) != "null" {
			value.EvidencePack = append(json.RawMessage(nil), evidence...)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func scanApprovedSnapshot(row pgx.Row) (reviewdomain.ApprovedSnapshot, error) {
	var value reviewdomain.ApprovedSnapshot
	var canonical, eligible, artifacts []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.WorkspaceID, &value.SubmissionID, &value.SubmissionRevisionID, &value.SubmissionType, &value.SchemaVersion, &value.ContentHash, &value.SubjectHash, &canonical, &eligible, &artifacts, &value.DecisionID, &value.CreatedBy, &value.CreatedAt)
	if err != nil {
		return value, err
	}
	value.CanonicalContent = append(json.RawMessage(nil), canonical...)
	if err := json.Unmarshal(eligible, &value.EligibleIDs); err != nil {
		return value, err
	}
	if err := json.Unmarshal(artifacts, &value.Artifacts); err != nil {
		return value, err
	}
	return value, nil
}

const approvedSnapshotSelect = `SELECT id,tenant_id,project_id,workspace_id,submission_id,submission_revision_id,submission_type,schema_version,content_hash,subject_hash,canonical_content,eligible_ids,artifacts,decision_id,created_by,created_at FROM approved_snapshots`

func (s *Store) ApprovedSnapshots(ctx context.Context, tenantID, projectID, submissionType string) ([]reviewdomain.ApprovedSnapshot, error) {
	values := []reviewdomain.ApprovedSnapshot{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		query := approvedSnapshotSelect + ` WHERE tenant_id=$1`
		args := []any{tenantID}
		if projectID != "" {
			query += fmt.Sprintf(` AND project_id=$%d`, len(args)+1)
			args = append(args, projectID)
		}
		if submissionType != "" {
			query += fmt.Sprintf(` AND submission_type=$%d`, len(args)+1)
			args = append(args, submissionType)
		}
		query += ` ORDER BY created_at DESC`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanApprovedSnapshot(rows)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}

func (s *Store) ApprovedSnapshot(ctx context.Context, tenantID, id string) (reviewdomain.ApprovedSnapshot, error) {
	var result reviewdomain.ApprovedSnapshot
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanApprovedSnapshot(tx.QueryRow(ctx, approvedSnapshotSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("批准快照")
		}
		return err
	})
	return result, err
}

func (s *Store) ApproveSubmissionRevision(ctx context.Context, submission reviewdomain.Submission, snapshot reviewdomain.ApprovedSnapshot, decision reviewdomain.ApprovalDecision) error {
	return s.withTenant(ctx, submission.TenantID, func(tx pgx.Tx) error {
		if err := insertApprovalDecision(ctx, tx, decision); err != nil {
			return err
		}
		if err := insertApprovedSnapshot(ctx, tx, snapshot); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE submissions SET status=$3,current_revision_id=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2 AND current_revision_id=$4 AND status=$6`, submission.TenantID, submission.ID, submission.Status, submission.CurrentRevisionID, submission.UpdatedAt, decision.PreviousState)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.Conflict("SUBMISSION_STATE_INVALID", "提交版本的当前版本号或状态已变化")
		}
		if _, err := tx.Exec(ctx, `UPDATE review_grants SET revoked_at=$3 WHERE tenant_id=$1 AND subject_type='submission_revision' AND subject_id=$2 AND revoked_at IS NULL AND decision_at IS NULL`, submission.TenantID, submission.CurrentRevisionID, decision.CreatedAt); err != nil {
			return dbError(err)
		}
		return nil
	})
}

func (s *Store) RecordSubmissionApproval(ctx context.Context, submission reviewdomain.Submission, decision reviewdomain.ApprovalDecision) error {
	return s.withTenant(ctx, submission.TenantID, func(tx pgx.Tx) error {
		if err := insertApprovalDecision(ctx, tx, decision); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE submissions SET status=$3,current_revision_id=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2 AND current_revision_id=$4 AND status=$6`, submission.TenantID, submission.ID, submission.Status, submission.CurrentRevisionID, submission.UpdatedAt, decision.PreviousState)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.Conflict("SUBMISSION_STATE_INVALID", "提交版本的当前版本号或状态已变化")
		}
		return nil
	})
}

func (s *Store) RequestSubmissionChanges(ctx context.Context, submission reviewdomain.Submission, decision reviewdomain.ApprovalDecision, comment reviewdomain.ReviewComment) error {
	return s.withTenant(ctx, submission.TenantID, func(tx pgx.Tx) error {
		if err := insertApprovalDecision(ctx, tx, decision); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO review_comments(id,tenant_id,project_id,review_cycle_id,subject_type,subject_id,carried_from_comment_id,shot_id,json_pointer,body,visibility,author_id,resolved_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, comment.ID, comment.TenantID, comment.ProjectID, comment.ReviewCycleID, comment.SubjectType, comment.SubjectID, nullable(comment.CarriedFromID), comment.ShotID, comment.JSONPointer, comment.Body, comment.Visibility, comment.AuthorID, comment.ResolvedAt, comment.CreatedAt); err != nil {
			return dbError(err)
		}
		result, err := tx.Exec(ctx, `UPDATE submissions SET status=$3,current_revision_id=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2 AND current_revision_id=$4 AND status=$6`, submission.TenantID, submission.ID, submission.Status, submission.CurrentRevisionID, submission.UpdatedAt, decision.PreviousState)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.Conflict("SUBMISSION_STATE_INVALID", "提交版本的当前版本号或状态已变化")
		}
		if _, err := tx.Exec(ctx, `UPDATE review_grants SET revoked_at=$3 WHERE tenant_id=$1 AND subject_type='submission_revision' AND subject_id=$2 AND revoked_at IS NULL AND decision_at IS NULL`, submission.TenantID, submission.CurrentRevisionID, decision.CreatedAt); err != nil {
			return dbError(err)
		}
		return nil
	})
}

func (s *Store) RejectSubmission(ctx context.Context, submission reviewdomain.Submission, decision reviewdomain.ApprovalDecision, comment reviewdomain.ReviewComment) error {
	return s.withTenant(ctx, submission.TenantID, func(tx pgx.Tx) error {
		if err := insertApprovalDecision(ctx, tx, decision); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO review_comments(id,tenant_id,project_id,review_cycle_id,subject_type,subject_id,carried_from_comment_id,shot_id,json_pointer,body,visibility,author_id,resolved_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, comment.ID, comment.TenantID, comment.ProjectID, comment.ReviewCycleID, comment.SubjectType, comment.SubjectID, nullable(comment.CarriedFromID), comment.ShotID, comment.JSONPointer, comment.Body, comment.Visibility, comment.AuthorID, comment.ResolvedAt, comment.CreatedAt); err != nil {
			return dbError(err)
		}
		result, err := tx.Exec(ctx, `UPDATE submissions SET status=$3,current_revision_id=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2 AND current_revision_id=$4 AND status=$6`, submission.TenantID, submission.ID, submission.Status, submission.CurrentRevisionID, submission.UpdatedAt, decision.PreviousState)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.Conflict("SUBMISSION_STATE_INVALID", "提交版本的当前版本号或状态已变化")
		}
		if _, err := tx.Exec(ctx, `UPDATE review_grants SET revoked_at=$3 WHERE tenant_id=$1 AND subject_type='submission_revision' AND subject_id=$2 AND revoked_at IS NULL AND decision_at IS NULL`, submission.TenantID, submission.CurrentRevisionID, decision.CreatedAt); err != nil {
			return dbError(err)
		}
		return nil
	})
}

func (s *Store) CreateSubmissionReviewGrant(ctx context.Context, submission reviewdomain.Submission, grant reviewdomain.ReviewGrant) error {
	return s.withTenant(ctx, submission.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO review_grants(id,tenant_id,project_id,subject_type,subject_id,subject_hash,reviewer_email,token_hash,otp_hash,expires_at,verified_at,revoked_at,decision_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, grant.ID, grant.TenantID, grant.ProjectID, grant.SubjectType, grant.SubjectID, grant.SubjectHash, grant.ReviewerEmail, grant.TokenHash, grant.OTPHash, grant.ExpiresAt, grant.VerifiedAt, grant.RevokedAt, grant.DecisionAt, grant.CreatedAt); err != nil {
			return dbError(err)
		}
		result, err := tx.Exec(ctx, `UPDATE submissions SET status=$3,updated_at=$4 WHERE tenant_id=$1 AND id=$2 AND current_revision_id=$5 AND status IN ('internally_approved','client_review')`, submission.TenantID, submission.ID, submission.Status, submission.UpdatedAt, submission.CurrentRevisionID)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.Conflict("SUBMISSION_STATE_INVALID", "提交版本的当前版本号或状态已变化")
		}
		return nil
	})
}

func (s *Store) CompleteSubmissionClientReview(ctx context.Context, submission reviewdomain.Submission, grant reviewdomain.ReviewGrant, decision reviewdomain.ApprovalDecision, comment *reviewdomain.ReviewComment, snapshot *reviewdomain.ApprovedSnapshot) error {
	return s.withTenant(ctx, submission.TenantID, func(tx pgx.Tx) error {
		var revokedAt, decisionAt *time.Time
		var expiresAt time.Time
		if err := tx.QueryRow(ctx, `SELECT revoked_at,decision_at,expires_at FROM review_grants WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, grant.TenantID, grant.ID).Scan(&revokedAt, &decisionAt, &expiresAt); err != nil {
			return dbError(err)
		}
		if revokedAt != nil || decisionAt != nil || !grant.DecisionAt.Before(expiresAt) {
			return fault.Conflict("REVIEW_ALREADY_DECIDED", "该审批链接已失效或已完成最终决策")
		}
		if err := insertApprovalDecision(ctx, tx, decision); err != nil {
			return err
		}
		if comment != nil {
			if _, err := tx.Exec(ctx, `INSERT INTO review_comments(id,tenant_id,project_id,review_cycle_id,subject_type,subject_id,carried_from_comment_id,shot_id,json_pointer,body,visibility,author_id,resolved_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, comment.ID, comment.TenantID, comment.ProjectID, nullable(comment.ReviewCycleID), comment.SubjectType, comment.SubjectID, nullable(comment.CarriedFromID), comment.ShotID, comment.JSONPointer, comment.Body, comment.Visibility, comment.AuthorID, comment.ResolvedAt, comment.CreatedAt); err != nil {
				return dbError(err)
			}
		}
		if snapshot != nil {
			if err := insertApprovedSnapshot(ctx, tx, *snapshot); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE review_grants SET verified_at=$3,decision_at=$4 WHERE tenant_id=$1 AND id=$2`, grant.TenantID, grant.ID, grant.VerifiedAt, grant.DecisionAt); err != nil {
			return dbError(err)
		}
		result, err := tx.Exec(ctx, `UPDATE submissions SET status=$3,updated_at=$4 WHERE tenant_id=$1 AND id=$2 AND current_revision_id=$5 AND status='client_review'`, submission.TenantID, submission.ID, submission.Status, submission.UpdatedAt, submission.CurrentRevisionID)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return fault.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效或状态已变化")
		}
		return nil
	})
}

func insertApprovalDecision(ctx context.Context, tx pgx.Tx, decision reviewdomain.ApprovalDecision) error {
	_, err := tx.Exec(ctx, `INSERT INTO approval_decisions(id,tenant_id,project_id,subject_type,subject_id,subject_hash,decision_stage,actor_id,decision,reason,previous_state,resulting_state,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, decision.ID, decision.TenantID, decision.ProjectID, decision.SubjectType, decision.SubjectID, decision.SubjectHash, defaultDecisionStage(decision.DecisionStage), decision.ActorID, decision.Decision, decision.Reason, decision.PreviousState, decision.ResultingState, decision.CreatedAt)
	return dbError(err)
}

func insertApprovedSnapshot(ctx context.Context, tx pgx.Tx, snapshot reviewdomain.ApprovedSnapshot) error {
	_, err := tx.Exec(ctx, `INSERT INTO approved_snapshots(id,tenant_id,project_id,workspace_id,submission_id,submission_revision_id,submission_type,schema_version,content_hash,subject_hash,canonical_content,eligible_ids,artifacts,decision_id,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, snapshot.ID, snapshot.TenantID, snapshot.ProjectID, snapshot.WorkspaceID, snapshot.SubmissionID, snapshot.SubmissionRevisionID, snapshot.SubmissionType, snapshot.SchemaVersion, snapshot.ContentHash, snapshot.SubjectHash, []byte(snapshot.CanonicalContent), jsonArrayValue(snapshot.EligibleIDs), jsonArrayValue(snapshot.Artifacts), snapshot.DecisionID, snapshot.CreatedBy, snapshot.CreatedAt)
	return dbError(err)
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
}
