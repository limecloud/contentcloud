package app_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestAutomationPollRequiresVerifiedEnvironmentPackAndCapabilityBeforeLease(t *testing.T) {
	ctx := context.Background()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	issuer, err := environment.NewIssuer("environment-automation-test", privateKey)
	must(t, err)
	registry := automationVerifiedRegistry(t)
	controlPlane, err := environment.NewControlPlane(issuer, automationProfile(), registry, 24*time.Hour)
	must(t, err)
	requirement := environment.CapabilityRequirement{ID: domain.KnowledgeExtractCapability, SchemaVersion: "1.0.0", Digest: "sha256:" + strings.Repeat("d", 64)}
	store := memory.New()
	service := app.New(store, slog.Default(), app.WithEnvironmentControlPlane(controlPlane), app.WithAutomationExecutionPolicy([]environment.CapabilityRequirement{requirement}, map[string][]string{requirement.ID: {"contentcloud-evidence-reasoning"}}))
	session, err := service.Register(ctx, "automation-gate@example.com", "long-enough-password", "Owner", "Automation Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "可信原文", nil)
	capability := domain.Capability{ID: requirement.ID, Version: requirement.SchemaVersion, Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, Digest: requirement.Digest, LocalOnly: true}
	connect, err := service.CreateConnectSession(ctx, actor, project.ID, "")
	must(t, err)
	connected, err := testsupport.ConnectBootstrap(ctx, service, actor, connect, app.ConnectDeviceInput{Hostname: "automation-local", Platform: "darwin", Arch: "arm64", Version: "test", Capabilities: []domain.Capability{capability}})
	must(t, err)
	if connected.EnvironmentManifest == nil {
		t.Fatal("configured control plane did not return Environment Manifest")
	}
	deviceActor, device, err := service.DeviceActor(ctx, connected.DeviceToken)
	must(t, err)
	run, err := service.CreateKnowledgeExtractionRun(ctx, actor, app.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: "automation-gate", OutputCount: 1}, "")
	must(t, err)
	bundle, err := store.ExecutionBundle(ctx, actor.TenantID, run.ID)
	must(t, err)
	if bundle.Subject.Type != "context_snapshot" || bundle.Subject.ID != run.InputSnapshotID || len(bundle.Packs) != 1 || bundle.Packs[0].ID != "contentcloud-evidence-reasoning" {
		t.Fatalf("run bundle is not bound to the frozen input and selected Pack: %#v", bundle)
	}

	_, err = service.PollWithEnvironment(ctx, deviceActor, device, []domain.Capability{capability}, nil)
	assertDomainCode(t, err, "ENVIRONMENT_PREPARATION_REQUIRED")
	assertRunUnleased(t, ctx, service, store, actor, run.ID)

	manifest := *connected.EnvironmentManifest
	lock := automationLock(manifest)
	missingPack := lock
	missingPack.Plugins = append([]environment.LockedPlugin(nil), lock.Plugins[:1]...)
	_, err = service.PollWithEnvironment(ctx, deviceActor, device, []domain.Capability{capability}, []app.AutomationEnvironmentClaim{{Manifest: manifest, Lock: missingPack}})
	assertDomainCode(t, err, "ENVIRONMENT_PREPARATION_REQUIRED")
	assertRunUnleased(t, ctx, service, store, actor, run.ID)

	wrongCapability := capability
	wrongCapability.Digest = "sha256:" + strings.Repeat("e", 64)
	_, err = service.PollWithEnvironment(ctx, deviceActor, device, []domain.Capability{wrongCapability}, []app.AutomationEnvironmentClaim{{Manifest: manifest, Lock: lock}})
	assertDomainCode(t, err, "ENVIRONMENT_PREPARATION_REQUIRED")
	assertRunUnleased(t, ctx, service, store, actor, run.ID)

	lease, err := service.PollWithRuntime(ctx, deviceActor, device, []domain.Capability{capability}, []app.AutomationEnvironmentClaim{{Manifest: manifest, Lock: lock}}, "0.14.0")
	must(t, err)
	if lease.Run.ID != run.ID || lease.ExecutionBundle == nil || lease.ExecutionBundle.BundleID != bundle.BundleID || lease.Attempt.CapabilityDigest != requirement.Digest {
		t.Fatalf("verified environment did not receive the exact bundle-bound lease: %#v", lease)
	}
	devices, err := service.Devices(ctx, actor, project.ID)
	must(t, err)
	if len(devices) != 1 || devices[0].Version != "0.14.0" {
		t.Fatalf("daemon poll did not refresh the running CLI version: %#v", devices)
	}
	verifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "environment-automation-test", Status: "active", PublicKey: publicKey}})
	must(t, err)
	must(t, verifier.VerifyBundle(*lease.ExecutionBundle, manifest, registry, environment.BundleVerifyOptions{ProjectID: project.ID, ExpectedSubject: bundle.Subject, Now: time.Now().UTC()}))
}

func assertRunUnleased(t *testing.T, ctx context.Context, service *app.Service, store *memory.Store, actor app.Actor, runID string) {
	t.Helper()
	stored, err := service.Run(ctx, actor, runID)
	must(t, err)
	if stored.State != "queued" || stored.AttemptCount != 0 || stored.ActiveAttemptID != "" {
		t.Fatalf("failed Automation environment gate mutated run: %#v", stored)
	}
	attempts, err := store.RunAttempts(ctx, actor.TenantID, runID)
	must(t, err)
	if len(attempts) != 0 {
		t.Fatalf("failed Automation environment gate created attempts: %#v", attempts)
	}
}

func automationProfile() environment.Profile {
	return environment.Profile{
		ID: "contentcloud.video-production", Version: "1.0.0", EnvironmentVersion: "2026.7.1", Harness: "codex", Marketplace: "contentcloud",
		Plugins: []environment.ProfilePlugin{
			{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.8.0", Required: true, Scope: "environment", Capabilities: []string{domain.KnowledgeExtractCapability}},
			{ID: "contentcloud-evidence-reasoning", Kind: "skill_pack", Version: "1.0.0", Scope: "task", Capabilities: []string{domain.KnowledgeExtractCapability}},
		},
		WorkspaceTemplate: environment.WorkspaceTemplateRef{ID: "workspace_marketing_video", Version: "2.2.0", Digest: "sha256:" + strings.Repeat("c", 64)},
		Capabilities:      []string{domain.KnowledgeExtractCapability},
		Policies:          environment.Policies{PublishRequiresConfirmation: true, AutomationEnabled: true},
	}
}

func automationVerifiedRegistry(t *testing.T) environment.VerifiedRegistry {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	registry := environment.Registry{SchemaVersion: "1.0", Entries: []environment.RegistryEntry{
		automationRegistryEntry("contentcloud-video-production", "scene_plugin", "0.8.0", "v0.8.0", "a"),
		automationRegistryEntry("contentcloud-evidence-reasoning", "skill_pack", "1.0.0", "v1.0.0", "b"),
	}}
	for index := range registry.Entries {
		registry.Entries[index].Signature = environment.RegistrySignature{Status: "verified", Algorithm: "ed25519", KeyID: "plugin-release-automation-test"}
		payload, payloadErr := environment.RegistryEntrySigningPayload(registry.Entries[index])
		must(t, payloadErr)
		registry.Entries[index].Signature.Value = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	}
	verifier, err := environment.NewRegistryVerifier([]environment.RegistryTrustedKey{{KeyID: "plugin-release-automation-test", Status: "active", PublicKey: publicKey}})
	must(t, err)
	verified, err := verifier.Verify(registry)
	must(t, err)
	return verified
}

func automationRegistryEntry(id, kind, version, ref, digestByte string) environment.RegistryEntry {
	return environment.RegistryEntry{
		ID: id, Kind: kind, Version: version, Source: environment.RegistrySource{Repository: "https://github.com/limecloud/contentcloud", Ref: ref}, License: "Apache-2.0", Digest: "sha256:" + strings.Repeat(digestByte, 64),
		Signature: environment.RegistrySignature{Status: "pending"}, CompatibleProfiles: []string{"contentcloud.video-production"}, Permissions: []string{"workspace:read"},
		DataFlow: environment.RegistryDataFlow{LocalByDefault: true, CloudActions: []string{}}, OutputSchemas: []string{"contracts/knowledge-candidates-1.0.schema.json"},
		Cost:       environment.RegistryCost{Model: "included", Notice: "Included in tests."},
		Evaluation: environment.RegistryEvaluation{Status: "passed", Report: ".agents/plugins/evaluations/test.json", Digest: "sha256:" + strings.Repeat("f", 64), Evidence: []string{"test"}}, Lifecycle: "published", Revocation: environment.RegistryRevocation{Status: "active"},
	}
}

func automationLock(manifest environment.Manifest) environment.EnvironmentLock {
	plugins := make([]environment.LockedPlugin, 0, len(manifest.Distribution.Plugins))
	for _, plugin := range manifest.Distribution.Plugins {
		plugins = append(plugins, environment.LockedPlugin{ID: plugin.ID, Kind: plugin.Kind, Version: plugin.Version, Digest: plugin.Digest, Installed: true})
	}
	return environment.EnvironmentLock{SchemaVersion: "1.0", ProjectID: manifest.ProjectID, ProfileID: manifest.ProfileID, ProfileVersion: manifest.ProfileVersion, EnvironmentVersion: manifest.EnvironmentVersion, Harness: manifest.Harness, ManifestDigest: manifest.Digest, Plugins: plugins, VerifiedAt: time.Now().UTC()}
}
