package app_test

import (
	"log/slog"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
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
	if project.TenantID != member.TenantID {
		t.Fatal("project fixture belongs to the wrong tenant")
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
