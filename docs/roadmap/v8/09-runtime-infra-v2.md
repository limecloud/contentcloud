# 09：Runtime Infra V2：可恢复执行内核升级

状态：`I0 已冻结；I1～I4 核心实现、I5 第二业务流容量边界切片已落地；真实 Provider/SDK 端到端、PostgreSQL 故障与 RLS/容量验收、Canary 未完成`。

更新时间：2026-08-09。

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
| 领域内核 | JobPlan、JobRun、NodeRun、JobEvent、State/StateCollection/StateRecord、Checkpoint、Effect、ToolCall 的状态转移；主要写路径已切到 `RuntimeCommandStore` | PostgreSQL 提交后故障注入、完整命令契约矩阵 | `internal/domain/runtime.go`、`internal/runtime/commands.go`、`internal/runtime/service.go` |
| 持久化 | PostgreSQL 迁移 `00014`～`00030`、RLS、复合外键、JobRun 准入冻结字段、追加事实权限、Memory/PostgreSQL Store、outbox 租约、资源账本、typed state、ToolCall、Runtime Explorer 快照、关系化计划 revision、Fanout/Join、Provider inbox/账单、Yield、投影重建事实、SessionStore 镜像表 | 真实数据库故障注入、RLS 越权和迁移历史重建 | `migrations/00014_agentic_job_runtime.sql`～`00030_runtime_session_store.sql` |
| 调度 | FakeHarness 的 Prepare/Start/Activate/Heartbeat/Finalize、owner/version/fence 围栏、资源预留与释放/消费/过期、优先级 aging 排序、租约回收 | PostgreSQL 多 worker 压测、跨租户公平性生产指标 | `internal/runtime/dispatch.go`、`internal/store/postgres/runtime_dispatch.go`、`runtime_resources.go` |
| 宿主执行 | 结构化事件接口、进程级 HarnessRegistry、FakeHarness、`DurableHarness` 的本地/SessionStore 镜像、跨进程 Resume、Yield/Resume Runtime 边界 | Codex/Claude 真实 SDK 会话恢复、真实宿主故障演练 | `internal/agentadapter/harness.go`、`harness_registry.go`、`durable_harness.go`、`session_store.go` |
| 状态与上下文 | StateCollection（四种一致性策略）、StateRecord CAS、引用型 ContextView、父子预算/工具子集校验，状态写入与事件/outbox 同事务 | Runtime state gateway 授权、生产 schema 发布/保留策略、Artifact 大值链路 | `internal/domain/runtime.go`、`internal/store/*/runtime_state_tools.go` |
| 外部操作 | Effect 状态机（unknown/reconciling 禁止盲重试）、ToolCall 状态机、Effect 的 Attempt/Reservation 绑定、Provider inbox 去重、Provider reconciliation、账单匹配/差异/无匹配记录、资源账本 | 真实服务商端到端回调/账单和补偿演练 | `runtime_effects`、`runtime_tool_calls`、`internal/runtime/provider.go`、`internal/store/*/runtime_provider.go` |
| 运营读取 | Durable outbox Projector、Runtime Explorer 持久化投影、投影延迟/积压指标、REST/SSE 读取、Replay 投影重建和 dry-run、Checkpoint Fork、Effect/Provider 对账入口 | 图/状态/费用完整读模型、生产告警和支持案例 | `internal/runtime/projector.go`、`internal/store/*/runtime_projection*.go`、`internal/app/runtime_explorer.go` |

本轮 `GOMAXPROCS=2 go test -p 1 ./...`、`git diff --check` 和 `node scripts/check-architecture.mjs` 已通过。`pnpm architecture` 在当前机器被 Corepack 的 pnpm 签名校验阻断，因此不把它写成项目失败；Node 直接执行同一架构脚本作为等价验证。以上仍不是 PostgreSQL RLS、提交后崩溃、生产容量、真实 Provider 或 Canary 验收。

I1～I4 的核心切片已落地，I5 已补上文章复盘 50 节点并行分析及第二批超限保护测试：事务命令和 outbox `claim/ack/retry`、fence 与资源账本、typed state/ToolCall/Checkpoint/Fork/Replay、Provider inbox/账单、Yield/Resume、DurableHarness + SessionStore、Projector、关系化 GraphPatch、FanoutSet/Join 均有 Memory/PostgreSQL 实现或本地持久化实现及定向测试。剩余退出条件集中在真实 Provider/SDK 端到端、PostgreSQL 故障注入与 RLS、100 节点/20 并发公平性、生产告警和 Canary。

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
- 终态清除原始 lease/fence，仅在追加 JobEvent 保存 worker actor 与 fence SHA-256 摘要；相同终态重试先验证 actor、fence 摘要和结果摘要，再执行幂等业务提交。
- `owner + version` 保留作为并发 CAS，但不再单独承担执行隔离。

### 4.4 业务状态与宿主状态分离

宿主 Thread/Session、stdout、完整聊天和本地工作区都属于执行证据或临时状态。它们可以丢失；ContentCloud 仍能依靠 `NodeResult`、StateMutation、EffectReceipt 和 Checkpoint 继续或人工接管。宿主会话恢复通过 `AgentHarnessAdapter` 提供，但不得改变 JobRun 的业务语义。

## 5. V2 领域与存储模型

### 5.1 先补控制面关系，再保留大字段引用

`runtime_states.values` 仍只承载旧 RuntimeState 的非权威兼容 payload；计划 revision、节点/边、FanoutSet/Member 已关系化，JSONB 不再作为新增调度/授权控制面的唯一事实源：

| 新对象 | 关键字段 | 目的 |
| --- | --- | --- |
| `runtime_plan_revisions` | `base_revision_id/graph_version/patch_key/digest` | 计划历史和 GraphPatch CAS（已落地） |
| `runtime_plan_nodes` | `revision_id/key/kind/depends_on/output_schema` | 节点可达性、版本和约束查询（已落地） |
| `runtime_plan_edges` | `revision_id/from_key/to_key` | 关系化边和唯一约束（已落地） |
| `runtime_fanout_sets/members` | `set_id/member_key/membership_digest/request_digest/status` | 成员封存、确定性子节点和 Join 输入冻结（已落地） |
| `runtime_resource_reservations` | `resource_key/quantity/owner/fence/state/expires_at` | 已落地的配额和资源账本，拒绝超卖 |
| `runtime_tool_calls` | `attempt_id/tool_name/request_digest/result_digest/state` | 已落地的工具授权状态和审计 |
| `runtime_outbox` | `event_id/topic/payload/attempts/next_attempt_at/locked_by/locked_until/delivered_at` | 投影、回调、对账的可靠投递与消费者围栏 |
| `runtime_provider_inbox` | `provider_id/message_id/received_digest/external_id/state` | Provider 回调去重、结果摘要和安全载荷镜像 |
| `runtime_provider_reconciliations` | `effect_id/request_key/observed_state/expected_minor/observed_minor/status` | Provider 结果与预期请求/费用对账 |
| `runtime_provider_bills` | `provider_id/bill_id/external_id/bill_digest/amount_minor/status` | 账单匹配、差异和无匹配账单事实 |
| `runtime_yields` | `attempt_id/reason/wait_refs/state/resume_key` | 释放执行资源后的等待与恢复边界 |
| `runtime_projection_rebuild_runs` | `job_run_id/mode/status/event_count/last_sequence/external_calls/integrity_status` | 投影 rebuild/dry-run 的运行事实和零外部调用证明 |
| `runtime_agent_sessions/events` | `harness_kind/session_id/sequence/digest` | 宿主会话和事件的镜像观察层，不作为 Runtime 业务强事务事实 |
| `runtime_state_collections` | `scope/schema_revision/writer_policy/retention` | 已落地的集合级类型和写入治理 |
| `runtime_state_records` | `collection/key/value_ref/revision/digest` | 已落地的小记录 CAS，大值只存引用 |

`runtime_plan_revisions` 现在是 JobRun 引用的不可变计划事实源；`00025` 已一次性切换权威读写并删除旧 `runtime_job_plans`，不保留双读壳。`runtime_states.values` 只承载旧 RuntimeState 的非权威兼容 payload；在所有调用方迁到 collection/record 后直接删除，当前没有用户数据，不建设导入或回填兼容层。

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

同一幂等键如果项目、WorkTask、计划摘要、绑定摘要、输入摘要、Runtime policy、契约版本、业务类型、输入快照、输出上限或 priority 不同，`Start` 已返回 `JOB_RUN_IDEMPOTENCY_MISMATCH`；只有完整准入快照一致时才复用已有 JobRun。

### 5.3 StateCollection

当前支持四种一致性策略：

| 模式 | 规则 |
| --- | --- |
| `cas_map` | 记录级版本比较并交换；冲突不自动合并 |
| `append_only` | 只追加带幂等键的事实，读取由服务端排序/汇总 |
| `single_writer` | 只有声明的节点或归并节点可写，其余只能读 |
| `reducer_owned` | 由声明的归并节点产生汇总结果；普通节点不能覆盖 |

集合必须有 `schema_revision`、最大记录/字段大小、保留策略、可读角色、写入者和数据分类。任意 JSON 覆盖、跨 Job 写入、把完整正文塞入运行状态都拒绝。ContextView 只携带引用、摘要和网关令牌，不携带密钥或完整对话。

### 5.4 ExternalEffect 与 ToolCall

`ToolCall` 是执行尝试内的一次工具交互，`ExternalEffect` 是可能改变外部世界的操作。两者不能只用一个通用 JSON 记录。

```text
ToolCall: proposed -> authorized -> running -> succeeded/failed/unknown
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

`Yield` 的 Runtime 语义已实现：主控 Agent 在等待子节点、人工 Gate 或外部 Effect 时原子释放 Node/Attempt/Agent lease 与资源预留，Resume 前校验等待条件，成功后恢复 NodeReady 和 AgentRunnable。`DurableHarness` + 可注入 `SessionStore` 已证明本地跨进程 Resume；真实宿主不支持 `Resume` 时仍需使用新 Attempt + 新 ContextView 恢复并记录降级原因。

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
| I0 权威边界冻结 | ADR、命令清单、错误码、聚合 owner、事件版本、指标基线 | 已完成；Runtime `runtime_* + RuntimeCommandStore` 为 current，V7 执行模型为 dead/forbidden-to-restore |
| I1 事务命令内核 | 聚合级 RuntimeCommandStore、JobEvent + outbox 同事务、Memory/PG 实现、outbox claim/ack/retry | 核心已完成；提交后进程终止注入和 PG 故障矩阵仍是生产门槛 |
| I2 围栏与资源账本 | fence token、reservation、reaper、公平调度排序、租户资源配额 | 核心已完成；20 worker/多租户 PostgreSQL 压测和公平性指标待验收 |
| I3 状态、Effect、恢复 | typed collection/record、ToolCall、unknown/reconciling、Checkpoint watermark、Fork/Replay、Provider inbox/账单、Yield/Resume | 核心已完成；真实 Provider 对账、故障注入和 Artifact 大值链路待验收 |
| I4 执行器与投影 | DurableHarness + SessionStore 镜像、outbox Projector、Runtime Explorer 投影/指标、rebuild/dry-run | 核心已完成；真实 SDK 恢复、PostgreSQL RLS/投影重建和告警运维验收待补 |
| I5 第二业务流与 Canary | 文章复盘 50 节点并行分析、超限保护、故障注入、灰度、回退和运维手册 | 第二流程容量边界测试已完成；100 节点/20 并发、故障矩阵和 Canary 待验收 |

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

### 7.3 旧运行链路治理与退出条件

| 分类 | 当前范围 | 约束与退出条件 |
| --- | --- | --- |
| `current` | `runtime_*` 表、`RuntimeCommandStore`、Projector、DurableHarness、Runtime worker 协议；Customer Studio 与知识提取的 JobRun，以及运行列表/ProjectProjection/lineage 投影 | Runtime 新能力只在这些 owner 内演进；远程 worker 只能凭 Attempt ID + fence token 续租和终态收敛；架构检查禁止 current 代码重新引入 V7 命令、RunToken 或租约 API |
| `compat` | `TaskRun` 与 `RunProgressEvent` JSON 只读 DTO、`run.list/show/events/log` CLI 展示命令 | 只从 JobRun/NodeRun/JobEvent 生成，不拥有存储、状态机或写 API；退出条件是下一版公开 API 统一采用 Runtime 术语 |
| `deprecated` | 全局 `store.Store`/`app.Service` 宽接口 | 不再承载执行方法；后续按业务模块拆窄，方法数只能减少 |
| `dead` | V7 `task_runs/run_attempts/run_progress_events/creative_execution_bundles`、daemon poll/report/finish 链、RunToken、旧 daemon journal/outbox | `00034` 和代码删除已完成；禁止恢复，历史只存在于迁移 evidence |

旧执行链已经完成生产引用归零、Store/API 删除和数据库删除迁移。保留的 `TaskRun` 名称只是公开读 DTO，不允许重新承载租约、attempt、token 或持久化字段。

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
