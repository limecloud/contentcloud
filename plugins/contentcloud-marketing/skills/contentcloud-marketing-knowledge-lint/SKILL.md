---
name: contentcloud-marketing-knowledge-lint
description: 对营销工作区的来源、知识、权利、索引和状态运行确定性检查，并按 Run 门禁交接查询。用户要求检查知识库、定位断链、恢复失败流水线或确认内容输入时使用。
---

# 营销知识校验

只报告确定性结果，不用模型判断替代审核。详细边界见 [workspace-boundary.md](../../references/workspace-boundary.md)。

## 执行

1. 调用 `workspace_context` 和 `local_run_show`，确认当前 Run 阶段是 `knowledge-lint`，并取得有效 Claim。
2. 调用 `source_verify`，确认来源摘要、路径和 MIME 没有漂移。
3. 调用 `knowledge_lint`，保留完整报告；失败时调用 `local_run_check` 写入 `kb-lint=failed` 和 finding，停止交接。
4. 成功时调用 `knowledge_diagnose` 生成素材覆盖诊断；需要交付知识包时调用 `knowledge_pack`，其状态仍保持候选，直到人工审核。
5. 调用 `local_run_check` 写入 `kb-lint=passed`，再调用 `local_run_advance` 进入 `query`；若本次只完成知识摄取，则按当前 Run 的允许转换进入 `done`。
6. 将错误分为结构错误、来源错误、状态门禁和需要人工决策的语义风险。

## 禁止

- 不删除孤立页、冲突页或原始来源。
- 不修复业务结论，不提升事实、主张或权利状态。
- 未通过 `kb-lint` 时不交接查询或内容编译。
