package app_test

import (
	"log/slog"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestPlatformOverviewAndTenantLifecycle(t *testing.T) {
	store := memory.New()
	service := app.New(store, slog.Default(), app.WithPlatformAdminEmails("platform@example.com"))
	adminSession, err := service.Register(t.Context(), "platform@example.com", "long-enough-password", "Platform Admin", "Admin Workspace")
	must(t, err)
	admin, _, err := service.SessionActor(t.Context(), adminSession.ID)
	must(t, err)
	if !admin.PlatformAdmin {
		t.Fatal("configured platform administrator was not recognized")
	}

	memberSession, err := service.Register(t.Context(), "member@example.com", "long-enough-password", "Member", "Customer Tenant")
	must(t, err)
	member, _, err := service.SessionActor(t.Context(), memberSession.ID)
	must(t, err)
	project, err := service.CreateProject(t.Context(), member, app.CreateProjectInput{BrandName: "Customer", ProductName: "Product"}, "project-create")
	must(t, err)

	if _, err := service.PlatformOverview(t.Context(), member); err == nil {
		t.Fatal("tenant administrator must not access the platform overview")
	} else {
		assertDomainCode(t, err, "PLATFORM_ADMIN_REQUIRED")
	}
	overview, err := service.PlatformOverview(t.Context(), admin)
	must(t, err)
	if overview.Counts.Tenants != 2 || overview.Counts.ActiveTenants != 2 || overview.Counts.Users != 2 || overview.Counts.Projects != 1 {
		t.Fatalf("unexpected platform counts %#v", overview.Counts)
	}
	if len(overview.Tenants) != 2 || len(overview.Users) != 2 {
		t.Fatalf("unexpected platform projections: tenants=%d users=%d", len(overview.Tenants), len(overview.Users))
	}
	for _, tenant := range overview.Tenants {
		if len(tenant.ContentTypes) != 1 || tenant.ContentTypes[0] != domain.ContentTypeVideoScript {
			t.Fatalf("tenant must default to video scripts only: %#v", tenant.ContentTypes)
		}
	}
	if project.TenantID != member.TenantID {
		t.Fatal("project fixture belongs to the wrong tenant")
	}
	if _, err := service.UpdatePlatformTenantContentCapability(t.Context(), member, member.TenantID, domain.ContentTypeWeChatArticle, true, "member-enable"); err == nil {
		t.Fatal("tenant administrator must not change platform content capabilities")
	} else {
		assertDomainCode(t, err, "PLATFORM_ADMIN_REQUIRED")
	}
	if _, err := service.UpdatePlatformTenantContentCapability(t.Context(), admin, member.TenantID, domain.ContentTypeVideoScript, false, "disable-video"); err == nil {
		t.Fatal("the default video script capability must not be mutable")
	} else {
		assertDomainCode(t, err, "TENANT_CONTENT_TYPE_INVALID")
	}
	updated, err := service.UpdatePlatformTenantContentCapability(t.Context(), admin, member.TenantID, domain.ContentTypeWeChatArticle, true, "enable-wechat")
	must(t, err)
	if len(updated.ContentTypes) != 2 || updated.ContentTypes[0] != domain.ContentTypeVideoScript || updated.ContentTypes[1] != domain.ContentTypeWeChatArticle {
		t.Fatalf("wechat article capability was not enabled: %#v", updated.ContentTypes)
	}
	adminTypes, err := service.TenantContentTypes(t.Context(), admin.TenantID)
	must(t, err)
	if len(adminTypes) != 1 || adminTypes[0] != domain.ContentTypeVideoScript {
		t.Fatalf("content capability leaked across tenants: %#v", adminTypes)
	}
	updated, err = service.UpdatePlatformTenantContentCapability(t.Context(), admin, member.TenantID, domain.ContentTypeWeChatArticle, false, "disable-wechat")
	must(t, err)
	if len(updated.ContentTypes) != 1 || updated.ContentTypes[0] != domain.ContentTypeVideoScript {
		t.Fatalf("wechat article capability was not disabled: %#v", updated.ContentTypes)
	}

	if _, err := service.UpdatePlatformTenantStatus(t.Context(), admin, admin.TenantID, "suspended", "suspend-current"); err == nil {
		t.Fatal("platform administrator must not suspend the current session tenant")
	} else {
		assertDomainCode(t, err, "CURRENT_TENANT_REQUIRED")
	}
	tenant, err := service.UpdatePlatformTenantStatus(t.Context(), admin, member.TenantID, "suspended", "suspend-customer")
	must(t, err)
	if tenant.Status != "suspended" {
		t.Fatalf("unexpected tenant status %q", tenant.Status)
	}
	if _, _, err := service.SessionActor(t.Context(), memberSession.ID); err == nil {
		t.Fatal("suspending a tenant must revoke its active sessions")
	}
	if _, err := service.Login(t.Context(), "member@example.com", "long-enough-password"); err == nil {
		t.Fatal("a user with only suspended tenants must not be able to log in")
	} else {
		assertDomainCode(t, err, "TENANT_REQUIRED")
	}
	tenant, err = service.UpdatePlatformTenantStatus(t.Context(), admin, member.TenantID, "active", "restore-customer")
	must(t, err)
	if tenant.Status != "active" {
		t.Fatalf("unexpected restored tenant status %q", tenant.Status)
	}
	if _, err := service.Login(t.Context(), "member@example.com", "long-enough-password"); err != nil {
		t.Fatalf("restored tenant member could not log in: %v", err)
	}
}
