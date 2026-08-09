package memory

import (
	"context"
	"sort"
	"strings"

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
	for _, receipt := range s.runtimeOutboxReceipts {
		if receipt.TenantID == tenantID && receipt.Subscriber == domain.RuntimeOutboxSubscriberProjection && receipt.DeliveredAt == nil {
			stats.Pending++
			if stats.OldestPending == nil || receipt.CreatedAt.Before(*stats.OldestPending) {
				created := receipt.CreatedAt
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

func (s *Store) RuntimeOutboxStats(_ context.Context, tenantID, subscriber string) (domain.RuntimeOutboxStats, error) {
	subscriber = strings.TrimSpace(subscriber)
	if subscriber == "" {
		return domain.RuntimeOutboxStats{}, domain.Invalid("OUTBOX_SUBSCRIBER_REQUIRED", "outbox 统计需要订阅者")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := domain.RuntimeOutboxStats{TenantID: tenantID, Subscriber: subscriber}
	for _, receipt := range s.runtimeOutboxReceipts {
		if receipt.TenantID != tenantID || receipt.Subscriber != subscriber || receipt.DeliveredAt != nil {
			continue
		}
		stats.Pending++
		if stats.OldestPending == nil || receipt.CreatedAt.Before(*stats.OldestPending) {
			created := receipt.CreatedAt
			stats.OldestPending = &created
		}
	}
	return stats, nil
}

func runtimeMaintenanceKey(tenantID, kind string) string { return tenantID + ":" + kind }

func (s *Store) SaveRuntimeMaintenanceHeartbeat(_ context.Context, value domain.RuntimeMaintenanceHeartbeat) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeMaintenanceKey(value.TenantID, value.Kind)
	if previous, ok := s.runtimeMaintenance[key]; ok {
		if value.UpdatedAt.Before(previous.UpdatedAt) {
			return nil
		}
		if value.LastSuccessAt == nil {
			value.LastSuccessAt = previous.LastSuccessAt
		}
		value.Version = previous.Version + 1
	} else {
		value.Version = 1
	}
	s.runtimeMaintenance[key] = value
	return nil
}

func (s *Store) RuntimeMaintenanceHeartbeat(_ context.Context, tenantID, kind string) (domain.RuntimeMaintenanceHeartbeat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeMaintenance[runtimeMaintenanceKey(tenantID, kind)]
	if !ok {
		return value, domain.NotFound("Runtime 运维心跳")
	}
	return value, nil
}

func sortedProjectionNodes(nodes []domain.NodeRun) []domain.NodeRun {
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeKey < nodes[j].NodeKey })
	return nodes
}
