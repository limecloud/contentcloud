package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/app"
)

func (s *Server) providerProfiles(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ProviderProfiles(r.Context(), actor, r.URL.Query().Get("provider_id"))
	s.dispatchResult(w, r, "provider.profile.list", value, err)
}

func (s *Server) availableProviderProfiles(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.AvailableProviderProfiles(r.Context(), actor, r.URL.Query().Get("provider_id"))
	s.dispatchResult(w, r, "provider.profile.available", value, err)
}

func (s *Server) createProviderProfile(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateProviderProfileInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CreateProviderProfile(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "provider.profile.create", value, err)
}

func (s *Server) providerProfile(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ProviderProfile(r.Context(), actor, chi.URLParam(r, "providerID"), chi.URLParam(r, "version"))
	s.dispatchResult(w, r, "provider.profile.show", value, err)
}

func (s *Server) publishProviderProfile(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.PublishProviderProfile(r.Context(), actor, chi.URLParam(r, "providerID"), chi.URLParam(r, "version"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "provider.profile.publish", value, err)
}

func (s *Server) providerBinding(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ProviderBindingForActor(r.Context(), actor, actor.TenantID, chi.URLParam(r, "providerID"))
	s.dispatchResult(w, r, "provider.binding.show", value, err)
}

func (s *Server) saveProviderBinding(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.ConfigureProviderBindingInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.ConfigureProviderBinding(r.Context(), actor, actor.TenantID, chi.URLParam(r, "providerID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "provider.binding.configure", value, err)
}

func (s *Server) adminProviderBinding(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ProviderBindingForActor(r.Context(), actor, chi.URLParam(r, "tenantID"), chi.URLParam(r, "providerID"))
	s.dispatchResult(w, r, "provider.binding.show", value, err)
}

func (s *Server) saveAdminProviderBinding(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.ConfigureProviderBindingInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.ConfigureProviderBinding(r.Context(), actor, chi.URLParam(r, "tenantID"), chi.URLParam(r, "providerID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "provider.binding.configure", value, err)
}
