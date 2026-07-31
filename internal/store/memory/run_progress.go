package memory

import (
	"context"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) AppendRunProgress(_ context.Context, event domain.RunProgressEvent) (domain.RunProgressEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runProgress[event.RunID] {
		if existing.TenantID == event.TenantID && existing.AttemptID == event.AttemptID && existing.Sequence == event.Sequence {
			return existing, nil
		}
	}
	s.runProgressCursor++
	event.Cursor = s.runProgressCursor
	s.runProgress[event.RunID] = append(s.runProgress[event.RunID], event)
	return event, nil
}

func (s *Store) RunProgress(_ context.Context, tenantID, runID string, after int64) ([]domain.RunProgressEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if run, ok := s.runs[runID]; !ok || run.TenantID != tenantID {
		return nil, domain.NotFound("任务")
	}
	result := []domain.RunProgressEvent{}
	for _, event := range s.runProgress[runID] {
		if event.TenantID == tenantID && event.Cursor > after {
			result = append(result, event)
		}
	}
	return result, nil
}
