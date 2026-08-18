package localsync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
)

func TestStorePersistsObservationIdempotentPublishAndProjectEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	state, err := store.ObserveProject(t.Context(), "project-1", "workspace-1", digest, now)
	if err != nil || state.LocalRevision != 1 || state.EventCursor != 1 {
		t.Fatalf("unexpected observed state: %#v, err=%v", state, err)
	}
	command := PublishCommand{RequestID: "request-1", WorkspaceID: "workspace-1", ProjectID: "project-1", SubjectRef: "workspace", BaseRevision: "0", ObservedDigest: digest, IdempotencyKey: "publish-1", CreatedAt: now.Add(time.Second)}
	first, err := store.QueuePublish(t.Context(), command)
	if err != nil || first.State != "queued" || first.EventCursor != 2 {
		t.Fatalf("unexpected command result: %#v, err=%v", first, err)
	}
	second, err := store.QueuePublish(t.Context(), command)
	if err != nil || second.CommandID != first.CommandID || second.EventCursor != first.EventCursor {
		t.Fatalf("idempotent command changed: %#v, err=%v", second, err)
	}
	events, cursor, gap, err := store.ListEvents(t.Context(), "project-1", 0, 100)
	if err != nil || gap || cursor != 2 || len(events) != 2 || events[1].Type != "workspace.publish.queued" {
		t.Fatalf("unexpected events: %#v cursor=%d gap=%v err=%v", events, cursor, gap, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	pending, err := reopened.PendingCommands(t.Context(), "project-1")
	if err != nil || len(pending) != 1 || pending[0].CommandID != first.CommandID {
		t.Fatalf("outbox was not durable: %#v, err=%v", pending, err)
	}
}

func TestStoreRejectsStaleAndReusedIdempotencyInputs(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	digest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	_, err = store.ObserveProject(t.Context(), "project-1", "workspace-1", digest, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	base := PublishCommand{RequestID: "request-1", WorkspaceID: "workspace-1", ProjectID: "project-1", SubjectRef: "workspace", BaseRevision: "0", ObservedDigest: digest, IdempotencyKey: "same", CreatedAt: time.Now()}
	if _, err := store.QueuePublish(t.Context(), base); err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.SubjectRef = "workspace/changed"
	var conflict *ConflictError
	if _, err := store.QueuePublish(t.Context(), changed); !errors.As(err, &conflict) || conflict.Code != "IDEMPOTENCY_KEY_REUSED" {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	stale := base
	stale.IdempotencyKey = "stale"
	stale.ObservedDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if _, err := store.QueuePublish(t.Context(), stale); !errors.As(err, &conflict) || conflict.Code != "WORKSPACE_DIGEST_STALE" {
		t.Fatalf("expected digest conflict, got %v", err)
	}
}

func TestObserveWorkspaceChangesDigestWithoutPersistingContents(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", Target: "codex-plugin", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	first, err := ObserveWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "40-work", "draft.txt")
	if err := os.WriteFile(path, []byte("private body"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := ObserveWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest || second.FileCount != first.FileCount+1 {
		t.Fatalf("workspace observation did not change: %#v -> %#v", first, second)
	}
}

func TestListEventsReportsFutureCursorGap(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	digest := "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	_, _ = store.ObserveProject(context.Background(), "project-1", "workspace-1", digest, time.Now())
	events, cursor, gap, err := store.ListEvents(context.Background(), "project-1", 20, 10)
	if err != nil || !gap || len(events) != 0 || cursor != 1 {
		t.Fatalf("future cursor must require resync: events=%#v cursor=%d gap=%v err=%v", events, cursor, gap, err)
	}
}

func TestStorePersistsWorkspaceUploadTransferParts(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	digest := "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if _, err := store.ObserveProject(t.Context(), "project-1", "workspace-1", digest, time.Now()); err != nil {
		t.Fatal(err)
	}
	transfer := UploadTransfer{ProjectID: "project-1", Ref: "40-work/draft.md", ContentDigest: digest, ByteSize: 5, SessionID: "session-1", State: "uploading", ConfirmedParts: []int{0, 2}, UpdatedAt: time.Now()}
	if err := store.SaveUploadTransfer(t.Context(), transfer); err != nil {
		t.Fatal(err)
	}
	transfer.State, transfer.ConfirmedParts = "completed", []int{0, 1, 2}
	if err := store.SaveUploadTransfer(t.Context(), transfer); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.UploadTransfer(t.Context(), transfer.ProjectID, transfer.Ref, transfer.ContentDigest)
	if err != nil || loaded.State != "completed" || loaded.SessionID != transfer.SessionID || len(loaded.ConfirmedParts) != 3 {
		t.Fatalf("upload transfer = %#v err=%v", loaded, err)
	}
}
