package composition

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	mediapipeline "github.com/limecloud/contentcloud/internal/integration/provider/media"
)

func TestDeterministicWorkerProducesIndependentValidatedMP4(t *testing.T) {
	source, err := os.ReadFile("../provider/media/testdata/fake-provider-video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	manifest := workerManifest(t, source)
	manifestDigest, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	worker := DeterministicWorker{}
	result, err := worker.Compose(context.Background(), Request{
		Manifest: manifest, ManifestDigest: manifestDigest,
		SourceArtifact: InputArtifact{ID: "video-1", Digest: mediapipeline.SHA256(source), MediaType: "video/mp4", FileName: "take.mp4", Body: source},
		Auxiliary:      []InputArtifact{{ID: "captions-1", Digest: mediapipeline.SHA256([]byte("captions")), MediaType: "text/vtt", FileName: "captions.vtt", Body: []byte("captions")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.MediaType != "video/mp4" || result.FileName == "" || bytes.Equal(result.Body, source) {
		t.Fatalf("worker did not produce an independent output: media=%q file=%q equal=%v", result.MediaType, result.FileName, bytes.Equal(result.Body, source))
	}
	if _, err := mediapipeline.ValidateDownload(mediapipeline.Download{Body: result.Body, MediaType: result.MediaType, FileName: result.FileName}, 100<<20); err != nil {
		t.Fatalf("worker output is not a valid MP4: %v", err)
	}
	if !bytes.Contains(result.Body, []byte(manifestDigest)) || !bytes.Contains(result.Body, []byte("captions-1")) {
		t.Fatal("worker output did not retain deterministic input lineage")
	}
	repeated, err := worker.Compose(context.Background(), Request{
		Manifest: manifest, ManifestDigest: manifestDigest,
		SourceArtifact: InputArtifact{ID: "video-1", Digest: mediapipeline.SHA256(source), MediaType: "video/mp4", FileName: "take.mp4", Body: source},
		Auxiliary:      []InputArtifact{{ID: "captions-1", Digest: mediapipeline.SHA256([]byte("captions")), MediaType: "text/vtt", FileName: "captions.vtt", Body: []byte("captions")}},
	})
	if err != nil || !bytes.Equal(repeated.Body, result.Body) {
		t.Fatalf("same manifest and inputs were not deterministic: err=%v", err)
	}
}

func TestDeterministicWorkerRejectsInputDigestDrift(t *testing.T) {
	source, err := os.ReadFile("../provider/media/testdata/fake-provider-video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	manifest := workerManifest(t, source)
	digest, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	_, err = (DeterministicWorker{}).Compose(context.Background(), Request{
		Manifest: manifest, ManifestDigest: digest,
		SourceArtifact: InputArtifact{ID: "video-1", Digest: "sha256:" + strings.Repeat("f", 64), MediaType: "video/mp4", FileName: "take.mp4", Body: source},
	})
	if err == nil || !strings.Contains(err.Error(), "摘要") {
		t.Fatalf("expected input digest rejection, got %v", err)
	}
}

func workerManifest(t *testing.T, source []byte) deliverydomain.CompositionManifest {
	t.Helper()
	videoDigest := mediapipeline.SHA256(source)
	return deliverydomain.CompositionManifest{
		Schema:           deliverydomain.CompositionSchemaRef{Name: deliverydomain.CompositionManifestSchema, Version: "1.0"},
		ApprovedSnapshot: deliverydomain.CompositionSnapshotRef{ID: "snapshot-1", Digest: "sha256:" + strings.Repeat("b", 64)},
		SelectedVideo:    deliverydomain.CompositionVideoRef{ArtifactID: "video-1", Digest: "sha256:" + videoDigest, MediaReviewID: "review-1", ReviewDigest: "sha256:" + strings.Repeat("c", 64)},
		Narration:        deliverydomain.CompositionNarration{Status: "disabled"},
		Subtitles:        deliverydomain.CompositionSubtitles{Status: "disabled", SourceKind: "none"},
		BrandCTA:         deliverydomain.CompositionBrandCTA{BrandConfigDigest: "sha256:" + strings.Repeat("d", 64), CTADigest: "sha256:" + strings.Repeat("e", 64)},
		Timeline:         deliverydomain.CompositionTimeline{CanvasWidth: 1080, CanvasHeight: 1920, FrameRateNumerator: 30, FrameRateDenominator: 1, DurationMS: 1000, TransitionVersion: "deterministic/1", Segments: []deliverydomain.CompositionSegment{{ID: "segment-1", StartMS: 0, EndMS: 1000, ArtifactID: "video-1", ArtifactDigest: "sha256:" + videoDigest}}},
		Renderer:         deliverydomain.CompositionRenderer{Name: RendererName, Version: RendererVersion, CapabilityDigest: "sha256:" + strings.Repeat("f", 64)},
		Output:           deliverydomain.CompositionOutput{Kind: "final_render", MediaType: "video/mp4", Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"},
	}
}
