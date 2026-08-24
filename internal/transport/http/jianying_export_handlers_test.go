package httpapi_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/application"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
	"github.com/limecloud/contentcloud/internal/work"
)

func TestJianyingExportBFFReturnsVerifiedArchiveAndIsTenantScoped(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default(), application.WithPlatformAdminEmails("demo@contentcloud.local"))
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	fixture := mustStudioBootstrap(t, client, server.URL)
	tasks := callBFF[[]work.WorkTask](t, client, http.MethodGet, server.URL+"/api/bff/tasks?project_id="+fixture.Projects[0].ID, nil)
	if len(tasks) != 1 {
		t.Fatalf("fixture task count=%d", len(tasks))
	}
	view := callBFF[application.WorkTaskView](t, client, http.MethodGet, server.URL+"/api/bff/tasks/"+tasks[0].ID, nil)
	finalArtifact := httpJianyingArtifact(t, view.Artifacts, "final_render")
	snapshot := httpJianyingSnapshot(t, view.ApprovedSnapshots, finalArtifact.ApprovedSnapshotID)
	selectedReview := httpJianyingReview(t, view.MediaReviews, deliverydomain.MediaReviewContent)
	finalReview := httpJianyingReview(t, view.MediaReviews, deliverydomain.MediaReviewFinal)
	selectedArtifact := httpJianyingArtifactByID(t, view.Artifacts, selectedReview.SubjectArtifactID)
	pkg := httpJianyingPackage(t, view.DeliveryPackages, snapshot.ID)
	manifest := httpJianyingManifest(snapshot, selectedReview, selectedArtifact)

	response := callJianyingEndpoint(t, client, server.URL+"/api/bff/projects/"+fixture.Projects[0].ID+"/jianying-export", manifest, snapshot.ID, finalReview.ID, pkg.ID)
	if response.FileName == "" || response.ContentBase64 == "" || response.ManifestDigest == "" || response.ArchiveDigest == "" || response.ByteSize == 0 {
		t.Fatalf("BFF returned incomplete Jianying export response: %#v", response)
	}
	if len(responseBody(t, response.ContentBase64)) != response.ByteSize {
		t.Fatalf("BFF byte_size=%d does not match decoded archive", response.ByteSize)
	}

	foreignSession, err := service.Identity.Register(t.Context(), "jianying-http-foreign@example.com", "long-enough-password", "外部用户", "外部团队")
	if err != nil {
		t.Fatal(err)
	}
	foreignJar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	foreignJar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: foreignSession.ID, Path: "/"}})
	foreignClient := &http.Client{Jar: foreignJar}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/bff/projects/"+fixture.Projects[0].ID+"/jianying-export", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	foreignResponse, err := foreignClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	foreignBody, _ := io.ReadAll(foreignResponse.Body)
	foreignResponse.Body.Close()
	if foreignResponse.StatusCode != http.StatusNotFound || strings.Contains(string(foreignBody), snapshot.ID) {
		t.Fatalf("cross-tenant export status=%d body=%s", foreignResponse.StatusCode, foreignBody)
	}
}

func TestJianyingExportBFFRequiresSessionAndRejectsUnapprovedReview(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/bff/projects/project-1/jianying-export", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated Jianying export status=%d, want 401", response.StatusCode)
	}
}

type jianyingHTTPResponse struct {
	FileName       string `json:"file_name"`
	ContentBase64  string `json:"content_base64"`
	ManifestDigest string `json:"manifest_digest"`
	ArchiveDigest  string `json:"archive_digest"`
	ByteSize       int    `json:"byte_size"`
}

func callJianyingEndpoint(t *testing.T, client *http.Client, target string, manifest deliverydomain.CompositionManifest, snapshotID, reviewID, packageID string) jianyingHTTPResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{"approved_snapshot_id": snapshotID, "final_review_id": reviewID, "delivery_package_id": packageID, "manifest": manifest})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		OK   bool                 `json:"ok"`
		Data jianyingHTTPResponse `json:"data"`
		Err  *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !envelope.OK {
		t.Fatalf("Jianying export status=%d error=%#v", response.StatusCode, envelope.Err)
	}
	return envelope.Data
}

func responseBody(t *testing.T, encoded string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func httpJianyingArtifact(t *testing.T, artifacts []deliverydomain.Artifact, kind string) deliverydomain.Artifact {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact
		}
	}
	t.Fatalf("artifact kind %q is missing", kind)
	return deliverydomain.Artifact{}
}

func httpJianyingArtifactByID(t *testing.T, artifacts []deliverydomain.Artifact, id string) deliverydomain.Artifact {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.ID == id {
			return artifact
		}
	}
	t.Fatalf("artifact %q is missing", id)
	return deliverydomain.Artifact{}
}

func httpJianyingSnapshot(t *testing.T, snapshots []reviewdomain.ApprovedSnapshot, id string) reviewdomain.ApprovedSnapshot {
	t.Helper()
	for _, snapshot := range snapshots {
		if snapshot.ID == id {
			return snapshot
		}
	}
	t.Fatalf("snapshot %q is missing", id)
	return reviewdomain.ApprovedSnapshot{}
}

func httpJianyingReview(t *testing.T, reviews []deliverydomain.MediaReview, kind string) deliverydomain.MediaReview {
	t.Helper()
	for _, review := range reviews {
		if review.ReviewKind == kind && review.Selected {
			return review
		}
	}
	t.Fatalf("review kind %q is missing", kind)
	return deliverydomain.MediaReview{}
}

func httpJianyingPackage(t *testing.T, packages []deliverydomain.DeliveryPackage, snapshotID string) deliverydomain.DeliveryPackage {
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

func httpJianyingManifest(snapshot reviewdomain.ApprovedSnapshot, selected deliverydomain.MediaReview, source deliverydomain.Artifact) deliverydomain.CompositionManifest {
	reviewDigest, _ := stablehash.Sum(struct {
		ID            string         `json:"id"`
		SubjectDigest string         `json:"subject_digest"`
		ReviewKind    string         `json:"review_kind"`
		Status        string         `json:"status"`
		Checks        map[string]any `json:"checks"`
		Selected      bool           `json:"selected"`
		RowVersion    int            `json:"row_version"`
	}{selected.ID, selected.SubjectDigest, selected.ReviewKind, selected.Status, selected.Checks, selected.Selected, selected.RowVersion})
	return deliverydomain.CompositionManifest{
		Schema:           deliverydomain.CompositionSchemaRef{Name: deliverydomain.CompositionManifestSchema, Version: "1.0"},
		ApprovedSnapshot: deliverydomain.CompositionSnapshotRef{ID: snapshot.ID, Digest: snapshot.SubjectHash},
		SelectedVideo:    deliverydomain.CompositionVideoRef{ArtifactID: source.ID, Digest: normalizeHTTPDigest(source.SHA256), MediaReviewID: selected.ID, ReviewDigest: "sha256:" + reviewDigest},
		Narration:        deliverydomain.CompositionNarration{Status: "disabled"},
		Subtitles:        deliverydomain.CompositionSubtitles{Status: "disabled", SourceKind: "none"},
		BrandCTA:         deliverydomain.CompositionBrandCTA{BrandConfigDigest: "sha256:" + strings.Repeat("0", 64), CTADigest: "sha256:" + strings.Repeat("1", 64)},
		Timeline:         deliverydomain.CompositionTimeline{CanvasWidth: 1080, CanvasHeight: 1920, FrameRateNumerator: 30, FrameRateDenominator: 1, DurationMS: 1000, TransitionVersion: "test/1", Segments: []deliverydomain.CompositionSegment{{ID: "source", StartMS: 0, EndMS: 1000, ArtifactID: source.ID, ArtifactDigest: normalizeHTTPDigest(source.SHA256)}}},
		Renderer:         deliverydomain.CompositionRenderer{Name: "test-renderer", Version: "1", CapabilityDigest: "sha256:" + strings.Repeat("2", 64)},
		Output:           deliverydomain.CompositionOutput{Kind: "final_render", MediaType: "video/mp4", Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"},
	}
}

func normalizeHTTPDigest(value string) string {
	value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
	return "sha256:" + value
}
