package app

import (
	"context"
	"slices"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Service) Device(ctx context.Context, actor Actor, id string) (domain.Device, error) {
	return s.store.Device(ctx, actor.TenantID, id)
}

func (s *Service) AttachDevice(ctx context.Context, actor Actor, deviceID, projectID, requestID string) (domain.Device, error) {
	if actor.Role != "tenant_admin" && actor.Role != "project_manager" {
		return domain.Device{}, domain.Policy("ROLE_DENIED", "当前角色不能授权设备", "联系租户管理员")
	}
	if _, err := s.projectForWrite(ctx, actor, projectID); err != nil {
		return domain.Device{}, err
	}
	if err := s.store.GrantDeviceProject(ctx, actor.TenantID, projectID, deviceID, actor.UserID, s.now().UTC()); err != nil {
		return domain.Device{}, err
	}
	device, err := s.store.Device(ctx, actor.TenantID, deviceID)
	if err == nil {
		s.audit(ctx, actor, projectID, "device.attached", "device", deviceID, requestID, map[string]any{})
	}
	return device, err
}

func (s *Service) DetachDevice(ctx context.Context, actor Actor, deviceID, projectID, requestID string) (domain.Device, error) {
	if actor.Role != "tenant_admin" && actor.Role != "project_manager" {
		return domain.Device{}, domain.Policy("ROLE_DENIED", "当前角色不能移除设备授权", "联系租户管理员")
	}
	if err := s.store.RevokeDeviceProject(ctx, actor.TenantID, projectID, deviceID, s.now().UTC()); err != nil {
		return domain.Device{}, err
	}
	device, err := s.store.Device(ctx, actor.TenantID, deviceID)
	if err == nil {
		device.ProjectIDs = slices.DeleteFunc(device.ProjectIDs, func(id string) bool { return id == projectID })
		s.audit(ctx, actor, projectID, "device.detached", "device", deviceID, requestID, map[string]any{})
	}
	return device, err
}

func (s *Service) RevokeDevice(ctx context.Context, actor Actor, id, requestID string) (domain.Device, error) {
	if actor.Role != "tenant_admin" && actor.Role != "project_manager" {
		return domain.Device{}, domain.Policy("ROLE_DENIED", "当前角色不能撤销设备", "联系租户管理员")
	}
	device, err := s.store.Device(ctx, actor.TenantID, id)
	if err != nil {
		return device, err
	}
	now := s.now().UTC()
	if err := s.store.RevokeDevice(ctx, actor.TenantID, id, now); err != nil {
		return device, err
	}
	device.RevokedAt = &now
	for _, projectID := range device.ProjectIDs {
		s.audit(ctx, actor, projectID, "device.revoked", "device", device.ID, requestID, map[string]any{})
	}
	return device, nil
}
