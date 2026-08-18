package application_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/catalog/environment"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

func TestBrowserBootstrapReturnsProjectBoundSignedEnvironmentManifest(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	issuer, err := environment.NewIssuer("environment-release-test", privateKey)
	must(t, err)
	controlPlane, err := environment.NewControlPlane(issuer, appEnvironmentProfile(), appVerifiedEnvironmentRegistry(t), 24*time.Hour)
	must(t, err)
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithEnvironmentControlPlane(controlPlane), application.WithPlatformAdminEmails("environment@example.com"))
	session, err := service.Identity.Register(t.Context(), "environment@example.com", "long-enough-password", "Environment", "Environment Tenant")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	must(t, err)
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: "douyin"}, "environment-project")
	must(t, err)
	connect, err := service.Workspace.CreateConnectSession(t.Context(), actor, project.ID, "environment-connect")
	must(t, err)

	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, application.ConnectDeviceInput{Hostname: "environment-mac", Platform: "darwin", Arch: "arm64", Version: "test"})
	must(t, err)
	if connected.EnvironmentManifest == nil {
		t.Fatal("browser bootstrap did not return an Environment Manifest")
	}
	if len(connected.EnvironmentManifest.ContentTypes) != 1 || connected.EnvironmentManifest.ContentTypes[0] != identitydomain.ContentTypeVideoScript {
		t.Fatalf("bootstrap manifest must default to video scripts only: %#v", connected.EnvironmentManifest.ContentTypes)
	}
	verifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "environment-release-test", Status: "active", PublicKey: publicKey}})
	must(t, err)
	must(t, verifier.Verify(*connected.EnvironmentManifest, environment.VerifyOptions{ProjectID: project.ID, ProfileID: "contentcloud.video-production", Harness: "codex", Now: time.Now().UTC()}))
	workspaceActor, binding, err := service.Workspace.WorkspaceActor(t.Context(), connected.WorkspaceToken)
	must(t, err)
	_, err = service.Identity.UpdatePlatformTenantContentCapability(t.Context(), actor, actor.TenantID, identitydomain.ContentTypeWeChatArticle, true, "enable-wechat")
	must(t, err)
	refreshed, err := service.Catalog.EnvironmentManifest(t.Context(), workspaceActor, binding)
	must(t, err)
	must(t, verifier.Verify(refreshed, environment.VerifyOptions{ProjectID: project.ID, ProfileID: "contentcloud.video-production", Harness: "codex", Now: time.Now().UTC()}))
	if len(refreshed.ContentTypes) != 2 || refreshed.ContentTypes[0] != identitydomain.ContentTypeVideoScript || refreshed.ContentTypes[1] != identitydomain.ContentTypeWeChatArticle {
		t.Fatalf("refreshed manifest did not project tenant content capabilities: %#v", refreshed.ContentTypes)
	}
	body, err := json.Marshal(connected.EnvironmentManifest)
	must(t, err)
	for _, forbidden := range []string{connected.DeviceToken, connected.WorkspaceToken, "private_key", "model_key"} {
		if forbidden != "" && strings.Contains(string(body), forbidden) {
			t.Fatalf("Environment Manifest leaked forbidden value %q", forbidden)
		}
	}
}

func appEnvironmentProfile() environment.Profile {
	return environment.Profile{
		ID: "contentcloud.video-production", Version: "1.0.0", EnvironmentVersion: "2026.7.1", Harness: "codex", Marketplace: "contentcloud",
		Plugins:           []environment.ProfilePlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.8.0", Required: true, Scope: "environment", Capabilities: []string{sourcedomain.KnowledgeExtractCapability}}},
		WorkspaceTemplate: environment.WorkspaceTemplateRef{ID: "workspace_marketing_video", Version: "2.2.0", Digest: "sha256:" + strings.Repeat("c", 64)},
		Capabilities:      []string{sourcedomain.KnowledgeExtractCapability}, Policies: environment.Policies{PublishRequiresConfirmation: true},
	}
}

func appEnvironmentRegistry() environment.Registry {
	return environment.Registry{SchemaVersion: "1.0", Entries: []environment.RegistryEntry{{
		ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.8.0",
		Source: environment.RegistrySource{Repository: "https://github.com/limecloud/contentcloud", Ref: "v0.8.0"}, License: "Apache-2.0", Digest: "sha256:" + strings.Repeat("a", 64),
		Signature: environment.RegistrySignature{Status: "pending"}, CompatibleProfiles: []string{"contentcloud.video-production"}, Permissions: []string{"workspace:read"},
		DataFlow: environment.RegistryDataFlow{LocalByDefault: true, CloudActions: []string{}}, OutputSchemas: []string{"contracts/content-item-3.0.schema.json"},
		Cost:       environment.RegistryCost{Model: "included", Notice: "Included in tests."},
		Evaluation: environment.RegistryEvaluation{Status: "passed", Report: ".agents/plugins/evaluations/test.json", Digest: "sha256:" + strings.Repeat("e", 64), Evidence: []string{"test"}}, Lifecycle: "published", Revocation: environment.RegistryRevocation{Status: "active"},
	}}}
}

func appVerifiedEnvironmentRegistry(t *testing.T) environment.VerifiedRegistry {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	registry := appEnvironmentRegistry()
	registry.Entries[0].Signature = environment.RegistrySignature{Status: "verified", Algorithm: "ed25519", KeyID: "plugin-release-test"}
	payload, err := environment.RegistryEntrySigningPayload(registry.Entries[0])
	must(t, err)
	registry.Entries[0].Signature.Value = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	verifier, err := environment.NewRegistryVerifier([]environment.RegistryTrustedKey{{KeyID: "plugin-release-test", Status: "active", PublicKey: publicKey}})
	must(t, err)
	verified, err := verifier.Verify(registry)
	must(t, err)
	return verified
}
