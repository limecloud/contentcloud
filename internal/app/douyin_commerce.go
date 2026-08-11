package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

func (s *Service) validateDouyinCommercePublication(ctx context.Context, actor Actor, delivery domain.TaskDelivery, binding domain.ChannelBinding, input PrepareChannelPublicationInput) (map[string]any, error) {
	if input.ContentProfileID != domain.DouyinCommerceProfileID || input.DouyinCommerce == nil {
		return nil, domain.Invalid("DOUYIN_COMMERCE_PUBLICATION_REFS_REQUIRED", "抖音电商发布必须提供 ContentProfileID 和类型化血缘引用")
	}
	refs := *input.DouyinCommerce
	if err := refs.Validate(); err != nil {
		return nil, err
	}
	if binding.Channel != "douyin" || binding.AccountRef != refs.AccountRef || delivery.ProjectID != refs.ValidationReceipt.ProjectID || refs.ValidationReceipt.AccountRef != binding.AccountRef {
		return nil, domain.Conflict("DOUYIN_COMMERCE_PUBLICATION_SCOPE_INVALID", "抖音电商校验回执、渠道账号和项目不一致")
	}
	if input.ScheduledAt == nil || !input.ScheduledAt.Equal(refs.ValidationReceipt.ScheduledAt) {
		return nil, domain.Conflict("DOUYIN_COMMERCE_SCHEDULE_MISMATCH", "计划发布时间必须与抖音电商校验回执一致")
	}
	if err := s.validateDouyinApprovedInputs(ctx, actor.TenantID, delivery.ProjectID, refs); err != nil {
		return nil, err
	}
	pkg, err := s.store.DeliveryPackage(ctx, actor.TenantID, delivery.DeliveryPackageID)
	if err != nil {
		return nil, err
	}
	if pkg.ProjectID != delivery.ProjectID || pkg.ContentItemID != refs.ContentItemID {
		return nil, domain.Conflict("DOUYIN_COMMERCE_DELIVERY_LINEAGE_INVALID", "DeliveryPackage 的项目或 ContentItem 与校验回执不一致")
	}
	var finalArtifact *domain.Artifact
	for index := range pkg.Manifest {
		artifact := &pkg.Manifest[index]
		if artifact.ID == refs.RenderedCreativeArtifactID {
			finalArtifact = artifact
			break
		}
	}
	if finalArtifact == nil || normalizedArtifactDigest(finalArtifact.SHA256) != refs.RenderedCreativeDigest {
		return nil, domain.Conflict("DOUYIN_COMMERCE_ARTIFACT_LINEAGE_INVALID", "最终成片 Artifact 不在 DeliveryPackage manifest 中，或摘要不一致")
	}
	if finalArtifact.ProjectID != delivery.ProjectID || finalArtifact.ApprovedSnapshotID == "" {
		return nil, domain.Conflict("DOUYIN_COMMERCE_ARTIFACT_SCOPE_INVALID", "最终成片 Artifact 不属于当前项目或没有批准快照")
	}
	metadata := cloneMetadata(input.Metadata)
	metadata["content_profile_id"] = input.ContentProfileID
	metadata["douyin_commerce"] = refs
	return metadata, nil
}

func (s *Service) validateDouyinApprovedInputs(ctx context.Context, tenantID, projectID string, refs domain.DouyinCommercePublicationRefs) error {
	strategyRaw, strategySnapshot, err := s.approvedObject(ctx, tenantID, projectID, "strategy", refs.AudienceStrategyApprovedSnapshotID, refs.AudienceStrategyVersionID)
	if err != nil {
		return err
	}
	var strategy domain.AudienceStrategyVersion
	if json.Unmarshal(strategyRaw, &strategy) != nil || strategy.Type != "audience_strategy_version" {
		return domain.Invalid("DOUYIN_COMMERCE_AUDIENCE_INVALID", "人群批准快照不包含有效策略版本")
	}
	if err := strategy.Validate(true); err != nil {
		return err
	}
	offerRaw, offerSnapshot, err := s.approvedObject(ctx, tenantID, projectID, "offer", refs.OfferApprovedSnapshotID, refs.OfferSnapshotID)
	if err != nil {
		return err
	}
	var offer domain.CommerceOfferSnapshot
	if json.Unmarshal(offerRaw, &offer) != nil || offer.Type != "commerce_offer_snapshot" {
		return domain.Invalid("DOUYIN_COMMERCE_OFFER_INVALID", "商品批准快照不包含有效 Offer")
	}
	if err := offer.Validate(refs.ValidationReceipt.ScheduledAt, true); err != nil {
		return err
	}
	contentRaw, contentSnapshot, err := s.approvedObject(ctx, tenantID, projectID, "content_batch", refs.ContentApprovedSnapshotID, refs.ContentItemID)
	if err != nil {
		return err
	}
	rendered, err := localworkspace.RenderContentItem(contentRaw)
	if err != nil {
		return err
	}
	if rendered.ContentHash != refs.ValidationReceipt.ContentItemDigest || rendered.Item.Channel != "douyin" || rendered.Item.ProjectID != projectID {
		return domain.Conflict("DOUYIN_COMMERCE_CONTENT_LINEAGE_INVALID", "ContentItem 摘要、渠道或项目与校验回执不一致")
	}
	voiceover, onScreen := douyinContentTextForServer(rendered.Item)
	voiceDigest, err := domain.DouyinTextDigest(voiceover)
	if err != nil || voiceDigest != refs.ValidationReceipt.VoiceoverTextDigest {
		return domain.Conflict("DOUYIN_COMMERCE_VOICEOVER_DIGEST_INVALID", "口播文本摘要与校验回执不一致")
	}
	onScreenDigest, err := domain.DouyinTextDigest(onScreen)
	if err != nil || onScreenDigest != refs.ValidationReceipt.OnScreenTextDigest {
		return domain.Conflict("DOUYIN_COMMERCE_ONSCREEN_DIGEST_INVALID", "字幕文本摘要与校验回执不一致")
	}
	receiptOffer := refs.ValidationReceipt.Offer
	if receiptOffer.SKUID != offer.SKUID || receiptOffer.ProductVersionID != offer.ProductVersionID || receiptOffer.DisplayPrice != strings.TrimSpace(offer.DisplayPrice) || receiptOffer.Currency != strings.ToUpper(strings.TrimSpace(offer.Currency)) || !sameStringSet(receiptOffer.Benefits, offer.Benefits) || !sameStringSet(receiptOffer.Conditions, offer.Conditions) || !stringSubset(refs.ValidationReceipt.ObservedBenefits, offer.Benefits) || !stringSubset(refs.ValidationReceipt.ObservedConditions, offer.Conditions) {
		return domain.Conflict("DOUYIN_COMMERCE_OFFER_FACTS_INVALID", "校验回执中的商品价格、币种、权益或条件与批准 Offer 不一致")
	}
	if refs.ValidationReceipt.ProjectID != projectID || refs.ValidationReceipt.ContentProfileID != domain.DouyinCommerceProfileID {
		return domain.Conflict("DOUYIN_COMMERCE_RECEIPT_SCOPE_INVALID", "校验回执的项目、Profile 或时间格式不一致")
	}
	if refs.ValidationReceipt.AudienceStrategyApprovedSnapshotID != strategySnapshot.ID || refs.ValidationReceipt.OfferApprovedSnapshotID != offerSnapshot.ID || refs.ValidationReceipt.ContentApprovedSnapshotID != contentSnapshot.ID {
		return domain.Conflict("DOUYIN_COMMERCE_SNAPSHOT_LINEAGE_INVALID", "校验回执中的批准快照引用与实际对象不一致")
	}
	storyboardRaw, storyboardSnapshot, err := s.approvedObject(ctx, tenantID, projectID, "storyboard", refs.StoryboardApprovedSnapshotID, refs.StoryboardPackageID)
	if err != nil {
		return err
	}
	var storyboard domain.StoryboardPackage
	if json.Unmarshal(storyboardRaw, &storyboard) != nil || storyboard.Type != "storyboard_package" {
		return domain.Invalid("DOUYIN_COMMERCE_STORYBOARD_INVALID", "分镜批准快照不包含有效分镜包")
	}
	if err := storyboard.Validate(true); err != nil {
		return err
	}
	lockedDigest, err := storyboard.ComputedLockedDigest()
	if err != nil || lockedDigest != storyboard.LockedDigest || lockedDigest != refs.ValidationReceipt.StoryboardLockedDigest || storyboard.ApprovedSnapshotID != contentSnapshot.ID || storyboard.ContentItemID != rendered.Item.ID {
		return domain.Conflict("DOUYIN_COMMERCE_STORYBOARD_LINEAGE_INVALID", "分镜锁定摘要、内容项或批准快照血缘不一致")
	}
	if refs.ValidationReceipt.StoryboardApprovedSnapshotID != storyboardSnapshot.ID || refs.ValidationReceipt.StoryboardPackageID != storyboard.ID {
		return domain.Conflict("DOUYIN_COMMERCE_STORYBOARD_REF_INVALID", "校验回执中的分镜引用与批准对象不一致")
	}
	if strategy.ProjectID != projectID || offer.ProjectID != projectID || contentSnapshot.ProjectID != projectID || storyboardSnapshot.ProjectID != projectID {
		return domain.Conflict("DOUYIN_COMMERCE_PROJECT_MISMATCH", "抖音电商批准对象不属于当前项目")
	}
	return nil
}

func (s *Service) approvedObject(ctx context.Context, tenantID, projectID, submissionType, snapshotID, objectID string) (json.RawMessage, domain.ApprovedSnapshot, error) {
	snapshot, err := s.store.ApprovedSnapshot(ctx, tenantID, snapshotID)
	if err != nil {
		return nil, domain.ApprovedSnapshot{}, err
	}
	if snapshot.ProjectID != projectID || snapshot.SubmissionType != submissionType || !containsString(snapshot.EligibleIDs, objectID) {
		return nil, snapshot, domain.Conflict("DOUYIN_COMMERCE_APPROVED_REF_INVALID", "批准快照不属于当前项目、类型不匹配或未包含指定对象")
	}
	var canonical struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(snapshot.CanonicalContent, &canonical); err != nil {
		return nil, snapshot, domain.Invalid("DOUYIN_COMMERCE_APPROVED_SNAPSHOT_INVALID", "批准快照缺少规范对象列表")
	}
	for _, raw := range canonical.Objects {
		var identity struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &identity) == nil && identity.ID == objectID {
			return raw, snapshot, nil
		}
	}
	return nil, snapshot, domain.NotFound("抖音电商批准对象")
}

func douyinContentTextForServer(content localworkspace.ContentItem) (string, string) {
	voiceover, onScreen := make([]string, 0, len(content.Shots)), make([]string, 0, len(content.Shots))
	for _, shot := range content.Shots {
		voiceover = append(voiceover, strings.TrimSpace(shot.Voiceover))
		onScreen = append(onScreen, strings.TrimSpace(shot.OnScreenText))
	}
	return strings.Join(voiceover, "\n"), strings.Join(onScreen, "\n")
}

func normalizedArtifactDigest(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(value, "sha256:") {
		value = strings.TrimPrefix(value, "sha256:")
	}
	if len(value) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return "sha256:" + value
}

func stringSubset(values, allowed []string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[strings.TrimSpace(value)] = struct{}{}
	}
	for _, value := range values {
		if _, ok := set[strings.TrimSpace(value)]; !ok {
			return false
		}
	}
	return true
}

func cloneMetadata(input map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range input {
		result[key] = value
	}
	return result
}
