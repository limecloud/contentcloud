package localconfig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	ServerURL      string          `json:"server_url,omitempty"`
	DaemonBindings []DaemonBinding `json:"daemon_bindings,omitempty"`
}

type DaemonBinding struct {
	ServerURL  string            `json:"server_url"`
	DeviceID   string            `json:"device_id"`
	Workspaces []DaemonWorkspace `json:"workspaces,omitempty"`
}

type DaemonWorkspace struct {
	WorkspaceID string `json:"workspace_id"`
	ProjectID   string `json:"project_id"`
	Root        string `json:"root"`
}

func Path() (string, error) {
	if custom := os.Getenv("CONTENTCLOUD_CONFIG_PATH"); custom != "" {
		return custom, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "contentcloud", "config.json"), nil
}
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("解析配置失败：%w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("解析配置失败：存在多余内容")
	}
	c.ServerURL = strings.TrimSpace(c.ServerURL)
	c.DaemonBindings = normalizeDaemonBindings(c.DaemonBindings)
	if c.ServerURL == "" {
		if binding, ok := c.PrimaryBinding(); ok {
			c.ServerURL = binding.ServerURL
		}
	}
	return c, nil
}

func Save(c Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return savePath(path, c)
}

func savePath(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Bindings returns a normalized copy of the current multi-workspace bindings.
func (c Config) Bindings() []DaemonBinding {
	return normalizeDaemonBindings(cloneDaemonBindings(c.DaemonBindings))
}

func (c Config) PrimaryBinding() (DaemonBinding, bool) {
	bindings := c.Bindings()
	if len(bindings) == 0 {
		return DaemonBinding{}, false
	}
	return bindings[0], true
}

func (c Config) PrimaryWorkspace() (DaemonBinding, DaemonWorkspace, bool) {
	binding, ok := c.PrimaryBinding()
	if !ok || len(binding.Workspaces) == 0 {
		return DaemonBinding{}, DaemonWorkspace{}, false
	}
	return binding, binding.Workspaces[0], true
}

func cloneDaemonBindings(bindings []DaemonBinding) []DaemonBinding {
	cloned := make([]DaemonBinding, len(bindings))
	for index, binding := range bindings {
		cloned[index] = binding
		cloned[index].Workspaces = append([]DaemonWorkspace(nil), binding.Workspaces...)
	}
	return cloned
}

func (c *Config) UpsertDaemonBinding(binding DaemonBinding) {
	if c == nil {
		return
	}
	c.DaemonBindings = normalizeDaemonBindings(upsertDaemonBinding(c.DaemonBindings, binding))
}

func (c *Config) RemoveDaemonBinding(deviceID string) {
	if c == nil {
		return
	}
	deviceID = strings.TrimSpace(deviceID)
	bindings := make([]DaemonBinding, 0, len(c.DaemonBindings))
	for _, binding := range c.DaemonBindings {
		if strings.TrimSpace(binding.DeviceID) != deviceID {
			bindings = append(bindings, binding)
		}
	}
	c.DaemonBindings = normalizeDaemonBindings(bindings)
}

func upsertDaemonBinding(bindings []DaemonBinding, incoming DaemonBinding) []DaemonBinding {
	incoming.ServerURL = strings.TrimRight(strings.TrimSpace(incoming.ServerURL), "/")
	incoming.DeviceID = strings.TrimSpace(incoming.DeviceID)
	if incoming.DeviceID == "" {
		return bindings
	}
	for index := range bindings {
		if strings.TrimSpace(bindings[index].DeviceID) != incoming.DeviceID || strings.TrimRight(strings.TrimSpace(bindings[index].ServerURL), "/") != incoming.ServerURL {
			continue
		}
		for _, workspace := range incoming.Workspaces {
			bindings[index].Workspaces = upsertDaemonWorkspace(bindings[index].Workspaces, workspace)
		}
		return bindings
	}
	copyBinding := incoming
	copyBinding.Workspaces = append([]DaemonWorkspace(nil), incoming.Workspaces...)
	return append(bindings, copyBinding)
}

func upsertDaemonWorkspace(workspaces []DaemonWorkspace, incoming DaemonWorkspace) []DaemonWorkspace {
	incoming.WorkspaceID = strings.TrimSpace(incoming.WorkspaceID)
	incoming.ProjectID = strings.TrimSpace(incoming.ProjectID)
	incoming.Root = strings.TrimSpace(incoming.Root)
	if incoming.WorkspaceID == "" && incoming.ProjectID == "" && incoming.Root == "" {
		return workspaces
	}
	for index := range workspaces {
		if (incoming.WorkspaceID != "" && strings.TrimSpace(workspaces[index].WorkspaceID) == incoming.WorkspaceID) ||
			(incoming.WorkspaceID == "" && incoming.ProjectID != "" && strings.TrimSpace(workspaces[index].ProjectID) == incoming.ProjectID) {
			workspaces[index] = incoming
			return workspaces
		}
	}
	return append(workspaces, incoming)
}

func normalizeDaemonBindings(bindings []DaemonBinding) []DaemonBinding {
	result := make([]DaemonBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding.ServerURL = strings.TrimRight(strings.TrimSpace(binding.ServerURL), "/")
		binding.DeviceID = strings.TrimSpace(binding.DeviceID)
		if binding.DeviceID == "" {
			continue
		}
		workspaces := make([]DaemonWorkspace, 0, len(binding.Workspaces))
		for _, workspace := range binding.Workspaces {
			workspaces = upsertDaemonWorkspace(workspaces, workspace)
		}
		binding.Workspaces = workspaces
		result = upsertDaemonBinding(result, binding)
	}
	return result
}

func SaveDeviceToken(deviceID, token string) error {
	if env := os.Getenv("CONTENTCLOUD_CREDENTIAL_FILE"); env != "" {
		return fmt.Errorf("拒绝使用明文凭据文件 %s", env)
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("%s 尚未实现安全凭据存储；拒绝降级为明文存储", runtime.GOOS)
	}
	cmd := exec.Command("security", "add-generic-password", "-U", "-a", deviceID, "-s", "contentcloud-device", "-w", token)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("将令牌保存到 macOS 钥匙串失败：%w：%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
func DeviceToken(deviceID string) (string, error) {
	if token := os.Getenv("CONTENTCLOUD_DEVICE_TOKEN"); token != "" {
		return token, nil
	}
	if deviceID == "" {
		return "", fmt.Errorf("尚未配置设备")
	}
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("%s 不支持安全凭据存储", runtime.GOOS)
	}
	out, err := exec.Command("security", "find-generic-password", "-w", "-a", deviceID, "-s", "contentcloud-device").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("从 macOS 钥匙串读取令牌失败：%w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func DeleteDeviceToken(deviceID string) error {
	if deviceID == "" {
		return nil
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("%s 不支持安全凭据存储", runtime.GOOS)
	}
	out, err := exec.Command("security", "delete-generic-password", "-a", deviceID, "-s", "contentcloud-device").CombinedOutput()
	if err != nil && !strings.Contains(string(out), "could not be found") {
		return fmt.Errorf("从 macOS 钥匙串删除设备令牌失败：%w", err)
	}
	return nil
}

func SaveWorkspaceToken(workspaceID, token string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("%s 尚未实现安全凭据存储；拒绝降级为明文存储", runtime.GOOS)
	}
	out, err := exec.Command("security", "add-generic-password", "-U", "-a", workspaceID, "-s", "contentcloud-workspace", "-w", token).CombinedOutput()
	if err != nil {
		return fmt.Errorf("将工作区令牌保存到 macOS 钥匙串失败：%w：%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func WorkspaceToken(workspaceID string) (string, error) {
	if token := os.Getenv("CONTENTCLOUD_WORKSPACE_TOKEN"); token != "" {
		return token, nil
	}
	if workspaceID == "" {
		return "", fmt.Errorf("尚未配置工作区")
	}
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("%s 不支持安全凭据存储", runtime.GOOS)
	}
	out, err := exec.Command("security", "find-generic-password", "-w", "-a", workspaceID, "-s", "contentcloud-workspace").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("从 macOS 钥匙串读取工作区令牌失败：%w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func SaveUserToken(serverURL, token string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("%s 尚未实现安全凭据存储；拒绝降级为明文存储", runtime.GOOS)
	}
	account := credentialAccount(serverURL)
	out, err := exec.Command("security", "add-generic-password", "-U", "-a", account, "-s", "contentcloud-user", "-w", token).CombinedOutput()
	if err != nil {
		return fmt.Errorf("将用户令牌保存到 macOS 钥匙串失败：%w：%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func UserToken(serverURL string) (string, error) {
	if token := os.Getenv("CONTENTCLOUD_TOKEN"); token != "" {
		return token, nil
	}
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("%s 不支持安全凭据存储", runtime.GOOS)
	}
	out, err := exec.Command("security", "find-generic-password", "-w", "-a", credentialAccount(serverURL), "-s", "contentcloud-user").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("从 macOS 钥匙串读取用户令牌失败：%w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func DeleteUserToken(serverURL string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("%s 不支持安全凭据存储", runtime.GOOS)
	}
	out, err := exec.Command("security", "delete-generic-password", "-a", credentialAccount(serverURL), "-s", "contentcloud-user").CombinedOutput()
	if err != nil && !strings.Contains(string(out), "could not be found") {
		return fmt.Errorf("从 macOS 钥匙串删除用户令牌失败：%w", err)
	}
	return nil
}

func credentialAccount(serverURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(serverURL, "/")))
	return "server-" + hex.EncodeToString(sum[:8])
}

func ResolveProject(explicit string, c Config) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if env := os.Getenv("CONTENTCLOUD_PROJECT_ID"); env != "" {
		return env, nil
	}
	cwd, err := os.Getwd()
	if err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			path := filepath.Join(dir, ".contentcloud", "project.json")
			if b, readErr := os.ReadFile(path); readErr == nil {
				var value struct {
					ProjectID string `json:"project_id"`
				}
				if json.Unmarshal(b, &value) == nil && value.ProjectID != "" {
					return value.ProjectID, nil
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	return "", fmt.Errorf("PROJECT_CONTEXT_REQUIRED")
}
