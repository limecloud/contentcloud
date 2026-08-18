package application

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func (s *ReviewService) ResolveReviewComment(ctx context.Context, actor Actor, id, requestID string) (reviewdomain.ReviewComment, error) {
	var err error
	actor, err = s.reviewActorWithRole(ctx, actor)
	if err != nil {
		return reviewdomain.ReviewComment{}, err
	}
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor", "reviewer"); err != nil {
		return reviewdomain.ReviewComment{}, err
	}
	target, err := s.review.ReviewComment(ctx, actor.TenantID, id)
	if err != nil {
		return reviewdomain.ReviewComment{}, err
	}
	if target.SubjectType != "submission_revision" {
		return reviewdomain.ReviewComment{}, fault.NotFound("提交内容版本批注")
	}
	revision, err := s.review.SubmissionRevision(ctx, actor.TenantID, target.SubjectID)
	if err != nil || revision.ProjectID != target.ProjectID {
		return reviewdomain.ReviewComment{}, fault.NotFound("提交内容版本批注")
	}
	if _, err := s.app.Identity.projectForWrite(ctx, actor, target.ProjectID); err != nil {
		return reviewdomain.ReviewComment{}, err
	}
	if target.ResolvedAt != nil {
		return target, nil
	}
	now := s.now().UTC()
	target.ResolvedAt = &now
	if err := s.review.SaveReviewComment(ctx, target); err != nil {
		return target, err
	}
	s.audit(ctx, actor, target.ProjectID, "review_comment.resolved", "review_comment", target.ID, requestID, map[string]any{"submission_revision_id": target.SubjectID})
	return target, nil
}

type ReviewProjection struct {
	Project    workspacedomain.Project      `json:"project"`
	Submission *SubmissionReviewSubject     `json:"submission,omitempty"`
	Comments   []reviewdomain.ReviewComment `json:"comments"`
	Verified   bool                         `json:"verified"`
}

type SubmissionReviewSubject struct {
	SubmissionID         string                             `json:"submission_id"`
	SubmissionRevisionID string                             `json:"submission_revision_id"`
	SubjectHash          string                             `json:"subject_hash"`
	SchemaVersion        string                             `json:"schema_version"`
	BaseSnapshotIDs      []string                           `json:"base_snapshot_ids"`
	EnvironmentDigest    string                             `json:"environment_digest"`
	ObjectRefs           []reviewdomain.SubmissionObjectRef `json:"object_refs"`
	Objects              []json.RawMessage                  `json:"objects"`
}

type ReviewDecisionResult struct {
	SubjectType      string                         `json:"subject_type"`
	SubjectID        string                         `json:"subject_id"`
	Status           string                         `json:"status"`
	ApprovedSnapshot *reviewdomain.ApprovedSnapshot `json:"approved_snapshot,omitempty"`
}

type SubmissionReviewStatus struct {
	Submission reviewdomain.Submission         `json:"submission"`
	Revision   reviewdomain.SubmissionRevision `json:"revision"`
	Grants     []reviewdomain.ReviewGrant      `json:"grants"`
}

func (s *ReviewService) CreateReviewGrant(ctx context.Context, actor Actor, revisionID, reviewerEmail, requestID string) (reviewdomain.ReviewGrant, error) {
	if !canManage(actor.Role) {
		return reviewdomain.ReviewGrant{}, fault.Policy("ROLE_DENIED", "当前角色不能创建客户审批", "联系项目负责人")
	}
	revision, err := s.review.SubmissionRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return reviewdomain.ReviewGrant{}, err
	}
	submission, err := s.review.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil {
		return reviewdomain.ReviewGrant{}, err
	}
	if _, err := s.app.Identity.projectForWrite(ctx, actor, revision.ProjectID); err != nil {
		return reviewdomain.ReviewGrant{}, err
	}
	if submission.SubmissionType != "content_batch" {
		return reviewdomain.ReviewGrant{}, fault.Invalid("REVIEW_SUBJECT_INVALID", "客户审批只接受内容批次（content_batch）的提交内容版本")
	}
	if submission.CurrentRevisionID != revision.ID || (submission.Status != "internally_approved" && submission.Status != "client_review") {
		return reviewdomain.ReviewGrant{}, fault.Conflict("SUBMISSION_STATE_INVALID", "只有当前已通过内审的内容批次（content_batch）版本可发起客户审批")
	}
	if err := s.requireInternalSubmissionApproval(ctx, revision); err != nil {
		return reviewdomain.ReviewGrant{}, err
	}
	if !strings.Contains(reviewerEmail, "@") {
		return reviewdomain.ReviewGrant{}, fault.Invalid("REVIEWER_EMAIL_INVALID", "客户审批邮箱无效")
	}
	plain, tokenHash, err := idgen.NewOpaqueToken("crg_", 32)
	if err != nil {
		return reviewdomain.ReviewGrant{}, err
	}
	otp, err := newReviewOTP()
	if err != nil {
		return reviewdomain.ReviewGrant{}, err
	}
	now := s.now().UTC()
	v := reviewdomain.ReviewGrant{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, ReviewerEmail: strings.ToLower(strings.TrimSpace(reviewerEmail)), TokenHash: tokenHash, OTPHash: idgen.TokenHash(otp), ExpiresAt: now.Add(7 * 24 * time.Hour), CreatedAt: now, PlaintextToken: plain, PlaintextOTP: otp}
	stored := v
	stored.PlaintextToken = ""
	stored.PlaintextOTP = ""
	submission.Status = "client_review"
	submission.UpdatedAt = now
	if err := s.review.CreateSubmissionReviewGrant(ctx, submission, stored); err != nil {
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

func (s *ReviewService) ReviewGrants(ctx context.Context, actor Actor, revisionID string) ([]reviewdomain.ReviewGrant, error) {
	if _, err := s.review.SubmissionRevision(ctx, actor.TenantID, revisionID); err != nil {
		return nil, err
	}
	return s.review.ReviewGrants(ctx, actor.TenantID, revisionID)
}

func (s *ReviewService) SubmissionReviewStatus(ctx context.Context, actor Actor, revisionID string) (SubmissionReviewStatus, error) {
	revision, err := s.review.SubmissionRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return SubmissionReviewStatus{}, err
	}
	submission, err := s.review.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil {
		return SubmissionReviewStatus{}, err
	}
	grants, err := s.review.ReviewGrants(ctx, actor.TenantID, revision.ID)
	if err != nil {
		return SubmissionReviewStatus{}, err
	}
	return SubmissionReviewStatus{Submission: submission, Revision: revision, Grants: grants}, nil
}

func (s *ReviewService) RevokeReviewGrant(ctx context.Context, actor Actor, grantID, requestID string) (reviewdomain.ReviewGrant, error) {
	if !canManage(actor.Role) {
		return reviewdomain.ReviewGrant{}, fault.Policy("ROLE_DENIED", "当前角色不能撤销客户审批", "联系项目负责人")
	}
	grant, err := s.review.ReviewGrant(ctx, actor.TenantID, grantID)
	if err != nil {
		return grant, err
	}
	if _, err := s.app.Identity.projectForWrite(ctx, actor, grant.ProjectID); err != nil {
		return grant, err
	}
	if grant.RevokedAt != nil {
		grant.TokenHash = ""
		grant.OTPHash = ""
		return grant, nil
	}
	now := s.now().UTC()
	grant.RevokedAt = &now
	if err := s.review.RevokeReviewGrant(ctx, grant.TenantID, grant.ID, now); err != nil {
		return grant, err
	}
	s.audit(ctx, actor, grant.ProjectID, "review_grant.revoked", "review_grant", grant.ID, requestID, map[string]any{"subject_type": grant.SubjectType, "subject_id": grant.SubjectID})
	grant.TokenHash = ""
	grant.OTPHash = ""
	return grant, nil
}

func (s *ReviewService) ReviewProjection(ctx context.Context, reviewToken string) (ReviewProjection, error) {
	grant, err := s.reviewGrant(ctx, reviewToken)
	if err != nil {
		return ReviewProjection{}, err
	}
	project, err := s.workspace.Project(ctx, grant.TenantID, grant.ProjectID)
	if err != nil {
		return ReviewProjection{}, err
	}
	if grant.VerifiedAt == nil {
		return ReviewProjection{Project: workspacedomain.Project{ID: project.ID, BrandName: project.BrandName, ProductName: project.ProductName}, Verified: false}, nil
	}
	revision, err := s.review.SubmissionRevision(ctx, grant.TenantID, grant.SubjectID)
	if err != nil || revision.ContentHash != grant.SubjectHash {
		return ReviewProjection{}, fault.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效")
	}
	submission, err := s.review.Submission(ctx, grant.TenantID, revision.SubmissionID)
	if err != nil || submission.CurrentRevisionID != revision.ID || submission.Status != "client_review" {
		return ReviewProjection{}, fault.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效或状态已变化")
	}
	comments, _ := s.review.ReviewComments(ctx, grant.TenantID, revision.ID)
	subject := &SubmissionReviewSubject{SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, SubjectHash: revision.ContentHash, SchemaVersion: revision.SchemaVersion, BaseSnapshotIDs: revision.BaseSnapshotIDs, EnvironmentDigest: revision.EnvironmentDigest, ObjectRefs: revision.Objects, Objects: submissionObjectContents(revision.Objects)}
	return ReviewProjection{Project: project, Submission: subject, Comments: clientVisibleComments(comments), Verified: true}, nil
}

func clientVisibleComments(comments []reviewdomain.ReviewComment) []reviewdomain.ReviewComment {
	publicComments := []reviewdomain.ReviewComment{}
	for _, comment := range comments {
		if comment.Visibility == "client" {
			publicComments = append(publicComments, comment)
		}
	}
	return publicComments
}

func (s *ReviewService) VerifyReviewGrant(ctx context.Context, reviewToken, otp string) (ReviewProjection, error) {
	grant, err := s.reviewGrant(ctx, reviewToken)
	if err != nil {
		return ReviewProjection{}, err
	}
	if subtle.ConstantTimeCompare([]byte(grant.OTPHash), []byte(idgen.TokenHash(strings.TrimSpace(otp)))) != 1 {
		return ReviewProjection{}, fault.E("authentication", "review_otp", "REVIEW_OTP_INVALID", "验证码错误", 3)
	}
	now := s.now().UTC()
	grant.VerifiedAt = &now
	if err := s.review.MarkReviewGrantVerified(ctx, grant.TenantID, grant.ID, now); err != nil {
		return ReviewProjection{}, err
	}
	return s.ReviewProjection(ctx, reviewToken)
}

func (s *ReviewService) DecideReviewGrant(ctx context.Context, reviewToken, decision, reason, shotID, requestID string) (ReviewDecisionResult, error) {
	grant, err := s.reviewGrant(ctx, reviewToken)
	if err != nil {
		return ReviewDecisionResult{}, err
	}
	if grant.VerifiedAt == nil {
		return ReviewDecisionResult{}, fault.E("authentication", "review_otp", "REVIEW_VERIFICATION_REQUIRED", "请先完成邮箱验证码验证", 3)
	}
	if grant.DecisionAt != nil {
		return ReviewDecisionResult{}, fault.Conflict("REVIEW_ALREADY_DECIDED", "该审批链接已完成最终决策")
	}
	return s.decideSubmissionReviewGrant(ctx, grant, decision, reason, shotID, requestID)
}

func (s *ReviewService) decideSubmissionReviewGrant(ctx context.Context, grant reviewdomain.ReviewGrant, decision, reason, shotID, requestID string) (ReviewDecisionResult, error) {
	revision, err := s.review.SubmissionRevision(ctx, grant.TenantID, grant.SubjectID)
	if err != nil || revision.ContentHash != grant.SubjectHash {
		return ReviewDecisionResult{}, fault.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效")
	}
	submission, err := s.review.Submission(ctx, grant.TenantID, revision.SubmissionID)
	if err != nil || submission.CurrentRevisionID != revision.ID || submission.Status != "client_review" || submission.SubmissionType != "content_batch" {
		return ReviewDecisionResult{}, fault.Conflict("REVIEW_SUBJECT_CHANGED", "审批对象已失效或状态已变化")
	}
	if err := s.requireInternalSubmissionApproval(ctx, revision); err != nil {
		return ReviewDecisionResult{}, err
	}
	now := s.now().UTC()
	previous := submission.Status
	var comment *reviewdomain.ReviewComment
	var snapshot *reviewdomain.ApprovedSnapshot
	switch decision {
	case "approve":
		if err := s.validateTenantSubmissionContentTypes(ctx, grant.TenantID, submission.SubmissionType, revision.ProjectID, revision.Objects); err != nil {
			return ReviewDecisionResult{}, err
		}
		if err := validateGovernedSubmissionObjects(submission.SubmissionType, revision.ProjectID, revision.BaseSnapshotIDs, revision.Objects, now); err != nil {
			return ReviewDecisionResult{}, err
		}
		baseSnapshots, err := s.loadSubmissionBaseSnapshots(ctx, grant.TenantID, revision.ProjectID, revision.BaseSnapshotIDs)
		if err != nil {
			return ReviewDecisionResult{}, err
		}
		if err := validateGovernedBaseSnapshotTypes(submission.SubmissionType, revision.Objects, baseSnapshots, now); err != nil {
			return ReviewDecisionResult{}, err
		}
		if err := s.requireResolvedComments(ctx, grant.TenantID, revision.ID, "client"); err != nil {
			return ReviewDecisionResult{}, err
		}
		canonical, err := canonicalSubmissionContent(submission, revision)
		if err != nil {
			return ReviewDecisionResult{}, err
		}
		submission.Status = "approved"
		snapshot = &reviewdomain.ApprovedSnapshot{ID: idgen.New(), TenantID: grant.TenantID, ProjectID: revision.ProjectID, WorkspaceID: revision.WorkspaceID, SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, SubmissionType: submission.SubmissionType, SchemaVersion: revision.SchemaVersion, ContentHash: revision.ContentHash, SubjectHash: revision.ContentHash, CanonicalContent: canonical, EligibleIDs: revision.EligibleObjectIDs(), Artifacts: revision.Artifacts, CreatedBy: "client:" + grant.ReviewerEmail, CreatedAt: now}
	case "return":
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return ReviewDecisionResult{}, fault.Invalid("REVIEW_REASON_REQUIRED", "退回修改必须填写原因")
		}
		submission.Status = "changes_requested"
		cycleID := s.submissionReviewCycleID(ctx, grant.TenantID, revision.ID)
		comment = &reviewdomain.ReviewComment{ID: idgen.New(), TenantID: grant.TenantID, ProjectID: grant.ProjectID, ReviewCycleID: cycleID, SubjectType: "submission_revision", SubjectID: revision.ID, ShotID: shotID, Body: reason, Visibility: "client", AuthorID: "client:" + grant.ReviewerEmail, CreatedAt: now}
	default:
		return ReviewDecisionResult{}, fault.Invalid("DECISION_INVALID", "客户审批决策无效")
	}
	approval := reviewdomain.ApprovalDecision{ID: idgen.New(), TenantID: grant.TenantID, ProjectID: grant.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, DecisionStage: "client", ActorID: "client:" + grant.ReviewerEmail, Decision: decision, Reason: reason, PreviousState: previous, ResultingState: submission.Status, CreatedAt: now}
	if snapshot != nil {
		snapshot.DecisionID = approval.ID
	}
	submission.UpdatedAt = now
	grant.DecisionAt = &now
	if err := s.review.CompleteSubmissionClientReview(ctx, submission, grant, approval, comment, snapshot); err != nil {
		return ReviewDecisionResult{}, err
	}
	s.audit(ctx, Actor{UserID: "client:" + grant.ReviewerEmail, TenantID: grant.TenantID, Type: "client"}, grant.ProjectID, "submission.client_reviewed", "submission_revision", revision.ID, requestID, map[string]any{"decision": decision, "to": submission.Status, "snapshot_id": valueOrEmptySnapshotID(snapshot)})
	return ReviewDecisionResult{SubjectType: "submission_revision", SubjectID: revision.ID, Status: submission.Status, ApprovedSnapshot: snapshot}, nil
}

func (s *ReviewService) requireInternalSubmissionApproval(ctx context.Context, revision reviewdomain.SubmissionRevision) error {
	decisions, err := s.review.Approvals(ctx, revision.TenantID, revision.ID)
	if err != nil {
		return err
	}
	for _, decision := range decisions {
		if decision.SubjectType == "submission_revision" && decision.SubjectHash == revision.ContentHash && decision.DecisionStage == "internal" && decision.Decision == "approve" {
			return nil
		}
	}
	return fault.Policy("INTERNAL_APPROVAL_REQUIRED", "客户审批前必须完成同一内容版本的内部批准", "先完成提交内容版本的内部审核")
}

func (s *ReviewService) submissionReviewCycleID(ctx context.Context, tenantID, revisionID string) string {
	cycles, err := s.review.ReviewCycles(ctx, tenantID, revisionID)
	if err == nil && len(cycles) > 0 {
		return cycles[0].ID
	}
	return ""
}

func valueOrEmptySnapshotID(snapshot *reviewdomain.ApprovedSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return snapshot.ID
}

func (s *ReviewService) reviewGrant(ctx context.Context, reviewToken string) (reviewdomain.ReviewGrant, error) {
	if !strings.HasPrefix(reviewToken, "crg_") {
		return reviewdomain.ReviewGrant{}, fault.E("authentication", "review_grant", "REVIEW_TOKEN_INVALID", "审批链接无效", 3)
	}
	grant, err := s.review.ReviewGrantByTokenHash(ctx, idgen.TokenHash(reviewToken))
	if err != nil || grant.SubjectType != "submission_revision" || grant.RevokedAt != nil || s.now().UTC().After(grant.ExpiresAt) {
		return reviewdomain.ReviewGrant{}, fault.E("authentication", "review_grant", "REVIEW_TOKEN_INVALID", "审批链接无效、已撤销或已过期", 3)
	}
	return grant, nil
}

func (s *ReviewService) ExportApprovedSnapshot(ctx context.Context, actor Actor, snapshotID, contentItemID, format, requestID string) (deliverydomain.Artifact, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return deliverydomain.Artifact{}, err
	}
	snapshot, rendered, err := s.renderApprovedSnapshotContentItem(ctx, actor, snapshotID, contentItemID)
	if err != nil {
		return deliverydomain.Artifact{}, err
	}
	file, err := renderedContentFile(rendered, format)
	if err != nil {
		return deliverydomain.Artifact{}, err
	}
	now := s.now().UTC()
	artifact := snapshotArtifact(snapshot, rendered, file, actor.UserID, now)
	artifact.ObjectKey = fmt.Sprintf("tenants/%s/projects/%s/approved-snapshots/%s/exports/%s/%s", actor.TenantID, snapshot.ProjectID, snapshot.ID, artifact.ID, artifact.FileName)
	if err := s.blobs.Put(ctx, artifact.ObjectKey, file.Body); err != nil {
		return artifact, err
	}
	if err := s.artifacts.CreateArtifact(ctx, artifact); err != nil {
		return artifact, err
	}
	s.audit(ctx, actor, snapshot.ProjectID, "approved_snapshot.exported", "artifact", artifact.ID, requestID, map[string]any{"approved_snapshot_id": snapshot.ID, "content_item_id": rendered.Item.ID, "format": file.Format, "sha256": file.SHA256})
	return artifact, nil
}

func (s *ReviewService) CreateDeliveryPackage(ctx context.Context, actor Actor, snapshotID, contentItemID, requestID string) (deliverydomain.DeliveryPackage, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return deliverydomain.DeliveryPackage{}, err
	}
	snapshot, rendered, err := s.renderApprovedSnapshotContentItem(ctx, actor, snapshotID, contentItemID)
	if err != nil {
		return deliverydomain.DeliveryPackage{}, err
	}
	now := s.now().UTC()
	value := deliverydomain.DeliveryPackage{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: snapshot.ProjectID, ApprovedSnapshotIDs: []string{snapshot.ID}, ContentItemID: rendered.Item.ID, Status: "ready", CreatedBy: actor.UserID, CreatedAt: now}
	artifacts := make([]deliverydomain.Artifact, 0, len(rendered.Files))
	for _, file := range rendered.Files {
		artifact := snapshotArtifact(snapshot, rendered, file, actor.UserID, now)
		artifact.ObjectKey = fmt.Sprintf("tenants/%s/projects/%s/delivery-packages/%s/%s", actor.TenantID, snapshot.ProjectID, value.ID, artifact.FileName)
		if err := s.blobs.Put(ctx, artifact.ObjectKey, file.Body); err != nil {
			return deliverydomain.DeliveryPackage{}, err
		}
		artifacts = append(artifacts, artifact)
	}
	if err := s.artifacts.CreateDeliveryPackage(ctx, value, artifacts); err != nil {
		return deliverydomain.DeliveryPackage{}, err
	}
	value.Manifest = artifacts
	s.audit(ctx, actor, snapshot.ProjectID, "delivery_package.created", "delivery_package", value.ID, requestID, map[string]any{"approved_snapshot_id": snapshot.ID, "content_item_id": rendered.Item.ID, "file_count": len(artifacts), "revision_hash": snapshot.ContentHash})
	return value, nil
}

func (s *ReviewService) DeliveryPackages(ctx context.Context, actor Actor, projectID string) ([]deliverydomain.DeliveryPackage, error) {
	if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.artifacts.DeliveryPackages(ctx, actor.TenantID, projectID)
}

func (s *ReviewService) DeliveryPackage(ctx context.Context, actor Actor, id string) (deliverydomain.DeliveryPackage, error) {
	return s.artifacts.DeliveryPackage(ctx, actor.TenantID, id)
}

func (s *ReviewService) ApprovedSnapshotArtifacts(ctx context.Context, actor Actor, snapshotID string) ([]deliverydomain.Artifact, error) {
	if _, err := s.review.ApprovedSnapshot(ctx, actor.TenantID, snapshotID); err != nil {
		return nil, err
	}
	return s.artifacts.ArtifactsByApprovedSnapshot(ctx, actor.TenantID, snapshotID)
}

func (s *ReviewService) renderApprovedSnapshotContentItem(ctx context.Context, actor Actor, snapshotID, contentItemID string) (reviewdomain.ApprovedSnapshot, localworkspace.RenderedContentDelivery, error) {
	snapshot, err := s.review.ApprovedSnapshot(ctx, actor.TenantID, snapshotID)
	if err != nil {
		return snapshot, localworkspace.RenderedContentDelivery{}, err
	}
	if _, err := s.app.Identity.projectForWrite(ctx, actor, snapshot.ProjectID); err != nil {
		return snapshot, localworkspace.RenderedContentDelivery{}, err
	}
	if snapshot.SubmissionType != "content_batch" || snapshot.SubmissionRevisionID == "" {
		return snapshot, localworkspace.RenderedContentDelivery{}, fault.Policy("SNAPSHOT_NOT_DELIVERABLE", "只有当前流程中经客户批准的内容批次快照才能生成新交付", "非当前快照仅用于历史读取和结果归因")
	}
	raw, err := approvedSnapshotObject(snapshot, contentItemID)
	if err != nil {
		return snapshot, localworkspace.RenderedContentDelivery{}, err
	}
	rendered, err := localworkspace.RenderContentItem(raw)
	return snapshot, rendered, err
}

func approvedSnapshotObject(snapshot reviewdomain.ApprovedSnapshot, objectID string) (json.RawMessage, error) {
	var canonical struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(snapshot.CanonicalContent, &canonical); err != nil || len(canonical.Objects) == 0 {
		return nil, fault.Invalid("APPROVED_SNAPSHOT_INVALID", "批准快照缺少规范对象列表")
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
		return nil, fault.NotFound("批准快照中的内容项")
	}
	if len(matches) > 1 {
		return nil, fault.Invalid("DELIVERY_CONTENT_ITEM_REQUIRED", "批准快照包含多个内容项，必须明确内容项标识（content_item_id）")
	}
	return matches[0], nil
}

func renderedContentFile(rendered localworkspace.RenderedContentDelivery, format string) (localworkspace.RenderedContentFile, error) {
	if format == "md" {
		format = "markdown"
	}
	for _, file := range rendered.Files {
		if file.Format == format {
			return file, nil
		}
	}
	return localworkspace.RenderedContentFile{}, fault.Invalid("EXPORT_FORMAT_INVALID", "导出格式必须为 Markdown、XLSX 或 JSON")
}

func snapshotArtifact(snapshot reviewdomain.ApprovedSnapshot, rendered localworkspace.RenderedContentDelivery, file localworkspace.RenderedContentFile, createdBy string, now time.Time) deliverydomain.Artifact {
	schemaID := sourcedomain.ArtifactExportSchemaMD
	if file.Format == "json" {
		schemaID = localworkspace.ContentItemSchema
	} else if file.Format == "xlsx" {
		schemaID = sourcedomain.ArtifactExportSchemaXLSX
	}
	return deliverydomain.Artifact{ID: idgen.New(), TenantID: snapshot.TenantID, ProjectID: snapshot.ProjectID, ApprovedSnapshotID: snapshot.ID, Kind: "delivery", CapabilityID: sourcedomain.ArtifactExportCapability, CapabilityVersion: "3.0.0", CapabilityDigest: "contentcloud-content-delivery@3", SchemaID: schemaID, MediaType: file.MediaType, FileName: file.Name, SHA256: file.SHA256, ByteSize: int64(len(file.Body)), Visibility: "client", RetentionClass: "audit", Purpose: "delivery", Metadata: map[string]any{"format": file.Format, "content_item_id": rendered.Item.ID, "content_hash": rendered.ContentHash, "revision_hash": snapshot.ContentHash, "approved_snapshot_id": snapshot.ID, "created_by": createdBy}, CreatedAt: now}
}

func (s *ReviewService) ArtifactBytes(ctx context.Context, actor Actor, id string) (deliverydomain.Artifact, []byte, error) {
	artifact, err := s.artifacts.Artifact(ctx, actor.TenantID, id)
	if err != nil {
		return artifact, nil, err
	}
	if artifact.ObjectKey == "" {
		return artifact, nil, fault.Policy("ARTIFACT_DOWNLOAD_UNAVAILABLE", "成果文件尚未托管", "重新生成批准快照的交付文件")
	}
	data, err := s.blobs.Get(ctx, artifact.ObjectKey)
	return artifact, data, err
}
