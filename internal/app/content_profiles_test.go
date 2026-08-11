package app_test

import (
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestInstallContentProfilesCompilesPublishedProviderNeutralSOPs(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "profile-owner@example.com", "long-enough-password", "Profile Owner", "Profile Team")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}

	profiles := service.ContentProfiles()
	if len(profiles) != 3 {
		t.Fatalf("expected three built-in content profiles, got %d", len(profiles))
	}
	for _, profile := range profiles {
		installed, err := service.InstallContentProfile(ctx, actor, profile.ID, "profile-test")
		if err != nil {
			t.Fatalf("install %s: %v", profile.ID, err)
		}
		if installed.SOP.Definition.SourceRef != "content-profile:"+profile.ID+"@"+profile.Version+"#"+profile.Digest {
			t.Fatalf("profile source ref was not pinned: %#v", installed.SOP.Definition)
		}
		if len(installed.SOP.Versions) != 1 || installed.SOP.Versions[0].Status != "published" {
			t.Fatalf("profile did not compile to one published SOP: %#v", installed.SOP.Versions)
		}
		version := installed.SOP.Versions[0]
		if len(version.Stages) != len(profile.Stages) || len(version.Gates) != len(profile.RequiredGates) {
			t.Fatalf("compiled SOP shape drifted for %s", profile.ID)
		}
		for _, stage := range version.Stages {
			if len(stage.RequiredCapabilities) != 1 || len(stage.AllowedExecutorKinds) == 0 {
				t.Fatalf("stage lost capability or executor routing: %#v", stage)
			}
			for _, kind := range stage.AllowedExecutorKinds {
				if kind == "codex" || kind == "claude_code" {
					t.Fatalf("profile hard-coded a branded executor: %#v", stage)
				}
			}
		}
		if !containsExecutorKind(version, "agent_saas") && profile.ID != "douyin-commerce-video" {
			t.Fatalf("profile %s cannot route work to Agent SaaS", profile.ID)
		}

		reinstalled, err := service.InstallContentProfile(ctx, actor, profile.ID, "profile-test-repeat")
		if err != nil {
			t.Fatalf("reinstall %s: %v", profile.ID, err)
		}
		if len(reinstalled.SOP.Versions) != 1 || reinstalled.SOP.Versions[0].ID != version.ID {
			t.Fatalf("idempotent install created a duplicate version: %#v", reinstalled.SOP.Versions)
		}
	}
}

func TestSerializedNovelContentTypeIsSupported(t *testing.T) {
	if !domain.ValidTenantContentType(domain.ContentTypeSerializedNovel) {
		t.Fatal("serialized novel content type must be available to projects")
	}
	if !domain.ValidOptionalTenantContentType(domain.ContentTypeSerializedNovel) {
		t.Fatal("serialized novel content type must be available to tenant capabilities")
	}
}

func containsExecutorKind(version domain.SOPVersion, expected string) bool {
	for _, stage := range version.Stages {
		if strings.Contains("|"+strings.Join(stage.AllowedExecutorKinds, "|")+"|", "|"+expected+"|") {
			return true
		}
	}
	return false
}
