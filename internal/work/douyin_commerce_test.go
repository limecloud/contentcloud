package work

import (
	"strings"
	"testing"
	"time"
)

func TestDouyinCommercePublicationRefsRequireReceiptLineage(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	digest := "sha256:" + strings.Repeat("a", 64)
	receipt := DouyinCommerceValidationReceipt{
		SchemaVersion:                      DouyinCommerceValidationSchema,
		ContentProfileID:                   DouyinCommerceProfileID,
		ProjectID:                          "project-1",
		AudienceStrategyApprovedSnapshotID: "strategy-snapshot",
		AudienceStrategyVersionID:          "strategy-1",
		OfferApprovedSnapshotID:            "offer-snapshot",
		OfferSnapshotID:                    "offer-1",
		ContentApprovedSnapshotID:          "content-snapshot",
		ContentItemID:                      "content-1",
		ContentItemDigest:                  digest,
		StoryboardApprovedSnapshotID:       "storyboard-snapshot",
		StoryboardPackageID:                "storyboard-1",
		StoryboardLockedDigest:             digest,
		RenderedCreativeArtifactID:         "artifact-1",
		RenderedCreativeDigest:             digest,
		VoiceoverTextDigest:                digest,
		OnScreenTextDigest:                 digest,
		LandingPageTextDigest:              digest,
		Offer: DouyinCommerceOfferFacts{
			SKUID: "sku-1", ProductVersionID: "product-1", DisplayPrice: "99.00", Currency: "CNY",
			Benefits: []string{"包邮"}, Conditions: []string{"现货"},
		},
		ObservedBenefits: []string{"包邮"}, ObservedConditions: []string{"现货"},
		AccountRef: "account-1", ProductAnchorRef: "anchor-1", LandingPageRef: "landing-1",
		ScheduledAt: now.Add(time.Hour), ValidatedAt: now,
	}
	var err error
	receipt.ReceiptDigest, err = receipt.ComputedDigest()
	if err != nil {
		t.Fatal(err)
	}
	refs := DouyinCommercePublicationRefs{
		AudienceStrategyApprovedSnapshotID: receipt.AudienceStrategyApprovedSnapshotID,
		AudienceStrategyVersionID:          receipt.AudienceStrategyVersionID,
		OfferApprovedSnapshotID:            receipt.OfferApprovedSnapshotID,
		OfferSnapshotID:                    receipt.OfferSnapshotID,
		ContentApprovedSnapshotID:          receipt.ContentApprovedSnapshotID,
		ContentItemID:                      receipt.ContentItemID,
		StoryboardApprovedSnapshotID:       receipt.StoryboardApprovedSnapshotID,
		StoryboardPackageID:                receipt.StoryboardPackageID,
		RenderedCreativeArtifactID:         receipt.RenderedCreativeArtifactID,
		RenderedCreativeDigest:             receipt.RenderedCreativeDigest,
		ValidationReceiptDigest:            receipt.ReceiptDigest,
		AccountRef:                         receipt.AccountRef,
		ProductAnchorRef:                   receipt.ProductAnchorRef,
		LandingPageRef:                     receipt.LandingPageRef,
		ValidationReceipt:                  receipt,
	}
	if err := refs.Validate(); err != nil {
		t.Fatalf("matching publication refs rejected: %v", err)
	}
	refs.RenderedCreativeDigest = "sha256:" + strings.Repeat("b", 64)
	if err := refs.Validate(); err == nil {
		t.Fatal("publication refs with a different creative digest must be rejected")
	}
}
