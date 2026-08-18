package runtime

import (
	"regexp"
	"strings"
)

var sha256Pattern = regexp.MustCompile(`^(sha256:)?[0-9a-f]{64}$`)

func validSHA256Digest(value string) bool {
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
