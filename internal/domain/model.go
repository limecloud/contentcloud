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
	ScriptPackageSchema       = "script-package/1.1"
	KnowledgeCandidatesSchema = "knowledge-candidates/1.0"
	TaskContractSchema        = "task-contract/1.0"
	ScriptCapability          = "contentcloud.script.generate"
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
	ID                  string     `json:"id"`
	TenantID            string     `json:"tenant_id"`
	ProjectID           string     `json:"project_id"`
	InviterUserID       string     `json:"inviter_user_id"`
	ConnectKeyHash      string     `json:"-"`
	State               string     `json:"state"`
	ExpiresAt           time.Time  `json:"expires_at"`
	ConsumedAt          *time.Time `json:"consumed_at,omitempty"`
	ConsumedDeviceID    string     `json:"consumed_device_id,omitempty"`
	PlaintextConnectKey string     `json:"connect_key,omitempty"`
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

type KnowledgeItem struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	ProjectID           string         `json:"project_id"`
	Kind                string         `json:"kind"`
	Title               string         `json:"title"`
	Statement           string         `json:"statement"`
	Subject             string         `json:"subject"`
	Predicate           string         `json:"predicate"`
	Value               TypedValue     `json:"value"`
	Scope               KnowledgeScope `json:"scope"`
	Status              string         `json:"status"`
	RiskLevel           string         `json:"risk_level"`
	AllowedChannels     []string       `json:"allowed_channels"`
	Evidence            []EvidenceRef  `json:"evidence"`
	ForbiddenExtensions []string       `json:"forbidden_extensions"`
	DependsOnFactIDs    []string       `json:"depends_on_fact_ids"`
	ValidFrom           *time.Time     `json:"valid_from,omitempty"`
	ValidUntil          *time.Time     `json:"valid_until,omitempty"`
	ExpiresAt           *time.Time     `json:"expires_at,omitempty"`
	ApprovedBy          string         `json:"approved_by,omitempty"`
	ApprovedAt          *time.Time     `json:"approved_at,omitempty"`
	OriginRunID         string         `json:"origin_run_id,omitempty"`
	DecisionRequired    bool           `json:"decision_required"`
	RowVersion          int            `json:"row_version"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
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

type KnowledgeConflict struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	ProjectID        string     `json:"project_id"`
	Subject          string     `json:"subject"`
	Predicate        string     `json:"predicate"`
	KnowledgeItemIDs []string   `json:"knowledge_item_ids"`
	Reason           string     `json:"reason"`
	Status           string     `json:"status"`
	ResolvedBy       string     `json:"resolved_by,omitempty"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	Resolution       string     `json:"resolution,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type DecisionRequest struct {
	ID                  string     `json:"id"`
	TenantID            string     `json:"tenant_id"`
	ProjectID           string     `json:"project_id"`
	ConflictID          string     `json:"conflict_id"`
	Question            string     `json:"question"`
	KnowledgeItemIDs    []string   `json:"knowledge_item_ids"`
	Status              string     `json:"status"`
	RequestedBy         string     `json:"requested_by"`
	ResolvedBy          string     `json:"resolved_by,omitempty"`
	ResolvedAt          *time.Time `json:"resolved_at,omitempty"`
	SelectedKnowledgeID string     `json:"selected_knowledge_id,omitempty"`
	Notes               string     `json:"notes,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type BriefVersion struct {
	ID                     string     `json:"id"`
	TenantID               string     `json:"tenant_id"`
	ProjectID              string     `json:"project_id"`
	Version                int        `json:"version"`
	Status                 string     `json:"status"`
	Objective              string     `json:"objective"`
	Audience               string     `json:"audience"`
	DemandMoment           string     `json:"demand_moment"`
	Scene                  string     `json:"scene"`
	Conflict               string     `json:"conflict"`
	PrimarySellingPoint    string     `json:"primary_selling_point"`
	SecondarySellingPoints []string   `json:"secondary_selling_points"`
	CTA                    string     `json:"cta"`
	Channel                string     `json:"channel"`
	AspectRatio            string     `json:"aspect_ratio"`
	EvidenceSummary        string     `json:"evidence_summary"`
	TargetDurationSeconds  int        `json:"target_duration_seconds"`
	PrimaryTestVariable    string     `json:"primary_test_variable"`
	ApprovedKnowledgeIDs   []string   `json:"approved_knowledge_ids"`
	FrameworkIDs           []string   `json:"framework_ids"`
	VisualizationPlanIDs   []string   `json:"visualization_plan_ids"`
	Viewpoint              string     `json:"viewpoint"`
	Constraints            []string   `json:"constraints"`
	SupersedesID           string     `json:"supersedes_id,omitempty"`
	RevisionReason         string     `json:"revision_reason,omitempty"`
	CreatedBy              string     `json:"created_by"`
	ApprovedBy             string     `json:"approved_by,omitempty"`
	ApprovedAt             *time.Time `json:"approved_at,omitempty"`
	ContentHash            string     `json:"content_hash"`
	CreatedAt              time.Time  `json:"created_at"`
}

type ContextSnapshot struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	ProjectID      string            `json:"project_id"`
	BriefVersionID string            `json:"brief_version_id"`
	BuilderVersion string            `json:"builder_version"`
	SchemaVersion  string            `json:"schema_version"`
	Knowledge      []KnowledgeItem   `json:"knowledge"`
	Assets         []AssetBundle     `json:"assets"`
	Sources        []ContractSource  `json:"sources,omitempty"`
	InputVersions  map[string]string `json:"input_versions"`
	ManifestHash   string            `json:"manifest_hash"`
	CreatedAt      time.Time         `json:"created_at"`
}

type TaskContract struct {
	ContractVersion       string               `json:"contract_version"`
	ContractID            string               `json:"contract_id"`
	RunID                 string               `json:"run_id"`
	TaskType              string               `json:"task_type"`
	Project               Project              `json:"project"`
	Brief                 BriefVersion         `json:"brief"`
	Knowledge             []KnowledgeItem      `json:"knowledge"`
	Assets                []AssetBundle        `json:"assets"`
	Sources               []ContractSource     `json:"sources,omitempty"`
	BaselineScriptVersion *ScriptVersion       `json:"baseline_script_version,omitempty"`
	ChangeRequest         *ScriptChangeRequest `json:"change_request,omitempty"`
	InputSnapshotID       string               `json:"input_snapshot_id"`
	OutputSchema          string               `json:"output_schema"`
	Capability            Capability           `json:"required_capability"`
	ManifestHash          string               `json:"manifest_hash"`
}

type TaskRun struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id"`
	ProjectID         string     `json:"project_id"`
	BriefVersionID    string     `json:"brief_version_id"`
	InputSnapshotID   string     `json:"input_snapshot_id"`
	IdempotencyKey    string     `json:"idempotency_key"`
	TaskType          string     `json:"task_type"`
	CapabilityID      string     `json:"capability_id"`
	CapabilityVersion string     `json:"capability_version"`
	InputSchema       string     `json:"input_schema"`
	OutputSchema      string     `json:"output_schema"`
	OutputCount       int        `json:"output_count"`
	DeliveryProfiles  []string   `json:"delivery_profiles"`
	ScriptID          string     `json:"script_id,omitempty"`
	BaselineVersionID string     `json:"baseline_script_version_id,omitempty"`
	ChangeType        string     `json:"change_type,omitempty"`
	InvariantFields   []string   `json:"invariant_fields,omitempty"`
	ExpectedChanges   []string   `json:"expected_changed_fields,omitempty"`
	Hypothesis        string     `json:"hypothesis,omitempty"`
	RevisionReason    string     `json:"revision_reason,omitempty"`
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

type ProductionBible struct {
	Subjects        []SubjectLock `json:"subjects"`
	SceneLock       string        `json:"scene_lock"`
	VisualStyleLock string        `json:"visual_style_lock"`
	AssetIDs        []string      `json:"asset_ids"`
}

type SubjectLock struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	IdentityAnchors  []string `json:"identity_anchors"`
	WardrobeAndProps []string `json:"wardrobe_and_props"`
}

type FrameSpec struct {
	VisualState string `json:"visual_state"`
	PromptZH    string `json:"prompt_zh"`
}

type Continuity struct {
	IncomingState string `json:"incoming_state"`
	OutgoingState string `json:"outgoing_state"`
	MovementAxis  string `json:"movement_axis"`
	LightingLock  string `json:"lighting_lock"`
	ProductLock   string `json:"product_lock"`
}

type Shot struct {
	ShotID               string     `json:"shot_id"`
	StartMS              int        `json:"start_ms"`
	EndMS                int        `json:"end_ms"`
	Role                 string     `json:"role"`
	NarrativePurpose     string     `json:"narrative_purpose"`
	Subject              string     `json:"subject"`
	VisualIntent         string     `json:"visual_intent"`
	SubjectAction        string     `json:"subject_action"`
	Composition          string     `json:"composition"`
	CameraMotion         string     `json:"camera_motion"`
	FirstFrame           FrameSpec  `json:"first_frame"`
	MotionSpec           string     `json:"motion_spec"`
	EndFrame             FrameSpec  `json:"end_frame"`
	Voiceover            string     `json:"voiceover,omitempty"`
	OnScreenText         string     `json:"on_screen_text,omitempty"`
	SoundIntent          string     `json:"sound_intent"`
	KnowledgeRefs        []string   `json:"knowledge_refs"`
	ReferenceAssetIDs    []string   `json:"reference_asset_ids"`
	NegativeConstraints  []string   `json:"negative_constraints"`
	Continuity           Continuity `json:"continuity"`
	ProductTruthStrategy string     `json:"product_truth_strategy"`
	VisualizationPlanID  string     `json:"visualization_plan_id,omitempty"`
	AcceptanceCriteria   []string   `json:"acceptance_criteria"`
	PlanB                string     `json:"plan_b,omitempty"`
}

type Citation struct {
	KnowledgeID string `json:"knowledge_id"`
	ShotID      string `json:"shot_id"`
	Usage       string `json:"usage"`
}

type BlockReason struct {
	Code       string `json:"code"`
	ObjectID   string `json:"object_id,omitempty"`
	Message    string `json:"message"`
	NextAction string `json:"next_action"`
}

type ScriptPackage struct {
	SchemaVersion         string           `json:"schema_version"`
	Deliverability        string           `json:"deliverability"`
	Title                 string           `json:"title"`
	Channel               string           `json:"channel"`
	TargetDurationSeconds int              `json:"target_duration_seconds"`
	AspectRatio           string           `json:"aspect_ratio"`
	CreativeStrategy      CreativeStrategy `json:"creative_strategy"`
	ProductionBible       ProductionBible  `json:"production_bible"`
	Narrative             []string         `json:"narrative"`
	Shots                 []Shot           `json:"shots"`
	Citations             []Citation       `json:"citations"`
	BlockedReasons        []BlockReason    `json:"blocked_reasons"`
	MissingInputs         []string         `json:"missing_inputs"`
}

type CreativeStrategy struct {
	Objective              string   `json:"objective"`
	Audience               string   `json:"audience"`
	DemandMoment           string   `json:"demand_moment"`
	PrimarySellingPoint    string   `json:"primary_selling_point"`
	SecondarySellingPoints []string `json:"secondary_selling_points"`
	CTA                    string   `json:"cta"`
	Hypothesis             string   `json:"hypothesis"`
	PrimaryTestVariable    string   `json:"primary_test_variable"`
	InvariantFields        []string `json:"invariant_fields"`
}

type Script struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	ProjectID string    `json:"project_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
}

type ScriptChangeRequest struct {
	ChangeType      string   `json:"change_type"`
	InvariantFields []string `json:"invariant_fields"`
	ChangedFields   []string `json:"changed_fields"`
	Hypothesis      string   `json:"hypothesis,omitempty"`
	RevisionReason  string   `json:"revision_reason"`
}

type ScriptVersion struct {
	ID              string           `json:"id"`
	TenantID        string           `json:"tenant_id"`
	ProjectID       string           `json:"project_id"`
	ScriptID        string           `json:"script_id"`
	RunID           string           `json:"run_id"`
	Version         int              `json:"version"`
	SupersedesID    string           `json:"supersedes_id,omitempty"`
	BaselineID      string           `json:"baseline_script_version_id,omitempty"`
	ChangeType      string           `json:"change_type"`
	InvariantFields []string         `json:"invariant_fields"`
	ChangedFields   []string         `json:"changed_fields"`
	Hypothesis      string           `json:"hypothesis,omitempty"`
	RevisionReason  string           `json:"revision_reason,omitempty"`
	Status          string           `json:"status"`
	InputSnapshotID string           `json:"input_snapshot_id"`
	ContentHash     string           `json:"content_hash"`
	Package         ScriptPackage    `json:"package"`
	Validation      ValidationReport `json:"validation"`
	CreatedAt       time.Time        `json:"created_at"`
}

type RunHeartbeat struct {
	Sequence int    `json:"sequence"`
	Phase    string `json:"phase"`
	Step     int    `json:"step"`
	Label    string `json:"label"`
}

type ValidationReport struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationIssue `json:"errors"`
	Warnings []ValidationIssue `json:"warnings"`
}

type ValidationIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ApprovalDecision struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	SubjectType    string    `json:"subject_type"`
	SubjectID      string    `json:"subject_id"`
	SubjectHash    string    `json:"subject_hash"`
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

func CompileSnapshot(project Project, brief BriefVersion, knowledge []KnowledgeItem, now time.Time) (ContextSnapshot, error) {
	return CompileSnapshotWithAssets(project, brief, knowledge, nil, now)
}

func CompileSnapshotWithAssets(project Project, brief BriefVersion, knowledge []KnowledgeItem, assets []AssetBundle, now time.Time) (ContextSnapshot, error) {
	sort.Slice(knowledge, func(i, j int) bool { return knowledge[i].ID < knowledge[j].ID })
	sort.Slice(assets, func(i, j int) bool { return assets[i].Asset.ID < assets[j].Asset.ID })
	versions := map[string]string{"brief:" + brief.ID: brief.ContentHash}
	for _, item := range knowledge {
		versions["knowledge:"+item.ID] = fmt.Sprintf("%d", item.RowVersion)
	}
	for _, bundle := range assets {
		versions["asset:"+bundle.Asset.ID] = bundle.Asset.Status
		versions["rights:"+bundle.Rights.ID] = fmt.Sprintf("%d", bundle.Rights.RowVersion)
	}
	manifest := struct {
		ProjectID string            `json:"project_id"`
		BriefID   string            `json:"brief_id"`
		Versions  map[string]string `json:"versions"`
		Builder   string            `json:"builder"`
		Schema    string            `json:"schema"`
	}{project.ID, brief.ID, versions, "context-compiler/1.0.0", TaskContractSchema}
	hash, err := CanonicalHash(manifest)
	if err != nil {
		return ContextSnapshot{}, err
	}
	return ContextSnapshot{ID: NewID(), TenantID: project.TenantID, ProjectID: project.ID, BriefVersionID: brief.ID, BuilderVersion: "1.0.0", SchemaVersion: TaskContractSchema, Knowledge: knowledge, Assets: assets, InputVersions: versions, ManifestHash: hash, CreatedAt: now.UTC()}, nil
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
	}{project.ID, versions, ordered, "context-compiler/1.0.0", TaskContractSchema}
	hash, err := CanonicalHash(payload)
	if err != nil {
		return ContextSnapshot{}, err
	}
	return ContextSnapshot{ID: NewID(), TenantID: project.TenantID, ProjectID: project.ID, BuilderVersion: "1.0.0", SchemaVersion: TaskContractSchema, Sources: ordered, InputVersions: versions, ManifestHash: hash, CreatedAt: now.UTC()}, nil
}

func ValidateScript(pkg ScriptPackage, contract TaskContract) ValidationReport {
	report := ValidationReport{Valid: true, Errors: []ValidationIssue{}, Warnings: []ValidationIssue{}}
	add := func(path, code, message string) {
		report.Valid = false
		report.Errors = append(report.Errors, ValidationIssue{Path: path, Code: code, Message: message})
	}
	if pkg.SchemaVersion != "1.1" {
		add("/schema_version", "SCHEMA_VERSION_UNSUPPORTED", "仅接受 Script Package 1.1")
	}
	if pkg.Deliverability != "blocked" && pkg.Deliverability != "review_ready" {
		add("/deliverability", "DELIVERABILITY_INVALID", "deliverability 必须为 blocked 或 review_ready")
	}
	if pkg.Deliverability == "blocked" && len(pkg.BlockedReasons) == 0 {
		add("/blocked_reasons", "BLOCK_REASON_REQUIRED", "阻断结果必须说明原因")
	}
	if pkg.Deliverability == "review_ready" && len(pkg.Shots) == 0 {
		add("/shots", "SHOTS_REQUIRED", "可审核剧本至少包含一个镜头")
	}
	if pkg.Deliverability == "review_ready" && pkg.AspectRatio != "9:16" {
		add("/aspect_ratio", "ASPECT_RATIO_INVALID", "抖音 V1 正式剧本必须使用 9:16")
	}
	allowedAssets := map[string]bool{}
	for _, bundle := range contract.Assets {
		allowedAssets[bundle.Asset.ID] = true
	}
	for index, assetID := range pkg.ProductionBible.AssetIDs {
		if !allowedAssets[assetID] {
			add(fmt.Sprintf("/production_bible/asset_ids/%d", index), "ASSET_RIGHTS_BLOCKED", "素材不在当前权利有效的 Task Contract 中")
		}
	}
	if pkg.Channel != contract.Brief.Channel {
		add("/channel", "CHANNEL_MISMATCH", "剧本渠道必须与 Brief 一致")
	}
	if pkg.CreativeStrategy.PrimarySellingPoint != contract.Brief.PrimarySellingPoint {
		add("/creative_strategy/primary_selling_point", "SELLING_POINT_MISMATCH", "主卖点必须与 Brief 一致")
	}
	if pkg.CreativeStrategy.PrimaryTestVariable != contract.Brief.PrimaryTestVariable {
		add("/creative_strategy/primary_test_variable", "TEST_VARIABLE_MISMATCH", "主要测试变量必须与 Brief 一致")
	}
	allowedKnowledge := map[string]bool{}
	for _, item := range contract.Knowledge {
		if item.Status == "approved" {
			allowedKnowledge[item.ID] = true
		}
	}
	lastEnd := 0
	roles := map[string]bool{}
	shotIDs := map[string]bool{}
	allowedPlans := map[string]bool{}
	for _, id := range contract.Brief.VisualizationPlanIDs {
		allowedPlans[id] = true
	}
	for i, shot := range pkg.Shots {
		path := fmt.Sprintf("/shots/%d", i)
		if shot.StartMS != lastEnd || shot.EndMS <= shot.StartMS {
			add(path, "TIMECODE_INVALID", "镜头时码必须连续递增")
		}
		lastEnd = shot.EndMS
		roles[shot.Role] = true
		if shot.ShotID == "" || shotIDs[shot.ShotID] {
			add(path+"/shot_id", "SHOT_ID_INVALID", "镜头 ID 必须存在且版本内唯一")
		}
		shotIDs[shot.ShotID] = true
		if strings.TrimSpace(shot.FirstFrame.PromptZH) == "" || strings.TrimSpace(shot.MotionSpec) == "" || strings.TrimSpace(shot.EndFrame.PromptZH) == "" {
			add(path, "GENERATION_SPEC_REQUIRED", "镜头必须包含首帧、动态和尾帧中文生成规格")
		}
		if len(shot.NegativeConstraints) == 0 || strings.TrimSpace(shot.ProductTruthStrategy) == "" {
			add(path, "GENERATION_GUARD_REQUIRED", "镜头必须包含负面约束和产品真实性策略")
		}
		if shot.Role == "proof" && !allowedPlans[shot.VisualizationPlanID] {
			add(path+"/visualization_plan_id", "VISUALIZATION_PLAN_REQUIRED", "proof 镜头必须引用 Brief 内已批准可视化方案")
		}
		if strings.TrimSpace(shot.AcceptanceCriteriaString()) == "" {
			add(path+"/acceptance_criteria", "ACCEPTANCE_CRITERIA_REQUIRED", "镜头必须有可观察验收条件")
		}
		for _, ref := range shot.KnowledgeRefs {
			if !allowedKnowledge[ref] {
				add(path+"/knowledge_refs", "KNOWLEDGE_REF_NOT_ALLOWED", "镜头引用了快照外或未批准知识")
			}
		}
	}
	for i, citation := range pkg.Citations {
		if !allowedKnowledge[citation.KnowledgeID] {
			add(fmt.Sprintf("/citations/%d/knowledge_id", i), "CITATION_NOT_ALLOWED", "引用必须来自任务快照内已批准知识")
		}
		if !shotIDs[citation.ShotID] {
			add(fmt.Sprintf("/citations/%d/shot_id", i), "CITATION_SHOT_NOT_FOUND", "引用的镜头不存在")
		}
	}
	if pkg.Deliverability == "review_ready" {
		for _, role := range []string{"hook", "product_solution", "proof", "cta"} {
			if !roles[role] {
				add("/shots", "NARRATIVE_ROLE_MISSING", "缺少必要叙事功能: "+role)
			}
		}
		if lastEnd != pkg.TargetDurationSeconds*1000 {
			add("/target_duration_seconds", "DURATION_MISMATCH", "镜头总时长必须等于目标时长")
		}
	}
	return report
}

func (s Shot) AcceptanceCriteriaString() string {
	return strings.Join(s.AcceptanceCriteria, " ")
}
