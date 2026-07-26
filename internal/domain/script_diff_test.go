package domain_test

import (
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestValidateScriptChangeEnforcesSingleVariableAndInvariants(t *testing.T) {
	baseline := domain.ScriptPackage{Title: "A", Narrative: []string{"one"}, CreativeStrategy: domain.CreativeStrategy{CTA: "buy"}}
	candidate := baseline
	candidate.Title = "B"
	changed, report := domain.ValidateScriptChange(baseline, candidate, domain.ScriptChangeRequest{ChangeType: "variant", ChangedFields: []string{"/title"}, InvariantFields: []string{"/creative_strategy/cta"}, Hypothesis: "title changes retention", RevisionReason: "test"})
	if !report.Valid || len(changed) != 1 || changed[0] != "/title" {
		t.Fatalf("expected valid title-only change, changed=%#v report=%#v", changed, report)
	}

	candidate.Narrative = []string{"two"}
	_, report = domain.ValidateScriptChange(baseline, candidate, domain.ScriptChangeRequest{ChangeType: "variant", ChangedFields: []string{"/title"}, InvariantFields: []string{"/narrative"}, Hypothesis: "title changes retention", RevisionReason: "test"})
	if report.Valid || !hasValidationCode(report, "VARIANT_UNDECLARED_CHANGE") || !hasValidationCode(report, "SCRIPT_INVARIANT_CHANGED") {
		t.Fatalf("undeclared invariant change was accepted: %#v", report)
	}
}

func TestValidJSONPointerRejectsMalformedEscape(t *testing.T) {
	for _, pointer := range []string{"/title", "/shots/0/voiceover", "/a~1b"} {
		if !domain.ValidJSONPointer(pointer) {
			t.Fatalf("valid pointer rejected: %s", pointer)
		}
	}
	for _, pointer := range []string{"", "title", "/bad~", "/bad~2escape"} {
		if domain.ValidJSONPointer(pointer) {
			t.Fatalf("invalid pointer accepted: %s", pointer)
		}
	}
}

func hasValidationCode(report domain.ValidationReport, code string) bool {
	for _, issue := range report.Errors {
		if issue.Code == code {
			return true
		}
	}
	return false
}
