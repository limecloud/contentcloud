package application_test

import (
	"log/slog"
	"testing"
	"time"

	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/testsupport"

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestOperationsExecutorsProjectDeviceFactsAndHealth(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	session, err := service.Identity.Register(t.Context(), "operations-executor@example.com", "long-enough-password", "运营人员", "执行端租户")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	must(t, err)
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "果木食品", ProductName: "品牌短片"}, "executor-project")
	must(t, err)
	connect, err := service.Workspace.CreateConnectSession(t.Context(), actor, project.ID, "executor-connect")
	must(t, err)
	capability := catalogdomain.Capability{ID: "storyboard_generation", Version: "1.0.0", Kind: "business_capability", InputSchema: "contentcloud.storyboard-input/1.0", OutputSchema: "contentcloud.storyboard/1.0", LocalOnly: true, Digest: "sha256:executor"}
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, application.ConnectDeviceInput{DisplayName: "分镜工作站", Hostname: "storyboard.local", Platform: "darwin", Arch: "arm64", Version: "0.21.0", Capabilities: []catalogdomain.Capability{capability}})
	must(t, err)
	now := time.Now().UTC()
	instance := workspacedomain.DaemonInstance{
		ID: idgen.New(), TenantID: actor.TenantID, DeviceID: connected.Device.ID,
		ConnectionEpoch: 1, ReportSequence: 1, Version: "0.21.0", State: "connected",
		Capabilities: map[string]any{
			"environment_status": "repair_required", "environment_reason": "plugin_drift", "runtime_status": "throttled", "runtime_reason": "capacity_limit", "harness_kind": "codex",
			"workspace_observations": []workspacedomain.DaemonWorkspaceObservation{{
				WorkspaceID: "workspace-1", ProjectID: project.ID, Status: "repair_required", Reason: "plugin_drift", Generation: "sha256:generation",
				EnvironmentDeclaration: "sha256:environment", PluginDeclaration: "sha256:plugin", SkillDeclaration: "sha256:skill",
				MCPDeclaration: "sha256:mcp", WorkspaceDeclaration: "sha256:workspace", PluginHostReceiptDigest: "sha256:receipt",
				ObservedSkillDigest: "sha256:observed-skill", ObservedMCPDigest: "sha256:observed-mcp", ObservedWorkspaceDigest: "sha256:observed-workspace", ObservedAt: now,
			}},
			"runtimes": []agentadapter.HarnessProbe{
				{Kind: "codex", Version: "codex 1.2.3", Status: "healthy", Capabilities: agentadapter.HarnessCapabilities{Kind: "codex", Version: "codex 1.2.3", Events: true, Resume: true}},
				{Kind: "claude", Status: "unhealthy", ErrorCode: "CLAUDE_AUTH_REQUIRED"},
			},
		},
		ActiveAttempts: []string{"attempt-2", "attempt-1"}, StartedAt: now.Add(-time.Minute), LastSeenAt: now,
	}
	must(t, store.SaveDaemonInstance(t.Context(), instance))

	directory, err := service.Operations.OperationsExecutors(t.Context(), actor)
	must(t, err)
	if len(directory.Executors) != 1 || directory.OnlineWindowSeconds != 45 {
		t.Fatalf("unexpected executor directory: %#v", directory)
	}
	executor := directory.Executors[0]
	if executor.ID != connected.Device.ID || executor.ExecutorType != "contentcloud_device" || executor.Status != "online" || executor.PresenceStatus != "online" {
		t.Fatalf("executor identity or health was not projected from the device: %#v", executor)
	}
	if executor.EnvironmentStatus != "repair_required" || executor.EnvironmentReason != "plugin_drift" || executor.RuntimeStatus != "throttled" || executor.RuntimeReason != "capacity_limit" {
		t.Fatalf("executor health axes were not projected from the daemon instance: %#v", executor)
	}
	if executor.DaemonInstanceID != instance.ID || executor.ConnectionEpoch != 1 || len(executor.ActiveAttemptIDs) != 2 || executor.ActiveAttemptIDs[0] != "attempt-1" {
		t.Fatalf("executor current-state facts are incomplete: %#v", executor)
	}
	if len(executor.Runtimes) != 2 || !executor.Runtimes[0].Selected || executor.Runtimes[0].Kind != "codex" || executor.Runtimes[0].Version != "codex 1.2.3" || executor.Runtimes[1].ErrorCode != "CLAUDE_AUTH_REQUIRED" {
		t.Fatalf("executor Runtime inventory is incomplete: %#v", executor.Runtimes)
	}
	if len(executor.Workspaces) != 1 || executor.Workspaces[0].WorkspaceID != "workspace-1" || executor.Workspaces[0].ProjectID != project.ID || executor.Workspaces[0].Generation != "sha256:generation" || executor.Workspaces[0].PluginReceiptDigest != "sha256:receipt" {
		t.Fatalf("executor Workspace inventory is incomplete: %#v", executor.Workspaces)
	}
	if executor.Hostname != "storyboard.local" || executor.Version != "0.21.0" || len(executor.Capabilities) != 1 || executor.Capabilities[0].Digest != capability.Digest {
		t.Fatalf("executor runtime facts are incomplete: %#v", executor)
	}
	if len(executor.Projects) != 1 || executor.Projects[0].ID != project.ID || executor.Projects[0].BrandName != "果木食品" || executor.Projects[0].ProductName != "品牌短片" {
		t.Fatalf("executor project scope is incomplete: %#v", executor.Projects)
	}

	instance.LastSeenAt = time.Now().UTC().Add(-time.Minute)
	instance.ReportSequence++
	must(t, store.SaveDaemonInstance(t.Context(), instance))
	executor, err = service.Operations.OperationsExecutor(t.Context(), actor, connected.Device.ID)
	must(t, err)
	if executor.Status != "offline" || executor.PresenceReason != "instance_stale" || executor.EnvironmentStatus != "unknown" || executor.RuntimeStatus != "unavailable" {
		t.Fatalf("stale heartbeat must project as offline: %#v", executor)
	}

	_, err = service.Workspace.RevokeDevice(t.Context(), actor, connected.Device.ID, "executor-revoke")
	must(t, err)
	executor, err = service.Operations.OperationsExecutor(t.Context(), actor, connected.Device.ID)
	must(t, err)
	if executor.Status != "revoked" || executor.StatusReason != "registration_revoked" || executor.PresenceStatus != "offline" || executor.RuntimeStatus != "unavailable" || executor.RevokedAt == nil {
		t.Fatalf("revoked registration must override heartbeat health: %#v", executor)
	}
}

func TestOperationsExecutorWithoutDaemonInstanceIsUnknown(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	session, err := service.Identity.Register(t.Context(), "unknown-executor@example.com", "long-enough-password", "运营人员", "执行端租户")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	must(t, err)
	device := workspacedomain.Device{ID: idgen.New(), TenantID: actor.TenantID, OwnerUserID: actor.UserID, DisplayName: "未上报实例", LastSeenAt: time.Now().UTC()}
	must(t, store.SaveDevice(t.Context(), device))

	executor, err := service.Operations.OperationsExecutor(t.Context(), actor, device.ID)
	must(t, err)
	if executor.PresenceStatus != "unknown" || executor.EnvironmentStatus != "unknown" || executor.RuntimeStatus != "unknown" {
		t.Fatalf("missing daemon current-state must remain unknown: %#v", executor)
	}
}

func TestOperationsExecutorsChooseLatestDaemonProcessAcrossInstanceEpochs(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	session, err := service.Identity.Register(t.Context(), "latest-executor@example.com", "long-enough-password", "运营人员", "执行端租户")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	must(t, err)
	device := workspacedomain.Device{ID: idgen.New(), TenantID: actor.TenantID, OwnerUserID: actor.UserID, DisplayName: "多进程执行端", LastSeenAt: time.Now().UTC()}
	must(t, store.SaveDevice(t.Context(), device))
	base := time.Now().UTC()
	must(t, store.SaveDaemonInstance(t.Context(), workspacedomain.DaemonInstance{
		ID: idgen.New(), TenantID: actor.TenantID, DeviceID: device.ID, ConnectionEpoch: 9, ReportSequence: 20,
		Version: "old", State: "connected", StartedAt: base.Add(-time.Minute), LastSeenAt: base.Add(-time.Second),
	}))
	newID := idgen.New()
	must(t, store.SaveDaemonInstance(t.Context(), workspacedomain.DaemonInstance{
		ID: newID, TenantID: actor.TenantID, DeviceID: device.ID, ConnectionEpoch: 1, ReportSequence: 1,
		Version: "new", State: "connected", StartedAt: base, LastSeenAt: base,
	}))
	executor, err := service.Operations.OperationsExecutor(t.Context(), actor, device.ID)
	must(t, err)
	if executor.DaemonInstanceID != newID {
		t.Fatalf("latest daemon process was not selected across instance epochs: %#v", executor)
	}
}

func TestOperationsExecutorsPreferLiveProcessOverLaterStoppedReport(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	session, err := service.Identity.Register(t.Context(), "live-executor@example.com", "long-enough-password", "运营人员", "执行端租户")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	must(t, err)
	device := workspacedomain.Device{ID: idgen.New(), TenantID: actor.TenantID, OwnerUserID: actor.UserID, DisplayName: "重连执行端", LastSeenAt: time.Now().UTC()}
	must(t, store.SaveDevice(t.Context(), device))
	base := time.Now().UTC()
	stoppedAt := base
	must(t, store.SaveDaemonInstance(t.Context(), workspacedomain.DaemonInstance{
		ID: idgen.New(), TenantID: actor.TenantID, DeviceID: device.ID, ConnectionEpoch: 4, ReportSequence: 10,
		Version: "old", State: "stopped", StartedAt: base.Add(-time.Minute), LastSeenAt: base, StoppedAt: &stoppedAt,
	}))
	liveID := idgen.New()
	must(t, store.SaveDaemonInstance(t.Context(), workspacedomain.DaemonInstance{
		ID: liveID, TenantID: actor.TenantID, DeviceID: device.ID, ConnectionEpoch: 1, ReportSequence: 1,
		Version: "new", State: "connected", StartedAt: base.Add(-time.Second), LastSeenAt: base.Add(-time.Second),
	}))
	executor, err := service.Operations.OperationsExecutor(t.Context(), actor, device.ID)
	must(t, err)
	if executor.DaemonInstanceID != liveID || executor.PresenceStatus != "online" {
		t.Fatalf("later stopped report hid a live replacement process: %#v", executor)
	}
}
