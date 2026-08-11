package contracts

import _ "embed"

//go:embed studio-surfaces-1.0.json
var StudioSurfacesV1Contract []byte

//go:embed workspace-3.0.schema.json
var WorkspaceV3Schema []byte

//go:embed source-registry-3.0.schema.json
var SourceRegistryV3Schema []byte

//go:embed knowledge-page-3.0.schema.json
var KnowledgePageV3Schema []byte

//go:embed knowledge-pack-3.0.schema.json
var KnowledgePackV3Schema []byte

//go:embed local-run-3.0.schema.json
var LocalRunV3Schema []byte

//go:embed handoff-1.0.schema.json
var HandoffV1Schema []byte

//go:embed content-batch-3.0.schema.json
var ContentBatchV3Schema []byte

//go:embed content-item-3.0.schema.json
var ContentItemV3Schema []byte

//go:embed article-brief-1.0.schema.json
var ArticleBriefV1Schema []byte

//go:embed article-1.0.schema.json
var ArticleV1Schema []byte

//go:embed wechat-delivery-1.0.schema.json
var WeChatDeliveryV1Schema []byte

//go:embed brief-3.0.schema.json
var BriefV3Schema []byte

//go:embed creative-directions-3.0.schema.json
var CreativeDirectionsV3Schema []byte

//go:embed submission-bundle-3.0.schema.json
var SubmissionBundleV3Schema []byte

//go:embed task-contract-1.0.schema.json
var TaskContractSchema []byte

//go:embed knowledge-candidates-1.0.schema.json
var KnowledgeCandidatesSchema []byte

//go:embed creative-environment-manifest-1.0.schema.json
var CreativeEnvironmentManifestSchema []byte

//go:embed creative-environment-profile-1.0.schema.json
var CreativeEnvironmentProfileSchema []byte

//go:embed environment-lock-1.0.schema.json
var EnvironmentLockSchema []byte

//go:embed local-execution-plan-1.0.schema.json
var LocalExecutionPlanSchema []byte

//go:embed creative-execution-bundle-1.0.schema.json
var CreativeExecutionBundleSchema []byte

//go:embed environment-trusted-keys-1.0.schema.json
var EnvironmentTrustedKeysSchema []byte

//go:embed environment-preparation-plan-1.0.schema.json
var EnvironmentPreparationPlanSchema []byte

//go:embed audience-taxonomy-1.0.schema.json
var AudienceTaxonomyV1Schema []byte

//go:embed audience-strategy-1.0.schema.json
var AudienceStrategyV1Schema []byte

//go:embed commerce-offer-1.0.schema.json
var CommerceOfferV1Schema []byte

//go:embed douyin-commerce-validation-1.0.schema.json
var DouyinCommerceValidationV1Schema []byte

//go:embed storyboard-package-1.0.schema.json
var StoryboardPackageV1Schema []byte

//go:embed seedance-prompt-package-1.0.schema.json
var SeedancePromptPackageV1Schema []byte

//go:embed published-creative-binding-1.0.schema.json
var PublishedCreativeBindingV1Schema []byte

//go:embed source-intake-1.0.schema.json
var SourceIntakeV1Schema []byte

//go:embed channel-publication-1.0.schema.json
var ChannelPublicationV1Schema []byte

//go:embed channel-callback-1.0.schema.json
var ChannelCallbackV1Schema []byte

//go:embed model-generation-1.0.schema.json
var ModelGenerationV1Schema []byte

//go:embed connector-sync-1.0.schema.json
var ConnectorSyncV1Schema []byte

//go:embed agent-execution-1.0.schema.json
var AgentExecutionV1Schema []byte

//go:embed novel-canon-1.0.schema.json
var NovelCanonV1Schema []byte

//go:embed novel-outline-1.0.schema.json
var NovelOutlineV1Schema []byte

//go:embed novel-chapter-1.0.schema.json
var NovelChapterV1Schema []byte

//go:embed novel-release-1.0.schema.json
var NovelReleaseV1Schema []byte

//go:embed openapi.yaml
var OpenAPIYAML []byte
