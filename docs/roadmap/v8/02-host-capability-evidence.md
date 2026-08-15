# 02：Codex / Claude Code 能力证据与适配边界

> 阅读对象：平台架构、执行适配、安全和法务负责人。本文把承载智能体会话和工具调用的 Codex、Claude Code 统称为“宿主”。普通读者只需记住一句话：宿主负责完成具体步骤，ContentCloud 负责任务编排并保存权威业务记录。

范围说明：本文评估 Runtime `AgentHarnessAdapter`，不代表本地 Agent Plugin 或 Workbench UI 的客户端支持状态。Plugin、Workspace 绑定、MCP Apps、Direct Browser 和 Headless 的多宿主矩阵以[本地工作台技术方案](../../product/customer-creation-studio/05-local-workbench-browser.md)为准；两个矩阵不能互相推导。

## 1. 结论

动态执行图（DAG）、同一执行实例（`JobRun`）内的共享状态、检查点和外部操作台账已经有 ContentCloud 内核实现，但尚未完成生产故障、RLS、容量和真实 Provider 验收；这些能力不能被描述成 Codex 或 Claude Code 为 ContentCloud 原生提供。

正确的分工是：

```text
Codex / Claude Code
  = 智能体循环 + 工具 + 隔离上下文 + 会话/线程原语

ContentCloud
  = 可持久化的执行图 + 事务状态 + 调度器 + 策略
    + 检查点 + 外部操作台账 + 权威业务数据
```

两种宿主都支持 MCP，因此 ContentCloud 可以向一次执行尝试（`Attempt`）暴露自建的 `state.cas`、`child.propose`、`effect.prepare` 等受限工具。智能体只能提交子任务提议，不能直接创建节点；调用是否被允许、如何提交事务、如何恢复以及如何跨租户隔离，仍必须由 ContentCloud 服务端实现。

官方能力证据核验截至 2026-08-05，仓库实现对账更新于 2026-08-11。本文件只引用届时可访问的官方公开资料和当前仓库代码；官方文档或 CLI 协议发生变化后，必须重新执行适配器兼容性验证，不能根据本文猜测新增能力。

## 2. 官方来源

### Codex

- [Subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents.md)：并行子智能体、独立线程、后续指令/等待/关闭操作、不同模型和并发上限。
- [Codex App Server](https://learn.chatgpt.com/docs/app-server.md)：`thread/start`、`thread/resume`、`thread/fork`、`turn/start`、事件流和工具项。
- [Codex SDK](https://learn.chatgpt.com/docs/codex-sdk.md)：以程序方式启动、继续和恢复本地 Codex 线程。
- [Model Context Protocol](https://learn.chatgpt.com/docs/extend/mcp.md)：Codex CLI/IDE/App 可连接 stdio 或 Streamable HTTP MCP 服务器。
- [Use Codex with the Agents SDK](https://learn.chatgpt.com/docs/mcp-server.md)：Codex 可作为 MCP 专用智能体接入更大的编排流程。

### Claude Code / Claude Agent SDK

- [Subagents](https://code.claude.com/docs/en/sub-agents)：独立上下文、工具/模型配置和并行子任务。
- [Agent Teams](https://code.claude.com/docs/en/agent-teams)：实验性的负责人/队友、共享任务清单、任务依赖、文件锁和站内信箱。
- [Dynamic Workflows](https://code.claude.com/docs/en/workflows)：JavaScript 脚本使用 `agent()`/`pipeline()` 协调几十到数百个智能体，脚本变量保存中间结果。
- [Agent SDK Sessions](https://code.claude.com/docs/en/agent-sdk/sessions)：对话会话的继续/恢复/分支；明确说明不是文件系统快照。
- [SessionStore](https://code.claude.com/docs/en/agent-sdk/session-storage)：把对话记录镜像到 S3、Redis 或 PostgreSQL 以跨主机恢复。
- [MCP](https://code.claude.com/docs/en/mcp)：stdio、HTTP 等 MCP 接入和权限配置。
- [Agent SDK Overview](https://code.claude.com/docs/en/agent-sdk/overview)：SDK 提供智能体循环、子智能体、MCP、会话、钩子和权限。

## 3. 能力矩阵

`已支持` 表示官方公开接口能直接提供这项能力；`部分支持` 表示只能拿来做其中一部分；`未发现` 表示截至核验日，官方文档没有给出我们需要的业务语义，不等于厂商未来不会提供。

| V8 需要的能力 | Codex | Claude Code | V8 结论 |
| --- | --- | --- | --- |
| 单智能体程序化执行 | 已支持：SDK、App Server、`codex exec` | 已支持：Agent SDK、`claude -p` | 可做智能体执行适配器（`AgentHarnessAdapter`） |
| 多智能体并行拆分 | 已支持：子智能体工作流（`subagent workflow`） | 已支持：子智能体；动态工作流（Dynamic Workflow）可用 `pipeline()` 批量运行 | 可作为节点内部或 ContentCloud 调度的执行器 |
| 智能体间协调 | 部分支持：主线程可创建、干预、等待和汇总子智能体 | 部分支持：实验性 Agent Teams 有站内信箱和共享任务清单 | 不作为跨租户权威数据源 |
| 动态分支/循环 | 未发现面向业务的执行图（DAG）接口；可以由主智能体或外部编排器决策 | 动态工作流脚本可表达循环、分支和并行拆分 | 执行图仍由 ContentCloud 持久化和校验 |
| 执行图依赖关系 | 未发现可持久化的执行图（DAG）约定 | Agent Teams 任务支持依赖；动态工作流由脚本控制 | 只作为宿主内部实现，不替代 `JobPlanRevision` |
| 会话恢复 | App Server/SDK 支持线程恢复 | Agent SDK 支持会话恢复；SessionStore 可跨主机镜像对话记录 | 映射到 `AgentInstance` 的会话引用，不映射为执行实例的检查点 |
| 会话分支 | App Server 支持 `thread/fork` | Agent SDK 支持 `forkSession` | 只复制对话历史；业务分支由新的 `JobRun` 完成 |
| 事件/工具追踪 | App Server 流式 `thread/*`、`turn/*`、`item/*` | SDK 消息流、hooks、会话对话记录 | 可采集为执行观测，不直接成为业务事件 |
| MCP 运行时网关 | 已支持 stdio/HTTP MCP | 已支持 stdio/HTTP 等 MCP | 两者共有的最小可移植工具面 |
| 类型化共享状态 + CAS | 未发现 | 未发现；团队任务清单不是通用类型化业务表 | ContentCloud/PostgreSQL 实现 |
| 跨执行实例的公平调度和配额 | 只有宿主线程/智能体数量限制 | Workflow 有 16 个并发、1,000 个总量等运行限制 | 租户、服务商、Token、预算公平性由 ContentCloud 实现 |
| 执行实例检查点 | Thread history 可恢复/分支，不含业务状态 | 会话恢复；文件检查点只处理文件 | ContentCloud 记录执行图、共享状态和事件水位 |
| 外部操作未知/对账/补偿 | 未发现完整台账语义 | 未发现完整台账语义 | ContentCloud 实现，不承诺“恰好一次” |
| 权限和工具边界 | 沙箱（`Sandbox`）、审批（`approval`）、自定义智能体（`custom agent`）、MCP 配置（`MCP config`） | 权限模式（`Permission mode`）、工具白名单（`tool allowlist`）、钩子（`hooks`）、MCP | 在宿主权限之外再增加限定到执行尝试（`Attempt`）范围的服务端授权 |

## 4. Claude Code 的重要限制

Claude Code 的能力不只是“子智能体回传摘要”，但官方限制决定它不能直接承担 V8 的任务控制职责。

### 4.1 Agent Teams

官方 Agent Teams 文档明确：

- 该功能仍处于实验阶段，默认关闭。
- 任务清单支持 `pending / in progress / completed` 和依赖；领取任务使用文件锁。
- 同进程队友不支持通过 `/resume` 或 `/rewind` 恢复。
- 任务状态可能滞后，可能需要人工修正，否则下游依赖会被阻塞。
- 一个会话只有一个团队；不支持嵌套团队；负责人固定。
- 团队运行配置会在会话结束时清理，任务清单虽保留，但不是远程事务数据库。

因此 Agent Teams 适合本地研究、评审或独立模块并行，不适合承担 ContentCloud 的权威执行状态、跨设备恢复、RLS、费用和外部操作。

### 4.2 Dynamic Workflows

官方的动态工作流（Dynamic Workflows）是可以阅读、保存和重新运行的 JavaScript 编排脚本：

- 脚本本身决定下一步，中间结果存在脚本变量中。
- `agent()` 启动一个智能体，`pipeline()` 对列表批量启动智能体。
- 运行时最多同时运行 16 个智能体、每次运行最多 1,000 个智能体。
- 编排脚本不能直接访问文件系统或 Shell；这些动作由子智能体执行。
- 不支持运行中途的用户输入；需要人工确认时必须拆分工作流。
- 暂停和恢复只在同一个 Claude Code 会话内有效；退出后再次启动会从头运行。
- 被停止时，未完成的智能体以及在它之后启动的智能体可能重新执行。

这证明 Claude Code 确实支持脚本化的大规模动态编排，也证明它不是一个跨会话、跨租户、具备事务共享状态和外部操作治理的持久化作业运行时。V8 可以在低风险节点内选配 Claude Workflow，但不能把整个营销活动的权威调度交给它。

### 4.3 Agent SDK Session

官方文档明确区分：

- 会话保存对话历史，包括提示词、工具调用及结果和回复。
- 会话分支会复制对话历史，但不会复制文件系统。
- 跨主机恢复需要搬移会话文件或使用 SessionStore。
- SessionStore 是对话记录镜像，写入失败会重试后丢弃远端批次，而本地智能体继续执行；它不是强事务业务状态库。

所以 V8 可以保存 `claude_session_id` 作为 `AgentInstance` 的执行器引用，但不能把 SessionStore 当作 `StateCollection` 或外部操作记录台账。

## 5. Codex 的重要边界

### 5.1 子智能体工作流（Subagent workflow）

Codex 官方资料支持：

- 主线程可以启动多个独立智能体线程并行处理任务。
- 主线程可以发送后续指令、等待结果、停止或关闭线程。
- 子智能体可以使用不同的模型、推理强度、沙箱、MCP 和技能配置。
- 官方建议优先并行处理以读取为主的任务；多个智能体同时修改同一文件会产生冲突。

这些能力足以支持受控的主控智能体和执行智能体协作，但资料没有定义跨租户执行图、共享 CAS 数据、外部费用、检查点或公平队列。因此 V8 不依赖“Codex 自己会记住所有子任务”。

### 5.2 App Server / SDK

App Server 提供最适合 V8 适配器的基础接口：

```text
thread/start | thread/resume | thread/fork
turn/start   | turn/steer    | turn/interrupt
thread/*     | turn/*        | item/* event stream
```

`item/*` 中可观察命令、文件变化、MCP 工具调用、协作工具调用和上下文压缩。V8 可以把这些事件转换为 `AgentExecutionEvent`，但只有经运行时网关校验并由服务端提交的内容，才能变成 `JobEvent`、`StateMutation` 或业务输出。

### 5.3 MCP

Codex 支持标准输入输出和 Streamable HTTP MCP，Claude Code 也支持标准输入输出、HTTP 等传输方式。这是 V8 的可移植交集。首版优先为每个 `Attempt` 启动本地标准输入输出 MCP 网关；如果后续需要常驻服务，则由 ContentCloud 适配层通过本机受控通道转发，并使用短期凭据。后半部分是 V8 的设计取舍，不代表宿主原生提供 Unix Socket MCP 传输。

## 6. 当前仓库与目标之间的差距

当前 Runtime 已不再通过一次性 Codex Adapter 执行。实现边界如下：

| 当前行为 | 已解决的问题 | 剩余门槛 |
| --- | --- | --- |
| Codex 使用 `exec --json`，首事件固定真实 thread ID，恢复使用 `exec resume <thread_id>` | 新 worker 进程可恢复 provider thread；不再自造 session 或使用 `--ephemeral` | 明确授权的在线 Start/中断/Resume smoke、CLI 版本兼容和商业使用评审 |
| worker 探测 Harness 能力并固定到 RuntimeAttempt | 控制面不根据本机 CLI 猜测远端能力 | worker 注册/下线和容量指标仍需生产化 |
| Harness 事件经 Attempt/owner/lease/fence/session 校验后只保存类型和摘要 | 迟到 worker 不能写入，transcript 不进入 Runtime | 生产日志泄漏扫描和告警验收 |
| Claude 使用 `stream-json`、真实 `session_id`、`--session-id`/`--resume` 的 Runtime Harness | 事件流、结构化结果、租户绑定和跨 Harness 实例 Resume 已接入 | 真实 Claude 在线 Start/中断/Resume smoke、CLI 版本兼容和权限与数据处理评审 |
| ContextView、Yield/Resume、State/Effect 命令、Attempt 范围 MCP Gateway 和设备命令入口已实现 | 宿主对话不再承担业务恢复，工具调用必须经过 fence、ContextView allowlist 和 ToolCall 审计 | 真实宿主 MCP stdio/HTTP smoke、在线模型中断/恢复 |

旧 `DurableHarness + SessionStore` 镜像没有生产消费者，代码、迁移文件和正向测试均已删除。`RuntimeAttempt.session_ref` 是唯一持久宿主绑定，完整 transcript 继续由 provider 拥有；带旧 Session Mirror 迁移记录的开发库必须重建。

## 7. `AgentHarnessAdapter` 契约

首版契约只表达两种宿主都能提供的交集：

```go
type AgentHarnessAdapter interface {
    Detect(ctx context.Context) (HarnessCapabilities, error)
    Start(ctx context.Context, StartAgentRequest) (AgentSessionRef, EventStream, error)
    Resume(ctx context.Context, ResumeAgentRequest) (EventStream, error)
    Interrupt(ctx context.Context, AgentSessionRef) error
    Inspect(ctx context.Context, AgentSessionRef) (AgentSessionStatus, error)
}
```

`StartAgentRequest` 只包含：

- 冻结的 `TaskContract`、`ExecutionBundle` 和 `ContextView`。
- 运行时网关的一次性连接信息。
- 模型和执行配置、工具白名单、沙箱或隔离配置、截止时间和预算。
- 已发布的输出 Schema。

它不包含数据库凭据、设备令牌、服务商密钥、任意模型地址或整个执行实例的原始状态。

## 8. 不依赖宿主原生子智能体的创建模式

为了保证 Codex、Claude Code 和 FakeHarness 行为一致，V8 的权威子任务创建流程是：

```text
主控智能体
  -> MCP child.propose(spec, input_refs, idempotency_key)
  -> ContentCloud 校验 GraphPatch + 策略 + 预算
  -> ContentCloud 创建 NodeRun / AgentInstance
  -> 调度器独立租约分配子任务给兼容的执行适配器
  -> 子任务输出提交到类型化状态
  -> 主控智能体以新的 ContextView 恢复
```

宿主自身的子智能体（subagent）、智能体团队（Agent Team）或动态工作流（Dynamic Workflow）只允许作为一个节点内部的非权威优化，并且最终结果仍需通过节点输出数据格式（Schema）。首版默认关闭此优化，先验证 ContentCloud 自己的持久调度。

## 9. 认证与商业使用边界

- 方案不能默认假设第三方产品可以复用或转售用户的消费级 Codex/Claude 登录和套餐额度；只有供应商条款和明确授权允许时才能采用这种方式。
- Claude Agent SDK 官方说明第三方产品应使用 API 密钥认证，除非另有批准；实施真实适配器前必须完成条款和数据处理评审。
- Codex SDK/App Server 的分发、认证和面向外部租户使用也需要在 M8-0 冻结；本文只证明技术接口存在，不推断商业授权。
- 本地用户主动安装并登录的 CLI 与平台托管的服务商是两种 `ExecutionProfile`，审计、成本和支持边界必须分开。

## 10. 证据门槛与原型验证

在实现运行时通用能力前，必须完成三个可重复的概念验证（POC）：

| POC | 内容 | 通过条件 |
| --- | --- | --- |
| POC-A Codex | CLI JSONL 启动、事件流、真实 thread ID、新进程恢复；后续补 MCP CAS 和在线中断演练 | helper-process 协议测试已通过；在线模型 smoke 与 MCP CAS 通过后才达到生产准入 |
| POC-B Claude | CLI `stream-json` 启动、真实 `session_id`、中断、`--resume` 和脱敏事件投影 | 跨 Harness 实例恢复；宿主事件不越过 Runtime fence；真实在线 smoke 与 MCP CAS 通过后才达到生产准入 |
| POC-C FakeHarness | 故障脚本覆盖超时、重复事件、未知外部操作和上下文压缩 | 已可确定性重放，CI 不调用付费模型 |

每个适配器都要发布 `HarnessCapabilities`，由调度器按能力匹配，不根据二进制名称猜测：

```text
events | resume | fork | mcp_stdio | mcp_http | structured_output
sandbox_profile | max_parallel_sessions | transcript_export
```

未通过 POC 或运行时探测的能力必须默认拒绝执行；不能降级为“让智能体自己记在文档里”。
