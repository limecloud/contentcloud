package memory

import (
	"context"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateRunAttempt(_ context.Context, attempt domain.RunAttempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runAttempts {
		if existing.RunID == attempt.RunID && (existing.State == "leased" || existing.State == "running") {
			return domain.Conflict("RUN_ATTEMPT_ACTIVE", "任务已有正在执行的尝试")
		}
	}
	s.runAttempts[attempt.ID] = attempt
	return nil
}

func (s *Store) RunAttempt(_ context.Context, tenantID, id string) (domain.RunAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	attempt, ok := s.runAttempts[id]
	if !ok || attempt.TenantID != tenantID {
		return attempt, domain.NotFound("任务执行尝试")
	}
	return attempt, nil
}

func (s *Store) RunAttempts(_ context.Context, tenantID, runID string) ([]domain.RunAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.RunAttempt{}
	for _, attempt := range s.runAttempts {
		if attempt.TenantID == tenantID && attempt.RunID == runID {
			attempt.TokenHash = ""
			result = append(result, attempt)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) SaveRunAttempt(_ context.Context, attempt domain.RunAttempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.runAttempts[attempt.ID]; !ok || existing.TenantID != attempt.TenantID {
		return domain.NotFound("任务执行尝试")
	}
	s.runAttempts[attempt.ID] = attempt
	return nil
}

func (s *Store) ExpireRunAttempts(_ context.Context, tenantID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, attempt := range s.runAttempts {
		if attempt.TenantID != tenantID || (attempt.State != "leased" && attempt.State != "running") || !attempt.LeaseExpiresAt.Before(now) {
			continue
		}
		finished := now
		attempt.State = "expired"
		attempt.FailureClass = "lease_expired"
		attempt.FinishedAt = &finished
		s.runAttempts[id] = attempt
		run := s.runs[attempt.RunID]
		if run.ActiveAttemptID != attempt.ID {
			continue
		}
		if run.AttemptCount >= 3 {
			run.State = "failed"
			run.ErrorCode = "RUN_ATTEMPTS_EXHAUSTED"
		} else {
			run.State = "queued"
			run.ErrorCode = ""
		}
		run.ActiveAttemptID = ""
		run.LeaseDeviceID = ""
		run.LeaseExpiresAt = nil
		run.RunTokenHash = ""
		run.UpdatedAt = now
		s.runs[run.ID] = run
	}
	return nil
}
