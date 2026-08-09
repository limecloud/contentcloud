package app_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestOperationsExecutorsProjectDeviceFactsAndHealth(t *testing.T) {
	store := memory.New()
	service := app.New(store, slog.Default())
	session, err := service.Register(t.Context(), "operations-executor@example.com", "long-enough-password", "运营人员", "执行端租户")
	must(t, err)
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	must(t, err)
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "果木食品", ProductName: "品牌短片"}, "executor-project")
	must(t, err)
	connect, err := service.CreateConnectSession(t.Context(), actor, project.ID, "executor-connect")
	must(t, err)
	capability := domain.Capability{ID: "storyboard_generation", Version: "1.0.0", Kind: "business_capability", InputSchema: "contentcloud.storyboard-input/1.0", OutputSchema: "contentcloud.storyboard/1.0", LocalOnly: true, Digest: "sha256:executor"}
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, app.ConnectDeviceInput{DisplayName: "分镜工作站", Hostname: "storyboard.local", Platform: "darwin", Arch: "arm64", Version: "0.21.0", Capabilities: []domain.Capability{capability}})
	must(t, err)

	directory, err := service.OperationsExecutors(t.Context(), actor)
	must(t, err)
	if len(directory.Executors) != 1 || directory.OnlineWindowSeconds != 120 {
		t.Fatalf("unexpected executor directory: %#v", directory)
	}
	executor := directory.Executors[0]
	if executor.ID != connected.Device.ID || executor.ExecutorType != "contentcloud_device" || executor.Status != "online" || executor.StatusReason != "heartbeat_recent" {
		t.Fatalf("executor identity or health was not projected from the device: %#v", executor)
	}
	if executor.Hostname != "storyboard.local" || executor.Version != "0.21.0" || len(executor.Capabilities) != 1 || executor.Capabilities[0].Digest != capability.Digest {
		t.Fatalf("executor runtime facts are incomplete: %#v", executor)
	}
	if len(executor.Projects) != 1 || executor.Projects[0].ID != project.ID || executor.Projects[0].BrandName != "果木食品" || executor.Projects[0].ProductName != "品牌短片" {
		t.Fatalf("executor project scope is incomplete: %#v", executor.Projects)
	}

	stale := connected.Device
	stale.LastSeenAt = time.Now().UTC().Add(-3 * time.Minute)
	must(t, store.SaveDevice(t.Context(), stale))
	executor, err = service.OperationsExecutor(t.Context(), actor, stale.ID)
	must(t, err)
	if executor.Status != "offline" || executor.StatusReason != "heartbeat_stale" {
		t.Fatalf("stale heartbeat must project as offline: %#v", executor)
	}

	_, err = service.RevokeDevice(t.Context(), actor, stale.ID, "executor-revoke")
	must(t, err)
	executor, err = service.OperationsExecutor(t.Context(), actor, stale.ID)
	must(t, err)
	if executor.Status != "revoked" || executor.StatusReason != "registration_revoked" || executor.RevokedAt == nil {
		t.Fatalf("revoked registration must override heartbeat health: %#v", executor)
	}
}
