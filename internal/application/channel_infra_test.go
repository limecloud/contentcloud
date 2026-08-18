package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/application"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	channeladapter "github.com/limecloud/contentcloud/internal/integration/provider/channel"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/work"
)

type callbackChannelAdapter struct{}

func (callbackChannelAdapter) Validate(ctx context.Context, request channeladapter.Request) error {
	return channeladapter.ManualAdapter{}.Validate(ctx, request)
}

func (callbackChannelAdapter) Prepare(ctx context.Context, request channeladapter.Request) (channeladapter.Prepared, error) {
	return channeladapter.ManualAdapter{}.Prepare(ctx, request)
}

func (callbackChannelAdapter) Submit(_ context.Context, prepared channeladapter.Prepared) (channeladapter.Receipt, error) {
	return channeladapter.Receipt{State: channeladapter.StateSubmitted, ExternalID: "remote-123", RequestDigest: prepared.RequestDigest, SafeSummary: map[string]any{}}, nil
}

func (callbackChannelAdapter) Inspect(_ context.Context, receipt channeladapter.Receipt) (channeladapter.Receipt, error) {
	return receipt, nil
}

func (callbackChannelAdapter) Withdraw(_ context.Context, receipt channeladapter.Receipt, _ string) (channeladapter.Receipt, error) {
	receipt.State = channeladapter.StateWithdrawn
	return receipt, nil
}

func TestManualChannelPublicationRequiresExternalReceipt(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), nil)
	session, err := service.Identity.Register(ctx, "channel-owner@example.com", "long-enough-password", "渠道负责人", "渠道租户")
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
		t.Fatalf("fixture delivery missing: %#v err=%v", deliveries, err)
	}
	delivery := deliveries[0]
	delivery.Status, delivery.DeliveredBy, delivery.DeliveredAt = deliverydomain.TaskDeliveryReady, "", nil
	if err := store.SaveTaskDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}

	binding, err := service.Delivery.CreateChannelBinding(ctx, actor, application.CreateChannelBindingInput{ProjectID: fixture.Project.ID, Channel: "wechat_official_account", AdapterID: "manual", AccountRef: "wechat-account-1", AuthorizationSecretRef: "secret://channels/wechat-account-1", Region: "cn"}, "binding")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Delivery.PrepareChannelPublication(ctx, actor, application.PrepareChannelPublicationInput{TaskDeliveryID: delivery.ID, ChannelBindingID: binding.ID, IdempotencyKey: "wechat-publish-1", Metadata: map[string]any{"title": "预览标题"}}, "prepare")
	if err != nil || prepared.State != deliverydomain.ChannelPublicationPrepared || prepared.RequestDigest == "" {
		t.Fatalf("publication was not prepared: %#v err=%v", prepared, err)
	}
	replayed, err := service.Delivery.PrepareChannelPublication(ctx, actor, application.PrepareChannelPublicationInput{TaskDeliveryID: delivery.ID, ChannelBindingID: binding.ID, IdempotencyKey: "wechat-publish-1"}, "replay")
	if err != nil || replayed.ID != prepared.ID {
		t.Fatalf("prepare idempotency failed: %#v err=%v", replayed, err)
	}

	pending, err := service.Delivery.SubmitChannelPublication(ctx, actor, prepared.ID, "submit")
	if err != nil || pending.State != deliverydomain.ChannelPublicationManualActionRequired || pending.ExternalID != "" {
		t.Fatalf("manual submit must require an operator: %#v err=%v", pending, err)
	}
	storedDelivery, _ := store.TaskDelivery(ctx, actor.TenantID, delivery.ID)
	if storedDelivery.Status != deliverydomain.TaskDeliveryReady {
		t.Fatalf("manual action must not impersonate delivery: %#v", storedDelivery)
	}
	if _, err := service.Delivery.RecordManualChannelReceipt(ctx, actor, pending.ID, application.RecordManualChannelReceiptInput{State: "published"}, "invalid-receipt"); err == nil {
		t.Fatal("published receipt without external ID and timestamp must fail")
	}
	publishedAt := time.Date(2026, 8, 11, 9, 30, 0, 0, time.UTC)
	published, err := service.Delivery.RecordManualChannelReceipt(ctx, actor, pending.ID, application.RecordManualChannelReceiptInput{State: "published", ExternalID: "wx-article-123", ExternalURL: "https://mp.weixin.qq.com/s/example", PublishedAt: &publishedAt, SafeSummary: map[string]any{"operator": "editor", "access_token": "must-not-leak"}}, "receipt")
	if err != nil || published.State != deliverydomain.ChannelPublicationPublished || published.PublishedAt == nil {
		t.Fatalf("manual receipt did not publish: %#v err=%v", published, err)
	}
	if published.SafeSummary["access_token"] != "[redacted]" || !strings.HasPrefix(published.ResponseDigest, "sha256:") {
		t.Fatalf("receipt was not digested and redacted: %#v", published)
	}
	storedDelivery, _ = store.TaskDelivery(ctx, actor.TenantID, delivery.ID)
	if storedDelivery.Status != deliverydomain.TaskDeliveryDelivered || storedDelivery.DeliveredAt == nil {
		t.Fatalf("published receipt did not advance delivery: %#v", storedDelivery)
	}
}

func TestDouyinCommercePublicationRequiresTypedValidationLineage(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), nil)
	session, err := service.Identity.Register(ctx, "douyin-lineage@example.com", "long-enough-password", "电商负责人", "电商租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := service.Operations.EnsureMarketingVideoDemoFixture(ctx, actor, "douyin-lineage")
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := service.Work.WorkTaskDeliveries(ctx, actor, fixture.Task.Task.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("fixture delivery missing: %#v %v", deliveries, err)
	}
	delivery := deliveries[0]
	delivery.Status = deliverydomain.TaskDeliveryReady
	delivery.DeliveredAt = nil
	delivery.DeliveredBy = ""
	if err := store.SaveTaskDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	binding, err := service.Delivery.CreateChannelBinding(ctx, actor, application.CreateChannelBindingInput{ProjectID: fixture.Project.ID, Channel: "douyin", AdapterID: "manual", AccountRef: "douyin-main", AuthorizationSecretRef: "secret://channels/douyin-main"}, "binding")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Delivery.PrepareChannelPublication(ctx, actor, application.PrepareChannelPublicationInput{TaskDeliveryID: delivery.ID, ChannelBindingID: binding.ID, IdempotencyKey: "douyin-lineage-1", ContentProfileID: work.DouyinCommerceProfileID}, "prepare")
	if err == nil {
		t.Fatal("douyin commerce profile accepted a publication without typed lineage")
	}
	if value, ok := err.(*fault.Error); !ok || value.Code != "DOUYIN_COMMERCE_PUBLICATION_REFS_REQUIRED" {
		t.Fatalf("unexpected missing lineage error: %v", err)
	}
}

func TestRemoteChannelCallbackIsDeduplicatedAndOwnsPublishedTransition(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	registry := channeladapter.NewRegistry()
	registry.Register("remote-test", callbackChannelAdapter{})
	service := application.New(application.DependenciesFrom(store), nil, application.WithChannelAdapterRegistry(registry))
	session, err := service.Identity.Register(ctx, "channel-callback@example.com", "long-enough-password", "渠道负责人", "渠道回调租户")
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
		t.Fatalf("fixture delivery missing: %#v err=%v", deliveries, err)
	}
	delivery := deliveries[0]
	delivery.Status, delivery.DeliveredBy, delivery.DeliveredAt = deliverydomain.TaskDeliveryReady, "", nil
	if err := store.SaveTaskDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	binding, err := service.Delivery.CreateChannelBinding(ctx, actor, application.CreateChannelBindingInput{ProjectID: fixture.Project.ID, Channel: "douyin", AdapterID: "remote-test", AccountRef: "douyin-main", AuthorizationSecretRef: "secret://channels/douyin-main"}, "")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Delivery.PrepareChannelPublication(ctx, actor, application.PrepareChannelPublicationInput{TaskDeliveryID: delivery.ID, ChannelBindingID: binding.ID, IdempotencyKey: "callback-publish-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service.Delivery.SubmitChannelPublication(ctx, actor, prepared.ID, "")
	if err != nil || submitted.State != deliverydomain.ChannelPublicationSubmitted {
		t.Fatalf("remote publication did not enter submitted: %#v %v", submitted, err)
	}
	studioBefore, err := service.Work.CustomerStudioDeliveries(ctx, actor)
	if err != nil || len(studioBefore.Publications) != 1 || studioBefore.Publications[0].Status != deliverydomain.ChannelPublicationSubmitted || studioBefore.Publications[0].PublishedAt != nil {
		t.Fatalf("Studio did not project submitted channel state: %#v %v", studioBefore, err)
	}
	publishedAt := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	input := application.ReceiveChannelCallbackInput{EventID: "event-1", PublicationID: submitted.ID, State: "published", ExternalID: submitted.ExternalID, ExternalURL: "https://douyin.example/video/remote-123", PublishedAt: &publishedAt, SafeSummary: map[string]any{"token": "redact", "result": "ok"}, PayloadDigest: "sha256:" + strings.Repeat("a", 64)}
	first, err := service.Delivery.ReceiveChannelCallback(ctx, actor.TenantID, "remote-test", input, "callback")
	if err != nil || !first.Applied || first.Publication.State != deliverydomain.ChannelPublicationPublished {
		t.Fatalf("callback did not publish: %#v %v", first, err)
	}
	if first.Publication.SafeSummary["token"] != "[redacted]" {
		t.Fatalf("callback summary leaked secret: %#v", first.Publication.SafeSummary)
	}
	studioAfter, err := service.Work.CustomerStudioDeliveries(ctx, actor)
	if err != nil || len(studioAfter.Publications) != 1 || studioAfter.Publications[0].Status != deliverydomain.ChannelPublicationPublished || studioAfter.Publications[0].PublishedAt == nil {
		t.Fatalf("Studio did not project published receipt state: %#v %v", studioAfter, err)
	}
	replayed, err := service.Delivery.ReceiveChannelCallback(ctx, actor.TenantID, "remote-test", input, "callback-replay")
	if err != nil || replayed.Applied || replayed.Publication.State != deliverydomain.ChannelPublicationPublished {
		t.Fatalf("callback replay was not idempotent: %#v %v", replayed, err)
	}
	storedDelivery, err := store.TaskDelivery(ctx, actor.TenantID, delivery.ID)
	if err != nil || storedDelivery.Status != deliverydomain.TaskDeliveryDelivered {
		t.Fatalf("published callback did not complete delivery: %#v %v", storedDelivery, err)
	}
	snapshots, err := service.Review.ApprovedSnapshots(ctx, actor, fixture.Project.ID, "content_batch")
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("fixture approved snapshot missing: %#v %v", snapshots, err)
	}
	performance, err := service.Delivery.ImportChannelPerformance(ctx, actor, first.Publication.ID, application.ImportChannelPerformanceInput{ApprovedSnapshotID: snapshots[0].ID, WindowHours: 24, Metrics: map[string]float64{"views": 1200, "clicks": 32}, IssueCategory: "creative"}, "channel-performance")
	if err != nil || len(performance.Observations) != 1 || performance.Observations[0].Platform != first.Publication.Channel || performance.Observations[0].AccountAlias != first.Publication.AccountRef {
		t.Fatalf("published receipt did not import performance observation: %#v %v", performance, err)
	}
}
