package application

import (
	"context"
	"strings"

	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

func (s *IdentityService) PlatformOverview(ctx context.Context, actor Actor) (identitydomain.PlatformOverview, error) {
	if !actor.PlatformAdmin {
		return identitydomain.PlatformOverview{}, fault.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以访问系统后台", "联系系统管理员配置平台权限")
	}
	tenants, err := s.identity.PlatformTenants(ctx)
	if err != nil {
		return identitydomain.PlatformOverview{}, err
	}
	users, err := s.identity.PlatformUsers(ctx)
	if err != nil {
		return identitydomain.PlatformOverview{}, err
	}
	counts := identitydomain.PlatformCounts{Tenants: len(tenants), Users: len(users)}
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
			users[i].Memberships = []identitydomain.PlatformUserMembership{}
		}
	}
	return identitydomain.PlatformOverview{Counts: counts, Tenants: tenants, Users: users, GeneratedAt: s.now().UTC()}, nil
}

func (s *IdentityService) UpdatePlatformTenantStatus(ctx context.Context, actor Actor, tenantID, status, requestID string) (identitydomain.Tenant, error) {
	if !actor.PlatformAdmin {
		return identitydomain.Tenant{}, fault.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以修改租户状态", "联系系统管理员配置平台权限")
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "active" && status != "suspended" {
		return identitydomain.Tenant{}, fault.Invalid("TENANT_STATUS_INVALID", "租户状态只能是运行中（active）或已停用（suspended）")
	}
	if tenantID == actor.TenantID && status != "active" {
		return identitydomain.Tenant{}, fault.Policy("CURRENT_TENANT_REQUIRED", "不能停用当前管理会话所在租户", "先切换到其他有效租户")
	}
	tenant, err := s.identity.SetTenantStatus(ctx, tenantID, status, s.now().UTC())
	if err != nil {
		return identitydomain.Tenant{}, err
	}
	s.audit(ctx, actor, "", "platform.tenant_status_changed", "tenant", tenant.ID, requestID, map[string]any{"status": status, "tenant_name": tenant.Name})
	return tenant, nil
}

func (s *IdentityService) UpdatePlatformTenantContentCapability(ctx context.Context, actor Actor, tenantID, contentType string, enabled bool, requestID string) (identitydomain.PlatformTenant, error) {
	if !actor.PlatformAdmin {
		return identitydomain.PlatformTenant{}, fault.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以修改租户内容能力", "联系系统管理员配置平台权限")
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if !identitydomain.ValidOptionalTenantContentType(contentType) {
		return identitydomain.PlatformTenant{}, fault.Invalid("TENANT_CONTENT_TYPE_INVALID", "内容类型不受支持或属于不可关闭的默认能力")
	}
	value := identitydomain.TenantContentCapability{TenantID: tenantID, ContentType: contentType, Enabled: enabled, UpdatedBy: actor.UserID, UpdatedAt: s.now().UTC()}
	if err := s.identity.SetTenantContentCapability(ctx, value); err != nil {
		return identitydomain.PlatformTenant{}, err
	}
	s.audit(ctx, actor, "", "platform.tenant_content_capability_changed", "tenant", tenantID, requestID, map[string]any{"content_type": contentType, "enabled": enabled})
	tenants, err := s.identity.PlatformTenants(ctx)
	if err != nil {
		return identitydomain.PlatformTenant{}, err
	}
	for _, tenant := range tenants {
		if tenant.ID == tenantID {
			return tenant, nil
		}
	}
	return identitydomain.PlatformTenant{}, fault.NotFound("租户")
}

func (s *IdentityService) TenantContentTypes(ctx context.Context, tenantID string) ([]string, error) {
	values, err := s.identity.TenantContentCapabilities(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return identitydomain.EnabledTenantContentTypes(values), nil
}
