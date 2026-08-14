package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func testSOP() domain.SOPVersion {
	return domain.SOPVersion{
		ID: "sop-v1", TenantID: "tenant-1", SOPID: "sop-1", Version: 1,
		SchemaVersion: domain.SOPSchemaVersion, Name: "营销测试流程", Status: "published",
		ContentTypes: []string{domain.ContentTypeMarketingVideo}, DefaultExecutionMode: "local",
		Stages: []domain.StageDefinition{
			{ID: "sources", Name: "资料准备", Order: 10, OutputSchema: "contentcloud.sources/1.0", ExecutionModes: []string{"local"}},
			{ID: "script", Name: "剧本方案", Order: 20, InputRefs: []string{"sources"}, OutputSchema: "contentcloud.script/1.0", ExecutionModes: []string{"agent"}, GateIDs: []string{"script_review"}},
			{ID: "delivery", Name: "交付", Order: 30, InputRefs: []string{"script"}, OutputSchema: "contentcloud.delivery/1.0", ExecutionModes: []string{"local"}},
		},
		Gates: []domain.GateDefinition{{ID: "script_review", Name: "剧本确认", Mode: domain.GateModeClientDecision, Blocking: true}},
	}
}

func testStartInput(workTaskID, idempotencyKey string) StartInput {
	return StartInput{
		TenantID: "tenant-1", ProjectID: "project-1", WorkTaskID: workTaskID, SOP: testSOP(),
		BindingDigest: "sha256:" + strings.Repeat("a", 64), InputDigest: "sha256:" + strings.Repeat("b", 64),
		RuntimePolicyID: "runtime-policy/test-v1", ContractMajor: 1, ContractMinor: 0,
		CreatedBy: "user-1", IdempotencyKey: idempotencyKey,
	}
}

func TestCompilerProducesDeterministicAcyclicPlan(t *testing.T) {
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	compiler := NewCompiler(domain.DefaultRuntimeLimits())
	one, err := compiler.CompileSOP(testSOP(), "tenant-1", "user-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Nodes) != 4 || len(one.Edges) != 3 {
		t.Fatalf("unexpected plan shape: nodes=%d edges=%d", len(one.Nodes), len(one.Edges))
	}
	if one.Nodes[2].Key != "gate:script_review" || one.Nodes[3].DependsOn[0] != "gate:script_review" {
		t.Fatalf("gate was not inserted before downstream stage: %#v", one.Nodes)
	}
	two, err := compiler.CompileSOP(testSOP(), "tenant-1", "user-1", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if one.Digest != two.Digest {
		t.Fatalf("plan digest changed with compile time: %s != %s", one.Digest, two.Digest)
	}
}

func TestRuntimeStartIdempotencyAndStateCAS(t *testing.T) {
	repo := memory.New()
	runtimeService := New(repo, func() time.Time { return time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC) })
	input := testStartInput("task-1", "job-1")
	first, err := runtimeService.Start(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtimeService.Start(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Job.ID != second.Job.ID || first.Plan.ID != second.Plan.ID {
		t.Fatalf("idempotency returned a different run: %s/%s", first.Job.ID, second.Job.ID)
	}
	if first.Job.State != domain.JobRunAdmitted {
		t.Fatalf("expected admission state, got %s", first.Job.State)
	}
	if first.Job.RootJobRunID != first.Job.ID || first.Job.BindingDigest != input.BindingDigest || first.Job.InputDigest != input.InputDigest || first.Job.RuntimePolicyID != input.RuntimePolicyID {
		t.Fatalf("runtime admission snapshot was not frozen: %#v", first.Job)
	}
	mismatched := input
	mismatched.InputDigest = "sha256:" + strings.Repeat("c", 64)
	if _, err := runtimeService.Start(t.Context(), mismatched); err == nil {
		t.Fatal("same idempotency key with a different admission snapshot must conflict")
	}
	nodes, err := runtimeService.Nodes(t.Context(), "tenant-1", first.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var source domain.NodeRun
	for _, node := range nodes {
		if node.NodeKey == "stage:sources" {
			source = node
		}
	}
	if source.State != domain.NodeReady {
		t.Fatalf("first node should be ready, got %s", source.State)
	}
	if _, err := runtimeService.TransitionNode(t.Context(), "tenant-1", source.ID, domain.NodeLeased, "runtime", "scheduler", source.Version); err != nil {
		t.Fatal(err)
	}
	source, err = repo.NodeRun(t.Context(), "tenant-1", source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeService.TransitionNode(t.Context(), "tenant-1", source.ID, domain.NodeRunning, "worker", "worker-1", source.Version); err != nil {
		t.Fatal(err)
	}
	source, err = repo.NodeRun(t.Context(), "tenant-1", source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeService.CompleteNode(t.Context(), "tenant-1", source.ID, []string{"source:1"}, "sha256:abc", "worker", "worker-1", source.Version); err != nil {
		t.Fatal(err)
	}
	state, err := runtimeService.MutateState(t.Context(), "tenant-1", first.Job.ID, domain.StateMutation{Collection: "brief", ExpectedRevision: 0, Set: map[string]any{"topic": "春日"}, IdempotencyKey: "state-1"}, "worker", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 1 || state.Values["topic"] != "春日" {
		t.Fatalf("unexpected runtime state: %#v", state)
	}
	if _, err := runtimeService.MutateState(t.Context(), "tenant-1", first.Job.ID, domain.StateMutation{Collection: "brief", ExpectedRevision: 0, Set: map[string]any{"topic": "旧值"}, IdempotencyKey: "state-2"}, "worker", "worker-1"); err == nil {
		t.Fatal("expected CAS conflict")
	}
}

func TestRuntimeStartPersistsStructuredExecutionBinding(t *testing.T) {
	repo := memory.New()
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	runtimeService := New(repo, func() time.Time { return now })
	input := testStartInput("task-binding", "job-binding")
	input.BindingDigest = ""
	input.ExecutionBinding = &domain.ExecutionBindingSnapshot{
		ProfileID: "profile.content-production", ProfileVersion: "2.1.0",
		RuntimePolicyID: input.RuntimePolicyID, HarnessKinds: []string{"fake"},
		AllowedTools: []string{ToolStateGet}, SandboxProfile: "fake", IsolationProfile: "workspace",
		EgressPolicy: "deny", DataClassification: "internal", MaxTokens: 2048,
		MaxDurationSeconds: 900, MaxCostMinor: 50, MaxDynamicDescendants: 4, FallbackPolicy: "none",
	}
	started, err := runtimeService.Start(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.ExecutionBindingSnapshot(t.Context(), input.TenantID, started.Job.BindingDigest)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Legacy || stored.ProfileID != input.ExecutionBinding.ProfileID || stored.MaxTokens != 2048 || stored.Digest != started.Job.BindingDigest {
		t.Fatalf("structured execution binding was not frozen: %#v", stored)
	}
	digest, err := stored.ContentDigest()
	if err != nil || digest != stored.Digest {
		t.Fatalf("binding digest mismatch: digest=%q err=%v stored=%q", digest, err, stored.Digest)
	}
}

func TestRuntimeStartMarksOpaqueBindingAsLegacy(t *testing.T) {
	repo := memory.New()
	runtimeService := New(repo, time.Now)
	input := testStartInput("task-legacy-binding", "job-legacy-binding")
	started, err := runtimeService.Start(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.ExecutionBindingSnapshot(t.Context(), input.TenantID, started.Job.BindingDigest)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Legacy || stored.Digest != input.BindingDigest {
		t.Fatalf("opaque compatibility binding was not marked legacy: %#v", stored)
	}
}

func TestRuntimeAvailableNotifierFiresOnlyAfterReadyStatePersists(t *testing.T) {
	repo := memory.New()
	runtimeService := New(repo, time.Now)
	notifications := make(chan string, 2)
	runtimeService.SetAvailableNotifier(func(tenantID string) { notifications <- tenantID })
	started, err := runtimeService.Start(t.Context(), testStartInput("task-notify", "job-notify"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case tenantID := <-notifications:
		if tenantID != "tenant-1" {
			t.Fatalf("notification tenant = %q", tenantID)
		}
	case <-time.After(time.Second):
		t.Fatal("ready Runtime did not publish an availability hint")
	}
	if _, err := runtimeService.Refresh(t.Context(), "tenant-1", started.Job.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case tenantID := <-notifications:
		if tenantID != "tenant-1" {
			t.Fatalf("refresh notification tenant = %q", tenantID)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh did not reassert available work")
	}
}

func TestEffectUnknownCannotBeRetriedBlindly(t *testing.T) {
	repo := memory.New()
	runtimeService := New(repo, time.Now)
	started, err := runtimeService.Start(t.Context(), testStartInput("task-1", ""))
	if err != nil {
		t.Fatal(err)
	}
	effect, err := runtimeService.RegisterEffect(t.Context(), domain.ExternalEffect{TenantID: "tenant-1", JobRunID: started.Job.ID, NodeRunID: started.Nodes[0].ID, Kind: "media.generate", IdempotencyKey: "effect-1", RequestDigest: "sha256:req", Currency: "CNY", SafeSummary: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	effect, err = runtimeService.ReconcileEffect(t.Context(), "tenant-1", effect.ID, domain.EffectUnknown, "", "", "TIMEOUT", effect.Version)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != domain.EffectUnknown {
		t.Fatalf("expected unknown state, got %s", effect.State)
	}
	if _, err := runtimeService.ReconcileEffect(t.Context(), "tenant-1", effect.ID, domain.EffectSubmitted, "external-2", "", "", effect.Version); err == nil {
		t.Fatal("unknown effect must reconcile before submission")
	}
}

func TestForkReusesCheckpointedNodeOutputsAndReplayRebuildsProjection(t *testing.T) {
	repo := memory.New()
	runtimeService := New(repo, time.Now)
	started, err := runtimeService.Start(t.Context(), testStartInput("task-fork", "source-job"))
	if err != nil {
		t.Fatal(err)
	}
	var sourceNode domain.NodeRun
	for _, node := range started.Nodes {
		if node.NodeKey == "stage:sources" {
			sourceNode = node
		}
	}
	if _, err := runtimeService.TransitionNode(t.Context(), "tenant-1", sourceNode.ID, domain.NodeLeased, "runtime", "scheduler", sourceNode.Version); err != nil {
		t.Fatal(err)
	}
	sourceNode, err = repo.NodeRun(t.Context(), "tenant-1", sourceNode.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeService.TransitionNode(t.Context(), "tenant-1", sourceNode.ID, domain.NodeRunning, "worker", "worker-1", sourceNode.Version); err != nil {
		t.Fatal(err)
	}
	sourceNode, err = repo.NodeRun(t.Context(), "tenant-1", sourceNode.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeService.CompleteNode(t.Context(), "tenant-1", sourceNode.ID, []string{"source:immutable"}, "sha256:source", "worker", "worker-1", sourceNode.Version); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := runtimeService.Checkpoint(t.Context(), "tenant-1", started.Job.ID, sourceNode.NodeKey, nil, []string{"source:immutable"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeService.Fork(t.Context(), "tenant-1", checkpoint.ID, "operator-1", "fork-active"); err == nil {
		t.Fatal("an active source job must not be forked")
	}
	if _, err := runtimeService.Cancel(t.Context(), "tenant-1", started.Job.ID, "user", "operator-1"); err != nil {
		t.Fatal(err)
	}
	forked, err := runtimeService.Fork(t.Context(), "tenant-1", checkpoint.ID, "operator-1", "fork-1")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := runtimeService.Fork(t.Context(), "tenant-1", checkpoint.ID, "operator-1", "fork-1")
	if err != nil || repeated.Job.ID != forked.Job.ID {
		t.Fatalf("fork idempotency returned a different job: %#v err=%v", repeated.Job, err)
	}
	if forked.Job.RootJobRunID != started.Job.ID || forked.Job.BindingDigest != started.Job.BindingDigest || forked.Job.InputDigest != started.Job.InputDigest || forked.Job.RuntimePolicyID != started.Job.RuntimePolicyID || forked.Job.ContractMajor != started.Job.ContractMajor || forked.Job.ContractMinor != started.Job.ContractMinor {
		t.Fatalf("fork did not inherit the immutable admission snapshot: %#v", forked.Job)
	}
	var reused, downstream domain.NodeRun
	for _, node := range forked.Nodes {
		switch node.NodeKey {
		case "stage:sources":
			reused = node
		case "stage:script":
			downstream = node
		}
	}
	if reused.State != domain.NodeSucceeded || reused.OutputDigest != "sha256:source" || len(reused.OutputRefs) != 1 || reused.OutputRefs[0] != "source:immutable" {
		t.Fatalf("fork did not reuse checkpointed output facts: %#v", reused)
	}
	if downstream.State != domain.NodeReady {
		t.Fatalf("fork did not continue after the checkpoint boundary: %#v", downstream)
	}
	sourceAfter, err := runtimeService.Job(t.Context(), "tenant-1", started.Job.ID)
	if err != nil || sourceAfter.State != domain.JobRunCancelled {
		t.Fatalf("fork changed the source job: %#v err=%v", sourceAfter, err)
	}
	replay, err := runtimeService.Replay(t.Context(), "tenant-1", forked.Job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if replay.ExternalCalls != 0 || !replay.ProjectionRebuilt || replay.IntegrityStatus != "verified" || replay.EventCount == 0 {
		t.Fatalf("unexpected replay result: %#v", replay)
	}
	projection, err := runtimeService.RuntimeExplorer(t.Context(), "tenant-1", forked.Job.ID)
	if err != nil || projection.JobRunID != forked.Job.ID || projection.LastEventSeq != replay.LastSequence {
		t.Fatalf("replay did not rebuild the runtime projection: %#v err=%v", projection, err)
	}
	dryRun, err := runtimeService.ReplayWithOptions(t.Context(), "tenant-1", forked.Job.ID, 0, true)
	if err != nil || !dryRun.DryRun || dryRun.ProjectionRebuilt || dryRun.RebuildRunID == "" || dryRun.ExternalCalls != 0 {
		t.Fatalf("dry-run projection replay violated read-only boundary: %#v err=%v", dryRun, err)
	}
	rebuildRuns, err := repo.RuntimeProjectionRebuilds(t.Context(), "tenant-1", forked.Job.ID)
	if err != nil || len(rebuildRuns) != 2 || rebuildRuns[0].Status != "completed" || rebuildRuns[0].Mode != "dry_run" || rebuildRuns[1].Mode != "rebuild" {
		t.Fatalf("projection rebuild facts were not persisted: %#v err=%v", rebuildRuns, err)
	}
}

func TestRuntimeResumeOnlyResumesPausedJob(t *testing.T) {
	repo := memory.New()
	runtimeService := New(repo, func() time.Time { return time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC) })
	started, err := runtimeService.Start(t.Context(), testStartInput("task-1", "resume-1"))
	if err != nil {
		t.Fatal(err)
	}
	job, err := repo.JobRun(t.Context(), "tenant-1", started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Transition(domain.JobRunPaused); err != nil {
		t.Fatal(err)
	}
	job.State = domain.JobRunPaused
	job.Version++
	commands, ok := any(repo).(RuntimeCommandStore)
	if !ok {
		t.Fatal("memory store must implement RuntimeCommandStore")
	}
	if _, err := commands.ApplyJobTransition(t.Context(), job, job.Version-1, domain.JobEvent{ID: domain.NewID(), TenantID: job.TenantID, JobRunID: job.ID, Type: "job.paused", ActorType: "test", Payload: map[string]any{}, OccurredAt: job.UpdatedAt}); err != nil {
		t.Fatal(err)
	}
	resumed, err := runtimeService.Resume(t.Context(), "tenant-1", job.ID, "user", "operator-1")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.State != domain.JobRunRunning {
		t.Fatalf("expected resumed job to be running, got %s", resumed.State)
	}
	if _, err := runtimeService.Resume(t.Context(), "tenant-1", job.ID, "user", "operator-1"); err == nil {
		t.Fatal("running job must not be resumed twice")
	}
}

func TestRuntimeNodeLeaseClaimHeartbeatAndExpiry(t *testing.T) {
	repo := memory.New()
	now := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	runtimeService := New(repo, func() time.Time { return now })
	started, err := runtimeService.Start(t.Context(), testStartInput("task-lease", "lease-job"))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := runtimeService.ClaimNode(t.Context(), "tenant-1", started.Job.ID, "worker-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.State != domain.NodeLeased || claimed.AttemptCount != 1 || claimed.LeaseOwner != "worker-a" {
		t.Fatalf("unexpected claim: %#v", claimed)
	}
	heartbeat, err := runtimeService.HeartbeatNode(t.Context(), "tenant-1", claimed.ID, "worker-a", claimed.Version, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if heartbeat.State != domain.NodeRunning || heartbeat.Version != claimed.Version+1 {
		t.Fatalf("unexpected heartbeat: %#v", heartbeat)
	}
	if _, err := runtimeService.HeartbeatNode(t.Context(), "tenant-1", claimed.ID, "worker-b", heartbeat.Version, time.Minute); err == nil {
		t.Fatal("different worker unexpectedly renewed the lease")
	}
	now = *heartbeat.LeaseExpiresAt
	if err := runtimeService.ExpireNodeLeases(t.Context(), "tenant-1", now); err != nil {
		t.Fatal(err)
	}
	expired, err := repo.NodeRun(t.Context(), "tenant-1", claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if expired.State != domain.NodeReady || expired.LeaseOwner != "" || expired.LeaseExpiresAt != nil {
		t.Fatalf("expired node was not returned to ready: %#v", expired)
	}
	if _, err := runtimeService.HeartbeatNode(t.Context(), "tenant-1", claimed.ID, "worker-a", expired.Version, time.Minute); err == nil {
		t.Fatal("expired worker unexpectedly renewed the lease")
	}
}
