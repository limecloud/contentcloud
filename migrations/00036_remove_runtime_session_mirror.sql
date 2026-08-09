-- +goose Up

DROP TABLE IF EXISTS runtime_agent_session_events;
DROP TABLE IF EXISTS runtime_agent_sessions;

-- +goose Down

-- The retired session mirror is intentionally not recreated. RuntimeAttempt
-- session_ref and the provider-owned Codex thread are the recovery boundary.
