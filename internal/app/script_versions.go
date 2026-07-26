package app

import (
	"context"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type CreateScriptChangeRunInput struct {
	BriefVersionID  string   `json:"brief_version_id"`
	ChangeType      string   `json:"change_type"`
	InvariantFields []string `json:"invariant_fields"`
	ChangedFields   []string `json:"changed_fields"`
	Hypothesis      string   `json:"hypothesis"`
	RevisionReason  string   `json:"revision_reason"`
	IdempotencyKey  string   `json:"idempotency_key"`
}

func (s *Service) CreateScriptChangeRun(ctx context.Context, actor Actor, baselineVersionID string, input CreateScriptChangeRunInput, requestID string) (domain.TaskRun, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.TaskRun{}, err
	}
	baseline, err := s.store.Script(ctx, actor.TenantID, baselineVersionID)
	if err != nil {
		return domain.TaskRun{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, baseline.ProjectID); err != nil {
		return domain.TaskRun{}, err
	}
	if baseline.ScriptID == "" {
		return domain.TaskRun{}, domain.Conflict("SCRIPT_LINEAGE_MISSING", "基线版本缺少逻辑剧本归属")
	}
	input.ChangeType = strings.TrimSpace(input.ChangeType)
	input.RevisionReason = strings.TrimSpace(input.RevisionReason)
	input.Hypothesis = strings.TrimSpace(input.Hypothesis)
	input.InvariantFields = uniqueNonEmpty(input.InvariantFields)
	input.ChangedFields = uniqueNonEmpty(input.ChangedFields)
	if input.ChangeType != "revision" && input.ChangeType != "variant" {
		return domain.TaskRun{}, domain.Invalid("SCRIPT_CHANGE_TYPE_INVALID", "剧本变更类型只允许 revision 或 variant")
	}
	if input.RevisionReason == "" {
		return domain.TaskRun{}, domain.Invalid("SCRIPT_REVISION_REASON_REQUIRED", "创建剧本新版本必须说明原因")
	}
	for _, pointer := range append(append([]string{}, input.InvariantFields...), input.ChangedFields...) {
		if !domain.ValidJSONPointer(pointer) {
			return domain.TaskRun{}, domain.Invalid("SCRIPT_CHANGE_POINTER_INVALID", "保留项和变化项必须使用 JSON Pointer")
		}
	}
	if input.ChangeType == "variant" {
		if len(input.ChangedFields) != 1 {
			return domain.TaskRun{}, domain.Invalid("VARIANT_SINGLE_VARIABLE_REQUIRED", "单变量变体必须且只能声明一个变化字段")
		}
		if input.Hypothesis == "" {
			return domain.TaskRun{}, domain.Invalid("VARIANT_HYPOTHESIS_REQUIRED", "单变量变体必须填写实验假设")
		}
	}
	briefID := strings.TrimSpace(input.BriefVersionID)
	if briefID == "" {
		baselineRun, err := s.store.Run(ctx, actor.TenantID, baseline.RunID)
		if err != nil {
			return domain.TaskRun{}, err
		}
		briefID = baselineRun.BriefVersionID
		brief, briefErr := s.store.Brief(ctx, actor.TenantID, briefID)
		if briefErr != nil || brief.Status != "approved" {
			briefs, listErr := s.store.Briefs(ctx, actor.TenantID, baseline.ProjectID)
			if listErr != nil {
				return domain.TaskRun{}, listErr
			}
			briefID = ""
			for _, candidate := range briefs {
				if candidate.Status == "approved" {
					briefID = candidate.ID
					break
				}
			}
		}
	}
	if briefID == "" {
		return domain.TaskRun{}, domain.Policy("APPROVED_BRIEF_REQUIRED", "剧本新版本需要一个当前已批准 Brief", "先批准新的 Brief 版本")
	}
	selectedBrief, err := s.store.Brief(ctx, actor.TenantID, briefID)
	if err != nil {
		return domain.TaskRun{}, err
	}
	if selectedBrief.ProjectID != baseline.ProjectID {
		return domain.TaskRun{}, domain.Policy("SCRIPT_BRIEF_PROJECT_MISMATCH", "剧本基线与 Brief 不属于同一项目", "选择该剧本项目内已批准的 Brief")
	}
	request := domain.ScriptChangeRequest{ChangeType: input.ChangeType, InvariantFields: input.InvariantFields, ChangedFields: input.ChangedFields, Hypothesis: input.Hypothesis, RevisionReason: input.RevisionReason}
	config := &scriptRunConfig{ScriptID: baseline.ScriptID, BaselineVersionID: baseline.ID, ChangeRequest: request}
	run, err := s.createScriptRun(ctx, actor, briefID, input.IdempotencyKey, requestID, config)
	if err == nil {
		s.audit(ctx, actor, baseline.ProjectID, "script.change_run_created", "task_run", run.ID, requestID, map[string]any{"script_id": baseline.ScriptID, "baseline_version_id": baseline.ID, "change_type": input.ChangeType})
	}
	return run, err
}
