package runtime

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type Projector struct {
	repo Repository
	now  func() time.Time
}

func NewProjector(repo Repository, now func() time.Time) *Projector {
	if now == nil {
		now = time.Now
	}
	return &Projector{repo: repo, now: now}
}

type ProjectionRunResult struct {
	Claimed   int `json:"claimed"`
	Projected int `json:"projected"`
	Retried   int `json:"retried"`
}

// RunOnce claims a durable outbox batch, rebuilds the Runtime Explorer view
// from authoritative snapshots, and acknowledges only after persistence.
func (p *Projector) RunOnce(ctx context.Context, tenantID, worker string, leaseFor time.Duration, limit int) (ProjectionRunResult, error) {
	result := ProjectionRunResult{}
	if p == nil || p.repo == nil {
		return result, domain.Policy("RUNTIME_PROJECTOR_UNAVAILABLE", "Runtime 投影器未配置存储", "配置 Runtime Repository 后重试")
	}
	commands := p.repo
	if leaseFor <= 0 {
		leaseFor = time.Minute
	}
	if limit <= 0 {
		limit = 50
	}
	now := p.now().UTC()
	leaseOwner := strings.TrimSpace(worker)
	if leaseOwner == "" {
		return result, domain.Invalid("RUNTIME_PROJECTOR_WORKER_REQUIRED", "Runtime 投影器需要稳定的工作器身份")
	}
	messages, err := commands.ClaimRuntimeOutbox(ctx, tenantID, domain.RuntimeOutboxSubscriberProjection, leaseOwner, now, leaseFor, limit)
	if err != nil {
		return result, err
	}
	result.Claimed = len(messages)
	for _, message := range messages {
		jobID := message.AggregateID
		if jobID == "" {
			if value, ok := message.Payload["job_run_id"].(string); ok {
				jobID = value
			}
		}
		job, projectErr := p.repo.JobRun(ctx, tenantID, jobID)
		if projectErr == nil {
			var nodes []domain.NodeRun
			nodes, projectErr = p.repo.NodeRuns(ctx, tenantID, jobID)
			if projectErr == nil {
				lastSequence := int64(0)
				switch value := message.Payload["sequence"].(type) {
				case float64:
					lastSequence = int64(value)
				case int64:
					lastSequence = value
				case int:
					lastSequence = int64(value)
				}
				if lastSequence == 0 {
					if events, eventErr := p.repo.JobEvents(ctx, tenantID, jobID, 0); eventErr == nil && len(events) > 0 {
						lastSequence = events[len(events)-1].Sequence
					}
				}
				view := domain.RuntimeExplorerView{TenantID: tenantID, JobRunID: jobID, Job: job, Nodes: nodes, LastEventSeq: lastSequence, SourceEventID: message.EventID, ProjectedAt: now}
				projectErr = p.repo.SaveRuntimeExplorer(ctx, view)
			}
		}
		if projectErr != nil {
			// A newer event may have won a concurrent projection race. The
			// durable snapshot is already ahead, so the older outbox message is
			// idempotently complete and must not poison the retry queue.
			if hasDomainCode(projectErr, "RUNTIME_PROJECTION_STALE") {
				if err := commands.AckRuntimeOutbox(ctx, tenantID, message.ID, domain.RuntimeOutboxSubscriberProjection, leaseOwner, now); err != nil {
					return result, err
				}
				result.Projected++
				continue
			}
			result.Retried++
			retryAt := now.Add(time.Second)
			if retryErr := commands.RetryRuntimeOutbox(ctx, tenantID, message.ID, domain.RuntimeOutboxSubscriberProjection, leaseOwner, now, retryAt, projectErr.Error()); retryErr != nil {
				return result, retryErr
			}
			continue
		}
		if err := commands.AckRuntimeOutbox(ctx, tenantID, message.ID, domain.RuntimeOutboxSubscriberProjection, leaseOwner, now); err != nil {
			return result, err
		}
		result.Projected++
	}
	return result, nil
}

func (s *Service) RuntimeExplorer(ctx context.Context, tenantID, jobID string) (domain.RuntimeExplorerView, error) {
	return s.repo.RuntimeExplorer(ctx, tenantID, jobID)
}

func (s *Service) RuntimeProjectionStats(ctx context.Context, tenantID string) (domain.RuntimeProjectionStats, error) {
	return s.repo.RuntimeProjectionStats(ctx, tenantID)
}

func (s *Service) RuntimeOutboxStats(ctx context.Context, tenantID, subscriber string) (domain.RuntimeOutboxStats, error) {
	return s.repo.RuntimeOutboxStats(ctx, tenantID, subscriber)
}

func (s *Service) SaveRuntimeMaintenanceHeartbeat(ctx context.Context, heartbeat domain.RuntimeMaintenanceHeartbeat) error {
	return s.repo.SaveRuntimeMaintenanceHeartbeat(ctx, heartbeat)
}

func (s *Service) RuntimeMaintenanceHeartbeat(ctx context.Context, tenantID, kind string) (domain.RuntimeMaintenanceHeartbeat, error) {
	return s.repo.RuntimeMaintenanceHeartbeat(ctx, tenantID, kind)
}
