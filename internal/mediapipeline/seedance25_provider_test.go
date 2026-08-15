package mediapipeline

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestSeedance25ProviderSubmitStatusAndCancel(t *testing.T) {
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/contents/generations/tasks":
			if got := r.Header.Get("Authorization"); got != "Bearer secret" {
				t.Errorf("authorization = %q", got)
			}
			if got := r.Header.Get("Idempotency-Key"); got != "media-job-1" {
				t.Errorf("idempotency key = %q", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
				t.Fatal(err)
			}
			w.Header().Set("X-Request-Id", "request-1")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"task-1","request_id":"request-1"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/contents/generations/tasks/task-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"task-1","status":"succeeded","content":{"video_url":"https://cdn.example/video.mp4"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/contents/generations/tasks/task-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewSeedance25Provider(Seedance25ProviderConfig{
		HTTPProviderConfig: HTTPProviderConfig{BaseURL: server.URL, AllowPrivateNetworks: true, AuthToken: "secret"},
		Resolver: Seedance25InputResolverFunc(func(_ context.Context, _ Request, _ domain.ProviderProfile) (Seedance25Input, error) {
			return Seedance25Input{Prompt: "镜头向前推进", Images: []Seedance25Media{{URL: "data:image/png;base64,AAAA", MediaType: "image/png", Role: "first_frame"}}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.ProviderProfile{Model: "dreamina-seedance-2-5-260628", Modes: []string{"image_to_video"}, Limits: map[string]any{"max_duration_seconds": 30, "max_reference_images": 30}, Pricing: map[string]any{"currency": "CNY", "per_second_minor": 2}}
	request := Request{TenantID: "tenant-1", ProjectID: "project-1", JobID: "job-1", IdempotencyKey: "media-job-1", StoryboardSnapshotID: "snapshot-1", PromptPackageArtifactID: "prompt-1", ProfileVersion: "1.0.0", Mode: "image_to_video", AspectRatio: "9:16", DurationSeconds: 5}
	estimate, err := provider.Estimate(request, profile)
	if err != nil || estimate.CostMinor != 10 || estimate.Currency != "CNY" {
		t.Fatalf("estimate = %#v err=%v", estimate, err)
	}
	submission, err := provider.Submit(t.Context(), request, profile)
	if err != nil || submission.ExternalJobID != "task-1" || submission.ProviderRequestID != "request-1" {
		t.Fatalf("submission = %#v err=%v", submission, err)
	}
	content, _ := seenBody["content"].([]any)
	if len(content) != 2 || seenBody["model"] != profile.Model || seenBody["ratio"] != "9:16" {
		t.Fatalf("unexpected ModelArk body: %#v", seenBody)
	}
	status, err := provider.Status(t.Context(), submission.ExternalJobID, profile)
	if err != nil || status.State != "succeeded" || status.OutputRef != "https://cdn.example/video.mp4" || status.Progress != 100 {
		t.Fatalf("status = %#v err=%v", status, err)
	}
	if err := provider.Cancel(t.Context(), submission.ExternalJobID, profile); err != nil {
		t.Fatal(err)
	}
}

func TestSeedance25ProviderRejectsUnsupportedInputAndUnknownSubmit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"temporary"}}`))
	}))
	defer server.Close()
	provider, err := NewSeedance25Provider(Seedance25ProviderConfig{
		HTTPProviderConfig: HTTPProviderConfig{BaseURL: server.URL, AllowPrivateNetworks: true},
		Resolver: Seedance25InputResolverFunc(func(_ context.Context, _ Request, _ domain.ProviderProfile) (Seedance25Input, error) {
			return Seedance25Input{Prompt: "x", Images: []Seedance25Media{{URL: "http://private.local/image.png", MediaType: "image/png"}}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.ProviderProfile{Model: "model", Modes: []string{"image_to_video"}, Pricing: map[string]any{"currency": "CNY", "per_job_minor": 1}}
	request := Request{JobID: "job", IdempotencyKey: "key", StoryboardSnapshotID: "snapshot", PromptPackageArtifactID: "prompt", Mode: "image_to_video", DurationSeconds: 1}
	if _, err := provider.Submit(t.Context(), request, profile); !containsDomainCode(err, "SEEDANCE_INPUT_URL_BLOCKED") {
		t.Fatalf("expected blocked input URL, got %v", err)
	}
	validProvider, err := NewSeedance25Provider(Seedance25ProviderConfig{
		HTTPProviderConfig: HTTPProviderConfig{BaseURL: server.URL, AllowPrivateNetworks: true},
		Resolver: Seedance25InputResolverFunc(func(_ context.Context, _ Request, _ domain.ProviderProfile) (Seedance25Input, error) {
			return Seedance25Input{Prompt: "x", Images: []Seedance25Media{{URL: "data:image/png;base64,AAAA", MediaType: "image/png"}}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validProvider.Submit(t.Context(), request, profile); !containsDomainCode(err, "PROVIDER_HTTP_ERROR") {
		t.Fatalf("expected provider HTTP error, got %v", err)
	}
	var providerErr *domain.Error
	_, err = validProvider.Submit(t.Context(), request, profile)
	if err == nil || !errors.As(err, &providerErr) || !providerErr.Retryable {
		t.Fatalf("expected normalized provider error, got %T: %v", err, err)
	}
}

func TestSeedance25ProviderEnforcesPhaseOneLimits(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	provider, err := NewSeedance25Provider(Seedance25ProviderConfig{
		HTTPProviderConfig: HTTPProviderConfig{BaseURL: server.URL, AllowPrivateNetworks: true},
		Resolver: Seedance25InputResolverFunc(func(_ context.Context, _ Request, _ domain.ProviderProfile) (Seedance25Input, error) {
			return Seedance25Input{}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.ProviderProfile{Modes: []string{"text_to_video", "image_to_video"}}
	request := Request{JobID: "job", IdempotencyKey: "key", StoryboardSnapshotID: "snapshot", PromptPackageArtifactID: "prompt", Mode: "text_to_video", DurationSeconds: 30}
	if err := provider.Validate(request, profile); err != nil {
		t.Fatalf("expected 30-second request to pass, got %v", err)
	}
	request.DurationSeconds = 31
	if err := provider.Validate(request, profile); !containsDomainCode(err, "PROVIDER_DURATION_LIMIT_EXCEEDED") {
		t.Fatalf("expected duration limit error, got %v", err)
	}

	request.Mode = "text_to_video"
	request.DurationSeconds = 1
	if err := validateSeedanceInput(request, Seedance25Input{Prompt: strings.Repeat("镜", 32000)}, profile); err != nil {
		t.Fatalf("expected 32000 UTF-8 characters to pass, got %v", err)
	}
	if err := validateSeedanceInput(request, Seedance25Input{Prompt: strings.Repeat("镜", 32001)}, profile); !containsDomainCode(err, "SEEDANCE_PROMPT_TOO_LARGE") {
		t.Fatalf("expected prompt character limit error, got %v", err)
	}

	request.Mode = "image_to_video"
	images := make([]Seedance25Media, 31)
	for i := range images {
		images[i] = Seedance25Media{URL: "data:image/png;base64,AAAA", MediaType: "image/png"}
	}
	if err := validateSeedanceInput(request, Seedance25Input{Prompt: "镜头", Images: images[:30]}, profile); err != nil {
		t.Fatalf("expected 30 reference images to pass, got %v", err)
	}
	if err := validateSeedanceInput(request, Seedance25Input{Prompt: "镜头", Images: images}, profile); !containsDomainCode(err, "PROVIDER_REFERENCE_LIMIT_EXCEEDED") {
		t.Fatalf("expected reference image limit error, got %v", err)
	}
}

func TestSeedance25ProviderBlocksUncontrolledHTTPSAndInvalidDataURLs(t *testing.T) {
	request := Request{JobID: "job", IdempotencyKey: "key", StoryboardSnapshotID: "snapshot", PromptPackageArtifactID: "prompt", Mode: "image_to_video", DurationSeconds: 1}
	profile := domain.ProviderProfile{Modes: []string{"image_to_video"}}
	if err := validateSeedanceInput(request, Seedance25Input{Prompt: "镜头", Images: []Seedance25Media{{URL: "data:image/png;base64,not-base64", MediaType: "image/png"}}}, profile); !containsDomainCode(err, "SEEDANCE_INPUT_URL_INVALID") {
		t.Fatalf("expected invalid data URL error, got %v", err)
	}
	if err := validateSeedanceInput(request, Seedance25Input{Prompt: "镜头", Images: []Seedance25Media{{URL: "https://untrusted.example/image.png", MediaType: "image/png"}}}, profile); err != nil {
		t.Fatalf("expected syntactically valid HTTPS URL before provider host policy, got %v", err)
	}
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	provider, err := NewSeedance25Provider(Seedance25ProviderConfig{
		HTTPProviderConfig: HTTPProviderConfig{BaseURL: server.URL, AllowPrivateNetworks: true, AllowedHosts: []string{"127.0.0.1"}},
		Resolver: Seedance25InputResolverFunc(func(context.Context, Request, domain.ProviderProfile) (Seedance25Input, error) {
			return Seedance25Input{Prompt: "镜头", Images: []Seedance25Media{{URL: "https://untrusted.example/image.png", MediaType: "image/png"}}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Submit(t.Context(), request, profile); !containsDomainCode(err, "SEEDANCE_INPUT_HOST_NOT_ALLOWED") {
		t.Fatalf("expected provider host policy error, got %v", err)
	}
}

func TestSeedance25ProviderReadsWrappedStatusAndNestedOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"status":"succeeded","content":[{"type":"video_url","video_url":{"url":"https://cdn.example/wrapped.mp4"}}]}}`))
	}))
	defer server.Close()
	provider, err := NewSeedance25Provider(Seedance25ProviderConfig{
		HTTPProviderConfig: HTTPProviderConfig{BaseURL: server.URL, AllowPrivateNetworks: true},
		Resolver: Seedance25InputResolverFunc(func(_ context.Context, _ Request, _ domain.ProviderProfile) (Seedance25Input, error) {
			return Seedance25Input{}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := provider.Status(t.Context(), "wrapped-task", domain.ProviderProfile{})
	if err != nil || status.State != "succeeded" || status.OutputRef != "https://cdn.example/wrapped.mp4" {
		t.Fatalf("wrapped status=%#v err=%v", status, err)
	}
}
