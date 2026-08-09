package app

import (
	"context"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	runtimeHeartbeatWarningAge  = 15 * time.Second
	runtimeHeartbeatCriticalAge = time.Minute
	runtimeBacklogWarningAge    = time.Minute
	runtimeBacklogCriticalAge   = 5 * time.Minute
	runtimeBacklogWarningCount  = 100
	runtimeBacklogCriticalCount = 1000
)

type RuntimeHealthThresholds struct {
	HeartbeatWarningSeconds  int64 `json:"heartbeat_warning_seconds"`
	HeartbeatCriticalSeconds int64 `json:"heartbeat_critical_seconds"`
	BacklogWarningSeconds    int64 `json:"backlog_warning_seconds"`
	BacklogCriticalSeconds   int64 `json:"backlog_critical_seconds"`
	BacklogWarningCount      int   `json:"backlog_warning_count"`
	BacklogCriticalCount     int   `json:"backlog_critical_count"`
}

type RuntimeHealthAlert struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Pending    int    `json:"pending,omitempty"`
	AgeSeconds int64  `json:"age_seconds,omitempty"`
}

type RuntimeTenantHealth struct {
	TenantID         string                              `json:"tenant_id"`
	TenantName       string                              `json:"tenant_name"`
	Status           string                              `json:"status"`
	Reaper           *domain.RuntimeMaintenanceHeartbeat `json:"reaper,omitempty"`
	Delivery         *domain.RuntimeMaintenanceHeartbeat `json:"delivery,omitempty"`
	ProjectionOutbox domain.RuntimeOutboxStats           `json:"projection_outbox"`
	BusinessOutbox   domain.RuntimeOutboxStats           `json:"business_outbox"`
	Alerts           []RuntimeHealthAlert                `json:"alerts"`
}

type RuntimeHealthReport struct {
	Status           string                         `json:"status"`
	Tenants          []RuntimeTenantHealth          `json:"tenants"`
	CapacityFairness []domain.RuntimeFairnessReport `json:"capacity_fairness"`
	Thresholds       RuntimeHealthThresholds        `json:"thresholds"`
	GeneratedAt      time.Time                      `json:"generated_at"`
}

func (s *Service) PlatformRuntimeHealth(ctx context.Context, actor Actor) (RuntimeHealthReport, error) {
	if !actor.PlatformAdmin {
		return RuntimeHealthReport{}, domain.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以访问 Runtime 健康状态", "联系系统管理员配置平台权限")
	}
	if s.runtimeService == nil {
		return RuntimeHealthReport{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	now := s.now().UTC()
	report := RuntimeHealthReport{
		Status: "healthy", Tenants: []RuntimeTenantHealth{}, CapacityFairness: []domain.RuntimeFairnessReport{}, GeneratedAt: now,
		Thresholds: RuntimeHealthThresholds{
			HeartbeatWarningSeconds: int64(runtimeHeartbeatWarningAge / time.Second), HeartbeatCriticalSeconds: int64(runtimeHeartbeatCriticalAge / time.Second),
			BacklogWarningSeconds: int64(runtimeBacklogWarningAge / time.Second), BacklogCriticalSeconds: int64(runtimeBacklogCriticalAge / time.Second),
			BacklogWarningCount: runtimeBacklogWarningCount, BacklogCriticalCount: runtimeBacklogCriticalCount,
		},
	}
	tenants, err := s.store.PlatformTenants(ctx)
	if err != nil {
		return RuntimeHealthReport{}, err
	}
	for _, tenant := range tenants {
		if tenant.Status != "active" {
			continue
		}
		health := RuntimeTenantHealth{TenantID: tenant.ID, TenantName: tenant.Name, Status: "healthy", Alerts: []RuntimeHealthAlert{}}
		health.ProjectionOutbox, err = s.runtimeService.RuntimeOutboxStats(ctx, tenant.ID, domain.RuntimeOutboxSubscriberProjection)
		if err != nil {
			return RuntimeHealthReport{}, err
		}
		health.BusinessOutbox, err = s.runtimeService.RuntimeOutboxStats(ctx, tenant.ID, domain.RuntimeOutboxSubscriberBusinessResult)
		if err != nil {
			return RuntimeHealthReport{}, err
		}
		health.Reaper, err = s.runtimeHealthHeartbeat(ctx, tenant.ID, domain.RuntimeMaintenanceReaper, now, &health.Alerts)
		if err != nil {
			return RuntimeHealthReport{}, err
		}
		health.Delivery, err = s.runtimeHealthHeartbeat(ctx, tenant.ID, domain.RuntimeMaintenanceDelivery, now, &health.Alerts)
		if err != nil {
			return RuntimeHealthReport{}, err
		}
		appendRuntimeBacklogAlert(&health.Alerts, now, health.ProjectionOutbox, "RUNTIME_PROJECTION_LAG", "Runtime 投影积压超过健康门槛")
		appendRuntimeBacklogAlert(&health.Alerts, now, health.BusinessOutbox, "RUNTIME_BUSINESS_RESULT_BACKLOG", "Runtime 业务结果交接积压超过健康门槛")
		health.Status = runtimeAlertsStatus(health.Alerts)
		report.Status = worseRuntimeHealthStatus(report.Status, health.Status)
		report.Tenants = append(report.Tenants, health)
	}
	activeTenantIDs := make([]string, 0, len(report.Tenants))
	resourceKeys := map[string]struct{}{}
	for _, tenant := range report.Tenants {
		activeTenantIDs = append(activeTenantIDs, tenant.TenantID)
		quotas, quotaErr := s.runtimeService.Repository().ResourceQuotas(ctx, tenant.TenantID)
		if quotaErr != nil {
			return RuntimeHealthReport{}, quotaErr
		}
		for _, quota := range quotas {
			resourceKeys[quota.ResourceKey] = struct{}{}
		}
	}
	for resourceKey := range resourceKeys {
		fairness, fairnessErr := s.runtimeService.FairnessReport(ctx, activeTenantIDs, resourceKey)
		if fairnessErr != nil {
			return RuntimeHealthReport{}, fairnessErr
		}
		report.CapacityFairness = append(report.CapacityFairness, fairness)
	}
	sort.Slice(report.CapacityFairness, func(i, j int) bool {
		return report.CapacityFairness[i].ResourceKey < report.CapacityFairness[j].ResourceKey
	})
	return report, nil
}

func (s *Service) runtimeHealthHeartbeat(ctx context.Context, tenantID, kind string, now time.Time, alerts *[]RuntimeHealthAlert) (*domain.RuntimeMaintenanceHeartbeat, error) {
	heartbeat, err := s.runtimeService.RuntimeMaintenanceHeartbeat(ctx, tenantID, kind)
	label := "Runtime reaper"
	code := "RUNTIME_REAPER_STALLED"
	if kind == domain.RuntimeMaintenanceDelivery {
		label, code = "Runtime delivery worker", "RUNTIME_DELIVERY_STALLED"
	}
	if domain.IsNotFound(err) {
		*alerts = append(*alerts, RuntimeHealthAlert{Code: code, Severity: "critical", Message: label + " 尚无成功心跳"})
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if heartbeat.LastSuccessAt == nil {
		*alerts = append(*alerts, RuntimeHealthAlert{Code: code, Severity: "critical", Message: label + " 尚无成功心跳"})
		return &heartbeat, nil
	}
	age := now.Sub(*heartbeat.LastSuccessAt)
	if age < 0 {
		age = 0
	}
	severity := ""
	if heartbeat.State == "failed" || age >= runtimeHeartbeatCriticalAge {
		severity = "critical"
	} else if age >= runtimeHeartbeatWarningAge {
		severity = "warning"
	}
	if severity != "" {
		*alerts = append(*alerts, RuntimeHealthAlert{Code: code, Severity: severity, Message: label + " 心跳超过健康门槛", AgeSeconds: int64(age / time.Second)})
	}
	return &heartbeat, nil
}

func appendRuntimeBacklogAlert(alerts *[]RuntimeHealthAlert, now time.Time, stats domain.RuntimeOutboxStats, code, message string) {
	if stats.Pending == 0 {
		return
	}
	age := time.Duration(0)
	if stats.OldestPending != nil {
		age = now.Sub(*stats.OldestPending)
		if age < 0 {
			age = 0
		}
	}
	severity := ""
	if stats.Pending >= runtimeBacklogCriticalCount || age >= runtimeBacklogCriticalAge {
		severity = "critical"
	} else if stats.Pending >= runtimeBacklogWarningCount || age >= runtimeBacklogWarningAge {
		severity = "warning"
	}
	if severity != "" {
		*alerts = append(*alerts, RuntimeHealthAlert{Code: code, Severity: severity, Message: message, Pending: stats.Pending, AgeSeconds: int64(age / time.Second)})
	}
}

func runtimeAlertsStatus(alerts []RuntimeHealthAlert) string {
	status := "healthy"
	for _, alert := range alerts {
		status = worseRuntimeHealthStatus(status, alert.Severity)
	}
	return status
}

func worseRuntimeHealthStatus(current, candidate string) string {
	rank := map[string]int{"healthy": 0, "warning": 1, "critical": 2}
	if rank[candidate] > rank[current] {
		return candidate
	}
	return current
}
