package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

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
	Results   []StudioAssetItem       `json:"results"`
}

type CustomerAssetSurface struct {
	Workspace       WorkspaceMaterialProjection   `json:"workspace"`
	CreativeResults CreativeResultAssetProjection `json:"creative_results"`
	Recent          RecentAssetProjection         `json:"recent"`
	GeneratedAt     time.Time                     `json:"generated_at"`
}

type CreateWorkspaceFolderInput struct {
	ProjectID string `json:"project_id"`
	ParentRef string `json:"parent_ref,omitempty"`
	Name      string `json:"name"`
}

func (s *Service) CustomerStudioAssets(ctx context.Context, actor Actor, projectID string) (CustomerAssetSurface, error) {
	workspace, err := s.CustomerWorkspaceMaterials(ctx, actor, projectID)
	if err != nil {
		return CustomerAssetSurface{}, err
	}
	results, err := s.CustomerStudioCreativeResults(ctx, actor, projectID)
	if err != nil {
		return CustomerAssetSurface{}, err
	}
	recent := RecentAssetProjection{Materials: append([]WorkspaceMaterialItem{}, workspace.Materials...), Results: append([]StudioAssetItem{}, results.Items...)}
	sort.SliceStable(recent.Materials, func(i, j int) bool {
		return workspaceMaterialRecentTime(recent.Materials[i]).After(workspaceMaterialRecentTime(recent.Materials[j]))
	})
	if len(recent.Materials) > 8 {
		recent.Materials = recent.Materials[:8]
	}
	sort.SliceStable(recent.Results, func(i, j int) bool { return recent.Results[i].CreatedAt.After(recent.Results[j].CreatedAt) })
	if len(recent.Results) > 8 {
		recent.Results = recent.Results[:8]
	}
	return CustomerAssetSurface{Workspace: workspace, CreativeResults: results, Recent: recent, GeneratedAt: s.now().UTC()}, nil
}

func (s *Service) CustomerWorkspaceMaterials(ctx context.Context, actor Actor, projectID string) (WorkspaceMaterialProjection, error) {
	projects, err := s.Projects(ctx, actor)
	if err != nil {
		return WorkspaceMaterialProjection{}, err
	}
	projectNames := map[string]string{}
	for _, project := range projects {
		projectNames[project.ID] = project.BrandName
	}
	if projectID != "" {
		if _, ok := projectNames[projectID]; !ok {
			return WorkspaceMaterialProjection{}, domain.NotFound("项目")
		}
	}
	folders, err := s.store.WorkspaceFolders(ctx, actor.TenantID, projectID)
	if err != nil {
		return WorkspaceMaterialProjection{}, err
	}
	materials, err := s.store.WorkspaceMaterials(ctx, actor.TenantID, projectID)
	if err != nil {
		return WorkspaceMaterialProjection{}, err
	}
	childCounts := map[string]int{}
	for _, folder := range folders {
		if folder.ParentID != "" {
			childCounts[folder.ParentID]++
		}
	}
	result := WorkspaceMaterialProjection{Folders: make([]WorkspaceFolderItem, 0, len(folders)), Materials: make([]WorkspaceMaterialItem, 0, len(materials)), Counts: map[string]int{}, GeneratedAt: s.now().UTC()}
	for _, folder := range folders {
		result.Folders = append(result.Folders, WorkspaceFolderItem{Ref: "folder:" + folder.ID, ParentRef: optionalRef("folder:", folder.ParentID), ProjectID: folder.ProjectID, ProjectName: projectNames[folder.ProjectID], Name: folder.Name, ChildCount: childCounts[folder.ID], CreatedAt: folder.CreatedAt, UpdatedAt: folder.UpdatedAt})
	}
	for _, material := range materials {
		revision, revisionErr := s.store.SourceRevision(ctx, actor.TenantID, material.SourceRevisionID)
		if revisionErr != nil {
			return WorkspaceMaterialProjection{}, revisionErr
		}
		result.Materials = append(result.Materials, workspaceMaterialItem(material, revision, projectNames[material.ProjectID]))
		result.Counts[material.MaterialKind]++
	}
	result.Counts["all"] = len(result.Materials)
	result.Counts["folders"] = len(result.Folders)
	return result, nil
}

func (s *Service) CreateCustomerWorkspaceFolder(ctx context.Context, actor Actor, input CreateWorkspaceFolderInput, requestID string) (WorkspaceFolderItem, error) {
	if !canCreateStudioTask(actor.Role) {
		return WorkspaceFolderItem{}, domain.Policy("ROLE_DENIED", "当前账号不能创建文件夹", "联系团队管理员调整创作权限")
	}
	project, err := s.store.Project(ctx, actor.TenantID, strings.TrimSpace(input.ProjectID))
	if err != nil {
		return WorkspaceFolderItem{}, err
	}
	if project.Status == "archived" {
		return WorkspaceFolderItem{}, domain.Policy("PROJECT_ARCHIVED", "已归档项目不能管理资料", "由租户管理员恢复项目后再操作")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 120 {
		return WorkspaceFolderItem{}, domain.Invalid("WORKSPACE_FOLDER_NAME_INVALID", "文件夹名称必须为 1 至 120 个字符")
	}
	parentID := strings.TrimPrefix(strings.TrimSpace(input.ParentRef), "folder:")
	now := s.now().UTC()
	folder := domain.WorkspaceFolder{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, ParentID: parentID, Name: name, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateWorkspaceFolder(ctx, folder); err != nil {
		return WorkspaceFolderItem{}, err
	}
	s.audit(ctx, actor, project.ID, "workspace_folder.created", "workspace_folder", folder.ID, requestID, map[string]any{"name": folder.Name})
	return WorkspaceFolderItem{Ref: "folder:" + folder.ID, ParentRef: optionalRef("folder:", folder.ParentID), ProjectID: project.ID, ProjectName: project.BrandName, Name: folder.Name, CreatedAt: folder.CreatedAt, UpdatedAt: folder.UpdatedAt}, nil
}

func (s *Service) UploadCustomerWorkspaceMaterial(ctx context.Context, actor Actor, projectID, folderRef, title, fileName, mimeType string, data []byte, requestID string) (WorkspaceMaterialItem, error) {
	if !canCreateStudioTask(actor.Role) {
		return WorkspaceMaterialItem{}, domain.Policy("ROLE_DENIED", "当前账号不能上传工作区资料", "联系团队管理员调整创作权限")
	}
	project, err := s.store.Project(ctx, actor.TenantID, strings.TrimSpace(projectID))
	if err != nil {
		return WorkspaceMaterialItem{}, err
	}
	if project.Status == "archived" {
		return WorkspaceMaterialItem{}, domain.Policy("PROJECT_ARCHIVED", "已归档项目不能上传资料", "由租户管理员恢复项目后再操作")
	}
	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "." || fileName == "" {
		return WorkspaceMaterialItem{}, domain.Invalid("WORKSPACE_MATERIAL_FILE_REQUIRED", "必须上传一个有效文件")
	}
	mimeType = strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0]))
	if expected, ok := sourceExtensionMIME[strings.ToLower(filepath.Ext(fileName))]; !ok || !sourceMIMEMatches(expected, mimeType) || !allowedSourceMIME[mimeType] {
		return WorkspaceMaterialItem{}, domain.Invalid("WORKSPACE_MATERIAL_MIME_INVALID", "文件类型与扩展名不匹配，或暂不支持该类型")
	}
	if folderRef != "" {
		folderID := strings.TrimPrefix(strings.TrimSpace(folderRef), "folder:")
		folder, folderErr := s.store.WorkspaceFolder(ctx, actor.TenantID, folderID)
		if folderErr != nil {
			return WorkspaceMaterialItem{}, folderErr
		}
		if folder.ProjectID != project.ID {
			return WorkspaceMaterialItem{}, domain.Policy("WORKSPACE_FOLDER_PROJECT_MISMATCH", "文件夹不属于当前项目", "选择当前项目下的文件夹")
		}
		folderRef = folderID
	}
	name := strings.TrimSpace(title)
	if name == "" {
		name = strings.TrimSuffix(fileName, filepath.Ext(fileName))
	}
	revision, err := s.uploadSource(ctx, actor, project.ID, name, "workspace_material", fileName, mimeType, data, requestID)
	if err != nil {
		return WorkspaceMaterialItem{}, err
	}
	now := s.now().UTC()
	material := domain.WorkspaceMaterial{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, FolderID: folderRef, SourceID: revision.SourceID, SourceRevisionID: revision.ID, MaterialKind: workspaceMaterialKind(mimeType), Origin: domain.WorkspaceMaterialUploaded, Usage: domain.WorkspaceMaterialProjectMaterial, Title: name, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateWorkspaceMaterial(ctx, material); err != nil {
		return WorkspaceMaterialItem{}, err
	}
	s.audit(ctx, actor, project.ID, "workspace_material.uploaded", "workspace_material", material.ID, requestID, map[string]any{"source_revision_id": revision.ID, "mime": mimeType, "byte_size": len(data)})
	return workspaceMaterialItem(material, revision, project.BrandName), nil
}

func (s *Service) CustomerWorkspaceMaterialBytes(ctx context.Context, actor Actor, materialID string) (WorkspaceMaterialItem, []byte, error) {
	material, err := s.store.WorkspaceMaterial(ctx, actor.TenantID, strings.TrimSpace(materialID))
	if err != nil {
		return WorkspaceMaterialItem{}, nil, err
	}
	project, err := s.store.Project(ctx, actor.TenantID, material.ProjectID)
	if err != nil {
		return WorkspaceMaterialItem{}, nil, err
	}
	revision, err := s.store.SourceRevision(ctx, actor.TenantID, material.SourceRevisionID)
	if err != nil {
		return WorkspaceMaterialItem{}, nil, err
	}
	data, err := s.SourceRevisionBytes(ctx, revision)
	if err != nil {
		return WorkspaceMaterialItem{}, nil, err
	}
	return workspaceMaterialItem(material, revision, project.BrandName), data, nil
}

func (s *Service) AttachCustomerWorkspaceMaterials(ctx context.Context, actor Actor, taskID string, input StudioAttachMaterialsInput, requestID string) (StudioTaskView, error) {
	if !canCreateStudioTask(actor.Role) {
		return StudioTaskView{}, domain.Policy("ROLE_DENIED", "当前账号不能修改任务资料", "联系团队管理员调整创作权限")
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, strings.TrimSpace(taskID))
	if err != nil {
		return StudioTaskView{}, err
	}
	if task.Status == domain.TaskStatusDelivered || task.Status == domain.TaskStatusCancelled {
		return StudioTaskView{}, domain.Conflict("STUDIO_TASK_INPUT_CLOSED", "已完成或已取消的任务不能继续加入资料")
	}
	refs, err := s.resolveCustomerWorkspaceMaterialRefs(ctx, actor, task.ProjectID, input.MaterialRefs)
	if err != nil {
		return StudioTaskView{}, err
	}
	for _, ref := range refs {
		task.InputRefs = appendUnique(task.InputRefs, ref)
	}
	if err := s.markCustomerWorkspaceMaterialsUsed(ctx, actor, task.ProjectID, input.MaterialRefs); err != nil {
		return StudioTaskView{}, err
	}
	if task.Status == domain.TaskStatusNeedsInput && len(task.InputRefs) > 0 {
		task.Status = domain.TaskStatusReady
		task.NextAction = "开始第一个流程阶段"
	}
	task.UpdatedAt = s.now().UTC()
	if err := s.store.SaveWorkTask(ctx, task); err != nil {
		return StudioTaskView{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "studio.task_materials_attached", "task", task.ID, requestID, map[string]any{"material_count": len(refs)})
	return s.CustomerStudioTask(ctx, actor, task.ID)
}

func (s *Service) resolveCustomerWorkspaceMaterialRefs(ctx context.Context, actor Actor, projectID string, refs []string) ([]string, error) {
	result := []string{}
	for _, ref := range uniqueNonEmpty(refs) {
		materialID := strings.TrimPrefix(strings.TrimSpace(ref), "material:")
		material, err := s.store.WorkspaceMaterial(ctx, actor.TenantID, materialID)
		if err != nil {
			return nil, err
		}
		if material.ProjectID != projectID {
			return nil, domain.Policy("WORKSPACE_MATERIAL_PROJECT_MISMATCH", "所选资料不属于当前项目", "选择当前项目下的工作区资料")
		}
		revision, err := s.store.SourceRevision(ctx, actor.TenantID, material.SourceRevisionID)
		if err != nil {
			return nil, err
		}
		result = append(result, fmt.Sprintf("source_revision:%s@sha256:%s", revision.ID, revision.SHA256))
	}
	return result, nil
}

func (s *Service) markCustomerWorkspaceMaterialsUsed(ctx context.Context, actor Actor, projectID string, refs []string) error {
	for _, ref := range uniqueNonEmpty(refs) {
		material, err := s.store.WorkspaceMaterial(ctx, actor.TenantID, strings.TrimPrefix(ref, "material:"))
		if err != nil {
			return err
		}
		if material.ProjectID != projectID {
			return domain.Policy("WORKSPACE_MATERIAL_PROJECT_MISMATCH", "所选资料不属于当前项目", "选择当前项目下的工作区资料")
		}
		now := s.now().UTC()
		material.LastUsedAt = &now
		material.UpdatedAt = now
		if err := s.store.SaveWorkspaceMaterial(ctx, material); err != nil {
			return err
		}
	}
	return nil
}

func workspaceMaterialItem(material domain.WorkspaceMaterial, revision domain.SourceRevision, projectName string) WorkspaceMaterialItem {
	return WorkspaceMaterialItem{Ref: "material:" + material.ID, FolderRef: optionalRef("folder:", material.FolderID), ProjectID: material.ProjectID, ProjectName: projectName, MaterialKind: material.MaterialKind, Origin: material.Origin, Usage: material.Usage, Title: material.Title, FileName: revision.FileName, MIMEType: defaultString(revision.DetectedMIME, revision.DeclaredMIME), ByteSize: revision.ByteSize, PreviewRef: "material:" + material.ID, ProcessingState: workspaceMaterialProcessingState(revision.ProcessingStatus), RightsSummary: "未登记独立权利结论", CreatedAt: material.CreatedAt, UpdatedAt: material.UpdatedAt, LastUsedAt: material.LastUsedAt}
}

func workspaceMaterialKind(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return domain.WorkspaceMaterialImage
	case strings.HasPrefix(mimeType, "video/"):
		return domain.WorkspaceMaterialVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return domain.WorkspaceMaterialAudio
	case mimeType == "text/csv" || strings.Contains(mimeType, "spreadsheet"):
		return domain.WorkspaceMaterialTable
	case mimeType == "application/pdf" || strings.Contains(mimeType, "wordprocessing") || strings.Contains(mimeType, "presentation") || strings.HasPrefix(mimeType, "text/"):
		return domain.WorkspaceMaterialDocument
	default:
		return domain.WorkspaceMaterialOther
	}
}

func workspaceMaterialProcessingState(status string) string {
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

func workspaceMaterialRecentTime(item WorkspaceMaterialItem) time.Time {
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
