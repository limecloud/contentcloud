package workspace_test

import (
	"errors"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/workspace"
)

func TestBootstrapActionCatalogRejectsExecutableActions(t *testing.T) {
	for _, kind := range []string{"shell", "script"} {
		catalog := workspace.BootstrapActionCatalog{SchemaVersion: workspace.BootstrapSchemaVersion, Actions: []workspace.BootstrapAction{{ID: "unsafe", Kind: kind, Title: "unsafe", Body: "unsafe"}}}
		assertDomainCode(t, workspace.ValidateBootstrapActionCatalog(catalog), "BOOTSTRAP_ACTION_CATALOG_INVALID")
	}
	if err := workspace.ValidateBootstrapActionCatalog(workspace.BootstrapActions()); err != nil {
		t.Fatalf("built-in Action Catalog is invalid: %v", err)
	}
	for _, rawURL := range []string{"http://downloads.example.com/repair", "javascript:alert(1)"} {
		catalog := workspace.BootstrapActionCatalog{SchemaVersion: workspace.BootstrapSchemaVersion, Actions: []workspace.BootstrapAction{{ID: "unsafe-url", Kind: "open_guide", Title: "unsafe", Body: "unsafe", DocURL: rawURL}}}
		assertDomainCode(t, workspace.ValidateBootstrapActionCatalog(catalog), "BOOTSTRAP_ACTION_URL_INVALID")
	}
}

func TestBootstrapProgressFactsRejectSecretsAndAbsolutePaths(t *testing.T) {
	base := workspace.BootstrapProgressEvent{SchemaVersion: workspace.BootstrapSchemaVersion, Sequence: 1, OccurredAt: time.Now().UTC(), Stage: "prerequisites", Status: "passed", CheckID: "runtime.node.version"}
	for _, facts := range []map[string]any{
		{"version": "Bearer customer-token"},
		{"version": "/Users/customer/private"},
		{"access_token": "not-allowed"},
	} {
		event := base
		event.Facts = facts
		if err := workspace.ValidateBootstrapEvent(event); err == nil {
			t.Fatalf("unsafe facts were accepted: %#v", facts)
		}
	}
}

func TestBootstrapDiagnosticRejectsSecretsAndUnknownFields(t *testing.T) {
	base := workspace.BootstrapDiagnosticSummary{
		SchemaVersion: workspace.BootstrapSchemaVersion,
		AttemptID:     "11111111-1111-4111-8111-111111111111",
		Platform:      "darwin",
		Arch:          "arm64",
		Versions:      map[string]string{"node": "20.10.0"},
		Checks:        []workspace.BootstrapDiagnosticCheck{{CheckID: "runtime.node.version", Status: "passed"}},
	}
	secret := base
	secret.Versions = map[string]string{"node": "access_token=private"}
	assertDomainCode(t, workspace.ValidateBootstrapDiagnostic(secret), "BOOTSTRAP_DIAGNOSTIC_SECRET_DETECTED")
	unknown := base
	unknown.Versions = map[string]string{"customer_path": "value"}
	assertDomainCode(t, workspace.ValidateBootstrapDiagnostic(unknown), "BOOTSTRAP_DIAGNOSTIC_FIELD_NOT_ALLOWED")
}

func TestBootstrapProgressProjectsBrowserDecision(t *testing.T) {
	now := time.Now().UTC()
	latest := workspace.BootstrapProgressEvent{SchemaVersion: workspace.BootstrapSchemaVersion, AttemptID: "attempt", Sequence: 1, OccurredAt: now, Stage: "authorizing", Status: "needs_action", ActionID: "open.browser.authorization", Facts: map[string]any{}}
	attempt := workspace.BootstrapAttempt{ID: "attempt", State: "approved", SupportCode: "SUP-123", UserCode: "ABCD-EFGH", UpdatedAt: now}
	approved := workspace.BootstrapProgressFrom(attempt, latest)
	if approved.Status != "started" || approved.ActionID != "" || approved.UserCode != "" {
		t.Fatalf("unexpected approved projection: %#v", approved)
	}
	attempt.State = "denied"
	denied := workspace.BootstrapProgressFrom(attempt, latest)
	if denied.Status != "failed" || denied.ErrorCode != "BOOTSTRAP_AUTHORIZATION_DENIED" {
		t.Fatalf("unexpected denied projection: %#v", denied)
	}
}

func assertDomainCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainError *fault.Error
	if !errors.As(err, &domainError) || domainError.Code != code {
		t.Fatalf("error = %#v, want code %s", err, code)
	}
}
