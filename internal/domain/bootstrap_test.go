package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestBootstrapActionCatalogRejectsExecutableActions(t *testing.T) {
	for _, kind := range []string{"shell", "script"} {
		catalog := domain.BootstrapActionCatalog{SchemaVersion: domain.BootstrapSchemaVersion, Actions: []domain.BootstrapAction{{ID: "unsafe", Kind: kind, Title: "unsafe", Body: "unsafe"}}}
		assertDomainCode(t, domain.ValidateBootstrapActionCatalog(catalog), "BOOTSTRAP_ACTION_CATALOG_INVALID")
	}
	if err := domain.ValidateBootstrapActionCatalog(domain.BootstrapActions()); err != nil {
		t.Fatalf("built-in Action Catalog is invalid: %v", err)
	}
	for _, rawURL := range []string{"http://downloads.example.com/repair", "javascript:alert(1)"} {
		catalog := domain.BootstrapActionCatalog{SchemaVersion: domain.BootstrapSchemaVersion, Actions: []domain.BootstrapAction{{ID: "unsafe-url", Kind: "open_guide", Title: "unsafe", Body: "unsafe", DocURL: rawURL}}}
		assertDomainCode(t, domain.ValidateBootstrapActionCatalog(catalog), "BOOTSTRAP_ACTION_URL_INVALID")
	}
}

func TestBootstrapProgressFactsRejectSecretsAndAbsolutePaths(t *testing.T) {
	base := domain.BootstrapProgressEvent{SchemaVersion: domain.BootstrapSchemaVersion, Sequence: 1, OccurredAt: time.Now().UTC(), Stage: "prerequisites", Status: "passed", CheckID: "runtime.node.version"}
	for _, facts := range []map[string]any{
		{"version": "Bearer customer-token"},
		{"version": "/Users/customer/private"},
		{"access_token": "not-allowed"},
	} {
		event := base
		event.Facts = facts
		if err := domain.ValidateBootstrapEvent(event); err == nil {
			t.Fatalf("unsafe facts were accepted: %#v", facts)
		}
	}
}

func TestBootstrapDiagnosticRejectsSecretsAndUnknownFields(t *testing.T) {
	base := domain.BootstrapDiagnosticSummary{
		SchemaVersion: domain.BootstrapSchemaVersion,
		AttemptID:     "11111111-1111-4111-8111-111111111111",
		Platform:      "darwin",
		Arch:          "arm64",
		Versions:      map[string]string{"node": "20.10.0"},
		Checks:        []domain.BootstrapDiagnosticCheck{{CheckID: "runtime.node.version", Status: "passed"}},
	}
	secret := base
	secret.Versions = map[string]string{"node": "access_token=private"}
	assertDomainCode(t, domain.ValidateBootstrapDiagnostic(secret), "BOOTSTRAP_DIAGNOSTIC_SECRET_DETECTED")
	unknown := base
	unknown.Versions = map[string]string{"customer_path": "value"}
	assertDomainCode(t, domain.ValidateBootstrapDiagnostic(unknown), "BOOTSTRAP_DIAGNOSTIC_FIELD_NOT_ALLOWED")
}

func TestBootstrapProgressProjectsBrowserDecision(t *testing.T) {
	now := time.Now().UTC()
	latest := domain.BootstrapProgressEvent{SchemaVersion: domain.BootstrapSchemaVersion, AttemptID: "attempt", Sequence: 1, OccurredAt: now, Stage: "authorizing", Status: "needs_action", ActionID: "open.browser.authorization", Facts: map[string]any{}}
	attempt := domain.BootstrapAttempt{ID: "attempt", State: "approved", SupportCode: "SUP-123", UserCode: "ABCD-EFGH", UpdatedAt: now}
	approved := domain.BootstrapProgressFrom(attempt, latest)
	if approved.Status != "started" || approved.ActionID != "" || approved.UserCode != "" {
		t.Fatalf("unexpected approved projection: %#v", approved)
	}
	attempt.State = "denied"
	denied := domain.BootstrapProgressFrom(attempt, latest)
	if denied.Status != "failed" || denied.ErrorCode != "BOOTSTRAP_AUTHORIZATION_DENIED" {
		t.Fatalf("unexpected denied projection: %#v", denied)
	}
}

func assertDomainCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != code {
		t.Fatalf("error = %#v, want code %s", err, code)
	}
}
