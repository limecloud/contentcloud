# 08：迁移、测试与验收

> 阅读对象：负责兼容迁移、测试和发布验收的研发人员。本文件记录的是实施门槛，不表示这些能力已经上线。

## 1. 当前基线对账

截至 2026-08-11：

- 当前版本为 `v0.24.0`。Runtime Infra V2 的 I1～I4 核心切片、I5 第二业务流容量边界和 provider-neutral HTTP/异步轮询恢复切片已进入当前工作区；每个工作包仍须独立运行完整验证，不能沿用历史结果。
- V7 的类型化 Stage 输出、媒体领域、MediaReview、最终 Artifact、DeliveryPackage 和 Web 投影已在 `v0.16.0/v0.17.0` 落地；工作区资料文件夹、上传和资料引用已在 `v0.18.0` 首次落地。
- V8 已落地 JobRun/NodeRun/JobEvent、独立 RuntimeAttempt、RuntimeCommandStore、事件/outbox 同事务、不可变 outbox + subscriber receipts、终态业务结果持久化消费、fence/资源预留账本、StateCollection/StateRecord CAS、ToolCall、Checkpoint watermark、Fork/Replay、ContextView/AgentInstance、FakeHarness 调度闭环、Codex CLI JSONL/thread resume Harness、Provider inbox/账单对账、Yield/Resume、Projector 和 Runtime Explorer 投影重建/dry-run；各文档必须继续区分已实现内核、离线协议测试与生产能力。
- 内置 SOP Registry 只接受完整的平台身份（固定 ID、`template_key`、`built_in`、`source_ref`）；按名称/形状认领旧短视频 SOP 和修复旧内置元数据的运行时迁移已删除，发现冲突时重建开发数据，不新增兼容分支。
- 根 `README.md`、平台基线、产品需求和 V8 路线图已互相指向；历史 V1-V7 路线图不再作为当前能力事实源。
- provider-neutral HTTP 适配器、签名/超时/SSRF 防护、异步 submit/status/cancel、到期轮询恢复、有上限流式下载、Runtime Effect 关联和 Provider callback/bill HMAC ingress 已有确定性 `httptest` 契约；未知提交不会自动重试；真实媒体服务商凭据、账单补偿演练、完整的媒体租约恢复和确定性后期处理仍未完成。
- Codex Runtime Harness 已使用官方 CLI JSONL 协议，保存 `thread.started` 的真实 thread ID，并通过 `codex exec resume <thread_id>` 在新的 Harness/worker 进程恢复；Claude Runtime Harness 已使用 `stream-json` 首事件 session ID 和 `--resume`，两者能力均在 worker 侧探测并固定到 Attempt，过程事件经 lease/fence/session 校验后只保存脱敏摘要。helper-process 测试不调用模型，真实在线 Codex/Claude smoke 尚未验收。

基线事实主要来自 `CHANGELOG.md`、`internal/domain`、`internal/runtime`、`internal/agentadapter`、`internal/mediapipeline`、Memory/PostgreSQL Store 以及迁移 `00012_v7_media_pipeline.sql`、`00014_agentic_job_runtime.sql`～`00043_runtime_tool_call_results.sql`。历史路线图只能作为背景，不能代替当前代码和测试结果。

V8 的第一个工作包必须先更新权威文档和能力登记表；不能在错误的基线上继续规划。

## 2. 迁移原则

1. **先增量再切换**：新增表、字段、索引和 API；只有迁移边界已切换且生产引用归零时，才删除旧表、旧 API 或旧目录；已经确认无消费者的 Runtime 旧结构应在迁移中直接删除，不保留双读壳。
2. **先旁路再成为权威**：先编译、比较和观测，不让新执行图直接决定生产调度。
3. **每类权威数据只保留一个写入者**：切换后明确 Job、Node、Stage 投影分别由谁写入，禁止长期并行维护两套状态机。
4. **不编造历史**：不为历史 Task 伪造 `JobEvent`、`Checkpoint`、`Artifact` 或服务商记录。
5. **由功能开关控制准入**：开关只决定新 Job 进入哪条路径；不能把已经运行的动态图降级成线性 Stage。
6. **优先向前恢复**：生产回退以停止新任务准入、排空、暂停或修复为主，不对已经写入数据的新表执行破坏性降级迁移。

本地 CLI 配置也遵循同一边界：`daemon_bindings` 是 current 唯一运行事实；`localconfig.Load()` 严格拒绝未知顶层字段，旧单工作区字段不再读取、重写、fallback 或双写，需要重新连接生成 current 配置。

SOP Registry 同样只认显式身份：同名、同结构或只有平台 ID 的不完整记录都不会被自动收编。新建任务私有 WorkspaceBinding 使用当前本地模板身份；`task_marketing_video` 等旧 source 标记不得再写入或恢复。

## 3. 数据迁移

迁移编号在实施时按仓库最新序列分配，逻辑顺序固定：

| 逻辑迁移 | 新增/变更 | 约束 |
| --- | --- | --- |
| MIG8-A | `job_runs`、`job_events` | RLS、租户和 WorkTask 范围内的创建幂等键唯一约束、同一项执行内的事件序列号、活动 WorkTask 的部分唯一索引 |
| MIG8-B | `job_plan_revisions`、`job_nodes`、`job_edges` | 版本不可变、计划摘要、不得建立跨 Job 外键 |
| MIG8-C | 独立 `runtime_node_runs`、`runtime_attempts` | 不复用 V7 `task_runs/run_attempts`；Node/Attempt/Context/Agent 使用同一租户、Job 和 Node 复合外键 |
| MIG8-D | `runtime_agent_instances`、`runtime_context_views` | 不透明会话引用只在受控读取路径中使用，运营投影必须脱敏，并限制父级范围 |
| MIG8-E | `state_collections`、`state_records`、`state_mutations` | 比较并交换（CAS）唯一版本、RLS、大小约束 |
| MIG8-F | `fanout_sets`、`fanout_members` | 成员集合封存摘要、确定性的子节点唯一键 |
| MIG8-G | `checkpoints` | 清单不可变、记录事件和状态水位线 |
| MIG8-H | `tool_calls`、`side_effects`、`resource_reservations` | 外部操作幂等、未知结果和对账、配额不变量 |
| MIG8-I | 服务商任务、Artifact 和输出增加兼容关联字段 | `side_effect_id`、`node_run_id` 可空，旧记录不回填猜测值 |

所有表都需要由 Memory Store 和 PostgreSQL Store 共同遵守同一份存储契约；迁移集成测试必须使用真实的行级安全策略（RLS）操作人上下文。

当前物理落地：`00014_agentic_job_runtime.sql`～`00020_runtime_append_only_permissions.sql` 建立 Runtime 基础表、ContextView/AgentInstance、RuntimeAttempt、outbox 和追加事实权限；`00021`～`00029` 增加 fence、资源账本、类型化状态、Provider 对账、Yield/Resume 和投影重建；`00031`～`00033` 冻结 JobRun 业务类型、输入快照和输出上限；`00034_remove_v7_execution.sql` 解开 JobRun 对单一 WorkTask 的物理外键，并删除 `task_runs/run_attempts/run_progress_events/creative_execution_bundles`；`00035_runtime_outbox_subscribers.sql` 将 outbox 收敛为不可变消息，并把投影与业务结果的投递状态迁到独立 subscriber receipts；Session Mirror 创建/删除迁移已从首个用户基线移除；`00037_runtime_maintenance_health.sql` 增加租户级 reaper/delivery 维护心跳及其 RLS；`00038_provider_poll_recovery.sql` 让异步 Provider 只按持久化 poll deadline 恢复，`00039_provider_poll_deadline.sql` 阻止缺失 deadline 的 unknown 提交进入重试循环；`00040_media_runtime_effect_links.sql` 为新媒体 Job/Attempt 增加可空 Runtime Job/Node/Attempt/Effect 关联，历史 V7 行保持未登记；`00041_runtime_schema_registry.sql` 增加租户隔离的 Schema draft/published/retired 生命周期和保留策略；`00042_runtime_read_pagination.sql` 增加 MCP 幂等唯一索引和 Runtime Explorer 读取索引；`00043_runtime_tool_call_results.sql` 为 ToolCall 增加受控 `safe_result`，保证成功幂等重放返回首次结果。迁移不使用 `CASCADE`，出现未识别依赖时整笔回滚。

## 4. 运行读模型与业务投影

### 4.1 WorkTask

- `intent`、输入、SOP、人工审批节点和正式内容关系仍来自 `WorkTask` 及现有领域对象。
- `latest_job_run_id` 和 `active_job_run_id` 通过读模型返回，首版不要求写回 `WorkTask` 表。
- `current_stage_id` 只作为当前关注阶段的投影，不再驱动 V8 调度器。
- 没有 JobRun 的 WorkTask 返回空运行投影，不补造执行历史。

### 4.2 StageRun

- 业务操作继续维护 StageRun；执行状态只来自 Runtime，不写第二套租约或终态。
- WorkTask `Runs` 只读取 Runtime JobRun/NodeRun 的单向业务投影。
- 尚未建立 JobRun 的任务返回空列表；读取投影不会反向补造历史。
- 投影器可以清空后重建；正式业务对象和运行时状态不能依赖投影器反向恢复。

### 4.3 RuntimeRun

- `RuntimeRun` / `RuntimeRunEvent` 是从 JobRun/NodeRun/JobEvent 生成的 current 只读模型；旧 `TaskRun` / `RunProgressEvent` DTO、`RunAttempt` 和旧执行表/API 已删除。
- NodeRun/RuntimeAttempt 使用唯一状态机；`runtime_job_runs` 的业务键不再强制外键到 WorkTask，因此知识提取等业务流不需要伪造 WorkTask。
- HTTP/BFF 新资源使用 NodeRun/RuntimeAttempt 术语；业务阶段兼容只通过单向投影完成。
- `RunProgress` 只投影 JobEvent；项目运行列表、Dashboard、ProjectProjection 和 lineage 只读取 Runtime。
- CLI daemon 与 `runtime-worker run` 共用 Runtime prepare/activate/heartbeat/finalize 协议，不再有独立 poll/report/outbox 状态机。
- Runtime worker 使用 `runtime.worker.prepare_next`、`runtime.worker.activate`、`runtime.worker.heartbeat`、`runtime.worker.event` 和 `runtime.worker.finalize` 协议；服务端从设备凭据派生 lease owner，远程请求只能携带 Attempt ID 与 fence token。成功结果先作为受控业务输出引用落 blob，再由业务 owner 校验并提交结构化结果。
- 知识提取以 `BusinessType=knowledge_extract` 创建 Runtime JobRun，冻结 `InputSnapshotID`、输出上限和证据契约；结果在终态前严格校验并写入内容寻址 Blob，终态后由独立 subscriber 幂等写入知识对象；重复候选包按包摘要和确定性知识对象 ID 幂等。

### 4.4 外部服务商

- 保留 `MediaGenerationJob` 和 `ProviderAttempt`。
- 新的服务商提交必须先关联外部操作记录；旧记录显示 `legacy_unledgered`，不推断外部幂等状态。
- 历史直通渲染（`passthrough render`）和 FakeProvider 结果保留原标签，不能在界面中升级为真实服务商生产证据。

## 5. 功能开关

建议按能力拆分开关，避免一个总开关同时改变数据、调度和界面：

```text
runtime_v8_shadow_compile
runtime_v8_job_run_admission
runtime_v8_linear_graph_scheduler
runtime_v8_state_gateway
runtime_v8_side_effect_ledger
runtime_v8_checkpoint_fork
runtime_v8_dynamic_graph
runtime_v8_harness_codex
runtime_v8_harness_claude
runtime_v8_campaign_pack
runtime_v8_explorer
```

每个开关都必须定义租户白名单、默认值、依赖关系和关闭行为。`dynamic_graph` 依赖线性调度器、共享状态网关、外部操作台账、检查点能力，以及至少一个通过一致性测试的智能体执行适配器。

## 6. 分阶段切换

```text
阶段 0  基线对账 + 契约 + 适配器概念验证
   |
阶段 1  旁路编译：SOP -> 执行图，不影响现有执行
   |
阶段 2  JobRun + 线性执行图调度器，保持已发布 SOP 的顺序语义
   |
阶段 3  共享状态网关 + ContextView + 主控智能体让出资源/恢复
   |
阶段 4  外部操作台账 + 可持久化的服务商状态机
   |
阶段 5  检查点/重放/分支执行
   |
阶段 6  受限的 GraphPatch + 并行拆分/汇聚
   |
阶段 7  营销活动标杆任务 + 运行诊断视图
   |
阶段 8  试点 / 稳定性压测 / 故障注入 / 生产门禁
```

动态能力在阶段 6 才打开。在此之前，所有运行时基础能力都可以先用线性图验证，降低一次上线过多新语义的风险。

## 7. 旁路编译验证

对每个已发布的 SOP 执行以下步骤：

1. 生成初始 `JobPlanRevision`，但不创建 JobRun 或节点租约。
2. 计算冻结基线中的 Stage `Order/InputRefs` 与执行边的对应关系。
3. 检查数据结构是否可达、人工审批节点是否完整，以及执行能力（`Capability`）是否匹配。
4. 将冻结基线的预期下一 Stage 与 Ready evaluator 的结果进行比较。
5. 只记录摘要、差异类别和错误码，不记录客户正文。

营销视频、文章和复盘的内置 SOP 连续通过后，才允许在小范围租户中启用线性执行图调度器。

## 8. 测试策略

### 8.1 领域与性质测试

- 覆盖 JobRun、NodeRun、RuntimeAttempt、AgentInstance、外部操作的完整状态转移矩阵。
- 验证随机有向无环图的无环性、拓扑可达性和 GraphPatch 只能追加的性质。
- 验证确定性的子节点键，以及相同 Patch/Mutation 的幂等性。
- 验证 FanoutSet 的成员集合封存和汇聚策略，覆盖零成员、部分失败、延迟上报和取消。
- 验证比较并交换（CAS）、单写入者、只追加和指定汇总写入者约束。
- 验证 ContextView 的选择、优先级、Token 预算和摘要稳定性。
- 已覆盖 StateCollection/StateRecord CAS、ToolCall 终态保护、Attempt-scoped MCP fence/allowlist/幂等、Schema draft/published/retired、Effect unknown/reconciling 禁止盲重试、Checkpoint watermark、Fork/Replay 零外部调用、Codex helper-process 真实 thread ID/新 Harness 实例 Resume、Provider inbox 去重/摘要冲突/unknown Effect/账单匹配与差异、Yield/Resume 等待条件和幂等，以及 Memory/PostgreSQL Fanout/Join 原子写入、幂等快照、quorum 取消、PostgreSQL 100 节点/20 worker 唯一领取、FairnessReport 和 50 节点第二业务流边界；真实 Provider Reconciler、在线 Codex/Claude/MCP 宿主演练、真实数据库提交后故障矩阵和多租户生产公平性仍待补。

### 8.2 存储与事务测试

- Memory Store 和 PostgreSQL Store 使用同一组存储契约测试数据，特别验证 Prepare/Activate/Heartbeat/Finalize 的原子边界。
- 使用 `FOR UPDATE SKIP LOCKED` 或 `ready + version CAS` 并发领取任务，不能出现双重租约、重复 RuntimeAttempt 或资源超卖。
- 验证 Node、RuntimeAttempt 和 AgentInstance 的联合租约过期收敛；旧 owner 和旧版本不能续租或提交结果。
- 验证相同终态结果摘要幂等成功，不同摘要返回冲突；事件序号在 Job 锁下连续分配。
- 验证 StateMutation、Event 和投影的原子性。
- 验证 StateRecord/ToolCall 命令与 Event/outbox 同事务，Projector 和业务结果 consumer 只 ack 各自 receipt，旧 projection 序号不能倒退；业务写成功但 ack 失败后必须可由新进程幂等恢复。
- 验证 GraphPatch 版本竞争时只能接受一个写入者。
- 验证 RLS 覆盖所有新表，并拦截跨租户或跨项目的外键攻击。
- 迁移集合已覆盖 `00021`～`00043`，并静态核对 fence、资源账本、State/ToolCall、Projection、Provider、Yield、Projection rebuild、outbox receipts、维护心跳、Provider poll deadline、媒体 Runtime Effect 关联、Schema Registry、Explorer 分页索引和 ToolCall `safe_result` 的约束及 session 镜像无 `CASCADE` 退役；专用 PostgreSQL 集成库已通过真实迁移、事务失败回滚、历史 Effect 空绑定、Provider/Resume 原子性、outbox subscriber receipt 隔离、Provider callback/bill ingress 和 Runtime 核心 RLS 越权测试（含投影重建、维护心跳）。代码已提供一次性 `after_commit` 故障钩子和幂等恢复集成用例；真实数据库故障环境和生产公平容量压测仍需专项验收。

### 8.3 执行适配器一致性测试

Fake、Codex 和 Claude 适配器使用同一套黑盒场景：

- 启动、事件流、结构化输出、取消和超时。
- MCP 读取、CAS、追加写入、过期令牌和越权工具调用。
- 进程被终止后恢复；Codex 已覆盖保存真实 thread ID 后由新 Harness 实例执行 `exec resume`，Claude 已覆盖保存真实 `session_id` 后由新 Harness 实例执行 `--resume`。能力快照声明 `resume=false` 或宿主拒绝恢复时，必须创建新 Attempt 并使用新的 ContextView 重新开始。
- 会话分支只影响智能体历史，不自动修改 JobRun。
- 上下文压缩后仍然遵守 TaskContract 和输出数据结构。
- 适配器版本或能力不匹配时默认拒绝执行。

CI 默认运行 FakeHarness 和不调用模型的 Codex CLI helper-process 协议测试；真实 Codex/Claude 冒烟测试必须在明确授权、低预算、非客户数据环境中执行，不能默认使用消费级登录来测试平台服务。

FakeHarness 的必测脚本包括：正常结构化结果、启动失败、启动成功但返回空事件流、重复相同/不同终态、未知事件、事件流无终态关闭、延迟超过一次心跳周期，以及租约到期。Codex 协议测试还必须覆盖首事件校验、thread ID 固定、Resume ID 一致、不同 Harness 实例恢复、结果文件和脱敏事件投影；即使租约尚未执行批量回收，过期 owner 提交的迟到事件或终态也必须被拒绝。

### 8.4 故障注入

| 注入点 | 预期结果 |
| --- | --- |
| 在 PrepareDispatch 提交前终止调度器 | 事务完整回滚，不产生 ContextView、Agent 绑定、RuntimeAttempt 或孤立租约 |
| 在 PrepareDispatch 提交后、Harness.Start 前终止调度器 | `prepared` Attempt 可诊断，并由租约回收原子释放 Node/Attempt/Agent |
| Harness.Start 成功后、ActivateDispatch 前终止调度器 | 尽力中断已知会话；无法清理时由租约回收，旧 worker 不得提交结果 |
| Harness.Start 返回空事件流 | Attempt 进入可重试失败，Node 和 Agent 释放执行权，不进入运行态 |
| 租约已到期但回收任务尚未运行 | 旧 worker 的终态提交返回 `DISPATCH_LEASE_STALE`；随后回收将 Attempt 置为 `expired` 并释放 Node/Agent |
| 终态事务提交后、worker 收到响应前终止 | 相同摘要重报返回幂等成功，不产生第二个终态事件 |
| Runtime 终态提交后、业务结果消费前终止 | 业务 receipt 保持 pending；新进程读取相同 output ref 并完成业务写入 |
| 业务对象写成功后、receipt ack 前终止 | 重新领取并幂等核对现有对象，不重复创建；不同 Blob 摘要拒绝并按上限退避重试 |
| 智能体启动前、工具调用中或输出后终止守护进程 | 租约可以恢复；已提交的 `mutation` 可以幂等重报 |
| 主控智能体提交 GraphPatch 时断网 | 相同键得到同一 Revision，或返回明确冲突 |
| 并行拆分创建到一半时终止进程 | 事务回滚，或按确定性子节点键补齐；不能有重复节点 |
| 服务商提交请求超时 | 外部操作变为 `unknown`，禁止盲目重提 |
| 回调重复、乱序或延迟到达 | 最终只保留一个终态，旧事件不能覆盖新状态 |
| 下载中断、文件过大或伪造 MIME | 不生成正式产物（`Artifact`）；及时释放流式资源 |
| 共享状态 CAS 冲突 | 一个请求成功，另一个返回可诊断的冲突 |
| 上下文构建超过预算 | 返回安全摘要或分页结果，或阻断执行；不能丢失审批和权利信息 |
| 事件投影器停止 | 调度状态继续保持一致；界面显示延迟，恢复后可重建投影 |
| 父 Job 取消时子智能体仍在运行 | 撤销租约，禁止新建子节点或外部操作，并保留已经提交的外部操作 |

### 8.5 安全测试

- 提示词注入测试用例尝试调用未授权工具、修改预算、跨同级步骤读取状态和自行通过人工审批节点。
- 扫描日志和事件中是否出现密钥、fence token、签名 URL 或提示词正文。
- 测试服务端请求伪造（SSRF）、重定向、DNS 重新绑定、压缩炸弹和大媒体文件造成的内存压力。
- 测试执行尝试令牌的重放、过期，以及跨执行步骤、执行实例和租户使用。
- 测试 GraphPatch 是否能绕过节点数、深度、边数、数据结构和服务商 URL 限制，或给已有节点追加前置依赖。

### 8.6 Web 界面测试

- 测试 URL 子路由、刷新、后退、深链接和两标签页并发操作。
- 使用包含 100/500 个节点的测试数据，验证筛选、选择、分页和 SSE 增量更新。
- 在 `1440 x 1000`、`390 x 844` 和最低 `320px` 宽度下检查 `scrollWidth` 和内容遮挡。
- 测试键盘操作、`focus-visible`、提示文字和可访问名称，以及不依赖颜色识别状态。
- 测试投影延迟、会话过期、未授权查看完整执行记录和未知事件的降级展示。

### 8.7 产品边界契约测试

- OpenAPI 必须能被 YAML 解析器无重复键加载，且所有 Studio 路由引用的 Envelope 和 DTO 都存在。
- 客户执行客户端目录只能把完整具备 `workspace_bootstrap` 契约的客户端标成可连接；当前仅 Codex 可用，Claude Code 等客户端不得因具备部分自动化能力而进入客户选择面。
- 新建客户任务必须按目标项目回源校验已连接执行客户端；一个项目的连接不能自动满足另一个项目的准入条件。
- 客户“创作结果” DTO 的 `result_type` 只允许 `persona / script / storyboard / image / video`，类型与状态是两个独立轴。
- 客户“我的资产”必须使用独立 Folder/Material DTO；普通资料类型和 `processing_state` 不得加入现有结果 DTO。
- 结果状态只允许 `draft / pending_confirmation / changes_requested / confirmed / delivered / superseded / blocked`，且只有 `confirmed` 和 `delivered` 可复用。
- 没有任何 `MediaReview` 的图片或视频 Artifact 必须投影为 `pending_confirmation`，不能通过“审核记录不存在”推导为 `confirmed`。
- 新的灵感保存契约使用 `keep_as_project_reference` / `saved_as_project_reference`；旧 `save_for_reuse` 只作兼容输入，响应不得继续返回 `saved_for_reuse`。
- 保留为项目参考的灵感只在当前任务或项目参考投影中出现；它不得出现在“创作结果”，也不得在没有明确导入动作时出现在“我的资产”。
- `DeliveryPackage` 只在交付视图中出现；工作区资料和创作结果 DTO 都不得增加 `delivery_package` 类型或复制交付正文。
- 投影重建必须产生相同的结果类型、状态、复用门禁和底层引用，且不触发模型、执行者或外部服务。

## 9. 性能与容量验收

首轮目标用于发现瓶颈，不宣传为通用规模上限：

- 单个执行实例：100 个节点、20 个逻辑并发请求；硬上限由 `RuntimePolicy` 控制。
- 单个租户：10 个并发执行实例；多租户测试需要证明普通优先级任务不会一直得不到执行机会。
- 共享状态：10,000 条小记录、1,000 次并发 CAS 或追加写入；大字段强制使用 ArtifactRef。
- 事件：验证单个执行实例 100,000 条事件的分页、过滤、投影重建和保留策略。
- 上下文：主控智能体不内联子智能体的完整执行记录，ContextView 必须在预算内稳定生成。
- 媒体：按声明的最大文件大小进行流式传输，执行器 RSS 不能随完整文件大小线性增长。

具体的延迟服务目标（SLO）要等阶段 2 建立真实基线后再确定；在完成测量之前，不在方案中编造百分位指标。

## 10. 业务验收

营销活动标杆任务必须证明：

1. 将一份需求简报冻结为输入快照，并并行拆分出至少 10 个候选节点。
2. 人工审批节点驳回一个方向时，只影响对应分支，不能污染其他分支。
3. 服务商并发量和总费用不能超过 RuntimePolicy。
4. 任意节点崩溃后都能继续执行；已经完成的节点不能重复调用模型或服务商。
5. 主控智能体只能收到子节点摘要和引用，不能自动聚合原始完整执行记录。
6. 汇聚操作必须按照已经冻结的 FanoutSet 和声明的策略完成，不能提前结束。
7. 最终底层产物（`Artifact`）、`MediaReview`、`DeliveryPackage` 和所有上游摘要都可以反向追溯。
8. 分支执行会创建新的 JobRun；旧 Run 和已经发生的外部操作继续保留完整审计记录。
9. 搜索结果、灵感和来源证据保持为任务输入或项目参考；只有客户明确导入后才建立“我的资产”资料引用。
10. 客户上传/导入资料、任务参考、创作结果和交付使用独立 DTO 与状态轴，Runtime 只保留固定引用。
11. 人物原型、剧本、分镜、图片和视频生成结果能通过输出绑定追溯到 JobRun/NodeRun；只有已确认或已交付结果能加入新任务。
12. 交付页可以引用同一结果与交付包，但不产生重复的资产正文。

第二个文章复盘执行图必须复用同一套运行时表和调度器；只允许新增内容业务包、数据结构和人工审批节点。

## 11. 发布与回退

### 11.1 发布顺序

```text
执行增量数据库迁移
  -> 部署向后兼容读取的服务端
  -> 部署支持能力协商的执行器和守护进程
  -> 开启旁路编译
  -> 为内部租户开启线性调度器
  -> 按依赖顺序开启共享状态、外部操作和检查点
  -> 为测试租户开启动态执行图
  -> 活动业务试点租户
  -> 逐步扩大范围
```

每一步都必须验证 Runtime 任务领取、结果交接、任务详情和交付发布条件仍然正常。

### 11.2 回退流程

```text
发生事故
  -> 停止新的 JobRun 准入和动态 GraphPatch
  -> 保留读取路径和数据库迁移
  -> 让可以安全结束的运行中节点自然排空
  -> 暂停存在未知外部操作或执行器不兼容的 JobRun
  -> 对外部操作和本地日志进行对账
  -> 部署修复后的服务端和执行器
  -> 从持久化状态恢复
```

不能把已经运行的动态 JobRun 转换成业务 StageRun 来“回滚”，因为这样会丢失依赖关系和外部操作语义。只能选择排空、暂停、取消，或部署向后兼容的 Runtime 版本恢复。生产回退不能删除 Runtime 表和历史事件。

## 12. 总体验收条件

- 动态图等增量能力关闭时，Runtime 线性核心流程不能出现行为回归。
- 旁路编译对所有内置 SOP 生成稳定摘要，并得出与原流程等价的下一阶段结果。
- 旧短视频 SOP 自动认领和内置元数据修复代码不存在；built-in SOP 必须具有完整 current 身份，架构守卫阻止迁移函数、`sop.legacy_migrated` 和旧 source 标记恢复。
- Runtime 线性调度器可以完成 `marketing_video` 测试场景。
- 100 个节点、20 路并发的稳定性压测中，不得出现重复租约、重复节点、重复输出或重复费用。
- 故障注入矩阵全部得到预期的恢复、阻断或人工对账状态。
- 所有权威状态写入都必须经过数据结构校验、CAS 或单写入者约束，并产生 JobEvent。
- 所有 GraphPatch 都必须产生新 Revision；历史节点契约保持不变。
- 重放过程的外部调用数必须为 0；分支执行不能改变源 JobRun。
- 跨租户安全、提示词注入、密钥和 SSRF 检查全部通过。
- Codex/Claude 至少有一个真实执行适配器通过一致性测试；另一个可以保持预览状态，不能虚报已经达到生产可用标准。
- 运行诊断视图和运维手册支持定位问题、暂停、恢复、对账和创建执行分支。
- 只有产品、工程、安全、内容运营和真实服务商/智能体执行器负责人共同签字，才能对外宣称运行时可以用于生产环境。
