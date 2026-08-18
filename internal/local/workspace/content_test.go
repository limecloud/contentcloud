package localworkspace

import (
	"archive/zip"
	"encoding/json"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestContentBatchItemLintFinalizeAndApprovedExport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	knowledge := LocalKnowledgeItem{ID: "fact:product", Kind: "fact", Status: "candidate", Title: "产品事实", Statement: "产品事实", Subject: "产品", Predicate: "事实", Value: sourcedomain.TypedValue{Type: "text", Text: "产品事实"}, Scope: sourcedomain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "low", AllowedChannels: []string{"douyin"}, Evidence: []sourcedomain.EvidenceRef{}, EvidenceIDs: []string{}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{}, Dimensions: []string{"category"}, Layers: []string{"product"}}
	if err := writeKnowledgePage(filepath.Join(root, "30-knowledge", "pages", "facts", "fact-product.md"), knowledge); err != nil {
		t.Fatal(err)
	}
	storeApprovedObject(t, root, "knowledge-snapshot", "knowledge", knowledge.ID, knowledge, now)

	brief := validLocalBrief(knowledge.ID)
	briefPath := filepath.Join(root, "50-production", "briefs", "brief-1.json")
	if err := replaceJSON(briefPath, brief, 0o600); err != nil {
		t.Fatal(err)
	}
	briefLint, _, err := LintBrief(root, "50-production/briefs/brief-1.json")
	if err != nil || !briefLint.Valid {
		t.Fatalf("brief lint failed: %+v %v", briefLint, err)
	}
	storeApprovedObject(t, root, "brief-snapshot", "brief", brief.ID, brief, now.Add(time.Minute))

	direction := CreativeDirection{ID: "direction:travel", Title: "旅行收尾", Angle: "把旅行记忆带回日常", HookType: "场景直入", VisualMotif: "旅行照片", Narrative: []string{"触发", "产品", "证明", "行动"}, Tone: "克制", TargetEmotion: "留恋", RiskRefs: []string{}, Status: "selected"}
	directionsPath := filepath.Join(root, "50-production", "plans", "directions.json")
	if err := replaceJSON(directionsPath, []CreativeDirection{direction}, 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := CreateContentBatch(CreateContentBatchOptions{Root: root, BriefID: brief.ID, DirectionsFile: "50-production/plans/directions.json", RequestedCount: 2, VariantDimension: "hook", ControlledDimensions: []string{"audience", "cta"}, BatchID: "content-batch-1", Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if created.Batch.Status != "candidate" || created.Batch.Publishable || len(created.Batch.KnowledgeSnapshotRefs) != 1 || created.Batch.KnowledgeSnapshotRefs[0] != "knowledge-snapshot" || filepath.Base(created.BatchPath) != "manifest.yaml" {
		t.Fatalf("unexpected batch: %+v", created)
	}

	reviewReady := validContentItem(created.Batch, direction, knowledge.ID)
	reviewPath := filepath.Join(root, "50-production", "batches", "content-batch-1", "content-review.json")
	if err := replaceJSON(reviewPath, reviewReady, 0o600); err != nil {
		t.Fatal(err)
	}
	report, _, err := LintContentItem(root, relativeWorkspacePath(root, reviewPath), created.BatchPath)
	if err != nil || !report.Valid {
		t.Fatalf("content item lint failed: %+v %v", report, err)
	}
	blocked := validBlockedContentItem(created.Batch, direction)
	blockedPath := filepath.Join(root, "50-production", "batches", "content-batch-1", "content-blocked.json")
	if err := replaceJSON(blockedPath, blocked, 0o600); err != nil {
		t.Fatal(err)
	}
	finalized, err := FinalizeContentBatch(root, created.BatchPath, []string{relativeWorkspacePath(root, reviewPath), relativeWorkspacePath(root, blockedPath)}, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Batch.Status != "blocked" || finalized.Batch.Publishable || len(finalized.Batch.BlockedReasons) == 0 || finalized.Report.ReviewReady != 1 || finalized.Report.Blocked != 1 {
		t.Fatalf("unexpected finalized batch: %+v", finalized)
	}

	storeApprovedObject(t, root, "content-snapshot", "content_batch", reviewReady.ID, reviewReady, now.Add(4*time.Minute))
	manifest, err := ExportApprovedContentItem(root, reviewReady.ID, "", now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ApprovedSnapshotID != "content-snapshot" || manifest.ContentItemID != reviewReady.ID || len(manifest.Files) != 3 {
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

func TestContentItemRevisionDiffRejectsUndeclaredDrift(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	batch := ContentBatch{ID: "batch", ProjectID: "project-1", BriefRef: "brief", ContextSnapshotID: "context"}
	direction := CreativeDirection{ID: "direction", Status: "selected"}
	base := validContentItem(batch, direction, "fact:product")
	base.ID = "content-item-version-1"
	candidate := base
	candidate.ID = "content-item-version-2"
	candidate.BasedOnVersionID = base.ID
	candidate.ChangeSummary = "调整标题，同时意外改变 CTA"
	candidate.Title = "新标题"
	candidate.Shots = append([]ContentShot(nil), base.Shots...)
	candidate.Shots[3].OnScreenText = "新的 CTA"
	basePath := filepath.Join(root, "50-production", "batches", "revisions", "base.json")
	candidatePath := filepath.Join(root, "50-production", "batches", "revisions", "candidate.json")
	if err := replaceJSON(basePath, base, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceJSON(candidatePath, candidate, 0o600); err != nil {
		t.Fatal(err)
	}
	diff, err := DiffContentItems(root, "50-production/batches/revisions/base.json", "50-production/batches/revisions/candidate.json", []string{"/title"})
	if err != nil {
		t.Fatal(err)
	}
	if diff.Valid || len(diff.UnexpectedPaths) != 1 || diff.UnexpectedPaths[0] != "/shots/3/on_screen_text" {
		t.Fatalf("unexpected diff: %+v", diff)
	}
}

func TestContentItemLintRequiresExplicitArraysAndBlockedReasons(t *testing.T) {
	batch := ContentBatch{ID: "batch", ContentKind: identitydomain.ContentTypeVideoScript, ContentSchemaRef: ContentItemSchema, Status: "candidate", ProjectID: "project-1", BriefRef: "brief", ContextSnapshotID: "context", DirectionIDs: []string{"direction"}, VariantDimension: "hook"}
	direction := CreativeDirection{ID: "direction", Title: "方向", Angle: "角度", HookType: "场景", VisualMotif: "画面", Narrative: []string{"开始"}, Tone: "克制", TargetEmotion: "期待", RiskRefs: []string{}, Status: "selected"}
	blocked := validBlockedContentItem(batch, direction)
	report := lintContentItem(blocked, batch, KnowledgeQueryResult{}, map[string]LocalKnowledgeItem{})
	if !report.Valid {
		t.Fatalf("blocked content item with reasons and explicit empty shots must pass: %+v", report)
	}
	blocked.BlockedReasons = []ContentBlockedReason{}
	report = lintContentItem(blocked, batch, KnowledgeQueryResult{}, map[string]LocalKnowledgeItem{})
	if report.Valid || !hasContentIssue(report, "CONTENT_ITEM_BLOCK_REASON_REQUIRED") {
		t.Fatalf("blocked content item without reasons must fail: %+v", report)
	}
	blocked = validBlockedContentItem(batch, direction)
	blocked.Citations = nil
	report = lintContentItem(blocked, batch, KnowledgeQueryResult{}, map[string]LocalKnowledgeItem{})
	if report.Valid || !hasContentIssue(report, "CONTENT_ITEM_ARRAY_REQUIRED") {
		t.Fatalf("missing required arrays must fail: %+v", report)
	}
}

func hasContentIssue(report ContentItemLintReport, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func validLocalBrief(knowledgeID string) LocalBrief {
	return LocalBrief{
		ID: "brief:1", Kind: "brief", Status: "candidate", SchemaVersion: BriefSchema, Deliverability: "review_ready", StrategyVersionID: "strategy:1", CampaignID: "campaign:1", ExperimentID: "experiment:1", Channel: "douyin", Objective: "产品认知", Audience: "南京旅行者", Scenario: "旅行结束", DemandMoment: "选择伴手礼", PainPoint: "普通纪念品缺少日常使用价值", PrimarySellingPoint: "把旅行记忆带回日常", SupportPoints: []string{"产品事实"}, Positioning: "城市文化伴手礼", VisualizationPlanIDs: []string{"visualization:1"}, AssetIDs: []string{}, TruthStrategy: "非产品环境生成，产品细节用实拍", PlanB: "静物实拍", Tone: "克制", BrandRuleIDs: []string{}, ApprovedClaimIDs: []string{}, ForbiddenClaims: []string{"功效承诺"}, HookExpectation: "首秒出现旅行收尾信号", NarrativeConstraints: []string{"单一 CTA"}, CTA: "了解产品", PrimaryVariable: "hook", ControlledVariables: []string{"audience", "cta"}, MeasurementWindow: "发布后24小时", EligibleKnowledgeIDs: []string{knowledgeID}, BlockedKnowledgeIDs: []string{}, RightsIDs: []string{}, RiskDecisionIDs: []string{}, DurationMinMS: 4000, DurationMaxMS: 4000, AspectRatio: "9:16", BlockedReasons: []string{}, MissingInputs: []string{},
	}
}

func validContentItem(batch ContentBatch, direction CreativeDirection, knowledgeID string) ContentItem {
	shots := []ContentShot{}
	roles := []string{"hook", "product_intro", "proof", "cta"}
	for index, role := range roles {
		incoming := "start"
		if index > 0 {
			incoming = "state-" + string(rune('0'+index))
		}
		outgoing := "state-" + string(rune('1'+index))
		shot := ContentShot{ShotID: "SHOT-" + string(rune('1'+index)), StartMS: index * 1000, EndMS: (index + 1) * 1000, Role: role, NarrativePurpose: role, Subject: "旅行照片", VisualIntent: "可观察画面", SubjectAction: "手移动照片", Composition: "近景", CameraMotion: "缓慢推近", FirstFrame: ContentFrame{VisualState: incoming, PromptZH: "首帧", AssetRefs: []string{}}, MotionSpec: "单一平移动作", EndFrame: ContentFrame{VisualState: outgoing, PromptZH: "尾帧", AssetRefs: []string{}}, Voiceover: "", OnScreenText: "", SoundIntent: "环境声", ProductionMode: "generated_non_product", KnowledgeRefs: []string{knowledgeID}, ClaimRefs: []string{}, AssetRefs: []string{}, RightsRefs: []string{}, ProductTruthStrategy: "不生成产品细节", NegativeConstraints: []string{"不得出现产品包装"}, Continuity: ContentContinuity{IncomingState: incoming, OutgoingState: outgoing, MovementAxis: "左到右", LightingLock: "自然光", ProductLock: "无产品细节", Anchors: []string{"旅行照片"}}, AcceptanceCriteria: []string{"动作清晰"}, PlanB: "改用静态照片"}
		if role == "proof" {
			shot.VisualizationPlanID = "visualization:1"
		}
		shots = append(shots, shot)
	}
	return ContentItem{
		ID: "content-item-version:1", Type: "content_item", Status: "candidate", SchemaVersion: ContentItemSchema, Deliverability: "review_ready", ProjectID: batch.ProjectID, ContentID: "content-item:1", ContentBatchID: batch.ID, BriefRef: batch.BriefRef, ContextSnapshotID: batch.ContextSnapshotID, ResolvedCommentIDs: []string{}, Direction: direction, Title: "旅行收尾", Channel: "douyin", DurationMS: 4000, AspectRatio: "9:16", Cover: ContentCover{Title: "旅行收尾", Subtitle: "", VisualIntent: "旅行照片", FirstViewSignal: "南京旅行照片", AssetRefs: []string{}, RightsRefs: []string{}, SafeArea: "中央安全区", OcclusionGuards: []string{"不遮挡主体"}}, NarrativeStructure: []ContentNarrativeSegment{}, Shots: shots, Citations: []ContentCitation{}, AssetRequirements: []ContentAssetRequirement{}, Experiment: ContentExperiment{PrimaryVariable: "hook", ControlledVariables: []string{"audience", "cta"}, Hypothesis: "场景钩子提高停留", MeasurementWindow: "24h", TargetMetrics: []string{"3s_retention"}}, GlobalConstraints: ContentGlobalConstraints{ForbiddenClaims: []string{"功效承诺"}, BrandRules: []string{}, ProductTruthRules: []string{"不生成产品细节"}, ContinuityLocks: []string{"自然光"}, PlatformSafeAreaRules: []string{"中央安全区"}}, BlockedReasons: []ContentBlockedReason{}, MissingInputs: []string{}, ValidationDeclarations: ContentValidationDeclarations{SchemaChecked: true, KnowledgeChecked: true, RightsChecked: true, ContinuityChecked: true, ExperimentChecked: true},
	}
}

func validBlockedContentItem(batch ContentBatch, direction CreativeDirection) ContentItem {
	pkg := validContentItem(batch, direction, "fact:product")
	pkg.ID = "content-item-version:blocked"
	pkg.ContentID = "content-item:blocked"
	pkg.Status = "blocked"
	pkg.Deliverability = "blocked"
	pkg.Shots = []ContentShot{}
	pkg.Citations = []ContentCitation{}
	pkg.BlockedReasons = []ContentBlockedReason{{Code: "ASSET_MISSING", Message: "缺少产品实拍", OwnerRole: "客户资料负责人", NextAction: "补充授权实拍"}}
	pkg.MissingInputs = []string{"产品实拍"}
	return pkg
}

func TestLintBriefDoesNotDependOnRemovedStrategySubmission(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	knowledge := LocalKnowledgeItem{ID: "fact:product", Kind: "fact", Status: "candidate", Title: "产品事实", Statement: "产品事实", Subject: "产品", Predicate: "事实", Value: sourcedomain.TypedValue{Type: "text", Text: "产品事实"}, Scope: sourcedomain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "low", AllowedChannels: []string{"douyin"}, Evidence: []sourcedomain.EvidenceRef{}, EvidenceIDs: []string{}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{}, Dimensions: []string{"category"}, Layers: []string{"product"}}
	if err := writeKnowledgePage(filepath.Join(root, "30-knowledge", "pages", "facts", "fact-product.md"), knowledge); err != nil {
		t.Fatal(err)
	}
	storeApprovedObject(t, root, "knowledge-snapshot", "knowledge", knowledge.ID, knowledge, now)

	briefPath := filepath.Join(root, "50-production", "briefs", "brief-1.json")
	writeBrief := func(brief LocalBrief) {
		t.Helper()
		if err := replaceJSON(briefPath, brief, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	brief := validLocalBrief(knowledge.ID)
	writeBrief(brief)
	report, _, err := LintBrief(root, "50-production/briefs/brief-1.json")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid {
		t.Fatalf("V3 Brief 不应依赖已删除的 strategy Submission：%+v", report.Issues)
	}
}

func storeApprovedObject(t *testing.T, root, snapshotID, submissionType, objectID string, object any, createdAt time.Time) {
	t.Helper()
	objects, err := json.Marshal([]any{object})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(map[string]any{"schema_version": reviewdomain.SubmissionSchemaVersion(submissionType), "submission_type": submissionType, "objects": json.RawMessage(objects)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := reviewdomain.ApprovedSnapshot{ID: snapshotID, SubmissionType: submissionType, CanonicalContent: canonical, EligibleIDs: []string{objectID}, CreatedAt: createdAt}
	if _, err := StoreApprovedSnapshot(root, snapshot, createdAt); err != nil {
		t.Fatal(err)
	}
}
