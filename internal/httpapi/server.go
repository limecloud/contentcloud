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
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", s.register)
		r.Post("/auth/login", s.login)
		if s.devMode {
			r.Post("/dev/bootstrap", s.devBootstrap)
		}
		r.Post("/cli/dispatch", s.dispatch)
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
		r.Get("/projects", s.projects)
		r.Post("/projects", s.createProject)
		r.Get("/projects/{projectID}", s.project)
		r.Patch("/projects/{projectID}", s.updateProject)
		r.Post("/projects/{projectID}/archive", s.archiveProject)
		r.Post("/projects/{projectID}/restore", s.restoreProject)
		r.Get("/project-templates", s.projectTemplates)
		r.Post("/project-templates", s.createProjectTemplate)
		r.Post("/projects/{projectID}/connect-sessions", s.createConnect)
		r.Get("/connect-sessions/{id}", s.connectStatus)
		r.Post("/connect-sessions/{id}/cancel", s.cancelConnectSession)
		r.Get("/projects/{projectID}/devices", s.devices)
		r.Get("/devices/{id}", s.device)
		r.Post("/projects/{projectID}/devices/{id}/attach", s.attachDevice)
		r.Post("/projects/{projectID}/devices/{id}/detach", s.detachDevice)
		r.Post("/devices/{id}/revoke", s.revokeDevice)
		r.Post("/device-auth/approve", s.approveDeviceAuth)
		r.Get("/projects/{projectID}/sources", s.sources)
		r.Post("/projects/{projectID}/sources/upload", s.uploadSource)
		r.Get("/sources/{sourceID}/revisions", s.sourceRevisions)
		r.Post("/sources/{sourceID}/revisions/upload", s.uploadSourceRevision)
		r.Get("/sources/{sourceID}/impact", s.sourceImpact)
		r.Get("/source-revisions/{id}/evidence", s.evidence)
		r.Get("/source-revisions/{id}", s.sourceRevision)
		r.Post("/evidence/{id}/review", s.reviewEvidence)
		r.Get("/projects/{projectID}/assets", s.assets)
		r.Post("/projects/{projectID}/assets", s.createAsset)
		r.Get("/assets/{id}/rights", s.rightsRecords)
		r.Post("/assets/{id}/rights", s.createRightsRecord)
		r.Post("/rights/{id}/review", s.reviewRightsRecord)
		r.Get("/projects/{projectID}/knowledge", s.knowledge)
		r.Post("/projects/{projectID}/knowledge", s.createKnowledge)
		r.Post("/projects/{projectID}/knowledge-extraction-runs", s.createKnowledgeExtractionRun)
		r.Post("/knowledge/{id}/review", s.reviewKnowledge)
		r.Get("/projects/{projectID}/knowledge-conflicts", s.knowledgeConflicts)
		r.Get("/projects/{projectID}/decision-requests", s.decisionRequests)
		r.Post("/decision-requests/{id}/resolve", s.resolveDecisionRequest)
		r.Get("/projects/{projectID}/benchmarks", s.benchmarks)
		r.Post("/projects/{projectID}/benchmarks", s.createBenchmark)
		r.Get("/projects/{projectID}/frameworks", s.frameworks)
		r.Post("/benchmarks/{id}/frameworks", s.createFramework)
		r.Get("/projects/{projectID}/shot-patterns", s.shotPatterns)
		r.Post("/frameworks/{id}/shot-patterns", s.createShotPattern)
		r.Get("/projects/{projectID}/selling-points", s.sellingPoints)
		r.Post("/projects/{projectID}/selling-points", s.createSellingPoint)
		r.Get("/projects/{projectID}/visualization-plans", s.visualizationPlans)
		r.Post("/selling-points/{id}/visualization-plans", s.createVisualizationPlan)
		r.Post("/visualization-plans/{id}/review", s.reviewVisualizationPlan)
		r.Get("/projects/{projectID}/briefs", s.briefs)
		r.Post("/projects/{projectID}/briefs", s.createBrief)
		r.Post("/briefs/{id}/versions", s.reviseBrief)
		r.Post("/briefs/{id}/review", s.reviewBrief)
		r.Post("/briefs/{id}/runs", s.createRun)
		r.Get("/projects/{projectID}/runs", s.runs)
		r.Get("/runs/{id}", s.run)
		r.Get("/runs/{id}/attempts", s.runAttempts)
		r.Post("/runs/{id}/cancel", s.cancelRun)
		r.Get("/projects/{projectID}/scripts", s.scripts)
		r.Get("/scripts/{id}", s.script)
		r.Post("/scripts/{id}/runs", s.createScriptChangeRun)
		r.Post("/scripts/{id}/review", s.reviewScript)
		r.Get("/scripts/{id}/comments", s.reviewComments)
		r.Post("/scripts/{id}/comments", s.createReviewComment)
		r.Get("/scripts/{id}/review-cycles", s.reviewCycles)
		r.Post("/comments/{id}/resolve", s.resolveReviewComment)
		r.Get("/scripts/{id}/review-grants", s.reviewGrants)
		r.Post("/scripts/{id}/review-grants", s.createReviewGrant)
		r.Post("/review-grants/{id}/revoke", s.revokeReviewGrant)
		r.Get("/scripts/{id}/artifacts", s.artifacts)
		r.Post("/scripts/{id}/exports", s.exportScript)
		r.Get("/artifacts/{id}/presentation", s.artifactPresentation)
		r.Post("/artifacts/{id}/local-open", s.createArtifactOpenRequest)
		r.Get("/artifact-open-requests/{id}", s.artifactOpenRequest)
		r.Get("/artifacts/{id}/download", s.downloadArtifact)
		r.Get("/projects/{projectID}/results", s.performanceObservations)
		r.Post("/projects/{projectID}/results", s.createPerformanceObservation)
		r.Get("/projects/{projectID}/performance-imports", s.performanceImportBatches)
		r.Post("/projects/{projectID}/performance-imports", s.createPerformanceImport)
		r.Get("/performance-imports/{id}", s.performanceImportDetails)
		r.Get("/projects/{projectID}/rating-decisions", s.ratingDecisions)
		r.Post("/projects/{projectID}/rating-decisions", s.createRatingDecision)
		r.Get("/projects/{projectID}/lineage", s.projectLineage)
		r.Get("/projects/{projectID}/impact", s.projectImpact)
		r.Get("/projects/{projectID}/audit", s.audit)
		r.Get("/projects/{projectID}/submissions", s.submissions)
		r.Get("/submissions/{id}", s.submissionDetails)
		r.Post("/submission-revisions/{id}/approve", s.approveSubmission)
		r.Post("/submission-revisions/{id}/request-changes", s.requestSubmissionChanges)
		r.Get("/projects/{projectID}/approved-snapshots", s.approvedSnapshots)
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
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in authInput
	if !s.decode(w, r, &in) {
		return
	}
	session, err := s.service.Register(r.Context(), in.Email, in.Password, in.DisplayName, in.TenantName)
	if err != nil {
		s.fail(w, r, "auth.register", err)
		return
	}
	s.setSession(w, session)
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
	s.setSession(w, session)
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
	s.setSession(w, session)
	actor, _, _ := s.service.SessionActor(r.Context(), session.ID)
	projects, _ := s.service.Projects(r.Context(), actor)
	if len(projects) == 0 {
		p, createErr := s.service.CreateProject(r.Context(), actor, app.CreateProjectInput{BrandName: "金陵古都香", ProductName: "金陵古法线香", Channel: "douyin", StageObjective: "验证传统香文化的年轻化内容表达", OwnerName: "陈汐", ReviewerName: "周岚", ClientApprover: "金陵古香品牌部"}, middleware.GetReqID(r.Context()))
		if createErr != nil {
			s.fail(w, r, "dev.bootstrap", createErr)
			return
		}
		if seedErr := s.seedDemo(r.Context(), actor, p.ID); seedErr != nil {
			s.fail(w, r, "dev.bootstrap", seedErr)
			return
		}
	}
	s.ok(w, r, "dev.bootstrap", map[string]any{"ready": true})
}
func (s *Server) setSession(w http.ResponseWriter, v domain.Session) {
	http.SetCookie(w, &http.Cookie{Name: "cc_session", Value: v.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: false, Expires: v.ExpiresAt})
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
	s.ok(w, r, "session.show", map[string]any{"user": user, "tenant": tenant, "role": actor.Role})
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

func (s *Server) knowledge(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Knowledge(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "knowledge.list", err)
		return
	}
	s.ok(w, r, "knowledge.list", v)
}
func (s *Server) createKnowledge(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateKnowledgeInput
	if !s.decode(w, r, &in) {
		return
	}
	in.ProjectID = chi.URLParam(r, "projectID")
	v, err := s.service.CreateKnowledge(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "knowledge.create", err)
		return
	}
	s.ok(w, r, "knowledge.create", v)
}
func (s *Server) createKnowledgeExtractionRun(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateKnowledgeExtractionRunInput
	if !s.decode(w, r, &in) {
		return
	}
	in.ProjectID = chi.URLParam(r, "projectID")
	v, err := s.service.CreateKnowledgeExtractionRun(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "knowledge.extract", err)
		return
	}
	s.ok(w, r, "knowledge.extract", v)
}
func (s *Server) reviewKnowledge(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		Decision string `json:"decision"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.ReviewKnowledge(r.Context(), actor, chi.URLParam(r, "id"), in.Decision, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "knowledge.review", err)
		return
	}
	s.ok(w, r, "knowledge.review", v)
}

func (s *Server) briefs(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Briefs(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "brief.list", err)
		return
	}
	s.ok(w, r, "brief.list", v)
}
func (s *Server) createBrief(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateBriefInput
	if !s.decode(w, r, &in) {
		return
	}
	in.ProjectID = chi.URLParam(r, "projectID")
	v, err := s.service.CreateBrief(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "brief.create", err)
		return
	}
	s.ok(w, r, "brief.create", v)
}
func (s *Server) reviseBrief(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	previous, err := s.service.Brief(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "brief.revise", err)
		return
	}
	var in app.CreateBriefInput
	if !s.decode(w, r, &in) {
		return
	}
	in.ProjectID = previous.ProjectID
	in.SupersedesID = previous.ID
	v, err := s.service.CreateBrief(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "brief.revise", err)
		return
	}
	s.ok(w, r, "brief.revise", v)
}
func (s *Server) reviewBrief(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.ReviewBriefWithReason(r.Context(), actor, chi.URLParam(r, "id"), in.Decision, in.Reason, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "brief.review", err)
		return
	}
	s.ok(w, r, "brief.review", v)
}
func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if r.ContentLength > 0 && !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.CreateScriptRun(r.Context(), actor, chi.URLParam(r, "id"), in.IdempotencyKey, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "run.create", err)
		return
	}
	s.ok(w, r, "run.create", v)
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
func (s *Server) scripts(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Scripts(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "script.list", err)
		return
	}
	s.ok(w, r, "script.list", v)
}
func (s *Server) script(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Script(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "script.show", err)
		return
	}
	s.ok(w, r, "script.show", v)
}
func (s *Server) createScriptChangeRun(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateScriptChangeRunInput
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.CreateScriptChangeRun(r.Context(), actor, chi.URLParam(r, "id"), in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "script.change.create", err)
		return
	}
	s.ok(w, r, "script.change.create", v)
}
func (s *Server) reviewScript(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		app.ReviewScriptInput
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	if in.Conclusion == "" {
		in.Conclusion = in.Reason
	}
	v, err := s.service.ReviewScriptWithInput(r.Context(), actor, chi.URLParam(r, "id"), in.ReviewScriptInput, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "script.review", err)
		return
	}
	s.ok(w, r, "script.review", v)
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
	case "device.connect":
		var in app.ConnectDeviceInput
		if err := json.Unmarshal(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "设备连接参数错误"))
			return
		}
		v, err := s.service.ConnectDevice(r.Context(), in)
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
	case "artifact.register":
		actor, device, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in app.RegisterArtifactInput
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "Artifact Envelope 参数错误"))
			return
		}
		value, err := s.service.RegisterArtifact(r.Context(), actor, device, in, middleware.GetReqID(r.Context()))
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, value)
	case "artifact.open.poll":
		actor, device, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			Capabilities []domain.Capability `json:"capabilities"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "Artifact 打开轮询参数错误"))
			return
		}
		value, err := s.service.PollArtifactOpen(r.Context(), actor, device, in.Capabilities)
		if err != nil {
			var domainError *domain.Error
			if errors.As(err, &domainError) && domainError.Code == "RESOURCE_NOT_FOUND" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, value)
	case "artifact.open.finish":
		actor, device, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			OpenRequestID string `json:"open_request_id"`
			State         string `json:"state"`
			Reason        string `json:"reason"`
		}
		if err := strictDecodeParams(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "Artifact 打开结果参数错误"))
			return
		}
		value, err := s.service.FinishArtifactOpen(r.Context(), actor, device, in.OpenRequestID, in.State, in.Reason)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		s.ok(w, r, req.Command, value)
	case "daemon.poll":
		actor, device, err := s.deviceFromRequest(r)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return
		}
		var in struct {
			Capabilities []domain.Capability `json:"capabilities"`
		}
		if err := json.Unmarshal(req.Params, &in); err != nil {
			s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "轮询参数错误"))
			return
		}
		lease, err := s.service.Poll(r.Context(), actor, device, in.Capabilities)
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

func (s *Server) seedDemo(ctx context.Context, actor app.Actor, projectID string) error {
	type demoKnowledge struct {
		name, sourceType, fileName, quote, locatorKind, locator string
		input                                                   app.CreateKnowledgeInput
	}
	items := []demoKnowledge{
		{name: "产品使用说明", sourceType: "product_spec", fileName: "product-manual.txt", quote: "本品为线香，建议在通风良好的室内空间使用。", locatorKind: "paragraph", locator: `{"paragraph":1}`, input: app.CreateKnowledgeInput{ProjectID: projectID, Kind: "fact", Title: "产品形制", Statement: "线香为细长条状香品，适合书房、茶席等室内场景。", RiskLevel: "low", AllowedChannels: []string{"douyin"}}},
		{name: "品牌传播边界", sourceType: "brand_manual", fileName: "brand-guide.txt", quote: "对外传播不得使用医疗、保健或改善疾病相关表述。", locatorKind: "paragraph", locator: `{"paragraph":1}`, input: app.CreateKnowledgeInput{ProjectID: projectID, Kind: "claim", Title: "品牌表达边界", Statement: "内容可表达传统制香工艺与日常仪式感，不宣称治疗或保健功效。", RiskLevel: "high", AllowedChannels: []string{"douyin"}}},
		{name: "视觉资产规范", sourceType: "visual_asset", fileName: "visual-guide.txt", quote: "商标和包装文字须使用品牌提供的原始视觉素材。", locatorKind: "paragraph", locator: `{"paragraph":1}`, input: app.CreateKnowledgeInput{ProjectID: projectID, Kind: "visual_rule", Title: "产品视觉真实性", Statement: "包装、Logo 和可读文字必须使用真实素材合成，不依赖生成模型重绘。", RiskLevel: "high", AllowedChannels: []string{"douyin"}}},
	}
	ids := make([]string, 0, len(items))
	worker := actor
	worker.Type = "worker"
	for _, item := range items {
		revision, err := s.service.UploadSource(ctx, actor, projectID, item.name, item.sourceType, item.fileName, "text/plain", []byte(item.quote), "")
		if err != nil {
			return err
		}
		_, err = s.service.CompleteSource(ctx, worker, revision.ID, app.CompleteSourceInput{DetectedMIME: "text/plain", Status: "ready", ParserVersion: "demo-seed/v1", Evidence: []app.CreateEvidenceInput{{LocatorKind: item.locatorKind, Locator: map[string]any{"paragraph": 1}, QuoteText: item.quote}}}, "")
		if err != nil {
			return err
		}
		item.input.Evidence = []domain.EvidenceRef{{SourceRevisionID: revision.ID, LocatorKind: item.locatorKind, Locator: item.locator, Quote: item.quote}}
		knowledge, err := s.service.CreateKnowledge(ctx, actor, item.input, "")
		if err != nil {
			return err
		}
		knowledge, err = s.service.ReviewKnowledge(ctx, actor, knowledge.ID, "approve", "")
		if err != nil {
			return err
		}
		ids = append(ids, knowledge.ID)
	}
	benchmark, err := s.service.CreateBenchmark(ctx, actor, app.CreateBenchmarkInput{ProjectID: projectID, Title: "安静书房场景内容拆解", Platform: "douyin", OriginalURL: "https://example.com/reference/quiet-study", RightsMode: "analysis_only", ValidationLevel: "observed", ValidationNote: "仅用于演示结构拆解"}, "")
	if err != nil {
		return err
	}
	framework, err := s.service.CreateFramework(ctx, actor, app.CreateFrameworkInput{BenchmarkID: benchmark.ID, Name: "噪音切换到香事仪式", VisualSequence: []string{"工作噪音", "点香动作", "烟迹与书房", "产品与引导"}, CopySequence: []string{"钩子", "需求时刻", "工艺证据", "行动引导"}}, "")
	if err != nil {
		return err
	}
	point, err := s.service.CreateSellingPoint(ctx, actor, app.CreateSellingPointInput{ProjectID: projectID, Title: "传统制香工艺带来的日常仪式感", Description: "用可观察的点香动作与真实产品素材呈现", Priority: 1, KnowledgeIDs: ids}, "")
	if err != nil {
		return err
	}
	plan, err := s.service.CreateVisualizationPlan(ctx, actor, app.CreateVisualizationPlanInput{SellingPointID: point.ID, Title: "真实线香与书房光影", ProofType: "process", Subjects: []string{"线香", "使用者的手"}, Setting: "傍晚书房", Props: []string{"香插", "书册"}, Implementation: "环境可生成，包装与 Logo 使用真实素材合成", ProductTruthStrategy: "real_asset_composite", Risks: []string{"生成文字变形"}, PlanB: "避开包装正面，只展示真实产品近景", AcceptanceCriteria: []string{"线香形制正确", "手部无畸变", "包装文字来自真实素材"}}, "")
	if err != nil {
		return err
	}
	plan, err = s.service.ReviewVisualizationPlan(ctx, actor, plan.ID, "approve", "")
	if err != nil {
		return err
	}
	brief, err := s.service.CreateBrief(ctx, actor, app.CreateBriefInput{ProjectID: projectID, Objective: "建立金陵古法线香的文化认知并引导收藏", Audience: "重视居家氛围与传统文化的 25-40 岁城市用户", DemandMoment: "结束一天工作，希望从信息噪音切换到安静独处", Scene: "晚间居家书桌，以真实线香完成安静仪式", Conflict: "高强度信息输入与恢复专注的需求冲突", PrimarySellingPoint: point.Title, EvidenceSummary: "使用已批准产品事实和真实点香过程证明日常仪式感", CTA: "进入品牌主页查看香事指南", Channel: "douyin", AspectRatio: "9:16", TargetDurationSeconds: 30, PrimaryTestVariable: "开场钩子", ApprovedKnowledgeIDs: ids, FrameworkIDs: []string{framework.ID}, VisualizationPlanIDs: []string{plan.ID}, Viewpoint: "user", Constraints: []string{"不得宣称医疗或保健功效"}}, "")
	if err != nil {
		return err
	}
	brief, err = s.service.ReviewBrief(ctx, actor, brief.ID, "submit", "")
	if err != nil {
		return err
	}
	_, err = s.service.ReviewBrief(ctx, actor, brief.ID, "approve", "")
	return err
}
