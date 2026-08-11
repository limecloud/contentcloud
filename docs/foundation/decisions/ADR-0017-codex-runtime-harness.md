# ADR-0017：Codex / Claude Runtime Harness

状态：`Proposed`（代码已按本决策实现，正式 Accepted 仍需架构、安全与商业使用评审）。

日期：2026-08-09。

决策者：平台工程与运行时维护者。

关联：

- [Runtime Infra V2](../../roadmap/v8/09-runtime-infra-v2.md)
- [ADR-0016 Runtime 统一事务命令内核](./ADR-0016-runtime-command-kernel.md)

## 背景

旧 Runtime worker 为每个 Attempt 生成 ContentCloud 自造的 session ID，再调用一次性 Adapter；它既不读取宿主事件流，也不能在 worker 重启后恢复真实会话。后来增加的 `DurableHarness + SessionStore` 只镜像自造会话和事件，没有生产消费者，反而形成了与 RuntimeAttempt/JobEvent 平行的第二套事实。当前 Codex 和 Claude 均通过各自的结构化流 Harness 接入，并以宿主返回的真实 thread/session 标识恢复。

Codex CLI 已提供稳定的 JSONL 执行协议和持久 thread：新执行以 `thread.started` 返回 thread ID，后续可用 `codex exec resume <thread_id>` 在新进程继续。ContentCloud 不需要复制 transcript 或另建宿主会话库。

## 决策

1. Codex Runtime 执行使用官方 CLI JSONL 协议：`codex exec --json` 启动，`codex exec resume <thread_id>` 恢复；禁止 `--ephemeral`。Claude Runtime 使用 `stream-json` 启动，保存首个结构化事件的 `session_id`，通过 `--resume <session_id>` 恢复。
2. 首个事件必须是合法的 `thread.started`。其 thread ID 作为不透明 `AgentSessionRef`，在 Activate 命令中固定到 `RuntimeAttempt.session_ref`；Resume 返回不同 ID 时默认拒绝。
3. Runtime worker 在执行节点探测 Harness 能力，并把 capability snapshot 固定到 Attempt。控制面不根据本机是否安装 CLI 猜测远端 worker 能力。
4. 远程 worker 只通过认证设备身份和 `prepare_next / activate / heartbeat / event / finalize` 窄协议推进 Attempt。Harness 事件必须校验 tenant、Attempt、owner、未过期 lease、fence token 和已绑定 session。
5. Runtime 只保存事件类型、受控状态、错误码和原始数据的规范摘要；不保存 stdout、完整 item、prompt、transcript、绝对路径或宿主凭据。单事件输入上限为 64 KiB。
6. `RuntimeAttempt.session_ref` 是 ContentCloud 唯一的持久宿主会话绑定。Provider 拥有 thread 内容，JobEvent 拥有脱敏执行观察事实；不再维护 `DurableHarness`、`SessionStore` 或 `runtime_agent_sessions/events` 镜像。
7. Codex 默认使用 `workspace-write` 沙箱和无人值守批准模式。隔离工作区、冻结契约、输出 Schema、Runtime 工具授权和网络策略仍由 ExecutionProfile/worker 边界负责。
8. Claude CLI 已通过 capability probe 确认支持 `stream-json`、真实 `session_id`、`--session-id` 和 `--resume`；Runtime 使用独立 Claude Stream Harness，只有探测到这些参数时才声明 `resume=true`，不再伪造或依赖宿主内存。

## 备选方案

### 方案 A：继续使用一次性 Codex Adapter

实现简单，但 session 是自造值，无法消费真实 `turn/item` 事件或恢复 provider thread。不采用。

### 方案 B：复制 transcript 到 ContentCloud SessionStore

可以获得第二份会话镜像，但增加敏感数据、顺序冲突、保留策略和恢复一致性问题；Runtime 已能凭 provider thread ID 恢复，不需要这套权威。不采用。

### 方案 C：直接接入 App Server 或 SDK

能力更完整，但当前 CLI 已提供本阶段需要的 thread、JSONL 和结构化结果协议。等 fork、steer、长驻连接或吞吐指标证明需要时再单独演进，不提前引入第二实现。

## 事实所有权与边界

| 事实 | 权威所有者 | ContentCloud 持久内容 |
| --- | --- | --- |
| Codex thread 与完整 transcript | Codex | 只保存不透明 thread ID |
| Attempt lease、fence、能力快照和终态 | RuntimeAttempt / RuntimeCommandStore | 权威快照与 JobEvent |
| Harness 过程事件 | Provider 产生，Runtime 筛选 | 类型、摘要、错误码，不保存原文 |
| 业务结果 | 对应业务拥有域 | Runtime 只固定内容寻址 output ref 与 digest |

## 兼容与迁移

- `DurableHarness`、`SessionStore`、PostgreSQL 实现和正向测试删除，不保留 compat 包装。
- `runtime_agent_sessions` 和 `runtime_agent_session_events` 从首个用户迁移基线中删除；创建/删除它们的历史迁移文件与正向测试不再存在。已有开发库遇到旧版本记录时必须重建。
- 架构扫描禁止在生产 Go 中恢复旧类型、表名或 `--ephemeral` Codex Runtime 路径。
- 旧 Attempt 没有真实 provider thread 时不能伪造；由 lease 回收并按新 ContextView 创建新 Attempt。

## 安全与运行影响

- 远程事件入口受设备身份、tenant、Attempt、owner、lease、fence 和 session 多重约束；迟到 worker 不能追加过程事件。
- 事件幂等键包含 Attempt、脱敏事件内容和发生时间摘要；PostgreSQL 在 JobRun 行锁内返回已有事件，重报不会产生第二条 JobEvent/outbox。
- ContentCloud 不接收原始 transcript，降低客户正文、工具参数和凭据进入运行日志的风险。
- 在线 Codex 使用的认证方式、费用归属和第三方租户分发仍需商业与安全评审；代码可恢复不等于允许平台托管。

## 验证

1. helper process 模拟官方 JSONL，覆盖 Start、真实 thread ID、结构化事件、结果、协议错误和跨 Harness 实例 Resume；测试不调用模型、不计费。
2. Runtime worker 测试覆盖 capability snapshot、真实 session 激活、持续 heartbeat、fenced event、结构化终态和业务 payload 分离。
3. Memory 与 PostgreSQL/RLS 测试覆盖事件重报幂等和 stale fence 拒绝；真实 PostgreSQL 用例需要 `CONTENTCLOUD_TEST_DATABASE_URL`。
4. 架构守卫禁止 Runtime 回退到一次性 Codex Adapter、`--ephemeral`、伪 session 或未围栏事件入口。
5. 生产准入前仍必须在明确授权、低预算、非客户数据环境完成一次真实 Codex CLI Start/中断/新进程 Resume 冒烟和版本兼容性记录。

## 回退

若 Codex CLI 协议变化或在线验证失败，停止 Codex Runtime 准入并让已领取 Attempt 过期回收；修复适配器后凭原 thread ID 恢复，或显式创建新 Attempt + ContextView。不得恢复 session 镜像或一次性 Codex Runtime 旁路。

## 后果

正面：Runtime 使用真实 provider session，跨进程恢复不依赖 ContentCloud 进程内存；事件、租约和业务结果各自只有一个事实源。

代价：依赖 Codex JSONL 和 Claude stream-json/CLI 参数协议；线上升级需要 capability probe 与真实 CLI smoke，不能把协议事件原文写入 Runtime 事实。
