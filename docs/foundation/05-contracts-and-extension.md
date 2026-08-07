# 契约、版本与创作流水线扩展规范

状态：`目标规范`。

更新时间：2026-08-07。

## 1. 目的

平台可扩展性来自稳定契约，不来自无限配置。新增创作流水线必须通过版本化业务包、体验模板、SOP、Schema、能力和执行绑定进入平台，不能修改 Runtime 以识别具体内容类型。

## 2. 契约分类

| 类型 | 示例 | 兼容责任 |
| --- | --- | --- |
| 业务 Schema | InspirationQuery、Persona、Script、StoryboardPackage | 业务包所有者 |
| 产品契约 | ExperienceTemplate、CustomerStep、CustomerAction、CreativeAssetCatalogItem | Studio / Experience 所有者 |
| 业务引用契约 | SourceRevisionRef、ApprovedSnapshotRef、ArtifactRef、CreativeAssetRef | 对应事实拥有域 + 使用方 |
| 流水线契约 | SOPVersion、StageDefinition、GateDefinition | Catalog 所有者 |
| Runtime 契约 | JobPlanRevision、NodeResult、StateMutation、Effect | Runtime 所有者 |
| 执行契约 | TaskContract、ContextView、Lease、Heartbeat | Runtime + Integration 所有者 |
| 外部集成契约 | SearchResult、ProviderRequest、Callback、Receipt | Connector / Provider 所有者 |
| API 契约 | Studio BFF、Operations BFF、CLI envelope | 对应接口所有者 |
| 事件契约 | JobEvent、AuditEvent、Projection cursor | 事件生产者所有者 |

当前代码证据：`internal/agentadapter/harness.go` 提供 `AgentHarnessAdapter`、能力探测、结构化事件流和 FakeHarness；`internal/runtime/context.go` 只从引用和策略构建不可变 `ContextView`；`internal/runtime/agent.go` 与迁移 `00015_runtime_agent_instances.sql` 已实现 ContextView/AgentInstance 持久化及父子权限收敛；`internal/runtime/graph_patch.go` 只负责受限 GraphPatch 的纯校验与新计划摘要。上述实现仍不等同于真实 SDK 会话恢复、资源让出/恢复调度或动态图生产切流。

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

现有契约名称在迁移期保持兼容，不为统一外观进行无价值重命名。

### 3.2 兼容变更

- 新增可选字段或枚举的可忽略值：minor。
- 删除字段、改变语义、收紧已接受输入或改变摘要算法：major。
- 任何消费者必须拒绝不支持的 major，不能静默按旧语义处理。
- 生产者在兼容期继续生成旧 major，直到消费者覆盖率达到退场门槛。
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

## 5. 创作结果资产目录与引用

`CreativeAssetCatalogItem` 是现有兼容契约名称，语义由 ADR-0013 收紧为“客户创作结果目录项”。它是 `CreativeResultAssetProjection` 的可重建行模型，不得再收录来源、灵感、知识、参考素材、权利记录或交付包，也不得并行创建语义相同的 `CreativeResultAssetCatalogItem` 第二套契约。

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
- 来源、灵感、知识、参考素材和权利记录使用 `InputRef` 或项目参考契约；交付包使用交付投影。三者不能复用结果资产类型字段。
- Runtime 和 WorkTask 使用 `subject_*` 和摘要，不把目录项状态作为权威。
- 创建任务时回源校验租户、版本、权利、用途和敏感等级。
- 结果对象版本映射必须确定：KnowledgeSnapshot 使用 Digest，TaskRevision 和 ApprovedSnapshot 使用 ContentHash，Artifact 使用 SHA-256。DeliveryPackage 只用于交付视图和推导 `delivered` 状态，不进入结果目录成为新资产类型。
- SourceRevision、Asset、KnowledgeObject 和 RightsRecord 只作为内部 lineage、权利校验或项目参考事实，不生成客户结果目录项。
- 目录投影 Schema 与引用 Schema 分别版本化；目录展示字段 minor 变更不能改变引用语义。
- 新增可收录对象类型前必须定义事实所有者、版本、失效、权限和重建规则。

若已发布旧版目录 Schema 曾允许输入型对象，停止收录属于语义收紧，必须通过新的 major、兼容读取映射和明确退场指标迁移，不能静默改变旧消费者行为。

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

输出必须先验证 tenant、attempt、lease、Schema、大小、摘要和引用。执行者不能直接把 NodeRun 标为成功；Runtime 在业务结果持久化后完成状态转移。

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
10. 运营预览 -> Canary -> 租户启用
11. 观测客户价值、资产复用、成本和故障后扩大范围
```

进入生产前必须用第二条结构不同的流程验证：新增内容类型不需要修改 Runtime 状态机、调度表或客户 Shell 基础设施。

## 14. 废弃规范

- 标记废弃版本、替代版本、最后创建时间和最晚移除版本。
- 对仍在运行的固定旧版本继续只读或完成支持。
- 新任务不得绑定已停用版本。
- 兼容读写期间记录调用量和租户覆盖率。
- 移除前必须证明零活跃绑定、历史可读、回退可行和迁移测试通过。
