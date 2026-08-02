package httpapi_test

import (
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestInputItemBFFTriage(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	bootstrap, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.Body.Close()

	project := callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", app.CreateProjectInput{BrandName: "收件品牌", ProductName: "收件产品"})
	item := callBFF[domain.InputItem](t, client, http.MethodPost, server.URL+"/api/bff/input-items", app.CreateInputItemInput{ProjectID: project.ID, SourceType: "conversation_bundle", Title: "本地摘要", Summary: "已确认的任务摘要", Disclosure: "project", IdempotencyKey: "http-input-1"})
	items := callBFF[[]domain.InputItem](t, client, http.MethodGet, server.URL+"/api/bff/input-items?status=untriaged", nil)
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("input item list did not return created item: %#v", items)
	}
	item = callBFF[domain.InputItem](t, client, http.MethodPost, server.URL+"/api/bff/input-items/"+item.ID+"/triage", app.TriageInputItemInput{Action: "create_task", ExpectedVersion: item.RowVersion, ProjectID: project.ID, ContentType: domain.ContentTypeVideoScript})
	if item.Status != domain.InputItemTaskCreated || item.TargetTaskID == "" {
		t.Fatalf("input item was not routed to a task: %#v", item)
	}
	created := callBFF[app.WorkTaskView](t, client, http.MethodGet, server.URL+"/api/bff/tasks/"+item.TargetTaskID, nil)
	if len(created.Task.InputRefs) != 1 || created.Task.InputRefs[0] != "input:"+item.ID {
		t.Fatalf("task did not retain input reference: %#v", created.Task)
	}
}
