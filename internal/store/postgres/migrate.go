package postgres

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/migrations"
)

func (s *Store) Migrate(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS contentcloud_schema_migrations(version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
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
