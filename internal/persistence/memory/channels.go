package memory

import (
	"context"
	"sort"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

func cloneChannelPublication(value deliverydomain.ChannelPublication) deliverydomain.ChannelPublication {
	value.NormalizeCollections()
	value.Checklist = append([]string{}, value.Checklist...)
	value.Preview = cloneTaskMap(value.Preview)
	value.Metadata = cloneTaskMap(value.Metadata)
	value.SafeSummary = cloneTaskMap(value.SafeSummary)
	return value
}

func (s *Store) CreateChannelBinding(_ context.Context, value deliverydomain.ChannelBinding) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.channelBindings[value.ID]; exists {
		return fault.Conflict("CHANNEL_BINDING_EXISTS", "渠道绑定已存在")
	}
	s.channelBindings[value.ID] = value
	return nil
}

func (s *Store) ChannelBindings(_ context.Context, tenantID, projectID string) ([]deliverydomain.ChannelBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := []deliverydomain.ChannelBinding{}
	for _, value := range s.channelBindings {
		if value.TenantID == tenantID && (projectID == "" || value.ProjectID == projectID) {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].CreatedAt.Before(values[j].CreatedAt) })
	return values, nil
}

func (s *Store) ChannelBinding(_ context.Context, tenantID, id string) (deliverydomain.ChannelBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.channelBindings[id]
	if !ok || value.TenantID != tenantID {
		return value, fault.NotFound("渠道绑定")
	}
	return value, nil
}

func (s *Store) SaveChannelBinding(_ context.Context, value deliverydomain.ChannelBinding) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.channelBindings[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return fault.NotFound("渠道绑定")
	}
	s.channelBindings[value.ID] = value
	return nil
}

func (s *Store) CreateChannelPublication(_ context.Context, value deliverydomain.ChannelPublication) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, current := range s.channelPublications {
		if current.TenantID == value.TenantID && current.IdempotencyKey == value.IdempotencyKey {
			return fault.Conflict("CHANNEL_PUBLICATION_IDEMPOTENCY_CONFLICT", "渠道发布幂等键已存在")
		}
	}
	s.channelPublications[value.ID] = cloneChannelPublication(value)
	return nil
}

func (s *Store) ChannelPublicationByIdempotencyKey(_ context.Context, tenantID, key string) (deliverydomain.ChannelPublication, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.channelPublications {
		if value.TenantID == tenantID && value.IdempotencyKey == key {
			return cloneChannelPublication(value), nil
		}
	}
	return deliverydomain.ChannelPublication{}, fault.NotFound("渠道发布")
}

func (s *Store) ChannelPublications(_ context.Context, tenantID, taskID string) ([]deliverydomain.ChannelPublication, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := []deliverydomain.ChannelPublication{}
	for _, value := range s.channelPublications {
		if value.TenantID == tenantID && (taskID == "" || value.TaskID == taskID) {
			values = append(values, cloneChannelPublication(value))
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].CreatedAt.Before(values[j].CreatedAt) })
	return values, nil
}

func (s *Store) ChannelPublication(_ context.Context, tenantID, id string) (deliverydomain.ChannelPublication, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.channelPublications[id]
	if !ok || value.TenantID != tenantID {
		return value, fault.NotFound("渠道发布")
	}
	return cloneChannelPublication(value), nil
}

func (s *Store) SaveChannelPublication(_ context.Context, value deliverydomain.ChannelPublication) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.channelPublications[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return fault.NotFound("渠道发布")
	}
	s.channelPublications[value.ID] = cloneChannelPublication(value)
	return nil
}

func (s *Store) ApplyChannelCallback(_ context.Context, value deliverydomain.ChannelPublication, receipt deliverydomain.ChannelCallbackReceipt) (bool, error) {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return false, err
	}
	if err := receipt.Validate(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.channelPublications[value.ID]
	if !ok || current.TenantID != value.TenantID || receipt.PublicationID != value.ID || receipt.TenantID != value.TenantID {
		return false, fault.NotFound("渠道发布")
	}
	key := receipt.TenantID + ":" + receipt.AdapterID + ":" + receipt.EventID
	if _, exists := s.channelCallbackReceipts[key]; exists {
		return false, nil
	}
	s.channelCallbackReceipts[key] = receipt
	s.channelPublications[value.ID] = cloneChannelPublication(value)
	return true, nil
}
