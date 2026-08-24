package localworkspace

import (
	"encoding/json"
	"testing"
)

func TestVideoScriptCompatibilityPayloadRendersDeterministicDelivery(t *testing.T) {
	raw, _, err := NormalizeVideoScriptForSubmission(json.RawMessage(`{"title":"测试脚本","scenes":[{"scene":1,"voiceover":"开场"}]}`), "project-1", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	first, err := RenderContentItem(raw)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderContentItem(raw)
	if err != nil {
		t.Fatal(err)
	}
	if first.ItemID != "video-script:task-1" || first.SchemaID != VideoScriptSchema || first.ContentHash != second.ContentHash || len(first.Files) != 3 {
		t.Fatalf("video script delivery is not stable or complete: first=%#v second=%#v", first, second)
	}
	for _, file := range first.Files {
		if len(file.Body) == 0 || file.SHA256 == "" {
			t.Fatalf("video script delivery contains an empty file: %#v", file)
		}
	}
}
