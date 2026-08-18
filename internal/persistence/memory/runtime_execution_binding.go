package memory

import (
	"context"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

func (s *Store) CreateExecutionBindingSnapshot(_ context.Context, value contentruntime.ExecutionBindingSnapshot) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimePlanKey(value.TenantID, value.Digest)
	if existing, ok := s.runtimeExecutionBindings[key]; ok {
		if existing.Digest == value.Digest {
			return fault.Conflict("EXECUTION_BINDING_SNAPSHOT_EXISTS", "相同摘要的 ExecutionBindingSnapshot 已存在")
		}
	}
	s.runtimeExecutionBindings[key] = value
	return nil
}

func (s *Store) ExecutionBindingSnapshot(_ context.Context, tenantID, digest string) (contentruntime.ExecutionBindingSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeExecutionBindings[runtimePlanKey(tenantID, digest)]
	if !ok {
		return value, fault.NotFound("ExecutionBindingSnapshot")
	}
	return value, nil
}
