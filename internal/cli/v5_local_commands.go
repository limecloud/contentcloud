package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

const localWorkspaceExecutionPlane = "local_workspace"

func (r *Root) localAudienceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "audience", Short: "创建和校验本地抖音人群策略候选方案"}

	taxonomy := &cobra.Command{Use: "taxonomy", Short: "校验已拉取到工作区且由服务端治理的人群目录"}
	var taxonomyDirectory string
	taxonomyLint := &cobra.Command{Use: "lint <taxonomy.json>", Args: cobra.ExactArgs(1), Short: "校验已拉取并经人工确认的人群目录", RunE: func(cmd *cobra.Command, args []string) error {
		report, value, err := localworkspace.LintAudienceTaxonomy(taxonomyDirectory, args[0], r.currentTime())
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("AUDIENCE_TAXONOMY_LINT_FAILED", "人群目录确定性校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.audience.taxonomy.lint", localExecutionResult(map[string]any{"taxonomy": value, "report": report}))
	}}
	taxonomyLint.Flags().StringVar(&taxonomyDirectory, "directory", "", "工作区路径；默认使用当前目录")
	taxonomy.AddCommand(taxonomyLint)

	strategy := &cobra.Command{Use: "strategy", Short: "创建和校验本地人群策略候选方案"}
	var scaffoldDirectory, taxonomyID, mode, objective, testType, primaryVariable string
	var audiences []string
	scaffold := &cobra.Command{Use: "scaffold", Args: cobra.NoArgs, Short: "根据已拉取的批准快照人群目录创建策略候选方案", RunE: func(cmd *cobra.Command, args []string) error {
		paths, values, err := localworkspace.ScaffoldAudienceStrategies(localworkspace.ScaffoldAudienceStrategiesOptions{
			Root: scaffoldDirectory, TaxonomySnapshotID: taxonomyID, Mode: mode, AudienceCodes: audiences,
			Objective: objective, TestType: testType, PrimaryVariable: primaryVariable,
		})
		if err != nil {
			return err
		}
		return r.writeOK("local.audience.strategy.scaffold", localExecutionResult(map[string]any{
			"paths": valuesOrEmpty(paths), "strategies": values, "next_action": "补齐证据和策略字段，执行 lint 后再显式发布 strategy",
		}))
	}}
	scaffold.Flags().StringVar(&scaffoldDirectory, "directory", "", "工作区路径；默认使用当前目录")
	scaffold.Flags().StringVar(&taxonomyID, "taxonomy", "", "已拉取策略批准快照中的对象 ID")
	scaffold.Flags().StringVar(&mode, "mode", "single", "策略模式：single、compare 或 explore")
	scaffold.Flags().StringSliceVar(&audiences, "audience", nil, "人群编码；single 传一个，compare 传两个或三个")
	scaffold.Flags().StringVar(&objective, "objective", "", "商业目标")
	scaffold.Flags().StringVar(&testType, "test-type", "", "测试类型：strict_ab、exploration_batch 或 audience_expression_fit_test")
	scaffold.Flags().StringVar(&primaryVariable, "primary-variable", "", "实验主变量")

	var strategyDirectory string
	strategyLint := &cobra.Command{Use: "lint <strategy.json>", Args: cobra.ExactArgs(1), Short: "校验可提交审核的人群策略候选方案", RunE: func(cmd *cobra.Command, args []string) error {
		report, value, err := localworkspace.LintAudienceStrategy(strategyDirectory, args[0], r.currentTime())
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("AUDIENCE_STRATEGY_LINT_FAILED", "人群策略确定性校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.audience.strategy.lint", localExecutionResult(map[string]any{"strategy": value, "report": report}))
	}}
	strategyLint.Flags().StringVar(&strategyDirectory, "directory", "", "工作区路径；默认使用当前目录")
	strategy.AddCommand(scaffold, strategyLint)

	cmd.AddCommand(taxonomy, strategy)
	return cmd
}

func (r *Root) localOfferCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "offer", Short: "根据有效时间校验本地商业报价快照"}
	var directory string
	var at string
	lint := &cobra.Command{Use: "lint <offer.json>", Args: cobra.ExactArgs(1), Short: "在渲染或发布前校验已确认的报价", RunE: func(cmd *cobra.Command, args []string) error {
		checkAt := r.currentTime()
		if at != "" {
			parsed, err := time.Parse(time.RFC3339, at)
			if err != nil {
				return domain.Invalid("OFFER_AT_INVALID", "--at 必须是 RFC3339 时间")
			}
			checkAt = parsed
		}
		report, value, err := localworkspace.LintCommerceOffer(directory, args[0], checkAt)
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("COMMERCE_OFFER_LINT_FAILED", "商业报价确定性校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.offer.lint", localExecutionResult(map[string]any{"offer": value, "report": report, "checked_at": checkAt.UTC()}))
	}}
	lint.Flags().StringVar(&directory, "directory", "", "工作区路径；默认使用当前目录")
	lint.Flags().StringVar(&at, "at", "", "RFC3339 格式的校验时间；默认使用当前时间")
	cmd.AddCommand(lint)
	return cmd
}

func (r *Root) localStoryboardCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "storyboard", Short: "根据已批准内容创建和校验本地分镜候选方案"}

	var createDirectory, snapshotID, contentItemID, packageID, capabilityID, capabilityVersion, capabilityDigest string
	create := &cobra.Command{Use: "create", Args: cobra.NoArgs, Short: "根据已拉取的内容批准快照创建分镜候选方案", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.CreateStoryboardPackage(localworkspace.CreateStoryboardPackageOptions{
			Root: createDirectory, ApprovedSnapshotID: snapshotID, ContentItemID: contentItemID, PackageID: packageID,
			Capability: domain.CapabilityRef{ID: capabilityID, Version: capabilityVersion, Digest: capabilityDigest},
		})
		if err != nil {
			return err
		}
		return r.writeOK("local.storyboard.create", localExecutionResult(map[string]any{
			"storyboard": result, "next_action": "逐镜头生成 first-frame，可选生成 end-frame，并生成 review-sheet 后执行 local storyboard prepare",
		}))
	}}
	create.Flags().StringVar(&createDirectory, "directory", "", "工作区路径；默认使用当前目录")
	create.Flags().StringVar(&snapshotID, "snapshot", "", "已拉取的 content_batch 批准快照 ID")
	create.Flags().StringVar(&contentItemID, "content-item", "", "已批准的 ContentItem ID")
	create.Flags().StringVar(&packageID, "id", "", "可选的分镜包 ID")
	create.Flags().StringVar(&capabilityID, "capability-id", "", "本地图片生成能力 ID")
	create.Flags().StringVar(&capabilityVersion, "capability-version", "", "本地图片生成能力版本")
	create.Flags().StringVar(&capabilityDigest, "capability-digest", "", "带 sha256: 前缀的本地能力摘要")

	var prepareDirectory string
	prepare := &cobra.Command{Use: "prepare <manifest.json>", Args: cobra.ExactArgs(1), Short: "发现已生成媒体，并准备可提交服务端审核的候选方案", RunE: func(cmd *cobra.Command, args []string) error {
		report, value, err := localworkspace.PrepareStoryboardReview(prepareDirectory, args[0])
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("STORYBOARD_PREPARE_FAILED", "分镜审核包准备失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.storyboard.prepare", localExecutionResult(map[string]any{
			"storyboard": value, "report": report, "next_action": "执行 publish storyboard；只有服务端批准后拉取的 storyboard ApprovedSnapshot 才代表已锁定",
		}))
	}}
	prepare.Flags().StringVar(&prepareDirectory, "directory", "", "工作区路径；默认使用当前目录")

	var lintDirectory string
	lint := &cobra.Command{Use: "lint <manifest.json>", Args: cobra.ExactArgs(1), Short: "发布前检查分镜媒体、权利信息和锁定摘要", RunE: func(cmd *cobra.Command, args []string) error {
		report, value, err := localworkspace.LintStoryboardPackage(lintDirectory, args[0])
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("STORYBOARD_LINT_FAILED", "分镜包确定性校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.storyboard.lint", localExecutionResult(map[string]any{"storyboard": value, "report": report}))
	}}
	lint.Flags().StringVar(&lintDirectory, "directory", "", "工作区路径；默认使用当前目录")

	cmd.AddCommand(create, prepare, lint)
	return cmd
}

func (r *Root) localSeedanceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "seedance", Short: "根据已拉取并锁定的分镜快照导出可直接使用的 Seedance 包"}
	var directory, snapshotID, storyboardID, packageID, profileVersion, adapterID, adapterVersion, adapterDigest, mode, aspectRatio, sound string
	var minDuration, maxDuration, maxImages, maxVideos, maxAudios int
	export := &cobra.Command{Use: "export", Args: cobra.NoArgs, Short: "在本地汇总提示词、上传映射、媒体文件和操作说明", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.ExportSeedancePackage(localworkspace.ExportSeedancePackageOptions{
			Root: directory, StoryboardSnapshotID: snapshotID, StoryboardPackageID: storyboardID, PackageID: packageID,
			ProviderProfileVersion: profileVersion, AdapterCapability: domain.CapabilityRef{ID: adapterID, Version: adapterVersion, Digest: adapterDigest},
			Mode: mode, AspectRatio: aspectRatio, Sound: sound, MinDurationSeconds: minDuration, MaxDurationSeconds: maxDuration,
			MaxImages: maxImages, MaxVideos: maxVideos, MaxAudios: maxAudios,
		})
		if err != nil {
			return err
		}
		return r.writeOK("local.seedance.export", localExecutionResult(map[string]any{
			"delivery": result, "authority": "validated_local_delivery", "next_action": "请按 README 在 Seedance 中手动上传，核对编号后逐段复制提示词",
		}))
	}}
	export.Flags().StringVar(&directory, "directory", "", "工作区路径；默认使用当前目录")
	export.Flags().StringVar(&snapshotID, "snapshot", "", "已拉取的分镜批准快照 ID")
	export.Flags().StringVar(&storyboardID, "storyboard", "", "符合条件的 StoryboardPackage 对象 ID")
	export.Flags().StringVar(&packageID, "id", "", "可选的不可变交付包 ID")
	export.Flags().StringVar(&profileVersion, "profile-version", "", "经人工确认的 Seedance 服务配置版本")
	export.Flags().StringVar(&adapterID, "adapter-id", "contentcloud.seedance-export", "适配器能力 ID")
	export.Flags().StringVar(&adapterVersion, "adapter-version", "1.0.0", "适配器能力版本")
	export.Flags().StringVar(&adapterDigest, "adapter-digest", "", "带 sha256: 前缀的适配器能力摘要")
	export.Flags().StringVar(&mode, "mode", "all_reference", "生成模式：first_last_frame、all_reference 或 extend")
	export.Flags().StringVar(&aspectRatio, "aspect-ratio", "9:16", "服务提供方使用的画面比例")
	export.Flags().StringVar(&sound, "sound", "environment_only", "服务提供方使用的声音设置")
	export.Flags().IntVar(&minDuration, "min-duration", 0, "已确认的单段最短生成秒数；必填")
	export.Flags().IntVar(&maxDuration, "max-duration", 0, "已确认的单段最长生成秒数；必填")
	export.Flags().IntVar(&maxImages, "max-images", 0, "已确认的最大图片参考数量；必填")
	export.Flags().IntVar(&maxVideos, "max-videos", 0, "已确认的最大视频参考数量")
	export.Flags().IntVar(&maxAudios, "max-audios", 0, "已确认的最大音频参考数量")

	var lintDirectory string
	lint := &cobra.Command{Use: "lint <package.json>", Args: cobra.ExactArgs(1), Short: "重新校验本地 Seedance 包及其锁定输入", RunE: func(cmd *cobra.Command, args []string) error {
		report, value, err := localworkspace.LintSeedancePackage(lintDirectory, args[0])
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("SEEDANCE_PACKAGE_LINT_FAILED", "Seedance 交付包确定性校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.seedance.lint", localExecutionResult(map[string]any{"authority": "validated_local_delivery", "package": value, "report": report}))
	}}
	lint.Flags().StringVar(&lintDirectory, "directory", "", "工作区路径；默认使用当前目录")
	cmd.AddCommand(export, lint)
	return cmd
}

func (r *Root) localDouyinCommerceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "douyin-commerce", Short: "校验抖音电商最终成片、商品事实和发布血缘"}

	var directory, audienceSnapshotID, audienceID, offerSnapshotID, offerID, contentSnapshotID, contentID string
	var storyboardSnapshotID, storyboardID, artifactID, artifactFile, landingTextFile, accountRef, anchorRef, landingRef, outputFile string
	var benefits, conditions []string
	var scheduledAt time.Time
	validate := &cobra.Command{Use: "validate", Args: cobra.NoArgs, Short: "生成不可变的抖音电商发布前校验回执", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(directory, domain.ContentTypeVideoScript); err != nil {
			return err
		}
		result, err := localworkspace.ValidateDouyinCommerce(localworkspace.ValidateDouyinCommerceOptions{
			Root:                               directory,
			AudienceStrategyApprovedSnapshotID: audienceSnapshotID, AudienceStrategyVersionID: audienceID,
			OfferApprovedSnapshotID: offerSnapshotID, OfferSnapshotID: offerID,
			ContentApprovedSnapshotID: contentSnapshotID, ContentItemID: contentID,
			StoryboardApprovedSnapshotID: storyboardSnapshotID, StoryboardPackageID: storyboardID,
			RenderedCreativeArtifactID: artifactID, RenderedCreativeFile: artifactFile, LandingPageTextFile: landingTextFile,
			ObservedBenefits: benefits, ObservedConditions: conditions,
			AccountRef: accountRef, ProductAnchorRef: anchorRef, LandingPageRef: landingRef,
			ScheduledAt: scheduledAt, ValidatedAt: r.currentTime(), OutputFile: outputFile,
		})
		if err != nil {
			return err
		}
		return r.writeOK("local.douyin-commerce.validate", localExecutionResult(map[string]any{"validation": result, "authority": "validated_local_delivery"}))
	}}
	validate.Flags().StringVar(&directory, "directory", "", "工作区路径；默认使用当前目录")
	validate.Flags().StringVar(&audienceSnapshotID, "audience-snapshot", "", "AudienceStrategyVersion 所在的批准快照 ID")
	validate.Flags().StringVar(&audienceID, "audience", "", "已批准的 AudienceStrategyVersion ID")
	validate.Flags().StringVar(&offerSnapshotID, "offer-snapshot", "", "CommerceOfferSnapshot 所在的批准快照 ID")
	validate.Flags().StringVar(&offerID, "offer", "", "已批准的 CommerceOfferSnapshot ID")
	validate.Flags().StringVar(&contentSnapshotID, "content-snapshot", "", "ContentItem 所在的批准快照 ID")
	validate.Flags().StringVar(&contentID, "content-item", "", "已批准的 ContentItem ID")
	validate.Flags().StringVar(&storyboardSnapshotID, "storyboard-snapshot", "", "StoryboardPackage 所在的批准快照 ID")
	validate.Flags().StringVar(&storyboardID, "storyboard", "", "已批准的 StoryboardPackage ID")
	validate.Flags().StringVar(&artifactID, "artifact-id", "", "DeliveryPackage 中最终成片 Artifact ID")
	validate.Flags().StringVar(&artifactFile, "artifact-file", "", "工作区内最终成片文件")
	validate.Flags().StringVar(&landingTextFile, "landing-page-text", "", "工作区内用于发布的落地页纯文本文件")
	validate.Flags().StringSliceVar(&benefits, "benefit", nil, "正文中出现且由 Offer 允许的权益；可重复")
	validate.Flags().StringSliceVar(&conditions, "condition", nil, "正文中出现且由 Offer 允许的条件；可重复")
	validate.Flags().StringVar(&accountRef, "account-ref", "", "固定的抖音账号引用")
	validate.Flags().StringVar(&anchorRef, "product-anchor-ref", "", "固定的商品锚点引用")
	validate.Flags().StringVar(&landingRef, "landing-page-ref", "", "固定的落地页引用")
	validate.Flags().TimeVar(&scheduledAt, "scheduled-at", time.Time{}, []string{time.RFC3339}, "计划发布时间（RFC3339）")
	validate.Flags().StringVar(&outputFile, "out", "", "校验回执输出路径")
	_ = validate.MarkFlagRequired("audience-snapshot")
	_ = validate.MarkFlagRequired("audience")
	_ = validate.MarkFlagRequired("offer-snapshot")
	_ = validate.MarkFlagRequired("offer")
	_ = validate.MarkFlagRequired("content-snapshot")
	_ = validate.MarkFlagRequired("content-item")
	_ = validate.MarkFlagRequired("storyboard-snapshot")
	_ = validate.MarkFlagRequired("storyboard")
	_ = validate.MarkFlagRequired("artifact-id")
	_ = validate.MarkFlagRequired("artifact-file")
	_ = validate.MarkFlagRequired("landing-page-text")
	_ = validate.MarkFlagRequired("account-ref")
	_ = validate.MarkFlagRequired("product-anchor-ref")
	_ = validate.MarkFlagRequired("landing-page-ref")
	_ = validate.MarkFlagRequired("scheduled-at")

	var lintDirectory, lintArtifactFile, lintLandingTextFile string
	lint := &cobra.Command{Use: "lint <validation-receipt.json>", Args: cobra.ExactArgs(1), Short: "复算校验回执并检测批准输入、成片和落地页漂移", RunE: func(cmd *cobra.Command, args []string) error {
		report, receipt, err := localworkspace.LintDouyinCommerceReceipt(lintDirectory, args[0], lintArtifactFile, lintLandingTextFile)
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("DOUYIN_COMMERCE_RECEIPT_LINT_FAILED", "抖音电商校验回执复算失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.douyin-commerce.lint", localExecutionResult(map[string]any{"receipt": receipt, "report": report, "authority": "validated_local_delivery"}))
	}}
	lint.Flags().StringVar(&lintDirectory, "directory", "", "工作区路径；默认使用当前目录")
	lint.Flags().StringVar(&lintArtifactFile, "artifact-file", "", "工作区内最终成片文件")
	lint.Flags().StringVar(&lintLandingTextFile, "landing-page-text", "", "工作区内用于发布的落地页纯文本文件")
	_ = lint.MarkFlagRequired("artifact-file")
	_ = lint.MarkFlagRequired("landing-page-text")

	cmd.AddCommand(validate, lint)
	return cmd
}

func localExecutionResult(data map[string]any) map[string]any {
	data["execution_plane"] = localWorkspaceExecutionPlane
	if _, exists := data["authority"]; !exists {
		data["authority"] = "candidate_only"
	}
	return data
}

func valuesOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
