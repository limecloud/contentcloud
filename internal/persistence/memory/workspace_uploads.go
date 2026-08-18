package memory

import (
	"context"
	"fmt"
	"sort"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func (s *Store) CreateWorkspaceUploadSession(_ context.Context, value workspacedomain.WorkspaceUploadSession) (workspacedomain.WorkspaceUploadSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding, ok := s.workspaceBindings[value.WorkspaceID]
	if !ok || binding.TenantID != value.TenantID || binding.ProjectID != value.ProjectID || binding.DeviceID != value.DeviceID || binding.Status != "active" || binding.RevokedAt != nil {
		return workspacedomain.WorkspaceUploadSession{}, fault.Conflict("WORKSPACE_BINDING_INVALID", "工作区绑定无效或与当前设备不匹配")
	}
	for _, existing := range s.workspaceUploadSessions {
		if existing.TenantID != value.TenantID || existing.WorkspaceID != value.WorkspaceID || existing.IdempotencyKey != value.IdempotencyKey {
			continue
		}
		if sameWorkspaceUploadSession(existing, value) {
			return existing, nil
		}
		return workspacedomain.WorkspaceUploadSession{}, fault.Conflict("WORKSPACE_UPLOAD_IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同的上传文件")
	}
	if _, exists := s.workspaceUploadSessions[value.ID]; exists {
		return workspacedomain.WorkspaceUploadSession{}, fault.Conflict("WORKSPACE_UPLOAD_SESSION_CONFLICT", "上传会话已存在")
	}
	s.workspaceUploadSessions[value.ID] = value
	return value, nil
}

func (s *Store) WorkspaceUploadSession(_ context.Context, tenantID, sessionID string) (workspacedomain.WorkspaceUploadSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.workspaceUploadSessions[sessionID]
	if !ok || value.TenantID != tenantID {
		return workspacedomain.WorkspaceUploadSession{}, fault.NotFound("工作区上传会话")
	}
	return value, nil
}

func (s *Store) SaveWorkspaceUploadPart(_ context.Context, tenantID string, value workspacedomain.WorkspaceUploadPart) (workspacedomain.WorkspaceUploadPart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.workspaceUploadSessions[value.SessionID]
	if !ok || session.TenantID != tenantID {
		return workspacedomain.WorkspaceUploadPart{}, fault.NotFound("工作区上传会话")
	}
	if session.State == "completed" {
		return workspacedomain.WorkspaceUploadPart{}, fault.Conflict("WORKSPACE_UPLOAD_COMPLETED", "上传会话已经完成")
	}
	key := workspaceUploadPartKey(tenantID, value.SessionID, value.PartNo)
	if existing, exists := s.workspaceUploadParts[key]; exists {
		if existing.Digest == value.Digest && existing.ByteSize == value.ByteSize && existing.ObjectKey == value.ObjectKey {
			return existing, nil
		}
		return workspacedomain.WorkspaceUploadPart{}, fault.Conflict("WORKSPACE_UPLOAD_PART_CONFLICT", "分片编号已对应其他内容")
	}
	s.workspaceUploadParts[key] = value
	session.State = "uploading"
	session.UpdatedAt = value.CreatedAt
	s.workspaceUploadSessions[session.ID] = session
	return value, nil
}

func (s *Store) WorkspaceUploadParts(_ context.Context, tenantID, sessionID string) ([]workspacedomain.WorkspaceUploadPart, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.workspaceUploadSessions[sessionID]
	if !ok || session.TenantID != tenantID {
		return nil, fault.NotFound("工作区上传会话")
	}
	values := make([]workspacedomain.WorkspaceUploadPart, 0, session.PartCount)
	for partNo := 0; partNo < session.PartCount; partNo++ {
		if value, exists := s.workspaceUploadParts[workspaceUploadPartKey(tenantID, sessionID, partNo)]; exists {
			values = append(values, value)
		}
	}
	return values, nil
}

func (s *Store) CompleteWorkspaceUpload(_ context.Context, session workspacedomain.WorkspaceUploadSession, object workspacedomain.WorkspaceObject) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.workspaceUploadSessions[session.ID]
	if !ok || current.TenantID != session.TenantID {
		return fault.NotFound("工作区上传会话")
	}
	if current.TenantID != object.TenantID || current.ContentDigest != object.ContentDigest || current.ByteSize != object.ByteSize || current.ProjectID != object.ProjectID || current.ObjectKey != object.ObjectKey {
		return fault.Conflict("WORKSPACE_UPLOAD_OBJECT_MISMATCH", "上传对象与会话声明不一致")
	}
	objectKey := workspaceObjectKey(object.TenantID, object.ProjectID, object.ContentDigest)
	if existing, exists := s.workspaceObjects[objectKey]; exists {
		if existing.ByteSize != object.ByteSize || existing.ObjectKey != object.ObjectKey {
			return fault.Conflict("WORKSPACE_OBJECT_CONFLICT", "相同摘要对应了不同的工作区对象")
		}
	} else {
		s.workspaceObjects[objectKey] = object
	}
	current.State = "completed"
	current.UpdatedAt = session.UpdatedAt
	s.workspaceUploadSessions[current.ID] = current
	return nil
}

func (s *Store) WorkspaceObject(_ context.Context, tenantID, projectID, digest string) (workspacedomain.WorkspaceObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.workspaceObjects[workspaceObjectKey(tenantID, projectID, digest)]
	if !ok {
		return workspacedomain.WorkspaceObject{}, fault.NotFound("工作区对象")
	}
	return value, nil
}

func (s *Store) WorkspaceObjects(_ context.Context, tenantID, projectID string, digests []string) ([]workspacedomain.WorkspaceObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := make([]workspacedomain.WorkspaceObject, 0, len(digests))
	for _, digest := range digests {
		if value, ok := s.workspaceObjects[workspaceObjectKey(tenantID, projectID, digest)]; ok {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ContentDigest < values[j].ContentDigest })
	return values, nil
}

func sameWorkspaceUploadSession(left, right workspacedomain.WorkspaceUploadSession) bool {
	return left.ProjectID == right.ProjectID && left.WorkspaceID == right.WorkspaceID && left.DeviceID == right.DeviceID && left.Ref == right.Ref &&
		left.ContentDigest == right.ContentDigest && left.ByteSize == right.ByteSize && left.ChunkSize == right.ChunkSize && left.PartCount == right.PartCount
}

func workspaceUploadPartKey(tenantID, sessionID string, partNo int) string {
	return fmt.Sprintf("%s/%s/%d", tenantID, sessionID, partNo)
}

func workspaceObjectKey(tenantID, projectID, digest string) string {
	return tenantID + "/" + projectID + "/" + digest
}
