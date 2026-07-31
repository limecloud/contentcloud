package localconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	ServerURL      string          `json:"server_url"`
	DeviceID       string          `json:"device_id,omitempty"`
	WorkspaceID    string          `json:"workspace_id,omitempty"`
	ProjectID      string          `json:"project_id,omitempty"`
	WorkspaceRoot  string          `json:"workspace_root,omitempty"`
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
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}
func Save(c Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
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

// RuntimeBindings returns the daemon registrations while preserving compatibility
// with the original single-workspace config shape.
func (c Config) RuntimeBindings() []DaemonBinding {
	bindings := cloneDaemonBindings(c.DaemonBindings)
	legacy := DaemonBinding{
		ServerURL: strings.TrimSpace(c.ServerURL),
		DeviceID:  strings.TrimSpace(c.DeviceID),
		Workspaces: []DaemonWorkspace{{
			WorkspaceID: strings.TrimSpace(c.WorkspaceID),
			ProjectID:   strings.TrimSpace(c.ProjectID),
			Root:        strings.TrimSpace(c.WorkspaceRoot),
		}},
	}
	if legacy.DeviceID != "" {
		bindings = upsertDaemonBinding(bindings, legacy)
	}
	return normalizeDaemonBindings(bindings)
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
		return fmt.Errorf("refusing plaintext credential file %s", env)
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("secure credential storage is not implemented for %s; refusing plaintext fallback", runtime.GOOS)
	}
	cmd := exec.Command("security", "add-generic-password", "-U", "-a", deviceID, "-s", "contentcloud-device", "-w", token)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("store token in macOS Keychain: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
func DeviceToken(deviceID string) (string, error) {
	if token := os.Getenv("CONTENTCLOUD_DEVICE_TOKEN"); token != "" {
		return token, nil
	}
	if deviceID == "" {
		return "", fmt.Errorf("device is not configured")
	}
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("secure credential storage is not available for %s", runtime.GOOS)
	}
	out, err := exec.Command("security", "find-generic-password", "-w", "-a", deviceID, "-s", "contentcloud-device").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read token from macOS Keychain: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func DeleteDeviceToken(deviceID string) error {
	if deviceID == "" {
		return nil
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("secure credential storage is not available for %s", runtime.GOOS)
	}
	out, err := exec.Command("security", "delete-generic-password", "-a", deviceID, "-s", "contentcloud-device").CombinedOutput()
	if err != nil && !strings.Contains(string(out), "could not be found") {
		return fmt.Errorf("delete device token from macOS Keychain: %w", err)
	}
	return nil
}

func SaveWorkspaceToken(workspaceID, token string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("secure credential storage is not implemented for %s; refusing plaintext fallback", runtime.GOOS)
	}
	out, err := exec.Command("security", "add-generic-password", "-U", "-a", workspaceID, "-s", "contentcloud-workspace", "-w", token).CombinedOutput()
	if err != nil {
		return fmt.Errorf("store workspace token in macOS Keychain: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func WorkspaceToken(workspaceID string) (string, error) {
	if token := os.Getenv("CONTENTCLOUD_WORKSPACE_TOKEN"); token != "" {
		return token, nil
	}
	if workspaceID == "" {
		return "", fmt.Errorf("workspace is not configured")
	}
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("secure credential storage is not available for %s", runtime.GOOS)
	}
	out, err := exec.Command("security", "find-generic-password", "-w", "-a", workspaceID, "-s", "contentcloud-workspace").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read workspace token from macOS Keychain: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func SaveUserToken(serverURL, token string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("secure credential storage is not implemented for %s; refusing plaintext fallback", runtime.GOOS)
	}
	account := credentialAccount(serverURL)
	out, err := exec.Command("security", "add-generic-password", "-U", "-a", account, "-s", "contentcloud-user", "-w", token).CombinedOutput()
	if err != nil {
		return fmt.Errorf("store user token in macOS Keychain: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func UserToken(serverURL string) (string, error) {
	if token := os.Getenv("CONTENTCLOUD_TOKEN"); token != "" {
		return token, nil
	}
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("secure credential storage is not available for %s", runtime.GOOS)
	}
	out, err := exec.Command("security", "find-generic-password", "-w", "-a", credentialAccount(serverURL), "-s", "contentcloud-user").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read user token from macOS Keychain: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func DeleteUserToken(serverURL string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("secure credential storage is not available for %s", runtime.GOOS)
	}
	out, err := exec.Command("security", "delete-generic-password", "-a", credentialAccount(serverURL), "-s", "contentcloud-user").CombinedOutput()
	if err != nil && !strings.Contains(string(out), "could not be found") {
		return fmt.Errorf("delete user token from macOS Keychain: %w", err)
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
