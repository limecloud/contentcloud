package pluginhost

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CanonicalPath resolves existing symlinks while still supporting paths whose
// final components do not exist yet.
func CanonicalPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return filepath.Clean(canonical), nil
	}
	current := absolute
	missing := []string{}
	for {
		if _, statErr := os.Stat(current); statErr == nil {
			canonicalParent, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				return "", evalErr
			}
			for index := len(missing) - 1; index >= 0; index-- {
				canonicalParent = filepath.Join(canonicalParent, missing[index])
			}
			return filepath.Clean(canonicalParent), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(absolute), nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
