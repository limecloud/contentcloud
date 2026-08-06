# 05：共享状态、上下文与故障恢复

> 阅读对象：状态存储、智能体上下文和故障恢复模块研发人员。业务读者可先看 README 中“多个步骤共用一套可靠记录”和“中断后从确认完成的位置继续”两节。本文约定：`WorkTask`=业务任务、`JobRun`=执行实例、`NodeRun`=执行步骤、`RuntimeAttempt`=执行尝试；跨步骤资料称为共享状态，`human_gate` 是人工审批节点。

## 1. 目标

V8 不会把数据库账号直接交给智能体，也不会用一份大型 JSON 或 Markdown 文档保存全部共享状态。系统将提供一层范围有限、规则明确的运行时数据接口：

```text
智能体 / 工作进程
   |
   | 本次执行专用的 MCP / 运行时网关
   v
命令校验
   |
   +--> PostgreSQL 事务
   |      StateMutation + 当前值 + JobEvent + 节点输出
   |
   +--> 对象存储
          大段正文 / 媒体 / 完整对话记录 / Artifact
```

目标是让多个智能体可靠协作，同时由服务端统一控制数据格式（Schema）、版本、权限、预算和审计。

## 2. 共享状态集合（`StateCollection`）

### 2.1 定义

每个集合在创建时固定：

| 字段 | 说明 |
| --- | --- |
| `job_run_id` / `branch_id` | 所属执行实例及其分支；不能由工具调用者自行指定 |
| `collection_key` | 同一执行实例内的稳定名称，例如 `audience_candidates` |
| `schema_id` / `schema_revision` | 已发布的 JSON 数据格式或领域数据格式 |
| `scope` | 执行实例、分支或步骤私有范围：`job`、`branch`、`node_private` |
| `read_policy` / `write_policy` | 角色、节点类型和字段级限制 |
| `consistency` | 单写入方、只追加、逐键 CAS 或归并节点专写 |
| `max_record_bytes` / `max_records` | 防止状态膨胀 |
| `retention_policy` | 执行实例结束后的保留、归档或删除规则 |
| `created_by_plan_revision` | 产生集合的 `JobPlanRevision` |
| `parent_collection_id` / `parent_watermark` | 仅分支继承时填写；指向源执行实例中不可变的读取边界 |
| `inheritance_mode` | `owned`、`snapshot_read_only` 或 `copy_on_write` |

智能体不能自行发布生产数据格式。首版集合只能来自内容业务包或已经批准的 `JobPlan`。主控智能体可以提交向后兼容的数据格式变更申请，但只有通过服务端兼容性检查并获得策略允许后，才能生成新的 `schema_revision`。

### 2.2 写入模式

| 模式 | 允许操作 | 用例 |
| --- | --- | --- |
| `single_writer` | 指定节点对当前版本执行 CAS 写入 | 营销活动需求、最终方向 |
| `append_only` | 带重复提交保护地追加，不修改旧记录 | 发现结果、候选项、审计观察 |
| `cas_map` | 每个键独立执行 CAS 写入 | 各受众的处理进度 |
| `reducer_owned` | 只允许声明的汇聚节点或归并器写入 | 并行结果汇总和排名 |

系统不提供通用的自动合并。需要合并时，必须使用已经发布的确定性归并器，并对输入集合版本和输出摘要进行测试。

### 2.3 分支继承与隔离

从检查点创建新的执行实例时，系统按检查点记录的 `state_watermarks` 解析可继承的共享状态；绝不把源执行实例的“最新状态”作为分支输入。每个集合必须在计划中声明以下一种继承方式：

- `owned`：只属于当前执行实例，不继承；新分支需要时创建自己的空集合。
- `snapshot_read_only`：分支只能读取 `parent_collection_id@parent_watermark`，不能写入，也不能随源执行实例之后的更新变化。
- `copy_on_write`：分支先按父集合冻结水位读取；第一次写入时，服务端在分支自己的集合中创建覆盖记录。后续读取按“分支覆盖记录优先、父集合冻结版本兜底”解析，所有 `StateMutation` 只写入分支集合。

`job` 范围的集合在分支中最多以 `snapshot_read_only` 或 `copy_on_write` 方式继承；`branch` 范围的集合必须归新分支所有；`node_private` 不自动继承。数据格式、读取策略、保留策略或内容权利不再兼容时，分支准入失败。服务端必须拒绝新分支使用源 `job_run_id` 写入共享状态，并把父集合、父水位和覆盖来源写入审计事件。

### 2.4 StateRecord 与 StateMutation

```text
StateRecord
  collection_id / key
  value_json or artifact_ref
  schema_revision
  version / digest
  created_by / updated_by

StateMutation (append-only)
  mutation_id / operation
  expected_version / resulting_version
  value_digest / patch_digest
  node_run_id / attempt_id / agent_instance_id
  idempotency_key / committed_at
```

同一个重复提交保护键（幂等键）和相同请求摘要，应返回首次处理结果；键相同但请求内容不同则返回冲突。比较并交换（CAS）写入失败后，智能体必须重新读取最新版本，系统不得让后一次写入直接覆盖前一次结果。

## 3. 运行时网关

Codex 和 Claude Code 都支持 MCP，因此首版运行时网关以 MCP 工具契约为基础。命令行和 SDK 接入也复用同一服务层，不另外维护一套业务规则。

### 3.1 最小工具集

| 工具 | 语义 | 权限 |
| --- | --- | --- |
| `state.get` | 按集合、键和版本读取一条记录 | `ContextView` 允许读取的集合 |
| `state.query` | 分页查询并返回版本和内容摘要 | 只允许预先声明的筛选和排序方式 |
| `state.cas` | 按预期版本写入或创建记录 | 集合的写入策略 |
| `state.append` | 幂等追加一条记录 | 只追加集合 |
| `artifact.resolve` | 获取允许读取的对象摘要/短期句柄 | 不直接返回永久签名 URL |
| `child.propose` | 提交受限的执行图变更申请 | 仅主控智能体 |
| `child.list` | 查看直接子节点的结构化状态 | 不返回同级节点的完整对话记录 |
| `job.wait` | 声明依赖、审批、外部操作或时间等待条件 | 当前节点和智能体 |
| `progress.emit` | 结构化进度和可公开摘要 | 大小和字段受限 |
| `effect.prepare` | 声明外部操作意图 | 节点策略和授权结果 |
| `effect.status` | 查询或核对已声明的外部操作 | 当前外部操作范围 |
| `checkpoint.request` | 请求在节点边界创建检查点 | 主控智能体或策略允许的节点 |

不提供 `sql.execute`、`schema.ddl`、任意 URL 抓取（`URL fetch`）、修改预算、通过人工审批节点或直接完成执行步骤的工具。

### 3.2 授权

每次工具调用的权限都从短期 `Attempt` 凭据逐级推导：

```text
tenant -> project -> job -> node -> attempt -> agent
```

请求正文中的 ID 只能进一步缩小权限范围，不能扩大范围。凭据到期、租约失效、执行实例进入取消状态或 `ContextView` 被撤回后，写操作必须默认拒绝。

## 4. 上下文视图（`ContextView`）

### 4.1 为什么一味扩大上下文仍然不够

更长的模型输入和自动上下文压缩（`compaction`）只能缓解输入长度问题，不能提供：

- 并发写隔离和 CAS。
- 精确的业务版本、审批状态和对象摘要。
- 确保不同智能体之间的最小权限隔离。
- 可重复的恢复输入。
- 外部操作的实际状态。

因此，每次启动或恢复智能体时，都要生成一份不可变的 `ContextView`。

### 4.2 数据流

```text
JobPlan + NodeSpec
        |
        +---- 输入选择器 ---------> 正式业务对象 / Artifact 引用
        |
        +---- 状态选择器 ---------> StateCollection@watermark
        |
        +---- 事件选择器 ---------> 最近的 JobEvent 摘要
        |
        +---- 策略 ---------------> 工具 / 模型 / 预算 / 数据披露
        |
        v
上下文构建器
  -> 按固定优先级，在 Token 上限内取数
  -> 生成带来源引用的脱敏摘要
  -> 生成 ContextView 清单和摘要
        |
        v
智能体本次执行
```

### 4.3 上下文视图包含什么

- 系统和运行时指令的版本与内容摘要。
- `TaskContract`、`NodeSpec`、输出数据格式和完成条件。
- 精确的来源（Source）、知识（Knowledge）、产物（Artifact）引用、版本和内容摘要。
- `StateRecord` 选择结果及读取水位。
- 直接子节点的状态和结构化摘要。
- 最近相关 JobEvent，而不是整个事件流。
- 允许使用的 MCP 工具、调用预算和截止时间。
- 数据来源标签：`trusted_policy`、`canonical_fact`、`agent_generated`、`untrusted_external`。

大段正文和媒体不直接放入上下文，由工具按权限分页读取。`ContextView.digest` 与 `Attempt` 绑定，结果上报时由服务端校验，避免输入已经变化后，智能体仍用旧资料提交正式结果。

### 4.4 上下文容量

确定性优先级：

```text
系统策略 / 任务契约 / 审批条件
  > 必需的正式输入
  > 尚未完成的子节点和外部操作摘要
  > 最近的相关事件
  > 可选示例和历史记录
```

超过预算时：

1. 移除可选数据。
2. 使用带来源引用的确定性摘要。
3. 提供分页查询工具。
4. 仍然超限时，对应执行步骤进入 `waiting_input`；没有其他可运行执行步骤时，执行实例进入 `waiting(input_too_large)`。不得静默截断人工审批节点条件、素材权利或任务契约。

## 5. 完整对话记录与业务状态分离

Codex 线程和 Claude 会话可以保存并恢复对话，但完整对话记录只用于观察执行过程：

- 默认只保存会话引用、模型、用量、工具事件和脱敏摘要。
- 是否导出完整对话记录由租户数据策略决定，导出后存入受限对象存储。
- 对话压缩不会改变 `StateCollection`、`JobEvent` 或正式内容数据。
- 智能体会话恢复失败时，可以根据 `ContextView` 创建新会话；`JobRun` 不会因此丢失。
- 宿主会话分支不会自动创建业务执行分支，反过来也一样。

Claude SessionStore 或 Codex thread storage 可以作为适配器保存对话记录的后端，但不能替代 ContentCloud 的状态事务。

## 6. 检查点（`Checkpoint`）

### 6.1 定义

`Checkpoint` 是在安全执行边界记录的一组逻辑状态引用：

```text
Checkpoint
  job_run_id / checkpoint_no / created_at
  plan_revision_id / graph_version / plan_digest
  node_status_manifest_digest
  state_watermarks[] / state_digests[]
  event_cursor
  output_artifact_refs[]
  side_effect_watermark
  context_summary_refs[]
  parent_checkpoint_id
  reason / created_by
```

不包含：

- OS 进程、内存、goroutine、文件描述符或网络连接。
- 活跃 lease、RunToken、Device token、Secret 或临时签名 URL。
- 浏览器句柄和未登记的服务商客户端状态。
- 模型隐藏推理或未经策略允许的完整 Transcript。

### 6.2 安全创建点

首版只在以下边界创建：

- 执行实例准入成功后。
- 节点输出、`StateMutation`、`JobEvent` 和 `SideEffectRecord` 状态已经原子提交后。
- 主控智能体让出执行资源前。
- 人工审批节点（`human_gate`）通过后。
- 操作员显式请求且当前没有未解决的事务提交。

正在运行的 `Attempt` 不能直接“快照”；它必须先完成、失败、过期或结束本次执行并让出资源。

## 7. 故障恢复、重放与创建执行分支

```text
                   常规故障恢复
JobRun + 已持久化的最新事件 --------------------> 原 JobRun 继续执行
        |
        | 重放状态投影，不执行任务
        +----------------------------------------> 重建读模型
        |
        | 从 Checkpoint 明确创建分支
        v
新 JobRun（共享不可变引用，采用新策略并重新准入）
```

### 7.1 故障恢复

- 租约过期后，将 `NodeRun` 送回 `ready` 或 `retry_wait`，再创建新的 `Attempt`。
- 守护进程日志（Daemon journal）重新上报时使用原重复提交保护键（`idempotency key`），服务端返回已经提交的结果。
- 智能体会话可以恢复时继续原会话；无法恢复时根据 `ContextView` 创建新会话。
- 恢复只前进，不撤销已经提交的状态或业务事实。

### 7.2 状态重放（`Replay`）

`Replay` 只根据检查点、执行事件和权威状态表重建读模型与诊断信息，不重新执行任何业务操作：

- 不调用 LLM、服务商、MCP 工具或外部 API。
- 不重新发送通知、上传或发布。
- 发现事件游标、状态摘要和读模型不一致时，停止重放并报告数据损坏。

### 7.3 创建新的执行分支

创建执行分支时生成新的 `JobRun`（执行实例）：

- 复用不可变的输入、正式产物引用，以及按检查点水位冻结、并且策略允许共享的状态快照。
- 写入新的根分支/父分支关系、`RuntimePolicy`、`JobPlanRevision` 和幂等作用域。
- 重新验证素材权利、Offer 有效期、人工审批节点、服务商配置、模型和预算。
- 原 `JobRun` 保持不变；新分支只记录后续新增或变更的状态，并按集合继承模式在分支自己的共享状态中写入，不能覆盖源执行实例。
- 旧分支已经生效的外部操作只作为历史引用，不会在新分支自动再次执行。

“回滚到检查点”在用户界面中必须显示为“从这里创建分支”，不能暗示外部世界被倒带。

## 8. 工具调用与外部操作

### 8.1 ToolCall

每次可观测工具调用记录：

```text
tool_call_id / tool_name / tool_schema_version
job/node/attempt/agent/session refs
request_digest / safe_request_summary
state: proposed | authorized | running | succeeded | failed | unknown
started_at / finished_at / usage / error_code
```

读操作和纯计算可以只记录 `ToolCall`；会改变外部状态、产生费用或产生不可逆结果的调用，必须先创建外部操作记录（`SideEffectRecord`）。

### 8.2 外部操作状态机

```text
planned -> authorized -> executing -> committed
                         |     |
                         |     +--> unknown -> reconciling
                         |                       |       |
                         |                       v       v
                         |                    committed failed
                         |
                         +--> failed -> retry_wait -> authorized

committed -> compensation_requested -> compensating
                                      |             |
                                      v             v
                                compensated  compensation_failed
```

### 8.3 结果不明

以下情况应进入结果不明状态（`unknown`），而不是直接判定失败：

- 向服务商提交请求时超时，无法确认外部任务是否已经创建。
- 外部系统返回成功后，本地在保存外部 ID 前崩溃。
- 文件上传连接断开，远端可能已经完成。
- 回调签名有效但本地缺少对应 Attempt 的最终状态。

处理流程：

```text
unknown
  -> 使用相同幂等键和请求摘要查询服务商
  -> 找到外部对象：绑定并进入 committed
  -> 服务商确认未创建：进入 failed/retry_wait
  -> 服务商无法查询或结果冲突：等待人工对账
```

处于 `unknown` 状态时，禁止生成新的幂等键再次提交。业务任务被取消也不改变这一规则：执行实例进入 `waiting(external_effect)`，只允许对账或登记人工对账结论；所有关联外部操作进入终态后，才回到 `canceling` 并完成取消。

### 8.4 补偿

补偿本身也是一条新的、可审计的外部操作，不能通过删除旧记录掩盖已经发生的动作。每类外部操作都要声明：

- 是否可补偿。
- 补偿时间窗口和所需的人工审批节点。
- 补偿操作的幂等键。
- 无法自动恢复时使用的人工运维手册。

例如，取消尚未开始生成的外部任务可能可以补偿；已经被外部用户下载的成片或已经发布的内容无法真正撤销，只能下架并保留历史。

## 9. 一致性边界

必须在一个 PostgreSQL 事务内完成：

- `StateMutation`、当前 `StateRecord` 投影和 `JobEvent`。
- 节点输出绑定、`NodeRun` 状态和 `JobEvent`。
- `GraphPatch`、新版本、`NodeSpec`/边和 `JobEvent`。
- `Attempt` 领取、`ResourceReservation`、租约和 `JobEvent`。
- 外部操作授权、费用预留和 `JobEvent`。

对象存储无法加入数据库事务时，使用发件箱/收件箱模式：先保存带内容摘要的待处理清单，再上传和校验，最后提交正式 `Artifact`。校验失败的二进制文件不能被正式输出引用。

## 10. 验收

- 两个智能体同时对同一版本执行 CAS 写入时，只有一个成功，另一个收到可诊断的版本冲突。
- 使用同一幂等键重复上报追加操作 100 次，只产生一条 `StateMutation`。
- 输入或状态水位发生变化后，`ContextView` 必须产生新的摘要；旧 `Attempt` 不能提交要求新上下文的输出。
- 进程在节点输出与状态提交之间的任意时刻被终止，都不能出现“节点成功但没有输出”或相反的不一致状态。
- 状态重放不触发任何智能体、外部服务或工具调用。
- 创建执行分支不修改原 `JobRun`；分支只能读取父集合的冻结水位，并在自己的 `copy_on_write` 覆盖集合中写入，同时重新校验权限、素材权利、费用和执行配置。
- 外部操作为 `unknown` 时禁止自动重试；对账成功后只能绑定一个外部对象。
- 状态、事件和完整对话记录中不得出现密钥、`RunToken`、永久下载地址或超出策略允许范围的原始正文。
