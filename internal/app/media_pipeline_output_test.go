package app

import (
	"context"
	"errors"
	"testing"

	"github.com/limecloud/contentcloud/internal/blob"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/mediapipeline"
	"github.com/limecloud/contentcloud/internal/store/memory"
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
	service := NewWithBlob(memory.New(), nil, store)
	_, err := service.persistMediaOutput(t.Context(), mediapipeline.FakeProvider{}, "fake-output:test", domain.ProviderProfile{}, "media/tenant/job/")
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
