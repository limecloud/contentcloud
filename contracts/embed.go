package contracts

import _ "embed"

//go:embed project-pages-1.0.json
var ProjectPagesV1Contract []byte

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
