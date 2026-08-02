# 业务工作流

## 1. 用户心智

普通用户只需要回答三件事：

1. 我要完成什么内容工作？
2. 这个工作属于哪个 Project，使用哪套 SOP？
3. 当前下一步是什么，需要谁处理？

用户不需要手动拼接 Revision、Evidence、Rights、Decision 和 Delivery；这些由 Task Run 根据 SOP 和底层事实自动组织。

“任务上下文”是结构化业务记录面，不是持续聊天面。页面只展示已经明确提交的输入补充、业务决定、运行摘要和交付事实；需要新增内容时必须选择具体业务动作，避免把每轮本地对话变成云端消息流。

## 2. 创建 Task

### 2.1 入口

Task 可以从以下入口创建：

- 工作区“新建任务”；
- 输入收集中的 Brief、资料或客户需求；
- Project 的“任务”页；
- 本地规则计划；
- Codex/Claude Code CLI 的受控触发；
- 已完成 Task 的“复制为新任务”。

### 2.2 创建步骤

```text
写一句工作目标
  -> 选择 Project
  -> 选择 SOP
  -> 确认输入资料和输出数量
  -> 选择负责人、截止时间和风险级别
  -> 创建 Task
  -> 展示第一条下一动作
```

创建向导不显示模型、Prompt、本机目录、Skill 路径或底层 API。高级配置只在 SOP Builder 和管理员后台出现。

### 2.3 Task 初始检查

创建前服务端检查：

- Project 可访问且未归档；
- Environment 健康且已发布所需 SOP；
- 内容能力已由租户显式启用；
- 必填输入存在且版本有效；
- Task 创建者有权限；
- 目标输出 Schema 和执行方式可用。

任何检查失败都返回可行动缺口，不创建半成品 Task。

## 3. 输入收集和任务分派

### 3.1 输入收集

输入收集保存尚未变成 Task 的本地输入：客户 Brief、Workspace 文件、评论、外部需求和本地规则触发事件。每条输入可以：

- 创建 Task；
- 合并到已有 Task；
- 归档为 Project 资料；
- 标记缺少信息；
- 转给流程负责人。

输入收集不直接改变知识、Revision 或 Project 状态。

### 3.2 分派

分派可以指定人员、角色或本地执行方式：

- 人员：编辑、策略、审核或项目负责人；
- Codex/Claude Code CLI：知识整理、策略分析、脚本草拟、检查；
- 本地规则：定时、批量、事件触发；
- 并行 Workspace：多个本地 CLI 会话按 Stage 协同。

分派结果写入 TaskRun/StageRun。执行资源只获得冻结输入和允许动作，不获得超出 Task 作用域的 Project 数据。

### 3.3 导入本地对话

导入是按需交接能力，不是默认同步能力。适合以下场景：

- 本地 CLI 已完成大量探索，需要把结论和少量关键片段交给其他成员继续；
- 本地运行失败，需要把脱敏后的上下文交给流程负责人诊断；
- 复盘 Task 需要证明某个选择如何形成，但不需要上传整段会话；
- 对话中出现潜在事实或知识线索，需要创建 Evidence 候选并重新验证来源。

不适合：

- 每一轮对话自动上报；
- 把 Web 变成 Codex 或 Claude Code 的云端聊天镜像；
- 用 Transcript 代替 Revision、Decision、Evidence 或业务讨论；
- 为了“以后可能有用”而批量保存完整会话。

操作流程：

```text
Task Detail 选择“导入本地对话”
  -> Web 创建 ConversationImport 请求
  -> 选择 Codex CLI / Claude Code CLI / Workspace Adapter
  -> 本地客户端展示可选会话
  -> 用户选择 Stage 摘要、特定轮次或完整 Transcript
  -> 客户端本地脱敏并预览
  -> 用户确认导出
  -> 客户端生成 ConversationBundle
  -> 服务端校验并绑定为 Task 输入或 Evidence 候选
```

不同客户端的职责：

| 客户端 | 本地输入格式 | 由客户端完成 | 统一输出 |
| --- | --- | --- | --- |
| Codex CLI | Codex 会话事件/JSONL | 会话发现、轮次重建、工具结果过滤、脱敏 | ConversationBundle |
| Claude Code CLI | Claude Code transcript/JSONL | 会话发现、内容块解析、工具结果过滤、脱敏 | ConversationBundle |
| Workspace Adapter | Workspace 插件或本地脚本定义 | 按 adapter contract 读取、选择、脱敏 | ConversationBundle |

Web 只展示 adapter 能力和连接状态，不出现本机路径、文件选择器或私有格式字段。若客户端未安装适配器，Web 只能提示去客户端安装或升级，不能自行尝试解析。

完整 Transcript 需要两次明确动作：Web 请求范围时选择完整导出，本地客户端预览后再次确认。任何一步取消都不上传内容；未完成请求到期后自动失效。

导入完成后默认只出现一条“已绑定对话输入”记录。用户必须通过独立动作把其中内容转为 Evidence 候选、补充 Brief 或创建修订任务，平台不能自动沉淀为正式知识。

## 4. 知识库业务工作流

### 4.1 来源登记与摄取

```text
登记 Source
  -> 记录 owner、披露范围、MIME 和 digest
  -> 本地解析并定位 Evidence
  -> 生成类型化候选对象和关系
  -> Schema / locator / relation lint
  -> 进入待审队列
```

本地 Workspace、受控文档、外部 URL 和 ConversationBundle 可以成为来源，但由各自客户端适配器负责解析。Web 只管理来源登记、摄取任务和结构化结果，不浏览任意本机文件。

摄取完成不是知识接受。系统必须显示生成了哪些 FactAssertion、Claim、Insight、RightsRecord、ConflictRecord 和 KnowledgeGap，以及每个对象的 Evidence 定位和影响范围。

### 4.2 状态审阅

流程负责人或被授权审核人从待审队列打开具体对象：

1. 检查对象类型、状态、适用范围和当前版本；
2. 打开 Source 元数据和 Evidence 定位；
3. 检查相关对象、冲突、Rights 和正在使用的 TaskRun；
4. 选择接受、拒绝、要求补证据或创建补料 Task；
5. 填写决定理由并提交状态转换；
6. 服务端生成新对象版本和 AuditEvent。

对象不能通过批量“全部通过”跨过自己的状态机。低风险、同来源且同 Schema 的候选可以批量审阅，但每个对象仍保留独立决定和 digest。

### 4.3 冲突与知识缺口

冲突必须同时展示双方对象版本、Evidence、时间和适用批次。解决动作包括选择一方、限定适用范围、接受风险或要求新来源；旧对象和 Evidence 不删除。

知识缺口必须包含：缺少什么、为何需要、阻断哪些 Claim/Stage/Task、负责人和建议来源。点击“创建补料任务”后，Task 与 KnowledgeGap 双向关联；补料完成后重新进入摄取和审阅，不直接关闭缺口。

### 4.4 知识包与快照

```text
选择业务用途
  -> 选择层级、对象和允许状态
  -> 运行冲突 / Rights / Evidence lint
  -> 影响分析
  -> 发布 KnowledgePack Version
  -> 生成不可变 KnowledgeSnapshot
  -> 新 TaskRun 显式绑定
```

新快照只影响显式选择它的新 TaskRun。历史 TaskRun、Revision 和 Delivery 始终保留原快照；Rights 撤销等安全事件通过影响分析产生阻断或修订 Task，不改写历史事实。

### 4.5 确定性查询

用户从知识库或 Task Detail 提交业务问题，并选择 Project/Pack/Snapshot 与允许状态。系统先执行类型化查询，返回 `eligible`、`blocked` 和 `gaps`，再可选生成解释摘要。

内容生产只能把 `eligible` 对象写入输入快照。`blocked` 必须进入检查结果，`gaps` 必须成为下一动作或补料 Task；查询本身不改变任何对象状态。

## 5. 内容生产主流程

### 5.1 低风险内部内容

```text
Task 创建
  -> Brief Stage
  -> Knowledge/Evidence Stage
  -> Strategy Stage
  -> Draft Stage
  -> Schema/Claim/Rights Check
  -> AcceptedSnapshot
  -> Delivery
```

该流程可以不配置人工审批。检查通过后由系统生成 AcceptedSnapshot，交付对象仍然只能引用有效快照。

### 5.2 高风险外部内容

```text
Task 创建
  -> Brief
  -> Knowledge/Evidence
  -> Strategy
  -> Draft
  -> Schema/Claim/Rights Check
  -> Internal Review (required)
  -> Client Decision (optional or required by SOP)
  -> AcceptedSnapshot
  -> Delivery
```

人工 Gate 只在对应 SOP 中启用。退回不会修改旧 Revision，而是生成 changes_requested 结果和新的修订 TaskRun。

## 6. 短视频脚本 SOP 示例

### Stage 1：需求 Brief

输入：客户需求、渠道、受众、输出数量、截止时间。

输出：Brief Revision，包含目标、边界、禁用项和验收条件。

默认 Gate：`required_check`，缺少目标或渠道时阻断。

### Stage 2：知识与证据

输入：Project Knowledge、Source、Evidence、Asset。

输出：可用知识快照和引用清单。

默认 Gate：`evidence_confirmation`；低风险内容可为 advisory，高风险主张必须阻断。

### Stage 3：策略

输入：Brief、知识快照、受众和渠道规则。

输出：角度、钩子、实验假设和内容约束。

默认 Gate：`none`。

### Stage 4：脚本创作

输入：Brief、策略、知识引用、可用素材。

输出：ContentBatch/Script Revision。

默认 Gate：Schema、Claim、Rights 检查。

### Stage 5：审核与交付

输入：当前 Revision 和所有检查结果。

输出：AcceptedSnapshot、DeliveryPackage。

默认 Gate：按 Project 风险策略配置，可为 none、internal_review 或 client_decision。

## 7. 文章 SOP 示例

```text
需求收集
  -> ArticleBrief
  -> 知识和引用
  -> 文章草稿
  -> 引用/权利/敏感表述检查
  -> 可选审核
  -> AcceptedSnapshot
  -> 交付或外部发布登记
```

文章和脚本使用同一 Task/Run/Gate/Delivery 引擎，只替换输入输出 Schema 和内容检查器。公众号文章能力仍由租户显式开启，SOP 存在不代表能力自动可用。

## 8. Task 详情工作流

Task Detail 的操作顺序固定为：

1. 查看业务目标、输出要求和当前状态；
2. 查看 SOP、Stage、负责人和下一动作；
3. 查看输入资料、Knowledge、Evidence 和 Rights 摘要；
4. 选择执行方式或继续当前 Run；
5. 需要交接或诊断时，请求本地客户端导出选择性上下文；
6. 查看检查结果、阻断和影响范围；
7. 提交 Revision 或处理 Gate；
8. 查看 AcceptedSnapshot、Delivery 和结果观察。

页面可以把多个底层事实组合成一个任务工作面，但标签必须明确：

- “本地执行摘要”不等于“内容已提交”；
- “已导入对话”不等于“已生成知识或 Revision”；
- “已批准”不等于“已交付”；
- “已交付”不等于“外部平台已发布”；
- “已发布”不等于“已有有效结果观察”。

## 9. SOP 设计工作流

### 9.1 选择模板

流程负责人进入 Project 的 SOP 页，看到：

- 当前 Project 是否已有 SOP；
- 可用模板和 Stage 数量；
- 适用内容类型和渠道；
- 所需 Capability；
- Gate 摘要；
- 模板版本和 digest；
- 最近验证结果。

模板只是起点。选择模板后必须生成 Project 可编辑的 SOP Draft，不直接修改模板或历史版本。

### 9.2 编辑 SOP

编辑顺序：

1. 设定流程名称、目标和适用内容类型；
2. 添加、删除、排序 Stage；
3. 配置每个 Stage 的输入、输出、角色和执行方式；
4. 配置检查器、Gate、阻断和升级路径；
5. 配置指标、自动化触发和失败恢复；
6. 运行 lint、影响分析和样例任务；
7. 创建版本并发布到 Environment。

### 9.3 发布

发布前必须检查：

- 所有输入输出 Schema 可解析；
- 所有角色、Capability、本地 CLI 配置和规则已注册；
- Gate 组合不会形成不可达路径；
- 至少有一个成功样例和一个失败样例；
- 权利、证据和安全硬门禁未被关闭；
- 已有 Project/Task 的影响范围明确。

## 10. 异常和恢复

### 缺少输入

```text
Stage blocked
  -> 生成缺口清单
  -> 创建补料 Task 或回到 Inbox
  -> 新输入进入 Project Context
  -> 新 TaskRun 继续
```

### Revision 被退回

```text
changes_requested
  -> 评论绑定旧 Revision
  -> 创建修订 TaskRun
  -> 本地修订
  -> 新 Revision
  -> 重新运行 Gate
```

### 权利失效

```text
Rights invalid
  -> 影响分析
  -> 阻断引用该素材的 Stage/Task
  -> 替换素材或补权利
  -> 新 Revision
```

### SOP 版本退休

已运行的 TaskRun 保留原版本并可以完成或取消；新 Task 必须选择新发布版本。服务端不自动改写历史 Run。

### Capability 关闭

停止创建依赖该能力的新 TaskRun，保留历史结果，并显示受影响 Project、Task、Stage 和恢复入口。

## 11. 角色默认工作面

| 角色 | 默认入口 | 主要动作 |
| --- | --- | --- |
| 普通成员 | 我的任务 | 领取、执行、提交、处理反馈 |
| 流程负责人 | Project SOP | 设计 Stage、Gate、角色、指标和升级路径 |
| 项目负责人 | 所有任务 | 调整范围、分派、处理阻断和交付 |
| 内容编辑 | Task Detail | 读取输入、创作候选、运行检查、提交 Revision |
| 知识负责人 | 知识库 | 登记来源、审阅对象、解决冲突、发布知识包和快照 |
| 审核人 | 输入收集/待审核 | 查看 Revision、证据和权利，提交可选决定 |
| 客户决定人 | 受邀 Task/Review | 只查看被授权版本，批准或提出修改 |
| 管理员 | 管理后台 | 管理 Environment、SOP、Capability、本地 CLI、ClientAdapter、规则、权限和审计 |

## 12. 业务完成定义

Task 不能仅因为执行进程退出而完成。至少满足：

- 输出符合 SOP Schema；
- 必需检查通过；
- 必需 Gate 已完成；
- Revision digest 和输入引用有效；
- Rights/Evidence 硬门禁通过；
- AcceptedSnapshot 已生成或业务负责人明确记录本轮无交付；
- 需要交付时 DeliveryPackage 已生成并绑定接收方。
