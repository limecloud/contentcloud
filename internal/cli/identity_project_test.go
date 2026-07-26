package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestIdentityProjectCommandSchemas(t *testing.T) {
	schemas := commandSchemas()
	for _, name := range []string{
		"tenant.switch", "membership.list", "membership.invite.list", "membership.invite.create",
		"membership.invite.accept", "membership.invite.revoke", "membership.update", "membership.revoke",
		"project.create", "project.update", "project.archive", "project.restore",
		"project_template.list", "project_template.create", "device.connect_session.create",
		"device.connect_session.show", "device.connect_session.cancel",
	} {
		if schemas[name] == nil {
			t.Fatalf("command schema %q is missing", name)
		}
	}
}

func TestIdentityProjectDryRunsDoNotRequireCredentials(t *testing.T) {
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	commands := [][]string{
		{"--json", "tenant", "switch", "tenant-2", "--dry-run"},
		{"--json", "team", "invite", "editor@example.com", "--role", "editor", "--dry-run"},
		{"--json", "team", "accept", "cci_example", "--dry-run"},
		{"--json", "team", "revoke-invite", "invite-1", "--dry-run"},
		{"--json", "team", "set-role", "user-1", "reviewer", "--dry-run"},
		{"--json", "team", "revoke", "user-1", "--dry-run"},
		{"--json", "project", "create", "--brand", "Brand", "--product", "Product", "--dry-run"},
		{"--json", "project", "update", "project-1", "--row-version", "1", "--brand", "New Brand", "--dry-run"},
		{"--json", "project", "archive", "project-1", "--row-version", "1", "--dry-run"},
		{"--json", "project", "templates", "create", "--name", "Douyin", "--dry-run"},
		{"--json", "device", "connect-create", "project-1", "--dry-run"},
		{"--json", "device", "connect-cancel", "connect-1", "--dry-run"},
	}
	for _, args := range commands {
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
			t.Fatalf("unexpected dry-run output for %v: %v %s", args, err, stdout.String())
		}
	}
}
