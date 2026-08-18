package httpapi

import (
	_ "embed"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed bootstrap.md
var bootstrapDocument string

func (s *Server) bootstrap(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, bootstrapDocument)
}

func (s *Server) bootstrapActions(w http.ResponseWriter, r *http.Request) {
	catalog, err := s.service.Workspace.BootstrapActionCatalog()
	if err != nil {
		s.fail(w, r, "bootstrap.actions", err)
		return
	}
	s.ok(w, r, "bootstrap.actions", catalog)
}

func (s *Server) bootstrapAuthorizationView(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Workspace.BootstrapAuthorizationView(r.Context(), actor, chi.URLParam(r, "projectID"), chi.URLParam(r, "attemptID"))
	s.dispatchResult(w, r, "bootstrap.authorization.show", value, err)
}

func (s *Server) approveBootstrapAuthorization(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Workspace.ApproveBootstrapAuthorization(r.Context(), actor, chi.URLParam(r, "id"), chi.URLParam(r, "attemptID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "bootstrap.authorization.approve", value, err)
}

func (s *Server) denyBootstrapAuthorization(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Workspace.DenyBootstrapAuthorization(r.Context(), actor, chi.URLParam(r, "id"), chi.URLParam(r, "attemptID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "bootstrap.authorization.deny", value, err)
}
