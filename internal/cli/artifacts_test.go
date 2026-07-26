package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectLocalArtifactResolvesPathAndHashesBytes(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "artifact.bin")
	if err := os.WriteFile(target, []byte("artifact bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "artifact-link.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	path, info, hash, err := inspectLocalArtifact(link)
	if err != nil {
		t.Fatal(err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if path != resolvedTarget || info.Size() != 14 || hash != "4659fc0570122b0e0aa14f4ff7c261b1fe51795a01ba79963f462ebf40d7520d" {
		t.Fatalf("unexpected local artifact inspection: path=%q size=%d hash=%s", path, info.Size(), hash)
	}
}

func TestArtifactCommandSchemasExposeComposedWorkflow(t *testing.T) {
	schemas := commandSchemas()
	for _, name := range []string{"artifact.register", "artifact.list", "artifact.presentation", "artifact.open", "artifact.open.status", "artifact.download"} {
		if schemas[name] == nil {
			t.Fatalf("command schema %q is missing", name)
		}
	}
}
