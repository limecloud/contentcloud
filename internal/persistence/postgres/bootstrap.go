package postgres

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/jackc/pgx/v5"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

const bootstrapAttemptSelect = `SELECT id,tenant_id,project_id,connect_session_id,attempt_token_hash,code_challenge,user_code,state,support_code,last_sequence,COALESCE(decided_by::text,''),created_at,updated_at,expires_at,decided_at,consumed_at,completed_at FROM bootstrap_attempts`

func scanBootstrapAttempt(row pgx.Row) (workspacedomain.BootstrapAttempt, error) {
	var value workspacedomain.BootstrapAttempt
	err := row.Scan(&value.ID, &value.TenantID, &value.ProjectID, &value.ConnectSessionID, &value.AttemptTokenHash, &value.CodeChallenge, &value.UserCode, &value.State, &value.SupportCode, &value.LastSequence, &value.DecidedBy, &value.CreatedAt, &value.UpdatedAt, &value.ExpiresAt, &value.DecidedAt, &value.ConsumedAt, &value.CompletedAt)
	return value, err
}

func (s *Store) CreateBootstrapAttemptForSession(ctx context.Context, sessionID string, attempt workspacedomain.BootstrapAttempt, now time.Time) (workspacedomain.BootstrapAttempt, error) {
	var tenantID, resolvedSessionID string
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id,session_id FROM contentcloud_lookup_connect_session($1)`, sessionID).Scan(&tenantID, &resolvedSessionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return attempt, fault.NotFound("连接会话")
		}
		return attempt, err
	}
	sessionID = resolvedSessionID
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var projectID, state string
		var expiresAt time.Time
		if err := tx.QueryRow(ctx, `SELECT project_id,state,expires_at FROM connect_sessions WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, sessionID).Scan(&projectID, &state, &expiresAt); err != nil {
			return dbError(err)
		}
		if state != "waiting_for_computer" || now.After(expiresAt) {
			return fault.Conflict("CONNECT_SESSION_UNAVAILABLE", "连接会话已过期、取消或被使用")
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bootstrap_attempts WHERE tenant_id=$1 AND connect_session_id=$2 AND state IN ('pending','approved') AND expires_at>$3)`, tenantID, sessionID, now).Scan(&active); err != nil {
			return err
		}
		if active {
			return fault.Conflict("BOOTSTRAP_AUTHORIZATION_ALREADY_STARTED", "这个初始化会话已有一台电脑等待确认")
		}
		attempt.TenantID = tenantID
		attempt.ProjectID = projectID
		attempt.ConnectSessionID = sessionID
		_, err := tx.Exec(ctx, `INSERT INTO bootstrap_attempts(id,tenant_id,project_id,connect_session_id,attempt_token_hash,code_challenge,user_code,state,support_code,last_sequence,decided_by,created_at,updated_at,expires_at,decided_at,consumed_at,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, attempt.ID, attempt.TenantID, attempt.ProjectID, attempt.ConnectSessionID, attempt.AttemptTokenHash, attempt.CodeChallenge, attempt.UserCode, attempt.State, attempt.SupportCode, attempt.LastSequence, nullable(attempt.DecidedBy), attempt.CreatedAt, attempt.UpdatedAt, attempt.ExpiresAt, attempt.DecidedAt, attempt.ConsumedAt, attempt.CompletedAt)
		return dbError(err)
	})
	return attempt, err
}

func (s *Store) BootstrapAttempt(ctx context.Context, tenantID, id string) (workspacedomain.BootstrapAttempt, error) {
	var result workspacedomain.BootstrapAttempt
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanBootstrapAttempt(tx.QueryRow(ctx, bootstrapAttemptSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		result = value
		if errors.Is(err, pgx.ErrNoRows) {
			return fault.NotFound("初始化尝试")
		}
		return err
	})
	result.AttemptTokenHash, result.CodeChallenge = "", ""
	return result, err
}

func (s *Store) BootstrapAttemptByTokenHash(ctx context.Context, tokenHash string) (workspacedomain.BootstrapAttempt, error) {
	var tenantID, attemptID string
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id,attempt_id FROM contentcloud_lookup_bootstrap_attempt($1)`, tokenHash).Scan(&tenantID, &attemptID); err != nil {
		return workspacedomain.BootstrapAttempt{}, fault.NotFound("初始化授权")
	}
	var result workspacedomain.BootstrapAttempt
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanBootstrapAttempt(tx.QueryRow(ctx, bootstrapAttemptSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, attemptID))
		result = value
		return dbError(err)
	})
	return result, err
}

func (s *Store) ApproveBootstrapAttempt(ctx context.Context, tenantID, sessionID, attemptID, userID string, now time.Time) (workspacedomain.BootstrapAttempt, error) {
	return s.decideBootstrapAttempt(ctx, tenantID, sessionID, attemptID, userID, "approved", now)
}

func (s *Store) DenyBootstrapAttempt(ctx context.Context, tenantID, sessionID, attemptID, userID string, now time.Time) (workspacedomain.BootstrapAttempt, error) {
	return s.decideBootstrapAttempt(ctx, tenantID, sessionID, attemptID, userID, "denied", now)
}

func (s *Store) decideBootstrapAttempt(ctx context.Context, tenantID, sessionID, attemptID, userID, state string, now time.Time) (workspacedomain.BootstrapAttempt, error) {
	var result workspacedomain.BootstrapAttempt
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		attempt, err := scanBootstrapAttempt(tx.QueryRow(ctx, bootstrapAttemptSelect+` WHERE tenant_id=$1 AND connect_session_id=$2 AND id=$3 FOR UPDATE`, tenantID, sessionID, attemptID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fault.NotFound("初始化授权")
			}
			return err
		}
		if attempt.State != "pending" || now.After(attempt.ExpiresAt) {
			return fault.Conflict("BOOTSTRAP_AUTHORIZATION_STATE_INVALID", "初始化授权已过期或已经处理")
		}
		var sessionState string
		var sessionExpires time.Time
		if err := tx.QueryRow(ctx, `SELECT state,expires_at FROM connect_sessions WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, sessionID).Scan(&sessionState, &sessionExpires); err != nil {
			return dbError(err)
		}
		if sessionState != "waiting_for_computer" || now.After(sessionExpires) {
			return fault.Conflict("CONNECT_SESSION_UNAVAILABLE", "连接会话已过期、取消或被使用")
		}
		if _, err := tx.Exec(ctx, `UPDATE bootstrap_attempts SET state=$4,decided_by=$5,decided_at=$6,updated_at=$6 WHERE tenant_id=$1 AND connect_session_id=$2 AND id=$3`, tenantID, sessionID, attemptID, state, userID, now); err != nil {
			return dbError(err)
		}
		if state == "denied" {
			if _, err := tx.Exec(ctx, `UPDATE connect_sessions SET state='canceled' WHERE tenant_id=$1 AND id=$2`, tenantID, sessionID); err != nil {
				return dbError(err)
			}
		}
		attempt.State, attempt.DecidedBy, attempt.UpdatedAt, attempt.DecidedAt = state, userID, now, &now
		result = attempt
		return nil
	})
	result.AttemptTokenHash, result.CodeChallenge = "", ""
	return result, err
}

func (s *Store) AppendBootstrapProgress(ctx context.Context, tokenHash string, event workspacedomain.BootstrapProgressEvent, now time.Time) (workspacedomain.BootstrapProgressEvent, error) {
	attempt, err := s.BootstrapAttemptByTokenHash(ctx, tokenHash)
	if err != nil {
		return event, fault.Conflict("BOOTSTRAP_ATTEMPT_TOKEN_INVALID", "初始化尝试凭据无效或已过期")
	}
	err = s.withTenant(ctx, attempt.TenantID, func(tx pgx.Tx) error {
		locked, err := scanBootstrapAttempt(tx.QueryRow(ctx, bootstrapAttemptSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, attempt.TenantID, attempt.ID))
		if err != nil {
			return dbError(err)
		}
		if now.After(locked.ExpiresAt.Add(30 * time.Minute)) {
			return fault.Conflict("BOOTSTRAP_ATTEMPT_TOKEN_INVALID", "初始化尝试凭据无效或已过期")
		}
		event.AttemptID = locked.ID
		var existing workspacedomain.BootstrapProgressEvent
		var facts []byte
		scanErr := tx.QueryRow(ctx, `SELECT schema_version,attempt_id,sequence,occurred_at,stage,status,check_id,error_code,action_id,facts FROM bootstrap_progress_events WHERE attempt_id=$1 AND sequence=$2`, locked.ID, event.Sequence).Scan(&existing.SchemaVersion, &existing.AttemptID, &existing.Sequence, &existing.OccurredAt, &existing.Stage, &existing.Status, &existing.CheckID, &existing.ErrorCode, &existing.ActionID, &facts)
		if scanErr == nil {
			existing.Facts, _ = decodeJSON[map[string]any](facts)
			if reflect.DeepEqual(existing, event) {
				return nil
			}
			return fault.Conflict("BOOTSTRAP_PROGRESS_SEQUENCE_CONFLICT", "同一进度序号（sequence）已存在不同事件")
		}
		if !errors.Is(scanErr, pgx.ErrNoRows) {
			return scanErr
		}
		if locked.State == "completed" || locked.State == "failed" || locked.State == "denied" {
			return fault.Conflict("BOOTSTRAP_PROGRESS_TERMINAL", "初始化尝试进入终态后不能追加新进度")
		}
		if event.Sequence != locked.LastSequence+1 {
			return fault.Conflict("BOOTSTRAP_PROGRESS_SEQUENCE_GAP", "初始化进度序号（sequence）必须连续递增")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO bootstrap_progress_events(attempt_id,sequence,tenant_id,project_id,schema_version,occurred_at,stage,status,check_id,error_code,action_id,facts) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, locked.ID, event.Sequence, locked.TenantID, locked.ProjectID, event.SchemaVersion, event.OccurredAt, event.Stage, event.Status, event.CheckID, event.ErrorCode, event.ActionID, jsonValue(event.Facts)); err != nil {
			return dbError(err)
		}
		_, err = tx.Exec(ctx, `UPDATE bootstrap_attempts SET last_sequence=$3,updated_at=$4 WHERE tenant_id=$1 AND id=$2`, locked.TenantID, locked.ID, event.Sequence, now)
		return err
	})
	return event, err
}

func (s *Store) BootstrapProgressForSession(ctx context.Context, tenantID, sessionID string) (*workspacedomain.BootstrapProgress, error) {
	var attempt workspacedomain.BootstrapAttempt
	var latest workspacedomain.BootstrapProgressEvent
	var facts []byte
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, bootstrapAttemptSelect+` WHERE tenant_id=$1 AND connect_session_id=$2 ORDER BY created_at DESC LIMIT 1`, tenantID, sessionID)
		value, err := scanBootstrapAttempt(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		attempt = value
		err = tx.QueryRow(ctx, `SELECT schema_version,attempt_id,sequence,occurred_at,stage,status,check_id,error_code,action_id,facts FROM bootstrap_progress_events WHERE attempt_id=$1 ORDER BY sequence DESC LIMIT 1`, attempt.ID).Scan(&latest.SchemaVersion, &latest.AttemptID, &latest.Sequence, &latest.OccurredAt, &latest.Stage, &latest.Status, &latest.CheckID, &latest.ErrorCode, &latest.ActionID, &facts)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err == nil {
			latest.Facts, _ = decodeJSON[map[string]any](facts)
		}
		return err
	})
	if err != nil || attempt.ID == "" {
		return nil, err
	}
	return workspacedomain.BootstrapProgressFrom(attempt, latest), nil
}

func (s *Store) ConsumeBootstrapAttempt(ctx context.Context, tokenHash string, device workspacedomain.Device, workspace workspacedomain.WorkspaceBinding, now time.Time) (workspacedomain.ConnectSession, workspacedomain.BootstrapAttempt, workspacedomain.Device, workspacedomain.WorkspaceBinding, error) {
	var tenantID, attemptID string
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id,attempt_id FROM contentcloud_lookup_bootstrap_attempt($1)`, tokenHash).Scan(&tenantID, &attemptID); err != nil {
		return workspacedomain.ConnectSession{}, workspacedomain.BootstrapAttempt{}, workspacedomain.Device{}, workspacedomain.WorkspaceBinding{}, fault.Conflict("BOOTSTRAP_AUTHORIZATION_INVALID", "初始化授权无效、已使用或已过期")
	}
	var session workspacedomain.ConnectSession
	var attempt workspacedomain.BootstrapAttempt
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		value, err := scanBootstrapAttempt(tx.QueryRow(ctx, bootstrapAttemptSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, attemptID))
		if err != nil || value.State != "approved" || value.ConsumedAt != nil || now.After(value.ExpiresAt) {
			return fault.Conflict("BOOTSTRAP_AUTHORIZATION_INVALID", "初始化授权无效、已使用或已过期")
		}
		v, err := scanConnect(tx.QueryRow(ctx, `SELECT id,tenant_id,project_id,inviter_user_id,state,expires_at,consumed_at,COALESCE(consumed_device_id::text,'') FROM connect_sessions WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, value.ConnectSessionID))
		if err != nil || v.State != "waiting_for_computer" || now.After(v.ExpiresAt) {
			return fault.Conflict("CONNECT_SESSION_UNAVAILABLE", "连接会话已过期、取消或被使用")
		}
		device.TenantID, device.OwnerUserID, device.ProjectIDs = v.TenantID, v.InviterUserID, []string{v.ProjectID}
		var existingID string
		var existingProjects []byte
		err = tx.QueryRow(ctx, `SELECT d.id,COALESCE((SELECT jsonb_agg(g.project_id::text) FROM project_device_grants g WHERE g.device_id=d.id AND g.revoked_at IS NULL),'[]'::jsonb) FROM devices d WHERE d.tenant_id=$1 AND d.machine_id=$2 AND d.revoked_at IS NULL FOR UPDATE`, v.TenantID, device.MachineID).Scan(&existingID, &existingProjects)
		if err == nil {
			device.ID = existingID
			device.ProjectIDs, _ = decodeJSON[[]string](existingProjects)
			if _, err := tx.Exec(ctx, `UPDATE devices SET owner_user_id=$3,display_name=$4,hostname=$5,platform=$6,arch=$7,daemon_version=$8,token_hash=$9,credential_version=credential_version+1,credential_rotated_at=$10,capability_manifests=$11,last_seen_at=$10 WHERE tenant_id=$1 AND id=$2`, device.TenantID, device.ID, device.OwnerUserID, device.DisplayName, device.Hostname, device.Platform, device.Arch, device.Version, device.TokenHash, now, jsonArrayValue(device.Capabilities)); err != nil {
				return dbError(err)
			}
			if err := tx.QueryRow(ctx, `SELECT credential_version,credential_rotated_at FROM devices WHERE tenant_id=$1 AND id=$2`, device.TenantID, device.ID).Scan(&device.CredentialVersion, &device.CredentialRotatedAt); err != nil {
				return err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		} else if _, err := tx.Exec(ctx, `INSERT INTO devices(id,tenant_id,owner_user_id,machine_id,display_name,hostname,platform,arch,daemon_version,token_hash,credential_version,credential_rotated_at,capability_manifests,last_seen_at,revoked_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, device.ID, device.TenantID, device.OwnerUserID, device.MachineID, device.DisplayName, device.Hostname, device.Platform, device.Arch, device.Version, device.TokenHash, device.CredentialVersion, device.CredentialRotatedAt, jsonArrayValue(device.Capabilities), device.LastSeenAt, device.RevokedAt); err != nil {
			return dbError(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO project_device_grants(tenant_id,project_id,device_id,granted_by,granted_at,revoked_at) VALUES($1,$2,$3,$4,$5,NULL) ON CONFLICT (tenant_id,project_id,device_id) DO UPDATE SET granted_by=EXCLUDED.granted_by,granted_at=EXCLUDED.granted_at,revoked_at=NULL`, v.TenantID, v.ProjectID, device.ID, v.InviterUserID, now); err != nil {
			return dbError(err)
		}
		foundProject := false
		for _, projectID := range device.ProjectIDs {
			foundProject = foundProject || projectID == v.ProjectID
		}
		if !foundProject {
			device.ProjectIDs = append(device.ProjectIDs, v.ProjectID)
		}
		workspace.TenantID, workspace.ProjectID, workspace.DeviceID, workspace.OwnerUserID = v.TenantID, v.ProjectID, device.ID, v.InviterUserID
		var existingWorkspaceID string
		err = tx.QueryRow(ctx, `SELECT id FROM workspace_bindings WHERE tenant_id=$1 AND project_id=$2 AND device_id=$3 AND status='active' AND revoked_at IS NULL ORDER BY initialized_at LIMIT 1 FOR UPDATE`, workspace.TenantID, workspace.ProjectID, workspace.DeviceID).Scan(&existingWorkspaceID)
		if err == nil {
			workspace.ID = existingWorkspaceID
			if _, err := tx.Exec(ctx, `UPDATE workspace_bindings SET owner_user_id=$4,template_id=$5,template_version=$6,targets=$7,credential_hash=$8,last_seen_at=$9 WHERE tenant_id=$1 AND id=$2 AND device_id=$3`, workspace.TenantID, workspace.ID, workspace.DeviceID, workspace.OwnerUserID, workspace.TemplateID, workspace.TemplateVersion, jsonArrayValue(workspace.Targets), workspace.CredentialHash, now); err != nil {
				return dbError(err)
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		} else if _, err := tx.Exec(ctx, `INSERT INTO workspace_bindings(id,tenant_id,project_id,device_id,owner_user_id,template_id,template_version,targets,credential_hash,status,initialized_at,last_seen_at,revoked_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, workspace.ID, workspace.TenantID, workspace.ProjectID, workspace.DeviceID, workspace.OwnerUserID, workspace.TemplateID, workspace.TemplateVersion, jsonArrayValue(workspace.Targets), workspace.CredentialHash, workspace.Status, workspace.InitializedAt, workspace.LastSeenAt, workspace.RevokedAt); err != nil {
			return dbError(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE connect_sessions SET state='verifying',consumed_at=$3,consumed_device_id=$4 WHERE tenant_id=$1 AND id=$2`, v.TenantID, v.ID, now, device.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bootstrap_attempts SET state='consumed',consumed_at=$3,updated_at=$3 WHERE tenant_id=$1 AND id=$2`, tenantID, attemptID, now); err != nil {
			return err
		}
		v.State, v.ConsumedAt, v.ConsumedDeviceID = "verifying", &now, device.ID
		value.State, value.ConsumedAt, value.UpdatedAt = "consumed", &now, now
		session, attempt = v, value
		return nil
	})
	attempt.AttemptTokenHash, attempt.CodeChallenge = "", ""
	return session, attempt, device, workspace, err
}

func (s *Store) CompleteBootstrapAttempt(ctx context.Context, tokenHash, state string, now time.Time) (workspacedomain.BootstrapAttempt, error) {
	if state != "completed" && state != "failed" {
		return workspacedomain.BootstrapAttempt{}, fault.Invalid("BOOTSTRAP_ATTEMPT_STATE_INVALID", "初始化完成状态无效")
	}
	attempt, err := s.BootstrapAttemptByTokenHash(ctx, tokenHash)
	if err != nil {
		return attempt, err
	}
	err = s.withTenant(ctx, attempt.TenantID, func(tx pgx.Tx) error {
		locked, err := scanBootstrapAttempt(tx.QueryRow(ctx, bootstrapAttemptSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, attempt.TenantID, attempt.ID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fault.NotFound("初始化尝试")
			}
			return err
		}
		if locked.State == state {
			attempt = locked
			return nil
		}
		if locked.State != "consumed" || now.After(locked.ExpiresAt.Add(30*time.Minute)) {
			return fault.Conflict("BOOTSTRAP_ATTEMPT_STATE_INVALID", "只有已完成浏览器授权并创建设备的初始化尝试可以进入终态")
		}
		if _, err := tx.Exec(ctx, `UPDATE bootstrap_attempts SET state=$3,updated_at=$4,completed_at=$4 WHERE tenant_id=$1 AND id=$2`, locked.TenantID, locked.ID, state, now); err != nil {
			return dbError(err)
		}
		locked.State, locked.UpdatedAt, locked.CompletedAt = state, now, &now
		attempt = locked
		return nil
	})
	attempt.AttemptTokenHash, attempt.CodeChallenge = "", ""
	return attempt, err
}

func (s *Store) CreateBootstrapDiagnostic(ctx context.Context, diagnostic workspacedomain.BootstrapDiagnostic) (workspacedomain.BootstrapDiagnostic, error) {
	var stored workspacedomain.BootstrapDiagnostic
	var summary []byte
	err := s.withTenant(ctx, diagnostic.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO bootstrap_diagnostics(id,tenant_id,project_id,attempt_id,support_code,digest,byte_size,summary,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(attempt_id,digest) DO UPDATE SET digest=EXCLUDED.digest RETURNING id,tenant_id,project_id,attempt_id,support_code,digest,byte_size,summary,created_at`, diagnostic.ID, diagnostic.TenantID, diagnostic.ProjectID, diagnostic.AttemptID, diagnostic.SupportCode, diagnostic.Digest, diagnostic.ByteSize, jsonValue(diagnostic.Summary), diagnostic.CreatedAt).Scan(&stored.ID, &stored.TenantID, &stored.ProjectID, &stored.AttemptID, &stored.SupportCode, &stored.Digest, &stored.ByteSize, &summary, &stored.CreatedAt)
		return dbError(err)
	})
	if err != nil {
		return diagnostic, err
	}
	stored.Summary, err = decodeJSON[workspacedomain.BootstrapDiagnosticSummary](summary)
	return stored, err
}
