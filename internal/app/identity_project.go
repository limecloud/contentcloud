package app

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

var validMembershipRoles = map[string]bool{
	"tenant_admin": true, "project_manager": true, "strategist": true,
	"editor": true, "reviewer": true, "viewer": true,
}

func requireRole(actor Actor, roles ...string) error {
	if actor.Type == "worker" || actor.Type == "device" {
		return nil
	}
	for _, role := range roles {
		if actor.Role == role {
			return nil
		}
	}
	return domain.Policy("ROLE_DENIED", "当前角色不能执行此操作", "联系项目负责人或租户管理员")
}

type MemberView struct {
	Membership  domain.Membership `json:"membership"`
	Email       string            `json:"email"`
	DisplayName string            `json:"display_name"`
}

func (s *Service) Tenants(ctx context.Context, actor Actor) ([]domain.Tenant, error) {
	return s.store.TenantsForUser(ctx, actor.UserID)
}

func (s *Service) SwitchTenant(ctx context.Context, sessionID, tenantID, requestID string) (domain.Session, error) {
	session, err := s.store.SessionByID(ctx, sessionID)
	if err != nil {
		return session, domain.E("authentication", "session", "SESSION_INVALID", "会话无效或已过期", 3)
	}
	membership, err := s.store.Membership(ctx, tenantID, session.UserID)
	if err != nil {
		return session, domain.NotFound("租户")
	}
	previousTenant := session.TenantID
	session.TenantID = tenantID
	if err := s.store.SaveSession(ctx, session); err != nil {
		return session, err
	}
	s.audit(ctx, Actor{UserID: session.UserID, TenantID: tenantID, Role: membership.Role, Type: "user"}, "", "session.tenant_switched", "session", session.ID, requestID, map[string]any{"from_tenant_id": previousTenant})
	return session, nil
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.store.RevokeSession(ctx, sessionID, s.now().UTC())
}

func (s *Service) Members(ctx context.Context, actor Actor) ([]MemberView, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer"); err != nil {
		return nil, domain.Policy("ROLE_DENIED", "当前角色不能查看团队成员", "联系租户管理员")
	}
	memberships, err := s.store.Memberships(ctx, actor.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]MemberView, 0, len(memberships))
	for _, membership := range memberships {
		user, err := s.store.UserByID(ctx, membership.UserID)
		if err != nil {
			return nil, err
		}
		out = append(out, MemberView{Membership: membership, Email: user.Email, DisplayName: user.DisplayName})
	}
	return out, nil
}

func (s *Service) CreateMembershipInvite(ctx context.Context, actor Actor, email, role, requestID string) (domain.MembershipInvite, error) {
	if actor.Role != "tenant_admin" {
		return domain.MembershipInvite{}, domain.Policy("ROLE_DENIED", "只有租户管理员可以邀请成员", "联系租户管理员")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") || !validMembershipRoles[role] {
		return domain.MembershipInvite{}, domain.Invalid("MEMBERSHIP_INVITE_INVALID", "邀请邮箱或角色无效")
	}
	if user, err := s.store.UserByEmail(ctx, email); err == nil {
		if _, memberErr := s.store.Membership(ctx, actor.TenantID, user.ID); memberErr == nil {
			return domain.MembershipInvite{}, domain.Conflict("MEMBERSHIP_EXISTS", "该用户已经是租户成员")
		}
	}
	plain, hash, err := domain.NewOpaqueToken("cci_", 24)
	if err != nil {
		return domain.MembershipInvite{}, err
	}
	now := s.now().UTC()
	invite := domain.MembershipInvite{ID: domain.NewID(), TenantID: actor.TenantID, Email: email, Role: role, InvitedBy: actor.UserID, TokenHash: hash, Status: "pending", ExpiresAt: now.Add(72 * time.Hour), CreatedAt: now, PlaintextToken: plain}
	stored := invite
	stored.PlaintextToken = ""
	if err := s.store.CreateMembershipInvite(ctx, stored); err != nil {
		return invite, err
	}
	s.audit(ctx, actor, "", "membership.invited", "membership_invite", invite.ID, requestID, map[string]any{"email": email, "role": role, "expires_at": invite.ExpiresAt})
	invite.TokenHash = ""
	return invite, nil
}

func (s *Service) MembershipInvites(ctx context.Context, actor Actor) ([]domain.MembershipInvite, error) {
	if actor.Role != "tenant_admin" {
		return nil, domain.Policy("ROLE_DENIED", "只有租户管理员可以查看成员邀请", "联系租户管理员")
	}
	return s.store.MembershipInvites(ctx, actor.TenantID)
}

func (s *Service) AcceptMembershipInvite(ctx context.Context, actor Actor, token, requestID string) (domain.Membership, error) {
	invite, err := s.store.MembershipInviteByTokenHash(ctx, domain.TokenHash(token))
	if err != nil {
		return domain.Membership{}, domain.Conflict("INVITE_INVALID", "邀请无效、已撤销或已过期")
	}
	user, err := s.store.UserByID(ctx, actor.UserID)
	if err != nil {
		return domain.Membership{}, err
	}
	now := s.now().UTC()
	if invite.Status != "pending" || invite.RevokedAt != nil || now.After(invite.ExpiresAt) || !strings.EqualFold(invite.Email, user.Email) {
		if invite.Status == "pending" && now.After(invite.ExpiresAt) {
			invite.Status = "expired"
			_ = s.store.SaveMembershipInvite(ctx, invite)
		}
		return domain.Membership{}, domain.Conflict("INVITE_INVALID", "邀请无效、已撤销、邮箱不匹配或已过期")
	}
	membership := domain.Membership{TenantID: invite.TenantID, UserID: user.ID, Role: invite.Role, Status: "active", CreatedAt: now}
	if err := s.store.SaveMembership(ctx, membership); err != nil {
		return membership, err
	}
	invite.Status, invite.AcceptedBy, invite.AcceptedAt = "accepted", user.ID, &now
	if err := s.store.SaveMembershipInvite(ctx, invite); err != nil {
		return membership, err
	}
	tenantActor := Actor{UserID: user.ID, TenantID: invite.TenantID, Role: invite.Role, Type: "user"}
	s.audit(ctx, tenantActor, "", "membership.invite_accepted", "membership", user.ID, requestID, map[string]any{"invite_id": invite.ID, "role": invite.Role})
	return membership, nil
}

func (s *Service) RevokeMembershipInvite(ctx context.Context, actor Actor, inviteID, requestID string) (domain.MembershipInvite, error) {
	if actor.Role != "tenant_admin" {
		return domain.MembershipInvite{}, domain.Policy("ROLE_DENIED", "只有租户管理员可以撤销邀请", "联系租户管理员")
	}
	invites, err := s.store.MembershipInvites(ctx, actor.TenantID)
	if err != nil {
		return domain.MembershipInvite{}, err
	}
	for _, invite := range invites {
		if invite.ID != inviteID {
			continue
		}
		if invite.Status != "pending" {
			return invite, domain.Conflict("INVITE_STATE_INVALID", "只有待接受邀请可以撤销")
		}
		now := s.now().UTC()
		invite.Status, invite.RevokedAt = "revoked", &now
		if err := s.store.SaveMembershipInvite(ctx, invite); err != nil {
			return invite, err
		}
		s.audit(ctx, actor, "", "membership.invite_revoked", "membership_invite", invite.ID, requestID, map[string]any{})
		return invite, nil
	}
	return domain.MembershipInvite{}, domain.NotFound("成员邀请")
}

func (s *Service) UpdateMembershipRole(ctx context.Context, actor Actor, userID, role, requestID string) (domain.Membership, error) {
	if actor.Role != "tenant_admin" || !validMembershipRoles[role] {
		return domain.Membership{}, domain.Policy("ROLE_DENIED", "只有租户管理员可以分配有效角色", "联系租户管理员")
	}
	membership, err := s.store.Membership(ctx, actor.TenantID, userID)
	if err != nil {
		return membership, err
	}
	if membership.Role == "tenant_admin" && role != "tenant_admin" {
		if err := s.ensureAnotherAdmin(ctx, actor.TenantID, userID); err != nil {
			return membership, err
		}
	}
	previous := membership.Role
	membership.Role = role
	if err := s.store.SaveMembership(ctx, membership); err != nil {
		return membership, err
	}
	s.audit(ctx, actor, "", "membership.role_changed", "membership", userID, requestID, map[string]any{"from": previous, "to": role})
	return membership, nil
}

func (s *Service) RevokeMembership(ctx context.Context, actor Actor, userID, requestID string) (domain.Membership, error) {
	if actor.Role != "tenant_admin" {
		return domain.Membership{}, domain.Policy("ROLE_DENIED", "只有租户管理员可以撤销成员", "联系租户管理员")
	}
	membership, err := s.store.Membership(ctx, actor.TenantID, userID)
	if err != nil {
		return membership, err
	}
	if membership.Role == "tenant_admin" {
		if err := s.ensureAnotherAdmin(ctx, actor.TenantID, userID); err != nil {
			return membership, err
		}
	}
	now := s.now().UTC()
	membership.Status, membership.RevokedAt = "revoked", &now
	if err := s.store.SaveMembership(ctx, membership); err != nil {
		return membership, err
	}
	if err := s.store.RevokeSessionsForUserTenant(ctx, userID, actor.TenantID, now); err != nil {
		return membership, err
	}
	s.audit(ctx, actor, "", "membership.revoked", "membership", userID, requestID, map[string]any{"role": membership.Role})
	return membership, nil
}

func (s *Service) ensureAnotherAdmin(ctx context.Context, tenantID, excludedUserID string) error {
	memberships, err := s.store.Memberships(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, membership := range memberships {
		if membership.UserID != excludedUserID && membership.Role == "tenant_admin" && membership.Status == "active" && membership.RevokedAt == nil {
			return nil
		}
	}
	return domain.Policy("LAST_TENANT_ADMIN", "不能撤销或降级租户最后一名管理员", "先任命另一名租户管理员")
}

type UpdateProjectInput struct {
	RowVersion     int     `json:"row_version"`
	BrandName      *string `json:"brand_name,omitempty"`
	ProductName    *string `json:"product_name,omitempty"`
	Channel        *string `json:"channel,omitempty"`
	StageObjective *string `json:"stage_objective,omitempty"`
	OwnerName      *string `json:"owner_name,omitempty"`
	ReviewerName   *string `json:"reviewer_name,omitempty"`
	ClientApprover *string `json:"client_approver,omitempty"`
}

func (s *Service) UpdateProject(ctx context.Context, actor Actor, projectID string, in UpdateProjectInput, requestID string) (domain.Project, error) {
	if !canManage(actor.Role) {
		return domain.Project{}, domain.Policy("ROLE_DENIED", "当前角色不能修改项目", "联系项目负责人")
	}
	project, err := s.projectForWrite(ctx, actor, projectID)
	if err != nil {
		return project, err
	}
	if in.RowVersion <= 0 {
		return project, domain.Invalid("ROW_VERSION_REQUIRED", "修改项目必须携带 row_version")
	}
	changed := false
	if in.BrandName != nil {
		changed = true
		project.BrandName = strings.TrimSpace(*in.BrandName)
	}
	if in.ProductName != nil {
		changed = true
		project.ProductName = strings.TrimSpace(*in.ProductName)
	}
	if in.Channel != nil {
		changed = true
		project.Channel = strings.TrimSpace(*in.Channel)
	}
	if in.StageObjective != nil {
		changed = true
		project.StageObjective = strings.TrimSpace(*in.StageObjective)
	}
	if in.OwnerName != nil {
		changed = true
		project.OwnerName = strings.TrimSpace(*in.OwnerName)
	}
	if in.ReviewerName != nil {
		changed = true
		project.ReviewerName = strings.TrimSpace(*in.ReviewerName)
	}
	if in.ClientApprover != nil {
		changed = true
		project.ClientApprover = strings.TrimSpace(*in.ClientApprover)
	}
	if !changed {
		return project, domain.Invalid("PROJECT_UPDATE_EMPTY", "至少提供一个需要修改的项目字段")
	}
	if project.BrandName == "" || project.ProductName == "" || project.Channel == "" {
		return project, domain.Invalid("PROJECT_FIELDS_REQUIRED", "品牌名、单品名和渠道必填")
	}
	expected := in.RowVersion
	project.RowVersion = expected + 1
	project.UpdatedAt = s.now().UTC()
	if err := s.store.UpdateProject(ctx, project, expected); err != nil {
		return project, err
	}
	s.audit(ctx, actor, project.ID, "project.updated", "project", project.ID, requestID, map[string]any{"row_version": project.RowVersion})
	return project, nil
}

func (s *Service) SetProjectLifecycle(ctx context.Context, actor Actor, projectID, action string, rowVersion int, requestID string) (domain.Project, error) {
	project, err := s.store.Project(ctx, actor.TenantID, projectID)
	if err != nil {
		return project, err
	}
	if rowVersion <= 0 || rowVersion != project.RowVersion {
		return project, domain.Conflict("ROW_VERSION_CONFLICT", "项目版本已变化，请刷新后重试")
	}
	previous := project.Status
	switch action {
	case "archive":
		if !canManage(actor.Role) || project.Status == "archived" {
			return project, domain.Conflict("PROJECT_STATE_INVALID", "当前项目不能归档")
		}
		project.Status = "archived"
	case "restore":
		if actor.Role != "tenant_admin" {
			return project, domain.Policy("ROLE_DENIED", "只有租户管理员可以恢复项目", "联系租户管理员")
		}
		if project.Status != "archived" {
			return project, domain.Conflict("PROJECT_STATE_INVALID", "只有已归档项目可以恢复")
		}
		project.Status = "active"
	default:
		return project, domain.Invalid("PROJECT_ACTION_INVALID", "项目动作无效")
	}
	project.RowVersion++
	project.UpdatedAt = s.now().UTC()
	if err := s.store.UpdateProject(ctx, project, rowVersion); err != nil {
		return project, err
	}
	s.audit(ctx, actor, project.ID, "project."+action+"d", "project", project.ID, requestID, map[string]any{"from": previous, "to": project.Status, "row_version": project.RowVersion})
	return project, nil
}

func (s *Service) projectForWrite(ctx context.Context, actor Actor, projectID string) (domain.Project, error) {
	project, err := s.store.Project(ctx, actor.TenantID, projectID)
	if err != nil {
		return project, err
	}
	if project.Status == "archived" {
		return project, domain.Policy("PROJECT_ARCHIVED", "已归档项目为只读", "由租户管理员恢复项目后再修改")
	}
	return project, nil
}

func (s *Service) CancelConnectSession(ctx context.Context, actor Actor, id, requestID string) (domain.ConnectSession, error) {
	if !canManage(actor.Role) {
		return domain.ConnectSession{}, domain.Policy("ROLE_DENIED", "当前角色不能取消连接", "联系项目负责人")
	}
	session, err := s.store.ConnectSessionByID(ctx, actor.TenantID, id)
	if err != nil {
		return session, err
	}
	if session.State != "waiting_for_computer" {
		return session, domain.Conflict("CONNECT_SESSION_STATE_INVALID", "只有等待电脑连接的会话可以取消")
	}
	session.State = "canceled"
	if err := s.store.SaveConnectSession(ctx, session); err != nil {
		return session, err
	}
	s.audit(ctx, actor, session.ProjectID, "connect_session.canceled", "connect_session", session.ID, requestID, map[string]any{})
	return session, nil
}

type CreateProjectTemplateInput struct {
	Name           string `json:"name"`
	Channel        string `json:"channel"`
	StageObjective string `json:"stage_objective"`
}

func (s *Service) CreateProjectTemplate(ctx context.Context, actor Actor, in CreateProjectTemplateInput, requestID string) (domain.ProjectTemplate, error) {
	if actor.Role != "tenant_admin" {
		return domain.ProjectTemplate{}, domain.Policy("ROLE_DENIED", "只有租户管理员可以创建项目模板", "联系租户管理员")
	}
	if strings.TrimSpace(in.Name) == "" {
		return domain.ProjectTemplate{}, domain.Invalid("PROJECT_TEMPLATE_NAME_REQUIRED", "模板名称必填")
	}
	now := s.now().UTC()
	template := domain.ProjectTemplate{ID: domain.NewID(), TenantID: actor.TenantID, Name: strings.TrimSpace(in.Name), Channel: defaultString(in.Channel, "douyin"), StageObjective: strings.TrimSpace(in.StageObjective), CreatedBy: actor.UserID, CreatedAt: now}
	if err := s.store.CreateProjectTemplate(ctx, template); err != nil {
		return template, err
	}
	s.audit(ctx, actor, "", "project_template.created", "project_template", template.ID, requestID, map[string]any{"name": template.Name})
	return template, nil
}

func (s *Service) ProjectTemplates(ctx context.Context, actor Actor) ([]domain.ProjectTemplate, error) {
	return s.store.ProjectTemplates(ctx, actor.TenantID)
}
