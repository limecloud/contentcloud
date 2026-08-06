package runtime

import (
	"context"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

// Repository is the small persistence boundary owned by the Runtime. It is
// deliberately separate from the broad business Store so Runtime state cannot
// become an accidental second source of content facts.
type Repository interface {
	CreatePlan(context.Context, domain.JobPlanRevision) error
	Plan(context.Context, string, string) (domain.JobPlanRevision, error)
	Plans(context.Context, string) ([]domain.JobPlanRevision, error)

	CreateJobBundle(context.Context, domain.JobRun, []domain.NodeRun, domain.JobEvent) error
	JobRun(context.Context, string, string) (domain.JobRun, error)
	JobRunByIdempotencyKey(context.Context, string, string) (domain.JobRun, error)
	JobRuns(context.Context, string, string) ([]domain.JobRun, error)
	SaveJobRun(context.Context, domain.JobRun, int) error
	NodeRuns(context.Context, string, string) ([]domain.NodeRun, error)
	NodeRun(context.Context, string, string) (domain.NodeRun, error)
	SaveNodeRun(context.Context, domain.NodeRun, int) error
	NextReadyNode(context.Context, string, string) (domain.NodeRun, error)
	// ClaimReadyNode atomically moves one ready node to leased. Implementations
	// must use a lock/conditional update so two workers cannot claim the same
	// node.
	ClaimReadyNode(context.Context, string, string, string, time.Time, time.Duration) (domain.NodeRun, error)
	// HeartbeatNode renews a lease only for its current owner and version. A
	// stale worker must receive a conflict instead of reviving the node.
	HeartbeatNode(context.Context, string, string, string, int, time.Time, time.Duration) (domain.NodeRun, error)

	CreateContextView(context.Context, domain.ContextView) error
	ContextView(context.Context, string, string) (domain.ContextView, error)
	ContextViews(context.Context, string, string) ([]domain.ContextView, error)
	CreateAgentInstance(context.Context, domain.AgentInstance) error
	AgentInstance(context.Context, string, string) (domain.AgentInstance, error)
	AgentInstances(context.Context, string, string) ([]domain.AgentInstance, error)
	SaveAgentInstance(context.Context, domain.AgentInstance, int) error
	AgentInstanceForNode(context.Context, string, string) (domain.AgentInstance, error)

	RuntimeAttempt(context.Context, string, string) (domain.RuntimeAttempt, error)
	RuntimeAttempts(context.Context, string, string) ([]domain.RuntimeAttempt, error)
	// PrepareDispatch atomically leases the node, creates its immutable attempt
	// context, creates or rebinds the logical agent, persists the attempt and
	// appends the corresponding event.
	PrepareDispatch(context.Context, domain.NodeRun, int, domain.RuntimeAttempt, domain.ContextView, domain.AgentInstance, bool, int, domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error)
	// ActivateDispatch atomically binds the external harness session and moves
	// NodeRun, RuntimeAttempt and AgentInstance into their active states.
	ActivateDispatch(context.Context, domain.NodeRun, int, domain.RuntimeAttempt, int, domain.AgentInstance, int, domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error)
	HeartbeatDispatch(context.Context, string, string, string, int, int, time.Time, time.Duration) (domain.NodeRun, domain.RuntimeAttempt, error)
	// FinalizeDispatch is idempotent for an identical terminal result digest and
	// rejects conflicting terminal reports.
	FinalizeDispatch(context.Context, domain.NodeRun, int, domain.RuntimeAttempt, int, domain.AgentInstance, int, domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error)

	AppendJobEvent(context.Context, domain.JobEvent) (domain.JobEvent, error)
	JobEvents(context.Context, string, string, int64) ([]domain.JobEvent, error)

	RuntimeState(context.Context, string, string, string) (domain.RuntimeState, error)
	SaveRuntimeStateCAS(context.Context, domain.RuntimeState, int, string) error

	CreateCheckpoint(context.Context, domain.Checkpoint) error
	Checkpoints(context.Context, string, string) ([]domain.Checkpoint, error)

	CreateEffect(context.Context, domain.ExternalEffect) error
	EffectByIdempotencyKey(context.Context, string, string) (domain.ExternalEffect, error)
	Effects(context.Context, string, string) ([]domain.ExternalEffect, error)
	SaveEffect(context.Context, domain.ExternalEffect, int) error

	// Runtime repositories can use this hook to expire leases without exposing
	// SQL or queue internals to the service layer.
	ExpireNodeLeases(context.Context, string, time.Time) error
}
