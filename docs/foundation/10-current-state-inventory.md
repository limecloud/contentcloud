# 当前代码证据与一次性整改基线

状态：`当前实现事实；随一次性整改更新`。

更新时间：2026-08-17。

本文记录代码今天真实存在什么，以及一次性整改后必须删除什么。目标目录和决策见 [04-code-organization.md](./04-code-organization.md)、[07-migration-and-delivery.md](./07-migration-and-delivery.md)、[ADR-0018](./decisions/ADR-0018-desktop-surface-and-repository-topology.md)。

## 1. 当前物理结构

当前仍有以下宽包或历史路径：

```text
internal/domain
internal/app
internal/store
internal/httpapi
internal/agentadapter
internal/localworkspace
internal/workbench
internal/localconfig
internal/environment
internal/mediapipeline
```

这些路径不是目标架构，也不允许继续增加新功能。Web Surface 已通过 Git rename 迁入 `apps/web`，但构建、CI、脚本、Docker 和文档引用仍需全量核对。`apps/desktop`、`internal/local/sync`、Desktop API 和 Electron 分发尚未实现。

## 2. 当前已存在、必须直接复用的事实

| 能力 | 当前证据 | 目标落点 | 处理规则 |
| --- | --- | --- | --- |
| Workspace 目录、View、Claim、Proposal、Apply、CAS | `internal/localworkspace`、`internal/workbench` | `internal/local/workspace`、`internal/local/workbench` | 直接移动并保留测试，不复制规则 |
| MCP stdio、Codex Harness、thread resume | `internal/cli`、`internal/agentadapter` | `internal/transport/mcp`、`internal/integration/agent` | 直接迁移，不创建 Electron Harness |
| 用户级 Daemon、设备绑定和 Runtime 控制 | `internal/cli/daemon_*`、`internal/cli/runtime_*` | `internal/local`、`internal/runtime`、`internal/transport/cli` | Workspace Sync 与 Runtime Worker 分模块托管 |
| Studio 资产投影 | `internal/experience/studio`、`apps/web/src/studio/assets` | `internal/experience/studio`、`apps/web/src/studio/assets` | 保持投影与业务写模型分离 |
| Cloud Revision、Review、Approval、Artifact、Delivery | `internal/domain`、`internal/app`、`internal/store` | `internal/review`、`internal/delivery` | 按事实所有者迁移，不保留宽 Service |
| Runtime Job/Node/Attempt/Effect | `internal/runtime` | `internal/runtime` | 保持独立，不导入内容类型业务模块 |
| 服务端 BFF、Admin、Public | `internal/httpapi`、`apps/web/src/admin`、`apps/web/src/views` | `internal/transport/http`、`apps/web` | DTO、路由和 Surface 依赖分开 |

## 3. 一次性目标证据

完成整改后必须满足：

```text
apps/web 存在，web 不存在
apps/desktop 存在且通过 Electron 安全 E2E
internal/local/{workspace,sync,workbench,desktopapi,config} 存在
internal/transport/{http,cli,mcp} 存在
internal/integration/{agent,connector,provider,pluginhost} 存在
internal/persistence/{postgres,memory,blob} 存在
identity/workspace/source/catalog/work/review/delivery/performance 独立拥有事实
internal/app、internal/domain、internal/store、internal/httpapi 等旧包不存在
```

## 4. 目录与 import 清理门禁

`pnpm architecture` 和 Go 检查必须阻断：

- 任何生产代码导入旧 `internal/app`、`internal/domain`、`internal/store`、`internal/httpapi`、`internal/localworkspace` 或 `internal/workbench`。
- `apps/desktop` Renderer 使用 Node API、文件系统、通用 IPC 或服务端 Token。
- `apps/web` 与 `apps/desktop` 互相导入业务页面或状态容器。
- Runtime 导入具体内容业务模块并直接修改业务事实。
- SQLite 表保存正文、审批、Revision、Artifact 或 Runtime 终态。
- 新 Repository 重新扩大全局 Store。
- 文档或用户文案把目标 Desktop 能力描述为已发布。

## 5. 当前与目标状态矩阵

| 能力 | 当前状态 | 目标状态 |
| --- | --- | --- |
| Web 迁入 `apps/web` | 已完成目录 rename，引用整理中 | 所有构建、CI、Docker、脚本和文档通过 |
| Electron Shell | 未实现 | Forge + Vite，Main/Preload/Renderer 安全边界 |
| Desktop 项目 View | 未实现 | 目录、资产、任务、审批、传输和活动 |
| Local Sync Engine | 未实现 | watcher、digest、outbox、cursor、冲突和恢复 |
| Resumable Upload | 服务端已有部分上传能力 | Desktop 分片、恢复、finalize 和处理事件 |
| Desktop Review Inbox | 服务端 Review/Approval 已有 | 精确 revision/digest 的持续审批工作面 |
| Codex Desktop Handoff | MCP/Browser Handoff 已有 | 对象引用、任务意图和受控深链 |
| 签名分发 | CLI Release 路径已有，Desktop 无 | macOS/Windows 正式安装、更新、卸载和恢复 |

## 6. 验证命令

```text
go test ./...
go vet ./...
pnpm architecture
pnpm --dir apps/web typecheck
pnpm --dir apps/web test
pnpm --dir apps/web build
pnpm --dir apps/desktop typecheck
pnpm --dir apps/desktop test
pnpm --dir apps/desktop test:e2e
```

Desktop 命令在应用目录建立后才加入根门禁；在此之前不能用“没有脚本”冒充已验证。
