package localworkspace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHTTPMemoryAdapterRevalidatesScopeAndSourceBeforeRecall(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	source := filepath.Join(root, "40-work", "remote.md")
	if err := os.WriteFile(source, []byte("远程记忆来源：年轻白领反馈。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-token" || request.URL.Path != "/query" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		var input MemoryRemoteQueryRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.Scope.WorkspaceID != "workspace-memory" || input.Scope.ProjectID != "project-memory" {
			http.Error(writer, "bad scope", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"candidates": []MemoryCandidate{
			{SchemaVersion: MemoryEntrySchema, MemoryID: "remote-good", Kind: "working", Scope: input.Scope, SourceRef: "40-work/remote.md", SourceDigest: memoryDigest(body), Summary: "远程召回的年轻白领候选", Trust: "memory_candidate", Status: "active", FormedBy: "remote/test", ObservedAt: time.Now().UTC()},
			{SchemaVersion: MemoryEntrySchema, MemoryID: "remote-wrong-scope", Kind: "working", Scope: MemoryScope{WorkspaceID: "other", ProjectID: "other"}, SourceRef: "40-work/remote.md", SourceDigest: memoryDigest(body), Summary: "越权候选", Trust: "memory_candidate", Status: "active"},
			{SchemaVersion: MemoryEntrySchema, MemoryID: "remote-stale", Kind: "working", Scope: input.Scope, SourceRef: "40-work/remote.md", SourceDigest: "sha256:stale", Summary: "陈旧候选", Trust: "memory_candidate", Status: "active"},
		}})
	}))
	defer server.Close()
	adapter, err := NewMemoryRemoteAdapter(MemoryRemoteAdapterConfig{Provider: "mem0", BaseURL: server.URL, AuthToken: "test-token", QueryPath: "/query", RememberPath: "/remember", AllowPrivateNetworks: true, AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := QueryRemoteMemory(t.Context(), root, adapter, MemoryQueryOptions{Root: root, Query: "年轻白领", Limit: 5, MaxChars: 2000})
	if err != nil || result.Backend != MemoryRemoteBackendPrefix+"mem0" || len(result.Candidates) != 1 || result.Candidates[0].MemoryID != "remote-good" || len(result.Warnings) != 2 {
		t.Fatalf("remote candidates were not safely filtered: %#v err=%v", result, err)
	}
}

func TestHTTPMemoryAdapterRememberSendsBoundRecord(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	source := filepath.Join(root, "40-work", "remote-remember.md")
	if err := os.WriteFile(source, []byte("远程写入来源。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	record, err := RememberMemory(MemoryRememberOptions{Root: root, MemoryID: "remote-record", Kind: "working", SourceRef: "40-work/remote-remember.md", Summary: "远程适配器测试候选"})
	if err != nil {
		t.Fatal(err)
	}
	received := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body MemoryRecord
		if request.URL.Path != "/remember" || request.Header.Get("Authorization") != "Bearer test-token" || json.NewDecoder(request.Body).Decode(&body) != nil || body.Scope != record.Record.Scope || body.MemoryID != record.Record.MemoryID {
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		received = true
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	adapter, err := NewMemoryRemoteAdapter(MemoryRemoteAdapterConfig{Provider: "tencentdb", BaseURL: server.URL, AuthToken: "test-token", RememberPath: "/remember", AllowPrivateNetworks: true, AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Remember(t.Context(), record.Record); err != nil || !received {
		t.Fatalf("remote remember failed: %v received=%v", err, received)
	}
}

func TestExtractMemoryBindsEveryCandidateToRequestedSource(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "40-work", "extract.md"), []byte("抽取来源正文。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/extract" {
			http.Error(writer, "bad path", http.StatusNotFound)
			return
		}
		var input MemoryRemoteExtractRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.SourceRef != "40-work/extract.md" || input.Content == "" || input.SourceDigest == "" {
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"candidates": []MemoryExtractedCandidate{
			{Kind: "knowledge", Summary: "抽取出的候选", ClaimKey: "extract.claim", FormedBy: "llm/test"},
			{Kind: "", Summary: "无类型候选"},
		}})
	}))
	defer server.Close()
	adapter, err := NewMemoryRemoteAdapter(MemoryRemoteAdapterConfig{Provider: "extractor", BaseURL: server.URL, ExtractPath: "/extract", AllowPrivateNetworks: true, AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	report, err := ExtractMemory(t.Context(), MemoryExtractOptions{Root: root, SourceRefs: []string{"40-work/extract.md"}, Adapter: adapter, Now: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)})
	if err != nil || report.CandidateCount != 2 || len(report.Remembered) != 1 || len(report.Rejected) != 1 || report.Remembered[0].Record.SourceRef != "40-work/extract.md" {
		t.Fatalf("unexpected extraction report: %#v err=%v", report, err)
	}
}

func TestEmbeddingAdapterSupportsExplicitHybridRerank(t *testing.T) {
	root := initMemoryTestWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "40-work", "embed.md"), []byte("Embedding 候选来源。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/embeddings" {
			http.Error(writer, "bad path", http.StatusNotFound)
			return
		}
		var input MemoryEmbeddingRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || len(input.Input) < 2 {
			http.Error(writer, "bad input", http.StatusBadRequest)
			return
		}
		items := make([]map[string]any, len(input.Input))
		for index := range input.Input {
			items[index] = map[string]any{"index": index, "embedding": []float32{1, 0}}
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": items})
	}))
	defer server.Close()
	adapter, err := NewMemoryRemoteAdapter(MemoryRemoteAdapterConfig{Provider: "embedding-test", BaseURL: server.URL, EmbeddingPath: "/embeddings", EmbeddingModel: "test-model", AllowPrivateNetworks: true, AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := QueryMemoryWithEmbedding(t.Context(), root, adapter, MemoryQueryOptions{Root: root, Query: "Embedding", Limit: 5, MaxChars: 2000})
	if err != nil || result.Backend != "remote:embedding-test+hybrid" || len(result.Candidates) == 0 {
		t.Fatalf("unexpected hybrid query result: %#v err=%v", result, err)
	}
}
