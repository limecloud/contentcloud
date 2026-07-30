package app_test

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/fixturev3"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestImportFixtureV3IsIdempotentAndBuildsProjection(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "fixture@example.com", "long-enough-password", "Fixture", "Fixture Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixture := fixturev3.Fixture{
		FixtureVersion:    "3.0",
		EnvironmentDigest: "sha256:3333333333333333333333333333333333333333333333333333333333333333",
		Project:           fixturev3.ProjectSpec{BrandName: "Fixture Brand", ProductName: "Fixture Product", Channel: "douyin", StageObjective: "Verify V3"},
		Workspace:         fixturev3.WorkspaceSpec{TemplateID: "workspace_marketing_agent", TemplateVersion: "3.0.0", Targets: []string{"codex"}, DeviceName: "Fixture Device"},
		Submissions: []fixturev3.SubmissionSpec{
			{
				SubmissionType: "context", Outcome: "approved", Message: "context",
				Objects: []fixturev3.SubmissionObject{{ID: "context:fixture:v1", Type: "project_context", Version: 1, Path: "10-context/context.json", Content: json.RawMessage(`{"status":"approved"}`)}},
			},
			{
				SubmissionType: "knowledge", Outcome: "submitted", BaseSnapshotTypes: []string{"context"}, Message: "knowledge",
				Objects: []fixturev3.SubmissionObject{{ID: "knowledge:fixture:v1", Type: "knowledge_page", Version: 1, Path: "30-knowledge/pages/facts/fixture.md", Content: json.RawMessage(`{"status":"candidate","risk_level":"low"}`)}},
			},
			{
				SubmissionType: "content_batch", Outcome: "changes_requested", BaseSnapshotTypes: []string{"context"}, Message: "content", ChangeReason: "Knowledge is pending", ChangePointer: "/objects/0/content/blocked_reasons",
				Objects: []fixturev3.SubmissionObject{{ID: "content-batch:fixture:v1", Type: "content_batch", Version: 1, Path: "50-production/batches/fixture/manifest.yaml", Content: json.RawMessage(`{"schema_version":"contentcloud.content-batch/3.0","content_kind":"video_script","status":"blocked","publishable":false,"blocked_reasons":["knowledge pending"]}`)}},
			},
		},
	}

	first, err := service.ImportFixtureV3(t.Context(), actor, fixture, "fixture-first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ImportFixtureV3(t.Context(), actor, fixture, "fixture-second")
	if err != nil {
		t.Fatal(err)
	}
	if first.Project.ID != second.Project.ID || first.Device.ID != second.Device.ID || first.Workspace.ID != second.Workspace.ID {
		t.Fatalf("fixture identities changed across retries: first=%#v second=%#v", first, second)
	}
	if len(second.Submissions) != 3 || len(second.Snapshots) != 1 {
		t.Fatalf("fixture retry duplicated governed records: %#v", second)
	}
	statusByType := map[string]string{}
	for _, submission := range second.Submissions {
		statusByType[submission.SubmissionType] = submission.Status
	}
	if statusByType["context"] != "approved" || statusByType["knowledge"] != "submitted" || statusByType["content_batch"] != "changes_requested" {
		t.Fatalf("fixture outcomes do not represent the V3 review flow: %#v", statusByType)
	}
	projection, err := service.ProjectProjection(t.Context(), actor, first.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Sections["onboarding"].Count != 1 || projection.Sections["methodology"].Status != "ready" || projection.Sections["knowledge"].Status != "pending" || projection.Sections["creative"].Status != "blocked" {
		t.Fatalf("fixture projection is incomplete: %#v", projection.Sections)
	}
	if len(projection.NextActions) != 1 || projection.NextActions[0].Navigation.View != "review" || projection.NextActions[0].Navigation.Focus == nil {
		t.Fatalf("fixture projection did not expose a typed review target: %#v", projection.NextActions)
	}
	current, err := service.ProjectSubmissionRevision(t.Context(), actor, first.Project.ID, projection.NextActions[0].Navigation.Focus.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.NextActions[0].Navigation.Focus.Kind != "submission_revision" || projection.NextActions[0].Navigation.Focus.Digest != current.Revision.ContentHash {
		t.Fatalf("fixture projection navigation is not bound to the immutable revision: action=%#v revision=%#v", projection.NextActions[0], current.Revision)
	}
}

func TestImportCompleteJinlingFixtureUsesExternalPackage(t *testing.T) {
	file, err := os.Open(filepath.Join("..", "..", "fixtures", "v3", "jinling-gudu.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	fixture, err := fixturev3.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "fixture-complete@example.com", "long-enough-password", "Fixture", "Fixture Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.ImportFixtureV3(t.Context(), actor, fixture, "fixture-complete-first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ImportFixtureV3(t.Context(), actor, fixture, "fixture-complete-second")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Submissions) != 3 || len(first.Snapshots) != 1 || len(second.Submissions) != 3 || len(second.Snapshots) != 1 {
		t.Fatalf("external fixture did not converge to expected server governance state: first=%#v second=%#v", first, second)
	}
}
