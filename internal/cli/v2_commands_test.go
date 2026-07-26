package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

func TestPublishPreflightAllowsBlockedScriptOnlyWithReasons(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	direction := localworkspace.CreativeDirection{ID: "direction:1", Title: "方向", Angle: "角度", HookType: "场景", VisualMotif: "画面", Narrative: []string{"开始"}, Tone: "克制", TargetEmotion: "期待", RiskRefs: []string{}, Status: "selected"}
	batch := localworkspace.CreativeBatch{ID: "batch-1", Kind: "creative_batch", Status: "ready", SchemaVersion: "2.0", ProjectID: "project-1", BriefVersionID: "brief:1", ContextSnapshotID: "context:1", DirectionIDs: []string{direction.ID}, VariantDimension: "hook"}
	batchRoot := filepath.Join(root, "outputs", "scripts", batch.ID)
	writeJSONFixture(t, filepath.Join(batchRoot, "batch.json"), batch)
	path := filepath.Join(batchRoot, "script-blocked.json")
	blocked := localworkspace.ScriptPackageV2{
		ID: "script-version:blocked", Kind: "script_package", Status: "blocked", SchemaVersion: "2.0", Deliverability: "blocked", ProjectID: "project-1", ScriptID: "script:blocked", CreativeBatchID: batch.ID, BriefVersionID: batch.BriefVersionID, ContextSnapshotID: batch.ContextSnapshotID,
		Direction: direction, Title: "待补资料", Channel: "douyin", DurationMS: 1000, AspectRatio: "9:16",
		Cover:              localworkspace.ScriptCover{Title: "待补资料", VisualIntent: "产品画面", FirstViewSignal: "产品", AssetRefs: []string{}, RightsRefs: []string{}, SafeArea: "中央", OcclusionGuards: []string{}},
		NarrativeStructure: []localworkspace.NarrativeSegment{}, Shots: []localworkspace.ScriptShotV2{}, Citations: []localworkspace.ScriptCitationV2{}, AssetRequirements: []localworkspace.ScriptAssetRequirement{},
		Experiment:        localworkspace.ScriptExperiment{PrimaryVariable: "hook", ControlledVariables: []string{}, Hypothesis: "待验证", MeasurementWindow: "24h", TargetMetrics: []string{}},
		GlobalConstraints: localworkspace.ScriptGlobalConstraints{ForbiddenClaims: []string{}, BrandRules: []string{}, ProductTruthRules: []string{}, ContinuityLocks: []string{}, PlatformSafeAreaRules: []string{}},
		BlockedReasons:    []localworkspace.ScriptBlockedReason{{Code: "ASSET_MISSING", Message: "缺少产品实拍", OwnerRole: "客户", NextAction: "补充素材"}}, MissingInputs: []string{"产品实拍"},
	}
	writeJSONFixture(t, path, blocked)
	bundle, preflight, err := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: "script"})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.SchemaVersion != localworkspace.ScriptPackageV2Schema || preflight.BlockedCount != 1 {
		t.Fatalf("unexpected V2 script preflight: %+v %+v", bundle, preflight)
	}
	secondPath := filepath.Join(batchRoot, "script-blocked-2.json")
	second := blocked
	second.ID = "script-version:blocked-2"
	second.ScriptID = "script:blocked-2"
	writeJSONFixture(t, secondPath, second)
	if _, _, err := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: "script"}); err == nil {
		t.Fatal("multiple discovered scripts must require explicit --file scope")
	}
	blocked.BlockedReasons = []localworkspace.ScriptBlockedReason{}
	writeJSONFixture(t, path, blocked)
	if _, _, err := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: "script", Files: []string{filepath.ToSlash(filepath.Join("outputs", "scripts", batch.ID, "script-blocked.json"))}}); err == nil {
		t.Fatal("blocked script without blocked_reasons must be rejected")
	}
}

func TestPublishPreflightRejectsBriefThatSkippedLocalLint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "outputs", "briefs", "invalid.json")
	writeJSONFixture(t, path, map[string]any{"id": "brief:invalid", "kind": "brief", "schema_version": "2.0", "status": "candidate", "deliverability": "review_ready", "objective": "产品认知", "audience": "旅行者"})
	if _, _, err := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: "brief", Files: []string{"outputs/briefs/invalid.json"}}); err == nil {
		t.Fatal("brief publish must reuse the full local Brief V2 lint")
	}
}

func TestPublishReadersRejectSymlinksOutsideWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"id":"fact:outside","kind":"fact"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "outside.json")
	if err := os.Symlink(outside, linked); err != nil {
		t.Skipf("当前文件系统不支持符号链接：%v", err)
	}

	_, _, _, _, err := readPublishObjects(root, "knowledge", []string{linked})
	assertCLIErrorCode(t, err, "PUBLISH_PATH_OUTSIDE_WORKSPACE")
	_, _, err = readDisclosures(root, linked)
	assertCLIErrorCode(t, err, "DISCLOSURE_PATH_OUTSIDE_WORKSPACE")
	if err := os.MkdirAll(filepath.Join(root, "outputs", "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	scriptLink := filepath.Join(root, "outputs", "scripts", "outside.json")
	if err := os.Symlink(outside, scriptLink); err != nil {
		t.Skipf("当前文件系统不支持符号链接：%v", err)
	}
	_, err = resolvePublishFiles(root, "script", nil)
	assertCLIErrorCode(t, err, "LOCAL_FILE_OUTSIDE_WORKSPACE")
}

func assertCLIErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func writeJSONFixture(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
