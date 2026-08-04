package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	KnowledgeCandidatesSchema = "knowledge-candidates/1.0"
	TaskContractSchema        = "task-contract/1.0"
)

type Tenant struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"display_name"`
	PasswordHash string     `json:"-"`
	VerifiedAt   *time.Time `json:"verified_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Membership struct {
	TenantID  string     `json:"tenant_id"`
	UserID    string     `json:"user_id"`
	Role      string     `json:"role"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type MembershipInvite struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	Email          string     `json:"email"`
	Role           string     `json:"role"`
	InvitedBy      string     `json:"invited_by"`
	TokenHash      string     `json:"-"`
	Status         string     `json:"status"`
	ExpiresAt      time.Time  `json:"expires_at"`
	AcceptedBy     string     `json:"accepted_by,omitempty"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	PlaintextToken string     `json:"invite_token,omitempty"`
}

func (v MembershipInvite) ValidateAcceptance(email string, now time.Time) error {
	if v.Status != "pending" || v.RevokedAt != nil || now.After(v.ExpiresAt) || !strings.EqualFold(v.Email, email) {
		return Conflict("INVITE_INVALID", "邀请无效、已撤销、邮箱不匹配或已过期")
	}
	return nil
}

type Session struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	TenantID  string     `json:"tenant_id"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type Project struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	Slug             string    `json:"slug"`
	BrandName        string    `json:"brand_name"`
	ProductName      string    `json:"product_name"`
	ContentType      string    `json:"content_type"`
	Channel          string    `json:"channel"`
	StageObjective   string    `json:"stage_objective"`
	Status           string    `json:"status"`
	OwnerName        string    `json:"owner_name"`
	ReviewerName     string    `json:"reviewer_name"`
	ClientApprover   string    `json:"client_approver"`
	RowVersion       int       `json:"row_version"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	ConnectedDevices int       `json:"connected_devices"`
	KnowledgeReady   int       `json:"knowledge_ready"`
	OpenBlockers     int       `json:"open_blockers"`
}

type ProjectTemplate struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Name           string    `json:"name"`
	Channel        string    `json:"channel"`
	StageObjective string    `json:"stage_objective"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

type ConnectSession struct {
	ID               string             `json:"id"`
	TenantID         string             `json:"tenant_id"`
	ProjectID        string             `json:"project_id"`
	InviterUserID    string             `json:"inviter_user_id"`
	State            string             `json:"state"`
	ExpiresAt        time.Time          `json:"expires_at"`
	ConsumedAt       *time.Time         `json:"consumed_at,omitempty"`
	ConsumedDeviceID string             `json:"consumed_device_id,omitempty"`
	Progress         *BootstrapProgress `json:"progress,omitempty"`
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

type Device struct {
	ID           string       `json:"id"`
	TenantID     string       `json:"tenant_id"`
	OwnerUserID  string       `json:"owner_user_id"`
	DisplayName  string       `json:"display_name"`
	Hostname     string       `json:"hostname"`
	Platform     string       `json:"platform"`
	Arch         string       `json:"arch"`
	Version      string       `json:"daemon_version"`
	TokenHash    string       `json:"-"`
	Capabilities []Capability `json:"capabilities"`
	ProjectIDs   []string     `json:"project_ids"`
	LastSeenAt   time.Time    `json:"last_seen_at"`
	RevokedAt    *time.Time   `json:"revoked_at,omitempty"`
}

type EvidenceRef struct {
	SourceRevisionID string `json:"source_revision_id"`
	LocatorKind      string `json:"locator_kind"`
	Locator          string `json:"locator"`
	Quote            string `json:"quote"`
}

type TypedValue struct {
	Type    string   `json:"type"`
	Text    string   `json:"text,omitempty"`
	Number  *float64 `json:"number,omitempty"`
	Boolean *bool    `json:"boolean,omitempty"`
	Unit    string   `json:"unit,omitempty"`
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

type TaskContract struct {
	ContractVersion string           `json:"contract_version"`
	ContractID      string           `json:"contract_id"`
	RunID           string           `json:"run_id"`
	TaskType        string           `json:"task_type"`
	Project         Project          `json:"project"`
	Sources         []ContractSource `json:"sources"`
	InputSnapshotID string           `json:"input_snapshot_id"`
	OutputSchema    string           `json:"output_schema"`
	Capability      Capability       `json:"required_capability"`
	ManifestHash    string           `json:"manifest_hash"`
}

type TaskRun struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id"`
	ProjectID         string     `json:"project_id"`
	WorkTaskID        string     `json:"work_task_id,omitempty"`
	SOPID             string     `json:"sop_id,omitempty"`
	SOPVersion        int        `json:"sop_version,omitempty"`
	SOPDigest         string     `json:"sop_digest,omitempty"`
	StageID           string     `json:"stage_id,omitempty"`
	ExecutionMode     string     `json:"execution_mode,omitempty"`
	ExecutorKind      string     `json:"executor_kind,omitempty"`
	OutputRefs        []string   `json:"output_refs,omitempty"`
	TaskRevisionID    string     `json:"task_revision_id,omitempty"`
	GateEvaluationID  string     `json:"gate_evaluation_id,omitempty"`
	InputSnapshotID   string     `json:"input_snapshot_id"`
	IdempotencyKey    string     `json:"idempotency_key"`
	TaskType          string     `json:"task_type"`
	CapabilityID      string     `json:"capability_id"`
	CapabilityVersion string     `json:"capability_version"`
	InputSchema       string     `json:"input_schema"`
	OutputSchema      string     `json:"output_schema"`
	OutputCount       int        `json:"output_count"`
	DeliveryProfiles  []string   `json:"delivery_profiles"`
	State             string     `json:"state"`
	Priority          int        `json:"priority"`
	AttemptCount      int        `json:"attempt_count"`
	ActiveAttemptID   string     `json:"active_attempt_id,omitempty"`
	LeaseDeviceID     string     `json:"lease_device_id,omitempty"`
	LeaseExpiresAt    *time.Time `json:"lease_expires_at,omitempty"`
	RunTokenHash      string     `json:"-"`
	ProgressLabel     string     `json:"progress_label,omitempty"`
	ErrorCode         string     `json:"error_code,omitempty"`
	CancelRequestedAt *time.Time `json:"cancel_requested_at,omitempty"`
	ReportHash        string     `json:"report_hash,omitempty"`
	HeartbeatSequence int        `json:"heartbeat_sequence"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type RunAttempt struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenant_id"`
	ProjectID         string         `json:"project_id"`
	RunID             string         `json:"run_id"`
	DeviceID          string         `json:"device_id"`
	State             string         `json:"state"`
	CapabilityID      string         `json:"capability_id"`
	CapabilityVersion string         `json:"capability_version"`
	CapabilityDigest  string         `json:"capability_digest"`
	InputSchema       string         `json:"input_schema"`
	OutputSchema      string         `json:"output_schema"`
	TokenHash         string         `json:"-"`
	LeaseExpiresAt    time.Time      `json:"lease_expires_at"`
	HeartbeatAt       *time.Time     `json:"heartbeat_at,omitempty"`
	StartedAt         *time.Time     `json:"started_at,omitempty"`
	FinishedAt        *time.Time     `json:"finished_at,omitempty"`
	ExitCode          *int           `json:"exit_code,omitempty"`
	FailureClass      string         `json:"failure_class,omitempty"`
	Usage             map[string]any `json:"usage"`
	TranscriptSummary string         `json:"transcript_summary,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}

func (r TaskRun) AcceptsCapability(capability Capability) bool {
	return capability.ID == r.CapabilityID &&
		capability.Version == r.CapabilityVersion &&
		capability.Kind == "business_capability" &&
		capability.InputSchema == r.InputSchema &&
		capability.OutputSchema == r.OutputSchema &&
		capability.LocalOnly
}

type RunHeartbeat struct {
	Sequence int    `json:"sequence"`
	Phase    string `json:"phase"`
	Step     int    `json:"step"`
	Label    string `json:"label"`
}

type RunProgressEvent struct {
	Cursor     int64     `json:"cursor"`
	TenantID   string    `json:"-"`
	ProjectID  string    `json:"project_id"`
	RunID      string    `json:"run_id"`
	AttemptID  string    `json:"attempt_id"`
	DeviceID   string    `json:"device_id"`
	Sequence   int       `json:"sequence"`
	Phase      string    `json:"phase"`
	Step       int       `json:"step"`
	Label      string    `json:"label"`
	OccurredAt time.Time `json:"occurred_at"`
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

type AuditEvent struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	ProjectID   string         `json:"project_id,omitempty"`
	ActorType   string         `json:"actor_type"`
	ActorID     string         `json:"actor_id"`
	Action      string         `json:"action"`
	SubjectType string         `json:"subject_type"`
	SubjectID   string         `json:"subject_id"`
	Summary     map[string]any `json:"summary"`
	RequestID   string         `json:"request_id"`
	CreatedAt   time.Time      `json:"created_at"`
}

func NewID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

func NewOpaqueToken(prefix string, bytes int) (plain string, hash string, err error) {
	b := make([]byte, bytes)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plain = prefix + base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(plain))
	return plain, hex.EncodeToString(sum[:]), nil
}

func TokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func CanonicalHash(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func CompileKnowledgeSnapshot(project Project, sources []ContractSource, now time.Time) (ContextSnapshot, error) {
	if len(sources) == 0 {
		return ContextSnapshot{}, fmt.Errorf("knowledge extraction requires at least one source revision")
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
	hash, err := CanonicalHash(payload)
	if err != nil {
		return ContextSnapshot{}, err
	}
	return ContextSnapshot{ID: NewID(), TenantID: project.TenantID, ProjectID: project.ID, BuilderVersion: "knowledge-contract/1.0.0", SchemaVersion: TaskContractSchema, Sources: ordered, InputVersions: versions, ManifestHash: hash, CreatedAt: now.UTC()}, nil
}
