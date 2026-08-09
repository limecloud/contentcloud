# 外部参考架构与 ContentCloud 边界

状态：`参考基线，依赖按边界择优引入`。

更新时间：2026-08-09。

本文记录对 ContentCloud Agentic Job Runtime、客户创作台、运营控制台和客户资产入口有直接启发的公开系统。它不是竞品清单，也不要求 ContentCloud 采用其中任何一个产品。

## 1. 结论

没有发现一个系统完整复制 ContentCloud 的组合，但各层已有成熟先例：

```text
Camunda 8              产品面和执行者分层
Dify                   流水线定义与客户应用发布
Temporal / LangGraph   可恢复执行、检查点和人工中断
Agent Memory 项目       记忆形成、作用域、时间语义和混合召回
Adobe GenStudio        创作、模板、资产和审批治理
Runway / Frame.io      生成结果血缘、版本和审核体验
```

ContentCloud 的新颖性不在于发明这些单点能力，而在于把它们组合到内容创作领域：客户看到简单的创作任务和资产工作区，运营维护流水线，Runtime 统一调度本地 Agent、托管 Agent、确定性 Worker、外部服务商和人工，客户资料与生成结果通过不同投影继续被使用。

## 2. 参考系统对照

| 系统 | 已验证的设计 | 可借鉴部分 | 不应直接复制 |
| --- | --- | --- | --- |
| Camunda 8 | Web Modeler、Zeebe、Job Workers、Tasklist、Operate 分开 | 运营编排、持久 Runtime、执行者领取任务、人工任务和独立运维 | BPMN 和通用流程对象不能直接成为客户创作语言 |
| Dify | Workflow/Chatflow 在画布中编排，再发布为 Web App、API 或 MCP | 定义、发布、运行、结果管理和 Human Input 分离 | 通用表单和聊天输出不足以表达剧本、分镜、版本和交付 |
| Temporal | Workflow Execution、Activity、事件历史、重试和长时间运行 | 把可恢复执行放在服务端，外部调用通过活动和幂等策略治理 | 不把 Temporal 的 Workflow History 当作 ContentCloud 业务正文 |
| LangGraph | Checkpointer、Thread、Interrupt、状态恢复和 Agent Server | Agent 内部状态可以恢复，人工中断可以回到明确节点 | Thread、消息和模型状态不能替代 WorkTask、Gate 和批准事实 |
| TencentDB Agent Memory | 当前默认分支是由 MemoryCore、MemoryProxy、MemoryKnowledge、MemoryPanel、SDK 和部署配置组成的团队级 Memory Hub；资产带 owner、version、status、visibility、ACL 和 Agent binding | 记忆资产元数据、分层形成与召回、最小 Agent loadout、身份防伪和渐进式披露 | 不复制 Team/User/Agent/Task、统一 Memory Asset、MemoryPanel 或透明模型代理，避免形成第二套身份、知识、能力和运营控制面 |
| Mem0 | Python 库与自托管 API；`add` 先检索再抽取并维护增删改历史，检索可融合语义、关键词和实体信号 | 小颗粒事实抽取、去重、调用者不能伪造 scope、变更历史和混合检索 | `user_id/agent_id/run_id` 不能替代租户、项目、工作和版本权限；抽取结果不能成为事实源 |
| LangMem | Python/LangGraph 工具库；按 namespace 检索、合并并 `put/delete`，可把形成过程移到后台 | 热路径与后台形成分离、namespace 模板、结构化 profile/collection 和动作白名单 | 不把 LangGraph Store 变成 ContentCloud 存储，也不允许 Agent 直接增删正式知识 |
| Graphiti | Python 时间知识图谱库；事件保留原文，事实边带有效/失效时间，并组合 BM25、向量、图遍历与重排 | 事件证据、矛盾事实的时间语义、历史保留和多路召回 | 首版不为记忆引入 Neo4j/FalkorDB，不把图关系提升为未经审核的业务事实 |
| Letta | Python Agent Server；有限大小且可只读的 Block 有版本和历史，Git 模式下文件是事实源、PostgreSQL 是缓存 | 有限上下文预算、可读/可编辑块、文件事实源与数据库缓存的明确分工 | Persona/Human Block 和 Letta Agent Runtime 不成为 ContentCloud 领域模型或执行主控 |
| Cognee | Python 数据与图流水线；Session 与长期知识分层，候选经筛选后才写入带来源的知识，并按 dataset/user/tenant 限制 | 短期记忆到长期知识的显式晋升、dataset scope、来源删除传播 | 不复制整套 graph/vector/ingest pipeline，也不让其权限模型包住 ContentCloud 权限模型 |
| Adobe GenStudio | Create、Assets、Experiences、Templates、Reviews/Approvals 分层 | 生成上下文、模板、品牌规则、草稿、批准结果和搜索分开 | 企业级营销套件的复杂导航不适合直接暴露给客户 |
| Runway | 生成结果进入 All Generations，上传输入进入 Private，结果拥有 lineage | 结果库、输入隔离、生成血缘和从结果继续变体 | Runway 的通用 Asset 术语不直接决定 ContentCloud 的客户命名 |
| Frame.io | Version Stack、比较、评论和审核沿版本管理 | 版本和审批是不同维度，历史版本不覆盖当前版本 | 它是审阅与媒体协作工具，不是内容任务 Runtime |

### 2.1 Agent Memory 源码级对照

以下矩阵用同一组维度比较六个项目，避免只按 README 中的产品术语类比：

| 项目 | 事实所有者与形成 | scope、时间与删除 | 召回 | 拓扑与接入代价 |
| --- | --- | --- | --- | --- |
| TencentDB Agent Memory | Memory Hub 自己拥有 Chat Memory、Skill、Wiki、CodeGraph Asset；资产带版本、状态和 binding | Team/User/Agent/Task、visibility、role default 与 ACL 共同判权；知识表带 service/team/owner/user/agent/task 维度 | L0-L3 渐进披露，BM25 与向量经 RRF 融合，Fixed Binding 形成最小 loadout | Node 22 多服务，可本地或团队部署；SDK/HTTP/MCP 可接入，但 Proxy/Panel/Core 一起采用会复制控制面 |
| Mem0 | `add` 先检索已有记忆，再由 LLM 选择 ADD/UPDATE/DELETE；SQL history 保留 old/new value | `user_id/agent_id/run_id` 至少存在一个，调用 metadata 不能覆盖这些身份字段；删除有历史但缺少 ContentCloud 领域版本语义 | 语义、关键词/BM25 与实体信号融合 | Python 库或 FastAPI 服务；Go 侧只能走独立进程/API，且仍需 ContentCloud 包装 scope 与来源 |
| LangMem | 在 LangGraph Store 中检索既有项，再 enrich/consolidate，最后 `put/delete`；可由 Agent 显式调用或后台执行 | namespace 模板提供隔离；稳定 key 支持替换和删除，但没有 ContentCloud 的租户权限与正式知识审批 | 依赖 Store 的 search，并在形成前取有限既有记忆 | Python/LangChain/LangGraph 库；嵌入会引入第二语言运行时和 LangGraph Store |
| Graphiti | Episode 保存原文，EntityEdge 保存从事件提取的事实；新事实可让冲突旧边失效而不删除历史 | `group_id` 限制查询；`valid_at/invalid_at/expired_at/reference_time` 表达双时间与过期 | 同时搜索 edge/node/episode/community，可组合 BM25、cosine、BFS、RRF/MMR/cross-encoder | Python 库并依赖 Neo4j/FalkorDB 等图存储；时间语义值得采用，图基础设施首版不值得引入 |
| Letta | Block 是 Agent 核心记忆单元，PostgreSQL 保存版本和 actor history；Git-enabled 模式以 Git 为事实源、数据库为缓存 | Block 可绑定 project、设 read-only 和 char limit；历史按单调 sequence 保存，但权限域仍属于 Letta Agent | Context Window Calculator 分别预算 system、core memory、filesystem、summary、messages、tools | 完整 Python Agent Server；适合借鉴预算和文件/缓存分工，不适合作为 ContentCloud 记忆库嵌入 |
| Cognee | Session cache 与 ingest 知识分开；session 候选经 curate 并与历史比较，接受后才生成带 session 来源的 Markdown 并进入知识管线 | dataset/user/tenant/role 约束访问；来源可驱动图投影删除 | 记录使用过的 graph/context/vector 结果，并通过图和向量管线补充上下文 | 大型 Python 数据/图平台；可借鉴晋升与来源删除传播，整体接入会复制 ingest、ACL 和存储栈 |

## 3. ContentCloud 的目标映射

```text
平台运营人员
    |
    v
ExperienceTemplate / Published SOP / Capability Registry
    |                         |
    |                         +--> 运营控制台：配置、发布、租户、诊断
    v
JobPlanRevision -> JobRun -> NodeRun -> RuntimeAttempt
    |
    +--> ContentCloud Worker
    +--> 本地 Codex / Claude Code
    +--> 托管 Agent
    +--> 搜索、图片、视频和其他 Provider
    +--> 人工 Gate
    |
    v
CustomerJourneyProjection            Customer Asset Surface
    |                             /                           \
    v                            v                             v
客户创作台：输入、进度、确认、交付  WorkspaceMaterialProjection  CreativeResultAssetProjection
                                 文件夹与导入资料               人物、剧本、分镜、图片、视频
```

### 3.1 与 Camunda 的关系

Camunda 的 Job Worker 证明了“能力类型”和“执行者实例”可以分离：Runtime 创建工作项，符合能力的 Worker 领取并完成，超时或失败后由 Runtime 重试或产生事件。ContentCloud 需要在此基础上补充内容输入版本、人工批准、外部费用、权利和结果资产。

### 3.2 与 Dify 的关系

Dify 证明了运营人员可以在复杂画布中设计 Workflow，再把它发布成面向终端用户的 Web App。ContentCloud 应复用这种发布关系，但用 `ExperienceTemplate` 定义更强的客户体验契约：客户步骤、输入表单、结果展示和唯一主要动作，而不是把内部节点画布直接开放给客户。

### 3.3 与持久 Runtime 的关系

Temporal 和 LangGraph 的共同启发是：模型会话、线程和进程内存都不应成为唯一事实源。ContentCloud 仍应由服务端保存 JobRun、NodeRun、Attempt、Gate、Effect、Checkpoint 和业务结果引用；Agent 只领取最小上下文并提交结构化结果。

### 3.4 与生成式内容产品的关系

Adobe、Runway 和 Frame.io 共同说明，内容产品至少需要把以下维度分开：

```text
输入参考       生成结果       内容类型       版本       审批状态       使用血缘
```

ContentCloud 在客户面把“入口统一”和“领域统一”分开：客户只看到一个“资产”入口，但“我的资产”承接明确上传/导入的工作区资料，“创作结果”承接流水线结果，搜索候选和来源证据仍留在任务参考。这样符合客户对文件工作区的直觉，又不要求客户判断底层治理对象。

### 3.5 与 Agent Memory 的关系

TencentDB Agent Memory 当前默认分支不是单机 SQLite 插件，而是可以本地部署或作为团队服务部署的 Memory Hub。`MemoryCore` 管理 Chat Memory、Skill 等资产与分层召回，`MemoryKnowledge` 管理 Wiki/CodeGraph，`MemoryPanel` 提供团队管控，`MemoryProxy` 代理模型请求并注入或回写上下文。其元数据和权限检查覆盖 Team、User、Agent、Task、ownership、visibility、ACL、version、status 与 fixed binding；代理桥接还会从会话侧强制注入身份，并限制模型可访问端点和写操作。

这些能力说明它是一个完整的记忆服务端/本地可部署平台，而不是 ContentCloud Go 进程可以直接链接的底层库。通过它的 SDK、HTTP 或 MCP 做试验是可行的，但必须位于 ContentCloud 受控适配器之后；Codex、WorkBuddy 等宿主不能直接连接 MemoryProxy 或自行提供 tenant、project、agent scope。

综合 TencentDB Agent Memory、Mem0、LangMem、Graphiti、Letta 和 Cognee 的源码，ContentCloud 借鉴以下原则：

1. **低层保留证据，高层保留结构。** 召回摘要必须回到工作区文件、对象版本、事件或受限会话引用。
2. **形成与使用分离。** 热路径只做确定性扫描、失效和召回；LLM 抽取、去重与合并可以后台执行，失败不能阻断权威文件读取。
3. **范围不能由模型声明。** scope 必须从已绑定工作区、项目、任务和 Attempt 凭据推导，请求参数只能缩小范围。
4. **时间和冲突是一等信息。** 候选要记录观察时间、有效区间和来源；新候选与旧候选冲突时保留历史，不能静默覆盖。
5. **文件与索引分工。** 本地文件继续承载可读、可编辑和可迁移的事实；SQLite、全文和可选向量数据只作为可删除重建的 Memory Projection。
6. **渐进式披露。** `workspace_context` 和 `ContextView` 先提供小型摘要，只有任务需要且权限允许时才读取大段正文或历史记录。

ContentCloud 不直接采用任何项目的固定层级作为领域模型，也不把自动生成的 Persona、Scenario、Skill、图边或会话摘要写入正式知识。记忆候选要成为跨任务可复用事实，必须经过 Source & Knowledge、Review 或相应业务域的显式晋升流程。

### 3.6 成熟库与自研边界

不应把“保持事实边界”误解为“所有检索算法和存储驱动都自己实现”。ContentCloud 的策略是复用机制、掌握语义：

| 能力 | 首选方式 | 决策 |
| --- | --- | --- |
| 本地元数据、全文检索与 BM25 | SQLite FTS5，通过成熟 Go SQLite 驱动接入；实现前用跨平台 spike 比较 `modernc.org/sqlite` 与 `mattn/go-sqlite3` 的 FTS5、CGO、发布体积和恢复表现 | **采用成熟组件**；BM25 使用 FTS5 内建实现，不自研搜索引擎 |
| 文件变化通知 | 当前以查询/重建时的确定性扫描为默认；未来可用 `fsnotify` 作为加速信号，但启动、恢复和查询前仍以路径、摘要和范围做确定性校验 | **暂不引入运行时 watcher**；文件事件不是事实，避免为本地 CLI 增加常驻生命周期 |
| 向量索引 | `sqlite-vec` 可做隔离试验，但当前仍是 alpha，必须验证各发布平台的扩展装载、召回收益和损坏重建 | **暂缓进入核心依赖**；没有指标前保持 FTS/BM25 |
| Embedding 与 LLM 抽取 | 复用现有 Provider/Agent 执行契约，结果写入候选投影并记录模型、提示和来源版本 | **复用平台适配器**；不让模型直接写正式知识 |
| 融合与排序 | 优先使用 FTS5 排名；只有评测证明需要时才增加 RRF 或语义通道 | **少量领域编排自研**；不为一个公式引入完整框架 |
| Mem0 / TencentDB Agent Memory | 通过受控 HTTP、SDK 或 MCP 适配器做可替换试验，只返回带 ContentCloud 来源和 scope 的候选 | **可选集成**；不得成为权威存储或默认模型代理 |
| LangMem / Graphiti / Letta / Cognee | 当前均要求 Python 运行时及各自存储/Agent 体系 | **借鉴源码，不作为首版依赖**；有独立业务指标时再评估 sidecar |

依赖选型必须满足四个条件：离线或明确授权时可用；索引可从来源确定性重建；删除与权限收窄能够传播；停用依赖不会丢失业务事实。首版只实现一个内建后端，不预先搭建空泛的多 Provider 插件体系；等第二个后端通过评测后再抽取适配接口。

当前内建实现已经采用 `modernc.org/sqlite` 的纯 Go 驱动和 SQLite FTS5/BM25：索引位于工作区 `.contentcloud/cache/memory/index.sqlite3`，由 `internal/localworkspace` 扫描允许的文本目录并可随时重建。对于需要跨会话保留的明确摘要，`workspace memory remember` / `memory_remember` 将其写入 `40-work/memory/records/*.json`；记录绑定已允许来源、派生 workspace/project scope、来源摘要和权限模式，始终保持 `memory_candidate`，且可在删除索引后从文件重建。`memory consolidate` 对相同 `claim_key` 做确定性去重和冲突报告，冲突候选不进入默认召回；`memory promote` 要求已接受的本地 Evidence，并仅导入 `candidate` 知识页，不创建审批快照。`memory extract`、`remote-query` 和 `QueryMemoryWithEmbedding` 通过受控 HTTP/Embedding 适配器显式运行，回验 scope、来源 digest、状态和向量维度；网络或适配器失败不影响本地 FTS。CLI/MCP 只暴露受绑定工作区 scope，因此 TencentDB Agent Memory、Mem0 等仍不会成为默认存储或服务端依赖。

## 4. 不采用的方案

### 4.1 不把 Dify 或 ComfyUI 画布直接给客户

画布适合运营配置、调试和能力组合，不适合客户完成一次具体创作。客户需要看到“现在得到什么、下一步做什么”，而不是节点、变量和边。

### 4.2 不直接把 Camunda 或 Temporal 作为客户领域模型

它们适合解决运行可靠性和流程执行，但不会替 ContentCloud 定义人物原型、剧本、分镜、审批和资产复用。首阶段继续保持模块化单体，用边界和契约借鉴这些系统；是否引入外部 Runtime 由真实吞吐、恢复和运维成本决定。

### 4.3 不把 Agent Client 当作连接后的全能主控

Codex、Claude Code 和其他客户端可以是执行者之一。它们不能获得任务全局状态、批准权、任意工具、平台密钥或无限预算。客户只需对需要本地能力的项目完成一次连接和健康检查，具体节点由 Runtime 按能力绑定执行者；当前客户连接协议只发布 Codex，其他客户端必须先通过完整 bootstrap 契约才能进入客户面。

### 4.4 不把 Agent 记忆库当作第二套事实源

记忆召回的目标是减少重复读取和重复说明，不是建立一套能覆盖文件、数据库或人工决定的新真相。向量相似度、LLM 摘要、用户画像和历史成功经验都可能过期或误召回；它们只能作为带来源、范围、时效和信任标签的候选上下文。删除本地记忆索引后必须能从允许读取的文件和云端引用重建，且不得影响正式任务、审批和交付。

### 4.5 不因已有开源项目就整体嵌入

许可证允许复用不代表领域边界兼容。TencentDB Agent Memory 会引入 Team/User/Agent/Task 和统一资产控制面；Mem0、LangMem、Graphiti、Letta、Cognee 会引入 Python 运行时以及各自的 scope、存储或 Agent 模型。首版整体嵌入其中任一项目的运维和迁移成本，都高于复用 SQLite FTS5 等底层机制的收益。只有当隔离评测证明其召回质量或形成成本显著优于内建方案时，才增加受控适配器。

## 5. 对 ContentCloud 文档和代码的约束

1. 客户、运营和 Runtime 必须拥有独立的路由、权限、BFF、DTO 和信息架构。
2. `ExperienceTemplate`、`Published SOP` 和 `JobPlanRevision` 必须按版本发布，任务开始后不可静默改写。
3. 能力先声明，执行者后绑定；Codex、Claude Code、Worker、Provider 和人工都实现执行契约。
4. 工作区资料与创作结果使用独立 DTO；客户 BFF 可以组合查询，但不增加万能可空字段集合。
5. `confirmed` 和 `delivered` 是可复用门槛，不能与 `persona`、`script`、`storyboard`、`image`、`video` 等结果类型混为一轴。
6. 客户资产入口、当前任务输入和正式交付必须保持边界；资产入口内部也要清楚区分“我的资产”和“创作结果”。
7. Runtime、目录投影和客户投影都必须可重建、可对账并覆盖空、失败、陈旧和权限状态。
8. 本地工作区文件继续作为本地创作事实源；SQLite、全文和向量检索只能建立可重建的 Memory Projection。
9. 所有记忆召回必须带来源引用、作用域、摘要和时效信息，不能直接修改业务对象或通过人工 Gate。
10. Codex、Claude Code、WorkBuddy 等宿主通过 ContentCloud CLI/MCP 或受控适配器使用记忆能力，不直接依赖某个第三方宿主 Hook。

## 6. Agent Memory 源码基线

本轮结论基于 2026-08-09 克隆的以下提交，而不只依据项目首页：

| 项目 | 分支 | 提交 | 许可证 |
| --- | --- | --- | --- |
| TencentDB-Agent-Memory | `feat/server_team` | `fe3230f176f1` | MIT |
| mem0 | `main` | `4debc58a8337` | Apache-2.0 |
| letta | `main` | `ff19ffeafeb5` | Apache-2.0 |
| langmem | `main` | `7c7ebf36b5e1` | MIT |
| graphiti | `main` | `425bf2481b51` | Apache-2.0 |
| cognee | `main` | `a148eab58eb2` | Apache-2.0 |

### 6.1 关键源码证据

- **TencentDB Agent Memory：** `MemoryCore/src/metadata/types.ts` 定义四类 Asset、Team/User/Agent/Task、visibility、ACL 与 binding；`MemoryCore/src/metadata/service/permission-checker.ts` 实现 owner、membership、role default 和 ACL 判权；`MemoryProxy/src/skill/skill-bridge.ts` 强制注入会话身份并限制 LLM 可达端点与写操作；`MemoryKnowledge/src/db/schema.ts` 把 service/team/owner/user/agent/task/version/status 写入 Wiki 与 CodeGraph Schema。
- **Mem0：** `mem0/memory/main.py` 在初始化时约束 `user_id/agent_id/run_id` scope，在 `add` 中检索旧记忆后做 LLM 抽取，并在搜索阶段融合语义、关键词与实体信号；`mem0/memory/storage.py` 保存 ADD/UPDATE/DELETE 的 old/new history。
- **LangMem：** `src/langmem/knowledge/extraction.py` 实现 namespace、稳定 key、先检索后 consolidate 及最终 `put/delete`；`src/langmem/knowledge/tools.py` 暴露显式 create/update/delete；`src/langmem/reflection.py` 把形成任务放入后台队列并取消同一 thread 的旧 pending task。
- **Graphiti：** `graphiti_core/nodes.py` 的 Episode 保存 raw content、source 与 valid time；`graphiti_core/edges.py` 的事实边保存 episode 引用和有效/失效/过期时间；`graphiti_core/utils/maintenance/edge_operations.py` 失效冲突旧事实；`graphiti_core/search/search.py` 与 `search_config_recipes.py` 组合 group scope、多对象检索、BM25、cosine、BFS 和重排。
- **Letta：** `letta/schemas/block.py` 与 `letta/orm/block.py` 定义 char limit、project、read-only 和乐观锁版本；`letta/orm/block_history.py` 保存 actor 与 sequence history；`letta/services/block_manager_git.py` 明确 Git source of truth 与 PostgreSQL cache；`letta/services/context_window_calculator/context_window_calculator.py` 分项计算上下文预算。
- **Cognee：** `cognee/api/v1/remember/remember.py` 分开 Session 与知识 ingest；`cognee/infrastructure/session/session_manager.py` 以 dataset 隔离 Session；`cognee/modules/session_distillation/distill.py` 筛选候选后写入带来源 Markdown；`cognee/modules/users/permissions/methods/get_all_user_permission_datasets.py` 执行 user/tenant/role 过滤；`cognee/modules/graph/methods/try_delete_data_by_graph_provenance.py` 按来源删除图投影。

## 7. 官方参考

- [Camunda Web Modeler](https://docs.camunda.io/docs/components/modeler/web-modeler/)
- [Camunda Job Workers](https://docs.camunda.io/docs/components/concepts/job-workers/)
- [Camunda Tasklist](https://docs.camunda.io/docs/components/tasklist/introduction-to-tasklist/)
- [Camunda Operate](https://docs.camunda.io/docs/components/operate/operate-introduction/)
- [Dify Workflow Web Apps](https://docs.dify.ai/en/cloud/use-dify/publish/webapp/workflow-webapp)
- [Dify Workflow & Chatflow](https://docs.dify.ai/en/cloud/use-dify/build/workflow-chatflow)
- [Dify Human Input](https://docs.dify.ai/en/cloud/use-dify/nodes/human-input)
- [Temporal Workflow Execution](https://docs.temporal.io/workflow-execution)
- [LangGraph Persistence](https://docs.langchain.com/oss/python/langgraph/durable-execution)
- [TencentDB Agent Memory](https://github.com/TencentCloud/TencentDB-Agent-Memory)
- [Mem0](https://github.com/mem0ai/mem0)
- [LangMem](https://github.com/langchain-ai/langmem)
- [Graphiti](https://github.com/getzep/graphiti)
- [Letta](https://github.com/letta-ai/letta)
- [Cognee](https://github.com/topoteretes/cognee)
- [Adobe GenStudio Create](https://experienceleague.adobe.com/en/docs/genstudio-for-performance-marketing/user-guide/create/overview)
- [Adobe GenStudio Assets and Experiences](https://experienceleague.adobe.com/en/docs/genstudio-for-performance-marketing/user-guide/content/manage-assets)
- [Adobe GenStudio Asset Details](https://experienceleague.adobe.com/en/docs/genstudio-for-performance-marketing/user-guide/content/asset-details)
- [Runway Asset Organization](https://help.runwayml.com/hc/en-us/articles/23998498329107-How-to-organize-assets)
- [Runway Asset Lineage](https://help.runwayml.com/hc/en-us/articles/53718574533395-Viewing-and-downloading-an-asset-s-lineage)
- [Frame.io Version Stacking](https://help.frame.io/en/articles/9101068-version-stacking)
