package localworkspace

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestCreativeBatchScriptLintFinalizeAndApprovedExport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	knowledge := LocalKnowledgeItem{ID: "fact:product", Kind: "fact", Status: "candidate", Title: "产品事实", Statement: "产品事实", Subject: "产品", Predicate: "事实", Value: domain.TypedValue{Type: "text", Text: "产品事实"}, Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "low", AllowedChannels: []string{"douyin"}, Evidence: []domain.EvidenceRef{}, EvidenceIDs: []string{}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{}, Dimensions: []string{"category"}, Layers: []string{"product"}}
	if err := replaceJSON(filepath.Join(root, "knowledge", "facts", "fact-product.json"), knowledge, 0o600); err != nil {
		t.Fatal(err)
	}
	storeApprovedObject(t, root, "knowledge-snapshot", "knowledge", knowledge.ID, knowledge, now)

	brief := validLocalBrief(knowledge.ID)
	storeApprovedObject(t, root, "strategy-snapshot", "strategy", brief.StrategyVersionID, map[string]any{"id": brief.StrategyVersionID, "kind": "strategy_version"}, now)
	briefPath := filepath.Join(root, "outputs", "briefs", "brief-1.json")
	if err := replaceJSON(briefPath, brief, 0o600); err != nil {
		t.Fatal(err)
	}
	briefLint, _, err := LintBrief(root, "outputs/briefs/brief-1.json")
	if err != nil || !briefLint.Valid {
		t.Fatalf("brief lint failed: %+v %v", briefLint, err)
	}
	storeApprovedObject(t, root, "brief-snapshot", "brief", brief.ID, brief, now.Add(time.Minute))

	direction := CreativeDirection{ID: "direction:travel", Title: "旅行收尾", Angle: "把旅行记忆带回日常", HookType: "场景直入", VisualMotif: "旅行照片", Narrative: []string{"触发", "产品", "证明", "行动"}, Tone: "克制", TargetEmotion: "留恋", RiskRefs: []string{}, Status: "selected"}
	directionsPath := filepath.Join(root, "work", "directions.json")
	if err := replaceJSON(directionsPath, []CreativeDirection{direction}, 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := CreateCreativeBatch(CreateCreativeBatchOptions{Root: root, BriefID: brief.ID, DirectionsFile: "work/directions.json", RequestedCount: 2, VariantDimension: "hook", ControlledDimensions: []string{"audience", "cta"}, BatchID: "creative-batch-1", Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if created.Batch.Status != "ready" || created.Batch.KnowledgeSnapshotID != "knowledge-snapshot" {
		t.Fatalf("unexpected batch: %+v", created)
	}

	reviewReady := validScriptPackageV2(created.Batch, direction, knowledge.ID)
	reviewPath := filepath.Join(root, "outputs", "scripts", "creative-batch-1", "script-review.json")
	if err := replaceJSON(reviewPath, reviewReady, 0o600); err != nil {
		t.Fatal(err)
	}
	report, _, err := LintScriptPackage(root, relativeWorkspacePath(root, reviewPath), created.BatchPath)
	if err != nil || !report.Valid {
		t.Fatalf("script lint failed: %+v %v", report, err)
	}
	blocked := validBlockedScriptPackageV2(created.Batch, direction)
	blockedPath := filepath.Join(root, "outputs", "scripts", "creative-batch-1", "script-blocked.json")
	if err := replaceJSON(blockedPath, blocked, 0o600); err != nil {
		t.Fatal(err)
	}
	finalized, err := FinalizeCreativeBatch(root, created.BatchPath, []string{relativeWorkspacePath(root, reviewPath), relativeWorkspacePath(root, blockedPath)}, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Batch.Status != "partially_blocked" || finalized.Report.ReviewReady != 1 || finalized.Report.Blocked != 1 {
		t.Fatalf("unexpected finalized batch: %+v", finalized)
	}

	storeApprovedObject(t, root, "script-snapshot", "script", reviewReady.ID, reviewReady, now.Add(4*time.Minute))
	manifest, err := ExportApprovedScript(root, reviewReady.ID, "", now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ApprovedSnapshotID != "script-snapshot" || len(manifest.Files) != 3 {
		t.Fatalf("unexpected delivery manifest: %+v", manifest)
	}
	for _, file := range manifest.Files {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing delivery file %s: %v", file.Path, err)
		}
		if file.Format == "xlsx" {
			reader, err := zip.OpenReader(path)
			if err != nil {
				t.Fatalf("xlsx is not a valid zip: %v", err)
			}
			_ = reader.Close()
		}
	}
}

func TestScriptRevisionDiffRejectsUndeclaredDrift(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	batch := CreativeBatch{ID: "batch", ProjectID: "project-1", BriefVersionID: "brief", ContextSnapshotID: "context"}
	direction := CreativeDirection{ID: "direction", Status: "selected"}
	base := validScriptPackageV2(batch, direction, "fact:product")
	base.ID = "script-version-1"
	candidate := base
	candidate.ID = "script-version-2"
	candidate.BasedOnVersionID = base.ID
	candidate.ChangeSummary = "调整标题，同时意外改变 CTA"
	candidate.Title = "新标题"
	candidate.Shots = append([]ScriptShotV2(nil), base.Shots...)
	candidate.Shots[3].OnScreenText = "新的 CTA"
	basePath := filepath.Join(root, "work", "base.json")
	candidatePath := filepath.Join(root, "work", "candidate.json")
	if err := replaceJSON(basePath, base, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceJSON(candidatePath, candidate, 0o600); err != nil {
		t.Fatal(err)
	}
	diff, err := DiffScriptPackages(root, "work/base.json", "work/candidate.json", []string{"/title"})
	if err != nil {
		t.Fatal(err)
	}
	if diff.Valid || len(diff.UnexpectedPaths) != 1 || diff.UnexpectedPaths[0] != "/shots/3/on_screen_text" {
		t.Fatalf("unexpected diff: %+v", diff)
	}
}

func TestScriptLintRequiresExplicitArraysAndBlockedReasons(t *testing.T) {
	batch := CreativeBatch{ID: "batch", Status: "ready", ProjectID: "project-1", BriefVersionID: "brief", ContextSnapshotID: "context", DirectionIDs: []string{"direction"}, VariantDimension: "hook"}
	direction := CreativeDirection{ID: "direction", Title: "方向", Angle: "角度", HookType: "场景", VisualMotif: "画面", Narrative: []string{"开始"}, Tone: "克制", TargetEmotion: "期待", RiskRefs: []string{}, Status: "selected"}
	blocked := validBlockedScriptPackageV2(batch, direction)
	report := lintScriptPackage(blocked, batch, KnowledgeQueryResult{}, map[string]LocalKnowledgeItem{})
	if !report.Valid {
		t.Fatalf("blocked script with reasons and explicit empty shots must pass: %+v", report)
	}
	blocked.BlockedReasons = []ScriptBlockedReason{}
	report = lintScriptPackage(blocked, batch, KnowledgeQueryResult{}, map[string]LocalKnowledgeItem{})
	if report.Valid || !hasScriptIssue(report, "SCRIPT_BLOCK_REASON_REQUIRED") {
		t.Fatalf("blocked script without reasons must fail: %+v", report)
	}
	blocked = validBlockedScriptPackageV2(batch, direction)
	blocked.Citations = nil
	report = lintScriptPackage(blocked, batch, KnowledgeQueryResult{}, map[string]LocalKnowledgeItem{})
	if report.Valid || !hasScriptIssue(report, "SCRIPT_ARRAY_REQUIRED") {
		t.Fatalf("missing required arrays must fail: %+v", report)
	}
}

func hasScriptIssue(report ScriptLintReport, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func validLocalBrief(knowledgeID string) LocalBrief {
	return LocalBrief{
		ID: "brief:1", Kind: "brief", Status: "candidate", SchemaVersion: "2.0", Deliverability: "review_ready", StrategyVersionID: "strategy:1", CampaignID: "campaign:1", ExperimentID: "experiment:1", Channel: "douyin", Objective: "产品认知", Audience: "南京旅行者", Scenario: "旅行结束", DemandMoment: "选择伴手礼", PainPoint: "普通纪念品缺少日常使用价值", PrimarySellingPoint: "把旅行记忆带回日常", SupportPoints: []string{"产品事实"}, Positioning: "城市文化伴手礼", VisualizationPlanIDs: []string{"visualization:1"}, AssetIDs: []string{}, TruthStrategy: "非产品环境生成，产品细节用实拍", PlanB: "静物实拍", Tone: "克制", BrandRuleIDs: []string{}, ApprovedClaimIDs: []string{}, ForbiddenClaims: []string{"功效承诺"}, HookExpectation: "首秒出现旅行收尾信号", NarrativeConstraints: []string{"单一 CTA"}, CTA: "了解产品", PrimaryVariable: "hook", ControlledVariables: []string{"audience", "cta"}, MeasurementWindow: "发布后24小时", EligibleKnowledgeIDs: []string{knowledgeID}, BlockedKnowledgeIDs: []string{}, RightsIDs: []string{}, RiskDecisionIDs: []string{}, DurationMinMS: 4000, DurationMaxMS: 4000, AspectRatio: "9:16", BlockedReasons: []string{}, MissingInputs: []string{},
	}
}

func validScriptPackageV2(batch CreativeBatch, direction CreativeDirection, knowledgeID string) ScriptPackageV2 {
	shots := []ScriptShotV2{}
	roles := []string{"hook", "product_intro", "proof", "cta"}
	for index, role := range roles {
		incoming := "start"
		if index > 0 {
			incoming = "state-" + string(rune('0'+index))
		}
		outgoing := "state-" + string(rune('1'+index))
		shot := ScriptShotV2{ShotID: "SHOT-" + string(rune('1'+index)), StartMS: index * 1000, EndMS: (index + 1) * 1000, Role: role, NarrativePurpose: role, Subject: "旅行照片", VisualIntent: "可观察画面", SubjectAction: "手移动照片", Composition: "近景", CameraMotion: "缓慢推近", FirstFrame: ScriptFrameV2{VisualState: incoming, PromptZH: "首帧", AssetRefs: []string{}}, MotionSpec: "单一平移动作", EndFrame: ScriptFrameV2{VisualState: outgoing, PromptZH: "尾帧", AssetRefs: []string{}}, Voiceover: "", OnScreenText: "", SoundIntent: "环境声", ProductionMode: "generated_non_product", KnowledgeRefs: []string{knowledgeID}, ClaimRefs: []string{}, AssetRefs: []string{}, RightsRefs: []string{}, ProductTruthStrategy: "不生成产品细节", NegativeConstraints: []string{"不得出现产品包装"}, Continuity: ScriptContinuityV2{IncomingState: incoming, OutgoingState: outgoing, MovementAxis: "左到右", LightingLock: "自然光", ProductLock: "无产品细节", Anchors: []string{"旅行照片"}}, AcceptanceCriteria: []string{"动作清晰"}, PlanB: "改用静态照片"}
		if role == "proof" {
			shot.VisualizationPlanID = "visualization:1"
		}
		shots = append(shots, shot)
	}
	return ScriptPackageV2{
		ID: "script-version:1", Kind: "script_package", Status: "candidate", SchemaVersion: "2.0", Deliverability: "review_ready", ProjectID: batch.ProjectID, ScriptID: "script:1", CreativeBatchID: batch.ID, BriefVersionID: batch.BriefVersionID, ContextSnapshotID: batch.ContextSnapshotID, ResolvedCommentIDs: []string{}, Direction: direction, Title: "旅行收尾", Channel: "douyin", DurationMS: 4000, AspectRatio: "9:16", Cover: ScriptCover{Title: "旅行收尾", Subtitle: "", VisualIntent: "旅行照片", FirstViewSignal: "南京旅行照片", AssetRefs: []string{}, RightsRefs: []string{}, SafeArea: "中央安全区", OcclusionGuards: []string{"不遮挡主体"}}, NarrativeStructure: []NarrativeSegment{}, Shots: shots, Citations: []ScriptCitationV2{}, AssetRequirements: []ScriptAssetRequirement{}, Experiment: ScriptExperiment{PrimaryVariable: "hook", ControlledVariables: []string{"audience", "cta"}, Hypothesis: "场景钩子提高停留", MeasurementWindow: "24h", TargetMetrics: []string{"3s_retention"}}, GlobalConstraints: ScriptGlobalConstraints{ForbiddenClaims: []string{"功效承诺"}, BrandRules: []string{}, ProductTruthRules: []string{"不生成产品细节"}, ContinuityLocks: []string{"自然光"}, PlatformSafeAreaRules: []string{"中央安全区"}}, BlockedReasons: []ScriptBlockedReason{}, MissingInputs: []string{}, ValidationDeclarations: ScriptValidationDeclarations{SchemaChecked: true, KnowledgeChecked: true, RightsChecked: true, ContinuityChecked: true, ExperimentChecked: true},
	}
}

func validBlockedScriptPackageV2(batch CreativeBatch, direction CreativeDirection) ScriptPackageV2 {
	pkg := validScriptPackageV2(batch, direction, "fact:product")
	pkg.ID = "script-version:blocked"
	pkg.ScriptID = "script:blocked"
	pkg.Status = "blocked"
	pkg.Deliverability = "blocked"
	pkg.Shots = []ScriptShotV2{}
	pkg.Citations = []ScriptCitationV2{}
	pkg.BlockedReasons = []ScriptBlockedReason{{Code: "ASSET_MISSING", Message: "缺少产品实拍", OwnerRole: "客户资料负责人", NextAction: "补充授权实拍"}}
	pkg.MissingInputs = []string{"产品实拍"}
	return pkg
}

func TestLintBriefRequiresApprovedStrategyVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	knowledge := LocalKnowledgeItem{ID: "fact:product", Kind: "fact", Status: "candidate", Title: "产品事实", Statement: "产品事实", Subject: "产品", Predicate: "事实", Value: domain.TypedValue{Type: "text", Text: "产品事实"}, Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "low", AllowedChannels: []string{"douyin"}, Evidence: []domain.EvidenceRef{}, EvidenceIDs: []string{}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{}, Dimensions: []string{"category"}, Layers: []string{"product"}}
	if err := replaceJSON(filepath.Join(root, "knowledge", "facts", "fact-product.json"), knowledge, 0o600); err != nil {
		t.Fatal(err)
	}
	storeApprovedObject(t, root, "knowledge-snapshot", "knowledge", knowledge.ID, knowledge, now)

	briefPath := filepath.Join(root, "outputs", "briefs", "brief-1.json")
	writeBrief := func(brief LocalBrief) {
		t.Helper()
		if err := replaceJSON(briefPath, brief, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	hasIssue := func(report KnowledgeLintReport, code string) bool {
		for _, issue := range report.Issues {
			if issue.Code == code {
				return true
			}
		}
		return false
	}

	missing := validLocalBrief(knowledge.ID)
	missing.StrategyVersionID = ""
	writeBrief(missing)
	report, _, err := LintBrief(root, "outputs/briefs/brief-1.json")
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || !hasIssue(report, "BRIEF_FIELD_REQUIRED") {
		t.Fatalf("缺少 strategy_version_id 应被拒绝：%+v", report.Issues)
	}

	unapproved := validLocalBrief(knowledge.ID)
	writeBrief(unapproved)
	report, _, err = LintBrief(root, "outputs/briefs/brief-1.json")
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || !hasIssue(report, "BRIEF_STRATEGY_NOT_APPROVED") {
		t.Fatalf("未批准的 strategy_version_id 应被拒绝：%+v", report.Issues)
	}

	storeApprovedObject(t, root, "strategy-snapshot", "strategy", unapproved.StrategyVersionID, map[string]any{"id": unapproved.StrategyVersionID, "kind": "strategy_version"}, now.Add(time.Minute))
	report, _, err = LintBrief(root, "outputs/briefs/brief-1.json")
	if err != nil || !report.Valid {
		t.Fatalf("已批准策略后 Brief 应通过：%+v %v", report.Issues, err)
	}
}

func storeApprovedObject(t *testing.T, root, snapshotID, submissionType, objectID string, object any, createdAt time.Time) {
	t.Helper()
	objects, err := json.Marshal([]any{object})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(map[string]any{"schema_version": "2.0", "submission_type": submissionType, "objects": json.RawMessage(objects)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := domain.ApprovedSnapshot{ID: snapshotID, SubmissionType: submissionType, CanonicalContent: canonical, EligibleIDs: []string{objectID}, CreatedAt: createdAt}
	if _, err := StoreApprovedSnapshot(root, snapshot, createdAt); err != nil {
		t.Fatal(err)
	}
}
