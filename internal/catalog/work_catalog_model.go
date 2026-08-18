package catalog

import "github.com/limecloud/contentcloud/internal/audit"

import "github.com/limecloud/contentcloud/internal/platform/stablehash"
import "github.com/limecloud/contentcloud/internal/platform/fault"
import "time"
import "strings"
import "sort"
import "fmt"

const (
	StageOutputSourceRevision     = "source_revision"
	StageOutputEvidenceSet        = "evidence_set"
	StageOutputKnowledgeObject    = "knowledge_object"
	StageOutputKnowledgeSnapshot  = "knowledge_snapshot"
	StageOutputSubmissionRevision = "submission_revision"
	StageOutputApprovedSnapshot   = "approved_snapshot"
	StageOutputStoryboardPackage  = "storyboard_package"
	StageOutputArtifact           = "artifact"
	StageOutputGenerationJob      = "generation_job"
	StageOutputMediaReview        = "media_review"
	StageOutputDeliveryPackage    = "delivery_package"
	StageOutputRolePrimary        = "primary"
	StageOutputRoleSupporting     = "supporting"
	StageOutputRolePreview        = "preview"
	StageOutputRoleSelectedTake   = "selected_take"
	StageOutputRoleFinal          = "final"
	StageOutputStatusCandidate    = "candidate"
	StageOutputStatusValidated    = "validated"
	StageOutputStatusApproved     = "approved"
	StageOutputStatusBlocked      = "blocked"
	StageOutputStatusFailed       = "failed"
	StageCompletionAllRequired    = "all_required"
	StageCompletionAtLeastOne     = "at_least_one"
	StageCompletionControlOnly    = "control_only"
)

var validStageOutputTypes = map[string]struct{}{
	StageOutputSourceRevision: {}, StageOutputEvidenceSet: {}, StageOutputKnowledgeObject: {},
	StageOutputKnowledgeSnapshot: {}, StageOutputSubmissionRevision: {}, StageOutputApprovedSnapshot: {},
	StageOutputStoryboardPackage: {}, StageOutputArtifact: {}, StageOutputGenerationJob: {},
	StageOutputMediaReview: {}, StageOutputDeliveryPackage: {},
}

func ValidStageOutputType(value string) bool {
	_, ok := validStageOutputTypes[value]
	return ok
}

type StageObjectRequirement struct {
	OutputType string `json:"output_type"`
	Role       string `json:"role,omitempty"`
	MinStatus  string `json:"min_status,omitempty"`
	MinCount   int    `json:"min_count,omitempty"`
}

type StageRetryPolicy struct {
	MaxAttempts        int      `json:"max_attempts,omitempty"`
	BackoffSeconds     int      `json:"backoff_seconds,omitempty"`
	AllowPartialRetry  bool     `json:"allow_partial_retry,omitempty"`
	RetryableErrorCode []string `json:"retryable_error_codes,omitempty"`
}

type StageCostPolicy struct {
	Currency                  string `json:"currency,omitempty"`
	MaxEstimatedCostMinor     int64  `json:"max_estimated_cost_minor,omitempty"`
	RequireApprovalAboveMinor int64  `json:"require_approval_above_minor,omitempty"`
	EstimateTTLSeconds        int    `json:"estimate_ttl_seconds,omitempty"`
}

type Capability struct {
	ID                   string   `json:"id"`
	Version              string   `json:"version"`
	Kind                 string   `json:"kind"`
	InputSchema          string   `json:"input_schema"`
	OutputSchema         string   `json:"output_schema"`
	PresentationProfiles []string `json:"presentation_profiles"`
	LocalOnly            bool     `json:"local_only"`
	Digest               string   `json:"digest"`
}

const (
	SOPSchemaVersion       = "contentcloud.sop/1.0"
	GateModeNone           = "none"
	GateModeAdvisory       = "advisory"
	GateModeRequiredCheck  = "required_check"
	GateModeInternalReview = "internal_review"
	GateModeClientDecision = "client_decision"
)

// Environment is the tenant-scoped execution boundary. It deliberately stores
// references and digests instead of local paths or client transcripts.
type Environment struct {
	ID                string                  `json:"id"`
	TenantID          string                  `json:"tenant_id"`
	Name              string                  `json:"name"`
	Slug              string                  `json:"slug"`
	Status            string                  `json:"status"`
	ManifestDigest    string                  `json:"manifest_digest"`
	DefaultSOPID      string                  `json:"default_sop_id,omitempty"`
	DefaultSOPVersion int                     `json:"default_sop_version,omitempty"`
	Capabilities      []EnvironmentCapability `json:"capabilities"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
}

type EnvironmentCapability struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
}

type SOPDefinition struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	ContentTypes   []string  `json:"content_types"`
	CurrentVersion int       `json:"current_version"`
	TemplateKey    string    `json:"template_key,omitempty"`
	BuiltIn        bool      `json:"built_in,omitempty"`
	SourceRef      string    `json:"source_ref,omitempty"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// NormalizeCollections keeps optional collection fields JSON-safe at the API boundary.
// Empty collections are represented as [] instead of null so clients can iterate safely.
func (v *SOPDefinition) NormalizeCollections() {
	v.ContentTypes = normalizeStrings(v.ContentTypes)
}

type SOPVersion struct {
	ID                   string            `json:"id"`
	TenantID             string            `json:"tenant_id"`
	SOPID                string            `json:"sop_id"`
	Version              int               `json:"version"`
	SchemaVersion        string            `json:"schema_version"`
	Name                 string            `json:"name"`
	Description          string            `json:"description"`
	ContentTypes         []string          `json:"content_types"`
	Stages               []StageDefinition `json:"stages"`
	Gates                []GateDefinition  `json:"gates"`
	DefaultExecutionMode string            `json:"default_execution_mode"`
	Digest               string            `json:"digest"`
	Status               string            `json:"status"`
	CreatedBy            string            `json:"created_by"`
	PublishedBy          string            `json:"published_by,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	PublishedAt          *time.Time        `json:"published_at,omitempty"`
}

type StageDefinition struct {
	ID                   string                   `json:"stage_id"`
	Name                 string                   `json:"name"`
	Order                int                      `json:"order"`
	OwnerRoles           []string                 `json:"owner_roles"`
	InputRefs            []string                 `json:"input_refs"`
	OutputSchema         string                   `json:"output_schema"`
	RequiredCapabilities []string                 `json:"required_capabilities"`
	AllowedExecutorKinds []string                 `json:"allowed_executor_kinds"`
	ExecutionModes       []string                 `json:"execution_modes"`
	Checks               []string                 `json:"checks"`
	GateIDs              []string                 `json:"gate_ids"`
	RetryMaxAttempts     int                      `json:"retry_max_attempts"`
	AcceptedInputTypes   []StageObjectRequirement `json:"accepted_input_types,omitempty"`
	RequiredOutputTypes  []StageObjectRequirement `json:"required_output_types,omitempty"`
	OutputSchemaRefs     []string                 `json:"output_schema_refs,omitempty"`
	CompletionPolicy     string                   `json:"completion_policy,omitempty"`
	ExecutorPolicy       string                   `json:"executor_policy,omitempty"`
	RetryPolicy          StageRetryPolicy         `json:"retry_policy,omitempty"`
	CostPolicy           StageCostPolicy          `json:"cost_policy,omitempty"`
}

type GateDefinition struct {
	ID              string   `json:"gate_id"`
	Name            string   `json:"name"`
	Mode            string   `json:"mode"`
	Blocking        bool     `json:"blocking"`
	AssigneeRoles   []string `json:"assignee_roles"`
	InputRefs       []string `json:"input_refs"`
	Checks          []string `json:"checks"`
	OnReject        string   `json:"on_reject"`
	EscalationHours int      `json:"escalation_hours"`
}

func (v *Environment) NormalizeCollections() {
	if v.Capabilities == nil {
		v.Capabilities = []EnvironmentCapability{}
	}
}

func (v *SOPVersion) NormalizeCollections() {
	v.ContentTypes = normalizeStrings(v.ContentTypes)
	if v.Stages == nil {
		v.Stages = []StageDefinition{}
	}
	if v.Gates == nil {
		v.Gates = []GateDefinition{}
	}
	for index := range v.Stages {
		stage := &v.Stages[index]
		stage.OwnerRoles = normalizeStrings(stage.OwnerRoles)
		stage.InputRefs = normalizeStrings(stage.InputRefs)
		stage.RequiredCapabilities = normalizeStrings(stage.RequiredCapabilities)
		stage.AllowedExecutorKinds = normalizeStrings(stage.AllowedExecutorKinds)
		stage.ExecutionModes = normalizeStrings(stage.ExecutionModes)
		stage.Checks = normalizeStrings(stage.Checks)
		stage.GateIDs = normalizeStrings(stage.GateIDs)
		if stage.AcceptedInputTypes == nil {
			stage.AcceptedInputTypes = []StageObjectRequirement{}
		} else {
			stage.AcceptedInputTypes = append([]StageObjectRequirement{}, stage.AcceptedInputTypes...)
		}
		if stage.RequiredOutputTypes == nil {
			stage.RequiredOutputTypes = []StageObjectRequirement{}
		} else {
			stage.RequiredOutputTypes = append([]StageObjectRequirement{}, stage.RequiredOutputTypes...)
		}
		stage.OutputSchemaRefs = normalizeStrings(stage.OutputSchemaRefs)
		stage.RetryPolicy.RetryableErrorCode = normalizeStrings(stage.RetryPolicy.RetryableErrorCode)
	}
	for index := range v.Gates {
		gate := &v.Gates[index]
		gate.AssigneeRoles = normalizeStrings(gate.AssigneeRoles)
		gate.InputRefs = normalizeStrings(gate.InputRefs)
		gate.Checks = normalizeStrings(gate.Checks)
	}
}

func normalizeStrings(value []string) []string {
	if value == nil {
		return []string{}
	}
	return append([]string{}, value...)
}

type ProjectSOPBinding struct {
	TenantID      string    `json:"tenant_id"`
	ProjectID     string    `json:"project_id"`
	EnvironmentID string    `json:"environment_id"`
	SOPID         string    `json:"sop_id"`
	SOPVersion    int       `json:"sop_version"`
	SOPDigest     string    `json:"sop_digest"`
	BoundBy       string    `json:"bound_by"`
	BoundAt       time.Time `json:"bound_at"`
}

type AdminWorkOSView struct {
	Environments []Environment      `json:"environments"`
	SOPs         []SOPSummary       `json:"sops"`
	Gates        []GateSummary      `json:"gates"`
	Capabilities []Capability       `json:"capabilities"`
	Audit        []audit.AuditEvent `json:"audit"`
	Usage        UsageSummary       `json:"usage"`
	GeneratedAt  time.Time          `json:"generated_at"`
}

type SOPSummary struct {
	Definition SOPDefinition `json:"definition"`
	Versions   []SOPVersion  `json:"versions"`
}

type GateSummary struct {
	SOPID      string `json:"sop_id"`
	SOPName    string `json:"sop_name"`
	SOPVersion int    `json:"sop_version"`
	ID         string `json:"gate_id"`
	Name       string `json:"name"`
	Mode       string `json:"mode"`
	Blocking   bool   `json:"blocking"`
	UsageCount int    `json:"usage_count"`
}

type UsageSummary struct {
	TaskCount        int            `json:"task_count"`
	RunningCount     int            `json:"running_count"`
	WaitingGateCount int            `json:"waiting_gate_count"`
	ByExecutionMode  map[string]int `json:"by_execution_mode"`
}

func (v SOPVersion) ContentDigest() (string, error) {
	v.NormalizeCollections()
	if v.SchemaVersion == "" {
		v.SchemaVersion = SOPSchemaVersion
	}
	stages := append([]StageDefinition(nil), v.Stages...)
	sort.SliceStable(stages, func(i, j int) bool { return stages[i].Order < stages[j].Order })
	gates := append([]GateDefinition(nil), v.Gates...)
	sort.SliceStable(gates, func(i, j int) bool { return gates[i].ID < gates[j].ID })
	return stablehash.Sum(struct {
		SchemaVersion string            `json:"schema_version"`
		SOPID         string            `json:"sop_id"`
		Version       int               `json:"version"`
		Name          string            `json:"name"`
		Description   string            `json:"description"`
		ContentTypes  []string          `json:"content_types"`
		Stages        []StageDefinition `json:"stages"`
		Gates         []GateDefinition  `json:"gates"`
		ExecutionMode string            `json:"default_execution_mode"`
	}{v.SchemaVersion, v.SOPID, v.Version, v.Name, v.Description, v.ContentTypes, stages, gates, v.DefaultExecutionMode})
}

func (v SOPVersion) Validate() error {
	v.NormalizeCollections()
	if v.SOPID == "" || v.Version < 1 || strings.TrimSpace(v.Name) == "" {
		return fault.Invalid("SOP_VERSION_INVALID", "SOP 版本缺少 ID、版本号或名称")
	}
	if v.SchemaVersion != "" && v.SchemaVersion != SOPSchemaVersion {
		return fault.Invalid("SOP_SCHEMA_UNSUPPORTED", "SOP Schema 版本不受支持")
	}
	if v.Digest != "" && !stablehash.Valid(v.Digest) {
		return fault.Invalid("SOP_DIGEST_INVALID", "SOP digest 必须是 sha256 十六进制摘要")
	}
	seenStages := map[string]bool{}
	lastOrder := -1
	for _, stage := range v.Stages {
		if stage.ID == "" || stage.Name == "" || stage.Order < 0 || seenStages[stage.ID] || stage.OutputSchema == "" {
			return fault.Invalid("SOP_STAGE_INVALID", "Stage 必须有唯一 ID、名称、顺序和输出 Schema")
		}
		if stage.Order <= lastOrder {
			return fault.Invalid("SOP_STAGE_ORDER_INVALID", "Stage 顺序必须稳定且递增")
		}
		lastOrder = stage.Order
		seenStages[stage.ID] = true
		if stage.CompletionPolicy != "" && stage.CompletionPolicy != StageCompletionAllRequired && stage.CompletionPolicy != StageCompletionAtLeastOne && stage.CompletionPolicy != StageCompletionControlOnly {
			return fault.Invalid("SOP_STAGE_COMPLETION_POLICY_INVALID", "Stage 完成策略无效")
		}
		for _, requirement := range append(append([]StageObjectRequirement{}, stage.AcceptedInputTypes...), stage.RequiredOutputTypes...) {
			if _, ok := validStageOutputTypes[requirement.OutputType]; !ok || requirement.MinCount < 0 {
				return fault.Invalid("SOP_STAGE_OBJECT_REQUIREMENT_INVALID", "Stage 对象契约包含无效类型或数量")
			}
		}
	}
	seenGates := map[string]bool{}
	for _, gate := range v.Gates {
		if gate.ID == "" || gate.Name == "" || seenGates[gate.ID] {
			return fault.Invalid("SOP_GATE_INVALID", "Gate 必须有唯一 ID 和名称")
		}
		switch gate.Mode {
		case GateModeNone, GateModeAdvisory, GateModeRequiredCheck, GateModeInternalReview, GateModeClientDecision:
		default:
			return fault.Invalid("SOP_GATE_MODE_INVALID", fmt.Sprintf("Gate %s 的模式无效", gate.ID))
		}
		seenGates[gate.ID] = true
	}
	for _, stage := range v.Stages {
		for _, gateID := range stage.GateIDs {
			if !seenGates[gateID] {
				return fault.Invalid("SOP_GATE_REFERENCE_INVALID", "Stage 引用了不存在的 Gate")
			}
		}
	}
	return nil
}

func (v Environment) Validate() error {
	if v.ID == "" || v.TenantID == "" || strings.TrimSpace(v.Name) == "" || strings.TrimSpace(v.Slug) == "" {
		return fault.Invalid("ENVIRONMENT_INVALID", "Environment 缺少必要字段")
	}
	if v.Status == "" {
		return fault.Invalid("ENVIRONMENT_STATUS_REQUIRED", "Environment 状态不能为空")
	}
	if v.ManifestDigest != "" && !stablehash.Valid(v.ManifestDigest) {
		return fault.Invalid("ENVIRONMENT_DIGEST_INVALID", "Environment manifest digest 必须是 sha256 十六进制摘要")
	}
	return nil
}
