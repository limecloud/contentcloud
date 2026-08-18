package application_test

import (
	"log/slog"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestConnectSessionCompletesOnlyAfterWorkspaceRegistration(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(t.Context(), "connect@example.com", "long-enough-password", "Connect User", "Connect Tenant")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	must(t, err)
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: "douyin"}, "connect-project")
	must(t, err)
	connect, err := service.Workspace.CreateConnectSession(t.Context(), actor, project.ID, "connect-session")
	must(t, err)

	device, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, application.ConnectDeviceInput{Hostname: "connect-mac", Platform: "darwin", Arch: "arm64", Version: "test"})
	must(t, err)
	status, err := service.Workspace.ConnectSession(t.Context(), actor, connect.ID)
	must(t, err)
	if status.State != "verifying" {
		t.Fatalf("state after device connection = %q, want verifying", status.State)
	}

	workspaceActor, binding, err := service.Workspace.WorkspaceActor(t.Context(), device.WorkspaceToken)
	must(t, err)
	registered, err := service.Review.RegisterWorkspace(t.Context(), workspaceActor, binding, "workspace_marketing_video", "2.0.0", []string{"codex-plugin"}, "workspace-register")
	must(t, err)
	if len(registered.Targets) != 1 || registered.Targets[0] != "codex" {
		t.Fatalf("registered targets = %#v, want normalized codex target", registered.Targets)
	}
	status, err = service.Workspace.ConnectSession(t.Context(), actor, connect.ID)
	must(t, err)
	if status.State != "connected" {
		t.Fatalf("state after workspace registration = %q, want connected", status.State)
	}
}
