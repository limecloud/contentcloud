package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/app"
)

func (s *Server) modelProviders(w http.ResponseWriter, r *http.Request) {
	s.ok(w, r, "model.provider.list", s.service.ModelProviderIDs())
}

func (s *Server) generateModelCandidate(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.GenerateModelCandidateInput
	if !s.decodeLimit(w, r, &input, 2<<20) {
		return
	}
	value, err := s.service.GenerateModelCandidate(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "model.candidate.generate", value, err)
}

func (s *Server) modelGenerationReceipts(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ModelGenerationReceipts(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "model.receipt.list", value, err)
}
