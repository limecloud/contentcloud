package application

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	"golang.org/x/crypto/argon2"

	auditdomain "github.com/limecloud/contentcloud/internal/audit"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	"github.com/limecloud/contentcloud/internal/catalog/environment"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/integration/connector"
	channeladapter "github.com/limecloud/contentcloud/internal/integration/provider/channel"
	mediapipeline "github.com/limecloud/contentcloud/internal/integration/provider/media"
	modelprovider "github.com/limecloud/contentcloud/internal/integration/provider/model"
	sourceinfra "github.com/limecloud/contentcloud/internal/integration/provider/source"
	"github.com/limecloud/contentcloud/internal/persistence"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type serviceCore struct {
	identity            persistence.IdentityRepository
	workspace           persistence.WorkspaceRepository
	source              persistence.SourceRepository
	knowledge           persistence.KnowledgeRepository
	catalog             persistence.CatalogRepository
	tasks               persistence.WorkRepository
	delivery            persistence.DeliveryRepository
	contexts            persistence.ContextRepository
	review              persistence.ReviewRepository
	artifacts           persistence.ArtifactRepository
	performance         persistence.PerformanceRepository
	auditRepo           persistence.AuditRepository
	deviceControl       deviceControlRepository
	runtimeWake         runtimeWakeBroker
	runtimeCommands     contentruntime.RuntimeCommandStore
	providerAdmin       providerProfileAdminStore
	now                 func() time.Time
	log                 *slog.Logger
	blobs               blob.Store
	platformAdminEmails map[string]struct{}
	environmentControl  *environment.ControlPlane
	automationPolicy    map[string]environment.CapabilityRequirement
	automationPackIDs   map[string][]string
	mediaAdapters       map[string]mediapipeline.Adapter
	sourceSearch        sourceinfra.SearchProvider
	sourceFetcher       *sourceinfra.Fetcher
	runtimeService      *contentruntime.Service
	runtimeHarnesses    *agentadapter.HarnessRegistry
	runtimeRollout      contentruntime.RolloutPolicy
	channelAdapters     *channeladapter.Registry
	modelProviders      *modelprovider.Registry
	connectorAdapters   *connector.Registry
	connectorRepository connector.Repository
}

// Dependencies is the application composition boundary. Each domain service
// receives only the repository contract it owns; optional capabilities are
// explicit ports rather than methods added to a global Store interface.
type Dependencies struct {
	Identity            persistence.IdentityRepository
	Workspace           persistence.WorkspaceRepository
	Source              persistence.SourceRepository
	Knowledge           persistence.KnowledgeRepository
	Catalog             persistence.CatalogRepository
	Work                persistence.WorkRepository
	Delivery            persistence.DeliveryRepository
	Contexts            persistence.ContextRepository
	Review              persistence.ReviewRepository
	Artifacts           persistence.ArtifactRepository
	Performance         persistence.PerformanceRepository
	Audit               persistence.AuditRepository
	Runtime             contentruntime.Repository
	DeviceControl       deviceControlRepository
	RuntimeWake         runtimeWakeBroker
	RuntimeCommands     contentruntime.RuntimeCommandStore
	ProviderAdmin       providerProfileAdminStore
	ConnectorRepository connector.Repository
}

// DependenciesFrom adapts a concrete persistence implementation at the
// process composition boundary. It deliberately returns data, not a Store
// facade, so application services cannot acquire unrelated capabilities.
func DependenciesFrom(value any) Dependencies {
	return Dependencies{
		Identity:            value.(persistence.IdentityRepository),
		Workspace:           value.(persistence.WorkspaceRepository),
		Source:              value.(persistence.SourceRepository),
		Knowledge:           value.(persistence.KnowledgeRepository),
		Catalog:             value.(persistence.CatalogRepository),
		Work:                value.(persistence.WorkRepository),
		Delivery:            value.(persistence.DeliveryRepository),
		Contexts:            value.(persistence.ContextRepository),
		Review:              value.(persistence.ReviewRepository),
		Artifacts:           value.(persistence.ArtifactRepository),
		Performance:         value.(persistence.PerformanceRepository),
		Audit:               value.(persistence.AuditRepository),
		Runtime:             value.(contentruntime.Repository),
		DeviceControl:       optionalDependency[deviceControlRepository](value),
		RuntimeWake:         optionalDependency[runtimeWakeBroker](value),
		RuntimeCommands:     optionalDependency[contentruntime.RuntimeCommandStore](value),
		ProviderAdmin:       optionalDependency[providerProfileAdminStore](value),
		ConnectorRepository: optionalDependency[connector.Repository](value),
	}
}

func optionalDependency[T any](value any) T {
	dependency, _ := value.(T)
	return dependency
}

type Application struct {
	Identity    *IdentityService
	Workspace   *WorkspaceService
	Source      *SourceService
	Catalog     *CatalogService
	Work        *WorkService
	Review      *ReviewService
	Delivery    *DeliveryService
	Performance *PerformanceService
	Runtime     *RuntimeService
	Operations  *OperationsService
}

type serviceScope struct {
	*serviceCore
	app *Application
}

type IdentityService struct{ *serviceScope }
type WorkspaceService struct{ *serviceScope }
type SourceService struct{ *serviceScope }
type CatalogService struct{ *serviceScope }
type WorkService struct{ *serviceScope }
type ReviewService struct{ *serviceScope }
type DeliveryService struct{ *serviceScope }
type PerformanceService struct{ *serviceScope }
type RuntimeService struct{ *serviceScope }
type OperationsService struct{ *serviceScope }

type runtimeWakeBroker interface {
	PublishRuntimeWake(context.Context, string) error
	ListenRuntimeWakes(context.Context, func(string)) error
}

type deviceControlRepository interface {
	RotateDeviceCredential(context.Context, string, string, string, time.Time) (workspacedomain.Device, error)
	SaveDaemonInstance(context.Context, workspacedomain.DaemonInstance) error
	DaemonInstance(context.Context, string, string) (workspacedomain.DaemonInstance, error)
	DaemonInstances(context.Context, string, string) ([]workspacedomain.DaemonInstance, error)
}

type Actor struct {
	UserID        string
	TenantID      string
	Role          string
	Type          string
	DeviceID      string
	WorkspaceID   string
	ProjectIDs    []string
	PlatformAdmin bool
}

type Dashboard struct {
	Tenant      identitydomain.Tenant     `json:"tenant"`
	Projects    []workspacedomain.Project `json:"projects"`
	RecentRuns  []work.RuntimeRun         `json:"recent_runs"`
	RecentAudit []auditdomain.AuditEvent  `json:"recent_audit"`
	Counts      map[string]int            `json:"counts"`
	Pipeline    []PipelineStage           `json:"pipeline"`
}

type PipelineStage struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Count   int    `json:"count"`
	Blocked int    `json:"blocked"`
}

type Option func(*serviceCore)

func WithPlatformAdminEmails(emails ...string) Option {
	return func(service *serviceCore) {
		for _, email := range emails {
			normalized := strings.ToLower(strings.TrimSpace(email))
			if normalized != "" {
				service.platformAdminEmails[normalized] = struct{}{}
			}
		}
	}
}

func WithEnvironmentControlPlane(controlPlane *environment.ControlPlane) Option {
	return func(service *serviceCore) {
		service.environmentControl = controlPlane
	}
}

func WithAutomationExecutionPolicy(requirements []environment.CapabilityRequirement, packIDs map[string][]string) Option {
	return func(service *serviceCore) {
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

func WithRuntimeHarnessRegistry(registry *agentadapter.HarnessRegistry) Option {
	return func(service *serviceCore) {
		service.runtimeHarnesses = registry
	}
}

func WithRuntimeRolloutPolicy(policy contentruntime.RolloutPolicy) Option {
	return func(service *serviceCore) {
		service.runtimeRollout = policy
	}
}

// WithMediaProviderAdapter registers one provider implementation at the App
// boundary. Media workers resolve providers through this map; no second
// provider registry or per-job fallback is allowed.
func WithMediaProviderAdapter(providerID string, adapter mediapipeline.Adapter) Option {
	return func(service *serviceCore) {
		providerID = strings.ToLower(strings.TrimSpace(providerID))
		if providerID == "" || adapter == nil {
			return
		}
		if service.mediaAdapters == nil {
			service.mediaAdapters = map[string]mediapipeline.Adapter{}
		}
		service.mediaAdapters[providerID] = adapter
	}
}

// WithSourceSearchProvider and WithSourceFetcher are dependency seams for
// provider-neutral search/fetch. Production uses the configured defaults;
// tests and self-hosted deployments can provide a controlled implementation.
func WithSourceSearchProvider(provider sourceinfra.SearchProvider) Option {
	return func(service *serviceCore) {
		if provider != nil {
			service.sourceSearch = provider
		}
	}
}

func WithSourceFetcher(fetcher *sourceinfra.Fetcher) Option {
	return func(service *serviceCore) {
		if fetcher != nil {
			service.sourceFetcher = fetcher
		}
	}
}

func WithChannelAdapterRegistry(registry *channeladapter.Registry) Option {
	return func(service *serviceCore) {
		if registry != nil {
			service.channelAdapters = registry
		}
	}
}

func WithModelProviderRegistry(registry *modelprovider.Registry) Option {
	return func(service *serviceCore) {
		if registry != nil {
			service.modelProviders = registry
		}
	}
}

func WithConnectorRegistry(registry *connector.Registry) Option {
	return func(service *serviceCore) {
		if registry != nil {
			service.connectorAdapters = registry
		}
	}
}

func WithConnectorRepository(repository connector.Repository) Option {
	return func(service *serviceCore) {
		if repository != nil {
			service.connectorRepository = repository
		}
	}
}

func New(dependencies Dependencies, logger *slog.Logger, options ...Option) *Application {
	return NewWithBlob(dependencies, logger, blob.NewMemory(), options...)
}

func NewWithBlob(dependencies Dependencies, logger *slog.Logger, blobs blob.Store, options ...Option) *Application {
	if logger == nil {
		logger = slog.Default()
	}
	if blobs == nil {
		blobs = blob.NewMemory()
	}
	core := &serviceCore{
		identity: dependencies.Identity, workspace: dependencies.Workspace, source: dependencies.Source, knowledge: dependencies.Knowledge, catalog: dependencies.Catalog, tasks: dependencies.Work,
		delivery: dependencies.Delivery, contexts: dependencies.Contexts, review: dependencies.Review, artifacts: dependencies.Artifacts, performance: dependencies.Performance, auditRepo: dependencies.Audit,
		now: time.Now, log: logger, blobs: blobs, platformAdminEmails: map[string]struct{}{},
		automationPolicy: map[string]environment.CapabilityRequirement{}, automationPackIDs: map[string][]string{},
		mediaAdapters: map[string]mediapipeline.Adapter{}, sourceSearch: sourceinfra.NewDefaultSearchProvider(),
		sourceFetcher: sourceinfra.NewDefaultFetcher(), runtimeHarnesses: agentadapter.NewDefaultHarnessRegistry(),
		runtimeRollout: contentruntime.DefaultRolloutPolicy(), channelAdapters: channeladapter.NewDefaultRegistry(),
		modelProviders: modelprovider.NewDefaultRegistry(), connectorAdapters: connector.NewDefaultRegistry(),
		deviceControl: dependencies.DeviceControl, runtimeWake: dependencies.RuntimeWake,
		runtimeCommands: dependencies.RuntimeCommands, providerAdmin: dependencies.ProviderAdmin,
		connectorRepository: dependencies.ConnectorRepository,
	}
	for _, option := range options {
		option(core)
	}
	if runtimeRepo := dependencies.Runtime; runtimeRepo != nil {
		// Keep the Runtime clock coupled to the App clock. Tests, replay tools,
		// and maintenance controllers may replace the App clock; capturing the
		// original function here would make lease/backlog observations drift.
		runtimeNow := func() time.Time { return core.now() }
		core.runtimeService = contentruntime.NewWithHarnessRegistry(runtimeRepo, runtimeNow, core.runtimeHarnesses)
		core.runtimeService.SetRolloutPolicy(core.runtimeRollout)
	}
	scope := &serviceScope{serviceCore: core}
	application := &Application{
		Identity: &IdentityService{scope}, Workspace: &WorkspaceService{scope}, Source: &SourceService{scope},
		Catalog: &CatalogService{scope}, Work: &WorkService{scope}, Review: &ReviewService{scope},
		Delivery: &DeliveryService{scope}, Performance: &PerformanceService{scope}, Runtime: &RuntimeService{scope},
		Operations: &OperationsService{scope},
	}
	scope.app = application
	return application
}

// Runtime exposes the platform-owned execution service to the Runtime BFF and
// to customer task adapters. Business facts still belong to their owning app
// services; this method only exposes execution metadata and commands.
func (s *RuntimeService) Runtime() *contentruntime.Service { return s.runtimeService }

func (s *RuntimeService) PublishRuntimeWake(ctx context.Context, tenantID string) error {
	if s.runtimeWake == nil {
		return nil
	}
	return s.runtimeWake.PublishRuntimeWake(ctx, strings.TrimSpace(tenantID))
}

func (s *RuntimeService) ListenRuntimeWakes(ctx context.Context, notify func(string)) error {
	if s.runtimeWake == nil {
		return nil
	}
	return s.runtimeWake.ListenRuntimeWakes(ctx, notify)
}

func (s *RuntimeService) HasRuntimeWakeBroker() bool {
	return s.runtimeWake != nil
}

// ProviderBinding exposes only the provider binding boundary needed by
// authenticated callback ingress. CredentialRef remains hidden from callers.
func (s *DeliveryService) ProviderBinding(ctx context.Context, tenantID, providerID string) (deliverydomain.ProviderBinding, error) {
	return s.delivery.ProviderBinding(ctx, tenantID, providerID)
}

// newRegistration 校验注册凭据并构造用户记录，不写入存储。
func newRegistration(email, password, displayName string, now time.Time) (identitydomain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") || len(password) < 10 {
		return identitydomain.User{}, fault.Invalid("REGISTRATION_INVALID", "邮箱无效或密码少于 10 位")
	}
	user := identitydomain.User{ID: idgen.New(), Email: email, DisplayName: strings.TrimSpace(displayName), PasswordHash: hashPassword(password), VerifiedAt: &now, CreatedAt: now}
	if user.DisplayName == "" {
		user.DisplayName = strings.Split(email, "@")[0]
	}
	return user, nil
}

func (s *IdentityService) Register(ctx context.Context, email, password, displayName, tenantName string) (identitydomain.Session, error) {
	now := s.now().UTC()
	user, err := newRegistration(email, password, displayName, now)
	if err != nil {
		return identitydomain.Session{}, err
	}
	if err := s.identity.CreateUser(ctx, user); err != nil {
		return identitydomain.Session{}, err
	}
	tenant := identitydomain.Tenant{ID: idgen.New(), Slug: slugify(tenantName), Name: strings.TrimSpace(tenantName), Status: "active", CreatedAt: now}
	if tenant.Name == "" {
		tenant.Name = "我的内容团队"
		tenant.Slug = "my-team"
	}
	if err := s.identity.CreateTenant(ctx, tenant, identitydomain.Membership{TenantID: tenant.ID, UserID: user.ID, Role: "tenant_admin", Status: "active", CreatedAt: now}); err != nil {
		return identitydomain.Session{}, err
	}
	session := identitydomain.Session{ID: idgen.New(), UserID: user.ID, TenantID: tenant.ID, ExpiresAt: now.Add(12 * time.Hour)}
	if err := s.identity.SaveSession(ctx, session); err != nil {
		return identitydomain.Session{}, err
	}
	s.audit(ctx, Actor{UserID: user.ID, TenantID: tenant.ID, Role: "tenant_admin", Type: "user"}, "", "tenant.created", "tenant", tenant.ID, "", map[string]any{"name": tenant.Name})
	return session, nil
}

// RegisterWithInvite 凭成员邀请令牌注册并直接加入邀请方租户，不创建新租户。
func (s *IdentityService) RegisterWithInvite(ctx context.Context, email, password, displayName, inviteToken string) (identitydomain.Session, error) {
	now := s.now().UTC()
	user, err := newRegistration(email, password, displayName, now)
	if err != nil {
		return identitydomain.Session{}, err
	}
	invite, err := s.validateInviteToken(ctx, inviteToken, user.Email, now)
	if err != nil {
		return identitydomain.Session{}, err
	}
	session := identitydomain.Session{ID: idgen.New(), UserID: user.ID, ExpiresAt: now.Add(12 * time.Hour)}
	session, membership, err := s.identity.RegisterWithInvite(ctx, user, idgen.TokenHash(inviteToken), session, now)
	if err != nil {
		return identitydomain.Session{}, err
	}
	s.audit(ctx, Actor{UserID: user.ID, TenantID: session.TenantID, Role: membership.Role, Type: "user"}, "", "membership.invite_accepted", "membership", user.ID, "", map[string]any{"invite_id": invite.ID, "role": membership.Role})
	return session, nil
}

func (s *IdentityService) Login(ctx context.Context, email, password string) (identitydomain.Session, error) {
	user, err := s.identity.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil || !checkPassword(user.PasswordHash, password) {
		return identitydomain.Session{}, fault.E("authentication", "credentials", "AUTH_INVALID", "邮箱或密码错误", 3)
	}
	if user.VerifiedAt == nil {
		return identitydomain.Session{}, fault.E("authentication", "email_verification", "EMAIL_NOT_VERIFIED", "邮箱尚未验证", 3)
	}
	tenants, err := s.identity.TenantsForUser(ctx, user.ID)
	if err != nil || len(tenants) == 0 {
		return identitydomain.Session{}, fault.E("authentication", "membership", "TENANT_REQUIRED", "用户没有可用租户", 3)
	}
	session := identitydomain.Session{ID: idgen.New(), UserID: user.ID, TenantID: tenants[0].ID, ExpiresAt: s.now().UTC().Add(12 * time.Hour)}
	if err := s.identity.SaveSession(ctx, session); err != nil {
		return identitydomain.Session{}, err
	}
	return session, nil
}

func (s *IdentityService) SessionActor(ctx context.Context, sessionID string) (Actor, identitydomain.User, error) {
	session, err := s.identity.SessionByID(ctx, sessionID)
	if err != nil {
		return Actor{}, identitydomain.User{}, fault.E("authentication", "session", "SESSION_INVALID", "会话无效或已过期", 3)
	}
	user, err := s.identity.UserByID(ctx, session.UserID)
	if err != nil {
		return Actor{}, identitydomain.User{}, err
	}
	m, err := s.identity.Membership(ctx, session.TenantID, session.UserID)
	if err != nil {
		return Actor{}, identitydomain.User{}, err
	}
	_, platformAdmin := s.platformAdminEmails[strings.ToLower(user.Email)]
	return Actor{UserID: user.ID, TenantID: session.TenantID, Role: m.Role, Type: "user", PlatformAdmin: platformAdmin}, user, nil
}

func (s *IdentityService) Tenant(ctx context.Context, actor Actor) (identitydomain.Tenant, error) {
	tenants, err := s.identity.TenantsForUser(ctx, actor.UserID)
	if err != nil {
		return identitydomain.Tenant{}, err
	}
	for _, t := range tenants {
		if t.ID == actor.TenantID {
			return t, nil
		}
	}
	return identitydomain.Tenant{}, fault.NotFound("租户")
}

func (s *OperationsService) Dashboard(ctx context.Context, actor Actor) (Dashboard, error) {
	tenant, err := s.app.Identity.Tenant(ctx, actor)
	if err != nil {
		return Dashboard{}, err
	}
	projects, err := s.workspace.Projects(ctx, actor.TenantID)
	if err != nil {
		return Dashboard{}, err
	}
	runs, _ := s.app.Runtime.runtimeRunsForTenant(ctx, actor.TenantID)
	if len(runs) > 8 {
		runs = runs[:8]
	}
	audits, _ := s.auditRepo.AuditEvents(ctx, actor.TenantID, "", 8)
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

func (s *WorkspaceService) CreateProject(ctx context.Context, actor Actor, in CreateProjectInput, requestID string) (workspacedomain.Project, error) {
	if !canManage(actor.Role) {
		return workspacedomain.Project{}, fault.Policy("ROLE_DENIED", "当前角色不能创建项目", "联系租户管理员")
	}
	if strings.TrimSpace(in.BrandName) == "" || strings.TrimSpace(in.ProductName) == "" {
		return workspacedomain.Project{}, fault.Invalid("PROJECT_FIELDS_REQUIRED", "品牌名和单品名必填")
	}
	in.ContentType = defaultString(strings.TrimSpace(in.ContentType), identitydomain.DefaultProjectContentType)
	if !identitydomain.ValidTenantContentType(in.ContentType) {
		return workspacedomain.Project{}, fault.Invalid("PROJECT_CONTENT_TYPE_INVALID", "项目内容类型不受支持")
	}
	if in.TemplateID != "" {
		template, err := s.workspace.ProjectTemplate(ctx, actor.TenantID, in.TemplateID)
		if err != nil {
			return workspacedomain.Project{}, err
		}
		if strings.TrimSpace(in.Channel) == "" {
			in.Channel = template.Channel
		}
		if strings.TrimSpace(in.StageObjective) == "" {
			in.StageObjective = template.StageObjective
		}
	}
	now := s.now().UTC()
	p := workspacedomain.Project{ID: idgen.New(), TenantID: actor.TenantID, Slug: slugify(in.BrandName + "-" + in.ProductName), BrandName: strings.TrimSpace(in.BrandName), ProductName: strings.TrimSpace(in.ProductName), ContentType: in.ContentType, Channel: defaultString(in.Channel, "douyin"), StageObjective: in.StageObjective, Status: "draft", OwnerName: in.OwnerName, ReviewerName: in.ReviewerName, ClientApprover: in.ClientApprover, RowVersion: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.workspace.CreateProject(ctx, p); err != nil {
		return p, err
	}
	if _, _, err := s.app.Work.ProjectSOP(ctx, actor, p.ID); err != nil {
		return p, err
	}
	s.audit(ctx, actor, p.ID, "project.created", "project", p.ID, requestID, map[string]any{"brand_name": p.BrandName, "product_name": p.ProductName, "content_type": p.ContentType})
	return p, nil
}
func (s *WorkspaceService) Projects(ctx context.Context, actor Actor) ([]workspacedomain.Project, error) {
	return s.workspace.Projects(ctx, actor.TenantID)
}
func (s *WorkspaceService) Project(ctx context.Context, actor Actor, id string) (workspacedomain.Project, error) {
	return s.workspace.Project(ctx, actor.TenantID, id)
}

func (s *WorkspaceService) CreateConnectSession(ctx context.Context, actor Actor, projectID, requestID string) (workspacedomain.ConnectSession, error) {
	if !canManage(actor.Role) {
		return workspacedomain.ConnectSession{}, fault.Policy("ROLE_DENIED", "当前角色不能连接设备", "联系项目负责人")
	}
	return s.createConnectSession(ctx, actor, projectID, requestID)
}

func (s *WorkspaceService) createConnectSession(ctx context.Context, actor Actor, projectID, requestID string) (workspacedomain.ConnectSession, error) {
	if _, err := s.app.Identity.projectForWrite(ctx, actor, projectID); err != nil {
		return workspacedomain.ConnectSession{}, err
	}
	now := s.now().UTC()
	v := workspacedomain.ConnectSession{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: projectID, InviterUserID: actor.UserID, State: "waiting_for_computer", ExpiresAt: now.Add(10 * time.Minute)}
	if err := s.workspace.CreateConnectSession(ctx, v); err != nil {
		return v, err
	}
	s.audit(ctx, actor, projectID, "connect_session.created", "connect_session", v.ID, requestID, map[string]any{"expires_at": v.ExpiresAt})
	return v, nil
}
func (s *WorkspaceService) ConnectSession(ctx context.Context, actor Actor, id string) (workspacedomain.ConnectSession, error) {
	session, err := s.workspace.ConnectSessionByID(ctx, actor.TenantID, id)
	if err != nil {
		return session, err
	}
	if session.State == "waiting_for_computer" && s.now().UTC().After(session.ExpiresAt) {
		session.State = "expired"
		if err := s.workspace.SaveConnectSession(ctx, session); err != nil {
			return session, err
		}
	}
	progress, err := s.workspace.BootstrapProgressForSession(ctx, actor.TenantID, id)
	if err != nil {
		return session, err
	}
	if session.State != "expired" {
		session.Progress = progress
	}
	return session, nil
}

type ConnectDeviceInput struct {
	MachineID    string                     `json:"machine_id"`
	DisplayName  string                     `json:"display_name"`
	Hostname     string                     `json:"hostname"`
	Platform     string                     `json:"platform"`
	Arch         string                     `json:"arch"`
	Version      string                     `json:"version"`
	Capabilities []catalogdomain.Capability `json:"capabilities"`
}
type ConnectDeviceResult struct {
	Device              workspacedomain.Device `json:"device"`
	DeviceToken         string                 `json:"device_token"`
	WorkspaceID         string                 `json:"workspace_id"`
	WorkspaceToken      string                 `json:"workspace_token"`
	ProjectID           string                 `json:"project_id"`
	EnvironmentManifest *environment.Manifest  `json:"environment_manifest,omitempty"`
	BootstrapAttemptID  string                 `json:"bootstrap_attempt_id,omitempty"`
}

type RotateDeviceCredentialResult struct {
	Device      workspacedomain.Device `json:"device"`
	DeviceToken string                 `json:"device_token"`
}

func (s *WorkspaceService) DeviceActor(ctx context.Context, token string) (Actor, workspacedomain.Device, error) {
	if !strings.HasPrefix(token, "dt_") {
		return Actor{}, workspacedomain.Device{}, fault.E("authentication", "device", "DEVICE_TOKEN_INVALID", "设备凭据无效", 3)
	}
	d, err := s.workspace.DeviceByTokenHash(ctx, idgen.TokenHash(token))
	if err != nil {
		if fault.IsNotFound(err) {
			return Actor{}, d, fault.E("authentication", "device", "DEVICE_TOKEN_INVALID", "设备凭据无效", 3)
		}
		return Actor{}, d, err
	}
	return Actor{UserID: d.OwnerUserID, TenantID: d.TenantID, Type: "device", DeviceID: d.ID, ProjectIDs: append([]string{}, d.ProjectIDs...)}, d, nil
}

func (s *WorkspaceService) WorkspaceActor(ctx context.Context, token string) (Actor, workspacedomain.WorkspaceBinding, error) {
	if !strings.HasPrefix(token, "wt_") {
		return Actor{}, workspacedomain.WorkspaceBinding{}, fault.E("authentication", "workspace", "WORKSPACE_TOKEN_INVALID", "工作区凭据无效", 3)
	}
	binding, err := s.workspace.WorkspaceBindingByTokenHash(ctx, idgen.TokenHash(token))
	if err != nil {
		return Actor{}, binding, fault.E("authentication", "workspace", "WORKSPACE_TOKEN_INVALID", "工作区凭据无效", 3)
	}
	actor := Actor{UserID: binding.OwnerUserID, TenantID: binding.TenantID, Role: "editor", Type: "workspace", DeviceID: binding.DeviceID, WorkspaceID: binding.ID}
	binding.CredentialHash = ""
	return actor, binding, nil
}
func (s *WorkspaceService) Devices(ctx context.Context, actor Actor, projectID string) ([]workspacedomain.Device, error) {
	return s.workspace.Devices(ctx, actor.TenantID, projectID)
}

func (s *ReviewService) Approvals(ctx context.Context, actor Actor, subjectID string) ([]reviewdomain.ApprovalDecision, error) {
	return s.review.Approvals(ctx, actor.TenantID, subjectID)
}

func (s *RuntimeService) Runs(ctx context.Context, actor Actor, projectID string) ([]work.RuntimeRun, error) {
	return s.runtimeRunsForProject(ctx, actor.TenantID, projectID)
}
func (s *RuntimeService) Run(ctx context.Context, actor Actor, id string) (work.RuntimeRun, error) {
	if job, ok, err := s.runtimeJob(ctx, actor.TenantID, id); err != nil {
		return work.RuntimeRun{}, err
	} else if ok {
		return s.projectRuntimeRun(ctx, job)
	}
	return work.RuntimeRun{}, fault.NotFound("Runtime JobRun")
}

func (s *RuntimeService) CancelRun(ctx context.Context, actor Actor, runID, requestID string) (work.RuntimeRun, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return work.RuntimeRun{}, err
	}
	if job, ok, err := s.runtimeJob(ctx, actor.TenantID, runID); err != nil {
		return work.RuntimeRun{}, err
	} else if ok {
		if _, err := s.runtimeService.Cancel(ctx, actor.TenantID, job.ID, "user", actor.UserID); err != nil {
			return work.RuntimeRun{}, err
		}
		runs, err := s.runtimeRunsForWorkTask(ctx, actor.TenantID, job.WorkTaskID)
		if err != nil {
			return work.RuntimeRun{}, err
		}
		if len(runs) == 0 {
			return work.RuntimeRun{}, fault.NotFound("Runtime JobRun 投影")
		}
		s.audit(ctx, actor, job.ProjectID, "runtime.cancelled", "job_run", job.ID, requestID, map[string]any{})
		return runs[0], nil
	}
	return work.RuntimeRun{}, fault.NotFound("Runtime JobRun")
}

func (s *OperationsService) Audit(ctx context.Context, actor Actor, projectID string, limit int) ([]auditdomain.AuditEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.auditRepo.AuditEvents(ctx, actor.TenantID, projectID, limit)
}

func (s *serviceCore) audit(ctx context.Context, actor Actor, projectID, action, subjectType, subjectID, requestID string, summary map[string]any) {
	if requestID == "" {
		requestID = "req_" + idgen.New()
	}
	event := auditdomain.AuditEvent{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: projectID, ActorType: defaultString(actor.Type, "user"), ActorID: actor.UserID, Action: action, SubjectType: subjectType, SubjectID: subjectID, Summary: summary, RequestID: requestID, CreatedAt: s.now().UTC()}
	if err := s.auditRepo.AppendAudit(ctx, event); err != nil {
		s.log.Error("append audit", "error", err, "action", action)
	}
}

func canManage(role string) bool { return role == "tenant_admin" || role == "project_manager" }

// canConnectStudioClient keeps customer self-service separate from team administration.
func canConnectStudioClient(role string) bool {
	return containsString([]string{"tenant_admin", "project_manager", "strategist", "editor"}, role)
}
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
		return "project-" + strings.Split(idgen.New(), "-")[0]
	}
	return strings.Join(parts, "-")
}

func hashPassword(password string) string {
	salt := []byte(idgen.New())
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
