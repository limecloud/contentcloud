package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/application"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
)

// providerBindingResponse deliberately exposes only whether a credential
// reference exists. The reference itself remains an ingress-only value.
type providerBindingResponse struct {
	deliverydomain.ProviderBinding
	CredentialConfigured bool `json:"credential_configured"`
}

func safeProviderBinding(value deliverydomain.ProviderBinding) providerBindingResponse {
	return providerBindingResponse{ProviderBinding: value, CredentialConfigured: strings.TrimSpace(value.CredentialRef) != ""}
}

func providerBindingResult(value deliverydomain.ProviderBinding, err error) any {
	if err != nil {
		return value
	}
	return safeProviderBinding(value)
}

func (s *Server) providerProfiles(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Delivery.ProviderProfiles(r.Context(), actor, r.URL.Query().Get("provider_id"))
	s.dispatchResult(w, r, "provider.profile.list", value, err)
}

func (s *Server) availableProviderProfiles(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Delivery.AvailableProviderProfiles(r.Context(), actor, r.URL.Query().Get("provider_id"))
	s.dispatchResult(w, r, "provider.profile.available", value, err)
}

func (s *Server) createProviderProfile(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.CreateProviderProfileInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.CreateProviderProfile(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "provider.profile.create", value, err)
}

func (s *Server) providerProfile(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Delivery.ProviderProfile(r.Context(), actor, chi.URLParam(r, "providerID"), chi.URLParam(r, "version"))
	s.dispatchResult(w, r, "provider.profile.show", value, err)
}

func (s *Server) publishProviderProfile(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Delivery.PublishProviderProfile(r.Context(), actor, chi.URLParam(r, "providerID"), chi.URLParam(r, "version"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "provider.profile.publish", value, err)
}

func (s *Server) providerBinding(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Delivery.ProviderBindingForActor(r.Context(), actor, actor.TenantID, chi.URLParam(r, "providerID"))
	s.dispatchResult(w, r, "provider.binding.show", providerBindingResult(value, err), err)
}

func (s *Server) saveProviderBinding(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.ConfigureProviderBindingInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.ConfigureProviderBinding(r.Context(), actor, actor.TenantID, chi.URLParam(r, "providerID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "provider.binding.configure", providerBindingResult(value, err), err)
}

func (s *Server) adminProviderBinding(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Delivery.ProviderBindingForActor(r.Context(), actor, chi.URLParam(r, "tenantID"), chi.URLParam(r, "providerID"))
	s.dispatchResult(w, r, "provider.binding.show", providerBindingResult(value, err), err)
}

func (s *Server) saveAdminProviderBinding(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.ConfigureProviderBindingInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.ConfigureProviderBinding(r.Context(), actor, chi.URLParam(r, "tenantID"), chi.URLParam(r, "providerID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "provider.binding.configure", providerBindingResult(value, err), err)
}
