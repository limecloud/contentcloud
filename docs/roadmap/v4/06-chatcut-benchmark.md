# ChatCut Agent + 云端编辑器对标

## 1. 审计范围

审计时间：2026-07-27。

公开证据：

- 产品入口：<https://chatcut.io/chatgpt>。
- 云端编辑器：`https://app.chatcut.io/<locale>/editor/<projectId>`。
- 公开 Plugin 仓库：<https://github.com/ChatCut-Inc/agent-plugin>。
- 审计 commit：`4748eef373ff7dbe64e959ae44f3351d468564ba`。
- 远程 MCP：`https://api.chatcut.io/api/external-mcp/mcp`。

本次只使用公开页面、公开 Plugin 和未认证 HTTP 响应，不推断 ChatCut 未公开的服务端实现。

## 2. 已确认架构

ChatCut 采用“本地 Agent 宿主 + 远程 MCP + 云端编辑器”的形态：

```text
Codex / ChatGPT Desktop
  ├─ local Plugin + Skills
  ├─ OAuth -> hosted ChatCut MCP
  │            -> Project / Timeline / Asset commands
  └─ Browser -> app.chatcut.io/.../editor/<projectId>
                         │
                         └─ same cloud project state
```

公开 Plugin 明确声明：

- MCP 通过 OAuth 连接远程 HTTP endpoint。
- Tool 对云端 Project、Timeline 和 Asset 执行真实修改。
- 用户可以在云端编辑器中观看 Agent 变更并继续手工编辑。
- `create_project`、`target_project`、`get_editor_url` 等结果可以返回 editor URL 和 Browser 导航提示。
- Codex 内置 Browser 使用精确 handoff URL；对外链接使用不含内部参数的 clean editor URL。

因此 ChatCut 不是 Slides 式 localhost 应用。它的项目事实、编辑器和远程 MCP 都在云端。

## 3. 本地媒体不等于本地项目

ChatCut 的 Codex Plugin 包含本地媒体导入辅助程序和 FFmpeg。其职责是：

```text
local media
  -> short-lived import session
  -> local preparation / normalization
  -> upload to ChatCut cloud
  -> cloud Asset
  -> editable Timeline item
```

这是一层本地 I/O 伴随能力，不是第二套本地 ChatCut 数据库。云端导出、远程帧检查和跨设备编辑最终依赖云端可读的 Asset。

这个边界与 ContentCloud 的“本地轻伴随层”思路相符：只把必须接触本机的数据处理留在本地，不复制完整云端产品。

## 4. Browser 为什么可持续交互

ChatCut 的 Agent 与 Browser 共享同一个云端项目事实：

```text
Agent MCP command -> cloud timeline state <- user edits in Web editor
```

所以 Browser 不是一次性结果预览，而是持续可见的云端工作台。Agent 创建、选择或修改项目后，会再次对齐当前 editor URL；用户也能直接播放、调整时间线和继续操作。

右侧面板仍由 Codex/ChatGPT 宿主 Browser 提供。ChatCut 负责返回 URL 和导航提示，不是 ChatCut 自己创建宿主右栏。

## 5. `/chatgpt` 双模入口

`chatcut.io/chatgpt` 面向两类消费者：

| 请求 | 返回 |
| --- | --- |
| 普通浏览器页面导航 | 人类可读的安装页面和复制 Prompt |
| 非浏览器/Agent 请求 | 纯文本安装、OAuth、验证和新对话交接指南 |

指南先判断宿主是否能安装桌面 Plugin，然后执行 Marketplace 安装、MCP OAuth、验证和新对话交接。它解决了“用户不知道如何把一个 SaaS 接入 Agent”的首用摩擦。

ContentCloud 可以采用同类入口，但不能把网页文字直接当作越权安装授权。官方入口只发布固定、版本化、无秘密的安装与 bootstrap 指南，Agent 仍遵守宿主确认和本地权限边界。

## 6. 与 ContentCloud 的相同点

| 相同模式 | ContentCloud 采用方式 |
| --- | --- |
| Plugin 提供领域 Skill 和 MCP 能力 | Scene Plugin + typed Tools |
| Tool 返回精确项目页面 | `resource_link` + typed Browser navigation |
| Browser 保持用户可见 | 云端治理工作台与 Codex 并排 |
| 稳定 project/object ID 定位 | project/view/focus/revision digest |
| OAuth/登录与业务授权分离 | bootstrap PKCE + Web session + tenant authorization |
| Browser 不可用时提供 clean link | CLI/IDE/fallback 契约 |
| 本地只承担必要 I/O | Workspace、LocalRun、lint、publish/pull |

这些证据支持 V4 使用云端 Web，而不是复制 Slides 的本地 Next.js/Electron 拓扑。

## 7. 与 ContentCloud 的根本差异

| 维度 | ChatCut | ContentCloud |
| --- | --- | --- |
| 作品事实源 | 云端 Project/Timeline | 本地候选 + 云端正式 Revision/Snapshot |
| Agent 主要写入 | 直接修改云端可编辑 Timeline | 修改本地候选，显式 publish 后形成云端 Revision |
| Browser 主要职责 | 创作和编辑同一作品 | 协作治理、证据、审核、决定和任务 |
| 本地原始数据 | 通过导入流程进入云端 Asset | 按 SourceDisclosure 决定是否及如何上传 |
| 人工边界 | 用户可继续编辑 Timeline | 最终 Decision 必须绑定 digest 和权限 |
| 同步方式 | 同一云端项目即时可见 | publish/pull 显式跨边界交换 |

因此 ContentCloud 不应复制“所有 Agent 修改都直接写云端可编辑对象”。这会绕开 LocalRun、披露预览和 Submission 审核轨道。

## 8. V4 采纳项

1. Browser 定义为持续可见、可操作的云端治理工作台，不是被动仪表盘。
2. bootstrap、publish、pull、doctor 成功后返回精确页面，并在 Browser 可用时立即打开。
3. 新增官方 `/codex` 双模接入入口，分别服务人类和 Agent。
4. clean resource link 与宿主导航提示职责分离；首版仍保持同一无秘密 URL。
5. Web 针对窄右栏提供聚焦布局，而不是把宽屏后台原样缩放。
6. 用户可以在 Web 执行 Comment、Decision、Assignment、Context Revision 和 Automation Plan 等允许的云端命令。

## 9. V4 不采纳项

- 不把云端 ProjectProjection 变成可随意直接编辑的正式业务对象。
- 不自动上传 Workspace、LocalRun transcript 或未发布候选。
- 不建设本地 ContentCloud Web 数据库、Next.js Studio 或 Electron。
- 不用 Browser 页面代替 publish/pull。
- 不在首版 resource link 中加入登录 token 或类似 `editor-boot-token` 的参数。
- 不因为 Agent 打开审核页就自动执行最终人工决定。

## 10. 结论

ChatCut 验证的是 ContentCloud V4 的产品表面，而不是要求 ContentCloud 改成全云端创作：

```text
产品体验学习 ChatCut
  -> Plugin 接入简单
  -> Browser 持续可见
  -> 深链定位精确
  -> Web 可以完成自身职责

数据边界坚持 ContentCloud
  -> 本地候选不自动上传
  -> 云端只持有正式交换物和治理事实
  -> publish/pull 显式
  -> 人工决定不可绕过
```

最终选择仍是“本地创作 + 云端治理 + 双向工作台”的混合模式。
