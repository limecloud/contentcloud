package memory

import (
	"context"
	"sort"

	"github.com/limecloud/contentcloud/internal/domain"
)

func projectionKey(tenantID, jobID string) string { return tenantID + ":" + jobID }

func (s *Store) SaveRuntimeExplorer(_ context.Context, view domain.RuntimeExplorerView) error {
	if view.TenantID == "" || view.JobRunID == "" || view.ProjectedAt.IsZero() {
		return domain.Invalid("RUNTIME_PROJECTION_INVALID", "Runtime Explorer 投影缺少执行实例或时间")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, ok := s.runtimeProjections[projectionKey(view.TenantID, view.JobRunID)]; ok && previous.LastEventSeq > view.LastEventSeq {
		return domain.Conflict("RUNTIME_PROJECTION_STALE", "投影事件序号不能倒退")
	}
	view.Nodes = append([]domain.NodeRun(nil), view.Nodes...)
	s.runtimeProjections[projectionKey(view.TenantID, view.JobRunID)] = view
	return nil
}

func (s *Store) RuntimeExplorer(_ context.Context, tenantID, jobID string) (domain.RuntimeExplorerView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	view, ok := s.runtimeProjections[projectionKey(tenantID, jobID)]
	if !ok {
		return domain.RuntimeExplorerView{}, domain.NotFound("Runtime Explorer 投影")
	}
	return view, nil
}

func (s *Store) RuntimeProjectionStats(_ context.Context, tenantID string) (domain.RuntimeProjectionStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := domain.RuntimeProjectionStats{TenantID: tenantID}
	for _, message := range s.runtimeOutbox {
		if message.TenantID == tenantID && message.DeliveredAt == nil {
			stats.Pending++
			if stats.OldestPending == nil || message.CreatedAt.Before(*stats.OldestPending) {
				created := message.CreatedAt
				stats.OldestPending = &created
			}
		}
	}
	for _, view := range s.runtimeProjections {
		if view.TenantID == tenantID && (stats.LastProjectedAt == nil || view.ProjectedAt.After(*stats.LastProjectedAt)) {
			projected := view.ProjectedAt
			stats.LastProjectedAt = &projected
		}
	}
	return stats, nil
}

func sortedProjectionNodes(nodes []domain.NodeRun) []domain.NodeRun {
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeKey < nodes[j].NodeKey })
	return nodes
}
