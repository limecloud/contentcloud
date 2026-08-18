package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	channeladapter "github.com/limecloud/contentcloud/internal/integration/provider/channel"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"

	"github.com/limecloud/contentcloud/internal/application"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
)

type ingressChannelAdapter struct{}

func (ingressChannelAdapter) Validate(ctx context.Context, request channeladapter.Request) error {
	return channeladapter.ManualAdapter{}.Validate(ctx, request)
}
func (ingressChannelAdapter) Prepare(ctx context.Context, request channeladapter.Request) (channeladapter.Prepared, error) {
	return channeladapter.ManualAdapter{}.Prepare(ctx, request)
}
func (ingressChannelAdapter) Submit(_ context.Context, prepared channeladapter.Prepared) (channeladapter.Receipt, error) {
	return channeladapter.Receipt{State: channeladapter.StateSubmitted, ExternalID: "remote-1", RequestDigest: prepared.RequestDigest, SafeSummary: map[string]any{}}, nil
}
func (ingressChannelAdapter) Inspect(_ context.Context, receipt channeladapter.Receipt) (channeladapter.Receipt, error) {
	return receipt, nil
}
func (ingressChannelAdapter) Withdraw(_ context.Context, receipt channeladapter.Receipt, _ string) (channeladapter.Receipt, error) {
	receipt.State = channeladapter.StateWithdrawn
	return receipt, nil
}

func TestChannelCallbackIngressAuthenticatesAndDeduplicates(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	registry := channeladapter.NewRegistry()
	registry.Register("remote-ingress", ingressChannelAdapter{})
	service := application.New(application.DependenciesFrom(store), nil, application.WithChannelAdapterRegistry(registry))
	session, err := service.Identity.Register(ctx, "channel-ingress@example.com", "long-enough-password", "Ingress", "Ingress Team")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := service.Operations.EnsureMarketingVideoDemoFixture(ctx, actor, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := service.Work.WorkTaskDeliveries(ctx, actor, fixture.Task.Task.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("fixture delivery missing: %#v %v", deliveries, err)
	}
	delivery := deliveries[0]
	delivery.Status, delivery.DeliveredAt, delivery.DeliveredBy = deliverydomain.TaskDeliveryReady, nil, ""
	if err := store.SaveTaskDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	binding, err := service.Delivery.CreateChannelBinding(ctx, actor, application.CreateChannelBindingInput{ProjectID: fixture.Project.ID, Channel: "douyin", AdapterID: "remote-ingress", AccountRef: "main", AuthorizationSecretRef: "secret://douyin/main"}, "")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Delivery.PrepareChannelPublication(ctx, actor, application.PrepareChannelPublicationInput{TaskDeliveryID: delivery.ID, ChannelBindingID: binding.ID, IdempotencyKey: "ingress-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service.Delivery.SubmitChannelPublication(ctx, actor, prepared.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	body, err := json.Marshal(map[string]any{"event_id": "event-1", "publication_id": submitted.ID, "state": "published", "external_id": submitted.ExternalID, "external_url": "https://douyin.example/video/remote-1", "published_at": publishedAt, "safe_summary": map[string]any{"status": "published"}})
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("channel-ingress-secret")
	server := httptest.NewServer(httpapi.New(service, nil, false, "", httpapi.WithChannelCallbackSecret(actor.TenantID, "remote-ingress", secret)).Handler())
	defer server.Close()
	endpoint := server.URL + "/api/v1/channels/remote-ingress/tenants/" + actor.TenantID + "/callbacks"
	response, err := http.DefaultClient.Do(signedProviderRequest(t, endpoint, body, secret))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("channel callback status=%d body=%s", response.StatusCode, data)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Applied bool `json:"applied"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || !envelope.OK || !envelope.Data.Applied {
		t.Fatalf("unexpected callback envelope: %#v %v", envelope, err)
	}
	replay, err := http.DefaultClient.Do(signedProviderRequest(t, endpoint, body, secret))
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Body.Close()
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("callback replay status=%d", replay.StatusCode)
	}
	var replayEnvelope struct {
		Data struct {
			Applied bool `json:"applied"`
		} `json:"data"`
	}
	if err := json.NewDecoder(replay.Body).Decode(&replayEnvelope); err != nil || replayEnvelope.Data.Applied {
		t.Fatalf("callback replay was applied twice: %#v %v", replayEnvelope, err)
	}
	bad, err := http.DefaultClient.Do(signedProviderRequest(t, endpoint, body, []byte("wrong")))
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad channel signature status=%d", bad.StatusCode)
	}
}
