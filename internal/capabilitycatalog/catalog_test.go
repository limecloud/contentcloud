package capabilitycatalog_test

import (
	"regexp"
	"testing"

	"github.com/limecloud/contentcloud/internal/capabilitycatalog"
	"github.com/limecloud/contentcloud/internal/domain"
)

func TestBuiltinsUseDeterministicSHA256Digests(t *testing.T) {
	first := capabilitycatalog.Builtins("0.5.0")
	second := capabilitycatalog.Builtins("0.5.0")
	if len(first) != 3 || len(second) != len(first) {
		t.Fatalf("builtin capabilities = %#v", first)
	}
	pattern := regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	for index := range first {
		if first[index].Digest != second[index].Digest || !pattern.MatchString(first[index].Digest) {
			t.Fatalf("capability digest is not deterministic SHA-256: %#v", first[index])
		}
	}
	changed, ok := capabilitycatalog.Exact(domain.ScriptCapability, "0.5.1")
	if !ok || changed.Digest == first[1].Digest {
		t.Fatal("implementation release version must be bound into the capability digest")
	}
}

func TestDigestCanonicalizesPresentationProfiles(t *testing.T) {
	capability, ok := capabilitycatalog.Exact(domain.ScriptCapability, "0.5.0")
	if !ok {
		t.Fatal("script capability missing")
	}
	reversed := capability
	reversed.PresentationProfiles = []string{"text", "review_projection/1.0"}
	if capabilitycatalog.Digest(capability, "0.5.0") != capabilitycatalog.Digest(reversed, "0.5.0") {
		t.Fatal("presentation profile order changed the canonical digest")
	}
}
