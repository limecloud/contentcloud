# 05：共享状态、上下文与故障恢复

> 阅读对象：状态存储、智能体上下文和故障恢复模块研发人员。业务读者可先看 README 中“多个步骤共用一套可靠记录”和“中断后从确认完成的位置继续”两节。本文约定：`WorkTask`=业务任务、`JobRun`=执行实例、`NodeRun`=执行步骤、`RuntimeAttempt`=执行尝试；跨步骤资料称为共享状态，`human_gate` 是人工审批节点。

## 1. 目标

V8 不会把数据库账号直接交给智能体，也不会用一份大型 JSON 或 Markdown 文档保存全部共享状态。系统将提供一层范围有限、规则明确的运行时数据接口：

```text
智能体 / 工作进程
   |
   | 本次执行专用的 MCP / 运行时网关
   v
命令校验
   |
   +--> PostgreSQL 事务
   |      StateMutation + 当前值 + JobEvent + 节点输出
   |
   +--> 对象存储
          大段正文 / 媒体 / 完整对话记录 / Artifact
```

目标是让多个智能体可靠协作，同时由服务端统一控制数据格式（Schema）、版本、权限、预算和审计。

### 1.1 本地 Workspace 与 Desktop 缓存

交互式本地工作区的未提交文件和草稿由 Local Workspace 负责；Desktop 通过 Go Daemon 观察目录、计算 digest、维护同步 outbox 和事件游标。SQLite/FTS5 只保存可重建索引、缓存、上传分片和游标，不能成为 `StateCollection`、Cloud Revision 或审批的第三事实源。Codex 通过 stdio MCP 进入同一 Kernel，Desktop 通过 typed local API 读取专用 View。

## 2. 共享状态集合（`StateCollection`）

### 2.1 定义

每个集合在创建时固定：

| 字段 | 说明 |
| --- | --- |
| `job_run_id` / `branch_id` | 所属执行实例及其分支；不能由工具调用者自行指定 |
| `collection_key` | 同一执行实例内的稳定名称，例如 `audience_candidates` |
| `schema_id` / `schema_revision` | 已发布的 JSON 数据格式或领域数据格式 |
| `scope` | 执行实例、分支或步骤私有范围：`job`、`branch`、`node_private` |
| `read_policy` / `write_policy` | 角色、节点类型和字段级限制 |
| `consistency` | 单写入方、只追加、逐键 CAS 或归并节点专写 |
| `max_record_bytes` / `max_records` | 防止状态膨胀 |
| `retention_policy` | 执行实例结束后的保留、归档或删除规则 |
| `created_by_plan_revision` | 产生集合的 `JobPlanRevision` |
| `parent_collection_id` / `parent_watermark` | 仅分支继承时填写；指向源执行实例中不可变的读取边界 |
| `inheritance_mode` | `owned`、`snapshot_read_only` 或 `copy_on_write` |

智能体不能自行发布生产数据格式。当前 Runtime Schema Registry 持久化 `draft -> published -> retired` 生命周期、定义摘要、兼容策略和 `job/30d/90d/forever` 保留策略；发布新 revision 时检查前一版本已发布，并拒绝根类型变化、既有字段类型变化和新增必填字段。`StateCollection` 只接受已发布的固定 `schema_id/schema_revision`。

### 2.2 写入模式

| 模式 | 允许操作 | 用例 |
| --- | --- | --- |
| `single_writer` | 指定节点对当前版本执行 CAS 写入 | 营销活动需求、最终方向 |
| `append_only` | 带重复提交保护地追加，不修改旧记录 | 发现结果、候选项、审计观察 |
| `cas_map` | 每个键独立执行 CAS 写入 | 各受众的处理进度 |
| `reducer_owned` | 只允许声明的汇聚节点或归并器写入 | 并行结果汇总和排名 |

系统不提供通用的自动合并。需要合并时，必须使用已经发布的确定性归并器，并对输入集合版本和输出摘要进行测试。

### 2.3 分支继承与隔离

从检查点创建新的执行实例时，系统按检查点记录的 `state_watermarks` 解析可继承的共享状态；绝不把源执行实例的“最新状态”作为分支输入。每个集合必须在计划中声明以下一种继承方式：

- `owned`：只属于当前执行实例，不继承；新分支需要时创建自己的空集合。
- `snapshot_read_only`：分支只能读取 `parent_collection_id@parent_watermark`，不能写入，也不能随源执行实例之后的更新变化。
- `copy_on_write`：分支先按父集合冻结水位读取；第一次写入时，服务端在分支自己的集合中创建覆盖记录。后续读取按“分支覆盖记录优先、父集合冻结版本兜底”解析，所有 `StateMutation` 只写入分支集合。

`job` 范围的集合在分支中最多以 `snapshot_read_only` 或 `copy_on_write` 方式继承；`branch` 范围的集合必须归新分支所有；`node_private` 不自动继承。数据格式、读取策略、保留策略或内容权利不再兼容时，分支准入失败。服务端必须拒绝新分支使用源 `job_run_id` 写入共享状态，并把父集合、父水位和覆盖来源写入审计事件。

### 2.4 StateRecord 与 StateMutation

```text
StateRecord
  collection_id / key
  value_json or artifact_ref
  schema_revision
  version / digest
  created_by / updated_by

StateMutation (append-only)
  mutation_id / operation
  expected_version / resulting_version
  value_digest / patch_digest
  node_run_id / attempt_id / agent_instance_id
  idempotency_key / committed_at
```

同一个重复提交保护键（幂等键）和相同请求摘要，应返回首次处理结果；键相同但请求内容不同则返回冲突。比较并交换（CAS）写入失败后，智能体必须重新读取最新版本，系统不得让后一次写入直接覆盖前一次结果。

## 3. 运行时网关

Codex 和 Claude Code 都支持 MCP，因此运行时网关以 MCP 工具契约为基础。当前设备鉴权入口是 `runtime.worker.mcp`；服务端再按 Attempt fence、租约、Agent、ContextView allowlist 和 StateRefs 逐级收窄权限，并把调用作为 ToolCall 状态记录。stdio/Streamable HTTP 宿主桥接仍需在线 smoke。

### 3.1 最小工具集

| 工具 | 语义 | 权限 |
| --- | --- | --- |
| `state.get` | 按集合、键和版本读取一条记录 | `ContextView` 允许读取的集合 |
| `state.query` | 分页查询并返回版本和内容摘要 | 只允许预先声明的筛选和排序方式 |
| `state.mutate` | 按预期版本写入或创建记录；append-only 集合由集合策略拒绝覆盖 | ContextView StateRefs 和集合写入策略 |
| `child.list` | 查看直接子节点的结构化状态 | 不返回同级节点的完整对话记录 |
| `effect.prepare` | 声明外部操作意图 | 节点策略和授权结果 |
| `effect.status` | 查询或核对已声明的外部操作 | 当前外部操作范围 |

当前不提供 `child.propose`、`artifact.resolve`、`job.wait`、`progress.emit` 或 `checkpoint.request` 的 MCP 入口；GraphPatch 仍由服务端授权命令调用，等 Agent 后代预算与 GraphPatch 能在同一事务扣减后才开放给 MCP。已有等待、进度和检查点能力仍通过 Runtime 命令使用。明确禁止 `sql.execute`、`schema.ddl`、任意 URL 抓取（`URL fetch`）、修改预算、通过人工审批节点、直接提交 Provider 或直接完成执行步骤。

### 3.2 授权

每次工具调用的权限都从短期 `Attempt` 凭据逐级推导：

```text
tenant -> project -> job -> node -> attempt -> agent
```

请求正文中的 ID 只能进一步缩小权限范围，不能扩大范围。凭据到期、租约失效、执行实例进入取消状态或 `ContextView` 被撤回后，写操作必须默认拒绝。

## 4. 上下文视图（`ContextView`）

### 4.1 为什么一味扩大上下文仍然不够

更长的模型输入和自动上下文压缩（`compaction`）只能缓解输入长度问题，不能提供：

- 并发写隔离和 CAS。
- 精确的业务版本、审批状态和对象摘要。
- 确保不同智能体之间的最小权限隔离。
- 可重复的恢复输入。
- 外部操作的实际状态。

因此，每次启动或恢复智能体时，都要生成一份不可变的 `ContextView`。

### 4.2 数据流

```text
JobPlan + NodeSpec
        |
        +---- 输入选择器 ---------> 正式业务对象 / Artifact 引用
        |
        +---- 状态选择器 ---------> StateCollection@watermark
        |
        +---- 事件选择器 ---------> 最近的 JobEvent 摘要
        |
        +---- 记忆选择器 ---------> 可追溯的本地/会话记忆候选
        |
        +---- 策略 ---------------> 工具 / 模型 / 预算 / 数据披露
        |
        v
上下文构建器
  -> 按固定优先级，在 Token 上限内取数
  -> 生成带来源引用的脱敏摘要
  -> 生成 ContextView 清单和摘要
        |
        v
智能体本次执行
```

### 4.3 上下文视图包含什么

- 系统和运行时指令的版本与内容摘要。
- `TaskContract`、`NodeSpec`、输出数据格式和完成条件。
- 精确的来源（Source）、知识（Knowledge）、产物（Artifact）引用、版本和内容摘要。
- `StateRecord` 选择结果及读取水位。
- 直接子节点的状态和结构化摘要。
- 最近相关 JobEvent，而不是整个事件流。
- 在权限、时效和 Token 预算内召回的记忆候选，以及每条候选的来源引用和生成摘要。
- 允许使用的 MCP 工具、调用预算和截止时间。
- 数据来源标签：`trusted_policy`、`canonical_fact`、`memory_candidate`、`agent_generated`、`untrusted_external`。

大段正文和媒体不直接放入上下文，由工具按权限分页读取。`ContextView.digest` 与 `Attempt` 绑定，结果上报时由服务端校验，避免输入已经变化后，智能体仍用旧资料提交正式结果。

### 4.4 上下文容量

确定性优先级：

```text
系统策略 / 任务契约 / 审批条件
  > 必需的正式输入
  > 尚未完成的子节点和外部操作摘要
  > 最近的相关事件
  > 可选示例和历史记录
```

超过预算时：

1. 移除可选数据。
2. 使用带来源引用的确定性摘要。
3. 提供分页查询工具。
4. 仍然超限时，对应执行步骤进入 `waiting_input`；没有其他可运行执行步骤时，执行实例进入 `waiting(input_too_large)`。不得静默截断人工审批节点条件、素材权利或任务契约。

## 5. 完整对话记录与业务状态分离

Codex 线程和 Claude 会话可以保存并恢复对话，但完整对话记录只用于观察执行过程：

- 默认只保存会话引用、模型、用量、工具事件和脱敏摘要。
- 是否导出完整对话记录由租户数据策略决定，导出后存入受限对象存储。
- 对话压缩不会改变 `StateCollection`、`JobEvent` 或正式内容数据。
- 智能体会话恢复失败时，可以根据 `ContextView` 创建新会话；`JobRun` 不会因此丢失。
- 宿主会话分支不会自动创建业务执行分支，反过来也一样。

Claude SessionStore 或 Codex thread storage 可以作为适配器保存对话记录的后端，但不能替代 ContentCloud 的状态事务。

### 5.1 为什么仍然需要记忆层

权威状态解决“现在真实发生了什么”，记忆层解决“过去哪些信息可能帮助当前执行”。只有状态而没有记忆，智能体会在每个新会话重复读取大量文件、日志和历史结果；只有记忆而没有权威状态，则会把过期摘要、模型判断或错误召回误当成业务事实。两者必须分离。

首版按用途区分四类记忆，不直接复制通用 Agent 产品的固定层级名称：

| 记忆类型 | 主要来源 | 用途 | 不得替代 |
| --- | --- | --- | --- |
| 工作记忆 | 当前 `ContextView`、`LocalRun`、`Handoff` | 继续当前任务、恢复未完成步骤 | `JobRun`、`NodeRun`、`StateCollection` |
| 执行记忆 | 历史运行摘要、工具结果引用、错误与恢复记录 | 避免重复尝试、定位相似故障 | `JobEvent`、`RuntimeAttempt`、`Effect` |
| 知识记忆 | 来源文件、证据、知识页面和已批准知识引用 | 召回项目背景、事实候选和方法 | `KnowledgeObject`、`KnowledgeSnapshot`、权利记录 |
| 交互记忆 | 策略允许的会话摘要、明确偏好和交接说明 | 跨会话减少重复说明 | 用户身份事实、授权、审批决定 |

记忆类型表达召回用途，不建立新的业务聚合。一个记忆条目可以引用多个权威对象，但不能反向拥有或改写这些对象。

### 5.2 本地文件、索引与云端事实

本地交互式工作区采用“文件事实源 + 可重建索引”，云端治理继续采用 PostgreSQL/Blob 权威事实：

```text
本地工作区文件                         云端 PostgreSQL / Blob
候选 / LocalRun / Handoff              版本 / 审批 / Runtime / 审计
          |                                      |
          +-------------- 引用与摘要 ------------+
                             |
                             v
                   本地 Memory Projection
               SQLite FTS5 / BM25（首阶段）
                  向量索引（按评测可选）
                  Markdown/JSON 摘要（可读）
                             |
                             v
                  受范围和预算限制的召回
                             |
                             v
                 workspace_context / ContextView
```

- 工作区文件保存需要人工检查、Agent 修改和跨工具迁移的本地正文；索引数据库不保存任何无法从文件或明确云端引用恢复的唯一事实。
- SQLite、全文索引、分词结果、Embedding、排序分数和聚合摘要都是派生数据。索引应位于实现定义的缓存目录，不进入提交包、批准摘要或跨设备同步事实。
- 索引必须记录源类型、稳定引用、内容摘要、作用域、信任标签、生成器版本和更新时间。源摘要变化、来源删除、权限收窄或保留期到期后，相应条目必须失效或删除。
- 检索可以使用关键词、BM25、向量或确定性融合，但最终返回必须携带来源引用；相似度分数不能提升事实可信度。
- Embedding 或摘要调用远程模型时，数据披露范围必须受环境清单和租户策略约束；没有授权时降级为本地解析或关键词检索。
- 索引损坏、丢失或版本不兼容时，系统应降级到文件扫描或明确阻断索引能力，并提供确定性重建；不得阻断对权威文件的直接读取。

### 5.3 记忆形成、召回与晋升

```text
允许读取的文件 / 对象 / 会话事件
         |                         |
         | 确定性热路径            | 可选后台形成
         v                         v
摘要校验 / 失效 / FTS 写入     LLM 提取 / 去重 / 冲突识别
         |                         |
         +------------+------------+
                      v
             Memory Projection（可重建）
                      |
          query + derived scope + freshness
                      v
               带引用的候选上下文
                      |
              人工或领域工作流确认
                      v
 KnowledgeSnapshot / SubmissionRevision / 其他正式事实
```

形成记忆不代表晋升为业务知识。需要跨任务正式复用的事实、主张、品牌规则、权利信息或内容，仍须进入对应业务域的验证、版本化和批准流程。未经晋升的记忆只能作为 `memory_candidate` 注入，并排在任务契约、批准条件和必需正式输入之后。

默认不把模型隐藏推理、密钥、完整凭据、永久下载地址或未获策略许可的完整对话写入记忆。记忆删除和保留策略必须同时覆盖可读摘要、原始引用、全文索引和向量索引，避免只删一层后仍可从另一层召回。

每个可召回条目至少满足以下契约：

| 字段 | 约束 |
| --- | --- |
| `memory_id` / `kind` | 派生条目的稳定标识及用途类型，不充当业务对象 ID |
| `scope` | 从已绑定 workspace、tenant、project、job、node 或 attempt 推导；调用者只能缩小，不能自行扩大 |
| `source_refs[]` | 指向文件路径或云端对象精确版本，并记录当时的内容摘要 |
| `summary` / `payload_ref` | 可召回摘要或派生内容；不得保存无法从来源恢复的唯一正文 |
| `observed_at` / `valid_from` / `valid_until` | 区分何时观察到、何时有效和何时失效，未知值不得由模型猜测 |
| `trust` / `status` | 至少区分 `active`、`stale`、`conflicted`、`tombstoned` 以及正式事实/候选/外部不可信来源 |
| `formed_by` | 确定性解析器或模型、提示、Schema 和实现版本 |
| `updated_at` / `expires_at` | 支持新鲜度过滤、保留策略和确定性重建 |

同一主张出现冲突时，不按“最后写入”直接覆盖。系统保留各自来源和有效区间，把无法确定的新旧关系标记为 `conflicted`，召回时同时披露冲突；只有权威来源版本或显式审核才能结束冲突。来源文件摘要变化时旧条目进入 `stale`；来源被删除或权限不再允许时，条目进入 `tombstoned` 并从所有可查询索引移除。后台队列失败只影响新候选形成，不能阻断确定性全文索引或权威文件读取。

### 5.4 成熟组件采用策略

ContentCloud 不自研数据库、全文搜索、文件监听或模型 SDK，但必须自己维护上述条目契约、范围推导、失效传播、晋升和审计，因为这些规则来自 ContentCloud 的领域对象和权限模型。

| 层次 | 候选组件 | 首阶段处理 |
| --- | --- | --- |
| SQLite 与 FTS5 | 在实现 spike 中比较 `modernc.org/sqlite` 与 `mattn/go-sqlite3`，验证 FTS5、中文查询、CGO、跨平台发布、数据库损坏和重建 | 选择一个 Go 驱动；直接使用 FTS5/BM25，不自研搜索引擎 |
| 文件变化通知 | `github.com/fsnotify/fsnotify` | 可用于增量触发，但恢复和查询仍校验路径与 SHA-256，避免丢事件造成陈旧结果 |
| 向量检索 | `sqlite-vec` | 仅做 feature flag 后的隔离评测；其 alpha 状态和原生扩展分发尚不满足默认依赖要求 |
| 候选形成 | ContentCloud 已有 Agent/Provider 执行契约 | 后台调用并记录模型与输入版本；无授权或调用失败时保持 FTS 降级路径 |
| 完整记忆框架 | Mem0 或 TencentDB Agent Memory 的自托管 API/SDK/MCP | 只允许作为可替换试验适配器；结果重新包装为 ContentCloud 记忆条目，不接管事实、权限和模型入口 |
| Python Agent/Graph 框架 | LangMem、Graphiti、Letta、Cognee | 首阶段只借鉴形成、时间和晋升模式，不引入 Python sidecar、图数据库或第二套 Agent Runtime |

不要为了未来可能切换后端预先设计复杂插件体系。首个内建实现先稳定 `MemoryEntry`、`MemoryQuery` 和 `MemoryResult` 的调用契约；只有第二个后端通过相同语料、权限和删除传播测试后，才提取真正需要的适配接口。

### 5.5 分阶段交付

| 优先级 | 交付内容 | 验收重点 |
| --- | --- | --- |
| P0 | 记忆条目契约；来源引用与摘要；scope 推导；新鲜度、信任和状态；来源变化、删除与权限收窄的失效传播；`ContextView` 的引用化、预算化召回 | 删除整个索引仍能工作；越权参数不能扩大 scope；陈旧或已删除来源不可召回 |
| P1 | 本地 SQLite FTS5/BM25 可重建索引；工作、执行、知识、交互四种用途；确定性重建和诊断 | 中英文固定语料的召回基线；跨平台 CLI 构建；损坏后恢复；无网络降级 |
| P2a | 显式、可审计的本地记忆候选记录；来源 digest/权限模式绑定；稳定 ID 的幂等写入和不可变冲突检测；CLI/MCP 入口 | 记录不脱离已绑定 scope；来源变化后 stale；清除索引后记录仍可重建；不会自动晋升为正式知识 |
| P2 | 后台抽取、去重、冲突和显式晋升；经评测后可选 Embedding 与融合检索；Mem0/TencentDB 适配器试验 | 与 P1 盲测比较召回质量、延迟、成本和权限正确性，未达到阈值不进入默认路径 |
| 暂缓 | 图数据库、统一 Memory Asset、独立 MemoryPanel、透明 LLM Proxy | 只有出现无法由现有领域和检索契约解决的明确需求时重新立项 |

### 5.6 当前落地状态（2026-08-09）

P0/P1 以及 P2a 的本地确定性部分已在 CLI/MCP 实现：

- `internal/local/workspace` 定义 `MemoryEntry`、`MemoryQuery`、来源摘要、派生 scope、信任/状态和字符预算契约。
- `modernc.org/sqlite` 提供纯 Go SQLite；`memory_fts` 使用 FTS5 trigram tokenizer 和 BM25 排序，投影位于 `.contentcloud/cache/memory/index.sqlite3`，不进入工作区事实提交。
- `workspace memory status|rebuild|query|clear` 以及 `memory_status`、`memory_rebuild`、`memory_query` MCP 工具均在本地运行。重建不上传正文，查询范围从已绑定 `workspace_id/project_id` 推导。`memory_extract`、`memory_remote_query` 是额外的显式远程适配器工具：只有调用方提供 endpoint 才会联网，抽取单次正文总量限制为 8 MiB，返回候选必须经过本地 scope、来源 digest、trust、status 和结构字段回验，远程不可用时不影响本地 FTS。
- `workspace memory remember` 和 `memory_remember` 将明确摘要保存到 `40-work/memory/records/*.json`，记录 `source_ref`、来源 digest、权限模式、scope、`formed_by` 和 `observed_at`；相同 ID 内容不可变，记录只作为 `memory_candidate`。
- 每次查询先校验来源文件摘要和索引元数据；来源变化、删除、权限或索引损坏会标记状态并回退到当前文件扫描。删除索引不会影响工作区文件或云端 PostgreSQL/Blob 权威事实。

P2 的实现路径已经具备，但默认热路径仍保持本地确定性：`memory consolidate` 负责重复/冲突报告并阻断冲突召回，`memory promote` 复用本地 Evidence 和 `knowledge-candidates/1.0` 导入链且只生成 `candidate`，`memory extract`、`remote-query` 与 `QueryMemoryWithEmbedding` 作为显式受控适配器调用。LLM、Embedding、Mem0/TencentDB 的真实部署仍需按同一语料、延迟、成本和权限正确性做独立评测；在评测完成前不改变 `workspace_context` 的默认 FTS 路径。

## 6. 检查点（`Checkpoint`）

### 6.1 定义

`Checkpoint` 是在安全执行边界记录的一组逻辑状态引用：

```text
Checkpoint
  job_run_id / checkpoint_no / created_at
  plan_revision_id / graph_version / plan_digest
  node_status_manifest_digest
  state_watermarks[] / state_digests[]
  event_cursor
  output_artifact_refs[]
  side_effect_watermark
  context_summary_refs[]
  parent_checkpoint_id
  reason / created_by
```

不包含：

- OS 进程、内存、goroutine、文件描述符或网络连接。
- 活跃 lease、fence token、Device token、Secret 或临时签名 URL。
- 浏览器句柄和未登记的服务商客户端状态。
- 模型隐藏推理或未经策略允许的完整 Transcript。

### 6.2 安全创建点

首版只在以下边界创建：

- 执行实例准入成功后。
- 节点输出、`StateMutation`、`JobEvent` 和 `SideEffectRecord` 状态已经原子提交后。
- 主控智能体让出执行资源前。
- 人工审批节点（`human_gate`）通过后。
- 操作员显式请求且当前没有未解决的事务提交。

正在运行的 `Attempt` 不能直接“快照”；它必须先完成、失败、过期或结束本次执行并让出资源。

## 7. 故障恢复、重放与创建执行分支

```text
                   常规故障恢复
JobRun + 已持久化的最新事件 --------------------> 原 JobRun 继续执行
        |
        | 重放状态投影，不执行任务
        +----------------------------------------> 重建读模型
        |
        | 从 Checkpoint 明确创建分支
        v
新 JobRun（共享不可变引用，采用新策略并重新准入）
```

### 7.1 故障恢复

- 租约过期后，将 `NodeRun` 送回 `ready` 或 `retry_wait`，再创建新的 `Attempt`。
- 守护进程日志（Daemon journal）重新上报时使用原重复提交保护键（`idempotency key`），服务端返回已经提交的结果。
- 智能体会话可以恢复时继续原会话；无法恢复时根据 `ContextView` 创建新会话。
- 恢复只前进，不撤销已经提交的状态或业务事实。

### 7.2 状态重放（`Replay`）

`Replay` 只根据检查点、执行事件和权威状态表重建读模型与诊断信息，不重新执行任何业务操作：

- 不调用 LLM、服务商、MCP 工具或外部 API。
- 不重新发送通知、上传或发布。
- 发现事件游标、状态摘要和读模型不一致时，停止重放并报告数据损坏。

### 7.3 创建新的执行分支

创建执行分支时生成新的 `JobRun`（执行实例）：

- 复用不可变的输入、正式产物引用，以及按检查点水位冻结、并且策略允许共享的状态快照。
- 写入新的根分支/父分支关系、`RuntimePolicy`、`JobPlanRevision` 和幂等作用域。
- 重新验证素材权利、Offer 有效期、人工审批节点、服务商配置、模型和预算。
- 原 `JobRun` 保持不变；新分支只记录后续新增或变更的状态，并按集合继承模式在分支自己的共享状态中写入，不能覆盖源执行实例。
- 旧分支已经生效的外部操作只作为历史引用，不会在新分支自动再次执行。

“回滚到检查点”在用户界面中必须显示为“从这里创建分支”，不能暗示外部世界被倒带。

## 8. 工具调用与外部操作

### 8.1 ToolCall

每次可观测工具调用记录：

```text
tool_call_id / tool_name / tool_schema_version
job/node/attempt/agent/session refs
request_digest / safe_request_summary
state: proposed | authorized | running | succeeded | failed | unknown
started_at / finished_at / usage / error_code
```

读操作和纯计算可以只记录 `ToolCall`；会改变外部状态、产生费用或产生不可逆结果的调用，必须先创建外部操作记录（`SideEffectRecord`）。

### 8.2 外部操作状态机

```text
planned -> authorized -> executing -> committed
                         |     |
                         |     +--> unknown -> reconciling
                         |                       |       |
                         |                       v       v
                         |                    committed failed
                         |
                         +--> failed -> retry_wait -> authorized

committed -> compensation_requested -> compensating
                                      |             |
                                      v             v
                                compensated  compensation_failed
```

### 8.3 结果不明

以下情况应进入结果不明状态（`unknown`），而不是直接判定失败：

- 向服务商提交请求时超时，无法确认外部任务是否已经创建。
- 外部系统返回成功后，本地在保存外部 ID 前崩溃。
- 文件上传连接断开，远端可能已经完成。
- 回调签名有效但本地缺少对应 Attempt 的最终状态。

处理流程：

```text
unknown
  -> 使用相同幂等键和请求摘要查询服务商
  -> 找到外部对象：绑定并进入 committed
  -> 服务商确认未创建：进入 failed/retry_wait
  -> 服务商无法查询或结果冲突：等待人工对账
```

处于 `unknown` 状态时，禁止生成新的幂等键再次提交。业务任务被取消也不改变这一规则：执行实例进入 `waiting(external_effect)`，只允许对账或登记人工对账结论；所有关联外部操作进入终态后，才回到 `canceling` 并完成取消。

### 8.4 补偿

补偿本身也是一条新的、可审计的外部操作，不能通过删除旧记录掩盖已经发生的动作。每类外部操作都要声明：

- 是否可补偿。
- 补偿时间窗口和所需的人工审批节点。
- 补偿操作的幂等键。
- 无法自动恢复时使用的人工运维手册。

例如，取消尚未开始生成的外部任务可能可以补偿；已经被外部用户下载的成片或已经发布的内容无法真正撤销，只能下架并保留历史。

## 9. 一致性边界

必须在一个 PostgreSQL 事务内完成：

- `StateMutation`、当前 `StateRecord` 投影和 `JobEvent`。
- 节点输出绑定、`NodeRun` 状态和 `JobEvent`。
- `GraphPatch`、新版本、`NodeSpec`/边和 `JobEvent`。
- `Attempt` 领取、`ResourceReservation`、租约和 `JobEvent`。
- 外部操作授权、费用预留和 `JobEvent`。

对象存储无法加入数据库事务时，使用发件箱/收件箱模式：先保存带内容摘要的待处理清单，再上传和校验，最后提交正式 `Artifact`。校验失败的二进制文件不能被正式输出引用。

## 10. 验收

- 两个智能体同时对同一版本执行 CAS 写入时，只有一个成功，另一个收到可诊断的版本冲突。
- 使用同一幂等键重复上报追加操作 100 次，只产生一条 `StateMutation`。
- 输入或状态水位发生变化后，`ContextView` 必须产生新的摘要；旧 `Attempt` 不能提交要求新上下文的输出。
- 进程在节点输出与状态提交之间的任意时刻被终止，都不能出现“节点成功但没有输出”或相反的不一致状态。
- 状态重放不触发任何智能体、外部服务或工具调用。
- 创建执行分支不修改原 `JobRun`；分支只能读取父集合的冻结水位，并在自己的 `copy_on_write` 覆盖集合中写入，同时重新校验权限、素材权利、费用和执行配置。
- 外部操作为 `unknown` 时禁止自动重试；对账成功后只能绑定一个外部对象。
- 状态、事件和完整对话记录中不得出现密钥、fence token、永久下载地址或超出策略允许范围的原始正文。
- 删除本地记忆索引后能够从允许读取的工作区文件和云端引用确定性重建，且重建前后相同查询的来源集合可解释。
- 源摘要、权限或保留策略发生变化后，陈旧记忆不再进入新 `ContextView`；任何召回结果都不能直接产生批准或 Runtime 状态变更。
