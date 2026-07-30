# ContentCloud 与 Claude Code

状态：**有限支持**。

ContentCloud 已实现 Claude Code 的本地 Automation Adapter，并支持 Workspace 注册。完整的 Workspace 初始化、交互式 Handoff 和受治理创作环境尚未开放。

## 当前能力

| 能力 | 状态 |
| --- | --- |
| 本地 Automation | 可用 |
| Workspace 注册 | 可用 |
| Workspace 初始化 | 即将支持 |
| 交互式 Handoff | 即将支持 |
| 创作环境 | 即将支持 |

本地 Automation 由 ContentCloud 在隔离、冻结的 Attempt Workspace 中调用 Claude Code，并要求结构化输出。它不是用户可自行拼装的完整内容生产流程。

## 当前限制

- Web 的“接入与初始化”暂不能用 Claude Code 完成完整 bootstrap。
- Web 的“在 Agent 中继续”暂不会为 Claude Code 生成恢复入口。
- 当前没有“Claude Code × 营销视频”的完整场景教程。
- 不应套用 Codex 的 Plugin、deep link 或 Handoff 命令。

在这些能力正式发布前，请使用 [Codex](codex.md) 完成完整交互式工作流。页面能力状态会直接来自 Agent Client Registry；能力开放后无需更换文档入口。
