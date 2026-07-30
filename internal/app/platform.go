package app

import (
	"context"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Service) PlatformOverview(ctx context.Context, actor Actor) (domain.PlatformOverview, error) {
	if !actor.PlatformAdmin {
		return domain.PlatformOverview{}, domain.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以访问系统后台", "联系系统管理员配置平台权限")
	}
	tenants, err := s.store.PlatformTenants(ctx)
	if err != nil {
		return domain.PlatformOverview{}, err
	}
	users, err := s.store.PlatformUsers(ctx)
	if err != nil {
		return domain.PlatformOverview{}, err
	}
	counts := domain.PlatformCounts{Tenants: len(tenants), Users: len(users)}
	for i := range tenants {
		if tenants[i].Status == "active" {
			counts.ActiveTenants++
		}
		counts.Projects += tenants[i].ProjectCount
		counts.OnlineDevices += tenants[i].DeviceCount
		counts.ActiveRuns += tenants[i].ActiveRunCount
	}
	for i := range users {
		_, users[i].IsPlatformAdmin = s.platformAdminEmails[strings.ToLower(users[i].Email)]
		if users[i].Memberships == nil {
			users[i].Memberships = []domain.PlatformUserMembership{}
		}
	}
	return domain.PlatformOverview{Counts: counts, Tenants: tenants, Users: users, GeneratedAt: s.now().UTC()}, nil
}

func (s *Service) UpdatePlatformTenantStatus(ctx context.Context, actor Actor, tenantID, status, requestID string) (domain.Tenant, error) {
	if !actor.PlatformAdmin {
		return domain.Tenant{}, domain.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以修改租户状态", "联系系统管理员配置平台权限")
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "active" && status != "suspended" {
		return domain.Tenant{}, domain.Invalid("TENANT_STATUS_INVALID", "租户状态只能是 active 或 suspended")
	}
	if tenantID == actor.TenantID && status != "active" {
		return domain.Tenant{}, domain.Policy("CURRENT_TENANT_REQUIRED", "不能停用当前管理会话所在租户", "先切换到其他有效租户")
	}
	tenant, err := s.store.SetTenantStatus(ctx, tenantID, status, s.now().UTC())
	if err != nil {
		return domain.Tenant{}, err
	}
	s.audit(ctx, actor, "", "platform.tenant_status_changed", "tenant", tenant.ID, requestID, map[string]any{"status": status, "tenant_name": tenant.Name})
	return tenant, nil
}

func (s *Service) UpdatePlatformTenantContentCapability(ctx context.Context, actor Actor, tenantID, contentType string, enabled bool, requestID string) (domain.PlatformTenant, error) {
	if !actor.PlatformAdmin {
		return domain.PlatformTenant{}, domain.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以修改租户内容能力", "联系系统管理员配置平台权限")
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if !domain.ValidOptionalTenantContentType(contentType) {
		return domain.PlatformTenant{}, domain.Invalid("TENANT_CONTENT_TYPE_INVALID", "内容类型不受支持或属于不可关闭的默认能力")
	}
	value := domain.TenantContentCapability{TenantID: tenantID, ContentType: contentType, Enabled: enabled, UpdatedBy: actor.UserID, UpdatedAt: s.now().UTC()}
	if err := s.store.SetTenantContentCapability(ctx, value); err != nil {
		return domain.PlatformTenant{}, err
	}
	s.audit(ctx, actor, "", "platform.tenant_content_capability_changed", "tenant", tenantID, requestID, map[string]any{"content_type": contentType, "enabled": enabled})
	tenants, err := s.store.PlatformTenants(ctx)
	if err != nil {
		return domain.PlatformTenant{}, err
	}
	for _, tenant := range tenants {
		if tenant.ID == tenantID {
			return tenant, nil
		}
	}
	return domain.PlatformTenant{}, domain.NotFound("租户")
}

func (s *Service) TenantContentTypes(ctx context.Context, tenantID string) ([]string, error) {
	values, err := s.store.TenantContentCapabilities(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return domain.EnabledTenantContentTypes(values), nil
}
