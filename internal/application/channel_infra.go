package application

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	channeladapter "github.com/limecloud/contentcloud/internal/integration/provider/channel"
	"github.com/limecloud/contentcloud/internal/work"
)

type CreateChannelBindingInput struct {
	ProjectID              string `json:"project_id"`
	Channel                string `json:"channel"`
	AdapterID              string `json:"adapter_id"`
	AccountRef             string `json:"account_ref"`
	AuthorizationSecretRef string `json:"authorization_secret_ref"`
	Region                 string `json:"region"`
}

type PrepareChannelPublicationInput struct {
	TaskDeliveryID   string                              `json:"task_delivery_id"`
	ChannelBindingID string                              `json:"channel_binding_id"`
	IdempotencyKey   string                              `json:"idempotency_key"`
	ContentProfileID string                              `json:"content_profile_id,omitempty"`
	DouyinCommerce   *work.DouyinCommercePublicationRefs `json:"douyin_commerce,omitempty"`
	ScheduledAt      *time.Time                          `json:"scheduled_at,omitempty"`
	Metadata         map[string]any                      `json:"metadata"`
}

type RecordManualChannelReceiptInput struct {
	State       string         `json:"state"`
	ExternalID  string         `json:"external_id"`
	ExternalURL string         `json:"external_url"`
	PublishedAt *time.Time     `json:"published_at,omitempty"`
	ErrorCode   string         `json:"error_code"`
	SafeSummary map[string]any `json:"safe_summary"`
}

type ReceiveChannelCallbackInput struct {
	EventID       string         `json:"event_id"`
	PublicationID string         `json:"publication_id"`
	State         string         `json:"state"`
	ExternalID    string         `json:"external_id"`
	ExternalURL   string         `json:"external_url"`
	PublishedAt   *time.Time     `json:"published_at,omitempty"`
	ObservedAt    *time.Time     `json:"observed_at,omitempty"`
	CostMinor     int64          `json:"cost_minor,omitempty"`
	Currency      string         `json:"currency,omitempty"`
	ErrorCode     string         `json:"error_code,omitempty"`
	SafeSummary   map[string]any `json:"safe_summary"`
	PayloadDigest string         `json:"-"`
}

type ReceiveChannelCallbackResult struct {
	Applied     bool                                  `json:"applied"`
	Receipt     deliverydomain.ChannelCallbackReceipt `json:"receipt"`
	Publication deliverydomain.ChannelPublication     `json:"publication"`
}

type ChannelReconcileResult struct {
	Inspected int                                 `json:"inspected"`
	Updated   int                                 `json:"updated"`
	Failed    int                                 `json:"failed"`
	Items     []deliverydomain.ChannelPublication `json:"items"`
}

type ImportChannelPerformanceInput struct {
	ApprovedSnapshotID string             `json:"approved_snapshot_id"`
	WindowHours        int                `json:"window_hours"`
	SampleStatus       string             `json:"sample_status"`
	Metrics            map[string]float64 `json:"metrics"`
	Currency           string             `json:"currency,omitempty"`
	Spend              float64            `json:"spend,omitempty"`
	GMV                float64            `json:"gmv,omitempty"`
	IssueCategory      string             `json:"issue_category"`
	Notes              string             `json:"notes"`
}

func (s *DeliveryService) ChannelAdapterIDs() []string {
	return s.channelAdapters.IDs()
}

func (s *DeliveryService) CreateChannelBinding(ctx context.Context, actor Actor, input CreateChannelBindingInput, requestID string) (deliverydomain.ChannelBinding, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil {
		return deliverydomain.ChannelBinding{}, err
	}
	if _, err := s.app.Identity.projectForWrite(ctx, actor, input.ProjectID); err != nil {
		return deliverydomain.ChannelBinding{}, err
	}
	adapterID := defaultString(strings.ToLower(strings.TrimSpace(input.AdapterID)), channeladapter.ManualAdapterID)
	if _, err := s.channelAdapters.Resolve(adapterID); err != nil {
		return deliverydomain.ChannelBinding{}, err
	}
	now := s.now().UTC()
	value := deliverydomain.ChannelBinding{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: input.ProjectID, Channel: strings.ToLower(strings.TrimSpace(input.Channel)), AdapterID: adapterID, AccountRef: strings.TrimSpace(input.AccountRef), AuthorizationSecretRef: strings.TrimSpace(input.AuthorizationSecretRef), Region: defaultString(strings.TrimSpace(input.Region), "global"), Status: deliverydomain.ChannelBindingActive, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := value.Validate(); err != nil {
		return value, err
	}
	if err := s.delivery.CreateChannelBinding(ctx, value); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.binding_created", "channel_binding", value.ID, requestID, map[string]any{"channel": value.Channel, "adapter_id": value.AdapterID, "account_ref": value.AccountRef})
	return value, nil
}

func (s *DeliveryService) ChannelBindings(ctx context.Context, actor Actor, projectID string) ([]deliverydomain.ChannelBinding, error) {
	if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.delivery.ChannelBindings(ctx, actor.TenantID, projectID)
}

func (s *DeliveryService) PrepareChannelPublication(ctx context.Context, actor Actor, input PrepareChannelPublicationInput, requestID string) (deliverydomain.ChannelPublication, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return deliverydomain.ChannelPublication{}, err
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		return deliverydomain.ChannelPublication{}, fault.Invalid("CHANNEL_IDEMPOTENCY_KEY_REQUIRED", "渠道发布必须提供幂等键")
	}
	if existing, err := s.delivery.ChannelPublicationByIdempotencyKey(ctx, actor.TenantID, key); err == nil {
		return existing, nil
	} else if !fault.IsNotFound(err) {
		return deliverydomain.ChannelPublication{}, err
	}
	delivery, err := s.delivery.TaskDelivery(ctx, actor.TenantID, input.TaskDeliveryID)
	if err != nil {
		return deliverydomain.ChannelPublication{}, err
	}
	if delivery.Status != deliverydomain.TaskDeliveryReady || delivery.IntegrityStatus != "complete" || delivery.DeliveryPackageID == "" {
		return deliverydomain.ChannelPublication{}, fault.Policy("CHANNEL_DELIVERY_NOT_READY", "渠道发布必须引用完整且 ready 的任务交付", "先构建并校验交付包")
	}
	binding, err := s.delivery.ChannelBinding(ctx, actor.TenantID, input.ChannelBindingID)
	if err != nil {
		return deliverydomain.ChannelPublication{}, err
	}
	if binding.ProjectID != delivery.ProjectID || binding.Status != deliverydomain.ChannelBindingActive {
		return deliverydomain.ChannelPublication{}, fault.Policy("CHANNEL_BINDING_NOT_ACTIVE", "渠道绑定不属于该项目或已停用", "选择当前项目的有效渠道绑定")
	}
	adapter, err := s.channelAdapters.Resolve(binding.AdapterID)
	if err != nil {
		return deliverydomain.ChannelPublication{}, err
	}
	metadata := cloneMetadata(input.Metadata)
	if input.ContentProfileID == work.DouyinCommerceProfileID {
		validated, validationErr := s.app.Review.validateDouyinCommercePublication(ctx, actor, delivery, binding, input)
		if validationErr != nil {
			return deliverydomain.ChannelPublication{}, validationErr
		}
		metadata = validated
	} else if input.DouyinCommerce != nil {
		return deliverydomain.ChannelPublication{}, fault.Invalid("CHANNEL_PROFILE_REFS_MISMATCH", "只有 douyin-commerce-video Profile 可以提供抖音电商类型化引用")
	}
	if input.ContentProfileID != "" {
		metadata["content_profile_id"] = input.ContentProfileID
	}
	request := channeladapter.Request{TenantID: actor.TenantID, ProjectID: delivery.ProjectID, DeliveryPackageID: delivery.DeliveryPackageID, DeliveryDigest: delivery.DeliveryDigest, Channel: binding.Channel, AccountRef: binding.AccountRef, AuthorizationRef: binding.AuthorizationSecretRef, IdempotencyKey: key, ScheduledAt: input.ScheduledAt, Metadata: metadata}
	prepared, err := adapter.Prepare(ctx, request)
	if err != nil {
		return deliverydomain.ChannelPublication{}, err
	}
	now := s.now().UTC()
	value := deliverydomain.ChannelPublication{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: delivery.ProjectID, TaskID: delivery.TaskID, TaskDeliveryID: delivery.ID, DeliveryPackageID: delivery.DeliveryPackageID, ChannelBindingID: binding.ID, Channel: binding.Channel, AccountRef: binding.AccountRef, State: deliverydomain.ChannelPublicationPrepared, IdempotencyKey: key, DeliveryDigest: delivery.DeliveryDigest, RequestDigest: prepared.RequestDigest, Checklist: prepared.Checklist, Preview: prepared.Preview, Metadata: request.Metadata, SafeSummary: map[string]any{}, ScheduledAt: input.ScheduledAt, ObservedAt: now, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	value.NormalizeCollections()
	if err := s.delivery.CreateChannelPublication(ctx, value); err != nil {
		if existing, replayErr := s.delivery.ChannelPublicationByIdempotencyKey(ctx, actor.TenantID, key); replayErr == nil {
			return existing, nil
		}
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.publication_prepared", "channel_publication", value.ID, requestID, map[string]any{"task_delivery_id": value.TaskDeliveryID, "channel": value.Channel, "request_digest": value.RequestDigest})
	return value, nil
}

func (s *DeliveryService) SubmitChannelPublication(ctx context.Context, actor Actor, id, requestID string) (deliverydomain.ChannelPublication, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return deliverydomain.ChannelPublication{}, err
	}
	value, binding, adapter, prepared, err := s.channelPublicationContext(ctx, actor, id)
	if err != nil {
		return value, err
	}
	if value.State != deliverydomain.ChannelPublicationPrepared {
		return value, fault.Conflict("CHANNEL_PUBLICATION_NOT_PREPARED", "只有 prepared 发布可以提交")
	}
	receipt, submitErr := adapter.Submit(ctx, prepared)
	now := s.now().UTC()
	value.SubmittedAt = &now
	applyChannelReceipt(&value, receipt, now)
	if submitErr != nil && value.State == deliverydomain.ChannelPublicationPrepared {
		value.State = deliverydomain.ChannelPublicationUnknown
		value.ErrorCode = "CHANNEL_SUBMIT_UNKNOWN"
	}
	if err := s.delivery.SaveChannelPublication(ctx, value); err != nil {
		return value, err
	}
	if err := s.completePublishedDelivery(ctx, actor, &value, now); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.publication_submitted", "channel_publication", value.ID, requestID, map[string]any{"adapter_id": binding.AdapterID, "state": value.State, "external_id": value.ExternalID})
	return value, submitErr
}

func (s *DeliveryService) InspectChannelPublication(ctx context.Context, actor Actor, id, requestID string) (deliverydomain.ChannelPublication, error) {
	value, _, adapter, _, err := s.channelPublicationContext(ctx, actor, id)
	if err != nil {
		return value, err
	}
	receipt, inspectErr := adapter.Inspect(ctx, publicationReceipt(value))
	now := s.now().UTC()
	applyChannelReceipt(&value, receipt, now)
	if inspectErr != nil {
		value.State = deliverydomain.ChannelPublicationUnknown
		value.ErrorCode = "CHANNEL_INSPECT_UNKNOWN"
	}
	if err := s.delivery.SaveChannelPublication(ctx, value); err != nil {
		return value, err
	}
	if err := s.completePublishedDelivery(ctx, actor, &value, now); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.publication_inspected", "channel_publication", value.ID, requestID, map[string]any{"state": value.State})
	return value, inspectErr
}

func (s *DeliveryService) WithdrawChannelPublication(ctx context.Context, actor Actor, id, reason, requestID string) (deliverydomain.ChannelPublication, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil {
		return deliverydomain.ChannelPublication{}, err
	}
	value, _, adapter, _, err := s.channelPublicationContext(ctx, actor, id)
	if err != nil {
		return value, err
	}
	if value.State != deliverydomain.ChannelPublicationPublished && value.State != deliverydomain.ChannelPublicationSubmitted && value.State != deliverydomain.ChannelPublicationUnknown {
		return value, fault.Conflict("CHANNEL_PUBLICATION_NOT_WITHDRAWABLE", "当前渠道发布状态不能撤回")
	}
	receipt, err := adapter.Withdraw(ctx, publicationReceipt(value), strings.TrimSpace(reason))
	if err != nil {
		return value, err
	}
	now := s.now().UTC()
	applyChannelReceipt(&value, receipt, now)
	if err := s.delivery.SaveChannelPublication(ctx, value); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.publication_withdrawn", "channel_publication", value.ID, requestID, map[string]any{"external_id": value.ExternalID, "reason": strings.TrimSpace(reason)})
	return value, nil
}

func (s *DeliveryService) RecordManualChannelReceipt(ctx context.Context, actor Actor, id string, input RecordManualChannelReceiptInput, requestID string) (deliverydomain.ChannelPublication, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return deliverydomain.ChannelPublication{}, err
	}
	value, binding, _, _, err := s.channelPublicationContext(ctx, actor, id)
	if err != nil {
		return value, err
	}
	if binding.AdapterID != channeladapter.ManualAdapterID || value.State != deliverydomain.ChannelPublicationManualActionRequired {
		return value, fault.Conflict("CHANNEL_MANUAL_RECEIPT_NOT_ALLOWED", "当前发布不处于人工回执待记录状态")
	}
	now := s.now().UTC()
	state := strings.ToLower(strings.TrimSpace(input.State))
	if state != deliverydomain.ChannelPublicationPublished && state != deliverydomain.ChannelPublicationFailed && state != deliverydomain.ChannelPublicationWithdrawn {
		return value, fault.Invalid("CHANNEL_MANUAL_RECEIPT_STATE_INVALID", "人工回执状态必须是 published、failed 或 withdrawn")
	}
	if state == deliverydomain.ChannelPublicationPublished && (strings.TrimSpace(input.ExternalID) == "" || input.PublishedAt == nil) {
		return value, fault.Invalid("CHANNEL_MANUAL_RECEIPT_INCOMPLETE", "人工发布成功必须记录外部 ID 和发布时间")
	}
	responseDigest, err := stablehash.Sum(input)
	if err != nil {
		return value, err
	}
	value.State, value.ExternalID, value.ExternalURL = state, strings.TrimSpace(input.ExternalID), strings.TrimSpace(input.ExternalURL)
	value.ResponseDigest, value.ErrorCode, value.PublishedAt = "sha256:"+responseDigest, strings.TrimSpace(input.ErrorCode), input.PublishedAt
	value.SafeSummary, value.ObservedAt, value.UpdatedAt = redactSafeSummary(input.SafeSummary), now, now
	if err := s.delivery.SaveChannelPublication(ctx, value); err != nil {
		return value, err
	}
	if err := s.completePublishedDelivery(ctx, actor, &value, now); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.manual_receipt_recorded", "channel_publication", value.ID, requestID, map[string]any{"state": value.State, "external_id": value.ExternalID})
	return value, nil
}

func (s *DeliveryService) ChannelPublications(ctx context.Context, actor Actor, taskID string) ([]deliverydomain.ChannelPublication, error) {
	if taskID != "" {
		if _, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID); err != nil {
			return nil, err
		}
	}
	return s.delivery.ChannelPublications(ctx, actor.TenantID, taskID)
}

func (s *DeliveryService) ReceiveChannelCallback(ctx context.Context, tenantID, adapterID string, input ReceiveChannelCallbackInput, requestID string) (ReceiveChannelCallbackResult, error) {
	result := ReceiveChannelCallbackResult{}
	tenantID, adapterID = strings.TrimSpace(tenantID), strings.ToLower(strings.TrimSpace(adapterID))
	input.EventID, input.PublicationID = strings.TrimSpace(input.EventID), strings.TrimSpace(input.PublicationID)
	if tenantID == "" || adapterID == "" || input.EventID == "" || input.PublicationID == "" {
		return result, fault.Invalid("CHANNEL_CALLBACK_SCOPE_INVALID", "渠道 Callback 缺少租户、Adapter、事件或发布 ID")
	}
	value, err := s.delivery.ChannelPublication(ctx, tenantID, input.PublicationID)
	if err != nil {
		return result, err
	}
	binding, err := s.delivery.ChannelBinding(ctx, tenantID, value.ChannelBindingID)
	if err != nil {
		return result, err
	}
	if binding.AdapterID != adapterID || binding.Status != deliverydomain.ChannelBindingActive || adapterID == channeladapter.ManualAdapterID {
		return result, fault.Policy("CHANNEL_CALLBACK_UNBOUND", "渠道 Callback 与有效的远程 Adapter 绑定不匹配", "检查租户、Adapter 和渠道绑定")
	}
	nextState := strings.ToLower(strings.TrimSpace(input.State))
	if !validChannelCallbackTransition(value.State, nextState) {
		return result, fault.Conflict("CHANNEL_CALLBACK_TRANSITION_INVALID", "渠道 Callback 不能执行当前状态迁移")
	}
	observedAt := s.now().UTC()
	if input.ObservedAt != nil {
		observedAt = input.ObservedAt.UTC()
	}
	if nextState == deliverydomain.ChannelPublicationPublished && (strings.TrimSpace(input.ExternalID) == "" || input.PublishedAt == nil) {
		return result, fault.Invalid("CHANNEL_CALLBACK_PUBLISHED_INCOMPLETE", "published Callback 必须包含外部 ID 和发布时间")
	}
	responseDigest, err := stablehash.Sum(input)
	if err != nil {
		return result, err
	}
	now := s.now().UTC()
	value.State = nextState
	value.ExternalID, value.ExternalURL = strings.TrimSpace(input.ExternalID), strings.TrimSpace(input.ExternalURL)
	value.ResponseDigest, value.ErrorCode = "sha256:"+responseDigest, strings.TrimSpace(input.ErrorCode)
	value.CostMinor, value.Currency, value.PublishedAt = input.CostMinor, strings.ToUpper(strings.TrimSpace(input.Currency)), input.PublishedAt
	value.SafeSummary, value.ObservedAt, value.UpdatedAt = redactSafeSummary(input.SafeSummary), observedAt, now
	receipt := deliverydomain.ChannelCallbackReceipt{ID: idgen.New(), TenantID: tenantID, PublicationID: value.ID, AdapterID: adapterID, EventID: input.EventID, PayloadDigest: input.PayloadDigest, State: nextState, SafeSummary: value.SafeSummary, ObservedAt: observedAt, ReceivedAt: now}
	if err := receipt.Validate(); err != nil {
		return result, err
	}
	applied, err := s.delivery.ApplyChannelCallback(ctx, value, receipt)
	if err != nil {
		return result, err
	}
	if !applied {
		value, err = s.delivery.ChannelPublication(ctx, tenantID, input.PublicationID)
		if err != nil {
			return result, err
		}
	}
	actor := Actor{UserID: "channel:" + adapterID, TenantID: tenantID, Role: "project_manager", Type: "channel"}
	if err := s.completePublishedDelivery(ctx, actor, &value, now); err != nil {
		return result, err
	}
	if applied {
		s.audit(ctx, actor, value.ProjectID, "channel.callback_received", "channel_publication", value.ID, requestID, map[string]any{"adapter_id": adapterID, "event_id": input.EventID, "state": value.State, "payload_digest": input.PayloadDigest})
	}
	return ReceiveChannelCallbackResult{Applied: applied, Receipt: receipt, Publication: value}, nil
}

func validChannelCallbackTransition(current, next string) bool {
	switch current {
	case deliverydomain.ChannelPublicationSubmitted, deliverydomain.ChannelPublicationUnknown:
		return next == deliverydomain.ChannelPublicationSubmitted || next == deliverydomain.ChannelPublicationPublished || next == deliverydomain.ChannelPublicationFailed || next == deliverydomain.ChannelPublicationUnknown || next == deliverydomain.ChannelPublicationWithdrawn
	case deliverydomain.ChannelPublicationPublished:
		return next == deliverydomain.ChannelPublicationPublished || next == deliverydomain.ChannelPublicationWithdrawn
	case deliverydomain.ChannelPublicationFailed:
		return next == deliverydomain.ChannelPublicationFailed
	case deliverydomain.ChannelPublicationWithdrawn:
		return next == deliverydomain.ChannelPublicationWithdrawn
	default:
		return false
	}
}

func (s *DeliveryService) ReconcileChannelPublications(ctx context.Context, actor Actor, limit int, requestID string) (ChannelReconcileResult, error) {
	result := ChannelReconcileResult{Items: []deliverydomain.ChannelPublication{}}
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil {
		return result, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return result, fault.Invalid("CHANNEL_RECONCILE_LIMIT_INVALID", "单次渠道对账最多处理 1000 条")
	}
	values, err := s.delivery.ChannelPublications(ctx, actor.TenantID, "")
	if err != nil {
		return result, err
	}
	for _, value := range values {
		if result.Inspected >= limit {
			break
		}
		if value.State != deliverydomain.ChannelPublicationSubmitted && value.State != deliverydomain.ChannelPublicationUnknown {
			continue
		}
		result.Inspected++
		updated, inspectErr := s.InspectChannelPublication(ctx, actor, value.ID, requestID)
		result.Items = append(result.Items, updated)
		if inspectErr != nil {
			result.Failed++
			continue
		}
		if updated.State != value.State || updated.ResponseDigest != value.ResponseDigest {
			result.Updated++
		}
	}
	return result, nil
}

func (s *DeliveryService) ImportChannelPerformance(ctx context.Context, actor Actor, publicationID string, input ImportChannelPerformanceInput, requestID string) (ImportPerformanceResult, error) {
	value, err := s.delivery.ChannelPublication(ctx, actor.TenantID, publicationID)
	if err != nil {
		return ImportPerformanceResult{}, err
	}
	if value.State != deliverydomain.ChannelPublicationPublished || value.PublishedAt == nil || value.ExternalID == "" {
		return ImportPerformanceResult{}, fault.Policy("CHANNEL_PERFORMANCE_NOT_PUBLISHED", "只有已取得 published 回执的渠道内容可以导入指标", "先完成渠道发布回执")
	}
	sampleStatus := strings.TrimSpace(input.SampleStatus)
	if sampleStatus == "" {
		sampleStatus = "insufficient_sample"
	}
	return s.app.Performance.ImportPerformanceObservations(ctx, actor, ImportPerformanceInput{ProjectID: value.ProjectID, SourceName: "channel-publication:" + value.ID, SourceFormat: "json", Observations: []CreateObservationInput{{ApprovedSnapshotID: strings.TrimSpace(input.ApprovedSnapshotID), Platform: value.Channel, AccountAlias: value.AccountRef, PublishedAt: value.PublishedAt.UTC(), WindowHours: input.WindowHours, SampleStatus: sampleStatus, Metrics: input.Metrics, Currency: input.Currency, Spend: input.Spend, GMV: input.GMV, IssueCategory: input.IssueCategory, Notes: input.Notes}}}, requestID)
}

func (s *DeliveryService) channelPublicationContext(ctx context.Context, actor Actor, id string) (deliverydomain.ChannelPublication, deliverydomain.ChannelBinding, channeladapter.Adapter, channeladapter.Prepared, error) {
	value, err := s.delivery.ChannelPublication(ctx, actor.TenantID, id)
	if err != nil {
		return value, deliverydomain.ChannelBinding{}, nil, channeladapter.Prepared{}, err
	}
	binding, err := s.delivery.ChannelBinding(ctx, actor.TenantID, value.ChannelBindingID)
	if err != nil {
		return value, binding, nil, channeladapter.Prepared{}, err
	}
	adapter, err := s.channelAdapters.Resolve(binding.AdapterID)
	if err != nil {
		return value, binding, nil, channeladapter.Prepared{}, err
	}
	request := channeladapter.Request{TenantID: value.TenantID, ProjectID: value.ProjectID, DeliveryPackageID: value.DeliveryPackageID, DeliveryDigest: value.DeliveryDigest, Channel: value.Channel, AccountRef: value.AccountRef, AuthorizationRef: binding.AuthorizationSecretRef, IdempotencyKey: value.IdempotencyKey, ScheduledAt: value.ScheduledAt, Metadata: value.Metadata}
	prepared := channeladapter.Prepared{Request: request, RequestDigest: value.RequestDigest, Checklist: value.Checklist, Preview: value.Preview, PreparedAt: value.CreatedAt}
	return value, binding, adapter, prepared, nil
}

func applyChannelReceipt(value *deliverydomain.ChannelPublication, receipt channeladapter.Receipt, now time.Time) {
	if receipt.State != "" {
		value.State = receipt.State
	}
	value.ExternalID, value.ExternalURL = receipt.ExternalID, receipt.ExternalURL
	value.ResponseDigest, value.ErrorCode = receipt.ResponseDigest, receipt.ErrorCode
	value.CostMinor, value.Currency, value.PublishedAt = receipt.CostMinor, receipt.Currency, receipt.PublishedAt
	value.SafeSummary, value.ObservedAt, value.UpdatedAt = redactSafeSummary(receipt.SafeSummary), now, now
}

func publicationReceipt(value deliverydomain.ChannelPublication) channeladapter.Receipt {
	return channeladapter.Receipt{Channel: value.Channel, AccountRef: value.AccountRef, State: value.State, ExternalID: value.ExternalID, ExternalURL: value.ExternalURL, RequestDigest: value.RequestDigest, ResponseDigest: value.ResponseDigest, ErrorCode: value.ErrorCode, CostMinor: value.CostMinor, Currency: value.Currency, PublishedAt: value.PublishedAt, ObservedAt: value.ObservedAt, SafeSummary: value.SafeSummary}
}

func redactSafeSummary(input map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range input {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "credential") {
			result[key] = "[redacted]"
		} else {
			result[key] = value
		}
	}
	return result
}

func (s *DeliveryService) completePublishedDelivery(ctx context.Context, actor Actor, publication *deliverydomain.ChannelPublication, now time.Time) error {
	if publication.State != deliverydomain.ChannelPublicationPublished {
		return nil
	}
	delivery, err := s.delivery.TaskDelivery(ctx, actor.TenantID, publication.TaskDeliveryID)
	if err != nil {
		return err
	}
	if delivery.Status != deliverydomain.TaskDeliveryDelivered {
		delivery.Status, delivery.DeliveredBy, delivery.DeliveredAt, delivery.UpdatedAt = deliverydomain.TaskDeliveryDelivered, actor.UserID, &now, now
		if err := s.delivery.SaveTaskDelivery(ctx, delivery); err != nil {
			return err
		}
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, delivery.TaskID)
	if err != nil {
		return err
	}
	if task.Status != work.TaskStatusDelivered {
		task.Status, task.NextAction, task.UpdatedAt = work.TaskStatusDelivered, "已发布并记录渠道回执", now
		return s.tasks.SaveWorkTask(ctx, task)
	}
	return nil
}
