package performance

import "time"

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
