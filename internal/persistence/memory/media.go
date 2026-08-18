package memory

import (
	"context"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/work"
)

func cloneStageOutput(value work.TaskStageOutput) work.TaskStageOutput {
	value.NormalizeCollections()
	value.Metadata = cloneTaskMap(value.Metadata)
	return value
}

func cloneMediaJob(value deliverydomain.MediaGenerationJob) deliverydomain.MediaGenerationJob {
	value.NormalizeCollections()
	value.InputArtifactRefs = append([]string{}, value.InputArtifactRefs...)
	return value
}

func cloneProviderProfile(value deliverydomain.ProviderProfile) deliverydomain.ProviderProfile {
	value.NormalizeCollections()
	value.Modes = append([]string{}, value.Modes...)
	value.InputMediaTypes = append([]string{}, value.InputMediaTypes...)
	value.Limits = cloneTaskMap(value.Limits)
	value.Pricing = cloneTaskMap(value.Pricing)
	return value
}

func cloneProviderAttempt(value deliverydomain.ProviderAttempt) deliverydomain.ProviderAttempt {
	value.NormalizeCollections()
	value.SafeRequestSummary = cloneTaskMap(value.SafeRequestSummary)
	value.SafeResponseSummary = cloneTaskMap(value.SafeResponseSummary)
	value.DisclosureManifest = cloneTaskMap(value.DisclosureManifest)
	return value
}

func cloneMediaReview(value deliverydomain.MediaReview) deliverydomain.MediaReview {
	value.NormalizeCollections()
	value.Checks = cloneTaskMap(value.Checks)
	return value
}

func (s *Store) CompleteStageRun(_ context.Context, run work.StageRun, outputs []work.TaskStageOutput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.stageRuns[run.ID]
	if !ok || current.TenantID != run.TenantID || current.TaskID != run.TaskID {
		return fault.NotFound("阶段运行")
	}
	for _, output := range outputs {
		output.NormalizeCollections()
		if err := output.Validate(); err != nil {
			return err
		}
		if output.TenantID != run.TenantID || output.TaskID != run.TaskID || output.StageRunID != run.ID || output.StageID != run.StageID {
			return fault.Invalid("TASK_STAGE_OUTPUT_SCOPE_INVALID", "阶段输出与阶段执行记录的作用域不一致")
		}
		if _, exists := s.stageOutputs[output.ID]; exists {
			return fault.Conflict("TASK_STAGE_OUTPUT_EXISTS", "阶段输出已存在")
		}
	}
	for _, output := range outputs {
		s.stageOutputs[output.ID] = cloneStageOutput(output)
	}
	run.Outputs = make([]work.TaskStageOutput, 0, len(outputs))
	for _, output := range outputs {
		run.Outputs = append(run.Outputs, cloneStageOutput(output))
	}
	s.stageRuns[run.ID] = run
	return nil
}

func (s *Store) TaskStageOutputs(_ context.Context, tenantID, taskID string) ([]work.TaskStageOutput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []work.TaskStageOutput{}
	for _, value := range s.stageOutputs {
		if value.TenantID == tenantID && value.TaskID == taskID {
			result = append(result, cloneStageOutput(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func providerProfileKey(providerID, version string) string { return providerID + ":" + version }

func (s *Store) CreateProviderProfile(_ context.Context, value deliverydomain.ProviderProfile) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := providerProfileKey(value.ProviderID, value.Version)
	if _, exists := s.providerProfiles[key]; exists {
		return fault.Conflict("PROVIDER_PROFILE_EXISTS", "服务提供方配置已存在")
	}
	s.providerProfiles[key] = cloneProviderProfile(value)
	return nil
}

func (s *Store) ProviderProfile(_ context.Context, providerID, version string) (deliverydomain.ProviderProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.providerProfiles[providerProfileKey(providerID, version)]
	if !ok {
		return value, fault.NotFound("服务商配置")
	}
	return cloneProviderProfile(value), nil
}

func (s *Store) SaveProviderProfile(_ context.Context, value deliverydomain.ProviderProfile) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := providerProfileKey(value.ProviderID, value.Version)
	if _, ok := s.providerProfiles[key]; !ok {
		return fault.NotFound("服务商配置")
	}
	s.providerProfiles[key] = cloneProviderProfile(value)
	return nil
}

func (s *Store) ProviderProfiles(_ context.Context, providerID string) ([]deliverydomain.ProviderProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []deliverydomain.ProviderProfile{}
	for _, value := range s.providerProfiles {
		if providerID == "" || value.ProviderID == providerID {
			result = append(result, cloneProviderProfile(value))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ProviderID == result[j].ProviderID {
			return result[i].Version < result[j].Version
		}
		return result[i].ProviderID < result[j].ProviderID
	})
	return result, nil
}

func (s *Store) SaveProviderBinding(_ context.Context, value deliverydomain.ProviderBinding) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providerProfiles[providerProfileKey(value.ProviderID, value.ProfileVersion)]; !ok {
		return fault.NotFound("服务商配置")
	}
	s.providerBindings[value.TenantID+":"+value.ProviderID] = value
	return nil
}

func (s *Store) ProviderBinding(_ context.Context, tenantID, providerID string) (deliverydomain.ProviderBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.providerBindings[tenantID+":"+providerID]
	if !ok {
		return value, fault.NotFound("服务商绑定")
	}
	return value, nil
}

func (s *Store) CreateMediaGenerationJob(_ context.Context, value deliverydomain.MediaGenerationJob) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.mediaJobs[value.ID]; exists {
		return fault.Conflict("MEDIA_JOB_EXISTS", "媒体生成任务已存在")
	}
	for _, existing := range s.mediaJobs {
		if existing.TenantID == value.TenantID && existing.IdempotencyKey == value.IdempotencyKey {
			return fault.Conflict("MEDIA_JOB_IDEMPOTENCY_CONFLICT", "相同幂等键已创建媒体生成任务")
		}
	}
	s.mediaJobs[value.ID] = cloneMediaJob(value)
	return nil
}

func (s *Store) PendingMediaGenerationJobs(_ context.Context, limit int) ([]deliverydomain.MediaGenerationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	result := []deliverydomain.MediaGenerationJob{}
	for _, value := range s.mediaJobs {
		pending := value.State == deliverydomain.MediaJobQueued || value.State == deliverydomain.MediaJobRetryWait
		if value.State == deliverydomain.MediaJobAwaitingExternal {
			for _, attempt := range s.providerAttempts {
				if attempt.TenantID == value.TenantID && attempt.GenerationJobID == value.ID && attempt.NextPollAt != nil && !attempt.NextPollAt.After(now) {
					pending = true
				}
			}
		}
		if pending {
			result = append(result, cloneMediaJob(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.Before(result[j].UpdatedAt) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *Store) MediaGenerationJob(_ context.Context, tenantID, id string) (deliverydomain.MediaGenerationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.mediaJobs[id]
	if !ok || value.TenantID != tenantID {
		return value, fault.NotFound("媒体生成任务")
	}
	return cloneMediaJob(value), nil
}

func (s *Store) MediaGenerationJobs(_ context.Context, tenantID, taskID string) ([]deliverydomain.MediaGenerationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []deliverydomain.MediaGenerationJob{}
	for _, value := range s.mediaJobs {
		if value.TenantID == tenantID && value.TaskID == taskID {
			result = append(result, cloneMediaJob(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) SaveMediaGenerationJob(_ context.Context, value deliverydomain.MediaGenerationJob, expectedVersion int) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.mediaJobs[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return fault.NotFound("媒体生成任务")
	}
	if current.RowVersion != expectedVersion {
		return fault.Conflict("MEDIA_JOB_STALE", "媒体生成任务已被其他操作更新")
	}
	if !deliverydomain.CanTransitionMediaJob(current.State, value.State) {
		return fault.Conflict("MEDIA_JOB_TRANSITION_INVALID", "媒体生成任务状态转换无效")
	}
	value.RowVersion = expectedVersion + 1
	s.mediaJobs[value.ID] = cloneMediaJob(value)
	return nil
}

func (s *Store) CreateProviderAttempt(_ context.Context, value deliverydomain.ProviderAttempt) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.providerAttempts[value.ID]; exists {
		return fault.Conflict("PROVIDER_ATTEMPT_EXISTS", "服务提供方执行尝试已存在")
	}
	for _, existing := range s.providerAttempts {
		if existing.TenantID == value.TenantID && existing.GenerationJobID == value.GenerationJobID && existing.AttemptNumber == value.AttemptNumber {
			return fault.Conflict("PROVIDER_ATTEMPT_NUMBER_EXISTS", "服务提供方执行尝试序号已存在")
		}
	}
	s.providerAttempts[value.ID] = cloneProviderAttempt(value)
	return nil
}

func (s *Store) SaveProviderAttempt(_ context.Context, value deliverydomain.ProviderAttempt) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.providerAttempts[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return fault.NotFound("服务商执行尝试")
	}
	s.providerAttempts[value.ID] = cloneProviderAttempt(value)
	return nil
}

func (s *Store) ProviderAttempts(_ context.Context, tenantID, jobID string) ([]deliverydomain.ProviderAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []deliverydomain.ProviderAttempt{}
	for _, value := range s.providerAttempts {
		if value.TenantID == tenantID && value.GenerationJobID == jobID {
			result = append(result, cloneProviderAttempt(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AttemptNumber < result[j].AttemptNumber })
	return result, nil
}

func (s *Store) ProviderAttemptsByRuntimeJob(_ context.Context, tenantID, runtimeJobID string) ([]deliverydomain.ProviderAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []deliverydomain.ProviderAttempt{}
	for _, value := range s.providerAttempts {
		if value.TenantID == tenantID && value.RuntimeJobRunID == runtimeJobID {
			result = append(result, cloneProviderAttempt(value))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].GenerationJobID == result[j].GenerationJobID {
			return result[i].AttemptNumber < result[j].AttemptNumber
		}
		return result[i].GenerationJobID < result[j].GenerationJobID
	})
	return result, nil
}

func (s *Store) CreateMediaReview(_ context.Context, value deliverydomain.MediaReview) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.mediaReviews[value.ID]; exists {
		return fault.Conflict("MEDIA_REVIEW_EXISTS", "媒体审核已存在")
	}
	if value.Selected {
		for _, existing := range s.mediaReviews {
			if existing.TenantID == value.TenantID && existing.TaskID == value.TaskID && existing.ReviewKind == value.ReviewKind && existing.Selected {
				return fault.Conflict("MEDIA_REVIEW_SELECTION_EXISTS", "该审核类型已有选中成片")
			}
		}
	}
	s.mediaReviews[value.ID] = cloneMediaReview(value)
	return nil
}

func (s *Store) SaveMediaReview(_ context.Context, value deliverydomain.MediaReview, expectedVersion int) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.mediaReviews[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return fault.NotFound("媒体审核")
	}
	if current.RowVersion != expectedVersion {
		return fault.Conflict("MEDIA_REVIEW_STALE", "媒体审核已被其他操作更新")
	}
	if value.Selected {
		for _, existing := range s.mediaReviews {
			if existing.ID != value.ID && existing.TenantID == value.TenantID && existing.TaskID == value.TaskID && existing.ReviewKind == value.ReviewKind && existing.Selected {
				return fault.Conflict("MEDIA_REVIEW_SELECTION_EXISTS", "该审核类型已有选中成片")
			}
		}
	}
	value.RowVersion = expectedVersion + 1
	s.mediaReviews[value.ID] = cloneMediaReview(value)
	return nil
}

func (s *Store) MediaReview(_ context.Context, tenantID, id string) (deliverydomain.MediaReview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.mediaReviews[id]
	if !ok || value.TenantID != tenantID {
		return value, fault.NotFound("媒体审核")
	}
	return cloneMediaReview(value), nil
}

func (s *Store) MediaReviews(_ context.Context, tenantID, taskID string) ([]deliverydomain.MediaReview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []deliverydomain.MediaReview{}
	for _, value := range s.mediaReviews {
		if value.TenantID == tenantID && value.TaskID == taskID {
			result = append(result, cloneMediaReview(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}
