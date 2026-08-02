# 架构：编排层与事实层分离

## 1. 架构结论

方案采用两条清晰边界：

1. **编排层**负责让人和机器完成工作：Environment、Project、SOP、Task、TaskRun、StageRun 和 ExecutionAttempt。
2. **事实层**负责证明发生了什么：Source、Evidence、Rights、SubmissionRevision、GateEvaluation、Decision、AcceptedSnapshot、DeliveryPackage 和 PerformanceObservation。

编排层可以被重建、投影和优化；事实层的关键记录不可被 UI 状态覆盖。Task 是一等用户对象，但不是新的内容事实源。

## 2. 分层结构

```text
┌────────────────────────────────────────────────────────────┐
│ User surface                                               │
│ Task / Inbox / My Tasks / Project Tasks / Task Detail      │
└──────────────────────────────┬─────────────────────────────┘
                               │ commands + projections
┌──────────────────────────────▼─────────────────────────────┐
│ Orchestration                                               │
│ Environment / SOP Registry / Project / Task / TaskRun       │
│ StageRun / ExecutionAttempt / NextAction                    │
└──────────────────────────────┬─────────────────────────────┘
                               │ typed refs + immutable versions
┌──────────────────────────────▼─────────────────────────────┐
│ Governance facts                                            │
│ Evidence / Rights / Revision / Gate / Decision / Snapshot   │
│ Delivery / Observation                                     │
└──────────────────────────────┬─────────────────────────────┘
                               │ local handoff
┌──────────────────────────────▼─────────────────────────────┐
│ Execution resources                                         │
│ Local Workspace / Codex CLI / Claude Code / Local Rules     │
└────────────────────────────────────────────────────────────┘
```

Web 只读取投影并发出受控命令；本地 Workspace 负责本地候选和工具执行；服务端负责权限、版本、事实写入和审计。

## 3. 作用域

### 3.1 Environment

Environment 是一组可独立运行的业务和技术配置边界，至少包含：

- SOP Registry 和当前发布版本；
- Content Capability 开关；
- 本地 Workspace、Codex/Claude Code CLI 和本地规则可用性；
- Role Policy、数据披露和保留策略；
- Environment Manifest、digest 和健康状态；
- 运行队列、成本预算和审计范围。

每个 Environment 都必须有显式 SOP 配置。没有隐式的“平台默认流程”可以绕过环境配置。

### 3.2 Tenant

Tenant 拥有 SOP 模板、角色、成员、内容能力和用量边界。Tenant 可以创建草稿、发布版本到指定 Environment，也可以撤回尚未被新 Task 绑定的版本。

### 3.3 Project

Project 是品牌、产品、客户、渠道、资料、知识、资产、任务和交付的上下文容器。Project 绑定一个或多个已发布 SOP Version，但不把每个 Stage 或 Gate 复制成常驻字段。

### 3.4 Task

Task 是一个用户可理解的工作对象，至少包含：

```text
task_id
environment_id
tenant_id
project_id
title
intent
content_type
sop_version_id
input_refs
requested_output
owner / assignee
priority
due_at
risk_profile
status
created_by / created_at / updated_at
```

Task 可以从全局工作区创建，也可以从 Project、Inbox、Chat 或 Automation 创建。创建时必须固定 Project 和 SOP Version；不得在运行中静默替换。

### 3.5 TaskRun

TaskRun 是 Task 的一次执行实例。它记录：

- 被绑定的 SOP Version 和 Environment digest；
- StageRun 列表和当前 Stage；
- 执行资源类型；
- 输入和输出引用；
- claim、lease、暂停、恢复和失败原因；
- 下一动作和审计事件。

Task 可以重新运行，但每次运行都必须拥有独立 TaskRun 和版本绑定。历史 Run 只读。

### 3.6 StageRun

StageRun 是 SOP Stage 在某个 TaskRun 上的运行投影/编排记录，包含：

- Stage Definition ID 和版本；
- 输入 refs、预期输出 Schema；
- 负责人和执行资源；
- Gate 结果和 blocker；
- 关联 Revision、Decision、AcceptedSnapshot 和 Delivery；
- 可重试、可退回和升级条件。

StageRun 不直接保存未发布的本地正文。

## 4. SOP 模型

### 4.1 SOPDefinition

SOPDefinition 是可复用流程的身份，包含名称、用途、适用内容类型、所属 Tenant/Environment 和当前发布版本。

### 4.2 SOPVersion

SOPVersion 是不可变的流程版本，至少包含：

```text
sop_version_id
sop_id
version
digest
status: draft | published | retired
stages[]
default_execution_mode
default_metrics[]
created_by / published_by
created_at / published_at
```

TaskRun 只绑定 published 版本。Draft 可以编辑，Published 只能通过新版本修改。

### 4.3 StageDefinition

每个 Stage 至少包含：

```text
stage_id
name
description
order
owner_roles[]
input_schema
input_refs[]
output_schema
required_capabilities[]
execution_modes[]
checks[]
gate_ids[]
retry_policy
escalation_policy
```

Stage 只定义业务步骤和产物，不定义模型、Prompt、本机路径或外部账号。

### 4.4 GateDefinition

Gate 是可配置的条件或人工决定，模式为：

| 模式 | 语义 |
| --- | --- |
| `none` | 不增加额外门禁，Stage 完成后继续 |
| `advisory` | 给出提示，不阻止继续 |
| `required_check` | 确定性检查必须通过 |
| `internal_review` | 指定内部角色决定 |
| `client_decision` | 指定客户角色决定 |
| `rights_confirmation` | 权利状态必须有效或明确豁免 |
| `evidence_confirmation` | 证据完整性必须满足 |

平台安全、越权、数据披露和权利硬约束不能被租户关闭。业务审批 Gate 可以关闭或换成 advisory。

## 5. 本地执行资源

```text
manual
  -> 用户在 Web 或 Workspace 完成步骤

local
  -> 本地 Workspace 执行，显式 publish 摘要或 Revision

cli
  -> Codex 或 Claude Code 通过本地 CLI 配置领取步骤，输入和输出仍受 Capability 契约约束

automation
  -> 本地 Hook、计划任务或批处理触发的隔离 Attempt

parallel
  -> 同一 Workspace 中多个本地 CLI 会话按 Stage 并行，最终仍写回统一事实链
```

执行资源不是内容完成的判定。CLI 进程或本地规则成功只能产生 ExecutionAttempt 成功；Task 只有在输出、检查、Gate 和 AcceptedSnapshot 条件满足后才进入对应正式状态。

### 5.1 客户端适配器

本地会话属于执行客户端，不属于 Web 平台。Codex CLI、Claude Code CLI 和 Workspace 工具各自维护自己的会话文件、事件结构和版本兼容逻辑，不能要求 Web 用同一种格式扫描或解析。

每个 `ClientAdapter` 至少注册：

```text
client_id
client_version
adapter_version
owned_formats[]
capabilities:
  supports_summary
  supports_selected_turns
  supports_full_transcript
  redaction_handled_locally
connection_status
last_seen_at
```

边界如下：

| 责任 | 客户端适配器 | Web / 服务端 |
| --- | --- | --- |
| 发现本地会话 | 是 | 否 |
| 解析客户端私有格式 | 是 | 否 |
| 选择会话与轮次 | 是 | 只声明请求范围 |
| 脱敏本机路径、令牌和账号 | 是 | 校验脱敏声明和 Schema |
| 生成 `ConversationBundle` | 是 | 否 |
| 创建、追踪 `ConversationImport` | 响应请求和上报状态 | 是 |
| 将导入内容升级为正式业务事实 | 否 | 通过独立人工/业务命令完成 |

新增客户端只需实现自己的适配器和统一 Bundle 输出，不修改 Web 端的 Transcript 解析逻辑。

### 5.2 ConversationBundle 与 ConversationImport

`ConversationBundle` 是客户端导出的结构化传输包，包含客户端身份、导出目的、选择范围、可公开内容、脱敏清单、授权声明和内容 digest。它可以承载 Stage 摘要或用户选择的片段；完整 Transcript 是额外授权的例外能力，不是默认数据路径。

`ConversationImport` 是服务端编排记录，只管理导入生命周期：

```text
requested
  -> awaiting_client_confirmation
  -> exported
  -> received
  -> attached | rejected
  -> expired
```

`attached` 只表示已绑定到 Task 上下文，不表示已创建 Revision、Knowledge、Evidence、Decision 或审计结论。后续转换必须重新经过对应 Schema、权限、来源、权利和人工确认命令。

### 5.3 对话导入数据流

```text
Task Detail 创建 ConversationImport 请求
  -> 指定 client_id、用途和期望范围
  -> 本地 ClientAdapter 收到请求
  -> 用户在客户端选择会话与轮次
  -> 客户端解析私有格式、脱敏并本地预览
  -> 用户确认导出
  -> ClientAdapter 生成 ConversationBundle
  -> 服务端校验 Schema、授权、作用域和 digest
  -> 绑定为 Task 输入或 Evidence 候选
```

页面打开、Task 运行和每轮本地对话都不会触发自动上报。服务端只接收必要的运行生命周期、结构化摘要和用户显式导出的 Bundle。

## 6. 知识基础设施

知识库不是 Markdown 页面集合，也不是向量检索的别名。它由类型化对象、来源证据、状态决策、关系、知识包和不可变快照共同组成；Markdown 只是一种可编辑投影。

### 6.1 七层业务覆盖

| 层级 | 主要对象 | 解决的问题 |
| --- | --- | --- |
| 身份与品牌 | Entity、BrandRule、IdentityRecord | 谁在表达，哪些品牌边界必须保持 |
| 产品与规格 | Product、FactAssertion、ProcessRecord | 产品事实、参数、生产和质检是否可证明 |
| 市场与受众 | Audience、Scenario、Insight | 面向谁、处于什么场景、观察依据是什么 |
| 表达与主张 | Claim、Message、ConstraintRecord | 可以说什么、在哪些渠道和范围内说 |
| 运营与渠道 | ChannelRule、DeliveryRule、Observation | 如何生产、交付、发布和回收结果 |
| 内容引擎 | Intent、Template、SOPBinding | 如何把知识稳定地转为内容任务输入 |
| 合规与权利 | RightsRecord、PolicyRule、Prohibition | 素材、主张和渠道是否可合法使用 |

层级用于覆盖分析，不替代对象类型。一个对象可以与多层对象建立关系，但必须有单一主类型和明确状态机。

### 6.2 类型化对象与状态

核心对象至少包括：

```text
Entity / Product / Audience / Scenario
FactAssertion / Claim / Insight
BrandRule / ConstraintRecord / RightsRecord
ConflictRecord / KnowledgeGap
KnowledgePack / KnowledgeSnapshot
```

状态不能合并为一个通用 `approved`：

- `FactAssertion`：`candidate -> needs_review -> verified | rejected | superseded`；
- `Claim`：`candidate -> needs_review -> approved | prohibited | expired`；
- `RightsRecord`：`pending -> valid | expired | revoked`；
- `Insight`：`candidate -> verified | rejected`；
- `ConflictRecord`：`open -> resolved | accepted_risk`；
- `KnowledgeGap`：`source_missing -> collecting -> resolved | waived`。

只有对象类型允许且状态属于当前查询策略的对象，才可以进入 `eligible` 结果。运行成功、摄取成功和模型高置信度都不能自动改变业务状态。

### 6.3 Source、Evidence 与关系

`Source` 保存原件身份、owner、MIME、digest、获取方式和披露范围；`Evidence` 保存可复核定位，例如页码、表格单元格、URL + captured_at 或 ConversationBundle block。知识对象引用 Evidence，不直接保存一段无法定位的摘录。

```text
Source
  -> Evidence locator
  -> candidate KnowledgeObject
  -> relation / conflict / gap
  -> human or deterministic decision
  -> accepted object version
```

Source 原件变化只会产生新的 digest 和候选版本，不覆盖历史 Evidence。冲突和缺口是一等对象，可以被指派、阻断查询并转为补料 Task。

### 6.4 摄取、知识包与快照

确定性摄取流程为：登记 Source、生成 digest、定位 Evidence、提取候选对象、执行 Schema/关系 lint、进入待审队列。摄取只产生候选，不直接生成可引用知识。

`KnowledgePack` 按业务用途选择对象和查询策略，例如“新品品牌与产品知识包”。发布知识包时生成不可变 `KnowledgeSnapshot`，包含对象版本、状态、关系、Evidence digest、权利摘要和 pack digest。TaskRun 绑定明确快照；新知识版本不会静默替换历史运行。

查询接口必须返回：

```text
eligible[]  当前范围内可确定性使用的对象 ID 和 Evidence
blocked[]   因状态、冲突、权利或范围被阻断的对象 ID 和原因
gaps[]      需要补料的 KnowledgeGap 和建议下一动作
snapshot    查询使用的知识包版本与 digest
```

自然语言摘要可以解释结果，但不能替代上述结构化结果。

## 7. 状态和不变量

### 7.1 Task 状态

```text
inbox -> ready -> running -> waiting_input
                    ├─ waiting_gate
                    ├─ changes_requested -> ready
                    ├─ failed -> ready / canceled
                    └─ accepted -> delivered -> archived
```

状态是投影，不允许前端直接写任意值。服务端根据 TaskRun、StageRun、Gate、Revision 和 Delivery 事实计算。

### 7.2 必须保持的不变量

1. TaskRun 绑定的 SOP Version、Environment digest 和 Project 不能被覆盖。
2. 同一 StageRun 的活动写入者必须有单一 claim；过期和冲突都必须留下审计。
3. Revision 必须绑定 TaskRun、输入快照和 Environment digest。
4. GateEvaluation 必须绑定当前 Revision digest；新 Revision 生成后旧决定不能自动复用。
5. AcceptedSnapshot 必须指向明确的 Approved/accepted Revision，不能由 Task 状态直接生成。
6. DeliveryPackage 只能引用仍然有效的 AcceptedSnapshot、Rights 和输出 Schema。
7. SOP 新版本只影响新 TaskRun；历史 Run 保留原版本可读性。
8. 未 publish 的本地正文不进入 Web API、Projection 或审计摘要。
9. Web 不发现或解析客户端私有 Transcript；格式兼容由对应 ClientAdapter 负责。
10. 完整 Transcript 必须同时具有客户端能力声明、用户显式授权和服务端作用域校验。
11. ConversationImport 不得自动产生 Revision、Knowledge、Evidence、Decision 或 AcceptedSnapshot。
12. Source 变化不得覆盖历史 Evidence；必须生成新 digest 和候选对象版本。
13. ConflictRecord 或硬性 KnowledgeGap 未解决时，相关对象不得进入确定性查询的 `eligible`。
14. TaskRun 绑定的 KnowledgeSnapshot 不得被新知识包版本静默替换。

## 8. 现有能力的归位

| 现有能力 | 目标归位 |
| --- | --- |
| Project / Tenant / Membership | Identity 和上下文边界 |
| Workspace / Connect / LocalRun | 执行资源和本地事实入口 |
| Content Pack / Capability | SOP 所需能力和输出 Schema |
| Submission / Revision | 正式提交和版本事实 |
| Evidence / Rights | 内容可用性的治理事实 |
| Review / Decision | 可选 Gate 的人工决定 |
| ApprovedSnapshot | 可交付的不可变结果 |
| DeliveryPackage | 外部接收和交接事实 |
| PerformanceObservation | 结果观察和学习输入 |
| ProjectProjection | 兼容读模型，逐步扩展为 Workspace/Task Projection |

方案不删除上述事实；需要删除的是把它们直接暴露为普通用户第一层导航的产品组织方式。

## 9. 投影架构

### 9.1 WorkspaceTaskProjection

面向输入收集、我的任务和所有任务，字段包括：

- Task 标题、Project、SOP、当前 Stage；
- Task 状态、优先级、负责人、截止时间；
- 下一动作、阻断类型和更新时间；
- 最近 Revision、Gate 和 Delivery 摘要；
- 允许动作列表。

### 9.2 TaskDetailProjection

面向任务详情，按以下顺序展示：

1. 业务目标和输出要求；
2. SOP、当前 Stage 和下一动作；
3. 输入资料、知识、Evidence、Rights 摘要；
4. 执行摘要和失败原因；
5. Revision、Gate、Decision 和 AcceptedSnapshot；
6. Delivery、结果和历史 Run；
7. 审计和允许动作。

### 9.3 SOPProjection

面向 SOP 设计和 Project SOP 页，显示：

- 模板、版本、digest 和发布状态；
- Stage 顺序、输入输出和负责人；
- Gate 模式及其是否阻断；
- 所需能力、自动化规则和指标；
- 当前被哪些 Project/Task 使用；
- 新版本影响范围。

Projection 只读事实，写入通过命令 API 完成。

## 10. 数据流

```text
CreateTask
  -> resolve Environment + Project + SOPVersion
  -> create Task + TaskRun
  -> compute first StageRun
  -> claim execution resource
  -> produce candidate / checks
  -> publish Revision or report local summary
  -> compute GateEvaluation
  -> optional Decision
  -> AcceptedSnapshot
  -> DeliveryPackage
  -> PerformanceObservation / Learning
```

每一步都有幂等键和可重建输入。Projection 刷新不会产生业务事实；页面打开不会触发本地文件上传或自动审批。

## 11. 扩展规则

- 新内容类型优先增加 Schema、SOP Stage 和 Capability，不新建核心 Task/Review/Delivery 系统。
- 新 Gate 类型必须实现统一的输入、输出、阻断和审计接口。
- 新执行资源必须实现统一的 claim、attempt、summary 和 cancellation 契约。
- 新 CLI 执行器只能通过 Registry 暴露能力、版本、输入输出和成本，不把具体本机命令写进业务 SOP。
- 新会话来源必须实现 ClientAdapter 并输出统一 ConversationBundle，不允许在 Web 增加私有格式解析分支。
- 新指标必须能引用 Task、Revision、Delivery 或 Observation，不允许前端手填“完成率”。
