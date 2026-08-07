package projectview

import (
	"errors"
	"net/url"
	"reflect"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestContractExposesV3ProjectViewsInStableOrder(t *testing.T) {
	want := []string{"setup", "overview", "context", "knowledge", "intelligence", "strategy", "planning", "creative", "review", "delivery", "learning", "automation"}
	if got := IDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected project view order: got=%v want=%v", got, want)
	}
	for _, id := range want {
		page, ok := Page(id)
		if !ok || page.Route != id || page.Description == "" {
			t.Fatalf("incomplete page contract for %q: %#v", id, page)
		}
	}
}

func TestBuildCreatesCanonicalProjectLinks(t *testing.T) {
	overview, err := Build("https://content.example.com/", "project-1", Target{View: "overview"})
	if err != nil || overview.URL != "https://content.example.com/projects/project-1/overview" {
		t.Fatalf("unexpected overview link: link=%#v error=%v", overview, err)
	}

	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	review, err := Build("https://content.example.com", "project-1", Target{View: "review", Focus: &Focus{Kind: "submission_revision", ID: "revision-1", Digest: digest}})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(review.URL)
	if err != nil || parsed.Path != "/projects/project-1/review" || parsed.Query().Get("focus_kind") != "submission_revision" || parsed.Query().Get("focus_id") != "revision-1" || parsed.Query().Get("expected_digest") != digest {
		t.Fatalf("unexpected review link: %q", review.URL)
	}

	setup, err := Build("http://127.0.0.1:8080", "project-1", Target{View: "setup", Focus: &Focus{Kind: "bootstrap_attempt", ID: "attempt-1"}})
	if err != nil || setup.URL != "http://127.0.0.1:8080/projects/project-1/setup?bootstrap_attempt=attempt-1" {
		t.Fatalf("unexpected setup link: link=%#v error=%v", setup, err)
	}
}

func TestBuildStudioConnectCreatesCustomerConnectionLink(t *testing.T) {
	target, err := BuildStudioConnect("https://content.example.com/", "11111111-1111-4111-8111-111111111111")
	if err != nil || target != "https://content.example.com/studio/connect?session=11111111-1111-4111-8111-111111111111" {
		t.Fatalf("unexpected Studio connect link: target=%q error=%v", target, err)
	}
	if _, err := BuildStudioConnect("https://content.example.com", "../session"); err == nil {
		t.Fatal("expected invalid connection session ID to be rejected")
	}
}

func TestBuildRejectsUntrustedTargetsAndInvalidFocus(t *testing.T) {
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tests := []struct {
		name       string
		serverBase string
		projectID  string
		target     Target
		code       string
	}{
		{name: "missing server", projectID: "project-1", target: Target{View: "overview"}, code: "WEB_TARGET_UNTRUSTED"},
		{name: "remote http", serverBase: "http://content.example.com", projectID: "project-1", target: Target{View: "overview"}, code: "WEB_TARGET_UNTRUSTED"},
		{name: "credentials", serverBase: "https://user:secret@content.example.com", projectID: "project-1", target: Target{View: "overview"}, code: "WEB_TARGET_UNTRUSTED"},
		{name: "base path", serverBase: "https://content.example.com/app", projectID: "project-1", target: Target{View: "overview"}, code: "WEB_TARGET_UNTRUSTED"},
		{name: "project traversal", serverBase: "https://content.example.com", projectID: "../other", target: Target{View: "overview"}, code: "PROJECT_ID_INVALID"},
		{name: "unknown view", serverBase: "https://content.example.com", projectID: "project-1", target: Target{View: "unknown"}, code: "PROJECT_VIEW_INVALID"},
		{name: "wrong focus", serverBase: "https://content.example.com", projectID: "project-1", target: Target{View: "review", Focus: &Focus{Kind: "bootstrap_attempt", ID: "attempt-1"}}, code: "PROJECT_FOCUS_INVALID"},
		{name: "focus traversal", serverBase: "https://content.example.com", projectID: "project-1", target: Target{View: "review", Focus: &Focus{Kind: "submission_revision", ID: "../revision", Digest: digest}}, code: "PROJECT_FOCUS_INVALID"},
		{name: "missing digest", serverBase: "https://content.example.com", projectID: "project-1", target: Target{View: "review", Focus: &Focus{Kind: "submission_revision", ID: "revision-1"}}, code: "PROJECT_FOCUS_DIGEST_REQUIRED"},
		{name: "short digest", serverBase: "https://content.example.com", projectID: "project-1", target: Target{View: "review", Focus: &Focus{Kind: "submission_revision", ID: "revision-1", Digest: "sha256:abc"}}, code: "PROJECT_FOCUS_DIGEST_INVALID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(test.serverBase, test.projectID, test.target)
			var domainError *domain.Error
			if !errors.As(err, &domainError) || domainError.Code != test.code {
				t.Fatalf("unexpected error: got=%v want code=%s", err, test.code)
			}
		})
	}
}

func TestValidateChecksNavigationWithoutConstructingURL(t *testing.T) {
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := Validate(Target{View: "review", Focus: &Focus{Kind: "submission_revision", ID: "revision-1", Digest: digest}}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(Target{View: "review", Focus: &Focus{Kind: "submission_revision", ID: "revision-1"}}); err == nil {
		t.Fatal("expected digest-bound navigation validation to fail")
	}
}
