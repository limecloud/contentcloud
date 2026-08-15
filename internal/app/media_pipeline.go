package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/blob"
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
	RuntimeJobRunID         string   `json:"runtime_job_run_id,omitempty"`
	RuntimeNodeRunID        string   `json:"runtime_node_run_id,omitempty"`
	RuntimeAttemptID        string   `json:"runtime_attempt_id,omitempty"`
	RuntimeEffectID         string   `json:"runtime_effect_id,omitempty"`
}

type MediaJobDecisionInput struct {
	ExpectedVersion int `json:"expected_version"`
}

type MediaJobSubmitReconciliationInput struct {
	ExpectedVersion int    `json:"expected_version"`
	ExternalJobID   string `json:"external_job_id"`
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
	mode := defaultString(strings.TrimSpace(input.Mode), "image_to_video")
	verifiedInputRefs := []string{}
	if mode != "text_to_video" {
		verifiedInputRefs, err = s.verifiedStoryboardInputArtifacts(ctx, actor.TenantID, storyboard)
		if err != nil {
			return domain.MediaGenerationJob{}, err
		}
		if len(input.InputArtifactRefs) > 0 && !sameStringSet(input.InputArtifactRefs, verifiedInputRefs) {
			return domain.MediaGenerationJob{}, domain.Conflict("MEDIA_JOB_INPUT_ARTIFACTS_MISMATCH", "视频生成任务的输入与服务端核验的锁定分镜素材不一致")
		}
	}
	if mode == "text_to_video" {
		input.InputArtifactRefs = []string{}
	} else {
		input.InputArtifactRefs = verifiedInputRefs
	}
	providerID := defaultString(strings.TrimSpace(input.ProviderID), "fake")
	profileVersion := defaultString(strings.TrimSpace(input.ProfileVersion), "1.0.0")
	profile, err := s.ensureProviderProfile(ctx, providerID, profileVersion)
	if err != nil {
		return domain.MediaGenerationJob{}, err
	}
	if providerID == Seedance25ProviderID {
		promptPackageID := strings.TrimSpace(input.PromptPackageArtifactID)
		if promptPackageID == "" {
			return domain.MediaGenerationJob{}, domain.Invalid("SEEDANCE_PROMPT_PACKAGE_REQUIRED", "Seedance 2.5 任务必须绑定已校验的 PromptPackage Artifact")
		}
		promptArtifact, artifactErr := s.store.Artifact(ctx, actor.TenantID, promptPackageID)
		if artifactErr != nil {
			return domain.MediaGenerationJob{}, artifactErr
		}
		if promptArtifact.ProjectID != task.ProjectID || promptArtifact.ApprovedSnapshotID != storyboard.ID || promptArtifact.Kind != "prompt_package" || promptArtifact.MediaType != "application/json" {
			return domain.MediaGenerationJob{}, domain.Conflict("SEEDANCE_PROMPT_PACKAGE_SCOPE_INVALID", "Seedance PromptPackage Artifact 与当前项目或批准快照不一致")
		}
		if version := metadataString(promptArtifact.Metadata, "provider_profile_version"); version != "" && version != profile.Version {
			return domain.MediaGenerationJob{}, domain.Conflict("SEEDANCE_PROMPT_PACKAGE_STALE", "Seedance PromptPackage Artifact 与当前 Provider Profile 版本不一致")
		}
	}
	now := s.now().UTC()
	if profile.Status != "published" || !profile.ExpiresAt.After(now) {
		return domain.MediaGenerationJob{}, domain.Policy("PROVIDER_PROFILE_UNAVAILABLE", "服务商配置版本未发布或已过期", "选择有效的服务商配置版本")
	}
	if !containsString(profile.Modes, mode) {
		return domain.MediaGenerationJob{}, domain.Invalid("PROVIDER_MODE_UNSUPPORTED", "服务商配置版本不支持当前生成模式")
	}
	adapter, adapterErr := s.mediaAdapter(providerID)
	if adapterErr != nil {
		return domain.MediaGenerationJob{}, domain.Policy("PROVIDER_ADAPTER_UNAVAILABLE", "服务商适配器未配置", "配置已发布 Provider Adapter 后重试")
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
		if binding.State != "active" || binding.ProfileVersion != profile.Version || !validProviderCredentialRef(binding.CredentialRef) {
			return domain.MediaGenerationJob{}, domain.Policy("PROVIDER_BINDING_UNAVAILABLE", "服务商绑定未启用、配置版本不一致或缺少受控凭据引用", "检查服务商配置")
		}
		maxAttempts = binding.MaxRetries + 1
		maxJobCost = binding.MaxJobCostMinor
	}
	estimateRequest := mediapipeline.Request{TenantID: task.TenantID, ProjectID: task.ProjectID, JobID: "estimate:" + task.ID, IdempotencyKey: "estimate:" + task.ID + ":" + providerID, StoryboardSnapshotID: storyboard.ID, PromptPackageArtifactID: strings.TrimSpace(input.PromptPackageArtifactID), ProfileVersion: profile.Version, Mode: mode, AspectRatio: defaultString(strings.TrimSpace(input.AspectRatio), "9:16"), DurationSeconds: duration, InputArtifactRefs: input.InputArtifactRefs}
	if err := adapter.Validate(estimateRequest, profile); err != nil {
		return domain.MediaGenerationJob{}, err
	}
	estimate, err := adapter.Estimate(estimateRequest, profile)
	if err != nil {
		return domain.MediaGenerationJob{}, err
	}
	estimatedCost := estimate.CostMinor
	if estimatedCost < 0 {
		return domain.MediaGenerationJob{}, domain.Invalid("PROVIDER_ESTIMATE_INVALID", "服务商返回了负数费用估算")
	}
	currency := strings.ToUpper(strings.TrimSpace(estimate.Currency))
	if len(currency) != 3 {
		currency = profileCurrency(profile)
	}
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
		Currency:                currency,
		MaxAttempts:             maxAttempts,
		RowVersion:              1,
		CreatedBy:               actor.UserID,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	job.RuntimeJobRunID = strings.TrimSpace(input.RuntimeJobRunID)
	job.RuntimeNodeRunID = strings.TrimSpace(input.RuntimeNodeRunID)
	job.RuntimeAttemptID = strings.TrimSpace(input.RuntimeAttemptID)
	job.RuntimeEffectID = strings.TrimSpace(input.RuntimeEffectID)
	if job.RuntimeJobRunID != "" || job.RuntimeNodeRunID != "" || job.RuntimeAttemptID != "" || job.RuntimeEffectID != "" {
		if s.runtimeService == nil {
			return domain.MediaGenerationJob{}, domain.Policy("MEDIA_JOB_RUNTIME_UNAVAILABLE", "媒体 Job 指定了 Runtime 关联但当前未启用 Runtime", "移除 Runtime 关联或启用 Runtime")
		}
		runtimeJob, runtimeErr := s.runtimeService.Job(ctx, actor.TenantID, job.RuntimeJobRunID)
		if runtimeErr != nil {
			return domain.MediaGenerationJob{}, runtimeErr
		}
		if runtimeJob.ProjectID != job.ProjectID || runtimeJob.WorkTaskID != job.TaskID {
			return domain.MediaGenerationJob{}, domain.Policy("MEDIA_JOB_RUNTIME_SCOPE_INVALID", "媒体 Job 的 Runtime 作用域与业务任务不一致", "绑定当前项目和任务的 Runtime 执行实例")
		}
		nodes, nodesErr := s.runtimeService.Nodes(ctx, actor.TenantID, runtimeJob.ID)
		if nodesErr != nil {
			return domain.MediaGenerationJob{}, nodesErr
		}
		foundNode := false
		for _, node := range nodes {
			if node.ID == job.RuntimeNodeRunID {
				foundNode = true
				break
			}
		}
		if !foundNode {
			return domain.MediaGenerationJob{}, domain.NotFound("媒体 Job Runtime 节点")
		}
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
	var attempt domain.ProviderAttempt
	var hasExternalJob bool
	if attempts, attemptsErr := s.store.ProviderAttempts(ctx, actor.TenantID, job.ID); attemptsErr != nil {
		return domain.MediaGenerationJob{}, attemptsErr
	} else if len(attempts) > 0 {
		attempt = attempts[len(attempts)-1]
		hasExternalJob = strings.TrimSpace(attempt.ExternalJobID) != ""
	}
	if hasExternalJob {
		profile, profileErr := s.ensureProviderProfile(ctx, job.ProviderID, job.ProfileVersion)
		if profileErr != nil {
			return domain.MediaGenerationJob{}, domain.Policy("PROVIDER_CANCEL_UNKNOWN", "取消前无法读取原服务商配置，外部任务状态不明", "恢复服务商配置后进行对账")
		}
		adapter, adapterErr := s.mediaAdapter(job.ProviderID)
		if adapterErr != nil {
			return domain.MediaGenerationJob{}, domain.Policy("PROVIDER_CANCEL_UNKNOWN", "取消前无法读取服务商适配器，外部任务状态不明", "恢复服务商适配器后进行对账")
		}
		if cancelErr := adapter.Cancel(ctx, attempt.ExternalJobID, profile); cancelErr != nil {
			now := s.now().UTC()
			nextPoll := now.Add(10 * time.Second)
			job.CancelRequestedAt = &now
			job.ErrorCode = "PROVIDER_CANCEL_UNKNOWN"
			job.ErrorDetailSafe = "服务商取消结果不明，等待对账"
			job.UpdatedAt = now
			job.State = domain.MediaJobAwaitingExternal
			job.LeaseOwner = ""
			job.LeaseExpiresAt = nil
			if saveErr := s.store.SaveMediaGenerationJob(ctx, job, input.ExpectedVersion); saveErr != nil {
				return domain.MediaGenerationJob{}, saveErr
			}
			job.RowVersion = input.ExpectedVersion + 1
			attempt.ErrorCode = "PROVIDER_CANCEL_UNKNOWN"
			attempt.ErrorDetailSafe = "服务商取消结果不明，等待对账"
			attempt.ProviderState = "cancel_requested"
			attempt.NextPollAt = &nextPoll
			attempt.UpdatedAt = now
			if saveErr := s.store.SaveProviderAttempt(ctx, attempt); saveErr != nil {
				return domain.MediaGenerationJob{}, saveErr
			}
			if job.RuntimeEffectID != "" {
				if effectErr := s.transitionMediaEffect(ctx, job, domain.EffectUnknown, attempt.ExternalJobID, "", "PROVIDER_CANCEL_UNKNOWN"); effectErr != nil {
					return domain.MediaGenerationJob{}, effectErr
				}
			}
			s.audit(ctx, actor, job.ProjectID, "media.job_cancel_unknown", "media_generation_job", job.ID, requestID, map[string]any{"external_job_id": attempt.ExternalJobID})
			return job, domain.Policy("PROVIDER_CANCEL_UNKNOWN", "服务商取消结果不明，已进入外部任务对账状态", "查询服务商状态并完成外部操作对账")
		}
		attempt.ProviderState = "cancelled"
		attempt.UpdatedAt = s.now().UTC()
		if saveErr := s.store.SaveProviderAttempt(ctx, attempt); saveErr != nil {
			return domain.MediaGenerationJob{}, saveErr
		}
		if job.RuntimeEffectID != "" {
			if effectErr := s.transitionMediaEffect(ctx, job, domain.EffectFailed, attempt.ExternalJobID, "", "PROVIDER_JOB_CANCELLED"); effectErr != nil {
				return domain.MediaGenerationJob{}, effectErr
			}
		}
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

// ReconcileMediaGenerationSubmit binds an operator-confirmed external task ID
// to an unknown submission. It is the only path that can recover a timed-out
// submit without issuing a second request with the same billable intent.
func (s *Service) ReconcileMediaGenerationSubmit(ctx context.Context, actor Actor, id string, input MediaJobSubmitReconciliationInput, requestID string) (domain.MediaGenerationJob, error) {
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
	if job.State != domain.MediaJobAwaitingExternal {
		return domain.MediaGenerationJob{}, domain.Conflict("MEDIA_JOB_RECONCILIATION_INVALID", "只有等待外部对账的媒体任务可以补录外部任务标识")
	}
	externalID := strings.TrimSpace(input.ExternalJobID)
	if externalID == "" || strings.ContainsAny(externalID, "/?#") || len(externalID) > 256 {
		return domain.MediaGenerationJob{}, domain.Invalid("PROVIDER_EXTERNAL_ID_INVALID", "服务商外部任务标识无效")
	}
	attempts, err := s.store.ProviderAttempts(ctx, actor.TenantID, job.ID)
	if err != nil || len(attempts) == 0 {
		if err != nil {
			return domain.MediaGenerationJob{}, err
		}
		return domain.MediaGenerationJob{}, domain.NotFound("服务商调用尝试")
	}
	attempt := attempts[len(attempts)-1]
	if strings.TrimSpace(attempt.ExternalJobID) != "" {
		if attempt.ExternalJobID != externalID {
			return domain.MediaGenerationJob{}, domain.Conflict("PROVIDER_EXTERNAL_ID_CONFLICT", "服务商外部任务标识已经绑定且不能覆盖")
		}
		return job, nil
	}
	now := s.now().UTC()
	attempt.ExternalJobID = externalID
	attempt.ProviderState = "reconciliation_pending"
	attempt.ErrorCode = ""
	attempt.ErrorDetailSafe = "已补录外部任务标识，等待服务商状态对账"
	attempt.NextPollAt = &now
	attempt.UpdatedAt = now
	if err := s.store.SaveProviderAttempt(ctx, attempt); err != nil {
		return domain.MediaGenerationJob{}, err
	}
	job.ErrorCode = ""
	job.ErrorDetailSafe = ""
	job.UpdatedAt = now
	if err := s.store.SaveMediaGenerationJob(ctx, job, input.ExpectedVersion); err != nil {
		return domain.MediaGenerationJob{}, err
	}
	job.RowVersion = input.ExpectedVersion + 1
	if job.RuntimeEffectID != "" {
		if err := s.transitionMediaEffect(ctx, job, domain.EffectReconciling, externalID, "", ""); err != nil {
			return domain.MediaGenerationJob{}, err
		}
	}
	s.audit(ctx, actor, job.ProjectID, "media.job_submit_reconciled", "media_generation_job", job.ID, requestID, map[string]any{"external_job_id": externalID})
	return job, nil
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
	resumingExternal := job.State == domain.MediaJobAwaitingExternal
	if job.State != domain.MediaJobQueued && !resumingExternal {
		return nil
	}
	profile, err := s.ensureProviderProfile(ctx, job.ProviderID, job.ProfileVersion)
	if err != nil {
		return s.failMediaJob(ctx, job, "PROVIDER_PROFILE_UNAVAILABLE", "服务商配置版本不可用")
	}
	adapter, err := s.mediaAdapter(job.ProviderID)
	if err != nil {
		return s.failMediaJob(ctx, job, "PROVIDER_ADAPTER_UNAVAILABLE", "服务商适配器未配置")
	}
	request := mediapipeline.Request{TenantID: job.TenantID, ProjectID: job.ProjectID, JobID: job.ID, IdempotencyKey: job.IdempotencyKey, StoryboardSnapshotID: job.StoryboardSnapshotID, PromptPackageArtifactID: job.PromptPackageArtifactID, ProfileVersion: job.ProfileVersion, Mode: job.Mode, AspectRatio: job.AspectRatio, DurationSeconds: job.DurationSeconds, InputArtifactRefs: job.InputArtifactRefs}
	if err := adapter.Validate(request, profile); err != nil {
		return s.failMediaJob(ctx, job, "PROVIDER_REQUEST_INVALID", "服务商请求校验失败")
	}
	var attempt domain.ProviderAttempt
	var externalJobID string
	now := s.now().UTC()
	if resumingExternal {
		attempts, attemptsErr := s.store.ProviderAttempts(ctx, tenantID, job.ID)
		if attemptsErr != nil || len(attempts) == 0 {
			return s.failMediaJob(ctx, job, "PROVIDER_ATTEMPT_MISSING", "异步服务商任务缺少本地调用尝试记录")
		}
		attempt = attempts[len(attempts)-1]
		externalJobID = strings.TrimSpace(attempt.ExternalJobID)
		if externalJobID == "" {
			return s.failMediaJob(ctx, job, "PROVIDER_EXTERNAL_ID_MISSING", "异步服务商任务缺少外部任务标识")
		}
		job, err = s.transitionMediaJob(ctx, job, domain.MediaJobGenerating, func(value *domain.MediaGenerationJob) {
			value.LeaseOwner = "media-worker"
			expires := s.now().UTC().Add(2 * time.Minute)
			value.LeaseExpiresAt = &expires
		})
		if err != nil {
			return err
		}
	} else {
		job, err = s.transitionMediaJob(ctx, job, domain.MediaJobSubmitting, func(value *domain.MediaGenerationJob) {
			value.AttemptCount++
			value.LeaseOwner = "media-worker"
			expires := s.now().UTC().Add(2 * time.Minute)
			value.LeaseExpiresAt = &expires
		})
		if err != nil {
			return err
		}
		requestHash, _ := domain.CanonicalHash(request)
		now = s.now().UTC()
		attempt = domain.ProviderAttempt{ID: domain.NewID(), TenantID: job.TenantID, ProjectID: job.ProjectID, GenerationJobID: job.ID, AttemptNumber: job.AttemptCount, ProviderID: job.ProviderID, RequestDigest: "sha256:" + requestHash, RuntimeJobRunID: job.RuntimeJobRunID, RuntimeNodeRunID: job.RuntimeNodeRunID, RuntimeAttemptID: job.RuntimeAttemptID, RuntimeEffectID: job.RuntimeEffectID, ProviderState: "submitting", SafeRequestSummary: map[string]any{"mode": job.Mode, "aspect_ratio": job.AspectRatio, "duration_seconds": job.DurationSeconds, "input_count": len(job.InputArtifactRefs)}, SafeResponseSummary: map[string]any{}, DisclosureManifest: map[string]any{"provider_id": job.ProviderID, "profile_digest": job.ProfileDigest, "data_retention": profile.DataRetention}, EstimatedCostMinor: job.EstimatedCostMinor, Currency: job.Currency, CreatedAt: now, UpdatedAt: now}
		if err := s.store.CreateProviderAttempt(ctx, attempt); err != nil {
			return s.failMediaJob(ctx, job, "PROVIDER_ATTEMPT_CREATE_FAILED", "无法记录服务商调用尝试")
		}
		if job.RuntimeJobRunID != "" {
			effect, effectErr := s.ensureMediaRuntimeEffect(ctx, job, attempt, attempt.RequestDigest)
			if effectErr != nil {
				return s.failMediaJob(ctx, job, "RUNTIME_EFFECT_REGISTER_FAILED", "无法登记媒体服务商外部操作")
			}
			attempt.RuntimeEffectID = effect.ID
			job.RuntimeEffectID = effect.ID
			if err := s.store.SaveProviderAttempt(ctx, attempt); err != nil {
				return err
			}
			job, err = s.transitionMediaJob(ctx, job, domain.MediaJobSubmitting, func(value *domain.MediaGenerationJob) {
				value.RuntimeEffectID = effect.ID
			})
			if err != nil {
				return err
			}
		}
		submission, submitErr := adapter.Submit(ctx, request, profile)
		if submitErr != nil {
			if !providerErrorRetryable(submitErr) {
				attempt.ProviderState = "failed"
				attempt.ErrorCode = "PROVIDER_SUBMIT_FAILED"
				attempt.ErrorDetailSafe = "服务商拒绝提交请求"
				attempt.UpdatedAt = s.now().UTC()
				if saveErr := s.store.SaveProviderAttempt(ctx, attempt); saveErr != nil {
					return saveErr
				}
				return s.failMediaJob(ctx, job, attempt.ErrorCode, attempt.ErrorDetailSafe)
			}
			attempt.ProviderState = "unknown"
			attempt.ErrorCode = "PROVIDER_SUBMIT_UNKNOWN"
			attempt.ErrorDetailSafe = "服务商提交结果未知，等待对账"
			attempt.UpdatedAt = s.now().UTC()
			if saveErr := s.store.SaveProviderAttempt(ctx, attempt); saveErr != nil {
				return saveErr
			}
			if job.RuntimeEffectID != "" {
				if effectErr := s.transitionMediaEffect(ctx, job, domain.EffectUnknown, "", "", attempt.ErrorCode); effectErr != nil {
					return effectErr
				}
			}
			_, transitionErr := s.transitionMediaJob(ctx, job, domain.MediaJobAwaitingExternal, func(value *domain.MediaGenerationJob) {
				value.LeaseOwner = ""
				value.LeaseExpiresAt = nil
			})
			if transitionErr != nil {
				return transitionErr
			}
			return submitErr
		}
		now = s.now().UTC()
		attempt.ExternalJobID = submission.ExternalJobID
		attempt.ProviderRequestID = submission.ProviderRequestID
		attempt.HTTPStatus = submission.HTTPStatus
		attempt.ProviderState = "submitted"
		attempt.SubmittedAt = &now
		attempt.UpdatedAt = now
		if err := s.store.SaveProviderAttempt(ctx, attempt); err != nil {
			return err
		}
		if job.RuntimeEffectID != "" {
			if effectErr := s.transitionMediaEffect(ctx, job, domain.EffectSubmitted, submission.ExternalJobID, "", ""); effectErr != nil {
				return effectErr
			}
		}
		externalJobID = submission.ExternalJobID
		job, err = s.transitionMediaJob(ctx, job, domain.MediaJobSubmitted, nil)
		if err != nil {
			return err
		}
		job, err = s.transitionMediaJob(ctx, job, domain.MediaJobGenerating, nil)
		if err != nil {
			return err
		}
	}
	providerStatus, statusErr := adapter.Status(ctx, externalJobID, profile)
	if statusErr != nil {
		now = s.now().UTC()
		attempt.ProviderState = "unknown"
		attempt.ErrorCode = "PROVIDER_STATUS_UNKNOWN"
		attempt.ErrorDetailSafe = "服务商状态查询结果未知，等待对账"
		attempt.LastPolledAt = &now
		nextPoll := now.Add(10 * time.Second)
		attempt.NextPollAt = &nextPoll
		attempt.UpdatedAt = now
		if saveErr := s.store.SaveProviderAttempt(ctx, attempt); saveErr != nil {
			return saveErr
		}
		if job.RuntimeEffectID != "" {
			if effectErr := s.transitionMediaEffect(ctx, job, domain.EffectUnknown, externalJobID, "", attempt.ErrorCode); effectErr != nil {
				return effectErr
			}
		}
		_, transitionErr := s.transitionMediaJob(ctx, job, domain.MediaJobAwaitingExternal, func(value *domain.MediaGenerationJob) {
			value.LeaseOwner = ""
			value.LeaseExpiresAt = nil
		})
		if transitionErr != nil {
			return transitionErr
		}
		return statusErr
	}
	providerState := strings.ToLower(strings.TrimSpace(providerStatus.State))
	attempt.HTTPStatus = providerStatus.HTTPStatus
	if providerState != "succeeded" && providerState != "completed" && providerState != "failed" && providerState != "cancelled" && providerState != "canceled" {
		now = s.now().UTC()
		attempt.ProviderState = providerState
		attempt.LastPolledAt = &now
		pollAfter := providerStatus.RetryAfterSeconds
		if pollAfter <= 0 {
			pollAfter = 10
		}
		nextPoll := now.Add(time.Duration(pollAfter) * time.Second)
		attempt.NextPollAt = &nextPoll
		attempt.UpdatedAt = now
		if err := s.store.SaveProviderAttempt(ctx, attempt); err != nil {
			return err
		}
		_, err = s.transitionMediaJob(ctx, job, domain.MediaJobAwaitingExternal, func(value *domain.MediaGenerationJob) {
			value.LeaseOwner = ""
			value.LeaseExpiresAt = nil
		})
		return err
	}
	if (providerState == "cancelled" || providerState == "canceled") && job.CancelRequestedAt != nil {
		now = s.now().UTC()
		attempt.ProviderState = providerState
		attempt.ErrorCode = ""
		attempt.ErrorDetailSafe = "服务商已确认取消"
		attempt.LastPolledAt = &now
		attempt.NextPollAt = nil
		attempt.CompletedAt = &now
		attempt.UpdatedAt = now
		if err := s.store.SaveProviderAttempt(ctx, attempt); err != nil {
			return err
		}
		_, err = s.transitionMediaJob(ctx, job, domain.MediaJobCancelled, func(value *domain.MediaGenerationJob) {
			value.ErrorCode = ""
			value.ErrorDetailSafe = ""
			value.LeaseOwner = ""
			value.LeaseExpiresAt = nil
		})
		return err
	}
	if providerState == "failed" || providerState == "cancelled" || providerState == "canceled" {
		attempt.ProviderState = providerState
		attempt.ErrorCode = "PROVIDER_JOB_FAILED"
		attempt.ErrorDetailSafe = "服务商任务未生成成功"
		attempt.UpdatedAt = s.now().UTC()
		if err := s.store.SaveProviderAttempt(ctx, attempt); err != nil {
			return err
		}
		if job.RuntimeEffectID != "" {
			if effectErr := s.transitionMediaEffect(ctx, job, domain.EffectFailed, externalJobID, "", attempt.ErrorCode); effectErr != nil {
				return effectErr
			}
		}
		return s.failMediaJob(ctx, job, "PROVIDER_JOB_FAILED", "服务商任务未生成成功")
	}
	job, err = s.transitionMediaJob(ctx, job, domain.MediaJobDownloading, nil)
	if err != nil {
		return err
	}
	job, err = s.transitionMediaJob(ctx, job, domain.MediaJobValidating, nil)
	if err != nil {
		return err
	}
	objectKeyPrefix := fmt.Sprintf("media/%s/%s/", job.TenantID, job.ID)
	output, err := s.persistMediaOutput(ctx, adapter, providerStatus.OutputRef, profile, objectKeyPrefix)
	if err != nil {
		var domainErr *domain.Error
		if errors.As(err, &domainErr) {
			return s.failMediaJob(ctx, job, "MEDIA_OUTPUT_INVALID", "服务商输出未通过媒体校验")
		}
		return s.failMediaJob(ctx, job, "PROVIDER_DOWNLOAD_FAILED", "服务商输出下载或对象写入失败")
	}
	if output.FileName == "" {
		return s.failMediaJob(ctx, job, "MEDIA_OUTPUT_INVALID", "服务商输出缺少安全文件名")
	}
	if output.ObjectKey == "" {
		return s.failMediaJob(ctx, job, "MEDIA_OBJECT_WRITE_FAILED", "媒体文件写入对象存储失败")
	}
	now = s.now().UTC()
	artifact := domain.Artifact{ID: domain.NewID(), TenantID: job.TenantID, ProjectID: job.ProjectID, ApprovedSnapshotID: job.StoryboardSnapshotID, Kind: "generated_video", CapabilityID: "contentcloud.media.generate", CapabilityVersion: "1.0.0", CapabilityDigest: job.ProfileDigest, SchemaID: "contentcloud.generated-video/1.0", MediaType: output.MediaType, FileName: output.FileName, SHA256: output.SHA256, ByteSize: output.ByteSize, ObjectKey: output.ObjectKey, Visibility: "client", RetentionClass: "audit", Purpose: "generated_take", Metadata: map[string]any{"task_id": job.TaskID, "generation_job_id": job.ID, "aspect_ratio": job.AspectRatio, "duration_seconds": job.DurationSeconds, "technical": output.Technical, "quarantined": false}, CreatedAt: now}
	if err := s.store.CreateArtifact(ctx, artifact); err != nil {
		if deleter, ok := s.blobs.(blob.DeleteStore); ok {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			_ = deleter.Delete(cleanupCtx, output.ObjectKey)
			cancel()
		}
		return s.failMediaJob(ctx, job, "MEDIA_ARTIFACT_CREATE_FAILED", "媒体成果文件保存失败")
	}
	technicalReview := domain.MediaReview{ID: domain.NewID(), TenantID: job.TenantID, ProjectID: job.ProjectID, TaskID: job.TaskID, GenerationJobID: job.ID, SubjectArtifactID: artifact.ID, SubjectDigest: normalizedSHA256(artifact.SHA256), ReviewKind: domain.MediaReviewTechnical, Status: domain.MediaReviewApproved, Checks: output.Technical, RowVersion: 1, CreatedBy: "media-worker", DecidedBy: "media-worker", DecidedAt: &now, CreatedAt: now, UpdatedAt: now}
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
	if job.RuntimeEffectID != "" {
		if err := s.transitionMediaEffect(ctx, job, domain.EffectSucceeded, externalJobID, "sha256:"+artifact.SHA256, ""); err != nil {
			return err
		}
	}
	_, err = s.transitionMediaJob(ctx, job, domain.MediaJobSucceeded, func(value *domain.MediaGenerationJob) {
		value.ActualCostMinor = providerStatus.ActualMinor
		value.LeaseOwner = ""
		value.LeaseExpiresAt = nil
	})
	return err
}

func (s *Service) ensureMediaRuntimeEffect(ctx context.Context, job domain.MediaGenerationJob, attempt domain.ProviderAttempt, requestDigest string) (domain.ExternalEffect, error) {
	if s.runtimeService == nil || job.RuntimeJobRunID == "" || job.RuntimeNodeRunID == "" {
		return domain.ExternalEffect{}, domain.Policy("RUNTIME_EFFECT_SCOPE_REQUIRED", "Runtime 管理的媒体提交缺少外部操作作用域", "绑定 JobRun 和 NodeRun 后重试")
	}
	if job.RuntimeEffectID != "" {
		effect, err := s.runtimeService.Effect(ctx, job.TenantID, job.RuntimeEffectID)
		if err != nil {
			return domain.ExternalEffect{}, err
		}
		if effect.JobRunID != job.RuntimeJobRunID || effect.NodeRunID != job.RuntimeNodeRunID || effect.RequestDigest != requestDigest {
			return domain.ExternalEffect{}, domain.Conflict("RUNTIME_EFFECT_SCOPE_CONFLICT", "媒体 Job 绑定的外部操作与请求摘要不一致")
		}
		return effect, nil
	}
	return s.runtimeService.RegisterEffect(ctx, domain.ExternalEffect{
		TenantID: job.TenantID, JobRunID: job.RuntimeJobRunID, NodeRunID: job.RuntimeNodeRunID,
		AttemptID: attempt.RuntimeAttemptID, Kind: "media.generate", IdempotencyKey: "media-effect:" + job.ID + ":" + attempt.ID,
		RequestDigest: requestDigest, CostMinor: job.EstimatedCostMinor, Currency: job.Currency,
		SafeSummary: map[string]any{"provider_id": job.ProviderID, "generation_job_id": job.ID, "attempt_id": attempt.ID},
	})
}

func (s *Service) transitionMediaEffect(ctx context.Context, job domain.MediaGenerationJob, next, externalID, responseDigest, errorCode string) error {
	if s.runtimeService == nil || job.RuntimeEffectID == "" {
		return nil
	}
	effect, err := s.runtimeService.Effect(ctx, job.TenantID, job.RuntimeEffectID)
	if err != nil {
		return err
	}
	if effect.State == next && effect.ExternalID == externalID && effect.ResponseDigest == responseDigest {
		return nil
	}
	if (next == domain.EffectSucceeded || next == domain.EffectFailed) && effect.State == domain.EffectUnknown {
		if effect, err = s.runtimeService.ReconcileEffect(ctx, job.TenantID, effect.ID, domain.EffectReconciling, externalID, responseDigest, errorCode, effect.Version); err != nil {
			return err
		}
	}
	if next == domain.EffectSucceeded && effect.State == domain.EffectSubmitted {
		if effect, err = s.runtimeService.ReconcileEffect(ctx, job.TenantID, effect.ID, domain.EffectAcknowledged, externalID, responseDigest, errorCode, effect.Version); err != nil {
			return err
		}
	}
	_, err = s.runtimeService.ReconcileEffect(ctx, job.TenantID, effect.ID, next, externalID, responseDigest, errorCode, effect.Version)
	return err
}

type persistedMediaOutput struct {
	Technical map[string]any
	SHA256    string
	ByteSize  int64
	MediaType string
	FileName  string
	ObjectKey string
}

func (s *Service) persistMediaOutput(ctx context.Context, adapter mediapipeline.Adapter, outputRef string, profile domain.ProviderProfile, objectKeyPrefix string) (result persistedMediaOutput, err error) {
	const maxBytes int64 = 10 << 20
	var writtenObjectKey string
	defer func() {
		if writtenObjectKey == "" || result.ObjectKey != "" {
			return
		}
		if deleter, ok := s.blobs.(blob.DeleteStore); ok {
			_ = deleter.Delete(context.WithoutCancel(ctx), writtenObjectKey)
		}
	}()
	streaming, supportsStreaming := adapter.(mediapipeline.StreamingDownloader)
	if !supportsStreaming {
		download, err := adapter.Download(ctx, outputRef, profile)
		if err != nil {
			return persistedMediaOutput{}, err
		}
		technical, err := mediapipeline.ValidateDownload(download, maxBytes)
		if err != nil {
			return persistedMediaOutput{}, err
		}
		if err := validateMediaFileName(download.FileName); err != nil {
			return persistedMediaOutput{}, err
		}
		objectKey := objectKeyPrefix + download.FileName
		// Mark the key before the write so a store that reports an error after
		// partially persisting the object can still be cleaned up by the defer.
		writtenObjectKey = objectKey
		if err := s.blobs.Put(ctx, objectKey, download.Body); err != nil {
			return persistedMediaOutput{}, err
		}
		return persistedMediaOutput{Technical: technical, SHA256: mediapipeline.SHA256(download.Body), ByteSize: int64(len(download.Body)), MediaType: download.MediaType, FileName: download.FileName, ObjectKey: objectKey}, nil
	}
	stream, err := streaming.OpenDownload(ctx, outputRef, profile)
	if err != nil {
		return persistedMediaOutput{}, err
	}
	if stream.Body == nil {
		return persistedMediaOutput{}, domain.Invalid("MEDIA_OUTPUT_STREAM_INVALID", "服务商输出流为空")
	}
	defer stream.Body.Close()
	if err := validateMediaFileName(stream.FileName); err != nil {
		return persistedMediaOutput{}, err
	}
	temp, err := os.CreateTemp("", "contentcloud-media-*")
	if err != nil {
		return persistedMediaOutput{}, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	written, copyErr := io.Copy(temp, io.LimitReader(stream.Body, maxBytes+1))
	if copyErr != nil {
		_ = temp.Close()
		return persistedMediaOutput{}, copyErr
	}
	if written > maxBytes {
		_ = temp.Close()
		return persistedMediaOutput{}, domain.Invalid("MEDIA_OUTPUT_SIZE_INVALID", "服务商输出为空或超过大小限制")
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return persistedMediaOutput{}, err
	}
	if err := temp.Close(); err != nil {
		return persistedMediaOutput{}, err
	}
	input, err := os.Open(tempName)
	if err != nil {
		return persistedMediaOutput{}, err
	}
	validated, validationErr := mediapipeline.ValidateDownloadReader(input, stream.MediaType, maxBytes)
	_ = input.Close()
	if validationErr != nil {
		return persistedMediaOutput{}, validationErr
	}
	input, err = os.Open(tempName)
	if err != nil {
		return persistedMediaOutput{}, err
	}
	objectKey := objectKeyPrefix + stream.FileName
	writtenObjectKey = objectKey
	if readerStore, ok := s.blobs.(blob.ReaderStore); ok {
		err = readerStore.PutReader(ctx, objectKey, input, validated.ByteSize)
	} else {
		var data []byte
		data, err = io.ReadAll(io.LimitReader(input, maxBytes+1))
		if err == nil && int64(len(data)) > maxBytes {
			err = domain.Invalid("MEDIA_OUTPUT_SIZE_INVALID", "服务商输出超过大小限制")
		}
		if err == nil {
			err = s.blobs.Put(ctx, objectKey, data)
		}
	}
	_ = input.Close()
	if err != nil {
		return persistedMediaOutput{}, err
	}
	return persistedMediaOutput{Technical: validated.Technical, SHA256: validated.SHA256, ByteSize: validated.ByteSize, MediaType: stream.MediaType, FileName: stream.FileName, ObjectKey: objectKey}, nil
}

func validateMediaFileName(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "/\\\x00") || value == "." || value == ".." {
		return domain.Invalid("MEDIA_OUTPUT_FILENAME_INVALID", "服务商输出文件名无效")
	}
	return nil
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

func providerErrorRetryable(err error) bool {
	var value *domain.Error
	if !errors.As(err, &value) {
		return true
	}
	return value.Retryable
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

func (s *Service) mediaAdapter(providerID string) (mediapipeline.Adapter, error) {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if adapter, ok := s.mediaAdapters[providerID]; ok {
		return adapter, nil
	}
	if providerID == "fake" {
		return mediapipeline.FakeProvider{}, nil
	}
	return nil, domain.NotFound("服务商适配器")
}

func profileCurrency(profile domain.ProviderProfile) string {
	value, _ := profile.Pricing["currency"].(string)
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 3 {
		return "CNY"
	}
	return value
}
