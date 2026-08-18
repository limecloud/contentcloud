package localworkspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

const (
	MemoryPromotionSchema      = "contentcloud.memory-promotion/1.0"
	memoryPromotionRelativeDir = "30-knowledge/imports"
)

type MemoryPromoteOptions struct {
	Root            string
	MemoryID        string
	KnowledgeKind   string
	Title           string
	Subject         string
	Predicate       string
	RiskLevel       string
	AllowedChannels []string
	EvidenceIDs     []string
	OriginRunID     string
	Now             time.Time
}

type MemoryPromotionReport struct {
	SchemaVersion string               `json:"schema_version"`
	MemoryID      string               `json:"memory_id"`
	Record        MemoryRecord         `json:"record"`
	KnowledgeID   string               `json:"knowledge_id"`
	PackageRef    string               `json:"package_ref"`
	Imported      []LocalKnowledgeItem `json:"imported"`
	Skipped       []string             `json:"skipped"`
	Warnings      []string             `json:"warnings"`
	PromotedAt    time.Time            `json:"promoted_at"`
}

// PromoteMemory converts one active, non-conflicted memory candidate into the
// existing knowledge-candidate import path. It never creates an approved
// snapshot; review and approval remain separate domain operations.
func PromoteMemory(options MemoryPromoteOptions) (MemoryPromotionReport, error) {
	root, scope, err := resolveMemoryScope(options.Root)
	if err != nil {
		return MemoryPromotionReport{}, err
	}
	memoryID := strings.TrimSpace(options.MemoryID)
	if memoryID == "" || memoryID != localSafeName(memoryID) || memoryID == "." || memoryID == ".." || strings.ContainsAny(memoryID, `/\\`) {
		return MemoryPromotionReport{}, fault.Invalid("MEMORY_PROMOTION_ID_INVALID", "必须指定有效的记忆候选 ID")
	}
	catalog, err := scanMemoryCatalog(root, scope)
	if err != nil {
		return MemoryPromotionReport{}, err
	}
	var record MemoryRecord
	found := false
	for _, file := range catalog.Records {
		if file.Record.MemoryID == memoryID {
			record = file.Record
			found = true
			break
		}
	}
	if !found {
		return MemoryPromotionReport{}, fault.NotFound("本地记忆候选")
	}
	if record.Status != "active" {
		return MemoryPromotionReport{}, fault.Policy("MEMORY_PROMOTION_NOT_ELIGIBLE", "陈旧、冲突或非 active 的记忆候选不能晋升", "先修复来源或解决记忆冲突后重试")
	}
	kind := strings.ToLower(strings.TrimSpace(options.KnowledgeKind))
	if !validKnowledgeKind(kind) {
		return MemoryPromotionReport{}, fault.Invalid("MEMORY_PROMOTION_KIND_INVALID", "knowledge_kind 必须为 fact、claim、visual_rule 或 methodology")
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		title = truncateRunes(record.Summary, 120)
	}
	subject := strings.TrimSpace(options.Subject)
	predicate := strings.TrimSpace(options.Predicate)
	if subject == "" || predicate == "" {
		return MemoryPromotionReport{}, fault.Invalid("MEMORY_PROMOTION_SUBJECT_REQUIRED", "显式晋升必须提供 subject 和 predicate")
	}
	risk := strings.ToLower(strings.TrimSpace(options.RiskLevel))
	if risk == "" {
		risk = "low"
	}
	if risk != "low" && risk != "medium" && risk != "high" {
		return MemoryPromotionReport{}, fault.Invalid("MEMORY_PROMOTION_RISK_INVALID", "risk_level 必须为 low、medium 或 high")
	}
	evidence, err := memoryPromotionEvidence(root, options.EvidenceIDs)
	if err != nil {
		return MemoryPromotionReport{}, err
	}
	candidate := sourcedomain.KnowledgeCandidate{
		Kind:                kind,
		Title:               title,
		Statement:           record.Summary,
		Subject:             subject,
		Predicate:           predicate,
		Value:               sourcedomain.TypedValue{Type: "text", Text: record.Summary},
		Scope:               sourcedomain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}},
		RiskLevel:           risk,
		AllowedChannels:     sortedNonNilStrings(options.AllowedChannels),
		Evidence:            evidence,
		ForbiddenExtensions: []string{},
		DependsOnFactIDs:    []string{},
	}
	pkg := sourcedomain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []sourcedomain.KnowledgeCandidate{candidate}, Warnings: []string{}}
	if err := validateKnowledgeCandidates(pkg); err != nil {
		return MemoryPromotionReport{}, err
	}
	packagePath := filepath.Join(root, filepath.FromSlash(memoryPromotionRelativeDir), localSafeName("memory-"+memoryID)+".json")
	packageRef := relativeWorkspacePath(root, packagePath)
	if err := writeMemoryPromotionPackage(packagePath, pkg); err != nil {
		return MemoryPromotionReport{}, err
	}
	imported, err := ImportKnowledgeCandidates(ImportKnowledgeOptions{Root: root, PackageFile: packageRef, OriginRunID: defaultLocalValue(strings.TrimSpace(options.OriginRunID), "memory:"+memoryID), Now: options.Now})
	if err != nil {
		return MemoryPromotionReport{}, err
	}
	knowledgeID := knowledgeItemFromCandidate(candidate, nil, "", normalizedMemoryTime(options.Now)).ID
	return MemoryPromotionReport{
		SchemaVersion: MemoryPromotionSchema,
		MemoryID:      memoryID,
		Record:        record,
		KnowledgeID:   knowledgeID,
		PackageRef:    packageRef,
		Imported:      imported.Imported,
		Skipped:       imported.Skipped,
		Warnings:      imported.Warnings,
		PromotedAt:    normalizedMemoryTime(options.Now),
	}, nil
}

func memoryPromotionEvidence(root string, ids []string) ([]sourcedomain.EvidenceRef, error) {
	cleanIDs := uniqueStrings(ids)
	if len(cleanIDs) == 0 {
		return nil, fault.Invalid("MEMORY_PROMOTION_EVIDENCE_REQUIRED", "晋升为正式知识候选必须指定已接受的 evidence_id")
	}
	index, err := loadEvidenceIndex(root)
	if err != nil {
		return nil, err
	}
	byID := map[string]LocalEvidence{}
	for _, spans := range index {
		for _, span := range spans {
			byID[span.ID] = span
		}
	}
	result := make([]sourcedomain.EvidenceRef, 0, len(cleanIDs))
	for _, id := range cleanIDs {
		span, ok := byID[id]
		if !ok {
			return nil, fault.Invalid("MEMORY_PROMOTION_EVIDENCE_NOT_FOUND", "evidence_id 不存在："+id)
		}
		if span.ReviewStatus != "accepted" {
			return nil, fault.Policy("MEMORY_PROMOTION_EVIDENCE_REVIEW_REQUIRED", "记忆候选引用的 evidence 尚未通过人工复核："+id, "先执行来源复核，再晋升记忆")
		}
		locator, err := json.Marshal(span.Locator)
		if err != nil {
			return nil, err
		}
		result = append(result, sourcedomain.EvidenceRef{SourceRevisionID: span.SourceID, LocatorKind: span.LocatorKind, Locator: string(locator), Quote: span.Quote})
	}
	return result, nil
}

func writeMemoryPromotionPackage(path string, pkg sourcedomain.KnowledgeExtractionPackage) error {
	body, err := json.Marshal(pkg)
	if err != nil {
		return err
	}
	if existing, readErr := os.ReadFile(path); readErr == nil {
		var current sourcedomain.KnowledgeExtractionPackage
		if err := strictUnmarshal(existing, &current); err != nil {
			return fault.Conflict("MEMORY_PROMOTION_PACKAGE_CONFLICT", "已有记忆晋升包格式无效，拒绝覆盖："+filepath.Base(path))
		}
		currentBody, _ := json.Marshal(current)
		if string(currentBody) == string(body) {
			return nil
		}
		return fault.Conflict("MEMORY_PROMOTION_PACKAGE_CONFLICT", "已有记忆晋升包内容不同，拒绝覆盖："+filepath.Base(path))
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	return replaceJSON(path, pkg, 0o600)
}

func sortedNonNilStrings(values []string) []string {
	result := uniqueStrings(values)
	if result == nil {
		return []string{}
	}
	sort.Strings(result)
	return result
}
