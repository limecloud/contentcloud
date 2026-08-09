package runtime

import (
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
)

func activeDispatchHandle(t *testing.T, service *Service, fake *agentadapter.FakeHarness, jobID string, input DispatchInput) DispatchHandle {
	t.Helper()
	handle, err := service.PrepareDispatch(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := fake.Start(t.Context(), agentadapter.StartAgentRequest{JobRunID: jobID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID})
	if err != nil {
		t.Fatal(err)
	}
	handle, err = service.ActivateDispatch(t.Context(), handle, session)
	if err != nil {
		t.Fatal(err)
	}
	return handle
}

func TestYieldReleasesLeaseAndResourcesThenResumesWithNewAttempt(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	service, repo, started := newDispatchRuntime(t, fake, time.Now)
	if err := repo.SaveResourceQuota(t.Context(), domain.ResourceQuota{TenantID: "tenant-1", ResourceKey: "agent.concurrent", Capacity: 1, Unit: "slots", Version: 1, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	input := dispatchInput(started.Job.ID)
	input.ResourceRequests = []domain.ResourceRequest{{ResourceKey: "agent.concurrent", Quantity: 1, Unit: "slots"}}
	handle := activeDispatchHandle(t, service, fake, started.Job.ID, input)
	sessionRef := handle.Agent.SessionRef

	yielded, err := service.YieldDispatch(t.Context(), handle, YieldDispatchInput{Reason: domain.YieldWaitHuman, WaitRefs: []string{"gate:approval-1"}, SafeSummary: map[string]any{"phase": "approval"}, UsedCostMinor: 10})
	if err != nil {
		t.Fatal(err)
	}
	if yielded.Yield.State != domain.RuntimeYieldOpen || yielded.Handle.Attempt.State != domain.RuntimeAttemptYielded || yielded.Handle.Node.State != domain.NodeWaitingHuman || yielded.Handle.Agent.State != domain.AgentWaitingGate {
		t.Fatalf("yield did not persist the waiting boundary: %#v", yielded)
	}
	if yielded.Handle.Node.LeaseOwner != "" || yielded.Handle.Attempt.FenceToken != "" || yielded.Handle.Agent.SessionRef != sessionRef {
		t.Fatalf("yield did not release execution ownership or preserve the opaque session: %#v", yielded.Handle)
	}
	reservations, err := repo.ResourceReservations(t.Context(), "tenant-1", started.Job.ID)
	if err != nil || len(reservations) != 1 || reservations[0].State != domain.ReservationReleased || reservations[0].ReleasedAt == nil {
		t.Fatalf("yield did not release held resources: %#v err=%v", reservations, err)
	}

	resolved, err := service.ResumeYield(t.Context(), "tenant-1", yielded.Yield.ID, "approve-1", "operator-1")
	if err != nil || resolved.State != domain.RuntimeYieldResolved {
		t.Fatalf("human wait was not resolved: %#v err=%v", resolved, err)
	}
	replayed, err := service.ResumeYield(t.Context(), "tenant-1", yielded.Yield.ID, "approve-1", "operator-1")
	if err != nil || replayed.Version != resolved.Version {
		t.Fatalf("yield resume was not idempotent: %#v err=%v", replayed, err)
	}
	resumed, err := service.PrepareDispatch(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Attempt.AttemptNo != 2 || resumed.ResumeSession == nil || resumed.ResumeSession.SessionID == "" || resumed.Agent.SessionRef != sessionRef {
		t.Fatalf("resume did not create a new Attempt bound to the prior session: %#v", resumed)
	}
}

func TestEffectYieldCannotResumeBeforeEffectConverges(t *testing.T) {
	fake := agentadapter.NewFakeHarness()
	service, _, started := newDispatchRuntime(t, fake, time.Now)
	handle := activeDispatchHandle(t, service, fake, started.Job.ID, dispatchInput(started.Job.ID))
	effect, err := service.RegisterEffect(t.Context(), domain.ExternalEffect{TenantID: "tenant-1", JobRunID: started.Job.ID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID, Kind: "media.generate", IdempotencyKey: "yield-effect-1", RequestDigest: "sha256:request", Currency: "CNY", SafeSummary: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	effect, err = service.ReconcileEffect(t.Context(), "tenant-1", effect.ID, domain.EffectSubmitted, "external-yield-1", "", "", effect.Version)
	if err != nil {
		t.Fatal(err)
	}
	effect, err = service.ReconcileEffect(t.Context(), "tenant-1", effect.ID, domain.EffectAcknowledged, "external-yield-1", "", "", effect.Version)
	if err != nil {
		t.Fatal(err)
	}
	yielded, err := service.YieldDispatch(t.Context(), handle, YieldDispatchInput{Reason: domain.YieldWaitEffect, WaitRefs: []string{effect.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResumeYield(t.Context(), "tenant-1", yielded.Yield.ID, "effect-resume-early", "reconciler"); !hasDomainCode(err, "RUNTIME_YIELD_NOT_READY") {
		t.Fatalf("pending effect must block resume, got %v", err)
	}
	effect, err = service.ReconcileEffect(t.Context(), "tenant-1", effect.ID, domain.EffectSucceeded, "external-yield-1", "sha256:result", "", effect.Version)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := service.ResumeYield(t.Context(), "tenant-1", yielded.Yield.ID, "effect-resume-ready", "reconciler")
	if err != nil || resolved.State != domain.RuntimeYieldResolved {
		t.Fatalf("successful effect did not release the wait: %#v err=%v", resolved, err)
	}
}
