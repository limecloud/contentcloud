package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	fixturev3 "github.com/limecloud/contentcloud/internal/bootstrap/fixture"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type FixtureImportResult struct {
	FixtureVersion string                           `json:"fixture_version"`
	Project        workspacedomain.Project          `json:"project"`
	Device         workspacedomain.Device           `json:"device"`
	Workspace      workspacedomain.WorkspaceBinding `json:"workspace"`
	Submissions    []reviewdomain.Submission        `json:"submissions"`
	Snapshots      []reviewdomain.ApprovedSnapshot  `json:"snapshots"`
}

func (s *ReviewService) ImportFixtureV3(ctx context.Context, actor Actor, fixture fixturev3.Fixture, requestID string) (FixtureImportResult, error) {
	if actor.Role != "tenant_admin" {
		return FixtureImportResult{}, fault.Policy("ROLE_DENIED", "只有租户管理员可以导入开发演示数据", "切换到租户管理员账号")
	}
	if err := fixture.Validate(); err != nil {
		return FixtureImportResult{}, fault.Invalid("FIXTURE_V3_INVALID", err.Error())
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

	snapshotByType := map[string]reviewdomain.ApprovedSnapshot{}
	existingSnapshots, err := s.review.ApprovedSnapshots(ctx, actor.TenantID, project.ID, "")
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
			submission, err = s.review.Submission(ctx, actor.TenantID, revision.SubmissionID)
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
				return FixtureImportResult{}, fault.Conflict("FIXTURE_SUBMISSION_STATE_INVALID", fmt.Sprintf("%s 提交记录当前状态为 %s，无法收敛为已批准（approved）", spec.SubmissionType, submission.Status))
			}
			if _, exists := snapshotByType[spec.SubmissionType]; !exists {
				return FixtureImportResult{}, fault.Conflict("FIXTURE_SNAPSHOT_MISSING", spec.SubmissionType+" 已批准但缺少已批准快照")
			}
		case "submitted":
			if submission.Status != "submitted" && submission.Status != "in_review" {
				return FixtureImportResult{}, fault.Conflict("FIXTURE_SUBMISSION_STATE_INVALID", fmt.Sprintf("%s 提交记录当前状态为 %s，无法收敛为已提交（submitted）", spec.SubmissionType, submission.Status))
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

	submissions, err := s.review.Submissions(ctx, actor.TenantID, project.ID)
	if err != nil {
		return FixtureImportResult{}, err
	}
	snapshots, err := s.review.ApprovedSnapshots(ctx, actor.TenantID, project.ID, "")
	if err != nil {
		return FixtureImportResult{}, err
	}
	return FixtureImportResult{FixtureVersion: fixture.FixtureVersion, Project: project, Device: device, Workspace: binding, Submissions: submissions, Snapshots: snapshots}, nil
}

func (s *ReviewService) ensureFixtureProject(ctx context.Context, actor Actor, fixture fixturev3.Fixture, requestID string) (workspacedomain.Project, error) {
	projects, err := s.workspace.Projects(ctx, actor.TenantID)
	if err != nil {
		return workspacedomain.Project{}, err
	}
	for _, project := range projects {
		if project.BrandName == fixture.Project.BrandName && project.ProductName == fixture.Project.ProductName {
			return project, nil
		}
	}
	return s.app.Workspace.CreateProject(ctx, actor, CreateProjectInput{
		BrandName: fixture.Project.BrandName, ProductName: fixture.Project.ProductName, Channel: fixture.Project.Channel,
		StageObjective: fixture.Project.StageObjective, OwnerName: fixture.Project.OwnerName, ReviewerName: fixture.Project.ReviewerName, ClientApprover: fixture.Project.ClientApprover,
	}, requestID)
}

func (s *ReviewService) ensureFixtureWorkspace(ctx context.Context, actor Actor, project workspacedomain.Project, workspace fixturev3.WorkspaceSpec, requestID string) (workspacedomain.Device, workspacedomain.WorkspaceBinding, error) {
	now := s.now().UTC()
	deviceID := fixturev3.DeterministicID("fixture-device", actor.TenantID+":"+project.ID)
	device, err := s.workspace.Device(ctx, actor.TenantID, deviceID)
	if isNotFound(err) {
		device = workspacedomain.Device{ID: deviceID, TenantID: actor.TenantID, OwnerUserID: actor.UserID, DisplayName: workspace.DeviceName, Hostname: "fixture.local", Platform: "development", Arch: "fixture", Version: "fixture-v3", TokenHash: fixturev3.SHA256Hex("device:" + actor.TenantID + ":" + project.ID), Capabilities: []catalogdomain.Capability{}, ProjectIDs: []string{}, LastSeenAt: now}
		if err = s.workspace.SaveDevice(ctx, device); err != nil {
			return workspacedomain.Device{}, workspacedomain.WorkspaceBinding{}, err
		}
	} else if err != nil {
		return workspacedomain.Device{}, workspacedomain.WorkspaceBinding{}, err
	}
	device, err = s.app.Workspace.AttachDevice(ctx, actor, deviceID, project.ID, requestID)
	if err != nil {
		return workspacedomain.Device{}, workspacedomain.WorkspaceBinding{}, err
	}

	workspaceID := fixturev3.DeterministicID("fixture-workspace", actor.TenantID+":"+project.ID)
	binding, err := s.workspace.WorkspaceBinding(ctx, actor.TenantID, workspaceID)
	if isNotFound(err) {
		binding = workspacedomain.WorkspaceBinding{ID: workspaceID, TenantID: actor.TenantID, ProjectID: project.ID, DeviceID: device.ID, OwnerUserID: actor.UserID, TemplateID: workspace.TemplateID, TemplateVersion: workspace.TemplateVersion, Targets: append([]string(nil), workspace.Targets...), CredentialHash: fixturev3.SHA256Hex("workspace:" + actor.TenantID + ":" + project.ID), Status: "active", InitializedAt: now, LastSeenAt: now}
		if err = s.workspace.CreateWorkspaceBinding(ctx, binding); err != nil {
			return workspacedomain.Device{}, workspacedomain.WorkspaceBinding{}, err
		}
	} else if err != nil {
		return workspacedomain.Device{}, workspacedomain.WorkspaceBinding{}, err
	}
	if binding.ProjectID != project.ID || binding.DeviceID != device.ID {
		return workspacedomain.Device{}, workspacedomain.WorkspaceBinding{}, fault.Conflict("FIXTURE_WORKSPACE_MISMATCH", "开发演示的本地工作区已绑定到其他项目或设备")
	}
	workspaceActor := Actor{UserID: actor.UserID, TenantID: actor.TenantID, Role: actor.Role, Type: "workspace", DeviceID: device.ID, WorkspaceID: binding.ID}
	binding, err = s.RegisterWorkspace(ctx, workspaceActor, binding, workspace.TemplateID, workspace.TemplateVersion, workspace.Targets, requestID)
	return device, binding, err
}

func (s *ReviewService) fixtureSubmission(ctx context.Context, actor Actor, projectID, workspaceID, submissionType string) (reviewdomain.Submission, reviewdomain.SubmissionRevision, bool, error) {
	submission, err := s.review.SubmissionByWorkspaceType(ctx, actor.TenantID, projectID, workspaceID, submissionType)
	if isNotFound(err) {
		return reviewdomain.Submission{}, reviewdomain.SubmissionRevision{}, false, nil
	}
	if err != nil {
		return reviewdomain.Submission{}, reviewdomain.SubmissionRevision{}, false, err
	}
	revision, err := s.review.SubmissionRevision(ctx, actor.TenantID, submission.CurrentRevisionID)
	if err != nil {
		return reviewdomain.Submission{}, reviewdomain.SubmissionRevision{}, false, err
	}
	return submission, revision, true, nil
}

func buildFixtureBundle(projectID, workspaceID string, fixture fixturev3.Fixture, spec fixturev3.SubmissionSpec, snapshotByType map[string]reviewdomain.ApprovedSnapshot) (reviewdomain.SubmissionBundle, error) {
	objects := make([]reviewdomain.SubmissionObjectRef, 0, len(spec.Objects))
	for _, object := range spec.Objects {
		ref, err := reviewdomain.NewSubmissionObjectRef(object.ID, object.Type, object.Version, object.Path, object.Content)
		if err != nil {
			return reviewdomain.SubmissionBundle{}, err
		}
		objects = append(objects, ref)
	}
	baseSnapshotIDs := make([]string, 0, len(spec.BaseSnapshotTypes))
	for _, submissionType := range spec.BaseSnapshotTypes {
		snapshot, exists := snapshotByType[submissionType]
		if !exists {
			return reviewdomain.SubmissionBundle{}, fault.Conflict("FIXTURE_BASE_SNAPSHOT_MISSING", "开发演示数据缺少 "+submissionType+" 已批准快照")
		}
		baseSnapshotIDs = append(baseSnapshotIDs, snapshot.ID)
	}
	bundle := reviewdomain.SubmissionBundle{
		BundleVersion: "3.0", SubmissionType: spec.SubmissionType, ProjectID: projectID, WorkspaceID: workspaceID, BaseSnapshotIDs: baseSnapshotIDs,
		Objects: objects, SourceDisclosures: []reviewdomain.SourceDisclosure{}, LocalRunSummary: reviewdomain.LocalRunSummary{RunID: "fixture:" + spec.SubmissionType, Stage: "fixture_import", Checks: []reviewdomain.LocalRunCheck{{Name: "fixture_schema", Status: "passed"}}, Versions: map[string]string{"fixture": fixture.FixtureVersion}},
		EnvironmentDigest: fixture.EnvironmentDigest, Artifacts: []reviewdomain.SubmissionArtifact{}, Message: strings.TrimSpace(spec.Message), IdempotencyKey: "fixture-v" + strings.ReplaceAll(fixture.FixtureVersion, ".", "-") + "-" + spec.SubmissionType,
	}
	if err := bundle.SetComputedHash(); err != nil {
		return reviewdomain.SubmissionBundle{}, err
	}
	return bundle, nil
}
