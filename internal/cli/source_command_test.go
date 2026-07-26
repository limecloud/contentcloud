package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceWriteDryRunsDoNotRequireCredentials(t *testing.T) {
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	file := filepath.Join(t.TempDir(), "manual.txt")
	if err := os.WriteFile(file, []byte("source material"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"--json", "--project", "project-1", "source", "upload", file, "--dry-run"},
		{"--json", "source", "revise", "source-1", file, "--dry-run"},
		{"--json", "source", "evidence-review", "evidence-1", "accept", "--dry-run"},
	} {
		var stdout, stderr bytes.Buffer
		root := &Root{stdout: &stdout, stderr: &stderr}
		command := root.command()
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("dry-run %v failed: %v; stderr=%s", args, err, stderr.String())
		}
		var envelope struct {
			OK   bool `json:"ok"`
			Data struct {
				DryRun bool `json:"dry_run"`
			} `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("decode dry-run output: %v; output=%s", err, stdout.String())
		}
		if !envelope.OK || !envelope.Data.DryRun {
			t.Fatalf("unexpected dry-run envelope: %s", stdout.String())
		}
	}
}

func TestSourceCommandSchemasCoverGovernedRevisionFlow(t *testing.T) {
	schemas := commandSchemas()
	for _, name := range []string{"source.revisions", "source.revise", "source.impact", "evidence.review", "asset.list", "asset.create", "rights.list", "rights.create", "rights.review"} {
		if schemas[name] == nil {
			t.Fatalf("command schema %q is missing", name)
		}
	}
}

func TestAssetRightsWriteDryRunsDoNotRequireCredentials(t *testing.T) {
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	for _, args := range [][]string{
		{"--json", "--project", "project-1", "asset", "create", "--name", "Hero", "--source-revision", "revision-1", "--usage", "owned", "--dry-run"},
		{"--json", "asset", "rights-create", "asset-1", "--holder", "Brand", "--channel", "douyin", "--proof-source-revision", "revision-1", "--dry-run"},
		{"--json", "asset", "rights-review", "rights-1", "approve", "--dry-run"},
	} {
		var stdout, stderr bytes.Buffer
		root := &Root{stdout: &stdout, stderr: &stderr}
		command := root.command()
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("dry-run %v failed: %v; stderr=%s", args, err, stderr.String())
		}
		var envelope struct {
			OK   bool `json:"ok"`
			Data struct {
				DryRun bool `json:"dry_run"`
			} `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || !envelope.Data.DryRun {
			t.Fatalf("unexpected dry-run output: %v %s", err, stdout.String())
		}
	}
}

func TestBriefWriteDryRunsDoNotRequireCredentials(t *testing.T) {
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	briefFile := filepath.Join(t.TempDir(), "brief.json")
	if err := os.WriteFile(briefFile, []byte(`{"project_id":"project-1","objective":"awareness"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--json", "brief", "create", "--file", briefFile, "--dry-run"},
		{"--json", "brief", "revise", "brief-1", "--file", briefFile, "--reason", "change objective", "--dry-run"},
		{"--json", "brief", "submit", "brief-1", "--dry-run"},
		{"--json", "brief", "return", "brief-1", "--reason", "evidence is unclear", "--dry-run"},
		{"--json", "brief", "approve", "brief-1", "--dry-run"},
	} {
		var stdout, stderr bytes.Buffer
		root := &Root{stdout: &stdout, stderr: &stderr}
		command := root.command()
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("dry-run %v failed: %v; stderr=%s", args, err, stderr.String())
		}
		var envelope struct {
			OK   bool `json:"ok"`
			Data struct {
				DryRun bool `json:"dry_run"`
			} `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || !envelope.Data.DryRun {
			t.Fatalf("unexpected dry-run output: %v %s", err, stdout.String())
		}
	}
}

func TestBriefCommandSchemasCoverImmutableRevisionFlow(t *testing.T) {
	schemas := commandSchemas()
	for _, name := range []string{"brief.list", "brief.show", "brief.create", "brief.revise", "brief.submit", "brief.return", "brief.approve"} {
		if schemas[name] == nil {
			t.Fatalf("command schema %q is missing", name)
		}
	}
}

func TestKnowledgeExtractionDryRunDoesNotRequireCredentials(t *testing.T) {
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	var stdout, stderr bytes.Buffer
	root := &Root{stdout: &stdout, stderr: &stderr}
	command := root.command()
	command.SetArgs([]string{"--json", "--project", "project-1", "knowledge", "extract", "--source-revision", "revision-1", "--count", "3", "--dry-run"})
	if err := command.Execute(); err != nil {
		t.Fatalf("knowledge extract dry-run failed: %v; stderr=%s", err, stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			DryRun bool `json:"dry_run"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || !envelope.Data.DryRun {
		t.Fatalf("unexpected dry-run output: %v %s", err, stdout.String())
	}
	if commandSchemas()["knowledge.extract"] == nil {
		t.Fatal("knowledge.extract command schema is missing")
	}
}

func TestScriptChangeDryRunsDoNotRequireCredentials(t *testing.T) {
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	for _, args := range [][]string{
		{"--json", "script", "revise", "script-version-1", "--reason", "resolve review feedback", "--changed-field", "/shots/0/voiceover", "--dry-run"},
		{"--json", "script", "variant", "script-version-1", "--reason", "test title", "--changed-field", "/title", "--hypothesis", "a concrete title improves retention", "--invariant", "/shots", "--dry-run"},
	} {
		var stdout, stderr bytes.Buffer
		root := &Root{stdout: &stdout, stderr: &stderr}
		command := root.command()
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("script change dry-run %v failed: %v; stderr=%s", args, err, stderr.String())
		}
		var envelope struct {
			OK   bool `json:"ok"`
			Data struct {
				DryRun bool `json:"dry_run"`
			} `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || !envelope.Data.DryRun {
			t.Fatalf("unexpected script change dry-run output: %v %s", err, stdout.String())
		}
	}
	for _, name := range []string{"script.revise", "script.variant"} {
		if commandSchemas()[name] == nil {
			t.Fatalf("command schema %q is missing", name)
		}
	}
}
