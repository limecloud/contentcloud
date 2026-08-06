package domain

import (
	"strings"
	"time"
)

const (
	InputItemUntriaged       = "untriaged"
	InputItemNeedsInfo       = "needs_info"
	InputItemRouted          = "routed"
	InputItemTaskCreated     = "task_created"
	InputItemTaskMerged      = "task_merged"
	InputItemProjectMaterial = "project_material"
	InputItemArchived        = "archived"
)

type InputItem struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	ProjectID      string         `json:"project_id,omitempty"`
	SourceType     string         `json:"source_type"`
	Title          string         `json:"title"`
	Summary        string         `json:"summary"`
	Body           string         `json:"body,omitempty"`
	SourceRef      string         `json:"source_ref,omitempty"`
	SourceDigest   string         `json:"source_digest,omitempty"`
	Disclosure     string         `json:"disclosure"`
	Status         string         `json:"status"`
	TargetTaskID   string         `json:"target_task_id,omitempty"`
	AssigneeUserID string         `json:"assignee_user_id,omitempty"`
	MissingFields  []string       `json:"missing_fields"`
	Metadata       map[string]any `json:"metadata"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	RowVersion     int            `json:"row_version"`
	CreatedBy      string         `json:"created_by"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func (v *InputItem) NormalizeCollections() {
	if v.MissingFields == nil {
		v.MissingFields = []string{}
	}
	if v.Metadata == nil {
		v.Metadata = map[string]any{}
	}
}

func (v InputItem) Validate() error {
	if v.ID == "" || v.TenantID == "" || strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.SourceType) == "" {
		return Invalid("INPUT_ITEM_INVALID", "输入收集记录缺少标题或来源类型")
	}
	if len(v.Title) > 300 || len(v.Summary) > 10000 || len(v.Body) > 500000 {
		return Invalid("INPUT_ITEM_TOO_LARGE", "输入收集内容超过大小限制")
	}
	switch v.SourceType {
	case "brief", "manual_inspiration", "workspace_file", "comment", "external_request", "trigger", "conversation_bundle", "other":
	default:
		return Invalid("INPUT_ITEM_SOURCE_INVALID", "输入收集来源类型无效")
	}
	switch v.Disclosure {
	case "project", "tenant", "restricted":
	default:
		return Invalid("INPUT_ITEM_DISCLOSURE_INVALID", "输入收集披露范围无效")
	}
	switch v.Status {
	case InputItemUntriaged, InputItemNeedsInfo, InputItemRouted, InputItemTaskCreated, InputItemTaskMerged, InputItemProjectMaterial, InputItemArchived:
	default:
		return Invalid("INPUT_ITEM_STATUS_INVALID", "输入收集状态无效")
	}
	if v.RowVersion < 1 {
		return Invalid("INPUT_ITEM_VERSION_INVALID", "输入收集记录版本无效")
	}
	if len(v.IdempotencyKey) > 128 {
		return Invalid("IDEMPOTENCY_KEY_INVALID", "幂等键长度不能超过 128 个字符")
	}
	return nil
}
