---
name: contentcloud-marketing-intent-content
description: 根据当前 Workspace 的客户知识包和渠道意图生成可追溯的营销内容候选，并处理缺少事实、权利或渠道输入的阻断。用户要求按客户意图生成内容时使用。
---

# 客户意图内容

意图配置来自当前 Workspace；Skill 只编排，不改变知识状态。详细边界见 [workspace-boundary.md](../../references/workspace-boundary.md)。

## 执行

1. 读取 `workspace_context`、`local_run_show`、客户 profile、知识包版本、意图配置、目标渠道和当前 Run。
2. 校验意图的目标、必需输入、输出 Schema、禁用表达和指标；缺少输入时先输出结构化阻断清单。
3. 调用 `knowledge_query` 获取 `eligible_ids`、`blocked_ids` 和参考对象；只使用当前快照允许的知识、主张和权利。
4. 生成标题、角度、结构或内容候选时保持一个主要目标和一个主要实验变量；把模型假设标记为候选。
5. 对事实不足或权利不足的请求生成 `CreativeDraft`，设置 `publishable=false`、`status=blocked`、`blocked_reasons`、`candidate_refs` 和 `missing_inputs`，并调用 `local_run_record` 记录阻断原因。
6. 合格请求交接 `$contentcloud-marketing-content-compile`，由视频、文章等形态 Skill 完成类型化编译和确定性校验。
7. 输出来源、风险、阻断项和建议指标；不得自动投放、发布或改写客户的批准状态。
