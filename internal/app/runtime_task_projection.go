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

func (s *Service) projectRuntimeJob(ctx context.Context, job domain.JobRun) (domain.TaskRun, error) {
	plan, err := s.runtimeService.Plan(ctx, job.TenantID, job.PlanRevisionID)
	if err != nil {
		return domain.TaskRun{}, err
	}
	nodes, err := s.runtimeService.Nodes(ctx, job.TenantID, job.ID)
	if err != nil {
		return domain.TaskRun{}, err
	}
	attemptCount := 0
	outputRefs := []string{}
	for _, node := range nodes {
		attemptCount += node.AttemptCount
		outputRefs = append(outputRefs, node.OutputRefs...)
	}
	projected := domain.TaskRun{
		ID: job.ID, TenantID: job.TenantID, ProjectID: job.ProjectID, WorkTaskID: job.WorkTaskID,
		InputSnapshotID: job.InputSnapshotID,
		SOPID:           plan.SOPID, SOPVersion: plan.SOPVersion, SOPDigest: plan.SOPDigest,
		StageID: runtimeProjectionStageID(nodes), ExecutionMode: "runtime", ExecutorKind: "runtime",
		OutputRefs: outputRefs, IdempotencyKey: job.IdempotencyKey, TaskType: "runtime.job",
		CapabilityID: "contentcloud.runtime", CapabilityVersion: "1.0.0",
		InputSchema: "contentcloud.runtime-input/1.0", OutputSchema: "contentcloud.runtime-output/1.0",
		OutputCount: len(outputRefs), DeliveryProfiles: []string{"workspace"}, State: runtimeTaskRunState(job.State),
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

func (s *Service) runtimeTaskRunsForProject(ctx context.Context, tenantID, projectID string) ([]domain.TaskRun, error) {
	if s.runtimeService == nil {
		return nil, nil
	}
	jobs, err := s.runtimeService.Jobs(ctx, tenantID, "")
	if err != nil {
		return nil, err
	}
	result := make([]domain.TaskRun, 0, len(jobs))
	for _, job := range jobs {
		if job.ProjectID != projectID {
			continue
		}
		projected, err := s.projectRuntimeJob(ctx, job)
		if err != nil {
			return nil, err
		}
		result = append(result, projected)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (s *Service) taskRunsForProject(ctx context.Context, tenantID, projectID string) ([]domain.TaskRun, error) {
	return s.runtimeTaskRunsForProject(ctx, tenantID, projectID)
}

func (s *Service) taskRunsForTenant(ctx context.Context, tenantID string) ([]domain.TaskRun, error) {
	if s.runtimeService == nil {
		return []domain.TaskRun{}, nil
	}
	jobs, err := s.runtimeService.Jobs(ctx, tenantID, "")
	if err != nil {
		return nil, err
	}
	runtimeRuns := make([]domain.TaskRun, 0, len(jobs))
	for _, job := range jobs {
		projected, err := s.projectRuntimeJob(ctx, job)
		if err != nil {
			return nil, err
		}
		runtimeRuns = append(runtimeRuns, projected)
	}
	sort.Slice(runtimeRuns, func(i, j int) bool { return runtimeRuns[i].UpdatedAt.After(runtimeRuns[j].UpdatedAt) })
	return runtimeRuns, nil
}

// runtimeTaskRunProjection keeps the existing WorkTask API readable while
// making Runtime JobRun/NodeRun the only execution fact source for new runs.
// An empty result means the task has not started a Runtime JobRun yet.
func (s *Service) runtimeTaskRunProjection(ctx context.Context, tenantID, taskID string) ([]domain.TaskRun, error) {
	if s.runtimeService == nil {
		return []domain.TaskRun{}, nil
	}
	jobs, err := s.runtimeService.Jobs(ctx, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return []domain.TaskRun{}, nil
	}
	result := make([]domain.TaskRun, 0, len(jobs))
	for _, job := range jobs {
		projected, err := s.projectRuntimeJob(ctx, job)
		if err != nil {
			return nil, err
		}
		result = append(result, projected)
	}
	return result, nil
}

func runtimeTaskRunState(state string) string {
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
