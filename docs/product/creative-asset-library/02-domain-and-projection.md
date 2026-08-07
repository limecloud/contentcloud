# 客户资产入口领域与投影规范

状态：`实施中；ADR-0011/0013/0014 已接受，工作区资料上传首切片与 Customer Studio 动态结果投影已实现，结果持久化 Projector 待完成`。

更新时间：2026-08-07。

## 1. 核心决定

客户“资产”入口组合跨域只读投影，不创建吞并所有内容的超级 `Asset` 聚合。“我的资产”读取客户明确上传或导入的工作区资料，“创作结果”读取流水线生成结果；输入候选和治理对象仍由任务参考投影与运营投影管理。

当前 `CustomerStudioAssets` 在查询时从 `WorkTask` 相关事实确定性组装 `CreativeAssetCatalogItem`，只验证了“创作结果”边界。目标 `Creative Result Projector` 落地后应替换这一动态组装路径；“我的资产”使用新的窄契约，不给现有结果行契约增加文件夹、上传和处理字段。

原因：现有对象拥有不同生命周期、权利规则和不可变语义。把它们复制进一个可写 `Asset` 会产生第二套正文、第二套状态和无法可靠传播的失效关系。

## 2. 工作区资料、任务参考、结果与交付事实

| 客户边界 | 底层事实对象 | 客户投影 | 说明 |
| --- | --- | --- | --- |
| 我的资产 | 客户明确拥有或管理的 `Asset` / `Artifact` 引用、文件版本和组织关系 | `WorkspaceMaterialProjection` | 上传、导入或登记后出现；文件夹不拥有正文 |
| 任务输入 / 项目参考 | 搜索候选、`SourceRevision`、`KnowledgeObject`、`RightsRecord` | 当前任务输入和项目参考 | 不会因被读取而自动进入资产 |
| 创作结果 | `KnowledgeSnapshot`、`TaskRevision`、`ApprovedSnapshot`、`Artifact` | `CreativeResultAssetProjection`（行契约为 `CreativeAssetCatalogItem`） | 对应人物原型、剧本、分镜、图片和视频 |
| 交付 | `DeliveryPackage`、`TaskDelivery` | 当前任务交付与下载 | 不复制成新的资产类型 |

工作区资料类型、来源、用途和处理状态分别表达；结果状态与结果类型也分开。普通文件不进入生成结果确认状态机。

```text
material_kind: document / image / video / audio / table / other
origin: uploaded / imported / linked
usage: project_material / project_reference
processing_state: uploading / processing / ready / failed

result_type: persona / script / storyboard / image / video
result_status: draft / pending_confirmation / changes_requested /
               confirmed / delivered / superseded / blocked
```

## 3. 现有事实对象

| 对象 | 当前语义 | 事实所有者 | 结果投影如何使用 |
| --- | --- | --- | --- |
| `Source` | 一项来源的逻辑身份 | Source & Knowledge | 仅供内部 lineage 和失效传播，不生成目录项 |
| `SourceRevision` | 不可变来源版本和摘要 | Source & Knowledge | 进入任务输入或项目参考；结果复用时供内部校验 |
| `Asset` | 已登记、受用途和权利约束的参考素材 | Source & Knowledge | 客户明确上传/导入的资料可被工作区投影引用；搜索候选仍只进入任务参考 |
| `RightsRecord` | 渠道、地区、期限和限制 | Source & Knowledge | 推导结果可复用状态，不复制为目录状态 |
| `KnowledgeObject` | 有证据和治理状态的知识 | Source & Knowledge | 进入知识/项目参考投影，不生成客户结果目录项 |
| `KnowledgeSnapshot` / `TaskRevision` | 流水线生成的结构化结果版本 | 对应内容业务域 | 按 Schema 映射为人物原型或剧本结果 |
| `ApprovedSnapshot` | 客户批准后的不可变内容版本 | Review & Approval | 提供确认事实，并可映射为分镜等正式结果 |
| `Artifact` | 从内容结果生成的生产文件 | Artifact & Delivery | 映射为图片或视频结果并提供受控预览 |
| `DeliveryPackage` | 正式交付集合 | Artifact & Delivery | 只进入交付视图，并参与推导已有结果的 `delivered` 状态 |
| `PerformanceObservation` | 对已确认内容的效果观察 | Performance & Learning | 首期只供运营分析，不作为资产正文 |

现有代码事实以 `internal/domain/content.go`、`internal/app/assets.go`、`internal/app/lineage.go` 和 `internal/domain/projection.go` 为准。

### 3.1 工作区资料投影契约

客户资产页面不是一个通用数组。BFF 组合以下专用契约：

```text
CustomerAssetSurface
├── WorkspaceMaterialProjection
│   ├── WorkspaceFolderItem
│   │   ├── folder_ref / parent_ref
│   │   ├── name / project_scope / child_count
│   │   └── created_at / updated_at
│   └── WorkspaceMaterialItem
│       ├── material_ref / folder_ref
│       ├── material_kind / origin / usage
│       ├── title / mime_type / size
│       ├── preview_ref / processing_state
│       ├── project_scope / rights_summary
│       └── created_at / updated_at
└── CreativeResultAssetProjection
    └── CreativeAssetCatalogItem
```

约束：

- 文件夹只保存组织关系，不保存文件正文、权利结论或任务状态。
- `material_ref` 固定底层对象身份；正式加入任务时还要固定版本和摘要。
- `processing_state` 只说明接收、解析和预览状态，不能表示内容已经确认或拥有可用权利。
- 上传、导入、移动和删除组织关系使用工作区资料命令；结果确认、退回和复用使用结果域命令。
- “最近使用”从两个投影的访问事件派生，不新增 `RecentAsset` 写模型。
- 当前代码已经按这些窄契约实现 `WorkspaceFolderItem`、`WorkspaceMaterialItem` 和 `WorkspaceMaterialProjection`，没有扩张 `StudioAssetItem`。连接器导入、外链登记、移动/删除和派生理解结果仍待后续工作包。

## 4. 创作结果目录投影

客户“创作结果”逻辑读模型：

```text
CreativeAssetCatalogItem
├── schema_version
├── catalog_item_id
├── tenant_id / project_scope
├── result_type / project / task
├── title / summary / version
├── status / reusable / blocked_reason
├── preview_ref / downloads
├── created_at
└── internal_lineage_ref / generated_at / projection_cursor
```

约束：

- `catalog_item_id` 由 tenant、result type、subject id 和版本策略确定性生成。
- `result_type` 只表达人物原型、剧本、分镜、图片和视频；状态不能写入类型字段。
- 结果目录的 `subject_ref` 和 `internal_lineage_ref` 只供服务端追溯，客户 DTO 不暴露底层对象名称。
- 目录不保存正文和原始媒体副本。
- `preview_ref` 必须是权限受控引用，不直接暴露 Blob ObjectKey。
- `blocked_reason` 和 `reusable` 是派生结果，不能通过通用目录更新 API 直接修改。
- 投影可以删除和重建，重建不得调用模型、执行者或外部服务。

## 5. 结果资产引用契约

客户把结果资产加入任务时生成版本化引用。任务输入和项目参考使用独立的 `InputRef`，不能复用本契约的结果类型字段：

```text
CreativeAssetRef
├── catalog_item_id          仅用于产品追溯
├── result_type / subject_id
├── subject_version_id
├── subject_digest
├── usage_intent
├── target_channel
├── selected_by / selected_at
└── validation_snapshot
    ├── reuse_state
    ├── rights_record_ids[]
    └── policy_digest
```

Runtime 和业务域以 `subject_*` 与摘要作为正式输入，不以目录项作为权威对象。`validation_snapshot` 说明选择当时为什么允许使用，但在产生新副作用或正式交付前仍需按策略重新检查有效权利。

### 5.1 现有对象的版本与摘要映射

| subject type | `subject_version_id` | `subject_digest` | 说明 |
| --- | --- | --- | --- |
| `knowledge_snapshot` | `KnowledgeSnapshot.ID` | `KnowledgeSnapshot.Digest` | 人物原型等结果版本 |
| `task_revision` | `TaskRevision.ID` | `TaskRevision.ContentHash` | 剧本等结果版本 |
| `approved_snapshot` | `ApprovedSnapshot.ID` | `ApprovedSnapshot.ContentHash` | 快照本身不可变 |
| `artifact` | `Artifact.ID` | `Artifact.SHA256` | 文件摘要固定具体内容 |

`subject_version_id` 对不可变对象可以等于 subject ID；契约必须允许字符串和明确的规范化表示，不能让消费者根据对象类型猜测“最新版本”。

## 6. 收录和分类规则

```text
业务事实发生变化
  -> 领域事件或兼容变更记录
  -> Catalog Projector 读取事实引用
  -> 应用结果类型、状态和可见性规则
  -> 生成或更新 CreativeAssetCatalogItem
  -> 客户结果投影与运营治理投影分别裁剪字段
```

首期使用显式、确定性规则，不用模型决定是否进入资产入口：

- 客户本地上传、从已授权连接器导入或明确登记的文件进入 `WorkspaceMaterialProjection`。
- 搜索候选和来源证据只有在客户执行明确导入动作后，才建立工作区资料引用；“保留为项目参考”本身不会导入。
- `KnowledgeSnapshot`、`TaskRevision`、`ApprovedSnapshot` 和合法 `Artifact` 创建后按结果类型收录。
- `draft`、`pending_confirmation` 和 `changes_requested` 结果可以展示，但不能复用。
- `confirmed` 和 `delivered` 结果设置 `reusable=true`，服务端仍在命令时重新校验输入权利和用途。
- 图片或视频 `Artifact` 没有对应审核事实时投影为 `pending_confirmation`；只有明确批准记录或正式交付事实才能推导为 `confirmed` / `delivered`。
- 来源治理对象、知识、权利、搜索结果、人工灵感和任务输入不收录到“创作结果”，也不会自动进入“我的资产”。
- 交付包通过交付投影展示，不复制为新的资产类型。

结果类型由对象类型和版本化业务 Schema 确定；状态由批准和交付事实推导。人工标签只补充检索，不改变事实类型、状态和权利规则。

## 7. 去重与版本

去重分为两层：

1. **对象身份去重：** 相同 `subject_ref` 只有一个目录项。
2. **内容相似提示：** 相同 SHA-256、规范 URL 或稳定业务摘要可形成重复组，供运营查看，但不自动合并不同事实对象。

自动合并会丢失权利、来源和批准历史，因此首期只提供重复提示。版本替代规则：

- 新版本创建新的事实对象或版本引用。
- 目录可以把新版本标记为 `current_version_ref`。
- 旧版本保留为 `superseded` 历史，不改写进行中的任务输入。

## 8. 可复用状态推导

```text
tenant/project access
  + subject status
  + source validity
  + rights status and time window
  + usage mode
  + channel/territory policy
  + sensitivity/visibility
  = reuse_state + blocking_reasons
```

优先级从高到低：

1. 跨租户、无权限或 Restricted 不允许披露：`blocked`。
2. 来源撤回、权利拒绝或用途禁止：`blocked`。
3. 权利过期：`expired`。
4. 等待权利或事实审核：`needs_review`。
5. 仅供分析或参考：`reference_only`。
6. 已被新版本替代：`superseded`。
7. 所有检查通过：`available`。

服务端在创建任务输入时重新计算，不信任浏览器提交的 `reuse_state`。

## 9. 失效传播和历史稳定性

```text
RightsRecord expired / SourceRevision invalidated
                    |
                    v
          catalog item becomes blocked
                    |
          +---------+----------+
          |                    |
          v                    v
   阻止进入新任务       Lineage impact query
                               |
                     标记未完成任务和待交付结果
```

- 已固定的历史任务引用不被静默替换。
- 尚未发生受限制副作用的任务在下一安全门禁重新校验并进入明确阻断。
- 已交付作品保留审计和历史可读性，但不因此继续获得新用途授权。
- 运营可以查看影响范围，修复底层事实后由投影恢复可用状态。

## 10. 查询与命令边界

### 客户查询

- 我的资产：按文件夹、资料类型、来源、项目、处理状态、关键词、时间和稳定游标查询。
- 创作结果：按结果类型、项目、确认状态、关键词、时间和稳定游标查询。
- 最近使用：组合两个投影的客户可见摘要，不复制正文或状态。
- 详情：按对象种类返回专用 DTO；客户端不得依靠大量可空字段猜测是哪一种资产。
- 选择器：仅返回与目标任务、渠道和用途兼容的工作区资料或已确认结果。

### 客户命令

- 创建、重命名和移动文件夹组织关系。
- 上传、导入或登记工作区资料，并管理它在客户工作区中的位置。
- 对固定资料版本发起预览、OCR、转写、摘要或标签建议任务。
- 把一个可用工作区资料引用加入 WorkTask 输入。
- 把一个已确认结果资产引用加入 WorkTask 输入。
- 从资产启动一个新任务。

“把来源保留为项目参考”属于任务输入/项目参考命令，不由资产目录提供；资产详情只能通过受控摘要追溯来源任务，不能反向修改输入事实。

### 运营查询和命令

- 查询失效、待审核、重复组、敏感预览和投影延迟。
- 调用拥有域命令处理来源、权利和批准问题。
- 触发幂等的单项重投影或全量重建。

组合查询层不提供 `updateCatalogItemStatus`、`replaceBody`、跨投影移动或“强制可用”命令。

## 11. 一致性、性能和容量

- 列表和搜索读取专用投影；BFF 只在“最近使用”或统一搜索需要时并行查询并稳定合并，不在每次请求中跨域加载正文。
- 创建任务、正式生成和交付等高风险命令必须回源校验，不能只信最终一致投影。
- 投影返回 `generated_at` 和 `projection_cursor`，延迟超过安全阈值时禁用危险动作。
- 列表使用稳定游标、有限 page size 和受控缩略图，不返回完整正文。
- 重建按 tenant/project 分区，支持检查点和重复执行。
- 首期不建立通用向量检索基础设施；关键词、标签、对象字段和稳定摘要足以验证价值。

## 12. 事件与重建

优先消费现有领域事件；事件不足时使用有所有者、游标和退场条件的兼容变更记录。不同投影分别消费：

- 工作区资料投影：`workspace_material_uploaded`、`workspace_material_imported`、`workspace_material_moved`、`workspace_material_versioned`。
- 任务输入/项目参考投影：`project_reference_saved`。
- 结果资产投影：`result_revision_created`、`approved_snapshot_created`、`artifact_created`。
- 交付投影及结果状态派生：`delivery_package_created`。
- 失效传播：`business_subject_invalidated`。

事件只携带 tenant、subject ref、version/digest、correlation 和小型变更摘要。投影器按事件 ID 幂等；乱序事件通过事实对象当前版本和事件发生时间收敛。

## 13. 测试矩阵

| 路径 | 必测情况 |
| --- | --- |
| 工作区收录 | 明确上传/导入后出现；搜索候选、来源证据和临时文件不自动出现 |
| 文件组织 | 创建/移动/重命名文件夹不改变文件版本、摘要、权利或任务引用 |
| 文件处理 | 上传中、解析中、成功、失败、重试、超大文件、恶意文件和连接器授权过期 |
| 结果收录 | 结果快照和 Artifact 自动出现；来源、项目参考和临时候选不进入创作结果 |
| 去重 | 同一 subject 重放不重复；相同摘要不同权利只提示不合并 |
| 引用 | 固定版本和摘要；页面陈旧时服务端拒绝 |
| 权利 | 渠道、地区、起止时间、拒绝、过期和仅分析模式 |
| 隔离 | 跨租户、跨项目、Restricted 预览和预签名地址 |
| 失效 | 来源撤回和权利过期阻止新使用并产生影响查询 |
| 重建 | 重放无外部副作用，结果摘要一致，重复事件幂等 |
| 投影延迟 | 显示延迟，危险动作禁用，回源校验仍正确 |
| 容量 | 分页、10k 目录项、缩略图失败和重建检查点 |

## 14. 非目标

- 不改变现有 `Asset` 的参考素材语义。
- 不把 `WorkspaceMaterialItem`、`CreativeAssetCatalogItem` 和交付 DTO 合并成万能契约。
- 不把目录投影当作 Runtime 输入正文或批准事实。
- 不自动合并不同来源、权利或批准链的相似内容。
- 不建设桌面全盘同步、企业网盘替代、关系图数据库或低代码资产图；文件夹、上传/导入和基础预览属于当前明确需求。
- 不在首期让模型自动决定权利、可见性和正式分类。
