package localworkspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestWeChatArticleGoldenJourney(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC)
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	fact := articleKnowledge("fact:travel", "fact", "旅行后整理照片能减少素材散落")
	claim := articleKnowledge("claim:product", "claim", "该产品支持按时间整理照片")
	for _, item := range []LocalKnowledgeItem{fact, claim} {
		path := filepath.Join(root, "30-knowledge", "pages", "facts", localSafeName(item.ID)+".md")
		if err := writeKnowledgePage(path, item); err != nil {
			t.Fatal(err)
		}
	}
	storeApprovedObjects(t, root, "knowledge-snapshot", "knowledge", []any{fact, claim}, []string{fact.ID, claim.ID}, now)

	brief := validArticleBrief(fact.ID, claim.ID)
	briefPath := filepath.Join(root, "50-production", "briefs", "article-brief-1.json")
	if err := replaceJSON(briefPath, brief, 0o600); err != nil {
		t.Fatal(err)
	}
	briefReport, _, err := LintArticleBrief(root, "50-production/briefs/article-brief-1.json")
	if err != nil || !briefReport.Valid {
		t.Fatalf("article brief lint failed: %+v, %v", briefReport, err)
	}
	storeApprovedObjects(t, root, "brief-snapshot", "brief", []any{brief}, []string{brief.ID}, now.Add(time.Minute))

	created, err := CreateArticleBatch(CreateArticleBatchOptions{Root: root, BriefID: brief.ID, RequestedCount: 1, BatchID: "article-batch-1", Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if created.Batch.ContentKind != domain.ContentTypeWeChatArticle || created.Batch.ContentSchemaRef != ArticleSchema || !containsString(created.Batch.DeliveryProfiles, "wechat_html") {
		t.Fatalf("unexpected article batch route: %+v", created.Batch)
	}

	item := validArticleItem(created.Batch, fact.ID, claim.ID)
	itemPath := filepath.Join(root, "50-production", "batches", created.Batch.ID, "items", "article-item-1.json")
	if err := replaceJSON(itemPath, item, 0o600); err != nil {
		t.Fatal(err)
	}
	itemReport, _, err := LintArticleItem(root, relativeWorkspacePath(root, itemPath), created.BatchPath)
	if err != nil || !itemReport.Valid {
		t.Fatalf("article item lint failed: %+v, %v", itemReport, err)
	}
	if itemReport.WordCount < brief.MinWordCount {
		t.Fatalf("article word count was not evaluated: %+v", itemReport)
	}

	finalized, batchReport, err := FinalizeArticleBatch(root, created.BatchPath, []string{relativeWorkspacePath(root, itemPath)}, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Status != "review_ready" || !finalized.Publishable || batchReport.ReviewReady != 1 {
		t.Fatalf("unexpected finalized article batch: %+v %+v", finalized, batchReport)
	}

	storeApprovedObjects(t, root, "article-snapshot", "content_batch", []any{item}, []string{item.ID}, now.Add(4*time.Minute))
	exported, err := ExportWeChatPackage(root, item.ID, "", now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if exported.Package.Status != "validated" || len(exported.Package.Files) != 5 || len(exported.Package.ExternalActions) != 5 {
		t.Fatalf("unexpected WeChat package: %+v", exported.Package)
	}
	if exported.Package.LayoutProfile.TemplateVersion != "1.0.0" || exported.Package.DOMIntegrity.SanitizeChanged || exported.Package.Metadata.Title == "" || len(exported.Package.Lineage.Artifacts) != len(exported.Package.Files) {
		t.Fatalf("WeChat template, DOM or lineage metadata is incomplete: %+v", exported.Package)
	}
	providerRoot := filepath.Dir(filepath.Join(root, filepath.FromSlash(exported.PackagePath)))
	htmlBody, err := os.ReadFile(filepath.Join(providerRoot, "article.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(htmlBody), "<script>") || !strings.Contains(string(htmlBody), "&lt;script&gt;") {
		t.Fatalf("article renderer did not escape unsafe text: %s", htmlBody)
	}
	observedPath := filepath.Join(root, "60-delivery", "wechat-observed.html")
	if err := os.WriteFile(observedPath, htmlBody, 0o600); err != nil {
		t.Fatal(err)
	}
	domDiff, err := InspectWeChatPlatformDOM(root, exported.PackagePath, relativeWorkspacePath(root, observedPath), now.Add(6*time.Minute))
	if err != nil || !domDiff.Matches {
		t.Fatalf("identical platform DOM did not match: %+v, %v", domDiff, err)
	}
	if err := os.WriteFile(observedPath, []byte("<article>platform stripped styles</article>"), 0o600); err != nil {
		t.Fatal(err)
	}
	domDiff, err = InspectWeChatPlatformDOM(root, exported.PackagePath, relativeWorkspacePath(root, observedPath), now.Add(7*time.Minute))
	if err != nil || domDiff.Matches || len(domDiff.MissingMarkers) == 0 {
		t.Fatalf("platform DOM cleaning drift was not detected: %+v, %v", domDiff, err)
	}
	packageReport, err := LintWeChatPackage(root, exported.PackagePath)
	if err != nil || !packageReport.Valid {
		t.Fatalf("WeChat package lint failed: %+v, %v", packageReport, err)
	}
	if err := os.WriteFile(filepath.Join(providerRoot, "article.md"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	packageReport, err = LintWeChatPackage(root, exported.PackagePath)
	if err != nil || packageReport.Valid || !hasArticleIssue(packageReport.Issues, "WECHAT_PACKAGE_FILE_DIGEST_MISMATCH") {
		t.Fatalf("tampered WeChat package was accepted: %+v, %v", packageReport, err)
	}
}

func TestArticleAssertionAndRevisionGates(t *testing.T) {
	batch := ContentBatch{ID: "article-batch", ContentKind: domain.ContentTypeWeChatArticle, ContentSchemaRef: ArticleSchema, ProjectID: "project-1", BriefRef: "article-brief", ContextSnapshotID: "context-1"}
	fact := articleKnowledge("fact:travel", "fact", "事实")
	claim := articleKnowledge("claim:product", "claim", "主张")
	item := validArticleItem(batch, fact.ID, claim.ID)
	query := KnowledgeQueryResult{Eligible: []KnowledgeQueryEntry{{Item: fact}, {Item: claim}}}
	references := map[string]LocalKnowledgeItem{fact.ID: fact, claim.ID: claim}
	report := lintArticleItem(item, batch, query, references)
	if !report.Valid {
		t.Fatalf("valid article item failed: %+v", report)
	}
	item.Blocks[1].Assertions[0].KnowledgeRefs = []string{}
	report = lintArticleItem(item, batch, query, references)
	if report.Valid || !hasArticleIssue(report.Issues, "ARTICLE_FACT_EVIDENCE_REQUIRED") {
		t.Fatalf("fact without evidence was accepted: %+v", report)
	}

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	base := validArticleItem(batch, fact.ID, claim.ID)
	base.ID = "article-version-1"
	candidate := base
	candidate.ID = "article-version-2"
	candidate.BasedOnVersionID = base.ID
	candidate.ChangeSummary = "只调整摘要"
	candidate.Summary = "新的摘要"
	candidate.Blocks = append([]ArticleBlock(nil), base.Blocks...)
	candidate.Blocks[1].Text = "未声明的正文变化"
	basePath := filepath.Join(root, "50-production", "batches", "revisions", "base.json")
	candidatePath := filepath.Join(root, "50-production", "batches", "revisions", "candidate.json")
	if err := replaceJSON(basePath, base, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceJSON(candidatePath, candidate, 0o600); err != nil {
		t.Fatal(err)
	}
	diff, err := DiffArticleItems(root, relativeWorkspacePath(root, basePath), relativeWorkspacePath(root, candidatePath), []string{"/summary"})
	if err != nil {
		t.Fatal(err)
	}
	if diff.Valid || !containsString(diff.UnexpectedPaths, "/blocks/1/text") {
		t.Fatalf("undeclared article revision drift was accepted: %+v", diff)
	}
}

func TestWeChatHTMLLintRejectsDesktopOnlyStructures(t *testing.T) {
	issues := lintWeChatHTML(`<article data-contentcloud-schema="contentcloud.article/1.0" style="width:900px"><table><tr><td>x</td></tr></table><pre><code>x</code></pre></article>`)
	if !hasArticleIssue(issues, "WECHAT_HTML_TABLE_UNSUPPORTED") || !hasArticleIssue(issues, "WECHAT_HTML_CODE_UNSUPPORTED") {
		t.Fatalf("table and code layout risks were not rejected: %+v", issues)
	}
}

func articleKnowledge(id, kind, statement string) LocalKnowledgeItem {
	return LocalKnowledgeItem{
		ID: id, Kind: kind, Status: "candidate", Title: statement, Statement: statement, Subject: "照片", Predicate: "能力", Value: domain.TypedValue{Type: "text", Text: statement},
		Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{WeChatChannel}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "low", AllowedChannels: []string{WeChatChannel},
		Evidence: []domain.EvidenceRef{}, EvidenceIDs: []string{}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{}, Dimensions: []string{"category"}, Layers: []string{"product"},
	}
}

func validArticleBrief(factID, claimID string) ArticleBrief {
	return ArticleBrief{
		ID: "article-brief-1", Kind: "article_brief", Status: "candidate", SchemaVersion: ArticleBriefSchema, Deliverability: "review_ready", IntentID: "intent:wechat-article", Channel: WeChatChannel,
		Topic: "旅行照片如何回到日常", ReaderPromise: "获得可执行的照片整理方法", ContentPillar: "使用指南", Objective: "建立产品认知", Audience: "旅行后需要整理照片的人", ReadingContext: "通勤时阅读",
		StructureType: "how_to", SectionGoals: []string{"解释问题", "给出步骤", "说明产品能力", "引导行动"}, OpeningStrategy: "从旅行结束后的照片散落切入", EndingStrategy: "给出当天可执行动作",
		TargetWordCount: 160, MinWordCount: 100, MaxWordCount: 500, Voice: "品牌编辑", Tone: "克制可信", NarrativePerson: "second", RequiredKnowledgeIDs: []string{factID}, ApprovedClaimIDs: []string{claimID},
		AssetIDs: []string{}, RightsIDs: []string{}, CoverIntent: "旅行照片与日常桌面", CTA: "开始整理本次旅行照片", PrimaryVariable: "title", ControlledVariables: []string{"opening", "structure", "cta", "cover"}, BlockedReasons: []string{}, MissingInputs: []string{},
	}
}

func validArticleItem(batch ContentBatch, factID, claimID string) ArticleItem {
	return ArticleItem{
		ID: "article-item-version-1", Type: "article_item", Status: "candidate", SchemaVersion: ArticleSchema, Deliverability: "review_ready", ProjectID: batch.ProjectID, ContentID: "article-content-1", ContentBatchID: batch.ID, BriefRef: batch.BriefRef, ContextSnapshotID: batch.ContextSnapshotID,
		ResolvedCommentIDs: []string{}, Language: "zh-CN", TitleCandidates: []ArticleTitle{{ID: "title-1", Text: "旅行结束后，照片怎么真正留下来", Strategy: "reader-benefit", RiskRefs: []string{}}}, SelectedTitleID: "title-1", Summary: "一套从照片散落到有序留存的具体方法。", AuthorDisplayName: "ContentCloud 编辑部",
		Cover: ArticleImage{AssetRef: "", RightsRef: "", AltText: "", Caption: "", Purpose: ""},
		Blocks: []ArticleBlock{
			{ID: "block-001", Type: "heading", Text: "旅行结束，整理才刚开始", Level: 2, Items: []string{}, Assertions: []ArticleAssertion{}, StyleMarks: []string{}},
			{ID: "block-002", Type: "paragraph", Text: "<script>alert(1)</script> 旅行后及时整理照片，可以减少素材散落，也让当时的记忆更容易被重新找到。", Items: []string{}, Assertions: []ArticleAssertion{{ID: "assertion-1", Type: "fact", KnowledgeRefs: []string{factID}, EvidenceRefs: []string{}, Attribution: ""}}, StyleMarks: []string{}},
			{ID: "block-003", Type: "paragraph", Text: "产品支持按时间整理照片，先从同一天的内容开始归档，再逐步补充地点和人物。", Items: []string{}, Assertions: []ArticleAssertion{{ID: "assertion-2", Type: "commercial_claim", KnowledgeRefs: []string{claimID}, EvidenceRefs: []string{}, Attribution: ""}}, StyleMarks: []string{"strong"}},
			{ID: "block-004", Type: "quote", Text: "先完成一次可用的整理，再追求完整。", Items: []string{}, Assertions: []ArticleAssertion{{ID: "assertion-3", Type: "quotation", KnowledgeRefs: []string{}, EvidenceRefs: []string{"evidence:editor"}, Attribution: "内容编辑"}}, StyleMarks: []string{}},
			{ID: "block-005", Type: "cta", Text: "今天先整理一组照片", Items: []string{}, Target: "product:photo-organizer", Assertions: []ArticleAssertion{}, StyleMarks: []string{}},
		},
		Attribution:     ArticleAttribution{SourceNames: []string{"ContentCloud 客户知识库"}, Disclosure: "事实与产品主张来自已批准知识快照。"},
		EditorialChecks: ArticleEditorialChecks{SchemaChecked: true, KnowledgeChecked: true, ClaimsChecked: true, QuotationsChecked: true, RightsChecked: true, ChannelChecked: true},
		ChannelHints:    ArticleChannelHints{HighlightBlockIDs: []string{"block-003"}, PreferredLineBreakBlockIDs: []string{"block-002"}}, BlockedReasons: []ContentBlockedReason{}, MissingInputs: []string{},
	}
}

func storeApprovedObjects(t *testing.T, root, snapshotID, submissionType string, objects []any, eligibleIDs []string, createdAt time.Time) {
	t.Helper()
	objectBody, err := json.Marshal(objects)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(map[string]any{"schema_version": domain.SubmissionSchemaVersion(submissionType), "submission_type": submissionType, "objects": json.RawMessage(objectBody)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := domain.ApprovedSnapshot{ID: snapshotID, SubmissionType: submissionType, CanonicalContent: canonical, EligibleIDs: eligibleIDs, CreatedAt: createdAt}
	if _, err := StoreApprovedSnapshot(root, snapshot, createdAt); err != nil {
		t.Fatal(err)
	}
}

func hasArticleIssue(issues []ContentLintIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
