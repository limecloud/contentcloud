package worker

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/persistence"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

type RuntimeEventRun struct {
	Tenants                 int        `json:"tenants"`
	ReaperTenants           int        `json:"reaper_tenants"`
	BusinessClaimed         int        `json:"business_claimed"`
	BusinessApplied         int        `json:"business_applied"`
	BusinessRetried         int        `json:"business_retried"`
	BusinessPending         int        `json:"business_pending"`
	OldestBusinessPending   *time.Time `json:"oldest_business_pending,omitempty"`
	ProjectionClaims        int        `json:"projection_claims"`
	Projected               int        `json:"projected"`
	ProjectionPending       int        `json:"projection_pending"`
	OldestProjectionPending *time.Time `json:"oldest_projection_pending,omitempty"`
}

func RuntimeEventWorkerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "contentcloud"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}

// ProcessRuntimeEvents advances the two durable subscribers backed by the
// Runtime outbox: business result materialization and Runtime Explorer.
func ProcessRuntimeEvents(ctx context.Context, identity persistence.IdentityRepository, repo contentruntime.Repository, service *application.Application, workerID string, limit int) (RuntimeEventRun, error) {
	result := RuntimeEventRun{}
	if identity == nil || repo == nil {
		return result, fault.Policy("RUNTIME_REPOSITORY_REQUIRED", "Runtime 事件 worker 需要身份与 Runtime 持久化端口", "升级存储实现后重试")
	}
	tenants, err := identity.PlatformTenants(ctx)
	if err != nil {
		return result, err
	}
	projector := contentruntime.NewProjector(repo, time.Now)
	for _, tenant := range tenants {
		if tenant.Status != "active" {
			continue
		}
		result.Tenants++
		startedAt := time.Now().UTC()
		reaperHeartbeat := contentruntime.RuntimeMaintenanceHeartbeat{TenantID: tenant.ID, Kind: contentruntime.RuntimeMaintenanceReaper, WorkerID: workerID, State: "running", LastStartedAt: startedAt, UpdatedAt: startedAt}
		if err := repo.SaveRuntimeMaintenanceHeartbeat(ctx, reaperHeartbeat); err != nil {
			return result, err
		}
		if err := service.Runtime.Runtime().ExpireNodeLeases(ctx, tenant.ID, startedAt); err != nil {
			reaperHeartbeat.State, reaperHeartbeat.LastErrorCode, reaperHeartbeat.UpdatedAt = "failed", "RUNTIME_REAPER_FAILED", time.Now().UTC()
			_ = repo.SaveRuntimeMaintenanceHeartbeat(ctx, reaperHeartbeat)
			return result, err
		}
		reaperCompletedAt := time.Now().UTC()
		reaperHeartbeat.State, reaperHeartbeat.LastSuccessAt, reaperHeartbeat.LastErrorCode, reaperHeartbeat.UpdatedAt = "succeeded", &reaperCompletedAt, "", reaperCompletedAt
		if err := repo.SaveRuntimeMaintenanceHeartbeat(ctx, reaperHeartbeat); err != nil {
			return result, err
		}
		result.ReaperTenants++

		deliveryHeartbeat := contentruntime.RuntimeMaintenanceHeartbeat{TenantID: tenant.ID, Kind: contentruntime.RuntimeMaintenanceDelivery, WorkerID: workerID, State: "running", LastStartedAt: reaperCompletedAt, UpdatedAt: reaperCompletedAt}
		if err := repo.SaveRuntimeMaintenanceHeartbeat(ctx, deliveryHeartbeat); err != nil {
			return result, err
		}
		business, err := service.Runtime.ConsumeRuntimeBusinessResults(ctx, tenant.ID, workerID+":business", time.Minute, limit)
		if err != nil {
			recordRuntimeDeliveryFailure(ctx, repo, deliveryHeartbeat)
			return result, err
		}
		result.BusinessClaimed += business.Claimed
		result.BusinessApplied += business.Applied
		result.BusinessRetried += business.Retried
		projection, err := projector.RunOnce(ctx, tenant.ID, workerID+":projection", time.Minute, limit)
		if err != nil {
			recordRuntimeDeliveryFailure(ctx, repo, deliveryHeartbeat)
			return result, err
		}
		result.ProjectionClaims += projection.Claimed
		result.Projected += projection.Projected
		businessStats, err := repo.RuntimeOutboxStats(ctx, tenant.ID, contentruntime.RuntimeOutboxSubscriberBusinessResult)
		if err != nil {
			recordRuntimeDeliveryFailure(ctx, repo, deliveryHeartbeat)
			return result, err
		}
		projectionStats, err := repo.RuntimeOutboxStats(ctx, tenant.ID, contentruntime.RuntimeOutboxSubscriberProjection)
		if err != nil {
			recordRuntimeDeliveryFailure(ctx, repo, deliveryHeartbeat)
			return result, err
		}
		result.BusinessPending += businessStats.Pending
		result.ProjectionPending += projectionStats.Pending
		result.OldestBusinessPending = earlierTime(result.OldestBusinessPending, businessStats.OldestPending)
		result.OldestProjectionPending = earlierTime(result.OldestProjectionPending, projectionStats.OldestPending)
		deliveryCompletedAt := time.Now().UTC()
		deliveryHeartbeat.State, deliveryHeartbeat.LastSuccessAt, deliveryHeartbeat.LastErrorCode, deliveryHeartbeat.UpdatedAt = "succeeded", &deliveryCompletedAt, "", deliveryCompletedAt
		if err := repo.SaveRuntimeMaintenanceHeartbeat(ctx, deliveryHeartbeat); err != nil {
			return result, err
		}
	}
	return result, nil
}

func recordRuntimeDeliveryFailure(ctx context.Context, repo contentruntime.Repository, heartbeat contentruntime.RuntimeMaintenanceHeartbeat) {
	heartbeat.State, heartbeat.LastErrorCode, heartbeat.UpdatedAt = "failed", "RUNTIME_DELIVERY_FAILED", time.Now().UTC()
	_ = repo.SaveRuntimeMaintenanceHeartbeat(ctx, heartbeat)
}

func earlierTime(current, candidate *time.Time) *time.Time {
	if candidate == nil {
		return current
	}
	if current == nil || candidate.Before(*current) {
		value := *candidate
		return &value
	}
	return current
}
