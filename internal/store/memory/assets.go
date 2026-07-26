package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateAsset(_ context.Context, value domain.Asset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets[value.ID] = value
	return nil
}

func (s *Store) Assets(_ context.Context, tenantID, projectID string) ([]domain.Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := []domain.Asset{}
	for _, value := range s.assets {
		if value.TenantID == tenantID && value.ProjectID == projectID {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) Asset(_ context.Context, tenantID, id string) (domain.Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.assets[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("素材")
	}
	return value, nil
}

func (s *Store) SaveAsset(_ context.Context, value domain.Asset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.assets[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("素材")
	}
	s.assets[value.ID] = value
	return nil
}

func (s *Store) CreateRightsRecord(_ context.Context, value domain.RightsRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rightsRecords[value.ID] = value
	return nil
}

func (s *Store) RightsRecords(_ context.Context, tenantID, assetID string) ([]domain.RightsRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := []domain.RightsRecord{}
	for _, value := range s.rightsRecords {
		if value.TenantID == tenantID && (assetID == "" || value.AssetID == assetID) {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) RightsRecord(_ context.Context, tenantID, id string) (domain.RightsRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.rightsRecords[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("权利记录")
	}
	return value, nil
}

func (s *Store) SaveRightsRecord(_ context.Context, value domain.RightsRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.rightsRecords[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("权利记录")
	}
	s.rightsRecords[value.ID] = value
	return nil
}
