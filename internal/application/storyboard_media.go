package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	composition "github.com/limecloud/contentcloud/internal/integration/composition"
	mediapipeline "github.com/limecloud/contentcloud/internal/integration/provider/media"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
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
	StageRunID       string                              `json:"stage_run_id"`
	SelectedReviewID string                              `json:"selected_review_id"`
	Manifest         *deliverydomain.CompositionManifest `json:"manifest"`
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
	if err := s.persistArtifactObject(ctx, artifact, input.Body); err != nil {
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
	if input.Manifest == nil {
		return FinalRenderResult{}, fault.Invalid("FINAL_RENDER_MANIFEST_REQUIRED", "最终渲染必须提供版本化合成清单")
	}
	if err := input.Manifest.Validate(); err != nil {
		return FinalRenderResult{}, err
	}
	manifest := *input.Manifest
	snapshot, err := s.review.ApprovedSnapshot(ctx, actor.TenantID, manifest.ApprovedSnapshot.ID)
	if err != nil {
		return FinalRenderResult{}, err
	}
	if snapshot.TenantID != actor.TenantID || snapshot.ProjectID != task.ProjectID {
		return FinalRenderResult{}, fault.Policy("FINAL_RENDER_SNAPSHOT_SCOPE_INVALID", "合成清单的批准快照不属于当前租户或项目", "选择当前任务项目已批准的快照")
	}
	snapshotDigest := snapshot.SubjectHash
	if strings.TrimSpace(snapshotDigest) == "" {
		snapshotDigest = snapshot.ContentHash
	}
	if normalizedSHA256(snapshotDigest) != normalizedSHA256(manifest.ApprovedSnapshot.Digest) {
		return FinalRenderResult{}, fault.Conflict("FINAL_RENDER_SNAPSHOT_DIGEST_MISMATCH", "批准快照摘要已漂移，合成清单已失效")
	}
	selected, err := s.delivery.MediaReview(ctx, actor.TenantID, strings.TrimSpace(input.SelectedReviewID))
	if err != nil {
		return FinalRenderResult{}, err
	}
	if selected.TaskID != task.ID || selected.ReviewKind != deliverydomain.MediaReviewContent || selected.Status != deliverydomain.MediaReviewApproved || !selected.Selected {
		return FinalRenderResult{}, fault.Policy("SELECTED_TAKE_REVIEW_REQUIRED", "最终渲染必须绑定当前任务已批准并选中的候选成片", "先完成成片内容质检并选择候选成片")
	}
	if selected.ID != manifest.SelectedVideo.MediaReviewID || selected.SubjectArtifactID != manifest.SelectedVideo.ArtifactID {
		return FinalRenderResult{}, fault.Conflict("FINAL_RENDER_SELECTED_VIDEO_MISMATCH", "合成清单与选中的视频审核不一致")
	}
	reviewDigest, err := mediaReviewDigest(selected)
	if err != nil {
		return FinalRenderResult{}, err
	}
	if normalizedSHA256(reviewDigest) != normalizedSHA256(manifest.SelectedVideo.ReviewDigest) {
		return FinalRenderResult{}, fault.Conflict("FINAL_RENDER_REVIEW_DIGEST_MISMATCH", "选中视频审核摘要已漂移，合成清单已失效")
	}
	source, err := s.artifacts.Artifact(ctx, actor.TenantID, selected.SubjectArtifactID)
	if err != nil {
		return FinalRenderResult{}, err
	}
	if source.ProjectID != task.ProjectID || normalizedSHA256(source.SHA256) != selected.SubjectDigest || source.MediaType != "video/mp4" || source.ByteSize <= 0 {
		return FinalRenderResult{}, fault.Conflict("SELECTED_TAKE_INTEGRITY_FAILED", "选中候选成片的成果文件与审核摘要不一致")
	}
	if source.ApprovedSnapshotID != snapshot.ID || normalizedSHA256(source.SHA256) != normalizedSHA256(manifest.SelectedVideo.Digest) {
		return FinalRenderResult{}, fault.Conflict("FINAL_RENDER_SOURCE_DIGEST_MISMATCH", "合成清单与选中视频成果摘要不一致")
	}
	if manifest.Output.MediaType != source.MediaType || manifest.Output.Container != "mp4" {
		return FinalRenderResult{}, fault.Policy("FINAL_RENDER_OUTPUT_CONTRACT_INVALID", "最终渲染输出契约与当前媒体成果不一致", "使用 video/mp4 和 mp4 输出契约")
	}
	if s.compositionWorker == nil {
		return FinalRenderResult{}, fault.Policy("FINAL_RENDER_RENDERER_UNSUPPORTED", "当前任务未绑定已批准的合成 Worker", "配置受控 Composition Worker 后重试")
	}
	manifestDigest, err := manifest.Digest()
	if err != nil {
		return FinalRenderResult{}, err
	}
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
	auxiliary, err := s.loadCompositionInputs(ctx, actor.TenantID, task.ProjectID, snapshot.ID, manifest, source)
	if err != nil {
		return FinalRenderResult{}, err
	}
	composed, err := s.compositionWorker.Compose(ctx, composition.Request{
		Manifest: manifest, ManifestDigest: manifestDigest,
		SourceArtifact: composition.InputArtifact{ID: source.ID, Digest: normalizedSHA256(source.SHA256), MediaType: source.MediaType, FileName: source.FileName, Body: body},
		Auxiliary:      auxiliary,
	})
	if err != nil {
		return FinalRenderResult{}, err
	}
	if err := validateMediaFileName(composed.FileName); err != nil {
		return FinalRenderResult{}, err
	}
	if _, err := mediapipeline.ValidateDownload(mediapipeline.Download{Body: composed.Body, MediaType: composed.MediaType, FileName: composed.FileName}, 100<<20); err != nil {
		return FinalRenderResult{}, fault.Invalid("FINAL_RENDER_OUTPUT_INVALID", "合成 Worker 输出未通过 MP4 完整性校验")
	}
	body = composed.Body
	now := s.now().UTC()
	artifactID := idgen.New()
	fileName := "final-" + filepath.Base(composed.FileName)
	objectKey := fmt.Sprintf("media/%s/%s/final/%s/%s", task.TenantID, task.ID, artifactID, fileName)
	if err := s.blobs.Put(ctx, objectKey, body); err != nil {
		return FinalRenderResult{}, s.cleanupFinalRenderBlob(ctx, task, requestID, manifestDigest, objectKey, err)
	}
	artifact := deliverydomain.Artifact{
		ID: artifactID, TenantID: task.TenantID, ProjectID: task.ProjectID, ApprovedSnapshotID: source.ApprovedSnapshotID,
		Kind: "final_render", CapabilityID: "contentcloud.media.final-render", CapabilityVersion: "1.0.0", CapabilityDigest: manifestDigest,
		SchemaID: "contentcloud.final-render/1.0", MediaType: source.MediaType, FileName: fileName, SHA256: mediapipeline.SHA256(body), ByteSize: int64(len(body)), ObjectKey: objectKey,
		Visibility: "client", RetentionClass: "audit", Purpose: "final_video",
		Metadata:  map[string]any{"task_id": task.ID, "approved_snapshot_id": snapshot.ID, "selected_take_artifact_id": source.ID, "selected_review_id": selected.ID, "source_digest": normalizedSHA256(source.SHA256), "render_manifest_digest": manifestDigest, "renderer": manifest.Renderer.Name, "renderer_version": manifest.Renderer.Version, "narration_status": manifest.Narration.Status, "subtitles_status": manifest.Subtitles.Status, "composition_technical": composed.Technical, "composition_input_count": len(auxiliary) + 1, "quarantined": false},
		CreatedAt: now,
	}
	review := deliverydomain.MediaReview{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, GenerationJobID: selected.GenerationJobID, SubjectArtifactID: artifact.ID, SubjectDigest: normalizedSHA256(artifact.SHA256), ReviewKind: deliverydomain.MediaReviewFinal, Status: deliverydomain.MediaReviewPending, Checks: map[string]any{}, RowVersion: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if writer, ok := s.artifacts.(interface {
		CreateFinalRender(context.Context, deliverydomain.Artifact, deliverydomain.MediaReview) error
	}); ok {
		if err := writer.CreateFinalRender(ctx, artifact, review); err != nil {
			return FinalRenderResult{}, s.cleanupFinalRenderBlob(ctx, task, requestID, manifestDigest, objectKey, err)
		}
	} else {
		return FinalRenderResult{}, s.cleanupFinalRenderBlob(ctx, task, requestID, manifestDigest, objectKey, fault.Policy("FINAL_RENDER_ATOMIC_STORE_REQUIRED", "最终成片必须由支持 Artifact 与最终审核原子写入的存储实现", "升级存储适配器后重试"))
	}
	s.audit(ctx, actor, task.ProjectID, "media.final_render_created", "artifact", artifact.ID, requestID, map[string]any{"selected_take_artifact_id": source.ID, "render_manifest_digest": manifestDigest, "sha256": normalizedSHA256(artifact.SHA256)})
	return FinalRenderResult{Artifact: artifact, Review: review}, nil
}

// cleanupFinalRenderBlob removes a speculative object after the database
// transaction rejects the corresponding Artifact and final review. A cleanup
// failure is promoted to a structured error so operators can recover it.
func cleanupFinalRenderBlob(ctx context.Context, blobs blob.Store, objectKey string, cause error) error {
	deleter, ok := blobs.(blob.DeleteStore)
	if !ok {
		return cause
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	cleanupErr := deleter.Delete(cleanupCtx, objectKey)
	if cleanupErr == nil || errors.Is(cleanupErr, blob.ErrNotFound) {
		return cause
	}
	if domainErr, ok := cause.(*fault.Error); ok {
		copy := *domainErr
		copy.Retryable = true
		copy.Message += "；临时对象清理失败"
		copy.Details = map[string]any{"object_key": objectKey, "cleanup_error": cleanupErr.Error(), "cause": cause.Error()}
		return &copy
	}
	return &fault.Error{Type: "storage", Subtype: "consistency", Code: "FINAL_RENDER_BLOB_CLEANUP_FAILED", Message: "最终成片数据库写入失败且临时对象清理失败", Retryable: true, Hint: "根据 object_key 清理孤立对象后重试", Details: map[string]any{"object_key": objectKey, "cleanup_error": cleanupErr.Error(), "cause": cause.Error()}, ExitCode: 6}
}

func (s *DeliveryService) cleanupFinalRenderBlob(ctx context.Context, task work.WorkTask, requestID, manifestDigest, objectKey string, cause error) error {
	err := cleanupFinalRenderBlob(ctx, s.blobs, objectKey, cause)
	if err == nil || err == cause || !isFinalRenderCleanupFailure(err) {
		return err
	}
	if s.runtimeRepo != nil {
		diagnostic := runtimeCleanupDiagnosticForFinalRender(task, requestID, manifestDigest, objectKey, cause, s.now().UTC())
		if diagnosticErr := s.runtimeRepo.CreateRuntimeCleanupDiagnostic(ctx, diagnostic); diagnosticErr != nil {
			// The original fact error and cleanup failure remain authoritative. A
			// diagnostic persistence failure must never hide either one.
			s.log.Error("persist final render cleanup diagnostic", "error", diagnosticErr, "object_key", objectKey)
		}
	}
	return err
}

func isFinalRenderCleanupFailure(err error) bool {
	var domainErr *fault.Error
	if !errors.As(err, &domainErr) {
		return false
	}
	if details, ok := domainErr.Details.(map[string]any); ok {
		_, hasCleanupError := details["cleanup_error"]
		_, hasObjectKey := details["object_key"]
		return hasCleanupError && hasObjectKey
	}
	return false
}

func runtimeCleanupDiagnosticForFinalRender(task work.WorkTask, requestID, manifestDigest, objectKey string, cause error, now time.Time) contentruntime.RuntimeCleanupDiagnostic {
	if strings.TrimSpace(requestID) == "" {
		requestID = "req_" + idgen.New()
	}
	causeCode := "FINAL_RENDER_STORE_FAILED"
	if domainErr, ok := cause.(*fault.Error); ok && strings.TrimSpace(domainErr.Code) != "" {
		causeCode = domainErr.Code
	}
	return contentruntime.RuntimeCleanupDiagnostic{
		ID:             idgen.New(),
		TenantID:       task.TenantID,
		ProjectID:      task.ProjectID,
		TaskID:         task.ID,
		RequestID:      requestID,
		ManifestDigest: manifestDigest,
		ObjectKey:      objectKey,
		CauseCode:      causeCode,
		CauseSummary:   "最终成片事实写入失败",
		CleanupError:   "BLOB_DELETE_FAILED",
		Status:         contentruntime.RuntimeCleanupPending,
		AttemptCount:   0,
		CreatedAt:      now,
		UpdatedAt:      now,
		Version:        1,
	}
}

func (s *DeliveryService) loadCompositionInputs(ctx context.Context, tenantID, projectID, snapshotID string, manifest deliverydomain.CompositionManifest, source deliverydomain.Artifact) ([]composition.InputArtifact, error) {
	artifacts := map[string]deliverydomain.Artifact{source.ID: source}
	sourceReferenced := false
	inputs := map[string]composition.InputArtifact{}
	for _, segment := range manifest.Timeline.Segments {
		artifact, ok := artifacts[segment.ArtifactID]
		if !ok {
			var err error
			artifact, err = s.artifacts.Artifact(ctx, tenantID, segment.ArtifactID)
			if err != nil {
				return nil, err
			}
			artifacts[artifact.ID] = artifact
		}
		if segment.ArtifactID == source.ID {
			sourceReferenced = true
		}
		if artifact.TenantID != tenantID || artifact.ProjectID != projectID || artifact.ApprovedSnapshotID != snapshotID || normalizedSHA256(artifact.SHA256) != normalizedSHA256(segment.ArtifactDigest) {
			return nil, fault.Conflict("FINAL_RENDER_TIMELINE_ARTIFACT_MISMATCH", "时间轴引用的成果文件摘要或作用域不一致")
		}
		if artifact.ID != source.ID {
			input, err := s.readCompositionArtifact(ctx, artifact)
			if err != nil {
				return nil, err
			}
			inputs[artifact.ID] = input
		}
	}
	if !sourceReferenced {
		return nil, fault.Invalid("FINAL_RENDER_TIMELINE_SOURCE_REQUIRED", "时间轴必须引用合成清单中的选中视频")
	}
	if manifest.Narration.Status == "enabled" {
		artifact, err := s.artifacts.Artifact(ctx, tenantID, manifest.Narration.ArtifactID)
		if err != nil {
			return nil, err
		}
		if artifact.TenantID != tenantID || artifact.ProjectID != projectID || artifact.ApprovedSnapshotID != snapshotID || normalizedSHA256(artifact.SHA256) != normalizedSHA256(manifest.Narration.Digest) || !strings.HasPrefix(artifact.MediaType, "audio/") {
			return nil, fault.Conflict("FINAL_RENDER_NARRATION_ARTIFACT_MISMATCH", "旁白成果文件摘要、媒体类型或作用域不一致")
		}
		input, err := s.readCompositionArtifact(ctx, artifact)
		if err != nil {
			return nil, err
		}
		inputs[artifact.ID] = input
	}
	if manifest.Subtitles.Status == "enabled" && manifest.Subtitles.SourceKind == "artifact" {
		artifact, err := s.artifacts.Artifact(ctx, tenantID, manifest.Subtitles.ArtifactID)
		if err != nil {
			return nil, err
		}
		if artifact.TenantID != tenantID || artifact.ProjectID != projectID || artifact.ApprovedSnapshotID != snapshotID || normalizedSHA256(artifact.SHA256) != normalizedSHA256(manifest.Subtitles.Digest) || !allowedSubtitleMediaType(artifact.MediaType) {
			return nil, fault.Conflict("FINAL_RENDER_SUBTITLE_ARTIFACT_MISMATCH", "字幕成果文件摘要、媒体类型或作用域不一致")
		}
		input, err := s.readCompositionArtifact(ctx, artifact)
		if err != nil {
			return nil, err
		}
		inputs[artifact.ID] = input
	}
	if manifest.Subtitles.Status == "enabled" && manifest.Subtitles.SourceKind == "inline" {
		return nil, fault.Policy("FINAL_RENDER_INLINE_SUBTITLES_UNSUPPORTED", "内嵌字幕缺少受控正文输入，不能静默合成", "将字幕正文登记为已批准快照范围内的字幕 Artifact 后重试")
	}
	result := make([]composition.InputArtifact, 0, len(inputs))
	for _, input := range inputs {
		result = append(result, input)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (s *DeliveryService) readCompositionArtifact(ctx context.Context, artifact deliverydomain.Artifact) (composition.InputArtifact, error) {
	if strings.TrimSpace(artifact.ObjectKey) == "" {
		return composition.InputArtifact{}, fault.Conflict("FINAL_RENDER_INPUT_OBJECT_MISSING", "合成输入成果缺少对象存储引用")
	}
	body, err := s.blobs.Get(ctx, artifact.ObjectKey)
	if err != nil {
		return composition.InputArtifact{}, err
	}
	if (artifact.ByteSize > 0 && int64(len(body)) != artifact.ByteSize) || normalizedSHA256(mediapipeline.SHA256(body)) != normalizedSHA256(artifact.SHA256) {
		return composition.InputArtifact{}, fault.Conflict("FINAL_RENDER_INPUT_BLOB_MISMATCH", "合成输入对象与成果摘要或字节数不一致")
	}
	return composition.InputArtifact{ID: artifact.ID, Digest: normalizedSHA256(artifact.SHA256), MediaType: artifact.MediaType, FileName: artifact.FileName, Body: body}, nil
}

func allowedSubtitleMediaType(mediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return strings.HasPrefix(mediaType, "text/") || mediaType == "application/json" || mediaType == "application/ttml+xml"
}

func newDeterministicCompositionManifest(snapshot reviewdomain.ApprovedSnapshot, selected deliverydomain.MediaReview, source deliverydomain.Artifact) (deliverydomain.CompositionManifest, error) {
	reviewDigest, err := mediaReviewDigest(selected)
	if err != nil {
		return deliverydomain.CompositionManifest{}, err
	}
	snapshotDigest := snapshot.SubjectHash
	if strings.TrimSpace(snapshotDigest) == "" {
		snapshotDigest = snapshot.ContentHash
	}
	return deliverydomain.CompositionManifest{
		Schema:           deliverydomain.CompositionSchemaRef{Name: deliverydomain.CompositionManifestSchema, Version: "1.0"},
		ApprovedSnapshot: deliverydomain.CompositionSnapshotRef{ID: snapshot.ID, Digest: snapshotDigest},
		SelectedVideo:    deliverydomain.CompositionVideoRef{ArtifactID: source.ID, Digest: normalizedSHA256(source.SHA256), MediaReviewID: selected.ID, ReviewDigest: reviewDigest},
		Narration:        deliverydomain.CompositionNarration{Status: "disabled"},
		Subtitles:        deliverydomain.CompositionSubtitles{Status: "disabled", SourceKind: "none"},
		BrandCTA:         deliverydomain.CompositionBrandCTA{BrandConfigDigest: "sha256:" + strings.Repeat("0", 64), CTADigest: "sha256:" + strings.Repeat("1", 64)},
		Timeline:         deliverydomain.CompositionTimeline{CanvasWidth: 1080, CanvasHeight: 1920, FrameRateNumerator: 30, FrameRateDenominator: 1, DurationMS: 1000, TransitionVersion: "deterministic/1", Segments: []deliverydomain.CompositionSegment{{ID: "selected-video", StartMS: 0, EndMS: 1000, ArtifactID: source.ID, ArtifactDigest: normalizedSHA256(source.SHA256)}}},
		Renderer:         deliverydomain.CompositionRenderer{Name: composition.RendererName, Version: composition.RendererVersion, CapabilityDigest: "sha256:" + strings.Repeat("2", 64)},
		Output:           deliverydomain.CompositionOutput{Kind: "final_render", MediaType: "video/mp4", Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"},
	}, nil
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
