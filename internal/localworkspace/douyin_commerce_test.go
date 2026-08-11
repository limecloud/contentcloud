package localworkspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestDouyinCommerceValidationFreezesOfferAndCreativeLineage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	strategy := domain.AudienceStrategyVersion{ID: "strategy-1", Type: "audience_strategy_version", SchemaVersion: domain.AudienceStrategySchema, ProjectID: "project-1", TaxonomySnapshotID: "taxonomy-1", AudienceCode: "gen_z", AudienceLabel: "Z世代", SegmentDefinition: "需求状态", Objective: "conversion", DemandMoment: "通勤", InsightStatement: "证据洞察", HookHypotheses: []string{"场景钩子"}, Scenario: "通勤场景", ProofOrder: []string{"规格"}, Objections: []string{"价格"}, CTAStrategy: "查看详情", EvidenceRefs: []string{"evidence-1"}, Confidence: "medium", TestType: "audience_expression_fit_test", PrimaryVariable: "audience", ControlledVariables: []string{"cta"}, TargetMetrics: []string{"clicks"}, Status: "review_ready"}
	offer := domain.CommerceOfferSnapshot{ID: "offer-1", Type: "commerce_offer_snapshot", SchemaVersion: domain.CommerceOfferSchema, ProjectID: "project-1", SKUID: "sku-1", ProductVersionID: "product-v1", ApprovedClaimRefs: []string{"claim-1"}, DisplayPrice: "168", Currency: "CNY", Benefits: []string{"包邮"}, Conditions: []string{"现货"}, EvidenceRefs: []string{"evidence-offer"}, CapturedAt: now, ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(24 * time.Hour), Status: "verified"}
	content := validContentItem(ContentBatch{ID: "batch-1", ProjectID: "project-1", BriefRef: "brief-1", ContextSnapshotID: "context-1"}, CreativeDirection{ID: "direction-1", Status: "selected"}, "claim-1")
	content.ID, content.ProjectID, content.Channel, content.Deliverability = "content-1", "project-1", "douyin", "review_ready"
	content.Shots = []ContentShot{{ShotID: "shot-1", StartMS: 0, EndMS: 1000, Voiceover: "真实商品演示", OnScreenText: "查看详情", ClaimRefs: []string{"claim-1"}, ProductTruthStrategy: "真实商品", RightsRefs: []string{"rights-1"}, AcceptanceCriteria: []string{"主体清晰"}, PlanB: "实拍"}}
	content.DurationMS = 1000
	asset := domain.StoryboardAsset{ID: "frame-1", Role: "first_frame", ShotID: "shot-1", Path: "50-production/media/frame-1.png", MediaType: "image/png", SHA256: strings.Repeat("a", 64), ByteSize: 10, RightsRefs: []string{"rights-1"}}
	review := domain.StoryboardAsset{ID: "review-1", Role: "review_sheet", Path: "50-production/media/review.png", MediaType: "image/png", SHA256: strings.Repeat("b", 64), ByteSize: 10, RightsRefs: []string{"rights-1"}}
	storyboard := domain.StoryboardPackage{ID: "storyboard-1", Type: "storyboard_package", SchemaVersion: domain.StoryboardPackageSchema, ProjectID: "project-1", ApprovedSnapshotID: "content-snapshot", ContentItemID: content.ID, GeneratorCapability: domain.CapabilityRef{ID: "image.test", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("c", 64)}, Status: "review_ready", Shots: []domain.StoryboardShot{{ShotID: "shot-1", StartMS: 0, EndMS: 1000, Role: "hook", FirstFrameArtifactID: asset.ID, ImagePromptZH: "展示商品", NegativeConstraints: []string{"无错误文字"}, AcceptanceCriteria: []string{"主体清晰"}, PlanB: "实拍"}}, Assets: []domain.StoryboardAsset{asset, review}, ReviewSheetArtifactID: review.ID, RightsRefs: []string{"rights-1"}, SourceDigest: "sha256:" + strings.Repeat("d", 64)}
	var err error
	storyboard.LockedDigest, err = storyboard.ComputedLockedDigest()
	if err != nil {
		t.Fatal(err)
	}
	storeCommerceSnapshot(t, root, "strategy-snapshot", "strategy", strategy.ID, strategy, now)
	storeCommerceSnapshot(t, root, "offer-snapshot", "offer", offer.ID, offer, now)
	storeCommerceSnapshot(t, root, "content-snapshot", "content_batch", content.ID, content, now)
	storeCommerceSnapshot(t, root, "storyboard-snapshot", "storyboard", storyboard.ID, storyboard, now)
	artifactPath := filepath.Join(root, "50-production", "media", "final.mp4")
	if err := os.WriteFile(artifactPath, []byte("final-video-v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	landingPath := filepath.Join(root, "50-production", "landing.txt")
	if err := os.WriteFile(landingPath, []byte("商品详情，现货，包邮"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ValidateDouyinCommerce(ValidateDouyinCommerceOptions{Root: root, AudienceStrategyApprovedSnapshotID: "strategy-snapshot", AudienceStrategyVersionID: strategy.ID, OfferApprovedSnapshotID: "offer-snapshot", OfferSnapshotID: offer.ID, ContentApprovedSnapshotID: "content-snapshot", ContentItemID: content.ID, StoryboardApprovedSnapshotID: "storyboard-snapshot", StoryboardPackageID: storyboard.ID, RenderedCreativeArtifactID: "artifact-final", RenderedCreativeFile: "50-production/media/final.mp4", LandingPageTextFile: "50-production/landing.txt", ObservedBenefits: []string{"包邮"}, ObservedConditions: []string{"现货"}, AccountRef: "douyin-main", ProductAnchorRef: "product-anchor-1", LandingPageRef: "landing-1", ScheduledAt: now.Add(time.Hour), ValidatedAt: now.Add(10 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.ReceiptDigest == "" || result.Path == "" {
		t.Fatalf("validation result missing immutable receipt: %+v", result)
	}
	if report, receipt, err := LintDouyinCommerceReceipt(root, result.Path, "50-production/media/final.mp4", "50-production/landing.txt"); err != nil || !report.Valid || receipt.ReceiptDigest != result.Receipt.ReceiptDigest {
		t.Fatalf("receipt lint did not reproduce the same evidence: report=%+v receipt=%+v err=%v", report, receipt, err)
	}
	if err := os.WriteFile(artifactPath, []byte("tampered-video"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, _, err := LintDouyinCommerceReceipt(root, result.Path, "50-production/media/final.mp4", "50-production/landing.txt")
	if err != nil || report.Valid {
		t.Fatalf("receipt lint accepted final artifact drift: report=%+v err=%v", report, err)
	}
}

func storeCommerceSnapshot(t *testing.T, root, snapshotID, submissionType, objectID string, object any, createdAt time.Time) {
	t.Helper()
	body, err := json.Marshal([]any{object})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(map[string]any{"schema_version": domain.SubmissionSchemaVersion(submissionType), "submission_type": submissionType, "objects": json.RawMessage(body)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := domain.ApprovedSnapshot{ID: snapshotID, ProjectID: "project-1", SubmissionType: submissionType, CanonicalContent: canonical, EligibleIDs: []string{objectID}, CreatedAt: createdAt}
	if _, err := StoreApprovedSnapshot(root, snapshot, createdAt); err != nil {
		t.Fatal(err)
	}
}
