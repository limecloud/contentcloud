package app

import (
	"context"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Service) runtimeJob(ctx context.Context, tenantID, jobID string) (domain.JobRun, bool, error) {
	if s.runtimeService == nil {
		return domain.JobRun{}, false, nil
	}
	job, err := s.runtimeService.Job(ctx, tenantID, jobID)
	if domain.IsNotFound(err) {
		return domain.JobRun{}, false, nil
	}
	if err != nil {
		return domain.JobRun{}, true, err
	}
	return job, true, nil
}

func (s *Service) projectRuntimeRun(ctx context.Context, job domain.JobRun) (domain.RuntimeRun, error) {
	plan, err := s.runtimeService.Plan(ctx, job.TenantID, job.PlanRevisionID)
	if err != nil {
		return domain.RuntimeRun{}, err
	}
	nodes, err := s.runtimeService.Nodes(ctx, job.TenantID, job.ID)
	if err != nil {
		return domain.RuntimeRun{}, err
	}
	attemptCount := 0
	outputRefs := []string{}
	for _, node := range nodes {
		attemptCount += node.AttemptCount
		outputRefs = append(outputRefs, node.OutputRefs...)
	}
	projected := domain.RuntimeRun{
		ID: job.ID, TenantID: job.TenantID, ProjectID: job.ProjectID, WorkTaskID: job.WorkTaskID,
		InputSnapshotID: job.InputSnapshotID,
		SOPID:           plan.SOPID, SOPVersion: plan.SOPVersion, SOPDigest: plan.SOPDigest,
		StageID: runtimeProjectionStageID(nodes), ExecutionMode: "runtime", ExecutorKind: "runtime",
		OutputRefs: outputRefs, IdempotencyKey: job.IdempotencyKey, TaskType: "runtime.job",
		CapabilityID: "contentcloud.runtime", CapabilityVersion: "1.0.0",
		InputSchema: "contentcloud.runtime-input/1.0", OutputSchema: "contentcloud.runtime-output/1.0",
		OutputCount: len(outputRefs), DeliveryProfiles: []string{"workspace"}, State: runtimeRunState(job.State),
		Priority: job.Priority, AttemptCount: attemptCount, ErrorCode: job.ErrorCode,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}
	if strings.TrimSpace(job.BusinessType) != "" {
		projected.TaskType = job.BusinessType
	}
	if job.BusinessType == "knowledge_extract" {
		projected.CapabilityID = domain.KnowledgeExtractCapability
		projected.CapabilityVersion = "1.0.0"
		projected.InputSchema = domain.TaskContractSchema
		projected.OutputSchema = domain.KnowledgeCandidatesSchema
		projected.OutputCount = job.BusinessOutputCount
		projected.DeliveryProfiles = []string{"cloud_native"}
	}
	return projected, nil
}

func (s *Service) runtimeRunsForProject(ctx context.Context, tenantID, projectID string) ([]domain.RuntimeRun, error) {
	if s.runtimeService == nil {
		return nil, nil
	}
	jobs, err := s.runtimeService.Jobs(ctx, tenantID, "")
	if err != nil {
		return nil, err
	}
	result := make([]domain.RuntimeRun, 0, len(jobs))
	for _, job := range jobs {
		if job.ProjectID != projectID {
			continue
		}
		projected, err := s.projectRuntimeRun(ctx, job)
		if err != nil {
			return nil, err
		}
		result = append(result, projected)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (s *Service) runtimeRunsForTenant(ctx context.Context, tenantID string) ([]domain.RuntimeRun, error) {
	if s.runtimeService == nil {
		return []domain.RuntimeRun{}, nil
	}
	jobs, err := s.runtimeService.Jobs(ctx, tenantID, "")
	if err != nil {
		return nil, err
	}
	runtimeRuns := make([]domain.RuntimeRun, 0, len(jobs))
	for _, job := range jobs {
		projected, err := s.projectRuntimeRun(ctx, job)
		if err != nil {
			return nil, err
		}
		runtimeRuns = append(runtimeRuns, projected)
	}
	sort.Slice(runtimeRuns, func(i, j int) bool { return runtimeRuns[i].UpdatedAt.After(runtimeRuns[j].UpdatedAt) })
	return runtimeRuns, nil
}

// runtimeRunsForWorkTask projects authoritative JobRun/NodeRun facts into the
// WorkTask read model. An empty result means no Runtime JobRun has started.
func (s *Service) runtimeRunsForWorkTask(ctx context.Context, tenantID, taskID string) ([]domain.RuntimeRun, error) {
	if s.runtimeService == nil {
		return []domain.RuntimeRun{}, nil
	}
	jobs, err := s.runtimeService.Jobs(ctx, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return []domain.RuntimeRun{}, nil
	}
	result := make([]domain.RuntimeRun, 0, len(jobs))
	for _, job := range jobs {
		projected, err := s.projectRuntimeRun(ctx, job)
		if err != nil {
			return nil, err
		}
		result = append(result, projected)
	}
	return result, nil
}

func runtimeRunState(state string) string {
	switch state {
	case domain.JobRunCompleted:
		return "succeeded"
	case domain.JobRunFailed, domain.JobRunRejected:
		return "failed"
	case domain.JobRunCancelled:
		return "cancelled"
	case domain.JobRunPaused:
		return "paused"
	default:
		return "running"
	}
}

func runtimeProjectionStageID(nodes []domain.NodeRun) string {
	first := ""
	for _, node := range nodes {
		if strings.HasPrefix(node.NodeKey, "stage:") {
			if first == "" {
				first = strings.TrimPrefix(node.NodeKey, "stage:")
			}
			switch node.State {
			case domain.NodePending, domain.NodeReady, domain.NodeWaitingResource, domain.NodeLeased, domain.NodeRunning, domain.NodeWaitingChildren, domain.NodeWaitingExternal, domain.NodeWaitingHuman, domain.NodeRetryableFailed, domain.NodeBlocked, domain.NodeLeaseExpired:
				return strings.TrimPrefix(node.NodeKey, "stage:")
			}
		}
	}
	if first != "" {
		return first
	}
	return "runtime"
}
