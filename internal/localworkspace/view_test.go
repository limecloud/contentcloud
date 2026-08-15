package localworkspace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestWorkspaceViewReadsFilesDirectoriesAndDigestBoundResources(t *testing.T) {
	root := workspaceViewFixture(t)
	ref := "50-production/脚本 候选.md"
	body := []byte("# 脚本候选\n\n内容来自本地工作区。\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), body, 0o600); err != nil {
		t.Fatal(err)
	}

	view, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "content_item", Ref: ref})
	if err != nil {
		t.Fatal(err)
	}
	if view.WorkspaceID != "workspace-view" || view.ProjectID != "project-view" || view.View.Text != string(body) || view.View.MIMEType != "text/markdown" || view.ObservedDigest == "" {
		t.Fatalf("unexpected workspace view: %#v", view)
	}
	if len(view.Resources) != 1 || len(view.View.Actions) != 0 {
		t.Fatalf("workspace view must expose only the digest-bound source resource: %#v", view)
	}
	resource, err := ReadWorkspaceResource(root, view.Resources[0].URI)
	if err != nil || resource.Text != string(body) || resource.MIMEType != "text/markdown" {
		t.Fatalf("digest-bound resource did not round-trip: resource=%#v err=%v", resource, err)
	}

	directory, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "file", Ref: "50-production"})
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := directory.View.Data.([]WorkspaceDirectoryEntry)
	found := false
	for _, entry := range entries {
		if entry.Ref == ref && entry.Kind == "file" && entry.MIMEType == "text/markdown" {
			found = true
		}
	}
	if !ok || !found {
		t.Fatalf("unexpected directory view: %#v", directory.View.Data)
	}

	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ReadWorkspaceResource(root, view.Resources[0].URI)
	assertWorkspaceViewErrorCode(t, err, "WORKSPACE_VIEW_STALE")
}

func TestWorkspaceViewRejectsEscapesSecretsAndOversizedDocuments(t *testing.T) {
	root := workspaceViewFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "50-production", "outside.md")); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(root, "50-production", "large.txt")
	if err := os.WriteFile(large, bytes.Repeat([]byte("x"), int(workspaceViewMaxBytes)+1), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		ref  string
		code string
	}{
		{ref: "../secret", code: "WORKSPACE_VIEW_PATH_INVALID"},
		{ref: ".git/config", code: "WORKSPACE_VIEW_PATH_DENIED"},
		{ref: ".contentcloud/workspace.yaml", code: "WORKSPACE_VIEW_PATH_DENIED"},
		{ref: "50-production/outside.md", code: "LOCAL_FILE_OUTSIDE_WORKSPACE"},
		{ref: "50-production/large.txt", code: "WORKSPACE_VIEW_FILE_TOO_LARGE"},
		{ref: "50-production/.env", code: "WORKSPACE_VIEW_PATH_DENIED"},
		{ref: "50-production/tokens/key.txt", code: "WORKSPACE_VIEW_PATH_DENIED"},
		{ref: "40-work/logs/output.txt", code: "WORKSPACE_VIEW_PATH_DENIED"},
		{ref: `C:\private.txt`, code: "WORKSPACE_VIEW_PATH_INVALID"},
	}
	for _, test := range tests {
		t.Run(test.ref, func(t *testing.T) {
			_, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "file", Ref: test.ref})
			assertWorkspaceViewErrorCode(t, err, test.code)
		})
	}
}

func TestWorkspaceViewStreamsLargeMediaWithoutInliningIt(t *testing.T) {
	root := workspaceViewFixture(t)
	ref := "50-production/source.png"
	body := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, bytes.Repeat([]byte{0}, int(workspaceViewMaxBytes))...)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), body, 0o600); err != nil {
		t.Fatal(err)
	}
	view, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "file", Ref: ref})
	if err != nil {
		t.Fatal(err)
	}
	if view.View.MIMEType != "image/png" || view.View.ByteSize != int64(len(body)) || len(view.Resources) != 1 {
		t.Fatalf("unexpected media view: %#v", view)
	}
	stream, err := OpenWorkspaceResource(root, view.Resources[0].URI)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Reader.Close()
	header := make([]byte, 8)
	if _, err := io.ReadFull(stream.Reader, header); err != nil || !bytes.Equal(header, body[:8]) {
		t.Fatalf("streamed media header mismatch: %x err=%v", header, err)
	}
	_, err = ReadWorkspaceResource(root, view.Resources[0].URI)
	assertWorkspaceViewErrorCode(t, err, "MCP_RESOURCE_TOO_LARGE")
}

func TestWorkspaceViewPreservesDirectoryPolicyErrors(t *testing.T) {
	root := workspaceViewFixture(t)
	directory := filepath.Join(root, "50-production", "too-many")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 201; index++ {
		name := filepath.Join(directory, fmt.Sprintf("item-%03d.txt", index))
		if err := os.WriteFile(name, []byte("item"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "file", Ref: "50-production/too-many"})
	assertWorkspaceViewErrorCode(t, err, "WORKSPACE_VIEW_DIRECTORY_TOO_LARGE")
}

func TestWorkspaceViewRejectsStaleRunRevision(t *testing.T) {
	root := workspaceViewFixture(t)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-view", Intent: "intent:content", Now: time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	view, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "run", RunID: run.RunID, ExpectedContextRevision: run.ContextRevision})
	if err != nil || view.ContextRevision != run.ContextRevision || view.RunID != run.RunID {
		t.Fatalf("unexpected run view: %#v err=%v", view, err)
	}
	_, err = BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "run", RunID: run.RunID, ExpectedContextRevision: run.ContextRevision + 1})
	assertWorkspaceViewErrorCode(t, err, "WORKSPACE_VIEW_STALE")
}

func TestWorkspaceSummaryUsesInjectedObservationTime(t *testing.T) {
	root := workspaceViewFixture(t)
	now := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	view, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "workspace_summary", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	context, ok := view.View.Data.(WorkspaceConversationContext)
	if !ok || !context.GeneratedAt.Equal(now) {
		t.Fatalf("workspace summary did not use injected observation time: %#v", view.View.Data)
	}
}

func TestWorkspaceViewTreatsCustomerHTMLAsText(t *testing.T) {
	root := workspaceViewFixture(t)
	ref := "50-production/customer.html"
	body := []byte(`<script>window.top.location='https://evil.example'</script>`)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), body, 0o600); err != nil {
		t.Fatal(err)
	}
	view, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "file", Ref: ref})
	if err != nil {
		t.Fatal(err)
	}
	if view.View.MIMEType != "text/plain" || view.View.Text != string(body) {
		t.Fatalf("customer HTML was exposed as executable content: %#v", view.View)
	}
}

func TestWorkspaceViewParsesJSONAsStructuredData(t *testing.T) {
	root := workspaceViewFixture(t)
	ref := "50-production/content.json"
	body := []byte(`{"title":"本地内容","version":2}`)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), body, 0o600); err != nil {
		t.Fatal(err)
	}
	view, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "file", Ref: ref})
	if err != nil {
		t.Fatal(err)
	}
	data, ok := view.View.Data.(map[string]any)
	if !ok || view.View.MIMEType != "application/json" || view.View.Text != "" || data["title"] != "本地内容" || data["version"] != float64(2) {
		t.Fatalf("JSON file was not exposed as structured data: %#v", view.View)
	}
}

func TestWorkspaceResourceRejectsRemovedPresentationNamespace(t *testing.T) {
	_, err := ReadWorkspaceResource(workspaceViewFixture(t), "contentcloud://workspace/presentations/pres_dead/index.html?digest=dead")
	assertWorkspaceViewErrorCode(t, err, "MCP_RESOURCE_URI_INVALID")
}

func workspaceViewFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, WorkspaceID: "workspace-view", ProjectID: "project-view", ServerURL: "https://content.example.com", CLIVersion: "test", Target: "none", Now: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	return root
}

func assertWorkspaceViewErrorCode(t *testing.T, err error, expected string) {
	t.Helper()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != expected {
		t.Fatalf("unexpected error: got=%v want_code=%s", err, expected)
	}
}
