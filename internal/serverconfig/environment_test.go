package serverconfig_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/capabilitycatalog"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/serverconfig"
)

func TestLoadEnvironmentBuildsVerifiedControlPlaneAndAutomationPolicy(t *testing.T) {
	config := environmentConfigFixture(t)
	runtime, err := serverconfig.LoadEnvironment(config)
	if err != nil {
		t.Fatal(err)
	}
	if !runtime.Enabled || runtime.ControlPlane == nil || len(runtime.AutomationRequirements) != 1 {
		t.Fatalf("environment runtime = %#v", runtime)
	}
	expected, _ := capabilitycatalog.Exact(domain.KnowledgeExtractCapability, "0.8.0")
	if runtime.AutomationRequirements[0].Digest != expected.Digest || len(runtime.AutomationPackIDs[expected.ID]) != 1 {
		t.Fatalf("automation policy did not use canonical capability catalog: %#v", runtime)
	}
	manifest, err := runtime.ControlPlane.Issue("project-1", []string{domain.ContentTypeVideoScript}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ProjectID != "project-1" || manifest.Signature.KeyID != "environment-config-test" {
		t.Fatalf("issued manifest = %#v", manifest)
	}
}

func TestLoadEnvironmentFailsClosedForPartialOrUnsafeConfiguration(t *testing.T) {
	if _, err := serverconfig.LoadEnvironment(serverconfig.EnvironmentConfig{ProfilePath: "profile.json"}); err == nil {
		t.Fatal("partial configuration was accepted")
	}

	config := environmentConfigFixture(t)
	insideKey := filepath.Join(config.RepositoryRoot, "environment.key")
	privateKey, err := os.ReadFile(config.SigningKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(insideKey, privateKey, 0o600); err != nil {
		t.Fatal(err)
	}
	config.SigningKeyPath = insideKey
	if _, err := serverconfig.LoadEnvironment(config); err == nil || !strings.Contains(err.Error(), "仓库之外") {
		t.Fatalf("repository key failure = %v", err)
	}

	config = environmentConfigFixture(t)
	if err := os.Chmod(config.SigningKeyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := serverconfig.LoadEnvironment(config); err == nil || !strings.Contains(err.Error(), "用户组或其他用户") {
		t.Fatalf("permissive key failure = %v", err)
	}
}

func TestLoadEnvironmentRequiresRegisteredSignerAndSignedRegistry(t *testing.T) {
	config := environmentConfigFixture(t)
	_, unrelatedPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.SigningKeyPath, unrelatedPrivateKey, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := serverconfig.LoadEnvironment(config); err == nil {
		t.Fatal("signer missing from Environment trust store was accepted")
	}

	config = environmentConfigFixture(t)
	var registry environment.Registry
	readJSON(t, config.RegistryPath, &registry)
	registry.Entries[0].Digest = "sha256:" + strings.Repeat("9", 64)
	writeJSON(t, config.RegistryPath, registry, 0o600)
	if _, err := serverconfig.LoadEnvironment(config); err == nil {
		t.Fatal("tampered signed Registry was accepted")
	}
}

func environmentConfigFixture(t *testing.T) serverconfig.EnvironmentConfig {
	t.Helper()
	root := t.TempDir()
	repositoryRoot := filepath.Join(root, "repo")
	secretsRoot := filepath.Join(root, "secrets")
	if err := os.MkdirAll(repositoryRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secretsRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	registryPublicKey, registryPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	environmentPublicKey, environmentPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile := environment.Profile{
		ID: "contentcloud.video-production", Version: "1.0.0", EnvironmentVersion: "2026.7.1", Harness: "codex", Marketplace: "contentcloud",
		Plugins: []environment.ProfilePlugin{
			{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.8.0", Required: true, Scope: "environment", Capabilities: []string{domain.KnowledgeExtractCapability}},
			{ID: "contentcloud-evidence-reasoning", Kind: "skill_pack", Version: "1.0.0", Required: true, Scope: "task", Capabilities: []string{domain.KnowledgeExtractCapability}},
		},
		WorkspaceTemplate: environment.WorkspaceTemplateRef{ID: "workspace_marketing_video", Version: "2.2.0", Digest: "sha256:" + strings.Repeat("c", 64)},
		Capabilities:      []string{domain.KnowledgeExtractCapability},
		Policies:          environment.Policies{PublishRequiresConfirmation: true, AutomationEnabled: true},
	}
	registry := environment.Registry{SchemaURL: "test", SchemaVersion: "1.0", Entries: []environment.RegistryEntry{
		configRegistryEntry("contentcloud-video-production", "scene_plugin", "0.8.0", "v0.8.0", "a"),
		configRegistryEntry("contentcloud-evidence-reasoning", "skill_pack", "1.0.0", "v1.0.0", "b"),
	}}
	for index := range registry.Entries {
		registry.Entries[index].Signature = environment.RegistrySignature{Status: "verified", Algorithm: "ed25519", KeyID: "plugin-release-config-test"}
		payload, err := environment.RegistryEntrySigningPayload(registry.Entries[index])
		if err != nil {
			t.Fatal(err)
		}
		registry.Entries[index].Signature.Value = base64.StdEncoding.EncodeToString(ed25519.Sign(registryPrivateKey, payload))
	}

	profilePath := filepath.Join(repositoryRoot, "profile.json")
	registryPath := filepath.Join(repositoryRoot, "registry.json")
	registryTrustPath := filepath.Join(repositoryRoot, "plugin-trust.json")
	environmentTrustPath := filepath.Join(repositoryRoot, "environment-trust.json")
	signingKeyPath := filepath.Join(secretsRoot, "environment.key")
	writeJSON(t, profilePath, profile, 0o600)
	writeJSON(t, registryPath, registry, 0o600)
	writeTrust(t, registryTrustPath, "plugin-release-config-test", registryPublicKey)
	writeTrust(t, environmentTrustPath, "environment-config-test", environmentPublicKey)
	if err := os.WriteFile(signingKeyPath, environmentPrivateKey, 0o600); err != nil {
		t.Fatal(err)
	}
	return serverconfig.EnvironmentConfig{
		ProfilePath: profilePath, RegistryPath: registryPath, RegistryTrustPath: registryTrustPath, EnvironmentTrustPath: environmentTrustPath,
		SigningKeyPath: signingKeyPath, SigningKeyID: "environment-config-test", CapabilityReleaseVersion: "0.8.0", ManifestTTL: 24 * time.Hour, RepositoryRoot: repositoryRoot,
	}
}

func configRegistryEntry(id, kind, version, ref, digestByte string) environment.RegistryEntry {
	return environment.RegistryEntry{
		ID: id, Kind: kind, Version: version, Source: environment.RegistrySource{Repository: "https://github.com/limecloud/contentcloud", Ref: ref}, License: "Apache-2.0", Digest: "sha256:" + strings.Repeat(digestByte, 64),
		CompatibleProfiles: []string{"contentcloud.video-production"}, Permissions: []string{"workspace:read"}, DataFlow: environment.RegistryDataFlow{LocalByDefault: true, CloudActions: []string{}},
		Cost:          environment.RegistryCost{Model: "included", Notice: "Included in tests."},
		OutputSchemas: []string{"contracts/knowledge-candidates-1.0.schema.json"}, Evaluation: environment.RegistryEvaluation{Status: "passed", Report: "evaluation.json", Digest: "sha256:" + strings.Repeat("e", 64), Evidence: []string{"test"}},
		Lifecycle: "published", Revocation: environment.RegistryRevocation{Status: "active"},
	}
}

func writeTrust(t *testing.T, path, keyID string, publicKey ed25519.PublicKey) {
	t.Helper()
	writeJSON(t, path, map[string]any{
		"$schema": "test", "schema_version": "1.0", "keys": []map[string]any{{"key_id": keyID, "algorithm": "ed25519", "status": "active", "public_key": base64.StdEncoding.EncodeToString(publicKey)}},
	}, 0o600)
}

func writeJSON(t *testing.T, path string, value any, mode os.FileMode) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatal(err)
	}
}
