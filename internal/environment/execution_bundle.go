package environment

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const executionBundleSignatureContext = "contentcloud.creative-execution-bundle.v1"

func BuildExecutionBundle(manifest Manifest, subject ExecutionSubject, capabilities []CapabilityRequirement, packIDs []string, issuedAt, expiresAt time.Time) (CreativeExecutionBundle, error) {
	if err := validateManifest(manifest, false); err != nil {
		return CreativeExecutionBundle{}, err
	}
	selected, err := selectBundlePacks(manifest, packIDs)
	if err != nil {
		return CreativeExecutionBundle{}, err
	}
	bundle := CreativeExecutionBundle{
		SchemaVersion: ExecutionBundleSchemaVersion, ProjectID: manifest.ProjectID, ProfileID: manifest.ProfileID,
		EnvironmentVersion: manifest.EnvironmentVersion, Subject: subject,
		RequiredCapabilities: append([]CapabilityRequirement(nil), capabilities...), Packs: selected,
		IssuedAt: issuedAt.UTC(), ExpiresAt: expiresAt.UTC(),
	}
	normalizeBundle(&bundle)
	if err := validateBundle(bundle, false); err != nil {
		return CreativeExecutionBundle{}, err
	}
	if err := validateBundleAgainstManifest(bundle, manifest); err != nil {
		return CreativeExecutionBundle{}, err
	}
	return bundle, nil
}

func (issuer *Issuer) SignBundle(bundle CreativeExecutionBundle) (CreativeExecutionBundle, error) {
	if issuer == nil {
		return CreativeExecutionBundle{}, domain.Invalid("EXECUTION_BUNDLE_SIGNER_REQUIRED", "创作执行包需要有效的签发器")
	}
	bundle.BundleID = ""
	bundle.Digest = ""
	bundle.Signature = Signature{}
	bundle.IssuedAt = bundle.IssuedAt.UTC()
	bundle.ExpiresAt = bundle.ExpiresAt.UTC()
	normalizeBundle(&bundle)
	if err := validateBundle(bundle, false); err != nil {
		return CreativeExecutionBundle{}, err
	}
	identity, err := executionBundleIdentityBytes(bundle)
	if err != nil {
		return CreativeExecutionBundle{}, err
	}
	identitySum := sha256.Sum256(identity)
	bundle.BundleID = "ceb_" + hex.EncodeToString(identitySum[:])
	payload, err := executionBundlePayloadBytes(bundle)
	if err != nil {
		return CreativeExecutionBundle{}, err
	}
	payloadSum := sha256.Sum256(payload)
	bundle.Digest = "sha256:" + hex.EncodeToString(payloadSum[:])
	signed, err := executionBundleSignedBytes(bundle.Digest)
	if err != nil {
		return CreativeExecutionBundle{}, err
	}
	bundle.Signature = Signature{Algorithm: "ed25519", KeyID: issuer.keyID, Value: base64.StdEncoding.EncodeToString(ed25519.Sign(issuer.privateKey, signed))}
	return bundle, nil
}

func (verifier *Verifier) VerifyBundle(bundle CreativeExecutionBundle, manifest Manifest, registry VerifiedRegistry, options BundleVerifyOptions) error {
	if verifier == nil {
		return domain.Invalid("EXECUTION_BUNDLE_VERIFIER_REQUIRED", "创作执行包需要可信的校验器")
	}
	if err := validateBundle(bundle, true); err != nil {
		return err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if bundle.IssuedAt.After(now.Add(5 * time.Minute)) {
		return domain.Conflict("EXECUTION_BUNDLE_NOT_YET_VALID", "创作执行包的签发时间晚于本机允许的时钟偏差")
	}
	if !now.Before(bundle.ExpiresAt) {
		return domain.Conflict("EXECUTION_BUNDLE_EXPIRED", "创作执行包已过期")
	}
	identity, err := executionBundleIdentityBytes(bundle)
	if err != nil {
		return err
	}
	identitySum := sha256.Sum256(identity)
	if expected := "ceb_" + hex.EncodeToString(identitySum[:]); bundle.BundleID != expected {
		return domain.Conflict("EXECUTION_BUNDLE_ID_MISMATCH", "创作执行包内容与 bundle_id 不一致")
	}
	payload, err := executionBundlePayloadBytes(bundle)
	if err != nil {
		return err
	}
	payloadSum := sha256.Sum256(payload)
	if expected := "sha256:" + hex.EncodeToString(payloadSum[:]); bundle.Digest != expected {
		return domain.Conflict("EXECUTION_BUNDLE_DIGEST_MISMATCH", "创作执行包内容与摘要不一致")
	}
	key, exists := verifier.keys[bundle.Signature.KeyID]
	if !exists || key.Status != "active" {
		return domain.Policy("EXECUTION_BUNDLE_SIGNING_KEY_UNTRUSTED", "创作执行包的签名密钥不受信任或已撤销", "刷新 Content Work OS 命令行工具或插件的可信公钥后重试")
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(bundle.Signature.Value)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return domain.Invalid("EXECUTION_BUNDLE_SIGNATURE_INVALID", "创作执行包的签名格式无效")
	}
	signed, err := executionBundleSignedBytes(bundle.Digest)
	if err != nil {
		return err
	}
	if !ed25519.Verify(key.PublicKey, signed, signature) {
		return domain.Conflict("EXECUTION_BUNDLE_SIGNATURE_MISMATCH", "创作执行包的签名验证失败")
	}
	projectID := options.ProjectID
	if projectID == "" {
		projectID = bundle.ProjectID
	}
	if err := verifier.Verify(manifest, VerifyOptions{ProjectID: projectID, ProfileID: bundle.ProfileID, Harness: manifest.Harness, Now: now}); err != nil {
		return err
	}
	if bundle.ProjectID != manifest.ProjectID || bundle.ProjectID != projectID {
		return domain.Conflict("EXECUTION_BUNDLE_PROJECT_MISMATCH", "创作执行包不属于当前项目")
	}
	if bundle.EnvironmentVersion != manifest.EnvironmentVersion {
		return domain.Conflict("EXECUTION_BUNDLE_ENVIRONMENT_MISMATCH", "创作执行包与当前环境版本不匹配")
	}
	if !emptyExecutionSubject(options.ExpectedSubject) && bundle.Subject != options.ExpectedSubject {
		return domain.Conflict("EXECUTION_BUNDLE_SUBJECT_MISMATCH", "创作执行包与已冻结的业务对象不匹配")
	}
	if err := validateBundleAgainstManifest(bundle, manifest); err != nil {
		return err
	}
	return validateBundleRegistry(bundle, manifest, registry)
}

func (resolver *Resolver) ResolveBundle(bundle CreativeExecutionBundle, manifest Manifest, registry VerifiedRegistry, lock EnvironmentLock, capabilities []domain.Capability, options BundleVerifyOptions) (BundleResolution, error) {
	if resolver == nil || resolver.verifier == nil {
		return BundleResolution{}, domain.Invalid("ENVIRONMENT_RESOLVER_REQUIRED", "创作执行包需要环境解析器")
	}
	if err := resolver.verifier.VerifyBundle(bundle, manifest, registry, options); err != nil {
		return BundleResolution{}, err
	}
	if err := ValidateLock(manifest, lock); err != nil {
		return BundleResolution{}, err
	}
	locked, err := lockedPlugins(lock.Plugins)
	if err != nil {
		return BundleResolution{}, err
	}
	resolution := BundleResolution{BundleID: bundle.BundleID, State: "ready", Packs: append([]PackRequirement(nil), bundle.Packs...)}
	manifestPlugins := make(map[string]PluginRef, len(manifest.Distribution.Plugins))
	for _, plugin := range manifest.Distribution.Plugins {
		manifestPlugins[plugin.ID] = plugin
	}
	for _, pack := range bundle.Packs {
		plugin := manifestPlugins[pack.ID]
		installed, exists := locked[pack.ID]
		reason := ""
		switch {
		case !exists || !installed.Installed:
			reason = "not_installed"
		case installed.Kind != pack.Kind:
			reason = "kind_mismatch"
		case installed.Version != pack.PluginVersion:
			reason = "version_mismatch"
		case installed.Digest != pack.Digest:
			reason = "digest_mismatch"
		}
		if reason != "" {
			resolution.PluginPreparation = append(resolution.PluginPreparation, PluginPreparation{Plugin: plugin, Reason: reason})
		}
	}
	for _, required := range bundle.RequiredCapabilities {
		resolution.CapabilityPreparation = appendCapabilityPreparation(resolution.CapabilityPreparation, required, capabilities)
	}
	if len(resolution.PluginPreparation) > 0 || len(resolution.CapabilityPreparation) > 0 {
		resolution.State = "environment_prepare"
	}
	return resolution, nil
}

func appendCapabilityPreparation(preparation []CapabilityPreparation, required CapabilityRequirement, capabilities []domain.Capability) []CapabilityPreparation {
	reason := "not_available"
	for _, capability := range capabilities {
		if capability.ID != required.ID {
			continue
		}
		switch {
		case capability.Kind != "business_capability" || !capability.LocalOnly:
			reason = "contract_mismatch"
		case capability.Version != required.SchemaVersion:
			reason = "version_mismatch"
		case capability.Digest != required.Digest:
			reason = "digest_mismatch"
		default:
			return preparation
		}
		break
	}
	return append(preparation, CapabilityPreparation{Capability: required, Reason: reason})
}

func selectBundlePacks(manifest Manifest, packIDs []string) ([]PackRequirement, error) {
	available := make(map[string]PluginRef, len(manifest.Distribution.Plugins))
	for _, plugin := range manifest.Distribution.Plugins {
		available[plugin.ID] = plugin
	}
	seen := make(map[string]struct{}, len(packIDs))
	selected := make([]PackRequirement, 0, len(packIDs))
	for _, id := range packIDs {
		if _, exists := seen[id]; exists {
			return nil, domain.Conflict("EXECUTION_BUNDLE_PACK_DUPLICATED", "创作执行包请求包含重复的能力包")
		}
		seen[id] = struct{}{}
		plugin, exists := available[id]
		if !exists || plugin.Scope != "task" || (plugin.Kind != "skill_pack" && plugin.Kind != "provider_mcp_pack") {
			return nil, domain.Policy("EXECUTION_BUNDLE_PACK_DENIED", "创作执行包只能选择环境清单允许列表中的任务级能力包", "通过 Content Work OS 创作环境配置精选能力包")
		}
		selected = append(selected, PackRequirement{ID: plugin.ID, Kind: plugin.Kind, PluginVersion: plugin.Version, Digest: plugin.Digest, Scope: "task", Required: true})
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	return selected, nil
}

func validateBundle(bundle CreativeExecutionBundle, requireSignature bool) error {
	if bundle.SchemaVersion != ExecutionBundleSchemaVersion || strings.TrimSpace(bundle.ProjectID) == "" || !dottedIDPattern.MatchString(bundle.ProfileID) || !versionPattern.MatchString(bundle.EnvironmentVersion) {
		return domain.Invalid("EXECUTION_BUNDLE_INVALID", "创作执行包缺少有效的项目、环境配置或环境版本")
	}
	if !dottedIDPattern.MatchString(bundle.Subject.Type) || strings.TrimSpace(bundle.Subject.ID) == "" || !digestPattern.MatchString(bundle.Subject.Digest) {
		return domain.Invalid("EXECUTION_BUNDLE_SUBJECT_INVALID", "创作执行包的业务对象缺少有效类型、ID 或摘要")
	}
	if bundle.IssuedAt.IsZero() || bundle.ExpiresAt.IsZero() || !bundle.ExpiresAt.After(bundle.IssuedAt) {
		return domain.Invalid("EXECUTION_BUNDLE_TIME_INVALID", "创作执行包的签发时间或过期时间无效")
	}
	if len(bundle.RequiredCapabilities) == 0 {
		return domain.Invalid("EXECUTION_BUNDLE_CAPABILITIES_REQUIRED", "创作执行包至少需要一项能力")
	}
	capabilities := map[string]struct{}{}
	for _, capability := range bundle.RequiredCapabilities {
		if !dottedIDPattern.MatchString(capability.ID) || !versionPattern.MatchString(capability.SchemaVersion) || !digestPattern.MatchString(capability.Digest) {
			return domain.Invalid("EXECUTION_BUNDLE_CAPABILITY_INVALID", "创作执行包中的能力引用无效")
		}
		if _, exists := capabilities[capability.ID]; exists {
			return domain.Conflict("EXECUTION_BUNDLE_CAPABILITY_DUPLICATED", "创作执行包包含重复能力")
		}
		capabilities[capability.ID] = struct{}{}
	}
	packs := map[string]struct{}{}
	for _, pack := range bundle.Packs {
		if !pluginIDPattern.MatchString(pack.ID) || (pack.Kind != "skill_pack" && pack.Kind != "provider_mcp_pack") || !versionPattern.MatchString(pack.PluginVersion) || !digestPattern.MatchString(pack.Digest) || pack.Scope != "task" || !pack.Required {
			return domain.Invalid("EXECUTION_BUNDLE_PACK_INVALID", "创作执行包中的能力包引用无效")
		}
		if _, exists := packs[pack.ID]; exists {
			return domain.Conflict("EXECUTION_BUNDLE_PACK_DUPLICATED", "创作执行包包含重复能力包")
		}
		packs[pack.ID] = struct{}{}
	}
	if requireSignature {
		if len(bundle.BundleID) != 68 || !strings.HasPrefix(bundle.BundleID, "ceb_") || !digestPattern.MatchString(bundle.Digest) || bundle.Signature.Algorithm != "ed25519" || !dottedIDPattern.MatchString(bundle.Signature.KeyID) || strings.TrimSpace(bundle.Signature.Value) == "" {
			return domain.Invalid("EXECUTION_BUNDLE_SIGNATURE_INVALID", "创作执行包缺少有效的 ID、摘要或 Ed25519 签名")
		}
	}
	return nil
}

func validateBundleAgainstManifest(bundle CreativeExecutionBundle, manifest Manifest) error {
	if bundle.ProjectID != manifest.ProjectID || bundle.ProfileID != manifest.ProfileID || bundle.EnvironmentVersion != manifest.EnvironmentVersion {
		return domain.Conflict("EXECUTION_BUNDLE_ENVIRONMENT_MISMATCH", "创作执行包与环境清单不匹配")
	}
	allowedCapabilities := stringSet(manifest.Capabilities)
	selectedPlugins := map[string]PluginRef{}
	for _, plugin := range manifest.Distribution.Plugins {
		if plugin.Required && plugin.Scope == "environment" {
			selectedPlugins[plugin.ID] = plugin
		}
	}
	for _, pack := range bundle.Packs {
		var matched *PluginRef
		for index := range manifest.Distribution.Plugins {
			plugin := manifest.Distribution.Plugins[index]
			if plugin.ID == pack.ID && plugin.Kind == pack.Kind && plugin.Version == pack.PluginVersion && plugin.Digest == pack.Digest && plugin.Scope == pack.Scope {
				copy := plugin
				matched = &copy
				break
			}
		}
		if matched == nil {
			return domain.Policy("EXECUTION_BUNDLE_PACK_DENIED", "创作执行包中的能力包不在当前环境清单允许列表中，或摘要不匹配", "刷新项目环境清单和创作执行包")
		}
		selectedPlugins[pack.ID] = *matched
	}
	provided := map[string]struct{}{}
	for _, plugin := range selectedPlugins {
		for _, capability := range plugin.Capabilities {
			provided[capability] = struct{}{}
		}
	}
	for _, capability := range bundle.RequiredCapabilities {
		if _, allowed := allowedCapabilities[capability.ID]; !allowed {
			return domain.Policy("EXECUTION_BUNDLE_CAPABILITY_DENIED", "创作执行包中的能力超出环境清单允许列表", "选择当前项目环境配置允许的能力")
		}
		if _, available := provided[capability.ID]; !available {
			return domain.Conflict("EXECUTION_BUNDLE_CAPABILITY_UNRESOLVED", "创作执行包中的能力没有对应的环境插件或任务能力包")
		}
	}
	return nil
}

func validateBundleRegistry(bundle CreativeExecutionBundle, manifest Manifest, verifiedRegistry VerifiedRegistry) error {
	registry := verifiedRegistry.raw()
	for _, pack := range bundle.Packs {
		entry, err := registry.Exact(pack.ID, pack.PluginVersion, pack.Digest)
		if err != nil {
			return err
		}
		if _, err := AssessRegistryEntry(entry, PurposeNewRun); err != nil {
			return err
		}
		if entry.Kind != pack.Kind || !contains(entry.CompatibleProfiles, manifest.ProfileID) {
			return domain.Policy("EXECUTION_BUNDLE_REGISTRY_MISMATCH", "创作执行包中的能力包与插件市场能力目录元数据不匹配", "刷新精选能力目录")
		}
	}
	return nil
}

func normalizeBundle(bundle *CreativeExecutionBundle) {
	sort.Slice(bundle.RequiredCapabilities, func(i, j int) bool { return bundle.RequiredCapabilities[i].ID < bundle.RequiredCapabilities[j].ID })
	sort.Slice(bundle.Packs, func(i, j int) bool { return bundle.Packs[i].ID < bundle.Packs[j].ID })
}

func executionBundleIdentityBytes(bundle CreativeExecutionBundle) ([]byte, error) {
	return json.Marshal(struct {
		SchemaVersion        string                  `json:"schema_version"`
		ProjectID            string                  `json:"project_id"`
		ProfileID            string                  `json:"profile_id"`
		EnvironmentVersion   string                  `json:"environment_version"`
		Subject              ExecutionSubject        `json:"subject"`
		RequiredCapabilities []CapabilityRequirement `json:"required_capabilities"`
		Packs                []PackRequirement       `json:"packs"`
		IssuedAt             time.Time               `json:"issued_at"`
		ExpiresAt            time.Time               `json:"expires_at"`
	}{bundle.SchemaVersion, bundle.ProjectID, bundle.ProfileID, bundle.EnvironmentVersion, bundle.Subject, bundle.RequiredCapabilities, bundle.Packs, bundle.IssuedAt.UTC(), bundle.ExpiresAt.UTC()})
}

func executionBundlePayloadBytes(bundle CreativeExecutionBundle) ([]byte, error) {
	identity, err := executionBundleIdentityBytes(bundle)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		BundleID string          `json:"bundle_id"`
		Payload  json.RawMessage `json:"payload"`
	}{bundle.BundleID, identity})
}

func executionBundleSignedBytes(digest string) ([]byte, error) {
	return json.Marshal(struct {
		Context string `json:"context"`
		Digest  string `json:"digest"`
	}{executionBundleSignatureContext, digest})
}

func emptyExecutionSubject(subject ExecutionSubject) bool {
	return subject.Type == "" && subject.ID == "" && subject.Digest == ""
}
