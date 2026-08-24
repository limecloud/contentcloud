package application_test

import (
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
)

func TestExportJianyingUsesGovernedFactsAndRoleScope(t *testing.T) {
	ctx := t.Context()
	service := application.New(application.DependenciesFrom(memory.New()), nil)
	session, err := service.Identity.Register(ctx, "jianying-export@example.com", "long-enough-password", "视频负责人", "视频团队")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := service.Operations.EnsureMarketingVideoDemoFixture(ctx, actor, "jianying-export-fixture")
	if err != nil {
		t.Fatal(err)
	}

	finalArtifact := findJianyingArtifact(t, fixture.Task.Artifacts, "final_render")
	snapshot := findJianyingSnapshot(t, fixture.Task.ApprovedSnapshots, finalArtifact.ApprovedSnapshotID)
	selectedReview := findJianyingReview(t, fixture.Task.MediaReviews, deliverydomain.MediaReviewContent)
	finalReview := findJianyingReview(t, fixture.Task.MediaReviews, deliverydomain.MediaReviewFinal)
	selectedArtifact := findJianyingArtifactByID(t, fixture.Task.Artifacts, selectedReview.SubjectArtifactID)
	pkg := findJianyingPackage(t, fixture.Task.DeliveryPackages, snapshot.ID)
	manifest := deterministicCompositionManifest(t, snapshot, selectedReview, selectedArtifact)
	in := application.JianyingExportInput{ProjectID: fixture.Project.ID, ApprovedSnapshotID: snapshot.ID, FinalReviewID: finalReview.ID, DeliveryPackageID: pkg.ID, Manifest: manifest}
	first, err := service.Delivery.ExportJianying(ctx, actor, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Delivery.ExportJianying(ctx, actor, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Body) == 0 || first.ArchiveDigest == "" || first.ArchiveDigest != second.ArchiveDigest || string(first.Body) != string(second.Body) {
		t.Fatalf("governed export is not deterministic: first=%#v second=%#v", first, second)
	}

	drifted := in
	drifted.Manifest.ApprovedSnapshot.Digest = "sha256:" + strings.Repeat("f", 64)
	if _, err := service.Delivery.ExportJianying(ctx, actor, drifted); err == nil {
		t.Fatal("snapshot digest drift must be rejected by the application boundary")
	}

	viewer := actor
	viewer.Role = "viewer"
	if _, err := service.Delivery.ExportJianying(ctx, viewer, in); err == nil {
		t.Fatal("viewer must not export a governed delivery archive")
	}

	foreignSession, err := service.Identity.Register(ctx, "jianying-foreign@example.com", "long-enough-password", "外部用户", "外部团队")
	if err != nil {
		t.Fatal(err)
	}
	foreign, _, err := service.Identity.SessionActor(ctx, foreignSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Delivery.ExportJianying(ctx, foreign, in); err == nil {
		t.Fatal("foreign tenant must not export another tenant's archive")
	}
}

func findJianyingArtifact(t *testing.T, artifacts []deliverydomain.Artifact, kind string) deliverydomain.Artifact {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact
		}
	}
	t.Fatalf("artifact kind %q is missing", kind)
	return deliverydomain.Artifact{}
}

func findJianyingArtifactByID(t *testing.T, artifacts []deliverydomain.Artifact, id string) deliverydomain.Artifact {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.ID == id {
			return artifact
		}
	}
	t.Fatalf("artifact %q is missing", id)
	return deliverydomain.Artifact{}
}

func findJianyingSnapshot(t *testing.T, snapshots []reviewdomain.ApprovedSnapshot, id string) reviewdomain.ApprovedSnapshot {
	t.Helper()
	for _, snapshot := range snapshots {
		if snapshot.ID == id {
			return snapshot
		}
	}
	t.Fatalf("snapshot %q is missing", id)
	return reviewdomain.ApprovedSnapshot{}
}

func findJianyingReview(t *testing.T, reviews []deliverydomain.MediaReview, kind string) deliverydomain.MediaReview {
	t.Helper()
	for _, review := range reviews {
		if review.ReviewKind == kind && review.Selected {
			return review
		}
	}
	t.Fatalf("selected review kind %q is missing", kind)
	return deliverydomain.MediaReview{}
}

func findJianyingPackage(t *testing.T, packages []deliverydomain.DeliveryPackage, snapshotID string) deliverydomain.DeliveryPackage {
	t.Helper()
	for _, pkg := range packages {
		for _, id := range pkg.ApprovedSnapshotIDs {
			if id == snapshotID {
				return pkg
			}
		}
	}
	t.Fatalf("delivery package for snapshot %q is missing", snapshotID)
	return deliverydomain.DeliveryPackage{}
}
