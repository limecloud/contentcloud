package application

import (
	"context"
	"errors"
	"testing"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	mediapipeline "github.com/limecloud/contentcloud/internal/integration/provider/media"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

type failAfterWriteBlobStore struct {
	items   map[string][]byte
	deleted []string
}

func (s *failAfterWriteBlobStore) Put(_ context.Context, key string, data []byte) error {
	if s.items == nil {
		s.items = map[string][]byte{}
	}
	s.items[key] = append([]byte(nil), data...)
	return errors.New("对象存储写入确认失败")
}

func (s *failAfterWriteBlobStore) Get(_ context.Context, key string) ([]byte, error) {
	value, ok := s.items[key]
	if !ok {
		return nil, blob.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *failAfterWriteBlobStore) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	delete(s.items, key)
	return nil
}

func TestPersistMediaOutputCleansObjectWhenWriteFailsAfterPersistence(t *testing.T) {
	store := &failAfterWriteBlobStore{}
	service := NewWithBlob(DependenciesFrom(memory.New()), nil, store)
	_, err := service.Delivery.persistMediaOutput(t.Context(), mediapipeline.FakeProvider{}, "fake-output:test", deliverydomain.ProviderProfile{}, "media/tenant/job/")
	if err == nil {
		t.Fatal("expected object store write failure")
	}
	key := "media/tenant/job/generated-take.mp4"
	if len(store.deleted) != 1 || store.deleted[0] != key {
		t.Fatalf("failed output write did not trigger orphan cleanup: deleted=%#v", store.deleted)
	}
	if _, err := store.Get(t.Context(), key); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("orphan output object remains after cleanup: %v", err)
	}
}

type cleanupFailureBlobStore struct {
	deleted []string
}

func (s *cleanupFailureBlobStore) Put(context.Context, string, []byte) error { return nil }

func (s *cleanupFailureBlobStore) Get(context.Context, string) ([]byte, error) {
	return nil, blob.ErrNotFound
}

func (s *cleanupFailureBlobStore) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return errors.New("对象存储删除确认失败")
}

func TestCleanupFinalRenderBlobSurfacesCleanupFailure(t *testing.T) {
	store := &cleanupFailureBlobStore{}
	cause := fault.Conflict("MEDIA_ARTIFACT_CREATE_FAILED", "媒体成果文件保存失败")
	err := cleanupFinalRenderBlob(t.Context(), store, "media/tenant/task/final.mp4", cause)
	if err == nil {
		t.Fatal("cleanup failure should return an error")
	}
	var domainErr *fault.Error
	if !errors.As(err, &domainErr) || !domainErr.Retryable || domainErr.Code != cause.Code {
		t.Fatalf("cleanup failure should be retryable and structured: %#v", err)
	}
	details, ok := domainErr.Details.(map[string]any)
	if !ok || details["cause"] != cause.Error() {
		t.Fatalf("cleanup failure lost the database cause: %#v", domainErr.Details)
	}
	if len(store.deleted) != 1 {
		t.Fatalf("expected one cleanup attempt, got %v", store.deleted)
	}
}
