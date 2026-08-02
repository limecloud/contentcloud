package postgres

import (
	"strings"
	"testing"
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
	if err == nil || !strings.Contains(err.Error(), "需重建开发数据库") {
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
	if err == nil || !strings.Contains(err.Error(), "migration 历史无效") {
		t.Fatalf("V5 migration without baseline must fail: %v", err)
	}
}

func TestValidateV3MigrationSetRejectsTenantCapabilitiesWithoutV5(t *testing.T) {
	err := validateV3MigrationSet(currentMigrationSet(), []string{v3BaselineMigration, tenantContentCapabilitiesMigration})
	if err == nil || !strings.Contains(err.Error(), "migration 历史无效") {
		t.Fatalf("tenant capability migration without V5 must fail: %v", err)
	}
}

func currentMigrationSet() []string {
	return []string{v3BaselineMigration, v5SubmissionTypesMigration, tenantContentCapabilitiesMigration, runProgressEventsMigration, knowledgeInfrastructureMigration, orchestrationInfrastructureMigration, taskGovernanceMigration, builtinSOPMetadataMigration, conversationImportsMigration, inputItemsMigration, workTaskIdempotencyMigration}
}

func TestJSONArrayValueNormalizesNilSlice(t *testing.T) {
	if got := string(jsonArrayValue[string](nil)); got != "[]" {
		t.Fatalf("nil JSON array encoded as %s, want []", got)
	}
}
