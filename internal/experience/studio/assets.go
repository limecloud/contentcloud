// Package studio owns customer-facing Studio projections and transport-neutral
// contracts. It must not depend on the legacy app, store, or HTTP packages.
package studio

import (
	"encoding/json"
	"time"
)

type Download struct {
	ID        string `json:"id"`
	FileName  string `json:"file_name"`
	MediaType string `json:"media_type"`
	ByteSize  int64  `json:"byte_size"`
	Href      string `json:"href"`
}

type CreativeResultItem struct {
	Ref           string     `json:"ref"`
	InputRef      string     `json:"-"`
	ResultType    string     `json:"result_type"`
	ProjectID     string     `json:"project_id"`
	ProjectName   string     `json:"project_name"`
	TaskID        string     `json:"task_id"`
	TaskTitle     string     `json:"task_title"`
	Title         string     `json:"title"`
	Summary       string     `json:"summary"`
	Version       string     `json:"version"`
	Status        string     `json:"status"`
	Reusable      bool       `json:"reusable"`
	BlockedReason string     `json:"blocked_reason,omitempty"`
	Downloads     []Download `json:"downloads"`
	CreatedAt     time.Time  `json:"created_at"`
}

type CreativeResultFact struct {
	Title      string `json:"title"`
	Statement  string `json:"statement"`
	ObjectType string `json:"object_type"`
	Layer      string `json:"layer"`
}

type CreativeResultMedia struct {
	AssetRef string   `json:"asset_ref"`
	ShotID   string   `json:"shot_id,omitempty"`
	Role     string   `json:"role,omitempty"`
	File     Download `json:"file"`
}

type CreativeResultDetail struct {
	Item          CreativeResultItem    `json:"item"`
	ContentFormat string                `json:"content_format"`
	Content       json.RawMessage       `json:"content"`
	Media         []CreativeResultMedia `json:"media"`
	ReadOnly      bool                  `json:"read_only"`
}

type CreativeResultProjection struct {
	Items       []CreativeResultItem `json:"items"`
	Counts      map[string]int       `json:"counts"`
	GeneratedAt time.Time            `json:"generated_at"`
}

type WorkspaceFolderItem struct {
	Ref         string    `json:"folder_ref"`
	ParentRef   string    `json:"parent_ref,omitempty"`
	ProjectID   string    `json:"project_id"`
	ProjectName string    `json:"project_name"`
	Name        string    `json:"name"`
	ChildCount  int       `json:"child_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WorkspaceMaterialItem struct {
	Ref             string     `json:"material_ref"`
	FolderRef       string     `json:"folder_ref,omitempty"`
	ProjectID       string     `json:"project_id"`
	ProjectName     string     `json:"project_name"`
	MaterialKind    string     `json:"material_kind"`
	Origin          string     `json:"origin"`
	Usage           string     `json:"usage"`
	Title           string     `json:"title"`
	FileName        string     `json:"file_name"`
	MIMEType        string     `json:"mime_type"`
	ByteSize        int64      `json:"byte_size"`
	PreviewRef      string     `json:"preview_ref,omitempty"`
	ProcessingState string     `json:"processing_state"`
	RightsSummary   string     `json:"rights_summary"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
}

type WorkspaceMaterialProjection struct {
	Folders     []WorkspaceFolderItem   `json:"folders"`
	Materials   []WorkspaceMaterialItem `json:"materials"`
	Counts      map[string]int          `json:"counts"`
	GeneratedAt time.Time               `json:"generated_at"`
}

type RecentAssetProjection struct {
	Materials []WorkspaceMaterialItem `json:"materials"`
	Results   []CreativeResultItem    `json:"results"`
}

type AssetSurface struct {
	Workspace       WorkspaceMaterialProjection `json:"workspace"`
	CreativeResults CreativeResultProjection    `json:"creative_results"`
	Recent          RecentAssetProjection       `json:"recent"`
	GeneratedAt     time.Time                   `json:"generated_at"`
}
