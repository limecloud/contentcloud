package memory

import (
	"context"
	"sort"

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

func (s *Store) CreateBenchmark(_ context.Context, v domain.BenchmarkContent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.benchmarks[v.ID] = v
	return nil
}

func (s *Store) Benchmarks(_ context.Context, tenantID, projectID string) ([]domain.BenchmarkContent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.BenchmarkContent{}
	for _, v := range s.benchmarks {
		if v.TenantID == tenantID && v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) Benchmark(_ context.Context, tenantID, id string) (domain.BenchmarkContent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.benchmarks[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("对标内容")
	}
	return v, nil
}

func (s *Store) CreateFramework(_ context.Context, v domain.ContentFramework) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frameworks[v.ID] = v
	return nil
}

func (s *Store) Frameworks(_ context.Context, tenantID, projectID string) ([]domain.ContentFramework, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.ContentFramework{}
	for _, v := range s.frameworks {
		if v.TenantID == tenantID && v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) Framework(_ context.Context, tenantID, id string) (domain.ContentFramework, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.frameworks[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("内容框架")
	}
	return v, nil
}

func (s *Store) CreateShotPattern(_ context.Context, v domain.ShotPattern) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shotPatterns[v.ID] = v
	return nil
}

func (s *Store) ShotPatterns(_ context.Context, tenantID, projectID string) ([]domain.ShotPattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.ShotPattern{}
	for _, v := range s.shotPatterns {
		if v.TenantID == tenantID && v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) CreateSellingPoint(_ context.Context, v domain.SellingPoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sellingPoints[v.ID] = v
	return nil
}

func (s *Store) SellingPoints(_ context.Context, tenantID, projectID string) ([]domain.SellingPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.SellingPoint{}
	for _, v := range s.sellingPoints {
		if v.TenantID == tenantID && v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Priority < out[j].Priority
	})
	return out, nil
}

func (s *Store) SellingPoint(_ context.Context, tenantID, id string) (domain.SellingPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.sellingPoints[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("卖点")
	}
	return v, nil
}

func (s *Store) CreateVisualizationPlan(_ context.Context, v domain.VisualizationPlan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visualizationPlans[v.ID] = v
	return nil
}

func (s *Store) VisualizationPlans(_ context.Context, tenantID, projectID string) ([]domain.VisualizationPlan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.VisualizationPlan{}
	for _, v := range s.visualizationPlans {
		if v.TenantID == tenantID && v.ProjectID == projectID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) VisualizationPlan(_ context.Context, tenantID, id string) (domain.VisualizationPlan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.visualizationPlans[id]
	if !ok || v.TenantID != tenantID {
		return v, domain.NotFound("可视化方案")
	}
	return v, nil
}

func (s *Store) SaveVisualizationPlan(_ context.Context, v domain.VisualizationPlan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.visualizationPlans[v.ID]
	if !ok || old.TenantID != v.TenantID {
		return domain.NotFound("可视化方案")
	}
	s.visualizationPlans[v.ID] = v
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

func (s *Store) SaveReviewGrant(_ context.Context, v domain.ReviewGrant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.reviewGrants[v.ID]; !ok {
		return domain.NotFound("客户审批授权")
	}
	s.reviewGrants[v.ID] = v
	return nil
}

func (s *Store) CreateArtifact(_ context.Context, v domain.Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts[v.ID] = v
	return nil
}

func (s *Store) Artifacts(_ context.Context, tenantID, scriptVersionID string) ([]domain.Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Artifact{}
	for _, v := range s.artifacts {
		if v.TenantID == tenantID && (scriptVersionID == "" || v.ScriptVersionID == scriptVersionID) {
			out = append(out, v)
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
