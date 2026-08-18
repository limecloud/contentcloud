package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/application"
	fixturev3 "github.com/limecloud/contentcloud/internal/bootstrap/fixture"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type Server struct {
	service                 *application.Application
	log                     *slog.Logger
	devMode                 bool
	webDist                 string
	devBootstrapMu          sync.Mutex
	providerCallbackSecrets map[string][]byte
	channelCallbackSecrets  map[string][]byte
	agentCallbackSecrets    map[string][]byte
	runtimeWakeHub          *runtimeWakeHub
	runtimeWakeContext      context.Context
}

type envelope struct {
	OK        bool           `json:"ok"`
	Command   string         `json:"command"`
	RequestID string         `json:"request_id"`
	Data      any            `json:"data,omitempty"`
	Meta      map[string]any `json:"meta"`
	Error     *fault.Error   `json:"error,omitempty"`
}

type actorKey struct{}

type Option func(*Server)

func WithRuntimeWakeContext(ctx context.Context) Option {
	return func(server *Server) {
		if ctx != nil {
			server.runtimeWakeContext = ctx
		}
	}
}

// WithProviderCallbackSecret registers an ingress-only HMAC secret. Secrets
// are keyed by the authenticated tenant/provider pair and are never exposed
// in response DTOs.
func WithProviderCallbackSecret(tenantID, providerID string, secret []byte) Option {
	return func(server *Server) {
		if server.providerCallbackSecrets == nil {
			server.providerCallbackSecrets = map[string][]byte{}
		}
		key := strings.TrimSpace(tenantID) + ":" + strings.ToLower(strings.TrimSpace(providerID))
		if key != ":" && len(secret) > 0 {
			server.providerCallbackSecrets[key] = append([]byte(nil), secret...)
		}
	}
}

func WithChannelCallbackSecret(tenantID, adapterID string, secret []byte) Option {
	return func(server *Server) {
		if server.channelCallbackSecrets == nil {
			server.channelCallbackSecrets = map[string][]byte{}
		}
		key := strings.TrimSpace(tenantID) + ":" + strings.ToLower(strings.TrimSpace(adapterID))
		if key != ":" && len(secret) > 0 {
			server.channelCallbackSecrets[key] = append([]byte(nil), secret...)
		}
	}
}

func WithAgentCallbackSecret(tenantID, harnessKind string, secret []byte) Option {
	return func(server *Server) {
		if server.agentCallbackSecrets == nil {
			server.agentCallbackSecrets = map[string][]byte{}
		}
		key := strings.TrimSpace(tenantID) + ":" + strings.ToLower(strings.TrimSpace(harnessKind))
		if key != ":" && len(secret) > 0 {
			server.agentCallbackSecrets[key] = append([]byte(nil), secret...)
		}
	}
}

func New(service *application.Application, logger *slog.Logger, devMode bool, webDist string, options ...Option) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	server := &Server{service: service, log: logger, devMode: devMode, webDist: webDist, providerCallbackSecrets: map[string][]byte{}, channelCallbackSecrets: map[string][]byte{}, agentCallbackSecrets: map[string][]byte{}, runtimeWakeHub: newRuntimeWakeHub(), runtimeWakeContext: context.Background()}
	for _, option := range options {
		option(server)
	}
	if service != nil && service.Runtime.Runtime() != nil {
		service.Runtime.Runtime().SetAvailableNotifier(server.publishRuntimeWake)
		if service.Runtime.HasRuntimeWakeBroker() {
			go func() {
				if err := service.Runtime.ListenRuntimeWakes(server.runtimeWakeContext, server.runtimeWakeHub.publish); err != nil && server.runtimeWakeContext.Err() == nil {
					server.log.Error("runtime wake listener stopped", "error", err)
				}
			}()
		}
	}
	return server
}

func (s *Server) publishRuntimeWake(tenantID string) {
	s.runtimeWakeHub.publish(tenantID)
	ctx, cancel := context.WithTimeout(s.runtimeWakeContext, 2*time.Second)
	defer cancel()
	if err := s.service.Runtime.PublishRuntimeWake(ctx, tenantID); err != nil && s.runtimeWakeContext.Err() == nil {
		s.log.Warn("runtime cross-instance wake publish failed", "tenant_id", tenantID, "error", err)
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, s.securityHeaders, s.accessLog)
	r.Get("/healthz", s.health)
	r.Get("/codex", codex)
	r.Get("/api/docs/catalog", s.docsCatalog)
	r.Get("/api/docs/pages/*", s.docsPage)
	r.Get("/api/bootstrap", s.bootstrap)
	r.Get("/api/bootstrap/actions", s.bootstrapActions)
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", s.register)
		r.Post("/auth/login", s.login)
		if s.devMode {
			r.Post("/dev/bootstrap", s.devBootstrap)
		}
		r.Post("/cli/dispatch", s.dispatch)
		r.Get("/runtime/worker/control", s.runtimeWorkerControl)
		r.Post("/runtime/mcp/call", s.runtimeGatewayCall)
		r.Post("/providers/{providerID}/tenants/{tenantID}/callbacks", s.providerCallback)
		r.Post("/providers/{providerID}/tenants/{tenantID}/bills", s.providerBill)
		r.Post("/channels/{adapterID}/tenants/{tenantID}/callbacks", s.channelCallback)
		r.Post("/agent-harnesses/{harnessKind}/tenants/{tenantID}/callbacks", s.agentCallback)
		r.Route("/admin", func(r chi.Router) {
			r.Use(s.requireSession)
			r.Get("/dashboard", s.platformOverview)
			r.Get("/runtime-health", s.platformRuntimeHealth)
			r.Patch("/tenants/{tenantID}", s.updatePlatformTenant)
			r.Put("/tenants/{tenantID}/content-capabilities/{contentType}", s.updatePlatformTenantContentCapability)
		})
	})
	r.Route("/api/review/{token}", func(r chi.Router) {
		r.Get("/projection", s.publicReviewProjection)
		r.Post("/verify", s.publicReviewVerify)
		r.Post("/decision", s.publicReviewDecision)
	})
	r.Route("/api/studio", func(r chi.Router) {
		r.Use(s.requireSession)
		r.Get("/bootstrap", s.customerStudioBootstrap)
		r.Get("/execution-clients", s.customerStudioExecutionClients)
		r.Post("/projects/{projectID}/connect-sessions", s.createCustomerStudioConnectSession)
		r.Get("/connect-sessions/{sessionID}", s.customerStudioConnectSession)
		r.Post("/connect-sessions/{sessionID}/approve", s.approveCustomerStudioConnectSession)
		r.Post("/connect-sessions/{sessionID}/deny", s.denyCustomerStudioConnectSession)
		r.Post("/connect-sessions/{sessionID}/cancel", s.cancelCustomerStudioConnectSession)
		r.Post("/session/switch", s.switchTenant)
		r.Post("/session/logout", s.logout)
		r.Get("/team/members", s.members)
		r.Patch("/team/members/{userID}", s.updateMembershipRole)
		r.Post("/team/members/{userID}/revoke", s.revokeMembership)
		r.Get("/team/invites", s.membershipInvites)
		r.Post("/team/invites", s.createMembershipInvite)
		r.Post("/team/invites/accept", s.acceptMembershipInvite)
		r.Post("/team/invites/{id}/revoke", s.revokeMembershipInvite)
		r.Get("/tasks", s.customerStudioTasks)
		r.Post("/tasks", s.createCustomerStudioTask)
		r.Get("/tasks/{taskID}", s.customerStudioTask)
		r.Post("/tasks/{taskID}/actions", s.customerStudioTaskAction)
		r.Post("/tasks/{taskID}/inspirations", s.addCustomerStudioInspiration)
		r.Post("/tasks/{taskID}/decisions/{decisionID}", s.decideCustomerStudioTask)
		r.Post("/tasks/{taskID}/assets", s.attachCustomerStudioAssets)
		r.Post("/tasks/{taskID}/materials", s.attachCustomerStudioMaterials)
		r.Post("/asset-folders", s.createCustomerStudioFolder)
		r.Post("/materials", s.uploadCustomerStudioMaterial)
		r.Get("/materials/{id}/download", s.downloadCustomerStudioMaterial)
		r.Get("/assets", s.customerStudioAssets)
		r.Get("/assets/results/{resultID}", s.customerStudioCreativeResult)
		r.Get("/deliveries", s.customerStudioDeliveries)
		r.Get("/artifacts/{id}/download", s.downloadArtifact)
		r.Get("/projects/{projectID}/knowledge-objects", s.knowledgeObjects)
		r.Post("/knowledge-objects/{id}/transitions", s.transitionKnowledgeObject)
		r.Get("/projects/{projectID}/knowledge-packs", s.knowledgePacks)
		r.Post("/projects/{projectID}/knowledge-packs", s.createKnowledgePack)
		r.Post("/knowledge-packs/{id}/publish", s.publishKnowledgePack)
		r.Get("/projects/{projectID}/knowledge-packs/{packID}/snapshots", s.knowledgeSnapshots)
		r.Post("/knowledge/query", s.queryKnowledge)
		r.Get("/projects/{projectID}/sources", s.sources)
		r.Post("/projects/{projectID}/sources/search", s.searchSources)
		r.Post("/projects/{projectID}/sources/fetch", s.fetchSource)
		r.Post("/projects/{projectID}/sources/upload", s.uploadSource)
		r.Get("/sources/{id}/revisions", s.sourceRevisions)
		r.Post("/sources/{sourceID}/revisions/upload", s.uploadSourceRevision)
		r.Get("/source-revisions/{id}/evidence", s.evidence)
	})
	r.Route("/api/bff", func(r chi.Router) {
		r.Use(s.requireSession)
		r.Get("/session", s.session)
		r.Post("/session/switch", s.switchTenant)
		r.Post("/session/logout", s.logout)
		r.Get("/tenants", s.tenants)
		r.Get("/team/members", s.members)
		r.Patch("/team/members/{userID}", s.updateMembershipRole)
		r.Post("/team/members/{userID}/revoke", s.revokeMembership)
		r.Get("/team/invites", s.membershipInvites)
		r.Post("/team/invites", s.createMembershipInvite)
		r.Post("/team/invites/accept", s.acceptMembershipInvite)
		r.Post("/team/invites/{id}/revoke", s.revokeMembershipInvite)
		r.Get("/dashboard", s.dashboard)
		r.Get("/admin/work-os", s.adminWorkOS)
		r.Get("/operations/executors", s.operationsExecutors)
		r.Get("/operations/executors/{executorID}", s.operationsExecutor)
		r.Get("/operations/skills", s.operationsSkills)
		r.Get("/operations/skills/{skillID}/versions/{skillVersion}", s.operationsSkill)
		r.Get("/runtime/jobs", s.runtimeJobs)
		r.Get("/runtime/jobs/{jobID}", s.runtimeJob)
		r.Get("/runtime/jobs/{jobID}/nodes", s.runtimeJobNodes)
		r.Get("/runtime/jobs/{jobID}/events", s.runtimeJobEvents)
		r.Get("/runtime/jobs/{jobID}/events/stream", s.runtimeJobEventsStream)
		r.Get("/runtime/jobs/{jobID}/effects", s.runtimeJobEffects)
		r.Get("/runtime/jobs/{jobID}/checkpoints", s.runtimeJobCheckpoints)
		r.Post("/runtime/jobs/{jobID}/refresh", s.refreshRuntimeJob)
		r.Post("/runtime/jobs/{jobID}/pause", s.pauseRuntimeJob)
		r.Post("/runtime/jobs/{jobID}/cancel", s.cancelRuntimeJob)
		r.Post("/runtime/jobs/{jobID}/resume", s.resumeRuntimeJob)
		r.Post("/runtime/jobs/{jobID}/replay", s.replayRuntimeJob)
		r.Post("/runtime/checkpoints/{checkpointID}/fork", s.forkRuntimeCheckpoint)
		r.Post("/runtime/effects/{effectID}/reconcile", s.reconcileRuntimeEffect)
		r.Post("/admin/environments", s.createAdminEnvironment)
		r.Patch("/admin/environments/{id}", s.updateAdminEnvironment)
		r.Post("/admin/sops", s.createAdminSOP)
		r.Patch("/admin/sops/{sopID}/versions/{version}", s.updateAdminSOPVersion)
		r.Post("/admin/sops/{sopID}/versions", s.createAdminSOPDraft)
		r.Get("/admin/sops/{sopID}/versions/{version}/lint", s.lintAdminSOPVersion)
		r.Get("/admin/sops/{sopID}/versions/{version}/impact", s.impactAdminSOPVersion)
		r.Get("/admin/sops/{sopID}/versions/{version}/preview", s.previewAdminSOPVersion)
		r.Post("/admin/sops/{sopID}/versions/{version}/retire", s.retireAdminSOPVersion)
		r.Post("/admin/sops/{sopID}/versions/{version}/publish", s.publishAdminSOPVersion)
		r.Get("/admin/sops/{sopID}/versions/{fromVersion}/diff/{toVersion}", s.diffAdminSOPVersions)
		r.Post("/admin/sops/{sopID}/rollback", s.rollbackAdminSOPVersion)
		r.Get("/agent-clients", s.agentClients)
		r.Get("/agent-harnesses", s.agentHarnesses)
		r.Get("/projects", s.projects)
		r.Post("/projects", s.createProject)
		r.Get("/projects/{projectID}", s.project)
		r.Get("/projects/{projectID}/sop", s.projectSOP)
		r.Patch("/projects/{projectID}/sop", s.bindProjectSOP)
		r.Get("/projects/{projectID}/knowledge-objects", s.knowledgeObjects)
		r.Post("/projects/{projectID}/knowledge-objects", s.createKnowledgeObject)
		r.Post("/knowledge-objects/{id}/transitions", s.transitionKnowledgeObject)
		r.Get("/knowledge-objects/{id}/decisions", s.knowledgeDecisions)
		r.Get("/projects/{projectID}/knowledge-packs", s.knowledgePacks)
		r.Post("/projects/{projectID}/knowledge-packs", s.createKnowledgePack)
		r.Post("/knowledge-packs/{id}/publish", s.publishKnowledgePack)
		r.Get("/projects/{projectID}/knowledge-packs/{packID}/snapshots", s.knowledgeSnapshots)
		r.Get("/knowledge-snapshots/{id}", s.knowledgeSnapshot)
		r.Post("/knowledge/query", s.queryKnowledge)
		r.Get("/projects/{projectID}/sources", s.sources)
		r.Post("/projects/{projectID}/sources", s.createSource)
		r.Post("/projects/{projectID}/sources/search", s.searchSources)
		r.Post("/projects/{projectID}/sources/fetch", s.fetchSource)
		r.Post("/projects/{projectID}/sources/upload", s.uploadSource)
		r.Get("/sources/{id}/revisions", s.sourceRevisions)
		r.Post("/sources/{sourceID}/revisions/upload", s.uploadSourceRevision)
		r.Get("/source-revisions/{id}", s.sourceRevision)
		r.Get("/sources/{id}/impact", s.sourceImpact)
		r.Get("/source-revisions/{id}/evidence", s.evidence)
		r.Post("/evidence/{id}/review", s.reviewEvidence)
		r.Get("/projects/{projectID}/projection", s.projectProjection)
		r.Get("/projects/{projectID}/agent-handoff", s.projectAgentHandoff)
		r.Patch("/projects/{projectID}", s.updateProject)
		r.Post("/projects/{projectID}/archive", s.archiveProject)
		r.Post("/projects/{projectID}/restore", s.restoreProject)
		r.Get("/tasks", s.workTasks)
		r.Post("/tasks", s.createWorkTask)
		r.Get("/input-items", s.inputItems)
		r.Post("/input-items", s.createInputItem)
		r.Get("/input-items/{id}", s.inputItem)
		r.Post("/input-items/{id}/triage", s.triageInputItem)
		r.Get("/tasks/{taskID}", s.workTask)
		r.Post("/tasks/{taskID}/actions", s.taskAction)
		r.Post("/tasks/{taskID}/stages/{stageID}/report", s.reportStage)
		r.Post("/tasks/{taskID}/media-jobs", s.createMediaGenerationJob)
		r.Post("/tasks/{taskID}/storyboard-artifacts", s.uploadStoryboardArtifact)
		r.Post("/tasks/{taskID}/seedance-prompt-package", s.uploadSeedancePromptPackage)
		r.Post("/tasks/{taskID}/final-render", s.createFinalRender)
		r.Post("/media-jobs/{id}/approve-cost", s.approveMediaGenerationJob)
		r.Post("/media-jobs/{id}/cancel", s.cancelMediaGenerationJob)
		r.Post("/media-jobs/{id}/reconcile-submit", s.reconcileMediaGenerationSubmit)
		r.Post("/media-reviews/{id}/decide", s.decideMediaReview)
		r.Post("/tasks/{taskID}/delivery-package", s.buildTaskDeliveryPackage)
		r.Get("/tasks/{taskID}/conversation-imports", s.taskConversationImports)
		r.Post("/tasks/{taskID}/conversation-imports", s.createConversationImport)
		r.Get("/tasks/{taskID}/runs", s.runsForTask)
		r.Get("/tasks/{taskID}/gates", s.taskGates)
		r.Post("/tasks/{taskID}/gates/{gateID}/decide", s.decideGate)
		r.Get("/tasks/{taskID}/revisions", s.taskRevisions)
		r.Post("/tasks/{taskID}/revisions", s.createTaskRevision)
		r.Get("/tasks/{taskID}/deliveries", s.taskDeliveries)
		r.Post("/tasks/{taskID}/deliveries", s.createTaskDelivery)
		r.Get("/channel-adapters", s.channelAdapters)
		r.Get("/projects/{projectID}/channel-bindings", s.channelBindings)
		r.Post("/projects/{projectID}/channel-bindings", s.createChannelBinding)
		r.Get("/channel-publications", s.channelPublications)
		r.Post("/channel-publications", s.prepareChannelPublication)
		r.Post("/channel-publications/reconcile", s.reconcileChannelPublications)
		r.Post("/channel-publications/{id}/submit", s.submitChannelPublication)
		r.Post("/channel-publications/{id}/inspect", s.inspectChannelPublication)
		r.Post("/channel-publications/{id}/receipt", s.recordManualChannelReceipt)
		r.Post("/channel-publications/{id}/withdraw", s.withdrawChannelPublication)
		r.Post("/channel-publications/{id}/performance", s.importChannelPerformance)
		r.Get("/model-providers", s.modelProviders)
		r.Post("/tasks/{taskID}/model-candidates", s.generateModelCandidate)
		r.Get("/tasks/{taskID}/model-receipts", s.modelGenerationReceipts)
		r.Get("/connector-adapters", s.connectorAdapters)
		r.Get("/projects/{projectID}/connector-bindings", s.connectorBindings)
		r.Post("/projects/{projectID}/connector-bindings", s.createConnectorBinding)
		r.Post("/connector-bindings/{id}/sync", s.syncConnector)
		r.Get("/connector-receipts", s.connectorReceipts)
		r.Get("/content-profiles", s.contentProfiles)
		r.Post("/content-profiles/{profileID}/install", s.installContentProfile)
		r.Get("/provider-profiles", s.availableProviderProfiles)
		r.Get("/provider-bindings/{providerID}", s.providerBinding)
		r.Put("/provider-bindings/{providerID}", s.saveProviderBinding)
		r.Get("/admin/provider-profiles", s.providerProfiles)
		r.Post("/admin/provider-profiles", s.createProviderProfile)
		r.Get("/admin/provider-profiles/{providerID}/{version}", s.providerProfile)
		r.Post("/admin/provider-profiles/{providerID}/{version}/publish", s.publishProviderProfile)
		r.Get("/admin/tenants/{tenantID}/provider-bindings/{providerID}", s.adminProviderBinding)
		r.Put("/admin/tenants/{tenantID}/provider-bindings/{providerID}", s.saveAdminProviderBinding)
		r.Get("/conversation-imports/{id}", s.conversationImport)
		r.Post("/conversation-imports/{id}/bundle", s.submitConversationBundle)
		r.Post("/conversation-imports/{id}/cancel", s.cancelConversationImport)
		r.Get("/project-templates", s.projectTemplates)
		r.Post("/project-templates", s.createProjectTemplate)
		r.Post("/projects/{projectID}/connect-sessions", s.createConnect)
		r.Get("/projects/{projectID}/bootstrap-attempts/{attemptID}", s.bootstrapAuthorizationView)
		r.Get("/connect-sessions/{id}", s.connectStatus)
		r.Post("/connect-sessions/{id}/cancel", s.cancelConnectSession)
		r.Post("/connect-sessions/{id}/attempts/{attemptID}/approve", s.approveBootstrapAuthorization)
		r.Post("/connect-sessions/{id}/attempts/{attemptID}/deny", s.denyBootstrapAuthorization)
		r.Get("/projects/{projectID}/devices", s.devices)
		r.Get("/devices/{id}", s.device)
		r.Post("/projects/{projectID}/devices/{id}/attach", s.attachDevice)
		r.Post("/projects/{projectID}/devices/{id}/detach", s.detachDevice)
		r.Post("/devices/{id}/revoke", s.revokeDevice)
		r.Post("/devices/{id}/credentials/rotate", s.rotateDeviceCredential)
		r.Post("/device-auth/approve", s.approveDeviceAuth)
		r.Get("/projects/{projectID}/runs", s.runs)
		r.Get("/runs/{id}", s.run)
		r.Get("/runs/{id}/progress", s.runProgress)
		r.Get("/runs/{id}/progress/stream", s.runProgressStream)
		r.Post("/runs/{id}/cancel", s.cancelRun)
		r.Post("/comments/{id}/resolve", s.resolveReviewComment)
		r.Get("/submission-revisions/{id}/review-grants", s.reviewGrants)
		r.Post("/submission-revisions/{id}/review-grants", s.createReviewGrant)
		r.Post("/review-grants/{id}/revoke", s.revokeReviewGrant)
		r.Get("/artifacts/{id}/download", s.downloadArtifact)
		r.Get("/projects/{projectID}/audit", s.audit)
		r.Get("/projects/{projectID}/submissions", s.submissions)
		r.Get("/projects/{projectID}/submission-revisions/{id}", s.projectSubmissionRevision)
		r.Get("/projects/{projectID}/submission-revisions/{id}/agent-handoff", s.reviewFeedbackAgentHandoff)
		r.Get("/submissions/{id}", s.submissionDetails)
		r.Post("/submission-revisions/{id}/approve", s.approveSubmission)
		r.Post("/submission-revisions/{id}/request-changes", s.requestSubmissionChanges)
		r.Get("/projects/{projectID}/approved-snapshots", s.approvedSnapshots)
		r.Get("/approved-snapshots/{id}/artifacts", s.approvedSnapshotArtifacts)
		r.Get("/projects/{projectID}/delivery-packages", s.deliveryPackages)
		r.Get("/delivery-packages/{id}", s.deliveryPackage)
	})
	r.Get("/*", s.static)
	return r
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.URL.Path == "/docs" || strings.HasPrefix(r.URL.Path, "/docs/") || strings.HasPrefix(r.URL.Path, "/api/docs/") {
			w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Info("http request", "method", r.Method, "path", r.URL.Path, "request_id", middleware.GetReqID(r.Context()), "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	s.ok(w, r, "health", map[string]any{"status": "ok", "service": "contentcloud-server", "zero_exec": true})
}

type authInput struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	TenantName  string `json:"tenant_name"`
	InviteToken string `json:"invite_token"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in authInput
	if !s.decode(w, r, &in) {
		return
	}
	var (
		session identitydomain.Session
		err     error
	)
	if strings.TrimSpace(in.InviteToken) != "" {
		session, err = s.service.Identity.RegisterWithInvite(r.Context(), in.Email, in.Password, in.DisplayName, strings.TrimSpace(in.InviteToken))
	} else {
		session, err = s.service.Identity.Register(r.Context(), in.Email, in.Password, in.DisplayName, in.TenantName)
	}
	if err != nil {
		s.fail(w, r, "auth.register", err)
		return
	}
	s.setSession(w, r, session)
	s.ok(w, r, "auth.register", map[string]any{"expires_at": session.ExpiresAt})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in authInput
	if !s.decode(w, r, &in) {
		return
	}
	session, err := s.service.Identity.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		s.fail(w, r, "auth.login", err)
		return
	}
	s.setSession(w, r, session)
	s.ok(w, r, "auth.login", map[string]any{"expires_at": session.ExpiresAt})
}
func (s *Server) devBootstrap(w http.ResponseWriter, r *http.Request) {
	s.devBootstrapMu.Lock()
	defer s.devBootstrapMu.Unlock()

	session, err := s.service.Identity.Login(r.Context(), "demo@contentcloud.local", "contentcloud-demo-2026")
	if err != nil {
		session, err = s.service.Identity.Register(r.Context(), "demo@contentcloud.local", "contentcloud-demo-2026", "林舟", "南京澄观内容科技")
	}
	if err != nil {
		s.fail(w, r, "dev.bootstrap", err)
		return
	}
	s.setSession(w, r, session)
	actor, _, _ := s.service.Identity.SessionActor(r.Context(), session.ID)
	data := map[string]any{"ready": true}
	if actor.PlatformAdmin {
		result, fixtureErr := s.service.Operations.EnsureMarketingVideoDemoFixture(r.Context(), actor, middleware.GetReqID(r.Context()))
		if fixtureErr != nil {
			s.fail(w, r, "dev.bootstrap", fixtureErr)
			return
		}
		data["marketing_video_fixture"] = result
	}
	fixture, err := fixturev3.Decode(r.Body)
	if err == nil {
		result, importErr := s.service.Review.ImportFixtureV3(r.Context(), actor, fixture, middleware.GetReqID(r.Context()))
		if importErr != nil {
			s.fail(w, r, "dev.bootstrap", importErr)
			return
		}
		data["fixture"] = result
	} else if !errors.Is(err, io.EOF) {
		s.fail(w, r, "dev.bootstrap", fault.Invalid("FIXTURE_V3_INVALID", err.Error()))
		return
	}
	s.ok(w, r, "dev.bootstrap", data)
}
func (s *Server) setSession(w http.ResponseWriter, r *http.Request, v identitydomain.Session) {
	http.SetCookie(w, &http.Cookie{Name: "cc_session", Value: v.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: requestIsHTTPS(r), Expires: v.ExpiresAt})
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("cc_session")
		if err != nil {
			s.fail(w, r, "session", fault.E("authentication", "session", "SESSION_REQUIRED", "请先登录", 3))
			return
		}
		actor, user, err := s.service.Identity.SessionActor(r.Context(), cookie.Value)
		if err != nil {
			s.fail(w, r, "session", err)
			return
		}
		ctx := context.WithValue(r.Context(), actorKey{}, struct {
			Actor application.Actor
			User  identitydomain.User
		}{actor, user})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func auth(r *http.Request) (application.Actor, identitydomain.User) {
	value, _ := r.Context().Value(actorKey{}).(struct {
		Actor application.Actor
		User  identitydomain.User
	})
	return value.Actor, value.User
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	actor, user := auth(r)
	tenant, err := s.service.Identity.Tenant(r.Context(), actor)
	if err != nil {
		s.fail(w, r, "session.show", err)
		return
	}
	s.ok(w, r, "session.show", map[string]any{"user": user, "tenant": tenant, "role": actor.Role, "is_platform_admin": actor.PlatformAdmin})
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Operations.Dashboard(r.Context(), actor)
	if err != nil {
		s.fail(w, r, "dashboard.show", err)
		return
	}
	s.ok(w, r, "dashboard.show", v)
}
func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Workspace.Projects(r.Context(), actor)
	if err != nil {
		s.fail(w, r, "project.list", err)
		return
	}
	s.ok(w, r, "project.list", v)
}
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in application.CreateProjectInput
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.Workspace.CreateProject(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "project.create", err)
		return
	}
	s.ok(w, r, "project.create", v)
}
func (s *Server) project(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Workspace.Project(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "project.show", err)
		return
	}
	s.ok(w, r, "project.show", v)
}
func (s *Server) createConnect(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Workspace.CreateConnectSession(r.Context(), actor, chi.URLParam(r, "projectID"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "device.connect_session.create", err)
		return
	}
	s.ok(w, r, "device.connect_session.create", v)
}
func (s *Server) connectStatus(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Workspace.ConnectSession(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "device.connect_session.show", err)
		return
	}
	s.ok(w, r, "device.connect_session.show", v)
}
func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Workspace.Devices(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "device.list", err)
		return
	}
	s.ok(w, r, "device.list", v)
}

func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Runtime.Runs(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "run.list", err)
		return
	}
	s.ok(w, r, "run.list", v)
}
func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Runtime.Run(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "run.show", err)
		return
	}
	s.ok(w, r, "run.show", v)
}
func (s *Server) runProgress(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	v, err := s.service.Runtime.RunProgress(r.Context(), actor, chi.URLParam(r, "id"), after)
	if err != nil {
		s.fail(w, r, "run.progress", err)
		return
	}
	s.ok(w, r, "run.progress", v)
}

func (s *Server) runProgressStream(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	runID := chi.URLParam(r, "id")
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if queryAfter, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64); err == nil && queryAfter > after {
		after = queryAfter
	}
	if _, err := s.service.Runtime.Run(r.Context(), actor, runID); err != nil {
		s.fail(w, r, "run.progress.stream", err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, r, "run.progress.stream", fault.E("internal", "stream", "STREAM_UNSUPPORTED", "服务端不支持进度流", 1))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = io.WriteString(w, "retry: 1000\n\n")
	flusher.Flush()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		events, err := s.service.Runtime.RunProgress(r.Context(), actor, runID, after)
		if err != nil {
			return
		}
		for _, event := range events {
			body, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "id: %d\nevent: progress\ndata: %s\n\n", event.Cursor, body)
			after = event.Cursor
		}
		if len(events) > 0 {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			_, _ = io.WriteString(w, "event: reconnect\ndata: {}\n\n")
			flusher.Flush()
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Operations.Audit(r.Context(), actor, chi.URLParam(r, "projectID"), 50)
	if err != nil {
		s.fail(w, r, "audit.list", err)
		return
	}
	s.ok(w, r, "audit.list", v)
}

type dispatchRequest struct {
	Command string          `json:"command"`
	Params  json.RawMessage `json:"params"`
}

func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	var req dispatchRequest
	if !s.decodeLimit(w, r, &req, 140<<20) {
		return
	}
	switch req.Command {
	case "bootstrap.authorization.start":
		var in application.StartBootstrapAuthorizationInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "初始化授权参数错误"))
			return
		}
		v, err := s.service.Workspace.StartBootstrapAuthorization(r.Context(), requestBaseURL(r), in)
		s.dispatchResult(w, r, req.Command, v, err)
	case "bootstrap.authorization.complete":
		var in application.CompleteBootstrapAuthorizationInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "初始化授权完成参数错误"))
			return
		}
		v, err := s.service.Workspace.CompleteBootstrapAuthorization(r.Context(), in)
		s.dispatchResult(w, r, req.Command, v, err)
	case "bootstrap.progress.append":
		var in struct {
			AttemptToken string                                 `json:"attempt_token"`
			Event        workspacedomain.BootstrapProgressEvent `json:"event"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "初始化进度参数错误"))
			return
		}
		v, err := s.service.Workspace.AppendBootstrapProgress(r.Context(), in.AttemptToken, in.Event)
		s.dispatchResult(w, r, req.Command, v, err)
	case "bootstrap.attempt.complete":
		var in struct {
			AttemptToken string `json:"attempt_token"`
			State        string `json:"state"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "初始化完成参数错误"))
			return
		}
		v, err := s.service.Workspace.CompleteBootstrapAttempt(r.Context(), in.AttemptToken, in.State)
		s.dispatchResult(w, r, req.Command, v, err)
	case "auth.login.start":
		v, err := s.service.Identity.StartUserDeviceLogin(r.Context(), requestBaseURL(r))
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, v)
	case "auth.login.complete":
		var in struct {
			DeviceCode string `json:"device_code"`
		}
		if err := json.Unmarshal(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "登录参数错误"))
			return
		}
		v, err := s.service.Identity.CompleteUserDeviceLogin(r.Context(), in.DeviceCode)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, v)
	case "workspace.register":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			TemplateID      string   `json:"template_id"`
			TemplateVersion string   `json:"template_version"`
			Targets         []string `json:"targets"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "工作区注册参数错误"))
			return
		}
		value, err := s.service.Review.RegisterWorkspace(r.Context(), actor, binding, in.TemplateID, in.TemplateVersion, in.Targets, middleware.GetReqID(r.Context()))
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.upload.start":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.StartWorkspaceUploadInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop 上传会话参数错误"))
			return
		}
		value, err := s.service.Workspace.StartWorkspaceUpload(r.Context(), actor, in)
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.upload.part":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.UploadWorkspacePartInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop 上传分片参数错误"))
			return
		}
		value, err := s.service.Workspace.UploadWorkspacePart(r.Context(), actor, in)
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.upload.finalize":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.FinalizeWorkspaceUploadInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop 上传完成参数错误"))
			return
		}
		value, err := s.service.Workspace.FinalizeWorkspaceUpload(r.Context(), actor, in)
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.workspace.publish":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.PublishWorkspaceRevisionInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop 工作区 Revision 参数错误"))
			return
		}
		value, err := s.service.Workspace.PublishWorkspaceRevision(r.Context(), actor, in)
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.workspace.latest":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			WorkspaceID string `json:"workspace_id"`
			ProjectID   string `json:"project_id"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop Cloud Revision 查询参数错误"))
			return
		}
		value, err := s.service.Workspace.LatestWorkspaceRevision(r.Context(), actor, in.WorkspaceID, in.ProjectID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.workspace.events":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			WorkspaceID string `json:"workspace_id"`
			ProjectID   string `json:"project_id"`
			After       int64  `json:"after"`
			Limit       int    `json:"limit"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop Cloud Revision 事件参数错误"))
			return
		}
		value, err := s.service.Workspace.WorkspaceRevisionEvents(r.Context(), actor, in.WorkspaceID, in.ProjectID, in.After, in.Limit)
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.review.inbox":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop 审批收件箱参数错误"))
			return
		}
		value, err := s.service.Review.DesktopReviewInbox(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.project.projection":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop 项目投影参数错误"))
			return
		}
		value, err := s.service.Review.DesktopProjectProjection(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.review.show":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			ProjectID  string `json:"project_id"`
			RevisionID string `json:"revision_id"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop 审批 Revision 参数错误"))
			return
		}
		value, err := s.service.Review.DesktopReviewRevision(r.Context(), actor, in.ProjectID, in.RevisionID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.review.comment":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.DesktopReviewCommentInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop 审批批注参数错误"))
			return
		}
		value, err := s.service.Review.AddDesktopReviewComment(r.Context(), actor, in, middleware.GetReqID(r.Context()))
		s.dispatchResult(w, r, req.Command, value, err)
	case "desktop.review.approve", "desktop.review.reject", "desktop.review.request_changes":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.DesktopReviewDecisionInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Desktop 审批决策参数错误"))
			return
		}
		switch req.Command {
		case "desktop.review.approve":
			value, callErr := s.service.Review.DesktopApprove(r.Context(), actor, in, middleware.GetReqID(r.Context()))
			s.dispatchResult(w, r, req.Command, value, callErr)
		case "desktop.review.reject":
			value, callErr := s.service.Review.DesktopReject(r.Context(), actor, in, middleware.GetReqID(r.Context()))
			s.dispatchResult(w, r, req.Command, value, callErr)
		default:
			value, callErr := s.service.Review.DesktopRequestChanges(r.Context(), actor, in, middleware.GetReqID(r.Context()))
			s.dispatchResult(w, r, req.Command, value, callErr)
		}
	case "bootstrap.diagnostic.upload":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var summary workspacedomain.BootstrapDiagnosticSummary
		if err := strictDecodeParams(req.Params, &summary); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "诊断摘要参数错误"))
			return
		}
		value, err := s.service.Workspace.UploadBootstrapDiagnostic(r.Context(), actor, binding, summary)
		s.dispatchResult(w, r, req.Command, value, err)
	case "environment.manifest.get":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value, err := s.service.Catalog.EnvironmentManifest(r.Context(), actor, binding)
		s.dispatchResult(w, r, req.Command, value, err)
	case "environment.registry.get":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value, err := s.service.Catalog.EnvironmentRegistry(r.Context(), actor, binding)
		s.dispatchResult(w, r, req.Command, value, err)
	case "conversation-import.show":
		actor, _, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			ID string `json:"id"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "对话导入查询参数错误"))
			return
		}
		value, err := s.service.Work.ConversationImport(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "conversation-import.bundle":
		actor, _, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			ImportID string                  `json:"import_id"`
			Bundle   work.ConversationBundle `json:"bundle"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "对话摘要包参数错误"))
			return
		}
		if in.ImportID == "" {
			in.ImportID = in.Bundle.ImportID
		}
		value, err := s.service.Work.SubmitConversationBundle(r.Context(), actor, in.ImportID, in.Bundle, middleware.GetReqID(r.Context()))
		s.dispatchResult(w, r, req.Command, value, err)
	case "submission.create":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var bundle reviewdomain.SubmissionBundle
		if err := strictDecodeParams(req.Params, &bundle); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "提交数据包参数错误"))
			return
		}
		value, err := s.service.Review.CreateSubmission(r.Context(), actor, binding, bundle, middleware.GetReqID(r.Context()))
		s.dispatchResult(w, r, req.Command, value, err)
	case "submission.workspace-list":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value, err := s.service.Review.WorkspaceSubmissions(r.Context(), actor, binding)
		s.dispatchResult(w, r, req.Command, value, err)
	case "submission.workspace-show":
		actor, _, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			ID string `json:"id"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "提交记录查询参数错误"))
			return
		}
		value, err := s.service.Review.SubmissionDetails(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "snapshot.workspace-list":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			SubmissionType string `json:"submission_type"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Snapshot 查询参数错误"))
			return
		}
		value, err := s.service.Review.ApprovedSnapshots(r.Context(), actor, binding.ProjectID, in.SubmissionType)
		s.dispatchResult(w, r, req.Command, value, err)
	case "snapshot.workspace-show":
		actor, _, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			ID string `json:"id"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Snapshot 查询参数错误"))
			return
		}
		value, err := s.service.Review.ApprovedSnapshot(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "feedback.workspace-list":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value, err := s.service.Review.WorkspaceFeedback(r.Context(), actor, binding)
		s.dispatchResult(w, r, req.Command, value, err)
	case "decision.workspace-list":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value, err := s.service.Review.WorkspaceDecisions(r.Context(), actor, binding)
		s.dispatchResult(w, r, req.Command, value, err)
	case "runtime.worker.prepare":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.RuntimeWorkerPrepareInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Runtime worker 准备参数错误"))
			return
		}
		value, err := s.service.Runtime.PrepareRuntimeWorker(r.Context(), actor, in)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value.GatewayURL = "/api/v1/runtime/mcp/call"
		s.ok(w, r, req.Command, value)
	case "runtime.worker.prepare_next":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.RuntimeWorkerPrepareNextInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Runtime worker 领取参数错误"))
			return
		}
		value, err := s.service.Runtime.PrepareNextRuntimeWorker(r.Context(), actor, in)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value.GatewayURL = "/api/v1/runtime/mcp/call"
		s.ok(w, r, req.Command, value)
	case "runtime.worker.activate":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.RuntimeWorkerActivateInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Runtime worker 激活参数错误"))
			return
		}
		value, err := s.service.Runtime.ActivateRuntimeWorker(r.Context(), actor, in)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, value)
	case "runtime.worker.heartbeat":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.RuntimeWorkerHeartbeatInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Runtime worker 心跳参数错误"))
			return
		}
		value, err := s.service.Runtime.HeartbeatRuntimeWorker(r.Context(), actor, in)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, value)
	case "runtime.worker.mcp":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.RuntimeMCPCallInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Runtime MCP 调用参数错误"))
			return
		}
		value, err := s.service.Runtime.CallRuntimeMCP(r.Context(), actor, in)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, value)
	case "runtime.worker.event":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.RuntimeWorkerEventInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Runtime worker 事件参数错误"))
			return
		}
		if err := s.service.Runtime.RecordRuntimeWorkerEvent(r.Context(), actor, in); err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, map[string]any{"recorded": true})
	case "runtime.worker.finalize":
		actor, _, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in application.RuntimeWorkerFinalizeInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "Runtime worker 终态参数错误"))
			return
		}
		value, err := s.service.Runtime.FinalizeRuntimeWorker(r.Context(), actor, in, middleware.GetReqID(r.Context()))
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, value)
	default:
		if s.handleUserDispatch(w, r, req) {
			return
		}
		s.fail(w, r, "cli.dispatch", fault.Invalid("COMMAND_UNKNOWN", "未知或未开放的 CLI 命令"))
	}
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return scheme + "://" + host
}
func (s *Server) deviceFromRequest(r *http.Request) (application.Actor, workspacedomain.Device, error) {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	return s.service.Workspace.DeviceActor(r.Context(), token)
}

func (s *Server) workspaceFromRequest(r *http.Request) (application.Actor, workspacedomain.WorkspaceBinding, error) {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	return s.service.Workspace.WorkspaceActor(r.Context(), token)
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, out any) bool {
	return s.decodeLimit(w, r, out, 2<<20)
}

func (s *Server) decodeLimit(w http.ResponseWriter, r *http.Request, out any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil && err != io.EOF {
		s.fail(w, r, "request.decode", fault.Invalid("INPUT_INVALID", fmt.Sprintf("请求格式错误: %v", err)))
		return false
	}
	return true
}
func (s *Server) ok(w http.ResponseWriter, r *http.Request, command string, data any) {
	s.write(w, http.StatusOK, envelope{OK: true, Command: command, RequestID: middleware.GetReqID(r.Context()), Data: data, Meta: map[string]any{}})
}
func (s *Server) fail(w http.ResponseWriter, r *http.Request, command string, err error) {
	de := &fault.Error{}
	if !errors.As(err, &de) {
		de = fault.E("internal", "unexpected", "INTERNAL_ERROR", "服务暂时不可用", 1)
		s.log.Error("request failed", "error", err, "command", command)
	}
	status := http.StatusBadRequest
	if de.Type == "not_found" {
		status = http.StatusNotFound
	} else {
		switch de.ExitCode {
		case 3:
			status = http.StatusUnauthorized
		case 4:
			status = http.StatusForbidden
		case 6:
			status = http.StatusConflict
		case 1:
			status = http.StatusInternalServerError
		}
	}
	s.write(w, status, envelope{OK: false, Command: command, RequestID: middleware.GetReqID(r.Context()), Meta: map[string]any{}, Error: de})
}
func (s *Server) write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.webDist == "" {
		http.NotFound(w, r)
		return
	}
	clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	path := filepath.Join(s.webDist, clean)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	index := filepath.Join(s.webDist, "index.html")
	if _, err := os.Stat(index); err != nil {
		http.Error(w, "Content Work OS 工作台尚未构建，请运行 pnpm --dir apps/web build", http.StatusServiceUnavailable)
		return
	}
	http.ServeFile(w, r, index)
}
