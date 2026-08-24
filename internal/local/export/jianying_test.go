package exportfmt

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
)

type jianyingFixtures struct {
	snapshot  reviewdomain.ApprovedSnapshot
	selected  deliverydomain.MediaReview
	final     deliverydomain.MediaReview
	pkg       deliverydomain.DeliveryPackage
	artifacts map[string]deliverydomain.Artifact
	blobs     *blob.MemoryStore
}

type snapshotReader struct{ value reviewdomain.ApprovedSnapshot }

func (r snapshotReader) ApprovedSnapshot(_ context.Context, tenantID, id string) (reviewdomain.ApprovedSnapshot, error) {
	if tenantID != r.value.TenantID || id != r.value.ID {
		return reviewdomain.ApprovedSnapshot{}, faultNotFound()
	}
	return r.value, nil
}

type reviewReader struct {
	values map[string]deliverydomain.MediaReview
}

func (r reviewReader) MediaReview(_ context.Context, tenantID, id string) (deliverydomain.MediaReview, error) {
	v, ok := r.values[id]
	if !ok || v.TenantID != tenantID {
		return deliverydomain.MediaReview{}, faultNotFound()
	}
	return v, nil
}

type packageReader struct {
	value deliverydomain.DeliveryPackage
}

func (r packageReader) DeliveryPackage(_ context.Context, tenantID, id string) (deliverydomain.DeliveryPackage, error) {
	if tenantID != r.value.TenantID || id != r.value.ID {
		return deliverydomain.DeliveryPackage{}, faultNotFound()
	}
	return r.value, nil
}

type artifactReader struct {
	values map[string]deliverydomain.Artifact
}

func (r artifactReader) Artifact(_ context.Context, tenantID, id string) (deliverydomain.Artifact, error) {
	v, ok := r.values[id]
	if !ok || v.TenantID != tenantID {
		return deliverydomain.Artifact{}, faultNotFound()
	}
	return v, nil
}

func faultNotFound() error { return blob.ErrNotFound }

func TestExportJianyingIsByteDeterministic(t *testing.T) {
	f := newJianyingFixtures(t)
	readers := JianyingReaders{Snapshots: snapshotReader{f.snapshot}, Reviews: reviewReader{map[string]deliverydomain.MediaReview{f.selected.ID: f.selected, f.final.ID: f.final}}, Packages: packageReader{f.pkg}, Artifacts: artifactReader{f.artifacts}, Blobs: f.blobs}
	in := JianyingExportInput{TenantID: f.snapshot.TenantID, ProjectID: f.snapshot.ProjectID, ApprovedSnapshotID: f.snapshot.ID, FinalReviewID: f.final.ID, DeliveryPackageID: f.pkg.ID, Manifest: testManifest(f)}
	a, err := ExportJianying(context.Background(), readers, in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ExportJianying(context.Background(), readers, in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Body, b.Body) || a.ArchiveDigest != b.ArchiveDigest {
		t.Fatal("同一输入未生成相同 ZIP 字节")
	}
	z, err := zip.NewReader(bytes.NewReader(a.Body), int64(len(a.Body)))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"draft_info.json", "manifest.json", "assets/final.mp4"}
	if len(z.File) != len(want) {
		t.Fatalf("ZIP 条目数量=%d, want=%d", len(z.File), len(want))
	}
	for i, name := range want {
		if z.File[i].Name != name {
			t.Fatalf("ZIP 条目 %d=%q, want %q", i, z.File[i].Name, name)
		}
	}
	if err := LintJianyingArchive(a.Body, JianyingLintInput{ManifestDigest: a.ManifestDigest, Artifacts: f.pkg.Manifest}); err != nil {
		t.Fatalf("导出 lint 失败: %v", err)
	}
}

func TestExportJianyingRejectsLineageAndBlobFailures(t *testing.T) {
	f := newJianyingFixtures(t)
	base := JianyingExportInput{TenantID: f.snapshot.TenantID, ProjectID: f.snapshot.ProjectID, ApprovedSnapshotID: f.snapshot.ID, FinalReviewID: f.final.ID, DeliveryPackageID: f.pkg.ID, Manifest: testManifest(f)}
	cases := []struct {
		name   string
		mutate func(*JianyingExportInput, *jianyingFixtures)
	}{
		{"snapshot drift", func(in *JianyingExportInput, _ *jianyingFixtures) {
			in.Manifest.ApprovedSnapshot.Digest = "sha256:" + strings.Repeat("f", 64)
		}},
		{"final review pending", func(in *JianyingExportInput, f *jianyingFixtures) { f.final.Status = deliverydomain.MediaReviewPending }},
		{"final artifact absent", func(in *JianyingExportInput, f *jianyingFixtures) { f.pkg.Manifest = nil }},
		{"unsafe filename", func(in *JianyingExportInput, f *jianyingFixtures) {
			a := f.pkg.Manifest[0]
			a.FileName = "../escape.mp4"
			f.pkg.Manifest[0] = a
		}},
		{"missing blob", func(in *JianyingExportInput, f *jianyingFixtures) { f.blobs = blob.NewMemory() }},
		{"tenant mismatch", func(in *JianyingExportInput, _ *jianyingFixtures) { in.TenantID = "tenant-other" }},
		{"duplicate target filename", func(in *JianyingExportInput, f *jianyingFixtures) {
			duplicate := f.pkg.Manifest[0]
			duplicate.ID = "final-2"
			f.pkg.Manifest = append(f.pkg.Manifest, duplicate)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copy := f
			copy.final = f.final
			copy.pkg = f.pkg
			copy.pkg.Manifest = append([]deliverydomain.Artifact(nil), f.pkg.Manifest...)
			copy.blobs = f.blobs
			in := base
			tc.mutate(&in, &copy)
			localReaders := JianyingReaders{Snapshots: snapshotReader{copy.snapshot}, Reviews: reviewReader{map[string]deliverydomain.MediaReview{copy.selected.ID: copy.selected, copy.final.ID: copy.final}}, Packages: packageReader{copy.pkg}, Artifacts: artifactReader{copy.artifacts}, Blobs: copy.blobs}
			if _, err := ExportJianying(context.Background(), localReaders, in); err == nil {
				t.Fatal("应拒绝非法导出输入")
			}
		})
	}
}

func newJianyingFixtures(t *testing.T) jianyingFixtures {
	t.Helper()
	const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	f := jianyingFixtures{snapshot: reviewdomain.ApprovedSnapshot{ID: "snapshot-1", TenantID: "tenant-1", ProjectID: "project-1", SubjectHash: digest, ContentHash: digest}, artifacts: map[string]deliverydomain.Artifact{}, blobs: blob.NewMemory()}
	sourceBody := []byte("source video")
	finalBody := []byte("final video")
	source := deliverydomain.Artifact{ID: "source-1", TenantID: f.snapshot.TenantID, ProjectID: f.snapshot.ProjectID, ApprovedSnapshotID: f.snapshot.ID, Kind: "generated_video", Purpose: "preview", MediaType: "video/mp4", FileName: "source.mp4", SHA256: digestBytes(sourceBody), ByteSize: int64(len(sourceBody)), ObjectKey: "source-1"}
	final := deliverydomain.Artifact{ID: "final-1", TenantID: f.snapshot.TenantID, ProjectID: f.snapshot.ProjectID, ApprovedSnapshotID: f.snapshot.ID, Kind: "final_render", Purpose: "final_video", MediaType: "video/mp4", FileName: "final.mp4", SHA256: digestBytes(finalBody), ByteSize: int64(len(finalBody)), ObjectKey: "final-1"}
	f.artifacts[source.ID] = source
	f.artifacts[final.ID] = final
	_ = f.blobs.Put(context.Background(), source.ObjectKey, sourceBody)
	_ = f.blobs.Put(context.Background(), final.ObjectKey, finalBody)
	f.selected = deliverydomain.MediaReview{ID: "review-content", TenantID: f.snapshot.TenantID, ProjectID: f.snapshot.ProjectID, TaskID: "task-1", SubjectArtifactID: source.ID, SubjectDigest: source.SHA256, ReviewKind: deliverydomain.MediaReviewContent, Status: deliverydomain.MediaReviewApproved, Selected: true, Checks: map[string]any{"media.content": true}, RowVersion: 2}
	f.final = deliverydomain.MediaReview{ID: "review-final", TenantID: f.snapshot.TenantID, ProjectID: f.snapshot.ProjectID, TaskID: "task-1", SubjectArtifactID: final.ID, SubjectDigest: final.SHA256, ReviewKind: deliverydomain.MediaReviewFinal, Status: deliverydomain.MediaReviewApproved, Selected: true, Checks: map[string]any{"media.final": true}, RowVersion: 2}
	f.pkg = deliverydomain.DeliveryPackage{ID: "package-1", TenantID: f.snapshot.TenantID, ProjectID: f.snapshot.ProjectID, ApprovedSnapshotIDs: []string{f.snapshot.ID}, Status: "ready", Manifest: []deliverydomain.Artifact{final}}
	return f
}

func testManifest(f jianyingFixtures) deliverydomain.CompositionManifest {
	return deliverydomain.CompositionManifest{Schema: deliverydomain.CompositionSchemaRef{Name: deliverydomain.CompositionManifestSchema, Version: "1.0"}, ApprovedSnapshot: deliverydomain.CompositionSnapshotRef{ID: f.snapshot.ID, Digest: snapshotDigest(f.snapshot)}, SelectedVideo: deliverydomain.CompositionVideoRef{ArtifactID: f.selected.SubjectArtifactID, Digest: f.selected.SubjectDigest, MediaReviewID: f.selected.ID, ReviewDigest: mediaReviewDigest(f.selected)}, Narration: deliverydomain.CompositionNarration{Status: "disabled"}, Subtitles: deliverydomain.CompositionSubtitles{Status: "disabled", SourceKind: "none"}, BrandCTA: deliverydomain.CompositionBrandCTA{BrandConfigDigest: "sha256:" + strings.Repeat("0", 64), CTADigest: "sha256:" + strings.Repeat("1", 64)}, Timeline: deliverydomain.CompositionTimeline{CanvasWidth: 1080, CanvasHeight: 1920, FrameRateNumerator: 30, FrameRateDenominator: 1, DurationMS: 1000, TransitionVersion: "test/1", Segments: []deliverydomain.CompositionSegment{{ID: "source", StartMS: 0, EndMS: 1000, ArtifactID: f.selected.SubjectArtifactID, ArtifactDigest: f.selected.SubjectDigest}}}, Renderer: deliverydomain.CompositionRenderer{Name: "test-renderer", Version: "1", CapabilityDigest: "sha256:" + strings.Repeat("2", 64)}, Output: deliverydomain.CompositionOutput{Kind: "final_render", MediaType: "video/mp4", Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}}
}
