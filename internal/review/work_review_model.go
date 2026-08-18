package review

import "github.com/limecloud/contentcloud/internal/platform/stablehash"
import "github.com/limecloud/contentcloud/internal/platform/fault"
import "time"
import "sort"
import "path"
import "encoding/json"
import "strings"

type ReviewComment struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	ProjectID     string     `json:"project_id"`
	ReviewCycleID string     `json:"review_cycle_id"`
	SubjectType   string     `json:"subject_type"`
	SubjectID     string     `json:"subject_id"`
	CarriedFromID string     `json:"carried_from_comment_id,omitempty"`
	ShotID        string     `json:"shot_id,omitempty"`
	JSONPointer   string     `json:"json_pointer,omitempty"`
	Body          string     `json:"body"`
	Visibility    string     `json:"visibility"`
	AuthorID      string     `json:"author_id"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type ReviewCycle struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	ProjectID      string     `json:"project_id"`
	SubjectType    string     `json:"subject_type"`
	SubjectID      string     `json:"subject_id"`
	CycleNumber    int        `json:"cycle_number"`
	Status         string     `json:"status"`
	Conclusion     string     `json:"conclusion,omitempty"`
	AssigneeUserID string     `json:"assignee_user_id,omitempty"`
	OpenedBy       string     `json:"opened_by"`
	DecidedBy      string     `json:"decided_by,omitempty"`
	OpenedAt       time.Time  `json:"opened_at"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type ReviewGrant struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	ProjectID      string     `json:"project_id"`
	SubjectType    string     `json:"subject_type"`
	SubjectID      string     `json:"subject_id"`
	SubjectHash    string     `json:"subject_hash"`
	ReviewerEmail  string     `json:"reviewer_email"`
	TokenHash      string     `json:"-"`
	OTPHash        string     `json:"-"`
	ExpiresAt      time.Time  `json:"expires_at"`
	VerifiedAt     *time.Time `json:"verified_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	DecisionAt     *time.Time `json:"decision_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	PlaintextToken string     `json:"review_token,omitempty"`
	PlaintextOTP   string     `json:"dev_otp,omitempty"`
}

func ValidJSONPointer(pointer string) bool {
	if pointer == "" || !strings.HasPrefix(pointer, "/") {
		return false
	}
	for index := 0; index < len(pointer); index++ {
		if pointer[index] == '~' && (index+1 >= len(pointer) || (pointer[index+1] != '0' && pointer[index+1] != '1')) {
			return false
		}
	}
	return true
}

type ApprovalDecision struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	SubjectType    string    `json:"subject_type"`
	SubjectID      string    `json:"subject_id"`
	SubjectHash    string    `json:"subject_hash"`
	DecisionStage  string    `json:"decision_stage"`
	ActorID        string    `json:"actor_id"`
	Decision       string    `json:"decision"`
	Reason         string    `json:"reason"`
	PreviousState  string    `json:"previous_state"`
	ResultingState string    `json:"resulting_state"`
	CreatedAt      time.Time `json:"created_at"`
}

const (
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
)

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
		return fault.Invalid("GATE_EVALUATION_INVALID", "Gate 决定缺少任务、StageRun 或 Gate 标识")
	}
	switch v.Status {
	case GateEvaluationPending, GateEvaluationApproved, GateEvaluationRejected, GateEvaluationChangesRequested, GateEvaluationExpired:
	default:
		return fault.Invalid("GATE_EVALUATION_STATUS_INVALID", "Gate 决定状态无效")
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
		return fault.Invalid("TASK_REVISION_INVALID", "Revision 缺少任务、内容类型、Schema 或内容")
	}
	switch v.Status {
	case TaskRevisionDraft, TaskRevisionSubmitted, TaskRevisionAccepted, TaskRevisionRejected, TaskRevisionSuperseded:
	default:
		return fault.Invalid("TASK_REVISION_STATUS_INVALID", "Revision 状态无效")
	}
	if v.ContentHash != "" && !stablehash.Valid(v.ContentHash) {
		return fault.Invalid("TASK_REVISION_HASH_INVALID", "Revision content hash 必须是 sha256 摘要")
	}
	return nil
}

type Submission struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	ProjectID         string    `json:"project_id"`
	WorkspaceID       string    `json:"workspace_id"`
	SubmissionType    string    `json:"submission_type"`
	Status            string    `json:"status"`
	CurrentRevisionID string    `json:"current_revision_id"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type LocalRunSummary struct {
	RunID      string            `json:"run_id,omitempty"`
	Stage      string            `json:"stage,omitempty"`
	Checks     []LocalRunCheck   `json:"checks"`
	InputHash  string            `json:"input_hash,omitempty"`
	OutputHash string            `json:"output_hash,omitempty"`
	Versions   map[string]string `json:"versions,omitempty"`
}

type LocalRunCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type SourceDisclosure struct {
	ID                   string          `json:"id,omitempty"`
	TenantID             string          `json:"tenant_id,omitempty"`
	ProjectID            string          `json:"project_id,omitempty"`
	SubmissionRevisionID string          `json:"submission_revision_id,omitempty"`
	SourceRef            string          `json:"source_ref"`
	Level                string          `json:"level"`
	SHA256               string          `json:"sha256"`
	ByteSize             int64           `json:"byte_size,omitempty"`
	EvidencePack         json.RawMessage `json:"evidence_pack,omitempty"`
	CreatedAt            time.Time       `json:"created_at,omitempty"`
}

type SubmissionArtifact struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	ByteSize  int64  `json:"byte_size"`
}

type SubmissionObjectRef struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Version int             `json:"version"`
	Digest  string          `json:"digest"`
	Path    string          `json:"path"`
	Content json.RawMessage `json:"content"`
}

func NewSubmissionObjectRef(id, objectType string, version int, objectPath string, content any) (SubmissionObjectRef, error) {
	body, err := json.Marshal(content)
	if err != nil {
		return SubmissionObjectRef{}, err
	}
	hash, err := stablehash.Sum(json.RawMessage(body))
	if err != nil {
		return SubmissionObjectRef{}, err
	}
	return SubmissionObjectRef{ID: id, Type: objectType, Version: version, Digest: "sha256:" + hash, Path: objectPath, Content: body}, nil
}

func SubmissionSchemaVersion(submissionType string) string {
	return "contentcloud." + submissionType + "/3.0"
}

func SubmissionTypes() []string {
	return []string{"context", "knowledge", "strategy", "offer", "brief", "content_batch", "asset_batch", "storyboard", "delivery", "result"}
}

func ValidSubmissionType(value string) bool {
	for _, candidate := range SubmissionTypes() {
		if value == candidate {
			return true
		}
	}
	return false
}

type SubmissionBundle struct {
	BundleVersion     string                `json:"bundle_version"`
	SubmissionType    string                `json:"submission_type"`
	ProjectID         string                `json:"project_id"`
	WorkspaceID       string                `json:"workspace_id"`
	BaseSnapshotIDs   []string              `json:"base_snapshot_ids"`
	Objects           []SubmissionObjectRef `json:"objects"`
	SourceDisclosures []SourceDisclosure    `json:"source_disclosures"`
	LocalRunSummary   LocalRunSummary       `json:"local_run_summary"`
	EnvironmentDigest string                `json:"environment_digest"`
	Artifacts         []SubmissionArtifact  `json:"artifacts"`
	Message           string                `json:"message,omitempty"`
	ContentHash       string                `json:"content_hash"`
	IdempotencyKey    string                `json:"idempotency_key"`
}

type SubmissionRevision struct {
	ID                string                `json:"id"`
	TenantID          string                `json:"tenant_id"`
	ProjectID         string                `json:"project_id"`
	WorkspaceID       string                `json:"workspace_id"`
	SubmissionID      string                `json:"submission_id"`
	RevisionNo        int                   `json:"revision_no"`
	SchemaVersion     string                `json:"schema_version"`
	ContentHash       string                `json:"content_hash"`
	BaseSnapshotIDs   []string              `json:"base_snapshot_ids"`
	EnvironmentDigest string                `json:"environment_digest"`
	LocalRunSummary   LocalRunSummary       `json:"local_run_summary"`
	Objects           []SubmissionObjectRef `json:"objects"`
	Artifacts         []SubmissionArtifact  `json:"artifacts"`
	Message           string                `json:"message,omitempty"`
	IdempotencyKey    string                `json:"idempotency_key"`
	EvidenceLimited   bool                  `json:"evidence_limited"`
	CreatedBy         string                `json:"created_by"`
	CreatedAt         time.Time             `json:"created_at"`
	SourceDisclosures []SourceDisclosure    `json:"source_disclosures"`
}

type ApprovedSnapshot struct {
	ID                   string               `json:"id"`
	TenantID             string               `json:"tenant_id"`
	ProjectID            string               `json:"project_id"`
	WorkspaceID          string               `json:"workspace_id"`
	SubmissionID         string               `json:"submission_id"`
	SubmissionRevisionID string               `json:"submission_revision_id"`
	SubmissionType       string               `json:"submission_type"`
	SchemaVersion        string               `json:"schema_version"`
	ContentHash          string               `json:"content_hash"`
	SubjectHash          string               `json:"subject_hash"`
	CanonicalContent     json.RawMessage      `json:"canonical_content"`
	EligibleIDs          []string             `json:"eligible_ids"`
	Artifacts            []SubmissionArtifact `json:"artifacts"`
	DecisionID           string               `json:"decision_id"`
	CreatedBy            string               `json:"created_by"`
	CreatedAt            time.Time            `json:"created_at"`
}

type ReviewFeedbackBundle struct {
	BundleVersion        string          `json:"bundle_version"`
	SubmissionID         string          `json:"submission_id"`
	SubmissionRevisionID string          `json:"submission_revision_id"`
	SubjectHash          string          `json:"subject_hash"`
	Comments             []ReviewComment `json:"comments"`
	CreatedAt            time.Time       `json:"created_at"`
}

type DecisionDelta struct {
	BundleVersion string             `json:"bundle_version"`
	ProjectID     string             `json:"project_id"`
	Decisions     []ApprovalDecision `json:"decisions"`
	CreatedAt     time.Time          `json:"created_at"`
}

func (b SubmissionBundle) Validate() error {
	if b.BundleVersion != "3.0" {
		return fault.Invalid("SUBMISSION_BUNDLE_VERSION_INVALID", "bundle_version 必须为 3.0")
	}
	if !ValidSubmissionType(b.SubmissionType) {
		return fault.Invalid("SUBMISSION_TYPE_INVALID", "submission_type 不受支持")
	}
	if strings.TrimSpace(b.ProjectID) == "" || strings.TrimSpace(b.WorkspaceID) == "" {
		return fault.Invalid("SUBMISSION_CONTEXT_REQUIRED", "project_id 和 workspace_id 必填")
	}
	if len(b.Objects) == 0 {
		return fault.Invalid("SUBMISSION_OBJECTS_INVALID", "objects 必须包含至少一个版本化对象")
	}
	if !stablehash.Matches(b.EnvironmentDigest) || !strings.HasPrefix(b.EnvironmentDigest, "sha256:") {
		return fault.Invalid("SUBMISSION_ENVIRONMENT_DIGEST_INVALID", "environment_digest 必须是带 sha256: 前缀的摘要")
	}
	seenObjects := map[string]bool{}
	for _, object := range b.Objects {
		if err := object.Validate(); err != nil {
			return err
		}
		if seenObjects[object.ID] {
			return fault.Invalid("SUBMISSION_OBJECT_DUPLICATE", "objects 不能包含重复 ID")
		}
		seenObjects[object.ID] = true
	}
	seenSnapshots := map[string]bool{}
	for _, id := range b.BaseSnapshotIDs {
		if strings.TrimSpace(id) == "" || seenSnapshots[id] {
			return fault.Invalid("BASE_SNAPSHOT_INVALID", "base_snapshot_ids 不能包含空值或重复值")
		}
		seenSnapshots[id] = true
	}
	if strings.TrimSpace(b.IdempotencyKey) == "" || len(b.IdempotencyKey) > 128 {
		return fault.Invalid("IDEMPOTENCY_KEY_REQUIRED", "idempotency_key 必填且不能超过 128 字符")
	}
	seenSources := map[string]bool{}
	for _, disclosure := range b.SourceDisclosures {
		if strings.TrimSpace(disclosure.SourceRef) == "" || seenSources[disclosure.SourceRef] {
			return fault.Invalid("SOURCE_DISCLOSURE_INVALID", "source_ref 必填且不能重复")
		}
		seenSources[disclosure.SourceRef] = true
		if disclosure.Level != "metadata_only" && disclosure.Level != "evidence_pack" && disclosure.Level != "full_source" {
			return fault.Invalid("SOURCE_DISCLOSURE_LEVEL_INVALID", "披露等级必须为 metadata_only、evidence_pack 或 full_source")
		}
		if !stablehash.Matches(disclosure.SHA256) {
			return fault.Invalid("SOURCE_DISCLOSURE_HASH_INVALID", "来源 SHA-256 无效")
		}
		if disclosure.Level == "evidence_pack" && len(disclosure.EvidencePack) == 0 {
			return fault.Invalid("EVIDENCE_PACK_REQUIRED", "evidence_pack 披露必须包含可审核证据包")
		}
		if disclosure.Level != "evidence_pack" && len(disclosure.EvidencePack) > 0 {
			return fault.Invalid("EVIDENCE_PACK_LEVEL_MISMATCH", "只有 evidence_pack 披露可以携带证据包正文")
		}
	}
	for _, artifact := range b.Artifacts {
		if artifact.Name == "" || artifact.ByteSize < 0 || !stablehash.Matches(artifact.SHA256) {
			return fault.Invalid("SUBMISSION_ARTIFACT_INVALID", "Artifact manifest 无效")
		}
	}
	computed, err := b.ComputedHash()
	if err != nil {
		return err
	}
	if stablehash.Normalize(b.ContentHash) != computed {
		return fault.Conflict("SUBMISSION_HASH_MISMATCH", "提交 content_hash 与服务端复算结果不一致")
	}
	return nil
}

func (b SubmissionBundle) ComputedHash() (string, error) {
	value := struct {
		BundleVersion     string                `json:"bundle_version"`
		SubmissionType    string                `json:"submission_type"`
		ProjectID         string                `json:"project_id"`
		WorkspaceID       string                `json:"workspace_id"`
		BaseSnapshotIDs   []string              `json:"base_snapshot_ids"`
		Objects           []SubmissionObjectRef `json:"objects"`
		SourceDisclosures []SourceDisclosure    `json:"source_disclosures"`
		LocalRunSummary   LocalRunSummary       `json:"local_run_summary"`
		EnvironmentDigest string                `json:"environment_digest"`
		Artifacts         []SubmissionArtifact  `json:"artifacts"`
		Message           string                `json:"message,omitempty"`
	}{b.BundleVersion, b.SubmissionType, b.ProjectID, b.WorkspaceID, b.BaseSnapshotIDs, b.Objects, b.SourceDisclosures, b.LocalRunSummary, b.EnvironmentDigest, b.Artifacts, b.Message}
	hash, err := stablehash.Sum(value)
	if err != nil {
		return "", err
	}
	return hash, nil
}

func (b *SubmissionBundle) SetComputedHash() error {
	hash, err := b.ComputedHash()
	if err != nil {
		return err
	}
	b.ContentHash = "sha256:" + hash
	return nil
}

func (r SubmissionRevision) EligibleObjectIDs() []string {
	ids := []string{}
	for _, object := range r.Objects {
		var content map[string]any
		if json.Unmarshal(object.Content, &content) != nil {
			continue
		}
		status, _ := content["status"].(string)
		if status == "" || status == "candidate" || status == "approved" || status == "verified" || status == "review_ready" {
			ids = append(ids, object.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func EvidenceLimited(objects []SubmissionObjectRef, disclosures []SourceDisclosure) bool {
	highRisk := false
	for _, object := range objects {
		content := strings.ToLower(string(object.Content))
		if strings.Contains(content, `"risk_level":"high"`) || strings.Contains(content, `"risk_level": "high"`) {
			highRisk = true
			break
		}
	}
	if !highRisk {
		return false
	}
	for _, disclosure := range disclosures {
		if disclosure.Level == "metadata_only" {
			return true
		}
	}
	return len(disclosures) == 0
}

func (o SubmissionObjectRef) Validate() error {
	if strings.TrimSpace(o.ID) == "" || strings.TrimSpace(o.Type) == "" || o.Version < 1 {
		return fault.Invalid("SUBMISSION_OBJECT_IDENTITY_INVALID", "object 需要 id、type 和正整数 version")
	}
	clean := path.Clean(strings.TrimSpace(o.Path))
	if clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, `\`) {
		return fault.Invalid("SUBMISSION_OBJECT_PATH_INVALID", "object path 必须是 Workspace 内的相对路径")
	}
	if len(o.Content) == 0 || !json.Valid(o.Content) || string(o.Content) == "null" {
		return fault.Invalid("SUBMISSION_OBJECT_CONTENT_INVALID", "object content 必须是有效 JSON")
	}
	hash, err := stablehash.Sum(json.RawMessage(o.Content))
	if err != nil || stablehash.Normalize(o.Digest) != hash {
		return fault.Conflict("SUBMISSION_OBJECT_DIGEST_MISMATCH", "object digest 与结构化 content 不一致")
	}
	return nil
}
