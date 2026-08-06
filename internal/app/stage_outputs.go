package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Service) prepareStageOutputs(ctx context.Context, actor Actor, task domain.WorkTask, run domain.StageRun, submitted []domain.TaskStageOutput, now time.Time) ([]domain.TaskStageOutput, []string, error) {
	outputs := make([]domain.TaskStageOutput, 0, len(submitted))
	refs := make([]string, 0, len(submitted))
	seen := map[string]bool{}
	createdBy := actor.UserID
	if createdBy == "" {
		createdBy = actor.DeviceID
	}
	for _, candidate := range submitted {
		candidate.ID = domain.NewID()
		candidate.TenantID = task.TenantID
		candidate.ProjectID = task.ProjectID
		candidate.TaskID = task.ID
		candidate.StageRunID = run.ID
		candidate.StageID = run.StageID
		candidate.CreatedBy = createdBy
		candidate.CreatedAt = now
		candidate.NormalizeCollections()
		if candidate.Role == "" {
			candidate.Role = domain.StageOutputRolePrimary
		}
		if candidate.Status == "" {
			candidate.Status = domain.StageOutputStatusValidated
		}
		if strings.TrimSpace(candidate.ObjectID) == "" {
			return nil, nil, domain.Invalid("TASK_STAGE_OUTPUT_INVALID", "流程阶段输出缺少规范对象标识")
		}
		key := candidate.OutputType + ":" + candidate.ObjectID + ":" + candidate.Role
		if seen[key] {
			return nil, nil, domain.Conflict("TASK_STAGE_OUTPUT_DUPLICATE", "同一流程阶段不能重复上报相同规范对象")
		}
		seen[key] = true
		actualDigest, actualVersion, actualStatus, err := s.resolveStageObject(ctx, task, candidate)
		if err != nil {
			return nil, nil, err
		}
		if candidate.ObjectDigest != "" && candidate.ObjectDigest != actualDigest {
			return nil, nil, domain.Conflict("TASK_STAGE_OUTPUT_DIGEST_MISMATCH", "流程阶段输出摘要与服务端规范对象不一致")
		}
		candidate.ObjectDigest = actualDigest
		if candidate.ObjectVersion > 0 && actualVersion > 0 && candidate.ObjectVersion != actualVersion {
			return nil, nil, domain.Conflict("TASK_STAGE_OUTPUT_VERSION_MISMATCH", "流程阶段输出版本与服务端规范对象不一致")
		}
		if candidate.ObjectVersion == 0 {
			candidate.ObjectVersion = actualVersion
		}
		candidate.Status = actualStatus
		if err := candidate.Validate(); err != nil {
			return nil, nil, err
		}
		outputs = append(outputs, candidate)
		refs = append(refs, fmt.Sprintf("%s:%s@%s", candidate.OutputType, candidate.ObjectID, candidate.ObjectDigest))
	}
	return outputs, refs, nil
}

func (s *Service) resolveStageObject(ctx context.Context, task domain.WorkTask, output domain.TaskStageOutput) (string, int, string, error) {
	requireProject := func(projectID string) error {
		if projectID != task.ProjectID {
			return domain.Policy("TASK_STAGE_OUTPUT_PROJECT_MISMATCH", "流程阶段输出不属于当前项目", "选择当前任务项目内的规范对象")
		}
		return nil
	}
	switch output.OutputType {
	case domain.StageOutputSourceRevision:
		value, err := s.store.SourceRevision(ctx, task.TenantID, output.ObjectID)
		if err != nil {
			return "", 0, "", err
		}
		if err := requireProject(value.ProjectID); err != nil {
			return "", 0, "", err
		}
		return normalizedSHA256(value.SHA256), 0, sourceRevisionOutputStatus(value), nil
	case domain.StageOutputEvidenceSet:
		revision, err := s.store.SourceRevision(ctx, task.TenantID, output.ObjectID)
		if err != nil {
			return "", 0, "", err
		}
		if err := requireProject(revision.ProjectID); err != nil {
			return "", 0, "", err
		}
		values, err := s.store.Evidence(ctx, task.TenantID, revision.ID)
		if err != nil {
			return "", 0, "", err
		}
		digest, err := domain.CanonicalHash(values)
		if err != nil {
			return "", 0, "", err
		}
		status := domain.StageOutputStatusValidated
		if len(values) == 0 {
			status = domain.StageOutputStatusBlocked
		}
		return "sha256:" + digest, 0, status, nil
	case domain.StageOutputKnowledgeObject:
		if output.ObjectVersion < 1 {
			return "", 0, "", domain.Invalid("TASK_STAGE_OUTPUT_VERSION_REQUIRED", "知识对象输出必须指定版本")
		}
		value, err := s.store.KnowledgeObject(ctx, task.TenantID, output.ObjectID, output.ObjectVersion)
		if err != nil {
			return "", 0, "", err
		}
		if err := requireProject(value.ProjectID); err != nil {
			return "", 0, "", err
		}
		return value.Digest, value.Version, knowledgeOutputStatus(value.Status), nil
	case domain.StageOutputKnowledgeSnapshot:
		value, err := s.store.KnowledgeSnapshot(ctx, task.TenantID, output.ObjectID)
		if err != nil {
			return "", 0, "", err
		}
		if err := requireProject(value.ProjectID); err != nil {
			return "", 0, "", err
		}
		return value.Digest, value.PackVersion, domain.StageOutputStatusApproved, nil
	case domain.StageOutputSubmissionRevision:
		value, err := s.store.SubmissionRevision(ctx, task.TenantID, output.ObjectID)
		if err == nil {
			if err := requireProject(value.ProjectID); err != nil {
				return "", 0, "", err
			}
			return value.ContentHash, value.RevisionNo, domain.StageOutputStatusValidated, nil
		}
		if !domain.IsNotFound(err) {
			return "", 0, "", err
		}
		if task.ContentType == domain.ContentTypeMarketingVideo {
			return "", 0, "", err
		}
		taskRevision, taskErr := s.store.TaskRevision(ctx, task.TenantID, output.ObjectID)
		if taskErr != nil {
			return "", 0, "", taskErr
		}
		if taskRevision.TaskID != task.ID || taskRevision.ProjectID != task.ProjectID {
			return "", 0, "", domain.Policy("TASK_STAGE_OUTPUT_SCOPE_INVALID", "内容版本不属于当前任务", "选择当前任务的内容版本")
		}
		return taskRevision.ContentHash, taskRevision.RevisionNo, taskRevisionOutputStatus(taskRevision.Status), nil
	case domain.StageOutputApprovedSnapshot, domain.StageOutputStoryboardPackage:
		value, err := s.store.ApprovedSnapshot(ctx, task.TenantID, output.ObjectID)
		if err != nil {
			return "", 0, "", err
		}
		if err := requireProject(value.ProjectID); err != nil {
			return "", 0, "", err
		}
		return value.SubjectHash, 0, domain.StageOutputStatusApproved, nil
	case domain.StageOutputArtifact:
		value, err := s.store.Artifact(ctx, task.TenantID, output.ObjectID)
		if err != nil {
			return "", 0, "", err
		}
		if err := requireProject(value.ProjectID); err != nil {
			return "", 0, "", err
		}
		return normalizedSHA256(value.SHA256), 0, artifactOutputStatus(value), nil
	case domain.StageOutputGenerationJob:
		value, err := s.store.MediaGenerationJob(ctx, task.TenantID, output.ObjectID)
		if err != nil {
			return "", 0, "", err
		}
		if value.TaskID != task.ID || value.ProjectID != task.ProjectID {
			return "", 0, "", domain.Policy("TASK_STAGE_OUTPUT_SCOPE_INVALID", "视频生成任务不属于当前任务", "选择当前任务的视频生成任务")
		}
		digest, err := mediaJobDigest(value)
		return digest, value.RowVersion, mediaJobOutputStatus(value.State), err
	case domain.StageOutputMediaReview:
		value, err := s.store.MediaReview(ctx, task.TenantID, output.ObjectID)
		if err != nil {
			return "", 0, "", err
		}
		if value.TaskID != task.ID || value.ProjectID != task.ProjectID {
			return "", 0, "", domain.Policy("TASK_STAGE_OUTPUT_SCOPE_INVALID", "媒体审核不属于当前任务", "选择当前任务的媒体审核")
		}
		digest, err := mediaReviewDigest(value)
		return digest, value.RowVersion, mediaReviewOutputStatus(value.Status), err
	case domain.StageOutputDeliveryPackage:
		value, err := s.store.DeliveryPackage(ctx, task.TenantID, output.ObjectID)
		if err != nil {
			return "", 0, "", err
		}
		if value.ProjectID != task.ProjectID || value.ContentItemID != task.ID {
			return "", 0, "", domain.Policy("TASK_STAGE_OUTPUT_SCOPE_INVALID", "交付包不属于当前任务", "选择当前任务的交付包")
		}
		digest, err := deliveryPackageDigest(value)
		status := domain.StageOutputStatusValidated
		if value.Status == "ready" && len(value.Manifest) > 0 {
			status = domain.StageOutputStatusApproved
		}
		return digest, 0, status, err
	default:
		return "", 0, "", domain.Invalid("TASK_STAGE_OUTPUT_TYPE_INVALID", "流程阶段输出类型不受支持")
	}
}

func validateStageOutputContract(stage domain.StageDefinition, outputs []domain.TaskStageOutput, strict bool) error {
	if len(stage.RequiredOutputTypes) == 0 {
		if strict && stage.CompletionPolicy != domain.StageCompletionControlOnly {
			return domain.Policy("STAGE_OUTPUT_CONTRACT_REQUIRED", "营销视频流程阶段缺少类型化输出契约", "使用 V7 营销视频流程规范或补齐流程阶段输出契约")
		}
		return nil
	}
	matchedAny := false
	for _, requirement := range stage.RequiredOutputTypes {
		minimum := requirement.MinCount
		if minimum < 1 {
			minimum = 1
		}
		count := 0
		for _, output := range outputs {
			if output.OutputType != requirement.OutputType || (requirement.Role != "" && output.Role != requirement.Role) || !stageOutputStatusAtLeast(output.Status, requirement.MinStatus) {
				continue
			}
			count++
		}
		if count > 0 {
			matchedAny = true
		}
		if stage.CompletionPolicy != domain.StageCompletionAtLeastOne && count < minimum {
			return domain.Policy("STAGE_REQUIRED_OUTPUT_MISSING", "流程阶段缺少满足契约的规范输出", fmt.Sprintf("补充 %s 类型、%s 角色的输出", requirement.OutputType, requirement.Role))
		}
	}
	if stage.CompletionPolicy == domain.StageCompletionAtLeastOne && !matchedAny {
		return domain.Policy("STAGE_REQUIRED_OUTPUT_MISSING", "流程阶段至少需要一个满足契约的规范输出", "补充当前流程阶段要求的输出")
	}
	return nil
}

func stageOutputStatusAtLeast(actual, minimum string) bool {
	if actual == domain.StageOutputStatusBlocked || actual == domain.StageOutputStatusFailed {
		return false
	}
	if minimum == "" {
		return true
	}
	rank := map[string]int{domain.StageOutputStatusCandidate: 1, domain.StageOutputStatusValidated: 2, domain.StageOutputStatusApproved: 3}
	return rank[actual] >= rank[minimum]
}

func normalizedSHA256(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(value, "sha256:") {
		value = "sha256:" + value
	}
	return value
}

func sourceRevisionOutputStatus(value domain.SourceRevision) string {
	if value.ProcessingStatus == "ready" || value.ProcessingStatus == "accepted" {
		return domain.StageOutputStatusValidated
	}
	if value.ProcessingStatus == "failed" {
		return domain.StageOutputStatusFailed
	}
	return domain.StageOutputStatusCandidate
}

func knowledgeOutputStatus(value string) string {
	switch value {
	case "verified", "approved", "valid", "active":
		return domain.StageOutputStatusApproved
	case "rejected", "blocked", "expired":
		return domain.StageOutputStatusBlocked
	default:
		return domain.StageOutputStatusCandidate
	}
}

func taskRevisionOutputStatus(value string) string {
	if value == domain.TaskRevisionAccepted {
		return domain.StageOutputStatusApproved
	}
	if value == domain.TaskRevisionRejected {
		return domain.StageOutputStatusBlocked
	}
	return domain.StageOutputStatusValidated
}

func artifactOutputStatus(value domain.Artifact) string {
	if quarantined, _ := value.Metadata["quarantined"].(bool); quarantined {
		return domain.StageOutputStatusBlocked
	}
	if value.ByteSize <= 0 || normalizedSHA256(value.SHA256) == "sha256:" {
		return domain.StageOutputStatusFailed
	}
	return domain.StageOutputStatusValidated
}

func mediaJobDigest(value domain.MediaGenerationJob) (string, error) {
	hash, err := domain.CanonicalHash(struct {
		ID              string `json:"id"`
		ProfileDigest   string `json:"profile_digest"`
		StoryboardID    string `json:"storyboard_snapshot_id"`
		State           string `json:"state"`
		RowVersion      int    `json:"row_version"`
		ActualCostMinor int64  `json:"actual_cost_minor"`
	}{value.ID, value.ProfileDigest, value.StoryboardSnapshotID, value.State, value.RowVersion, value.ActualCostMinor})
	return "sha256:" + hash, err
}

func mediaJobOutputStatus(value string) string {
	switch value {
	case domain.MediaJobSucceeded:
		return domain.StageOutputStatusValidated
	case domain.MediaJobFailed, domain.MediaJobOutputInvalid, domain.MediaJobCancelled:
		return domain.StageOutputStatusFailed
	case domain.MediaJobBudgetBlocked:
		return domain.StageOutputStatusBlocked
	default:
		return domain.StageOutputStatusCandidate
	}
}

func mediaReviewDigest(value domain.MediaReview) (string, error) {
	hash, err := domain.CanonicalHash(struct {
		ID            string         `json:"id"`
		SubjectDigest string         `json:"subject_digest"`
		ReviewKind    string         `json:"review_kind"`
		Status        string         `json:"status"`
		Checks        map[string]any `json:"checks"`
		Selected      bool           `json:"selected"`
		RowVersion    int            `json:"row_version"`
	}{value.ID, value.SubjectDigest, value.ReviewKind, value.Status, value.Checks, value.Selected, value.RowVersion})
	return "sha256:" + hash, err
}

func mediaReviewOutputStatus(value string) string {
	switch value {
	case domain.MediaReviewApproved:
		return domain.StageOutputStatusApproved
	case domain.MediaReviewChanges, domain.MediaReviewRejected:
		return domain.StageOutputStatusBlocked
	default:
		return domain.StageOutputStatusCandidate
	}
}

func deliveryPackageDigest(value domain.DeliveryPackage) (string, error) {
	type artifactDigest struct {
		ID     string `json:"id"`
		SHA256 string `json:"sha256"`
	}
	manifest := make([]artifactDigest, 0, len(value.Manifest))
	for _, artifact := range value.Manifest {
		manifest = append(manifest, artifactDigest{ID: artifact.ID, SHA256: normalizedSHA256(artifact.SHA256)})
	}
	hash, err := domain.CanonicalHash(struct {
		ID            string           `json:"id"`
		ContentItemID string           `json:"content_item_id"`
		Status        string           `json:"status"`
		Manifest      []artifactDigest `json:"manifest"`
	}{value.ID, value.ContentItemID, value.Status, manifest})
	return "sha256:" + hash, err
}
