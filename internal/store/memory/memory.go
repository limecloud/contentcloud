package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	storecontract "github.com/limecloud/contentcloud/internal/store"
)

type Store struct {
	mu                   sync.RWMutex
	users                map[string]domain.User
	userByEmail          map[string]string
	sessions             map[string]domain.Session
	tenants              map[string]domain.Tenant
	memberships          map[string]domain.Membership
	membershipInvites    map[string]domain.MembershipInvite
	projects             map[string]domain.Project
	projectTemplates     map[string]domain.ProjectTemplate
	connects             map[string]domain.ConnectSession
	bootstrapAttempts    map[string]domain.BootstrapAttempt
	bootstrapEvents      map[string]map[int64]domain.BootstrapProgressEvent
	bootstrapDiagnostics map[string]domain.BootstrapDiagnostic
	devices              map[string]domain.Device
	workspaceBindings    map[string]domain.WorkspaceBinding
	userDeviceFlows      map[string]domain.UserDeviceFlow
	cliTokens            map[string]domain.CLIToken
	sources              map[string]domain.Source
	revisions            map[string]domain.SourceRevision
	evidence             map[string]domain.EvidenceSpan
	assets               map[string]domain.Asset
	rightsRecords        map[string]domain.RightsRecord
	knowledge            map[string]domain.KnowledgeItem
	knowledgeConflicts   map[string]domain.KnowledgeConflict
	decisionRequests     map[string]domain.DecisionRequest
	snapshots            map[string]domain.ContextSnapshot
	runs                 map[string]domain.TaskRun
	executionBundles     map[string]environment.CreativeExecutionBundle
	runAttempts          map[string]domain.RunAttempt
	approvals            map[string]domain.ApprovalDecision
	reviewCycles         map[string]domain.ReviewCycle
	reviewComments       map[string]domain.ReviewComment
	reviewGrants         map[string]domain.ReviewGrant
	submissions          map[string]domain.Submission
	submissionRevisions  map[string]domain.SubmissionRevision
	approvedSnapshots    map[string]domain.ApprovedSnapshot
	artifacts            map[string]domain.Artifact
	deliveryPackages     map[string]domain.DeliveryPackage
	performanceBatches   map[string]domain.PerformanceImportBatch
	observations         map[string]domain.PerformanceObservation
	ratingDecisions      map[string]domain.RatingDecision
	audits               []domain.AuditEvent
}

func New() *Store {
	return &Store{
		users: map[string]domain.User{}, userByEmail: map[string]string{}, sessions: map[string]domain.Session{},
		tenants: map[string]domain.Tenant{}, memberships: map[string]domain.Membership{}, membershipInvites: map[string]domain.MembershipInvite{}, projects: map[string]domain.Project{}, projectTemplates: map[string]domain.ProjectTemplate{},
		connects: map[string]domain.ConnectSession{}, bootstrapAttempts: map[string]domain.BootstrapAttempt{}, bootstrapEvents: map[string]map[int64]domain.BootstrapProgressEvent{}, bootstrapDiagnostics: map[string]domain.BootstrapDiagnostic{}, devices: map[string]domain.Device{}, workspaceBindings: map[string]domain.WorkspaceBinding{}, userDeviceFlows: map[string]domain.UserDeviceFlow{}, cliTokens: map[string]domain.CLIToken{},
		sources: map[string]domain.Source{}, revisions: map[string]domain.SourceRevision{}, evidence: map[string]domain.EvidenceSpan{}, assets: map[string]domain.Asset{}, rightsRecords: map[string]domain.RightsRecord{}, knowledge: map[string]domain.KnowledgeItem{}, knowledgeConflicts: map[string]domain.KnowledgeConflict{}, decisionRequests: map[string]domain.DecisionRequest{},
		snapshots: map[string]domain.ContextSnapshot{}, runs: map[string]domain.TaskRun{}, executionBundles: map[string]environment.CreativeExecutionBundle{}, runAttempts: map[string]domain.RunAttempt{},
		approvals: map[string]domain.ApprovalDecision{}, reviewCycles: map[string]domain.ReviewCycle{}, reviewComments: map[string]domain.ReviewComment{}, reviewGrants: map[string]domain.ReviewGrant{}, submissions: map[string]domain.Submission{}, submissionRevisions: map[string]domain.SubmissionRevision{}, approvedSnapshots: map[string]domain.ApprovedSnapshot{}, artifacts: map[string]domain.Artifact{}, deliveryPackages: map[string]domain.DeliveryPackage{}, performanceBatches: map[string]domain.PerformanceImportBatch{}, observations: map[string]domain.PerformanceObservation{}, ratingDecisions: map[string]domain.RatingDecision{}, audits: []domain.AuditEvent{},
	}
}

func membershipKey(tenantID, userID string) string { return tenantID + ":" + userID }

func (s *Store) CreateUser(_ context.Context, value domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	email := strings.ToLower(value.Email)
	if _, exists := s.userByEmail[email]; exists {
		return domain.Conflict("EMAIL_EXISTS", "邮箱已注册")
	}
	s.users[value.ID] = value
	s.userByEmail[email] = value.ID
	return nil
}
func (s *Store) UserByEmail(_ context.Context, email string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.userByEmail[strings.ToLower(email)]
	if !ok {
		return domain.User{}, domain.NotFound("用户")
	}
	return s.users[id], nil
}
func (s *Store) UserByID(_ context.Context, id string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.users[id]
	if !ok {
		return v, domain.NotFound("用户")
	}
	return v, nil
}
func (s *Store) SaveUser(_ context.Context, v domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.users[v.ID]
	if !ok {
		return domain.NotFound("用户")
	}
	if !strings.EqualFold(old.Email, v.Email) {
		if _, exists := s.userByEmail[strings.ToLower(v.Email)]; exists {
			return domain.Conflict("EMAIL_EXISTS", "邮箱已注册")
		}
		delete(s.userByEmail, strings.ToLower(old.Email))
		s.userByEmail[strings.ToLower(v.Email)] = v.ID
	}
	s.users[v.ID] = v
	return nil
}
func (s *Store) SaveSession(_ context.Context, v domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[v.ID] = v
	return nil
}
func (s *Store) SessionByID(_ context.Context, id string) (domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.sessions[id]
	if !ok || v.RevokedAt != nil || time.Now().After(v.ExpiresAt) {
		return v, domain.NotFound("会话")
	}
	return v, nil
}
func (s *Store) RevokeSession(_ context.Context, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	if !ok {
		return domain.NotFound("会话")
	}
	v.RevokedAt = &now
	s.sessions[id] = v
	return nil
}
func (s *Store) RevokeSessionsForUserTenant(_ context.Context, userID, tenantID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, v := range s.sessions {
		if v.UserID == userID && v.TenantID == tenantID && v.RevokedAt == nil {
			v.RevokedAt = &now
			s.sessions[id] = v
		}
	}
	return nil
}

func (s *Store) CreateTenant(_ context.Context, t domain.Tenant, m domain.Membership) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[t.ID] = t
	if m.Status == "" {
		m.Status = "active"
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = t.CreatedAt
	}
	s.memberships[membershipKey(m.TenantID, m.UserID)] = m
	return nil
}
func (s *Store) TenantsForUser(_ context.Context, userID string) ([]domain.Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Tenant{}
	for _, m := range s.memberships {
		if m.UserID == userID && m.Status == "active" && m.RevokedAt == nil && s.tenants[m.TenantID].Status == "active" {
			out = append(out, s.tenants[m.TenantID])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Membership(_ context.Context, tenantID, userID string) (domain.Membership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.memberships[membershipKey(tenantID, userID)]
	if !ok || v.Status != "active" || v.RevokedAt != nil {
		return v, domain.NotFound("成员关系")
	}
	return v, nil
}

func (s *Store) Memberships(_ context.Context, tenantID string) ([]domain.Membership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Membership{}
	for _, v := range s.memberships {
		if v.TenantID == tenantID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) PlatformTenants(_ context.Context) ([]domain.PlatformTenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	out := make([]domain.PlatformTenant, 0, len(s.tenants))
	for _, tenant := range s.tenants {
		value := domain.PlatformTenant{Tenant: tenant}
		for _, membership := range s.memberships {
			if membership.TenantID == tenant.ID && membership.Status == "active" && membership.RevokedAt == nil {
				value.MemberCount++
			}
		}
		for _, project := range s.projects {
			if project.TenantID == tenant.ID {
				value.ProjectCount++
				if value.LastActivityAt == nil || project.UpdatedAt.After(*value.LastActivityAt) {
					updatedAt := project.UpdatedAt
					value.LastActivityAt = &updatedAt
				}
			}
		}
		for _, device := range s.devices {
			if device.TenantID == tenant.ID && device.RevokedAt == nil && now.Sub(device.LastSeenAt) <= 2*time.Minute {
				value.DeviceCount++
			}
		}
		for _, run := range s.runs {
			if run.TenantID == tenant.ID && (run.State == "queued" || run.State == "leased" || run.State == "running") {
				value.ActiveRunCount++
			}
		}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) PlatformUsers(_ context.Context) ([]domain.PlatformUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.PlatformUser, 0, len(s.users))
	for _, user := range s.users {
		value := domain.PlatformUser{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, VerifiedAt: user.VerifiedAt, CreatedAt: user.CreatedAt, Memberships: []domain.PlatformUserMembership{}}
		for _, membership := range s.memberships {
			if membership.UserID == user.ID {
				value.Memberships = append(value.Memberships, domain.PlatformUserMembership{TenantID: membership.TenantID, TenantName: s.tenants[membership.TenantID].Name, Role: membership.Role, Status: membership.Status})
			}
		}
		sort.Slice(value.Memberships, func(i, j int) bool { return value.Memberships[i].TenantName < value.Memberships[j].TenantName })
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) SetTenantStatus(_ context.Context, tenantID, status string, now time.Time) (domain.Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tenant, ok := s.tenants[tenantID]
	if !ok {
		return tenant, domain.NotFound("租户")
	}
	tenant.Status = status
	s.tenants[tenantID] = tenant
	if status == "suspended" {
		for id, session := range s.sessions {
			if session.TenantID == tenantID && session.RevokedAt == nil {
				session.RevokedAt = &now
				s.sessions[id] = session
			}
		}
	}
	return tenant, nil
}

func (s *Store) SaveMembership(_ context.Context, v domain.Membership) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := membershipKey(v.TenantID, v.UserID)
	if _, ok := s.memberships[key]; !ok {
		if v.Status == "" {
			v.Status = "active"
		}
	}
	s.memberships[key] = v
	return nil
}

func (s *Store) CreateMembershipInvite(_ context.Context, v domain.MembershipInvite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.membershipInvites {
		if existing.TenantID == v.TenantID && strings.EqualFold(existing.Email, v.Email) && existing.Status == "pending" {
			return domain.Conflict("INVITE_EXISTS", "该邮箱已有待接受邀请")
		}
	}
	s.membershipInvites[v.ID] = v
	return nil
}

func (s *Store) MembershipInviteByTokenHash(_ context.Context, hash string) (domain.MembershipInvite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.membershipInvites {
		if v.TokenHash == hash {
			return v, nil
		}
	}
	return domain.MembershipInvite{}, domain.NotFound("成员邀请")
}

func (s *Store) MembershipInvites(_ context.Context, tenantID string) ([]domain.MembershipInvite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.MembershipInvite{}
	for _, v := range s.membershipInvites {
		if v.TenantID == tenantID {
			v.TokenHash, v.PlaintextToken = "", ""
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) SaveMembershipInvite(_ context.Context, v domain.MembershipInvite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.membershipInvites[v.ID]; !ok {
		return domain.NotFound("成员邀请")
	}
	s.membershipInvites[v.ID] = v
	return nil
}

func (s *Store) pendingMembershipInvite(tokenHash, email string, now time.Time) (domain.MembershipInvite, error) {
	for _, invite := range s.membershipInvites {
		if invite.TokenHash != tokenHash {
			continue
		}
		if err := invite.ValidateAcceptance(email, now); err != nil {
			return domain.MembershipInvite{}, err
		}
		return invite, nil
	}
	return domain.MembershipInvite{}, domain.Conflict("INVITE_INVALID", "邀请无效、已撤销、邮箱不匹配或已过期")
}

func (s *Store) redeemMembershipInvite(invite domain.MembershipInvite, userID string, now time.Time) domain.Membership {
	membership := domain.Membership{TenantID: invite.TenantID, UserID: userID, Role: invite.Role, Status: "active", CreatedAt: now}
	s.memberships[membershipKey(membership.TenantID, membership.UserID)] = membership
	invite.Status, invite.AcceptedBy, invite.AcceptedAt = "accepted", userID, &now
	s.membershipInvites[invite.ID] = invite
	return membership
}

func (s *Store) AcceptMembershipInvite(_ context.Context, tokenHash string, user domain.User, now time.Time) (domain.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[user.ID]; !ok {
		return domain.Membership{}, domain.NotFound("用户")
	}
	invite, err := s.pendingMembershipInvite(tokenHash, user.Email, now)
	if err != nil {
		return domain.Membership{}, err
	}
	return s.redeemMembershipInvite(invite, user.ID, now), nil
}

func (s *Store) RegisterWithInvite(_ context.Context, user domain.User, tokenHash string, session domain.Session, now time.Time) (domain.Session, domain.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, err := s.pendingMembershipInvite(tokenHash, user.Email, now)
	if err != nil {
		return domain.Session{}, domain.Membership{}, err
	}
	email := strings.ToLower(user.Email)
	if _, exists := s.userByEmail[email]; exists {
		return domain.Session{}, domain.Membership{}, domain.Conflict("EMAIL_EXISTS", "邮箱已注册")
	}
	session.UserID, session.TenantID = user.ID, invite.TenantID
	membership := s.redeemMembershipInvite(invite, user.ID, now)
	s.users[user.ID] = user
	s.userByEmail[email] = user.ID
	s.sessions[session.ID] = session
	return session, membership, nil
}

func (s *Store) CreateProject(_ context.Context, v domain.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[v.ID] = v
	return nil
}
func (s *Store) Projects(_ context.Context, tenantID string) ([]domain.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Project{}
	for _, v := range s.projects {
		if v.TenantID == tenantID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) Project(_ context.Context, tenantID, id string) (domain.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.projects[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("项目")
	}
	return v, nil
}
func (s *Store) SaveProject(_ context.Context, v domain.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.projects[v.ID]; !ok || old.TenantID != v.TenantID {
		return domain.NotFound("项目")
	}
	s.projects[v.ID] = v
	return nil
}

func (s *Store) UpdateProject(_ context.Context, v domain.Project, expectedVersion int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.projects[v.ID]
	if !ok || old.TenantID != v.TenantID {
		return domain.NotFound("项目")
	}
	if old.RowVersion != expectedVersion {
		return domain.Conflict("ROW_VERSION_CONFLICT", "项目已被其他用户修改")
	}
	s.projects[v.ID] = v
	return nil
}

func (s *Store) CreateProjectTemplate(_ context.Context, v domain.ProjectTemplate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.projectTemplates {
		if existing.TenantID == v.TenantID && existing.Name == v.Name {
			return domain.Conflict("PROJECT_TEMPLATE_EXISTS", "项目模板名称已存在")
		}
	}
	s.projectTemplates[v.ID] = v
	return nil
}

func (s *Store) ProjectTemplates(_ context.Context, tenantID string) ([]domain.ProjectTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.ProjectTemplate{}
	for _, v := range s.projectTemplates {
		if v.TenantID == tenantID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) ProjectTemplate(_ context.Context, tenantID, id string) (domain.ProjectTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.projectTemplates[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("项目模板")
	}
	return v, nil
}

func (s *Store) CreateConnectSession(_ context.Context, v domain.ConnectSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connects[v.ID] = v
	return nil
}
func (s *Store) ConnectSessionByID(_ context.Context, tenantID, id string) (domain.ConnectSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.connects[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("连接会话")
	}
	return v, nil
}
func (s *Store) SaveConnectSession(_ context.Context, v domain.ConnectSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.connects[v.ID]
	if !ok || old.TenantID != v.TenantID {
		return domain.NotFound("连接会话")
	}
	s.connects[v.ID] = v
	return nil
}
func (s *Store) SaveDevice(_ context.Context, v domain.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.devices[v.ID]; ok && v.TokenHash == "" {
		v.TokenHash = existing.TokenHash
	}
	s.devices[v.ID] = v
	for id, session := range s.connects {
		if session.ConsumedDeviceID != v.ID || session.State != "verifying" {
			continue
		}
		session.State = "connected"
		s.connects[id] = session
		project := s.projects[session.ProjectID]
		project.ConnectedDevices++
		project.Status = "active"
		project.UpdatedAt = v.LastSeenAt
		project.RowVersion++
		s.projects[project.ID] = project
	}
	return nil
}
func (s *Store) DeviceByTokenHash(_ context.Context, hash string) (domain.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.devices {
		if v.TokenHash == hash && v.RevokedAt == nil {
			return v, nil
		}
	}
	return domain.Device{}, domain.NotFound("设备")
}
func (s *Store) Devices(_ context.Context, tenantID, projectID string) ([]domain.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Device{}
	for _, v := range s.devices {
		if v.TenantID != tenantID {
			continue
		}
		if projectID != "" && !contains(v.ProjectIDs, projectID) {
			continue
		}
		v.TokenHash = ""
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeenAt.After(out[j].LastSeenAt) })
	return out, nil
}

func (s *Store) Device(_ context.Context, tenantID, id string) (domain.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.devices[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("设备")
	}
	v.TokenHash = ""
	return v, nil
}

func (s *Store) GrantDeviceProject(_ context.Context, tenantID, projectID, deviceID, _ string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	device, ok := s.devices[deviceID]
	if !ok || device.TenantID != tenantID || device.RevokedAt != nil {
		return domain.NotFound("设备")
	}
	project, ok := s.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return domain.NotFound("项目")
	}
	if contains(device.ProjectIDs, projectID) {
		return nil
	}
	device.ProjectIDs = append(device.ProjectIDs, projectID)
	s.devices[deviceID] = device
	project.ConnectedDevices++
	project.UpdatedAt = now
	project.RowVersion++
	s.projects[projectID] = project
	return nil
}

func (s *Store) RevokeDeviceProject(_ context.Context, tenantID, projectID, deviceID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	device, ok := s.devices[deviceID]
	if !ok || device.TenantID != tenantID {
		return domain.NotFound("设备")
	}
	found := false
	next := make([]string, 0, len(device.ProjectIDs))
	for _, id := range device.ProjectIDs {
		if id == projectID {
			found = true
			continue
		}
		next = append(next, id)
	}
	if !found {
		return domain.NotFound("项目设备授权")
	}
	device.ProjectIDs = next
	s.devices[deviceID] = device
	if project, ok := s.projects[projectID]; ok && project.TenantID == tenantID {
		if project.ConnectedDevices > 0 {
			project.ConnectedDevices--
		}
		project.UpdatedAt = now
		project.RowVersion++
		s.projects[projectID] = project
	}
	return nil
}

func (s *Store) RevokeDevice(_ context.Context, tenantID, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.devices[id]
	if !ok || v.TenantID != tenantID {
		return domain.NotFound("设备")
	}
	v.RevokedAt = &now
	s.devices[id] = v
	return nil
}

func (s *Store) CreateUserDeviceFlow(_ context.Context, v domain.UserDeviceFlow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userDeviceFlows[v.ID] = v
	return nil
}

func (s *Store) UserDeviceFlowByCodeHash(_ context.Context, hash string) (domain.UserDeviceFlow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.userDeviceFlows {
		if v.DeviceCodeHash == hash {
			return v, nil
		}
	}
	return domain.UserDeviceFlow{}, domain.NotFound("登录授权")
}

func (s *Store) UserDeviceFlowByUserCode(_ context.Context, code string) (domain.UserDeviceFlow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.userDeviceFlows {
		if strings.EqualFold(v.UserCode, code) {
			return v, nil
		}
	}
	return domain.UserDeviceFlow{}, domain.NotFound("登录授权")
}

func (s *Store) SaveUserDeviceFlow(_ context.Context, v domain.UserDeviceFlow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.userDeviceFlows[v.ID]; !ok {
		return domain.NotFound("登录授权")
	}
	s.userDeviceFlows[v.ID] = v
	return nil
}

func (s *Store) CreateCLIToken(_ context.Context, v domain.CLIToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cliTokens[v.ID] = v
	return nil
}

func (s *Store) CLITokenByHash(_ context.Context, hash string) (domain.CLIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.cliTokens {
		if v.TokenHash == hash && v.RevokedAt == nil && time.Now().Before(v.ExpiresAt) {
			return v, nil
		}
	}
	return domain.CLIToken{}, domain.NotFound("CLI 凭据")
}

func (s *Store) RevokeCLIToken(_ context.Context, hash string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, v := range s.cliTokens {
		if v.TokenHash == hash {
			v.RevokedAt = &now
			s.cliTokens[id] = v
			return nil
		}
	}
	return domain.NotFound("CLI 凭据")
}

func (s *Store) CreateKnowledge(_ context.Context, v domain.KnowledgeItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.knowledge[v.ID] = v
	return nil
}
func (s *Store) Knowledge(_ context.Context, tenantID, projectID string) ([]domain.KnowledgeItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.KnowledgeItem{}
	for _, v := range s.knowledge {
		if v.TenantID == tenantID && v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) KnowledgeItem(_ context.Context, tenantID, id string) (domain.KnowledgeItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.knowledge[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("知识项")
	}
	return v, nil
}
func (s *Store) SaveKnowledge(_ context.Context, v domain.KnowledgeItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.knowledge[v.ID]; !ok || old.TenantID != v.TenantID {
		return domain.NotFound("知识项")
	}
	s.knowledge[v.ID] = v
	return nil
}

func (s *Store) CreateSnapshot(_ context.Context, v domain.ContextSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[v.ID] = v
	return nil
}
func (s *Store) Snapshot(_ context.Context, tenantID, id string) (domain.ContextSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.snapshots[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("上下文快照")
	}
	return v, nil
}
func (s *Store) CreateRun(_ context.Context, v domain.TaskRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createRun(v)
}

func (s *Store) CreateRunWithBundle(_ context.Context, v domain.TaskRun, bundle environment.CreativeExecutionBundle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bundle.ProjectID != v.ProjectID || bundle.Subject.ID != v.InputSnapshotID {
		return domain.Conflict("EXECUTION_BUNDLE_RUN_MISMATCH", "CreativeExecutionBundle 与 TaskRun 不匹配")
	}
	if err := s.createRun(v); err != nil {
		return err
	}
	s.executionBundles[v.ID] = bundle
	return nil
}

func (s *Store) createRun(v domain.TaskRun) error {
	for _, r := range s.runs {
		if r.TenantID == v.TenantID && r.IdempotencyKey == v.IdempotencyKey {
			return domain.Conflict("IDEMPOTENCY_CONFLICT", "幂等键已存在")
		}
	}
	s.runs[v.ID] = v
	return nil
}

func (s *Store) ExecutionBundle(_ context.Context, tenantID, runID string) (environment.CreativeExecutionBundle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, exists := s.runs[runID]
	bundle, bundled := s.executionBundles[runID]
	if !exists || run.TenantID != tenantID || !bundled {
		return environment.CreativeExecutionBundle{}, domain.NotFound("CreativeExecutionBundle")
	}
	return bundle, nil
}
func (s *Store) Runs(_ context.Context, tenantID, projectID string) ([]domain.TaskRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.TaskRun{}
	for _, v := range s.runs {
		if v.TenantID == tenantID && (projectID == "" || v.ProjectID == projectID) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Run(_ context.Context, tenantID, id string) (domain.TaskRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.runs[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("任务")
	}
	return v, nil
}
func (s *Store) SaveRun(_ context.Context, v domain.TaskRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.runs[v.ID]; !ok || old.TenantID != v.TenantID {
		return domain.NotFound("任务")
	}
	s.runs[v.ID] = v
	return nil
}
func (s *Store) LeaseNextRun(_ context.Context, tenantID, deviceID string, eligible []storecontract.RunLeaseCandidate, attemptID, tokenHash string, now time.Time) (domain.TaskRun, domain.RunAttempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	device, ok := s.devices[deviceID]
	if !ok || device.TenantID != tenantID || device.RevokedAt != nil {
		return domain.TaskRun{}, domain.RunAttempt{}, domain.NotFound("设备")
	}
	capabilities := make(map[string]domain.Capability, len(eligible))
	for _, candidate := range eligible {
		if candidate.RunID == "" || candidate.Capability.ID == "" {
			continue
		}
		capabilities[candidate.RunID] = candidate.Capability
	}
	var candidates []domain.TaskRun
	for _, r := range s.runs {
		if r.State != "queued" || !contains(device.ProjectIDs, r.ProjectID) {
			continue
		}
		capability, allowed := capabilities[r.ID]
		if allowed && r.AcceptsCapability(capability) {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return domain.TaskRun{}, domain.RunAttempt{}, domain.NotFound("可领取任务")
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
		}
		return candidates[i].Priority > candidates[j].Priority
	})
	v := candidates[0]
	until := now.Add(5 * time.Minute)
	v.State = "leased"
	v.LeaseDeviceID = deviceID
	v.LeaseExpiresAt = &until
	v.AttemptCount++
	v.UpdatedAt = now
	capability := capabilities[v.ID]
	attempt := domain.RunAttempt{ID: attemptID, TenantID: tenantID, ProjectID: v.ProjectID, RunID: v.ID, DeviceID: deviceID, State: "leased", CapabilityID: capability.ID, CapabilityVersion: capability.Version, CapabilityDigest: capability.Digest, InputSchema: capability.InputSchema, OutputSchema: capability.OutputSchema, TokenHash: tokenHash, LeaseExpiresAt: until, Usage: map[string]any{}, CreatedAt: now}
	v.ActiveAttemptID = attempt.ID
	v.RunTokenHash = tokenHash
	v.ProgressLabel = "任务已领取"
	s.runs[v.ID] = v
	s.runAttempts[attempt.ID] = attempt
	return v, attempt, nil
}

func (s *Store) CreateApproval(_ context.Context, v domain.ApprovalDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.DecisionStage == "" {
		v.DecisionStage = "legacy"
	}
	s.approvals[v.ID] = v
	return nil
}
func (s *Store) Approvals(_ context.Context, tenantID, subjectID string) ([]domain.ApprovalDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.ApprovalDecision{}
	for _, v := range s.approvals {
		if v.TenantID == tenantID && (subjectID == "" || v.SubjectID == subjectID) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) AppendAudit(_ context.Context, v domain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits = append(s.audits, v)
	return nil
}
func (s *Store) AuditEvents(_ context.Context, tenantID, projectID string, limit int) ([]domain.AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.AuditEvent{}
	for i := len(s.audits) - 1; i >= 0 && len(out) < limit; i-- {
		v := s.audits[i]
		if v.TenantID == tenantID && (projectID == "" || v.ProjectID == projectID) {
			out = append(out, v)
		}
	}
	return out, nil
}

func contains(values []string, wanted string) bool {
	for _, v := range values {
		if v == wanted {
			return true
		}
	}
	return false
}

var _ interface{} = (*Store)(nil)
