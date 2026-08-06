package memory

import (
	"context"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateSource(_ context.Context, source domain.Source, revision domain.SourceRevision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.revisions {
		if existing.TenantID == revision.TenantID && existing.ProjectID == revision.ProjectID && existing.SHA256 == revision.SHA256 {
			return domain.Conflict("SOURCE_DUPLICATE", "项目内已存在相同文件")
		}
	}
	s.sources[source.ID] = source
	s.revisions[revision.ID] = revision
	return nil
}

func (s *Store) CreateSourceRevision(_ context.Context, revision domain.SourceRevision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, ok := s.sources[revision.SourceID]
	if !ok || source.TenantID != revision.TenantID || source.ProjectID != revision.ProjectID {
		return domain.NotFound("来源")
	}
	for _, existing := range s.revisions {
		if existing.TenantID == revision.TenantID && existing.ProjectID == revision.ProjectID && existing.SHA256 == revision.SHA256 {
			return domain.Conflict("SOURCE_DUPLICATE", "项目内已存在相同文件")
		}
	}
	s.revisions[revision.ID] = revision
	source.Status = revision.ProcessingStatus
	source.LatestRevision = revision.ID
	source.RevisionCount++
	s.sources[source.ID] = source
	return nil
}

func (s *Store) Sources(_ context.Context, tenantID, projectID string) ([]domain.Source, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Source{}
	for _, v := range s.sources {
		if v.TenantID == tenantID && v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) Source(_ context.Context, tenantID, id string) (domain.Source, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.sources[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("来源")
	}
	return v, nil
}

func (s *Store) SourceRevisions(_ context.Context, tenantID, sourceID string) ([]domain.SourceRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	source, ok := s.sources[sourceID]
	if !ok || source.TenantID != tenantID {
		return nil, domain.NotFound("来源")
	}
	out := []domain.SourceRevision{}
	for _, v := range s.revisions {
		if v.TenantID == tenantID && v.SourceID == sourceID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) SourceRevision(_ context.Context, tenantID, id string) (domain.SourceRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.revisions[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("来源版本")
	}
	return v, nil
}

func (s *Store) SaveSourceRevision(_ context.Context, v domain.SourceRevision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.revisions[v.ID]
	if !ok || old.TenantID != v.TenantID {
		return domain.NotFound("来源版本")
	}
	s.revisions[v.ID] = v
	if source, exists := s.sources[v.SourceID]; exists {
		source.Status = v.ProcessingStatus
		sourcesRevisionCount := 0
		for _, revision := range s.revisions {
			if revision.SourceID == source.ID {
				sourcesRevisionCount++
			}
		}
		source.RevisionCount = sourcesRevisionCount
		source.LatestRevision = v.ID
		s.sources[source.ID] = source
	}
	return nil
}

func (s *Store) PendingSourceRevisions(_ context.Context, limit int) ([]domain.SourceRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.SourceRevision{}
	for _, v := range s.revisions {
		if v.ProcessingStatus == "pending" {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) ClaimSourceRevision(_ context.Context, tenantID, id string) (domain.SourceRevision, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.revisions[id]
	if !ok || v.TenantID != tenantID {
		return v, false, domain.NotFound("来源版本")
	}
	if v.ProcessingStatus != "pending" {
		return v, false, nil
	}
	v.ProcessingStatus = "processing"
	s.revisions[id] = v
	return v, true, nil
}

func (s *Store) CreateEvidence(_ context.Context, v domain.EvidenceSpan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidence[v.ID] = v
	return nil
}

func (s *Store) Evidence(_ context.Context, tenantID, revisionID string) ([]domain.EvidenceSpan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.EvidenceSpan{}
	for _, v := range s.evidence {
		if v.TenantID == tenantID && v.RevisionID == revisionID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) EvidenceSpan(_ context.Context, tenantID, id string) (domain.EvidenceSpan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.evidence[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("证据片段")
	}
	return v, nil
}

func (s *Store) SaveEvidence(_ context.Context, v domain.EvidenceSpan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.evidence[v.ID]
	if !ok || old.TenantID != v.TenantID {
		return domain.NotFound("证据片段")
	}
	s.evidence[v.ID] = v
	return nil
}

func (s *Store) CreateReviewCycle(_ context.Context, cycle domain.ReviewCycle) (domain.ReviewCycle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	maxCycle := 0
	for _, existing := range s.reviewCycles {
		if existing.TenantID == cycle.TenantID && existing.SubjectID == cycle.SubjectID && existing.CycleNumber > maxCycle {
			maxCycle = existing.CycleNumber
		}
	}
	cycle.CycleNumber = maxCycle + 1
	s.reviewCycles[cycle.ID] = cycle
	return cycle, nil
}

func (s *Store) ReviewCycles(_ context.Context, tenantID, subjectID string) ([]domain.ReviewCycle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ReviewCycle{}
	for _, cycle := range s.reviewCycles {
		if cycle.TenantID == tenantID && (subjectID == "" || cycle.SubjectID == subjectID) {
			result = append(result, cycle)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CycleNumber > result[j].CycleNumber })
	return result, nil
}

func (s *Store) SaveReviewCycle(_ context.Context, cycle domain.ReviewCycle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.reviewCycles[cycle.ID]; !ok || existing.TenantID != cycle.TenantID {
		return domain.NotFound("审核周期")
	}
	s.reviewCycles[cycle.ID] = cycle
	return nil
}

func (s *Store) CreateReviewComment(_ context.Context, v domain.ReviewComment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviewComments[v.ID] = v
	return nil
}

func (s *Store) ReviewComments(_ context.Context, tenantID, subjectID string) ([]domain.ReviewComment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.ReviewComment{}
	for _, v := range s.reviewComments {
		if v.TenantID == tenantID && (subjectID == "" || v.SubjectID == subjectID) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) ReviewComment(_ context.Context, tenantID, id string) (domain.ReviewComment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.reviewComments[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("审核批注")
	}
	return v, nil
}

func (s *Store) SaveReviewComment(_ context.Context, v domain.ReviewComment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.reviewComments[v.ID]
	if !ok || old.TenantID != v.TenantID {
		return domain.NotFound("审核批注")
	}
	s.reviewComments[v.ID] = v
	return nil
}

func (s *Store) CreateReviewGrant(_ context.Context, v domain.ReviewGrant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviewGrants[v.ID] = v
	return nil
}

func (s *Store) ReviewGrant(_ context.Context, tenantID, id string) (domain.ReviewGrant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	grant, ok := s.reviewGrants[id]
	if !ok || grant.TenantID != tenantID {
		return grant, domain.NotFound("客户审批授权")
	}
	return grant, nil
}

func (s *Store) ReviewGrants(_ context.Context, tenantID, subjectID string) ([]domain.ReviewGrant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ReviewGrant{}
	for _, grant := range s.reviewGrants {
		if grant.TenantID == tenantID && (subjectID == "" || grant.SubjectID == subjectID) {
			grant.TokenHash = ""
			grant.OTPHash = ""
			result = append(result, grant)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) ReviewGrantByTokenHash(_ context.Context, hash string) (domain.ReviewGrant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.reviewGrants {
		if v.TokenHash == hash {
			return v, nil
		}
	}
	return domain.ReviewGrant{}, domain.NotFound("客户审批授权")
}

func (s *Store) MarkReviewGrantVerified(_ context.Context, tenantID, id string, verifiedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	grant, ok := s.reviewGrants[id]
	if !ok || grant.TenantID != tenantID || grant.RevokedAt != nil || grant.DecisionAt != nil || !verifiedAt.Before(grant.ExpiresAt) {
		return domain.Conflict("REVIEW_GRANT_STATE_INVALID", "客户审批授权已撤销、已完成或已过期")
	}
	if grant.VerifiedAt == nil {
		grant.VerifiedAt = &verifiedAt
		s.reviewGrants[id] = grant
	}
	return nil
}

func (s *Store) RevokeReviewGrant(_ context.Context, tenantID, id string, revokedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	grant, ok := s.reviewGrants[id]
	if !ok || grant.TenantID != tenantID || grant.DecisionAt != nil {
		return domain.Conflict("REVIEW_GRANT_STATE_INVALID", "客户审批授权不存在或已完成")
	}
	if grant.RevokedAt == nil {
		grant.RevokedAt = &revokedAt
		s.reviewGrants[id] = grant
	}
	return nil
}

func (s *Store) CreateArtifact(_ context.Context, v domain.Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ApprovedSnapshotID == "" {
		return domain.Invalid("ARTIFACT_SNAPSHOT_REQUIRED", "成果文件必须绑定批准快照")
	}
	snapshot, exists := s.approvedSnapshots[v.ApprovedSnapshotID]
	if !exists || snapshot.TenantID != v.TenantID || snapshot.ProjectID != v.ProjectID {
		return domain.NotFound("批准快照")
	}
	s.artifacts[v.ID] = v
	return nil
}

func (s *Store) ArtifactsByApprovedSnapshot(_ context.Context, tenantID, snapshotID string) ([]domain.Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Artifact{}
	for _, value := range s.artifacts {
		if value.TenantID == tenantID && value.ApprovedSnapshotID == snapshotID {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) Artifact(_ context.Context, tenantID, id string) (domain.Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.artifacts[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("产物")
	}
	return v, nil
}

func (s *Store) CreateDeliveryPackage(_ context.Context, value domain.DeliveryPackage, artifacts []domain.Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.deliveryPackages[value.ID]; exists {
		return domain.Conflict("DELIVERY_PACKAGE_EXISTS", "交付包已存在")
	}
	approved := make(map[string]bool, len(value.ApprovedSnapshotIDs))
	for _, snapshotID := range value.ApprovedSnapshotIDs {
		snapshot, exists := s.approvedSnapshots[snapshotID]
		if !exists || snapshot.TenantID != value.TenantID || snapshot.ProjectID != value.ProjectID {
			return domain.NotFound("批准快照")
		}
		approved[snapshotID] = true
	}
	for _, artifact := range artifacts {
		if artifact.TenantID != value.TenantID || artifact.ProjectID != value.ProjectID || !approved[artifact.ApprovedSnapshotID] {
			return domain.Invalid("DELIVERY_ARTIFACT_SCOPE_INVALID", "交付成果文件必须绑定交付包中的批准快照")
		}
		if existing, exists := s.artifacts[artifact.ID]; exists {
			if existing.TenantID != artifact.TenantID || existing.ProjectID != artifact.ProjectID || existing.ApprovedSnapshotID != artifact.ApprovedSnapshotID || existing.SHA256 != artifact.SHA256 {
				return domain.Conflict("ARTIFACT_IDENTITY_MISMATCH", "交付成果文件与已保存对象不一致")
			}
		}
	}
	for _, artifact := range artifacts {
		if _, exists := s.artifacts[artifact.ID]; !exists {
			s.artifacts[artifact.ID] = artifact
		}
	}
	value.Manifest = append([]domain.Artifact(nil), artifacts...)
	s.deliveryPackages[value.ID] = value
	return nil
}

func (s *Store) DeliveryPackages(_ context.Context, tenantID, projectID string) ([]domain.DeliveryPackage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.DeliveryPackage{}
	for _, value := range s.deliveryPackages {
		if value.TenantID == tenantID && (projectID == "" || value.ProjectID == projectID) {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) DeliveryPackage(_ context.Context, tenantID, id string) (domain.DeliveryPackage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.deliveryPackages[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("交付包")
	}
	return value, nil
}

func (s *Store) CreatePerformanceImportBatch(_ context.Context, batch domain.PerformanceImportBatch, observations []domain.PerformanceObservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.performanceBatches[batch.ID]; exists {
		return domain.Conflict("PERFORMANCE_IMPORT_EXISTS", "结果导入批次已存在")
	}
	for _, candidate := range observations {
		for _, existing := range s.observations {
			if existing.TenantID == batch.TenantID && existing.ProjectID == batch.ProjectID && existing.DedupKey == candidate.DedupKey {
				return domain.Conflict("PERFORMANCE_OBSERVATION_DUPLICATE", "结果观察已存在")
			}
		}
	}
	s.performanceBatches[batch.ID] = batch
	for _, observation := range observations {
		s.observations[observation.ID] = observation
	}
	return nil
}

func (s *Store) PerformanceImportBatches(_ context.Context, tenantID, projectID string) ([]domain.PerformanceImportBatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.PerformanceImportBatch{}
	for _, v := range s.performanceBatches {
		if v.TenantID == tenantID && v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) PerformanceImportBatch(_ context.Context, tenantID, id string) (domain.PerformanceImportBatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.performanceBatches[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("结果导入批次")
	}
	return v, nil
}

func (s *Store) ExistingPerformanceDedupKeys(_ context.Context, tenantID, projectID string, keys []string) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wanted := map[string]bool{}
	for _, key := range keys {
		wanted[key] = true
	}
	out := map[string]string{}
	for _, v := range s.observations {
		if v.TenantID == tenantID && v.ProjectID == projectID && wanted[v.DedupKey] {
			out[v.DedupKey] = v.ID
		}
	}
	return out, nil
}

func (s *Store) PerformanceObservation(_ context.Context, tenantID, id string) (domain.PerformanceObservation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.observations[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("结果观察")
	}
	return v, nil
}

func (s *Store) PerformanceObservations(_ context.Context, tenantID, projectID string) ([]domain.PerformanceObservation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.PerformanceObservation{}
	for _, v := range s.observations {
		if v.TenantID == tenantID && v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishedAt.After(out[j].PublishedAt) })
	return out, nil
}

func (s *Store) CreateRatingDecision(_ context.Context, v domain.RatingDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.ratingDecisions[v.ID]; exists {
		return domain.Conflict("RATING_DECISION_EXISTS", "评级决策已存在")
	}
	s.ratingDecisions[v.ID] = v
	return nil
}

func (s *Store) RatingDecisions(_ context.Context, tenantID, projectID string) ([]domain.RatingDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.RatingDecision{}
	for _, v := range s.ratingDecisions {
		if v.TenantID == tenantID && v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
