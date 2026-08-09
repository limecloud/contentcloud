package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) SaveAgentSession(ctx context.Context, record agentadapter.AgentSessionRecord) error {
	if record.TenantID == "" || record.Session.HarnessKind == "" || record.Session.SessionID == "" || record.State == "" || record.LastEventAt.IsZero() {
		return domain.Invalid("AGENT_SESSION_INVALID", "Agent SessionStore 记录缺少租户、会话或状态")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = record.LastEventAt
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.LastEventAt
	}
	return s.withTenant(ctx, record.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_agent_sessions(tenant_id,harness_kind,session_id,state,last_event_at,error_code,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,1,$7,$8) ON CONFLICT (tenant_id,harness_kind,session_id) DO UPDATE SET state=EXCLUDED.state,last_event_at=EXCLUDED.last_event_at,error_code=EXCLUDED.error_code,version=runtime_agent_sessions.version+1,updated_at=EXCLUDED.updated_at WHERE runtime_agent_sessions.last_event_at <= EXCLUDED.last_event_at`, record.TenantID, record.Session.HarnessKind, record.Session.SessionID, record.State, record.LastEventAt, record.ErrorCode, record.CreatedAt, record.UpdatedAt)
		return dbError(err)
	})
}

func (s *Store) AgentSession(ctx context.Context, tenantID string, ref agentadapter.AgentSessionRef) (agentadapter.AgentSessionRecord, error) {
	var result agentadapter.AgentSessionRecord
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT tenant_id,harness_kind,session_id,state,last_event_at,error_code,version,created_at,updated_at FROM runtime_agent_sessions WHERE tenant_id=$1 AND harness_kind=$2 AND session_id=$3`, tenantID, ref.HarnessKind, ref.SessionID).Scan(&result.TenantID, &result.Session.HarnessKind, &result.Session.SessionID, &result.State, &result.LastEventAt, &result.ErrorCode, &result.Version, &result.CreatedAt, &result.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("Agent SessionStore 会话")
		}
		result.Session.TenantID = result.TenantID
		return err
	})
	return result, err
}

func (s *Store) AppendAgentEvent(ctx context.Context, value agentadapter.AgentSessionEvent) error {
	if value.TenantID == "" || value.Session.HarnessKind == "" || value.Session.SessionID == "" || value.Sequence < 1 || value.Event.Type == "" || value.Digest == "" || value.CreatedAt.IsZero() {
		return domain.Invalid("AGENT_SESSION_EVENT_INVALID", "Agent SessionStore 事件缺少会话、序号或摘要")
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_agent_session_events(tenant_id,harness_kind,session_id,sequence,event_type,event_data,error_code,occurred_at,digest,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (tenant_id,harness_kind,session_id,digest) DO NOTHING`, value.TenantID, value.Session.HarnessKind, value.Session.SessionID, value.Sequence, value.Event.Type, nullableAgentEventData(value.Event.Data), value.Event.ErrorCode, value.Event.OccurredAt, value.Digest, value.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) AgentEvents(ctx context.Context, tenantID string, ref agentadapter.AgentSessionRef, after int64) ([]agentadapter.AgentSessionEvent, error) {
	result := []agentadapter.AgentSessionEvent{}
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT sequence,event_type,event_data,error_code,occurred_at,digest,created_at FROM runtime_agent_session_events WHERE tenant_id=$1 AND harness_kind=$2 AND session_id=$3 AND sequence>$4 ORDER BY sequence`, tenantID, ref.HarnessKind, ref.SessionID, after)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value := agentadapter.AgentSessionEvent{TenantID: tenantID, Session: agentadapter.AgentSessionRef{TenantID: tenantID, HarnessKind: ref.HarnessKind, SessionID: ref.SessionID}}
			var data []byte
			if err := rows.Scan(&value.Sequence, &value.Event.Type, &data, &value.Event.ErrorCode, &value.Event.OccurredAt, &value.Digest, &value.CreatedAt); err != nil {
				return err
			}
			value.Event.Session = value.Session
			if string(data) != "null" {
				value.Event.Data = append(json.RawMessage(nil), data...)
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	return result, err
}

func nullableAgentEventData(data json.RawMessage) []byte {
	if len(data) == 0 {
		return []byte("null")
	}
	return data
}
