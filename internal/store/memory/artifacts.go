package memory

import (
	"context"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateArtifactOpenRequest(_ context.Context, value domain.ArtifactOpenRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact, artifactExists := s.artifacts[value.ArtifactID]
	device, deviceExists := s.devices[value.DeviceID]
	if !artifactExists || !deviceExists || artifact.TenantID != value.TenantID || device.TenantID != value.TenantID || artifact.ProjectID != value.ProjectID {
		return domain.NotFound("Artifact 打开目标")
	}
	s.artifactOpenRequests[value.ID] = value
	return nil
}

func (s *Store) ArtifactOpenRequest(_ context.Context, tenantID, id string) (domain.ArtifactOpenRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.artifactOpenRequests[id]
	if !exists || value.TenantID != tenantID {
		return value, domain.NotFound("Artifact 打开请求")
	}
	return value, nil
}

func (s *Store) PendingArtifactOpenRequests(_ context.Context, tenantID, deviceID string, now time.Time, limit int) ([]domain.ArtifactOpenRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	values := []domain.ArtifactOpenRequest{}
	for _, value := range s.artifactOpenRequests {
		if value.TenantID == tenantID && value.DeviceID == deviceID && value.State == "pending" && now.Before(value.ExpiresAt) {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].CreatedAt.Before(values[j].CreatedAt) })
	if len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}

func (s *Store) SaveArtifactOpenRequest(_ context.Context, value domain.ArtifactOpenRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, exists := s.artifactOpenRequests[value.ID]
	if !exists || existing.TenantID != value.TenantID {
		return domain.NotFound("Artifact 打开请求")
	}
	s.artifactOpenRequests[value.ID] = value
	return nil
}

func (s *Store) ExpireArtifactOpenRequests(_ context.Context, tenantID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, value := range s.artifactOpenRequests {
		if value.TenantID == tenantID && (value.State == "pending" || value.State == "accepted") && !now.Before(value.ExpiresAt) {
			value.State = "expired"
			value.CompletedAt = &now
			s.artifactOpenRequests[id] = value
		}
	}
	return nil
}
