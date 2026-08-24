package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) runtimeCleanupDiagnostics(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	value, err := s.service.Operations.RuntimeCleanupDiagnostics(r.Context(), actor, r.URL.Query().Get("status"), limit)
	s.dispatchResult(w, r, "runtime.cleanup_diagnostics.list", value, err)
}

func (s *Server) runtimeCleanupDiagnostic(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Operations.RuntimeCleanupDiagnostic(r.Context(), actor, chi.URLParam(r, "diagnosticID"))
	s.dispatchResult(w, r, "runtime.cleanup_diagnostic.show", value, err)
}

func (s *Server) retryRuntimeCleanupDiagnostic(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Operations.RetryRuntimeCleanupDiagnostic(r.Context(), actor, chi.URLParam(r, "diagnosticID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "runtime.cleanup_diagnostic.retry", value, err)
}
