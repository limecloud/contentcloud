package application_test

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

func TestSerializedNovelClosesThroughSharedReviewDeliveryAndLearning(t *testing.T) {
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default(), application.WithPlatformAdminEmails("novel-admin@example.com"))
	session, err := service.Identity.Register(t.Context(), "novel-admin@example.com", "long-enough-password", "Novel Admin", "Novel Tenant")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	must(t, err)
	_, err = service.Identity.UpdatePlatformTenantContentCapability(t.Context(), actor, actor.TenantID, identitydomain.ContentTypeSerializedNovel, true, "enable-novel")
	must(t, err)
	profile, err := service.Catalog.InstallContentProfile(t.Context(), actor, "serialized-novel", "novel-profile")
	must(t, err)
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Novel Studio", ProductName: "The Long Thread", ContentType: identitydomain.ContentTypeSerializedNovel, Channel: "web_novel"}, "novel-project")
	must(t, err)
	task, err := service.Work.CreateWorkTask(t.Context(), actor, application.CreateWorkTaskInput{ProjectID: project.ID, SOPID: profile.SOP.Definition.ID, SOPVersion: profile.SOP.Versions[0].Version, Title: "第十章章节任务", Intent: "完成一次有连续性证据的章节交付", ContentType: identitydomain.ContentTypeSerializedNovel, InputRefs: []string{"brief:novel"}}, "novel-task")
	must(t, err)
	if task.Task.SOPID != profile.SOP.Definition.ID || task.Task.ContentType != identitydomain.ContentTypeSerializedNovel {
		t.Fatalf("novel task did not bind the profile SOP: %#v", task.Task)
	}

	chapter := localworkspace.NovelChapter{SchemaVersion: localworkspace.NovelChapterSchema, ID: "chapter:10", SeriesID: "series:long-thread", ChapterNo: 10, Title: "回信", Summary: "主角在京城打开一封迟来的信", Body: "主角在城门边拆开信封，发现落款来自已经失踪的老师。", OutlineRef: "outline:volume-1:10", CharacterRefs: []string{"hero"}, LocationRefs: []string{"capital"}, ResolvedThreads: []string{}, OpenedThreads: []localworkspace.NovelThread{{ID: "thread:teacher", Description: "老师的去向", OpenedIn: 10}}, TimelineOrder: 10, Status: "review_ready"}
	content, err := json.Marshal(chapter)
	must(t, err)
	revision, err := service.Work.CreateTaskRevision(t.Context(), actor, task.Task.ID, application.CreateTaskRevisionInput{ContentType: identitydomain.ContentTypeSerializedNovel, SchemaVersion: localworkspace.NovelChapterSchema, Content: content}, "novel-submission")
	must(t, err)
	if revision.ID == "" || revision.SchemaVersion != localworkspace.NovelChapterSchema {
		t.Fatalf("novel compatibility entrypoint did not return governed revision: %#v", revision)
	}
	internalApproval, err := service.Review.ApproveSubmission(t.Context(), actor, revision.ID, "章节内部审核通过", "novel-internal-approve")
	must(t, err)
	if internalApproval.ApprovedSnapshot != nil || internalApproval.Submission.Status != "internally_approved" {
		t.Fatalf("novel internal approval bypassed client review: %#v", internalApproval)
	}
	grant, err := service.Review.CreateReviewGrant(t.Context(), actor, revision.ID, "novel-client@example.com", "novel-client-grant")
	must(t, err)
	_, err = service.Review.VerifyReviewGrant(t.Context(), grant.PlaintextToken, grant.PlaintextOTP)
	must(t, err)
	decision, err := service.Review.DecideReviewGrant(t.Context(), grant.PlaintextToken, "approve", "章节客户确认通过", "", "novel-client-approve")
	must(t, err)
	if decision.ApprovedSnapshot == nil {
		t.Fatal("novel client approval did not create an ApprovedSnapshot")
	}
	snapshot := *decision.ApprovedSnapshot
	delivery, err := service.Review.CreateDeliveryPackage(t.Context(), actor, snapshot.ID, chapter.ID, "novel-delivery")
	must(t, err)
	if delivery.ContentItemID != chapter.ID || len(delivery.Manifest) != 3 {
		t.Fatalf("novel delivery did not use the approved chapter renderer: %#v", delivery)
	}
	formats := map[string]bool{}
	for _, artifact := range delivery.Manifest {
		formats[artifact.Metadata["format"].(string)] = true
		if artifact.ApprovedSnapshotID != snapshot.ID {
			t.Fatalf("novel artifact lost snapshot lineage: %#v", artifact)
		}
	}
	for _, format := range []string{"json", "markdown", "xlsx"} {
		if !formats[format] {
			t.Fatalf("novel delivery is missing %s: %#v", format, formats)
		}
	}
	now := time.Now().UTC()
	performance, err := service.Performance.ImportPerformanceObservations(t.Context(), actor, application.ImportPerformanceInput{ProjectID: project.ID, SourceName: "novel-manual", SourceFormat: "manual", Observations: []application.CreateObservationInput{{RowNumber: 1, ApprovedSnapshotID: snapshot.ID, Platform: "web_novel", AccountAlias: "series-main", PublishedAt: now.Add(-24 * time.Hour), WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"views": 1200, "completion_rate": 0.62}, Currency: "CNY", Spend: 0, GMV: 0, IssueCategory: "creative"}}}, "novel-performance")
	must(t, err)
	rating, err := service.Performance.CreateRatingDecision(t.Context(), actor, application.CreateRatingDecisionInput{ProjectID: project.ID, SubjectType: "approved_snapshot", SubjectID: snapshot.ID, ObservationIDs: []string{performance.Observations[0].ID}, Rating: "seed_candidate", Reason: "章节完成率足以进入下一轮小规模测试", NextAction: "保留 Canon，测试下一章开篇"}, "novel-rating")
	must(t, err)
	if rating.Decision.SubjectID != snapshot.ID {
		t.Fatalf("novel rating lost snapshot lineage: %#v", rating)
	}
	view, err := service.Work.WorkTask(t.Context(), actor, task.Task.ID)
	must(t, err)
	if len(view.Revisions) != 1 || len(view.ApprovedSnapshots) != 1 || len(view.Artifacts) != 3 || len(view.DeliveryPackages) != 1 {
		t.Fatalf("novel task projection did not rebuild shared facts: revisions=%d snapshots=%d artifacts=%d packages=%d", len(view.Revisions), len(view.ApprovedSnapshots), len(view.Artifacts), len(view.DeliveryPackages))
	}
}
