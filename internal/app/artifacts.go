package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const artifactDeviceOnlineWindow = 45 * time.Second

type RegisterArtifactInput struct {
	Envelope     domain.ExtensionArtifactEnvelopeV1 `json:"envelope"`
	FileName     string                             `json:"file_name"`
	Capabilities []domain.Capability                `json:"capabilities"`
	DryRun       bool                               `json:"dry_run"`
}

type RegisterArtifactResult struct {
	DryRun   bool            `json:"dry_run"`
	Artifact domain.Artifact `json:"artifact"`
}

type ArtifactOpenResult struct {
	DryRun       bool                        `json:"dry_run"`
	Request      *domain.ArtifactOpenRequest `json:"request,omitempty"`
	Presentation domain.ArtifactPresentation `json:"presentation"`
}

func (s *Service) RegisterArtifact(ctx context.Context, actor Actor, device domain.Device, input RegisterArtifactInput, requestID string) (RegisterArtifactResult, error) {
	if actor.Type != "device" || actor.DeviceID == "" || actor.DeviceID != device.ID {
		return RegisterArtifactResult{}, domain.E("authentication", "device", "DEVICE_TOKEN_INVALID", "设备凭据无效", 3)
	}
	if len(input.Capabilities) > 0 {
		device.Capabilities = input.Capabilities
		device.LastSeenAt = s.now().UTC()
		if err := s.store.SaveDevice(ctx, device); err != nil {
			return RegisterArtifactResult{}, err
		}
	}
	if err := domain.ValidateExtensionArtifactEnvelope(input.Envelope); err != nil {
		return RegisterArtifactResult{}, err
	}
	script, err := s.store.Script(ctx, actor.TenantID, input.Envelope.ScriptVersionID)
	if err != nil || script.ProjectID != input.Envelope.ProjectID {
		return RegisterArtifactResult{}, domain.NotFound("剧本版本")
	}
	if !slices.Contains(device.ProjectIDs, script.ProjectID) {
		return RegisterArtifactResult{}, domain.NotFound("项目设备授权")
	}
	if !deviceHasArtifactCapability(device, input.Envelope.Capability) {
		return RegisterArtifactResult{}, domain.Policy("ARTIFACT_CAPABILITY_MISMATCH", "设备当前 capability 与 Artifact Envelope 不匹配", "重新探测本机 capability 后再登记产物")
	}
	fileName := strings.TrimSpace(input.FileName)
	if fileName == "" || filepath.Base(filepath.Clean(fileName)) != fileName || len(fileName) > 255 || containsControlCharacter(fileName) {
		return RegisterArtifactResult{}, domain.Invalid("ARTIFACT_FILE_NAME_INVALID", "Artifact 文件名必须是不含路径的安全文件名")
	}
	now := s.now().UTC()
	artifact := domain.Artifact{
		ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: script.ProjectID, ScriptVersionID: script.ID,
		Kind: "extension", CapabilityID: input.Envelope.Capability.ID, CapabilityVersion: input.Envelope.Capability.Version,
		CapabilityDigest: input.Envelope.Capability.Digest, SchemaID: input.Envelope.SchemaID, MediaType: input.Envelope.MediaType,
		FileName: fileName, SHA256: input.Envelope.SHA256, ByteSize: input.Envelope.Size, Visibility: "internal",
		RetentionClass: "project", Purpose: "primary", SourceDeviceID: device.ID, ValidationStatus: "valid",
		Envelope: &input.Envelope, PresentationTier: "metadata_only", Metadata: copyArtifactMetadata(input.Envelope.Metadata), CreatedAt: now,
	}
	if input.DryRun {
		artifact.ID = ""
		return RegisterArtifactResult{DryRun: true, Artifact: artifact}, nil
	}
	existingArtifacts, err := s.store.Artifacts(ctx, actor.TenantID, script.ID)
	if err != nil {
		return RegisterArtifactResult{}, err
	}
	for _, existing := range existingArtifacts {
		if existing.Kind == "extension" && existing.SourceDeviceID == device.ID && existing.SHA256 == artifact.SHA256 && existing.SchemaID == artifact.SchemaID && existing.CapabilityDigest == artifact.CapabilityDigest {
			return RegisterArtifactResult{Artifact: existing}, nil
		}
	}
	if err := s.store.CreateArtifact(ctx, artifact); err != nil {
		return RegisterArtifactResult{}, err
	}
	s.audit(ctx, actor, script.ProjectID, "artifact.registered", "artifact", artifact.ID, requestID, map[string]any{"script_version_id": script.ID, "schema_id": artifact.SchemaID, "sha256": artifact.SHA256, "byte_size": artifact.ByteSize})
	return RegisterArtifactResult{Artifact: artifact}, nil
}

func (s *Service) ArtifactPresentations(ctx context.Context, actor Actor, scriptVersionID string) ([]domain.ArtifactPresentation, error) {
	if scriptVersionID != "" {
		if _, err := s.store.Script(ctx, actor.TenantID, scriptVersionID); err != nil {
			return nil, err
		}
	}
	artifacts, err := s.store.Artifacts(ctx, actor.TenantID, scriptVersionID)
	if err != nil {
		return nil, err
	}
	devices, err := s.store.Devices(ctx, actor.TenantID, "")
	if err != nil {
		return nil, err
	}
	values := make([]domain.ArtifactPresentation, 0, len(artifacts))
	for _, artifact := range artifacts {
		values = append(values, s.buildArtifactPresentation(artifact, artifacts, devices))
	}
	return values, nil
}

func (s *Service) ArtifactPresentation(ctx context.Context, actor Actor, artifactID string) (domain.ArtifactPresentation, error) {
	target, err := s.store.Artifact(ctx, actor.TenantID, artifactID)
	if err != nil {
		return domain.ArtifactPresentation{}, err
	}
	var artifacts []domain.Artifact
	if target.ApprovedSnapshotID != "" {
		artifacts, err = s.store.ArtifactsByApprovedSnapshot(ctx, actor.TenantID, target.ApprovedSnapshotID)
	} else {
		artifacts, err = s.store.Artifacts(ctx, actor.TenantID, target.ScriptVersionID)
	}
	if err != nil {
		return domain.ArtifactPresentation{}, err
	}
	devices, err := s.store.Devices(ctx, actor.TenantID, "")
	if err != nil {
		return domain.ArtifactPresentation{}, err
	}
	return s.buildArtifactPresentation(target, artifacts, devices), nil
}

func (s *Service) CreateArtifactOpenRequest(ctx context.Context, actor Actor, artifactID, deviceID string, dryRun bool, requestID string) (ArtifactOpenResult, error) {
	presentation, err := s.ArtifactPresentation(ctx, actor, artifactID)
	if err != nil {
		return ArtifactOpenResult{}, err
	}
	if presentation.Tier != "local_open" || presentation.SourceDevice == nil || presentation.SourceDevice.ID != deviceID || !slices.Contains(presentation.Actions, "local_open") {
		return ArtifactOpenResult{}, domain.Policy("ARTIFACT_LOCAL_OPEN_UNAVAILABLE", "Artifact 当前不能在指定设备打开", "确认来源设备在线、仍获项目授权且 capability 未变化")
	}
	if dryRun {
		return ArtifactOpenResult{DryRun: true, Presentation: presentation}, nil
	}
	now := s.now().UTC()
	if err := s.store.ExpireArtifactOpenRequests(ctx, actor.TenantID, now); err != nil {
		return ArtifactOpenResult{}, err
	}
	pending, err := s.store.PendingArtifactOpenRequests(ctx, actor.TenantID, deviceID, now, 20)
	if err != nil {
		return ArtifactOpenResult{}, err
	}
	for _, existing := range pending {
		if existing.ArtifactID == artifactID && existing.RequestedBy == actor.UserID {
			return ArtifactOpenResult{Request: &existing, Presentation: presentation}, nil
		}
	}
	openRequest := domain.ArtifactOpenRequest{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: presentation.Artifact.ProjectID, ArtifactID: artifactID, DeviceID: deviceID, RequestedBy: actor.UserID, State: "pending", ExpiresAt: now.Add(time.Minute), CreatedAt: now}
	if err := s.store.CreateArtifactOpenRequest(ctx, openRequest); err != nil {
		return ArtifactOpenResult{}, err
	}
	s.audit(ctx, actor, presentation.Artifact.ProjectID, "artifact.local_open_requested", "artifact_open_request", openRequest.ID, requestID, map[string]any{"artifact_id": artifactID, "device_id": deviceID, "expires_at": openRequest.ExpiresAt})
	return ArtifactOpenResult{Request: &openRequest, Presentation: presentation}, nil
}

func (s *Service) ArtifactOpenRequest(ctx context.Context, actor Actor, openRequestID string) (domain.ArtifactOpenRequest, error) {
	value, err := s.store.ArtifactOpenRequest(ctx, actor.TenantID, openRequestID)
	if err != nil {
		return value, err
	}
	if !s.now().UTC().Before(value.ExpiresAt) && (value.State == "pending" || value.State == "accepted") {
		now := s.now().UTC()
		value.State = "expired"
		value.CompletedAt = &now
		_ = s.store.SaveArtifactOpenRequest(ctx, value)
	}
	return value, nil
}

func (s *Service) PollArtifactOpen(ctx context.Context, actor Actor, device domain.Device, capabilities []domain.Capability) (domain.ArtifactOpenLease, error) {
	if actor.Type != "device" || actor.DeviceID != device.ID {
		return domain.ArtifactOpenLease{}, domain.E("authentication", "device", "DEVICE_TOKEN_INVALID", "设备凭据无效", 3)
	}
	now := s.now().UTC()
	device.LastSeenAt = now
	device.Capabilities = capabilities
	if err := s.store.SaveDevice(ctx, device); err != nil {
		return domain.ArtifactOpenLease{}, err
	}
	if err := s.store.ExpireArtifactOpenRequests(ctx, actor.TenantID, now); err != nil {
		return domain.ArtifactOpenLease{}, err
	}
	values, err := s.store.PendingArtifactOpenRequests(ctx, actor.TenantID, device.ID, now, 1)
	if err != nil {
		return domain.ArtifactOpenLease{}, err
	}
	if len(values) == 0 {
		return domain.ArtifactOpenLease{}, domain.NotFound("Artifact 打开请求")
	}
	return domain.ArtifactOpenLease{OpenRequestID: values[0].ID, ArtifactID: values[0].ArtifactID}, nil
}

func (s *Service) FinishArtifactOpen(ctx context.Context, actor Actor, device domain.Device, openRequestID, state, reason string) (domain.ArtifactOpenRequest, error) {
	if actor.Type != "device" || actor.DeviceID != device.ID {
		return domain.ArtifactOpenRequest{}, domain.E("authentication", "device", "DEVICE_TOKEN_INVALID", "设备凭据无效", 3)
	}
	value, err := s.store.ArtifactOpenRequest(ctx, actor.TenantID, openRequestID)
	if err != nil || value.DeviceID != device.ID {
		return domain.ArtifactOpenRequest{}, domain.NotFound("Artifact 打开请求")
	}
	if value.State == state && (state == "opened" || state == "not_available" || state == "failed") {
		return value, nil
	}
	now := s.now().UTC()
	if !now.Before(value.ExpiresAt) {
		value.State = "expired"
		value.CompletedAt = &now
		_ = s.store.SaveArtifactOpenRequest(ctx, value)
		return value, domain.Conflict("ARTIFACT_OPEN_EXPIRED", "Artifact 打开请求已过期")
	}
	if !validArtifactOpenTransition(value.State, state) {
		return value, domain.Conflict("ARTIFACT_OPEN_STATE_INVALID", "Artifact 打开状态转换无效")
	}
	if !validArtifactOpenReason(state, reason) {
		return value, domain.Invalid("ARTIFACT_OPEN_REASON_INVALID", "Artifact 打开失败原因必须使用稳定脱敏分类")
	}
	value.State = state
	value.Reason = reason
	if state == "accepted" {
		value.AcceptedAt = &now
	} else {
		if value.AcceptedAt == nil {
			value.AcceptedAt = &now
		}
		value.CompletedAt = &now
	}
	if err := s.store.SaveArtifactOpenRequest(ctx, value); err != nil {
		return value, err
	}
	return value, nil
}

func (s *Service) buildArtifactPresentation(target domain.Artifact, artifacts []domain.Artifact, devices []domain.Device) domain.ArtifactPresentation {
	value := domain.ArtifactPresentation{Artifact: target, Tier: "metadata_only", Renditions: []domain.Artifact{}, Actions: []string{}}
	for _, artifact := range artifacts {
		if artifact.DerivedFromArtifactID != target.ID || artifact.ValidationStatus != "valid" {
			continue
		}
		if artifact.SchemaID == domain.ReviewProjectionSchema && artifact.ObjectKey != "" && value.ReviewProjection == nil {
			projection := artifact
			value.ReviewProjection = &projection
		}
		if artifact.ObjectKey != "" && domain.SafeRenditionMediaType(artifact.MediaType) && slices.Contains([]string{"thumbnail", "preview", "poster", "transcript"}, artifact.Purpose) {
			value.Renditions = append(value.Renditions, artifact)
		}
	}
	var sourceDevice *domain.Device
	for index := range devices {
		if devices[index].ID == target.SourceDeviceID {
			sourceDevice = &devices[index]
			break
		}
	}
	if sourceDevice != nil {
		online := sourceDevice.RevokedAt == nil && s.now().UTC().Sub(sourceDevice.LastSeenAt) <= artifactDeviceOnlineWindow
		value.SourceDevice = &domain.ArtifactSourceDevice{ID: sourceDevice.ID, DisplayName: sourceDevice.DisplayName, Online: online}
	}
	switch {
	case target.ValidationStatus == "valid" && target.SchemaID == domain.ScriptPackageSchema:
		value.Tier = "cloud_native"
		value.Actions = append(value.Actions, "view_native")
	case target.ValidationStatus == "valid" && target.ObjectKey != "" && domain.SafeRenditionMediaType(target.MediaType):
		value.Tier = "safe_rendition"
		value.Actions = append(value.Actions, "preview")
	case len(value.Renditions) > 0:
		value.Tier = "safe_rendition"
		value.Actions = append(value.Actions, "preview")
	case sourceDevice != nil && value.SourceDevice.Online && slices.Contains(sourceDevice.ProjectIDs, target.ProjectID) && deviceHasArtifactCapability(*sourceDevice, domain.ArtifactCapabilityRef{ID: target.CapabilityID, Version: target.CapabilityVersion, Digest: target.CapabilityDigest}) && capabilityHasProfile(*sourceDevice, target.CapabilityID, "local_open"):
		value.Tier = "local_open"
		value.Actions = append(value.Actions, "local_open")
	}
	if target.ObjectKey != "" && target.ValidationStatus == "valid" {
		value.Actions = append(value.Actions, "download")
	}
	value.Artifact.PresentationTier = value.Tier
	return value
}

func (s *Service) ensureCoreScriptArtifact(ctx context.Context, script domain.ScriptVersion, device domain.Device, capability domain.Capability) {
	if _, err := s.store.Artifact(ctx, script.TenantID, script.ID); err == nil {
		return
	}
	body, _ := json.Marshal(script.Package)
	value := domain.Artifact{
		ID: script.ID, TenantID: script.TenantID, ProjectID: script.ProjectID, ScriptVersionID: script.ID, Kind: "core",
		CapabilityID: capability.ID, CapabilityVersion: capability.Version, CapabilityDigest: capability.Digest,
		SchemaID: domain.ScriptPackageSchema, MediaType: "application/json", FileName: fmt.Sprintf("script-package-v%d.json", script.Version),
		SHA256: script.ContentHash, ByteSize: int64(len(body)), Visibility: "client", RetentionClass: "audit", Purpose: "review",
		SourceDeviceID: device.ID, ValidationStatus: "valid", PresentationTier: "cloud_native", Metadata: map[string]any{"schema_version": script.Package.SchemaVersion}, CreatedAt: script.CreatedAt,
	}
	if err := s.store.CreateArtifact(ctx, value); err != nil {
		s.log.Warn("create core script artifact", "error", err, "script_version_id", script.ID)
	}
}

func deviceHasArtifactCapability(device domain.Device, expected domain.ArtifactCapabilityRef) bool {
	for _, capability := range device.Capabilities {
		if capability.ID == expected.ID && capability.Version == expected.Version && capability.Digest == expected.Digest {
			return true
		}
	}
	return false
}

func capabilityHasProfile(device domain.Device, capabilityID, profile string) bool {
	for _, capability := range device.Capabilities {
		if capability.ID == capabilityID && slices.Contains(capability.PresentationProfiles, profile) {
			return true
		}
	}
	return false
}

func validArtifactOpenTransition(current, next string) bool {
	if current == "pending" {
		return next == "accepted" || next == "opened" || next == "not_available" || next == "failed"
	}
	return current == "accepted" && (next == "opened" || next == "not_available" || next == "failed")
}

func validArtifactOpenReason(state, reason string) bool {
	if state == "accepted" || state == "opened" {
		return reason == ""
	}
	return slices.Contains([]string{"local_index_missing", "file_changed", "launcher_unavailable", "open_failed"}, reason)
}

func copyArtifactMetadata(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if character < 0x20 {
			return true
		}
	}
	return false
}
