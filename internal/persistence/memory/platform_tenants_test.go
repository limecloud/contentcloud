package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
)

func TestPlatformTenantsCountsRuntimeJobs(t *testing.T) {
	ctx := context.Background()
	store := New()
	now := time.Now().UTC()
	tenantID, userID, projectID, taskID := idgen.New(), idgen.New(), idgen.New(), idgen.New()
	if err := store.CreateTenant(ctx, identitydomain.Tenant{ID: tenantID, Slug: "runtime-counts", Name: "Runtime Counts", Status: "active", CreatedAt: now}, identitydomain.Membership{TenantID: tenantID, UserID: userID, Role: "tenant_admin", Status: "active", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	jobID := idgen.New()
	job := contentruntime.JobRun{ID: jobID, TenantID: tenantID, ProjectID: projectID, WorkTaskID: taskID, PlanRevisionID: idgen.New(), PlanDigest: "sha256:" + strings.Repeat("a", 64), BindingDigest: "sha256:" + strings.Repeat("b", 64), InputDigest: "sha256:" + strings.Repeat("c", 64), RuntimePolicyID: "runtime.test/1", ContractMajor: 1, RootJobRunID: jobID, State: contentruntime.JobRunRunning, Version: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now}
	event := contentruntime.JobEvent{ID: idgen.New(), TenantID: tenantID, JobRunID: jobID, Sequence: 1, Type: "job.created", ActorType: "test", OccurredAt: now}
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
