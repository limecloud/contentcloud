package localconfig

import (
	"path/filepath"
	"testing"
)

func TestLocalArtifactIndexRoundTrip(t *testing.T) {
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	expected := LocalArtifact{ArtifactID: "artifact-1", Path: "/private/local-only/file.bin", SHA256: "abc", ByteSize: 42}
	if err := SaveLocalArtifact(expected); err != nil {
		t.Fatal(err)
	}
	actual, err := LocalArtifactByID(expected.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("unexpected local artifact index value: %#v", actual)
	}
}
