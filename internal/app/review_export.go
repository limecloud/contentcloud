package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/exportfmt"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

type CreateReviewCommentInput struct {
	SubjectID   string `json:"subject_id"`
	ShotID      string `json:"shot_id"`
	JSONPointer string `json:"json_pointer"`
	Body        string `json:"body"`
	Visibility  string `json:"visibility"`
}

func (s *Service) CreateReviewComment(ctx context.Context, actor Actor, in CreateReviewCommentInput, requestID string) (domain.ReviewComment, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor", "reviewer"); err != nil {
		return domain.ReviewComment{}, err
	}
	script, err := s.store.Script(ctx, actor.TenantID, in.SubjectID)
	if err != nil {
		return domain.ReviewComment{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, script.ProjectID); err != nil {
		return domain.ReviewComment{}, err
	}
	if strings.TrimSpace(in.Body) == "" {
		return domain.ReviewComment{}, domain.Invalid("COMMENT_BODY_REQUIRED", "批注内容必填")
	}
	if in.JSONPointer != "" && !domain.ValidJSONPointer(in.JSONPointer) {
		return domain.ReviewComment{}, domain.Invalid("COMMENT_POINTER_INVALID", "批注位置必须使用合法 JSON Pointer")
	}
	if in.ShotID != "" {
		found := false
		for _, shot := range script.Package.Shots {
			found = found || shot.ShotID == in.ShotID
		}
		if !found {
			return domain.ReviewComment{}, domain.NotFound("镜头")
		}
	}
	visibility := defaultString(in.Visibility, "internal")
	if visibility != "internal" && visibility != "client" {
		return domain.ReviewComment{}, domain.Invalid("COMMENT_VISIBILITY_INVALID", "批注可见性必须为 internal 或 client")
	}
	cycle, _, err := s.ensureReviewCycle(ctx, actor, script)
	if err != nil {
		return domain.ReviewComment{}, err
	}
	v := domain.ReviewComment{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: script.ProjectID, ReviewCycleID: cycle.ID, SubjectType: "script_version", SubjectID: script.ID, ShotID: in.ShotID, JSONPointer: in.JSONPointer, Body: strings.TrimSpace(in.Body), Visibility: visibility, AuthorID: actor.UserID, CreatedAt: s.now().UTC()}
	if err := s.store.CreateReviewComment(ctx, v); err != nil {
		return v, err
	}
	s.audit(ctx, actor, script.ProjectID, "review_comment.created", "review_comment", v.ID, requestID, map[string]any{"script_version_id": script.ID, "shot_id": v.ShotID, "visibility": v.Visibility})
	return v, nil
}

func (s *Service) ReviewComments(ctx context.Context, actor Actor, scriptID string) ([]domain.ReviewComment, error) {
	if _, err := s.store.Script(ctx, actor.TenantID, scriptID); err != nil {
		return nil, err
	}
	return s.store.ReviewComments(ctx, actor.TenantID, scriptID)
}

func (s *Service) ResolveReviewComment(ctx context.Context, actor Actor, id, requestID string) (domain.ReviewComment, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor", "reviewer"); err != nil {
		return domain.ReviewComment{}, err
	}
	target, err := s.store.ReviewComment(ctx, actor.TenantID, id)
	if err != nil {
		return domain.ReviewComment{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, target.ProjectID); err != nil {
		return domain.ReviewComment{}, err
	}
	if target.ResolvedAt != nil {
		return target, nil
	}
	now := s.now().UTC()
	target.ResolvedAt = &now
	if err := s.store.SaveReviewComment(ctx, target); err != nil {
		return target, err
	}
	s.audit(ctx, actor, target.ProjectID, "review_comment.resolved", "review_comment", target.ID, requestID, map[string]any{"script_version_id": target.SubjectID})
	return target, nil
}

type ReviewProjection struct {
	Project    domain.Project           `json:"project"`
	Script     domain.ScriptVersion     `json:"script,omitempty"`
	Submission *SubmissionReviewSubject `json:"submission,omitempty"`
	Comments   []domain.ReviewComment   `json:"comments"`
	Verified   bool                     `json:"verified"`
}

type SubmissionReviewSubject struct {
	SubmissionID         string          `json:"submission_id"`
	SubmissionRevisionID string          `json:"submission_revision_id"`
	SubjectHash          string          `json:"subject_hash"`
	SchemaVersion        string          `json:"schema_version"`
	Objects              json.RawMessage `json:"objects"`
}

type ReviewDecisionResult struct {
	SubjectType      string                   `json:"subject_type"`
	SubjectID        string                   `json:"subject_id"`
	Status           string                   `json:"status"`
	ApprovedSnapshot *domain.ApprovedSnapshot `json:"approved_snapshot,omitempty"`
}

type SubmissionReviewStatus struct {
	Submission domain.Submission         `json:"submission"`
	Revision   domain.SubmissionRevision `json:"revision"`
	Grants     []domain.ReviewGrant      `json:"grants"`
}

func (s *Service) CreateReviewGrant(ctx context.Context, actor Actor, revisionID, reviewerEmail, requestID string) (domain.ReviewGrant, error) {
	if !canManage(actor.Role) {
		return domain.ReviewGrant{}, domain.Policy("ROLE_DENIED", "当前角色不能创建客户审批", "联系项目负责人")
	}
	revision, err := s.store.SubmissionRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return domain.ReviewGrant{}, err
	}
	submission, err := s.store.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil {
		return domain.ReviewGrant{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, revision.ProjectID); err != nil {
		return domain.ReviewGrant{}, err
	}
	if submission.SubmissionType != "script" {
		return domain.ReviewGrant{}, domain.Invalid("REVIEW_SUBJECT_INVALID", "客户审批只接受 script SubmissionRevision")
	}
	if submission.CurrentRevisionID != revision.ID || (submission.Status != "internally_approved" && submission.Status != "client_review") {
		return domain.ReviewGrant{}, domain.Conflict("SUBMISSION_STATE_INVALID", "只有当前已通过内审的 script SubmissionRevision 可发起客户审批")
	}
	if err := s.requireInternalSubmissionApproval(ctx, revision); err != nil {
		return domain.ReviewGrant{}, err
	}
	if !strings.Contains(reviewerEmail, "@") {
		return domain.ReviewGrant{}, domain.Invalid("REVIEWER_EMAIL_INVALID", "客户审批邮箱无效")
	}
	plain, tokenHash, err := domain.NewOpaqueToken("crg_", 32)
	if err != nil {
		return domain.ReviewGrant{}, err
	}
	otp, err := newReviewOTP()
	if err != nil {
		return domain.ReviewGrant{}, err
	}
	now := s.now().UTC()
	v := domain.ReviewGrant{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, ReviewerEmail: strings.ToLower(strings.TrimSpace(reviewerEmail)), TokenHash: tokenHash, OTPHash: domain.TokenHash(otp), ExpiresAt: now.Add(7 * 24 * time.Hour), CreatedAt: now, PlaintextToken: plain, PlaintextOTP: otp}
	stored := v
	stored.PlaintextToken = ""
	stored.PlaintextOTP = ""
	submission.Status = "client_review"
	submission.UpdatedAt = now
	if err := s.store.CreateSubmissionReviewGrant(ctx, submission, stored); err != nil {
		return v, err
	}
	s.audit(ctx, actor, revision.ProjectID, "review_grant.created", "review_grant", v.ID, requestID, map[string]any{"submission_revision_id": revision.ID, "expires_at": v.ExpiresAt})
	return v, nil
}

func newReviewOTP() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func (s *Service) ReviewGrants(ctx context.Context, actor Actor, revisionID string) ([]domain.ReviewGrant, error) {
	if _, err := s.store.SubmissionRevision(ctx, actor.TenantID, revisionID); err != nil {
		return nil, err
	}
	return s.store.ReviewGrants(ctx, actor.TenantID, revisionID)
}

func (s *Service) SubmissionReviewStatus(ctx context.Context, actor Actor, revisionID string) (SubmissionReviewStatus, error) {
	revision, err := s.store.SubmissionRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return SubmissionReviewStatus{}, err
	}
	submission, err := s.store.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil {
		return SubmissionReviewStatus{}, err
	}
	grants, err := s.store.ReviewGrants(ctx, actor.TenantID, revision.ID)
	if err != nil {
		return SubmissionReviewStatus{}, err
	}
	return SubmissionReviewStatus{Submission: submission, Revision: revision, Grants: grants}, nil
}

func (s *Service) LegacyReviewGrants(ctx context.Context, actor Actor, scriptVersionID string) ([]domain.ReviewGrant, error) {
	if _, err := s.store.Script(ctx, actor.TenantID, scriptVersionID); err != nil {
		return nil, err
	}
	return s.store.ReviewGrants(ctx, actor.TenantID, scriptVersionID)
}

func (s *Service) RevokeReviewGrant(ctx context.Context, actor Actor, grantID, requestID string) (domain.ReviewGrant, error) {
	if !canManage(actor.Role) {
		return domain.ReviewGrant{}, domain.Policy("ROLE_DENIED", "当前角色不能撤销客户审批", "联系项目负责人")
	}
	grant, err := s.store.ReviewGrant(ctx, actor.TenantID, grantID)
	if err != nil {
		return grant, err
	}
	if _, err := s.projectForWrite(ctx, actor, grant.ProjectID); err != nil {
		return grant, err
	}
	if grant.RevokedAt != nil {
		grant.TokenHash = ""
		grant.OTPHash = ""
		return grant, nil
	}
	now := s.now().UTC()
	grant.RevokedAt = &now
	if err := s.store.RevokeReviewGrant(ctx, grant.TenantID, grant.ID, now); err != nil {
		return grant, err
	}
	s.audit(ctx, actor, grant.ProjectID, "review_grant.revoked", "review_grant", grant.ID, requestID, map[string]any{"subject_type": grant.SubjectType, "subject_id": grant.SubjectID})
	grant.TokenHash = ""
	grant.OTPHash = ""
	return grant, nil
}

func (s *Service) ReviewProjection(ctx context.Context, reviewToken string) (ReviewProjection, error) {
	grant, err := s.reviewGrant(ctx, reviewToken)
	if err != nil {
		return ReviewProjection{}, err
	}
	project, err := s.store.Project(ctx, grant.TenantID, grant.ProjectID)
	if err != nil {
		return ReviewProjection{}, err
	}
	if grant.VerifiedAt == nil {
		return ReviewProjection{Project: domain.Project{ID: project.ID, BrandName: project.BrandName, ProductName: project.ProductName}, Verified: false}, nil
	}
	if grant.SubjectType == "script_version" {
		script, err := s.store.Script(ctx, grant.TenantID, grant.SubjectID)
		if err != nil || script.ContentHash != grant.SubjectHash {
			return ReviewProjection{}, domain.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效")
		}
		comments, _ := s.store.ReviewComments(ctx, grant.TenantID, script.ID)
		return ReviewProjection{Project: project, Script: script, Comments: clientVisibleComments(comments), Verified: true}, nil
	}
	if grant.SubjectType != "submission_revision" {
		return ReviewProjection{}, domain.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象类型无效")
	}
	revision, err := s.store.SubmissionRevision(ctx, grant.TenantID, grant.SubjectID)
	if err != nil || revision.ContentHash != grant.SubjectHash {
		return ReviewProjection{}, domain.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效")
	}
	submission, err := s.store.Submission(ctx, grant.TenantID, revision.SubmissionID)
	if err != nil || submission.CurrentRevisionID != revision.ID || submission.Status != "client_review" {
		return ReviewProjection{}, domain.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效或状态已变化")
	}
	comments, _ := s.store.ReviewComments(ctx, grant.TenantID, revision.ID)
	subject := &SubmissionReviewSubject{SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, SubjectHash: revision.ContentHash, SchemaVersion: revision.SchemaVersion, Objects: revision.Objects}
	return ReviewProjection{Project: project, Submission: subject, Comments: clientVisibleComments(comments), Verified: true}, nil
}

func clientVisibleComments(comments []domain.ReviewComment) []domain.ReviewComment {
	publicComments := []domain.ReviewComment{}
	for _, comment := range comments {
		if comment.Visibility == "client" {
			publicComments = append(publicComments, comment)
		}
	}
	return publicComments
}

func (s *Service) VerifyReviewGrant(ctx context.Context, reviewToken, otp string) (ReviewProjection, error) {
	grant, err := s.reviewGrant(ctx, reviewToken)
	if err != nil {
		return ReviewProjection{}, err
	}
	if subtle.ConstantTimeCompare([]byte(grant.OTPHash), []byte(domain.TokenHash(strings.TrimSpace(otp)))) != 1 {
		return ReviewProjection{}, domain.E("authentication", "review_otp", "REVIEW_OTP_INVALID", "验证码错误", 3)
	}
	now := s.now().UTC()
	grant.VerifiedAt = &now
	if err := s.store.MarkReviewGrantVerified(ctx, grant.TenantID, grant.ID, now); err != nil {
		return ReviewProjection{}, err
	}
	return s.ReviewProjection(ctx, reviewToken)
}

func (s *Service) DecideReviewGrant(ctx context.Context, reviewToken, decision, reason, shotID, requestID string) (ReviewDecisionResult, error) {
	grant, err := s.reviewGrant(ctx, reviewToken)
	if err != nil {
		return ReviewDecisionResult{}, err
	}
	if grant.VerifiedAt == nil {
		return ReviewDecisionResult{}, domain.E("authentication", "review_otp", "REVIEW_VERIFICATION_REQUIRED", "请先完成邮箱验证码验证", 3)
	}
	if grant.DecisionAt != nil {
		return ReviewDecisionResult{}, domain.Conflict("REVIEW_ALREADY_DECIDED", "该审批链接已完成最终决策")
	}
	if grant.SubjectType == "script_version" {
		return s.decideLegacyReviewGrant(ctx, grant, decision, reason, shotID, requestID)
	}
	if grant.SubjectType != "submission_revision" {
		return ReviewDecisionResult{}, domain.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象类型无效")
	}
	return s.decideSubmissionReviewGrant(ctx, grant, decision, reason, shotID, requestID)
}

func (s *Service) decideSubmissionReviewGrant(ctx context.Context, grant domain.ReviewGrant, decision, reason, shotID, requestID string) (ReviewDecisionResult, error) {
	revision, err := s.store.SubmissionRevision(ctx, grant.TenantID, grant.SubjectID)
	if err != nil || revision.ContentHash != grant.SubjectHash {
		return ReviewDecisionResult{}, domain.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效")
	}
	submission, err := s.store.Submission(ctx, grant.TenantID, revision.SubmissionID)
	if err != nil || submission.CurrentRevisionID != revision.ID || submission.Status != "client_review" || submission.SubmissionType != "script" {
		return ReviewDecisionResult{}, domain.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效或状态已变化")
	}
	if err := s.requireInternalSubmissionApproval(ctx, revision); err != nil {
		return ReviewDecisionResult{}, err
	}
	now := s.now().UTC()
	previous := submission.Status
	var comment *domain.ReviewComment
	var snapshot *domain.ApprovedSnapshot
	switch decision {
	case "approve":
		if err := s.requireResolvedComments(ctx, grant.TenantID, revision.ID, "client"); err != nil {
			return ReviewDecisionResult{}, err
		}
		canonical, err := canonicalSubmissionContent(submission, revision)
		if err != nil {
			return ReviewDecisionResult{}, err
		}
		submission.Status = "approved"
		snapshot = &domain.ApprovedSnapshot{ID: domain.NewID(), TenantID: grant.TenantID, ProjectID: revision.ProjectID, WorkspaceID: revision.WorkspaceID, SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, SubmissionType: submission.SubmissionType, SchemaVersion: revision.SchemaVersion, ContentHash: revision.ContentHash, SubjectHash: revision.ContentHash, CanonicalContent: canonical, EligibleIDs: revision.EligibleObjectIDs(), Artifacts: revision.Artifacts, CreatedBy: "client:" + grant.ReviewerEmail, CreatedAt: now, Origin: "current"}
	case "return":
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return ReviewDecisionResult{}, domain.Invalid("REVIEW_REASON_REQUIRED", "退回修改必须填写原因")
		}
		submission.Status = "changes_requested"
		cycleID := s.submissionReviewCycleID(ctx, grant.TenantID, revision.ID)
		comment = &domain.ReviewComment{ID: domain.NewID(), TenantID: grant.TenantID, ProjectID: grant.ProjectID, ReviewCycleID: cycleID, SubjectType: "submission_revision", SubjectID: revision.ID, ShotID: shotID, Body: reason, Visibility: "client", AuthorID: "client:" + grant.ReviewerEmail, CreatedAt: now}
	default:
		return ReviewDecisionResult{}, domain.Invalid("DECISION_INVALID", "客户审批决策无效")
	}
	approval := domain.ApprovalDecision{ID: domain.NewID(), TenantID: grant.TenantID, ProjectID: grant.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, DecisionStage: "client", ActorID: "client:" + grant.ReviewerEmail, Decision: decision, Reason: reason, PreviousState: previous, ResultingState: submission.Status, CreatedAt: now}
	if snapshot != nil {
		snapshot.DecisionID = approval.ID
	}
	submission.UpdatedAt = now
	grant.DecisionAt = &now
	if err := s.store.CompleteSubmissionClientReview(ctx, submission, grant, approval, comment, snapshot); err != nil {
		return ReviewDecisionResult{}, err
	}
	s.audit(ctx, Actor{UserID: "client:" + grant.ReviewerEmail, TenantID: grant.TenantID, Type: "client"}, grant.ProjectID, "submission.client_reviewed", "submission_revision", revision.ID, requestID, map[string]any{"decision": decision, "to": submission.Status, "snapshot_id": valueOrEmptySnapshotID(snapshot)})
	return ReviewDecisionResult{SubjectType: "submission_revision", SubjectID: revision.ID, Status: submission.Status, ApprovedSnapshot: snapshot}, nil
}

func (s *Service) decideLegacyReviewGrant(ctx context.Context, grant domain.ReviewGrant, decision, reason, shotID, requestID string) (ReviewDecisionResult, error) {
	script, err := s.store.Script(ctx, grant.TenantID, grant.SubjectID)
	if err != nil || script.ContentHash != grant.SubjectHash || script.Status != "client_review" {
		return ReviewDecisionResult{}, domain.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效或状态已变化")
	}
	previous := script.Status
	switch decision {
	case "approve":
		if err := s.requireResolvedComments(ctx, grant.TenantID, script.ID, "client"); err != nil {
			return ReviewDecisionResult{}, err
		}
		if err := s.validateScriptApprovalDependencies(ctx, grant.TenantID, script); err != nil {
			script.Status = "review_required"
			_ = s.store.SaveScript(ctx, script)
			return ReviewDecisionResult{}, err
		}
		script.Status = "approved"
	case "return":
		if strings.TrimSpace(reason) == "" {
			return ReviewDecisionResult{}, domain.Invalid("REVIEW_REASON_REQUIRED", "退回修改必须填写原因")
		}
		script.Status = "revision_requested"
	default:
		return ReviewDecisionResult{}, domain.Invalid("DECISION_INVALID", "客户审批决策无效")
	}
	now := s.now().UTC()
	grant.DecisionAt = &now
	approval := domain.ApprovalDecision{ID: domain.NewID(), TenantID: grant.TenantID, ProjectID: grant.ProjectID, SubjectType: "script_version", SubjectID: script.ID, SubjectHash: script.ContentHash, DecisionStage: "legacy", ActorID: "client:" + grant.ReviewerEmail, Decision: decision, Reason: reason, PreviousState: previous, ResultingState: script.Status, CreatedAt: now}
	var comment *domain.ReviewComment
	if decision == "return" && reason != "" {
		cycles, _ := s.store.ReviewCycles(ctx, grant.TenantID, script.ID)
		cycleID := ""
		if len(cycles) > 0 {
			cycleID = cycles[0].ID
		}
		value := domain.ReviewComment{ID: domain.NewID(), TenantID: grant.TenantID, ProjectID: grant.ProjectID, ReviewCycleID: cycleID, SubjectType: "script_version", SubjectID: script.ID, ShotID: shotID, Body: reason, Visibility: "client", AuthorID: "client:" + grant.ReviewerEmail, CreatedAt: now}
		comment = &value
	}
	if err := s.store.CompleteLegacyClientReview(ctx, script, grant, approval, comment); err != nil {
		return ReviewDecisionResult{}, err
	}
	if decision == "approve" {
		_ = s.supersedeApprovedScriptVersions(ctx, grant.TenantID, script)
	}
	s.audit(ctx, Actor{UserID: "client:" + grant.ReviewerEmail, TenantID: grant.TenantID, Type: "client"}, grant.ProjectID, "script.client_reviewed", "script_version", script.ID, requestID, map[string]any{"decision": decision, "to": script.Status})
	return ReviewDecisionResult{SubjectType: "script_version", SubjectID: script.ID, Status: script.Status}, nil
}

func (s *Service) requireInternalSubmissionApproval(ctx context.Context, revision domain.SubmissionRevision) error {
	decisions, err := s.store.Approvals(ctx, revision.TenantID, revision.ID)
	if err != nil {
		return err
	}
	for _, decision := range decisions {
		if decision.SubjectType == "submission_revision" && decision.SubjectHash == revision.ContentHash && decision.DecisionStage == "internal" && decision.Decision == "approve" {
			return nil
		}
	}
	return domain.Policy("INTERNAL_APPROVAL_REQUIRED", "客户审批前必须完成同一 revision 的内部批准", "先完成 SubmissionRevision 内审")
}

func (s *Service) submissionReviewCycleID(ctx context.Context, tenantID, revisionID string) string {
	cycles, err := s.store.ReviewCycles(ctx, tenantID, revisionID)
	if err == nil && len(cycles) > 0 {
		return cycles[0].ID
	}
	return ""
}

func valueOrEmptySnapshotID(snapshot *domain.ApprovedSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return snapshot.ID
}

func (s *Service) validateScriptApprovalDependencies(ctx context.Context, tenantID string, script domain.ScriptVersion) error {
	run, err := s.store.Run(ctx, tenantID, script.RunID)
	if err != nil {
		return err
	}
	brief, err := s.store.Brief(ctx, tenantID, run.BriefVersionID)
	if err != nil || brief.Status != "approved" {
		return domain.Policy("BRIEF_NOT_APPROVED", "剧本绑定的 Brief 已失效", "创建基于当前已批准 Brief 的新剧本版本")
	}
	if err := s.validateBriefDependencies(ctx, tenantID, brief); err != nil {
		return err
	}
	snapshot, err := s.store.Snapshot(ctx, tenantID, script.InputSnapshotID)
	if err != nil {
		return err
	}
	eligible, err := s.eligibleAssets(ctx, tenantID, script.ProjectID, brief.Channel, s.now().UTC())
	if err != nil {
		return err
	}
	currentAssets := map[string]bool{}
	for _, bundle := range eligible {
		currentAssets[bundle.Asset.ID] = true
	}
	for _, bundle := range snapshot.Assets {
		if !currentAssets[bundle.Asset.ID] {
			return domain.Policy("ASSET_RIGHTS_BLOCKED", "剧本依赖素材的当前权利已失效", "更新权利记录后创建新剧本版本")
		}
	}
	return nil
}

func (s *Service) supersedeApprovedScriptVersions(ctx context.Context, tenantID string, approved domain.ScriptVersion) error {
	versions, err := s.store.Scripts(ctx, tenantID, approved.ProjectID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	for _, version := range versions {
		if version.ID == approved.ID || version.ScriptID != approved.ScriptID || version.Status != "approved" {
			continue
		}
		version.Status = "superseded"
		if err := s.store.SaveScript(ctx, version); err != nil {
			return err
		}
		grants, _ := s.store.ReviewGrants(ctx, tenantID, version.ID)
		for _, grant := range grants {
			if grant.RevokedAt == nil && grant.DecisionAt == nil {
				grant.RevokedAt = &now
				_ = s.store.RevokeReviewGrant(ctx, grant.TenantID, grant.ID, now)
			}
		}
	}
	return nil
}

func (s *Service) reviewGrant(ctx context.Context, reviewToken string) (domain.ReviewGrant, error) {
	if !strings.HasPrefix(reviewToken, "crg_") {
		return domain.ReviewGrant{}, domain.E("authentication", "review_grant", "REVIEW_TOKEN_INVALID", "审批链接无效", 3)
	}
	grant, err := s.store.ReviewGrantByTokenHash(ctx, domain.TokenHash(reviewToken))
	if err != nil || grant.RevokedAt != nil || s.now().UTC().After(grant.ExpiresAt) {
		return domain.ReviewGrant{}, domain.E("authentication", "review_grant", "REVIEW_TOKEN_INVALID", "审批链接无效、已撤销或已过期", 3)
	}
	return grant, nil
}

func (s *Service) ExportScript(ctx context.Context, actor Actor, scriptID, format, requestID string) (domain.Artifact, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.Artifact{}, err
	}
	script, err := s.store.Script(ctx, actor.TenantID, scriptID)
	if err != nil {
		return domain.Artifact{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, script.ProjectID); err != nil {
		return domain.Artifact{}, err
	}
	if script.Status != "approved" {
		return domain.Artifact{}, domain.Policy("SCRIPT_NOT_APPROVED", "只有客户批准版本可以导出", "先完成内部与客户审批")
	}
	var data []byte
	var mediaType, extension, schemaID string
	switch format {
	case "json":
		data, err = json.MarshalIndent(script.Package, "", "  ")
		mediaType, extension, schemaID = "application/json", "json", domain.ScriptPackageSchema
	case "markdown", "md":
		data = []byte(renderMarkdown(script))
		mediaType, extension, schemaID = "text/markdown", "md", domain.ArtifactExportSchemaMD
	case "xlsx":
		data, err = renderXLSX(script)
		mediaType, extension, schemaID = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx", domain.ArtifactExportSchemaXLSX
	default:
		return domain.Artifact{}, domain.Invalid("EXPORT_FORMAT_INVALID", "导出格式必须为 markdown、xlsx 或 json")
	}
	if err != nil {
		return domain.Artifact{}, err
	}
	hash := domain.TokenHash(string(data))
	derivedFrom := ""
	if _, lookupErr := s.store.Artifact(ctx, actor.TenantID, script.ID); lookupErr == nil {
		derivedFrom = script.ID
	}
	artifact := domain.Artifact{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: script.ProjectID, ScriptVersionID: script.ID, Kind: "export", CapabilityID: domain.ArtifactExportCapability, CapabilityVersion: "1.0.0", CapabilityDigest: "contentcloud-server-script-export@1", SchemaID: schemaID, MediaType: mediaType, FileName: fmt.Sprintf("script-v%d.%s", script.Version, extension), SHA256: hash, ByteSize: int64(len(data)), Visibility: "internal", RetentionClass: "project", DerivedFromArtifactID: derivedFrom, Purpose: "download", ValidationStatus: "valid", PresentationTier: "metadata_only", Metadata: map[string]any{"format": format, "script_hash": script.ContentHash, "schema_version": script.Package.SchemaVersion}, CreatedAt: s.now().UTC()}
	artifact.ObjectKey = fmt.Sprintf("tenants/%s/projects/%s/scripts/%s/exports/%s/%s", actor.TenantID, script.ProjectID, script.ID, artifact.ID, artifact.FileName)
	if err := s.blobs.Put(ctx, artifact.ObjectKey, data); err != nil {
		return artifact, err
	}
	if err := s.store.CreateArtifact(ctx, artifact); err != nil {
		return artifact, err
	}
	s.audit(ctx, actor, script.ProjectID, "script.exported", "artifact", artifact.ID, requestID, map[string]any{"script_version_id": script.ID, "format": format, "sha256": hash})
	return artifact, nil
}

func (s *Service) ExportApprovedSnapshot(ctx context.Context, actor Actor, snapshotID, scriptID, format, requestID string) (domain.Artifact, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.Artifact{}, err
	}
	snapshot, rendered, err := s.renderApprovedSnapshotScript(ctx, actor, snapshotID, scriptID)
	if err != nil {
		return domain.Artifact{}, err
	}
	file, err := renderedScriptFile(rendered, format)
	if err != nil {
		return domain.Artifact{}, err
	}
	now := s.now().UTC()
	artifact := snapshotArtifact(snapshot, rendered, file, actor.UserID, now)
	artifact.ObjectKey = fmt.Sprintf("tenants/%s/projects/%s/approved-snapshots/%s/exports/%s/%s", actor.TenantID, snapshot.ProjectID, snapshot.ID, artifact.ID, artifact.FileName)
	if err := s.blobs.Put(ctx, artifact.ObjectKey, file.Body); err != nil {
		return artifact, err
	}
	if err := s.store.CreateArtifact(ctx, artifact); err != nil {
		return artifact, err
	}
	s.audit(ctx, actor, snapshot.ProjectID, "approved_snapshot.exported", "artifact", artifact.ID, requestID, map[string]any{"approved_snapshot_id": snapshot.ID, "script_id": rendered.Package.ID, "format": file.Format, "sha256": file.SHA256})
	return artifact, nil
}

func (s *Service) CreateDeliveryPackage(ctx context.Context, actor Actor, snapshotID, scriptID, requestID string) (domain.DeliveryPackage, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.DeliveryPackage{}, err
	}
	snapshot, rendered, err := s.renderApprovedSnapshotScript(ctx, actor, snapshotID, scriptID)
	if err != nil {
		return domain.DeliveryPackage{}, err
	}
	now := s.now().UTC()
	value := domain.DeliveryPackage{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: snapshot.ProjectID, ApprovedSnapshotIDs: []string{snapshot.ID}, ScriptID: rendered.Package.ID, Status: "ready", CreatedBy: actor.UserID, CreatedAt: now}
	artifacts := make([]domain.Artifact, 0, len(rendered.Files))
	for _, file := range rendered.Files {
		artifact := snapshotArtifact(snapshot, rendered, file, actor.UserID, now)
		artifact.ObjectKey = fmt.Sprintf("tenants/%s/projects/%s/delivery-packages/%s/%s", actor.TenantID, snapshot.ProjectID, value.ID, artifact.FileName)
		if err := s.blobs.Put(ctx, artifact.ObjectKey, file.Body); err != nil {
			return domain.DeliveryPackage{}, err
		}
		artifacts = append(artifacts, artifact)
	}
	if err := s.store.CreateDeliveryPackage(ctx, value, artifacts); err != nil {
		return domain.DeliveryPackage{}, err
	}
	value.Manifest = artifacts
	s.audit(ctx, actor, snapshot.ProjectID, "delivery_package.created", "delivery_package", value.ID, requestID, map[string]any{"approved_snapshot_id": snapshot.ID, "script_id": rendered.Package.ID, "file_count": len(artifacts), "revision_hash": snapshot.ContentHash})
	return value, nil
}

func (s *Service) DeliveryPackages(ctx context.Context, actor Actor, projectID string) ([]domain.DeliveryPackage, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.DeliveryPackages(ctx, actor.TenantID, projectID)
}

func (s *Service) DeliveryPackage(ctx context.Context, actor Actor, id string) (domain.DeliveryPackage, error) {
	return s.store.DeliveryPackage(ctx, actor.TenantID, id)
}

func (s *Service) ApprovedSnapshotArtifacts(ctx context.Context, actor Actor, snapshotID string) ([]domain.Artifact, error) {
	if _, err := s.store.ApprovedSnapshot(ctx, actor.TenantID, snapshotID); err != nil {
		return nil, err
	}
	return s.store.ArtifactsByApprovedSnapshot(ctx, actor.TenantID, snapshotID)
}

func (s *Service) renderApprovedSnapshotScript(ctx context.Context, actor Actor, snapshotID, scriptID string) (domain.ApprovedSnapshot, localworkspace.RenderedScriptDelivery, error) {
	snapshot, err := s.store.ApprovedSnapshot(ctx, actor.TenantID, snapshotID)
	if err != nil {
		return snapshot, localworkspace.RenderedScriptDelivery{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, snapshot.ProjectID); err != nil {
		return snapshot, localworkspace.RenderedScriptDelivery{}, err
	}
	if snapshot.Origin != "current" || snapshot.SubmissionType != "script" || snapshot.SubmissionRevisionID == "" {
		return snapshot, localworkspace.RenderedScriptDelivery{}, domain.Policy("SNAPSHOT_NOT_DELIVERABLE", "只有当前轨道客户批准的 script ApprovedSnapshot 可生成新交付", "V1 影子快照仅用于历史读取和结果归因")
	}
	raw, err := approvedSnapshotObject(snapshot, scriptID)
	if err != nil {
		return snapshot, localworkspace.RenderedScriptDelivery{}, err
	}
	rendered, err := localworkspace.RenderScriptPackageV2(raw)
	return snapshot, rendered, err
}

func approvedSnapshotObject(snapshot domain.ApprovedSnapshot, objectID string) (json.RawMessage, error) {
	var canonical struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(snapshot.CanonicalContent, &canonical); err != nil || len(canonical.Objects) == 0 {
		return nil, domain.Invalid("APPROVED_SNAPSHOT_INVALID", "批准快照缺少 canonical objects")
	}
	eligible := map[string]bool{}
	for _, id := range snapshot.EligibleIDs {
		eligible[id] = true
	}
	matches := []json.RawMessage{}
	for _, raw := range canonical.Objects {
		var identity struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &identity) == nil && identity.ID != "" && eligible[identity.ID] && (objectID == "" || identity.ID == objectID) {
			matches = append(matches, raw)
		}
	}
	if len(matches) == 0 {
		return nil, domain.NotFound("批准快照中的 script")
	}
	if len(matches) > 1 {
		return nil, domain.Invalid("DELIVERY_SCRIPT_REQUIRED", "批准快照包含多个 script，必须明确 script_id")
	}
	return matches[0], nil
}

func renderedScriptFile(rendered localworkspace.RenderedScriptDelivery, format string) (localworkspace.RenderedScriptFile, error) {
	if format == "md" {
		format = "markdown"
	}
	for _, file := range rendered.Files {
		if file.Format == format {
			return file, nil
		}
	}
	return localworkspace.RenderedScriptFile{}, domain.Invalid("EXPORT_FORMAT_INVALID", "导出格式必须为 markdown、xlsx 或 json")
}

func snapshotArtifact(snapshot domain.ApprovedSnapshot, rendered localworkspace.RenderedScriptDelivery, file localworkspace.RenderedScriptFile, createdBy string, now time.Time) domain.Artifact {
	schemaID := domain.ArtifactExportSchemaMD
	if file.Format == "json" {
		schemaID = localworkspace.ScriptPackageV2Schema
	} else if file.Format == "xlsx" {
		schemaID = domain.ArtifactExportSchemaXLSX
	}
	return domain.Artifact{ID: domain.NewID(), TenantID: snapshot.TenantID, ProjectID: snapshot.ProjectID, ApprovedSnapshotID: snapshot.ID, Kind: "delivery", CapabilityID: domain.ArtifactExportCapability, CapabilityVersion: "2.0.0", CapabilityDigest: "contentcloud-script-delivery@2", SchemaID: schemaID, MediaType: file.MediaType, FileName: file.Name, SHA256: file.SHA256, ByteSize: int64(len(file.Body)), Visibility: "client", RetentionClass: "audit", Purpose: "delivery", ValidationStatus: "valid", PresentationTier: "cloud_native", Metadata: map[string]any{"format": file.Format, "script_id": rendered.Package.ID, "script_hash": rendered.ScriptHash, "revision_hash": snapshot.ContentHash, "approved_snapshot_id": snapshot.ID, "created_by": createdBy}, CreatedAt: now}
}

func (s *Service) Artifacts(ctx context.Context, actor Actor, scriptID string) ([]domain.Artifact, error) {
	if scriptID != "" {
		if _, err := s.store.Script(ctx, actor.TenantID, scriptID); err != nil {
			return nil, err
		}
	}
	return s.store.Artifacts(ctx, actor.TenantID, scriptID)
}

func (s *Service) ArtifactBytes(ctx context.Context, actor Actor, id string) (domain.Artifact, []byte, error) {
	artifact, err := s.store.Artifact(ctx, actor.TenantID, id)
	if err != nil {
		return artifact, nil, err
	}
	if artifact.ObjectKey == "" || artifact.ValidationStatus != "valid" {
		return artifact, nil, domain.Policy("ARTIFACT_DOWNLOAD_UNAVAILABLE", "Artifact 字节未托管或尚未通过校验", "在来源设备本机打开，或等待安全 rendition 就绪")
	}
	data, err := s.blobs.Get(ctx, artifact.ObjectKey)
	return artifact, data, err
}

func renderMarkdown(script domain.ScriptVersion) string {
	pkg := script.Package
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", pkg.Title)
	fmt.Fprintf(&b, "- 版本：%d\n- Schema：%s\n- 内容摘要：%s\n- 渠道：%s\n- 画幅：%s\n- 时长：%d 秒\n\n", script.Version, pkg.SchemaVersion, script.ContentHash, pkg.Channel, pkg.AspectRatio, pkg.TargetDurationSeconds)
	fmt.Fprintf(&b, "## 创意策略\n\n目标：%s\n\n受众：%s\n\n需求时刻：%s\n\n主卖点：%s\n\n测试变量：%s\n\n", pkg.CreativeStrategy.Objective, pkg.CreativeStrategy.Audience, pkg.CreativeStrategy.DemandMoment, pkg.CreativeStrategy.PrimarySellingPoint, pkg.CreativeStrategy.PrimaryTestVariable)
	b.WriteString("## 镜头表\n\n| 镜头 | 时码 | 功能 | 画面与动作 | 口播/字幕 | 生成约束 | 验收 |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, shot := range pkg.Shots {
		visual := shot.VisualIntent + "；" + shot.SubjectAction + "；" + shot.Composition + "；" + shot.CameraMotion
		dialogue := shot.Voiceover + " / " + shot.OnScreenText
		guards := shot.FirstFrame.PromptZH + "；" + shot.MotionSpec + "；" + shot.EndFrame.PromptZH + "；禁止：" + strings.Join(shot.NegativeConstraints, "、")
		fmt.Fprintf(&b, "| %s | %.1f-%.1fs | %s | %s | %s | %s | %s |\n", md(shot.ShotID), float64(shot.StartMS)/1000, float64(shot.EndMS)/1000, md(shot.Role), md(visual), md(dialogue), md(guards), md(strings.Join(shot.AcceptanceCriteria, "；")))
	}
	b.WriteString("\n## 引用\n\n")
	for _, citation := range pkg.Citations {
		fmt.Fprintf(&b, "- `%s` → `%s`（%s）\n", citation.KnowledgeID, citation.ShotID, citation.Usage)
	}
	return b.String()
}

func md(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}

func renderXLSX(script domain.ScriptVersion) ([]byte, error) {
	headers := []string{"镜头ID", "开始(ms)", "结束(ms)", "功能", "叙事目的", "主体", "画面意图", "主体动作", "构图", "相机运动", "首帧提示", "动态提示", "尾帧提示", "口播", "字幕", "声音", "知识引用", "可视化方案", "负面约束", "连续性", "真实性策略", "验收条件", "Plan B"}
	rows := [][]string{headers}
	for _, shot := range script.Package.Shots {
		rows = append(rows, []string{shot.ShotID, strconv.Itoa(shot.StartMS), strconv.Itoa(shot.EndMS), shot.Role, shot.NarrativePurpose, shot.Subject, shot.VisualIntent, shot.SubjectAction, shot.Composition, shot.CameraMotion, shot.FirstFrame.PromptZH, shot.MotionSpec, shot.EndFrame.PromptZH, shot.Voiceover, shot.OnScreenText, shot.SoundIntent, strings.Join(shot.KnowledgeRefs, ","), shot.VisualizationPlanID, strings.Join(shot.NegativeConstraints, "；"), shot.Continuity.IncomingState + " → " + shot.Continuity.OutgoingState, shot.ProductTruthStrategy, strings.Join(shot.AcceptanceCriteria, "；"), shot.PlanB})
	}
	return exportfmt.XLSX("镜头", rows)
}
