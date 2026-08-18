package memory

import (
	"context"
	"sort"
	"strings"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

func resourceQuotaKey(tenantID, resourceKey string) string { return tenantID + ":" + resourceKey }

func (s *Store) SaveResourceQuota(_ context.Context, quota contentruntime.ResourceQuota) error {
	if err := quota.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := resourceQuotaKey(quota.TenantID, quota.ResourceKey)
	if previous, ok := s.runtimeResourceQuotas[key]; ok && previous.Version != quota.Version-1 {
		return fault.Conflict("RESOURCE_QUOTA_VERSION_CONFLICT", "资源配额已被更新")
	}
	s.runtimeResourceQuotas[key] = quota
	return nil
}

func (s *Store) ResourceQuotas(_ context.Context, tenantID string) ([]contentruntime.ResourceQuota, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []contentruntime.ResourceQuota{}
	for _, quota := range s.runtimeResourceQuotas {
		if quota.TenantID == tenantID {
			result = append(result, quota)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ResourceKey < result[j].ResourceKey })
	return result, nil
}

func (s *Store) ResourceReservations(_ context.Context, tenantID, jobID string) ([]contentruntime.ResourceReservation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []contentruntime.ResourceReservation{}
	for _, reservation := range s.runtimeReservations {
		if reservation.TenantID == tenantID && (jobID == "" || reservation.JobRunID == jobID) {
			result = append(result, reservation)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func validateResourceReservationsLocked(s *Store, reservations []contentruntime.ResourceReservation) error {
	requested := map[string]int64{}
	for _, reservation := range reservations {
		if err := reservation.Validate(); err != nil {
			return err
		}
		if previous, ok := s.runtimeReservations[runtimePlanKey(reservation.TenantID, reservation.ID)]; ok {
			if previous.IdempotencyKey == reservation.IdempotencyKey {
				continue
			}
			return fault.Conflict("RESOURCE_RESERVATION_EXISTS", "资源预留已经存在")
		}
		for _, existing := range s.runtimeReservations {
			if existing.TenantID == reservation.TenantID && existing.IdempotencyKey == reservation.IdempotencyKey {
				return fault.Conflict("RESOURCE_RESERVATION_IDEMPOTENCY", "资源预留幂等键已被不同请求使用")
			}
		}
		requested[resourceQuotaKey(reservation.TenantID, reservation.ResourceKey)] += reservation.Quantity
	}
	for key, amount := range requested {
		quota, hasQuota := s.runtimeResourceQuotas[key]
		if !hasQuota {
			continue
		}
		used := int64(0)
		for _, existing := range s.runtimeReservations {
			if resourceQuotaKey(existing.TenantID, existing.ResourceKey) == key && existing.State == contentruntime.ReservationHeld {
				if strings.TrimSpace(existing.Unit) != quota.Unit {
					return fault.Invalid("RESOURCE_UNIT_MISMATCH", "资源预留单位与配额不一致")
				}
				used += existing.Quantity
			}
		}
		if used+amount > quota.Capacity {
			return fault.Conflict("RESOURCE_QUOTA_EXCEEDED", "资源配额不足，拒绝超卖")
		}
	}
	return nil
}
