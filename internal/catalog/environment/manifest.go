package environment

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

const manifestSignatureContext = "contentcloud.creative-environment-manifest.v1"

var (
	pluginIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	dottedIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	versionPattern  = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)
	digestPattern   = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

type Issuer struct {
	keyID      string
	privateKey ed25519.PrivateKey
}

type TrustedKey struct {
	KeyID     string
	Status    string
	PublicKey ed25519.PublicKey
}

type Verifier struct {
	keys map[string]TrustedKey
}

type VerifyOptions struct {
	ProjectID string
	ProfileID string
	Harness   string
	Now       time.Time
}

func NewIssuer(keyID string, privateKey ed25519.PrivateKey) (*Issuer, error) {
	if !dottedIDPattern.MatchString(keyID) || len(privateKey) != ed25519.PrivateKeySize {
		return nil, fault.Invalid("ENVIRONMENT_SIGNING_KEY_INVALID", "环境清单需要有效的 key_id 和 Ed25519 私钥")
	}
	return &Issuer{keyID: keyID, privateKey: append(ed25519.PrivateKey(nil), privateKey...)}, nil
}

func NewVerifier(keys []TrustedKey) (*Verifier, error) {
	trusted := make(map[string]TrustedKey, len(keys))
	for _, key := range keys {
		if !dottedIDPattern.MatchString(key.KeyID) || len(key.PublicKey) != ed25519.PublicKeySize || (key.Status != "active" && key.Status != "revoked") {
			return nil, fault.Invalid("ENVIRONMENT_TRUST_KEY_INVALID", "环境清单信任库包含无效的 Ed25519 公钥")
		}
		if _, exists := trusted[key.KeyID]; exists {
			return nil, fault.Conflict("ENVIRONMENT_TRUST_KEY_DUPLICATED", "环境清单信任库包含重复的 key_id")
		}
		key.PublicKey = append(ed25519.PublicKey(nil), key.PublicKey...)
		trusted[key.KeyID] = key
	}
	return &Verifier{keys: trusted}, nil
}

func (issuer *Issuer) Sign(manifest Manifest) (Manifest, error) {
	manifest.IssuedAt = manifest.IssuedAt.UTC()
	manifest.ExpiresAt = manifest.ExpiresAt.UTC()
	manifest.Digest = ""
	manifest.Signature = Signature{}
	if err := validateManifest(manifest, false); err != nil {
		return Manifest{}, err
	}
	payload, err := manifestPayloadBytes(manifest)
	if err != nil {
		return Manifest{}, err
	}
	sum := sha256.Sum256(payload)
	manifest.Digest = "sha256:" + hex.EncodeToString(sum[:])
	signed, err := manifestSignedBytes(manifest.Digest)
	if err != nil {
		return Manifest{}, err
	}
	manifest.Signature = Signature{Algorithm: "ed25519", KeyID: issuer.keyID, Value: base64.StdEncoding.EncodeToString(ed25519.Sign(issuer.privateKey, signed))}
	return manifest, nil
}

func (verifier *Verifier) Verify(manifest Manifest, options VerifyOptions) error {
	if err := validateManifest(manifest, true); err != nil {
		return err
	}
	if options.ProjectID != "" && manifest.ProjectID != options.ProjectID {
		return fault.Conflict("ENVIRONMENT_PROJECT_MISMATCH", "环境清单不属于当前项目")
	}
	if options.ProfileID != "" && manifest.ProfileID != options.ProfileID {
		return fault.Conflict("ENVIRONMENT_PROFILE_MISMATCH", "环境清单的配置标识不匹配")
	}
	if options.Harness != "" && manifest.Harness != options.Harness {
		return fault.Conflict("ENVIRONMENT_HARNESS_MISMATCH", "环境清单的智能体宿主不匹配")
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if manifest.IssuedAt.After(now.Add(5 * time.Minute)) {
		return fault.Conflict("ENVIRONMENT_MANIFEST_NOT_YET_VALID", "环境清单的签发时间晚于本机允许的时钟偏差")
	}
	if !now.Before(manifest.ExpiresAt) {
		return fault.Conflict("ENVIRONMENT_MANIFEST_EXPIRED", "环境清单已过期")
	}
	payload, err := manifestPayloadBytes(manifest)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	actualDigest := "sha256:" + hex.EncodeToString(sum[:])
	if actualDigest != manifest.Digest {
		return fault.Conflict("ENVIRONMENT_MANIFEST_DIGEST_MISMATCH", "环境清单内容与摘要不一致")
	}
	key, exists := verifier.keys[manifest.Signature.KeyID]
	if !exists || key.Status != "active" {
		return fault.Policy("ENVIRONMENT_SIGNING_KEY_UNTRUSTED", "环境清单的签名密钥不受信任或已撤销", "刷新 Content Work OS 命令行工具或插件的可信公钥后重试")
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(manifest.Signature.Value)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fault.Invalid("ENVIRONMENT_SIGNATURE_INVALID", "环境清单的签名格式无效")
	}
	signed, err := manifestSignedBytes(manifest.Digest)
	if err != nil {
		return err
	}
	if !ed25519.Verify(key.PublicKey, signed, signature) {
		return fault.Conflict("ENVIRONMENT_SIGNATURE_MISMATCH", "环境清单的签名验证失败")
	}
	return nil
}

func validateManifest(manifest Manifest, requireSignature bool) error {
	if manifest.SchemaVersion != ManifestSchemaVersion || strings.TrimSpace(manifest.ProjectID) == "" || !dottedIDPattern.MatchString(manifest.ProfileID) || !versionPattern.MatchString(manifest.ProfileVersion) || !versionPattern.MatchString(manifest.EnvironmentVersion) {
		return fault.Invalid("ENVIRONMENT_MANIFEST_INVALID", "环境清单缺少有效的项目、环境配置或版本")
	}
	if _, err := agentadapter.RequireCapability(manifest.Harness, agentadapter.CapabilityCreativeEnvironment); err != nil || !pluginIDPattern.MatchString(manifest.Distribution.Marketplace) {
		return fault.Invalid("ENVIRONMENT_DISTRIBUTION_INVALID", "环境清单中的智能体宿主或插件市场无效")
	}
	if !dottedIDPattern.MatchString(manifest.WorkspaceTemplate.ID) || !versionPattern.MatchString(manifest.WorkspaceTemplate.Version) || !digestPattern.MatchString(manifest.WorkspaceTemplate.Digest) {
		return fault.Invalid("ENVIRONMENT_TEMPLATE_INVALID", "环境清单中的工作区模板引用无效")
	}
	if manifest.IssuedAt.IsZero() || manifest.ExpiresAt.IsZero() || !manifest.ExpiresAt.After(manifest.IssuedAt) {
		return fault.Invalid("ENVIRONMENT_MANIFEST_TIME_INVALID", "环境清单的签发时间或过期时间无效")
	}
	if err := validateUniqueStrings(manifest.Capabilities, dottedIDPattern, "ENVIRONMENT_CAPABILITIES_INVALID"); err != nil {
		return err
	}
	if err := validateContentTypes(manifest.ContentTypes); err != nil {
		return err
	}
	if len(manifest.Distribution.Plugins) == 0 {
		return fault.Invalid("ENVIRONMENT_PLUGINS_REQUIRED", "环境清单至少需要一个受控插件")
	}
	seen := map[string]struct{}{}
	for _, plugin := range manifest.Distribution.Plugins {
		if err := validatePluginRef(plugin); err != nil {
			return err
		}
		if _, exists := seen[plugin.ID]; exists {
			return fault.Conflict("ENVIRONMENT_PLUGIN_DUPLICATED", "环境清单包含重复插件")
		}
		seen[plugin.ID] = struct{}{}
	}
	if requireSignature {
		if !digestPattern.MatchString(manifest.Digest) || manifest.Signature.Algorithm != "ed25519" || !dottedIDPattern.MatchString(manifest.Signature.KeyID) || strings.TrimSpace(manifest.Signature.Value) == "" {
			return fault.Invalid("ENVIRONMENT_SIGNATURE_INVALID", "环境清单缺少有效摘要或 Ed25519 签名")
		}
	}
	return nil
}

func validatePluginRef(plugin PluginRef) error {
	if !pluginIDPattern.MatchString(plugin.ID) || !validPluginKind(plugin.Kind) || !versionPattern.MatchString(plugin.Version) || !digestPattern.MatchString(plugin.Digest) || (plugin.Scope != "environment" && plugin.Scope != "task") {
		return fault.Invalid("ENVIRONMENT_PLUGIN_INVALID", fmt.Sprintf("环境插件 %q 的引用无效", plugin.ID))
	}
	if err := validateUniqueStrings(plugin.Capabilities, dottedIDPattern, "ENVIRONMENT_PLUGIN_CAPABILITIES_INVALID"); err != nil {
		return err
	}
	return nil
}

func manifestPayloadBytes(manifest Manifest) ([]byte, error) {
	payload := struct {
		SchemaVersion      string               `json:"schema_version"`
		ProjectID          string               `json:"project_id"`
		ProfileID          string               `json:"profile_id"`
		ProfileVersion     string               `json:"profile_version"`
		EnvironmentVersion string               `json:"environment_version"`
		Harness            string               `json:"harness"`
		Distribution       Distribution         `json:"distribution"`
		WorkspaceTemplate  WorkspaceTemplateRef `json:"workspace_template"`
		Capabilities       []string             `json:"capabilities"`
		ContentTypes       []string             `json:"content_types"`
		Policies           Policies             `json:"policies"`
		IssuedAt           time.Time            `json:"issued_at"`
		ExpiresAt          time.Time            `json:"expires_at"`
	}{manifest.SchemaVersion, manifest.ProjectID, manifest.ProfileID, manifest.ProfileVersion, manifest.EnvironmentVersion, manifest.Harness, manifest.Distribution, manifest.WorkspaceTemplate, manifest.Capabilities, manifest.ContentTypes, manifest.Policies, manifest.IssuedAt.UTC(), manifest.ExpiresAt.UTC()}
	return json.Marshal(payload)
}

func validateContentTypes(values []string) error {
	if len(values) == 0 {
		return fault.Invalid("ENVIRONMENT_CONTENT_TYPES_INVALID", "环境清单必须声明租户可用的内容类型")
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !identitydomain.ValidTenantContentType(value) {
			return fault.Invalid("ENVIRONMENT_CONTENT_TYPES_INVALID", "环境清单包含不受支持的内容类型")
		}
		if _, exists := seen[value]; exists {
			return fault.Conflict("ENVIRONMENT_CONTENT_TYPES_INVALID", "环境清单包含重复的内容类型")
		}
		seen[value] = struct{}{}
	}
	if _, exists := seen[identitydomain.ContentTypeVideoScript]; !exists {
		return fault.Invalid("ENVIRONMENT_CONTENT_TYPES_INVALID", "环境清单缺少默认的短视频剧本内容类型（video_script）")
	}
	return nil
}

func manifestSignedBytes(digest string) ([]byte, error) {
	return json.Marshal(struct {
		Context string `json:"context"`
		Digest  string `json:"digest"`
	}{manifestSignatureContext, digest})
}

func validateUniqueStrings(values []string, pattern *regexp.Regexp, code string) error {
	if len(values) == 0 {
		return fault.Invalid(code, "受控列表不能为空")
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !pattern.MatchString(value) {
			return fault.Invalid(code, "受控列表包含无效标识")
		}
		if _, exists := seen[value]; exists {
			return fault.Conflict(code, "受控列表包含重复标识")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validPluginKind(kind string) bool {
	return kind == "scene_plugin" || kind == "skill_pack" || kind == "provider_mcp_pack"
}
