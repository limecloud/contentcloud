# Content Work OS 使用文档

Content Work OS 是 Studio-first 的内容生产与云端治理系统。客户默认在 Web 创作台中选择场景、提交资料、查看进度、确认结果和取得交付；受支持的智能体客户端可作为资料整理、研究、策略和创意修订的执行者，再把明确选择的版本提交到 Web 工作台进行审核、批准和交付。

本文档同时服务两种阅读方式：

- 用户在 Content Work OS 的 `/docs` 文档中心按客户端或内容形态查找指南。
- 智能体和开发者直接读取本目录中的同源 Markdown 文档。

文档只描述已经验证的能力。标记为“有限支持”或“即将支持”的页面不会提供尚未实现的安装、初始化、交接或发布命令。

## 从哪里开始

1. 阅读[开始使用](getting-started.md)，了解如何从 Web 创作台开始任务，以及何时连接执行客户端。
2. 阅读[受治理的内容工作流](concepts/governed-workflow.md)，理解本地候选、云端内容版本和批准快照之间的区别。
3. 按客户端查看接入状态，或按内容形态查看当前支持范围。
4. 只有找到明确标记为“可用”的场景指南后，才执行对应流程。

## 按客户端查看

| 客户端 | 当前状态 | 文档 |
| --- | --- | --- |
| Codex | 完整接入能力可用 | [Codex](clients/codex.md) |
| Claude Code | 控制面有限可用，客户侧完整接入仍在进行 | [Claude Code](clients/claude-code.md) |
| Claude Desktop/Web、Cursor、VS Code GitHub Copilot | 上游协议候选，ContentCloud 尚未准入 | 在 Web 文档中心查看动态状态页 |
| GitHub Copilot 其他 Surface、Kiro、Gemini CLI | Headless/协议候选，尚未准入 | 在 Web 文档中心查看动态状态页 |
| Cline、Windsurf、Continue | 规划状态 | 在 Web 文档中心查看动态状态页 |
| Hermes、OpenClaw、WorkBuddy | 规划状态 | 在 Web 文档中心查看动态状态页 |
| Grok Bot、NanoClaw | 非首发 | 在 Web 文档中心查看动态状态页 |

客户端状态来自智能体客户端注册表（`Agent Client Registry`）。Web 文档中心会按注册表中的最新能力状态生成客户端目录，避免文档和产品状态各自维护一份事实。

## 按内容形态查看

| 内容形态 | 当前状态 | 文档 |
| --- | --- | --- |
| 营销视频 | 可用 | [营销视频](content-types/marketing-video.md) |
| 微信公众号文章 | 可用，需租户开通 | [微信公众号文章](content-types/wechat-article.md) |

未来新增邮件简报、社交媒体帖子、播客脚本或直播话术时，会先增加内容形态页；只有真实可用的“客户端 × 内容形态”组合才会增加场景教程。

## 当前可用场景

- [使用 Codex 制作营销视频内容](guides/marketing-video/codex.md)
- [使用 Codex 制作微信公众号文章](guides/wechat-article/codex.md)

## 状态说明

- **可用**：已有受支持的产品流程和可验证实现。
- **有限支持**：部分底层能力可用，但不能完成完整初始化、交接或内容生产流程。
- **即将支持**：已进入兼容设计，尚无可执行实现。

上游兼容目录只作为调研证据，不会直接改变这里的客户可用状态。每个客户端都必须独立完成安装、Workspace 绑定、stdio MCP 生命周期、呈现或 Headless 降级以及安全验收。

遇到任务、执行客户端连接、本地工作区或任务交接问题时，查看[故障排查](troubleshooting/workspace-and-handoff.md)。

产品分层、运营后台和 Runtime 目标架构见 [`docs/foundation`](../foundation/README.md)、[`docs/product`](../product/README.md) 和 [`docs/roadmap/v8`](../roadmap/v8/README.md)；本目录只描述已经验证、可执行的客户能力。
