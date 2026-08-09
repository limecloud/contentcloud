package runtime

import (
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func stateCollectionForTest(started StartResult, nodeKey, key string, consistency string, maxRecords int) domain.StateCollection {
	now := started.Job.CreatedAt
	return domain.StateCollection{
		ID:              domain.NewID(),
		TenantID:        started.Job.TenantID,
		JobRunID:        started.Job.ID,
		CollectionKey:   key,
		Scope:           "job",
		SchemaID:        "contentcloud.test-state",
		SchemaRevision:  2,
		Consistency:     consistency,
		WriterNodeKey:   nodeKey,
		MaxRecordBytes:  1024,
		MaxRecords:      maxRecords,
		RetentionPolicy: "job",
		WritePolicy:     []string{"worker-1", "node:" + nodeKey},
		Revision:        0,
		Watermark:       0,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func stateRecordForTest(collection domain.StateCollection, key, actor string) domain.StateRecord {
	now := collection.CreatedAt
	return domain.StateRecord{
		ID:             domain.NewID(),
		TenantID:       collection.TenantID,
		CollectionID:   collection.ID,
		Key:            key,
		Value:          map[string]any{"value": key},
		SchemaRevision: collection.SchemaRevision,
		Version:        1,
		Digest:         "sha256:state-" + key,
		CreatedBy:      actor,
		UpdatedBy:      actor,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func publishStateSchemaForTest(t *testing.T, service *Service, collection domain.StateCollection) {
	t.Helper()
	for revision := 1; revision <= collection.SchemaRevision; revision++ {
		if existing, err := service.RuntimeSchema(t.Context(), collection.TenantID, collection.SchemaID, revision); err == nil {
			if existing.Status != "published" {
				t.Fatalf("test Runtime Schema is not published: %#v", existing)
			}
			continue
		}
		draft, err := service.CreateRuntimeSchema(t.Context(), RuntimeSchemaInput{TenantID: collection.TenantID, SchemaID: collection.SchemaID, Revision: revision, Definition: map[string]any{"type": "object"}, RetentionPolicy: collection.RetentionPolicy, CreatedBy: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.PublishRuntimeSchema(t.Context(), collection.TenantID, collection.SchemaID, revision, draft.Version); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStateCollectionAuthorizationAndLimits(t *testing.T) {
	repo := memory.New()
	service := New(repo, time.Now)
	started, err := service.Start(t.Context(), testStartInput("state-auth", "state-auth-job"))
	if err != nil {
		t.Fatal(err)
	}
	nodeKey := started.Plan.Nodes[0].Key
	collection := stateCollectionForTest(started, nodeKey, "brief", "cas_map", 1)
	publishStateSchemaForTest(t, service, collection)
	if err := service.CreateStateCollection(t.Context(), collection); err != nil {
		t.Fatal(err)
	}

	record := stateRecordForTest(collection, "topic", "worker-1")
	stored, err := service.StateRecordCAS(t.Context(), record, 0)
	if err != nil || stored.Version != 1 {
		t.Fatalf("authorized state write failed: %#v err=%v", stored, err)
	}

	wrongSchema := stateRecordForTest(collection, "other", "worker-1")
	wrongSchema.SchemaRevision++
	if _, err := service.StateRecordCAS(t.Context(), wrongSchema, 0); !hasDomainCode(err, "STATE_SCHEMA_REVISION_CONFLICT") {
		t.Fatalf("schema drift was accepted: %v", err)
	}
	unauthorized := stateRecordForTest(collection, "unauthorized", "worker-2")
	if _, err := service.StateRecordCAS(t.Context(), unauthorized, 0); !hasDomainCode(err, "STATE_WRITE_FORBIDDEN") {
		t.Fatalf("unauthorized state write was accepted: %v", err)
	}
	limited := stateRecordForTest(collection, "second", "worker-1")
	if _, err := service.StateRecordCAS(t.Context(), limited, 0); !hasDomainCode(err, "STATE_COLLECTION_RECORD_LIMIT") {
		t.Fatalf("record limit was not enforced: %v", err)
	}

	appendCollection := stateCollectionForTest(started, "", "events", "append_only", 2)
	appendCollection.WriterNodeKey = ""
	if err := service.CreateStateCollection(t.Context(), appendCollection); err != nil {
		t.Fatal(err)
	}
	first := stateRecordForTest(appendCollection, "event-1", "worker-1")
	if _, err := service.StateRecordCAS(t.Context(), first, 0); err != nil {
		t.Fatal(err)
	}
	first.Value = map[string]any{"value": "updated"}
	first.UpdatedAt = first.UpdatedAt.Add(time.Second)
	if _, err := service.StateRecordCAS(t.Context(), first, 1); !hasDomainCode(err, "STATE_APPEND_ONLY_UPDATE_FORBIDDEN") {
		t.Fatalf("append-only overwrite was accepted: %v", err)
	}
}

func TestStateCollectionSingleWriterRequiresWriterIdentity(t *testing.T) {
	repo := memory.New()
	service := New(repo, time.Now)
	started, err := service.Start(t.Context(), testStartInput("state-writer", "state-writer-job"))
	if err != nil {
		t.Fatal(err)
	}
	writerNode := started.Plan.Nodes[0].Key
	collection := stateCollectionForTest(started, writerNode, "reducer", "single_writer", 2)
	publishStateSchemaForTest(t, service, collection)
	if err := service.CreateStateCollection(t.Context(), collection); err != nil {
		t.Fatal(err)
	}
	record := stateRecordForTest(collection, "summary", "node:"+writerNode)
	if _, err := service.StateRecordCAS(t.Context(), record, 0); err != nil {
		t.Fatal(err)
	}
	record = stateRecordForTest(collection, "blocked", "worker-1")
	if _, err := service.StateRecordCAS(t.Context(), record, 0); !hasDomainCode(err, "STATE_WRITE_FORBIDDEN") {
		t.Fatalf("single writer accepted a non-writer actor: %v", err)
	}
}

func TestToolCallRequiresActiveAttemptAndAllowedTool(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	service, repo, started := newDispatchRuntime(t, fake, time.Now)
	handle, err := service.PrepareDispatch(t.Context(), dispatchInput(started.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := fake.Start(t.Context(), agentadapter.StartAgentRequest{JobRunID: handle.Node.JobRunID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID})
	if err != nil {
		t.Fatal(err)
	}
	handle, err = service.ActivateDispatch(t.Context(), handle, session)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	call := domain.ToolCall{TenantID: started.Job.TenantID, JobRunID: started.Job.ID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID, AgentInstanceID: handle.Agent.ID, ID: domain.NewID(), ToolName: "state.get", SchemaVersion: "contentcloud.tool/state.get/1", RequestDigest: "sha256:request", SafeRequest: map[string]any{"collection": "brief"}, State: domain.ToolCallProposed, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := service.CreateToolCall(t.Context(), call); err != nil {
		t.Fatal(err)
	}
	call, err = repo.ToolCall(t.Context(), call.TenantID, call.ID)
	if err != nil {
		t.Fatal(err)
	}
	next := call
	next.State = domain.ToolCallAuthorized
	next.Version++
	next.UpdatedAt = now.Add(time.Second)
	if _, err := service.TransitionToolCall(t.Context(), next, call.Version); err != nil {
		t.Fatal(err)
	}

	unauthorized := next
	unauthorized.ID = domain.NewID()
	unauthorized.Version = 1
	unauthorized.State = domain.ToolCallProposed
	unauthorized.ToolName = "provider.submit"
	unauthorized.RequestDigest = "sha256:other-request"
	unauthorized.CreatedAt = now
	unauthorized.UpdatedAt = now
	if err := service.CreateToolCall(t.Context(), unauthorized); !hasDomainCode(err, "TOOL_CALL_NOT_ALLOWED") {
		t.Fatalf("tool outside ContextView allowlist was accepted: %v", err)
	}

	stale := next
	stale.Version++
	stale.State = domain.ToolCallRunning
	stale.UpdatedAt = now.Add(2 * time.Second)
	if _, err := service.TransitionToolCall(t.Context(), stale, call.Version); !hasDomainCode(err, "TOOL_CALL_VERSION_CONFLICT") {
		t.Fatalf("stale ToolCall transition was accepted: %v", err)
	}
}
