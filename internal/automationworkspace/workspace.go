package automationworkspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
)

const SchemaVersion = "1.0"

type Lease struct {
	SchemaVersion    string    `json:"schema_version"`
	AttemptID        string    `json:"attempt_id"`
	RunID            string    `json:"run_id"`
	ProjectID        string    `json:"project_id"`
	ContractDigest   string    `json:"contract_digest"`
	BundleID         string    `json:"bundle_id,omitempty"`
	BundleDigest     string    `json:"bundle_digest,omitempty"`
	CapabilityID     string    `json:"capability_id"`
	CapabilityDigest string    `json:"capability_digest"`
	StartedAt        time.Time `json:"started_at"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type Options struct {
	BaseDir       string
	ForbiddenRoot string
	AttemptID     string
	RunID         string
	ProjectID     string
	Contract      domain.TaskContract
	Bundle        *environment.CreativeExecutionBundle
	OutputSchema  []byte
	Skill         []byte
	Now           time.Time
	ExpiresAt     time.Time
}

type Workspace struct {
	Root  string
	Lease Lease
	base  string
}

func Begin(options Options) (*Workspace, error) {
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(options.AttemptID) == "" || strings.TrimSpace(options.RunID) == "" || strings.TrimSpace(options.ProjectID) == "" || !options.ExpiresAt.After(now) {
		return nil, domain.Invalid("AUTOMATION_WORKSPACE_LEASE_INVALID", "Automation workspace 需要有效 Attempt、Run、Project 和未过期租约")
	}
	if options.Contract.RunID != options.RunID || options.Contract.Project.ID != options.ProjectID || strings.TrimSpace(options.Contract.Capability.ID) == "" || strings.TrimSpace(options.Contract.Capability.Digest) == "" {
		return nil, domain.Conflict("AUTOMATION_WORKSPACE_CONTRACT_MISMATCH", "Task Contract 与 Automation lease 身份不一致")
	}
	if len(options.OutputSchema) == 0 || len(options.Skill) == 0 {
		return nil, domain.Invalid("AUTOMATION_WORKSPACE_RESOURCES_REQUIRED", "Automation workspace 缺少输出 Schema 或 Skill")
	}
	base, err := resolveBase(options.BaseDir)
	if err != nil {
		return nil, err
	}
	if err := rejectOverlap(base, options.ForbiddenRoot); err != nil {
		return nil, err
	}
	if err := ensurePrivateDirectory(base); err != nil {
		return nil, err
	}
	contractBody, err := json.MarshalIndent(options.Contract, "", "  ")
	if err != nil {
		return nil, err
	}
	contractBody = append(contractBody, '\n')
	contractSum := sha256.Sum256(contractBody)
	lease := Lease{
		SchemaVersion: SchemaVersion, AttemptID: options.AttemptID, RunID: options.RunID, ProjectID: options.ProjectID,
		ContractDigest: "sha256:" + hex.EncodeToString(contractSum[:]), CapabilityID: options.Contract.Capability.ID,
		CapabilityDigest: options.Contract.Capability.Digest, StartedAt: now, ExpiresAt: options.ExpiresAt.UTC(),
	}
	if options.Bundle != nil {
		if options.Bundle.ProjectID != options.ProjectID || strings.TrimSpace(options.Bundle.BundleID) == "" || strings.TrimSpace(options.Bundle.Digest) == "" {
			return nil, domain.Conflict("AUTOMATION_WORKSPACE_BUNDLE_MISMATCH", "CreativeExecutionBundle 与 Automation workspace 身份不一致")
		}
		lease.BundleID = options.Bundle.BundleID
		lease.BundleDigest = options.Bundle.Digest
	}

	root := filepath.Join(base, attemptDirectory(options.AttemptID))
	if err := createAttemptDirectory(root, lease, now); err != nil {
		return nil, err
	}
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			_ = os.RemoveAll(root)
		}
	}()
	leaseBody, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return nil, err
	}
	leaseBody = append(leaseBody, '\n')
	files := []struct {
		name string
		body []byte
		mode fs.FileMode
	}{
		{name: "lease.json", body: leaseBody, mode: 0o600},
		{name: "contract.json", body: contractBody, mode: 0o400},
		{name: "output.schema.json", body: options.OutputSchema, mode: 0o400},
		{name: "SKILL.md", body: options.Skill, mode: 0o400},
	}
	if options.Bundle != nil {
		bundleBody, marshalErr := json.MarshalIndent(options.Bundle, "", "  ")
		if marshalErr != nil {
			return nil, marshalErr
		}
		files = append(files, struct {
			name string
			body []byte
			mode fs.FileMode
		}{name: "execution-bundle.json", body: append(bundleBody, '\n'), mode: 0o400})
	}
	for _, file := range files {
		if err := writeExclusive(filepath.Join(root, file.name), file.body, file.mode); err != nil {
			return nil, err
		}
	}
	cleanupOnError = false
	return &Workspace{Root: root, Lease: lease, base: base}, nil
}

func (workspace *Workspace) Cleanup() error {
	if workspace == nil || strings.TrimSpace(workspace.Root) == "" || strings.TrimSpace(workspace.base) == "" || strings.TrimSpace(workspace.Lease.AttemptID) == "" {
		return domain.Invalid("AUTOMATION_WORKSPACE_INVALID", "Automation workspace cleanup 缺少受管目录身份")
	}
	expected := filepath.Join(workspace.base, attemptDirectory(workspace.Lease.AttemptID))
	if filepath.Clean(workspace.Root) != expected || filepath.Dir(expected) != filepath.Clean(workspace.base) {
		return domain.Policy("AUTOMATION_WORKSPACE_CLEANUP_DENIED", "拒绝清理非受管 Automation workspace", "只允许删除当前 Attempt 的内容寻址目录")
	}
	if err := os.RemoveAll(expected); err != nil {
		return err
	}
	return nil
}

func (workspace *Workspace) Renew(expiresAt time.Time) error {
	if workspace == nil || strings.TrimSpace(workspace.Root) == "" || strings.TrimSpace(workspace.base) == "" || strings.TrimSpace(workspace.Lease.AttemptID) == "" {
		return domain.Invalid("AUTOMATION_WORKSPACE_INVALID", "Automation workspace renew 缺少受管目录身份")
	}
	expected := filepath.Join(workspace.base, attemptDirectory(workspace.Lease.AttemptID))
	if filepath.Clean(workspace.Root) != expected || filepath.Dir(expected) != filepath.Clean(workspace.base) {
		return domain.Policy("AUTOMATION_WORKSPACE_RENEW_DENIED", "拒绝续租非受管 Automation workspace", "只允许续租当前 Attempt 的内容寻址目录")
	}
	expiresAt = expiresAt.UTC()
	leasePath := filepath.Join(expected, "lease.json")
	info, err := os.Lstat(leasePath)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return domain.Policy("AUTOMATION_WORKSPACE_LEASE_UNSAFE", "Automation workspace lease 必须是权限为 0600 的普通文件", "停止当前 Attempt 并重新创建隔离工作区")
	}
	body, err := os.ReadFile(leasePath)
	if err != nil {
		return err
	}
	var current Lease
	if json.Unmarshal(body, &current) != nil || !sameLeaseIdentity(current, workspace.Lease) || !current.ExpiresAt.Equal(workspace.Lease.ExpiresAt) {
		return domain.Conflict("AUTOMATION_WORKSPACE_LEASE_CHANGED", "Automation workspace lease 已被并发修改")
	}
	if !expiresAt.After(current.ExpiresAt) {
		return nil
	}
	current.ExpiresAt = expiresAt
	if err := writeLeaseAtomic(leasePath, current); err != nil {
		return err
	}
	workspace.Lease = current
	return nil
}

func resolveBase(explicit string) (string, error) {
	base := strings.TrimSpace(explicit)
	if base == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(cache, "contentcloud", "automation")
	}
	return filepath.Abs(base)
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return domain.Policy("AUTOMATION_WORKSPACE_ROOT_UNSAFE", "Automation workspace root 必须是非 symlink 的私有目录", "使用权限为 0700 的独立缓存目录")
	}
	return nil
}

func rejectOverlap(base, forbidden string) error {
	if strings.TrimSpace(forbidden) == "" {
		return nil
	}
	interactive, err := filepath.Abs(forbidden)
	if err != nil {
		return err
	}
	if within(base, interactive) || within(interactive, base) {
		return domain.Policy("AUTOMATION_WORKSPACE_OVERLAP", "Automation workspace 不能与交互式 ContentCloud Workspace 重叠", "使用独立的用户缓存目录执行后台 Attempt")
	}
	return nil
}

func within(path, root string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func attemptDirectory(attemptID string) string {
	sum := sha256.Sum256([]byte(attemptID))
	return "attempt-" + hex.EncodeToString(sum[:16])
}

func createAttemptDirectory(root string, wanted Lease, now time.Time) error {
	if err := os.Mkdir(root, 0o700); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return domain.Policy("AUTOMATION_WORKSPACE_ATTEMPT_UNSAFE", "Automation Attempt 路径不是受管目录", "检查隔离工作区后重试")
	}
	var existing Lease
	body, err := os.ReadFile(filepath.Join(root, "lease.json"))
	if err != nil || json.Unmarshal(body, &existing) != nil || existing.ExpiresAt.IsZero() || !sameLeaseIdentity(existing, wanted) {
		return domain.Conflict("AUTOMATION_WORKSPACE_LEASE_INVALID", "已存在的 Automation workspace lease 无法安全恢复")
	}
	if existing.ExpiresAt.After(now) {
		conflict := domain.Conflict("AUTOMATION_WORKSPACE_LEASE_ACTIVE", "同一 Automation Attempt 已在本机执行")
		conflict.Details = map[string]any{"attempt_id": existing.AttemptID, "expires_at": existing.ExpiresAt}
		return conflict
	}
	if filepath.Dir(root) == root {
		return fmt.Errorf("refusing to clean broad automation path %s", root)
	}
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	return os.Mkdir(root, 0o700)
}

func sameLeaseIdentity(existing, wanted Lease) bool {
	return existing.SchemaVersion == SchemaVersion &&
		existing.AttemptID == wanted.AttemptID &&
		existing.RunID == wanted.RunID &&
		existing.ProjectID == wanted.ProjectID &&
		existing.ContractDigest == wanted.ContractDigest &&
		existing.BundleID == wanted.BundleID &&
		existing.BundleDigest == wanted.BundleDigest &&
		existing.CapabilityID == wanted.CapabilityID &&
		existing.CapabilityDigest == wanted.CapabilityDigest
}

func writeLeaseAtomic(path string, lease Lease) (returnErr error) {
	body, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".lease-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if returnErr != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func writeExclusive(path string, body []byte, mode fs.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
