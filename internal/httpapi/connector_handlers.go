package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/app"
)

func (s *Server) connectorAdapters(w http.ResponseWriter, r *http.Request) {
	s.ok(w, r, "connector.adapter.list", s.service.ConnectorAdapterIDs())
}

func (s *Server) createConnectorBinding(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateConnectorBindingInput
	if !s.decode(w, r, &input) {
		return
	}
	input.ProjectID = chi.URLParam(r, "projectID")
	value, err := s.service.CreateConnectorBinding(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "connector.binding.create", value, err)
}

func (s *Server) connectorBindings(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ConnectorBindings(r.Context(), actor, chi.URLParam(r, "projectID"))
	s.dispatchResult(w, r, "connector.binding.list", value, err)
}

func (s *Server) syncConnector(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.SyncConnectorInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.SyncConnector(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "connector.sync", value, err)
}

func (s *Server) connectorReceipts(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ConnectorReceipts(r.Context(), actor, r.URL.Query().Get("binding_id"))
	s.dispatchResult(w, r, "connector.receipt.list", value, err)
}
