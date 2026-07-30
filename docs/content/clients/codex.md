# ContentCloud 与 Codex

状态：**可用**。

Codex 是当前能力最完整的 ContentCloud 客户端，支持本地 Automation、Workspace 注册与初始化、交互式 Handoff 和受治理创作环境。

## 可用能力

| 能力 | 状态 |
| --- | --- |
| 本地 Automation | 可用 |
| Workspace 注册 | 可用 |
| Workspace 初始化 | 可用 |
| 交互式 Handoff | 可用 |
| 创作环境 | 可用 |

## 接入方式

1. 在具备本机配置权限的 Codex Desktop 或 Codex CLI 中运行。
2. 登录 ContentCloud，在项目“接入与初始化”页创建 ConnectSession。
3. 使用页面提供的固定 Prompt 或 bootstrap 计划连接 Workspace。
4. 核对 Marketplace、Plugin、目标目录变化和 `plan_id` 后，再确认执行 apply。
5. 初始化成功后，在同一 Workspace Root 新建 Codex 对话。
6. 新对话先调用 `workspace_context`；仅在返回 `repair_required` 时调用 `workspace_doctor`。

固定 Marketplace、Plugin 身份、命令和安全说明以兼容入口 [`/codex`](/codex) 为准。该入口继续提供浏览器 HTML 和 Agent 可读文本输出。

## 日常使用

- 使用 `workspace_context` 恢复当前项目、Run 和 Handoff。
- 在写入前选择明确的 Run，并取得单写者 claim。
- 使用内容形态对应的 Skill 生成或修订候选。
- 在 publish 前运行本地 lint 和 `publish_preflight`。
- 只在用户确认精确 `plan_id` 后执行 publish。
- Web 退回后，通过“在 Agent 中修订”打开绑定精确 Revision 和 digest 的新对话。

## 当前内容形态

- [营销视频](../content-types/marketing-video.md)：可用。
- [微信公众号文章](../content-types/wechat-article.md)：可用，需租户显式开通。

完整场景流程见[使用 Codex 制作营销视频内容](../guides/marketing-video/codex.md)和[使用 Codex 制作微信公众号文章](../guides/wechat-article/codex.md)。
