package environment_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/integration/pluginbuiltin"
	"github.com/limecloud/contentcloud/internal/integration/pluginidentity"
)

func TestMarketingSkillPackComposesWithCoreEnvironmentExecution(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	profileID := "contentcloud.marketing"
	marketingPackage, err := pluginbuiltin.Load(t.TempDir(), pluginidentity.Marketing, pluginidentity.MarketingVersion)
	if err != nil {
		t.Fatal(err)
	}

	profile := environment.Profile{
		ID: profileID, Version: "1.0.0", EnvironmentVersion: "2026.8.15", Harness: "codex", Marketplace: "contentcloud",
		Plugins: []environment.ProfilePlugin{
			{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: "0.27.0", Required: true, Scope: "environment", Capabilities: []string{domain.KnowledgeExtractCapability}},
			{ID: pluginidentity.Marketing, Kind: "skill_pack", Version: pluginidentity.MarketingVersion, Required: false, Scope: "task", Capabilities: []string{"contentcloud.marketing.knowledge-governance", "contentcloud.marketing.content-orchestration"}},
		},
		WorkspaceTemplate: environment.WorkspaceTemplateRef{ID: "workspace_marketing_agent", Version: "3.0.0", Digest: "sha256:" + repeat("c", 64)},
		Capabilities:      []string{domain.KnowledgeExtractCapability, "contentcloud.marketing.knowledge-governance", "contentcloud.marketing.content-orchestration"},
		Policies:          environment.Policies{PublishRequiresConfirmation: true},
	}
	registry := environment.Registry{SchemaVersion: "1.0", Entries: []environment.RegistryEntry{
		registryEntry("contentcloud-video-production", "scene_plugin", "0.27.0", "https://github.com/limecloud/contentcloud", "v0.27.0", "sha256:"+repeat("a", 64), []string{profileID}),
		registryEntry(pluginidentity.Marketing, "skill_pack", pluginidentity.MarketingVersion, "https://github.com/limecloud/contentcloud", "v0.1.0", marketingPackage.Digest, []string{profileID}),
	}}
	_, verifiedRegistry, _ := signAndVerifyRegistry(t, registry)

	unsigned, err := environment.BuildManifest("project-marketing", []string{domain.ContentTypeVideoScript}, profile, verifiedRegistry, now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	issuer, verifier, err := marketingIssuerAndVerifier(t)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := issuer.Sign(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := environment.NewResolver(verifier)
	if err != nil {
		t.Fatal(err)
	}
	lock := lockForManifest(manifest)
	request := environment.LocalPlanRequest{ProjectID: "project-marketing", RunID: "run-marketing", Intent: "content", RequiredCapabilities: []string{"contentcloud.marketing.content-orchestration"}}
	plan, err := resolver.ResolveLocal(manifest, verifiedRegistry, lock, request, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "ready" || len(plan.Plugins) != 2 || plan.Plugins[0].ID != pluginidentity.Marketing || plan.Plugins[0].Kind != "skill_pack" {
		t.Fatalf("marketing execution plan = %#v", plan)
	}

	missing := lock
	missing.Plugins = []environment.LockedPlugin{lock.Plugins[1]}
	prepare, err := resolver.ResolveLocal(manifest, verifiedRegistry, missing, request, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if prepare.State != "environment_prepare" || len(prepare.Preparation) != 1 || prepare.Preparation[0].Plugin.ID != pluginidentity.Marketing || prepare.Preparation[0].Reason != "not_installed" {
		t.Fatalf("marketing preparation plan = %#v", prepare)
	}
}

func marketingIssuerAndVerifier(t *testing.T) (*environment.Issuer, *environment.Verifier, error) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	issuer, err := environment.NewIssuer("marketing-environment-test", privateKey)
	if err != nil {
		return nil, nil, err
	}
	verifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "marketing-environment-test", Status: "active", PublicKey: publicKey}})
	if err != nil {
		return nil, nil, err
	}
	return issuer, verifier, nil
}
