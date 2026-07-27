package domain

import "time"

const ProjectProjectionSchemaVersion = "contentcloud.project-projection/3.0"

type ProjectProjection struct {
	SchemaVersion string                       `json:"schema_version"`
	Project       ProjectProjectionHeader      `json:"project"`
	Sections      map[string]ProjectionSection `json:"sections"`
	Governance    ProjectionGovernance         `json:"governance"`
	Submissions   []ProjectionSubmission       `json:"submissions"`
	Snapshots     []ProjectionSnapshot         `json:"snapshots"`
	NextActions   []ProjectionAction           `json:"next_actions"`
	GeneratedAt   time.Time                    `json:"generated_at"`
}

type ProjectProjectionHeader struct {
	ID             string `json:"id"`
	BrandName      string `json:"brand_name"`
	ProductName    string `json:"product_name"`
	Channel        string `json:"channel"`
	StageObjective string `json:"stage_objective"`
	Status         string `json:"status"`
}

type ProjectionSection struct {
	Status    string     `json:"status"`
	Count     int        `json:"count"`
	Pending   int        `json:"pending"`
	Blocked   int        `json:"blocked"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type ProjectionGovernance struct {
	PendingReviews        int `json:"pending_reviews"`
	ChangesRequested      int `json:"changes_requested"`
	BlockedContentBatches int `json:"blocked_content_batches"`
	ActiveAutomationRuns  int `json:"active_automation_runs"`
}

type ProjectionSubmission struct {
	ID                string    `json:"id"`
	Type              string    `json:"type"`
	Status            string    `json:"status"`
	CurrentRevisionID string    `json:"current_revision_id"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ProjectionSnapshot struct {
	ID                   string    `json:"id"`
	Type                 string    `json:"type"`
	SubmissionRevisionID string    `json:"submission_revision_id"`
	EligibleCount        int       `json:"eligible_count"`
	CreatedAt            time.Time `json:"created_at"`
}

type ProjectNavigationFocus struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Digest string `json:"digest,omitempty"`
}

type ProjectNavigation struct {
	View  string                  `json:"view"`
	Focus *ProjectNavigationFocus `json:"focus,omitempty"`
}

type ProjectionAction struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Label      string            `json:"label"`
	Enabled    bool              `json:"enabled"`
	Reason     string            `json:"reason,omitempty"`
	Navigation ProjectNavigation `json:"navigation"`
}
