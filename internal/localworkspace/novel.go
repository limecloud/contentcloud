package localworkspace

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
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
		err := domain.Invalid("NOVEL_CONTINUITY_FAILED", "章节连续性校验失败")
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
		return NovelCanon{}, NovelChapter{}, "", domain.Invalid("NOVEL_CANON_JSON_INVALID", err.Error())
	}
	var chapter NovelChapter
	if err := readStrictJSON(chapterPath, &chapter); err != nil {
		return NovelCanon{}, NovelChapter{}, "", domain.Invalid("NOVEL_CHAPTER_JSON_INVALID", err.Error())
	}
	return canon, chapter, resolved, nil
}

func BuildNovelRelease(options BuildNovelReleaseOptions) (BuildNovelReleaseResult, error) {
	canon, chapter, resolved, err := loadNovelFiles(options.Root, options.CanonFile, options.ChapterFile)
	if err != nil {
		return BuildNovelReleaseResult{}, err
	}
	if report := LintNovelContinuity(canon, chapter); !report.Valid {
		validationErr := domain.Invalid("NOVEL_CONTINUITY_FAILED", "章节连续性校验失败")
		validationErr.Details = report
		return BuildNovelReleaseResult{}, validationErr
	}
	if chapter.SchemaVersion != NovelChapterSchema || chapter.ID == "" || chapter.SeriesID != canon.SeriesID || chapter.Status != "approved" || chapter.ApprovedSnapshotID == "" || strings.TrimSpace(chapter.Body) == "" {
		return BuildNovelReleaseResult{}, domain.Policy("NOVEL_CHAPTER_NOT_APPROVED", "只有引用 ApprovedSnapshot 的已批准章节才能生成发布包", "先完成章节审核和批准快照")
	}
	if strings.TrimSpace(options.ProjectID) == "" {
		return BuildNovelReleaseResult{}, domain.Invalid("NOVEL_PROJECT_REQUIRED", "小说发布包必须绑定项目")
	}
	canonDigest, err := domain.CanonicalHash(canon)
	if err != nil {
		return BuildNovelReleaseResult{}, err
	}
	chapterDigest, err := domain.CanonicalHash(chapter)
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
		return BuildNovelReleaseResult{}, domain.Policy("DELIVERY_PATH_OUTSIDE_WORKSPACE", "交付目录必须位于当前工作区", "使用 60-delivery/packages 下的目录")
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
