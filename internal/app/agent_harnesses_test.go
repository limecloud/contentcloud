package app_test

import (
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestAgentHarnessCapabilitiesExposePiAndAgentSaaSWithoutBrandAssumption(t *testing.T) {
	service := app.New(memory.New(), nil)
	session, err := service.Register(t.Context(), "harness-owner@example.com", "long-enough-password", "Harness", "Harness Team")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	values, err := service.AgentHarnessCapabilities(t.Context(), actor)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, value := range values {
		found[value.Kind] = true
		if value.Kind == "fake" && !value.Available {
			t.Fatalf("configured harness was reported unavailable: %#v", value)
		}
	}
	for _, kind := range []string{"codex", "claude", "remote-http", "pi", "agent-saas"} {
		if !found[kind] {
			t.Fatalf("harness catalog is missing %s: %#v", kind, values)
		}
	}
}
