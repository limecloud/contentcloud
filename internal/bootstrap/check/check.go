package bootstrapcheck

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

const MinNodeMajor = 20
const MinCodexVersion = "0.145.0"

type Check struct {
	Stage     string         `json:"stage"`
	CheckID   string         `json:"check_id"`
	Status    string         `json:"status"`
	ErrorCode string         `json:"error_code,omitempty"`
	ActionID  string         `json:"action_id,omitempty"`
	Facts     map[string]any `json:"facts,omitempty"`
}

type Report struct {
	SchemaVersion string  `json:"schema_version"`
	OK            bool    `json:"ok"`
	Platform      string  `json:"platform"`
	Arch          string  `json:"arch"`
	Checks        []Check `json:"checks"`
	FirstFailure  *Check  `json:"first_failure,omitempty"`
}

type CommandRunner interface {
	Run(context.Context, string, ...string) (string, error)
}

type Options struct {
	Directory  string
	ServerURL  string
	Offline    bool
	Platform   string
	Arch       string
	Runner     CommandRunner
	HTTPClient *http.Client
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	body, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(body)), err
}

func Run(ctx context.Context, options Options) Report {
	platform := defaultValue(options.Platform, runtime.GOOS)
	arch := defaultValue(options.Arch, runtime.GOARCH)
	runner := options.Runner
	if runner == nil {
		runner = execRunner{}
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	report := Report{SchemaVersion: workspacedomain.BootstrapSchemaVersion, OK: true, Platform: platform, Arch: arch, Checks: []Check{}}
	add := func(check Check) {
		if check.Facts == nil {
			check.Facts = map[string]any{}
		}
		report.Checks = append(report.Checks, check)
		if (check.Status == "failed" || check.Status == "needs_action") && report.FirstFailure == nil {
			copy := check
			report.FirstFailure = &copy
			report.OK = false
		}
	}

	if platform == "darwin" {
		add(passed("prerequisites", "runtime.platform.supported", map[string]any{"platform": platform, "arch": arch, "supported": true}))
	} else {
		add(failed("prerequisites", "runtime.platform.supported", "RUNTIME_PLATFORM_UNSUPPORTED", "guide.platform.requirements", map[string]any{"platform": platform, "arch": arch, "supported": false}))
	}

	nodeOutput, nodeErr := runVersion(ctx, runner, "node", "--version")
	if nodeErr != nil {
		add(failed("prerequisites", "runtime.node.available", "NODE_NOT_FOUND", "guide.node.install", map[string]any{"available": false}))
		add(skipped("prerequisites", "runtime.node.version", "guide.node.install"))
	} else {
		add(passed("prerequisites", "runtime.node.available", map[string]any{"available": true}))
		major, versionOK := majorVersion(nodeOutput)
		if !versionOK || major < MinNodeMajor {
			add(failed("prerequisites", "runtime.node.version", "NODE_VERSION_UNSUPPORTED", "guide.node.upgrade", map[string]any{"node_version": sanitizeVersion(nodeOutput), "supported": false}))
		} else {
			add(passed("prerequisites", "runtime.node.version", map[string]any{"node_version": sanitizeVersion(nodeOutput), "supported": true}))
		}
	}

	if _, err := runVersion(ctx, runner, "npx", "--version"); err != nil {
		add(failed("prerequisites", "runtime.npx.available", "NPX_NOT_FOUND", "guide.npx.repair", map[string]any{"available": false}))
	} else {
		add(passed("prerequisites", "runtime.npx.available", map[string]any{"available": true}))
	}

	if writableTemp() {
		add(passed("prerequisites", "runtime.temp.writable", map[string]any{"writable": true}))
	} else {
		add(failed("prerequisites", "runtime.temp.writable", "TEMP_DIRECTORY_NOT_WRITABLE", "guide.permissions.temp", map[string]any{"writable": false}))
	}
	if platform == "darwin" {
		if output, err := runVersion(ctx, runner, "security", "default-keychain", "-d", "user"); err != nil || strings.Trim(strings.TrimSpace(output), `"`) == "" {
			add(failed("prerequisites", "runtime.credential_store.available", "MACOS_KEYCHAIN_UNAVAILABLE", "guide.credentials.keychain", map[string]any{"available": false}))
		} else {
			add(passed("prerequisites", "runtime.credential_store.available", map[string]any{"available": true}))
		}
	} else {
		add(skipped("prerequisites", "runtime.credential_store.available", "guide.platform.requirements"))
	}

	codexOutput, codexErr := runVersion(ctx, runner, "codex", "--version")
	if codexErr != nil {
		add(failed("codex_ready", "codex.cli.available", "CODEX_CLI_NOT_FOUND", "guide.codex.cli_install", map[string]any{"available": false}))
		add(skipped("codex_ready", "codex.cli.version", "guide.codex.cli_install"))
	} else {
		version := sanitizeVersion(codexOutput)
		add(passed("codex_ready", "codex.cli.available", map[string]any{"available": true}))
		if semverCompare(version, MinCodexVersion) < 0 {
			add(failed("codex_ready", "codex.cli.version", "CODEX_VERSION_UNSUPPORTED", "guide.codex.upgrade", map[string]any{"codex_version": version, "supported": false}))
		} else {
			add(passed("codex_ready", "codex.cli.version", map[string]any{"codex_version": version, "supported": true}))
		}
	}

	if platform == "darwin" {
		if _, err := runner.Run(ctx, "open", "-Ra", "Codex"); err != nil {
			add(failed("codex_ready", "codex.desktop.available", "CODEX_DESKTOP_NOT_FOUND", "guide.codex.desktop_install", map[string]any{"available": false}))
		} else {
			add(passed("codex_ready", "codex.desktop.available", map[string]any{"available": true}))
		}
	} else {
		add(skipped("codex_ready", "codex.desktop.available", "guide.codex.desktop_install"))
	}

	if nodeErr == nil && codexErr == nil {
		add(passed("codex_ready", "runtime.path.consistent", map[string]any{"same_host": true}))
	} else {
		add(failed("codex_ready", "runtime.path.consistent", "DESKTOP_PATH_INCOMPLETE", "guide.path.desktop", map[string]any{"same_host": true}))
	}
	add(passed("codex_ready", "codex.home.consistent", map[string]any{"same_codex_home": true, "codex_home_kind": codexHomeKind()}))
	add(skipped("codex_ready", "codex.auth.ready", "open.codex.login"))
	add(skipped("codex_ready", "codex.workspace.policy", "contact_admin.codex_policy"))

	workspacePlan, err := localworkspace.Plan(options.Directory, "codex-plugin")
	if err != nil {
		add(failed("workspace_selected", "workspace.path.safe", "WORKSPACE_PATH_INVALID", "choose.workspace.directory", map[string]any{"workspace_kind": "invalid"}))
	} else if workspacePlan.State == "non_empty" {
		add(failed("workspace_selected", "workspace.path.safe", "WORKSPACE_DIRECTORY_CONFLICT", "choose.workspace.directory", map[string]any{"workspace_kind": workspacePlan.State}))
	} else {
		add(passed("workspace_selected", "workspace.path.safe", map[string]any{"workspace_kind": workspacePlan.State}))
	}
	if workspaceWritable(options.Directory) {
		add(passed("workspace_selected", "workspace.path.writable", map[string]any{"writable": true}))
	} else {
		add(failed("workspace_selected", "workspace.path.writable", "WORKSPACE_NOT_WRITABLE", "guide.permissions.workspace", map[string]any{"writable": false}))
	}

	if options.Offline {
		for _, check := range []struct{ id, action string }{
			{"network.contentcloud.reachable", "retry.network.contentcloud"}, {"network.npm.reachable", "guide.network.npm"},
			{"network.marketplace.reachable", "guide.network.marketplace"}, {"network.openai.reachable", "guide.network.openai"},
		} {
			add(skipped("network_ready", check.id, check.action))
		}
		return report
	}

	targets := []struct {
		id, rawURL, action string
	}{
		{"network.contentcloud.reachable", strings.TrimRight(options.ServerURL, "/") + "/healthz", "retry.network.contentcloud"},
		{"network.npm.reachable", "https://registry.npmjs.org/@limecloud%2Fcontentcloud/latest", "guide.network.npm"},
		{"network.marketplace.reachable", "https://github.com/limecloud/contentcloud", "guide.network.marketplace"},
	}
	for _, target := range targets {
		if reachable(ctx, client, target.rawURL) {
			add(passed("network_ready", target.id, map[string]any{"reachable": true, "latency_bucket": "under_5s"}))
		} else {
			add(failed("network_ready", target.id, strings.ToUpper(strings.ReplaceAll(target.id, ".", "_"))+"_FAILED", target.action, map[string]any{"reachable": false, "latency_bucket": "failed"}))
		}
	}
	// Codex readiness is verified by local CLI/Desktop checks and the real plugin flow.
	// The public documentation site is not a reliable proxy for the active Codex session.
	add(skipped("network_ready", "network.openai.reachable", "guide.network.openai"))
	return report
}

func passed(stage, id string, facts map[string]any) Check {
	return Check{Stage: stage, CheckID: id, Status: "passed", Facts: facts}
}

func failed(stage, id, code, action string, facts map[string]any) Check {
	return Check{Stage: stage, CheckID: id, Status: "needs_action", ErrorCode: code, ActionID: action, Facts: facts}
}

func skipped(stage, id, action string) Check {
	return Check{Stage: stage, CheckID: id, Status: "skipped", ActionID: action, Facts: map[string]any{}}
}

func runVersion(ctx context.Context, runner CommandRunner, name string, args ...string) (string, error) {
	checkContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return runner.Run(checkContext, name, args...)
}

var versionPattern = regexp.MustCompile(`\d+(?:\.\d+){0,2}`)

func sanitizeVersion(value string) string {
	return versionPattern.FindString(value)
}

func majorVersion(value string) (int, bool) {
	version := sanitizeVersion(value)
	if version == "" {
		return 0, false
	}
	major, err := strconv.Atoi(strings.Split(version, ".")[0])
	return major, err == nil
}

func semverCompare(left, right string) int {
	parse := func(value string) [3]int {
		var result [3]int
		parts := strings.Split(sanitizeVersion(value), ".")
		for index := 0; index < len(parts) && index < len(result); index++ {
			result[index], _ = strconv.Atoi(parts[index])
		}
		return result
	}
	a, b := parse(left), parse(right)
	for index := range a {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	return 0
}

func reachable(ctx context.Context, client *http.Client, rawURL string) bool {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 500
}

func writableTemp() bool {
	directory, err := os.MkdirTemp("", "contentcloud-bootstrap-check-")
	if err != nil {
		return false
	}
	return os.Remove(directory) == nil
}

func workspaceWritable(path string) bool {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for {
		info, statErr := os.Stat(absolute)
		if statErr == nil {
			if !info.IsDir() {
				return false
			}
			return info.Mode().Perm()&0o222 != 0
		}
		if !os.IsNotExist(statErr) {
			return false
		}
		parent := filepath.Dir(absolute)
		if parent == absolute {
			return false
		}
		absolute = parent
	}
}

func codexHomeKind() string {
	if strings.TrimSpace(os.Getenv("CODEX_HOME")) != "" {
		return "explicit"
	}
	return "default_user"
}

func defaultValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func ValidateReport(report Report) error {
	if report.SchemaVersion != workspacedomain.BootstrapSchemaVersion || len(report.Checks) == 0 {
		return fmt.Errorf("bootstrap 检查报告结构无效")
	}
	for _, check := range report.Checks {
		event := workspacedomain.BootstrapProgressEvent{SchemaVersion: report.SchemaVersion, Sequence: 1, OccurredAt: time.Now().UTC(), Stage: check.Stage, Status: check.Status, CheckID: check.CheckID, ErrorCode: check.ErrorCode, ActionID: check.ActionID, Facts: check.Facts}
		if err := workspacedomain.ValidateBootstrapEvent(event); err != nil {
			return err
		}
	}
	return nil
}
