package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateRuntimeProjectionRebuild(_ context.Context, run domain.RuntimeProjectionRebuildRun) error {
	if err := run.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimePlanKey(run.TenantID, run.ID)
	if _, exists := s.runtimeProjectionRebuilds[key]; exists {
		return domain.Conflict("RUNTIME_PROJECTION_REBUILD_EXISTS", "投影重建运行事实已存在")
	}
	s.runtimeProjectionRebuilds[key] = run
	return nil
}

func (s *Store) UpdateRuntimeProjectionRebuild(_ context.Context, run domain.RuntimeProjectionRebuildRun, expectedVersion int) error {
	if err := run.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimePlanKey(run.TenantID, run.ID)
	current, ok := s.runtimeProjectionRebuilds[key]
	if !ok {
		return domain.NotFound("投影重建运行事实")
	}
	if current.Version != expectedVersion || run.Version != expectedVersion+1 {
		return domain.Conflict("RUNTIME_PROJECTION_REBUILD_VERSION_CONFLICT", "投影重建运行事实已被更新")
	}
	s.runtimeProjectionRebuilds[key] = run
	return nil
}

func (s *Store) RuntimeProjectionRebuilds(_ context.Context, tenantID, jobID string) ([]domain.RuntimeProjectionRebuildRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.RuntimeProjectionRebuildRun{}
	for _, run := range s.runtimeProjectionRebuilds {
		if run.TenantID == tenantID && (jobID == "" || run.JobRunID == jobID) {
			result = append(result, run)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartedAt.After(result[j].StartedAt) })
	return result, nil
}
