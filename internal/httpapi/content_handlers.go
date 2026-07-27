package httpapi

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.RevokeDevice(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "device.revoke", err)
		return
	}
	s.ok(w, r, "device.revoke", v)
}

func (s *Server) device(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Device(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "device.show", err)
		return
	}
	s.ok(w, r, "device.show", v)
}

func (s *Server) attachDevice(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.AttachDevice(r.Context(), actor, chi.URLParam(r, "id"), chi.URLParam(r, "projectID"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "device.attach", err)
		return
	}
	s.ok(w, r, "device.attach", v)
}

func (s *Server) detachDevice(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.DetachDevice(r.Context(), actor, chi.URLParam(r, "id"), chi.URLParam(r, "projectID"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "device.detach", err)
		return
	}
	s.ok(w, r, "device.detach", v)
}

func (s *Server) approveDeviceAuth(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		UserCode string `json:"user_code"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.ApproveUserDeviceLogin(r.Context(), actor, in.UserCode)
	if err != nil {
		s.fail(w, r, "auth.device.approve", err)
		return
	}
	s.ok(w, r, "auth.device.approve", v)
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.CancelRun(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "run.cancel", err)
		return
	}
	s.ok(w, r, "run.cancel", v)
}

func (s *Server) resolveReviewComment(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.ResolveReviewComment(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "review_comment.resolve", err)
		return
	}
	s.ok(w, r, "review_comment.resolve", v)
}

func (s *Server) createReviewGrant(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		ReviewerEmail string `json:"reviewer_email"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.CreateReviewGrant(r.Context(), actor, chi.URLParam(r, "id"), in.ReviewerEmail, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "review_grant.create", err)
		return
	}
	s.ok(w, r, "review_grant.create", v)
}

func (s *Server) reviewGrants(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.ReviewGrants(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "review_grant.list", err)
		return
	}
	s.ok(w, r, "review_grant.list", v)
}

func (s *Server) revokeReviewGrant(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.RevokeReviewGrant(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "review_grant.revoke", err)
		return
	}
	s.ok(w, r, "review_grant.revoke", v)
}

func (s *Server) approvedSnapshotArtifacts(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ApprovedSnapshotArtifacts(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "approved_snapshot.artifacts", err)
		return
	}
	s.ok(w, r, "approved_snapshot.artifacts", value)
}

func (s *Server) deliveryPackages(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.DeliveryPackages(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "delivery.list", err)
		return
	}
	s.ok(w, r, "delivery.list", value)
}

func (s *Server) deliveryPackage(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.DeliveryPackage(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "delivery.show", err)
		return
	}
	s.ok(w, r, "delivery.show", value)
}

func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	artifact, data, err := s.service.ArtifactBytes(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "artifact.download", err)
		return
	}
	w.Header().Set("Content-Type", artifact.MediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(filepath.Base(artifact.FileName), "\"", "")))
	w.Header().Set("X-Content-SHA256", artifact.SHA256)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) publicReviewProjection(w http.ResponseWriter, r *http.Request) {
	v, err := s.service.ReviewProjection(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		s.fail(w, r, "review.projection", err)
		return
	}
	s.ok(w, r, "review.projection", v)
}

func (s *Server) publicReviewVerify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OTP string `json:"otp"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.VerifyReviewGrant(r.Context(), chi.URLParam(r, "token"), in.OTP)
	if err != nil {
		s.fail(w, r, "review.verify", err)
		return
	}
	s.ok(w, r, "review.verify", v)
}

func (s *Server) publicReviewDecision(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
		ShotID   string `json:"shot_id"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.DecideReviewGrant(r.Context(), chi.URLParam(r, "token"), in.Decision, in.Reason, in.ShotID, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "review.decision", err)
		return
	}
	s.ok(w, r, "review.decision", v)
}
