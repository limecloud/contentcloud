package localworkspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type ReviewFeedbackInboxItem struct {
	ID       string                      `json:"id"`
	Path     string                      `json:"path"`
	SHA256   string                      `json:"sha256"`
	Feedback domain.ReviewFeedbackBundle `json:"feedback"`
}

func StoreReviewFeedback(root string, feedback domain.ReviewFeedbackBundle, now time.Time) (ReviewFeedbackInboxItem, error) {
	id, hash, err := reviewFeedbackIdentity(feedback)
	if err != nil {
		return ReviewFeedbackInboxItem{}, err
	}
	path, err := StorePulledBundle(root, "feedback", id, feedback, now)
	if err != nil {
		return ReviewFeedbackInboxItem{}, err
	}
	resolved, err := FindRoot(root)
	if err != nil {
		return ReviewFeedbackInboxItem{}, err
	}
	return ReviewFeedbackInboxItem{ID: id, Path: relativeWorkspacePath(resolved, path), SHA256: "sha256:" + hash, Feedback: feedback}, nil
}

func ReviewFeedbackInbox(root string) ([]ReviewFeedbackInboxItem, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(resolved, ".contentcloud", "inbox", "review-feedback", "*.json"))
	if err != nil {
		return nil, err
	}
	items := make([]ReviewFeedbackInboxItem, 0, len(paths))
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var feedback domain.ReviewFeedbackBundle
		if err := json.Unmarshal(body, &feedback); err != nil {
			return nil, domain.Invalid("REVIEW_FEEDBACK_INVALID", "本地审核反馈不是有效的 JSON 数据包")
		}
		id, hash, err := reviewFeedbackIdentity(feedback)
		if err != nil {
			return nil, err
		}
		if strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) != id {
			return nil, domain.Conflict("REVIEW_FEEDBACK_DIGEST_MISMATCH", "本地审核反馈文件名与内容 digest 不一致")
		}
		items = append(items, ReviewFeedbackInboxItem{ID: id, Path: relativeWorkspacePath(resolved, path), SHA256: "sha256:" + hash, Feedback: feedback})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Feedback.CreatedAt.Equal(items[j].Feedback.CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].Feedback.CreatedAt.After(items[j].Feedback.CreatedAt)
	})
	return items, nil
}

func reviewFeedbackIdentity(feedback domain.ReviewFeedbackBundle) (string, string, error) {
	if feedback.BundleVersion != "1.0" || strings.TrimSpace(feedback.SubmissionID) == "" || strings.TrimSpace(feedback.SubmissionRevisionID) == "" || strings.TrimSpace(feedback.SubjectHash) == "" || feedback.CreatedAt.IsZero() {
		return "", "", domain.Invalid("REVIEW_FEEDBACK_INVALID", "审核反馈数据包缺少固定版本、提交记录、审核对象摘要或创建时间")
	}
	for _, comment := range feedback.Comments {
		if strings.TrimSpace(comment.ID) == "" || strings.TrimSpace(comment.Body) == "" || comment.CreatedAt.IsZero() {
			return "", "", domain.Invalid("REVIEW_FEEDBACK_INVALID", "审核反馈包含无 ID、正文或创建时间的 comment")
		}
	}
	hash, err := domain.CanonicalHash(feedback)
	if err != nil {
		return "", "", err
	}
	return "feedback-" + hash[:24], hash, nil
}
