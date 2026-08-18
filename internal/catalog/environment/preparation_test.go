package environment_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/catalog/environment"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
)

func TestPreparationPlanBindsSignedPermissionsCostAndExecutionPlan(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	profile, rawRegistry := fixtureProfileAndRegistry()
	_, registry, _ := signAndVerifyRegistry(t, rawRegistry)
	manifest, err := environment.BuildManifest("project-1", []string{identitydomain.ContentTypeVideoScript}, profile, registry, now, now.Add(24*time.Hour))
	must(t, err)
	execution := environment.LocalExecutionPlan{
		SchemaVersion: "1.0", PlanID: "lep_" + repeat("1", 64), RunID: "run-1", Intent: "asset_generate", RequiredCapabilities: []string{"contentcloud.asset.generate"},
		Plugins: []environment.PluginRef{manifest.Distribution.Plugins[1]}, Preparation: []environment.PluginPreparation{{Plugin: manifest.Distribution.Plugins[1], Reason: "not_installed"}},
		InputRefs: []string{}, EnvironmentDigest: "sha256:" + repeat("2", 64), State: "environment_prepare",
	}
	plan, err := environment.BuildPreparationPlan("project-1", execution, registry)
	must(t, err)
	if plan.State != "ready" || !plan.RequiresConfirmation || !plan.RequiresNewChat || len(plan.Actions) != 1 || !strings.HasPrefix(plan.PreparationID, "epp_") {
		t.Fatalf("preparation plan = %#v", plan)
	}
	if plan.Actions[0].Cost.Model != "included" || len(plan.Actions[0].Permissions) == 0 {
		t.Fatalf("signed disclosures missing: %#v", plan.Actions[0])
	}
	repeated, err := environment.BuildPreparationPlan("project-1", execution, registry)
	must(t, err)
	if repeated.PreparationID != plan.PreparationID {
		t.Fatal("preparation plan is not deterministic")
	}
}

func TestPreparedLockAddsOnlyExactConfirmedTaskPack(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 30, 0, 0, time.UTC)
	profile, rawRegistry := fixtureProfileAndRegistry()
	_, registry, _ := signAndVerifyRegistry(t, rawRegistry)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	issuer, err := environment.NewIssuer("environment-preparation-test", privateKey)
	must(t, err)
	unsigned, err := environment.BuildManifest("project-1", []string{identitydomain.ContentTypeVideoScript}, profile, registry, now, now.Add(24*time.Hour))
	must(t, err)
	manifest, err := issuer.Sign(unsigned)
	must(t, err)
	verifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "environment-preparation-test", Status: "active", PublicKey: publicKey}})
	must(t, err)
	resolver, err := environment.NewResolver(verifier)
	must(t, err)
	current := lockForManifest(manifest)
	current.Plugins = current.Plugins[:1]
	execution, err := resolver.ResolveLocal(manifest, registry, current, environment.LocalPlanRequest{
		ProjectID: "project-1", RunID: "run-prepare", Intent: "asset_generate",
		RequiredCapabilities: []string{"contentcloud.asset.generate"}, InputRefs: []string{"approved-snapshot:aps-1"},
	}, now.Add(time.Minute))
	must(t, err)
	plan, err := environment.BuildPreparationPlan("project-1", execution, registry)
	must(t, err)
	next, err := environment.PreparedLock(manifest, current, plan, registry, now.Add(2*time.Minute))
	must(t, err)
	if len(next.Plugins) != 2 || next.Plugins[1].ID != "contentcloud-visual-storytelling" || !next.Plugins[1].Installed {
		t.Fatalf("prepared lock = %#v", next)
	}
	must(t, environment.ValidateLock(manifest, next))

	repair := plan
	repair.Actions = append([]environment.PreparationAction(nil), plan.Actions...)
	repair.Actions[0].Reason = "digest_mismatch"
	_, err = environment.PreparedLock(manifest, current, repair, registry, now.Add(2*time.Minute))
	assertCode(t, err, "ENVIRONMENT_PREPARATION_REPAIR_REQUIRED")
}
