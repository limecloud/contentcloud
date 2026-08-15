---
name: contentcloud-marketing-knowledge-query
description: 从营销工作区查询可追溯知识并明确区分可用、已阻断和仅供参考对象。用户询问产品事实、卖点、渠道限制、素材权利、冲突或内容输入时使用。
---

# 营销知识查询

查询结果必须可回到稳定 ID、证据和来源定位。详细边界见 [workspace-boundary.md](../../references/workspace-boundary.md)。

## 执行

1. 调用 `workspace_context`、`local_run_show`，确认 Run 已通过 `kb-lint` 且阶段为 `query`。
2. 根据渠道和时间范围调用 `knowledge_query`；不得从聊天历史、客户原文或模型常识补齐缺失字段。
3. 把结果分别记录为 `eligible_ids`、`blocked_ids` 和参考对象；对冲突、过期、缺权利和缺证据项写出原因。
4. 对确定性结论提供知识 ID、证据 ID、source locator、状态和适用渠道。
5. 调用 `local_run_record` 记录查询结果。`intent:query` 调用 `local_run_advance` 进入 `done`；`intent:content` 进入 `compile` 并交接 `$contentcloud-marketing-content-compile`。

## 输出门禁

- `verified` FactAssertion 才能作为确定事实。
- `approved` Claim 才能作为对外主张。
- `valid` RightsRecord 才能支持素材使用。
- 任一门禁不满足时输出 blocked 结果，不把候选改写成可发布输入。
