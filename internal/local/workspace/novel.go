package localworkspace

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	exportfmt "github.com/limecloud/contentcloud/internal/local/export"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

const (
	NovelCanonSchema   = "contentcloud.novel-canon/1.0"
	NovelOutlineSchema = "contentcloud.novel-outline/1.0"
	NovelChapterSchema = "contentcloud.novel-chapter/1.0"
	NovelReleaseSchema = "contentcloud.novel-release/1.0"
)

type NovelCharacter struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Traits  []string `json:"traits"`
}

type NovelThread struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	OpenedIn    int    `json:"opened_in"`
}

type NovelTimelineEvent struct {
	ID      string   `json:"id"`
	Order   int      `json:"order"`
	Summary string   `json:"summary"`
	Refs    []string `json:"refs"`
}

type NovelCanon struct {
	SchemaVersion string               `json:"schema_version"`
	SeriesID      string               `json:"series_id"`
	Version       int                  `json:"version"`
	Characters    []NovelCharacter     `json:"characters"`
	Locations     []string             `json:"locations"`
	WorldRules    []string             `json:"world_rules"`
	OpenThreads   []NovelThread        `json:"open_threads"`
	Timeline      []NovelTimelineEvent `json:"timeline"`
}

type NovelChapter struct {
	SchemaVersion      string        `json:"schema_version,omitempty"`
	ID                 string        `json:"id,omitempty"`
	SeriesID           string        `json:"series_id,omitempty"`
	ChapterNo          int           `json:"chapter_no"`
	Title              string        `json:"title"`
	Summary            string        `json:"summary"`
	Body               string        `json:"body,omitempty"`
	OutlineRef         string        `json:"outline_ref,omitempty"`
	CharacterRefs      []string      `json:"character_refs"`
	LocationRefs       []string      `json:"location_refs"`
	ResolvedThreads    []string      `json:"resolved_threads"`
	OpenedThreads      []NovelThread `json:"opened_threads"`
	TimelineOrder      int           `json:"timeline_order"`
	Status             string        `json:"status,omitempty"`
	ApprovedSnapshotID string        `json:"approved_snapshot_id,omitempty"`
}

// ValidateNovelChapterForSubmission validates the business payload before it
// enters the shared Submission chain. A chapter is review-ready here; the
// immutable ApprovedSnapshot is the only place where approval is recorded.
func ValidateNovelChapterForSubmission(raw json.RawMessage) (NovelChapter, error) {
	var chapter NovelChapter
	if err := strictUnmarshal(raw, &chapter); err != nil {
		return chapter, fault.Invalid("NOVEL_CHAPTER_JSON_INVALID", err.Error())
	}
	if chapter.SchemaVersion != NovelChapterSchema || strings.TrimSpace(chapter.ID) == "" || strings.TrimSpace(chapter.SeriesID) == "" {
		return chapter, fault.Invalid("NOVEL_CHAPTER_IDENTITY_INVALID", "小说章节需要有效的 schema、章节标识和系列标识")
	}
	if chapter.ChapterNo < 1 || strings.TrimSpace(chapter.Title) == "" || strings.TrimSpace(chapter.Summary) == "" || strings.TrimSpace(chapter.Body) == "" || strings.TrimSpace(chapter.OutlineRef) == "" || chapter.TimelineOrder < 1 {
		return chapter, fault.Invalid("NOVEL_CHAPTER_FIELDS_REQUIRED", "小说章节需要编号、标题、摘要、正文、大纲引用和时间线顺序")
	}
	if chapter.Status != "review_ready" || strings.TrimSpace(chapter.ApprovedSnapshotID) != "" {
		return chapter, fault.Policy("NOVEL_CHAPTER_NOT_REVIEW_READY", "只有 review_ready 且未携带批准快照的章节才能提交", "将章节修订为 review_ready 后重新提交")
	}
	if err := validateNovelChapterCollections(chapter); err != nil {
		return chapter, err
	}
	return chapter, nil
}

func validateNovelChapterCollections(chapter NovelChapter) error {
	if !uniqueStringsAreValid(chapter.CharacterRefs) || !uniqueStringsAreValid(chapter.LocationRefs) || !uniqueStringsAreValid(chapter.ResolvedThreads) {
		return fault.Invalid("NOVEL_CHAPTER_REFERENCES_INVALID", "小说章节引用集合必须是非空且唯一的字符串")
	}
	seen := map[string]bool{}
	for _, thread := range chapter.OpenedThreads {
		if strings.TrimSpace(thread.ID) == "" || strings.TrimSpace(thread.Description) == "" || thread.OpenedIn != chapter.ChapterNo || seen[thread.ID] {
			return fault.Invalid("NOVEL_CHAPTER_THREAD_INVALID", "小说章节新增伏笔必须有唯一标识、描述并绑定当前章节")
		}
		seen[thread.ID] = true
	}
	return nil
}

func uniqueStringsAreValid(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

// RenderNovelChapterDelivery turns an approved snapshot object into stable
// files. The snapshot, not the chapter status field, is the approval boundary.
func RenderNovelChapterDelivery(raw json.RawMessage) (RenderedContentDelivery, error) {
	var chapter NovelChapter
	if err := strictUnmarshal(raw, &chapter); err != nil {
		return RenderedContentDelivery{}, fault.Invalid("NOVEL_CHAPTER_JSON_INVALID", err.Error())
	}
	if chapter.SchemaVersion != NovelChapterSchema || strings.TrimSpace(chapter.ID) == "" || strings.TrimSpace(chapter.Body) == "" || (chapter.Status != "review_ready" && chapter.Status != "approved") {
		return RenderedContentDelivery{}, fault.Policy("NOVEL_CHAPTER_DELIVERY_BLOCKED", "只有已通过审批流程的小说章节才能生成交付文件", "完成章节内审和客户审批后重试")
	}
	jsonBody, err := json.MarshalIndent(chapter, "", "  ")
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	jsonBody = append(jsonBody, '\n')
	markdown := []byte(renderNovelChapterMarkdown(chapter))
	rows := [][]string{{"字段", "值"}, {"系列", chapter.SeriesID}, {"章节", fmt.Sprintf("第 %d 章", chapter.ChapterNo)}, {"标题", chapter.Title}, {"摘要", chapter.Summary}, {"时间线顺序", fmt.Sprint(chapter.TimelineOrder)}, {"大纲引用", chapter.OutlineRef}, {"角色引用", strings.Join(chapter.CharacterRefs, ", ")}, {"地点引用", strings.Join(chapter.LocationRefs, ", ")}, {"正文", chapter.Body}}
	xlsx, err := exportfmt.XLSX("小说章节", rows)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	hash, err := stablehash.Sum(chapter)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	files := []RenderedContentFile{
		{Format: "json", Name: "novel-chapter.json", MediaType: "application/json", Body: jsonBody, SHA256: digest(jsonBody)},
		{Format: "markdown", Name: "novel-chapter.md", MediaType: "text/markdown", Body: markdown, SHA256: digest(markdown)},
		{Format: "xlsx", Name: "novel-chapter.xlsx", MediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Body: xlsx, SHA256: digest(xlsx)},
	}
	return RenderedContentDelivery{ItemID: chapter.ID, SchemaID: NovelChapterSchema, ContentHash: "sha256:" + hash, Files: files}, nil
}

func renderNovelChapterMarkdown(chapter NovelChapter) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# 第 %d 章：%s\n\n", chapter.ChapterNo, chapter.Title)
	fmt.Fprintf(&out, "系列：%s\n\n摘要：%s\n\n", chapter.SeriesID, chapter.Summary)
	if len(chapter.CharacterRefs) > 0 {
		fmt.Fprintf(&out, "角色：%s\n\n", strings.Join(chapter.CharacterRefs, "、"))
	}
	if len(chapter.LocationRefs) > 0 {
		fmt.Fprintf(&out, "地点：%s\n\n", strings.Join(chapter.LocationRefs, "、"))
	}
	out.WriteString(chapter.Body)
	out.WriteString("\n")
	return out.String()
}

type NovelOutlineChapter struct {
	ChapterNo     int      `json:"chapter_no"`
	Title         string   `json:"title"`
	Goal          string   `json:"goal"`
	CharacterRefs []string `json:"character_refs"`
	ThreadRefs    []string `json:"thread_refs"`
}

type NovelOutline struct {
	SchemaVersion string                `json:"schema_version"`
	ID            string                `json:"id"`
	SeriesID      string                `json:"series_id"`
	CanonVersion  int                   `json:"canon_version"`
	Volume        int                   `json:"volume"`
	Arc           string                `json:"arc"`
	Chapters      []NovelOutlineChapter `json:"chapters"`
}

type NovelReleaseFile struct {
	Format    string `json:"format"`
	Path      string `json:"path"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	ByteSize  int64  `json:"byte_size"`
}

type NovelReleaseCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type NovelRelease struct {
	SchemaVersion      string              `json:"schema_version"`
	ID                 string              `json:"id"`
	ProjectID          string              `json:"project_id"`
	SeriesID           string              `json:"series_id"`
	ChapterID          string              `json:"chapter_id"`
	ChapterNo          int                 `json:"chapter_no"`
	ApprovedSnapshotID string              `json:"approved_snapshot_id"`
	CanonVersion       int                 `json:"canon_version"`
	CanonDigest        string              `json:"canon_digest"`
	ChapterDigest      string              `json:"chapter_digest"`
	ChannelProfileRef  string              `json:"channel_profile_ref"`
	Files              []NovelReleaseFile  `json:"files"`
	ExternalActions    []string            `json:"external_actions"`
	Checks             []NovelReleaseCheck `json:"checks"`
	Status             string              `json:"status"`
	CreatedAt          time.Time           `json:"created_at"`
}

type BuildNovelReleaseOptions struct {
	Root              string
	ProjectID         string
	CanonFile         string
	ChapterFile       string
	OutputDirectory   string
	ChannelProfileRef string
	Now               time.Time
}

type BuildNovelReleaseResult struct {
	PackagePath string       `json:"package_path"`
	Package     NovelRelease `json:"package"`
}

type ContinuityReport struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func LintNovelContinuity(canon NovelCanon, chapter NovelChapter) ContinuityReport {
	report := ContinuityReport{Valid: true, Errors: []string{}, Warnings: []string{}}
	if canon.SchemaVersion != NovelCanonSchema || canon.SeriesID == "" || canon.Version < 1 {
		report.Errors = append(report.Errors, "canon schema、series_id 或 version 无效")
	}
	if chapter.ChapterNo < 1 || strings.TrimSpace(chapter.Title) == "" || strings.TrimSpace(chapter.Summary) == "" {
		report.Errors = append(report.Errors, "章节缺少编号、标题或摘要")
	}
	if chapter.SchemaVersion != "" && chapter.SchemaVersion != NovelChapterSchema {
		report.Errors = append(report.Errors, "章节 schema_version 无效")
	}
	if chapter.SeriesID != "" && chapter.SeriesID != canon.SeriesID {
		report.Errors = append(report.Errors, "章节 series_id 与 Canon 不一致")
	}
	characters := map[string]bool{}
	for _, character := range canon.Characters {
		if character.ID == "" || character.Name == "" || characters[character.ID] {
			report.Errors = append(report.Errors, "Canon 存在无效或重复角色 ID")
			continue
		}
		characters[character.ID] = true
	}
	for _, ref := range chapter.CharacterRefs {
		if !characters[ref] {
			report.Errors = append(report.Errors, fmt.Sprintf("章节引用未知角色 %s", ref))
		}
	}
	locations := map[string]bool{}
	for _, location := range canon.Locations {
		locations[location] = true
	}
	for _, ref := range chapter.LocationRefs {
		if !locations[ref] {
			report.Errors = append(report.Errors, fmt.Sprintf("章节引用未知地点 %s", ref))
		}
	}
	threads := map[string]bool{}
	for _, thread := range canon.OpenThreads {
		threads[thread.ID] = true
	}
	for _, id := range chapter.ResolvedThreads {
		if !threads[id] {
			report.Errors = append(report.Errors, fmt.Sprintf("章节解决了未知或已关闭伏笔 %s", id))
		}
	}
	opened := map[string]bool{}
	for _, thread := range chapter.OpenedThreads {
		if thread.ID == "" || thread.OpenedIn != chapter.ChapterNo || threads[thread.ID] || opened[thread.ID] {
			report.Errors = append(report.Errors, "新伏笔必须使用唯一 ID 并绑定当前章节")
		}
		opened[thread.ID] = true
	}
	maxOrder := 0
	seenOrder := map[int]bool{}
	for _, event := range canon.Timeline {
		if event.Order < 1 || seenOrder[event.Order] {
			report.Errors = append(report.Errors, "Canon 时间线顺序无效或重复")
		}
		seenOrder[event.Order] = true
		if event.Order > maxOrder {
			maxOrder = event.Order
		}
	}
	if chapter.TimelineOrder <= maxOrder {
		report.Errors = append(report.Errors, "章节时间线不能早于或覆盖已发布 Canon 事件")
	}
	if len(chapter.CharacterRefs) == 0 {
		report.Warnings = append(report.Warnings, "章节没有显式角色引用")
	}
	sort.Strings(report.Errors)
	sort.Strings(report.Warnings)
	report.Valid = len(report.Errors) == 0
	return report
}

// ApplyNovelChapter is the only deterministic Canon evolution operation. Agent
// output remains a chapter candidate until this function validates it.
func ApplyNovelChapter(canon NovelCanon, chapter NovelChapter) (NovelCanon, error) {
	report := LintNovelContinuity(canon, chapter)
	if !report.Valid {
		err := fault.Invalid("NOVEL_CONTINUITY_FAILED", "章节连续性校验失败")
		err.Details = report
		return NovelCanon{}, err
	}
	resolved := map[string]bool{}
	for _, id := range chapter.ResolvedThreads {
		resolved[id] = true
	}
	next := canon
	next.Version++
	next.Characters = append([]NovelCharacter{}, canon.Characters...)
	next.Locations = append([]string{}, canon.Locations...)
	next.WorldRules = append([]string{}, canon.WorldRules...)
	next.OpenThreads = make([]NovelThread, 0, len(canon.OpenThreads)+len(chapter.OpenedThreads))
	for _, thread := range canon.OpenThreads {
		if !resolved[thread.ID] {
			next.OpenThreads = append(next.OpenThreads, thread)
		}
	}
	next.OpenThreads = append(next.OpenThreads, chapter.OpenedThreads...)
	next.Timeline = append([]NovelTimelineEvent{}, canon.Timeline...)
	refs := uniqueStrings(append(append(append([]string{}, chapter.CharacterRefs...), chapter.LocationRefs...), append(chapter.ResolvedThreads, threadIDs(chapter.OpenedThreads)...)...))
	next.Timeline = append(next.Timeline, NovelTimelineEvent{ID: fmt.Sprintf("chapter-%d", chapter.ChapterNo), Order: chapter.TimelineOrder, Summary: chapter.Summary, Refs: refs})
	return next, nil
}

func ApplyNovelChapterFiles(root, canonFile, chapterFile, outputFile string) (NovelCanon, error) {
	canon, chapter, _, err := loadNovelFiles(root, canonFile, chapterFile)
	if err != nil {
		return NovelCanon{}, err
	}
	next, err := ApplyNovelChapter(canon, chapter)
	if err != nil {
		return NovelCanon{}, err
	}
	resolved, err := FindRoot(root)
	if err != nil {
		return NovelCanon{}, err
	}
	if strings.TrimSpace(outputFile) == "" {
		outputFile = canonFile
	}
	outputPath, err := resolveWorkspaceFile(resolved, outputFile)
	if err != nil {
		return NovelCanon{}, err
	}
	if err := replaceJSON(outputPath, next, 0o600); err != nil {
		return NovelCanon{}, err
	}
	return next, nil
}

func LintNovelContinuityFiles(root, canonFile, chapterFile string) (ContinuityReport, error) {
	canon, chapter, _, err := loadNovelFiles(root, canonFile, chapterFile)
	if err != nil {
		return ContinuityReport{}, err
	}
	return LintNovelContinuity(canon, chapter), nil
}

func loadNovelFiles(root, canonFile, chapterFile string) (NovelCanon, NovelChapter, string, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return NovelCanon{}, NovelChapter{}, "", err
	}
	canonPath, err := resolveWorkspaceFile(resolved, canonFile)
	if err != nil {
		return NovelCanon{}, NovelChapter{}, "", err
	}
	chapterPath, err := resolveWorkspaceFile(resolved, chapterFile)
	if err != nil {
		return NovelCanon{}, NovelChapter{}, "", err
	}
	var canon NovelCanon
	if err := readStrictJSON(canonPath, &canon); err != nil {
		return NovelCanon{}, NovelChapter{}, "", fault.Invalid("NOVEL_CANON_JSON_INVALID", err.Error())
	}
	var chapter NovelChapter
	if err := readStrictJSON(chapterPath, &chapter); err != nil {
		return NovelCanon{}, NovelChapter{}, "", fault.Invalid("NOVEL_CHAPTER_JSON_INVALID", err.Error())
	}
	return canon, chapter, resolved, nil
}

func BuildNovelRelease(options BuildNovelReleaseOptions) (BuildNovelReleaseResult, error) {
	canon, chapter, resolved, err := loadNovelFiles(options.Root, options.CanonFile, options.ChapterFile)
	if err != nil {
		return BuildNovelReleaseResult{}, err
	}
	if report := LintNovelContinuity(canon, chapter); !report.Valid {
		validationErr := fault.Invalid("NOVEL_CONTINUITY_FAILED", "章节连续性校验失败")
		validationErr.Details = report
		return BuildNovelReleaseResult{}, validationErr
	}
	if chapter.SchemaVersion != NovelChapterSchema || chapter.ID == "" || chapter.SeriesID != canon.SeriesID || chapter.Status != "approved" || chapter.ApprovedSnapshotID == "" || strings.TrimSpace(chapter.Body) == "" {
		return BuildNovelReleaseResult{}, fault.Policy("NOVEL_CHAPTER_NOT_APPROVED", "只有引用 ApprovedSnapshot 的已批准章节才能生成发布包", "先完成章节审核和批准快照")
	}
	if strings.TrimSpace(options.ProjectID) == "" {
		return BuildNovelReleaseResult{}, fault.Invalid("NOVEL_PROJECT_REQUIRED", "小说发布包必须绑定项目")
	}
	canonDigest, err := stablehash.Sum(canon)
	if err != nil {
		return BuildNovelReleaseResult{}, err
	}
	chapterDigest, err := stablehash.Sum(chapter)
	if err != nil {
		return BuildNovelReleaseResult{}, err
	}
	packageID := "novel-release-" + chapterDigest[:12]
	packageRoot := options.OutputDirectory
	if strings.TrimSpace(packageRoot) == "" {
		packageRoot = filepath.Join(resolved, "60-delivery", "packages", packageID)
	} else if !filepath.IsAbs(packageRoot) {
		packageRoot = filepath.Join(resolved, filepath.FromSlash(packageRoot))
	}
	packageRoot, err = filepath.Abs(packageRoot)
	if err != nil {
		return BuildNovelReleaseResult{}, err
	}
	relative, err := filepath.Rel(resolved, packageRoot)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return BuildNovelReleaseResult{}, fault.Policy("DELIVERY_PATH_OUTSIDE_WORKSPACE", "交付目录必须位于当前工作区", "使用 60-delivery/packages 下的目录")
	}
	canonBody, _ := json.MarshalIndent(canon, "", "  ")
	canonBody = append(canonBody, '\n')
	chapterBody, _ := json.MarshalIndent(chapter, "", "  ")
	chapterBody = append(chapterBody, '\n')
	textBody := []byte(chapter.Title + "\n\n" + chapter.Body + "\n")
	outputs := []struct {
		format, path, mediaType string
		body                    []byte
	}{{"chapter_json", "chapter.json", "application/json", chapterBody}, {"chapter_text", "chapter.txt", "text/plain", textBody}, {"canon_json", "canon.json", "application/json", canonBody}}
	files := make([]NovelReleaseFile, 0, len(outputs))
	for _, output := range outputs {
		if err := replaceFile(filepath.Join(packageRoot, output.path), output.body, 0o600); err != nil {
			return BuildNovelReleaseResult{}, err
		}
		files = append(files, NovelReleaseFile{Format: output.format, Path: output.path, MediaType: output.mediaType, SHA256: "sha256:" + digest(output.body), ByteSize: int64(len(output.body))})
	}
	channelProfileRef := strings.TrimSpace(options.ChannelProfileRef)
	if channelProfileRef == "" {
		channelProfileRef = "channel:web-novel@1.0.0"
	}
	now := localNow(options.Now)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	release := NovelRelease{SchemaVersion: NovelReleaseSchema, ID: packageID, ProjectID: options.ProjectID, SeriesID: canon.SeriesID, ChapterID: chapter.ID, ChapterNo: chapter.ChapterNo, ApprovedSnapshotID: chapter.ApprovedSnapshotID, CanonVersion: canon.Version, CanonDigest: "sha256:" + canonDigest, ChapterDigest: "sha256:" + chapterDigest, ChannelProfileRef: channelProfileRef, Files: files, ExternalActions: []string{"manual_login", "manual_preview", "manual_publish", "record_external_binding"}, Checks: []NovelReleaseCheck{{Name: "approved_snapshot", Status: "passed"}, {Name: "canon_continuity", Status: "passed"}, {Name: "delivery_integrity", Status: "passed"}}, Status: "validated", CreatedAt: now}
	packagePath := filepath.Join(packageRoot, "package.json")
	if err := replaceJSON(packagePath, release, 0o600); err != nil {
		return BuildNovelReleaseResult{}, err
	}
	return BuildNovelReleaseResult{PackagePath: relativeWorkspacePath(resolved, packagePath), Package: release}, nil
}

func threadIDs(threads []NovelThread) []string {
	result := make([]string, 0, len(threads))
	for _, thread := range threads {
		result = append(result, thread.ID)
	}
	return result
}
