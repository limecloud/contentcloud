package localworkspace

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

var localRunStages = map[string]map[string]bool{
	"ingest":  {"ingest": true, "knowledge-lint": true, "done": true},
	"query":   {"ingest": true, "knowledge-lint": true, "query": true, "done": true},
	"content": {"ingest": true, "knowledge-lint": true, "query": true, "compile": true, "output-lint": true, "done": true},
}

var localRunTransitions = map[string]map[string]bool{
	"ingest":         {"knowledge-lint": true},
	"knowledge-lint": {"query": true, "done": true},
	"query":          {"compile": true, "done": true},
	"compile":        {"output-lint": true},
	"output-lint":    {"done": true},
}

type LocalRunContext struct {
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Intent        string            `json:"intent"`
	Stage         string            `json:"stage"`
	Status        string            `json:"status"`
	SourceRefs    []string          `json:"source_refs"`
	ChangedIDs    []string          `json:"changed_ids"`
	EligibleIDs   []string          `json:"eligible_ids"`
	BlockedIDs    []string          `json:"blocked_ids"`
	Findings      []string          `json:"findings"`
	OutputPaths   []string          `json:"output_paths"`
	Checks        []LocalRunCheck   `json:"checks"`
	History       []LocalRunHistory `json:"history"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type LocalRunCheck struct {
	Name    string    `json:"name"`
	Status  string    `json:"status"`
	Stage   string    `json:"stage"`
	Command string    `json:"command,omitempty"`
	Detail  string    `json:"detail,omitempty"`
	At      time.Time `json:"at"`
}

type LocalRunHistory struct {
	Event    string    `json:"event"`
	Stage    string    `json:"stage,omitempty"`
	From     string    `json:"from,omitempty"`
	To       string    `json:"to,omitempty"`
	Name     string    `json:"name,omitempty"`
	Status   string    `json:"status,omitempty"`
	Findings []string  `json:"findings,omitempty"`
	At       time.Time `json:"at"`
}

type LocalRunPointer struct {
	SchemaVersion string    `json:"schema_version"`
	RunID         string    `json:"run_id"`
	ContextPath   string    `json:"context_path"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type InitLocalRunOptions struct {
	Root       string
	RunID      string
	Intent     string
	SourceRefs []string
	WithIngest bool
	Now        time.Time
}

type RecordLocalRunOptions struct {
	Root        string
	RunID       string
	SourceRefs  []string
	ChangedIDs  []string
	EligibleIDs []string
	BlockedIDs  []string
	Findings    []string
	OutputPaths []string
	Now         time.Time
}

type CheckLocalRunOptions struct {
	Root    string
	RunID   string
	Name    string
	Status  string
	Command string
	Detail  string
	Now     time.Time
}

type LocalRunValidation struct {
	Valid      bool                       `json:"valid"`
	RunCount   int                        `json:"run_count"`
	CurrentRun string                     `json:"current_run,omitempty"`
	Results    []LocalRunValidationResult `json:"results"`
}

type LocalRunValidationResult struct {
	RunID  string   `json:"run_id"`
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

func InitLocalRun(options InitLocalRunOptions) (LocalRunContext, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return LocalRunContext{}, err
	}
	intent := strings.ToLower(strings.TrimSpace(options.Intent))
	if localRunStages[intent] == nil {
		return LocalRunContext{}, domain.Invalid("LOCAL_RUN_INTENT_INVALID", "intent 只允许 ingest、query 或 content")
	}
	now := localNow(options.Now)
	runID := strings.TrimSpace(options.RunID)
	if runID == "" {
		runID = "local-run-" + now.Format("20060102T150405Z") + "-" + strings.ReplaceAll(domain.NewID()[:8], "-", "")
	}
	if !localSourceIDPattern.MatchString(runID) {
		return LocalRunContext{}, domain.Invalid("LOCAL_RUN_ID_INVALID", "run ID 只能包含字母、数字、冒号、点、下划线和连字符")
	}
	stage := "knowledge-lint"
	if options.WithIngest || intent == "ingest" {
		stage = "ingest"
	}
	context := LocalRunContext{
		SchemaVersion: SchemaVersion,
		RunID:         runID,
		Intent:        intent,
		Stage:         stage,
		Status:        "in_progress",
		SourceRefs:    uniqueStrings(options.SourceRefs),
		ChangedIDs:    []string{},
		EligibleIDs:   []string{},
		BlockedIDs:    []string{},
		Findings:      []string{},
		OutputPaths:   []string{},
		Checks:        []LocalRunCheck{},
		History:       []LocalRunHistory{{Event: "initialized", To: stage, At: now}},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	path := localRunPath(root, runID)
	if _, err := os.Stat(path); err == nil {
		return LocalRunContext{}, domain.Conflict("LOCAL_RUN_EXISTS", "相同 run ID 已存在")
	} else if !errors.Is(err, os.ErrNotExist) {
		return LocalRunContext{}, err
	}
	if err := saveLocalRun(root, context, now); err != nil {
		return LocalRunContext{}, err
	}
	return context, nil
}

func ShowLocalRun(root, runID string) (LocalRunContext, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return LocalRunContext{}, err
	}
	return loadLocalRun(resolved, runID)
}

func RecordLocalRun(options RecordLocalRunOptions) (LocalRunContext, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return LocalRunContext{}, err
	}
	context, err := loadLocalRun(root, options.RunID)
	if err != nil {
		return LocalRunContext{}, err
	}
	if context.Status == "completed" {
		return LocalRunContext{}, domain.Conflict("LOCAL_RUN_COMPLETED", "已完成的 LocalRun 不可再修改")
	}
	context.SourceRefs = mergeStrings(context.SourceRefs, options.SourceRefs)
	context.ChangedIDs = mergeStrings(context.ChangedIDs, options.ChangedIDs)
	context.EligibleIDs = mergeStrings(context.EligibleIDs, options.EligibleIDs)
	context.BlockedIDs = mergeStrings(context.BlockedIDs, options.BlockedIDs)
	context.Findings = mergeStrings(context.Findings, options.Findings)
	context.OutputPaths = mergeStrings(context.OutputPaths, options.OutputPaths)
	now := localNow(options.Now)
	context.History = append(context.History, LocalRunHistory{Event: "recorded", Stage: context.Stage, At: now})
	if err := saveLocalRun(root, context, now); err != nil {
		return LocalRunContext{}, err
	}
	return context, nil
}

func CheckLocalRun(options CheckLocalRunOptions) (LocalRunContext, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return LocalRunContext{}, err
	}
	name := strings.TrimSpace(options.Name)
	status := strings.ToLower(strings.TrimSpace(options.Status))
	if name == "" || (status != "passed" && status != "failed") {
		return LocalRunContext{}, domain.Invalid("LOCAL_RUN_CHECK_INVALID", "check 需要 name，status 只允许 passed 或 failed")
	}
	context, err := loadLocalRun(root, options.RunID)
	if err != nil {
		return LocalRunContext{}, err
	}
	if context.Status == "completed" {
		return LocalRunContext{}, domain.Conflict("LOCAL_RUN_COMPLETED", "已完成的 LocalRun 不可再修改")
	}
	now := localNow(options.Now)
	check := LocalRunCheck{Name: name, Status: status, Stage: context.Stage, Command: strings.TrimSpace(options.Command), Detail: strings.TrimSpace(options.Detail), At: now}
	context.Checks = append(context.Checks, check)
	context.History = append(context.History, LocalRunHistory{Event: "check", Stage: context.Stage, Name: name, Status: status, At: now})
	if status == "failed" {
		context.Status = "failed"
	}
	if err := saveLocalRun(root, context, now); err != nil {
		return LocalRunContext{}, err
	}
	return context, nil
}

func AdvanceLocalRun(root, runID, target string, additions RecordLocalRunOptions, now time.Time) (LocalRunContext, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return LocalRunContext{}, err
	}
	context, err := loadLocalRun(resolved, runID)
	if err != nil {
		return LocalRunContext{}, err
	}
	target = strings.ToLower(strings.TrimSpace(target))
	if context.Status != "in_progress" {
		return LocalRunContext{}, domain.Conflict("LOCAL_RUN_STATUS_INVALID", "只有 in_progress LocalRun 可以推进")
	}
	if !localRunTransitions[context.Stage][target] || !localRunStages[context.Intent][target] {
		return LocalRunContext{}, domain.Conflict("LOCAL_RUN_TRANSITION_INVALID", "LocalRun 阶段转换不允许："+context.Stage+" -> "+target)
	}
	context.SourceRefs = mergeStrings(context.SourceRefs, additions.SourceRefs)
	context.ChangedIDs = mergeStrings(context.ChangedIDs, additions.ChangedIDs)
	context.EligibleIDs = mergeStrings(context.EligibleIDs, additions.EligibleIDs)
	context.BlockedIDs = mergeStrings(context.BlockedIDs, additions.BlockedIDs)
	context.Findings = mergeStrings(context.Findings, additions.Findings)
	context.OutputPaths = mergeStrings(context.OutputPaths, additions.OutputPaths)
	if context.Stage == "knowledge-lint" && !latestPassedLocalRunCheck(context, "kb-lint", "knowledge-lint") {
		return LocalRunContext{}, domain.Policy("LOCAL_RUN_KNOWLEDGE_LINT_REQUIRED", "knowledge-lint 阶段需要通过 kb-lint", "先运行 contentcloud local knowledge lint 并记录检查结果")
	}
	if context.Stage == "query" && target == "compile" && len(context.EligibleIDs) == 0 && len(context.BlockedIDs) == 0 {
		return LocalRunContext{}, domain.Policy("LOCAL_RUN_QUERY_RESULT_REQUIRED", "query 阶段必须记录 eligible_ids 或 blocked_ids", "先运行 contentcloud local knowledge query")
	}
	if context.Stage == "compile" && len(context.OutputPaths) == 0 {
		return LocalRunContext{}, domain.Policy("LOCAL_RUN_OUTPUT_REQUIRED", "compile 阶段必须记录 output_paths", "记录本地输出文件后再进入 output-lint")
	}
	if context.Stage == "output-lint" && !latestPassedLocalRunCheck(context, "content-lint", "output-lint") {
		return LocalRunContext{}, domain.Policy("LOCAL_RUN_CONTENT_LINT_REQUIRED", "output-lint 阶段需要通过 content-lint", "先完成确定性内容校验")
	}
	at := localNow(now)
	context.History = append(context.History,
		LocalRunHistory{Event: "completed", Stage: context.Stage, At: at},
		LocalRunHistory{Event: "handoff", From: context.Stage, To: target, At: at},
	)
	context.Stage = target
	if target == "done" {
		context.Status = "completed"
	}
	if err := saveLocalRun(resolved, context, at); err != nil {
		return LocalRunContext{}, err
	}
	return context, nil
}

func ResumeLocalRun(root, runID string, now time.Time) (LocalRunContext, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return LocalRunContext{}, err
	}
	context, err := loadLocalRun(resolved, runID)
	if err != nil {
		return LocalRunContext{}, err
	}
	if context.Status != "failed" {
		return LocalRunContext{}, domain.Conflict("LOCAL_RUN_NOT_FAILED", "只有 failed LocalRun 可以恢复")
	}
	at := localNow(now)
	context.Status = "in_progress"
	context.History = append(context.History, LocalRunHistory{Event: "resumed", Stage: context.Stage, At: at})
	if err := saveLocalRun(resolved, context, at); err != nil {
		return LocalRunContext{}, err
	}
	return context, nil
}

func FailLocalRun(root, runID string, findings []string, now time.Time) (LocalRunContext, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return LocalRunContext{}, err
	}
	context, err := loadLocalRun(resolved, runID)
	if err != nil {
		return LocalRunContext{}, err
	}
	if context.Status == "completed" {
		return LocalRunContext{}, domain.Conflict("LOCAL_RUN_COMPLETED", "已完成的 LocalRun 不可标记失败")
	}
	at := localNow(now)
	findings = uniqueStrings(findings)
	context.Findings = mergeStrings(context.Findings, findings)
	context.Status = "failed"
	context.History = append(context.History, LocalRunHistory{Event: "failed", Stage: context.Stage, Findings: findings, At: at})
	if err := saveLocalRun(resolved, context, at); err != nil {
		return LocalRunContext{}, err
	}
	return context, nil
}

func ValidateLocalRuns(root string) (LocalRunValidation, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return LocalRunValidation{}, err
	}
	files, err := filepath.Glob(filepath.Join(resolved, "work", "runs", "*.json"))
	if err != nil {
		return LocalRunValidation{}, err
	}
	sort.Strings(files)
	report := LocalRunValidation{Valid: true, RunCount: len(files), Results: []LocalRunValidationResult{}}
	for _, path := range files {
		var context LocalRunContext
		result := LocalRunValidationResult{Valid: true, Errors: []string{}}
		if err := readJSON(path, &context); err != nil {
			result.RunID = strings.TrimSuffix(filepath.Base(path), ".json")
			result.Errors = append(result.Errors, err.Error())
		} else {
			result.RunID = context.RunID
			result.Errors = validateLocalRun(context)
		}
		result.Valid = len(result.Errors) == 0
		if !result.Valid {
			report.Valid = false
		}
		report.Results = append(report.Results, result)
	}
	pointerPath := filepath.Join(resolved, "work", "current-run.json")
	var pointer LocalRunPointer
	if err := readJSON(pointerPath, &pointer); err == nil {
		report.CurrentRun = pointer.RunID
		pointed := filepath.Clean(filepath.Join(resolved, "work", filepath.FromSlash(pointer.ContextPath)))
		if _, statErr := os.Stat(pointed); statErr != nil {
			report.Valid = false
			report.Results = append(report.Results, LocalRunValidationResult{RunID: pointer.RunID, Valid: false, Errors: []string{"current-run.json 指向不存在的 context"}})
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return LocalRunValidation{}, err
	}
	return report, nil
}

func loadLocalRun(root, runID string) (LocalRunContext, error) {
	if strings.TrimSpace(runID) == "" {
		var pointer LocalRunPointer
		if err := readJSON(filepath.Join(root, "work", "current-run.json"), &pointer); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return LocalRunContext{}, domain.NotFound("当前 LocalRun")
			}
			return LocalRunContext{}, err
		}
		runID = pointer.RunID
	}
	if !localSourceIDPattern.MatchString(runID) {
		return LocalRunContext{}, domain.Invalid("LOCAL_RUN_ID_INVALID", "run ID 无效")
	}
	var context LocalRunContext
	if err := readJSON(localRunPath(root, runID), &context); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LocalRunContext{}, domain.NotFound("LocalRun")
		}
		return LocalRunContext{}, err
	}
	if problems := validateLocalRun(context); len(problems) > 0 {
		err := domain.Invalid("LOCAL_RUN_CONTEXT_INVALID", "LocalRunContext 校验失败")
		err.Details = map[string]any{"errors": problems}
		return LocalRunContext{}, err
	}
	return context, nil
}

func saveLocalRun(root string, context LocalRunContext, now time.Time) error {
	context.SchemaVersion = SchemaVersion
	context.UpdatedAt = localNow(now)
	path := localRunPath(root, context.RunID)
	if err := replaceJSON(path, context, 0o600); err != nil {
		return err
	}
	pointer := LocalRunPointer{SchemaVersion: SchemaVersion, RunID: context.RunID, ContextPath: filepath.ToSlash(filepath.Join("runs", context.RunID+".json")), UpdatedAt: context.UpdatedAt}
	return replaceJSON(filepath.Join(root, "work", "current-run.json"), pointer, 0o600)
}

func localRunPath(root, runID string) string {
	return filepath.Join(root, "work", "runs", runID+".json")
}

func validateLocalRun(context LocalRunContext) []string {
	problems := []string{}
	if context.SchemaVersion != SchemaVersion {
		problems = append(problems, "schema_version 不受支持")
	}
	if context.RunID == "" || !localSourceIDPattern.MatchString(context.RunID) {
		problems = append(problems, "run_id 无效")
	}
	if localRunStages[context.Intent] == nil {
		problems = append(problems, "intent 无效")
	} else if !localRunStages[context.Intent][context.Stage] {
		problems = append(problems, "stage 与 intent 不兼容")
	}
	if context.Status != "in_progress" && context.Status != "failed" && context.Status != "completed" {
		problems = append(problems, "status 无效")
	}
	if context.Stage == "done" && context.Status != "completed" {
		problems = append(problems, "done stage 必须是 completed")
	}
	if context.Stage != "done" && context.Status == "completed" {
		problems = append(problems, "completed status 必须处于 done stage")
	}
	if context.History == nil || context.Checks == nil {
		problems = append(problems, "history 和 checks 必须存在")
	}
	return problems
}

func latestPassedLocalRunCheck(context LocalRunContext, name, stage string) bool {
	for index := len(context.Checks) - 1; index >= 0; index-- {
		check := context.Checks[index]
		if check.Name == name && check.Stage == stage {
			return check.Status == "passed"
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func mergeStrings(current, additions []string) []string {
	return uniqueStrings(append(append([]string(nil), current...), additions...))
}
