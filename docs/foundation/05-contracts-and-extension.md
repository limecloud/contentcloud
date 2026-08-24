# 契约、版本与创作流水线扩展规范

状态：`目标规范；Registry 持久化、发布和租户范围控制已实现首切片`。

更新时间：2026-08-23。

## 1. 目的

平台可扩展性来自稳定契约，不来自无限配置。新增创作流水线必须通过版本化业务包、体验模板、SOP、Schema、能力和执行绑定进入平台，不能修改 Runtime 以识别具体内容类型。

## 2. 契约分类

| 类型 | 示例 | 兼容责任 |
| --- | --- | --- |
| 业务 Schema | InspirationQuery、Persona、Script、StoryboardPackage | 业务包所有者 |
| 产品契约 | ExperienceTemplate、CustomerStep、CustomerAction、WorkspaceFolderItem、WorkspaceMaterialItem、CreativeAssetCatalogItem | Studio / Experience 所有者 |
| 业务引用契约 | SourceRevisionRef、WorkspaceMaterialRef、ApprovedSnapshotRef、ArtifactRef、CreativeAssetRef | 对应事实拥有域 + 使用方 |
| 流水线契约 | SOPVersion、StageDefinition、GateDefinition | Catalog 所有者 |
| Runtime 契约 | JobPlanRevision、NodeResult、StateMutation、Effect | Runtime 所有者 |
| 执行契约 | TaskContract、ContextView、Lease、Heartbeat | Runtime + Integration 所有者 |
| 外部集成契约 | SearchResult、ProviderRequest、Callback、Receipt | Connector / Provider 所有者 |
| API 契约 | Studio BFF、Operations BFF、CLI envelope | 对应接口所有者 |
| 事件契约 | JobEvent、AuditEvent、Projection cursor | 事件生产者所有者 |

当前代码证据：`internal/integration/agent/harness.go` 提供 `AgentHarnessAdapter`、能力探测、结构化事件流和 FakeHarness；`internal/integration/agent/codex_harness.go` 使用 Codex CLI JSONL 协议保存真实 thread ID，并通过 `codex exec resume <thread_id>` 支持跨 worker 进程恢复；`internal/integration/agent/claude_harness.go` 使用 Claude `stream-json`、真实 `session_id` 和 `--resume` 支持跨 Harness 实例恢复；`internal/runtime/context.go` 只从引用和策略构建不可变 `ContextView`；`internal/runtime/agent.go` 与迁移 `00015_runtime_agent_instances.sql` 已实现 ContextView/AgentInstance 持久化及父子权限收敛；`internal/runtime/graph_patch.go` 只负责受限 GraphPatch 的纯校验与新计划摘要。Runtime 的 Yield/Resume 已落地，但真实 Codex/Claude 在线冒烟、Provider 端到端和动态图生产切流仍是独立门槛。

## 3. 版本规则

### 3.1 Schema 标识

使用稳定命名和显式版本：

```text
contentcloud.<domain>.<object>/<major>.<minor>

contentcloud.inspiration.query/1.0
contentcloud.runtime.node-result/1.0
contentcloud.experience.template/1.0
contentcloud.experience.creative-asset-catalog/1.0
contentcloud.work.creative-asset-ref/1.0
```

当前尚无外部生产消费者；内部契约按目标语义一次性切换，不保留旧名称、别名或双写。已经真实对外发布的契约未来必须通过独立 ADR 决定版本窗口，不能把这条例外扩展到内部 Go 包、开发 Fixture 或未发布 API。

### 3.2 兼容变更

- 新增可选字段或枚举的可忽略值：minor。
- 删除字段、改变语义、收紧已接受输入或改变摘要算法：major。
- 任何消费者必须拒绝不支持的 major，不能静默按旧语义处理。
- 当前整改中的生产者只生成目标 major；旧 major 的代码、Fixture 和正向测试同步删除。
- Schema 文件、Go 类型、OpenAPI 和示例 Fixture 必须在同一变更中更新。

### 3.3 摘要与固定

正式对象使用规范化序列化和 SHA-256 摘要。任务开始时至少固定：

```text
experience_template_id + version + digest
sop_id + version + digest
job_plan_revision_id + digest
execution_binding_digest
input_snapshot_refs + digests
contract major/minor versions
```

不得使用“当前最新版本”作为进行中任务的隐式输入。

## 4. ExperienceTemplate

体验模板把平台能力包装成客户可使用的创作产品：

```text
ExperienceTemplateVersion
├── id / version / status / digest
├── customer_name / content_type / availability
├── input_form_schema_ref
├── customer_steps[]
│   ├── title / outcome / visibility
│   ├── runtime_stage_refs[]
│   └── result_presentation_ref
├── customer_actions[]
├── published_sop_ref
├── required_capabilities[]
├── gate_policy_refs[]
├── tenant_eligibility
└── release metadata
```

体验模板只定义客户步骤映射，不保存执行状态。客户步骤可聚合多个 NodeRun，其状态由投影确定性计算。

体验原语首阶段限制为：表单输入、资料选择、候选列表、版本比较、人工确认、媒体预览和交付下载。超出原语的复杂体验通过版本化业务 feature 实现，不扩展成任意页面配置语言。

### 4.1 Business Workbench Plugin 1.0.0

业务工作台插件是客户产品面的声明式扩展。它描述某个业务如何组织客户导航、阶段、画布和主要动作，但不拥有业务事实，也不成为新的执行入口。插件必须绑定一个已经发布并通过运营门禁的 `ExperienceTemplate`，不能绕过模板直接把页面暴露给租户。

插件包的最小结构如下：

```text
workbench-plugins/<id>/
├── workbench.json       版本化 manifest，唯一入口
├── previews/            可选静态预览资源
└── fixtures/            manifest 与渲染契约测试输入
```

`workbench.json` 由 [`workbench-plugin.schema.json`](../../contracts/workbench-plugins/1.0.0/workbench-plugin.schema.json) 约束，服务端解析器位于 `internal/experience/workbench`。1.0.0 只允许以下声明：

```text
id / version / name / content_types
experience.template_id
ui.renderer = approved
ui.layout / ui.density / ui.theme
ui.navigation[] / ui.stages[] / primary_action
```

以下内容明确不属于业务工作台插件：

- Go 服务端代码、数据库迁移、任意 API URL 或自定义状态写路径。
- Provider 凭据、模型配置、预算策略、Runtime 状态机和执行租约。
- 任意浏览器脚本、远程 iframe、上传的前端 bundle 或可执行文件。
- Agent Plugin 的安装、Skill/MCP 发现和宿主权限。Agent Plugin 仍由 `plugins/` 与宿主安装边界管理，不能用业务工作台 manifest 替代。

#### Registry 生命周期

```text
提交插件包
  -> manifest/schema 校验
  -> ExperienceTemplate、action、content_type 和主题白名单校验
  -> 运营预览（不影响租户）
  -> 发布不可变版本与 digest
  -> published（新任务可选）
  -> retired（正常退役，可恢复；历史任务继续使用）
  -> revoked（安全撤销，不可恢复；历史任务也阻断）
  -> Customer BFF 按租户返回可用工作台
  -> 新任务固定 plugin + template 版本，后续不随 latest 漂移
```

Registry 必须保存 `plugin_id`、版本、manifest、摘要、状态、模板别名、租户启用范围和生命周期原因；发布者、请求号和状态变化记录进入平台审计。启用前要检查模板仍为 `published`，所有 `primary_action` 都存在于服务端 action contract，且同一租户作用域内每个 `content_type` 只有一个默认工作台。未知 major 版本拒绝加载；minor 版本只有在解析器声明支持时才可进入预览或租户启用。

当前实现证据：`internal/experience/workbench` 已完成 manifest 白名单、批准 `primary_action` 闭合校验和首方 Registry；`migrations/00054_workbench_registry.sql` 建立平台范围的 `workbench_plugin_versions`；`internal/persistence/memory` 与 `internal/persistence/postgres` 实现 `WorkbenchRepository`，并有 `workbench_integration_test.go` 作为真实 PostgreSQL 入口；`/api/bff/admin/workbenches` 提供平台管理员清单、登记草稿和状态更新；后台“业务工作台”页面显示版本、摘要、业务类型、租户范围和生命周期原因。空表时使用首方声明作为安全回退；持久化版本必须使用新的 `plugin_id@version`，不能覆盖首方同名版本，租户定制的已发布版本优先于平台默认版本。

`retired` 只影响新任务和工作台选择器，不改变已经固定版本的进行中任务；恢复时必须重新通过发布门禁。`revoked` 用于签名、来源或安全契约失效，对新旧任务都失败关闭；进入该状态后只能用相同租户范围和相同原因进行幂等重试，不能恢复或改写原因。完整状态图见[业务工作台 Registry 生命周期](../../diagrams/contentcloud-workbench-registry-lifecycle.svg)。如果模板、action 或主题资源校验失败，宿主必须显示明确的不可用状态，并回退到平台安全错误页，不能静默套用另一个业务工作台。

#### 前端装配边界

```text
apps/web/src/platform       会话、租户、权限、错误和无业务 UI 原语
apps/web/src/workbench      宿主、manifest 类型、registry 查询和 action contract
apps/web/src/workbenches    视频、文章、电商、连载小说等业务 feature
apps/web/src/studio         迁移期旧客户路由，不再新增业务专属组件
```

`workbench/` 只提供宿主能力，不能把所有业务压成同一个导航或画布。`workbenches/<business>/` 可以拥有自己的信息架构、对象语言、阶段表达、密度和主题 token，但只能通过平台契约访问会话、项目、资产、审核、版本和交付。`shared/` 只放没有业务所有权的 UI 原语，不能成为新的 `common-business` 汇聚点。

业务工作台插件与 Agent Plugin 的关系是引用关系，不是继承关系：ExperienceTemplate/Capability Binding 可以指定某个受批准的 Agent Plugin 或执行者能力，客户插件本身不能安装、调用或替换 Agent。

## 4.2 跨业务内容的事实收敛

业务 Schema 只定义工作台对象和渲染字段，不定义第二套流程。文章使用 `contentcloud.article/1.0`，电商使用 `contentcloud.commerce-content/1.0`，普通视频脚本使用兼容契约 `contentcloud.video_script/1.0`，小说章节使用 `contentcloud.novel-chapter/1.0`；正式提交都进入 `content_batch -> SubmissionRevision`，批准后统一生成 `ApprovedSnapshot`、三格式 `Artifact` 和幂等 `DeliveryPackage`。旧任务版本 API 的兼容响应由 Submission 投影生成，禁止新写入绕过 Review 或直接生成交付文件。普通视频脚本仅在历史流程已通过全部 Gate 的 accepted 任务上记录 `automated_gate` 批准，保留既有语义但仍使用平台审批事实。未来业务插件必须沿用该契约组合，并为自己的对象定义稳定 ID、版本、摘要、阻断项和缺失输入集合。

`serialized_novel` 已形成首个完整插件纵向切片：首方 Registry 发布 `novel-editor`，业务 feature 位于 `apps/web/src/workbenches/serialized-novel`，章节校验位于 `internal/local/workspace/novel.go`，正式提交、批准、交付和效果分别复用 Submission、Review、ApprovedSnapshot、Artifact、Delivery 和 Performance。章节只有 `review_ready` 可提交，`approved_snapshot_id` 只能出现在批准后的服务器投影中，工作台不能自行写入。该完成状态只代表平台内部事实链，不代表真实内容商店或连载渠道已经接通。

模型 Provider 生成的候选是执行层的 `draft` 临时事实：`ModelGenerationReceipt` 固定 Provider、请求摘要、响应摘要和用量，候选本身不具备审批、交付或效果归因资格。只有业务工作台或兼容提交入口重新校验并创建 `SubmissionRevision` 后，内容才进入正式事实链。

## 5. 客户资产入口契约

客户 BFF 组合 `WorkspaceMaterialProjection` 与 `CreativeResultAssetProjection`，但两套契约分别版本化。文件夹、文件处理、结果确认和交付不能塞进一个包含大量可空字段的通用 `AssetItem`。

```text
WorkspaceFolderItem
├── folder_ref / parent_ref
├── name / project_scope / child_count
└── created_at / updated_at

WorkspaceMaterialItem
├── material_ref / folder_ref
├── material_kind / origin / usage
├── title / mime_type / size / preview_ref
├── processing_state / rights_summary
└── created_at / updated_at

WorkspaceMaterialRef
├── material_ref / version / digest
├── usage_intent / target_task_ref
└── validation_snapshot
```

工作区资料规则：

- `material_kind`、`origin`、`usage` 和 `processing_state` 是独立维度。
- 文件夹只表达组织关系，不能作为任务输入或权利事实。
- 上传、导入和登记必须产生稳定资料身份；加入任务时固定具体版本和摘要。
- OCR、转写、摘要和标签是可重建派生物，不覆盖原文件，不自动产生权利或批准结论。
- 当前 `CreativeAssetCatalogItem` / `StudioAssetItem` 不增加这些字段。

### 5.1 创作结果目录与引用

`CreativeAssetCatalogItem` 是当前契约名称，语义由 ADR-0013 收紧为“客户创作结果目录项”。它是 `CreativeResultAssetProjection` 的可重建行模型，不得再收录来源、灵感、知识、参考素材、权利记录或交付包，也不得并行创建语义相同的 `CreativeResultAssetCatalogItem` 第二套契约。

```text
CreativeAssetCatalogItem
├── catalog_item_id / result_type / display
├── project_ref / source_task_ref
├── subject_ref + version + digest
├── status / reusable / blocking_reasons[]
├── visibility / preview_ref
└── internal_lineage_ref / generated_at / projection_cursor
```

`CreativeAssetRef` 是任务输入契约：

```text
CreativeAssetRef
├── catalog_item_id              产品追溯，可选
├── subject_type / subject_id
├── subject_version_id / digest
├── usage_intent / target_channel
└── validation_snapshot
```

规则：

- 目录项只收录人物原型、剧本、分镜、图片和视频等流水线生成结果；只引用拥有域事实，不复制正文或媒体。
- `result_type` 与 `status` 是两个独立维度。类型固定为 `persona / script / storyboard / image / video`；状态使用有限集合 `draft / pending_confirmation / changes_requested / confirmed / delivered / superseded / blocked`。
- 只有 `confirmed` 和 `delivered` 结果可以被新任务正式复用；浏览器提交的 `reusable` 不可信，服务端必须重新计算。
- 搜索候选、灵感、知识、来源证据和权利记录使用 `InputRef` 或项目参考契约；客户明确上传/导入的资料使用 `WorkspaceMaterialRef`；交付包使用交付投影。它们不能复用结果资产类型字段。
- Runtime 和 WorkTask 使用 `subject_*` 和摘要，不把目录项状态作为权威。
- 创建任务时回源校验租户、版本、权利、用途和敏感等级。
- 结果对象版本映射必须确定：KnowledgeSnapshot 使用 Digest，TaskRevision 和 ApprovedSnapshot 使用 ContentHash，Artifact 使用 SHA-256。DeliveryPackage 只用于交付视图和推导 `delivered` 状态，不进入结果目录成为新资产类型。
- SourceRevision、KnowledgeObject 和 RightsRecord 只作为内部 lineage、权利校验或项目参考事实，不生成客户结果目录项。客户明确上传/导入形成的 Asset 引用可以进入工作区资料投影，但仍不能伪装成生成结果。
- 目录投影 Schema 与引用 Schema 分别版本化；目录展示字段 minor 变更不能改变引用语义。
- 新增可收录对象类型前必须定义事实所有者、版本、失效、权限和重建规则。

当前未发布旧版目录 Schema；停止收录输入型对象时直接更新 major、消费者、Fixture 和测试并删除旧读取映射。未来若存在真实外部消费者，必须另立 ADR 后才允许限时版本窗口。

## 6. SOP 与 JobPlanRevision

现有 `SOPVersion`、`StageDefinition` 和 `GateDefinition` 继续作为流水线定义来源：

```text
SOPVersion
  -> validate stage order, schemas, capabilities, gates
  -> compile immutable nodes and edges
  -> calculate plan digest
  -> produce JobPlanRevision
```

编译器必须验证：

- 节点和 Gate ID 唯一。
- 输入 Schema 可从上游或固定输入到达。
- 图无环，规模、深度和动态扩展不超过上限。
- 所需能力存在已批准实现或允许人工节点。
- Gate 引用有效，拒绝和修改路径明确。
- 输出可以关联到拥有该业务事实的领域。
- 客户步骤映射覆盖所有客户可见阻断和决定。

只有多个真实业务流证明 SOP 无法表达稳定语义时，才新增 PipelineDefinition 持久化对象。

## 7. 能力注册与执行绑定

### 7.1 Capability

```text
Capability
├── id / version / digest
├── input_schema / output_schema
├── execution_modes[]
├── data_classification
├── side_effect_class
├── cost_model
├── timeout / limits
└── health / availability
```

能力 ID 使用业务动词，不包含供应商名称：

- `source.search`
- `source.fetch`
- `insight.propose`
- `persona.propose`
- `content.script.propose`
- `storyboard.compose`
- `media.video.generate`
- `delivery.package.build`

### 7.2 ExecutionBindingSnapshot

执行绑定根据以下输入产生并固定：

- 租户与项目策略。
- 数据位置和披露等级。
- 执行者批准状态、版本、平台和区域。
- 网络出口、工具白名单和隔离等级。
- 预算、并发、优先级和健康状态。
- 是否需要用户已有登录会话。

自动回退只能使用已发布策略，且不能扩大数据披露、权限、副作用或预算。否则节点进入明确阻断或人工决定。

## 8. 业务包规范

每个创作流水线业务包包含：

```text
CreativePack
├── manifest
│   ├── id / version / compatible_runtime
│   ├── schemas[]
│   ├── capabilities[]
│   ├── sop_templates[]
│   ├── checks[] / gates[]
│   └── presentation_profiles[]
├── contracts/
├── deterministic validators/
├── optional agent skills/
├── fixtures/
├── contract tests/
└── migration notes/
```

业务包不能：

- 直接读写 Runtime 数据库。
- 注册任意脚本为服务端执行能力。
- 绕过平台认证、预算、Gate 和 Artifact 校验。
- 把模型 Prompt 作为唯一输出契约。
- 依赖某个 Agent 的私有任务列表或聊天格式。

## 9. 节点执行契约

### 9.1 输入

```text
NodeExecutionContract
├── contract_version
├── job_id / node_id / attempt_id
├── tenant_scope
├── capability + fixed version/digest
├── context_view_ref + digest
├── allowed_tools[]
├── input_schema / output_schema
├── budget / deadline / lease
└── idempotency_key
```

### 9.2 输出

```text
NodeResult
├── contract_version
├── job_id / node_id / attempt_id
├── status
├── output_refs[]
├── candidate_payload_ref
├── warnings[]
├── usage
├── provenance
└── result_digest
```

输出必须先验证 tenant、attempt、lease、Schema、大小、摘要和引用，并把结构化业务 payload 固定为内容寻址 Blob。执行者不能直接把 NodeRun 标为成功；Runtime 原子完成终态和事件/outbox receipts，业务拥有域再由持久化 subscriber 从 output ref 幂等物化对象。

## 10. API 规范

### 10.1 命令

- 所有会产生状态变化的 API 使用显式命令语义。
- 请求携带幂等键、预期版本或决定摘要。
- 成功返回新版本、允许动作和支持关联 ID。
- 冲突返回当前版本和恢复建议，不用通用 500。
- 高风险动作返回影响摘要，并要求客户端提交同一摘要确认。

### 10.2 查询

- 客户查询返回业务 DTO，不泄露 Runtime 内部对象。
- 运营查询返回诊断 DTO，但密钥、完整 ContextView 和本地绝对路径仍脱敏。
- 大列表使用稳定游标和明确 page size 上限。
- 投影返回生成时间和游标，客户端可以识别延迟。
- 资产选择器只返回与目标租户、项目、用途和渠道兼容的目录项；服务端仍在命令时回源校验。

### 10.3 错误 envelope

```json
{
  "error": {
    "code": "STUDIO_ACTION_CONFLICT",
    "message": "结果已经更新，请查看当前版本后重新确认",
    "cause": "submitted_digest_mismatch",
    "recovery": "reload_current_revision",
    "support_reference": "sup_xxx"
  }
}
```

客户 message 使用业务语言；运营和开发者接口可以额外返回安全的技术原因。任何错误都至少说明问题、原因和恢复方法。

## 11. 事件规范

- 事件是已经发生的事实，名称使用过去时。
- 每个 Job 内有单调递增序号或可验证顺序。
- 包含 tenant、actor、correlation、causation、schema version 和发生时间。
- 事件 payload 只包含小型稳定字段和引用，不包含密钥、完整外部响应和大正文。
- 消费者按 event ID 幂等，未知 minor 字段可忽略，未知 major 拒绝。
- 事件重放只能重建读模型，不调用执行者或外部服务。

## 12. 外部连接器规范

连接器必须实现：

- 明确输入输出 Schema。
- 超时、限流、分页和最大响应限制。
- 凭据 SecretRef，不把明文写入业务对象。
- 稳定请求 ID、幂等键和外部操作记录。
- 回调签名、重放保护和乱序处理。
- 结果不明对账能力或明确人工处理路径。
- Fixture、契约测试和低预算测试环境。
- 数据区域、保留、训练使用和披露政策元数据。

## 13. 新流水线扩展流程

```text
1. 定义客户结果和停止条件
2. 选择或新增版本化业务 Schema
3. 复用 SOP / Stage / Gate 定义流水线
4. 声明所需能力，不指定供应商
5. 实现或批准业务包和连接器
6. 编译 JobPlanRevision 并运行静态检查
7. 用 Fixture 完成契约和故障测试
8. 定义哪些生成结果进入统一资产目录，以及确认门禁和复用状态如何推导
9. 建立 ExperienceTemplate 客户投影
10. 编写并校验 Business Workbench Plugin manifest，绑定已发布 ExperienceTemplate
11. 运营预览 -> Canary -> 租户启用
12. 观测客户价值、资产复用、成本和故障后扩大范围
```

进入生产前必须用第二条结构不同的流程验证：新增内容类型不需要修改 Runtime 状态机、调度表或客户 Shell 基础设施。

## 14. 废弃规范

- 标记废弃版本、替代版本、最后创建时间和最晚移除版本。
- 对仍在运行的固定旧版本继续只读或完成支持。
- 新任务不得绑定已停用版本。
- 兼容读写期间记录调用量和租户覆盖率。
- 移除前必须证明零活跃绑定、历史可读、回退可行和迁移测试通过。
业务工作台 manifest 的 `ui.panels` 是受限的声明式扩展：它表达业务面板标题、对象语言、阶段映射和入口目标，仍由宿主批准 renderer 负责布局；它不能携带任意 React bundle、远程 iframe、Provider 配置或第二套任务/审批/产物状态。
