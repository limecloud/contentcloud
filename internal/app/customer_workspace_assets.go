package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
	experiencestudio "github.com/limecloud/contentcloud/internal/experience/studio"
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

func (s *Service) CustomerStudioAssets(ctx context.Context, actor Actor, projectID string) (CustomerAssetSurface, error) {
	workspace, err := s.CustomerWorkspaceMaterials(ctx, actor, projectID)
	if err != nil {
		return CustomerAssetSurface{}, err
	}
	results, err := s.CustomerStudioCreativeResults(ctx, actor, projectID)
	if err != nil {
		return CustomerAssetSurface{}, err
	}
	return experiencestudio.BuildAssetSurface(workspace, results, s.now()), nil
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
	queries := experiencestudio.NewAssetQueries(s.store, s.now)
	return queries.WorkspaceMaterials(ctx, actor.TenantID, projectID, projectNames)
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
	return experiencestudio.ProjectWorkspaceFolder(folder, project.BrandName), nil
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
	return experiencestudio.ProjectWorkspaceMaterial(material, revision, project.BrandName), nil
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
	return experiencestudio.ProjectWorkspaceMaterial(material, revision, project.BrandName), data, nil
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
