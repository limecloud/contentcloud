package contracts

import (
	"encoding/json"
	"testing"
)

func TestEmbeddedSchemasAreValidJSON(t *testing.T) {
	for name, body := range map[string][]byte{
		"knowledge-candidates-1.0": KnowledgeCandidatesSchema,
		"brief-2.0":                BriefV2Schema,
		"creative-directions-2.0":  CreativeDirectionsV2Schema,
		"script-package-1.1":       ScriptPackageSchema,
		"script-package-2.0":       ScriptPackageV2Schema,
		"task-contract-1.0":        TaskContractSchema,
	} {
		var schema map[string]any
		if len(body) == 0 || json.Unmarshal(body, &schema) != nil || schema["$id"] == "" {
			t.Fatalf("embedded schema %s is missing or invalid", name)
		}
	}
}
