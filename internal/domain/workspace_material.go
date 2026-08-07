package domain

import "time"

const (
	WorkspaceMaterialDocument = "document"
	WorkspaceMaterialImage    = "image"
	WorkspaceMaterialVideo    = "video"
	WorkspaceMaterialAudio    = "audio"
	WorkspaceMaterialTable    = "table"
	WorkspaceMaterialOther    = "other"

	WorkspaceMaterialUploaded = "uploaded"
	WorkspaceMaterialImported = "imported"
	WorkspaceMaterialLinked   = "linked"

	WorkspaceMaterialProjectMaterial  = "project_material"
	WorkspaceMaterialProjectReference = "project_reference"
)

// WorkspaceFolder owns customer organization only; file content stays in SourceRevision.
type WorkspaceFolder struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	ProjectID string    `json:"project_id"`
	ParentID  string    `json:"parent_id,omitempty"`
	Name      string    `json:"name"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkspaceMaterial binds a customer-owned workspace entry to one fixed source revision.
type WorkspaceMaterial struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	ProjectID        string     `json:"project_id"`
	FolderID         string     `json:"folder_id,omitempty"`
	SourceID         string     `json:"source_id"`
	SourceRevisionID string     `json:"source_revision_id"`
	MaterialKind     string     `json:"material_kind"`
	Origin           string     `json:"origin"`
	Usage            string     `json:"usage"`
	Title            string     `json:"title"`
	CreatedBy        string     `json:"created_by"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
