package localworkspace

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLintNovelContinuityUsesCanonNotAgentMemory(t *testing.T) {
	canon := NovelCanon{SchemaVersion: NovelCanonSchema, SeriesID: "series-1", Version: 3, Characters: []NovelCharacter{{ID: "hero", Name: "主角"}}, Locations: []string{"capital"}, OpenThreads: []NovelThread{{ID: "thread-1", Description: "信件", OpenedIn: 1}}, Timeline: []NovelTimelineEvent{{ID: "event-1", Order: 8, Summary: "到达京城"}}}
	chapter := NovelChapter{ChapterNo: 9, Title: "回信", Summary: "主角在京城打开信件", CharacterRefs: []string{"hero"}, LocationRefs: []string{"capital"}, ResolvedThreads: []string{"thread-1"}, OpenedThreads: []NovelThread{{ID: "thread-2", Description: "失踪者", OpenedIn: 9}}, TimelineOrder: 9}
	if report := LintNovelContinuity(canon, chapter); !report.Valid {
		t.Fatalf("valid chapter failed continuity lint: %#v", report)
	}
	chapter.CharacterRefs = []string{"agent-remembers-this-name"}
	if report := LintNovelContinuity(canon, chapter); report.Valid {
		t.Fatal("unknown character must fail even if an agent claims to remember it")
	}
}

func TestApplyNovelChapterUpdatesCanonDeterministically(t *testing.T) {
	canon := NovelCanon{SchemaVersion: NovelCanonSchema, SeriesID: "series-1", Version: 3, Characters: []NovelCharacter{{ID: "hero", Name: "主角", Aliases: []string{}, Traits: []string{}}}, Locations: []string{"capital"}, WorldRules: []string{}, OpenThreads: []NovelThread{{ID: "thread-1", Description: "信件", OpenedIn: 1}}, Timeline: []NovelTimelineEvent{{ID: "event-1", Order: 8, Summary: "到达京城", Refs: []string{"hero"}}}}
	chapter := NovelChapter{SchemaVersion: NovelChapterSchema, ID: "chapter-9", SeriesID: "series-1", ChapterNo: 9, Title: "回信", Summary: "主角在京城打开信件", Body: "正文", OutlineRef: "outline-1", CharacterRefs: []string{"hero"}, LocationRefs: []string{"capital"}, ResolvedThreads: []string{"thread-1"}, OpenedThreads: []NovelThread{{ID: "thread-2", Description: "失踪者", OpenedIn: 9}}, TimelineOrder: 9, Status: "approved", ApprovedSnapshotID: "snapshot-chapter-9"}
	next, err := ApplyNovelChapter(canon, chapter)
	if err != nil {
		t.Fatal(err)
	}
	if next.Version != 4 || len(next.OpenThreads) != 1 || next.OpenThreads[0].ID != "thread-2" || len(next.Timeline) != 2 || next.Timeline[1].ID != "chapter-9" {
		t.Fatalf("unexpected canon evolution: %#v", next)
	}
	if canon.Version != 3 || canon.OpenThreads[0].ID != "thread-1" || len(canon.Timeline) != 1 {
		t.Fatalf("input canon was mutated: %#v", canon)
	}
}

func TestBuildNovelReleasePinsApprovedChapterAndCanon(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	canon := NovelCanon{SchemaVersion: NovelCanonSchema, SeriesID: "series-1", Version: 4, Characters: []NovelCharacter{{ID: "hero", Name: "主角", Aliases: []string{}, Traits: []string{}}}, Locations: []string{"capital"}, WorldRules: []string{}, OpenThreads: []NovelThread{{ID: "thread-2", Description: "失踪者", OpenedIn: 9}}, Timeline: []NovelTimelineEvent{{ID: "event-1", Order: 8, Summary: "到达京城", Refs: []string{"hero"}}}}
	chapter := NovelChapter{SchemaVersion: NovelChapterSchema, ID: "chapter-10", SeriesID: "series-1", ChapterNo: 10, Title: "追踪", Summary: "主角寻找失踪者", Body: "主角从城门开始寻找线索。", OutlineRef: "outline-1", CharacterRefs: []string{"hero"}, LocationRefs: []string{"capital"}, ResolvedThreads: []string{}, OpenedThreads: []NovelThread{}, TimelineOrder: 10, Status: "approved", ApprovedSnapshotID: "snapshot-chapter-10"}
	canonPath := filepath.Join(root, "40-knowledge", "novel", "canon.json")
	chapterPath := filepath.Join(root, "50-production", "novel", "chapter-10.json")
	if err := replaceJSON(canonPath, canon, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceJSON(chapterPath, chapter, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := BuildNovelRelease(BuildNovelReleaseOptions{Root: root, ProjectID: "project-1", CanonFile: relativeWorkspacePath(root, canonPath), ChapterFile: relativeWorkspacePath(root, chapterPath), Now: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Package.SchemaVersion != NovelReleaseSchema || result.Package.ApprovedSnapshotID != chapter.ApprovedSnapshotID || result.Package.CanonVersion != canon.Version || len(result.Package.Files) != 3 || result.Package.Status != "validated" {
		t.Fatalf("unexpected release package: %#v", result.Package)
	}
	if result.Package.CanonDigest == "" || result.Package.ChapterDigest == "" || result.PackagePath == "" {
		t.Fatalf("release lineage is incomplete: %#v", result)
	}
}
