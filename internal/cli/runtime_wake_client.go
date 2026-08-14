package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	runtimeWakeReconnectBase = time.Second
	runtimeWakeReconnectMax  = 30 * time.Second
	runtimeWakeReadTimeout   = 45 * time.Second
	runtimeWakeWriteTimeout  = 5 * time.Second
)

type runtimeWakeFrame struct {
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

type runtimeWakeObservation struct {
	State     string
	ErrorCode string
}

type runtimeWakeClientState struct {
	mu                    sync.Mutex
	instanceID            string
	connectionEpoch       int64
	reportSequence        int64
	pid                   int
	version               string
	startedAt             time.Time
	capabilities          map[string]any
	workspaceObservations []domain.DaemonWorkspaceObservation
	activeAttempts        map[string]struct{}
	changed               chan struct{}
}

func newRuntimeWakeClientState(version string, capabilities map[string]any) *runtimeWakeClientState {
	return &runtimeWakeClientState{
		instanceID: uuid.NewString(), pid: os.Getpid(), version: strings.TrimSpace(version),
		startedAt: time.Now().UTC(), capabilities: cloneRuntimeCapabilities(capabilities), workspaceObservations: []domain.DaemonWorkspaceObservation{},
		activeAttempts: map[string]struct{}{}, changed: make(chan struct{}, 1),
	}
}

func (s *runtimeWakeClientState) beginConnection() runtimeWakeFrame {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectionEpoch++
	s.reportSequence = 0
	return s.nextFrameLocked()
}

func (s *runtimeWakeClientState) snapshot() runtimeWakeFrame {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextFrameLocked()
}

func (s *runtimeWakeClientState) nextFrameLocked() runtimeWakeFrame {
	s.reportSequence++
	active := make([]string, 0, len(s.activeAttempts))
	for attemptID := range s.activeAttempts {
		active = append(active, attemptID)
	}
	sort.Strings(active)
	return runtimeWakeFrame{
		Type: "control.sync_state", DaemonInstanceID: s.instanceID,
		ConnectionEpoch: s.connectionEpoch, ReportSequence: s.reportSequence,
		PID: s.pid, Version: s.version, State: "connected", StartedAt: s.startedAt,
		Capabilities: cloneRuntimeCapabilities(s.capabilities), ActiveAttempts: active,
		WorkspaceObservations: append([]domain.DaemonWorkspaceObservation(nil), s.workspaceObservations...),
	}
}

func (s *runtimeWakeClientState) setWorkspaceObservations(observations []domain.DaemonWorkspaceObservation) {
	if s == nil {
		return
	}
	s.mu.Lock()
	next := append([]domain.DaemonWorkspaceObservation(nil), observations...)
	changed := !reflect.DeepEqual(s.workspaceObservations, next)
	if changed {
		s.workspaceObservations = next
	}
	s.mu.Unlock()
	if changed {
		signalRuntimeWake(s.changed)
	}
}

func (s *runtimeWakeClientState) setAttempt(attemptID string, active bool) {
	if s == nil || strings.TrimSpace(attemptID) == "" {
		return
	}
	s.mu.Lock()
	changed := false
	if active {
		if _, exists := s.activeAttempts[strings.TrimSpace(attemptID)]; !exists {
			s.activeAttempts[strings.TrimSpace(attemptID)] = struct{}{}
			changed = true
		}
	} else {
		if _, exists := s.activeAttempts[strings.TrimSpace(attemptID)]; exists {
			delete(s.activeAttempts, strings.TrimSpace(attemptID))
			changed = true
		}
	}
	s.mu.Unlock()
	if changed {
		signalRuntimeWake(s.changed)
	}
}

func (s *runtimeWakeClientState) setCapabilities(capabilities map[string]any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	next := cloneRuntimeCapabilities(capabilities)
	changed := !reflect.DeepEqual(s.capabilities, next)
	if changed {
		s.capabilities = next
	}
	s.mu.Unlock()
	if changed {
		signalRuntimeWake(s.changed)
	}
}

func (s *runtimeWakeClientState) capabilitiesSnapshot() map[string]any {
	if s == nil {
		return map[string]any{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneRuntimeCapabilities(s.capabilities)
}

func (s *runtimeWakeClientState) runtimeAvailable() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status, _ := s.capabilities["runtime_status"].(string)
	return status == "healthy"
}

func cloneRuntimeCapabilities(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

var errRuntimeWakeAuthRejected = errors.New("runtime control credential rejected")

func runRuntimeWakeClient(ctx context.Context, serverURL, token string, wake chan<- struct{}, stderr io.Writer, observe func(runtimeWakeObservation)) {
	runRuntimeWakeClientWithState(ctx, serverURL, token, wake, stderr, observe, newRuntimeWakeClientState(Version, nil), nil)
}

func runRuntimeWakeClientWithState(ctx context.Context, serverURL, token string, wake chan<- struct{}, stderr io.Writer, observe func(runtimeWakeObservation), state *runtimeWakeClientState, ready chan<- struct{}) {
	attempt := 0
	for ctx.Err() == nil {
		notifyRuntimeWake(observe, runtimeWakeObservation{State: "connecting"})
		err := readRuntimeWakeConnectionWithState(ctx, serverURL, token, wake, func() {
			attempt = 0
			notifyRuntimeWake(observe, runtimeWakeObservation{State: "open"})
			signalRuntimeWake(ready)
		}, state)
		if ctx.Err() != nil {
			notifyRuntimeWake(observe, runtimeWakeObservation{State: "stopped"})
			return
		}
		if errors.Is(err, errRuntimeWakeAuthRejected) {
			fmt.Fprintln(stderr, "runtime control channel stopped: device credential rejected")
			notifyRuntimeWake(observe, runtimeWakeObservation{State: "auth_rejected", ErrorCode: "DEVICE_AUTH_REJECTED"})
			return
		}
		if err != nil {
			fmt.Fprintf(stderr, "runtime control channel disconnected: %v\n", err)
		}
		delay := runtimeWakeReconnectDelay(attempt, rand.Float64())
		if attempt < 30 {
			attempt++
		}
		notifyRuntimeWake(observe, runtimeWakeObservation{State: "backoff", ErrorCode: "CONTROL_DISCONNECTED"})
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func readRuntimeWakeConnection(ctx context.Context, serverURL, token string, wake chan<- struct{}, accepted func()) error {
	return readRuntimeWakeConnectionWithState(ctx, serverURL, token, wake, accepted, newRuntimeWakeClientState(Version, nil))
}

func readRuntimeWakeConnectionWithState(ctx context.Context, serverURL, token string, wake chan<- struct{}, accepted func(), state *runtimeWakeClientState) error {
	controlURL, err := runtimeControlURL(serverURL)
	if err != nil {
		return err
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	connection, response, err := websocket.Dial(ctx, controlURL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		if response != nil {
			if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
				return fmt.Errorf("%w: HTTP %d", errRuntimeWakeAuthRejected, response.StatusCode)
			}
			return fmt.Errorf("control handshake returned HTTP %d: %w", response.StatusCode, err)
		}
		return err
	}
	defer connection.CloseNow()
	connection.SetReadLimit(128 << 10)
	syncFrame := state.beginConnection()
	if err := writeRuntimeWakeFrame(ctx, connection, syncFrame); err != nil {
		return err
	}
	serverFrames := make(chan runtimeWakeFrame, 1)
	readerDone := make(chan error, 1)
	go func() {
		for {
			readCtx, cancel := context.WithTimeout(ctx, runtimeWakeReadTimeout)
			messageType, body, readErr := connection.Read(readCtx)
			cancel()
			if readErr != nil {
				readerDone <- readErr
				return
			}
			if messageType != websocket.MessageText {
				continue
			}
			var frame runtimeWakeFrame
			if json.Unmarshal(body, &frame) != nil || strings.TrimSpace(frame.Type) == "" {
				continue
			}
			select {
			case serverFrames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	acceptedFrame := false
	stateDirty := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readerDone:
			return err
		case <-state.changed:
			if !acceptedFrame {
				stateDirty = true
				continue
			}
			if err := writeRuntimeWakeFrame(ctx, connection, state.snapshot()); err != nil {
				return err
			}
		case frame := <-serverFrames:
			if !acceptedFrame {
				if frame.Type != "control.ready" || (frame.DaemonInstanceID != "" && frame.DaemonInstanceID != syncFrame.DaemonInstanceID) || (frame.ConnectionEpoch != 0 && frame.ConnectionEpoch != syncFrame.ConnectionEpoch) {
					return errors.New("runtime control channel did not acknowledge current daemon state")
				}
				acceptedFrame = true
				if accepted != nil {
					accepted()
				}
				if stateDirty {
					stateDirty = false
					if err := writeRuntimeWakeFrame(ctx, connection, state.snapshot()); err != nil {
						return err
					}
				}
				continue
			}
			switch frame.Type {
			case "runtime.available":
				signalRuntimeWake(wake)
			case "control.heartbeat":
				if err := writeRuntimeWakeFrame(ctx, connection, state.snapshot()); err != nil {
					return err
				}
			}
		}
	}
}

func writeRuntimeWakeFrame(ctx context.Context, connection *websocket.Conn, frame runtimeWakeFrame) error {
	body, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, runtimeWakeWriteTimeout)
	defer cancel()
	return connection.Write(writeCtx, websocket.MessageText, body)
}

func notifyRuntimeWake(observe func(runtimeWakeObservation), observation runtimeWakeObservation) {
	if observe != nil {
		observe(observation)
	}
}

func signalRuntimeWake(wake chan<- struct{}) {
	if wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

func runtimeControlURL(serverURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", fmt.Errorf("unsupported server URL scheme %q", parsed.Scheme)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/v1/runtime/worker/control"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func runtimeWakeReconnectDelay(attempt int, jitter float64) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 1 {
		jitter = 1
	}
	delay := runtimeWakeReconnectBase
	for i := 0; i < attempt && delay < runtimeWakeReconnectMax; i++ {
		delay *= 2
		if delay > runtimeWakeReconnectMax {
			delay = runtimeWakeReconnectMax
		}
	}
	return delay + time.Duration(float64(delay/4)*jitter)
}
