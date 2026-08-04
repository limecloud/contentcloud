-- +goose Up

ALTER TABLE tenant_content_capabilities
  DROP CONSTRAINT tenant_content_capabilities_content_type_check,
  ADD CONSTRAINT tenant_content_capabilities_content_type_check
    CHECK (content_type IN ('marketing_video','wechat_article'));

ALTER TABLE brand_projects
  ADD COLUMN content_type text NOT NULL DEFAULT 'marketing_video'
  CHECK (content_type IN ('video_script','marketing_video','wechat_article'));

-- +goose Down

ALTER TABLE brand_projects DROP COLUMN IF EXISTS content_type;

-- Keep the additive tenant capability enum to avoid deleting tenant data.
