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
			return EnvironmentRuntime{}, fmt.Errorf("解析 CONTENTCLOUD_ENVIRONMENT_MANIFEST_TTL 失败：%w", err)
		}
		ttl = parsed
	}
	repositoryRoot, err := os.Getwd()
	if err != nil {
		return EnvironmentRuntime{}, fmt.Errorf("解析服务端工作目录失败：%w", err)
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
		return EnvironmentRuntime{}, domain.Invalid("ENVIRONMENT_CONFIG_INCOMPLETE", "环境控制平面配置必须同时提供环境配置、插件注册表、两类信任库、签名文件、key_id 和能力发布版本")
	}
	if !releaseVersionPattern.MatchString(config.CapabilityReleaseVersion) {
		return EnvironmentRuntime{}, domain.Invalid("CAPABILITY_RELEASE_VERSION_INVALID", "能力发布版本必须是不可变的语义化版本")
	}
	if config.ManifestTTL == 0 {
		config.ManifestTTL = defaultManifestTTL
	}

	var profile environment.Profile
	if err := readStrictJSONFile(config.ProfilePath, &profile); err != nil {
		return EnvironmentRuntime{}, fmt.Errorf("加载创作环境配置失败：%w", err)
	}
	var registry environment.Registry
	if err := readStrictJSONFile(config.RegistryPath, &registry); err != nil {
		return EnvironmentRuntime{}, fmt.Errorf("加载插件注册表失败：%w", err)
	}
	registryVerifier, err := environment.LoadRegistryVerifier(config.RegistryTrustPath)
	if err != nil {
		return EnvironmentRuntime{}, err
	}
	verifiedRegistry, err := registryVerifier.Verify(registry)
	if err != nil {
		return EnvironmentRuntime{}, fmt.Errorf("验证插件注册表失败：%w", err)
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
		return EnvironmentRuntime{}, fmt.Errorf("初始化环境控制平面失败：%w", err)
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
			return nil, nil, domain.Policy("AUTOMATION_CAPABILITY_UNSUPPORTED", "创作环境配置包含服务端能力目录尚未实现的能力", "使用当前发布版本支持的确定性能力")
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
		return nil, fmt.Errorf("检查环境签名私钥失败：%w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, domain.Invalid("ENVIRONMENT_SIGNING_KEY_FILE_INVALID", "环境签名私钥必须是普通文件，不能是符号链接")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, domain.Policy("ENVIRONMENT_SIGNING_KEY_PERMISSIONS", "环境签名私钥不能允许用户组或其他用户访问", "将私钥权限收紧为 0400 或 0600")
	}
	if repositoryRoot != "" {
		inside, err := pathWithin(repositoryRoot, path)
		if err != nil {
			return nil, err
		}
		if inside {
			return nil, domain.Policy("ENVIRONMENT_SIGNING_KEY_IN_REPOSITORY", "环境签名私钥必须位于代码仓库之外", "将私钥移动到访问受限的密钥目录")
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取环境签名私钥失败：%w", err)
	}
	if len(body) != ed25519.PrivateKeySize {
		return nil, domain.Invalid("ENVIRONMENT_SIGNING_KEY_FORMAT", "环境签名私钥必须是 64 字节的 Ed25519 私钥二进制文件")
	}
	return append(ed25519.PrivateKey(nil), body...), nil
}

func pathWithin(root, target string) (bool, error) {
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return false, fmt.Errorf("解析代码仓库根目录失败：%w", err)
	}
	targetPath, err := filepath.Abs(target)
	if err != nil {
		return false, fmt.Errorf("解析签名私钥路径失败：%w", err)
	}
	relative, err := filepath.Rel(rootPath, targetPath)
	if err != nil {
		return false, fmt.Errorf("比较签名私钥路径失败：%w", err)
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
			return fmt.Errorf("不允许在一个文件中包含多个 JSON 文档")
		}
		return err
	}
	return nil
}
