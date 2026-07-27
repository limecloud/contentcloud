package app_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestArtifactEnvelopePresentationAndDeclarativeLocalOpen(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	service := app.New(store, slog.Default())
	session, err := service.Register(ctx, "artifact@example.com", "long-enough-password", "Owner", "Artifact Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	must(t, err)
	capability := domain.Capability{ID: domain.ArtifactExportCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.ScriptPackageSchema, OutputSchema: "extension-artifact-envelope/1.0", PresentationProfiles: []string{"local_open"}, LocalOnly: true, Digest: "contentcloud-artifact-export@test"}
	connect, err := service.CreateConnectSession(ctx, actor, project.ID, "")
	must(t, err)
	connected, err := testsupport.ConnectBootstrap(ctx, service, actor, connect, app.ConnectDeviceInput{Hostname: "local", Platform: "darwin", Arch: "arm64", Version: "test", Capabilities: []domain.Capability{capability}})
	must(t, err)
	deviceActor, device, err := service.DeviceActor(ctx, connected.DeviceToken)
	must(t, err)
	now := time.Now().UTC()
	logical := domain.Script{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, Title: "Script", CreatedAt: now}
	script, err := store.CreateScript(ctx, logical, domain.ScriptVersion{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, ScriptID: logical.ID, Status: "review_ready", ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Package: domain.ScriptPackage{SchemaVersion: "1.1"}, Validation: domain.ValidationReport{Valid: true}, CreatedAt: now})
	must(t, err)
	envelope := domain.ExtensionArtifactEnvelopeV1{EnvelopeVersion: "1.0", ProjectID: project.ID, ScriptVersionID: script.ID, Capability: domain.ArtifactCapabilityRef{ID: capability.ID, Version: capability.Version, Digest: capability.Digest}, SchemaID: "moyin.timeline/1.0", MediaType: "text/html", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Size: 128, Renditions: []domain.ArtifactRenditionRef{}, Metadata: map[string]any{"variant": "A"}}
	registered, err := service.RegisterArtifact(ctx, deviceActor, device, app.RegisterArtifactInput{Envelope: envelope, FileName: "preview.html", Capabilities: []domain.Capability{capability}}, "")
	must(t, err)
	presentation, err := service.ArtifactPresentation(ctx, actor, registered.Artifact.ID)
	must(t, err)
	if presentation.Tier != "local_open" || presentation.SourceDevice == nil || !presentation.SourceDevice.Online {
		t.Fatalf("active HTML must only be locally opened, got %#v", presentation)
	}
	openResult, err := service.CreateArtifactOpenRequest(ctx, actor, registered.Artifact.ID, device.ID, false, "")
	must(t, err)
	lease, err := service.PollArtifactOpen(ctx, deviceActor, device, []domain.Capability{capability})
	must(t, err)
	if lease.OpenRequestID != openResult.Request.ID || lease.ArtifactID != registered.Artifact.ID {
		t.Fatalf("daemon lease must only contain stable IDs: %#v", lease)
	}
	_, err = service.FinishArtifactOpen(ctx, deviceActor, device, lease.OpenRequestID, "accepted", "")
	must(t, err)
	finished, err := service.FinishArtifactOpen(ctx, deviceActor, device, lease.OpenRequestID, "opened", "")
	must(t, err)
	if finished.State != "opened" || finished.CompletedAt == nil {
		t.Fatalf("open request did not reach terminal state: %#v", finished)
	}
	if _, err := service.FinishArtifactOpen(ctx, deviceActor, device, lease.OpenRequestID, "failed", "/private/local/path"); err == nil {
		t.Fatal("daemon must not report arbitrary local details")
	}

	device.LastSeenAt = time.Now().Add(-2 * time.Minute)
	must(t, store.SaveDevice(ctx, device))
	presentation, err = service.ArtifactPresentation(ctx, actor, registered.Artifact.ID)
	must(t, err)
	if presentation.Tier != "metadata_only" || presentation.SourceDevice == nil || presentation.SourceDevice.Online {
		t.Fatalf("offline device must deterministically downgrade presentation: %#v", presentation)
	}
}

func TestArtifactRegistrationRejectsCapabilityMismatch(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	service := app.New(store, slog.Default())
	session, _ := service.Register(ctx, "mismatch@example.com", "long-enough-password", "Owner", "Mismatch Tenant")
	actor, _, _ := service.SessionActor(ctx, session.ID)
	project, _ := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	connect, _ := service.CreateConnectSession(ctx, actor, project.ID, "")
	capability := domain.Capability{ID: domain.ArtifactExportCapability, Version: "1.0.0", Digest: "trusted", PresentationProfiles: []string{"local_open"}}
	connected, _ := testsupport.ConnectBootstrap(ctx, service, actor, connect, app.ConnectDeviceInput{Hostname: "local", Capabilities: []domain.Capability{capability}})
	deviceActor, device, _ := service.DeviceActor(ctx, connected.DeviceToken)
	now := time.Now().UTC()
	logical := domain.Script{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, Title: "Script", CreatedAt: now}
	script, _ := store.CreateScript(ctx, logical, domain.ScriptVersion{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, ScriptID: logical.ID, Package: domain.ScriptPackage{SchemaVersion: "1.1"}, CreatedAt: now})
	envelope := domain.ExtensionArtifactEnvelopeV1{EnvelopeVersion: "1.0", ProjectID: project.ID, ScriptVersionID: script.ID, Capability: domain.ArtifactCapabilityRef{ID: capability.ID, Version: capability.Version, Digest: "tampered"}, SchemaID: "opaque/1.0", MediaType: "application/octet-stream", SHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Size: 10, Renditions: []domain.ArtifactRenditionRef{}, Metadata: map[string]any{}}
	_, err := service.RegisterArtifact(ctx, deviceActor, device, app.RegisterArtifactInput{Envelope: envelope}, "")
	assertDomainCode(t, err, "ARTIFACT_CAPABILITY_MISMATCH")
}
