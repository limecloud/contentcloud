package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/work"
)

func cloneInputItem(value work.InputItem) work.InputItem {
	value.NormalizeCollections()
	value.MissingFields = append([]string{}, value.MissingFields...)
	value.Metadata = cloneTaskMap(value.Metadata)
	return value
}

func (s *Store) CreateInputItem(_ context.Context, value work.InputItem) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.inputItems[value.ID]; exists {
		return fault.Conflict("INPUT_ITEM_EXISTS", "输入收集记录已存在")
	}
	if value.IdempotencyKey != "" {
		for _, existing := range s.inputItems {
			if existing.TenantID == value.TenantID && existing.IdempotencyKey == value.IdempotencyKey {
				return fault.Conflict("IDEMPOTENCY_REPLAY", "相同幂等键已经创建过输入收集记录")
			}
		}
	}
	s.inputItems[value.ID] = cloneInputItem(value)
	return nil
}

func (s *Store) InputItems(_ context.Context, tenantID, projectID, status, assigneeUserID string) ([]work.InputItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []work.InputItem{}
	for _, value := range s.inputItems {
		if value.TenantID != tenantID || (projectID != "" && value.ProjectID != projectID) || (status != "" && value.Status != status) || (assigneeUserID != "" && value.AssigneeUserID != assigneeUserID) {
			continue
		}
		result = append(result, cloneInputItem(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (s *Store) InputItem(_ context.Context, tenantID, id string) (work.InputItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.inputItems[id]
	if !ok || value.TenantID != tenantID {
		return value, fault.NotFound("输入收集记录")
	}
	return cloneInputItem(value), nil
}

func (s *Store) InputItemByIdempotencyKey(_ context.Context, tenantID, key string) (work.InputItem, error) {
	if key == "" {
		return work.InputItem{}, fault.NotFound("输入收集记录")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.inputItems {
		if value.TenantID == tenantID && value.IdempotencyKey == key {
			return cloneInputItem(value), nil
		}
	}
	return work.InputItem{}, fault.NotFound("输入收集记录")
}

func (s *Store) SaveInputItem(_ context.Context, value work.InputItem, expectedVersion int) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.inputItems[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return fault.NotFound("输入收集记录")
	}
	if current.RowVersion != expectedVersion || value.RowVersion != expectedVersion+1 {
		return fault.Conflict("INPUT_ITEM_VERSION_CONFLICT", "输入收集记录已被其他人更新，请刷新后重试")
	}
	if current.Status == work.InputItemArchived && value.Status != current.Status {
		return fault.Conflict("INPUT_ITEM_ARCHIVED", "已归档的输入收集记录不可再次分流")
	}
	s.inputItems[value.ID] = cloneInputItem(value)
	return nil
}
