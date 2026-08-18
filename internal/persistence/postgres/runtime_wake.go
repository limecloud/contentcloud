package postgres

import (
	"context"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

const runtimeWakeChannel = "contentcloud_runtime_wake"

func (s *Store) PublishRuntimeWake(ctx context.Context, tenantID string) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return fault.Invalid("RUNTIME_WAKE_TENANT_REQUIRED", "Runtime wake 缺少租户范围")
	}
	_, err := s.pool.Exec(ctx, `SELECT pg_notify($1,$2)`, runtimeWakeChannel, tenantID)
	return err
}

func (s *Store) ListenRuntimeWakes(ctx context.Context, notify func(string)) error {
	connection, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `LISTEN `+runtimeWakeChannel); err != nil {
		return err
	}
	for {
		event, err := connection.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		if tenantID := strings.TrimSpace(event.Payload); tenantID != "" && notify != nil {
			notify(tenantID)
		}
	}
}
