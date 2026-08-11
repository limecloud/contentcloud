package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/sourceinfra"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

type fixtureSearchProvider struct{}

func (fixtureSearchProvider) Search(context.Context, string, int) ([]sourceinfra.SearchResult, error) {
	return []sourceinfra.SearchResult{{Title: "Fixture", URL: "https://example.com/found", Snippet: "evidence", Rank: 1}}, nil
}

func TestSearchSourcesUsesProviderAndProjectScope(t *testing.T) {
	service := app.New(memory.New(), nil, app.WithSourceSearchProvider(fixtureSearchProvider{}))
	session, err := service.Register(t.Context(), "source-search@example.com", "long-enough-password", "Source", "Source Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.SearchSources(t.Context(), actor, app.SearchSourcesInput{ProjectID: project.ID, Query: "fixture", Limit: 5}, "req-search")
	if err != nil || len(result.Results) != 1 || result.SearchKey == "" {
		t.Fatalf("unexpected result %#v, err=%v", result, err)
	}
}

func TestFetchSourcePersistsAndReusesDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<article>fixed source</article>"))
	}))
	defer server.Close()
	fetcher := &sourceinfra.Fetcher{AllowedHosts: []string{strings.TrimPrefix(server.URL, "http://")}, Client: server.Client(), MaxBytes: 1024}
	service := app.New(memory.New(), nil, app.WithSourceFetcher(fetcher))
	session, err := service.Register(t.Context(), "source-fetch@example.com", "long-enough-password", "Source", "Source Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	input := app.FetchSourceInput{ProjectID: project.ID, URL: server.URL + "/article"}
	first, err := service.FetchSource(t.Context(), actor, input, "req-fetch-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Reused || first.Revision.ProcessingStatus != "pending" || first.Digest == "" {
		t.Fatalf("unexpected first receipt %#v", first)
	}
	second, err := service.FetchSource(t.Context(), actor, input, "req-fetch-2")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Reused || second.Revision.ID != first.Revision.ID {
		t.Fatalf("fetch retry must reuse revision: first=%#v second=%#v", first, second)
	}
}
