package app

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
)

const operationsExecutorOnlineWindow = daemonInstanceFreshFor

type OperationsExecutorProject struct {
	ID          string `json:"id"`
	BrandName   string `json:"brand_name"`
	ProductName string `json:"product_name"`
	Status      string `json:"status"`
}

type OperationsExecutorRuntime struct {
	Kind         string                           `json:"kind"`
	Version      string                           `json:"version,omitempty"`
	Status       string                           `json:"status"`
	ErrorCode    string                           `json:"error_code,omitempty"`
	Selected     bool                             `json:"selected"`
	Capabilities agentadapter.HarnessCapabilities `json:"capabilities,omitempty"`
}

type OperationsExecutorWorkspace struct {
	WorkspaceID                string    `json:"workspace_id"`
	ProjectID                  string    `json:"project_id"`
	Status                     string    `json:"status"`
	Reason                     string    `json:"reason"`
	ErrorCode                  string    `json:"error_code,omitempty"`
	Generation                 string    `json:"generation,omitempty"`
	EnvironmentDigest          string    `json:"environment_digest,omitempty"`
	PluginDeclarationDigest    string    `json:"plugin_declaration_digest,omitempty"`
	SkillDeclarationDigest     string    `json:"skill_declaration_digest,omitempty"`
	MCPDeclarationDigest       string    `json:"mcp_declaration_digest,omitempty"`
	WorkspaceDeclarationDigest string    `json:"workspace_declaration_digest,omitempty"`
	PluginReceiptDigest        string    `json:"plugin_receipt_digest,omitempty"`
	SkillObservationDigest     string    `json:"skill_observation_digest,omitempty"`
	MCPObservationDigest       string    `json:"mcp_observation_digest,omitempty"`
	WorkspaceObservationDigest string    `json:"workspace_observation_digest,omitempty"`
	ObservedAt                 time.Time `json:"observed_at"`
}

type OperationsExecutor struct {
	ID                string                        `json:"id"`
	TenantID          string                        `json:"tenant_id"`
	DisplayName       string                        `json:"display_name"`
	ExecutorType      string                        `json:"executor_type"`
	Status            string                        `json:"status"`
	StatusReason      string                        `json:"status_reason"`
	PresenceStatus    string                        `json:"presence_status"`
	PresenceReason    string                        `json:"presence_reason,omitempty"`
	EnvironmentStatus string                        `json:"environment_status"`
	EnvironmentReason string                        `json:"environment_reason,omitempty"`
	RuntimeStatus     string                        `json:"runtime_status"`
	RuntimeReason     string                        `json:"runtime_reason,omitempty"`
	DaemonInstanceID  string                        `json:"daemon_instance_id,omitempty"`
	ConnectionEpoch   int64                         `json:"connection_epoch,omitempty"`
	ActiveAttemptIDs  []string                      `json:"active_attempt_ids"`
	Runtimes          []OperationsExecutorRuntime   `json:"runtimes"`
	Workspaces        []OperationsExecutorWorkspace `json:"workspaces"`
	Hostname          string                        `json:"hostname"`
	Platform          string                        `json:"platform"`
	Arch              string                        `json:"arch"`
	Version           string                        `json:"version"`
	Capabilities      []domain.Capability           `json:"capabilities"`
	Projects          []OperationsExecutorProject   `json:"projects"`
	LastSeenAt        time.Time                     `json:"last_seen_at"`
	RevokedAt         *time.Time                    `json:"revoked_at,omitempty"`
}

type OperationsExecutorDirectory struct {
	Executors           []OperationsExecutor `json:"executors"`
	GeneratedAt         time.Time            `json:"generated_at"`
	OnlineWindowSeconds int                  `json:"online_window_seconds"`
}

func (s *Service) OperationsExecutors(ctx context.Context, actor Actor) (OperationsExecutorDirectory, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return OperationsExecutorDirectory{}, err
	}
	now := s.now().UTC()
	devices, err := s.store.Devices(ctx, actor.TenantID, "")
	if err != nil {
		return OperationsExecutorDirectory{}, err
	}
	projects, err := s.store.Projects(ctx, actor.TenantID)
	if err != nil {
		return OperationsExecutorDirectory{}, err
	}
	if s.deviceControl == nil {
		return OperationsExecutorDirectory{}, domain.Policy("DAEMON_INSTANCE_STORE_UNAVAILABLE", "DaemonInstance 持久层未配置", "检查服务端设备控制存储配置")
	}
	instances, err := s.deviceControl.DaemonInstances(ctx, actor.TenantID, "")
	if err != nil {
		return OperationsExecutorDirectory{}, err
	}
	projectByID := make(map[string]domain.Project, len(projects))
	for _, project := range projects {
		projectByID[project.ID] = project
	}
	instanceByDeviceID := currentDaemonInstances(instances)
	executors := make([]OperationsExecutor, 0, len(devices))
	for _, device := range devices {
		executors = append(executors, projectOperationsExecutor(device, projectByID, instanceByDeviceID[device.ID], now))
	}
	return OperationsExecutorDirectory{
		Executors:           executors,
		GeneratedAt:         now,
		OnlineWindowSeconds: int(operationsExecutorOnlineWindow / time.Second),
	}, nil
}

func (s *Service) OperationsExecutor(ctx context.Context, actor Actor, id string) (OperationsExecutor, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return OperationsExecutor{}, err
	}
	device, err := s.store.Device(ctx, actor.TenantID, id)
	if err != nil {
		return OperationsExecutor{}, err
	}
	projects, err := s.store.Projects(ctx, actor.TenantID)
	if err != nil {
		return OperationsExecutor{}, err
	}
	if s.deviceControl == nil {
		return OperationsExecutor{}, domain.Policy("DAEMON_INSTANCE_STORE_UNAVAILABLE", "DaemonInstance 持久层未配置", "检查服务端设备控制存储配置")
	}
	instances, err := s.deviceControl.DaemonInstances(ctx, actor.TenantID, device.ID)
	if err != nil {
		return OperationsExecutor{}, err
	}
	projectByID := make(map[string]domain.Project, len(projects))
	for _, project := range projects {
		projectByID[project.ID] = project
	}
	return projectOperationsExecutor(device, projectByID, currentDaemonInstances(instances)[device.ID], s.now().UTC()), nil
}

func projectOperationsExecutor(device domain.Device, projectByID map[string]domain.Project, instance domain.DaemonInstance, now time.Time) OperationsExecutor {
	capabilities := append([]domain.Capability{}, device.Capabilities...)
	sort.Slice(capabilities, func(i, j int) bool {
		if capabilities[i].ID == capabilities[j].ID {
			return capabilities[i].Version < capabilities[j].Version
		}
		return capabilities[i].ID < capabilities[j].ID
	})
	projects := make([]OperationsExecutorProject, 0, len(device.ProjectIDs))
	for _, projectID := range device.ProjectIDs {
		project := projectByID[projectID]
		projects = append(projects, OperationsExecutorProject{ID: projectID, BrandName: project.BrandName, ProductName: project.ProductName, Status: project.Status})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].ID < projects[j].ID })
	presenceStatus, presenceReason, environmentStatus, environmentReason, runtimeStatus, runtimeReason := operationsExecutorHealth(device, instance, now)
	status, statusReason := presenceStatus, presenceReason
	if device.RevokedAt != nil {
		status, statusReason = "revoked", "registration_revoked"
	}
	activeAttemptIDs := append([]string{}, instance.ActiveAttempts...)
	sort.Strings(activeAttemptIDs)
	runtimes := operationsExecutorRuntimes(instance.Capabilities)
	workspaces := operationsExecutorWorkspaces(instance.Capabilities)
	lastSeenAt := device.LastSeenAt
	if instance.LastSeenAt.After(lastSeenAt) {
		lastSeenAt = instance.LastSeenAt
	}
	return OperationsExecutor{
		ID: device.ID, TenantID: device.TenantID, DisplayName: device.DisplayName, ExecutorType: "contentcloud_device",
		Status: status, StatusReason: statusReason,
		PresenceStatus: presenceStatus, PresenceReason: presenceReason,
		EnvironmentStatus: environmentStatus, EnvironmentReason: environmentReason,
		RuntimeStatus: runtimeStatus, RuntimeReason: runtimeReason,
		DaemonInstanceID: instance.ID, ConnectionEpoch: instance.ConnectionEpoch, ActiveAttemptIDs: activeAttemptIDs, Runtimes: runtimes, Workspaces: workspaces,
		Hostname: device.Hostname, Platform: device.Platform, Arch: device.Arch, Version: device.Version,
		Capabilities: capabilities, Projects: projects, LastSeenAt: lastSeenAt, RevokedAt: device.RevokedAt,
	}
}

func operationsExecutorWorkspaces(capabilities map[string]any) []OperationsExecutorWorkspace {
	result := []OperationsExecutorWorkspace{}
	body, err := json.Marshal(capabilities["workspace_observations"])
	if err != nil {
		return result
	}
	var observations []domain.DaemonWorkspaceObservation
	if json.Unmarshal(body, &observations) != nil {
		return result
	}
	for _, observation := range observations {
		result = append(result, OperationsExecutorWorkspace{
			WorkspaceID: observation.WorkspaceID, ProjectID: observation.ProjectID, Status: observation.Status,
			Reason: observation.Reason, ErrorCode: observation.ErrorCode, Generation: observation.Generation,
			EnvironmentDigest: observation.EnvironmentDeclaration, PluginDeclarationDigest: observation.PluginDeclaration,
			SkillDeclarationDigest: observation.SkillDeclaration, MCPDeclarationDigest: observation.MCPDeclaration,
			WorkspaceDeclarationDigest: observation.WorkspaceDeclaration, PluginReceiptDigest: observation.PluginHostReceiptDigest,
			SkillObservationDigest: observation.ObservedSkillDigest, MCPObservationDigest: observation.ObservedMCPDigest,
			WorkspaceObservationDigest: observation.ObservedWorkspaceDigest, ObservedAt: observation.ObservedAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ProjectID != result[j].ProjectID {
			return result[i].ProjectID < result[j].ProjectID
		}
		return result[i].WorkspaceID < result[j].WorkspaceID
	})
	return result
}

func operationsExecutorRuntimes(capabilities map[string]any) []OperationsExecutorRuntime {
	result := []OperationsExecutorRuntime{}
	raw, exists := capabilities["runtimes"]
	if !exists || raw == nil {
		return result
	}
	body, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(body, &result) != nil {
		return []OperationsExecutorRuntime{}
	}
	if result == nil {
		result = []OperationsExecutorRuntime{}
	}
	selected, _ := capabilities["harness_kind"].(string)
	for index := range result {
		result[index].Selected = result[index].Kind == selected
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Selected != result[j].Selected {
			return result[i].Selected
		}
		if result[i].Status != result[j].Status {
			return result[i].Status == "healthy"
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}

func currentDaemonInstances(instances []domain.DaemonInstance) map[string]domain.DaemonInstance {
	current := make(map[string]domain.DaemonInstance)
	for _, instance := range instances {
		candidate, ok := current[instance.DeviceID]
		// connection_epoch is scoped to a DaemonInstance process identity. A
		// freshly started process gets a new ID and starts at epoch 1, so epoch
		// must not be compared across different instances.
		instanceLive := instance.State != "stopped" && instance.StoppedAt == nil
		candidateLive := candidate.State != "stopped" && candidate.StoppedAt == nil
		if !ok || (instanceLive && !candidateLive) ||
			(instanceLive == candidateLive && instance.LastSeenAt.After(candidate.LastSeenAt)) ||
			(instanceLive == candidateLive && instance.LastSeenAt.Equal(candidate.LastSeenAt) && instance.ConnectionEpoch > candidate.ConnectionEpoch) ||
			(instanceLive == candidateLive && instance.LastSeenAt.Equal(candidate.LastSeenAt) && instance.ConnectionEpoch == candidate.ConnectionEpoch && instance.ReportSequence > candidate.ReportSequence) {
			current[instance.DeviceID] = instance
		}
	}
	return current
}

func operationsExecutorHealth(device domain.Device, instance domain.DaemonInstance, now time.Time) (string, string, string, string, string, string) {
	environmentStatus, environmentReason := daemonCapabilityStatus(instance.Capabilities, "environment_status", "environment_reason", map[string]bool{
		"ready": true, "repair_required": true, "blocked": true, "unknown": true,
	})
	if environmentStatus == "" {
		environmentStatus, environmentReason = "unknown", "not_reported"
	}
	if device.RevokedAt != nil {
		return "offline", "registration_revoked", "unknown", "registration_revoked", "unavailable", "registration_revoked"
	}
	if instance.ID == "" {
		return "unknown", "instance_not_reported", "unknown", "instance_not_reported", "unknown", "instance_not_reported"
	}
	if instance.State == "stopped" || instance.StoppedAt != nil {
		return "offline", "instance_stopped", "unknown", "instance_stopped", "unavailable", "instance_stopped"
	}
	if !instance.LastSeenAt.After(now.Add(-daemonInstanceFreshFor)) {
		return "offline", "instance_stale", "unknown", "instance_stale", "unavailable", "instance_stale"
	}
	presenceStatus, presenceReason := "online", "instance_connected"
	if instance.State == "degraded" {
		presenceReason = "instance_degraded"
	}
	runtimeStatus, runtimeReason := daemonCapabilityStatus(instance.Capabilities, "runtime_status", "runtime_reason", map[string]bool{
		"healthy": true, "degraded": true, "throttled": true, "unavailable": true, "unknown": true,
	})
	if runtimeStatus == "" {
		if instance.State == "connected" {
			runtimeStatus, runtimeReason = "healthy", "instance_connected"
		} else {
			runtimeStatus, runtimeReason = "degraded", "instance_degraded"
		}
	}
	return presenceStatus, presenceReason, environmentStatus, environmentReason, runtimeStatus, runtimeReason
}

func daemonCapabilityStatus(capabilities map[string]any, statusKey, reasonKey string, allowed map[string]bool) (string, string) {
	status, _ := capabilities[statusKey].(string)
	if !allowed[status] {
		return "", ""
	}
	reason, _ := capabilities[reasonKey].(string)
	return status, reason
}
