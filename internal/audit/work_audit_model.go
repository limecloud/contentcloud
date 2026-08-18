package audit

import "time"

type AuditEvent struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	ProjectID   string         `json:"project_id,omitempty"`
	ActorType   string         `json:"actor_type"`
	ActorID     string         `json:"actor_id"`
	Action      string         `json:"action"`
	SubjectType string         `json:"subject_type"`
	SubjectID   string         `json:"subject_id"`
	Summary     map[string]any `json:"summary"`
	RequestID   string         `json:"request_id"`
	CreatedAt   time.Time      `json:"created_at"`
}
