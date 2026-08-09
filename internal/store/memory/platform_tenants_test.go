package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestPlatformTenantsCountsRuntimeJobs(t *testing.T) {
	ctx := context.Background()
	store := New()
	now := time.Now().UTC()
	tenantID, userID, projectID, taskID := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	if err := store.CreateTenant(ctx, domain.Tenant{ID: tenantID, Slug: "runtime-counts", Name: "Runtime Counts", Status: "active", CreatedAt: now}, domain.Membership{TenantID: tenantID, UserID: userID, Role: "tenant_admin", Status: "active", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	jobID := domain.NewID()
	job := domain.JobRun{ID: jobID, TenantID: tenantID, ProjectID: projectID, WorkTaskID: taskID, PlanRevisionID: domain.NewID(), PlanDigest: "sha256:" + strings.Repeat("a", 64), BindingDigest: "sha256:" + strings.Repeat("b", 64), InputDigest: "sha256:" + strings.Repeat("c", 64), RuntimePolicyID: "runtime.test/1", ContractMajor: 1, RootJobRunID: jobID, State: domain.JobRunRunning, Version: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now}
	event := domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: jobID, Sequence: 1, Type: "job.created", ActorType: "test", OccurredAt: now}
	if err := store.CreateJobBundle(ctx, job, nil, event); err != nil {
		t.Fatal(err)
	}
	tenantViews, err := store.PlatformTenants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tenantViews) != 1 || tenantViews[0].ActiveRunCount != 1 {
		t.Fatalf("runtime jobs were not counted: %#v", tenantViews)
	}
}
