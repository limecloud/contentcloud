package domain

import "time"

const (
	KnowledgeExtractCapability = "contentcloud.knowledge.extract"
	ArtifactExportCapability   = "contentcloud.artifact.export"
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
	RunID     string              `json:"run_id"`
	Items     []KnowledgeItem     `json:"items"`
	Conflicts []KnowledgeConflict `json:"conflicts"`
	Warnings  []string            `json:"warnings"`
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

type BenchmarkContent struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	ProjectID       string    `json:"project_id"`
	Title           string    `json:"title"`
	Platform        string    `json:"platform"`
	OriginalURL     string    `json:"original_url,omitempty"`
	RightsMode      string    `json:"rights_mode"`
	ValidationLevel string    `json:"validation_level"`
	ValidationNote  string    `json:"validation_note"`
	CreatedAt       time.Time `json:"created_at"`
}

type ContentFramework struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	BenchmarkID    string    `json:"benchmark_id"`
	Name           string    `json:"name"`
	VisualSequence []string  `json:"visual_sequence"`
	CopySequence   []string  `json:"copy_sequence"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type ShotPattern struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	ProjectID    string    `json:"project_id"`
	FrameworkID  string    `json:"framework_id"`
	Role         string    `json:"role"`
	Purpose      string    `json:"purpose"`
	Subject      string    `json:"subject"`
	Action       string    `json:"action"`
	ProofType    string    `json:"proof_type"`
	FailureModes []string  `json:"failure_modes"`
	CreatedAt    time.Time `json:"created_at"`
}

type SellingPoint struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	ProjectID    string    `json:"project_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Priority     int       `json:"priority"`
	KnowledgeIDs []string  `json:"knowledge_ids"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

type VisualizationPlan struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	ProjectID            string    `json:"project_id"`
	SellingPointID       string    `json:"selling_point_id"`
	Title                string    `json:"title"`
	ProofType            string    `json:"proof_type"`
	ShotPatternID        string    `json:"shot_pattern_id,omitempty"`
	Subjects             []string  `json:"subjects"`
	Setting              string    `json:"setting"`
	Props                []string  `json:"props"`
	Implementation       string    `json:"implementation"`
	ProductTruthStrategy string    `json:"product_truth_strategy"`
	Risks                []string  `json:"risks"`
	PlanB                string    `json:"plan_b"`
	AcceptanceCriteria   []string  `json:"acceptance_criteria"`
	Status               string    `json:"status"`
	CreatedAt            time.Time `json:"created_at"`
}

type PerformanceObservation struct {
	ID              string             `json:"id"`
	TenantID        string             `json:"tenant_id"`
	ProjectID       string             `json:"project_id"`
	ImportBatchID   string             `json:"import_batch_id"`
	RowNumber       int                `json:"row_number"`
	ScriptVersionID string             `json:"script_version_id"`
	Platform        string             `json:"platform"`
	AccountAlias    string             `json:"account_alias"`
	PublishedAt     time.Time          `json:"published_at"`
	WindowHours     int                `json:"window_hours"`
	SampleStatus    string             `json:"sample_status"`
	Metrics         map[string]float64 `json:"metrics"`
	Currency        string             `json:"currency,omitempty"`
	Spend           float64            `json:"spend"`
	GMV             float64            `json:"gmv"`
	ROI             *float64           `json:"roi,omitempty"`
	DedupKey        string             `json:"dedup_key"`
	IssueCategory   string             `json:"issue_category"`
	Notes           string             `json:"notes"`
	CreatedAt       time.Time          `json:"created_at"`
}

type PerformanceImportRowError struct {
	RowNumber int    `json:"row_number"`
	Field     string `json:"field,omitempty"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type PerformanceImportBatch struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	ProjectID     string    `json:"project_id"`
	SourceName    string    `json:"source_name"`
	SourceFormat  string    `json:"source_format"`
	SourceSHA256  string    `json:"source_sha256"`
	Currency      string    `json:"currency,omitempty"`
	RowCount      int       `json:"row_count"`
	ImportedCount int       `json:"imported_count"`
	Status        string    `json:"status"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

type PerformanceImportDetails struct {
	Batch        PerformanceImportBatch   `json:"batch"`
	Observations []PerformanceObservation `json:"observations"`
}

type RatingDecision struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	SubjectType    string    `json:"subject_type"`
	SubjectID      string    `json:"subject_id"`
	ObservationIDs []string  `json:"observation_ids"`
	Rating         string    `json:"rating"`
	Reason         string    `json:"reason"`
	NextAction     string    `json:"next_action"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

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

type Artifact struct {
	ID                    string                       `json:"id"`
	TenantID              string                       `json:"tenant_id"`
	ProjectID             string                       `json:"project_id"`
	ScriptVersionID       string                       `json:"script_version_id"`
	Kind                  string                       `json:"kind"`
	CapabilityID          string                       `json:"capability_id"`
	CapabilityVersion     string                       `json:"capability_version"`
	CapabilityDigest      string                       `json:"capability_digest"`
	SchemaID              string                       `json:"schema_id"`
	MediaType             string                       `json:"media_type"`
	FileName              string                       `json:"file_name"`
	SHA256                string                       `json:"sha256"`
	ByteSize              int64                        `json:"byte_size"`
	ObjectKey             string                       `json:"-"`
	Visibility            string                       `json:"visibility"`
	RetentionClass        string                       `json:"retention_class"`
	DerivedFromArtifactID string                       `json:"derived_from_artifact_id,omitempty"`
	Purpose               string                       `json:"purpose"`
	SourceDeviceID        string                       `json:"source_device_id,omitempty"`
	ValidationStatus      string                       `json:"validation_status"`
	ValidationError       string                       `json:"validation_error,omitempty"`
	Envelope              *ExtensionArtifactEnvelopeV1 `json:"envelope,omitempty"`
	PresentationTier      string                       `json:"presentation_tier"`
	Metadata              map[string]any               `json:"metadata"`
	CreatedAt             time.Time                    `json:"created_at"`
}

type ArtifactCapabilityRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

type ArtifactRef struct {
	SHA256    string `json:"sha256"`
	MediaType string `json:"media_type"`
	Size      int64  `json:"size"`
}

type ArtifactRenditionRef struct {
	Purpose  string      `json:"purpose"`
	Artifact ArtifactRef `json:"artifact"`
}

type ExtensionArtifactEnvelopeV1 struct {
	EnvelopeVersion  string                 `json:"envelope_version"`
	ProjectID        string                 `json:"project_id"`
	ScriptVersionID  string                 `json:"script_version_id"`
	Capability       ArtifactCapabilityRef  `json:"capability"`
	SchemaID         string                 `json:"schema_id"`
	MediaType        string                 `json:"media_type"`
	SHA256           string                 `json:"sha256"`
	Size             int64                  `json:"size"`
	ReviewProjection *ArtifactRef           `json:"review_projection,omitempty"`
	Renditions       []ArtifactRenditionRef `json:"renditions"`
	Metadata         map[string]any         `json:"metadata"`
}

type ReviewProjectionSectionV1 struct {
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	Summary         string   `json:"summary"`
	ScriptPointer   string   `json:"script_pointer,omitempty"`
	ThumbnailSHA256 string   `json:"thumbnail_sha256,omitempty"`
	Warnings        []string `json:"warnings"`
}

type ArtifactReviewProjectionV1 struct {
	SchemaVersion   string                      `json:"schema_version"`
	Title           string                      `json:"title"`
	Summary         string                      `json:"summary"`
	ScriptVersionID string                      `json:"script_version_id"`
	Sections        []ReviewProjectionSectionV1 `json:"sections"`
}

type ArtifactSourceDevice struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Online      bool   `json:"online"`
}

type ArtifactPresentation struct {
	Artifact         Artifact              `json:"artifact"`
	Tier             string                `json:"tier"`
	ReviewProjection *Artifact             `json:"review_projection,omitempty"`
	Renditions       []Artifact            `json:"renditions"`
	Actions          []string              `json:"actions"`
	SourceDevice     *ArtifactSourceDevice `json:"source_device,omitempty"`
}

type ArtifactOpenRequest struct {
	ID          string     `json:"open_request_id"`
	TenantID    string     `json:"tenant_id"`
	ProjectID   string     `json:"project_id"`
	ArtifactID  string     `json:"artifact_id"`
	DeviceID    string     `json:"device_id"`
	RequestedBy string     `json:"requested_by"`
	State       string     `json:"state"`
	Reason      string     `json:"reason,omitempty"`
	ExpiresAt   time.Time  `json:"expires_at"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type ArtifactOpenLease struct {
	OpenRequestID string `json:"open_request_id"`
	ArtifactID    string `json:"artifact_id"`
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

type UserDeviceFlow struct {
	ID             string     `json:"id"`
	DeviceCodeHash string     `json:"-"`
	UserCode       string     `json:"user_code"`
	UserID         string     `json:"user_id,omitempty"`
	TenantID       string     `json:"tenant_id,omitempty"`
	State          string     `json:"state"`
	ExpiresAt      time.Time  `json:"expires_at"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
	ConsumedAt     *time.Time `json:"consumed_at,omitempty"`
}

type CLIToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	TenantID  string     `json:"tenant_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
