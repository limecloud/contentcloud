package localworkspace

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/catalog/environment"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

func TestEnvironmentStateStoresAndVerifiesSignedManifestAndExactLock(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "codex-plugin", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	manifest, verifier, registry, registryVerifier := workspaceEnvironmentFixture(t, now)
	if _, err := StoreEnvironmentRegistry(root, registry, registryVerifier); err != nil {
		t.Fatal(err)
	}
	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.8.0", Digest: "sha256:" + strings.Repeat("a", 64), Installed: true}}
	state, err := StoreEnvironment(root, manifest, installed, verifier, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if state.Health != "ready" || state.Lock.ManifestDigest != manifest.Digest || !EnvironmentCheck(root, verifier, registryVerifier, now.Add(time.Minute)).OK {
		t.Fatalf("environment state = %#v", state)
	}
	loaded, err := LoadEnvironment(root, verifier, now.Add(time.Minute))
	if err != nil || loaded.Manifest.Digest != manifest.Digest || len(loaded.Lock.Plugins) != 1 {
		t.Fatalf("loaded environment = %#v, err = %v", loaded, err)
	}
	if _, err := RequireContentType(root, identitydomain.ContentTypeVideoScript, verifier, now.Add(time.Minute)); err != nil {
		t.Fatalf("default video content type was denied: %v", err)
	}
	if _, err := RequireContentType(root, identitydomain.ContentTypeWeChatArticle, verifier, now.Add(time.Minute)); domainCode(err) != "CONTENT_TYPE_NOT_ENABLED" {
		t.Fatalf("disabled WeChat content type was not denied: %v", err)
	}
	claim, err := ReadEnvironmentClaim(root)
	if err != nil || claim.Health != "unverified_claim" || claim.Manifest.Digest != manifest.Digest || claim.Lock.ManifestDigest != manifest.Digest {
		t.Fatalf("automation environment claim = %#v, err = %v", claim, err)
	}
	manifestInfo, err := os.Stat(filepath.Join(root, ".contentcloud", environmentManifestFile))
	if err != nil || manifestInfo.Mode().Perm() != 0o400 {
		t.Fatalf("manifest mode = %v, err = %v", manifestInfo.Mode().Perm(), err)
	}
}

func TestEnvironmentStateFailsClosedForWrongProjectMissingPluginAndTampering(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "codex-plugin", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	manifest, verifier, _, _ := workspaceEnvironmentFixture(t, now)

	wrongProject := manifest
	wrongProject.ProjectID = "project-2"
	assertEnvironmentCode(t, storeEnvironmentError(root, wrongProject, nil, verifier, now), "ENVIRONMENT_PROJECT_MISMATCH")
	assertEnvironmentCode(t, storeEnvironmentError(root, manifest, nil, verifier, now), "ENVIRONMENT_REQUIRED_PLUGIN_MISSING")

	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.8.0", Digest: "sha256:" + strings.Repeat("a", 64), Installed: true}}
	if _, err := StoreEnvironment(root, manifest, installed, verifier, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".contentcloud", environmentManifestFile)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(body), sourcedomain.KnowledgeExtractCapability, "contentcloud.knowledge.tampered", 1)
	if err := os.WriteFile(path, []byte(tampered), 0o400); err != nil {
		t.Fatal(err)
	}
	assertEnvironmentCode(t, loadEnvironmentError(root, verifier, now.Add(time.Minute)), "ENVIRONMENT_MANIFEST_DIGEST_MISMATCH")
}

func TestEnvironmentLockCompareAndSwapRejectsConcurrentChange(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 45, 0, 0, time.UTC)
	root := t.TempDir()
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "codex-plugin", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	manifest, verifier, registry, registryVerifier := workspaceEnvironmentFixture(t, now)
	if _, err := StoreEnvironmentRegistry(root, registry, registryVerifier); err != nil {
		t.Fatal(err)
	}
	installed := []environment.LockedPlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.8.0", Digest: "sha256:" + strings.Repeat("a", 64), Installed: true}}
	state, err := StoreEnvironment(root, manifest, installed, verifier, now)
	if err != nil {
		t.Fatal(err)
	}
	next := state.Lock
	next.VerifiedAt = now.Add(time.Minute)
	if err := CompareAndSwapEnvironmentLock(root, manifest, state.Lock, next); err != nil {
		t.Fatal(err)
	}
	if err := CompareAndSwapEnvironmentLock(root, manifest, state.Lock, next); err == nil {
		t.Fatal("stale environment.lock compare-and-swap unexpectedly succeeded")
	} else {
		assertEnvironmentCode(t, err, "ENVIRONMENT_LOCK_CHANGED")
	}
}

func workspaceEnvironmentFixture(t *testing.T, now time.Time) (environment.Manifest, *environment.Verifier, environment.Registry, *environment.RegistryVerifier) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := environment.NewIssuer("environment-workspace-test", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	profile := environment.Profile{
		ID: "contentcloud.video-production", Version: "1.0.0", EnvironmentVersion: "2026.7.1", Harness: "codex", Marketplace: "contentcloud",
		Plugins:           []environment.ProfilePlugin{{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.8.0", Required: true, Scope: "environment", Capabilities: []string{sourcedomain.KnowledgeExtractCapability}}},
		WorkspaceTemplate: environment.WorkspaceTemplateRef{ID: "workspace_marketing_video", Version: "2.2.0", Digest: "sha256:" + strings.Repeat("c", 64)}, Capabilities: []string{sourcedomain.KnowledgeExtractCapability}, Policies: environment.Policies{PublishRequiresConfirmation: true},
	}
	registry := environment.Registry{SchemaVersion: "1.0", Entries: []environment.RegistryEntry{{
		ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.8.0", Source: environment.RegistrySource{Repository: "https://github.com/limecloud/contentcloud", Ref: "v0.8.0"}, License: "Apache-2.0", Digest: "sha256:" + strings.Repeat("a", 64),
		Signature: environment.RegistrySignature{Status: "verified", Algorithm: "ed25519", KeyID: "plugin-release-workspace-test"}, CompatibleProfiles: []string{profile.ID}, Permissions: []string{"workspace:read"},
		DataFlow: environment.RegistryDataFlow{LocalByDefault: true, CloudActions: []string{}}, OutputSchemas: []string{"contracts/content-item-3.0.schema.json"},
		Cost:       environment.RegistryCost{Model: "included", Notice: "Included in tests."},
		Evaluation: environment.RegistryEvaluation{Status: "passed", Report: ".agents/plugins/evaluations/test.json", Digest: "sha256:" + strings.Repeat("e", 64), Evidence: []string{"test"}}, Lifecycle: "published", Revocation: environment.RegistryRevocation{Status: "active"},
	}}}
	registryPublicKey, registryPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := environment.RegistryEntrySigningPayload(registry.Entries[0])
	if err != nil {
		t.Fatal(err)
	}
	registry.Entries[0].Signature.Value = base64.StdEncoding.EncodeToString(ed25519.Sign(registryPrivateKey, payload))
	registryVerifier, err := environment.NewRegistryVerifier([]environment.RegistryTrustedKey{{KeyID: "plugin-release-workspace-test", Status: "active", PublicKey: registryPublicKey}})
	if err != nil {
		t.Fatal(err)
	}
	verifiedRegistry, err := registryVerifier.Verify(registry)
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := environment.BuildManifest("project-1", []string{identitydomain.ContentTypeVideoScript}, profile, verifiedRegistry, now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := issuer.Sign(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "environment-workspace-test", Status: "active", PublicKey: publicKey}})
	if err != nil {
		t.Fatal(err)
	}
	return manifest, verifier, registry, registryVerifier
}

func storeEnvironmentError(root string, manifest environment.Manifest, installed []environment.LockedPlugin, verifier *environment.Verifier, now time.Time) error {
	_, err := StoreEnvironment(root, manifest, installed, verifier, now)
	return err
}

func loadEnvironmentError(root string, verifier *environment.Verifier, now time.Time) error {
	_, err := LoadEnvironment(root, verifier, now)
	return err
}

func assertEnvironmentCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainError *fault.Error
	if !errors.As(err, &domainError) || domainError.Code != code {
		t.Fatalf("error = %#v, want %s", err, code)
	}
}
