package application

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	mediapipeline "github.com/limecloud/contentcloud/internal/integration/provider/media"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"github.com/limecloud/contentcloud/internal/work"
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
	Artifact deliverydomain.Artifact    `json:"artifact"`
	Review   deliverydomain.MediaReview `json:"review"`
}

func (s *DeliveryService) UploadStoryboardArtifact(ctx context.Context, actor Actor, taskID string, input UploadStoryboardArtifactInput, requestID string) (deliverydomain.Artifact, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return deliverydomain.Artifact{}, err
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return deliverydomain.Artifact{}, err
	}
	if task.ContentType != identitydomain.ContentTypeMarketingVideo || (task.CurrentStageID != "storyboard" && task.CurrentStageID != "generation") {
		return deliverydomain.Artifact{}, fault.Policy("STORYBOARD_ARTIFACT_STAGE_INVALID", "分镜素材只能在分镜或视频生成阶段登记", "打开当前营销视频任务的分镜阶段")
	}
	if len(input.Body) == 0 || len(input.Body) > maxStoryboardArtifactBytes {
		return deliverydomain.Artifact{}, fault.Invalid("STORYBOARD_ARTIFACT_SIZE_INVALID", "分镜素材大小必须在 1 字节至 25 MB 之间")
	}
	snapshot, err := s.review.ApprovedSnapshot(ctx, actor.TenantID, strings.TrimSpace(input.SnapshotID))
	if err != nil {
		return deliverydomain.Artifact{}, err
	}
	if snapshot.ProjectID != task.ProjectID || snapshot.SubmissionType != "storyboard" {
		return deliverydomain.Artifact{}, fault.Policy("STORYBOARD_SNAPSHOT_SCOPE_INVALID", "分镜快照不属于当前任务项目", "选择当前项目已批准的分镜快照")
	}
	storyboard, ok, err := storyboardPackageFromSnapshot(snapshot)
	if err != nil {
		return deliverydomain.Artifact{}, err
	}
	if !ok {
		return deliverydomain.Artifact{}, fault.Invalid("STORYBOARD_PACKAGE_REQUIRED", "已批准快照不包含可登记媒体的分镜包")
	}
	var asset work.StoryboardAsset
	for _, candidate := range storyboard.Assets {
		if candidate.ID == strings.TrimSpace(input.AssetID) {
			asset = candidate
			break
		}
	}
	if asset.ID == "" {
		return deliverydomain.Artifact{}, fault.NotFound("分镜素材")
	}
	sha := mediapipeline.SHA256(input.Body)
	if !strings.EqualFold(sha, strings.TrimPrefix(asset.SHA256, "sha256:")) || (asset.ByteSize > 0 && asset.ByteSize != int64(len(input.Body))) {
		return deliverydomain.Artifact{}, fault.Conflict("STORYBOARD_ARTIFACT_DIGEST_MISMATCH", "上传素材与锁定分镜中的摘要或字节数不一致")
	}
	detectedMIME := http.DetectContentType(input.Body)
	if detectedMIME != asset.MediaType {
		return deliverydomain.Artifact{}, fault.Invalid("STORYBOARD_ARTIFACT_MIME_MISMATCH", "上传素材的实际媒体类型与锁定分镜不一致")
	}
	existing, err := s.artifacts.ArtifactsByApprovedSnapshot(ctx, actor.TenantID, snapshot.ID)
	if err != nil {
		return deliverydomain.Artifact{}, err
	}
	for _, artifact := range existing {
		if metadataString(artifact.Metadata, "storyboard_asset_id") != asset.ID {
			continue
		}
		if normalizedSHA256(artifact.SHA256) != normalizedSHA256(sha) {
			return deliverydomain.Artifact{}, fault.Conflict("STORYBOARD_ARTIFACT_ALREADY_CHANGED", "该分镜素材 ID 已绑定不同摘要")
		}
		return artifact, nil
	}
	now := s.now().UTC()
	artifactID := idgen.New()
	fileName := filepath.Base(asset.Path)
	if fileName == "." || fileName == "" {
		fileName = filepath.Base(input.FileName)
	}
	objectKey := fmt.Sprintf("storyboards/%s/%s/%s/%s", task.TenantID, snapshot.ID, artifactID, fileName)
	if err := s.blobs.Put(ctx, objectKey, input.Body); err != nil {
		return deliverydomain.Artifact{}, err
	}
	kind := "storyboard_media"
	if strings.HasPrefix(asset.MediaType, "image/") {
		kind = "storyboard_image"
	}
	artifact := deliverydomain.Artifact{
		ID: artifactID, TenantID: task.TenantID, ProjectID: task.ProjectID, ApprovedSnapshotID: snapshot.ID,
		Kind: kind, CapabilityID: storyboard.GeneratorCapability.ID, CapabilityVersion: storyboard.GeneratorCapability.Version,
		CapabilityDigest: storyboard.GeneratorCapability.Digest, SchemaID: "contentcloud.storyboard-asset/1.0",
		MediaType: asset.MediaType, FileName: fileName, SHA256: sha, ByteSize: int64(len(input.Body)), ObjectKey: objectKey,
		Visibility: "client", RetentionClass: "audit", Purpose: asset.Role,
		Metadata:  map[string]any{"task_id": task.ID, "storyboard_asset_id": asset.ID, "role": asset.Role, "shot_id": asset.ShotID, "source_path": asset.Path, "rights_refs": asset.RightsRefs, "locked_digest": storyboard.LockedDigest, "quarantined": false},
		CreatedAt: now,
	}
	if err := s.artifacts.CreateArtifact(ctx, artifact); err != nil {
		return deliverydomain.Artifact{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "storyboard.artifact_uploaded", "artifact", artifact.ID, requestID, map[string]any{"snapshot_id": snapshot.ID, "storyboard_asset_id": asset.ID, "sha256": normalizedSHA256(artifact.SHA256)})
	return artifact, nil
}

func (s *DeliveryService) CreateFinalRender(ctx context.Context, actor Actor, taskID string, input CreateFinalRenderInput, requestID string) (FinalRenderResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor", "reviewer"); err != nil {
		return FinalRenderResult{}, err
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return FinalRenderResult{}, err
	}
	if task.ContentType != identitydomain.ContentTypeMarketingVideo || task.Status != work.TaskStatusRunning || task.CurrentStageID != "postproduction" {
		return FinalRenderResult{}, fault.Policy("FINAL_RENDER_STAGE_INVALID", "最终渲染只能在运行中的后期阶段创建", "先选择候选成片并开始后期阶段")
	}
	runs, err := s.tasks.StageRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return FinalRenderResult{}, err
	}
	run, err := currentStageRun(task, runs)
	if err != nil {
		return FinalRenderResult{}, err
	}
	if input.StageRunID != "" && input.StageRunID != run.ID {
		return FinalRenderResult{}, fault.Conflict("FINAL_RENDER_STAGE_NOT_CURRENT", "最终渲染必须绑定当前流程阶段执行记录")
	}
	selected, err := s.delivery.MediaReview(ctx, actor.TenantID, strings.TrimSpace(input.SelectedReviewID))
	if err != nil {
		return FinalRenderResult{}, err
	}
	if selected.TaskID != task.ID || selected.ReviewKind != deliverydomain.MediaReviewContent || selected.Status != deliverydomain.MediaReviewApproved || !selected.Selected {
		return FinalRenderResult{}, fault.Policy("SELECTED_TAKE_REVIEW_REQUIRED", "最终渲染必须绑定当前任务已批准并选中的候选成片", "先完成成片内容质检并选择候选成片")
	}
	source, err := s.artifacts.Artifact(ctx, actor.TenantID, selected.SubjectArtifactID)
	if err != nil {
		return FinalRenderResult{}, err
	}
	if source.ProjectID != task.ProjectID || normalizedSHA256(source.SHA256) != selected.SubjectDigest || source.MediaType != "video/mp4" || source.ByteSize <= 0 {
		return FinalRenderResult{}, fault.Conflict("SELECTED_TAKE_INTEGRITY_FAILED", "选中候选成片的成果文件与审核摘要不一致")
	}
	manifestDigest, err := stablehash.Sum(struct {
		TaskID       string `json:"task_id"`
		SourceID     string `json:"source_artifact_id"`
		SourceDigest string `json:"source_digest"`
		Renderer     string `json:"renderer"`
	}{task.ID, source.ID, normalizedSHA256(source.SHA256), "contentcloud.passthrough-render/1.0"})
	if err != nil {
		return FinalRenderResult{}, err
	}
	manifestDigest = "sha256:" + manifestDigest
	existing, err := s.artifacts.ArtifactsByApprovedSnapshot(ctx, actor.TenantID, source.ApprovedSnapshotID)
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
		return FinalRenderResult{}, fault.Conflict("SELECTED_TAKE_BLOB_MISMATCH", "选中候选成片的存储文件与成果文件摘要不一致")
	}
	now := s.now().UTC()
	artifactID := idgen.New()
	fileName := "final-" + filepath.Base(source.FileName)
	objectKey := fmt.Sprintf("media/%s/%s/final/%s/%s", task.TenantID, task.ID, artifactID, fileName)
	if err := s.blobs.Put(ctx, objectKey, body); err != nil {
		return FinalRenderResult{}, err
	}
	artifact := deliverydomain.Artifact{
		ID: artifactID, TenantID: task.TenantID, ProjectID: task.ProjectID, ApprovedSnapshotID: source.ApprovedSnapshotID,
		Kind: "final_render", CapabilityID: "contentcloud.media.final-render", CapabilityVersion: "1.0.0", CapabilityDigest: manifestDigest,
		SchemaID: "contentcloud.final-render/1.0", MediaType: source.MediaType, FileName: fileName, SHA256: mediapipeline.SHA256(body), ByteSize: int64(len(body)), ObjectKey: objectKey,
		Visibility: "client", RetentionClass: "audit", Purpose: "final_video",
		Metadata:  map[string]any{"task_id": task.ID, "selected_take_artifact_id": source.ID, "selected_review_id": selected.ID, "source_digest": normalizedSHA256(source.SHA256), "render_manifest_digest": manifestDigest, "renderer": "passthrough", "quarantined": false},
		CreatedAt: now,
	}
	if err := s.artifacts.CreateArtifact(ctx, artifact); err != nil {
		return FinalRenderResult{}, err
	}
	review, err := s.ensureFinalReview(ctx, actor, task, selected, artifact)
	if err != nil {
		return FinalRenderResult{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "media.final_render_created", "artifact", artifact.ID, requestID, map[string]any{"selected_take_artifact_id": source.ID, "render_manifest_digest": manifestDigest, "sha256": normalizedSHA256(artifact.SHA256)})
	return FinalRenderResult{Artifact: artifact, Review: review}, nil
}

func (s *DeliveryService) ensureFinalReview(ctx context.Context, actor Actor, task work.WorkTask, selected deliverydomain.MediaReview, artifact deliverydomain.Artifact) (deliverydomain.MediaReview, error) {
	reviews, err := s.delivery.MediaReviews(ctx, actor.TenantID, task.ID)
	if err != nil {
		return deliverydomain.MediaReview{}, err
	}
	for _, review := range reviews {
		if review.ReviewKind == deliverydomain.MediaReviewFinal && review.SubjectArtifactID == artifact.ID {
			return review, nil
		}
	}
	now := s.now().UTC()
	review := deliverydomain.MediaReview{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, GenerationJobID: selected.GenerationJobID, SubjectArtifactID: artifact.ID, SubjectDigest: normalizedSHA256(artifact.SHA256), ReviewKind: deliverydomain.MediaReviewFinal, Status: deliverydomain.MediaReviewPending, Checks: map[string]any{}, RowVersion: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.delivery.CreateMediaReview(ctx, review); err != nil {
		return deliverydomain.MediaReview{}, err
	}
	return review, nil
}

func storyboardPackageFromSnapshot(snapshot reviewdomain.ApprovedSnapshot) (work.StoryboardPackage, bool, error) {
	var envelope struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(snapshot.CanonicalContent, &envelope); err != nil {
		return work.StoryboardPackage{}, false, fault.Invalid("STORYBOARD_SNAPSHOT_JSON_INVALID", "分镜已批准快照的正文不是有效 JSON")
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
		var value work.StoryboardPackage
		if err := json.Unmarshal(raw, &value); err != nil {
			return work.StoryboardPackage{}, false, fault.Invalid("STORYBOARD_PACKAGE_JSON_INVALID", "分镜包正文不是有效 JSON")
		}
		if err := value.Validate(true); err != nil {
			return work.StoryboardPackage{}, false, err
		}
		return value, true, nil
	}
	return work.StoryboardPackage{}, false, nil
}

func (s *DeliveryService) verifiedStoryboardInputArtifacts(ctx context.Context, tenantID string, snapshot reviewdomain.ApprovedSnapshot) ([]string, error) {
	storyboard, ok, err := storyboardPackageFromSnapshot(snapshot)
	if err != nil || !ok {
		return nil, err
	}
	stored, err := s.artifacts.ArtifactsByApprovedSnapshot(ctx, tenantID, snapshot.ID)
	if err != nil {
		return nil, err
	}
	byAssetID := map[string]deliverydomain.Artifact{}
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
			return nil, fault.Policy("STORYBOARD_ARTIFACT_BYTES_REQUIRED", "锁定分镜的媒体文件尚未完整登记", "上传所有首尾帧和参考素材后再创建视频生成任务")
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
