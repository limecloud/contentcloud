package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/testsupport"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type failingDeviceLookupStore struct {
	*memory.Store
}

type runtimeControlFixture struct {
	service    *application.Application
	actor      application.Actor
	project    workspacedomain.Project
	connected  application.ConnectDeviceResult
	controlURL string
}

func (failingDeviceLookupStore) DeviceByTokenHash(context.Context, string) (workspacedomain.Device, error) {
	return workspacedomain.Device{}, errors.New("database unavailable")
}

func TestRuntimeWorkerControlAuthenticatesDeviceAndResyncsOnConnect(t *testing.T) {
	fixture := newRuntimeControlFixture(t)

	if connection, response, dialErr := websocket.Dial(t.Context(), fixture.controlURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer invalid"}}}); dialErr == nil {
		connection.Close(websocket.StatusNormalClosure, "")
		t.Fatal("invalid device credential opened the control channel")
	} else if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid credential response = %#v err=%v", response, dialErr)
	}

	connection, _, err := websocket.Dial(t.Context(), fixture.controlURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer " + fixture.connected.DeviceToken}}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	instanceID := idgen.New()
	syncBody, _ := json.Marshal(map[string]any{
		"type": "control.sync_state", "daemon_instance_id": instanceID,
		"connection_epoch": 1, "report_seq": 1, "pid": 42, "version": "test",
		"state": "connected", "capabilities": map[string]any{"harness_kind": "fake"},
		"active_attempts": []string{}, "started_at": time.Now().UTC(),
	})
	if err := connection.Write(t.Context(), websocket.MessageText, syncBody); err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"control.ready", "runtime.available"} {
		readCtx, cancel := context.WithTimeout(t.Context(), time.Second)
		_, body, readErr := connection.Read(readCtx)
		cancel()
		if readErr != nil {
			t.Fatal(readErr)
		}
		var frame struct {
			Type             string `json:"type"`
			DaemonInstanceID string `json:"daemon_instance_id"`
		}
		if json.Unmarshal(body, &frame) != nil || frame.Type != wanted {
			t.Fatalf("control frame = %s, want %s", body, wanted)
		}
		if wanted == "control.ready" && frame.DaemonInstanceID != instanceID {
			t.Fatalf("ready frame instance = %q, want %q", frame.DaemonInstanceID, instanceID)
		}
	}
	instances, err := fixture.service.Workspace.DaemonInstances(t.Context(), fixture.actor, fixture.connected.Device.ID)
	if err != nil || len(instances) != 1 || instances[0].ID != instanceID || instances[0].State != "connected" {
		t.Fatalf("daemon instance current state = %#v err=%v", instances, err)
	}

	sop := runtimeControlSOP(fixture.actor.TenantID)
	if _, err := fixture.service.Runtime.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: fixture.actor.TenantID, ProjectID: fixture.project.ID, WorkTaskID: "control-task", SOP: sop, BindingDigest: "sha256:" + strings.Repeat("a", 64), InputDigest: "sha256:" + strings.Repeat("b", 64), RuntimePolicyID: "runtime-policy/control", ContractMajor: 1, CreatedBy: fixture.actor.UserID, IdempotencyKey: "control-job"}); err != nil {
		t.Fatal(err)
	}
	readCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, body, err := connection.Read(readCtx)
	if err != nil {
		t.Fatal(err)
	}
	var frame struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(body, &frame) != nil || frame.Type != "runtime.available" {
		t.Fatalf("runtime notification frame = %s", body)
	}
}

func TestRuntimeWorkerControlRejectsDuplicateAndSameEpochResurrection(t *testing.T) {
	fixture := newRuntimeControlFixture(t)
	instanceID := idgen.New()
	startedAt := time.Now().UTC().Add(-time.Minute)
	connection := openRuntimeControl(t, fixture, instanceID, 1, 1, startedAt)

	writeRuntimeControlSync(t, connection, instanceID, 1, 1, startedAt)
	readCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	_, _, readErr := connection.Read(readCtx)
	cancel()
	connection.CloseNow()
	if readErr == nil {
		t.Fatal("duplicate report_seq remained accepted on the control channel")
	}
	waitRuntimeControlState(t, fixture, instanceID, "stopped", 1, 2)

	connection, _, err := websocket.Dial(t.Context(), fixture.controlURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer " + fixture.connected.DeviceToken}}})
	if err != nil {
		t.Fatal(err)
	}
	writeRuntimeControlSync(t, connection, instanceID, 1, 3, startedAt)
	readCtx, cancel = context.WithTimeout(t.Context(), time.Second)
	_, _, readErr = connection.Read(readCtx)
	cancel()
	connection.CloseNow()
	if readErr == nil {
		t.Fatal("stopped DaemonInstance resurrected without a new connection_epoch")
	}
	waitRuntimeControlState(t, fixture, instanceID, "stopped", 1, 2)

	connection = openRuntimeControl(t, fixture, instanceID, 2, 1, startedAt)
	waitRuntimeControlState(t, fixture, instanceID, "connected", 2, 1)
	connection.CloseNow()
	waitRuntimeControlState(t, fixture, instanceID, "stopped", 2, 2)
}

func TestRuntimeWorkerControlNewProcessFencesOldConnection(t *testing.T) {
	fixture := newRuntimeControlFixture(t)
	oldID := idgen.New()
	newID := idgen.New()
	oldStartedAt := time.Now().UTC()
	oldConnection := openRuntimeControl(t, fixture, oldID, 1, 1, oldStartedAt)
	defer oldConnection.CloseNow()

	// Deliberately report an older client clock. Process ownership is ordered by
	// the server-side device lock, not by untrusted started_at timestamps.
	newConnection := openRuntimeControl(t, fixture, newID, 1, 1, oldStartedAt.Add(-time.Hour))
	defer newConnection.CloseNow()
	waitRuntimeControlState(t, fixture, oldID, "stopped", 1, 1)
	waitRuntimeControlState(t, fixture, newID, "connected", 1, 1)

	writeRuntimeControlSync(t, oldConnection, oldID, 1, 2, oldStartedAt)
	readCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	_, _, readErr := oldConnection.Read(readCtx)
	cancel()
	if readErr == nil {
		t.Fatal("old daemon connection remained writable after a new process took ownership")
	}
	waitRuntimeControlState(t, fixture, newID, "connected", 1, 1)

	newConnection.CloseNow()
	waitRuntimeControlState(t, fixture, newID, "stopped", 1, 2)
}

func TestRuntimeWorkerControlTreatsDeviceLookupFailureAsTransient(t *testing.T) {
	service := application.New(application.DependenciesFrom(failingDeviceLookupStore{Store: memory.New()}), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	controlURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/runtime/worker/control"

	connection, response, err := websocket.Dial(t.Context(), controlURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer dt_transient"}},
	})
	if connection != nil {
		connection.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("transient device lookup response = %#v err=%v", response, err)
	}
}

func newRuntimeControlFixture(t *testing.T) runtimeControlFixture {
	t.Helper()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(t.Context(), "control@example.com", "long-enough-password", "Control", "Control Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: "douyin"}, "control-project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.Workspace.CreateConnectSession(t.Context(), actor, project.ID, "control-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, application.ConnectDeviceInput{Hostname: "control-mac", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	t.Cleanup(server.Close)
	return runtimeControlFixture{
		service: service, actor: actor, project: project, connected: connected,
		controlURL: "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/runtime/worker/control",
	}
}

func openRuntimeControl(t *testing.T, fixture runtimeControlFixture, instanceID string, epoch, sequence int64, startedAt time.Time) *websocket.Conn {
	t.Helper()
	connection, _, err := websocket.Dial(t.Context(), fixture.controlURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer " + fixture.connected.DeviceToken}}})
	if err != nil {
		t.Fatal(err)
	}
	writeRuntimeControlSync(t, connection, instanceID, epoch, sequence, startedAt)
	for _, wanted := range []string{"control.ready", "runtime.available"} {
		readCtx, cancel := context.WithTimeout(t.Context(), time.Second)
		_, body, readErr := connection.Read(readCtx)
		cancel()
		if readErr != nil {
			connection.CloseNow()
			t.Fatal(readErr)
		}
		var frame struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(body, &frame) != nil || frame.Type != wanted {
			connection.CloseNow()
			t.Fatalf("control frame = %s, want %s", body, wanted)
		}
	}
	return connection
}

func writeRuntimeControlSync(t *testing.T, connection *websocket.Conn, instanceID string, epoch, sequence int64, startedAt time.Time) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"type": "control.sync_state", "daemon_instance_id": instanceID,
		"connection_epoch": epoch, "report_seq": sequence, "pid": 42, "version": "test",
		"state": "connected", "capabilities": map[string]any{"harness_kind": "fake"},
		"active_attempts": []string{}, "started_at": startedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Write(t.Context(), websocket.MessageText, body); err != nil {
		t.Fatal(err)
	}
}

func waitRuntimeControlState(t *testing.T, fixture runtimeControlFixture, instanceID, state string, epoch, sequence int64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		instances, err := fixture.service.Workspace.DaemonInstances(t.Context(), fixture.actor, fixture.connected.Device.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, instance := range instances {
			if instance.ID == instanceID && instance.State == state && instance.ConnectionEpoch == epoch && instance.ReportSequence == sequence {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("DaemonInstance %s did not reach state=%s epoch=%d sequence=%d", instanceID, state, epoch, sequence)
}

func runtimeControlSOP(tenantID string) catalogdomain.SOPVersion {
	return catalogdomain.SOPVersion{ID: "control-sop-v1", TenantID: tenantID, SOPID: "control-sop", Version: 1, SchemaVersion: catalogdomain.SOPSchemaVersion, Name: "Control", Status: "published", DefaultExecutionMode: "agent", Stages: []catalogdomain.StageDefinition{{ID: "execute", Name: "Execute", Order: 10, OutputSchema: "contentcloud.control/1.0", ExecutionModes: []string{"agent"}}}}
}
