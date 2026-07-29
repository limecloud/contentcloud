package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestServerRejectsLocalV5CandidatesAsFormalSubmissions(t *testing.T) {
	strategy := domain.AudienceStrategyVersion{
		ID: "strategy-1", Type: "audience_strategy_version", SchemaVersion: domain.AudienceStrategySchema, ProjectID: "project-1",
		TaxonomySnapshotID: "taxonomy-1", AudienceCode: "gen_z", AudienceLabel: "Z世代", SegmentDefinition: "需求状态",
		Objective: "conversion", DemandMoment: "通勤", InsightStatement: "有证据的洞察", HookHypotheses: []string{"场景钩子"}, Scenario: "通勤",
		ProofOrder: []string{"规格"}, Objections: []string{"体积"}, CTAStrategy: "查看详情", EvidenceRefs: []string{"evidence-1"}, Confidence: "medium",
		TestType: "audience_expression_fit_test", PrimaryVariable: "audience", ControlledVariables: []string{"cta"}, TargetMetrics: []string{"ctr"}, Constraints: []string{}, Status: "candidate",
	}
	object, err := domain.NewSubmissionObjectRef(strategy.ID, strategy.Type, 1, "50-production/strategies/strategy-1.json", strategy)
	if err != nil {
		t.Fatal(err)
	}
	mismatched := object
	mismatched.ID = "strategy-ref-does-not-match-content"
	assertAppV5Code(t, validateGovernedSubmissionObjects("strategy", "project-1", nil, []domain.SubmissionObjectRef{mismatched}, time.Now()), "SUBMISSION_OBJECT_IDENTITY_MISMATCH")
	err = validateGovernedSubmissionObjects("strategy", "project-1", nil, []domain.SubmissionObjectRef{object}, time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC))
	assertAppV5Code(t, err, "AUDIENCE_STRATEGY_NOT_REVIEW_READY")

	asset := domain.StoryboardAsset{ID: "asset-1", Role: "first_frame", ShotID: "shot-1", Path: "50-production/media/first.png", MediaType: "image/png", SHA256: strings.Repeat("a", 64), ByteSize: 10, RightsRefs: []string{"rights-1"}}
	storyboard := domain.StoryboardPackage{
		ID: "storyboard-1", Type: "storyboard_package", SchemaVersion: domain.StoryboardPackageSchema, ProjectID: "project-1", ApprovedSnapshotID: "content-snapshot", ContentItemID: "content-1",
		GeneratorCapability: domain.CapabilityRef{ID: "image.test", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("b", 64)}, Status: "candidate",
		Shots:  []domain.StoryboardShot{{ShotID: "shot-1", StartMS: 0, EndMS: 1000, Role: "hook", FirstFrameArtifactID: asset.ID, ImagePromptZH: "首帧", NegativeConstraints: []string{"无文字"}, AcceptanceCriteria: []string{"主体清晰"}, PlanB: "实拍"}},
		Assets: []domain.StoryboardAsset{asset}, RightsRefs: []string{"rights-1"}, SourceDigest: "sha256:" + strings.Repeat("c", 64), LockedDigest: "sha256:" + strings.Repeat("d", 64),
	}
	object, err = domain.NewSubmissionObjectRef(storyboard.ID, storyboard.Type, 1, "50-production/media/storyboards/storyboard-1/manifest.json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	err = validateGovernedSubmissionObjects("storyboard", "project-1", []string{"content-snapshot"}, []domain.SubmissionObjectRef{object}, time.Now())
	assertAppV5Code(t, err, "STORYBOARD_NOT_REVIEW_READY")

	reviewSheet := domain.StoryboardAsset{ID: "review-1", Role: "review_sheet", Path: "50-production/media/review.png", MediaType: "image/png", SHA256: strings.Repeat("e", 64), ByteSize: 10, RightsRefs: []string{"rights-1"}}
	storyboard.Status = "review_ready"
	storyboard.ReviewSheetArtifactID = reviewSheet.ID
	storyboard.Assets = append(storyboard.Assets, reviewSheet)
	object, err = domain.NewSubmissionObjectRef(storyboard.ID, storyboard.Type, 1, "50-production/media/storyboards/storyboard-1/manifest.json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	err = validateGovernedSubmissionObjects("storyboard", "project-1", []string{"content-snapshot"}, []domain.SubmissionObjectRef{object}, time.Now())
	assertAppV5Code(t, err, "STORYBOARD_LOCKED_DIGEST_MISMATCH")
	storyboard.LockedDigest, err = storyboard.ComputedLockedDigest()
	if err != nil {
		t.Fatal(err)
	}
	object, err = domain.NewSubmissionObjectRef(storyboard.ID, storyboard.Type, 1, "50-production/media/storyboards/storyboard-1/manifest.json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateGovernedSubmissionObjects("storyboard", "project-1", []string{"content-snapshot"}, []domain.SubmissionObjectRef{object}, time.Now()); err != nil {
		t.Fatalf("server rejected a review-ready storyboard with a valid locked digest: %v", err)
	}
}

func TestServerRequiresApprovedTaxonomyBaselineForAudienceStrategy(t *testing.T) {
	now := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	taxonomy := domain.AudienceTaxonomySnapshot{
		ID: "taxonomy-1", Type: "audience_taxonomy_snapshot", SchemaVersion: domain.AudienceTaxonomySchema,
		Provider: "oceanengine_yuntu", TaxonomyID: "douyin-commerce-eight-audiences", TaxonomyVersion: "2026-07-29",
		Segments: domain.DefaultDouyinAudienceSegments(), SourceURL: "https://school.oceanengine.com/", CapturedAt: now,
		EffectiveFrom: now, ExpiresAt: now.Add(30 * 24 * time.Hour), VerificationStatus: "human_verified", SourceSHA256: strings.Repeat("a", 64), Status: "review_ready",
	}
	strategy := domain.AudienceStrategyVersion{
		ID: "strategy-1", Type: "audience_strategy_version", SchemaVersion: domain.AudienceStrategySchema, ProjectID: "project-1",
		TaxonomySnapshotID: taxonomy.ID, AudienceCode: taxonomy.Segments[0].Code, AudienceLabel: taxonomy.Segments[0].Label, SegmentDefinition: taxonomy.Segments[0].Definition,
		Objective: "conversion", DemandMoment: "通勤", InsightStatement: "有证据的洞察", HookHypotheses: []string{"场景钩子"}, Scenario: "通勤",
		ProofOrder: []string{"规格"}, Objections: []string{"体积"}, CTAStrategy: "查看详情", EvidenceRefs: []string{"evidence-1"}, Confidence: "medium",
		TestType: "audience_expression_fit_test", PrimaryVariable: "audience", ControlledVariables: []string{"cta"}, TargetMetrics: []string{"ctr"}, Constraints: []string{}, Status: "review_ready",
	}
	object, err := domain.NewSubmissionObjectRef(strategy.ID, strategy.Type, 1, "50-production/strategies/strategy-1.json", strategy)
	if err != nil {
		t.Fatal(err)
	}
	assertAppV5Code(t, validateGovernedBaseSnapshotTypes("strategy", []domain.SubmissionObjectRef{object}, nil, now), "AUDIENCE_TAXONOMY_BASE_SNAPSHOT_INVALID")
	canonical, err := json.Marshal(map[string]any{"submission_type": "strategy", "objects": []any{taxonomy}})
	if err != nil {
		t.Fatal(err)
	}
	baseSnapshots := map[string]domain.ApprovedSnapshot{
		"taxonomy-snapshot": {ID: "taxonomy-snapshot", ProjectID: "project-1", SubmissionType: "strategy", CanonicalContent: canonical, EligibleIDs: []string{taxonomy.ID}},
	}
	if err := validateGovernedBaseSnapshotTypes("strategy", []domain.SubmissionObjectRef{object}, baseSnapshots, now); err != nil {
		t.Fatalf("server rejected strategy with its approved taxonomy baseline: %v", err)
	}
	strategy.AudienceLabel = "被篡改的人群名称"
	object, err = domain.NewSubmissionObjectRef(strategy.ID, strategy.Type, 1, "50-production/strategies/strategy-1.json", strategy)
	if err != nil {
		t.Fatal(err)
	}
	assertAppV5Code(t, validateGovernedBaseSnapshotTypes("strategy", []domain.SubmissionObjectRef{object}, baseSnapshots, now), "AUDIENCE_STRATEGY_TAXONOMY_MISMATCH")
	assertAppV5Code(t, validateGovernedBaseSnapshotTypes("strategy", []domain.SubmissionObjectRef{object}, baseSnapshots, taxonomy.ExpiresAt), "AUDIENCE_TAXONOMY_NOT_REVIEW_READY")
}

func TestServerValidatesStoryboardContentBaseline(t *testing.T) {
	contentItem := map[string]any{"id": "content-1", "type": "content_item", "title": "已批准剧本"}
	sourceHash, err := domain.CanonicalHash(contentItem)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(map[string]any{"submission_type": "content_batch", "objects": []any{contentItem}})
	if err != nil {
		t.Fatal(err)
	}
	baseSnapshots := map[string]domain.ApprovedSnapshot{
		"content-snapshot": {ID: "content-snapshot", ProjectID: "project-1", SubmissionType: "content_batch", CanonicalContent: canonical, EligibleIDs: []string{"content-1"}},
	}
	storyboard := domain.StoryboardPackage{
		ID: "storyboard-1", Type: "storyboard_package", ApprovedSnapshotID: "content-snapshot", ContentItemID: "content-1", SourceDigest: "sha256:" + sourceHash,
	}
	object, err := domain.NewSubmissionObjectRef(storyboard.ID, storyboard.Type, 1, "50-production/media/storyboards/storyboard-1/manifest.json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateGovernedBaseSnapshotTypes("storyboard", []domain.SubmissionObjectRef{object}, baseSnapshots, time.Now()); err != nil {
		t.Fatalf("server rejected storyboard with matching content baseline: %v", err)
	}
	storyboard.ContentItemID = "content-missing"
	object, err = domain.NewSubmissionObjectRef(storyboard.ID, storyboard.Type, 1, "50-production/media/storyboards/storyboard-1/manifest.json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	assertAppV5Code(t, validateGovernedBaseSnapshotTypes("storyboard", []domain.SubmissionObjectRef{object}, baseSnapshots, time.Now()), "STORYBOARD_CONTENT_ITEM_BASE_INVALID")
	storyboard.ContentItemID = "content-1"
	storyboard.SourceDigest = "sha256:" + strings.Repeat("f", 64)
	object, err = domain.NewSubmissionObjectRef(storyboard.ID, storyboard.Type, 1, "50-production/media/storyboards/storyboard-1/manifest.json", storyboard)
	if err != nil {
		t.Fatal(err)
	}
	assertAppV5Code(t, validateGovernedBaseSnapshotTypes("storyboard", []domain.SubmissionObjectRef{object}, baseSnapshots, time.Now()), "STORYBOARD_SOURCE_DIGEST_MISMATCH")
}

func assertAppV5Code(t *testing.T, err error, code string) {
	t.Helper()
	value, ok := err.(*domain.Error)
	if !ok || value.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}
