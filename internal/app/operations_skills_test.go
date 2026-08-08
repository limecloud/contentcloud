package app_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestOperationsSkillsProjectsOnlyVerifiedSkillPacks(t *testing.T) {
	controlPlane := operationsSkillControlPlane(t)
	service := app.New(memory.New(), slog.Default(), app.WithEnvironmentControlPlane(controlPlane), app.WithPlatformAdminEmails("skills@example.com"))
	session, err := service.Register(t.Context(), "skills@example.com", "long-enough-password", "技能包运营", "技能包租户")
	must(t, err)
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	must(t, err)

	directory, err := service.OperationsSkills(t.Context(), actor)
	must(t, err)
	if !directory.Configured || directory.Source != "verified_plugin_registry" || directory.RegistrySchemaVersion != "1.0" {
		t.Fatalf("unexpected skill directory metadata: %#v", directory)
	}
	if len(directory.Skills) != 1 {
		t.Fatalf("only skill_pack entries should be projected: %#v", directory.Skills)
	}
	skill := directory.Skills[0]
	if skill.ID != "contentcloud-script-writing" || skill.Version != "1.2.0" || !skill.AvailableForNewRuns {
		t.Fatalf("skill identity or eligibility is incomplete: %#v", skill)
	}
	if skill.Signature.Status != "verified" || skill.Signature.KeyID != "plugin-release-operations-test" || skill.Evaluation.Status != "passed" {
		t.Fatalf("verification facts are incomplete: %#v", skill)
	}
	if len(skill.Permissions) != 1 || len(skill.OutputSchemas) != 1 || len(skill.Evaluation.Evidence) != 1 {
		t.Fatalf("skill contract collections are incomplete: %#v", skill)
	}
	body, err := json.Marshal(skill)
	must(t, err)
	if strings.Contains(string(body), "signature-value-must-not-leak") || strings.Contains(string(body), `"value"`) {
		t.Fatalf("operations projection leaked the registry signature value: %s", body)
	}

	detail, err := service.OperationsSkill(t.Context(), actor, skill.ID, skill.Version)
	must(t, err)
	if detail.Digest != skill.Digest || detail.Source.Ref != "v1.2.0" {
		t.Fatalf("skill detail does not match the directory: %#v", detail)
	}
}

func TestOperationsSkillsReturnsHonestUnconfiguredDirectory(t *testing.T) {
	service := app.New(memory.New(), slog.Default(), app.WithPlatformAdminEmails("skills-empty@example.com"))
	session, err := service.Register(t.Context(), "skills-empty@example.com", "long-enough-password", "技能包运营", "技能包租户")
	must(t, err)
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	must(t, err)

	directory, err := service.OperationsSkills(t.Context(), actor)
	must(t, err)
	if directory.Configured || directory.Skills == nil || len(directory.Skills) != 0 {
		t.Fatalf("unconfigured skill registry must return an explicit empty directory: %#v", directory)
	}
}

func TestOperationsSkillsRequirePlatformAdmin(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "skills-member@example.com", "long-enough-password", "租户管理员", "技能包租户")
	must(t, err)
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	must(t, err)
	if _, err := service.OperationsSkills(t.Context(), actor); err == nil {
		t.Fatal("tenant administrators must not read the platform skill registry")
	}
}

func operationsSkillControlPlane(t *testing.T) *environment.ControlPlane {
	t.Helper()
	registry := appEnvironmentRegistry()
	skill := registry.Entries[0]
	skill.ID = "contentcloud-script-writing"
	skill.Kind = "skill_pack"
	skill.Version = "1.2.0"
	skill.Source.Ref = "v1.2.0"
	registry.Entries = append(registry.Entries, skill)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	for index := range registry.Entries {
		registry.Entries[index].Signature = environment.RegistrySignature{Status: "verified", Algorithm: "ed25519", KeyID: "plugin-release-operations-test"}
		payload, payloadErr := environment.RegistryEntrySigningPayload(registry.Entries[index])
		must(t, payloadErr)
		registry.Entries[index].Signature.Value = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	}
	verifier, err := environment.NewRegistryVerifier([]environment.RegistryTrustedKey{{KeyID: "plugin-release-operations-test", Status: "active", PublicKey: publicKey}})
	must(t, err)
	verified, err := verifier.Verify(registry)
	must(t, err)
	issuer, err := environment.NewIssuer("environment-operations-test", privateKey)
	must(t, err)
	profile := appEnvironmentProfile()
	profile.Plugins = append(profile.Plugins, environment.ProfilePlugin{ID: skill.ID, Kind: skill.Kind, Version: skill.Version, Required: true, Scope: "task", Capabilities: []string{"script_generation"}})
	controlPlane, err := environment.NewControlPlane(issuer, profile, verified, 24*time.Hour)
	must(t, err)
	return controlPlane
}
