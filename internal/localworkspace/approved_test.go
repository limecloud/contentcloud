package localworkspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestApprovedSnapshotCacheKeepsImmutableVersions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 7, 27, 14, 0, 0, 0, time.UTC)
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	first := approvedSnapshotFixture(t, "snapshot-1", "revision-1", "fact-1", now)
	second := approvedSnapshotFixture(t, "snapshot-2", "revision-2", "fact-2", now.Add(time.Minute))
	firstRecord, err := StoreApprovedSnapshot(root, first, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StoreApprovedSnapshot(root, second, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	items, err := ApprovedSnapshotInbox(root, "knowledge")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != second.ID || items[1].ID != first.ID || items[0].SubmissionRevisionID == items[1].SubmissionRevisionID {
		t.Fatalf("unexpected immutable snapshot versions: %#v", items)
	}
	shown, err := ShowApprovedSnapshot(root, first.ID)
	if err != nil || shown.Summary.SHA256 != firstRecord.Summary.SHA256 || shown.Snapshot.SubmissionRevisionID != "revision-1" {
		t.Fatalf("unexpected snapshot show: %#v err=%v", shown, err)
	}
	for _, name := range []string{"snapshot.json", "snapshot.sha256"} {
		info, err := os.Stat(filepath.Join(root, ".contentcloud", "cache", "approved", first.ID, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o222 != 0 {
			t.Fatalf("approved cache file is writable: %s mode=%o", name, info.Mode().Perm())
		}
	}

	changed := first
	changed.SubmissionRevisionID = "revision-replaced"
	if _, err := StoreApprovedSnapshot(root, changed, now.Add(2*time.Minute)); err == nil {
		t.Fatal("same snapshot ID with changed content overwrote an immutable cache entry")
	}
	shown, err = ShowApprovedSnapshot(root, first.ID)
	if err != nil || shown.Snapshot.SubmissionRevisionID != "revision-1" {
		t.Fatalf("immutable snapshot changed after conflict: %#v err=%v", shown, err)
	}
}

func TestApprovedSnapshotCacheRejectsTamperingAndUnverifiedLegacyEntry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 7, 27, 14, 0, 0, 0, time.UTC)
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	snapshot := approvedSnapshotFixture(t, "snapshot-verified", "revision-1", "fact-1", now)
	if _, err := StoreApprovedSnapshot(root, snapshot, now); err != nil {
		t.Fatal(err)
	}
	path := approvedSnapshotPath(root, snapshot.ID)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot.SubmissionRevisionID = "revision-tampered"
	body, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	_, err = ShowApprovedSnapshot(root, snapshot.ID)
	assertApprovedErrorCode(t, err, "APPROVED_SNAPSHOT_DIGEST_MISMATCH")

	legacy := approvedSnapshotFixture(t, "snapshot-legacy", "revision-legacy", "fact-legacy", now.Add(time.Minute))
	if _, err := StorePulledBundle(root, "approved", legacy.ID, legacy, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	_, err = ShowApprovedSnapshot(root, legacy.ID)
	assertApprovedErrorCode(t, err, "APPROVED_SNAPSHOT_DIGEST_MISSING")
}

func approvedSnapshotFixture(t *testing.T, snapshotID, revisionID, objectID string, createdAt time.Time) domain.ApprovedSnapshot {
	t.Helper()
	canonical, err := json.Marshal(map[string]any{
		"schema_version":  "contentcloud.knowledge/2.0",
		"submission_type": "knowledge",
		"objects":         []map[string]any{{"id": objectID, "kind": "fact", "status": "approved"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return domain.ApprovedSnapshot{
		ID: snapshotID, ProjectID: "project-1", WorkspaceID: "workspace-1", SubmissionID: "submission-1", SubmissionRevisionID: revisionID,
		SubmissionType: "knowledge", SchemaVersion: "contentcloud.knowledge/2.0", ContentHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SubjectHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CanonicalContent: canonical, EligibleIDs: []string{objectID}, CreatedAt: createdAt,
	}
}

func assertApprovedErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}
