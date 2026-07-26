package memory

import (
	"context"
	"sort"

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

func (s *Store) CreateSubmissionRevision(_ context.Context, submission domain.Submission, revision domain.SubmissionRevision, disclosures []domain.SourceDisclosure) error {
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
	s.submissions[submission.ID] = submission
	s.submissionRevisions[revision.ID] = revision
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
