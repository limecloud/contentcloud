package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

const workspaceRevisionSelect = `SELECT id,tenant_id,project_id,workspace_id,device_id,schema_version,revision_no,COALESCE(base_revision_id::text,'0'),content_digest,files,client_mutation_id,idempotency_key,created_at FROM workspace_revisions`

func scanWorkspaceRevision(row pgx.Row) (workspacedomain.WorkspaceRevision, error) {
	var value workspacedomain.WorkspaceRevision
	var files []byte
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.WorkspaceID, &value.DeviceID, &value.SchemaVersion, &value.RevisionNo, &value.BaseRevisionID, &value.ContentDigest, &files, &value.ClientMutationID, &value.IdempotencyKey, &value.CreatedAt)
	if err == nil {
		err = json.Unmarshal(files, &value.Files)
	}
	return value, err
}

func (s *Store) PublishWorkspaceRevision(ctx context.Context, value workspacedomain.WorkspaceRevision) (workspacedomain.WorkspaceRevision, error) {
	var result workspacedomain.WorkspaceRevision
	err := s.withTenantCommand(ctx, value.TenantID, "workspace_revision.publish", func(tx pgx.Tx) error {
		var bindingProject, bindingDevice, bindingStatus string
		var revoked bool
		if err := tx.QueryRow(ctx, `SELECT project_id,COALESCE(device_id::text,''),status,revoked_at IS NOT NULL FROM workspace_bindings WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, value.TenantID, value.WorkspaceID).Scan(&bindingProject, &bindingDevice, &bindingStatus, &revoked); err != nil {
			return dbError(err)
		}
		if bindingProject != value.ProjectID || bindingDevice != value.DeviceID || bindingStatus != "active" || revoked {
			return fault.Conflict("WORKSPACE_BINDING_INVALID", "工作区绑定无效或与当前设备不匹配")
		}
		existing, err := scanWorkspaceRevision(tx.QueryRow(ctx, workspaceRevisionSelect+` WHERE tenant_id=$1 AND workspace_id=$2 AND idempotency_key=$3`, value.TenantID, value.WorkspaceID, value.IdempotencyKey))
		if err == nil {
			if samePostgresWorkspaceRevision(existing, value) {
				result = existing
				return nil
			}
			return fault.Conflict("WORKSPACE_REVISION_IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同的工作区 Revision")
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		latest, err := scanWorkspaceRevision(tx.QueryRow(ctx, workspaceRevisionSelect+` WHERE tenant_id=$1 AND workspace_id=$2 ORDER BY revision_no DESC LIMIT 1`, value.TenantID, value.WorkspaceID))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		currentID := "0"
		if latest.ID != "" {
			currentID = latest.ID
		}
		if value.BaseRevisionID != currentID {
			conflict := fault.Conflict("WORKSPACE_REVISION_STALE", "Cloud Revision 已变化，拒绝覆盖新版本")
			conflict.Details = map[string]any{"expected_base_revision": currentID, "provided_base_revision": value.BaseRevisionID}
			return conflict
		}
		if latest.ID != "" && latest.ContentDigest == value.ContentDigest {
			return fault.Conflict("WORKSPACE_REVISION_UNCHANGED", "工作区内容摘要未变化")
		}
		value.RevisionNo = latest.RevisionNo + 1
		_, err = tx.Exec(ctx, `INSERT INTO workspace_revisions(id,tenant_id,project_id,workspace_id,device_id,schema_version,revision_no,base_revision_id,content_digest,files,client_mutation_id,idempotency_key,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'0')::uuid,$9,$10,$11,$12,$13)`,
			value.ID, value.TenantID, value.ProjectID, value.WorkspaceID, value.DeviceID, value.SchemaVersion, value.RevisionNo, value.BaseRevisionID, value.ContentDigest, jsonArrayValue(value.Files), value.ClientMutationID, value.IdempotencyKey, value.CreatedAt)
		if err != nil {
			return dbError(err)
		}
		result = value
		return nil
	})
	return result, err
}

func (s *Store) LatestWorkspaceRevision(ctx context.Context, tenantID, workspaceID string) (workspacedomain.WorkspaceRevision, error) {
	var result workspacedomain.WorkspaceRevision
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanWorkspaceRevision(tx.QueryRow(ctx, workspaceRevisionSelect+` WHERE tenant_id=$1 AND workspace_id=$2 ORDER BY revision_no DESC LIMIT 1`, tenantID, workspaceID))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("工作区 Revision")
		}
		return err
	})
	return result, err
}

func (s *Store) WorkspaceRevisionsAfter(ctx context.Context, tenantID, workspaceID string, after int64, limit int) ([]workspacedomain.WorkspaceRevision, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	values := make([]workspacedomain.WorkspaceRevision, 0)
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, workspaceRevisionSelect+` WHERE tenant_id=$1 AND workspace_id=$2 AND revision_no>$3 ORDER BY revision_no LIMIT $4`, tenantID, workspaceID, after, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanWorkspaceRevision(rows)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}

func samePostgresWorkspaceRevision(left, right workspacedomain.WorkspaceRevision) bool {
	return left.ProjectID == right.ProjectID && left.WorkspaceID == right.WorkspaceID && left.DeviceID == right.DeviceID &&
		left.BaseRevisionID == right.BaseRevisionID && left.ContentDigest == right.ContentDigest && slices.Equal(left.Files, right.Files) && left.ClientMutationID == right.ClientMutationID
}

const workspaceUploadSessionSelect = `SELECT id,tenant_id,project_id,workspace_id,device_id,file_ref,content_digest,byte_size,chunk_size,part_count,state,object_key,idempotency_key,created_at,updated_at,expires_at FROM workspace_upload_sessions`

func scanWorkspaceUploadSession(row pgx.Row) (workspacedomain.WorkspaceUploadSession, error) {
	var value workspacedomain.WorkspaceUploadSession
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.WorkspaceID, &value.DeviceID, &value.Ref, &value.ContentDigest, &value.ByteSize, &value.ChunkSize, &value.PartCount, &value.State, &value.ObjectKey, &value.IdempotencyKey, &value.CreatedAt, &value.UpdatedAt, &value.ExpiresAt)
	return value, err
}

func (s *Store) CreateWorkspaceUploadSession(ctx context.Context, value workspacedomain.WorkspaceUploadSession) (workspacedomain.WorkspaceUploadSession, error) {
	var result workspacedomain.WorkspaceUploadSession
	err := s.withTenantCommand(ctx, value.TenantID, "workspace_upload.create", func(tx pgx.Tx) error {
		var bindingProject, bindingDevice, bindingStatus string
		var revoked bool
		if err := tx.QueryRow(ctx, `SELECT project_id,COALESCE(device_id::text,''),status,revoked_at IS NOT NULL FROM workspace_bindings WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, value.TenantID, value.WorkspaceID).Scan(&bindingProject, &bindingDevice, &bindingStatus, &revoked); err != nil {
			return dbError(err)
		}
		if bindingProject != value.ProjectID || bindingDevice != value.DeviceID || bindingStatus != "active" || revoked {
			return fault.Conflict("WORKSPACE_BINDING_INVALID", "工作区绑定无效或与当前设备不匹配")
		}
		existing, err := scanWorkspaceUploadSession(tx.QueryRow(ctx, workspaceUploadSessionSelect+` WHERE tenant_id=$1 AND workspace_id=$2 AND idempotency_key=$3`, value.TenantID, value.WorkspaceID, value.IdempotencyKey))
		if err == nil {
			if samePostgresWorkspaceUploadSession(existing, value) {
				result = existing
				return nil
			}
			return fault.Conflict("WORKSPACE_UPLOAD_IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同的上传文件")
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO workspace_upload_sessions(id,tenant_id,project_id,workspace_id,device_id,file_ref,content_digest,byte_size,chunk_size,part_count,state,object_key,idempotency_key,created_at,updated_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
			value.ID, value.TenantID, value.ProjectID, value.WorkspaceID, value.DeviceID, value.Ref, value.ContentDigest, value.ByteSize, value.ChunkSize, value.PartCount, value.State, value.ObjectKey, value.IdempotencyKey, value.CreatedAt, value.UpdatedAt, value.ExpiresAt)
		if err != nil {
			return dbError(err)
		}
		result = value
		return nil
	})
	return result, err
}

func (s *Store) WorkspaceUploadSession(ctx context.Context, tenantID, sessionID string) (workspacedomain.WorkspaceUploadSession, error) {
	var result workspacedomain.WorkspaceUploadSession
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanWorkspaceUploadSession(tx.QueryRow(ctx, workspaceUploadSessionSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, sessionID))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("工作区上传会话")
		}
		return err
	})
	return result, err
}

func (s *Store) SaveWorkspaceUploadPart(ctx context.Context, tenantID string, value workspacedomain.WorkspaceUploadPart) (workspacedomain.WorkspaceUploadPart, error) {
	var result workspacedomain.WorkspaceUploadPart
	err := s.withTenantCommand(ctx, tenantID, "workspace_upload.part", func(tx pgx.Tx) error {
		session, err := scanWorkspaceUploadSession(tx.QueryRow(ctx, workspaceUploadSessionSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, value.SessionID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("工作区上传会话")
		}
		if err != nil {
			return err
		}
		if session.State == "completed" {
			return fault.Conflict("WORKSPACE_UPLOAD_COMPLETED", "上传会话已经完成")
		}
		var existing workspacedomain.WorkspaceUploadPart
		err = tx.QueryRow(ctx, `SELECT session_id,part_no,content_digest,byte_size,object_key,created_at FROM workspace_upload_parts WHERE tenant_id=$1 AND session_id=$2 AND part_no=$3`, tenantID, value.SessionID, value.PartNo).
			Scan(&existing.SessionID, &existing.PartNo, &existing.Digest, &existing.ByteSize, &existing.ObjectKey, &existing.CreatedAt)
		if err == nil {
			if existing.Digest == value.Digest && existing.ByteSize == value.ByteSize && existing.ObjectKey == value.ObjectKey {
				result = existing
				return nil
			}
			return fault.Conflict("WORKSPACE_UPLOAD_PART_CONFLICT", "分片编号已对应其他内容")
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workspace_upload_parts(tenant_id,session_id,part_no,content_digest,byte_size,object_key,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, tenantID, value.SessionID, value.PartNo, value.Digest, value.ByteSize, value.ObjectKey, value.CreatedAt); err != nil {
			return dbError(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE workspace_upload_sessions SET state='uploading',updated_at=$3 WHERE tenant_id=$1 AND id=$2`, tenantID, value.SessionID, value.CreatedAt); err != nil {
			return err
		}
		result = value
		return nil
	})
	return result, err
}

func (s *Store) WorkspaceUploadParts(ctx context.Context, tenantID, sessionID string) ([]workspacedomain.WorkspaceUploadPart, error) {
	values := make([]workspacedomain.WorkspaceUploadPart, 0)
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT session_id,part_no,content_digest,byte_size,object_key,created_at FROM workspace_upload_parts WHERE tenant_id=$1 AND session_id=$2 ORDER BY part_no`, tenantID, sessionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value workspacedomain.WorkspaceUploadPart
			if err := rows.Scan(&value.SessionID, &value.PartNo, &value.Digest, &value.ByteSize, &value.ObjectKey, &value.CreatedAt); err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}

func (s *Store) CompleteWorkspaceUpload(ctx context.Context, session workspacedomain.WorkspaceUploadSession, object workspacedomain.WorkspaceObject) error {
	return s.withTenantCommand(ctx, session.TenantID, "workspace_upload.complete", func(tx pgx.Tx) error {
		current, err := scanWorkspaceUploadSession(tx.QueryRow(ctx, workspaceUploadSessionSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, session.TenantID, session.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("工作区上传会话")
		}
		if err != nil {
			return err
		}
		if current.TenantID != object.TenantID || current.ContentDigest != object.ContentDigest || current.ByteSize != object.ByteSize || current.ProjectID != object.ProjectID || current.ObjectKey != object.ObjectKey {
			return fault.Conflict("WORKSPACE_UPLOAD_OBJECT_MISMATCH", "上传对象与会话声明不一致")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workspace_objects(tenant_id,project_id,content_digest,byte_size,object_key,created_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (tenant_id,project_id,content_digest) DO NOTHING`, object.TenantID, object.ProjectID, object.ContentDigest, object.ByteSize, object.ObjectKey, object.CreatedAt); err != nil {
			return dbError(err)
		}
		var storedSize int64
		var storedKey string
		if err := tx.QueryRow(ctx, `SELECT byte_size,object_key FROM workspace_objects WHERE tenant_id=$1 AND project_id=$2 AND content_digest=$3`, object.TenantID, object.ProjectID, object.ContentDigest).Scan(&storedSize, &storedKey); err != nil {
			return err
		}
		if storedSize != object.ByteSize || storedKey != object.ObjectKey {
			return fault.Conflict("WORKSPACE_OBJECT_CONFLICT", "相同摘要对应了不同的工作区对象")
		}
		_, err = tx.Exec(ctx, `UPDATE workspace_upload_sessions SET state='completed',updated_at=$3 WHERE tenant_id=$1 AND id=$2`, session.TenantID, session.ID, session.UpdatedAt)
		return err
	})
}

func (s *Store) WorkspaceObject(ctx context.Context, tenantID, projectID, digest string) (workspacedomain.WorkspaceObject, error) {
	var result workspacedomain.WorkspaceObject
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT tenant_id,project_id,content_digest,byte_size,object_key,created_at FROM workspace_objects WHERE tenant_id=$1 AND project_id=$2 AND content_digest=$3`, tenantID, projectID, digest).
			Scan(&result.TenantID, &result.ProjectID, &result.ContentDigest, &result.ByteSize, &result.ObjectKey, &result.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("工作区对象")
		}
		return err
	})
	return result, err
}

func (s *Store) WorkspaceObjects(ctx context.Context, tenantID, projectID string, digests []string) ([]workspacedomain.WorkspaceObject, error) {
	values := make([]workspacedomain.WorkspaceObject, 0, len(digests))
	if len(digests) == 0 {
		return values, nil
	}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT tenant_id,project_id,content_digest,byte_size,object_key,created_at FROM workspace_objects WHERE tenant_id=$1 AND project_id=$2 AND content_digest=ANY($3)`, tenantID, projectID, digests)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value workspacedomain.WorkspaceObject
			if err := rows.Scan(&value.TenantID, &value.ProjectID, &value.ContentDigest, &value.ByteSize, &value.ObjectKey, &value.CreatedAt); err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	sort.Slice(values, func(i, j int) bool { return values[i].ContentDigest < values[j].ContentDigest })
	return values, err
}

func samePostgresWorkspaceUploadSession(left, right workspacedomain.WorkspaceUploadSession) bool {
	return left.ProjectID == right.ProjectID && left.WorkspaceID == right.WorkspaceID && left.DeviceID == right.DeviceID && left.Ref == right.Ref &&
		left.ContentDigest == right.ContentDigest && left.ByteSize == right.ByteSize && left.ChunkSize == right.ChunkSize && left.PartCount == right.PartCount
}
