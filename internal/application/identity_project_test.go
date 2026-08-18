package application_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

func TestMembershipInviteTenantSwitchRoleAndRevocation(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	adminSession, err := service.Identity.Register(ctx, "admin@example.com", "password-123", "管理员", "甲租户")
	if err != nil {
		t.Fatal(err)
	}
	admin, _, err := service.Identity.SessionActor(ctx, adminSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	memberSession, err := service.Identity.Register(ctx, "editor@example.com", "password-123", "编导", "个人租户")
	if err != nil {
		t.Fatal(err)
	}
	memberActor, _, err := service.Identity.SessionActor(ctx, memberSession.ID)
	if err != nil {
		t.Fatal(err)
	}

	invite, err := service.Identity.CreateMembershipInvite(ctx, admin, "editor@example.com", "editor", "req-invite")
	if err != nil {
		t.Fatal(err)
	}
	if invite.PlaintextToken == "" || invite.TokenHash != "" {
		t.Fatalf("invite must return plaintext once without hash: %#v", invite)
	}
	revokedInvite, err := service.Identity.CreateMembershipInvite(ctx, admin, "revoked@example.com", "viewer", "req-revoked-invite")
	if err != nil {
		t.Fatal(err)
	}
	revokedInvite, err = service.Identity.RevokeMembershipInvite(ctx, admin, revokedInvite.ID, "req-revoke-invite")
	if err != nil || revokedInvite.Status != "revoked" {
		t.Fatalf("invite revoke failed: %#v %v", revokedInvite, err)
	}
	listedInvites, err := service.Identity.MembershipInvites(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range listedInvites {
		if listed.PlaintextToken != "" || listed.TokenHash != "" {
			t.Fatalf("listed invite leaked a credential: %#v", listed)
		}
	}
	membership, err := service.Identity.AcceptMembershipInvite(ctx, memberActor, invite.PlaintextToken, "req-accept")
	if err != nil {
		t.Fatal(err)
	}
	if membership.TenantID != admin.TenantID || membership.Role != "editor" {
		t.Fatalf("unexpected membership: %#v", membership)
	}
	if _, err := service.Identity.SwitchTenant(ctx, memberSession.ID, admin.TenantID, "req-switch"); err != nil {
		t.Fatal(err)
	}
	switched, _, err := service.Identity.SessionActor(ctx, memberSession.ID)
	if err != nil || switched.Role != "editor" || switched.TenantID != admin.TenantID {
		t.Fatalf("switch failed: actor=%#v err=%v", switched, err)
	}
	if _, err := service.Identity.Members(ctx, switched); err == nil {
		t.Fatal("editor must not list tenant members")
	}
	updated, err := service.Identity.UpdateMembershipRole(ctx, admin, switched.UserID, "reviewer", "req-reviewer-role")
	if err != nil || updated.Role != "reviewer" {
		t.Fatalf("reviewer role update failed: %#v %v", updated, err)
	}
	reviewer := switched
	reviewer.Role = "reviewer"
	members, err := service.Identity.Members(ctx, reviewer)
	if err != nil || len(members) != 2 {
		t.Fatalf("reviewer must be able to list assignees: %#v %v", members, err)
	}

	updated, err = service.Identity.UpdateMembershipRole(ctx, admin, switched.UserID, "tenant_admin", "req-role")
	if err != nil || updated.Role != "tenant_admin" {
		t.Fatalf("role update failed: %#v %v", updated, err)
	}
	if _, err := service.Identity.RevokeMembership(ctx, admin, admin.UserID, "req-revoke"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Identity.SessionActor(ctx, adminSession.ID); err == nil {
		t.Fatal("revoked member session must fail immediately")
	}
}

func TestLastTenantAdminCannotBeRevoked(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(ctx, "admin@example.com", "password-123", "管理员", "甲租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, _ := service.Identity.SessionActor(ctx, session.ID)
	_, err = service.Identity.RevokeMembership(ctx, actor, actor.UserID, "req-last-admin")
	assertDomainCode(t, err, "LAST_TENANT_ADMIN")
}

func TestProjectTemplateOptimisticLockAndArchiveReadOnly(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(ctx, "admin@example.com", "password-123", "管理员", "甲租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, _ := service.Identity.SessionActor(ctx, session.ID)
	template, err := service.Identity.CreateProjectTemplate(ctx, actor, application.CreateProjectTemplateInput{Name: "抖音单品", Channel: "douyin", StageObjective: "验证主卖点"}, "req-template")
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{TemplateID: template.ID, BrandName: "金陵古香", ProductName: "线香"}, "req-project")
	if err != nil {
		t.Fatal(err)
	}
	if project.Channel != template.Channel || project.StageObjective != template.StageObjective {
		t.Fatalf("template fields not applied: %#v", project)
	}
	channel := "xiaohongshu"
	updated, err := service.Identity.UpdateProject(ctx, actor, project.ID, application.UpdateProjectInput{RowVersion: project.RowVersion, Channel: &channel}, "req-update")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Identity.UpdateProject(ctx, actor, project.ID, application.UpdateProjectInput{RowVersion: project.RowVersion, Channel: &channel}, "req-stale")
	assertDomainCode(t, err, "ROW_VERSION_CONFLICT")
	archived, err := service.Identity.SetProjectLifecycle(ctx, actor, project.ID, "archive", updated.RowVersion, "req-archive")
	if err != nil || archived.Status != "archived" {
		t.Fatalf("archive failed: %#v %v", archived, err)
	}
	_, err = service.Workspace.CreateConnectSession(ctx, actor, project.ID, "req-connect")
	assertDomainCode(t, err, "PROJECT_ARCHIVED")
	restored, err := service.Identity.SetProjectLifecycle(ctx, actor, project.ID, "restore", archived.RowVersion, "req-restore")
	if err != nil || restored.Status != "active" {
		t.Fatalf("restore failed: %#v %v", restored, err)
	}
	connect, err := service.Workspace.CreateConnectSession(ctx, actor, restored.ID, "req-connect-active")
	if err != nil || connect.ID == "" || connect.State != "waiting_for_computer" || connect.Progress != nil {
		t.Fatalf("connect session create failed: %#v %v", connect, err)
	}
	canceled, err := service.Identity.CancelConnectSession(ctx, actor, connect.ID, "req-connect-cancel")
	if err != nil || canceled.State != "canceled" {
		t.Fatalf("connect session cancel failed: %#v %v", canceled, err)
	}
	if _, err := service.Identity.CancelConnectSession(ctx, actor, connect.ID, "req-connect-cancel-replay"); err == nil {
		t.Fatal("canceling a terminal connect session must fail")
	}
}

func TestProjectTemplatesProvisionBuiltinPresetsIdempotently(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(ctx, "templates@example.com", "password-123", "管理员", "模板租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.Identity.ProjectTemplates(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 {
		t.Fatalf("expected three builtin project templates, got %#v", first)
	}
	for _, template := range first {
		if template.ID == "" || template.TenantID != actor.TenantID || template.CreatedBy != actor.UserID {
			t.Fatalf("builtin template is not tenant-scoped: %#v", template)
		}
	}

	second, err := service.Identity.ProjectTemplates(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first) {
		t.Fatalf("builtin templates are not idempotent: first=%d second=%d", len(first), len(second))
	}
}
