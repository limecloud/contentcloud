package mediapipeline

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestHTTPProviderSubmitStatusCancelAndStreamingDownload(t *testing.T) {
	secret := []byte("test-signing-secret")
	var seenSubmit bool
	var seenCancel bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer provider-token" {
			t.Errorf("authorization = %q", got)
		}
		if r.Header.Get("X-ContentCloud-Request-Digest") == "" || r.Header.Get("X-ContentCloud-Timestamp") == "" || r.Header.Get("X-ContentCloud-Signature") == "" {
			t.Errorf("request signature headers missing: %#v", r.Header)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/generations":
			seenSubmit = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["idempotency_key"] != "idem-1" {
				t.Errorf("unexpected submit payload: %#v err=%v", payload, err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"external_job_id":"job-1","provider_request_id":"req-1"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/generations/job-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"state":"succeeded","progress":100,"output_url":"/download/take.mp4","actual_cost_minor":42}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/generations/job-1/cancel":
			seenCancel = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/download/take.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Disposition", `attachment; filename="take.mp4"`)
			_, _ = w.Write(minimalMP4Fixture())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(HTTPProviderConfig{BaseURL: server.URL, AllowPrivateNetworks: true, AuthToken: "provider-token", SigningSecret: secret, Timeout: time.Second, MaxDownloadBytes: 1 << 20, Now: func() time.Time { return time.Unix(1700000000, 0).UTC() }})
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.ProviderProfile{Modes: []string{"image_to_video"}, Limits: map[string]any{"max_duration_seconds": 60}, Pricing: map[string]any{"currency": "CNY", "per_job_minor": 10}}
	request := Request{JobID: "job-local", IdempotencyKey: "idem-1", StoryboardSnapshotID: "story-1", Mode: "image_to_video", DurationSeconds: 15}
	if err := provider.Validate(request, profile); err != nil {
		t.Fatal(err)
	}
	submission, err := provider.Submit(t.Context(), request, profile)
	if err != nil || submission.ExternalJobID != "job-1" || submission.ProviderRequestID != "req-1" || !seenSubmit {
		t.Fatalf("unexpected submission: %#v err=%v", submission, err)
	}
	status, err := provider.Status(t.Context(), submission.ExternalJobID, profile)
	if err != nil || status.State != "succeeded" || status.Progress != 100 || status.OutputRef != "/download/take.mp4" || status.ActualMinor != 42 {
		t.Fatalf("unexpected status: %#v err=%v", status, err)
	}
	if err := provider.Cancel(t.Context(), submission.ExternalJobID, profile); err != nil || !seenCancel {
		t.Fatalf("cancel failed: %v", err)
	}
	stream, err := provider.OpenDownload(t.Context(), status.OutputRef, profile)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(stream.Body)
	_ = stream.Body.Close()
	if err != nil || stream.MediaType != "video/mp4" || stream.FileName != "take.mp4" || len(body) < 32 {
		t.Fatalf("unexpected streamed download: media=%q name=%q bytes=%d err=%v", stream.MediaType, stream.FileName, len(body), err)
	}
	if got := streamSignature(secret, "1700000000", "{}", "/v1/generations/job-1/cancel"); got == "" {
		t.Fatal("signature fixture was empty")
	}
}

func TestHTTPProviderRejectsSSRFAndOversizedDownload(t *testing.T) {
	if _, err := NewHTTPProvider(HTTPProviderConfig{BaseURL: "http://127.0.0.1:8080"}); !containsDomainCode(err, "PROVIDER_SSRF_BLOCKED") {
		t.Fatalf("private provider endpoint was accepted: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer server.Close()
	provider, err := NewHTTPProvider(HTTPProviderConfig{BaseURL: server.URL, AllowPrivateNetworks: true, MaxDownloadBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.OpenDownload(t.Context(), "/take.mp4", domain.ProviderProfile{}); !containsDomainCode(err, "MEDIA_OUTPUT_SIZE_INVALID") {
		t.Fatalf("oversized content length was accepted: %v", err)
	}
}

func TestHTTPProviderRetryableHTTPErrorIsSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"secret":"must-not-leak","message":"busy"}`)
	}))
	defer server.Close()
	provider, err := NewHTTPProvider(HTTPProviderConfig{BaseURL: server.URL, AllowPrivateNetworks: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Submit(t.Context(), Request{JobID: "j", IdempotencyKey: "i", StoryboardSnapshotID: "s", DurationSeconds: 1}, domain.ProviderProfile{Modes: []string{""}})
	if err == nil {
		t.Fatal("rate limited provider unexpectedly succeeded")
	}
	value, ok := err.(*domain.Error)
	if !ok || !value.Retryable || value.Code != "PROVIDER_HTTP_ERROR" {
		t.Fatalf("unexpected provider error: %#v", err)
	}
	if strings.Contains(value.Error(), "must-not-leak") {
		t.Fatalf("provider response leaked unsafe payload: %v", value)
	}
}

func streamSignature(secret []byte, timestamp, body, requestPath string) string {
	digest := sha256.Sum256([]byte(body))
	h := hmac.New(sha256.New, secret)
	_, _ = h.Write([]byte(timestamp + "\n" + hex.EncodeToString(digest[:]) + "\n" + requestPath))
	return hex.EncodeToString(h.Sum(nil))
}

func minimalMP4Fixture() []byte {
	return []byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2\x00\x00\x00\x08moov\x00\x00\x00\x08mdat")
}

func containsDomainCode(err error, code string) bool {
	value, ok := err.(*domain.Error)
	return ok && value.Code == code
}
