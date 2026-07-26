package memory

import (
	"context"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateWorkspaceBinding(_ context.Context, value domain.WorkspaceBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.workspaceBindings {
		if existing.CredentialHash == value.CredentialHash {
			return domain.Conflict("WORKSPACE_CREDENTIAL_EXISTS", "工作区凭据已存在")
		}
	}
	s.workspaceBindings[value.ID] = value
	return nil
}

func (s *Store) WorkspaceBindingByTokenHash(_ context.Context, hash string) (domain.WorkspaceBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.workspaceBindings {
		if value.CredentialHash == hash && value.Status == "active" && value.RevokedAt == nil {
			return value, nil
		}
	}
	return domain.WorkspaceBinding{}, domain.NotFound("工作区凭据")
}

func (s *Store) WorkspaceBinding(_ context.Context, tenantID, id string) (domain.WorkspaceBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.workspaceBindings[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("工作区绑定")
	}
	value.CredentialHash = ""
	return value, nil
}

func (s *Store) SaveWorkspaceBinding(_ context.Context, value domain.WorkspaceBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.workspaceBindings[value.ID]
	if !ok || existing.TenantID != value.TenantID {
		return domain.NotFound("工作区绑定")
	}
	if value.CredentialHash == "" {
		value.CredentialHash = existing.CredentialHash
	}
	s.workspaceBindings[value.ID] = value
	return nil
}

func (s *Store) CreateSubmissionRevision(_ context.Context, submission domain.Submission, revision domain.SubmissionRevision, disclosures []domain.SourceDisclosure, cycle domain.ReviewCycle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.submissionRevisions {
		if existing.WorkspaceID == revision.WorkspaceID && existing.IdempotencyKey == revision.IdempotencyKey {
			return domain.Conflict("SUBMISSION_IDEMPOTENCY_CONFLICT", "该幂等键已创建提交版本")
		}
	}
	if existing, ok := s.submissions[submission.ID]; ok {
		if existing.TenantID != submission.TenantID || existing.ProjectID != submission.ProjectID {
			return domain.NotFound("Submission")
		}
	}
	for index := range disclosures {
		disclosures[index].TenantID = revision.TenantID
		disclosures[index].ProjectID = revision.ProjectID
		disclosures[index].SubmissionRevisionID = revision.ID
	}
	revision.SourceDisclosures = append([]domain.SourceDisclosure(nil), disclosures...)
	cycle.CycleNumber = 1
	s.submissions[submission.ID] = submission
	s.submissionRevisions[revision.ID] = revision
	s.reviewCycles[cycle.ID] = cycle
	for id, grant := range s.reviewGrants {
		grantRevision, ok := s.submissionRevisions[grant.SubjectID]
		if grant.SubjectType == "submission_revision" && ok && grantRevision.SubmissionID == submission.ID && grant.SubjectID != revision.ID && grant.RevokedAt == nil && grant.DecisionAt == nil {
			revokedAt := revision.CreatedAt
			grant.RevokedAt = &revokedAt
			s.reviewGrants[id] = grant
		}
	}
	return nil
}

func (s *Store) SubmissionByWorkspaceType(_ context.Context, tenantID, projectID, workspaceID, submissionType string) (domain.Submission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.submissions {
		if value.TenantID == tenantID && value.ProjectID == projectID && value.WorkspaceID == workspaceID && value.SubmissionType == submissionType {
			return value, nil
		}
	}
	return domain.Submission{}, domain.NotFound("Submission")
}

func (s *Store) Submissions(_ context.Context, tenantID, projectID string) ([]domain.Submission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := []domain.Submission{}
	for _, value := range s.submissions {
		if value.TenantID == tenantID && (projectID == "" || value.ProjectID == projectID) {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	return values, nil
}

func (s *Store) Submission(_ context.Context, tenantID, id string) (domain.Submission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.submissions[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("Submission")
	}
	return value, nil
}

func (s *Store) SubmissionRevision(_ context.Context, tenantID, id string) (domain.SubmissionRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.submissionRevisions[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("SubmissionRevision")
	}
	return value, nil
}

func (s *Store) SubmissionRevisions(_ context.Context, tenantID, submissionID string) ([]domain.SubmissionRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := []domain.SubmissionRevision{}
	for _, value := range s.submissionRevisions {
		if value.TenantID == tenantID && value.SubmissionID == submissionID {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].RevisionNo > values[j].RevisionNo })
	return values, nil
}

func (s *Store) ApprovedSnapshots(_ context.Context, tenantID, projectID, submissionType string) ([]domain.ApprovedSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := []domain.ApprovedSnapshot{}
	for _, value := range s.approvedSnapshots {
		if value.TenantID == tenantID && (projectID == "" || value.ProjectID == projectID) && (submissionType == "" || value.SubmissionType == submissionType) {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].CreatedAt.After(values[j].CreatedAt) })
	return values, nil
}

func (s *Store) ApprovedSnapshot(_ context.Context, tenantID, id string) (domain.ApprovedSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.approvedSnapshots[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("ApprovedSnapshot")
	}
	return value, nil
}

func (s *Store) RecordSubmissionApproval(_ context.Context, submission domain.Submission, decision domain.ApprovalDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.submissions[submission.ID]
	if !ok || existing.TenantID != submission.TenantID {
		return domain.NotFound("Submission")
	}
	s.submissions[submission.ID] = submission
	s.approvals[decision.ID] = decision
	return nil
}

func (s *Store) ApproveSubmissionRevision(_ context.Context, submission domain.Submission, snapshot domain.ApprovedSnapshot, decision domain.ApprovalDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.submissions[submission.ID]
	if !ok || existing.TenantID != submission.TenantID {
		return domain.NotFound("Submission")
	}
	if _, ok := s.submissionRevisions[snapshot.SubmissionRevisionID]; !ok {
		return domain.NotFound("SubmissionRevision")
	}
	s.submissions[submission.ID] = submission
	s.approvedSnapshots[snapshot.ID] = snapshot
	s.approvals[decision.ID] = decision
	return nil
}

func (s *Store) RequestSubmissionChanges(_ context.Context, submission domain.Submission, decision domain.ApprovalDecision, comment domain.ReviewComment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.submissions[submission.ID]
	if !ok || existing.TenantID != submission.TenantID {
		return domain.NotFound("Submission")
	}
	s.submissions[submission.ID] = submission
	s.approvals[decision.ID] = decision
	s.reviewComments[comment.ID] = comment
	for id, grant := range s.reviewGrants {
		if grant.TenantID == submission.TenantID && grant.SubjectID == submission.CurrentRevisionID && grant.SubjectType == "submission_revision" && grant.RevokedAt == nil && grant.DecisionAt == nil {
			revokedAt := decision.CreatedAt
			grant.RevokedAt = &revokedAt
			s.reviewGrants[id] = grant
		}
	}
	return nil
}

func (s *Store) CreateSubmissionReviewGrant(_ context.Context, submission domain.Submission, grant domain.ReviewGrant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.submissions[submission.ID]
	if !ok || existing.TenantID != submission.TenantID {
		return domain.NotFound("Submission")
	}
	if _, ok := s.submissionRevisions[grant.SubjectID]; !ok {
		return domain.NotFound("SubmissionRevision")
	}
	s.submissions[submission.ID] = submission
	s.reviewGrants[grant.ID] = grant
	return nil
}

func (s *Store) CompleteSubmissionClientReview(_ context.Context, submission domain.Submission, grant domain.ReviewGrant, decision domain.ApprovalDecision, comment *domain.ReviewComment, snapshot *domain.ApprovedSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.submissions[submission.ID]
	if !ok || existing.TenantID != submission.TenantID {
		return domain.NotFound("Submission")
	}
	storedGrant, ok := s.reviewGrants[grant.ID]
	if !ok || storedGrant.DecisionAt != nil || storedGrant.RevokedAt != nil || time.Now().After(storedGrant.ExpiresAt) {
		return domain.Conflict("REVIEW_ALREADY_DECIDED", "该审批链接已失效或已完成最终决策")
	}
	s.submissions[submission.ID] = submission
	s.reviewGrants[grant.ID] = grant
	s.approvals[decision.ID] = decision
	if comment != nil {
		s.reviewComments[comment.ID] = *comment
	}
	if snapshot != nil {
		if _, exists := s.approvedSnapshots[snapshot.ID]; exists {
			return domain.Conflict("APPROVED_SNAPSHOT_EXISTS", "批准快照已存在")
		}
		s.approvedSnapshots[snapshot.ID] = *snapshot
	}
	return nil
}

func (s *Store) CompleteLegacyClientReview(_ context.Context, script domain.ScriptVersion, grant domain.ReviewGrant, decision domain.ApprovalDecision, comment *domain.ReviewComment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	storedGrant, ok := s.reviewGrants[grant.ID]
	if !ok || storedGrant.TenantID != grant.TenantID || storedGrant.VerifiedAt == nil || storedGrant.RevokedAt != nil || storedGrant.DecisionAt != nil || !decision.CreatedAt.Before(storedGrant.ExpiresAt) {
		return domain.Conflict("REVIEW_GRANT_STATE_INVALID", "客户审批授权未验证、已撤销、已完成或已过期")
	}
	storedScript, ok := s.scripts[script.ID]
	if !ok || storedScript.TenantID != script.TenantID || storedScript.ContentHash != script.ContentHash || storedScript.Status != "client_review" {
		return domain.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效或状态已变化")
	}
	storedScript.Status = script.Status
	s.scripts[script.ID] = storedScript
	decision.DecisionStage = defaultMemoryDecisionStage(decision.DecisionStage)
	s.approvals[decision.ID] = decision
	if comment != nil {
		s.reviewComments[comment.ID] = *comment
	}
	decidedAt := decision.CreatedAt
	storedGrant.DecisionAt = &decidedAt
	s.reviewGrants[grant.ID] = storedGrant
	return nil
}

func defaultMemoryDecisionStage(value string) string {
	if value == "" {
		return "legacy"
	}
	return value
}
