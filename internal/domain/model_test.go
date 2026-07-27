package domain

import (
	"testing"
	"time"
)

func TestCompileSnapshotIsDeterministic(t *testing.T) {
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	project := Project{ID: NewID(), TenantID: NewID()}
	a := ContractSource{SourceID: NewID(), RevisionID: NewID(), SHA256: "sha256:a"}
	b := ContractSource{SourceID: NewID(), RevisionID: NewID(), SHA256: "sha256:b"}
	first, err := CompileKnowledgeSnapshot(project, []ContractSource{a, b}, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileKnowledgeSnapshot(project, []ContractSource{b, a}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if first.ManifestHash != second.ManifestHash {
		t.Fatalf("manifest hash changed with ordering or time: %s != %s", first.ManifestHash, second.ManifestHash)
	}
	if first.ID == second.ID {
		t.Fatal("new snapshot must keep a distinct immutable identity")
	}
}
