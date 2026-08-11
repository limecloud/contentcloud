package modelprovider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSGLangOpenAICompatibleAdapterDetectsAndCompletes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "content-model"}}})
		case "/v1/chat/completions":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request["model"] != "content-model" || request["response_format"] == nil {
				t.Fatalf("structured request missing: %#v", request)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "request-1", "model": "content-model", "choices": []any{map[string]any{"message": map[string]any{"content": `{"title":"候选标题"}`}}}, "usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := NewSGLang(Config{Endpoint: server.URL, Model: "content-model", APIKey: "key", Client: server.Client(), AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	capability, err := adapter.Detect(t.Context())
	if err != nil || capability.Provider != "sglang" || !capability.StructuredOutput {
		t.Fatalf("unexpected capability %#v err=%v", capability, err)
	}
	result, err := adapter.Complete(t.Context(), CompletionRequest{Messages: []Message{{Role: "user", Content: "生成标题"}}, ResponseSchema: map[string]any{"type": "object", "required": []string{"title"}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "sglang" || len(result.Structured) == 0 || result.TotalTokens != 12 {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestVLLMAdapterRejectsInvalidStructuredOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "not-json"}}}})
	}))
	defer server.Close()
	adapter, err := NewVLLM(Config{Endpoint: server.URL, Model: "content-model", Client: server.Client(), AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Complete(t.Context(), CompletionRequest{Messages: []Message{{Role: "user", Content: "generate"}}, ResponseSchema: map[string]any{"type": "object"}})
	if err == nil {
		t.Fatal("invalid structured output must be rejected")
	}
}
