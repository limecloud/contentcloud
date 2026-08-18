package localworkspace

import (
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReviewFeedbackInboxKeepsImmutableRevisionsOfOneSubmissionRevision(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	feedback := reviewFeedbackFixture(now)
	first, err := StoreReviewFeedback(root, feedback, now)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := StoreReviewFeedback(root, feedback, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != replayed.ID || first.Path != replayed.Path {
		t.Fatalf("identical feedback was not idempotent: first=%+v replay=%+v", first, replayed)
	}

	feedback.Comments = append(feedback.Comments, reviewdomain.ReviewComment{ID: "comment-2", Body: "请收紧 CTA", CreatedAt: now.Add(2 * time.Minute)})
	feedback.CreatedAt = now.Add(2 * time.Minute)
	second, err := StoreReviewFeedback(root, feedback, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.Path == first.Path {
		t.Fatalf("new feedback revision overwrote the prior bundle: first=%+v second=%+v", first, second)
	}

	items, err := ReviewFeedbackInbox(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("unexpected feedback inbox: %+v", items)
	}
	for _, item := range items {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(item.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o222 != 0 {
			t.Fatalf("pulled feedback is writable: %s mode=%o", item.Path, info.Mode().Perm())
		}
	}
}

func TestReviewFeedbackInboxRejectsDigestMismatch(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	feedback := reviewFeedbackFixture(now)
	if _, err := StorePulledBundle(root, "feedback", "feedback-wrong", feedback, now); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewFeedbackInbox(root); err == nil {
		t.Fatal("feedback inbox accepted a filename that did not match its content digest")
	}
}

func reviewFeedbackFixture(now time.Time) reviewdomain.ReviewFeedbackBundle {
	return reviewdomain.ReviewFeedbackBundle{
		BundleVersion: "1.0", SubmissionID: "submission-1", SubmissionRevisionID: "revision-1", SubjectHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: now,
		Comments: []reviewdomain.ReviewComment{{ID: "comment-1", Body: "请补充证据", CreatedAt: now}},
	}
}
