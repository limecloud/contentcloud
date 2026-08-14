package memory

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateBootstrapAttemptForSession(_ context.Context, sessionID string, attempt domain.BootstrapAttempt, now time.Time) (domain.BootstrapAttempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.connects[sessionID]
	if !ok {
		return attempt, domain.NotFound("连接会话")
	}
	if session.State != "waiting_for_computer" || now.After(session.ExpiresAt) {
		return attempt, domain.Conflict("CONNECT_SESSION_UNAVAILABLE", "连接会话已过期、取消或被使用")
	}
	for _, existing := range s.bootstrapAttempts {
		if existing.ConnectSessionID == sessionID && (existing.State == "pending" || existing.State == "approved") && now.Before(existing.ExpiresAt) {
			return attempt, domain.Conflict("BOOTSTRAP_AUTHORIZATION_ALREADY_STARTED", "这个初始化会话已有一台电脑等待确认")
		}
	}
	attempt.TenantID = session.TenantID
	attempt.ProjectID = session.ProjectID
	attempt.ConnectSessionID = session.ID
	s.bootstrapAttempts[attempt.ID] = attempt
	s.bootstrapEvents[attempt.ID] = map[int64]domain.BootstrapProgressEvent{}
	return attempt, nil
}

func (s *Store) BootstrapAttempt(_ context.Context, tenantID, id string) (domain.BootstrapAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	attempt, ok := s.bootstrapAttempts[id]
	if !ok || attempt.TenantID != tenantID {
		return attempt, domain.NotFound("初始化尝试")
	}
	attempt.AttemptTokenHash = ""
	attempt.CodeChallenge = ""
	return attempt, nil
}

func (s *Store) BootstrapAttemptByTokenHash(_ context.Context, tokenHash string) (domain.BootstrapAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, attempt := range s.bootstrapAttempts {
		if attempt.AttemptTokenHash == tokenHash {
			return attempt, nil
		}
	}
	return domain.BootstrapAttempt{}, domain.NotFound("初始化授权")
}

func (s *Store) ApproveBootstrapAttempt(_ context.Context, tenantID, sessionID, attemptID, userID string, now time.Time) (domain.BootstrapAttempt, error) {
	return s.decideBootstrapAttempt(tenantID, sessionID, attemptID, userID, "approved", now)
}

func (s *Store) DenyBootstrapAttempt(_ context.Context, tenantID, sessionID, attemptID, userID string, now time.Time) (domain.BootstrapAttempt, error) {
	return s.decideBootstrapAttempt(tenantID, sessionID, attemptID, userID, "denied", now)
}

func (s *Store) decideBootstrapAttempt(tenantID, sessionID, attemptID, userID, state string, now time.Time) (domain.BootstrapAttempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.bootstrapAttempts[attemptID]
	if !ok || attempt.TenantID != tenantID || attempt.ConnectSessionID != sessionID {
		return attempt, domain.NotFound("初始化授权")
	}
	session, ok := s.connects[sessionID]
	if !ok || session.State != "waiting_for_computer" || now.After(session.ExpiresAt) {
		return attempt, domain.Conflict("CONNECT_SESSION_UNAVAILABLE", "连接会话已过期、取消或被使用")
	}
	if attempt.State != "pending" || now.After(attempt.ExpiresAt) {
		return attempt, domain.Conflict("BOOTSTRAP_AUTHORIZATION_STATE_INVALID", "初始化授权已过期或已经处理")
	}
	attempt.State = state
	attempt.DecidedBy = userID
	attempt.UpdatedAt = now
	attempt.DecidedAt = &now
	if state == "denied" {
		session.State = "canceled"
		s.connects[session.ID] = session
	}
	s.bootstrapAttempts[attempt.ID] = attempt
	return sanitizedAttempt(attempt), nil
}

func (s *Store) AppendBootstrapProgress(_ context.Context, tokenHash string, event domain.BootstrapProgressEvent, now time.Time) (domain.BootstrapProgressEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attemptID := ""
	var attempt domain.BootstrapAttempt
	for id, candidate := range s.bootstrapAttempts {
		if candidate.AttemptTokenHash == tokenHash {
			attemptID, attempt = id, candidate
			break
		}
	}
	if attemptID == "" || now.After(attempt.ExpiresAt.Add(30*time.Minute)) {
		return event, domain.Conflict("BOOTSTRAP_ATTEMPT_TOKEN_INVALID", "初始化尝试凭据无效或已过期")
	}
	event.AttemptID = attemptID
	if existing, ok := s.bootstrapEvents[attemptID][event.Sequence]; ok {
		if reflect.DeepEqual(existing, event) {
			return existing, nil
		}
		return event, domain.Conflict("BOOTSTRAP_PROGRESS_SEQUENCE_CONFLICT", "同一进度序号（sequence）已存在不同事件")
	}
	if attempt.State == "completed" || attempt.State == "failed" || attempt.State == "denied" {
		return event, domain.Conflict("BOOTSTRAP_PROGRESS_TERMINAL", "初始化尝试进入终态后不能追加新进度")
	}
	if event.Sequence != attempt.LastSequence+1 {
		return event, domain.Conflict("BOOTSTRAP_PROGRESS_SEQUENCE_GAP", "初始化进度序号（sequence）必须连续递增")
	}
	s.bootstrapEvents[attemptID][event.Sequence] = event
	attempt.LastSequence = event.Sequence
	attempt.UpdatedAt = now
	s.bootstrapAttempts[attemptID] = attempt
	return event, nil
}

func (s *Store) BootstrapProgressForSession(_ context.Context, tenantID, sessionID string) (*domain.BootstrapProgress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	attempts := make([]domain.BootstrapAttempt, 0)
	for _, attempt := range s.bootstrapAttempts {
		if attempt.TenantID == tenantID && attempt.ConnectSessionID == sessionID {
			attempts = append(attempts, attempt)
		}
	}
	if len(attempts) == 0 {
		return nil, nil
	}
	sort.Slice(attempts, func(i, j int) bool { return attempts[i].CreatedAt.After(attempts[j].CreatedAt) })
	attempt := attempts[0]
	var latest domain.BootstrapProgressEvent
	for sequence, event := range s.bootstrapEvents[attempt.ID] {
		if sequence > latest.Sequence {
			latest = event
		}
	}
	return domain.BootstrapProgressFrom(attempt, latest), nil
}

func (s *Store) ConsumeBootstrapAttempt(_ context.Context, tokenHash string, device domain.Device, workspace domain.WorkspaceBinding, now time.Time) (domain.ConnectSession, domain.BootstrapAttempt, domain.Device, domain.WorkspaceBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var attempt domain.BootstrapAttempt
	for _, candidate := range s.bootstrapAttempts {
		if candidate.AttemptTokenHash == tokenHash {
			attempt = candidate
			break
		}
	}
	if attempt.ID == "" || attempt.State != "approved" || attempt.ConsumedAt != nil || now.After(attempt.ExpiresAt) {
		return domain.ConnectSession{}, attempt, domain.Device{}, domain.WorkspaceBinding{}, domain.Conflict("BOOTSTRAP_AUTHORIZATION_INVALID", "初始化授权无效、已使用或已过期")
	}
	session, ok := s.connects[attempt.ConnectSessionID]
	if !ok || session.State != "waiting_for_computer" || now.After(session.ExpiresAt) {
		return session, attempt, domain.Device{}, domain.WorkspaceBinding{}, domain.Conflict("CONNECT_SESSION_UNAVAILABLE", "连接会话已过期、取消或被使用")
	}
	for _, existing := range s.devices {
		if existing.TenantID == session.TenantID && existing.MachineID == device.MachineID && existing.RevokedAt == nil {
			device.ID = existing.ID
			device.ProjectIDs = append([]string(nil), existing.ProjectIDs...)
			device.CredentialVersion = existing.CredentialVersion + 1
			break
		}
	}
	if device.CredentialVersion < 1 {
		device.CredentialVersion = 1
	}
	device.CredentialRotatedAt = now
	session.State = "verifying"
	session.ConsumedAt = &now
	session.ConsumedDeviceID = device.ID
	s.connects[session.ID] = session
	device.TenantID = session.TenantID
	device.OwnerUserID = session.InviterUserID
	if !contains(device.ProjectIDs, session.ProjectID) {
		device.ProjectIDs = append(device.ProjectIDs, session.ProjectID)
	}
	s.devices[device.ID] = device
	for _, existing := range s.workspaceBindings {
		if existing.TenantID == session.TenantID && existing.ProjectID == session.ProjectID && existing.DeviceID == device.ID && existing.Status == "active" && existing.RevokedAt == nil {
			workspace.ID = existing.ID
			workspace.InitializedAt = existing.InitializedAt
			break
		}
	}
	workspace.TenantID = session.TenantID
	workspace.ProjectID = session.ProjectID
	workspace.DeviceID = device.ID
	workspace.OwnerUserID = session.InviterUserID
	s.workspaceBindings[workspace.ID] = workspace
	attempt.State = "consumed"
	attempt.ConsumedAt = &now
	attempt.UpdatedAt = now
	s.bootstrapAttempts[attempt.ID] = attempt
	return session, sanitizedAttempt(attempt), device, workspace, nil
}

func (s *Store) CompleteBootstrapAttempt(_ context.Context, tokenHash, state string, now time.Time) (domain.BootstrapAttempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state != "completed" && state != "failed" {
		return domain.BootstrapAttempt{}, domain.Invalid("BOOTSTRAP_ATTEMPT_STATE_INVALID", "初始化完成状态无效")
	}
	for id, attempt := range s.bootstrapAttempts {
		if attempt.AttemptTokenHash != tokenHash {
			continue
		}
		if attempt.State == state {
			return sanitizedAttempt(attempt), nil
		}
		if attempt.State != "consumed" || now.After(attempt.ExpiresAt.Add(30*time.Minute)) {
			return sanitizedAttempt(attempt), domain.Conflict("BOOTSTRAP_ATTEMPT_STATE_INVALID", "只有已完成浏览器授权并创建设备的初始化尝试可以进入终态")
		}
		attempt.State = state
		attempt.UpdatedAt = now
		attempt.CompletedAt = &now
		s.bootstrapAttempts[id] = attempt
		return sanitizedAttempt(attempt), nil
	}
	return domain.BootstrapAttempt{}, domain.NotFound("初始化尝试")
}

func (s *Store) CreateBootstrapDiagnostic(_ context.Context, diagnostic domain.BootstrapDiagnostic) (domain.BootstrapDiagnostic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.bootstrapDiagnostics[diagnostic.ID]; ok {
		if reflect.DeepEqual(existing, diagnostic) {
			return existing, nil
		}
		return diagnostic, domain.Conflict("BOOTSTRAP_DIAGNOSTIC_CONFLICT", "诊断摘要 ID 已存在不同内容")
	}
	for _, existing := range s.bootstrapDiagnostics {
		if existing.TenantID == diagnostic.TenantID && existing.AttemptID == diagnostic.AttemptID && existing.Digest == diagnostic.Digest {
			return existing, nil
		}
	}
	s.bootstrapDiagnostics[diagnostic.ID] = diagnostic
	return diagnostic, nil
}

func sanitizedAttempt(attempt domain.BootstrapAttempt) domain.BootstrapAttempt {
	attempt.AttemptTokenHash = ""
	attempt.CodeChallenge = ""
	return attempt
}
