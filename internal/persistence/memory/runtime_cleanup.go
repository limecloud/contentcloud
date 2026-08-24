package memory

import (
	"context"
	"sort"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

func runtimeCleanupKey(tenantID, id string) string { return tenantID + ":" + id }

func (s *Store) CreateRuntimeCleanupDiagnostic(_ context.Context, value contentruntime.RuntimeCleanupDiagnostic) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runtimeCleanupDiagnostics {
		if existing.TenantID == value.TenantID && existing.ObjectKey == value.ObjectKey {
			return fault.Conflict("RUNTIME_CLEANUP_DIAGNOSTIC_EXISTS", "Blob 清理诊断已存在")
		}
	}
	key := runtimeCleanupKey(value.TenantID, value.ID)
	if _, exists := s.runtimeCleanupDiagnostics[key]; exists {
		return fault.Conflict("RUNTIME_CLEANUP_DIAGNOSTIC_EXISTS", "Blob 清理诊断已存在")
	}
	s.runtimeCleanupDiagnostics[key] = value
	return nil
}

func (s *Store) UpdateRuntimeCleanupDiagnostic(_ context.Context, value contentruntime.RuntimeCleanupDiagnostic, expectedVersion int) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeCleanupKey(value.TenantID, value.ID)
	current, ok := s.runtimeCleanupDiagnostics[key]
	if !ok {
		return fault.NotFound("Blob 清理诊断")
	}
	if current.Version != expectedVersion || value.Version != expectedVersion+1 {
		return fault.Conflict("RUNTIME_CLEANUP_DIAGNOSTIC_VERSION_CONFLICT", "Blob 清理诊断已被更新")
	}
	if err := value.ValidateTransition(current); err != nil {
		return err
	}
	s.runtimeCleanupDiagnostics[key] = value
	return nil
}

func (s *Store) RuntimeCleanupDiagnostic(_ context.Context, tenantID, id string) (contentruntime.RuntimeCleanupDiagnostic, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeCleanupDiagnostics[runtimeCleanupKey(tenantID, id)]
	if !ok {
		return value, fault.NotFound("Blob 清理诊断")
	}
	return value, nil
}

func (s *Store) RuntimeCleanupDiagnostics(_ context.Context, tenantID, status string, limit int) ([]contentruntime.RuntimeCleanupDiagnostic, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	result := make([]contentruntime.RuntimeCleanupDiagnostic, 0)
	for _, value := range s.runtimeCleanupDiagnostics {
		if value.TenantID == tenantID && (status == "" || value.Status == status) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.Before(result[j].UpdatedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
