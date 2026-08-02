package domain

import (
	"testing"
	"time"
)

func TestKnowledgeSnapshotQuerySeparatesEligibleBlockedAndGaps(t *testing.T) {
	now := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	objects := []KnowledgeObject{
		{ID: "fact:weight", TenantID: "tenant-1", ProjectID: "project-1", ObjectType: "FactAssertion", Layer: "product", Version: 1, Status: "verified", Title: "净重", EvidenceRefs: []string{"evidence:weight"}},
		{ID: "claim:portable", TenantID: "tenant-1", ProjectID: "project-1", ObjectType: "Claim", Layer: "expression", Version: 1, Status: "candidate", Title: "轻便", EvidenceRefs: []string{"evidence:portable"}},
		{ID: "claim:conflicted", TenantID: "tenant-1", ProjectID: "project-1", ObjectType: "Claim", Layer: "expression", Version: 1, Status: "approved", Title: "冲突表达", EvidenceRefs: []string{"evidence:conflict"}, ConflictRefs: []string{"conflict:price"}},
		{ID: "conflict:price", TenantID: "tenant-1", ProjectID: "project-1", ObjectType: "ConflictRecord", Layer: "compliance", Version: 1, Status: "open", Title: "价格冲突"},
		{ID: "rights:image", TenantID: "tenant-1", ProjectID: "project-1", ObjectType: "RightsRecord", Layer: "compliance", Version: 1, Status: "expired", Title: "素材权利"},
		{ID: "asset:hero", TenantID: "tenant-1", ProjectID: "project-1", ObjectType: "Asset", Layer: "compliance", Version: 1, Status: "approved", Title: "主图", EvidenceRefs: []string{"evidence:asset"}, RightsRefs: []string{"rights:image"}},
		{ID: "gap:audience", TenantID: "tenant-1", ProjectID: "project-1", ObjectType: "KnowledgeGap", Layer: "market", Version: 1, Status: "open", Title: "缺少受众证据", NextAction: "REQUEST_SOURCE", Impact: "阻断新品首发脚本"},
	}
	pack := KnowledgePack{ID: "pack:content", TenantID: "tenant-1", ProjectID: "project-1", Name: "内容知识包", Purpose: "content", Version: 1, Status: "published", ObjectRefs: make([]KnowledgePackObjectRef, 0, len(objects)), QueryPolicy: DefaultKnowledgeQueryPolicy()}
	for _, object := range objects {
		pack.ObjectRefs = append(pack.ObjectRefs, KnowledgePackObjectRef{ObjectID: object.ID, Version: object.Version})
	}
	digest, err := pack.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	pack.Digest = digest
	snapshot, err := BuildKnowledgeSnapshot(pack, objects, now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := EvaluateKnowledgeSnapshot(snapshot, pack.QueryPolicy, KnowledgeQuery{SnapshotID: snapshot.ID, Channel: "short_video", At: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Eligible) != 1 || result.Eligible[0].ObjectID != "fact:weight" {
		t.Fatalf("unexpected eligible result: %#v", result.Eligible)
	}
	if len(result.Blocked) != 5 {
		t.Fatalf("unexpected blocked result: %#v", result.Blocked)
	}
	if len(result.Gaps) != 1 || result.Gaps[0].ObjectID != "gap:audience" {
		t.Fatalf("unexpected gaps result: %#v", result.Gaps)
	}
	second, err := EvaluateKnowledgeSnapshot(snapshot, pack.QueryPolicy, KnowledgeQuery{SnapshotID: snapshot.ID, Channel: "short_video", At: now})
	if err != nil || result.QueryDigest != second.QueryDigest {
		t.Fatalf("same query must have stable digest: %#v %#v", result, second)
	}
}

func TestKnowledgeSnapshotFreezesObjectVersion(t *testing.T) {
	now := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	first := KnowledgeObject{ID: "fact:name", TenantID: "tenant-1", ProjectID: "project-1", ObjectType: "FactAssertion", Layer: "identity", Version: 1, Status: "verified", Title: "名称", Statement: "旧名称", EvidenceRefs: []string{"evidence:name"}}
	pack := KnowledgePack{ID: "pack:identity", TenantID: "tenant-1", ProjectID: "project-1", Name: "身份包", Purpose: "identity", Version: 1, Status: "published", ObjectRefs: []KnowledgePackObjectRef{{ObjectID: first.ID, Version: first.Version}}, QueryPolicy: DefaultKnowledgeQueryPolicy()}
	digest, _ := pack.ContentDigest()
	pack.Digest = digest
	snapshot, err := BuildKnowledgeSnapshot(pack, []KnowledgeObject{first}, now)
	if err != nil {
		t.Fatal(err)
	}
	updated := first
	updated.Version = 2
	updated.Statement = "新名称"
	if snapshot.Objects[0].Version != 1 || snapshot.Objects[0].Statement != "旧名称" {
		t.Fatalf("snapshot was changed by later object version: %#v", snapshot.Objects[0])
	}
	if updated.Statement == snapshot.Objects[0].Statement {
		t.Fatal("fixture update did not change object")
	}
}
