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
	FanoutSet(context.Context, string, string) (domain.FanoutSet, error)
	FanoutSetByIdempotencyKey(context.Context, string, string, string) (domain.FanoutSet, error)
	FanoutSets(context.Context, string, string) ([]domain.FanoutSet, error)
	FanoutMembers(context.Context, string, string) ([]domain.FanoutMember, error)

	CreateJobBundle(context.Context, domain.JobRun, []domain.NodeRun, domain.JobEvent) error
	JobRun(context.Context, string, string) (domain.JobRun, error)
	JobRunByIdempotencyKey(context.Context, string, string) (domain.JobRun, error)
	JobRuns(context.Context, string, string) ([]domain.JobRun, error)
	NodeRuns(context.Context, string, string) ([]domain.NodeRun, error)
	NodeRun(context.Context, string, string) (domain.NodeRun, error)
	NextReadyNode(context.Context, string, string) (domain.NodeRun, error)

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
	RuntimeYield(context.Context, string, string) (domain.RuntimeYield, error)
	RuntimeYields(context.Context, string, string) ([]domain.RuntimeYield, error)
	// PrepareDispatch atomically leases the node, creates its immutable attempt
	// context, creates or rebinds the logical agent, persists the attempt and
	// appends the corresponding event.
	PrepareDispatch(context.Context, domain.NodeRun, int, domain.RuntimeAttempt, domain.ContextView, domain.AgentInstance, bool, int, []domain.ResourceReservation, domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error)
	// ActivateDispatch atomically binds the external harness session and moves
	// NodeRun, RuntimeAttempt and AgentInstance into their active states.
	ActivateDispatch(context.Context, domain.NodeRun, int, domain.RuntimeAttempt, int, domain.AgentInstance, int, domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error)
	HeartbeatDispatch(context.Context, string, string, string, string, int, int, time.Time, time.Duration) (domain.NodeRun, domain.RuntimeAttempt, error)
	// FinalizeDispatch is idempotent for an identical terminal result digest and
	// rejects conflicting terminal reports.
	FinalizeDispatch(context.Context, domain.NodeRun, int, domain.RuntimeAttempt, int, domain.AgentInstance, int, string, domain.JobEvent) (domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error)
	YieldDispatch(context.Context, domain.RuntimeYield, domain.NodeRun, int, domain.RuntimeAttempt, int, domain.AgentInstance, int, string, domain.JobEvent) (domain.RuntimeYield, domain.NodeRun, domain.RuntimeAttempt, domain.AgentInstance, error)
	ResolveRuntimeYield(context.Context, domain.RuntimeYield, int, domain.NodeRun, int, domain.AgentInstance, int, domain.JobEvent) (domain.RuntimeYield, domain.NodeRun, domain.AgentInstance, error)

	AppendRuntimeEvent(context.Context, domain.JobEvent) (domain.JobEvent, error)
	JobEvents(context.Context, string, string, int64) ([]domain.JobEvent, error)

	RuntimeState(context.Context, string, string, string) (domain.RuntimeState, error)
	CreateStateCollection(context.Context, domain.StateCollection) error
	StateCollection(context.Context, string, string) (domain.StateCollection, error)
	StateCollections(context.Context, string, string) ([]domain.StateCollection, error)
	StateRecord(context.Context, string, string) (domain.StateRecord, error)
	StateRecords(context.Context, string, string) ([]domain.StateRecord, error)
	ApplyStateRecordCAS(context.Context, domain.StateRecord, int) (domain.StateRecord, error)
	CreateToolCall(context.Context, domain.ToolCall) error
	ToolCall(context.Context, string, string) (domain.ToolCall, error)
	ToolCalls(context.Context, string, string) ([]domain.ToolCall, error)
	ApplyToolCallTransition(context.Context, domain.ToolCall, int) (domain.ToolCall, error)

	CreateCheckpoint(context.Context, domain.Checkpoint) error
	Checkpoint(context.Context, string, string) (domain.Checkpoint, error)
	Checkpoints(context.Context, string, string) ([]domain.Checkpoint, error)

	Effect(context.Context, string, string) (domain.ExternalEffect, error)
	EffectByIdempotencyKey(context.Context, string, string) (domain.ExternalEffect, error)
	EffectByExternalID(context.Context, string, string, string) (domain.ExternalEffect, error)
	Effects(context.Context, string, string) ([]domain.ExternalEffect, error)
	ProviderInboxMessage(context.Context, string, string) (domain.ProviderInboxMessage, error)
	ProviderInboxMessages(context.Context, string, string) ([]domain.ProviderInboxMessage, error)
	ProviderReconciliation(context.Context, string, string) (domain.ProviderReconciliation, error)
	ProviderReconciliations(context.Context, string, string) ([]domain.ProviderReconciliation, error)
	ProviderBillRecords(context.Context, string, string) ([]domain.ProviderBillRecord, error)

	// Runtime repositories can use this hook to expire leases without exposing
	// SQL or queue internals to the service layer.
	ExpireNodeLeases(context.Context, string, time.Time) error
	SaveResourceQuota(context.Context, domain.ResourceQuota) error
	ResourceQuotas(context.Context, string) ([]domain.ResourceQuota, error)
	ResourceReservations(context.Context, string, string) ([]domain.ResourceReservation, error)
	SaveRuntimeExplorer(context.Context, domain.RuntimeExplorerView) error
	RuntimeExplorer(context.Context, string, string) (domain.RuntimeExplorerView, error)
	RuntimeProjectionStats(context.Context, string) (domain.RuntimeProjectionStats, error)
	CreateRuntimeProjectionRebuild(context.Context, domain.RuntimeProjectionRebuildRun) error
	UpdateRuntimeProjectionRebuild(context.Context, domain.RuntimeProjectionRebuildRun, int) error
	RuntimeProjectionRebuilds(context.Context, string, string) ([]domain.RuntimeProjectionRebuildRun, error)
}
