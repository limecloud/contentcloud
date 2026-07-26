package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type CreateKnowledgeExtractionRunInput struct {
	ProjectID         string   `json:"project_id"`
	SourceRevisionIDs []string `json:"source_revision_ids"`
	IdempotencyKey    string   `json:"idempotency_key"`
	OutputCount       int      `json:"output_count"`
}

func (s *Service) CreateKnowledgeExtractionRun(ctx context.Context, actor Actor, in CreateKnowledgeExtractionRunInput, requestID string) (domain.TaskRun, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer"); err != nil {
		return domain.TaskRun{}, err
	}
	project, err := s.projectForWrite(ctx, actor, in.ProjectID)
	if err != nil {
		return domain.TaskRun{}, err
	}
	revisionIDs := uniqueNonEmpty(in.SourceRevisionIDs)
	if len(revisionIDs) == 0 {
		return domain.TaskRun{}, domain.Invalid("SOURCE_REVISION_REQUIRED", "知识提取至少需要一个来源版本")
	}
	if in.OutputCount == 0 {
		in.OutputCount = 20
	}
	if in.OutputCount < 1 || in.OutputCount > 20 {
		return domain.TaskRun{}, domain.Invalid("OUTPUT_COUNT_INVALID", "知识候选数量必须在 1 到 20 之间")
	}
	sources := make([]domain.ContractSource, 0, len(revisionIDs))
	for _, revisionID := range revisionIDs {
		revision, err := s.store.SourceRevision(ctx, actor.TenantID, revisionID)
		if err != nil || revision.ProjectID != project.ID {
			return domain.TaskRun{}, domain.Policy("SOURCE_REVISION_PROJECT_MISMATCH", "来源版本不属于当前项目", "只选择当前项目内的来源版本")
		}
		if revision.ProcessingStatus != "ready" {
			return domain.TaskRun{}, domain.Policy("SOURCE_REVISION_NOT_READY", "来源版本尚未完成可信解析", "等待来源状态变为 ready")
		}
		source, err := s.store.Source(ctx, actor.TenantID, revision.SourceID)
		if err != nil {
			return domain.TaskRun{}, err
		}
		spans, err := s.store.Evidence(ctx, actor.TenantID, revision.ID)
		if err != nil {
			return domain.TaskRun{}, err
		}
		evidence := []domain.ContractEvidence{}
		for _, span := range spans {
			if span.ReviewStatus != "accepted" {
				continue
			}
			evidence = append(evidence, domain.ContractEvidence{ID: span.ID, LocatorKind: span.LocatorKind, Locator: span.Locator, Quote: span.QuoteText, QuoteHash: span.QuoteHash})
		}
		if len(evidence) == 0 {
			return domain.TaskRun{}, domain.Policy("ACCEPTED_EVIDENCE_REQUIRED", "来源版本没有可用于提取的已验收证据", "先复核并接受至少一个证据片段")
		}
		sources = append(sources, domain.ContractSource{SourceID: source.ID, RevisionID: revision.ID, Name: source.Name, SourceType: source.SourceType, FileName: revision.FileName, SHA256: revision.SHA256, DetectedMIME: revision.DetectedMIME, Evidence: evidence})
	}
	snapshot, err := domain.CompileKnowledgeSnapshot(project, sources, s.now())
	if err != nil {
		return domain.TaskRun{}, err
	}
	if err := s.store.CreateSnapshot(ctx, snapshot); err != nil {
		return domain.TaskRun{}, err
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		in.IdempotencyKey = domain.NewID()
	}
	now := s.now().UTC()
	run := domain.TaskRun{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, InputSnapshotID: snapshot.ID, IdempotencyKey: in.IdempotencyKey, TaskType: "knowledge_extract", CapabilityID: domain.KnowledgeExtractCapability, CapabilityVersion: "1.0.0", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, OutputCount: in.OutputCount, DeliveryProfiles: []string{"cloud_native"}, State: "queued", Priority: 60, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return run, err
	}
	s.audit(ctx, actor, run.ProjectID, "knowledge_extraction_run.created", "task_run", run.ID, requestID, map[string]any{"snapshot_id": snapshot.ID, "source_revision_count": len(sources), "manifest_hash": snapshot.ManifestHash})
	return run, nil
}

func (s *Service) ReportTask(ctx context.Context, actor Actor, device domain.Device, runID, attemptID, runToken string, body json.RawMessage, requestID string) (any, error) {
	run, err := s.store.Run(ctx, actor.TenantID, runID)
	if err != nil {
		return nil, err
	}
	switch run.TaskType {
	case "script_generate", "script_revise":
		var pkg domain.ScriptPackage
		if err := decodeStrict(body, &pkg); err != nil {
			s.failMalformedOutput(ctx, actor, device, run, attemptID, runToken, "script_json")
			return nil, domain.Invalid("CAPABILITY_OUTPUT_INVALID", "本地 Agent 返回的 Script Package JSON 无效: "+err.Error())
		}
		return s.ReportRunAttempt(ctx, actor, device, runID, attemptID, runToken, pkg, requestID)
	case "knowledge_extract":
		var pkg domain.KnowledgeExtractionPackage
		if err := decodeStrict(body, &pkg); err != nil {
			s.failMalformedOutput(ctx, actor, device, run, attemptID, runToken, "knowledge_json")
			return nil, domain.Invalid("CAPABILITY_OUTPUT_INVALID", "本地 Agent 返回的知识候选 JSON 无效: "+err.Error())
		}
		return s.reportKnowledgeExtraction(ctx, actor, device, run, attemptID, runToken, pkg, requestID)
	default:
		return nil, domain.Invalid("TASK_TYPE_UNSUPPORTED", "当前客户端报告不支持该任务类型")
	}
}

func (s *Service) reportKnowledgeExtraction(ctx context.Context, actor Actor, device domain.Device, run domain.TaskRun, attemptID, runToken string, pkg domain.KnowledgeExtractionPackage, requestID string) (domain.KnowledgeExtractionResult, error) {
	hash, _ := domain.CanonicalHash(pkg)
	if run.State == "succeeded" {
		if run.ReportHash != hash {
			return domain.KnowledgeExtractionResult{}, domain.Conflict("REPORT_CONFLICT", "同一任务已报告不同内容")
		}
		return s.knowledgeExtractionResult(ctx, actor.TenantID, run, pkg.Warnings)
	}
	if run.TaskType != "knowledge_extract" {
		return domain.KnowledgeExtractionResult{}, domain.Conflict("RUN_LEASE_INVALID", "任务租约不属于当前设备或任务类型不匹配")
	}
	attempt, err := s.activeRunAttempt(ctx, actor, device, run, attemptID, runToken, s.now().UTC())
	if err != nil {
		return domain.KnowledgeExtractionResult{}, err
	}
	if run.CancelRequestedAt != nil {
		_, _ = s.FinishRunAttempt(ctx, actor, device, run.ID, attempt.ID, runToken, FinishRunAttemptInput{Outcome: "canceled", FailureClass: "user_canceled"}, requestID)
		return domain.KnowledgeExtractionResult{}, domain.Conflict("RUN_CANCELED", "任务已取消，结果不会入库")
	}
	if pkg.SchemaVersion != "1.0" || len(pkg.Candidates) == 0 || len(pkg.Candidates) > run.OutputCount {
		return s.rejectKnowledgeOutput(ctx, run, attempt, "output_validation", "知识候选 Schema 版本或数量不符合任务契约", domain.Invalid("KNOWLEDGE_PACKAGE_INVALID", "知识候选 Schema 版本或候选数量不符合任务契约"))
	}
	project, err := s.projectForWrite(ctx, actor, run.ProjectID)
	if err != nil || project.Status == "archived" {
		return domain.KnowledgeExtractionResult{}, domain.Policy("PROJECT_ARCHIVED", "已归档项目不能接收新的 Agent 结果", "恢复项目或取消任务")
	}
	snapshot, err := s.store.Snapshot(ctx, actor.TenantID, run.InputSnapshotID)
	if err != nil {
		return domain.KnowledgeExtractionResult{}, err
	}
	inputs := make([]CreateKnowledgeInput, 0, len(pkg.Candidates))
	for _, candidate := range pkg.Candidates {
		if !knowledgeCandidateEnumsValid(candidate) || !evidenceWithinKnowledgeContract(candidate.Evidence, snapshot.Sources) {
			return s.rejectKnowledgeOutput(ctx, run, attempt, "output_grounding", "知识候选类型、风险或证据超出冻结契约", domain.E("content", "policy", "KNOWLEDGE_CANDIDATE_INVALID", "知识候选类型、风险或证据超出 Task Contract", 7))
		}
		input := knowledgeCandidateInput(run, candidate)
		if err := s.validateKnowledgeInput(ctx, actor.TenantID, input); err != nil {
			return s.rejectKnowledgeOutput(ctx, run, attempt, "output_validation", "知识候选未通过确定性业务校验", err)
		}
		inputs = append(inputs, input)
	}
	for _, input := range inputs {
		if _, err := s.createKnowledge(ctx, actor, input, requestID, true); err != nil {
			return domain.KnowledgeExtractionResult{}, err
		}
	}
	run.State = "succeeded"
	run.ReportHash = hash
	run.ProgressLabel = "知识候选已进入人工审核"
	run.UpdatedAt = s.now().UTC()
	if err := s.succeedRunAttempt(ctx, &run, attempt); err != nil {
		return domain.KnowledgeExtractionResult{}, err
	}
	if err := s.store.SaveRun(ctx, run); err != nil {
		return domain.KnowledgeExtractionResult{}, err
	}
	result, err := s.knowledgeExtractionResult(ctx, actor.TenantID, run, pkg.Warnings)
	if err == nil {
		s.audit(ctx, actor, run.ProjectID, "knowledge_extraction_run.reported", "task_run", run.ID, requestID, map[string]any{"candidate_count": len(result.Items), "conflict_count": len(result.Conflicts)})
	}
	return result, err
}

func (s *Service) rejectKnowledgeOutput(ctx context.Context, run domain.TaskRun, attempt domain.RunAttempt, failureClass, summary string, reportErr error) (domain.KnowledgeExtractionResult, error) {
	if _, finishErr := s.failRunAttempt(ctx, run, attempt, failureClass, nil, nil, summary); finishErr != nil {
		return domain.KnowledgeExtractionResult{}, errors.Join(reportErr, finishErr)
	}
	return domain.KnowledgeExtractionResult{}, reportErr
}

func (s *Service) failMalformedOutput(ctx context.Context, actor Actor, device domain.Device, run domain.TaskRun, attemptID, runToken, failureClass string) {
	attempt, err := s.activeRunAttempt(ctx, actor, device, run, attemptID, runToken, s.now().UTC())
	if err == nil {
		_, _ = s.failRunAttempt(ctx, run, attempt, failureClass, nil, nil, "本地 Agent 返回无法解析的结构化输出")
	}
}

func knowledgeCandidateInput(run domain.TaskRun, candidate domain.KnowledgeCandidate) CreateKnowledgeInput {
	return CreateKnowledgeInput{ProjectID: run.ProjectID, Kind: candidate.Kind, Title: strings.TrimSpace(candidate.Title), Statement: strings.TrimSpace(candidate.Statement), Subject: strings.TrimSpace(candidate.Subject), Predicate: strings.TrimSpace(candidate.Predicate), Value: candidate.Value, Scope: candidate.Scope, RiskLevel: candidate.RiskLevel, AllowedChannels: candidate.AllowedChannels, Evidence: candidate.Evidence, ForbiddenExtensions: candidate.ForbiddenExtensions, DependsOnFactIDs: candidate.DependsOnFactIDs, ValidFrom: candidate.ValidFrom, ValidUntil: candidate.ValidUntil, ExpiresAt: candidate.ExpiresAt, OriginRunID: run.ID}
}

func knowledgeCandidateEnumsValid(candidate domain.KnowledgeCandidate) bool {
	allowedKind := map[string]bool{"fact": true, "claim": true, "visual_rule": true, "methodology": true}
	allowedRisk := map[string]bool{"low": true, "medium": true, "high": true}
	return allowedKind[candidate.Kind] && allowedRisk[candidate.RiskLevel] && strings.TrimSpace(candidate.Subject) != "" && strings.TrimSpace(candidate.Predicate) != "" && len(candidate.Evidence) > 0
}

func evidenceWithinKnowledgeContract(refs []domain.EvidenceRef, sources []domain.ContractSource) bool {
	allowed := map[string]bool{}
	for _, source := range sources {
		for _, evidence := range source.Evidence {
			locator, _ := json.Marshal(evidence.Locator)
			allowed[source.RevisionID+"\x00"+evidence.LocatorKind+"\x00"+string(locator)+"\x00"+evidence.Quote] = true
		}
	}
	for _, ref := range refs {
		var locator any
		if json.Unmarshal([]byte(ref.Locator), &locator) != nil {
			return false
		}
		canonical, _ := json.Marshal(locator)
		if !allowed[ref.SourceRevisionID+"\x00"+ref.LocatorKind+"\x00"+string(canonical)+"\x00"+ref.Quote] {
			return false
		}
	}
	return true
}

func evidenceLocatorMatches(encoded string, locator map[string]any) bool {
	var decoded any
	if json.Unmarshal([]byte(encoded), &decoded) != nil {
		return false
	}
	wanted, err := domain.CanonicalHash(decoded)
	if err != nil {
		return false
	}
	actual, err := domain.CanonicalHash(locator)
	return err == nil && wanted == actual
}

func (s *Service) knowledgeExtractionResult(ctx context.Context, tenantID string, run domain.TaskRun, warnings []string) (domain.KnowledgeExtractionResult, error) {
	all, err := s.store.Knowledge(ctx, tenantID, run.ProjectID)
	if err != nil {
		return domain.KnowledgeExtractionResult{}, err
	}
	items := []domain.KnowledgeItem{}
	ids := map[string]bool{}
	for _, item := range all {
		if item.OriginRunID == run.ID {
			items = append(items, item)
			ids[item.ID] = true
		}
	}
	allConflicts, err := s.store.KnowledgeConflicts(ctx, tenantID, run.ProjectID)
	if err != nil {
		return domain.KnowledgeExtractionResult{}, err
	}
	conflicts := []domain.KnowledgeConflict{}
	for _, conflict := range allConflicts {
		for _, id := range conflict.KnowledgeItemIDs {
			if ids[id] {
				conflicts = append(conflicts, conflict)
				break
			}
		}
	}
	return domain.KnowledgeExtractionResult{RunID: run.ID, Items: items, Conflicts: conflicts, Warnings: warnings}, nil
}

func decodeStrict(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return domain.Invalid("INPUT_INVALID", "JSON 只能包含一个对象")
		}
		return err
	}
	return nil
}
