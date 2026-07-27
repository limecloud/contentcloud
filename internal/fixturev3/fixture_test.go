package fixturev3

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeFixture(t *testing.T) {
	fixture, err := Decode(strings.NewReader(`{
        "fixture_version":"3.0",
        "environment_digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333",
        "project":{"brand_name":"Fixture Brand","product_name":"Fixture Product","channel":"douyin","stage_objective":"test","owner_name":"Owner","reviewer_name":"Reviewer","client_approver":"Client"},
        "workspace":{"template_id":"workspace_marketing_agent","template_version":"3.0.0","targets":["codex"],"device_name":"Fixture Device"},
        "submissions":[{"submission_type":"context","outcome":"approved","base_snapshot_types":[],"message":"context","objects":[{"id":"context:fixture:v1","type":"project_context","version":1,"path":"10-context/context.json","content":{"status":"approved"}}]}]
    }`))
	if err != nil {
		t.Fatal(err)
	}
	if fixture.FixtureVersion != "3.0" || len(fixture.Submissions) != 1 {
		t.Fatalf("unexpected fixture: %#v", fixture)
	}
	if fixture.Submissions[0].SubmissionType != "context" || fixture.Submissions[0].Outcome != "approved" {
		t.Fatalf("fixture workflow order is invalid: %#v", fixture.Submissions)
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	_, err := Decode(strings.NewReader(`{"fixture_version":"3.0","unknown":true}`))
	if err == nil {
		t.Fatal("expected unknown fixture field to be rejected")
	}
}

func TestDeterministicID(t *testing.T) {
	first := DeterministicID("workspace", "project-1")
	if first != DeterministicID("workspace", "project-1") || first == DeterministicID("workspace", "project-2") {
		t.Fatalf("deterministic IDs are unstable: %q", first)
	}
}

func TestDecodeCompleteJinlingFixture(t *testing.T) {
	file, err := os.Open(filepath.Join("..", "..", "fixtures", "v3", "jinling-gudu.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	fixture, err := Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.Scenario == nil || len(fixture.Scenario.Sources) != 20 || len(fixture.Scenario.Methodology.Dimensions) != 15 || len(fixture.Scenario.ContentBatch.Items) != 10 {
		t.Fatalf("complete fixture lost its acceptance shape: %#v", fixture.Scenario)
	}
}
