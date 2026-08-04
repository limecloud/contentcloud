package app

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/limecloud/contentcloud/internal/blob"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/store"
)

type Service struct {
	store               store.Store
	now                 func() time.Time
	log                 *slog.Logger
	blobs               blob.Store
	platformAdminEmails map[string]struct{}
	environmentControl  *environment.ControlPlane
	automationPolicy    map[string]environment.CapabilityRequirement
	automationPackIDs   map[string][]string
	daemonVersions      daemonVersionPolicy
}

type Actor struct {
	UserID        string
	TenantID      string
	Role          string
	Type          string
	DeviceID      string
	WorkspaceID   string
	PlatformAdmin bool
}

type Dashboard struct {
	Tenant      domain.Tenant       `json:"tenant"`
	Projects    []domain.Project    `json:"projects"`
	RecentRuns  []domain.TaskRun    `json:"recent_runs"`
	RecentAudit []domain.AuditEvent `json:"recent_audit"`
	Counts      map[string]int      `json:"counts"`
	Pipeline    []PipelineStage     `json:"pipeline"`
}

type PipelineStage struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Count   int    `json:"count"`
	Blocked int    `json:"blocked"`
}

type Option func(*Service)

func WithPlatformAdminEmails(emails ...string) Option {
	return func(service *Service) {
		for _, email := range emails {
			normalized := strings.ToLower(strings.TrimSpace(email))
			if normalized != "" {
				service.platformAdminEmails[normalized] = struct{}{}
			}
		}
	}
}

func WithEnvironmentControlPlane(controlPlane *environment.ControlPlane) Option {
	return func(service *Service) {
		service.environmentControl = controlPlane
	}
}

func WithAutomationExecutionPolicy(requirements []environment.CapabilityRequirement, packIDs map[string][]string) Option {
	return func(service *Service) {
		service.automationPolicy = make(map[string]environment.CapabilityRequirement, len(requirements))
		for _, requirement := range requirements {
			service.automationPolicy[requirement.ID] = requirement
		}
		service.automationPackIDs = make(map[string][]string, len(packIDs))
		for capabilityID, ids := range packIDs {
			service.automationPackIDs[capabilityID] = append([]string(nil), ids...)
		}
	}
}

func New(st store.Store, logger *slog.Logger, options ...Option) *Service {
	return NewWithBlob(st, logger, blob.NewMemory(), options...)
}

func NewWithBlob(st store.Store, logger *slog.Logger, blobs blob.Store, options ...Option) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if blobs == nil {
		blobs = blob.NewMemory()
	}
	service := &Service{store: st, now: time.Now, log: logger, blobs: blobs, platformAdminEmails: map[string]struct{}{}, automationPolicy: map[string]environment.CapabilityRequirement{}, automationPackIDs: map[string][]string{}}
	for _, option := range options {
		option(service)
	}
	return service
}

// newRegistration 校验注册凭据并构造用户记录，不写入存储。
func newRegistration(email, password, displayName string, now time.Time) (domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") || len(password) < 10 {
		return domain.User{}, domain.Invalid("REGISTRATION_INVALID", "邮箱无效或密码少于 10 位")
	}
	user := domain.User{ID: domain.NewID(), Email: email, DisplayName: strings.TrimSpace(displayName), PasswordHash: hashPassword(password), VerifiedAt: &now, CreatedAt: now}
	if user.DisplayName == "" {
		user.DisplayName = strings.Split(email, "@")[0]
	}
	return user, nil
}

func (s *Service) Register(ctx context.Context, email, password, displayName, tenantName string) (domain.Session, error) {
	now := s.now().UTC()
	user, err := newRegistration(email, password, displayName, now)
	if err != nil {
		return domain.Session{}, err
	}
	if err := s.store.CreateUser(ctx, user); err != nil {
		return domain.Session{}, err
	}
	tenant := domain.Tenant{ID: domain.NewID(), Slug: slugify(tenantName), Name: strings.TrimSpace(tenantName), Status: "active", CreatedAt: now}
	if tenant.Name == "" {
		tenant.Name = "我的内容团队"
		tenant.Slug = "my-team"
	}
	if err := s.store.CreateTenant(ctx, tenant, domain.Membership{TenantID: tenant.ID, UserID: user.ID, Role: "tenant_admin", Status: "active", CreatedAt: now}); err != nil {
		return domain.Session{}, err
	}
	session := domain.Session{ID: domain.NewID(), UserID: user.ID, TenantID: tenant.ID, ExpiresAt: now.Add(12 * time.Hour)}
	if err := s.store.SaveSession(ctx, session); err != nil {
		return domain.Session{}, err
	}
	s.audit(ctx, Actor{UserID: user.ID, TenantID: tenant.ID, Role: "tenant_admin", Type: "user"}, "", "tenant.created", "tenant", tenant.ID, "", map[string]any{"name": tenant.Name})
	return session, nil
}

// RegisterWithInvite 凭成员邀请令牌注册并直接加入邀请方租户，不创建新租户。
func (s *Service) RegisterWithInvite(ctx context.Context, email, password, displayName, inviteToken string) (domain.Session, error) {
	now := s.now().UTC()
	user, err := newRegistration(email, password, displayName, now)
	if err != nil {
		return domain.Session{}, err
	}
	invite, err := s.validateInviteToken(ctx, inviteToken, user.Email, now)
	if err != nil {
		return domain.Session{}, err
	}
	session := domain.Session{ID: domain.NewID(), UserID: user.ID, ExpiresAt: now.Add(12 * time.Hour)}
	session, membership, err := s.store.RegisterWithInvite(ctx, user, domain.TokenHash(inviteToken), session, now)
	if err != nil {
		return domain.Session{}, err
	}
	s.audit(ctx, Actor{UserID: user.ID, TenantID: session.TenantID, Role: membership.Role, Type: "user"}, "", "membership.invite_accepted", "membership", user.ID, "", map[string]any{"invite_id": invite.ID, "role": membership.Role})
	return session, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (domain.Session, error) {
	user, err := s.store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil || !checkPassword(user.PasswordHash, password) {
		return domain.Session{}, domain.E("authentication", "credentials", "AUTH_INVALID", "邮箱或密码错误", 3)
	}
	if user.VerifiedAt == nil {
		return domain.Session{}, domain.E("authentication", "email_verification", "EMAIL_NOT_VERIFIED", "邮箱尚未验证", 3)
	}
	tenants, err := s.store.TenantsForUser(ctx, user.ID)
	if err != nil || len(tenants) == 0 {
		return domain.Session{}, domain.E("authentication", "membership", "TENANT_REQUIRED", "用户没有可用租户", 3)
	}
	session := domain.Session{ID: domain.NewID(), UserID: user.ID, TenantID: tenants[0].ID, ExpiresAt: s.now().UTC().Add(12 * time.Hour)}
	if err := s.store.SaveSession(ctx, session); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (s *Service) SessionActor(ctx context.Context, sessionID string) (Actor, domain.User, error) {
	session, err := s.store.SessionByID(ctx, sessionID)
	if err != nil {
		return Actor{}, domain.User{}, domain.E("authentication", "session", "SESSION_INVALID", "会话无效或已过期", 3)
	}
	user, err := s.store.UserByID(ctx, session.UserID)
	if err != nil {
		return Actor{}, domain.User{}, err
	}
	m, err := s.store.Membership(ctx, session.TenantID, session.UserID)
	if err != nil {
		return Actor{}, domain.User{}, err
	}
	_, platformAdmin := s.platformAdminEmails[strings.ToLower(user.Email)]
	return Actor{UserID: user.ID, TenantID: session.TenantID, Role: m.Role, Type: "user", PlatformAdmin: platformAdmin}, user, nil
}

func (s *Service) Tenant(ctx context.Context, actor Actor) (domain.Tenant, error) {
	tenants, err := s.store.TenantsForUser(ctx, actor.UserID)
	if err != nil {
		return domain.Tenant{}, err
	}
	for _, t := range tenants {
		if t.ID == actor.TenantID {
			return t, nil
		}
	}
	return domain.Tenant{}, domain.NotFound("租户")
}

func (s *Service) Dashboard(ctx context.Context, actor Actor) (Dashboard, error) {
	tenant, err := s.Tenant(ctx, actor)
	if err != nil {
		return Dashboard{}, err
	}
	projects, err := s.store.Projects(ctx, actor.TenantID)
	if err != nil {
		return Dashboard{}, err
	}
	runs, _ := s.store.Runs(ctx, actor.TenantID, "")
	if len(runs) > 8 {
		runs = runs[:8]
	}
	audits, _ := s.store.AuditEvents(ctx, actor.TenantID, "", 8)
	counts := map[string]int{"projects": len(projects), "devices": 0, "knowledge_ready": 0, "open_blockers": 0, "review_ready": 0}
	for _, p := range projects {
		counts["devices"] += p.ConnectedDevices
		counts["knowledge_ready"] += p.KnowledgeReady
		counts["open_blockers"] += p.OpenBlockers
	}
	for _, r := range runs {
		if r.State == "succeeded" {
			counts["review_ready"]++
		}
	}
	pipeline := []PipelineStage{{"source", "资料就绪", len(projects), 0}, {"knowledge", "知识治理", counts["knowledge_ready"], counts["open_blockers"]}, {"submission", "待审提交", counts["review_ready"], 0}, {"approval", "客户批准", 0, 0}, {"delivery", "交付与结果", 0, 0}}
	return Dashboard{Tenant: tenant, Projects: projects, RecentRuns: runs, RecentAudit: audits, Counts: counts, Pipeline: pipeline}, nil
}

type CreateProjectInput struct {
	TemplateID     string `json:"template_id"`
	BrandName      string `json:"brand_name"`
	ProductName    string `json:"product_name"`
	ContentType    string `json:"content_type"`
	Channel        string `json:"channel"`
	StageObjective string `json:"stage_objective"`
	OwnerName      string `json:"owner_name"`
	ReviewerName   string `json:"reviewer_name"`
	ClientApprover string `json:"client_approver"`
}

func (s *Service) CreateProject(ctx context.Context, actor Actor, in CreateProjectInput, requestID string) (domain.Project, error) {
	if !canManage(actor.Role) {
		return domain.Project{}, domain.Policy("ROLE_DENIED", "当前角色不能创建项目", "联系租户管理员")
	}
	if strings.TrimSpace(in.BrandName) == "" || strings.TrimSpace(in.ProductName) == "" {
		return domain.Project{}, domain.Invalid("PROJECT_FIELDS_REQUIRED", "品牌名和单品名必填")
	}
	in.ContentType = defaultString(strings.TrimSpace(in.ContentType), domain.DefaultProjectContentType)
	if !domain.ValidTenantContentType(in.ContentType) {
		return domain.Project{}, domain.Invalid("PROJECT_CONTENT_TYPE_INVALID", "项目内容类型不受支持")
	}
	if in.TemplateID != "" {
		template, err := s.store.ProjectTemplate(ctx, actor.TenantID, in.TemplateID)
		if err != nil {
			return domain.Project{}, err
		}
		if strings.TrimSpace(in.Channel) == "" {
			in.Channel = template.Channel
		}
		if strings.TrimSpace(in.StageObjective) == "" {
			in.StageObjective = template.StageObjective
		}
	}
	now := s.now().UTC()
	p := domain.Project{ID: domain.NewID(), TenantID: actor.TenantID, Slug: slugify(in.BrandName + "-" + in.ProductName), BrandName: strings.TrimSpace(in.BrandName), ProductName: strings.TrimSpace(in.ProductName), ContentType: in.ContentType, Channel: defaultString(in.Channel, "douyin"), StageObjective: in.StageObjective, Status: "draft", OwnerName: in.OwnerName, ReviewerName: in.ReviewerName, ClientApprover: in.ClientApprover, RowVersion: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateProject(ctx, p); err != nil {
		return p, err
	}
	if _, _, err := s.ProjectSOP(ctx, actor, p.ID); err != nil {
		return p, err
	}
	s.audit(ctx, actor, p.ID, "project.created", "project", p.ID, requestID, map[string]any{"brand_name": p.BrandName, "product_name": p.ProductName, "content_type": p.ContentType})
	return p, nil
}
func (s *Service) Projects(ctx context.Context, actor Actor) ([]domain.Project, error) {
	return s.store.Projects(ctx, actor.TenantID)
}
func (s *Service) Project(ctx context.Context, actor Actor, id string) (domain.Project, error) {
	return s.store.Project(ctx, actor.TenantID, id)
}

func (s *Service) CreateConnectSession(ctx context.Context, actor Actor, projectID, requestID string) (domain.ConnectSession, error) {
	if !canManage(actor.Role) {
		return domain.ConnectSession{}, domain.Policy("ROLE_DENIED", "当前角色不能连接设备", "联系项目负责人")
	}
	if _, err := s.projectForWrite(ctx, actor, projectID); err != nil {
		return domain.ConnectSession{}, err
	}
	now := s.now().UTC()
	v := domain.ConnectSession{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: projectID, InviterUserID: actor.UserID, State: "waiting_for_computer", ExpiresAt: now.Add(10 * time.Minute)}
	if err := s.store.CreateConnectSession(ctx, v); err != nil {
		return v, err
	}
	s.audit(ctx, actor, projectID, "connect_session.created", "connect_session", v.ID, requestID, map[string]any{"expires_at": v.ExpiresAt})
	return v, nil
}
func (s *Service) ConnectSession(ctx context.Context, actor Actor, id string) (domain.ConnectSession, error) {
	session, err := s.store.ConnectSessionByID(ctx, actor.TenantID, id)
	if err != nil {
		return session, err
	}
	if session.State == "waiting_for_computer" && s.now().UTC().After(session.ExpiresAt) {
		session.State = "expired"
		if err := s.store.SaveConnectSession(ctx, session); err != nil {
			return session, err
		}
	}
	progress, err := s.store.BootstrapProgressForSession(ctx, actor.TenantID, id)
	if err != nil {
		return session, err
	}
	if session.State != "expired" {
		session.Progress = progress
	}
	return session, nil
}

type ConnectDeviceInput struct {
	DisplayName  string              `json:"display_name"`
	Hostname     string              `json:"hostname"`
	Platform     string              `json:"platform"`
	Arch         string              `json:"arch"`
	Version      string              `json:"version"`
	Capabilities []domain.Capability `json:"capabilities"`
}
type ConnectDeviceResult struct {
	Device              domain.Device         `json:"device"`
	DeviceToken         string                `json:"device_token"`
	WorkspaceID         string                `json:"workspace_id"`
	WorkspaceToken      string                `json:"workspace_token"`
	ProjectID           string                `json:"project_id"`
	EnvironmentManifest *environment.Manifest `json:"environment_manifest,omitempty"`
	BootstrapAttemptID  string                `json:"bootstrap_attempt_id,omitempty"`
}

func (s *Service) DeviceActor(ctx context.Context, token string) (Actor, domain.Device, error) {
	if !strings.HasPrefix(token, "dt_") {
		return Actor{}, domain.Device{}, domain.E("authentication", "device", "DEVICE_TOKEN_INVALID", "设备凭据无效", 3)
	}
	d, err := s.store.DeviceByTokenHash(ctx, domain.TokenHash(token))
	if err != nil {
		return Actor{}, d, domain.E("authentication", "device", "DEVICE_TOKEN_INVALID", "设备凭据无效", 3)
	}
	return Actor{UserID: d.OwnerUserID, TenantID: d.TenantID, Type: "device", DeviceID: d.ID}, d, nil
}

func (s *Service) WorkspaceActor(ctx context.Context, token string) (Actor, domain.WorkspaceBinding, error) {
	if !strings.HasPrefix(token, "wt_") {
		return Actor{}, domain.WorkspaceBinding{}, domain.E("authentication", "workspace", "WORKSPACE_TOKEN_INVALID", "工作区凭据无效", 3)
	}
	binding, err := s.store.WorkspaceBindingByTokenHash(ctx, domain.TokenHash(token))
	if err != nil {
		return Actor{}, binding, domain.E("authentication", "workspace", "WORKSPACE_TOKEN_INVALID", "工作区凭据无效", 3)
	}
	actor := Actor{UserID: binding.OwnerUserID, TenantID: binding.TenantID, Role: "editor", Type: "workspace", DeviceID: binding.DeviceID, WorkspaceID: binding.ID}
	binding.CredentialHash = ""
	return actor, binding, nil
}
func (s *Service) Devices(ctx context.Context, actor Actor, projectID string) ([]domain.Device, error) {
	return s.store.Devices(ctx, actor.TenantID, projectID)
}

func (s *Service) Approvals(ctx context.Context, actor Actor, subjectID string) ([]domain.ApprovalDecision, error) {
	return s.store.Approvals(ctx, actor.TenantID, subjectID)
}

func (s *Service) Runs(ctx context.Context, actor Actor, projectID string) ([]domain.TaskRun, error) {
	return s.store.Runs(ctx, actor.TenantID, projectID)
}
func (s *Service) Run(ctx context.Context, actor Actor, id string) (domain.TaskRun, error) {
	return s.store.Run(ctx, actor.TenantID, id)
}

type Lease struct {
	Run             domain.TaskRun                       `json:"run"`
	Attempt         domain.RunAttempt                    `json:"attempt"`
	Contract        domain.TaskContract                  `json:"contract"`
	ExecutionBundle *environment.CreativeExecutionBundle `json:"execution_bundle,omitempty"`
	LeaseExpiresAt  time.Time                            `json:"lease_expires_at"`
	RunToken        string                               `json:"run_token"`
}

func (s *Service) Poll(ctx context.Context, actor Actor, device domain.Device, caps []domain.Capability) (Lease, error) {
	return s.PollWithRuntime(ctx, actor, device, caps, nil, "")
}

func (s *Service) PollWithEnvironment(ctx context.Context, actor Actor, device domain.Device, caps []domain.Capability, claims []AutomationEnvironmentClaim) (Lease, error) {
	return s.PollWithRuntime(ctx, actor, device, caps, claims, "")
}

func (s *Service) PollWithRuntime(ctx context.Context, actor Actor, device domain.Device, caps []domain.Capability, claims []AutomationEnvironmentClaim, daemonVersion string) (Lease, error) {
	now := s.now().UTC()
	device.LastSeenAt = now
	device.Capabilities = caps
	if daemonVersion = strings.TrimSpace(daemonVersion); daemonVersion != "" {
		device.Version = daemonVersion
	}
	_ = s.store.SaveDevice(ctx, device)
	if err := s.store.ExpireRunAttempts(ctx, actor.TenantID, now); err != nil {
		return Lease{}, err
	}
	eligible, bundles, snapshots, err := s.automationLeaseCandidates(ctx, actor, caps, claims, now)
	if err != nil {
		return Lease{}, err
	}
	runToken, runTokenHash, err := domain.NewOpaqueToken("rt_", 32)
	if err != nil {
		return Lease{}, err
	}
	run, attempt, err := s.store.LeaseNextRun(ctx, actor.TenantID, device.ID, eligible, domain.NewID(), runTokenHash, now)
	if err != nil {
		return Lease{}, err
	}
	snapshot, ok := snapshots[run.ID]
	if !ok {
		snapshot, err = s.store.Snapshot(ctx, actor.TenantID, run.InputSnapshotID)
		if err != nil {
			return Lease{}, err
		}
	}
	project, _ := s.store.Project(ctx, actor.TenantID, run.ProjectID)
	capability := domain.Capability{ID: attempt.CapabilityID, Version: attempt.CapabilityVersion, Kind: "business_capability", InputSchema: attempt.InputSchema, OutputSchema: attempt.OutputSchema, Digest: attempt.CapabilityDigest, LocalOnly: true}
	contract := domain.TaskContract{ContractVersion: "1.0", ContractID: snapshot.ID, RunID: run.ID, TaskType: run.TaskType, Project: project, Sources: snapshot.Sources, InputSnapshotID: snapshot.ID, OutputSchema: run.OutputSchema, Capability: capability, ManifestHash: snapshot.ManifestHash}
	lease := Lease{Run: run, Attempt: attempt, Contract: contract, LeaseExpiresAt: *run.LeaseExpiresAt, RunToken: runToken}
	if bundle, exists := bundles[run.ID]; exists {
		bundleCopy := bundle
		lease.ExecutionBundle = &bundleCopy
	}
	return lease, nil
}

func (s *Service) HeartbeatRun(ctx context.Context, actor Actor, device domain.Device, runID, attemptID, runToken string, heartbeat domain.RunHeartbeat, requestID string) (domain.TaskRun, error) {
	run, err := s.store.Run(ctx, actor.TenantID, runID)
	if err != nil {
		return run, err
	}
	now := s.now().UTC()
	attempt, err := s.activeRunAttempt(ctx, actor, device, run, attemptID, runToken, now)
	if err != nil {
		return run, err
	}
	if heartbeat.Sequence <= run.HeartbeatSequence {
		return run, domain.Conflict("HEARTBEAT_SEQUENCE_INVALID", "心跳序号必须单调递增")
	}
	if strings.TrimSpace(heartbeat.Phase) == "" || strings.TrimSpace(heartbeat.Label) == "" || heartbeat.Step < 0 {
		return run, domain.Invalid("HEARTBEAT_PROGRESS_INVALID", "心跳必须包含 phase、非负 step 和可展示 label")
	}
	run.HeartbeatSequence = heartbeat.Sequence
	run.State = "running"
	run.ProgressLabel = heartbeat.Label
	leaseUntil := now.Add(5 * time.Minute)
	run.LeaseExpiresAt = &leaseUntil
	run.UpdatedAt = now
	attempt.State = "running"
	attempt.HeartbeatAt = &now
	attempt.LeaseExpiresAt = leaseUntil
	if attempt.StartedAt == nil {
		attempt.StartedAt = &now
	}
	if err := s.store.SaveRunAttempt(ctx, attempt); err != nil {
		return run, err
	}
	if err := s.store.SaveRun(ctx, run); err != nil {
		return run, err
	}
	if _, progressErr := s.store.AppendRunProgress(ctx, domain.RunProgressEvent{TenantID: actor.TenantID, ProjectID: run.ProjectID, RunID: run.ID, AttemptID: attempt.ID, DeviceID: device.ID, Sequence: heartbeat.Sequence, Phase: strings.TrimSpace(heartbeat.Phase), Step: heartbeat.Step, Label: strings.TrimSpace(heartbeat.Label), OccurredAt: now}); progressErr != nil {
		s.log.Warn("append run progress", "run_id", run.ID, "attempt_id", attempt.ID, "error", progressErr)
	}
	return run, nil
}

func (s *Service) CancelRun(ctx context.Context, actor Actor, runID, requestID string) (domain.TaskRun, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.TaskRun{}, err
	}
	run, err := s.store.Run(ctx, actor.TenantID, runID)
	if err != nil {
		return run, err
	}
	if run.State == "succeeded" || run.State == "failed" || run.State == "canceled" {
		return run, domain.Conflict("RUN_TERMINAL", "终态任务不能取消")
	}
	now := s.now().UTC()
	if run.State == "queued" {
		run.State = "canceled"
		run.ProgressLabel = "已取消"
	} else {
		run.CancelRequestedAt = &now
		run.ProgressLabel = "等待客户端取消"
	}
	run.UpdatedAt = now
	if err := s.store.SaveRun(ctx, run); err != nil {
		return run, err
	}
	s.audit(ctx, actor, run.ProjectID, "run.cancel_requested", "task_run", run.ID, requestID, map[string]any{"state": run.State})
	return run, nil
}

func (s *Service) Audit(ctx context.Context, actor Actor, projectID string, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.store.AuditEvents(ctx, actor.TenantID, projectID, limit)
}

func (s *Service) audit(ctx context.Context, actor Actor, projectID, action, subjectType, subjectID, requestID string, summary map[string]any) {
	if requestID == "" {
		requestID = "req_" + domain.NewID()
	}
	event := domain.AuditEvent{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: projectID, ActorType: defaultString(actor.Type, "user"), ActorID: actor.UserID, Action: action, SubjectType: subjectType, SubjectID: subjectID, Summary: summary, RequestID: requestID, CreatedAt: s.now().UTC()}
	if err := s.store.AppendAudit(ctx, event); err != nil {
		s.log.Error("append audit", "error", err, "action", action)
	}
}

func canManage(role string) bool { return role == "tenant_admin" || role == "project_manager" }
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func slugify(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	parts := strings.FieldsFunc(v, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= '一' && r <= '龥') })
	if len(parts) == 0 {
		return "project-" + strings.Split(domain.NewID(), "-")[0]
	}
	return strings.Join(parts, "-")
}

func hashPassword(password string) string {
	salt := []byte(domain.NewID())
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("%s.%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
}
func checkPassword(encoded, password string) bool {
	parts := strings.Split(encoded, ".")
	if len(parts) != 2 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[0])
	expected, err2 := base64.RawStdEncoding.DecodeString(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
