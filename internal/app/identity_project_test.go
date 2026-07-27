package app_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestMembershipInviteTenantSwitchRoleAndRevocation(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	adminSession, err := service.Register(ctx, "admin@example.com", "password-123", "管理员", "甲租户")
	if err != nil {
		t.Fatal(err)
	}
	admin, _, err := service.SessionActor(ctx, adminSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	memberSession, err := service.Register(ctx, "editor@example.com", "password-123", "编导", "个人租户")
	if err != nil {
		t.Fatal(err)
	}
	memberActor, _, err := service.SessionActor(ctx, memberSession.ID)
	if err != nil {
		t.Fatal(err)
	}

	invite, err := service.CreateMembershipInvite(ctx, admin, "editor@example.com", "editor", "req-invite")
	if err != nil {
		t.Fatal(err)
	}
	if invite.PlaintextToken == "" || invite.TokenHash != "" {
		t.Fatalf("invite must return plaintext once without hash: %#v", invite)
	}
	revokedInvite, err := service.CreateMembershipInvite(ctx, admin, "revoked@example.com", "viewer", "req-revoked-invite")
	if err != nil {
		t.Fatal(err)
	}
	revokedInvite, err = service.RevokeMembershipInvite(ctx, admin, revokedInvite.ID, "req-revoke-invite")
	if err != nil || revokedInvite.Status != "revoked" {
		t.Fatalf("invite revoke failed: %#v %v", revokedInvite, err)
	}
	listedInvites, err := service.MembershipInvites(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range listedInvites {
		if listed.PlaintextToken != "" || listed.TokenHash != "" {
			t.Fatalf("listed invite leaked a credential: %#v", listed)
		}
	}
	membership, err := service.AcceptMembershipInvite(ctx, memberActor, invite.PlaintextToken, "req-accept")
	if err != nil {
		t.Fatal(err)
	}
	if membership.TenantID != admin.TenantID || membership.Role != "editor" {
		t.Fatalf("unexpected membership: %#v", membership)
	}
	if _, err := service.SwitchTenant(ctx, memberSession.ID, admin.TenantID, "req-switch"); err != nil {
		t.Fatal(err)
	}
	switched, _, err := service.SessionActor(ctx, memberSession.ID)
	if err != nil || switched.Role != "editor" || switched.TenantID != admin.TenantID {
		t.Fatalf("switch failed: actor=%#v err=%v", switched, err)
	}
	if _, err := service.Members(ctx, switched); err == nil {
		t.Fatal("editor must not list tenant members")
	}
	updated, err := service.UpdateMembershipRole(ctx, admin, switched.UserID, "reviewer", "req-reviewer-role")
	if err != nil || updated.Role != "reviewer" {
		t.Fatalf("reviewer role update failed: %#v %v", updated, err)
	}
	reviewer := switched
	reviewer.Role = "reviewer"
	members, err := service.Members(ctx, reviewer)
	if err != nil || len(members) != 2 {
		t.Fatalf("reviewer must be able to list assignees: %#v %v", members, err)
	}

	updated, err = service.UpdateMembershipRole(ctx, admin, switched.UserID, "tenant_admin", "req-role")
	if err != nil || updated.Role != "tenant_admin" {
		t.Fatalf("role update failed: %#v %v", updated, err)
	}
	if _, err := service.RevokeMembership(ctx, admin, admin.UserID, "req-revoke"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.SessionActor(ctx, adminSession.ID); err == nil {
		t.Fatal("revoked member session must fail immediately")
	}
}

func TestLastTenantAdminCannotBeRevoked(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "admin@example.com", "password-123", "管理员", "甲租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, _ := service.SessionActor(ctx, session.ID)
	_, err = service.RevokeMembership(ctx, actor, actor.UserID, "req-last-admin")
	assertDomainCode(t, err, "LAST_TENANT_ADMIN")
}

func TestProjectTemplateOptimisticLockAndArchiveReadOnly(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "admin@example.com", "password-123", "管理员", "甲租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, _ := service.SessionActor(ctx, session.ID)
	template, err := service.CreateProjectTemplate(ctx, actor, app.CreateProjectTemplateInput{Name: "抖音单品", Channel: "douyin", StageObjective: "验证主卖点"}, "req-template")
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{TemplateID: template.ID, BrandName: "金陵古香", ProductName: "线香"}, "req-project")
	if err != nil {
		t.Fatal(err)
	}
	if project.Channel != template.Channel || project.StageObjective != template.StageObjective {
		t.Fatalf("template fields not applied: %#v", project)
	}
	channel := "xiaohongshu"
	updated, err := service.UpdateProject(ctx, actor, project.ID, app.UpdateProjectInput{RowVersion: project.RowVersion, Channel: &channel}, "req-update")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.UpdateProject(ctx, actor, project.ID, app.UpdateProjectInput{RowVersion: project.RowVersion, Channel: &channel}, "req-stale")
	assertDomainCode(t, err, "ROW_VERSION_CONFLICT")
	archived, err := service.SetProjectLifecycle(ctx, actor, project.ID, "archive", updated.RowVersion, "req-archive")
	if err != nil || archived.Status != "archived" {
		t.Fatalf("archive failed: %#v %v", archived, err)
	}
	_, err = service.CreateConnectSession(ctx, actor, project.ID, "req-connect")
	assertDomainCode(t, err, "PROJECT_ARCHIVED")
	restored, err := service.SetProjectLifecycle(ctx, actor, project.ID, "restore", archived.RowVersion, "req-restore")
	if err != nil || restored.Status != "active" {
		t.Fatalf("restore failed: %#v %v", restored, err)
	}
	connect, err := service.CreateConnectSession(ctx, actor, restored.ID, "req-connect-active")
	if err != nil || connect.ID == "" || connect.State != "waiting_for_computer" || connect.Progress != nil {
		t.Fatalf("connect session create failed: %#v %v", connect, err)
	}
	canceled, err := service.CancelConnectSession(ctx, actor, connect.ID, "req-connect-cancel")
	if err != nil || canceled.State != "canceled" {
		t.Fatalf("connect session cancel failed: %#v %v", canceled, err)
	}
	if _, err := service.CancelConnectSession(ctx, actor, connect.ID, "req-connect-cancel-replay"); err == nil {
		t.Fatal("canceling a terminal connect session must fail")
	}
}
