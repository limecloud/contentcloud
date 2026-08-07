# Content Work OS 与 Codex

状态：**可用**。

Codex 是当前能力最完整的 Content Work OS 客户端，支持本地自动化、工作区注册与初始化、交互式任务交接和受治理创作环境。

## 可用能力

| 能力 | 状态 |
| --- | --- |
| 本地自动化 | 可用 |
| 本地工作区注册 | 可用 |
| 本地工作区初始化 | 可用 |
| 交互式任务交接 | 可用 |
| 创作环境 | 可用 |

## 接入方式

1. 在具备本机配置权限的 Codex Desktop 或 Codex CLI 中运行。
2. 登录 Content Work OS，在项目“执行客户端”页创建连接会话（`ConnectSession`）。
3. 使用页面提供的固定操作指令或初始化计划连接本地工作区。
4. 核对插件市场（`Marketplace`）、插件、目标目录变化和 `plan_id` 后，再确认执行应用操作（`apply`）。
5. 初始化成功后，在同一本地工作区根目录中新建 Codex 对话。
6. 新对话先调用 `workspace_context`；仅在返回 `repair_required` 时调用 `workspace_doctor`。

固定的插件市场、插件身份、命令和安全说明以兼容入口 [`/codex`](/codex) 为准。该入口继续提供浏览器 HTML 和智能体可读文本输出。

## 日常使用

- 使用 `workspace_context` 恢复当前项目、执行记录（`Run`）和任务交接状态。
- 在写入前选择明确的执行记录，并取得单写者声明（`claim`）。
- 使用内容形态对应的技能生成或修订候选。
- 在提交前运行本地检查（`lint`）和 `publish_preflight`。
- 只在用户确认精确的 `plan_id` 后执行提交。
- Web 工作台退回后，通过“在智能体客户端中修订”打开绑定精确内容版本和摘要的新对话。

## 当前内容形态

- [营销视频](../content-types/marketing-video.md)：可用。
- [微信公众号文章](../content-types/wechat-article.md)：可用，需租户显式开通。

完整场景流程见[使用 Codex 制作营销视频内容](../guides/marketing-video/codex.md)和[使用 Codex 制作微信公众号文章](../guides/wechat-article/codex.md)。
