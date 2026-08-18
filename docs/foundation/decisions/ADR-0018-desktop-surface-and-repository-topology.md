# ADR-0018：Desktop 产品面、Electron 技术栈与仓库拓扑

状态：`Accepted`

日期：2026-08-17。

决策者：产品与平台工程。

关联：

- [Content Work OS Desktop](../../product/content-work-os-desktop/README.md)
- [系统架构与 Agentic Job Runtime](../03-system-and-runtime.md)
- [代码组织、模块边界与依赖规范](../04-code-organization.md)
- [ADR-0017 Codex Runtime Harness](./ADR-0017-codex-runtime-harness.md)

## 背景

ContentCloud 已经生成包含上下文、来源、知识、生产、结果和交付的本地项目目录，但现有主要呈现通道仍围绕 Web、CLI、MCP Apps 和短期 Browser Workbench。Codex 只会在用户与 AI 交互时渲染当前任务，不能持续承担项目目录、同步队列、上传进度、审批收件箱、大媒体预览和系统通知。

同时，决策制定时仓库把 Web 放在根 `web/`，Go 代码分散在宽泛的 `internal/app`、`internal/domain`、`internal/store` 和 `internal/httpapi`。新增 Desktop 时继续沿用这些路径会扩大含混边界，并形成 Electron Node 后端、Go CLI 和服务端三套应用层。

项目仍处于早期研发期，没有生产兼容负担。本决策采用一次性目标结构，不保留旧 import、类型别名、兼容 Facade、双写或旧目录转发。

## 决策

### 1. 三种工作面

1. **Codex Surface** 是任务期 AI 工作面，负责需求理解、推理、生成、解释、工具调用和本次 Proposal 确认。
2. **Content Work OS Desktop** 是持续项目工作面，负责项目目录、资产预览、同步、上传、审批、任务状态、通知和交付。
3. **Web Studio / Operations** 是团队与组织工作面，负责跨设备协作、租户治理、运营配置、公开审核和 Runtime 诊断。

三者使用同一业务对象、稳定引用、revision、digest、权限和命令规则，但使用不同 View。禁止为了复用 UI 而把 Codex 会话、Desktop 项目面和 Web 团队面做成同一页面。

### 2. Electron 技术栈

- Electron 使用实施时受支持的稳定版本并锁定精确版本。
- Renderer 复用仓库现有 React 18、TypeScript 5.7、Vite 6、React Router 和 Lucide。
- 构建、打包和发布统一使用 Electron Forge 与其 Vite Plugin，不同时引入 `electron-vite` 和 `electron-builder`。
- 测试使用 Vitest 和 Playwright Electron。
- `BrowserWindow` 固定 `sandbox: true`、`contextIsolation: true`、`nodeIntegration: false`。
- Preload 只通过 `contextBridge` 暴露版本化、运行时校验的窄命令和事件。
- Renderer 不读取文件系统、系统凭据、任意 URL、任意命令或 ContentCloud 设备 Token。

Electron Main 只拥有窗口、托盘、系统通知、协议处理、签名更新、应用生命周期和受控 IPC。Workspace、同步、上传、审批事务、服务端 API、重试和冲突规则由 Go 本地服务拥有。

### 3. 本地进程边界

现有 `contentcloud daemon` 扩展为用户级本地服务进程，在独立模块内托管 Runtime Worker 与 Desktop Workspace Sync。两个子系统共享设备身份、连接健康和进程生命周期，但不共享业务状态机。

```text
Electron Renderer
  -> typed Preload Bridge
  -> Electron Main
  -> authenticated local API / event stream
  -> ContentCloud Go Daemon
       |-> Local Workspace Kernel
       |-> Sync / Upload / Review Inbox
       |-> Runtime Worker
       `-> ContentCloud Server

Codex
  -> stdio MCP Adapter
  -> same Local Workspace Kernel and Server Commands
```

Electron 退出不自动停止用户级 Daemon。用户显式退出后台服务、注销设备或卸载时才停止长期同步和 Runtime 准入。

### 4. 目标仓库拓扑

```text
apps/
├── web/
└── desktop/
    ├── src/main/
    ├── src/preload/
    ├── src/renderer/
    └── tests/e2e/

packages/
├── contentcloud/
├── ui/
└── contracts-ts/

cmd/
├── contentcloud/
├── contentcloud-server/
└── contentcloud-worker/

internal/
├── identity/ workspace/ source/ catalog/
├── work/ review/ delivery/ performance/
├── runtime/
├── experience/{studio,operations,projection}/
├── local/{workspace,sync,workbench,desktopapi,config}/
├── integration/{agent,connector,provider,pluginhost}/
├── persistence/{postgres,memory,blob}/
├── transport/{http,cli,mcp}/
└── bootstrap/
```

`packages/ui` 只共享设计 Token、品牌、图标封装和无业务所有权的 UI 原语。Desktop 和 Web 不共享业务页面、路由、状态容器或权限推断。

### 5. 一次性切换

- `web/` 直接迁入 `apps/web/`，所有构建、CI、Docker、脚本和文档引用同步修改。
- `localworkspace`、`workbench`、`localconfig` 直接迁入 `internal/local`，不保留旧包。
- `agentadapter`、Connector 和 Provider 直接迁入 `internal/integration`。
- CLI、HTTP 和 MCP 直接迁入 `internal/transport`。
- 业务模型、命令、查询和 Repository 端口按事实所有者直接迁入业务模块。
- 开发数据库和本地 Fixture 可以重建；不为尚未发布的数据形态增加兼容迁移。
- 已提交且用于当前 Schema 的数据库迁移历史仍保持可重放；是否 squash 由独立数据库基线决策处理，不在目录重构中隐式删除。

完成时旧目录、旧 import、旧脚本路径、兼容别名和宽接口引用必须为零。实现过程可以按依赖顺序验证，但不得把中间兼容状态作为可合并结果。

## 备选方案

### 方案 A：只依赖 Codex 和 MCP Apps

无法持续呈现项目目录、后台传输、审批收件箱和系统通知，也无法在 AI 会话结束后继续同步。不采用。

### 方案 B：Electron 直接包装远程 Web

交付较快，但离线目录、系统权限、本地大媒体、Daemon 生命周期和安全凭据边界仍然缺失，并把 Node Renderer 变成高权限网络客户端。不采用。

### 方案 C：Electron 内实现 Node 业务后端

会复制 Go 中的 Workspace、Claim、Proposal、审批、上传和服务端客户端，形成第二套业务内核。不采用。

### 方案 D：Tauri

包体较小，但仓库已有 React/Vite 与 Go 本地服务；引入 Rust 只为 Desktop Shell 增加第三种后端语言和工具链，没有解决新的业务问题。不采用。

### 方案 E：渐进兼容迁移

生产系统通常应保留兼容窗口，但当前处于早期研发期。保留旧路径、别名和双写只会增加长期清理成本。不采用。

## 事实所有权与边界

| 事实 | 权威所有者 | Surface 权限 |
| --- | --- | --- |
| 未提交项目文件和本地草稿 | Local Workspace | Desktop 持续展示；Codex 经 Kernel 修改 |
| 云端 Revision、审批、团队和交付 | ContentCloud Server | Desktop/Web/Codex 通过命令和查询使用 |
| AI thread 与 transcript | Codex Host | ContentCloud 只保存允许的不透明会话引用 |
| Desktop 窗口和系统通知 | Electron Main | 不得升级为业务事实 |
| 同步缓存、索引和 outbox | Go Local Service | 可删除、可重建，不是第三事实源 |

## 安全与运行影响

- 所有 Renderer 输入均视为不可信，Main 和 Go 服务重新校验命令、Workspace、revision、digest、权限和幂等键。
- Main 拒绝任意导航、新窗口、权限请求和未经允许的外部协议。
- 设备 Token 和上传凭据不进入 Renderer、URL、日志、环境变量或 Codex 上下文。
- Desktop 发布必须包含 macOS 签名与 notarization、Windows 签名、更新签名验证和安装包冒烟。
- Linux 可以保持预览，但不能用未签名包冒充正式跨平台交付。

## 验证

1. 架构门禁确认旧目录和旧 import 为零。
2. Electron E2E 确认 Renderer 无 Node 能力、Preload 白名单稳定、外部导航被拒绝。
3. Codex Apply 后 Desktop 收到同一 revision/digest 的失效事件并刷新。
4. Electron 关闭后 Daemon 继续完成已排队同步；重开后恢复进度。
5. Web、Desktop 和 Codex 针对同一审批输入版本得到一致状态。
6. macOS arm64/x64 与 Windows x64 安装、升级、卸载和恢复路径完成签名构建验证。

## 回退

若 Desktop 未达到发布门槛，可以不分发 Electron 包，继续使用 Web、Codex、Direct Browser 和 Headless。代码目录不回退到旧结构；修复应在目标模块内完成。

## 后果

正面：项目持续体验、AI 任务体验和团队治理体验各自清晰；Electron 不复制 Go 业务内核；仓库目录直接表达所有权。

代价：一次性改动范围大，所有 import、构建、测试、容器、发布和文档必须在同一整改中完成，不能留下半迁移状态。
