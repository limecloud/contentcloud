package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	"image/png":        true,
	"image/jpeg":       true,
	"video/mp4":        true,
	"video/quicktime":  true,
	"video/webm":       true,
	"audio/mpeg":       true,
	"audio/wav":        true,
	"audio/x-wav":      true,
	"audio/mp4":        true,
	"text/csv":         true,
	"text/html":        true,
	"text/markdown":    true,
	"text/plain":       true,
	"application/json": true,
}

var sourceExtensionMIME = map[string]string{
	".pdf": "application/pdf", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".mp4": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm",
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".m4a": "audio/mp4",
	".csv": "text/csv", ".html": "text/html", ".htm": "text/html", ".json": "application/json", ".md": "text/markdown", ".txt": "text/plain",
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
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return domain.SourceRevision{}, err
	}
	return s.createSource(ctx, actor, in, requestID)
}

func (s *Service) createSource(ctx context.Context, actor Actor, in CreateSourceInput, requestID string) (domain.SourceRevision, error) {
	if _, err := s.projectForWrite(ctx, actor, in.ProjectID); err != nil {
		return domain.SourceRevision{}, err
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.FileName) == "" {
		return domain.SourceRevision{}, domain.Invalid("SOURCE_FIELDS_REQUIRED", "来源名称和文件名必填")
	}
	if !allowedSourceMIME[in.MIME] {
		return domain.SourceRevision{}, domain.E("validation", "content_type", "SOURCE_MIME_BLOCKED", "暂不支持该文档、图片、音视频或表格类型", 7)
	}
	if in.ByteSize <= 0 || in.ByteSize > 100*1024*1024 {
		return domain.SourceRevision{}, domain.Invalid("SOURCE_SIZE_INVALID", "单个文件大小必须在 1 字节至 100 MB 之间")
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
		return domain.SourceRevision{}, domain.Invalid("SOURCE_REVISION_INVALID", "来源版本需要有效的文件名和受支持的媒体类型")
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
		return domain.SourceRevision{}, domain.Invalid("SOURCE_SIZE_INVALID", "单个文件大小必须在 1 字节至 100 MB 之间")
	}
	extension := strings.ToLower(filepath.Ext(fileName))
	if expected, ok := sourceExtensionMIME[extension]; !ok || !sourceMIMEMatches(expected, declaredMIME) {
		return domain.SourceRevision{}, domain.E("validation", "content_type", "SOURCE_MIME_MISMATCH", "文件扩展名与声明类型不一致", 7)
	}
	sum := sha256.Sum256(data)
	revision, err := s.CreateSourceRevision(ctx, actor, CreateSourceRevisionInput{SourceID: sourceID, FileName: fileName, MIME: declaredMIME, SHA256: hex.EncodeToString(sum[:]), ByteSize: int64(len(data))}, requestID)
	if err != nil {
		return revision, err
	}
	revision.ProcessingStatus = "uploading"
	if err := s.store.SaveSourceRevision(ctx, revision); err != nil {
		return revision, err
	}
	if err := s.blobs.Put(ctx, revision.ObjectKey, data); err != nil {
		revision.ProcessingStatus = "failed"
		revision.ErrorCode = "OBJECT_WRITE_FAILED"
		_ = s.store.SaveSourceRevision(ctx, revision)
		return revision, err
	}
	revision.ProcessingStatus = "pending"
	if err := s.store.SaveSourceRevision(ctx, revision); err != nil {
		return revision, err
	}
	return revision, nil
}

func (s *Service) UploadSource(ctx context.Context, actor Actor, projectID, name, sourceType, fileName, declaredMIME string, data []byte, requestID string) (domain.SourceRevision, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist"); err != nil {
		return domain.SourceRevision{}, err
	}
	return s.uploadSource(ctx, actor, projectID, name, sourceType, fileName, declaredMIME, data, requestID)
}

func (s *Service) uploadSource(ctx context.Context, actor Actor, projectID, name, sourceType, fileName, declaredMIME string, data []byte, requestID string) (domain.SourceRevision, error) {
	if len(data) == 0 || len(data) > 100*1024*1024 {
		return domain.SourceRevision{}, domain.Invalid("SOURCE_SIZE_INVALID", "单个文件大小必须在 1 字节至 100 MB 之间")
	}
	extension := strings.ToLower(filepath.Ext(fileName))
	expected, ok := sourceExtensionMIME[extension]
	if !ok || !sourceMIMEMatches(expected, declaredMIME) {
		return domain.SourceRevision{}, domain.E("validation", "content_type", "SOURCE_MIME_MISMATCH", "文件扩展名与声明类型不一致", 7)
	}
	sum := sha256.Sum256(data)
	revision, err := s.createSource(ctx, actor, CreateSourceInput{ProjectID: projectID, Name: name, SourceType: sourceType, FileName: filepath.Base(fileName), MIME: expected, SHA256: hex.EncodeToString(sum[:]), ByteSize: int64(len(data))}, requestID)
	if err != nil {
		return revision, err
	}
	revision.ProcessingStatus = "uploading"
	if err := s.store.SaveSourceRevision(ctx, revision); err != nil {
		return revision, err
	}
	if err := s.blobs.Put(ctx, revision.ObjectKey, data); err != nil {
		revision.ProcessingStatus = "failed"
		revision.ErrorCode = "OBJECT_WRITE_FAILED"
		_ = s.store.SaveSourceRevision(ctx, revision)
		return revision, err
	}
	revision.ProcessingStatus = "pending"
	if err := s.store.SaveSourceRevision(ctx, revision); err != nil {
		return revision, err
	}
	return revision, nil
}

func sourceMIMEMatches(expected, declared string) bool {
	declared = strings.ToLower(strings.TrimSpace(strings.SplitN(declared, ";", 2)[0]))
	if expected == declared {
		return true
	}
	return expected == "audio/wav" && declared == "audio/x-wav"
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
		return revision, domain.Policy("ROLE_DENIED", "当前角色不能完成来源解析", "等待后台处理程序或审核员处理")
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
		return span, domain.Invalid("EVIDENCE_DECISION_INVALID", "证据复核只允许“接受（accept）”或“拒绝（reject）”")
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
		if impact.ObjectType != "knowledge_object" {
			continue
		}
		object, err := s.store.KnowledgeObject(ctx, actor.TenantID, impact.ObjectID, 0)
		if err != nil || !containsString(domain.KnowledgeEligibleStatuses, object.Status) {
			continue
		}
		payload := map[string]any{}
		for key, value := range object.Payload {
			payload[key] = value
		}
		_, err = s.CreateKnowledgeObject(ctx, actor, CreateKnowledgeObjectInput{
			ProjectID:       object.ProjectID,
			ID:              object.ID,
			ObjectType:      object.ObjectType,
			Layer:           object.Layer,
			Status:          "needs_review",
			Title:           object.Title,
			Statement:       object.Statement,
			Payload:         payload,
			Dimensions:      object.Dimensions,
			AllowedChannels: object.AllowedChannels,
			EvidenceRefs:    object.EvidenceRefs,
			RelationRefs:    object.RelationRefs,
			RightsRefs:      object.RightsRefs,
			ConflictRefs:    object.ConflictRefs,
			ValidFrom:       object.ValidFrom,
			ValidUntil:      object.ValidUntil,
			ExpiresAt:       object.ExpiresAt,
		}, requestID)
		if err != nil {
			return impacts, err
		}
		changedKnowledge[object.ID] = true
	}
	if len(changedKnowledge) == 0 {
		return impacts, nil
	}
	s.audit(ctx, actor, source.ProjectID, "source.impact_propagated", "source", source.ID, requestID, map[string]any{"knowledge_count": len(changedKnowledge)})
	return s.collectSourceImpact(ctx, actor.TenantID, source.ProjectID, revisions), nil
}

func (s *Service) collectSourceImpact(ctx context.Context, tenantID, projectID string, revisions []domain.SourceRevision) []ImpactItem {
	revisionIDs := map[string]bool{}
	for _, revision := range revisions {
		revisionIDs[revision.ID] = true
	}
	impacts := []ImpactItem{}
	knowledge, _ := s.store.KnowledgeObjects(ctx, tenantID, projectID)
	latest := map[string]domain.KnowledgeObject{}
	for _, object := range knowledge {
		if current, ok := latest[object.ID]; !ok || object.Version > current.Version {
			latest[object.ID] = object
		}
	}
	for _, object := range latest {
		for _, evidenceID := range object.EvidenceRefs {
			span, err := s.store.EvidenceSpan(ctx, tenantID, evidenceID)
			if err == nil && revisionIDs[span.RevisionID] {
				impacts = append(impacts, ImpactItem{ObjectType: "knowledge_object", ObjectID: object.ID, Reason: "引用了该来源版本的证据", CurrentStatus: object.Status, SuggestedAction: "复核新版本中的原文和值"})
				break
			}
		}
	}
	return impacts
}
