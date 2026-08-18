package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	auditdomain "github.com/limecloud/contentcloud/internal/audit"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/integration/connector"
	performancedomain "github.com/limecloud/contentcloud/internal/performance"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type Store struct {
	mu                        sync.RWMutex
	users                     map[string]identitydomain.User
	userByEmail               map[string]string
	sessions                  map[string]identitydomain.Session
	tenants                   map[string]identitydomain.Tenant
	tenantContentCaps         map[string]identitydomain.TenantContentCapability
	memberships               map[string]identitydomain.Membership
	membershipInvites         map[string]identitydomain.MembershipInvite
	projects                  map[string]workspacedomain.Project
	projectTemplates          map[string]workspacedomain.ProjectTemplate
	connects                  map[string]workspacedomain.ConnectSession
	bootstrapAttempts         map[string]workspacedomain.BootstrapAttempt
	bootstrapEvents           map[string]map[int64]workspacedomain.BootstrapProgressEvent
	bootstrapDiagnostics      map[string]workspacedomain.BootstrapDiagnostic
	devices                   map[string]workspacedomain.Device
	daemonInstances           map[string]workspacedomain.DaemonInstance
	workspaceBindings         map[string]workspacedomain.WorkspaceBinding
	workspaceRevisions        map[string]workspacedomain.WorkspaceRevision
	workspaceUploadSessions   map[string]workspacedomain.WorkspaceUploadSession
	workspaceUploadParts      map[string]workspacedomain.WorkspaceUploadPart
	workspaceObjects          map[string]workspacedomain.WorkspaceObject
	userDeviceFlows           map[string]workspacedomain.UserDeviceFlow
	cliTokens                 map[string]workspacedomain.CLIToken
	sources                   map[string]sourcedomain.Source
	revisions                 map[string]sourcedomain.SourceRevision
	evidence                  map[string]sourcedomain.EvidenceSpan
	assets                    map[string]sourcedomain.Asset
	workspaceFolders          map[string]workspacedomain.WorkspaceFolder
	workspaceMaterials        map[string]workspacedomain.WorkspaceMaterial
	rightsRecords             map[string]sourcedomain.RightsRecord
	knowledgeObjects          map[string]sourcedomain.KnowledgeObject
	knowledgeDecisions        map[string]sourcedomain.KnowledgeDecision
	knowledgePacks            map[string]sourcedomain.KnowledgePack
	knowledgeSnapshots        map[string]sourcedomain.KnowledgeSnapshot
	environments              map[string]catalogdomain.Environment
	sopDefinitions            map[string]catalogdomain.SOPDefinition
	sopVersions               map[string]catalogdomain.SOPVersion
	projectSOPBindings        map[string]catalogdomain.ProjectSOPBinding
	workTasks                 map[string]work.WorkTask
	inputItems                map[string]work.InputItem
	conversationImports       map[string]work.ConversationImport
	stageRuns                 map[string]work.StageRun
	stageOutputs              map[string]work.TaskStageOutput
	providerProfiles          map[string]deliverydomain.ProviderProfile
	providerBindings          map[string]deliverydomain.ProviderBinding
	mediaJobs                 map[string]deliverydomain.MediaGenerationJob
	providerAttempts          map[string]deliverydomain.ProviderAttempt
	mediaReviews              map[string]deliverydomain.MediaReview
	gateEvaluations           map[string]reviewdomain.GateEvaluation
	taskRevisions             map[string]reviewdomain.TaskRevision
	taskDeliveries            map[string]deliverydomain.TaskDelivery
	channelBindings           map[string]deliverydomain.ChannelBinding
	channelPublications       map[string]deliverydomain.ChannelPublication
	channelCallbackReceipts   map[string]deliverydomain.ChannelCallbackReceipt
	modelGenerationReceipts   map[string]deliverydomain.ModelGenerationReceipt
	connectorBindings         map[string]connector.Binding
	connectorSyncLeases       map[string]connector.SyncLease
	connectorRecords          map[string]connector.RecordMapping
	connectorReceipts         map[string]connector.SyncReceipt
	snapshots                 map[string]sourcedomain.ContextSnapshot
	runtimePlans              map[string]contentruntime.JobPlanRevision
	runtimeExecutionBindings  map[string]contentruntime.ExecutionBindingSnapshot
	runtimeJobs               map[string]contentruntime.JobRun
	runtimeNodes              map[string]contentruntime.NodeRun
	runtimeFanoutSets         map[string]contentruntime.FanoutSet
	runtimeFanoutMembers      map[string]contentruntime.FanoutMember
	runtimeEvents             map[string][]contentruntime.JobEvent
	runtimeOutbox             map[string]contentruntime.RuntimeOutboxMessage
	runtimeOutboxReceipts     map[string]runtimeOutboxReceipt
	runtimeContextViews       map[string]contentruntime.ContextView
	runtimeAgents             map[string]contentruntime.AgentInstance
	runtimeAttempts           map[string]contentruntime.RuntimeAttempt
	runtimeStates             map[string]contentruntime.RuntimeState
	runtimeStateMutations     map[string]string
	runtimeCheckpoints        map[string]contentruntime.Checkpoint
	runtimeEffects            map[string]contentruntime.ExternalEffect
	runtimeProviderInbox      map[string]contentruntime.ProviderInboxMessage
	runtimeProviderRecons     map[string]contentruntime.ProviderReconciliation
	runtimeProviderBills      map[string]contentruntime.ProviderBillRecord
	runtimeYields             map[string]contentruntime.RuntimeYield
	runtimeStateCollections   map[string]contentruntime.StateCollection
	runtimeStateRecords       map[string]contentruntime.StateRecord
	runtimeToolCalls          map[string]contentruntime.ToolCall
	runtimeProjections        map[string]contentruntime.RuntimeExplorerView
	runtimeProjectionRebuilds map[string]contentruntime.RuntimeProjectionRebuildRun
	runtimeMaintenance        map[string]contentruntime.RuntimeMaintenanceHeartbeat
	runtimeResourceQuotas     map[string]contentruntime.ResourceQuota
	runtimeReservations       map[string]contentruntime.ResourceReservation
	runtimeSchemas            map[string]contentruntime.RuntimeSchema
	approvals                 map[string]reviewdomain.ApprovalDecision
	reviewCycles              map[string]reviewdomain.ReviewCycle
	reviewComments            map[string]reviewdomain.ReviewComment
	reviewGrants              map[string]reviewdomain.ReviewGrant
	submissions               map[string]reviewdomain.Submission
	submissionRevisions       map[string]reviewdomain.SubmissionRevision
	approvedSnapshots         map[string]reviewdomain.ApprovedSnapshot
	artifacts                 map[string]deliverydomain.Artifact
	deliveryPackages          map[string]deliverydomain.DeliveryPackage
	performanceBatches        map[string]performancedomain.PerformanceImportBatch
	observations              map[string]performancedomain.PerformanceObservation
	ratingDecisions           map[string]performancedomain.RatingDecision
	audits                    []auditdomain.AuditEvent
}

func New() *Store {
	return &Store{
		users: map[string]identitydomain.User{}, userByEmail: map[string]string{}, sessions: map[string]identitydomain.Session{},
		tenants: map[string]identitydomain.Tenant{}, tenantContentCaps: map[string]identitydomain.TenantContentCapability{}, memberships: map[string]identitydomain.Membership{}, membershipInvites: map[string]identitydomain.MembershipInvite{}, projects: map[string]workspacedomain.Project{}, projectTemplates: map[string]workspacedomain.ProjectTemplate{},
		connects: map[string]workspacedomain.ConnectSession{}, bootstrapAttempts: map[string]workspacedomain.BootstrapAttempt{}, bootstrapEvents: map[string]map[int64]workspacedomain.BootstrapProgressEvent{}, bootstrapDiagnostics: map[string]workspacedomain.BootstrapDiagnostic{}, devices: map[string]workspacedomain.Device{}, daemonInstances: map[string]workspacedomain.DaemonInstance{}, workspaceBindings: map[string]workspacedomain.WorkspaceBinding{}, workspaceRevisions: map[string]workspacedomain.WorkspaceRevision{}, workspaceUploadSessions: map[string]workspacedomain.WorkspaceUploadSession{}, workspaceUploadParts: map[string]workspacedomain.WorkspaceUploadPart{}, workspaceObjects: map[string]workspacedomain.WorkspaceObject{}, userDeviceFlows: map[string]workspacedomain.UserDeviceFlow{}, cliTokens: map[string]workspacedomain.CLIToken{},
		sources: map[string]sourcedomain.Source{}, revisions: map[string]sourcedomain.SourceRevision{}, evidence: map[string]sourcedomain.EvidenceSpan{}, assets: map[string]sourcedomain.Asset{}, workspaceFolders: map[string]workspacedomain.WorkspaceFolder{}, workspaceMaterials: map[string]workspacedomain.WorkspaceMaterial{}, rightsRecords: map[string]sourcedomain.RightsRecord{}, knowledgeObjects: map[string]sourcedomain.KnowledgeObject{}, knowledgeDecisions: map[string]sourcedomain.KnowledgeDecision{}, knowledgePacks: map[string]sourcedomain.KnowledgePack{}, knowledgeSnapshots: map[string]sourcedomain.KnowledgeSnapshot{}, environments: map[string]catalogdomain.Environment{}, sopDefinitions: map[string]catalogdomain.SOPDefinition{}, sopVersions: map[string]catalogdomain.SOPVersion{}, projectSOPBindings: map[string]catalogdomain.ProjectSOPBinding{}, workTasks: map[string]work.WorkTask{}, inputItems: map[string]work.InputItem{}, conversationImports: map[string]work.ConversationImport{}, stageRuns: map[string]work.StageRun{}, stageOutputs: map[string]work.TaskStageOutput{}, providerProfiles: map[string]deliverydomain.ProviderProfile{}, providerBindings: map[string]deliverydomain.ProviderBinding{}, mediaJobs: map[string]deliverydomain.MediaGenerationJob{}, providerAttempts: map[string]deliverydomain.ProviderAttempt{}, mediaReviews: map[string]deliverydomain.MediaReview{}, gateEvaluations: map[string]reviewdomain.GateEvaluation{}, taskRevisions: map[string]reviewdomain.TaskRevision{}, taskDeliveries: map[string]deliverydomain.TaskDelivery{}, channelBindings: map[string]deliverydomain.ChannelBinding{}, channelPublications: map[string]deliverydomain.ChannelPublication{}, channelCallbackReceipts: map[string]deliverydomain.ChannelCallbackReceipt{}, modelGenerationReceipts: map[string]deliverydomain.ModelGenerationReceipt{}, connectorBindings: map[string]connector.Binding{}, connectorSyncLeases: map[string]connector.SyncLease{}, connectorRecords: map[string]connector.RecordMapping{}, connectorReceipts: map[string]connector.SyncReceipt{},
		snapshots:    map[string]sourcedomain.ContextSnapshot{},
		runtimePlans: map[string]contentruntime.JobPlanRevision{}, runtimeExecutionBindings: map[string]contentruntime.ExecutionBindingSnapshot{}, runtimeJobs: map[string]contentruntime.JobRun{}, runtimeNodes: map[string]contentruntime.NodeRun{}, runtimeFanoutSets: map[string]contentruntime.FanoutSet{}, runtimeFanoutMembers: map[string]contentruntime.FanoutMember{}, runtimeEvents: map[string][]contentruntime.JobEvent{}, runtimeOutbox: map[string]contentruntime.RuntimeOutboxMessage{}, runtimeOutboxReceipts: map[string]runtimeOutboxReceipt{}, runtimeContextViews: map[string]contentruntime.ContextView{}, runtimeAgents: map[string]contentruntime.AgentInstance{}, runtimeAttempts: map[string]contentruntime.RuntimeAttempt{}, runtimeStates: map[string]contentruntime.RuntimeState{}, runtimeStateMutations: map[string]string{}, runtimeCheckpoints: map[string]contentruntime.Checkpoint{}, runtimeEffects: map[string]contentruntime.ExternalEffect{}, runtimeProviderInbox: map[string]contentruntime.ProviderInboxMessage{}, runtimeProviderRecons: map[string]contentruntime.ProviderReconciliation{}, runtimeProviderBills: map[string]contentruntime.ProviderBillRecord{}, runtimeYields: map[string]contentruntime.RuntimeYield{}, runtimeResourceQuotas: map[string]contentruntime.ResourceQuota{}, runtimeReservations: map[string]contentruntime.ResourceReservation{}, runtimeSchemas: map[string]contentruntime.RuntimeSchema{}, runtimeStateCollections: map[string]contentruntime.StateCollection{}, runtimeStateRecords: map[string]contentruntime.StateRecord{}, runtimeToolCalls: map[string]contentruntime.ToolCall{}, runtimeProjections: map[string]contentruntime.RuntimeExplorerView{}, runtimeProjectionRebuilds: map[string]contentruntime.RuntimeProjectionRebuildRun{}, runtimeMaintenance: map[string]contentruntime.RuntimeMaintenanceHeartbeat{},
		approvals: map[string]reviewdomain.ApprovalDecision{}, reviewCycles: map[string]reviewdomain.ReviewCycle{}, reviewComments: map[string]reviewdomain.ReviewComment{}, reviewGrants: map[string]reviewdomain.ReviewGrant{}, submissions: map[string]reviewdomain.Submission{}, submissionRevisions: map[string]reviewdomain.SubmissionRevision{}, approvedSnapshots: map[string]reviewdomain.ApprovedSnapshot{}, artifacts: map[string]deliverydomain.Artifact{}, deliveryPackages: map[string]deliverydomain.DeliveryPackage{}, performanceBatches: map[string]performancedomain.PerformanceImportBatch{}, observations: map[string]performancedomain.PerformanceObservation{}, ratingDecisions: map[string]performancedomain.RatingDecision{}, audits: []auditdomain.AuditEvent{},
	}
}

type runtimeOutboxReceipt struct {
	TenantID      string
	MessageID     string
	Subscriber    string
	Attempts      int
	NextAttemptAt time.Time
	LockedBy      string
	LockedUntil   *time.Time
	DeliveredAt   *time.Time
	LastError     string
	CreatedAt     time.Time
}

func membershipKey(tenantID, userID string) string { return tenantID + ":" + userID }

func (s *Store) CreateUser(_ context.Context, value identitydomain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	email := strings.ToLower(value.Email)
	if _, exists := s.userByEmail[email]; exists {
		return fault.Conflict("EMAIL_EXISTS", "邮箱已注册")
	}
	s.users[value.ID] = value
	s.userByEmail[email] = value.ID
	return nil
}
func (s *Store) UserByEmail(_ context.Context, email string) (identitydomain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.userByEmail[strings.ToLower(email)]
	if !ok {
		return identitydomain.User{}, fault.NotFound("用户")
	}
	return s.users[id], nil
}
func (s *Store) UserByID(_ context.Context, id string) (identitydomain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.users[id]
	if !ok {
		return v, fault.NotFound("用户")
	}
	return v, nil
}
func (s *Store) SaveUser(_ context.Context, v identitydomain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.users[v.ID]
	if !ok {
		return fault.NotFound("用户")
	}
	if !strings.EqualFold(old.Email, v.Email) {
		if _, exists := s.userByEmail[strings.ToLower(v.Email)]; exists {
			return fault.Conflict("EMAIL_EXISTS", "邮箱已注册")
		}
		delete(s.userByEmail, strings.ToLower(old.Email))
		s.userByEmail[strings.ToLower(v.Email)] = v.ID
	}
	s.users[v.ID] = v
	return nil
}
func (s *Store) SaveSession(_ context.Context, v identitydomain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[v.ID] = v
	return nil
}
func (s *Store) SessionByID(_ context.Context, id string) (identitydomain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.sessions[id]
	if !ok || v.RevokedAt != nil || time.Now().After(v.ExpiresAt) {
		return v, fault.NotFound("会话")
	}
	return v, nil
}
func (s *Store) RevokeSession(_ context.Context, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	if !ok {
		return fault.NotFound("会话")
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

func (s *Store) CreateTenant(_ context.Context, t identitydomain.Tenant, m identitydomain.Membership) error {
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
func (s *Store) TenantsForUser(_ context.Context, userID string) ([]identitydomain.Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []identitydomain.Tenant{}
	for _, m := range s.memberships {
		if m.UserID == userID && m.Status == "active" && m.RevokedAt == nil && s.tenants[m.TenantID].Status == "active" {
			out = append(out, s.tenants[m.TenantID])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Membership(_ context.Context, tenantID, userID string) (identitydomain.Membership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.memberships[membershipKey(tenantID, userID)]
	if !ok || v.Status != "active" || v.RevokedAt != nil {
		return v, fault.NotFound("成员关系")
	}
	return v, nil
}

func (s *Store) Memberships(_ context.Context, tenantID string) ([]identitydomain.Membership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []identitydomain.Membership{}
	for _, v := range s.memberships {
		if v.TenantID == tenantID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) PlatformTenants(_ context.Context) ([]identitydomain.PlatformTenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	out := make([]identitydomain.PlatformTenant, 0, len(s.tenants))
	for _, tenant := range s.tenants {
		value := identitydomain.PlatformTenant{Tenant: tenant, ContentTypes: identitydomain.EnabledTenantContentTypes(s.tenantContentCapabilitiesLocked(tenant.ID))}
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
		for _, job := range s.runtimeJobs {
			if job.TenantID != tenant.ID {
				continue
			}
			if job.State == contentruntime.JobRunCreated || job.State == contentruntime.JobRunAdmitted || job.State == contentruntime.JobRunRunning || job.State == contentruntime.JobRunWaitingHuman {
				value.ActiveRunCount++
			}
		}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) TenantContentCapabilities(_ context.Context, tenantID string) ([]identitydomain.TenantContentCapability, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.tenants[tenantID]; !ok {
		return nil, fault.NotFound("租户")
	}
	return s.tenantContentCapabilitiesLocked(tenantID), nil
}

func (s *Store) SetTenantContentCapability(_ context.Context, value identitydomain.TenantContentCapability) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tenants[value.TenantID]; !ok {
		return fault.NotFound("租户")
	}
	s.tenantContentCaps[value.TenantID+":"+value.ContentType] = value
	return nil
}

func (s *Store) tenantContentCapabilitiesLocked(tenantID string) []identitydomain.TenantContentCapability {
	values := []identitydomain.TenantContentCapability{}
	for _, value := range s.tenantContentCaps {
		if value.TenantID == tenantID {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ContentType < values[j].ContentType })
	return values
}

func (s *Store) PlatformUsers(_ context.Context) ([]identitydomain.PlatformUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]identitydomain.PlatformUser, 0, len(s.users))
	for _, user := range s.users {
		value := identitydomain.PlatformUser{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, VerifiedAt: user.VerifiedAt, CreatedAt: user.CreatedAt, Memberships: []identitydomain.PlatformUserMembership{}}
		for _, membership := range s.memberships {
			if membership.UserID == user.ID {
				value.Memberships = append(value.Memberships, identitydomain.PlatformUserMembership{TenantID: membership.TenantID, TenantName: s.tenants[membership.TenantID].Name, Role: membership.Role, Status: membership.Status})
			}
		}
		sort.Slice(value.Memberships, func(i, j int) bool { return value.Memberships[i].TenantName < value.Memberships[j].TenantName })
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) SetTenantStatus(_ context.Context, tenantID, status string, now time.Time) (identitydomain.Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tenant, ok := s.tenants[tenantID]
	if !ok {
		return tenant, fault.NotFound("租户")
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

func (s *Store) SaveMembership(_ context.Context, v identitydomain.Membership) error {
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

func (s *Store) CreateMembershipInvite(_ context.Context, v identitydomain.MembershipInvite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.membershipInvites {
		if existing.TenantID == v.TenantID && strings.EqualFold(existing.Email, v.Email) && existing.Status == "pending" {
			return fault.Conflict("INVITE_EXISTS", "该邮箱已有待接受邀请")
		}
	}
	s.membershipInvites[v.ID] = v
	return nil
}

func (s *Store) MembershipInviteByTokenHash(_ context.Context, hash string) (identitydomain.MembershipInvite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.membershipInvites {
		if v.TokenHash == hash {
			return v, nil
		}
	}
	return identitydomain.MembershipInvite{}, fault.NotFound("成员邀请")
}

func (s *Store) MembershipInvites(_ context.Context, tenantID string) ([]identitydomain.MembershipInvite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []identitydomain.MembershipInvite{}
	for _, v := range s.membershipInvites {
		if v.TenantID == tenantID {
			v.TokenHash, v.PlaintextToken = "", ""
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) SaveMembershipInvite(_ context.Context, v identitydomain.MembershipInvite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.membershipInvites[v.ID]; !ok {
		return fault.NotFound("成员邀请")
	}
	s.membershipInvites[v.ID] = v
	return nil
}

func (s *Store) pendingMembershipInvite(tokenHash, email string, now time.Time) (identitydomain.MembershipInvite, error) {
	for _, invite := range s.membershipInvites {
		if invite.TokenHash != tokenHash {
			continue
		}
		if err := invite.ValidateAcceptance(email, now); err != nil {
			return identitydomain.MembershipInvite{}, err
		}
		return invite, nil
	}
	return identitydomain.MembershipInvite{}, fault.Conflict("INVITE_INVALID", "邀请无效、已撤销、邮箱不匹配或已过期")
}

func (s *Store) redeemMembershipInvite(invite identitydomain.MembershipInvite, userID string, now time.Time) identitydomain.Membership {
	membership := identitydomain.Membership{TenantID: invite.TenantID, UserID: userID, Role: invite.Role, Status: "active", CreatedAt: now}
	s.memberships[membershipKey(membership.TenantID, membership.UserID)] = membership
	invite.Status, invite.AcceptedBy, invite.AcceptedAt = "accepted", userID, &now
	s.membershipInvites[invite.ID] = invite
	return membership
}

func (s *Store) AcceptMembershipInvite(_ context.Context, tokenHash string, user identitydomain.User, now time.Time) (identitydomain.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[user.ID]; !ok {
		return identitydomain.Membership{}, fault.NotFound("用户")
	}
	invite, err := s.pendingMembershipInvite(tokenHash, user.Email, now)
	if err != nil {
		return identitydomain.Membership{}, err
	}
	return s.redeemMembershipInvite(invite, user.ID, now), nil
}

func (s *Store) RegisterWithInvite(_ context.Context, user identitydomain.User, tokenHash string, session identitydomain.Session, now time.Time) (identitydomain.Session, identitydomain.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, err := s.pendingMembershipInvite(tokenHash, user.Email, now)
	if err != nil {
		return identitydomain.Session{}, identitydomain.Membership{}, err
	}
	email := strings.ToLower(user.Email)
	if _, exists := s.userByEmail[email]; exists {
		return identitydomain.Session{}, identitydomain.Membership{}, fault.Conflict("EMAIL_EXISTS", "邮箱已注册")
	}
	session.UserID, session.TenantID = user.ID, invite.TenantID
	membership := s.redeemMembershipInvite(invite, user.ID, now)
	s.users[user.ID] = user
	s.userByEmail[email] = user.ID
	s.sessions[session.ID] = session
	return session, membership, nil
}

func (s *Store) CreateProject(_ context.Context, v workspacedomain.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[v.ID] = v
	return nil
}
func (s *Store) Projects(_ context.Context, tenantID string) ([]workspacedomain.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []workspacedomain.Project{}
	for _, v := range s.projects {
		if v.TenantID == tenantID {
			out = append(out, s.projectWithKnowledgeMetricsLocked(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) Project(_ context.Context, tenantID, id string) (workspacedomain.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.projects[id]
	if !ok || v.TenantID != tenantID {
		return v, fault.NotFound("项目")
	}
	return s.projectWithKnowledgeMetricsLocked(v), nil
}

func (s *Store) projectWithKnowledgeMetricsLocked(project workspacedomain.Project) workspacedomain.Project {
	project.KnowledgeReady = 0
	project.OpenBlockers = 0
	for _, object := range s.knowledgeObjects {
		if object.TenantID != project.TenantID || object.ProjectID != project.ID {
			continue
		}
		switch object.Status {
		case "verified", "approved", "valid", "active":
			project.KnowledgeReady++
		case "candidate", "needs_review", "conflicted", "blocked", "open":
			project.OpenBlockers++
		}
	}
	return project
}
func (s *Store) SaveProject(_ context.Context, v workspacedomain.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.projects[v.ID]; !ok || old.TenantID != v.TenantID {
		return fault.NotFound("项目")
	}
	s.projects[v.ID] = v
	return nil
}

func (s *Store) UpdateProject(_ context.Context, v workspacedomain.Project, expectedVersion int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.projects[v.ID]
	if !ok || old.TenantID != v.TenantID {
		return fault.NotFound("项目")
	}
	if old.RowVersion != expectedVersion {
		return fault.Conflict("ROW_VERSION_CONFLICT", "项目已被其他用户修改")
	}
	s.projects[v.ID] = v
	return nil
}

func (s *Store) CreateProjectTemplate(_ context.Context, v workspacedomain.ProjectTemplate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.projectTemplates {
		if existing.TenantID == v.TenantID && existing.Name == v.Name {
			return fault.Conflict("PROJECT_TEMPLATE_EXISTS", "项目模板名称已存在")
		}
	}
	s.projectTemplates[v.ID] = v
	return nil
}

func (s *Store) ProjectTemplates(_ context.Context, tenantID string) ([]workspacedomain.ProjectTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []workspacedomain.ProjectTemplate{}
	for _, v := range s.projectTemplates {
		if v.TenantID == tenantID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) ProjectTemplate(_ context.Context, tenantID, id string) (workspacedomain.ProjectTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.projectTemplates[id]
	if !ok || v.TenantID != tenantID {
		return v, fault.NotFound("项目模板")
	}
	return v, nil
}

func (s *Store) CreateConnectSession(_ context.Context, v workspacedomain.ConnectSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connects[v.ID] = v
	return nil
}
func (s *Store) ConnectSessionByID(_ context.Context, tenantID, id string) (workspacedomain.ConnectSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.connects[id]
	if !ok || v.TenantID != tenantID {
		return v, fault.NotFound("连接会话")
	}
	return v, nil
}
func (s *Store) SaveConnectSession(_ context.Context, v workspacedomain.ConnectSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.connects[v.ID]
	if !ok || old.TenantID != v.TenantID {
		return fault.NotFound("连接会话")
	}
	s.connects[v.ID] = v
	return nil
}
func (s *Store) SaveDevice(_ context.Context, v workspacedomain.Device) error {
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
func (s *Store) RotateDeviceCredential(_ context.Context, tenantID, deviceID, tokenHash string, now time.Time) (workspacedomain.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	device, ok := s.devices[deviceID]
	if !ok || device.TenantID != tenantID || device.RevokedAt != nil {
		return device, fault.NotFound("设备")
	}
	device.TokenHash = tokenHash
	device.CredentialVersion++
	if device.CredentialVersion < 1 {
		device.CredentialVersion = 1
	}
	device.CredentialRotatedAt = now
	s.devices[deviceID] = device
	device.TokenHash = ""
	return device, nil
}
func (s *Store) DeviceByTokenHash(_ context.Context, hash string) (workspacedomain.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.devices {
		if v.TokenHash == hash && v.RevokedAt == nil {
			return v, nil
		}
	}
	return workspacedomain.Device{}, fault.NotFound("设备")
}
func (s *Store) Devices(_ context.Context, tenantID, projectID string) ([]workspacedomain.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []workspacedomain.Device{}
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

func (s *Store) Device(_ context.Context, tenantID, id string) (workspacedomain.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.devices[id]
	if !ok || v.TenantID != tenantID {
		return v, fault.NotFound("设备")
	}
	v.TokenHash = ""
	return v, nil
}

func (s *Store) SaveDaemonInstance(_ context.Context, value workspacedomain.DaemonInstance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	device, ok := s.devices[value.DeviceID]
	if !ok || device.TenantID != value.TenantID || device.RevokedAt != nil {
		return fault.NotFound("设备")
	}
	if _, exists := s.daemonInstances[value.ID]; !exists && value.ConnectionEpoch == 1 && value.ReportSequence == 1 && value.State != "stopped" {
		for id, other := range s.daemonInstances {
			if id == value.ID || other.TenantID != value.TenantID || other.DeviceID != value.DeviceID || other.State == "stopped" {
				continue
			}
			stoppedAt := value.LastSeenAt
			other.State = "stopped"
			other.StoppedAt = &stoppedAt
			s.daemonInstances[id] = other
		}
	}
	if existing, ok := s.daemonInstances[value.ID]; ok {
		stale := value.ConnectionEpoch < existing.ConnectionEpoch ||
			(value.ConnectionEpoch == existing.ConnectionEpoch && value.ReportSequence <= existing.ReportSequence) ||
			(value.ConnectionEpoch == existing.ConnectionEpoch && existing.State == "stopped" && value.State != "stopped")
		for _, other := range s.daemonInstances {
			if other.ID != existing.ID && other.TenantID == existing.TenantID && other.DeviceID == existing.DeviceID && other.State != "stopped" {
				stale = true
				break
			}
		}
		if existing.TenantID != value.TenantID || existing.DeviceID != value.DeviceID || stale {
			return fault.Conflict("DAEMON_INSTANCE_REPORT_STALE", "DaemonInstance 状态报告已过期")
		}
	}
	s.daemonInstances[value.ID] = value
	return nil
}

func (s *Store) DaemonInstance(_ context.Context, tenantID, id string) (workspacedomain.DaemonInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.daemonInstances[id]
	if !ok || value.TenantID != tenantID {
		return value, fault.NotFound("DaemonInstance")
	}
	return value, nil
}

func (s *Store) DaemonInstances(_ context.Context, tenantID, deviceID string) ([]workspacedomain.DaemonInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := []workspacedomain.DaemonInstance{}
	for _, value := range s.daemonInstances {
		if value.TenantID == tenantID && (deviceID == "" || value.DeviceID == deviceID) {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].LastSeenAt.After(values[j].LastSeenAt) })
	return values, nil
}

func (s *Store) GrantDeviceProject(_ context.Context, tenantID, projectID, deviceID, _ string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	device, ok := s.devices[deviceID]
	if !ok || device.TenantID != tenantID || device.RevokedAt != nil {
		return fault.NotFound("设备")
	}
	project, ok := s.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return fault.NotFound("项目")
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
		return fault.NotFound("设备")
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
		return fault.NotFound("项目设备授权")
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
		return fault.NotFound("设备")
	}
	v.RevokedAt = &now
	s.devices[id] = v
	return nil
}

func (s *Store) CreateUserDeviceFlow(_ context.Context, v workspacedomain.UserDeviceFlow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userDeviceFlows[v.ID] = v
	return nil
}

func (s *Store) UserDeviceFlowByCodeHash(_ context.Context, hash string) (workspacedomain.UserDeviceFlow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.userDeviceFlows {
		if v.DeviceCodeHash == hash {
			return v, nil
		}
	}
	return workspacedomain.UserDeviceFlow{}, fault.NotFound("登录授权")
}

func (s *Store) UserDeviceFlowByUserCode(_ context.Context, code string) (workspacedomain.UserDeviceFlow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.userDeviceFlows {
		if strings.EqualFold(v.UserCode, code) {
			return v, nil
		}
	}
	return workspacedomain.UserDeviceFlow{}, fault.NotFound("登录授权")
}

func (s *Store) SaveUserDeviceFlow(_ context.Context, v workspacedomain.UserDeviceFlow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.userDeviceFlows[v.ID]; !ok {
		return fault.NotFound("登录授权")
	}
	s.userDeviceFlows[v.ID] = v
	return nil
}

func (s *Store) CreateCLIToken(_ context.Context, v workspacedomain.CLIToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cliTokens[v.ID] = v
	return nil
}

func (s *Store) CLITokenByHash(_ context.Context, hash string) (workspacedomain.CLIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.cliTokens {
		if v.TokenHash == hash && v.RevokedAt == nil && time.Now().Before(v.ExpiresAt) {
			return v, nil
		}
	}
	return workspacedomain.CLIToken{}, fault.NotFound("CLI 凭据")
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
	return fault.NotFound("CLI 凭据")
}

func (s *Store) CreateSnapshot(_ context.Context, v sourcedomain.ContextSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[v.ID] = v
	return nil
}
func (s *Store) Snapshot(_ context.Context, tenantID, id string) (sourcedomain.ContextSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.snapshots[id]
	if !ok || v.TenantID != tenantID {
		return v, fault.NotFound("上下文快照")
	}
	return v, nil
}
func (s *Store) CreateApproval(_ context.Context, v reviewdomain.ApprovalDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.DecisionStage == "" {
		v.DecisionStage = "internal"
	}
	s.approvals[v.ID] = v
	return nil
}
func (s *Store) Approvals(_ context.Context, tenantID, subjectID string) ([]reviewdomain.ApprovalDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []reviewdomain.ApprovalDecision{}
	for _, v := range s.approvals {
		if v.TenantID == tenantID && (subjectID == "" || v.SubjectID == subjectID) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) AppendAudit(_ context.Context, v auditdomain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits = append(s.audits, v)
	return nil
}
func (s *Store) AuditEvents(_ context.Context, tenantID, projectID string, limit int) ([]auditdomain.AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []auditdomain.AuditEvent{}
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
