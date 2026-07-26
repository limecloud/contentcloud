package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateWorkspaceBinding(ctx context.Context, value domain.WorkspaceBinding) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO workspace_bindings(id,tenant_id,project_id,device_id,owner_user_id,template_id,template_version,targets,credential_hash,status,initialized_at,last_seen_at,revoked_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, value.ID, value.TenantID, value.ProjectID, nullable(value.DeviceID), value.OwnerUserID, value.TemplateID, value.TemplateVersion, jsonValue(value.Targets), value.CredentialHash, value.Status, value.InitializedAt, value.LastSeenAt, value.RevokedAt)
		return dbError(err)
	})
}

func scanWorkspaceBinding(row pgx.Row) (domain.WorkspaceBinding, error) {
	var value domain.WorkspaceBinding
	var targets []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.DeviceID, &value.OwnerUserID, &value.TemplateID, &value.TemplateVersion, &targets, &value.CredentialHash, &value.Status, &value.InitializedAt, &value.LastSeenAt, &value.RevokedAt)
	if err == nil {
		err = json.Unmarshal(targets, &value.Targets)
	}
	return value, err
}

const workspaceBindingSelect = `SELECT id,tenant_id,project_id,COALESCE(device_id::text,''),owner_user_id,template_id,template_version,targets,credential_hash,status,initialized_at,last_seen_at,revoked_at FROM workspace_bindings`

func (s *Store) WorkspaceBindingByTokenHash(ctx context.Context, hash string) (domain.WorkspaceBinding, error) {
	var tenantID, workspaceID string
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id,workspace_id FROM contentcloud_lookup_workspace_token($1)`, hash).Scan(&tenantID, &workspaceID); err != nil {
		return domain.WorkspaceBinding{}, domain.NotFound("工作区凭据")
	}
	var result domain.WorkspaceBinding
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanWorkspaceBinding(tx.QueryRow(ctx, workspaceBindingSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, workspaceID))
		result = value
		return dbError(err)
	})
	return result, err
}

func (s *Store) WorkspaceBinding(ctx context.Context, tenantID, id string) (domain.WorkspaceBinding, error) {
	var result domain.WorkspaceBinding
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanWorkspaceBinding(tx.QueryRow(ctx, workspaceBindingSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("工作区绑定")
		}
		return err
	})
	result.CredentialHash = ""
	return result, err
}

func (s *Store) SaveWorkspaceBinding(ctx context.Context, value domain.WorkspaceBinding) error {
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE workspace_bindings SET template_id=$3,template_version=$4,targets=$5,status=$6,last_seen_at=$7,revoked_at=$8 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.TemplateID, value.TemplateVersion, jsonValue(value.Targets), value.Status, value.LastSeenAt, value.RevokedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("工作区绑定")
		}
		return nil
	})
}

func (s *Store) CreateSubmissionRevision(ctx context.Context, submission domain.Submission, revision domain.SubmissionRevision, disclosures []domain.SourceDisclosure, cycle domain.ReviewCycle) error {
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
		if _, err := tx.Exec(ctx, `INSERT INTO submission_revisions(id,tenant_id,project_id,workspace_id,submission_id,revision_no,schema_version,content_hash,base_approved_snapshot_id,local_run_summary,objects,artifacts,message,idempotency_key,evidence_limited,created_by,created_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, revision.ID, revision.TenantID, revision.ProjectID, revision.WorkspaceID, revision.SubmissionID, revision.RevisionNo, revision.SchemaVersion, revision.ContentHash, nullable(revision.BaseApprovedSnapshotID), jsonValue(revision.LocalRunSummary), []byte(revision.Objects), jsonValue(revision.Artifacts), revision.Message, revision.IdempotencyKey, revision.EvidenceLimited, revision.CreatedBy, revision.CreatedAt); err != nil {
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
			return domain.NotFound("Submission")
		}
		return nil
	})
}

func scanSubmission(row pgx.Row) (domain.Submission, error) {
	var value domain.Submission
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.WorkspaceID, &value.SubmissionType, &value.Status, &value.CurrentRevisionID, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

const submissionSelect = `SELECT id,tenant_id,project_id,workspace_id,submission_type,status,COALESCE(current_revision_id::text,''),created_by,created_at,updated_at FROM submissions`

func (s *Store) SubmissionByWorkspaceType(ctx context.Context, tenantID, projectID, workspaceID, submissionType string) (domain.Submission, error) {
	var result domain.Submission
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanSubmission(tx.QueryRow(ctx, submissionSelect+` WHERE tenant_id=$1 AND project_id=$2 AND workspace_id=$3 AND submission_type=$4`, tenantID, projectID, workspaceID, submissionType))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Submission")
		}
		return err
	})
	return result, err
}

func (s *Store) Submissions(ctx context.Context, tenantID, projectID string) ([]domain.Submission, error) {
	values := []domain.Submission{}
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

func (s *Store) Submission(ctx context.Context, tenantID, id string) (domain.Submission, error) {
	var result domain.Submission
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanSubmission(tx.QueryRow(ctx, submissionSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Submission")
		}
		return err
	})
	return result, err
}

func scanSubmissionRevision(row pgx.Row) (domain.SubmissionRevision, error) {
	var value domain.SubmissionRevision
	var localRun, objects, artifacts []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.WorkspaceID, &value.SubmissionID, &value.RevisionNo, &value.SchemaVersion, &value.ContentHash, &value.BaseApprovedSnapshotID, &localRun, &objects, &artifacts, &value.Message, &value.IdempotencyKey, &value.EvidenceLimited, &value.CreatedBy, &value.CreatedAt)
	if err != nil {
		return value, err
	}
	value.Objects = append(json.RawMessage(nil), objects...)
	if err := json.Unmarshal(localRun, &value.LocalRunSummary); err != nil {
		return value, err
	}
	if err := json.Unmarshal(artifacts, &value.Artifacts); err != nil {
		return value, err
	}
	return value, nil
}

const submissionRevisionSelect = `SELECT id,tenant_id,project_id,workspace_id,submission_id,revision_no,schema_version,content_hash,COALESCE(base_approved_snapshot_id::text,''),local_run_summary,objects,artifacts,message,idempotency_key,evidence_limited,created_by,created_at FROM submission_revisions`

func (s *Store) SubmissionRevision(ctx context.Context, tenantID, id string) (domain.SubmissionRevision, error) {
	var result domain.SubmissionRevision
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanSubmissionRevision(tx.QueryRow(ctx, submissionRevisionSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("SubmissionRevision")
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

func (s *Store) SubmissionRevisions(ctx context.Context, tenantID, submissionID string) ([]domain.SubmissionRevision, error) {
	values := []domain.SubmissionRevision{}
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

func sourceDisclosures(ctx context.Context, tx pgx.Tx, tenantID, revisionID string) ([]domain.SourceDisclosure, error) {
	rows, err := tx.Query(ctx, `SELECT id,tenant_id,project_id,submission_revision_id,source_ref,disclosure_level,sha256,byte_size,COALESCE(evidence_pack,'null'::jsonb),created_at FROM source_disclosures WHERE tenant_id=$1 AND submission_revision_id=$2 ORDER BY source_ref`, tenantID, revisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []domain.SourceDisclosure{}
	for rows.Next() {
		var value domain.SourceDisclosure
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

func scanApprovedSnapshot(row pgx.Row) (domain.ApprovedSnapshot, error) {
	var value domain.ApprovedSnapshot
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

func (s *Store) ApprovedSnapshots(ctx context.Context, tenantID, projectID, submissionType string) ([]domain.ApprovedSnapshot, error) {
	values := []domain.ApprovedSnapshot{}
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

func (s *Store) ApprovedSnapshot(ctx context.Context, tenantID, id string) (domain.ApprovedSnapshot, error) {
	var result domain.ApprovedSnapshot
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanApprovedSnapshot(tx.QueryRow(ctx, approvedSnapshotSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("ApprovedSnapshot")
		}
		return err
	})
	return result, err
}

func (s *Store) ApproveSubmissionRevision(ctx context.Context, submission domain.Submission, snapshot domain.ApprovedSnapshot, decision domain.ApprovalDecision) error {
	return s.withTenant(ctx, submission.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO approval_decisions(id,tenant_id,project_id,subject_type,subject_id,subject_hash,actor_id,decision,reason,previous_state,resulting_state,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, decision.ID, decision.TenantID, decision.ProjectID, decision.SubjectType, decision.SubjectID, decision.SubjectHash, decision.ActorID, decision.Decision, decision.Reason, decision.PreviousState, decision.ResultingState, decision.CreatedAt); err != nil {
			return dbError(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO approved_snapshots(id,tenant_id,project_id,workspace_id,submission_id,submission_revision_id,submission_type,schema_version,content_hash,subject_hash,canonical_content,eligible_ids,artifacts,decision_id,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, snapshot.ID, snapshot.TenantID, snapshot.ProjectID, snapshot.WorkspaceID, snapshot.SubmissionID, snapshot.SubmissionRevisionID, snapshot.SubmissionType, snapshot.SchemaVersion, snapshot.ContentHash, snapshot.SubjectHash, []byte(snapshot.CanonicalContent), jsonValue(snapshot.EligibleIDs), jsonValue(snapshot.Artifacts), snapshot.DecisionID, snapshot.CreatedBy, snapshot.CreatedAt); err != nil {
			return dbError(err)
		}
		result, err := tx.Exec(ctx, `UPDATE submissions SET status=$3,current_revision_id=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2`, submission.TenantID, submission.ID, submission.Status, submission.CurrentRevisionID, submission.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("Submission")
		}
		return nil
	})
}

func (s *Store) RequestSubmissionChanges(ctx context.Context, submission domain.Submission, decision domain.ApprovalDecision, comment domain.ReviewComment) error {
	return s.withTenant(ctx, submission.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO approval_decisions(id,tenant_id,project_id,subject_type,subject_id,subject_hash,actor_id,decision,reason,previous_state,resulting_state,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, decision.ID, decision.TenantID, decision.ProjectID, decision.SubjectType, decision.SubjectID, decision.SubjectHash, decision.ActorID, decision.Decision, decision.Reason, decision.PreviousState, decision.ResultingState, decision.CreatedAt); err != nil {
			return dbError(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO review_comments(id,tenant_id,project_id,review_cycle_id,subject_type,subject_id,carried_from_comment_id,shot_id,json_pointer,body,visibility,author_id,resolved_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, comment.ID, comment.TenantID, comment.ProjectID, comment.ReviewCycleID, comment.SubjectType, comment.SubjectID, nullable(comment.CarriedFromID), comment.ShotID, comment.JSONPointer, comment.Body, comment.Visibility, comment.AuthorID, comment.ResolvedAt, comment.CreatedAt); err != nil {
			return dbError(err)
		}
		result, err := tx.Exec(ctx, `UPDATE submissions SET status=$3,current_revision_id=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2`, submission.TenantID, submission.ID, submission.Status, submission.CurrentRevisionID, submission.UpdatedAt)
		if err != nil {
			return dbError(err)
		}
		if result.RowsAffected() == 0 {
			return domain.NotFound("Submission")
		}
		return nil
	})
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
}
