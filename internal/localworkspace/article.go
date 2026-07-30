package localworkspace

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	ArticleBriefSchema      = "contentcloud.article-brief/1.0"
	ArticleSchema           = "contentcloud.article/1.0"
	WeChatDeliverySchema    = "contentcloud.wechat-delivery/1.0"
	WeChatChannel           = "wechat_official_account"
	WeChatChannelProfileRef = "channel:wechat-official-account-cn@1.0.0"
)

type ArticleBrief struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	Status               string   `json:"status"`
	SchemaVersion        string   `json:"schema_version"`
	Deliverability       string   `json:"deliverability"`
	IntentID             string   `json:"intent_id"`
	Channel              string   `json:"channel"`
	Topic                string   `json:"topic"`
	ReaderPromise        string   `json:"reader_promise"`
	ContentPillar        string   `json:"content_pillar"`
	Objective            string   `json:"objective"`
	Audience             string   `json:"audience"`
	ReadingContext       string   `json:"reading_context"`
	StructureType        string   `json:"structure_type"`
	SectionGoals         []string `json:"section_goals"`
	OpeningStrategy      string   `json:"opening_strategy"`
	EndingStrategy       string   `json:"ending_strategy"`
	TargetWordCount      int      `json:"target_word_count"`
	MinWordCount         int      `json:"min_word_count"`
	MaxWordCount         int      `json:"max_word_count"`
	Voice                string   `json:"voice"`
	Tone                 string   `json:"tone"`
	NarrativePerson      string   `json:"narrative_person"`
	RequiredKnowledgeIDs []string `json:"required_knowledge_ids"`
	ApprovedClaimIDs     []string `json:"approved_claim_ids"`
	AssetIDs             []string `json:"asset_ids"`
	RightsIDs            []string `json:"rights_ids"`
	CoverIntent          string   `json:"cover_intent"`
	CTA                  string   `json:"cta"`
	PrimaryVariable      string   `json:"primary_variable"`
	ControlledVariables  []string `json:"controlled_variables"`
	BlockedReasons       []string `json:"blocked_reasons"`
	MissingInputs        []string `json:"missing_inputs"`
}

type ArticleTitle struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Strategy string   `json:"strategy"`
	RiskRefs []string `json:"risk_refs"`
}

type ArticleImage struct {
	AssetRef  string `json:"asset_ref"`
	RightsRef string `json:"rights_ref"`
	AltText   string `json:"alt_text"`
	Caption   string `json:"caption"`
	Purpose   string `json:"purpose"`
}

type ArticleAssertion struct {
	ID            string   `json:"id"`
	Type          string   `json:"type"`
	KnowledgeRefs []string `json:"knowledge_refs"`
	EvidenceRefs  []string `json:"evidence_refs"`
	Attribution   string   `json:"attribution"`
}

type ArticleBlock struct {
	ID          string             `json:"id"`
	Type        string             `json:"type"`
	Text        string             `json:"text"`
	Level       int                `json:"level"`
	Ordered     bool               `json:"ordered"`
	Items       []string           `json:"items"`
	AssetRef    string             `json:"asset_ref"`
	RightsRef   string             `json:"rights_ref"`
	AltText     string             `json:"alt_text"`
	Caption     string             `json:"caption"`
	Purpose     string             `json:"purpose"`
	CalloutKind string             `json:"callout_kind"`
	Target      string             `json:"target"`
	Assertions  []ArticleAssertion `json:"assertions"`
	StyleMarks  []string           `json:"style_marks"`
}

type ArticleAttribution struct {
	SourceNames []string `json:"source_names"`
	Disclosure  string   `json:"disclosure"`
}

type ArticleEditorialChecks struct {
	SchemaChecked     bool `json:"schema_checked"`
	KnowledgeChecked  bool `json:"knowledge_checked"`
	ClaimsChecked     bool `json:"claims_checked"`
	QuotationsChecked bool `json:"quotations_checked"`
	RightsChecked     bool `json:"rights_checked"`
	ChannelChecked    bool `json:"channel_checked"`
}

type ArticleChannelHints struct {
	HighlightBlockIDs          []string `json:"highlight_block_ids"`
	PreferredLineBreakBlockIDs []string `json:"preferred_line_break_block_ids"`
}

type ArticleItem struct {
	ID                 string                 `json:"id"`
	Type               string                 `json:"type"`
	Status             string                 `json:"status"`
	SchemaVersion      string                 `json:"schema_version"`
	Deliverability     string                 `json:"deliverability"`
	ProjectID          string                 `json:"project_id"`
	ContentID          string                 `json:"content_id"`
	ContentBatchID     string                 `json:"content_batch_id"`
	BriefRef           string                 `json:"brief_ref"`
	ContextSnapshotID  string                 `json:"context_snapshot_id"`
	BasedOnVersionID   string                 `json:"based_on_version_id,omitempty"`
	ResolvedCommentIDs []string               `json:"resolved_comment_ids"`
	ChangeSummary      string                 `json:"change_summary,omitempty"`
	Language           string                 `json:"language"`
	TitleCandidates    []ArticleTitle         `json:"title_candidates"`
	SelectedTitleID    string                 `json:"selected_title_id"`
	Summary            string                 `json:"summary"`
	AuthorDisplayName  string                 `json:"author_display_name"`
	Cover              ArticleImage           `json:"cover"`
	Blocks             []ArticleBlock         `json:"blocks"`
	Attribution        ArticleAttribution     `json:"attribution"`
	EditorialChecks    ArticleEditorialChecks `json:"editorial_checks"`
	ChannelHints       ArticleChannelHints    `json:"channel_hints"`
	BlockedReasons     []ContentBlockedReason `json:"blocked_reasons"`
	MissingInputs      []string               `json:"missing_inputs"`
}

type ArticleBriefLintReport struct {
	Valid   bool               `json:"valid"`
	File    string             `json:"file"`
	BriefID string             `json:"brief_id,omitempty"`
	Issues  []ContentLintIssue `json:"issues"`
}

type ArticleItemLintReport struct {
	Valid          bool               `json:"valid"`
	File           string             `json:"file"`
	ArticleItemID  string             `json:"article_item_id,omitempty"`
	Deliverability string             `json:"deliverability,omitempty"`
	ContentHash    string             `json:"content_hash,omitempty"`
	WordCount      int                `json:"word_count"`
	Issues         []ContentLintIssue `json:"issues"`
}

type ArticleBatchLintReport struct {
	Valid       bool                    `json:"valid"`
	BatchID     string                  `json:"batch_id"`
	Requested   int                     `json:"requested"`
	Received    int                     `json:"received"`
	ReviewReady int                     `json:"review_ready"`
	Blocked     int                     `json:"blocked"`
	Results     []ArticleItemLintReport `json:"results"`
}

type CreateArticleBatchOptions struct {
	Root           string
	BriefID        string
	RequestedCount int
	BatchID        string
	Now            time.Time
}

type ArticleItemDiff struct {
	Valid           bool     `json:"valid"`
	BaselineID      string   `json:"baseline_id"`
	CandidateID     string   `json:"candidate_id"`
	ChangedPaths    []string `json:"changed_paths"`
	AllowedPaths    []string `json:"allowed_paths"`
	UnexpectedPaths []string `json:"unexpected_paths"`
}

type WeChatRenderer struct {
	CapabilityID string `json:"capability_id"`
	Version      string `json:"version"`
	Digest       string `json:"digest"`
}

type WeChatDeliveryFile struct {
	Format    string `json:"format"`
	Path      string `json:"path"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	ByteSize  int64  `json:"byte_size"`
}

type WeChatAssetMapping struct {
	BlockID   string `json:"block_id"`
	AssetRef  string `json:"asset_ref"`
	RightsRef string `json:"rights_ref"`
	Purpose   string `json:"purpose"`
	State     string `json:"state"`
}

type WeChatPackageCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type WeChatDeliveryPackage struct {
	SchemaVersion      string               `json:"schema_version"`
	ID                 string               `json:"id"`
	ProjectID          string               `json:"project_id"`
	ApprovedSnapshotID string               `json:"approved_snapshot_id"`
	ContentItemID      string               `json:"content_item_id"`
	ContentDigest      string               `json:"content_digest"`
	ChannelProfileRef  string               `json:"channel_profile_ref"`
	Renderer           WeChatRenderer       `json:"renderer"`
	Files              []WeChatDeliveryFile `json:"files"`
	AssetMapping       []WeChatAssetMapping `json:"asset_mapping"`
	ExternalActions    []string             `json:"external_actions"`
	Checks             []WeChatPackageCheck `json:"checks"`
	Status             string               `json:"status"`
	CreatedAt          time.Time            `json:"created_at"`
}

type ExportWeChatPackageResult struct {
	PackagePath string                `json:"package_path"`
	Package     WeChatDeliveryPackage `json:"package"`
}

type WeChatPackageLintReport struct {
	Valid  bool               `json:"valid"`
	File   string             `json:"file"`
	Issues []ContentLintIssue `json:"issues"`
}

func ValidateArticleItemForSubmission(raw json.RawMessage, projectID string) (ArticleItem, error) {
	var item ArticleItem
	if err := strictUnmarshal(raw, &item); err != nil {
		return item, domain.Invalid("ARTICLE_ITEM_JSON_INVALID", err.Error())
	}
	if item.ProjectID != projectID {
		return item, domain.Conflict("ARTICLE_ITEM_PROJECT_MISMATCH", "ArticleItem 不属于当前项目")
	}
	batch := ContentBatch{ID: item.ContentBatchID, ContentKind: domain.ContentTypeWeChatArticle, ContentSchemaRef: ArticleSchema, ProjectID: item.ProjectID, BriefRef: item.BriefRef, ContextSnapshotID: item.ContextSnapshotID}
	query := KnowledgeQueryResult{Eligible: []KnowledgeQueryEntry{}}
	refs := map[string]LocalKnowledgeItem{}
	for _, block := range item.Blocks {
		for _, assertion := range block.Assertions {
			for _, ref := range assertion.KnowledgeRefs {
				kind := "fact"
				if assertion.Type == "commercial_claim" {
					kind = "claim"
				}
				if _, exists := refs[ref]; !exists || kind == "claim" {
					refs[ref] = LocalKnowledgeItem{ID: ref, Kind: kind}
				}
			}
		}
	}
	keys := make([]string, 0, len(refs))
	for ref := range refs {
		keys = append(keys, ref)
	}
	sort.Strings(keys)
	for _, ref := range keys {
		query.Eligible = append(query.Eligible, KnowledgeQueryEntry{Item: refs[ref]})
	}
	report := lintArticleItem(item, batch, query, refs)
	if !report.Valid {
		err := domain.Invalid("ARTICLE_ITEM_SUBMISSION_INVALID", "ArticleItem 未通过服务端结构复验")
		err.Details = report
		return item, err
	}
	return item, nil
}

func ValidateArticleBriefForSubmission(raw json.RawMessage) (ArticleBrief, error) {
	var brief ArticleBrief
	if err := strictUnmarshal(raw, &brief); err != nil {
		return brief, domain.Invalid("ARTICLE_BRIEF_JSON_INVALID", err.Error())
	}
	query := KnowledgeQueryResult{Eligible: []KnowledgeQueryEntry{}}
	seen := map[string]bool{}
	for _, ref := range append(append([]string{}, brief.RequiredKnowledgeIDs...), brief.ApprovedClaimIDs...) {
		if !seen[ref] {
			query.Eligible = append(query.Eligible, KnowledgeQueryEntry{Item: LocalKnowledgeItem{ID: ref}})
			seen[ref] = true
		}
	}
	report := lintArticleBrief(brief, query)
	if !report.Valid {
		err := domain.Invalid("ARTICLE_BRIEF_SUBMISSION_INVALID", "ArticleBrief 未通过服务端结构复验")
		err.Details = report
		return brief, err
	}
	return brief, nil
}

func LintArticleBrief(root, file string) (ArticleBriefLintReport, ArticleBrief, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ArticleBriefLintReport{}, ArticleBrief{}, err
	}
	path, err := resolveWorkspaceFile(resolved, file)
	if err != nil {
		return ArticleBriefLintReport{}, ArticleBrief{}, err
	}
	var brief ArticleBrief
	if err := readStrictJSON(path, &brief); err != nil {
		return ArticleBriefLintReport{}, brief, domain.Invalid("ARTICLE_BRIEF_JSON_INVALID", err.Error())
	}
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: resolved, Channel: WeChatChannel})
	if err != nil {
		return ArticleBriefLintReport{}, brief, err
	}
	report := lintArticleBrief(brief, query)
	report.File = relativeWorkspacePath(resolved, path)
	return report, brief, nil
}

func lintArticleBrief(brief ArticleBrief, query KnowledgeQueryResult) ArticleBriefLintReport {
	report := ArticleBriefLintReport{Valid: true, BriefID: brief.ID, Issues: []ContentLintIssue{}}
	add := func(code, path, message string) {
		report.Issues = append(report.Issues, ContentLintIssue{Severity: "error", Code: code, Path: path, Message: message})
	}
	if brief.SchemaVersion != ArticleBriefSchema || brief.Kind != "article_brief" || brief.ID == "" || (brief.Status != "candidate" && brief.Status != "blocked") {
		add("ARTICLE_BRIEF_IDENTITY_INVALID", "/", "ArticleBrief identity 或状态无效")
	}
	if brief.Deliverability != "review_ready" && brief.Deliverability != "blocked" {
		add("ARTICLE_BRIEF_DELIVERABILITY_INVALID", "/deliverability", "deliverability 只允许 review_ready 或 blocked")
	}
	if brief.Channel != WeChatChannel {
		add("ARTICLE_BRIEF_CHANNEL_INVALID", "/channel", "首版 ArticleBrief 只支持 wechat_official_account")
	}
	for path, value := range map[string]string{
		"/intent_id": brief.IntentID, "/topic": brief.Topic, "/reader_promise": brief.ReaderPromise, "/content_pillar": brief.ContentPillar,
		"/objective": brief.Objective, "/audience": brief.Audience, "/reading_context": brief.ReadingContext, "/opening_strategy": brief.OpeningStrategy,
		"/ending_strategy": brief.EndingStrategy, "/voice": brief.Voice, "/tone": brief.Tone, "/cover_intent": brief.CoverIntent, "/cta": brief.CTA,
	} {
		if strings.TrimSpace(value) == "" {
			add("ARTICLE_BRIEF_FIELD_REQUIRED", path, "字段必填")
		}
	}
	if !validArticleStructure(brief.StructureType) || !validNarrativePerson(brief.NarrativePerson) || !validArticleVariant(brief.PrimaryVariable) {
		add("ARTICLE_BRIEF_ENUM_INVALID", "/", "structure_type、narrative_person 或 primary_variable 不受支持")
	}
	for path, values := range map[string][]string{
		"/section_goals": brief.SectionGoals, "/required_knowledge_ids": brief.RequiredKnowledgeIDs, "/approved_claim_ids": brief.ApprovedClaimIDs,
		"/asset_ids": brief.AssetIDs, "/rights_ids": brief.RightsIDs, "/controlled_variables": brief.ControlledVariables,
		"/blocked_reasons": brief.BlockedReasons, "/missing_inputs": brief.MissingInputs,
	} {
		if values == nil {
			add("ARTICLE_BRIEF_ARRAY_REQUIRED", path, "必填数组不能缺失")
		} else if !allUnique(values) {
			add("ARTICLE_BRIEF_ARRAY_DUPLICATED", path, "数组值不能重复")
		}
	}
	if len(brief.SectionGoals) == 0 || len(brief.RequiredKnowledgeIDs) == 0 {
		add("ARTICLE_BRIEF_INPUTS_REQUIRED", "/", "至少需要一个 section goal 和 required knowledge")
	}
	if brief.MinWordCount < 100 || brief.TargetWordCount < brief.MinWordCount || brief.MaxWordCount < brief.TargetWordCount || brief.MaxWordCount > 20000 {
		add("ARTICLE_BRIEF_WORD_COUNT_INVALID", "/target_word_count", "篇幅必须满足 100 <= min <= target <= max <= 20000")
	}
	if containsString(brief.ControlledVariables, brief.PrimaryVariable) {
		add("ARTICLE_BRIEF_EXPERIMENT_INVALID", "/controlled_variables", "主要变量不能同时作为控制变量")
	}
	eligible := map[string]bool{}
	for _, entry := range query.Eligible {
		eligible[entry.Item.ID] = true
	}
	for _, id := range append(append([]string{}, brief.RequiredKnowledgeIDs...), brief.ApprovedClaimIDs...) {
		if !eligible[id] {
			add("ARTICLE_BRIEF_KNOWLEDGE_NOT_ELIGIBLE", "/required_knowledge_ids", "引用知识未进入当前 ApprovedSnapshot："+id)
		}
	}
	if brief.Deliverability == "review_ready" {
		if brief.Status != "candidate" || len(brief.BlockedReasons) > 0 || len(brief.MissingInputs) > 0 {
			add("ARTICLE_BRIEF_REVIEW_READY_BLOCKED", "/deliverability", "review_ready ArticleBrief 不能保留阻断项")
		}
	} else if brief.Status != "blocked" || len(brief.BlockedReasons) == 0 {
		add("ARTICLE_BRIEF_BLOCK_REASON_REQUIRED", "/blocked_reasons", "blocked ArticleBrief 必须说明原因")
	}
	report.Valid = len(report.Issues) == 0
	return report
}

func CreateArticleBatch(options CreateArticleBatchOptions) (CreateContentBatchResult, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	briefRaw, briefSnapshot, err := latestApprovedObject(root, "brief", options.BriefID)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	var brief ArticleBrief
	if err := strictUnmarshal(briefRaw, &brief); err != nil {
		return CreateContentBatchResult{}, domain.Invalid("APPROVED_ARTICLE_BRIEF_INVALID", "批准快照中的 ArticleBrief 无效："+err.Error())
	}
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: root, Channel: WeChatChannel})
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	if report := lintArticleBrief(brief, query); !report.Valid || brief.Deliverability != "review_ready" {
		validationErr := domain.Policy("APPROVED_ARTICLE_BRIEF_BLOCKED", "批准的 ArticleBrief 未通过公众号门禁", "修订并重新批准 ArticleBrief")
		validationErr.Details = report
		return CreateContentBatchResult{}, validationErr
	}
	if query.ApprovedSnapshotID == "" {
		return CreateContentBatchResult{}, domain.Policy("KNOWLEDGE_SNAPSHOT_REQUIRED", "创建公众号文章批次需要已拉取的 Knowledge ApprovedSnapshot", "先执行 contentcloud pull approved --type knowledge")
	}
	count := options.RequestedCount
	if count == 0 {
		count = 1
	}
	if count < 1 || count > 10 {
		return CreateContentBatchResult{}, domain.Invalid("ARTICLE_BATCH_COUNT_INVALID", "requested_count 必须为 1 到 10")
	}
	status, err := LoadStatus(root)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	deliveryProfiles := []string{"json", "markdown", "wechat_html", "asset_manifest", "operator_readme"}
	hashInput := map[string]any{"project_id": status.Binding.ProjectID, "content_kind": domain.ContentTypeWeChatArticle, "content_schema_ref": ArticleSchema, "delivery_profiles": deliveryProfiles, "brief_id": brief.ID, "brief_snapshot_id": briefSnapshot.ID, "knowledge_snapshot_id": query.ApprovedSnapshotID, "requested_count": count, "primary_variable": brief.PrimaryVariable, "controlled_variables": brief.ControlledVariables}
	hash, err := domain.CanonicalHash(hashInput)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	batchID := strings.TrimSpace(options.BatchID)
	if batchID == "" {
		batchID = "article-batch-" + hash[:12]
	}
	if !localSourceIDPattern.MatchString(batchID) {
		return CreateContentBatchResult{}, domain.Invalid("CONTENT_BATCH_ID_INVALID", "batch ID 无效")
	}
	contextHash, _ := domain.CanonicalHash(map[string]any{"brief_snapshot_id": briefSnapshot.ID, "knowledge_snapshot_id": query.ApprovedSnapshotID, "eligible_ids": knowledgeEntryIDs(query.Eligible)})
	now := localNow(options.Now)
	batch := ContentBatch{
		SchemaVersion: ContentBatchSchema, ID: batchID, IntentID: brief.IntentID, ContentKind: domain.ContentTypeWeChatArticle, ContentSchemaRef: ArticleSchema, DeliveryProfiles: deliveryProfiles,
		BriefRef: brief.ID, KnowledgeSnapshotRefs: []string{query.ApprovedSnapshotID}, Status: "candidate", Publishable: false, ContentItemRefs: []string{}, BlockedReasons: []string{"批次尚未完成本地文章校验"}, Checks: []ContentBatchCheck{{Name: "context_freeze", Status: "passed"}},
		ProjectID: status.Binding.ProjectID, BriefSnapshotID: briefSnapshot.ID, ContextSnapshotID: "project-context-" + contextHash[:12], RequestedCount: count, VariantDimension: brief.PrimaryVariable, ControlledDimensions: uniqueStrings(brief.ControlledVariables), ContentHash: "sha256:" + hash, CreatedAt: now, UpdatedAt: now,
	}
	batchRoot := filepath.Join(root, "50-production", "batches", localSafeName(batchID))
	batchPath := filepath.Join(batchRoot, "manifest.yaml")
	if _, readErr := os.Stat(batchPath); readErr == nil {
		existing, loadErr := loadContentBatch(root, relativeWorkspacePath(root, batchPath), batchID)
		if loadErr == nil && existing.ContentHash == batch.ContentHash {
			return CreateContentBatchResult{BatchPath: relativeWorkspacePath(root, batchPath), ContextPath: relativeWorkspacePath(root, filepath.Join(batchRoot, "context.json")), Batch: existing}, nil
		}
		return CreateContentBatchResult{}, domain.Conflict("CONTENT_BATCH_IMMUTABLE_CONFLICT", "相同 batch ID 已存在不同内容")
	} else if !os.IsNotExist(readErr) {
		return CreateContentBatchResult{}, readErr
	}
	planRaw, _ := json.Marshal(map[string]any{"structure_type": brief.StructureType, "section_goals": brief.SectionGoals, "opening_strategy": brief.OpeningStrategy, "ending_strategy": brief.EndingStrategy})
	context := LocalContentContext{
		SchemaVersion: ContentContextSchema, Batch: batch, ProjectID: batch.ProjectID, BriefSnapshotID: batch.BriefSnapshotID, ContextSnapshotID: batch.ContextSnapshotID,
		DirectionIDs: []string{}, RequestedCount: batch.RequestedCount, VariantDimension: batch.VariantDimension, ControlledDimensions: batch.ControlledDimensions,
		ContentKind: batch.ContentKind, ContentSchemaRef: batch.ContentSchemaRef, DeliveryProfiles: batch.DeliveryProfiles, ContentHash: batch.ContentHash,
		Brief: append(json.RawMessage(nil), briefRaw...), Plan: planRaw, Eligible: query.Eligible, Blocked: query.Blocked, GeneratedAt: now,
	}
	contextPath := filepath.Join(batchRoot, "context.json")
	if err := replaceYAML(batchPath, batch); err != nil {
		return CreateContentBatchResult{}, err
	}
	if err := replaceJSON(contextPath, context, 0o600); err != nil {
		return CreateContentBatchResult{}, err
	}
	return CreateContentBatchResult{BatchPath: relativeWorkspacePath(root, batchPath), ContextPath: relativeWorkspacePath(root, contextPath), Batch: batch}, nil
}

func LintArticleItem(root, file, batchFile string) (ArticleItemLintReport, ArticleItem, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ArticleItemLintReport{}, ArticleItem{}, err
	}
	path, err := resolveWorkspaceFile(resolved, file)
	if err != nil {
		return ArticleItemLintReport{}, ArticleItem{}, err
	}
	var item ArticleItem
	if err := readStrictJSON(path, &item); err != nil {
		return ArticleItemLintReport{}, item, domain.Invalid("ARTICLE_ITEM_JSON_INVALID", err.Error())
	}
	batch, err := loadContentBatch(resolved, batchFile, item.ContentBatchID)
	if err != nil {
		return ArticleItemLintReport{}, item, err
	}
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: resolved, Channel: WeChatChannel})
	if err != nil {
		return ArticleItemLintReport{}, item, err
	}
	references, err := loadKnowledgeReferenceIndex(resolved)
	if err != nil {
		return ArticleItemLintReport{}, item, err
	}
	report := lintArticleItem(item, batch, query, references)
	var brief ArticleBrief
	if err := strictUnmarshal(batch.BriefRaw, &brief); err != nil {
		return ArticleItemLintReport{}, item, domain.Invalid("ARTICLE_BATCH_BRIEF_INVALID", "冻结 ArticleBrief 无效："+err.Error())
	}
	report = lintArticleItemAgainstBrief(report, brief)
	report.File = relativeWorkspacePath(resolved, path)
	if hash, hashErr := domain.CanonicalHash(item); hashErr == nil {
		report.ContentHash = "sha256:" + hash
	}
	return report, item, nil
}

func lintArticleItemAgainstBrief(report ArticleItemLintReport, brief ArticleBrief) ArticleItemLintReport {
	if report.WordCount < brief.MinWordCount || report.WordCount > brief.MaxWordCount {
		report.Issues = append(report.Issues, ContentLintIssue{Severity: "error", Code: "ARTICLE_WORD_COUNT_OUT_OF_RANGE", Path: "/blocks", Message: fmt.Sprintf("正文计数 %d 不在 ArticleBrief 范围 %d-%d", report.WordCount, brief.MinWordCount, brief.MaxWordCount)})
	}
	report.Valid = len(report.Issues) == 0
	return report
}

func lintArticleItem(item ArticleItem, batch ContentBatch, query KnowledgeQueryResult, refs map[string]LocalKnowledgeItem) ArticleItemLintReport {
	report := ArticleItemLintReport{Valid: true, ArticleItemID: item.ID, Deliverability: item.Deliverability, WordCount: articleWordCount(item), Issues: []ContentLintIssue{}}
	add := func(code, path, message string) {
		report.Issues = append(report.Issues, ContentLintIssue{Severity: "error", Code: code, Path: path, Message: message})
	}
	if batch.ContentKind != domain.ContentTypeWeChatArticle || batch.ContentSchemaRef != ArticleSchema {
		add("ARTICLE_BATCH_ROUTE_MISMATCH", "/content_batch_id", "公众号文章只能属于 wechat_article / contentcloud.article/1.0 批次")
	}
	if item.SchemaVersion != ArticleSchema || item.Type != "article_item" || item.ID == "" || item.ContentID == "" || (item.Status != "candidate" && item.Status != "blocked") {
		add("ARTICLE_ITEM_IDENTITY_INVALID", "/", "ArticleItem identity 或状态无效")
	}
	if item.Deliverability != "review_ready" && item.Deliverability != "blocked" {
		add("ARTICLE_ITEM_DELIVERABILITY_INVALID", "/deliverability", "deliverability 只允许 review_ready 或 blocked")
	}
	if item.ContentBatchID != batch.ID || item.BriefRef != batch.BriefRef || item.ContextSnapshotID != batch.ContextSnapshotID || item.ProjectID != batch.ProjectID {
		add("ARTICLE_ITEM_BATCH_CONTEXT_MISMATCH", "/", "project/batch/brief/context 必须与 ContentBatch 冻结值一致")
	}
	if item.Language != "zh-CN" || strings.TrimSpace(item.Summary) == "" || utf8.RuneCountInString(item.Summary) > 120 || strings.TrimSpace(item.AuthorDisplayName) == "" {
		add("ARTICLE_ITEM_METADATA_INVALID", "/", "文章需要 zh-CN、1-120 字摘要和作者显示名")
	}
	for path, missing := range map[string]bool{
		"/resolved_comment_ids": item.ResolvedCommentIDs == nil, "/title_candidates": item.TitleCandidates == nil, "/blocks": item.Blocks == nil,
		"/attribution/source_names": item.Attribution.SourceNames == nil, "/channel_hints/highlight_block_ids": item.ChannelHints.HighlightBlockIDs == nil,
		"/channel_hints/preferred_line_break_block_ids": item.ChannelHints.PreferredLineBreakBlockIDs == nil, "/blocked_reasons": item.BlockedReasons == nil, "/missing_inputs": item.MissingInputs == nil,
	} {
		if missing {
			add("ARTICLE_ITEM_ARRAY_REQUIRED", path, "必填数组不能缺失")
		}
	}
	if len(item.TitleCandidates) == 0 || len(item.TitleCandidates) > 5 {
		add("ARTICLE_TITLE_COUNT_INVALID", "/title_candidates", "标题候选数量必须为 1 到 5")
	}
	titleIDs := map[string]bool{}
	for index, title := range item.TitleCandidates {
		path := "/title_candidates/" + strconv.Itoa(index)
		if title.ID == "" || titleIDs[title.ID] || strings.TrimSpace(title.Text) == "" || utf8.RuneCountInString(title.Text) > 64 || title.Strategy == "" || title.RiskRefs == nil {
			add("ARTICLE_TITLE_INVALID", path, "标题需要唯一 ID、1-64 字文本、策略和显式 risk_refs")
		}
		titleIDs[title.ID] = true
	}
	if !titleIDs[item.SelectedTitleID] {
		add("ARTICLE_SELECTED_TITLE_INVALID", "/selected_title_id", "selected_title_id 必须引用标题候选")
	}
	if item.Cover.AssetRef != "" && (item.Cover.RightsRef == "" || item.Cover.AltText == "" || item.Cover.Purpose == "") {
		add("ARTICLE_COVER_RIGHTS_INVALID", "/cover", "封面素材需要 rights_ref、alt_text 和 purpose")
	}
	if item.Deliverability == "blocked" {
		if item.Status != "blocked" || len(item.BlockedReasons) == 0 {
			add("ARTICLE_BLOCK_REASON_REQUIRED", "/blocked_reasons", "blocked ArticleItem 必须说明阻断原因")
		}
		report.Valid = len(report.Issues) == 0
		return report
	}
	if item.Status != "candidate" || len(item.BlockedReasons) > 0 || len(item.MissingInputs) > 0 {
		add("ARTICLE_REVIEW_READY_BLOCKED", "/deliverability", "review_ready ArticleItem 不能保留阻断项")
	}
	if len(item.Blocks) == 0 {
		add("ARTICLE_BLOCKS_REQUIRED", "/blocks", "review_ready ArticleItem 至少需要一个正文 block")
	}
	eligible := map[string]bool{}
	for _, entry := range query.Eligible {
		eligible[entry.Item.ID] = true
	}
	blockIDs := map[string]bool{}
	assertionIDs := map[string]bool{}
	lastHeadingLevel := 0
	for index, block := range item.Blocks {
		path := "/blocks/" + strconv.Itoa(index)
		if block.ID == "" || blockIDs[block.ID] {
			add("ARTICLE_BLOCK_ID_INVALID", path+"/id", "block id 必填且唯一")
		}
		blockIDs[block.ID] = true
		if block.Items == nil || block.Assertions == nil || block.StyleMarks == nil || !allUnique(block.StyleMarks) {
			add("ARTICLE_BLOCK_ARRAY_INVALID", path, "items、assertions、style_marks 必须显式且 style_marks 不重复")
		}
		switch block.Type {
		case "heading":
			if strings.TrimSpace(block.Text) == "" || (block.Level != 2 && block.Level != 3) || (lastHeadingLevel == 0 && block.Level != 2) || (lastHeadingLevel != 0 && block.Level > lastHeadingLevel+1) {
				add("ARTICLE_HEADING_INVALID", path, "标题只允许 H2/H3，首个标题必须为 H2 且不能跳级")
			}
			lastHeadingLevel = block.Level
		case "paragraph":
			if strings.TrimSpace(block.Text) == "" {
				add("ARTICLE_PARAGRAPH_EMPTY", path+"/text", "段落不能为空")
			}
		case "list":
			if len(block.Items) == 0 {
				add("ARTICLE_LIST_EMPTY", path+"/items", "列表至少需要一项")
			}
		case "quote":
			if strings.TrimSpace(block.Text) == "" {
				add("ARTICLE_QUOTE_EMPTY", path+"/text", "引用不能为空")
			}
		case "image":
			if block.AssetRef == "" || block.RightsRef == "" || block.AltText == "" || block.Purpose == "" {
				add("ARTICLE_IMAGE_RIGHTS_INVALID", path, "图片 block 需要 asset_ref、rights_ref、alt_text 和 purpose")
			}
		case "callout":
			if strings.TrimSpace(block.Text) == "" || (block.CalloutKind != "note" && block.CalloutKind != "conclusion" && block.CalloutKind != "warning") {
				add("ARTICLE_CALLOUT_INVALID", path, "callout 需要文本和受支持类型")
			}
		case "divider":
		case "cta":
			if strings.TrimSpace(block.Text) == "" || strings.TrimSpace(block.Target) == "" {
				add("ARTICLE_CTA_INVALID", path, "CTA 需要文案和目标")
			}
		default:
			add("ARTICLE_BLOCK_TYPE_INVALID", path+"/type", "block type 不受支持")
		}
		for assertionIndex, assertion := range block.Assertions {
			assertionPath := path + "/assertions/" + strconv.Itoa(assertionIndex)
			if assertion.ID == "" || assertionIDs[assertion.ID] || assertion.KnowledgeRefs == nil || assertion.EvidenceRefs == nil || !allUnique(assertion.KnowledgeRefs) || !allUnique(assertion.EvidenceRefs) || !validAssertionType(assertion.Type) {
				add("ARTICLE_ASSERTION_INVALID", assertionPath, "assertion 需要唯一 ID、受支持类型和显式引用数组")
			}
			assertionIDs[assertion.ID] = true
			for _, ref := range assertion.KnowledgeRefs {
				if !eligible[ref] {
					add("ARTICLE_ASSERTION_KNOWLEDGE_NOT_ELIGIBLE", assertionPath+"/knowledge_refs", "引用知识未进入当前 ApprovedSnapshot："+ref)
				}
			}
			switch assertion.Type {
			case "fact":
				if len(assertion.KnowledgeRefs) == 0 {
					add("ARTICLE_FACT_EVIDENCE_REQUIRED", assertionPath, "fact 必须引用 eligible knowledge")
				}
			case "commercial_claim":
				if len(assertion.KnowledgeRefs) == 0 {
					add("ARTICLE_CLAIM_APPROVAL_REQUIRED", assertionPath, "commercial_claim 必须引用 approved claim")
				}
				for _, ref := range assertion.KnowledgeRefs {
					if value, ok := refs[ref]; !ok || value.Kind != "claim" {
						add("ARTICLE_CLAIM_APPROVAL_REQUIRED", assertionPath, "commercial_claim 引用必须是 claim："+ref)
					}
				}
			case "quotation":
				if len(assertion.EvidenceRefs) == 0 && strings.TrimSpace(assertion.Attribution) == "" {
					add("ARTICLE_QUOTATION_ATTRIBUTION_REQUIRED", assertionPath, "quotation 必须有 evidence_ref 或 attribution")
				}
			case "personal_experience":
				if strings.TrimSpace(assertion.Attribution) == "" {
					add("ARTICLE_EXPERIENCE_DISCLOSURE_REQUIRED", assertionPath, "personal_experience 必须披露来源身份")
				}
			}
		}
	}
	for _, blockID := range append(append([]string{}, item.ChannelHints.HighlightBlockIDs...), item.ChannelHints.PreferredLineBreakBlockIDs...) {
		if !blockIDs[blockID] {
			add("ARTICLE_CHANNEL_HINT_BLOCK_MISSING", "/channel_hints", "channel hint 引用了不存在的 block："+blockID)
		}
	}
	checks := item.EditorialChecks
	if !checks.SchemaChecked || !checks.KnowledgeChecked || !checks.ClaimsChecked || !checks.QuotationsChecked || !checks.RightsChecked || !checks.ChannelChecked {
		add("ARTICLE_EDITORIAL_CHECKS_REQUIRED", "/editorial_checks", "正式候选必须声明六类确定性检查均已执行")
	}
	if report.WordCount < 1 {
		add("ARTICLE_WORD_COUNT_INVALID", "/blocks", "文章正文不能为空")
	}
	report.Valid = len(report.Issues) == 0
	return report
}

func LintArticleBatch(root, batchFile string, contentFiles []string) (ArticleBatchLintReport, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ArticleBatchLintReport{}, err
	}
	batch, err := loadContentBatch(resolved, batchFile, "")
	if err != nil {
		return ArticleBatchLintReport{}, err
	}
	report := ArticleBatchLintReport{Valid: true, BatchID: batch.ID, Requested: batch.RequestedCount, Received: len(contentFiles), Results: []ArticleItemLintReport{}}
	if batch.ContentKind != domain.ContentTypeWeChatArticle || len(contentFiles) != batch.RequestedCount {
		report.Valid = false
	}
	seen := map[string]bool{}
	for _, file := range contentFiles {
		itemReport, item, err := LintArticleItem(resolved, file, batchFile)
		if err != nil {
			return ArticleBatchLintReport{}, err
		}
		if seen[item.ID] {
			itemReport.Valid = false
			itemReport.Issues = append(itemReport.Issues, ContentLintIssue{Severity: "error", Code: "ARTICLE_ITEM_ID_DUPLICATE", Path: "/id", Message: "批次内 ArticleItem ID 重复"})
		}
		seen[item.ID] = true
		if !itemReport.Valid {
			report.Valid = false
		}
		if item.Deliverability == "review_ready" {
			report.ReviewReady++
		} else if item.Deliverability == "blocked" {
			report.Blocked++
		}
		report.Results = append(report.Results, itemReport)
	}
	return report, nil
}

func FinalizeArticleBatch(root, batchFile string, contentFiles []string, now time.Time) (ContentBatch, ArticleBatchLintReport, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ContentBatch{}, ArticleBatchLintReport{}, err
	}
	report, err := LintArticleBatch(resolved, batchFile, contentFiles)
	if err != nil {
		return ContentBatch{}, report, err
	}
	if !report.Valid {
		validationErr := domain.Invalid("ARTICLE_BATCH_LINT_FAILED", "公众号文章批次校验失败")
		validationErr.Details = report
		return ContentBatch{}, report, validationErr
	}
	batch, err := loadContentBatch(resolved, batchFile, report.BatchID)
	if err != nil {
		return ContentBatch{}, report, err
	}
	batch.Status, batch.Publishable, batch.BlockedReasons = "review_ready", true, []string{}
	if report.Blocked > 0 || report.ReviewReady == 0 {
		batch.Status, batch.Publishable = "blocked", false
		batch.BlockedReasons = []string{"批次包含 blocked ArticleItem 或没有 review_ready ArticleItem"}
	}
	files := make([]string, 0, len(contentFiles))
	for _, file := range contentFiles {
		path, resolveErr := resolveWorkspaceFile(resolved, file)
		if resolveErr != nil {
			return ContentBatch{}, report, resolveErr
		}
		files = append(files, relativeWorkspacePath(resolved, path))
	}
	batch.ContentItemRefs = uniqueStrings(files)
	batch.Checks = []ContentBatchCheck{{Name: "article_item_lint", Status: "passed"}, {Name: "batch_completeness", Status: "passed"}}
	batch.UpdatedAt = localNow(now)
	batch.ProducedAt = &batch.UpdatedAt
	path, err := resolveWorkspaceFile(resolved, batchFile)
	if err != nil {
		return ContentBatch{}, report, err
	}
	if err := replaceYAML(path, batch); err != nil {
		return ContentBatch{}, report, err
	}
	return batch, report, nil
}

func DiffArticleItems(root, baselineFile, candidateFile string, allowedPaths []string) (ArticleItemDiff, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ArticleItemDiff{}, err
	}
	baselinePath, err := resolveWorkspaceFile(resolved, baselineFile)
	if err != nil {
		return ArticleItemDiff{}, err
	}
	candidatePath, err := resolveWorkspaceFile(resolved, candidateFile)
	if err != nil {
		return ArticleItemDiff{}, err
	}
	var baseline, candidate ArticleItem
	if err := readStrictJSON(baselinePath, &baseline); err != nil {
		return ArticleItemDiff{}, domain.Invalid("ARTICLE_ITEM_BASELINE_INVALID", err.Error())
	}
	if err := readStrictJSON(candidatePath, &candidate); err != nil {
		return ArticleItemDiff{}, domain.Invalid("ARTICLE_ITEM_CANDIDATE_INVALID", err.Error())
	}
	if candidate.BasedOnVersionID != baseline.ID || strings.TrimSpace(candidate.ChangeSummary) == "" {
		return ArticleItemDiff{}, domain.Invalid("ARTICLE_ITEM_REVISION_METADATA_INVALID", "修订稿必须引用基线 ID 并填写 change_summary")
	}
	leftBody, _ := json.Marshal(baseline)
	rightBody, _ := json.Marshal(candidate)
	var left, right any
	_ = json.Unmarshal(leftBody, &left)
	_ = json.Unmarshal(rightBody, &right)
	changes := []string{}
	collectJSONDiff("", left, right, &changes)
	bookkeeping := []string{"/id", "/status", "/based_on_version_id", "/resolved_comment_ids", "/change_summary"}
	allowed := uniqueStrings(append(append([]string{}, allowedPaths...), bookkeeping...))
	unexpected := []string{}
	for _, path := range changes {
		if !pathAllowed(path, allowed) {
			unexpected = append(unexpected, path)
		}
	}
	return ArticleItemDiff{Valid: len(unexpected) == 0, BaselineID: baseline.ID, CandidateID: candidate.ID, ChangedPaths: changes, AllowedPaths: allowed, UnexpectedPaths: unexpected}, nil
}

func ExportWeChatPackage(root, contentItemID, outputDirectory string, now time.Time) (ExportWeChatPackageResult, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ExportWeChatPackageResult{}, err
	}
	raw, snapshot, err := latestApprovedObject(resolved, "content_batch", contentItemID)
	if err != nil {
		return ExportWeChatPackageResult{}, err
	}
	var item ArticleItem
	if err := strictUnmarshal(raw, &item); err != nil || item.SchemaVersion != ArticleSchema || item.Deliverability != "review_ready" {
		return ExportWeChatPackageResult{}, domain.Policy("APPROVED_ARTICLE_ITEM_INVALID", "只有已批准的 review_ready ArticleItem 能导出公众号交付包", "修订并重新批准 ArticleItem")
	}
	contentHash, err := domain.CanonicalHash(item)
	if err != nil {
		return ExportWeChatPackageResult{}, err
	}
	contentDigest := "sha256:" + contentHash
	packageID := "wechat-package-" + contentHash[:12]
	packageRoot := outputDirectory
	if strings.TrimSpace(packageRoot) == "" {
		packageRoot = filepath.Join(resolved, "60-delivery", "packages", localSafeName(packageID))
	} else if !filepath.IsAbs(packageRoot) {
		packageRoot = filepath.Join(resolved, filepath.FromSlash(packageRoot))
	}
	absolute, err := filepath.Abs(packageRoot)
	if err != nil {
		return ExportWeChatPackageResult{}, err
	}
	relative, err := filepath.Rel(resolved, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ExportWeChatPackageResult{}, domain.Policy("DELIVERY_PATH_OUTSIDE_WORKSPACE", "交付目录必须位于当前工作区", "使用 60-delivery/packages 下的目录")
	}
	packageRoot = absolute
	providerRoot := filepath.Join(packageRoot, "providers", "wechat-official-account")
	markdown := []byte(renderArticleMarkdown(item))
	fragment := []byte(renderArticleHTML(item))
	preview := []byte(renderArticlePreview(item, string(fragment)))
	articleJSON, _ := json.MarshalIndent(item, "", "  ")
	articleJSON = append(articleJSON, '\n')
	readme := []byte(renderWeChatReadme(item, snapshot.ID, contentDigest))
	outputs := []struct {
		format, path, mediaType string
		body                    []byte
	}{
		{"json", "article.json", "application/json", articleJSON},
		{"markdown", "article.md", "text/markdown", markdown},
		{"wechat_html", "article.html", "text/html", fragment},
		{"operator_readme", "README.md", "text/markdown", readme},
		{"preview_html", "preview/article-preview.html", "text/html", preview},
	}
	files := make([]WeChatDeliveryFile, 0, len(outputs))
	for _, output := range outputs {
		path := filepath.Join(providerRoot, filepath.FromSlash(output.path))
		if err := replaceFile(path, output.body, 0o600); err != nil {
			return ExportWeChatPackageResult{}, err
		}
		files = append(files, WeChatDeliveryFile{Format: output.format, Path: output.path, MediaType: output.mediaType, SHA256: articleDigest(output.body), ByteSize: int64(len(output.body))})
	}
	assets := articleAssetMappings(item)
	rendererDigest := articleDigest([]byte("contentcloud.wechat.package/1.0.0:semantic-html-v1"))
	pkg := WeChatDeliveryPackage{
		SchemaVersion: WeChatDeliverySchema, ID: packageID, ProjectID: item.ProjectID, ApprovedSnapshotID: snapshot.ID, ContentItemID: item.ID, ContentDigest: contentDigest,
		ChannelProfileRef: WeChatChannelProfileRef, Renderer: WeChatRenderer{CapabilityID: "contentcloud.wechat.package", Version: "1.0.0", Digest: rendererDigest}, Files: files, AssetMapping: assets,
		ExternalActions: []string{"manual_login", "manual_asset_upload", "manual_preview", "manual_publish", "record_external_binding"},
		Checks:          []WeChatPackageCheck{{Name: "approved_snapshot", Status: "passed"}, {Name: "article_schema", Status: "passed"}, {Name: "safe_html", Status: "passed"}, {Name: "asset_rights", Status: "passed"}}, Status: "validated", CreatedAt: localNow(now),
	}
	packagePath := filepath.Join(providerRoot, "package.json")
	if err := replaceJSON(packagePath, pkg, 0o600); err != nil {
		return ExportWeChatPackageResult{}, err
	}
	summary := map[string]any{"schema_version": "contentcloud.delivery-package/1.0", "id": packageID, "provider": "wechat-official-account", "package_ref": relativeWorkspacePath(resolved, packagePath), "content_digest": contentDigest, "status": "validated"}
	if err := replaceJSON(filepath.Join(packageRoot, "manifest.json"), summary, 0o600); err != nil {
		return ExportWeChatPackageResult{}, err
	}
	return ExportWeChatPackageResult{PackagePath: relativeWorkspacePath(resolved, packagePath), Package: pkg}, nil
}

func LintWeChatPackage(root, packageFile string) (WeChatPackageLintReport, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return WeChatPackageLintReport{}, err
	}
	path, err := resolveWorkspaceFile(resolved, packageFile)
	if err != nil {
		return WeChatPackageLintReport{}, err
	}
	var pkg WeChatDeliveryPackage
	if err := readStrictJSON(path, &pkg); err != nil {
		return WeChatPackageLintReport{}, domain.Invalid("WECHAT_PACKAGE_JSON_INVALID", err.Error())
	}
	report := WeChatPackageLintReport{Valid: true, File: relativeWorkspacePath(resolved, path), Issues: []ContentLintIssue{}}
	add := func(code, field, message string) {
		report.Issues = append(report.Issues, ContentLintIssue{Severity: "error", Code: code, Path: field, Message: message})
	}
	if pkg.SchemaVersion != WeChatDeliverySchema || pkg.ID == "" || pkg.ProjectID == "" || pkg.ApprovedSnapshotID == "" || pkg.ContentItemID == "" || pkg.ChannelProfileRef != WeChatChannelProfileRef || pkg.Status != "validated" {
		add("WECHAT_PACKAGE_IDENTITY_INVALID", "/", "公众号交付包 identity、profile 或状态无效")
	}
	if pkg.Renderer.CapabilityID != "contentcloud.wechat.package" || pkg.Renderer.Version != "1.0.0" || !strings.HasPrefix(pkg.Renderer.Digest, "sha256:") {
		add("WECHAT_PACKAGE_RENDERER_INVALID", "/renderer", "renderer 引用无效")
	}
	if len(pkg.Files) < 5 || len(pkg.Checks) == 0 || pkg.AssetMapping == nil || pkg.ExternalActions == nil {
		add("WECHAT_PACKAGE_CONTENT_INCOMPLETE", "/", "交付包文件、检查或外部动作不完整")
	}
	base := filepath.Dir(path)
	seen := map[string]bool{}
	for index, file := range pkg.Files {
		field := "/files/" + strconv.Itoa(index)
		clean := filepath.Clean(filepath.FromSlash(file.Path))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || seen[clean] {
			add("WECHAT_PACKAGE_FILE_PATH_INVALID", field+"/path", "文件路径必须唯一且位于 provider package 内")
			continue
		}
		seen[clean] = true
		body, readErr := os.ReadFile(filepath.Join(base, clean))
		if readErr != nil || int64(len(body)) != file.ByteSize || articleDigest(body) != file.SHA256 {
			add("WECHAT_PACKAGE_FILE_DIGEST_MISMATCH", field, "文件缺失、大小或摘要不一致")
		}
	}
	report.Valid = len(report.Issues) == 0
	return report, nil
}

func articleWordCount(item ArticleItem) int {
	parts := []string{}
	for _, block := range item.Blocks {
		parts = append(parts, block.Text)
		parts = append(parts, block.Items...)
	}
	count := 0
	inLatinWord := false
	for _, char := range strings.Join(parts, " ") {
		switch {
		case unicode.Is(unicode.Han, char):
			count++
			inLatinWord = false
		case unicode.IsLetter(char) || unicode.IsDigit(char):
			if !inLatinWord {
				count++
				inLatinWord = true
			}
		default:
			inLatinWord = false
		}
	}
	return count
}

func articleDigest(body []byte) string {
	return "sha256:" + digest(body)
}

func selectedArticleTitle(item ArticleItem) string {
	for _, title := range item.TitleCandidates {
		if title.ID == item.SelectedTitleID {
			return title.Text
		}
	}
	return ""
}

func renderArticleMarkdown(item ArticleItem) string {
	var out strings.Builder
	out.WriteString("# " + selectedArticleTitle(item) + "\n\n")
	out.WriteString("> " + item.Summary + "\n\n")
	for _, block := range item.Blocks {
		switch block.Type {
		case "heading":
			out.WriteString(strings.Repeat("#", block.Level) + " " + block.Text + "\n\n")
		case "paragraph":
			out.WriteString(block.Text + "\n\n")
		case "list":
			for index, value := range block.Items {
				prefix := "- "
				if block.Ordered {
					prefix = strconv.Itoa(index+1) + ". "
				}
				out.WriteString(prefix + value + "\n")
			}
			out.WriteString("\n")
		case "quote":
			out.WriteString("> " + strings.ReplaceAll(block.Text, "\n", "\n> ") + "\n\n")
		case "image":
			out.WriteString("![" + block.AltText + "](asset:" + block.AssetRef + ")\n\n")
			if block.Caption != "" {
				out.WriteString("*" + block.Caption + "*\n\n")
			}
		case "callout":
			out.WriteString("> **" + block.CalloutKind + "** " + block.Text + "\n\n")
		case "divider":
			out.WriteString("---\n\n")
		case "cta":
			out.WriteString("**" + block.Text + "**\n\n目标：" + block.Target + "\n\n")
		}
	}
	return out.String()
}

func renderArticleHTML(item ArticleItem) string {
	var out strings.Builder
	out.WriteString("<article data-contentcloud-schema=\"contentcloud.article/1.0\">\n")
	for _, block := range item.Blocks {
		id := html.EscapeString(block.ID)
		text := html.EscapeString(block.Text)
		switch block.Type {
		case "heading":
			fmt.Fprintf(&out, "<h%d id=\"%s\">%s</h%d>\n", block.Level, id, text, block.Level)
		case "paragraph":
			fmt.Fprintf(&out, "<p id=\"%s\">%s</p>\n", id, strings.ReplaceAll(text, "\n", "<br>"))
		case "list":
			tag := "ul"
			if block.Ordered {
				tag = "ol"
			}
			fmt.Fprintf(&out, "<%s id=\"%s\">", tag, id)
			for _, value := range block.Items {
				out.WriteString("<li>" + html.EscapeString(value) + "</li>")
			}
			fmt.Fprintf(&out, "</%s>\n", tag)
		case "quote":
			fmt.Fprintf(&out, "<blockquote id=\"%s\">%s</blockquote>\n", id, text)
		case "image":
			fmt.Fprintf(&out, "<figure id=\"%s\" data-asset-ref=\"%s\"><p>[图片待上传：%s]</p><figcaption>%s</figcaption></figure>\n", id, html.EscapeString(block.AssetRef), html.EscapeString(block.AltText), html.EscapeString(block.Caption))
		case "callout":
			fmt.Fprintf(&out, "<aside id=\"%s\"><strong>%s</strong><p>%s</p></aside>\n", id, html.EscapeString(block.CalloutKind), text)
		case "divider":
			fmt.Fprintf(&out, "<hr id=\"%s\">\n", id)
		case "cta":
			fmt.Fprintf(&out, "<section id=\"%s\"><strong>%s</strong><p>%s</p></section>\n", id, text, html.EscapeString(block.Target))
		}
	}
	out.WriteString("</article>\n")
	return out.String()
}

func renderArticlePreview(item ArticleItem, fragment string) string {
	return "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>" + html.EscapeString(selectedArticleTitle(item)) + "</title><style>body{margin:0;background:#f5f5f5;color:#222;font:16px/1.8 system-ui,sans-serif}main{max-width:720px;margin:0 auto;padding:32px 24px;background:#fff}h1{font-size:28px;line-height:1.3}h2{margin-top:32px;font-size:22px}h3{margin-top:24px;font-size:18px}img{max-width:100%}figure{margin:24px 0;padding:18px;background:#f3f5f7}blockquote,aside{margin:20px 0;padding:12px 16px;border-left:3px solid #2878c7;background:#f5f8fb}</style></head><body><main><h1>" + html.EscapeString(selectedArticleTitle(item)) + "</h1><p>" + html.EscapeString(item.Summary) + "</p>" + fragment + "</main></body></html>\n"
}

func renderWeChatReadme(item ArticleItem, snapshotID, contentDigest string) string {
	return "# 微信公众号交付说明\n\n" +
		"- 标题：" + selectedArticleTitle(item) + "\n" +
		"- 摘要：" + item.Summary + "\n" +
		"- ApprovedSnapshot：`" + snapshotID + "`\n" +
		"- 内容摘要：`" + contentDigest + "`\n" +
		"- Channel Profile：`" + WeChatChannelProfileRef + "`\n\n" +
		"## 后台操作\n\n1. 人工登录微信公众号后台并新建图文草稿。\n2. 按 `asset_mapping` 顺序上传封面和正文图片，核对权利记录。\n3. 填写标题、摘要、作者；复制 `article.html` 正文并发送预览。\n4. 人工复核原创/转载、广告、隐私、链接、二维码、价格、优惠和 CTA。\n5. 发布前再次核对预览；发布后回填外部内容 ID、URL、时间和账号引用。\n\n本交付包不会登录、上传、创建草稿或发布。\n"
}

func articleAssetMappings(item ArticleItem) []WeChatAssetMapping {
	values := []WeChatAssetMapping{}
	if item.Cover.AssetRef != "" {
		values = append(values, WeChatAssetMapping{BlockID: "cover", AssetRef: item.Cover.AssetRef, RightsRef: item.Cover.RightsRef, Purpose: item.Cover.Purpose, State: "manual_upload_required"})
	}
	for _, block := range item.Blocks {
		if block.Type == "image" {
			values = append(values, WeChatAssetMapping{BlockID: block.ID, AssetRef: block.AssetRef, RightsRef: block.RightsRef, Purpose: block.Purpose, State: "manual_upload_required"})
		}
	}
	return values
}

func validArticleStructure(value string) bool {
	switch value {
	case "problem_solution", "how_to", "case_study", "brand_story", "opinion_analysis", "curated_guide":
		return true
	default:
		return false
	}
}

func validNarrativePerson(value string) bool {
	return value == "first" || value == "second" || value == "third"
}

func validArticleVariant(value string) bool {
	return value == "title" || value == "opening" || value == "structure" || value == "cta" || value == "cover"
}

func validAssertionType(value string) bool {
	switch value {
	case "fact", "commercial_claim", "quotation", "editorial_opinion", "personal_experience", "hypothesis":
		return true
	default:
		return false
	}
}

func sortedArticleFiles(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
