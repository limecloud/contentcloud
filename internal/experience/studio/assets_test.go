package studio

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCreativeResultItemJSONDoesNotExposeInternalInputRef(t *testing.T) {
	encoded, err := json.Marshal(CreativeResultItem{
		Ref:       "result:result-1",
		InputRef:  "approved_snapshot:snapshot-1@sha256:private",
		Downloads: []Download{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "input_ref") || strings.Contains(string(encoded), "private") {
		t.Fatalf("customer projection leaked internal input reference: %s", encoded)
	}
}

func TestAssetSurfaceKeepsMaterialsAndResultsAsSeparateProjections(t *testing.T) {
	encoded, err := json.Marshal(AssetSurface{
		Workspace:       WorkspaceMaterialProjection{Materials: []WorkspaceMaterialItem{}},
		CreativeResults: CreativeResultProjection{Items: []CreativeResultItem{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	if _, ok := value["workspace"]; !ok {
		t.Fatal("asset surface must expose the workspace material projection")
	}
	if _, ok := value["creative_results"]; !ok {
		t.Fatal("asset surface must expose the creative result projection")
	}
}
