# V2 领域与数据模型

## 1. 建模原则

1. 本地工作区是未发布草稿事实源；云端 Submission、人工决定和 ApprovedSnapshot 是跨团队治理事实源。
2. 需要审批、复现或交付的内容使用不可变版本；可编辑聚合只保存当前指针。
3. 所有对象带 `tenant_id`；项目内对象同时带 `project_id`，数据库查询不能依赖调用方自行补租户条件。
4. 来源、证据、事实、主张、素材和权利分离，市场结构与品牌事实分离。
5. V2 只增加当前九域和剧本闭环需要的抽象，不建设通用低代码实体系统。
6. 客户端只能通过不可变 Submission 提交内容；云端状态变化由领域服务校验并写入 append-only AuditEvent。

## 2. 聚合关系

```mermaid
erDiagram
    TENANT ||--o{ MEMBERSHIP : has
    TENANT ||--o{ CLIENT_ACCOUNT : serves
    TENANT ||--o{ TENANT_SERVICE_TEMPLATE : owns
    CLIENT_ACCOUNT ||--o{ BRAND : owns
    BRAND ||--o{ PRODUCT : offers
    PRODUCT ||--o{ BRAND_PROJECT : scopes
    BRAND_PROJECT ||--o{ PROJECT_ASSIGNMENT : assigns
    BRAND_PROJECT ||--o{ WORKSPACE_BINDING : binds
    BRAND_PROJECT ||--o{ SUBMISSION : receives
    SUBMISSION ||--o{ SUBMISSION_REVISION : versions
    SUBMISSION_REVISION ||--o{ SOURCE_DISCLOSURE : discloses
    SUBMISSION_REVISION ||--o{ REVIEW_CYCLE : reviewed_by
    SUBMISSION_REVISION ||--o| APPROVED_SNAPSHOT : publishes
    BRAND_PROJECT ||--o{ PROJECT_CONTEXT_SNAPSHOT : freezes
    BRAND_PROJECT ||--o{ AUTOMATION_PLAN : configures

    BRAND_PROJECT ||--o{ SOURCE : contains
    SOURCE ||--o{ SOURCE_REVISION : versions
    SOURCE_REVISION ||--o{ EVIDENCE_SPAN : locates
    EVIDENCE_SPAN }o--o{ KNOWLEDGE_ITEM : supports
    BRAND_PROJECT ||--o{ ASSET : contains
    ASSET ||--o{ RIGHTS_RECORD : governed_by

    BRAND_PROJECT ||--o{ RESEARCH_TASK : requests
    RESEARCH_TASK ||--o{ BENCHMARK_CASE : produces
    BENCHMARK_CASE ||--o{ MARKET_INSIGHT : derives
    BRAND_PROJECT ||--o{ STRATEGY : owns
    STRATEGY ||--o{ STRATEGY_VERSION : versions
    STRATEGY_VERSION ||--o{ VISUALIZATION_PLAN : selects

    BRAND_PROJECT ||--o{ CONTENT_PLAN : plans
    CONTENT_PLAN ||--o{ CAMPAIGN : groups
    CAMPAIGN ||--o{ EXPERIMENT_PLAN : tests
    CAMPAIGN ||--o{ BRIEF : owns
    BRIEF ||--o{ BRIEF_VERSION : versions

    BRIEF_VERSION ||--o{ CREATIVE_BATCH : requests
    CREATIVE_BATCH ||--o{ SCRIPT : creates
    SCRIPT ||--o{ SCRIPT_VERSION : versions
    SCRIPT_VERSION ||--o{ SHOT : contains

    REVIEW_CYCLE ||--o{ REVIEW_COMMENT : contains
    SUBMISSION_REVISION ||--o{ APPROVAL_DECISION : binds
    SUBMISSION_REVISION ||--o{ REVIEW_GRANT : shares
    APPROVED_SNAPSHOT }o--o{ DELIVERY_PACKAGE : packages
    DELIVERY_PACKAGE ||--o{ PRODUCTION_HANDOFF : hands_off
    APPROVED_SNAPSHOT ||--o{ PERFORMANCE_OBSERVATION : measures
    PERFORMANCE_OBSERVATION }o--o{ LEARNING : informs

    AUTOMATION_PLAN ||--o{ AUTOMATION_PLAN_VERSION : versions
    AUTOMATION_PLAN_VERSION ||--o{ TASK_RUN : triggers
    TASK_RUN ||--o{ RUN_ATTEMPT : attempts
    RUN_ATTEMPT ||--o{ RUN_OUTPUT : produces
```

### 2.1 单轨审批：SubmissionRevision 是唯一云端审批主体

V2 只保留一条云端治理轨道。所有需要人工决定的对象都以 `SubmissionRevision` 进入审核、以 `ApprovedSnapshot` 固化批准结果，不存在第二条"批准后再物化成 ScriptVersion/BriefVersion"的平行路径。

| 环节 | 主体 | 说明 |
| --- | --- | --- |
| 内部批注与退回 | `SubmissionRevision` | ReviewCycle、ReviewComment 挂在 revision 上，`subject_path` 定位到镜头或字段 |
| 内部与客户批准 | `SubmissionRevision` | ApprovalDecision 绑定 `subject_type=submission_revision` 和 `subject_hash=content_hash` |
| 客户安全链接 | `SubmissionRevision` | ReviewGrant 绑定具体 revision，不绑定"最新版本" |
| 交付与导出 | `ApprovedSnapshot` | DeliveryPackage 引用一个或多个 client approved 的 script ApprovedSnapshot |
| 投放结果绑定 | `ApprovedSnapshot` | PerformanceObservation 的内容版本即 ApprovedSnapshot ID |
| 下游影响传播 | `ApprovedSnapshot` | lineage edge 的终点是快照，不是本地文件 |

内容身份和批准资格分离：

- `BriefVersion`、`Script`、`ScriptVersion`、`Shot` 是**本地产生的内容身份**，ID 在工作区创建、随 publish 原样带入 revision 正文。
- **批准资格由云端授予**：某个 `brief_version_id` 是否"已批准"，取决于它是否出现在对应 brief `ApprovedSnapshot` 的 `eligible_ids` 中；`script_id` 同理。
- 因此本地 lint 判断"上游是否已批准"时，读取 `.contentcloud/cache/approved/` 中已 pull 的快照，而不是查询云端是否存在同名 ScriptVersion 记录。

这一设计使 `contracts/script-package-2.0.schema.json` 的 `brief_version_id`、`script_id` 等字段语义不变，无需重命名。

### 2.2 V1 ScriptVersion 轨道的退役

V1 的 `ScriptVersion` 曾同时是内容载体和审批主体。V2 保留它的**只读历史**语义，并停止在它上面开新审批：

1. 已存在的 V1 `ScriptVersion`、`ReviewCycle`、`ApprovalDecision`、`ReviewGrant`、导出记录和 `PerformanceObservation` 全部保持可读，历史决定不改写。
2. 迁移时为每条 V1 已批准 ScriptVersion 回填一条 `origin=v1_import` 的只读 `ApprovedSnapshot` 影子记录，`external_ref` 保留原 ScriptVersion ID，使交付、结果和 lineage 查询在新旧数据上形态一致。
3. 回填不重算 hash：影子快照沿用 V1 `content_hash`，`schema_version` 标记为 `1.x`。
4. 切换完成并观察稳定后，停止创建新的 ScriptVersion 记录及其 ReviewCycle；读路径按 `12-migration-and-delivery-plan.md` 的退役节奏保留至少一个稳定版本周期。

> 实现现状：`internal/app/review_cycles.go` 与 `internal/app/review_export.go` 目前仍以 `script_version` 为 subject，改挂 `submission_revision` 是波次一的 P0 改造项，见 `14-implementation-status.md`。

## 3. 通用字段与版本规则

可变聚合通用字段：

```text
id, tenant_id, project_id?, status, version,
created_at, created_by, updated_at, updated_by
```

云端不可变 Submission/批准版本通用字段：

```text
id, aggregate_id, version_no, schema_version, content_hash,
based_on_version_id?, context_snapshot_id,
created_at, created_by, superseded_at?
```

- `version` 用于乐观锁；更新请求必须携带 expected version。
- `content_hash` 基于 canonical JSON 计算，审批、导出和 lineage 使用该值。
- `based_on_version_id` 表示修订/变体的直接基线，不代替更细的 lineage edge。
- 版本创建后内容不可修改；错误通过新版本修正。

本地草稿不要求每次编辑都进入云端版本表。本地 `LocalRunContext`、文件 hash 和模板锁负责过程可复现；只有 publish 后才产生 SubmissionRevision。

### 3.1 本地工作区对象（不进入云端事务库）

| 对象 | 存储位置 | 作用 |
| --- | --- | --- |
| `WorkspaceTemplateLock` | `.contentcloud/template.lock` | 模板、Skill、MCP 和 Schema 版本 |
| `WorkspaceSyncState` | `.contentcloud/sync-state.json` | 最近 pull/publish 游标和本地 hash |
| `LocalRunContext` | `work/runs/*.json` | ingest/query/compile/lint 阶段与交接历史 |
| 本地草稿 | knowledge/work/outputs | 未发布知识、策略、Brief 和剧本 |

服务端可保存 WorkspaceBinding 的 workspace ID、设备、模板版本、最后同步时间和 capability，但不保存本地绝对路径。

## 4. 客户与四层上下文

### 4.1 `client_accounts`、`brands`、`products`

`ClientAccount` 表示营销公司服务的企业客户，不等于系统租户。一个租户可以服务多个客户；客户可包含多个品牌和产品。

关键字段：name、industry、service_status、primary_contact、data_region、retention_policy_id。

### 4.2 方法论与模板

| 对象 | 作用 |
| --- | --- |
| `MethodologyTemplateVersion` | 平台级 15 维诊断、研发节点、治理和内容研发规则 |
| `TenantServiceTemplateVersion` | 营销公司的服务包、角色、Gate、检查表、意图集合和交付标准 |
| `BrandKnowledgePackVersion` | 客户/品牌七层上下文、覆盖、缺口、资源和限制 |
| `ProjectContextSnapshot` | 一次 Run 实际可使用的不可变最小上下文 |

`ProjectContextSnapshot` 保存被选中的版本 ID、eligible/blocked ID 集、manifest hash 和任务所需的最小内容，不复制不可控的整个客户知识库。

### 4.3 覆盖规则

```mermaid
flowchart LR
    P[平台默认] --> T[租户覆盖]
    T --> B[客户/品牌覆盖]
    B --> J[项目参数]
    J --> S[不可变快照]
    X[禁止覆盖的治理规则] -.拒绝.-> T
    X -.拒绝.-> B
    X -.拒绝.-> J
```

- 覆盖项必须在模板中声明 `overridable=true`。
- 禁止覆盖人类审批、租户隔离、证据资格、权利门禁和服务端 zero-agent 边界。
- 上层发布新版本不改变已存在快照；项目 rebase 生成影响项并要求人工确认。

## 5. 九域聚合

### 5.0 Submission 与批准快照

`Submission` 是某类业务检查点的稳定身份，类型为 knowledge、research、strategy、brief、script、delivery 或 performance。`SubmissionRevision` 保存 manifest、content hash、base approved snapshot、提交说明、本地 RunContext 摘要和披露策略。

```mermaid
stateDiagram-v2
    [*] --> preparing
    preparing --> submitted: CLI publish完成
    submitted --> in_review
    in_review --> changes_requested
    changes_requested --> submitted: 新revision
    in_review --> approved
    in_review --> rejected
    approved --> superseded: 新批准版本
    submitted --> withdrawn
```

`rejected` 是终态：该 Submission 不再接受新 revision，需要另建 Submission 重新走流程。内审或客户的"退回修改"一律进入 `changes_requested`，不使用 `rejected`。

批准时服务端生成 `ApprovedSnapshot`，包含批准的 canonical 内容、subject hash、决定、允许后续本地使用的 eligible IDs 和下载 manifest。`DecisionDelta`/`ReviewFeedbackBundle` 是客户端 pull 的不可变反馈包。

一个 Submission 的审批分内部与客户两个阶段，但绑定同一个 revision：内部批准记录 `decision_stage=internal`，客户 OTP 批准记录 `decision_stage=client`。只有两个阶段都通过的 revision 才生成可交付的 `ApprovedSnapshot`。

`SourceDisclosure` 对每个来源记录 `metadata_only|evidence_pack|full_source`。默认 evidence_pack；高风险 Claim/Rights 若证据等级不满足租户策略则不能远程批准。

### 5.1 项目与治理

新增 `ClientAccount`、`Brand`、`Product`、`ProjectAssignment`、`GateDecision`、`ProjectRisk`、`ImpactAction`。现有 `BrandProject`、Membership、Device、ConnectSession 和 AuditEvent 原位保留。

项目状态：

```mermaid
stateDiagram-v2
    [*] --> setup
    setup --> active: Gate 0 ready
    active --> on_hold: 人工暂停
    on_hold --> active: 恢复并记录原因
    active --> completed: 服务目标完成
    completed --> active: reopen
    setup --> archived: 取消
    active --> archived: 归档
    completed --> archived: 归档
```

### 5.2 可信知识

V1 Source、SourceRevision、EvidenceSpan、KnowledgeItem、Conflict、DecisionRequest、Asset 和 RightsRecord 继续使用。V2 明确 KnowledgeItem kind：

- `fact_assertion`
- `claim`
- `brand_rule`
- `audience_fact`
- `scenario_fact`
- `synthesis`

状态资格仍按类型决定，不能用一个通用 approved 取代：

```mermaid
stateDiagram-v2
    [*] --> candidate
    candidate --> needs_review
    needs_review --> verified: Fact人工确认
    needs_review --> approved: Claim人工确认
    needs_review --> rejected
    needs_review --> conflicted
    conflicted --> needs_review: 冲突解决
    verified --> review_required: 来源/证据变化
    approved --> review_required: 来源/范围变化
    review_required --> needs_review: 发起复核
    verified --> expired: 有效期到期
    approved --> expired: 有效期到期
```

### 5.3 市场与内容情报

本地研究配置首先存在工作区；publish 后形成 Research Submission。云端 `ResearchTask` 只表示已提交研究或 Automation 配置：research_type、scope、platforms、competitors、queries、time_window、source_policy、status。

`BenchmarkCase`：source snapshot、ownership、channel、performance evidence、visual framework、copy framework、shot patterns、reuse boundary。

`MarketInsight`：statement、evidence refs、confidence、observed_at、applicability、adoption status。状态为 candidate、adopted、rejected、stale；adopted 只代表策略参考资格，不代表品牌事实资格。

### 5.4 产品营销策略

`StrategyVersion` 组合 Audience、Scenario、DemandMoment、PainPoint、SellingPoint 和 adopted Insight。`VisualizationPlan` 继续沿用 V1 并增加：asset strategy、generation mode、truth level、continuity anchor、production risk。

状态：draft -> internal_review -> approved/revision_requested -> review_required -> retired。

### 5.5 内容策划

`ContentPlan` 是周期和渠道计划；`Campaign` 是一个业务主题；`ExperimentPlan` 是单变量测试；`BriefVersion` 是创意生产的不可变输入。

Brief 必须引用 approved StrategyVersion 和至少一个 approved VisualizationPlan，该约束从波次一起生效：`contracts/brief-2.0.schema.json` 中 `strategy_version_id` 与 `visualization_plan_ids` 均为必填，本地 Brief lint 校验二者都落在已 pull 的 strategy ApprovedSnapshot 的 eligible IDs 内。V1 Brief 记录迁移为默认 Campaign 下的 BriefVersion，不改变原 ID。

### 5.6 创意生产

`CreativeDirection` 表示创意方向：angle、hook、narrative、visual motif、tone、target emotion、risks。

`CreativeBatch` 首先是本地批次 manifest：brief snapshot、direction IDs、count、variant dimension、output schema 和本地 status。publish 后云端以 SubmissionRevision 保存候选集合；远程 Automation 才额外关联 TaskRun。

`Script` 是稳定内容身份，`ScriptVersion` 是不可变稿件，`Shot` 是版本内的 JSON 子对象。三者都在本地工作区产生，publish 后作为 SubmissionRevision 正文的一部分进入云端；云端可以建读优化投影表用于列表和比较，但不允许在投影上编辑，也不再为它们单独开审批流（见 §2.1）。

### 5.7 审核与客户协作

沿用 ReviewCycle、ReviewComment、ReviewGrant、ApprovalDecision，全部以 `SubmissionRevision` 为主体。新增字段定位 `subject_path`，例如 `/objects/0/shots/shot-03/voiceover`。

ApprovalDecision 绑定 subject_type、subject_id、subject_hash、decision_stage、actor、decision、reason、previous_state、resulting_state。V2 新记录的 `subject_type` 固定为 `submission_revision`，`subject_hash` 取 revision 的 `content_hash`；`script_version` 仅出现在 V1 历史记录中。

ReviewGrant 绑定 tenant、project、`submission_revision_id`、客户邮箱和有效期。revision 被新 revision 取代后，旧 grant 自动失效，客户须使用新链接。

### 5.8 交付与外部制作

`DeliveryPackage`：一组 client approved 的 script `ApprovedSnapshot`（多对多）、格式、manifest、hash、recipient、delivery status。

`ProductionHandoff`：shot production method、asset checklist、missing inputs、tool suggestion、rights boundary、acceptance checklist、external status 和 final media refs。

V2 只记录外部制作，不存供应商密钥，不自动提交视频生成任务。

### 5.9 投放结果与学习

沿用 ImportBatch、PerformanceObservation、RatingDecision 和 Memory/Lineage 设计。`PerformanceObservation` 的内容版本引用改为 `approved_snapshot_id`；V1 历史观察通过 §2.2 的影子快照获得同一形态。新增 `Learning` 作为候选结论：target_type、target_id、observation_ids、statement、confidence、sample warning、recommended action、adoption decision。一条 Learning 可引用多条 Observation，一条 Observation 也可支撑多条 Learning。

`Learning=adopted` 也不能自动修改 StrategyVersion；采纳动作必须创建新策略/Brief 或显式 ImpactAction。

## 6. Automation、Run 与 Output

`AutomationPlan` 是稳定身份；`AutomationPlanVersion` 是不可变配置；`TaskRun` 是一次远程/事件/定时调度；`RunAttempt` 是一次设备执行；`RunOutput` 必须转换为 SubmissionRevision。

```text
AutomationPlanVersion
  trigger + business_scope + template + parameters
  -> TaskRun
      -> RunAttempt(device/capability/lease)
          -> RunOutput(schema/subject/projection)
              -> SubmissionRevision
                  -> human review / ApprovedSnapshot
```

RunOutput 不得直接批准、发布或覆盖业务对象。普通本地操作不使用上述模型，详细边界见 `06-local-workspace-and-publishing.md` 和 `07-automation-and-run-model.md`。

## 7. Lineage 与影响传播

采用显式 edge：

```text
local_source -> local evidence/knowledge -> knowledge submission -> approved snapshot
approved knowledge -> local strategy/brief -> brief submission -> approved brief
approved brief -> local script -> script submission -> approved script
approved script -> delivery_package -> performance_observation -> learning
```

edge 的两端只能是本地内容身份或云端快照，不再出现 `script_version` 作为独立审批节点。

上游变化只执行两步：

1. 确定性计算受影响对象和影响严重度。
2. 创建 ImpactAction，并按规则将正式下游标记为 `review_required`。

系统不得自动重写下游内容，也不得把所有历史批准记录改成无效；历史决定保持可审计，当前可用性单独表达。

## 8. 数据一致性约束

- tenant/project 外键必须一致，跨项目引用默认拒绝。
- 已批准 Brief 引用的 `strategy_version_id` 必须落在某个 approved 且未失效的 strategy ApprovedSnapshot 的 eligible IDs 中（波次一起生效）。
- 进入 review 的 script SubmissionRevision 必须通过本地 preflight 和服务端 manifest 复核。
- `decision_stage=client` 的批准必须先有同一 revision 上 `decision_stage=internal` 的批准，且无 unresolved blocking comment。
- DeliveryPackage 只能引用两阶段均已批准的 script ApprovedSnapshot。
- PerformanceObservation 必须引用具体 ApprovedSnapshot 和统计窗口。
- schedule trigger 只能用于模板声明的 automation type。
- 同一业务幂等键在 tenant + operation 范围内唯一。

## 9. V1 兼容映射

| V1 | V2 |
| --- | --- |
| BrandProject 中的品牌/单品字段 | 原值保留，同时创建 ClientAccount/Brand/Product 引用 |
| BriefVersion | 归入默认 ContentPlan/Campaign，ID 和 hash 不变 |
| script generation TaskRun | 保留为 V1 远程执行历史；V2 普通生成迁移为 Submission，不伪造 AutomationPlan |
| ScriptPackage 1.x | 只读兼容；修订时显式升级为 2.0 |
| Artifact | 继续保存二进制/扩展产物；新增 RunOutput 负责业务投影关系 |
| PerformanceObservation | 原位保留，内容版本引用改指影子 ApprovedSnapshot，补 Campaign/Experiment/CreativeDirection lineage |
| ScriptVersion 及其 ReviewCycle/Approval/Grant | 只读历史；按 §2.2 回填 `origin=v1_import` 影子 ApprovedSnapshot，不再开新审批 |

V1 云端 TaskRun 生成的 ScriptVersion 保持可读。迁移后新的普通创作默认由本地 publish 创建，审批统一走 Submission 轨；只有明确 Automation 来源的产出才要求 TaskRun/RunAttempt lineage。

数据库迁移必须可回滚结构变更，不回滚已产生的业务决定；任何数据回填先 dry-run 输出数量、冲突和不可映射项。
