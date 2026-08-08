# 09：Runtime Infra V2：可恢复执行内核升级

状态：`I0 已冻结；I1 命令内核与 outbox 消费协议已实现，待提交后进程终止注入、投影消费与并发验收`。

更新时间：2026-08-08。

本文不是另起一套 V9，也不是把 ContentCloud 改造成通用工作流平台。它是对 V8 的基础设施升级：把已经存在的 JobRun、NodeRun、RuntimeAttempt 和 Harness 闭环，收敛为一套可以长期运行、跨进程恢复、可对账、可重放、可逐步扩展的 Durable Runtime 内核。

## 1. 结论先行

V8 当前已经证明了“模型和数据库状态可以完成一次调度闭环”，但还没有证明“系统可以在真实生产故障中可靠地继续推进”。下一阶段的主目标不应是先做动态图、更多 Agent 或更复杂的运营页面，而是先补齐以下五个底层保证：

1. **权威状态只有一个写入口**：状态变更、事件、资源释放和投影通知在同一事务中提交，不能出现“状态已变、事件丢失”。
2. **每次执行都可被围栏（fencing）**：租约过期或执行器重启后，旧 worker 的迟到写入必须被拒绝；`owner + version` 之外增加不可猜的 `fence_token`。
3. **调度只领取已被资源账本允许的工作**：租户、Job、能力、区域、执行器、Token、费用和服务商并发都必须在领取前预留，不能先执行再补账。
4. **外部副作用具有结果不明语义**：超时只能进入 `unknown`，由持久化对账器确认后才能重试；重放和分支绝不触发外部调用。
5. **恢复依赖 ContentCloud 记录，不依赖宿主记忆**：宿主会话可以恢复是优化项，Node/State/Effect/Checkpoint 才是业务恢复事实。

推荐的技术路线是 **PostgreSQL-first、模块化单体、事件驱动投影**：PostgreSQL 保存权威快照、追加事件、租约和 outbox；Blob 只保存大对象；worker 和 Harness 通过窄协议连接。暂不引入 Temporal、Kafka 或新的消息数据库，等吞吐、故障隔离或团队边界由真实指标证明后再拆服务。

## 2. 当前实现对账

以下判断来自当前工作区代码与迁移，不把路线图目标当成已实现事实。

| 层 | 当前已经存在 | 仍然不能宣称的能力 | 证据 |
| --- | --- | --- | --- |
| 领域内核 | JobPlan、JobRun、NodeRun、JobEvent、State、Checkpoint、Effect 模型和状态转移；Service 已切到 `RuntimeCommandStore` | PG 提交后故障注入、投影消费和完整命令契约矩阵 | `internal/domain/runtime.go`、`internal/runtime/commands.go`、`internal/runtime/service.go` |
| 持久化 | PostgreSQL 迁移 `00014`～`00020`、RLS、复合外键、追加事实权限收敛、Memory/PostgreSQL Store、带消费者租约的 `runtime_outbox` | 计划版本的关系化图存储、分支、Fanout/Join、资源账本 | `migrations/00014_agentic_job_runtime.sql`～`00020_runtime_append_only_permissions.sql` |
| 调度 | FakeHarness 的 Prepare/Start/Activate/Heartbeat/Finalize、租约过期回收 | 跨 Job 公平性、资源预留、配额、防超卖、可恢复调度队列 | `internal/runtime/dispatch.go`、`internal/store/postgres/runtime_dispatch.go` |
| 宿主执行 | 结构化事件接口、进程级 HarnessRegistry、FakeHarness | Codex/Claude 跨进程会话恢复、主动让出、SessionStore | `internal/agentadapter/harness.go`、`harness_registry.go` |
| 状态与上下文 | JSON 集合 CAS、引用型 ContextView、父子预算/工具子集校验 | 类型化 StateRecord、写入策略、网关授权、大小/保留/水位线治理 | `internal/runtime/context.go`、`service.go` |
| 外部操作 | Effect 状态机、幂等键、请求/响应摘要字段 | ToolCall、ResourceReservation、持久化轮询/回调、账单核对和补偿 | `runtime_effects`、`Service.RegisterEffect` |
| 运营读取 | Runtime REST、SSE 游标、脱敏 Agent/ContextView 摘要 | 图/状态/费用读模型、投影延迟、对账动作和分支执行 | `internal/httpapi/runtime_handlers.go`、`web/src/admin/views/AdminRuntimePage.tsx` |

升级前基线的全量 `go test ./...` 与 `pnpm architecture` 曾通过；这只是回归基线，不是生产门槛。本轮 Runtime 定向测试和架构检查通过；全量结果仍受工作区其他未提交的 CLI/插件迁移改动影响，本升级不改变这些改动。

I1 当前已落地：`RuntimeCommandStore`、事件与 outbox 同事务写入、持久化 outbox 消费者租约（`claim/ack/retry`）、Memory/PostgreSQL 实现，以及 Service 主要写路径迁移。仍未宣称 I1 完成，直到补齐提交后进程终止注入、投影消费和迁移后的并发验证。

## 3. 目标架构

```text
                         Commands / MCP / Worker callbacks
                                      |
                                      v
┌──────────────────────────────────────────────────────────────┐
│ Runtime Command Kernel                                       │
│ Admission · Transition · Dispatch · State · Effect · Fork   │
│ 每个命令：校验 -> 锁定聚合 -> CAS -> 快照/事件/outbox 同事务 │
└───────────────┬───────────────────────────┬──────────────────┘
                |                           |
                v                           v
┌──────────────────────────┐     ┌─────────────────────────────┐
│ PostgreSQL Authority     │     │ Durable Dispatch             │
│ aggregate snapshots      │     │ scheduler + quota ledger     │
│ append-only events       │     │ lease + fence token + reaper │
│ state/effect/reservation │     │ inbox/outbox                 │
└───────────────┬──────────┘     └──────────────┬──────────────┘
                |                                 |
                v                                 v
       ┌──────────────────┐              ┌────────────────────────┐
       │ Projection workers│              │ Execution adapters       │
       │ customer/ops/SSE  │              │ deterministic / agent / │
       │ rebuild + reconcile│              │ provider / human        │
       └──────────────────┘              └───────────┬────────────┘
                                                    |
                                                    v
                                      structured result / callback
                                      -> command kernel (never direct DB)
```

模块仍然可以部署在 `contentcloud-server` 和 `contentcloud-worker` 内。网络拆分不是本阶段目标；端口和事务边界必须先稳定。

## 4. 四条权威边界

### 4.1 Definition、Plan、Run、Projection 分离

```text
ExperienceTemplateVersion + SOPVersion
              -> deterministic compiler
              -> JobPlanRevision (immutable)
              -> JobRun admission snapshot (binding/policy/input digests)
              -> NodeRun / Attempt / State / Effect
              -> customer + operations projections
```

- `JobPlanRevision` 一旦被 JobRun 引用不可变；GraphPatch 只生成新 revision。
- JobRun 必须固定 `plan_digest`、`binding_digest`、`runtime_policy_id`、输入快照摘要、契约 major/minor 和父/根执行引用。
- 客户阶段、创作结果、交付和资产目录只能消费 Runtime 引用，不能回写 Runtime 状态。
- 投影允许重建和延迟；权威命令不能依赖投影已经追平。

### 4.2 Snapshot + Event + Outbox

每个写命令必须产生以下原子结果：

```text
aggregate snapshot/version
  + append-only JobEvent(sequence)
  + outbox message(event_id, projection/reconcile target)
```

禁止在 application service 中先 `Save*`，再单独 `AppendJobEvent`。当前 `TransitionNode`、`CompleteNode`、`FailNode`、`MutateState` 和 `ReconcileEffect` 已改为调用 `RuntimeCommandStore` 的事务命令；Service 仍负责读取、领域校验和构造事件，不能再直接组合宽写方法。后续新聚合必须沿用同一端口形态，例如：

```go
type RuntimeCommandStore interface {
    ApplyJobTransition(context.Context, domain.JobRun, int, domain.JobEvent) (domain.JobRun, error)
    ApplyNodeTransition(context.Context, domain.NodeRun, int, domain.JobEvent) (domain.NodeRun, error)
    ApplyStateMutation(context.Context, string, string, domain.StateMutation, domain.JobEvent) (domain.RuntimeState, error)
    RegisterEffectCommand(context.Context, domain.ExternalEffect, domain.JobEvent) (domain.ExternalEffect, error)
    ApplyEffectTransition(context.Context, domain.ExternalEffect, int, domain.JobEvent) (domain.ExternalEffect, error)
}
```

接口内部使用数据库事务；Harness、Provider 和 HTTP 请求不在事务内执行。Memory Store 必须通过同一组契约测试模拟提交原子性和崩溃窗口。

### 4.3 Lease、Generation、Fence

所有可执行对象使用三元围栏：

```text
lease_owner + lease_expires_at + fence_token
```

- 每次 Prepare 或重新领取生成新的随机 `fence_token`，写入 NodeRun、Attempt、Reservation 和本地执行信封。
- Heartbeat、事件上报、Finalize、Cancel、Reconcile 都必须携带 token；token 不匹配直接返回稳定错误码。
- 到期回收先写终态/释放资源，再允许新 Attempt 领取；旧 worker 即使尚未被 reaper 处理，也不能提交迟到结果。
- `owner + version` 保留作为并发 CAS，但不再单独承担执行隔离。

### 4.4 业务状态与宿主状态分离

宿主 Thread/Session、stdout、完整聊天和本地工作区都属于执行证据或临时状态。它们可以丢失；ContentCloud 仍能依靠 `NodeResult`、StateMutation、EffectReceipt 和 Checkpoint 继续或人工接管。宿主会话恢复通过 `AgentHarnessAdapter` 提供，但不得改变 JobRun 的业务语义。

## 5. V2 领域与存储模型

### 5.1 先补控制面关系，再保留大字段引用

当前 `runtime_job_plans.nodes/edges` 和 `runtime_states.values` 都是 JSONB。JSONB 适合保存不稳定 payload，但不适合作为调度、并发、授权和查询的唯一控制面。建议新增迁移，不删除旧列：

| 新对象 | 关键字段 | 目的 |
| --- | --- | --- |
| `runtime_plan_revisions` | `job_run_id/base_revision/graph_version/digest/created_by` | 计划历史和 GraphPatch CAS |
| `runtime_plan_nodes` | `revision_id/key/kind/depends_on/output_schema/policy_digest` | 节点可达性、版本和约束查询 |
| `runtime_plan_edges` | `revision_id/from_key/to_key` | 关系化边和唯一约束 |
| `runtime_fanout_sets/members` | `set_id/member_key/frozen_digest/status` | 成员封存和 Join 输入冻结 |
| `runtime_resource_reservations` | `resource_key/quantity/owner/fence/state/expires_at` | 配额和资源账本，拒绝超卖 |
| `runtime_tool_calls` | `attempt_id/tool_name/request_digest/result_digest/state` | 工具授权、幂等和审计 |
| `runtime_outbox` | `event_id/topic/payload/attempts/next_attempt_at/locked_by/locked_until/delivered_at` | 投影、回调、对账的可靠投递与消费者围栏 |
| `runtime_inbox` | `consumer/message_id/received_digest` | 回调和事件去重 |
| `runtime_state_collections` | `scope/schema_revision/writer_policy/retention` | 集合级类型和写入治理 |
| `runtime_state_records` | `collection/key/value_ref/revision/digest` | 小记录 CAS，大值只存引用 |

旧 `runtime_job_plans` 可作为兼容读取源，直到新 revision 投影连续对账通过；旧 `runtime_states` 通过一次性导入成为 collection snapshot，不回填虚假的历史 mutation。

### 5.2 JobRun admission snapshot

`StartInput` 不能只依赖 SOP 和幂等键。准入时必须保存：

```text
experience_template_ref
sop_ref + sop_digest
plan_revision_ref + plan_digest
execution_binding_digest
runtime_policy_id + policy_digest
input_snapshot_refs + digests
contract_versions
budget_ceiling + reservation_summary
```

同一幂等键如果输入摘要、租户、项目或计划摘要不同，必须返回冲突，不能像当前 `Start` 一样直接复用已有 JobRun。

### 5.3 StateCollection

首版只支持三种写入模式：

| 模式 | 规则 |
| --- | --- |
| `cas_record` | 记录级版本比较并交换；冲突不自动合并 |
| `append_only` | 只追加带幂等键的事实，读取由服务端排序/汇总 |
| `single_writer` | 只有声明的节点或归并节点可写，其余只能读 |

集合必须有 `schema_revision`、最大记录/字段大小、保留策略、可读角色、写入者和数据分类。任意 JSON 覆盖、跨 Job 写入、把完整正文塞入运行状态都拒绝。ContextView 只携带引用、摘要和网关令牌，不携带密钥或完整对话。

### 5.4 ExternalEffect 与 ToolCall

`ToolCall` 是执行尝试内的一次工具交互，`ExternalEffect` 是可能改变外部世界的操作。两者不能只用一个通用 JSON 记录。

```text
ToolCall: proposed -> authorized -> executing -> succeeded/failed
Effect:   planned -> authorized -> submitted -> acknowledged
                    -> succeeded/failed
                    -> unknown -> reconciling -> succeeded/failed/manual_action
```

Effect 必须绑定 `node_run_id/attempt_id/resource_reservation_id`，保存请求摘要、稳定幂等键、服务商绑定、账单摘要和对账截止时间。`unknown` 不提供盲重试动作；只能由 Reconciler 或授权运维命令推进。

## 6. 调度和执行协议

### 6.1 Ready evaluator 与公平调度

Ready evaluator 只计算候选，不取得执行权；Scheduler 在一个事务中完成：

```text
candidate selection
  -> tenant/job/provider/resource quota check
  -> reservation rows insert/update
  -> NodeRun + Attempt + Agent + fence token
  -> event + outbox
```

选择策略先采用 PostgreSQL `FOR UPDATE SKIP LOCKED`，排序键固定为 `effective_priority, waiting_since, created_at, id`。`effective_priority` 必须受租户上限和 aging（等待时间提升）限制，避免大任务长期独占资源。每个资源键都必须能查询“已用、已预留、上限、释放原因”。

### 6.2 Harness 三阶段协议升级

保留现有 Prepare/Start/Activate/Finalize，增加 `fence_token`、`execution_envelope_digest` 和可恢复事件游标：

```text
Prepare(tx) -> dispatch envelope persisted
Start(outside tx) -> opaque session ref
Activate(tx) -> session + token fenced
Heartbeat(tx) -> lease + progress cursor
Yield(tx) -> release reservations, Agent runnable
Resume(outside tx) -> new Attempt or supported session resume
Finalize(tx) -> result envelope + receipts + release resources
```

`Yield` 是新语义：主控 Agent 在等待子节点、人工 Gate 或外部 Effect 时，必须释放稀缺 Agent 并发名额；不能通过长心跳占住资源。真实宿主不支持 `Resume` 时，必须使用新 Attempt + 新 ContextView 恢复，且记录降级原因。

### 6.3 Scheduler、Reaper、Reconciler 分工

- **Scheduler**：只负责领取、预留和分派，不解析宿主私有事件。
- **Reaper**：扫描到期 lease/reservation，使用 fence token 原子收敛 Attempt/Node/Agent/Resource。
- **Reconciler**：处理 `unknown` Effect、Provider callback、账单差异和本地 outbox 失败。
- **Projector**：从 JobEvent/outbox 重建客户与运营读模型；停止时不影响权威状态。

四类 worker 都必须幂等、可水平扩展，并把失败重试写回 `next_attempt_at`；不能依赖内存定时器或单进程 Registry 才能恢复。

## 7. 实施顺序（V8.1 overlay）

旧 W8 工作包继续作为能力清单；以下 overlay 是进入 W8-07～W8-10 之前的基础顺序。

| 阶段 | 交付 | 退出条件 |
| --- | --- | --- |
| I0 权威边界冻结 | ADR、命令清单、错误码、聚合 owner、事件版本、指标基线 | 每个写入路径只有一个 owner；所有新字段都有 digest/版本/回退说明 |
| I1 事务命令内核 | 聚合级 RuntimeCommandStore、JobEvent + outbox 同事务、Memory/PG 契约测试 | 注入任意提交后崩溃，不能出现快照和事件不一致；事件失败可重试 |
| I2 围栏与资源账本 | fence token、reservation、reaper、公平调度、租户/Job/Provider 配额 | 20 worker/多租户竞争无双租约、无超卖、无旧 owner 写入；小任务有进展 |
| I3 状态、Effect、恢复 | typed collection、ToolCall、Effect Reconciler、Checkpoint watermark、Fork | CAS/追加/归并有策略；unknown 不盲重试；重放和分支零外部调用 |
| I4 执行器与投影 | 至少一个可跨进程恢复 Harness、Provider 测试适配器、outbox projector、Runtime Explorer 读模型 | 重启 server/worker 后可继续；投影可重建；延迟和积压可观测 |
| I5 第二业务流与 Canary | 文章复盘或知识复盘流程、故障注入、灰度、回退和运维手册 | 第二流程不改 Runtime 状态机；Canary 可停止新准入并向前恢复 |

依赖图：

```text
I0 -> I1 -> I2 -> I3 -> I4 -> I5
             |           |
             +-> W8-04   +-> W8-11/W8-12
I3 通过后才允许 W8-10 dynamic_graph
```

### 7.1 第一批应实现的最小切片

第一批只选一个低风险、无真实外部扣费的线性 SOP，完成：

```text
admit -> claim/reserve -> deterministic worker -> heartbeat
      -> finalize(result) -> event/outbox -> projection -> restart/recover
```

FakeHarness 继续作为 CI 基础；真实宿主和真实 Provider 不作为第一批的必要依赖。先证明“一个节点故障后能继续，且不重复产出”，再引入多节点和外部副作用。

### 7.2 明确不做

- 不先拆微服务，不先引入 Kafka/Temporal。
- 不把宿主的 Team、Thread、Task list 或聊天记录当作 Runtime 权威。
- 不在同一轮同时重写领域模型、客户产品和存储驱动；先建立端口和事务命令，再迁移实现。
- 不通过长期双写解决迁移问题；双写只能是有期限、可度量、可关闭的兼容窗口。
- 不把“能跑通 FakeHarness”写成“支持生产恢复”。

## 8. 生产门槛与故障矩阵

进入真实租户前必须通过：

| 类别 | 必测故障 | 预期 |
| --- | --- | --- |
| 事务 | snapshot 已提交、event/outbox 未提交时进程终止 | 整体回滚或通过 outbox 重试恢复，不能部分成功 |
| 围栏 | lease 到期后旧 worker 迟到 Finalize | 稳定返回 `DISPATCH_FENCE_STALE`，不改变结果 |
| 调度 | 多租户/多 Provider 并发抢占 | 不超卖；租户 aging 保证小任务 eventually runs |
| 宿主 | Start 后、Activate 前重启；跨进程 Resume | Attempt 可回收；支持恢复则续会话，否则新 Attempt + ContextView |
| 状态 | 两个写者 CAS、重复 mutation、超大值 | 一个成功、一个冲突；重复幂等；大值转 ArtifactRef |
| 外部 | Submit 超时、重复 callback、账单不一致 | `unknown` + reconcile；只保留一个终态；账单差异进入人工处理 |
| 重放 | 重建投影、从 checkpoint fork | 执行器调用数为 0；源 JobRun 不改变 |
| 安全 | 跨租户引用、工具越权、提示词注入、SSRF | RLS/网关/connector 拒绝；事件和日志无秘密 |
| 运维 | outbox 积压、projection 延迟、reaper 停止 | 权威状态仍一致；告警可定位；恢复后可追平 |

## 9. 需要先冻结的 ADR

1. [ADR-0016](../../foundation/decisions/ADR-0016-runtime-command-kernel.md)：RuntimeCommandStore 的事务边界与事件版本。
2. `ADR-00xx`：PostgreSQL-first 调度、outbox、reaper 和 reconciler，不引入外部工作流引擎。
3. `ADR-00xx`：Fence token、执行信封和迟到上报拒绝规则。
4. `ADR-00xx`：StateCollection 的三种写入模式和 schema 发布流程。
5. `ADR-00xx`：Effect/ToolCall/ProviderAttempt 的所有权和 unknown 对账责任。
6. `ADR-00xx`：真实 Codex/Claude 适配器的认证、数据保留、跨进程 SessionStore 和商业边界。

## 10. 完成定义

V8.1 基础设施完成，不等于全部 V8 产品能力完成。只有同时满足以下条件，才可以打开动态执行图或对外宣称“可恢复运行时”：

- 所有权威写入都由事务命令完成，状态、事件和 outbox 可一起重试。
- 旧 owner、旧版本和旧 fence token 的写入全部拒绝。
- 资源预留、释放、租约回收和费用核算没有超卖或重复记账。
- 至少一个执行适配器可跨进程恢复，或明确记录新 Attempt 降级恢复。
- `unknown` 外部操作必须先对账；重放、投影重建和分支不会产生外部调用。
- Memory/PostgreSQL 契约测试、故障注入、RLS、容量、指标、Canary 和回退演练通过。
- 第二种结构不同的业务流只增加业务包、Schema 和 Gate，不增加 Runtime 专用状态机。
