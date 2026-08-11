package localworkspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

var (
	commerceCNYPricePattern  = regexp.MustCompile(`(?:[¥￥]\s*([0-9]+(?:\.[0-9]+)?)|([0-9]+(?:\.[0-9]+)?)\s*元)`)
	commerceCodePricePattern = regexp.MustCompile(`\b([A-Z]{3})\s*([0-9]+(?:\.[0-9]+)?)\b`)
	commercePromotionPattern = regexp.MustCompile(`(?:优惠|优惠券|赠品|赠送|满[0-9]+减[0-9]+|库存|包邮|折扣)`)
)

type ValidateDouyinCommerceOptions struct {
	Root                               string
	AudienceStrategyApprovedSnapshotID string
	AudienceStrategyVersionID          string
	OfferApprovedSnapshotID            string
	OfferSnapshotID                    string
	ContentApprovedSnapshotID          string
	ContentItemID                      string
	StoryboardApprovedSnapshotID       string
	StoryboardPackageID                string
	RenderedCreativeArtifactID         string
	RenderedCreativeFile               string
	LandingPageTextFile                string
	ObservedBenefits                   []string
	ObservedConditions                 []string
	AccountRef                         string
	ProductAnchorRef                   string
	LandingPageRef                     string
	ScheduledAt                        time.Time
	ValidatedAt                        time.Time
	OutputFile                         string
}

type DouyinCommerceValidationResult struct {
	Path    string                                 `json:"path"`
	Receipt domain.DouyinCommerceValidationReceipt `json:"receipt"`
}

func ValidateDouyinCommerce(options ValidateDouyinCommerceOptions) (DouyinCommerceValidationResult, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return DouyinCommerceValidationResult{}, err
	}
	receipt, err := buildDouyinCommerceReceipt(root, options)
	if err != nil {
		return DouyinCommerceValidationResult{}, err
	}
	output := strings.TrimSpace(options.OutputFile)
	if output == "" {
		output = filepath.Join("60-delivery", "validations", localSafeName(receipt.ContentItemID)+".douyin-commerce.json")
	}
	path, err := douyinOutputPath(root, output)
	if err != nil {
		return DouyinCommerceValidationResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return DouyinCommerceValidationResult{}, err
	}
	if err := writeJSON(path, receipt); err != nil {
		return DouyinCommerceValidationResult{}, err
	}
	return DouyinCommerceValidationResult{Path: relativeWorkspacePath(root, path), Receipt: receipt}, nil
}

func douyinOutputPath(root, output string) (string, error) {
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := output
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootPath, filepath.FromSlash(path))
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(rootPath, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", domain.Policy("LOCAL_FILE_OUTSIDE_WORKSPACE", "校验回执必须位于当前工作区", "使用 60-delivery/validations 下的输出路径")
	}
	return filepath.Clean(absolute), nil
}

func LintDouyinCommerceReceipt(root, receiptFile, renderedCreativeFile, landingPageTextFile string) (V5LintReport, domain.DouyinCommerceValidationReceipt, error) {
	resolved, path, err := resolveV5JSON(root, receiptFile)
	if err != nil {
		return V5LintReport{}, domain.DouyinCommerceValidationReceipt{}, err
	}
	var receipt domain.DouyinCommerceValidationReceipt
	if err := readStrictJSON(path, &receipt); err != nil {
		return V5LintReport{}, receipt, domain.Invalid("DOUYIN_COMMERCE_RECEIPT_JSON_INVALID", err.Error())
	}
	report := v5Report(resolved, path, receipt.ContentItemID, "douyin_commerce_validation")
	options := ValidateDouyinCommerceOptions{
		Root:                               resolved,
		AudienceStrategyApprovedSnapshotID: receipt.AudienceStrategyApprovedSnapshotID,
		AudienceStrategyVersionID:          receipt.AudienceStrategyVersionID,
		OfferApprovedSnapshotID:            receipt.OfferApprovedSnapshotID,
		OfferSnapshotID:                    receipt.OfferSnapshotID,
		ContentApprovedSnapshotID:          receipt.ContentApprovedSnapshotID,
		ContentItemID:                      receipt.ContentItemID,
		StoryboardApprovedSnapshotID:       receipt.StoryboardApprovedSnapshotID,
		StoryboardPackageID:                receipt.StoryboardPackageID,
		RenderedCreativeArtifactID:         receipt.RenderedCreativeArtifactID,
		RenderedCreativeFile:               renderedCreativeFile,
		LandingPageTextFile:                landingPageTextFile,
		ObservedBenefits:                   receipt.ObservedBenefits,
		ObservedConditions:                 receipt.ObservedConditions,
		AccountRef:                         receipt.AccountRef,
		ProductAnchorRef:                   receipt.ProductAnchorRef,
		LandingPageRef:                     receipt.LandingPageRef,
		ScheduledAt:                        receipt.ScheduledAt,
		ValidatedAt:                        receipt.ValidatedAt,
	}
	recomputed, buildErr := buildDouyinCommerceReceipt(resolved, options)
	if buildErr != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: domainErrorCode(buildErr), Message: buildErr.Error()})
	} else if recomputed.ReceiptDigest != receipt.ReceiptDigest {
		report.Issues = append(report.Issues, V5LintIssue{Code: "DOUYIN_COMMERCE_RECEIPT_INPUT_DRIFT", Message: "批准输入、最终媒体或落地页文本已偏离校验回执"})
	}
	if err := receipt.Validate(); err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: domainErrorCode(err), Message: err.Error()})
	}
	return finishV5Report(report, receipt), receipt, nil
}

func buildDouyinCommerceReceipt(root string, options ValidateDouyinCommerceOptions) (domain.DouyinCommerceValidationReceipt, error) {
	scheduledAt, validatedAt := options.ScheduledAt.UTC(), localNow(options.ValidatedAt)
	if scheduledAt.IsZero() {
		return domain.DouyinCommerceValidationReceipt{}, domain.Invalid("DOUYIN_COMMERCE_SCHEDULE_REQUIRED", "抖音电商校验必须固定计划发布时间")
	}

	strategyRaw, strategySnapshot, err := exactApprovedObject(root, "strategy", options.AudienceStrategyApprovedSnapshotID, options.AudienceStrategyVersionID)
	if err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	var strategy domain.AudienceStrategyVersion
	if json.Unmarshal(strategyRaw, &strategy) != nil || strategy.Type != "audience_strategy_version" {
		return domain.DouyinCommerceValidationReceipt{}, domain.Invalid("DOUYIN_COMMERCE_AUDIENCE_INVALID", "人群策略批准快照不包含有效 AudienceStrategyVersion")
	}
	if err := strategy.Validate(true); err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}

	offerRaw, offerSnapshot, err := exactApprovedObject(root, "offer", options.OfferApprovedSnapshotID, options.OfferSnapshotID)
	if err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	var offer domain.CommerceOfferSnapshot
	if json.Unmarshal(offerRaw, &offer) != nil || offer.Type != "commerce_offer_snapshot" {
		return domain.DouyinCommerceValidationReceipt{}, domain.Invalid("DOUYIN_COMMERCE_OFFER_INVALID", "商品方案批准快照不包含有效 CommerceOfferSnapshot")
	}
	if err := offer.Validate(scheduledAt, true); err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}

	contentRaw, contentSnapshot, err := exactApprovedObject(root, "content_batch", options.ContentApprovedSnapshotID, options.ContentItemID)
	if err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	rendered, err := RenderContentItem(contentRaw)
	if err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	content := rendered.Item
	if strings.ToLower(strings.TrimSpace(content.Channel)) != "douyin" {
		return domain.DouyinCommerceValidationReceipt{}, domain.Policy("DOUYIN_COMMERCE_CONTENT_CHANNEL_INVALID", "抖音电商校验只接受 channel=douyin 的已批准内容", "选择抖音内容项或重新提交审核")
	}

	storyboardRaw, storyboardSnapshot, err := exactApprovedObject(root, "storyboard", options.StoryboardApprovedSnapshotID, options.StoryboardPackageID)
	if err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	var storyboard domain.StoryboardPackage
	if json.Unmarshal(storyboardRaw, &storyboard) != nil || storyboard.Type != "storyboard_package" {
		return domain.DouyinCommerceValidationReceipt{}, domain.Invalid("DOUYIN_COMMERCE_STORYBOARD_INVALID", "分镜批准快照不包含有效 StoryboardPackage")
	}
	if err := storyboard.Validate(true); err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	lockedDigest, err := storyboard.ComputedLockedDigest()
	if err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	if storyboard.LockedDigest != lockedDigest || storyboard.ApprovedSnapshotID != contentSnapshot.ID || storyboard.ContentItemID != content.ID {
		return domain.DouyinCommerceValidationReceipt{}, domain.Conflict("DOUYIN_COMMERCE_STORYBOARD_LINEAGE_INVALID", "分镜摘要或 ContentItem 批准血缘不一致")
	}

	for _, value := range []struct {
		name string
		got  string
	}{
		{"strategy", strategy.ProjectID}, {"offer", offer.ProjectID}, {"content", content.ProjectID}, {"storyboard", storyboard.ProjectID},
	} {
		if value.got != strategy.ProjectID {
			return domain.DouyinCommerceValidationReceipt{}, domain.Conflict("DOUYIN_COMMERCE_PROJECT_MISMATCH", value.name+" 不属于同一项目")
		}
	}

	voiceover, onScreen := douyinContentText(content)
	landingPath, err := resolveWorkspaceFile(root, options.LandingPageTextFile)
	if err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	landingBody, err := os.ReadFile(landingPath)
	if err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	landingText := strings.TrimSpace(string(landingBody))
	if landingText == "" {
		return domain.DouyinCommerceValidationReceipt{}, domain.Invalid("DOUYIN_COMMERCE_LANDING_TEXT_REQUIRED", "落地页文本不能为空")
	}
	observedBenefits := sortedUniqueStrings(options.ObservedBenefits)
	observedConditions := sortedUniqueStrings(options.ObservedConditions)
	allText := strings.Join([]string{voiceover, onScreen, landingText}, "\n")
	if err := validateDouyinOfferText(allText, offer, observedBenefits, observedConditions); err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	if err := validateDouyinShotClaims(content, offer); err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}

	artifactPath, err := resolveWorkspaceFile(root, options.RenderedCreativeFile)
	if err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	artifactBody, err := os.ReadFile(artifactPath)
	if err != nil {
		return domain.DouyinCommerceValidationReceipt{}, err
	}
	info, err := os.Stat(artifactPath)
	if err != nil || !info.Mode().IsRegular() || len(artifactBody) == 0 {
		return domain.DouyinCommerceValidationReceipt{}, domain.Invalid("DOUYIN_COMMERCE_ARTIFACT_INVALID", "最终成片必须是工作区内非空普通文件")
	}
	artifactSum := sha256.Sum256(artifactBody)
	voiceDigest, _ := domain.DouyinTextDigest(voiceover)
	onScreenDigest, _ := domain.DouyinTextDigest(onScreen)
	landingDigest, _ := domain.DouyinTextDigest(landingText)

	receipt := domain.DouyinCommerceValidationReceipt{
		SchemaVersion: domain.DouyinCommerceValidationSchema, ContentProfileID: domain.DouyinCommerceProfileID, ProjectID: strategy.ProjectID,
		AudienceStrategyApprovedSnapshotID: strategySnapshot.ID, AudienceStrategyVersionID: strategy.ID,
		OfferApprovedSnapshotID: offerSnapshot.ID, OfferSnapshotID: offer.ID,
		ContentApprovedSnapshotID: contentSnapshot.ID, ContentItemID: content.ID, ContentItemDigest: rendered.ContentHash,
		StoryboardApprovedSnapshotID: storyboardSnapshot.ID, StoryboardPackageID: storyboard.ID, StoryboardLockedDigest: storyboard.LockedDigest,
		RenderedCreativeArtifactID: strings.TrimSpace(options.RenderedCreativeArtifactID), RenderedCreativeDigest: "sha256:" + hex.EncodeToString(artifactSum[:]),
		VoiceoverTextDigest: voiceDigest, OnScreenTextDigest: onScreenDigest, LandingPageTextDigest: landingDigest,
		Offer:            domain.DouyinCommerceOfferFacts{SKUID: offer.SKUID, ProductVersionID: offer.ProductVersionID, DisplayPrice: strings.TrimSpace(offer.DisplayPrice), Currency: strings.ToUpper(strings.TrimSpace(offer.Currency)), Benefits: sortedUniqueStrings(offer.Benefits), Conditions: sortedUniqueStrings(offer.Conditions)},
		ObservedBenefits: observedBenefits, ObservedConditions: observedConditions,
		AccountRef: strings.TrimSpace(options.AccountRef), ProductAnchorRef: strings.TrimSpace(options.ProductAnchorRef), LandingPageRef: strings.TrimSpace(options.LandingPageRef),
		ScheduledAt: scheduledAt, ValidatedAt: validatedAt,
	}
	receipt.ReceiptDigest, err = receipt.ComputedDigest()
	if err != nil {
		return receipt, err
	}
	if err := receipt.Validate(); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func exactApprovedObject(root, submissionType, snapshotID, objectID string) (json.RawMessage, domain.ApprovedSnapshot, error) {
	record, err := ShowApprovedSnapshot(root, strings.TrimSpace(snapshotID))
	if err != nil {
		return nil, domain.ApprovedSnapshot{}, err
	}
	snapshot := record.Snapshot
	if snapshot.SubmissionType != submissionType || !containsLocalString(snapshot.EligibleIDs, objectID) {
		return nil, snapshot, domain.Conflict("DOUYIN_COMMERCE_APPROVED_REF_INVALID", "批准快照类型或 eligible_ids 与指定对象不一致")
	}
	var canonical struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if json.Unmarshal(snapshot.CanonicalContent, &canonical) != nil {
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

func douyinContentText(content ContentItem) (string, string) {
	voiceover, onScreen := make([]string, 0, len(content.Shots)), make([]string, 0, len(content.Shots))
	for _, shot := range content.Shots {
		voiceover = append(voiceover, strings.TrimSpace(shot.Voiceover))
		onScreen = append(onScreen, strings.TrimSpace(shot.OnScreenText))
	}
	return strings.Join(voiceover, "\n"), strings.Join(onScreen, "\n")
}

func validateDouyinShotClaims(content ContentItem, offer domain.CommerceOfferSnapshot) error {
	approved := make(map[string]bool, len(offer.ApprovedClaimRefs))
	for _, ref := range offer.ApprovedClaimRefs {
		approved[ref] = true
	}
	for _, shot := range content.Shots {
		if !dynamicOfferTextPattern.MatchString(shot.Voiceover+" "+shot.OnScreenText) && !commercePromotionPattern.MatchString(shot.Voiceover+" "+shot.OnScreenText) {
			continue
		}
		matched := false
		for _, ref := range shot.ClaimRefs {
			matched = matched || approved[ref]
		}
		if !matched {
			return domain.Policy("DOUYIN_COMMERCE_DYNAMIC_CLAIM_REQUIRED", "镜头 "+shot.ShotID+" 的动态商品文案未引用 Offer 批准 Claim", "为该镜头补充 Offer approved_claim_refs 中的引用并重新审核")
		}
	}
	return nil
}

func validateDouyinOfferText(text string, offer domain.CommerceOfferSnapshot, benefits, conditions []string) error {
	offerPrice, err := normalizedPrice(offer.DisplayPrice)
	if err != nil {
		return domain.Invalid("DOUYIN_COMMERCE_OFFER_PRICE_INVALID", "Offer display_price 无法规范化")
	}
	for _, match := range commerceCNYPricePattern.FindAllStringSubmatch(text, -1) {
		if strings.ToUpper(offer.Currency) != "CNY" || normalizedPriceValue(firstNonEmpty(match[1], match[2])) != offerPrice {
			return domain.Conflict("DOUYIN_COMMERCE_PRICE_MISMATCH", "口播、字幕或落地页中的人民币价格与批准 Offer 不一致")
		}
	}
	for _, match := range commerceCodePricePattern.FindAllStringSubmatch(text, -1) {
		if match[1] != strings.ToUpper(offer.Currency) || normalizedPriceValue(match[2]) != offerPrice {
			return domain.Conflict("DOUYIN_COMMERCE_PRICE_MISMATCH", "口播、字幕或落地页中的币种或价格与批准 Offer 不一致")
		}
	}
	if commercePromotionPattern.MatchString(text) && len(benefits)+len(conditions) == 0 {
		return domain.Policy("DOUYIN_COMMERCE_PROMOTION_FACT_REQUIRED", "内容包含优惠、赠品、库存或条件文案，但没有声明结构化观察事实", "使用 --benefit/--condition 声明正文中的 Offer 允许事实")
	}
	if err := validateObservedOfferFacts(text, benefits, offer.Benefits, "BENEFIT"); err != nil {
		return err
	}
	return validateObservedOfferFacts(text, conditions, offer.Conditions, "CONDITION")
}

func validateObservedOfferFacts(text string, observed, allowed []string, kind string) error {
	allowedSet := make(map[string]bool, len(allowed))
	for _, value := range allowed {
		allowedSet[strings.TrimSpace(value)] = true
	}
	for _, value := range observed {
		if !allowedSet[value] {
			return domain.Policy("DOUYIN_COMMERCE_"+kind+"_NOT_ALLOWED", "内容声明了 Offer 未允许的权益或条件："+value, "更新 Offer 并重新批准，或移除该文案")
		}
		if !strings.Contains(text, value) {
			return domain.Conflict("DOUYIN_COMMERCE_"+kind+"_NOT_IN_TEXT", "结构化观察事实未出现在口播、字幕或落地页："+value)
		}
	}
	return nil
}

func normalizedPrice(value string) (string, error) {
	match := regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`).FindString(strings.ReplaceAll(value, ",", ""))
	if match == "" {
		return "", fmt.Errorf("price missing")
	}
	return normalizedPriceValue(match), nil
}

func normalizedPriceValue(value string) string {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatFloat(number, 'f', -1, 64)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func sortedUniqueStrings(values []string) []string {
	result := uniqueStrings(values)
	sort.Strings(result)
	return result
}

func containsLocalString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
