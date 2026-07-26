-- +goose Up

CREATE TABLE review_cycles (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  subject_type text NOT NULL,
  subject_id uuid NOT NULL,
  cycle_number integer NOT NULL CHECK (cycle_number > 0),
  status text NOT NULL CHECK (status IN ('open','approved','changes_requested','superseded')),
  conclusion text NOT NULL DEFAULT '',
  assignee_user_id text NOT NULL DEFAULT '',
  opened_by text NOT NULL,
  decided_by text NOT NULL DEFAULT '',
  opened_at timestamptz NOT NULL,
  decided_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(subject_id,cycle_number)
);

ALTER TABLE review_cycles ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_review_cycles ON review_cycles
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE INDEX review_cycles_subject_idx ON review_cycles(tenant_id,subject_id,cycle_number DESC);

ALTER TABLE review_comments
  ADD COLUMN review_cycle_id uuid REFERENCES review_cycles(id),
  ADD COLUMN carried_from_comment_id uuid REFERENCES review_comments(id);
CREATE INDEX review_comments_cycle_idx ON review_comments(tenant_id,review_cycle_id,created_at);

-- +goose Down
DROP INDEX IF EXISTS review_comments_cycle_idx;
ALTER TABLE review_comments DROP COLUMN IF EXISTS carried_from_comment_id, DROP COLUMN IF EXISTS review_cycle_id;
DROP TABLE IF EXISTS review_cycles;
