package postgres_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	storepg "github.com/limecloud/contentcloud/internal/store/postgres"
)

func TestRuntimePostgresClaimsOneHundredNodesWithTwentyConcurrentWorkers(t *testing.T) {
	databaseURL := os.Getenv("CONTENTCLOUD_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTENTCLOUD_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := storepg.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)

	service := app.New(store, slog.Default())
	suffix := domain.NewID()
	session, err := service.Register(ctx, fmt.Sprintf("capacity-%s@example.com", suffix), "long-enough-password", "Capacity", "Capacity "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id=$1`, actor.TenantID)
	}()
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Capacity", ProductName: "Runtime Capacity"}, "")
	if err != nil {
		t.Fatal(err)
	}

	sop := domain.SOPVersion{
		ID: "postgres-capacity-sop-v1", TenantID: actor.TenantID, SOPID: "postgres-capacity-sop", Version: 1,
		SchemaVersion: domain.SOPSchemaVersion, Name: "PostgreSQL Capacity", Status: "published", DefaultExecutionMode: "agent",
	}
	for index := 0; index < 100; index++ {
		sop.Stages = append(sop.Stages, domain.StageDefinition{ID: fmt.Sprintf("node-%03d", index), Name: fmt.Sprintf("Node %03d", index), Order: index + 1, OutputSchema: "contentcloud.capacity/1.0", ExecutionModes: []string{"agent"}})
	}
	started, err := service.Runtime().Start(ctx, contentStartInput(actor.TenantID, project.ID, "postgres-capacity-"+suffix, sop))
	if err != nil {
		t.Fatal(err)
	}
	capabilities := agentadapter.HarnessCapabilities{Kind: "fake", Events: true, Resume: true, StructuredOutput: true, MaxParallelSessions: 20}

	claimedNodes := map[string]bool{}
	claimedAttempts := map[string]bool{}
	var claimedMu sync.Mutex
	for wave := 0; wave < 5; wave++ {
		var wait sync.WaitGroup
		errorsByWorker := make(chan error, 20)
		for workerIndex := 0; workerIndex < 20; workerIndex++ {
			workerID := fmt.Sprintf("postgres-worker-%02d-%02d", wave, workerIndex)
			wait.Add(1)
			go func(owner string) {
				defer wait.Done()
				handle, dispatchErr := service.Runtime().PrepareRemoteDispatch(ctx, contentDispatchInput(actor.TenantID, started.Job.ID, owner), capabilities)
				if dispatchErr != nil {
					errorsByWorker <- dispatchErr
					return
				}
				claimedMu.Lock()
				defer claimedMu.Unlock()
				if claimedNodes[handle.Node.ID] || claimedAttempts[handle.Attempt.ID] {
					errorsByWorker <- fmt.Errorf("duplicate PostgreSQL lease: node=%s attempt=%s", handle.Node.ID, handle.Attempt.ID)
					return
				}
				claimedNodes[handle.Node.ID] = true
				claimedAttempts[handle.Attempt.ID] = true
			}(workerID)
		}
		wait.Wait()
		close(errorsByWorker)
		for workerErr := range errorsByWorker {
			if workerErr != nil {
				t.Fatal(workerErr)
			}
		}
	}
	if len(claimedNodes) != 100 || len(claimedAttempts) != 100 {
		t.Fatalf("PostgreSQL 100-node claim lost work: nodes=%d attempts=%d", len(claimedNodes), len(claimedAttempts))
	}
	attempts, err := service.Runtime().Attempts(ctx, actor.TenantID, started.Job.ID)
	if err != nil || len(attempts) != 100 {
		t.Fatalf("PostgreSQL created unexpected RuntimeAttempt count: attempts=%d err=%v", len(attempts), err)
	}
	if _, err := service.Runtime().PrepareRemoteDispatch(ctx, contentDispatchInput(actor.TenantID, started.Job.ID, "postgres-worker-overflow"), capabilities); !domain.IsNotFound(err) {
		t.Fatalf("PostgreSQL scheduler returned work after all 100 nodes were leased: %v", err)
	}
}

func contentStartInput(tenantID, projectID, idempotencyKey string, sop domain.SOPVersion) contentruntime.StartInput {
	return contentruntime.StartInput{TenantID: tenantID, ProjectID: projectID, WorkTaskID: "postgres-capacity-task-" + idempotencyKey, SOP: sop, BindingDigest: "sha256:" + repeatCapacityHex(64, 'a'), InputDigest: "sha256:" + repeatCapacityHex(64, 'b'), RuntimePolicyID: "runtime-policy/postgres-capacity-v1", ContractMajor: 1, ContractMinor: 0, CreatedBy: "capacity-test", IdempotencyKey: idempotencyKey}
}

func contentDispatchInput(tenantID, jobID, owner string) contentruntime.DispatchInput {
	return contentruntime.DispatchInput{TenantID: tenantID, JobRunID: jobID, Owner: owner, HarnessKind: "fake", Role: "capacity", ExecutionProfileID: "profile-capacity", MaxTokens: 128, LeaseFor: time.Minute}
}

func repeatCapacityHex(count int, value byte) string {
	bytes := make([]byte, count)
	for index := range bytes {
		bytes[index] = value
	}
	return string(bytes)
}
