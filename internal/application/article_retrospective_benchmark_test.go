package application_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

// TestArticleRetrospectiveBenchmarkUsesRuntimeFanoutAndSharedLearningFacts
// covers a structurally different workflow: observations fan out by channel,
// analysis joins into one learning decision, and the decision still points to
// the immutable approved snapshot rather than a retrospective-only subject.
func TestArticleRetrospectiveBenchmarkUsesRuntimeFanoutAndSharedLearningFacts(t *testing.T) {
	ctx, service, store, actor, binding := v3ContentFixture(t)
	revision := publishV3ContentItem(t, ctx, service, binding, "retro-content", "retro-content-publish")
	if _, err := service.Review.ApproveSubmission(ctx, actor, revision.ID, "复盘基线版本内部审核通过", "retro-content-internal"); err != nil {
		t.Fatal(err)
	}
	grant, err := service.Review.CreateReviewGrant(ctx, actor, revision.ID, "retro-client@example.com", "retro-content-grant")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review.VerifyReviewGrant(ctx, grant.PlaintextToken, grant.PlaintextOTP); err != nil {
		t.Fatal(err)
	}
	decision, err := service.Review.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "复盘基线版本客户确认", "", "retro-content-client")
	if err != nil || decision.ApprovedSnapshot == nil {
		t.Fatalf("retro baseline did not reach an approved snapshot: %#v err=%v", decision, err)
	}
	snapshotID := decision.ApprovedSnapshot.ID

	sop := catalogdomain.SOPVersion{
		ID: "article-retro-benchmark-v1", TenantID: actor.TenantID, SOPID: "article-retro-benchmark", Version: 1,
		SchemaVersion: catalogdomain.SOPSchemaVersion, Name: "文章复盘基准流程", Status: "published",
		ContentTypes: []string{identitydomain.ContentTypeWeChatArticle}, DefaultExecutionMode: "local",
		Stages: []catalogdomain.StageDefinition{
			{ID: "result", Name: "结果导入", Order: 10, OutputSchema: "contentcloud.performance_observation/1.0", ExecutionModes: []string{"local"}},
			{ID: "analysis", Name: "渠道归因分析", Order: 20, InputRefs: []string{"result"}, OutputSchema: "contentcloud.retro_analysis/1.0", ExecutionModes: []string{"agent"}},
			{ID: "learning", Name: "学习候选汇聚", Order: 30, InputRefs: []string{"analysis"}, OutputSchema: "contentcloud.learning_candidate/1.0", ExecutionModes: []string{"local"}},
		},
	}
	runtimeService := service.Runtime.Runtime()
	started, err := runtimeService.Start(ctx, contentruntime.StartInput{
		TenantID: actor.TenantID, ProjectID: binding.ProjectID, WorkTaskID: "article-retro-task", BusinessType: "article_retrospective",
		SOP: sop, BindingDigest: "sha256:" + strings.Repeat("a", 64), InputDigest: "sha256:" + strings.Repeat("b", 64),
		RuntimePolicyID: "runtime-policy/test-v1", ContractMajor: 1, ContractMinor: 0, CreatedBy: actor.UserID, IdempotencyKey: "article-retro-runtime",
	})
	if err != nil {
		t.Fatal(err)
	}
	items := []contentruntime.FanoutItemInput{}
	for index := 1; index <= 4; index++ {
		items = append(items, contentruntime.FanoutItemInput{ItemKey: fmt.Sprintf("channel:%02d", index), ItemDigest: "sha256:" + strings.Repeat(string(rune('a'+index)), 64)})
	}
	fanout, err := runtimeService.CreateFanoutSet(ctx, contentruntime.CreateFanoutSetInput{
		TenantID: actor.TenantID, JobRunID: started.Job.ID, MapNodeKey: "stage:analysis", JoinNodeKey: "stage:learning",
		SourceCollection: "article.performance", SourceRevision: 1, SourceWatermark: 1, Generation: 1,
		IdempotencyKey: "article-retro-fanout", Reason: "按渠道并行归因并汇聚下一轮学习候选",
		JoinPolicy:   contentruntime.JoinPolicy{Strategy: contentruntime.JoinAll},
		NodeTemplate: contentruntime.JobPlanNode{Kind: "retro_analysis", Name: "渠道归因", OutputSchema: "contentcloud.retro_analysis/1.0", RetryMaxAttempts: 1}, Items: items,
	})
	if err != nil || len(fanout.Nodes) != 4 {
		t.Fatalf("article retrospective fanout was not created through Runtime: nodes=%d err=%v", len(fanout.Nodes), err)
	}

	nodes, err := runtimeService.Nodes(ctx, actor.TenantID, started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var resultNode, analysisNode contentruntime.NodeRun
	for _, node := range nodes {
		switch node.NodeKey {
		case "stage:result":
			resultNode = node
		case "stage:analysis":
			analysisNode = node
		}
	}
	if resultNode.State != contentruntime.NodeReady || analysisNode.ID == "" {
		t.Fatalf("article retrospective entry nodes are not ready: result=%#v analysis=%#v", resultNode, analysisNode)
	}
	completeRetroNode(t, runtimeService, actor.TenantID, resultNode, "performance:batch")
	if _, err := runtimeService.Refresh(ctx, actor.TenantID, started.Job.ID); err != nil {
		t.Fatal(err)
	}
	analysisNode, err = runtimeService.Repository().NodeRun(ctx, actor.TenantID, analysisNode.ID)
	if err != nil {
		t.Fatal(err)
	}
	if analysisNode.State != contentruntime.NodeReady {
		t.Fatalf("retro analysis map node was not unlocked: %s", analysisNode.State)
	}
	completeRetroNode(t, runtimeService, actor.TenantID, analysisNode, "analysis:seed")
	if _, err := runtimeService.Refresh(ctx, actor.TenantID, started.Job.ID); err != nil {
		t.Fatal(err)
	}
	for _, node := range fanout.Nodes {
		current, err := runtimeService.Repository().NodeRun(ctx, actor.TenantID, node.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.State != contentruntime.NodeReady {
			t.Fatalf("retro analysis node %s was not unlocked: %s", current.NodeKey, current.State)
		}
		completeRetroNode(t, runtimeService, actor.TenantID, current, "analysis:"+current.NodeKey)
	}
	joined, err := runtimeService.JoinFanoutSet(ctx, actor.TenantID, fanout.Set.ID, "runtime.join")
	if err != nil || joined.Status != contentruntime.FanoutSucceeded {
		t.Fatalf("article retrospective join did not succeed: %#v err=%v", joined, err)
	}
	if _, err := runtimeService.Refresh(ctx, actor.TenantID, started.Job.ID); err != nil {
		t.Fatal(err)
	}
	finalNodes, err := runtimeService.Nodes(ctx, actor.TenantID, started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var learning contentruntime.NodeRun
	for _, node := range finalNodes {
		if node.NodeKey == "stage:learning" {
			learning = node
		}
	}
	if learning.State != contentruntime.NodeReady {
		t.Fatalf("retro join did not unlock learning stage: %s", learning.State)
	}

	now := time.Now().UTC().Add(-24 * time.Hour)
	observations := make([]application.CreateObservationInput, 0, 4)
	for index, channel := range []string{"douyin", "wechat", "xiaohongshu", "video_account"} {
		observations = append(observations, application.CreateObservationInput{
			RowNumber: index + 1, ApprovedSnapshotID: snapshotID, Platform: channel, AccountAlias: "retro-" + channel,
			PublishedAt: now, WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"views": float64(1000 + index*250), "completion_rate": 0.5 + float64(index)/20},
			Currency: "CNY", IssueCategory: "creative",
		})
	}
	imported, err := service.Performance.ImportPerformanceObservations(ctx, actor, application.ImportPerformanceInput{ProjectID: binding.ProjectID, SourceName: "article-retro-results.csv", SourceFormat: "csv", Observations: observations}, "article-retro-results")
	if err != nil || len(imported.Observations) != 4 {
		t.Fatalf("retrospective observations were not imported: count=%d err=%v", len(imported.Observations), err)
	}
	learningDecision, err := service.Performance.CreateRatingDecision(ctx, actor, application.CreateRatingDecisionInput{
		ProjectID: binding.ProjectID, SubjectType: "approved_snapshot", SubjectID: snapshotID,
		ObservationIDs: []string{imported.Observations[0].ID, imported.Observations[1].ID, imported.Observations[2].ID, imported.Observations[3].ID},
		Rating:         "seed_candidate", Reason: "四个渠道结果完成汇聚，保留证据并提出下一轮受控假设", NextAction: "固定事实与权利边界，只测试一个开场变量",
	}, "article-retro-learning")
	if err != nil || learningDecision.Decision.SubjectID != snapshotID || len(learningDecision.Decision.ObservationIDs) != 4 {
		t.Fatalf("retrospective learning decision lost shared lineage: %#v err=%v", learningDecision, err)
	}
	storedObservations, err := store.PerformanceObservations(ctx, actor.TenantID, binding.ProjectID)
	if err != nil || len(storedObservations) != 4 {
		t.Fatalf("retrospective wrote unexpected observation facts: %#v err=%v", storedObservations, err)
	}
}

func completeRetroNode(t *testing.T, service *contentruntime.Service, tenantID string, node contentruntime.NodeRun, outputRef string) contentruntime.NodeRun {
	t.Helper()
	leased, err := service.TransitionNode(t.Context(), tenantID, node.ID, contentruntime.NodeLeased, "runtime", "retro-worker", node.Version)
	if err != nil {
		t.Fatal(err)
	}
	running, err := service.TransitionNode(t.Context(), tenantID, node.ID, contentruntime.NodeRunning, "worker", "retro-worker", leased.Version)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := service.CompleteNode(t.Context(), tenantID, node.ID, []string{outputRef}, "sha256:"+strings.Repeat("d", 64), "worker", "retro-worker", running.Version)
	if err != nil {
		t.Fatal(err)
	}
	return completed
}
