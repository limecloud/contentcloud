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
	return []string{v3BaselineMigration, v5SubmissionTypesMigration, tenantContentCapabilitiesMigration, runProgressEventsMigration, knowledgeInfrastructureMigration, orchestrationInfrastructureMigration, taskGovernanceMigration, builtinSOPMetadataMigration, conversationImportsMigration, inputItemsMigration, workTaskIdempotencyMigration, mediaPipelineMigration, projectContentTypeMigration, agenticJobRuntimeMigration, runtimeAgentInstancesMigration, runtimeAttemptsMigration, workspaceMaterialsMigration, runtimeCommandKernelMigration, runtimeOutboxDeliveryMigration, runtimeAppendOnlyPermissionsMigration}
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

func TestJSONArrayValueNormalizesNilSlice(t *testing.T) {
	if got := string(jsonArrayValue[string](nil)); got != "[]" {
		t.Fatalf("nil JSON array encoded as %s, want []", got)
	}
}
