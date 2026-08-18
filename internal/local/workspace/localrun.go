package localworkspace

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
)

var localRunStages = map[string]map[string]bool{
	"content": {"ingest": true, "knowledge-lint": true, "query": true, "compile": true, "output-lint": true, "done": true},
}

const LocalRunSchemaVersion = "contentcloud.local-run/3.0"

var localRunTransitions = map[string]map[string]bool{
	"ingest":         {"knowledge-lint": true},
	"knowledge-lint": {"query": true, "done": true},
	"query":          {"compile": true, "done": true},
	"compile":        {"output-lint": true},
	"output-lint":    {"done": true},
}

type LocalRunContext struct {
	SchemaVersion   string             `json:"schema_version"`
	ContextRevision uint64             `json:"context_revision"`
	RunID           string             `json:"run_id"`
	Intent          string             `json:"intent_id"`
	Stage           string             `json:"stage"`
	Status          string             `json:"status"`
	InputRefs       []LocalRunInputRef `json:"input_refs"`
	ChangedIDs      []string           `json:"changed_ids"`
	EligibleIDs     []string           `json:"eligible_ids"`
	BlockedIDs      []string           `json:"blocked_ids"`
	Findings        []string           `json:"findings"`
	OutputPaths     []string           `json:"output_refs"`
	Checks          []LocalRunCheck    `json:"checks"`
	History         []LocalRunHistory  `json:"history"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type LocalRunInputRef struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
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

type InitLocalRunOptions struct {
	Root       string
	RunID      string
	Intent     string
	InputIDs   []string
	WithIngest bool
	Now        time.Time
}

type RecordLocalRunOptions struct {
	Root             string
	RunID            string
	ClaimToken       string
	ExpectedRevision uint64
	InputIDs         []string
	ChangedIDs       []string
	EligibleIDs      []string
	BlockedIDs       []string
	Findings         []string
	OutputPaths      []string
	Now              time.Time
}

type CheckLocalRunOptions struct {
	Root             string
	RunID            string
	ClaimToken       string
	ExpectedRevision uint64
	Name             string
	Status           string
	Command          string
	Detail           string
	Now              time.Time
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
	intent := strings.TrimSpace(options.Intent)
	if !strings.HasPrefix(intent, "intent:") || !localSourceIDPattern.MatchString(intent) {
		return LocalRunContext{}, fault.Invalid("LOCAL_RUN_INTENT_INVALID", "intent_id 必须使用 intent:<name> 格式的稳定 ID")
	}
	inputRefs, err := resolveLocalRunInputRefs(root, options.InputIDs)
	if err != nil {
		return LocalRunContext{}, err
	}
	now := localNow(options.Now)
	runID := strings.TrimSpace(options.RunID)
	if runID == "" {
		runID = "run_" + now.Format("20060102T150405Z") + "_" + strings.ReplaceAll(idgen.New()[:8], "-", "")
	}
	if !localSourceIDPattern.MatchString(runID) {
		return LocalRunContext{}, fault.Invalid("LOCAL_RUN_ID_INVALID", "运行 ID 只能包含字母、数字、冒号、点、下划线和连字符")
	}
	stage := "knowledge-lint"
	if options.WithIngest {
		stage = "ingest"
	}
	context := LocalRunContext{
		SchemaVersion: LocalRunSchemaVersion,
		RunID:         runID,
		Intent:        intent,
		Stage:         stage,
		Status:        "active",
		InputRefs:     inputRefs,
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
		return LocalRunContext{}, fault.Conflict("LOCAL_RUN_EXISTS", "相同 run ID 已存在")
	} else if !errors.Is(err, os.ErrNotExist) {
		return LocalRunContext{}, err
	}
	context, err = saveLocalRun(root, context, now)
	if err != nil {
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
	return recordLocalRun(options, false)
}

func RecordClaimedLocalRun(options RecordLocalRunOptions) (LocalRunContext, error) {
	return recordLocalRun(options, true)
}

func recordLocalRun(options RecordLocalRunOptions, requireClaim bool) (LocalRunContext, error) {
	return recordLocalRunInternal(options, requireClaim, false)
}

// recordClaimedLocalRunWithMutationLock is used by compound local mutations
// that already hold the Run lock. It prevents the second save from opening a
// write window inside the surrounding transaction.
func recordClaimedLocalRunWithMutationLock(options RecordLocalRunOptions) (LocalRunContext, error) {
	return recordLocalRunInternal(options, true, true)
}

func recordLocalRunInternal(options RecordLocalRunOptions, requireClaim, mutationLockHeld bool) (LocalRunContext, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return LocalRunContext{}, err
	}
	context, err := loadLocalRun(root, options.RunID)
	if err != nil {
		return LocalRunContext{}, err
	}
	if context.Status == "completed" {
		return LocalRunContext{}, fault.Conflict("LOCAL_RUN_COMPLETED", "已完成的本地运行不可再修改")
	}
	if requireClaim {
		if err := validateClaimedRunWrite(root, context, options.ClaimToken, options.ExpectedRevision, options.Now); err != nil {
			return LocalRunContext{}, err
		}
	}
	inputRefs, err := resolveLocalRunInputRefs(root, options.InputIDs)
	if err != nil {
		return LocalRunContext{}, err
	}
	context.InputRefs = mergeLocalRunInputRefs(context.InputRefs, inputRefs)
	context.ChangedIDs = mergeStrings(context.ChangedIDs, options.ChangedIDs)
	context.EligibleIDs = mergeStrings(context.EligibleIDs, options.EligibleIDs)
	context.BlockedIDs = mergeStrings(context.BlockedIDs, options.BlockedIDs)
	context.Findings = mergeStrings(context.Findings, options.Findings)
	context.OutputPaths = mergeStrings(context.OutputPaths, options.OutputPaths)
	now := localNow(options.Now)
	context.History = append(context.History, LocalRunHistory{Event: "recorded", Stage: context.Stage, At: now})
	if mutationLockHeld {
		context, err = saveLocalRunUnlocked(root, context, now)
	} else {
		context, err = saveLocalRun(root, context, now)
	}
	if err != nil {
		return LocalRunContext{}, err
	}
	if requireClaim {
		if err := updateRunClaimRevision(root, context.RunID, options.ClaimToken, context.ContextRevision, now); err != nil {
			return LocalRunContext{}, err
		}
	}
	return context, nil
}

func CheckLocalRun(options CheckLocalRunOptions) (LocalRunContext, error) {
	return checkLocalRun(options, false)
}

func CheckClaimedLocalRun(options CheckLocalRunOptions) (LocalRunContext, error) {
	return checkLocalRun(options, true)
}

func checkLocalRun(options CheckLocalRunOptions, requireClaim bool) (LocalRunContext, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return LocalRunContext{}, err
	}
	name := strings.TrimSpace(options.Name)
	status := strings.ToLower(strings.TrimSpace(options.Status))
	if name == "" || (status != "passed" && status != "failed") {
		return LocalRunContext{}, fault.Invalid("LOCAL_RUN_CHECK_INVALID", "检查项需要 name，status 只允许 passed 或 failed")
	}
	context, err := loadLocalRun(root, options.RunID)
	if err != nil {
		return LocalRunContext{}, err
	}
	if context.Status == "completed" {
		return LocalRunContext{}, fault.Conflict("LOCAL_RUN_COMPLETED", "已完成的本地运行不可再修改")
	}
	if requireClaim {
		if err := validateClaimedRunWrite(root, context, options.ClaimToken, options.ExpectedRevision, options.Now); err != nil {
			return LocalRunContext{}, err
		}
	}
	now := localNow(options.Now)
	check := LocalRunCheck{Name: name, Status: status, Stage: context.Stage, Command: strings.TrimSpace(options.Command), Detail: strings.TrimSpace(options.Detail), At: now}
	context.Checks = append(context.Checks, check)
	context.History = append(context.History, LocalRunHistory{Event: "check", Stage: context.Stage, Name: name, Status: status, At: now})
	if status == "failed" {
		context.Status = "failed"
	}
	context, err = saveLocalRun(root, context, now)
	if err != nil {
		return LocalRunContext{}, err
	}
	if requireClaim {
		if err := updateRunClaimRevision(root, context.RunID, options.ClaimToken, context.ContextRevision, now); err != nil {
			return LocalRunContext{}, err
		}
	}
	return context, nil
}

func AdvanceLocalRun(root, runID, target string, additions RecordLocalRunOptions, now time.Time) (LocalRunContext, error) {
	return advanceLocalRun(root, runID, target, additions, now, false)
}

func AdvanceClaimedLocalRun(root, runID, target string, additions RecordLocalRunOptions, now time.Time) (LocalRunContext, error) {
	return advanceLocalRun(root, runID, target, additions, now, true)
}

func advanceLocalRun(root, runID, target string, additions RecordLocalRunOptions, now time.Time, requireClaim bool) (LocalRunContext, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return LocalRunContext{}, err
	}
	context, err := loadLocalRun(resolved, runID)
	if err != nil {
		return LocalRunContext{}, err
	}
	target = strings.ToLower(strings.TrimSpace(target))
	if context.Status != "active" {
		return LocalRunContext{}, fault.Conflict("LOCAL_RUN_STATUS_INVALID", "只有运行中的本地任务可以推进")
	}
	if requireClaim {
		if err := validateClaimedRunWrite(resolved, context, additions.ClaimToken, additions.ExpectedRevision, now); err != nil {
			return LocalRunContext{}, err
		}
	}
	if !localRunTransitions[context.Stage][target] || !localRunStages["content"][target] {
		return LocalRunContext{}, fault.Conflict("LOCAL_RUN_TRANSITION_INVALID", "不允许执行该本地运行阶段转换："+context.Stage+" -> "+target)
	}
	inputRefs, err := resolveLocalRunInputRefs(resolved, additions.InputIDs)
	if err != nil {
		return LocalRunContext{}, err
	}
	context.InputRefs = mergeLocalRunInputRefs(context.InputRefs, inputRefs)
	context.ChangedIDs = mergeStrings(context.ChangedIDs, additions.ChangedIDs)
	context.EligibleIDs = mergeStrings(context.EligibleIDs, additions.EligibleIDs)
	context.BlockedIDs = mergeStrings(context.BlockedIDs, additions.BlockedIDs)
	context.Findings = mergeStrings(context.Findings, additions.Findings)
	context.OutputPaths = mergeStrings(context.OutputPaths, additions.OutputPaths)
	if context.Stage == "knowledge-lint" && !latestPassedLocalRunCheck(context, "kb-lint", "knowledge-lint") {
		return LocalRunContext{}, fault.Policy("LOCAL_RUN_KNOWLEDGE_LINT_REQUIRED", "知识校验阶段必须通过 kb-lint", "先运行 contentcloud local knowledge lint 并记录检查结果")
	}
	if context.Stage == "query" && target == "compile" && len(context.EligibleIDs) == 0 && len(context.BlockedIDs) == 0 {
		return LocalRunContext{}, fault.Policy("LOCAL_RUN_QUERY_RESULT_REQUIRED", "查询阶段必须记录 eligible_ids 或 blocked_ids", "先运行 contentcloud local knowledge query")
	}
	if context.Stage == "compile" && len(context.OutputPaths) == 0 {
		return LocalRunContext{}, fault.Policy("LOCAL_RUN_OUTPUT_REQUIRED", "编译阶段必须记录 output_paths", "记录本地输出文件后再进入 output-lint")
	}
	if context.Stage == "output-lint" && !latestPassedLocalRunCheck(context, "content-lint", "output-lint") {
		return LocalRunContext{}, fault.Policy("LOCAL_RUN_CONTENT_LINT_REQUIRED", "输出校验阶段必须通过 content-lint", "先完成确定性内容校验")
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
	context, err = saveLocalRun(resolved, context, at)
	if err != nil {
		return LocalRunContext{}, err
	}
	if requireClaim {
		if err := updateRunClaimRevision(resolved, context.RunID, additions.ClaimToken, context.ContextRevision, at); err != nil {
			return LocalRunContext{}, err
		}
	}
	return context, nil
}

func ResumeLocalRun(root, runID string, now time.Time) (LocalRunContext, error) {
	return resumeLocalRun(root, runID, "", 0, now, false)
}

func ResumeClaimedLocalRun(root, runID, claimToken string, expectedRevision uint64, now time.Time) (LocalRunContext, error) {
	return resumeLocalRun(root, runID, claimToken, expectedRevision, now, true)
}

func resumeLocalRun(root, runID, claimToken string, expectedRevision uint64, now time.Time, requireClaim bool) (LocalRunContext, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return LocalRunContext{}, err
	}
	context, err := loadLocalRun(resolved, runID)
	if err != nil {
		return LocalRunContext{}, err
	}
	if context.Status != "failed" {
		return LocalRunContext{}, fault.Conflict("LOCAL_RUN_NOT_FAILED", "只有失败的本地运行可以恢复")
	}
	if requireClaim {
		if err := validateClaimedRunWrite(resolved, context, claimToken, expectedRevision, now); err != nil {
			return LocalRunContext{}, err
		}
	}
	at := localNow(now)
	context.Status = "active"
	context.History = append(context.History, LocalRunHistory{Event: "resumed", Stage: context.Stage, At: at})
	context, err = saveLocalRun(resolved, context, at)
	if err != nil {
		return LocalRunContext{}, err
	}
	if requireClaim {
		if err := updateRunClaimRevision(resolved, context.RunID, claimToken, context.ContextRevision, at); err != nil {
			return LocalRunContext{}, err
		}
	}
	return context, nil
}

func FailLocalRun(root, runID string, findings []string, now time.Time) (LocalRunContext, error) {
	return failLocalRun(root, runID, findings, "", 0, now, false)
}

func FailClaimedLocalRun(root, runID string, findings []string, claimToken string, expectedRevision uint64, now time.Time) (LocalRunContext, error) {
	return failLocalRun(root, runID, findings, claimToken, expectedRevision, now, true)
}

func failLocalRun(root, runID string, findings []string, claimToken string, expectedRevision uint64, now time.Time, requireClaim bool) (LocalRunContext, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return LocalRunContext{}, err
	}
	context, err := loadLocalRun(resolved, runID)
	if err != nil {
		return LocalRunContext{}, err
	}
	if context.Status == "completed" {
		return LocalRunContext{}, fault.Conflict("LOCAL_RUN_COMPLETED", "已完成的本地运行不能再标记为失败")
	}
	if requireClaim {
		if err := validateClaimedRunWrite(resolved, context, claimToken, expectedRevision, now); err != nil {
			return LocalRunContext{}, err
		}
	}
	at := localNow(now)
	findings = uniqueStrings(findings)
	context.Findings = mergeStrings(context.Findings, findings)
	context.Status = "failed"
	context.History = append(context.History, LocalRunHistory{Event: "failed", Stage: context.Stage, Findings: findings, At: at})
	context, err = saveLocalRun(resolved, context, at)
	if err != nil {
		return LocalRunContext{}, err
	}
	if requireClaim {
		if err := updateRunClaimRevision(resolved, context.RunID, claimToken, context.ContextRevision, at); err != nil {
			return LocalRunContext{}, err
		}
	}
	return context, nil
}

func ValidateLocalRuns(root string) (LocalRunValidation, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return LocalRunValidation{}, err
	}
	files, err := filepath.Glob(filepath.Join(resolved, "40-work", "runs", "*", "context.json"))
	if err != nil {
		return LocalRunValidation{}, err
	}
	sort.Strings(files)
	report := LocalRunValidation{Valid: true, RunCount: len(files), Results: []LocalRunValidationResult{}}
	for _, path := range files {
		var context LocalRunContext
		result := LocalRunValidationResult{Valid: true, Errors: []string{}}
		if err := readJSON(path, &context); err != nil {
			result.RunID = filepath.Base(filepath.Dir(path))
			result.Errors = append(result.Errors, err.Error())
		} else {
			normalizeLocalRunContext(&context)
			result.RunID = context.RunID
			result.Errors = validateLocalRun(context)
		}
		result.Valid = len(result.Errors) == 0
		if !result.Valid {
			report.Valid = false
		}
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func loadLocalRun(root, runID string) (LocalRunContext, error) {
	if strings.TrimSpace(runID) == "" {
		paths, err := filepath.Glob(filepath.Join(root, "40-work", "runs", "*", "context.json"))
		if err != nil {
			return LocalRunContext{}, err
		}
		active := []string{}
		for _, path := range paths {
			var candidate LocalRunContext
			if readJSON(path, &candidate) == nil && candidate.Status != "completed" {
				active = append(active, candidate.RunID)
			}
		}
		if len(active) == 0 {
			return LocalRunContext{}, fault.NotFound("运行中的本地任务")
		}
		if len(active) != 1 {
			conflict := fault.Conflict("LOCAL_RUN_SELECTION_REQUIRED", "存在多个活动 Run，必须显式指定 run_id")
			conflict.Details = map[string]any{"run_ids": active}
			return LocalRunContext{}, conflict
		}
		runID = active[0]
	}
	if !localSourceIDPattern.MatchString(runID) {
		return LocalRunContext{}, fault.Invalid("LOCAL_RUN_ID_INVALID", "运行 ID 无效")
	}
	var context LocalRunContext
	if err := readJSON(localRunPath(root, runID), &context); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LocalRunContext{}, fault.NotFound("本地运行")
		}
		return LocalRunContext{}, err
	}
	normalizeLocalRunContext(&context)
	if problems := validateLocalRun(context); len(problems) > 0 {
		err := fault.Invalid("LOCAL_RUN_CONTEXT_INVALID", "本地运行上下文校验失败")
		err.Details = map[string]any{"errors": problems}
		return LocalRunContext{}, err
	}
	return context, nil
}

func saveLocalRun(root string, context LocalRunContext, now time.Time) (LocalRunContext, error) {
	release, err := acquireLocalRunMutationLock(root, context.RunID, now)
	if err != nil {
		return LocalRunContext{}, err
	}
	defer release()
	return saveLocalRunUnlocked(root, context, now)
}

func saveLocalRunUnlocked(root string, context LocalRunContext, now time.Time) (LocalRunContext, error) {
	path := localRunPath(root, context.RunID)
	var current LocalRunContext
	if err := readJSON(path, &current); err == nil {
		normalizeLocalRunContext(&current)
		if context.ContextRevision != current.ContextRevision {
			conflict := fault.Conflict("LOCAL_RUN_REVISION_CONFLICT", "本地运行上下文的版本已经变化")
			conflict.Details = map[string]any{"expected_revision": context.ContextRevision, "current_revision": current.ContextRevision, "run_id": context.RunID}
			return LocalRunContext{}, conflict
		}
		context.ContextRevision++
	} else if errors.Is(err, os.ErrNotExist) {
		context.ContextRevision = 1
	} else {
		return LocalRunContext{}, err
	}
	context.SchemaVersion = LocalRunSchemaVersion
	context.UpdatedAt = localNow(now)
	if err := replaceJSON(path, context, 0o600); err != nil {
		return LocalRunContext{}, err
	}
	return context, nil
}

func localRunPath(root, runID string) string {
	return filepath.Join(root, "40-work", "runs", runID, "context.json")
}

func validateLocalRun(context LocalRunContext) []string {
	problems := []string{}
	if context.SchemaVersion != LocalRunSchemaVersion {
		problems = append(problems, "schema_version 不受支持")
	}
	if context.RunID == "" || !localSourceIDPattern.MatchString(context.RunID) {
		problems = append(problems, "run_id 无效")
	}
	if context.ContextRevision == 0 {
		problems = append(problems, "context_revision 必须大于 0")
	}
	if !strings.HasPrefix(context.Intent, "intent:") || !localSourceIDPattern.MatchString(context.Intent) {
		problems = append(problems, "intent_id 无效")
	} else if !localRunStages["content"][context.Stage] {
		problems = append(problems, "stage 无效")
	}
	if context.Status != "active" && context.Status != "failed" && context.Status != "completed" {
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
	seenInputs := map[string]string{}
	for _, ref := range context.InputRefs {
		if !localSourceIDPattern.MatchString(ref.ID) || !validSHA256Digest(ref.Digest) {
			problems = append(problems, "input_refs 包含无效 ID 或 digest")
			continue
		}
		if digest, exists := seenInputs[ref.ID]; exists && digest != ref.Digest {
			problems = append(problems, "input_refs 中同一 ID 对应多个 digest")
		}
		seenInputs[ref.ID] = ref.Digest
	}
	return problems
}

func normalizeLocalRunContext(context *LocalRunContext) {
	if context.ContextRevision == 0 {
		context.ContextRevision = 1
	}
	if context.InputRefs == nil {
		context.InputRefs = []LocalRunInputRef{}
	}
}

func resolveLocalRunInputRefs(root string, ids []string) ([]LocalRunInputRef, error) {
	refs := make([]LocalRunInputRef, 0, len(ids))
	for _, id := range uniqueStrings(ids) {
		source, err := LocalSourceByID(root, id)
		if err != nil {
			invalid := fault.Invalid("LOCAL_RUN_INPUT_REF_INVALID", "input_ref 必须引用已登记的不可变来源："+id)
			invalid.Hint = "先执行 contentcloud local source register，再用返回的来源 ID 初始化运行"
			return nil, invalid
		}
		refs = append(refs, LocalRunInputRef{ID: source.ID, Digest: "sha256:" + source.SHA256})
	}
	return refs, nil
}

func mergeLocalRunInputRefs(current, additions []LocalRunInputRef) []LocalRunInputRef {
	byID := make(map[string]LocalRunInputRef, len(current)+len(additions))
	for _, ref := range current {
		byID[ref.ID] = ref
	}
	for _, ref := range additions {
		byID[ref.ID] = ref
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	refs := make([]LocalRunInputRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, byID[id])
	}
	return refs
}

func validSHA256Digest(value string) bool {
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func acquireLocalRunMutationLock(root, runID string, now time.Time) (func(), error) {
	directory := filepath.Join(root, ".contentcloud", "locks", "runs")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(directory, runID+".mutation.lock")
	acquire := func() error {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, writeErr := file.WriteString(localNow(now).Format(time.RFC3339Nano) + "\n")
		closeErr := file.Close()
		if writeErr != nil {
			_ = os.Remove(path)
			return writeErr
		}
		if closeErr != nil {
			_ = os.Remove(path)
			return closeErr
		}
		return nil
	}
	if err := acquire(); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, statErr := os.Stat(path)
		if statErr != nil || localNow(now).Sub(info.ModTime()) <= time.Minute {
			return nil, fault.Conflict("LOCAL_RUN_MUTATION_IN_PROGRESS", "另一个进程正在更新本地运行上下文")
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := acquire(); err != nil {
			return nil, fault.Conflict("LOCAL_RUN_MUTATION_IN_PROGRESS", "另一个进程正在更新本地运行上下文")
		}
	}
	return func() { _ = os.Remove(path) }, nil
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
