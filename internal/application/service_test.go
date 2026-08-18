package application_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/persistence/memory"

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

func TestTenantIsolation(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	aSession, _ := service.Identity.Register(ctx, "a@example.com", "long-enough-password", "A", "Tenant A")
	bSession, _ := service.Identity.Register(ctx, "b@example.com", "long-enough-password", "B", "Tenant B")
	a, _, _ := service.Identity.SessionActor(ctx, aSession.ID)
	b, _, _ := service.Identity.SessionActor(ctx, bSession.ID)
	project, _ := service.Workspace.CreateProject(ctx, a, application.CreateProjectInput{BrandName: "Same", ProductName: "Product"}, "")
	if _, err := service.Workspace.Project(ctx, b, project.ID); err == nil {
		t.Fatal("tenant B accessed tenant A project")
	}
}

func capabilities() []catalogdomain.Capability {
	return []catalogdomain.Capability{
		{ID: sourcedomain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: sourcedomain.TaskContractSchema, OutputSchema: sourcedomain.KnowledgeCandidatesSchema, LocalOnly: true},
	}
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

var _ = time.Second

// inviteFixture 返回一个租户管理员 actor 与一封发给 invitedEmail 的待接受邀请。
func inviteFixture(t *testing.T, service *application.Application, invitedEmail, role string) (application.Actor, identitydomain.MembershipInvite) {
	t.Helper()
	ctx := context.Background()
	adminSession, err := service.Identity.Register(ctx, "inviter@example.com", "long-enough-password", "管理员", "邀请方租户")
	must(t, err)
	admin, _, err := service.Identity.SessionActor(ctx, adminSession.ID)
	must(t, err)
	invite, err := service.Identity.CreateMembershipInvite(ctx, admin, invitedEmail, role, "req-invite")
	must(t, err)
	return admin, invite
}

func TestRegisterWithInviteJoinsInvitingTenant(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	admin, invite := inviteFixture(t, service, "newbie@example.com", "reviewer")

	session, err := service.Identity.RegisterWithInvite(ctx, "newbie@example.com", "long-enough-password", "新同事", invite.PlaintextToken)
	must(t, err)
	if session.TenantID != admin.TenantID {
		t.Fatalf("session must land in inviting tenant: got %s want %s", session.TenantID, admin.TenantID)
	}
	actor, user, err := service.Identity.SessionActor(ctx, session.ID)
	must(t, err)
	if actor.Role != "reviewer" {
		t.Fatalf("role must come from invite: got %s", actor.Role)
	}
	if user.DisplayName != "新同事" {
		t.Fatalf("unexpected display name %q", user.DisplayName)
	}
	// 关键区别：不得创建一个属于自己的新租户。
	tenants, err := service.Identity.Tenants(ctx, actor)
	must(t, err)
	if len(tenants) != 1 || tenants[0].ID != admin.TenantID {
		t.Fatalf("invited user must belong to exactly the inviting tenant: %#v", tenants)
	}
	if _, err := service.Identity.RegisterWithInvite(ctx, "other@example.com", "long-enough-password", "", invite.PlaintextToken); err == nil {
		t.Fatal("accepted invite must not be reusable")
	}
}

func TestRegisterWithInviteRejectsMismatchedEmailWithoutCreatingUser(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	_, invite := inviteFixture(t, service, "newbie@example.com", "reviewer")

	_, err := service.Identity.RegisterWithInvite(ctx, "attacker@example.com", "long-enough-password", "", invite.PlaintextToken)
	assertDomainCode(t, err, "INVITE_INVALID")
	// 邀请校验失败不得留下孤儿用户：该邮箱仍可正常注册自己的团队。
	if _, err := service.Identity.Register(ctx, "attacker@example.com", "long-enough-password", "", "自建租户"); err != nil {
		t.Fatalf("email must remain unregistered after failed invite: %v", err)
	}
}

func TestRegisterWithInviteRejectsRevokedAndUnknownToken(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	adminSession, err := service.Identity.Register(ctx, "inviter@example.com", "long-enough-password", "管理员", "邀请方租户")
	must(t, err)
	admin, _, err := service.Identity.SessionActor(ctx, adminSession.ID)
	must(t, err)
	invite, err := service.Identity.CreateMembershipInvite(ctx, admin, "revoked@example.com", "viewer", "req-invite")
	must(t, err)
	if _, err := service.Identity.RevokeMembershipInvite(ctx, admin, invite.ID, "req-revoke"); err != nil {
		t.Fatal(err)
	}

	_, err = service.Identity.RegisterWithInvite(ctx, "revoked@example.com", "long-enough-password", "", invite.PlaintextToken)
	assertDomainCode(t, err, "INVITE_INVALID")
	_, err = service.Identity.RegisterWithInvite(ctx, "nobody@example.com", "long-enough-password", "", "cci_not-a-real-token")
	assertDomainCode(t, err, "INVITE_INVALID")
}

func TestRegisterWithInviteStillValidatesCredentials(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	_, invite := inviteFixture(t, service, "newbie@example.com", "reviewer")

	_, err := service.Identity.RegisterWithInvite(ctx, "newbie@example.com", "short", "", invite.PlaintextToken)
	assertDomainCode(t, err, "REGISTRATION_INVALID")
	// 凭据校验先于邀请核销，邀请必须仍然可用。
	if _, err := service.Identity.RegisterWithInvite(ctx, "newbie@example.com", "long-enough-password", "", invite.PlaintextToken); err != nil {
		t.Fatalf("invite must survive a rejected registration attempt: %v", err)
	}
}

func TestAcceptMembershipInviteOnlySucceedsOnce(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	admin, invite := inviteFixture(t, service, "member@example.com", "editor")
	memberSession, err := service.Identity.Register(ctx, "member@example.com", "long-enough-password", "成员", "成员自己的租户")
	must(t, err)
	member, _, err := service.Identity.SessionActor(ctx, memberSession.ID)
	must(t, err)

	start := make(chan struct{})
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range errs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, errs[index] = service.Identity.AcceptMembershipInvite(ctx, member, invite.PlaintextToken, "req-accept")
		}(index)
	}
	close(start)
	wait.Wait()

	successes := 0
	for _, acceptErr := range errs {
		if acceptErr == nil {
			successes++
			continue
		}
		var domainErr *fault.Error
		if !errors.As(acceptErr, &domainErr) || domainErr.Code != "INVITE_INVALID" {
			t.Fatalf("unexpected concurrent accept error: %v", acceptErr)
		}
	}
	if successes != 1 {
		t.Fatalf("invite must be accepted exactly once, got %d successes", successes)
	}
	accepted, err := service.Identity.Members(ctx, admin)
	must(t, err)
	if len(accepted) != 2 {
		t.Fatalf("inviting tenant must contain admin and accepted member: %#v", accepted)
	}
}
