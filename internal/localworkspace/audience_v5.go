package localworkspace

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type V5LintIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type V5LintReport struct {
	Valid        bool          `json:"valid"`
	File         string        `json:"file"`
	ObjectID     string        `json:"object_id,omitempty"`
	ObjectType   string        `json:"object_type,omitempty"`
	ContentHash  string        `json:"content_hash,omitempty"`
	LockedDigest string        `json:"locked_digest,omitempty"`
	Issues       []V5LintIssue `json:"issues"`
}

type CreateDouyinTaxonomyOptions struct {
	Root            string
	ID              string
	TaxonomyVersion string
	SourceURL       string
	SourceSHA256    string
	CapturedAt      time.Time
	EffectiveFrom   time.Time
	ExpiresAt       time.Time
}

func CreateDouyinAudienceTaxonomy(options CreateDouyinTaxonomyOptions) (string, domain.AudienceTaxonomySnapshot, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return "", domain.AudienceTaxonomySnapshot{}, err
	}
	if options.ID == "" {
		options.ID = domain.NewID()
	}
	if localSafeName(options.ID) != options.ID {
		return "", domain.AudienceTaxonomySnapshot{}, domain.Invalid("AUDIENCE_TAXONOMY_ID_INVALID", "人群目录 ID 只能包含字母、数字、点、下划线和连字符")
	}
	if options.CapturedAt.IsZero() {
		options.CapturedAt = time.Now().UTC()
	}
	if options.EffectiveFrom.IsZero() {
		options.EffectiveFrom = options.CapturedAt
	}
	taxonomy := domain.AudienceTaxonomySnapshot{
		ID: options.ID, Type: "audience_taxonomy_snapshot", SchemaVersion: domain.AudienceTaxonomySchema,
		Provider: "oceanengine_yuntu", TaxonomyID: "douyin-commerce-eight-audiences", TaxonomyVersion: strings.TrimSpace(options.TaxonomyVersion),
		Segments: domain.DefaultDouyinAudienceSegments(), SourceURL: strings.TrimSpace(options.SourceURL), CapturedAt: options.CapturedAt.UTC(),
		EffectiveFrom: options.EffectiveFrom.UTC(), ExpiresAt: options.ExpiresAt.UTC(), VerificationStatus: "unverified", SourceSHA256: strings.ToLower(strings.TrimSpace(options.SourceSHA256)), Status: "candidate",
	}
	if err := taxonomy.Validate(options.CapturedAt.UTC(), false); err != nil {
		return "", taxonomy, err
	}
	path := filepath.Join(root, "50-production", "strategies", taxonomy.ID+".json")
	if err := writeJSON(path, taxonomy); err != nil {
		return "", taxonomy, err
	}
	return relativeWorkspacePath(root, path), taxonomy, nil
}

type ScaffoldAudienceStrategiesOptions struct {
	Root               string
	TaxonomySnapshotID string
	Mode               string
	AudienceCodes      []string
	Objective          string
	TestType           string
	PrimaryVariable    string
}

func ScaffoldAudienceStrategies(options ScaffoldAudienceStrategiesOptions) ([]string, []domain.AudienceStrategyVersion, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return nil, nil, err
	}
	raw, _, err := latestApprovedObject(root, "strategy", options.TaxonomySnapshotID)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil, nil, domain.Policy("AUDIENCE_TAXONOMY_PULL_REQUIRED", "本机没有指定的已批准人群目录", "先执行 contentcloud pull approved --type strategy")
		}
		return nil, nil, err
	}
	var taxonomy domain.AudienceTaxonomySnapshot
	if err := json.Unmarshal(raw, &taxonomy); err != nil || taxonomy.Type != "audience_taxonomy_snapshot" {
		return nil, nil, domain.Invalid("AUDIENCE_TAXONOMY_INVALID", "批准快照对象不是有效人群目录")
	}
	if err := taxonomy.Validate(time.Now().UTC(), true); err != nil {
		return nil, nil, err
	}
	selected, err := selectAudienceSegments(taxonomy.Segments, options.Mode, options.AudienceCodes)
	if err != nil {
		return nil, nil, err
	}
	if options.TestType == "" {
		if options.Mode == "explore" {
			options.TestType = "exploration_batch"
		} else {
			options.TestType = "audience_expression_fit_test"
		}
	}
	if options.PrimaryVariable == "" {
		options.PrimaryVariable = "audience"
	}
	status, err := LoadStatus(root)
	if err != nil {
		return nil, nil, err
	}
	paths := make([]string, 0, len(selected))
	strategies := make([]domain.AudienceStrategyVersion, 0, len(selected))
	for _, segment := range selected {
		strategy := domain.AudienceStrategyVersion{
			ID: domain.NewID(), Type: "audience_strategy_version", SchemaVersion: domain.AudienceStrategySchema, ProjectID: status.Binding.ProjectID,
			TaxonomySnapshotID: taxonomy.ID, AudienceCode: segment.Code, AudienceLabel: segment.Label, SegmentDefinition: segment.Definition,
			Objective: strings.TrimSpace(options.Objective), HookHypotheses: []string{}, ProofOrder: []string{}, Objections: []string{}, EvidenceRefs: []string{},
			Confidence: "low", TestType: options.TestType, PrimaryVariable: options.PrimaryVariable, ControlledVariables: []string{}, TargetMetrics: []string{}, Constraints: []string{}, Status: "candidate",
		}
		path := filepath.Join(root, "50-production", "strategies", strategy.ID+".json")
		if err := writeJSON(path, strategy); err != nil {
			return nil, nil, err
		}
		paths = append(paths, relativeWorkspacePath(root, path))
		strategies = append(strategies, strategy)
	}
	return paths, strategies, nil
}

func LintAudienceTaxonomy(root, file string, now time.Time) (V5LintReport, domain.AudienceTaxonomySnapshot, error) {
	resolved, path, err := resolveV5JSON(root, file)
	if err != nil {
		return V5LintReport{}, domain.AudienceTaxonomySnapshot{}, err
	}
	var value domain.AudienceTaxonomySnapshot
	if err := readStrictJSON(path, &value); err != nil {
		return V5LintReport{}, value, domain.Invalid("AUDIENCE_TAXONOMY_JSON_INVALID", err.Error())
	}
	report := v5Report(resolved, path, value.ID, value.Type)
	if err := value.Validate(now.UTC(), true); err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: domainErrorCode(err), Message: err.Error()})
	}
	return finishV5Report(report, value), value, nil
}

func LintAudienceStrategy(root, file string, now time.Time) (V5LintReport, domain.AudienceStrategyVersion, error) {
	resolved, path, err := resolveV5JSON(root, file)
	if err != nil {
		return V5LintReport{}, domain.AudienceStrategyVersion{}, err
	}
	var value domain.AudienceStrategyVersion
	if err := readStrictJSON(path, &value); err != nil {
		return V5LintReport{}, value, domain.Invalid("AUDIENCE_STRATEGY_JSON_INVALID", err.Error())
	}
	report := v5Report(resolved, path, value.ID, value.Type)
	if err := value.Validate(true); err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: domainErrorCode(err), Message: err.Error()})
	}
	raw, _, err := latestApprovedObject(resolved, "strategy", value.TaxonomySnapshotID)
	if err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: "AUDIENCE_TAXONOMY_BASE_SNAPSHOT_REQUIRED", Message: "本机没有策略引用的 taxonomy ApprovedSnapshot；先执行 contentcloud pull approved --type strategy"})
	} else {
		var taxonomy domain.AudienceTaxonomySnapshot
		if err := json.Unmarshal(raw, &taxonomy); err != nil || taxonomy.Type != "audience_taxonomy_snapshot" {
			report.Issues = append(report.Issues, V5LintIssue{Code: "AUDIENCE_TAXONOMY_BASE_SNAPSHOT_INVALID", Message: "taxonomy_snapshot_id 未引用有效 AudienceTaxonomySnapshot"})
		} else if err := value.ValidateAgainstTaxonomy(taxonomy, now.UTC()); err != nil {
			report.Issues = append(report.Issues, V5LintIssue{Code: domainErrorCode(err), Message: err.Error()})
		}
	}
	return finishV5Report(report, value), value, nil
}

func LintCommerceOffer(root, file string, at time.Time) (V5LintReport, domain.CommerceOfferSnapshot, error) {
	resolved, path, err := resolveV5JSON(root, file)
	if err != nil {
		return V5LintReport{}, domain.CommerceOfferSnapshot{}, err
	}
	var value domain.CommerceOfferSnapshot
	if err := readStrictJSON(path, &value); err != nil {
		return V5LintReport{}, value, domain.Invalid("COMMERCE_OFFER_JSON_INVALID", err.Error())
	}
	report := v5Report(resolved, path, value.ID, value.Type)
	if err := value.Validate(at.UTC(), true); err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: domainErrorCode(err), Message: err.Error()})
	}
	return finishV5Report(report, value), value, nil
}

func selectAudienceSegments(all []domain.AudienceSegment, mode string, codes []string) ([]domain.AudienceSegment, error) {
	mode = strings.TrimSpace(mode)
	if mode != "single" && mode != "compare" && mode != "explore" {
		return nil, domain.Invalid("AUDIENCE_MODE_INVALID", "mode 只允许 single、compare 或 explore")
	}
	if mode == "explore" {
		return append([]domain.AudienceSegment(nil), all...), nil
	}
	if (mode == "single" && len(codes) != 1) || (mode == "compare" && (len(codes) < 2 || len(codes) > 3)) {
		return nil, domain.Invalid("AUDIENCE_SELECTION_INVALID", "single 必须选择 1 类，compare 必须选择 2 至 3 类")
	}
	index := map[string]domain.AudienceSegment{}
	for _, segment := range all {
		index[segment.Code] = segment
	}
	selected := []domain.AudienceSegment{}
	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code] {
			return nil, domain.Invalid("AUDIENCE_SELECTION_DUPLICATE", "人群代码不能重复")
		}
		segment, ok := index[code]
		if !ok {
			return nil, domain.Invalid("AUDIENCE_CODE_UNKNOWN", "人群代码不在已批准目录中："+code)
		}
		seen[code] = true
		selected = append(selected, segment)
	}
	return selected, nil
}

func resolveV5JSON(root, file string) (string, string, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return "", "", err
	}
	path, err := ResolveWorkspaceFile(resolved, file)
	return resolved, path, err
}

func v5Report(root, path, id, objectType string) V5LintReport {
	return V5LintReport{Valid: true, File: relativeWorkspacePath(root, path), ObjectID: id, ObjectType: objectType, Issues: []V5LintIssue{}}
}

func finishV5Report(report V5LintReport, value any) V5LintReport {
	hash, err := domain.CanonicalHash(value)
	if err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: "CONTENT_HASH_FAILED", Message: err.Error()})
	} else {
		report.ContentHash = "sha256:" + hash
	}
	report.Valid = len(report.Issues) == 0
	return report
}

func domainErrorCode(err error) string {
	var value *domain.Error
	if errors.As(err, &value) {
		return value.Code
	}
	return "VALIDATION_FAILED"
}
