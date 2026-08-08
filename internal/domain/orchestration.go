package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	SOPSchemaVersion       = "contentcloud.sop/1.0"
	GateModeNone           = "none"
	GateModeAdvisory       = "advisory"
	GateModeRequiredCheck  = "required_check"
	GateModeInternalReview = "internal_review"
	GateModeClientDecision = "client_decision"

	TaskStatusNeedsInput  = "needs_input"
	TaskStatusReady       = "ready"
	TaskStatusRunning     = "running"
	TaskStatusPaused      = "paused"
	TaskStatusWaitingGate = "waiting_gate"
	TaskStatusBlocked     = "blocked"
	TaskStatusAccepted    = "accepted"
	TaskStatusDelivered   = "delivered"
	TaskStatusCancelled   = "cancelled"

	StageRunStatusPending     = "pending"
	StageRunStatusRunning     = "running"
	StageRunStatusWaitingGate = "waiting_gate"
	StageRunStatusBlocked     = "blocked"
	StageRunStatusCompleted   = "completed"
	StageRunStatusCancelled   = "cancelled"

	GateEvaluationPending          = "pending"
	GateEvaluationApproved         = "approved"
	GateEvaluationRejected         = "rejected"
	GateEvaluationChangesRequested = "changes_requested"
	GateEvaluationExpired          = "expired"

	TaskRevisionDraft      = "draft"
	TaskRevisionSubmitted  = "submitted"
	TaskRevisionAccepted   = "accepted"
	TaskRevisionRejected   = "rejected"
	TaskRevisionSuperseded = "superseded"

	TaskDeliveryReady     = "ready"
	TaskDeliveryDelivered = "delivered"
	TaskDeliveryFailed    = "failed"
	TaskDeliveryCancelled = "cancelled"
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

// WorkTask is the user-facing work object. Formal content facts remain in
// Revision, Evidence, Gate and Delivery records.
type WorkTask struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	ProjectID       string         `json:"project_id"`
	EnvironmentID   string         `json:"environment_id"`
	SOPID           string         `json:"sop_id"`
	SOPVersion      int            `json:"sop_version"`
	SOPDigest       string         `json:"sop_digest"`
	Title           string         `json:"title"`
	Intent          string         `json:"intent"`
	ContentType     string         `json:"content_type"`
	InputRefs       []string       `json:"input_refs"`
	RequestedOutput map[string]any `json:"requested_output"`
	AssigneeUserID  string         `json:"assignee_user_id,omitempty"`
	Priority        string         `json:"priority"`
	DueAt           *time.Time     `json:"due_at,omitempty"`
	RiskProfile     string         `json:"risk_profile"`
	IdempotencyKey  string         `json:"-"`
	Status          string         `json:"status"`
	CurrentStageID  string         `json:"current_stage_id"`
	NextAction      string         `json:"next_action"`
	CreatedBy       string         `json:"created_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type StageRun struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id"`
	TaskID        string            `json:"task_id"`
	StageID       string            `json:"stage_id"`
	Status        string            `json:"status"`
	ExecutionMode string            `json:"execution_mode"`
	InputRefs     []string          `json:"input_refs"`
	OutputRefs    []string          `json:"output_refs"`
	Outputs       []TaskStageOutput `json:"outputs"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
	CompletedAt   *time.Time        `json:"completed_at,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type GateEvaluation struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenant_id"`
	ProjectID  string         `json:"project_id"`
	TaskID     string         `json:"task_id"`
	StageRunID string         `json:"stage_run_id"`
	GateID     string         `json:"gate_id"`
	GateMode   string         `json:"gate_mode"`
	Status     string         `json:"status"`
	RevisionID string         `json:"revision_id,omitempty"`
	InputRefs  []string       `json:"input_refs"`
	Checks     map[string]any `json:"checks"`
	Decision   string         `json:"decision,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	DecidedBy  string         `json:"decided_by,omitempty"`
	DecidedAt  *time.Time     `json:"decided_at,omitempty"`
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func (v *GateEvaluation) NormalizeCollections() {
	if v.InputRefs == nil {
		v.InputRefs = []string{}
	}
	if v.Checks == nil {
		v.Checks = map[string]any{}
	}
}

func (v GateEvaluation) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.StageRunID == "" || v.GateID == "" {
		return Invalid("GATE_EVALUATION_INVALID", "Gate 决定缺少任务、StageRun 或 Gate 标识")
	}
	switch v.Status {
	case GateEvaluationPending, GateEvaluationApproved, GateEvaluationRejected, GateEvaluationChangesRequested, GateEvaluationExpired:
	default:
		return Invalid("GATE_EVALUATION_STATUS_INVALID", "Gate 决定状态无效")
	}
	return nil
}

type TaskRevision struct {
	ID                   string          `json:"id"`
	TenantID             string          `json:"tenant_id"`
	ProjectID            string          `json:"project_id"`
	TaskID               string          `json:"task_id"`
	RevisionNo           int             `json:"revision_no"`
	ContentType          string          `json:"content_type"`
	SchemaVersion        string          `json:"schema_version"`
	Content              json.RawMessage `json:"content"`
	ContentHash          string          `json:"content_hash"`
	SOPDigest            string          `json:"sop_digest"`
	KnowledgeSnapshotIDs []string        `json:"knowledge_snapshot_ids"`
	EvidenceSummary      map[string]any  `json:"evidence_summary"`
	RightsSummary        map[string]any  `json:"rights_summary"`
	Status               string          `json:"status"`
	SubmittedBy          string          `json:"submitted_by"`
	SubmittedAt          *time.Time      `json:"submitted_at,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
}

func (v *TaskRevision) NormalizeCollections() {
	if v.KnowledgeSnapshotIDs == nil {
		v.KnowledgeSnapshotIDs = []string{}
	}
	if v.EvidenceSummary == nil {
		v.EvidenceSummary = map[string]any{}
	}
	if v.RightsSummary == nil {
		v.RightsSummary = map[string]any{}
	}
}

func (v TaskRevision) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.RevisionNo < 1 || strings.TrimSpace(v.ContentType) == "" || strings.TrimSpace(v.SchemaVersion) == "" || len(v.Content) == 0 {
		return Invalid("TASK_REVISION_INVALID", "Revision 缺少任务、内容类型、Schema 或内容")
	}
	switch v.Status {
	case TaskRevisionDraft, TaskRevisionSubmitted, TaskRevisionAccepted, TaskRevisionRejected, TaskRevisionSuperseded:
	default:
		return Invalid("TASK_REVISION_STATUS_INVALID", "Revision 状态无效")
	}
	if v.ContentHash != "" && !validSHA256Digest(v.ContentHash) {
		return Invalid("TASK_REVISION_HASH_INVALID", "Revision content hash 必须是 sha256 摘要")
	}
	return nil
}

type TaskDelivery struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id"`
	ProjectID         string     `json:"project_id"`
	TaskID            string     `json:"task_id"`
	RevisionID        string     `json:"revision_id"`
	Destination       string     `json:"destination"`
	Status            string     `json:"status"`
	Manifest          []string   `json:"manifest"`
	DeliveryPackageID string     `json:"delivery_package_id,omitempty"`
	IntegrityStatus   string     `json:"integrity_status"`
	DeliveryDigest    string     `json:"delivery_digest"`
	DeliveredBy       string     `json:"delivered_by,omitempty"`
	DeliveredAt       *time.Time `json:"delivered_at,omitempty"`
	ErrorCode         string     `json:"error_code,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (v *TaskDelivery) NormalizeCollections() {
	if v.Manifest == nil {
		v.Manifest = []string{}
	}
}

func (v TaskDelivery) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.RevisionID == "" || strings.TrimSpace(v.Destination) == "" {
		return Invalid("TASK_DELIVERY_INVALID", "交付缺少任务、Revision 或目的地")
	}
	switch v.Status {
	case TaskDeliveryReady, TaskDeliveryDelivered, TaskDeliveryFailed, TaskDeliveryCancelled:
	default:
		return Invalid("TASK_DELIVERY_STATUS_INVALID", "交付状态无效")
	}
	if v.DeliveryDigest != "" && !validSHA256Digest(v.DeliveryDigest) {
		return Invalid("TASK_DELIVERY_HASH_INVALID", "交付摘要必须是 sha256 摘要")
	}
	if v.Status == TaskDeliveryDelivered && (v.DeliveryPackageID == "" || len(v.Manifest) == 0 || v.IntegrityStatus != "complete") {
		return Invalid("TASK_DELIVERY_INCOMPLETE", "已交付记录必须引用完整 DeliveryPackage 和非空 manifest")
	}
	return nil
}

type AdminWorkOSView struct {
	Environments []Environment `json:"environments"`
	SOPs         []SOPSummary  `json:"sops"`
	Gates        []GateSummary `json:"gates"`
	Capabilities []Capability  `json:"capabilities"`
	Audit        []AuditEvent  `json:"audit"`
	Usage        UsageSummary  `json:"usage"`
	GeneratedAt  time.Time     `json:"generated_at"`
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
	return CanonicalHash(struct {
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
		return Invalid("SOP_VERSION_INVALID", "SOP 版本缺少 ID、版本号或名称")
	}
	if v.SchemaVersion != "" && v.SchemaVersion != SOPSchemaVersion {
		return Invalid("SOP_SCHEMA_UNSUPPORTED", "SOP Schema 版本不受支持")
	}
	if v.Digest != "" && !validSHA256Digest(v.Digest) {
		return Invalid("SOP_DIGEST_INVALID", "SOP digest 必须是 sha256 十六进制摘要")
	}
	seenStages := map[string]bool{}
	lastOrder := -1
	for _, stage := range v.Stages {
		if stage.ID == "" || stage.Name == "" || stage.Order < 0 || seenStages[stage.ID] || stage.OutputSchema == "" {
			return Invalid("SOP_STAGE_INVALID", "Stage 必须有唯一 ID、名称、顺序和输出 Schema")
		}
		if stage.Order <= lastOrder {
			return Invalid("SOP_STAGE_ORDER_INVALID", "Stage 顺序必须稳定且递增")
		}
		lastOrder = stage.Order
		seenStages[stage.ID] = true
		if stage.CompletionPolicy != "" && stage.CompletionPolicy != StageCompletionAllRequired && stage.CompletionPolicy != StageCompletionAtLeastOne && stage.CompletionPolicy != StageCompletionControlOnly {
			return Invalid("SOP_STAGE_COMPLETION_POLICY_INVALID", "Stage 完成策略无效")
		}
		for _, requirement := range append(append([]StageObjectRequirement{}, stage.AcceptedInputTypes...), stage.RequiredOutputTypes...) {
			if _, ok := validStageOutputTypes[requirement.OutputType]; !ok || requirement.MinCount < 0 {
				return Invalid("SOP_STAGE_OBJECT_REQUIREMENT_INVALID", "Stage 对象契约包含无效类型或数量")
			}
		}
	}
	seenGates := map[string]bool{}
	for _, gate := range v.Gates {
		if gate.ID == "" || gate.Name == "" || seenGates[gate.ID] {
			return Invalid("SOP_GATE_INVALID", "Gate 必须有唯一 ID 和名称")
		}
		switch gate.Mode {
		case GateModeNone, GateModeAdvisory, GateModeRequiredCheck, GateModeInternalReview, GateModeClientDecision:
		default:
			return Invalid("SOP_GATE_MODE_INVALID", fmt.Sprintf("Gate %s 的模式无效", gate.ID))
		}
		seenGates[gate.ID] = true
	}
	for _, stage := range v.Stages {
		for _, gateID := range stage.GateIDs {
			if !seenGates[gateID] {
				return Invalid("SOP_GATE_REFERENCE_INVALID", "Stage 引用了不存在的 Gate")
			}
		}
	}
	return nil
}

func (v Environment) Validate() error {
	if v.ID == "" || v.TenantID == "" || strings.TrimSpace(v.Name) == "" || strings.TrimSpace(v.Slug) == "" {
		return Invalid("ENVIRONMENT_INVALID", "Environment 缺少必要字段")
	}
	if v.Status == "" {
		return Invalid("ENVIRONMENT_STATUS_REQUIRED", "Environment 状态不能为空")
	}
	if v.ManifestDigest != "" && !validSHA256Digest(v.ManifestDigest) {
		return Invalid("ENVIRONMENT_DIGEST_INVALID", "Environment manifest digest 必须是 sha256 十六进制摘要")
	}
	return nil
}

func (v WorkTask) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.SOPID == "" || v.SOPVersion < 1 || strings.TrimSpace(v.Title) == "" {
		return Invalid("TASK_INVALID", "任务缺少项目、SOP 或标题")
	}
	if v.Priority == "" {
		return Invalid("TASK_PRIORITY_REQUIRED", "任务优先级不能为空")
	}
	if v.SOPDigest != "" && !validSHA256Digest(v.SOPDigest) {
		return Invalid("TASK_SOP_DIGEST_INVALID", "任务必须固定合法的 SOP digest")
	}
	if len(v.IdempotencyKey) > 128 {
		return Invalid("IDEMPOTENCY_KEY_INVALID", "idempotency_key 不能超过 128 字符")
	}
	if v.Status != "" {
		switch v.Status {
		case TaskStatusNeedsInput, TaskStatusReady, TaskStatusRunning, TaskStatusPaused, TaskStatusWaitingGate, TaskStatusBlocked, TaskStatusAccepted, TaskStatusDelivered, TaskStatusCancelled:
		default:
			return Invalid("TASK_STATUS_INVALID", "任务状态无效")
		}
	}
	return nil
}

func validSHA256Digest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[len("sha256:"):] {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
