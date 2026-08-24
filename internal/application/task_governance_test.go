package application_test

import (
	"encoding/json"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"github.com/limecloud/contentcloud/internal/work"
)

func TestTaskLifecycleRevisionAndDelivery(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), nil)
	session, err := service.Identity.Register(ctx, "task-owner@example.com", "long-enough-password", "任务负责人", "任务团队")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "测试品牌", ProductName: "测试产品", ContentType: identitydomain.ContentTypeVideoScript}, "")
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.Work.CreateWorkTask(ctx, actor, application.CreateWorkTaskInput{ProjectID: project.ID, Title: "完成一条可交付脚本", ContentType: identitydomain.ContentTypeVideoScript, InputRefs: []string{"brief:test"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.Work.TaskAction(ctx, actor, view.Task.ID, application.TaskActionInput{Action: "start"}, "")
	if err != nil || view.Task.Status != work.TaskStatusRunning || len(view.Runs) != 1 {
		t.Fatalf("start did not create running task run: view=%#v err=%v", view, err)
	}
	for view.Task.Status == work.TaskStatusRunning {
		stageID := view.Task.CurrentStageID
		stageRunID := ""
		for _, stageRun := range view.StageRuns {
			if stageRun.StageID == stageID {
				stageRunID = stageRun.ID
			}
		}
		view, err = service.Work.ReportStage(ctx, actor, view.Task.ID, application.StageReportInput{StageRunID: stageRunID, StageID: stageID, Status: work.StageRunStatusCompleted, OutputRefs: []string{"output:" + stageID}, Checks: map[string]any{"passed": true, "content.schema": true, "claim.references": true, "rights.references": true}}, "")
		if err != nil {
			t.Fatal(err)
		}
		if view.Task.Status == work.TaskStatusReady {
			view, err = service.Work.TaskAction(ctx, actor, view.Task.ID, application.TaskActionInput{Action: "start"}, "")
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if view.Task.Status != work.TaskStatusAccepted {
		t.Fatalf("final stage should accept task, got %#v", view.Task)
	}
	content, _ := json.Marshal(map[string]any{"title": "测试脚本", "scenes": []any{map[string]any{"scene": 1, "voiceover": "开场"}}})
	revision, err := service.Work.CreateTaskRevision(ctx, actor, view.Task.ID, application.CreateTaskRevisionInput{ContentType: identitydomain.ContentTypeVideoScript, Content: content}, "")
	if err != nil || revision.Status != reviewdomain.TaskRevisionAccepted {
		t.Fatalf("revision should be accepted after final stage: %#v err=%v", revision, err)
	}
	submissions, err := store.Submissions(ctx, actor.TenantID, project.ID)
	if err != nil || len(submissions) != 1 || submissions[0].SubmissionType != "content_batch" {
		t.Fatalf("video script compatibility entrypoint did not create a content submission: %#v err=%v", submissions, err)
	}
	snapshots, err := store.ApprovedSnapshots(ctx, actor.TenantID, project.ID, "content_batch")
	if err != nil || len(snapshots) != 1 || snapshots[0].SubmissionRevisionID != revision.ID {
		t.Fatalf("accepted task did not lock an ApprovedSnapshot: %#v err=%v", snapshots, err)
	}
	packageValue, err := service.Review.CreateDeliveryPackage(ctx, actor, snapshots[0].ID, "video-script:"+view.Task.ID, "video-script-delivery")
	if err != nil || packageValue.Status != "ready" || len(packageValue.Manifest) != 3 {
		t.Fatalf("video script ApprovedSnapshot did not produce a complete delivery package: %#v err=%v", packageValue, err)
	}
	performance, err := service.Performance.ImportPerformanceObservations(ctx, actor, application.ImportPerformanceInput{ProjectID: project.ID, SourceName: "video-script-results", SourceFormat: "json", Observations: []application.CreateObservationInput{{ApprovedSnapshotID: snapshots[0].ID, Platform: "douyin", AccountAlias: "main", PublishedAt: snapshots[0].CreatedAt, WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"impressions": 100}}}}, "video-script-performance")
	if err != nil || len(performance.Observations) != 1 || performance.Observations[0].ApprovedSnapshotID != snapshots[0].ID {
		t.Fatalf("video script delivery did not enter the performance fact chain: %#v err=%v", performance, err)
	}
	rating, err := service.Performance.CreateRatingDecision(ctx, actor, application.CreateRatingDecisionInput{ProjectID: project.ID, SubjectType: "approved_snapshot", SubjectID: snapshots[0].ID, ObservationIDs: []string{performance.Observations[0].ID}, Rating: "seed_candidate", Reason: "统一主链回归", NextAction: "生成一个受控变体"}, "video-script-rating")
	if err != nil || rating.Decision.SubjectID != snapshots[0].ID {
		t.Fatalf("video script performance did not produce a governed rating: %#v err=%v", rating, err)
	}
	delivery, err := service.Work.CreateTaskDelivery(ctx, actor, view.Task.ID, application.CreateTaskDeliveryInput{RevisionID: revision.ID, Destination: "workspace"}, "")
	if err != nil || delivery.Status != deliverydomain.TaskDeliveryReady || delivery.IntegrityStatus != "script_only" {
		t.Fatalf("delivery must default to an explicit ready state: %#v err=%v", delivery, err)
	}
	view, err = service.Work.WorkTask(ctx, actor, view.Task.ID)
	if err != nil || view.Task.Status != work.TaskStatusAccepted || len(view.Deliveries) != 1 {
		t.Fatalf("task projection did not reflect delivery: view=%#v err=%v", view, err)
	}
}

func TestClientDecisionGateRequiresClientApprover(t *testing.T) {
	ctx := t.Context()
	service := application.New(application.DependenciesFrom(memory.New()), nil)
	session, err := service.Identity.Register(ctx, "client-gate-owner@example.com", "long-enough-password", "流程负责人", "客户 Gate 租户")
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Work.CreateSOP(ctx, owner, application.CreateSOPInput{Name: "客户确认流程", ContentTypes: []string{identitydomain.ContentTypeVideoScript}, Stages: []catalogdomain.StageDefinition{{ID: "confirm", Name: "客户确认", Order: 10, OutputSchema: "contentcloud.video_script/1.0", ExecutionModes: []string{"local"}, GateIDs: []string{"client_confirm"}}}, Gates: []catalogdomain.GateDefinition{{ID: "client_confirm", Name: "客户确认", Mode: catalogdomain.GateModeClientDecision, Blocking: true, AssigneeRoles: []string{"client_approver"}}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.Work.PublishSOP(ctx, owner, created.Definition.ID, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	environment, err := service.Work.CreateEnvironment(ctx, owner, application.SaveEnvironmentInput{Name: "客户确认环境", Slug: "client-gate", Status: "active", DefaultSOPID: published.SOPID, DefaultSOPVersion: published.Version}, "")
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, owner, application.CreateProjectInput{BrandName: "客户品牌", ProductName: "客户产品", ContentType: identitydomain.ContentTypeVideoScript}, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.Work.CreateWorkTask(ctx, owner, application.CreateWorkTaskInput{ProjectID: project.ID, Title: "等待客户确认", ContentType: identitydomain.ContentTypeVideoScript, InputRefs: []string{"brief:client"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if task.Task.EnvironmentID != environment.ID {
		t.Fatalf("task did not use client gate environment: %#v", task.Task)
	}
	task, err = service.Work.TaskAction(ctx, owner, task.Task.ID, application.TaskActionInput{Action: "start"}, "")
	if err != nil {
		t.Fatal(err)
	}
	stage := task.StageRuns[0]
	task, err = service.Work.ReportStage(ctx, owner, task.Task.ID, application.StageReportInput{StageRunID: stage.ID, StageID: stage.StageID, Status: work.StageRunStatusCompleted, OutputRefs: []string{"output:client"}}, "")
	if err != nil || task.Task.Status != work.TaskStatusWaitingGate || len(task.Gates) != 1 {
		t.Fatalf("client decision gate was not opened: task=%#v err=%v", task.Task, err)
	}
	reviewer := application.Actor{UserID: idgen.New(), TenantID: owner.TenantID, Role: "reviewer", Type: "user"}
	if _, err := service.Work.DecideGate(ctx, reviewer, task.Task.ID, task.Gates[0].GateID, application.GateDecisionInput{Decision: "approved"}, ""); err == nil {
		t.Fatal("ordinary project owner must not bypass client_decision Gate")
	}
	client := application.Actor{UserID: idgen.New(), TenantID: owner.TenantID, Role: "client_approver", Type: "user"}
	decided, err := service.Work.DecideGate(ctx, client, task.Task.ID, task.Gates[0].GateID, application.GateDecisionInput{Decision: "approved", Reason: "客户确认通过"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if decided.Task.Status != work.TaskStatusAccepted {
		t.Fatalf("client decision did not advance task: %#v", decided.Task)
	}
}
