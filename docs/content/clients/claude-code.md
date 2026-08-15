# Content Work OS 与 Claude Code

状态：**有限支持**。

Content Work OS 已实现 Claude Code 的本地自动化适配器、私有 Plugin 投影和本地工作区注册。投影使用 `${CLAUDE_PROJECT_DIR}` 把稳定项目根注入 stdio MCP，因此插件从只读安装目录启动时，`workspace_context` 仍能绑定用户当前项目。完整的客户侧初始化、交互式任务交接和受治理创作环境尚未开放。

## 当前能力

| 能力 | 状态 |
| --- | --- |
| 本地自动化 | 可用 |
| 本地工作区注册 | 可用 |
| Claude Plugin / 中文 Skills | 控制面可用 |
| stdio MCP 自动生命周期 | 控制面可用 |
| 当前项目根自动绑定 | 已实现并有契约测试 |
| 本地工作区初始化 | 即将支持 |
| 交互式任务交接 | 即将支持 |
| 创作环境 | 即将支持 |
| MCP Apps 内联工作台 | 未验证 |

本地自动化由 Content Work OS 在隔离、固定的任务尝试工作区中调用 Claude Code，并要求结构化输出。它不是用户可自行拼装的完整内容生产流程。

## 当前限制

- Web 工作台的“执行客户端”暂不能用 Claude Code 完成完整连接。
- Web 工作台的“在智能体客户端中继续”暂不会为 Claude Code 生成恢复入口。
- 当前没有“Claude Code × 营销视频”的完整场景教程。
- Claude Code 使用自己的 `.claude-plugin`、Marketplace 和 `.mcp.json` 投影；它不在 Agent Plugins 官方兼容客户端目录中，不应套用 Codex 的插件、深度链接或任务交接命令。
- Claude Code 的 Chrome/Edge 集成可以操作 localhost 页面，但不会自动消费 ContentCloud 私有 handoff，也不能证明 token 不进入模型上下文。
- Claude Code CLI 当前没有已证明的 MCP Apps 内联 UI；富 UI 不可用时只能使用 Tool、`structuredContent` 和 MCP Resource。

在这些能力正式发布前，请使用 [Codex](codex.md) 完成完整交互式工作流。页面能力状态会直接来自智能体客户端注册表；能力开放后无需更换文档入口。
