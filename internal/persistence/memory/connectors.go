package memory

import (
	"context"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/integration/connector"
)

func connectorBindingKey(tenantID, id string) string { return tenantID + ":" + id }
func connectorRecordKey(tenantID, bindingID, externalID string) string {
	return tenantID + ":" + bindingID + ":" + externalID
}

func (s *Store) CreateBinding(_ context.Context, value connector.Binding) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := connectorBindingKey(value.TenantID, value.ID)
	if _, exists := s.connectorBindings[key]; exists {
		return fault.Conflict("CONNECTOR_BINDING_EXISTS", "Connector 绑定已存在")
	}
	for _, current := range s.connectorBindings {
		if current.TenantID == value.TenantID && current.ProjectID == value.ProjectID && current.ConnectorID == value.ConnectorID && current.Region == value.Region {
			return fault.Conflict("CONNECTOR_BINDING_EXISTS", "项目中已存在相同 Connector 和区域的绑定")
		}
	}
	s.connectorBindings[key] = value
	return nil
}

func (s *Store) Bindings(_ context.Context, tenantID, projectID string) ([]connector.Binding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := []connector.Binding{}
	for _, value := range s.connectorBindings {
		if value.TenantID == tenantID && (projectID == "" || value.ProjectID == projectID) {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].CreatedAt.Before(values[j].CreatedAt) })
	return values, nil
}

func (s *Store) Binding(_ context.Context, tenantID, id string) (connector.Binding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.connectorBindings[connectorBindingKey(tenantID, id)]
	if !ok {
		return value, fault.NotFound("Connector 绑定")
	}
	return value, nil
}

func (s *Store) AcquireSyncLease(_ context.Context, tenantID, bindingID string, lease connector.SyncLease) error {
	if lease.Owner == "" || !lease.ExpiresAt.After(time.Now().UTC()) {
		return fault.Invalid("CONNECTOR_SYNC_LEASE_INVALID", "Connector 同步租约缺少所有者或有效期")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := connectorBindingKey(tenantID, bindingID)
	if _, exists := s.connectorBindings[key]; !exists {
		return fault.NotFound("Connector 绑定")
	}
	if current, exists := s.connectorSyncLeases[key]; exists && current.Owner != lease.Owner && current.ExpiresAt.After(time.Now().UTC()) {
		return fault.Conflict("CONNECTOR_SYNC_IN_PROGRESS", "Connector 绑定正在同步")
	}
	s.connectorSyncLeases[key] = lease
	return nil
}

func (s *Store) ReleaseSyncLease(_ context.Context, tenantID, bindingID, owner string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := connectorBindingKey(tenantID, bindingID)
	if current, exists := s.connectorSyncLeases[key]; exists && current.Owner == owner {
		delete(s.connectorSyncLeases, key)
	}
	return nil
}

func (s *Store) Record(_ context.Context, tenantID, bindingID, externalID string) (connector.RecordMapping, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.connectorRecords[connectorRecordKey(tenantID, bindingID, externalID)]
	if !ok {
		return value, fault.NotFound("Connector 记录映射")
	}
	value.NormalizeCollections()
	return value, nil
}

func (s *Store) SaveRecord(_ context.Context, value connector.RecordMapping) error {
	value.NormalizeCollections()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.connectorBindings[connectorBindingKey(value.TenantID, value.BindingID)]; !exists {
		return fault.NotFound("Connector 绑定")
	}
	s.connectorRecords[connectorRecordKey(value.TenantID, value.BindingID, value.ExternalID)] = value
	return nil
}

func (s *Store) ActiveRecordsForSource(_ context.Context, tenantID, sourceID string) ([]connector.RecordMapping, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := []connector.RecordMapping{}
	for _, value := range s.connectorRecords {
		if value.TenantID == tenantID && value.SourceID == sourceID && !value.Deleted {
			value.NormalizeCollections()
			values = append(values, value)
		}
	}
	return values, nil
}

func (s *Store) CommitReceipt(_ context.Context, binding connector.Binding, expectedCursor, leaseOwner string, receipt connector.SyncReceipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := connectorBindingKey(binding.TenantID, binding.ID)
	current, exists := s.connectorBindings[key]
	if !exists {
		return fault.NotFound("Connector 绑定")
	}
	if current.Cursor != expectedCursor {
		return fault.Conflict("CONNECTOR_CURSOR_CONFLICT", "Connector 游标已被另一同步推进")
	}
	lease, leased := s.connectorSyncLeases[key]
	if !leased || lease.Owner != leaseOwner || !lease.ExpiresAt.After(time.Now().UTC()) {
		return fault.Conflict("CONNECTOR_SYNC_LEASE_LOST", "Connector 同步租约已失效")
	}
	current.Cursor, current.UpdatedAt = receipt.NextCursor, receipt.ObservedAt
	s.connectorBindings[key] = current
	s.connectorReceipts[connectorBindingKey(receipt.TenantID, receipt.ID)] = receipt
	delete(s.connectorSyncLeases, key)
	return nil
}

func (s *Store) Receipts(_ context.Context, tenantID, bindingID string) ([]connector.SyncReceipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := []connector.SyncReceipt{}
	for _, value := range s.connectorReceipts {
		if value.TenantID == tenantID && (bindingID == "" || value.BindingID == bindingID) {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ObservedAt.After(values[j].ObservedAt) })
	return values, nil
}
