package source

import "github.com/limecloud/contentcloud/internal/workspace"
import "github.com/limecloud/contentcloud/internal/catalog"

import "github.com/limecloud/contentcloud/internal/platform/stablehash"
import "github.com/limecloud/contentcloud/internal/platform/idgen"
import "github.com/limecloud/contentcloud/internal/platform/fault"
import "strings"
import "sort"
import "fmt"
import "time"

const (
	TaskContractSchema         = "task-contract/1.0"
	KnowledgeExtractCapability = "contentcloud.knowledge.extract"
	ArtifactExportCapability   = "contentcloud.artifact.export"
	ArtifactExportSchemaMD     = "contentcloud.content-delivery.markdown/3.0"
	ArtifactExportSchemaXLSX   = "contentcloud.content-delivery.xlsx/3.0"
)

type Source struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	Name           string    `json:"name"`
	SourceType     string    `json:"source_type"`
	Status         string    `json:"status"`
	RevisionCount  int       `json:"revision_count"`
	LatestRevision string    `json:"latest_revision_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type SourceRevision struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	ProjectID        string     `json:"project_id"`
	SourceID         string     `json:"source_id"`
	FileName         string     `json:"file_name"`
	ObjectKey        string     `json:"object_key"`
	SHA256           string     `json:"sha256"`
	ByteSize         int64      `json:"byte_size"`
	DeclaredMIME     string     `json:"declared_mime"`
	DetectedMIME     string     `json:"detected_mime"`
	ProcessingStatus string     `json:"processing_status"`
	ParserVersion    string     `json:"parser_version,omitempty"`
	ErrorCode        string     `json:"error_code,omitempty"`
	SupersedesID     string     `json:"supersedes_id,omitempty"`
	UploadedBy       string     `json:"uploaded_by,omitempty"`
	EffectiveFrom    *time.Time `json:"effective_from,omitempty"`
	EffectiveTo      *time.Time `json:"effective_to,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type EvidenceSpan struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	ProjectID     string         `json:"project_id"`
	RevisionID    string         `json:"revision_id"`
	LocatorKind   string         `json:"locator_kind"`
	Locator       map[string]any `json:"locator"`
	QuoteText     string         `json:"quote_text"`
	QuoteHash     string         `json:"quote_hash"`
	OCRConfidence *float64       `json:"ocr_confidence,omitempty"`
	ReviewStatus  string         `json:"review_status"`
	ReviewedBy    string         `json:"reviewed_by,omitempty"`
	ReviewedAt    *time.Time     `json:"reviewed_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}

type ContractEvidence struct {
	ID          string         `json:"id"`
	LocatorKind string         `json:"locator_kind"`
	Locator     map[string]any `json:"locator"`
	Quote       string         `json:"quote"`
	QuoteHash   string         `json:"quote_hash"`
}

type TypedValue struct {
	Type    string   `json:"type"`
	Text    string   `json:"text,omitempty"`
	Number  *float64 `json:"number,omitempty"`
	Boolean *bool    `json:"boolean,omitempty"`
	Unit    string   `json:"unit,omitempty"`
}

type TaskContract struct {
	ContractVersion string             `json:"contract_version"`
	ContractID      string             `json:"contract_id"`
	RunID           string             `json:"run_id"`
	TaskType        string             `json:"task_type"`
	Project         workspace.Project  `json:"project"`
	Sources         []ContractSource   `json:"sources"`
	InputSnapshotID string             `json:"input_snapshot_id"`
	OutputSchema    string             `json:"output_schema"`
	Capability      catalog.Capability `json:"required_capability"`
	ManifestHash    string             `json:"manifest_hash"`
}

type ContractSource struct {
	SourceID     string             `json:"source_id"`
	RevisionID   string             `json:"revision_id"`
	Name         string             `json:"name"`
	SourceType   string             `json:"source_type"`
	FileName     string             `json:"file_name"`
	SHA256       string             `json:"sha256"`
	DetectedMIME string             `json:"detected_mime"`
	Evidence     []ContractEvidence `json:"evidence"`
}

type KnowledgeCandidate struct {
	Kind                string         `json:"kind"`
	Title               string         `json:"title"`
	Statement           string         `json:"statement"`
	Subject             string         `json:"subject"`
	Predicate           string         `json:"predicate"`
	Value               TypedValue     `json:"value"`
	Scope               KnowledgeScope `json:"scope"`
	RiskLevel           string         `json:"risk_level"`
	AllowedChannels     []string       `json:"allowed_channels"`
	Evidence            []EvidenceRef  `json:"evidence"`
	ForbiddenExtensions []string       `json:"forbidden_extensions"`
	DependsOnFactIDs    []string       `json:"depends_on_fact_ids"`
	ValidFrom           *time.Time     `json:"valid_from,omitempty"`
	ValidUntil          *time.Time     `json:"valid_until,omitempty"`
	ExpiresAt           *time.Time     `json:"expires_at,omitempty"`
}

type KnowledgeExtractionPackage struct {
	SchemaVersion string               `json:"schema_version"`
	Candidates    []KnowledgeCandidate `json:"candidates"`
	Warnings      []string             `json:"warnings"`
}

type KnowledgeExtractionResult struct {
	RunID    string            `json:"run_id"`
	Objects  []KnowledgeObject `json:"objects"`
	Warnings []string          `json:"warnings"`
}

type Asset struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	ProjectID        string    `json:"project_id"`
	Name             string    `json:"name"`
	AssetType        string    `json:"asset_type"`
	SourceRevisionID string    `json:"source_revision_id"`
	UsageMode        string    `json:"usage_mode"`
	Status           string    `json:"status"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type RightsRecord struct {
	ID                    string     `json:"id"`
	TenantID              string     `json:"tenant_id"`
	ProjectID             string     `json:"project_id"`
	AssetID               string     `json:"asset_id"`
	RightsHolder          string     `json:"rights_holder"`
	RightsType            string     `json:"rights_type"`
	Territories           []string   `json:"territories"`
	Channels              []string   `json:"channels"`
	ValidFrom             *time.Time `json:"valid_from,omitempty"`
	ValidUntil            *time.Time `json:"valid_until,omitempty"`
	ProofSourceRevisionID string     `json:"proof_source_revision_id"`
	Restrictions          []string   `json:"restrictions"`
	Status                string     `json:"status"`
	ReviewedBy            string     `json:"reviewed_by,omitempty"`
	ReviewedAt            *time.Time `json:"reviewed_at,omitempty"`
	RowVersion            int        `json:"row_version"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type AssetBundle struct {
	Asset  Asset        `json:"asset"`
	Rights RightsRecord `json:"rights"`
}

const (
	KnowledgeObjectSchema   = "contentcloud.knowledge-object/1.0"
	KnowledgeSnapshotSchema = "contentcloud.knowledge-snapshot/1.0"
)

var KnowledgeLayers = []string{
	"identity",
	"product",
	"market",
	"expression",
	"operations",
	"content_engine",
	"compliance",
}

var KnowledgeEligibleStatuses = []string{"verified", "approved", "valid", "active"}

// KnowledgeObject is the server-side representation of a typed knowledge fact.
// The payload carries type-specific fields while governance references stay
// explicit and queryable.
type KnowledgeObject struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	ProjectID       string         `json:"project_id"`
	ObjectType      string         `json:"object_type"`
	Layer           string         `json:"layer"`
	Version         int            `json:"version"`
	Status          string         `json:"status"`
	Title           string         `json:"title"`
	Statement       string         `json:"statement"`
	Payload         map[string]any `json:"payload"`
	Dimensions      []string       `json:"dimensions"`
	AllowedChannels []string       `json:"allowed_channels"`
	EvidenceRefs    []string       `json:"evidence_refs"`
	RelationRefs    []string       `json:"relation_refs"`
	RightsRefs      []string       `json:"rights_refs"`
	ConflictRefs    []string       `json:"conflict_refs"`
	DecisionRef     string         `json:"decision_ref,omitempty"`
	NextAction      string         `json:"next_action,omitempty"`
	Impact          string         `json:"impact,omitempty"`
	ValidFrom       *time.Time     `json:"valid_from,omitempty"`
	ValidUntil      *time.Time     `json:"valid_until,omitempty"`
	ExpiresAt       *time.Time     `json:"expires_at,omitempty"`
	Digest          string         `json:"digest"`
	CreatedBy       string         `json:"created_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type KnowledgePackObjectRef struct {
	ObjectID string `json:"object_id"`
	Version  int    `json:"version"`
}

type KnowledgeQueryPolicy struct {
	EligibleStatuses   []string `json:"eligible_statuses"`
	AllowedObjectTypes []string `json:"allowed_object_types"`
	RequireEvidence    bool     `json:"require_evidence"`
	BlockOnConflict    bool     `json:"block_on_conflict"`
	BlockOnRights      bool     `json:"block_on_rights_failure"`
}

func DefaultKnowledgeQueryPolicy() KnowledgeQueryPolicy {
	return KnowledgeQueryPolicy{
		EligibleStatuses: append([]string(nil), KnowledgeEligibleStatuses...),
		RequireEvidence:  true,
		BlockOnConflict:  true,
		BlockOnRights:    true,
	}
}

type KnowledgePack struct {
	ID          string                   `json:"id"`
	TenantID    string                   `json:"tenant_id"`
	ProjectID   string                   `json:"project_id"`
	Name        string                   `json:"name"`
	Purpose     string                   `json:"purpose"`
	Version     int                      `json:"version"`
	Status      string                   `json:"status"`
	ObjectRefs  []KnowledgePackObjectRef `json:"object_refs"`
	QueryPolicy KnowledgeQueryPolicy     `json:"query_policy"`
	Digest      string                   `json:"digest"`
	CreatedBy   string                   `json:"created_by"`
	PublishedBy string                   `json:"published_by,omitempty"`
	CreatedAt   time.Time                `json:"created_at"`
	PublishedAt *time.Time               `json:"published_at,omitempty"`
}

type KnowledgeSnapshot struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id"`
	ProjectID   string            `json:"project_id"`
	PackID      string            `json:"pack_id"`
	PackVersion int               `json:"pack_version"`
	PackDigest  string            `json:"pack_digest"`
	Objects     []KnowledgeObject `json:"objects"`
	Digest      string            `json:"digest"`
	CreatedBy   string            `json:"created_by"`
	CreatedAt   time.Time         `json:"created_at"`
}

type KnowledgeDecision struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	ProjectID       string    `json:"project_id"`
	ObjectID        string    `json:"object_id"`
	PreviousVersion int       `json:"previous_version"`
	ResultVersion   int       `json:"result_version"`
	SubjectDigest   string    `json:"subject_digest"`
	Decision        string    `json:"decision"`
	Reason          string    `json:"reason"`
	ActorID         string    `json:"actor_id"`
	CreatedAt       time.Time `json:"created_at"`
}

func (d KnowledgeDecision) Validate() error {
	if strings.TrimSpace(d.ID) == "" || strings.TrimSpace(d.TenantID) == "" || strings.TrimSpace(d.ProjectID) == "" || strings.TrimSpace(d.ObjectID) == "" {
		return fault.Invalid("KNOWLEDGE_DECISION_CONTEXT_REQUIRED", "知识决策缺少身份或作用域")
	}
	if d.PreviousVersion < 1 || d.ResultVersion != d.PreviousVersion+1 || !strings.HasPrefix(d.SubjectDigest, "sha256:") || !stablehash.Matches(d.SubjectDigest) {
		return fault.Invalid("KNOWLEDGE_DECISION_VERSION_INVALID", "知识决策版本或 subject_digest 无效")
	}
	if d.Decision != "approve" && d.Decision != "reject" {
		return fault.Invalid("KNOWLEDGE_DECISION_INVALID", "知识决策只允许 approve 或 reject")
	}
	if strings.TrimSpace(d.Reason) == "" || strings.TrimSpace(d.ActorID) == "" || d.CreatedAt.IsZero() {
		return fault.Invalid("KNOWLEDGE_DECISION_REASON_REQUIRED", "知识决策必须包含原因、主体和时间")
	}
	return nil
}

type KnowledgeQuery struct {
	SnapshotID  string    `json:"snapshot_id"`
	Channel     string    `json:"channel,omitempty"`
	Layers      []string  `json:"layers,omitempty"`
	ObjectTypes []string  `json:"object_types,omitempty"`
	ObjectIDs   []string  `json:"object_ids,omitempty"`
	At          time.Time `json:"at,omitempty"`
}

type KnowledgeQueryEntry struct {
	ObjectID     string   `json:"object_id"`
	ObjectType   string   `json:"object_type"`
	Layer        string   `json:"layer"`
	Status       string   `json:"status"`
	EvidenceRefs []string `json:"evidence_refs"`
	Reasons      []string `json:"reasons,omitempty"`
}

type KnowledgeGapResult struct {
	ObjectID   string `json:"object_id"`
	Layer      string `json:"layer"`
	NextAction string `json:"next_action"`
	Impact     string `json:"impact,omitempty"`
}

type KnowledgeQueryResult struct {
	SnapshotID  string                `json:"snapshot_id"`
	QueryDigest string                `json:"query_digest"`
	Eligible    []KnowledgeQueryEntry `json:"eligible"`
	Blocked     []KnowledgeQueryEntry `json:"blocked"`
	Gaps        []KnowledgeGapResult  `json:"gaps"`
}

func (o KnowledgeObject) Validate() error {
	if strings.TrimSpace(o.ID) == "" || strings.TrimSpace(o.TenantID) == "" || strings.TrimSpace(o.ProjectID) == "" {
		return fault.Invalid("KNOWLEDGE_OBJECT_CONTEXT_REQUIRED", "知识对象必须包含 id、tenant_id 和 project_id")
	}
	if strings.TrimSpace(o.ObjectType) == "" || !validKnowledgeObjectType(o.ObjectType) {
		return fault.Invalid("KNOWLEDGE_OBJECT_TYPE_INVALID", "知识对象类型不受支持")
	}
	if !containsKnowledgeValue(KnowledgeLayers, o.Layer) {
		return fault.Invalid("KNOWLEDGE_LAYER_INVALID", "知识对象必须属于七层之一")
	}
	if o.Version < 1 {
		return fault.Invalid("KNOWLEDGE_OBJECT_VERSION_INVALID", "知识对象版本必须大于 0")
	}
	if strings.TrimSpace(o.Status) == "" || !validKnowledgeObjectStatus(o.Status) {
		return fault.Invalid("KNOWLEDGE_OBJECT_STATUS_INVALID", "知识对象状态不受支持")
	}
	if o.ValidUntil != nil && o.ValidFrom != nil && !o.ValidUntil.After(*o.ValidFrom) {
		return fault.Invalid("KNOWLEDGE_OBJECT_TIME_INVALID", "valid_until 必须晚于 valid_from")
	}
	if o.Digest != "" {
		digest, err := o.ContentDigest()
		if err != nil {
			return err
		}
		if digest != o.Digest {
			return fault.Conflict("KNOWLEDGE_OBJECT_DIGEST_MISMATCH", "知识对象 digest 与内容不一致")
		}
	}
	return nil
}

func (o KnowledgeObject) ContentDigest() (string, error) {
	payloadValue := o.Payload
	if payloadValue == nil {
		payloadValue = map[string]any{}
	}
	payload := struct {
		ID              string         `json:"id"`
		ProjectID       string         `json:"project_id"`
		ObjectType      string         `json:"object_type"`
		Layer           string         `json:"layer"`
		Version         int            `json:"version"`
		Status          string         `json:"status"`
		Title           string         `json:"title"`
		Statement       string         `json:"statement"`
		Payload         map[string]any `json:"payload"`
		Dimensions      []string       `json:"dimensions"`
		AllowedChannels []string       `json:"allowed_channels"`
		EvidenceRefs    []string       `json:"evidence_refs"`
		RelationRefs    []string       `json:"relation_refs"`
		RightsRefs      []string       `json:"rights_refs"`
		ConflictRefs    []string       `json:"conflict_refs"`
		DecisionRef     string         `json:"decision_ref,omitempty"`
		NextAction      string         `json:"next_action,omitempty"`
		Impact          string         `json:"impact,omitempty"`
		ValidFrom       *time.Time     `json:"valid_from,omitempty"`
		ValidUntil      *time.Time     `json:"valid_until,omitempty"`
		ExpiresAt       *time.Time     `json:"expires_at,omitempty"`
	}{o.ID, o.ProjectID, o.ObjectType, o.Layer, o.Version, o.Status, o.Title, o.Statement, payloadValue, sortedStrings(o.Dimensions), sortedStrings(o.AllowedChannels), sortedStrings(o.EvidenceRefs), sortedStrings(o.RelationRefs), sortedStrings(o.RightsRefs), sortedStrings(o.ConflictRefs), o.DecisionRef, o.NextAction, o.Impact, o.ValidFrom, o.ValidUntil, o.ExpiresAt}
	hash, err := stablehash.Sum(payload)
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func (p KnowledgePack) ContentDigest() (string, error) {
	refs := append([]KnowledgePackObjectRef(nil), p.ObjectRefs...)
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].ObjectID == refs[j].ObjectID {
			return refs[i].Version < refs[j].Version
		}
		return refs[i].ObjectID < refs[j].ObjectID
	})
	policy := normalizeKnowledgeQueryPolicy(p.QueryPolicy)
	hash, err := stablehash.Sum(struct {
		ID          string                   `json:"id"`
		TenantID    string                   `json:"tenant_id"`
		ProjectID   string                   `json:"project_id"`
		Name        string                   `json:"name"`
		Purpose     string                   `json:"purpose"`
		Version     int                      `json:"version"`
		ObjectRefs  []KnowledgePackObjectRef `json:"object_refs"`
		QueryPolicy KnowledgeQueryPolicy     `json:"query_policy"`
	}{p.ID, p.TenantID, p.ProjectID, p.Name, p.Purpose, p.Version, refs, policy})
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func (p KnowledgePack) Validate() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.TenantID) == "" || strings.TrimSpace(p.ProjectID) == "" {
		return fault.Invalid("KNOWLEDGE_PACK_CONTEXT_REQUIRED", "知识包必须包含 id、tenant_id 和 project_id")
	}
	if p.Version < 1 || len(p.ObjectRefs) == 0 {
		return fault.Invalid("KNOWLEDGE_PACK_OBJECTS_REQUIRED", "知识包必须包含版本和至少一个对象引用")
	}
	seen := map[string]bool{}
	for _, ref := range p.ObjectRefs {
		if strings.TrimSpace(ref.ObjectID) == "" || ref.Version < 0 || seen[ref.ObjectID] {
			return fault.Invalid("KNOWLEDGE_PACK_OBJECT_REF_INVALID", "知识包对象引用必须唯一且版本不能为负数")
		}
		seen[ref.ObjectID] = true
	}
	if p.Status != "draft" && p.Status != "published" && p.Status != "retired" {
		return fault.Invalid("KNOWLEDGE_PACK_STATUS_INVALID", "知识包状态只允许 draft、published 或 retired")
	}
	if err := validateKnowledgeQueryPolicy(p.QueryPolicy); err != nil {
		return err
	}
	if p.Digest != "" {
		digest, err := p.ContentDigest()
		if err != nil {
			return err
		}
		if digest != p.Digest {
			return fault.Conflict("KNOWLEDGE_PACK_DIGEST_MISMATCH", "知识包 digest 与内容不一致")
		}
	}
	return nil
}

func BuildKnowledgeSnapshot(pack KnowledgePack, objects []KnowledgeObject, now time.Time) (KnowledgeSnapshot, error) {
	if pack.Status != "published" {
		return KnowledgeSnapshot{}, fault.Policy("KNOWLEDGE_PACK_NOT_PUBLISHED", "只有已发布知识包才能生成快照", "先发布知识包")
	}
	if err := pack.Validate(); err != nil {
		return KnowledgeSnapshot{}, err
	}
	byRef := make(map[string]KnowledgeObject, len(objects))
	for _, object := range objects {
		if err := object.Validate(); err != nil {
			return KnowledgeSnapshot{}, err
		}
		if object.Digest == "" {
			digest, err := object.ContentDigest()
			if err != nil {
				return KnowledgeSnapshot{}, err
			}
			object.Digest = digest
		}
		if object.TenantID != pack.TenantID || object.ProjectID != pack.ProjectID {
			return KnowledgeSnapshot{}, fault.Conflict("KNOWLEDGE_OBJECT_SCOPE_MISMATCH", "知识对象不属于知识包作用域")
		}
		key := fmt.Sprintf("%s@%d", object.ID, object.Version)
		byRef[key] = object
	}
	selected := make([]KnowledgeObject, 0, len(pack.ObjectRefs))
	for _, ref := range pack.ObjectRefs {
		var selectedObject KnowledgeObject
		if ref.Version > 0 {
			selectedObject = byRef[fmt.Sprintf("%s@%d", ref.ObjectID, ref.Version)]
		} else {
			for _, object := range objects {
				if object.ID == ref.ObjectID && (selectedObject.ID == "" || object.Version > selectedObject.Version) {
					selectedObject = object
				}
			}
		}
		if selectedObject.ID == "" {
			return KnowledgeSnapshot{}, fault.NotFound("知识包引用的知识对象")
		}
		selected = append(selected, selectedObject)
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].ID == selected[j].ID {
			return selected[i].Version < selected[j].Version
		}
		return selected[i].ID < selected[j].ID
	})
	if now.IsZero() {
		now = time.Now().UTC()
	}
	packDigest := pack.Digest
	if packDigest == "" {
		var err error
		packDigest, err = pack.ContentDigest()
		if err != nil {
			return KnowledgeSnapshot{}, err
		}
	}
	snapshot := KnowledgeSnapshot{ID: idgen.New(), TenantID: pack.TenantID, ProjectID: pack.ProjectID, PackID: pack.ID, PackVersion: pack.Version, PackDigest: packDigest, Objects: selected, CreatedBy: pack.PublishedBy, CreatedAt: now.UTC()}
	digest, err := snapshot.ContentDigest()
	if err != nil {
		return KnowledgeSnapshot{}, err
	}
	snapshot.Digest = digest
	return snapshot, nil
}

func EvaluateKnowledgeSnapshot(snapshot KnowledgeSnapshot, policy KnowledgeQueryPolicy, query KnowledgeQuery) (KnowledgeQueryResult, error) {
	if err := snapshot.Validate(); err != nil {
		return KnowledgeQueryResult{}, err
	}
	if err := validateKnowledgeQueryPolicy(policy); err != nil {
		return KnowledgeQueryResult{}, err
	}
	if query.At.IsZero() {
		query.At = time.Now().UTC()
	}
	query.At = query.At.UTC()
	policy = normalizeKnowledgeQueryPolicy(policy)
	byID := make(map[string]KnowledgeObject, len(snapshot.Objects))
	for _, object := range snapshot.Objects {
		byID[object.ID] = object
	}
	result := KnowledgeQueryResult{SnapshotID: snapshot.ID, Eligible: []KnowledgeQueryEntry{}, Blocked: []KnowledgeQueryEntry{}, Gaps: []KnowledgeGapResult{}}
	for _, object := range snapshot.Objects {
		if len(query.Layers) > 0 && !containsKnowledgeValue(query.Layers, object.Layer) {
			continue
		}
		if len(query.ObjectTypes) > 0 && !containsKnowledgeValue(query.ObjectTypes, object.ObjectType) {
			continue
		}
		if len(query.ObjectIDs) > 0 && !containsKnowledgeValue(query.ObjectIDs, object.ID) {
			continue
		}
		if object.ObjectType == "KnowledgeGap" {
			if object.Status == "resolved" || object.Status == "waived" || object.Status == "superseded" || object.Status == "rejected" {
				continue
			}
			next := object.NextAction
			if next == "" {
				next = "REQUEST_SOURCE"
			}
			result.Gaps = append(result.Gaps, KnowledgeGapResult{ObjectID: object.ID, Layer: object.Layer, NextAction: next, Impact: object.Impact})
			continue
		}
		entry := KnowledgeQueryEntry{ObjectID: object.ID, ObjectType: object.ObjectType, Layer: object.Layer, Status: object.Status, EvidenceRefs: append([]string(nil), object.EvidenceRefs...)}
		reasons := knowledgeObjectBlockReasons(object, byID, policy, query.Channel, query.At)
		if len(reasons) > 0 {
			entry.Reasons = reasons
			result.Blocked = append(result.Blocked, entry)
			continue
		}
		result.Eligible = append(result.Eligible, entry)
	}
	queryDigest, err := stablehash.Sum(struct {
		SnapshotID  string               `json:"snapshot_id"`
		Channel     string               `json:"channel"`
		Layers      []string             `json:"layers"`
		ObjectTypes []string             `json:"object_types"`
		ObjectIDs   []string             `json:"object_ids"`
		At          time.Time            `json:"at"`
		Policy      KnowledgeQueryPolicy `json:"policy"`
	}{snapshot.ID, strings.TrimSpace(query.Channel), sortedStrings(query.Layers), sortedStrings(query.ObjectTypes), sortedStrings(query.ObjectIDs), query.At, policy})
	if err != nil {
		return KnowledgeQueryResult{}, err
	}
	result.QueryDigest = "sha256:" + queryDigest
	return result, nil
}

func (s KnowledgeSnapshot) ContentDigest() (string, error) {
	hash, err := stablehash.Sum(struct {
		ID          string            `json:"id"`
		TenantID    string            `json:"tenant_id"`
		ProjectID   string            `json:"project_id"`
		PackID      string            `json:"pack_id"`
		PackVersion int               `json:"pack_version"`
		PackDigest  string            `json:"pack_digest"`
		Objects     []KnowledgeObject `json:"objects"`
		CreatedBy   string            `json:"created_by"`
		CreatedAt   time.Time         `json:"created_at"`
	}{s.ID, s.TenantID, s.ProjectID, s.PackID, s.PackVersion, s.PackDigest, s.Objects, s.CreatedBy, s.CreatedAt})
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func (s KnowledgeSnapshot) Validate() error {
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.TenantID) == "" || strings.TrimSpace(s.ProjectID) == "" || strings.TrimSpace(s.PackID) == "" || s.PackVersion < 1 || len(s.Objects) == 0 {
		return fault.Invalid("KNOWLEDGE_SNAPSHOT_INVALID", "知识快照必须包含身份、作用域、知识包版本和对象")
	}
	if !strings.HasPrefix(s.PackDigest, "sha256:") || !stablehash.Matches(s.PackDigest) || !strings.HasPrefix(s.Digest, "sha256:") || !stablehash.Matches(s.Digest) {
		return fault.Invalid("KNOWLEDGE_SNAPSHOT_DIGEST_INVALID", "知识快照或知识包 digest 无效")
	}
	seen := map[string]bool{}
	for _, object := range s.Objects {
		key := fmt.Sprintf("%s@%d", object.ID, object.Version)
		if seen[key] || object.TenantID != s.TenantID || object.ProjectID != s.ProjectID || object.Digest == "" {
			return fault.Conflict("KNOWLEDGE_SNAPSHOT_OBJECT_INVALID", "知识快照包含重复、跨作用域或无 digest 的对象")
		}
		if err := object.Validate(); err != nil {
			return err
		}
		seen[key] = true
	}
	digest, err := s.ContentDigest()
	if err != nil {
		return err
	}
	if digest != s.Digest {
		return fault.Conflict("KNOWLEDGE_SNAPSHOT_DIGEST_MISMATCH", "知识快照 digest 与内容不一致")
	}
	return nil
}

func knowledgeObjectBlockReasons(object KnowledgeObject, byID map[string]KnowledgeObject, policy KnowledgeQueryPolicy, channel string, at time.Time) []string {
	reasons := []string{}
	if !containsKnowledgeValue(policy.EligibleStatuses, object.Status) {
		reasons = append(reasons, "STATUS_"+strings.ToUpper(object.Status))
	}
	if len(policy.AllowedObjectTypes) > 0 && !containsKnowledgeValue(policy.AllowedObjectTypes, object.ObjectType) {
		reasons = append(reasons, "OBJECT_TYPE_NOT_ALLOWED")
	}
	if policy.RequireEvidence && len(object.EvidenceRefs) == 0 {
		reasons = append(reasons, "EVIDENCE_REQUIRED")
	}
	if channel != "" && len(object.AllowedChannels) > 0 && !containsKnowledgeValue(object.AllowedChannels, channel) && !containsKnowledgeValue(object.AllowedChannels, "*") {
		reasons = append(reasons, "CHANNEL_NOT_ALLOWED")
	}
	if object.ValidFrom != nil && at.Before(object.ValidFrom.UTC()) {
		reasons = append(reasons, "NOT_YET_VALID")
	}
	if object.ValidUntil != nil && !at.Before(object.ValidUntil.UTC()) {
		reasons = append(reasons, "VALIDITY_ENDED")
	}
	if object.ExpiresAt != nil && !at.Before(object.ExpiresAt.UTC()) {
		reasons = append(reasons, "EXPIRED")
	}
	if policy.BlockOnConflict {
		for _, ref := range object.ConflictRefs {
			conflict, ok := byID[ref]
			if !ok {
				reasons = append(reasons, "CONFLICT_REFERENCE_MISSING:"+ref)
			} else if conflict.Status != "resolved" && conflict.Status != "accepted_risk" {
				reasons = append(reasons, "CONFLICT_OPEN:"+ref)
			}
		}
	}
	if policy.BlockOnRights {
		for _, ref := range object.RightsRefs {
			rights, ok := byID[ref]
			if !ok {
				reasons = append(reasons, "RIGHTS_REFERENCE_MISSING:"+ref)
			} else if rights.Status != "valid" && rights.Status != "approved" && rights.Status != "active" {
				reasons = append(reasons, "RIGHTS_NOT_USABLE:"+ref)
			}
		}
	}
	return uniqueKnowledgeStrings(reasons)
}

func normalizeKnowledgeQueryPolicy(policy KnowledgeQueryPolicy) KnowledgeQueryPolicy {
	if len(policy.EligibleStatuses) == 0 {
		policy.EligibleStatuses = append([]string(nil), KnowledgeEligibleStatuses...)
	}
	policy.EligibleStatuses = sortedStrings(policy.EligibleStatuses)
	policy.AllowedObjectTypes = sortedStrings(policy.AllowedObjectTypes)
	return policy
}

func validateKnowledgeQueryPolicy(policy KnowledgeQueryPolicy) error {
	for _, status := range normalizeKnowledgeQueryPolicy(policy).EligibleStatuses {
		if !containsKnowledgeValue(KnowledgeEligibleStatuses, status) {
			return fault.Invalid("KNOWLEDGE_QUERY_STATUS_INVALID", "知识查询策略只能使用已治理的可用状态")
		}
	}
	return nil
}

func validKnowledgeObjectType(value string) bool {
	switch value {
	case "FactAssertion", "Claim", "Audience", "Scenario", "Insight", "BrandRule", "ConstraintRecord", "Process", "Campaign", "Learning", "Asset", "RightsRecord", "ConflictRecord", "KnowledgeGap", "DomainObject":
		return true
	default:
		return false
	}
}

func validKnowledgeObjectStatus(value string) bool {
	switch value {
	case "candidate", "needs_review", "pending", "verified", "approved", "valid", "active", "blocked", "conflicted", "prohibited", "expired", "rejected", "superseded", "revoked", "open", "source_missing", "collecting", "resolved", "waived", "accepted_risk":
		return true
	default:
		return false
	}
}

func containsKnowledgeValue(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func sortedStrings(values []string) []string {
	result := uniqueKnowledgeStrings(values)
	sort.Strings(result)
	return result
}

func uniqueKnowledgeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

const KnowledgeCandidatesSchema = "knowledge-candidates/1.0"

type EvidenceRef struct {
	SourceRevisionID string `json:"source_revision_id"`
	LocatorKind      string `json:"locator_kind"`
	Locator          string `json:"locator"`
	Quote            string `json:"quote"`
}

type KnowledgeScope struct {
	Regions         []string `json:"regions"`
	Channels        []string `json:"channels"`
	Audiences       []string `json:"audiences"`
	ProductVariants []string `json:"product_variants"`
}

type ContextSnapshot struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	ProjectID      string            `json:"project_id"`
	BuilderVersion string            `json:"builder_version"`
	SchemaVersion  string            `json:"schema_version"`
	Sources        []ContractSource  `json:"sources,omitempty"`
	InputVersions  map[string]string `json:"input_versions"`
	ManifestHash   string            `json:"manifest_hash"`
	CreatedAt      time.Time         `json:"created_at"`
}

func CompileKnowledgeSnapshot(project workspace.Project, sources []ContractSource, now time.Time) (ContextSnapshot, error) {
	if len(sources) == 0 {
		return ContextSnapshot{}, fmt.Errorf("知识提取至少需要一个来源修订版本")
	}
	ordered := append([]ContractSource(nil), sources...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].RevisionID < ordered[j].RevisionID })
	versions := make(map[string]string, len(ordered))
	for index := range ordered {
		sort.Slice(ordered[index].Evidence, func(i, j int) bool { return ordered[index].Evidence[i].ID < ordered[index].Evidence[j].ID })
		versions["source_revision:"+ordered[index].RevisionID] = ordered[index].SHA256
	}
	payload := struct {
		ProjectID string            `json:"project_id"`
		Versions  map[string]string `json:"versions"`
		Sources   []ContractSource  `json:"sources"`
		Builder   string            `json:"builder"`
		Schema    string            `json:"schema"`
	}{project.ID, versions, ordered, "knowledge-contract/1.0.0", TaskContractSchema}
	hash, err := stablehash.Sum(payload)
	if err != nil {
		return ContextSnapshot{}, err
	}
	return ContextSnapshot{ID: idgen.New(), TenantID: project.TenantID, ProjectID: project.ID, BuilderVersion: "knowledge-contract/1.0.0", SchemaVersion: TaskContractSchema, Sources: ordered, InputVersions: versions, ManifestHash: hash, CreatedAt: now.UTC()}, nil
}
