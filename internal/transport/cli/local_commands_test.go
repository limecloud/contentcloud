package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

func TestLocalCLIExecutesClientFirstKnowledgeFlow(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: workspace, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	material := filepath.Join(t.TempDir(), "product.txt")
	if err := os.WriteFile(material, []byte("产品规格为20支。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertLocalCLI_OK(t, "local", "source", "register", material, "--directory", workspace, "--id", "source:product")
	assertLocalCLI_OK(t, "local", "source", "ingest", "source:product", "--directory", workspace)
	bundle, err := localworkspace.IngestLocalSource(workspace, "source:product", zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	locator, _ := json.Marshal(bundle.Evidence[0].Locator)
	pkg := sourcedomain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []sourcedomain.KnowledgeCandidate{{
		Kind: "fact", Title: "产品规格", Statement: bundle.Evidence[0].Quote, Subject: "产品", Predicate: "规格", Value: sourcedomain.TypedValue{Type: "text", Text: "20支"},
		Scope: sourcedomain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "low", AllowedChannels: []string{}, Evidence: []sourcedomain.EvidenceRef{{SourceRevisionID: "source:product", LocatorKind: bundle.Evidence[0].LocatorKind, Locator: string(locator), Quote: bundle.Evidence[0].Quote}}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{},
	}}, Warnings: []string{}}
	body, _ := json.Marshal(pkg)
	packagePath := filepath.Join(workspace, "40-work", "candidates.json")
	if err := os.WriteFile(packagePath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	assertLocalCLI_OK(t, "local", "run", "init", "--directory", workspace, "--id", "local-run-cli", "--intent", "intent:content")
	assertLocalCLI_OK(t, "local", "knowledge", "import", "40-work/candidates.json", "--directory", workspace, "--run", "local-run-cli")
	assertLocalCLI_OK(t, "local", "knowledge", "lint", "--directory", workspace)
	assertLocalCLI_OK(t, "local", "knowledge", "query", "--directory", workspace)
	assertLocalCLI_OK(t, "local", "knowledge", "diagnose", "--directory", workspace)
	assertLocalCLI_OK(t, "local", "knowledge", "pack", "--directory", workspace, "--name", "测试知识包")

	for _, name := range []string{
		"local.source.register", "local.source.list", "local.source.show", "local.source.ingest", "local.source.verify",
		"local.run.init", "local.run.show", "local.run.record", "local.run.check", "local.run.advance", "local.run.resume", "local.run.fail", "local.run.validate",
		"local.run.claim", "local.run.renew", "local.run.release", "local.run.claim-status",
		"local.handoff.create-ready", "local.handoff.list-ready", "local.handoff.accept", "local.handoff.complete", "local.handoff.supersede",
		"local.knowledge.import", "local.knowledge.lint", "local.knowledge.query", "local.knowledge.diagnose", "local.knowledge.pack",
		"local.audience.taxonomy.lint", "local.audience.strategy.scaffold", "local.audience.strategy.lint", "local.offer.lint",
		"local.brief.lint", "local.content.batch.init", "local.content.batch.lint", "local.content.batch.finalize", "local.content.item.lint", "local.content.item.diff", "local.content.delivery.export",
		"local.storyboard.create", "local.storyboard.prepare", "local.storyboard.lint",
		"local.seedance.export", "local.seedance.lint",
	} {
		if commandSchemas()[name] == nil {
			t.Fatalf("missing command schema %s", name)
		}
	}
}

func assertLocalCLI_OK(t *testing.T, args ...string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := (&Root{stdout: &stdout, stderr: &stderr}).command()
	command.SetArgs(append([]string{"--json"}, args...))
	if err := command.Execute(); err != nil {
		t.Fatalf("command %v failed: %v; stderr=%s", args, err, stderr.String())
	}
	var envelope struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK {
		t.Fatalf("unexpected command output for %v: %v %s", args, err, stdout.String())
	}
}

func zeroTime() time.Time { return time.Time{} }
