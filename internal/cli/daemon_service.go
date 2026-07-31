package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	userDaemonLabel         = "com.goodvision.contentcloud"
	userDaemonSchemaVersion = "1.0"
)

type userDaemonState struct {
	SchemaVersion  string              `json:"schema_version"`
	Supported      bool                `json:"supported"`
	Installed      bool                `json:"installed"`
	Running        bool                `json:"running"`
	AlreadyRunning bool                `json:"already_running,omitempty"`
	PID            int                 `json:"pid,omitempty"`
	Version        string              `json:"version,omitempty"`
	Executable     string              `json:"executable,omitempty"`
	PlistPath      string              `json:"plist_path,omitempty"`
	LogPath        string              `json:"log_path,omitempty"`
	ErrorLogPath   string              `json:"error_log_path,omitempty"`
	PendingReports int                 `json:"pending_reports"`
	DeadLetters    int                 `json:"dead_letters"`
	UpdatedAt      *time.Time          `json:"updated_at,omitempty"`
	Runtime        *daemonRuntimeState `json:"runtime,omitempty"`
}

type userDaemonService interface {
	Start() (userDaemonState, error)
	Stop() (userDaemonState, error)
	Restart() (userDaemonState, error)
	Status() (userDaemonState, error)
	Uninstall() error
}

type launchdCommandRunner func(name string, args ...string) ([]byte, error)

type launchdDaemonService struct {
	home        string
	executable  string
	version     string
	uid         int
	now         func() time.Time
	run         launchdCommandRunner
	environment map[string]string
}

type userDaemonMetadata struct {
	SchemaVersion string    `json:"schema_version"`
	Version       string    `json:"version"`
	Executable    string    `json:"executable"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (r *Root) localDaemonService() (userDaemonService, error) {
	if r.daemonFactory != nil {
		return r.daemonFactory()
	}
	return newUserDaemonService()
}

func newUserDaemonService() (userDaemonService, error) {
	if runtime.GOOS != "darwin" {
		return nil, domain.Policy("DAEMON_PLATFORM_UNSUPPORTED", "当前平台尚不支持 ContentCloud 用户级常驻服务", "在 macOS 上使用 LaunchAgent，其他平台暂时使用 daemon run 前台模式")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &launchdDaemonService{
		home: home, executable: executable, version: Version, uid: os.Getuid(), now: time.Now,
		environment: daemonLaunchEnvironment(home),
		run: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		},
	}, nil
}

func (s *launchdDaemonService) Start() (userDaemonState, error) {
	return s.start(false)
}

func (s *launchdDaemonService) Restart() (userDaemonState, error) {
	return s.start(true)
}

func (s *launchdDaemonService) start(force bool) (userDaemonState, error) {
	state, err := s.Status()
	if err != nil {
		return state, err
	}
	if !force && state.Running && state.Version == s.version && state.Executable == s.executable {
		state.AlreadyRunning = true
		return state, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.plistPath()), 0o700); err != nil {
		return state, err
	}
	if err := os.MkdirAll(s.configDir(), 0o700); err != nil {
		return state, err
	}
	if err := writeDaemonFile(s.plistPath(), []byte(s.plist()), 0o600); err != nil {
		return state, err
	}
	metadata := userDaemonMetadata{SchemaVersion: userDaemonSchemaVersion, Version: s.version, Executable: s.executable, UpdatedAt: s.currentTime()}
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return state, err
	}
	if err := writeDaemonFile(s.metadataPath(), body, 0o600); err != nil {
		return state, err
	}
	domainName := fmt.Sprintf("gui/%d", s.uid)
	_, _ = s.run("launchctl", "bootout", domainName+"/"+userDaemonLabel)
	if output, runErr := s.run("launchctl", "bootstrap", domainName, s.plistPath()); runErr != nil {
		return state, fmt.Errorf("launchctl bootstrap: %w: %s", runErr, strings.TrimSpace(string(output)))
	}
	if output, runErr := s.run("launchctl", "kickstart", "-k", domainName+"/"+userDaemonLabel); runErr != nil {
		return state, fmt.Errorf("launchctl kickstart: %w: %s", runErr, strings.TrimSpace(string(output)))
	}
	state, err = s.Status()
	if err == nil && !state.Running {
		return state, domain.Conflict("DAEMON_START_INCOMPLETE", "ContentCloud Daemon 已注册但未进入运行状态")
	}
	return state, err
}

func (s *launchdDaemonService) Stop() (userDaemonState, error) {
	domainName := fmt.Sprintf("gui/%d", s.uid)
	if output, err := s.run("launchctl", "bootout", domainName+"/"+userDaemonLabel); err != nil {
		state, statusErr := s.Status()
		if statusErr == nil && !state.Running {
			return state, nil
		}
		return state, fmt.Errorf("launchctl bootout: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return s.Status()
}

func (s *launchdDaemonService) Status() (userDaemonState, error) {
	state := userDaemonState{
		SchemaVersion: userDaemonSchemaVersion, Supported: true, PlistPath: s.plistPath(),
		LogPath: s.logPath(), ErrorLogPath: s.logPath(),
	}
	if info, err := os.Stat(s.plistPath()); err == nil {
		state.Installed = info.Mode().IsRegular()
	} else if !errors.Is(err, os.ErrNotExist) {
		return state, err
	}
	if body, err := os.ReadFile(s.metadataPath()); err == nil {
		var metadata userDaemonMetadata
		if json.Unmarshal(body, &metadata) == nil && metadata.SchemaVersion == userDaemonSchemaVersion {
			state.Version = metadata.Version
			state.Executable = metadata.Executable
			state.UpdatedAt = &metadata.UpdatedAt
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return state, err
	}
	if runtimeState, runtimeErr := loadDaemonRuntimeState(); runtimeErr == nil {
		state.Runtime = runtimeState
	}
	state.PendingReports, state.DeadLetters, _ = daemonJournalCounts()
	output, err := s.run("launchctl", "print", fmt.Sprintf("gui/%d/%s", s.uid, userDaemonLabel))
	if err != nil {
		return state, nil
	}
	state.PID, state.Running = parseLaunchdStatus(output)
	return state, nil
}

func (s *launchdDaemonService) Uninstall() error {
	if _, err := s.Stop(); err != nil {
		return err
	}
	for _, path := range []string{s.plistPath(), s.metadataPath()} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *launchdDaemonService) configDir() string {
	return filepath.Join(s.home, "Library", "Application Support", "ContentCloud")
}

func (s *launchdDaemonService) plistPath() string {
	return filepath.Join(s.home, "Library", "LaunchAgents", userDaemonLabel+".plist")
}

func (s *launchdDaemonService) metadataPath() string {
	return filepath.Join(s.configDir(), "daemon.json")
}

func (s *launchdDaemonService) logPath() string {
	return filepath.Join(s.configDir(), "daemon.log")
}

func (s *launchdDaemonService) errorLogPath() string {
	return filepath.Join(s.configDir(), "daemon-error.log")
}

func (s *launchdDaemonService) currentTime() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func (s *launchdDaemonService) plist() string {
	environment := make([]string, 0, len(s.environment))
	keys := make([]string, 0, len(s.environment))
	for key := range s.environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		environment = append(environment, "<key>"+html.EscapeString(key)+"</key><string>"+html.EscapeString(s.environment[key])+"</string>")
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>` + userDaemonLabel + `</string>
<key>ProgramArguments</key><array><string>` + html.EscapeString(s.executable) + `</string><string>daemon</string><string>run</string><string>--log-file</string><string>` + html.EscapeString(s.logPath()) + `</string></array>
<key>EnvironmentVariables</key><dict>` + strings.Join(environment, "") + `</dict>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ProcessType</key><string>Background</string><key>ThrottleInterval</key><integer>5</integer>
<key>StandardOutPath</key><string>/dev/null</string>
<key>StandardErrorPath</key><string>/dev/null</string>
</dict></plist>`
}

func daemonLaunchEnvironment(home string) map[string]string {
	allowed := []string{
		"PATH", "LANG", "LC_ALL", "TMPDIR", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "XDG_CONFIG_HOME",
		"SSL_CERT_FILE", "SSL_CERT_DIR", "AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION", "AWS_CONFIG_FILE",
		"AWS_SHARED_CREDENTIALS_FILE", "GOOGLE_APPLICATION_CREDENTIALS", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX",
	}
	values := map[string]string{"HOME": home}
	for _, key := range allowed {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			values[key] = value
		}
	}
	if values["PATH"] == "" {
		values["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}
	return values
}

func writeDaemonFile(path string, body []byte, mode os.FileMode) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, body, mode); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func parseLaunchdStatus(body []byte) (int, bool) {
	pid := 0
	running := false
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pid = ") {
			pid, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "pid = ")))
		}
		if line == "state = running" {
			running = true
		}
	}
	return pid, running || pid > 0
}

func uninstallUserDaemon() error {
	service, err := newUserDaemonService()
	if err != nil {
		return err
	}
	return service.Uninstall()
}
