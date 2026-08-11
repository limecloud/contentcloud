package channeladapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fixtureRequest() Request {
	return Request{TenantID: "tenant-1", ProjectID: "project-1", DeliveryPackageID: "package-1", DeliveryDigest: "sha256:" + strings.Repeat("a", 64), Channel: "wechat_official_account", AccountRef: "account:brand", AuthorizationRef: "authorization:1", IdempotencyKey: "publish-1", Metadata: map[string]any{}}
}

func TestManualAdapterNeverClaimsPublication(t *testing.T) {
	adapter := ManualAdapter{}
	prepared, err := adapter.Prepare(t.Context(), fixtureRequest())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := adapter.Submit(t.Context(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != StateManualActionRequired || receipt.ExternalID != "" || receipt.PublishedAt != nil {
		t.Fatalf("manual preparation claimed publication: %#v", receipt)
	}
}

func TestHTTPAdapterSubmitInspectAndWithdraw(t *testing.T) {
	publishedAt := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/publications":
			if r.Header.Get("Idempotency-Key") != "publish-1" {
				t.Fatalf("missing idempotency key")
			}
			_ = json.NewEncoder(w).Encode(Receipt{State: StateSubmitted, ExternalID: "external-1", SafeSummary: map[string]any{"token": "secret", "stage": "queued"}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/publications/external-1":
			_ = json.NewEncoder(w).Encode(Receipt{State: StatePublished, ExternalURL: "https://example.com/article", PublishedAt: &publishedAt, SafeSummary: map[string]any{"stage": "published"}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/publications/external-1/withdraw":
			_ = json.NewEncoder(w).Encode(Receipt{State: StateWithdrawn, SafeSummary: map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := NewHTTP(HTTPConfig{Endpoint: server.URL, Client: server.Client(), AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := adapter.Prepare(t.Context(), fixtureRequest())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := adapter.Submit(t.Context(), prepared)
	if err != nil || receipt.State != StateSubmitted || receipt.SafeSummary["token"] != "[redacted]" {
		t.Fatalf("unexpected submit receipt %#v err=%v", receipt, err)
	}
	receipt, err = adapter.Inspect(t.Context(), receipt)
	if err != nil || receipt.State != StatePublished || receipt.PublishedAt == nil {
		t.Fatalf("unexpected inspect receipt %#v err=%v", receipt, err)
	}
	receipt, err = adapter.Withdraw(t.Context(), receipt, "内容过期")
	if err != nil || receipt.State != StateWithdrawn {
		t.Fatalf("unexpected withdraw receipt %#v err=%v", receipt, err)
	}
}
