package blob

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrNotFound = errors.New("文件对象不存在")

type Store interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
}

// ReaderStore is an optional large-object path. Callers must still enforce
// their domain-specific size and content checks before publishing metadata.
type ReaderStore interface {
	PutReader(context.Context, string, io.Reader, int64) error
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string][]byte
}

func NewMemory() *MemoryStore {
	return &MemoryStore{items: map[string][]byte{}}
}

func (s *MemoryStore) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = append([]byte(nil), data...)
	return nil
}

func (s *MemoryStore) PutReader(_ context.Context, key string, reader io.Reader, size int64) error {
	if size > maxMemoryObjectBytes {
		return errors.New("内存对象超过大小限制")
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxMemoryObjectBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxMemoryObjectBytes || (size >= 0 && int64(len(data)) != size) {
		return errors.New("内存对象大小无效")
	}
	return s.Put(context.Background(), key, data)
}

func (s *MemoryStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.items[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

type LocalStore struct {
	root string
}

const maxMemoryObjectBytes = 150 * 1024 * 1024

func NewLocal(root string) (*LocalStore, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	return &LocalStore{root: abs}, nil
}

func (s *LocalStore) path(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(key, "/")))
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." || filepath.IsAbs(clean) {
		return "", fs.ErrInvalid
	}
	path := filepath.Join(s.root, clean)
	if !strings.HasPrefix(path, s.root+string(filepath.Separator)) {
		return "", fs.ErrInvalid
	}
	return path, nil
}

func (s *LocalStore) Put(_ context.Context, key string, data []byte) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".contentcloud-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *LocalStore) PutReader(_ context.Context, key string, reader io.Reader, size int64) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".contentcloud-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	written, copyErr := io.Copy(tmp, io.LimitReader(reader, maxMemoryObjectBytes+1))
	if copyErr != nil {
		_ = tmp.Close()
		return copyErr
	}
	if written > maxMemoryObjectBytes || (size >= 0 && written != size) {
		_ = tmp.Close()
		return errors.New("对象大小无效")
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *LocalStore) Get(_ context.Context, key string) ([]byte, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	value, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return value, err
}
