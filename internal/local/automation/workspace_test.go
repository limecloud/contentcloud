package automationworkspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	"github.com/limecloud/contentcloud/internal/catalog/environment"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestAttemptWorkspaceFreezesInputsWithoutRunCredentialAndUsesExclusiveLease(t *testing.T) {
	now := time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC)
	base := filepath.Join(t.TempDir(), "automation")
	contract := testContract()
	bundle := &environment.CreativeExecutionBundle{BundleID: "ceb_test", ProjectID: "project-1", Digest: "sha256:" + strings.Repeat("b", 64)}
	options := Options{
		BaseDir: base, AttemptID: "attempt-1", RunID: "run-1", ProjectID: "project-1", Contract: contract, Bundle: bundle,
		OutputSchema: []byte(`{"type":"object"}`), Skill: []byte("# Test Skill\n"), Now: now, ExpiresAt: now.Add(5 * time.Minute),
	}
	workspace, err := Begin(options)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(workspace.Root) != base || workspace.Lease.AttemptID != "attempt-1" || workspace.Lease.ContractDigest == "" || workspace.Lease.BundleID != "ceb_test" {
		t.Fatalf("workspace = %#v", workspace)
	}
	for _, file := range []struct {
		name string
		mode os.FileMode
	}{{"lease.json", 0o600}, {"contract.json", 0o400}, {"output.schema.json", 0o400}, {"SKILL.md", 0o400}, {"execution-bundle.json", 0o400}} {
		path := filepath.Join(workspace.Root, file.name)
		info, statErr := os.Lstat(path)
		if statErr != nil {
			t.Fatalf("stat %s: %v", file.name, statErr)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != file.mode {
			t.Fatalf("%s mode=%v", file.name, info.Mode().Perm())
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(body), "rt_super_secret") || strings.Contains(string(body), "run_token") {
			t.Fatalf("%s contains a run credential", file.name)
		}
	}
	if _, err := Begin(options); errorCode(err) != "AUTOMATION_WORKSPACE_LEASE_ACTIVE" {
		t.Fatalf("duplicate attempt error = %#v", err)
	}
	if err := workspace.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace.Root); !os.IsNotExist(err) {
		t.Fatalf("workspace remains after cleanup: %v", err)
	}
}

func TestAttemptWorkspaceRejectsInteractiveOverlapAndRecoversOnlyExpiredOwnedLease(t *testing.T) {
	now := time.Date(2026, 7, 27, 15, 30, 0, 0, time.UTC)
	parent := t.TempDir()
	interactive := filepath.Join(parent, "interactive")
	if err := os.Mkdir(interactive, 0o700); err != nil {
		t.Fatal(err)
	}
	options := Options{
		BaseDir: filepath.Join(interactive, ".contentcloud", "automation"), ForbiddenRoot: interactive,
		AttemptID: "attempt-2", RunID: "run-1", ProjectID: "project-1", Contract: testContract(),
		OutputSchema: []byte(`{"type":"object"}`), Skill: []byte("# Test Skill\n"), Now: now, ExpiresAt: now.Add(time.Minute),
	}
	if _, err := Begin(options); errorCode(err) != "AUTOMATION_WORKSPACE_OVERLAP" {
		t.Fatalf("overlap error = %#v", err)
	}

	options.BaseDir = filepath.Join(parent, "automation")
	options.ForbiddenRoot = interactive
	first, err := Begin(options)
	if err != nil {
		t.Fatal(err)
	}
	leasePath := filepath.Join(first.Root, "lease.json")
	leaseBody, err := os.ReadFile(leasePath)
	if err != nil {
		t.Fatal(err)
	}
	var mismatched Lease
	if err := json.Unmarshal(leaseBody, &mismatched); err != nil {
		t.Fatal(err)
	}
	mismatched.RunID = "other-run"
	mismatchedBody, err := json.MarshalIndent(mismatched, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leasePath, append(mismatchedBody, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	options.Now = now.Add(2 * time.Minute)
	options.ExpiresAt = now.Add(7 * time.Minute)
	if _, err := Begin(options); errorCode(err) != "AUTOMATION_WORKSPACE_LEASE_INVALID" {
		t.Fatalf("mismatched expired lease error = %#v", err)
	}
	if err := os.WriteFile(leasePath, leaseBody, 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := Begin(options)
	if err != nil {
		t.Fatalf("recover expired attempt: %v", err)
	}
	if recovered.Root != first.Root || !recovered.Lease.StartedAt.Equal(options.Now) {
		t.Fatalf("recovered workspace = %#v", recovered)
	}
	if err := recovered.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestAttemptWorkspaceRejectsOverlapWithAnyInteractiveRoot(t *testing.T) {
	now := time.Date(2026, 7, 27, 15, 45, 0, 0, time.UTC)
	first := t.TempDir()
	second := t.TempDir()
	options := Options{
		BaseDir: filepath.Join(second, ".contentcloud", "automation"), ForbiddenRoots: []string{first, second},
		AttemptID: "attempt-multi-root", RunID: "run-1", ProjectID: "project-1", Contract: testContract(),
		OutputSchema: []byte(`{"type":"object"}`), Skill: []byte("# Test Skill\n"), Now: now, ExpiresAt: now.Add(time.Minute),
	}
	if _, err := Begin(options); errorCode(err) != "AUTOMATION_WORKSPACE_OVERLAP" {
		t.Fatalf("second workspace overlap error = %#v", err)
	}
}

func TestAttemptWorkspaceRenewsExclusiveLeaseFromServerExpiry(t *testing.T) {
	now := time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC)
	options := Options{
		BaseDir: filepath.Join(t.TempDir(), "automation"), AttemptID: "attempt-renew", RunID: "run-1", ProjectID: "project-1", Contract: testContract(),
		OutputSchema: []byte(`{"type":"object"}`), Skill: []byte("# Test Skill\n"), Now: now, ExpiresAt: now.Add(time.Minute),
	}
	workspace, err := Begin(options)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	renewedExpiry := now.Add(6 * time.Minute)
	if err := workspace.Renew(renewedExpiry); err != nil {
		t.Fatal(err)
	}
	if !workspace.Lease.ExpiresAt.Equal(renewedExpiry) {
		t.Fatalf("renewed lease expiry = %s", workspace.Lease.ExpiresAt)
	}
	options.Now = now.Add(2 * time.Minute)
	options.ExpiresAt = now.Add(7 * time.Minute)
	if _, err := Begin(options); errorCode(err) != "AUTOMATION_WORKSPACE_LEASE_ACTIVE" {
		t.Fatalf("renewed active lease error = %#v", err)
	}
}

func testContract() sourcedomain.TaskContract {
	return sourcedomain.TaskContract{
		ContractVersion: "1.0", ContractID: "snapshot-1", RunID: "run-1", TaskType: "knowledge_extract",
		Project: workspacedomain.Project{ID: "project-1"}, Sources: []sourcedomain.ContractSource{}, InputSnapshotID: "snapshot-1", OutputSchema: sourcedomain.KnowledgeCandidatesSchema,
		Capability:   catalogdomain.Capability{ID: sourcedomain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: sourcedomain.TaskContractSchema, OutputSchema: sourcedomain.KnowledgeCandidatesSchema, Digest: "sha256:" + strings.Repeat("a", 64), LocalOnly: true},
		ManifestHash: "sha256:" + strings.Repeat("c", 64),
	}
}

func errorCode(err error) string {
	var domainError *fault.Error
	if errors.As(err, &domainError) {
		return domainError.Code
	}
	return ""
}
