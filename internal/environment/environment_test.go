package environment_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
)

func TestManifestSignatureBindsPayloadProjectExpiryAndTrust(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	issuer, err := environment.NewIssuer("environment-release-2026", privateKey)
	must(t, err)
	verifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "environment-release-2026", Status: "active", PublicKey: publicKey}})
	must(t, err)

	manifest := signedManifest(t, issuer, now)
	must(t, verifier.Verify(manifest, environment.VerifyOptions{ProjectID: "project-1", ProfileID: "contentcloud.video-production", Harness: "codex", Now: now.Add(time.Minute)}))

	tampered := manifest
	tampered.Capabilities = append([]string(nil), manifest.Capabilities...)
	tampered.Capabilities[0] = "contentcloud.knowledge.tampered"
	assertCode(t, verifier.Verify(tampered, environment.VerifyOptions{ProjectID: "project-1", Harness: "codex", Now: now}), "ENVIRONMENT_MANIFEST_DIGEST_MISMATCH")
	assertCode(t, verifier.Verify(manifest, environment.VerifyOptions{ProjectID: "project-2", Harness: "codex", Now: now}), "ENVIRONMENT_PROJECT_MISMATCH")
	assertCode(t, verifier.Verify(manifest, environment.VerifyOptions{ProjectID: "project-1", Harness: "codex", Now: manifest.ExpiresAt}), "ENVIRONMENT_MANIFEST_EXPIRED")

	revokedVerifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "environment-release-2026", Status: "revoked", PublicKey: publicKey}})
	must(t, err)
	assertCode(t, revokedVerifier.Verify(manifest, environment.VerifyOptions{ProjectID: "project-1", Harness: "codex", Now: now}), "ENVIRONMENT_SIGNING_KEY_UNTRUSTED")
}

func TestBuildManifestUsesOnlyExactPublishedCompatibleRegistryEntries(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	profile, rawRegistry := fixtureProfileAndRegistry()
	signedRegistry, registry, registryVerifier := signAndVerifyRegistry(t, rawRegistry)
	manifest, err := environment.BuildManifest("project-1", profile, registry, now, now.Add(24*time.Hour))
	must(t, err)
	if len(manifest.Distribution.Plugins) != 2 || manifest.Distribution.Plugins[0].ID != "contentcloud-video-production" || manifest.Distribution.Plugins[1].ID != "contentcloud-visual-storytelling" {
		t.Fatalf("resolved plugins = %#v", manifest.Distribution.Plugins)
	}
	if manifest.Distribution.Plugins[0].SourceRef != "v0.8.0" || manifest.Distribution.Plugins[1].SourceRef != "v1.2.0" {
		t.Fatalf("registry refs were not preserved: %#v", manifest.Distribution.Plugins)
	}

	tampered := signedRegistry
	tampered.Entries = append([]environment.RegistryEntry(nil), signedRegistry.Entries...)
	tampered.Entries[0].Lifecycle = "evaluated"
	_, err = registryVerifier.Verify(tampered)
	assertCode(t, err, "REGISTRY_SIGNATURE_MISMATCH")

	unpublished := rawRegistry
	unpublished.Entries = append([]environment.RegistryEntry(nil), rawRegistry.Entries...)
	unpublished.Entries[0].Lifecycle = "evaluated"
	_, verifiedUnpublished, _ := signAndVerifyRegistry(t, unpublished)
	assertCode(t, buildManifestError(profile, verifiedUnpublished, now), "REGISTRY_ENTRY_NOT_PUBLISHED")

	incompatible := rawRegistry
	incompatible.Entries = append([]environment.RegistryEntry(nil), rawRegistry.Entries...)
	incompatible.Entries[1].CompatibleProfiles = []string{"contentcloud.other"}
	_, verifiedIncompatible, _ := signAndVerifyRegistry(t, incompatible)
	assertCode(t, buildManifestError(profile, verifiedIncompatible, now), "ENVIRONMENT_PROFILE_REGISTRY_MISMATCH")

	revoked := rawRegistry
	revoked.Entries = append([]environment.RegistryEntry(nil), rawRegistry.Entries...)
	revoked.Entries[1].Lifecycle = "revoked"
	revoked.Entries[1].Revocation = environment.RegistryRevocation{Status: "revoked", Severity: "high", Reason: "supply-chain incident"}
	_, verifiedRevoked, _ := signAndVerifyRegistry(t, revoked)
	assertCode(t, buildManifestError(profile, verifiedRevoked, now), "REGISTRY_ENTRY_REVOKED")
}

func TestRevokedEntryBlocksNewUseButRemainsHistoricallyAuditable(t *testing.T) {
	_, registry := fixtureProfileAndRegistry()
	entry := registry.Entries[0]
	entry.Lifecycle = "revoked"
	entry.Revocation = environment.RegistryRevocation{Status: "revoked", Severity: "high", Reason: "credential exfiltration risk"}
	registry.Entries[0] = entry
	signed, _, _ := signAndVerifyRegistry(t, registry)
	entry = signed.Entries[0]

	_, err := environment.AssessRegistryEntry(entry, environment.PurposeNewInstall)
	assertCode(t, err, "REGISTRY_ENTRY_REVOKED")
	_, err = environment.AssessRegistryEntry(entry, environment.PurposeNewRun)
	assertCode(t, err, "REGISTRY_ENTRY_HIGH_RISK_REVOKED")
	disposition, err := environment.AssessRegistryEntry(entry, environment.PurposeHistoricalAudit)
	must(t, err)
	if !disposition.Allowed || !disposition.HistoricalOnly || disposition.Warning != "credential exfiltration risk" {
		t.Fatalf("historical disposition = %#v", disposition)
	}
}

func TestLocalResolverIntersectsManifestRegistryAndLock(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	issuer, err := environment.NewIssuer("environment-release-2026", privateKey)
	must(t, err)
	verifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "environment-release-2026", Status: "active", PublicKey: publicKey}})
	must(t, err)
	resolver, err := environment.NewResolver(verifier)
	must(t, err)
	profile, rawRegistry := fixtureProfileAndRegistry()
	_, registry, _ := signAndVerifyRegistry(t, rawRegistry)
	unsigned, err := environment.BuildManifest("project-1", profile, registry, now, now.Add(24*time.Hour))
	must(t, err)
	manifest, err := issuer.Sign(unsigned)
	must(t, err)
	lock := lockForManifest(manifest)
	request := environment.LocalPlanRequest{ProjectID: "project-1", RunID: "local-run-1", Intent: "asset_generate", RequiredCapabilities: []string{"contentcloud.asset.generate"}, InputRefs: []string{"approved-snapshot:aps-1"}}

	ready, err := resolver.ResolveLocal(manifest, registry, lock, request, now.Add(time.Minute))
	must(t, err)
	if ready.State != "ready" || len(ready.Plugins) != 2 || len(ready.Preparation) != 0 || ready.RequiresServer || ready.PlanID == "" {
		t.Fatalf("ready plan = %#v", ready)
	}
	repeated, err := resolver.ResolveLocal(manifest, registry, lock, request, now.Add(time.Minute))
	must(t, err)
	if !reflect.DeepEqual(ready, repeated) {
		t.Fatalf("local plan is not deterministic:\nfirst=%#v\nsecond=%#v", ready, repeated)
	}

	missing := lock
	missing.Plugins = append([]environment.LockedPlugin(nil), lock.Plugins[:1]...)
	prepare, err := resolver.ResolveLocal(manifest, registry, missing, request, now.Add(time.Minute))
	must(t, err)
	if prepare.State != "environment_prepare" || len(prepare.Preparation) != 1 || prepare.Preparation[0].Plugin.ID != "contentcloud-visual-storytelling" || prepare.Preparation[0].Reason != "not_installed" {
		t.Fatalf("preparation plan = %#v", prepare)
	}

	denied := request
	denied.RequiredCapabilities = []string{"contentcloud.video.compose"}
	assertCode(t, resolveError(resolver, manifest, registry, lock, denied, now), "LOCAL_EXECUTION_CAPABILITY_DENIED")

	staleLock := lock
	staleLock.ManifestDigest = "sha256:" + repeat("f", 64)
	assertCode(t, resolveError(resolver, manifest, registry, staleLock, request, now), "ENVIRONMENT_LOCK_MISMATCH")

	revoked := rawRegistry
	revoked.Entries = append([]environment.RegistryEntry(nil), rawRegistry.Entries...)
	revoked.Entries[1].Lifecycle = "revoked"
	revoked.Entries[1].Revocation = environment.RegistryRevocation{Status: "revoked", Severity: "high", Reason: "high-risk incident"}
	_, verifiedRevoked, _ := signAndVerifyRegistry(t, revoked)
	assertCode(t, resolveError(resolver, manifest, verifiedRevoked, lock, request, now), "REGISTRY_ENTRY_HIGH_RISK_REVOKED")
}

func fixtureProfileAndRegistry() (environment.Profile, environment.Registry) {
	sceneDigest := "sha256:" + repeat("a", 64)
	packDigest := "sha256:" + repeat("b", 64)
	profile := environment.Profile{
		ID: "contentcloud.video-production", Version: "1.0.0", EnvironmentVersion: "2026.7.1", Harness: "codex", Marketplace: "contentcloud",
		Plugins: []environment.ProfilePlugin{
			{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.8.0", Required: true, Scope: "environment", Capabilities: []string{domain.KnowledgeExtractCapability}},
			{ID: "contentcloud-visual-storytelling", Kind: "skill_pack", Version: "1.2.0", Required: false, Scope: "task", Capabilities: []string{"contentcloud.asset.generate"}},
		},
		WorkspaceTemplate: environment.WorkspaceTemplateRef{ID: "workspace_marketing_video", Version: "2.2.0", Digest: "sha256:" + repeat("c", 64)},
		Capabilities:      []string{domain.KnowledgeExtractCapability, "contentcloud.asset.generate"},
		Policies:          environment.Policies{PublishRequiresConfirmation: true},
	}
	registry := environment.Registry{SchemaVersion: "1.0", Entries: []environment.RegistryEntry{
		registryEntry("contentcloud-video-production", "scene_plugin", "0.8.0", "https://github.com/limecloud/contentcloud", "v0.8.0", sceneDigest, []string{profile.ID}),
		registryEntry("contentcloud-visual-storytelling", "skill_pack", "1.2.0", "https://github.com/limecloud/contentcloud-packs", "v1.2.0", packDigest, []string{profile.ID}),
		registryEntry("contentcloud-unrelated", "skill_pack", "9.0.0", "https://example.invalid/unrelated", "v9.0.0", "sha256:"+repeat("d", 64), []string{"contentcloud.other"}),
	}}
	return profile, registry
}

func signedManifest(t *testing.T, issuer *environment.Issuer, now time.Time) environment.Manifest {
	t.Helper()
	profile, registry := fixtureProfileAndRegistry()
	_, verifiedRegistry, _ := signAndVerifyRegistry(t, registry)
	manifest, err := environment.BuildManifest("project-1", profile, verifiedRegistry, now, now.Add(24*time.Hour))
	must(t, err)
	manifest, err = issuer.Sign(manifest)
	must(t, err)
	return manifest
}

func lockForManifest(manifest environment.Manifest) environment.EnvironmentLock {
	plugins := make([]environment.LockedPlugin, 0, len(manifest.Distribution.Plugins))
	for _, plugin := range manifest.Distribution.Plugins {
		plugins = append(plugins, environment.LockedPlugin{ID: plugin.ID, Kind: plugin.Kind, Version: plugin.Version, Digest: plugin.Digest, Installed: true})
	}
	return environment.EnvironmentLock{SchemaVersion: "1.0", ProjectID: manifest.ProjectID, ProfileID: manifest.ProfileID, ProfileVersion: manifest.ProfileVersion, EnvironmentVersion: manifest.EnvironmentVersion, Harness: manifest.Harness, ManifestDigest: manifest.Digest, Plugins: plugins, VerifiedAt: manifest.IssuedAt}
}

func buildManifestError(profile environment.Profile, registry environment.VerifiedRegistry, now time.Time) error {
	_, err := environment.BuildManifest("project-1", profile, registry, now, now.Add(24*time.Hour))
	return err
}

func resolveError(resolver *environment.Resolver, manifest environment.Manifest, registry environment.VerifiedRegistry, lock environment.EnvironmentLock, request environment.LocalPlanRequest, now time.Time) error {
	_, err := resolver.ResolveLocal(manifest, registry, lock, request, now)
	return err
}

func registryEntry(id, kind, version, repository, ref, digest string, profiles []string) environment.RegistryEntry {
	return environment.RegistryEntry{
		ID: id, Kind: kind, Version: version, Source: environment.RegistrySource{Repository: repository, Ref: ref}, License: "Apache-2.0", Digest: digest,
		Signature: environment.RegistrySignature{Status: "pending"}, CompatibleProfiles: profiles, Permissions: []string{"workspace:read"},
		DataFlow: environment.RegistryDataFlow{LocalByDefault: true, CloudActions: []string{}}, OutputSchemas: []string{"contracts/content-item-3.0.schema.json"},
		Cost:       environment.RegistryCost{Model: "included", Notice: "Included in tests."},
		Evaluation: environment.RegistryEvaluation{Status: "passed", Report: ".agents/plugins/evaluations/test.json", Digest: "sha256:" + repeat("e", 64), Evidence: []string{"test"}},
		Lifecycle:  "published", Revocation: environment.RegistryRevocation{Status: "active"},
	}
}

func signAndVerifyRegistry(t *testing.T, registry environment.Registry) (environment.Registry, environment.VerifiedRegistry, *environment.RegistryVerifier) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	registry.Entries = append([]environment.RegistryEntry(nil), registry.Entries...)
	for index := range registry.Entries {
		registry.Entries[index].Signature = environment.RegistrySignature{Status: "verified", Algorithm: "ed25519", KeyID: "plugin-release-test"}
		payload, payloadErr := environment.RegistryEntrySigningPayload(registry.Entries[index])
		must(t, payloadErr)
		registry.Entries[index].Signature.Value = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	}
	verifier, err := environment.NewRegistryVerifier([]environment.RegistryTrustedKey{{KeyID: "plugin-release-test", Status: "active", PublicKey: publicKey}})
	must(t, err)
	verified, err := verifier.Verify(registry)
	must(t, err)
	return registry, verified, verifier
}

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != code {
		t.Fatalf("error = %#v, want domain code %s", err, code)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func repeat(value string, count int) string {
	result := ""
	for range count {
		result += value
	}
	return result
}
