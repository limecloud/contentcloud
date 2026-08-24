package application

import (
	"context"
	"sort"
	"strings"
	"time"

	workbenchdomain "github.com/limecloud/contentcloud/internal/experience/workbench"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

// WorkbenchRegistryView is the platform control-plane projection. It exposes
// only declarative customer-surface versions and their lifecycle assignment.
type WorkbenchRegistryView struct {
	Entries     []workbenchdomain.Entry `json:"entries"`
	GeneratedAt time.Time               `json:"generated_at"`
}

type RegisterWorkbenchInput struct {
	Manifest        workbenchdomain.Manifest `json:"manifest"`
	Status          string                   `json:"status,omitempty"`
	TemplateAliases []string                 `json:"template_aliases,omitempty"`
	TenantIDs       []string                 `json:"tenant_ids,omitempty"`
}

type UpdateWorkbenchStateInput struct {
	Status    string   `json:"status"`
	TenantIDs []string `json:"tenant_ids"`
	Reason    string   `json:"reason,omitempty"`
}

func (s *serviceCore) effectiveWorkbenchRegistry(ctx context.Context) (*workbenchdomain.Registry, error) {
	base := s.workbenchRegistry
	if base == nil {
		base = workbenchdomain.DefaultRegistry()
	}
	byKey := make(map[string]workbenchdomain.Entry)
	for _, entry := range base.Entries() {
		byKey[entry.String()] = entry
	}
	if s.workbenchRepository != nil {
		persisted, err := s.workbenchRepository.WorkbenchEntries(ctx)
		if err != nil {
			return nil, err
		}
		for _, entry := range persisted {
			byKey[entry.String()] = entry
		}
	}
	entries := make([]workbenchdomain.Entry, 0, len(byKey))
	for _, entry := range byKey {
		entries = append(entries, entry)
	}
	return workbenchdomain.NewRegistry(entries)
}

func (s *OperationsService) WorkbenchRegistry(ctx context.Context, actor Actor) (WorkbenchRegistryView, error) {
	if !actor.PlatformAdmin {
		return WorkbenchRegistryView{}, fault.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以管理业务工作台版本", "使用已授权的平台管理员账号")
	}
	registry, err := s.effectiveWorkbenchRegistry(ctx)
	if err != nil {
		return WorkbenchRegistryView{}, err
	}
	return WorkbenchRegistryView{Entries: registry.Entries(), GeneratedAt: s.now().UTC()}, nil
}

func (s *OperationsService) RegisterWorkbench(ctx context.Context, actor Actor, input RegisterWorkbenchInput, requestID string) (workbenchdomain.Entry, error) {
	if !actor.PlatformAdmin {
		return workbenchdomain.Entry{}, fault.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以登记业务工作台版本", "使用已授权的平台管理员账号")
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = "draft"
	}
	if status != "draft" {
		return workbenchdomain.Entry{}, fault.Invalid("WORKBENCH_REGISTRATION_MUST_BE_DRAFT", "业务工作台必须先登记为草稿，再通过独立发布门禁启用")
	}
	entry := workbenchdomain.Entry{Manifest: input.Manifest, Status: status, TemplateAliases: append([]string(nil), input.TemplateAliases...), TenantIDs: append([]string(nil), input.TenantIDs...)}
	registry, err := workbenchdomain.NewRegistry([]workbenchdomain.Entry{entry})
	if err != nil {
		return workbenchdomain.Entry{}, err
	}
	entry = registry.Entries()[0]
	effective, err := s.effectiveWorkbenchRegistry(ctx)
	if err != nil {
		return workbenchdomain.Entry{}, err
	}
	for _, existing := range effective.Entries() {
		if existing.String() == entry.String() {
			return workbenchdomain.Entry{}, fault.Conflict("WORKBENCH_PLUGIN_DUPLICATED", "业务工作台插件的 ID 和版本已存在，必须登记新的不可变版本")
		}
	}
	if s.workbenchRepository == nil {
		return workbenchdomain.Entry{}, fault.Policy("WORKBENCH_REGISTRY_STORE_UNAVAILABLE", "业务工作台 Registry 持久层未配置", "检查服务端持久层配置")
	}
	if err := s.workbenchRepository.CreateWorkbenchEntry(ctx, entry); err != nil {
		return workbenchdomain.Entry{}, err
	}
	s.audit(ctx, actor, "", "platform.workbench_registered", "workbench_plugin", entry.String(), requestID, map[string]any{"status": entry.Status, "digest": entry.Digest})
	return entry, nil
}

func (s *OperationsService) UpdateWorkbenchState(ctx context.Context, actor Actor, id, version string, input UpdateWorkbenchStateInput, requestID string) (workbenchdomain.Entry, error) {
	if !actor.PlatformAdmin {
		return workbenchdomain.Entry{}, fault.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以修改业务工作台版本状态", "使用已授权的平台管理员账号")
	}
	if s.workbenchRepository == nil {
		return workbenchdomain.Entry{}, fault.Policy("WORKBENCH_REGISTRY_STORE_UNAVAILABLE", "业务工作台 Registry 持久层未配置", "检查服务端持久层配置")
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	tenantIDs := uniqueWorkbenchTenantIDs(input.TenantIDs)
	current, err := s.workbenchRepository.WorkbenchEntry(ctx, id, version)
	if err != nil {
		return workbenchdomain.Entry{}, err
	}
	if err := validateWorkbenchStateTransition(current, status, tenantIDs, input.Reason); err != nil {
		return workbenchdomain.Entry{}, err
	}
	if status == "published" {
		if err := s.validateWorkbenchPublication(ctx, current, tenantIDs); err != nil {
			return workbenchdomain.Entry{}, err
		}
	}
	entry, err := s.workbenchRepository.UpdateWorkbenchEntryState(ctx, id, version, current.Status, status, tenantIDs, input.Reason)
	if err != nil {
		return workbenchdomain.Entry{}, err
	}
	s.audit(ctx, actor, "", "platform.workbench_state_changed", "workbench_plugin", entry.String(), requestID, map[string]any{"previous_status": current.Status, "status": entry.Status, "tenant_ids": entry.TenantIDs, "reason": strings.TrimSpace(input.Reason)})
	return entry, nil
}

func validateWorkbenchStateTransition(current workbenchdomain.Entry, target string, tenantIDs []string, reason string) error {
	allowed := map[string]map[string]struct{}{
		"draft":     {"draft": {}, "published": {}, "revoked": {}},
		"published": {"published": {}, "retired": {}, "revoked": {}},
		"retired":   {"retired": {}, "published": {}, "revoked": {}},
		"revoked":   {"revoked": {}},
	}
	if _, ok := allowed[current.Status][target]; !ok {
		return fault.Conflict("WORKBENCH_PLUGIN_STATE_TRANSITION_INVALID", "业务工作台版本不能从当前状态切换到目标状态")
	}
	if current.Status == "revoked" && (!workbenchdomain.SameTenantScope(current.TenantIDs, tenantIDs) || current.LifecycleReason != strings.TrimSpace(reason)) {
		return fault.Conflict("WORKBENCH_PLUGIN_REVOKED_IMMUTABLE", "已安全撤销的业务工作台版本不能再修改租户范围或撤销原因")
	}
	if target == "revoked" && strings.TrimSpace(reason) == "" {
		return fault.Invalid("WORKBENCH_PLUGIN_REVOCATION_REASON_REQUIRED", "安全撤销业务工作台版本时必须填写原因")
	}
	return nil
}

func (s *OperationsService) validateWorkbenchPublication(ctx context.Context, candidate workbenchdomain.Entry, tenantIDs []string) error {
	templateID := strings.TrimSpace(candidate.Manifest.Experience.TemplateID)
	if _, approved := s.approvedWorkbenchTemplates[templateID]; !approved {
		return fault.Policy("WORKBENCH_TEMPLATE_NOT_APPROVED", "业务工作台引用的体验模板尚未登记为平台批准版本", "先发布对应的体验模板，再发布业务工作台")
	}
	entries, err := s.workbenchRepository.WorkbenchEntries(ctx)
	if err != nil {
		return err
	}
	if s.workbenchRegistry != nil {
		entries = append(entries, s.workbenchRegistry.Entries()...)
	}
	candidate.TenantIDs = append([]string(nil), tenantIDs...)
	for _, existing := range entries {
		if existing.String() == candidate.String() || existing.Status != "published" {
			continue
		}
		if !workbenchdomain.ScopesConflict(existing.TenantIDs, candidate.TenantIDs) {
			continue
		}
		if workbenchdomain.SharesContentType(existing.Manifest.ContentTypes, candidate.Manifest.ContentTypes) {
			return fault.Conflict("WORKBENCH_CONTENT_TYPE_DEFAULT_EXISTS", "同一发布范围内已经存在该内容类型的已发布工作台；请停用旧版本或改用独立租户范围")
		}
	}
	return nil
}

func uniqueWorkbenchTenantIDs(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
