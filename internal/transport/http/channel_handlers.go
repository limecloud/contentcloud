package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/application"
)

func (s *Server) channelAdapters(w http.ResponseWriter, r *http.Request) {
	s.ok(w, r, "channel.adapter.list", s.service.Delivery.ChannelAdapterIDs())
}

func (s *Server) createChannelBinding(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.CreateChannelBindingInput
	if !s.decode(w, r, &input) {
		return
	}
	input.ProjectID = chi.URLParam(r, "projectID")
	value, err := s.service.Delivery.CreateChannelBinding(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "channel.binding.create", value, err)
}

func (s *Server) channelBindings(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Delivery.ChannelBindings(r.Context(), actor, chi.URLParam(r, "projectID"))
	s.dispatchResult(w, r, "channel.binding.list", value, err)
}

func (s *Server) prepareChannelPublication(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.PrepareChannelPublicationInput
	if !s.decode(w, r, &input) {
		return
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	value, err := s.service.Delivery.PrepareChannelPublication(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "channel.publication.prepare", value, err)
}

func (s *Server) submitChannelPublication(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Delivery.SubmitChannelPublication(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "channel.publication.submit", value, err)
}

func (s *Server) inspectChannelPublication(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Delivery.InspectChannelPublication(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "channel.publication.inspect", value, err)
}

func (s *Server) recordManualChannelReceipt(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.RecordManualChannelReceiptInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.RecordManualChannelReceipt(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "channel.publication.receipt", value, err)
}

func (s *Server) withdrawChannelPublication(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.WithdrawChannelPublication(r.Context(), actor, chi.URLParam(r, "id"), input.Reason, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "channel.publication.withdraw", value, err)
}

func (s *Server) channelPublications(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Delivery.ChannelPublications(r.Context(), actor, r.URL.Query().Get("task_id"))
	s.dispatchResult(w, r, "channel.publication.list", value, err)
}

func (s *Server) reconcileChannelPublications(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		Limit int `json:"limit"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.ReconcileChannelPublications(r.Context(), actor, input.Limit, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "channel.publication.reconcile", value, err)
}

func (s *Server) importChannelPerformance(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.ImportChannelPerformanceInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.ImportChannelPerformance(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "channel.performance.import", value, err)
}
