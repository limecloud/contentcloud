package httpapi_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestPerformanceResultBFFIsAtomicAndSupportsRatings(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	service := app.New(store, slog.Default())
	session, err := service.Register(ctx, "results-bff@example.com", "long-enough-password", "Results", "Results Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Results Brand", ProductName: "Results Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	logical := domain.Script{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, Title: "Results Script", CreatedAt: now}
	script, err := store.CreateScript(ctx, logical, domain.ScriptVersion{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, Status: "approved", Package: domain.ScriptPackage{Title: "Results Script"}, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}

	created := callBFF[app.ImportPerformanceResult](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/results", app.CreateObservationInput{ScriptVersionID: script.ID, Platform: "douyin", AccountAlias: "brand-main", PublishedAt: now.Add(-24 * time.Hour), WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"impressions": 1200}, Currency: "CNY", Spend: 100, GMV: 250, IssueCategory: "creative"})
	if created.Batch.ID == "" || len(created.Observations) != 1 || created.Observations[0].ROI == nil || *created.Observations[0].ROI != 2.5 {
		t.Fatalf("unexpected result response: %#v", created)
	}
	batches := callBFF[[]domain.PerformanceImportBatch](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/performance-imports", nil)
	if len(batches) != 1 || batches[0].ID != created.Batch.ID {
		t.Fatalf("batch route did not return the immutable import: %#v", batches)
	}
	details := callBFF[domain.PerformanceImportDetails](t, client, http.MethodGet, server.URL+"/api/bff/performance-imports/"+created.Batch.ID, nil)
	if len(details.Observations) != 1 || details.Observations[0].ImportBatchID != details.Batch.ID {
		t.Fatalf("batch detail lineage is incomplete: %#v", details)
	}
	rating := callBFF[app.CreateRatingDecisionResult](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/rating-decisions", app.CreateRatingDecisionInput{SubjectType: "script_version", SubjectID: script.ID, ObservationIDs: []string{created.Observations[0].ID}, Rating: "seed_candidate", Reason: "人工判断可进入下一轮", NextAction: "创建单变量变体"})
	if rating.Decision.ID == "" {
		t.Fatalf("rating decision was not created: %#v", rating)
	}
	ratings := callBFF[[]domain.RatingDecision](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/rating-decisions", nil)
	if len(ratings) != 1 || ratings[0].ID != rating.Decision.ID {
		t.Fatalf("rating list mismatch: %#v", ratings)
	}
	graph := callBFF[domain.LineageGraph](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/lineage?focus_type=script_version&focus_id="+script.ID+"&direction=both", nil)
	if graph.FocusKey != "script_version:"+script.ID || !lineageContains(graph, "performance_observation:"+created.Observations[0].ID) || !lineageContains(graph, "rating_decision:"+rating.Decision.ID) {
		t.Fatalf("BFF lineage projection is incomplete: %#v", graph)
	}
	impact := callBFF[domain.ImpactAnalysis](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/impact?focus_type=script_version&focus_id="+script.ID, nil)
	if impact.Focus == nil || impact.Focus.ID != script.ID || len(impact.Items) < 2 {
		t.Fatalf("BFF impact projection is incomplete: %#v", impact)
	}
	audits := callBFF[[]domain.AuditEvent](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/audit", nil)
	if !hasAuditAction(audits, "performance_import.created") || !hasAuditAction(audits, "rating_decision.created") {
		t.Fatalf("result writes are missing immutable audit coverage: %#v", audits)
	}

	badInput := app.ImportPerformanceInput{SourceName: "mixed.csv", SourceFormat: "csv", Observations: []app.CreateObservationInput{
		{RowNumber: 2, ScriptVersionID: script.ID, Platform: "douyin", AccountAlias: "account-a", PublishedAt: now.Add(-48 * time.Hour), WindowHours: 24, SampleStatus: "insufficient_sample", Metrics: map[string]float64{}, Currency: "CNY", Spend: 10},
		{RowNumber: 3, ScriptVersionID: script.ID, Platform: "douyin", AccountAlias: "account-b", PublishedAt: now.Add(-48 * time.Hour), WindowHours: 24, SampleStatus: "insufficient_sample", Metrics: map[string]float64{}, Currency: "USD", Spend: 10},
	}}
	body, _ := json.Marshal(badInput)
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/performance-imports", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var rejected struct {
		OK    bool          `json:"ok"`
		Error *domain.Error `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&rejected); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest || rejected.Error == nil || rejected.Error.Code != "RESULT_IMPORT_REJECTED" || rejected.Error.Details == nil {
		t.Fatalf("expected structured atomic rejection, status=%d response=%#v", response.StatusCode, rejected)
	}
	remaining := callBFF[[]domain.PerformanceObservation](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/results", nil)
	if len(remaining) != 1 {
		t.Fatalf("rejected batch left partial observations: %#v", remaining)
	}
}

func lineageContains(graph domain.LineageGraph, key string) bool {
	for _, node := range graph.Nodes {
		if node.Key == key {
			return true
		}
	}
	return false
}

func hasAuditAction(events []domain.AuditEvent, action string) bool {
	for _, event := range events {
		if event.Action == action && event.RequestID != "" && event.SubjectID != "" {
			return true
		}
	}
	return false
}
