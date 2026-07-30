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
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/fixturev3"
)

type Server struct {
	service *app.Service
	log     *slog.Logger
	devMode bool
	webDist string
}

type envelope struct {
	OK        bool           `json:"ok"`
	Command   string         `json:"command"`
	RequestID string         `json:"request_id"`
	Data      any            `json:"data,omitempty"`
	Meta      map[string]any `json:"meta"`
	Error     *domain.Error  `json:"error,omitempty"`
}

type actorKey struct{}

func New(service *app.Service, logger *slog.Logger, devMode bool, webDist string) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{service: service, log: logger, devMode: devMode, webDist: webDist}
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
		r.Route("/admin", func(r chi.Router) {
			r.Use(s.requireSession)
			r.Get("/dashboard", s.platformOverview)
			r.Patch("/tenants/{tenantID}", s.updatePlatformTenant)
			r.Put("/tenants/{tenantID}/content-capabilities/{contentType}", s.updatePlatformTenantContentCapability)
		})
	})
	r.Route("/api/review/{token}", func(r chi.Router) {
		r.Get("/projection", s.publicReviewProjection)
		r.Post("/verify", s.publicReviewVerify)
		r.Post("/decision", s.publicReviewDecision)
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
		r.Get("/agent-clients", s.agentClients)
		r.Get("/projects", s.projects)
		r.Post("/projects", s.createProject)
		r.Get("/projects/{projectID}", s.project)
		r.Get("/projects/{projectID}/projection", s.projectProjection)
		r.Get("/projects/{projectID}/codex-handoff", s.projectCodexHandoff)
		r.Get("/projects/{projectID}/agent-handoff", s.projectAgentHandoff)
		r.Patch("/projects/{projectID}", s.updateProject)
		r.Post("/projects/{projectID}/archive", s.archiveProject)
		r.Post("/projects/{projectID}/restore", s.restoreProject)
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
		r.Post("/device-auth/approve", s.approveDeviceAuth)
		r.Get("/projects/{projectID}/runs", s.runs)
		r.Get("/runs/{id}", s.run)
		r.Get("/runs/{id}/attempts", s.runAttempts)
		r.Post("/runs/{id}/cancel", s.cancelRun)
		r.Post("/comments/{id}/resolve", s.resolveReviewComment)
		r.Get("/submission-revisions/{id}/review-grants", s.reviewGrants)
		r.Post("/submission-revisions/{id}/review-grants", s.createReviewGrant)
		r.Post("/review-grants/{id}/revoke", s.revokeReviewGrant)
		r.Get("/artifacts/{id}/download", s.downloadArtifact)
		r.Get("/projects/{projectID}/audit", s.audit)
		r.Get("/projects/{projectID}/submissions", s.submissions)
		r.Get("/projects/{projectID}/submission-revisions/{id}", s.projectSubmissionRevision)
		r.Get("/projects/{projectID}/submission-revisions/{id}/codex-handoff", s.reviewFeedbackCodexHandoff)
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
		session domain.Session
		err     error
	)
	if strings.TrimSpace(in.InviteToken) != "" {
		session, err = s.service.RegisterWithInvite(r.Context(), in.Email, in.Password, in.DisplayName, strings.TrimSpace(in.InviteToken))
	} else {
		session, err = s.service.Register(r.Context(), in.Email, in.Password, in.DisplayName, in.TenantName)
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
	session, err := s.service.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		s.fail(w, r, "auth.login", err)
		return
	}
	s.setSession(w, r, session)
	s.ok(w, r, "auth.login", map[string]any{"expires_at": session.ExpiresAt})
}
func (s *Server) devBootstrap(w http.ResponseWriter, r *http.Request) {
	session, err := s.service.Login(r.Context(), "demo@contentcloud.local", "contentcloud-demo-2026")
	if err != nil {
		session, err = s.service.Register(r.Context(), "demo@contentcloud.local", "contentcloud-demo-2026", "林舟", "南京澄观内容科技")
	}
	if err != nil {
		s.fail(w, r, "dev.bootstrap", err)
		return
	}
	s.setSession(w, r, session)
	actor, _, _ := s.service.SessionActor(r.Context(), session.ID)
	data := map[string]any{"ready": true}
	fixture, err := fixturev3.Decode(r.Body)
	if err == nil {
		result, importErr := s.service.ImportFixtureV3(r.Context(), actor, fixture, middleware.GetReqID(r.Context()))
		if importErr != nil {
			s.fail(w, r, "dev.bootstrap", importErr)
			return
		}
		data["fixture"] = result
	} else if !errors.Is(err, io.EOF) {
		s.fail(w, r, "dev.bootstrap", domain.Invalid("FIXTURE_V3_INVALID", err.Error()))
		return
	}
	s.ok(w, r, "dev.bootstrap", data)
}
func (s *Server) setSession(w http.ResponseWriter, r *http.Request, v domain.Session) {
	http.SetCookie(w, &http.Cookie{Name: "cc_session", Value: v.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: requestIsHTTPS(r), Expires: v.ExpiresAt})
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("cc_session")
		if err != nil {
			s.fail(w, r, "session", domain.E("authentication", "session", "SESSION_REQUIRED", "请先登录", 3))
			return
		}
		actor, user, err := s.service.SessionActor(r.Context(), cookie.Value)
		if err != nil {
			s.fail(w, r, "session", err)
			return
		}
		ctx := context.WithValue(r.Context(), actorKey{}, struct {
			Actor app.Actor
			User  domain.User
		}{actor, user})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func auth(r *http.Request) (app.Actor, domain.User) {
	value, _ := r.Context().Value(actorKey{}).(struct {
		Actor app.Actor
		User  domain.User
	})
	return value.Actor, value.User
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	actor, user := auth(r)
	tenant, err := s.service.Tenant(r.Context(), actor)
	if err != nil {
		s.fail(w, r, "session.show", err)
		return
	}
	s.ok(w, r, "session.show", map[string]any{"user": user, "tenant": tenant, "role": actor.Role, "is_platform_admin": actor.PlatformAdmin})
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Dashboard(r.Context(), actor)
	if err != nil {
		s.fail(w, r, "dashboard.show", err)
		return
	}
	s.ok(w, r, "dashboard.show", v)
}
func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Projects(r.Context(), actor)
	if err != nil {
		s.fail(w, r, "project.list", err)
		return
	}
	s.ok(w, r, "project.list", v)
}
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateProjectInput
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.CreateProject(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "project.create", err)
		return
	}
	s.ok(w, r, "project.create", v)
}
func (s *Server) project(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Project(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "project.show", err)
		return
	}
	s.ok(w, r, "project.show", v)
}
func (s *Server) createConnect(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.CreateConnectSession(r.Context(), actor, chi.URLParam(r, "projectID"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "device.connect_session.create", err)
		return
	}
	s.ok(w, r, "device.connect_session.create", v)
}
func (s *Server) connectStatus(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.ConnectSession(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "device.connect_session.show", err)
		return
	}
	s.ok(w, r, "device.connect_session.show", v)
}
func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Devices(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "device.list", err)
		return
	}
	s.ok(w, r, "device.list", v)
}

func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Runs(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "run.list", err)
		return
	}
	s.ok(w, r, "run.list", v)
}
func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Run(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "run.show", err)
		return
	}
	s.ok(w, r, "run.show", v)
}
func (s *Server) runAttempts(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.RunAttempts(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "run.attempts", err)
		return
	}
	s.ok(w, r, "run.attempts", v)
}
func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Audit(r.Context(), actor, chi.URLParam(r, "projectID"), 50)
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
		var in app.StartBootstrapAuthorizationInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "初始化授权参数错误"))
			return
		}
		v, err := s.service.StartBootstrapAuthorization(r.Context(), requestBaseURL(r), in)
		s.dispatchResult(w, r, req.Command, v, err)
	case "bootstrap.authorization.complete":
		var in app.CompleteBootstrapAuthorizationInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "初始化授权完成参数错误"))
			return
		}
		v, err := s.service.CompleteBootstrapAuthorization(r.Context(), in)
		s.dispatchResult(w, r, req.Command, v, err)
	case "bootstrap.progress.append":
		var in struct {
			AttemptToken string                        `json:"attempt_token"`
			Event        domain.BootstrapProgressEvent `json:"event"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "初始化进度参数错误"))
			return
		}
		v, err := s.service.AppendBootstrapProgress(r.Context(), in.AttemptToken, in.Event)
		s.dispatchResult(w, r, req.Command, v, err)
	case "bootstrap.attempt.complete":
		var in struct {
			AttemptToken string `json:"attempt_token"`
			State        string `json:"state"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "初始化完成参数错误"))
			return
		}
		v, err := s.service.CompleteBootstrapAttempt(r.Context(), in.AttemptToken, in.State)
		s.dispatchResult(w, r, req.Command, v, err)
	case "auth.login.start":
		v, err := s.service.StartUserDeviceLogin(r.Context(), requestBaseURL(r))
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
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "登录参数错误"))
			return
		}
		v, err := s.service.CompleteUserDeviceLogin(r.Context(), in.DeviceCode)
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
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "工作区注册参数错误"))
			return
		}
		value, err := s.service.RegisterWorkspace(r.Context(), actor, binding, in.TemplateID, in.TemplateVersion, in.Targets, middleware.GetReqID(r.Context()))
		s.dispatchResult(w, r, req.Command, value, err)
	case "bootstrap.diagnostic.upload":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var summary domain.BootstrapDiagnosticSummary
		if err := strictDecodeParams(req.Params, &summary); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "诊断摘要参数错误"))
			return
		}
		value, err := s.service.UploadBootstrapDiagnostic(r.Context(), actor, binding, summary)
		s.dispatchResult(w, r, req.Command, value, err)
	case "environment.manifest.get":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value, err := s.service.EnvironmentManifest(r.Context(), actor, binding)
		s.dispatchResult(w, r, req.Command, value, err)
	case "environment.registry.get":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value, err := s.service.EnvironmentRegistry(r.Context(), actor, binding)
		s.dispatchResult(w, r, req.Command, value, err)
	case "submission.create":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var bundle domain.SubmissionBundle
		if err := strictDecodeParams(req.Params, &bundle); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "SubmissionBundle 参数错误"))
			return
		}
		value, err := s.service.CreateSubmission(r.Context(), actor, binding, bundle, middleware.GetReqID(r.Context()))
		s.dispatchResult(w, r, req.Command, value, err)
	case "submission.workspace-list":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value, err := s.service.WorkspaceSubmissions(r.Context(), actor, binding)
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
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "Submission 查询参数错误"))
			return
		}
		value, err := s.service.SubmissionDetails(r.Context(), actor, in.ID)
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
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "Snapshot 查询参数错误"))
			return
		}
		value, err := s.service.ApprovedSnapshots(r.Context(), actor, binding.ProjectID, in.SubmissionType)
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
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "Snapshot 查询参数错误"))
			return
		}
		value, err := s.service.ApprovedSnapshot(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "feedback.workspace-list":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value, err := s.service.WorkspaceFeedback(r.Context(), actor, binding)
		s.dispatchResult(w, r, req.Command, value, err)
	case "decision.workspace-list":
		actor, binding, err := s.workspaceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		value, err := s.service.WorkspaceDecisions(r.Context(), actor, binding)
		s.dispatchResult(w, r, req.Command, value, err)
	case "daemon.poll":
		actor, device, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			Capabilities []domain.Capability              `json:"capabilities"`
			Environments []app.AutomationEnvironmentClaim `json:"environments"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "轮询参数错误"))
			return
		}
		lease, err := s.service.PollWithEnvironment(r.Context(), actor, device, in.Capabilities, in.Environments)
		if err != nil {
			var de *domain.Error
			if errors.As(err, &de) && de.Code == "RESOURCE_NOT_FOUND" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, lease)
	case "run.report":
		actor, device, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			RunID     string          `json:"run_id"`
			AttemptID string          `json:"attempt_id"`
			RunToken  string          `json:"run_token"`
			Package   json.RawMessage `json:"package"`
		}
		if err := json.Unmarshal(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "报告参数错误"))
			return
		}
		v, err := s.service.ReportTask(r.Context(), actor, device, in.RunID, in.AttemptID, in.RunToken, in.Package, middleware.GetReqID(r.Context()))
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, v)
	case "run.heartbeat":
		actor, device, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			RunID     string              `json:"run_id"`
			AttemptID string              `json:"attempt_id"`
			RunToken  string              `json:"run_token"`
			Heartbeat domain.RunHeartbeat `json:"heartbeat"`
		}
		if err := json.Unmarshal(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "心跳参数错误"))
			return
		}
		v, err := s.service.HeartbeatRun(r.Context(), actor, device, in.RunID, in.AttemptID, in.RunToken, in.Heartbeat, middleware.GetReqID(r.Context()))
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, map[string]any{"run": v, "cancel_requested": v.CancelRequestedAt != nil})
	case "run.finish":
		actor, device, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			RunID     string `json:"run_id"`
			AttemptID string `json:"attempt_id"`
			RunToken  string `json:"run_token"`
			app.FinishRunAttemptInput
		}
		if err := json.Unmarshal(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "Attempt 完成参数错误"))
			return
		}
		v, err := s.service.FinishRunAttempt(r.Context(), actor, device, in.RunID, in.AttemptID, in.RunToken, in.FinishRunAttemptInput, middleware.GetReqID(r.Context()))
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, v)
	default:
		if s.handleUserDispatch(w, r, req) {
			return
		}
		s.fail(w, r, "cli.dispatch", domain.Invalid("COMMAND_UNKNOWN", "未知或未开放的 CLI 命令"))
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
func (s *Server) deviceFromRequest(r *http.Request) (app.Actor, domain.Device, error) {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	return s.service.DeviceActor(r.Context(), token)
}

func (s *Server) workspaceFromRequest(r *http.Request) (app.Actor, domain.WorkspaceBinding, error) {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	return s.service.WorkspaceActor(r.Context(), token)
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, out any) bool {
	return s.decodeLimit(w, r, out, 2<<20)
}

func (s *Server) decodeLimit(w http.ResponseWriter, r *http.Request, out any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil && err != io.EOF {
		s.fail(w, r, "request.decode", domain.Invalid("INPUT_INVALID", fmt.Sprintf("请求格式错误: %v", err)))
		return false
	}
	return true
}
func (s *Server) ok(w http.ResponseWriter, r *http.Request, command string, data any) {
	s.write(w, http.StatusOK, envelope{OK: true, Command: command, RequestID: middleware.GetReqID(r.Context()), Data: data, Meta: map[string]any{}})
}
func (s *Server) fail(w http.ResponseWriter, r *http.Request, command string, err error) {
	de := &domain.Error{}
	if !errors.As(err, &de) {
		de = domain.E("internal", "unexpected", "INTERNAL_ERROR", "服务暂时不可用", 1)
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
		http.Error(w, "ContentCloud Web 尚未构建，请运行 pnpm --dir web build", http.StatusServiceUnavailable)
		return
	}
	http.ServeFile(w, r, index)
}
