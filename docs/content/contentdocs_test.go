package contentdocs

import (
	"errors"
	"strings"
	"testing"
)

func TestCatalogMergesAgentRegistryAndContentDocumentation(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if catalog.SchemaVersion != SchemaVersion || len(catalog.Clients) != 6 || len(catalog.ContentTypes) != 2 || len(catalog.Guides) != 2 {
		t.Fatalf("unexpected documentation catalog: %#v", catalog)
	}
	status := map[string]Status{}
	for _, client := range catalog.Clients {
		status[client.ID] = client.Status
		if client.PageSlug != "clients/"+client.ID || len(client.Capabilities) != 5 {
			t.Fatalf("incomplete client documentation: %#v", client)
		}
	}
	if status["codex"] != StatusAvailable || status["claude-code"] != StatusLimited || status["cursor"] != StatusPlanned {
		t.Fatalf("client status did not follow registry: %#v", status)
	}
	if catalog.ContentTypes[1].ID != "wechat-article" || catalog.ContentTypes[1].Status != StatusAvailable {
		t.Fatalf("WeChat article documentation must follow the implemented capability: %#v", catalog.ContentTypes[1])
	}
}

func TestPagesLoadExplicitAndGeneratedMarkdown(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range catalog.Pages {
		page, pageErr := LoadPage(summary.Slug)
		if pageErr != nil || page.Slug != summary.Slug {
			t.Fatalf("catalog page %q is not loadable: page=%#v err=%v", summary.Slug, page, pageErr)
		}
	}

	codex, err := LoadPage("clients/codex")
	if err != nil || codex.Status != StatusAvailable || !strings.Contains(codex.Markdown, "workspace_context") {
		t.Fatalf("unexpected Codex page: %#v err=%v", codex, err)
	}
	cursor, err := LoadPage("clients/cursor")
	if err != nil || cursor.Status != StatusPlanned || !strings.Contains(cursor.Markdown, "不提供安装") {
		t.Fatalf("unexpected generated Cursor page: %#v err=%v", cursor, err)
	}
}

func TestInternalAndUnsafePagesAreNeverExposed(t *testing.T) {
	for _, slug := range []string{"internal/multi-content-expansion", "../catalog", "clients/unknown", "clients/codex/extra", ""} {
		_, err := LoadPage(slug)
		if !errors.Is(err, ErrPageNotFound) {
			t.Fatalf("page %q returned %v, want ErrPageNotFound", slug, err)
		}
	}
}
