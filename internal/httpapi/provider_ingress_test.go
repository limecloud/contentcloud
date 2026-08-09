package httpapi_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestProviderCallbackIngressAuthenticatesAndDeduplicates(t *testing.T) {
	store := memory.New()
	service := app.New(store, nil)
	now := time.Now().UTC()
	profile := domain.ProviderProfile{ProviderID: "provider-a", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), AdapterVersion: "provider-a/1", Model: "model", Region: "global", Modes: []string{"image_to_video"}, InputMediaTypes: []string{"application/json"}, OutputMediaType: "video/mp4", DataRetention: "ephemeral", Status: "published", VerifiedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	if err := store.CreateProviderProfile(t.Context(), profile); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProviderBinding(t.Context(), domain.ProviderBinding{TenantID: "tenant-1", ProviderID: "provider-a", ProfileVersion: profile.Version, State: "active", EgressPolicy: "public", MaxConcurrency: 1, UpdatedBy: "test", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	started, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: "tenant-1", ProjectID: "project-1", WorkTaskID: "provider-task", BusinessType: "media.generate", SOP: ingressSOP(), BindingDigest: "sha256:" + strings.Repeat("b", 64), InputDigest: "sha256:" + strings.Repeat("c", 64), RuntimePolicyID: "runtime.test/1", ContractMajor: 1, CreatedBy: "test", IdempotencyKey: "provider-ingress-job"})
	if err != nil {
		t.Fatal(err)
	}
	effect, err := service.Runtime().RegisterEffect(t.Context(), domain.ExternalEffect{TenantID: "tenant-1", JobRunID: started.Job.ID, NodeRunID: started.Nodes[0].ID, Kind: "media.generate", IdempotencyKey: "provider-ingress-effect", RequestDigest: "sha256:" + strings.Repeat("d", 64), Currency: "CNY", SafeSummary: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	effect, err = service.Runtime().ReconcileEffect(t.Context(), "tenant-1", effect.ID, domain.EffectSubmitted, "external-1", "", "", effect.Version)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("ingress-secret")
	server := httptest.NewServer(httpapi.New(service, nil, false, "", httpapi.WithProviderCallbackSecret("tenant-1", "provider-a", secret)).Handler())
	defer server.Close()
	payload := []byte(`{"effect_id":"` + effect.ID + `","message_id":"message-1","external_id":"external-1","provider_state":"completed","cost_minor":0,"currency":"CNY","safe_payload":{"status":"completed"}}`)
	request := signedProviderRequest(t, server.URL+"/api/v1/providers/provider-a/tenants/tenant-1/callbacks", payload, secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("callback status=%d body=%s", response.StatusCode, body)
	}
	var envelope struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || !envelope.OK {
		t.Fatalf("callback response=%#v err=%v", envelope, err)
	}
	replayed := signedProviderRequest(t, server.URL+"/api/v1/providers/provider-a/tenants/tenant-1/callbacks", payload, secret)
	replayResponse, err := http.DefaultClient.Do(replayed)
	if err != nil || replayResponse.StatusCode != http.StatusOK {
		t.Fatalf("callback replay status=%v err=%v", replayResponse, err)
	}
	if replayResponse != nil {
		_ = replayResponse.Body.Close()
	}
	stored, err := service.Runtime().Effect(t.Context(), "tenant-1", effect.ID)
	if err != nil || stored.State != domain.EffectSucceeded {
		t.Fatalf("callback did not converge effect: %#v err=%v", stored, err)
	}
}

func TestProviderCallbackIngressRejectsBadSignatureAndOversizeBody(t *testing.T) {
	store := memory.New()
	service := app.New(store, nil)
	now := time.Now().UTC()
	profile := domain.ProviderProfile{ProviderID: "provider-a", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), AdapterVersion: "provider-a/1", Model: "model", Region: "global", Modes: []string{"image_to_video"}, InputMediaTypes: []string{"application/json"}, OutputMediaType: "video/mp4", DataRetention: "ephemeral", Status: "published", VerifiedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	_ = store.CreateProviderProfile(t.Context(), profile)
	_ = store.SaveProviderBinding(t.Context(), domain.ProviderBinding{TenantID: "tenant-1", ProviderID: "provider-a", ProfileVersion: profile.Version, State: "active", EgressPolicy: "public", MaxConcurrency: 1, UpdatedBy: "test", UpdatedAt: now})
	server := httptest.NewServer(httpapi.New(service, nil, false, "", httpapi.WithProviderCallbackSecret("tenant-1", "provider-a", []byte("secret"))).Handler())
	defer server.Close()
	request := signedProviderRequest(t, server.URL+"/api/v1/providers/provider-a/tenants/tenant-1/callbacks", []byte(`{"message_id":"m","external_id":"e","provider_state":"completed","safe_payload":{}}`), []byte("wrong"))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad signature status=%d", response.StatusCode)
	}
	_ = response.Body.Close()
	oversize := bytes.Repeat([]byte("x"), 256<<10+1)
	request = signedProviderRequest(t, server.URL+"/api/v1/providers/provider-a/tenants/tenant-1/callbacks", oversize, []byte("secret"))
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversize status=%d", response.StatusCode)
	}
	_ = response.Body.Close()
}

func TestProviderBillIngressAuthenticatesMatchesAndDeduplicates(t *testing.T) {
	store := memory.New()
	service := app.New(store, nil)
	now := time.Now().UTC()
	profile := domain.ProviderProfile{ProviderID: "provider-a", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), AdapterVersion: "provider-a/1", Model: "model", Region: "global", Modes: []string{"image_to_video"}, InputMediaTypes: []string{"application/json"}, OutputMediaType: "video/mp4", DataRetention: "ephemeral", Status: "published", VerifiedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	if err := store.CreateProviderProfile(t.Context(), profile); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProviderBinding(t.Context(), domain.ProviderBinding{TenantID: "tenant-1", ProviderID: "provider-a", ProfileVersion: profile.Version, State: "active", EgressPolicy: "public", MaxConcurrency: 1, UpdatedBy: "test", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	started, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: "tenant-1", ProjectID: "project-1", WorkTaskID: "provider-bill-job", BusinessType: "media.generate", SOP: ingressSOP(), BindingDigest: "sha256:" + strings.Repeat("b", 64), InputDigest: "sha256:" + strings.Repeat("c", 64), RuntimePolicyID: "runtime.test/1", ContractMajor: 1, CreatedBy: "test", IdempotencyKey: "provider-bill-job"})
	if err != nil {
		t.Fatal(err)
	}
	effect, err := service.Runtime().RegisterEffect(t.Context(), domain.ExternalEffect{TenantID: "tenant-1", JobRunID: started.Job.ID, NodeRunID: started.Nodes[0].ID, Kind: "media.generate", IdempotencyKey: "provider-bill-effect", RequestDigest: "sha256:" + strings.Repeat("d", 64), CostMinor: 120, Currency: "CNY", SafeSummary: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Runtime().ReconcileEffect(t.Context(), "tenant-1", effect.ID, domain.EffectSubmitted, "external-1", "", "", effect.Version); err != nil {
		t.Fatal(err)
	}
	secret := []byte("ingress-secret")
	server := httptest.NewServer(httpapi.New(service, nil, false, "", httpapi.WithProviderCallbackSecret("tenant-1", "provider-a", secret)).Handler())
	defer server.Close()
	payload := []byte(`{"effect_id":"` + effect.ID + `","bill_id":"bill-1","external_id":"external-1","bill_digest":"sha256:` + strings.Repeat("e", 64) + `","amount_minor":120,"currency":"CNY","observed_at":"2026-08-09T10:00:00Z"}`)
	response, err := http.DefaultClient.Do(signedProviderRequest(t, server.URL+"/api/v1/providers/provider-a/tenants/tenant-1/bills", payload, secret))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		t.Fatalf("bill status=%d body=%s", response.StatusCode, body)
	}
	_ = response.Body.Close()
	replay, err := http.DefaultClient.Do(signedProviderRequest(t, server.URL+"/api/v1/providers/provider-a/tenants/tenant-1/bills", payload, secret))
	if err != nil || replay.StatusCode != http.StatusOK {
		t.Fatalf("bill replay status=%v err=%v", replay, err)
	}
	_ = replay.Body.Close()
	bills, err := store.ProviderBillRecords(t.Context(), "tenant-1", "")
	if err != nil || len(bills) != 1 || bills[0].Status != domain.ProviderBillMatched || bills[0].EffectID != effect.ID {
		t.Fatalf("bill was not matched idempotently: bills=%#v err=%v", bills, err)
	}
	conflictPayload := []byte(`{"effect_id":"` + effect.ID + `","bill_id":"bill-1","external_id":"external-1","bill_digest":"sha256:` + strings.Repeat("f", 64) + `","amount_minor":130,"currency":"CNY","observed_at":"2026-08-09T10:00:00Z"}`)
	conflict, err := http.DefaultClient.Do(signedProviderRequest(t, server.URL+"/api/v1/providers/provider-a/tenants/tenant-1/bills", conflictPayload, secret))
	if err != nil {
		t.Fatal(err)
	}
	if conflict.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(conflict.Body)
		_ = conflict.Body.Close()
		t.Fatalf("bill digest conflict status=%d body=%s", conflict.StatusCode, body)
	}
	_ = conflict.Body.Close()
}

func signedProviderRequest(t *testing.T, endpoint string, body, secret []byte) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	digest := sha256.Sum256(body)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp + "\n" + hex.EncodeToString(digest[:]) + "\n" + request.URL.Path))
	request.Header.Set("X-ContentCloud-Timestamp", timestamp)
	request.Header.Set("X-ContentCloud-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func ingressSOP() domain.SOPVersion {
	return domain.SOPVersion{ID: "sop-v1", TenantID: "tenant-1", SOPID: "sop-1", Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: "Provider ingress", Status: "published", ContentTypes: []string{domain.ContentTypeMarketingVideo}, DefaultExecutionMode: "local", Stages: []domain.StageDefinition{{ID: "sources", Name: "资料", Order: 10, OutputSchema: "contentcloud.sources/1.0", ExecutionModes: []string{"local"}}}}
}
