package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

func TestPublishPlanIDIsStableAndBindsExactInputs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	packPath := filepath.Join(root, "knowledge", "packs", "knowledge.json")
	writeJSONFixture(t, packPath, []map[string]any{{"id": "fact-1", "kind": "fact", "status": "verified"}})
	base := publishBuildOptions{Root: root, SubmissionType: "knowledge", Files: []string{packPath}}
	_, first, err := buildPublishCheckpoint(base)
	if err != nil {
		t.Fatal(err)
	}
	_, replayed, err := buildPublishCheckpoint(base)
	if err != nil {
		t.Fatal(err)
	}
	if first.PlanID != replayed.PlanID || !strings.HasPrefix(first.PlanID, "pp_") {
		t.Fatalf("publish plan_id is not deterministic: first=%s replay=%s", first.PlanID, replayed.PlanID)
	}

	assertChanged := func(name string, options publishBuildOptions) {
		t.Helper()
		_, changed, err := buildPublishCheckpoint(options)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if changed.PlanID == first.PlanID {
			t.Fatalf("%s did not invalidate plan_id %s", name, first.PlanID)
		}
	}
	withMessage := base
	withMessage.Message = "请重点审核来源范围"
	assertChanged("message change", withMessage)
	withIdempotencyKey := base
	withIdempotencyKey.IdempotencyKey = "knowledge:manual-review-2"
	assertChanged("idempotency key change", withIdempotencyKey)

	disclosuresPath := filepath.Join(root, "knowledge", "packs", "source-disclosures.json")
	writeJSONFixture(t, disclosuresPath, []domain.SourceDisclosure{{SourceRef: "source-1", Level: "metadata_only", SHA256: strings.Repeat("a", 64)}})
	withDisclosures := base
	withDisclosures.DisclosuresFile = disclosuresPath
	assertChanged("disclosure change", withDisclosures)

	writeJSONFixture(t, packPath, []map[string]any{{"id": "fact-2", "kind": "fact", "status": "verified"}})
	assertChanged("business file change", base)
	writeJSONFixture(t, packPath, []map[string]any{{"id": "fact-1", "kind": "fact", "status": "verified"}})
	status, err := localworkspace.LoadStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	status.Template.CLIVersion = "changed-environment"
	writeJSONFixture(t, filepath.Join(root, ".contentcloud", "template.lock"), status.Template)
	assertChanged("environment change", base)
}

func TestPublishCLIRejectsMissingOrStalePlanBeforeCloudWrite(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		t.Fatalf("publish validation unexpectedly reached the server: %s", request.URL.Path)
	}))
	defer server.Close()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", ServerURL: server.URL, Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	writeJSONFixture(t, filepath.Join(root, "knowledge", "packs", "knowledge.json"), []map[string]any{{"id": "fact-1", "kind": "fact", "status": "verified"}})
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_test_workspace")

	for _, test := range []struct {
		name string
		args []string
		code string
	}{
		{name: "missing plan", args: []string{"--json", "publish", "knowledge", "--yes"}, code: "PUBLISH_PLAN_ID_REQUIRED"},
		{name: "stale plan", args: []string{"--json", "publish", "knowledge", "--yes", "--plan-id", "pp_" + strings.Repeat("0", 64)}, code: "PUBLISH_PLAN_STALE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := (&Root{stdout: &stdout, stderr: &stderr}).command()
			command.SetArgs(test.args)
			assertCLIErrorCode(t, command.Execute(), test.code)
		})
	}
	if requests != 0 {
		t.Fatalf("publish validation performed %d cloud writes", requests)
	}
}

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
