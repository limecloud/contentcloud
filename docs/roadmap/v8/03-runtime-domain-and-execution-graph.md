# 03：运行时领域模型与执行图

> 阅读对象：后端、数据库和编排模块研发人员。本文件说明系统内部怎样保存一次执行，普通用户不需要理解这些对象和字段。本文约定：`WorkTask` 是业务任务，`JobRun` 是执行实例，`NodeRun` 是执行步骤，`RuntimeAttempt` 是执行尝试；任务关系用执行图（DAG）表示，跨步骤资料使用共享状态，`human_gate` 是人工审批节点。

## 1. 聚合边界

V8 新增一个运行时聚合根 `JobRun`，但不新增与 `WorkTask` 同义的通用 `JobDefinition`。

```text
WorkTask                              业务任务及其治理
  └── 执行实例 JobRun [1..n]         一次执行、重跑或分支
      ├── 执行计划版本 JobPlanRevision [1..n] 初始图与受控变更
      │   ├── 节点规格 NodeSpec
      │   └── 节点依赖 NodeDependency
      ├── NodeRun（V8 独立运行时节点）[1..n]
      │   └── 执行尝试 RuntimeAttempt [1..n]
      ├── 智能体实例 AgentInstance [0..n]
      ├── 状态集合 StateCollection / Mutation
      ├── 逻辑检查点 Checkpoint
      ├── 外部操作记录 SideEffectRecord
      └── 执行事件 JobEvent
```

所有对象都必须记录 `tenant_id`；项目内对象还要记录 `project_id`。服务端根据登录身份、`JobRun` 和租约计算对象归属，不能信任智能体上报的租户或项目编号。

## 2. 现有对象的演进

| 现有对象 | V8 语义 | 迁移方式 |
| --- | --- | --- |
| `WorkTask` | 业务任务 / 业务聚合根 | 不改表名；增加 active/latest `JobRun` 投影 |
| `SOPVersion` | 业务流程与人工审批节点约束 | 发布时可编译成初始 JobPlanRevision |
| `StageRun` | 业务阶段投影 | 保留；从多个 `NodeRun` 聚合状态 |
| `TaskRun` | V7 兼容执行记录 | 保留现有路径；不再作为 V8 NodeRun 的物理表 |
| `RunAttempt` | V7 兼容执行尝试 | 只关联 `task_runs`，不参与 V8 Runtime 调度 |
| `RuntimeAttempt` | V8 唯一权威执行尝试 | 独立 `runtime_attempts`，保存租约、能力快照、Harness 会话引用和结构化结果摘要 |
| `TaskStageOutput` | 执行输出绑定（`JobOutputBinding`） | 保留物理名称；绑定 `NodeRun`/`Artifact` 时增加兼容字段 |
| `ContextSnapshot` | 输入快照基础 | `ContextView` 引用它，不改变历史快照 |
| `CreativeExecutionBundle` | 执行包（`ExecutionBundle`） | 保留并扩展运行策略和执行配置引用 |
| `MediaGenerationJob` | 服务商作业 | 保留；关联 `side_effect_id`，避免与 `JobRun` 混淆 |
| `ProviderAttempt` | 外部执行尝试 | 保留；受外部操作记录状态机约束 |

V8 首版不物理重命名 `TaskRun`、`TaskStageOutput` 或媒体表。API/BFF 可以展示 `NodeRun`、`JobOutputBinding` 和 `ProviderJob` 语义，数据库迁移保持增量方式。

## 3. 执行实例（`JobRun`）

### 3.1 主要字段

| 字段 | 记录内容 |
| --- | --- |
| `id` | `JobRun` 的唯一标识 |
| `work_task_id` | 对应的业务任务；首版必填 |
| `idempotency_key` / `request_digest` | 识别重复创建请求；同一个键和相同内容返回原记录，同一个键但内容不同则返回冲突 |
| `parent_job_run_id` / `root_job_run_id` | 父执行、根执行或派生执行关系 |
| `plan_revision_id` | 本次采用的不可变执行计划版本（`JobPlanRevision`） |
| `input_snapshot_id` | 本次采用的输入对象、版本和摘要 |
| `execution_bundle_id` | 能力、运行环境（`Environment`）、工具和服务商版本 |
| `runtime_policy_id` | 并发、预算、失败隔离、数据披露和恢复策略 |
| `branch_kind` | `initial`、`retry`、`fork`、`replay` |
| `state` | 当前运行状态 |
| `graph_version` / `event_cursor` | 当前执行图版本和已处理事件位置 |
| `budget_reservation_id` | 模型用量、服务商调用和存储预算 |

### 3.2 不变量

- 创建时冻结 `work_task_id`、SOP 版本/摘要、输入快照、`ExecutionBundle` 和 `RuntimePolicy`。
- 创建命令必须带租户和 `WorkTask` 范围内的幂等键及请求摘要；键和摘要都相同时返回原记录，键相同但摘要不同时返回冲突。
- 一个 `WorkTask` 同时最多只能有一个运行中的 `JobRun`；重试、创建分支或重放需要新建执行实例时，必须先结束或暂停旧实例。
- `JobRun` 进入终态后不能原地回退。
- `succeeded` 只说明执行图成功；`WorkTask` 是否 `accepted` 或 `delivered` 仍由业务审批和正式交付数据决定。
- 每次状态变化和执行图版本变化都要产生 `JobEvent`。

### 3.3 状态机

```text
draft
  |
  v
validating --invalid--> draft(admission_error)
  |
  v
queued --------> running <-----------------------------+
  |                |   |                                |
  |                |   +--> waiting --------------------+
  |                |          |                         |
  |                |          +--> paused --> queued ---+
  |                |
  |                +--> completing --> succeeded
  |                +--> failed
  |
  +-----------> canceling --all effects final--> canceled
                        |
                        +--unknown/reconcile required--> waiting(external_effect)
                                                             |
                                                             +--> canceling

running / waiting / paused --cancel requested--> canceling
```

`waiting` 必须带结构化 `wait_reason`：

```text
dependency | resource | human_gate | child_nodes
external_effect | retry_backoff | operator_pause | input_too_large
```

不允许只有一段自由文本解释等待原因。`wait_details` 只能保存安全的对象 ID、deadline、quota key 和错误码。取消请求已经写入后，`waiting(external_effect)` 只允许对账、登记人工对账结论或继续取消；不得恢复执行步骤、创建新步骤或再次提交外部操作。所有关联外部操作进入终态后，服务端才把执行实例送回 `canceling` 并完成取消。

## 4. 执行计划版本（`JobPlanRevision`）与执行图

### 4.1 初始编译

现有 SOP 按阶段的 `Order` 排序后编译为等价链，确保打开功能开关时 V7 行为不变：

```text
SOP Stage[0] -> Stage[1] -> ... -> Stage[n]
     |              |
     v              v
  NodeSpec        NodeSpec
```

每个 `NodeSpec` 固定：

- `node_key`、`kind`、业务 `stage_id` 和显示名。
- 输入选择器、输出 Schema、完成策略和人工审批节点。
- 执行器类型、所需能力和 `ExecutionProfile`。
- 重试、超时、失败域、资源请求和成本策略。
- 可读 `StateCollection`、可写 `StateCollection` 和 `ContextView` 策略。

### 4.2 节点种类

首版只支持五种：

| Kind | 执行语义 |
| --- | --- |
| `agent` | 通过 `AgentHarnessAdapter` 执行，允许受限的子任务提议 |
| `deterministic` | 服务端或本地白名单工作进程；输入输出都有固定格式 |
| `provider` | 提交/轮询/下载外部服务商作业，必须关联外部操作记录 |
| `human_gate` | 人工审批节点；不占用工作进程，等待明确的人工决定 |
| `join` | 服务端确定性汇聚/归并，不调用 LLM |

智能体的内部推理步骤不自动变成 `JobNode`。只有需要独立租约、输出、预算、失败恢复或审计的工作，才升级为节点。

### 4.3 Revision

`JobPlanRevision` 是不可变文档：

```text
revision_no
parent_revision_id
sop_id / sop_version / sop_digest
nodes_digest / edges_digest / plan_digest
created_by / created_at
change_reason / source_patch_id
```

当前 `graph_version` 指向最新已接受的版本。`NodeRun` 始终记录自己创建时的版本，不因为后续追加节点而改变历史契约。

## 5. 受限图变更（`GraphPatch`）

主控智能体不能直接改节点或边，只能通过运行时网关提交命令：

```json
{
  "expected_graph_version": 3,
  "idempotency_key": "audience-fanout-v1",
  "reason": "为已批准的三个受众创建脚本候选",
  "add_nodes": [],
  "add_edges": [],
  "cancel_pending_node_keys": []
}
```

服务端在一个事务内验证：

1. 申请中的预期版本与当前执行图版本一致。
2. `GraphPatch` 只新增节点或边，或者取消尚未取得租约的节点。
3. 新增边的下游必须是本次申请新增的节点，不得给已有节点追加前置依赖；已有节点的规格和依赖始终保持不变。
4. 已开始/完成节点、输出 Schema、权限和预算不能修改。
5. 合并后仍为 DAG；不存在自引用边、重复边或跨执行实例引用。
6. 输入选择器可由上游输出或已有状态满足。
7. 执行能力、执行器、模型、工具、网络出口和服务商均来自白名单内的执行配置。
8. `max_nodes`、`max_depth`、`max_fanout`、并发和预算不会越界。
9. Patch 摘要和幂等键没有与不同请求复用。

校验成功后写入新的 `JobPlanRevision`、`NodeSpec`/Dependency、图版本和 `JobEvent`；任何一步失败都不留下半张图。

### 5.1 首版不支持

- 删除已经存在的节点或边。
- 修改运行中节点的 Prompt、Schema、权限或资源请求。
- 任意条件表达式或用户脚本 DSL。
- 图内循环；重试由 RetryPolicy 表达，业务返工创建新节点或分支 `JobRun`。
- 智能体自行发布 State Schema 或 `ExecutionProfile`。

## 6. 并行拆分与汇聚

### 6.1 冻结成员集合

并行拆分不能以“当前查询到多少条”为成员依据，否则运行中新增记录会让汇聚提前或永远无法完成。拆分节点必须先封闭一个不可变的 `FanoutSet`：

```text
StateCollection@watermark
  -> FanoutSet
       id
       source_collection + source_version
       item_keys[] + item_digests[]
       membership_digest
       closed_at
  -> 子 `NodeRun`
```

子节点幂等键：

```text
sha256(job_run_id, map_node_key, fanout_set_id, item_key, generation)
```

调度器、智能体或网络重试都只能得到同一个子节点。

### 6.2 汇聚策略

汇聚等待的是已关闭 `FanoutSet` 的成员，而不是任意子节点计数。每个汇聚步骤在发布时必须固定 `zero_member_policy` 和（使用 quorum 时）`quorum_stop_policy`；运行时不能临时猜测：

- `zero_member_policy` 默认 `fail`；只有输出 Schema 明确允许空结果时，才能选择 `succeed_empty`。成员数为 0 时先处理该策略，不计算比例，也不按“0/0 达标”自动成功。
- `quorum_stop_policy` 只能是 `wait_all_terminal` 或 `cancel_pending`。后者达到阈值后只能在同一事务中取消尚未领取的 `pending/ready` 成员；已租约或运行中的成员必须先到达终态，过期租约和迟到上报不得改变已经冻结的汇聚输入。

首版策略：

| 策略 | 完成条件 |
| --- | --- |
| `all` | 所有成员成功；任一永久失败则汇聚失败 |
| `min_success(n)` | 至少 n 个成功，剩余成员达到终态 |
| `quorum(percent)` | 需要成功数为 `ceil(member_count * percent / 100)`；`percent` 必须在 1 到 100。按已声明的停止策略处理剩余成员后才汇聚 |
| `best_effort` | 所有成员进入终态后聚合成功项；至少一个成功 |
| `fail_fast` | 第一个永久失败后取消仍未开始的同级节点 |

汇总工作由已经发布的固定函数或独立节点完成，不允许多个工作进程直接覆盖同一个结果。

若永久失败已经使“当前成功数 + 未终态成员数”小于所需成功数，汇聚立即失败并按失败策略处理尚未开始的成员；不能继续等待一个不可能达到的 quorum。

## 7. 执行步骤（`NodeRun`）与执行尝试（`RuntimeAttempt`）

### 7.1 NodeRun 状态机

```text
pending --dependencies ready--> ready --claim--> leased --> running
                                  ^        |          |
                                  |        |          +--> waiting_child
                                  |        |          +--> waiting_gate
                                  |        |          +--> waiting_effect
                                  |        |          +--> waiting_input
                                  |        |          +--> succeeded
                                  |        |          +--> failed
                                  |        |          +--> canceling -> canceled
                                  |        |
                                  +--------+-- lease expired
                                  |
                                  +-- retry_wait <-- retryable failure
```

等待子节点、人工审批节点或外部操作时，智能体必须提交结构化等待条件，结束当前 `Attempt` 并释放执行资源。上下文仍然超限时，执行步骤进入 `waiting_input`；如果没有其他可运行执行步骤，执行实例显示为 `waiting(input_too_large)`。依赖满足或输入被压缩、拆分后，同一个 `NodeRun` 和 `AgentInstance` 通过新的 `Attempt` 恢复；主控智能体不能长期占用执行名额。

### 7.2 RuntimeAttempt 状态机

```text
prepared -> running -> succeeded
    |          |----> retryable_failed
    |          |----> failed
    |          |----> cancelled
    |          +----> expired
    |
    +---------------> retryable_failed / failed / cancelled / expired
```

`prepared` 表示数据库已经原子保存 Node 租约、ContextView、AgentInstance 绑定、RuntimeAttempt 和 JobEvent，但外部 Harness 尚未确认启动。它专门覆盖“数据库提交后、外部进程启动前后”这一崩溃窗口。

当前实现使用 `retryable_failed` 结束本次 Attempt 并把逻辑 Agent 送回 `runnable`。后续实现主动让出资源时，可以增加明确的 `yielded` 终态；在该状态进入代码前，文档和 API 不得假装已经支持。主动让出必须同时提交：

- 最新的结构化进度。
- 等待条件。
- 智能体会话引用或可恢复的上下文摘要。
- 状态和事件水位。
- 不存在未登记的外部操作声明。

`RuntimeAttempt` 至少保存 `job_run_id/node_run_id/agent_instance_id/context_view_id`、`attempt_no`、Harness 能力快照、不透明 `session_ref`、租约、输出引用、结果摘要、错误码和版本。`NodeRun` 与 `RuntimeAttempt` 使用同一 owner 和到期时间续租；任一版本或 owner 不匹配时，旧 worker 必须失败关闭。

旧 V7 `RunAttempt` 的 `RunToken`、本地守护进程日志和上报 API 继续保留，但不复用为 V8 RuntimeAttempt。两套模型不能共享表、外键或权威状态。

## 8. 智能体实例（`AgentInstance`）

`AgentInstance` 表示逻辑身份，不等于操作系统进程或某一轮模型调用：

```text
created -> runnable -> active
active -> waiting_children / waiting_gate / waiting_effect -> runnable
active -> completed / failed / canceling -> cancelled
```

建议字段：

| 字段 | 说明 |
| --- | --- |
| `job_run_id` / `node_run_id` | 运行时归属 |
| `parent_agent_instance_id` | 主控智能体与执行智能体的层级关系 |
| `role` | supervisor、researcher、writer、reviewer 等发布角色 |
| `harness_kind` / `session_ref` | Codex/Claude/Fake 及其不透明会话 ID |
| `execution_profile_id` | 模型、工具、沙箱、出口和预算 |
| `context_view_id` | 本次激活的可见输入 |
| `depth` / `remaining_descendants` | 当前深度和剩余派生能力；最大深度来自执行计划 |
| `state` | 逻辑生命周期 |

子智能体的策略只能是父策略和节点策略的交集。运行时拒绝模型、工具、状态范围、网络出口或预算的权限提升。

当前落地边界：`runtime_context_views`、`runtime_agent_instances` 与 `runtime_attempts` 已通过复合外键和强制 RLS 持久化。Runtime 会原子准备 Node/ContextView/AgentInstance/RuntimeAttempt/Event，在 Harness 启动后原子激活，并在终态事件到达时原子完成 Node/Attempt/Agent；租约过期会将 Attempt 置为 `expired`、Node 重新送回就绪判断，并将活跃逻辑 Agent 送回 `runnable`。同一 Node 重试会创建新的 Attempt 和 ContextView，但复用同一个根 AgentInstance。真实 Codex/Claude SDK 会话恢复、主动等待条件、资源预留和 `yielded` 语义尚未实现。

## 9. 执行事件（`JobEvent`）

V8 不把整个系统改造成事件溯源。在线状态表仍是查询和调度的权威数据源；`JobEvent` 只记录追加式运行历史，用于诊断、投影校验和逻辑重放。

每个事件至少包含：

```text
event_id / job_run_id / sequence
event_type / occurred_at / committed_at
actor_type / actor_id
node_run_id / attempt_id / agent_instance_id (optional)
payload_schema / payload_digest / safe_payload
causation_id / correlation_id / idempotency_key
```

同一个 `JobRun` 的 `sequence` 在数据库事务内单调增加。以下变化必须与事件在同一事务中提交：

- JobRun/NodeRun/RuntimeAttempt 状态迁移。
- `GraphPatch` 接受或拒绝。
- `StateMutation` 和输出绑定。
- `ResourceReservation` 获取/释放。
- `Checkpoint` 创建。
- 外部操作记录状态变化。

原始 Prompt、完整对话记录、Secret、RunToken、签名 URL 和大媒体不进入脱敏事件内容。

## 10. StageRun 投影

`StageRun` 继续服务内容工作台（Content Work OS）用户，不把运行时细节直接暴露成业务状态：

```text
StageRun.pending
  = 对应 NodeRun 尚未满足依赖

StageRun.running
  = 至少一个对应 `NodeRun` 处于 leased 或 running，且没有阻断性的人工审批节点

StageRun.waiting_gate
  = 阻断性 `human_gate` 未决定

StageRun.blocked
  = 所需输入、资源或外部操作进入不可自动恢复状态

StageRun.completed
  = 阶段完成策略满足，且输出契约通过
```

多个动态节点可以属于同一阶段。`current_stage_id` 在兼容 API 中只表示用户最需要关注的业务阶段，不再决定调度器下一步。

## 11. 核心不变量测试

- 任意 `JobPlanRevision` 都必须通过无环检测和 Schema 可达性检查。
- `GraphPatch` 不能给已有节点追加前置依赖，即使该节点尚未取得租约。
- 相同幂等键和内容摘要的 `GraphPatch` 只创建一个版本；幂等键相同但内容摘要不同则返回冲突。
- 同一个 `FanoutSet` 成员键不会创建第二个 `NodeRun`；零成员按已发布的 `zero_member_policy` 处理。
- quorum 使用封闭成员数和向上取整的阈值；达到阈值后的取消只能影响尚未领取的成员，永久失败使阈值不可能达到时必须失败。
- 进入终态的 `JobRun`、`NodeRun` 和 `RuntimeAttempt` 不能回到非终态；相同结果摘要的重复终态上报返回幂等成功，不同摘要返回冲突。
- 一个 `NodeRun` 同时最多有一个有效租约；过期令牌不能写入状态或输出。
- `NodeRun` 成功必须满足输出契约；业务交付仍必须满足人工审批节点和正式产物（`Artifact`）要求。
- `StageRun` 投影删除后可以从 `JobRun`、`NodeRun`、人工审批节点和输出重新构建。
