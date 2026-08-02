# 执行计划

## 1. 目标

目标是交付一个可配置的 Content Work OS 基础：

1. 企业可以在环境后台维护自己的 SOP。
2. 普通用户可以从 Task 开始，不需要先理解底层领域对象。
3. 一个 Task 可以按绑定的 SOP 运行，并使用本地 Workspace、Codex/Claude Code CLI 或本地规则执行。
4. 复杂流程可以封装在 SOP 中，底层仍由 Revision、Evidence、Rights、Gate、Decision、AcceptedSnapshot 和 Delivery 托底。
5. 审批、客户确认和其他 Gate 按风险和业务需要配置，不是全局固定路径。
6. 后续增加新内容类型、新 CLI 执行器、新本地规则和新交付渠道时，不复制数据平面。

## 2. 问题判断

### 2.1 此前方案的偏差

旧方案并非治理能力错误，而是产品入口错误：

- 把 Project 和内容领域页面放在第一层，用户需要理解系统后才能开始工作。
- 把 Stage、Gate、Assignment 和交接当成主旅程，缺少一个简单的用户工作对象。
- 明确禁止通用 Task，导致参考样本中的任务优先体验无法进入产品模型。
- 把审批、审核和交接描述成固定路径，没有把企业 SOP 差异表达为配置。
- 试图用 Project Projection 直接承载用户操作和治理事实，增加页面复杂度。

### 2.2 方案修正

对象分为三层：

```text
用户层：Task、Inbox、My Tasks、Task Detail
方法层：SOP、SOP Version、Stage、Gate、Role、Automation Rule
事实层：Source、Evidence、Rights、SubmissionRevision、Decision、AcceptedSnapshot、Delivery、Observation
```

Task 只负责组织用户工作和执行状态；SOP 只负责定义流程；事实层负责证明发生了什么以及什么可以交付。

## 3. 范围边界

### 3.1 必须交付

| 区域 | 结果 |
| --- | --- |
| Environment | 环境配置、能力开关、SOP Registry、角色和审计边界 |
| SOP | 模板、Stage、输入/输出、Gate、执行方式、版本、发布和回滚 |
| Task | 创建、分派、状态、输入、输出、负责人、截止时间、优先级和下一动作 |
| Run | TaskRun、StageRun、执行尝试、claim、进度摘要和失败恢复 |
| Content | 视频脚本和文章的首条纵向链路 |
| Governance | Revision、Evidence、Rights、GateEvaluation、Decision、AcceptedSnapshot |
| Delivery | 交付包、接收方、格式和交付状态 |
| Admin | SOP Builder、Gate Policy、本地执行配置、权限、用量和审计 |
| Web | Task-first 工作区、Project Task/SOP 页面、任务详情和后台入口 |

### 3.2 暂不交付

- 通用外部项目管理协作套件。
- 自动登录、自动上传或自动发布第三方平台。
- 以聊天 transcript 作为可审计内容依据。
- 任意用户自定义脚本直接修改云端事实。
- 为每个新内容类型建立独立审批、交付或指标系统。

## 4. 目标对象模型

```text
Environment
  ├─ CapabilityRegistry
  ├─ SOPRegistry
  ├─ RolePolicy
  └─ Local Execution Registry (Workspace / CLI / Rules)

Project
  ├─ Context / Knowledge / Assets
  ├─ SOPBinding (固定 SOP Version)
  ├─ Tasks
  └─ Deliveries / Observations

Task
  ├─ TaskInputRefs
  ├─ TaskRun
  │   ├─ StageRun
  │   └─ ExecutionAttempt
  ├─ SubmissionRevision
  ├─ GateEvaluation / Decision (可选)
  ├─ AcceptedSnapshot
  └─ DeliveryPackage
```

对象的写入责任、生命周期和版本规则见 `01-architecture.md`；对外 API 和安全规则见 `02-contracts-and-security.md`。

## 5. 里程碑

### 契约冻结

目标：冻结 Task-first、SOP 可配置、Gate 可选和唯一事实源四项决策。

产物：

- Task/SOP/Stage/Gate/Run 数据契约；
- Environment、Tenant、Project 作用域规则；
- 低风险和高风险内容的两条示例 SOP；
- 主导航和管理员后台信息架构；
- 旧页面迁移和删除清单。

门禁：不能再出现“禁止通用 Task”或“审批固定必经”的产品规则。

### 基础设施与 Registry

目标：让环境可以配置、发布和校验 SOP。

工作项：

- `SOPDefinition`、`SOPVersion`、`StageDefinition`、`GateDefinition` 存储；
- SOP digest、版本、状态、发布者和生效时间；
- Environment Manifest 对 SOP、Capability、本地 Workspace、CLI 配置和规则的绑定；
- Schema 校验、不可关闭安全 Gate 和审计事件；
- Tenant/Environment/Project 的权限矩阵；
- 旧模板到新 SOP 的一次性迁移工具。

验收：两个环境可以使用不同 SOP，而不修改代码或数据库表结构。

### Task 与 Run

目标：从创建 Task 跑到一个可审查 Revision。

工作项：

- Task 创建、列表、分派、claim、取消、恢复和状态投影；
- TaskRun 绑定不可变 SOP Version；
- StageRun 的输入输出引用和唯一下一动作；
- LocalRun、CliRun、AutomationAttempt 和 ParallelRun 的统一执行摘要；
- 任务中断、claim 冲突、过期和失败恢复；
- 只上传显式 publish 的 Revision，不上传本地未提交正文。

验收：普通用户可以创建一个脚本或文章任务，并看到下一动作、执行摘要和待处理 Gate。

### 内容生产链

目标：跑通两种内容类型的真实工作流。

首批 SOP：

- 资料与知识建设：Brief、来源登记、Evidence/知识候选、知识快照。
- 短视频脚本：Brief、知识/证据、策略、脚本、检查、可选审核、交付。
- 文章：Brief、知识引用、写作、引用/权利检查、可选审核、交付。
- 活动结果复盘：结果导入、版本绑定、问题归因、改进建议、复盘交接。

平台为每个新租户预置以上四条通用模板。模板只提供可运行的基础结构，企业必须能够复制为自定义 SOP；旧版本只做精确识别和增量升级，不自动改绑 Environment、Project 或历史 Task。

验收：同一 Task 平台可以切换内容 Schema；不复制 Project、Review、Delivery 或指标系统。

### SOP Builder 与管理后台

目标：流程负责人不改代码也能管理 SOP。

工作项：

- 模板选择、空白 SOP、Stage 编排和版本草稿；
- 输入、输出、角色、能力、指标和失败路径配置；
- Gate 模式、条件、负责人、升级路径和是否阻断配置；
- 发布前 lint、影响分析、版本对比、发布和回滚；
- Workspace、CLI 配置、本地规则和用量视图；
- 权限、审计和敏感配置的后台管理。

### 主工作区

目标：把 Task-first 变成默认产品体验。

工作区一级入口：

```text
新建任务 / 输入收集 / 知识库 / 我的任务 / 所有任务 / 任务上下文
本地规则 / CLI 执行器 / 工作区节点 / 用量
```

Project 一级入口：

```text
任务 / SOP
```

知识库升级为一等工作面，覆盖类型化对象、来源与 Evidence、待审与冲突、知识包与不可变快照、确定性查询。旧审核、交付和学习页面保留为 Task 深链工作面，Task 纵向链路稳定后删除重复主导航。

### 试点与交接

目标：一个真实租户低成本使用，并能由客户团队继续运行。

产物：

- 试点 SOP 和环境配置；
- Task/Run/Revision/Gate 的完整审计；
- 使用、成本、周期、返工和交付指标；
- 客户可继续使用的 SOP、权限、验证样本和交接包；
- 旧 Project 页面和兼容 API 的删除清单。

## 6. 工程工作包

| 工作包 | 依赖 | 输出 |
| --- | --- | --- |
| 契约和不变量 | 无 | Schema、状态、权限、digest 规则 |
| Environment 与 SOP Registry | 契约和不变量 | 存储、API、审计、发布流程 |
| Task 与 TaskRun | 契约和不变量、Environment 与 SOP Registry | 创建、列表、claim、恢复、投影 |
| Stage 与 Gate 引擎 | Environment 与 SOP Registry、Task 与 TaskRun | 可选 Gate 和确定性状态计算 |
| Revision、Decision 与 Delivery | Task 与 TaskRun、Stage 与 Gate 引擎 | 内容治理纵向链路 |
| 视频与文章 Schema | Revision、Decision 与 Delivery | 两种内容类型 |
| Task-first Web | Task 与 TaskRun、Stage 与 Gate 引擎 | 工作区、Project Task、Task Detail |
| SOP Builder 与 Admin | Environment 与 SOP Registry、Stage 与 Gate 引擎 | 配置、版本、发布、审计 |
| 本地 Workspace、CLI 与 Rules | Task 与 TaskRun | 本地执行和用量 |
| 迁移和删除旧主路径 | Task-first Web、SOP Builder 与 Admin | 单一主流程、无双写 |
| 试点和交接 | Revision、Decision 与 Delivery、迁移和删除旧主路径 | 指标、Runbook、交接包 |

## 7. 执行原则

- 先契约和基础设施，再做视觉页面。
- 先跑通一个低风险 Task，再扩展高风险 Gate。
- 所有状态必须能回溯到事实或明确的投影输入。
- 所有环境都由 SOP 配置驱动，不能把流程写死在页面条件分支中。
- UI 可以简化复杂度，但不能隐藏阻断、版本、权利和证据事实。
- 删除旧主路径前先完成迁移和读模型对账，不保留长期兼容双写。

## 8. 风险和应对

| 风险 | 表现 | 应对 |
| --- | --- | --- |
| Task 变成第二套事实源 | Task 状态和 Revision/Run 状态不一致 | Task 只做工作流对象，正式事实仍由治理对象提供 |
| SOP 配置过度复杂 | 每个客户都需要工程介入 | 先提供 Stage、输入输出、角色、Gate、执行方式五类配置 |
| Gate 变成审批套件 | 低风险内容也被阻塞 | Gate 明确区分 advisory、required、decision 和 none |
| 环境配置漂移 | 同一 SOP 在不同环境行为不同 | Manifest 固定版本和 digest，TaskRun 记录绑定版本 |
| CLI 结果被误认为完成 | 运行结束但没有有效 Revision | 只有 publish、Gate 和 AcceptedSnapshot 才能推进正式状态 |
| 旧页面长期并存 | 用户不知道该从哪里开始 | 主工作区稳定后删除旧主导航，试点与交接阶段删除兼容路径 |

## 9. 完成定义

只有在以下条件同时满足时才算完成：

1. 两个环境可以独立配置和发布不同 SOP。
2. 普通用户可以在两次主要操作内创建 Task 并选择 Project/SOP。
3. TaskRun 固定 SOP Version，状态和下一动作可重建。
4. 至少一个低风险内容任务可以不经过人工审批完成交付。
5. 至少一个高风险任务可以启用内部审核或客户确认 Gate。
6. 每个交付结果都能追溯到 Revision、输入快照、Evidence/Rights 摘要和 SOP digest。
7. 失败、退回、claim 冲突、SOP 版本变更和能力关闭都有明确恢复路径。
8. 管理员可以在后台完成 SOP 配置、发布、回滚、权限和审计查看。
