# V1 到 V2 增量迁移与交付计划

## 1. 交付原则

V2 不推倒 V1，但会把普通创作从云端 TaskRun 迁到本地工作区。每个波次都必须形成可运行、可回滚、可独立 UAT 的纵向能力，并保持 V1 已有 TaskRun、ScriptVersion、审批和导出历史可读。

```text
先扩 Schema 和读模型
-> 双读/兼容写入
-> 回填与验证
-> 新业务入口启用
-> 观察
-> 退役兼容路径
```

不允许一次迁移同时替换领域模型、协议、客户端和页面而没有兼容期。

## 2. 当前基线

### 继承

- Go 控制面、CLI/Daemon、React Web、PostgreSQL、S3、OpenAPI/JSON Schema。
- tenant、membership、project、device、connect session。
- SourceRevision、EvidenceSpan、KnowledgeItem、Conflict、Rights。
- SellingPoint、VisualizationPlan、BriefVersion、ScriptVersion。
- TaskRun、RunAttempt、Artifact、租约、heartbeat、cancel、usage 和 safe summary。
- ReviewCycle、Comment、Approval、Grant、导出、Performance、Lineage 和 Audit。

### 已知缺口

- Client/Brand/Product 分层和四层上下文继承。
- 市场研究、内容计划、创意方向/批次、交付交接的正式聚合。
- ScriptPackage V2 和完整剧本工作台。
- 本地 init、WorkspaceTemplate、Skills/MCP、Submission 和 publish/pull。
- Automation Plan、schedule/event trigger、PlanChangeRequest、RunOutput。
- 通用 Run 详情和九域导航。

## 3. 波次一：本地工作区、知识发布与剧本工程

### 目标

把金陵古都香演进为可复用 V2 本地模板，走通 Web 建项目、init、知识 publish/审批/pull、剧本 publish/审批和交付；不要求把全部原始资料和草稿迁入云端。

### 领域与数据

- 增加 ClientAccount、Brand、Product 和项目引用。
- 增加 WorkspaceBinding、WorkspaceTemplateManifest/Lock、Submission/Revision、SourceDisclosure、DecisionDelta 和 ApprovedSnapshot。
- 增加 Methodology、TenantServiceTemplate、BrandKnowledgePack 和 ProjectContextSnapshot。
- 增加 ContentPlan、Campaign、ExperimentPlan、CreativeDirection 和 CreativeBatch。
- 升级 ScriptPackage 2.0；现有 1.x 保持只读和导出兼容。
- LocalRunContext 留在本地；Automation RunOutput 延后到波次三。

### 产品与客户端

- 项目创建扩展为客户/品牌/产品/模板和一次性 init code。
- 实现 `contentcloud init`、workspace doctor/upgrade、项目级 Skills/MCP。
- 实现本地素材诊断、七层知识包、LocalRunContext 和 publish/pull。
- 实现本地候选比较和逐镜头 lint；云端只实现 Submission 审阅和结构化 diff。
- 普通生成不需要 capability；`script.generate@2.x` capability 留给波次三 Automation。
- 客户审批和三格式导出使用 ScriptPackage V2。

### 迁移

1. 将 `jinling-gudu` 映射为新版 WorkspaceTemplate 和初始客户工作区。
2. 先生成 dry-run 报告：数量、重复 ID、无法映射状态、缺失 locator 和 hash。
3. 在本地迁移候选状态，不自动提升 verified/approved/valid，也不默认上传 raw。
4. 将首批十条脚本保留为本地 CreativeBatch；原 blocked 状态和原因不变。
5. 分批 publish Knowledge/Script Submission，由真实审核员决定后生成 ApprovedSnapshot。

### 波次验收

- 金陵古都香 Golden Journey 通过。
- V1 现有 TaskRun、ScriptPackage、审批历史和导出不回归。
- 普通本地 ingest/generate/revise 全程不创建云端 TaskRun。
- 客户审批固定 hash；修改上游后新稿必须重审。

## 4. 波次二：九域业务工作台

### 目标

把九域从文档模型变成真实工作台，并用第二个传统行业客户验证可复制性。

### 实施

- 项目总览重构为 Gate、Workspace 状态、Submission、阻断、负责人和交付状态。
- 上线 ResearchTask、BenchmarkCase、MarketInsight 和情报采纳。
- 上线 StrategyVersion、ContentPlan 和跨域 lineage。
- 上线 DeliveryPackage、ProductionHandoff 和外部制作状态。
- 完成九域导航、全局待办、风险、审批和项目组合视图。
- 补齐各域 publish/pull、BFF、权限、审计和云端治理页面，不建设在线正文编辑器。

### 第二客户标准

- 行业与金陵古都香不同。
- 复用同一平台方法论和租户模板。
- 有独立知识包、品牌规范、权利和客户审批人。
- 至少完成来源、策略、Brief、剧本和审批，不要求正式投放。

### 波次验收

- 九域每域至少一个真实业务对象、状态流、责任人和主用例。
- 两客户之间不存在搜索、列表、对象存储、任务和审批泄漏。
- 市场案例结构不能被误用为品牌事实依据。

## 5. 波次三：持续自动化与运营学习

### 目标

增加受控持续执行，而不把 ContentCloud 变成 Loop 产品或服务端 Agent 平台。

### 实施

- Automation Plan/Version、模板目录、remote/event/schedule 和 closed completion policy。
- Automation 工作区授权、隔离执行和本地来源 hash 校验。
- Scheduler、调度游标、离线策略、连续失败自动暂停。
- 通用 Run 详情、Attempt、heartbeat、Output 和安全执行摘要。
- PlanChangeRequest 和 ImprovementProposal 人工确认流程。
- 站内/邮件通知策略、周期复盘和候选 Learning。
- 最后实施安全投影扩展和 Hosted Preview。

### 波次验收

- monitor、generate、review 三种类型完成端到端验证。
- schedule 不能用于正式生成、审批和交付模板。
- late report、租约过期、重复触发和通知失败不重复创建业务产物。
- Hosted Preview 失败可降级且不影响审批。

## 6. 数据迁移策略

### Expand

- 新表、新 nullable 外键、新索引和新 Schema 版本先上线。
- 旧服务继续运行，新代码对旧数据有默认映射。

### Backfill

- 后台批次按 tenant/project 处理，记录 checkpoint、数量、错误和不可映射项。
- Client/Brand/Product 从现有项目字段回填，无法唯一判断时进入人工映射清单。
- 旧 Brief 创建默认 ContentPlan/Campaign；旧 script Run 创建 one-off CreativeBatch。

### Verify

- 比较对象数量、hash、审批引用、导出内容和 lineage 完整性。
- 随机抽样和金陵古都香全量逐项核对。

### Contract

- 新入口稳定并完成观察后，停止旧写路径。
- 兼容读路径至少保留一个稳定版本周期；删除前提供使用统计和退役通知。

## 7. API 与客户端发布顺序

1. 服务端先支持新旧 Schema，Feature Flag 默认关闭。
2. 发布 CLI/Daemon，支持 capability 2.x 和旧任务。
3. 灰度启用内部租户，验证 lease、output 和业务导入。
4. 启用金陵古都香项目，再启用第二客户。
5. Web 新入口开启；旧入口保留可回退链接。
6. 观察稳定后停止创建旧 Schema 任务。

服务端不得向不兼容客户端投递 2.x 任务。客户端升级失败不能阻断 Web 人工工作流。

## 8. Feature Flags

| Flag | 波次 | 回退行为 |
| --- | --- | --- |
| `v2_client_context` | 1 | 使用现有项目字段和 V1 快照 |
| `script_package_v2` | 1 | 继续生成/读取 1.x |
| `creative_batches` | 1 | 使用单次 script run |
| `nine_domain_navigation` | 2 | 返回 V1 项目导航 |
| `research_workspace` | 2 | 仅展示既有框架数据 |
| `automation_plans` | 3 | 保留人工 run once |
| `hosted_preview` | 3/P3 | 原生视图/安全预览/本机打开 |

Feature Flag 只切入口和行为，不允许形成两个并行事实源。

## 9. 团队与工作流

建议最小团队：产品/业务负责人、技术负责人、Go 后端、Web 前端、CLI/Daemon 工程师、QA/安全、南京试点业务代表。剧本 Schema 和业务门禁必须由内容策略/编导/审核员共同验收。

每个波次按以下节奏：契约冻结 -> migration dry-run -> 纵向实现 -> 自动化验证 -> 内部 dogfood -> 试点 UAT -> 灰度 -> 观察 -> 完成度矩阵更新。

## 10. Definition of Done

- 需求、领域、CLI/OpenAPI/Schema、Web 和审计语义一致。
- 正常、blocked、权限、离线、超时、重试和影响路径有自动化测试。
- 数据迁移有 dry-run、真实 PostgreSQL 证据、核对报告和回退步骤。
- 没有服务端 LLM/Agent 依赖，没有程序化直连私有 API 的新入口。
- 文档、原型和实现状态同步更新。
- 客户业务门禁由有责任的试点人员签署，不由开发者代签。

## 11. 风险登记

| 风险 | 影响 | 控制 |
| --- | --- | --- |
| 九域同时扩展导致失焦 | 延迟核心交付 | 三波次，第一波先完成纵向剧本闭环 |
| 四层继承过度复杂 | 用户不理解当前规则来源 | 原型显示有效值、来源层和覆盖差异 |
| 自动化喧宾夺主 | 产品退化为 Run 面板 | 业务页面主入口，运行中心只作横向诊断 |
| ScriptPackage 过度约束创作 | 候选同质化 | 固定治理和交付字段，创意方向保持开放候选 |
| 客户资料不足 | 只能生成 blocked 草稿 | 15维诊断、补料清单和 CreativeDraft 路径 |
| 未知产物渲染失败 | 审阅中断 | 原生核心、安全投影、预览件、本机打开、元数据降级 |
| 定时任务产生噪音 | 团队忽略通知 | 类型 allowlist、actionable 默认、失败自动暂停 |
