package postgres

import (
	"strings"
	"testing"
)

func TestValidateV3MigrationSet(t *testing.T) {
	available := []string{v3BaselineMigration}
	for _, applied := range [][]string{nil, {v3BaselineMigration}} {
		if err := validateV3MigrationSet(available, applied); err != nil {
			t.Fatalf("current V3 migration set was rejected: %v", err)
		}
	}
}

func TestValidateV3MigrationSetRejectsLegacyHistory(t *testing.T) {
	err := validateV3MigrationSet([]string{v3BaselineMigration}, []string{"00001_core.sql"})
	if err == nil || !strings.Contains(err.Error(), "需重建开发数据库") {
		t.Fatalf("legacy migration history must require a development database rebuild: %v", err)
	}
}

func TestValidateV3MigrationSetRejectsMultipleAvailableMigrations(t *testing.T) {
	err := validateV3MigrationSet([]string{v3BaselineMigration, "00002_compat.sql"}, nil)
	if err == nil {
		t.Fatal("V3 migration set must remain a single baseline")
	}
}

func TestJSONArrayValueNormalizesNilSlice(t *testing.T) {
	if got := string(jsonArrayValue[string](nil)); got != "[]" {
		t.Fatalf("nil JSON array encoded as %s, want []", got)
	}
}
