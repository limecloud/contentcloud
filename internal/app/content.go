package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

var allowedSourceMIME = map[string]bool{
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"image/png":  true,
	"image/jpeg": true,
	"text/plain": true,
}

type CreateSourceInput struct {
	ProjectID     string     `json:"project_id"`
	Name          string     `json:"name"`
	SourceType    string     `json:"source_type"`
	FileName      string     `json:"file_name"`
	MIME          string     `json:"mime"`
	SHA256        string     `json:"sha256"`
	ByteSize      int64      `json:"byte_size"`
	ObjectKey     string     `json:"object_key"`
	EffectiveFrom *time.Time `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
}

func (s *Service) CreateSource(ctx context.Context, actor Actor, in CreateSourceInput, requestID string) (domain.SourceRevision, error) {
	if _, err := s.projectForWrite(ctx, actor, in.ProjectID); err != nil {
		return domain.SourceRevision{}, err
	}
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return domain.SourceRevision{}, err
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.FileName) == "" {
		return domain.SourceRevision{}, domain.Invalid("SOURCE_FIELDS_REQUIRED", "来源名称和文件名必填")
	}
	if !allowedSourceMIME[in.MIME] {
		return domain.SourceRevision{}, domain.E("validation", "content_type", "SOURCE_MIME_BLOCKED", "仅支持 PDF、DOCX、XLSX、PPTX、PNG 和 JPEG", 7)
	}
	if in.ByteSize <= 0 || in.ByteSize > 100*1024*1024 {
		return domain.SourceRevision{}, domain.Invalid("SOURCE_SIZE_INVALID", "单文件大小必须在 1B 到 100MB 之间")
	}
	if len(in.SHA256) != 64 {
		return domain.SourceRevision{}, domain.Invalid("SOURCE_HASH_INVALID", "来源必须提供 SHA-256")
	}
	now := s.now().UTC()
	source := domain.Source{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: in.ProjectID, Name: strings.TrimSpace(in.Name), SourceType: defaultString(in.SourceType, "brand_manual"), Status: "pending", RevisionCount: 1, CreatedAt: now}
	revisionID := domain.NewID()
	objectKey := in.ObjectKey
	if objectKey == "" {
		objectKey = fmt.Sprintf("tenants/%s/projects/%s/sources/%s/%s/original", actor.TenantID, in.ProjectID, source.ID, revisionID)
	}
	revision := domain.SourceRevision{ID: revisionID, TenantID: actor.TenantID, ProjectID: in.ProjectID, SourceID: source.ID, FileName: in.FileName, ObjectKey: objectKey, SHA256: strings.ToLower(in.SHA256), ByteSize: in.ByteSize, DeclaredMIME: in.MIME, DetectedMIME: in.MIME, ProcessingStatus: "pending", UploadedBy: actor.UserID, EffectiveFrom: in.EffectiveFrom, EffectiveTo: in.EffectiveTo, CreatedAt: now}
	source.LatestRevision = revision.ID
	if err := s.store.CreateSource(ctx, source, revision); err != nil {
		return revision, err
	}
	s.audit(ctx, actor, in.ProjectID, "source.created", "source_revision", revision.ID, requestID, map[string]any{"source_id": source.ID, "mime": in.MIME, "byte_size": in.ByteSize, "sha256": in.SHA256})
	return revision, nil
}

type CreateSourceRevisionInput struct {
	SourceID      string     `json:"source_id"`
	FileName      string     `json:"file_name"`
	MIME          string     `json:"mime"`
	SHA256        string     `json:"sha256"`
	ByteSize      int64      `json:"byte_size"`
	ObjectKey     string     `json:"object_key"`
	EffectiveFrom *time.Time `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
}

func (s *Service) CreateSourceRevision(ctx context.Context, actor Actor, in CreateSourceRevisionInput, requestID string) (domain.SourceRevision, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return domain.SourceRevision{}, err
	}
	source, err := s.store.Source(ctx, actor.TenantID, in.SourceID)
	if err != nil {
		return domain.SourceRevision{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, source.ProjectID); err != nil {
		return domain.SourceRevision{}, err
	}
	if strings.TrimSpace(in.FileName) == "" || !allowedSourceMIME[in.MIME] {
		return domain.SourceRevision{}, domain.Invalid("SOURCE_REVISION_INVALID", "来源修订需要支持的文件名和 MIME")
	}
	if in.ByteSize <= 0 || in.ByteSize > 100*1024*1024 || len(in.SHA256) != 64 {
		return domain.SourceRevision{}, domain.Invalid("SOURCE_REVISION_INVALID", "来源修订大小或 SHA-256 无效")
	}
	revisions, err := s.store.SourceRevisions(ctx, actor.TenantID, source.ID)
	if err != nil {
		return domain.SourceRevision{}, err
	}
	now := s.now().UTC()
	revision := domain.SourceRevision{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: source.ProjectID, SourceID: source.ID, FileName: strings.TrimSpace(in.FileName), SHA256: strings.ToLower(in.SHA256), ByteSize: in.ByteSize, DeclaredMIME: in.MIME, DetectedMIME: in.MIME, ProcessingStatus: "pending", UploadedBy: actor.UserID, EffectiveFrom: in.EffectiveFrom, EffectiveTo: in.EffectiveTo, CreatedAt: now}
	if len(revisions) > 0 {
		revision.SupersedesID = revisions[0].ID
	}
	revision.ObjectKey = in.ObjectKey
	if revision.ObjectKey == "" {
		revision.ObjectKey = fmt.Sprintf("tenants/%s/projects/%s/sources/%s/%s/original", actor.TenantID, source.ProjectID, source.ID, revision.ID)
	}
	if err := s.store.CreateSourceRevision(ctx, revision); err != nil {
		return revision, err
	}
	impacts, err := s.propagateSourceChange(ctx, actor, source, revisions, requestID)
	if err != nil {
		return revision, err
	}
	s.audit(ctx, actor, source.ProjectID, "source.revision_created", "source_revision", revision.ID, requestID, map[string]any{"source_id": source.ID, "supersedes_id": revision.SupersedesID, "affected_objects": len(impacts)})
	return revision, nil
}

func (s *Service) UploadSourceRevision(ctx context.Context, actor Actor, sourceID, fileName, declaredMIME string, data []byte, requestID string) (domain.SourceRevision, error) {
	if len(data) == 0 || len(data) > 100*1024*1024 {
		return domain.SourceRevision{}, domain.Invalid("SOURCE_SIZE_INVALID", "单文件大小必须在 1B 到 100MB 之间")
	}
	extension := strings.ToLower(filepath.Ext(fileName))
	extensionMIME := map[string]string{".pdf": "application/pdf", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".txt": "text/plain"}
	if expected, ok := extensionMIME[extension]; !ok || expected != declaredMIME {
		return domain.SourceRevision{}, domain.E("validation", "content_type", "SOURCE_MIME_MISMATCH", "文件扩展名与声明类型不一致", 7)
	}
	sum := sha256.Sum256(data)
	revision, err := s.CreateSourceRevision(ctx, actor, CreateSourceRevisionInput{SourceID: sourceID, FileName: fileName, MIME: declaredMIME, SHA256: hex.EncodeToString(sum[:]), ByteSize: int64(len(data))}, requestID)
	if err != nil {
		return revision, err
	}
	if err := s.blobs.Put(ctx, revision.ObjectKey, data); err != nil {
		revision.ProcessingStatus = "failed"
		revision.ErrorCode = "OBJECT_WRITE_FAILED"
		_ = s.store.SaveSourceRevision(ctx, revision)
		return revision, err
	}
	return revision, nil
}

func (s *Service) UploadSource(ctx context.Context, actor Actor, projectID, name, sourceType, fileName, declaredMIME string, data []byte, requestID string) (domain.SourceRevision, error) {
	if len(data) == 0 || len(data) > 100*1024*1024 {
		return domain.SourceRevision{}, domain.Invalid("SOURCE_SIZE_INVALID", "单文件大小必须在 1B 到 100MB 之间")
	}
	extension := strings.ToLower(filepath.Ext(fileName))
	extensionMIME := map[string]string{".pdf": "application/pdf", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".txt": "text/plain"}
	expected, ok := extensionMIME[extension]
	if !ok || declaredMIME != expected {
		return domain.SourceRevision{}, domain.E("validation", "content_type", "SOURCE_MIME_MISMATCH", "文件扩展名与声明类型不一致", 7)
	}
	sum := sha256.Sum256(data)
	revision, err := s.CreateSource(ctx, actor, CreateSourceInput{ProjectID: projectID, Name: name, SourceType: sourceType, FileName: filepath.Base(fileName), MIME: declaredMIME, SHA256: hex.EncodeToString(sum[:]), ByteSize: int64(len(data))}, requestID)
	if err != nil {
		return revision, err
	}
	if err := s.blobs.Put(ctx, revision.ObjectKey, data); err != nil {
		revision.ProcessingStatus = "failed"
		revision.ErrorCode = "OBJECT_WRITE_FAILED"
		_ = s.store.SaveSourceRevision(ctx, revision)
		return revision, err
	}
	return revision, nil
}

func (s *Service) Sources(ctx context.Context, actor Actor, projectID string) ([]domain.Source, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.Sources(ctx, actor.TenantID, projectID)
}

func (s *Service) SourceRevisions(ctx context.Context, actor Actor, sourceID string) ([]domain.SourceRevision, error) {
	source, err := s.store.Source(ctx, actor.TenantID, sourceID)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Project(ctx, actor.TenantID, source.ProjectID); err != nil {
		return nil, err
	}
	return s.store.SourceRevisions(ctx, actor.TenantID, sourceID)
}

func (s *Service) Evidence(ctx context.Context, actor Actor, revisionID string) ([]domain.EvidenceSpan, error) {
	revision, err := s.store.SourceRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Project(ctx, actor.TenantID, revision.ProjectID); err != nil {
		return nil, err
	}
	return s.store.Evidence(ctx, actor.TenantID, revisionID)
}

func (s *Service) SourceRevision(ctx context.Context, actor Actor, revisionID string) (domain.SourceRevision, error) {
	revision, err := s.store.SourceRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return revision, err
	}
	if _, err := s.store.Project(ctx, actor.TenantID, revision.ProjectID); err != nil {
		return revision, err
	}
	return revision, nil
}

func (s *Service) PendingSourceRevisions(ctx context.Context, limit int) ([]domain.SourceRevision, error) {
	return s.store.PendingSourceRevisions(ctx, limit)
}

func (s *Service) SourceRevisionBytes(ctx context.Context, revision domain.SourceRevision) ([]byte, error) {
	return s.blobs.Get(ctx, revision.ObjectKey)
}

func (s *Service) ClaimSourceRevision(ctx context.Context, tenantID, revisionID string) (domain.SourceRevision, bool, error) {
	return s.store.ClaimSourceRevision(ctx, tenantID, revisionID)
}

type CompleteSourceInput struct {
	DetectedMIME  string                `json:"detected_mime"`
	Status        string                `json:"status"`
	ParserVersion string                `json:"parser_version"`
	ErrorCode     string                `json:"error_code"`
	Evidence      []CreateEvidenceInput `json:"evidence"`
}

type CreateEvidenceInput struct {
	LocatorKind   string         `json:"locator_kind"`
	Locator       map[string]any `json:"locator"`
	QuoteText     string         `json:"quote_text"`
	OCRConfidence *float64       `json:"ocr_confidence,omitempty"`
}

func (s *Service) CompleteSource(ctx context.Context, actor Actor, revisionID string, in CompleteSourceInput, requestID string) (domain.SourceRevision, error) {
	revision, err := s.store.SourceRevision(ctx, actor.TenantID, revisionID)
	if err != nil {
		return revision, err
	}
	if _, err := s.projectForWrite(ctx, actor, revision.ProjectID); err != nil {
		return revision, err
	}
	if actor.Type != "worker" && actor.Role != "tenant_admin" && actor.Role != "reviewer" {
		return revision, domain.Policy("ROLE_DENIED", "当前角色不能完成来源解析", "等待 Worker 或审核员处理")
	}
	if in.Status != "ready" && in.Status != "needs_review" && in.Status != "failed" {
		return revision, domain.Invalid("SOURCE_STATUS_INVALID", "解析状态无效")
	}
	if in.DetectedMIME != "" && in.DetectedMIME != revision.DeclaredMIME {
		in.Status = "failed"
		in.ErrorCode = "MIME_MISMATCH"
	}
	revision.DetectedMIME = defaultString(in.DetectedMIME, revision.DeclaredMIME)
	revision.ProcessingStatus = in.Status
	revision.ParserVersion = in.ParserVersion
	revision.ErrorCode = in.ErrorCode
	if err := s.store.SaveSourceRevision(ctx, revision); err != nil {
		return revision, err
	}
	if in.Status != "failed" {
		for _, candidate := range in.Evidence {
			quote := strings.TrimSpace(candidate.QuoteText)
			if quote == "" || candidate.LocatorKind == "" {
				continue
			}
			sum := sha256.Sum256([]byte(quote))
			status := "accepted"
			if candidate.OCRConfidence != nil && *candidate.OCRConfidence < 0.85 {
				status = "needs_review"
			}
			span := domain.EvidenceSpan{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, RevisionID: revision.ID, LocatorKind: candidate.LocatorKind, Locator: candidate.Locator, QuoteText: quote, QuoteHash: hex.EncodeToString(sum[:]), OCRConfidence: candidate.OCRConfidence, ReviewStatus: status, CreatedAt: s.now().UTC()}
			if err := s.store.CreateEvidence(ctx, span); err != nil {
				return revision, err
			}
		}
	}
	s.audit(ctx, actor, revision.ProjectID, "source.processed", "source_revision", revision.ID, requestID, map[string]any{"status": revision.ProcessingStatus, "evidence_count": len(in.Evidence), "error_code": revision.ErrorCode})
	return revision, nil
}

func (s *Service) ReviewEvidence(ctx context.Context, actor Actor, evidenceID, decision, requestID string) (domain.EvidenceSpan, error) {
	if err := requireRole(actor, "tenant_admin", "reviewer"); err != nil {
		return domain.EvidenceSpan{}, err
	}
	span, err := s.store.EvidenceSpan(ctx, actor.TenantID, evidenceID)
	if err != nil {
		return span, err
	}
	if _, err := s.projectForWrite(ctx, actor, span.ProjectID); err != nil {
		return span, err
	}
	previous := span.ReviewStatus
	switch decision {
	case "accept":
		span.ReviewStatus = "accepted"
	case "reject":
		span.ReviewStatus = "rejected"
	default:
		return span, domain.Invalid("EVIDENCE_DECISION_INVALID", "证据复核只允许 accept 或 reject")
	}
	now := s.now().UTC()
	span.ReviewedBy, span.ReviewedAt = actor.UserID, &now
	if err := s.store.SaveEvidence(ctx, span); err != nil {
		return span, err
	}
	if previous == "accepted" && span.ReviewStatus != "accepted" {
		revision, _ := s.store.SourceRevision(ctx, actor.TenantID, span.RevisionID)
		source, _ := s.store.Source(ctx, actor.TenantID, revision.SourceID)
		revisions, _ := s.store.SourceRevisions(ctx, actor.TenantID, source.ID)
		_, _ = s.propagateSourceChange(ctx, actor, source, revisions, requestID)
	}
	s.audit(ctx, actor, span.ProjectID, "evidence.reviewed", "evidence_span", span.ID, requestID, map[string]any{"from": previous, "to": span.ReviewStatus})
	return span, nil
}

type ImpactItem struct {
	ObjectType      string `json:"object_type"`
	ObjectID        string `json:"object_id"`
	Reason          string `json:"reason"`
	CurrentStatus   string `json:"current_status"`
	SuggestedAction string `json:"suggested_action"`
}

func (s *Service) SourceImpact(ctx context.Context, actor Actor, sourceID string) ([]ImpactItem, error) {
	source, err := s.store.Source(ctx, actor.TenantID, sourceID)
	if err != nil {
		return nil, err
	}
	revisions, err := s.store.SourceRevisions(ctx, actor.TenantID, source.ID)
	if err != nil {
		return nil, err
	}
	return s.collectSourceImpact(ctx, actor.TenantID, source.ProjectID, revisions), nil
}

func (s *Service) propagateSourceChange(ctx context.Context, actor Actor, source domain.Source, revisions []domain.SourceRevision, requestID string) ([]ImpactItem, error) {
	impacts := s.collectSourceImpact(ctx, actor.TenantID, source.ProjectID, revisions)
	changedKnowledge := map[string]bool{}
	for _, impact := range impacts {
		if impact.ObjectType != "knowledge_item" {
			continue
		}
		item, err := s.store.KnowledgeItem(ctx, actor.TenantID, impact.ObjectID)
		if err != nil || (item.Status != "approved" && item.Status != "expired") {
			continue
		}
		item.Status = "review_required"
		item.RowVersion++
		item.UpdatedAt = s.now().UTC()
		if err := s.store.SaveKnowledge(ctx, item); err != nil {
			return impacts, err
		}
		changedKnowledge[item.ID] = true
	}
	if len(changedKnowledge) == 0 {
		return impacts, nil
	}
	briefs, _ := s.store.Briefs(ctx, actor.TenantID, source.ProjectID)
	changedBriefs := map[string]bool{}
	for _, brief := range briefs {
		if !containsAny(brief.ApprovedKnowledgeIDs, changedKnowledge) {
			continue
		}
		if brief.Status == "approved" || brief.Status == "internal_review" {
			brief.Status = "review_required"
			if err := s.store.SaveBrief(ctx, brief); err != nil {
				return impacts, err
			}
			changedBriefs[brief.ID] = true
		}
	}
	scripts, _ := s.store.Scripts(ctx, actor.TenantID, source.ProjectID)
	for _, script := range scripts {
		run, err := s.store.Run(ctx, actor.TenantID, script.RunID)
		if err != nil || !changedBriefs[run.BriefVersionID] {
			continue
		}
		if script.Status != "blocked" && script.Status != "revision_requested" {
			script.Status = "review_required"
			if err := s.store.SaveScript(ctx, script); err != nil {
				return impacts, err
			}
		}
	}
	s.audit(ctx, actor, source.ProjectID, "source.impact_propagated", "source", source.ID, requestID, map[string]any{"knowledge_count": len(changedKnowledge), "brief_count": len(changedBriefs)})
	return s.collectSourceImpact(ctx, actor.TenantID, source.ProjectID, revisions), nil
}

func (s *Service) collectSourceImpact(ctx context.Context, tenantID, projectID string, revisions []domain.SourceRevision) []ImpactItem {
	revisionIDs := map[string]bool{}
	for _, revision := range revisions {
		revisionIDs[revision.ID] = true
	}
	impacts := []ImpactItem{}
	knowledge, _ := s.store.Knowledge(ctx, tenantID, projectID)
	knowledgeIDs := map[string]bool{}
	for _, item := range knowledge {
		for _, evidence := range item.Evidence {
			if revisionIDs[evidence.SourceRevisionID] {
				knowledgeIDs[item.ID] = true
				impacts = append(impacts, ImpactItem{ObjectType: "knowledge_item", ObjectID: item.ID, Reason: "引用了该逻辑来源的证据", CurrentStatus: item.Status, SuggestedAction: "复核新修订中的原文和值"})
				break
			}
		}
	}
	briefIDs := map[string]bool{}
	briefs, _ := s.store.Briefs(ctx, tenantID, projectID)
	for _, brief := range briefs {
		if containsAny(brief.ApprovedKnowledgeIDs, knowledgeIDs) {
			briefIDs[brief.ID] = true
			impacts = append(impacts, ImpactItem{ObjectType: "brief_version", ObjectID: brief.ID, Reason: "引用了受影响知识", CurrentStatus: brief.Status, SuggestedAction: "知识复核后创建或批准新 Brief 版本"})
		}
	}
	scripts, _ := s.store.Scripts(ctx, tenantID, projectID)
	for _, script := range scripts {
		run, err := s.store.Run(ctx, tenantID, script.RunID)
		if err == nil && briefIDs[run.BriefVersionID] {
			impacts = append(impacts, ImpactItem{ObjectType: "script_version", ObjectID: script.ID, Reason: "输入 Brief 的知识依据已变化", CurrentStatus: script.Status, SuggestedAction: "基于新 Brief 修订剧本"})
		}
	}
	return impacts
}

func containsAny(values []string, wanted map[string]bool) bool {
	for _, value := range values {
		if wanted[value] {
			return true
		}
	}
	return false
}

type CreateBenchmarkInput struct {
	ProjectID       string `json:"project_id"`
	Title           string `json:"title"`
	Platform        string `json:"platform"`
	OriginalURL     string `json:"original_url"`
	RightsMode      string `json:"rights_mode"`
	ValidationLevel string `json:"validation_level"`
	ValidationNote  string `json:"validation_note"`
}

func (s *Service) CreateBenchmark(ctx context.Context, actor Actor, in CreateBenchmarkInput, requestID string) (domain.BenchmarkContent, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return domain.BenchmarkContent{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, in.ProjectID); err != nil {
		return domain.BenchmarkContent{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return domain.BenchmarkContent{}, domain.Invalid("BENCHMARK_TITLE_REQUIRED", "对标内容标题必填")
	}
	if in.OriginalURL != "" {
		parsed, err := url.ParseRequestURI(in.OriginalURL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return domain.BenchmarkContent{}, domain.Invalid("BENCHMARK_URL_INVALID", "对标链接无效")
		}
	}
	level := defaultString(in.ValidationLevel, "observed")
	if level == "internally_verified" && strings.TrimSpace(in.ValidationNote) == "" {
		return domain.BenchmarkContent{}, domain.Policy("SALES_EVIDENCE_REQUIRED", "内部验证必须记录销售或投放依据", "补充可审计的验证说明")
	}
	v := domain.BenchmarkContent{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: in.ProjectID, Title: strings.TrimSpace(in.Title), Platform: defaultString(in.Platform, "douyin"), OriginalURL: in.OriginalURL, RightsMode: defaultString(in.RightsMode, "analysis_only"), ValidationLevel: level, ValidationNote: in.ValidationNote, CreatedAt: s.now().UTC()}
	if err := s.store.CreateBenchmark(ctx, v); err != nil {
		return v, err
	}
	s.audit(ctx, actor, in.ProjectID, "benchmark.created", "benchmark_content", v.ID, requestID, map[string]any{"validation_level": v.ValidationLevel, "rights_mode": v.RightsMode})
	return v, nil
}

func (s *Service) Benchmarks(ctx context.Context, actor Actor, projectID string) ([]domain.BenchmarkContent, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.Benchmarks(ctx, actor.TenantID, projectID)
}

type CreateFrameworkInput struct {
	BenchmarkID    string   `json:"benchmark_id"`
	Name           string   `json:"name"`
	VisualSequence []string `json:"visual_sequence"`
	CopySequence   []string `json:"copy_sequence"`
}

func (s *Service) CreateFramework(ctx context.Context, actor Actor, in CreateFrameworkInput, requestID string) (domain.ContentFramework, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return domain.ContentFramework{}, err
	}
	benchmark, err := s.store.Benchmark(ctx, actor.TenantID, in.BenchmarkID)
	if err != nil {
		return domain.ContentFramework{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, benchmark.ProjectID); err != nil {
		return domain.ContentFramework{}, err
	}
	if strings.TrimSpace(in.Name) == "" || len(in.VisualSequence) == 0 || len(in.CopySequence) == 0 {
		return domain.ContentFramework{}, domain.Invalid("FRAMEWORK_FIELDS_REQUIRED", "框架名称、画面序列和文案序列必填")
	}
	v := domain.ContentFramework{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: benchmark.ProjectID, BenchmarkID: benchmark.ID, Name: in.Name, VisualSequence: in.VisualSequence, CopySequence: in.CopySequence, Status: "approved", CreatedAt: s.now().UTC()}
	if err := s.store.CreateFramework(ctx, v); err != nil {
		return v, err
	}
	s.audit(ctx, actor, v.ProjectID, "framework.created", "content_framework", v.ID, requestID, map[string]any{"benchmark_id": v.BenchmarkID})
	return v, nil
}

func (s *Service) Frameworks(ctx context.Context, actor Actor, projectID string) ([]domain.ContentFramework, error) {
	return s.store.Frameworks(ctx, actor.TenantID, projectID)
}

type CreateShotPatternInput struct {
	FrameworkID  string   `json:"framework_id"`
	Role         string   `json:"role"`
	Purpose      string   `json:"purpose"`
	Subject      string   `json:"subject"`
	Action       string   `json:"action"`
	ProofType    string   `json:"proof_type"`
	FailureModes []string `json:"failure_modes"`
}

func (s *Service) CreateShotPattern(ctx context.Context, actor Actor, in CreateShotPatternInput, requestID string) (domain.ShotPattern, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return domain.ShotPattern{}, err
	}
	framework, err := s.store.Framework(ctx, actor.TenantID, in.FrameworkID)
	if err != nil {
		return domain.ShotPattern{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, framework.ProjectID); err != nil {
		return domain.ShotPattern{}, err
	}
	allowedRole := map[string]bool{"hook": true, "pain": true, "product_intro": true, "usage": true, "proof": true, "cta": true}
	if !allowedRole[in.Role] || strings.TrimSpace(in.Purpose) == "" || strings.TrimSpace(in.Action) == "" {
		return domain.ShotPattern{}, domain.Invalid("SHOT_PATTERN_INVALID", "镜头模式需要有效功能、目的和动作")
	}
	v := domain.ShotPattern{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: framework.ProjectID, FrameworkID: framework.ID, Role: in.Role, Purpose: in.Purpose, Subject: in.Subject, Action: in.Action, ProofType: in.ProofType, FailureModes: in.FailureModes, CreatedAt: s.now().UTC()}
	if err := s.store.CreateShotPattern(ctx, v); err != nil {
		return v, err
	}
	s.audit(ctx, actor, v.ProjectID, "shot_pattern.created", "shot_pattern", v.ID, requestID, map[string]any{"framework_id": v.FrameworkID, "role": v.Role})
	return v, nil
}

func (s *Service) ShotPatterns(ctx context.Context, actor Actor, projectID string) ([]domain.ShotPattern, error) {
	return s.store.ShotPatterns(ctx, actor.TenantID, projectID)
}

type CreateSellingPointInput struct {
	ProjectID    string   `json:"project_id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Priority     int      `json:"priority"`
	KnowledgeIDs []string `json:"knowledge_ids"`
}

func (s *Service) CreateSellingPoint(ctx context.Context, actor Actor, in CreateSellingPointInput, requestID string) (domain.SellingPoint, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return domain.SellingPoint{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, in.ProjectID); err != nil {
		return domain.SellingPoint{}, err
	}
	if strings.TrimSpace(in.Title) == "" || len(in.KnowledgeIDs) == 0 {
		return domain.SellingPoint{}, domain.Invalid("SELLING_POINT_INVALID", "卖点标题和知识依据必填")
	}
	for _, id := range in.KnowledgeIDs {
		item, err := s.store.KnowledgeItem(ctx, actor.TenantID, id)
		if err != nil || item.ProjectID != in.ProjectID || item.Status != "approved" {
			return domain.SellingPoint{}, domain.Policy("SELLING_POINT_KNOWLEDGE_BLOCKED", "卖点引用了未批准知识", "先完成知识审核")
		}
	}
	v := domain.SellingPoint{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: in.ProjectID, Title: in.Title, Description: in.Description, Priority: in.Priority, KnowledgeIDs: in.KnowledgeIDs, Status: "approved", CreatedAt: s.now().UTC()}
	if err := s.store.CreateSellingPoint(ctx, v); err != nil {
		return v, err
	}
	s.audit(ctx, actor, v.ProjectID, "selling_point.created", "selling_point", v.ID, requestID, map[string]any{"priority": v.Priority})
	return v, nil
}

func (s *Service) SellingPoints(ctx context.Context, actor Actor, projectID string) ([]domain.SellingPoint, error) {
	return s.store.SellingPoints(ctx, actor.TenantID, projectID)
}

type CreateVisualizationPlanInput struct {
	SellingPointID       string   `json:"selling_point_id"`
	Title                string   `json:"title"`
	ProofType            string   `json:"proof_type"`
	ShotPatternID        string   `json:"shot_pattern_id"`
	Subjects             []string `json:"subjects"`
	Setting              string   `json:"setting"`
	Props                []string `json:"props"`
	Implementation       string   `json:"implementation"`
	ProductTruthStrategy string   `json:"product_truth_strategy"`
	Risks                []string `json:"risks"`
	PlanB                string   `json:"plan_b"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
}

func (s *Service) CreateVisualizationPlan(ctx context.Context, actor Actor, in CreateVisualizationPlanInput, requestID string) (domain.VisualizationPlan, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return domain.VisualizationPlan{}, err
	}
	point, err := s.store.SellingPoint(ctx, actor.TenantID, in.SellingPointID)
	if err != nil {
		return domain.VisualizationPlan{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, point.ProjectID); err != nil {
		return domain.VisualizationPlan{}, err
	}
	if in.Title == "" || in.ProofType == "" || in.Implementation == "" || in.ProductTruthStrategy == "" || len(in.AcceptanceCriteria) == 0 {
		return domain.VisualizationPlan{}, domain.Invalid("VISUALIZATION_PLAN_INVALID", "可视化方案缺少证据类型、实现方式、真实性策略或验收条件")
	}
	if in.ShotPatternID != "" {
		patterns, _ := s.store.ShotPatterns(ctx, actor.TenantID, point.ProjectID)
		found := false
		for _, pattern := range patterns {
			found = found || pattern.ID == in.ShotPatternID
		}
		if !found {
			return domain.VisualizationPlan{}, domain.NotFound("镜头模式")
		}
	}
	v := domain.VisualizationPlan{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: point.ProjectID, SellingPointID: point.ID, Title: in.Title, ProofType: in.ProofType, ShotPatternID: in.ShotPatternID, Subjects: in.Subjects, Setting: in.Setting, Props: in.Props, Implementation: in.Implementation, ProductTruthStrategy: in.ProductTruthStrategy, Risks: in.Risks, PlanB: in.PlanB, AcceptanceCriteria: in.AcceptanceCriteria, Status: "needs_review", CreatedAt: s.now().UTC()}
	if err := s.store.CreateVisualizationPlan(ctx, v); err != nil {
		return v, err
	}
	s.audit(ctx, actor, v.ProjectID, "visualization_plan.created", "visualization_plan", v.ID, requestID, map[string]any{"selling_point_id": v.SellingPointID})
	return v, nil
}

func (s *Service) ReviewVisualizationPlan(ctx context.Context, actor Actor, id, decision, requestID string) (domain.VisualizationPlan, error) {
	if err := requireRole(actor, "tenant_admin", "reviewer"); err != nil {
		return domain.VisualizationPlan{}, err
	}
	v, err := s.store.VisualizationPlan(ctx, actor.TenantID, id)
	if err != nil {
		return v, err
	}
	if _, err := s.projectForWrite(ctx, actor, v.ProjectID); err != nil {
		return v, err
	}
	previous := v.Status
	switch decision {
	case "approve":
		v.Status = "approved"
	case "reject":
		v.Status = "rejected"
	case "return":
		v.Status = "needs_review"
	default:
		return v, domain.Invalid("DECISION_INVALID", "审核决策无效")
	}
	if err := s.store.SaveVisualizationPlan(ctx, v); err != nil {
		return v, err
	}
	s.audit(ctx, actor, v.ProjectID, "visualization_plan.reviewed", "visualization_plan", v.ID, requestID, map[string]any{"from": previous, "to": v.Status})
	return v, nil
}

func (s *Service) VisualizationPlans(ctx context.Context, actor Actor, projectID string) ([]domain.VisualizationPlan, error) {
	return s.store.VisualizationPlans(ctx, actor.TenantID, projectID)
}
