package runtime_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/limecloud/contentcloud/internal/runtime"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
)

func newDispatchRuntime(t *testing.T, fake *agentadapter.FakeHarness, now func() time.Time) (*Service, *memory.Store, StartResult) {
	t.Helper()
	repo := memory.New()
	registry := agentadapter.NewHarnessRegistry()
	if err := registry.Register("fake", fake); err != nil {
		t.Fatal(err)
	}
	service := NewWithHarnessRegistry(repo, now, registry)
	started, err := service.Start(t.Context(), testStartInput("task-dispatch", idgen.New()))
	if err != nil {
		t.Fatal(err)
	}
	return service, repo, started
}

func dispatchInput(jobID string) DispatchInput {
	return DispatchInput{
		TenantID: "tenant-1", JobRunID: jobID, Owner: "worker-1", HarnessKind: "fake",
		Role: "node_executor", ExecutionProfileID: "profile-fake", InputRefs: []string{"asset:source-1"},
		StateRefs: []string{"brief@1"}, AllowedTools: []string{"artifact.resolve", "state.get"},
		MaxTokens: 4096, BudgetMinor: 100, RemainingDescendants: 2, LeaseFor: time.Minute,
	}
}

func TestDispatchNextCompletesNodeAttemptAgentAndRefreshesJob(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	result := json.RawMessage(`{"output_refs":["asset:result-1"],"output_digest":"sha256:result","safe_summary":{"kind":"script","api_key":"must-not-persist"},"used_cost_minor":20}`)
	fake.QueueScript(agentadapter.FakeHarnessScript{Events: []agentadapter.FakeHarnessScriptEvent{{Type: "progress", Data: json.RawMessage(`{"percent":50}`)}, {Type: "result.completed", Data: result}}})
	service, repo, started := newDispatchRuntime(t, fake, time.Now)

	dispatched, err := service.DispatchNext(t.Context(), dispatchInput(started.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	if dispatched.Handle.Attempt.State != RuntimeAttemptSucceeded || dispatched.Handle.Node.State != NodeSucceeded || dispatched.Handle.Agent.State != AgentCompleted {
		t.Fatalf("dispatch did not converge atomically: %#v", dispatched.Handle)
	}
	if dispatched.Handle.Attempt.ResultDigest == "" || dispatched.Handle.Node.OutputDigest != "sha256:result" || dispatched.Handle.Agent.UsedCostMinor != 20 {
		t.Fatalf("dispatch result metadata was not persisted: %#v", dispatched.Handle)
	}
	if dispatched.Handle.Attempt.SafeSummary["api_key"] != "[redacted]" {
		t.Fatalf("unsafe summary field was persisted: %#v", dispatched.Handle.Attempt.SafeSummary)
	}
	attempts, err := repo.RuntimeAttempts(t.Context(), "tenant-1", started.Job.ID)
	if err != nil || len(attempts) != 1 || attempts[0].ContextViewID != dispatched.Handle.ContextView.ID {
		t.Fatalf("unexpected persisted RuntimeAttempt: %#v err=%v", attempts, err)
	}
	events, err := repo.JobEvents(t.Context(), "tenant-1", started.Job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"attempt.prepared": false, "attempt.running": false, "attempt.event": false, "attempt.succeeded": false}
	for _, event := range events {
		if _, ok := wanted[event.Type]; ok {
			wanted[event.Type] = true
		}
	}
	for eventType, seen := range wanted {
		if !seen {
			t.Fatalf("missing dispatch event %s in %#v", eventType, events)
		}
	}
}

func TestPrepareDispatchRejectsPausedJobBeforeLeasing(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	service, repo, started := newDispatchRuntime(t, fake, time.Now)
	if _, err := service.Pause(t.Context(), "tenant-1", started.Job.ID, "user", "operator-1"); err != nil {
		t.Fatal(err)
	}

	if _, err := service.PrepareDispatch(t.Context(), dispatchInput(started.Job.ID)); !hasDomainCode(err, "JOB_RUN_PAUSED") {
		t.Fatalf("paused job must reject new dispatch leases, got %v", err)
	}
	attempts, err := repo.RuntimeAttempts(t.Context(), "tenant-1", started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 0 {
		t.Fatalf("paused dispatch created runtime attempts: %#v", attempts)
	}
}

func TestDispatchStartFailureReturnsNodeAndAgentToRetryableState(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	fake.QueueScript(agentadapter.FakeHarnessScript{StartError: errors.New("fake start failed")})
	service, repo, started := newDispatchRuntime(t, fake, time.Now)

	dispatched, err := service.DispatchNext(t.Context(), dispatchInput(started.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	if dispatched.Handle.Attempt.State != RuntimeAttemptRetryableFailed || dispatched.Handle.Attempt.ErrorCode != "HARNESS_START_FAILED" || dispatched.Handle.Agent.State != AgentRunnable {
		t.Fatalf("start failure did not converge: %#v", dispatched.Handle)
	}
	node, err := repo.NodeRun(t.Context(), "tenant-1", dispatched.Handle.Node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if node.State != NodeReady || node.LeaseOwner != "" || node.LeaseExpiresAt != nil {
		t.Fatalf("retryable node was not returned to ready: %#v", node)
	}
}

func TestDispatchStreamCloseWithoutTerminalIsRetryable(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	fake.QueueScript(agentadapter.FakeHarnessScript{})
	service, _, started := newDispatchRuntime(t, fake, time.Now)

	dispatched, err := service.DispatchNext(t.Context(), dispatchInput(started.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	if dispatched.Handle.Attempt.State != RuntimeAttemptRetryableFailed || dispatched.Handle.Attempt.ErrorCode != "HARNESS_STREAM_CLOSED" {
		t.Fatalf("closed stream did not become retryable failure: %#v", dispatched.Handle.Attempt)
	}
}

func TestDispatchMissingEventStreamIsRetryable(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	fake.QueueScript(agentadapter.FakeHarnessScript{MissingStream: true})
	service, repo, started := newDispatchRuntime(t, fake, time.Now)

	dispatched, err := service.DispatchNext(t.Context(), dispatchInput(started.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	if dispatched.Handle.Attempt.State != RuntimeAttemptRetryableFailed || dispatched.Handle.Attempt.ErrorCode != "HARNESS_STREAM_MISSING" {
		t.Fatalf("missing stream did not become retryable failure: %#v", dispatched.Handle.Attempt)
	}
	node, err := repo.NodeRun(t.Context(), "tenant-1", dispatched.Handle.Node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if node.State != NodeReady || dispatched.Handle.Agent.State != AgentRunnable {
		t.Fatalf("missing stream did not release dispatch ownership: node=%#v agent=%#v", node, dispatched.Handle.Agent)
	}
}

func TestDispatchHeartbeatKeepsShortLeaseAlive(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	result := json.RawMessage(`{"output_refs":[],"safe_summary":{},"used_cost_minor":0}`)
	fake.QueueScript(agentadapter.FakeHarnessScript{Events: []agentadapter.FakeHarnessScriptEvent{{Type: "result.completed", Data: result, Delay: 120 * time.Millisecond}}})
	service, _, started := newDispatchRuntime(t, fake, time.Now)
	input := dispatchInput(started.Job.ID)
	input.LeaseFor = 60 * time.Millisecond

	dispatched, err := service.DispatchNext(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if dispatched.Handle.Attempt.State != RuntimeAttemptSucceeded || dispatched.Handle.Attempt.Version < 4 {
		t.Fatalf("heartbeat did not renew attempt lease: %#v", dispatched.Handle.Attempt)
	}
}

func TestFinalizeDispatchIsIdempotentForSameDigestAndRejectsConflict(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	service, _, started := newDispatchRuntime(t, fake, time.Now)
	handle, err := service.PrepareDispatch(t.Context(), dispatchInput(started.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := fake.Start(t.Context(), agentadapter.StartAgentRequest{JobRunID: handle.Node.JobRunID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID})
	if err != nil {
		t.Fatal(err)
	}
	handle, err = service.ActivateDispatch(t.Context(), handle, session)
	if err != nil {
		t.Fatal(err)
	}
	outcome := DispatchOutcome{State: RuntimeAttemptSucceeded, OutputRefs: []string{"asset:1"}, OutputDigest: "sha256:output", ResultDigest: "sha256:terminal", SafeSummary: map[string]any{}, UsedCostMinor: 0}
	first, err := service.FinalizeDispatch(t.Context(), handle, outcome)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.FinalizeDispatch(t.Context(), first.Handle, outcome)
	if err != nil || second.Handle.Attempt.ID != first.Handle.Attempt.ID {
		t.Fatalf("identical terminal result was not idempotent: %#v err=%v", second, err)
	}
	outcome.ResultDigest = "sha256:different"
	if _, err := service.FinalizeDispatch(t.Context(), first.Handle, outcome); err == nil {
		t.Fatal("conflicting terminal result was accepted")
	}
}

func TestExpireDispatchLeaseExpiresAttemptAndReleasesAgent(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	fake := agentadapter.NewFakeHarness()
	service, repo, started := newDispatchRuntime(t, fake, func() time.Time { return now })
	input := dispatchInput(started.Job.ID)
	input.LeaseFor = time.Minute
	handle, err := service.PrepareDispatch(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := fake.Start(t.Context(), agentadapter.StartAgentRequest{JobRunID: handle.Node.JobRunID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ActivateDispatch(t.Context(), handle, session); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if err := service.ExpireNodeLeases(t.Context(), "tenant-1", now); err != nil {
		t.Fatal(err)
	}
	attempt, err := repo.RuntimeAttempt(t.Context(), "tenant-1", handle.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := repo.AgentInstance(t.Context(), "tenant-1", handle.Agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	node, err := repo.NodeRun(t.Context(), "tenant-1", handle.Node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.State != RuntimeAttemptExpired || agent.State != AgentRunnable || node.State != NodeReady {
		t.Fatalf("lease expiry did not converge all runtime records: node=%#v attempt=%#v agent=%#v", node, attempt, agent)
	}
}

func TestFinalizeDispatchRejectsExpiredLeaseBeforeReclamation(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	fake := agentadapter.NewFakeHarness()
	service, repo, started := newDispatchRuntime(t, fake, func() time.Time { return now })
	input := dispatchInput(started.Job.ID)
	input.LeaseFor = time.Minute
	handle, err := service.PrepareDispatch(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := fake.Start(t.Context(), agentadapter.StartAgentRequest{JobRunID: handle.Node.JobRunID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID})
	if err != nil {
		t.Fatal(err)
	}
	handle, err = service.ActivateDispatch(t.Context(), handle, session)
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(time.Minute + time.Nanosecond)
	_, err = service.FinalizeDispatch(t.Context(), handle, DispatchOutcome{State: RuntimeAttemptSucceeded, ResultDigest: "sha256:late-result", SafeSummary: map[string]any{}, UsedCostMinor: 0})
	if err == nil || !hasDomainCode(err, "DISPATCH_LEASE_STALE") {
		t.Fatalf("expired lease accepted terminal result: %v", err)
	}

	if err := service.ExpireNodeLeases(t.Context(), "tenant-1", now); err != nil {
		t.Fatal(err)
	}
	attempt, err := repo.RuntimeAttempt(t.Context(), "tenant-1", handle.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := repo.AgentInstance(t.Context(), "tenant-1", handle.Agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	node, err := repo.NodeRun(t.Context(), "tenant-1", handle.Node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.State != RuntimeAttemptExpired || agent.State != AgentRunnable || node.State != NodeReady {
		t.Fatalf("lease reclamation did not converge after stale terminal rejection: node=%#v attempt=%#v agent=%#v", node, attempt, agent)
	}
}

func TestPrepareDispatchConcurrentWorkersCreateUniqueAttempts(t *testing.T) {
	const workers = 20
	sop := testSOP()
	sop.ID, sop.SOPID, sop.Name = "parallel-v1", "parallel", "并发调度流程"
	sop.Stages = make([]catalogdomain.StageDefinition, 0, workers)
	sop.Gates = nil
	for index := 0; index < workers; index++ {
		sop.Stages = append(sop.Stages, catalogdomain.StageDefinition{ID: fmt.Sprintf("node-%02d", index), Name: fmt.Sprintf("并发节点 %02d", index), Order: index + 1, OutputSchema: "contentcloud.test/1.0", ExecutionModes: []string{"agent"}})
	}
	repo := memory.New()
	fake := agentadapter.NewFakeHarness()
	registry := agentadapter.NewHarnessRegistry()
	if err := registry.Register("fake", fake); err != nil {
		t.Fatal(err)
	}
	service := NewWithHarnessRegistry(repo, time.Now, registry)
	startInput := testStartInput("task-parallel", "parallel-job")
	startInput.SOP = sop
	started, err := service.Start(t.Context(), startInput)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	var lock sync.Mutex
	handles := make([]DispatchHandle, 0, workers)
	errorsSeen := make([]error, 0)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			input := dispatchInput(started.Job.ID)
			input.Owner = fmt.Sprintf("worker-%02d", worker)
			handle, err := service.PrepareDispatch(t.Context(), input)
			lock.Lock()
			defer lock.Unlock()
			if err != nil {
				errorsSeen = append(errorsSeen, err)
				return
			}
			handles = append(handles, handle)
		}(index)
	}
	wait.Wait()
	if len(errorsSeen) != 0 || len(handles) != workers {
		t.Fatalf("concurrent prepare failed: handles=%d errors=%v", len(handles), errorsSeen)
	}
	nodeIDs, attemptIDs := map[string]bool{}, map[string]bool{}
	for _, handle := range handles {
		if nodeIDs[handle.Node.ID] || attemptIDs[handle.Attempt.ID] {
			t.Fatalf("duplicate dispatch created: %#v", handle)
		}
		nodeIDs[handle.Node.ID] = true
		attemptIDs[handle.Attempt.ID] = true
	}
}

func TestRetryCreatesNewAttemptAndContextButReusesLogicalAgent(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	fake.QueueScript(agentadapter.FakeHarnessScript{StartError: errors.New("first failure")})
	fake.QueueScript(agentadapter.FakeHarnessScript{StartError: errors.New("second failure")})
	service, repo, started := newDispatchRuntime(t, fake, time.Now)

	first, err := service.DispatchNext(t.Context(), dispatchInput(started.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.DispatchNext(t.Context(), dispatchInput(started.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	if first.Handle.Agent.ID != second.Handle.Agent.ID || first.Handle.Attempt.ID == second.Handle.Attempt.ID || first.Handle.ContextView.ID == second.Handle.ContextView.ID {
		t.Fatalf("retry identity boundaries are wrong: first=%#v second=%#v", first.Handle, second.Handle)
	}
	attempts, err := repo.RuntimeAttempts(t.Context(), "tenant-1", started.Job.ID)
	if err != nil || len(attempts) != 2 || attempts[0].AttemptNo != 1 || attempts[1].AttemptNo != 2 {
		t.Fatalf("unexpected retry attempts: %#v err=%v", attempts, err)
	}
}
