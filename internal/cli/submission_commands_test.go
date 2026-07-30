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
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
	"gopkg.in/yaml.v3"
)

func TestPublishPlanIDIsStableAndBindsExactInputs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	packPath := filepath.Join(root, "30-knowledge", "packs", "knowledge.json")
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

	disclosuresPath := filepath.Join(root, "30-knowledge", "packs", "source-disclosures.json")
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
	writeJSONFixture(t, filepath.Join(root, "30-knowledge", "packs", "knowledge.json"), []map[string]any{{"id": "fact-1", "kind": "fact", "status": "verified"}})
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

func TestPublishPreflightUsesContentBatchManifestAndAllowsBlockedItems(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	direction := localworkspace.CreativeDirection{ID: "direction:1", Title: "方向", Angle: "角度", HookType: "场景", VisualMotif: "画面", Narrative: []string{"开始"}, Tone: "克制", TargetEmotion: "期待", RiskRefs: []string{}, Status: "selected"}
	batch := localworkspace.ContentBatch{ID: "batch-1", IntentID: "intent:content", ContentKind: domain.ContentTypeVideoScript, ContentSchemaRef: localworkspace.ContentItemSchema, DeliveryProfiles: []string{"json", "markdown", "xlsx"}, Status: "blocked", SchemaVersion: localworkspace.ContentBatchSchema, Publishable: false, BriefRef: "brief:1", KnowledgeSnapshotRefs: []string{"snapshot:knowledge"}, ContentItemRefs: []string{}, BlockedReasons: []string{"批次包含 blocked ContentItem"}, Checks: []localworkspace.ContentBatchCheck{{Name: "content_item_lint", Status: "passed"}}}
	batchRoot := filepath.Join(root, "50-production", "batches", batch.ID)
	path := filepath.Join(batchRoot, "content-blocked.json")
	relativePath := filepath.ToSlash(filepath.Join("50-production", "batches", batch.ID, "content-blocked.json"))
	batch.ContentItemRefs = []string{relativePath}
	writeYAMLFixture(t, filepath.Join(batchRoot, "manifest.yaml"), batch)
	writeJSONFixture(t, filepath.Join(batchRoot, "context.json"), localworkspace.LocalContentContext{SchemaVersion: localworkspace.ContentContextSchema, Batch: batch, ProjectID: "project-1", BriefSnapshotID: "snapshot:brief", ContextSnapshotID: "context:1", DirectionIDs: []string{direction.ID}, RequestedCount: 1, VariantDimension: "hook", ControlledDimensions: []string{}, ContentKind: domain.ContentTypeVideoScript, ContentSchemaRef: localworkspace.ContentItemSchema, DeliveryProfiles: []string{"json", "markdown", "xlsx"}, ContentHash: "sha256:" + strings.Repeat("a", 64), GeneratedAt: time.Now().UTC()})
	blocked := localworkspace.ContentItem{
		ID: "content-item:blocked", Type: "content_item", Status: "blocked", SchemaVersion: localworkspace.ContentItemSchema, Deliverability: "blocked", ProjectID: "project-1", ContentID: "content:blocked", ContentBatchID: batch.ID, BriefRef: batch.BriefRef, ContextSnapshotID: "context:1",
		Direction: direction, Title: "待补资料", Channel: "douyin", DurationMS: 1000, AspectRatio: "9:16",
		Cover:              localworkspace.ContentCover{Title: "待补资料", VisualIntent: "产品画面", FirstViewSignal: "产品", AssetRefs: []string{}, RightsRefs: []string{}, SafeArea: "中央", OcclusionGuards: []string{}},
		NarrativeStructure: []localworkspace.ContentNarrativeSegment{}, Shots: []localworkspace.ContentShot{}, Citations: []localworkspace.ContentCitation{}, AssetRequirements: []localworkspace.ContentAssetRequirement{},
		Experiment:        localworkspace.ContentExperiment{PrimaryVariable: "hook", ControlledVariables: []string{}, Hypothesis: "待验证", MeasurementWindow: "24h", TargetMetrics: []string{}},
		GlobalConstraints: localworkspace.ContentGlobalConstraints{ForbiddenClaims: []string{}, BrandRules: []string{}, ProductTruthRules: []string{}, ContinuityLocks: []string{}, PlatformSafeAreaRules: []string{}},
		BlockedReasons:    []localworkspace.ContentBlockedReason{{Code: "ASSET_MISSING", Message: "缺少产品实拍", OwnerRole: "客户", NextAction: "补充素材"}}, MissingInputs: []string{"产品实拍"},
	}
	writeJSONFixture(t, path, blocked)
	bundle, preflight, err := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: "content_batch"})
	if err != nil {
		t.Fatal(err)
	}
	if preflight.SchemaVersion != "contentcloud.content_batch/3.0" || bundle.EnvironmentDigest == "" || len(bundle.Objects) != 1 || preflight.BlockedCount != 1 {
		t.Fatalf("unexpected V3 content batch preflight: %+v %+v", bundle, preflight)
	}
	secondPath := filepath.Join(batchRoot, "content-blocked-2.json")
	second := blocked
	second.ID = "content-item:blocked-2"
	second.ContentID = "content:blocked-2"
	writeJSONFixture(t, secondPath, second)
	batch.ContentItemRefs = append(batch.ContentItemRefs, filepath.ToSlash(filepath.Join("50-production", "batches", batch.ID, "content-blocked-2.json")))
	writeYAMLFixture(t, filepath.Join(batchRoot, "manifest.yaml"), batch)
	bundle, preflight, err = buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: "content_batch"})
	if err != nil || len(bundle.Objects) != 2 || preflight.BlockedCount != 2 {
		t.Fatalf("manifest did not define the exact multi-item publish scope: bundle=%+v preflight=%+v err=%v", bundle, preflight, err)
	}
	blocked.BlockedReasons = []localworkspace.ContentBlockedReason{}
	writeJSONFixture(t, path, blocked)
	if _, _, err := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: "content_batch", Files: []string{relativePath}}); err == nil {
		t.Fatal("blocked ContentItem without blocked_reasons must be rejected")
	}
}

func TestPublishPreflightRejectsBriefThatSkippedLocalLint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "50-production", "briefs", "invalid.json")
	writeJSONFixture(t, path, map[string]any{"id": "brief:invalid", "kind": "brief", "schema_version": localworkspace.BriefSchema, "status": "candidate", "deliverability": "review_ready", "objective": "产品认知", "audience": "旅行者"})
	if _, _, err := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: "brief", Files: []string{"50-production/briefs/invalid.json"}}); err == nil {
		t.Fatal("brief publish must reuse the full local V3 Brief lint")
	}
}

func TestStrategyPublishPreflightIncludesApprovedTaxonomyBaseline(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	taxonomy := domain.AudienceTaxonomySnapshot{
		ID: "taxonomy-1", Type: "audience_taxonomy_snapshot", SchemaVersion: domain.AudienceTaxonomySchema,
		Provider: "oceanengine_yuntu", TaxonomyID: "douyin-commerce-eight-audiences", TaxonomyVersion: now.Format("2006-01-02"),
		Segments: domain.DefaultDouyinAudienceSegments(), SourceURL: "https://school.oceanengine.com/", CapturedAt: now,
		EffectiveFrom: now, ExpiresAt: now.Add(30 * 24 * time.Hour), VerificationStatus: "human_verified", SourceSHA256: strings.Repeat("a", 64), Status: "review_ready",
	}
	canonical, err := json.Marshal(map[string]any{"schema_version": "contentcloud.strategy/3.0", "submission_type": "strategy", "objects": []any{taxonomy}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := domain.ApprovedSnapshot{
		ID: "taxonomy-snapshot", ProjectID: "project-1", WorkspaceID: "workspace-1", SubmissionType: "strategy", SchemaVersion: "contentcloud.strategy/3.0",
		CanonicalContent: canonical, EligibleIDs: []string{taxonomy.ID}, CreatedAt: now,
	}
	if _, err := localworkspace.StoreApprovedSnapshots(root, []domain.ApprovedSnapshot{snapshot}, now); err != nil {
		t.Fatal(err)
	}
	strategy := domain.AudienceStrategyVersion{
		ID: "strategy-1", Type: "audience_strategy_version", SchemaVersion: domain.AudienceStrategySchema, ProjectID: "project-1",
		TaxonomySnapshotID: taxonomy.ID, AudienceCode: taxonomy.Segments[0].Code, AudienceLabel: taxonomy.Segments[0].Label, SegmentDefinition: taxonomy.Segments[0].Definition,
		Objective: "conversion", DemandMoment: "通勤", InsightStatement: "有证据的洞察", HookHypotheses: []string{"场景钩子"}, Scenario: "通勤",
		ProofOrder: []string{"规格"}, Objections: []string{"体积"}, CTAStrategy: "查看详情", EvidenceRefs: []string{"evidence-1"}, Confidence: "medium",
		TestType: "audience_expression_fit_test", PrimaryVariable: "audience", ControlledVariables: []string{"cta"}, TargetMetrics: []string{"ctr"}, Constraints: []string{}, Status: "review_ready",
	}
	strategyPath := filepath.Join(root, "50-production", "strategies", "strategy-1.json")
	writeJSONFixture(t, strategyPath, strategy)
	bundle, preflight, err := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: "strategy", Files: []string{strategyPath}})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.BaseSnapshotIDs) != 1 || bundle.BaseSnapshotIDs[0] != snapshot.ID || len(preflight.BaseSnapshotIDs) != 1 || preflight.BaseSnapshotIDs[0] != snapshot.ID {
		t.Fatalf("strategy preflight omitted taxonomy baseline: bundle=%v preflight=%v", bundle.BaseSnapshotIDs, preflight.BaseSnapshotIDs)
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
	batchRoot := filepath.Join(root, "50-production", "batches", "outside")
	if err := os.MkdirAll(batchRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestLink := filepath.Join(batchRoot, "manifest.yaml")
	if err := os.Symlink(outside, manifestLink); err != nil {
		t.Skipf("当前文件系统不支持符号链接：%v", err)
	}
	_, err = resolvePublishFiles(root, "content_batch", nil)
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

func writeYAMLFixture(t *testing.T, path string, value any) {
	t.Helper()
	body, err := yaml.Marshal(value)
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
