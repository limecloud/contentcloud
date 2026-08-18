package application_test

import (
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/work"
)

func TestInputItemTriageCreatesTaskAndUsesRowVersion(t *testing.T) {
	ctx := t.Context()
	service := application.New(application.DependenciesFrom(memory.New()), nil)
	session, err := service.Identity.Register(ctx, "input-item@example.com", "long-enough-password", "输入负责人", "输入租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "输入品牌", ProductName: "输入产品", ContentType: identitydomain.ContentTypeVideoScript}, "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := service.Work.CreateInputItem(ctx, actor, application.CreateInputItemInput{ProjectID: project.ID, SourceType: "brief", Title: "客户 Brief", Summary: "需要一轮短视频脚本", IdempotencyKey: "input-item-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Work.CreateInputItem(ctx, actor, application.CreateInputItemInput{ProjectID: project.ID, SourceType: "brief", Title: "不同标题", IdempotencyKey: "input-item-1"}, "")
	if err != nil || replay.ID != item.ID {
		t.Fatalf("idempotency replay created a second input: replay=%#v err=%v", replay, err)
	}
	otherProject, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "另一个品牌", ProductName: "另一个产品"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Work.TriageInputItem(ctx, actor, item.ID, application.TriageInputItemInput{Action: "create_task", ExpectedVersion: item.RowVersion, ProjectID: otherProject.ID}, ""); err == nil {
		t.Fatal("input item should not be moved to another project during task triage")
	}

	missing, err := service.Work.TriageInputItem(ctx, actor, item.ID, application.TriageInputItemInput{Action: "mark_missing", ExpectedVersion: item.RowVersion, MissingFields: []string{"产品规格", "产品规格", "授权证明"}}, "")
	if err != nil || missing.Status != work.InputItemNeedsInfo || len(missing.MissingFields) != 2 {
		t.Fatalf("missing-information triage failed: value=%#v err=%v", missing, err)
	}
	if _, err := service.Work.TriageInputItem(ctx, actor, item.ID, application.TriageInputItemInput{Action: "archive", ExpectedVersion: item.RowVersion}, ""); err == nil {
		t.Fatal("stale input item version should be rejected")
	}

	created, err := service.Work.TriageInputItem(ctx, actor, item.ID, application.TriageInputItemInput{Action: "create_task", ExpectedVersion: missing.RowVersion, ProjectID: project.ID, ContentType: identitydomain.ContentTypeVideoScript, Title: "从 Brief 创建任务"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != work.InputItemTaskCreated || created.TargetTaskID == "" {
		t.Fatalf("task triage did not bind a task: %#v", created)
	}
	task, err := service.Work.WorkTask(ctx, actor, created.TargetTaskID)
	if err != nil || len(task.Task.InputRefs) != 1 || task.Task.InputRefs[0] != "input:"+item.ID {
		t.Fatalf("created task did not retain input reference: task=%#v err=%v", task, err)
	}

	manual, err := service.Work.CreateWorkTask(ctx, actor, application.CreateWorkTaskInput{ProjectID: project.ID, Title: "先创建再补充灵感", ContentType: identitydomain.ContentTypeVideoScript}, "")
	if err != nil || manual.Task.Status != work.TaskStatusNeedsInput {
		t.Fatalf("task without input should wait for input: task=%#v err=%v", manual.Task, err)
	}
	inspiration, err := service.Work.CreateInputItem(ctx, actor, application.CreateInputItemInput{ProjectID: project.ID, SourceType: "manual_inspiration", Title: "人物灵感", Summary: "真实主理人的一天"}, "")
	if err != nil {
		t.Fatal(err)
	}
	merged, err := service.Work.TriageInputItem(ctx, actor, inspiration.ID, application.TriageInputItemInput{Action: "merge_task", ExpectedVersion: inspiration.RowVersion, TaskID: manual.Task.ID}, "")
	if err != nil || merged.Status != work.InputItemTaskMerged {
		t.Fatalf("input was not merged into task: input=%#v err=%v", merged, err)
	}
	ready, err := service.Work.WorkTask(ctx, actor, manual.Task.ID)
	if err != nil || ready.Task.Status != work.TaskStatusReady || ready.Task.NextAction != "开始第一个流程阶段" || len(ready.Task.InputRefs) != 1 {
		t.Fatalf("merged input should make task ready: task=%#v err=%v", ready.Task, err)
	}
}
