package localworkspace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
)

const (
	environmentManifestFile = "environment.manifest.json"
	environmentLockFile     = "environment.lock"
	environmentRegistryFile = "environment.registry.json"
)

type EnvironmentState struct {
	Manifest environment.Manifest        `json:"manifest"`
	Lock     environment.EnvironmentLock `json:"lock"`
	Health   string                      `json:"health"`
}

// ReadEnvironmentClaim 只读取本地声明；服务端仍须在发放租约前验证签名和 Registry trust。
func ReadEnvironmentClaim(root string) (EnvironmentState, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return EnvironmentState{}, err
	}
	if err := ensureEnvironmentPreparationIdle(resolved, time.Now().UTC()); err != nil {
		return EnvironmentState{}, err
	}
	var manifest environment.Manifest
	if err := readStrictJSON(filepath.Join(resolved, ".contentcloud", environmentManifestFile), &manifest); err != nil {
		return EnvironmentState{}, err
	}
	var lock environment.EnvironmentLock
	if err := readStrictJSON(filepath.Join(resolved, ".contentcloud", environmentLockFile), &lock); err != nil {
		return EnvironmentState{}, err
	}
	binding, err := ProjectBinding(resolved)
	if err != nil {
		return EnvironmentState{}, err
	}
	if manifest.ProjectID != binding.ProjectID || lock.ProjectID != binding.ProjectID {
		return EnvironmentState{}, domain.Conflict("AUTOMATION_ENVIRONMENT_PROJECT_MISMATCH", "本地环境声明与工作区项目不一致")
	}
	if err := environment.ValidateLock(manifest, lock); err != nil {
		return EnvironmentState{}, err
	}
	return EnvironmentState{Manifest: manifest, Lock: lock, Health: "unverified_claim"}, nil
}

func StoreEnvironment(root string, manifest environment.Manifest, installed []environment.LockedPlugin, verifier *environment.Verifier, now time.Time) (EnvironmentState, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return EnvironmentState{}, err
	}
	binding, err := ProjectBinding(resolved)
	if err != nil {
		return EnvironmentState{}, err
	}
	if verifier == nil {
		return EnvironmentState{}, domain.Invalid("ENVIRONMENT_VERIFIER_REQUIRED", "保存环境清单前必须提供可信校验器")
	}
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := verifier.Verify(manifest, environment.VerifyOptions{ProjectID: binding.ProjectID, Harness: manifest.Harness, Now: now}); err != nil {
		return EnvironmentState{}, err
	}
	lock := environment.EnvironmentLock{
		SchemaVersion: "1.0", ProjectID: manifest.ProjectID, ProfileID: manifest.ProfileID, ProfileVersion: manifest.ProfileVersion,
		EnvironmentVersion: manifest.EnvironmentVersion, Harness: manifest.Harness, ManifestDigest: manifest.Digest,
		Plugins: append([]environment.LockedPlugin(nil), installed...), VerifiedAt: now,
	}
	if err := environment.ValidateLock(manifest, lock); err != nil {
		return EnvironmentState{}, err
	}
	directory := filepath.Join(resolved, ".contentcloud")
	if err := replaceJSON(filepath.Join(directory, environmentManifestFile), manifest, 0o400); err != nil {
		return EnvironmentState{}, err
	}
	if err := replaceJSON(filepath.Join(directory, environmentLockFile), lock, 0o600); err != nil {
		return EnvironmentState{}, err
	}
	return EnvironmentState{Manifest: manifest, Lock: lock, Health: "ready"}, nil
}

func StoreEnvironmentRegistry(root string, registry environment.Registry, verifier *environment.RegistryVerifier) (environment.VerifiedRegistry, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return environment.VerifiedRegistry{}, err
	}
	if verifier == nil {
		return environment.VerifiedRegistry{}, domain.Invalid("REGISTRY_VERIFIER_REQUIRED", "保存环境能力目录前必须提供可信校验器")
	}
	verified, err := verifier.Verify(registry)
	if err != nil {
		return environment.VerifiedRegistry{}, err
	}
	if err := replaceJSON(filepath.Join(resolved, ".contentcloud", environmentRegistryFile), registry, 0o400); err != nil {
		return environment.VerifiedRegistry{}, err
	}
	return verified, nil
}

func LoadEnvironmentRegistry(root string, verifier *environment.RegistryVerifier) (environment.VerifiedRegistry, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return environment.VerifiedRegistry{}, err
	}
	if verifier == nil {
		return environment.VerifiedRegistry{}, domain.Invalid("REGISTRY_VERIFIER_REQUIRED", "读取环境能力目录前必须提供可信校验器")
	}
	var registry environment.Registry
	if err := readStrictJSON(filepath.Join(resolved, ".contentcloud", environmentRegistryFile), &registry); err != nil {
		return environment.VerifiedRegistry{}, err
	}
	return verifier.Verify(registry)
}

func LoadEnvironment(root string, verifier *environment.Verifier, now time.Time) (EnvironmentState, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return EnvironmentState{}, err
	}
	if verifier == nil {
		return EnvironmentState{}, domain.Invalid("ENVIRONMENT_VERIFIER_REQUIRED", "读取环境清单前必须提供可信校验器")
	}
	var manifest environment.Manifest
	if err := readStrictJSON(filepath.Join(resolved, ".contentcloud", environmentManifestFile), &manifest); err != nil {
		return EnvironmentState{}, err
	}
	var lock environment.EnvironmentLock
	if err := readStrictJSON(filepath.Join(resolved, ".contentcloud", environmentLockFile), &lock); err != nil {
		return EnvironmentState{}, err
	}
	binding, err := ProjectBinding(resolved)
	if err != nil {
		return EnvironmentState{}, err
	}
	if err := verifier.Verify(manifest, environment.VerifyOptions{ProjectID: binding.ProjectID, Harness: manifest.Harness, Now: now}); err != nil {
		return EnvironmentState{}, err
	}
	if err := environment.ValidateLock(manifest, lock); err != nil {
		return EnvironmentState{}, err
	}
	return EnvironmentState{Manifest: manifest, Lock: lock, Health: "ready"}, nil
}

func RequireContentType(root, contentType string, verifier *environment.Verifier, now time.Time) (EnvironmentState, error) {
	if !domain.ValidTenantContentType(contentType) {
		return EnvironmentState{}, domain.Invalid("CONTENT_TYPE_INVALID", "请求的内容类型不受支持")
	}
	state, err := LoadEnvironment(root, verifier, now)
	if err != nil {
		return EnvironmentState{}, err
	}
	for _, enabled := range state.Manifest.ContentTypes {
		if enabled == contentType {
			return state, nil
		}
	}
	return EnvironmentState{}, domain.Policy("CONTENT_TYPE_NOT_ENABLED", "当前租户未开通内容类型 "+contentType, "由平台管理员在租户后台开通后，刷新工作区环境清单")
}

// CompareAndSwapEnvironmentLock commits one verified environment transition without overwriting concurrent state.
func CompareAndSwapEnvironmentLock(root string, manifest environment.Manifest, expected, next environment.EnvironmentLock) error {
	resolved, err := FindRoot(root)
	if err != nil {
		return err
	}
	if err := environment.ValidateLock(manifest, expected); err != nil {
		return err
	}
	if err := environment.ValidateLock(manifest, next); err != nil {
		return err
	}
	var current environment.EnvironmentLock
	if err := readStrictJSON(filepath.Join(resolved, ".contentcloud", environmentLockFile), &current); err != nil {
		return err
	}
	if !reflect.DeepEqual(current, expected) {
		return domain.Conflict("ENVIRONMENT_LOCK_CHANGED", "environment.lock 在准备期间发生变化，请重新生成计划")
	}
	return replaceJSON(filepath.Join(resolved, ".contentcloud", environmentLockFile), next, 0o600)
}

func EnvironmentCheck(root string, verifier *environment.Verifier, registryVerifier *environment.RegistryVerifier, now time.Time) Check {
	state, err := LoadEnvironment(root, verifier, now)
	if err == nil {
		var registry environment.VerifiedRegistry
		registry, err = LoadEnvironmentRegistry(root, registryVerifier)
		if err == nil {
			err = environment.ValidateManifestRegistry(state.Manifest, registry, environment.PurposeNewRun)
		}
	}
	if err == nil {
		return Check{OK: true, Required: true, Message: "签名 Manifest、Registry 与本地 environment.lock 一致"}
	}
	message := err.Error()
	if errors.Is(err, os.ErrNotExist) {
		message = "缺少 Environment Manifest 或 environment.lock"
	}
	return Check{OK: false, Required: true, Message: message}
}
