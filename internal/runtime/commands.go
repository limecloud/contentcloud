package runtime

import (
	"context"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

// RuntimeCommandStore owns all Runtime state changes that must publish an
// event. Implementations commit the snapshot, JobEvent and outbox row together.
type RuntimeCommandStore interface {
	AppendRuntimeEvent(context.Context, domain.JobEvent) (domain.JobEvent, error)
	ClaimReadyNodeCommand(context.Context, string, string, string, time.Time, time.Duration, domain.JobEvent) (domain.NodeRun, error)
	HeartbeatNodeCommand(context.Context, string, string, string, int, time.Time, time.Duration, domain.JobEvent) (domain.NodeRun, error)
	ApplyJobTransition(context.Context, domain.JobRun, int, domain.JobEvent) (domain.JobRun, error)
	ApplyGraphPatchCommand(context.Context, domain.JobRun, int, domain.JobPlanRevision, []domain.NodeRun, []string, domain.JobEvent) (domain.JobRun, error)
	CreateFanoutSetCommand(context.Context, domain.JobRun, int, domain.JobPlanRevision, domain.FanoutSet, []domain.FanoutMember, []domain.NodeRun, domain.JobEvent) (domain.JobRun, error)
	ApplyFanoutJoinCommand(context.Context, domain.FanoutSet, int, []domain.FanoutMember, []string, domain.JobEvent) (domain.FanoutSet, error)
	ApplyNodeTransition(context.Context, domain.NodeRun, int, domain.JobEvent) (domain.NodeRun, error)
	ApplyStateMutation(context.Context, string, string, domain.StateMutation, domain.JobEvent) (domain.RuntimeState, error)
	ApplyStateRecordCommand(context.Context, domain.StateRecord, int, domain.JobEvent) (domain.StateRecord, error)
	RegisterToolCallCommand(context.Context, domain.ToolCall, domain.JobEvent) (domain.ToolCall, error)
	ApplyToolCallTransitionCommand(context.Context, domain.ToolCall, int, domain.JobEvent) (domain.ToolCall, error)
	RegisterEffectCommand(context.Context, domain.ExternalEffect, domain.JobEvent) (domain.ExternalEffect, error)
	ApplyEffectTransition(context.Context, domain.ExternalEffect, int, domain.JobEvent) (domain.ExternalEffect, error)
	ReceiveProviderInboxCommand(context.Context, domain.ProviderInboxMessage, *domain.ExternalEffect, int, *domain.ProviderReconciliation, domain.JobEvent) (domain.ProviderInboxMessage, domain.ExternalEffect, error)
	RecordProviderBillCommand(context.Context, domain.ProviderBillRecord, *domain.ProviderReconciliation, domain.JobEvent) (domain.ProviderBillRecord, error)
	ResolveProviderReconciliationCommand(context.Context, domain.ProviderReconciliation, domain.ExternalEffect, int, domain.JobEvent) (domain.ProviderReconciliation, domain.ExternalEffect, error)
	RuntimeOutboxMessages(context.Context, string, time.Time, int) ([]domain.RuntimeOutboxMessage, error)
	ClaimRuntimeOutbox(context.Context, string, string, time.Time, time.Duration, int) ([]domain.RuntimeOutboxMessage, error)
	AckRuntimeOutbox(context.Context, string, string, string, time.Time) error
	RetryRuntimeOutbox(context.Context, string, string, string, time.Time, time.Time, string) error
}
