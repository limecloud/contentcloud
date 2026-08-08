package app

import (
	"context"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const operationsExecutorOnlineWindow = 2 * time.Minute

type OperationsExecutorProject struct {
	ID          string `json:"id"`
	BrandName   string `json:"brand_name"`
	ProductName string `json:"product_name"`
	Status      string `json:"status"`
}

type OperationsExecutor struct {
	ID           string                      `json:"id"`
	TenantID     string                      `json:"tenant_id"`
	DisplayName  string                      `json:"display_name"`
	ExecutorType string                      `json:"executor_type"`
	Status       string                      `json:"status"`
	StatusReason string                      `json:"status_reason"`
	Hostname     string                      `json:"hostname"`
	Platform     string                      `json:"platform"`
	Arch         string                      `json:"arch"`
	Version      string                      `json:"version"`
	Capabilities []domain.Capability         `json:"capabilities"`
	Projects     []OperationsExecutorProject `json:"projects"`
	LastSeenAt   time.Time                   `json:"last_seen_at"`
	RevokedAt    *time.Time                  `json:"revoked_at,omitempty"`
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
	projectByID := make(map[string]domain.Project, len(projects))
	for _, project := range projects {
		projectByID[project.ID] = project
	}
	executors := make([]OperationsExecutor, 0, len(devices))
	for _, device := range devices {
		executors = append(executors, projectOperationsExecutor(device, projectByID, now))
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
	projectByID := make(map[string]domain.Project, len(projects))
	for _, project := range projects {
		projectByID[project.ID] = project
	}
	return projectOperationsExecutor(device, projectByID, s.now().UTC()), nil
}

func projectOperationsExecutor(device domain.Device, projectByID map[string]domain.Project, now time.Time) OperationsExecutor {
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
	status, reason := "offline", "heartbeat_stale"
	if device.RevokedAt != nil {
		status, reason = "revoked", "registration_revoked"
	} else if device.LastSeenAt.After(now.Add(-operationsExecutorOnlineWindow)) {
		status, reason = "online", "heartbeat_recent"
	}
	return OperationsExecutor{
		ID:           device.ID,
		TenantID:     device.TenantID,
		DisplayName:  device.DisplayName,
		ExecutorType: "contentcloud_device",
		Status:       status,
		StatusReason: reason,
		Hostname:     device.Hostname,
		Platform:     device.Platform,
		Arch:         device.Arch,
		Version:      device.Version,
		Capabilities: capabilities,
		Projects:     projects,
		LastSeenAt:   device.LastSeenAt,
		RevokedAt:    device.RevokedAt,
	}
}
