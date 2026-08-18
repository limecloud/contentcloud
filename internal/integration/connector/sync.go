package connector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

type Binding struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	ProjectID        string    `json:"project_id"`
	ConnectorID      string    `json:"connector_id"`
	AuthorizationRef string    `json:"authorization_ref"`
	Region           string    `json:"region"`
	Status           string    `json:"status"`
	Cursor           string    `json:"cursor"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

const (
	BindingActive   = "active"
	BindingDisabled = "disabled"
)

func (v Binding) Validate() error {
	if strings.TrimSpace(v.ID) == "" || strings.TrimSpace(v.TenantID) == "" || strings.TrimSpace(v.ProjectID) == "" || strings.TrimSpace(v.ConnectorID) == "" {
		return fault.Invalid("CONNECTOR_BINDING_INVALID", "Connector 绑定缺少租户、项目或 Connector")
	}
	if !validSecretRef(v.AuthorizationRef) {
		return fault.Invalid("CONNECTOR_AUTHORIZATION_REF_INVALID", "Connector 授权必须保存 SecretRef，不能保存明文 Token")
	}
	if v.Status != BindingActive && v.Status != BindingDisabled {
		return fault.Invalid("CONNECTOR_BINDING_STATUS_INVALID", "Connector 绑定状态无效")
	}
	return nil
}

func validSecretRef(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "secret://") || strings.HasPrefix(value, "vault://") || strings.HasPrefix(value, "env://")
}

type PullRequest struct {
	Binding Binding `json:"binding"`
	Cursor  string  `json:"cursor"`
	Limit   int     `json:"limit"`
}

type Record struct {
	ExternalID string         `json:"external_id"`
	Version    string         `json:"version"`
	Title      string         `json:"title"`
	SourceURL  string         `json:"source_url,omitempty"`
	MIME       string         `json:"mime,omitempty"`
	Body       []byte         `json:"-"`
	Digest     string         `json:"digest,omitempty"`
	Deleted    bool           `json:"deleted"`
	DeletedAt  *time.Time     `json:"deleted_at,omitempty"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Rights     map[string]any `json:"rights"`
	Metadata   map[string]any `json:"metadata"`
}

type PullResult struct {
	Records    []Record `json:"records"`
	NextCursor string   `json:"next_cursor"`
	HasMore    bool     `json:"has_more"`
}

type Adapter interface {
	Pull(context.Context, PullRequest) (PullResult, error)
}

type SyncReceipt struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	SchemaVersion  string    `json:"schema_version"`
	BindingID      string    `json:"binding_id"`
	ConnectorID    string    `json:"connector_id"`
	PreviousCursor string    `json:"previous_cursor"`
	NextCursor     string    `json:"next_cursor"`
	Records        []Record  `json:"records"`
	UpsertCount    int       `json:"upsert_count"`
	TombstoneCount int       `json:"tombstone_count"`
	HasMore        bool      `json:"has_more"`
	Digest         string    `json:"digest"`
	ObservedAt     time.Time `json:"observed_at"`
}

// RecordMapping is the durable bridge from an external object/version to the
// existing Source/SourceRevision chain. Connector state never owns content.
type RecordMapping struct {
	TenantID        string         `json:"tenant_id"`
	ProjectID       string         `json:"project_id"`
	BindingID       string         `json:"binding_id"`
	ExternalID      string         `json:"external_id"`
	ExternalVersion string         `json:"external_version"`
	SourceID        string         `json:"source_id,omitempty"`
	RevisionID      string         `json:"revision_id,omitempty"`
	Digest          string         `json:"digest,omitempty"`
	SourceURL       string         `json:"source_url,omitempty"`
	Deleted         bool           `json:"deleted"`
	DeletedAt       *time.Time     `json:"deleted_at,omitempty"`
	Rights          map[string]any `json:"rights"`
	Metadata        map[string]any `json:"metadata"`
	ObservedAt      time.Time      `json:"observed_at"`
}

type SyncLease struct {
	Owner     string
	ExpiresAt time.Time
}

func (v *RecordMapping) NormalizeCollections() {
	if v.Rights == nil {
		v.Rights = map[string]any{}
	}
	if v.Metadata == nil {
		v.Metadata = map[string]any{}
	}
}

// Repository is intentionally separate from the global application Store.
// Connector persistence is a module port, not another reason to grow the
// legacy cross-domain Store interface.
type Repository interface {
	CreateBinding(context.Context, Binding) error
	Bindings(context.Context, string, string) ([]Binding, error)
	Binding(context.Context, string, string) (Binding, error)
	AcquireSyncLease(context.Context, string, string, SyncLease) error
	ReleaseSyncLease(context.Context, string, string, string) error
	Record(context.Context, string, string, string) (RecordMapping, error)
	SaveRecord(context.Context, RecordMapping) error
	ActiveRecordsForSource(context.Context, string, string) ([]RecordMapping, error)
	CommitReceipt(context.Context, Binding, string, string, SyncReceipt) error
	Receipts(context.Context, string, string) ([]SyncReceipt, error)
}

type Engine struct {
	adapter Adapter
	now     func() time.Time
}

func New(adapter Adapter) *Engine { return &Engine{adapter: adapter, now: time.Now} }

func (e *Engine) Sync(ctx context.Context, request PullRequest) (SyncReceipt, error) {
	if e == nil || e.adapter == nil {
		return SyncReceipt{}, fault.Policy("CONNECTOR_ADAPTER_UNAVAILABLE", "Connector Adapter 未配置", "绑定可用 Connector 后重试")
	}
	if err := request.Binding.Validate(); err != nil {
		return SyncReceipt{}, err
	}
	if request.Binding.Status != BindingActive {
		return SyncReceipt{}, fault.Policy("CONNECTOR_BINDING_DISABLED", "Connector 绑定已停用", "启用绑定后重试")
	}
	if request.Limit <= 0 {
		request.Limit = 100
	}
	if request.Limit > 1000 {
		return SyncReceipt{}, fault.Invalid("CONNECTOR_LIMIT_INVALID", "单次 Connector 同步最多 1000 条")
	}
	result, err := e.adapter.Pull(ctx, request)
	if err != nil {
		return SyncReceipt{}, err
	}
	if len(result.Records) > request.Limit || (result.HasMore && strings.TrimSpace(result.NextCursor) == "") {
		return SyncReceipt{}, fault.Invalid("CONNECTOR_RESULT_INVALID", "Connector 返回数量超限或缺少下一游标")
	}
	seen := map[string]string{}
	upserts, tombstones := 0, 0
	for index := range result.Records {
		record := &result.Records[index]
		record.ExternalID, record.Version = strings.TrimSpace(record.ExternalID), strings.TrimSpace(record.Version)
		if record.ExternalID == "" || record.Version == "" || record.UpdatedAt.IsZero() {
			return SyncReceipt{}, fault.Invalid("CONNECTOR_RECORD_INVALID", "Connector 记录缺少外部 ID、版本或更新时间")
		}
		if prior, exists := seen[record.ExternalID]; exists {
			if prior == record.Version {
				return SyncReceipt{}, fault.Conflict("CONNECTOR_RECORD_DUPLICATE", "Connector 同一页返回了重复记录")
			}
			return SyncReceipt{}, fault.Conflict("CONNECTOR_VERSION_AMBIGUOUS", "Connector 同一页返回同一对象的多个版本")
		}
		seen[record.ExternalID] = record.Version
		if record.Rights == nil {
			record.Rights = map[string]any{}
		}
		if record.Metadata == nil {
			record.Metadata = map[string]any{}
		}
		if record.Deleted {
			if len(record.Body) != 0 || record.DeletedAt == nil {
				return SyncReceipt{}, fault.Invalid("CONNECTOR_TOMBSTONE_INVALID", "删除记录必须包含 deleted_at 且不能携带正文")
			}
			record.Digest = ""
			tombstones++
			continue
		}
		if len(record.Body) == 0 || strings.TrimSpace(record.MIME) == "" {
			return SyncReceipt{}, fault.Invalid("CONNECTOR_RECORD_BODY_REQUIRED", "非删除记录必须包含正文和 MIME")
		}
		digest := sha256.Sum256(record.Body)
		record.Digest = "sha256:" + hex.EncodeToString(digest[:])
		upserts++
	}
	now := e.now().UTC()
	receipt := SyncReceipt{TenantID: request.Binding.TenantID, ProjectID: request.Binding.ProjectID, SchemaVersion: "contentcloud.connector-sync/1.0", BindingID: request.Binding.ID, ConnectorID: request.Binding.ConnectorID, PreviousCursor: request.Cursor, NextCursor: result.NextCursor, Records: result.Records, UpsertCount: upserts, TombstoneCount: tombstones, HasMore: result.HasMore, ObservedAt: now}
	digest, err := stablehash.Sum(struct {
		SchemaVersion  string    `json:"schema_version"`
		BindingID      string    `json:"binding_id"`
		ConnectorID    string    `json:"connector_id"`
		PreviousCursor string    `json:"previous_cursor"`
		NextCursor     string    `json:"next_cursor"`
		Records        []Record  `json:"records"`
		UpsertCount    int       `json:"upsert_count"`
		TombstoneCount int       `json:"tombstone_count"`
		HasMore        bool      `json:"has_more"`
		ObservedAt     time.Time `json:"observed_at"`
	}{receipt.SchemaVersion, receipt.BindingID, receipt.ConnectorID, receipt.PreviousCursor, receipt.NextCursor, receipt.Records, receipt.UpsertCount, receipt.TombstoneCount, receipt.HasMore, receipt.ObservedAt})
	if err != nil {
		return SyncReceipt{}, err
	}
	receipt.Digest = "sha256:" + digest
	return receipt, nil
}
