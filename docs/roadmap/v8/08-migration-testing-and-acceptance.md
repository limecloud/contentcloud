# 08：迁移、测试与验收

> 阅读对象：负责兼容迁移、测试和发布验收的研发人员。本文件记录的是实施门槛，不表示这些能力已经上线。

## 1. 当前基线对账

截至 2026-08-06：

- 当前版本为 `v0.17.0`。Runtime 第一批实现已进入当前工作区；每个工作包仍须独立运行完整验证，不能沿用历史结果。
- V7 的类型化 Stage 输出、媒体领域、MediaReview、最终 Artifact、DeliveryPackage 和 Web 投影已在 `v0.16.0/v0.17.0` 落地。
- V8 已落地 JobRun/NodeRun/JobEvent、独立 RuntimeAttempt、状态 CAS、ContextView/AgentInstance 持久化、FakeHarness 调度闭环、联合租约和首版 Runtime Explorer；各文档必须继续区分已实现内核与生产能力。
- 根 `README.md` 已指向平台基线、产品需求和 V8 路线图；历史 V2/V7 路线图不再作为当前能力事实源。
- 真实媒体服务商、可持久化的轮询和回调、完整的媒体租约恢复、受限的流式下载和确定性后期处理仍未完成。
- Codex/Claude 适配器当前仍使用 legacy CLI 进程，不保存可跨 ContentCloud 进程恢复的宿主会话；HarnessRegistry 只解决单进程实例复用，不等于真实 SDK 恢复已经完成。

基线事实主要来自 `CHANGELOG.md`、`internal/domain`、`internal/runtime`、`internal/agentadapter`、Memory/PostgreSQL Store 以及迁移 `00012_v7_media_pipeline.sql`、`00014_agentic_job_runtime.sql`、`00015_runtime_agent_instances.sql`、`00016_runtime_attempts.sql`。历史路线图只能作为背景，不能代替当前代码和测试结果。

V8 的第一个工作包必须先更新权威文档和能力登记表；不能在错误的基线上继续规划。

## 2. 迁移原则

1. **先增量再切换**：新增表、字段、索引和 API；不删除或物理重命名 V7 对象。
2. **先旁路再成为权威**：先编译、比较和观测，不让新执行图直接决定生产调度。
3. **每类权威数据只保留一个写入者**：切换后明确 Job、Node、Stage 投影分别由谁写入，禁止长期并行维护两套状态机。
4. **不编造历史**：不为历史 Task 伪造 `JobEvent`、`Checkpoint`、`Artifact` 或服务商记录。
5. **由功能开关控制准入**：开关只决定新 Job 进入哪条路径；不能把已经运行的动态图降级成线性 Stage。
6. **优先向前恢复**：生产回退以停止新任务准入、排空、暂停或修复为主，不对已经写入数据的新表执行破坏性降级迁移。

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

当前物理落地：`00014_agentic_job_runtime.sql` 覆盖 JobPlan、JobRun、NodeRun、JobEvent、State、Checkpoint 和 ExternalEffect；`00015_runtime_agent_instances.sql` 覆盖 ContextView 与 AgentInstance；`00016_runtime_attempts.sql` 覆盖独立 RuntimeAttempt、复合外键、活动租约索引和强制 RLS。TaskRun/RunAttempt 继续作为 V7 兼容路径，MIG8-F 以及完整的 MIG8-H/MIG8-I 仍未落地。

## 4. 兼容投影

### 4.1 WorkTask

- `intent`、输入、SOP、人工审批节点和正式内容关系仍来自 `WorkTask` 及现有领域对象。
- `latest_job_run_id` 和 `active_job_run_id` 通过读模型返回，首版不要求写回 `WorkTask` 表。
- `current_stage_id` 只作为当前关注阶段的投影，不再驱动 V8 调度器。
- V7 Task 没有 JobRun 时显示 `legacy_linear`，不自动补造一段执行历史。

### 4.2 StageRun

- V7 写入路径继续直接维护 StageRun。
- V8 写入路径由投影器根据 `NodeRun`、人工审批节点和输出生成 `StageRun`。
- 同一个 WorkTask 不能同时启用 V7 直接写入者和 V8 投影器写入者。
- 投影器可以清空后重建；正式业务对象和运行时状态不能依赖投影器反向恢复。

### 4.3 TaskRun

- 旧 TaskRun/RunAttempt 的领取、上报和日志 API 保持不变，只服务 V7 兼容路径。
- V8 NodeRun/RuntimeAttempt 使用独立表和状态机，不双写 V7 执行状态，也不把 `runtime_job_runs` 错接到 `task_runs` 外键。
- HTTP/BFF 新资源使用 NodeRun/RuntimeAttempt 术语；业务阶段兼容只通过单向投影完成。

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
阶段 2  JobRun + 线性执行图调度器，复现 V7 顺序行为
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
2. 计算旧 Stage 的 `Order/InputRefs` 与新执行边的对应关系。
3. 检查数据结构是否可达、人工审批节点是否完整，以及执行能力（`Capability`）是否匹配。
4. 将预期的下一 Stage 与 Ready evaluator 的结果进行比较。
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

### 8.2 存储与事务测试

- Memory Store 和 PostgreSQL Store 使用同一组存储契约测试数据，特别验证 Prepare/Activate/Heartbeat/Finalize 的原子边界。
- 使用 `FOR UPDATE SKIP LOCKED` 或 `ready + version CAS` 并发领取任务，不能出现双重租约、重复 RuntimeAttempt 或资源超卖。
- 验证 Node、RuntimeAttempt 和 AgentInstance 的联合租约过期收敛；旧 owner 和旧版本不能续租或提交结果。
- 验证相同终态结果摘要幂等成功，不同摘要返回冲突；事件序号在 Job 锁下连续分配。
- 验证 StateMutation、Event 和投影的原子性。
- 验证 GraphPatch 版本竞争时只能接受一个写入者。
- 验证 RLS 覆盖所有新表，并拦截跨租户或跨项目的外键攻击。
- 从最新生产前版本执行迁移，验证历史空值和 `legacy` 分类正确。

### 8.3 执行适配器一致性测试

Fake、Codex 和 Claude 适配器使用同一套黑盒场景：

- 启动、事件流、结构化输出、取消和超时。
- MCP 读取、CAS、追加写入、过期令牌和越权工具调用。
- 进程被终止后恢复；宿主不支持恢复时，使用新会话和 ContextView 重新开始。
- 会话分支只影响智能体历史，不自动修改 JobRun。
- 上下文压缩后仍然遵守 TaskContract 和输出数据结构。
- 适配器版本或能力不匹配时默认拒绝执行。

CI 默认只运行 `FakeHarness`；真实 Codex/Claude 冒烟测试必须在明确授权、低预算、非客户数据环境中执行，不能使用消费级登录来测试平台服务。

FakeHarness 的必测脚本包括：正常结构化结果、启动失败、启动成功但返回空事件流、重复相同/不同终态、未知事件、事件流无终态关闭、延迟超过一次心跳周期，以及租约到期。测试还必须证明 HarnessRegistry 多次 Resolve 返回同一适配器实例并保留会话状态；即使租约尚未执行批量回收，过期 owner 提交的迟到终态也必须被拒绝。

### 8.4 故障注入

| 注入点 | 预期结果 |
| --- | --- |
| 在 PrepareDispatch 提交前终止调度器 | 事务完整回滚，不产生 ContextView、Agent 绑定、RuntimeAttempt 或孤立租约 |
| 在 PrepareDispatch 提交后、Harness.Start 前终止调度器 | `prepared` Attempt 可诊断，并由租约回收原子释放 Node/Attempt/Agent |
| Harness.Start 成功后、ActivateDispatch 前终止调度器 | 尽力中断已知会话；无法清理时由租约回收，旧 worker 不得提交结果 |
| Harness.Start 返回空事件流 | Attempt 进入可重试失败，Node 和 Agent 释放执行权，不进入运行态 |
| 租约已到期但回收任务尚未运行 | 旧 worker 的终态提交返回 `DISPATCH_LEASE_STALE`；随后回收将 Attempt 置为 `expired` 并释放 Node/Agent |
| 终态事务提交后、worker 收到响应前终止 | 相同摘要重报返回幂等成功，不产生第二个终态事件 |
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
- 扫描日志和事件中是否出现密钥、RunToken、签名 URL 或提示词正文。
- 测试服务端请求伪造（SSRF）、重定向、DNS 重新绑定、压缩炸弹和大媒体文件造成的内存压力。
- 测试执行尝试令牌的重放、过期，以及跨执行步骤、执行实例和租户使用。
- 测试 GraphPatch 是否能绕过节点数、深度、边数、数据结构和服务商 URL 限制，或给已有节点追加前置依赖。

### 8.6 Web 界面测试

- 测试 URL 子路由、刷新、后退、深链接和两标签页并发操作。
- 使用包含 100/500 个节点的测试数据，验证筛选、选择、分页和 SSE 增量更新。
- 在 `1440 x 1000`、`390 x 844` 和最低 `320px` 宽度下检查 `scrollWidth` 和内容遮挡。
- 测试键盘操作、`focus-visible`、提示文字和可访问名称，以及不依赖颜色识别状态。
- 测试投影延迟、会话过期、未授权查看完整执行记录和未知事件的降级展示。

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
7. 最终正式产物（`Artifact`）、`MediaReview`、`DeliveryPackage` 和所有上游摘要都可以反向追溯。
8. 分支执行会创建新的 JobRun；旧 Run 和已经发生的外部操作继续保留完整审计记录。

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

每一步都必须验证旧 V7 的任务领取、结果上报、任务详情和交付发布条件仍然正常。

### 11.2 回退流程

```text
发生事故
  -> 停止新的 V8 执行实例准入和动态 GraphPatch
  -> 保留读取路径和数据库迁移
  -> 让可以安全结束的运行中节点自然排空
  -> 暂停存在未知外部操作或执行器不兼容的 JobRun
  -> 对外部操作和本地日志进行对账
  -> 部署修复后的服务端和执行器
  -> 从持久化状态恢复
```

不能把已经运行的动态 JobRun“转换回”V7 线性 Stage，因为这样会丢失依赖关系和外部操作语义。只能选择排空、暂停、取消，或使用兼容的 V8 版本恢复。生产回退不能删除新表和历史事件。

## 12. 总体验收条件

- V8 功能开关全部关闭时，V7 核心流程不能出现行为回归。
- 旁路编译对所有内置 SOP 生成稳定摘要，并得出与原流程等价的下一阶段结果。
- V8 线性调度器可以完成 V7 `marketing_video` 测试场景。
- 100 个节点、20 路并发的稳定性压测中，不得出现重复租约、重复节点、重复输出或重复费用。
- 故障注入矩阵全部得到预期的恢复、阻断或人工对账状态。
- 所有权威状态写入都必须经过数据结构校验、CAS 或单写入者约束，并产生 JobEvent。
- 所有 GraphPatch 都必须产生新 Revision；历史节点契约保持不变。
- 重放过程的外部调用数必须为 0；分支执行不能改变源 JobRun。
- 跨租户安全、提示词注入、密钥和 SSRF 检查全部通过。
- Codex/Claude 至少有一个真实执行适配器通过一致性测试；另一个可以保持预览状态，不能虚报已经达到生产可用标准。
- 运行诊断视图和运维手册支持定位问题、暂停、恢复、对账和创建执行分支。
- 只有产品、工程、安全、内容运营和真实服务商/智能体执行器负责人共同签字，才能对外宣称运行时可以用于生产环境。
