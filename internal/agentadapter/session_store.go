package agentadapter

import (
	"context"
	"time"
)

// SessionStore mirrors opaque host sessions and structured events. It is an
// observation/recovery aid only; RuntimeAttempt, State and Effect facts remain
// in the Runtime command store.
type SessionStore interface {
	SaveAgentSession(context.Context, AgentSessionRecord) error
	AgentSession(context.Context, string, AgentSessionRef) (AgentSessionRecord, error)
	AppendAgentEvent(context.Context, AgentSessionEvent) error
	AgentEvents(context.Context, string, AgentSessionRef, int64) ([]AgentSessionEvent, error)
}

type AgentSessionRecord struct {
	TenantID    string          `json:"tenant_id"`
	Session     AgentSessionRef `json:"session"`
	State       string          `json:"state"`
	LastEventAt time.Time       `json:"last_event_at"`
	ErrorCode   string          `json:"error_code,omitempty"`
	Version     int             `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type AgentSessionEvent struct {
	TenantID  string          `json:"tenant_id"`
	Session   AgentSessionRef `json:"session"`
	Sequence  int64           `json:"sequence"`
	Event     AgentEvent      `json:"event"`
	Digest    string          `json:"digest"`
	CreatedAt time.Time       `json:"created_at"`
}
