package application

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	"github.com/limecloud/contentcloud/internal/work"
)

type CreateConversationImportInput struct {
	ClientID       string `json:"client_id"`
	NodeID         string `json:"node_id"`
	Purpose        string `json:"purpose"`
	RequestedScope string `json:"requested_scope"`
	AttachAs       string `json:"attach_as"`
	RetentionDays  int    `json:"retention_days"`
	StageRunID     string `json:"stage_run_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

var conversationAdapterVersions = map[string]string{
	"codex":             "0.1.0",
	"claude-code":       "0.1.0",
	"workspace-adapter": "0.1.0",
}

func (s *WorkService) CreateConversationImport(ctx context.Context, actor Actor, taskID string, input CreateConversationImportInput, requestID string) (work.ConversationImport, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer"); err != nil {
		return work.ConversationImport{}, err
	}
	if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
		if existing, err := s.tasks.ConversationImportByIdempotencyKey(ctx, actor.TenantID, key); err == nil {
			if existing.TaskID != taskID || existing.ClientID != strings.ToLower(strings.TrimSpace(input.ClientID)) || existing.Purpose != input.Purpose || existing.RequestedScope != input.RequestedScope || existing.AttachAs != input.AttachAs {
				return work.ConversationImport{}, fault.Conflict("IDEMPOTENCY_REPLAY_MISMATCH", "幂等键已用于不同的对话导入请求")
			}
			return existing, nil
		}
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return work.ConversationImport{}, err
	}
	clientID := strings.ToLower(strings.TrimSpace(input.ClientID))
	adapterVersion, ok := conversationAdapterVersions[clientID]
	if !ok {
		return work.ConversationImport{}, fault.Invalid("CLIENT_ADAPTER_UNSUPPORTED", "当前客户端没有可用的对话导出适配器")
	}
	if input.StageRunID != "" {
		stageRuns, stageErr := s.tasks.StageRuns(ctx, actor.TenantID, taskID)
		if stageErr != nil {
			return work.ConversationImport{}, stageErr
		}
		found := false
		for _, stageRun := range stageRuns {
			if stageRun.ID == input.StageRunID {
				found = true
				break
			}
		}
		if !found {
			return work.ConversationImport{}, fault.NotFound("流程阶段执行记录")
		}
	}
	retentionDays := input.RetentionDays
	if retentionDays == 0 {
		retentionDays = 30
	}
	now := s.now().UTC()
	value := work.ConversationImport{
		ID:             idgen.New(),
		TenantID:       actor.TenantID,
		ProjectID:      task.ProjectID,
		TaskID:         task.ID,
		StageRunID:     strings.TrimSpace(input.StageRunID),
		NodeID:         strings.TrimSpace(input.NodeID),
		ClientID:       clientID,
		AdapterVersion: adapterVersion,
		AdapterID:      clientID + "@" + adapterVersion,
		Purpose:        strings.TrimSpace(input.Purpose),
		RequestedScope: strings.TrimSpace(input.RequestedScope),
		AttachAs:       strings.TrimSpace(input.AttachAs),
		RetentionDays:  retentionDays,
		Status:         work.ConversationImportAwaitingConfirmation,
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		ExpiresAt:      now.Add(time.Duration(retentionDays) * 24 * time.Hour),
		CreatedBy:      actor.UserID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := value.Validate(); err != nil {
		return work.ConversationImport{}, err
	}
	if err := s.tasks.CreateConversationImport(ctx, value); err != nil {
		if value.IdempotencyKey != "" {
			if existing, lookupErr := s.tasks.ConversationImportByIdempotencyKey(ctx, actor.TenantID, value.IdempotencyKey); lookupErr == nil {
				return existing, nil
			}
		}
		return work.ConversationImport{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "conversation_import.created", "conversation_import", value.ID, requestID, map[string]any{"task_id": task.ID, "client_id": value.ClientID, "adapter_id": value.AdapterID, "requested_scope": value.RequestedScope, "attach_as": value.AttachAs, "expires_at": value.ExpiresAt})
	return value, nil
}

func (s *WorkService) ConversationImport(ctx context.Context, actor Actor, importID string) (work.ConversationImport, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "viewer"); err != nil && actor.Type != "device" && actor.Type != "worker" {
		return work.ConversationImport{}, err
	}
	value, err := s.tasks.ConversationImport(ctx, actor.TenantID, importID)
	if err != nil {
		return work.ConversationImport{}, err
	}
	return s.expireConversationImport(ctx, value)
}

func (s *WorkService) TaskConversationImports(ctx context.Context, actor Actor, taskID string) ([]work.ConversationImport, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "viewer"); err != nil {
		return nil, err
	}
	if _, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID); err != nil {
		return nil, err
	}
	values, err := s.tasks.ConversationImportsForTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return nil, err
	}
	result := make([]work.ConversationImport, 0, len(values))
	for _, value := range values {
		expired, expireErr := s.expireConversationImport(ctx, value)
		if expireErr != nil {
			return nil, expireErr
		}
		result = append(result, expired)
	}
	return result, nil
}

func (s *WorkService) SubmitConversationBundle(ctx context.Context, actor Actor, importID string, bundle work.ConversationBundle, requestID string) (work.ConversationImport, error) {
	if actor.Type != "device" && actor.Type != "worker" {
		if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer"); err != nil {
			return work.ConversationImport{}, err
		}
	}
	value, err := s.tasks.ConversationImport(ctx, actor.TenantID, importID)
	if err != nil {
		return work.ConversationImport{}, err
	}
	value, err = s.expireConversationImport(ctx, value)
	if err != nil {
		return work.ConversationImport{}, err
	}
	if actor.DeviceID != "" && value.NodeID != "" && actor.DeviceID != value.NodeID {
		return work.ConversationImport{}, fault.Policy("CONVERSATION_IMPORT_NODE_MISMATCH", "客户端节点与导入请求绑定的节点不一致", "在绑定节点上重新导出")
	}
	if err := bundle.ValidateAgainst(value, s.now().UTC()); err != nil {
		return work.ConversationImport{}, err
	}
	now := s.now().UTC()
	value.Bundle = &bundle
	value.Status = work.ConversationImportUploaded
	value.UploadedAt = &now
	value.UpdatedAt = now
	if err := s.tasks.SaveConversationImport(ctx, value); err != nil {
		return work.ConversationImport{}, err
	}
	s.audit(ctx, actor, value.ProjectID, "conversation_import.uploaded", "conversation_import", value.ID, requestID, map[string]any{"task_id": value.TaskID, "bundle_id": bundle.BundleID, "content_digest": bundle.ContentDigest, "attach_as": value.AttachAs})
	return value, nil
}

func (s *WorkService) CancelConversationImport(ctx context.Context, actor Actor, importID, requestID string) (work.ConversationImport, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer"); err != nil {
		return work.ConversationImport{}, err
	}
	value, err := s.tasks.ConversationImport(ctx, actor.TenantID, importID)
	if err != nil {
		return work.ConversationImport{}, err
	}
	value, err = s.expireConversationImport(ctx, value)
	if err != nil {
		return work.ConversationImport{}, err
	}
	if value.Status != work.ConversationImportAwaitingConfirmation {
		return work.ConversationImport{}, fault.Conflict("CONVERSATION_IMPORT_NOT_CANCELLABLE", "只有等待客户端确认的导入请求可以取消")
	}
	now := s.now().UTC()
	value.Status = work.ConversationImportCancelled
	value.CancelledAt = &now
	value.UpdatedAt = now
	if err := s.tasks.SaveConversationImport(ctx, value); err != nil {
		return work.ConversationImport{}, err
	}
	s.audit(ctx, actor, value.ProjectID, "conversation_import.cancelled", "conversation_import", value.ID, requestID, map[string]any{"task_id": value.TaskID})
	return value, nil
}

func (s *WorkService) expireConversationImport(ctx context.Context, value work.ConversationImport) (work.ConversationImport, error) {
	if value.Status == work.ConversationImportAwaitingConfirmation && !s.now().UTC().Before(value.ExpiresAt) {
		value.Status = work.ConversationImportExpired
		value.UpdatedAt = s.now().UTC()
		if err := s.tasks.SaveConversationImport(ctx, value); err != nil {
			return work.ConversationImport{}, err
		}
	}
	return value, nil
}
