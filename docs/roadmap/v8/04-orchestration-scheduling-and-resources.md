# 04：任务编排、调度与资源管理

> 阅读对象：调度、执行器和资源治理模块研发人员。本文件中的“执行步骤、租约、配额”都是后台实现，用户界面只展示进度、等待原因和可执行操作。本文沿用：`WorkTask`=业务任务、`JobRun`=执行实例、`NodeRun`=执行步骤、`RuntimeAttempt`=执行尝试；执行图（DAG）中的 `human_gate` 是人工审批节点。

## 1. 控制面与执行面

```text
                         可恢复的任务控制面
┌─────────────────────────────────────────────────────────────────────┐
│ 准入校验 -> 执行图编译 -> 就绪判断 -> 公平调度                       │
│                 |                   |                  |              │
│                 v                   v                  v              │
│       执行实例/步骤状态          资源台账            租约/心跳         │
│                 |                   |                  |              │
│                 +-------- PostgreSQL 事务数据 ----------------------+
└───────────────────────────────┬─────────────────────────────────────┘
                                │ 领取 / 上报 / 运行时网关
         ┌──────────────────────┼────────────────────────┐
         v                      v                        v
  本地智能体守护进程       固定程序工作进程          外部服务工作进程
  Codex / Claude        解析/渲染/校验             提交/轮询/下载
         \______________________|________________________/
                                |
                            执行面
```

控制面负责判断哪些任务可以运行、何时运行、可使用多少资源，以及执行结果能否提交。执行面只执行已经签发的任务契约（`TaskContract`）和执行包（`ExecutionBundle`），不能自行决定跨租户优先级、预算或业务审批。

## 2. 执行实例准入校验

`JobRun` 从草稿态（`draft`）进入排队态（`queued`）前，必须完成以下校验：

- `WorkTask`、SOP、输入快照、`ExecutionBundle` 和 `RuntimePolicy` 的版本可读，内容摘要一致。
- 所有初始执行步骤使用的数据格式（Schema）、能力、执行器配置和人工审批节点条件均已发布。
- 租户允许使用相应的智能体执行适配层、外部服务商、运行区域和数据披露范围。
- 最大节点数、并发数、模型输入长度（Token）、费用、运行时间和存储用量均有明确上限。
- 初始预算可以预留；费用无法预估的外部服务节点必须设置费用审批。
- 执行计划是有向无环图，所有必需输入都能由初始输入或上游输出提供。

准入失败时返回稳定错误码，`JobRun` 保持在 `draft`，不得创建可被工作进程领取的半成品节点。

## 3. 节点就绪判断

节点只有同时满足以下条件，才能从等待态（`pending`）进入就绪态（`ready`）：

1. 所有必需的前置节点都已达到允许的终态。
2. 汇聚节点对应的并行成员集合（`FanoutSet`）已经封闭，且完成条件可以判定。
3. 阻断性审批已通过，并且批准所针对的对象版本仍然有效。
4. 输入选择器找到的对象真实存在，属于当前租户和项目，且内容摘要一致。
5. 重试策略（`RetryPolicy`）指定的 `retry_at` 时间已经到达。
6. 上级执行实例没有暂停、取消或失败。

就绪状态必须由服务端的确定性代码计算。智能体可以提出下一步建议，但不能直接把节点标记为 `ready`。

## 4. PostgreSQL 调度与 Harness 三阶段协议

V8 继续使用现有的 PostgreSQL 租约机制，首版不引入新的消息系统：

```text
HarnessRegistry.Resolve + 能力快照
  |
  v
PrepareDispatch（数据库事务）
  Node ready -> leased
  + RuntimeAttempt(prepared)
  + ContextView
  + 创建或复用 AgentInstance(runnable)
  + JobEvent
  |
  v 事务提交后
Harness.Start（事务外）
  |
  +-- 启动失败 --> FinalizeDispatch(retryable_failed/failed)
  v
ActivateDispatch（数据库事务）
  Node running + Attempt running + Agent active + session_ref
  |
  v
结构化事件消费 + Node/Attempt 联合心跳
  |
  v
FinalizeDispatch（数据库事务）
  Node + Attempt + Agent + JobEvent 原子终态
  |
  v
刷新 JobRun 和下游 ready 节点
```

候选 Node 可以用 `FOR UPDATE SKIP LOCKED` 直接选择，也可以先读取后在 PrepareDispatch 中通过版本和 `ready` 状态做 CAS；无论采用哪种方式，最终领取、Attempt、ContextView、Agent 绑定和事件必须在同一事务提交。当前实现采用后者，并在多个 worker 竞争同一节点时自动重新选择。

外部 Harness 启动绝不能放进数据库事务。不得先启动进程再写 Attempt，也不得先扣预算后留下没有 Attempt 的孤立预留。资源台账落地后，其预留必须并入 PrepareDispatch 事务。

### 4.1 崩溃窗口与恢复

| 崩溃位置 | 持久化状态 | 恢复动作 |
| --- | --- | --- |
| PrepareDispatch 提交前 | 无半成品 | 事务回滚，Node 仍可领取 |
| PrepareDispatch 提交后、Harness.Start 前 | Attempt `prepared`、Node `leased` | 租约到期后 Attempt `expired`，Node 重新就绪，Agent 保持/回到 `runnable` |
| Harness.Start 后、ActivateDispatch 前 | 数据库仍为 `prepared` | 激活失败时尽力中断会话并原子失败；进程崩溃则由租约回收 |
| Attempt `running` 期间 | Node/Attempt 共用 owner 和租约 | worker 定期联合续租；旧版本或旧 owner 无法复活租约 |
| 收到终态后、响应前 | 终态和事件已经原子提交 | 相同结果摘要幂等成功，不同摘要明确冲突 |
| 事件流关闭但没有终态 | Attempt 可诊断 | 记录 `HARNESS_STREAM_CLOSED` 并按重试上限收敛 |

### 4.2 公平性

当前 `TaskRun` 主要按 `priority DESC, created_at` 领取。V8 增加以下最小公平约束：

- 每个租户和每个业务任务都设置运行中执行步骤数上限。
- 同一优先级内，等待越久的执行步骤越优先，避免普通任务一直得不到执行机会。
- 单个营销活动的并行执行步骤不能占满全部外部服务或智能体执行名额。
- 高优先级任务可以更快获得资源，但仍要遵守硬性配额和已批准预算。
- 系统保留少量恢复专用名额，避免新任务占满资源后无法处理状态不明的任务。

每个 `RuntimePolicy` 必须定义 `fair_wait_window`。它只约束已经就绪、拥有至少一个兼容执行器且除调度分配外所有硬性条件都满足的执行步骤：从 `eligible_ready_at` 起累计等待，到取得租约时停止；暂停、`retry_backoff`、人工审批节点、结果不明的外部操作、输入超限和没有兼容执行器的时间不计入。

当累计等待达到窗口时，执行步骤进入防饥饿队列。除预先声明的事故恢复专用名额外，只要所需资源释放，它必须先于共享同一资源键、但更晚进入就绪状态的执行步骤取得租约，不再按业务优先级继续延后；这仍不能突破租户、业务任务、服务商并发和预算硬上限。若没有兼容执行器或资源容量从未释放，系统显示 `waiting(resource)`，不承诺虚假的等待上限。

首版不实现复杂的加权公平队列。需要通过“一个大租户持续提交、多个小租户间歇提交”的长时间稳定性测试，在已配置的 `fair_wait_window`、兼容执行器持续在线且资源会释放的条件下，证明每个合格的普通执行步骤都在窗口内获得租约。

## 5. 资源预留（`ResourceReservation`）

资源不是一列 `concurrency`，而是可组合的版本化请求：

| 资源类型 | 示例 key | 何时持有 |
| --- | --- | --- |
| 智能体执行名额 | `harness:codex:local-device-1` | 智能体本次执行期间 |
| 固定程序工作进程 | `worker:render:region-cn` | 计算执行期间 |
| 外部服务并发 | `provider:video:profile-v3` | 外部任务运行期间 |
| 模型输入长度预算（Token） | `model-profile:writer-v2` | 启动前预留，使用后结算 |
| 费用预算 | `tenant/project/job/currency` | 外部操作授权后，直至结算或释放 |
| 浏览器会话 | `browser-profile:approved` | 受时限控制的会话期间 |
| 存储与媒体处理 | `blob-ingress`、`transcode` | 上传或转码期间 |

资源预留记录（`ResourceReservation`）至少包含：

```text
resource_key / quantity / unit
tenant_id / job_run_id / node_run_id / attempt_id
state: requested | held | consumed | released | expired
expires_at / idempotency_key
estimated_cost / actual_usage
```

不同资源的占用周期并不相同。外部任务提交完成后，应释放本地智能体执行名额，但继续保留外部服务并发和费用预留，直到外部任务结束或转入人工对账。

## 6. 执行器匹配

### 6.1 执行配置（`ExecutionProfile`）

节点只能引用已经发布的执行配置（`ExecutionProfile`）：

下面列出的是配置键，不能由智能体在运行时自由修改。它们分别表示工具白名单、隔离方式、网络出口白名单、区域和数据分类，以及 Token、时间和费用上限：

```text
profile_id + version + digest
executor_kind
harness/provider/model refs
required capability digests
tool allowlist / MCP 服务白名单
sandbox or isolation profile
egress allowlist / region / data classification
token / time / cost ceilings
fallback policy
```

智能体不能自行指定任意模型名、外部服务地址、MCP 服务或密钥引用。确需动态选择时，只能从 `RuntimePolicy` 明确列出的执行配置中选择，并记录选择依据。

### 6.2 执行能力协商

守护进程或工作进程注册自己实际具备的能力：

```text
adapter kind + version
resume / fork / event_stream / structured_output
mcp_stdio / mcp_http
sandbox profiles
available model profiles
max sessions / current load
```

调度器只能选择满足全部硬性要求的执行器。能力不匹配时，节点进入资源等待状态（`waiting(resource)`）；不得擅自取消 MCP、会话恢复或隔离要求后继续运行。

Harness 实例由进程级 `HarnessRegistry` 显式注入并长期复用。`SelectHarness` 只保留给旧的一次性调用路径；Runtime 禁止每次领取时重新构造适配器，因为 Fake/CLI/SDK 会话表属于适配器实例，重建会使已保存的 `session_ref` 无法恢复。未知 Harness 或缺少结构化事件/结果能力时默认拒绝执行。

## 7. 主控智能体让出资源与恢复执行

主控智能体创建子节点后，不能一直占用进程等待：

```text
主控智能体本次执行
  -> 提交执行图变更申请（GraphPatch）
  -> 运行时校验并创建子节点
  -> 写入检查点和子节点等待条件
  -> 结束本次执行，释放智能体执行名额

子节点全部达到规定状态
  -> 服务端确认等待条件满足
  -> 生成包含子节点摘要的新 ContextView
  -> 为同一 AgentInstance 创建新的 Attempt 并恢复执行
```

宿主会话可以恢复时，适配器使用不透明的会话引用；无法恢复时，则根据已经提交的上下文摘要创建新会话。无论采用哪种方式，执行实例的状态都不能依赖原进程持续存活。

## 8. 重试、失败范围与背压

### 8.1 重试策略（`RetryPolicy`）

每个 `NodeSpec` 都要固定以下规则：

- `max_attempts`、指数退避和随机抖动上限。
- 允许自动重试的错误码白名单。
- 是否允许切换到明确配置的备用 `ExecutionProfile`。
- 重试前是否必须先核对外部操作状态。
- 是否允许复用部分输出。

只有被服务端判定为可重试的失败才能自动重试。提示词中出现“再试一次”不能覆盖系统策略。

### 8.2 失败域

| 失败处理范围 | 行为 |
| --- | --- |
| `node` | 只让当前节点失败，后续节点按边的失败策略处理 |
| `fanout_item` | 隔离单个并行成员，汇聚节点按完成策略判断 |
| `branch` | 取消该分支未开始节点，其他分支继续 |
| `job` | `JobRun` 进入取消中或失败状态，并向下传播取消请求 |
| `manual` | `JobRun` 进入等待状态，由操作员决定重试、跳过或创建执行分支 |

### 8.3 背压

- 外部服务限流时进入资源等待，不让智能体循环轮询。
- Token 或费用不足时进入明确的审批流程，不自动切换到廉价模型降低质量。
- 状态或事件写入延迟超过阈值时，停止发放新租约。
- 守护进程的本地日志积压超过上限时，只允许补报和恢复，不再领取新任务。
- `ContextView` 超出预算时使用确定性摘要或分页，不能截断关键审批和素材权利信息。

## 9. 取消传播

```text
收到执行实例取消请求
  -> 停止创建和领取新节点
  -> pending/ready -> canceled
  -> 撤销有效租约并中断智能体
  -> 在策略允许时取消外部任务
  -> 等待运行中的外部操作变为状态明确
  -> 清理过期的 Attempt 和资源预留
  -> 所有关联外部操作已终态：JobRun 进入 canceled
  -> 存在 unknown/对账中的外部操作：JobRun 进入 waiting(external_effect)
     仅允许对账、登记人工结论或继续取消；对账完成后回到 canceling
```

取消采用协作式处理，不能保证立即停止。已经生效的外部操作不会因执行实例取消而自动撤销，需要另行执行补偿操作。

## 10. 外部服务执行流程

V7 已有 `MediaGenerationJob` 和 `ProviderAttempt`，但当前实现仍有必须先补的生产缺口：

- 适配器只返回 `FakeProvider`，尚未接入真实付费服务商。
- 提交、查询状态和下载都在一次工作进程调用中完成，尚未形成可跨进程恢复的轮询或回调状态机。
- 处于 `submitting/generating/downloading` 状态时崩溃，租约回收流程尚不完整。
- 下载正文使用 `[]byte`，尚未证明大媒体有界流式处理。
- 最终渲染仍是直接透传，没有完成字幕、品牌、行动号召和商品信息的确定性渲染。

这些缺口必须作为 V8 的独立交付项，不能因为引入运行时概念就视为已经解决。目标流程如下：

```text
授权外部操作
  -> 使用稳定幂等键提交任务
  -> 保存 external_job_id，无法确认时标记为 unknown
  -> 释放工作进程租约
  -> 接收回调或由定时节点轮询
  -> 校验状态与费用
  -> 以流式方式下载到对象存储
  -> 校验媒体并生成 Artifact
  -> 将外部操作标记为 committed
```

真实服务商上线需要单独验收；持续集成仍只使用 `FakeProvider` 和固定协议测试数据。

## 11. 模型路由和成本

V8 允许同一执行实例的不同步骤使用不同的已批准模型配置，但主控智能体不能任意选择供应商：

- `JobPlan` 可以指定固定配置。
- `RuntimePolicy` 可以给出有序候选集，并限定允许启用备用配置的错误类型。
- 高价值写作或决策，与批量分类或摘要，可以使用不同的已批准配置。
- 用量必须按执行实例、执行步骤、执行尝试、`AgentInstance`、模型和租户归集。
- 预算接近耗尽时产生告警事件；达到硬上限后停止新的智能体执行，并进入费用审批。
- 费用展示要区分估算、预留、服务商上报和对账完成四种口径。

## 12. 调度验收

- 同一个执行步骤同时最多只能有一个有效租约，过期租约的令牌不能提交结果。
- 20 个并发工作进程领取 100 个测试执行步骤时，不产生重复 `Attempt` 或资源超卖。
- 大租户持续占用时，在兼容执行器持续在线且资源会释放的条件下，小租户每个合格执行步骤的 `lease_at - eligible_ready_at` 不超过其 `fair_wait_window`；暂停、人工审批、重试退避、输入超限和无兼容执行器的时间必须单独记录且不计入。
- 上级执行实例取消后，新的子任务申请、租约和费用授权全部被拒绝；存在结果不明外部操作时，执行实例必须保持 `waiting(external_effect)`，不得直接标记为 `canceled`。
- 主控智能体等待子节点时，不持续占用执行名额。
- 外部任务提交时网络断开，操作进入 `unknown`；调度器不会自动创建第二个外部任务。
- 执行器能力或版本不匹配时默认拒绝执行，并显示缺失能力，不得改用未经批准的配置。
