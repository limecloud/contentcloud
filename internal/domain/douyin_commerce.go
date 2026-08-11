package domain

import (
	"strings"
	"time"
)

const (
	DouyinCommerceProfileID        = "douyin-commerce-video"
	DouyinCommerceValidationSchema = "contentcloud.douyin-commerce-validation/1.0"
)

// DouyinCommerceOfferFacts is the normalized subset of an approved offer that
// is allowed to appear in the final creative and landing page.
type DouyinCommerceOfferFacts struct {
	SKUID            string   `json:"sku_id"`
	ProductVersionID string   `json:"product_version_id"`
	DisplayPrice     string   `json:"display_price"`
	Currency         string   `json:"currency"`
	Benefits         []string `json:"benefits"`
	Conditions       []string `json:"conditions"`
}

// DouyinCommerceValidationReceipt freezes the deterministic pre-publication
// checks. It is evidence for ChannelPublication, not a second publication or
// approval aggregate.
type DouyinCommerceValidationReceipt struct {
	SchemaVersion                      string                   `json:"schema_version"`
	ContentProfileID                   string                   `json:"content_profile_id"`
	ProjectID                          string                   `json:"project_id"`
	AudienceStrategyApprovedSnapshotID string                   `json:"audience_strategy_approved_snapshot_id"`
	AudienceStrategyVersionID          string                   `json:"audience_strategy_version_id"`
	OfferApprovedSnapshotID            string                   `json:"offer_approved_snapshot_id"`
	OfferSnapshotID                    string                   `json:"offer_snapshot_id"`
	ContentApprovedSnapshotID          string                   `json:"content_approved_snapshot_id"`
	ContentItemID                      string                   `json:"content_item_id"`
	ContentItemDigest                  string                   `json:"content_item_digest"`
	StoryboardApprovedSnapshotID       string                   `json:"storyboard_approved_snapshot_id"`
	StoryboardPackageID                string                   `json:"storyboard_package_id"`
	StoryboardLockedDigest             string                   `json:"storyboard_locked_digest"`
	RenderedCreativeArtifactID         string                   `json:"rendered_creative_artifact_id"`
	RenderedCreativeDigest             string                   `json:"rendered_creative_digest"`
	VoiceoverTextDigest                string                   `json:"voiceover_text_digest"`
	OnScreenTextDigest                 string                   `json:"on_screen_text_digest"`
	LandingPageTextDigest              string                   `json:"landing_page_text_digest"`
	Offer                              DouyinCommerceOfferFacts `json:"offer"`
	ObservedBenefits                   []string                 `json:"observed_benefits"`
	ObservedConditions                 []string                 `json:"observed_conditions"`
	AccountRef                         string                   `json:"account_ref"`
	ProductAnchorRef                   string                   `json:"product_anchor_ref"`
	LandingPageRef                     string                   `json:"landing_page_ref"`
	ScheduledAt                        time.Time                `json:"scheduled_at"`
	ValidatedAt                        time.Time                `json:"validated_at"`
	ReceiptDigest                      string                   `json:"receipt_digest"`
}

func (v DouyinCommerceValidationReceipt) Validate() error {
	if v.SchemaVersion != DouyinCommerceValidationSchema || v.ContentProfileID != DouyinCommerceProfileID || strings.TrimSpace(v.ProjectID) == "" {
		return Invalid("DOUYIN_COMMERCE_RECEIPT_IDENTITY_INVALID", "抖音电商校验回执缺少正确 Schema、内容 Profile 或项目")
	}
	for _, value := range []string{
		v.AudienceStrategyApprovedSnapshotID, v.AudienceStrategyVersionID,
		v.OfferApprovedSnapshotID, v.OfferSnapshotID,
		v.ContentApprovedSnapshotID, v.ContentItemID,
		v.StoryboardApprovedSnapshotID, v.StoryboardPackageID,
		v.RenderedCreativeArtifactID, v.AccountRef, v.ProductAnchorRef, v.LandingPageRef,
		v.Offer.SKUID, v.Offer.ProductVersionID, v.Offer.DisplayPrice, v.Offer.Currency,
	} {
		if strings.TrimSpace(value) == "" {
			return Invalid("DOUYIN_COMMERCE_RECEIPT_FIELD_REQUIRED", "抖音电商校验回执缺少固定的事实或发布引用")
		}
	}
	for _, digest := range []string{
		v.ContentItemDigest, v.StoryboardLockedDigest, v.RenderedCreativeDigest,
		v.VoiceoverTextDigest, v.OnScreenTextDigest, v.LandingPageTextDigest, v.ReceiptDigest,
	} {
		if !validSHA256Digest(digest) {
			return Invalid("DOUYIN_COMMERCE_RECEIPT_DIGEST_INVALID", "抖音电商校验回执包含无效 SHA-256 摘要")
		}
	}
	if len(v.Offer.Currency) != 3 || !uniqueNonEmpty(v.Offer.Benefits) || !uniqueNonEmpty(v.Offer.Conditions) || !uniqueNonEmpty(v.ObservedBenefits) || !uniqueNonEmpty(v.ObservedConditions) {
		return Invalid("DOUYIN_COMMERCE_RECEIPT_OFFER_INVALID", "抖音电商校验回执中的 Offer 事实为空、重复或币种无效")
	}
	if v.ScheduledAt.IsZero() || v.ValidatedAt.IsZero() || v.ValidatedAt.After(v.ScheduledAt) {
		return Invalid("DOUYIN_COMMERCE_RECEIPT_TIME_INVALID", "抖音电商校验必须发生在计划发布时间之前")
	}
	digest, err := v.ComputedDigest()
	if err != nil {
		return err
	}
	if digest != v.ReceiptDigest {
		return Conflict("DOUYIN_COMMERCE_RECEIPT_DIGEST_MISMATCH", "抖音电商校验回执摘要与内容不一致")
	}
	return nil
}

func (v DouyinCommerceValidationReceipt) ComputedDigest() (string, error) {
	v.ReceiptDigest = ""
	hash, err := CanonicalHash(v)
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func DouyinTextDigest(value string) (string, error) {
	hash, err := CanonicalHash(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

// DouyinCommercePublicationRefs binds a validated creative to the existing
// ChannelPublication intent and its exact approved inputs.
type DouyinCommercePublicationRefs struct {
	AudienceStrategyApprovedSnapshotID string                          `json:"audience_strategy_approved_snapshot_id"`
	AudienceStrategyVersionID          string                          `json:"audience_strategy_version_id"`
	OfferApprovedSnapshotID            string                          `json:"offer_approved_snapshot_id"`
	OfferSnapshotID                    string                          `json:"offer_snapshot_id"`
	ContentApprovedSnapshotID          string                          `json:"content_approved_snapshot_id"`
	ContentItemID                      string                          `json:"content_item_id"`
	StoryboardApprovedSnapshotID       string                          `json:"storyboard_approved_snapshot_id"`
	StoryboardPackageID                string                          `json:"storyboard_package_id"`
	RenderedCreativeArtifactID         string                          `json:"rendered_creative_artifact_id"`
	RenderedCreativeDigest             string                          `json:"rendered_creative_digest"`
	ValidationReceiptDigest            string                          `json:"validation_receipt_digest"`
	AccountRef                         string                          `json:"account_ref"`
	ProductAnchorRef                   string                          `json:"product_anchor_ref"`
	LandingPageRef                     string                          `json:"landing_page_ref"`
	ValidationReceipt                  DouyinCommerceValidationReceipt `json:"validation_receipt"`
}

func (v DouyinCommercePublicationRefs) Validate() error {
	for _, value := range []string{
		v.AudienceStrategyApprovedSnapshotID, v.AudienceStrategyVersionID,
		v.OfferApprovedSnapshotID, v.OfferSnapshotID,
		v.ContentApprovedSnapshotID, v.ContentItemID,
		v.StoryboardApprovedSnapshotID, v.StoryboardPackageID,
		v.RenderedCreativeArtifactID, v.AccountRef, v.ProductAnchorRef, v.LandingPageRef,
	} {
		if strings.TrimSpace(value) == "" {
			return Invalid("DOUYIN_COMMERCE_PUBLICATION_REF_REQUIRED", "抖音电商发布缺少批准快照、内容、资产、账号、商品锚点或落地页引用")
		}
	}
	if !validSHA256Digest(v.RenderedCreativeDigest) || !validSHA256Digest(v.ValidationReceiptDigest) {
		return Invalid("DOUYIN_COMMERCE_PUBLICATION_DIGEST_INVALID", "抖音电商发布引用包含无效摘要")
	}
	if err := v.ValidationReceipt.Validate(); err != nil {
		return err
	}
	receipt := v.ValidationReceipt
	for _, pair := range [][2]string{
		{v.AudienceStrategyApprovedSnapshotID, receipt.AudienceStrategyApprovedSnapshotID},
		{v.AudienceStrategyVersionID, receipt.AudienceStrategyVersionID},
		{v.OfferApprovedSnapshotID, receipt.OfferApprovedSnapshotID},
		{v.OfferSnapshotID, receipt.OfferSnapshotID},
		{v.ContentApprovedSnapshotID, receipt.ContentApprovedSnapshotID},
		{v.ContentItemID, receipt.ContentItemID},
		{v.StoryboardApprovedSnapshotID, receipt.StoryboardApprovedSnapshotID},
		{v.StoryboardPackageID, receipt.StoryboardPackageID},
		{v.RenderedCreativeArtifactID, receipt.RenderedCreativeArtifactID},
		{v.AccountRef, receipt.AccountRef},
		{v.ProductAnchorRef, receipt.ProductAnchorRef},
		{v.LandingPageRef, receipt.LandingPageRef},
		{v.RenderedCreativeDigest, receipt.RenderedCreativeDigest},
	} {
		if strings.TrimSpace(pair[0]) != strings.TrimSpace(pair[1]) {
			return Conflict("DOUYIN_COMMERCE_PUBLICATION_LINEAGE_MISMATCH", "发布引用与校验回执的对象、账号或成片摘要不一致")
		}
	}
	if v.ValidationReceiptDigest != v.ValidationReceipt.ReceiptDigest {
		return Conflict("DOUYIN_COMMERCE_PUBLICATION_RECEIPT_MISMATCH", "发布引用的校验摘要与校验回执不一致")
	}
	return nil
}
