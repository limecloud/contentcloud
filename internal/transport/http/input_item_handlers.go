package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/application"
)

func (s *Server) inputItems(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.InputItems(r.Context(), actor, application.InputItemQuery{
		ProjectID:      r.URL.Query().Get("project_id"),
		Status:         r.URL.Query().Get("status"),
		AssigneeUserID: r.URL.Query().Get("assignee_user_id"),
		Mine:           r.URL.Query().Get("mine") == "true",
	})
	s.dispatchResult(w, r, "input_item.list", value, err)
}

func (s *Server) createInputItem(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.CreateInputItemInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Work.CreateInputItem(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "input_item.create", value, err)
}

func (s *Server) inputItem(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.InputItem(r.Context(), actor, chi.URLParam(r, "id"))
	s.dispatchResult(w, r, "input_item.show", value, err)
}

func (s *Server) triageInputItem(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.TriageInputItemInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Work.TriageInputItem(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "input_item.triage", value, err)
}
