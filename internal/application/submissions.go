package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"

	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type SubmissionDetails struct {
	Submission reviewdomain.Submission           `json:"submission"`
	Revisions  []reviewdomain.SubmissionRevision `json:"revisions"`
	Comments   []reviewdomain.ReviewComment      `json:"comments"`
}

type SubmissionRevisionView struct {
	Submission reviewdomain.Submission         `json:"submission"`
	Revision   reviewdomain.SubmissionRevision `json:"revision"`
	Comments   []reviewdomain.ReviewComment    `json:"comments"`
}

type SubmissionApprovalResult struct {
	Submission       reviewdomain.Submission        `json:"submission"`
	Decision         reviewdomain.ApprovalDecision  `json:"decision"`
	ApprovedSnapshot *reviewdomain.ApprovedSnapshot `json:"approved_snapshot,omitempty"`
}

func (s *ReviewService) RegisterWorkspace(ctx context.Context, actor Actor, binding workspacedomain.WorkspaceBinding, templateID, templateVersion string, targets []string, requestID string) (workspacedomain.WorkspaceBinding, error) {
	if actor.Type != "workspace" || actor.WorkspaceID != binding.ID {
		return binding, fault.Policy("WORKSPACE_SCOPE_DENIED", "工作区凭据与绑定不匹配", "重新执行项目初始化")
	}
	if strings.TrimSpace(templateID) == "" || strings.TrimSpace(templateVersion) == "" {
		return binding, fault.Invalid("WORKSPACE_TEMPLATE_REQUIRED", "工作区模板标识（template_id）和模板版本（template_version）必填")
	}
	normalizedTargets := make([]string, 0, len(targets))
	seenTargets := map[agentadapter.ClientID]struct{}{}
	for _, target := range targets {
		// codex-plugin 是早期 Bootstrap 的分发模式，不是独立客户端。
		if strings.EqualFold(strings.TrimSpace(target), "codex-plugin") {
			target = string(agentadapter.ClientCodex)
		}
		client, err := agentadapter.RequireCapability(target, agentadapter.CapabilityWorkspaceRegister)
		if err != nil {
			return binding, err
		}
		if _, exists := seenTargets[client.ID]; exists {
			continue
		}
		seenTargets[client.ID] = struct{}{}
		normalizedTargets = append(normalizedTargets, string(client.ID))
	}
	binding.TemplateID = templateID
	binding.TemplateVersion = templateVersion
	binding.Targets = normalizedTargets
	binding.LastSeenAt = s.now().UTC()
	if err := s.workspace.SaveWorkspaceBinding(ctx, binding); err != nil {
		return binding, err
	}
	if binding.DeviceID != "" {
		device, err := s.workspace.Device(ctx, binding.TenantID, binding.DeviceID)
		if err != nil {
			return binding, err
		}
		device.LastSeenAt = binding.LastSeenAt
		if err := s.workspace.SaveDevice(ctx, device); err != nil {
			return binding, err
		}
	}
	binding.CredentialHash = ""
	s.audit(ctx, actor, binding.ProjectID, "workspace.registered", "workspace_binding", binding.ID, requestID, map[string]any{"template_version": templateVersion, "targets": normalizedTargets})
	return binding, nil
}

func (s *ReviewService) CreateSubmission(ctx context.Context, actor Actor, binding workspacedomain.WorkspaceBinding, bundle reviewdomain.SubmissionBundle, requestID string) (reviewdomain.SubmissionRevision, error) {
	if actor.Type != "workspace" || actor.WorkspaceID != binding.ID || bundle.WorkspaceID != binding.ID || bundle.ProjectID != binding.ProjectID {
		return reviewdomain.SubmissionRevision{}, fault.Policy("WORKSPACE_SCOPE_DENIED", "提交不属于当前工作区和项目", "检查本地 .contentcloud/workspace.yaml 后重试")
	}
	if err := bundle.Validate(); err != nil {
		return reviewdomain.SubmissionRevision{}, err
	}
	if err := s.validateTenantSubmissionContentTypes(ctx, binding.TenantID, bundle.SubmissionType, bundle.ProjectID, bundle.Objects); err != nil {
		return reviewdomain.SubmissionRevision{}, err
	}
	now := s.now().UTC()
	if err := validateGovernedSubmissionObjects(bundle.SubmissionType, bundle.ProjectID, bundle.BaseSnapshotIDs, bundle.Objects, now); err != nil {
		return reviewdomain.SubmissionRevision{}, err
	}
	if _, err := s.workspace.Project(ctx, binding.TenantID, binding.ProjectID); err != nil {
		return reviewdomain.SubmissionRevision{}, err
	}
	baseSnapshots, err := s.loadSubmissionBaseSnapshots(ctx, binding.TenantID, binding.ProjectID, bundle.BaseSnapshotIDs)
	if err != nil {
		return reviewdomain.SubmissionRevision{}, err
	}
	if err := validateGovernedBaseSnapshotTypes(bundle.SubmissionType, bundle.Objects, baseSnapshots, now); err != nil {
		return reviewdomain.SubmissionRevision{}, err
	}
	submission, err := s.review.SubmissionByWorkspaceType(ctx, binding.TenantID, binding.ProjectID, binding.ID, bundle.SubmissionType)
	if err != nil && !isNotFound(err) {
		return reviewdomain.SubmissionRevision{}, err
	}
	revisionNo := 1
	if isNotFound(err) {
		submission = reviewdomain.Submission{ID: idgen.New(), TenantID: binding.TenantID, ProjectID: binding.ProjectID, WorkspaceID: binding.ID, SubmissionType: bundle.SubmissionType, Status: "preparing", CreatedBy: binding.ID, CreatedAt: now, UpdatedAt: now}
	} else {
		revisions, err := s.review.SubmissionRevisions(ctx, binding.TenantID, submission.ID)
		if err != nil {
			return reviewdomain.SubmissionRevision{}, err
		}
		for _, existing := range revisions {
			if existing.IdempotencyKey == bundle.IdempotencyKey {
				if existing.ContentHash != normalizeSubmissionHash(bundle.ContentHash) {
					return reviewdomain.SubmissionRevision{}, fault.Conflict("IDEMPOTENCY_CONTENT_MISMATCH", "相同幂等键不能提交不同内容")
				}
				return existing, nil
			}
			if existing.RevisionNo >= revisionNo {
				revisionNo = existing.RevisionNo + 1
			}
		}
	}
	disclosures := append([]reviewdomain.SourceDisclosure(nil), bundle.SourceDisclosures...)
	for index := range disclosures {
		disclosures[index].ID = idgen.New()
		disclosures[index].TenantID = binding.TenantID
		disclosures[index].ProjectID = binding.ProjectID
		disclosures[index].CreatedAt = now
	}
	revision := reviewdomain.SubmissionRevision{
		ID: idgen.New(), TenantID: binding.TenantID, ProjectID: binding.ProjectID, WorkspaceID: binding.ID, SubmissionID: submission.ID,
		RevisionNo: revisionNo, SchemaVersion: reviewdomain.SubmissionSchemaVersion(bundle.SubmissionType), ContentHash: normalizeSubmissionHash(bundle.ContentHash), BaseSnapshotIDs: append([]string{}, bundle.BaseSnapshotIDs...), EnvironmentDigest: bundle.EnvironmentDigest,
		LocalRunSummary: bundle.LocalRunSummary, Objects: cloneSubmissionObjects(bundle.Objects), Artifacts: append([]reviewdomain.SubmissionArtifact{}, bundle.Artifacts...), Message: strings.TrimSpace(bundle.Message),
		IdempotencyKey: bundle.IdempotencyKey, EvidenceLimited: reviewdomain.EvidenceLimited(bundle.Objects, disclosures), CreatedBy: binding.ID, CreatedAt: now, SourceDisclosures: disclosures,
	}
	submission.Status = "submitted"
	submission.CurrentRevisionID = revision.ID
	submission.UpdatedAt = now
	cycle := reviewdomain.ReviewCycle{ID: idgen.New(), TenantID: binding.TenantID, ProjectID: binding.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, Status: "open", OpenedBy: binding.ID, OpenedAt: now, CreatedAt: now}
	if err := s.review.CreateSubmissionRevision(ctx, submission, revision, disclosures, cycle); err != nil {
		return reviewdomain.SubmissionRevision{}, err
	}
	s.audit(ctx, actor, binding.ProjectID, "submission.published", "submission_revision", revision.ID, requestID, map[string]any{"submission_id": submission.ID, "type": submission.SubmissionType, "revision_no": revision.RevisionNo, "content_hash": revision.ContentHash, "evidence_limited": revision.EvidenceLimited})
	return revision, nil
}

func (s *ReviewService) Submissions(ctx context.Context, actor Actor, projectID string) ([]reviewdomain.Submission, error) {
	if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.review.Submissions(ctx, actor.TenantID, projectID)
}

func (s *ReviewService) WorkspaceSubmissions(ctx context.Context, actor Actor, binding workspacedomain.WorkspaceBinding) ([]reviewdomain.Submission, error) {
	if actor.WorkspaceID != binding.ID {
		return nil, fault.Policy("WORKSPACE_SCOPE_DENIED", "工作区范围不匹配", "重新初始化工作区")
	}
	values, err := s.review.Submissions(ctx, actor.TenantID, binding.ProjectID)
	if err != nil {
		return nil, err
	}
	filtered := make([]reviewdomain.Submission, 0, len(values))
	for _, value := range values {
		if value.WorkspaceID == binding.ID {
			filtered = append(filtered, value)
		}
	}
	return filtered, nil
}

func (s *ReviewService) SubmissionDetails(ctx context.Context, actor Actor, id string) (SubmissionDetails, error) {
	submission, err := s.review.Submission(ctx, actor.TenantID, id)
	if err != nil {
		return SubmissionDetails{}, err
	}
	if actor.Type == "workspace" && submission.WorkspaceID != actor.WorkspaceID {
		return SubmissionDetails{}, fault.NotFound("提交记录")
	}
	revisions, err := s.review.SubmissionRevisions(ctx, actor.TenantID, submission.ID)
	if err != nil {
		return SubmissionDetails{}, err
	}
	comments := []reviewdomain.ReviewComment{}
	for _, revision := range revisions {
		values, err := s.review.ReviewComments(ctx, actor.TenantID, revision.ID)
		if err != nil {
			return SubmissionDetails{}, err
		}
		comments = append(comments, values...)
	}
	return SubmissionDetails{Submission: submission, Revisions: revisions, Comments: comments}, nil
}

func (s *ReviewService) ProjectSubmissionRevision(ctx context.Context, actor Actor, projectID, revisionID string) (SubmissionRevisionView, error) {
	if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
		return SubmissionRevisionView{}, err
	}
	revision, err := s.review.SubmissionRevision(ctx, actor.TenantID, revisionID)
	if err != nil || revision.ProjectID != projectID {
		if err == nil {
			err = fault.NotFound("提交内容版本")
		}
		return SubmissionRevisionView{}, err
	}
	submission, err := s.review.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil || submission.ProjectID != projectID {
		if err == nil {
			err = fault.NotFound("提交记录")
		}
		return SubmissionRevisionView{}, err
	}
	if actor.Type == "workspace" && submission.WorkspaceID != actor.WorkspaceID {
		return SubmissionRevisionView{}, fault.NotFound("提交内容版本")
	}
	comments, err := s.review.ReviewComments(ctx, actor.TenantID, revision.ID)
	if err != nil {
		return SubmissionRevisionView{}, err
	}
	return SubmissionRevisionView{Submission: submission, Revision: revision, Comments: comments}, nil
}

func (s *ReviewService) ApproveSubmission(ctx context.Context, actor Actor, revisionID, reason, requestID string) (SubmissionApprovalResult, error) {
	var err error
	actor, err = s.reviewActorWithRole(ctx, actor)
	if err != nil {
		return SubmissionApprovalResult{}, err
	}
	if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer"); err != nil {
		return SubmissionApprovalResult{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return SubmissionApprovalResult{}, fault.Invalid("APPROVAL_REASON_REQUIRED", "批准必须填写整版结论")
	}
	revision, err := s.review.SubmissionRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return SubmissionApprovalResult{}, err
	}
	submission, err := s.review.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil {
		return SubmissionApprovalResult{}, err
	}
	if submission.CurrentRevisionID != revision.ID || (submission.Status != "submitted" && submission.Status != "in_review") {
		return SubmissionApprovalResult{}, fault.Conflict("SUBMISSION_STATE_INVALID", "只能批准当前待审核的提交内容版本")
	}
	if revision.EvidenceLimited {
		return SubmissionApprovalResult{}, fault.Policy("EVIDENCE_LEVEL_INSUFFICIENT", "高风险内容的来源披露不足，不能远程批准", "上传证据包（evidence_pack）或完整来源（full_source），或完成受治理的本地核验")
	}
	now := s.now().UTC()
	if err := s.validateTenantSubmissionContentTypes(ctx, actor.TenantID, submission.SubmissionType, revision.ProjectID, revision.Objects); err != nil {
		return SubmissionApprovalResult{}, err
	}
	if err := validateGovernedSubmissionObjects(submission.SubmissionType, revision.ProjectID, revision.BaseSnapshotIDs, revision.Objects, now); err != nil {
		return SubmissionApprovalResult{}, err
	}
	baseSnapshots, err := s.loadSubmissionBaseSnapshots(ctx, actor.TenantID, revision.ProjectID, revision.BaseSnapshotIDs)
	if err != nil {
		return SubmissionApprovalResult{}, err
	}
	if err := validateGovernedBaseSnapshotTypes(submission.SubmissionType, revision.Objects, baseSnapshots, now); err != nil {
		return SubmissionApprovalResult{}, err
	}
	if err := s.requireResolvedComments(ctx, actor.TenantID, revision.ID, ""); err != nil {
		return SubmissionApprovalResult{}, err
	}
	resultingState := "approved"
	if submission.SubmissionType == "content_batch" {
		resultingState = "internally_approved"
	}
	decision := reviewdomain.ApprovalDecision{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, DecisionStage: "internal", ActorID: actor.UserID, Decision: "approve", Reason: strings.TrimSpace(reason), PreviousState: submission.Status, ResultingState: resultingState, CreatedAt: now}
	submission.Status = resultingState
	submission.UpdatedAt = now
	result := SubmissionApprovalResult{Submission: submission, Decision: decision}
	if submission.SubmissionType == "content_batch" {
		if err := s.review.RecordSubmissionApproval(ctx, submission, decision); err != nil {
			return SubmissionApprovalResult{}, err
		}
		s.audit(ctx, actor, revision.ProjectID, "submission.internally_approved", "submission_revision", revision.ID, requestID, map[string]any{"submission_id": submission.ID, "content_hash": revision.ContentHash})
		return result, nil
	}
	canonical, err := canonicalSubmissionContent(submission, revision)
	if err != nil {
		return SubmissionApprovalResult{}, err
	}
	snapshot := reviewdomain.ApprovedSnapshot{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, WorkspaceID: revision.WorkspaceID, SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, SubmissionType: submission.SubmissionType, SchemaVersion: revision.SchemaVersion, ContentHash: revision.ContentHash, SubjectHash: revision.ContentHash, CanonicalContent: canonical, EligibleIDs: revision.EligibleObjectIDs(), Artifacts: revision.Artifacts, DecisionID: decision.ID, CreatedBy: actor.UserID, CreatedAt: now}
	if err := s.review.ApproveSubmissionRevision(ctx, submission, snapshot, decision); err != nil {
		return SubmissionApprovalResult{}, err
	}
	s.audit(ctx, actor, revision.ProjectID, "submission.approved", "submission_revision", revision.ID, requestID, map[string]any{"submission_id": submission.ID, "snapshot_id": snapshot.ID, "content_hash": snapshot.ContentHash})
	result.ApprovedSnapshot = &snapshot
	return result, nil
}

func (s *ReviewService) RequestSubmissionChanges(ctx context.Context, actor Actor, revisionID, reason, jsonPointer, requestID string) (reviewdomain.Submission, error) {
	var err error
	actor, err = s.reviewActorWithRole(ctx, actor)
	if err != nil {
		return reviewdomain.Submission{}, err
	}
	if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer"); err != nil {
		return reviewdomain.Submission{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return reviewdomain.Submission{}, fault.Invalid("CHANGE_REASON_REQUIRED", "修改要求必须填写具体原因")
	}
	if jsonPointer != "" && !reviewdomain.ValidJSONPointer(jsonPointer) {
		return reviewdomain.Submission{}, fault.Invalid("COMMENT_POINTER_INVALID", "批注位置必须使用合法的 JSON 指针")
	}
	revision, err := s.review.SubmissionRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return reviewdomain.Submission{}, err
	}
	submission, err := s.review.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil {
		return submission, err
	}
	if submission.CurrentRevisionID != revision.ID || (submission.Status != "submitted" && submission.Status != "in_review" && submission.Status != "internally_approved" && submission.Status != "client_review") {
		return submission, fault.Conflict("SUBMISSION_STATE_INVALID", "只能退回当前待审核的提交内容版本")
	}
	cycles, err := s.review.ReviewCycles(ctx, actor.TenantID, revision.ID)
	if err != nil {
		return submission, err
	}
	var cycle reviewdomain.ReviewCycle
	if len(cycles) > 0 && cycles[0].Status == "open" {
		cycle = cycles[0]
	} else {
		now := s.now().UTC()
		cycle = reviewdomain.ReviewCycle{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, Status: "open", OpenedBy: actor.UserID, OpenedAt: now, CreatedAt: now}
		cycle, err = s.review.CreateReviewCycle(ctx, cycle)
		if err != nil {
			return submission, err
		}
	}
	now := s.now().UTC()
	comment := reviewdomain.ReviewComment{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, ReviewCycleID: cycle.ID, SubjectType: "submission_revision", SubjectID: revision.ID, JSONPointer: jsonPointer, Body: reason, Visibility: "internal", AuthorID: actor.UserID, CreatedAt: now}
	decision := reviewdomain.ApprovalDecision{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, DecisionStage: "internal", ActorID: actor.UserID, Decision: "request_changes", Reason: reason, PreviousState: submission.Status, ResultingState: "changes_requested", CreatedAt: now}
	submission.Status = "changes_requested"
	submission.UpdatedAt = now
	if err := s.review.RequestSubmissionChanges(ctx, submission, decision, comment); err != nil {
		return submission, err
	}
	s.audit(ctx, actor, revision.ProjectID, "submission.changes_requested", "submission_revision", revision.ID, requestID, map[string]any{"submission_id": submission.ID, "json_pointer": jsonPointer})
	return submission, nil
}

func canonicalSubmissionContent(submission reviewdomain.Submission, revision reviewdomain.SubmissionRevision) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"schema_version": revision.SchemaVersion, "submission_type": submission.SubmissionType, "objects": submissionObjectContents(revision.Objects), "object_refs": revision.Objects,
		"base_snapshot_ids": revision.BaseSnapshotIDs, "environment_digest": revision.EnvironmentDigest,
		"source_disclosures": revision.SourceDisclosures, "artifacts": revision.Artifacts, "local_run_summary": revision.LocalRunSummary,
	})
}

func submissionObjectContents(values []reviewdomain.SubmissionObjectRef) []json.RawMessage {
	contents := make([]json.RawMessage, len(values))
	for index := range values {
		contents[index] = append(json.RawMessage(nil), values[index].Content...)
	}
	return contents
}

func cloneSubmissionObjects(values []reviewdomain.SubmissionObjectRef) []reviewdomain.SubmissionObjectRef {
	cloned := make([]reviewdomain.SubmissionObjectRef, len(values))
	copy(cloned, values)
	for index := range cloned {
		cloned[index].Content = append(json.RawMessage(nil), values[index].Content...)
	}
	return cloned
}

func validateGovernedSubmissionObjects(submissionType, projectID string, baseSnapshotIDs []string, objects []reviewdomain.SubmissionObjectRef, now time.Time) error {
	if submissionType == "storyboard" && len(objects) != 1 {
		return fault.Invalid("STORYBOARD_SUBMISSION_CARDINALITY_INVALID", "分镜提交内容版本必须且只能包含一个分镜包")
	}
	for _, object := range objects {
		if submissionType == "strategy" || submissionType == "offer" || submissionType == "storyboard" {
			var identity struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			}
			if err := json.Unmarshal(object.Content, &identity); err != nil || identity.ID != object.ID || identity.Type != object.Type {
				return fault.Invalid("SUBMISSION_OBJECT_IDENTITY_MISMATCH", "V5 对象引用中的标识和类型（id/type）必须与结构化正文一致")
			}
		}
		switch submissionType {
		case "strategy":
			switch object.Type {
			case "audience_taxonomy_snapshot":
				var value work.AudienceTaxonomySnapshot
				if err := json.Unmarshal(object.Content, &value); err != nil {
					return fault.Invalid("AUDIENCE_TAXONOMY_JSON_INVALID", "人群目录不是有效 JSON")
				}
				if err := value.Validate(now, true); err != nil {
					return err
				}
			case "audience_strategy_version":
				var value work.AudienceStrategyVersion
				if err := json.Unmarshal(object.Content, &value); err != nil {
					return fault.Invalid("AUDIENCE_STRATEGY_JSON_INVALID", "人群策略不是有效 JSON")
				}
				if value.ProjectID != projectID {
					return fault.Conflict("AUDIENCE_STRATEGY_PROJECT_MISMATCH", "人群策略不属于当前项目")
				}
				if err := value.Validate(true); err != nil {
					return err
				}
			default:
				return fault.Invalid("STRATEGY_OBJECT_TYPE_INVALID", "策略提交（strategy）只接受人群目录快照（AudienceTaxonomySnapshot）或人群策略版本（AudienceStrategyVersion）")
			}
		case "offer":
			if object.Type != "commerce_offer_snapshot" {
				return fault.Invalid("OFFER_OBJECT_TYPE_INVALID", "商品方案提交（offer）只接受商品方案快照（CommerceOfferSnapshot）")
			}
			var value work.CommerceOfferSnapshot
			if err := json.Unmarshal(object.Content, &value); err != nil {
				return fault.Invalid("COMMERCE_OFFER_JSON_INVALID", "商品方案不是有效 JSON")
			}
			if value.ProjectID != projectID {
				return fault.Conflict("COMMERCE_OFFER_PROJECT_MISMATCH", "商品方案不属于当前项目")
			}
			if err := value.Validate(now, true); err != nil {
				return err
			}
		case "storyboard":
			if object.Type != "storyboard_package" {
				return fault.Invalid("STORYBOARD_OBJECT_TYPE_INVALID", "分镜提交（storyboard）只接受分镜包（StoryboardPackage）")
			}
			var value work.StoryboardPackage
			if err := json.Unmarshal(object.Content, &value); err != nil {
				return fault.Invalid("STORYBOARD_JSON_INVALID", "分镜包不是有效 JSON")
			}
			if value.ProjectID != projectID {
				return fault.Conflict("STORYBOARD_PROJECT_MISMATCH", "分镜包不属于当前项目")
			}
			if !containsSubmissionString(baseSnapshotIDs, value.ApprovedSnapshotID) {
				return fault.Invalid("STORYBOARD_BASE_SNAPSHOT_REQUIRED", "分镜包的已批准快照标识（approved_snapshot_id）必须出现在提交内容版本的基线快照列表（base_snapshot_ids）中")
			}
			if err := value.Validate(true); err != nil {
				return err
			}
			lockedDigest, err := value.ComputedLockedDigest()
			if err != nil {
				return err
			}
			if lockedDigest != value.LockedDigest {
				return fault.Conflict("STORYBOARD_LOCKED_DIGEST_MISMATCH", "分镜包的锁定摘要（locked_digest）与服务端复算结果不一致")
			}
		}
	}
	return nil
}

func (s *ReviewService) validateTenantSubmissionContentTypes(ctx context.Context, tenantID, submissionType, projectID string, objects []reviewdomain.SubmissionObjectRef) error {
	contentType := ""
	for _, object := range objects {
		var identity struct {
			SchemaVersion string `json:"schema_version"`
		}
		if err := json.Unmarshal(object.Content, &identity); err != nil {
			return fault.Invalid("SUBMISSION_OBJECT_JSON_INVALID", "提交对象不是有效 JSON")
		}
		objectContentType := ""
		switch submissionType {
		case "brief":
			if identity.SchemaVersion == localworkspace.ArticleBriefSchema {
				if _, err := localworkspace.ValidateArticleBriefForSubmission(object.Content); err != nil {
					return err
				}
				objectContentType = identitydomain.ContentTypeWeChatArticle
			}
		case "content_batch":
			switch identity.SchemaVersion {
			case localworkspace.ContentItemSchema:
				var itemIdentity struct {
					ContentKind string `json:"content_kind"`
				}
				if err := json.Unmarshal(object.Content, &itemIdentity); err != nil {
					return fault.Invalid("CONTENT_ITEM_KIND_INVALID", "内容项的内容类型（content_kind）无效")
				}
				objectContentType = itemIdentity.ContentKind
				if objectContentType == "" {
					objectContentType = identitydomain.ContentTypeVideoScript
				}
				if !identitydomain.ValidTenantContentType(objectContentType) {
					return fault.Invalid("CONTENT_ITEM_KIND_INVALID", "内容项的内容类型（content_kind）不受支持")
				}
			case localworkspace.ArticleSchema:
				if _, err := localworkspace.ValidateArticleItemForSubmission(object.Content, projectID); err != nil {
					return err
				}
				objectContentType = identitydomain.ContentTypeWeChatArticle
			case localworkspace.ContentBatchSchema:
				var batchIdentity struct {
					ContentKind string `json:"content_kind"`
				}
				if err := json.Unmarshal(object.Content, &batchIdentity); err != nil || !identitydomain.ValidTenantContentType(batchIdentity.ContentKind) {
					return fault.Invalid("CONTENT_BATCH_KIND_INVALID", "内容批次清单缺少受支持的内容类型（content_kind）")
				}
				objectContentType = batchIdentity.ContentKind
			default:
				return fault.Invalid("CONTENT_SCHEMA_UNSUPPORTED", "内容批次（content_batch）包含不受支持的内容格式")
			}
		}
		if objectContentType == "" {
			continue
		}
		if contentType != "" && contentType != objectContentType {
			return fault.Invalid("CONTENT_BATCH_KIND_MIXED", "同一提交内容版本不能混合不同内容类型")
		}
		contentType = objectContentType
	}
	if contentType == "" || contentType == identitydomain.ContentTypeVideoScript {
		return nil
	}
	enabled, err := s.app.Identity.TenantContentTypes(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, value := range enabled {
		if value == contentType {
			return nil
		}
	}
	return fault.Policy("CONTENT_TYPE_NOT_ENABLED", "当前租户未开通内容类型（"+contentType+"）", "联系平台管理员开通后刷新本地工作区的执行环境清单")
}

func validateGovernedBaseSnapshotTypes(submissionType string, objects []reviewdomain.SubmissionObjectRef, baseSnapshots map[string]reviewdomain.ApprovedSnapshot, now time.Time) error {
	for _, object := range objects {
		switch submissionType {
		case "content_batch":
			var identity struct {
				SchemaVersion string `json:"schema_version"`
			}
			if err := json.Unmarshal(object.Content, &identity); err != nil || identity.SchemaVersion != localworkspace.ArticleSchema {
				continue
			}
			var item localworkspace.ArticleItem
			if err := json.Unmarshal(object.Content, &item); err != nil {
				return fault.Invalid("ARTICLE_ITEM_JSON_INVALID", "文章内容项不是有效 JSON")
			}
			briefFound := false
			for _, snapshot := range baseSnapshots {
				if snapshot.SubmissionType == "brief" && containsSubmissionString(snapshot.EligibleIDs, item.BriefRef) {
					briefFound = true
					break
				}
			}
			if !briefFound {
				return fault.Policy("ARTICLE_BRIEF_BASE_SNAPSHOT_REQUIRED", "文章内容项必须引用当前项目已批准的文章创作简报", "发布时包含已固定的创作简报批准快照")
			}
			for _, block := range item.Blocks {
				for _, assertion := range block.Assertions {
					for _, reference := range assertion.KnowledgeRefs {
						kind, found := submissionKnowledgeKind(baseSnapshots, reference)
						if !found {
							return fault.Policy("ARTICLE_KNOWLEDGE_BASE_SNAPSHOT_REQUIRED", "文章内容项中的事实陈述引用未进入已批准知识快照："+reference, "刷新知识快照并重新发布")
						}
						if assertion.Type == "commercial_claim" && kind != "claim" {
							return fault.Policy("ARTICLE_CLAIM_BASE_INVALID", "商业主张（commercial_claim）必须引用已批准的营销主张："+reference, "改用已批准的营销主张，或调整事实陈述类型")
						}
					}
				}
			}
		case "strategy":
			if object.Type != "audience_strategy_version" {
				continue
			}
			var strategy work.AudienceStrategyVersion
			if err := json.Unmarshal(object.Content, &strategy); err != nil {
				return fault.Invalid("AUDIENCE_STRATEGY_JSON_INVALID", "人群策略版本不是有效 JSON")
			}
			taxonomy, found, err := audienceTaxonomyFromBaseSnapshots(strategy.TaxonomySnapshotID, baseSnapshots)
			if err != nil {
				return err
			}
			if !found {
				return fault.Conflict("AUDIENCE_TAXONOMY_BASE_SNAPSHOT_INVALID", "人群策略版本必须引用当前项目已批准的人群目录基线")
			}
			if err := strategy.ValidateAgainstTaxonomy(taxonomy, now); err != nil {
				return err
			}
		case "storyboard":
			var value work.StoryboardPackage
			if err := json.Unmarshal(object.Content, &value); err != nil {
				return fault.Invalid("STORYBOARD_JSON_INVALID", "分镜包不是有效 JSON")
			}
			snapshot, ok := baseSnapshots[value.ApprovedSnapshotID]
			if !ok || snapshot.SubmissionType != "content_batch" {
				return fault.Conflict("STORYBOARD_CONTENT_SNAPSHOT_INVALID", "分镜包必须引用当前项目的已批准内容批次快照")
			}
			raw, err := approvedSnapshotObject(snapshot, value.ContentItemID)
			if err != nil {
				if fault.IsNotFound(err) {
					return fault.Conflict("STORYBOARD_CONTENT_ITEM_BASE_INVALID", "分镜包的内容项标识（content_item_id）不在所引用批准快照的可用对象中")
				}
				return err
			}
			hash, err := stablehash.Sum(json.RawMessage(raw))
			if err != nil {
				return err
			}
			if value.SourceDigest != "sha256:"+hash {
				return fault.Conflict("STORYBOARD_SOURCE_DIGEST_MISMATCH", "分镜包的来源摘要（source_digest）与已批准内容项不一致")
			}
		}
	}
	return nil
}

func submissionKnowledgeKind(baseSnapshots map[string]reviewdomain.ApprovedSnapshot, objectID string) (string, bool) {
	for _, snapshot := range baseSnapshots {
		if snapshot.SubmissionType != "knowledge" || !containsSubmissionString(snapshot.EligibleIDs, objectID) {
			continue
		}
		var canonical struct {
			Objects []json.RawMessage `json:"objects"`
		}
		if json.Unmarshal(snapshot.CanonicalContent, &canonical) != nil {
			continue
		}
		for _, raw := range canonical.Objects {
			var identity struct {
				ID   string `json:"id"`
				Kind string `json:"kind"`
			}
			if json.Unmarshal(raw, &identity) == nil && identity.ID == objectID {
				return identity.Kind, true
			}
		}
	}
	return "", false
}

func (s *ReviewService) loadSubmissionBaseSnapshots(ctx context.Context, tenantID, projectID string, snapshotIDs []string) (map[string]reviewdomain.ApprovedSnapshot, error) {
	values := make(map[string]reviewdomain.ApprovedSnapshot, len(snapshotIDs))
	for _, snapshotID := range snapshotIDs {
		snapshot, err := s.review.ApprovedSnapshot(ctx, tenantID, snapshotID)
		if err != nil {
			return nil, err
		}
		if snapshot.ProjectID != projectID {
			return nil, fault.Conflict("BASE_SNAPSHOT_MISMATCH", "批准基线不属于当前项目")
		}
		values[snapshot.ID] = snapshot
	}
	return values, nil
}

func audienceTaxonomyFromBaseSnapshots(objectID string, snapshots map[string]reviewdomain.ApprovedSnapshot) (work.AudienceTaxonomySnapshot, bool, error) {
	for _, snapshot := range snapshots {
		if snapshot.SubmissionType != "strategy" || !containsSubmissionString(snapshot.EligibleIDs, objectID) {
			continue
		}
		raw, err := approvedSnapshotObject(snapshot, objectID)
		if err != nil {
			if fault.IsNotFound(err) {
				return work.AudienceTaxonomySnapshot{}, false, fault.Conflict("AUDIENCE_TAXONOMY_BASE_SNAPSHOT_INVALID", "人群目录批准快照的可用对象标识（eligible_ids）与规范对象列表不一致")
			}
			return work.AudienceTaxonomySnapshot{}, false, err
		}
		var taxonomy work.AudienceTaxonomySnapshot
		if err := json.Unmarshal(raw, &taxonomy); err != nil || taxonomy.Type != "audience_taxonomy_snapshot" {
			return work.AudienceTaxonomySnapshot{}, false, fault.Conflict("AUDIENCE_TAXONOMY_BASE_SNAPSHOT_INVALID", "人群目录快照标识（taxonomy_snapshot_id）未引用有效的人群目录快照")
		}
		return taxonomy, true, nil
	}
	return work.AudienceTaxonomySnapshot{}, false, nil
}

func containsSubmissionString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s *ReviewService) ApprovedSnapshots(ctx context.Context, actor Actor, projectID, submissionType string) ([]reviewdomain.ApprovedSnapshot, error) {
	if actor.Type == "workspace" {
		binding, err := s.workspace.WorkspaceBinding(ctx, actor.TenantID, actor.WorkspaceID)
		if err != nil || binding.ProjectID != projectID {
			return nil, fault.NotFound("项目")
		}
	} else if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.review.ApprovedSnapshots(ctx, actor.TenantID, projectID, submissionType)
}

func (s *ReviewService) ApprovedSnapshot(ctx context.Context, actor Actor, id string) (reviewdomain.ApprovedSnapshot, error) {
	snapshot, err := s.review.ApprovedSnapshot(ctx, actor.TenantID, id)
	if err != nil {
		return snapshot, err
	}
	if actor.Type == "workspace" && snapshot.WorkspaceID != actor.WorkspaceID {
		return reviewdomain.ApprovedSnapshot{}, fault.NotFound("已批准快照")
	}
	return snapshot, nil
}

func (s *ReviewService) WorkspaceFeedback(ctx context.Context, actor Actor, binding workspacedomain.WorkspaceBinding) ([]reviewdomain.ReviewFeedbackBundle, error) {
	submissions, err := s.WorkspaceSubmissions(ctx, actor, binding)
	if err != nil {
		return nil, err
	}
	bundles := []reviewdomain.ReviewFeedbackBundle{}
	for _, submission := range submissions {
		revisions, err := s.review.SubmissionRevisions(ctx, actor.TenantID, submission.ID)
		if err != nil {
			return nil, err
		}
		for _, revision := range revisions {
			comments, err := s.review.ReviewComments(ctx, actor.TenantID, revision.ID)
			if err != nil {
				return nil, err
			}
			if len(comments) == 0 {
				continue
			}
			bundles = append(bundles, reviewdomain.ReviewFeedbackBundle{BundleVersion: "1.0", SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, SubjectHash: revision.ContentHash, Comments: comments, CreatedAt: comments[len(comments)-1].CreatedAt})
		}
	}
	return bundles, nil
}

func (s *ReviewService) WorkspaceDecisions(ctx context.Context, actor Actor, binding workspacedomain.WorkspaceBinding) (reviewdomain.DecisionDelta, error) {
	submissions, err := s.WorkspaceSubmissions(ctx, actor, binding)
	if err != nil {
		return reviewdomain.DecisionDelta{}, err
	}
	decisions := []reviewdomain.ApprovalDecision{}
	for _, submission := range submissions {
		revisions, err := s.review.SubmissionRevisions(ctx, actor.TenantID, submission.ID)
		if err != nil {
			return reviewdomain.DecisionDelta{}, err
		}
		for _, revision := range revisions {
			values, err := s.review.Approvals(ctx, actor.TenantID, revision.ID)
			if err != nil {
				return reviewdomain.DecisionDelta{}, err
			}
			decisions = append(decisions, values...)
		}
	}
	return reviewdomain.DecisionDelta{BundleVersion: "1.0", ProjectID: binding.ProjectID, Decisions: decisions, CreatedAt: s.now().UTC()}, nil
}

func (s *ReviewService) requireResolvedComments(ctx context.Context, tenantID, subjectID, visibility string) error {
	comments, err := s.review.ReviewComments(ctx, tenantID, subjectID)
	if err != nil {
		return err
	}
	for _, comment := range comments {
		if comment.ResolvedAt == nil && (visibility == "" || comment.Visibility == visibility) {
			return fault.Policy("REVIEW_COMMENTS_UNRESOLVED", "仍有未解决审核批注，不能批准", "先解决所有适用批注")
		}
	}
	return nil
}

func isNotFound(err error) bool {
	var domainError *fault.Error
	return errors.As(err, &domainError) && domainError.Type == "not_found"
}

func normalizeSubmissionHash(value string) string {
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
	return "sha256:" + normalized
}
