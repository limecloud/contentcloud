package plugin

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type packageFile struct {
	Path       string
	Executable bool
	Size       int64
}

func inspectPackageRoot(path string, limits Limits) (string, []packageFile, int64, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil, 0, fmt.Errorf("plugin root is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, 0, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", nil, 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", nil, 0, fmt.Errorf("plugin root must be a non-symlink directory")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", nil, 0, err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", nil, 0, err
	}
	files := make([]packageFile, 0)
	var total int64
	err = filepath.WalkDir(resolved, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == resolved {
			return nil
		}
		relative, err := filepath.Rel(resolved, current)
		if err != nil || !isContainedRelative(relative) {
			return fmt.Errorf("package path escapes plugin root: %s", current)
		}
		depth := len(strings.Split(filepath.ToSlash(relative), "/"))
		if depth > limits.MaxDepth {
			return fmt.Errorf("package path exceeds maximum depth %d: %s", limits.MaxDepth, filepath.ToSlash(relative))
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("package contains unsupported symlink: %s", filepath.ToSlash(relative))
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("package contains non-regular file: %s", filepath.ToSlash(relative))
		}
		if len(files)+1 > limits.MaxFiles {
			return fmt.Errorf("package exceeds maximum file count %d", limits.MaxFiles)
		}
		if info.Size() > limits.MaxFileBytes {
			return fmt.Errorf("package file exceeds maximum size %d: %s", limits.MaxFileBytes, filepath.ToSlash(relative))
		}
		total += info.Size()
		if total > limits.MaxPackBytes {
			return fmt.Errorf("package exceeds maximum byte size %d", limits.MaxPackBytes)
		}
		files = append(files, packageFile{Path: filepath.ToSlash(relative), Executable: info.Mode()&0o111 != 0, Size: info.Size()})
		return nil
	})
	if err != nil {
		return "", nil, 0, err
	}
	return resolved, files, total, nil
}

func readPackageFile(root, relative string, maxBytes int64) ([]byte, error) {
	if !isContainedRelative(filepath.FromSlash(relative)) {
		return nil, fmt.Errorf("invalid plugin-relative path %q", relative)
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("plugin path is not a regular file: %s", relative)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("plugin file exceeds maximum size: %s", relative)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if !pathWithin(root, resolved) {
		return nil, fmt.Errorf("plugin path escapes root: %s", relative)
	}
	return os.ReadFile(resolved)
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && isContainedRelative(relative)
}

func isContainedRelative(path string) bool {
	if path == "" || path == "." || filepath.IsAbs(path) {
		return path == "."
	}
	clean := filepath.Clean(path)
	return clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
