package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/connector"
	"github.com/limecloud/contentcloud/internal/domain"
)

type CreateConnectorBindingInput struct {
	ProjectID        string `json:"project_id"`
	ConnectorID      string `json:"connector_id"`
	AuthorizationRef string `json:"authorization_ref"`
	Region           string `json:"region"`
}

type SyncConnectorInput struct {
	Limit int `json:"limit"`
}

func (s *Service) ConnectorAdapterIDs() []string { return s.connectorAdapters.IDs() }

func (s *Service) CreateConnectorBinding(ctx context.Context, actor Actor, input CreateConnectorBindingInput, requestID string) (connector.Binding, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil {
		return connector.Binding{}, err
	}
	if s.connectorRepository == nil {
		return connector.Binding{}, domain.Policy("CONNECTOR_STORE_UNAVAILABLE", "Connector 持久化未配置", "配置 Connector Repository 后重试")
	}
	if _, err := s.projectForWrite(ctx, actor, input.ProjectID); err != nil {
		return connector.Binding{}, err
	}
	connectorID := strings.ToLower(strings.TrimSpace(input.ConnectorID))
	if _, err := s.connectorAdapters.Resolve(connectorID); err != nil {
		return connector.Binding{}, err
	}
	now := s.now().UTC()
	value := connector.Binding{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: input.ProjectID, ConnectorID: connectorID, AuthorizationRef: strings.TrimSpace(input.AuthorizationRef), Region: defaultString(strings.TrimSpace(input.Region), "global"), Status: connector.BindingActive, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := value.Validate(); err != nil {
		return value, err
	}
	if err := s.connectorRepository.CreateBinding(ctx, value); err != nil {
		return value, err
	}
	s.audit(ctx, actor, value.ProjectID, "connector.binding_created", "connector_binding", value.ID, requestID, map[string]any{"connector_id": value.ConnectorID, "region": value.Region})
	return value, nil
}

func (s *Service) ConnectorBindings(ctx context.Context, actor Actor, projectID string) ([]connector.Binding, error) {
	if s.connectorRepository == nil {
		return nil, domain.Policy("CONNECTOR_STORE_UNAVAILABLE", "Connector 持久化未配置", "配置 Connector Repository 后重试")
	}
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.connectorRepository.Bindings(ctx, actor.TenantID, projectID)
}

func (s *Service) SyncConnector(ctx context.Context, actor Actor, bindingID string, input SyncConnectorInput, requestID string) (connector.SyncReceipt, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return connector.SyncReceipt{}, err
	}
	if s.connectorRepository == nil {
		return connector.SyncReceipt{}, domain.Policy("CONNECTOR_STORE_UNAVAILABLE", "Connector 持久化未配置", "配置 Connector Repository 后重试")
	}
	binding, err := s.connectorRepository.Binding(ctx, actor.TenantID, bindingID)
	if err != nil {
		return connector.SyncReceipt{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, binding.ProjectID); err != nil {
		return connector.SyncReceipt{}, err
	}
	leaseOwner := domain.NewID()
	if err := s.connectorRepository.AcquireSyncLease(ctx, actor.TenantID, binding.ID, connector.SyncLease{Owner: leaseOwner, ExpiresAt: s.now().UTC().Add(15 * time.Minute)}); err != nil {
		return connector.SyncReceipt{}, err
	}
	defer func() {
		_ = s.connectorRepository.ReleaseSyncLease(context.WithoutCancel(ctx), actor.TenantID, binding.ID, leaseOwner)
	}()
	// Re-read after acquiring the lease so the pull always starts from the
	// cursor protected by this owner, including after a prior lease expired.
	binding, err = s.connectorRepository.Binding(ctx, actor.TenantID, bindingID)
	if err != nil {
		return connector.SyncReceipt{}, err
	}
	adapter, err := s.connectorAdapters.Resolve(binding.ConnectorID)
	if err != nil {
		return connector.SyncReceipt{}, err
	}
	receipt, err := connector.New(adapter).Sync(ctx, connector.PullRequest{Binding: binding, Cursor: binding.Cursor, Limit: input.Limit})
	if err != nil {
		return receipt, err
	}
	for _, record := range receipt.Records {
		if err := s.materializeConnectorRecord(ctx, actor, binding, record, requestID); err != nil {
			return receipt, err
		}
	}
	receipt.ID = domain.NewID()
	if err := s.connectorRepository.CommitReceipt(ctx, binding, binding.Cursor, leaseOwner, receipt); err != nil {
		return receipt, err
	}
	s.audit(ctx, actor, binding.ProjectID, "connector.sync_committed", "connector_sync_receipt", receipt.ID, requestID, map[string]any{"binding_id": binding.ID, "previous_cursor": receipt.PreviousCursor, "next_cursor": receipt.NextCursor, "upsert_count": receipt.UpsertCount, "tombstone_count": receipt.TombstoneCount, "digest": receipt.Digest})
	return receipt, nil
}

func (s *Service) ConnectorReceipts(ctx context.Context, actor Actor, bindingID string) ([]connector.SyncReceipt, error) {
	if s.connectorRepository == nil {
		return nil, domain.Policy("CONNECTOR_STORE_UNAVAILABLE", "Connector 持久化未配置", "配置 Connector Repository 后重试")
	}
	if bindingID != "" {
		if _, err := s.connectorRepository.Binding(ctx, actor.TenantID, bindingID); err != nil {
			return nil, err
		}
	}
	return s.connectorRepository.Receipts(ctx, actor.TenantID, bindingID)
}

func (s *Service) materializeConnectorRecord(ctx context.Context, actor Actor, binding connector.Binding, record connector.Record, requestID string) error {
	mapping, err := s.connectorRepository.Record(ctx, actor.TenantID, binding.ID, record.ExternalID)
	if err != nil && !domain.IsNotFound(err) {
		return err
	}
	found := err == nil
	if found && mapping.ExternalVersion == record.Version && mapping.Digest == record.Digest && mapping.Deleted == record.Deleted {
		return nil
	}
	now := s.now().UTC()
	next := connector.RecordMapping{TenantID: actor.TenantID, ProjectID: binding.ProjectID, BindingID: binding.ID, ExternalID: record.ExternalID, ExternalVersion: record.Version, SourceURL: record.SourceURL, Digest: record.Digest, Deleted: record.Deleted, DeletedAt: record.DeletedAt, Rights: record.Rights, Metadata: record.Metadata, ObservedAt: now}
	if found {
		next.SourceID, next.RevisionID = mapping.SourceID, mapping.RevisionID
	}
	if record.Deleted {
		if err := s.connectorRepository.SaveRecord(ctx, next); err != nil {
			return err
		}
		if found && mapping.SourceID != "" {
			active, activeErr := s.connectorRepository.ActiveRecordsForSource(ctx, actor.TenantID, mapping.SourceID)
			if activeErr != nil {
				return activeErr
			}
			if len(active) == 0 {
				marker, marshalErr := json.Marshal(map[string]any{"schema_version": "contentcloud.connector-tombstone/1.0", "binding_id": binding.ID, "external_id": record.ExternalID, "external_version": record.Version, "deleted_at": record.DeletedAt})
				if marshalErr != nil {
					return marshalErr
				}
				revision, revisionErr := s.UploadSourceRevision(ctx, actor, mapping.SourceID, "connector-tombstone.json", "application/json", marker, requestID)
				if revisionErr != nil {
					return revisionErr
				}
				revision.ProcessingStatus = "invalidated"
				if err := s.store.SaveSourceRevision(ctx, revision); err != nil {
					return err
				}
				next.RevisionID = revision.ID
				return s.connectorRepository.SaveRecord(ctx, next)
			}
		}
		return nil
	}

	fileName, err := connectorFileName(record)
	if err != nil {
		return err
	}
	var revision domain.SourceRevision
	if found && mapping.SourceID != "" {
		revision, err = s.UploadSourceRevision(ctx, actor, mapping.SourceID, fileName, record.MIME, record.Body, requestID)
	} else {
		name := strings.TrimSpace(record.Title)
		if name == "" {
			name = record.ExternalID
		}
		revision, err = s.uploadSource(ctx, actor, binding.ProjectID, name, "connector:"+binding.ConnectorID, fileName, record.MIME, record.Body, requestID)
	}
	if err != nil {
		var value *domain.Error
		if !errors.As(err, &value) || (value.Code != "SOURCE_DUPLICATE" && value.Code != "RESOURCE_CONFLICT") {
			return err
		}
		revision, err = s.findSourceRevisionByDigest(ctx, actor, binding.ProjectID, strings.TrimPrefix(record.Digest, "sha256:"))
		if err != nil {
			return err
		}
	}
	next.SourceID, next.RevisionID = revision.SourceID, revision.ID
	return s.connectorRepository.SaveRecord(ctx, next)
}

func (s *Service) findSourceRevisionByDigest(ctx context.Context, actor Actor, projectID, digest string) (domain.SourceRevision, error) {
	sources, err := s.store.Sources(ctx, actor.TenantID, projectID)
	if err != nil {
		return domain.SourceRevision{}, err
	}
	for _, source := range sources {
		revisions, err := s.store.SourceRevisions(ctx, actor.TenantID, source.ID)
		if err != nil {
			return domain.SourceRevision{}, err
		}
		for _, revision := range revisions {
			if revision.SHA256 == digest {
				return revision, nil
			}
		}
	}
	return domain.SourceRevision{}, domain.NotFound("相同摘要的来源版本")
}

func connectorFileName(record connector.Record) (string, error) {
	extension := map[string]string{"application/json": ".json", "application/pdf": ".pdf", "text/csv": ".csv", "text/html": ".html", "text/markdown": ".md", "text/plain": ".txt", "image/jpeg": ".jpg", "image/png": ".png", "video/mp4": ".mp4", "audio/mpeg": ".mp3"}[strings.ToLower(strings.TrimSpace(strings.SplitN(record.MIME, ";", 2)[0]))]
	if extension == "" {
		return "", domain.Invalid("CONNECTOR_MIME_UNSUPPORTED", "Connector 记录 MIME 不受支持，不能以错误扩展名物化")
	}
	name := filepath.Base(strings.TrimSpace(record.ExternalID))
	name = strings.Map(func(value rune) rune {
		if value == '-' || value == '_' || value == '.' || value >= '0' && value <= '9' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' {
			return value
		}
		return '-'
	}, name)
	if name == "" || name == "." {
		name = "connector-record"
	}
	if filepath.Ext(name) == "" {
		name += extension
	}
	return name, nil
}
