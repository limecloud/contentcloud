package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/channeladapter"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
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
	service := app.New(store, nil)
	session, err := service.Register(ctx, "channel-owner@example.com", "long-enough-password", "渠道负责人", "渠道租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := service.EnsureMarketingVideoDemoFixture(ctx, actor, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := service.WorkTaskDeliveries(ctx, actor, fixture.Task.Task.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("fixture delivery missing: %#v err=%v", deliveries, err)
	}
	delivery := deliveries[0]
	delivery.Status, delivery.DeliveredBy, delivery.DeliveredAt = domain.TaskDeliveryReady, "", nil
	if err := store.SaveTaskDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}

	binding, err := service.CreateChannelBinding(ctx, actor, app.CreateChannelBindingInput{ProjectID: fixture.Project.ID, Channel: "wechat_official_account", AdapterID: "manual", AccountRef: "wechat-account-1", AuthorizationSecretRef: "secret://channels/wechat-account-1", Region: "cn"}, "binding")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.PrepareChannelPublication(ctx, actor, app.PrepareChannelPublicationInput{TaskDeliveryID: delivery.ID, ChannelBindingID: binding.ID, IdempotencyKey: "wechat-publish-1", Metadata: map[string]any{"title": "预览标题"}}, "prepare")
	if err != nil || prepared.State != domain.ChannelPublicationPrepared || prepared.RequestDigest == "" {
		t.Fatalf("publication was not prepared: %#v err=%v", prepared, err)
	}
	replayed, err := service.PrepareChannelPublication(ctx, actor, app.PrepareChannelPublicationInput{TaskDeliveryID: delivery.ID, ChannelBindingID: binding.ID, IdempotencyKey: "wechat-publish-1"}, "replay")
	if err != nil || replayed.ID != prepared.ID {
		t.Fatalf("prepare idempotency failed: %#v err=%v", replayed, err)
	}

	pending, err := service.SubmitChannelPublication(ctx, actor, prepared.ID, "submit")
	if err != nil || pending.State != domain.ChannelPublicationManualActionRequired || pending.ExternalID != "" {
		t.Fatalf("manual submit must require an operator: %#v err=%v", pending, err)
	}
	storedDelivery, _ := store.TaskDelivery(ctx, actor.TenantID, delivery.ID)
	if storedDelivery.Status != domain.TaskDeliveryReady {
		t.Fatalf("manual action must not impersonate delivery: %#v", storedDelivery)
	}
	if _, err := service.RecordManualChannelReceipt(ctx, actor, pending.ID, app.RecordManualChannelReceiptInput{State: "published"}, "invalid-receipt"); err == nil {
		t.Fatal("published receipt without external ID and timestamp must fail")
	}
	publishedAt := time.Date(2026, 8, 11, 9, 30, 0, 0, time.UTC)
	published, err := service.RecordManualChannelReceipt(ctx, actor, pending.ID, app.RecordManualChannelReceiptInput{State: "published", ExternalID: "wx-article-123", ExternalURL: "https://mp.weixin.qq.com/s/example", PublishedAt: &publishedAt, SafeSummary: map[string]any{"operator": "editor", "access_token": "must-not-leak"}}, "receipt")
	if err != nil || published.State != domain.ChannelPublicationPublished || published.PublishedAt == nil {
		t.Fatalf("manual receipt did not publish: %#v err=%v", published, err)
	}
	if published.SafeSummary["access_token"] != "[redacted]" || !strings.HasPrefix(published.ResponseDigest, "sha256:") {
		t.Fatalf("receipt was not digested and redacted: %#v", published)
	}
	storedDelivery, _ = store.TaskDelivery(ctx, actor.TenantID, delivery.ID)
	if storedDelivery.Status != domain.TaskDeliveryDelivered || storedDelivery.DeliveredAt == nil {
		t.Fatalf("published receipt did not advance delivery: %#v", storedDelivery)
	}
}

func TestDouyinCommercePublicationRequiresTypedValidationLineage(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	service := app.New(store, nil)
	session, err := service.Register(ctx, "douyin-lineage@example.com", "long-enough-password", "电商负责人", "电商租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := service.EnsureMarketingVideoDemoFixture(ctx, actor, "douyin-lineage")
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := service.WorkTaskDeliveries(ctx, actor, fixture.Task.Task.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("fixture delivery missing: %#v %v", deliveries, err)
	}
	delivery := deliveries[0]
	delivery.Status = domain.TaskDeliveryReady
	delivery.DeliveredAt = nil
	delivery.DeliveredBy = ""
	if err := store.SaveTaskDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	binding, err := service.CreateChannelBinding(ctx, actor, app.CreateChannelBindingInput{ProjectID: fixture.Project.ID, Channel: "douyin", AdapterID: "manual", AccountRef: "douyin-main", AuthorizationSecretRef: "secret://channels/douyin-main"}, "binding")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.PrepareChannelPublication(ctx, actor, app.PrepareChannelPublicationInput{TaskDeliveryID: delivery.ID, ChannelBindingID: binding.ID, IdempotencyKey: "douyin-lineage-1", ContentProfileID: domain.DouyinCommerceProfileID}, "prepare")
	if err == nil {
		t.Fatal("douyin commerce profile accepted a publication without typed lineage")
	}
	if value, ok := err.(*domain.Error); !ok || value.Code != "DOUYIN_COMMERCE_PUBLICATION_REFS_REQUIRED" {
		t.Fatalf("unexpected missing lineage error: %v", err)
	}
}

func TestRemoteChannelCallbackIsDeduplicatedAndOwnsPublishedTransition(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	registry := channeladapter.NewRegistry()
	registry.Register("remote-test", callbackChannelAdapter{})
	service := app.New(store, nil, app.WithChannelAdapterRegistry(registry))
	session, err := service.Register(ctx, "channel-callback@example.com", "long-enough-password", "渠道负责人", "渠道回调租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := service.EnsureMarketingVideoDemoFixture(ctx, actor, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := service.WorkTaskDeliveries(ctx, actor, fixture.Task.Task.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("fixture delivery missing: %#v err=%v", deliveries, err)
	}
	delivery := deliveries[0]
	delivery.Status, delivery.DeliveredBy, delivery.DeliveredAt = domain.TaskDeliveryReady, "", nil
	if err := store.SaveTaskDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	binding, err := service.CreateChannelBinding(ctx, actor, app.CreateChannelBindingInput{ProjectID: fixture.Project.ID, Channel: "douyin", AdapterID: "remote-test", AccountRef: "douyin-main", AuthorizationSecretRef: "secret://channels/douyin-main"}, "")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.PrepareChannelPublication(ctx, actor, app.PrepareChannelPublicationInput{TaskDeliveryID: delivery.ID, ChannelBindingID: binding.ID, IdempotencyKey: "callback-publish-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service.SubmitChannelPublication(ctx, actor, prepared.ID, "")
	if err != nil || submitted.State != domain.ChannelPublicationSubmitted {
		t.Fatalf("remote publication did not enter submitted: %#v %v", submitted, err)
	}
	studioBefore, err := service.CustomerStudioDeliveries(ctx, actor)
	if err != nil || len(studioBefore.Publications) != 1 || studioBefore.Publications[0].Status != domain.ChannelPublicationSubmitted || studioBefore.Publications[0].PublishedAt != nil {
		t.Fatalf("Studio did not project submitted channel state: %#v %v", studioBefore, err)
	}
	publishedAt := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	input := app.ReceiveChannelCallbackInput{EventID: "event-1", PublicationID: submitted.ID, State: "published", ExternalID: submitted.ExternalID, ExternalURL: "https://douyin.example/video/remote-123", PublishedAt: &publishedAt, SafeSummary: map[string]any{"token": "redact", "result": "ok"}, PayloadDigest: "sha256:" + strings.Repeat("a", 64)}
	first, err := service.ReceiveChannelCallback(ctx, actor.TenantID, "remote-test", input, "callback")
	if err != nil || !first.Applied || first.Publication.State != domain.ChannelPublicationPublished {
		t.Fatalf("callback did not publish: %#v %v", first, err)
	}
	if first.Publication.SafeSummary["token"] != "[redacted]" {
		t.Fatalf("callback summary leaked secret: %#v", first.Publication.SafeSummary)
	}
	studioAfter, err := service.CustomerStudioDeliveries(ctx, actor)
	if err != nil || len(studioAfter.Publications) != 1 || studioAfter.Publications[0].Status != domain.ChannelPublicationPublished || studioAfter.Publications[0].PublishedAt == nil {
		t.Fatalf("Studio did not project published receipt state: %#v %v", studioAfter, err)
	}
	replayed, err := service.ReceiveChannelCallback(ctx, actor.TenantID, "remote-test", input, "callback-replay")
	if err != nil || replayed.Applied || replayed.Publication.State != domain.ChannelPublicationPublished {
		t.Fatalf("callback replay was not idempotent: %#v %v", replayed, err)
	}
	storedDelivery, err := store.TaskDelivery(ctx, actor.TenantID, delivery.ID)
	if err != nil || storedDelivery.Status != domain.TaskDeliveryDelivered {
		t.Fatalf("published callback did not complete delivery: %#v %v", storedDelivery, err)
	}
	snapshots, err := service.ApprovedSnapshots(ctx, actor, fixture.Project.ID, "content_batch")
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("fixture approved snapshot missing: %#v %v", snapshots, err)
	}
	performance, err := service.ImportChannelPerformance(ctx, actor, first.Publication.ID, app.ImportChannelPerformanceInput{ApprovedSnapshotID: snapshots[0].ID, WindowHours: 24, Metrics: map[string]float64{"views": 1200, "clicks": 32}, IssueCategory: "creative"}, "channel-performance")
	if err != nil || len(performance.Observations) != 1 || performance.Observations[0].Platform != first.Publication.Channel || performance.Observations[0].AccountAlias != first.Publication.AccountRef {
		t.Fatalf("published receipt did not import performance observation: %#v %v", performance, err)
	}
}
