package capabilityrouting

import (
	"strings"
	"testing"
)

func TestManagedBlockInspectionAndUpdatePreserveUserContent(t *testing.T) {
	userContent := "# 用户规则\n\n保留这段正文。\n"
	updated, err := UpdateManagedBlock(userContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updated, userContent) || Inspect(updated).Status != "current" {
		t.Fatalf("unexpected managed document:\n%s", updated)
	}
	outdated := strings.Replace(updated, "version="+Version, "version=0.0.1", 1)
	if Inspect(outdated).Status != "outdated" {
		t.Fatalf("expected outdated inspection: %+v", Inspect(outdated))
	}
	repaired, err := UpdateManagedBlock(outdated)
	if err != nil {
		t.Fatal(err)
	}
	if repaired != updated || Inspect(repaired).Status != "current" {
		t.Fatalf("repair drifted user content:\n%s", repaired)
	}
}

func TestUpdateManagedBlockRejectsMalformedOwnedBlock(t *testing.T) {
	_, err := UpdateManagedBlock("before\n" + startPrefix + "version=broken -->\n")
	if err == nil {
		t.Fatal("expected malformed managed block error")
	}
}
