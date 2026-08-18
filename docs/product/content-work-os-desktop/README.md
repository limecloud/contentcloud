# Content Work OS Desktop

状态：`Preview 实现进行中；D3-D7 的核心链路已实现，正式签名、更新和多平台安装门禁仍未完成`。

更新时间：2026-08-18。

上位规范：[平台基线](../../foundation/README.md)、[ADR-0018](../../foundation/decisions/ADR-0018-desktop-surface-and-repository-topology.md)、[ADR-0019](../../foundation/decisions/ADR-0019-local-cloud-sync-authority.md)。本目录是 Desktop 产品、技术边界和交付门禁的唯一专项事实源。

## 1. 产品结论

Content Work OS Desktop 是持续存在的项目工作面，不是 Codex 外壳、Web 包装器、通用文件管理器或第二套 Agent 对话产品。

```text
Codex
  负责一次 AI 任务的理解、推理、生成、解释和确认

Desktop
  负责跨会话存在的项目目录、同步、上传、审批、任务和交付

Web Studio / Operations
  负责跨设备团队协作、租户治理、运营配置和运行诊断
```

三者对同一对象使用相同 ID、revision、digest、权限和命令语义，但各自读取为当前工作面设计的类型化 View。

## 2. 用户价值

Desktop 解决 Codex 和浏览器单独无法稳定解决的问题：

- 项目内容目录始终可见，不依赖某次 AI 会话。
- 本地修改、Codex Apply、上传和云端 Revision 有连续状态。
- 大图片、PDF、音频和视频可以本地预览、传输和恢复。
- 审批、评论、变更请求和交付进入持续收件箱。
- 断网后仍可整理和创作，恢复连接后安全同步。
- Daemon、Connector、凭据、通知、更新和诊断具有系统级生命周期。

## 3. 一级信息架构

```text
项目
├── 概览             当前阶段、待办、最近变化、阻断
├── 内容目录
│   ├── 上下文        10-context
│   ├── 来源          20-sources
│   ├── 知识          30-knowledge
│   ├── 工作          40-work
│   ├── 生产          50-production
│   ├── 交付          60-delivery
│   ├── 结果          70-results
│   └── 归档          90-archive
├── 资产              上传资料、生成结果、大媒体预览和血缘
├── 任务              运行阶段、等待输入、失败和恢复
├── 审批              待审、变更请求、已批准、已拒绝、已过期
├── 传输              上传、下载、同步、冲突和失败重试
└── 活动              评论、决定、发布、交付和审计摘要

全局
├── 项目切换
├── 收件箱
├── Connector
├── 设备与账号
└── 设置与诊断
```

内容目录同时提供物理目录视图和业务视图。物理路径用于本地可检查性，业务视图用于类型、状态、来源、审批和血缘。两者引用同一文件和对象，不复制正文。

## 4. Desktop 与 Codex 的协作

Desktop 中允许出现以下 AI 协作命令：

- “交给 Codex 修改”
- “解释这条审核意见”
- “基于此版本继续创作”
- “修复同步冲突中的文本版本”
- “生成这个交付项缺失的内容”

这些命令只创建带稳定对象引用、revision、digest 和意图的 Handoff。Desktop 不嵌入聊天界面，不复制 Codex transcript。Codex 完成修改后仍通过 Workspace Claim、Proposal 和 Apply 写入，Desktop 从同一事件和 View 读取结果。

Codex 可以在任务结果中提供“在 Desktop 中查看”的受控对象引用。是否能直接导航由 Host Capability 决定；不支持时返回可读对象位置，不把本机绝对路径或秘密放入模型内容。

## 5. Desktop 必须拥有的工作流

1. 打开或创建已绑定 Workspace。
2. 查看并检索完整项目内容目录。
3. 观察本地、Codex 和外部编辑器产生的变化。
4. 将本地内容安全同步为 Cloud Revision。
5. 拖入和批量上传资料，观察处理状态并恢复失败传输。
6. 接收审批，查看精确版本、评论并作出决定。
7. 查看 Runtime 进度、等待条件、失败和用户可执行恢复动作。
8. 组装交付、执行发布前检查并查看发布回执。
9. 在断网、重启、升级和多设备冲突后恢复。

## 6. 不属于 Desktop 的能力

- 不实现第二套 Agent Chat、Prompt IDE 或 Codex transcript 浏览器。
- 不实现通用 IDE、任意代码执行器或全磁盘文件管理器。
- 不接管平台运营、租户管理员和跨租户 Runtime Explorer。
- 不直接写服务端数据库、对象存储或审批事实。
- 不用 Renderer 状态、按钮隐藏或自然语言确认承担授权。
- 不为实时多人逐字符编辑提前引入 CRDT。

## 7. 文档导航

| 文档 | 作用 |
| --- | --- |
| [02-architecture-and-technology.md](./02-architecture-and-technology.md) | 进程、组件、技术栈、安全和部署边界 |
| [03-sync-review-upload.md](./03-sync-review-upload.md) | 同步、上传、审批、冲突、离线和恢复协议 |
| [04-delivery-plan.md](./04-delivery-plan.md) | 一次性代码整改、测试、分发和完成定义 |
| [05-release-and-updates.md](./05-release-and-updates.md) | 跨平台打包、签名、更新通道和发布门禁 |
| [技术图册](../../tech/README.md) | Mermaid 图源、SVG/PNG 渲染产物与可编辑 Excalidraw |

## 8. 当前实现口径

截至 2026-08-18：

- Local Workspace、Claim、Proposal、Apply、Browser Workbench、MCP Apps 最小协议、Daemon、设备绑定、服务端 Revision、审批、资产和 Runtime 已分别存在。
- Web Surface 位于 `apps/web/`；Electron Surface 位于 `apps/desktop/`。
- Electron 43.4.0、Forge/Vite、React Renderer、安全窗口、窄 Preload 白名单、Daemon 离线态、Vitest、Playwright Electron E2E 和本地未签名 package 已完成。
- Go Desktop API 已接入长期运行 Daemon，只监听 `127.0.0.1` 随机端口，使用 `0600` 发现文件与 capability 鉴权，只返回 Workspace-relative 投影。
- Workspace Revision 发布、4 MiB 可恢复分片上传（512 MiB 单文件上限）、持久 outbox/cursor、审批收件箱/Revision diff/批注/批准/拒绝/要求修改、Runtime/Delivery 项目投影已接入。
- Electron 通过 typed Preload 调用 Daemon，Daemon 再使用设备绑定的 Cloud client；Renderer 没有通用 HTTP、文件系统或 Cloud token 权限。
- 正式签名、notarization、自动更新、Windows 真机安装和跨平台升级仍是发布门禁，当前不能宣称已完成。

对外用户文档在正式门禁通过前只能标记 Desktop 为目标或 Preview，不得宣称可下载或已支持全平台。
