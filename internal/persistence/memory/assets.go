package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

func (s *Store) CreateAsset(_ context.Context, value sourcedomain.Asset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets[value.ID] = value
	return nil
}

func (s *Store) Assets(_ context.Context, tenantID, projectID string) ([]sourcedomain.Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := []sourcedomain.Asset{}
	for _, value := range s.assets {
		if value.TenantID == tenantID && value.ProjectID == projectID {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) Asset(_ context.Context, tenantID, id string) (sourcedomain.Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.assets[id]
	if !ok || value.TenantID != tenantID {
		return value, fault.NotFound("素材")
	}
	return value, nil
}

func (s *Store) SaveAsset(_ context.Context, value sourcedomain.Asset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.assets[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return fault.NotFound("素材")
	}
	s.assets[value.ID] = value
	return nil
}

func (s *Store) CreateRightsRecord(_ context.Context, value sourcedomain.RightsRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rightsRecords[value.ID] = value
	return nil
}

func (s *Store) RightsRecords(_ context.Context, tenantID, assetID string) ([]sourcedomain.RightsRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := []sourcedomain.RightsRecord{}
	for _, value := range s.rightsRecords {
		if value.TenantID == tenantID && (assetID == "" || value.AssetID == assetID) {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) RightsRecord(_ context.Context, tenantID, id string) (sourcedomain.RightsRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.rightsRecords[id]
	if !ok || value.TenantID != tenantID {
		return value, fault.NotFound("权利记录")
	}
	return value, nil
}

func (s *Store) SaveRightsRecord(_ context.Context, value sourcedomain.RightsRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.rightsRecords[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return fault.NotFound("权利记录")
	}
	s.rightsRecords[value.ID] = value
	return nil
}
