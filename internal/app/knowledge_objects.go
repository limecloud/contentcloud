package app

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type CreateKnowledgeObjectInput struct {
	ProjectID       string         `json:"project_id"`
	ID              string         `json:"id"`
	ObjectType      string         `json:"object_type"`
	Layer           string         `json:"layer"`
	Status          string         `json:"status"`
	Title           string         `json:"title"`
	Statement       string         `json:"statement"`
	Payload         map[string]any `json:"payload"`
	Dimensions      []string       `json:"dimensions"`
	AllowedChannels []string       `json:"allowed_channels"`
	EvidenceRefs    []string       `json:"evidence_refs"`
	RelationRefs    []string       `json:"relation_refs"`
	RightsRefs      []string       `json:"rights_refs"`
	ConflictRefs    []string       `json:"conflict_refs"`
	DecisionRef     string         `json:"decision_ref"`
	NextAction      string         `json:"next_action"`
	Impact          string         `json:"impact"`
	ValidFrom       *time.Time     `json:"valid_from"`
	ValidUntil      *time.Time     `json:"valid_until"`
	ExpiresAt       *time.Time     `json:"expires_at"`
}

type CreateKnowledgePackInput struct {
	ProjectID   string                          `json:"project_id"`
	ID          string                          `json:"id"`
	Name        string                          `json:"name"`
	Purpose     string                          `json:"purpose"`
	ObjectRefs  []domain.KnowledgePackObjectRef `json:"object_refs"`
	QueryPolicy domain.KnowledgeQueryPolicy     `json:"query_policy"`
}

type ReviewKnowledgeObjectInput struct {
	ExpectedVersion int    `json:"expected_version"`
	ExpectedDigest  string `json:"expected_digest"`
	Decision        string `json:"decision"`
	Reason          string `json:"reason"`
}

type QueryKnowledgeInput struct {
	ProjectID   string    `json:"project_id"`
	SnapshotID  string    `json:"snapshot_id"`
	PackID      string    `json:"pack_id"`
	Channel     string    `json:"channel"`
	Layers      []string  `json:"layers"`
	ObjectTypes []string  `json:"object_types"`
	ObjectIDs   []string  `json:"object_ids"`
	At          time.Time `json:"at"`
}

func (s *Service) CreateKnowledgeObject(ctx context.Context, actor Actor, in CreateKnowledgeObjectInput, requestID string) (domain.KnowledgeObject, error) {
	if actor.Type != "device" && actor.Type != "worker" {
		if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer"); err != nil {
			return domain.KnowledgeObject{}, err
		}
	}
	if _, err := s.projectForWrite(ctx, actor, in.ProjectID); err != nil {
		return domain.KnowledgeObject{}, err
	}
	if err := s.validateKnowledgeEvidenceRefs(ctx, actor.TenantID, in.ProjectID, in.EvidenceRefs, false); err != nil {
		return domain.KnowledgeObject{}, err
	}
	now := s.now().UTC()
	objectID := strings.TrimSpace(in.ID)
	if objectID == "" {
		objectID = "knowledge:" + domain.NewID()
	}
	objectType := strings.TrimSpace(in.ObjectType)
	layer := strings.TrimSpace(in.Layer)
	version := 1
	if existing, err := s.store.KnowledgeObject(ctx, actor.TenantID, objectID, 0); err == nil {
		if existing.ProjectID != in.ProjectID {
			return domain.KnowledgeObject{}, domain.Conflict("KNOWLEDGE_OBJECT_ID_SCOPE_CONFLICT", "知识对象 ID 已被其他 Project 使用")
		}
		version = existing.Version + 1
	} else if !isNotFound(err) {
		return domain.KnowledgeObject{}, err
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "candidate"
		if objectType == "KnowledgeGap" || objectType == "ConflictRecord" {
			status = "open"
		}
	}
	creationStatusAllowed := status == "candidate" || status == "needs_review"
	if objectType == "KnowledgeGap" {
		creationStatusAllowed = creationStatusAllowed || status == "open" || status == "source_missing" || status == "collecting"
	} else if objectType == "ConflictRecord" {
		creationStatusAllowed = creationStatusAllowed || status == "open"
	}
	if !creationStatusAllowed {
		return domain.KnowledgeObject{}, domain.Policy("KNOWLEDGE_OBJECT_STATUS_COMMAND_REQUIRED", "创建对象不能直接写入已验证或已批准状态", "先创建候选，再使用知识决策命令")
	}
	payload := in.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	object := domain.KnowledgeObject{
		ID: objectID, TenantID: actor.TenantID, ProjectID: in.ProjectID, ObjectType: objectType, Layer: layer, Version: version, Status: status,
		Title: strings.TrimSpace(in.Title), Statement: strings.TrimSpace(in.Statement), Payload: payload, Dimensions: append([]string(nil), in.Dimensions...), AllowedChannels: append([]string(nil), in.AllowedChannels...), EvidenceRefs: append([]string(nil), in.EvidenceRefs...), RelationRefs: append([]string(nil), in.RelationRefs...), RightsRefs: append([]string(nil), in.RightsRefs...), ConflictRefs: append([]string(nil), in.ConflictRefs...), DecisionRef: strings.TrimSpace(in.DecisionRef), NextAction: strings.TrimSpace(in.NextAction), Impact: strings.TrimSpace(in.Impact), ValidFrom: in.ValidFrom, ValidUntil: in.ValidUntil, ExpiresAt: in.ExpiresAt, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
	}
	if err := object.Validate(); err != nil {
		return domain.KnowledgeObject{}, err
	}
	digest, err := object.ContentDigest()
	if err != nil {
		return domain.KnowledgeObject{}, err
	}
	object.Digest = digest
	if err := s.store.CreateKnowledgeObject(ctx, object); err != nil {
		return domain.KnowledgeObject{}, err
	}
	s.audit(ctx, actor, object.ProjectID, "knowledge_object.created", "knowledge_object", object.ID, requestID, map[string]any{"version": object.Version, "object_type": object.ObjectType, "layer": object.Layer})
	return object, nil
}

func (s *Service) validateKnowledgeEvidenceRefs(ctx context.Context, tenantID, projectID string, evidenceRefs []string, requireAccepted bool) error {
	seen := make(map[string]struct{}, len(evidenceRefs))
	for _, rawID := range evidenceRefs {
		evidenceID := strings.TrimSpace(rawID)
		if evidenceID == "" {
			return domain.Invalid("KNOWLEDGE_EVIDENCE_REF_INVALID", "知识对象的 Evidence 引用不能为空")
		}
		if _, ok := seen[evidenceID]; ok {
			return domain.Invalid("KNOWLEDGE_EVIDENCE_REF_DUPLICATE", "知识对象不能重复引用同一个 Evidence")
		}
		seen[evidenceID] = struct{}{}
		span, err := s.store.EvidenceSpan(ctx, tenantID, evidenceID)
		if err != nil {
			if domain.IsNotFound(err) {
				return domain.Policy("KNOWLEDGE_EVIDENCE_NOT_FOUND", "知识对象引用的 Evidence 不存在或无权访问", "选择当前项目内已验收的 Evidence")
			}
			return err
		}
		if span.TenantID != tenantID || span.ProjectID != projectID {
			return domain.Policy("KNOWLEDGE_EVIDENCE_PROJECT_MISMATCH", "知识对象引用的 Evidence 不属于当前项目", "选择当前项目内已验收的 Evidence")
		}
		if requireAccepted && span.ReviewStatus != "accepted" {
			return domain.Policy("KNOWLEDGE_EVIDENCE_NOT_ACCEPTED", "知识对象只能引用已验收的 Evidence", "先完成 Evidence 复核，再提交知识决策")
		}
	}
	return nil
}

func (s *Service) ReviewKnowledgeObject(ctx context.Context, actor Actor, objectID string, in ReviewKnowledgeObjectInput, requestID string) (domain.KnowledgeObject, domain.KnowledgeDecision, error) {
	if err := requireRole(actor, "tenant_admin", "reviewer"); err != nil {
		return domain.KnowledgeObject{}, domain.KnowledgeDecision{}, err
	}
	current, err := s.store.KnowledgeObject(ctx, actor.TenantID, objectID, 0)
	if err != nil {
		return current, domain.KnowledgeDecision{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, current.ProjectID); err != nil {
		return current, domain.KnowledgeDecision{}, err
	}
	if current.Version != in.ExpectedVersion || current.Digest != strings.TrimSpace(in.ExpectedDigest) {
		return current, domain.KnowledgeDecision{}, domain.Conflict("KNOWLEDGE_OBJECT_VERSION_CONFLICT", "知识对象版本或 digest 已变化")
	}
	decisionValue := strings.TrimSpace(in.Decision)
	if decisionValue != "approve" && decisionValue != "reject" {
		return current, domain.KnowledgeDecision{}, domain.Invalid("KNOWLEDGE_DECISION_INVALID", "知识决策只允许 approve 或 reject")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return current, domain.KnowledgeDecision{}, domain.Invalid("KNOWLEDGE_DECISION_REASON_REQUIRED", "知识决策必须填写原因")
	}
	if current.Status != "candidate" && current.Status != "needs_review" {
		return current, domain.KnowledgeDecision{}, domain.Conflict("KNOWLEDGE_OBJECT_STATE_INVALID", "当前知识对象状态不能审核")
	}
	if current.ObjectType == "KnowledgeGap" || current.ObjectType == "ConflictRecord" {
		return current, domain.KnowledgeDecision{}, domain.Invalid("KNOWLEDGE_OBJECT_DECISION_UNSUPPORTED", "知识缺口和冲突必须使用各自的解决命令")
	}
	if decisionValue == "approve" && len(current.EvidenceRefs) == 0 {
		return current, domain.KnowledgeDecision{}, domain.Policy("KNOWLEDGE_EVIDENCE_REQUIRED", "没有 Evidence 的知识对象不能批准", "先补充可定位 Evidence")
	}
	if decisionValue == "approve" {
		if err := s.validateKnowledgeEvidenceRefs(ctx, current.TenantID, current.ProjectID, current.EvidenceRefs, true); err != nil {
			return current, domain.KnowledgeDecision{}, err
		}
	}
	now := s.now().UTC()
	decision := domain.KnowledgeDecision{ID: domain.NewID(), TenantID: current.TenantID, ProjectID: current.ProjectID, ObjectID: current.ID, PreviousVersion: current.Version, ResultVersion: current.Version + 1, SubjectDigest: current.Digest, Decision: decisionValue, Reason: strings.TrimSpace(in.Reason), ActorID: actor.UserID, CreatedAt: now}
	result := current
	result.Version = decision.ResultVersion
	result.DecisionRef = decision.ID
	result.CreatedBy = actor.UserID
	result.CreatedAt = now
	result.UpdatedAt = now
	result.Digest = ""
	if decisionValue == "reject" {
		result.Status = "rejected"
	} else {
		result.Status = approvedKnowledgeObjectStatus(result.ObjectType)
	}
	digest, err := result.ContentDigest()
	if err != nil {
		return current, domain.KnowledgeDecision{}, err
	}
	result.Digest = digest
	if err := s.store.CreateKnowledgeObjectDecision(ctx, result, decision); err != nil {
		return current, domain.KnowledgeDecision{}, err
	}
	s.audit(ctx, actor, result.ProjectID, "knowledge_object.decided", "knowledge_object", result.ID, requestID, map[string]any{"decision_id": decision.ID, "decision": decision.Decision, "previous_version": decision.PreviousVersion, "result_version": decision.ResultVersion})
	return result, decision, nil
}

func (s *Service) KnowledgeDecisions(ctx context.Context, actor Actor, objectID string) ([]domain.KnowledgeDecision, error) {
	object, err := s.store.KnowledgeObject(ctx, actor.TenantID, objectID, 0)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Project(ctx, actor.TenantID, object.ProjectID); err != nil {
		return nil, err
	}
	return s.store.KnowledgeDecisions(ctx, actor.TenantID, objectID)
}

func (s *Service) KnowledgeObject(ctx context.Context, actor Actor, objectID string, version int) (domain.KnowledgeObject, error) {
	object, err := s.store.KnowledgeObject(ctx, actor.TenantID, objectID, version)
	if err != nil {
		return object, err
	}
	if _, err := s.store.Project(ctx, actor.TenantID, object.ProjectID); err != nil {
		return object, err
	}
	return object, nil
}

func (s *Service) KnowledgeObjects(ctx context.Context, actor Actor, projectID string) ([]domain.KnowledgeObject, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.KnowledgeObjects(ctx, actor.TenantID, projectID)
}

func (s *Service) CreateKnowledgePack(ctx context.Context, actor Actor, in CreateKnowledgePackInput, requestID string) (domain.KnowledgePack, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "reviewer"); err != nil {
		return domain.KnowledgePack{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, in.ProjectID); err != nil {
		return domain.KnowledgePack{}, err
	}
	now := s.now().UTC()
	policy := in.QueryPolicy
	if len(policy.EligibleStatuses) == 0 {
		policy.EligibleStatuses = append([]string(nil), domain.KnowledgeEligibleStatuses...)
	}
	policy.RequireEvidence = true
	policy.BlockOnConflict = true
	policy.BlockOnRights = true
	pack := domain.KnowledgePack{ID: strings.TrimSpace(in.ID), TenantID: actor.TenantID, ProjectID: in.ProjectID, Name: strings.TrimSpace(in.Name), Purpose: strings.TrimSpace(in.Purpose), Version: 1, Status: "draft", ObjectRefs: append([]domain.KnowledgePackObjectRef(nil), in.ObjectRefs...), QueryPolicy: policy, CreatedBy: actor.UserID, CreatedAt: now}
	if pack.ID == "" {
		pack.ID = "pack:" + domain.NewID()
	}
	if pack.Name == "" {
		pack.Name = "未命名知识包"
	}
	if pack.Purpose == "" {
		pack.Purpose = "content_production"
	}
	if err := pack.Validate(); err != nil {
		return domain.KnowledgePack{}, err
	}
	digest, err := pack.ContentDigest()
	if err != nil {
		return domain.KnowledgePack{}, err
	}
	pack.Digest = digest
	if err := s.store.CreateKnowledgePack(ctx, pack); err != nil {
		return domain.KnowledgePack{}, err
	}
	s.audit(ctx, actor, pack.ProjectID, "knowledge_pack.created", "knowledge_pack", pack.ID, requestID, map[string]any{"version": pack.Version, "object_count": len(pack.ObjectRefs)})
	return pack, nil
}

func (s *Service) PublishKnowledgePack(ctx context.Context, actor Actor, packID, requestID string) (domain.KnowledgePack, domain.KnowledgeSnapshot, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "reviewer"); err != nil {
		return domain.KnowledgePack{}, domain.KnowledgeSnapshot{}, err
	}
	pack, err := s.store.KnowledgePack(ctx, actor.TenantID, packID)
	if err != nil {
		return pack, domain.KnowledgeSnapshot{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, pack.ProjectID); err != nil {
		return pack, domain.KnowledgeSnapshot{}, err
	}
	if pack.Status != "draft" {
		return pack, domain.KnowledgeSnapshot{}, domain.Conflict("KNOWLEDGE_PACK_STATE_INVALID", "只有 draft 知识包可以发布")
	}
	objects := make([]domain.KnowledgeObject, 0, len(pack.ObjectRefs))
	for _, ref := range pack.ObjectRefs {
		object, err := s.store.KnowledgeObject(ctx, actor.TenantID, ref.ObjectID, ref.Version)
		if err != nil {
			return pack, domain.KnowledgeSnapshot{}, err
		}
		if object.ProjectID != pack.ProjectID {
			return pack, domain.KnowledgeSnapshot{}, domain.Conflict("KNOWLEDGE_OBJECT_SCOPE_MISMATCH", "知识包引用了其他 Project 的对象")
		}
		if ref.Version > 0 && object.Version != ref.Version {
			return pack, domain.KnowledgeSnapshot{}, domain.Conflict("KNOWLEDGE_OBJECT_VERSION_MISSING", "知识包引用的对象版本不存在")
		}
		objects = append(objects, object)
	}
	now := s.now().UTC()
	pack.Status = "published"
	pack.PublishedBy = actor.UserID
	pack.PublishedAt = &now
	snapshot, err := domain.BuildKnowledgeSnapshot(pack, objects, now)
	if err != nil {
		return pack, domain.KnowledgeSnapshot{}, err
	}
	if err := s.store.SaveKnowledgePack(ctx, pack); err != nil {
		return pack, domain.KnowledgeSnapshot{}, err
	}
	if err := s.store.CreateKnowledgeSnapshot(ctx, snapshot); err != nil {
		return pack, domain.KnowledgeSnapshot{}, err
	}
	s.audit(ctx, actor, pack.ProjectID, "knowledge_pack.published", "knowledge_pack", pack.ID, requestID, map[string]any{"snapshot_id": snapshot.ID, "pack_digest": pack.Digest})
	return pack, snapshot, nil
}

func (s *Service) KnowledgePacks(ctx context.Context, actor Actor, projectID string) ([]domain.KnowledgePack, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.KnowledgePacks(ctx, actor.TenantID, projectID)
}

func (s *Service) KnowledgeSnapshot(ctx context.Context, actor Actor, snapshotID string) (domain.KnowledgeSnapshot, error) {
	snapshot, err := s.store.KnowledgeSnapshot(ctx, actor.TenantID, snapshotID)
	if err != nil {
		return snapshot, err
	}
	if _, err := s.store.Project(ctx, actor.TenantID, snapshot.ProjectID); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (s *Service) KnowledgeSnapshots(ctx context.Context, actor Actor, projectID, packID string) ([]domain.KnowledgeSnapshot, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.KnowledgeSnapshots(ctx, actor.TenantID, projectID, packID)
}

func (s *Service) QueryKnowledge(ctx context.Context, actor Actor, in QueryKnowledgeInput) (domain.KnowledgeQueryResult, error) {
	if strings.TrimSpace(in.SnapshotID) == "" && strings.TrimSpace(in.PackID) == "" {
		return domain.KnowledgeQueryResult{}, domain.Invalid("KNOWLEDGE_QUERY_TARGET_REQUIRED", "查询必须指定 snapshot_id 或 pack_id")
	}
	var snapshot domain.KnowledgeSnapshot
	var pack domain.KnowledgePack
	var err error
	if strings.TrimSpace(in.SnapshotID) != "" {
		snapshot, err = s.store.KnowledgeSnapshot(ctx, actor.TenantID, in.SnapshotID)
	} else {
		pack, err = s.store.KnowledgePack(ctx, actor.TenantID, in.PackID)
		if err == nil {
			var snapshots []domain.KnowledgeSnapshot
			projectID := in.ProjectID
			if projectID == "" {
				projectID = pack.ProjectID
			}
			snapshots, err = s.store.KnowledgeSnapshots(ctx, actor.TenantID, projectID, in.PackID)
			if err == nil && len(snapshots) == 0 {
				err = domain.NotFound("知识快照")
			} else if err == nil {
				snapshot = snapshots[0]
			}
		}
	}
	if err != nil {
		return domain.KnowledgeQueryResult{}, err
	}
	if _, err := s.store.Project(ctx, actor.TenantID, snapshot.ProjectID); err != nil {
		return domain.KnowledgeQueryResult{}, err
	}
	if in.ProjectID != "" && snapshot.ProjectID != in.ProjectID {
		return domain.KnowledgeQueryResult{}, domain.Conflict("KNOWLEDGE_QUERY_SCOPE_MISMATCH", "查询 Project 与快照不一致")
	}
	if pack.ID == "" {
		pack, err = s.store.KnowledgePack(ctx, actor.TenantID, snapshot.PackID)
		if err != nil {
			return domain.KnowledgeQueryResult{}, err
		}
	}
	if err := pack.Validate(); err != nil {
		return domain.KnowledgeQueryResult{}, err
	}
	return domain.EvaluateKnowledgeSnapshot(snapshot, pack.QueryPolicy, domain.KnowledgeQuery{SnapshotID: snapshot.ID, Channel: in.Channel, Layers: in.Layers, ObjectTypes: in.ObjectTypes, ObjectIDs: in.ObjectIDs, At: in.At})
}

func approvedKnowledgeObjectStatus(objectType string) string {
	switch objectType {
	case "FactAssertion", "Insight":
		return "verified"
	case "Claim", "Asset":
		return "approved"
	case "RightsRecord":
		return "valid"
	default:
		return "active"
	}
}
