package runtime

import (
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
	input := StartInput{TenantID: "tenant-1", ProjectID: "project-1", WorkTaskID: "task-1", SOP: testSOP(), CreatedBy: "user-1", IdempotencyKey: "job-1"}
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

func TestEffectUnknownCannotBeRetriedBlindly(t *testing.T) {
	repo := memory.New()
	runtimeService := New(repo, time.Now)
	started, err := runtimeService.Start(t.Context(), StartInput{TenantID: "tenant-1", ProjectID: "project-1", WorkTaskID: "task-1", SOP: testSOP(), CreatedBy: "user-1"})
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

func TestRuntimeResumeOnlyResumesPausedJob(t *testing.T) {
	repo := memory.New()
	runtimeService := New(repo, func() time.Time { return time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC) })
	started, err := runtimeService.Start(t.Context(), StartInput{TenantID: "tenant-1", ProjectID: "project-1", WorkTaskID: "task-1", SOP: testSOP(), CreatedBy: "user-1", IdempotencyKey: "resume-1"})
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
	started, err := runtimeService.Start(t.Context(), StartInput{TenantID: "tenant-1", ProjectID: "project-1", WorkTaskID: "task-lease", SOP: testSOP(), CreatedBy: "user-1", IdempotencyKey: "lease-job"})
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
