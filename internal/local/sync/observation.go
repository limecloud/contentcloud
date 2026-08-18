package localsync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

var contentRoots = []string{
	"10-context", "20-sources", "30-knowledge", "40-work",
	"50-production", "60-delivery", "70-results", "90-archive",
}

type Observation struct {
	WorkspaceID string
	ProjectID   string
	Digest      string
	FileCount   int
	ByteSize    int64
	Files       []workspacedomain.WorkspaceRevisionFile
}

type ObservationError struct {
	Code string
	Ref  string
}

func (e *ObservationError) Error() string {
	if e.Ref == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Ref)
}

func ObserveWorkspace(root string) (Observation, error) {
	status, err := localworkspace.LoadStatus(root)
	if err != nil {
		return Observation{}, err
	}
	entries := make([]workspacedomain.WorkspaceRevisionFile, 0)
	for _, contentRoot := range contentRoots {
		base := filepath.Join(status.Root, filepath.FromSlash(contentRoot))
		if _, err := os.Stat(base); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return Observation{}, err
		}
		err := filepath.WalkDir(base, func(path string, directoryEntry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			ref, err := filepath.Rel(status.Root, path)
			if err != nil {
				return err
			}
			ref = filepath.ToSlash(ref)
			if directoryEntry.Type()&os.ModeSymlink != 0 {
				return &ObservationError{Code: "WORKSPACE_SYMLINK_DENIED", Ref: ref}
			}
			if directoryEntry.IsDir() {
				return nil
			}
			infoBefore, err := directoryEntry.Info()
			if err != nil {
				return err
			}
			if !infoBefore.Mode().IsRegular() {
				return &ObservationError{Code: "WORKSPACE_FILE_TYPE_DENIED", Ref: ref}
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			hash := sha256.New()
			_, copyErr := io.Copy(hash, file)
			infoAfter, statErr := file.Stat()
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if statErr != nil {
				return statErr
			}
			if closeErr != nil {
				return closeErr
			}
			if infoBefore.Size() != infoAfter.Size() || !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
				return &ObservationError{Code: "WORKSPACE_FILE_UNSTABLE", Ref: ref}
			}
			entries = append(entries, workspacedomain.WorkspaceRevisionFile{Ref: ref, Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)), ByteSize: infoAfter.Size()})
			return nil
		})
		if err != nil {
			return Observation{}, err
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Ref < entries[j].Ref })
	var bytes int64
	for _, item := range entries {
		bytes += item.ByteSize
	}
	return Observation{
		WorkspaceID: status.Binding.WorkspaceID,
		ProjectID:   status.Binding.ProjectID,
		Digest:      workspacedomain.WorkspaceContentDigest(entries),
		FileCount:   len(entries),
		ByteSize:    bytes,
		Files:       entries,
	}, nil
}
