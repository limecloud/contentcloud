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
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/contracts"
	"github.com/limecloud/contentcloud/internal/domain"
	builtinskills "github.com/limecloud/contentcloud/skills"
)

const (
	SchemaVersion   = "2.0"
	TemplateID      = "workspace_marketing_video"
	TemplateVersion = "2.0.0"
)

type Binding struct {
	SchemaVersion string    `json:"schema_version"`
	WorkspaceID   string    `json:"workspace_id"`
	ProjectID     string    `json:"project_id"`
	DeviceID      string    `json:"device_id,omitempty"`
	ServerURL     string    `json:"server_url"`
	InitializedAt time.Time `json:"initialized_at"`
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
	Root        string
	WorkspaceID string
	ProjectID   string
	DeviceID    string
	ServerURL   string
	CLIVersion  string
	Target      string
	Now         time.Time
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
	for _, path := range []string{".contentcloud/project.yaml", ".contentcloud/template.lock", ".contentcloud/sync-state.json"} {
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
	files, _, err := template(plan.Targets)
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
	binding := Binding{SchemaVersion: SchemaVersion, WorkspaceID: workspaceID, ProjectID: options.ProjectID, DeviceID: options.DeviceID, ServerURL: strings.TrimRight(options.ServerURL, "/"), InitializedAt: options.Now}
	lock := TemplateLock{SchemaVersion: SchemaVersion, TemplateID: TemplateID, TemplateVersion: TemplateVersion, CLIVersion: options.CLIVersion, Targets: plan.Targets, Files: managed, Skills: skillComponents, MCPServers: []InstalledComponent{{Name: "contentcloud-local", Version: options.CLIVersion}}, InstalledAt: options.Now}
	syncState := SyncState{SchemaVersion: SchemaVersion, Published: map[string]PublishedCheckpoint{}, UpdatedAt: options.Now}
	if err := writeJSON(filepath.Join(plan.Root, ".contentcloud", "project.yaml"), binding); err != nil {
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
	info, err := os.Stat(absolute)
	if err == nil && !info.IsDir() {
		absolute = filepath.Dir(absolute)
	}
	for dir := absolute; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".contentcloud", "project.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", domain.NotFound("ContentCloud 工作区")
}

func LoadStatus(root string) (Status, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return Status{}, err
	}
	var binding Binding
	if err := readJSON(filepath.Join(resolved, ".contentcloud", "project.yaml"), &binding); err != nil {
		return Status{}, fmt.Errorf("read workspace binding: %w", err)
	}
	var lock TemplateLock
	if err := readJSON(filepath.Join(resolved, ".contentcloud", "template.lock"), &lock); err != nil {
		return Status{}, fmt.Errorf("read template lock: %w", err)
	}
	var syncState SyncState
	if err := readJSON(filepath.Join(resolved, ".contentcloud", "sync-state.json"), &syncState); err != nil {
		return Status{}, fmt.Errorf("read sync state: %w", err)
	}
	modified, missing := verifyManagedFiles(resolved, lock.Files)
	return Status{
		Root:                 resolved,
		Initialized:          true,
		Binding:              binding,
		Template:             lock,
		Sync:                 syncState,
		SourceCount:          countFiles(filepath.Join(resolved, "raw", "inbox")),
		PendingFeedbackCount: countFiles(filepath.Join(resolved, ".contentcloud", "inbox", "review-feedback")),
		PendingDecisionCount: countFiles(filepath.Join(resolved, ".contentcloud", "inbox", "decision-deltas")),
		ModifiedManagedFiles: modified,
		MissingManagedFiles:  missing,
		AutomationEnabled:    false,
	}, nil
}

func Doctor(root string) (DoctorReport, error) {
	status, err := LoadStatus(root)
	if err != nil {
		return DoctorReport{}, err
	}
	checks := map[string]Check{
		"workspace_binding": {OK: status.Binding.ProjectID != "" && status.Binding.WorkspaceID != "", Required: true, Message: "项目与工作区绑定可读"},
		"template_lock":     {OK: status.Template.TemplateVersion != "", Required: true, Message: "模板锁文件可读"},
		"managed_files":     {OK: len(status.ModifiedManagedFiles) == 0 && len(status.MissingManagedFiles) == 0, Required: true, Message: managedMessage(status)},
		"skills":            {OK: installedSkillsOK(status.Root, status.Template), Required: true, Message: "项目级 Skills 已安装"},
		"mcp":               {OK: fileExists(filepath.Join(status.Root, ".contentcloud", "mcp", "contentcloud-local.json")), Required: true, Message: "contentcloud-local MCP 配置已安装"},
		"automation":        {OK: true, Required: false, Message: "后台 Automation Daemon 未启用（普通本地创作不需要）"},
	}
	ok := true
	for _, check := range checks {
		if check.Required && !check.OK {
			ok = false
		}
	}
	return DoctorReport{OK: ok, Root: status.Root, Checks: checks}, nil
}

func ProjectBinding(root string) (Binding, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return Binding{}, err
	}
	var binding Binding
	if err := readJSON(filepath.Join(resolved, ".contentcloud", "project.yaml"), &binding); err != nil {
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
		return "", domain.Invalid("PULL_BUNDLE_ID_INVALID", "pull bundle ID 无效")
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
			return "", domain.Conflict("PULL_IMMUTABLE_CONFLICT", "本地已有同 ID 但内容不同的 pull bundle")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	} else {
		mode := fs.FileMode(0o600)
		if kind == "approved" {
			mode = 0o400
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(destination, body, mode); err != nil {
			return "", err
		}
	}
	state.UpdatedAt = now.UTC()
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
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func template(targets []string) ([]templateFile, []string, error) {
	dirs := []string{
		".contentcloud/inbox/review-feedback", ".contentcloud/inbox/decision-deltas", ".contentcloud/cache/approved", ".contentcloud/skills", ".contentcloud/mcp",
		"methodology", "ontology/rules", "ontology/vocabularies", "schemas", "knowledge/index", "knowledge/sources", "knowledge/evidence", "knowledge/facts", "knowledge/claims", "knowledge/assets", "knowledge/rights", "knowledge/conflicts", "knowledge/packs",
		"raw/inbox", "work/runs", "workflows", "scripts", "outputs/briefs", "outputs/scripts", "outputs/storyboards", "outputs/reports", "outputs/delivery",
	}
	files := []templateFile{
		{path: "AGENTS.md", mode: "managed_merge", body: []byte(agentInstructions)},
		{path: "methodology/README.md", mode: "managed_merge", body: []byte(methodologyReadme)},
		{path: "ontology/classes.yaml", mode: "managed_replace", body: []byte(classesYAML)},
		{path: "ontology/properties.yaml", mode: "managed_replace", body: []byte(propertiesYAML)},
		{path: "raw/.gitignore", mode: "managed_replace", body: []byte("inbox/*\n!inbox/.gitkeep\n")},
		{path: "raw/inbox/.gitkeep", mode: "managed_replace", body: []byte{}},
		{path: "raw/source-registry.yaml", mode: "seed_once", body: []byte("{\n  \"schema_version\": \"2.0\",\n  \"sources\": []\n}\n")},
		{path: "schemas/knowledge-candidates-1.0.schema.json", mode: "managed_replace", body: contracts.KnowledgeCandidatesSchema},
		{path: "schemas/brief-2.0.schema.json", mode: "managed_replace", body: contracts.BriefV2Schema},
		{path: "schemas/creative-directions-2.0.schema.json", mode: "managed_replace", body: contracts.CreativeDirectionsV2Schema},
		{path: "schemas/script-package-2.0.schema.json", mode: "managed_replace", body: contracts.ScriptPackageV2Schema},
		{path: "work/current-focus.md", mode: "seed_once", body: []byte("# 当前焦点\n\n")},
		{path: "work/conflicts.md", mode: "seed_once", body: []byte("# 待解决冲突\n\n")},
		{path: "work/knowledge-gaps.md", mode: "seed_once", body: []byte("# 知识缺口\n\n")},
		{path: "work/review-queue.md", mode: "seed_once", body: []byte("# 本地审核队列\n\n")},
		{path: "workflows/knowledge-to-script.md", mode: "managed_merge", body: []byte(workflowReadme)},
		{path: ".contentcloud/mcp/contentcloud-local.json", mode: "managed_replace", body: []byte(mcpDescriptor)},
	}
	for _, name := range builtinskills.Names() {
		skillFiles, err := builtinskills.Files(name)
		if err != nil {
			return nil, nil, err
		}
		for _, path := range skillFiles {
			body, err := builtinskills.Read(name, path)
			if err != nil {
				return nil, nil, err
			}
			files = append(files, templateFile{path: filepath.ToSlash(filepath.Join(".contentcloud", "skills", name, path)), mode: "managed_replace", body: body})
			for _, target := range targets {
				destination := agentSkillPath(target, name, path)
				files = append(files, templateFile{path: destination, mode: "managed_replace", body: body})
			}
		}
	}
	for _, target := range targets {
		switch target {
		case "codex":
			files = append(files, templateFile{path: ".codex/config.toml", mode: "managed_merge", body: []byte(codexMCPConfig)})
		case "claude":
			files = append(files, templateFile{path: ".mcp.json", mode: "managed_merge", body: []byte(claudeMCPConfig)})
		}
	}
	sort.Strings(dirs)
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, dirs, nil
}

func targets(value string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return []string{"codex", "claude"}, nil
	case "codex":
		return []string{"codex"}, nil
	case "claude":
		return []string{"claude"}, nil
	case "none":
		return []string{}, nil
	default:
		return nil, domain.Invalid("WORKSPACE_TARGET_INVALID", "--target 必须为 codex、claude、all 或 none")
	}
}

func inspectTarget(root string) (string, []string, error) {
	if _, err := os.Stat(filepath.Join(root, ".contentcloud", "project.yaml")); err == nil {
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
	err := domain.Conflict("WORKSPACE_DIRECTORY_NOT_EMPTY", "目标目录非空且不是 ContentCloud 工作区")
	err.Hint = "请选择空目录，或先确认并整理冲突文件：" + strings.Join(shown, ", ")
	err.Details = map[string]any{"conflicts": paths}
	return err
}

func agentSkillPath(target, name, path string) string {
	if target == "claude" {
		return filepath.ToSlash(filepath.Join(".claude", "skills", name, path))
	}
	return filepath.ToSlash(filepath.Join(".agents", "skills", name, path))
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

func readJSON(path string, value any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, value); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
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

func verifyManagedFiles(root string, files []ManagedFile) ([]string, []string) {
	modified := []string{}
	missing := []string{}
	for _, file := range files {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.Path)))
		if errors.Is(err, os.ErrNotExist) {
			missing = append(missing, file.Path)
			continue
		}
		if err != nil || digest(body) != file.SHA256 {
			modified = append(modified, file.Path)
		}
	}
	sort.Strings(modified)
	sort.Strings(missing)
	return modified, missing
}

func installedSkillsOK(root string, lock TemplateLock) bool {
	for _, skill := range lock.Skills {
		body, err := os.ReadFile(filepath.Join(root, ".contentcloud", "skills", skill.Name, "SKILL.md"))
		if err != nil || digest(body) != skill.SHA256 {
			return false
		}
	}
	return len(lock.Skills) > 0
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

const agentInstructions = `# ContentCloud V2 项目工作区

- 原始资料和未发布草稿以本地文件为准；云端只接收显式 publish 的不可变检查点。
- 处理客户事实时先登记来源和证据，不得用模型常识补全产品事实、权利或合规结论。
- 生成营销视频剧本前必须完成知识校验、Brief 和引用检查。
- 任何 ContentCloud 服务端通信都必须通过 contentcloud CLI 或 contentcloud-local MCP。
- 不直接调用私有 HTTP、对象存储地址，不读取或输出本地凭据。
- 未经用户明确确认，不上传 raw/ 中的原始资料，不启动 Automation Daemon。
`

const methodologyReadme = `# 方法论

本目录保存项目采用的方法论和业务规则。客户事实应写入 knowledge/，当前任务状态应写入 work/，生成结果应写入 outputs/。
`

const workflowReadme = `# 知识到剧本

1. 用 contentcloud local source register/ingest 登记客户资料并生成可定位 EvidenceBundle。
2. 初始化 LocalRunContext，由本地 Agent Skill 从已接受证据生成 knowledge-candidates/1.0。
3. 用 contentcloud local knowledge import/lint/query/diagnose/pack 完成候选治理、15维诊断和七层知识包。
4. 用 contentcloud publish knowledge --dry-run 检查审核可见范围，再显式提交云端审核。
5. 拉取 ApprovedSnapshot 后，基于 eligible 知识完成策略和 Brief。
6. 生成带引用、镜头连续性和可生成性约束的 Script Package，并显式 publish。
`

const classesYAML = `schema_version: "2.0"
classes:
  - Client
  - Brand
  - Product
  - Source
  - Evidence
  - Fact
  - Claim
  - Asset
  - Rights
  - Brief
  - ScriptPackage
`

const propertiesYAML = `schema_version: "2.0"
required:
  Fact: [id, subject, predicate, value, evidence_refs]
  Claim: [id, statement, risk_level, evidence_refs]
  ScriptPackage: [schema_version, title, shots, citations]
`

const mcpDescriptor = `{
  "name": "contentcloud-local",
  "version": "2.0",
  "transport": "stdio",
  "command": "contentcloud",
  "args": ["mcp", "serve"],
  "network_boundary": "all cloud communication is delegated to the contentcloud CLI"
}
`

const codexMCPConfig = `[mcp_servers.contentcloud-local]
command = "contentcloud"
args = ["mcp", "serve"]
`

const claudeMCPConfig = `{
  "mcpServers": {
    "contentcloud-local": {
      "command": "contentcloud",
      "args": ["mcp", "serve"]
    }
  }
}
`
