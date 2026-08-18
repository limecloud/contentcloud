package localsync

import (
	"context"
	"time"
)

type CloudPublisher interface {
	PublishWorkspace(context.Context, PendingCommand) (CloudRevision, error)
}

type CloudEventReader interface {
	WorkspaceEvents(context.Context, string, string, int64) (CloudEvents, error)
}

type PublishError struct {
	Code      string
	Retryable bool
	Conflict  bool
}

func (e *PublishError) Error() string { return e.Code }

type Processor struct {
	Store      *Store
	Publisher  CloudPublisher
	WorkerID   string
	DeviceID   string
	ProjectIDs []string
	Now        func() time.Time
}

func (p *Processor) RunOnce(ctx context.Context) (bool, error) {
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	startedAt := now().UTC()
	command, claimed, err := p.Store.ClaimPublish(ctx, p.WorkerID, p.DeviceID, p.ProjectIDs, startedAt, 30*time.Second)
	if err != nil || !claimed {
		return claimed, err
	}
	revision, publishErr := p.Publisher.PublishWorkspace(ctx, command)
	completedAt := now().UTC()
	if publishErr == nil {
		return true, p.Store.CompletePublish(ctx, command.CommandID, p.WorkerID, revision, completedAt)
	}
	classified, ok := publishErr.(*PublishError)
	if !ok {
		classified = &PublishError{Code: "NETWORK_ERROR", Retryable: true}
	}
	retryAt := completedAt.Add(retryDelay(command.Attempts))
	return true, p.Store.FailPublish(ctx, command.CommandID, p.WorkerID, classified.Code, classified.Retryable, classified.Conflict, retryAt, completedAt)
}

func (p *Processor) ReconcileWorkspace(ctx context.Context, workspaceID, projectID string) (bool, error) {
	reader, ok := p.Publisher.(CloudEventReader)
	if !ok {
		return false, nil
	}
	state, err := p.Store.ProjectState(ctx, projectID)
	if err != nil {
		return false, err
	}
	page, err := reader.WorkspaceEvents(ctx, workspaceID, projectID, state.CloudCursor)
	if err != nil {
		return false, err
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	if page.ResyncRequired {
		return true, p.Store.RequireCloudResync(ctx, projectID, "CLOUD_EVENT_CURSOR_GAP", now().UTC())
	}
	if len(page.Events) == 0 {
		return false, nil
	}
	return true, p.Store.ApplyCloudEvents(ctx, projectID, workspaceID, page.Events, now().UTC())
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second << min(attempt-1, 6)
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}
