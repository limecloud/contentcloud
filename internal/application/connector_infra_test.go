package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/integration/connector"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

type cursorConnector struct{ now time.Time }

type staticConnector struct{ result connector.PullResult }

func (v staticConnector) Pull(context.Context, connector.PullRequest) (connector.PullResult, error) {
	return v.result, nil
}

type blockingConnector struct {
	started chan struct{}
	release chan struct{}
	result  connector.PullResult
}

func (v *blockingConnector) Pull(ctx context.Context, _ connector.PullRequest) (connector.PullResult, error) {
	close(v.started)
	select {
	case <-v.release:
		return v.result, nil
	case <-ctx.Done():
		return connector.PullResult{}, ctx.Err()
	}
}

type failOnceCommitRepository struct {
	connector.Repository
	mu        sync.Mutex
	remaining int
}

func (v *failOnceCommitRepository) CommitReceipt(ctx context.Context, binding connector.Binding, cursor, leaseOwner string, receipt connector.SyncReceipt) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.remaining > 0 {
		v.remaining--
		return errors.New("fixture commit interruption")
	}
	return v.Repository.CommitReceipt(ctx, binding, cursor, leaseOwner, receipt)
}

type sharedSourceConnector struct{ now time.Time }

func (v sharedSourceConnector) Pull(_ context.Context, request connector.PullRequest) (connector.PullResult, error) {
	switch request.Cursor {
	case "":
		body := []byte("shared source body")
		return connector.PullResult{NextCursor: "cursor-1", Records: []connector.Record{
			{ExternalID: "record-a", Version: "v1", MIME: "text/plain", Body: body, UpdatedAt: v.now},
			{ExternalID: "record-b", Version: "v1", MIME: "text/plain", Body: body, UpdatedAt: v.now},
		}}, nil
	case "cursor-1":
		deletedAt := v.now.Add(time.Minute)
		return connector.PullResult{NextCursor: "cursor-2", Records: []connector.Record{{ExternalID: "record-a", Version: "v2", Deleted: true, DeletedAt: &deletedAt, UpdatedAt: deletedAt}}}, nil
	default:
		deletedAt := v.now.Add(2 * time.Minute)
		return connector.PullResult{NextCursor: "cursor-3", Records: []connector.Record{{ExternalID: "record-b", Version: "v2", Deleted: true, DeletedAt: &deletedAt, UpdatedAt: deletedAt}}}, nil
	}
}

func (v cursorConnector) Pull(_ context.Context, request connector.PullRequest) (connector.PullResult, error) {
	if request.Cursor == "" {
		return connector.PullResult{NextCursor: "cursor-1", Records: []connector.Record{{ExternalID: "article-1", Version: "v1", Title: "首篇文章", SourceURL: "https://cms.example/articles/1", MIME: "text/html", Body: []byte("<article>first</article>"), UpdatedAt: v.now, Rights: map[string]any{"usage": "owned"}, Metadata: map[string]any{"collection": "articles"}}}}, nil
	}
	deletedAt := v.now.Add(time.Minute)
	return connector.PullResult{NextCursor: "cursor-2", Records: []connector.Record{{ExternalID: "article-1", Version: "v2", Title: "首篇文章", Deleted: true, DeletedAt: &deletedAt, UpdatedAt: deletedAt}}}, nil
}

func TestConnectorSyncMaterializesSourceRevisionAndTombstone(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	registry := connector.NewRegistry()
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	registry.Register("cms", cursorConnector{now: now})
	service := application.New(application.DependenciesFrom(store), nil, application.WithConnectorRegistry(registry))
	session, err := service.Identity.Register(ctx, "connector-owner@example.com", "long-enough-password", "Connector", "Connector Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Source.CreateConnectorBinding(ctx, actor, application.CreateConnectorBindingInput{ProjectID: project.ID, ConnectorID: "cms", AuthorizationRef: "plaintext-token", Region: "cn"}, "bad-secret"); err == nil {
		t.Fatal("明文 token 不得进入 Connector 绑定")
	}
	binding, err := service.Source.CreateConnectorBinding(ctx, actor, application.CreateConnectorBindingInput{ProjectID: project.ID, ConnectorID: "cms", AuthorizationRef: "secret://connectors/cms", Region: "cn"}, "bind")
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Source.SyncConnector(ctx, actor, binding.ID, application.SyncConnectorInput{Limit: 100}, "sync-1")
	if err != nil || first.NextCursor != "cursor-1" || first.UpsertCount != 1 {
		t.Fatalf("unexpected first sync: %#v err=%v", first, err)
	}
	mapping, err := store.Record(ctx, actor.TenantID, binding.ID, "article-1")
	if err != nil || mapping.SourceID == "" || mapping.RevisionID == "" || mapping.Deleted {
		t.Fatalf("record was not linked to SourceRevision: %#v err=%v", mapping, err)
	}
	firstRevisionID := mapping.RevisionID
	second, err := service.Source.SyncConnector(ctx, actor, binding.ID, application.SyncConnectorInput{Limit: 100}, "sync-2")
	if err != nil || second.NextCursor != "cursor-2" || second.TombstoneCount != 1 {
		t.Fatalf("unexpected tombstone sync: %#v err=%v", second, err)
	}
	mapping, err = store.Record(ctx, actor.TenantID, binding.ID, "article-1")
	if err != nil || !mapping.Deleted || mapping.RevisionID == firstRevisionID {
		t.Fatalf("tombstone was not persisted: %#v err=%v", mapping, err)
	}
	revision, err := store.SourceRevision(ctx, actor.TenantID, mapping.RevisionID)
	if err != nil || revision.ProcessingStatus != "invalidated" {
		t.Fatalf("tombstone did not invalidate SourceRevision: %#v err=%v", revision, err)
	}
	bindings, _ := service.Source.ConnectorBindings(ctx, actor, project.ID)
	if len(bindings) != 1 || bindings[0].Cursor != "cursor-2" {
		t.Fatalf("opaque cursor was not committed: %#v", bindings)
	}
	receipts, _ := service.Source.ConnectorReceipts(ctx, actor, binding.ID)
	if len(receipts) != 2 || receipts[0].Digest == "" {
		t.Fatalf("sync receipts missing: %#v", receipts)
	}
}

func TestConnectorSyncRejectsUnsupportedMIMEBeforeMaterialization(t *testing.T) {
	ctx := t.Context()
	now := time.Now().UTC()
	store := memory.New()
	adapter := staticConnector{result: connector.PullResult{Records: []connector.Record{{ExternalID: "binary-1", Version: "v1", Title: "binary", MIME: "application/octet-stream", Body: []byte{1, 2, 3}, UpdatedAt: now}}}}
	registry := connector.NewRegistry()
	registry.Register("fixture", adapter)
	service := application.New(application.DependenciesFrom(store), nil, application.WithConnectorRegistry(registry))
	session, err := service.Identity.Register(ctx, "connector-mime@example.com", "long-enough-password", "Connector", "Connector Team")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := service.Source.CreateConnectorBinding(ctx, actor, application.CreateConnectorBindingInput{ProjectID: project.ID, ConnectorID: "fixture", AuthorizationRef: "secret://fixture"}, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Source.SyncConnector(ctx, actor, binding.ID, application.SyncConnectorInput{}, "")
	var domainErr *fault.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "CONNECTOR_MIME_UNSUPPORTED" {
		t.Fatalf("unsupported MIME was not rejected explicitly: %v", err)
	}
	sources, err := store.Sources(ctx, actor.TenantID, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("unsupported MIME created source facts: %#v", sources)
	}
}

func TestConnectorSyncLeaseRejectsConcurrentPull(t *testing.T) {
	ctx := t.Context()
	now := time.Now().UTC()
	store := memory.New()
	adapter := &blockingConnector{started: make(chan struct{}), release: make(chan struct{}), result: connector.PullResult{NextCursor: "cursor-1", Records: []connector.Record{{ExternalID: "record-1", Version: "v1", MIME: "text/plain", Body: []byte("body"), UpdatedAt: now}}}}
	registry := connector.NewRegistry()
	registry.Register("blocking", adapter)
	service := application.New(application.DependenciesFrom(store), nil, application.WithConnectorRegistry(registry))
	session, err := service.Identity.Register(ctx, "connector-lease@example.com", "long-enough-password", "Connector", "Lease Team")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, _ := service.Identity.SessionActor(ctx, session.ID)
	project, _ := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	binding, err := service.Source.CreateConnectorBinding(ctx, actor, application.CreateConnectorBindingInput{ProjectID: project.ID, ConnectorID: "blocking", AuthorizationRef: "secret://blocking"}, "")
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() {
		_, syncErr := service.Source.SyncConnector(ctx, actor, binding.ID, application.SyncConnectorInput{}, "first")
		firstDone <- syncErr
	}()
	<-adapter.started
	_, err = service.Source.SyncConnector(ctx, actor, binding.ID, application.SyncConnectorInput{}, "second")
	var domainErr *fault.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "CONNECTOR_SYNC_IN_PROGRESS" {
		t.Fatalf("concurrent sync was not rejected by lease: %v", err)
	}
	close(adapter.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("leased sync failed: %v", err)
	}
}

func TestConnectorSyncReplaysMaterializedPageAfterCommitInterruption(t *testing.T) {
	ctx := t.Context()
	now := time.Now().UTC()
	store := memory.New()
	repository := &failOnceCommitRepository{Repository: store, remaining: 1}
	registry := connector.NewRegistry()
	registry.Register("fixture", staticConnector{result: connector.PullResult{NextCursor: "cursor-1", Records: []connector.Record{{ExternalID: "record-1", Version: "v1", MIME: "text/plain", Body: []byte("stable body"), UpdatedAt: now}}}})
	service := application.New(application.DependenciesFrom(store), nil, application.WithConnectorRegistry(registry), application.WithConnectorRepository(repository))
	session, err := service.Identity.Register(ctx, "connector-replay@example.com", "long-enough-password", "Connector", "Replay Team")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, _ := service.Identity.SessionActor(ctx, session.ID)
	project, _ := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	binding, _ := service.Source.CreateConnectorBinding(ctx, actor, application.CreateConnectorBindingInput{ProjectID: project.ID, ConnectorID: "fixture", AuthorizationRef: "secret://fixture"}, "")
	if _, err := service.Source.SyncConnector(ctx, actor, binding.ID, application.SyncConnectorInput{}, "interrupted"); err == nil {
		t.Fatal("fixture commit interruption was not returned")
	}
	receipt, err := service.Source.SyncConnector(ctx, actor, binding.ID, application.SyncConnectorInput{}, "replay")
	if err != nil || receipt.NextCursor != "cursor-1" {
		t.Fatalf("replayed page did not commit: %#v err=%v", receipt, err)
	}
	mapping, err := store.Record(ctx, actor.TenantID, binding.ID, "record-1")
	if err != nil {
		t.Fatal(err)
	}
	revisions, err := store.SourceRevisions(ctx, actor.TenantID, mapping.SourceID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("page replay duplicated SourceRevision: %#v err=%v", revisions, err)
	}
}

func TestConnectorTombstoneKeepsSharedSourceUntilLastActiveRecord(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	registry := connector.NewRegistry()
	registry.Register("shared", sharedSourceConnector{now: time.Now().UTC()})
	service := application.New(application.DependenciesFrom(store), nil, application.WithConnectorRegistry(registry))
	session, err := service.Identity.Register(ctx, "connector-shared@example.com", "long-enough-password", "Connector", "Shared Team")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, _ := service.Identity.SessionActor(ctx, session.ID)
	project, _ := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	binding, _ := service.Source.CreateConnectorBinding(ctx, actor, application.CreateConnectorBindingInput{ProjectID: project.ID, ConnectorID: "shared", AuthorizationRef: "secret://shared"}, "")
	if _, err := service.Source.SyncConnector(ctx, actor, binding.ID, application.SyncConnectorInput{}, "create"); err != nil {
		t.Fatal(err)
	}
	a, _ := store.Record(ctx, actor.TenantID, binding.ID, "record-a")
	b, _ := store.Record(ctx, actor.TenantID, binding.ID, "record-b")
	if a.SourceID == "" || a.SourceID != b.SourceID || a.RevisionID != b.RevisionID {
		t.Fatalf("duplicate external records did not share SourceRevision: a=%#v b=%#v", a, b)
	}
	if _, err := service.Source.SyncConnector(ctx, actor, binding.ID, application.SyncConnectorInput{}, "delete-a"); err != nil {
		t.Fatal(err)
	}
	revision, err := store.SourceRevision(ctx, actor.TenantID, b.RevisionID)
	if err != nil || revision.ProcessingStatus == "invalidated" {
		t.Fatalf("first tombstone invalidated a still-referenced source: %#v err=%v", revision, err)
	}
	if _, err := service.Source.SyncConnector(ctx, actor, binding.ID, application.SyncConnectorInput{}, "delete-b"); err != nil {
		t.Fatal(err)
	}
	b, _ = store.Record(ctx, actor.TenantID, binding.ID, "record-b")
	finalRevision, err := store.SourceRevision(ctx, actor.TenantID, b.RevisionID)
	if err != nil || finalRevision.ProcessingStatus != "invalidated" {
		t.Fatalf("last tombstone did not create invalidation revision: %#v err=%v", finalRevision, err)
	}
}
