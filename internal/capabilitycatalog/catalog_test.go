package capabilitycatalog_test

import (
	"regexp"
	"testing"

	"github.com/limecloud/contentcloud/internal/capabilitycatalog"
	"github.com/limecloud/contentcloud/internal/domain"
)

func TestBuiltinsUseDeterministicSHA256Digests(t *testing.T) {
	first := capabilitycatalog.Builtins("0.7.0")
	second := capabilitycatalog.Builtins("0.7.0")
	if len(first) != 1 || len(second) != len(first) {
		t.Fatalf("builtin capabilities = %#v", first)
	}
	pattern := regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	for index := range first {
		if first[index].Digest != second[index].Digest || !pattern.MatchString(first[index].Digest) {
			t.Fatalf("capability digest is not deterministic SHA-256: %#v", first[index])
		}
	}
	changed, ok := capabilitycatalog.Exact(domain.KnowledgeExtractCapability, "0.5.1")
	if !ok || changed.Digest == first[0].Digest {
		t.Fatal("implementation release version must be bound into the capability digest")
	}
}

func TestDigestCanonicalizesPresentationProfiles(t *testing.T) {
	capability, ok := capabilitycatalog.Exact(domain.KnowledgeExtractCapability, "0.7.0")
	if !ok {
		t.Fatal("knowledge capability missing")
	}
	reversed := capability
	reversed.PresentationProfiles = []string{"cloud_native"}
	if capabilitycatalog.Digest(capability, "0.7.0") != capabilitycatalog.Digest(reversed, "0.7.0") {
		t.Fatal("presentation profile order changed the canonical digest")
	}
}
