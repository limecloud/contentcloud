package application_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	mediapipeline "github.com/limecloud/contentcloud/internal/integration/provider/media"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	"github.com/limecloud/contentcloud/internal/work"
)

func TestMarketingVideoGoldenJourney(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), nil)
	session, err := service.Identity.Register(ctx, "marketing-video@example.com", "long-enough-password", "视频负责人", "视频团队")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "金陵古都", ProductName: "金陵古都香", Channel: "douyin"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetTenantContentCapability(ctx, identitydomain.TenantContentCapability{TenantID: actor.TenantID, ContentType: identitydomain.ContentTypeMarketingVideo, Enabled: true, UpdatedBy: actor.UserID, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	admin, err := service.Work.AdminWorkOS(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var marketingSOP catalogdomain.SOPVersion
	for _, summary := range admin.SOPs {
		if summary.Definition.TemplateKey == "marketing_video_production" {
			marketingSOP = summary.Versions[0]
		}
	}
	if marketingSOP.ID == "" {
		t.Fatal("marketing video SOP was not provisioned")
	}
	environment, err := service.Work.CreateEnvironment(ctx, actor, application.SaveEnvironmentInput{Name: "营销视频环境", Slug: "marketing-video", Status: "active", DefaultSOPID: marketingSOP.SOPID, DefaultSOPVersion: marketingSOP.Version}, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.Work.CreateWorkTask(ctx, actor, application.CreateWorkTaskInput{ProjectID: project.ID, EnvironmentID: environment.ID, Title: "金陵古都香短视频", ContentType: identitydomain.ContentTypeMarketingVideo, InputRefs: []string{"brief:jinling-gudu"}}, "")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	sourceID := idgen.New()
	sourceRevision := sourcedomain.SourceRevision{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: project.ID, SourceID: sourceID, FileName: "jinling-gudu.md", ObjectKey: "sources/jinling-gudu.md", SHA256: strings.Repeat("1", 64), ByteSize: 128, DeclaredMIME: "text/markdown", DetectedMIME: "text/markdown", ProcessingStatus: "ready", UploadedBy: actor.UserID, CreatedAt: now}
	if err := store.CreateSource(ctx, sourcedomain.Source{ID: sourceID, TenantID: actor.TenantID, ProjectID: project.ID, Name: "金陵古都参考资料", SourceType: "document", Status: "ready", RevisionCount: 1, LatestRevision: sourceRevision.ID, CreatedAt: now}, sourceRevision); err != nil {
		t.Fatal(err)
	}
	knowledgeObject := sourcedomain.KnowledgeObject{ID: "fact:jinling-history", TenantID: actor.TenantID, ProjectID: project.ID, ObjectType: "FactAssertion", Layer: "product", Version: 1, Status: "approved", Title: "金陵文化表达", Statement: "仅使用已核验的南京历史文化表达。", Payload: map[string]any{"scope": "brand_story"}, AllowedChannels: []string{"douyin"}, EvidenceRefs: []string{"evidence:jinling"}, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	knowledgeObject.Digest, err = knowledgeObject.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateKnowledgeObject(ctx, knowledgeObject); err != nil {
		t.Fatal(err)
	}
	pack := sourcedomain.KnowledgePack{ID: "pack:jinling", TenantID: actor.TenantID, ProjectID: project.ID, Name: "金陵知识包", Purpose: "marketing_video", Version: 1, Status: "published", ObjectRefs: []sourcedomain.KnowledgePackObjectRef{{ObjectID: knowledgeObject.ID, Version: 1}}, QueryPolicy: sourcedomain.DefaultKnowledgeQueryPolicy(), CreatedBy: actor.UserID, PublishedBy: actor.UserID, CreatedAt: now, PublishedAt: &now}
	pack.Digest, err = pack.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateKnowledgePack(ctx, pack); err != nil {
		t.Fatal(err)
	}
	knowledgeSnapshot, err := sourcedomain.BuildKnowledgeSnapshot(pack, []sourcedomain.KnowledgeObject{knowledgeObject}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateKnowledgeSnapshot(ctx, knowledgeSnapshot); err != nil {
		t.Fatal(err)
	}
	task = startTaskStage(t, service, actor, task)
	task = reportTaskStage(t, service, actor, task, []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputSourceRevision, ObjectID: sourceRevision.ID, Role: catalogdomain.StageOutputRolePrimary}}, map[string]any{"source.registered": true})
	task = startTaskStage(t, service, actor, task)
	task = reportTaskStage(t, service, actor, task, []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputKnowledgeSnapshot, ObjectID: knowledgeSnapshot.ID, Role: catalogdomain.StageOutputRolePrimary}}, map[string]any{"claim.references": true, "rights.references": true})

	task = startTaskStage(t, service, actor, task)
	scriptBody, _ := json.Marshal(map[string]any{"title": "一缕金陵香，穿过六朝烟水", "scenes": []any{map[string]any{"scene": 1, "duration_seconds": 4, "visual": "明城墙晨光", "voiceover": "一座城，把时间藏进香气。"}, map[string]any{"scene": 2, "duration_seconds": 6, "visual": "香具与产品细节", "voiceover": "以经核验的金陵文化意象，讲述当代东方气息。"}}})
	scriptRevision, err := service.Work.CreateTaskRevision(ctx, actor, task.Task.ID, application.CreateTaskRevisionInput{ContentType: identitydomain.ContentTypeMarketingVideo, Content: scriptBody, KnowledgeSnapshotIDs: []string{knowledgeSnapshot.ID}, EvidenceSummary: map[string]any{"verified": true}, RightsSummary: map[string]any{"passed": true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	task = reportTaskStage(t, service, actor, task, []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputSubmissionRevision, ObjectID: scriptRevision.ID, ObjectVersion: scriptRevision.RevisionNo, Role: catalogdomain.StageOutputRolePrimary}}, map[string]any{"content.schema": true, "claim.references": true})
	task = approveCurrentGate(t, service, actor, task)
	contentSnapshots, err := service.Review.ApprovedSnapshots(ctx, actor, project.ID, "content_batch")
	if err != nil || len(contentSnapshots) != 1 {
		t.Fatalf("script gate did not create content_batch ApprovedSnapshot: snapshots=%#v err=%v", contentSnapshots, err)
	}
	contentSnapshot := contentSnapshots[0]
	if taskRevisions, err := store.TaskRevisions(ctx, actor.TenantID, task.Task.ID); err != nil || len(taskRevisions) != 0 {
		t.Fatalf("marketing video task wrote legacy TaskRevision: revisions=%#v err=%v", taskRevisions, err)
	}
	workspaceBinding, err := store.WorkspaceBinding(ctx, actor.TenantID, task.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if workspaceBinding.TemplateID != localworkspace.TemplateID || workspaceBinding.TemplateVersion != localworkspace.TemplateVersion {
		t.Fatalf("marketing video task created a stale workspace template binding: %#v", workspaceBinding)
	}

	task = startTaskStage(t, service, actor, task)
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	storyboardSnapshot := createStoryboardSubmission(t, service, store, actor, task, contentSnapshot, png, now)
	task = reportTaskStage(t, service, actor, task, []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputStoryboardPackage, ObjectID: storyboardSnapshot.ID, Role: catalogdomain.StageOutputRolePrimary}}, map[string]any{"storyboard.locked": true, "rights.references": true})
	task = approveCurrentGate(t, service, actor, task)
	if _, err := service.Delivery.UploadStoryboardArtifact(ctx, actor, task.Task.ID, application.UploadStoryboardArtifactInput{SnapshotID: storyboardSnapshot.ID, AssetID: "asset-first-frame", FileName: "first-frame.png", Body: png}, "golden-storyboard-asset"); err != nil {
		t.Fatal(err)
	}

	task = startTaskStage(t, service, actor, task)
	var storyboardEnvelope struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(storyboardSnapshot.CanonicalContent, &storyboardEnvelope); err != nil || len(storyboardEnvelope.Objects) != 1 {
		t.Fatalf("storyboard package missing from approved snapshot: %v", err)
	}
	var lockedStoryboard work.StoryboardPackage
	if err := json.Unmarshal(storyboardEnvelope.Objects[0], &lockedStoryboard); err != nil {
		t.Fatal(err)
	}
	promptPackage := work.SeedancePromptPackage{
		ID: "prompt-package:" + task.Task.ID, Type: "seedance_prompt_package", SchemaVersion: work.SeedancePromptPackageSchema,
		StoryboardSnapshotID: storyboardSnapshot.ID, StoryboardPackageID: lockedStoryboard.ID, StoryboardLockedDigest: lockedStoryboard.LockedDigest,
		Provider: "seedance", ProviderProfileVersion: "1.0.0", AdapterCapability: work.CapabilityRef{ID: "contentcloud.seedance-execution", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("c", 64)},
		Mode: "all_reference", Settings: work.SeedanceSettings{AspectRatio: "9:16", DurationSeconds: 15, Sound: "environment_only"},
		UploadManifest: []work.SeedanceUpload{{Reference: "@图片1", ArtifactID: "asset-first-frame", File: "first-frame.png", Purpose: "first_frame", SHA256: mediapipeline.SHA256(png)}},
		Segments:       []work.SeedanceSegment{{ID: "segment-1", Order: 1, StartMS: 0, EndMS: 4000, PromptZH: "@图片1 镜头向前推进", AcceptanceCriteria: []string{"首帧稳定"}}},
		Validation:     work.SeedanceValidation{ReferencesChecked: true, LimitsChecked: true, RightsChecked: true, OfferChecked: true, DigestChecked: true}, Status: "validated",
	}
	promptBody, err := json.Marshal(promptPackage)
	if err != nil {
		t.Fatal(err)
	}
	promptArtifact, err := service.Delivery.UploadSeedancePromptPackage(ctx, actor, task.Task.ID, application.UploadSeedancePromptPackageInput{SnapshotID: storyboardSnapshot.ID, FileName: "package.json", Body: promptBody}, "golden-seedance-prompt")
	if err != nil || promptArtifact.Kind != "prompt_package" || promptArtifact.MediaType != "application/json" {
		t.Fatalf("prompt package Artifact was not registered: %#v err=%v", promptArtifact, err)
	}
	job, err := service.Delivery.CreateMediaGenerationJob(ctx, actor, task.Task.ID, application.CreateMediaGenerationJobInput{StageRunID: currentRun(t, task).ID, StoryboardSnapshotID: storyboardSnapshot.ID, PromptPackageArtifactID: promptArtifact.ID, ProviderID: "fake", ProfileVersion: "1.0.0", Mode: "image_to_video", AspectRatio: "9:16", DurationSeconds: 15, IdempotencyKey: "golden-media-job"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Delivery.ProcessMediaGenerationJob(ctx, actor.TenantID, job.ID); err != nil {
		t.Fatal(err)
	}
	task, err = service.Work.WorkTask(ctx, actor, task.Task.ID)
	if err != nil || len(task.MediaReviews) != 2 || task.MediaJobs[0].State != deliverydomain.MediaJobSucceeded {
		t.Fatalf("media worker did not create canonical outputs: view=%#v err=%v", task, err)
	}
	artifact := findArtifact(t, task.Artifacts, "generated_video")
	job = task.MediaJobs[0]
	task = reportTaskStage(t, service, actor, task, []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputGenerationJob, ObjectID: job.ID, ObjectVersion: job.RowVersion, Role: catalogdomain.StageOutputRolePrimary}, {OutputType: catalogdomain.StageOutputArtifact, ObjectID: artifact.ID, Role: catalogdomain.StageOutputRolePreview}}, map[string]any{"media.technical": true, "cost.confirmed": true})

	task = startTaskStage(t, service, actor, task)
	contentReview := findMediaReview(t, task.MediaReviews, deliverydomain.MediaReviewContent)
	task, err = service.Delivery.DecideMediaReview(ctx, actor, contentReview.ID, application.MediaReviewDecisionInput{ExpectedVersion: contentReview.RowVersion, Decision: "approved", Reason: "画面与剧本一致", Selected: true, Checks: map[string]any{"media.content": true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	contentReview = findMediaReview(t, task.MediaReviews, deliverydomain.MediaReviewContent)
	task = reportTaskStage(t, service, actor, task, []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputMediaReview, ObjectID: contentReview.ID, ObjectVersion: contentReview.RowVersion, Role: catalogdomain.StageOutputRoleSelectedTake}}, map[string]any{"media.technical": true, "media.content": true})
	task = approveCurrentGate(t, service, actor, task)

	task = startTaskStage(t, service, actor, task)
	finalRender, err := service.Delivery.CreateFinalRender(ctx, actor, task.Task.ID, application.CreateFinalRenderInput{StageRunID: currentRun(t, task).ID, SelectedReviewID: contentReview.ID}, "")
	if err != nil || finalRender.Artifact.Kind != "final_render" || finalRender.Artifact.ID == artifact.ID {
		t.Fatalf("final render did not create an independent artifact: %#v err=%v", finalRender, err)
	}
	finalReview := finalRender.Review
	task, err = service.Delivery.DecideMediaReview(ctx, actor, finalReview.ID, application.MediaReviewDecisionInput{ExpectedVersion: finalReview.RowVersion, Decision: "approved", Reason: "最终成片批准", Selected: true, Checks: map[string]any{"media.final": true, "offer.valid": true, "rights.references": true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	finalReview = findMediaReview(t, task.MediaReviews, deliverydomain.MediaReviewFinal)
	task = reportTaskStage(t, service, actor, task, []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputArtifact, ObjectID: finalRender.Artifact.ID, Role: catalogdomain.StageOutputRoleFinal}, {OutputType: catalogdomain.StageOutputMediaReview, ObjectID: finalReview.ID, ObjectVersion: finalReview.RowVersion, Role: catalogdomain.StageOutputRoleFinal}}, map[string]any{"media.final": true, "offer.valid": true, "rights.references": true})
	task = approveCurrentGate(t, service, actor, task)

	task = startTaskStage(t, service, actor, task)
	deliveryPackage, err := service.Delivery.BuildTaskDeliveryPackage(ctx, actor, task.Task.ID, application.BuildTaskDeliveryPackageInput{FinalReviewID: finalReview.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	task = reportTaskStage(t, service, actor, task, []work.TaskStageOutput{{OutputType: catalogdomain.StageOutputDeliveryPackage, ObjectID: deliveryPackage.ID, Role: catalogdomain.StageOutputRoleFinal}}, map[string]any{"delivery.integrity": true})
	if task.Task.Status != work.TaskStatusAccepted {
		t.Fatalf("all marketing video stages should accept task: %#v", task.Task)
	}

	finalScriptRevisions, err := service.Work.WorkTaskRevisions(ctx, actor, task.Task.ID)
	if err != nil || len(finalScriptRevisions) != 1 || finalScriptRevisions[0].Status != reviewdomain.TaskRevisionAccepted {
		t.Fatalf("final script revision projection was not accepted: %#v err=%v", finalScriptRevisions, err)
	}
	deliver := true
	delivery, err := service.Work.CreateTaskDelivery(ctx, actor, task.Task.ID, application.CreateTaskDeliveryInput{RevisionID: finalScriptRevisions[0].ID, DeliveryPackageID: deliveryPackage.ID, Destination: "workspace", Deliver: &deliver}, "")
	if err != nil {
		t.Fatal(err)
	}
	if delivery.Status != deliverydomain.TaskDeliveryDelivered || delivery.IntegrityStatus != "complete" || len(delivery.Manifest) != 1 {
		t.Fatalf("delivery hard gate did not produce complete delivery: %#v", delivery)
	}
	finalView, err := service.Work.WorkTask(ctx, actor, task.Task.ID)
	if err != nil || finalView.Task.Status != work.TaskStatusDelivered || len(finalView.StageOutputs) != 10 || len(finalView.DeliveryPackages) != 1 || len(finalView.ProviderAttempts) != 1 {
		t.Fatalf("final task projection is incomplete: view=%#v err=%v", finalView, err)
	}
}

func createStoryboardSubmission(t *testing.T, service *application.Application, store *memory.Store, actor application.Actor, task application.WorkTaskView, contentSnapshot reviewdomain.ApprovedSnapshot, png []byte, now time.Time) reviewdomain.ApprovedSnapshot {
	t.Helper()
	var envelope struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(contentSnapshot.CanonicalContent, &envelope); err != nil || len(envelope.Objects) != 1 {
		t.Fatalf("content snapshot object missing: %#v err=%v", contentSnapshot, err)
	}
	sourceHash, err := stablehash.Sum(envelope.Objects[0])
	if err != nil {
		t.Fatal(err)
	}
	assetHash := mediapipeline.SHA256(png)
	storyboard := work.StoryboardPackage{
		ID: "storyboard:" + task.Task.ID, Type: "storyboard_package", SchemaVersion: work.StoryboardPackageSchema,
		ProjectID: task.Task.ProjectID, ApprovedSnapshotID: contentSnapshot.ID, ContentItemID: "content-item:" + task.Task.ID,
		GeneratorCapability: work.CapabilityRef{ID: "contentcloud.storyboard.generator", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("b", 64)},
		Status:              "review_ready", ReviewSheetArtifactID: "asset-review-sheet", SourceDigest: "sha256:" + sourceHash,
		RightsRefs: []string{"rights:jinling-gudu"},
		Shots:      []work.StoryboardShot{{ShotID: "shot-1", StartMS: 0, EndMS: 4000, Role: "hero", FirstFrameArtifactID: "asset-first-frame", ImagePromptZH: "明城墙晨光中的金陵古都香产品定帧", Subject: "金陵古都香", Product: "金陵古都香", Scene: "南京城墙晨光", Composition: "主体居中", Lighting: "自然晨光", Camera: "缓慢推进", Action: "香气随晨雾展开", IncomingState: "抽屉关闭", OutgoingState: "抽屉打开", MovementAxis: "前后", LightingLock: "暖色晨光", ProductLock: "产品包装保持一致", Anchors: []string{"南京城墙"}, AssetRefs: []string{"asset-first-frame"}, RightsRefs: []string{"rights:jinling-gudu"}, KnowledgeRefs: []string{"fact:jinling-history"}, NegativeConstraints: []string{"不虚构历史事实"}, AcceptanceCriteria: []string{"首帧清晰可识别"}, PlanB: "若晨雾不足，保持城墙与产品构图"}},
		Assets: []work.StoryboardAsset{
			{ID: "asset-first-frame", Role: "first_frame", ShotID: "shot-1", Path: "50-production/media/shot-1/first-frame.png", MediaType: "image/png", SHA256: assetHash, ByteSize: int64(len(png)), RightsRefs: []string{"rights:jinling-gudu"}},
			{ID: "asset-review-sheet", Role: "review_sheet", Path: "50-production/media/review-sheet.png", MediaType: "image/png", SHA256: assetHash, ByteSize: int64(len(png)), RightsRefs: []string{"rights:jinling-gudu"}},
		},
	}
	storyboard.LockedDigest, err = storyboard.ComputedLockedDigest()
	if err != nil {
		t.Fatal(err)
	}
	object, err := reviewdomain.NewSubmissionObjectRef(storyboard.ID, "storyboard_package", 1, "40-storyboard/packages/"+storyboard.ID+".json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := store.WorkspaceBinding(t.Context(), actor.TenantID, task.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspaceActor := application.Actor{TenantID: binding.TenantID, WorkspaceID: binding.ID, Type: "workspace", Role: "workspace"}
	bundle := reviewdomain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "storyboard", ProjectID: binding.ProjectID, WorkspaceID: binding.ID, BaseSnapshotIDs: []string{contentSnapshot.ID}, EnvironmentDigest: task.Task.SOPDigest, Objects: []reviewdomain.SubmissionObjectRef{object}, SourceDisclosures: []reviewdomain.SourceDisclosure{}, Artifacts: []reviewdomain.SubmissionArtifact{}, LocalRunSummary: reviewdomain.LocalRunSummary{Stage: "storyboard", Checks: []reviewdomain.LocalRunCheck{{Name: "storyboard.locked", Status: "passed"}}}, IdempotencyKey: "task-storyboard:" + task.Task.ID}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	revision, err := service.Review.CreateSubmission(t.Context(), workspaceActor, binding, bundle, "golden-storyboard-submission")
	if err != nil {
		t.Fatal(err)
	}
	approval, err := service.Review.ApproveSubmission(t.Context(), actor, revision.ID, "分镜锁定", "golden-storyboard-approve")
	if err != nil || approval.ApprovedSnapshot == nil {
		t.Fatalf("storyboard submission was not approved: %#v err=%v", approval, err)
	}
	return *approval.ApprovedSnapshot
}

func startTaskStage(t *testing.T, service *application.Application, actor application.Actor, task application.WorkTaskView) application.WorkTaskView {
	t.Helper()
	value, err := service.Work.TaskAction(t.Context(), actor, task.Task.ID, application.TaskActionInput{Action: "start"}, "")
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func reportTaskStage(t *testing.T, service *application.Application, actor application.Actor, task application.WorkTaskView, outputs []work.TaskStageOutput, checks map[string]any) application.WorkTaskView {
	t.Helper()
	run := currentRun(t, task)
	value, err := service.Work.ReportStage(t.Context(), actor, task.Task.ID, application.StageReportInput{StageRunID: run.ID, StageID: run.StageID, Status: work.StageRunStatusCompleted, Outputs: outputs, Checks: checks}, "")
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func approveCurrentGate(t *testing.T, service *application.Application, actor application.Actor, task application.WorkTaskView) application.WorkTaskView {
	t.Helper()
	for _, gate := range task.Gates {
		if gate.Status != reviewdomain.GateEvaluationPending {
			continue
		}
		value, err := service.Work.DecideGate(t.Context(), actor, task.Task.ID, gate.ID, application.GateDecisionInput{Decision: "approved", Reason: "Golden Journey 批准"}, "")
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	t.Fatal("pending gate not found")
	return application.WorkTaskView{}
}

func currentRun(t *testing.T, task application.WorkTaskView) work.StageRun {
	t.Helper()
	for _, run := range task.StageRuns {
		if run.StageID == task.Task.CurrentStageID {
			return run
		}
	}
	t.Fatalf("current run %s not found", task.Task.CurrentStageID)
	return work.StageRun{}
}

func findMediaReview(t *testing.T, reviews []deliverydomain.MediaReview, kind string) deliverydomain.MediaReview {
	t.Helper()
	for _, review := range reviews {
		if review.ReviewKind == kind {
			return review
		}
	}
	t.Fatalf("media review %s not found", kind)
	return deliverydomain.MediaReview{}
}

func findArtifact(t *testing.T, artifacts []deliverydomain.Artifact, kind string) deliverydomain.Artifact {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact
		}
	}
	t.Fatalf("artifact %s not found", kind)
	return deliverydomain.Artifact{}
}
