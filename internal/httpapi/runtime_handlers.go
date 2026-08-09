package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Server) runtimeJobs(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	value, err := s.service.RuntimeJobs(r.Context(), actor, r.URL.Query().Get("project_id"), r.URL.Query().Get("state"), after, limit)
	s.dispatchResult(w, r, "runtime.jobs.list", value, err)
}

func (s *Server) runtimeJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.RuntimeJobDetail(r.Context(), actor, chi.URLParam(r, "jobID"))
	s.dispatchResult(w, r, "runtime.job.show", value, err)
}

func (s *Server) runtimeJobNodes(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	value, err := s.service.RuntimeJobNodesPage(r.Context(), actor, chi.URLParam(r, "jobID"), after, limit)
	s.dispatchResult(w, r, "runtime.job.nodes", value, err)
}

func (s *Server) runtimeJobEvents(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	value, err := s.service.RuntimeJobEvents(r.Context(), actor, chi.URLParam(r, "jobID"), after, limit)
	s.dispatchResult(w, r, "runtime.job.events", value, err)
}

func (s *Server) runtimeJobEventsStream(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	jobID := chi.URLParam(r, "jobID")
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if queryAfter, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64); err == nil && queryAfter > after {
		after = queryAfter
	}
	if _, err := s.service.RuntimeJobDetail(r.Context(), actor, jobID); err != nil {
		s.fail(w, r, "runtime.job.events.stream", err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, r, "runtime.job.events.stream", domain.E("internal", "stream", "STREAM_UNSUPPORTED", "服务端不支持运行时事件流", 1))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = io.WriteString(w, "retry: 1000\n\n")
	flusher.Flush()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		events, err := s.service.RuntimeJobEvents(r.Context(), actor, jobID, after)
		if err != nil {
			return
		}
		for _, event := range events {
			body, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "id: %d\nevent: runtime\ndata: %s\n\n", event.Sequence, body)
			after = event.Sequence
		}
		if len(events) > 0 {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			_, _ = io.WriteString(w, "event: reconnect\ndata: {}\n\n")
			flusher.Flush()
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) runtimeJobEffects(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	value, err := s.service.RuntimeJobEffectsPage(r.Context(), actor, chi.URLParam(r, "jobID"), after, limit)
	s.dispatchResult(w, r, "runtime.job.effects", value, err)
}

func (s *Server) runtimeJobCheckpoints(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	value, err := s.service.RuntimeJobCheckpointsPage(r.Context(), actor, chi.URLParam(r, "jobID"), after, limit)
	s.dispatchResult(w, r, "runtime.job.checkpoints", value, err)
}

func (s *Server) refreshRuntimeJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.RefreshRuntimeJob(r.Context(), actor, chi.URLParam(r, "jobID"))
	s.dispatchResult(w, r, "runtime.job.refresh", value, err)
}

func (s *Server) cancelRuntimeJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CancelRuntimeJob(r.Context(), actor, chi.URLParam(r, "jobID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "runtime.job.cancel", value, err)
}

func (s *Server) pauseRuntimeJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.PauseRuntimeJob(r.Context(), actor, chi.URLParam(r, "jobID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "runtime.job.pause", value, err)
}

func (s *Server) resumeRuntimeJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ResumeRuntimeJob(r.Context(), actor, chi.URLParam(r, "jobID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "runtime.job.resume", value, err)
}

func (s *Server) forkRuntimeCheckpoint(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.ForkRuntimeCheckpoint(r.Context(), actor, chi.URLParam(r, "checkpointID"), input.IdempotencyKey, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "runtime.checkpoint.fork", value, err)
}

func (s *Server) replayRuntimeJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	dryRun := false
	if raw := r.URL.Query().Get("dry_run"); raw != "" {
		parsed, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			s.fail(w, r, "runtime.job.replay", domain.Invalid("RUNTIME_REPLAY_DRY_RUN_INVALID", "dry_run 必须是布尔值"))
			return
		}
		dryRun = parsed
	}
	value, err := s.service.ReplayRuntimeJobWithOptions(r.Context(), actor, chi.URLParam(r, "jobID"), after, dryRun)
	s.dispatchResult(w, r, "runtime.job.replay", value, err)
}

func (s *Server) reconcileRuntimeEffect(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.BeginRuntimeEffectReconciliation(r.Context(), actor, chi.URLParam(r, "effectID"), input.ExpectedVersion, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "runtime.effect.reconcile", value, err)
}
