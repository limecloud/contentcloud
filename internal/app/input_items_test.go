package app_test

import (
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestInputItemTriageCreatesTaskAndUsesRowVersion(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "input-item@example.com", "long-enough-password", "输入负责人", "输入租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "输入品牌", ProductName: "输入产品"}, "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := service.CreateInputItem(ctx, actor, app.CreateInputItemInput{ProjectID: project.ID, SourceType: "brief", Title: "客户 Brief", Summary: "需要一轮短视频脚本", IdempotencyKey: "input-item-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.CreateInputItem(ctx, actor, app.CreateInputItemInput{ProjectID: project.ID, SourceType: "brief", Title: "不同标题", IdempotencyKey: "input-item-1"}, "")
	if err != nil || replay.ID != item.ID {
		t.Fatalf("idempotency replay created a second input: replay=%#v err=%v", replay, err)
	}
	otherProject, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "另一个品牌", ProductName: "另一个产品"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.TriageInputItem(ctx, actor, item.ID, app.TriageInputItemInput{Action: "create_task", ExpectedVersion: item.RowVersion, ProjectID: otherProject.ID}, ""); err == nil {
		t.Fatal("input item should not be moved to another project during task triage")
	}

	missing, err := service.TriageInputItem(ctx, actor, item.ID, app.TriageInputItemInput{Action: "mark_missing", ExpectedVersion: item.RowVersion, MissingFields: []string{"产品规格", "产品规格", "授权证明"}}, "")
	if err != nil || missing.Status != domain.InputItemNeedsInfo || len(missing.MissingFields) != 2 {
		t.Fatalf("missing-information triage failed: value=%#v err=%v", missing, err)
	}
	if _, err := service.TriageInputItem(ctx, actor, item.ID, app.TriageInputItemInput{Action: "archive", ExpectedVersion: item.RowVersion}, ""); err == nil {
		t.Fatal("stale input item version should be rejected")
	}

	created, err := service.TriageInputItem(ctx, actor, item.ID, app.TriageInputItemInput{Action: "create_task", ExpectedVersion: missing.RowVersion, ProjectID: project.ID, ContentType: domain.ContentTypeVideoScript, Title: "从 Brief 创建任务"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != domain.InputItemTaskCreated || created.TargetTaskID == "" {
		t.Fatalf("task triage did not bind a task: %#v", created)
	}
	task, err := service.WorkTask(ctx, actor, created.TargetTaskID)
	if err != nil || len(task.Task.InputRefs) != 1 || task.Task.InputRefs[0] != "input:"+item.ID {
		t.Fatalf("created task did not retain input reference: task=%#v err=%v", task, err)
	}
}
