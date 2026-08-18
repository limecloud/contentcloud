package localsync

import (
	"context"
	"testing"
	"time"
)

type stubPublisher struct {
	revision CloudRevision
	events   CloudEvents
	err      error
	calls    int
}

func (p *stubPublisher) WorkspaceEvents(_ context.Context, _, _ string, _ int64) (CloudEvents, error) {
	return p.events, p.err
}

func (p *stubPublisher) PublishWorkspace(_ context.Context, _ PendingCommand) (CloudRevision, error) {
	p.calls++
	return p.revision, p.err
}

func TestProcessorReconcilesProjectScopedCloudCursorAndRemoteConflict(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 8, 18, 16, 0, 0, 0, time.UTC)
	localDigest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	remoteDigest := "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	_, _ = store.ObserveProject(t.Context(), "project-1", "workspace-1", localDigest, now)
	publisher := &stubPublisher{events: CloudEvents{NextCursor: 1, Events: []CloudRevisionEvent{{ID: "revision-1", ProjectID: "project-1", WorkspaceID: "workspace-1", RevisionNo: 1, ContentDigest: remoteDigest}}}}
	processor := Processor{Store: store, Publisher: publisher, WorkerID: "worker-1", ProjectIDs: []string{"project-1"}, Now: func() time.Time { return now.Add(time.Second) }}
	changed, err := processor.ReconcileWorkspace(t.Context(), "workspace-1", "project-1")
	if err != nil || !changed {
		t.Fatalf("reconcile changed=%v err=%v", changed, err)
	}
	state, err := store.ProjectState(t.Context(), "project-1")
	if err != nil || state.CloudCursor != 1 || state.CloudRevision != "revision-1" || state.ConflictCode != "WORKSPACE_REMOTE_CONTENT_PENDING" {
		t.Fatalf("unexpected remote projection: %#v err=%v", state, err)
	}
	publisher.events = CloudEvents{ResyncRequired: true}
	changed, err = processor.ReconcileWorkspace(t.Context(), "workspace-1", "project-1")
	if err != nil || !changed {
		t.Fatalf("gap reconcile changed=%v err=%v", changed, err)
	}
	state, _ = store.ProjectState(t.Context(), "project-1")
	if state.CloudCursor != 0 || state.ConflictCode != "CLOUD_EVENT_CURSOR_GAP" {
		t.Fatalf("gap did not reset scoped cursor: %#v", state)
	}
}

func TestProcessorCompletesDurablePublishAndAdvancesCloudRevision(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 8, 18, 14, 0, 0, 0, time.UTC)
	digest := "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if _, err := store.ObserveProject(t.Context(), "project-1", "workspace-1", digest, now); err != nil {
		t.Fatal(err)
	}
	command := PublishCommand{RequestID: "mutation-1", WorkspaceID: "workspace-1", ProjectID: "project-1", SubjectRef: "workspace", BaseRevision: "0", ObservedDigest: digest, IdempotencyKey: "idem-1", CreatedAt: now}
	if _, err := store.QueuePublish(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	publisher := &stubPublisher{revision: CloudRevision{ID: "revision-1", ContentDigest: digest}}
	processor := Processor{Store: store, Publisher: publisher, WorkerID: "worker-1", ProjectIDs: []string{"project-1"}, Now: func() time.Time { return now.Add(time.Second) }}
	worked, err := processor.RunOnce(t.Context())
	if err != nil || !worked || publisher.calls != 1 {
		t.Fatalf("processor worked=%v calls=%d err=%v", worked, publisher.calls, err)
	}
	state, err := store.ProjectState(t.Context(), "project-1")
	if err != nil || state.CloudRevision != "revision-1" || state.SyncedDigest != digest || state.TransferState != "synced" {
		t.Fatalf("unexpected synced state: %#v err=%v", state, err)
	}
	events, _, _, err := store.ListEvents(t.Context(), "project-1", 0, 100)
	if err != nil || events[len(events)-1].Type != "workspace.publish.synced" {
		t.Fatalf("sync event missing: %#v err=%v", events, err)
	}
}

func TestProcessorClassifiesRetryAndConflictWithoutLosingOutbox(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC)
	digest := "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	_, _ = store.ObserveProject(t.Context(), "project-1", "workspace-1", digest, now)
	_, _ = store.QueuePublish(t.Context(), PublishCommand{RequestID: "mutation-1", WorkspaceID: "workspace-1", ProjectID: "project-1", SubjectRef: "workspace", BaseRevision: "0", ObservedDigest: digest, IdempotencyKey: "idem-1", CreatedAt: now})
	publisher := &stubPublisher{err: &PublishError{Code: "UPSTREAM_UNAVAILABLE", Retryable: true}}
	processor := Processor{Store: store, Publisher: publisher, WorkerID: "worker-1", ProjectIDs: []string{"project-1"}, Now: func() time.Time { return now.Add(time.Second) }}
	if worked, err := processor.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("retry run worked=%v err=%v", worked, err)
	}
	state, _ := store.ProjectState(t.Context(), "project-1")
	if state.TransferState != "queued" {
		t.Fatalf("retryable command was not retained: %#v", state)
	}
	publisher.err = &PublishError{Code: "WORKSPACE_REVISION_STALE", Conflict: true}
	processor.Now = func() time.Time { return now.Add(4 * time.Second) }
	if worked, err := processor.RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("conflict run worked=%v err=%v", worked, err)
	}
	state, _ = store.ProjectState(t.Context(), "project-1")
	if state.TransferState != "failed" {
		t.Fatalf("conflict did not stop retry: %#v", state)
	}
}
