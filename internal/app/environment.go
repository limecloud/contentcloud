package app

import (
	"context"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
)

func (s *Service) EnvironmentManifest(_ context.Context, actor Actor, binding domain.WorkspaceBinding) (environment.Manifest, error) {
	if actor.Type != "workspace" || actor.WorkspaceID == "" || actor.WorkspaceID != binding.ID || binding.ProjectID == "" {
		return environment.Manifest{}, domain.Policy("ENVIRONMENT_WORKSPACE_DENIED", "只有当前项目的 Workspace Credential 可以获取 Environment Manifest", "重新绑定 ContentCloud Workspace")
	}
	if s.environmentControl == nil {
		return environment.Manifest{}, domain.Conflict("ENVIRONMENT_CONTROL_PLANE_UNAVAILABLE", "Environment Control Plane 尚未配置")
	}
	return s.environmentControl.Issue(binding.ProjectID, s.now().UTC())
}

func (s *Service) EnvironmentRegistry(_ context.Context, actor Actor, binding domain.WorkspaceBinding) (environment.Registry, error) {
	if actor.Type != "workspace" || actor.WorkspaceID == "" || actor.WorkspaceID != binding.ID || binding.ProjectID == "" {
		return environment.Registry{}, domain.Policy("ENVIRONMENT_WORKSPACE_DENIED", "只有当前项目的 Workspace Credential 可以获取 Environment Registry", "重新绑定 ContentCloud Workspace")
	}
	if s.environmentControl == nil {
		return environment.Registry{}, domain.Conflict("ENVIRONMENT_CONTROL_PLANE_UNAVAILABLE", "Environment Control Plane 尚未配置")
	}
	return s.environmentControl.Registry()
}
