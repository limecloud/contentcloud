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
	ServerURL   string `json:"server_url"`
	DeviceID    string `json:"device_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
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
