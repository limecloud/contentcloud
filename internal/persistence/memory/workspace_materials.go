package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func (s *Store) CreateWorkspaceFolder(_ context.Context, value workspacedomain.WorkspaceFolder) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value.ParentID != "" {
		parent, ok := s.workspaceFolders[value.ParentID]
		if !ok || parent.TenantID != value.TenantID || parent.ProjectID != value.ProjectID {
			return fault.NotFound("上级文件夹")
		}
	}
	s.workspaceFolders[value.ID] = value
	return nil
}

func (s *Store) WorkspaceFolders(_ context.Context, tenantID, projectID string) ([]workspacedomain.WorkspaceFolder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := []workspacedomain.WorkspaceFolder{}
	for _, value := range s.workspaceFolders {
		if value.TenantID == tenantID && (projectID == "" || value.ProjectID == projectID) {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *Store) WorkspaceFolder(_ context.Context, tenantID, id string) (workspacedomain.WorkspaceFolder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.workspaceFolders[id]
	if !ok || value.TenantID != tenantID {
		return value, fault.NotFound("文件夹")
	}
	return value, nil
}

func (s *Store) CreateWorkspaceMaterial(_ context.Context, value workspacedomain.WorkspaceMaterial) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value.FolderID != "" {
		folder, ok := s.workspaceFolders[value.FolderID]
		if !ok || folder.TenantID != value.TenantID || folder.ProjectID != value.ProjectID {
			return fault.NotFound("文件夹")
		}
	}
	s.workspaceMaterials[value.ID] = value
	return nil
}

func (s *Store) WorkspaceMaterials(_ context.Context, tenantID, projectID string) ([]workspacedomain.WorkspaceMaterial, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := []workspacedomain.WorkspaceMaterial{}
	for _, value := range s.workspaceMaterials {
		if value.TenantID == tenantID && (projectID == "" || value.ProjectID == projectID) {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

func (s *Store) WorkspaceMaterial(_ context.Context, tenantID, id string) (workspacedomain.WorkspaceMaterial, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.workspaceMaterials[id]
	if !ok || value.TenantID != tenantID {
		return value, fault.NotFound("工作区资料")
	}
	return value, nil
}

func (s *Store) SaveWorkspaceMaterial(_ context.Context, value workspacedomain.WorkspaceMaterial) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.workspaceMaterials[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return fault.NotFound("工作区资料")
	}
	s.workspaceMaterials[value.ID] = value
	return nil
}
