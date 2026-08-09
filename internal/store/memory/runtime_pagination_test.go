package memory

import (
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

func TestRuntimeRepositoryPagesAtStorageBoundary(t *testing.T) {
	store := New()
	now := time.Date(2026, time.August, 9, 10, 0, 0, 0, time.UTC)
	service := contentruntime.New(store, func() time.Time { return now })
	sop := domain.SOPVersion{
		ID: "sop-page-v1", TenantID: "tenant-page", SOPID: "sop-page", Version: 1, Name: "分页测试", Status: "published", SchemaVersion: domain.SOPSchemaVersion,
		Stages: []domain.StageDefinition{
			{ID: "first", Name: "第一步", Order: 10, OutputSchema: "contentcloud.page/1.0", ExecutionModes: []string{"local"}},
			{ID: "second", Name: "第二步", Order: 20, InputRefs: []string{"first"}, OutputSchema: "contentcloud.page/1.0", ExecutionModes: []string{"local"}},
		},
	}
	started, err := service.Start(t.Context(), contentruntime.StartInput{TenantID: "tenant-page", ProjectID: "project-page", WorkTaskID: "task-page", SOP: sop, BindingDigest: "sha256:" + repeatPaginationHex('b'), InputDigest: "sha256:" + repeatPaginationHex('c'), RuntimePolicyID: contentruntime.DefaultRuntimePolicyID, ContractMajor: contentruntime.RuntimeContractMajor, CreatedBy: "operator", IdempotencyKey: "page-start"})
	if err != nil {
		t.Fatal(err)
	}
	nodes, hasMore, err := store.NodeRunsPage(t.Context(), "tenant-page", started.Job.ID, 0, 1)
	if err != nil || len(nodes) != 1 || !hasMore || nodes[0].NodeKey != "stage:first" {
		t.Fatalf("unexpected first node page: nodes=%#v has_more=%v err=%v", nodes, hasMore, err)
	}
	nodes, hasMore, err = store.NodeRunsPage(t.Context(), "tenant-page", started.Job.ID, 1, 1)
	if err != nil || len(nodes) != 1 || hasMore || nodes[0].NodeKey != "stage:second" {
		t.Fatalf("unexpected second node page: nodes=%#v has_more=%v err=%v", nodes, hasMore, err)
	}
	events, err := store.JobEventsPage(t.Context(), "tenant-page", started.Job.ID, 0, 1)
	if err != nil || len(events) != 1 || events[0].Sequence != 1 {
		t.Fatalf("unexpected event page: events=%#v err=%v", events, err)
	}
}

func repeatPaginationHex(value byte) string {
	result := make([]byte, 64)
	for index := range result {
		result[index] = value
	}
	return string(result)
}
