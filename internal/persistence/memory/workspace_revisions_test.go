package memory

import (
	"errors"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestWorkspaceRevisionUsesBaseCASAndIdempotency(t *testing.T) {
	store := New()
	now := time.Date(2026, 8, 18, 13, 0, 0, 0, time.UTC)
	if err := store.CreateWorkspaceBinding(t.Context(), workspacedomain.WorkspaceBinding{ID: "workspace-1", TenantID: "tenant-1", ProjectID: "project-1", DeviceID: "device-1", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	first := workspacedomain.WorkspaceRevision{ID: "revision-1", TenantID: "tenant-1", ProjectID: "project-1", WorkspaceID: "workspace-1", DeviceID: "device-1", SchemaVersion: workspacedomain.WorkspaceRevisionSchemaVersion, BaseRevisionID: "0", ContentDigest: "sha256:" + repeatHex('a'), ClientMutationID: "mutation-1", IdempotencyKey: "idem-1", CreatedAt: now}
	created, err := store.PublishWorkspaceRevision(t.Context(), first)
	if err != nil || created.RevisionNo != 1 {
		t.Fatalf("first revision = %#v err=%v", created, err)
	}
	replayed, err := store.PublishWorkspaceRevision(t.Context(), first)
	if err != nil || replayed.ID != first.ID || replayed.RevisionNo != 1 {
		t.Fatalf("idempotent replay = %#v err=%v", replayed, err)
	}
	second := first
	second.ID, second.ContentDigest, second.IdempotencyKey, second.ClientMutationID, second.BaseRevisionID = "revision-2", "sha256:"+repeatHex('b'), "idem-2", "mutation-2", first.ID
	if created, err := store.PublishWorkspaceRevision(t.Context(), second); err != nil || created.RevisionNo != 2 {
		t.Fatalf("second revision = %#v err=%v", created, err)
	}
	stale := second
	stale.ID, stale.ContentDigest, stale.IdempotencyKey, stale.ClientMutationID = "revision-3", "sha256:"+repeatHex('c'), "idem-3", "mutation-3"
	var conflict *fault.Error
	if _, err := store.PublishWorkspaceRevision(t.Context(), stale); !errors.As(err, &conflict) || conflict.Code != "WORKSPACE_REVISION_STALE" {
		t.Fatalf("expected stale revision conflict, got %v", err)
	}
}

func repeatHex(value byte) string {
	result := make([]byte, 64)
	for index := range result {
		result[index] = value
	}
	return string(result)
}
