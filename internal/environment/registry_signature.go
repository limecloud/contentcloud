package environment

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"

	"github.com/limecloud/contentcloud/internal/domain"
)

const registrySignatureContext = "contentcloud.plugin-release-signature.v1"

type RegistryTrustedKey struct {
	KeyID     string
	Status    string
	PublicKey ed25519.PublicKey
}

type RegistryVerifier struct {
	keys map[string]RegistryTrustedKey
}

type VerifiedRegistry struct {
	registry Registry
}

func NewRegistryVerifier(keys []RegistryTrustedKey) (*RegistryVerifier, error) {
	trusted := make(map[string]RegistryTrustedKey, len(keys))
	for _, key := range keys {
		if !dottedIDPattern.MatchString(key.KeyID) || len(key.PublicKey) != ed25519.PublicKeySize || (key.Status != "active" && key.Status != "revoked") {
			return nil, domain.Invalid("REGISTRY_TRUST_KEY_INVALID", "插件市场能力目录的信任库包含无效的 Ed25519 公钥")
		}
		if _, exists := trusted[key.KeyID]; exists {
			return nil, domain.Conflict("REGISTRY_TRUST_KEY_DUPLICATED", "插件市场能力目录的信任库包含重复的 key_id")
		}
		key.PublicKey = append(ed25519.PublicKey(nil), key.PublicKey...)
		trusted[key.KeyID] = key
	}
	return &RegistryVerifier{keys: trusted}, nil
}

func (verifier *RegistryVerifier) Verify(registry Registry) (VerifiedRegistry, error) {
	if verifier == nil || registry.SchemaVersion != "1.0" || len(registry.Entries) == 0 {
		return VerifiedRegistry{}, domain.Invalid("REGISTRY_INVALID", "插件市场能力目录缺少校验器、schema_version 或条目列表")
	}
	seen := map[string]struct{}{}
	for _, entry := range registry.Entries {
		if err := validateRegistryEntryMetadata(entry); err != nil {
			return VerifiedRegistry{}, err
		}
		identity := entry.ID + "@" + entry.Version
		if _, exists := seen[identity]; exists {
			return VerifiedRegistry{}, domain.Conflict("REGISTRY_VERSION_AMBIGUOUS", "插件市场能力目录中，同一 ID 和版本对应多个条目")
		}
		seen[identity] = struct{}{}
		if err := verifier.VerifyEntry(entry); err != nil {
			return VerifiedRegistry{}, err
		}
	}
	copy, err := clone(registry)
	if err != nil {
		return VerifiedRegistry{}, err
	}
	return VerifiedRegistry{registry: copy}, nil
}

func (verifier *RegistryVerifier) VerifyEntry(entry RegistryEntry) error {
	if entry.Signature.Status != "verified" || entry.Signature.Algorithm != "ed25519" || !dottedIDPattern.MatchString(entry.Signature.KeyID) {
		return domain.Invalid("REGISTRY_SIGNATURE_INVALID", "插件市场能力目录条目缺少已验证的 Ed25519 签名")
	}
	key, exists := verifier.keys[entry.Signature.KeyID]
	if !exists || key.Status != "active" {
		return domain.Policy("REGISTRY_SIGNING_KEY_UNTRUSTED", "插件市场能力目录的签名密钥不受信任或已撤销", "刷新 Content Work OS 命令行工具或插件的可信公钥后重试")
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(entry.Signature.Value)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return domain.Invalid("REGISTRY_SIGNATURE_INVALID", "插件市场能力目录条目的签名格式无效")
	}
	payload, err := RegistryEntrySigningPayload(entry)
	if err != nil {
		return err
	}
	if !ed25519.Verify(key.PublicKey, payload, signature) {
		return domain.Conflict("REGISTRY_SIGNATURE_MISMATCH", "插件市场能力目录条目的签名验证失败")
	}
	return nil
}

func RegistryEntrySigningPayload(entry RegistryEntry) ([]byte, error) {
	payload := map[string]any{
		"context":             registrySignatureContext,
		"id":                  entry.ID,
		"kind":                entry.Kind,
		"version":             entry.Version,
		"source":              entry.Source,
		"license":             entry.License,
		"digest":              entry.Digest,
		"compatible_profiles": entry.CompatibleProfiles,
		"permissions":         entry.Permissions,
		"data_flow":           entry.DataFlow,
		"cost":                entry.Cost,
		"output_schemas":      entry.OutputSchemas,
		"evaluation":          entry.Evaluation,
		"lifecycle":           entry.Lifecycle,
		"revocation":          entry.Revocation,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var canonical any
	if err := json.Unmarshal(body, &canonical); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

func (registry VerifiedRegistry) raw() Registry {
	return registry.registry
}

func (registry VerifiedRegistry) Registry() Registry {
	copy, err := clone(registry.registry)
	if err != nil {
		return Registry{}
	}
	return copy
}
