package localworkspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

var knowledgeDimensions = []KnowledgeDimensionDefinition{
	{Key: "customer-pain", Label: "客户痛点", Keywords: []string{"客户痛点", "用户痛点", "痛点", "困扰", "不便", "pain"}},
	{Key: "customer-solution", Label: "客户方案", Keywords: []string{"客户方案", "用户方案", "解决方案", "购买理由", "customer solution"}},
	{Key: "benchmark", Label: "标杆内容", Keywords: []string{"标杆", "爆款", "案例", "benchmark"}},
	{Key: "competitors", Label: "竞品", Keywords: []string{"竞品", "竞争", "对手", "竞对", "competitor"}},
	{Key: "sales-channel", Label: "销售渠道", Keywords: []string{"渠道", "门店", "抖音", "小红书", "电商", "直播", "sales channel"}},
	{Key: "theme-subbrand", Label: "主题与子品牌", Keywords: []string{"主题", "子品牌", "品牌定位", "品牌", "theme", "subbrand"}},
	{Key: "culture-story", Label: "文化故事", Keywords: []string{"文化", "故事", "历史", "金陵", "南京", "传承", "culture"}},
	{Key: "scent-formula", Label: "香型与配方", Keywords: []string{"香型", "香气", "配方", "成分", "香味", "scent", "formula"}},
	{Key: "usage-scenario", Label: "使用场景", Keywords: []string{"场景", "使用", "送礼", "伴手礼", "居家", "办公", "scenario"}},
	{Key: "solution-value", Label: "方案价值", Keywords: []string{"价值", "利益", "好处", "解决", "体验", "solution value"}},
	{Key: "category", Label: "品类", Keywords: []string{"品类", "线香", "香品", "category"}},
	{Key: "form", Label: "产品形态", Keywords: []string{"形态", "造型", "款式", "结构", "form"}},
	{Key: "materials-factories", Label: "材料与工厂", Keywords: []string{"材料", "材质", "原料", "工厂", "生产", "制造", "material", "factory"}},
	{Key: "packaging-assembly", Label: "包装与组装", Keywords: []string{"包装", "包材", "组装", "装配", "packaging", "assembly"}},
	{Key: "spec-cost-price", Label: "规格成本价格", Keywords: []string{"规格", "尺寸", "重量", "成本", "价格", "售价", "spec", "price", "cost"}},
}

var knowledgeLayerNames = []string{"identity", "product", "market", "expression", "operations", "content_engine", "compliance"}

type LocalKnowledgeItem struct {
	ID                  string                `json:"id"`
	Kind                string                `json:"kind"`
	Title               string                `json:"title"`
	Statement           string                `json:"statement"`
	Subject             string                `json:"subject"`
	Predicate           string                `json:"predicate"`
	Value               domain.TypedValue     `json:"value"`
	Scope               domain.KnowledgeScope `json:"scope"`
	Status              string                `json:"status"`
	RiskLevel           string                `json:"risk_level"`
	AllowedChannels     []string              `json:"allowed_channels"`
	Evidence            []domain.EvidenceRef  `json:"evidence"`
	EvidenceIDs         []string              `json:"evidence_ids"`
	ForbiddenExtensions []string              `json:"forbidden_extensions"`
	DependsOnFactIDs    []string              `json:"depends_on_fact_ids"`
	AssetRefs           []string              `json:"asset_refs,omitempty"`
	RightsRefs          []string              `json:"rights_refs,omitempty"`
	ConflictRefs        []string              `json:"conflict_refs,omitempty"`
	DecisionRefs        []string              `json:"decision_refs,omitempty"`
	Dimensions          []string              `json:"dimensions"`
	Layers              []string              `json:"layers"`
	ValidFrom           *time.Time            `json:"valid_from,omitempty"`
	ValidUntil          *time.Time            `json:"valid_until,omitempty"`
	ExpiresAt           *time.Time            `json:"expires_at,omitempty"`
	ApprovalSnapshotID  string                `json:"approval_snapshot_id,omitempty"`
	OriginRunID         string                `json:"origin_run_id,omitempty"`
	ContentHash         string                `json:"content_hash"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
}

type ImportKnowledgeOptions struct {
	Root        string
	PackageFile string
	OriginRunID string
	Now         time.Time
}

type KnowledgeImportReport struct {
	SchemaVersion string               `json:"schema_version"`
	PackageFile   string               `json:"package_file"`
	Imported      []LocalKnowledgeItem `json:"imported"`
	Skipped       []string             `json:"skipped"`
	Warnings      []string             `json:"warnings"`
}

type KnowledgeLintIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	ItemID   string `json:"item_id,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

type KnowledgeLintReport struct {
	Valid        bool                 `json:"valid"`
	ItemCount    int                  `json:"item_count"`
	ErrorCount   int                  `json:"error_count"`
	WarningCount int                  `json:"warning_count"`
	Issues       []KnowledgeLintIssue `json:"issues"`
}

type QueryKnowledgeOptions struct {
	Root    string
	Channel string
	At      time.Time
}

type KnowledgeQueryEntry struct {
	Item    LocalKnowledgeItem `json:"item"`
	Reasons []string           `json:"reasons"`
	Source  string             `json:"source"`
}

type KnowledgeQueryResult struct {
	Channel            string                `json:"channel,omitempty"`
	At                 time.Time             `json:"at"`
	ApprovedSnapshotID string                `json:"approved_snapshot_id,omitempty"`
	Eligible           []KnowledgeQueryEntry `json:"eligible"`
	Blocked            []KnowledgeQueryEntry `json:"blocked"`
	Informational      []KnowledgeQueryEntry `json:"informational"`
}

type KnowledgeDimensionDefinition struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Keywords []string `json:"-"`
}

type KnowledgeDimensionStatus struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Status    string   `json:"status"`
	ItemIDs   []string `json:"item_ids"`
	Eligible  int      `json:"eligible"`
	Blocked   int      `json:"blocked"`
	Candidate int      `json:"candidate"`
	NextInput string   `json:"next_input,omitempty"`
}

type KnowledgeDiagnosis struct {
	SchemaVersion string                     `json:"schema_version"`
	Dimensions    []KnowledgeDimensionStatus `json:"dimensions"`
	Covered       int                        `json:"covered"`
	NeedsReview   int                        `json:"needs_review"`
	Missing       int                        `json:"missing"`
}

type PackKnowledgeOptions struct {
	Root   string
	PackID string
	Name   string
	Now    time.Time
}

type KnowledgePackManifest struct {
	ID            string              `json:"id"`
	Kind          string              `json:"kind"`
	Status        string              `json:"status"`
	SchemaVersion string              `json:"schema_version"`
	Name          string              `json:"name"`
	Layers        map[string][]string `json:"layers"`
	ItemCount     int                 `json:"item_count"`
	ContentHash   string              `json:"content_hash"`
	CreatedAt     time.Time           `json:"created_at"`
}

type KnowledgePackResult struct {
	Manifest        KnowledgePackManifest `json:"manifest"`
	PackPath        string                `json:"pack_path"`
	DisclosuresPath string                `json:"disclosures_path"`
	ObjectCount     int                   `json:"object_count"`
	SourceCount     int                   `json:"source_count"`
}

func ImportKnowledgeCandidates(options ImportKnowledgeOptions) (KnowledgeImportReport, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return KnowledgeImportReport{}, err
	}
	path, err := resolveWorkspaceFile(root, options.PackageFile)
	if err != nil {
		return KnowledgeImportReport{}, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return KnowledgeImportReport{}, err
	}
	var pkg domain.KnowledgeExtractionPackage
	if err := strictUnmarshal(body, &pkg); err != nil {
		return KnowledgeImportReport{}, domain.Invalid("KNOWLEDGE_CANDIDATES_JSON_INVALID", "候选包必须是 knowledge-candidates/1.0 JSON 对象")
	}
	if err := validateKnowledgeCandidates(pkg); err != nil {
		return KnowledgeImportReport{}, err
	}
	evidence, err := loadEvidenceIndex(root)
	if err != nil {
		return KnowledgeImportReport{}, err
	}
	now := localNow(options.Now)
	report := KnowledgeImportReport{SchemaVersion: pkg.SchemaVersion, PackageFile: relativeWorkspacePath(root, path), Imported: []LocalKnowledgeItem{}, Skipped: []string{}, Warnings: append([]string(nil), pkg.Warnings...)}
	for _, candidate := range pkg.Candidates {
		evidenceIDs, matchErr := matchCandidateEvidence(candidate.Evidence, evidence)
		if matchErr != nil {
			return KnowledgeImportReport{}, matchErr
		}
		item := knowledgeItemFromCandidate(candidate, evidenceIDs, strings.TrimSpace(options.OriginRunID), now)
		directory := "facts"
		if candidate.Kind == "claim" || candidate.Kind == "visual_rule" {
			directory = "claims"
		}
		destination := filepath.Join(root, "knowledge", directory, localSafeName(item.ID)+".json")
		if existingBody, readErr := os.ReadFile(destination); readErr == nil {
			var existing LocalKnowledgeItem
			if json.Unmarshal(existingBody, &existing) == nil && existing.ContentHash == item.ContentHash {
				report.Skipped = append(report.Skipped, item.ID)
				continue
			}
			return KnowledgeImportReport{}, domain.Conflict("KNOWLEDGE_ITEM_IMMUTABLE_CONFLICT", "相同知识 ID 已存在不同内容："+item.ID)
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return KnowledgeImportReport{}, readErr
		}
		if err := replaceJSON(destination, item, 0o600); err != nil {
			return KnowledgeImportReport{}, err
		}
		report.Imported = append(report.Imported, item)
	}
	return report, nil
}

func LintKnowledge(root string) (KnowledgeLintReport, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return KnowledgeLintReport{}, err
	}
	items, paths, err := loadLocalKnowledgeItems(resolved)
	if err != nil {
		return KnowledgeLintReport{}, err
	}
	evidence, evidenceErr := loadEvidenceIndex(resolved)
	if evidenceErr != nil {
		return KnowledgeLintReport{}, evidenceErr
	}
	references, err := loadKnowledgeReferenceIndex(resolved)
	if err != nil {
		return KnowledgeLintReport{}, err
	}
	report := KnowledgeLintReport{Valid: true, ItemCount: len(items), Issues: []KnowledgeLintIssue{}}
	seen := map[string]string{}
	for index, item := range items {
		path := paths[index]
		add := func(severity, code, message string) {
			report.Issues = append(report.Issues, KnowledgeLintIssue{Severity: severity, Code: code, ItemID: item.ID, Path: path, Message: message})
		}
		if item.ID == "" || item.Kind == "" {
			add("error", "KNOWLEDGE_ID_KIND_REQUIRED", "id 和 kind 必填")
		}
		if previous := seen[item.ID]; previous != "" {
			add("error", "KNOWLEDGE_ID_DUPLICATE", "ID 与 "+previous+" 重复")
		} else {
			seen[item.ID] = path
		}
		if !validKnowledgeKind(item.Kind) {
			add("error", "KNOWLEDGE_KIND_INVALID", "kind 不受支持")
		}
		if !validLocalKnowledgeStatus(item.Status) {
			add("error", "KNOWLEDGE_STATUS_INVALID", "status 不受支持")
		}
		if (item.Status == "verified" || item.Status == "approved" || item.Status == "valid") && item.ApprovalSnapshotID == "" && len(item.DecisionRefs) == 0 {
			add("error", "KNOWLEDGE_DECISION_REQUIRED", "verified/approved/valid 状态必须有审批快照或 decision_refs")
		}
		if _, matchErr := matchCandidateEvidence(item.Evidence, evidence); matchErr != nil {
			add("error", "KNOWLEDGE_EVIDENCE_INVALID", matchErr.Error())
		}
		if len(item.Evidence) != len(item.EvidenceIDs) {
			add("error", "KNOWLEDGE_EVIDENCE_ID_MISMATCH", "evidence 与 evidence_ids 数量不一致")
		}
		for _, dependency := range item.DependsOnFactIDs {
			if referenced, ok := references[dependency]; !ok {
				add("error", "KNOWLEDGE_DEPENDENCY_MISSING", "depends_on_fact_ids 引用不存在："+dependency)
			} else if referenced.Kind != "fact" {
				add("error", "KNOWLEDGE_DEPENDENCY_NOT_FACT", "依赖项不是 fact："+dependency)
			}
		}
		for _, ref := range append(append(append([]string{}, item.AssetRefs...), item.RightsRefs...), item.ConflictRefs...) {
			if _, ok := references[ref]; !ok {
				add("error", "KNOWLEDGE_REFERENCE_MISSING", "引用对象不存在："+ref)
			}
		}
		if len(item.Dimensions) == 0 {
			add("warning", "KNOWLEDGE_DIMENSION_UNCLASSIFIED", "未映射到 15 维方法论，可在审核前补充分类")
		}
		if len(item.Layers) == 0 {
			add("warning", "KNOWLEDGE_LAYER_UNCLASSIFIED", "未映射到七层 KnowledgePack")
		}
	}
	for _, issue := range report.Issues {
		if issue.Severity == "error" {
			report.ErrorCount++
			report.Valid = false
		} else {
			report.WarningCount++
		}
	}
	return report, nil
}

func QueryKnowledge(options QueryKnowledgeOptions) (KnowledgeQueryResult, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return KnowledgeQueryResult{}, err
	}
	items, _, err := loadLocalKnowledgeItems(root)
	if err != nil {
		return KnowledgeQueryResult{}, err
	}
	references, err := loadKnowledgeReferenceIndex(root)
	if err != nil {
		return KnowledgeQueryResult{}, err
	}
	at := options.At.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	channel := strings.TrimSpace(options.Channel)
	snapshot, snapshotObjects, hasSnapshot, err := latestKnowledgeSnapshot(root)
	if err != nil {
		return KnowledgeQueryResult{}, err
	}
	byID := map[string]LocalKnowledgeItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, item := range snapshotObjects {
		if _, exists := byID[item.ID]; !exists {
			byID[item.ID] = item
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	eligibleSet := map[string]bool{}
	if hasSnapshot {
		for _, id := range snapshot.EligibleIDs {
			eligibleSet[id] = true
		}
	}
	result := KnowledgeQueryResult{Channel: channel, At: at, Eligible: []KnowledgeQueryEntry{}, Blocked: []KnowledgeQueryEntry{}, Informational: []KnowledgeQueryEntry{}}
	if hasSnapshot {
		result.ApprovedSnapshotID = snapshot.ID
	}
	for _, id := range ids {
		item := byID[id]
		reasons := knowledgeBlockReasons(item, channel, at, references, eligibleSet, hasSnapshot)
		entry := KnowledgeQueryEntry{Item: item, Reasons: reasons, Source: "local"}
		if eligibleSet[id] {
			entry.Source = "approved_snapshot"
		}
		if len(reasons) > 0 {
			result.Blocked = append(result.Blocked, entry)
			continue
		}
		if hasSnapshot && eligibleSet[id] {
			result.Eligible = append(result.Eligible, entry)
			continue
		}
		if !hasSnapshot && (item.Status == "verified" || item.Status == "approved") && (item.ApprovalSnapshotID != "" || len(item.DecisionRefs) > 0) {
			result.Eligible = append(result.Eligible, entry)
			continue
		}
		entry.Reasons = []string{"尚未进入 ApprovedSnapshot，仅可作为背景信息"}
		result.Informational = append(result.Informational, entry)
	}
	return result, nil
}

func DiagnoseKnowledge(root, channel string, at time.Time) (KnowledgeDiagnosis, error) {
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: root, Channel: channel, At: at})
	if err != nil {
		return KnowledgeDiagnosis{}, err
	}
	result := KnowledgeDiagnosis{SchemaVersion: SchemaVersion, Dimensions: []KnowledgeDimensionStatus{}}
	for _, definition := range knowledgeDimensions {
		status := KnowledgeDimensionStatus{Key: definition.Key, Label: definition.Label, Status: "missing", ItemIDs: []string{}, NextInput: "补充可定位来源和证据"}
		for _, entry := range query.Eligible {
			if containsString(entry.Item.Dimensions, definition.Key) {
				status.Eligible++
				status.ItemIDs = append(status.ItemIDs, entry.Item.ID)
			}
		}
		for _, entry := range query.Blocked {
			if containsString(entry.Item.Dimensions, definition.Key) {
				status.Blocked++
				status.ItemIDs = append(status.ItemIDs, entry.Item.ID)
			}
		}
		for _, entry := range query.Informational {
			if containsString(entry.Item.Dimensions, definition.Key) {
				status.Candidate++
				status.ItemIDs = append(status.ItemIDs, entry.Item.ID)
			}
		}
		status.ItemIDs = uniqueStrings(status.ItemIDs)
		if status.Eligible > 0 {
			status.Status = "covered"
			status.NextInput = ""
			result.Covered++
		} else if status.Blocked > 0 || status.Candidate > 0 {
			status.Status = "needs_review"
			status.NextInput = "完成候选审核、冲突处理或权利补充"
			result.NeedsReview++
		} else {
			result.Missing++
		}
		result.Dimensions = append(result.Dimensions, status)
	}
	return result, nil
}

func PackKnowledge(options PackKnowledgeOptions) (KnowledgePackResult, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return KnowledgePackResult{}, err
	}
	lint, err := LintKnowledge(root)
	if err != nil {
		return KnowledgePackResult{}, err
	}
	if !lint.Valid {
		err := domain.Invalid("KNOWLEDGE_LINT_FAILED", "知识库存在阻断问题，不能打包")
		err.Details = lint
		return KnowledgePackResult{}, err
	}
	items, _, err := loadLocalKnowledgeItems(root)
	if err != nil {
		return KnowledgePackResult{}, err
	}
	if len(items) == 0 {
		return KnowledgePackResult{}, domain.Invalid("KNOWLEDGE_EMPTY", "没有可打包的知识对象")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	layers := map[string][]string{}
	for _, layer := range knowledgeLayerNames {
		layers[layer] = []string{}
	}
	for _, item := range items {
		for _, layer := range item.Layers {
			if _, ok := layers[layer]; ok {
				layers[layer] = append(layers[layer], item.ID)
			}
		}
	}
	contentHash, err := domain.CanonicalHash(items)
	if err != nil {
		return KnowledgePackResult{}, err
	}
	packID := strings.TrimSpace(options.PackID)
	if packID == "" {
		packID = "knowledge-pack-" + strings.TrimPrefix(contentHash, "sha256:")[:12]
	}
	if !localSourceIDPattern.MatchString(packID) {
		return KnowledgePackResult{}, domain.Invalid("KNOWLEDGE_PACK_ID_INVALID", "pack ID 无效")
	}
	now := localNow(options.Now)
	manifest := KnowledgePackManifest{ID: packID, Kind: "knowledge_pack_manifest", Status: "informational", SchemaVersion: SchemaVersion, Name: defaultLocalValue(options.Name, "ContentCloud 客户知识包"), Layers: layers, ItemCount: len(items), ContentHash: contentHash, CreatedAt: now}
	objects := make([]any, 0, len(items)+1)
	objects = append(objects, manifest)
	for _, item := range items {
		objects = append(objects, item)
	}
	packPath := filepath.Join(root, "knowledge", "packs", localSafeName(packID)+".json")
	if err := replaceJSON(packPath, objects, 0o600); err != nil {
		return KnowledgePackResult{}, err
	}
	disclosures, err := knowledgeSourceDisclosures(root, items)
	if err != nil {
		return KnowledgePackResult{}, err
	}
	disclosuresPath := filepath.Join(root, "knowledge", "index", localSafeName(packID)+"-disclosures.json")
	if err := replaceJSON(disclosuresPath, disclosures, 0o600); err != nil {
		return KnowledgePackResult{}, err
	}
	return KnowledgePackResult{Manifest: manifest, PackPath: relativeWorkspacePath(root, packPath), DisclosuresPath: relativeWorkspacePath(root, disclosuresPath), ObjectCount: len(objects), SourceCount: len(disclosures)}, nil
}

func validateKnowledgeCandidates(pkg domain.KnowledgeExtractionPackage) error {
	if pkg.SchemaVersion != "1.0" || len(pkg.Candidates) == 0 || len(pkg.Candidates) > 100 || pkg.Warnings == nil {
		return domain.Invalid("KNOWLEDGE_CANDIDATES_SCHEMA_INVALID", "schema_version 必须为 1.0，candidates 数量必须为 1 到 100")
	}
	if len(pkg.Warnings) > 100 {
		return domain.Invalid("KNOWLEDGE_CANDIDATES_WARNINGS_INVALID", "warnings 不能超过 100 条")
	}
	for index, candidate := range pkg.Candidates {
		if !validKnowledgeKind(candidate.Kind) || strings.TrimSpace(candidate.Title) == "" || strings.TrimSpace(candidate.Statement) == "" || strings.TrimSpace(candidate.Subject) == "" || strings.TrimSpace(candidate.Predicate) == "" {
			return domain.Invalid("KNOWLEDGE_CANDIDATE_INVALID", fmt.Sprintf("candidate %d 的 kind/title/statement/subject/predicate 无效", index+1))
		}
		if candidate.RiskLevel != "low" && candidate.RiskLevel != "medium" && candidate.RiskLevel != "high" {
			return domain.Invalid("KNOWLEDGE_RISK_INVALID", fmt.Sprintf("candidate %d risk_level 无效", index+1))
		}
		if candidate.Value.Type != "text" && candidate.Value.Type != "number" && candidate.Value.Type != "boolean" && candidate.Value.Type != "date" && candidate.Value.Type != "enum" {
			return domain.Invalid("KNOWLEDGE_VALUE_INVALID", fmt.Sprintf("candidate %d value.type 无效", index+1))
		}
		if len(candidate.Evidence) == 0 {
			return domain.Invalid("KNOWLEDGE_EVIDENCE_REQUIRED", fmt.Sprintf("candidate %d 必须包含 evidence", index+1))
		}
		if !allUnique(candidate.AllowedChannels) || !allUnique(candidate.ForbiddenExtensions) || !allUnique(candidate.DependsOnFactIDs) {
			return domain.Invalid("KNOWLEDGE_ARRAY_DUPLICATE", fmt.Sprintf("candidate %d 的数组字段不能重复", index+1))
		}
		if candidate.AllowedChannels == nil || candidate.ForbiddenExtensions == nil || candidate.DependsOnFactIDs == nil || candidate.Scope.Regions == nil || candidate.Scope.Channels == nil || candidate.Scope.Audiences == nil || candidate.Scope.ProductVariants == nil {
			return domain.Invalid("KNOWLEDGE_ARRAY_REQUIRED", fmt.Sprintf("candidate %d 必须显式返回所有数组字段", index+1))
		}
		if (candidate.Value.Type == "number" && candidate.Value.Number == nil) ||
			(candidate.Value.Type == "boolean" && candidate.Value.Boolean == nil) ||
			((candidate.Value.Type == "text" || candidate.Value.Type == "date" || candidate.Value.Type == "enum") && strings.TrimSpace(candidate.Value.Text) == "") {
			return domain.Invalid("KNOWLEDGE_VALUE_REQUIRED", fmt.Sprintf("candidate %d 的 value 与 type 不匹配", index+1))
		}
		evidenceKeys := map[string]bool{}
		for _, ref := range candidate.Evidence {
			key := ref.SourceRevisionID + "\x00" + ref.LocatorKind + "\x00" + ref.Locator + "\x00" + ref.Quote
			if evidenceKeys[key] {
				return domain.Invalid("KNOWLEDGE_EVIDENCE_DUPLICATE", fmt.Sprintf("candidate %d 的 evidence 不能重复", index+1))
			}
			evidenceKeys[key] = true
		}
	}
	return nil
}

func loadEvidenceIndex(root string) (map[string][]LocalEvidence, error) {
	sources, err := LocalSources(root)
	if err != nil {
		return nil, err
	}
	index := map[string][]LocalEvidence{}
	for _, source := range sources {
		if source.EvidencePath == "" {
			continue
		}
		var bundle LocalEvidenceBundle
		if err := readJSON(filepath.Join(root, filepath.FromSlash(source.EvidencePath)), &bundle); err != nil {
			return nil, err
		}
		if bundle.SourceID != source.ID || bundle.SourceSHA256 != source.SHA256 {
			return nil, domain.Conflict("LOCAL_EVIDENCE_SOURCE_MISMATCH", "EvidenceBundle 与 SourceRegistry 不一致："+source.ID)
		}
		index[source.ID] = bundle.Evidence
	}
	return index, nil
}

func matchCandidateEvidence(refs []domain.EvidenceRef, index map[string][]LocalEvidence) ([]string, error) {
	matched := make([]string, 0, len(refs))
	for _, ref := range refs {
		spans := index[ref.SourceRevisionID]
		if len(spans) == 0 {
			return nil, domain.Invalid("KNOWLEDGE_SOURCE_EVIDENCE_MISSING", "来源尚未 ingest，或 source_revision_id 不是本地不可变 source ID："+ref.SourceRevisionID)
		}
		locator, err := canonicalLocatorString(ref.Locator)
		if err != nil {
			return nil, domain.Invalid("KNOWLEDGE_LOCATOR_INVALID", "evidence.locator 必须是 JSON object 字符串")
		}
		found := false
		for _, span := range spans {
			spanLocator, _ := json.Marshal(span.Locator)
			canonicalSpan, _ := canonicalLocatorString(string(spanLocator))
			if span.LocatorKind == ref.LocatorKind && canonicalSpan == locator && span.Quote == ref.Quote {
				if span.ReviewStatus != "accepted" {
					return nil, domain.Policy("KNOWLEDGE_EVIDENCE_REVIEW_REQUIRED", "证据尚未通过本地人工复核："+span.ID, "先复核 OCR/视觉证据，再生成候选")
				}
				matched = append(matched, span.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, domain.Invalid("KNOWLEDGE_EVIDENCE_NOT_EXACT", "候选 evidence 未与 EvidenceBundle 的 locator 和 quote 精确匹配")
		}
	}
	return matched, nil
}

func canonicalLocatorString(value string) (string, error) {
	var locator map[string]any
	if err := json.Unmarshal([]byte(value), &locator); err != nil || locator == nil {
		return "", errors.New("invalid locator")
	}
	body, err := json.Marshal(locator)
	return string(body), err
}

func knowledgeItemFromCandidate(candidate domain.KnowledgeCandidate, evidenceIDs []string, runID string, now time.Time) LocalKnowledgeItem {
	hashInput := struct {
		Kind      string                `json:"kind"`
		Title     string                `json:"title"`
		Statement string                `json:"statement"`
		Subject   string                `json:"subject"`
		Predicate string                `json:"predicate"`
		Value     domain.TypedValue     `json:"value"`
		Scope     domain.KnowledgeScope `json:"scope"`
		Evidence  []domain.EvidenceRef  `json:"evidence"`
	}{candidate.Kind, candidate.Title, candidate.Statement, candidate.Subject, candidate.Predicate, candidate.Value, candidate.Scope, candidate.Evidence}
	body, _ := json.Marshal(hashInput)
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	id := candidate.Kind + ":" + semanticSlug(candidate.Subject+"-"+candidate.Predicate) + "-" + hash[:12]
	dimensions := inferKnowledgeDimensions(candidate.Title, candidate.Subject, candidate.Predicate, candidate.Statement)
	layers := inferKnowledgeLayers(candidate.Kind, candidate.RiskLevel, dimensions)
	return LocalKnowledgeItem{
		ID: id, Kind: candidate.Kind, Title: strings.TrimSpace(candidate.Title), Statement: strings.TrimSpace(candidate.Statement), Subject: strings.TrimSpace(candidate.Subject), Predicate: strings.TrimSpace(candidate.Predicate), Value: candidate.Value,
		Scope: normalizeKnowledgeScope(candidate.Scope), Status: "candidate", RiskLevel: candidate.RiskLevel, AllowedChannels: uniqueStrings(candidate.AllowedChannels), Evidence: candidate.Evidence, EvidenceIDs: evidenceIDs,
		ForbiddenExtensions: uniqueStrings(candidate.ForbiddenExtensions), DependsOnFactIDs: uniqueStrings(candidate.DependsOnFactIDs), Dimensions: dimensions, Layers: layers,
		ValidFrom: candidate.ValidFrom, ValidUntil: candidate.ValidUntil, ExpiresAt: candidate.ExpiresAt, OriginRunID: runID, ContentHash: "sha256:" + hash, CreatedAt: now, UpdatedAt: now,
	}
}

func loadLocalKnowledgeItems(root string) ([]LocalKnowledgeItem, []string, error) {
	files := []string{}
	for _, directory := range []string{"facts", "claims"} {
		matches, err := filepath.Glob(filepath.Join(root, "knowledge", directory, "*.json"))
		if err != nil {
			return nil, nil, err
		}
		files = append(files, matches...)
	}
	sort.Strings(files)
	items := make([]LocalKnowledgeItem, 0, len(files))
	paths := make([]string, 0, len(files))
	for _, path := range files {
		var item LocalKnowledgeItem
		if err := readJSON(path, &item); err != nil {
			return nil, nil, err
		}
		items = append(items, item)
		paths = append(paths, relativeWorkspacePath(root, path))
	}
	return items, paths, nil
}

func loadKnowledgeReferenceIndex(root string) (map[string]LocalKnowledgeItem, error) {
	items, _, err := loadLocalKnowledgeItems(root)
	if err != nil {
		return nil, err
	}
	index := map[string]LocalKnowledgeItem{}
	for _, item := range items {
		index[item.ID] = item
	}
	for _, directory := range []string{"assets", "rights", "conflicts"} {
		files, err := filepath.Glob(filepath.Join(root, "knowledge", directory, "*.json"))
		if err != nil {
			return nil, err
		}
		for _, path := range files {
			var object struct {
				ID     string `json:"id"`
				Kind   string `json:"kind"`
				Status string `json:"status"`
			}
			if err := readJSON(path, &object); err != nil {
				return nil, err
			}
			if object.ID != "" {
				index[object.ID] = LocalKnowledgeItem{ID: object.ID, Kind: object.Kind, Status: object.Status}
			}
		}
	}
	return index, nil
}

func knowledgeBlockReasons(item LocalKnowledgeItem, channel string, at time.Time, refs map[string]LocalKnowledgeItem, eligible map[string]bool, hasSnapshot bool) []string {
	reasons := []string{}
	switch item.Status {
	case "blocked", "conflicted", "expired", "prohibited", "superseded":
		reasons = append(reasons, "status="+item.Status)
	}
	if item.ValidFrom != nil && at.Before(item.ValidFrom.UTC()) {
		reasons = append(reasons, "尚未到生效时间")
	}
	if item.ValidUntil != nil && at.After(item.ValidUntil.UTC()) {
		reasons = append(reasons, "已超过 valid_until")
	}
	if item.ExpiresAt != nil && at.After(item.ExpiresAt.UTC()) {
		reasons = append(reasons, "已过期")
	}
	if channel != "" && len(item.AllowedChannels) > 0 && !containsString(item.AllowedChannels, channel) {
		reasons = append(reasons, "不允许用于渠道 "+channel)
	}
	for _, dependency := range item.DependsOnFactIDs {
		if _, ok := refs[dependency]; !ok {
			reasons = append(reasons, "依赖缺失 "+dependency)
		} else if hasSnapshot && !eligible[dependency] {
			reasons = append(reasons, "依赖未进入 ApprovedSnapshot "+dependency)
		}
	}
	for _, conflict := range item.ConflictRefs {
		if value, ok := refs[conflict]; !ok || value.Status != "resolved" {
			reasons = append(reasons, "冲突未解决 "+conflict)
		}
	}
	for _, rights := range item.RightsRefs {
		if value, ok := refs[rights]; !ok || (value.Status != "valid" && value.Status != "approved") {
			reasons = append(reasons, "权利记录不可用 "+rights)
		}
	}
	if !hasSnapshot && item.Status == "candidate" && item.RiskLevel == "high" {
		reasons = append(reasons, "高风险候选必须先完成人工审批")
	}
	return uniqueStrings(reasons)
}

func latestKnowledgeSnapshot(root string) (domain.ApprovedSnapshot, []LocalKnowledgeItem, bool, error) {
	files, err := filepath.Glob(filepath.Join(root, ".contentcloud", "cache", "approved", "*", "snapshot.json"))
	if err != nil {
		return domain.ApprovedSnapshot{}, nil, false, err
	}
	var latest domain.ApprovedSnapshot
	found := false
	for _, path := range files {
		var snapshot domain.ApprovedSnapshot
		if err := readJSON(path, &snapshot); err != nil {
			return latest, nil, false, err
		}
		if snapshot.SubmissionType != "knowledge" {
			continue
		}
		if !found || snapshot.CreatedAt.After(latest.CreatedAt) {
			latest = snapshot
			found = true
		}
	}
	if !found {
		return latest, []LocalKnowledgeItem{}, false, nil
	}
	var canonical struct {
		Objects json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(latest.CanonicalContent, &canonical); err != nil {
		return latest, nil, false, domain.Invalid("APPROVED_SNAPSHOT_CONTENT_INVALID", "ApprovedSnapshot canonical_content 无效")
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(canonical.Objects, &raws); err != nil {
		return latest, nil, false, domain.Invalid("APPROVED_SNAPSHOT_OBJECTS_INVALID", "ApprovedSnapshot objects 无效")
	}
	items := []LocalKnowledgeItem{}
	for _, raw := range raws {
		var item LocalKnowledgeItem
		if json.Unmarshal(raw, &item) == nil && item.ID != "" && validKnowledgeKind(item.Kind) {
			item.ApprovalSnapshotID = latest.ID
			items = append(items, item)
		}
	}
	return latest, items, true, nil
}

func knowledgeSourceDisclosures(root string, items []LocalKnowledgeItem) ([]domain.SourceDisclosure, error) {
	sources, err := LocalSources(root)
	if err != nil {
		return nil, err
	}
	needed := map[string]bool{}
	for _, item := range items {
		for _, ref := range item.Evidence {
			needed[ref.SourceRevisionID] = true
		}
	}
	result := []domain.SourceDisclosure{}
	for _, source := range sources {
		if !needed[source.ID] {
			continue
		}
		var evidencePack json.RawMessage
		if source.EvidencePath != "" {
			evidencePack, err = os.ReadFile(filepath.Join(root, filepath.FromSlash(source.EvidencePath)))
			if err != nil {
				return nil, err
			}
		}
		result = append(result, domain.SourceDisclosure{SourceRef: source.ID, Level: "evidence_pack", SHA256: source.SHA256, ByteSize: source.ByteSize, EvidencePack: evidencePack})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].SourceRef < result[j].SourceRef })
	return result, nil
}

func inferKnowledgeDimensions(values ...string) []string {
	text := strings.ToLower(strings.Join(values, " "))
	result := []string{}
	for _, dimension := range knowledgeDimensions {
		for _, keyword := range dimension.Keywords {
			if strings.Contains(text, strings.ToLower(keyword)) {
				result = append(result, dimension.Key)
				break
			}
		}
	}
	return result
}

func inferKnowledgeLayers(kind, risk string, dimensions []string) []string {
	result := []string{}
	for _, dimension := range dimensions {
		switch dimension {
		case "theme-subbrand", "culture-story":
			result = append(result, "identity")
		case "scent-formula", "category", "form", "spec-cost-price":
			result = append(result, "product")
		case "customer-pain", "customer-solution", "benchmark", "competitors", "sales-channel", "usage-scenario", "solution-value":
			result = append(result, "market")
		case "materials-factories", "packaging-assembly":
			result = append(result, "operations")
		}
	}
	if kind == "visual_rule" {
		result = append(result, "expression")
	}
	if kind == "methodology" || containsString(dimensions, "benchmark") {
		result = append(result, "content_engine")
	}
	if kind == "claim" || risk == "high" {
		result = append(result, "compliance")
	}
	if len(result) == 0 {
		if kind == "fact" {
			result = append(result, "product")
		} else {
			result = append(result, "expression")
		}
	}
	return uniqueStrings(result)
}

func normalizeKnowledgeScope(scope domain.KnowledgeScope) domain.KnowledgeScope {
	scope.Regions = uniqueStrings(scope.Regions)
	scope.Channels = uniqueStrings(scope.Channels)
	scope.Audiences = uniqueStrings(scope.Audiences)
	scope.ProductVariants = uniqueStrings(scope.ProductVariants)
	return scope
}

func validKnowledgeKind(value string) bool {
	return value == "fact" || value == "claim" || value == "visual_rule" || value == "methodology"
}

func validLocalKnowledgeStatus(value string) bool {
	switch value {
	case "candidate", "review_ready", "verified", "approved", "valid", "conflicted", "expired", "blocked", "prohibited", "superseded", "informational", "resolved":
		return true
	default:
		return false
	}
}

func ResolveWorkspaceFile(root, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", domain.Invalid("LOCAL_FILE_REQUIRED", "必须指定工作区内文件")
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootPath, filepath.FromSlash(path))
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", domain.Policy("LOCAL_FILE_OUTSIDE_WORKSPACE", "文件必须位于当前工作区", "将 Agent 输出写入工作区后再导入")
	}
	return filepath.Clean(resolvedPath), nil
}

func resolveWorkspaceFile(root, value string) (string, error) {
	return ResolveWorkspaceFile(root, value)
}

func relativeWorkspacePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func semanticSlug(value string) string {
	slug := localSafeName(strings.ToLower(strings.TrimSpace(value)))
	if len(slug) > 72 {
		slug = slug[:72]
	}
	return slug
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func allUnique(values []string) bool {
	return len(uniqueStrings(values)) == len(values)
}
