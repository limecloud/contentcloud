package delivery

import (
	"strings"
	"testing"
)

func testCompositionManifest() CompositionManifest {
	digest := "sha256:" + strings.Repeat("a", 64)
	return CompositionManifest{
		Schema:           CompositionSchemaRef{Name: CompositionManifestSchema, Version: "1.0"},
		ApprovedSnapshot: CompositionSnapshotRef{ID: "snapshot-1", Digest: digest},
		SelectedVideo:    CompositionVideoRef{ArtifactID: "artifact-video", Digest: digest, MediaReviewID: "review-content", ReviewDigest: digest},
		Narration:        CompositionNarration{Status: "disabled"},
		Subtitles:        CompositionSubtitles{Status: "disabled", SourceKind: "none"},
		BrandCTA:         CompositionBrandCTA{BrandConfigDigest: digest, CTADigest: digest},
		Timeline: CompositionTimeline{
			CanvasWidth: 1080, CanvasHeight: 1920, FrameRateNumerator: 30, FrameRateDenominator: 1, DurationMS: 4000,
			TransitionVersion: "transitions/1.0", Segments: []CompositionSegment{{ID: "segment-1", StartMS: 0, EndMS: 4000, ArtifactID: "artifact-video", ArtifactDigest: digest}},
		},
		Renderer: CompositionRenderer{Name: "contentcloud.compositor", Version: "1.0.0", CapabilityDigest: digest},
		Output:   CompositionOutput{Kind: "final_render", MediaType: "video/mp4", Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"},
	}
}

func TestCompositionManifestDigestIsStable(t *testing.T) {
	first := testCompositionManifest()
	second := testCompositionManifest()
	firstDigest, err := first.Digest()
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := second.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest || !strings.HasPrefix(firstDigest, "sha256:") {
		t.Fatalf("expected stable canonical digest, got %q and %q", firstDigest, secondDigest)
	}
}

func TestCompositionManifestRejectsImplicitNarrationAndSubtitleInputs(t *testing.T) {
	manifest := testCompositionManifest()
	manifest.Narration = CompositionNarration{}
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected narration status validation error")
	}
	manifest = testCompositionManifest()
	manifest.Subtitles = CompositionSubtitles{Status: "disabled", SourceKind: "inline"}
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected disabled subtitle input validation error")
	}
}

func TestCompositionManifestDigestChangesWithTimelineInput(t *testing.T) {
	manifest := testCompositionManifest()
	first, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	manifest.Timeline.Segments[0].EndMS = 3500
	second, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("expected timeline change to produce a new digest")
	}
}
