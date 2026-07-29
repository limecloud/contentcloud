package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
)

type SubmissionDetails struct {
	Submission domain.Submission           `json:"submission"`
	Revisions  []domain.SubmissionRevision `json:"revisions"`
	Comments   []domain.ReviewComment      `json:"comments"`
}

type SubmissionRevisionView struct {
	Submission domain.Submission         `json:"submission"`
	Revision   domain.SubmissionRevision `json:"revision"`
	Comments   []domain.ReviewComment    `json:"comments"`
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
	s.audit(ctx, actor, binding.ProjectID, "workspace.registered", "workspace_binding", binding.ID, requestID, map[string]any{"template_version": templateVersion, "targets": normalizedTargets})
	return binding, nil
}

func (s *Service) CreateSubmission(ctx context.Context, actor Actor, binding domain.WorkspaceBinding, bundle domain.SubmissionBundle, requestID string) (domain.SubmissionRevision, error) {
	if actor.Type != "workspace" || actor.WorkspaceID != binding.ID || bundle.WorkspaceID != binding.ID || bundle.ProjectID != binding.ProjectID {
		return domain.SubmissionRevision{}, domain.Policy("WORKSPACE_SCOPE_DENIED", "提交不属于当前工作区和项目", "检查本地 .contentcloud/workspace.yaml 后重试")
	}
	if err := bundle.Validate(); err != nil {
		return domain.SubmissionRevision{}, err
	}
	now := s.now().UTC()
	if err := validateGovernedSubmissionObjects(bundle.SubmissionType, bundle.ProjectID, bundle.BaseSnapshotIDs, bundle.Objects, now); err != nil {
		return domain.SubmissionRevision{}, err
	}
	if _, err := s.store.Project(ctx, binding.TenantID, binding.ProjectID); err != nil {
		return domain.SubmissionRevision{}, err
	}
	baseSnapshots, err := s.loadSubmissionBaseSnapshots(ctx, binding.TenantID, binding.ProjectID, bundle.BaseSnapshotIDs)
	if err != nil {
		return domain.SubmissionRevision{}, err
	}
	if err := validateGovernedBaseSnapshotTypes(bundle.SubmissionType, bundle.Objects, baseSnapshots, now); err != nil {
		return domain.SubmissionRevision{}, err
	}
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
		RevisionNo: revisionNo, SchemaVersion: domain.SubmissionSchemaVersion(bundle.SubmissionType), ContentHash: normalizeSubmissionHash(bundle.ContentHash), BaseSnapshotIDs: append([]string{}, bundle.BaseSnapshotIDs...), EnvironmentDigest: bundle.EnvironmentDigest,
		LocalRunSummary: bundle.LocalRunSummary, Objects: cloneSubmissionObjects(bundle.Objects), Artifacts: append([]domain.SubmissionArtifact{}, bundle.Artifacts...), Message: strings.TrimSpace(bundle.Message),
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

func (s *Service) ProjectSubmissionRevision(ctx context.Context, actor Actor, projectID, revisionID string) (SubmissionRevisionView, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return SubmissionRevisionView{}, err
	}
	revision, err := s.store.SubmissionRevision(ctx, actor.TenantID, revisionID)
	if err != nil || revision.ProjectID != projectID {
		if err == nil {
			err = domain.NotFound("SubmissionRevision")
		}
		return SubmissionRevisionView{}, err
	}
	submission, err := s.store.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil || submission.ProjectID != projectID {
		if err == nil {
			err = domain.NotFound("Submission")
		}
		return SubmissionRevisionView{}, err
	}
	if actor.Type == "workspace" && submission.WorkspaceID != actor.WorkspaceID {
		return SubmissionRevisionView{}, domain.NotFound("SubmissionRevision")
	}
	comments, err := s.store.ReviewComments(ctx, actor.TenantID, revision.ID)
	if err != nil {
		return SubmissionRevisionView{}, err
	}
	return SubmissionRevisionView{Submission: submission, Revision: revision, Comments: comments}, nil
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
	now := s.now().UTC()
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
	decision := domain.ApprovalDecision{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, DecisionStage: "internal", ActorID: actor.UserID, Decision: "approve", Reason: strings.TrimSpace(reason), PreviousState: submission.Status, ResultingState: resultingState, CreatedAt: now}
	submission.Status = resultingState
	submission.UpdatedAt = now
	result := SubmissionApprovalResult{Submission: submission, Decision: decision}
	if submission.SubmissionType == "content_batch" {
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
	snapshot := domain.ApprovedSnapshot{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, WorkspaceID: revision.WorkspaceID, SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, SubmissionType: submission.SubmissionType, SchemaVersion: revision.SchemaVersion, ContentHash: revision.ContentHash, SubjectHash: revision.ContentHash, CanonicalContent: canonical, EligibleIDs: revision.EligibleObjectIDs(), Artifacts: revision.Artifacts, DecisionID: decision.ID, CreatedBy: actor.UserID, CreatedAt: now}
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
		"schema_version": revision.SchemaVersion, "submission_type": submission.SubmissionType, "objects": submissionObjectContents(revision.Objects), "object_refs": revision.Objects,
		"base_snapshot_ids": revision.BaseSnapshotIDs, "environment_digest": revision.EnvironmentDigest,
		"source_disclosures": revision.SourceDisclosures, "artifacts": revision.Artifacts, "local_run_summary": revision.LocalRunSummary,
	})
}

func submissionObjectContents(values []domain.SubmissionObjectRef) []json.RawMessage {
	contents := make([]json.RawMessage, len(values))
	for index := range values {
		contents[index] = append(json.RawMessage(nil), values[index].Content...)
	}
	return contents
}

func cloneSubmissionObjects(values []domain.SubmissionObjectRef) []domain.SubmissionObjectRef {
	cloned := make([]domain.SubmissionObjectRef, len(values))
	copy(cloned, values)
	for index := range cloned {
		cloned[index].Content = append(json.RawMessage(nil), values[index].Content...)
	}
	return cloned
}

func validateGovernedSubmissionObjects(submissionType, projectID string, baseSnapshotIDs []string, objects []domain.SubmissionObjectRef, now time.Time) error {
	if submissionType == "storyboard" && len(objects) != 1 {
		return domain.Invalid("STORYBOARD_SUBMISSION_CARDINALITY_INVALID", "storyboard SubmissionRevision 必须且只能包含一个 StoryboardPackage")
	}
	for _, object := range objects {
		if submissionType == "strategy" || submissionType == "offer" || submissionType == "storyboard" {
			var identity struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			}
			if err := json.Unmarshal(object.Content, &identity); err != nil || identity.ID != object.ID || identity.Type != object.Type {
				return domain.Invalid("SUBMISSION_OBJECT_IDENTITY_MISMATCH", "V5 object ref 的 id/type 必须与结构化正文一致")
			}
		}
		switch submissionType {
		case "strategy":
			switch object.Type {
			case "audience_taxonomy_snapshot":
				var value domain.AudienceTaxonomySnapshot
				if err := json.Unmarshal(object.Content, &value); err != nil {
					return domain.Invalid("AUDIENCE_TAXONOMY_JSON_INVALID", "人群目录不是有效 JSON")
				}
				if err := value.Validate(now, true); err != nil {
					return err
				}
			case "audience_strategy_version":
				var value domain.AudienceStrategyVersion
				if err := json.Unmarshal(object.Content, &value); err != nil {
					return domain.Invalid("AUDIENCE_STRATEGY_JSON_INVALID", "人群策略不是有效 JSON")
				}
				if value.ProjectID != projectID {
					return domain.Conflict("AUDIENCE_STRATEGY_PROJECT_MISMATCH", "人群策略不属于当前项目")
				}
				if err := value.Validate(true); err != nil {
					return err
				}
			default:
				return domain.Invalid("STRATEGY_OBJECT_TYPE_INVALID", "strategy 只接受 AudienceTaxonomySnapshot 或 AudienceStrategyVersion")
			}
		case "offer":
			if object.Type != "commerce_offer_snapshot" {
				return domain.Invalid("OFFER_OBJECT_TYPE_INVALID", "offer 只接受 CommerceOfferSnapshot")
			}
			var value domain.CommerceOfferSnapshot
			if err := json.Unmarshal(object.Content, &value); err != nil {
				return domain.Invalid("COMMERCE_OFFER_JSON_INVALID", "Offer 不是有效 JSON")
			}
			if value.ProjectID != projectID {
				return domain.Conflict("COMMERCE_OFFER_PROJECT_MISMATCH", "Offer 不属于当前项目")
			}
			if err := value.Validate(now, true); err != nil {
				return err
			}
		case "storyboard":
			if object.Type != "storyboard_package" {
				return domain.Invalid("STORYBOARD_OBJECT_TYPE_INVALID", "storyboard 只接受 StoryboardPackage")
			}
			var value domain.StoryboardPackage
			if err := json.Unmarshal(object.Content, &value); err != nil {
				return domain.Invalid("STORYBOARD_JSON_INVALID", "StoryboardPackage 不是有效 JSON")
			}
			if value.ProjectID != projectID {
				return domain.Conflict("STORYBOARD_PROJECT_MISMATCH", "StoryboardPackage 不属于当前项目")
			}
			if !containsSubmissionString(baseSnapshotIDs, value.ApprovedSnapshotID) {
				return domain.Invalid("STORYBOARD_BASE_SNAPSHOT_REQUIRED", "StoryboardPackage approved_snapshot_id 必须出现在 SubmissionRevision base_snapshot_ids 中")
			}
			if err := value.Validate(true); err != nil {
				return err
			}
			lockedDigest, err := value.ComputedLockedDigest()
			if err != nil {
				return err
			}
			if lockedDigest != value.LockedDigest {
				return domain.Conflict("STORYBOARD_LOCKED_DIGEST_MISMATCH", "StoryboardPackage locked_digest 与服务端复算结果不一致")
			}
		}
	}
	return nil
}

func validateGovernedBaseSnapshotTypes(submissionType string, objects []domain.SubmissionObjectRef, baseSnapshots map[string]domain.ApprovedSnapshot, now time.Time) error {
	for _, object := range objects {
		switch submissionType {
		case "strategy":
			if object.Type != "audience_strategy_version" {
				continue
			}
			var strategy domain.AudienceStrategyVersion
			if err := json.Unmarshal(object.Content, &strategy); err != nil {
				return domain.Invalid("AUDIENCE_STRATEGY_JSON_INVALID", "AudienceStrategyVersion 不是有效 JSON")
			}
			taxonomy, found, err := audienceTaxonomyFromBaseSnapshots(strategy.TaxonomySnapshotID, baseSnapshots)
			if err != nil {
				return err
			}
			if !found {
				return domain.Conflict("AUDIENCE_TAXONOMY_BASE_SNAPSHOT_INVALID", "AudienceStrategyVersion 必须引用当前项目已批准的 taxonomy 基线")
			}
			if err := strategy.ValidateAgainstTaxonomy(taxonomy, now); err != nil {
				return err
			}
		case "storyboard":
			var value domain.StoryboardPackage
			if err := json.Unmarshal(object.Content, &value); err != nil {
				return domain.Invalid("STORYBOARD_JSON_INVALID", "StoryboardPackage 不是有效 JSON")
			}
			snapshot, ok := baseSnapshots[value.ApprovedSnapshotID]
			if !ok || snapshot.SubmissionType != "content_batch" {
				return domain.Conflict("STORYBOARD_CONTENT_SNAPSHOT_INVALID", "StoryboardPackage 必须引用当前项目的 content_batch ApprovedSnapshot")
			}
			raw, err := approvedSnapshotObject(snapshot, value.ContentItemID)
			if err != nil {
				if domain.IsNotFound(err) {
					return domain.Conflict("STORYBOARD_CONTENT_ITEM_BASE_INVALID", "StoryboardPackage content_item_id 不在所引用 ApprovedSnapshot 的 eligible objects 中")
				}
				return err
			}
			hash, err := domain.CanonicalHash(json.RawMessage(raw))
			if err != nil {
				return err
			}
			if value.SourceDigest != "sha256:"+hash {
				return domain.Conflict("STORYBOARD_SOURCE_DIGEST_MISMATCH", "StoryboardPackage source_digest 与批准 ContentItem 不一致")
			}
		}
	}
	return nil
}

func (s *Service) loadSubmissionBaseSnapshots(ctx context.Context, tenantID, projectID string, snapshotIDs []string) (map[string]domain.ApprovedSnapshot, error) {
	values := make(map[string]domain.ApprovedSnapshot, len(snapshotIDs))
	for _, snapshotID := range snapshotIDs {
		snapshot, err := s.store.ApprovedSnapshot(ctx, tenantID, snapshotID)
		if err != nil {
			return nil, err
		}
		if snapshot.ProjectID != projectID {
			return nil, domain.Conflict("BASE_SNAPSHOT_MISMATCH", "批准基线不属于当前项目")
		}
		values[snapshot.ID] = snapshot
	}
	return values, nil
}

func audienceTaxonomyFromBaseSnapshots(objectID string, snapshots map[string]domain.ApprovedSnapshot) (domain.AudienceTaxonomySnapshot, bool, error) {
	for _, snapshot := range snapshots {
		if snapshot.SubmissionType != "strategy" || !containsSubmissionString(snapshot.EligibleIDs, objectID) {
			continue
		}
		raw, err := approvedSnapshotObject(snapshot, objectID)
		if err != nil {
			if domain.IsNotFound(err) {
				return domain.AudienceTaxonomySnapshot{}, false, domain.Conflict("AUDIENCE_TAXONOMY_BASE_SNAPSHOT_INVALID", "taxonomy ApprovedSnapshot eligible_ids 与 canonical objects 不一致")
			}
			return domain.AudienceTaxonomySnapshot{}, false, err
		}
		var taxonomy domain.AudienceTaxonomySnapshot
		if err := json.Unmarshal(raw, &taxonomy); err != nil || taxonomy.Type != "audience_taxonomy_snapshot" {
			return domain.AudienceTaxonomySnapshot{}, false, domain.Conflict("AUDIENCE_TAXONOMY_BASE_SNAPSHOT_INVALID", "taxonomy_snapshot_id 未引用有效 AudienceTaxonomySnapshot")
		}
		return taxonomy, true, nil
	}
	return domain.AudienceTaxonomySnapshot{}, false, nil
}

func containsSubmissionString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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

func (s *Service) requireResolvedComments(ctx context.Context, tenantID, subjectID, visibility string) error {
	comments, err := s.store.ReviewComments(ctx, tenantID, subjectID)
	if err != nil {
		return err
	}
	for _, comment := range comments {
		if comment.ResolvedAt == nil && (visibility == "" || comment.Visibility == visibility) {
			return domain.Policy("REVIEW_COMMENTS_UNRESOLVED", "仍有未解决审核批注，不能批准", "先解决所有适用批注")
		}
	}
	return nil
}

func isNotFound(err error) bool {
	var domainError *domain.Error
	return errors.As(err, &domainError) && domainError.Type == "not_found"
}

func normalizeSubmissionHash(value string) string {
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
	return "sha256:" + normalized
}
