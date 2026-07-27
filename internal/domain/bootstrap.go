package domain

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const BootstrapSchemaVersion = "1.0"

var bootstrapStages = []string{
	"prerequisites",
	"codex_ready",
	"network_ready",
	"workspace_selected",
	"plan_ready",
	"awaiting_confirmation",
	"plugin_installing",
	"authorizing",
	"workspace_initializing",
	"doctor_running",
	"registering",
	"opening_desktop",
	"complete",
}

var bootstrapStatuses = map[string]bool{
	"started": true, "passed": true, "needs_action": true, "failed": true, "skipped": true,
}

var bootstrapFactKeys = map[string]bool{
	"platform": true, "arch": true, "cli_version": true, "node_version": true,
	"codex_version": true, "host_kind": true, "codex_home_kind": true,
	"available": true, "supported": true, "writable": true, "reachable": true,
	"latency_bucket": true, "workspace_kind": true, "installed": true, "version": true,
	"same_host": true, "same_codex_home": true, "rollback_complete": true,
}

var bootstrapSecretPattern = regexp.MustCompile(`(?i)(cck_|cbt_|\b(?:connect[_-]?key|access[_-]?token|refresh[_-]?token|authorization|cookie|password|secret)\b|bearer\s+)`)

type BootstrapAttempt struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	ProjectID        string     `json:"project_id"`
	ConnectSessionID string     `json:"connect_session_id"`
	AttemptTokenHash string     `json:"-"`
	CodeChallenge    string     `json:"-"`
	UserCode         string     `json:"user_code"`
	State            string     `json:"state"`
	SupportCode      string     `json:"support_code"`
	LastSequence     int64      `json:"last_sequence"`
	DecidedBy        string     `json:"decided_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	ExpiresAt        time.Time  `json:"expires_at"`
	DecidedAt        *time.Time `json:"decided_at,omitempty"`
	ConsumedAt       *time.Time `json:"consumed_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

type BootstrapProgressEvent struct {
	SchemaVersion string         `json:"schema_version"`
	AttemptID     string         `json:"attempt_id"`
	Sequence      int64          `json:"sequence"`
	OccurredAt    time.Time      `json:"occurred_at"`
	Stage         string         `json:"stage"`
	Status        string         `json:"status"`
	CheckID       string         `json:"check_id,omitempty"`
	ErrorCode     string         `json:"error_code,omitempty"`
	ActionID      string         `json:"action_id,omitempty"`
	Facts         map[string]any `json:"facts,omitempty"`
}

type BootstrapProgress struct {
	AttemptID   string           `json:"attempt_id"`
	Stage       string           `json:"stage"`
	Status      string           `json:"status"`
	Step        int              `json:"step"`
	StepCount   int              `json:"step_count"`
	CheckID     string           `json:"check_id,omitempty"`
	ErrorCode   string           `json:"error_code,omitempty"`
	ActionID    string           `json:"action_id,omitempty"`
	Action      *BootstrapAction `json:"action,omitempty"`
	SupportCode string           `json:"support_code"`
	UserCode    string           `json:"user_code,omitempty"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type BootstrapAction struct {
	ID                   string   `json:"action_id"`
	Kind                 string   `json:"kind"`
	Title                string   `json:"title"`
	Body                 string   `json:"body"`
	DocURL               string   `json:"doc_url,omitempty"`
	Handler              string   `json:"handler,omitempty"`
	RequiresConfirmation bool     `json:"requires_confirmation"`
	Recheck              []string `json:"recheck"`
}

type BootstrapActionCatalog struct {
	SchemaVersion string            `json:"schema_version"`
	Actions       []BootstrapAction `json:"actions"`
}

type BootstrapDiagnosticSummary struct {
	SchemaVersion    string                     `json:"schema_version"`
	AttemptID        string                     `json:"attempt_id"`
	Platform         string                     `json:"platform"`
	Arch             string                     `json:"arch"`
	Versions         map[string]string          `json:"versions"`
	Checks           []BootstrapDiagnosticCheck `json:"checks"`
	ManagedDigests   map[string]string          `json:"managed_digests,omitempty"`
	RollbackComplete *bool                      `json:"rollback_complete,omitempty"`
}

type BootstrapDiagnosticCheck struct {
	CheckID   string `json:"check_id"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
}

type BootstrapDiagnostic struct {
	ID          string                     `json:"id"`
	TenantID    string                     `json:"tenant_id"`
	ProjectID   string                     `json:"project_id"`
	AttemptID   string                     `json:"attempt_id"`
	SupportCode string                     `json:"support_code"`
	Digest      string                     `json:"digest"`
	ByteSize    int64                      `json:"byte_size"`
	Summary     BootstrapDiagnosticSummary `json:"summary"`
	CreatedAt   time.Time                  `json:"created_at"`
}

func BootstrapStageStep(stage string) (int, int) {
	for index, candidate := range bootstrapStages {
		if candidate == stage {
			return index + 1, len(bootstrapStages)
		}
	}
	return 0, len(bootstrapStages)
}

func BootstrapProgressFrom(attempt BootstrapAttempt, latest BootstrapProgressEvent) *BootstrapProgress {
	stage, status, actionID, updatedAt := "authorizing", "needs_action", "open.browser.authorization", attempt.UpdatedAt
	if latest.Sequence > 0 {
		stage, status, actionID, updatedAt = latest.Stage, latest.Status, latest.ActionID, latest.OccurredAt
	}
	if stage == "authorizing" && status == "needs_action" {
		switch attempt.State {
		case "approved":
			status, actionID = "started", ""
		case "denied":
			status, actionID = "failed", ""
			latest.ErrorCode = "BOOTSTRAP_AUTHORIZATION_DENIED"
		}
	}
	step, count := BootstrapStageStep(stage)
	progress := &BootstrapProgress{AttemptID: attempt.ID, Stage: stage, Status: status, Step: step, StepCount: count, CheckID: latest.CheckID, ErrorCode: latest.ErrorCode, ActionID: actionID, SupportCode: attempt.SupportCode, UpdatedAt: updatedAt}
	if action, ok := BootstrapActionByID(actionID); ok {
		progress.Action = &action
	}
	if stage == "authorizing" && attempt.State == "pending" {
		progress.UserCode = attempt.UserCode
	}
	return progress
}

func ValidateBootstrapEvent(event BootstrapProgressEvent) error {
	if event.SchemaVersion != BootstrapSchemaVersion {
		return Invalid("BOOTSTRAP_PROGRESS_SCHEMA_INVALID", "bootstrap progress schema_version 必须为 1.0")
	}
	if event.Sequence < 1 {
		return Invalid("BOOTSTRAP_PROGRESS_SEQUENCE_INVALID", "bootstrap progress sequence 必须大于 0")
	}
	if step, _ := BootstrapStageStep(event.Stage); step == 0 {
		return Invalid("BOOTSTRAP_PROGRESS_STAGE_INVALID", "bootstrap progress stage 不受支持")
	}
	if !bootstrapStatuses[event.Status] {
		return Invalid("BOOTSTRAP_PROGRESS_STATUS_INVALID", "bootstrap progress status 不受支持")
	}
	if event.CheckID != "" && !BootstrapCheckIDs()[event.CheckID] {
		return Invalid("BOOTSTRAP_CHECK_ID_INVALID", "bootstrap check_id 不在版本化目录中")
	}
	if event.ActionID != "" {
		if _, ok := BootstrapActionByID(event.ActionID); !ok {
			return Invalid("BOOTSTRAP_ACTION_ID_INVALID", "bootstrap action_id 不在版本化目录中")
		}
	}
	for key, value := range event.Facts {
		if !bootstrapFactKeys[key] {
			return Invalid("BOOTSTRAP_FACT_NOT_ALLOWED", fmt.Sprintf("bootstrap fact %q 不在允许列表中", key))
		}
		if err := validateBootstrapScalar(value); err != nil {
			return err
		}
	}
	return nil
}

func BootstrapCheckIDs() map[string]bool {
	ids := []string{
		"runtime.platform.supported", "runtime.node.available", "runtime.node.version", "runtime.npx.available",
		"runtime.temp.writable", "runtime.credential_store.available", "runtime.path.consistent", "codex.cli.available", "codex.cli.version",
		"codex.desktop.available", "codex.auth.ready", "codex.home.consistent", "codex.workspace.policy",
		"network.contentcloud.reachable", "network.npm.reachable", "network.marketplace.reachable", "network.openai.reachable",
		"codex.marketplace.identity", "codex.marketplace.source_conflict", "codex.plugin.identity",
		"codex.plugin.source_conflict", "codex.plugin.enabled", "codex.plugin.new_session",
		"workspace.path.safe", "workspace.path.writable", "workspace.binding", "workspace.template_lock",
		"workspace.managed_files", "workspace.capability_routing", "environment.signature", "environment.lock",
		"workspace.registration", "desktop.new_chat",
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func BootstrapActions() BootstrapActionCatalog {
	actions := []BootstrapAction{
		{ID: "guide.platform.requirements", Kind: "open_guide", Title: "检查受支持的电脑环境", Body: "首批支持 macOS。请确认系统和芯片架构后重新检查。", DocURL: "/help/bootstrap/requirements", Recheck: []string{"runtime.platform.supported"}},
		{ID: "guide.node.install", Kind: "open_guide", Title: "安装 Node.js 20 或更高版本", Body: "完成安装后关闭并重新打开 Codex，再运行环境检查。", DocURL: "https://nodejs.org/en/download", Recheck: []string{"runtime.node.available", "runtime.node.version", "runtime.npx.available"}},
		{ID: "guide.node.upgrade", Kind: "open_guide", Title: "升级 Node.js", Body: "当前 Node.js 版本过低，需要升级到 20 或更高版本。", DocURL: "https://nodejs.org/en/download", Recheck: []string{"runtime.node.version", "runtime.npx.available"}},
		{ID: "guide.npx.repair", Kind: "open_guide", Title: "修复 npm 与 npx", Body: "确认 npm/npx 与 Node.js 来自同一套安装，然后重新打开 Codex。", DocURL: "https://docs.npmjs.com/downloading-and-installing-node-js-and-npm", Recheck: []string{"runtime.npx.available"}},
		{ID: "guide.permissions.temp", Kind: "open_guide", Title: "修复临时目录权限", Body: "ContentCloud 需要在系统临时目录创建短期检测文件。", DocURL: "/help/bootstrap/permissions", Recheck: []string{"runtime.temp.writable"}},
		{ID: "guide.credentials.keychain", Kind: "open_guide", Title: "检查 macOS 钥匙串", Body: "ContentCloud 需要使用当前用户的默认钥匙串保存设备和工作区凭据。解锁钥匙串后重新检查。", DocURL: "/help/bootstrap/keychain", Recheck: []string{"runtime.credential_store.available"}},
		{ID: "guide.path.desktop", Kind: "open_guide", Title: "让 Codex Desktop 识别命令行工具", Body: "重新打开 Codex Desktop；若仍失败，请按教程统一 Desktop 与终端的 PATH。", DocURL: "/help/bootstrap/codex-path", Recheck: []string{"runtime.path.consistent"}},
		{ID: "guide.codex.cli_install", Kind: "open_guide", Title: "安装 Codex CLI", Body: "安装 Codex CLI 后重新打开终端和 Codex Desktop。", DocURL: "https://developers.openai.com/codex/quickstart", Recheck: []string{"codex.cli.available", "codex.cli.version"}},
		{ID: "guide.codex.upgrade", Kind: "open_guide", Title: "升级 Codex", Body: "当前 Codex CLI 不在服务端兼容范围内。升级后重新检查。", DocURL: "https://developers.openai.com/codex/quickstart", Recheck: []string{"codex.cli.version"}},
		{ID: "guide.codex.desktop_install", Kind: "open_guide", Title: "安装 Codex Desktop", Body: "安装并登录 Codex Desktop，然后回到这里重新检查。", DocURL: "https://developers.openai.com/codex/app", Recheck: []string{"codex.desktop.available"}},
		{ID: "open.codex.login", Kind: "open_codex", Title: "登录 Codex", Body: "在 Codex 中完成登录，再重新检查当前步骤。", Handler: "codex_login", Recheck: []string{"codex.auth.ready"}},
		{ID: "guide.codex.home_mismatch", Kind: "open_guide", Title: "统一 Codex 配置目录", Body: "CLI 与 Desktop 需要在同一用户和同一 CODEX_HOME 下运行。", DocURL: "/help/bootstrap/codex-home", Recheck: []string{"codex.home.consistent"}},
		{ID: "contact_admin.codex_policy", Kind: "contact_support", Title: "联系 Codex 管理员", Body: "当前 Workspace 策略可能禁止 Plugin 或 MCP。", Handler: "contact_admin", Recheck: []string{"codex.workspace.policy"}},
		{ID: "retry.network.contentcloud", Kind: "retry_check", Title: "重新连接 ContentCloud", Body: "检查网络后只重试 ContentCloud 服务连接。", Handler: "retry_check", Recheck: []string{"network.contentcloud.reachable"}},
		{ID: "guide.network.npm", Kind: "open_guide", Title: "检查 npm 网络", Body: "确认网络或代理允许访问 npm Registry。", DocURL: "/help/bootstrap/network", Recheck: []string{"network.npm.reachable"}},
		{ID: "guide.network.marketplace", Kind: "open_guide", Title: "检查 Plugin 来源网络", Body: "确认网络或代理允许访问固定的 Plugin Marketplace 来源。", DocURL: "/help/bootstrap/network", Recheck: []string{"network.marketplace.reachable"}},
		{ID: "guide.network.openai", Kind: "open_guide", Title: "检查 Codex 网络", Body: "确认 Codex 可以登录并访问 OpenAI 服务。", DocURL: "https://developers.openai.com/codex/quickstart", Recheck: []string{"network.openai.reachable"}},
		{ID: "repair.marketplace.install", Kind: "run_managed_repair", Title: "安装 ContentCloud Marketplace", Body: "将展示固定来源和版本的安装计划，确认后才执行。", Handler: "marketplace_install", RequiresConfirmation: true, Recheck: []string{"codex.marketplace.identity"}},
		{ID: "contact_support.marketplace_identity_conflict", Kind: "contact_support", Title: "Marketplace 来源冲突", Body: "检测到同名但不同来源的 Marketplace，不会自动覆盖。", Handler: "diagnostic_support", Recheck: []string{"codex.marketplace.source_conflict"}},
		{ID: "repair.plugin.install", Kind: "run_managed_repair", Title: "安装 ContentCloud Plugin", Body: "将展示固定 Plugin 版本和来源，确认后才执行。", Handler: "plugin_install", RequiresConfirmation: true, Recheck: []string{"codex.plugin.identity"}},
		{ID: "contact_support.plugin_identity_conflict", Kind: "contact_support", Title: "Plugin 来源冲突", Body: "检测到同名但不同来源的 Plugin，不会自动覆盖。", Handler: "diagnostic_support", Recheck: []string{"codex.plugin.source_conflict"}},
		{ID: "repair.plugin.enable", Kind: "run_managed_repair", Title: "启用 ContentCloud Plugin", Body: "确认后启用已验证来源的 Plugin。", Handler: "plugin_enable", RequiresConfirmation: true, Recheck: []string{"codex.plugin.enabled"}},
		{ID: "open.codex.new_workspace_chat", Kind: "open_codex", Title: "打开新的项目对话", Body: "Plugin、Skill 或 MCP 变化需要在新对话中生效。", Handler: "new_workspace_chat", Recheck: []string{"codex.plugin.new_session", "desktop.new_chat"}},
		{ID: "choose.workspace.directory", Kind: "choose_directory", Title: "选择新的工作区目录", Body: "请选择空目录或已绑定同一项目的 ContentCloud 目录。", Handler: "choose_directory", Recheck: []string{"workspace.path.safe", "workspace.path.writable"}},
		{ID: "guide.permissions.workspace", Kind: "open_guide", Title: "修复工作区权限", Body: "当前目录不可写，请选择有权限的目录。", DocURL: "/help/bootstrap/permissions", Recheck: []string{"workspace.path.writable"}},
		{ID: "repair.bootstrap.resume", Kind: "run_managed_repair", Title: "恢复初始化", Body: "复用已存在的安全凭据和工作区绑定继续初始化。", Handler: "bootstrap_resume", RequiresConfirmation: true, Recheck: []string{"workspace.binding", "workspace.template_lock"}},
		{ID: "review.workspace.managed_files", Kind: "open_guide", Title: "检查受管文件变化", Body: "先查看 ContentCloud 受管文件的漂移，再决定是否恢复。", DocURL: "/help/bootstrap/managed-files", Recheck: []string{"workspace.managed_files"}},
		{ID: "repair.routing.update", Kind: "run_managed_repair", Title: "更新能力路由", Body: "确认后只更新 ContentCloud 管理的能力路由。", Handler: "routing_update", RequiresConfirmation: true, Recheck: []string{"workspace.capability_routing"}},
		{ID: "contact_support.environment_signature", Kind: "contact_support", Title: "环境签名验证失败", Body: "为避免安装被篡改的内容，此问题必须由支持人员核验。", Handler: "diagnostic_support", Recheck: []string{"environment.signature"}},
		{ID: "repair.environment.plan", Kind: "run_managed_repair", Title: "重新生成环境计划", Body: "拉取并验证最新签名环境后重新展示变更计划。", Handler: "environment_plan", RequiresConfirmation: true, Recheck: []string{"environment.lock"}},
		{ID: "retry.bootstrap.resume", Kind: "run_managed_repair", Title: "继续注册工作区", Body: "本地检查已完成，只重试工作区注册。", Handler: "bootstrap_resume", RequiresConfirmation: true, Recheck: []string{"workspace.registration"}},
		{ID: "open.codex.recovery_prompt", Kind: "copy_fixed_command", Title: "复制恢复入口", Body: "复制当前 CLI 固定的恢复命令，在工作区中新建 Codex 对话。", Handler: "copy_bootstrap_resume", Recheck: []string{"desktop.new_chat"}},
		{ID: "open.browser.authorization", Kind: "open_browser_auth", Title: "确认这台电脑", Body: "在已登录的 ContentCloud 页面确认项目和本机授权。", Handler: "approve_bootstrap_authorization", Recheck: []string{}},
		{ID: "create.diagnostic.bundle", Kind: "create_diagnostic_bundle", Title: "生成诊断摘要", Body: "在本机生成脱敏摘要，预览后再决定是否上传。", Handler: "create_diagnostic_bundle", RequiresConfirmation: true, Recheck: []string{}},
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })
	return BootstrapActionCatalog{SchemaVersion: BootstrapSchemaVersion, Actions: actions}
}

func BootstrapActionByID(id string) (BootstrapAction, bool) {
	for _, action := range BootstrapActions().Actions {
		if action.ID == id {
			return action, true
		}
	}
	return BootstrapAction{}, false
}

func ValidateBootstrapActionCatalog(catalog BootstrapActionCatalog) error {
	allowedKinds := map[string]bool{
		"retry_check": true, "open_guide": true, "open_browser_auth": true, "open_codex": true,
		"choose_directory": true, "run_managed_repair": true, "copy_fixed_command": true,
		"create_diagnostic_bundle": true, "contact_support": true,
	}
	seen := map[string]bool{}
	for _, action := range catalog.Actions {
		if action.ID == "" || seen[action.ID] || !allowedKinds[action.Kind] {
			return Invalid("BOOTSTRAP_ACTION_CATALOG_INVALID", "Action Catalog 包含重复 ID 或不受支持的 kind")
		}
		seen[action.ID] = true
		if action.Kind == "run_managed_repair" && (action.Handler == "" || !action.RequiresConfirmation) {
			return Invalid("BOOTSTRAP_ACTION_CATALOG_INVALID", "受管修复必须使用固定 handler 并要求确认")
		}
		if action.DocURL != "" {
			parsed, err := url.Parse(action.DocURL)
			if err != nil || (parsed.IsAbs() && parsed.Scheme != "https") || strings.Contains(strings.ToLower(action.DocURL), "javascript:") {
				return Invalid("BOOTSTRAP_ACTION_URL_INVALID", "Action Catalog 文档地址必须是 HTTPS 或站内路径")
			}
		}
	}
	return nil
}

func ValidateBootstrapDiagnostic(summary BootstrapDiagnosticSummary) error {
	if summary.SchemaVersion != BootstrapSchemaVersion || summary.AttemptID == "" {
		return Invalid("BOOTSTRAP_DIAGNOSTIC_INVALID", "诊断摘要缺少 schema_version 或 attempt_id")
	}
	if len(summary.Checks) > 64 || len(summary.Versions) > 8 || len(summary.ManagedDigests) > 16 {
		return Invalid("BOOTSTRAP_DIAGNOSTIC_TOO_LARGE", "诊断摘要字段数量超出限制")
	}
	for key, value := range summary.Versions {
		if key != "node" && key != "contentcloud_cli" && key != "codex_cli" && key != "codex_desktop" {
			return Invalid("BOOTSTRAP_DIAGNOSTIC_FIELD_NOT_ALLOWED", "诊断摘要包含未允许的版本字段")
		}
		if bootstrapSecretPattern.MatchString(value) || len(value) > 80 {
			return Invalid("BOOTSTRAP_DIAGNOSTIC_SECRET_DETECTED", "诊断摘要可能包含秘密或异常长字段")
		}
	}
	for _, check := range summary.Checks {
		if !BootstrapCheckIDs()[check.CheckID] || !bootstrapStatuses[check.Status] {
			return Invalid("BOOTSTRAP_DIAGNOSTIC_CHECK_INVALID", "诊断摘要包含未知 check 或 status")
		}
	}
	for key, value := range summary.ManagedDigests {
		if key != "environment_lock" && key != "plugin_spec" && key != "workspace_binding" {
			return Invalid("BOOTSTRAP_DIAGNOSTIC_FIELD_NOT_ALLOWED", "诊断摘要包含未允许的 digest 字段")
		}
		if value != "" && !strings.HasPrefix(value, "sha256:") {
			return Invalid("BOOTSTRAP_DIAGNOSTIC_DIGEST_INVALID", "诊断摘要 digest 格式错误")
		}
	}
	return nil
}

func validateBootstrapScalar(value any) error {
	switch typed := value.(type) {
	case string:
		if len(typed) > 120 || bootstrapSecretPattern.MatchString(typed) || strings.HasPrefix(typed, "/") || strings.Contains(typed, `:\`) {
			return Invalid("BOOTSTRAP_FACT_VALUE_NOT_ALLOWED", "bootstrap fact 包含秘密、绝对路径或异常长字符串")
		}
	case bool, float64, float32, int, int32, int64, nil:
		return nil
	default:
		return Invalid("BOOTSTRAP_FACT_VALUE_NOT_ALLOWED", "bootstrap fact 只能是非秘密标量")
	}
	return nil
}
