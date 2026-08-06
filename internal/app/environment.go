package app

import (
	"context"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
)

func (s *Service) EnvironmentManifest(ctx context.Context, actor Actor, binding domain.WorkspaceBinding) (environment.Manifest, error) {
	if actor.Type != "workspace" || actor.WorkspaceID == "" || actor.WorkspaceID != binding.ID || binding.ProjectID == "" {
		return environment.Manifest{}, domain.Policy("ENVIRONMENT_WORKSPACE_DENIED", "只有当前项目的本地工作区凭据可以获取执行环境清单", "重新绑定 Content Work OS 本地工作区")
	}
	if s.environmentControl == nil {
		return environment.Manifest{}, domain.Conflict("ENVIRONMENT_CONTROL_PLANE_UNAVAILABLE", "执行环境控制服务尚未配置")
	}
	contentTypes, err := s.TenantContentTypes(ctx, actor.TenantID)
	if err != nil {
		return environment.Manifest{}, err
	}
	return s.environmentControl.Issue(binding.ProjectID, contentTypes, s.now().UTC())
}

func (s *Service) EnvironmentRegistry(_ context.Context, actor Actor, binding domain.WorkspaceBinding) (environment.Registry, error) {
	if actor.Type != "workspace" || actor.WorkspaceID == "" || actor.WorkspaceID != binding.ID || binding.ProjectID == "" {
		return environment.Registry{}, domain.Policy("ENVIRONMENT_WORKSPACE_DENIED", "只有当前项目的本地工作区凭据可以获取执行环境注册表", "重新绑定 Content Work OS 本地工作区")
	}
	if s.environmentControl == nil {
		return environment.Registry{}, domain.Conflict("ENVIRONMENT_CONTROL_PLANE_UNAVAILABLE", "执行环境控制服务尚未配置")
	}
	return s.environmentControl.Registry()
}
