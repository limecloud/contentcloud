package contracts

import _ "embed"

//go:embed script-package-1.1.schema.json
var ScriptPackageSchema []byte

//go:embed script-package-2.0.schema.json
var ScriptPackageV2Schema []byte

//go:embed brief-2.0.schema.json
var BriefV2Schema []byte

//go:embed creative-directions-2.0.schema.json
var CreativeDirectionsV2Schema []byte

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
