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
