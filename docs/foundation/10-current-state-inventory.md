# 当前代码证据与一次性整改基线

状态：`当前实现事实；分层平台、业务工作台、编排主链和 Desktop D3-D7 核心链路已完成，正式签名分发、真实外部依赖和生产故障矩阵仍在进行`。

更新时间：2026-08-23。

本文记录代码今天真实存在什么，以及一次性整改后必须删除什么。目标目录和决策见 [04-code-organization.md](./04-code-organization.md)、[07-migration-and-delivery.md](./07-migration-and-delivery.md)、[ADR-0018](./decisions/ADR-0018-desktop-surface-and-repository-topology.md)。

## 1. 当前物理结构

当前物理目录已经收口为：

```text
apps/{web,desktop}
internal/{application,bootstrap,catalog,experience,integration,local,persistence,platform,runtime,transport,work}
internal/local/{automation,config,desktopapi,export,ingest,workbench,workspace}
internal/persistence/{memory,postgres,blob}
internal/transport/{client,cli,http}
```

旧顶层包已删除，生产代码旧 import 扫描为零。业务事实已按域落到 `internal/identity`、`internal/workspace`、`internal/source`、`internal/catalog`、`internal/work`、`internal/review`、`internal/delivery`、`internal/performance`、`internal/audit` 和 `internal/runtime`；`internal/application` 已完成命名服务化。Desktop Sync、可恢复上传、审批命令、Runtime/Delivery 投影、工作台 Registry、任务级工作台版本固定和 PipelineSummary 已进入当前实现；正式签名分发和真实进程故障矩阵仍未完成。

平台主链保持单一事实源，业务工作台只负责业务体验编排：

```text
业务工作台层       apps/web/src/workbenches + internal/experience/workbench
        ↓           业务导航、阶段、画布和定制主题
应用编排层         internal/application/orchestration + work/task services
        ↓           WorkTask、JobRun、Stage、ActionContract
统一业务事实层     source / work / review / delivery / performance
        ↓           Revision、ApprovedSnapshot、Artifact、Delivery、Observation、Rating
治理与执行层       internal/runtime + catalog + integration + worker
        ↓           能力、权限、租约、CAS、Effect、Provider、回调
产物交付闭环层     internal/delivery + channel_infra + performance_results + lineage
                    交付包、发布回执、效果归因和学习决策
```

## 2. 当前已存在、必须直接复用的事实

| 能力 | 当前证据 | 目标落点 | 处理规则 |
| --- | --- | --- | --- |
| Workspace 目录、View、Claim、Proposal、Apply、CAS | `internal/local/workspace`、`internal/local/workbench` | 当前落点 | 保留同一 Kernel，不复制规则 |
| MCP stdio、Codex Harness、thread resume | `internal/transport/cli`、`internal/integration/agent` | 当前落点；后续可按所有权拆 `transport/mcp` | 不创建 Electron Harness |
| 用户级 Daemon、设备绑定和 Runtime 控制 | `internal/transport/cli/daemon_*`、`internal/transport/cli/runtime_*`、`internal/runtime/worker` | 当前落点；真实长期 Daemon 和故障演练仍待完成 | Workspace Sync 与 Runtime Worker 使用独立状态和错误域 |
| Studio 资产投影 | `internal/experience/studio`、`apps/web/src/studio/assets` | `internal/experience/studio`、`apps/web/src/studio/assets` | 保持投影与业务写模型分离 |
| Cloud Revision、Review、Approval、Artifact、Delivery | `internal/review`、`internal/delivery`、命名应用服务、12 个窄 Repository 组合 | `internal/review`、`internal/delivery` | 应用服务只协调用例，不拥有跨域事实 |
| Runtime Job/Node/Attempt/Effect | `internal/runtime` | `internal/runtime` | 保持独立，不导入内容类型业务模块 |
| 服务端 BFF、Admin、Public | `internal/transport/http`、`apps/web/src/admin`、`apps/web/src/views` | 当前落点 | DTO、路由和 Surface 依赖分开 |

## 3. 一次性目标证据

完成整改后必须满足：

```text
apps/web 存在，web 不存在
apps/desktop 存在且通过 Electron 安全 E2E
internal/local/{workspace,sync,workbench,desktopapi,config} 存在
internal/transport/{http,cli,mcp} 存在
internal/integration/{agent,connector,provider,pluginhost} 存在
internal/persistence/{postgres,memory,blob} 存在
internal/platform/fault 存在且稳定错误不再由宽业务包拥有
identity/workspace/source/catalog/work/review/delivery/performance 独立拥有事实
internal/app、internal/domain、internal/store、internal/httpapi 等旧包不存在
```

## 4. 目录与 import 清理门禁

`pnpm architecture` 和 Go 检查必须阻断：

- 任何生产代码导入旧 `internal/app`、`internal/domain`、`internal/store`、`internal/httpapi`、`internal/localworkspace` 或 `internal/workbench`。
- `apps/desktop` Renderer 使用 Node API、文件系统、通用 IPC 或服务端 Token。
- `apps/web` 与 `apps/desktop` 互相导入业务页面或状态容器。
- Runtime 导入具体内容业务模块并直接修改业务事实。
- 工作台声明导入 Runtime、Review、Delivery、Persistence 或 Integration，或历史任务改用当前选择器版本。
- 工作台登记接口直接写入 `published`，或数据库角色获得 manifest/digest 的通用更新权限。
- SQLite 表保存正文、审批、Revision、Artifact 或 Runtime 终态。
- 新 Repository 重新扩大全局 Store。
- 文档或用户文案把目标 Desktop 能力描述为已发布。

## 5. 当前与目标状态矩阵

| 能力 | 当前状态 | 目标状态 |
| --- | --- | --- |
| Web 迁入 `apps/web` | 已完成，构建和 CI 使用新路径 | 所有脚本和文档持续由门禁防回流 |
| Electron Shell | Forge + Vite、安全窗口、CSP、fuses 和安全 E2E 已实现 | 正式签名与更新验证 |
| Desktop 项目 View | 快照、内容目录、Runtime/Delivery 投影和 allowed actions 已实现 | 完整活动流与细粒度通知 |
| Local Sync Engine | digest、outbox、cursor、幂等发布和冲突状态已实现 | watcher 稳定窗口和多设备冲突 E2E |
| Resumable Upload | 4 MiB 分片、512 MiB 上限、resume/finalize 已实现 | 真实媒体预览与跨平台故障矩阵 |
| Desktop Review Inbox | inbox、Revision diff、批注、批准/拒绝/要求修改已实现 | 真实进程 E2E 与复杂 Gate 组合验收 |
| Agent Handoff | Codex 深链和 Claude Code 命令交接已实现，均绑定项目/修订摘要；Claude CLI `--host claude` 工作区 bootstrap 已按宿主准入 | Claude 网页客户入口、Pi、远程 Agent 和其他宿主仍按 Adapter 逐项验收 |
| 签名分发 | Desktop Forge makers、更新通道契约、本地 package 和 CI package matrix 已有；尚无真实签名发布 | macOS/Windows 正式安装、更新、卸载和恢复 |

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
pnpm desktop:release:check
```

根级脚本、Makefile 和 CI 必须直接覆盖 Desktop typecheck、unit、package 与 Electron E2E；没有脚本不能冒充已验证。
