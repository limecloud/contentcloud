package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateKnowledgeConflict(_ context.Context, conflict domain.KnowledgeConflict, request domain.DecisionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.knowledgeConflicts[conflict.ID] = conflict
	s.decisionRequests[request.ID] = request
	return nil
}

func (s *Store) KnowledgeConflicts(_ context.Context, tenantID, projectID string) ([]domain.KnowledgeConflict, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := []domain.KnowledgeConflict{}
	for _, value := range s.knowledgeConflicts {
		if value.TenantID == tenantID && value.ProjectID == projectID {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) KnowledgeConflict(_ context.Context, tenantID, id string) (domain.KnowledgeConflict, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.knowledgeConflicts[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("知识冲突")
	}
	return value, nil
}

func (s *Store) SaveKnowledgeConflict(_ context.Context, value domain.KnowledgeConflict) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.knowledgeConflicts[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("知识冲突")
	}
	s.knowledgeConflicts[value.ID] = value
	return nil
}

func (s *Store) DecisionRequests(_ context.Context, tenantID, projectID string) ([]domain.DecisionRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := []domain.DecisionRequest{}
	for _, value := range s.decisionRequests {
		if value.TenantID == tenantID && value.ProjectID == projectID {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) DecisionRequest(_ context.Context, tenantID, id string) (domain.DecisionRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.decisionRequests[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("决策请求")
	}
	return value, nil
}

func (s *Store) SaveDecisionRequest(_ context.Context, value domain.DecisionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.decisionRequests[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("决策请求")
	}
	s.decisionRequests[value.ID] = value
	return nil
}
