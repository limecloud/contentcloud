package app_test

import (
	"log/slog"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestArticleSubmissionRequiresTenantCapabilityAndApprovedEvidence(t *testing.T) {
	service := app.New(memory.New(), slog.Default(), app.WithPlatformAdminEmails("article-admin@example.com"))
	session, err := service.Register(t.Context(), "article-admin@example.com", "long-enough-password", "Article Admin", "Article Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	must(t, err)
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: localworkspace.WeChatChannel}, "article-project")
	must(t, err)
	connect, err := service.CreateConnectSession(t.Context(), actor, project.ID, "article-connect")
	must(t, err)
	connected, err := testsupport.ConnectBootstrap(t.Context(), service, actor, connect, app.ConnectDeviceInput{Hostname: "article-test"})
	must(t, err)
	workspaceActor, binding, err := service.WorkspaceActor(t.Context(), connected.WorkspaceToken)
	must(t, err)

	fact := map[string]any{"id": "fact:article", "kind": "fact", "status": "candidate", "risk_level": "low"}
	claim := map[string]any{"id": "claim:article", "kind": "claim", "status": "candidate", "risk_level": "low"}
	knowledgeSnapshot := publishAndApproveArticleFixture(t, service, actor, workspaceActor, binding, "knowledge", []fixtureSubmissionObject{{ID: "fact:article", Type: "fact", Content: fact}, {ID: "claim:article", Type: "claim", Content: claim}}, nil, "article-knowledge")

	_, err = service.UpdatePlatformTenantContentCapability(t.Context(), actor, actor.TenantID, domain.ContentTypeWeChatArticle, true, "enable-wechat")
	must(t, err)
	brief := localworkspace.ArticleBrief{
		ID: "article-brief:1", Kind: "article_brief", Status: "candidate", SchemaVersion: localworkspace.ArticleBriefSchema, Deliverability: "review_ready", IntentID: "intent:article", Channel: localworkspace.WeChatChannel,
		Topic: "主题", ReaderPromise: "读者收益", ContentPillar: "指南", Objective: "建立认知", Audience: "目标读者", ReadingContext: "通勤", StructureType: "how_to", SectionGoals: []string{"问题", "步骤"}, OpeningStrategy: "问题切入", EndingStrategy: "行动收束",
		TargetWordCount: 300, MinWordCount: 100, MaxWordCount: 600, Voice: "编辑", Tone: "可信", NarrativePerson: "second", RequiredKnowledgeIDs: []string{"fact:article"}, ApprovedClaimIDs: []string{"claim:article"}, AssetIDs: []string{}, RightsIDs: []string{}, CoverIntent: "真实场景", CTA: "开始行动", PrimaryVariable: "title", ControlledVariables: []string{"opening"}, BlockedReasons: []string{}, MissingInputs: []string{},
	}
	briefSnapshot := publishAndApproveArticleFixture(t, service, actor, workspaceActor, binding, "brief", []fixtureSubmissionObject{{ID: brief.ID, Type: "article_brief", Content: brief}}, []string{knowledgeSnapshot.ID}, "article-brief")

	item := serviceArticleItem(project.ID, brief.ID, "claim:article")
	_, err = service.UpdatePlatformTenantContentCapability(t.Context(), actor, actor.TenantID, domain.ContentTypeWeChatArticle, false, "disable-wechat")
	must(t, err)
	if _, err := createArticleContentSubmission(t, service, workspaceActor, binding, item, []string{briefSnapshot.ID, knowledgeSnapshot.ID}, "article-disabled"); err == nil {
		t.Fatal("disabled tenant created an ArticleItem submission")
	} else {
		assertDomainCode(t, err, "CONTENT_TYPE_NOT_ENABLED")
	}

	_, err = service.UpdatePlatformTenantContentCapability(t.Context(), actor, actor.TenantID, domain.ContentTypeWeChatArticle, true, "re-enable-wechat")
	must(t, err)
	badClaim := serviceArticleItem(project.ID, brief.ID, "fact:article")
	if _, err := createArticleContentSubmission(t, service, workspaceActor, binding, badClaim, []string{briefSnapshot.ID, knowledgeSnapshot.ID}, "article-bad-claim"); err == nil {
		t.Fatal("commercial claim backed only by a fact was accepted")
	} else {
		assertDomainCode(t, err, "ARTICLE_CLAIM_BASE_INVALID")
	}
	revision, err := createArticleContentSubmission(t, service, workspaceActor, binding, item, []string{briefSnapshot.ID, knowledgeSnapshot.ID}, "article-valid")
	must(t, err)
	if revision.ID == "" || len(revision.BaseSnapshotIDs) != 2 {
		t.Fatalf("unexpected ArticleItem revision: %#v", revision)
	}
}

type fixtureSubmissionObject struct {
	ID      string
	Type    string
	Content any
}

func publishAndApproveArticleFixture(t *testing.T, service *app.Service, actor, workspaceActor app.Actor, binding domain.WorkspaceBinding, submissionType string, values []fixtureSubmissionObject, baseSnapshotIDs []string, key string) domain.ApprovedSnapshot {
	t.Helper()
	objects := make([]domain.SubmissionObjectRef, 0, len(values))
	for _, value := range values {
		object, err := domain.NewSubmissionObjectRef(value.ID, value.Type, 1, submissionType+"/"+value.ID+".json", value.Content)
		must(t, err)
		objects = append(objects, object)
	}
	bundle := domain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: submissionType, ProjectID: binding.ProjectID, WorkspaceID: binding.ID, BaseSnapshotIDs: baseSnapshotIDs, EnvironmentDigest: submissionEnvironmentDigest, Objects: objects, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{{Name: submissionType + "-lint", Status: "passed"}}}, IdempotencyKey: key}
	must(t, bundle.SetComputedHash())
	revision, err := service.CreateSubmission(t.Context(), workspaceActor, binding, bundle, key)
	must(t, err)
	approved, err := service.ApproveSubmission(t.Context(), actor, revision.ID, "fixture approved", key+"-approve")
	must(t, err)
	if approved.ApprovedSnapshot == nil {
		t.Fatalf("%s fixture did not create an ApprovedSnapshot", submissionType)
	}
	return *approved.ApprovedSnapshot
}

func createArticleContentSubmission(t *testing.T, service *app.Service, workspaceActor app.Actor, binding domain.WorkspaceBinding, item localworkspace.ArticleItem, baseSnapshotIDs []string, key string) (domain.SubmissionRevision, error) {
	t.Helper()
	object, err := domain.NewSubmissionObjectRef(item.ID, item.Type, 1, "50-production/batches/"+item.ContentBatchID+"/items/"+item.ID+".json", item)
	if err != nil {
		return domain.SubmissionRevision{}, err
	}
	bundle := domain.SubmissionBundle{BundleVersion: "3.0", SubmissionType: "content_batch", ProjectID: binding.ProjectID, WorkspaceID: binding.ID, BaseSnapshotIDs: baseSnapshotIDs, EnvironmentDigest: submissionEnvironmentDigest, Objects: []domain.SubmissionObjectRef{object}, SourceDisclosures: []domain.SourceDisclosure{}, Artifacts: []domain.SubmissionArtifact{}, LocalRunSummary: domain.LocalRunSummary{Checks: []domain.LocalRunCheck{{Name: "article-item-lint", Status: "passed"}}}, IdempotencyKey: key}
	if err := bundle.SetComputedHash(); err != nil {
		return domain.SubmissionRevision{}, err
	}
	return service.CreateSubmission(t.Context(), workspaceActor, binding, bundle, key)
}

func serviceArticleItem(projectID, briefID, claimID string) localworkspace.ArticleItem {
	return localworkspace.ArticleItem{
		ID: "article-item:" + claimID, Type: "article_item", Status: "candidate", SchemaVersion: localworkspace.ArticleSchema, Deliverability: "review_ready", ProjectID: projectID, ContentID: "article-content:1", ContentBatchID: "article-batch:1", BriefRef: briefID, ContextSnapshotID: "article-context:1", ResolvedCommentIDs: []string{}, Language: "zh-CN",
		TitleCandidates: []localworkspace.ArticleTitle{{ID: "title-1", Text: "一篇经过证据校验的公众号文章", Strategy: "reader-benefit", RiskRefs: []string{}}}, SelectedTitleID: "title-1", Summary: "用于验证租户能力和服务端证据门禁。", AuthorDisplayName: "ContentCloud",
		Cover: localworkspace.ArticleImage{}, Blocks: []localworkspace.ArticleBlock{
			{ID: "block-1", Type: "heading", Text: "核心内容", Level: 2, Items: []string{}, Assertions: []localworkspace.ArticleAssertion{}, StyleMarks: []string{}},
			{ID: "block-2", Type: "paragraph", Text: "这是一段需要批准主张支持的正文。", Items: []string{}, Assertions: []localworkspace.ArticleAssertion{{ID: "assertion-1", Type: "commercial_claim", KnowledgeRefs: []string{claimID}, EvidenceRefs: []string{}, Attribution: ""}}, StyleMarks: []string{}},
		}, Attribution: localworkspace.ArticleAttribution{SourceNames: []string{"Approved Knowledge"}, Disclosure: "来自已批准知识快照"},
		EditorialChecks: localworkspace.ArticleEditorialChecks{SchemaChecked: true, KnowledgeChecked: true, ClaimsChecked: true, QuotationsChecked: true, RightsChecked: true, ChannelChecked: true}, ChannelHints: localworkspace.ArticleChannelHints{HighlightBlockIDs: []string{}, PreferredLineBreakBlockIDs: []string{}}, BlockedReasons: []localworkspace.ContentBlockedReason{}, MissingInputs: []string{},
	}
}
