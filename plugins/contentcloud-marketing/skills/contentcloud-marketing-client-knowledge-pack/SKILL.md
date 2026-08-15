---
name: contentcloud-marketing-client-knowledge-pack
description: 根据当前 Workspace 的客户资料和方法论构建可审阅的品牌与产品知识包，并输出覆盖诊断、来源披露、缺口和风险。用户要求建设或更新客户营销 Agent 知识包时使用。
---

# 客户营销知识包

知识包是候选综合层，不是事实源或批准快照。详细边界见 [workspace-boundary.md](../../references/workspace-boundary.md)。

## 执行

1. 调用 `workspace_context`，确认客户、品牌、产品或服务和项目目标；缺少项目简报时先调用 `workspace_project_brief`，要求用户确认。
2. 调用 `local_run_show` 确认当前 Run 和 Claim，再读取 Workspace 中已登记的来源、方法论映射、客户 profile、意图配置和当前知识状态；不要从 Plugin 包推断客户信息。
3. 调用 `knowledge_diagnose` 生成 15 维素材覆盖、冲突和缺口报告。
4. 依据当前合格来源调用 `knowledge_pack`，组织 identity、product、market、expression、operations、content_engine、compliance 七层候选。
5. 每个条目保留稳定 ID、来源引用、状态、用途和风险；把缺口写入 Workspace 的受管工作记录。
6. 运行 `knowledge_lint`，将 `changed_ids`、诊断路径、知识包路径和阻断项记录到当前 Run。
7. 调用 `local_run_record` 记录知识包和诊断输出，再将知识包交接 `$contentcloud-marketing-knowledge-query`；事实、主张、权利和发布状态仍由人工/云端治理决定。

## 保护边界

- 不把客户资料、客户名称、品牌素材或报价写进公共 Plugin。
- 不将 Synthesis 伪装成 `verified`、`approved` 或 `valid`。
- 不因资料缺失而使用模型常识补齐产品、价格、功效、历史或权利。
