package postgres

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/migrations"
)

const v3BaselineMigration = "00001_v3_baseline.sql"
const v5SubmissionTypesMigration = "00002_v5_submission_types.sql"
const tenantContentCapabilitiesMigration = "00003_tenant_content_capabilities.sql"
const runProgressEventsMigration = "00004_run_progress_events.sql"
const knowledgeInfrastructureMigration = "00005_knowledge_infrastructure.sql"
const orchestrationInfrastructureMigration = "00006_orchestration_infrastructure.sql"
const taskGovernanceMigration = "00007_task_governance.sql"
const builtinSOPMetadataMigration = "00008_builtin_sop_metadata.sql"
const conversationImportsMigration = "00009_conversation_imports.sql"
const inputItemsMigration = "00010_input_items.sql"
const workTaskIdempotencyMigration = "00011_work_task_idempotency.sql"

func (s *Store) Migrate(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS contentcloud_schema_migrations(version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	appliedRows, err := conn.Query(ctx, `SELECT version FROM contentcloud_schema_migrations ORDER BY version`)
	if err != nil {
		return err
	}
	applied := []string{}
	for appliedRows.Next() {
		var version string
		if err := appliedRows.Scan(&version); err != nil {
			appliedRows.Close()
			return err
		}
		applied = append(applied, version)
	}
	if err := appliedRows.Err(); err != nil {
		appliedRows.Close()
		return err
	}
	appliedRows.Close()
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return err
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if err := validateV3MigrationSet(names, applied); err != nil {
		return err
	}
	for _, name := range names {
		var applied bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contentcloud_schema_migrations WHERE version=$1)`, name).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		up := strings.SplitN(string(body), "-- +goose Down", 2)[0]
		sql := "BEGIN;\n" + up + fmt.Sprintf("\nINSERT INTO contentcloud_schema_migrations(version) VALUES('%s');\nCOMMIT;", strings.ReplaceAll(name, "'", "''"))
		results, err := conn.Conn().PgConn().Exec(ctx, sql).ReadAll()
		if err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		for _, result := range results {
			if result.Err != nil {
				return fmt.Errorf("apply migration %s: %w", name, result.Err)
			}
		}
	}
	return nil
}

func validateV3MigrationSet(available, applied []string) error {
	expected := []string{v3BaselineMigration, v5SubmissionTypesMigration, tenantContentCapabilitiesMigration, runProgressEventsMigration, knowledgeInfrastructureMigration, orchestrationInfrastructureMigration}
	// Keep the pure validator compatible with callers that validate the
	// pre-governance six-file set; Migrate itself passes the current embedded
	// set and therefore requires every current infrastructure migration.
	suffix := []string{taskGovernanceMigration, builtinSOPMetadataMigration, conversationImportsMigration, inputItemsMigration, workTaskIdempotencyMigration}
	for length := len(suffix); length >= 1; length-- {
		if len(available) == len(expected)+length {
			candidate := append([]string{}, suffix[:length]...)
			matches := true
			for index := range candidate {
				if available[len(expected)+index] != candidate[index] {
					matches = false
					break
				}
			}
			if matches {
				expected = append(expected, candidate...)
			}
			break
		}
	}
	if len(available) != len(expected) {
		return fmt.Errorf("migration 集合必须为 %v，当前为 %v", expected, available)
	}
	for index := range expected {
		if available[index] != expected[index] {
			return fmt.Errorf("migration 集合必须为 %v，当前为 %v", expected, available)
		}
	}
	for index, version := range applied {
		if index < len(expected) && version == expected[index] {
			continue
		}
		known := false
		for _, candidate := range expected {
			if version == candidate {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("检测到旧数据库 migration %s；V3 不提供历史兼容升级，需重建开发数据库", version)
		}
		return fmt.Errorf("migration 历史必须是 %v 的连续前缀，当前为 %v；migration 历史无效", expected, applied)
	}
	return nil
}
