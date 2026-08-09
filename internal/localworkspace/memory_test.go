package localworkspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemoryRebuildAndQueryUsesSQLiteFTS5AndDerivedScope(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	brief := filepath.Join(root, "10-context", "project-brief.yaml")
	writeMemoryTestFile(t, brief, "目标受众是年轻白领。内容目标是提升品牌信任和复购。\n")
	writeMemoryTestFile(t, filepath.Join(root, "40-work", "focus.md"), "当前焦点：验证年轻白领对品牌内容的反馈。\n")

	rebuilt, err := RebuildMemory(root, time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.State != MemoryStateReady || rebuilt.Backend != MemoryBackendSQLiteFTS5 || rebuilt.SourceCount < 2 || rebuilt.EntryCount == 0 {
		t.Fatalf("unexpected rebuild report: %#v", rebuilt)
	}
	status, err := MemoryStatus(root, time.Now())
	if err != nil || status.State != MemoryStateReady {
		t.Fatalf("memory status is not ready: %#v err=%v", status, err)
	}

	result, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "年轻白领", Limit: 5, MaxChars: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if result.Backend != MemoryBackendSQLiteFTS5 || result.IndexState != MemoryStateReady || len(result.Candidates) == 0 {
		t.Fatalf("SQLite memory query did not return candidates: %#v", result)
	}
	for _, candidate := range result.Candidates {
		if candidate.Scope.WorkspaceID != "workspace-memory" || candidate.Scope.ProjectID != "project-memory" || candidate.Trust != "memory_candidate" || candidate.Status != "active" || candidate.SourceDigest == "" {
			t.Fatalf("candidate lost derived scope or provenance: %#v", candidate)
		}
	}

	shortQuery, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "白领", Limit: 5, MaxChars: 2000})
	if err != nil || shortQuery.Backend != MemoryBackendScan || len(shortQuery.Candidates) == 0 {
		t.Fatalf("short CJK query must use safe scan fallback: %#v err=%v", shortQuery, err)
	}
}

func TestMemoryQueryInvalidatesChangedAndDeletedSources(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	path := filepath.Join(root, "30-knowledge", "pages", "fact.md")
	writeMemoryTestFile(t, path, "旧事实：产品颜色是红色。\n")
	if _, err := RebuildMemory(root, time.Now()); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("新事实：产品颜色是蓝色。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, originalInfo.ModTime(), originalInfo.ModTime()); err != nil {
		t.Fatal(err)
	}

	oldResult, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "红色", Limit: 5, MaxChars: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if oldResult.IndexState != MemoryStateStale || len(oldResult.Candidates) != 0 {
		t.Fatalf("changed source must invalidate old candidate: %#v", oldResult)
	}
	newResult, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "蓝色", Limit: 5, MaxChars: 2000})
	if err != nil || len(newResult.Candidates) == 0 || newResult.Backend != MemoryBackendScan {
		t.Fatalf("changed source must be available through current-file fallback: %#v err=%v", newResult, err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	deletedResult, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "蓝色", Limit: 5, MaxChars: 2000})
	if err != nil || deletedResult.IndexState != MemoryStateStale || len(deletedResult.Candidates) != 0 {
		t.Fatalf("deleted source must disappear from recall: %#v err=%v", deletedResult, err)
	}
}

func TestMemoryPermissionChangesInvalidateIndex(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	path := filepath.Join(root, "30-knowledge", "pages", "permission.md")
	writeMemoryTestFile(t, path, "权限变化也必须让记忆索引陈旧。\n")
	if _, err := RebuildMemory(root, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "权限变化", Limit: 5, MaxChars: 2000})
	if err != nil || result.IndexState != MemoryStateStale || result.Backend != MemoryBackendScan || len(result.Candidates) == 0 {
		t.Fatalf("permission changes must invalidate indexed results and use current-file fallback: %#v err=%v", result, err)
	}
}

func TestMemoryPermissionNarrowingTombstonesRememberedCandidate(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	path := filepath.Join(root, "40-work", "permission-narrowed.md")
	writeMemoryTestFile(t, path, "权限收窄后不能继续召回的候选。\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RememberMemory(MemoryRememberOptions{Root: root, MemoryID: "memr_permission_narrowed", Kind: "working", SourceRef: "40-work/permission-narrowed.md", Summary: "权限收窄候选"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := scanMemoryCatalog(root, MemoryScope{WorkspaceID: "workspace-memory", ProjectID: "project-memory"})
	if err != nil || len(catalog.Records) != 1 || catalog.Records[0].Record.Status != "tombstoned" {
		t.Fatalf("permission narrowing must tombstone the record: %#v err=%v", catalog.Records, err)
	}
	result, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "权限收窄候选", Limit: 5, MaxChars: 2000})
	if err != nil || len(result.Candidates) != 0 || result.IndexState != MemoryStateMissing {
		t.Fatalf("tombstoned candidate must not be recalled: %#v err=%v", result, err)
	}
}

func TestMemoryRejectsCopiedScopeAndCorruptProjection(t *testing.T) {
	first := initMemoryTestWorkspace(t)
	writeMemoryTestFile(t, filepath.Join(first, "10-context", "project-brief.yaml"), "工作区一的受限事实。\n")
	if _, err := RebuildMemory(first, time.Now()); err != nil {
		t.Fatal(err)
	}
	second := initMemoryTestWorkspaceWithScope(t, "workspace-memory-two", "project-memory-two")
	writeMemoryTestFile(t, filepath.Join(second, "10-context", "project-brief.yaml"), "工作区二的独立事实。\n")
	firstIndex, err := os.ReadFile(memoryIndexPath(first))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memoryIndexPath(second), firstIndex, 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := MemoryStatus(second, time.Now())
	if err != nil || status.State != MemoryStateIncompatible {
		t.Fatalf("copied projection must not cross scopes: %#v err=%v", status, err)
	}
	result, err := QueryMemory(MemoryQueryOptions{Root: second, Query: "工作区二", Limit: 5, MaxChars: 2000})
	if err != nil || len(result.Candidates) == 0 || result.Candidates[0].Scope.WorkspaceID != "workspace-memory-two" {
		t.Fatalf("scope fallback did not return current workspace: %#v err=%v", result, err)
	}

	if err := os.WriteFile(memoryIndexPath(second), []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = MemoryStatus(second, time.Now())
	if err != nil || status.State != MemoryStateCorrupt {
		t.Fatalf("corrupt projection must be diagnosed: %#v err=%v", status, err)
	}
	result, err = QueryMemory(MemoryQueryOptions{Root: second, Query: "工作区二", Limit: 5, MaxChars: 2000})
	if err != nil || result.Backend != MemoryBackendScan || len(result.Candidates) == 0 {
		t.Fatalf("corrupt projection must fall back to current files: %#v err=%v", result, err)
	}
}

func TestMemorySkipsSymlinkBinaryAndOversizedFiles(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	writeMemoryTestFile(t, filepath.Join(root, "30-knowledge", "pages", "safe.md"), "安全事实：只允许读取工作区内的文本。\n")
	external := filepath.Join(t.TempDir(), "outside.txt")
	writeMemoryTestFile(t, external, "不应被索引的外部秘密。\n")
	link := filepath.Join(root, "30-knowledge", "pages", "linked.md")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlink test is unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "30-knowledge", "pages", "binary.bin"), []byte{0, 1, 2}, 0o600); err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("x", memoryMaxFileBytes+1)
	writeMemoryTestFile(t, filepath.Join(root, "30-knowledge", "pages", "large.txt"), large)
	report, err := RebuildMemory(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.SkippedCount < 2 || len(report.Warnings) == 0 {
		t.Fatalf("unsafe sources must be skipped and reported: %#v", report)
	}
	result, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "外部秘密", Limit: 5, MaxChars: 2000})
	if err != nil || len(result.Candidates) != 0 {
		t.Fatalf("external symlink content must not be recalled: %#v err=%v", result, err)
	}
}

func TestMemoryClearRemovesSQLiteSidecars(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	writeMemoryTestFile(t, filepath.Join(root, "10-context", "project-brief.yaml"), "需要清理索引缓存的工作区。\n")
	if _, err := RebuildMemory(root, time.Now()); err != nil {
		t.Fatal(err)
	}
	base := memoryIndexPath(root)
	for _, suffix := range []string{"-wal", "-shm", "-journal", ".backup", ".tmp-test"} {
		if err := os.WriteFile(base+suffix, []byte("sidecar"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	report, err := ClearMemory(root, time.Now())
	if err != nil || !report.Cleared {
		t.Fatalf("memory clear failed: %#v err=%v", report, err)
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal", ".backup", ".tmp-test"} {
		if _, statErr := os.Stat(base + suffix); !os.IsNotExist(statErr) {
			t.Fatalf("memory artifact %q remains after clear: %v", base+suffix, statErr)
		}
	}
}

func TestRememberMemoryBindsSourceAndSurvivesIndexClear(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	source := filepath.Join(root, "40-work", "focus.md")
	writeMemoryTestFile(t, source, "当前焦点：验证年轻白领内容反馈。\n")
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	recorded, err := RememberMemory(MemoryRememberOptions{Root: root, MemoryID: "memr_focus", Kind: "working", SourceRef: "40-work/focus.md", Summary: "当前项目优先验证年轻白领反馈。", FormedBy: "codex/test", Now: now})
	if err != nil || recorded.Record.MemoryID != "memr_focus" || recorded.Record.SourceDigest == "" || recorded.Record.SourceMode == 0 {
		t.Fatalf("unexpected memory record: %#v err=%v", recorded, err)
	}
	if _, err := RememberMemory(MemoryRememberOptions{Root: root, MemoryID: "memr_focus", Kind: "working", SourceRef: "40-work/focus.md", Summary: "不同摘要", Now: now}); err == nil {
		t.Fatal("same memory ID must reject immutable content changes")
	}
	if _, err := RebuildMemory(root, now); err != nil {
		t.Fatal(err)
	}
	result, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "优先验证", Limit: 5, MaxChars: 2000, Now: now})
	if err != nil || len(result.Candidates) == 0 || result.Candidates[0].MemoryID != "memr_focus" {
		t.Fatalf("remembered candidate was not indexed: %#v err=%v", result, err)
	}
	if _, err := ClearMemory(root, now); err != nil {
		t.Fatal(err)
	}
	rebuiltAfterClear, err := RebuildMemory(root, now.Add(time.Minute))
	if err != nil || rebuiltAfterClear.EntryCount == 0 {
		t.Fatalf("remembered record did not survive projection clear: %#v err=%v", rebuiltAfterClear, err)
	}
	if err := os.WriteFile(source, []byte("当前焦点：改为验证蓝领反馈。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "优先验证", Limit: 5, MaxChars: 2000, Now: now.Add(2 * time.Minute)})
	if err != nil || stale.IndexState != MemoryStateStale || len(stale.Candidates) != 0 {
		t.Fatalf("source changes must stale remembered candidates: %#v err=%v", stale, err)
	}
}

func TestRememberMemoryIDCollisionWithGeneratedSourceChunkIsDeterministic(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	sourceRef := "40-work/collision.md"
	writeMemoryTestFile(t, filepath.Join(root, filepath.FromSlash(sourceRef)), "来源块与显式候选使用相同的初始 ID。\n")
	_, scope, err := resolveMemoryScope(root)
	if err != nil {
		t.Fatal(err)
	}
	generatedID := memoryID(scope, sourceRef, 0)
	if _, err := RememberMemory(MemoryRememberOptions{Root: root, MemoryID: generatedID, Kind: "working", SourceRef: sourceRef, Summary: "显式候选必须保留稳定 ID"}); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := RebuildMemory(root, time.Now())
	if err != nil || rebuilt.EntryCount < 2 {
		t.Fatalf("ID collision must not break deterministic rebuild: %#v err=%v", rebuilt, err)
	}
	result, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "稳定", Limit: 5, MaxChars: 2000})
	if err != nil || len(result.Candidates) != 1 || result.Candidates[0].MemoryID != generatedID {
		t.Fatalf("explicit candidate lost after collision handling: %#v err=%v", result, err)
	}
}

func TestRememberMemoryRejectsUnindexedSource(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	writeMemoryTestFile(t, filepath.Join(root, "20-sources", "originals", "secret.txt"), "原始素材不直接作为记忆来源。\n")
	if _, err := RememberMemory(MemoryRememberOptions{Root: root, Kind: "knowledge", SourceRef: "20-sources/originals/secret.txt", Summary: "不应被记住"}); err == nil {
		t.Fatal("original material must not be accepted as a memory source")
	}
}

func TestMemoryConsolidationBlocksConflictsAndDeduplicatesExactRecords(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	source := filepath.Join(root, "40-work", "claims.md")
	writeMemoryTestFile(t, source, "项目主张来源。\n")
	if _, err := RememberMemory(MemoryRememberOptions{Root: root, MemoryID: "memr-1", Kind: "knowledge", ClaimKey: "product.price", SourceRef: "40-work/claims.md", Summary: "产品价格为 168 元"}); err != nil {
		t.Fatal(err)
	}
	if _, err := RememberMemory(MemoryRememberOptions{Root: root, MemoryID: "memr-2", Kind: "knowledge", ClaimKey: "product.price", SourceRef: "40-work/claims.md", Summary: "产品价格为 168 元"}); err != nil {
		t.Fatal(err)
	}
	if _, err := RememberMemory(MemoryRememberOptions{Root: root, MemoryID: "memr-3", Kind: "knowledge", ClaimKey: "product.price", SourceRef: "40-work/claims.md", Summary: "产品价格为 188 元"}); err != nil {
		t.Fatal(err)
	}
	report, err := ConsolidateMemory(MemoryConsolidationOptions{Root: root})
	if err != nil || report.DuplicateCount != 1 || report.ConflictCount != 3 || len(report.Duplicates) != 1 || len(report.Conflicts) != 1 {
		t.Fatalf("unexpected consolidation report: %#v err=%v", report, err)
	}
	if _, err := RebuildMemory(root, time.Now()); err != nil {
		t.Fatal(err)
	}
	query, err := QueryMemory(MemoryQueryOptions{Root: root, Query: "产品价格", Limit: 10, MaxChars: 4000})
	if err != nil || query.ConflictCount != 3 || len(query.Candidates) != 0 {
		t.Fatalf("conflicted candidates must not enter default recall: %#v err=%v", query, err)
	}
}

func TestMemoryPromoteUsesAcceptedEvidenceAndLeavesCandidateUnapproved(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	meterial := filepath.Join(t.TempDir(), "product.txt")
	if err := os.WriteFile(meterial, []byte("产品建议零售价为168元。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterLocalSource(RegisterLocalSourceOptions{Root: root, File: meterial, ID: "source:product", StorageMode: "copy"}); err != nil {
		t.Fatal(err)
	}
	bundle, err := IngestLocalSource(root, "source:product", time.Time{})
	if err != nil || len(bundle.Evidence) == 0 {
		t.Fatalf("source ingest failed: %#v err=%v", bundle, err)
	}
	writeMemoryTestFile(t, filepath.Join(root, "40-work", "memory-source.md"), "产品价格来自已复核来源。\n")
	if _, err := RememberMemory(MemoryRememberOptions{Root: root, MemoryID: "memr-promote", Kind: "knowledge", SourceRef: "40-work/memory-source.md", Summary: "产品建议零售价为 168 元"}); err != nil {
		t.Fatal(err)
	}
	promoted, err := PromoteMemory(MemoryPromoteOptions{Root: root, MemoryID: "memr-promote", KnowledgeKind: "fact", Subject: "产品", Predicate: "建议零售价", EvidenceIDs: []string{bundle.Evidence[0].ID}, Now: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)})
	if err != nil || promoted.KnowledgeID == "" || len(promoted.Imported) != 1 || promoted.Imported[0].Status != "candidate" {
		t.Fatalf("memory promotion failed: %#v err=%v", promoted, err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(promoted.PackageRef))); err != nil {
		t.Fatalf("promotion package missing: %v", err)
	}
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: root})
	if err != nil || len(query.Informational) != 1 || len(query.Eligible) != 0 {
		t.Fatalf("promoted knowledge must remain informational before approval: %#v err=%v", query, err)
	}
}

func initMemoryTestWorkspace(t *testing.T) string {
	t.Helper()
	return initMemoryTestWorkspaceWithScope(t, "workspace-memory", "project-memory")
}

func initMemoryTestWorkspaceWithScope(t *testing.T, workspaceID, projectID string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, WorkspaceID: workspaceID, ProjectID: projectID, Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeMemoryTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
