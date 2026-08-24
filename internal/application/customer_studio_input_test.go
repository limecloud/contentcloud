package application_test

import (
	"log/slog"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/testsupport"
	"github.com/limecloud/contentcloud/internal/work"
)

func TestCustomerStudioInputChangeRebindsRuntimeDigest(t *testing.T) {
	service, actor, projectID, experience, _ := customerStudioInputFixture(t)
	created, err := service.Work.CreateCustomerStudioTask(t.Context(), actor, application.StudioCreateTaskInput{
		ExperienceID: experience.ID, ProjectID: projectID, Title: "输入版本测试", Goal: "验证新增资料会重新编排 Runtime",
		BusinessBrief: application.StudioBusinessBrief{Audience: "新客户", Channel: "抖音", Tone: "真实", Keywords: []string{"资料", "版本"}},
	}, "studio-input-create")
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := service.Runtime.Runtime().Jobs(t.Context(), actor.TenantID, created.Task.ID)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("initial Runtime job count = %d, err=%v", len(jobs), err)
	}
	createdStored, err := service.Work.WorkTask(t.Context(), actor, created.Task.ID)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := service.Work.AddCustomerStudioInspiration(t.Context(), actor, created.Task.ID, application.StudioAddInspirationInput{
		Title: "补充资料", Body: "这是在 Runtime 尚未开始执行时补充的资料。",
	}, "studio-input-append")
	if err != nil {
		t.Fatal(err)
	}
	updatedStored, err := service.Work.WorkTask(t.Context(), actor, updated.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updatedStored.Task.InputRefs) <= len(createdStored.Task.InputRefs) {
		t.Fatalf("input refs did not grow: before=%v after=%v", createdStored.Task.InputRefs, updatedStored.Task.InputRefs)
	}
	jobs, err = service.Runtime.Runtime().Jobs(t.Context(), actor.TenantID, created.Task.ID)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("rebound Runtime job count = %d, err=%v", len(jobs), err)
	}
	var activeJob, cancelledJob contentruntime.JobRun
	for _, job := range jobs {
		if job.State == contentruntime.JobRunCancelled {
			cancelledJob = job
		} else {
			activeJob = job
		}
	}
	if cancelledJob.ID == "" || activeJob.ID == "" {
		t.Fatalf("input change did not replace the queued Runtime job: %#v", jobs)
	}
	latest := activeJob
	inputDigest, err := stablehash.Sum(struct {
		ContentType     string         `json:"content_type"`
		InputRefs       []string       `json:"input_refs"`
		RequestedOutput map[string]any `json:"requested_output"`
	}{updatedStored.Task.ContentType, updatedStored.Task.InputRefs, updatedStored.Task.RequestedOutput})
	if err != nil {
		t.Fatal(err)
	}
	if latest.InputDigest != "sha256:"+inputDigest {
		t.Fatalf("Runtime input digest = %q, want sha256:%s", latest.InputDigest, inputDigest)
	}

	if _, err := service.Work.CustomerStudioTaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "start"}, "studio-input-start"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Work.AddCustomerStudioInspiration(t.Context(), actor, created.Task.ID, application.StudioAddInspirationInput{Title: "迟到资料", Body: "不能在执行中追加"}, "studio-input-frozen"); err == nil {
		t.Fatal("input mutation during execution was accepted")
	} else if value, ok := err.(*fault.Error); !ok || value.Code != "STUDIO_TASK_INPUT_FROZEN" {
		t.Fatalf("unexpected input freeze error: %v", err)
	}
}

func TestWorkTaskRetryCreatesNewRuntimeJobAfterTerminalFailure(t *testing.T) {
	service, actor, projectID, experience, _ := customerStudioInputFixture(t)
	created, err := service.Work.CreateCustomerStudioTask(t.Context(), actor, application.StudioCreateTaskInput{
		ExperienceID: experience.ID, ProjectID: projectID, Title: "Runtime 重试测试", Goal: "验证终态 JobRun 不原地恢复",
		BusinessBrief: application.StudioBusinessBrief{Audience: "新客户", Channel: "抖音", Tone: "真实", Keywords: []string{"重试"}},
	}, "runtime-retry-create")
	if err != nil {
		t.Fatal(err)
	}
	jops, err := service.Runtime.Runtime().Jobs(t.Context(), actor.TenantID, created.Task.ID)
	if err != nil || len(jops) != 1 {
		t.Fatalf("initial jobs = %d, err=%v", len(jops), err)
	}
	if _, err := service.Runtime.Runtime().Cancel(t.Context(), actor.TenantID, jops[0].ID, "test", actor.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "retry"}, "runtime-retry-schedule"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "start"}, "runtime-retry-start"); err != nil {
		t.Fatal(err)
	}
	jops, err = service.Runtime.Runtime().Jobs(t.Context(), actor.TenantID, created.Task.ID)
	if err != nil || len(jops) != 2 {
		t.Fatalf("retry jobs = %d, err=%v", len(jops), err)
	}
	var activeJob, cancelledJob contentruntime.JobRun
	for _, job := range jops {
		if job.State == contentruntime.JobRunCancelled {
			cancelledJob = job
		} else {
			activeJob = job
		}
	}
	if activeJob.ID == "" || cancelledJob.ID == "" || activeJob.IdempotencyKey == cancelledJob.IdempotencyKey {
		t.Fatalf("retry reused terminal Runtime fact: %#v", jops)
	}
}

func TestWorkTaskCancelCancelsRuntimeJob(t *testing.T) {
	service, actor, projectID, experience, _ := customerStudioInputFixture(t)
	created, err := service.Work.CreateCustomerStudioTask(t.Context(), actor, application.StudioCreateTaskInput{
		ExperienceID: experience.ID, ProjectID: projectID, Title: "Runtime 取消测试", Goal: "验证取消任务不会留下活动执行",
		BusinessBrief: application.StudioBusinessBrief{Audience: "新客户", Channel: "抖音", Tone: "真实", Keywords: []string{"取消"}},
	}, "runtime-cancel-create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "cancel"}, "runtime-cancel"); err != nil {
		t.Fatal(err)
	}
	jobs, err := service.Runtime.Runtime().Jobs(t.Context(), actor.TenantID, created.Task.ID)
	if err != nil || len(jobs) != 1 || jobs[0].State != contentruntime.JobRunCancelled {
		t.Fatalf("cancelled task left Runtime job active: jobs=%#v err=%v", jobs, err)
	}
}

func TestWorkTaskPauseAndResumeMirrorRuntimeState(t *testing.T) {
	service, actor, projectID, experience, _ := customerStudioInputFixture(t)
	created, err := service.Work.CreateCustomerStudioTask(t.Context(), actor, application.StudioCreateTaskInput{
		ExperienceID: experience.ID, ProjectID: projectID, Title: "Runtime 暂停测试", Goal: "验证任务控制面与 Runtime 状态一致",
		BusinessBrief: application.StudioBusinessBrief{Audience: "新客户", Channel: "抖音", Tone: "真实", Keywords: []string{"暂停", "恢复"}},
	}, "runtime-pause-create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "start"}, "runtime-pause-start"); err != nil {
		t.Fatal(err)
	}
	paused, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "pause"}, "runtime-pause")
	if err != nil || paused.Task.Status != work.TaskStatusPaused {
		t.Fatalf("task pause failed: task=%#v err=%v", paused.Task, err)
	}
	jobs, err := service.Runtime.Runtime().Jobs(t.Context(), actor.TenantID, created.Task.ID)
	if err != nil || len(jobs) != 1 || jobs[0].State != contentruntime.JobRunPaused {
		t.Fatalf("task pause did not mirror Runtime: jobs=%#v err=%v", jobs, err)
	}
	resumed, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "resume"}, "runtime-resume")
	if err != nil || resumed.Task.Status != work.TaskStatusRunning {
		t.Fatalf("task resume failed: task=%#v err=%v", resumed.Task, err)
	}
	jobs, err = service.Runtime.Runtime().Jobs(t.Context(), actor.TenantID, created.Task.ID)
	if err != nil || len(jobs) != 1 || jobs[0].State != contentruntime.JobRunRunning {
		t.Fatalf("task resume did not mirror Runtime: jobs=%#v err=%v", jobs, err)
	}
}

func TestWorkTaskRetryFromPausedCancelsOldRuntimeAndStartsFreshJob(t *testing.T) {
	service, actor, projectID, experience, _ := customerStudioInputFixture(t)
	created, err := service.Work.CreateCustomerStudioTask(t.Context(), actor, application.StudioCreateTaskInput{
		ExperienceID: experience.ID, ProjectID: projectID, Title: "暂停重试测试", Goal: "验证暂停任务重试会重开 Runtime 执行",
		BusinessBrief: application.StudioBusinessBrief{Audience: "新客户", Channel: "抖音", Tone: "真实", Keywords: []string{"暂停", "重试"}},
	}, "runtime-paused-retry-create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "start"}, "runtime-paused-retry-start"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "pause"}, "runtime-paused-retry-pause"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "retry"}, "runtime-paused-retry-schedule"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "start"}, "runtime-paused-retry-restart"); err != nil {
		t.Fatal(err)
	}

	jobs, err := service.Runtime.Runtime().Jobs(t.Context(), actor.TenantID, created.Task.ID)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("paused retry jobs = %d, err=%v", len(jobs), err)
	}
	var paused, active contentruntime.JobRun
	for _, job := range jobs {
		switch job.State {
		case contentruntime.JobRunCancelled:
			paused = job
		default:
			active = job
		}
	}
	if paused.ID == "" || active.ID == "" || paused.IdempotencyKey == active.IdempotencyKey {
		t.Fatalf("paused retry did not create a fresh Runtime job: %#v", jobs)
	}
	if active.State != contentruntime.JobRunCreated && active.State != contentruntime.JobRunAdmitted && active.State != contentruntime.JobRunRunning {
		t.Fatalf("fresh Runtime job state = %q, want an active admission state", active.State)
	}
}

func TestWorkTaskStartDoesNotPersistRunningWhenRuntimeAdmissionFails(t *testing.T) {
	service, actor, projectID, experience, _ := customerStudioInputFixture(t)
	created, err := service.Work.CreateCustomerStudioTask(t.Context(), actor, application.StudioCreateTaskInput{
		ExperienceID: experience.ID, ProjectID: projectID, Title: "准入失败测试", Goal: "验证 Runtime 拒绝时任务不会伪装成运行中",
		BusinessBrief: application.StudioBusinessBrief{Audience: "新客户", Channel: "抖音", Tone: "真实", Keywords: []string{"准入"}},
	}, "runtime-admission-failure-create")
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := service.Runtime.Runtime().Jobs(t.Context(), actor.TenantID, created.Task.ID)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("initial Runtime jobs = %#v, err=%v", jobs, err)
	}
	if _, err := service.Runtime.Runtime().Cancel(t.Context(), actor.TenantID, jobs[0].ID, "test", actor.UserID); err != nil {
		t.Fatal(err)
	}
	service.Runtime.Runtime().SetRolloutPolicy(contentruntime.RolloutPolicy{AdmissionEnabled: false, DynamicGraphEnabled: true})
	if _, err := service.Work.TaskAction(t.Context(), actor, created.Task.ID, application.TaskActionInput{Action: "start"}, "runtime-admission-failure-start"); err == nil {
		t.Fatal("Runtime admission failure was accepted")
	}
	view, err := service.Work.WorkTask(t.Context(), actor, created.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Task.Status == work.TaskStatusRunning {
		t.Fatalf("task persisted running after Runtime admission failure: %#v", view.Task)
	}
	if len(view.Runs) != 1 || view.Runs[0].State != work.TaskStatusCancelled {
		t.Fatalf("Runtime admission failure created an unexpected job: %#v", view.Runs)
	}
}

func customerStudioInputFixture(t *testing.T) (*application.Application, application.Actor, string, application.StudioExperience, application.StudioUser) {
	t.Helper()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("input-admin@example.com"))
	session, err := service.Identity.Register(t.Context(), "input-admin@example.com", "long-enough-password", "Input Admin", "Input Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, user, err := service.Identity.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Identity.UpdatePlatformTenantContentCapability(t.Context(), actor, actor.TenantID, identitydomain.ContentTypeMarketingVideo, true, "enable-input-test"); err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Input Brand", ProductName: "Input Product", ContentType: identitydomain.ContentTypeMarketingVideo, Channel: "douyin"}, "input-project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.Workspace.CreateConnectSession(t.Context(), actor, project.ID, "input-connect")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, application.ConnectDeviceInput{Hostname: "input-test", Platform: "darwin", Arch: "arm64", Version: "test"}); err != nil {
		t.Fatal(err)
	}
	experiences, err := service.Work.CustomerStudioBootstrap(t.Context(), actor, user)
	if err != nil || len(experiences.Experiences) == 0 {
		t.Fatalf("customer studio bootstrap = %#v, err=%v", experiences, err)
	}
	for _, experience := range experiences.Experiences {
		if experience.ContentType == identitydomain.ContentTypeMarketingVideo && len(experience.ProjectIDs) > 0 {
			return service, actor, project.ID, experience, application.StudioUser{ID: user.ID, DisplayName: user.DisplayName}
		}
	}
	t.Fatal("marketing video experience was not available")
	return nil, application.Actor{}, "", application.StudioExperience{}, application.StudioUser{}
}
