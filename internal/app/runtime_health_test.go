package app

import (
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestPlatformRuntimeHealthReportsHeartbeatAndBacklogThresholds(t *testing.T) {
	store := memory.New()
	service := New(store, nil, WithPlatformAdminEmails("runtime-health@example.com"))
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	session, err := service.Register(t.Context(), "runtime-health@example.com", "long-enough-password", "Runtime Health", "Runtime Health")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PlatformRuntimeHealth(t.Context(), Actor{TenantID: actor.TenantID}); err == nil {
		t.Fatal("non-platform actor accessed Runtime health")
	}

	report, err := service.PlatformRuntimeHealth(t.Context(), actor)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "critical" || !runtimeHealthHasAlert(report, "RUNTIME_REAPER_STALLED") {
		t.Fatalf("missing reaper heartbeat was not critical: %#v", report)
	}

	for _, kind := range []string{domain.RuntimeMaintenanceReaper, domain.RuntimeMaintenanceDelivery} {
		success := now
		if err := store.SaveRuntimeMaintenanceHeartbeat(t.Context(), domain.RuntimeMaintenanceHeartbeat{TenantID: actor.TenantID, Kind: kind, WorkerID: "worker-1", State: "succeeded", LastStartedAt: now, LastSuccessAt: &success, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	project, err := service.CreateProject(t.Context(), actor, CreateProjectInput{BrandName: "Runtime", ProductName: "Health"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sop := domain.SOPVersion{ID: "runtime-health-sop-v1", TenantID: actor.TenantID, SOPID: "runtime-health-sop", Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: "Runtime Health", Status: "published", DefaultExecutionMode: "agent", Digest: "sha256:" + strings.Repeat("a", 64), Stages: []domain.StageDefinition{{ID: "execute", Name: "Execute", Order: 10, OutputSchema: "contentcloud.runtime-health/1.0", ExecutionModes: []string{"agent"}}}}
	if _, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: actor.TenantID, ProjectID: project.ID, WorkTaskID: "runtime-health-task", BusinessType: "runtime.health", SOP: sop, BindingDigest: "sha256:" + strings.Repeat("b", 64), InputDigest: "sha256:" + strings.Repeat("c", 64), RuntimePolicyID: "runtime-policy/health", ContractMajor: 1, CreatedBy: actor.UserID, IdempotencyKey: "runtime-health-job"}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Minute)
	report, err = service.PlatformRuntimeHealth(t.Context(), actor)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "critical" || !runtimeHealthHasAlert(report, "RUNTIME_REAPER_STALLED") || !runtimeHealthHasAlert(report, "RUNTIME_PROJECTION_LAG") {
		t.Fatalf("stalled Runtime health was not surfaced: %#v", report)
	}
}

func runtimeHealthHasAlert(report RuntimeHealthReport, code string) bool {
	for _, tenant := range report.Tenants {
		for _, alert := range tenant.Alerts {
			if alert.Code == code {
				return true
			}
		}
	}
	return false
}
