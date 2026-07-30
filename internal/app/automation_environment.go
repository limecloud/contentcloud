package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/store"
)

type AutomationEnvironmentClaim struct {
	Manifest environment.Manifest        `json:"manifest"`
	Lock     environment.EnvironmentLock `json:"lock"`
}

func (s *Service) createTaskRun(ctx context.Context, run domain.TaskRun, snapshot domain.ContextSnapshot) error {
	if len(s.automationPolicy) == 0 {
		return s.store.CreateRun(ctx, run)
	}
	if s.environmentControl == nil {
		return domain.Conflict("AUTOMATION_ENVIRONMENT_CONTROL_REQUIRED", "Automation Execution Policy 需要 Environment Control Plane")
	}
	requirement, exists := s.automationPolicy[run.CapabilityID]
	if !exists || requirement.ID != run.CapabilityID || requirement.SchemaVersion != run.CapabilityVersion {
		return domain.Policy("AUTOMATION_CAPABILITY_POLICY_MISSING", "TaskRun capability 没有精确的 Automation Execution Policy", "配置 capability schema version 和 digest 后再创建任务")
	}
	contentTypes, err := s.TenantContentTypes(ctx, run.TenantID)
	if err != nil {
		return err
	}
	bundle, err := s.environmentControl.IssueExecutionBundle(environment.ExecutionBundleRequest{
		ProjectID:    run.ProjectID,
		ContentTypes: contentTypes,
		Subject: environment.ExecutionSubject{
			Type: "context_snapshot", ID: snapshot.ID, Digest: normalizedSnapshotDigest(snapshot.ManifestHash),
		},
		RequiredCapabilities: []environment.CapabilityRequirement{requirement},
		PackIDs:              s.automationPackIDs[run.CapabilityID],
	}, s.now().UTC())
	if err != nil {
		return err
	}
	return s.store.CreateRunWithBundle(ctx, run, bundle)
}

func (s *Service) automationLeaseCandidates(ctx context.Context, actor Actor, capabilities []domain.Capability, claims []AutomationEnvironmentClaim, now time.Time) ([]store.RunLeaseCandidate, map[string]environment.CreativeExecutionBundle, map[string]domain.ContextSnapshot, error) {
	runs, err := s.store.Runs(ctx, actor.TenantID, "")
	if err != nil {
		return nil, nil, nil, err
	}
	claimsByProject, err := indexAutomationClaims(claims)
	if err != nil {
		return nil, nil, nil, err
	}
	eligible := make([]store.RunLeaseCandidate, 0, len(runs))
	bundles := make(map[string]environment.CreativeExecutionBundle)
	snapshots := make(map[string]domain.ContextSnapshot)
	var preparationRequired error
	for _, run := range runs {
		if run.State != "queued" || run.AttemptCount >= 3 {
			continue
		}
		capability, matched := matchingCapability(run, capabilities)
		if !matched {
			continue
		}
		if len(s.automationPolicy) == 0 {
			eligible = append(eligible, store.RunLeaseCandidate{RunID: run.ID, Capability: capability})
			continue
		}
		bundle, bundleErr := s.store.ExecutionBundle(ctx, actor.TenantID, run.ID)
		if bundleErr != nil {
			if isNotFound(bundleErr) {
				return nil, nil, nil, domain.Conflict("EXECUTION_BUNDLE_REQUIRED", "受治理 Automation TaskRun 缺少 CreativeExecutionBundle")
			}
			return nil, nil, nil, bundleErr
		}
		snapshot, snapshotErr := s.store.Snapshot(ctx, actor.TenantID, run.InputSnapshotID)
		if snapshotErr != nil {
			return nil, nil, nil, snapshotErr
		}
		if !bundleMatchesRun(bundle, run) {
			return nil, nil, nil, domain.Conflict("EXECUTION_BUNDLE_RUN_MISMATCH", "CreativeExecutionBundle capability 与 TaskRun 或设备声明不匹配")
		}
		claim, claimed := claimsByProject[run.ProjectID]
		if !claimed {
			preparationRequired = newEnvironmentPreparationError(run, bundle, "environment_claim_missing", nil)
			continue
		}
		resolution, resolveErr := s.environmentControl.ResolveExecutionBundle(bundle, claim.Manifest, claim.Lock, capabilities, environment.BundleVerifyOptions{
			ProjectID: run.ProjectID,
			ExpectedSubject: environment.ExecutionSubject{
				Type: "context_snapshot", ID: snapshot.ID, Digest: normalizedSnapshotDigest(snapshot.ManifestHash),
			},
			Now: now,
		})
		if resolveErr != nil {
			if environmentDriftError(resolveErr) {
				preparationRequired = newEnvironmentPreparationError(run, bundle, domainErrorCode(resolveErr), nil)
				continue
			}
			return nil, nil, nil, resolveErr
		}
		if resolution.State != "ready" {
			preparationRequired = newEnvironmentPreparationError(run, bundle, "environment_not_ready", resolution)
			continue
		}
		eligible = append(eligible, store.RunLeaseCandidate{RunID: run.ID, Capability: capability})
		bundles[run.ID] = bundle
		snapshots[run.ID] = snapshot
	}
	if len(eligible) == 0 && preparationRequired != nil {
		return nil, nil, nil, preparationRequired
	}
	return eligible, bundles, snapshots, nil
}

func indexAutomationClaims(claims []AutomationEnvironmentClaim) (map[string]AutomationEnvironmentClaim, error) {
	indexed := make(map[string]AutomationEnvironmentClaim, len(claims))
	for _, claim := range claims {
		projectID := strings.TrimSpace(claim.Manifest.ProjectID)
		if projectID == "" || claim.Lock.ProjectID != projectID {
			return nil, domain.Invalid("AUTOMATION_ENVIRONMENT_CLAIM_INVALID", "Automation Environment Claim 缺少一致的 project_id")
		}
		if _, exists := indexed[projectID]; exists {
			return nil, domain.Conflict("AUTOMATION_ENVIRONMENT_CLAIM_DUPLICATED", "同一项目包含重复 Automation Environment Claim")
		}
		indexed[projectID] = claim
	}
	return indexed, nil
}

func matchingCapability(run domain.TaskRun, capabilities []domain.Capability) (domain.Capability, bool) {
	for _, capability := range capabilities {
		if run.AcceptsCapability(capability) {
			return capability, true
		}
	}
	return domain.Capability{}, false
}

func bundleMatchesRun(bundle environment.CreativeExecutionBundle, run domain.TaskRun) bool {
	if bundle.ProjectID != run.ProjectID || bundle.Subject.Type != "context_snapshot" || bundle.Subject.ID != run.InputSnapshotID {
		return false
	}
	for _, required := range bundle.RequiredCapabilities {
		if required.ID == run.CapabilityID && required.SchemaVersion == run.CapabilityVersion {
			return true
		}
	}
	return false
}

func normalizedSnapshotDigest(value string) string {
	if strings.HasPrefix(value, "sha256:") {
		return value
	}
	return "sha256:" + value
}

func newEnvironmentPreparationError(run domain.TaskRun, bundle environment.CreativeExecutionBundle, reason string, resolution any) error {
	err := domain.Policy("ENVIRONMENT_PREPARATION_REQUIRED", "设备环境未满足 Automation TaskRun，领取前必须完成环境准备", "在没有活动 Run 的窗口完成 Environment doctor 和 Pack 安装后重试")
	err.Details = map[string]any{"run_id": run.ID, "project_id": run.ProjectID, "bundle_id": bundle.BundleID, "reason": reason, "resolution": resolution}
	return err
}

func environmentDriftError(err error) bool {
	code := domainErrorCode(err)
	return strings.HasPrefix(code, "ENVIRONMENT_LOCK_") || code == "ENVIRONMENT_REQUIRED_PLUGIN_MISSING" || code == "ENVIRONMENT_MANIFEST_EXPIRED" || code == "EXECUTION_BUNDLE_ENVIRONMENT_MISMATCH"
}

func domainErrorCode(err error) string {
	var value *domain.Error
	if errors.As(err, &value) {
		return value.Code
	}
	return "ENVIRONMENT_VALIDATION_FAILED"
}
