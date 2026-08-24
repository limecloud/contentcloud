package work

import (
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

func TestStoryboardVisualBindingLocksIdentitySourceAndAnchor(t *testing.T) {
	storyboard := validVisualBindingStoryboard()
	if err := storyboard.Validate(true); err != nil {
		t.Fatalf("visual binding storyboard should validate: %v", err)
	}
	first, err := storyboard.ComputedLockedDigest()
	if err != nil {
		t.Fatal(err)
	}
	storyboard.VisualBindings[0].SourceDigest = "sha256:" + strings.Repeat("b", 64)
	second, err := storyboard.ComputedLockedDigest()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("changing a visual source digest must invalidate the locked storyboard digest")
	}
}

func TestStoryboardVisualBindingRejectsUnknownAnchorAndShotBinding(t *testing.T) {
	storyboard := validVisualBindingStoryboard()
	storyboard.VisualBindings[0].AnchorAssetID = "missing-anchor"
	assertStoryboardVisualBindingCode(t, storyboard.Validate(true), "STORYBOARD_VISUAL_BINDING_ANCHOR_INVALID")

	storyboard = validVisualBindingStoryboard()
	storyboard.Shots[0].VisualBindingRefs = []string{"missing-binding"}
	assertStoryboardVisualBindingCode(t, storyboard.Validate(true), "STORYBOARD_VISUAL_BINDING_REF_UNKNOWN")

	storyboard = validVisualBindingStoryboard()
	storyboard.VisualBindings[0].RightsRefs = []string{"rights:missing"}
	assertStoryboardVisualBindingCode(t, storyboard.Validate(true), "STORYBOARD_VISUAL_BINDING_RIGHTS_INVALID")
}

func validVisualBindingStoryboard() StoryboardPackage {
	const digestA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	anchor := StoryboardAsset{ID: "anchor-product", Role: "identity_anchor", Path: "50-production/media/anchors/product.png", MediaType: "image/png", SHA256: digestA, ByteSize: 10, RightsRefs: []string{"rights:product"}}
	first := StoryboardAsset{ID: "frame-1", Role: "first_frame", ShotID: "shot-1", Path: "50-production/media/shot-1/first-frame.png", MediaType: "image/png", SHA256: digestA, ByteSize: 10, RightsRefs: []string{"rights:product"}}
	review := StoryboardAsset{ID: "review-1", Role: "review_sheet", Path: "50-production/media/review-sheet.png", MediaType: "image/png", SHA256: digestA, ByteSize: 10, RightsRefs: []string{"rights:product"}}
	storyboard := StoryboardPackage{
		ID: "storyboard-visual-1", Type: "storyboard_package", SchemaVersion: StoryboardPackageSchema, ProjectID: "project-1", ApprovedSnapshotID: "content-snapshot", ContentItemID: "content-1",
		GeneratorCapability: CapabilityRef{ID: "image.test", Version: "1.0.0", Digest: "sha256:" + digestA}, Status: "review_ready",
		Assets: []StoryboardAsset{anchor, first, review},
		VisualBindings: []StoryboardVisualBinding{
			{ID: "visual:product-1", Kind: "product", SourceRef: "material:product-1", SourceDigest: "sha256:" + digestA, AnchorAssetID: anchor.ID, RightsRefs: []string{"rights:product"}},
		},
		Shots: []StoryboardShot{
			{ShotID: "shot-1", StartMS: 0, EndMS: 1000, Role: "hook", FirstFrameArtifactID: first.ID, ImagePromptZH: "使用固定产品参考", PlanB: "使用已批准实拍", NegativeConstraints: []string{"不得变更包装"}, AcceptanceCriteria: []string{"产品可识别"}, VisualBindingRefs: []string{"visual:product-1"}},
		},
		ReviewSheetArtifactID: review.ID, RightsRefs: []string{"rights:product"}, SourceDigest: "sha256:" + digestA,
	}
	locked, err := storyboard.ComputedLockedDigest()
	if err != nil {
		panic(err)
	}
	storyboard.LockedDigest = locked
	return storyboard
}

func assertStoryboardVisualBindingCode(t *testing.T, err error, code string) {
	t.Helper()
	value, ok := err.(*fault.Error)
	if !ok || value.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}
