package environment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
)

func BuildManifest(projectID string, profile Profile, verifiedRegistry VerifiedRegistry, issuedAt, expiresAt time.Time) (Manifest, error) {
	registry := verifiedRegistry.raw()
	_, harnessErr := agentadapter.RequireCapability(profile.Harness, agentadapter.CapabilityCreativeEnvironment)
	if strings.TrimSpace(projectID) == "" || !dottedIDPattern.MatchString(profile.ID) || !versionPattern.MatchString(profile.Version) || !versionPattern.MatchString(profile.EnvironmentVersion) || harnessErr != nil || !pluginIDPattern.MatchString(profile.Marketplace) {
		return Manifest{}, domain.Invalid("ENVIRONMENT_PROFILE_INVALID", "Creative Environment Profile 缺少有效项目、版本、Harness 或 Marketplace")
	}
	if issuedAt.IsZero() || expiresAt.IsZero() || !expiresAt.After(issuedAt) {
		return Manifest{}, domain.Invalid("ENVIRONMENT_PROFILE_TIME_INVALID", "Environment Manifest 有效期无效")
	}
	if err := validateUniqueStrings(profile.Capabilities, dottedIDPattern, "ENVIRONMENT_PROFILE_CAPABILITIES_INVALID"); err != nil {
		return Manifest{}, err
	}
	plugins := make([]PluginRef, 0, len(profile.Plugins))
	providedCapabilities := map[string]struct{}{}
	requiredScene := false
	seenPlugins := map[string]struct{}{}
	for _, allowed := range profile.Plugins {
		if _, exists := seenPlugins[allowed.ID]; exists {
			return Manifest{}, domain.Conflict("ENVIRONMENT_PROFILE_PLUGIN_DUPLICATED", "Creative Environment Profile 包含重复 Plugin")
		}
		seenPlugins[allowed.ID] = struct{}{}
		if !pluginIDPattern.MatchString(allowed.ID) || !validPluginKind(allowed.Kind) || !versionPattern.MatchString(allowed.Version) || (allowed.Scope != "environment" && allowed.Scope != "task") {
			return Manifest{}, domain.Invalid("ENVIRONMENT_PROFILE_PLUGIN_INVALID", "Creative Environment Profile Plugin allowlist 无效")
		}
		if err := validateUniqueStrings(allowed.Capabilities, dottedIDPattern, "ENVIRONMENT_PROFILE_PLUGIN_CAPABILITIES_INVALID"); err != nil {
			return Manifest{}, err
		}
		entry, err := exactRegistryVersion(registry, allowed.ID, allowed.Version)
		if err != nil {
			return Manifest{}, err
		}
		if _, err := AssessRegistryEntry(entry, PurposeNewInstall); err != nil {
			return Manifest{}, err
		}
		if entry.Kind != allowed.Kind || !contains(entry.CompatibleProfiles, profile.ID) {
			return Manifest{}, domain.Policy("ENVIRONMENT_PROFILE_REGISTRY_MISMATCH", "Profile allowlist 与 Marketplace Registry kind/compatibility 不匹配", "修正 Profile 或发布兼容的 Registry entry")
		}
		plugins = append(plugins, PluginRef{ID: entry.ID, Kind: entry.Kind, Version: entry.Version, SourceRef: entry.Source.Ref, Digest: entry.Digest, Required: allowed.Required, Scope: allowed.Scope, Capabilities: sortedCopy(allowed.Capabilities)})
		for _, capability := range allowed.Capabilities {
			providedCapabilities[capability] = struct{}{}
		}
		if allowed.Kind == "scene_plugin" && allowed.Required && allowed.Scope == "environment" {
			requiredScene = true
		}
	}
	if !requiredScene {
		return Manifest{}, domain.Policy("ENVIRONMENT_SCENE_PLUGIN_REQUIRED", "Creative Environment Profile 必须包含一个环境级必装 Scene Plugin", "配置已发布的 ContentCloud Scene Plugin")
	}
	for _, capability := range profile.Capabilities {
		if _, provided := providedCapabilities[capability]; !provided {
			return Manifest{}, domain.Conflict("ENVIRONMENT_CAPABILITY_UNRESOLVED", "Profile capability 没有对应的 allowlisted Plugin")
		}
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].ID < plugins[j].ID })
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion, ProjectID: projectID, ProfileID: profile.ID, ProfileVersion: profile.Version,
		EnvironmentVersion: profile.EnvironmentVersion, Harness: profile.Harness,
		Distribution: Distribution{Marketplace: profile.Marketplace, Plugins: plugins}, WorkspaceTemplate: profile.WorkspaceTemplate,
		Capabilities: sortedCopy(profile.Capabilities), Policies: profile.Policies, IssuedAt: issuedAt.UTC(), ExpiresAt: expiresAt.UTC(),
	}
	if err := validateManifest(manifest, false); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

type Resolver struct {
	verifier *Verifier
}

func NewResolver(verifier *Verifier) (*Resolver, error) {
	if verifier == nil {
		return nil, domain.Invalid("ENVIRONMENT_VERIFIER_REQUIRED", "Environment Resolver 需要可信 Manifest verifier")
	}
	return &Resolver{verifier: verifier}, nil
}

func (resolver *Resolver) ResolveLocal(manifest Manifest, verifiedRegistry VerifiedRegistry, lock EnvironmentLock, request LocalPlanRequest, now time.Time) (LocalExecutionPlan, error) {
	registry := verifiedRegistry.raw()
	if strings.TrimSpace(request.ProjectID) == "" || strings.TrimSpace(request.RunID) == "" || strings.TrimSpace(request.Intent) == "" {
		return LocalExecutionPlan{}, domain.Invalid("LOCAL_EXECUTION_REQUEST_INVALID", "LocalExecutionPlan 需要 project_id、run_id 和 intent")
	}
	if err := resolver.verifier.Verify(manifest, VerifyOptions{ProjectID: request.ProjectID, Harness: manifest.Harness, Now: now}); err != nil {
		return LocalExecutionPlan{}, err
	}
	if err := validateUniqueStrings(request.RequiredCapabilities, dottedIDPattern, "LOCAL_EXECUTION_CAPABILITIES_INVALID"); err != nil {
		return LocalExecutionPlan{}, err
	}
	if err := validateInputRefs(request.InputRefs); err != nil {
		return LocalExecutionPlan{}, err
	}
	allowedCapabilities := stringSet(manifest.Capabilities)
	for _, capability := range request.RequiredCapabilities {
		if _, allowed := allowedCapabilities[capability]; !allowed {
			return LocalExecutionPlan{}, domain.Policy("LOCAL_EXECUTION_CAPABILITY_DENIED", "LocalExecutionPlan 请求超出 Environment Manifest allowlist", "选择当前项目 Profile 允许的 capability")
		}
	}
	if err := validateLockIdentity(lock, manifest); err != nil {
		return LocalExecutionPlan{}, err
	}
	locked, err := lockedPlugins(lock.Plugins)
	if err != nil {
		return LocalExecutionPlan{}, err
	}
	requestedCapabilities := stringSet(request.RequiredCapabilities)
	selected := make([]PluginRef, 0, len(manifest.Distribution.Plugins))
	preparation := []PluginPreparation{}
	provided := map[string]struct{}{}
	for _, plugin := range manifest.Distribution.Plugins {
		selectPlugin := plugin.Required && plugin.Scope == "environment"
		for _, capability := range plugin.Capabilities {
			if _, requested := requestedCapabilities[capability]; requested {
				selectPlugin = true
				provided[capability] = struct{}{}
			}
		}
		if !selectPlugin {
			continue
		}
		entry, err := registry.Exact(plugin.ID, plugin.Version, plugin.Digest)
		if err != nil {
			return LocalExecutionPlan{}, err
		}
		if _, err := AssessRegistryEntry(entry, PurposeNewRun); err != nil {
			return LocalExecutionPlan{}, err
		}
		selected = append(selected, plugin)
		installed, exists := locked[plugin.ID]
		reason := ""
		switch {
		case !exists || !installed.Installed:
			reason = "not_installed"
		case installed.Kind != plugin.Kind:
			reason = "kind_mismatch"
		case installed.Version != plugin.Version:
			reason = "version_mismatch"
		case installed.Digest != plugin.Digest:
			reason = "digest_mismatch"
		}
		if reason != "" {
			preparation = append(preparation, PluginPreparation{Plugin: plugin, Reason: reason})
		}
	}
	for capability := range requestedCapabilities {
		if _, ok := provided[capability]; !ok {
			return LocalExecutionPlan{}, domain.Conflict("LOCAL_EXECUTION_CAPABILITY_UNRESOLVED", "Manifest allowlist 中没有 Plugin 提供请求的 capability")
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	sort.Slice(preparation, func(i, j int) bool { return preparation[i].Plugin.ID < preparation[j].Plugin.ID })
	state := "ready"
	if len(preparation) > 0 {
		state = "environment_prepare"
	}
	plan := LocalExecutionPlan{SchemaVersion: LocalPlanSchemaVersion, RunID: request.RunID, Intent: request.Intent, RequiredCapabilities: sortedCopy(request.RequiredCapabilities), Plugins: selected, Preparation: preparation, InputRefs: sortedCopy(request.InputRefs), EnvironmentDigest: manifest.Digest, State: state, RequiresServer: false}
	body, err := json.Marshal(plan)
	if err != nil {
		return LocalExecutionPlan{}, err
	}
	sum := sha256.Sum256(body)
	plan.PlanID = "lep_" + hex.EncodeToString(sum[:])
	return plan, nil
}

func exactRegistryVersion(registry Registry, id, version string) (RegistryEntry, error) {
	var matches []RegistryEntry
	for _, entry := range registry.Entries {
		if entry.ID == id && entry.Version == version {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 0 {
		return RegistryEntry{}, domain.NotFound("Profile allowlist 对应的 Marketplace Registry entry")
	}
	if len(matches) > 1 {
		return RegistryEntry{}, domain.Conflict("REGISTRY_VERSION_AMBIGUOUS", "Marketplace Registry 同一 ID/version 存在多个 entry")
	}
	return matches[0], nil
}

func validateLockIdentity(lock EnvironmentLock, manifest Manifest) error {
	if lock.SchemaVersion == "" && len(lock.Plugins) == 0 {
		return nil
	}
	if lock.ProjectID != manifest.ProjectID || lock.ProfileID != manifest.ProfileID || lock.ProfileVersion != manifest.ProfileVersion || lock.EnvironmentVersion != manifest.EnvironmentVersion || lock.Harness != manifest.Harness || lock.ManifestDigest != manifest.Digest {
		return domain.Conflict("ENVIRONMENT_LOCK_MISMATCH", "本地 environment.lock 与已验证 Manifest 不一致")
	}
	return nil
}

func ValidateLock(manifest Manifest, lock EnvironmentLock) error {
	if lock.SchemaVersion != "1.0" || lock.VerifiedAt.IsZero() {
		return domain.Invalid("ENVIRONMENT_LOCK_INVALID", "environment.lock 缺少有效 schema_version 或 verified_at")
	}
	if err := validateLockIdentity(lock, manifest); err != nil {
		return err
	}
	locked, err := lockedPlugins(lock.Plugins)
	if err != nil {
		return err
	}
	allowed := make(map[string]PluginRef, len(manifest.Distribution.Plugins))
	for _, plugin := range manifest.Distribution.Plugins {
		allowed[plugin.ID] = plugin
		if plugin.Required && plugin.Scope == "environment" {
			installed, exists := locked[plugin.ID]
			if !exists || !installed.Installed || installed.Kind != plugin.Kind || installed.Version != plugin.Version || installed.Digest != plugin.Digest {
				return domain.Conflict("ENVIRONMENT_REQUIRED_PLUGIN_MISSING", "environment.lock 未证明必装环境 Plugin 的精确版本和 digest")
			}
		}
	}
	for id, installed := range locked {
		plugin, exists := allowed[id]
		if !exists || installed.Kind != plugin.Kind || installed.Version != plugin.Version || installed.Digest != plugin.Digest {
			return domain.Policy("ENVIRONMENT_LOCK_PLUGIN_DENIED", "environment.lock 包含 Manifest allowlist 之外或版本不匹配的 Plugin", "重新执行受控环境准备")
		}
	}
	return nil
}

func ValidateManifestRegistry(manifest Manifest, verifiedRegistry VerifiedRegistry, purpose RegistryPurpose) error {
	registry := verifiedRegistry.raw()
	for _, plugin := range manifest.Distribution.Plugins {
		entry, err := registry.Exact(plugin.ID, plugin.Version, plugin.Digest)
		if err != nil {
			return err
		}
		if _, err := AssessRegistryEntry(entry, purpose); err != nil {
			return err
		}
		if entry.Kind != plugin.Kind || entry.Source.Ref != plugin.SourceRef || !contains(entry.CompatibleProfiles, manifest.ProfileID) {
			return domain.Conflict("ENVIRONMENT_MANIFEST_REGISTRY_MISMATCH", "Environment Manifest 与签名 Marketplace Registry 不一致")
		}
	}
	return nil
}

func lockedPlugins(plugins []LockedPlugin) (map[string]LockedPlugin, error) {
	result := make(map[string]LockedPlugin, len(plugins))
	for _, plugin := range plugins {
		if !pluginIDPattern.MatchString(plugin.ID) || !validPluginKind(plugin.Kind) || !versionPattern.MatchString(plugin.Version) || !digestPattern.MatchString(plugin.Digest) {
			return nil, domain.Invalid("ENVIRONMENT_LOCK_PLUGIN_INVALID", "environment.lock 包含无效 Plugin")
		}
		if _, exists := result[plugin.ID]; exists {
			return nil, domain.Conflict("ENVIRONMENT_LOCK_PLUGIN_DUPLICATED", "environment.lock 包含重复 Plugin")
		}
		result[plugin.ID] = plugin
	}
	return result, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortedCopy(values []string) []string {
	copy := append([]string(nil), values...)
	sort.Strings(copy)
	return copy
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validateInputRefs(values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return domain.Invalid("LOCAL_EXECUTION_INPUT_REF_INVALID", "LocalExecutionPlan input_refs 包含空值")
		}
		if _, exists := seen[value]; exists {
			return domain.Conflict("LOCAL_EXECUTION_INPUT_REF_DUPLICATED", "LocalExecutionPlan input_refs 包含重复值")
		}
		seen[value] = struct{}{}
	}
	return nil
}
