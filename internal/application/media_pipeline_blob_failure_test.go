package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

var errFinalRenderBlobRead = errors.New("候选成片对象读取失败")
var errFinalRenderBlobPut = errors.New("最终成片对象写入确认失败")

type getFailureBlobStore struct {
	inner *blob.MemoryStore
}

func (s *getFailureBlobStore) Put(ctx context.Context, key string, data []byte) error {
	return s.inner.Put(ctx, key, data)
}

func (s *getFailureBlobStore) Get(context.Context, string) ([]byte, error) {
	return nil, errFinalRenderBlobRead
}

func (s *getFailureBlobStore) Delete(ctx context.Context, key string) error {
	return s.inner.Delete(ctx, key)
}

func TestCreateFinalRenderBlobGetFailureLeavesNoFinalFacts(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	blobs := &getFailureBlobStore{inner: blob.NewMemory()}
	service := application.NewWithBlob(application.DependenciesFrom(store), nil, blobs)
	session, err := service.Identity.Register(ctx, "final-render-blob-read@example.com", "long-enough-password", "最终成片测试", "视频租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Operations.EnsureMarketingVideoDemoFixture(ctx, actor, "blob-get-failure"); !errors.Is(err, errFinalRenderBlobRead) {
		t.Fatalf("expected candidate video blob read failure, got %v", err)
	}

	projects, err := service.Workspace.Projects(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var projectID string
	for _, project := range projects {
		if project.BrandName == "金陵古都" && project.ProductName == "金陵古都香" {
			projectID = project.ID
			break
		}
	}
	if projectID == "" {
		t.Fatal("fixture project was not persisted before the expected blob failure")
	}
	tasks, err := service.Work.WorkTasks(ctx, actor, projectID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("expected one partially progressed fixture task: tasks=%#v err=%v", tasks, err)
	}
	view, err := service.Work.WorkTask(ctx, actor, tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range view.Artifacts {
		if artifact.Kind == "final_render" {
			t.Fatalf("blob Get failure created final_render artifact: %#v", artifact)
		}
	}
	for _, review := range view.MediaReviews {
		if review.ReviewKind == deliverydomain.MediaReviewFinal {
			t.Fatalf("blob Get failure created final media review: %#v", review)
		}
	}
}

type finalPutFailureBlobStore struct {
	inner       *blob.MemoryStore
	writeBefore bool
	deleted     []string
}

func (s *finalPutFailureBlobStore) Put(ctx context.Context, key string, data []byte) error {
	if strings.Contains(key, "/final/") {
		if s.writeBefore {
			if err := s.inner.Put(ctx, key, data); err != nil {
				return err
			}
		}
		return errFinalRenderBlobPut
	}
	return s.inner.Put(ctx, key, data)
}

func (s *finalPutFailureBlobStore) Get(ctx context.Context, key string) ([]byte, error) {
	return s.inner.Get(ctx, key)
}

func (s *finalPutFailureBlobStore) Delete(ctx context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return s.inner.Delete(ctx, key)
}

func TestCreateFinalRenderBlobPutFailureLeavesNoFinalFactsAndCleansObject(t *testing.T) {
	for _, writeBefore := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-write", true: "after-write"}[writeBefore], func(t *testing.T) {
			ctx := t.Context()
			store := memory.New()
			blobs := &finalPutFailureBlobStore{inner: blob.NewMemory(), writeBefore: writeBefore}
			service := application.NewWithBlob(application.DependenciesFrom(store), nil, blobs)
			session, err := service.Identity.Register(ctx, "final-render-blob-put-"+map[bool]string{false: "before", true: "after"}[writeBefore]+"@example.com", "long-enough-password", "最终成片测试", "视频租户")
			if err != nil {
				t.Fatal(err)
			}
			actor, _, err := service.Identity.SessionActor(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := service.Operations.EnsureMarketingVideoDemoFixture(ctx, actor, "blob-put-failure"); !errors.Is(err, errFinalRenderBlobPut) {
				t.Fatalf("expected final render Blob Put failure, got %v", err)
			}
			if len(blobs.deleted) != 1 || !strings.Contains(blobs.deleted[0], "/final/") {
				t.Fatalf("final render Blob failure did not trigger exactly one cleanup: deleted=%#v", blobs.deleted)
			}
			if _, err := blobs.Get(ctx, blobs.deleted[0]); !errors.Is(err, blob.ErrNotFound) {
				t.Fatalf("orphan final render object remains after cleanup: %v", err)
			}

			projects, err := service.Workspace.Projects(ctx, actor)
			if err != nil {
				t.Fatal(err)
			}
			var projectID string
			for _, project := range projects {
				if project.BrandName == "金陵古都" && project.ProductName == "金陵古都香" {
					projectID = project.ID
					break
				}
			}
			if projectID == "" {
				t.Fatal("fixture project was not persisted before the expected blob failure")
			}
			tasks, err := service.Work.WorkTasks(ctx, actor, projectID)
			if err != nil || len(tasks) != 1 {
				t.Fatalf("expected one partially progressed fixture task: tasks=%#v err=%v", tasks, err)
			}
			view, err := service.Work.WorkTask(ctx, actor, tasks[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, artifact := range view.Artifacts {
				if artifact.Kind == "final_render" {
					t.Fatalf("blob Put failure created final_render artifact: %#v", artifact)
				}
			}
			for _, review := range view.MediaReviews {
				if review.ReviewKind == deliverydomain.MediaReviewFinal {
					t.Fatalf("blob Put failure created final media review: %#v", review)
				}
			}
		})
	}
}

type finalPutCleanupFailureBlobStore struct {
	inner *blob.MemoryStore
}

func (s *finalPutCleanupFailureBlobStore) Put(ctx context.Context, key string, data []byte) error {
	if strings.Contains(key, "/final/") {
		if err := s.inner.Put(ctx, key, data); err != nil {
			return err
		}
		return errFinalRenderBlobPut
	}
	return s.inner.Put(ctx, key, data)
}

func (s *finalPutCleanupFailureBlobStore) Get(ctx context.Context, key string) ([]byte, error) {
	return s.inner.Get(ctx, key)
}

func (s *finalPutCleanupFailureBlobStore) Delete(context.Context, string) error {
	return errors.New("对象存储删除确认失败，路径不应进入诊断摘要")
}

func TestCreateFinalRenderCleanupFailurePersistsSanitizedDiagnostic(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	blobs := &finalPutCleanupFailureBlobStore{inner: blob.NewMemory()}
	service := application.NewWithBlob(application.DependenciesFrom(store), nil, blobs)
	session, err := service.Identity.Register(ctx, "final-render-cleanup-diagnostic@example.com", "long-enough-password", "最终成片测试", "视频租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Operations.EnsureMarketingVideoDemoFixture(ctx, actor, "cleanup-diagnostic"); err == nil {
		t.Fatal("expected final render failure")
	}
	projects, err := service.Workspace.Projects(ctx, actor)
	if err != nil || len(projects) == 0 {
		t.Fatalf("fixture project missing: projects=%#v err=%v", projects, err)
	}
	diagnostics, err := store.RuntimeCleanupDiagnostics(ctx, actor.TenantID, contentruntime.RuntimeCleanupPending, 10)
	if err != nil || len(diagnostics) != 1 {
		t.Fatalf("cleanup diagnostic missing: diagnostics=%#v err=%v", diagnostics, err)
	}
	diagnostic := diagnostics[0]
	if diagnostic.ProjectID != projects[0].ID || diagnostic.AttemptCount != 0 || diagnostic.CleanupError != "BLOB_DELETE_FAILED" || diagnostic.ManifestDigest == "" {
		t.Fatalf("cleanup diagnostic fields invalid: %#v", diagnostic)
	}
	if strings.Contains(diagnostic.CauseSummary, "对象存储") || strings.Contains(diagnostic.CauseSummary, "路径") || strings.Contains(diagnostic.CleanupError, "对象存储") {
		t.Fatalf("cleanup diagnostic leaked raw error text: %#v", diagnostic)
	}
}
