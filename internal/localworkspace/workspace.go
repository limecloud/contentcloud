package localworkspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/contracts"
	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/capabilityrouting"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	builtinskills "github.com/limecloud/contentcloud/plugins/contentcloud-video-production/skills"
	"gopkg.in/yaml.v3"
)

const (
	WorkspaceSchemaVersion    = "contentcloud.workspace/3.0"
	TemplateLockSchemaVersion = "contentcloud.template-lock/3.0"
	SyncStateSchemaVersion    = "contentcloud.sync-state/3.0"
	TemplateID                = "workspace_marketing_agent"
	TemplateVersion           = "3.0.0"
	TemplateDigest            = "sha256:abaa8a271bfc4efe2f29e199b761bd2b040867a0dd1e34710babde407f3ce54e"
	LayoutVersion             = 3
)

func CurrentTemplateRef() environment.WorkspaceTemplateRef {
	return environment.WorkspaceTemplateRef{ID: TemplateID, Version: TemplateVersion, Digest: TemplateDigest}
}

type Binding struct {
	SchemaVersion     string    `json:"schema_version" yaml:"schema_version"`
	WorkspaceID       string    `json:"workspace_id" yaml:"workspace_id"`
	ProjectID         string    `json:"project_id" yaml:"project_id"`
	LayoutVersion     int       `json:"layout_version" yaml:"layout_version"`
	ContextVersionID  string    `json:"context_version_id,omitempty" yaml:"context_version_id,omitempty"`
	EnvironmentDigest string    `json:"environment_digest,omitempty" yaml:"environment_digest,omitempty"`
	DeviceID          string    `json:"device_id,omitempty" yaml:"device_id,omitempty"`
	ServerURL         string    `json:"server_url,omitempty" yaml:"server_url,omitempty"`
	CreatedAt         time.Time `json:"created_at" yaml:"created_at"`
}

type ManagedFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   string `json:"mode"`
}

type InstalledComponent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256,omitempty"`
}

type TemplateLock struct {
	SchemaVersion   string               `json:"schema_version"`
	TemplateID      string               `json:"template_id"`
	TemplateVersion string               `json:"template_version"`
	CLIVersion      string               `json:"cli_version"`
	Targets         []string             `json:"targets"`
	Files           []ManagedFile        `json:"files"`
	Skills          []InstalledComponent `json:"skills"`
	MCPServers      []InstalledComponent `json:"mcp_servers"`
	InstalledAt     time.Time            `json:"installed_at"`
}

type SyncState struct {
	SchemaVersion      string                         `json:"schema_version"`
	Published          map[string]PublishedCheckpoint `json:"published"`
	PublishCursor      string                         `json:"publish_cursor,omitempty"`
	FeedbackCursor     string                         `json:"feedback_cursor,omitempty"`
	DecisionCursor     string                         `json:"decision_cursor,omitempty"`
	ApprovedCursor     string                         `json:"approved_cursor,omitempty"`
	ApprovedSnapshotID string                         `json:"approved_snapshot_id,omitempty"`
	LastPulledAt       *time.Time                     `json:"last_pulled_at,omitempty"`
	UpdatedAt          time.Time                      `json:"updated_at"`
}

type PublishedCheckpoint struct {
	SubmissionRevisionID string    `json:"submission_revision_id"`
	ContentHash          string    `json:"content_hash"`
	PublishedAt          time.Time `json:"published_at"`
}

type FileAction struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Mode   string `json:"mode,omitempty"`
}

type InitPlan struct {
	Root        string       `json:"root"`
	State       string       `json:"state"`
	Targets     []string     `json:"targets"`
	Directories []string     `json:"directories"`
	Files       []FileAction `json:"files"`
	Conflicts   []string     `json:"conflicts"`
	WouldUpload bool         `json:"would_upload"`
	WouldDaemon bool         `json:"would_enable_daemon"`
}

type InitOptions struct {
	Root              string
	WorkspaceID       string
	ProjectID         string
	DeviceID          string
	ServerURL         string
	CLIVersion        string
	Target            string
	EnvironmentDigest string
	Now               time.Time
}

type Status struct {
	Root                 string       `json:"root"`
	Initialized          bool         `json:"initialized"`
	Binding              Binding      `json:"binding"`
	Template             TemplateLock `json:"template"`
	Sync                 SyncState    `json:"sync"`
	SourceCount          int          `json:"source_count"`
	PendingFeedbackCount int          `json:"pending_feedback_count"`
	PendingDecisionCount int          `json:"pending_decision_count"`
	ModifiedManagedFiles []string     `json:"modified_managed_files"`
	MissingManagedFiles  []string     `json:"missing_managed_files"`
	AutomationEnabled    bool         `json:"automation_enabled"`
}

type Check struct {
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	Message  string `json:"message"`
}

type DoctorReport struct {
	OK     bool             `json:"ok"`
	Root   string           `json:"root"`
	Checks map[string]Check `json:"checks"`
}

type templateFile struct {
	path string
	mode string
	body []byte
}

func Plan(root, target string) (InitPlan, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return InitPlan{}, err
	}
	targets, err := targets(target)
	if err != nil {
		return InitPlan{}, err
	}
	state, conflicts, err := inspectTarget(absolute)
	if err != nil {
		return InitPlan{}, err
	}
	files, dirs, err := template(targets)
	if err != nil {
		return InitPlan{}, err
	}
	actions := make([]FileAction, 0, len(files)+3)
	for _, file := range files {
		actions = append(actions, FileAction{Path: file.path, Action: "create", Mode: file.mode})
	}
	for _, path := range []string{".contentcloud/workspace.yaml", ".contentcloud/template.lock", ".contentcloud/sync-state.json"} {
		actions = append(actions, FileAction{Path: path, Action: "create", Mode: "local_state"})
	}
	if state == "workspace" {
		for i := range actions {
			actions[i].Action = "skip"
		}
	}
	return InitPlan{Root: absolute, State: state, Targets: targets, Directories: dirs, Files: actions, Conflicts: conflicts, WouldUpload: false, WouldDaemon: false}, nil
}

func Initialize(options InitOptions) (Status, error) {
	plan, err := Plan(options.Root, options.Target)
	if err != nil {
		return Status{}, err
	}
	if plan.State == "workspace" {
		return LoadStatus(plan.Root)
	}
	if plan.State == "non_empty" {
		return Status{}, conflictError(plan.Conflicts)
	}
	if strings.TrimSpace(options.ProjectID) == "" {
		return Status{}, domain.Invalid("PROJECT_ID_REQUIRED", "初始化工作区需要项目 ID")
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	} else {
		options.Now = options.Now.UTC()
	}
	if err := os.MkdirAll(plan.Root, 0o700); err != nil {
		return Status{}, err
	}
	for _, dir := range plan.Directories {
		if err := os.MkdirAll(filepath.Join(plan.Root, filepath.FromSlash(dir)), 0o700); err != nil {
			return Status{}, err
		}
	}
	files, _, err := templateWithCLIVersion(plan.Targets, options.CLIVersion)
	if err != nil {
		return Status{}, err
	}
	managed := make([]ManagedFile, 0, len(files))
	for _, file := range files {
		path := filepath.Join(plan.Root, filepath.FromSlash(file.path))
		if err := writeNewFile(path, file.body); err != nil {
			return Status{}, err
		}
		managed = append(managed, ManagedFile{Path: file.path, SHA256: digest(file.body), Mode: file.mode})
	}
	skillComponents := make([]InstalledComponent, 0, len(builtinskills.Names()))
	for _, name := range builtinskills.Names() {
		body, err := builtinskills.Read(name, "SKILL.md")
		if err != nil {
			return Status{}, err
		}
		skillComponents = append(skillComponents, InstalledComponent{Name: name, Version: options.CLIVersion, SHA256: digest(body)})
	}
	workspaceID := options.WorkspaceID
	if workspaceID == "" {
		workspaceID = domain.NewID()
	}
	lock := TemplateLock{SchemaVersion: TemplateLockSchemaVersion, TemplateID: TemplateID, TemplateVersion: TemplateVersion, CLIVersion: options.CLIVersion, Targets: plan.Targets, Files: managed, Skills: skillComponents, MCPServers: []InstalledComponent{{Name: "contentcloud-local", Version: options.CLIVersion}}, InstalledAt: options.Now}
	environmentDigest := strings.TrimSpace(options.EnvironmentDigest)
	if environmentDigest == "" {
		hash, hashErr := domain.CanonicalHash(lock)
		if hashErr != nil {
			return Status{}, hashErr
		}
		environmentDigest = "sha256:" + hash
	}
	binding := Binding{SchemaVersion: WorkspaceSchemaVersion, WorkspaceID: workspaceID, ProjectID: options.ProjectID, LayoutVersion: LayoutVersion, EnvironmentDigest: environmentDigest, DeviceID: options.DeviceID, ServerURL: strings.TrimRight(options.ServerURL, "/"), CreatedAt: options.Now}
	syncState := SyncState{SchemaVersion: SyncStateSchemaVersion, Published: map[string]PublishedCheckpoint{}, UpdatedAt: options.Now}
	if err := writeYAML(filepath.Join(plan.Root, ".contentcloud", "workspace.yaml"), binding); err != nil {
		return Status{}, err
	}
	if err := writeJSON(filepath.Join(plan.Root, ".contentcloud", "template.lock"), lock); err != nil {
		return Status{}, err
	}
	if err := writeJSON(filepath.Join(plan.Root, ".contentcloud", "sync-state.json"), syncState); err != nil {
		return Status{}, err
	}
	return LoadStatus(plan.Root)
}

func FindRoot(start string) (string, error) {
	if strings.TrimSpace(start) == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	absolute, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		absolute = resolved
	}
	info, err := os.Stat(absolute)
	if err == nil && !info.IsDir() {
		absolute = filepath.Dir(absolute)
	}
	for dir := absolute; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".contentcloud", "workspace.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", domain.NotFound("Content Work OS 工作区")
}

func LoadStatus(root string) (Status, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return Status{}, err
	}
	var binding Binding
	if err := readYAML(filepath.Join(resolved, ".contentcloud", "workspace.yaml"), &binding); err != nil {
		return Status{}, fmt.Errorf("读取工作区绑定失败：%w", err)
	}
	if binding.SchemaVersion != WorkspaceSchemaVersion || binding.LayoutVersion != LayoutVersion || binding.WorkspaceID == "" || binding.ProjectID == "" {
		return Status{}, domain.Conflict("WORKSPACE_LAYOUT_UNSUPPORTED", "工作区不是受支持的 Content Work OS V3 布局")
	}
	var lock TemplateLock
	if err := readJSON(filepath.Join(resolved, ".contentcloud", "template.lock"), &lock); err != nil {
		return Status{}, fmt.Errorf("读取模板锁定信息失败：%w", err)
	}
	var syncState SyncState
	if err := readJSON(filepath.Join(resolved, ".contentcloud", "sync-state.json"), &syncState); err != nil {
		return Status{}, fmt.Errorf("读取同步状态失败：%w", err)
	}
	modified, missing := verifyManagedFiles(resolved, lock.Files, lock.CLIVersion)
	automationEnabled := false
	if environmentState, environmentErr := ReadEnvironmentClaim(resolved); environmentErr == nil {
		automationEnabled = environmentState.Manifest.Policies.AutomationEnabled
	}
	return Status{
		Root:                 resolved,
		Initialized:          true,
		Binding:              binding,
		Template:             lock,
		Sync:                 syncState,
		SourceCount:          sourceCount(resolved),
		PendingFeedbackCount: countFiles(filepath.Join(resolved, ".contentcloud", "inbox", "review-feedback")),
		PendingDecisionCount: countFiles(filepath.Join(resolved, ".contentcloud", "inbox", "decision-deltas")),
		ModifiedManagedFiles: modified,
		MissingManagedFiles:  missing,
		AutomationEnabled:    automationEnabled,
	}, nil
}

func Doctor(root string) (DoctorReport, error) {
	status, err := LoadStatus(root)
	if err != nil {
		return DoctorReport{}, err
	}
	skillsOK, skillsMessage := installedSkillsCheck(status.Root, status.Template)
	mcpOK, mcpMessage := installedMCPCheck(status.Root, status.Template)
	routingInspection, _ := InspectCapabilityRouting(status.Root)
	automationMessage := "已签名的环境清单未启用后台自动化"
	if status.AutomationEnabled {
		automationMessage = "已签名的环境清单已启用后台自动化"
	}
	checks := map[string]Check{
		"workspace_binding":  {OK: status.Binding.SchemaVersion == WorkspaceSchemaVersion && status.Binding.LayoutVersion == LayoutVersion && status.Binding.ProjectID != "" && status.Binding.WorkspaceID != "", Required: true, Message: "V3 项目与工作区绑定可读"},
		"workspace_writable": bindingWriteProbe(status.Root),
		"template_lock":      {OK: status.Template.TemplateVersion != "", Required: true, Message: "模板锁文件可读"},
		"managed_files":      {OK: len(status.ModifiedManagedFiles) == 0 && len(status.MissingManagedFiles) == 0, Required: true, Message: managedMessage(status)},
		"skills":             {OK: skillsOK, Required: true, Message: skillsMessage},
		"mcp":                {OK: mcpOK, Required: true, Message: mcpMessage},
		"capability_routing": {OK: routingInspection.Status == "current", Required: true, Message: "Content Work OS 能力路由受管块状态：" + routingInspection.Status},
		"automation":         {OK: true, Required: false, Message: automationMessage},
	}
	ok := true
	for _, check := range checks {
		if check.Required && !check.OK {
			ok = false
		}
	}
	return DoctorReport{OK: ok, Root: status.Root, Checks: checks}, nil
}

func DoctorWithEnvironment(root string, verifier *environment.Verifier, registryVerifier *environment.RegistryVerifier, now time.Time) (DoctorReport, error) {
	report, err := Doctor(root)
	if err != nil {
		return DoctorReport{}, err
	}
	report.Checks["environment"] = EnvironmentCheck(report.Root, verifier, registryVerifier, now)
	report.OK = requiredChecksOK(report.Checks)
	return report, nil
}

func requiredChecksOK(checks map[string]Check) bool {
	for _, check := range checks {
		if check.Required && !check.OK {
			return false
		}
	}
	return true
}

func ProjectBinding(root string) (Binding, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return Binding{}, err
	}
	var binding Binding
	if err := readYAML(filepath.Join(resolved, ".contentcloud", "workspace.yaml"), &binding); err != nil {
		return Binding{}, err
	}
	return binding, nil
}

func RecordPublished(root, submissionType, revisionID, contentHash string, now time.Time) error {
	resolved, state, err := loadSyncState(root)
	if err != nil {
		return err
	}
	if state.Published == nil {
		state.Published = map[string]PublishedCheckpoint{}
	}
	state.Published[submissionType] = PublishedCheckpoint{SubmissionRevisionID: revisionID, ContentHash: contentHash, PublishedAt: now.UTC()}
	state.PublishCursor = revisionID
	state.UpdatedAt = now.UTC()
	return replaceJSON(filepath.Join(resolved, ".contentcloud", "sync-state.json"), state, 0o600)
}

func StorePulledBundle(root, kind, id string, value any, now time.Time) (string, error) {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return "", domain.Invalid("PULL_BUNDLE_ID_INVALID", "拉取包 ID 无效")
	}
	resolved, state, err := loadSyncState(root)
	if err != nil {
		return "", err
	}
	var destination string
	switch kind {
	case "feedback":
		destination = filepath.Join(resolved, ".contentcloud", "inbox", "review-feedback", id+".json")
		state.FeedbackCursor = id
	case "decisions":
		destination = filepath.Join(resolved, ".contentcloud", "inbox", "decision-deltas", id+".json")
		state.DecisionCursor = id
	case "approved":
		destination = filepath.Join(resolved, ".contentcloud", "cache", "approved", id, "snapshot.json")
		state.ApprovedCursor = id
		state.ApprovedSnapshotID = id
	default:
		return "", domain.Invalid("PULL_KIND_INVALID", "pull 类型无效")
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	body = append(body, '\n')
	if existing, err := os.ReadFile(destination); err == nil {
		if !bytes.Equal(existing, body) {
			return "", domain.Conflict("PULL_IMMUTABLE_CONFLICT", "本地已有相同 ID 但内容不同的拉取包")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	} else {
		mode := fs.FileMode(0o400)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(destination, body, mode); err != nil {
			return "", err
		}
	}
	state.UpdatedAt = now.UTC()
	pulledAt := now.UTC()
	state.LastPulledAt = &pulledAt
	if err := replaceJSON(filepath.Join(resolved, ".contentcloud", "sync-state.json"), state, 0o600); err != nil {
		return "", err
	}
	return destination, nil
}

func loadSyncState(root string) (string, SyncState, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return "", SyncState{}, err
	}
	var state SyncState
	if err := readJSON(filepath.Join(resolved, ".contentcloud", "sync-state.json"), &state); err != nil {
		return "", state, err
	}
	return resolved, state, nil
}

func replaceJSON(path string, value any, mode fs.FileMode) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return replaceFile(path, body, mode)
}

func replaceFile(path string, body []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".contentcloud-*.tmp")
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
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

const defaultMCPCLIVersion = "0.27.0"

func template(targets []string) ([]templateFile, []string, error) {
	return templateWithCLIVersion(targets, defaultMCPCLIVersion)
}

func templateWithCLIVersion(targets []string, cliVersion string) ([]templateFile, []string, error) {
	cliVersion = normalizeMCPCLIVersion(cliVersion)
	dirs := []string{
		".contentcloud/inbox/assignments", ".contentcloud/inbox/review-feedback", ".contentcloud/inbox/decisions", ".contentcloud/cache/approved", ".contentcloud/cache/schemas", ".contentcloud/cache/memory", ".contentcloud/locks/runs", ".contentcloud/locks", ".contentcloud/mcp", ".contentcloud/tmp",
		"00-inbox/ideas", "00-inbox/unregistered-sources",
		"10-context/intents",
		"20-sources/originals", "20-sources/extracts",
		"30-knowledge/schema", "30-knowledge/pages/sources", "30-knowledge/pages/evidence", "30-knowledge/pages/facts", "30-knowledge/pages/claims", "30-knowledge/pages/assets", "30-knowledge/pages/rights", "30-knowledge/pages/conflicts", "30-knowledge/pages/domain", "30-knowledge/imports", "30-knowledge/packs",
		"40-work/queues", "40-work/runs", "40-work/handoffs", "40-work/memory/records",
		"50-production/plans", "50-production/campaigns", "50-production/strategies", "50-production/offers", "50-production/briefs", "50-production/batches", "50-production/scripts", "50-production/media", "50-production/media/storyboards",
		"60-delivery/packages", "60-delivery/exports",
		"70-results/imports", "70-results/observations", "70-results/learnings",
		"90-archive", "workflows", "scripts",
	}
	files := []templateFile{
		{path: "AGENTS.md", mode: "managed_block", body: []byte(capabilityrouting.ManagedBlock())},
		{path: "00-inbox/.gitignore", mode: "managed_replace", body: []byte("unregistered-sources/*\n!unregistered-sources/.gitkeep\n")},
		{path: "00-inbox/unregistered-sources/.gitkeep", mode: "managed_replace", body: []byte{}},
		{path: ".contentcloud/cache/memory/.gitignore", mode: "managed_replace", body: []byte("*\n!.gitignore\n")},
		{path: "10-context/client.yaml", mode: "seed_once", body: []byte(contextClientYAML)},
		{path: "10-context/project-brief.yaml", mode: "seed_once", body: []byte(contextProjectBriefYAML)},
		{path: "10-context/project.yaml", mode: "seed_once", body: []byte(contextProjectYAML)},
		{path: "10-context/methodology.yaml", mode: "seed_once", body: []byte(contextMethodologyYAML)},
		{path: "10-context/service-plan.yaml", mode: "seed_once", body: []byte(contextServicePlanYAML)},
		{path: "20-sources/registry.yaml", mode: "seed_once", body: []byte(sourceRegistryYAML)},
		{path: "30-knowledge/schema/workspace-3.0.schema.json", mode: "managed_replace", body: contracts.WorkspaceV3Schema},
		{path: "30-knowledge/schema/source-registry-3.0.schema.json", mode: "managed_replace", body: contracts.SourceRegistryV3Schema},
		{path: "30-knowledge/schema/knowledge-page-3.0.schema.json", mode: "managed_replace", body: contracts.KnowledgePageV3Schema},
		{path: "30-knowledge/schema/knowledge-pack-3.0.schema.json", mode: "managed_replace", body: contracts.KnowledgePackV3Schema},
		{path: "30-knowledge/schema/local-run-3.0.schema.json", mode: "managed_replace", body: contracts.LocalRunV3Schema},
		{path: "30-knowledge/schema/handoff-1.0.schema.json", mode: "managed_replace", body: contracts.HandoffV1Schema},
		{path: "30-knowledge/schema/content-batch-3.0.schema.json", mode: "managed_replace", body: contracts.ContentBatchV3Schema},
		{path: "30-knowledge/schema/submission-bundle-3.0.schema.json", mode: "managed_replace", body: contracts.SubmissionBundleV3Schema},
		{path: "30-knowledge/schema/audience-taxonomy-1.0.schema.json", mode: "managed_replace", body: contracts.AudienceTaxonomyV1Schema},
		{path: "30-knowledge/schema/audience-strategy-1.0.schema.json", mode: "managed_replace", body: contracts.AudienceStrategyV1Schema},
		{path: "30-knowledge/schema/commerce-offer-1.0.schema.json", mode: "managed_replace", body: contracts.CommerceOfferV1Schema},
		{path: "30-knowledge/schema/douyin-commerce-validation-1.0.schema.json", mode: "managed_replace", body: contracts.DouyinCommerceValidationV1Schema},
		{path: "30-knowledge/schema/storyboard-package-1.0.schema.json", mode: "managed_replace", body: contracts.StoryboardPackageV1Schema},
		{path: "30-knowledge/schema/seedance-prompt-package-1.0.schema.json", mode: "managed_replace", body: contracts.SeedancePromptPackageV1Schema},
		{path: "30-knowledge/schema/published-creative-binding-1.0.schema.json", mode: "managed_replace", body: contracts.PublishedCreativeBindingV1Schema},
		{path: "30-knowledge/index.md", mode: "generated", body: []byte(knowledgeIndexMarkdown)},
		{path: "40-work/focus.md", mode: "seed_once", body: []byte("# 当前焦点\n\n")},
		{path: "40-work/queues/review.md", mode: "seed_once", body: []byte("# 本地审核队列\n\n")},
		{path: "40-work/queues/decisions.md", mode: "seed_once", body: []byte("# 待决策项\n\n")},
		{path: "40-work/queues/gaps.md", mode: "seed_once", body: []byte("# 知识缺口\n\n")},
		{path: "workflows/knowledge-to-content.md", mode: "managed_replace", body: []byte(workflowReadme)},
		{path: ".contentcloud/mcp/contentcloud-local.json", mode: "managed_replace", body: []byte(mcpDescriptor(cliVersion))},
	}
	for _, target := range targets {
		switch target {
		case "codex":
			files = append(files, templateFile{path: ".codex/config.toml", mode: "managed_merge", body: []byte(codexMCPConfig(cliVersion))})
		}
	}
	sort.Strings(dirs)
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, dirs, nil
}

func targets(value string) ([]string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "codex-plugin":
		return []string{"codex-plugin"}, nil
	case "none":
		return []string{}, nil
	}
	client, err := agentadapter.RequireCapability(normalized, agentadapter.CapabilityWorkspaceBootstrap)
	if err != nil {
		return nil, err
	}
	return []string{string(client.ID)}, nil
}

func inspectTarget(root string) (string, []string, error) {
	if _, err := os.Stat(filepath.Join(root, ".contentcloud", "workspace.yaml")); err == nil {
		return "workspace", nil, nil
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return "missing", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	if len(entries) == 0 {
		return "empty", nil, nil
	}
	conflicts := make([]string, 0, len(entries))
	for _, entry := range entries {
		conflicts = append(conflicts, entry.Name())
	}
	sort.Strings(conflicts)
	return "non_empty", conflicts, nil
}

func conflictError(paths []string) error {
	shown := paths
	if len(shown) > 8 {
		shown = shown[:8]
	}
	err := domain.Conflict("WORKSPACE_DIRECTORY_NOT_EMPTY", "目标目录非空，且不是 Content Work OS 工作区")
	err.Hint = "请选择空目录，或先确认并整理冲突文件：" + strings.Join(shown, ", ")
	err.Details = map[string]any{"conflicts": paths}
	return err
}

func writeNewFile(path string, body []byte) error {
	if _, err := os.Stat(path); err == nil {
		return domain.Conflict("WORKSPACE_FILE_EXISTS", "初始化拒绝覆盖已有文件："+path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeNewFile(path, body)
}

func writeYAML(path string, value any) error {
	body, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return writeNewFile(path, body)
}

func readYAML(path string, value any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("解析 %s 失败：%w", path, err)
	}
	return nil
}

func readJSON(path string, value any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, value); err != nil {
		return fmt.Errorf("解析 %s 失败：%w", path, err)
	}
	return nil
}

func readStrictJSON(path string, value any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return strictUnmarshal(body, value)
}

func strictUnmarshal(body []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON 只能包含一个顶层值")
	}
	return nil
}

func verifyManagedFiles(root string, files []ManagedFile, cliVersion string) ([]string, []string) {
	modified := []string{}
	missing := []string{}
	for _, file := range files {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.Path)))
		if errors.Is(err, os.ErrNotExist) {
			missing = append(missing, file.Path)
			continue
		}
		if err != nil {
			modified = append(modified, file.Path)
			continue
		}
		if file.Mode == "managed_block" && file.Path == "AGENTS.md" {
			if capabilityrouting.Inspect(string(body)).Status != "current" {
				modified = append(modified, file.Path)
			}
			continue
		}
		if file.Mode == "seed_once" || file.Mode == "generated" {
			continue
		}
		if file.Mode == "managed_merge" {
			files, _, err := templateWithCLIVersion([]string{"codex"}, cliVersion)
			if err == nil {
				for _, expected := range files {
					if expected.path == file.Path && !bytes.Contains(body, expected.body) {
						modified = append(modified, file.Path)
					}
				}
			}
			continue
		}
		if digest(body) != file.SHA256 {
			modified = append(modified, file.Path)
		}
	}
	sort.Strings(modified)
	sort.Strings(missing)
	return modified, missing
}

func InspectCapabilityRouting(root string) (capabilityrouting.Inspection, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return capabilityrouting.Inspection{}, err
	}
	body, err := os.ReadFile(filepath.Join(resolved, "AGENTS.md"))
	if errors.Is(err, os.ErrNotExist) {
		return capabilityrouting.Inspection{Status: "missing"}, nil
	}
	if err != nil {
		return capabilityrouting.Inspection{}, err
	}
	return capabilityrouting.Inspect(string(body)), nil
}

func UpdateCapabilityRouting(root string) (capabilityrouting.Inspection, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return capabilityrouting.Inspection{}, err
	}
	path := filepath.Join(resolved, "AGENTS.md")
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		body = nil
	} else if err != nil {
		return capabilityrouting.Inspection{}, err
	}
	updated, err := capabilityrouting.UpdateManagedBlock(string(body))
	if err != nil {
		return capabilityrouting.Inspection{}, err
	}
	if err := replaceFile(path, []byte(updated), 0o600); err != nil {
		return capabilityrouting.Inspection{}, err
	}
	return capabilityrouting.Inspect(updated), nil
}

func installedSkillsCheck(root string, lock TemplateLock) (bool, string) {
	_ = root
	locked := map[string]InstalledComponent{}
	for _, skill := range lock.Skills {
		locked[skill.Name] = skill
	}
	for _, name := range builtinskills.Names() {
		body, err := builtinskills.Read(name, "SKILL.md")
		component, ok := locked[name]
		if err != nil || !ok || component.SHA256 != digest(body) {
			return false, "模板锁中的插件技能摘要不完整或不一致"
		}
	}
	if hasTarget(lock.Targets, "codex-plugin") {
		return true, "技能由 Codex 插件提供，工作区不复制技能源码"
	}
	return true, "插件技能的版本摘要完整；宿主安装状态由初始化检查确认"
}

func installedMCPCheck(root string, lock TemplateLock) (bool, string) {
	if !fileExists(filepath.Join(root, ".contentcloud", "mcp", "contentcloud-local.json")) {
		return false, "Content Work OS MCP 审计描述缺失"
	}
	if hasTarget(lock.Targets, "codex") && !fileExists(filepath.Join(root, ".codex", "config.toml")) {
		return false, "Codex 项目级 MCP 配置缺失"
	}
	if hasTarget(lock.Targets, "codex-plugin") {
		return true, "contentcloud-local MCP 由 Codex 插件提供，工作区审计描述完整"
	}
	return true, "contentcloud-local 项目级 MCP 配置完整"
}

func bindingWriteProbe(root string) Check {
	directory := filepath.Join(root, ".contentcloud", "tmp")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Check{OK: false, Required: true, Message: "无法创建 Content Work OS 临时目录：" + err.Error()}
	}
	file, err := os.CreateTemp(directory, "doctor-write-*.tmp")
	if err != nil {
		return Check{OK: false, Required: true, Message: "工作区不可写，请在 Codex 中信任并打开项目根目录：" + err.Error()}
	}
	path := file.Name()
	destination := path + ".committed"
	defer os.Remove(path)
	defer os.Remove(destination)
	if _, err := file.WriteString("contentcloud-v3-write-probe\n"); err != nil {
		_ = file.Close()
		return Check{OK: false, Required: true, Message: "工作区写入探针失败：" + err.Error()}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return Check{OK: false, Required: true, Message: "工作区写入探针无法落盘：" + err.Error()}
	}
	if err := file.Close(); err != nil {
		return Check{OK: false, Required: true, Message: "工作区写入探针关闭失败：" + err.Error()}
	}
	if err := os.Rename(path, destination); err != nil {
		return Check{OK: false, Required: true, Message: "工作区原子替换探针失败：" + err.Error()}
	}
	body, err := os.ReadFile(destination)
	if err != nil || string(body) != "contentcloud-v3-write-probe\n" {
		return Check{OK: false, Required: true, Message: "工作区原子替换结果无法校验"}
	}
	return Check{OK: true, Required: true, Message: ".contentcloud 原子写入探针通过"}
}

func sourceCount(root string) int {
	registry, err := loadSourceRegistry(root)
	if err != nil {
		return 0
	}
	return len(registry.Sources)
}

func hasTarget(targets []string, expected string) bool {
	for _, target := range targets {
		if target == expected {
			return true
		}
	}
	return false
}

func countFiles(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err == nil && entry != nil && !entry.IsDir() && entry.Name() != ".gitkeep" {
			count++
		}
		return nil
	})
	return count
}

func managedMessage(status Status) string {
	if len(status.MissingManagedFiles) > 0 {
		return fmt.Sprintf("缺少 %d 个受管文件", len(status.MissingManagedFiles))
	}
	if len(status.ModifiedManagedFiles) > 0 {
		return fmt.Sprintf("有 %d 个受管文件已修改", len(status.ModifiedManagedFiles))
	}
	return "受管文件完整性校验通过"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

const workflowReadme = `# 知识到剧本

1. 登记 20-sources/ 原件并生成可定位 Evidence。
2. 在独立 LocalRun 中把候选写入 30-knowledge/pages/**/*.md。
3. 运行知识 lint、15 维诊断和七层 KnowledgePack 构建。
4. 显式 preflight 并确认后发布 knowledge Revision。
5. 仅在用户要求刷新时拉取 ApprovedSnapshot，后续对话优先读取本地不可变缓存。
6. 基于 Intent、eligible/blocked 集合生成 ContentBatch；blocked 批次可评审但不可交付。
`

const contextClientYAML = `schema_version: contentcloud.client-context/3.0
client_id: ""
brand_refs: []
product_refs: []
owners: []
`

const contextProjectBriefYAML = `schema_version: contentcloud.project-brief/1.0
status: draft
client: ""
brand: ""
product_or_service: ""
objective: ""
channels: []
audience: ""
material_refs: []
notes: ""
`

const contextProjectYAML = `schema_version: contentcloud.project-context/3.0
stage: initiation
gate: in_progress
objectives: []
constraints: []
`

const contextMethodologyYAML = `schema_version: contentcloud.methodology-context/3.0
methodology_version_id: ""
dimensions: []
`

const contextServicePlanYAML = `schema_version: contentcloud.service-plan/3.0
phase: initiation
roles: []
gates: []
deliverables: []
`

const sourceRegistryYAML = `schema_version: contentcloud.source-registry/3.0
sources: []
`

const knowledgeIndexMarkdown = `# 可信知识索引

本文件是可重建投影。唯一可编辑知识事实源位于 30-knowledge/pages/。
`

func mcpDescriptor(cliVersion string) string {
	return fmt.Sprintf(`{
  "name": "contentcloud-local",
  "version": "3.0",
  "transport": "stdio",
  "command": "npx",
  "args": ["--yes", "@limecloud/contentcloud@%s", "mcp", "serve"],
  "network_boundary": "all cloud communication is delegated to the contentcloud CLI"
}
`, cliVersion)
}

func codexMCPConfig(cliVersion string) string {
	return fmt.Sprintf(`[mcp_servers.contentcloud-local]
command = "npx"
args = ["--yes", "@limecloud/contentcloud@%s", "mcp", "serve"]
`, cliVersion)
}

func normalizeMCPCLIVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultMCPCLIVersion
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '-' || char == '_' {
			continue
		}
		return defaultMCPCLIVersion
	}
	return value
}
