# 营销视频

状态：**可用**。

营销视频是 Content Work OS 当前正式支持的内容形态。系统覆盖可信知识、受众策略、创作简报、营销视频剧本、分镜、Seedance 提示包、审核和交付治理。

## 生产链

```text
来源资料
  -> 证据与可信知识
  -> 受众与营销策略
  -> 创作简报
  -> 内容批次与营销视频剧本
  -> Web 工作台审核与已批准快照
  -> 分镜与 Seedance 交付
```

## 当前技能

- 工作区管理：读取、恢复、交接、提交和打开受治理的 Web 视图。
- 知识提取：从固定的证据包中提取有依据的知识候选。
- 营销视频剧本：生成或修订带引用的营销内容。
- 抖音受众策略：形成抖音电商受众策略。
- 分镜生产：从已批准内容生成分镜生产对象。
- Seedance 导出：生成受约束的 Seedance 交付包。
- Seedance 2.5 执行（预览）：在费用批准后，通过 ContentCloud Media Job 执行单镜头生成并回收已校验 Artifact；正式开放前仍需真实 Provider 验收。

## 关键门禁

- 只使用当前知识快照中的可用事实、营销主张、素材和权利记录。
- 缺少正式输入时可以形成已阻断候选，但不得伪造依据。
- 分镜和 Seedance 只能使用通过检查的上游内容。
- Seedance 2.5 的服务端执行只能从 `MediaGenerationJob` 进入；手动 Seedance 上传和服务端执行的结果都必须重新经过技术与内容审核。
- 本地候选、提交内容版本、已批准快照和外部发布状态必须分别表达。

## 客户端支持

| 客户端 | 场景状态 |
| --- | --- |
| Codex | 可用，查看[完整教程](../guides/marketing-video/codex.md) |
| Claude Code | 底层能力有限可用，暂无完整场景教程 |
| Claude Desktop/Web、Cursor、VS Code GitHub Copilot | 上游协议候选，ContentCloud 场景尚未准入 |
| GitHub Copilot 其他 Surface、Kiro、Gemini CLI、Cline、Windsurf、Continue | 规划状态 |
| Hermes、OpenClaw、WorkBuddy、Grok Bot、NanoClaw | 规划或非首发 |
