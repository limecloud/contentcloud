package environment_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"reflect"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
)

func TestCreativeExecutionBundleIsDeterministicAndBindsSubjectEnvironmentAndTrust(t *testing.T) {
	now := time.Date(2026, 7, 27, 14, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	issuer, err := environment.NewIssuer("environment-release-2026", privateKey)
	must(t, err)
	verifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "environment-release-2026", Status: "active", PublicKey: publicKey}})
	must(t, err)
	profile, rawRegistry := fixtureProfileAndRegistry()
	_, registry, _ := signAndVerifyRegistry(t, rawRegistry)
	controlPlane, err := environment.NewControlPlane(issuer, profile, registry, 24*time.Hour)
	must(t, err)
	subject := environment.ExecutionSubject{Type: "approved_snapshot", ID: "aps-1", Digest: "sha256:" + repeat("1", 64)}
	required := []environment.CapabilityRequirement{{ID: "contentcloud.asset.generate", SchemaVersion: "1.0.0", Digest: "sha256:" + repeat("2", 64)}}
	request := environment.ExecutionBundleRequest{ProjectID: "project-1", Subject: subject, RequiredCapabilities: required, PackIDs: []string{"contentcloud-visual-storytelling"}}

	bundle, err := controlPlane.IssueExecutionBundle(request, now)
	must(t, err)
	repeated, err := controlPlane.IssueExecutionBundle(request, now)
	must(t, err)
	if !reflect.DeepEqual(bundle, repeated) || len(bundle.BundleID) != 68 || bundle.Digest == "" {
		t.Fatalf("bundle is not deterministic or content-addressed:\nfirst=%#v\nsecond=%#v", bundle, repeated)
	}
	manifest := signedManifest(t, issuer, now)
	resolver, err := environment.NewResolver(verifier)
	must(t, err)
	lock := lockForManifest(manifest)
	capabilities := []domain.Capability{{ID: required[0].ID, Version: required[0].SchemaVersion, Kind: "business_capability", Digest: required[0].Digest, LocalOnly: true}}
	resolution, err := resolver.ResolveBundle(bundle, manifest, registry, lock, capabilities, environment.BundleVerifyOptions{ProjectID: "project-1", ExpectedSubject: subject, Now: now.Add(time.Minute)})
	must(t, err)
	if resolution.State != "ready" || len(resolution.Packs) != 1 || len(resolution.PluginPreparation) != 0 || len(resolution.CapabilityPreparation) != 0 {
		t.Fatalf("ready resolution = %#v", resolution)
	}

	tampered := bundle
	tampered.Subject.Digest = "sha256:" + repeat("3", 64)
	assertCode(t, verifier.VerifyBundle(tampered, manifest, registry, environment.BundleVerifyOptions{ProjectID: "project-1", Now: now}), "EXECUTION_BUNDLE_ID_MISMATCH")
	wrongSubject := subject
	wrongSubject.ID = "aps-2"
	assertCode(t, verifier.VerifyBundle(bundle, manifest, registry, environment.BundleVerifyOptions{ProjectID: "project-1", ExpectedSubject: wrongSubject, Now: now}), "EXECUTION_BUNDLE_SUBJECT_MISMATCH")
	assertCode(t, verifier.VerifyBundle(bundle, manifest, registry, environment.BundleVerifyOptions{ProjectID: "project-2", Now: now}), "ENVIRONMENT_PROJECT_MISMATCH")
	assertCode(t, verifier.VerifyBundle(bundle, manifest, registry, environment.BundleVerifyOptions{ProjectID: "project-1", Now: bundle.ExpiresAt}), "EXECUTION_BUNDLE_EXPIRED")

	revokedVerifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "environment-release-2026", Status: "revoked", PublicKey: publicKey}})
	must(t, err)
	assertCode(t, revokedVerifier.VerifyBundle(bundle, manifest, registry, environment.BundleVerifyOptions{ProjectID: "project-1", Now: now}), "EXECUTION_BUNDLE_SIGNING_KEY_UNTRUSTED")
}

func TestCreativeExecutionBundleFailsClosedForPackRegistryLockAndCapabilityDrift(t *testing.T) {
	now := time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC)
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
	controlPlane, err := environment.NewControlPlane(issuer, profile, registry, 24*time.Hour)
	must(t, err)
	subject := environment.ExecutionSubject{Type: "automation_task", ID: "run-1", Digest: "sha256:" + repeat("4", 64)}
	required := environment.CapabilityRequirement{ID: "contentcloud.asset.generate", SchemaVersion: "1.0.0", Digest: "sha256:" + repeat("5", 64)}
	bundle, err := controlPlane.IssueExecutionBundle(environment.ExecutionBundleRequest{ProjectID: "project-1", Subject: subject, RequiredCapabilities: []environment.CapabilityRequirement{required}, PackIDs: []string{"contentcloud-visual-storytelling"}}, now)
	must(t, err)
	manifest := signedManifest(t, issuer, now)
	lock := lockForManifest(manifest)
	matching := []domain.Capability{{ID: required.ID, Version: required.SchemaVersion, Kind: "business_capability", Digest: required.Digest, LocalOnly: true}}
	options := environment.BundleVerifyOptions{ProjectID: "project-1", ExpectedSubject: subject, Now: now.Add(time.Minute)}

	missingPack := lock
	missingPack.Plugins = append([]environment.LockedPlugin(nil), lock.Plugins[:1]...)
	resolution, err := resolver.ResolveBundle(bundle, manifest, registry, missingPack, matching, options)
	must(t, err)
	if resolution.State != "environment_prepare" || len(resolution.PluginPreparation) != 1 || resolution.PluginPreparation[0].Reason != "not_installed" {
		t.Fatalf("missing Pack resolution = %#v", resolution)
	}

	wrongDigest := append([]domain.Capability(nil), matching...)
	wrongDigest[0].Digest = "sha256:" + repeat("6", 64)
	resolution, err = resolver.ResolveBundle(bundle, manifest, registry, lock, wrongDigest, options)
	must(t, err)
	if resolution.State != "environment_prepare" || len(resolution.CapabilityPreparation) != 1 || resolution.CapabilityPreparation[0].Reason != "digest_mismatch" {
		t.Fatalf("capability drift resolution = %#v", resolution)
	}

	revoked := rawRegistry
	revoked.Entries = append([]environment.RegistryEntry(nil), rawRegistry.Entries...)
	revoked.Entries[1].Lifecycle = "revoked"
	revoked.Entries[1].Revocation = environment.RegistryRevocation{Status: "revoked", Severity: "high", Reason: "supply-chain incident"}
	_, verifiedRevoked, _ := signAndVerifyRegistry(t, revoked)
	assertCode(t, resolveBundleError(resolver, bundle, manifest, verifiedRevoked, lock, matching, options), "REGISTRY_ENTRY_HIGH_RISK_REVOKED")

	_, err = controlPlane.IssueExecutionBundle(environment.ExecutionBundleRequest{ProjectID: "project-1", Subject: subject, RequiredCapabilities: []environment.CapabilityRequirement{required}, PackIDs: []string{"contentcloud-video-production"}}, now)
	assertCode(t, err, "EXECUTION_BUNDLE_PACK_DENIED")

	unsupported := required
	unsupported.ID = "contentcloud.video.compose"
	_, err = controlPlane.IssueExecutionBundle(environment.ExecutionBundleRequest{ProjectID: "project-1", Subject: subject, RequiredCapabilities: []environment.CapabilityRequirement{unsupported}}, now)
	assertCode(t, err, "EXECUTION_BUNDLE_CAPABILITY_DENIED")
}

func resolveBundleError(resolver *environment.Resolver, bundle environment.CreativeExecutionBundle, manifest environment.Manifest, registry environment.VerifiedRegistry, lock environment.EnvironmentLock, capabilities []domain.Capability, options environment.BundleVerifyOptions) error {
	_, err := resolver.ResolveBundle(bundle, manifest, registry, lock, capabilities, options)
	return err
}
