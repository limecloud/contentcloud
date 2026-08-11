package domain

import "time"

type ModelGenerationReceipt struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	TaskID         string    `json:"task_id"`
	TaskRevisionID string    `json:"task_revision_id"`
	ProviderID     string    `json:"provider_id"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	RequestID      string    `json:"request_id,omitempty"`
	RequestDigest  string    `json:"request_digest"`
	ResponseDigest string    `json:"response_digest"`
	InputTokens    int64     `json:"input_tokens"`
	OutputTokens   int64     `json:"output_tokens"`
	TotalTokens    int64     `json:"total_tokens"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func (v ModelGenerationReceipt) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.TaskRevisionID == "" || v.ProviderID == "" || v.Provider == "" || v.Model == "" {
		return Invalid("MODEL_GENERATION_RECEIPT_INVALID", "模型生成回执缺少任务、候选修订或 Provider")
	}
	if !validSHA256Digest(v.RequestDigest) || !validSHA256Digest(v.ResponseDigest) || v.InputTokens < 0 || v.OutputTokens < 0 || v.TotalTokens < 0 {
		return Invalid("MODEL_GENERATION_RECEIPT_USAGE_INVALID", "模型生成回执摘要或用量无效")
	}
	return nil
}
