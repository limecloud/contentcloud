package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/google/uuid"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

const daemonInstanceFreshFor = 45 * time.Second

type DaemonInstanceReportInput struct {
	ID                    string                                       `json:"daemon_instance_id"`
	ConnectionEpoch       int64                                        `json:"connection_epoch"`
	ReportSequence        int64                                        `json:"report_seq"`
	PID                   int                                          `json:"pid,omitempty"`
	Version               string                                       `json:"version"`
	State                 string                                       `json:"state"`
	Capabilities          map[string]any                               `json:"capabilities,omitempty"`
	WorkspaceObservations []workspacedomain.DaemonWorkspaceObservation `json:"workspace_observations,omitempty"`
	ActiveAttempts        []string                                     `json:"active_attempts,omitempty"`
	StartedAt             time.Time                                    `json:"started_at"`
}

func (s *WorkspaceService) ReportDaemonInstance(ctx context.Context, actor Actor, input DaemonInstanceReportInput) (workspacedomain.DaemonInstance, error) {
	if actor.Type != "device" || strings.TrimSpace(actor.DeviceID) == "" {
		return workspacedomain.DaemonInstance{}, fault.Policy("DEVICE_AUTH_REQUIRED", "DaemonInstance 状态报告只接受设备凭据", "使用已注册设备的凭据")
	}
	input.ID = strings.TrimSpace(input.ID)
	input.Version = strings.TrimSpace(input.Version)
	input.State = strings.TrimSpace(input.State)
	if _, err := uuid.Parse(input.ID); err != nil || input.ConnectionEpoch < 1 || input.ReportSequence < 1 || input.PID < 0 || input.Version == "" || input.StartedAt.IsZero() {
		return workspacedomain.DaemonInstance{}, fault.Invalid("DAEMON_INSTANCE_REPORT_INVALID", "DaemonInstance 状态报告缺少有效的实例、代际、序列或进程信息")
	}
	if input.State != "connected" && input.State != "degraded" && input.State != "stopped" {
		return workspacedomain.DaemonInstance{}, fault.Invalid("DAEMON_INSTANCE_STATE_INVALID", "DaemonInstance 状态无效")
	}
	if len(input.ActiveAttempts) > 32 {
		return workspacedomain.DaemonInstance{}, fault.Policy("DAEMON_INSTANCE_REPORT_TOO_LARGE", "DaemonInstance 活跃 Attempt 数量超过上限", "检查本地 worker 状态并重新同步")
	}
	if len(input.WorkspaceObservations) > 64 {
		return workspacedomain.DaemonInstance{}, fault.Policy("DAEMON_INSTANCE_REPORT_TOO_LARGE", "DaemonInstance 工作区观察数量超过上限", "减少本地工作区绑定后重新同步")
	}
	active := make([]string, 0, len(input.ActiveAttempts))
	seen := map[string]struct{}{}
	for _, attemptID := range input.ActiveAttempts {
		attemptID = strings.TrimSpace(attemptID)
		if attemptID == "" {
			continue
		}
		if _, ok := seen[attemptID]; ok {
			continue
		}
		seen[attemptID] = struct{}{}
		active = append(active, attemptID)
	}
	sort.Strings(active)
	now := s.now().UTC()
	capabilities := cloneDaemonCapabilities(input.Capabilities)
	if capabilities == nil {
		capabilities = map[string]any{}
	}
	if input.WorkspaceObservations != nil {
		observations := make([]workspacedomain.DaemonWorkspaceObservation, 0, len(input.WorkspaceObservations))
		seenWorkspaces := map[string]struct{}{}
		for _, observation := range input.WorkspaceObservations {
			observation.WorkspaceID = strings.TrimSpace(observation.WorkspaceID)
			observation.ProjectID = strings.TrimSpace(observation.ProjectID)
			if observation.WorkspaceID == "" || observation.ProjectID == "" || observation.ObservedAt.IsZero() {
				return workspacedomain.DaemonInstance{}, fault.Invalid("DAEMON_WORKSPACE_OBSERVATION_INVALID", "DaemonInstance 工作区观察缺少工作区、项目或观察时间")
			}
			if _, exists := seenWorkspaces[observation.WorkspaceID]; exists {
				return workspacedomain.DaemonInstance{}, fault.Conflict("DAEMON_WORKSPACE_OBSERVATION_DUPLICATED", "DaemonInstance 工作区观察包含重复工作区")
			}
			seenWorkspaces[observation.WorkspaceID] = struct{}{}
			if observation.Status != "ready" && observation.Status != "repair_required" && observation.Status != "blocked" && observation.Status != "unknown" {
				return workspacedomain.DaemonInstance{}, fault.Invalid("DAEMON_WORKSPACE_OBSERVATION_INVALID", "DaemonInstance 工作区环境状态无效")
			}
			observations = append(observations, observation)
		}
		capabilities["workspace_observations"] = observations
	}
	instance := workspacedomain.DaemonInstance{
		ID: input.ID, TenantID: actor.TenantID, DeviceID: actor.DeviceID,
		ConnectionEpoch: input.ConnectionEpoch, ReportSequence: input.ReportSequence,
		PID: input.PID, Version: input.Version, State: input.State,
		Capabilities: capabilities, ActiveAttempts: active,
		StartedAt: input.StartedAt.UTC(), LastSeenAt: now,
	}
	if instance.State == "stopped" {
		instance.StoppedAt = &now
	}
	if s.deviceControl == nil {
		return workspacedomain.DaemonInstance{}, fault.Policy("DAEMON_INSTANCE_STORE_UNAVAILABLE", "DaemonInstance 持久层未配置", "检查服务端设备控制存储配置")
	}
	if err := s.deviceControl.SaveDaemonInstance(ctx, instance); err != nil {
		return workspacedomain.DaemonInstance{}, err
	}
	device, err := s.workspace.Device(ctx, actor.TenantID, actor.DeviceID)
	if err != nil {
		return workspacedomain.DaemonInstance{}, err
	}
	device.LastSeenAt = now
	device.Version = input.Version
	if err := s.workspace.SaveDevice(ctx, device); err != nil {
		return workspacedomain.DaemonInstance{}, err
	}
	return instance, nil
}

func cloneDaemonCapabilities(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func (s *WorkspaceService) DaemonInstances(ctx context.Context, actor Actor, deviceID string) ([]workspacedomain.DaemonInstance, error) {
	if strings.TrimSpace(actor.TenantID) == "" {
		return nil, fault.Policy("TENANT_REQUIRED", "查询 DaemonInstance 缺少租户范围", "重新登录后重试")
	}
	if s.deviceControl == nil {
		return nil, fault.Policy("DAEMON_INSTANCE_STORE_UNAVAILABLE", "DaemonInstance 持久层未配置", "检查服务端设备控制存储配置")
	}
	return s.deviceControl.DaemonInstances(ctx, actor.TenantID, strings.TrimSpace(deviceID))
}
