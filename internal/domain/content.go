package domain

import "time"

const (
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

type PerformanceObservation struct {
	ID                 string             `json:"id"`
	TenantID           string             `json:"tenant_id"`
	ProjectID          string             `json:"project_id"`
	ImportBatchID      string             `json:"import_batch_id"`
	RowNumber          int                `json:"row_number"`
	ApprovedSnapshotID string             `json:"approved_snapshot_id"`
	Platform           string             `json:"platform"`
	AccountAlias       string             `json:"account_alias"`
	PublishedAt        time.Time          `json:"published_at"`
	WindowHours        int                `json:"window_hours"`
	SampleStatus       string             `json:"sample_status"`
	Metrics            map[string]float64 `json:"metrics"`
	Currency           string             `json:"currency,omitempty"`
	Spend              float64            `json:"spend"`
	GMV                float64            `json:"gmv"`
	ROI                *float64           `json:"roi,omitempty"`
	DedupKey           string             `json:"dedup_key"`
	IssueCategory      string             `json:"issue_category"`
	Notes              string             `json:"notes"`
	CreatedAt          time.Time          `json:"created_at"`
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
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenant_id"`
	ProjectID          string         `json:"project_id"`
	ApprovedSnapshotID string         `json:"approved_snapshot_id"`
	Kind               string         `json:"kind"`
	CapabilityID       string         `json:"capability_id"`
	CapabilityVersion  string         `json:"capability_version"`
	CapabilityDigest   string         `json:"capability_digest"`
	SchemaID           string         `json:"schema_id"`
	MediaType          string         `json:"media_type"`
	FileName           string         `json:"file_name"`
	SHA256             string         `json:"sha256"`
	ByteSize           int64          `json:"byte_size"`
	ObjectKey          string         `json:"-"`
	Visibility         string         `json:"visibility"`
	RetentionClass     string         `json:"retention_class"`
	Purpose            string         `json:"purpose"`
	Metadata           map[string]any `json:"metadata"`
	CreatedAt          time.Time      `json:"created_at"`
}

type DeliveryPackage struct {
	ID                  string     `json:"id"`
	TenantID            string     `json:"tenant_id"`
	ProjectID           string     `json:"project_id"`
	ApprovedSnapshotIDs []string   `json:"approved_snapshot_ids"`
	ContentItemID       string     `json:"content_item_id"`
	Status              string     `json:"status"`
	Manifest            []Artifact `json:"manifest"`
	CreatedBy           string     `json:"created_by"`
	CreatedAt           time.Time  `json:"created_at"`
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
