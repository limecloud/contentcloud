package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) projectProjection(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Operations.ProjectProjection(r.Context(), actor, chi.URLParam(r, "projectID"))
	s.dispatchResult(w, r, "project.projection", value, err)
}
