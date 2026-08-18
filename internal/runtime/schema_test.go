package runtime_test

import (
	"testing"
	"time"

	. "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
)

func TestRuntimeSchemaPublishCompatibilityAndRetentionLifecycle(t *testing.T) {
	repo := memory.New()
	service := New(repo, func() time.Time { return time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC) })
	draft, err := service.CreateRuntimeSchema(t.Context(), RuntimeSchemaInput{TenantID: "tenant-1", SchemaID: "contentcloud.test-state", Revision: 1, Definition: map[string]any{"type": "object", "properties": map[string]any{"topic": map[string]any{"type": "string"}}}, RetentionPolicy: "30d", CreatedBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Status != "draft" || draft.Digest == "" {
		t.Fatalf("schema draft was not recorded: %#v", draft)
	}
	published, err := service.PublishRuntimeSchema(t.Context(), "tenant-1", draft.SchemaID, 1, draft.Version)
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != "published" || published.PublishedAt == nil {
		t.Fatalf("schema was not published: %#v", published)
	}
	if _, err := service.PublishRuntimeSchema(t.Context(), "tenant-1", draft.SchemaID, 1, draft.Version); !hasDomainCode(err, "RUNTIME_SCHEMA_NOT_DRAFT") {
		t.Fatalf("published schema was accepted for a second publish: %v", err)
	}
	v2, err := service.CreateRuntimeSchema(t.Context(), RuntimeSchemaInput{TenantID: "tenant-1", SchemaID: draft.SchemaID, Revision: 2, Definition: draft.Definition, RetentionPolicy: "forever", CreatedBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishRuntimeSchema(t.Context(), "tenant-1", draft.SchemaID, 2, v2.Version); err != nil {
		t.Fatal(err)
	}
	v3, err := service.CreateRuntimeSchema(t.Context(), RuntimeSchemaInput{TenantID: "tenant-1", SchemaID: draft.SchemaID, Revision: 3, Definition: map[string]any{"type": "object", "properties": map[string]any{"required_new": map[string]any{"type": "string"}}, "required": []any{"required_new"}}, RetentionPolicy: "forever", CreatedBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishRuntimeSchema(t.Context(), "tenant-1", draft.SchemaID, 3, v3.Version); !hasDomainCode(err, "RUNTIME_SCHEMA_COMPATIBILITY_FAILED") {
		t.Fatalf("backward-incompatible schema was published: %v", err)
	}
	retired, err := service.RetireRuntimeSchema(t.Context(), "tenant-1", draft.SchemaID, 1, published.Version)
	if err != nil {
		t.Fatal(err)
	}
	if retired.Status != "retired" || retired.RetiredAt == nil || retired.RetainUntil == nil || retired.RetainUntil.Sub(*retired.RetiredAt) != 30*24*time.Hour {
		t.Fatalf("schema was not retired: %#v", retired)
	}
	all, err := service.RuntimeSchemas(t.Context(), "tenant-1", draft.SchemaID)
	if err != nil || len(all) != 3 {
		t.Fatalf("schema history was not retained: %#v err=%v", all, err)
	}
	if _, err := service.CreateRuntimeSchema(t.Context(), RuntimeSchemaInput{TenantID: "tenant-1", SchemaID: "invalid", Revision: 1, Definition: map[string]any{"type": "object"}, RetentionPolicy: "7d", CreatedBy: "operator"}); !hasDomainCode(err, "RUNTIME_SCHEMA_RETENTION_INVALID") {
		t.Fatalf("unknown retention policy was accepted: %v", err)
	}
}

func TestStateCollectionRequiresPublishedRuntimeSchema(t *testing.T) {
	service := New(memory.New(), time.Now)
	started, err := service.Start(t.Context(), testStartInput("schema-policy", idgen.New()))
	if err != nil {
		t.Fatal(err)
	}
	collection := stateCollectionForTest(started, started.Plan.Nodes[0].Key, "governed", "cas_map", 10)
	collection.SchemaRevision = 1
	if err := service.CreateStateCollection(t.Context(), collection); !hasDomainCode(err, "STATE_SCHEMA_NOT_PUBLISHED") {
		t.Fatalf("state collection accepted an unregistered schema: %v", err)
	}
	if _, err := service.CreateRuntimeSchema(t.Context(), RuntimeSchemaInput{TenantID: collection.TenantID, SchemaID: collection.SchemaID, Revision: 1, Definition: map[string]any{"type": "object"}, RetentionPolicy: "job", CreatedBy: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := service.CreateStateCollection(t.Context(), collection); !hasDomainCode(err, "STATE_SCHEMA_NOT_PUBLISHED") {
		t.Fatalf("state collection accepted a draft schema: %v", err)
	}
}
