# 内容创作 AI Infra 能力地图

状态：`当前实现对账 + 首个用户前的收口能力地图`。

更新时间：2026-08-23。

## 1. 状态口径

不要用一个 `partial` 把“已有代码”“本地可用”“只有契约”“只有 Fixture”混在一起。每项能力同时记录实现状态和可用范围：

| 状态 | 含义 |
| --- | --- |
| `current` | 代码、契约和测试形成可继续演进的支持能力 |
| `current-local` | 本地工作区可运行，外部或服务端闭环仍有限 |
| `current-server` | 服务端事实、审核或投影已实现 |
| `partial` | 有代码、契约和测试，但真实 Provider、平台账号或运营闭环仍缺 |
| `external-dependency` | ContentCloud Adapter/契约已具备，真实平台账号、配额或 SaaS 服务由外部提供 |

实现状态描述“现在有什么”。无用户迁移义务时，重复入口直接删除，不进入新的 Capability、SOP 或 Runtime 分支。

## 2. ContentCloud 当前对象图

```text
Workspace
  ├── LocalRun 3.0 + Claim + Handoff 1.0
  ├── 20-sources/ SourceRegistry 3.0
  │     └── EvidenceBundle 3.0
  ├── 30-knowledge/ Knowledge pages + KnowledgePack 3.0
  ├── 50-production/
  │     ├── Brief 3.0 / ArticleBrief 1.0
  │     ├── ContentBatch 3.0 -> ContentItem 3.0 / Article 1.0
  │     └── StoryboardPackage 1.0 -> SeedancePromptPackage 1.0
  └── 60-delivery/ DeliveryPackage / WeChatDelivery 1.0

ContentCloud server
  ├── SubmissionBundle 3.0 -> SubmissionRevision
  ├── ReviewGrant / ApprovalDecision -> ApprovedSnapshot
  ├── Source / KnowledgeObject / KnowledgeSnapshot / RightsRecord
  ├── Artifact / DeliveryPackage / PerformanceObservation
  └── CustomerJourney / WorkspaceMaterial / CreativeResult projections

V8 Runtime
  └── WorkTask -> JobRun -> NodeRun -> RuntimeAttempt -> Effect/Event
```

这些对象已经存在，Infra 的任务是连接它们的执行、版本和权限边界，而不是为每个能力创建一个泛化对象。

## 3. 执行平面与所有权

| 执行平面 | 适合做什么 | 当前状态 | 事实所有者 |
| --- | --- | --- | --- |
| 本地工作区 + Plugin/Skill/MCP | 资料读取、知识提取、内容候选、Lint、Handoff、交付导出 | `current-local` | LocalRun、工作区文件、V3 包 |
| ContentCloud 服务端 | 提交接收、租户隔离、审核、批准快照、Artifact 和投影 | `current-server` | Submission、Review、Source/Knowledge、Artifact |
| V8 Agentic Runtime | 长时自动化、租约、状态、恢复、Effect、Outbox | `current` | JobRun、NodeRun、RuntimeAttempt、Effect |
| 外部执行者 | 本地/远程 Agent、Agent SaaS、创作 SaaS、Worker、模型、人工和渠道 | `current` / `external-dependency` | Adapter、Runtime 和回执由 ContentCloud 管理；具体进程、账号和配额由外部系统提供 |

Runtime 只拥有执行事实；业务域拥有来源、知识、内容、审批、产物、交付和外部回执事实。

### 3.1 工作面能力映射

| 能力 | Codex | Desktop | Web Studio / Operations |
| --- | --- | --- | --- |
| 项目内容目录 | 当前任务引用与 MCP View | 完整本地目录、业务分组和离线状态 | 已提交的团队目录与云端投影 |
| 内容修改 | Claim -> Proposal -> Apply | 外部编辑观察、冲突处理和受控命令 | 基于 Cloud Revision 创建团队修订 |
| 同步与上传 | 可发起显式命令，不维护队列 | 持久队列、分片恢复、游标和状态 | 服务端策略、配额和治理结果 |
| 审批 | 解释反馈、生成修订，不作决定 | 个人收件箱和精确版本决定 | 公开/团队审核与运营治理 |
| Runtime | 交互执行或 Harness | 用户可理解的进度、等待和恢复动作 | Runtime Explorer 与跨租户运维 |

Desktop 当前为 `target`，不能写成已经发布。其完成门槛以 [Desktop 一次性交付计划](../product/content-work-os-desktop/04-delivery-plan.md) 为准。

## 4. 任务入口与需求定义

| 能力 | ContentCloud 当前对象 | 状态 | 真实边界 |
| --- | --- | --- | --- |
| 客户任务入口 | ExperienceTemplate、WorkTask、Studio BFF | `current-server` | 客户围绕内容任务工作，不直接编辑 Runtime 图 |
| SOP 与 Gate | SOPVersion、StageDefinition、GateDefinition | `current-server` | 任务准入固定版本和摘要 |
| 结构化 Brief | `contentcloud.brief/3.0`、`article-brief/1.0` | `current-local` | 先在工作区生成并 lint，再提交审核 |
| 内容批次 | `contentcloud.content-batch/3.0` | `current-local` / `current-server` | 视频脚本、文章、电商和连载小说均以业务 Schema 校验后进入统一 SubmissionRevision；不混装内容类型 |
| 受众策略 | `audience-strategy/1.0`、Yuntu taxonomy snapshot | `current-local` / `current-server` | 受众策略是内容前置事实，不应被泛化 Brief 替代 |
| 批量矩阵与变体 | ContentBatch fanout、controlled variables | `current-local` | 有批次与单项 lint；实验分析仍有限 |
| 线上实验与自动优化 | PerformanceObservation、RatingDecision、Learning Candidate | `partial` | 导入和人工复盘已有，自动反馈策略仍需真实效果数据 |

## 5. 搜索、发现与趋势

| 能力 | 当前状态 | 已有事实 | 缺口 |
| --- | --- | --- | --- |
| 人工补充灵感 | `current-server` | Studio 保存任务输入，可选项目参考 | 搜索结果与人工输入仍需统一候选契约 |
| 本地资料选择 | `current-local` / `current-server` | WorkspaceMaterial、SourceRevision、摘要固定 | OCR/ASR 等派生能力仍有限 |
| `source.search` Capability | `current-server` | `internal/integration/provider/source` Provider、查询摘要和结果物化 | 搜索供应商账号和配额 |
| `source.fetch` Capability | `current-server` | 受控 Fetcher、白名单、SSRF/大小限制和 Evidence 生成 | 目标站点授权、robots 和网络可达性 |
| Web 搜索 | `current-server` / `external-dependency` | `source.search` API、SearchReceipt、SourceRevision | 搜索 Provider、计费和平台合规 |
| 趋势/热榜 | `partial` / `external-dependency` | 统一查询/采集边界可复用 | 平台热榜 API、时效与授权 |
| 指定站点采集 | `current-server` / `external-dependency` | 白名单 Fetcher、tombstone 和 Evidence 物化 | 站点授权、robots、频率 |
| 内部资料搜索 | `partial` | 资产和本地 Memory 有查询投影 | 企业知识库、权限裁剪索引仍未统一 |
| 联邦搜索与去重 | `partial` | SourceRevision 摘要可作为底层不变量 | 多源质量评测和语义去重策略 |

搜索候选不能直接成为 Knowledge 或 Creative Result。正确推进路径是：

```text
query/candidate -> SourceRevision -> EvidenceBundle
  -> 用户选择/项目参考 -> Knowledge candidate
  -> 人工治理 -> KnowledgeSnapshot / ApprovedSnapshot
```

## 6. 采集、解析与数据接入

| 能力 | 状态 | 当前可复用实现 | 下一缺口 |
| --- | --- | --- | --- |
| 文件登记 | `current-local` | MIME 检测、SHA-256、copy/reference、重复和冲突拒绝 | 扩展生产文件类型与异步处理 |
| 本地 EvidenceBundle | `current-local` | `IngestLocalSource`、ParserVersion、locator、quote、review status | 更多解析器、OCR 置信度和大文件策略 |
| WorkspaceMaterial 上传/导入 | `current-server` / `partial` | 文件上传、SourceRevision、项目隔离、处理状态、预览 | 更完整处理队列和失败隔离 |
| URL 抓取 | `current-server` / `external-dependency` | Fetcher、HostPolicy、大小/协议限制和 SourceRevision | 目标站点授权、robots、网络 |
| API 连接器 | `current-server` | Connector Registry、并发租约、cursor 原子提交、tombstone、重放 | OAuth 和远端 API |
| OCR/ASR/文档结构解析 | `partial` | `ingest.Parse` 和确定性解析入口 | 页码、时间码、置信度和大型媒体 Worker |
| 媒体探测 | `partial` | `mediapipeline` 和媒体 Artifact 校验 | 统一规格探测 Worker |
| 规范化/去重 | `partial` | 文件摘要、SourceRevision 和部分本地校验 | 跨来源规范 URL、语义近重复和质量评测 |

采集层输出只能是 Source、SourceRevision、EvidenceBundle、WorkspaceMaterial 或候选引用，不得直接写入已批准知识或客户结果目录。

## 7. 知识、权利与上下文

| 能力 | 当前对象 | 状态 |
| --- | --- | --- |
| 知识候选导入 | Markdown knowledge pages、`knowledge_import` | `current-local` |
| 知识检查与打包 | `knowledge_lint/query/diagnose/pack`、KnowledgePack 3.0 | `current-local` |
| 服务端知识治理 | `domain.KnowledgePack` 聚合、KnowledgeObject、KnowledgeSnapshot、Decision | `current-server` |
| 证据引用 | EvidenceRef、EvidenceSpan、SourceRevision digest | `current` |
| 权利和冲突 | RightsRecord、Conflict candidate、rights refs | `partial` |
| ContextView | Runtime 固定引用、摘要和策略视图 | `current` |
| RAG/全文/向量索引 | 可重建投影 | `partial` |
| 引用完整度与失效传播 | Content/Knowledge lint 有部分检查 | `partial` |

这里不是两个交换 Schema 并存。本地 `contentcloud.knowledge-pack/3.0` 是 workspace 文件交换契约；服务端 `domain.KnowledgePack` 是持久化领域聚合，通过对象引用、查询策略和发布状态参与知识治理，并没有 `contentcloud.knowledge-pack/1.0` API 契约。此前零引用的 `contentcloud.knowledge-pack/1.0` 常量已经删除，架构守卫禁止恢复这个虚假版本标识。两者未来若需要互通，应通过显式 Submission/DTO 映射，不得因为名称相同就合并职责。

## 8. 数据互通与 Agent 协作

### 8.1 已有契约

| 契约 | 用途 | 状态 |
| --- | --- | --- |
| `SourceRevision` / `EvidenceRef` | 固定来源和可定位证据 | `current` |
| `contentcloud.local-run/3.0` | 本地运行、检查、输入和输出引用 | `current-local` |
| `contentcloud.handoff/1.0` | 跨对话交接、Claim、输入摘要和恢复 | `current-local` |
| `contentcloud.agent-handoff/1.0` | Studio/API 向已发布 Agent 客户端生成有目标约束的启动交接 | `current-server`；Codex deep link 与 Claude Code command Adapter 已验证，Pi、远程 Agent 和其他宿主保持 planned |
| `contentcloud.knowledge-pack/3.0` | 本地知识包 | `current-local` |
| `contentcloud.content-batch/3.0` | 内容矩阵、状态、交付 profile 和 blocked reasons | `current-local` |
| `contentcloud.submission-bundle/3.0` | 本地向服务端提交的不可变包 | `current` |
| Runtime JobPlan/ContextView/Effect | 自动化执行、状态、工具和外部副作用 | `current` |
| Agent Plugin Claims/Registry | 能力、Schema、权限、成本、数据流、评测和撤回 | `current` / `partial` |

### 8.2 Handoff 边界

当前存在两个职责不同、都不是 Codex 专属的 Handoff：

1. `contentcloud.handoff/1.0` 是 LocalRun 内的持久恢复事实，负责 Claim 释放、输入输出摘要、下一动作和跨对话接续。
2. `contentcloud.agent-handoff/1.0` 是 Studio/API 到 Agent 宿主的启动交接，负责客户端能力、目标项目/修订、Plugin 身份、启动方式和安全提示。API 与 DTO 是通用的，Codex deep link 与 Claude Code command Adapter 已达到 `available`；Pi Agent 或 SaaS 接入应增加 Adapter，而不是复制路由和业务 SOP。

两者之间通过 `project_id`、目标引用、digest 和首次 `workspace_context` 校验衔接，但不共享生命周期，也不应被合并成第三个泛化 Envelope。远程 Agent/SaaS 的异步运行状态继续由 Runtime Attempt/Effect/Receipt 持有。

### 8.3 开放执行者类型

业务节点绑定的是能力，不是执行者品牌。例如脚本生成、图片生成、公众号排版或渠道发布这类能力节点，可以由不同方式完成；具体 Capability ID 必须来自已发布目录，不能把下面的分类名称当作现有契约：

| 执行者类型 | 典型方式 | 当前状态 | 必须提供的适配能力 |
| --- | --- | --- | --- |
| 本地交互 Agent | Codex、Claude Code、Pi Agent、其他 Agent 客户端 | `current`：Harness 注册和 Pi Runtime 测试已存在；宿主进程仍是 `external-dependency` | Workspace 绑定、输入摘要、结构化输出、Claim/Handoff |
| Runtime Agent Harness | 无人值守 Codex/Claude、Pi、remote-http、agent-saas | `current` | Detect、Start、Resume、Interrupt、Inspect、事件流 |
| 远程/托管 Agent | 自建 Agent 服务、托管 Agent Runtime | `current-server` | 身份、租约、幂等、事件、超时、取消、恢复和账单 |
| Agent 工作流 SaaS | 研究、编排、营销自动化类 SaaS | `current-server` / `external-dependency` | Capability/Schema 映射、异步 Effect、Webhook/Inspect/Receipt |
| 垂直创作 SaaS | 图片、视频、剪辑、配音、排版、翻译、发布平台 | `partial` / `external-dependency` | 固定输入文件、派生产物、权利/费用、外部 ID 和导出清单 |
| 确定性 Worker | Parser、Lint、Renderer、Transcoder、Packager | `current` | 可重复输入输出、错误码、资源计量和 Artifact digest |
| 浏览器/Computer Use | 使用用户登录态完成只能在 UI 中执行的步骤 | `external-dependency` | 逐步授权、域名约束、不可重复副作用保护、截图/结果证明 |
| 人工 | 作者、编辑、审核、设计、运营、发布人 | `current` | Human Task、截止时间、输入输出清单、Decision/Receipt |

接入一个新执行者只允许增加 Adapter、ExecutionProfile、能力声明和测试 Fixture，不得复制业务 SOP 或新增平行内容事实。执行者输出先进入 candidate；只有现有 lint、review、ApprovedSnapshot、Artifact/Delivery 流程可以把它推进为正式结果。

## 9. 模型、媒体和确定性工具

| 能力 | 状态 | 当前边界 |
| --- | --- | --- |
| 通用 Agent Harness 接口 | `current` | `AgentHarnessAdapter` 定义 Detect/Start/Resume/Interrupt/Inspect，Runtime 按 `HarnessKind` 和 `ExecutionProfileID` 调度 |
| Codex/Claude Harness | `current` / `partial` | 已有结构化事件与恢复实现；客户侧发布能力和宿主能力并不完全对等 |
| Pi Agent/其他本地 Agent Harness | `current` / `external-dependency` | Pi Runtime 契约测试已存在；具体 Agent 安装和能力需逐环境验证 |
| 远程 Agent/Agent SaaS Adapter | `current-server` / `external-dependency` | API/Webhook/Effect、durable inbox、重放和回执已实现，不能把 SaaS 会话当权威任务状态 |
| 垂直创作 SaaS Adapter | `partial` / `external-dependency` | 当前部分由人工外部工具和媒体 Provider 完成；真实服务仍需输入映射、产物摘要、费用和回执 |
| 文本/脚本/文章生成 | `current-local` | Provider-neutral 内容 Schema，候选由 Skill 写入批次 |
| Storyboard 图像候选 | `current-local` | Capability digest、首尾帧、Review Sheet、locked_digest |
| 异步媒体 Provider | `partial` | MediaGenerationJob、Effect、Provider inbox、取消/对账状态和账单模型已有；视频批量准入 DTO、整批原子写入、确认后统一入队、月度预算与并发门禁和 HTTP 入口已完成；未知提交具备轮询调度、外部 ID 补录和无重复提交恢复；状态查询未知可恢复到 `running`、`failed` 或已请求取消的 `cancelled`，且不重复提交或创建 Attempt；终态实际费用统一回写任务与调用尝试，取消 unknown 后可收敛 Runtime Effect；D1 模拟恢复矩阵、D2-1 存储接口/测试盘点和 D2-2 PostgreSQL 集成测试实现已完成，D2 对照范围见[阶段 D2 PostgreSQL 对照计划](../roadmap/v8/25-stage-d2-postgres-parity-plan.md)，真实 PostgreSQL、Provider 和完整账单回执仍待完成 |
| `media.video.generate` | `partial` / `external-dependency` | Built-in SOP、媒体任务、批量准入、月度预算与并发门禁和 Artifact 已有；视频生产迁移以受治理的资产锁定、批量准入、Provider 回执与交付包为唯一主链，批量准入已完成，Provider 未知提交恢复已启动，实际生成回执及后续交付仍依赖阶段 D-G |
| Model Gateway | `partial` | vLLM/SGLang OpenAI-compatible Provider 已接入，不创建额外 Gateway 事实层 |
| vLLM/SGLang Adapter | `current-server` / `external-dependency` | Provider registry、请求/响应摘要和 receipt 已有；端点、模型和 GPU 是外部依赖 |
| 转码、渲染、打包 | `current` / `partial` | 确定性 Worker、Artifact、DeliveryPackage 已有；视频生产迁移 阶段 E 已完成合成输入与血缘审计、E1a 文档契约、E1b Go Manifest 值对象、E1c Manifest 驱动入口、E1d-a 矩阵基线、E1d-c 最终审核交付硬门禁、E1d-b Memory 原子写入测试、PostgreSQL 原子事务测试入口和 Blob `Get`/`Put` 失败无事实测试；`CreateFinalRender` 重新校验快照、审核、Artifact、时间轴和输出契约，并按 digest 幂等；清理失败已自动登记脱敏 Runtime 诊断，Operations/BFF 已提供租户范围查询、详情和 CAS 重试；跨进程恢复、PostgreSQL 真实故障注入、真实合成和剪映草稿能力仍待补齐，具体恢复队列见 [E1d-b 恢复执行计划](../roadmap/v8/28-e1d-b-recovery-execution-plan.md)；能力将实现为受版本控制的渲染/导出 Adapter，不创建平行交付模型 |

R2-2 文档同步（2026-08-23）：已完成 Runtime 清理诊断状态 CAS；Memory/PostgreSQL 均拒绝旧版本、非法状态转移、终态重试和身份篡改。R2-3 自动登记、R2-4/R2-5 查询与 CAS 重试入口已完成；独立进程恢复和真实 PostgreSQL 执行仍待验收，详见 [R2 清理重试计划](../roadmap/v8/29-e1d-b-r2-cleanup-retry-plan.md)。

## 10. 产物、资产与反馈

| 能力 | 当前状态 | 事实边界 |
| --- | --- | --- |
| Workspace Material | `current-server` / `partial` | 客户上传/导入资料投影，不等同于生成结果 |
| Creative Result | `current-server` / `partial` | 人物、剧本、分镜、图片、视频结果投影 |
| ApprovedSnapshot | `current-server` | 唯一的服务端批准事实，不能在本地伪造 |
| Artifact | `current-server` / `partial` | blob digest、媒体规格、Provider/能力和用途 |
| DeliveryPackage | `current-server` / `partial` | 交付视图和派生文件，不成为新的资产写模型 |
| 视频生产资产一致性 | `partial` | `StoryboardVisualBinding` 已固定角色/场景/道具/商品来源引用、摘要、身份锚点和权利引用，并纳入 `locked_digest`；真实 WorkspaceMaterial/Artifact 跨版本失效传播仍待阶段 G 验收 |
| 剪映草稿交付 | `partial` / `target` | F0 文档基线与 F1-F4b 只读 exporter、应用/HTTP/CLI 交付编排入口、确定性 ZIP、完整输入/血缘校验、失败矩阵、契约测试和导出 lint 已完成；当前输出固定 ZIP、manifest 和安全命名素材，F5 真实导入验收尚未完成，且导出不表示已发布；该能力不创建平行任务、审批、Artifact 或 DeliveryPackage 模型 |
| WeChatDeliveryPackage | `current-local` | 已批准文章到可手工上传包的确定性投影 |
| 血缘 | `partial` | ApprovedSnapshot -> Artifact -> Delivery -> TaskDelivery -> ChannelPublication -> PerformanceObservation -> RatingDecision 已有 Memory/模拟渠道切片；真实渠道和外部指标证据仍待补齐 |
| 效果观察和学习候选 | `partial` | 可导入 PerformanceObservation、RatingDecision，不能自动改写事实 |
| 自动渠道指标回流 | `partial` / `external-dependency` | PerformanceObservation、Channel callback/reconcile 已有；真实渠道指标 API 仍是外部依赖 |

## 11. 发布与分发

| 能力 | 状态 | 说明 |
| --- | --- | --- |
| ContentCloud `publish_preflight/apply` | `current` | 提交本地不可变包到服务端，需同一 `plan_id` 显式确认 |
| WeChat 交付包 | `current-local` | `wechat_package_export/lint`，不登录、不上传、不自动发布 |
| 人工发布证明 | `partial` | 可记录外部绑定，但不能把本地交付包当成已发布 |
| Channel Registry/账号授权 | `current-server` / `external-dependency` | SecretRef、账号、区域、Adapter 和状态已实现；真实账号由外部提供 |
| 渠道规格和渠道派生 | `partial` | Content Schema、DeliveryPackage 和 Adapter 已有，平台专属派生按真实渠道补齐 |
| 自动 Submit/Inspect/Withdraw/Receipt | `current-server` / `external-dependency` | ChannelPublication 状态机、幂等、Callback、Reconcile 已有；真实平台 Adapter 是外部依赖 |
| 互动与表现回流 | `partial` / `external-dependency` | 指标导入和归因绑定已有；平台指标口径和账号权限仍需外部提供 |

## 12. Plugin、Connector 和开发者平台

| 能力 | 状态 | 当前事实 |
| --- | --- | --- |
| Agent Plugin 1.0.0 包 | `current` | `plugin.json`、skills、mcp.json、ContentCloud claims |
| 包校验与摘要 | `current` | 路径、文件限制、manifest、Skill/MCP 发现和 digest |
| 签名 Registry | `current` / `partial` | Ed25519、发布 profile、评测、生命周期、撤回 |
| Codex/Claude Host Adapter | `current` / `partial` | 计划、确认、CAS、安装、回执、回滚 |
| Capability Registry | `partial` | 能力 ID、版本、Schema、执行模式、成本和副作用 |
| Connector SDK | `current` | 需要授权、增量、错误、删除和权利语义的核心实现已存在 |
| Contract Test Kit | `partial` | 已有各域测试和 Fixture，尚未抽成统一外部套件 |
| 任意第三方插件市场 | `partial` | 当前只治理官方/精选包，不开放任意代码和任意网络 |

## 13. 首个用户前的删除清单与缺口优先级

### 13.1 已完成收口与剩余历史证据

| 旧/重复 surface | 现行事实源 | 动作 | 清理结果 |
| --- | --- | --- | --- |
| 虚假的 `contentcloud.knowledge-pack/1.0` Schema 标识 | 本地交换 `knowledge-pack/3.0` + 服务端领域聚合 | 零引用常量已删除；守卫禁止恢复 | 已删除 |
| 旧草案 `contentcloud.evidence-bundle/1.0` | `contentcloud.evidence-bundle/3.0` | 生产解析与正向断言不允许恢复 | 已删除 |
| 旧品牌专用 Handoff facade | 通用 `contentcloud.agent-handoff/1.0` + 客户端 Adapter | 品牌专用路由/DTO 已不存在；所有宿主复用同一 Adapter | 已删除 |
| 顶层单工作区本地配置与加载时迁移 | `daemon_bindings` | 旧字段解码、合并、重写和兼容测试已删除；守卫禁止恢复 | 已删除 |
| 按名称/形状识别旧短视频 SOP、修复旧内置元数据 | 显式 built-in ID + TemplateKey + BuiltIn + SourceRef | 运行时迁移和正向兼容测试已删除；守卫禁止恢复 | 已删除 |
| V7 Runs/旧 Session Mirror 生产代码与迁移 | V8 Runtime Job/Attempt/Session Ref | 生产读取、旁路投影、Session Mirror 创建/删除迁移和正向测试均已删除；带旧迁移记录的开发库必须重建 | 已删除 |
| 没有真实消费者的渠道兼容入口 | 当前 Delivery/Channel Adapter | 删除入口，不建立 fallback；人工 Runbook 作为明确的 current 模式保留 | 已删除 |

业务修订、批准快照、Artifact 和外部回执不在删除清单内，它们是用户真正需要的历史事实。

### 13.2 继续建设的唯一主线

1. 保持上述旧入口不可恢复；迁移集合已经压平，带有已删除 Session Mirror 版本记录的开发库必须重建。
2. 把本地 V3 workspace 与服务端 Submission/Review/ApprovedSnapshot 画成一条真实可验证链路。
3. 将 `source.search/fetch` 从 Capability 声明推进到一个真实、白名单、可计费、可回执的采集器。
4. 把 Plugin/Skill/Environment Binding 纳入能力分发主链，而不是泛称 Connector SDK。
5. 统一 ContentBatch、Article、Storyboard、Delivery 的跨平面引用和摘要校验。
6. 用一个真实渠道补齐 `Validate -> Prepare -> Submit -> Inspect -> Receipt`；微信公众号人工 Runbook 是 current 模式，不是兼容层。
7. 用一个非 Codex/Claude 执行者验证开放接入契约；Pi Agent、远程 Agent 或 Agent SaaS 三选一，以真实能力和可测恢复语义为准。
8. 在真实 Provider、渠道回执和效果数据出现后，再建设 Model Gateway、联邦搜索、RAG、自动优化和多渠道发布。
