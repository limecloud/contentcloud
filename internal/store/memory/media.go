package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/domain"
)

func cloneStageOutput(value domain.TaskStageOutput) domain.TaskStageOutput {
	value.NormalizeCollections()
	value.Metadata = cloneTaskMap(value.Metadata)
	return value
}

func cloneMediaJob(value domain.MediaGenerationJob) domain.MediaGenerationJob {
	value.NormalizeCollections()
	value.InputArtifactRefs = append([]string{}, value.InputArtifactRefs...)
	return value
}

func cloneProviderProfile(value domain.ProviderProfile) domain.ProviderProfile {
	value.NormalizeCollections()
	value.Modes = append([]string{}, value.Modes...)
	value.InputMediaTypes = append([]string{}, value.InputMediaTypes...)
	value.Limits = cloneTaskMap(value.Limits)
	value.Pricing = cloneTaskMap(value.Pricing)
	return value
}

func cloneProviderAttempt(value domain.ProviderAttempt) domain.ProviderAttempt {
	value.NormalizeCollections()
	value.SafeRequestSummary = cloneTaskMap(value.SafeRequestSummary)
	value.SafeResponseSummary = cloneTaskMap(value.SafeResponseSummary)
	value.DisclosureManifest = cloneTaskMap(value.DisclosureManifest)
	return value
}

func cloneMediaReview(value domain.MediaReview) domain.MediaReview {
	value.NormalizeCollections()
	value.Checks = cloneTaskMap(value.Checks)
	return value
}

func (s *Store) CompleteStageRun(_ context.Context, run domain.StageRun, outputs []domain.TaskStageOutput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.stageRuns[run.ID]
	if !ok || current.TenantID != run.TenantID || current.TaskID != run.TaskID {
		return domain.NotFound("StageRun")
	}
	for _, output := range outputs {
		output.NormalizeCollections()
		if err := output.Validate(); err != nil {
			return err
		}
		if output.TenantID != run.TenantID || output.TaskID != run.TaskID || output.StageRunID != run.ID || output.StageID != run.StageID {
			return domain.Invalid("TASK_STAGE_OUTPUT_SCOPE_INVALID", "Stage 输出与 StageRun 作用域不一致")
		}
		if _, exists := s.stageOutputs[output.ID]; exists {
			return domain.Conflict("TASK_STAGE_OUTPUT_EXISTS", "Stage 输出已存在")
		}
	}
	for _, output := range outputs {
		s.stageOutputs[output.ID] = cloneStageOutput(output)
	}
	run.Outputs = make([]domain.TaskStageOutput, 0, len(outputs))
	for _, output := range outputs {
		run.Outputs = append(run.Outputs, cloneStageOutput(output))
	}
	s.stageRuns[run.ID] = run
	return nil
}

func (s *Store) TaskStageOutputs(_ context.Context, tenantID, taskID string) ([]domain.TaskStageOutput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.TaskStageOutput{}
	for _, value := range s.stageOutputs {
		if value.TenantID == tenantID && value.TaskID == taskID {
			result = append(result, cloneStageOutput(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func providerProfileKey(providerID, version string) string { return providerID + ":" + version }

func (s *Store) CreateProviderProfile(_ context.Context, value domain.ProviderProfile) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := providerProfileKey(value.ProviderID, value.Version)
	if _, exists := s.providerProfiles[key]; exists {
		return domain.Conflict("PROVIDER_PROFILE_EXISTS", "Provider Profile 已存在")
	}
	s.providerProfiles[key] = cloneProviderProfile(value)
	return nil
}

func (s *Store) ProviderProfile(_ context.Context, providerID, version string) (domain.ProviderProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.providerProfiles[providerProfileKey(providerID, version)]
	if !ok {
		return value, domain.NotFound("Provider Profile")
	}
	return cloneProviderProfile(value), nil
}

func (s *Store) SaveProviderBinding(_ context.Context, value domain.ProviderBinding) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providerProfiles[providerProfileKey(value.ProviderID, value.ProfileVersion)]; !ok {
		return domain.NotFound("Provider Profile")
	}
	s.providerBindings[value.TenantID+":"+value.ProviderID] = value
	return nil
}

func (s *Store) ProviderBinding(_ context.Context, tenantID, providerID string) (domain.ProviderBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.providerBindings[tenantID+":"+providerID]
	if !ok {
		return value, domain.NotFound("Provider Binding")
	}
	return value, nil
}

func (s *Store) CreateMediaGenerationJob(_ context.Context, value domain.MediaGenerationJob) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.mediaJobs[value.ID]; exists {
		return domain.Conflict("MEDIA_JOB_EXISTS", "媒体 Job 已存在")
	}
	for _, existing := range s.mediaJobs {
		if existing.TenantID == value.TenantID && existing.IdempotencyKey == value.IdempotencyKey {
			return domain.Conflict("MEDIA_JOB_IDEMPOTENCY_CONFLICT", "相同幂等键已创建媒体 Job")
		}
	}
	s.mediaJobs[value.ID] = cloneMediaJob(value)
	return nil
}

func (s *Store) PendingMediaGenerationJobs(_ context.Context, limit int) ([]domain.MediaGenerationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.MediaGenerationJob{}
	for _, value := range s.mediaJobs {
		if value.State == domain.MediaJobQueued || value.State == domain.MediaJobRetryWait {
			result = append(result, cloneMediaJob(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.Before(result[j].UpdatedAt) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *Store) MediaGenerationJob(_ context.Context, tenantID, id string) (domain.MediaGenerationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.mediaJobs[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("媒体 Job")
	}
	return cloneMediaJob(value), nil
}

func (s *Store) MediaGenerationJobs(_ context.Context, tenantID, taskID string) ([]domain.MediaGenerationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.MediaGenerationJob{}
	for _, value := range s.mediaJobs {
		if value.TenantID == tenantID && value.TaskID == taskID {
			result = append(result, cloneMediaJob(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) SaveMediaGenerationJob(_ context.Context, value domain.MediaGenerationJob, expectedVersion int) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.mediaJobs[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("媒体 Job")
	}
	if current.RowVersion != expectedVersion {
		return domain.Conflict("MEDIA_JOB_STALE", "媒体 Job 已被其他操作更新")
	}
	if !domain.CanTransitionMediaJob(current.State, value.State) {
		return domain.Conflict("MEDIA_JOB_TRANSITION_INVALID", "媒体 Job 状态转换无效")
	}
	value.RowVersion = expectedVersion + 1
	s.mediaJobs[value.ID] = cloneMediaJob(value)
	return nil
}

func (s *Store) CreateProviderAttempt(_ context.Context, value domain.ProviderAttempt) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.providerAttempts[value.ID]; exists {
		return domain.Conflict("PROVIDER_ATTEMPT_EXISTS", "Provider Attempt 已存在")
	}
	for _, existing := range s.providerAttempts {
		if existing.TenantID == value.TenantID && existing.GenerationJobID == value.GenerationJobID && existing.AttemptNumber == value.AttemptNumber {
			return domain.Conflict("PROVIDER_ATTEMPT_NUMBER_EXISTS", "Provider Attempt 序号已存在")
		}
	}
	s.providerAttempts[value.ID] = cloneProviderAttempt(value)
	return nil
}

func (s *Store) SaveProviderAttempt(_ context.Context, value domain.ProviderAttempt) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.providerAttempts[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("Provider Attempt")
	}
	s.providerAttempts[value.ID] = cloneProviderAttempt(value)
	return nil
}

func (s *Store) ProviderAttempts(_ context.Context, tenantID, jobID string) ([]domain.ProviderAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ProviderAttempt{}
	for _, value := range s.providerAttempts {
		if value.TenantID == tenantID && value.GenerationJobID == jobID {
			result = append(result, cloneProviderAttempt(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AttemptNumber < result[j].AttemptNumber })
	return result, nil
}

func (s *Store) CreateMediaReview(_ context.Context, value domain.MediaReview) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.mediaReviews[value.ID]; exists {
		return domain.Conflict("MEDIA_REVIEW_EXISTS", "媒体审核已存在")
	}
	if value.Selected {
		for _, existing := range s.mediaReviews {
			if existing.TenantID == value.TenantID && existing.TaskID == value.TaskID && existing.ReviewKind == value.ReviewKind && existing.Selected {
				return domain.Conflict("MEDIA_REVIEW_SELECTION_EXISTS", "该审核类型已有选中成片")
			}
		}
	}
	s.mediaReviews[value.ID] = cloneMediaReview(value)
	return nil
}

func (s *Store) SaveMediaReview(_ context.Context, value domain.MediaReview, expectedVersion int) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.mediaReviews[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("媒体审核")
	}
	if current.RowVersion != expectedVersion {
		return domain.Conflict("MEDIA_REVIEW_STALE", "媒体审核已被其他操作更新")
	}
	if value.Selected {
		for _, existing := range s.mediaReviews {
			if existing.ID != value.ID && existing.TenantID == value.TenantID && existing.TaskID == value.TaskID && existing.ReviewKind == value.ReviewKind && existing.Selected {
				return domain.Conflict("MEDIA_REVIEW_SELECTION_EXISTS", "该审核类型已有选中成片")
			}
		}
	}
	value.RowVersion = expectedVersion + 1
	s.mediaReviews[value.ID] = cloneMediaReview(value)
	return nil
}

func (s *Store) MediaReview(_ context.Context, tenantID, id string) (domain.MediaReview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.mediaReviews[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("媒体审核")
	}
	return cloneMediaReview(value), nil
}

func (s *Store) MediaReviews(_ context.Context, tenantID, taskID string) ([]domain.MediaReview, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.MediaReview{}
	for _, value := range s.mediaReviews {
		if value.TenantID == tenantID && value.TaskID == taskID {
			result = append(result, cloneMediaReview(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}
