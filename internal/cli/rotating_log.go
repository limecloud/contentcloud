package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	defaultDaemonLogMaxBytes = 10 << 20
	defaultDaemonLogBackups  = 5
)

type rotatingLogWriter struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	backups int
	file    *os.File
}

func newRotatingLogWriter(path string) (*rotatingLogWriter, error) {
	writer := &rotatingLogWriter{path: path, maxSize: defaultDaemonLogMaxBytes, backups: defaultDaemonLogBackups}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := writer.prune(); err != nil {
		return nil, err
	}
	return writer, nil
}

func (w *rotatingLogWriter) Write(body []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ensureFile(); err != nil {
		return 0, err
	}
	if info, err := w.file.Stat(); err == nil && info.Size()+int64(len(body)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	return w.file.Write(body)
}

func (w *rotatingLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *rotatingLogWriter) ensureFile() error {
	if w.file != nil {
		return nil
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	w.file = file
	return nil
}

func (w *rotatingLogWriter) rotate() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}
	for index := w.backups - 1; index >= 1; index-- {
		oldPath := fmt.Sprintf("%s.%d", w.path, index)
		newPath := fmt.Sprintf("%s.%d", w.path, index+1)
		if err := os.Rename(oldPath, newPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Rename(w.path, w.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return w.ensureFile()
}

func (w *rotatingLogWriter) prune() error {
	entries, err := os.ReadDir(filepath.Dir(w.path))
	if err != nil {
		return err
	}
	base := filepath.Base(w.path)
	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) <= len(base) || entry.Name()[:len(base)] != base || entry.Name()[len(base)] != '.' {
			continue
		}
		var index int
		if _, scanErr := fmt.Sscanf(entry.Name()[len(base)+1:], "%d", &index); scanErr != nil || index <= w.backups {
			continue
		}
		if err := os.Remove(filepath.Join(filepath.Dir(w.path), entry.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

var _ io.Writer = (*rotatingLogWriter)(nil)
