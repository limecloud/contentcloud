package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/mediapipeline"
)

type CreateMediaGenerationJobInput struct {
	StageRunID              string   `json:"stage_run_id"`
	StoryboardSnapshotID    string   `json:"storyboard_snapshot_id"`
	PromptPackageArtifactID string   `json:"prompt_package_artifact_id,omitempty"`
	ProviderID              string   `json:"provider_id"`
	ProfileVersion          string   `json:"profile_version"`
	Mode                    string   `json:"mode"`
	AspectRatio             string   `json:"aspect_ratio"`
	DurationSeconds         int      `json:"duration_seconds"`
	InputArtifactRefs       []string `json:"input_artifact_refs"`
	IdempotencyKey          string   `json:"idempotency_key,omitempty"`
}

type MediaJobDecisionInput struct {
	ExpectedVersion int `json:"expected_version"`
}

type MediaReviewDecisionInput struct {
	ExpectedVersion int            `json:"expected_version"`
	Decision        string         `json:"decision"`
	Reason          string         `json:"reason"`
	Selected        bool           `json:"selected"`
	Checks          map[string]any `json:"checks"`
}

type BuildTaskDeliveryPackageInput struct {
	FinalReviewID string `json:"final_review_id"`
}

func (s *Service) CreateMediaGenerationJob(ctx context.Context, actor Actor, taskID string, input CreateMediaGenerationJobInput, requestID string) (domain.MediaGenerationJob, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.MediaGenerationJob{}, err
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return domain.MediaGenerationJob{}, err
	}
	if task.ContentType != domain.ContentTypeMarketingVideo {
		return domain.MediaGenerationJob{}, domain.Policy("MEDIA_JOB_CONTENT_TYPE_REQUIRED", "只有营销视频任务可以创建视频生成任务", "使用营销视频全流程规范创建任务")
	}
	runs, err := s.store.StageRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return domain.MediaGenerationJob{}, err
	}
	run, err := currentStageRun(task, runs)
	if err != nil {
		return domain.MediaGenerationJob{}, err
	}
	if input.StageRunID != "" && input.StageRunID != run.ID {
		return domain.MediaGenerationJob{}, domain.Conflict("MEDIA_JOB_STAGE_NOT_CURRENT", "视频生成任务必须绑定当前流程阶段执行记录")
	}
	if run.Status != domain.StageRunStatusRunning || run.StageID != "generation" {
		return domain.MediaGenerationJob{}, domain.Policy("MEDIA_JOB_STAGE_INVALID", "只有运行中的视频生成阶段可以创建生成任务", "先完成并批准分镜阶段")
	}
	storyboard, err := s.store.ApprovedSnapshot(ctx, actor.TenantID, strings.TrimSpace(input.StoryboardSnapshotID))
	if err != nil {
		return domain.MediaGenerationJob{}, err
	}
	if storyboard.ProjectID != task.ProjectID {
		return domain.MediaGenerationJob{}, domain.Policy("MEDIA_JOB_STORYBOARD_SCOPE_INVALID", "分镜快照不属于当前项目", "选择当前任务项目内已批准的分镜")
	}
	verifiedInputRefs, err := s.verifiedStoryboardInputArtifacts(ctx, actor.TenantID, storyboard)
	if err != nil {
		return domain.MediaGenerationJob{}, err
	}
	if len(input.InputArtifactRefs) > 0 && !sameStringSet(input.InputArtifactRefs, verifiedInputRefs) {
		return domain.MediaGenerationJob{}, domain.Conflict("MEDIA_JOB_INPUT_ARTIFACTS_MISMATCH", "视频生成任务的输入与服务端核验的锁定分镜素材不一致")
	}
	input.InputArtifactRefs = verifiedInputRefs
	providerID := defaultString(strings.TrimSpace(input.ProviderID), "fake")
	profileVersion := defaultString(strings.TrimSpace(input.ProfileVersion), "1.0.0")
	profile, err := s.ensureProviderProfile(ctx, providerID, profileVersion)
	if err != nil {
		return domain.MediaGenerationJob{}, err
	}
	now := s.now().UTC()
	if profile.Status != "published" || !profile.ExpiresAt.After(now) {
		return domain.MediaGenerationJob{}, domain.Policy("PROVIDER_PROFILE_UNAVAILABLE", "服务商配置版本未发布或已过期", "选择有效的服务商配置版本")
	}
	mode := defaultString(strings.TrimSpace(input.Mode), "image_to_video")
	if !containsString(profile.Modes, mode) {
		return domain.MediaGenerationJob{}, domain.Invalid("PROVIDER_MODE_UNSUPPORTED", "服务商配置版本不支持当前生成模式")
	}
	duration := input.DurationSeconds
	if duration < 1 {
		duration = 15
	}
	maxAttempts := 3
	maxJobCost := int64(0)
	if providerID != "fake" {
		binding, bindingErr := s.store.ProviderBinding(ctx, actor.TenantID, providerID)
		if bindingErr != nil {
			return domain.MediaGenerationJob{}, domain.Policy("PROVIDER_NOT_CONFIGURED", "当前租户尚未配置该服务商", "由租户管理员配置服务商绑定")
		}
		if binding.State != "active" || binding.ProfileVersion != profile.Version {
			return domain.MediaGenerationJob{}, domain.Policy("PROVIDER_BINDING_UNAVAILABLE", "服务商绑定未启用或配置版本不一致", "检查服务商配置")
		}
		maxAttempts = binding.MaxRetries + 1
		maxJobCost = binding.MaxJobCostMinor
	}
	estimatedCost := profileCostMinor(profile)
	if maxJobCost > 0 && estimatedCost > maxJobCost {
		return domain.MediaGenerationJob{}, domain.Policy("MEDIA_JOB_COST_LIMIT_EXCEEDED", "视频生成任务的估算费用超过单次任务上限", "降低生成规格或调整预算")
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("media:%s:%s:%s:%s", task.ID, storyboard.ID, providerID, mode)
	}
	state := domain.MediaJobQueued
	if estimatedCost > 0 {
		state = domain.MediaJobAwaitingCostApproval
	}
	job := domain.MediaGenerationJob{
		ID:                      domain.NewID(),
		TenantID:                task.TenantID,
		ProjectID:               task.ProjectID,
		TaskID:                  task.ID,
		StageRunID:              run.ID,
		StoryboardSnapshotID:    storyboard.ID,
		PromptPackageArtifactID: strings.TrimSpace(input.PromptPackageArtifactID),
		ProviderID:              providerID,
		ProfileVersion:          profile.Version,
		ProfileDigest:           profile.Digest,
		Model:                   profile.Model,
		Mode:                    mode,
		AspectRatio:             defaultString(strings.TrimSpace(input.AspectRatio), "9:16"),
		DurationSeconds:         duration,
		InputArtifactRefs:       append([]string{}, input.InputArtifactRefs...),
		State:                   state,
		IdempotencyKey:          idempotencyKey,
		EstimatedCostMinor:      estimatedCost,
		Currency:                profileCurrency(profile),
		MaxAttempts:             maxAttempts,
		RowVersion:              1,
		CreatedBy:               actor.UserID,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	job.NormalizeCollections()
	if err := s.store.CreateMediaGenerationJob(ctx, job); err != nil {
		return domain.MediaGenerationJob{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "media.job_created", "media_generation_job", job.ID, requestID, map[string]any{"provider_id": providerID, "profile_version": profile.Version, "estimated_cost_minor": estimatedCost, "currency": job.Currency})
	return job, nil
}

func (s *Service) ApproveMediaGenerationJob(ctx context.Context, actor Actor, id string, input MediaJobDecisionInput, requestID string) (domain.MediaGenerationJob, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil {
		return domain.MediaGenerationJob{}, err
	}
	job, err := s.store.MediaGenerationJob(ctx, actor.TenantID, id)
	if err != nil {
		return domain.MediaGenerationJob{}, err
	}
	if input.ExpectedVersion != job.RowVersion {
		return domain.MediaGenerationJob{}, domain.Conflict("MEDIA_JOB_STALE", "视频生成任务已被其他操作更新")
	}
	if job.State != domain.MediaJobAwaitingCostApproval && job.State != domain.MediaJobBudgetBlocked {
		return domain.MediaGenerationJob{}, domain.Conflict("MEDIA_JOB_COST_DECISION_INVALID", "当前视频生成任务不在费用确认状态")
	}
	job.State = domain.MediaJobQueued
	job.ErrorCode = ""
	job.ErrorDetailSafe = ""
	job.UpdatedAt = s.now().UTC()
	if err := s.store.SaveMediaGenerationJob(ctx, job, input.ExpectedVersion); err != nil {
		return domain.MediaGenerationJob{}, err
	}
	job.RowVersion = input.ExpectedVersion + 1
	s.audit(ctx, actor, job.ProjectID, "media.cost_approved", "media_generation_job", job.ID, requestID, map[string]any{"estimated_cost_minor": job.EstimatedCostMinor, "currency": job.Currency})
	return job, nil
}

func (s *Service) CancelMediaGenerationJob(ctx context.Context, actor Actor, id string, input MediaJobDecisionInput, requestID string) (domain.MediaGenerationJob, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.MediaGenerationJob{}, err
	}
	job, err := s.store.MediaGenerationJob(ctx, actor.TenantID, id)
	if err != nil {
		return domain.MediaGenerationJob{}, err
	}
	if input.ExpectedVersion != job.RowVersion {
		return domain.MediaGenerationJob{}, domain.Conflict("MEDIA_JOB_STALE", "视频生成任务已被其他操作更新")
	}
	if !domain.CanTransitionMediaJob(job.State, domain.MediaJobCancelled) {
		return domain.MediaGenerationJob{}, domain.Conflict("MEDIA_JOB_NOT_CANCELLABLE", "当前视频生成任务不能取消")
	}
	now := s.now().UTC()
	job.State = domain.MediaJobCancelled
	job.CancelRequestedAt = &now
	job.UpdatedAt = now
	if err := s.store.SaveMediaGenerationJob(ctx, job, input.ExpectedVersion); err != nil {
		return domain.MediaGenerationJob{}, err
	}
	job.RowVersion = input.ExpectedVersion + 1
	s.audit(ctx, actor, job.ProjectID, "media.job_cancelled", "media_generation_job", job.ID, requestID, nil)
	return job, nil
}

func (s *Service) PendingMediaGenerationJobs(ctx context.Context, limit int) ([]domain.MediaGenerationJob, error) {
	return s.store.PendingMediaGenerationJobs(ctx, limit)
}

func (s *Service) ProcessMediaGenerationJob(ctx context.Context, tenantID, id string) error {
	job, err := s.store.MediaGenerationJob(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if job.State == domain.MediaJobRetryWait {
		job, err = s.transitionMediaJob(ctx, job, domain.MediaJobQueued, nil)
		if err != nil {
			return err
		}
	}
	if job.State != domain.MediaJobQueued {
		return nil
	}
	job, err = s.transitionMediaJob(ctx, job, domain.MediaJobSubmitting, func(value *domain.MediaGenerationJob) {
		value.AttemptCount++
		value.LeaseOwner = "media-worker"
		expires := s.now().UTC().Add(2 * time.Minute)
		value.LeaseExpiresAt = &expires
	})
	if err != nil {
		return err
	}
	profile, err := s.ensureProviderProfile(ctx, job.ProviderID, job.ProfileVersion)
	if err != nil {
		return s.failMediaJob(ctx, job, "PROVIDER_PROFILE_UNAVAILABLE", "服务商配置版本不可用")
	}
	adapter, err := mediaAdapter(job.ProviderID)
	if err != nil {
		return s.failMediaJob(ctx, job, "PROVIDER_ADAPTER_UNAVAILABLE", "服务商适配器未配置")
	}
	request := mediapipeline.Request{JobID: job.ID, IdempotencyKey: job.IdempotencyKey, StoryboardSnapshotID: job.StoryboardSnapshotID, Mode: job.Mode, AspectRatio: job.AspectRatio, DurationSeconds: job.DurationSeconds, InputArtifactRefs: job.InputArtifactRefs}
	if err := adapter.Validate(request, profile); err != nil {
		return s.failMediaJob(ctx, job, "PROVIDER_REQUEST_INVALID", "服务商请求校验失败")
	}
	requestHash, _ := domain.CanonicalHash(request)
	now := s.now().UTC()
	attempt := domain.ProviderAttempt{ID: domain.NewID(), TenantID: job.TenantID, ProjectID: job.ProjectID, GenerationJobID: job.ID, AttemptNumber: job.AttemptCount, ProviderID: job.ProviderID, RequestDigest: "sha256:" + requestHash, ProviderState: "submitting", SafeRequestSummary: map[string]any{"mode": job.Mode, "aspect_ratio": job.AspectRatio, "duration_seconds": job.DurationSeconds, "input_count": len(job.InputArtifactRefs)}, SafeResponseSummary: map[string]any{}, DisclosureManifest: map[string]any{"provider_id": job.ProviderID, "profile_digest": job.ProfileDigest, "data_retention": profile.DataRetention}, EstimatedCostMinor: job.EstimatedCostMinor, Currency: job.Currency, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateProviderAttempt(ctx, attempt); err != nil {
		return s.failMediaJob(ctx, job, "PROVIDER_ATTEMPT_CREATE_FAILED", "无法记录服务商调用尝试")
	}
	submission, err := adapter.Submit(ctx, request, profile)
	if err != nil {
		attempt.ProviderState = "failed"
		attempt.ErrorCode = "PROVIDER_SUBMIT_FAILED"
		attempt.ErrorDetailSafe = "服务商提交失败"
		attempt.UpdatedAt = s.now().UTC()
		_ = s.store.SaveProviderAttempt(ctx, attempt)
		return s.failMediaJob(ctx, job, attempt.ErrorCode, attempt.ErrorDetailSafe)
	}
	now = s.now().UTC()
	attempt.ExternalJobID = submission.ExternalJobID
	attempt.ProviderRequestID = submission.ProviderRequestID
	attempt.ProviderState = "submitted"
	attempt.SubmittedAt = &now
	attempt.UpdatedAt = now
	if err := s.store.SaveProviderAttempt(ctx, attempt); err != nil {
		return err
	}
	job, err = s.transitionMediaJob(ctx, job, domain.MediaJobSubmitted, nil)
	if err != nil {
		return err
	}
	job, err = s.transitionMediaJob(ctx, job, domain.MediaJobGenerating, nil)
	if err != nil {
		return err
	}
	providerStatus, err := adapter.Status(ctx, submission.ExternalJobID, profile)
	if err != nil || providerStatus.State != "succeeded" {
		return s.failMediaJob(ctx, job, "PROVIDER_STATUS_FAILED", "服务商未返回成功状态")
	}
	job, err = s.transitionMediaJob(ctx, job, domain.MediaJobDownloading, nil)
	if err != nil {
		return err
	}
	download, err := adapter.Download(ctx, providerStatus.OutputRef, profile)
	if err != nil {
		return s.failMediaJob(ctx, job, "PROVIDER_DOWNLOAD_FAILED", "服务商输出下载失败")
	}
	job, err = s.transitionMediaJob(ctx, job, domain.MediaJobValidating, nil)
	if err != nil {
		return err
	}
	technical, err := mediapipeline.ValidateDownload(download, 10<<20)
	if err != nil {
		return s.failMediaJob(ctx, job, "MEDIA_OUTPUT_INVALID", "服务商输出未通过媒体校验")
	}
	sha := mediapipeline.SHA256(download.Body)
	objectKey := fmt.Sprintf("media/%s/%s/%s", job.TenantID, job.ID, download.FileName)
	if err := s.blobs.Put(ctx, objectKey, download.Body); err != nil {
		return s.failMediaJob(ctx, job, "MEDIA_OBJECT_WRITE_FAILED", "媒体文件写入对象存储失败")
	}
	now = s.now().UTC()
	artifact := domain.Artifact{ID: domain.NewID(), TenantID: job.TenantID, ProjectID: job.ProjectID, ApprovedSnapshotID: job.StoryboardSnapshotID, Kind: "generated_video", CapabilityID: "contentcloud.media.generate", CapabilityVersion: "1.0.0", CapabilityDigest: job.ProfileDigest, SchemaID: "contentcloud.generated-video/1.0", MediaType: download.MediaType, FileName: download.FileName, SHA256: sha, ByteSize: int64(len(download.Body)), ObjectKey: objectKey, Visibility: "client", RetentionClass: "audit", Purpose: "generated_take", Metadata: map[string]any{"task_id": job.TaskID, "generation_job_id": job.ID, "aspect_ratio": job.AspectRatio, "duration_seconds": job.DurationSeconds, "technical": technical, "quarantined": false}, CreatedAt: now}
	if err := s.store.CreateArtifact(ctx, artifact); err != nil {
		return s.failMediaJob(ctx, job, "MEDIA_ARTIFACT_CREATE_FAILED", "媒体成果文件保存失败")
	}
	technicalReview := domain.MediaReview{ID: domain.NewID(), TenantID: job.TenantID, ProjectID: job.ProjectID, TaskID: job.TaskID, GenerationJobID: job.ID, SubjectArtifactID: artifact.ID, SubjectDigest: normalizedSHA256(artifact.SHA256), ReviewKind: domain.MediaReviewTechnical, Status: domain.MediaReviewApproved, Checks: technical, RowVersion: 1, CreatedBy: "media-worker", DecidedBy: "media-worker", DecidedAt: &now, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateMediaReview(ctx, technicalReview); err != nil {
		return s.failMediaJob(ctx, job, "MEDIA_TECHNICAL_REVIEW_FAILED", "无法创建媒体技术审核")
	}
	contentReview := domain.MediaReview{ID: domain.NewID(), TenantID: job.TenantID, ProjectID: job.ProjectID, TaskID: job.TaskID, GenerationJobID: job.ID, SubjectArtifactID: artifact.ID, SubjectDigest: normalizedSHA256(artifact.SHA256), ReviewKind: domain.MediaReviewContent, Status: domain.MediaReviewPending, Checks: map[string]any{}, RowVersion: 1, CreatedBy: "media-worker", CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateMediaReview(ctx, contentReview); err != nil {
		return s.failMediaJob(ctx, job, "MEDIA_CONTENT_REVIEW_FAILED", "无法创建媒体内容审核")
	}
	now = s.now().UTC()
	attempt.ProviderState = "succeeded"
	attempt.SafeResponseSummary = map[string]any{"artifact_id": artifact.ID, "sha256": artifact.SHA256, "byte_size": artifact.ByteSize}
	attempt.ActualCostMinor = providerStatus.ActualMinor
	attempt.DownloadedAt = &now
	attempt.CompletedAt = &now
	attempt.UpdatedAt = now
	if err := s.store.SaveProviderAttempt(ctx, attempt); err != nil {
		return err
	}
	_, err = s.transitionMediaJob(ctx, job, domain.MediaJobSucceeded, func(value *domain.MediaGenerationJob) {
		value.ActualCostMinor = providerStatus.ActualMinor
		value.LeaseOwner = ""
		value.LeaseExpiresAt = nil
	})
	return err
}

func (s *Service) DecideMediaReview(ctx context.Context, actor Actor, id string, input MediaReviewDecisionInput, requestID string) (WorkTaskView, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer", "client_approver"); err != nil {
		return WorkTaskView{}, err
	}
	review, err := s.store.MediaReview(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	if review.ReviewKind == domain.MediaReviewTechnical {
		return WorkTaskView{}, domain.Policy("MEDIA_TECHNICAL_REVIEW_IMMUTABLE", "技术审核只能由媒体执行器写入", "请对内容审核或最终审核作出决定")
	}
	if review.ReviewKind == domain.MediaReviewFinal && actor.Role != "tenant_admin" && actor.Role != "client_approver" {
		return WorkTaskView{}, domain.Policy("MEDIA_FINAL_REVIEW_ROLE_REQUIRED", "最终成片只能由客户决定人或租户管理员批准", "请由最终批准角色处理")
	}
	if review.ReviewKind == domain.MediaReviewFinal {
		artifact, artifactErr := s.store.Artifact(ctx, actor.TenantID, review.SubjectArtifactID)
		if artifactErr != nil {
			return WorkTaskView{}, artifactErr
		}
		if artifact.Kind != "final_render" || artifact.Purpose != "final_video" || normalizedSHA256(artifact.SHA256) != review.SubjectDigest {
			return WorkTaskView{}, domain.Conflict("FINAL_RENDER_ARTIFACT_REQUIRED", "最终审核必须绑定独立的最终成片成果文件")
		}
	}
	if input.ExpectedVersion != review.RowVersion {
		return WorkTaskView{}, domain.Conflict("MEDIA_REVIEW_STALE", "媒体审核已被其他操作更新")
	}
	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	switch decision {
	case "approved":
		review.Status = domain.MediaReviewApproved
	case "changes_requested":
		review.Status = domain.MediaReviewChanges
	case "rejected":
		review.Status = domain.MediaReviewRejected
	default:
		return WorkTaskView{}, domain.Invalid("MEDIA_REVIEW_DECISION_INVALID", "媒体审核决定无效")
	}
	if decision == "approved" && !input.Selected {
		return WorkTaskView{}, domain.Invalid("MEDIA_REVIEW_SELECTION_REQUIRED", "批准候选成片时必须明确选中对应成果文件")
	}
	now := s.now().UTC()
	review.Selected = decision == "approved" && input.Selected
	review.DecisionReason = strings.TrimSpace(input.Reason)
	review.DecidedBy = actor.UserID
	review.DecidedAt = &now
	review.Checks = input.Checks
	review.UpdatedAt = now
	if err := s.store.SaveMediaReview(ctx, review, input.ExpectedVersion); err != nil {
		return WorkTaskView{}, err
	}
	review.RowVersion = input.ExpectedVersion + 1
	s.audit(ctx, actor, review.ProjectID, "media.review_decided", "media_review", review.ID, requestID, map[string]any{"decision": decision, "selected": review.Selected, "artifact_id": review.SubjectArtifactID})
	return s.WorkTask(ctx, actor, review.TaskID)
}

func (s *Service) BuildTaskDeliveryPackage(ctx context.Context, actor Actor, taskID string, input BuildTaskDeliveryPackageInput, requestID string) (domain.DeliveryPackage, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.DeliveryPackage{}, err
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return domain.DeliveryPackage{}, err
	}
	review, err := s.store.MediaReview(ctx, actor.TenantID, input.FinalReviewID)
	if err != nil {
		return domain.DeliveryPackage{}, err
	}
	if review.TaskID != task.ID || review.ProjectID != task.ProjectID || review.ReviewKind != domain.MediaReviewFinal || review.Status != domain.MediaReviewApproved || !review.Selected {
		return domain.DeliveryPackage{}, domain.Policy("FINAL_MEDIA_REVIEW_REQUIRED", "交付包必须绑定当前任务已批准并选中的最终成片审核", "先完成最终成片批准")
	}
	artifact, err := s.store.Artifact(ctx, actor.TenantID, review.SubjectArtifactID)
	if err != nil {
		return domain.DeliveryPackage{}, err
	}
	if normalizedSHA256(artifact.SHA256) != review.SubjectDigest || artifact.ProjectID != task.ProjectID || artifact.ByteSize <= 0 {
		return domain.DeliveryPackage{}, domain.Conflict("FINAL_ARTIFACT_INTEGRITY_FAILED", "最终成果文件与审核摘要不一致或文件无效")
	}
	if artifact.Kind != "final_render" || artifact.Purpose != "final_video" {
		return domain.DeliveryPackage{}, domain.Policy("FINAL_RENDER_ARTIFACT_REQUIRED", "交付包只能使用独立的最终成片", "先在后期阶段生成并批准最终成片")
	}
	if quarantined, _ := artifact.Metadata["quarantined"].(bool); quarantined {
		return domain.DeliveryPackage{}, domain.Policy("FINAL_ARTIFACT_QUARANTINED", "最终成果文件处于隔离状态，不能交付", "处理媒体安全问题后重新批准")
	}
	now := s.now().UTC()
	value := domain.DeliveryPackage{ID: domain.NewID(), TenantID: task.TenantID, ProjectID: task.ProjectID, ApprovedSnapshotIDs: []string{artifact.ApprovedSnapshotID}, ContentItemID: task.ID, Status: "ready", Manifest: []domain.Artifact{artifact}, CreatedBy: actor.UserID, CreatedAt: now}
	if err := s.store.CreateDeliveryPackage(ctx, value, []domain.Artifact{artifact}); err != nil {
		return domain.DeliveryPackage{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "delivery.package_built", "delivery_package", value.ID, requestID, map[string]any{"artifact_id": artifact.ID, "artifact_digest": normalizedSHA256(artifact.SHA256), "final_review_id": review.ID})
	return value, nil
}

func (s *Service) transitionMediaJob(ctx context.Context, job domain.MediaGenerationJob, state string, mutate func(*domain.MediaGenerationJob)) (domain.MediaGenerationJob, error) {
	expected := job.RowVersion
	job.State = state
	job.UpdatedAt = s.now().UTC()
	if mutate != nil {
		mutate(&job)
	}
	if err := s.store.SaveMediaGenerationJob(ctx, job, expected); err != nil {
		return domain.MediaGenerationJob{}, err
	}
	job.RowVersion = expected + 1
	return job, nil
}

func (s *Service) failMediaJob(ctx context.Context, job domain.MediaGenerationJob, code, detail string) error {
	_, err := s.transitionMediaJob(ctx, job, domain.MediaJobFailed, func(value *domain.MediaGenerationJob) {
		value.ErrorCode = code
		value.ErrorDetailSafe = detail
		value.LeaseOwner = ""
		value.LeaseExpiresAt = nil
	})
	if err != nil {
		return err
	}
	return domain.Policy(code, detail, "检查服务商配置或重试生成")
}

func (s *Service) ensureProviderProfile(ctx context.Context, providerID, version string) (domain.ProviderProfile, error) {
	profile, err := s.store.ProviderProfile(ctx, providerID, version)
	if err == nil || providerID != "fake" || !domain.IsNotFound(err) {
		return profile, err
	}
	profile = fakeProviderProfile()
	if err := s.store.CreateProviderProfile(ctx, profile); err != nil {
		if existing, lookupErr := s.store.ProviderProfile(ctx, providerID, version); lookupErr == nil {
			return existing, nil
		}
		return domain.ProviderProfile{}, err
	}
	return profile, nil
}

func fakeProviderProfile() domain.ProviderProfile {
	return domain.ProviderProfile{ProviderID: "fake", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("0", 64), AdapterVersion: "fake/1.0.0", Model: "fixture-video", Region: "local", Modes: []string{"text_to_video", "image_to_video"}, InputMediaTypes: []string{"image/jpeg", "image/png", "application/json"}, OutputMediaType: "video/mp4", Limits: map[string]any{"max_duration_seconds": 60, "max_bytes": 10 << 20}, DataRetention: "ephemeral", Pricing: map[string]any{"currency": "CNY", "per_job_minor": 0}, Status: "published", VerifiedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func mediaAdapter(providerID string) (mediapipeline.Adapter, error) {
	if providerID == "fake" {
		return mediapipeline.FakeProvider{}, nil
	}
	return nil, domain.NotFound("服务商适配器")
}

func profileCostMinor(profile domain.ProviderProfile) int64 {
	switch value := profile.Pricing["per_job_minor"].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func profileCurrency(profile domain.ProviderProfile) string {
	value, _ := profile.Pricing["currency"].(string)
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 3 {
		return "CNY"
	}
	return value
}
