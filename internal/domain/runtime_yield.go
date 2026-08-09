package domain

import "time"

const (
	YieldWaitChildren = "wait_children"
	YieldWaitHuman    = "wait_human"
	YieldWaitEffect   = "wait_effect"

	RuntimeYieldOpen     = "open"
	RuntimeYieldResolved = "resolved"
)

// RuntimeYield is the durable boundary between two RuntimeAttempts of the
// same logical AgentInstance. It records only wait identities, never process
// or memory state.
type RuntimeYield struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	JobRunID        string     `json:"job_run_id"`
	NodeRunID       string     `json:"node_run_id"`
	AttemptID       string     `json:"attempt_id"`
	AgentInstanceID string     `json:"agent_instance_id"`
	Reason          string     `json:"reason"`
	WaitRefs        []string   `json:"wait_refs"`
	State           string     `json:"state"`
	ResumeKey       string     `json:"resume_key,omitempty"`
	Version         int        `json:"version"`
	YieldedAt       time.Time  `json:"yielded_at"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (value RuntimeYield) Validate() error {
	if value.ID == "" || value.TenantID == "" || value.JobRunID == "" || value.NodeRunID == "" || value.AttemptID == "" || value.AgentInstanceID == "" || value.Version < 1 || value.YieldedAt.IsZero() || value.CreatedAt.IsZero() || value.UpdatedAt.IsZero() {
		return Invalid("RUNTIME_YIELD_INVALID", "RuntimeYield 缺少执行范围、等待原因或版本")
	}
	switch value.Reason {
	case YieldWaitChildren, YieldWaitHuman, YieldWaitEffect:
	default:
		return Invalid("RUNTIME_YIELD_REASON_INVALID", "RuntimeYield 等待原因无效")
	}
	if len(value.WaitRefs) == 0 {
		return Invalid("RUNTIME_YIELD_REFS_REQUIRED", "RuntimeYield 必须冻结至少一个等待引用")
	}
	switch value.State {
	case RuntimeYieldOpen:
		if value.ResolvedAt != nil || value.ResumeKey != "" {
			return Invalid("RUNTIME_YIELD_OPEN_INVALID", "开放的 RuntimeYield 不能包含恢复结果")
		}
	case RuntimeYieldResolved:
		if value.ResolvedAt == nil || value.ResumeKey == "" {
			return Invalid("RUNTIME_YIELD_RESOLVED_INVALID", "已恢复的 RuntimeYield 缺少恢复身份或时间")
		}
	default:
		return Invalid("RUNTIME_YIELD_STATE_INVALID", "RuntimeYield 状态无效")
	}
	return nil
}
