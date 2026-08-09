-- +goose Up

CREATE TABLE runtime_plan_revisions (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  base_revision_id text,
  graph_version integer NOT NULL DEFAULT 1 CHECK (graph_version > 0),
  patch_key text NOT NULL DEFAULT '',
  patch_reason text NOT NULL DEFAULT '',
  sop_id text NOT NULL,
  sop_version integer NOT NULL CHECK (sop_version > 0),
  sop_digest text NOT NULL,
  schema_version text NOT NULL,
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  customer_steps jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(customer_steps) = 'array'),
  limits jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(limits) = 'object'),
  compiled_at timestamptz NOT NULL,
  compiled_by text NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,digest),
  FOREIGN KEY (tenant_id,base_revision_id) REFERENCES runtime_plan_revisions(tenant_id,id)
);

CREATE TABLE runtime_plan_nodes (
  tenant_id uuid NOT NULL,
  revision_id text NOT NULL,
  node_key text NOT NULL,
  kind text NOT NULL,
  stage_id text NOT NULL DEFAULT '',
  gate_id text NOT NULL DEFAULT '',
  name text NOT NULL,
  input_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(input_refs) = 'array'),
  output_schema text NOT NULL,
  required_capabilities jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(required_capabilities) = 'array'),
  execution_modes jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(execution_modes) = 'array'),
  customer_step_id text NOT NULL DEFAULT '',
  side_effect_class text NOT NULL DEFAULT '',
  retry_max_attempts integer NOT NULL DEFAULT 1 CHECK (retry_max_attempts > 0),
  PRIMARY KEY (tenant_id,revision_id,node_key),
  FOREIGN KEY (tenant_id,revision_id) REFERENCES runtime_plan_revisions(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE runtime_plan_edges (
  tenant_id uuid NOT NULL,
  revision_id text NOT NULL,
  from_key text NOT NULL,
  to_key text NOT NULL,
  PRIMARY KEY (tenant_id,revision_id,from_key,to_key),
  FOREIGN KEY (tenant_id,revision_id) REFERENCES runtime_plan_revisions(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,revision_id,from_key) REFERENCES runtime_plan_nodes(tenant_id,revision_id,node_key) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,revision_id,to_key) REFERENCES runtime_plan_nodes(tenant_id,revision_id,node_key) ON DELETE CASCADE,
  CHECK (from_key <> to_key)
);

INSERT INTO runtime_plan_revisions(
  tenant_id,id,base_revision_id,graph_version,patch_key,patch_reason,sop_id,sop_version,sop_digest,
  schema_version,digest,customer_steps,limits,compiled_at,compiled_by
)
SELECT tenant_id,id,NULL,1,'','',sop_id,sop_version,sop_digest,schema_version,digest,customer_steps,limits,compiled_at,compiled_by
FROM runtime_job_plans;

INSERT INTO runtime_plan_nodes(
  tenant_id,revision_id,node_key,kind,stage_id,gate_id,name,input_refs,output_schema,
  required_capabilities,execution_modes,customer_step_id,side_effect_class,retry_max_attempts
)
SELECT p.tenant_id,p.id,n.node_key,n.kind,COALESCE(n.stage_id,''),COALESCE(n.gate_id,''),n.name,
       COALESCE(n.input_refs,'[]'::jsonb),n.output_schema,COALESCE(n.required_capabilities,'[]'::jsonb),
       COALESCE(n.execution_modes,'[]'::jsonb),COALESCE(n.customer_step_id,''),COALESCE(n.side_effect_class,''),
       GREATEST(COALESCE(n.retry_max_attempts,1),1)
FROM runtime_job_plans p
CROSS JOIN LATERAL jsonb_to_recordset(p.nodes) AS n(
  node_key text, kind text, stage_id text, gate_id text, name text, input_refs jsonb,
  output_schema text, required_capabilities jsonb, execution_modes jsonb, customer_step_id text,
  side_effect_class text, retry_max_attempts integer
);

INSERT INTO runtime_plan_edges(tenant_id,revision_id,from_key,to_key)
SELECT p.tenant_id,p.id,e.from_key,e.to_key
FROM runtime_job_plans p
CROSS JOIN LATERAL jsonb_to_recordset(p.edges) AS e(from_key text,to_key text);

ALTER TABLE runtime_job_runs DROP CONSTRAINT IF EXISTS runtime_job_runs_plan_revision_id_fkey;
ALTER TABLE runtime_job_runs DROP CONSTRAINT IF EXISTS runtime_job_runs_tenant_id_plan_revision_id_fkey;
ALTER TABLE runtime_job_runs
  ADD CONSTRAINT runtime_job_runs_plan_revision_id_fkey
  FOREIGN KEY (tenant_id,plan_revision_id) REFERENCES runtime_plan_revisions(tenant_id,id);

DROP TABLE runtime_job_plans;

DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY['runtime_plan_revisions','runtime_plan_nodes','runtime_plan_edges'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = current_setting(''app.tenant_id'', true)::uuid) WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true)::uuid)', table_name);
  END LOOP;
END $$;

REVOKE UPDATE,DELETE ON runtime_plan_revisions,runtime_plan_nodes,runtime_plan_edges FROM contentcloud_runtime;

-- +goose Down

CREATE TABLE runtime_job_plans (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  sop_id text NOT NULL,
  sop_version integer NOT NULL,
  sop_digest text NOT NULL,
  schema_version text NOT NULL,
  digest text NOT NULL,
  nodes jsonb NOT NULL,
  edges jsonb NOT NULL,
  customer_steps jsonb NOT NULL DEFAULT '[]'::jsonb,
  limits jsonb NOT NULL DEFAULT '{}'::jsonb,
  compiled_at timestamptz NOT NULL,
  compiled_by text NOT NULL,
  PRIMARY KEY (tenant_id,id), UNIQUE (tenant_id,digest)
);

INSERT INTO runtime_job_plans(tenant_id,id,sop_id,sop_version,sop_digest,schema_version,digest,nodes,edges,customer_steps,limits,compiled_at,compiled_by)
SELECT tenant_id,id,sop_id,sop_version,sop_digest,schema_version,digest,'[]'::jsonb,'[]'::jsonb,customer_steps,limits,compiled_at,compiled_by
FROM runtime_plan_revisions;

ALTER TABLE runtime_job_runs DROP CONSTRAINT IF EXISTS runtime_job_runs_plan_revision_id_fkey;
ALTER TABLE runtime_job_runs DROP CONSTRAINT IF EXISTS runtime_job_runs_tenant_id_plan_revision_id_fkey;
ALTER TABLE runtime_job_runs ADD CONSTRAINT runtime_job_runs_plan_revision_id_fkey FOREIGN KEY (tenant_id,plan_revision_id) REFERENCES runtime_job_plans(tenant_id,id);
DROP TABLE runtime_plan_edges;
DROP TABLE runtime_plan_nodes;
DROP TABLE runtime_plan_revisions;
