package runtime

import (
	"context"
	"time"
)

// Repository is the small persistence boundary owned by the Runtime. It is
// deliberately separate from the broad business Store so Runtime state cannot
// become an accidental second source of content facts.
type Repository interface {
	RuntimeCommandStore

	CreatePlan(context.Context, JobPlanRevision) error
	CreateExecutionBindingSnapshot(context.Context, ExecutionBindingSnapshot) error
	ExecutionBindingSnapshot(context.Context, string, string) (ExecutionBindingSnapshot, error)
	Plan(context.Context, string, string) (JobPlanRevision, error)
	Plans(context.Context, string) ([]JobPlanRevision, error)
	FanoutSet(context.Context, string, string) (FanoutSet, error)
	FanoutSetByIdempotencyKey(context.Context, string, string, string) (FanoutSet, error)
	FanoutSets(context.Context, string, string) ([]FanoutSet, error)
	FanoutMembers(context.Context, string, string) ([]FanoutMember, error)

	CreateJobBundle(context.Context, JobRun, []NodeRun, JobEvent) error
	JobRun(context.Context, string, string) (JobRun, error)
	JobRunByIdempotencyKey(context.Context, string, string) (JobRun, error)
	JobRuns(context.Context, string, string) ([]JobRun, error)
	JobRunsPage(context.Context, string, string, string, int, int) ([]JobRun, bool, error)
	NodeRuns(context.Context, string, string) ([]NodeRun, error)
	NodeRunsPage(context.Context, string, string, int, int) ([]NodeRun, bool, error)
	NodeRun(context.Context, string, string) (NodeRun, error)
	NextReadyNode(context.Context, string, string, []string) (NodeRun, error)

	CreateContextView(context.Context, ContextView) error
	ContextView(context.Context, string, string) (ContextView, error)
	ContextViews(context.Context, string, string) ([]ContextView, error)
	CreateAgentInstance(context.Context, AgentInstance) error
	AgentInstance(context.Context, string, string) (AgentInstance, error)
	AgentInstances(context.Context, string, string) ([]AgentInstance, error)
	SaveAgentInstance(context.Context, AgentInstance, int) error
	AgentInstanceForNode(context.Context, string, string) (AgentInstance, error)

	RuntimeAttempt(context.Context, string, string) (RuntimeAttempt, error)
	RuntimeAttemptByGatewayTokenHash(context.Context, string) (RuntimeAttempt, error)
	RuntimeAttempts(context.Context, string, string) ([]RuntimeAttempt, error)
	RuntimeYield(context.Context, string, string) (RuntimeYield, error)
	RuntimeYields(context.Context, string, string) ([]RuntimeYield, error)
	// PrepareDispatch atomically leases the node, creates its immutable attempt
	// context, creates or rebinds the logical agent, persists the attempt and
	// appends the corresponding event.
	PrepareDispatch(context.Context, NodeRun, int, RuntimeAttempt, ContextView, AgentInstance, bool, int, []ResourceReservation, JobEvent) (NodeRun, RuntimeAttempt, AgentInstance, error)
	// ActivateDispatch atomically binds the external harness session and moves
	// NodeRun, RuntimeAttempt and AgentInstance into their active states.
	ActivateDispatch(context.Context, NodeRun, int, RuntimeAttempt, int, AgentInstance, int, JobEvent) (NodeRun, RuntimeAttempt, AgentInstance, error)
	HeartbeatDispatch(context.Context, string, string, string, string, int, int, time.Time, time.Duration) (NodeRun, RuntimeAttempt, error)
	// FinalizeDispatch is idempotent for an identical terminal result digest and
	// rejects conflicting terminal reports.
	FinalizeDispatch(context.Context, NodeRun, int, RuntimeAttempt, int, AgentInstance, int, string, JobEvent) (NodeRun, RuntimeAttempt, AgentInstance, error)
	YieldDispatch(context.Context, RuntimeYield, NodeRun, int, RuntimeAttempt, int, AgentInstance, int, string, JobEvent) (RuntimeYield, NodeRun, RuntimeAttempt, AgentInstance, error)
	ResolveRuntimeYield(context.Context, RuntimeYield, int, NodeRun, int, AgentInstance, int, JobEvent) (RuntimeYield, NodeRun, AgentInstance, error)

	JobEvents(context.Context, string, string, int64) ([]JobEvent, error)
	JobEventsPage(context.Context, string, string, int64, int) ([]JobEvent, error)

	RuntimeState(context.Context, string, string, string) (RuntimeState, error)
	CreateStateCollection(context.Context, StateCollection) error
	StateCollection(context.Context, string, string) (StateCollection, error)
	StateCollections(context.Context, string, string) ([]StateCollection, error)
	StateRecord(context.Context, string, string) (StateRecord, error)
	StateRecords(context.Context, string, string) ([]StateRecord, error)
	ToolCall(context.Context, string, string) (ToolCall, error)
	ToolCallByIdempotencyKey(context.Context, string, string, string, string) (ToolCall, error)
	ToolCalls(context.Context, string, string) ([]ToolCall, error)

	CreateCheckpoint(context.Context, Checkpoint) error
	Checkpoint(context.Context, string, string) (Checkpoint, error)
	Checkpoints(context.Context, string, string) ([]Checkpoint, error)
	CheckpointsPage(context.Context, string, string, int, int) ([]Checkpoint, bool, error)

	Effect(context.Context, string, string) (ExternalEffect, error)
	EffectByIdempotencyKey(context.Context, string, string) (ExternalEffect, error)
	EffectByExternalID(context.Context, string, string, string) (ExternalEffect, error)
	Effects(context.Context, string, string) ([]ExternalEffect, error)
	EffectsPage(context.Context, string, string, int, int) ([]ExternalEffect, bool, error)
	ProviderInboxMessage(context.Context, string, string) (ProviderInboxMessage, error)
	ProviderInboxMessages(context.Context, string, string) ([]ProviderInboxMessage, error)
	ProviderReconciliation(context.Context, string, string) (ProviderReconciliation, error)
	ProviderReconciliations(context.Context, string, string) ([]ProviderReconciliation, error)
	ProviderBillRecords(context.Context, string, string) ([]ProviderBillRecord, error)

	CreateRuntimeSchema(context.Context, RuntimeSchema) error
	RuntimeSchema(context.Context, string, string, int) (RuntimeSchema, error)
	RuntimeSchemas(context.Context, string, string) ([]RuntimeSchema, error)
	PublishRuntimeSchema(context.Context, RuntimeSchema, int) (RuntimeSchema, error)
	RetireRuntimeSchema(context.Context, RuntimeSchema, int) (RuntimeSchema, error)

	// Runtime repositories can use this hook to expire leases without exposing
	// SQL or queue internals to the service layer.
	ExpireNodeLeases(context.Context, string, time.Time) error
	SaveResourceQuota(context.Context, ResourceQuota) error
	ResourceQuotas(context.Context, string) ([]ResourceQuota, error)
	ResourceReservations(context.Context, string, string) ([]ResourceReservation, error)
	SaveRuntimeExplorer(context.Context, RuntimeExplorerView) error
	RuntimeExplorer(context.Context, string, string) (RuntimeExplorerView, error)
	RuntimeOutboxStats(context.Context, string, string) (RuntimeOutboxStats, error)
	RuntimeProjectionStats(context.Context, string) (RuntimeProjectionStats, error)
	SaveRuntimeMaintenanceHeartbeat(context.Context, RuntimeMaintenanceHeartbeat) error
	RuntimeMaintenanceHeartbeat(context.Context, string, string) (RuntimeMaintenanceHeartbeat, error)
	CreateRuntimeProjectionRebuild(context.Context, RuntimeProjectionRebuildRun) error
	UpdateRuntimeProjectionRebuild(context.Context, RuntimeProjectionRebuildRun, int) error
	RuntimeProjectionRebuilds(context.Context, string, string) ([]RuntimeProjectionRebuildRun, error)
}
