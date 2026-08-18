package runtime

import (
	"context"
	"time"
)

// RuntimeCommandStore owns all Runtime state changes that must publish an
// event. Implementations commit the snapshot, JobEvent and outbox row together.
type RuntimeCommandStore interface {
	AppendRuntimeEvent(context.Context, JobEvent) (JobEvent, error)
	AppendFencedRuntimeEvent(context.Context, string, string, string, string, time.Time, JobEvent) (JobEvent, error)
	ClaimReadyNodeCommand(context.Context, string, string, string, time.Time, time.Duration, JobEvent) (NodeRun, error)
	HeartbeatNodeCommand(context.Context, string, string, string, int, time.Time, time.Duration, JobEvent) (NodeRun, error)
	ApplyJobTransition(context.Context, JobRun, int, JobEvent) (JobRun, error)
	ApplyGraphPatchCommand(context.Context, JobRun, int, JobPlanRevision, []NodeRun, []string, JobEvent) (JobRun, error)
	CreateFanoutSetCommand(context.Context, JobRun, int, JobPlanRevision, FanoutSet, []FanoutMember, []NodeRun, JobEvent) (JobRun, error)
	ApplyFanoutJoinCommand(context.Context, FanoutSet, int, []FanoutMember, []string, JobEvent) (FanoutSet, error)
	ApplyNodeTransition(context.Context, NodeRun, int, JobEvent) (NodeRun, error)
	ApplyStateMutation(context.Context, string, string, StateMutation, JobEvent) (RuntimeState, error)
	ApplyStateRecordCommand(context.Context, StateRecord, int, JobEvent) (StateRecord, error)
	ApplyFencedStateRecordCommand(context.Context, StateRecord, int, string, string, time.Time, JobEvent) (StateRecord, error)
	RegisterToolCallCommand(context.Context, ToolCall, JobEvent) (ToolCall, error)
	RegisterFencedToolCallCommand(context.Context, ToolCall, string, time.Time, JobEvent) (ToolCall, error)
	ApplyToolCallTransitionCommand(context.Context, ToolCall, int, JobEvent) (ToolCall, error)
	ApplyFencedToolCallTransitionCommand(context.Context, ToolCall, int, string, time.Time, JobEvent) (ToolCall, error)
	RegisterEffectCommand(context.Context, ExternalEffect, JobEvent) (ExternalEffect, error)
	RegisterFencedEffectCommand(context.Context, ExternalEffect, string, time.Time, JobEvent) (ExternalEffect, error)
	ApplyEffectTransition(context.Context, ExternalEffect, int, JobEvent) (ExternalEffect, error)
	ReceiveProviderInboxCommand(context.Context, ProviderInboxMessage, *ExternalEffect, int, *ProviderReconciliation, JobEvent) (ProviderInboxMessage, ExternalEffect, error)
	// Agent harness callbacks share the Provider inbox table, but have no
	// ExternalEffect or ProviderReconciliation. The two commands make the
	// durable-receive and post-processing boundary explicit.
	ReceiveAgentInboxCommand(context.Context, ProviderInboxMessage, JobEvent) (ProviderInboxMessage, error)
	CompleteAgentInboxCommand(context.Context, ProviderInboxMessage, int, JobEvent) (ProviderInboxMessage, error)
	RecordProviderBillCommand(context.Context, ProviderBillRecord, *ProviderReconciliation, JobEvent) (ProviderBillRecord, error)
	ResolveProviderReconciliationCommand(context.Context, ProviderReconciliation, ExternalEffect, int, JobEvent) (ProviderReconciliation, ExternalEffect, error)
	RuntimeOutboxMessages(context.Context, string, string, time.Time, int) ([]RuntimeOutboxMessage, error)
	ClaimRuntimeOutbox(context.Context, string, string, string, time.Time, time.Duration, int) ([]RuntimeOutboxMessage, error)
	AckRuntimeOutbox(context.Context, string, string, string, string, time.Time) error
	RetryRuntimeOutbox(context.Context, string, string, string, string, time.Time, time.Time, string) error
}
