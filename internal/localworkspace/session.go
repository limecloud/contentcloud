package localworkspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
)

const workspaceSessionSchema = "contentcloud.workspace-session/1.0"

type SessionBinding struct {
	SchemaVersion           string    `json:"schema_version"`
	SessionID               string    `json:"session_id"`
	WorkspaceID             string    `json:"workspace_id"`
	ProjectID               string    `json:"project_id"`
	EnvironmentDigest       string    `json:"environment_digest,omitempty"`
	EnvironmentLock         string    `json:"environment_lock_digest,omitempty"`
	EnvironmentDeclaration  string    `json:"environment_declaration_digest,omitempty"`
	PluginDeclaration       string    `json:"plugin_declaration_digest,omitempty"`
	SkillDeclaration        string    `json:"skill_declaration_digest,omitempty"`
	MCPDeclaration          string    `json:"mcp_declaration_digest,omitempty"`
	WorkspaceDeclaration    string    `json:"workspace_declaration_digest,omitempty"`
	PluginReceiptDigest     string    `json:"plugin_receipt_digest,omitempty"`
	PluginHostReceiptDigest string    `json:"plugin_host_receipt_digest,omitempty"`
	SkillDigest             string    `json:"skill_digest,omitempty"`
	MCPDigest               string    `json:"mcp_digest,omitempty"`
	WorkspaceTemplateDigest string    `json:"workspace_template_digest,omitempty"`
	Generation              string    `json:"generation"`
	CreatedAt               time.Time `json:"created_at"`
}

func EnsureSessionBinding(root, sessionID string, now time.Time) (SessionBinding, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return SessionBinding{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(sessionID) > 128 {
		return SessionBinding{}, domain.Invalid("WORKSPACE_SESSION_ID_INVALID", "工作区会话 ID 必须非空且不超过 128 个字符")
	}
	current, err := currentSessionBinding(resolved, sessionID, now)
	if err != nil {
		return SessionBinding{}, err
	}
	path := sessionBindingPath(resolved, sessionID)
	var stored SessionBinding
	readErr := readJSON(path, &stored)
	if errors.Is(readErr, os.ErrNotExist) {
		writeErr := writeExclusiveJSON(path, current)
		if writeErr != nil && !errors.Is(writeErr, os.ErrExist) {
			return SessionBinding{}, writeErr
		}
		if errors.Is(writeErr, os.ErrExist) {
			readErr = readJSON(path, &stored)
		} else {
			return current, nil
		}
	}
	if readErr != nil {
		return SessionBinding{}, readErr
	}
	if stored.SchemaVersion != workspaceSessionSchema || stored.SessionID != sessionID || stored.Generation == "" {
		return SessionBinding{}, domain.Invalid("WORKSPACE_SESSION_BINDING_INVALID", "工作区会话绑定文件无效")
	}
	if stored.Generation != current.Generation {
		err := domain.Conflict("WORKSPACE_SESSION_COMPONENTS_CHANGED", "当前对话绑定的 Plugin、Skill、MCP 或环境版本已经变化")
		err.Details = map[string]any{"requires_new_session": true, "session_id": sessionID, "previous_generation": stored.Generation, "current_generation": current.Generation}
		return SessionBinding{}, err
	}
	return stored, nil
}

// ObserveSessionBinding calculates the current local installation facts without
// creating or updating a conversation binding file. Daemon current-state uses
// this read-only view so status reporting never changes workspace state.
func ObserveSessionBinding(root string, now time.Time) (SessionBinding, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return SessionBinding{}, err
	}
	return currentSessionBinding(resolved, "daemon-current-state", now)
}

// ObserveWorkspace evaluates local readiness without writing a doctor probe or
// session file. It is safe to call from the long-running Daemon.
func ObserveWorkspace(root string, now time.Time) (domain.DaemonWorkspaceObservation, error) {
	status, err := LoadStatus(root)
	if err != nil {
		return domain.DaemonWorkspaceObservation{}, err
	}
	binding, err := currentSessionBinding(status.Root, "daemon-current-state", now)
	if err != nil {
		return domain.DaemonWorkspaceObservation{}, err
	}
	observation := domain.DaemonWorkspaceObservation{
		WorkspaceID: status.Binding.WorkspaceID, ProjectID: status.Binding.ProjectID,
		Status: "ready", Reason: "local_components_observed", Generation: binding.Generation,
		EnvironmentManifestDigest: status.Binding.EnvironmentDigest,
		EnvironmentDeclaration:    binding.EnvironmentDeclaration, PluginDeclaration: binding.PluginDeclaration,
		SkillDeclaration: binding.SkillDeclaration, MCPDeclaration: binding.MCPDeclaration,
		WorkspaceDeclaration: binding.WorkspaceDeclaration, PluginHostReceiptDigest: binding.PluginHostReceiptDigest,
		ObservedSkillDigest: binding.SkillDigest, ObservedMCPDigest: binding.MCPDigest,
		ObservedWorkspaceDigest: binding.WorkspaceTemplateDigest, ObservedAt: localNow(now),
	}
	switch {
	case binding.EnvironmentDeclaration == "" || binding.PluginDeclaration == "" || binding.SkillDeclaration == "" || binding.MCPDeclaration == "" || binding.WorkspaceDeclaration == "":
		observation.Status, observation.Reason = "repair_required", "environment_declaration_unavailable"
	case len(status.ModifiedManagedFiles) > 0 || len(status.MissingManagedFiles) > 0:
		observation.Status, observation.Reason = "repair_required", "managed_files_drift"
	default:
		skillsOK, _ := installedSkillsCheck(status.Root, status.Template)
		mcpOK, _ := installedMCPCheck(status.Root, status.Template)
		if !skillsOK {
			observation.Status, observation.Reason = "repair_required", "skill_drift"
		} else if !mcpOK {
			observation.Status, observation.Reason = "repair_required", "mcp_drift"
		} else if hasPluginTarget(status.Template.Targets) && binding.PluginHostReceiptDigest == "" {
			observation.Status, observation.Reason = "repair_required", "plugin_receipt_missing"
		}
	}
	return observation, nil
}

func hasPluginTarget(targets []string) bool {
	for _, target := range targets {
		if target == "codex-plugin" || target == "claude-plugin" {
			return true
		}
	}
	return false
}

func currentSessionBinding(root, sessionID string, now time.Time) (SessionBinding, error) {
	status, err := LoadStatus(root)
	if err != nil {
		return SessionBinding{}, err
	}
	lockBody, err := json.Marshal(status.Template)
	if err != nil {
		return SessionBinding{}, err
	}
	lockDigest := "sha256:" + digest(lockBody)
	manifestDigest := status.Binding.EnvironmentDigest
	environmentLockDigest := ""
	declaredPluginDigest := ""
	declarations := environment.DeclarationDigests{}
	workspaceDeclaration := ""
	if body, readErr := os.ReadFile(filepath.Join(root, ".contentcloud", environmentManifestFile)); readErr == nil {
		var manifest environment.Manifest
		if json.Unmarshal(body, &manifest) == nil {
			declarations, err = environment.DigestsForManifest(manifest)
			if err != nil {
				return SessionBinding{}, err
			}
			workspaceDeclaration = manifest.WorkspaceTemplate.Digest
		}
	}
	if body, readErr := os.ReadFile(filepath.Join(root, ".contentcloud", environmentLockFile)); readErr == nil {
		environmentLockDigest = "sha256:" + digest(body)
		var lock environment.EnvironmentLock
		if json.Unmarshal(body, &lock) == nil {
			pluginBody, _ := json.Marshal(lock.Plugins)
			declaredPluginDigest = "sha256:" + digest(pluginBody)
		}
	}
	pluginDigest, pluginHostReceiptDigest, err := currentPluginReceiptDigests(status.Template.Targets, declaredPluginDigest)
	if err != nil {
		return SessionBinding{}, err
	}
	skillBody, _ := json.Marshal(status.Template.Skills)
	mcpBody, _ := json.Marshal(status.Template.MCPServers)
	skillDigest := "sha256:" + digest(skillBody)
	mcpDigest := "sha256:" + digest(mcpBody)
	generationBody, err := json.Marshal(map[string]string{
		"workspace_id": status.Binding.WorkspaceID, "project_id": status.Binding.ProjectID,
		"environment_digest": manifestDigest, "environment_lock_digest": environmentLockDigest,
		"environment_declaration_digest": declarations.Environment, "plugin_declaration_digest": declarations.Plugin,
		"skill_declaration_digest": declarations.Skill, "mcp_declaration_digest": declarations.MCP,
		"workspace_declaration_digest": workspaceDeclaration, "plugin_receipt_digest": pluginDigest,
		"plugin_host_receipt_digest": pluginHostReceiptDigest, "skill_digest": skillDigest,
		"mcp_digest": mcpDigest, "template_digest": lockDigest,
	})
	if err != nil {
		return SessionBinding{}, err
	}
	return SessionBinding{
		SchemaVersion: workspaceSessionSchema, SessionID: sessionID, WorkspaceID: status.Binding.WorkspaceID, ProjectID: status.Binding.ProjectID,
		EnvironmentDigest: manifestDigest, EnvironmentLock: environmentLockDigest,
		EnvironmentDeclaration: declarations.Environment, PluginDeclaration: declarations.Plugin,
		SkillDeclaration: declarations.Skill, MCPDeclaration: declarations.MCP, WorkspaceDeclaration: workspaceDeclaration,
		PluginReceiptDigest: pluginDigest, PluginHostReceiptDigest: pluginHostReceiptDigest,
		SkillDigest: skillDigest, MCPDigest: mcpDigest, WorkspaceTemplateDigest: lockDigest,
		Generation: "sha256:" + digest(generationBody), CreatedAt: localNow(now),
	}, nil
}

func currentPluginReceiptDigests(targets []string, declaredDigest string) (string, string, error) {
	host := pluginhost.HostID("")
	for _, target := range targets {
		switch target {
		case "codex-plugin":
			host = pluginhost.HostCodex
		case "claude-plugin":
			host = pluginhost.HostClaude
		}
	}
	if host == "" {
		return declaredDigest, "", nil
	}
	root, err := pluginhost.DefaultStoreRoot()
	if err != nil {
		return "", "", err
	}
	receiptDigest, err := (&pluginhost.Store{Root: root}).ReceiptDigest(host)
	if err != nil {
		return "", "", err
	}
	body, err := json.Marshal(map[string]string{"declared": declaredDigest, "host": string(host), "receipts": receiptDigest})
	if err != nil {
		return "", "", err
	}
	return "sha256:" + digest(body), receiptDigest, nil
}

func sessionBindingPath(root, sessionID string) string {
	return filepath.Join(root, ".contentcloud", "sessions", domain.TokenHash(sessionID)+".json")
}
