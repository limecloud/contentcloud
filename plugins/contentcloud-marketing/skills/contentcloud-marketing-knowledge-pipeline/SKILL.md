---
name: contentcloud-marketing-knowledge-pipeline
description: 编排营销工作区的来源摄取、知识治理、可用性查询和内容交接。用户要求从客户资料形成可追溯营销知识、更新知识后继续内容生产或恢复中断流程时使用。
---

# 营销知识内容流水线

按一个可恢复的本地 Run 串联营销知识与内容阶段。详细边界见 [workspace-boundary.md](../../references/workspace-boundary.md)。

## 执行

1. 调用 `workspace_context`、`workspace_status`，确认当前 Workspace Root、项目和模板。
2. 根据用户目标调用 `local_run_init`，`intent` 必须使用稳定 ID：`intent:ingest`、`intent:query` 或 `intent:content`；有新来源时设置 `with_ingest=true`。
3. 调用 `local_run_claim` 取得单写入者占用。记录 Run ID、Claim Token 和 Context Revision，不把 Token 写入输出文件。
4. 有新来源时交接 `$contentcloud-marketing-knowledge-ingest`；否则从 `$contentcloud-marketing-knowledge-lint` 开始。
5. 知识校验通过后交接 `$contentcloud-marketing-knowledge-query`，要求分别记录 `eligible_ids` 和 `blocked_ids`。
6. `intent:content` 时交接 `$contentcloud-marketing-content-compile`；`intent:query` 时记录结果后结束。
7. 每次阶段交接都调用 `local_run_record` 记录输入、eligible/blocked、finding 或 output refs，调用 `local_run_check` 写入确定性检查，再以 `local_run_advance` 推进；随后调用 `local_run_show` 确认前一阶段的检查、输入摘要和输出引用存在。
8. 失败时调用 `local_run_fail`，保留 finding；修复后调用 `local_run_resume`，不得新建 Run 隐藏失败历史。
9. 完成后调用 `local_run_release` 或创建准备好的 `handoff_create_ready`，向用户报告 Run、产物、阻断项和下一步。

## 禁止

- 不把客户资料、品牌名称或客户 Skill 写入 Plugin 包。
- 不在 Skill 中维护 Node/Ruby RunContext 或第二套状态机。
- 不把候选知识、内容草稿或预检结果称为已批准、已发布。
