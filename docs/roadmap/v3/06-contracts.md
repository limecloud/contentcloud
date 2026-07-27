# V3 文件、Bundle 与页面投影契约

## 1. 设计原则

V3 的接口以业务对象 ID、版本和 digest 为边界，不以相对路径、文件名或 Codex 对话 ID 为边界。文件路径只存在于本地引用中；跨边界传输时必须变成 manifest。

## 2. Workspace Manifest

`.contentcloud/workspace.yaml` 是 V3 Workspace 的本地根契约：

```yaml
schema_version: contentcloud.workspace/3.0
workspace_id: ws_01H...
project_id: prj_01H...
layout_version: 3
context_version_id: pcv_01H...
environment_digest: sha256:...
created_at: 2026-07-27T00:00:00Z
```

字段规则：

- `workspace_id` 在 bootstrap 创建后不可更改。
- `project_id` 与 Workspace Credential、Submission 和 Assignment 必须一致。
- `layout_version=3` 是硬门禁；不是 V3 时 doctor 只给迁移计划，不在旧目录中就地运行 V3 Skill。
- `environment_digest` 必须和 `.contentcloud/environment.lock` 一致。
- 不记录本机绝对路径、token、PKCE verifier 或 Codex 对话 ID。

## 3. Source Registry

`20-sources/registry.yaml` 只登记原件和解析状态：

```yaml
schema_version: contentcloud.source-registry/3.0
sources:
  - id: source:brand-manual-20260423
    title: 沸氏香铺品牌手册20260423.pdf
    location:
      kind: workspace_file
      path: originals/brand-manual-20260423.pdf
    sha256: 23c0...
    mime_type: application/pdf
    source_kind: brand_manual
    ingest_status: extracted
    extraction_ref: extract:brand-manual-20260423@1
```

`location.kind` 允许 `workspace_file`、`external_readonly_ref`、`cloud_assignment`。无论位置如何，原件不由 Agent 修改；更新原件必须登记 Source 新版本。

## 4. Knowledge Page

每个 `30-knowledge/pages/**/*.md` 必须有 YAML frontmatter：

```yaml
---
id: fact:package-dimensions
type: FactAssertion
version: 2
status: conflicted
subject_refs: [sku:jinling-gudu-incense]
value:
  dimensions_mm: [68, 26, 182]
source_refs:
  - source:package-artwork-20251203#page=1
evidence_refs:
  - evidence:package-artwork-20251203:page-1:box-dimensions
decision_refs: []
supersedes: fact:package-dimensions@1
---
```

通用字段：`id`、`type`、`version`、`status`、`source_refs`、`evidence_refs`、`decision_refs`。类型 Schema 决定其他字段和状态机。

## 5. KnowledgePack

KnowledgePack 只保留对象引用和质量，不复制对象正文：

```yaml
schema_version: contentcloud.knowledge-pack/3.0
id: pack:jinling-gudu@1
status: candidate
methodology_version_id: methodology:product-rd@1
layers:
  identity: [brand:feishi-incense-shop, product:jinling-gudu]
  product: [fact:product-name, fact:package-dimensions]
  market: [audience:nanjing-traveler, scenario:travel-ending]
  expression: [claim:bring-nanjing-home, rule:use-source-artwork]
  operations: [process:produce-incense]
  content_engine: [campaign:douyin-scene-exploration]
  compliance: [rights:product-photo-dsc1343, conflict:package-dimensions]
quality:
  methodology_coverage: {covered: 15, required: 15}
  eligible_counts: {facts: 0, claims: 0, rights: 0}
  output_mode: blocked_creative_exploration_only
```

## 6. LocalRunContext 与 Handoff

`40-work/runs/<run-id>/context.json`：

```json
{
  "schema_version": "contentcloud.local-run/3.0",
  "run_id": "run_...",
  "intent_id": "intent:douyin-short-video",
  "stage": "compile",
  "status": "active",
  "context_revision": 8,
  "input_refs": [{"id":"pack:jinling-gudu@1","digest":"sha256:..."}],
  "eligible_ids": [],
  "blocked_ids": ["claim:medical-effects"],
  "output_refs": [],
  "checks": [],
  "history": []
}
```

`40-work/handoffs/<handoff-id>.json`：

```json
{
  "schema_version": "contentcloud.handoff/1.0",
  "handoff_id": "hnd_...",
  "run_id": "run_...",
  "context_revision": 8,
  "status": "ready",
  "input_refs": [{"id":"brief:travel-ending@2","digest":"sha256:..."}],
  "completed_checks": ["knowledge-lint"],
  "blockers": ["rights:product-photo-dsc1343"],
  "next_action": "创建仅用于创意评审的 blocked 批次"
}
```

所有引用对象都必须在接管前重新计算 digest；Handoff 不能包含 transcript、prompt、token 和未版本化的长文本。

## 7. ContentBatch Manifest

`50-production/batches/<batch-id>/manifest.yaml`：

```yaml
schema_version: contentcloud.content-batch/3.0
id: batch:douyin-first-10
intent_id: intent:douyin-short-video
brief_ref: brief:travel-ending@2
knowledge_snapshot_refs: [aps_knowledge_...]
status: blocked
publishable: false
content_item_refs:
  - draft:dy-jinling-01-travel-ending
blocked_reasons:
  - FactAssertion 尚未 verified。
  - Claim 尚未 approved。
  - 产品素材尚无 valid RightsRecord。
checks:
  - name: content-lint
    status: passed
```

`publishable=false` 时 `delivery` preflight 必须失败；`content_batch` publish 可以成功，以便进入创意评审。

## 8. SubmissionBundle

本地 publish 使用一个稳定 Bundle，不扫描并上传整个目录：

```json
{
  "bundle_version": "3.0",
  "submission_type": "knowledge",
  "project_id": "prj_...",
  "workspace_id": "ws_...",
  "base_snapshot_ids": ["aps_..."],
  "objects": [
    {"id":"claim:ancient-formula","type":"Claim","version":1,"digest":"sha256:...","path":"30-knowledge/pages/claims/ancient-formula.md","content":{"id":"claim:ancient-formula","status":"candidate"}}
  ],
  "source_disclosures": [
    {"source_ref":"source:product-copy","level":"evidence_pack","evidence_pack_ref":"evidence:..."}
  ],
  "local_run_summary": {
    "run_id":"run_...",
    "stage":"output_lint",
    "checks":[{"name":"knowledge-lint","status":"passed"}]
  },
  "environment_digest": "sha256:...",
  "idempotency_key": "..."
}
```

`path` 仅作本地可读提示，不作为服务端定位依据。`content` 是用户在 preflight 中确认披露、供审核使用的结构化正文；`digest` 必须由它复算得到。服务端校验对象摘要、类型 Schema、base snapshot、项目边界和来源披露后创建 Revision，原始来源文件仍不随对象正文上传。

## 9. Web Projection Contract

每个项目页面通过统一的 `ProjectProjection` 读取：

```json
{
  "project": {"id":"prj_...","stage":"initiation","gate":"in_progress"},
  "workspace": {"state":"connected","environment_health":"ready","last_pull_at":"..."},
  "coverage": {"methodology":"15/15","sources":"20/20","eligible":{"facts":0,"claims":0,"rights":0}},
  "next_actions": [{
    "id":"review-submission-rev_...",
    "kind":"review",
    "label":"处理待审核 Revision",
    "enabled":true,
    "navigation":{
      "view":"review",
      "focus":{"kind":"submission_revision","id":"rev_...","digest":"sha256:..."}
    }
  }],
  "work": {"assignment_count":1,"submitted_run_summary_count":2},
  "governance": {"pending_reviews":1,"blocked_content_batches":1}
}
```

各页面可以有专门 query，但不得再次拼装另一套项目状态。所有 `next_actions` 必须携带由共享 Page Contract 校验的类型化 `navigation`，定位到明确对象、决定、Assignment 或本地继续入口；Projection 不返回任意绝对 URL，Web 和 MCP 分别从同一目标构造宿主链接。V3 信息架构仍在开发，因此 view/focus 在联合评审前属于已实现草案，不是冻结契约。

## 10. ExecutionBundle

```json
{
  "bundle_id": "ceb_...",
  "project_id": "prj_...",
  "subject_refs": [{"id":"assignment:...","digest":"sha256:..."}],
  "capability_ids": ["contentcloud.content.script.compile@3"],
  "pack_refs": [{"plugin":"contentcloud-video-production","version":"0.7.0","digest":"sha256:..."}],
  "environment_digest": "sha256:...",
  "expires_at": "...",
  "signature": "..."
}
```

业务对象正文与 `ExecutionBundle` 永远分开。任何 Script 中出现的命令、URL 或“安装 Skill”文字都不得影响 Resolver。

## 11. PostgreSQL V3 Baseline

开发期数据库契约只有一个入口：`migrations/00001_v3_baseline.sql`。

- 空库直接创建当前 V3 表、RLS、不可变触发器和受控 lookup function。
- `contentcloud_schema_migrations` 只允许出现 `00001_v3_baseline.sql`；存在任何旧版本记录时迁移必须失败，并提示重建开发数据库。
- 不提供 V1/V2 backfill、字段别名、兼容 view、双写或在线升级路径。
- 正式事实链固定为 `SubmissionRevision -> Decision -> ApprovedSnapshot -> Artifact / Result / Projection`。
- `BenchmarkContent`、`ContentFramework`、`ShotPattern`、`SellingPoint`、`VisualizationPlan`、`BriefVersion`、`Script` 和 `ScriptVersion` 对应表均为 `dead`，不得恢复。
- schema 声明为 JSON 数组的列必须把缺省集合持久化为 `[]`，不能写入 JSON `null`。
