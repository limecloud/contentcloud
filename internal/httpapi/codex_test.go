package httpapi

import (
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestCodexRoutePrecedesSPAFallback(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/codex", nil)
	request.Header.Set("Accept", "text/plain")
	response := httptest.NewRecorder()
	New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), false, "").Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), codexGuideSchemaVersion) {
		t.Fatalf("/codex route fell through: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestCodexContentNegotiationUsesNavigationHeaders(t *testing.T) {
	tests := []struct {
		name        string
		accept      string
		destination string
		contentType string
		marker      string
	}{
		{name: "browser document", accept: "text/html,application/xhtml+xml", destination: "document", contentType: "text/html; charset=utf-8", marker: "<!doctype html>"},
		{name: "HTML explicitly rejected", accept: "text/html;q=0,text/plain", destination: "document", contentType: "text/plain; charset=utf-8", marker: "执行规则"},
		{name: "explicit plain text", accept: "text/plain", destination: "document", contentType: "text/plain; charset=utf-8", marker: "schema_version: contentcloud.codex-guide/1.0"},
		{name: "non-navigation client", accept: "text/html", destination: "empty", contentType: "text/plain; charset=utf-8", marker: "执行规则"},
		{name: "agent default", accept: "*/*", contentType: "text/plain; charset=utf-8", marker: "执行规则"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://contentcloud.example.com/codex", nil)
			request.Header.Set("Accept", test.accept)
			if test.destination != "" {
				request.Header.Set("Sec-Fetch-Dest", test.destination)
			}
			response := httptest.NewRecorder()
			codex(response, request)

			result := response.Result()
			defer result.Body.Close()
			body, err := io.ReadAll(result.Body)
			if err != nil {
				t.Fatal(err)
			}
			if result.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", result.StatusCode)
			}
			if got := result.Header.Get("Content-Type"); got != test.contentType {
				t.Fatalf("Content-Type = %q, want %q", got, test.contentType)
			}
			if got := result.Header.Get("Vary"); got != codexGuideVary {
				t.Fatalf("Vary = %q", got)
			}
			if !strings.Contains(string(body), test.marker) {
				t.Fatalf("response is missing %q", test.marker)
			}
		})
	}
}

func TestCodexHTMLAndTextSharePinnedGuideFacts(t *testing.T) {
	guide := newCodexGuide()
	bodyFor := func(destination, accept, userAgent string) string {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/codex", nil)
		request.Header.Set("Sec-Fetch-Dest", destination)
		request.Header.Set("Accept", accept)
		request.Header.Set("User-Agent", userAgent)
		response := httptest.NewRecorder()
		codex(response, request)
		return response.Body.String()
	}
	htmlBody := bodyFor("document", "text/html", "Mozilla/5.0")
	htmlText := html.UnescapeString(htmlBody)
	textBody := bodyFor("empty", "text/html", "arbitrary-agent/1.0")

	facts := []string{
		guide.SchemaVersion,
		guide.Version,
		guide.MarketplaceSource,
		guide.MarketplaceRef,
		guide.PluginID,
		guide.ContextTool,
		guide.DoctorTool,
		guide.OpenViewTool,
	}
	for _, step := range guide.Steps {
		facts = append(facts, step.Commands...)
	}
	for _, fact := range facts {
		if !strings.Contains(htmlText, fact) || !strings.Contains(textBody, fact) {
			t.Errorf("shared guide fact %q is missing from HTML or text", fact)
		}
	}
	if !strings.Contains(htmlBody, "Content Work OS") {
		t.Fatal("HTML guide did not render")
	}

	request := httptest.NewRequest(http.MethodGet, "/codex", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	request.Header.Set("Accept", "text/html")
	response := httptest.NewRecorder()
	request.Header.Set("User-Agent", "not-a-browser")
	codex(response, request)
	if response.Body.String() != htmlBody {
		t.Fatal("User-Agent changed content negotiation or guide content")
	}
	if policy := response.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "default-src 'none'") || !strings.Contains(policy, "frame-ancestors 'none'") {
		t.Fatalf("unexpected Content-Security-Policy %q", policy)
	}
}

func TestCodexGuideContainsNoRuntimeSecretsOrAbsolutePaths(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/codex", nil)
	request.Header.Set("Accept", "text/plain")
	response := httptest.NewRecorder()
	codex(response, request)
	body := response.Body.String()

	for _, forbidden := range []string{"token=", "/Users/", "C:\\Users\\", "@latest", "evil.example"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("guide contains forbidden runtime value %q", forbidden)
		}
	}
	if regexp.MustCompile(`(?i)(?:Bearer\s+\S+|\b(?:ct|cck|sk)[_-][A-Za-z0-9]{8,})`).MatchString(body) {
		t.Fatal("guide contains a value shaped like a runtime secret")
	}
	marketplaceCommand := "codex plugin marketplace add limecloud/contentcloud --ref v0.19.0 --json"
	if strings.Count(body, marketplaceCommand) != 1 {
		t.Fatalf("fixed Marketplace command count = %d", strings.Count(body, marketplaceCommand))
	}
	if !strings.Contains(body, "读取它不授权安装") || !strings.Contains(body, "当前对话不会热加载") {
		t.Fatal("guide is missing authorization or new-conversation boundary")
	}
}
