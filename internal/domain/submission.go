package domain

import (
	"encoding/json"
	"path"
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
	hash, err := CanonicalHash(json.RawMessage(body))
	if err != nil {
		return SubmissionObjectRef{}, err
	}
	return SubmissionObjectRef{ID: id, Type: objectType, Version: version, Digest: "sha256:" + hash, Path: objectPath, Content: body}, nil
}

func SubmissionSchemaVersion(submissionType string) string {
	return "contentcloud." + submissionType + "/3.0"
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
	allowedTypes := map[string]bool{"context": true, "knowledge": true, "brief": true, "content_batch": true, "asset_batch": true, "delivery": true, "result": true}
	if b.BundleVersion != "3.0" {
		return Invalid("SUBMISSION_BUNDLE_VERSION_INVALID", "bundle_version 必须为 3.0")
	}
	if !allowedTypes[b.SubmissionType] {
		return Invalid("SUBMISSION_TYPE_INVALID", "submission_type 不受支持")
	}
	if strings.TrimSpace(b.ProjectID) == "" || strings.TrimSpace(b.WorkspaceID) == "" {
		return Invalid("SUBMISSION_CONTEXT_REQUIRED", "project_id 和 workspace_id 必填")
	}
	if len(b.Objects) == 0 {
		return Invalid("SUBMISSION_OBJECTS_INVALID", "objects 必须包含至少一个版本化对象")
	}
	if !sha256Pattern.MatchString(b.EnvironmentDigest) || !strings.HasPrefix(b.EnvironmentDigest, "sha256:") {
		return Invalid("SUBMISSION_ENVIRONMENT_DIGEST_INVALID", "environment_digest 必须是带 sha256: 前缀的摘要")
	}
	seenObjects := map[string]bool{}
	for _, object := range b.Objects {
		if err := object.Validate(); err != nil {
			return err
		}
		if seenObjects[object.ID] {
			return Invalid("SUBMISSION_OBJECT_DUPLICATE", "objects 不能包含重复 ID")
		}
		seenObjects[object.ID] = true
	}
	seenSnapshots := map[string]bool{}
	for _, id := range b.BaseSnapshotIDs {
		if strings.TrimSpace(id) == "" || seenSnapshots[id] {
			return Invalid("BASE_SNAPSHOT_INVALID", "base_snapshot_ids 不能包含空值或重复值")
		}
		seenSnapshots[id] = true
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
		if disclosure.Level != "evidence_pack" && len(disclosure.EvidencePack) > 0 {
			return Invalid("EVIDENCE_PACK_LEVEL_MISMATCH", "只有 evidence_pack 披露可以携带证据包正文")
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

func normalizeHash(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
}

func (o SubmissionObjectRef) Validate() error {
	if strings.TrimSpace(o.ID) == "" || strings.TrimSpace(o.Type) == "" || o.Version < 1 {
		return Invalid("SUBMISSION_OBJECT_IDENTITY_INVALID", "object 需要 id、type 和正整数 version")
	}
	clean := path.Clean(strings.TrimSpace(o.Path))
	if clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, `\`) {
		return Invalid("SUBMISSION_OBJECT_PATH_INVALID", "object path 必须是 Workspace 内的相对路径")
	}
	if len(o.Content) == 0 || !json.Valid(o.Content) || string(o.Content) == "null" {
		return Invalid("SUBMISSION_OBJECT_CONTENT_INVALID", "object content 必须是有效 JSON")
	}
	hash, err := CanonicalHash(json.RawMessage(o.Content))
	if err != nil || normalizeHash(o.Digest) != hash {
		return Conflict("SUBMISSION_OBJECT_DIGEST_MISMATCH", "object digest 与结构化 content 不一致")
	}
	return nil
}
