package httpapi_test

import (
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	workbenchdomain "github.com/limecloud/contentcloud/internal/experience/workbench"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
)

func TestAdminWorkbenchRegistryEndpoints(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("operator@example.com"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	if _, err := service.Identity.Register(t.Context(), "operator@example.com", "long-enough-password", "平台运营", "运营团队"); err != nil {
		t.Fatal(err)
	}
	login := callBFF[map[string]any](t, client, http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{"email": "operator@example.com", "password": "long-enough-password"})
	if login == nil {
		t.Fatal("login did not return a response")
	}
	view := callBFF[application.WorkbenchRegistryView](t, client, http.MethodGet, server.URL+"/api/bff/admin/workbenches", nil)
	base := workbenchdomain.DefaultRegistry().Entries()[1]
	base.Manifest.ID = "operator-article-workbench"
	base.Manifest.Version = "2.0.0"
	created := callBFF[workbenchdomain.Entry](t, client, http.MethodPost, server.URL+"/api/bff/admin/workbenches", application.RegisterWorkbenchInput{Manifest: base.Manifest, TemplateAliases: base.TemplateAliases})
	if created.Status != "draft" || created.Digest == "" || len(view.Entries) == 0 {
		t.Fatalf("unexpected registry response: created=%#v initial=%#v", created, view)
	}
	updated := callBFF[workbenchdomain.Entry](t, client, http.MethodPatch, server.URL+"/api/bff/admin/workbenches/"+created.Manifest.ID+"/versions/"+created.Manifest.Version, application.UpdateWorkbenchStateInput{Status: "published", TenantIDs: []string{"tenant-a"}})
	if updated.Status != "published" || len(updated.TenantIDs) != 1 {
		t.Fatalf("unexpected updated registry entry: %#v", updated)
	}
}
