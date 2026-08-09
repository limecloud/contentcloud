package runtime

import (
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestRuntimeCommandWritesEventAndOutboxTogether(t *testing.T) {
	repo := memory.New()
	service := New(repo, func() time.Time { return time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC) })
	started, err := service.Start(t.Context(), testStartInput("task-1", ""))
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := service.Events(t.Context(), "tenant-1", started.Job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeOutbox, err := repo.RuntimeOutboxMessages(t.Context(), "tenant-1", time.Now(), 100)
	if err != nil {
		t.Fatal(err)
	}
	node, err := repo.NodeRun(t.Context(), "tenant-1", started.Nodes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.TransitionNode(t.Context(), "tenant-1", node.ID, domain.NodeReady, "runtime", "runtime", node.Version); err != nil {
		t.Fatal(err)
	}
	afterEvents, err := service.Events(t.Context(), "tenant-1", started.Job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	afterOutbox, err := repo.RuntimeOutboxMessages(t.Context(), "tenant-1", time.Now(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEvents)-len(beforeEvents) < 1 || len(afterEvents)-len(beforeEvents) != len(afterOutbox)-len(beforeOutbox) {
		t.Fatalf("command must append matching event and outbox rows: events %d->%d outbox %d->%d", len(beforeEvents), len(afterEvents), len(beforeOutbox), len(afterOutbox))
	}
}

func TestRuntimeStateMutationIsIdempotentAcrossEventAndOutbox(t *testing.T) {
	repo := memory.New()
	service := New(repo, time.Now)
	started, err := service.Start(t.Context(), testStartInput("task-1", ""))
	if err != nil {
		t.Fatal(err)
	}
	mutation := domain.StateMutation{Collection: "brief", ExpectedRevision: 0, Set: map[string]any{"topic": "春日"}, IdempotencyKey: "state-1"}
	first, err := service.MutateState(t.Context(), "tenant-1", started.Job.ID, mutation, "worker", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	eventsAfterFirst, _ := service.Events(t.Context(), "tenant-1", started.Job.ID, 0)
	outboxAfterFirst, _ := repo.RuntimeOutboxMessages(t.Context(), "tenant-1", time.Now(), 100)
	second, err := service.MutateState(t.Context(), "tenant-1", started.Job.ID, mutation, "worker", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	eventsAfterSecond, _ := service.Events(t.Context(), "tenant-1", started.Job.ID, 0)
	outboxAfterSecond, _ := repo.RuntimeOutboxMessages(t.Context(), "tenant-1", time.Now(), 100)
	if first.Revision != 1 || second.Revision != 1 || len(eventsAfterSecond) != len(eventsAfterFirst) || len(outboxAfterSecond) != len(outboxAfterFirst) {
		t.Fatal("repeating an idempotent state command must not append another event or outbox row")
	}
}

func TestRuntimeOutboxClaimAckAndRetryAreFenced(t *testing.T) {
	repo := memory.New()
	service := New(repo, func() time.Time { return time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC) })
	if _, err := service.Start(t.Context(), testStartInput("task-1", "")); err != nil {
		t.Fatal(err)
	}
	commands, ok := any(repo).(RuntimeCommandStore)
	if !ok {
		t.Fatal("memory store must implement RuntimeCommandStore")
	}
	now := time.Date(2026, 8, 8, 0, 1, 0, 0, time.UTC)
	claimed, err := commands.ClaimRuntimeOutbox(t.Context(), "tenant-1", "projector-a", now, time.Minute, 100)
	if err != nil || len(claimed) == 0 {
		t.Fatalf("expected claimed outbox messages, got %#v, err=%v", claimed, err)
	}
	if claimed[0].Attempts != 1 || claimed[0].LockedBy != "projector-a" || claimed[0].LockedUntil == nil {
		t.Fatalf("claim must persist consumer lease and attempt count: %#v", claimed[0])
	}
	if second, err := commands.ClaimRuntimeOutbox(t.Context(), "tenant-1", "projector-b", now, time.Minute, 100); err != nil || len(second) != 0 {
		t.Fatalf("a live outbox lease must exclude other consumers: %#v, err=%v", second, err)
	}
	pending, err := commands.RuntimeOutboxMessages(t.Context(), "tenant-1", now, 1000)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	for _, message := range pending {
		if _, exists := seen[message.ID]; exists {
			t.Fatalf("outbox batch rollback must not duplicate message %q", message.ID)
		}
		seen[message.ID] = struct{}{}
	}
	if err := commands.AckRuntimeOutbox(t.Context(), "tenant-1", claimed[0].ID, "projector-b", now.Add(10*time.Second)); err == nil {
		t.Fatal("wrong consumer must not acknowledge an outbox message")
	}
	if err := commands.RetryRuntimeOutbox(t.Context(), "tenant-1", claimed[0].ID, "projector-a", now.Add(10*time.Second), now.Add(30*time.Second), "projection timeout"); err != nil {
		t.Fatal(err)
	}
	retryReady, err := commands.ClaimRuntimeOutbox(t.Context(), "tenant-1", "projector-b", now.Add(30*time.Second), time.Minute, 100)
	if err != nil || len(retryReady) != 1 {
		t.Fatalf("retried message should be claimable by another consumer: %#v, err=%v", retryReady, err)
	}
	if retryReady[0].Attempts != 2 || retryReady[0].LastError != "projection timeout" {
		t.Fatalf("retry must preserve attempt history and error: %#v", retryReady[0])
	}
	if err := commands.AckRuntimeOutbox(t.Context(), "tenant-1", retryReady[0].ID, "projector-b", now.Add(40*time.Second)); err != nil {
		t.Fatal(err)
	}
	if pending, err := commands.RuntimeOutboxMessages(t.Context(), "tenant-1", now.Add(40*time.Second), 10); err != nil || len(pending) != 0 {
		t.Fatalf("acknowledged message must leave the pending queue: %#v, err=%v", pending, err)
	}
}

func TestRuntimeEventContractRejectsMissingTypeInMemory(t *testing.T) {
	repo := memory.New()
	service := New(repo, time.Now)
	started, err := service.Start(t.Context(), testStartInput("task-1", ""))
	if err != nil {
		t.Fatal(err)
	}
	commands, ok := any(repo).(RuntimeCommandStore)
	if !ok {
		t.Fatal("memory store must implement RuntimeCommandStore")
	}
	if _, err := commands.AppendRuntimeEvent(t.Context(), domain.JobEvent{ID: domain.NewID(), TenantID: "tenant-1", JobRunID: started.Job.ID, ActorType: "test", OccurredAt: time.Now()}); err == nil {
		t.Fatal("events without a type must be rejected")
	}
}
