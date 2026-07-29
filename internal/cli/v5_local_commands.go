package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

const codexLocalExecutionPlane = "codex_local"

func (r *Root) localAudienceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "audience", Short: "Create and validate local Douyin audience strategy candidates"}

	taxonomy := &cobra.Command{Use: "taxonomy", Short: "Validate a server-governed audience taxonomy pulled into this workspace"}
	var taxonomyDirectory string
	taxonomyLint := &cobra.Command{Use: "lint <taxonomy.json>", Args: cobra.ExactArgs(1), Short: "Validate a pulled and human-verified audience taxonomy", RunE: func(cmd *cobra.Command, args []string) error {
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
	taxonomyLint.Flags().StringVar(&taxonomyDirectory, "directory", "", "workspace path; defaults to current directory")
	taxonomy.AddCommand(taxonomyLint)

	strategy := &cobra.Command{Use: "strategy", Short: "Scaffold and validate local audience strategy candidates"}
	var scaffoldDirectory, taxonomyID, mode, objective, testType, primaryVariable string
	var audiences []string
	scaffold := &cobra.Command{Use: "scaffold", Args: cobra.NoArgs, Short: "Create candidate strategies from a pulled ApprovedSnapshot taxonomy", RunE: func(cmd *cobra.Command, args []string) error {
		paths, values, err := localworkspace.ScaffoldAudienceStrategies(localworkspace.ScaffoldAudienceStrategiesOptions{
			Root: scaffoldDirectory, TaxonomySnapshotID: taxonomyID, Mode: mode, AudienceCodes: audiences,
			Objective: objective, TestType: testType, PrimaryVariable: primaryVariable,
		})
		if err != nil {
			return err
		}
		return r.writeOK("local.audience.strategy.scaffold", localExecutionResult(map[string]any{
			"paths": valuesOrEmpty(paths), "strategies": values, "next_action": "补齐证据与策略字段，lint 后再显式 publish strategy",
		}))
	}}
	scaffold.Flags().StringVar(&scaffoldDirectory, "directory", "", "workspace path; defaults to current directory")
	scaffold.Flags().StringVar(&taxonomyID, "taxonomy", "", "object ID from a pulled strategy ApprovedSnapshot")
	scaffold.Flags().StringVar(&mode, "mode", "single", "single, compare, or explore")
	scaffold.Flags().StringSliceVar(&audiences, "audience", nil, "audience code; one for single, two or three for compare")
	scaffold.Flags().StringVar(&objective, "objective", "", "commerce objective")
	scaffold.Flags().StringVar(&testType, "test-type", "", "strict_ab, exploration_batch, or audience_expression_fit_test")
	scaffold.Flags().StringVar(&primaryVariable, "primary-variable", "", "experiment primary variable")

	var strategyDirectory string
	strategyLint := &cobra.Command{Use: "lint <strategy.json>", Args: cobra.ExactArgs(1), Short: "Validate a review-ready audience strategy candidate", RunE: func(cmd *cobra.Command, args []string) error {
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
	strategyLint.Flags().StringVar(&strategyDirectory, "directory", "", "workspace path; defaults to current directory")
	strategy.AddCommand(scaffold, strategyLint)

	cmd.AddCommand(taxonomy, strategy)
	return cmd
}

func (r *Root) localOfferCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "offer", Short: "Validate local CommerceOfferSnapshot files against their active window"}
	var directory string
	var at string
	lint := &cobra.Command{Use: "lint <offer.json>", Args: cobra.ExactArgs(1), Short: "Validate a verified offer before render or publish", RunE: func(cmd *cobra.Command, args []string) error {
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
			lintErr := domain.Invalid("COMMERCE_OFFER_LINT_FAILED", "Offer 确定性校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.offer.lint", localExecutionResult(map[string]any{"offer": value, "report": report, "checked_at": checkAt.UTC()}))
	}}
	lint.Flags().StringVar(&directory, "directory", "", "workspace path; defaults to current directory")
	lint.Flags().StringVar(&at, "at", "", "RFC3339 validation time; defaults to now")
	cmd.AddCommand(lint)
	return cmd
}

func (r *Root) localStoryboardCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "storyboard", Short: "Build and validate local storyboard candidates from approved content"}

	var createDirectory, snapshotID, contentItemID, packageID, capabilityID, capabilityVersion, capabilityDigest string
	create := &cobra.Command{Use: "create", Args: cobra.NoArgs, Short: "Create a candidate storyboard from a pulled content ApprovedSnapshot", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.CreateStoryboardPackage(localworkspace.CreateStoryboardPackageOptions{
			Root: createDirectory, ApprovedSnapshotID: snapshotID, ContentItemID: contentItemID, PackageID: packageID,
			Capability: domain.CapabilityRef{ID: capabilityID, Version: capabilityVersion, Digest: capabilityDigest},
		})
		if err != nil {
			return err
		}
		return r.writeOK("local.storyboard.create", localExecutionResult(map[string]any{
			"storyboard": result, "next_action": "逐镜头生成 first-frame，可选 end-frame，并生成 review-sheet 后执行 local storyboard prepare",
		}))
	}}
	create.Flags().StringVar(&createDirectory, "directory", "", "workspace path; defaults to current directory")
	create.Flags().StringVar(&snapshotID, "snapshot", "", "pulled content_batch ApprovedSnapshot ID")
	create.Flags().StringVar(&contentItemID, "content-item", "", "approved ContentItem ID")
	create.Flags().StringVar(&packageID, "id", "", "optional storyboard package ID")
	create.Flags().StringVar(&capabilityID, "capability-id", "", "local image generation capability ID")
	create.Flags().StringVar(&capabilityVersion, "capability-version", "", "local image generation capability version")
	create.Flags().StringVar(&capabilityDigest, "capability-digest", "", "local capability digest with sha256: prefix")

	var prepareDirectory string
	prepare := &cobra.Command{Use: "prepare <manifest.json>", Args: cobra.ExactArgs(1), Short: "Discover generated media and prepare the candidate for server review", RunE: func(cmd *cobra.Command, args []string) error {
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
			"storyboard": value, "report": report, "next_action": "执行 publish storyboard；只有服务端批准后 pull 的 storyboard ApprovedSnapshot 才代表 locked",
		}))
	}}
	prepare.Flags().StringVar(&prepareDirectory, "directory", "", "workspace path; defaults to current directory")

	var lintDirectory string
	lint := &cobra.Command{Use: "lint <manifest.json>", Args: cobra.ExactArgs(1), Short: "Check storyboard media, rights metadata, and locked digest before publish", RunE: func(cmd *cobra.Command, args []string) error {
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
	lint.Flags().StringVar(&lintDirectory, "directory", "", "workspace path; defaults to current directory")

	cmd.AddCommand(create, prepare, lint)
	return cmd
}

func (r *Root) localSeedanceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "seedance", Short: "Export copy-ready Seedance packages from a pulled locked storyboard snapshot"}
	var directory, snapshotID, storyboardID, packageID, profileVersion, adapterID, adapterVersion, adapterDigest, mode, aspectRatio, sound string
	var minDuration, maxDuration, maxImages, maxVideos, maxAudios int
	export := &cobra.Command{Use: "export", Args: cobra.NoArgs, Short: "Compile prompts, upload mapping, media, and an operator README locally", RunE: func(cmd *cobra.Command, args []string) error {
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
			"delivery": result, "authority": "validated_local_delivery", "next_action": "用户按 README 在 Seedance 手工上传、核对编号并逐段复制提示词",
		}))
	}}
	export.Flags().StringVar(&directory, "directory", "", "workspace path; defaults to current directory")
	export.Flags().StringVar(&snapshotID, "snapshot", "", "pulled storyboard ApprovedSnapshot ID")
	export.Flags().StringVar(&storyboardID, "storyboard", "", "eligible StoryboardPackage object ID")
	export.Flags().StringVar(&packageID, "id", "", "optional immutable delivery package ID")
	export.Flags().StringVar(&profileVersion, "profile-version", "", "human-verified Seedance provider profile version")
	export.Flags().StringVar(&adapterID, "adapter-id", "contentcloud.seedance-export", "adapter capability ID")
	export.Flags().StringVar(&adapterVersion, "adapter-version", "1.0.0", "adapter capability version")
	export.Flags().StringVar(&adapterDigest, "adapter-digest", "", "adapter capability digest with sha256: prefix")
	export.Flags().StringVar(&mode, "mode", "all_reference", "first_last_frame, all_reference, or extend")
	export.Flags().StringVar(&aspectRatio, "aspect-ratio", "9:16", "provider aspect ratio")
	export.Flags().StringVar(&sound, "sound", "environment_only", "provider sound setting")
	export.Flags().IntVar(&minDuration, "min-duration", 0, "verified minimum generated seconds per segment; required")
	export.Flags().IntVar(&maxDuration, "max-duration", 0, "verified maximum generated seconds per segment; required")
	export.Flags().IntVar(&maxImages, "max-images", 0, "verified maximum image references; required")
	export.Flags().IntVar(&maxVideos, "max-videos", 0, "verified maximum video references")
	export.Flags().IntVar(&maxAudios, "max-audios", 0, "verified maximum audio references")

	var lintDirectory string
	lint := &cobra.Command{Use: "lint <package.json>", Args: cobra.ExactArgs(1), Short: "Revalidate a local Seedance package and its locked inputs", RunE: func(cmd *cobra.Command, args []string) error {
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
	lint.Flags().StringVar(&lintDirectory, "directory", "", "workspace path; defaults to current directory")
	cmd.AddCommand(export, lint)
	return cmd
}

func localExecutionResult(data map[string]any) map[string]any {
	data["execution_plane"] = codexLocalExecutionPlane
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
