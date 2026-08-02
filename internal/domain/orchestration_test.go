package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSOPVersionNormalizeCollectionsProducesJSONArrays(t *testing.T) {
	version := SOPVersion{
		SOPID:   "sop-1",
		Version: 1,
		Name:    "流程",
		Stages:  []StageDefinition{{ID: "input", Name: "输入", OutputSchema: "contentcloud.brief/1.0"}},
		Gates:   []GateDefinition{{ID: "review", Name: "检查", Mode: GateModeAdvisory}},
	}
	version.NormalizeCollections()
	body, err := json.Marshal(version)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(body)
	for _, field := range []string{"content_types", "owner_roles", "input_refs", "required_capabilities", "execution_modes", "checks", "gate_ids", "assignee_roles"} {
		if strings.Contains(serialized, `"`+field+`":null`) {
			t.Fatalf("normalized SOP field %s must not be null: %s", field, serialized)
		}
	}
	if !strings.Contains(serialized, `"content_types":[]`) || !strings.Contains(serialized, `"gate_ids":[]`) {
		t.Fatalf("normalized SOP fields must be JSON arrays: %s", serialized)
	}
}
