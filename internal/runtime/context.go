package runtime

import (
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

// ContextViewInput accepts only immutable references and execution policy. A
// caller cannot smuggle source正文 or credentials into the persisted view.
type ContextViewInput struct {
	TenantID     string
	JobRunID     string
	NodeRunID    string
	AttemptID    string
	InputRefs    []string
	StateRefs    []string
	EventRefs    []string
	AllowedTools []string
	MaxTokens    int
	BudgetMinor  int64
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

func BuildContextView(input ContextViewInput) (domain.ContextView, error) {
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.JobRunID) == "" || strings.TrimSpace(input.NodeRunID) == "" || strings.TrimSpace(input.AttemptID) == "" {
		return domain.ContextView{}, domain.Invalid("CONTEXT_VIEW_INPUT_INVALID", "ContextView 缺少执行引用")
	}
	if input.MaxTokens <= 0 || input.BudgetMinor < 0 || input.CreatedAt.IsZero() || !input.ExpiresAt.After(input.CreatedAt) {
		return domain.ContextView{}, domain.Invalid("CONTEXT_VIEW_POLICY_INVALID", "ContextView 预算或有效期无效")
	}
	view := domain.ContextView{ID: domain.NewID(), TenantID: strings.TrimSpace(input.TenantID), JobRunID: strings.TrimSpace(input.JobRunID), NodeRunID: strings.TrimSpace(input.NodeRunID), AttemptID: strings.TrimSpace(input.AttemptID), SchemaVersion: domain.ContextViewSchema, InputRefs: sortedRefs(input.InputRefs), StateRefs: sortedRefs(input.StateRefs), EventRefs: sortedRefs(input.EventRefs), AllowedTools: sortedRefs(input.AllowedTools), MaxTokens: input.MaxTokens, BudgetMinor: input.BudgetMinor, CreatedAt: input.CreatedAt.UTC(), ExpiresAt: input.ExpiresAt.UTC()}
	digest, err := domain.CanonicalHash(struct {
		SchemaVersion string
		TenantID      string
		JobRunID      string
		NodeRunID     string
		AttemptID     string
		InputRefs     []string
		StateRefs     []string
		EventRefs     []string
		AllowedTools  []string
		MaxTokens     int
		BudgetMinor   int64
		ExpiresAt     time.Time
	}{view.SchemaVersion, view.TenantID, view.JobRunID, view.NodeRunID, view.AttemptID, view.InputRefs, view.StateRefs, view.EventRefs, view.AllowedTools, view.MaxTokens, view.BudgetMinor, view.ExpiresAt})
	if err != nil {
		return domain.ContextView{}, err
	}
	view.Digest = "sha256:" + digest
	if err := view.Validate(); err != nil {
		return domain.ContextView{}, err
	}
	return view, nil
}

func sortedRefs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
