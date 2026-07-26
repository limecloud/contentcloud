package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) platformOverview(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.PlatformOverview(r.Context(), actor)
	s.dispatchResult(w, r, "platform.overview", value, err)
}

func (s *Server) updatePlatformTenant(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		Status string `json:"status"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.UpdatePlatformTenantStatus(r.Context(), actor, chi.URLParam(r, "tenantID"), input.Status, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "platform.tenant.update", value, err)
}
