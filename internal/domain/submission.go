package domain

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
)

var sha256Pattern = regexp.MustCompile(`^(sha256:)?[0-9a-f]{64}$`)

type WorkspaceBinding struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	ProjectID       string     `json:"project_id"`
	DeviceID        string     `json:"device_id,omitempty"`
	OwnerUserID     string     `json:"owner_user_id"`
	TemplateID      string     `json:"template_id"`
	TemplateVersion string     `json:"template_version"`
	Targets         []string   `json:"targets"`
	CredentialHash  string     `json:"-"`
	Status          string     `json:"status"`
	InitializedAt   time.Time  `json:"initialized_at"`
	LastSeenAt      time.Time  `json:"last_seen_at"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
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

type SubmissionBundle struct {
	BundleVersion          string               `json:"bundle_version"`
	SchemaVersion          string               `json:"schema_version"`
	SubmissionType         string               `json:"submission_type"`
	ProjectID              string               `json:"project_id"`
	WorkspaceID            string               `json:"workspace_id"`
	BaseApprovedSnapshotID string               `json:"base_approved_snapshot_id,omitempty"`
	LocalRunSummary        LocalRunSummary      `json:"local_run_summary"`
	Objects                json.RawMessage      `json:"objects"`
	SourceDisclosures      []SourceDisclosure   `json:"source_disclosures"`
	Artifacts              []SubmissionArtifact `json:"artifacts"`
	Message                string               `json:"message,omitempty"`
	ContentHash            string               `json:"content_hash"`
	IdempotencyKey         string               `json:"idempotency_key"`
}

type SubmissionRevision struct {
	ID                     string               `json:"id"`
	TenantID               string               `json:"tenant_id"`
	ProjectID              string               `json:"project_id"`
	WorkspaceID            string               `json:"workspace_id"`
	SubmissionID           string               `json:"submission_id"`
	RevisionNo             int                  `json:"revision_no"`
	SchemaVersion          string               `json:"schema_version"`
	ContentHash            string               `json:"content_hash"`
	BaseApprovedSnapshotID string               `json:"base_approved_snapshot_id,omitempty"`
	LocalRunSummary        LocalRunSummary      `json:"local_run_summary"`
	Objects                json.RawMessage      `json:"objects"`
	Artifacts              []SubmissionArtifact `json:"artifacts"`
	Message                string               `json:"message,omitempty"`
	IdempotencyKey         string               `json:"idempotency_key"`
	EvidenceLimited        bool                 `json:"evidence_limited"`
	CreatedBy              string               `json:"created_by"`
	CreatedAt              time.Time            `json:"created_at"`
	SourceDisclosures      []SourceDisclosure   `json:"source_disclosures"`
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
	Origin               string               `json:"origin"`
	ExternalRef          string               `json:"external_ref,omitempty"`
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
	allowedTypes := map[string]bool{"knowledge": true, "research": true, "strategy": true, "brief": true, "script": true, "delivery": true, "performance": true}
	if b.BundleVersion != "1.0" {
		return Invalid("SUBMISSION_BUNDLE_VERSION_INVALID", "bundle_version 必须为 1.0")
	}
	if !allowedTypes[b.SubmissionType] {
		return Invalid("SUBMISSION_TYPE_INVALID", "submission_type 不受支持")
	}
	if strings.TrimSpace(b.ProjectID) == "" || strings.TrimSpace(b.WorkspaceID) == "" {
		return Invalid("SUBMISSION_CONTEXT_REQUIRED", "project_id 和 workspace_id 必填")
	}
	if strings.TrimSpace(b.SchemaVersion) == "" {
		return Invalid("SUBMISSION_SCHEMA_REQUIRED", "schema_version 必填")
	}
	if !validJSONArray(b.Objects) {
		return Invalid("SUBMISSION_OBJECTS_INVALID", "objects 必须是有效 JSON 数组")
	}
	if strings.TrimSpace(b.IdempotencyKey) == "" || len(b.IdempotencyKey) > 128 {
		return Invalid("IDEMPOTENCY_KEY_REQUIRED", "idempotency_key 必填且不能超过 128 字符")
	}
	seenSources := map[string]bool{}
	for _, disclosure := range b.SourceDisclosures {
		if strings.TrimSpace(disclosure.SourceRef) == "" || seenSources[disclosure.SourceRef] {
			return Invalid("SOURCE_DISCLOSURE_INVALID", "source_ref 必填且不能重复")
		}
		seenSources[disclosure.SourceRef] = true
		if disclosure.Level != "metadata_only" && disclosure.Level != "evidence_pack" && disclosure.Level != "full_source" {
			return Invalid("SOURCE_DISCLOSURE_LEVEL_INVALID", "披露等级必须为 metadata_only、evidence_pack 或 full_source")
		}
		if !sha256Pattern.MatchString(disclosure.SHA256) {
			return Invalid("SOURCE_DISCLOSURE_HASH_INVALID", "来源 SHA-256 无效")
		}
		if disclosure.Level == "evidence_pack" && len(disclosure.EvidencePack) == 0 {
			return Invalid("EVIDENCE_PACK_REQUIRED", "evidence_pack 披露必须包含可审核证据包")
		}
	}
	for _, artifact := range b.Artifacts {
		if artifact.Name == "" || artifact.ByteSize < 0 || !sha256Pattern.MatchString(artifact.SHA256) {
			return Invalid("SUBMISSION_ARTIFACT_INVALID", "Artifact manifest 无效")
		}
	}
	computed, err := b.ComputedHash()
	if err != nil {
		return err
	}
	if normalizeHash(b.ContentHash) != computed {
		return Conflict("SUBMISSION_HASH_MISMATCH", "提交 content_hash 与服务端复算结果不一致")
	}
	return nil
}

func (b SubmissionBundle) ComputedHash() (string, error) {
	value := struct {
		BundleVersion          string               `json:"bundle_version"`
		SchemaVersion          string               `json:"schema_version"`
		SubmissionType         string               `json:"submission_type"`
		ProjectID              string               `json:"project_id"`
		WorkspaceID            string               `json:"workspace_id"`
		BaseApprovedSnapshotID string               `json:"base_approved_snapshot_id,omitempty"`
		LocalRunSummary        LocalRunSummary      `json:"local_run_summary"`
		Objects                json.RawMessage      `json:"objects"`
		SourceDisclosures      []SourceDisclosure   `json:"source_disclosures"`
		Artifacts              []SubmissionArtifact `json:"artifacts"`
		Message                string               `json:"message,omitempty"`
	}{b.BundleVersion, b.SchemaVersion, b.SubmissionType, b.ProjectID, b.WorkspaceID, b.BaseApprovedSnapshotID, b.LocalRunSummary, b.Objects, b.SourceDisclosures, b.Artifacts, b.Message}
	hash, err := CanonicalHash(value)
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
	var objects []map[string]any
	if json.Unmarshal(r.Objects, &objects) != nil {
		return []string{}
	}
	ids := []string{}
	for _, object := range objects {
		id, _ := object["id"].(string)
		status, _ := object["status"].(string)
		if id == "" {
			continue
		}
		if status == "" || status == "candidate" || status == "approved" || status == "verified" || status == "review_ready" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func EvidenceLimited(objects json.RawMessage, disclosures []SourceDisclosure) bool {
	highRisk := strings.Contains(strings.ToLower(string(objects)), `"risk_level":"high"`) || strings.Contains(strings.ToLower(string(objects)), `"risk_level": "high"`)
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

func normalizeHash(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
}

func validJSONArray(body json.RawMessage) bool {
	var value []json.RawMessage
	return len(body) > 0 && json.Unmarshal(body, &value) == nil
}
