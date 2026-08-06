# 系统架构与 Agentic Job Runtime

状态：`目标规范；Runtime 详细实现继续由 docs/roadmap/v8 定义`。

更新时间：2026-08-05。

## 1. 系统上下文

下图是工程架构图，用于说明组件、信任和执行边界，不是客户产品首图。客户叙事使用“资料与已有资产 -> Content Work OS 创作任务 -> 可确认结果与专业工具”，见[产品叙事规范](../product/00-product-narrative.md)。

```text
┌──────────────────────────────────────────────────────────────┐
│ Human Surfaces                                               │
│ Customer Studio / Asset Library   Settings     Admin Console  │
└───────────┬────────────────────┬────────────────────┬─────────┘
            │                    │                    │
            v                    v                    v
┌──────────────────────────────────────────────────────────────┐
│ Experience / Application Layer                               │
│ Studio BFF · Operations BFF · Commands · Asset Projections   │
└──────────────────────────────┬───────────────────────────────┘
                               │
┌──────────────────────────────v───────────────────────────────┐
│ Business Domains                                             │
│ Catalog · Work · Source/Knowledge · Review · Delivery        │
└───────────────┬───────────────────────────────┬──────────────┘
                │ plan / refs                   │ decisions / refs
                v                               v
┌──────────────────────────────────────────────────────────────┐
│ ContentCloud Agentic Job Runtime                             │
│ Job · Graph · Node · Attempt · State · Lease · Effect        │
│ Scheduler · Budget · Checkpoint · Event · Projection         │
└───────┬──────────────────┬──────────────────┬────────────────┘
        │                  │                  │
        v                  v                  v
 Deterministic Worker   Agent Adapter   Provider Connector
 ContentCloud process   Codex/Claude    Search/Media/API
        \                  |                  /
         +-----------------+-----------------+
                           |
                           v
                   Candidate / ArtifactRef
                           |
                           v
                   Business Gate / Human
```

## 2. 部署策略

首阶段继续使用模块化单体：

```text
contentcloud-server
├── studio BFF
├── operations BFF
├── business domain services
├── runtime command/query services
└── PostgreSQL / Blob access

contentcloud-worker
├── deterministic jobs
├── connector execution
├── media polling / reconciliation
└── projection rebuild

contentcloud CLI / daemon
├── local workspace
├── agent adapter
├── MCP tools
└── lease / heartbeat / result submission
```

模块边界不等于网络边界。只有出现独立扩缩容、故障隔离、合规区域或团队所有权需求，且监控数据证明模块化单体无法满足时，才通过 ADR 拆服务。

## 3. Runtime 的责任

Runtime 负责：

- 为 WorkTask 创建一次 JobRun 并固定 JobPlanRevision。
- 判断节点依赖、资源、权限、预算和 Gate 是否满足。
- 为执行者发放最小权限租约和 ContextView。
- 保存 NodeRun、RunAttempt、StateMutation、Effect 和有序事件。
- 处理取消、重试、租约过期、中断恢复、检查点和执行分支。
- 对外部副作用执行登记、幂等、结果核对和补偿流程。
- 生成客户与运营运行投影。
- 接受已通过拥有域校验的 `CreativeAssetRef`，并把底层对象版本和摘要固定到任务输入。

Runtime 不负责：

- 保存来源、知识、剧本、分镜和交付正文。
- 决定事实、权利、营销主张、费用和最终内容是否批准。
- 理解客户页面布局和文案。
- 把某个模型、MCP 或 Agent 客户端写死在业务阶段中。
- 直接宣称外部平台已经完成发布。
- 拥有创作资产目录、权利状态或资产正文；这些内容由 Experience 投影和对应业务域负责。

## 4. 定义、计划和运行的分离

```text
ExperienceTemplateVersion     客户体验定义
          |
          +--> SOPVersion     业务阶段、Schema、能力、Gate
                       |
                       v compile + validate
                 JobPlanRevision
                 节点 / 边 / 策略 / 限额
                       |
                       v bind at admission
                ExecutionBindingSnapshot
                 具体执行者 / 区域 / 预算
                       |
                       v
                     JobRun
```

- 定义可修改但按版本发布。
- JobPlanRevision 一旦用于 JobRun 就不可变。
- ExecutionBindingSnapshot 固定具体执行方式和披露等级。
- 紧急停用能力会阻断受影响节点，不静默切换到权限更宽的实现。

## 5. 核心状态机

### 5.1 JobRun

```text
created -> admitted -> running -> waiting_human -> running -> completed
             |           |             |             |
             |           |             |             +-> failed
             |           |             +-> cancelled
             |           +-> paused -> running
             +-> rejected

terminal: completed / failed / cancelled / rejected
```

终态不能原地回到运行态。重新执行或从检查点恢复必须创建新 JobRun 或显式分支，并引用源 JobRun。

### 5.2 NodeRun

```text
pending -> ready -> leased -> running -> succeeded
   |         |        |         |
   |         |        |         +-> retryable_failed -> ready
   |         |        +-> lease_expired -> ready
   |         +-> waiting_resource -> ready
   +-> blocked / skipped / cancelled

running -> waiting_external -> succeeded / unknown / failed
running -> waiting_human    -> succeeded / changes_requested / cancelled
```

无效转移由领域方法和事务条件拒绝。例如 `succeeded -> running`、未满足依赖时 `pending -> leased`、结果不明时自动重试外部副作用。

### 5.3 Effect

```text
registered -> submitted -> acknowledged -> succeeded
                    |             |
                    |             +-> failed
                    +-> unknown -> reconciling -> succeeded / failed / manual_action
```

网络超时只产生 `unknown`，不能推断服务商未执行。只有确认未执行且策略允许时才创建新的外部尝试。

## 6. 典型数据流

### 6.1 客户启动任务

```text
Studio form
  -> BFF validates tenant + template + idempotency
  -> Work domain creates WorkTask and input snapshot refs
  -> Runtime compiles/adopts immutable JobPlanRevision
  -> admission fixes capability binding and budget
  -> JobRun created
  -> customer projection returns first business step
```

影子路径：

| 路径 | 行为 |
| --- | --- |
| 缺少输入 | 不创建 JobRun，返回具体字段和允许格式 |
| 空输入 | 按 Schema 明确拒绝或接受为空，不由模型猜测 |
| 重复提交 | 相同幂等键返回原任务；摘要不同则冲突 |
| 模板已停用 | 拒绝新任务，不影响已固定旧版本 |
| 无执行能力 | WorkTask 可保留草稿，JobRun 准入失败并给出运营原因 |
| 预算不足 | 返回需要审批或调整的业务动作 |

### 6.2 节点执行

```text
Scheduler finds ready NodeRun
  -> reserve resource + create lease transactionally
  -> build minimum ContextView
  -> executor accepts lease
  -> heartbeat / progress / candidate outputs
  -> validate output schema + digest + tenant scope
  -> persist business candidate through owning domain
  -> NodeRun stores refs and completes
  -> downstream readiness recalculated
```

### 6.3 人工决定

```text
NodeRun waits at Gate
  -> Review domain creates fixed SubmissionRevision / GateEvaluation
  -> customer sees version, diff, evidence, rights, cost impact
  -> server validates actor + current digest + idempotency
  -> immutable decision persisted
  -> Runtime consumes decision ref
  -> affected downstream nodes continue, invalidate or branch
```

### 6.4 中断恢复

```text
process / agent stops
  -> lease heartbeat expires
  -> Attempt marked interrupted or expired
  -> reconcile external effects before retry
  -> validate checkpoint refs and input digests
  -> create new Attempt with minimum context
  -> continue unfinished node
```

完整聊天历史和进程内存不是检查点。检查点只包含可验证的业务引用、执行状态摘要和恢复位置。

### 6.5 资产沉淀与复用

```text
Source / ApprovedSnapshot / Artifact / DeliveryPackage changed
  -> owning domain persists fact and emits event/ref
  -> CreativeAssetCatalog projector updates read model
  -> customer selects catalog item for a target task
  -> owning domains revalidate tenant + version + rights + usage
  -> Work domain freezes CreativeAssetRef into input snapshot
  -> Runtime receives subject ref + version + digest
  -> lineage records reuse and downstream outputs
```

目录投影延迟不影响事实正确性。列表可以最终一致，但创建任务、产生外部副作用和正式交付前必须回源校验。权利或来源失效时，新任务被阻止；已固定任务不被静默换成新版本，并在下一安全门禁重新评估。

## 7. 调度与资源

调度至少考虑：

- 节点依赖和 Gate。
- 租户、Job 和执行模式并发上限。
- 服务商与模型配额。
- 数据区域、隔离等级和本地设备在线状态。
- Token、费用、媒体时长和存储预算。
- 优先级与等待时长，避免大任务长期挤占小任务。

所有资源预留与租约发放必须在同一事务或可证明的原子协议中完成。运行中 Agent 等待子任务时必须释放稀缺并发名额，通过新 Attempt 恢复，而不是持续占用。

## 8. 上下文与状态

- ContextView 只包含节点必需的已批准输入、允许工具、预算和输出 Schema。
- 大正文和媒体通过 ArtifactRef 或 SourceRevisionRef 引用，不写入通用 JSON 状态。
- 共享状态使用类型化集合、CAS、追加或专用归并，不允许任意 JSON merge。
- 状态写入带 tenant、job、node、schema version、expected revision 和 idempotency key。
- 本地敏感资料默认不上传；只提交用户选择的摘要、引用和候选。
- 执行者输出全部重新校验，不信任客户端已验证声明。

## 9. 错误与恢复注册表

| 代码路径 | 失败 | Runtime 行为 | 客户看到 | 运营看到 |
| --- | --- | --- | --- | --- |
| 创建任务 | 重复、模板停用、Schema 无效 | 幂等返回或明确拒绝 | 修正输入或继续原任务 | 具体错误码和摘要 |
| 编译计划 | 环、缺 Schema、能力不可达 | 不准入 | 当前场景暂不可用 | lint 失败位置 |
| 领取节点 | 资源不足、并发冲突 | 等待并重算 | 正在等待处理能力 | 资源和队列指标 |
| Agent 执行 | 离线、会话丢失、输出无效 | 租约过期、新 Attempt 或阻断 | 连接工具或等待恢复 | Attempt 失败类和宿主版本 |
| 搜索/爬取 | 429、拒绝访问、SSRF | 退避、拒绝或部分完成 | 保留已有结果并提供下一动作 | Connector 诊断和预算 |
| 模型调用 | 空、拒绝、无效 JSON | 验证失败，按策略有限重试 | 未产生可用候选 | 模型、Schema 和失败类 |
| 外部生成 | 超时或回调乱序 | Effect unknown 后对账 | 正在核对，请勿重提 | 外部请求和对账状态 |
| 人工决定 | 版本过期、双标签页提交 | CAS 冲突，返回当前版本 | 结果已更新，请重新确认 | 决定摘要冲突 |
| 资产复用 | 权利过期、来源失效、投影陈旧 | 回源校验并拒绝陈旧引用 | 该资产当前不可使用，请查看原因或选择新版本 | 底层对象、策略摘要和影响范围 |
| 投影读取 | 延迟或重建 | 标记游标与延迟 | 正在更新，保留上次稳定结果 | projection lag 指标 |

禁止吞掉错误后继续，也禁止用同一个 `internal_error` 覆盖所有可恢复原因。

## 10. 扩展与容量边界

### 10.1 首阶段硬上限

具体数字由 V8 POC 和容量测试冻结，但必须存在：

- 单 Job 节点数、图深度和动态后代数。
- 单租户、单 Job、单服务商并发。
- 单节点上下文、输出、日志和运行时长。
- 单任务 Token、费用、媒体时长和存储。
- 外部查询、页面数、响应大小和重定向次数。

### 10.2 10 倍负载先坏在哪里

预期首批瓶颈：当前宽 Store 接口背后的数据库连接、投影聚合查询、外部服务商配额、大上下文构建和媒体下载。实现前必须为队列等待、数据库 p95、投影延迟、ContextView 大小和 Provider 配额建立基线。

### 10.3 100 倍负载

100 倍负载不通过无限增加 Agent 解决。先按租户和资源分区、批量读写、分页投影、限制图规模和流式 Blob；只有独立扩缩容与故障域数据明确后才拆 Worker 或 Runtime 服务。

## 11. Runtime 验收

1. 关闭新 Runtime 时，现有线性流程继续工作。
2. 旁路编译对现有 SOP 生成确定性计划并与旧阶段判断等价。
3. 同一输入和固定计划可重放读模型，不产生外部调用。
4. 节点失败不丢弃其他成功节点的正式结果。
5. 外部结果不明不会盲目重试。
6. 两条结构不同的业务流不增加专用调度状态。
7. 客户、运营和 Runtime API 具有独立契约和权限测试。
8. 所有运行对象和业务事实都能通过引用与摘要追溯。
9. 同一批准结果可以通过资产目录进入新任务，但 Runtime 不依赖目录项作为权威正文。
