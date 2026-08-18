package runtime

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

// FairnessReport builds an operator-facing capacity snapshot across the
// explicitly supplied tenants. The caller is responsible for choosing a
// tenant set it is allowed to observe; the repository still applies RLS on
// every per-tenant read.
func (s *Service) FairnessReport(ctx context.Context, tenantIDs []string, resourceKey string) (RuntimeFairnessReport, error) {
	if s == nil || s.repo == nil {
		return RuntimeFairnessReport{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	ids := uniqueTenantIDs(tenantIDs)
	if len(ids) == 0 {
		return RuntimeFairnessReport{}, fault.Invalid("RUNTIME_FAIRNESS_TENANTS_REQUIRED", "公平性观测至少需要一个租户")
	}
	resourceKey = strings.TrimSpace(resourceKey)
	now := s.now().UTC()
	rows := make([]RuntimeTenantCapacity, 0, len(ids))
	unit := ""
	for _, tenantID := range ids {
		quotas, err := s.repo.ResourceQuotas(ctx, tenantID)
		if err != nil {
			return RuntimeFairnessReport{}, err
		}
		reservations, err := s.repo.ResourceReservations(ctx, tenantID, "")
		if err != nil {
			return RuntimeFairnessReport{}, err
		}
		byKey := make(map[string]ResourceQuota, len(quotas))
		for _, quota := range quotas {
			if resourceKey == "" || quota.ResourceKey == resourceKey {
				byKey[quota.ResourceKey] = quota
			}
		}
		if resourceKey != "" {
			if _, ok := byKey[resourceKey]; !ok {
				// A tenant without this quota is still represented with zero
				// capacity, making starvation visible in the report.
				byKey[resourceKey] = ResourceQuota{TenantID: tenantID, ResourceKey: resourceKey}
			}
		}
		for key, quota := range byKey {
			row := ComputeTenantCapacity(tenantID, key, quota, reservations, now)
			rows = append(rows, row)
			if unit == "" {
				unit = row.Unit
			}
		}
	}
	if resourceKey == "" {
		keys := map[string]struct{}{}
		for _, row := range rows {
			keys[row.ResourceKey] = struct{}{}
		}
		if len(keys) != 1 {
			return RuntimeFairnessReport{}, fault.Invalid("RUNTIME_FAIRNESS_RESOURCE_REQUIRED", "多个资源配额并存时必须指定 resource_key")
		}
		for key := range keys {
			resourceKey = key
		}
	}
	// ResourceQuotas are per tenant, so rows are one per tenant for an
	// explicitly selected resource. Keep deterministic output for dashboards.
	sort.Slice(rows, func(i, j int) bool { return rows[i].TenantID < rows[j].TenantID })
	if len(rows) == 0 {
		return RuntimeFairnessReport{ResourceKey: resourceKey, Tenants: []RuntimeTenantCapacity{}, ObservedAt: now}, nil
	}
	report := RuntimeFairnessReport{ResourceKey: resourceKey, Unit: unit, Tenants: rows, ObservedAt: now}
	var shareSum, shareSquareSum float64
	for index, row := range rows {
		report.TotalCapacity += row.Capacity
		report.TotalHeld += row.Held
		if row.UtilizationBPS > report.MaxUtilizationBPS {
			report.MaxUtilizationBPS = row.UtilizationBPS
		}
		if index == 0 || row.UtilizationBPS < report.MinUtilizationBPS {
			report.MinUtilizationBPS = row.UtilizationBPS
		}
		share := float64(row.UtilizationBPS) / 10000
		shareSum += share
		shareSquareSum += share * share
	}
	if shareSquareSum > 0 {
		jain := shareSum * shareSum / (float64(len(rows)) * shareSquareSum)
		report.JainIndexBPS = int64(math.Round(jain * 10000))
	}
	return report, nil
}

// ComputeTenantCapacity derives one tenant's capacity row without accessing
// persistence. It is exported so callers and black-box tests can verify the
// fairness calculation independently from report assembly.
func ComputeTenantCapacity(tenantID, resourceKey string, quota ResourceQuota, reservations []ResourceReservation, now time.Time) RuntimeTenantCapacity {
	row := RuntimeTenantCapacity{TenantID: tenantID, ResourceKey: resourceKey, Unit: quota.Unit, Capacity: quota.Capacity, ObservedAt: now}
	for _, reservation := range reservations {
		if reservation.ResourceKey != resourceKey || reservation.State != ReservationHeld {
			continue
		}
		if reservation.ExpiresAt != nil && !reservation.ExpiresAt.After(now) {
			row.ExpiredHeld += reservation.Quantity
			continue
		}
		row.Held += reservation.Quantity
	}
	if row.Capacity > 0 {
		row.UtilizationBPS = row.Held * 10000 / row.Capacity
		if row.UtilizationBPS > 10000 {
			row.UtilizationBPS = 10000
		}
	}
	return row
}

func uniqueTenantIDs(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
