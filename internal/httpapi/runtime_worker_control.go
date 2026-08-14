package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	runtimeControlHeartbeatInterval = 20 * time.Second
	runtimeControlIOTimeout         = 5 * time.Second
	runtimeControlSyncTimeout       = 10 * time.Second
)

type runtimeWakeHub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan struct{}]struct{}
}

func newRuntimeWakeHub() *runtimeWakeHub {
	return &runtimeWakeHub{subscribers: map[string]map[chan struct{}]struct{}{}}
}

func (h *runtimeWakeHub) subscribe(tenantID string) (<-chan struct{}, func()) {
	wake := make(chan struct{}, 1)
	h.mu.Lock()
	if h.subscribers[tenantID] == nil {
		h.subscribers[tenantID] = map[chan struct{}]struct{}{}
	}
	h.subscribers[tenantID][wake] = struct{}{}
	h.mu.Unlock()
	return wake, func() {
		h.mu.Lock()
		delete(h.subscribers[tenantID], wake)
		if len(h.subscribers[tenantID]) == 0 {
			delete(h.subscribers, tenantID)
		}
		h.mu.Unlock()
	}
}

func (h *runtimeWakeHub) publish(tenantID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for wake := range h.subscribers[strings.TrimSpace(tenantID)] {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

type runtimeControlFrame struct {
	Type                  string                              `json:"type"`
	DaemonInstanceID      string                              `json:"daemon_instance_id,omitempty"`
	ConnectionEpoch       int64                               `json:"connection_epoch,omitempty"`
	ReportSequence        int64                               `json:"report_seq,omitempty"`
	PID                   int                                 `json:"pid,omitempty"`
	Version               string                              `json:"version,omitempty"`
	State                 string                              `json:"state,omitempty"`
	Capabilities          map[string]any                      `json:"capabilities,omitempty"`
	WorkspaceObservations []domain.DaemonWorkspaceObservation `json:"workspace_observations,omitempty"`
	ActiveAttempts        []string                            `json:"active_attempts,omitempty"`
	StartedAt             time.Time                           `json:"started_at,omitempty"`
}

func (s *Server) runtimeWorkerControl(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	actor, _, err := s.service.DeviceActor(r.Context(), token)
	if err != nil {
		status := http.StatusServiceUnavailable
		var domainErr *domain.Error
		if errors.As(err, &domainErr) && domainErr.Code == "DEVICE_TOKEN_INVALID" {
			status = http.StatusUnauthorized
		}
		http.Error(w, http.StatusText(status), status)
		return
	}
	connection, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	connection.SetReadLimit(128 << 10)

	frame, err := readRuntimeControlSync(r.Context(), connection)
	if err != nil {
		connection.Close(websocket.StatusPolicyViolation, "control.sync_state required")
		return
	}
	instance, err := s.reportRuntimeControlState(r.Context(), actor, frame, "connected")
	if err != nil {
		connection.Close(websocket.StatusPolicyViolation, "invalid daemon instance state")
		return
	}
	lastFrame := frame
	defer func() {
		lastFrame.ReportSequence++
		disconnectCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), runtimeControlIOTimeout)
		defer cancel()
		_, _ = s.reportRuntimeControlState(disconnectCtx, actor, lastFrame, "stopped")
	}()

	wakes, unsubscribe := s.runtimeWakeHub.subscribe(actor.TenantID)
	defer unsubscribe()
	readerFrames := make(chan runtimeControlFrame, 1)
	readerDone := make(chan error, 1)
	go func() {
		for {
			_, body, readErr := connection.Read(r.Context())
			if readErr != nil {
				readerDone <- readErr
				return
			}
			var next runtimeControlFrame
			if json.Unmarshal(body, &next) != nil || next.Type != "control.sync_state" {
				readerDone <- domain.Invalid("DAEMON_CONTROL_FRAME_INVALID", "Runtime 控制通道只接受 current-state 状态报告")
				return
			}
			select {
			case readerFrames <- next:
			case <-r.Context().Done():
				return
			}
		}
	}()

	if err := writeRuntimeControlFrame(r.Context(), connection, runtimeControlFrame{Type: "control.ready", DaemonInstanceID: instance.ID, ConnectionEpoch: instance.ConnectionEpoch, ReportSequence: instance.ReportSequence}); err != nil {
		return
	}
	if err := writeRuntimeControlFrame(r.Context(), connection, runtimeControlFrame{Type: "runtime.available"}); err != nil {
		return
	}
	heartbeat := time.NewTicker(runtimeControlHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-readerDone:
			return
		case next := <-readerFrames:
			if next.DaemonInstanceID != instance.ID || next.ConnectionEpoch != instance.ConnectionEpoch || next.ReportSequence <= lastFrame.ReportSequence {
				connection.Close(websocket.StatusPolicyViolation, "stale daemon instance report")
				return
			}
			if _, err := s.reportRuntimeControlState(r.Context(), actor, next, "connected"); err != nil {
				connection.Close(websocket.StatusPolicyViolation, "invalid daemon instance report")
				return
			}
			lastFrame = next
		case <-wakes:
			if err := writeRuntimeControlFrame(r.Context(), connection, runtimeControlFrame{Type: "runtime.available"}); err != nil {
				return
			}
		case <-heartbeat.C:
			pingCtx, cancel := context.WithTimeout(r.Context(), runtimeControlIOTimeout)
			err := connection.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
			if err := writeRuntimeControlFrame(r.Context(), connection, runtimeControlFrame{Type: "control.heartbeat"}); err != nil {
				return
			}
		}
	}
}

func readRuntimeControlSync(ctx context.Context, connection *websocket.Conn) (runtimeControlFrame, error) {
	readCtx, cancel := context.WithTimeout(ctx, runtimeControlSyncTimeout)
	defer cancel()
	messageType, body, err := connection.Read(readCtx)
	if err != nil {
		return runtimeControlFrame{}, err
	}
	var frame runtimeControlFrame
	if messageType != websocket.MessageText || json.Unmarshal(body, &frame) != nil || frame.Type != "control.sync_state" {
		return runtimeControlFrame{}, domain.Invalid("DAEMON_CONTROL_SYNC_REQUIRED", "Runtime 控制通道第一帧必须是 current-state 状态报告")
	}
	return frame, nil
}

func (s *Server) reportRuntimeControlState(ctx context.Context, actor app.Actor, frame runtimeControlFrame, state string) (domain.DaemonInstance, error) {
	return s.service.ReportDaemonInstance(ctx, actor, app.DaemonInstanceReportInput{
		ID: frame.DaemonInstanceID, ConnectionEpoch: frame.ConnectionEpoch, ReportSequence: frame.ReportSequence,
		PID: frame.PID, Version: frame.Version, State: state, Capabilities: frame.Capabilities,
		ActiveAttempts: frame.ActiveAttempts, StartedAt: frame.StartedAt,
		WorkspaceObservations: frame.WorkspaceObservations,
	})
}

func writeRuntimeControlFrame(ctx context.Context, connection *websocket.Conn, frame runtimeControlFrame) error {
	body, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, runtimeControlIOTimeout)
	defer cancel()
	return connection.Write(writeCtx, websocket.MessageText, body)
}
