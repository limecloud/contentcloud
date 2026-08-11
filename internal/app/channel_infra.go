package app

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/channeladapter"
	"github.com/limecloud/contentcloud/internal/domain"
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
	TaskDeliveryID   string                                `json:"task_delivery_id"`
	ChannelBindingID string                                `json:"channel_binding_id"`
	IdempotencyKey   string                                `json:"idempotency_key"`
	ContentProfileID string                                `json:"content_profile_id,omitempty"`
	DouyinCommerce   *domain.DouyinCommercePublicationRefs `json:"douyin_commerce,omitempty"`
	ScheduledAt      *time.Time                            `json:"scheduled_at,omitempty"`
	Metadata         map[string]any                        `json:"metadata"`
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
	Applied     bool                          `json:"applied"`
	Receipt     domain.ChannelCallbackReceipt `json:"receipt"`
	Publication domain.ChannelPublication     `json:"publication"`
}

type ChannelReconcileResult struct {
	Inspected int                         `json:"inspected"`
	Updated   int                         `json:"updated"`
	Failed    int                         `json:"failed"`
	Items     []domain.ChannelPublication `json:"items"`
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

func (s *Service) ChannelAdapterIDs() []string {
	return s.channelAdapters.IDs()
}

func (s *Service) CreateChannelBinding(ctx context.Context, actor Actor, input CreateChannelBindingInput, requestID string) (domain.ChannelBinding, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil {
		return domain.ChannelBinding{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, input.ProjectID); err != nil {
		return domain.ChannelBinding{}, err
	}
	adapterID := defaultString(strings.ToLower(strings.TrimSpace(input.AdapterID)), channeladapter.ManualAdapterID)
	if _, err := s.channelAdapters.Resolve(adapterID); err != nil {
		return domain.ChannelBinding{}, err
	}
	now := s.now().UTC()
	value := domain.ChannelBinding{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: input.ProjectID, Channel: strings.ToLower(strings.TrimSpace(input.Channel)), AdapterID: adapterID, AccountRef: strings.TrimSpace(input.AccountRef), AuthorizationSecretRef: strings.TrimSpace(input.AuthorizationSecretRef), Region: defaultString(strings.TrimSpace(input.Region), "global"), Status: domain.ChannelBindingActive, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := value.Validate(); err != nil {
		return value, err
	}
	if err := s.store.CreateChannelBinding(ctx, value); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.binding_created", "channel_binding", value.ID, requestID, map[string]any{"channel": value.Channel, "adapter_id": value.AdapterID, "account_ref": value.AccountRef})
	return value, nil
}

func (s *Service) ChannelBindings(ctx context.Context, actor Actor, projectID string) ([]domain.ChannelBinding, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.ChannelBindings(ctx, actor.TenantID, projectID)
}

func (s *Service) PrepareChannelPublication(ctx context.Context, actor Actor, input PrepareChannelPublicationInput, requestID string) (domain.ChannelPublication, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.ChannelPublication{}, err
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		return domain.ChannelPublication{}, domain.Invalid("CHANNEL_IDEMPOTENCY_KEY_REQUIRED", "渠道发布必须提供幂等键")
	}
	if existing, err := s.store.ChannelPublicationByIdempotencyKey(ctx, actor.TenantID, key); err == nil {
		return existing, nil
	} else if !domain.IsNotFound(err) {
		return domain.ChannelPublication{}, err
	}
	delivery, err := s.store.TaskDelivery(ctx, actor.TenantID, input.TaskDeliveryID)
	if err != nil {
		return domain.ChannelPublication{}, err
	}
	if delivery.Status != domain.TaskDeliveryReady || delivery.IntegrityStatus != "complete" || delivery.DeliveryPackageID == "" {
		return domain.ChannelPublication{}, domain.Policy("CHANNEL_DELIVERY_NOT_READY", "渠道发布必须引用完整且 ready 的任务交付", "先构建并校验交付包")
	}
	binding, err := s.store.ChannelBinding(ctx, actor.TenantID, input.ChannelBindingID)
	if err != nil {
		return domain.ChannelPublication{}, err
	}
	if binding.ProjectID != delivery.ProjectID || binding.Status != domain.ChannelBindingActive {
		return domain.ChannelPublication{}, domain.Policy("CHANNEL_BINDING_NOT_ACTIVE", "渠道绑定不属于该项目或已停用", "选择当前项目的有效渠道绑定")
	}
	adapter, err := s.channelAdapters.Resolve(binding.AdapterID)
	if err != nil {
		return domain.ChannelPublication{}, err
	}
	metadata := cloneMetadata(input.Metadata)
	if input.ContentProfileID == domain.DouyinCommerceProfileID {
		validated, validationErr := s.validateDouyinCommercePublication(ctx, actor, delivery, binding, input)
		if validationErr != nil {
			return domain.ChannelPublication{}, validationErr
		}
		metadata = validated
	} else if input.DouyinCommerce != nil {
		return domain.ChannelPublication{}, domain.Invalid("CHANNEL_PROFILE_REFS_MISMATCH", "只有 douyin-commerce-video Profile 可以提供抖音电商类型化引用")
	}
	if input.ContentProfileID != "" {
		metadata["content_profile_id"] = input.ContentProfileID
	}
	request := channeladapter.Request{TenantID: actor.TenantID, ProjectID: delivery.ProjectID, DeliveryPackageID: delivery.DeliveryPackageID, DeliveryDigest: delivery.DeliveryDigest, Channel: binding.Channel, AccountRef: binding.AccountRef, AuthorizationRef: binding.AuthorizationSecretRef, IdempotencyKey: key, ScheduledAt: input.ScheduledAt, Metadata: metadata}
	prepared, err := adapter.Prepare(ctx, request)
	if err != nil {
		return domain.ChannelPublication{}, err
	}
	now := s.now().UTC()
	value := domain.ChannelPublication{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: delivery.ProjectID, TaskID: delivery.TaskID, TaskDeliveryID: delivery.ID, DeliveryPackageID: delivery.DeliveryPackageID, ChannelBindingID: binding.ID, Channel: binding.Channel, AccountRef: binding.AccountRef, State: domain.ChannelPublicationPrepared, IdempotencyKey: key, DeliveryDigest: delivery.DeliveryDigest, RequestDigest: prepared.RequestDigest, Checklist: prepared.Checklist, Preview: prepared.Preview, Metadata: request.Metadata, SafeSummary: map[string]any{}, ScheduledAt: input.ScheduledAt, ObservedAt: now, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	value.NormalizeCollections()
	if err := s.store.CreateChannelPublication(ctx, value); err != nil {
		if existing, replayErr := s.store.ChannelPublicationByIdempotencyKey(ctx, actor.TenantID, key); replayErr == nil {
			return existing, nil
		}
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.publication_prepared", "channel_publication", value.ID, requestID, map[string]any{"task_delivery_id": value.TaskDeliveryID, "channel": value.Channel, "request_digest": value.RequestDigest})
	return value, nil
}

func (s *Service) SubmitChannelPublication(ctx context.Context, actor Actor, id, requestID string) (domain.ChannelPublication, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.ChannelPublication{}, err
	}
	value, binding, adapter, prepared, err := s.channelPublicationContext(ctx, actor, id)
	if err != nil {
		return value, err
	}
	if value.State != domain.ChannelPublicationPrepared {
		return value, domain.Conflict("CHANNEL_PUBLICATION_NOT_PREPARED", "只有 prepared 发布可以提交")
	}
	receipt, submitErr := adapter.Submit(ctx, prepared)
	now := s.now().UTC()
	value.SubmittedAt = &now
	applyChannelReceipt(&value, receipt, now)
	if submitErr != nil && value.State == domain.ChannelPublicationPrepared {
		value.State = domain.ChannelPublicationUnknown
		value.ErrorCode = "CHANNEL_SUBMIT_UNKNOWN"
	}
	if err := s.store.SaveChannelPublication(ctx, value); err != nil {
		return value, err
	}
	if err := s.completePublishedDelivery(ctx, actor, &value, now); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.publication_submitted", "channel_publication", value.ID, requestID, map[string]any{"adapter_id": binding.AdapterID, "state": value.State, "external_id": value.ExternalID})
	return value, submitErr
}

func (s *Service) InspectChannelPublication(ctx context.Context, actor Actor, id, requestID string) (domain.ChannelPublication, error) {
	value, _, adapter, _, err := s.channelPublicationContext(ctx, actor, id)
	if err != nil {
		return value, err
	}
	receipt, inspectErr := adapter.Inspect(ctx, publicationReceipt(value))
	now := s.now().UTC()
	applyChannelReceipt(&value, receipt, now)
	if inspectErr != nil {
		value.State = domain.ChannelPublicationUnknown
		value.ErrorCode = "CHANNEL_INSPECT_UNKNOWN"
	}
	if err := s.store.SaveChannelPublication(ctx, value); err != nil {
		return value, err
	}
	if err := s.completePublishedDelivery(ctx, actor, &value, now); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.publication_inspected", "channel_publication", value.ID, requestID, map[string]any{"state": value.State})
	return value, inspectErr
}

func (s *Service) WithdrawChannelPublication(ctx context.Context, actor Actor, id, reason, requestID string) (domain.ChannelPublication, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil {
		return domain.ChannelPublication{}, err
	}
	value, _, adapter, _, err := s.channelPublicationContext(ctx, actor, id)
	if err != nil {
		return value, err
	}
	if value.State != domain.ChannelPublicationPublished && value.State != domain.ChannelPublicationSubmitted && value.State != domain.ChannelPublicationUnknown {
		return value, domain.Conflict("CHANNEL_PUBLICATION_NOT_WITHDRAWABLE", "当前渠道发布状态不能撤回")
	}
	receipt, err := adapter.Withdraw(ctx, publicationReceipt(value), strings.TrimSpace(reason))
	if err != nil {
		return value, err
	}
	now := s.now().UTC()
	applyChannelReceipt(&value, receipt, now)
	if err := s.store.SaveChannelPublication(ctx, value); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.publication_withdrawn", "channel_publication", value.ID, requestID, map[string]any{"external_id": value.ExternalID, "reason": strings.TrimSpace(reason)})
	return value, nil
}

func (s *Service) RecordManualChannelReceipt(ctx context.Context, actor Actor, id string, input RecordManualChannelReceiptInput, requestID string) (domain.ChannelPublication, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
		return domain.ChannelPublication{}, err
	}
	value, binding, _, _, err := s.channelPublicationContext(ctx, actor, id)
	if err != nil {
		return value, err
	}
	if binding.AdapterID != channeladapter.ManualAdapterID || value.State != domain.ChannelPublicationManualActionRequired {
		return value, domain.Conflict("CHANNEL_MANUAL_RECEIPT_NOT_ALLOWED", "当前发布不处于人工回执待记录状态")
	}
	now := s.now().UTC()
	state := strings.ToLower(strings.TrimSpace(input.State))
	if state != domain.ChannelPublicationPublished && state != domain.ChannelPublicationFailed && state != domain.ChannelPublicationWithdrawn {
		return value, domain.Invalid("CHANNEL_MANUAL_RECEIPT_STATE_INVALID", "人工回执状态必须是 published、failed 或 withdrawn")
	}
	if state == domain.ChannelPublicationPublished && (strings.TrimSpace(input.ExternalID) == "" || input.PublishedAt == nil) {
		return value, domain.Invalid("CHANNEL_MANUAL_RECEIPT_INCOMPLETE", "人工发布成功必须记录外部 ID 和发布时间")
	}
	responseDigest, err := domain.CanonicalHash(input)
	if err != nil {
		return value, err
	}
	value.State, value.ExternalID, value.ExternalURL = state, strings.TrimSpace(input.ExternalID), strings.TrimSpace(input.ExternalURL)
	value.ResponseDigest, value.ErrorCode, value.PublishedAt = "sha256:"+responseDigest, strings.TrimSpace(input.ErrorCode), input.PublishedAt
	value.SafeSummary, value.ObservedAt, value.UpdatedAt = redactSafeSummary(input.SafeSummary), now, now
	if err := s.store.SaveChannelPublication(ctx, value); err != nil {
		return value, err
	}
	if err := s.completePublishedDelivery(ctx, actor, &value, now); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "channel.manual_receipt_recorded", "channel_publication", value.ID, requestID, map[string]any{"state": value.State, "external_id": value.ExternalID})
	return value, nil
}

func (s *Service) ChannelPublications(ctx context.Context, actor Actor, taskID string) ([]domain.ChannelPublication, error) {
	if taskID != "" {
		if _, err := s.store.WorkTask(ctx, actor.TenantID, taskID); err != nil {
			return nil, err
		}
	}
	return s.store.ChannelPublications(ctx, actor.TenantID, taskID)
}

func (s *Service) ReceiveChannelCallback(ctx context.Context, tenantID, adapterID string, input ReceiveChannelCallbackInput, requestID string) (ReceiveChannelCallbackResult, error) {
	result := ReceiveChannelCallbackResult{}
	tenantID, adapterID = strings.TrimSpace(tenantID), strings.ToLower(strings.TrimSpace(adapterID))
	input.EventID, input.PublicationID = strings.TrimSpace(input.EventID), strings.TrimSpace(input.PublicationID)
	if tenantID == "" || adapterID == "" || input.EventID == "" || input.PublicationID == "" {
		return result, domain.Invalid("CHANNEL_CALLBACK_SCOPE_INVALID", "渠道 Callback 缺少租户、Adapter、事件或发布 ID")
	}
	value, err := s.store.ChannelPublication(ctx, tenantID, input.PublicationID)
	if err != nil {
		return result, err
	}
	binding, err := s.store.ChannelBinding(ctx, tenantID, value.ChannelBindingID)
	if err != nil {
		return result, err
	}
	if binding.AdapterID != adapterID || binding.Status != domain.ChannelBindingActive || adapterID == channeladapter.ManualAdapterID {
		return result, domain.Policy("CHANNEL_CALLBACK_UNBOUND", "渠道 Callback 与有效的远程 Adapter 绑定不匹配", "检查租户、Adapter 和渠道绑定")
	}
	nextState := strings.ToLower(strings.TrimSpace(input.State))
	if !validChannelCallbackTransition(value.State, nextState) {
		return result, domain.Conflict("CHANNEL_CALLBACK_TRANSITION_INVALID", "渠道 Callback 不能执行当前状态迁移")
	}
	observedAt := s.now().UTC()
	if input.ObservedAt != nil {
		observedAt = input.ObservedAt.UTC()
	}
	if nextState == domain.ChannelPublicationPublished && (strings.TrimSpace(input.ExternalID) == "" || input.PublishedAt == nil) {
		return result, domain.Invalid("CHANNEL_CALLBACK_PUBLISHED_INCOMPLETE", "published Callback 必须包含外部 ID 和发布时间")
	}
	responseDigest, err := domain.CanonicalHash(input)
	if err != nil {
		return result, err
	}
	now := s.now().UTC()
	value.State = nextState
	value.ExternalID, value.ExternalURL = strings.TrimSpace(input.ExternalID), strings.TrimSpace(input.ExternalURL)
	value.ResponseDigest, value.ErrorCode = "sha256:"+responseDigest, strings.TrimSpace(input.ErrorCode)
	value.CostMinor, value.Currency, value.PublishedAt = input.CostMinor, strings.ToUpper(strings.TrimSpace(input.Currency)), input.PublishedAt
	value.SafeSummary, value.ObservedAt, value.UpdatedAt = redactSafeSummary(input.SafeSummary), observedAt, now
	receipt := domain.ChannelCallbackReceipt{ID: domain.NewID(), TenantID: tenantID, PublicationID: value.ID, AdapterID: adapterID, EventID: input.EventID, PayloadDigest: input.PayloadDigest, State: nextState, SafeSummary: value.SafeSummary, ObservedAt: observedAt, ReceivedAt: now}
	if err := receipt.Validate(); err != nil {
		return result, err
	}
	applied, err := s.store.ApplyChannelCallback(ctx, value, receipt)
	if err != nil {
		return result, err
	}
	if !applied {
		value, err = s.store.ChannelPublication(ctx, tenantID, input.PublicationID)
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
	case domain.ChannelPublicationSubmitted, domain.ChannelPublicationUnknown:
		return next == domain.ChannelPublicationSubmitted || next == domain.ChannelPublicationPublished || next == domain.ChannelPublicationFailed || next == domain.ChannelPublicationUnknown || next == domain.ChannelPublicationWithdrawn
	case domain.ChannelPublicationPublished:
		return next == domain.ChannelPublicationPublished || next == domain.ChannelPublicationWithdrawn
	case domain.ChannelPublicationFailed:
		return next == domain.ChannelPublicationFailed
	case domain.ChannelPublicationWithdrawn:
		return next == domain.ChannelPublicationWithdrawn
	default:
		return false
	}
}

func (s *Service) ReconcileChannelPublications(ctx context.Context, actor Actor, limit int, requestID string) (ChannelReconcileResult, error) {
	result := ChannelReconcileResult{Items: []domain.ChannelPublication{}}
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil {
		return result, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return result, domain.Invalid("CHANNEL_RECONCILE_LIMIT_INVALID", "单次渠道对账最多处理 1000 条")
	}
	values, err := s.store.ChannelPublications(ctx, actor.TenantID, "")
	if err != nil {
		return result, err
	}
	for _, value := range values {
		if result.Inspected >= limit {
			break
		}
		if value.State != domain.ChannelPublicationSubmitted && value.State != domain.ChannelPublicationUnknown {
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

func (s *Service) ImportChannelPerformance(ctx context.Context, actor Actor, publicationID string, input ImportChannelPerformanceInput, requestID string) (ImportPerformanceResult, error) {
	value, err := s.store.ChannelPublication(ctx, actor.TenantID, publicationID)
	if err != nil {
		return ImportPerformanceResult{}, err
	}
	if value.State != domain.ChannelPublicationPublished || value.PublishedAt == nil || value.ExternalID == "" {
		return ImportPerformanceResult{}, domain.Policy("CHANNEL_PERFORMANCE_NOT_PUBLISHED", "只有已取得 published 回执的渠道内容可以导入指标", "先完成渠道发布回执")
	}
	sampleStatus := strings.TrimSpace(input.SampleStatus)
	if sampleStatus == "" {
		sampleStatus = "insufficient_sample"
	}
	return s.ImportPerformanceObservations(ctx, actor, ImportPerformanceInput{ProjectID: value.ProjectID, SourceName: "channel-publication:" + value.ID, SourceFormat: "json", Observations: []CreateObservationInput{{ApprovedSnapshotID: strings.TrimSpace(input.ApprovedSnapshotID), Platform: value.Channel, AccountAlias: value.AccountRef, PublishedAt: value.PublishedAt.UTC(), WindowHours: input.WindowHours, SampleStatus: sampleStatus, Metrics: input.Metrics, Currency: input.Currency, Spend: input.Spend, GMV: input.GMV, IssueCategory: input.IssueCategory, Notes: input.Notes}}}, requestID)
}

func (s *Service) channelPublicationContext(ctx context.Context, actor Actor, id string) (domain.ChannelPublication, domain.ChannelBinding, channeladapter.Adapter, channeladapter.Prepared, error) {
	value, err := s.store.ChannelPublication(ctx, actor.TenantID, id)
	if err != nil {
		return value, domain.ChannelBinding{}, nil, channeladapter.Prepared{}, err
	}
	binding, err := s.store.ChannelBinding(ctx, actor.TenantID, value.ChannelBindingID)
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

func applyChannelReceipt(value *domain.ChannelPublication, receipt channeladapter.Receipt, now time.Time) {
	if receipt.State != "" {
		value.State = receipt.State
	}
	value.ExternalID, value.ExternalURL = receipt.ExternalID, receipt.ExternalURL
	value.ResponseDigest, value.ErrorCode = receipt.ResponseDigest, receipt.ErrorCode
	value.CostMinor, value.Currency, value.PublishedAt = receipt.CostMinor, receipt.Currency, receipt.PublishedAt
	value.SafeSummary, value.ObservedAt, value.UpdatedAt = redactSafeSummary(receipt.SafeSummary), now, now
}

func publicationReceipt(value domain.ChannelPublication) channeladapter.Receipt {
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

func (s *Service) completePublishedDelivery(ctx context.Context, actor Actor, publication *domain.ChannelPublication, now time.Time) error {
	if publication.State != domain.ChannelPublicationPublished {
		return nil
	}
	delivery, err := s.store.TaskDelivery(ctx, actor.TenantID, publication.TaskDeliveryID)
	if err != nil {
		return err
	}
	if delivery.Status != domain.TaskDeliveryDelivered {
		delivery.Status, delivery.DeliveredBy, delivery.DeliveredAt, delivery.UpdatedAt = domain.TaskDeliveryDelivered, actor.UserID, &now, now
		if err := s.store.SaveTaskDelivery(ctx, delivery); err != nil {
			return err
		}
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, delivery.TaskID)
	if err != nil {
		return err
	}
	if task.Status != domain.TaskStatusDelivered {
		task.Status, task.NextAction, task.UpdatedAt = domain.TaskStatusDelivered, "已发布并记录渠道回执", now
		return s.store.SaveWorkTask(ctx, task)
	}
	return nil
}
