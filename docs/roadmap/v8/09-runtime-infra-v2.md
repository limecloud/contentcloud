# 09：Runtime Infra V2：可恢复执行内核升级

状态：`I0 已冻结；I1～I5 核心切片、Attempt-scoped MCP Gateway、Runtime Schema Registry、Provider HTTP ingress、Runtime Effect 关联、流式媒体对象链和 Claude stream-json/session resume 已进入代码；专用 PostgreSQL 集成库已通过迁移、核心 RLS、事务回滚、outbox receipts、fenced replay、Provider ingress httptest 和 Harness helper-process 验证；真实 Provider 凭据/回调、真实提交后故障环境、生产告警和 Canary 未完成`。

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
| 领域内核 | JobPlan、JobRun、NodeRun、JobEvent、State/StateCollection/StateRecord、Checkpoint、Effect、ToolCall 的状态转移；主要写路径已切到 `RuntimeCommandStore` | 真实 PostgreSQL 进程/网络提交后故障环境、完整命令契约矩阵 | `internal/domain/runtime.go`、`internal/runtime/commands.go`、`internal/runtime/service.go` |
| 持久化 | PostgreSQL 迁移 `00014`～`00043`、RLS、复合外键、JobRun 准入冻结字段、追加事实权限、Memory/PostgreSQL Store、不可变 outbox + subscriber receipts、资源账本、typed state、ToolCall（含安全结果重放）、Runtime Explorer 快照、关系化计划 revision、Fanout/Join、Provider inbox/账单、Yield、投影重建事实、异步 Provider 到期轮询和 Runtime Schema Registry；`00036` 已删除零消费者 session 镜像表，`00037` 已增加维护心跳，`00038` 收敛 Provider poll recovery，`00039` 阻止缺失 deadline 的 unknown 提交进入重试循环，`00040` 为媒体 Job/Attempt 增加显式 Runtime Effect 关联且历史行保持空关联，`00041` 增加 Schema draft/published/retired 与保留策略，`00042` 增加幂等/Explorer 索引，`00043` 增加 ToolCall safe_result；专用 PostgreSQL 集成库已通过迁移、核心 RLS（含投影重建和维护心跳越权负向）、事务回滚和 receipt 隔离 | 真实数据库提交后故障和迁移历史重建演练 | `migrations/00014_agentic_job_runtime.sql`～`00043_runtime_tool_call_results.sql`、`internal/store/postgres/*_integration_test.go` |
| 调度 | FakeHarness 的 Prepare/Start/Activate/Heartbeat/Finalize、owner/version/fence 围栏、资源预留与释放/消费/过期、优先级 aging 排序、租约回收；PostgreSQL 100 节点/20 worker 并发领取；Runtime FairnessReport 输出按租户资源利用率、过期 held 和 Jain 指数 | 生产公平性长时压测和提交后故障注入 | `internal/runtime/dispatch.go`、`internal/runtime/fairness.go`、`internal/store/postgres/runtime_dispatch.go`、`runtime_resources.go`、`runtime_capacity_integration_test.go` |
| 宿主执行 | 结构化事件接口、FakeHarness、worker 侧能力探测与 Attempt capability snapshot、Codex CLI JSONL/thread ID、`exec resume`、Claude CLI stream-json/session ID/`--resume`、持续 heartbeat、fenced 脱敏事件和结构化终态 | 真实 Codex/Claude 在线模型 Start/中断/新进程 Resume、真实宿主故障演练 | `internal/agentadapter/harness.go`、`codex_harness.go`、`claude_harness.go`、`internal/cli/runtime_worker.go`、`internal/runtime/dispatch.go` |
| 状态与上下文 | StateCollection（四种一致性策略）、StateRecord CAS、引用型 ContextView、父子预算/工具子集校验、Attempt-scoped MCP Gateway（state/child/effect 工具授权）和 Runtime Schema Registry（draft/published/retired） | 真实宿主 MCP stdio/HTTP smoke、Artifact 大值链路和 Schema JSON Schema 编译器 | `internal/runtime/mcp_gateway.go`、`internal/runtime/schema.go`、`internal/domain/runtime.go`、`internal/store/*/runtime_state_tools.go` |
| 外部操作 | Effect 状态机（unknown/reconciling 禁止盲重试）、媒体 Job/Attempt 显式 Effect 关联、ToolCall 状态机、Effect 的 Attempt/Reservation 绑定、Provider inbox 去重、Provider reconciliation、账单匹配/差异/无匹配记录、签名/时间窗/租户绑定 ingress、资源账本 | 真实服务商端到端回调/账单和补偿演练 | `runtime_effects`、`runtime_provider_*`、`internal/runtime/provider.go`、`internal/httpapi/provider_ingress.go` |
| 运营读取 | Durable outbox Projector、Runtime Explorer 持久化投影、投影延迟/积压指标、Job 与 nodes/effects/checkpoints 分页、事件单次上限、REST/SSE 读取、Replay 投影重建和 dry-run、Checkpoint Fork、Effect/Provider Attempt/账单/对账读模型、关系化计划边和脱敏 StateRecord 摘要 | 完整动态图操作、生产告警和支持案例 | `internal/runtime/projector.go`、`internal/store/*/runtime_projection*.go`、`internal/app/runtime_explorer.go` |

本轮 `GOMAXPROCS=2 go test -mod=readonly -p 1 -count=1 ./...`、专用 PostgreSQL `go test -mod=readonly -race -p 1 -count=1 ./internal/store/postgres`、HTTP/localworkspace/Runtime/Memory/PostgreSQL 定向 Go race、Web typecheck/22 个文件 111 个测试、`npm run check:plugin`、`git diff --check` 和 `node scripts/check-architecture.mjs` 均已通过。以上仍不是提交后崩溃、生产容量公平性、真实 Provider、在线宿主或 Canary 验收。

I1～I4 的核心切片已落地，I5 已补上文章复盘 50 节点并行分析及第二批超限保护测试：事务命令、不可变 outbox 与独立 subscriber `claim/ack/retry`、终态业务结果持久化消费、fence 与资源账本、typed state/ToolCall/Checkpoint/Fork/Replay、Provider inbox/账单、Yield/Resume、Codex JSONL/thread resume Harness、Claude stream-json/session resume Harness、Projector、关系化 GraphPatch、FanoutSet/Join 均有 Memory/PostgreSQL 实现或确定性协议测试。专用 PostgreSQL 集成库已验证迁移、核心 RLS（含投影重建和维护心跳越权负向）、事务失败回滚、fenced event replay、独立 subscriber receipt 和 100 节点/20 worker 并发领取；业务结果已覆盖“业务写成功但 ack 失败”、不同摘要拒绝、独立投影 ack、重复消费和新进程恢复；Codex/Claude helper-process 测试已覆盖真实会话标识和跨 Harness 实例 Resume，但没有调用在线模型。剩余退出条件集中在真实 Provider、在线 Codex/Claude smoke、提交后故障注入、多租户公平性、生产告警和 Canary。

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
  + immutable outbox message(event_id, payload)
  + subscriber receipt(message_id, subscriber, lease/retry/ack)
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
- 终态清除原始 lease/fence，仅在追加 JobEvent 保存 worker actor 与 fence SHA-256 摘要；相同终态重试只验证 actor、fence 摘要和结果摘要并返回同一终态。
- 业务结果在终态前完成严格契约校验并写入内容寻址 Blob；终态事件发布独立业务 subscriber receipt，业务对象由后台消费者幂等派生，不能在 Finalize 请求内同步写入。
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
| `runtime_outbox` | `event_id/topic/payload/created_at` | 与 JobEvent 同事务发布的不可变消息，不保存任何单一消费者状态 |
| `runtime_outbox_receipts` | `message_id/subscriber/attempts/next_attempt_at/locked_by/locked_until/delivered_at` | Projector、业务结果等订阅者各自独立的租约、退避与确认 |
| `runtime_provider_inbox` | `provider_id/message_id/received_digest/external_id/state` | Provider 回调去重、结果摘要和安全载荷镜像 |
| `runtime_provider_reconciliations` | `effect_id/request_key/observed_state/expected_minor/observed_minor/status` | Provider 结果与预期请求/费用对账 |
| `runtime_provider_bills` | `provider_id/bill_id/external_id/bill_digest/amount_minor/status` | 账单匹配、差异和无匹配账单事实 |
| `runtime_yields` | `attempt_id/reason/wait_refs/state/resume_key` | 释放执行资源后的等待与恢复边界 |
| `runtime_projection_rebuild_runs` | `job_run_id/mode/status/event_count/last_sequence/external_calls/integrity_status` | 投影 rebuild/dry-run 的运行事实和零外部调用证明 |
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
Finalize(tx) -> result refs + event/subscriber receipts + release resources
```

`Yield` 的 Runtime 语义已实现：主控 Agent 在等待子节点、人工 Gate 或外部 Effect 时原子释放 Node/Attempt/Agent lease 与资源预留，Resume 前校验等待条件，成功后恢复 NodeReady 和 AgentRunnable。Codex Harness 直接保存 `thread.started` 返回的真实 thread ID，新 worker 进程通过 `codex exec resume <thread_id>` 恢复；Claude Harness 保存首个结构化事件中的真实 `session_id`，新 worker 进程通过 `--resume <session_id>` 恢复；两者都不复制 transcript，不依赖进程级 Registry 或 ContentCloud session 镜像。宿主声明 `resume=false` 或原会话不可恢复时，只能使用新 Attempt + 新 ContextView 恢复并记录降级原因。

### 6.3 Scheduler、Reaper、Reconciler 分工

- **Scheduler**：只负责领取、预留和分派，不解析宿主私有事件。
- **Reaper**：扫描到期 lease/reservation，使用 fence token 原子收敛 Attempt/Node/Agent/Resource。
- **Reconciler**：处理 `unknown` Effect、Provider callback、账单差异和本地 outbox 失败。
- **Projector**：从 JobEvent/outbox 重建客户与运营读模型；停止时不影响权威状态。
- **Business result consumer**：从成功 Attempt 的 `runtime-result:` 引用读取并核对 Blob，通过业务拥有域幂等写入对象，完成后只 ack 自己的 receipt。

四类 worker 都必须幂等、可水平扩展，并把失败重试写回 `next_attempt_at`；不能依赖内存定时器或单进程 Registry 才能恢复。

## 7. 实施顺序（V8.1 overlay）

旧 W8 工作包继续作为能力清单；以下 overlay 是进入 W8-07～W8-10 之前的基础顺序。

| 阶段 | 交付 | 退出条件 |
| --- | --- | --- |
| I0 权威边界冻结 | ADR、命令清单、错误码、聚合 owner、事件版本、指标基线 | 已完成；Runtime `runtime_* + RuntimeCommandStore` 为 current，V7 执行模型为 dead/forbidden-to-restore |
| I1 事务命令内核 | 聚合级 RuntimeCommandStore、JobEvent + outbox 同事务、Memory/PG 实现、subscriber claim/ack/retry | 核心已完成；专用 PG 已覆盖事务回滚、不可变消息、多订阅 receipt 和业务结果崩溃恢复，代码级提交后故障钩子已具备，真实数据库故障矩阵仍是生产门槛 |
| I2 围栏与资源账本 | fence token、reservation、reaper、公平调度排序、租户资源配额 | 核心已完成；Memory/PostgreSQL 已覆盖 100 节点/20 worker 唯一领取，PostgreSQL 多租户 aging 公平性指标和生产压测待验收 |
| I3 状态、Effect、恢复 | typed collection/record、ToolCall、unknown/reconciling、Checkpoint watermark、Fork/Replay、Provider inbox/账单、Yield/Resume | 核心已完成；真实 Provider 对账、故障注入和 Artifact 大值链路待验收 |
| I4 执行器与投影 | Codex JSONL/thread resume Harness、Claude stream-json/session resume Harness、worker capability snapshot、fenced event、outbox Projector、Runtime Explorer 投影/指标、rebuild/dry-run | 核心已完成；代码级提交后故障钩子和恢复测试已具备，真实 Codex/Claude 在线 smoke、数据库故障环境和告警运维验收待补 |
| I5 第二业务流与 Canary | 文章复盘 50 节点并行分析、超限保护、故障注入、灰度、回退和运维手册 | 第二流程容量边界、PostgreSQL 100 节点/20 worker 领取测试和运维手册已完成；多租户公平性、故障矩阵和真实 Canary 待验收 |

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
      -> validate/blob -> finalize(result ref) -> event/outbox receipts
      -> business materialization + projection -> restart/recover
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
| `current` | JobRun/NodeRun/RuntimeAttempt 及其 `runtime_*` 权威表、`RuntimeCommandStore`、不可变 outbox/subscriber receipts、Projector、业务结果 consumer、Codex JSONL/thread resume Harness、Claude stream-json/session resume Harness、Runtime worker 协议；Customer Studio 与知识提取的 JobRun；`RuntimeRun` / `RuntimeRunEvent`、`run.list/show/events/log` 以及运行列表/ProjectProjection/lineage 投影 | Runtime 新能力只在这些 owner 内演进；远程 worker 只能凭 Attempt ID + fence token 续租、上报事件和终态收敛；公开读模型只从 JobRun/NodeRun/JobEvent 生成，不拥有独立存储或状态机 |
| `compat` | StageRun 客户业务阶段投影；本地配置旧 `device_id/workspace_id/project_id/workspace_root` 只在 `localconfig.Load()` 的一次性迁移边界解码 | StageRun 只表达 SOP 业务阶段，不参与 Runtime 调度；旧配置字段读入后立即重写为 `daemon_bindings`，CLI 运行期只消费 current 绑定；退出条件是支持的已安装 CLI 完成一次启动迁移并确认无旧配置回流，随后删除 `configFile` 旧字段解码 |
| `deprecated` | 全局 `store.Store`/`app.Service` 宽接口 | 不再承载执行方法；后续按业务模块拆窄，方法数只能减少 |
| `dead` | V7 `task_runs/run_attempts/run_progress_events/creative_execution_bundles`、`TaskRun` / `RunProgressEvent` 公开 DTO 名称、`task_run` lineage 类型、daemon poll/report/finish 链、RunToken、旧 daemon journal/outbox；`DurableHarness`、`SessionStore`、`runtime_agent_sessions/events` 镜像；Runtime 内一次性 Codex/Claude Adapter、伪 session | `00034/00036`、代码与 Web 类型删除已完成；架构守卫禁止恢复，历史只存在于迁移 evidence |

旧执行链和旧公开 DTO 已完成生产引用归零、Store/API/类型删除和数据库删除迁移。`RuntimeRun` / `RuntimeRunEvent` 是 current 读模型，不允许重新承载租约、attempt、token、终态权威或独立持久化字段。

## 8. 生产门槛与故障矩阵

进入真实租户前必须通过：

| 类别 | 必测故障 | 预期 |
| --- | --- | --- |
| 事务 | snapshot 已提交、event/outbox 未提交时进程终止 | 整体回滚或通过 outbox 重试恢复，不能部分成功 |
| 业务交接 | Runtime 终态提交后、业务 consumer 领取前重启 | subscriber receipt 保持 pending；新进程从 output ref 读取并核对 Blob 后继续 |
| 业务交接 | 业务对象写成功、receipt ack 前终止 | 同一 receipt 重新领取；摘要围栏和确定性对象 ID 保证幂等，不产生重复对象 |
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
6. [ADR-0017](../../foundation/decisions/ADR-0017-codex-runtime-harness.md)：Codex CLI JSONL/thread resume、capability snapshot、fenced event、数据保留和商业边界。

## 10. 完成定义

V8.1 基础设施完成，不等于全部 V8 产品能力完成。只有同时满足以下条件，才可以打开动态执行图或对外宣称“可恢复运行时”：

- 所有权威写入都由事务命令完成，状态、事件和 outbox 可一起重试。
- 每个 outbox subscriber 独立确认；业务结果不同摘要被拒绝，进程重启和重复消费不丢失或重复写入业务事实。
- 旧 owner、旧版本和旧 fence token 的写入全部拒绝。
- 资源预留、释放、租约回收和费用核算没有超卖或重复记账。
- 至少一个执行适配器可跨进程恢复，或明确记录新 Attempt 降级恢复。
- `unknown` 外部操作必须先对账；重放、投影重建和分支不会产生外部调用。
- Memory/PostgreSQL 契约测试、故障注入、RLS、容量、指标、Canary 和回退演练通过。
- 第二种结构不同的业务流只增加业务包、Schema 和 Gate，不增加 Runtime 专用状态机。
