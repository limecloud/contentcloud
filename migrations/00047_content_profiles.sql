-- +goose Up

ALTER TABLE tenant_content_capabilities
  DROP CONSTRAINT tenant_content_capabilities_content_type_check,
  ADD CONSTRAINT tenant_content_capabilities_content_type_check
    CHECK (content_type IN ('marketing_video','wechat_article','serialized_novel'));

ALTER TABLE brand_projects
  DROP CONSTRAINT brand_projects_content_type_check,
  ADD CONSTRAINT brand_projects_content_type_check
    CHECK (content_type IN ('video_script','marketing_video','wechat_article','serialized_novel'));

-- +goose Down

ALTER TABLE brand_projects
  DROP CONSTRAINT brand_projects_content_type_check,
  ADD CONSTRAINT brand_projects_content_type_check
    CHECK (content_type IN ('video_script','marketing_video','wechat_article'));

ALTER TABLE tenant_content_capabilities
  DROP CONSTRAINT tenant_content_capabilities_content_type_check,
  ADD CONSTRAINT tenant_content_capabilities_content_type_check
    CHECK (content_type IN ('marketing_video','wechat_article'));
