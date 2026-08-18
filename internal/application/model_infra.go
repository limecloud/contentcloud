package application

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	modelprovider "github.com/limecloud/contentcloud/internal/integration/provider/model"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"github.com/limecloud/contentcloud/internal/work"
)

type GenerateModelCandidateInput struct {
	ProviderID     string                  `json:"provider_id"`
	Messages       []modelprovider.Message `json:"messages"`
	ResponseSchema map[string]any          `json:"response_schema"`
	ContentType    string                  `json:"content_type"`
	SchemaVersion  string                  `json:"schema_version"`
	Temperature    *float64                `json:"temperature,omitempty"`
	MaxTokens      int                     `json:"max_tokens,omitempty"`
	RequestID      string                  `json:"request_id,omitempty"`
}

type GenerateModelCandidateResult struct {
	Revision reviewdomain.TaskRevision             `json:"revision"`
	Receipt  deliverydomain.ModelGenerationReceipt `json:"receipt"`
}

func (s *DeliveryService) ModelProviderIDs() []string { return s.modelProviders.IDs() }

func (s *DeliveryService) GenerateModelCandidate(ctx context.Context, actor Actor, taskID string, input GenerateModelCandidateInput, requestID string) (GenerateModelCandidateResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return GenerateModelCandidateResult{}, err
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return GenerateModelCandidateResult{}, err
	}
	if task.Status == work.TaskStatusCancelled || task.Status == work.TaskStatusDelivered {
		return GenerateModelCandidateResult{}, fault.Policy("MODEL_CANDIDATE_TASK_CLOSED", "已取消或已交付任务不能继续生成候选", "创建新任务或新的修订工作项")
	}
	providerID := strings.ToLower(strings.TrimSpace(input.ProviderID))
	provider, err := s.modelProviders.Resolve(providerID)
	if err != nil {
		return GenerateModelCandidateResult{}, err
	}
	if len(input.Messages) == 0 || len(input.ResponseSchema) == 0 || strings.TrimSpace(input.SchemaVersion) == "" {
		return GenerateModelCandidateResult{}, fault.Invalid("MODEL_CANDIDATE_CONTRACT_REQUIRED", "模型候选必须提供消息、响应 JSON Schema 和业务 Schema 版本")
	}
	request := modelprovider.CompletionRequest{Messages: input.Messages, Temperature: input.Temperature, MaxTokens: input.MaxTokens, ResponseSchema: input.ResponseSchema, RequestID: input.RequestID}
	requestDigest, err := stablehash.Sum(request)
	if err != nil {
		return GenerateModelCandidateResult{}, err
	}
	completion, err := provider.Complete(ctx, request)
	if err != nil {
		return GenerateModelCandidateResult{}, fault.Policy("MODEL_GENERATION_FAILED", "模型 Provider 未返回可验证候选", "检查 Provider、模型、Schema、配额和超时后重试")
	}
	content := completion.Structured
	if len(content) == 0 {
		content = json.RawMessage(completion.Content)
	}
	var object map[string]any
	if json.Unmarshal(content, &object) != nil || object == nil {
		return GenerateModelCandidateResult{}, fault.Invalid("MODEL_CANDIDATE_OBJECT_REQUIRED", "模型候选必须是 JSON 对象")
	}
	canonicalContent, err := json.Marshal(object)
	if err != nil {
		return GenerateModelCandidateResult{}, err
	}
	contentDigest, err := stablehash.Sum(object)
	if err != nil {
		return GenerateModelCandidateResult{}, err
	}
	responseDigest, err := stablehash.Sum(completion)
	if err != nil {
		return GenerateModelCandidateResult{}, err
	}
	revisions, err := s.delivery.TaskRevisions(ctx, actor.TenantID, task.ID)
	if err != nil {
		return GenerateModelCandidateResult{}, err
	}
	now := s.now().UTC()
	revision := reviewdomain.TaskRevision{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, RevisionNo: len(revisions) + 1, ContentType: defaultString(strings.TrimSpace(input.ContentType), task.ContentType), SchemaVersion: strings.TrimSpace(input.SchemaVersion), Content: canonicalContent, ContentHash: "sha256:" + contentDigest, SOPDigest: task.SOPDigest, KnowledgeSnapshotIDs: []string{}, EvidenceSummary: map[string]any{}, RightsSummary: map[string]any{}, Status: reviewdomain.TaskRevisionDraft, SubmittedBy: actor.UserID, CreatedAt: now}
	receipt := deliverydomain.ModelGenerationReceipt{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, TaskRevisionID: revision.ID, ProviderID: providerID, Provider: completion.Provider, Model: completion.Model, RequestID: completion.RequestID, RequestDigest: "sha256:" + requestDigest, ResponseDigest: "sha256:" + responseDigest, InputTokens: completion.InputTokens, OutputTokens: completion.OutputTokens, TotalTokens: completion.TotalTokens, CreatedBy: actor.UserID, CreatedAt: now}
	if err := s.delivery.CreateModelGeneratedRevision(ctx, revision, receipt); err != nil {
		return GenerateModelCandidateResult{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "model.candidate_generated", "task_revision", revision.ID, requestID, map[string]any{"provider_id": providerID, "model": receipt.Model, "request_digest": receipt.RequestDigest, "response_digest": receipt.ResponseDigest, "total_tokens": receipt.TotalTokens})
	return GenerateModelCandidateResult{Revision: revision, Receipt: receipt}, nil
}

func (s *DeliveryService) ModelGenerationReceipts(ctx context.Context, actor Actor, taskID string) ([]deliverydomain.ModelGenerationReceipt, error) {
	if _, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID); err != nil {
		return nil, err
	}
	return s.delivery.ModelGenerationReceipts(ctx, actor.TenantID, taskID)
}
