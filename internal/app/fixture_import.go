package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/fixturev3"
)

type FixtureImportResult struct {
	FixtureVersion string                    `json:"fixture_version"`
	Project        domain.Project            `json:"project"`
	Device         domain.Device             `json:"device"`
	Workspace      domain.WorkspaceBinding   `json:"workspace"`
	Submissions    []domain.Submission       `json:"submissions"`
	Snapshots      []domain.ApprovedSnapshot `json:"snapshots"`
}

func (s *Service) ImportFixtureV3(ctx context.Context, actor Actor, fixture fixturev3.Fixture, requestID string) (FixtureImportResult, error) {
	if actor.Role != "tenant_admin" {
		return FixtureImportResult{}, domain.Policy("ROLE_DENIED", "只有租户管理员可以导入开发演示数据", "切换到租户管理员账号")
	}
	if err := fixture.Validate(); err != nil {
		return FixtureImportResult{}, domain.Invalid("FIXTURE_V3_INVALID", err.Error())
	}

	project, err := s.ensureFixtureProject(ctx, actor, fixture, requestID)
	if err != nil {
		return FixtureImportResult{}, err
	}
	device, binding, err := s.ensureFixtureWorkspace(ctx, actor, project, fixture.Workspace, requestID)
	if err != nil {
		return FixtureImportResult{}, err
	}
	workspaceActor := Actor{UserID: actor.UserID, TenantID: actor.TenantID, Role: actor.Role, Type: "workspace", DeviceID: device.ID, WorkspaceID: binding.ID}

	snapshotByType := map[string]domain.ApprovedSnapshot{}
	existingSnapshots, err := s.store.ApprovedSnapshots(ctx, actor.TenantID, project.ID, "")
	if err != nil {
		return FixtureImportResult{}, err
	}
	for _, snapshot := range existingSnapshots {
		if _, exists := snapshotByType[snapshot.SubmissionType]; !exists {
			snapshotByType[snapshot.SubmissionType] = snapshot
		}
	}

	for _, spec := range fixture.Submissions {
		submission, revision, found, err := s.fixtureSubmission(ctx, actor, project.ID, binding.ID, spec.SubmissionType)
		if err != nil {
			return FixtureImportResult{}, err
		}
		if !found {
			bundle, buildErr := buildFixtureBundle(project.ID, binding.ID, fixture, spec, snapshotByType)
			if buildErr != nil {
				return FixtureImportResult{}, buildErr
			}
			revision, err = s.CreateSubmission(ctx, workspaceActor, binding, bundle, requestID)
			if err != nil {
				return FixtureImportResult{}, err
			}
			submission, err = s.store.Submission(ctx, actor.TenantID, revision.SubmissionID)
			if err != nil {
				return FixtureImportResult{}, err
			}
		}

		switch spec.Outcome {
		case "approved":
			if submission.Status == "submitted" || submission.Status == "in_review" {
				approval, approveErr := s.ApproveSubmission(ctx, actor, revision.ID, "V3 开发演示基线批准", requestID)
				if approveErr != nil {
					return FixtureImportResult{}, approveErr
				}
				submission = approval.Submission
				if approval.ApprovedSnapshot != nil {
					snapshotByType[spec.SubmissionType] = *approval.ApprovedSnapshot
				}
			}
			if submission.Status != "approved" {
				return FixtureImportResult{}, domain.Conflict("FIXTURE_SUBMISSION_STATE_INVALID", fmt.Sprintf("%s 提交记录当前状态为 %s，无法收敛为已批准（approved）", spec.SubmissionType, submission.Status))
			}
			if _, exists := snapshotByType[spec.SubmissionType]; !exists {
				return FixtureImportResult{}, domain.Conflict("FIXTURE_SNAPSHOT_MISSING", spec.SubmissionType+" 已批准但缺少已批准快照")
			}
		case "submitted":
			if submission.Status != "submitted" && submission.Status != "in_review" {
				return FixtureImportResult{}, domain.Conflict("FIXTURE_SUBMISSION_STATE_INVALID", fmt.Sprintf("%s 提交记录当前状态为 %s，无法收敛为已提交（submitted）", spec.SubmissionType, submission.Status))
			}
		case "changes_requested":
			if submission.Status != "changes_requested" {
				returned, returnErr := s.RequestSubmissionChanges(ctx, actor, revision.ID, spec.ChangeReason, spec.ChangePointer, requestID)
				if returnErr != nil {
					return FixtureImportResult{}, returnErr
				}
				submission = returned
			}
		}
	}

	submissions, err := s.store.Submissions(ctx, actor.TenantID, project.ID)
	if err != nil {
		return FixtureImportResult{}, err
	}
	snapshots, err := s.store.ApprovedSnapshots(ctx, actor.TenantID, project.ID, "")
	if err != nil {
		return FixtureImportResult{}, err
	}
	return FixtureImportResult{FixtureVersion: fixture.FixtureVersion, Project: project, Device: device, Workspace: binding, Submissions: submissions, Snapshots: snapshots}, nil
}

func (s *Service) ensureFixtureProject(ctx context.Context, actor Actor, fixture fixturev3.Fixture, requestID string) (domain.Project, error) {
	projects, err := s.store.Projects(ctx, actor.TenantID)
	if err != nil {
		return domain.Project{}, err
	}
	for _, project := range projects {
		if project.BrandName == fixture.Project.BrandName && project.ProductName == fixture.Project.ProductName {
			return project, nil
		}
	}
	return s.CreateProject(ctx, actor, CreateProjectInput{
		BrandName: fixture.Project.BrandName, ProductName: fixture.Project.ProductName, Channel: fixture.Project.Channel,
		StageObjective: fixture.Project.StageObjective, OwnerName: fixture.Project.OwnerName, ReviewerName: fixture.Project.ReviewerName, ClientApprover: fixture.Project.ClientApprover,
	}, requestID)
}

func (s *Service) ensureFixtureWorkspace(ctx context.Context, actor Actor, project domain.Project, workspace fixturev3.WorkspaceSpec, requestID string) (domain.Device, domain.WorkspaceBinding, error) {
	now := s.now().UTC()
	deviceID := fixturev3.DeterministicID("fixture-device", actor.TenantID+":"+project.ID)
	device, err := s.store.Device(ctx, actor.TenantID, deviceID)
	if isNotFound(err) {
		device = domain.Device{ID: deviceID, TenantID: actor.TenantID, OwnerUserID: actor.UserID, DisplayName: workspace.DeviceName, Hostname: "fixture.local", Platform: "development", Arch: "fixture", Version: "fixture-v3", TokenHash: fixturev3.SHA256Hex("device:" + actor.TenantID + ":" + project.ID), Capabilities: []domain.Capability{}, ProjectIDs: []string{}, LastSeenAt: now}
		if err = s.store.SaveDevice(ctx, device); err != nil {
			return domain.Device{}, domain.WorkspaceBinding{}, err
		}
	} else if err != nil {
		return domain.Device{}, domain.WorkspaceBinding{}, err
	}
	device, err = s.AttachDevice(ctx, actor, deviceID, project.ID, requestID)
	if err != nil {
		return domain.Device{}, domain.WorkspaceBinding{}, err
	}

	workspaceID := fixturev3.DeterministicID("fixture-workspace", actor.TenantID+":"+project.ID)
	binding, err := s.store.WorkspaceBinding(ctx, actor.TenantID, workspaceID)
	if isNotFound(err) {
		binding = domain.WorkspaceBinding{ID: workspaceID, TenantID: actor.TenantID, ProjectID: project.ID, DeviceID: device.ID, OwnerUserID: actor.UserID, TemplateID: workspace.TemplateID, TemplateVersion: workspace.TemplateVersion, Targets: append([]string(nil), workspace.Targets...), CredentialHash: fixturev3.SHA256Hex("workspace:" + actor.TenantID + ":" + project.ID), Status: "active", InitializedAt: now, LastSeenAt: now}
		if err = s.store.CreateWorkspaceBinding(ctx, binding); err != nil {
			return domain.Device{}, domain.WorkspaceBinding{}, err
		}
	} else if err != nil {
		return domain.Device{}, domain.WorkspaceBinding{}, err
	}
	if binding.ProjectID != project.ID || binding.DeviceID != device.ID {
		return domain.Device{}, domain.WorkspaceBinding{}, domain.Conflict("FIXTURE_WORKSPACE_MISMATCH", "开发演示的本地工作区已绑定到其他项目或设备")
	}
	workspaceActor := Actor{UserID: actor.UserID, TenantID: actor.TenantID, Role: actor.Role, Type: "workspace", DeviceID: device.ID, WorkspaceID: binding.ID}
	binding, err = s.RegisterWorkspace(ctx, workspaceActor, binding, workspace.TemplateID, workspace.TemplateVersion, workspace.Targets, requestID)
	return device, binding, err
}

func (s *Service) fixtureSubmission(ctx context.Context, actor Actor, projectID, workspaceID, submissionType string) (domain.Submission, domain.SubmissionRevision, bool, error) {
	submission, err := s.store.SubmissionByWorkspaceType(ctx, actor.TenantID, projectID, workspaceID, submissionType)
	if isNotFound(err) {
		return domain.Submission{}, domain.SubmissionRevision{}, false, nil
	}
	if err != nil {
		return domain.Submission{}, domain.SubmissionRevision{}, false, err
	}
	revision, err := s.store.SubmissionRevision(ctx, actor.TenantID, submission.CurrentRevisionID)
	if err != nil {
		return domain.Submission{}, domain.SubmissionRevision{}, false, err
	}
	return submission, revision, true, nil
}

func buildFixtureBundle(projectID, workspaceID string, fixture fixturev3.Fixture, spec fixturev3.SubmissionSpec, snapshotByType map[string]domain.ApprovedSnapshot) (domain.SubmissionBundle, error) {
	objects := make([]domain.SubmissionObjectRef, 0, len(spec.Objects))
	for _, object := range spec.Objects {
		ref, err := domain.NewSubmissionObjectRef(object.ID, object.Type, object.Version, object.Path, object.Content)
		if err != nil {
			return domain.SubmissionBundle{}, err
		}
		objects = append(objects, ref)
	}
	baseSnapshotIDs := make([]string, 0, len(spec.BaseSnapshotTypes))
	for _, submissionType := range spec.BaseSnapshotTypes {
		snapshot, exists := snapshotByType[submissionType]
		if !exists {
			return domain.SubmissionBundle{}, domain.Conflict("FIXTURE_BASE_SNAPSHOT_MISSING", "开发演示数据缺少 "+submissionType+" 已批准快照")
		}
		baseSnapshotIDs = append(baseSnapshotIDs, snapshot.ID)
	}
	bundle := domain.SubmissionBundle{
		BundleVersion: "3.0", SubmissionType: spec.SubmissionType, ProjectID: projectID, WorkspaceID: workspaceID, BaseSnapshotIDs: baseSnapshotIDs,
		Objects: objects, SourceDisclosures: []domain.SourceDisclosure{}, LocalRunSummary: domain.LocalRunSummary{RunID: "fixture:" + spec.SubmissionType, Stage: "fixture_import", Checks: []domain.LocalRunCheck{{Name: "fixture_schema", Status: "passed"}}, Versions: map[string]string{"fixture": fixture.FixtureVersion}},
		EnvironmentDigest: fixture.EnvironmentDigest, Artifacts: []domain.SubmissionArtifact{}, Message: strings.TrimSpace(spec.Message), IdempotencyKey: "fixture-v" + strings.ReplaceAll(fixture.FixtureVersion, ".", "-") + "-" + spec.SubmissionType,
	}
	if err := bundle.SetComputedHash(); err != nil {
		return domain.SubmissionBundle{}, err
	}
	return bundle, nil
}
