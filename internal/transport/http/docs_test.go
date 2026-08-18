package httpapi_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
)

func TestPublicDocumentationCatalogAndPages(t *testing.T) {
	server := httptest.NewServer(httpapi.New(application.New(application.DependenciesFrom(memory.New()), slog.Default()), slog.Default(), false, "").Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/api/docs/catalog")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("X-Robots-Tag") != "noindex, nofollow" {
		t.Fatalf("catalog status=%d headers=%v", response.StatusCode, response.Header)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			SchemaVersion string `json:"schema_version"`
			Clients       []any  `json:"clients"`
			ContentTypes  []any  `json:"content_types"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.SchemaVersion != "contentcloud.docs-catalog/1.0" || len(envelope.Data.Clients) != 6 || len(envelope.Data.ContentTypes) != 2 {
		t.Fatalf("unexpected catalog envelope: %#v", envelope)
	}

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/docs/pages/clients/codex", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "text/markdown")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/markdown") || !strings.Contains(string(body), "workspace_context") {
		t.Fatalf("markdown page status=%d content-type=%q body=%s", response.StatusCode, response.Header.Get("Content-Type"), body)
	}
}

func TestDocumentationDoesNotExposeInternalPages(t *testing.T) {
	server := httptest.NewServer(httpapi.New(application.New(application.DependenciesFrom(memory.New()), slog.Default()), slog.Default(), false, "").Handler())
	defer server.Close()

	for _, path := range []string{"/api/docs/pages/internal/multi-content-expansion", "/api/docs/pages/clients/unknown"} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s status=%d, want 404", path, response.StatusCode)
		}
	}
}
