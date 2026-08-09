package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
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
			return domain.TaskRun{}, domain.Policy("SOURCE_REVISION_NOT_READY", "来源版本尚未完成可信解析", "等待来源状态变为“就绪（ready）”")
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
			return domain.TaskRun{}, domain.Policy("ACCEPTED_EVIDENCE_REQUIRED", "来源版本没有可用于提取的已接受证据", "先复核并接受至少一个证据片段")
		}
		sources = append(sources, domain.ContractSource{SourceID: source.ID, RevisionID: revision.ID, Name: source.Name, SourceType: source.SourceType, FileName: revision.FileName, SHA256: revision.SHA256, DetectedMIME: revision.DetectedMIME, Evidence: evidence})
	}
	snapshot, err := domain.CompileKnowledgeSnapshot(project, sources, s.now())
	if err != nil {
		return domain.TaskRun{}, err
	}
	// The frozen input snapshot is content-addressed so concurrent retries do
	// not create different Runtime admission identities for the same contract.
	snapshot.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("contentcloud:context-snapshot:"+snapshot.ManifestHash)).String()
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		in.IdempotencyKey = domain.NewID()
	}
	if s.runtimeService == nil {
		return domain.TaskRun{}, domain.Policy("RUNTIME_UNAVAILABLE", "知识提取需要已配置的 Runtime", "联系平台运营人员启用 Runtime")
	}
	runtimeKey := "knowledge-extraction:" + project.ID + ":" + in.IdempotencyKey
	if existing, lookupErr := s.runtimeService.JobByIdempotencyKey(ctx, actor.TenantID, runtimeKey); lookupErr == nil {
		if existing.ProjectID != project.ID || existing.BusinessType != "knowledge_extract" || existing.InputDigest != "sha256:"+snapshot.ManifestHash || existing.BusinessOutputCount != in.OutputCount {
			return domain.TaskRun{}, domain.Conflict("JOB_RUN_IDEMPOTENCY_MISMATCH", "知识提取幂等键已用于不同的输入契约")
		}
		return s.projectRuntimeJob(ctx, existing)
	} else if !domain.IsNotFound(lookupErr) {
		return domain.TaskRun{}, lookupErr
	}
	if err := s.store.CreateSnapshot(ctx, snapshot); err != nil {
		existing, lookupErr := s.store.Snapshot(ctx, actor.TenantID, snapshot.ID)
		if lookupErr != nil || !sameContextSnapshotIdentity(existing, snapshot) {
			return domain.TaskRun{}, err
		}
	}
	start, err := s.runtimeService.Start(ctx, contentruntime.StartInput{
		TenantID: actor.TenantID, ProjectID: project.ID,
		WorkTaskID:   runtimeKey,
		BusinessType: "knowledge_extract", InputSnapshotID: snapshot.ID, BusinessOutputCount: in.OutputCount,
		SOP: knowledgeExtractionSOP(), BindingDigest: "sha256:" + snapshot.ManifestHash,
		InputDigest: "sha256:" + snapshot.ManifestHash, RuntimePolicyID: "runtime-policy/knowledge-extract-v1",
		ContractMajor: 1, ContractMinor: 0, Priority: 60, CreatedBy: actor.UserID,
		IdempotencyKey: runtimeKey, CorrelationID: requestID,
	})
	if err != nil {
		return domain.TaskRun{}, err
	}
	projected, err := s.projectRuntimeJob(ctx, start.Job)
	if err != nil {
		return domain.TaskRun{}, err
	}
	projected.InputSnapshotID = snapshot.ID
	projected.OutputCount = in.OutputCount
	s.audit(ctx, actor, start.Job.ProjectID, "knowledge_extraction_run.created", "job_run", start.Job.ID, requestID, map[string]any{"snapshot_id": snapshot.ID, "source_revision_count": len(sources), "manifest_hash": snapshot.ManifestHash})
	return projected, nil
}

func sameContextSnapshotIdentity(existing, requested domain.ContextSnapshot) bool {
	return existing.ID == requested.ID &&
		existing.TenantID == requested.TenantID &&
		existing.ProjectID == requested.ProjectID &&
		existing.BuilderVersion == requested.BuilderVersion &&
		existing.SchemaVersion == requested.SchemaVersion &&
		existing.ManifestHash == requested.ManifestHash
}

func knowledgeExtractionSOP() domain.SOPVersion {
	digest, _ := domain.CanonicalHash(struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
		Schema  string `json:"schema"`
	}{"contentcloud.knowledge.extract", 1, domain.KnowledgeCandidatesSchema})
	return domain.SOPVersion{
		ID: "contentcloud.knowledge.extract/v1", SOPID: "contentcloud.knowledge.extract", Version: 1,
		SchemaVersion: domain.SOPSchemaVersion, Name: "知识候选提取", Status: "published",
		ContentTypes: []string{"knowledge_extract"}, DefaultExecutionMode: "agent", Digest: "sha256:" + digest,
		Stages: []domain.StageDefinition{{ID: "knowledge_extract", Name: "提取知识候选", Order: 10, OutputSchema: domain.KnowledgeCandidatesSchema, RequiredCapabilities: []string{domain.KnowledgeExtractCapability}, ExecutionModes: []string{"agent"}, RetryMaxAttempts: 3}},
	}
}

// importKnowledgePackage is the Runtime worker business handoff boundary. It
// validates the frozen evidence contract before creating candidates and uses a
// package digest marker to make retries idempotent.
func (s *Service) importKnowledgePackage(ctx context.Context, actor Actor, run domain.TaskRun, pkg domain.KnowledgeExtractionPackage, requestID string) (domain.KnowledgeExtractionResult, string, error) {
	hash, inputs, err := s.validateKnowledgePackage(ctx, actor, run, pkg)
	if err != nil {
		return domain.KnowledgeExtractionResult{}, "", err
	}
	objects := make([]domain.KnowledgeObject, 0, len(inputs))
	for _, input := range inputs {
		existing, lookupErr := s.store.KnowledgeObject(ctx, actor.TenantID, input.ID, 0)
		if lookupErr == nil {
			marker, _ := existing.Payload["runtime_result_digest"].(string)
			if marker != "sha256:"+hash || existing.ProjectID != run.ProjectID {
				return domain.KnowledgeExtractionResult{}, "", domain.Conflict("RUNTIME_BUSINESS_RESULT_CONFLICT", "同一 Runtime 结果引用了不同的知识候选内容")
			}
			objects = append(objects, existing)
			continue
		}
		if !domain.IsNotFound(lookupErr) {
			return domain.KnowledgeExtractionResult{}, "", lookupErr
		}
		object, createErr := s.CreateKnowledgeObject(ctx, actor, input, requestID)
		if createErr != nil {
			return domain.KnowledgeExtractionResult{}, "", createErr
		}
		objects = append(objects, object)
	}
	return domain.KnowledgeExtractionResult{RunID: run.ID, Objects: objects, Warnings: pkg.Warnings}, "sha256:" + hash, nil
}

func (s *Service) validateKnowledgePackage(ctx context.Context, actor Actor, run domain.TaskRun, pkg domain.KnowledgeExtractionPackage) (string, []CreateKnowledgeObjectInput, error) {
	hash, err := domain.CanonicalHash(pkg)
	if err != nil {
		return "", nil, err
	}
	if pkg.SchemaVersion != "1.0" || len(pkg.Candidates) == 0 || (run.OutputCount > 0 && len(pkg.Candidates) > run.OutputCount) || len(pkg.Candidates) > 20 {
		return "", nil, domain.Invalid("KNOWLEDGE_PACKAGE_INVALID", "知识候选格式版本或候选数量不符合任务契约")
	}
	project, err := s.projectForWrite(ctx, actor, run.ProjectID)
	if err != nil {
		return "", nil, err
	}
	if project.Status == "archived" {
		return "", nil, domain.Policy("PROJECT_ARCHIVED", "已归档项目不能接收新的智能体结果", "恢复项目或取消任务")
	}
	if strings.TrimSpace(run.InputSnapshotID) == "" {
		return "", nil, domain.Invalid("KNOWLEDGE_INPUT_SNAPSHOT_REQUIRED", "知识提取缺少冻结输入快照")
	}
	snapshot, err := s.store.Snapshot(ctx, actor.TenantID, run.InputSnapshotID)
	if err != nil {
		return "", nil, err
	}
	inputs := make([]CreateKnowledgeObjectInput, 0, len(pkg.Candidates))
	for index, candidate := range pkg.Candidates {
		if !knowledgeCandidateEnumsValid(candidate) || !evidenceWithinKnowledgeContract(candidate.Evidence, snapshot.Sources) {
			return "", nil, domain.E("content", "policy", "KNOWLEDGE_CANDIDATE_GROUNDING_INVALID", "知识候选类型、风险或证据超出任务契约", 7)
		}
		evidenceIDs, err := s.knowledgeEvidenceIDs(ctx, actor.TenantID, run.ProjectID, candidate.Evidence)
		if err != nil {
			return "", nil, err
		}
		input := knowledgeCandidateObjectInput(run, candidate, evidenceIDs, index)
		if input.Payload == nil {
			input.Payload = map[string]any{}
		}
		input.Payload["runtime_result_digest"] = "sha256:" + hash
		inputs = append(inputs, input)
	}
	return hash, inputs, nil
}

func knowledgeCandidateObjectInput(run domain.TaskRun, candidate domain.KnowledgeCandidate, evidenceIDs []string, index int) CreateKnowledgeObjectInput {
	objectType, layer := knowledgeCandidateObjectType(candidate.Kind)
	payload := map[string]any{
		"kind":                 candidate.Kind,
		"subject":              strings.TrimSpace(candidate.Subject),
		"predicate":            strings.TrimSpace(candidate.Predicate),
		"value":                candidate.Value,
		"scope":                candidate.Scope,
		"risk_level":           candidate.RiskLevel,
		"forbidden_extensions": append([]string(nil), candidate.ForbiddenExtensions...),
		"depends_on_fact_ids":  append([]string(nil), candidate.DependsOnFactIDs...),
		"origin_run_id":        run.ID,
	}
	return CreateKnowledgeObjectInput{ProjectID: run.ProjectID, ID: "knowledge:" + run.ID + ":" + strconv.Itoa(index+1), ObjectType: objectType, Layer: layer, Status: "needs_review", Title: strings.TrimSpace(candidate.Title), Statement: strings.TrimSpace(candidate.Statement), Payload: payload, AllowedChannels: append([]string(nil), candidate.AllowedChannels...), EvidenceRefs: evidenceIDs, ValidFrom: candidate.ValidFrom, ValidUntil: candidate.ValidUntil, ExpiresAt: candidate.ExpiresAt}
}

func knowledgeCandidateObjectType(kind string) (string, string) {
	switch kind {
	case "fact":
		return "FactAssertion", "product"
	case "claim":
		return "Claim", "expression"
	case "visual_rule":
		return "BrandRule", "identity"
	case "methodology":
		return "Process", "operations"
	default:
		return "DomainObject", "operations"
	}
}

func knowledgeCandidateEnumsValid(candidate domain.KnowledgeCandidate) bool {
	allowedKind := map[string]bool{"fact": true, "claim": true, "visual_rule": true, "methodology": true}
	allowedRisk := map[string]bool{"low": true, "medium": true, "high": true}
	return allowedKind[candidate.Kind] && allowedRisk[candidate.RiskLevel] && strings.TrimSpace(candidate.Subject) != "" && strings.TrimSpace(candidate.Predicate) != "" && len(candidate.Evidence) > 0
}

func (s *Service) knowledgeEvidenceIDs(ctx context.Context, tenantID, projectID string, refs []domain.EvidenceRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.SourceRevisionID) == "" || strings.TrimSpace(ref.LocatorKind) == "" || strings.TrimSpace(ref.Quote) == "" {
			return nil, domain.Invalid("EVIDENCE_REF_INVALID", "证据引用缺少来源版本、定位类型或原文")
		}
		revision, err := s.store.SourceRevision(ctx, tenantID, ref.SourceRevisionID)
		if err != nil {
			return nil, err
		}
		if revision.ProjectID != projectID {
			return nil, domain.Policy("EVIDENCE_PROJECT_MISMATCH", "证据来源不属于当前项目", "选择当前项目内已接受的证据")
		}
		spans, err := s.store.Evidence(ctx, tenantID, revision.ID)
		if err != nil {
			return nil, err
		}
		matched := ""
		for _, span := range spans {
			if span.ProjectID == projectID && span.LocatorKind == ref.LocatorKind && evidenceLocatorMatches(ref.Locator, span.Locator) && span.QuoteText == ref.Quote && span.ReviewStatus == "accepted" {
				matched = span.ID
				break
			}
		}
		if matched == "" {
			return nil, domain.Policy("EVIDENCE_NOT_ACCEPTED", "证据原文或定位与已接受的片段不一致", "重新选择来源中已接受的证据")
		}
		ids = append(ids, matched)
	}
	return ids, nil
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
