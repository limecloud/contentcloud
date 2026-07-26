package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type SubmissionDetails struct {
	Submission domain.Submission           `json:"submission"`
	Revisions  []domain.SubmissionRevision `json:"revisions"`
	Comments   []domain.ReviewComment      `json:"comments"`
}

type SubmissionApprovalResult struct {
	Submission       domain.Submission        `json:"submission"`
	Decision         domain.ApprovalDecision  `json:"decision"`
	ApprovedSnapshot *domain.ApprovedSnapshot `json:"approved_snapshot,omitempty"`
}

func (s *Service) RegisterWorkspace(ctx context.Context, actor Actor, binding domain.WorkspaceBinding, templateID, templateVersion string, targets []string, requestID string) (domain.WorkspaceBinding, error) {
	if actor.Type != "workspace" || actor.WorkspaceID != binding.ID {
		return binding, domain.Policy("WORKSPACE_SCOPE_DENIED", "工作区凭据与绑定不匹配", "重新执行项目初始化")
	}
	if strings.TrimSpace(templateID) == "" || strings.TrimSpace(templateVersion) == "" {
		return binding, domain.Invalid("WORKSPACE_TEMPLATE_REQUIRED", "template_id 和 template_version 必填")
	}
	for _, target := range targets {
		if target != "codex" && target != "claude" {
			return binding, domain.Invalid("WORKSPACE_TARGET_INVALID", "工作区 target 只允许 codex 或 claude")
		}
	}
	binding.TemplateID = templateID
	binding.TemplateVersion = templateVersion
	binding.Targets = append([]string(nil), targets...)
	binding.LastSeenAt = s.now().UTC()
	if err := s.store.SaveWorkspaceBinding(ctx, binding); err != nil {
		return binding, err
	}
	if binding.DeviceID != "" {
		device, err := s.store.Device(ctx, binding.TenantID, binding.DeviceID)
		if err != nil {
			return binding, err
		}
		device.LastSeenAt = binding.LastSeenAt
		if err := s.store.SaveDevice(ctx, device); err != nil {
			return binding, err
		}
	}
	binding.CredentialHash = ""
	s.audit(ctx, actor, binding.ProjectID, "workspace.registered", "workspace_binding", binding.ID, requestID, map[string]any{"template_version": templateVersion, "targets": targets})
	return binding, nil
}

func (s *Service) CreateSubmission(ctx context.Context, actor Actor, binding domain.WorkspaceBinding, bundle domain.SubmissionBundle, requestID string) (domain.SubmissionRevision, error) {
	if actor.Type != "workspace" || actor.WorkspaceID != binding.ID || bundle.WorkspaceID != binding.ID || bundle.ProjectID != binding.ProjectID {
		return domain.SubmissionRevision{}, domain.Policy("WORKSPACE_SCOPE_DENIED", "提交不属于当前工作区和项目", "检查本地 project.yaml 后重试")
	}
	if err := bundle.Validate(); err != nil {
		return domain.SubmissionRevision{}, err
	}
	if _, err := s.store.Project(ctx, binding.TenantID, binding.ProjectID); err != nil {
		return domain.SubmissionRevision{}, err
	}
	if bundle.BaseApprovedSnapshotID != "" {
		snapshot, err := s.store.ApprovedSnapshot(ctx, binding.TenantID, bundle.BaseApprovedSnapshotID)
		if err != nil {
			return domain.SubmissionRevision{}, err
		}
		if snapshot.ProjectID != binding.ProjectID || snapshot.SubmissionType != bundle.SubmissionType {
			return domain.SubmissionRevision{}, domain.Conflict("BASE_SNAPSHOT_MISMATCH", "批准基线与当前项目或提交类型不匹配")
		}
	}
	now := s.now().UTC()
	submission, err := s.store.SubmissionByWorkspaceType(ctx, binding.TenantID, binding.ProjectID, binding.ID, bundle.SubmissionType)
	if err != nil && !isNotFound(err) {
		return domain.SubmissionRevision{}, err
	}
	revisionNo := 1
	if isNotFound(err) {
		submission = domain.Submission{ID: domain.NewID(), TenantID: binding.TenantID, ProjectID: binding.ProjectID, WorkspaceID: binding.ID, SubmissionType: bundle.SubmissionType, Status: "preparing", CreatedBy: binding.ID, CreatedAt: now, UpdatedAt: now}
	} else {
		revisions, err := s.store.SubmissionRevisions(ctx, binding.TenantID, submission.ID)
		if err != nil {
			return domain.SubmissionRevision{}, err
		}
		for _, existing := range revisions {
			if existing.IdempotencyKey == bundle.IdempotencyKey {
				if existing.ContentHash != normalizeSubmissionHash(bundle.ContentHash) {
					return domain.SubmissionRevision{}, domain.Conflict("IDEMPOTENCY_CONTENT_MISMATCH", "相同幂等键不能提交不同内容")
				}
				return existing, nil
			}
			if existing.RevisionNo >= revisionNo {
				revisionNo = existing.RevisionNo + 1
			}
		}
	}
	disclosures := append([]domain.SourceDisclosure(nil), bundle.SourceDisclosures...)
	for index := range disclosures {
		disclosures[index].ID = domain.NewID()
		disclosures[index].TenantID = binding.TenantID
		disclosures[index].ProjectID = binding.ProjectID
		disclosures[index].CreatedAt = now
	}
	revision := domain.SubmissionRevision{
		ID: domain.NewID(), TenantID: binding.TenantID, ProjectID: binding.ProjectID, WorkspaceID: binding.ID, SubmissionID: submission.ID,
		RevisionNo: revisionNo, SchemaVersion: bundle.SchemaVersion, ContentHash: normalizeSubmissionHash(bundle.ContentHash), BaseApprovedSnapshotID: bundle.BaseApprovedSnapshotID,
		LocalRunSummary: bundle.LocalRunSummary, Objects: append(json.RawMessage(nil), bundle.Objects...), Artifacts: append([]domain.SubmissionArtifact{}, bundle.Artifacts...), Message: strings.TrimSpace(bundle.Message),
		IdempotencyKey: bundle.IdempotencyKey, EvidenceLimited: domain.EvidenceLimited(bundle.Objects, disclosures), CreatedBy: binding.ID, CreatedAt: now, SourceDisclosures: disclosures,
	}
	submission.Status = "submitted"
	submission.CurrentRevisionID = revision.ID
	submission.UpdatedAt = now
	cycle := domain.ReviewCycle{ID: domain.NewID(), TenantID: binding.TenantID, ProjectID: binding.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, Status: "open", OpenedBy: binding.ID, OpenedAt: now, CreatedAt: now}
	if err := s.store.CreateSubmissionRevision(ctx, submission, revision, disclosures, cycle); err != nil {
		return domain.SubmissionRevision{}, err
	}
	s.audit(ctx, actor, binding.ProjectID, "submission.published", "submission_revision", revision.ID, requestID, map[string]any{"submission_id": submission.ID, "type": submission.SubmissionType, "revision_no": revision.RevisionNo, "content_hash": revision.ContentHash, "evidence_limited": revision.EvidenceLimited})
	return revision, nil
}

func (s *Service) Submissions(ctx context.Context, actor Actor, projectID string) ([]domain.Submission, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.Submissions(ctx, actor.TenantID, projectID)
}

func (s *Service) WorkspaceSubmissions(ctx context.Context, actor Actor, binding domain.WorkspaceBinding) ([]domain.Submission, error) {
	if actor.WorkspaceID != binding.ID {
		return nil, domain.Policy("WORKSPACE_SCOPE_DENIED", "工作区范围不匹配", "重新初始化工作区")
	}
	values, err := s.store.Submissions(ctx, actor.TenantID, binding.ProjectID)
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.Submission, 0, len(values))
	for _, value := range values {
		if value.WorkspaceID == binding.ID {
			filtered = append(filtered, value)
		}
	}
	return filtered, nil
}

func (s *Service) SubmissionDetails(ctx context.Context, actor Actor, id string) (SubmissionDetails, error) {
	submission, err := s.store.Submission(ctx, actor.TenantID, id)
	if err != nil {
		return SubmissionDetails{}, err
	}
	if actor.Type == "workspace" && submission.WorkspaceID != actor.WorkspaceID {
		return SubmissionDetails{}, domain.NotFound("Submission")
	}
	revisions, err := s.store.SubmissionRevisions(ctx, actor.TenantID, submission.ID)
	if err != nil {
		return SubmissionDetails{}, err
	}
	comments := []domain.ReviewComment{}
	for _, revision := range revisions {
		values, err := s.store.ReviewComments(ctx, actor.TenantID, revision.ID)
		if err != nil {
			return SubmissionDetails{}, err
		}
		comments = append(comments, values...)
	}
	return SubmissionDetails{Submission: submission, Revisions: revisions, Comments: comments}, nil
}

func (s *Service) ApproveSubmission(ctx context.Context, actor Actor, revisionID, reason, requestID string) (SubmissionApprovalResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer"); err != nil {
		return SubmissionApprovalResult{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return SubmissionApprovalResult{}, domain.Invalid("APPROVAL_REASON_REQUIRED", "批准必须填写整版结论")
	}
	revision, err := s.store.SubmissionRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return SubmissionApprovalResult{}, err
	}
	submission, err := s.store.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil {
		return SubmissionApprovalResult{}, err
	}
	if submission.CurrentRevisionID != revision.ID || (submission.Status != "submitted" && submission.Status != "in_review") {
		return SubmissionApprovalResult{}, domain.Conflict("SUBMISSION_STATE_INVALID", "只能批准当前待审 SubmissionRevision")
	}
	if revision.EvidenceLimited {
		return SubmissionApprovalResult{}, domain.Policy("EVIDENCE_LEVEL_INSUFFICIENT", "高风险内容的来源披露不足，不能远程批准", "上传 evidence_pack/full_source，或完成受治理的本地核验")
	}
	if err := s.requireResolvedComments(ctx, actor.TenantID, revision.ID, ""); err != nil {
		return SubmissionApprovalResult{}, err
	}
	now := s.now().UTC()
	resultingState := "approved"
	if submission.SubmissionType == "script" {
		resultingState = "internally_approved"
	}
	decision := domain.ApprovalDecision{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, DecisionStage: "internal", ActorID: actor.UserID, Decision: "approve", Reason: strings.TrimSpace(reason), PreviousState: submission.Status, ResultingState: resultingState, CreatedAt: now}
	submission.Status = resultingState
	submission.UpdatedAt = now
	result := SubmissionApprovalResult{Submission: submission, Decision: decision}
	if submission.SubmissionType == "script" {
		if err := s.store.RecordSubmissionApproval(ctx, submission, decision); err != nil {
			return SubmissionApprovalResult{}, err
		}
		s.audit(ctx, actor, revision.ProjectID, "submission.internally_approved", "submission_revision", revision.ID, requestID, map[string]any{"submission_id": submission.ID, "content_hash": revision.ContentHash})
		return result, nil
	}
	canonical, err := canonicalSubmissionContent(submission, revision)
	if err != nil {
		return SubmissionApprovalResult{}, err
	}
	snapshot := domain.ApprovedSnapshot{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, WorkspaceID: revision.WorkspaceID, SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, SubmissionType: submission.SubmissionType, SchemaVersion: revision.SchemaVersion, ContentHash: revision.ContentHash, SubjectHash: revision.ContentHash, CanonicalContent: canonical, EligibleIDs: revision.EligibleObjectIDs(), Artifacts: revision.Artifacts, DecisionID: decision.ID, CreatedBy: actor.UserID, CreatedAt: now, Origin: "current"}
	if err := s.store.ApproveSubmissionRevision(ctx, submission, snapshot, decision); err != nil {
		return SubmissionApprovalResult{}, err
	}
	s.audit(ctx, actor, revision.ProjectID, "submission.approved", "submission_revision", revision.ID, requestID, map[string]any{"submission_id": submission.ID, "snapshot_id": snapshot.ID, "content_hash": snapshot.ContentHash})
	result.ApprovedSnapshot = &snapshot
	return result, nil
}

func (s *Service) RequestSubmissionChanges(ctx context.Context, actor Actor, revisionID, reason, jsonPointer, requestID string) (domain.Submission, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer"); err != nil {
		return domain.Submission{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return domain.Submission{}, domain.Invalid("CHANGE_REASON_REQUIRED", "修改要求必须填写具体原因")
	}
	if jsonPointer != "" && !domain.ValidJSONPointer(jsonPointer) {
		return domain.Submission{}, domain.Invalid("COMMENT_POINTER_INVALID", "批注位置必须使用合法 JSON Pointer")
	}
	revision, err := s.store.SubmissionRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return domain.Submission{}, err
	}
	submission, err := s.store.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil {
		return submission, err
	}
	if submission.CurrentRevisionID != revision.ID || (submission.Status != "submitted" && submission.Status != "in_review" && submission.Status != "internally_approved" && submission.Status != "client_review") {
		return submission, domain.Conflict("SUBMISSION_STATE_INVALID", "只能退回当前待审 SubmissionRevision")
	}
	cycles, err := s.store.ReviewCycles(ctx, actor.TenantID, revision.ID)
	if err != nil {
		return submission, err
	}
	var cycle domain.ReviewCycle
	if len(cycles) > 0 && cycles[0].Status == "open" {
		cycle = cycles[0]
	} else {
		now := s.now().UTC()
		cycle = domain.ReviewCycle{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, Status: "open", OpenedBy: actor.UserID, OpenedAt: now, CreatedAt: now}
		cycle, err = s.store.CreateReviewCycle(ctx, cycle)
		if err != nil {
			return submission, err
		}
	}
	now := s.now().UTC()
	comment := domain.ReviewComment{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, ReviewCycleID: cycle.ID, SubjectType: "submission_revision", SubjectID: revision.ID, JSONPointer: jsonPointer, Body: reason, Visibility: "internal", AuthorID: actor.UserID, CreatedAt: now}
	decision := domain.ApprovalDecision{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, DecisionStage: "internal", ActorID: actor.UserID, Decision: "request_changes", Reason: reason, PreviousState: submission.Status, ResultingState: "changes_requested", CreatedAt: now}
	submission.Status = "changes_requested"
	submission.UpdatedAt = now
	if err := s.store.RequestSubmissionChanges(ctx, submission, decision, comment); err != nil {
		return submission, err
	}
	s.audit(ctx, actor, revision.ProjectID, "submission.changes_requested", "submission_revision", revision.ID, requestID, map[string]any{"submission_id": submission.ID, "json_pointer": jsonPointer})
	return submission, nil
}

func canonicalSubmissionContent(submission domain.Submission, revision domain.SubmissionRevision) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"schema_version": revision.SchemaVersion, "submission_type": submission.SubmissionType, "objects": revision.Objects,
		"source_disclosures": revision.SourceDisclosures, "artifacts": revision.Artifacts, "local_run_summary": revision.LocalRunSummary,
	})
}

func (s *Service) ApprovedSnapshots(ctx context.Context, actor Actor, projectID, submissionType string) ([]domain.ApprovedSnapshot, error) {
	if actor.Type == "workspace" {
		binding, err := s.store.WorkspaceBinding(ctx, actor.TenantID, actor.WorkspaceID)
		if err != nil || binding.ProjectID != projectID {
			return nil, domain.NotFound("项目")
		}
	} else if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.ApprovedSnapshots(ctx, actor.TenantID, projectID, submissionType)
}

func (s *Service) ApprovedSnapshot(ctx context.Context, actor Actor, id string) (domain.ApprovedSnapshot, error) {
	snapshot, err := s.store.ApprovedSnapshot(ctx, actor.TenantID, id)
	if err != nil {
		return snapshot, err
	}
	if actor.Type == "workspace" && snapshot.WorkspaceID != actor.WorkspaceID {
		return domain.ApprovedSnapshot{}, domain.NotFound("ApprovedSnapshot")
	}
	return snapshot, nil
}

func (s *Service) WorkspaceFeedback(ctx context.Context, actor Actor, binding domain.WorkspaceBinding) ([]domain.ReviewFeedbackBundle, error) {
	submissions, err := s.WorkspaceSubmissions(ctx, actor, binding)
	if err != nil {
		return nil, err
	}
	bundles := []domain.ReviewFeedbackBundle{}
	for _, submission := range submissions {
		revisions, err := s.store.SubmissionRevisions(ctx, actor.TenantID, submission.ID)
		if err != nil {
			return nil, err
		}
		for _, revision := range revisions {
			comments, err := s.store.ReviewComments(ctx, actor.TenantID, revision.ID)
			if err != nil {
				return nil, err
			}
			if len(comments) == 0 {
				continue
			}
			bundles = append(bundles, domain.ReviewFeedbackBundle{BundleVersion: "1.0", SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, SubjectHash: revision.ContentHash, Comments: comments, CreatedAt: comments[len(comments)-1].CreatedAt})
		}
	}
	return bundles, nil
}

func (s *Service) WorkspaceDecisions(ctx context.Context, actor Actor, binding domain.WorkspaceBinding) (domain.DecisionDelta, error) {
	submissions, err := s.WorkspaceSubmissions(ctx, actor, binding)
	if err != nil {
		return domain.DecisionDelta{}, err
	}
	decisions := []domain.ApprovalDecision{}
	for _, submission := range submissions {
		revisions, err := s.store.SubmissionRevisions(ctx, actor.TenantID, submission.ID)
		if err != nil {
			return domain.DecisionDelta{}, err
		}
		for _, revision := range revisions {
			values, err := s.store.Approvals(ctx, actor.TenantID, revision.ID)
			if err != nil {
				return domain.DecisionDelta{}, err
			}
			decisions = append(decisions, values...)
		}
	}
	return domain.DecisionDelta{BundleVersion: "1.0", ProjectID: binding.ProjectID, Decisions: decisions, CreatedAt: s.now().UTC()}, nil
}

func isNotFound(err error) bool {
	var domainError *domain.Error
	return errors.As(err, &domainError) && domainError.Type == "not_found"
}

func normalizeSubmissionHash(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
}
