package postgres

import (
	"context"
	"errors"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/jackc/pgx/v5"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
)

const channelBindingSelect = `SELECT tenant_id,id,project_id,channel,adapter_id,account_ref,authorization_secret_ref,region,status,created_by,created_at,updated_at FROM channel_bindings`

func scanChannelBinding(row pgx.Row) (deliverydomain.ChannelBinding, error) {
	var value deliverydomain.ChannelBinding
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.Channel, &value.AdapterID, &value.AccountRef, &value.AuthorizationSecretRef, &value.Region, &value.Status, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	return value, dbError(err)
}

func (s *Store) CreateChannelBinding(ctx context.Context, value deliverydomain.ChannelBinding) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO channel_bindings(tenant_id,id,project_id,channel,adapter_id,account_ref,authorization_secret_ref,region,status,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, value.TenantID, value.ID, value.ProjectID, value.Channel, value.AdapterID, value.AccountRef, value.AuthorizationSecretRef, value.Region, value.Status, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) ChannelBindings(ctx context.Context, tenantID, projectID string) ([]deliverydomain.ChannelBinding, error) {
	values := []deliverydomain.ChannelBinding{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, channelBindingSelect+` WHERE tenant_id=$1 AND ($2='' OR project_id::text=$2) ORDER BY created_at`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanChannelBinding(rows)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}

func (s *Store) ChannelBinding(ctx context.Context, tenantID, id string) (deliverydomain.ChannelBinding, error) {
	var value deliverydomain.ChannelBinding
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var err error
		value, err = scanChannelBinding(tx.QueryRow(ctx, channelBindingSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		if errors.Is(err, pgx.ErrNoRows) || fault.IsNotFound(err) {
			return fault.NotFound("渠道绑定")
		}
		return err
	})
	return value, err
}

func (s *Store) SaveChannelBinding(ctx context.Context, value deliverydomain.ChannelBinding) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE channel_bindings SET status=$3,adapter_id=$4,account_ref=$5,authorization_secret_ref=$6,region=$7,updated_at=$8 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.Status, value.AdapterID, value.AccountRef, value.AuthorizationSecretRef, value.Region, value.UpdatedAt)
		if err == nil && command.RowsAffected() == 0 {
			return fault.NotFound("渠道绑定")
		}
		return dbError(err)
	})
}

const channelPublicationSelect = `SELECT tenant_id,id,project_id,task_id,task_delivery_id,delivery_package_id,channel_binding_id,channel,account_ref,state,idempotency_key,delivery_digest,request_digest,response_digest,external_id,external_url,checklist,preview,metadata,safe_summary,cost_minor,currency,error_code,scheduled_at,submitted_at,published_at,observed_at,created_by,created_at,updated_at FROM channel_publications`

func scanChannelPublication(row pgx.Row) (deliverydomain.ChannelPublication, error) {
	var value deliverydomain.ChannelPublication
	var checklist, preview, metadata, summary []byte
	err := row.Scan(&value.TenantID, &value.ID, &value.ProjectID, &value.TaskID, &value.TaskDeliveryID, &value.DeliveryPackageID, &value.ChannelBindingID, &value.Channel, &value.AccountRef, &value.State, &value.IdempotencyKey, &value.DeliveryDigest, &value.RequestDigest, &value.ResponseDigest, &value.ExternalID, &value.ExternalURL, &checklist, &preview, &metadata, &summary, &value.CostMinor, &value.Currency, &value.ErrorCode, &value.ScheduledAt, &value.SubmittedAt, &value.PublishedAt, &value.ObservedAt, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.Checklist, err = decodeJSON[[]string](checklist)
	}
	if err == nil {
		value.Preview, err = decodeJSON[map[string]any](preview)
	}
	if err == nil {
		value.Metadata, err = decodeJSON[map[string]any](metadata)
	}
	if err == nil {
		value.SafeSummary, err = decodeJSON[map[string]any](summary)
	}
	value.NormalizeCollections()
	return value, dbError(err)
}

func (s *Store) CreateChannelPublication(ctx context.Context, value deliverydomain.ChannelPublication) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO channel_publications(tenant_id,id,project_id,task_id,task_delivery_id,delivery_package_id,channel_binding_id,channel,account_ref,state,idempotency_key,delivery_digest,request_digest,response_digest,external_id,external_url,checklist,preview,metadata,safe_summary,cost_minor,currency,error_code,scheduled_at,submitted_at,published_at,observed_at,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30)`, value.TenantID, value.ID, value.ProjectID, value.TaskID, value.TaskDeliveryID, value.DeliveryPackageID, value.ChannelBindingID, value.Channel, value.AccountRef, value.State, value.IdempotencyKey, value.DeliveryDigest, value.RequestDigest, value.ResponseDigest, value.ExternalID, value.ExternalURL, jsonArrayValue(value.Checklist), jsonValue(value.Preview), jsonValue(value.Metadata), jsonValue(value.SafeSummary), value.CostMinor, value.Currency, value.ErrorCode, value.ScheduledAt, value.SubmittedAt, value.PublishedAt, value.ObservedAt, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) ChannelPublicationByIdempotencyKey(ctx context.Context, tenantID, key string) (deliverydomain.ChannelPublication, error) {
	var value deliverydomain.ChannelPublication
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var err error
		value, err = scanChannelPublication(tx.QueryRow(ctx, channelPublicationSelect+` WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
		if fault.IsNotFound(err) {
			return fault.NotFound("渠道发布")
		}
		return err
	})
	return value, err
}

func (s *Store) ChannelPublications(ctx context.Context, tenantID, taskID string) ([]deliverydomain.ChannelPublication, error) {
	values := []deliverydomain.ChannelPublication{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, channelPublicationSelect+` WHERE tenant_id=$1 AND ($2='' OR task_id=$2) ORDER BY created_at`, tenantID, taskID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanChannelPublication(rows)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		return rows.Err()
	})
	return values, err
}

func (s *Store) ChannelPublication(ctx context.Context, tenantID, id string) (deliverydomain.ChannelPublication, error) {
	var value deliverydomain.ChannelPublication
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var err error
		value, err = scanChannelPublication(tx.QueryRow(ctx, channelPublicationSelect+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
		if fault.IsNotFound(err) {
			return fault.NotFound("渠道发布")
		}
		return err
	})
	return value, err
}

func (s *Store) SaveChannelPublication(ctx context.Context, value deliverydomain.ChannelPublication) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `UPDATE channel_publications SET state=$3,response_digest=$4,external_id=$5,external_url=$6,safe_summary=$7,cost_minor=$8,currency=$9,error_code=$10,submitted_at=$11,published_at=$12,observed_at=$13,updated_at=$14 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.State, value.ResponseDigest, value.ExternalID, value.ExternalURL, jsonValue(value.SafeSummary), value.CostMinor, value.Currency, value.ErrorCode, value.SubmittedAt, value.PublishedAt, value.ObservedAt, value.UpdatedAt)
		if err == nil && command.RowsAffected() == 0 {
			return fault.NotFound("渠道发布")
		}
		return dbError(err)
	})
}

func (s *Store) ApplyChannelCallback(ctx context.Context, value deliverydomain.ChannelPublication, receipt deliverydomain.ChannelCallbackReceipt) (bool, error) {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return false, err
	}
	if err := receipt.Validate(); err != nil {
		return false, err
	}
	applied := false
	err := s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `INSERT INTO channel_callback_receipts(tenant_id,id,publication_id,adapter_id,event_id,payload_digest,state,safe_summary,observed_at,received_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (tenant_id,adapter_id,event_id) DO NOTHING`, receipt.TenantID, receipt.ID, receipt.PublicationID, receipt.AdapterID, receipt.EventID, receipt.PayloadDigest, receipt.State, jsonValue(receipt.SafeSummary), receipt.ObservedAt, receipt.ReceivedAt)
		if err != nil {
			return dbError(err)
		}
		if command.RowsAffected() == 0 {
			return nil
		}
		command, err = tx.Exec(ctx, `UPDATE channel_publications SET state=$3,response_digest=$4,external_id=$5,external_url=$6,safe_summary=$7,cost_minor=$8,currency=$9,error_code=$10,submitted_at=$11,published_at=$12,observed_at=$13,updated_at=$14 WHERE tenant_id=$1 AND id=$2`, value.TenantID, value.ID, value.State, value.ResponseDigest, value.ExternalID, value.ExternalURL, jsonValue(value.SafeSummary), value.CostMinor, value.Currency, value.ErrorCode, value.SubmittedAt, value.PublishedAt, value.ObservedAt, value.UpdatedAt)
		if err == nil && command.RowsAffected() == 0 {
			return fault.NotFound("渠道发布")
		}
		if err != nil {
			return dbError(err)
		}
		applied = true
		return nil
	})
	return applied, err
}
