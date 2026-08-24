package runtime_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/limecloud/contentcloud/internal/runtime"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

// TestMarketingCampaignBenchmarkUsesSharedRuntimeForTenParallelVariants
// exercises the platform side of a marketing campaign. The business sample
// only supplies a published SOP and a frozen membership list; Fanout, Join,
// node leases, CAS and terminal aggregation remain Runtime responsibilities.
func TestMarketingCampaignBenchmarkUsesSharedRuntimeForTenParallelVariants(t *testing.T) {
	sop := testSOP()
	sop.ID, sop.SOPID, sop.Name = "marketing-campaign-v1", "marketing-campaign", "营销活动内容包"
	sop.ContentTypes = []string{identitydomain.ContentTypeMarketingVideo}
	sop.Gates = nil
	sop.Stages = []catalogdomain.StageDefinition{
		{ID: "brief", Name: "活动简报", Order: 10, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local"}},
		{ID: "variants", Name: "渠道变体", Order: 20, InputRefs: []string{"brief"}, OutputSchema: "contentcloud.campaign_variant/1.0", ExecutionModes: []string{"agent"}},
		{ID: "merge", Name: "活动内容汇聚", Order: 30, InputRefs: []string{"variants"}, OutputSchema: "contentcloud.campaign_package/1.0", ExecutionModes: []string{"local"}},
	}
	repo := memory.New()
	service := New(repo, fixedRuntimeTime)
	started, err := service.Start(t.Context(), StartInput{
		TenantID: "tenant-1", ProjectID: "project-campaign", WorkTaskID: "task-campaign",
		BusinessType: "marketing_campaign", SOP: sop, BindingDigest: "sha256:" + strings.Repeat("a", 64),
		InputDigest: "sha256:" + strings.Repeat("b", 64), RuntimePolicyID: "runtime-policy/test-v1",
		ContractMajor: 1, ContractMinor: 0, CreatedBy: "user-1", IdempotencyKey: "campaign-runtime-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(started.Nodes) != 3 {
		t.Fatalf("unexpected initial campaign plan: %d nodes", len(started.Nodes))
	}

	items := make([]FanoutItemInput, 0, 10)
	for index := 1; index <= 10; index++ {
		items = append(items, FanoutItemInput{
			ItemKey:    fmt.Sprintf("channel-variant:%02d", index),
			ItemDigest: "sha256:" + strings.Repeat(string(rune('a'+index)), 64),
		})
	}
	created, err := service.CreateFanoutSet(t.Context(), CreateFanoutSetInput{
		TenantID: "tenant-1", JobRunID: started.Job.ID, MapNodeKey: "stage:variants", JoinNodeKey: "stage:merge",
		SourceCollection: "campaign.channels", SourceRevision: 1, SourceWatermark: 1,
		Generation: 1, IdempotencyKey: "campaign-fanout-1", Reason: "按冻结渠道集合生成十个并行变体",
		JoinPolicy: JoinPolicy{Strategy: JoinAll}, NodeTemplate: JobPlanNode{Kind: "campaign_variant", Name: "渠道变体", OutputSchema: "contentcloud.campaign_variant/1.0", RetryMaxAttempts: 1}, Items: items,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Set.MemberCount != 10 || len(created.Members) != 10 || len(created.Nodes) != 10 {
		t.Fatalf("campaign fanout did not freeze ten members: set=%#v nodes=%d", created.Set, len(created.Nodes))
	}

	// Complete the map stage so Runtime, rather than the workbench, unlocks
	// all fanout members through dependency refresh.
	nodes, err := service.Nodes(t.Context(), "tenant-1", started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var briefNode, mapNode NodeRun
	for _, node := range nodes {
		switch node.NodeKey {
		case "stage:brief":
			briefNode = node
		case "stage:variants":
			mapNode = node
		}
	}
	if briefNode.ID == "" || mapNode.ID == "" {
		t.Fatal("campaign brief or map node was not created")
	}
	if briefNode.State != NodeReady {
		t.Fatalf("campaign brief node should be ready, got %s", briefNode.State)
	}
	if briefNode = completeBenchmarkNode(t, service, "tenant-1", briefNode, "campaign:brief"); briefNode.State != NodeSucceeded {
		t.Fatalf("campaign brief node did not complete: %#v", briefNode)
	}
	if _, err := service.Refresh(t.Context(), "tenant-1", started.Job.ID); err != nil {
		t.Fatal(err)
	}
	mapNode, err = repo.NodeRun(t.Context(), "tenant-1", mapNode.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mapNode.State != NodeReady {
		t.Fatalf("campaign map node should be ready, got %s", mapNode.State)
	}
	mapNode = completeBenchmarkNode(t, service, "tenant-1", mapNode, "campaign:brief")
	if mapNode.State != NodeSucceeded {
		t.Fatalf("campaign map node did not complete: %#v", mapNode)
	}
	if _, err := service.Refresh(t.Context(), "tenant-1", started.Job.ID); err != nil {
		t.Fatal(err)
	}

	for _, node := range created.Nodes {
		current, err := repo.NodeRun(t.Context(), "tenant-1", node.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.State != NodeReady {
			t.Fatalf("fanout member %s was not unlocked by Runtime: %s", current.NodeKey, current.State)
		}
		current = completeBenchmarkNode(t, service, "tenant-1", current, "campaign:"+current.NodeKey)
		if current.State != NodeSucceeded {
			t.Fatalf("fanout member %s did not complete", current.NodeKey)
		}
	}
	joined, err := service.JoinFanoutSet(t.Context(), "tenant-1", created.Set.ID, "runtime.join")
	if err != nil {
		t.Fatal(err)
	}
	if joined.Status != FanoutSucceeded {
		t.Fatalf("campaign join did not aggregate all ten variants: %#v", joined)
	}
	if _, err := service.Refresh(t.Context(), "tenant-1", started.Job.ID); err != nil {
		t.Fatal(err)
	}
	finalNodes, err := service.Nodes(t.Context(), "tenant-1", started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalNodes) != 13 {
		t.Fatalf("campaign benchmark changed plan shape unexpectedly: got %d nodes", len(finalNodes))
	}
	var merge NodeRun
	for _, node := range finalNodes {
		if node.NodeKey == "stage:merge" {
			merge = node
		}
	}
	if merge.State != NodeReady {
		t.Fatalf("join did not unlock the shared merge stage: %s", merge.State)
	}
}

func completeBenchmarkNode(t *testing.T, service *Service, tenantID string, node NodeRun, outputRef string) NodeRun {
	t.Helper()
	leased, err := service.TransitionNode(t.Context(), tenantID, node.ID, NodeLeased, "runtime", "benchmark-worker", node.Version)
	if err != nil {
		t.Fatal(err)
	}
	running, err := service.TransitionNode(t.Context(), tenantID, node.ID, NodeRunning, "worker", "benchmark-worker", leased.Version)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := service.CompleteNode(t.Context(), tenantID, node.ID, []string{outputRef}, "sha256:"+strings.Repeat("d", 64), "worker", "benchmark-worker", running.Version)
	if err != nil {
		t.Fatal(err)
	}
	return completed
}
