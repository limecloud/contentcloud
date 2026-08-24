package postgres_test

import (
	"context"
	"os"
	"testing"

	workbenchdomain "github.com/limecloud/contentcloud/internal/experience/workbench"
	storepg "github.com/limecloud/contentcloud/internal/persistence/postgres"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
)

func TestWorkbenchRegistryLifecycleWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("CONTENTCLOUD_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTENTCLOUD_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := storepg.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	manifest := workbenchdomain.DefaultRegistry().Entries()[0].Manifest
	manifest.ID = "integration-workbench-" + idgen.New()
	manifest.Version = "9.0.0"
	entry := workbenchdomain.Entry{Manifest: manifest, Status: "draft", TenantIDs: []string{"tenant-integration"}}
	if err := store.CreateWorkbenchEntry(ctx, entry); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.WorkbenchEntry(ctx, manifest.ID, manifest.Version)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "draft" || loaded.Digest == "" || len(loaded.TenantIDs) != 1 {
		t.Fatalf("unexpected persisted workbench entry: %#v", loaded)
	}
	updated, err := store.UpdateWorkbenchEntryState(ctx, manifest.ID, manifest.Version, "draft", "published", []string{"tenant-published"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "published" || updated.TenantIDs[0] != "tenant-published" {
		t.Fatalf("unexpected updated workbench entry: %#v", updated)
	}
	revoked, err := store.UpdateWorkbenchEntryState(ctx, manifest.ID, manifest.Version, "published", "revoked", updated.TenantIDs, "integration signature failure")
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Status != "revoked" || revoked.LifecycleReason != "integration signature failure" {
		t.Fatalf("unexpected revoked workbench entry: %#v", revoked)
	}
	if _, err := store.UpdateWorkbenchEntryState(ctx, manifest.ID, manifest.Version, "revoked", "revoked", revoked.TenantIDs, "rewritten reason"); err == nil {
		t.Fatal("revoked workbench reason was overwritten")
	}
	if _, err := store.WorkbenchEntry(ctx, manifest.ID, "404.0.0"); err == nil {
		t.Fatal("missing workbench version returned nil error")
	}
}
