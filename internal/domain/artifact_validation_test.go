package domain

import "testing"

func TestValidateExtensionArtifactEnvelopeRejectsActiveRenditionAndNestedMetadata(t *testing.T) {
	base := ExtensionArtifactEnvelopeV1{
		EnvelopeVersion: "1.0", ProjectID: NewID(), ScriptVersionID: NewID(),
		Capability: ArtifactCapabilityRef{ID: "com.example.renderer", Version: "1.2.3", Digest: "sha256:renderer"},
		SchemaID:   "example/project/1.0", MediaType: "application/octet-stream",
		SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 42,
		Renditions: []ArtifactRenditionRef{}, Metadata: map[string]any{"variant": "A"},
	}
	if err := ValidateExtensionArtifactEnvelope(base); err != nil {
		t.Fatal(err)
	}
	active := base
	active.Renditions = []ArtifactRenditionRef{{Purpose: "preview", Artifact: ArtifactRef{SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", MediaType: "text/html", Size: 20}}}
	if err := ValidateExtensionArtifactEnvelope(active); err == nil {
		t.Fatal("ordinary HTML must not be accepted as a safe rendition")
	}
	nested := base
	nested.Metadata = map[string]any{"private": map[string]any{"path": "/tmp/private"}}
	if err := ValidateExtensionArtifactEnvelope(nested); err == nil {
		t.Fatal("metadata must remain scalar")
	}
}

func TestValidateArtifactReviewProjectionRequiresStablePointers(t *testing.T) {
	scriptID := NewID()
	value := ArtifactReviewProjectionV1{SchemaVersion: "1.0", Title: "Review", Summary: "Summary", ScriptVersionID: scriptID, Sections: []ReviewProjectionSectionV1{{ID: "hook", Label: "Hook", Summary: "Opening", ScriptPointer: "/shots/0", Warnings: []string{}}}}
	if err := ValidateArtifactReviewProjection(value, scriptID); err != nil {
		t.Fatal(err)
	}
	value.Sections[0].ScriptPointer = "shots/0"
	if err := ValidateArtifactReviewProjection(value, scriptID); err == nil {
		t.Fatal("invalid JSON Pointer must be rejected")
	}
}
