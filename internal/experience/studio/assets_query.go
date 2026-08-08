package studio

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

// WorkspaceAssetReader is the read-side port required to build the customer
// material projection. Storage implementations satisfy it structurally.
type WorkspaceAssetReader interface {
	WorkspaceFolders(context.Context, string, string) ([]domain.WorkspaceFolder, error)
	WorkspaceMaterials(context.Context, string, string) ([]domain.WorkspaceMaterial, error)
	SourceRevision(context.Context, string, string) (domain.SourceRevision, error)
}

type AssetQueries struct {
	workspace WorkspaceAssetReader
	now       func() time.Time
}

func NewAssetQueries(workspace WorkspaceAssetReader, now func() time.Time) AssetQueries {
	if now == nil {
		now = time.Now
	}
	return AssetQueries{workspace: workspace, now: now}
}

func (q AssetQueries) WorkspaceMaterials(ctx context.Context, tenantID, projectID string, projectNames map[string]string) (WorkspaceMaterialProjection, error) {
	folders, err := q.workspace.WorkspaceFolders(ctx, tenantID, projectID)
	if err != nil {
		return WorkspaceMaterialProjection{}, err
	}
	materials, err := q.workspace.WorkspaceMaterials(ctx, tenantID, projectID)
	if err != nil {
		return WorkspaceMaterialProjection{}, err
	}
	childCounts := map[string]int{}
	for _, folder := range folders {
		if folder.ParentID != "" {
			childCounts[folder.ParentID]++
		}
	}
	result := WorkspaceMaterialProjection{
		Folders:     make([]WorkspaceFolderItem, 0, len(folders)),
		Materials:   make([]WorkspaceMaterialItem, 0, len(materials)),
		Counts:      map[string]int{},
		GeneratedAt: q.now().UTC(),
	}
	for _, folder := range folders {
		item := ProjectWorkspaceFolder(folder, projectNames[folder.ProjectID])
		item.ChildCount = childCounts[folder.ID]
		result.Folders = append(result.Folders, item)
	}
	for _, material := range materials {
		revision, revisionErr := q.workspace.SourceRevision(ctx, tenantID, material.SourceRevisionID)
		if revisionErr != nil {
			return WorkspaceMaterialProjection{}, revisionErr
		}
		item := ProjectWorkspaceMaterial(material, revision, projectNames[material.ProjectID])
		result.Materials = append(result.Materials, item)
		result.Counts[item.MaterialKind]++
	}
	result.Counts["all"] = len(result.Materials)
	result.Counts["folders"] = len(result.Folders)
	return result, nil
}

func ProjectWorkspaceFolder(folder domain.WorkspaceFolder, projectName string) WorkspaceFolderItem {
	return WorkspaceFolderItem{
		Ref:         "folder:" + folder.ID,
		ParentRef:   optionalRef("folder:", folder.ParentID),
		ProjectID:   folder.ProjectID,
		ProjectName: projectName,
		Name:        folder.Name,
		CreatedAt:   folder.CreatedAt,
		UpdatedAt:   folder.UpdatedAt,
	}
}

func ProjectWorkspaceMaterial(material domain.WorkspaceMaterial, revision domain.SourceRevision, projectName string) WorkspaceMaterialItem {
	mimeType := strings.TrimSpace(revision.DetectedMIME)
	if mimeType == "" {
		mimeType = strings.TrimSpace(revision.DeclaredMIME)
	}
	return WorkspaceMaterialItem{
		Ref:             "material:" + material.ID,
		FolderRef:       optionalRef("folder:", material.FolderID),
		ProjectID:       material.ProjectID,
		ProjectName:     projectName,
		MaterialKind:    material.MaterialKind,
		Origin:          material.Origin,
		Usage:           material.Usage,
		Title:           material.Title,
		FileName:        revision.FileName,
		MIMEType:        mimeType,
		ByteSize:        revision.ByteSize,
		PreviewRef:      "material:" + material.ID,
		ProcessingState: materialProcessingState(revision.ProcessingStatus),
		RightsSummary:   "未登记独立权利结论",
		CreatedAt:       material.CreatedAt,
		UpdatedAt:       material.UpdatedAt,
		LastUsedAt:      material.LastUsedAt,
	}
}

func BuildAssetSurface(workspace WorkspaceMaterialProjection, results CreativeResultProjection, now time.Time) AssetSurface {
	recent := RecentAssetProjection{
		Materials: append([]WorkspaceMaterialItem{}, workspace.Materials...),
		Results:   append([]CreativeResultItem{}, results.Items...),
	}
	sort.SliceStable(recent.Materials, func(i, j int) bool {
		return materialRecentTime(recent.Materials[i]).After(materialRecentTime(recent.Materials[j]))
	})
	if len(recent.Materials) > 8 {
		recent.Materials = recent.Materials[:8]
	}
	sort.SliceStable(recent.Results, func(i, j int) bool {
		return recent.Results[i].CreatedAt.After(recent.Results[j].CreatedAt)
	})
	if len(recent.Results) > 8 {
		recent.Results = recent.Results[:8]
	}
	return AssetSurface{Workspace: workspace, CreativeResults: results, Recent: recent, GeneratedAt: now.UTC()}
}

func materialProcessingState(status string) string {
	switch status {
	case "uploading":
		return "uploading"
	case "ready":
		return "ready"
	case "failed":
		return "failed"
	default:
		return "processing"
	}
}

func materialRecentTime(item WorkspaceMaterialItem) time.Time {
	if item.LastUsedAt != nil {
		return *item.LastUsedAt
	}
	return item.UpdatedAt
}

func optionalRef(prefix, value string) string {
	if value == "" {
		return ""
	}
	return prefix + value
}
