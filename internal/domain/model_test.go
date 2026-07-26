package domain

import (
	"testing"
	"time"
)

func TestCompileSnapshotIsDeterministic(t *testing.T) {
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	project := Project{ID: NewID(), TenantID: NewID()}
	brief := BriefVersion{ID: NewID(), ContentHash: "brief-hash"}
	a := KnowledgeItem{ID: NewID(), RowVersion: 2}
	b := KnowledgeItem{ID: NewID(), RowVersion: 3}
	first, err := CompileSnapshot(project, brief, []KnowledgeItem{a, b}, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileSnapshot(project, brief, []KnowledgeItem{b, a}, now.Add(time.Hour))
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

func TestValidateScriptRejectsUnknownKnowledge(t *testing.T) {
	knowledgeID := NewID()
	contract := TaskContract{Brief: BriefVersion{Channel: "douyin", PrimarySellingPoint: "daily ritual", PrimaryTestVariable: "hook", VisualizationPlanIDs: []string{"plan-1"}}, Knowledge: []KnowledgeItem{{ID: knowledgeID, Status: "approved"}}}
	pkg := minimalPackage(knowledgeID)
	pkg.Shots[0].KnowledgeRefs = []string{NewID()}
	report := ValidateScript(pkg, contract)
	if report.Valid {
		t.Fatal("expected unknown knowledge ref to fail")
	}
	assertIssue(t, report, "KNOWLEDGE_REF_NOT_ALLOWED")
}

func TestValidateScriptAcceptsCompleteTimeline(t *testing.T) {
	knowledgeID := NewID()
	contract := TaskContract{Brief: BriefVersion{Channel: "douyin", PrimarySellingPoint: "daily ritual", PrimaryTestVariable: "hook", VisualizationPlanIDs: []string{"plan-1"}}, Knowledge: []KnowledgeItem{{ID: knowledgeID, Status: "approved"}}}
	report := ValidateScript(minimalPackage(knowledgeID), contract)
	if !report.Valid {
		t.Fatalf("expected valid package, got %#v", report.Errors)
	}
}

func TestValidateScriptRejectsAssetOutsideRightsSnapshot(t *testing.T) {
	knowledgeID := "knowledge-1"
	contract := TaskContract{
		Brief:     BriefVersion{Channel: "douyin", PrimarySellingPoint: "daily ritual", PrimaryTestVariable: "hook", VisualizationPlanIDs: []string{"plan-1"}},
		Knowledge: []KnowledgeItem{{ID: knowledgeID, Status: "approved"}},
		Assets:    []AssetBundle{{Asset: Asset{ID: "allowed-asset", Status: "approved"}, Rights: RightsRecord{ID: "rights-1", Status: "approved"}}},
	}
	pkg := minimalPackage(knowledgeID)
	pkg.ProductionBible.AssetIDs = []string{"unlicensed-asset"}
	report := ValidateScript(pkg, contract)
	if report.Valid {
		t.Fatalf("expected asset rights failure, got %#v", report)
	}
	assertIssue(t, report, "ASSET_RIGHTS_BLOCKED")
}

func minimalPackage(knowledgeID string) ScriptPackage {
	roles := []string{"hook", "product_solution", "proof", "cta"}
	shots := make([]Shot, 0, len(roles))
	for i, role := range roles {
		planID := ""
		if role == "proof" {
			planID = "plan-1"
		}
		shots = append(shots, Shot{ShotID: "SH0" + string(rune('1'+i)), StartMS: i * 2500, EndMS: (i + 1) * 2500, Role: role, NarrativePurpose: "move decision", Subject: "product", VisualIntent: "visible action", SubjectAction: "hand moves product", Composition: "medium", CameraMotion: "static", FirstFrame: FrameSpec{VisualState: "start", PromptZH: "start"}, MotionSpec: "move", EndFrame: FrameSpec{VisualState: "end", PromptZH: "end"}, SoundIntent: "room", KnowledgeRefs: []string{knowledgeID}, NegativeConstraints: []string{"no drift"}, Continuity: Continuity{}, ProductTruthStrategy: "real_asset_composite", VisualizationPlanID: planID, AcceptanceCriteria: []string{"product remains legible"}})
	}
	return ScriptPackage{SchemaVersion: "1.1", Deliverability: "review_ready", Title: "test", Channel: "douyin", TargetDurationSeconds: 10, AspectRatio: "9:16", CreativeStrategy: CreativeStrategy{PrimarySellingPoint: "daily ritual", PrimaryTestVariable: "hook"}, ProductionBible: ProductionBible{}, Narrative: roles, Shots: shots, Citations: []Citation{}, BlockedReasons: []BlockReason{}, MissingInputs: []string{}}
}
func assertIssue(t *testing.T, report ValidationReport, code string) {
	t.Helper()
	for _, issue := range report.Errors {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("missing issue %s in %#v", code, report.Errors)
}
