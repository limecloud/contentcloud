package postgres

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/migrations"
)

const v3BaselineMigration = "00001_v3_baseline.sql"

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
	if len(available) != 1 || available[0] != v3BaselineMigration {
		return fmt.Errorf("V3 migration 集合必须且只能包含 %s，当前为 %v", v3BaselineMigration, available)
	}
	for _, version := range applied {
		if version != v3BaselineMigration {
			return fmt.Errorf("检测到旧数据库 migration %s；V3 不提供历史兼容升级，需重建开发数据库", version)
		}
	}
	return nil
}
