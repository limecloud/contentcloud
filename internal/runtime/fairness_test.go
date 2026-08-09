package runtime

import (
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestFairnessReportExcludesExpiredReservationsAndComputesJainIndex(t *testing.T) {
	repo := memory.New()
	service := New(repo, time.Now)
	now := time.Now().UTC()
	for _, tenantID := range []string{"tenant-1", "tenant-2"} {
		if err := repo.SaveResourceQuota(t.Context(), domain.ResourceQuota{TenantID: tenantID, ResourceKey: "agent.concurrent", Capacity: 4, Unit: "slots", Version: 1, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	first := testStartInput("fairness-1", domain.NewID())
	first.SOP.TenantID = first.TenantID
	startedA, err := service.Start(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	second := testStartInput("fairness-2", domain.NewID())
	second.TenantID = "tenant-2"
	second.ProjectID = "project-2"
	second.SOP.TenantID = second.TenantID
	second.SOP.ID = "sop-tenant-2"
	startedB, err := service.Start(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}
	caps := agentadapter.HarnessCapabilities{Kind: "fake", Events: true, Resume: true, StructuredOutput: true, MaxParallelSessions: 8}
	for _, input := range []DispatchInput{
		{TenantID: "tenant-1", JobRunID: startedA.Job.ID, Owner: "worker-a", HarnessKind: "fake", Role: "worker", ExecutionProfileID: "fairness", MaxTokens: 128, LeaseFor: time.Minute, ResourceRequests: []domain.ResourceRequest{{ResourceKey: "agent.concurrent", Quantity: 2, Unit: "slots"}}},
		{TenantID: "tenant-2", JobRunID: startedB.Job.ID, Owner: "worker-b", HarnessKind: "fake", Role: "worker", ExecutionProfileID: "fairness", MaxTokens: 128, LeaseFor: time.Minute, ResourceRequests: []domain.ResourceRequest{{ResourceKey: "agent.concurrent", Quantity: 1, Unit: "slots"}}},
	} {
		if _, err := service.PrepareRemoteDispatch(t.Context(), input, caps); err != nil {
			t.Fatal(err)
		}
	}
	report, err := service.FairnessReport(t.Context(), []string{"tenant-2", "tenant-1", "tenant-1"}, "agent.concurrent")
	if err != nil {
		t.Fatal(err)
	}
	if report.JainIndexBPS != 9000 || report.TotalCapacity != 8 || report.TotalHeld != 3 || report.MinUtilizationBPS != 2500 || report.MaxUtilizationBPS != 5000 {
		t.Fatalf("unexpected fairness report: %#v", report)
	}
	if len(report.Tenants) != 2 || report.Tenants[0].TenantID != "tenant-1" || report.Tenants[1].TenantID != "tenant-2" {
		t.Fatalf("fairness tenants are not deterministic: %#v", report.Tenants)
	}
}

func TestCapacityRowSeparatesExpiredHeldReservations(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	expires := now.Add(-time.Second)
	row := capacityRow("tenant-1", "agent.concurrent", domain.ResourceQuota{Capacity: 4, Unit: "slots"}, []domain.ResourceReservation{
		{ResourceKey: "agent.concurrent", Quantity: 2, State: domain.ReservationHeld, ExpiresAt: &expires},
		{ResourceKey: "agent.concurrent", Quantity: 1, State: domain.ReservationHeld, ExpiresAt: ptrTime(now.Add(time.Minute))},
	}, now)
	if row.Held != 1 || row.ExpiredHeld != 2 || row.UtilizationBPS != 2500 {
		t.Fatalf("expired held reservations were counted as live capacity: %#v", row)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }
