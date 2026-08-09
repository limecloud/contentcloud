package postgres

import (
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/migrations"
)

func TestValidateV3MigrationSet(t *testing.T) {
	available := currentMigrationSet()
	for _, applied := range [][]string{nil, {v3BaselineMigration}, {v3BaselineMigration, v5SubmissionTypesMigration}, {v3BaselineMigration, v5SubmissionTypesMigration, tenantContentCapabilitiesMigration}, {v3BaselineMigration, v5SubmissionTypesMigration, tenantContentCapabilitiesMigration, runProgressEventsMigration}, {v3BaselineMigration, v5SubmissionTypesMigration, tenantContentCapabilitiesMigration, runProgressEventsMigration, knowledgeInfrastructureMigration}, available} {
		if err := validateV3MigrationSet(available, applied); err != nil {
			t.Fatalf("current V3 migration set was rejected: %v", err)
		}
	}
}

func TestValidateV3MigrationSetRejectsLegacyHistory(t *testing.T) {
	err := validateV3MigrationSet(currentMigrationSet(), []string{"00001_core.sql"})
	if err == nil || !strings.Contains(err.Error(), "需要重建开发数据库") {
		t.Fatalf("legacy migration history must require a development database rebuild: %v", err)
	}
}

func TestValidateV3MigrationSetRejectsUnexpectedAvailableMigrations(t *testing.T) {
	err := validateV3MigrationSet([]string{v3BaselineMigration, "00002_compat.sql"}, nil)
	if err == nil {
		t.Fatal("unexpected migration set was accepted")
	}
}

func TestValidateV3MigrationSetRejectsV5WithoutBaseline(t *testing.T) {
	err := validateV3MigrationSet(currentMigrationSet(), []string{v5SubmissionTypesMigration})
	if err == nil || !strings.Contains(err.Error(), "迁移历史无效") {
		t.Fatalf("V5 migration without baseline must fail: %v", err)
	}
}

func TestValidateV3MigrationSetRejectsTenantCapabilitiesWithoutV5(t *testing.T) {
	err := validateV3MigrationSet(currentMigrationSet(), []string{v3BaselineMigration, tenantContentCapabilitiesMigration})
	if err == nil || !strings.Contains(err.Error(), "迁移历史无效") {
		t.Fatalf("tenant capability migration without V5 must fail: %v", err)
	}
}

func currentMigrationSet() []string {
	return []string{v3BaselineMigration, v5SubmissionTypesMigration, tenantContentCapabilitiesMigration, runProgressEventsMigration, knowledgeInfrastructureMigration, orchestrationInfrastructureMigration, taskGovernanceMigration, builtinSOPMetadataMigration, conversationImportsMigration, inputItemsMigration, workTaskIdempotencyMigration, mediaPipelineMigration, projectContentTypeMigration, agenticJobRuntimeMigration, runtimeAgentInstancesMigration, runtimeAttemptsMigration, workspaceMaterialsMigration, runtimeCommandKernelMigration, runtimeOutboxDeliveryMigration, runtimeAppendOnlyPermissionsMigration, runtimeFencingAndResourcesMigration, runtimeStateToolCallsMigration, runtimeProjectionMigration, runtimeJobContractMigration, runtimePlanRelationalMigration, runtimeFanoutJoinMigration, runtimeProviderInboxMigration, runtimeYieldResumeMigration, runtimeProjectionRebuildMigration, runtimeSessionStoreMigration, runtimeBusinessBindingMigration, runtimeInputSnapshotMigration, runtimeBusinessOutputMigration, removeV7ExecutionMigration}
}

func TestRuntimePlanRelationalMigrationReplacesJSONControlPlane(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimePlanRelationalMigration)
	if err != nil {
		t.Fatalf("read runtime plan migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"CREATE TABLE runtime_plan_revisions",
		"CREATE TABLE runtime_plan_nodes",
		"CREATE TABLE runtime_plan_edges",
		"jsonb_to_recordset(p.nodes)",
		"DROP TABLE runtime_job_plans",
		"runtime_job_runs_plan_revision_id_fkey",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("runtime plan migration must contain %q", required)
		}
	}
}

func TestRuntimeJobContractMigrationFreezesAdmissionIdentity(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeJobContractMigration)
	if err != nil {
		t.Fatalf("read runtime job contract migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"ALTER TABLE runtime_job_runs ADD COLUMN binding_digest",
		"ALTER TABLE runtime_job_runs ADD COLUMN input_digest",
		"ALTER TABLE runtime_job_runs ADD COLUMN runtime_policy_id",
		"ALTER TABLE runtime_job_runs ADD COLUMN contract_major",
		"ALTER TABLE runtime_job_runs ADD COLUMN contract_minor",
		"ALTER TABLE runtime_job_runs ADD COLUMN root_job_run_id",
		"runtime_job_runs_root_idx",
		"runtime_job_runs_root_job_fk",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("runtime job contract migration must contain %q", required)
		}
	}
}

func TestRuntimeFencingAndResourcesMigrationAddsFencingAndReservationLedger(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeFencingAndResourcesMigration)
	if err != nil {
		t.Fatalf("read runtime fencing migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"ALTER TABLE runtime_node_runs ADD COLUMN fence_token",
		"ALTER TABLE runtime_attempts ADD COLUMN fence_token",
		"CREATE TABLE runtime_resource_quotas",
		"CREATE TABLE runtime_resource_reservations",
		"runtime_resource_reservations_active_idx",
		"ALTER TABLE %I FORCE ROW LEVEL SECURITY",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("runtime fencing migration must contain %q", required)
		}
	}
}

func TestRuntimeStateToolCallsMigrationKeepsHistoricalEffectsNullable(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeStateToolCallsMigration)
	if err != nil {
		t.Fatalf("read runtime state/tool migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"ALTER TABLE runtime_checkpoints ADD COLUMN event_cursor",
		"CREATE TABLE runtime_state_collections",
		"CREATE TABLE runtime_state_records",
		"CREATE TABLE runtime_tool_calls",
		"ALTER TABLE runtime_effects ADD COLUMN attempt_id text;",
		"ALTER TABLE runtime_effects ADD COLUMN resource_reservation_id text;",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("runtime state/tool migration must contain %q", required)
		}
	}
	if strings.Contains(up, "ALTER TABLE runtime_effects ADD COLUMN attempt_id text NOT NULL") || strings.Contains(up, "ALTER TABLE runtime_effects ADD COLUMN resource_reservation_id text NOT NULL") {
		t.Fatal("historical effects must be allowed to keep empty attempt/reservation bindings")
	}
}

func TestRuntimeProjectionMigrationAddsRLSReadModel(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeProjectionMigration)
	if err != nil {
		t.Fatalf("read runtime projection migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"CREATE TABLE runtime_projection_snapshots",
		"last_event_sequence",
		"ALTER TABLE runtime_projection_snapshots FORCE ROW LEVEL SECURITY",
		"CREATE POLICY tenant_isolation ON runtime_projection_snapshots",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("runtime projection migration must contain %q", required)
		}
	}
}

func TestRuntimeOutboxDeliveryMigrationAddsConsumerLease(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeOutboxDeliveryMigration)
	if err != nil {
		t.Fatalf("read runtime outbox delivery migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{"ALTER TABLE runtime_outbox ADD COLUMN locked_by", "runtime_outbox_lock_idx"} {
		if !strings.Contains(up, required) {
			t.Fatalf("runtime outbox delivery migration must contain %q", required)
		}
	}
}

func TestRuntimeAppendOnlyPermissionsMigrationRevokesDirectMutation(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeAppendOnlyPermissionsMigration)
	if err != nil {
		t.Fatalf("read runtime append-only permissions migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{"REVOKE UPDATE,DELETE ON runtime_job_plans", "REVOKE DELETE ON runtime_job_runs"} {
		if !strings.Contains(up, required) {
			t.Fatalf("runtime append-only migration must contain %q", required)
		}
	}
}

func TestRuntimeCommandKernelMigrationAddsOutboxWithRLS(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeCommandKernelMigration)
	if err != nil {
		t.Fatalf("read runtime command kernel migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"CREATE TABLE runtime_outbox",
		"UNIQUE (tenant_id,event_id)",
		"FOREIGN KEY (tenant_id,event_id) REFERENCES runtime_job_events(tenant_id,id)",
		"ALTER TABLE runtime_outbox FORCE ROW LEVEL SECURITY",
		"CREATE POLICY tenant_isolation ON runtime_outbox",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("runtime command kernel migration must contain %q", required)
		}
	}
}

func TestProjectContentTypeMigrationExpandsTenantCapabilityConstraint(t *testing.T) {
	body, err := migrations.Files.ReadFile(projectContentTypeMigration)
	if err != nil {
		t.Fatalf("read project content type migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"DROP CONSTRAINT tenant_content_capabilities_content_type_check",
		"CHECK (content_type IN ('marketing_video','wechat_article'))",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("project content type migration must contain %q", required)
		}
	}
}

func TestRuntimeAgentInstancesMigrationEnforcesScopeAndRLS(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeAgentInstancesMigration)
	if err != nil {
		t.Fatalf("read runtime AgentInstance migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"CREATE TABLE runtime_context_views",
		"CREATE TABLE runtime_agent_instances",
		"FOREIGN KEY (tenant_id,job_run_id,node_run_id) REFERENCES runtime_node_runs(tenant_id,job_run_id,id)",
		"FOREIGN KEY (tenant_id,job_run_id,node_run_id,context_view_id) REFERENCES runtime_context_views(tenant_id,job_run_id,node_run_id,id)",
		"FOREIGN KEY (tenant_id,job_run_id,parent_agent_instance_id) REFERENCES runtime_agent_instances(tenant_id,job_run_id,id)",
		"ALTER TABLE %I FORCE ROW LEVEL SECURITY",
		"CREATE POLICY tenant_isolation",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("runtime AgentInstance migration must contain %q", required)
		}
	}
}

func TestRuntimeAttemptsMigrationEnforcesScopeStateAndRLS(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeAttemptsMigration)
	if err != nil {
		t.Fatalf("read RuntimeAttempt migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"CREATE TABLE runtime_attempts",
		"UNIQUE (tenant_id,node_run_id,attempt_no)",
		"FOREIGN KEY (tenant_id,job_run_id,node_run_id,context_view_id) REFERENCES runtime_context_views(tenant_id,job_run_id,node_run_id,id)",
		"FOREIGN KEY (tenant_id,job_run_id,node_run_id,agent_instance_id) REFERENCES runtime_agent_instances(tenant_id,job_run_id,node_run_id,id)",
		"ALTER TABLE runtime_attempts FORCE ROW LEVEL SECURITY",
		"CREATE POLICY tenant_isolation ON runtime_attempts",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("RuntimeAttempt migration must contain %q", required)
		}
	}
}

func TestRuntimeProviderInboxMigrationAddsReconciliationFacts(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeProviderInboxMigration)
	if err != nil {
		t.Fatalf("read Provider inbox migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"CREATE TABLE runtime_provider_inbox",
		"CREATE TABLE runtime_provider_reconciliations",
		"CREATE TABLE runtime_provider_bills",
		"UNIQUE (tenant_id,provider_id,message_id)",
		"UNIQUE (tenant_id,request_key)",
		"ALTER TABLE %I FORCE ROW LEVEL SECURITY",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("Provider inbox migration must contain %q", required)
		}
	}
}

func TestRuntimeYieldResumeMigrationAddsWaitingAndLeaseReleaseState(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeYieldResumeMigration)
	if err != nil {
		t.Fatalf("read Yield/Resume migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"'waiting_children'",
		"'yielded'",
		"CREATE TABLE runtime_yields",
		"UNIQUE (tenant_id,attempt_id)",
		"runtime_yields_resume_key_idx",
		"ALTER TABLE runtime_yields FORCE ROW LEVEL SECURITY",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("Yield/Resume migration must contain %q", required)
		}
	}
}

func TestRuntimeProjectionRebuildMigrationRecordsDryRunFacts(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeProjectionRebuildMigration)
	if err != nil {
		t.Fatalf("read projection rebuild migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"CREATE TABLE runtime_projection_rebuild_runs",
		"'dry_run'",
		"external_calls integer NOT NULL DEFAULT 0",
		"CREATE INDEX runtime_projection_rebuild_runs_job_idx",
		"ALTER TABLE runtime_projection_rebuild_runs FORCE ROW LEVEL SECURITY",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("projection rebuild migration must contain %q", required)
		}
	}
}

func TestRuntimeSessionStoreMigrationIsAnIsolatedTenantScopedMirror(t *testing.T) {
	body, err := migrations.Files.ReadFile(runtimeSessionStoreMigration)
	if err != nil {
		t.Fatalf("read SessionStore migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"CREATE TABLE runtime_agent_sessions",
		"CREATE TABLE runtime_agent_session_events",
		"UNIQUE (tenant_id,harness_kind,session_id,digest)",
		"FOREIGN KEY (tenant_id,harness_kind,session_id) REFERENCES runtime_agent_sessions",
		"ALTER TABLE %I FORCE ROW LEVEL SECURITY",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("SessionStore migration must contain %q", required)
		}
	}
}

func TestRemoveV7ExecutionMigrationDropsOnlyRetiredExecutionTables(t *testing.T) {
	body, err := migrations.Files.ReadFile(removeV7ExecutionMigration)
	if err != nil {
		t.Fatalf("read V7 execution removal migration: %v", err)
	}
	up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"DROP CONSTRAINT IF EXISTS runtime_job_runs_tenant_id_work_task_id_fkey",
		"ALTER TABLE IF EXISTS knowledge_items",
		"DROP CONSTRAINT IF EXISTS knowledge_items_origin_run_id_fkey",
		"DROP TABLE IF EXISTS run_progress_events",
		"DROP TABLE IF EXISTS run_attempts",
		"DROP TABLE IF EXISTS creative_execution_bundles",
		"DROP TABLE IF EXISTS task_runs",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("V7 execution removal migration must contain %q", required)
		}
	}
	if strings.Contains(up, "CASCADE") {
		t.Fatal("V7 execution removal migration must not cascade into unrelated tables")
	}
}

func TestJSONArrayValueNormalizesNilSlice(t *testing.T) {
	if got := string(jsonArrayValue[string](nil)); got != "[]" {
		t.Fatalf("nil JSON array encoded as %s, want []", got)
	}
}
