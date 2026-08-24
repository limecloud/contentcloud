package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	workbenchdomain "github.com/limecloud/contentcloud/internal/experience/workbench"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

// withPlatform runs a control-plane command under the runtime database role.
// Workbench manifests are platform-scoped, so they deliberately do not carry a
// tenant RLS context. Application services still gate every call to platform
// administrators before reaching this repository.
func (s *Store) withPlatform(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE contentcloud_runtime`); err != nil {
		return fmt.Errorf("启用 RLS 运行角色失败：%w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanWorkbenchEntry(row interface{ Scan(...any) error }) (workbenchdomain.Entry, error) {
	var entry workbenchdomain.Entry
	var manifest []byte
	if err := row.Scan(&entry.Manifest.ID, &entry.Manifest.Version, &manifest, &entry.Digest, &entry.Status, &entry.LifecycleReason, &entry.TemplateAliases, &entry.TenantIDs); err != nil {
		return entry, dbError(err)
	}
	if err := json.Unmarshal(manifest, &entry.Manifest); err != nil {
		return entry, fault.Invalid("WORKBENCH_PLUGIN_MANIFEST_INVALID", "数据库中的业务工作台声明无效")
	}
	return entry, nil
}

func workbenchStringArray(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (s *Store) WorkbenchEntries(ctx context.Context) ([]workbenchdomain.Entry, error) {
	entries := []workbenchdomain.Entry{}
	err := s.withPlatform(ctx, func(tx pgx.Tx) error {
		var err error
		entries, err = workbenchEntriesTx(ctx, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].String() < entries[j].String() })
	return entries, nil
}

func workbenchEntriesTx(ctx context.Context, tx pgx.Tx) ([]workbenchdomain.Entry, error) {
	entries := []workbenchdomain.Entry{}
	rows, err := tx.Query(ctx, `SELECT id,version,manifest,digest,status,lifecycle_reason,template_aliases,tenant_ids FROM workbench_plugin_versions ORDER BY id,version`)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	for rows.Next() {
		entry, scanErr := scanWorkbenchEntry(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) WorkbenchEntry(ctx context.Context, id, version string) (workbenchdomain.Entry, error) {
	var entry workbenchdomain.Entry
	err := s.withPlatform(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT id,version,manifest,digest,status,lifecycle_reason,template_aliases,tenant_ids FROM workbench_plugin_versions WHERE id=$1 AND version=$2`, strings.TrimSpace(id), strings.TrimSpace(version))
		var err error
		entry, err = scanWorkbenchEntry(row)
		if errors.Is(err, pgx.ErrNoRows) || fault.IsNotFound(err) {
			return fault.NotFound("业务工作台插件版本")
		}
		return err
	})
	if err != nil {
		return workbenchdomain.Entry{}, err
	}
	return entry, nil
}

func (s *Store) CreateWorkbenchEntry(ctx context.Context, value workbenchdomain.Entry) error {
	registry, err := workbenchdomain.NewRegistry([]workbenchdomain.Entry{value})
	if err != nil {
		return err
	}
	value = registry.Entries()[0]
	if value.Status != "draft" {
		return fault.Invalid("WORKBENCH_REGISTRATION_MUST_BE_DRAFT", "业务工作台必须先登记为草稿，再通过独立发布门禁启用")
	}
	return s.withPlatform(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO workbench_plugin_versions(id,version,manifest,digest,status,lifecycle_reason,template_aliases,tenant_ids) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, value.Manifest.ID, value.Manifest.Version, jsonValue(value.Manifest), value.Digest, value.Status, value.LifecycleReason, workbenchStringArray(value.TemplateAliases), workbenchStringArray(value.TenantIDs))
		return dbError(err)
	})
}

func (s *Store) UpdateWorkbenchEntryState(ctx context.Context, id, version, expectedStatus, status string, tenantIDs []string, reason string) (workbenchdomain.Entry, error) {
	expectedStatus = strings.ToLower(strings.TrimSpace(expectedStatus))
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "draft" && status != "published" && status != "retired" && status != "revoked" {
		return workbenchdomain.Entry{}, fault.Invalid("WORKBENCH_PLUGIN_STATUS_INVALID", "业务工作台插件状态无效")
	}
	if status == "revoked" && strings.TrimSpace(reason) == "" {
		return workbenchdomain.Entry{}, fault.Invalid("WORKBENCH_PLUGIN_REVOCATION_REASON_REQUIRED", "安全撤销业务工作台版本时必须填写原因")
	}
	var entry workbenchdomain.Entry
	err := s.withPlatform(ctx, func(tx pgx.Tx) error {
		// All state transitions share one short transaction lock so concurrent
		// publications cannot both pass the same scope/content-type check.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('contentcloud:workbench-registry'))`); err != nil {
			return dbError(err)
		}
		current, err := scanWorkbenchEntry(tx.QueryRow(ctx, `SELECT id,version,manifest,digest,status,lifecycle_reason,template_aliases,tenant_ids FROM workbench_plugin_versions WHERE id=$1 AND version=$2 FOR UPDATE`, strings.TrimSpace(id), strings.TrimSpace(version)))
		if errors.Is(err, pgx.ErrNoRows) || fault.IsNotFound(err) {
			return fault.NotFound("业务工作台插件版本")
		}
		if err != nil {
			return err
		}
		if current.Status != expectedStatus {
			return fault.Conflict("WORKBENCH_PLUGIN_STATE_STALE", "业务工作台版本状态已变化，请刷新后重试")
		}
		if current.Status == "revoked" && (!workbenchdomain.SameTenantScope(current.TenantIDs, tenantIDs) || current.LifecycleReason != strings.TrimSpace(reason)) {
			return fault.Conflict("WORKBENCH_PLUGIN_REVOKED_IMMUTABLE", "已安全撤销的业务工作台版本不能再修改租户范围或撤销原因")
		}
		if status == "published" {
			entries, err := workbenchEntriesTx(ctx, tx)
			if err != nil {
				return err
			}
			for _, existing := range entries {
				if existing.String() == current.String() || existing.Status != "published" || !workbenchdomain.ScopesConflict(existing.TenantIDs, tenantIDs) {
					continue
				}
				if workbenchdomain.SharesContentType(existing.Manifest.ContentTypes, current.Manifest.ContentTypes) {
					return fault.Conflict("WORKBENCH_CONTENT_TYPE_DEFAULT_EXISTS", "同一发布范围内已经存在该内容类型的已发布工作台；请停用旧版本或改用独立租户范围")
				}
			}
		}
		lifecycleReason := ""
		if status == "revoked" {
			lifecycleReason = strings.TrimSpace(reason)
		}
		row := tx.QueryRow(ctx, `UPDATE workbench_plugin_versions SET status=$4,tenant_ids=$5,lifecycle_reason=$6,updated_at=now() WHERE id=$1 AND version=$2 AND status=$3 RETURNING id,version,manifest,digest,status,lifecycle_reason,template_aliases,tenant_ids`, strings.TrimSpace(id), strings.TrimSpace(version), expectedStatus, status, workbenchStringArray(tenantIDs), lifecycleReason)
		var scanErr error
		entry, scanErr = scanWorkbenchEntry(row)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return fault.Conflict("WORKBENCH_PLUGIN_STATE_STALE", "业务工作台版本状态已变化，请刷新后重试")
		}
		return scanErr
	})
	return entry, err
}
