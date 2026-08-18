package runtime_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/limecloud/contentcloud/internal/runtime"

	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/platform/fault"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

func TestRuntimeClaimsOneHundredIndependentNodesWithTwentyConcurrentWorkers(t *testing.T) {
	sop := testSOP()
	sop.ID, sop.SOPID, sop.Name = "capacity-sop-v1", "capacity-sop", "Runtime Capacity"
	sop.Gates = nil
	sop.Stages = make([]catalogdomain.StageDefinition, 0, 100)
	for index := 0; index < 100; index++ {
		sop.Stages = append(sop.Stages, catalogdomain.StageDefinition{ID: fmt.Sprintf("node-%03d", index), Name: fmt.Sprintf("Node %03d", index), Order: index + 1, OutputSchema: "contentcloud.capacity/1.0", ExecutionModes: []string{"agent"}})
	}
	repo := memory.New()
	service := New(repo, time.Now)
	input := testStartInput("capacity-task", "capacity-job")
	input.SOP = sop
	started, err := service.Start(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := repo.NodeRuns(t.Context(), "tenant-1", started.Job.ID)
	if err != nil || len(nodes) != 100 {
		t.Fatalf("Runtime did not admit exactly 100 nodes: nodes=%d err=%v", len(nodes), err)
	}

	capabilities := agentadapter.HarnessCapabilities{Kind: "fake", Events: true, Resume: true, StructuredOutput: true, MaxParallelSessions: 20}
	claimedNodes := map[string]bool{}
	claimedAttempts := map[string]bool{}
	var claimedMu sync.Mutex
	for wave := 0; wave < 5; wave++ {
		var wg sync.WaitGroup
		errorsByWorker := make(chan error, 20)
		for workerIndex := 0; workerIndex < 20; workerIndex++ {
			workerID := fmt.Sprintf("worker-%02d-%02d", wave, workerIndex)
			wg.Add(1)
			go func() {
				defer wg.Done()
				handle, err := service.PrepareRemoteDispatch(t.Context(), DispatchInput{TenantID: "tenant-1", JobRunID: started.Job.ID, Owner: workerID, HarnessKind: "fake", Role: "capacity", ExecutionProfileID: "profile-capacity", MaxTokens: 128, LeaseFor: time.Minute}, capabilities)
				if err != nil {
					errorsByWorker <- err
					return
				}
				claimedMu.Lock()
				if claimedNodes[handle.Node.ID] || claimedAttempts[handle.Attempt.ID] {
					errorsByWorker <- fmt.Errorf("duplicate lease: node=%s attempt=%s", handle.Node.ID, handle.Attempt.ID)
				} else {
					claimedNodes[handle.Node.ID] = true
					claimedAttempts[handle.Attempt.ID] = true
				}
				claimedMu.Unlock()
			}()
		}
		wg.Wait()
		close(errorsByWorker)
		for workerErr := range errorsByWorker {
			if workerErr != nil {
				t.Fatal(workerErr)
			}
		}
	}
	if len(claimedNodes) != 100 || len(claimedAttempts) != 100 {
		t.Fatalf("100-node/20-worker claim lost work: nodes=%d attempts=%d", len(claimedNodes), len(claimedAttempts))
	}
	if _, err := service.PrepareRemoteDispatch(t.Context(), DispatchInput{TenantID: "tenant-1", JobRunID: started.Job.ID, Owner: "worker-overflow", HarnessKind: "fake", Role: "capacity", ExecutionProfileID: "profile-capacity", MaxTokens: 128, LeaseFor: time.Minute}, capabilities); !fault.IsNotFound(err) {
		t.Fatalf("scheduler returned work after all 100 nodes were leased: %v", err)
	}
}
