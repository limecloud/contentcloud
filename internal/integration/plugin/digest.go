package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func packageDigest(root string, files []packageFile) (string, error) {
	sorted := append([]packageFile(nil), files...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left].Path < sorted[right].Path })
	hash := sha256.New()
	for _, file := range sorted {
		_, _ = io.WriteString(hash, file.Path)
		_, _ = hash.Write([]byte{0})
		if file.Executable {
			_, _ = io.WriteString(hash, "executable")
		} else {
			_, _ = io.WriteString(hash, "regular")
		}
		_, _ = hash.Write([]byte{0})
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.Path)))
		if err != nil {
			return "", err
		}
		_, _ = hash.Write(body)
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func bytesDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
