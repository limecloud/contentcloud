package cli

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/localconfig"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestDaemonEndToEndWithRealServicePollFixtureReportAndProgress(t *testing.T) {
	ctx := t.Context()
	now := time.Now().UTC()
	store := memory.New()
	service := app.New(store, slog.Default(), app.WithDaemonVersionPolicy("0.9.0", Version, "https://content.example.com/downloads"))
	session, err := service.Register(ctx, "daemon-e2e@example.com", "long-enough-password", "Daemon E2E", "Daemon E2E Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "E2E Brand", ProductName: "E2E Product"}, "e2e-project")
	if err != nil {
		t.Fatal(err)
	}
	source := domain.Source{ID: "source-e2e", TenantID: actor.TenantID, ProjectID: project.ID, Name: "Product Facts", SourceType: "product_manual", Status: "ready", RevisionCount: 1, LatestRevision: "revision-e2e", CreatedAt: now}
	revision := domain.SourceRevision{ID: "revision-e2e", TenantID: actor.TenantID, ProjectID: project.ID, SourceID: source.ID, FileName: "facts.txt", SHA256: strings.Repeat("a", 64), ByteSize: 16, DeclaredMIME: "text/plain", DetectedMIME: "text/plain", ProcessingStatus: "ready", CreatedAt: now}
	if err := store.CreateSource(ctx, source, revision); err != nil {
		t.Fatal(err)
	}
	evidence := domain.EvidenceSpan{ID: "evidence-e2e", TenantID: actor.TenantID, ProjectID: project.ID, RevisionID: revision.ID, LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: "E2E verified product fact", QuoteHash: "sha256:" + strings.Repeat("b", 64), ReviewStatus: "accepted", ReviewedBy: actor.UserID, ReviewedAt: &now, CreatedAt: now}
	if err := store.CreateEvidence(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	run, err := service.CreateKnowledgeExtractionRun(ctx, actor, app.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{revision.ID}, IdempotencyKey: "daemon-e2e", OutputCount: 1}, "e2e-run")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.CreateConnectSession(ctx, actor, project.ID, "e2e-connect")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(ctx, service, actor, connect, app.ConnectDeviceInput{Hostname: "daemon-e2e.local", Platform: "darwin", Arch: "arm64", Version: Version, Capabilities: builtinCapabilities()})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()

	stateDir := t.TempDir()
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("CONTENTCLOUD_DAEMON_STATE_DIR", stateDir)
	t.Setenv("CONTENTCLOUD_AUTOMATION_ROOT", filepath.Join(t.TempDir(), "automation"))
	t.Setenv("CONTENTCLOUD_DEVICE_TOKEN", connected.DeviceToken)
	if err := localconfig.Save(localconfig.Config{
		ServerURL: server.URL, DeviceID: connected.Device.ID, WorkspaceID: connected.WorkspaceID, ProjectID: project.ID,
		DaemonBindings: []localconfig.DaemonBinding{{ServerURL: server.URL, DeviceID: connected.Device.ID, Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: connected.WorkspaceID, ProjectID: project.ID}}}},
	}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	runtime := &Root{stdout: &stdout, stderr: &stderr}
	command := runtime.command()
	command.SetArgs([]string{"--json", "daemon", "run", "--once", "--fixture"})
	if err := command.Execute(); err != nil {
		t.Fatalf("daemon end-to-end execution failed: %v; stderr=%s", err, stderr.String())
	}

	storedRun, err := service.Run(ctx, actor, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	attempts, err := service.RunAttempts(ctx, actor, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.Knowledge(ctx, actor, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := service.RunProgress(ctx, actor, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	pending, dead, err := daemonJournalCounts()
	if err != nil {
		t.Fatal(err)
	}
	if storedRun.State != "succeeded" || len(attempts) != 1 || attempts[0].State != "succeeded" || len(items) != 1 || len(events) != 1 || events[0].Phase != "succeeded" || pending != 0 || dead != 0 {
		t.Fatalf("incomplete daemon end-to-end result: run=%#v attempts=%#v items=%#v events=%#v pending=%d dead=%d", storedRun, attempts, items, events, pending, dead)
	}
	runtimeState, err := loadDaemonRuntimeState()
	if err != nil || runtimeState.BindingCount != 1 || runtimeState.WorkspaceCount != 1 || runtimeState.ActiveTasks != 0 || runtimeState.RuntimePolicy.UpdateRequired {
		t.Fatalf("unexpected daemon runtime state: %#v err=%v", runtimeState, err)
	}
}
