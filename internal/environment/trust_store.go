package environment

import (
	"bytes"
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

//go:embed default-trusted-keys.json
var defaultTrustedKeysJSON []byte

//go:embed default-plugin-trusted-keys.json
var defaultPluginTrustedKeysJSON []byte

type EncodedTrustedKey struct {
	KeyID     string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	Status    string `json:"status"`
	PublicKey string `json:"public_key"`
}

type TrustedKeySet struct {
	SchemaURL     string              `json:"$schema"`
	SchemaVersion string              `json:"schema_version"`
	Keys          []EncodedTrustedKey `json:"keys"`
}

func DefaultManifestVerifier() (*Verifier, error) {
	return ManifestVerifierJSON(defaultTrustedKeysJSON)
}

func DefaultRegistryVerifier() (*RegistryVerifier, error) {
	return RegistryVerifierJSON(defaultPluginTrustedKeysJSON)
}

func LoadManifestVerifier(path string) (*Verifier, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Environment trust store: %w", err)
	}
	return ManifestVerifierJSON(body)
}

func LoadRegistryVerifier(path string) (*RegistryVerifier, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Plugin Registry trust store: %w", err)
	}
	return RegistryVerifierJSON(body)
}

func ManifestVerifierJSON(body []byte) (*Verifier, error) {
	keys, err := decodeTrustedKeySet(body)
	if err != nil {
		return nil, err
	}
	converted := make([]TrustedKey, 0, len(keys))
	for _, key := range keys {
		converted = append(converted, TrustedKey{KeyID: key.KeyID, Status: key.Status, PublicKey: key.PublicKey})
	}
	return NewVerifier(converted)
}

func RegistryVerifierJSON(body []byte) (*RegistryVerifier, error) {
	keys, err := decodeTrustedKeySet(body)
	if err != nil {
		return nil, err
	}
	converted := make([]RegistryTrustedKey, 0, len(keys))
	for _, key := range keys {
		converted = append(converted, RegistryTrustedKey{KeyID: key.KeyID, Status: key.Status, PublicKey: key.PublicKey})
	}
	return NewRegistryVerifier(converted)
}

func decodeTrustedKeySet(body []byte) ([]TrustedKey, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var document TrustedKeySet
	if err := decoder.Decode(&document); err != nil {
		return nil, domain.Invalid("TRUST_STORE_INVALID", "可信公钥文件不是严格的 JSON 文档")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if strings.TrimSpace(document.SchemaURL) == "" || document.SchemaVersion != "1.0" {
		return nil, domain.Invalid("TRUST_STORE_INVALID", "可信公钥文件缺少 $schema 或 schema_version 1.0")
	}
	decoded := make([]TrustedKey, 0, len(document.Keys))
	active := 0
	for _, key := range document.Keys {
		publicKey, err := base64.StdEncoding.Strict().DecodeString(key.PublicKey)
		if err != nil || len(publicKey) != ed25519.PublicKeySize || key.Algorithm != "ed25519" {
			return nil, domain.Invalid("TRUST_STORE_KEY_INVALID", "可信公钥文件包含无效 Ed25519 公钥")
		}
		decoded = append(decoded, TrustedKey{KeyID: key.KeyID, Status: key.Status, PublicKey: ed25519.PublicKey(publicKey)})
		if key.Status == "active" {
			active++
		}
	}
	if active == 0 {
		return nil, domain.Invalid("TRUST_STORE_ACTIVE_KEY_REQUIRED", "可信公钥文件至少需要一个 active Ed25519 公钥")
	}
	return decoded, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return domain.Invalid("TRUST_STORE_INVALID", "可信公钥文件只能包含一个 JSON 文档")
	}
	return nil
}
