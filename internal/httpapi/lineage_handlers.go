package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/limecloud/contentcloud/internal/app"
)

func lineageQuery(r *http.Request) app.LineageQuery {
	return app.LineageQuery{
		FocusType: r.URL.Query().Get("focus_type"),
		FocusID:   r.URL.Query().Get("focus_id"),
		Direction: r.URL.Query().Get("direction"),
	}
}

func (s *Server) projectLineage(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ProjectLineage(r.Context(), actor, chi.URLParam(r, "projectID"), lineageQuery(r))
	s.dispatchResult(w, r, "lineage.show", value, err)
}

func (s *Server) projectImpact(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ProjectImpact(r.Context(), actor, chi.URLParam(r, "projectID"), lineageQuery(r))
	s.dispatchResult(w, r, "lineage.impact", value, err)
}
