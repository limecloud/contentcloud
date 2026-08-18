package application_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	sourceinfra "github.com/limecloud/contentcloud/internal/integration/provider/source"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

type fixtureSearchProvider struct{}

func (fixtureSearchProvider) Search(context.Context, string, int) ([]sourceinfra.SearchResult, error) {
	return []sourceinfra.SearchResult{{Title: "Fixture", URL: "https://example.com/found", Snippet: "evidence", Rank: 1}}, nil
}

func TestSearchSourcesUsesProviderAndProjectScope(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), nil, application.WithSourceSearchProvider(fixtureSearchProvider{}))
	session, err := service.Identity.Register(t.Context(), "source-search@example.com", "long-enough-password", "Source", "Source Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Source.SearchSources(t.Context(), actor, application.SearchSourcesInput{ProjectID: project.ID, Query: "fixture", Limit: 5}, "req-search")
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
	service := application.New(application.DependenciesFrom(memory.New()), nil, application.WithSourceFetcher(fetcher))
	session, err := service.Identity.Register(t.Context(), "source-fetch@example.com", "long-enough-password", "Source", "Source Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	input := application.FetchSourceInput{ProjectID: project.ID, URL: server.URL + "/article"}
	first, err := service.Source.FetchSource(t.Context(), actor, input, "req-fetch-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Reused || first.Revision.ProcessingStatus != "pending" || first.Digest == "" {
		t.Fatalf("unexpected first receipt %#v", first)
	}
	second, err := service.Source.FetchSource(t.Context(), actor, input, "req-fetch-2")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Reused || second.Revision.ID != first.Revision.ID {
		t.Fatalf("fetch retry must reuse revision: first=%#v second=%#v", first, second)
	}
}
