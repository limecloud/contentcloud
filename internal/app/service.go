package app

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/limecloud/contentcloud/internal/blob"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store"
)

type Service struct {
	store store.Store
	now   func() time.Time
	log   *slog.Logger
	blobs blob.Store
}

type Actor struct {
	UserID      string
	TenantID    string
	Role        string
	Type        string
	DeviceID    string
	WorkspaceID string
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

func New(st store.Store, logger *slog.Logger) *Service {
	return NewWithBlob(st, logger, blob.NewMemory())
}

func NewWithBlob(st store.Store, logger *slog.Logger, blobs blob.Store) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if blobs == nil {
		blobs = blob.NewMemory()
	}
	return &Service{store: st, now: time.Now, log: logger, blobs: blobs}
}

func (s *Service) Register(ctx context.Context, email, password, displayName, tenantName string) (domain.Session, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") || len(password) < 10 {
		return domain.Session{}, domain.Invalid("REGISTRATION_INVALID", "邮箱无效或密码少于 10 位")
	}
	now := s.now().UTC()
	user := domain.User{ID: domain.NewID(), Email: email, DisplayName: strings.TrimSpace(displayName), PasswordHash: hashPassword(password), VerifiedAt: &now, CreatedAt: now}
	if user.DisplayName == "" {
		user.DisplayName = strings.Split(email, "@")[0]
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
	return Actor{UserID: user.ID, TenantID: session.TenantID, Role: m.Role, Type: "user"}, user, nil
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
	pipeline := []PipelineStage{{"source", "资料就绪", len(projects), 0}, {"knowledge", "知识治理", counts["knowledge_ready"], counts["open_blockers"]}, {"brief", "Brief", 0, 0}, {"script", "剧本审核", counts["review_ready"], 0}, {"approval", "客户批准", 0, 0}}
	return Dashboard{Tenant: tenant, Projects: projects, RecentRuns: runs, RecentAudit: audits, Counts: counts, Pipeline: pipeline}, nil
}

type CreateProjectInput struct {
	TemplateID     string `json:"template_id"`
	BrandName      string `json:"brand_name"`
	ProductName    string `json:"product_name"`
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
	p := domain.Project{ID: domain.NewID(), TenantID: actor.TenantID, Slug: slugify(in.BrandName + "-" + in.ProductName), BrandName: strings.TrimSpace(in.BrandName), ProductName: strings.TrimSpace(in.ProductName), Channel: defaultString(in.Channel, "douyin"), StageObjective: in.StageObjective, Status: "draft", OwnerName: in.OwnerName, ReviewerName: in.ReviewerName, ClientApprover: in.ClientApprover, RowVersion: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateProject(ctx, p); err != nil {
		return p, err
	}
	s.audit(ctx, actor, p.ID, "project.created", "project", p.ID, requestID, map[string]any{"brand_name": p.BrandName, "product_name": p.ProductName})
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
	plain, hash, err := domain.NewOpaqueToken("cck_", 24)
	if err != nil {
		return domain.ConnectSession{}, err
	}
	now := s.now().UTC()
	v := domain.ConnectSession{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: projectID, InviterUserID: actor.UserID, ConnectKeyHash: hash, State: "waiting_for_computer", ExpiresAt: now.Add(10 * time.Minute), PlaintextConnectKey: plain}
	stored := v
	stored.PlaintextConnectKey = ""
	if err := s.store.CreateConnectSession(ctx, stored); err != nil {
		return v, err
	}
	s.audit(ctx, actor, projectID, "connect_session.created", "connect_session", v.ID, requestID, map[string]any{"expires_at": v.ExpiresAt})
	return v, nil
}
func (s *Service) ConnectSession(ctx context.Context, actor Actor, id string) (domain.ConnectSession, error) {
	return s.store.ConnectSessionByID(ctx, actor.TenantID, id)
}

type ConnectDeviceInput struct {
	ConnectKey   string              `json:"connect_key"`
	DisplayName  string              `json:"display_name"`
	Hostname     string              `json:"hostname"`
	Platform     string              `json:"platform"`
	Arch         string              `json:"arch"`
	Version      string              `json:"version"`
	Capabilities []domain.Capability `json:"capabilities"`
}
type ConnectDeviceResult struct {
	Device         domain.Device `json:"device"`
	DeviceToken    string        `json:"device_token"`
	WorkspaceID    string        `json:"workspace_id"`
	WorkspaceToken string        `json:"workspace_token"`
	ProjectID      string        `json:"project_id"`
}

func (s *Service) ConnectDevice(ctx context.Context, in ConnectDeviceInput) (ConnectDeviceResult, error) {
	if !strings.HasPrefix(in.ConnectKey, "cck_") {
		return ConnectDeviceResult{}, domain.Invalid("CONNECT_KEY_INVALID", "连接码格式错误")
	}
	token, tokenHash, err := domain.NewOpaqueToken("dt_", 32)
	if err != nil {
		return ConnectDeviceResult{}, err
	}
	workspaceToken, workspaceTokenHash, err := domain.NewOpaqueToken("wt_", 32)
	if err != nil {
		return ConnectDeviceResult{}, err
	}
	now := s.now().UTC()
	d := domain.Device{ID: domain.NewID(), DisplayName: defaultString(in.DisplayName, in.Hostname), Hostname: in.Hostname, Platform: defaultString(in.Platform, runtime.GOOS), Arch: defaultString(in.Arch, runtime.GOARCH), Version: in.Version, TokenHash: tokenHash, Capabilities: in.Capabilities, LastSeenAt: now}
	workspace := domain.WorkspaceBinding{ID: domain.NewID(), TemplateID: "workspace_marketing_video", TemplateVersion: "", Targets: []string{}, CredentialHash: workspaceTokenHash, Status: "active", InitializedAt: now, LastSeenAt: now}
	session, err := s.store.ConsumeConnectSession(ctx, domain.TokenHash(in.ConnectKey), d, workspace, now)
	if err != nil {
		return ConnectDeviceResult{}, err
	}
	d.TenantID = session.TenantID
	d.OwnerUserID = session.InviterUserID
	d.ProjectIDs = []string{session.ProjectID}
	s.audit(ctx, Actor{UserID: d.OwnerUserID, TenantID: d.TenantID, Type: "device", DeviceID: d.ID}, session.ProjectID, "device.connected", "device", d.ID, "", map[string]any{"platform": d.Platform})
	d.TokenHash = ""
	return ConnectDeviceResult{Device: d, DeviceToken: token, WorkspaceID: workspace.ID, WorkspaceToken: workspaceToken, ProjectID: session.ProjectID}, nil
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
	return Actor{UserID: binding.OwnerUserID, TenantID: binding.TenantID, Role: "editor", Type: "workspace", DeviceID: binding.DeviceID, WorkspaceID: binding.ID}, binding, nil
}
func (s *Service) Devices(ctx context.Context, actor Actor, projectID string) ([]domain.Device, error) {
	return s.store.Devices(ctx, actor.TenantID, projectID)
}

type CreateKnowledgeInput struct {
	ProjectID           string                `json:"project_id"`
	Kind                string                `json:"kind"`
	Title               string                `json:"title"`
	Statement           string                `json:"statement"`
	Subject             string                `json:"subject"`
	Predicate           string                `json:"predicate"`
	Value               domain.TypedValue     `json:"value"`
	Scope               domain.KnowledgeScope `json:"scope"`
	RiskLevel           string                `json:"risk_level"`
	AllowedChannels     []string              `json:"allowed_channels"`
	Evidence            []domain.EvidenceRef  `json:"evidence"`
	ForbiddenExtensions []string              `json:"forbidden_extensions"`
	DependsOnFactIDs    []string              `json:"depends_on_fact_ids"`
	ValidFrom           *time.Time            `json:"valid_from"`
	ValidUntil          *time.Time            `json:"valid_until"`
	ExpiresAt           *time.Time            `json:"expires_at"`
	OriginRunID         string                `json:"-"`
}

func (s *Service) CreateKnowledge(ctx context.Context, actor Actor, in CreateKnowledgeInput, requestID string) (domain.KnowledgeItem, error) {
	return s.createKnowledge(ctx, actor, in, requestID, false)
}

func (s *Service) createKnowledge(ctx context.Context, actor Actor, in CreateKnowledgeInput, requestID string, fromRun bool) (domain.KnowledgeItem, error) {
	if fromRun && actor.Type != "device" {
		return domain.KnowledgeItem{}, domain.Policy("RUN_ACTOR_REQUIRED", "只有已认证设备可以导入知识提取结果", "通过 ContentCloud Daemon 报告任务")
	}
	if !fromRun {
		if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer"); err != nil {
			return domain.KnowledgeItem{}, err
		}
	}
	if _, err := s.projectForWrite(ctx, actor, in.ProjectID); err != nil {
		return domain.KnowledgeItem{}, err
	}
	if err := s.validateKnowledgeInput(ctx, actor.TenantID, in); err != nil {
		return domain.KnowledgeItem{}, err
	}
	now := s.now().UTC()
	status := "needs_review"
	if len(in.Evidence) == 0 {
		status = "candidate"
	}
	value := in.Value
	if value.Type == "" {
		value = domain.TypedValue{Type: "text", Text: in.Statement}
	}
	subject := defaultString(strings.TrimSpace(in.Subject), strings.TrimSpace(in.Title))
	predicate := defaultString(strings.TrimSpace(in.Predicate), defaultString(in.Kind, "fact"))
	v := domain.KnowledgeItem{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: in.ProjectID, Kind: defaultString(in.Kind, "fact"), Title: in.Title, Statement: in.Statement, Subject: subject, Predicate: predicate, Value: value, Scope: in.Scope, Status: status, RiskLevel: defaultString(in.RiskLevel, "medium"), AllowedChannels: uniqueNonEmpty(in.AllowedChannels), Evidence: in.Evidence, ForbiddenExtensions: uniqueNonEmpty(in.ForbiddenExtensions), DependsOnFactIDs: uniqueNonEmpty(in.DependsOnFactIDs), ValidFrom: in.ValidFrom, ValidUntil: in.ValidUntil, ExpiresAt: in.ExpiresAt, OriginRunID: in.OriginRunID, DecisionRequired: true, RowVersion: 1, CreatedAt: now, UpdatedAt: now}
	existing, err := s.store.Knowledge(ctx, actor.TenantID, in.ProjectID)
	if err != nil {
		return v, err
	}
	conflictingIDs := []string{}
	valueHash, _ := domain.CanonicalHash(v.Value)
	for index := range existing {
		item := existing[index]
		if item.Subject != v.Subject || item.Predicate != v.Predicate || item.Status == "rejected" || item.Status == "expired" {
			continue
		}
		existingHash, _ := domain.CanonicalHash(item.Value)
		if existingHash == valueHash {
			continue
		}
		conflictingIDs = append(conflictingIDs, item.ID)
		if item.Status == "approved" {
			item.Status = "review_required"
		} else {
			item.Status = "conflicted"
		}
		item.RowVersion++
		item.UpdatedAt = now
		if err := s.store.SaveKnowledge(ctx, item); err != nil {
			return v, err
		}
	}
	if len(conflictingIDs) > 0 {
		v.Status = "conflicted"
	}
	if err := s.store.CreateKnowledge(ctx, v); err != nil {
		return v, err
	}
	if len(conflictingIDs) > 0 {
		conflictingIDs = append(conflictingIDs, v.ID)
		conflict := domain.KnowledgeConflict{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: v.ProjectID, Subject: v.Subject, Predicate: v.Predicate, KnowledgeItemIDs: conflictingIDs, Reason: "同一 subject/predicate 存在不同有效值", Status: "open", CreatedAt: now, UpdatedAt: now}
		decision := domain.DecisionRequest{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: v.ProjectID, ConflictID: conflict.ID, Question: "请确认“" + v.Subject + " / " + v.Predicate + "”应采用哪个值", KnowledgeItemIDs: conflictingIDs, Status: "open", RequestedBy: actor.UserID, CreatedAt: now}
		if err := s.store.CreateKnowledgeConflict(ctx, conflict, decision); err != nil {
			return v, err
		}
	}
	s.audit(ctx, actor, v.ProjectID, "knowledge.created", "knowledge_item", v.ID, requestID, map[string]any{"status": v.Status, "kind": v.Kind, "origin_run_id": v.OriginRunID})
	return v, nil
}

func (s *Service) validateKnowledgeInput(ctx context.Context, tenantID string, in CreateKnowledgeInput) error {
	if in.Title == "" || in.Statement == "" {
		return domain.Invalid("KNOWLEDGE_FIELDS_REQUIRED", "知识标题和陈述必填")
	}
	if err := validateKnowledgeValue(in.Value); err != nil {
		return err
	}
	if in.ValidFrom != nil && in.ValidUntil != nil && !in.ValidUntil.After(*in.ValidFrom) {
		return domain.Invalid("KNOWLEDGE_PERIOD_INVALID", "知识有效期结束时间必须晚于开始时间")
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(s.now().UTC()) {
		return domain.Invalid("KNOWLEDGE_EXPIRY_INVALID", "新知识的过期时间必须晚于当前时间")
	}
	for _, dependencyID := range in.DependsOnFactIDs {
		dependency, err := s.store.KnowledgeItem(ctx, tenantID, dependencyID)
		if err != nil || dependency.ProjectID != in.ProjectID || dependency.Kind != "fact" || !knowledgeActive(dependency, "", s.now().UTC()) {
			return domain.Policy("KNOWLEDGE_DEPENDENCY_BLOCKED", "依赖事实不存在、未批准或已失效", "选择当前项目内有效的已批准事实")
		}
	}
	if len(in.Evidence) > 0 {
		if err := s.validateKnowledgeEvidence(ctx, tenantID, in.ProjectID, in.Evidence); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Knowledge(ctx context.Context, actor Actor, projectID string) ([]domain.KnowledgeItem, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	items, err := s.store.Knowledge(ctx, actor.TenantID, projectID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	for index := range items {
		if items[index].Status == "approved" && !knowledgeWithinTime(items[index], now) {
			items[index].Status = "expired"
			items[index].RowVersion++
			items[index].UpdatedAt = now
			_ = s.store.SaveKnowledge(ctx, items[index])
		}
	}
	return items, nil
}
func (s *Service) KnowledgeItem(ctx context.Context, actor Actor, id string) (domain.KnowledgeItem, error) {
	return s.store.KnowledgeItem(ctx, actor.TenantID, id)
}
func (s *Service) ReviewKnowledge(ctx context.Context, actor Actor, id, decision, requestID string) (domain.KnowledgeItem, error) {
	if err := requireRole(actor, "tenant_admin", "reviewer"); err != nil {
		return domain.KnowledgeItem{}, err
	}
	v, err := s.store.KnowledgeItem(ctx, actor.TenantID, id)
	if err != nil {
		return v, err
	}
	if _, err := s.projectForWrite(ctx, actor, v.ProjectID); err != nil {
		return v, err
	}
	prev := v.Status
	switch decision {
	case "approve":
		if len(v.Evidence) == 0 {
			return v, domain.Policy("EVIDENCE_REQUIRED", "正式知识缺少已验收证据", "补充来源定位后再批准")
		}
		if err := s.validateKnowledgeEvidence(ctx, actor.TenantID, v.ProjectID, v.Evidence); err != nil {
			return v, err
		}
		if !knowledgeWithinTime(v, s.now().UTC()) {
			return v, domain.Policy("KNOWLEDGE_NOT_EFFECTIVE", "知识尚未生效或已过期", "调整有效期或创建新知识版本")
		}
		conflicts, err := s.store.KnowledgeConflicts(ctx, actor.TenantID, v.ProjectID)
		if err != nil {
			return v, err
		}
		for _, conflict := range conflicts {
			if conflict.Status == "open" && containsString(conflict.KnowledgeItemIDs, v.ID) {
				return v, domain.Policy("KNOWLEDGE_CONFLICT_OPEN", "该知识仍有未解决冲突", "先完成品牌事实决策请求")
			}
		}
		for _, dependencyID := range v.DependsOnFactIDs {
			dependency, err := s.store.KnowledgeItem(ctx, actor.TenantID, dependencyID)
			if err != nil || !knowledgeActive(dependency, "", s.now().UTC()) {
				return v, domain.Policy("KNOWLEDGE_DEPENDENCY_BLOCKED", "依赖事实已失效", "先复核依赖事实")
			}
		}
		v.Status = "approved"
		now := s.now().UTC()
		v.ApprovedBy = actor.UserID
		v.ApprovedAt = &now
	case "reject":
		v.Status = "rejected"
		v.ApprovedBy, v.ApprovedAt = "", nil
	case "conflict":
		v.Status = "conflicted"
	case "return":
		v.Status = "needs_review"
	default:
		return v, domain.Invalid("DECISION_INVALID", "审核决策无效")
	}
	v.RowVersion++
	v.UpdatedAt = s.now().UTC()
	if err := s.store.SaveKnowledge(ctx, v); err != nil {
		return v, err
	}
	p, _ := s.store.Project(ctx, actor.TenantID, v.ProjectID)
	if prev != "approved" && v.Status == "approved" {
		p.KnowledgeReady++
	}
	if prev == "approved" && v.Status != "approved" {
		p.KnowledgeReady--
	}
	p.UpdatedAt = v.UpdatedAt
	p.RowVersion++
	_ = s.store.SaveProject(ctx, p)
	s.audit(ctx, actor, v.ProjectID, "knowledge.reviewed", "knowledge_item", v.ID, requestID, map[string]any{"from": prev, "to": v.Status})
	return v, nil
}

func (s *Service) validateKnowledgeEvidence(ctx context.Context, tenantID, projectID string, refs []domain.EvidenceRef) error {
	for _, ref := range refs {
		if strings.TrimSpace(ref.SourceRevisionID) == "" || strings.TrimSpace(ref.LocatorKind) == "" || strings.TrimSpace(ref.Quote) == "" {
			return domain.Invalid("EVIDENCE_REF_INVALID", "证据引用缺少来源版本、定位类型或原文")
		}
		revision, err := s.store.SourceRevision(ctx, tenantID, ref.SourceRevisionID)
		if err != nil {
			return err
		}
		if revision.ProjectID != projectID {
			return domain.Policy("EVIDENCE_PROJECT_MISMATCH", "证据来源不属于当前项目", "选择当前项目内的已验收证据")
		}
		if revision.ProcessingStatus != "ready" {
			return domain.Policy("EVIDENCE_SOURCE_NOT_READY", "证据来源尚未完成可信解析", "等待来源状态变为 ready 并完成证据验收")
		}
		spans, err := s.store.Evidence(ctx, tenantID, revision.ID)
		if err != nil {
			return err
		}
		matched := false
		for _, span := range spans {
			if span.ProjectID == projectID && span.LocatorKind == ref.LocatorKind && evidenceLocatorMatches(ref.Locator, span.Locator) && span.QuoteText == ref.Quote && span.ReviewStatus == "accepted" {
				matched = true
				break
			}
		}
		if !matched {
			return domain.Policy("EVIDENCE_NOT_ACCEPTED", "证据原文或定位与已验收片段不一致", "重新选择来源中的已验收证据，不要手工改写原文")
		}
	}
	return nil
}

type CreateBriefInput struct {
	ProjectID              string   `json:"project_id"`
	Objective              string   `json:"objective"`
	Audience               string   `json:"audience"`
	DemandMoment           string   `json:"demand_moment"`
	Scene                  string   `json:"scene"`
	Conflict               string   `json:"conflict"`
	PrimarySellingPoint    string   `json:"primary_selling_point"`
	SecondarySellingPoints []string `json:"secondary_selling_points"`
	CTA                    string   `json:"cta"`
	Channel                string   `json:"channel"`
	AspectRatio            string   `json:"aspect_ratio"`
	EvidenceSummary        string   `json:"evidence_summary"`
	TargetDurationSeconds  int      `json:"target_duration_seconds"`
	PrimaryTestVariable    string   `json:"primary_test_variable"`
	ApprovedKnowledgeIDs   []string `json:"approved_knowledge_ids"`
	FrameworkIDs           []string `json:"framework_ids"`
	VisualizationPlanIDs   []string `json:"visualization_plan_ids"`
	Viewpoint              string   `json:"viewpoint"`
	Constraints            []string `json:"constraints"`
	SupersedesID           string   `json:"supersedes_id"`
	RevisionReason         string   `json:"revision_reason"`
}

func (s *Service) CreateBrief(ctx context.Context, actor Actor, in CreateBriefInput, requestID string) (domain.BriefVersion, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return domain.BriefVersion{}, err
	}
	p, err := s.projectForWrite(ctx, actor, in.ProjectID)
	if err != nil {
		return domain.BriefVersion{}, err
	}
	if strings.TrimSpace(in.Objective) == "" || strings.TrimSpace(in.Audience) == "" || strings.TrimSpace(in.DemandMoment) == "" || strings.TrimSpace(in.Scene) == "" || strings.TrimSpace(in.Conflict) == "" || strings.TrimSpace(in.PrimarySellingPoint) == "" || strings.TrimSpace(in.EvidenceSummary) == "" || strings.TrimSpace(in.CTA) == "" || strings.TrimSpace(in.PrimaryTestVariable) == "" {
		return domain.BriefVersion{}, domain.Invalid("BRIEF_FIELDS_REQUIRED", "目标、人群、需求时刻、场景、冲突、主卖点、证据摘要、CTA 和唯一测试变量必填")
	}
	if len(in.SecondarySellingPoints) > 2 {
		return domain.BriefVersion{}, domain.Invalid("BRIEF_SECONDARY_SELLING_POINTS_INVALID", "次卖点最多两个")
	}
	in.AspectRatio = defaultString(in.AspectRatio, "9:16")
	if in.AspectRatio != "9:16" {
		return domain.BriefVersion{}, domain.Invalid("BRIEF_ASPECT_RATIO_INVALID", "抖音 V1 Brief 画幅必须为 9:16")
	}
	if in.SupersedesID != "" {
		previous, err := s.store.Brief(ctx, actor.TenantID, in.SupersedesID)
		if err != nil || previous.ProjectID != p.ID {
			return domain.BriefVersion{}, domain.Policy("BRIEF_SUPERSEDES_INVALID", "被修订 Brief 不属于当前项目", "选择当前项目的历史 Brief")
		}
		if strings.TrimSpace(in.RevisionReason) == "" {
			return domain.BriefVersion{}, domain.Invalid("BRIEF_REVISION_REASON_REQUIRED", "修订 Brief 必须说明原因")
		}
	}
	if in.TargetDurationSeconds == 0 {
		in.TargetDurationSeconds = 30
	}
	if in.TargetDurationSeconds < 10 || in.TargetDurationSeconds > 120 {
		return domain.BriefVersion{}, domain.Invalid("BRIEF_DURATION_INVALID", "目标时长必须在 10 到 120 秒之间")
	}
	for _, id := range in.ApprovedKnowledgeIDs {
		k, err := s.store.KnowledgeItem(ctx, actor.TenantID, id)
		if err != nil || k.ProjectID != p.ID || !knowledgeActive(k, defaultString(in.Channel, p.Channel), s.now().UTC()) {
			return domain.BriefVersion{}, domain.Policy("KNOWLEDGE_BLOCKED", "Brief 引用了不可用知识", "先批准所有引用知识")
		}
	}
	if len(in.ApprovedKnowledgeIDs) == 0 {
		return domain.BriefVersion{}, domain.Policy("KNOWLEDGE_REQUIRED", "Brief 至少引用一条已批准知识", "先完成知识治理")
	}
	if len(in.FrameworkIDs) == 0 {
		return domain.BriefVersion{}, domain.Policy("FRAMEWORK_REQUIRED", "Brief 至少引用一个可用内容框架", "先完成对标拆解")
	}
	for _, id := range in.FrameworkIDs {
		framework, err := s.store.Framework(ctx, actor.TenantID, id)
		if err != nil || framework.ProjectID != p.ID || framework.Status != "approved" {
			return domain.BriefVersion{}, domain.Policy("FRAMEWORK_BLOCKED", "Brief 引用了不可用内容框架", "先批准内容框架")
		}
	}
	if len(in.VisualizationPlanIDs) == 0 {
		return domain.BriefVersion{}, domain.Policy("VISUALIZATION_PLAN_REQUIRED", "主卖点必须有已批准可视化方案", "先建立并审核画面证明方案")
	}
	for _, id := range in.VisualizationPlanIDs {
		plan, err := s.store.VisualizationPlan(ctx, actor.TenantID, id)
		if err != nil || plan.ProjectID != p.ID || plan.Status != "approved" {
			return domain.BriefVersion{}, domain.Policy("VISUALIZATION_PLAN_BLOCKED", "Brief 引用了不可用可视化方案", "先批准画面证明方案")
		}
		point, pointErr := s.store.SellingPoint(ctx, actor.TenantID, plan.SellingPointID)
		if pointErr != nil || point.Title != in.PrimarySellingPoint {
			return domain.BriefVersion{}, domain.Policy("VISUALIZATION_PLAN_SELLING_POINT_MISMATCH", "画面方案不属于当前主卖点", "选择主卖点对应的已批准画面方案")
		}
	}
	all, _ := s.store.Briefs(ctx, actor.TenantID, p.ID)
	now := s.now().UTC()
	v := domain.BriefVersion{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: p.ID, Version: len(all) + 1, Status: "draft", Objective: strings.TrimSpace(in.Objective), Audience: strings.TrimSpace(in.Audience), DemandMoment: strings.TrimSpace(in.DemandMoment), Scene: strings.TrimSpace(in.Scene), Conflict: strings.TrimSpace(in.Conflict), PrimarySellingPoint: strings.TrimSpace(in.PrimarySellingPoint), SecondarySellingPoints: uniqueNonEmpty(in.SecondarySellingPoints), CTA: strings.TrimSpace(in.CTA), Channel: defaultString(in.Channel, p.Channel), AspectRatio: in.AspectRatio, EvidenceSummary: strings.TrimSpace(in.EvidenceSummary), TargetDurationSeconds: in.TargetDurationSeconds, PrimaryTestVariable: strings.TrimSpace(in.PrimaryTestVariable), ApprovedKnowledgeIDs: uniqueNonEmpty(in.ApprovedKnowledgeIDs), FrameworkIDs: uniqueNonEmpty(in.FrameworkIDs), VisualizationPlanIDs: uniqueNonEmpty(in.VisualizationPlanIDs), Viewpoint: defaultString(in.Viewpoint, "user"), Constraints: uniqueNonEmpty(in.Constraints), SupersedesID: in.SupersedesID, RevisionReason: strings.TrimSpace(in.RevisionReason), CreatedBy: actor.UserID, CreatedAt: now}
	v.ContentHash, _ = domain.CanonicalHash(v)
	if err := s.store.CreateBrief(ctx, v); err != nil {
		return v, err
	}
	s.audit(ctx, actor, v.ProjectID, "brief.created", "brief_version", v.ID, requestID, map[string]any{"version": v.Version})
	return v, nil
}
func (s *Service) Briefs(ctx context.Context, actor Actor, projectID string) ([]domain.BriefVersion, error) {
	return s.store.Briefs(ctx, actor.TenantID, projectID)
}
func (s *Service) Brief(ctx context.Context, actor Actor, id string) (domain.BriefVersion, error) {
	return s.store.Brief(ctx, actor.TenantID, id)
}
func (s *Service) Approvals(ctx context.Context, actor Actor, subjectID string) ([]domain.ApprovalDecision, error) {
	return s.store.Approvals(ctx, actor.TenantID, subjectID)
}
func (s *Service) ReviewBrief(ctx context.Context, actor Actor, id, decision, requestID string) (domain.BriefVersion, error) {
	return s.ReviewBriefWithReason(ctx, actor, id, decision, "", requestID)
}

func (s *Service) ReviewBriefWithReason(ctx context.Context, actor Actor, id, decision, reason, requestID string) (domain.BriefVersion, error) {
	v, err := s.store.Brief(ctx, actor.TenantID, id)
	if err != nil {
		return v, err
	}
	if _, err := s.projectForWrite(ctx, actor, v.ProjectID); err != nil {
		return v, err
	}
	prev := v.Status
	switch decision {
	case "submit":
		if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
			return v, err
		}
		if v.Status != "draft" && v.Status != "revision_requested" {
			return v, domain.Conflict("BRIEF_STATE_INVALID", "当前状态不能提交")
		}
		v.Status = "internal_review"
	case "approve":
		if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer"); err != nil {
			return v, err
		}
		if v.Status != "internal_review" {
			return v, domain.Conflict("BRIEF_STATE_INVALID", "只有内审中的 Brief 可批准")
		}
		if err := s.validateBriefDependencies(ctx, actor.TenantID, v); err != nil {
			return v, err
		}
		v.Status = "approved"
		now := s.now().UTC()
		v.ApprovedBy = actor.UserID
		v.ApprovedAt = &now
	case "return":
		if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer"); err != nil {
			return v, err
		}
		if strings.TrimSpace(reason) == "" {
			return v, domain.Invalid("BRIEF_RETURN_REASON_REQUIRED", "退回 Brief 必须填写原因")
		}
		v.Status = "revision_requested"
	default:
		return v, domain.Invalid("DECISION_INVALID", "审核决策无效")
	}
	if err := s.store.SaveBrief(ctx, v); err != nil {
		return v, err
	}
	if decision == "approve" {
		approval := domain.ApprovalDecision{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: v.ProjectID, SubjectType: "brief_version", SubjectID: v.ID, SubjectHash: v.ContentHash, ActorID: actor.UserID, Decision: "approve", Reason: strings.TrimSpace(reason), PreviousState: prev, ResultingState: v.Status, CreatedAt: s.now().UTC()}
		if err := s.store.CreateApproval(ctx, approval); err != nil {
			return v, err
		}
		if err := s.supersedeBriefAndScripts(ctx, actor, v, requestID); err != nil {
			return v, err
		}
	}
	s.audit(ctx, actor, v.ProjectID, "brief.reviewed", "brief_version", v.ID, requestID, map[string]any{"from": prev, "to": v.Status})
	return v, nil
}

func (s *Service) validateBriefDependencies(ctx context.Context, tenantID string, brief domain.BriefVersion) error {
	now := s.now().UTC()
	for _, id := range brief.ApprovedKnowledgeIDs {
		item, err := s.store.KnowledgeItem(ctx, tenantID, id)
		if err != nil || item.ProjectID != brief.ProjectID || !knowledgeActive(item, brief.Channel, now) {
			return domain.Policy("KNOWLEDGE_BLOCKED", "Brief 上游知识已失效", "复核知识后创建新 Brief 版本")
		}
	}
	for _, id := range brief.FrameworkIDs {
		framework, err := s.store.Framework(ctx, tenantID, id)
		if err != nil || framework.ProjectID != brief.ProjectID || framework.Status != "approved" {
			return domain.Policy("FRAMEWORK_BLOCKED", "Brief 引用框架已失效", "复核内容框架后重试")
		}
	}
	for _, id := range brief.VisualizationPlanIDs {
		plan, err := s.store.VisualizationPlan(ctx, tenantID, id)
		if err != nil || plan.ProjectID != brief.ProjectID || plan.Status != "approved" {
			return domain.Policy("VISUALIZATION_PLAN_BLOCKED", "Brief 画面方案已失效", "复核画面方案后重试")
		}
	}
	return nil
}

func (s *Service) supersedeBriefAndScripts(ctx context.Context, actor Actor, approved domain.BriefVersion, requestID string) error {
	if approved.SupersedesID == "" {
		return nil
	}
	previous, err := s.store.Brief(ctx, actor.TenantID, approved.SupersedesID)
	if err != nil {
		return err
	}
	previous.Status = "superseded"
	if err := s.store.SaveBrief(ctx, previous); err != nil {
		return err
	}
	scripts, err := s.store.Scripts(ctx, actor.TenantID, approved.ProjectID)
	if err != nil {
		return err
	}
	affected := 0
	for _, script := range scripts {
		run, runErr := s.store.Run(ctx, actor.TenantID, script.RunID)
		if runErr != nil || run.BriefVersionID != previous.ID || script.Status == "blocked" || script.Status == "revision_requested" || script.Status == "superseded" {
			continue
		}
		script.Status = "review_required"
		if err := s.store.SaveScript(ctx, script); err != nil {
			return err
		}
		affected++
	}
	s.audit(ctx, actor, approved.ProjectID, "brief.superseded", "brief_version", previous.ID, requestID, map[string]any{"replacement_id": approved.ID, "affected_scripts": affected})
	return nil
}

func (s *Service) CreateScriptRun(ctx context.Context, actor Actor, briefID, idempotencyKey, requestID string) (domain.TaskRun, error) {
	return s.createScriptRun(ctx, actor, briefID, idempotencyKey, requestID, nil)
}

type scriptRunConfig struct {
	ScriptID          string
	BaselineVersionID string
	ChangeRequest     domain.ScriptChangeRequest
}

func (s *Service) createScriptRun(ctx context.Context, actor Actor, briefID, idempotencyKey, requestID string, config *scriptRunConfig) (domain.TaskRun, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.TaskRun{}, err
	}
	brief, err := s.store.Brief(ctx, actor.TenantID, briefID)
	if err != nil {
		return domain.TaskRun{}, err
	}
	if brief.Status != "approved" {
		return domain.TaskRun{}, domain.Policy("BRIEF_NOT_APPROVED", "只有已批准 Brief 可以生成正式剧本", "先完成 Brief 内审")
	}
	project, err := s.projectForWrite(ctx, actor, brief.ProjectID)
	if err != nil {
		return domain.TaskRun{}, err
	}
	if idempotencyKey != "" {
		runs, err := s.store.Runs(ctx, actor.TenantID, project.ID)
		if err != nil {
			return domain.TaskRun{}, err
		}
		for _, existing := range runs {
			if existing.IdempotencyKey == idempotencyKey {
				return existing, nil
			}
		}
	}
	knowledge := []domain.KnowledgeItem{}
	for _, id := range brief.ApprovedKnowledgeIDs {
		k, err := s.store.KnowledgeItem(ctx, actor.TenantID, id)
		if err != nil || !knowledgeActive(k, brief.Channel, s.now().UTC()) {
			return domain.TaskRun{}, domain.Policy("KNOWLEDGE_BLOCKED", "Brief 上游知识已失效", "复核知识后创建新 Brief 版本")
		}
		knowledge = append(knowledge, k)
	}
	assets, err := s.eligibleAssets(ctx, actor.TenantID, project.ID, brief.Channel, s.now().UTC())
	if err != nil {
		return domain.TaskRun{}, err
	}
	snapshot, err := domain.CompileSnapshotWithAssets(project, brief, knowledge, assets, s.now())
	if err != nil {
		return domain.TaskRun{}, err
	}
	if err := s.store.CreateSnapshot(ctx, snapshot); err != nil {
		return domain.TaskRun{}, err
	}
	if idempotencyKey == "" {
		idempotencyKey = domain.NewID()
	}
	now := s.now().UTC()
	run := domain.TaskRun{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, BriefVersionID: brief.ID, InputSnapshotID: snapshot.ID, IdempotencyKey: idempotencyKey, TaskType: "script_generate", CapabilityID: domain.ScriptCapability, CapabilityVersion: "1.1.0", InputSchema: domain.TaskContractSchema, OutputSchema: domain.ScriptPackageSchema, OutputCount: 1, DeliveryProfiles: []string{"review_projection/1.0", "text"}, State: "queued", Priority: 50, CreatedAt: now, UpdatedAt: now}
	if config != nil {
		run.TaskType = "script_revise"
		run.ScriptID = config.ScriptID
		run.BaselineVersionID = config.BaselineVersionID
		run.ChangeType = config.ChangeRequest.ChangeType
		run.InvariantFields = append([]string(nil), config.ChangeRequest.InvariantFields...)
		run.ExpectedChanges = append([]string(nil), config.ChangeRequest.ChangedFields...)
		run.Hypothesis = config.ChangeRequest.Hypothesis
		run.RevisionReason = config.ChangeRequest.RevisionReason
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return run, err
	}
	s.audit(ctx, actor, run.ProjectID, "run.created", "task_run", run.ID, requestID, map[string]any{"snapshot_id": snapshot.ID, "manifest_hash": snapshot.ManifestHash})
	return run, nil
}
func (s *Service) Runs(ctx context.Context, actor Actor, projectID string) ([]domain.TaskRun, error) {
	return s.store.Runs(ctx, actor.TenantID, projectID)
}
func (s *Service) Run(ctx context.Context, actor Actor, id string) (domain.TaskRun, error) {
	return s.store.Run(ctx, actor.TenantID, id)
}

type Lease struct {
	Run            domain.TaskRun      `json:"run"`
	Attempt        domain.RunAttempt   `json:"attempt"`
	Contract       domain.TaskContract `json:"contract"`
	LeaseExpiresAt time.Time           `json:"lease_expires_at"`
	RunToken       string              `json:"run_token"`
}

func (s *Service) Poll(ctx context.Context, actor Actor, device domain.Device, caps []domain.Capability) (Lease, error) {
	now := s.now().UTC()
	device.LastSeenAt = now
	device.Capabilities = caps
	_ = s.store.SaveDevice(ctx, device)
	if err := s.store.ExpireRunAttempts(ctx, actor.TenantID, now); err != nil {
		return Lease{}, err
	}
	runToken, runTokenHash, err := domain.NewOpaqueToken("rt_", 32)
	if err != nil {
		return Lease{}, err
	}
	run, attempt, err := s.store.LeaseNextRun(ctx, actor.TenantID, device.ID, caps, domain.NewID(), runTokenHash, now)
	if err != nil {
		return Lease{}, err
	}
	snapshot, err := s.store.Snapshot(ctx, actor.TenantID, run.InputSnapshotID)
	if err != nil {
		return Lease{}, err
	}
	project, _ := s.store.Project(ctx, actor.TenantID, run.ProjectID)
	brief := domain.BriefVersion{}
	if run.BriefVersionID != "" {
		brief, _ = s.store.Brief(ctx, actor.TenantID, run.BriefVersionID)
	}
	var baseline *domain.ScriptVersion
	var changeRequest *domain.ScriptChangeRequest
	if run.BaselineVersionID != "" {
		value, err := s.store.Script(ctx, actor.TenantID, run.BaselineVersionID)
		if err != nil {
			return Lease{}, err
		}
		baseline = &value
		changeRequest = &domain.ScriptChangeRequest{ChangeType: run.ChangeType, InvariantFields: append([]string(nil), run.InvariantFields...), ChangedFields: append([]string(nil), run.ExpectedChanges...), Hypothesis: run.Hypothesis, RevisionReason: run.RevisionReason}
	}
	capability := domain.Capability{ID: attempt.CapabilityID, Version: attempt.CapabilityVersion, Kind: "business_capability", InputSchema: attempt.InputSchema, OutputSchema: attempt.OutputSchema, Digest: attempt.CapabilityDigest, LocalOnly: true}
	contract := domain.TaskContract{ContractVersion: "1.0", ContractID: snapshot.ID, RunID: run.ID, TaskType: run.TaskType, Project: project, Brief: brief, Knowledge: snapshot.Knowledge, Assets: snapshot.Assets, Sources: snapshot.Sources, BaselineScriptVersion: baseline, ChangeRequest: changeRequest, InputSnapshotID: snapshot.ID, OutputSchema: run.OutputSchema, Capability: capability, ManifestHash: snapshot.ManifestHash}
	return Lease{Run: run, Attempt: attempt, Contract: contract, LeaseExpiresAt: *run.LeaseExpiresAt, RunToken: runToken}, nil
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

func (s *Service) ReportRun(ctx context.Context, actor Actor, device domain.Device, runID, runToken string, pkg domain.ScriptPackage, requestID string) (domain.ScriptVersion, error) {
	return s.ReportRunAttempt(ctx, actor, device, runID, "", runToken, pkg, requestID)
}

func (s *Service) ReportRunAttempt(ctx context.Context, actor Actor, device domain.Device, runID, attemptID, runToken string, pkg domain.ScriptPackage, requestID string) (domain.ScriptVersion, error) {
	run, err := s.store.Run(ctx, actor.TenantID, runID)
	if err != nil {
		return domain.ScriptVersion{}, err
	}
	hash, _ := domain.CanonicalHash(pkg)
	if run.State == "succeeded" || run.State == "failed" {
		scripts, _ := s.store.Scripts(ctx, actor.TenantID, run.ProjectID)
		for _, existing := range scripts {
			if existing.RunID != run.ID {
				continue
			}
			if existing.ContentHash == hash {
				return existing, nil
			}
			return domain.ScriptVersion{}, domain.Conflict("REPORT_CONFLICT", "同一任务已报告不同内容")
		}
	}
	attempt, err := s.activeRunAttempt(ctx, actor, device, run, attemptID, runToken, s.now().UTC())
	if err != nil {
		return domain.ScriptVersion{}, err
	}
	if run.CancelRequestedAt != nil {
		_, _ = s.FinishRunAttempt(ctx, actor, device, run.ID, attempt.ID, runToken, FinishRunAttemptInput{Outcome: "canceled", FailureClass: "user_canceled"}, requestID)
		return domain.ScriptVersion{}, domain.Conflict("RUN_CANCELED", "任务已取消，结果不会入库")
	}
	snapshot, err := s.store.Snapshot(ctx, actor.TenantID, run.InputSnapshotID)
	if err != nil {
		return domain.ScriptVersion{}, err
	}
	project, _ := s.store.Project(ctx, actor.TenantID, run.ProjectID)
	brief, _ := s.store.Brief(ctx, actor.TenantID, run.BriefVersionID)
	contract := domain.TaskContract{Project: project, Brief: brief, Knowledge: snapshot.Knowledge, Assets: snapshot.Assets, InputSnapshotID: snapshot.ID, OutputSchema: domain.ScriptPackageSchema}
	if project.Status == "archived" {
		return domain.ScriptVersion{}, domain.Policy("PROJECT_ARCHIVED", "已归档项目不能接收新的 Agent 结果", "恢复项目或取消任务")
	}
	report := domain.ValidateScript(pkg, contract)
	changedFields := []string{}
	if run.TaskType == "script_revise" {
		if run.ScriptID == "" || run.BaselineVersionID == "" || (run.ChangeType != "revision" && run.ChangeType != "variant") {
			_, _ = s.failRunAttempt(ctx, run, attempt, "change_contract", nil, nil, "剧本变更任务缺少不可变基线或变化声明")
			return domain.ScriptVersion{}, domain.E("content", "contract", "SCRIPT_CHANGE_CONTRACT_INVALID", "剧本变更任务契约不完整", 7)
		}
		baseline, baselineErr := s.store.Script(ctx, actor.TenantID, run.BaselineVersionID)
		if baselineErr != nil || baseline.ScriptID != run.ScriptID || baseline.ProjectID != run.ProjectID {
			_, _ = s.failRunAttempt(ctx, run, attempt, "change_contract", nil, nil, "剧本变更基线不属于目标逻辑剧本")
			return domain.ScriptVersion{}, domain.E("content", "contract", "SCRIPT_BASELINE_INVALID", "剧本变更基线不存在或不属于目标逻辑剧本", 7)
		}
		changeRequest := domain.ScriptChangeRequest{ChangeType: run.ChangeType, InvariantFields: run.InvariantFields, ChangedFields: run.ExpectedChanges, Hypothesis: run.Hypothesis, RevisionReason: run.RevisionReason}
		var changeReport domain.ValidationReport
		changedFields, changeReport = domain.ValidateScriptChange(baseline.Package, pkg, changeRequest)
		report = domain.MergeValidationReports(report, changeReport)
	}
	if !report.Valid {
		_, _ = s.failRunAttempt(ctx, run, attempt, "output_validation", nil, nil, "Script Package 未通过确定性业务校验")
		return domain.ScriptVersion{}, domain.E("content", "schema", "CAPABILITY_OUTPUT_INVALID", "Script Package 未通过服务端校验，任务将按重试策略处理", 7)
	}
	status := "blocked"
	if pkg.Deliverability == "review_ready" {
		status = "review_ready"
	}
	now := s.now().UTC()
	script := domain.ScriptVersion{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: run.ProjectID, ScriptID: run.ScriptID, RunID: run.ID, SupersedesID: run.BaselineVersionID, BaselineID: run.BaselineVersionID, ChangeType: defaultString(run.ChangeType, "initial"), InvariantFields: append([]string(nil), run.InvariantFields...), ChangedFields: changedFields, Hypothesis: run.Hypothesis, RevisionReason: run.RevisionReason, Status: status, InputSnapshotID: run.InputSnapshotID, ContentHash: hash, Package: pkg, Validation: report, CreatedAt: now}
	if run.ScriptID == "" {
		logical := domain.Script{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: run.ProjectID, Title: pkg.Title, CreatedAt: now}
		script, err = s.store.CreateScript(ctx, logical, script)
	} else {
		script, err = s.store.CreateScriptVersion(ctx, script)
	}
	if err != nil {
		return script, err
	}
	if _, _, cycleErr := s.ensureReviewCycle(ctx, actor, script); cycleErr != nil {
		s.log.Error("create script review cycle", "error", cycleErr, "script_version_id", script.ID)
	}
	run.State = "succeeded"
	run.ScriptID = script.ScriptID
	run.UpdatedAt = s.now().UTC()
	run.ProgressLabel = "策略校验完成"
	run.ReportHash = hash
	if err := s.succeedRunAttempt(ctx, &run, attempt); err != nil {
		return script, err
	}
	if err := s.store.SaveRun(ctx, run); err != nil {
		return script, err
	}
	s.ensureCoreScriptArtifact(ctx, script, device, domain.Capability{ID: attempt.CapabilityID, Version: attempt.CapabilityVersion, Digest: attempt.CapabilityDigest})
	s.audit(ctx, actor, run.ProjectID, "run.reported", "task_run", run.ID, requestID, map[string]any{"script_version_id": script.ID, "valid": report.Valid, "deliverability": pkg.Deliverability})
	return script, nil
}

func (s *Service) Scripts(ctx context.Context, actor Actor, projectID string) ([]domain.ScriptVersion, error) {
	return s.store.Scripts(ctx, actor.TenantID, projectID)
}
func (s *Service) Script(ctx context.Context, actor Actor, id string) (domain.ScriptVersion, error) {
	return s.store.Script(ctx, actor.TenantID, id)
}
func (s *Service) ReviewScript(ctx context.Context, actor Actor, id, decision, reason, requestID string) (domain.ScriptVersion, error) {
	return s.ReviewScriptWithInput(ctx, actor, id, ReviewScriptInput{Decision: decision, Conclusion: reason}, requestID)
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

func StableKnowledge(items []domain.KnowledgeItem) []domain.KnowledgeItem {
	out := append([]domain.KnowledgeItem(nil), items...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
