package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	storepg "github.com/limecloud/contentcloud/internal/store/postgres"
)

// A response lost immediately after COMMIT must be recoverable by retrying the
// same admission. This is the failure mode that ordinary rollback tests miss.
func TestRuntimePostCommitFaultRecoversThroughIdempotency(t *testing.T) {
	databaseURL := os.Getenv("CONTENTCLOUD_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTENTCLOUD_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	var armed atomic.Bool
	store, err := storepg.New(ctx, databaseURL, storepg.WithCommitFaultInjector(func(scope string) error {
		if scope == "runtime.create_job_bundle:after_commit" && armed.CompareAndSwap(true, false) {
			return errors.New("simulated process crash")
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	service := app.New(store, slog.Default())
	suffix := domain.NewID()
	session, err := service.Register(ctx, fmt.Sprintf("post-commit-%s@example.com", suffix), "long-enough-password", "Post Commit", "Post Commit "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	defer func() { _, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id=$1`, actor.TenantID) }()
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Post Commit", ProductName: "Runtime"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sop := domain.SOPVersion{
		ID: domain.NewID(), TenantID: actor.TenantID, SOPID: domain.NewID(), Version: 1,
		SchemaVersion: domain.SOPSchemaVersion, Name: "Post Commit", Status: "published", DefaultExecutionMode: "agent",
		Stages: []domain.StageDefinition{{ID: "source", Name: "Source", Order: 1, OutputSchema: "contentcloud.test/1.0", ExecutionModes: []string{"agent"}}},
	}
	input := contentruntime.StartInput{TenantID: actor.TenantID, ProjectID: project.ID, WorkTaskID: "post-commit-task-" + suffix, SOP: sop, BindingDigest: "sha256:" + repeatHex(64, 'a'), InputDigest: "sha256:" + repeatHex(64, 'b'), RuntimePolicyID: "runtime-policy/post-commit", ContractMajor: 1, CreatedBy: actor.UserID, IdempotencyKey: "post-commit-start-" + suffix}
	armed.Store(true)
	first, err := service.Runtime().Start(ctx, input)
	if err != nil {
		t.Fatalf("Runtime did not recover the ambiguous post-commit result: %v", err)
	}
	if armed.Load() {
		t.Fatal("post-commit fault injector did not reach runtime.create_job_bundle")
	}
	replayed, err := service.Runtime().Start(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Job.ID != first.Job.ID || replayed.Job.IdempotencyKey != input.IdempotencyKey || len(replayed.Nodes) != 1 {
		t.Fatalf("idempotent retry returned the wrong runtime bundle: %#v", replayed)
	}
	jops, err := service.Runtime().Jobs(ctx, actor.TenantID, input.WorkTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(jops) != 1 {
		t.Fatalf("post-commit retry duplicated JobRun rows: %d", len(jops))
	}
	events, err := service.Runtime().Events(ctx, actor.TenantID, replayed.Job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "job.created" {
		t.Fatalf("post-commit retry duplicated Runtime events: %#v", events)
	}
}
