---
name: contentcloud-marketing-content-compile
description: 根据已查询的合格知识和客户意图编排可审阅的营销内容候选，并交接视频、文章或其他内容形态 Skill。用户要求生成营销脚本、文章、商品卡、直播话术或渠道交付包时使用。
---

# 营销内容编排

负责跨渠道编排，不拥有视频镜头、文章区块或外部平台私有格式。详细边界见 [workspace-boundary.md](../../references/workspace-boundary.md)。

## 执行

1. 调用 `workspace_context` 后确认 `local_run_show` 阶段为 `compile`，并读取已记录的 `eligible_ids`、`blocked_ids`、客户意图和渠道。
2. 对输入调用 `brief_lint`；缺少合格知识、权利、渠道规则或人工选择时输出 blocked 候选和补料清单。
3. 根据渠道交接相应形态 Plugin 和入口 Skill：视频使用 `contentcloud-video-production` 的 `$contentcloud-marketing-video-script`，文章使用 `contentcloud-wechat-article` 的 `$contentcloud-article-planning`；后续再按需要交接视觉、长文或交付 Skill，不要在本 Skill 中伪造另一种内容 Schema。
4. 将每个候选的知识、主张、资产、权利、实验变量和阻断原因写入 Workspace 相对输出路径。
5. 调用 `content_batch_lint`，失败时记录 `content-lint=failed` 并停在 `output-lint`；通过后调用 `content_batch_finalize`。
6. 调用 `local_run_record` 记录 `output_refs`，再调用 `local_run_check` 记录 `content-lint=passed`，最后推进 Run。
7. 需要云端审核时先调用 `publish_preflight`，展示准确 `plan_id` 和披露范围；只有用户明确确认同一 `plan_id` 后才调用 `publish_apply`。

## 禁止

- 不自动批准、发布、登录渠道或上传外部平台。
- 不把营销创意、历史内容或客户评论当成事实。
- 不把 `CreativeDraft`、`review_ready` 或预检结果称为 `published`。
