# 业务域、事实所有权与核心对象

状态：`目标规范，现有对象处置需在实施前通过 ADR 冻结`。

更新时间：2026-08-05。

## 1. 为什么先定义事实所有权

如果 Runtime、客户投影、Agent 会话和业务对象都能修改同一件事，系统会出现无法解释的“多个真相”。本规范为每类事实指定唯一拥有者；其他模块只能通过命令修改，或通过版本化引用读取。

## 2. 业务域地图

```text
Identity & Tenancy
        |
        v
Workspace & Brand Context
        |
        +--------------------+
        |                    |
        v                    v
Source & Knowledge     Creative Product Catalog
        |              Experience / SOP / Capability
        +---------+----------+
                  |
                  v
             Work Management
               WorkTask
                  |
                  v
            Runtime Execution
        Job / Node / Attempt / Effect
                  |
      +-----------+------------+
      |                        |
      v                        v
Review & Approval       Artifact & Delivery
      |                        |
      +-----------+------------+
                  v
         Performance & Learning

Source / Review / Delivery / Learning
                  |
                  v
      Experience Read Projections
 CustomerJourney / CreativeAssetCatalog / Operations

Operations & Audit 横切所有域，但不拥有其业务正文
```

## 3. 领域职责

| 业务域 | 拥有的事实 | 不拥有 |
| --- | --- | --- |
| Identity & Tenancy | User、Tenant、Membership、Session、平台角色 | 创作任务和执行状态 |
| Workspace & Brand Context | Project、品牌上下文、项目成员、WorkspaceBinding | SOP 发布和 Agent 会话状态 |
| Source & Knowledge | Source、SourceRevision、EvidenceSpan、KnowledgeObject、KnowledgeSnapshot、RightsRecord | 任务调度和批准决定 |
| Creative Product Catalog | ExperienceTemplate、SOPVersion、StageDefinition、GateDefinition、Capability、发布与租户启用 | 进行中任务状态 |
| Work Management | WorkTask、客户意图、请求输出、当前业务阶段摘要 | 节点租约和执行尝试 |
| Runtime Execution | JobRun、JobPlanRevision、NodeRun、RunAttempt、Lease、State、Effect、执行事件 | 来源正文、批准正文和交付正文 |
| Review & Approval | SubmissionRevision、TaskRevision、ReviewCycle、GateEvaluation、ApprovedSnapshot、ApprovalDecision | 自动生成内容和执行调度 |
| Artifact & Delivery | Artifact、MediaGenerationJob、DeliveryPackage、TaskDelivery、完整性 | 外部平台真实发布结果，除非有回执 |
| Performance & Learning | PerformanceObservation、RatingDecision、已批准学习候选 | 自动改写知识和 SOP |
| Experience & Projection | CustomerJourneyProjection、CreativeAssetCatalogItem（结果投影）、RuntimeOperationsProjection、角色适配 DTO | 来源、权利、批准、产物、交付和执行权威状态 |
| Operations & Audit | AuditEvent、用量读模型、告警、支持关联 | 直接修改其他域权威状态 |

## 4. 核心聚合关系

```text
WorkTask  用户想完成的一项工作
  |
  +-- fixes --> ExperienceTemplateVersion
  +-- fixes --> SOPVersion + digest
  +-- refs  --> approved inputs
  |
  +-- has many --> JobRun  同一工作的一次执行或执行分支
                     |
                     +-- fixes --> JobPlanRevision
                     +-- has many --> NodeRun
                                        |
                                        +-- has many --> RunAttempt
                                        +-- refs --> business inputs/outputs
                                        +-- may wait --> GateEvaluation
```

### 4.1 WorkTask

拥有客户意图、期望输出、优先级和业务阶段摘要。现有 `domain.WorkTask` 继续复用并扩展体验模板和 Job 引用，不重建 `CreationTask`。

### 4.2 JobRun

表示 WorkTask 的一次完整执行。重新执行、从检查点分支或更换批准路线时创建新 JobRun，旧 JobRun 保持不可变历史。

### 4.3 NodeRun

表示固定 JobPlanRevision 中一个可调度节点的执行状态。NodeRun 拥有依赖、状态、输入输出引用和门禁等待，但不保存大正文。

### 4.4 RunAttempt

表示某个执行者取得节点租约后的一次尝试。现有 `domain.RunAttempt` 直接扩展以关联 NodeRun；执行进程重启或租约过期创建新 Attempt，不覆盖旧尝试。

## 5. 现有运行对象处置

当前仓库同时存在 `WorkTask`、`StageRun`、`TaskRun` 和 `RunAttempt`。V8 计划引入 `JobRun` 与 `NodeRun`。实施前必须通过 ADR 冻结下列关系：

| 当前对象 | 当前语义 | 目标处置 | 退场或保留条件 |
| --- | --- | --- | --- |
| `WorkTask` | 用户工作对象 | `Extend` | 永久保留，去除底层执行细节 |
| `StageRun` | SOP 业务阶段进度 | `Compat` 或业务投影 | NodeRun 能确定性生成同等阶段状态后停止权威写入 |
| `TaskRun` | 单能力、可租赁执行 | `Rename/Extend` 为 NodeRun，或作为兼容执行记录 | 必须选择一个方向，禁止与 NodeRun 永久双写 |
| `RunAttempt` | 执行租约和尝试 | `Extend` | 关联 NodeRun，继续保留历史 |
| `JobRun` | 尚未实现的整项执行 | `New` | 成为 WorkTask 下完整执行聚合 |
| `NodeRun` | 尚未实现的执行节点 | `New or Rename` | 与 TaskRun 的关系由 ADR 决定 |

禁止先创建新表和接口，再在实现后解释这些对象的关系。

## 6. 配置对象处置

| 当前对象 | 目标关系 |
| --- | --- |
| `ProjectTemplate` | 仅用于创建项目预设；不得与客户场景 `ExperienceTemplate` 混用。若语义重叠，重命名并迁移。 |
| `SOPVersion` | 继续作为业务流水线定义；只有无法表达的稳定语义才增量扩展。 |
| `StageDefinition` / `GateDefinition` | 继续作为阶段与门禁契约，不创建平行定义。 |
| `Capability` / `capabilitycatalog` | 扩展为统一能力注册表，保持版本和摘要。 |
| Environment Manifest / ExecutionBundle | 继续提供执行环境和绑定快照，不再创建第二套 Binding Manifest。 |
| `ProjectProjection` / `projectview` | 复用投影模式，拆出 CustomerJourneyProjection；禁止平行维护独立任务状态。 |
| `CreativeAssetCatalogItem` | 新建可重建只读结果投影，面向客户只引用人物、剧本、分镜、图片和视频结果；输入、权利和交付由各自投影表达，不得成为统一写模型。 |

## 7. 事实所有权矩阵

| 事实 | 唯一写入方 | Runtime 行为 | 客户投影行为 |
| --- | --- | --- | --- |
| 来源内容与摘要 | Source & Knowledge | 保存 revision ref 和 digest | 展示来源与引用摘要 |
| 已批准知识 | Source & Knowledge + 人工决定 | 构建固定 ContextView 引用 | 展示已确认事实和限制 |
| 客户任务意图 | Work Management | 创建 JobRun 时固定快照 | 展示任务目标和业务阶段 |
| 节点执行状态 | Runtime | 权威写入 | 翻译为有限客户状态 |
| 候选内容 | 对应业务包 / Submission 域 | 关联生成 Node 和输入摘要 | 展示候选和来源 |
| 批准决定 | Review & Approval | 等待、消费不可变决定引用 | 展示决定影响和生效版本 |
| 文件产物 | Artifact & Delivery | 保存 ArtifactRef 和生成状态 | 预览或下载允许结果 |
| 外部平台发布 | Delivery / Provider 回执 | 记录 Effect 和回执引用 | 只有真实回执才显示发布成功 |
| 跨任务结果资产目录项 | Experience & Projection | 仅保存生成结果引用或输出引用 | 组合结果类型、名称、预览、版本和可复用状态 |

## 8. 候选到正式输入的推进

```text
Untrusted Input
  -> Collected Source
  -> Normalized Source + Evidence
  -> Candidate Insight / Candidate Content
  -> Customer Selected
  -> Governance Review when required
  -> ApprovedSnapshot / Accepted Revision
  -> Fixed downstream input
```

任何推进都必须保留上游引用和摘要。客户选择只表示业务意向，不自动批准事实、营销主张、版权或平台合规。

## 9. 跨任务资产沉淀与复用

```text
SourceRevision / Asset / KnowledgeObject / RightsRecord
                    |
                    v
          WorkTask 输入与项目参考投影

ApprovedSnapshot / TaskRevision / Artifact
                    |
                    v
       CreativeAssetCatalogItem 结果只读投影
                    |
                    v
        CreativeAssetRef -> WorkTask input snapshot
```

- 来源、灵感、知识、权利和任务输入不进入客户创作资产目录；它们保留在任务输入或项目参考投影。
- `CreativeAssetCatalogItem` 只收录人物原型、剧本、分镜、图片和视频等流水线结果；正文仍由拥有域提供。
- 结果状态决定是否可以复用，目录项只保存显示摘要、受控预览和版本化事实引用。
- `CreativeAssetRef` 固定底层对象类型、ID、版本和摘要，Runtime 不以目录项作为权威输入。
- 权利到期或来源失效阻止新任务使用，并通过 lineage 定位受影响下游；历史任务和交付不被静默改写。

详细规范见 [`docs/product/creative-asset-library/02-domain-and-projection.md`](../product/creative-asset-library/02-domain-and-projection.md)。

## 10. 投影规则

- 投影可以组合多个业务域，但不能成为权威写模型。
- 投影必须包含生成时间、版本或游标，并能识别延迟。
- 允许动作由领域服务计算，投影不得自行推导危险命令。
- 客户投影不包含内部 ID、密钥、完整上下文、执行命令和模型原始响应。
- 运营投影可以包含诊断字段，但仍遵守租户、角色和数据分类。
- 投影损坏可以重建；重建不得调用执行者或产生外部副作用。
- 跨域目录投影只能调用各域 Query 或消费事件，不通过共享可变聚合取得写权限。
- 高风险复用命令必须回源校验租户、当前版本、权利和用途，不能只信最终一致投影。

## 11. 领域变更门禁

新增核心对象前必须回答：

1. 现有对象为什么无法通过兼容扩展表达？
2. 谁拥有其状态，谁只能引用？
3. 生命周期和不可逆终态是什么？
4. 幂等键、版本或摘要是什么？
5. 如何迁移、双读比较、回退和删除旧路径？
6. 哪些契约测试能证明没有形成第二套事实？
