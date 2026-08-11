package connector

import (
	"context"
	"testing"
	"time"
)

type fakeAdapter struct{ result PullResult }

func (f fakeAdapter) Pull(context.Context, PullRequest) (PullResult, error) { return f.result, nil }

func TestEngineNormalizesIncrementalRecordsAndTombstones(t *testing.T) {
	now := time.Now().UTC()
	deletedAt := now.Add(-time.Minute)
	engine := New(fakeAdapter{result: PullResult{NextCursor: "cursor-2", Records: []Record{{ExternalID: "article-1", Version: "v2", MIME: "text/html", Body: []byte("<p>body</p>"), UpdatedAt: now}, {ExternalID: "article-2", Version: "v3", Deleted: true, DeletedAt: &deletedAt, UpdatedAt: now}}}})
	receipt, err := engine.Sync(t.Context(), PullRequest{Binding: Binding{ID: "binding-1", TenantID: "tenant-1", ProjectID: "project-1", ConnectorID: "cms", AuthorizationRef: "secret://cms", Status: BindingActive}, Cursor: "cursor-1", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.UpsertCount != 1 || receipt.TombstoneCount != 1 || receipt.Records[0].Digest == "" || receipt.Digest == "" {
		t.Fatalf("unexpected sync receipt: %#v", receipt)
	}
}

func TestEngineRejectsAmbiguousVersions(t *testing.T) {
	now := time.Now().UTC()
	engine := New(fakeAdapter{result: PullResult{Records: []Record{{ExternalID: "same", Version: "v1", MIME: "text/plain", Body: []byte("one"), UpdatedAt: now}, {ExternalID: "same", Version: "v2", MIME: "text/plain", Body: []byte("two"), UpdatedAt: now}}}})
	if _, err := engine.Sync(t.Context(), PullRequest{Binding: Binding{ID: "binding-1", TenantID: "tenant-1", ProjectID: "project-1", ConnectorID: "cms", AuthorizationRef: "secret://cms", Status: BindingActive}, Limit: 100}); err == nil {
		t.Fatal("ambiguous versions must fail closed")
	}
}
