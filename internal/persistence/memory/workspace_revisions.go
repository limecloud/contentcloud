package memory

import (
	"context"
	"slices"
	"sort"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func (s *Store) PublishWorkspaceRevision(_ context.Context, value workspacedomain.WorkspaceRevision) (workspacedomain.WorkspaceRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding, ok := s.workspaceBindings[value.WorkspaceID]
	if !ok || binding.TenantID != value.TenantID || binding.ProjectID != value.ProjectID || binding.DeviceID != value.DeviceID || binding.Status != "active" || binding.RevokedAt != nil {
		return workspacedomain.WorkspaceRevision{}, fault.Conflict("WORKSPACE_BINDING_INVALID", "工作区绑定无效或与当前设备不匹配")
	}
	var latest workspacedomain.WorkspaceRevision
	for _, existing := range s.workspaceRevisions {
		if existing.TenantID != value.TenantID || existing.WorkspaceID != value.WorkspaceID {
			continue
		}
		if existing.IdempotencyKey == value.IdempotencyKey {
			if sameWorkspaceRevision(existing, value) {
				return existing, nil
			}
			return workspacedomain.WorkspaceRevision{}, fault.Conflict("WORKSPACE_REVISION_IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同的工作区 Revision")
		}
		if existing.RevisionNo > latest.RevisionNo {
			latest = existing
		}
	}
	currentID := "0"
	if latest.ID != "" {
		currentID = latest.ID
	}
	if value.BaseRevisionID != currentID {
		conflict := fault.Conflict("WORKSPACE_REVISION_STALE", "Cloud Revision 已变化，拒绝覆盖新版本")
		conflict.Details = map[string]any{"expected_base_revision": currentID, "provided_base_revision": value.BaseRevisionID}
		return workspacedomain.WorkspaceRevision{}, conflict
	}
	if latest.ID != "" && latest.ContentDigest == value.ContentDigest {
		return workspacedomain.WorkspaceRevision{}, fault.Conflict("WORKSPACE_REVISION_UNCHANGED", "工作区内容摘要未变化")
	}
	value.RevisionNo = latest.RevisionNo + 1
	s.workspaceRevisions[value.ID] = value
	return value, nil
}

func (s *Store) WorkspaceRevisionsAfter(_ context.Context, tenantID, workspaceID string, after int64, limit int) ([]workspacedomain.WorkspaceRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := make([]workspacedomain.WorkspaceRevision, 0)
	for _, candidate := range s.workspaceRevisions {
		if candidate.TenantID == tenantID && candidate.WorkspaceID == workspaceID && candidate.RevisionNo > after {
			values = append(values, candidate)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].RevisionNo < values[j].RevisionNo })
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}

func (s *Store) LatestWorkspaceRevision(_ context.Context, tenantID, workspaceID string) (workspacedomain.WorkspaceRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest workspacedomain.WorkspaceRevision
	for _, candidate := range s.workspaceRevisions {
		if candidate.TenantID == tenantID && candidate.WorkspaceID == workspaceID && candidate.RevisionNo > latest.RevisionNo {
			latest = candidate
		}
	}
	if latest.ID == "" {
		return latest, fault.NotFound("工作区 Revision")
	}
	return latest, nil
}

func sameWorkspaceRevision(left, right workspacedomain.WorkspaceRevision) bool {
	return left.ProjectID == right.ProjectID && left.WorkspaceID == right.WorkspaceID && left.DeviceID == right.DeviceID &&
		left.BaseRevisionID == right.BaseRevisionID && left.ContentDigest == right.ContentDigest && slices.Equal(left.Files, right.Files) && left.ClientMutationID == right.ClientMutationID
}
