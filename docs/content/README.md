# ContentCloud 使用文档

ContentCloud 是本地优先的内容生产与云端治理系统。你可以在受支持的 Agent 客户端中完成资料整理、可信知识、策略、Brief 和内容创作，再把明确选择的版本提交到 Web 进行审核、批准和交付。

本文档同时服务两种阅读方式：

- 用户在 ContentCloud 的 `/docs` 文档中心按客户端或内容形态查找指南。
- Agent 和开发者直接读取本目录中的同源 Markdown。

文档只描述已经验证的能力。标记为“有限支持”或“即将支持”的页面不会提供尚未实现的安装、初始化、交接或发布命令。

## 从哪里开始

1. 阅读[开始使用](getting-started.md)，了解如何选择客户端和内容形态。
2. 阅读[受治理的内容工作流](concepts/governed-workflow.md)，理解本地候选、云端 Revision 和批准快照之间的区别。
3. 按客户端查看接入状态，或按内容形态查看当前支持范围。
4. 只有找到明确标记为“可用”的场景指南后，才执行对应流程。

## 按客户端查看

| 客户端 | 当前状态 | 文档 |
| --- | --- | --- |
| Codex | 完整接入能力可用 | [Codex](clients/codex.md) |
| Claude Code | 本地 Automation 与 Workspace 注册可用，其他能力仍在接入 | [Claude Code](clients/claude-code.md) |
| WorkBuddy | 即将支持 | 在 Web 文档中心查看动态状态页 |
| Cursor | 即将支持 | 在 Web 文档中心查看动态状态页 |
| Hermes | 即将支持 | 在 Web 文档中心查看动态状态页 |
| OpenClaw | 即将支持 | 在 Web 文档中心查看动态状态页 |

客户端状态来自 ContentCloud Agent Client Registry。Web 文档中心会按 Registry 的最新能力状态生成客户端目录，避免文档和产品状态各自维护一份事实。

## 按内容形态查看

| 内容形态 | 当前状态 | 文档 |
| --- | --- | --- |
| 营销视频 | 可用 | [营销视频](content-types/marketing-video.md) |
| 微信公众号文章 | 可用，需租户开通 | [微信公众号文章](content-types/wechat-article.md) |

未来新增 Newsletter、社交媒体帖子、播客脚本或直播话术时，会先增加内容形态页；只有真实可用的“客户端 × 内容形态”组合才会增加场景教程。

## 当前可用场景

- [使用 Codex 制作营销视频内容](guides/marketing-video/codex.md)
- [使用 Codex 制作微信公众号文章](guides/wechat-article/codex.md)

## 状态说明

- **可用**：已有受支持的产品流程和可验证实现。
- **有限支持**：部分底层能力可用，但不能完成完整初始化、交接或内容生产流程。
- **即将支持**：已进入兼容设计，尚无可执行实现。

遇到 Workspace、初始化或 Handoff 问题时，查看[故障排查](troubleshooting/workspace-and-handoff.md)。
