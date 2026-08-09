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
	setupTx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	setupStatements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO tenants(id,slug,name,status,created_at) VALUES($1,$2,$3,'active',$4)`, []any{tenantID, "runtime-test-" + tenantID, "Runtime test", now}},
		{`INSERT INTO brand_projects(id,tenant_id,slug,brand_name,product_name,channel,status,owner_name,reviewer_name,client_approver,created_at,updated_at) VALUES($1,$2,$3,'Runtime test','Runtime test','douyin','active','','','',$4,$4)`, []any{projectID, tenantID, "runtime-test-" + projectID, now}},
		{`INSERT INTO environments(tenant_id,id,name,slug,status,default_sop_id,default_sop_version,created_at,updated_at) VALUES($1,$2,'Runtime test','runtime-test','active',$3,1,$4,$4)`, []any{tenantID, environmentID, sopID, now}},
		{`INSERT INTO sop_definitions(tenant_id,id,name,current_version,created_by,created_at,updated_at) VALUES($1,$2,'Runtime test',1,'test',$3,$3)`, []any{tenantID, sopID, now}},
		{`INSERT INTO sop_versions(tenant_id,id,sop_id,version,schema_version,name,content_types,stages,gates,default_execution_mode,digest,status,created_by,published_by,created_at,published_at) VALUES($1,$2,$3,1,$4,'Runtime test','["marketing_video"]','[{"id":"source","name":"Source","order":10,"output_schema":"contentcloud.source/1.0","execution_modes":["local"]}]','[]','local',$5,'published','test','test',$6,$6)`, []any{tenantID, sopVersionID, sopID, domain.SOPSchemaVersion, "sha256:" + repeatHex(64, 'a'), now}},
		{`INSERT INTO work_tasks(tenant_id,id,project_id,environment_id,sop_id,sop_version,sop_digest,title,content_type,priority,risk_profile,status,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,1,$6,'Runtime test','marketing_video','normal','low','ready','test',$7,$7)`, []any{tenantID, taskID, projectID, environmentID, sopID, "sha256:" + repeatHex(64, 'a'), now}},
	}
	for _, statement := range setupStatements {
		if _, err := setupTx.Exec(ctx, statement.query, statement.args...); err != nil {
			_ = setupTx.Rollback(ctx)
			t.Fatal(err)
		}
	}
	if err := setupTx.Commit(ctx); err != nil {
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
	job := domain.JobRun{
		ID: jobID, TenantID: tenantID, ProjectID: projectID, WorkTaskID: taskID,
		PlanRevisionID: plan.ID, PlanDigest: plan.Digest,
		BindingDigest: "sha256:" + repeatHex(64, 'b'), InputDigest: "sha256:" + repeatHex(64, 'c'),
		RuntimePolicyID: "runtime-policy/postgres-test-v1", ContractMajor: 1, ContractMinor: 0, RootJobRunID: jobID,
		State: domain.JobRunCreated, Version: 1, CreatedBy: "test", CreatedAt: now, UpdatedAt: now,
	}
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
	if _, err := permissionTx.Exec(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := permissionTx.Exec(ctx, `SET LOCAL ROLE contentcloud_runtime`); err != nil {
		t.Fatal(err)
	}
	if _, err := permissionTx.Exec(ctx, `DELETE FROM runtime_job_events WHERE tenant_id=$1 AND id=$2`, tenantID, initialEvent.ID); err == nil {
		t.Fatal("runtime role must not delete append-only JobEvent")
	}
	_ = permissionTx.Rollback(ctx)

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

	patched, err := contentruntime.ApplyGraphPatch(plan, plan.GraphVersion, contentruntime.GraphPatch{
		ExpectedGraphVersion: plan.GraphVersion,
		IdempotencyKey:       "fanout-postgres-1",
		Reason:               "验证 PostgreSQL Fanout 原子写入",
		AddNodes:             []domain.JobPlanNode{{Key: "fanout:item-a", Kind: "fanout_item", Name: "Fanout item A", DependsOn: []string{"stage:source"}, OutputSchema: "contentcloud.test/1.0", RetryMaxAttempts: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	closedAt := now.Add(2 * time.Minute)
	patched.Plan.CompiledAt = closedAt
	nextJob := job
	nextJob.PlanRevisionID, nextJob.PlanDigest = patched.Plan.ID, patched.Plan.Digest
	nextJob.Version, nextJob.UpdatedAt = 2, closedAt
	fanoutSetID, fanoutMemberID, fanoutNodeID := domain.NewID(), domain.NewID(), domain.NewID()
	fanoutSet := domain.FanoutSet{ID: fanoutSetID, TenantID: tenantID, JobRunID: jobID, MapNodeKey: "stage:source", JoinNodeKey: "stage:source", Generation: 1, IdempotencyKey: "fanout-postgres-1", MembershipDigest: "sha256:" + repeatHex(64, 'e'), RequestDigest: "sha256:" + repeatHex(64, 'f'), MemberCount: 1, JoinPolicy: domain.JoinPolicy{Strategy: domain.JoinAll, ZeroMemberPolicy: domain.ZeroMemberFail}, Status: domain.FanoutClosed, Version: 1, ClosedAt: &closedAt, CreatedAt: closedAt, UpdatedAt: closedAt}
	fanoutNode := domain.NodeRun{ID: fanoutNodeID, TenantID: tenantID, JobRunID: jobID, NodeKey: "fanout:item-a", State: domain.NodePending, OutputRefs: []string{}, Version: 1, CreatedAt: closedAt, UpdatedAt: closedAt}
	fanoutMember := domain.FanoutMember{ID: fanoutMemberID, TenantID: tenantID, FanoutSetID: fanoutSetID, MemberKey: "fanout:item-a", ItemKey: "item-a", ItemDigest: "sha256:item-a", Generation: 1, NodeRunID: fanoutNodeID, State: domain.FanoutMemberPending, OutputRefs: []string{}, Version: 1, CreatedAt: closedAt, UpdatedAt: closedAt}
	if visibleJob, err := store.JobRun(ctx, tenantID, jobID); err != nil || visibleJob.ID != jobID {
		t.Fatalf("job must be visible before Fanout command: %#v err=%v", visibleJob, err)
	}
	createdJob, err := commands.CreateFanoutSetCommand(ctx, nextJob, job.Version, patched.Plan, fanoutSet, []domain.FanoutMember{fanoutMember}, []domain.NodeRun{fanoutNode}, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: jobID, Type: "fanout.created", ActorType: "test", IdempotencyKey: "fanout:postgres-1", Payload: map[string]any{"fanout_set_id": fanoutSetID}, OccurredAt: closedAt})
	if err != nil {
		t.Fatal(err)
	}
	storedSet, err := store.FanoutSet(ctx, tenantID, fanoutSetID)
	if err != nil {
		t.Fatal(err)
	}
	storedMembers, err := store.FanoutMembers(ctx, tenantID, fanoutSetID)
	if err != nil {
		t.Fatal(err)
	}
	if createdJob.PlanRevisionID != patched.Plan.ID || storedSet.MembershipDigest != fanoutSet.MembershipDigest || len(storedMembers) != 1 || storedMembers[0].NodeRunID != fanoutNodeID {
		t.Fatalf("PostgreSQL Fanout command did not commit all facts: job=%#v set=%#v members=%#v", createdJob, storedSet, storedMembers)
	}

	viewID, agentID, attemptID := domain.NewID(), domain.NewID(), domain.NewID()
	reservationID, collectionID, recordID, toolCallID := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	fixtureStatements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO runtime_context_views(tenant_id,id,job_run_id,node_run_id,attempt_id,schema_version,input_refs,state_refs,event_refs,allowed_tools,max_tokens,budget_minor,digest,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,'[]','[]','[]','[]',1024,0,$7,$8,$9)`, []any{tenantID, viewID, jobID, nodeID, attemptID, domain.ContextViewSchema, "sha256:" + repeatHex(64, 'b'), now, now.Add(time.Hour)}},
		{`INSERT INTO runtime_agent_instances(tenant_id,id,job_run_id,node_run_id,role,harness_kind,execution_profile_id,context_view_id,state,depth,remaining_descendants,budget_minor,used_cost_minor,version,created_at,updated_at) VALUES($1,$2,$3,$4,'worker','fake','test',$5,'created',0,0,0,0,1,$6,$6)`, []any{tenantID, agentID, jobID, nodeID, viewID, now}},
		{`INSERT INTO runtime_attempts(tenant_id,id,job_run_id,node_run_id,agent_instance_id,context_view_id,attempt_no,harness_kind,capabilities,state,lease_owner,fence_token,lease_expires_at,output_refs,safe_summary,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,1,'fake','{}','prepared','worker-a','fence-a',$7,'[]','{}',1,$8,$8)`, []any{tenantID, attemptID, jobID, nodeID, agentID, viewID, now.Add(time.Hour), now}},
		{`INSERT INTO runtime_resource_quotas(tenant_id,resource_key,capacity,unit,version,updated_at) VALUES($1,'agent.concurrent',2,'slots',1,$2)`, []any{tenantID, now}},
		{`INSERT INTO runtime_resource_reservations(tenant_id,id,job_run_id,node_run_id,attempt_id,resource_key,quantity,unit,state,fence_token,idempotency_key,expires_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'agent.concurrent',1,'slots','held','fence-a',$6,$7,$8,$8)`, []any{tenantID, reservationID, jobID, nodeID, attemptID, "reservation-" + reservationID, now.Add(time.Hour), now}},
		{`INSERT INTO runtime_state_collections(tenant_id,id,job_run_id,collection_key,scope,schema_id,schema_revision,consistency,max_record_bytes,max_records,revision,watermark,created_at,updated_at) VALUES($1,$2,$3,'brief','job','contentcloud.test/1.0',1,'cas_map',4096,10,0,0,$4,$4)`, []any{tenantID, collectionID, jobID, now}},
		{`INSERT INTO runtime_state_records(tenant_id,id,collection_id,key,value,schema_revision,version,digest,created_by,updated_by,created_at,updated_at) VALUES($1,$2,$3,'topic','{}',1,1,$4,'test','test',$5,$5)`, []any{tenantID, recordID, collectionID, "sha256:" + repeatHex(64, 'c'), now}},
		{`INSERT INTO runtime_tool_calls(tenant_id,id,job_run_id,node_run_id,attempt_id,agent_instance_id,tool_name,schema_version,request_digest,safe_request,state,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,'runtime.test','1.0',$7,'{}','proposed',1,$8,$8)`, []any{tenantID, toolCallID, jobID, nodeID, attemptID, agentID, "sha256:" + repeatHex(64, 'd'), now}},
		{`INSERT INTO runtime_projection_snapshots(tenant_id,job_run_id,job,nodes,last_event_sequence,source_event_id,projected_at) VALUES($1,$2,'{}','[]',1,$3,$4)`, []any{tenantID, jobID, initialEvent.ID, now}},
	}
	fixtureTx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range fixtureStatements {
		if _, err := fixtureTx.Exec(ctx, statement.query, statement.args...); err != nil {
			_ = fixtureTx.Rollback(ctx)
			t.Fatal(err)
		}
	}
	if err := fixtureTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	otherTenantID := domain.NewID()
	rlsTx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rlsTx.Rollback(ctx)
	if _, err := rlsTx.Exec(ctx, `SELECT set_config('app.tenant_id',$1,true)`, otherTenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := rlsTx.Exec(ctx, `SET LOCAL ROLE contentcloud_runtime`); err != nil {
		t.Fatal(err)
	}
	isolatedRows := []struct {
		table string
		query string
		id    string
	}{
		{"runtime_resource_quotas", `SELECT count(*) FROM runtime_resource_quotas WHERE tenant_id=$1 AND resource_key='agent.concurrent'`, tenantID},
		{"runtime_resource_reservations", `SELECT count(*) FROM runtime_resource_reservations WHERE tenant_id=$1 AND id=$2`, reservationID},
		{"runtime_state_collections", `SELECT count(*) FROM runtime_state_collections WHERE tenant_id=$1 AND id=$2`, collectionID},
		{"runtime_state_records", `SELECT count(*) FROM runtime_state_records WHERE tenant_id=$1 AND id=$2`, recordID},
		{"runtime_tool_calls", `SELECT count(*) FROM runtime_tool_calls WHERE tenant_id=$1 AND id=$2`, toolCallID},
		{"runtime_projection_snapshots", `SELECT count(*) FROM runtime_projection_snapshots WHERE tenant_id=$1 AND job_run_id=$2`, jobID},
		{"runtime_fanout_sets", `SELECT count(*) FROM runtime_fanout_sets WHERE tenant_id=$1 AND id=$2`, fanoutSetID},
		{"runtime_fanout_members", `SELECT count(*) FROM runtime_fanout_members WHERE tenant_id=$1 AND id=$2`, fanoutMemberID},
	}
	for _, row := range isolatedRows {
		var count int
		args := []any{tenantID}
		if row.table != "runtime_resource_quotas" {
			args = append(args, row.id)
		}
		if err := rlsTx.QueryRow(ctx, row.query, args...).Scan(&count); err != nil {
			t.Fatalf("query %s through Runtime RLS: %v", row.table, err)
		}
		if count != 0 {
			t.Fatalf("other tenant saw %s row through Runtime RLS: count=%d", row.table, count)
		}
	}
}

func repeatHex(count int, value byte) string {
	return strings.Repeat(string(value), count)
}
