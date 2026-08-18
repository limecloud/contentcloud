package localworkspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/work"
)

func TestAudienceStrategyScaffoldRequiresPulledTaxonomyAndProducesCandidates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC)
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ScaffoldAudienceStrategies(ScaffoldAudienceStrategiesOptions{Root: root, TaxonomySnapshotID: "taxonomy-1", Mode: "single", AudienceCodes: []string{"gen_z"}, Objective: "conversion"}); err == nil {
		t.Fatal("strategy scaffold accepted a taxonomy that was not pulled from the server")
	}
	taxonomy := work.AudienceTaxonomySnapshot{
		ID: "taxonomy-1", Type: "audience_taxonomy_snapshot", SchemaVersion: work.AudienceTaxonomySchema,
		Provider: "oceanengine_yuntu", TaxonomyID: "douyin-commerce-eight-audiences", TaxonomyVersion: "2026-07-29",
		Segments: work.DefaultDouyinAudienceSegments(), SourceURL: "https://school.oceanengine.com/", CapturedAt: now,
		EffectiveFrom: now, ExpiresAt: now.Add(90 * 24 * time.Hour), VerificationStatus: "human_verified", SourceSHA256: strings.Repeat("a", 64), Status: "review_ready",
	}
	incompleteTaxonomy := taxonomy
	incompleteTaxonomy.Segments = incompleteTaxonomy.Segments[:7]
	assertV5DomainCode(t, incompleteTaxonomy.Validate(now, true), "AUDIENCE_TAXONOMY_SEGMENTS_INVALID")
	storeApprovedObject(t, root, "taxonomy-snapshot", "strategy", taxonomy.ID, taxonomy, now)
	paths, strategies, err := ScaffoldAudienceStrategies(ScaffoldAudienceStrategiesOptions{Root: root, TaxonomySnapshotID: taxonomy.ID, Mode: "compare", AudienceCodes: []string{"gen_z", "refined_mothers"}, Objective: "conversion"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || len(strategies) != 2 || strategies[0].Status != "candidate" || strategies[1].Status != "candidate" {
		t.Fatalf("unexpected strategy candidates: paths=%v values=%+v", paths, strategies)
	}
	strategy := strategies[0]
	strategy.DemandMoment = "通勤前快速决策"
	strategy.InsightStatement = "项目评论研究显示用户优先关注便携性"
	strategy.HookHypotheses = []string{"首秒展示通勤收纳场景"}
	strategy.Scenario = "早高峰通勤"
	strategy.ProofOrder = []string{"已批准规格", "真实收纳演示"}
	strategy.Objections = []string{"是否占空间"}
	strategy.CTAStrategy = "查看当前商品详情"
	strategy.EvidenceRefs = []string{"evidence:comments-1"}
	strategy.Confidence = "medium"
	strategy.ControlledVariables = []string{"hook", "cta"}
	strategy.TargetMetrics = []string{"product_click_rate"}
	strategy.Constraints = []string{"不推断收入"}
	strategy.Status = "review_ready"
	if err := replaceJSON(filepath.Join(root, filepath.FromSlash(paths[0])), strategy, 0o600); err != nil {
		t.Fatal(err)
	}
	report, _, err := LintAudienceStrategy(root, paths[0], now)
	if err != nil || !report.Valid {
		t.Fatalf("review-ready strategy lint failed: %+v %v", report, err)
	}
	strategy.AudienceLabel = "被篡改的人群名称"
	if err := replaceJSON(filepath.Join(root, filepath.FromSlash(paths[0])), strategy, 0o600); err != nil {
		t.Fatal(err)
	}
	report, _, err = LintAudienceStrategy(root, paths[0], now)
	if err != nil || report.Valid || !v5ReportHasCode(report, "AUDIENCE_STRATEGY_TAXONOMY_MISMATCH") {
		t.Fatalf("strategy lint accepted taxonomy drift: %+v %v", report, err)
	}
	_, explored, err := ScaffoldAudienceStrategies(ScaffoldAudienceStrategiesOptions{Root: root, TaxonomySnapshotID: taxonomy.ID, Mode: "explore", Objective: "conversion"})
	if err != nil || len(explored) != 8 {
		t.Fatalf("explore must create eight lightweight candidates: count=%d err=%v", len(explored), err)
	}
}

func TestStoryboardApprovalBoundaryAndSeedanceExport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	item := validContentItem(ContentBatch{ID: "batch-1", ProjectID: "project-1", BriefRef: "brief-1", ContextSnapshotID: "context-1"}, CreativeDirection{ID: "direction-1", Status: "selected"}, "fact:1")
	item.Shots = item.Shots[:1]
	item.DurationMS = item.Shots[0].EndMS
	item.Shots[0].RightsRefs = []string{"rights:product-1"}
	storeApprovedObject(t, root, "content-snapshot", "content_batch", item.ID, item, now)
	created, err := CreateStoryboardPackage(CreateStoryboardPackageOptions{
		Root: root, ApprovedSnapshotID: "content-snapshot", ContentItemID: item.ID, PackageID: "storyboard-1",
		Capability: work.CapabilityRef{ID: "image.test", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("b", 64)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Package.Status != "candidate" {
		t.Fatalf("local storyboard must start as candidate: %+v", created.Package)
	}
	shotDirectory := filepath.Join(root, filepath.FromSlash(created.ShotPaths[0]))
	if err := os.WriteFile(filepath.Join(shotDirectory, "first-frame.png"), []byte("first-frame-v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestDirectory := filepath.Dir(filepath.Join(root, filepath.FromSlash(created.ManifestPath)))
	if err := os.WriteFile(filepath.Join(manifestDirectory, "review-sheet.png"), []byte("review-sheet-v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, prepared, err := PrepareStoryboardReview(root, created.ManifestPath)
	if err != nil || !report.Valid {
		t.Fatalf("prepare storyboard failed: %+v %v", report, err)
	}
	if prepared.Status != "review_ready" || prepared.Shots[0].FirstFrameArtifactID == "" {
		t.Fatalf("prepared storyboard did not persist discovered shot media: %+v", prepared)
	}
	tamperedSource := prepared
	tamperedSource.SourceDigest = "sha256:" + strings.Repeat("f", 64)
	assertV5DomainCode(t, validateStoryboardSource(root, tamperedSource), "STORYBOARD_SOURCE_DIGEST_MISMATCH")
	if _, err := LoadLockedStoryboardSnapshot(root, "missing-server-snapshot", prepared.ID); err == nil {
		t.Fatal("local review_ready storyboard was accepted as server-locked")
	}
	storeApprovedObject(t, root, "storyboard-snapshot", "storyboard", prepared.ID, prepared, now.Add(time.Minute))
	locked, err := LoadLockedStoryboardSnapshot(root, "storyboard-snapshot", prepared.ID)
	if err != nil || locked.LockedDigest != prepared.LockedDigest {
		t.Fatalf("pulled storyboard snapshot was not accepted: %+v %v", locked, err)
	}
	exported, err := ExportSeedancePackage(ExportSeedancePackageOptions{
		Root: root, StoryboardSnapshotID: "storyboard-snapshot", StoryboardPackageID: prepared.ID, PackageID: "seedance-1",
		ProviderProfileVersion: "seedance-profile:manual-2026-07-29", AdapterCapability: work.CapabilityRef{ID: "contentcloud.seedance-export", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("c", 64)},
		Mode: "all_reference", AspectRatio: "9:16", Sound: "environment_only", MinDurationSeconds: 4, MaxDurationSeconds: 15, MaxImages: 9, MaxVideos: 3, MaxAudios: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if exported.Package.Status != "validated" || len(exported.Package.UploadManifest) != 1 || exported.Package.UploadManifest[0].Reference != "@图片1" || len(exported.PromptPaths) != 1 {
		t.Fatalf("unexpected Seedance package: %+v", exported)
	}
	prompt, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(exported.PromptPaths[0])))
	if err != nil || !strings.Contains(string(prompt), "声音意图：environment_only") || strings.Count(string(prompt), prepared.Shots[0].Action) != 1 {
		t.Fatalf("copy-ready prompt is incomplete or duplicates the action: %v %s", err, prompt)
	}
	lintReport, _, err := LintSeedancePackage(root, exported.PackagePath)
	if err != nil || !lintReport.Valid {
		t.Fatalf("exported Seedance package did not lint: %+v %v", lintReport, err)
	}
	readme, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(exported.ReadmePath)))
	if err != nil || !strings.Contains(string(readme), "@图片1") || !strings.Contains(string(readme), "Adapter digest") || !strings.Contains(string(readme), "Sound: `environment_only`") || !strings.Contains(string(readme), "验收：") || !strings.Contains(string(readme), "用户在对应外部平台确认") {
		t.Fatalf("operator README is incomplete: %v %s", err, readme)
	}
	dynamicOffer := prepared
	dynamicOffer.ID = "storyboard-dynamic-offer"
	dynamicOffer.Shots = append([]work.StoryboardShot(nil), prepared.Shots...)
	dynamicOffer.Shots[0].Action = "展示到手价99元"
	dynamicOffer.LockedDigest, err = dynamicOffer.ComputedLockedDigest()
	if err != nil {
		t.Fatal(err)
	}
	storeApprovedObject(t, root, "storyboard-offer-snapshot", "storyboard", dynamicOffer.ID, dynamicOffer, now.Add(2*time.Minute))
	_, err = ExportSeedancePackage(ExportSeedancePackageOptions{
		Root: root, StoryboardSnapshotID: "storyboard-offer-snapshot", StoryboardPackageID: dynamicOffer.ID, PackageID: "seedance-offer",
		ProviderProfileVersion: "seedance-profile:manual-2026-07-29", AdapterCapability: work.CapabilityRef{ID: "contentcloud.seedance-export", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("c", 64)},
		Mode: "all_reference", AspectRatio: "9:16", MinDurationSeconds: 4, MaxDurationSeconds: 15, MaxImages: 9, MaxVideos: 3, MaxAudios: 3,
	})
	assertV5DomainCode(t, err, "SEEDANCE_DYNAMIC_OFFER_TEXT_BLOCKED")

	if err := os.WriteFile(filepath.Join(shotDirectory, "first-frame.png"), []byte("first-frame-drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ExportSeedancePackage(ExportSeedancePackageOptions{
		Root: root, StoryboardSnapshotID: "storyboard-snapshot", StoryboardPackageID: prepared.ID, PackageID: "seedance-2",
		ProviderProfileVersion: "seedance-profile:manual-2026-07-29", AdapterCapability: work.CapabilityRef{ID: "contentcloud.seedance-export", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("c", 64)},
		Mode: "all_reference", AspectRatio: "9:16", MinDurationSeconds: 4, MaxDurationSeconds: 15, MaxImages: 9, MaxVideos: 3, MaxAudios: 3,
	})
	assertV5DomainCode(t, err, "STORYBOARD_LOCKED_MEDIA_DRIFT")
}

func TestStoryboardShotIDsCannotEscapeTheirPackage(t *testing.T) {
	shot := work.StoryboardShot{
		ShotID: "../../outside", StartMS: 0, EndMS: 1000, Role: "hook", ImagePromptZH: "首帧", PlanB: "实拍",
		NegativeConstraints: []string{"无文字"}, AcceptanceCriteria: []string{"主体清晰"},
	}
	assertV5DomainCode(t, shot.Validate(nil, false), "STORYBOARD_SHOT_INVALID")

	colonName := storyboardShotDirectoryName("shot:01")
	if strings.ContainsAny(colonName, `:/\\`) || colonName == storyboardShotDirectoryName("shot-01") {
		t.Fatalf("storyboard shot directory names must be portable and collision-resistant: %q", colonName)
	}
}

func assertV5DomainCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainError *fault.Error
	if !errors.As(err, &domainError) || domainError.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func v5ReportHasCode(report V5LintReport, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
