package memory

import (
	"context"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateExecutionBindingSnapshot(_ context.Context, value domain.ExecutionBindingSnapshot) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimePlanKey(value.TenantID, value.Digest)
	if existing, ok := s.runtimeExecutionBindings[key]; ok {
		if existing.Digest == value.Digest {
			return domain.Conflict("EXECUTION_BINDING_SNAPSHOT_EXISTS", "相同摘要的 ExecutionBindingSnapshot 已存在")
		}
	}
	s.runtimeExecutionBindings[key] = value
	return nil
}

func (s *Store) ExecutionBindingSnapshot(_ context.Context, tenantID, digest string) (domain.ExecutionBindingSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeExecutionBindings[runtimePlanKey(tenantID, digest)]
	if !ok {
		return value, domain.NotFound("ExecutionBindingSnapshot")
	}
	return value, nil
}
