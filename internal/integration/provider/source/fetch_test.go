package sourceinfra

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetcherAllowsExplicitFixtureHostAndReturnsDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>Hello &amp; world</body></html>"))
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	fetcher := &Fetcher{AllowedHosts: []string{host}, Client: server.Client(), MaxBytes: 1024}
	result, err := fetcher.Fetch(context.Background(), server.URL+"/article")
	if err != nil {
		t.Fatal(err)
	}
	if result.MIME != "text/html" || result.FinalURL != server.URL+"/article" || result.Digest == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestFetcherRejectsPrivateHostWithoutAllowlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	_, err := (&Fetcher{Client: server.Client()}).Fetch(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "私有") {
		t.Fatalf("expected private host rejection, got %v", err)
	}
}

func TestFetcherRejectsRedirectOutsideAllowlist(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("secret")) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	allowed := strings.TrimPrefix(redirect.URL, "http://")
	_, err := (&Fetcher{AllowedHosts: []string{allowed}, Client: redirect.Client()}).Fetch(context.Background(), redirect.URL)
	if err == nil || !strings.Contains(err.Error(), "私有") {
		t.Fatalf("expected redirect rejection, got %v", err)
	}
}

func TestFetcherRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("0123456789")) }))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	_, err := (&Fetcher{AllowedHosts: []string{host}, Client: server.Client(), MaxBytes: 5}).Fetch(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "maximum size") {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

func TestURLSearchProviderSupportsCredentialFreeIntake(t *testing.T) {
	provider := NewDefaultSearchProvider()
	results, err := provider.Search(context.Background(), "https://example.com/brief", 5)
	if err != nil || len(results) != 1 || results[0].URL != "https://example.com/brief" {
		t.Fatalf("unexpected results %#v, err=%v", results, err)
	}
}

func TestFetcherRespectsRobotsAndRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /private\nAllow: /private/public\n"))
			return
		}
		_, _ = w.Write([]byte("public"))
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	fetcher := &Fetcher{AllowedHosts: []string{host}, Client: server.Client(), RespectRobots: true, MinInterval: time.Hour}
	if _, err := fetcher.Fetch(context.Background(), server.URL+"/private"); err == nil || !strings.Contains(err.Error(), "robots.txt") {
		t.Fatalf("expected robots rejection, got %v", err)
	}
	// A blocked request still consumes the host rate slot so callers cannot
	// hammer robots.txt or probe paths at unlimited frequency.
	if _, err := fetcher.Fetch(context.Background(), server.URL+"/private/public"); err == nil || !strings.Contains(err.Error(), "频繁") {
		t.Fatalf("expected rate rejection, got %v", err)
	}
}
