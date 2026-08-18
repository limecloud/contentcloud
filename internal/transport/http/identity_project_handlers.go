package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/application"
)

func (s *Server) tenants(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Identity.Tenants(r.Context(), actor)
	s.dispatchResult(w, r, "tenant.list", value, err)
}

func (s *Server) switchTenant(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TenantID string `json:"tenant_id"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	cookie, err := r.Cookie("cc_session")
	if err != nil {
		s.fail(w, r, "tenant.switch", err)
		return
	}
	session, err := s.service.Identity.SwitchTenant(r.Context(), cookie.Value, in.TenantID, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "tenant.switch", err)
		return
	}
	s.setSession(w, r, session)
	s.ok(w, r, "tenant.switch", session)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("cc_session")
	if err == nil {
		err = s.service.Identity.Logout(r.Context(), cookie.Value)
	}
	if err != nil {
		s.fail(w, r, "session.logout", err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "cc_session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: requestIsHTTPS(r), MaxAge: -1})
	s.ok(w, r, "session.logout", map[string]any{"logged_out": true})
}

func (s *Server) members(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Identity.Members(r.Context(), actor)
	s.dispatchResult(w, r, "membership.list", value, err)
}

func (s *Server) updateMembershipRole(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		Role string `json:"role"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	value, err := s.service.Identity.UpdateMembershipRole(r.Context(), actor, chi.URLParam(r, "userID"), in.Role, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "membership.update", value, err)
}

func (s *Server) revokeMembership(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Identity.RevokeMembership(r.Context(), actor, chi.URLParam(r, "userID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "membership.revoke", value, err)
}

func (s *Server) membershipInvites(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Identity.MembershipInvites(r.Context(), actor)
	s.dispatchResult(w, r, "membership.invite.list", value, err)
}

func (s *Server) createMembershipInvite(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	value, err := s.service.Identity.CreateMembershipInvite(r.Context(), actor, in.Email, in.Role, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "membership.invite.create", value, err)
}

func (s *Server) acceptMembershipInvite(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		Token string `json:"token"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	value, err := s.service.Identity.AcceptMembershipInvite(r.Context(), actor, in.Token, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "membership.invite.accept", value, err)
}

func (s *Server) revokeMembershipInvite(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Identity.RevokeMembershipInvite(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "membership.invite.revoke", value, err)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in application.UpdateProjectInput
	if !s.decode(w, r, &in) {
		return
	}
	value, err := s.service.Identity.UpdateProject(r.Context(), actor, chi.URLParam(r, "projectID"), in, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "project.update", value, err)
}

func (s *Server) archiveProject(w http.ResponseWriter, r *http.Request) {
	s.projectLifecycle(w, r, "archive")
}

func (s *Server) restoreProject(w http.ResponseWriter, r *http.Request) {
	s.projectLifecycle(w, r, "restore")
}

func (s *Server) projectLifecycle(w http.ResponseWriter, r *http.Request, action string) {
	actor, _ := auth(r)
	var in struct {
		RowVersion int `json:"row_version"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	value, err := s.service.Identity.SetProjectLifecycle(r.Context(), actor, chi.URLParam(r, "projectID"), action, in.RowVersion, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "project."+action, value, err)
}

func (s *Server) projectTemplates(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Identity.ProjectTemplates(r.Context(), actor)
	s.dispatchResult(w, r, "project_template.list", value, err)
}

func (s *Server) createProjectTemplate(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in application.CreateProjectTemplateInput
	if !s.decode(w, r, &in) {
		return
	}
	value, err := s.service.Identity.CreateProjectTemplate(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "project_template.create", value, err)
}

func (s *Server) cancelConnectSession(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Identity.CancelConnectSession(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "device.connect_session.cancel", value, err)
}
