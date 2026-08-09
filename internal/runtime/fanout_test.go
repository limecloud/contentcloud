package runtime

import (
	"strconv"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestEvaluateJoinUsesFrozenMembershipAndExplicitZeroPolicy(t *testing.T) {
	now := fixedRuntimeTime()
	set := domain.FanoutSet{ID: "set-1", TenantID: "tenant-1", JobRunID: "job-1", MapNodeKey: "map", JoinNodeKey: "join", Generation: 1, IdempotencyKey: "fanout-1", MembershipDigest: "sha256:" + strings.Repeat("a", 64), RequestDigest: "sha256:" + strings.Repeat("b", 64), MemberCount: 0, JoinPolicy: domain.JoinPolicy{Strategy: domain.JoinAll, ZeroMemberPolicy: domain.ZeroMemberFail}, Status: domain.FanoutClosed, Version: 1, ClosedAt: &now, CreatedAt: now, UpdatedAt: now}
	decision, err := domain.EvaluateJoin(set, []domain.FanoutMember{})
	if err != nil || !decision.Terminal || decision.Status != domain.FanoutFailed {
		t.Fatalf("zero-member fail policy was not applied: %#v err=%v", decision, err)
	}
	set.JoinPolicy.ZeroMemberPolicy = domain.ZeroMemberSucceedEmpty
	decision, err = domain.EvaluateJoin(set, []domain.FanoutMember{})
	if err != nil || decision.Status != domain.FanoutSucceeded {
		t.Fatalf("zero-member succeed policy was not applied: %#v err=%v", decision, err)
	}
}

func TestCreateAndJoinFanoutSetIsAtomicAndDeterministic(t *testing.T) {
	repo := memory.New()
	service := New(repo, fixedRuntimeTime)
	started, err := service.Start(t.Context(), testStartInput("fanout-task", "fanout-job"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateFanoutSet(t.Context(), CreateFanoutSetInput{
		TenantID: "tenant-1", JobRunID: started.Job.ID, MapNodeKey: "stage:sources", JoinNodeKey: "stage:delivery", Generation: 1,
		IdempotencyKey: "fanout-set-1", Reason: "按冻结受众集合展开", JoinPolicy: domain.JoinPolicy{Strategy: domain.JoinQuorum, QuorumPercent: 50, ZeroMemberPolicy: domain.ZeroMemberFail, QuorumStopPolicy: domain.QuorumCancelPending},
		NodeTemplate: domain.JobPlanNode{Kind: "stage", Name: "受众脚本", OutputSchema: "contentcloud.script/1.0", RetryMaxAttempts: 1},
		Items:        []FanoutItemInput{{ItemKey: "audience:b", ItemDigest: "sha256:b"}, {ItemKey: "audience:a", ItemDigest: "sha256:a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Set.Status != domain.FanoutClosed || created.Set.MemberCount != 2 || len(created.Members) != 2 || len(created.Nodes) != 2 {
		t.Fatalf("fanout snapshot was not persisted: %#v", created)
	}
	if created.Members[0].ItemKey != "audience:a" || created.Members[0].MemberKey == created.Members[1].MemberKey {
		t.Fatalf("fanout membership is not sorted/deterministic: %#v", created.Members)
	}
	repeated, err := service.CreateFanoutSet(t.Context(), CreateFanoutSetInput{TenantID: "tenant-1", JobRunID: started.Job.ID, MapNodeKey: "stage:sources", JoinNodeKey: "stage:delivery", Generation: 1, IdempotencyKey: "fanout-set-1", JoinPolicy: created.Set.JoinPolicy, NodeTemplate: domain.JobPlanNode{Kind: "stage", Name: "受众脚本", OutputSchema: "contentcloud.script/1.0", RetryMaxAttempts: 1}, Items: []FanoutItemInput{{ItemKey: "audience:a", ItemDigest: "sha256:a"}, {ItemKey: "audience:b", ItemDigest: "sha256:b"}}})
	if err != nil || repeated.Set.ID != created.Set.ID || repeated.Plan.ID != created.Plan.ID {
		t.Fatalf("fanout idempotency did not reload the frozen set: %#v err=%v", repeated, err)
	}
	first := created.Nodes[0]
	if _, err := service.TransitionNode(t.Context(), "tenant-1", first.ID, domain.NodeReady, "runtime", "scheduler", first.Version); err != nil {
		t.Fatal(err)
	}
	first, err = repo.NodeRun(t.Context(), "tenant-1", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.TransitionNode(t.Context(), "tenant-1", first.ID, domain.NodeLeased, "runtime", "worker-1", first.Version); err != nil {
		t.Fatal(err)
	}
	first, err = repo.NodeRun(t.Context(), "tenant-1", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.TransitionNode(t.Context(), "tenant-1", first.ID, domain.NodeRunning, "worker", "worker-1", first.Version); err != nil {
		t.Fatal(err)
	}
	first, err = repo.NodeRun(t.Context(), "tenant-1", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteNode(t.Context(), "tenant-1", first.ID, []string{"script:a"}, "sha256:result-a", "worker", "worker-1", first.Version); err != nil {
		t.Fatal(err)
	}
	joined, err := service.JoinFanoutSet(t.Context(), "tenant-1", created.Set.ID, "joiner")
	if err != nil {
		t.Fatal(err)
	}
	if joined.Status != domain.FanoutSucceeded {
		t.Fatalf("quorum join did not cancel pending member and succeed: %#v", joined)
	}
	members, err := repo.FanoutMembers(t.Context(), "tenant-1", created.Set.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range members {
		if member.ItemKey == "audience:b" && member.State != domain.FanoutMemberCancelled {
			t.Fatalf("pending member was not cancelled: %#v", member)
		}
	}
}

func TestRuntimeSupportsArticleRetrospectiveFanoutAtCapacityWithoutBusinessTables(t *testing.T) {
	sop := testSOP()
	sop.ID, sop.SOPID, sop.Name = "article-retro-v1", "article-retro", "文章复盘并行分析"
	sop.ContentTypes = []string{domain.ContentTypeWeChatArticle}
	sop.Gates = nil
	sop.Stages = []domain.StageDefinition{
		{ID: "result", Name: "结果导入", Order: 10, OutputSchema: "contentcloud.performance_observation/1.0", ExecutionModes: []string{"local"}},
		{ID: "analysis", Name: "并行归因", Order: 20, InputRefs: []string{"result"}, OutputSchema: "contentcloud.retro_analysis/1.0", ExecutionModes: []string{"agent"}},
		{ID: "learning", Name: "汇聚学习", Order: 30, InputRefs: []string{"analysis"}, OutputSchema: "contentcloud.learning_candidate/1.0", ExecutionModes: []string{"agent"}},
	}
	repo := memory.New()
	service := New(repo, fixedRuntimeTime)
	input := testStartInput("article-retro-task", "article-retro-job")
	input.SOP = sop
	started, err := service.Start(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]FanoutItemInput, 0, 50)
	for index := 0; index < 50; index++ {
		items = append(items, FanoutItemInput{ItemKey: "channel:" + strconv.Itoa(index), ItemDigest: "sha256:" + strings.Repeat("a", 64)})
	}
	created, err := service.CreateFanoutSet(t.Context(), CreateFanoutSetInput{TenantID: "tenant-1", JobRunID: started.Job.ID, MapNodeKey: "stage:analysis", JoinNodeKey: "stage:learning", Generation: 1, IdempotencyKey: "article-retro-fanout-1", Reason: "按渠道与人群并行复盘", JoinPolicy: domain.JoinPolicy{Strategy: domain.JoinBestEffort}, NodeTemplate: domain.JobPlanNode{Kind: "retro_analysis", Name: "渠道归因", OutputSchema: "contentcloud.retro_analysis/1.0", RetryMaxAttempts: 1}, Items: items})
	if err != nil || len(created.Members) != 50 || len(created.Nodes) != 50 {
		t.Fatalf("article retrospective fanout did not use shared Runtime capacity: nodes=%d members=%d err=%v", len(created.Nodes), len(created.Members), err)
	}
	items = append(items[:0], items...)
	if _, err := service.CreateFanoutSet(t.Context(), CreateFanoutSetInput{TenantID: "tenant-1", JobRunID: started.Job.ID, MapNodeKey: "stage:analysis", JoinNodeKey: "stage:learning", Generation: 2, IdempotencyKey: "article-retro-fanout-2", Reason: "超出动态图规模上限", JoinPolicy: domain.JoinPolicy{Strategy: domain.JoinAll}, NodeTemplate: domain.JobPlanNode{Kind: "retro_analysis", Name: "第二批归因", OutputSchema: "contentcloud.retro_analysis/1.0", RetryMaxAttempts: 1}, Items: items}); !hasDomainCode(err, "JOB_PLAN_NODE_LIMIT") {
		t.Fatalf("dynamic graph should reject the 101st node without a business-specific table, got %v", err)
	}
}
