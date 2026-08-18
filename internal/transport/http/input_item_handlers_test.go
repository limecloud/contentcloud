package httpapi_test

import (
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestInputItemBFFTriage(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.Body.Close()

	project := callBFF[workspacedomain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", application.CreateProjectInput{BrandName: "收件品牌", ProductName: "收件产品", ContentType: identitydomain.ContentTypeVideoScript})
	item := callBFF[work.InputItem](t, client, http.MethodPost, server.URL+"/api/bff/input-items", application.CreateInputItemInput{ProjectID: project.ID, SourceType: "conversation_bundle", Title: "本地摘要", Summary: "已确认的任务摘要", Disclosure: "project", IdempotencyKey: "http-input-1"})
	items := callBFF[[]work.InputItem](t, client, http.MethodGet, server.URL+"/api/bff/input-items?status=untriaged", nil)
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("input item list did not return created item: %#v", items)
	}
	item = callBFF[work.InputItem](t, client, http.MethodPost, server.URL+"/api/bff/input-items/"+item.ID+"/triage", application.TriageInputItemInput{Action: "create_task", ExpectedVersion: item.RowVersion, ProjectID: project.ID, ContentType: identitydomain.ContentTypeVideoScript})
	if item.Status != work.InputItemTaskCreated || item.TargetTaskID == "" {
		t.Fatalf("input item was not routed to a task: %#v", item)
	}
	created := callBFF[application.WorkTaskView](t, client, http.MethodGet, server.URL+"/api/bff/tasks/"+item.TargetTaskID, nil)
	if len(created.Task.InputRefs) != 1 || created.Task.InputRefs[0] != "input:"+item.ID {
		t.Fatalf("task did not retain input reference: %#v", created.Task)
	}
}
