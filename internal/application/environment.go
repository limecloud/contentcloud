package application

import (
	"context"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/catalog/environment"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func (s *CatalogService) EnvironmentManifest(ctx context.Context, actor Actor, binding workspacedomain.WorkspaceBinding) (environment.Manifest, error) {
	if actor.Type != "workspace" || actor.WorkspaceID == "" || actor.WorkspaceID != binding.ID || binding.ProjectID == "" {
		return environment.Manifest{}, fault.Policy("ENVIRONMENT_WORKSPACE_DENIED", "只有当前项目的本地工作区凭据可以获取执行环境清单", "重新绑定 Content Work OS 本地工作区")
	}
	if s.environmentControl == nil {
		return environment.Manifest{}, fault.Conflict("ENVIRONMENT_CONTROL_PLANE_UNAVAILABLE", "执行环境控制服务尚未配置")
	}
	contentTypes, err := s.app.Identity.TenantContentTypes(ctx, actor.TenantID)
	if err != nil {
		return environment.Manifest{}, err
	}
	return s.environmentControl.Issue(binding.ProjectID, contentTypes, s.now().UTC())
}

func (s *CatalogService) EnvironmentRegistry(_ context.Context, actor Actor, binding workspacedomain.WorkspaceBinding) (environment.Registry, error) {
	if actor.Type != "workspace" || actor.WorkspaceID == "" || actor.WorkspaceID != binding.ID || binding.ProjectID == "" {
		return environment.Registry{}, fault.Policy("ENVIRONMENT_WORKSPACE_DENIED", "只有当前项目的本地工作区凭据可以获取执行环境注册表", "重新绑定 Content Work OS 本地工作区")
	}
	if s.environmentControl == nil {
		return environment.Registry{}, fault.Conflict("ENVIRONMENT_CONTROL_PLANE_UNAVAILABLE", "执行环境控制服务尚未配置")
	}
	return s.environmentControl.Registry()
}
