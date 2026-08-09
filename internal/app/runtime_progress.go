package app

import (
	"context"

	"github.com/limecloud/contentcloud/internal/domain"
)

// RunProgress is the stable read projection for the existing run-events
// surface. Runtime JobEvents are authoritative.
func (s *Service) RunProgress(ctx context.Context, actor Actor, runID string, after int64) ([]domain.RuntimeRunEvent, error) {
	job, ok, err := s.runtimeJob(ctx, actor.TenantID, runID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.NotFound("Runtime JobRun")
	}
	if after < 0 {
		after = 0
	}
	events, err := s.runtimeService.Events(ctx, actor.TenantID, job.ID, after)
	if err != nil {
		return nil, err
	}
	result := make([]domain.RuntimeRunEvent, 0, len(events))
	for _, event := range events {
		attemptID := ""
		if event.Payload != nil {
			if value, ok := event.Payload["attempt_id"].(string); ok {
				attemptID = value
			}
		}
		result = append(result, domain.RuntimeRunEvent{Cursor: event.Sequence, TenantID: event.TenantID, ProjectID: job.ProjectID, RunID: job.ID, AttemptID: attemptID, DeviceID: event.ActorID, Sequence: int(event.Sequence), Phase: event.Type, Step: int(event.Sequence), Label: event.Type, OccurredAt: event.OccurredAt})
	}
	return result, nil
}
