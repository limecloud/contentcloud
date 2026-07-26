package contracts

import _ "embed"

//go:embed script-package-1.1.schema.json
var ScriptPackageSchema []byte

//go:embed task-contract-1.0.schema.json
var TaskContractSchema []byte

//go:embed knowledge-candidates-1.0.schema.json
var KnowledgeCandidatesSchema []byte
