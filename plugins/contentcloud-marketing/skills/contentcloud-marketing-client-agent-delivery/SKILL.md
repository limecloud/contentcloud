---
name: contentcloud-marketing-client-agent-delivery
description: 编排客户品牌与产品营销 Agent 的诊断、知识包、意图内容、治理检查和交付报告。用户要求完成一次客户 Agent 建设、持续运营或交付复盘时使用。
---

# 客户营销 Agent 交付

交付的是可持续更新的 Workspace 和治理记录，不是一份脱离来源的长文档。详细边界见 [workspace-boundary.md](../../references/workspace-boundary.md)。

## 执行顺序

1. 调用 `workspace_context` 和 `workspace_project_brief`，确认客户、品牌、产品或服务、目标、渠道和受众。
2. 初始化 `local_run`，取得 Run Claim，记录输入来源和交付目标。
3. 交接 `$contentcloud-marketing-client-knowledge-pack`，完成素材诊断、知识包、冲突、缺口和方法论覆盖。
4. 交接 `$contentcloud-marketing-knowledge-ingest` 与 `$contentcloud-marketing-knowledge-lint`，只有 `kb-lint=passed` 才能继续。
5. 交接 `$contentcloud-marketing-knowledge-query` 和 `$contentcloud-marketing-intent-content`，分别记录可用/阻断知识及候选产物。
6. 对需要渠道格式的内容交接 `$contentcloud-marketing-content-compile`，再按渠道进入视频 `$contentcloud-marketing-video-script` 或文章 `$contentcloud-article-planning` 及其后续 Skill。
7. 生成交付报告，包含方法论覆盖、知识包版本、意图、产物引用、风险、客户决策项和后续维护建议。
8. 完成前运行所有确定性检查；需要云端写入时先调用 `publish_preflight`，只有用户明确确认准确 `plan_id` 后才调用 `publish_apply`；最后释放 Claim 或创建带摘要的跨对话 handoff。

## 失败与审核

- 任一阶段失败都保留当前 Run、finding 和输入摘要；修复后 resume。
- 缺事实、主张、权利或渠道能力时只输出 blocked 候选。
- 不自动批准知识、内容、权利或发布；云端写入必须经过 `publish_preflight`、准确 `plan_id` 和用户确认。
