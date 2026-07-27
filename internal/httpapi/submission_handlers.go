package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Server) submissions(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Submissions(r.Context(), actor, chi.URLParam(r, "projectID"))
	s.dispatchResult(w, r, "submission.list", value, err)
}

func (s *Server) submissionDetails(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.SubmissionDetails(r.Context(), actor, chi.URLParam(r, "id"))
	s.dispatchResult(w, r, "submission.show", value, err)
}

func (s *Server) projectSubmissionRevision(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ProjectSubmissionRevision(r.Context(), actor, chi.URLParam(r, "projectID"), chi.URLParam(r, "id"))
	s.dispatchResult(w, r, "submission.revision.show", value, err)
}

func (s *Server) approveSubmission(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.ApproveSubmission(r.Context(), actor, chi.URLParam(r, "id"), input.Reason, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "submission.approve", value, err)
}

func (s *Server) requestSubmissionChanges(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		Reason      string `json:"reason"`
		JSONPointer string `json:"json_pointer"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.RequestSubmissionChanges(r.Context(), actor, chi.URLParam(r, "id"), input.Reason, input.JSONPointer, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "submission.request_changes", value, err)
}

func (s *Server) approvedSnapshots(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ApprovedSnapshots(r.Context(), actor, chi.URLParam(r, "projectID"), r.URL.Query().Get("type"))
	if err != nil {
		s.fail(w, r, "snapshot.list", err)
		return
	}
	if value == nil {
		value = []domain.ApprovedSnapshot{}
	}
	s.ok(w, r, "snapshot.list", value)
}
