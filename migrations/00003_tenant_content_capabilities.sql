-- +goose Up

CREATE TABLE tenant_content_capabilities (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  content_type text NOT NULL CHECK (content_type IN ('wechat_article')),
  enabled boolean NOT NULL DEFAULT false,
  updated_by uuid NOT NULL REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,content_type)
);

CREATE INDEX tenant_content_capabilities_enabled_idx
  ON tenant_content_capabilities(content_type,tenant_id)
  WHERE enabled;

-- +goose Down

DROP TABLE IF EXISTS tenant_content_capabilities;
