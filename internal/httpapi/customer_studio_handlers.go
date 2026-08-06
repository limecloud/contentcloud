package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/app"
)

func (s *Server) customerStudioBootstrap(w http.ResponseWriter, r *http.Request) {
	actor, user := auth(r)
	value, err := s.service.CustomerStudioBootstrap(r.Context(), actor, user)
	s.dispatchResult(w, r, "studio.bootstrap", value, err)
}

func (s *Server) customerStudioTasks(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CustomerStudioTasks(r.Context(), actor)
	s.dispatchResult(w, r, "studio.task.list", value, err)
}

func (s *Server) customerStudioTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CustomerStudioTask(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "studio.task.show", value, err)
}

func (s *Server) createCustomerStudioTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.StudioCreateTaskInput
	if !s.decode(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	value, err := s.service.CreateCustomerStudioTask(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.task.create", value, err)
}

func (s *Server) customerStudioTaskAction(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.TaskActionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CustomerStudioTaskAction(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.task.action", value, err)
}

func (s *Server) addCustomerStudioInspiration(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.StudioAddInspirationInput
	if !s.decode(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	value, err := s.service.AddCustomerStudioInspiration(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.inspiration.add", value, err)
}

func (s *Server) decideCustomerStudioTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.GateDecisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.DecideCustomerStudioTask(r.Context(), actor, chi.URLParam(r, "taskID"), chi.URLParam(r, "decisionID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.decision.create", value, err)
}

func (s *Server) attachCustomerStudioAssets(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.StudioAttachAssetsInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.AttachCustomerStudioAssets(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.task_assets.attach", value, err)
}

func (s *Server) customerStudioAssets(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CustomerStudioAssets(r.Context(), actor, strings.TrimSpace(r.URL.Query().Get("project_id")))
	s.dispatchResult(w, r, "studio.asset.list", value, err)
}

func (s *Server) customerStudioDeliveries(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CustomerStudioDeliveries(r.Context(), actor)
	s.dispatchResult(w, r, "studio.delivery.list", value, err)
}
