package application

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	experiencestudio "github.com/limecloud/contentcloud/internal/experience/studio"
	"github.com/limecloud/contentcloud/internal/persistence"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type WorkspaceFolderItem = experiencestudio.WorkspaceFolderItem
type WorkspaceMaterialItem = experiencestudio.WorkspaceMaterialItem
type WorkspaceMaterialProjection = experiencestudio.WorkspaceMaterialProjection
type RecentAssetProjection = experiencestudio.RecentAssetProjection
type CustomerAssetSurface = experiencestudio.AssetSurface

type CreateWorkspaceFolderInput struct {
	ProjectID string `json:"project_id"`
	ParentRef string `json:"parent_ref,omitempty"`
	Name      string `json:"name"`
}

func (s *SourceService) CustomerStudioAssets(ctx context.Context, actor Actor, projectID string) (CustomerAssetSurface, error) {
	workspace, err := s.CustomerWorkspaceMaterials(ctx, actor, projectID)
	if err != nil {
		return CustomerAssetSurface{}, err
	}
	results, err := s.app.Work.CustomerStudioCreativeResults(ctx, actor, projectID)
	if err != nil {
		return CustomerAssetSurface{}, err
	}
	return experiencestudio.BuildAssetSurface(workspace, results, s.now()), nil
}

func (s *SourceService) CustomerWorkspaceMaterials(ctx context.Context, actor Actor, projectID string) (WorkspaceMaterialProjection, error) {
	projects, err := s.app.Workspace.Projects(ctx, actor)
	if err != nil {
		return WorkspaceMaterialProjection{}, err
	}
	projectNames := map[string]string{}
	for _, project := range projects {
		projectNames[project.ID] = project.BrandName
	}
	if projectID != "" {
		if _, ok := projectNames[projectID]; !ok {
			return WorkspaceMaterialProjection{}, fault.NotFound("项目")
		}
	}
	queries := experiencestudio.NewAssetQueries(workspaceAssetReader{workspace: s.workspace, source: s.source}, s.now)
	return queries.WorkspaceMaterials(ctx, actor.TenantID, projectID, projectNames)
}

type workspaceAssetReader struct {
	workspace persistence.WorkspaceRepository
	source    persistence.SourceRepository
}

func (r workspaceAssetReader) WorkspaceFolders(ctx context.Context, tenantID, projectID string) ([]experiencestudio.WorkspaceFolderRecord, error) {
	values, err := r.workspace.WorkspaceFolders(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	result := make([]experiencestudio.WorkspaceFolderRecord, 0, len(values))
	for _, value := range values {
		result = append(result, studioWorkspaceFolderRecord(value))
	}
	return result, nil
}

func (r workspaceAssetReader) WorkspaceMaterials(ctx context.Context, tenantID, projectID string) ([]experiencestudio.WorkspaceMaterialRecord, error) {
	values, err := r.workspace.WorkspaceMaterials(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	result := make([]experiencestudio.WorkspaceMaterialRecord, 0, len(values))
	for _, value := range values {
		result = append(result, studioWorkspaceMaterialRecord(value))
	}
	return result, nil
}

func (r workspaceAssetReader) SourceRevision(ctx context.Context, tenantID, revisionID string) (experiencestudio.SourceRevisionRecord, error) {
	value, err := r.source.SourceRevision(ctx, tenantID, revisionID)
	if err != nil {
		return experiencestudio.SourceRevisionRecord{}, err
	}
	return studioSourceRevisionRecord(value), nil
}

func studioWorkspaceFolderRecord(value workspacedomain.WorkspaceFolder) experiencestudio.WorkspaceFolderRecord {
	return experiencestudio.WorkspaceFolderRecord{ID: value.ID, ParentID: value.ParentID, ProjectID: value.ProjectID, Name: value.Name, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func studioWorkspaceMaterialRecord(value workspacedomain.WorkspaceMaterial) experiencestudio.WorkspaceMaterialRecord {
	return experiencestudio.WorkspaceMaterialRecord{ID: value.ID, FolderID: value.FolderID, ProjectID: value.ProjectID, SourceRevisionID: value.SourceRevisionID, MaterialKind: value.MaterialKind, Origin: value.Origin, Usage: value.Usage, Title: value.Title, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, LastUsedAt: value.LastUsedAt}
}

func studioSourceRevisionRecord(value sourcedomain.SourceRevision) experiencestudio.SourceRevisionRecord {
	return experiencestudio.SourceRevisionRecord{FileName: value.FileName, DeclaredMIME: value.DeclaredMIME, DetectedMIME: value.DetectedMIME, ByteSize: value.ByteSize, ProcessingStatus: value.ProcessingStatus}
}

func (s *SourceService) CreateCustomerWorkspaceFolder(ctx context.Context, actor Actor, input CreateWorkspaceFolderInput, requestID string) (WorkspaceFolderItem, error) {
	if !canCreateStudioTask(actor.Role) {
		return WorkspaceFolderItem{}, fault.Policy("ROLE_DENIED", "当前账号不能创建文件夹", "联系团队管理员调整创作权限")
	}
	project, err := s.workspace.Project(ctx, actor.TenantID, strings.TrimSpace(input.ProjectID))
	if err != nil {
		return WorkspaceFolderItem{}, err
	}
	if project.Status == "archived" {
		return WorkspaceFolderItem{}, fault.Policy("PROJECT_ARCHIVED", "已归档项目不能管理资料", "由租户管理员恢复项目后再操作")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 120 {
		return WorkspaceFolderItem{}, fault.Invalid("WORKSPACE_FOLDER_NAME_INVALID", "文件夹名称必须为 1 至 120 个字符")
	}
	parentID := strings.TrimPrefix(strings.TrimSpace(input.ParentRef), "folder:")
	now := s.now().UTC()
	folder := workspacedomain.WorkspaceFolder{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: project.ID, ParentID: parentID, Name: name, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.workspace.CreateWorkspaceFolder(ctx, folder); err != nil {
		return WorkspaceFolderItem{}, err
	}
	s.audit(ctx, actor, project.ID, "workspace_folder.created", "workspace_folder", folder.ID, requestID, map[string]any{"name": folder.Name})
	return experiencestudio.ProjectWorkspaceFolder(studioWorkspaceFolderRecord(folder), project.BrandName), nil
}

func (s *SourceService) UploadCustomerWorkspaceMaterial(ctx context.Context, actor Actor, projectID, folderRef, title, fileName, mimeType string, data []byte, requestID string) (WorkspaceMaterialItem, error) {
	if !canCreateStudioTask(actor.Role) {
		return WorkspaceMaterialItem{}, fault.Policy("ROLE_DENIED", "当前账号不能上传工作区资料", "联系团队管理员调整创作权限")
	}
	project, err := s.workspace.Project(ctx, actor.TenantID, strings.TrimSpace(projectID))
	if err != nil {
		return WorkspaceMaterialItem{}, err
	}
	if project.Status == "archived" {
		return WorkspaceMaterialItem{}, fault.Policy("PROJECT_ARCHIVED", "已归档项目不能上传资料", "由租户管理员恢复项目后再操作")
	}
	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "." || fileName == "" {
		return WorkspaceMaterialItem{}, fault.Invalid("WORKSPACE_MATERIAL_FILE_REQUIRED", "必须上传一个有效文件")
	}
	mimeType = strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0]))
	if expected, ok := sourceExtensionMIME[strings.ToLower(filepath.Ext(fileName))]; !ok || !sourceMIMEMatches(expected, mimeType) || !allowedSourceMIME[mimeType] {
		return WorkspaceMaterialItem{}, fault.Invalid("WORKSPACE_MATERIAL_MIME_INVALID", "文件类型与扩展名不匹配，或暂不支持该类型")
	}
	if folderRef != "" {
		folderID := strings.TrimPrefix(strings.TrimSpace(folderRef), "folder:")
		folder, folderErr := s.workspace.WorkspaceFolder(ctx, actor.TenantID, folderID)
		if folderErr != nil {
			return WorkspaceMaterialItem{}, folderErr
		}
		if folder.ProjectID != project.ID {
			return WorkspaceMaterialItem{}, fault.Policy("WORKSPACE_FOLDER_PROJECT_MISMATCH", "文件夹不属于当前项目", "选择当前项目下的文件夹")
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
	material := workspacedomain.WorkspaceMaterial{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: project.ID, FolderID: folderRef, SourceID: revision.SourceID, SourceRevisionID: revision.ID, MaterialKind: workspaceMaterialKind(mimeType), Origin: workspacedomain.WorkspaceMaterialUploaded, Usage: workspacedomain.WorkspaceMaterialProjectMaterial, Title: name, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.workspace.CreateWorkspaceMaterial(ctx, material); err != nil {
		return WorkspaceMaterialItem{}, err
	}
	s.audit(ctx, actor, project.ID, "workspace_material.uploaded", "workspace_material", material.ID, requestID, map[string]any{"source_revision_id": revision.ID, "mime": mimeType, "byte_size": len(data)})
	return experiencestudio.ProjectWorkspaceMaterial(studioWorkspaceMaterialRecord(material), studioSourceRevisionRecord(revision), project.BrandName), nil
}

func (s *SourceService) CustomerWorkspaceMaterialBytes(ctx context.Context, actor Actor, materialID string) (WorkspaceMaterialItem, []byte, error) {
	material, err := s.workspace.WorkspaceMaterial(ctx, actor.TenantID, strings.TrimSpace(materialID))
	if err != nil {
		return WorkspaceMaterialItem{}, nil, err
	}
	project, err := s.workspace.Project(ctx, actor.TenantID, material.ProjectID)
	if err != nil {
		return WorkspaceMaterialItem{}, nil, err
	}
	revision, err := s.source.SourceRevision(ctx, actor.TenantID, material.SourceRevisionID)
	if err != nil {
		return WorkspaceMaterialItem{}, nil, err
	}
	data, err := s.SourceRevisionBytes(ctx, revision)
	if err != nil {
		return WorkspaceMaterialItem{}, nil, err
	}
	return experiencestudio.ProjectWorkspaceMaterial(studioWorkspaceMaterialRecord(material), studioSourceRevisionRecord(revision), project.BrandName), data, nil
}

func (s *SourceService) AttachCustomerWorkspaceMaterials(ctx context.Context, actor Actor, taskID string, input StudioAttachMaterialsInput, requestID string) (StudioTaskView, error) {
	if !canCreateStudioTask(actor.Role) {
		return StudioTaskView{}, fault.Policy("ROLE_DENIED", "当前账号不能修改任务资料", "联系团队管理员调整创作权限")
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, strings.TrimSpace(taskID))
	if err != nil {
		return StudioTaskView{}, err
	}
	if task.Status == work.TaskStatusDelivered || task.Status == work.TaskStatusCancelled {
		return StudioTaskView{}, fault.Conflict("STUDIO_TASK_INPUT_CLOSED", "已完成或已取消的任务不能继续加入资料")
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
	if task.Status == work.TaskStatusNeedsInput && len(task.InputRefs) > 0 {
		task.Status = work.TaskStatusReady
		task.NextAction = "开始第一个流程阶段"
	}
	task.UpdatedAt = s.now().UTC()
	if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
		return StudioTaskView{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "studio.task_materials_attached", "task", task.ID, requestID, map[string]any{"material_count": len(refs)})
	return s.app.Work.CustomerStudioTask(ctx, actor, task.ID)
}

func (s *SourceService) resolveCustomerWorkspaceMaterialRefs(ctx context.Context, actor Actor, projectID string, refs []string) ([]string, error) {
	result := []string{}
	for _, ref := range uniqueNonEmpty(refs) {
		materialID := strings.TrimPrefix(strings.TrimSpace(ref), "material:")
		material, err := s.workspace.WorkspaceMaterial(ctx, actor.TenantID, materialID)
		if err != nil {
			return nil, err
		}
		if material.ProjectID != projectID {
			return nil, fault.Policy("WORKSPACE_MATERIAL_PROJECT_MISMATCH", "所选资料不属于当前项目", "选择当前项目下的工作区资料")
		}
		revision, err := s.source.SourceRevision(ctx, actor.TenantID, material.SourceRevisionID)
		if err != nil {
			return nil, err
		}
		result = append(result, fmt.Sprintf("source_revision:%s@sha256:%s", revision.ID, revision.SHA256))
	}
	return result, nil
}

func (s *SourceService) markCustomerWorkspaceMaterialsUsed(ctx context.Context, actor Actor, projectID string, refs []string) error {
	for _, ref := range uniqueNonEmpty(refs) {
		material, err := s.workspace.WorkspaceMaterial(ctx, actor.TenantID, strings.TrimPrefix(ref, "material:"))
		if err != nil {
			return err
		}
		if material.ProjectID != projectID {
			return fault.Policy("WORKSPACE_MATERIAL_PROJECT_MISMATCH", "所选资料不属于当前项目", "选择当前项目下的工作区资料")
		}
		now := s.now().UTC()
		material.LastUsedAt = &now
		material.UpdatedAt = now
		if err := s.workspace.SaveWorkspaceMaterial(ctx, material); err != nil {
			return err
		}
	}
	return nil
}

func workspaceMaterialKind(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return workspacedomain.WorkspaceMaterialImage
	case strings.HasPrefix(mimeType, "video/"):
		return workspacedomain.WorkspaceMaterialVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return workspacedomain.WorkspaceMaterialAudio
	case mimeType == "text/csv" || strings.Contains(mimeType, "spreadsheet"):
		return workspacedomain.WorkspaceMaterialTable
	case mimeType == "application/pdf" || strings.Contains(mimeType, "wordprocessing") || strings.Contains(mimeType, "presentation") || strings.HasPrefix(mimeType, "text/"):
		return workspacedomain.WorkspaceMaterialDocument
	default:
		return workspacedomain.WorkspaceMaterialOther
	}
}
