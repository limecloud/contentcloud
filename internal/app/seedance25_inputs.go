package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/limecloud/contentcloud/internal/blob"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/mediapipeline"
	"github.com/limecloud/contentcloud/internal/store"
)

const (
	seedance25MaxInputBytes      int64 = 8 << 20
	seedance25MaxRequestBytes    int64 = 64 << 20
	seedance25MaxReferenceImages       = 30
)

// Seedance25ArtifactResolver turns ContentCloud's immutable prompt package and
// Artifact IDs into provider-safe data URLs. It deliberately does not create
// public links for local/object-store files in the first phase.
type Seedance25ArtifactResolver struct {
	store store.Store
	blobs blob.Store
}

func NewSeedance25ArtifactResolver(st store.Store, blobs blob.Store) *Seedance25ArtifactResolver {
	return &Seedance25ArtifactResolver{store: st, blobs: blobs}
}

func (r *Seedance25ArtifactResolver) Resolve(ctx context.Context, request mediapipeline.Request, profile domain.ProviderProfile) (mediapipeline.Seedance25Input, error) {
	if r == nil || r.store == nil || r.blobs == nil {
		return mediapipeline.Seedance25Input{}, domain.Policy("SEEDANCE_INPUT_RESOLVER_UNAVAILABLE", "Seedance Artifact 输入解析器不可用", "配置 ContentCloud Store 和 Blob Store")
	}
	promptArtifact, err := r.store.Artifact(ctx, request.TenantID, strings.TrimSpace(request.PromptPackageArtifactID))
	if err != nil {
		return mediapipeline.Seedance25Input{}, err
	}
	if promptArtifact.ProjectID != request.ProjectID || promptArtifact.ApprovedSnapshotID != request.StoryboardSnapshotID {
		return mediapipeline.Seedance25Input{}, domain.Policy("SEEDANCE_PROMPT_PACKAGE_SCOPE_INVALID", "Seedance 提示包不属于当前项目或批准快照", "重新选择当前项目的已批准提示包")
	}
	if promptArtifact.Kind != "prompt_package" || promptArtifact.MediaType != "application/json" {
		return mediapipeline.Seedance25Input{}, domain.Policy("SEEDANCE_PROMPT_PACKAGE_TYPE_INVALID", "Seedance 提示包 Artifact 类型不受支持", "使用已登记的 JSON prompt_package Artifact")
	}
	body, err := r.blobs.Get(ctx, promptArtifact.ObjectKey)
	if err != nil {
		return mediapipeline.Seedance25Input{}, err
	}
	if int64(len(body)) > seedance25MaxInputBytes {
		return mediapipeline.Seedance25Input{}, domain.Invalid("SEEDANCE_PROMPT_PACKAGE_SIZE_INVALID", "Seedance 提示包超过大小限制")
	}
	if normalizedSHA256(promptArtifact.SHA256) != normalizedSHA256(mediapipeline.SHA256(body)) {
		return mediapipeline.Seedance25Input{}, domain.Conflict("SEEDANCE_PROMPT_PACKAGE_DIGEST_MISMATCH", "Seedance 提示包 Artifact 摘要与对象内容不一致")
	}
	var promptPackage domain.SeedancePromptPackage
	if err := json.Unmarshal(body, &promptPackage); err != nil {
		return mediapipeline.Seedance25Input{}, domain.Invalid("SEEDANCE_PROMPT_PACKAGE_JSON_INVALID", "Seedance 提示包不是有效 JSON")
	}
	if err := promptPackage.Validate(); err != nil {
		return mediapipeline.Seedance25Input{}, err
	}
	if promptPackage.StoryboardSnapshotID != request.StoryboardSnapshotID || (request.ProfileVersion != "" && promptPackage.ProviderProfileVersion != request.ProfileVersion) {
		return mediapipeline.Seedance25Input{}, domain.Conflict("SEEDANCE_PROMPT_PACKAGE_STALE", "Seedance 提示包与当前快照或 Provider Profile 版本不一致")
	}
	if request.Mode == "text_to_video" && promptPackage.Mode != "text_to_video" {
		return mediapipeline.Seedance25Input{}, domain.Conflict("SEEDANCE_MODE_DRIFT", "Media Job 模式与锁定提示包模式不一致")
	}
	if request.Mode == "image_to_video" && promptPackage.Mode == "text_to_video" {
		return mediapipeline.Seedance25Input{}, domain.Conflict("SEEDANCE_MODE_DRIFT", "Media Job 模式与锁定提示包模式不一致")
	}
	if promptPackage.Mode == "extend" || promptPackage.Mode == "first_last_frame" {
		return mediapipeline.Seedance25Input{}, domain.Policy("SEEDANCE_MODE_UNSUPPORTED", "Seedance 2.5 第一阶段暂不支持续写或首尾帧组合模式", "使用 text_to_video 或 image_to_video 单镜头提示包")
	}
	if promptPackage.Mode == "text_to_video" && len(promptPackage.UploadManifest) > 0 {
		return mediapipeline.Seedance25Input{}, domain.Invalid("SEEDANCE_TEXT_INPUT_INVALID", "text_to_video 提示包不能包含图片输入")
	}
	if len(promptPackage.Segments) != 1 {
		return mediapipeline.Seedance25Input{}, domain.Policy("SEEDANCE_SINGLE_SEGMENT_REQUIRED", "Seedance 2.5 第一阶段只能执行一个分段", "拆分为单镜头任务后重试")
	}
	if len(promptPackage.UploadManifest) > seedance25MaxReferenceImages {
		return mediapipeline.Seedance25Input{}, domain.Invalid("PROVIDER_REFERENCE_LIMIT_EXCEEDED", "Seedance 2.5 图片引用超过 30 张限制")
	}
	if request.DurationSeconds != promptPackage.Settings.DurationSeconds {
		return mediapipeline.Seedance25Input{}, domain.Conflict("SEEDANCE_SETTINGS_DRIFT", "Media Job 时长与锁定提示包设置不一致")
	}
	if strings.TrimSpace(request.AspectRatio) != "" && request.AspectRatio != promptPackage.Settings.AspectRatio {
		return mediapipeline.Seedance25Input{}, domain.Conflict("SEEDANCE_SETTINGS_DRIFT", "Media Job 画幅与锁定提示包设置不一致")
	}
	segment := promptPackage.Segments[0]
	artifacts, err := r.store.ArtifactsByApprovedSnapshot(ctx, request.TenantID, request.StoryboardSnapshotID)
	if err != nil {
		return mediapipeline.Seedance25Input{}, err
	}
	lockedSnapshot, err := r.store.ApprovedSnapshot(ctx, request.TenantID, request.StoryboardSnapshotID)
	if err != nil {
		return mediapipeline.Seedance25Input{}, err
	}
	lockedAssets := map[string]domain.StoryboardAsset{}
	lockedStoryboard, ok, err := storyboardPackageFromSnapshot(lockedSnapshot)
	if err != nil {
		return mediapipeline.Seedance25Input{}, err
	}
	if !ok {
		return mediapipeline.Seedance25Input{}, domain.Invalid("STORYBOARD_PACKAGE_REQUIRED", "已批准快照不包含可绑定 Seedance 的分镜包")
	}
	for _, asset := range lockedStoryboard.Assets {
		if asset.Role != "review_sheet" {
			lockedAssets[asset.ID] = asset
		}
	}
	byStoryboardAsset := map[string]domain.Artifact{}
	for _, artifact := range artifacts {
		if assetID := metadataString(artifact.Metadata, "storyboard_asset_id"); assetID != "" {
			byStoryboardAsset[assetID] = artifact
		}
	}
	resolvedIDs := make([]string, 0, len(promptPackage.UploadManifest))
	input := mediapipeline.Seedance25Input{Prompt: strings.TrimSpace(segment.PromptZH)}
	var totalMediaBytes int64
	for _, upload := range promptPackage.UploadManifest {
		if _, ok := lockedAssets[upload.ArtifactID]; !ok {
			return mediapipeline.Seedance25Input{}, domain.Conflict("SEEDANCE_INPUT_ARTIFACT_MISMATCH", fmt.Sprintf("Seedance 输入 Artifact 不在锁定分镜资产清单中：%s", upload.ArtifactID))
		}
		artifact, ok := byStoryboardAsset[upload.ArtifactID]
		if !ok || artifact.ProjectID != request.ProjectID || normalizedSHA256(artifact.SHA256) != normalizedSHA256(upload.SHA256) {
			return mediapipeline.Seedance25Input{}, domain.Conflict("SEEDANCE_INPUT_ARTIFACT_MISMATCH", fmt.Sprintf("Seedance 输入 Artifact 与锁定清单不一致：%s", upload.ArtifactID))
		}
		resolvedIDs = append(resolvedIDs, artifact.ID)
		if artifact.MediaType != "image/jpeg" && artifact.MediaType != "image/png" && artifact.MediaType != "image/webp" {
			return mediapipeline.Seedance25Input{}, domain.Policy("SEEDANCE_INPUT_MEDIA_UNSUPPORTED", "第一阶段只接受图片 Artifact", "将视频或音频引用留到后续 Provider Profile")
		}
		mediaBody, err := r.blobs.Get(ctx, artifact.ObjectKey)
		if err != nil {
			return mediapipeline.Seedance25Input{}, err
		}
		if int64(len(mediaBody)) == 0 || int64(len(mediaBody)) > seedance25MaxInputBytes {
			return mediapipeline.Seedance25Input{}, domain.Invalid("SEEDANCE_INPUT_SIZE_INVALID", "Seedance 图片 Artifact 为空或超过大小限制")
		}
		totalMediaBytes += int64(len(mediaBody))
		if totalMediaBytes > seedance25MaxRequestBytes {
			return mediapipeline.Seedance25Input{}, domain.Invalid("SEEDANCE_INPUT_SIZE_INVALID", "Seedance 输入总大小超过请求限制")
		}
		if normalizedSHA256(artifact.SHA256) != normalizedSHA256(mediapipeline.SHA256(mediaBody)) {
			return mediapipeline.Seedance25Input{}, domain.Conflict("SEEDANCE_INPUT_DIGEST_MISMATCH", fmt.Sprintf("Seedance 输入 Artifact 内容摘要不一致：%s", upload.ArtifactID))
		}
		role := "reference_image"
		if strings.Contains(strings.ToLower(upload.Purpose), "first") {
			role = "first_frame"
		} else if strings.Contains(strings.ToLower(upload.Purpose), "last") {
			role = "last_frame"
		}
		input.Images = append(input.Images, mediapipeline.Seedance25Media{URL: "data:" + artifact.MediaType + ";base64," + base64.StdEncoding.EncodeToString(mediaBody), MediaType: artifact.MediaType, Role: role})
	}
	if len(request.InputArtifactRefs) > 0 && !sameStringSet(request.InputArtifactRefs, resolvedIDs) {
		return mediapipeline.Seedance25Input{}, domain.Conflict("SEEDANCE_INPUT_ARTIFACTS_MISMATCH", "Seedance 提示包输入与 Media Job 输入不一致")
	}
	if len(input.Prompt) == 0 {
		return mediapipeline.Seedance25Input{}, domain.Invalid("SEEDANCE_PROMPT_REQUIRED", "Seedance 单镜头提示词不能为空")
	}
	if utf8.RuneCountInString(input.Prompt) > 32000 {
		return mediapipeline.Seedance25Input{}, domain.Invalid("SEEDANCE_PROMPT_TOO_LARGE", "Seedance 单镜头提示词超过 32000 字符限制")
	}
	return input, nil
}

var _ mediapipeline.Seedance25InputResolver = (*Seedance25ArtifactResolver)(nil)

const maxSeedancePromptPackageBytes = 8 << 20

type UploadSeedancePromptPackageInput struct {
	SnapshotID string
	FileName   string
	Body       []byte
}

// UploadSeedancePromptPackage registers the immutable JSON package that the
// Provider resolver consumes. The package is checked against the approved
// storyboard before it becomes an Artifact, so a later Media Job only needs
// to carry the Artifact ID.
func (s *Service) UploadSeedancePromptPackage(ctx context.Context, actor Actor, taskID string, input UploadSeedancePromptPackageInput, requestID string) (domain.Artifact, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.Artifact{}, err
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return domain.Artifact{}, err
	}
	if task.ContentType != domain.ContentTypeMarketingVideo || (task.CurrentStageID != "storyboard" && task.CurrentStageID != "generation") {
		return domain.Artifact{}, domain.Policy("SEEDANCE_PROMPT_PACKAGE_STAGE_INVALID", "Seedance 提示包只能在分镜或视频生成阶段登记", "打开当前营销视频任务的分镜或生成阶段")
	}
	if len(input.Body) == 0 || len(input.Body) > maxSeedancePromptPackageBytes {
		return domain.Artifact{}, domain.Invalid("SEEDANCE_PROMPT_PACKAGE_SIZE_INVALID", "Seedance 提示包大小必须在 1 字节至 8 MB 之间")
	}
	snapshot, err := s.store.ApprovedSnapshot(ctx, actor.TenantID, strings.TrimSpace(input.SnapshotID))
	if err != nil {
		return domain.Artifact{}, err
	}
	if snapshot.ProjectID != task.ProjectID || snapshot.SubmissionType != "storyboard" {
		return domain.Artifact{}, domain.Policy("SEEDANCE_PROMPT_PACKAGE_SCOPE_INVALID", "Seedance 提示包必须绑定当前项目已批准的分镜快照", "选择当前项目的已批准分镜快照")
	}
	storyboard, ok, err := storyboardPackageFromSnapshot(snapshot)
	if err != nil {
		return domain.Artifact{}, err
	}
	if !ok {
		return domain.Artifact{}, domain.Invalid("STORYBOARD_PACKAGE_REQUIRED", "已批准快照不包含可绑定 Seedance 的分镜包")
	}
	var promptPackage domain.SeedancePromptPackage
	if err := json.Unmarshal(input.Body, &promptPackage); err != nil {
		return domain.Artifact{}, domain.Invalid("SEEDANCE_PROMPT_PACKAGE_JSON_INVALID", "Seedance 提示包不是有效 JSON")
	}
	if err := promptPackage.Validate(); err != nil {
		return domain.Artifact{}, err
	}
	if promptPackage.StoryboardSnapshotID != snapshot.ID || promptPackage.StoryboardPackageID != storyboard.ID || promptPackage.StoryboardLockedDigest != storyboard.LockedDigest {
		return domain.Artifact{}, domain.Conflict("SEEDANCE_PROMPT_PACKAGE_STALE", "Seedance 提示包与批准分镜快照或锁定摘要不一致")
	}
	sha := mediapipeline.SHA256(input.Body)
	existing, err := s.store.ArtifactsByApprovedSnapshot(ctx, actor.TenantID, snapshot.ID)
	if err != nil {
		return domain.Artifact{}, err
	}
	for _, artifact := range existing {
		if artifact.Kind == "prompt_package" && normalizedSHA256(artifact.SHA256) == normalizedSHA256(sha) {
			return artifact, nil
		}
	}
	fileName := filepath.Base(input.FileName)
	if fileName == "." || fileName == "" {
		fileName = "seedance-prompt-package.json"
	}
	now := s.now().UTC()
	artifactID := domain.NewID()
	objectKey := fmt.Sprintf("seedance/%s/%s/prompt-packages/%s/%s", task.TenantID, snapshot.ID, artifactID, fileName)
	if err := s.blobs.Put(ctx, objectKey, input.Body); err != nil {
		return domain.Artifact{}, err
	}
	artifact := domain.Artifact{
		ID: artifactID, TenantID: task.TenantID, ProjectID: task.ProjectID, ApprovedSnapshotID: snapshot.ID,
		Kind: "prompt_package", CapabilityID: promptPackage.AdapterCapability.ID, CapabilityVersion: promptPackage.AdapterCapability.Version,
		CapabilityDigest: promptPackage.AdapterCapability.Digest, SchemaID: promptPackage.SchemaVersion, MediaType: "application/json",
		FileName: fileName, SHA256: sha, ByteSize: int64(len(input.Body)), ObjectKey: objectKey, Visibility: "client", RetentionClass: "audit", Purpose: "seedance_prompt_package",
		Metadata: map[string]any{"task_id": task.ID, "provider_profile_version": promptPackage.ProviderProfileVersion, "prompt_package_id": promptPackage.ID, "storyboard_locked_digest": promptPackage.StoryboardLockedDigest}, CreatedAt: now,
	}
	if err := s.store.CreateArtifact(ctx, artifact); err != nil {
		if deleter, ok := s.blobs.(blob.DeleteStore); ok {
			_ = deleter.Delete(ctx, objectKey)
		}
		return domain.Artifact{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "seedance.prompt_package_uploaded", "artifact", artifact.ID, requestID, map[string]any{"snapshot_id": snapshot.ID, "sha256": normalizedSHA256(sha), "provider_profile_version": promptPackage.ProviderProfileVersion})
	return artifact, nil
}
