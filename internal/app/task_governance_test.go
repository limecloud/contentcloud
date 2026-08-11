package app_test

import (
	"encoding/json"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestTaskLifecycleRevisionAndDelivery(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "task-owner@example.com", "long-enough-password", "任务负责人", "任务团队")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "测试品牌", ProductName: "测试产品", ContentType: domain.ContentTypeVideoScript}, "")
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.CreateWorkTask(ctx, actor, app.CreateWorkTaskInput{ProjectID: project.ID, Title: "完成一条可交付脚本", ContentType: domain.ContentTypeVideoScript, InputRefs: []string{"brief:test"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.TaskAction(ctx, actor, view.Task.ID, app.TaskActionInput{Action: "start"}, "")
	if err != nil || view.Task.Status != domain.TaskStatusRunning || len(view.Runs) != 1 {
		t.Fatalf("start did not create running task run: view=%#v err=%v", view, err)
	}
	for view.Task.Status == domain.TaskStatusRunning {
		stageID := view.Task.CurrentStageID
		stageRunID := ""
		for _, stageRun := range view.StageRuns {
			if stageRun.StageID == stageID {
				stageRunID = stageRun.ID
			}
		}
		view, err = service.ReportStage(ctx, actor, view.Task.ID, app.StageReportInput{StageRunID: stageRunID, StageID: stageID, Status: domain.StageRunStatusCompleted, OutputRefs: []string{"output:" + stageID}, Checks: map[string]any{"passed": true, "content.schema": true, "claim.references": true, "rights.references": true}}, "")
		if err != nil {
			t.Fatal(err)
		}
		if view.Task.Status == domain.TaskStatusReady {
			view, err = service.TaskAction(ctx, actor, view.Task.ID, app.TaskActionInput{Action: "start"}, "")
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if view.Task.Status != domain.TaskStatusAccepted {
		t.Fatalf("final stage should accept task, got %#v", view.Task)
	}
	content, _ := json.Marshal(map[string]any{"title": "测试脚本", "scenes": []any{map[string]any{"scene": 1, "voiceover": "开场"}}})
	revision, err := service.CreateTaskRevision(ctx, actor, view.Task.ID, app.CreateTaskRevisionInput{ContentType: domain.ContentTypeVideoScript, Content: content}, "")
	if err != nil || revision.Status != domain.TaskRevisionAccepted {
		t.Fatalf("revision should be accepted after final stage: %#v err=%v", revision, err)
	}
	delivery, err := service.CreateTaskDelivery(ctx, actor, view.Task.ID, app.CreateTaskDeliveryInput{RevisionID: revision.ID, Destination: "workspace"}, "")
	if err != nil || delivery.Status != domain.TaskDeliveryReady || delivery.IntegrityStatus != "script_only" {
		t.Fatalf("delivery must default to an explicit ready state: %#v err=%v", delivery, err)
	}
	view, err = service.WorkTask(ctx, actor, view.Task.ID)
	if err != nil || view.Task.Status != domain.TaskStatusAccepted || len(view.Deliveries) != 1 {
		t.Fatalf("task projection did not reflect delivery: view=%#v err=%v", view, err)
	}
}

func TestClientDecisionGateRequiresClientApprover(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "client-gate-owner@example.com", "long-enough-password", "流程负责人", "客户 Gate 租户")
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSOP(ctx, owner, app.CreateSOPInput{Name: "客户确认流程", ContentTypes: []string{domain.ContentTypeVideoScript}, Stages: []domain.StageDefinition{{ID: "confirm", Name: "客户确认", Order: 10, OutputSchema: "contentcloud.video_script/1.0", ExecutionModes: []string{"local"}, GateIDs: []string{"client_confirm"}}}, Gates: []domain.GateDefinition{{ID: "client_confirm", Name: "客户确认", Mode: domain.GateModeClientDecision, Blocking: true, AssigneeRoles: []string{"client_approver"}}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.PublishSOP(ctx, owner, created.Definition.ID, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	environment, err := service.CreateEnvironment(ctx, owner, app.SaveEnvironmentInput{Name: "客户确认环境", Slug: "client-gate", Status: "active", DefaultSOPID: published.SOPID, DefaultSOPVersion: published.Version}, "")
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, owner, app.CreateProjectInput{BrandName: "客户品牌", ProductName: "客户产品", ContentType: domain.ContentTypeVideoScript}, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateWorkTask(ctx, owner, app.CreateWorkTaskInput{ProjectID: project.ID, Title: "等待客户确认", ContentType: domain.ContentTypeVideoScript, InputRefs: []string{"brief:client"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if task.Task.EnvironmentID != environment.ID {
		t.Fatalf("task did not use client gate environment: %#v", task.Task)
	}
	task, err = service.TaskAction(ctx, owner, task.Task.ID, app.TaskActionInput{Action: "start"}, "")
	if err != nil {
		t.Fatal(err)
	}
	stage := task.StageRuns[0]
	task, err = service.ReportStage(ctx, owner, task.Task.ID, app.StageReportInput{StageRunID: stage.ID, StageID: stage.StageID, Status: domain.StageRunStatusCompleted, OutputRefs: []string{"output:client"}}, "")
	if err != nil || task.Task.Status != domain.TaskStatusWaitingGate || len(task.Gates) != 1 {
		t.Fatalf("client decision gate was not opened: task=%#v err=%v", task.Task, err)
	}
	reviewer := app.Actor{UserID: domain.NewID(), TenantID: owner.TenantID, Role: "reviewer", Type: "user"}
	if _, err := service.DecideGate(ctx, reviewer, task.Task.ID, task.Gates[0].GateID, app.GateDecisionInput{Decision: "approved"}, ""); err == nil {
		t.Fatal("ordinary project owner must not bypass client_decision Gate")
	}
	client := app.Actor{UserID: domain.NewID(), TenantID: owner.TenantID, Role: "client_approver", Type: "user"}
	decided, err := service.DecideGate(ctx, client, task.Task.ID, task.Gates[0].GateID, app.GateDecisionInput{Decision: "approved", Reason: "客户确认通过"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if decided.Task.Status != domain.TaskStatusAccepted {
		t.Fatalf("client decision did not advance task: %#v", decided.Task)
	}
}
