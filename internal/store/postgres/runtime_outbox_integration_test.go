package postgres_test

import (
	"context"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	storepg "github.com/limecloud/contentcloud/internal/store/postgres"
)

func TestRuntimeOutboxPostgresClaimAndCommandRollback(t *testing.T) {
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

	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	tenantID, projectID, taskID, environmentID, sopID := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	sopVersionID := domain.NewID()
	planID, jobID, nodeID := domain.NewID(), domain.NewID(), domain.NewID()
	defer func() {
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	}()
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenants(id,slug,name,status,created_at) VALUES($1,$2,$3,'active',$4);
		INSERT INTO brand_projects(id,tenant_id,slug,brand_name,product_name,channel,status,owner_name,reviewer_name,client_approver,created_at,updated_at) VALUES($5,$1,$6,'Runtime test','Runtime test','douyin','active','','','',$4,$4);
		INSERT INTO environments(tenant_id,id,name,slug,status,default_sop_id,default_sop_version,created_at,updated_at) VALUES($1,$7,'Runtime test','runtime-test','active',$8,1,$4,$4);
		INSERT INTO sop_definitions(tenant_id,id,name,current_version,created_by,created_at,updated_at) VALUES($1,$8,'Runtime test',1,'test',$4,$4);
		INSERT INTO sop_versions(tenant_id,id,sop_id,version,schema_version,name,content_types,stages,gates,default_execution_mode,digest,status,created_by,published_by,created_at,published_at) VALUES($1,$9,$8,1,$10,'Runtime test','["marketing_video"]','[{"id":"source","name":"Source","order":10,"output_schema":"contentcloud.source/1.0","execution_modes":["local"]}]','[]','local',$11,'published','test','test',$4,$4);
		INSERT INTO work_tasks(tenant_id,id,project_id,environment_id,sop_id,sop_version,sop_digest,title,content_type,priority,risk_profile,status,created_by,created_at,updated_at) VALUES($1,$12,$5,$7,$8,1,$11,'Runtime test','marketing_video','normal','low','ready','test',$4,$4)
	`, tenantID, "runtime-test-"+tenantID, "Runtime test", now, projectID, "runtime-test-"+projectID, environmentID, sopID, sopVersionID, domain.SOPSchemaVersion, "sha256:"+repeatHex(64, 'a'), taskID); err != nil {
		t.Fatal(err)
	}

	sop := domain.SOPVersion{ID: sopVersionID, TenantID: tenantID, SOPID: sopID, Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: "Runtime test", Status: "published", Digest: "sha256:" + repeatHex(64, 'a'), ContentTypes: []string{"marketing_video"}, Stages: []domain.StageDefinition{{ID: "source", Name: "Source", Order: 10, OutputSchema: "contentcloud.source/1.0", ExecutionModes: []string{"local"}}}}
	plan, err := contentruntime.NewCompiler(domain.DefaultRuntimeLimits()).CompileSOP(sop, tenantID, "test", now)
	if err != nil {
		t.Fatal(err)
	}
	plan.ID = planID
	if err := store.CreatePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	job := domain.JobRun{ID: jobID, TenantID: tenantID, ProjectID: projectID, WorkTaskID: taskID, PlanRevisionID: plan.ID, PlanDigest: plan.Digest, State: domain.JobRunCreated, Version: 1, CreatedBy: "test", CreatedAt: now, UpdatedAt: now}
	node := domain.NodeRun{ID: nodeID, TenantID: tenantID, JobRunID: jobID, NodeKey: "stage:source", State: domain.NodePending, OutputRefs: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now}
	initialEvent := domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: jobID, Sequence: 1, Type: "job.created", ActorType: "test", Payload: map[string]any{}, OccurredAt: now}
	if err := store.CreateJobBundle(ctx, job, []domain.NodeRun{node}, initialEvent); err != nil {
		t.Fatal(err)
	}
	commands, ok := any(store).(contentruntime.RuntimeCommandStore)
	if !ok {
		t.Fatal("PostgreSQL store must implement RuntimeCommandStore")
	}

	claimed, err := commands.ClaimRuntimeOutbox(ctx, tenantID, "projector-a", now, time.Minute, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("expected one claimed message, got %#v, err=%v", claimed, err)
	}
	var second []domain.RuntimeOutboxMessage
	var secondErr error
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		second, secondErr = commands.ClaimRuntimeOutbox(ctx, tenantID, "projector-b", now, time.Minute, 1)
	}()
	wait.Wait()
	if secondErr != nil || len(second) != 0 {
		t.Fatalf("a live PostgreSQL outbox lease must exclude the second consumer: %#v, err=%v", second, secondErr)
	}
	if err := commands.AckRuntimeOutbox(ctx, tenantID, claimed[0].ID, "projector-a", now.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	permissionTx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer permissionTx.Rollback(ctx)
	if _, err := permissionTx.Exec(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := permissionTx.Exec(ctx, `SET LOCAL ROLE contentcloud_runtime`); err != nil {
		t.Fatal(err)
	}
	if _, err := permissionTx.Exec(ctx, `DELETE FROM runtime_job_events WHERE tenant_id=$1 AND id=$2`, tenantID, initialEvent.ID); err == nil {
		t.Fatal("runtime role must not delete append-only JobEvent")
	}

	beforeNode, err := store.NodeRun(ctx, tenantID, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := store.JobEvents(ctx, tenantID, jobID, 0)
	if err != nil {
		t.Fatal(err)
	}
	next := beforeNode
	if err := next.Transition(domain.NodeReady); err != nil {
		t.Fatal(err)
	}
	next.State = domain.NodeReady
	next.Version++
	_, err = commands.ApplyNodeTransition(ctx, next, beforeNode.Version, domain.JobEvent{ID: beforeEvents[0].ID, TenantID: tenantID, JobRunID: jobID, Type: "node.ready", ActorType: "test", Payload: map[string]any{}, OccurredAt: now.Add(time.Minute)})
	if err == nil {
		t.Fatal("duplicate event id must fail the command")
	}
	afterNode, err := store.NodeRun(ctx, tenantID, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	afterEvents, err := store.JobEvents(ctx, tenantID, jobID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(afterNode, beforeNode) || len(afterEvents) != len(beforeEvents) {
		t.Fatalf("event insertion failure must roll back the snapshot: before=%#v/%d after=%#v/%d", beforeNode, len(beforeEvents), afterNode, len(afterEvents))
	}
}

func repeatHex(count int, value byte) string {
	return strings.Repeat(string(value), count)
}
