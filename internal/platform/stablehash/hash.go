package stablehash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func Sum(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// Valid reports whether value is a lowercase SHA-256 digest with the
// canonical "sha256:" prefix.
func Valid(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[len("sha256:"):] {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

// Matches accepts either a raw lowercase SHA-256 hex string or the canonical
// prefixed form used by persisted domain facts.
func Matches(value string) bool {
	if strings.HasPrefix(value, "sha256:") {
		return Valid(value)
	}
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func Normalize(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
}
