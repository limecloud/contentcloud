# 试点、指标与交接

## 1. 试点原则

试点不是演示一套页面，而是验证一条可复制的内容工作方式：

- 企业能否自己配置 SOP；
- 普通用户能否低成本创建并完成 Task；
- 基础设施能否承受不同执行方式；
- 治理门禁是否只阻断真正的风险；
- 客户团队能否在没有交付人员持续代操作的情况下继续运行。

## 2. 首批试点范围

### 2.1 业务边界

- 一个租户；
- 两个 Environment：试验环境和稳定环境；
- 一个 Project；
- 两类内容：短视频脚本、文章；
- 两个 SOP Version：低风险和高风险；
- 3-6 名真实参与者；
- 一个主要渠道和一个交付格式。

### 2.2 参与角色

| 角色 | 人数 | 责任 |
| --- | --- | --- |
| 业务负责人 | 1 | 目标、范围、结果和最终业务确认 |
| 流程负责人 | 1 | SOP、Stage、Gate、升级和交接 |
| 内容编辑 | 1-3 | Task 执行、修订和提交 |
| 审核人 | 1 | 需要时处理内部 Gate |
| 客户决定人 | 0-1 | 高风险内容的可选客户 Gate |
| 平台管理员 | 1 | Environment、Capability、权限、审计和成本 |

## 3. 试点节奏

### 第 0 周：基础配置

产物：

- Environment Manifest 和能力开关；
- 租户 SOP 草稿；
- Project 上下文、知识和初始输入；
- 角色和权限；
- 低风险与高风险样例 Task。

验收：流程负责人可以在后台完成 SOP 草稿，不需要工程师修改代码。

### 第 1 周：跑通低风险 Task

目标：完成至少 3 个不需要人工审批的 Task。

观测：

- 创建 Task 用时；
- 从 Task 到第一条有效 Revision 的时间；
- 输入缺口数量；
- 检查失败和重试次数；
- AcceptedSnapshot 生成率。

### 第 2 周：跑通高风险 Gate

目标：完成至少 2 个启用内部审核或客户确认的 Task。

观测：

- Gate 等待时间；
- 退回原因和修订次数；
- 证据/权利阻断率；
- 旧 Revision 是否保持可追溯；
- 新 Revision 是否正确绑定最新 digest。

### 第 3 周：执行方式和批量

目标：至少使用两种执行方式：本地 Workspace + Agent 或 Automation。

观测：

- 每种执行方式的成功率、耗时和成本；
- claim 冲突、lease 过期和恢复次数；
- 批量 Task 的排队时间；
- 运行失败是否能转为补料或人工处理。

### 第 4 周：交付和复制

目标：客户团队独立创建新 Task、调整 SOP 草稿并完成一次交接。

产物：

- 新 SOP Version；
- 交付包和内容版本清单；
- 指标和成本报告；
- 未解决问题和下一轮计划；
- 客户运行手册。

## 4. 指标体系

### 4.1 激活指标

- 从登录到创建第一个 Task 的时间；
- 从 Task 创建到选择 SOP 的完成率；
- 从初始化 Environment 到首个有效 Revision 的时间；
- 流程负责人独立创建 SOP Draft 的比例。

### 4.2 业务效率指标

- Task 从 ready 到 accepted 的中位周期；
- 首次 Revision 通过率；
- 每个 Task 的修订次数；
- 输入缺口导致的等待时间；
- Gate 等待时间占比；
- 交付准时率。

### 4.3 质量和治理指标

- Evidence/Rights 硬门禁发现率；
- 无依据主张进入 Revision 的比例；
- Gate 误阻断率和漏阻断率；
- Revision、Decision、Snapshot、Delivery 的绑定完整率；
- 未 publish 本地内容意外进入 Web 的次数，目标为零；
- 旧版本被错误覆盖的次数，目标为零。

### 4.4 基础设施指标

- Task Projection P95 响应时间；
- Task 创建成功率；
- claim 冲突恢复时间；
- Automation/Agent Attempt 成功率；
- Environment/SOP digest 校验失败数；
- 事件和审计延迟；
- 单个 Task 的模型、运行和存储成本。

### 4.5 复制指标

- 新 Project 绑定已发布 SOP 的时间；
- 新 Environment 配置所需人工介入次数；
- 同一 SOP 复用到不同 Project 的成功率；
- 客户团队独立完成 Task 的比例；
- 从一个 SOP Version 复制出新版本的时间。

## 5. 事件记录

至少记录以下事件：

```text
task.created
task.claimed
task.started
task.waiting_input
task.stage_completed
task.revision_published
task.gate_evaluated
task.decision_recorded
task.accepted
task.delivered
sop.draft_created
sop.version_published
sop.version_retired
execution.attempt_started
execution.attempt_failed
capability.disabled
```

每个事件包含租户、Environment、Project、Task、SOP digest、actor、request ID、时间和关联事实 ID。事件用于指标和审计，不替代正式业务事实。

## 6. 交接包

交接包必须能让客户团队独立继续，至少包含：

### 6.1 SOP 和配置

- 已发布 SOP Version、digest 和适用 Project；
- Stage、输入输出、角色和 Gate 说明；
- 低风险/高风险流程差异；
- 自动化、Agent 和 Swarm 的能力清单；
- Environment 配置和能力开关。

### 6.2 内容和治理

- AcceptedSnapshot、DeliveryPackage 和 Revision digest；
- Evidence、Rights、检查器和样例；
- 已处理和未处理的 blocker；
- Gate、Decision 和审计记录；
- 客户可见和不可见内容边界。

### 6.3 运行手册

- 创建 Task、选 SOP、补料、执行、提交和交付；
- TaskRun 中断、claim 冲突、能力关闭和版本退休的恢复；
- SOP 草稿、样例运行、lint、发布和回滚；
- 权限、成员和用量管理；
- 支持码和升级路径。

### 6.4 下一轮

- 真实结果观察和数据质量；
- 下一轮应该保留、删除或调整的 Stage；
- Gate 误阻断和漏阻断清单；
- 新内容类型、新渠道和新 Agent 的候选；
- 明确不做的范围。

## 7. 试点退出条件

满足以下任一情况时暂停扩展，而不是继续增加功能：

- 用户仍需要先学习领域页面才能创建 Task；
- 流程负责人不能独立修改 SOP；
- 低风险 Task 大量被固定审批阻塞；
- Revision、Gate 或 Delivery 事实无法完整追溯；
- 客户团队无法独立完成第二轮 Task；
- 运行成本或失败恢复不可解释。

## 8. 试点成功标准

首批试点至少达到：

1. 80% 的普通用户可以在 5 分钟内创建第一个 Task。
2. 低风险 Task 中至少 60% 不需要人工审批即可完成 AcceptedSnapshot。
3. 高风险 Task 的每个退回都能绑定到具体 Revision、Gate 和原因。
4. 流程负责人可以在一个工作日内创建并发布一个 SOP 小版本。
5. 客户团队可以独立完成至少一轮新 Task 和一次交付。
6. 未 publish 内容泄露、跨租户访问和历史 Revision 覆盖均为零。
