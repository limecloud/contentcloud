package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) contentProfiles(w http.ResponseWriter, r *http.Request) {
	s.ok(w, r, "content_profile.list", s.service.Catalog.ContentProfiles())
}

func (s *Server) installContentProfile(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Catalog.InstallContentProfile(r.Context(), actor, chi.URLParam(r, "profileID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "content_profile.install", value, err)
}
