package localworkspace

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	"gopkg.in/yaml.v3"
)

type knowledgePage struct {
	ID                  string                  `yaml:"id"`
	Type                string                  `yaml:"type"`
	Version             int                     `yaml:"version"`
	Supersedes          string                  `yaml:"supersedes,omitempty"`
	Status              string                  `yaml:"status"`
	Title               string                  `yaml:"title,omitempty"`
	Text                string                  `yaml:"text,omitempty"`
	Subject             string                  `yaml:"subject,omitempty"`
	Predicate           string                  `yaml:"predicate,omitempty"`
	Value               knowledgePageValue      `yaml:"value,omitempty"`
	Scope               knowledgePageScope      `yaml:"scope,omitempty"`
	RiskLevel           string                  `yaml:"risk_level,omitempty"`
	AllowedChannels     []string                `yaml:"allowed_channels"`
	SubjectRefs         []string                `yaml:"subject_refs"`
	AboutRefs           []string                `yaml:"about_refs"`
	SourceRefs          []string                `yaml:"source_refs"`
	EvidenceRefs        []string                `yaml:"evidence_refs"`
	Evidence            []knowledgePageEvidence `yaml:"evidence,omitempty"`
	DecisionRefs        []string                `yaml:"decision_refs"`
	ForbiddenExtensions []string                `yaml:"forbidden_extensions"`
	DependsOnFactIDs    []string                `yaml:"depends_on_fact_ids"`
	AssetRefs           []string                `yaml:"asset_refs"`
	RightsRefs          []string                `yaml:"rights_refs"`
	ConflictRefs        []string                `yaml:"conflict_refs"`
	Dimensions          []string                `yaml:"dimensions"`
	Layers              []string                `yaml:"layers"`
	ValidFrom           *time.Time              `yaml:"valid_from,omitempty"`
	ValidUntil          *time.Time              `yaml:"valid_until,omitempty"`
	ExpiresAt           *time.Time              `yaml:"expires_at,omitempty"`
	ApprovalSnapshotID  string                  `yaml:"approval_snapshot_id,omitempty"`
	OriginRunID         string                  `yaml:"origin_run_id,omitempty"`
	ContentHash         string                  `yaml:"content_hash,omitempty"`
	CreatedAt           time.Time               `yaml:"created_at,omitempty"`
	UpdatedAt           time.Time               `yaml:"updated_at,omitempty"`
}

type knowledgePageValue struct {
	Type    string   `yaml:"type,omitempty"`
	Text    string   `yaml:"text,omitempty"`
	Number  *float64 `yaml:"number,omitempty"`
	Boolean *bool    `yaml:"boolean,omitempty"`
	Unit    string   `yaml:"unit,omitempty"`
}

type knowledgePageScope struct {
	Regions         []string `yaml:"regions"`
	Channels        []string `yaml:"channels"`
	Audiences       []string `yaml:"audiences"`
	ProductVariants []string `yaml:"product_variants"`
}

type knowledgePageEvidence struct {
	SourceRevisionID string `yaml:"source_revision_id"`
	LocatorKind      string `yaml:"locator_kind"`
	Locator          string `yaml:"locator"`
	Quote            string `yaml:"quote"`
}

func writeKnowledgePage(path string, item LocalKnowledgeItem) error {
	page := knowledgePageFromItem(item)
	frontmatter, err := yaml.Marshal(page)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	body.WriteString("---\n")
	body.Write(frontmatter)
	body.WriteString("---\n\n# ")
	body.WriteString(defaultLocalValue(item.Title, item.ID))
	body.WriteString("\n\n")
	body.WriteString(strings.TrimSpace(item.Statement))
	body.WriteByte('\n')
	return replaceFile(path, body.Bytes(), 0o600)
}

func readKnowledgePage(path string) (LocalKnowledgeItem, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return LocalKnowledgeItem{}, err
	}
	frontmatter, err := splitKnowledgeFrontmatter(body)
	if err != nil {
		return LocalKnowledgeItem{}, fmt.Errorf("%s: %w", path, err)
	}
	var page knowledgePage
	decoder := yaml.NewDecoder(bytes.NewReader(frontmatter))
	decoder.KnownFields(true)
	if err := decoder.Decode(&page); err != nil {
		return LocalKnowledgeItem{}, fmt.Errorf("%s：知识页面前置信息无效：%w", path, err)
	}
	item, err := page.item()
	if err != nil {
		return LocalKnowledgeItem{}, fmt.Errorf("%s: %w", path, err)
	}
	return item, nil
}

func splitKnowledgeFrontmatter(body []byte) ([]byte, error) {
	normalized := bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil, errors.New("知识页面必须以 YAML 前置信息开头")
	}
	end := bytes.Index(normalized[4:], []byte("\n---\n"))
	if end < 0 {
		return nil, errors.New("知识页面的 YAML 前置信息未闭合")
	}
	return normalized[4 : 4+end], nil
}

func knowledgePageFromItem(item LocalKnowledgeItem) knowledgePage {
	version := item.Version
	if version < 1 {
		version = 1
	}
	sources := []string{}
	evidence := make([]knowledgePageEvidence, 0, len(item.Evidence))
	for _, ref := range item.Evidence {
		sources = append(sources, ref.SourceRevisionID)
		evidence = append(evidence, knowledgePageEvidence{SourceRevisionID: ref.SourceRevisionID, LocatorKind: ref.LocatorKind, Locator: ref.Locator, Quote: ref.Quote})
	}
	return knowledgePage{
		ID: item.ID, Type: knowledgeTypeForKind(item.Kind), Version: version, Supersedes: item.Supersedes, Status: item.Status, Title: item.Title, Text: item.Statement,
		Subject: item.Subject, Predicate: item.Predicate,
		Value:     knowledgePageValue{Type: item.Value.Type, Text: item.Value.Text, Number: item.Value.Number, Boolean: item.Value.Boolean, Unit: item.Value.Unit},
		Scope:     knowledgePageScope{Regions: item.Scope.Regions, Channels: item.Scope.Channels, Audiences: item.Scope.Audiences, ProductVariants: item.Scope.ProductVariants},
		RiskLevel: item.RiskLevel, AllowedChannels: nonNilStrings(item.AllowedChannels), SubjectRefs: []string{}, AboutRefs: []string{},
		SourceRefs: uniqueStrings(sources), EvidenceRefs: nonNilStrings(item.EvidenceIDs), Evidence: evidence, DecisionRefs: nonNilStrings(item.DecisionRefs),
		ForbiddenExtensions: nonNilStrings(item.ForbiddenExtensions), DependsOnFactIDs: nonNilStrings(item.DependsOnFactIDs), AssetRefs: nonNilStrings(item.AssetRefs), RightsRefs: nonNilStrings(item.RightsRefs), ConflictRefs: nonNilStrings(item.ConflictRefs),
		Dimensions: nonNilStrings(item.Dimensions), Layers: nonNilStrings(item.Layers), ValidFrom: item.ValidFrom, ValidUntil: item.ValidUntil, ExpiresAt: item.ExpiresAt,
		ApprovalSnapshotID: item.ApprovalSnapshotID, OriginRunID: item.OriginRunID, ContentHash: item.ContentHash, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func (page knowledgePage) item() (LocalKnowledgeItem, error) {
	kind, ok := knowledgeKindForType(page.Type)
	if !ok {
		return LocalKnowledgeItem{}, fault.Invalid("KNOWLEDGE_TYPE_INVALID", "不支持该知识页面类型："+page.Type)
	}
	if page.ID == "" || page.Version < 1 {
		return LocalKnowledgeItem{}, fault.Invalid("KNOWLEDGE_ID_VERSION_REQUIRED", "知识页面需要稳定的 id 和正整数 version")
	}
	if !validKnowledgePageStatus(page.Type, page.Status) {
		return LocalKnowledgeItem{}, fault.Invalid("KNOWLEDGE_STATUS_INVALID", page.Type+" 不允许 status="+page.Status)
	}
	evidence := make([]sourcedomain.EvidenceRef, 0, len(page.Evidence))
	for _, ref := range page.Evidence {
		evidence = append(evidence, sourcedomain.EvidenceRef{SourceRevisionID: ref.SourceRevisionID, LocatorKind: ref.LocatorKind, Locator: ref.Locator, Quote: ref.Quote})
	}
	return LocalKnowledgeItem{
		ID: page.ID, Version: page.Version, Supersedes: page.Supersedes, Kind: kind, Title: page.Title, Statement: page.Text, Subject: page.Subject, Predicate: page.Predicate,
		Value:  sourcedomain.TypedValue{Type: page.Value.Type, Text: page.Value.Text, Number: page.Value.Number, Boolean: page.Value.Boolean, Unit: page.Value.Unit},
		Scope:  sourcedomain.KnowledgeScope{Regions: nonNilStrings(page.Scope.Regions), Channels: nonNilStrings(page.Scope.Channels), Audiences: nonNilStrings(page.Scope.Audiences), ProductVariants: nonNilStrings(page.Scope.ProductVariants)},
		Status: page.Status, RiskLevel: page.RiskLevel, AllowedChannels: nonNilStrings(page.AllowedChannels), Evidence: evidence, EvidenceIDs: nonNilStrings(page.EvidenceRefs),
		ForbiddenExtensions: nonNilStrings(page.ForbiddenExtensions), DependsOnFactIDs: nonNilStrings(page.DependsOnFactIDs), AssetRefs: nonNilStrings(page.AssetRefs), RightsRefs: nonNilStrings(page.RightsRefs), ConflictRefs: nonNilStrings(page.ConflictRefs), DecisionRefs: nonNilStrings(page.DecisionRefs),
		Dimensions: nonNilStrings(page.Dimensions), Layers: nonNilStrings(page.Layers), ValidFrom: page.ValidFrom, ValidUntil: page.ValidUntil, ExpiresAt: page.ExpiresAt,
		ApprovalSnapshotID: page.ApprovalSnapshotID, OriginRunID: page.OriginRunID, ContentHash: page.ContentHash, CreatedAt: page.CreatedAt, UpdatedAt: page.UpdatedAt,
	}, nil
}

func knowledgePageFiles(root string, types ...string) ([]string, error) {
	allowed := map[string]bool{}
	for _, value := range types {
		allowed[value] = true
	}
	files := []string{}
	base := filepath.Join(root, "30-knowledge", "pages")
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			return nil
		}
		if len(allowed) > 0 {
			item, err := readKnowledgePage(path)
			if err != nil {
				return err
			}
			if !allowed[knowledgeTypeForKind(item.Kind)] {
				return nil
			}
		}
		files = append(files, path)
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	sort.Strings(files)
	return files, err
}

func knowledgeTypeForKind(kind string) string {
	switch kind {
	case "fact":
		return "FactAssertion"
	case "claim", "visual_rule":
		return "Claim"
	case "asset":
		return "Asset"
	case "rights":
		return "RightsRecord"
	case "conflict":
		return "Conflict"
	default:
		return "DomainObject"
	}
}

func knowledgeKindForType(value string) (string, bool) {
	switch value {
	case "FactAssertion":
		return "fact", true
	case "Claim":
		return "claim", true
	case "Asset":
		return "asset", true
	case "RightsRecord":
		return "rights", true
	case "Conflict":
		return "conflict", true
	case "Source", "Evidence", "DomainObject", "Audience", "Scenario", "BrandRule", "Process", "Campaign", "Learning":
		return "domain", true
	default:
		return "", false
	}
}

func validKnowledgePageStatus(kind, status string) bool {
	allowed := map[string]map[string]bool{
		"FactAssertion": {"candidate": true, "needs_review": true, "verified": true, "conflicted": true, "rejected": true, "superseded": true},
		"Claim":         {"candidate": true, "needs_review": true, "approved": true, "blocked": true, "rejected": true, "superseded": true},
		"RightsRecord":  {"candidate": true, "needs_review": true, "valid": true, "expired": true, "revoked": true, "rejected": true},
		"Conflict":      {"open": true, "needs_decision": true, "resolved": true, "superseded": true},
		"Asset":         {"candidate": true, "needs_review": true, "available": true, "blocked": true, "superseded": true},
	}
	if values := allowed[kind]; values != nil {
		return values[status]
	}
	return status == "candidate" || status == "needs_review" || status == "active" || status == "superseded"
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return uniqueStrings(values)
}
