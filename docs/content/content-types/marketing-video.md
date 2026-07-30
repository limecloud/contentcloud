# 营销视频

状态：**可用**。

营销视频是 ContentCloud 当前正式支持的内容形态。系统覆盖可信知识、受众策略、Brief、营销视频剧本、分镜、Seedance 提示包、审核和交付治理。

## 生产链

```text
来源资料
  -> Evidence 与可信知识
  -> 受众与营销策略
  -> Brief
  -> ContentBatch 与营销视频剧本
  -> Web 审核与 ApprovedSnapshot
  -> 分镜与 Seedance 交付
```

## 当前 Skills

- Workspace：读取、恢复、交接、提交和打开受治理 Web 视图。
- Knowledge Extraction：从冻结的 EvidenceBundle 提取有依据的知识候选。
- Marketing Video Script：生成或修订带引用的营销内容。
- Douyin Audience Strategy：形成抖音电商受众策略。
- Storyboard Production：从已批准内容生成分镜生产对象。
- Seedance Export：生成受约束的 Seedance 交付包。

## 关键门禁

- 只使用当前知识快照中 eligible 的 Fact、Claim、Asset 和 Rights。
- 缺少正式输入时可以形成 blocked candidate，但不得伪造依据。
- Storyboard 和 Seedance 只能消费符合门禁的上游内容。
- 本地候选、SubmissionRevision、ApprovedSnapshot 和外部发布状态必须分别表达。

## 客户端支持

| 客户端 | 场景状态 |
| --- | --- |
| Codex | 可用，查看[完整教程](../guides/marketing-video/codex.md) |
| Claude Code | 底层能力有限可用，暂无完整场景教程 |
| WorkBuddy、Cursor、Hermes、OpenClaw | 即将支持 |
