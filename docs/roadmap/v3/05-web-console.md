# V3 Web 后台信息架构与展现

## 1. Web 的产品角色

Web 是项目控制台和人工治理界面，不是本地 Workspace 的网页文件浏览器。它回答五个问题：

1. 项目现在处于哪个服务阶段，下一步是谁做什么？
2. 客户资料和 15 维方法论覆盖是否足够？
3. 哪些事实、主张、素材和权利可用，哪些被阻断？
4. 本地生产了什么、提交了哪个版本、审核到哪里？
5. 已交付内容表现如何，哪些学习可以进入下一轮？

## 2. 界面基线与项目级导航

V3 不另起一套后台视觉。`docs/roadmap/v2/prototype.html` 是界面基线，保留以下特征：

- 深色左侧项目导航、顶部面包屑与搜索。
- 安静、紧凑、适合反复操作的工作台布局。
- 项目总览首屏包含项目实物图、Gate、风险、待审和下一动作。
- 业务按“开始、业务治理、自动化”分组，而不是一个页面堆满卡片。
- 列表、版本 diff、状态、证据披露和审核动作使用高密度表格/面板。

V3 导航在 V2 原型上增加“方法论与上下文”，其余业务层级保持一致：

```text
开始
  01 接入与初始化
  02 项目总览

业务治理
  M  方法论与上下文
  K  可信知识
  R  市场情报
  S  营销策略
  P  内容策划
  C  创意与剧本
  V  审核协作
  D  交付制作
  L  结果学习

自动化
  A  Automation 与运行
```

设置、团队、审计和环境高级信息放入全局菜单或相应业务页的次级入口，不拆散主业务流。普通用户不看到 Marketplace、MCP、SubmissionProjection 等实现术语。

## 3. 接入与初始化

该页面沿用 V2 原型的四步结构，但数据全部来自真实 BootstrapAttempt 和 WorkspaceBinding：

```text
云端创建项目
  -> 初始化本地 Workspace
  -> 建立客户上下文和知识候选
  -> 进入意图内容生产
```

页面展示：

- Codex Desktop、CLI、Node/npx 和 Keychain 前置检查。
- 当前 Bootstrap stage、唯一下一动作和支持码。
- 将安装的 Scene Plugin、能力、权限、网络和费用边界。
- Workspace V3 目录、Context、Schema 和 doctor 状态。
- 需要 Codex 专用配置时显示 `.codex/config.toml` 变更；ContentCloud 状态显示为 `.contentcloud/`，不得混称。
- Codex 是否打开正确根目录、用户是否信任该项目、有效 sandbox/permissions 是否允许原子写 `.contentcloud/`。

## 4. 项目总览

### 4.1 首屏信息

项目总览必须直接显示：

- 客户、品牌、产品和服务方案。
- 当前研发节点、Gate 状态和负责人。
- Workspace 是否已连接、环境健康和最近同步时间。
- 15 维覆盖：资料覆盖、已决覆盖、主要缺口。
- 七层知识包质量。
- 活动 Assignment、最近 Submission 和待处理决定。
- 当前最重要的 1 个下一动作，而不是泛化功能介绍。

金陵古都香 Fixture 的正确首屏示例：

```text
当前节点：立项
资料：20 / 20 已登记
方法论：15 / 15 有候选覆盖
正式可用：Fact 0、Claim 0、Rights 0
当前模式：仅允许 blocked creative exploration
主要待决：包装尺寸、当前价格、商标状态、素材权利、安全审核
下一动作：客户负责人确认当前签批包装尺寸及依据
```

### 4.2 快捷动作

- 初始化/修复本地 Workspace。
- 创建 WorkAssignment。
- 查看当前待决策项。
- 打开最近 Submission。
- 复制无秘密的 Codex 继续 Prompt。

## 5. 方法论与上下文

### 5.1 页面结构

页面分为四个 Tab：

| Tab | 展示 |
| --- | --- |
| 15 维诊断 | 客户、产品、供应链维度；来源、证据质量、已决覆盖、缺口和负责人 |
| 研发节点 | 立项、样品、收口、生产/质检的 Gate 和交付物 |
| 七层知识包 | identity/product/market/expression/operations/content_engine/compliance |
| 内容意图 | 渠道、目标、必需输入、输出 Schema、禁用项和指标 |

### 5.2 允许动作

- 调整项目负责人或截止时间。
- 基于现有模板创建 Context 修订请求。
- 创建补料 Assignment。
- 查看版本 diff 和影响范围。

Web 不直接编辑本地 `methodology.yaml` 或 `knowledge-pack.yaml`。任何改变都形成 ProjectContextVersion 或 WorkAssignment。

## 6. 可信知识

### 6.1 来源视图

来源列表显示真实 `Source` projection：

- 标题、来源类型、MIME、hash 缩写和版本。
- 原件位置是本地引用、云端托管还是仅 metadata。
- ingest 状态、Evidence 数量和最近变更。
- 来源披露级别和影响对象数量。

来源详情按“原件信息、提取结果、Evidence、关联对象、影响”组织。没有 `full_source` 权限时不展示伪预览。

### 6.2 知识治理视图

使用类型 Tab，而不是一个通用知识列表：

```text
事实 Fact
主张 Claim
素材 Asset
权利 Rights
冲突 Conflict
规则与其他 Domain Objects
```

每个对象显示：

- 当前本地候选版本或云端批准版本。
- 类型化状态和 eligible/blocked。
- source/evidence refs。
- 风险、适用渠道、有效期和影响对象。
- 本次 Revision diff、审核意见和决定依据。

### 6.3 审核动作

- Fact：verify / reject / request evidence。
- Claim：approve / prohibit / request changes。
- Rights：validate / expire / reject / request proof。
- Conflict：选择依据、保持冲突或请求补料。

按钮文案必须匹配对象类型，不能统一叫“批准知识”。

## 7. 市场情报

市场情报只展示已 publish 的 Research/Insight 业务对象及采纳决定：

- 公开来源或企业来源、快照时间和适用范围。
- Benchmark、观察到的模式和证据质量。
- `candidate / adopted / rejected / stale` 状态。
- “仅结构参考”与“可作为客户事实”的严格区别。

研究发生在本地 Agent 或 Automation，Web 不直接抓网页，也不把 Insight 自动写成 Fact。

## 8. 营销策略

展示 Audience、Scenario、DemandMoment、PainPoint、SellingPoint、已采纳 Insight 和 VisualizationPlan 的版本化组合。页面保留 V2 原型的三列摘要和可视化方案表，但只显示 Submission/Snapshot 投影，修改动作创建 Assignment 或策略修订任务。

## 9. 内容策划

展示 ContentPlan、Campaign、Experiment 和 Brief 的关系、版本 diff、输入 Snapshot、单变量约束和审核状态。Brief 正文在本地编译；Web 负责批注、批准或退回。

## 10. 创意与剧本

页面按生产层次组织：

```text
Plan / Campaign
  -> Brief
  -> ContentBatch
  -> Script / Copy
  -> Media candidates
  -> Video / Delivery candidates
```

### 10.1 列表信息

- Intent、渠道、批次目的和目标数量。
- 输入 Snapshot 和 KnowledgePack 版本。
- eligible/blocked 引用数量。
- 本地状态、Submission 状态和批准状态分列显示。
- 产物数量、lint、阻断原因和最近 Handoff。

### 10.2 blocked 内容

blocked 内容必须有独立视觉状态：

- 可以打开、比较、批注和创建补料任务。
- 不能进入交付、投放或自动生成正式素材。
- 页面直接列出缺少的 Fact、Claim、Rights 或安全决定。
- “解决阻断”进入对应知识对象或创建 Assignment，不提供绕过按钮。

## 11. 审核协作

### 11.1 审核队列

按业务优先级而不是数据表分组：

- 等待内部审核。
- 等待客户决定。
- 已退回待修订。
- 已批准待客户端 pull。
- 已过期或受上游影响待复核。

### 11.2 Revision 详情

必须同时展示：

- Submission 类型、版本、digest、提交者和环境 provenance。
- base ApprovedSnapshot。
- 对象级和字段级 diff。
- SourceDisclosure 和 Evidence 可见性。
- LocalRunSummary 的阶段与 checks，不显示 transcript。
- 评论、决定、subject 状态和下游影响。

### 11.3 客户审核链接

客户只看到本次 Revision 所需内容、证据和决定按钮，不看到内部工作区、其他客户、Plugin 或技术诊断。链接绑定 revision digest；新 revision 产生后旧链接失效。

## 12. 交付制作

页面包含 DeliveryPackage、批准快照清单、格式、接收人、验收条件、外部制作交接、缺失输入和成片回传。只有 ApprovedSnapshot 能进入交付包。

## 13. 结果学习

页面包含平台结果 ImportBatch、Observation、样本质量、candidate Learning、采纳决定和建议影响范围。

Learning 被采纳后只创建下一轮 Context/Brief 动作，不自动修改历史内容。

## 14. Automation 与运行

沿用 V2 原型的计划列表和 Run 详情，但严格区分本地对话与云端 Automation：

- 普通 LocalRun 不进入该列表，只在显式 publish 时显示摘要。
- AutomationPlan 显示 trigger、capability、Environment、最近 Run 和下一次执行。
- Run 详情显示 lease、Attempt、冻结输入、检查、Submission 输出和安全摘要。
- Generate 类型是否允许 schedule 由服务策略决定，首版默认只允许 remote run once。

## 15. 工作记录

Web 不显示 Codex 对话列表，而显示业务工作摘要：

- WorkspaceBinding 和设备健康。
- WorkAssignment 状态。
- 用户显式 publish 的 LocalRunSummary。
- Handoff 数量和状态摘要，不包含本机路径和正文。
- Automation TaskRun/Attempt。
- 审计事件和支持记录。

本地普通 Run 默认不上传，因此 Web 可以显示“本地有未提交工作”的同步摘要，但不能声称知道其中具体内容。

## 16. 设置与创作环境

普通用户看到业务表达：

- 当前支持的创作能力。
- 环境版本、健康、更新和权限。
- 已启用的可选能力、第三方账号和费用边界。
- 初始化、修复、升级、重置和支持码。

管理员可以进入高级区查看 Plugin/Skill/Provider Pack 的版本和签名状态。Marketplace 供给管理不放在项目主导航。

## 17. 角色视图

| 角色 | 默认首页 | 主要动作 |
| --- | --- | --- |
| 项目负责人 | 项目总览 | 看 Gate、分派任务、处理阻断、提交客户审核 |
| 创作者 | 内容生产 | 查看 Assignment、版本、反馈和交付条件 |
| 知识整理者 | 来源与知识 | 查看证据、候选、冲突和补料 |
| 内部审核人 | 审核与决定 | 对 Revision 做类型化决定 |
| 客户审核人 | 限定审核页 | 查看披露证据并批准/退回 |
| 管理员 | 设置与环境 | 模板、权限、环境和 Automation |

## 18. Demo 与空状态

V3 删除三个 TXT seed。开发演示使用版本化 Fixture，至少包含：

- 20 个真实类型 Source metadata。
- 15 个方法论维度和 4 个研发节点。
- 七层 KnowledgePack。
- Fact/Claim/Asset/Rights/Conflict 各至少两个状态。
- 一个 completed LocalRunSummary。
- 一个 blocked 的十条脚本批次。
- 一个待审核 Submission 和一个 ApprovedSnapshot 示例。

Fixture 必须来自脱敏、固定、可重复导入的数据包，不能在 HTTP handler 中逐条硬编码业务对象。

空状态只告诉用户当前缺的业务输入和唯一下一动作。例如：

```text
尚未连接本地 Workspace
[初始化本地创作环境]
```

不能用“探索更多功能”“体验 AI 创作”等营销文案代替工作状态。

## 19. 页面验收

1. 项目负责人不进入第二层页面就能知道当前节点、主要阻断和下一动作。
2. 来源页面能同时表达本地原件、云端披露和 Markdown 知识投影的区别。
3. 任一 Claim 都能点到 Evidence、Decision 和受影响内容。
4. 任一 Script 都能点到 Intent、Brief、Knowledge Snapshot、RunSummary 和 SubmissionRevision。
5. blocked 批次无法创建 DeliveryPackage。
6. Web 的所有业务修改动作都归结为 Assignment、Comment、Decision、Context Revision 或 Automation Plan，不直接改本地 projection。
7. 页面不显示虚假的本机路径、同步进度或 Codex 对话状态。
8. V3 实现与 `prototype.html` 建立逐页追踪；任何原型入口缺失都必须在 PLAN 中明确标为未实现，不能用底层 API 已完成代替页面完成。
