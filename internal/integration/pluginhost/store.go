package pluginhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/integration/plugin"
)

type Store struct {
	Root string
}

// DefaultStoreRoot returns the client-owned Plugin Host store location without
// creating directories or changing local state.
func DefaultStoreRoot() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("CONTENTCLOUD_PLUGIN_STORE")); configured != "" {
		return filepath.Abs(configured)
	}
	if configPath := os.Getenv("CONTENTCLOUD_CONFIG_PATH"); configPath != "" {
		return filepath.Join(filepath.Dir(configPath), "plugins"), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "contentcloud", "plugins"), nil
}

func NewStore(root string) (*Store, error) {
	root = filepath.Clean(root)
	if root == "." || root == string(filepath.Separator) || strings.TrimSpace(root) == "" {
		return nil, domain.Invalid("PLUGIN_HOST_STORE_INVALID", "插件宿主存储目录必须是明确的客户端目录")
	}
	for _, name := range []string{"packages", "receipts", "locks", "staging"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			return nil, err
		}
	}
	return &Store{Root: root}, nil
}

func (s *Store) PackagePath(ref ReleaseRef) string {
	return filepath.Join(s.Root, "packages", safeID(ref.PluginID), safeID(ref.Version), digestDirectory(ref.Digest))
}

func (s *Store) ReceiptPath(host HostID, pluginID string) string {
	return filepath.Join(s.Root, "receipts", safeID(string(host)), safeID(pluginID)+".json")
}

func (s *Store) DataPath(host HostID, ref ReleaseRef) string {
	return filepath.Join(s.Root, "data", safeID(string(host)), safeID(ref.PluginID), safeID(ref.Version), digestDirectory(ref.Digest))
}

func (s *Store) HostPath(host HostID) string {
	return filepath.Join(s.Root, "hosts", safeID(string(host)))
}

// ReceiptDigest hashes the actual on-disk installation receipts for a host.
// It intentionally reads raw bytes so malformed receipts also cause a
// session-generation change and are handled by the Plugin Host doctor.
func (s *Store) ReceiptDigest(host HostID) (string, error) {
	directory := filepath.Join(s.Root, "receipts", safeID(string(host)))
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		body, readErr := os.ReadFile(filepath.Join(directory, name))
		if readErr != nil {
			return "", readErr
		}
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(body)
		_, _ = hash.Write([]byte{0})
	}
	if len(names) == 0 {
		return "", nil
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Store) LoadReceipt(host HostID, pluginID string) (*Receipt, error) {
	body, err := os.ReadFile(s.ReceiptPath(host, pluginID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var receipt Receipt
	if err := json.Unmarshal(body, &receipt); err != nil {
		return nil, domain.Invalid("PLUGIN_HOST_RECEIPT_INVALID", "本地插件安装回执无法解析")
	}
	if receipt.SchemaVersion != SchemaVersion || receipt.HostID != host || receipt.Release.PluginID != pluginID {
		return nil, domain.Invalid("PLUGIN_HOST_RECEIPT_INVALID", "本地插件安装回执身份不一致")
	}
	return &receipt, nil
}

func (s *Store) SaveReceipt(receipt Receipt) error {
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(s.ReceiptPath(receipt.HostID, receipt.Release.PluginID), append(body, '\n'), 0o600)
}

func (s *Store) DeleteReceipt(host HostID, pluginID string) error {
	if err := os.Remove(s.ReceiptPath(host, pluginID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) Lock(host HostID) (func(), error) {
	path := filepath.Join(s.Root, "locks", safeID(string(host))+".lock")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, domain.Conflict("PLUGIN_HOST_CAS_CONFLICT", "同一宿主上的插件变更正在进行")
		}
		return nil, err
	}
	_, _ = file.WriteString(uuid.NewString())
	_ = file.Close()
	return func() { _ = os.Remove(path) }, nil
}

func (s *Store) Stage(pkg plugin.Package, installationID string) (string, error) {
	stage := filepath.Join(s.Root, "staging", safeID(installationID))
	if err := os.RemoveAll(stage); err != nil {
		return "", err
	}
	if err := copyTree(pkg.Root, stage); err != nil {
		_ = os.RemoveAll(stage)
		return "", domain.Invalid("PLUGIN_HOST_STAGE_FAILED", err.Error())
	}
	loaded, err := plugin.Load(stage)
	if err != nil || loaded.Digest != pkg.Digest {
		_ = os.RemoveAll(stage)
		return "", domain.Conflict("PLUGIN_HOST_STAGE_DIGEST_MISMATCH", "插件 staging 内容与计划摘要不一致")
	}
	return stage, nil
}

func (s *Store) CommitStage(stage string, ref ReleaseRef) (string, error) {
	destination := s.PackagePath(ref)
	if existing, err := os.Stat(destination); err == nil {
		if !existing.IsDir() {
			return "", domain.Conflict("PLUGIN_HOST_PACKAGE_PATH_INVALID", "已有插件包路径不是目录")
		}
		_ = os.RemoveAll(stage)
		return destination, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", err
	}
	if err := os.Rename(stage, destination); err != nil {
		return "", err
	}
	return destination, nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("plugin staging rejects non-regular file %s", relative)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	})
}

func atomicWrite(path string, body []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".contentcloud-write-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
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

func safeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty"
	}
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '-' || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func digestDirectory(digest string) string {
	return strings.TrimPrefix(digest, "sha256:")
}
