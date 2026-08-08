package projectview

import (
	"errors"
	"net/url"
	"reflect"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestContractExposesStudioSurfacesInStableOrder(t *testing.T) {
	want := []string{"home", "connect", "tasks", "assets", "knowledge", "team", "deliveries"}
	if got := IDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected Studio surface order: got=%v want=%v", got, want)
	}
	for _, id := range want {
		page, ok := Page(id)
		if !ok || page.Route == "" || page.Description == "" {
			t.Fatalf("incomplete Studio surface contract for %q: %#v", id, page)
		}
	}
}

func TestBuildCreatesCanonicalStudioLinks(t *testing.T) {
	home, err := Build("https://content.example.com/", "project-1", Target{View: "home"})
	if err != nil || home.URL != "https://content.example.com/studio?project=project-1" {
		t.Fatalf("unexpected home link: link=%#v error=%v", home, err)
	}

	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tasks, err := Build("https://content.example.com", "project-1", Target{View: "tasks", Focus: &Focus{Kind: "submission_revision", ID: "revision-1", Digest: digest}})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(tasks.URL)
	if err != nil || parsed.Path != "/studio/tasks" || parsed.Query().Get("project") != "project-1" || parsed.Query().Get("focus_kind") != "submission_revision" || parsed.Query().Get("focus_id") != "revision-1" || parsed.Query().Get("expected_digest") != digest {
		t.Fatalf("unexpected focused task link: %q", tasks.URL)
	}

	connect, err := Build("http://127.0.0.1:8080", "project-1", Target{View: "connect", Focus: &Focus{Kind: "bootstrap_attempt", ID: "attempt-1"}})
	if err != nil || connect.URL != "http://127.0.0.1:8080/studio/connect?bootstrap_attempt=attempt-1&project=project-1" {
		t.Fatalf("unexpected connect link: link=%#v error=%v", connect, err)
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
		{name: "missing server", projectID: "project-1", target: Target{View: "home"}, code: "WEB_TARGET_UNTRUSTED"},
		{name: "remote http", serverBase: "http://content.example.com", projectID: "project-1", target: Target{View: "home"}, code: "WEB_TARGET_UNTRUSTED"},
		{name: "credentials", serverBase: "https://user:secret@content.example.com", projectID: "project-1", target: Target{View: "home"}, code: "WEB_TARGET_UNTRUSTED"},
		{name: "base path", serverBase: "https://content.example.com/app", projectID: "project-1", target: Target{View: "home"}, code: "WEB_TARGET_UNTRUSTED"},
		{name: "project traversal", serverBase: "https://content.example.com", projectID: "../other", target: Target{View: "home"}, code: "PROJECT_ID_INVALID"},
		{name: "unknown view", serverBase: "https://content.example.com", projectID: "project-1", target: Target{View: "unknown"}, code: "PROJECT_VIEW_INVALID"},
		{name: "wrong focus", serverBase: "https://content.example.com", projectID: "project-1", target: Target{View: "tasks", Focus: &Focus{Kind: "bootstrap_attempt", ID: "attempt-1"}}, code: "PROJECT_FOCUS_INVALID"},
		{name: "focus traversal", serverBase: "https://content.example.com", projectID: "project-1", target: Target{View: "tasks", Focus: &Focus{Kind: "submission_revision", ID: "../revision", Digest: digest}}, code: "PROJECT_FOCUS_INVALID"},
		{name: "missing digest", serverBase: "https://content.example.com", projectID: "project-1", target: Target{View: "deliveries", Focus: &Focus{Kind: "snapshot", ID: "snapshot-1"}}, code: "PROJECT_FOCUS_DIGEST_REQUIRED"},
		{name: "short digest", serverBase: "https://content.example.com", projectID: "project-1", target: Target{View: "deliveries", Focus: &Focus{Kind: "snapshot", ID: "snapshot-1", Digest: "sha256:abc"}}, code: "PROJECT_FOCUS_DIGEST_INVALID"},
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
	if err := Validate(Target{View: "tasks", Focus: &Focus{Kind: "submission_revision", ID: "revision-1", Digest: digest}}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(Target{View: "deliveries", Focus: &Focus{Kind: "snapshot", ID: "snapshot-1"}}); err == nil {
		t.Fatal("expected digest-bound navigation validation to fail")
	}
}
