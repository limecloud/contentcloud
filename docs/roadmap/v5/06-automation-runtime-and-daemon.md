# Automation Runtime 与常驻 Daemon

状态：`无人值守恢复与本地集成闭环已实现，进入真实设备验收`。

更新时间：2026-07-31。

本方案区分交互式 Codex 与 Automation Codex。两者都运行在用户电脑上，但授权时点、交互方式和责任不同。

## 1. 最终决策

1. 用户在 Bootstrap Plan、浏览器设备授权、Workspace 注册和任务入队之前确认业务范围。
2. 服务端只向已注册设备签发带租约的 TaskContract、ExecutionBundle 和 RunToken。
3. 租约签发后，Automation Codex 使用 `dangerFullAccess` 和 `approvalPolicy=never`，执行中不再等待人工审批。
4. Claude Code Adapter 使用 `bypassPermissions`，不禁用 Bash、工具、Plugin、Skill 或 MCP。
5. Agent 继承宿主 Provider、代理、工具链和登录环境，但移除全部 `CONTENTCLOUD_*` 控制面变量，再只注入 `CONTENTCLOUD_AGENT_RUN=1`。
6. 安全边界由执行前后的控制面承担：签名 Bundle、冻结资源、Attempt 私有目录、租约、心跳、取消、结果 Schema 和服务端复验。
7. 这不是操作系统沙箱。Automation Agent 获得的是完成真实媒体和文件任务所需的本机权限，错误 TaskContract 的风险必须在入队前解决。

## 2. 两类 Codex 的执行边界

| 场景 | 授权方式 | 本机权限 | 服务端权限 | 是否等待确认 |
| --- | --- | --- | --- | --- |
| 交互式创作会话 | 用户当前对话与具体命令 | 按 Codex 会话配置 | 只能通过 CLI publish/pull 契约 | 高风险业务动作继续确认 |
| Automation Codex | 已确认 Automation Plan + 服务端租约 | `dangerFullAccess`，允许 Shell/网络/工具 | 仅持有当前 RunToken，不能批准或发布 | 执行中不确认 |
| ContentCloud 服务端 | 用户/设备/Workspace 凭据 | 不读取本机文件 | 租约、审核、批准、归因和审计 | 按服务端工作流 |
| Seedance/抖音 | 用户外部账户 | 外部平台自身权限 | ContentCloud 不代登录 | 上传、生成、发布由用户执行 |

`approvalPolicy=never` 只适用于服务端已经签发的 Automation Attempt，不等于交互式 Codex 可以绕过 `publish --accept`、审核、Seedance 上传或抖音发布确认。

## 3. Daemon 生命周期

Bootstrap 的 `--accept` 明确包含安装和启动用户级 Daemon。macOS 使用 LaunchAgent：

```text
bootstrap apply/resume
  -> workspace.register
  -> daemon start
  -> 写入 LaunchAgent + daemon.json
  -> launchctl bootstrap + kickstart
  -> daemon run
  -> daemon.poll -> lease -> heartbeat -> Agent -> report
```

CLI 提供以下幂等命令：

```bash
contentcloud daemon start
contentcloud daemon status
contentcloud daemon restart
contentcloud daemon stop
contentcloud daemon run --log-file <path> # 前台调试/服务管理器入口
```

- `start`：相同版本和可执行文件已运行时返回 `already_running`，不重启。
- `status`：返回 installed/running、PID、版本、可执行文件、日志路径和最后一次 runtime health；Daemon 停止后仍可读取持久状态。
- `restart`：重写 LaunchAgent 并切换到当前 CLI 二进制。
- `stop`：停止但保留安装；`contentcloud down --yes` 才撤销设备并卸载。
- `RunAtLoad + KeepAlive`：登录后自动启动，异常退出后由 launchd 拉起。
- LaunchAgent 固化当前 `PATH`、`HOME`、Agent 配置目录和非秘密云配置路径，保证后台能找到 Codex/Claude；API key 不写入 plist，优先使用 Agent 登录态、Keychain 或 Provider 凭据文件。

## 4. 已安装与版本更新

| 状态 | Bootstrap/Update 行为 |
| --- | --- |
| 未安装 Daemon | 注册成功后安装并启动 |
| 同版本、同路径且运行中 | 幂等返回，不重启 |
| 已安装但未运行 | 重新 bootstrap/kickstart |
| CLI 版本或二进制路径变化 | 重写 LaunchAgent，切到新路径并重启 |
| 未启用 Daemon 的用户运行 npm update | `daemon restart --if-installed` 成功跳过，不自动启用 |

`@limecloud/contentcloud` 安装器继续从固定 GitHub Release 下载压缩二进制并校验 `checksums.txt`。更新成功后由新二进制执行 `daemon restart --if-installed`，避免 LaunchAgent 长期指向旧版本。Daemon 每次 `poll` 上报当前 `daemon_version`，服务端设备记录因此反映真实运行版本，而不是初次注册版本。

## 5. Agent 进程监管

- Codex 使用 `codex exec --dangerously-bypass-approvals-and-sandbox`。
- Claude 使用 `--permission-mode bypassPermissions`。
- 每个 Agent 进程使用独立进程组。
- Attempt 取消、心跳失败、租约超时或 Daemon 收到退出信号时，终止整个进程树。
- Agent 标准输出和错误输出都有大小上限；正式结果必须通过冻结的 `output.schema.json`。
- Agent 只能把任务产物写入 Attempt 目录；冻结的 Contract、Skill、Schema 和 ExecutionBundle 均为只读文件。

### 5.1 结果恢复

Daemon 在领取 Attempt 后立即写入权限为 `0600` 的本地 journal。Agent 产出先原子写入 journal，再调用 `run.report`；网络失败、进程崩溃或 daemon 重启时，下次启动会按设备与服务端绑定重放。正在执行但没有结果的 journal 会被标记为 `daemon_restarted` 并通过 `run.finish` 释放租约，避免重复执行。永久服务端拒绝会保留 `.dead` dead-letter 文件；`daemon status` 暴露 `pending_reports` 和 `dead_letters`，供诊断与人工处理。

### 5.2 多 Workspace、并发和日志

本地配置保留旧的单 Workspace 字段，同时维护 `daemon_bindings`：每个绑定有独立设备凭据、服务端和多个 Workspace 根目录。Daemon 轮询多个绑定，默认最多并发 2 个 Attempt，可通过 `CONTENTCLOUD_DAEMON_MAX_CONCURRENT_TASKS` 调整，硬上限为 8。每个 Attempt 使用独立私有目录和服务端租约；Automation 根目录必须与当前绑定中的任一交互 Workspace 根目录互不包含。

Daemon 使用大小上限为 10 MiB、保留 5 个备份的受管日志文件，LaunchAgent 只传入日志路径，API key 不进入服务配置。`daemon status` 额外显示绑定数、活动任务、最近轮询、Provider 版本、待重报数、dead-letter 数和服务端更新策略。

### 5.3 版本策略和进度通道

服务端通过 `CONTENTCLOUD_DAEMON_MIN_VERSION`、`CONTENTCLOUD_DAEMON_LATEST_VERSION` 和 `CONTENTCLOUD_DAEMON_UPDATE_URL` 配置兼容窗口。Daemon 每次 poll 获得 `update_available`/`update_required`，低于最小版本时不再领取新任务，只保留在线状态并提示经过 checksum 校验的 npm 更新器。

Daemon poll 使用最多 25 秒的 HTTP long-poll，短 deadline 不会被固定轮询间隔放大，断线后自动回到 polling fallback。Attempt 心跳写入不可变 `run_progress_events`；CLI 使用 `contentcloud run events <run-id> --after <cursor>` 增量读取，`contentcloud run log <run-id>` 返回完整脱敏进度。服务端提供 `/api/bff/runs/{id}/progress/stream` 的 SSE，支持 `Last-Event-ID` 断线续传。这里没有引入 Alook 的 session/steering 数据模型；进度只反映冻结 Attempt 的执行事实。

## 6. 借鉴 Alook 的范围

参考实现固定为 [`@alook/cli@0.0.160`](https://www.npmjs.com/package/@alook/cli)、[仓库 commit `57e09fd50fbfce715a4b68e7f3b01d1b7296b041`](https://github.com/alookai/alook/tree/57e09fd50fbfce715a4b68e7f3b01d1b7296b041)，Apache-2.0。ContentCloud 借鉴的是经过真实无人值守场景验证的运行模式，不复制其业务协议：

- 已采用：Codex 全权限/无审批、Claude bypass、Agent 完整继承 Daemon 环境、LaunchAgent 固化非秘密运行路径、进程树终止、Daemon 自动启动、心跳、断线轮询恢复、版本更新后重启。
- 已实现：完成结果 journal 重报、HTTP long-poll、进度 SSE、日志轮转、多绑定并发配额、服务端 `update_available/update_required` 策略。
- 有意不复制：Alook 的 WebSocket steering、会话消息、附件浏览和会议模型；这些能力不属于 ContentCloud 的 TaskContract/Attempt 事实源，后续若有明确业务需求再以独立领域对象评审。
- 不直接照搬：Alook 的会话/steering 数据模型。ContentCloud 的事实源仍是 TaskContract、ExecutionBundle、SubmissionRevision 和 ApprovedSnapshot。

## 7. 验收

1. Bootstrap Plan 明确显示 `would_enable_daemon=true`，未确认不能 apply。
2. 注册成功后 Daemon 只启动一次，重复 resume 不产生重复服务。
3. 更新 CLI 后 `daemon status` 显示新版本和新可执行文件。
4. Codex 参数包含 bypass，且不包含 `read-only`；Claude 不包含禁工具或 safe mode。
5. Provider 环境变量可继承，任何 `CONTENTCLOUD_*` 凭据不得进入 Agent。
6. 取消正在执行的 Agent 后，其子 Shell、浏览器和媒体进程不残留。
7. Daemon poll 上报当前版本；服务端 Device 的 daemon_version 随运行版本更新。
8. Automation Agent 不能伪造服务端批准，也不能代替用户操作 Seedance 或抖音。
9. 已完成结果在网络失败时保留在 journal，Daemon 重启后重报并清除 outbox。
10. 未完成 Attempt 在重启后以 `daemon_restarted` 结束，不重复执行同一租约。
11. 两个设备/Workspace 绑定可在默认并发 2 下同时完成 Fixture Attempt，且无共享工作区。
12. 低于最小版本的 Daemon 保持在线但不领取任务；可选更新与强制更新可区分。
13. HTTP long-poll 遵守短等待 deadline，SSE 按 `Last-Event-ID` 恢复且不重放旧事件。
14. Daemon 停止后 `status` 仍可读取最近 runtime health、待重报数和 dead-letter 数。

以上 1-14 已有自动化或本地集成覆盖；LaunchAgent、真实 Provider 登录态、真实网络中断和 npm 覆盖安装仍需在真实 macOS 设备完成发布验收。
