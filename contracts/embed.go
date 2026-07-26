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
