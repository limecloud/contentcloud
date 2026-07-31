package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingLogWriterBoundsActiveAndBackupFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	writer, err := newRotatingLogWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	writer.maxSize, writer.backups = 8, 2
	for index := 0; index < 5; index++ {
		if _, err := writer.Write([]byte(strings.Repeat("x", 6))); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 3 {
		t.Fatalf("too many log files: %#v", entries)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
