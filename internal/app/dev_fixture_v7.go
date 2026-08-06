package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/fixturev3"
	"github.com/limecloud/contentcloud/internal/mediapipeline"
)

const (
	marketingVideoDemoFixtureVersion = "7.0.0"
	marketingVideoDemoIdempotencyKey = "fixture:v7:jinling-gudu:marketing-video"
)

type MarketingVideoDemoFixtureResult struct {
	FixtureVersion string         `json:"fixture_version"`
	Project        domain.Project `json:"project"`
	Task           WorkTaskView   `json:"task"`
	Reused         bool           `json:"reused"`
}

// EnsureMarketingVideoDemoFixture materializes a complete V7 journey through
// canonical server objects. It is invoked only by the development HTTP route.
func (s *Service) EnsureMarketingVideoDemoFixture(ctx context.Context, actor Actor, requestID string) (MarketingVideoDemoFixtureResult, error) {
	if actor.Role != "tenant_admin" {
		return MarketingVideoDemoFixtureResult{}, domain.Policy("ROLE_DENIED", "只有租户管理员可以导入开发演示数据", "切换到租户管理员账号")
	}
	projects, err := s.Projects(ctx, actor)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	var project domain.Project
	for _, candidate := range projects {
		if candidate.BrandName == "金陵古都" && candidate.ProductName == "金陵古都香" && candidate.ContentType == domain.ContentTypeMarketingVideo {
			project = candidate
			break
		}
	}
	if err := s.store.SetTenantContentCapability(ctx, domain.TenantContentCapability{TenantID: actor.TenantID, ContentType: domain.ContentTypeMarketingVideo, Enabled: true, UpdatedBy: actor.UserID, UpdatedAt: s.now().UTC()}); err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	if project.ID == "" {
		project, err = s.CreateProject(ctx, actor, CreateProjectInput{
			BrandName: "金陵古都", ProductName: "金陵古都香", ContentType: domain.ContentTypeMarketingVideo,
			Channel: "douyin", StageObjective: "完成 26 秒营销短视频的剧本、分镜、生成、质检与交付",
			OwnerName: "内容制片", ReviewerName: "品牌审核", ClientApprover: "客户决定人",
		}, fixtureRequestID(requestID, "project"))
		if err != nil {
			return MarketingVideoDemoFixtureResult{}, err
		}
	}
	if _, _, err := s.ensureFixtureWorkspace(ctx, actor, project, fixturev3.WorkspaceSpec{
		TemplateID: "workspace_marketing_video", TemplateVersion: marketingVideoDemoFixtureVersion,
		Targets: []string{"codex"}, DeviceName: "V7 演示创作环境",
	}, fixtureRequestID(requestID, "workspace")); err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	project, err = s.Project(ctx, actor, project.ID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}

	tasks, err := s.WorkTasks(ctx, actor, project.ID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	for _, candidate := range tasks {
		if candidate.IdempotencyKey != marketingVideoDemoIdempotencyKey {
			continue
		}
		view, viewErr := s.WorkTask(ctx, actor, candidate.ID)
		if viewErr != nil {
			return MarketingVideoDemoFixtureResult{}, viewErr
		}
		if view.Task.Status != domain.TaskStatusDelivered {
			return MarketingVideoDemoFixtureResult{}, domain.Policy("DEV_FIXTURE_V7_INCOMPLETE", "已有 V7 演示任务未完整收敛", "检查任务当前流程阶段后重试开发环境初始化")
		}
		return MarketingVideoDemoFixtureResult{FixtureVersion: marketingVideoDemoFixtureVersion, Project: project, Task: view, Reused: true}, nil
	}

	sourceRevision, evidenceIDs, err := s.createMarketingVideoDemoSource(ctx, actor, project)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	knowledgeSnapshot, knowledgeIDs, rightsID, err := s.createMarketingVideoDemoKnowledge(ctx, actor, project, sourceRevision, evidenceIDs)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err := s.CreateWorkTask(ctx, actor, CreateWorkTaskInput{
		ProjectID: project.ID, Title: "别把南京放回抽屉｜26 秒短视频成片", Intent: "marketing_video",
		ContentType: domain.ContentTypeMarketingVideo, InputRefs: []string{sourceRevision.ID, knowledgeSnapshot.ID},
		RequestedOutput: map[string]any{"aspect_ratio": "9:16", "duration_seconds": 26, "deliverable": "final_video"},
		Priority:        "high", RiskProfile: "medium", IdempotencyKey: marketingVideoDemoIdempotencyKey,
	}, fixtureRequestID(requestID, "task"))
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}

	task, err = s.startDemoStage(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.reportDemoStage(ctx, actor, task, []domain.TaskStageOutput{{OutputType: domain.StageOutputSourceRevision, ObjectID: sourceRevision.ID, Role: domain.StageOutputRolePrimary}}, map[string]any{"source.registered": true}, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}

	task, err = s.startDemoStage(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.reportDemoStage(ctx, actor, task, []domain.TaskStageOutput{{OutputType: domain.StageOutputKnowledgeSnapshot, ObjectID: knowledgeSnapshot.ID, Role: domain.StageOutputRolePrimary}}, map[string]any{"claim.references": true, "rights.references": true}, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}

	task, err = s.startDemoStage(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	scriptRevision, err := s.CreateTaskRevision(ctx, actor, task.Task.ID, CreateTaskRevisionInput{
		ContentType: domain.ContentTypeMarketingVideo, SchemaVersion: "contentcloud.marketing_video_script/1.0",
		Content: marketingVideoDemoScript(), KnowledgeSnapshotIDs: []string{knowledgeSnapshot.ID},
		EvidenceSummary: map[string]any{"source_revision_ids": []string{sourceRevision.ID}, "evidence_ids": evidenceIDs, "reference_workspace": "marketing/jinling-gudu"},
		RightsSummary:   map[string]any{"checked": true, "rights_refs": []string{rightsID}, "restriction": "开发演示素材，不得直接投放"},
	}, fixtureRequestID(requestID, "script"))
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.reportDemoStage(ctx, actor, task, []domain.TaskStageOutput{{OutputType: domain.StageOutputSubmissionRevision, ObjectID: scriptRevision.ID, ObjectVersion: scriptRevision.RevisionNo, Role: domain.StageOutputRolePrimary}}, map[string]any{"content.schema": true, "claim.references": true}, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.approveDemoGate(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	contentSnapshot, err := s.demoContentSnapshot(ctx, actor, project.ID, scriptRevision.ContentHash)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}

	task, err = s.startDemoStage(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	storyboardSnapshot, storyboardAssets, err := s.createMarketingVideoDemoStoryboard(ctx, actor, task, contentSnapshot, knowledgeIDs, rightsID, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	for _, asset := range storyboardAssets {
		if _, err := s.UploadStoryboardArtifact(ctx, actor, task.Task.ID, UploadStoryboardArtifactInput{SnapshotID: storyboardSnapshot.ID, AssetID: asset.Asset.ID, FileName: asset.Asset.Path, Body: asset.Body}, fixtureRequestID(requestID, "storyboard-asset")); err != nil {
			return MarketingVideoDemoFixtureResult{}, err
		}
	}
	task, err = s.reportDemoStage(ctx, actor, task, []domain.TaskStageOutput{{OutputType: domain.StageOutputStoryboardPackage, ObjectID: storyboardSnapshot.ID, Role: domain.StageOutputRolePrimary}}, map[string]any{"storyboard.locked": true, "rights.references": true}, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.approveDemoGate(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}

	task, err = s.startDemoStage(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	job, err := s.CreateMediaGenerationJob(ctx, actor, task.Task.ID, CreateMediaGenerationJobInput{
		StageRunID: demoCurrentRun(task).ID, StoryboardSnapshotID: storyboardSnapshot.ID, ProviderID: "fake", ProfileVersion: "1.0.0",
		Mode: "image_to_video", AspectRatio: "9:16", DurationSeconds: 26, IdempotencyKey: marketingVideoDemoIdempotencyKey + ":media",
	}, fixtureRequestID(requestID, "media-job"))
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.waitForDemoMedia(ctx, actor, task.Task.ID, job.ID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	generatedArtifact, err := demoArtifact(task.Artifacts, "generated_video")
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	job = task.MediaJobs[0]
	task, err = s.reportDemoStage(ctx, actor, task, []domain.TaskStageOutput{{OutputType: domain.StageOutputGenerationJob, ObjectID: job.ID, ObjectVersion: job.RowVersion, Role: domain.StageOutputRolePrimary}, {OutputType: domain.StageOutputArtifact, ObjectID: generatedArtifact.ID, Role: domain.StageOutputRolePreview}}, map[string]any{"media.technical": true, "cost.confirmed": true}, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}

	task, err = s.startDemoStage(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	contentReview, err := demoMediaReview(task.MediaReviews, domain.MediaReviewContent)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.DecideMediaReview(ctx, actor, contentReview.ID, MediaReviewDecisionInput{ExpectedVersion: contentReview.RowVersion, Decision: "approved", Reason: "画面遵守真实产品占位、无功效与无未授权地标边界", Selected: true, Checks: map[string]any{"media.content": true, "script.alignment": true, "rights.references": true}}, fixtureRequestID(requestID, "take-review"))
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	contentReview, err = demoMediaReview(task.MediaReviews, domain.MediaReviewContent)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.reportDemoStage(ctx, actor, task, []domain.TaskStageOutput{{OutputType: domain.StageOutputMediaReview, ObjectID: contentReview.ID, ObjectVersion: contentReview.RowVersion, Role: domain.StageOutputRoleSelectedTake}}, map[string]any{"media.technical": true, "media.content": true}, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.approveDemoGate(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}

	task, err = s.startDemoStage(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	finalRender, err := s.CreateFinalRender(ctx, actor, task.Task.ID, CreateFinalRenderInput{StageRunID: demoCurrentRun(task).ID, SelectedReviewID: contentReview.ID}, fixtureRequestID(requestID, "final-render"))
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.DecideMediaReview(ctx, actor, finalRender.Review.ID, MediaReviewDecisionInput{ExpectedVersion: finalRender.Review.RowVersion, Decision: "approved", Reason: "最终成片、CTA、权利边界和交付摘要均已核验", Selected: true, Checks: map[string]any{"media.final": true, "offer.valid": true, "rights.references": true}}, fixtureRequestID(requestID, "final-review"))
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	finalReview, err := demoMediaReview(task.MediaReviews, domain.MediaReviewFinal)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.reportDemoStage(ctx, actor, task, []domain.TaskStageOutput{{OutputType: domain.StageOutputArtifact, ObjectID: finalRender.Artifact.ID, Role: domain.StageOutputRoleFinal}, {OutputType: domain.StageOutputMediaReview, ObjectID: finalReview.ID, ObjectVersion: finalReview.RowVersion, Role: domain.StageOutputRoleFinal}}, map[string]any{"media.final": true, "offer.valid": true, "rights.references": true}, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.approveDemoGate(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}

	task, err = s.startDemoStage(ctx, actor, task, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	deliveryPackage, err := s.BuildTaskDeliveryPackage(ctx, actor, task.Task.ID, BuildTaskDeliveryPackageInput{FinalReviewID: finalReview.ID}, fixtureRequestID(requestID, "delivery-package"))
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	task, err = s.reportDemoStage(ctx, actor, task, []domain.TaskStageOutput{{OutputType: domain.StageOutputDeliveryPackage, ObjectID: deliveryPackage.ID, Role: domain.StageOutputRoleFinal}}, map[string]any{"delivery.integrity": true}, requestID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	revisions, err := s.WorkTaskRevisions(ctx, actor, task.Task.ID)
	if err != nil || len(revisions) == 0 {
		if err == nil {
			err = domain.NotFound("已接受的剧本内容版本")
		}
		return MarketingVideoDemoFixtureResult{}, err
	}
	if _, err := s.CreateTaskDelivery(ctx, actor, task.Task.ID, CreateTaskDeliveryInput{RevisionID: revisions[len(revisions)-1].ID, DeliveryPackageID: deliveryPackage.ID, Destination: "workspace"}, fixtureRequestID(requestID, "delivery")); err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	finalView, err := s.WorkTask(ctx, actor, task.Task.ID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	project, err = s.Project(ctx, actor, project.ID)
	if err != nil {
		return MarketingVideoDemoFixtureResult{}, err
	}
	return MarketingVideoDemoFixtureResult{FixtureVersion: marketingVideoDemoFixtureVersion, Project: project, Task: finalView}, nil
}

func (s *Service) createMarketingVideoDemoSource(ctx context.Context, actor Actor, project domain.Project) (domain.SourceRevision, []string, error) {
	now := s.now().UTC()
	body := []byte(marketingVideoDemoSource)
	sourceID, revisionID := domain.NewID(), domain.NewID()
	objectKey := fmt.Sprintf("sources/%s/%s/jinling-gudu-v7.md", actor.TenantID, revisionID)
	if err := s.blobs.Put(ctx, objectKey, body); err != nil {
		return domain.SourceRevision{}, nil, err
	}
	revision := domain.SourceRevision{ID: revisionID, TenantID: actor.TenantID, ProjectID: project.ID, SourceID: sourceID, FileName: "jinling-gudu-v7-evidence.md", ObjectKey: objectKey, SHA256: mediapipeline.SHA256(body), ByteSize: int64(len(body)), DeclaredMIME: "text/markdown", DetectedMIME: "text/markdown", ProcessingStatus: "ready", ParserVersion: "dev-fixture-v7", UploadedBy: actor.UserID, CreatedAt: now}
	if err := s.store.CreateSource(ctx, domain.Source{ID: sourceID, TenantID: actor.TenantID, ProjectID: project.ID, Name: "金陵古都香 V7 验收证据包", SourceType: "document", Status: "ready", RevisionCount: 1, LatestRevision: revision.ID, CreatedAt: now}, revision); err != nil {
		return domain.SourceRevision{}, nil, err
	}
	quotes := []string{
		"旅行纪念品从抽屉回到日常，唯一测试变量为收纳逆转钩子。",
		"产品只允许使用完成版本和权利核验的真实素材，不生成或替换包装、标志和标签。",
		"不写价格、成分、历史起源、功效、权威背书、固定体验时长或交通携带结论。",
		"唯一 CTA：你会把哪座城市带回日常？",
	}
	ids := make([]string, 0, len(quotes))
	for index, quote := range quotes {
		id := domain.NewID()
		span := domain.EvidenceSpan{ID: id, TenantID: actor.TenantID, ProjectID: project.ID, RevisionID: revision.ID, LocatorKind: "markdown", Locator: map[string]any{"section": index + 1, "source_ref": "marketing/jinling-gudu/outputs/scripts/jinling-douyin-20260803/11-drawer-reversal.md"}, QuoteText: quote, QuoteHash: mediapipeline.SHA256([]byte(quote)), ReviewStatus: "approved", ReviewedBy: actor.UserID, ReviewedAt: &now, CreatedAt: now}
		if err := s.store.CreateEvidence(ctx, span); err != nil {
			return domain.SourceRevision{}, nil, err
		}
		ids = append(ids, id)
	}
	return revision, ids, nil
}

func (s *Service) createMarketingVideoDemoKnowledge(ctx context.Context, actor Actor, project domain.Project, source domain.SourceRevision, evidenceIDs []string) (domain.KnowledgeSnapshot, []string, string, error) {
	now := s.now().UTC()
	asset := domain.Asset{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, Name: "V7 分镜开发示意素材", AssetType: "storyboard_fixture", SourceRevisionID: source.ID, UsageMode: "development_fixture", Status: "active", CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateAsset(ctx, asset); err != nil {
		return domain.KnowledgeSnapshot{}, nil, "", err
	}
	rights := domain.RightsRecord{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, AssetID: asset.ID, RightsHolder: "Content Work OS 开发演示环境", RightsType: "internal_demo", Territories: []string{"CN"}, Channels: []string{"internal_demo"}, ValidFrom: &now, ProofSourceRevisionID: source.ID, Restrictions: []string{"仅用于本地开发演示", "不得作为金陵古都香正式产品素材投放"}, Status: "valid", ReviewedBy: actor.UserID, ReviewedAt: &now, RowVersion: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateRightsRecord(ctx, rights); err != nil {
		return domain.KnowledgeSnapshot{}, nil, "", err
	}
	type knowledgeSpec struct {
		objectType string
		layer      string
		status     string
		title      string
		statement  string
		payload    map[string]any
		evidence   []string
		rights     []string
	}
	specs := []knowledgeSpec{
		{"FactAssertion", "product", "verified", "项目工作对象", "本任务工作对象名称为“金陵古都香”；该名称仅用于项目识别，不自动批准价格、成分、规格或历史主张。", map[string]any{"subject": "sku:jinling-gudu-incense", "truth_boundary": "project_identity_only"}, evidenceIDs[:1], nil},
		{"Scenario", "market", "approved", "旅行记忆回到日常", "面向周末书桌场景，用“从抽屉取回旅行记忆”的动作建立收纳逆转钩子。", map[string]any{"audience": "城市白领候选", "scenario": "home_weekend", "hook": "drawer_reversal", "single_variable": true}, []string{evidenceIDs[0], evidenceIDs[3]}, nil},
		{"ConstraintRecord", "compliance", "active", "内容禁写边界", "不得写价格、成分、历史起源、功效、权威背书、固定体验时长或交通携带结论。", map[string]any{"risk_level": "blocked", "applies_to": []string{"script", "storyboard", "final_video"}}, []string{evidenceIDs[2]}, nil},
		{"RightsRecord", "compliance", "valid", "开发示意素材权利边界", "分镜使用系统生成的开发示意图；不得替代真实产品包装、品牌标志或客户已签批素材。", map[string]any{"rights_record_id": rights.ID, "usage": "internal_demo"}, []string{evidenceIDs[1]}, []string{rights.ID}},
	}
	objects := make([]domain.KnowledgeObject, 0, len(specs))
	refs := make([]domain.KnowledgePackObjectRef, 0, len(specs))
	ids := make([]string, 0, len(specs))
	for index, spec := range specs {
		object := domain.KnowledgeObject{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, ObjectType: spec.objectType, Layer: spec.layer, Version: 1, Status: spec.status, Title: spec.title, Statement: spec.statement, Payload: spec.payload, Dimensions: []string{fmt.Sprintf("v7_demo_%d", index+1)}, AllowedChannels: []string{"douyin"}, EvidenceRefs: spec.evidence, RightsRefs: spec.rights, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
		digest, err := object.ContentDigest()
		if err != nil {
			return domain.KnowledgeSnapshot{}, nil, "", err
		}
		object.Digest = digest
		if err := s.store.CreateKnowledgeObject(ctx, object); err != nil {
			return domain.KnowledgeSnapshot{}, nil, "", err
		}
		objects = append(objects, object)
		refs = append(refs, domain.KnowledgePackObjectRef{ObjectID: object.ID, Version: object.Version})
		ids = append(ids, object.ID)
	}
	pack := domain.KnowledgePack{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, Name: "金陵古都香｜抽屉逆转 V7 知识包", Purpose: "marketing_video", Version: 1, Status: "published", ObjectRefs: refs, QueryPolicy: domain.DefaultKnowledgeQueryPolicy(), CreatedBy: actor.UserID, PublishedBy: actor.UserID, CreatedAt: now, PublishedAt: &now}
	digest, err := pack.ContentDigest()
	if err != nil {
		return domain.KnowledgeSnapshot{}, nil, "", err
	}
	pack.Digest = digest
	if err := s.store.CreateKnowledgePack(ctx, pack); err != nil {
		return domain.KnowledgeSnapshot{}, nil, "", err
	}
	snapshot, err := domain.BuildKnowledgeSnapshot(pack, objects, now)
	if err != nil {
		return domain.KnowledgeSnapshot{}, nil, "", err
	}
	if err := s.store.CreateKnowledgeSnapshot(ctx, snapshot); err != nil {
		return domain.KnowledgeSnapshot{}, nil, "", err
	}
	return snapshot, ids, rights.ID, nil
}

type demoStoryboardAsset struct {
	Asset domain.StoryboardAsset
	Body  []byte
}

func (s *Service) createMarketingVideoDemoStoryboard(ctx context.Context, actor Actor, task WorkTaskView, contentSnapshot domain.ApprovedSnapshot, knowledgeIDs []string, rightsID, requestID string) (domain.ApprovedSnapshot, []demoStoryboardAsset, error) {
	var envelope struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(contentSnapshot.CanonicalContent, &envelope); err != nil || len(envelope.Objects) != 1 {
		return domain.ApprovedSnapshot{}, nil, domain.Invalid("DEV_FIXTURE_CONTENT_SNAPSHOT_INVALID", "剧本批准快照缺少唯一的内容项")
	}
	sourceHash, err := domain.CanonicalHash(envelope.Objects[0])
	if err != nil {
		return domain.ApprovedSnapshot{}, nil, err
	}
	type shotSpec struct {
		startMS, endMS                                  int
		role, scene, action, camera, incoming, outgoing string
	}
	shots := []shotSpec{
		{0, 3000, "hook", "周末书桌的抽屉打开，旅行票根露出", "手停在准备合上抽屉的动作上", "固定俯拍，轻微推进", "票根在抽屉内", "手停住"},
		{3000, 7000, "friction", "票根被推向抽屉，桌面工作通知亮起", "日常节奏重新覆盖旅行记忆", "桌面侧俯拍", "抽屉打开", "票根靠近抽屉"},
		{7000, 12000, "bridge", "手把真实产品素材占位留在桌角", "只建立一个看得见的位置，不替换包装", "中近景缓慢横移", "桌角留空", "产品占位进入"},
		{12000, 18000, "proof", "票根靠在产品占位旁形成稳定构图", "把一段南京放回日常动作", "固定机位，焦点从票根到产品", "票根分离", "票根与产品并置"},
		{18000, 23000, "resolution", "同一光线下合上电脑，桌面保留旅行物件", "工作结束，记忆没有被收回抽屉", "中景锁定", "电脑打开", "电脑合上"},
		{23000, 26000, "cta", "桌面近景停留，保留评论引导空间", "你会把哪座城市带回日常？", "近景静止 3 秒", "完整桌面", "CTA 留白"},
	}
	storyboardShots := make([]domain.StoryboardShot, 0, len(shots))
	assets := make([]demoStoryboardAsset, 0, len(shots)+1)
	for index, spec := range shots {
		body, imageErr := marketingVideoDemoFrame(index)
		if imageErr != nil {
			return domain.ApprovedSnapshot{}, nil, imageErr
		}
		shotID := fmt.Sprintf("shot-%02d", index+1)
		assetID := fmt.Sprintf("asset-%s-first-frame", shotID)
		asset := domain.StoryboardAsset{ID: assetID, Role: "first_frame", ShotID: shotID, Path: fmt.Sprintf("50-production/media/%s/first-frame.png", shotID), MediaType: "image/png", SHA256: mediapipeline.SHA256(body), ByteSize: int64(len(body)), RightsRefs: []string{rightsID}}
		assets = append(assets, demoStoryboardAsset{Asset: asset, Body: body})
		storyboardShots = append(storyboardShots, domain.StoryboardShot{ShotID: shotID, StartMS: spec.startMS, EndMS: spec.endMS, Role: spec.role, FirstFrameArtifactID: assetID, ImagePromptZH: spec.scene + "；仅作构图示意，不生成产品包装、标志或可读标签。", Subject: "周末书桌与旅行票根", Product: "金陵古都香真实素材占位", Scene: spec.scene, Composition: "9:16 竖屏，桌面主体位于中下三分区", Lighting: "同一扇窗的自然侧光", Camera: spec.camera, Action: spec.action, IncomingState: spec.incoming, OutgoingState: spec.outgoing, MovementAxis: "桌面纵深轴", LightingLock: "全片保持中性自然侧光", ProductLock: "不生成或修改包装、标志、标签", Anchors: []string{"旅行票根", "周末书桌"}, AssetRefs: []string{assetID}, RightsRefs: []string{rightsID}, KnowledgeRefs: append([]string(nil), knowledgeIDs...), NegativeConstraints: []string{"不生成产品包装和品牌标志", "不出现未授权南京地标", "不展示点燃、烟量或功效"}, AcceptanceCriteria: []string{"动作与剧本时间段一致", "产品只以审核素材占位表达", "画面保留 9:16 安全区"}, PlanB: "若产品素材未完成签批，仅保留中性占位和票根动作，不输出对外成片。"})
	}
	reviewBody, err := marketingVideoDemoFrame(len(shots))
	if err != nil {
		return domain.ApprovedSnapshot{}, nil, err
	}
	reviewAsset := domain.StoryboardAsset{ID: "asset-review-sheet", Role: "review_sheet", Path: "50-production/media/review-sheet.png", MediaType: "image/png", SHA256: mediapipeline.SHA256(reviewBody), ByteSize: int64(len(reviewBody)), RightsRefs: []string{rightsID}}
	assets = append(assets, demoStoryboardAsset{Asset: reviewAsset, Body: reviewBody})
	storyboardAssets := make([]domain.StoryboardAsset, 0, len(assets))
	for _, value := range assets {
		storyboardAssets = append(storyboardAssets, value.Asset)
	}
	storyboard := domain.StoryboardPackage{ID: "storyboard:" + task.Task.ID, Type: "storyboard_package", SchemaVersion: domain.StoryboardPackageSchema, ProjectID: task.Task.ProjectID, ApprovedSnapshotID: contentSnapshot.ID, ContentItemID: "content-item:" + task.Task.ID, GeneratorCapability: domain.CapabilityRef{ID: "contentcloud.storyboard.fixture", Version: marketingVideoDemoFixtureVersion, Digest: "sha256:" + strings.Repeat("b", 64)}, Status: "review_ready", Shots: storyboardShots, Assets: storyboardAssets, ReviewSheetArtifactID: reviewAsset.ID, RightsRefs: []string{rightsID}, SourceDigest: "sha256:" + sourceHash}
	storyboard.LockedDigest, err = storyboard.ComputedLockedDigest()
	if err != nil {
		return domain.ApprovedSnapshot{}, nil, err
	}
	object, err := domain.NewSubmissionObjectRef(storyboard.ID, "storyboard_package", 1, "40-storyboard/packages/"+storyboard.ID+".json", storyboard)
	if err != nil {
		return domain.ApprovedSnapshot{}, nil, err
	}
	binding, err := s.store.WorkspaceBinding(ctx, actor.TenantID, task.Task.ID)
	if err != nil {
		return domain.ApprovedSnapshot{}, nil, err
	}
	workspaceActor := Actor{TenantID: binding.TenantID, WorkspaceID: binding.ID, Type: "workspace", Role: "workspace"}
	bundle := domain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "storyboard", ProjectID: binding.ProjectID, WorkspaceID: binding.ID, BaseSnapshotIDs: []string{contentSnapshot.ID}, EnvironmentDigest: task.Task.SOPDigest, Objects: []domain.SubmissionObjectRef{object}, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Stage: "storyboard", Checks: []domain.LocalRunCheck{{Name: "storyboard.locked", Status: "passed"}, {Name: "rights.references", Status: "passed"}}}, IdempotencyKey: marketingVideoDemoIdempotencyKey + ":storyboard"}
	if err := bundle.SetComputedHash(); err != nil {
		return domain.ApprovedSnapshot{}, nil, err
	}
	revision, err := s.CreateSubmission(ctx, workspaceActor, binding, bundle, fixtureRequestID(requestID, "storyboard-submission"))
	if err != nil {
		return domain.ApprovedSnapshot{}, nil, err
	}
	approval, err := s.ApproveSubmission(ctx, actor, revision.ID, "分镜结构、素材摘要和权利边界已锁定", fixtureRequestID(requestID, "storyboard-approval"))
	if err != nil || approval.ApprovedSnapshot == nil {
		if err == nil {
			err = domain.NotFound("分镜已批准快照")
		}
		return domain.ApprovedSnapshot{}, nil, err
	}
	return *approval.ApprovedSnapshot, assets, nil
}

func (s *Service) startDemoStage(ctx context.Context, actor Actor, task WorkTaskView, requestID string) (WorkTaskView, error) {
	return s.TaskAction(ctx, actor, task.Task.ID, TaskActionInput{Action: "start"}, fixtureRequestID(requestID, "start-"+task.Task.CurrentStageID))
}

func (s *Service) reportDemoStage(ctx context.Context, actor Actor, task WorkTaskView, outputs []domain.TaskStageOutput, checks map[string]any, requestID string) (WorkTaskView, error) {
	run := demoCurrentRun(task)
	if run.ID == "" {
		return WorkTaskView{}, domain.NotFound("当前流程阶段执行记录")
	}
	return s.ReportStage(ctx, actor, task.Task.ID, StageReportInput{StageRunID: run.ID, StageID: run.StageID, Status: domain.StageRunStatusCompleted, Outputs: outputs, Checks: checks}, fixtureRequestID(requestID, "report-"+run.StageID))
}

func (s *Service) approveDemoGate(ctx context.Context, actor Actor, task WorkTaskView, requestID string) (WorkTaskView, error) {
	for _, gate := range task.Gates {
		if gate.Status == domain.GateEvaluationPending {
			return s.DecideGate(ctx, actor, task.Task.ID, gate.ID, GateDecisionInput{Decision: "approved", Reason: "V7 开发演示审核通过"}, fixtureRequestID(requestID, "gate-"+gate.GateID))
		}
	}
	return WorkTaskView{}, domain.NotFound("待处理的检查与审批项")
}

func (s *Service) demoContentSnapshot(ctx context.Context, actor Actor, projectID, contentHash string) (domain.ApprovedSnapshot, error) {
	snapshots, err := s.ApprovedSnapshots(ctx, actor, projectID, "content_batch")
	if err != nil {
		return domain.ApprovedSnapshot{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.ContentHash == contentHash {
			return snapshot, nil
		}
	}
	return domain.ApprovedSnapshot{}, domain.NotFound("剧本已批准快照")
}

func (s *Service) waitForDemoMedia(ctx context.Context, actor Actor, taskID, jobID string) (WorkTaskView, error) {
	for attempt := 0; attempt < 80; attempt++ {
		job, err := s.store.MediaGenerationJob(ctx, actor.TenantID, jobID)
		if err != nil {
			return WorkTaskView{}, err
		}
		switch job.State {
		case domain.MediaJobQueued, domain.MediaJobRetryWait:
			if err := s.ProcessMediaGenerationJob(ctx, actor.TenantID, job.ID); err != nil {
				return WorkTaskView{}, err
			}
		case domain.MediaJobSucceeded:
			return s.WorkTask(ctx, actor, taskID)
		case domain.MediaJobFailed, domain.MediaJobCancelled, domain.MediaJobBudgetBlocked, domain.MediaJobOutputInvalid:
			return WorkTaskView{}, domain.Policy("DEV_FIXTURE_MEDIA_FAILED", "开发演示视频生成未成功", "检查模拟服务商输出和媒体校验")
		}
		select {
		case <-ctx.Done():
			return WorkTaskView{}, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return WorkTaskView{}, domain.Policy("DEV_FIXTURE_MEDIA_TIMEOUT", "开发演示视频生成超时", "检查媒体执行器状态")
}

func demoCurrentRun(task WorkTaskView) domain.StageRun {
	for _, run := range task.StageRuns {
		if run.StageID == task.Task.CurrentStageID {
			return run
		}
	}
	return domain.StageRun{}
}

func demoArtifact(artifacts []domain.Artifact, kind string) (domain.Artifact, error) {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact, nil
		}
	}
	return domain.Artifact{}, domain.NotFound("媒体成果文件")
}

func demoMediaReview(reviews []domain.MediaReview, kind string) (domain.MediaReview, error) {
	for _, review := range reviews {
		if review.ReviewKind == kind {
			return review, nil
		}
	}
	return domain.MediaReview{}, domain.NotFound("媒体审核")
}

func fixtureRequestID(base, suffix string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "dev-fixture-v7"
	}
	return base + ":" + suffix
}

func marketingVideoDemoScript() json.RawMessage {
	value := map[string]any{
		"title":            "别把南京放回抽屉",
		"duration_seconds": 26,
		"channel":          "douyin",
		"angle":            "旅行纪念品从抽屉回到日常",
		"hook_type":        "收纳逆转",
		"cta":              "你会把哪座城市带回日常？",
		"scenes": []map[string]any{
			{"id": "scene-01", "scene": 1, "duration_seconds": 3, "visual": "固定机位拍周末书桌，抽屉打开，里面是旅行票根和一张未整理的照片。", "voiceover": "旅行回来，最先被收起来的，常常是那段慢下来的时间。", "on_screen_text": "别把旅行收进抽屉"},
			{"id": "scene-02", "scene": 2, "duration_seconds": 4, "visual": "手把票根推向抽屉，桌面另一侧的工作提醒亮起，不出现第三方应用界面。", "voiceover": "照片进相册，纪念品进抽屉，日子又回到消息列表。", "on_screen_text": "记忆很容易被日常盖住"},
			{"id": "scene-03", "scene": 3, "duration_seconds": 5, "visual": "手停住，只把完成权利核验的真实产品素材占位留在桌角。", "voiceover": "这次不急着收起来，先给它留一个看得见的位置。", "on_screen_text": "给日常留一个位置"},
			{"id": "scene-04", "scene": 4, "duration_seconds": 6, "visual": "把票根靠在产品占位旁，调整成稳定桌面构图；不点燃，不展示规格、价格或包装细节。", "voiceover": "不是把旅行搬回家，是给日常留一个想起它的动作。", "on_screen_text": "把一段南京放回日常"},
			{"id": "scene-05", "scene": 5, "duration_seconds": 5, "visual": "同一张桌面保持同一光线，手合上电脑，票根和产品占位继续留在画面。", "voiceover": "不讲一个很大的故事，今天只让桌面替你记住。", "on_screen_text": "不必很大声，也可以被记住"},
			{"id": "scene-06", "scene": 6, "duration_seconds": 3, "visual": "回到同一桌面近景，停留后出现评论引导。", "voiceover": "你会把哪座城市带回日常？", "on_screen_text": "评论区告诉我"},
		},
		"production_constraints": []string{"不得生成或替换产品包装、标志和可读标签", "不得使用未授权南京地标或网络照片", "不展示点燃、烟量、燃烧时长、价格、成分、功效或权威背书"},
	}
	body, _ := json.Marshal(value)
	return body
}

func marketingVideoDemoFrame(index int) ([]byte, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, 360, 640))
	palettes := [][4]color.RGBA{
		{{20, 42, 34, 255}, {225, 211, 179, 255}, {158, 72, 55, 255}, {24, 127, 120, 255}},
		{{32, 40, 48, 255}, {224, 218, 204, 255}, {77, 101, 120, 255}, {194, 79, 95, 255}},
		{{31, 52, 42, 255}, {236, 229, 211, 255}, {183, 130, 56, 255}, {20, 184, 166, 255}},
		{{43, 36, 35, 255}, {231, 217, 194, 255}, {126, 70, 52, 255}, {59, 111, 143, 255}},
		{{24, 39, 31, 255}, {213, 220, 210, 255}, {80, 101, 85, 255}, {180, 91, 60, 255}},
		{{19, 32, 25, 255}, {242, 238, 225, 255}, {172, 135, 61, 255}, {0, 127, 120, 255}},
		{{24, 39, 31, 255}, {245, 247, 245, 255}, {180, 91, 60, 255}, {59, 111, 143, 255}},
	}
	palette := palettes[index%len(palettes)]
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: palette[1]}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, 0, 360, 86), &image.Uniform{C: palette[0]}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(26, 118, 334, 540), &image.Uniform{C: color.RGBA{244, 241, 232, 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(48, 150, 312, 286), &image.Uniform{C: palette[2]}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(62, 166, 298, 270), &image.Uniform{C: color.RGBA{42, 48, 43, 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(84+index*4, 314, 178+index*4, 464), &image.Uniform{C: color.RGBA{217, 188, 105, 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(198-index*3, 332, 284-index*3, 492), &image.Uniform{C: palette[0]}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(211-index*3, 350, 271-index*3, 474), &image.Uniform{C: palette[3]}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(48, 568, 312, 577), &image.Uniform{C: palette[3]}, image.Point{}, draw.Src)
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

const marketingVideoDemoSource = `# 金陵古都香 V7 验收证据包

来源范围：marketing/jinling-gudu/outputs/scripts/jinling-douyin-20260803/11-drawer-reversal.md 及关联本体页面。

## 创意方向

旅行纪念品从抽屉回到日常，唯一测试变量为收纳逆转钩子。

## 素材边界

产品只允许使用完成版本和权利核验的真实素材，不生成或替换包装、标志和标签。

## 禁写边界

不写价格、成分、历史起源、功效、权威背书、固定体验时长或交通携带结论。

## CTA

唯一 CTA：你会把哪座城市带回日常？
`
