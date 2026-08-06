package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/mediapipeline"
)

const maxStoryboardArtifactBytes = 25 * 1024 * 1024

type UploadStoryboardArtifactInput struct {
	SnapshotID string
	AssetID    string
	FileName   string
	Body       []byte
}

type CreateFinalRenderInput struct {
	StageRunID       string `json:"stage_run_id"`
	SelectedReviewID string `json:"selected_review_id"`
}

type FinalRenderResult struct {
	Artifact domain.Artifact    `json:"artifact"`
	Review   domain.MediaReview `json:"review"`
}

func (s *Service) UploadStoryboardArtifact(ctx context.Context, actor Actor, taskID string, input UploadStoryboardArtifactInput, requestID string) (domain.Artifact, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.Artifact{}, err
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return domain.Artifact{}, err
	}
	if task.ContentType != domain.ContentTypeMarketingVideo || (task.CurrentStageID != "storyboard" && task.CurrentStageID != "generation") {
		return domain.Artifact{}, domain.Policy("STORYBOARD_ARTIFACT_STAGE_INVALID", "分镜素材只能在分镜或视频生成阶段登记", "打开当前营销视频任务的分镜阶段")
	}
	if len(input.Body) == 0 || len(input.Body) > maxStoryboardArtifactBytes {
		return domain.Artifact{}, domain.Invalid("STORYBOARD_ARTIFACT_SIZE_INVALID", "分镜素材大小必须在 1 字节至 25 MB 之间")
	}
	snapshot, err := s.store.ApprovedSnapshot(ctx, actor.TenantID, strings.TrimSpace(input.SnapshotID))
	if err != nil {
		return domain.Artifact{}, err
	}
	if snapshot.ProjectID != task.ProjectID || snapshot.SubmissionType != "storyboard" {
		return domain.Artifact{}, domain.Policy("STORYBOARD_SNAPSHOT_SCOPE_INVALID", "分镜快照不属于当前任务项目", "选择当前项目已批准的分镜快照")
	}
	storyboard, ok, err := storyboardPackageFromSnapshot(snapshot)
	if err != nil {
		return domain.Artifact{}, err
	}
	if !ok {
		return domain.Artifact{}, domain.Invalid("STORYBOARD_PACKAGE_REQUIRED", "已批准快照不包含可登记媒体的分镜包")
	}
	var asset domain.StoryboardAsset
	for _, candidate := range storyboard.Assets {
		if candidate.ID == strings.TrimSpace(input.AssetID) {
			asset = candidate
			break
		}
	}
	if asset.ID == "" {
		return domain.Artifact{}, domain.NotFound("分镜素材")
	}
	sha := mediapipeline.SHA256(input.Body)
	if !strings.EqualFold(sha, strings.TrimPrefix(asset.SHA256, "sha256:")) || (asset.ByteSize > 0 && asset.ByteSize != int64(len(input.Body))) {
		return domain.Artifact{}, domain.Conflict("STORYBOARD_ARTIFACT_DIGEST_MISMATCH", "上传素材与锁定分镜中的摘要或字节数不一致")
	}
	detectedMIME := http.DetectContentType(input.Body)
	if detectedMIME != asset.MediaType {
		return domain.Artifact{}, domain.Invalid("STORYBOARD_ARTIFACT_MIME_MISMATCH", "上传素材的实际媒体类型与锁定分镜不一致")
	}
	existing, err := s.store.ArtifactsByApprovedSnapshot(ctx, actor.TenantID, snapshot.ID)
	if err != nil {
		return domain.Artifact{}, err
	}
	for _, artifact := range existing {
		if metadataString(artifact.Metadata, "storyboard_asset_id") != asset.ID {
			continue
		}
		if normalizedSHA256(artifact.SHA256) != normalizedSHA256(sha) {
			return domain.Artifact{}, domain.Conflict("STORYBOARD_ARTIFACT_ALREADY_CHANGED", "该分镜素材 ID 已绑定不同摘要")
		}
		return artifact, nil
	}
	now := s.now().UTC()
	artifactID := domain.NewID()
	fileName := filepath.Base(asset.Path)
	if fileName == "." || fileName == "" {
		fileName = filepath.Base(input.FileName)
	}
	objectKey := fmt.Sprintf("storyboards/%s/%s/%s/%s", task.TenantID, snapshot.ID, artifactID, fileName)
	if err := s.blobs.Put(ctx, objectKey, input.Body); err != nil {
		return domain.Artifact{}, err
	}
	kind := "storyboard_media"
	if strings.HasPrefix(asset.MediaType, "image/") {
		kind = "storyboard_image"
	}
	artifact := domain.Artifact{
		ID: artifactID, TenantID: task.TenantID, ProjectID: task.ProjectID, ApprovedSnapshotID: snapshot.ID,
		Kind: kind, CapabilityID: storyboard.GeneratorCapability.ID, CapabilityVersion: storyboard.GeneratorCapability.Version,
		CapabilityDigest: storyboard.GeneratorCapability.Digest, SchemaID: "contentcloud.storyboard-asset/1.0",
		MediaType: asset.MediaType, FileName: fileName, SHA256: sha, ByteSize: int64(len(input.Body)), ObjectKey: objectKey,
		Visibility: "client", RetentionClass: "audit", Purpose: asset.Role,
		Metadata:  map[string]any{"task_id": task.ID, "storyboard_asset_id": asset.ID, "role": asset.Role, "shot_id": asset.ShotID, "source_path": asset.Path, "rights_refs": asset.RightsRefs, "locked_digest": storyboard.LockedDigest, "quarantined": false},
		CreatedAt: now,
	}
	if err := s.store.CreateArtifact(ctx, artifact); err != nil {
		return domain.Artifact{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "storyboard.artifact_uploaded", "artifact", artifact.ID, requestID, map[string]any{"snapshot_id": snapshot.ID, "storyboard_asset_id": asset.ID, "sha256": normalizedSHA256(artifact.SHA256)})
	return artifact, nil
}

func (s *Service) CreateFinalRender(ctx context.Context, actor Actor, taskID string, input CreateFinalRenderInput, requestID string) (FinalRenderResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor", "reviewer"); err != nil {
		return FinalRenderResult{}, err
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return FinalRenderResult{}, err
	}
	if task.ContentType != domain.ContentTypeMarketingVideo || task.Status != domain.TaskStatusRunning || task.CurrentStageID != "postproduction" {
		return FinalRenderResult{}, domain.Policy("FINAL_RENDER_STAGE_INVALID", "最终渲染只能在运行中的后期阶段创建", "先选择候选成片并开始后期阶段")
	}
	runs, err := s.store.StageRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return FinalRenderResult{}, err
	}
	run, err := currentStageRun(task, runs)
	if err != nil {
		return FinalRenderResult{}, err
	}
	if input.StageRunID != "" && input.StageRunID != run.ID {
		return FinalRenderResult{}, domain.Conflict("FINAL_RENDER_STAGE_NOT_CURRENT", "最终渲染必须绑定当前流程阶段执行记录")
	}
	selected, err := s.store.MediaReview(ctx, actor.TenantID, strings.TrimSpace(input.SelectedReviewID))
	if err != nil {
		return FinalRenderResult{}, err
	}
	if selected.TaskID != task.ID || selected.ReviewKind != domain.MediaReviewContent || selected.Status != domain.MediaReviewApproved || !selected.Selected {
		return FinalRenderResult{}, domain.Policy("SELECTED_TAKE_REVIEW_REQUIRED", "最终渲染必须绑定当前任务已批准并选中的候选成片", "先完成成片内容质检并选择候选成片")
	}
	source, err := s.store.Artifact(ctx, actor.TenantID, selected.SubjectArtifactID)
	if err != nil {
		return FinalRenderResult{}, err
	}
	if source.ProjectID != task.ProjectID || normalizedSHA256(source.SHA256) != selected.SubjectDigest || source.MediaType != "video/mp4" || source.ByteSize <= 0 {
		return FinalRenderResult{}, domain.Conflict("SELECTED_TAKE_INTEGRITY_FAILED", "选中候选成片的成果文件与审核摘要不一致")
	}
	manifestDigest, err := domain.CanonicalHash(struct {
		TaskID       string `json:"task_id"`
		SourceID     string `json:"source_artifact_id"`
		SourceDigest string `json:"source_digest"`
		Renderer     string `json:"renderer"`
	}{task.ID, source.ID, normalizedSHA256(source.SHA256), "contentcloud.passthrough-render/1.0"})
	if err != nil {
		return FinalRenderResult{}, err
	}
	manifestDigest = "sha256:" + manifestDigest
	existing, err := s.store.ArtifactsByApprovedSnapshot(ctx, actor.TenantID, source.ApprovedSnapshotID)
	if err != nil {
		return FinalRenderResult{}, err
	}
	for _, artifact := range existing {
		if artifact.Kind == "final_render" && metadataString(artifact.Metadata, "render_manifest_digest") == manifestDigest {
			review, ensureErr := s.ensureFinalReview(ctx, actor, task, selected, artifact)
			return FinalRenderResult{Artifact: artifact, Review: review}, ensureErr
		}
	}
	body, err := s.blobs.Get(ctx, source.ObjectKey)
	if err != nil {
		return FinalRenderResult{}, err
	}
	if int64(len(body)) != source.ByteSize || normalizedSHA256(mediapipeline.SHA256(body)) != normalizedSHA256(source.SHA256) {
		return FinalRenderResult{}, domain.Conflict("SELECTED_TAKE_BLOB_MISMATCH", "选中候选成片的存储文件与成果文件摘要不一致")
	}
	now := s.now().UTC()
	artifactID := domain.NewID()
	fileName := "final-" + filepath.Base(source.FileName)
	objectKey := fmt.Sprintf("media/%s/%s/final/%s/%s", task.TenantID, task.ID, artifactID, fileName)
	if err := s.blobs.Put(ctx, objectKey, body); err != nil {
		return FinalRenderResult{}, err
	}
	artifact := domain.Artifact{
		ID: artifactID, TenantID: task.TenantID, ProjectID: task.ProjectID, ApprovedSnapshotID: source.ApprovedSnapshotID,
		Kind: "final_render", CapabilityID: "contentcloud.media.final-render", CapabilityVersion: "1.0.0", CapabilityDigest: manifestDigest,
		SchemaID: "contentcloud.final-render/1.0", MediaType: source.MediaType, FileName: fileName, SHA256: mediapipeline.SHA256(body), ByteSize: int64(len(body)), ObjectKey: objectKey,
		Visibility: "client", RetentionClass: "audit", Purpose: "final_video",
		Metadata:  map[string]any{"task_id": task.ID, "selected_take_artifact_id": source.ID, "selected_review_id": selected.ID, "source_digest": normalizedSHA256(source.SHA256), "render_manifest_digest": manifestDigest, "renderer": "passthrough", "quarantined": false},
		CreatedAt: now,
	}
	if err := s.store.CreateArtifact(ctx, artifact); err != nil {
		return FinalRenderResult{}, err
	}
	review, err := s.ensureFinalReview(ctx, actor, task, selected, artifact)
	if err != nil {
		return FinalRenderResult{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "media.final_render_created", "artifact", artifact.ID, requestID, map[string]any{"selected_take_artifact_id": source.ID, "render_manifest_digest": manifestDigest, "sha256": normalizedSHA256(artifact.SHA256)})
	return FinalRenderResult{Artifact: artifact, Review: review}, nil
}

func (s *Service) ensureFinalReview(ctx context.Context, actor Actor, task domain.WorkTask, selected domain.MediaReview, artifact domain.Artifact) (domain.MediaReview, error) {
	reviews, err := s.store.MediaReviews(ctx, actor.TenantID, task.ID)
	if err != nil {
		return domain.MediaReview{}, err
	}
	for _, review := range reviews {
		if review.ReviewKind == domain.MediaReviewFinal && review.SubjectArtifactID == artifact.ID {
			return review, nil
		}
	}
	now := s.now().UTC()
	review := domain.MediaReview{ID: domain.NewID(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, GenerationJobID: selected.GenerationJobID, SubjectArtifactID: artifact.ID, SubjectDigest: normalizedSHA256(artifact.SHA256), ReviewKind: domain.MediaReviewFinal, Status: domain.MediaReviewPending, Checks: map[string]any{}, RowVersion: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateMediaReview(ctx, review); err != nil {
		return domain.MediaReview{}, err
	}
	return review, nil
}

func storyboardPackageFromSnapshot(snapshot domain.ApprovedSnapshot) (domain.StoryboardPackage, bool, error) {
	var envelope struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(snapshot.CanonicalContent, &envelope); err != nil {
		return domain.StoryboardPackage{}, false, domain.Invalid("STORYBOARD_SNAPSHOT_JSON_INVALID", "分镜已批准快照的正文不是有效 JSON")
	}
	candidates := envelope.Objects
	if len(candidates) == 0 {
		candidates = []json.RawMessage{snapshot.CanonicalContent}
	}
	for _, raw := range candidates {
		var identity struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Type != "storyboard_package" {
			continue
		}
		var value domain.StoryboardPackage
		if err := json.Unmarshal(raw, &value); err != nil {
			return domain.StoryboardPackage{}, false, domain.Invalid("STORYBOARD_PACKAGE_JSON_INVALID", "分镜包正文不是有效 JSON")
		}
		if err := value.Validate(true); err != nil {
			return domain.StoryboardPackage{}, false, err
		}
		return value, true, nil
	}
	return domain.StoryboardPackage{}, false, nil
}

func (s *Service) verifiedStoryboardInputArtifacts(ctx context.Context, tenantID string, snapshot domain.ApprovedSnapshot) ([]string, error) {
	storyboard, ok, err := storyboardPackageFromSnapshot(snapshot)
	if err != nil || !ok {
		return nil, err
	}
	stored, err := s.store.ArtifactsByApprovedSnapshot(ctx, tenantID, snapshot.ID)
	if err != nil {
		return nil, err
	}
	byAssetID := map[string]domain.Artifact{}
	for _, artifact := range stored {
		if id := metadataString(artifact.Metadata, "storyboard_asset_id"); id != "" {
			byAssetID[id] = artifact
		}
	}
	refs := []string{}
	for _, asset := range storyboard.Assets {
		if asset.Role == "review_sheet" {
			continue
		}
		artifact, exists := byAssetID[asset.ID]
		if !exists || normalizedSHA256(artifact.SHA256) != normalizedSHA256(asset.SHA256) || (asset.ByteSize > 0 && artifact.ByteSize != asset.ByteSize) {
			return nil, domain.Policy("STORYBOARD_ARTIFACT_BYTES_REQUIRED", "锁定分镜的媒体文件尚未完整登记", "上传所有首尾帧和参考素材后再创建视频生成任务")
		}
		refs = append(refs, artifact.ID)
	}
	return refs, nil
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string{}, left...)
	right = append([]string{}, right...)
	sort.Strings(left)
	sort.Strings(right)
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func metadataString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
