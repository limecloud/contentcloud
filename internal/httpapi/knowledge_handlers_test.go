package httpapi_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestSourceUploadBFFCreatesRealRevisions(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	project := callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", app.CreateProjectInput{BrandName: "来源品牌", ProductName: "来源产品"})

	first := callMultipartBFF[domain.SourceRevision](t, client, server.URL+"/api/bff/projects/"+project.ID+"/sources/upload", "brief.txt", []byte("第一版真实来源"), map[string]string{"name": "品牌 Brief", "source_type": "local_import", "file_type": "text/plain"})
	if first.SourceID == "" || first.SHA256 == "" || first.ByteSize != int64(len([]byte("第一版真实来源"))) || first.ProcessingStatus != "pending" {
		t.Fatalf("unexpected uploaded source revision: %#v", first)
	}
	second := callMultipartBFF[domain.SourceRevision](t, client, server.URL+"/api/bff/sources/"+first.SourceID+"/revisions/upload", "brief.txt", []byte("第二版真实来源"), map[string]string{"file_type": "text/plain"})
	if second.SourceID != first.SourceID || second.SupersedesID != first.ID || second.SHA256 == first.SHA256 {
		t.Fatalf("source revision did not preserve history: first=%#v second=%#v", first, second)
	}
	fetched := callBFF[domain.SourceRevision](t, client, http.MethodGet, server.URL+"/api/bff/source-revisions/"+second.ID, nil)
	if fetched.ID != second.ID || fetched.SupersedesID != first.ID {
		t.Fatalf("source revision detail mismatch: %#v", fetched)
	}
}

func callMultipartBFF[T any](t *testing.T, client *http.Client, target, fileName string, content []byte, fields map[string]string) T {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		OK    bool          `json:"ok"`
		Data  T             `json:"data"`
		Error *domain.Error `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !envelope.OK {
		t.Fatalf("multipart request failed: status=%d error=%#v", response.StatusCode, envelope.Error)
	}
	return envelope.Data
}

func TestKnowledgeBFFVerticalSlice(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	project := callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects", app.CreateProjectInput{BrandName: "知识品牌", ProductName: "知识产品"})
	session, err := service.Login(t.Context(), "demo@contentcloud.local", "contentcloud-demo-2026")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := service.UploadSource(t.Context(), actor, project.ID, "规格来源", "brand_manual", "weight.txt", "text/plain", []byte("净重 50g"), "")
	if err != nil {
		t.Fatal(err)
	}
	worker := actor
	worker.Type = "worker"
	if _, err := service.CompleteSource(t.Context(), worker, revision.ID, app.CompleteSourceInput{DetectedMIME: "text/plain", Status: "ready", ParserVersion: "test", Evidence: []app.CreateEvidenceInput{{LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: "净重 50g"}}}, ""); err != nil {
		t.Fatal(err)
	}
	spans, err := service.Evidence(t.Context(), actor, revision.ID)
	if err != nil || len(spans) != 1 {
		t.Fatalf("expected one accepted evidence span, got %d: %v", len(spans), err)
	}
	fact := callBFF[domain.KnowledgeObject](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/knowledge-objects", map[string]any{"id": "fact:weight", "object_type": "FactAssertion", "layer": "product", "title": "净重", "evidence_refs": []string{spans[0].ID}})
	encodedFactID := strings.ReplaceAll(url.PathEscape(fact.ID), ":", "%3A")
	reviewed := callBFF[struct {
		Object   domain.KnowledgeObject   `json:"object"`
		Decision domain.KnowledgeDecision `json:"decision"`
	}](t, client, http.MethodPost, server.URL+"/api/bff/knowledge-objects/"+encodedFactID+"/transitions", app.ReviewKnowledgeObjectInput{ExpectedVersion: fact.Version, ExpectedDigest: fact.Digest, Decision: "approve", Reason: "已复核规格来源"})
	fact = reviewed.Object
	if fact.Status != "verified" || reviewed.Decision.ID == "" {
		t.Fatalf("unexpected knowledge review result: %#v", reviewed)
	}
	gap := callBFF[domain.KnowledgeObject](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/knowledge-objects", map[string]any{"id": "gap:audience", "object_type": "KnowledgeGap", "layer": "market", "status": "open", "title": "受众缺口", "next_action": "REQUEST_SOURCE"})
	pack := callBFF[domain.KnowledgePack](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/knowledge-packs", app.CreateKnowledgePackInput{ID: "pack:launch", Name: "上市知识包", Purpose: "launch", ObjectRefs: []domain.KnowledgePackObjectRef{{ObjectID: fact.ID, Version: fact.Version}, {ObjectID: gap.ID, Version: gap.Version}}})
	if !pack.QueryPolicy.RequireEvidence || !pack.QueryPolicy.BlockOnConflict || !pack.QueryPolicy.BlockOnRights {
		t.Fatalf("hard knowledge gates were not applied: %#v", pack.QueryPolicy)
	}
	published := callBFF[struct {
		Pack     domain.KnowledgePack     `json:"pack"`
		Snapshot domain.KnowledgeSnapshot `json:"snapshot"`
	}](t, client, http.MethodPost, server.URL+"/api/bff/knowledge-packs/"+strings.ReplaceAll(url.PathEscape(pack.ID), ":", "%3A")+"/publish", map[string]any{})
	if published.Pack.Status != "published" || published.Snapshot.ID == "" {
		t.Fatalf("unexpected published pack: %#v", published)
	}
	query := callBFF[domain.KnowledgeQueryResult](t, client, http.MethodPost, server.URL+"/api/bff/knowledge/query", map[string]any{"project_id": project.ID, "snapshot_id": published.Snapshot.ID, "channel": "short_video", "at": time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)})
	if len(query.Eligible) != 1 || query.Eligible[0].ObjectID != fact.ID || len(query.Gaps) != 1 || query.Gaps[0].ObjectID != gap.ID {
		t.Fatalf("unexpected knowledge query: %#v", query)
	}
	objects := callBFF[[]domain.KnowledgeObject](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/knowledge-objects", nil)
	if len(objects) != 3 {
		t.Fatalf("unexpected knowledge objects: %#v", objects)
	}
	decisions := callBFF[[]domain.KnowledgeDecision](t, client, http.MethodGet, server.URL+"/api/bff/knowledge-objects/"+encodedFactID+"/decisions", nil)
	if len(decisions) != 1 || decisions[0].ResultVersion != fact.Version {
		t.Fatalf("unexpected knowledge decisions: %#v", decisions)
	}
	snapshot := callBFF[domain.KnowledgeSnapshot](t, client, http.MethodGet, server.URL+"/api/bff/knowledge-snapshots/"+strings.ReplaceAll(url.PathEscape(published.Snapshot.ID), ":", "%3A"), nil)
	if snapshot.ID != published.Snapshot.ID || snapshot.PackID != pack.ID {
		t.Fatalf("unexpected knowledge snapshot: %#v", snapshot)
	}
	snapshots := callBFF[[]domain.KnowledgeSnapshot](t, client, http.MethodGet, server.URL+"/api/bff/projects/"+project.ID+"/knowledge-packs/"+strings.ReplaceAll(url.PathEscape(pack.ID), ":", "%3A")+"/snapshots", nil)
	if len(snapshots) != 1 || snapshots[0].ID != published.Snapshot.ID {
		t.Fatalf("unexpected knowledge snapshots: %#v", snapshots)
	}
}
