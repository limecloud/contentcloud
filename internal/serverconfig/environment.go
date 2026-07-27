package serverconfig

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/capabilitycatalog"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
)

const defaultManifestTTL = 24 * time.Hour

var releaseVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)

type EnvironmentConfig struct {
	ProfilePath              string
	RegistryPath             string
	RegistryTrustPath        string
	EnvironmentTrustPath     string
	SigningKeyPath           string
	SigningKeyID             string
	CapabilityReleaseVersion string
	ManifestTTL              time.Duration
	RepositoryRoot           string
}

type EnvironmentRuntime struct {
	Enabled                bool
	ControlPlane           *environment.ControlPlane
	AutomationRequirements []environment.CapabilityRequirement
	AutomationPackIDs      map[string][]string
}

func EnvironmentFromEnv() (EnvironmentRuntime, error) {
	ttl := defaultManifestTTL
	if raw := strings.TrimSpace(os.Getenv("CONTENTCLOUD_ENVIRONMENT_MANIFEST_TTL")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return EnvironmentRuntime{}, fmt.Errorf("parse CONTENTCLOUD_ENVIRONMENT_MANIFEST_TTL: %w", err)
		}
		ttl = parsed
	}
	repositoryRoot, err := os.Getwd()
	if err != nil {
		return EnvironmentRuntime{}, fmt.Errorf("resolve server working directory: %w", err)
	}
	return LoadEnvironment(EnvironmentConfig{
		ProfilePath:              strings.TrimSpace(os.Getenv("CONTENTCLOUD_ENVIRONMENT_PROFILE_FILE")),
		RegistryPath:             strings.TrimSpace(os.Getenv("CONTENTCLOUD_PLUGIN_REGISTRY_FILE")),
		RegistryTrustPath:        strings.TrimSpace(os.Getenv("CONTENTCLOUD_PLUGIN_TRUST_FILE")),
		EnvironmentTrustPath:     strings.TrimSpace(os.Getenv("CONTENTCLOUD_ENVIRONMENT_TRUST_FILE")),
		SigningKeyPath:           strings.TrimSpace(os.Getenv("CONTENTCLOUD_ENVIRONMENT_SIGNING_KEY_FILE")),
		SigningKeyID:             strings.TrimSpace(os.Getenv("CONTENTCLOUD_ENVIRONMENT_SIGNING_KEY_ID")),
		CapabilityReleaseVersion: strings.TrimSpace(os.Getenv("CONTENTCLOUD_CAPABILITY_RELEASE_VERSION")),
		ManifestTTL:              ttl,
		RepositoryRoot:           repositoryRoot,
	})
}

func LoadEnvironment(config EnvironmentConfig) (EnvironmentRuntime, error) {
	values := []string{config.ProfilePath, config.RegistryPath, config.RegistryTrustPath, config.EnvironmentTrustPath, config.SigningKeyPath, config.SigningKeyID, config.CapabilityReleaseVersion}
	configured := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			configured++
		}
	}
	if configured == 0 {
		return EnvironmentRuntime{AutomationPackIDs: map[string][]string{}}, nil
	}
	if configured != len(values) {
		return EnvironmentRuntime{}, domain.Invalid("ENVIRONMENT_CONFIG_INCOMPLETE", "Environment Control Plane 配置必须同时提供 Profile、Registry、两类 trust store、signer 文件/key_id 和 capability release version")
	}
	if !releaseVersionPattern.MatchString(config.CapabilityReleaseVersion) {
		return EnvironmentRuntime{}, domain.Invalid("CAPABILITY_RELEASE_VERSION_INVALID", "capability release version 必须是不可变语义版本")
	}
	if config.ManifestTTL == 0 {
		config.ManifestTTL = defaultManifestTTL
	}

	var profile environment.Profile
	if err := readStrictJSONFile(config.ProfilePath, &profile); err != nil {
		return EnvironmentRuntime{}, fmt.Errorf("load Creative Environment Profile: %w", err)
	}
	var registry environment.Registry
	if err := readStrictJSONFile(config.RegistryPath, &registry); err != nil {
		return EnvironmentRuntime{}, fmt.Errorf("load Plugin Registry: %w", err)
	}
	registryVerifier, err := environment.LoadRegistryVerifier(config.RegistryTrustPath)
	if err != nil {
		return EnvironmentRuntime{}, err
	}
	verifiedRegistry, err := registryVerifier.Verify(registry)
	if err != nil {
		return EnvironmentRuntime{}, fmt.Errorf("verify Plugin Registry: %w", err)
	}
	manifestVerifier, err := environment.LoadManifestVerifier(config.EnvironmentTrustPath)
	if err != nil {
		return EnvironmentRuntime{}, err
	}
	privateKey, err := loadPrivateKey(config.SigningKeyPath, config.RepositoryRoot)
	if err != nil {
		return EnvironmentRuntime{}, err
	}
	issuer, err := environment.NewIssuer(config.SigningKeyID, privateKey)
	if err != nil {
		return EnvironmentRuntime{}, err
	}
	controlPlane, err := environment.NewControlPlaneWithVerifier(issuer, manifestVerifier, profile, verifiedRegistry, config.ManifestTTL)
	if err != nil {
		return EnvironmentRuntime{}, fmt.Errorf("initialize Environment Control Plane: %w", err)
	}

	requirements, packIDs, err := automationPolicy(profile, config.CapabilityReleaseVersion)
	if err != nil {
		return EnvironmentRuntime{}, err
	}
	if !profile.Policies.AutomationEnabled {
		requirements = nil
		packIDs = map[string][]string{}
	}
	return EnvironmentRuntime{Enabled: true, ControlPlane: controlPlane, AutomationRequirements: requirements, AutomationPackIDs: packIDs}, nil
}

func automationPolicy(profile environment.Profile, releaseVersion string) ([]environment.CapabilityRequirement, map[string][]string, error) {
	requirements := make([]environment.CapabilityRequirement, 0, len(profile.Capabilities))
	packIDs := make(map[string][]string, len(profile.Capabilities))
	for _, capabilityID := range profile.Capabilities {
		capability, exists := capabilitycatalog.Exact(capabilityID, releaseVersion)
		if !exists {
			return nil, nil, domain.Policy("AUTOMATION_CAPABILITY_UNSUPPORTED", "Creative Environment Profile 包含服务端 catalog 未实现的 capability", "使用当前 release 支持的确定性 capability")
		}
		requirements = append(requirements, environment.CapabilityRequirement{ID: capability.ID, SchemaVersion: capability.Version, Digest: capability.Digest})
	}
	for _, plugin := range profile.Plugins {
		if plugin.Scope != "task" {
			continue
		}
		for _, capabilityID := range plugin.Capabilities {
			packIDs[capabilityID] = append(packIDs[capabilityID], plugin.ID)
		}
	}
	sort.Slice(requirements, func(i, j int) bool { return requirements[i].ID < requirements[j].ID })
	for capabilityID := range packIDs {
		sort.Strings(packIDs[capabilityID])
	}
	return requirements, packIDs, nil
}

func loadPrivateKey(path, repositoryRoot string) (ed25519.PrivateKey, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect Environment signing key: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, domain.Invalid("ENVIRONMENT_SIGNING_KEY_FILE_INVALID", "Environment signing key 必须是普通文件，不能是符号链接")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, domain.Policy("ENVIRONMENT_SIGNING_KEY_PERMISSIONS", "Environment signing key 不能允许 group/other 访问", "将私钥权限收紧为 0400 或 0600")
	}
	if repositoryRoot != "" {
		inside, err := pathWithin(repositoryRoot, path)
		if err != nil {
			return nil, err
		}
		if inside {
			return nil, domain.Policy("ENVIRONMENT_SIGNING_KEY_IN_REPOSITORY", "Environment signing key 必须位于代码仓库之外", "将私钥移动到受限 secrets 目录")
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Environment signing key: %w", err)
	}
	if len(body) != ed25519.PrivateKeySize {
		return nil, domain.Invalid("ENVIRONMENT_SIGNING_KEY_FORMAT", "Environment signing key 必须是 64 字节 Ed25519 私钥二进制文件")
	}
	return append(ed25519.PrivateKey(nil), body...), nil
}

func pathWithin(root, target string) (bool, error) {
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return false, fmt.Errorf("resolve repository root: %w", err)
	}
	targetPath, err := filepath.Abs(target)
	if err != nil {
		return false, fmt.Errorf("resolve signing key path: %w", err)
	}
	relative, err := filepath.Rel(rootPath, targetPath)
	if err != nil {
		return false, fmt.Errorf("compare signing key path: %w", err)
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}

func readStrictJSONFile(path string, target any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON documents are not allowed")
		}
		return err
	}
	return nil
}
