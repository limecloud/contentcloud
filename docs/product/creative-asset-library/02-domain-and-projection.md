# 创作资产目录领域与投影规范

状态：`目标架构，待 ADR 接受和实现`。

更新时间：2026-08-05。

## 1. 核心决定

统一创作资产目录使用跨域只读投影，不创建吞并所有内容的超级 `Asset` 聚合。

原因：现有对象拥有不同生命周期、权利规则和不可变语义。把它们复制进一个可写 `Asset` 会产生第二套正文、第二套状态和无法可靠传播的失效关系。

## 2. 现有事实对象

| 对象 | 当前语义 | 事实所有者 | 目录如何使用 |
| --- | --- | --- | --- |
| `Source` | 一项来源的逻辑身份 | Source & Knowledge | 展示来源名称和状态 |
| `SourceRevision` | 不可变来源版本和摘要 | Source & Knowledge | 作为可复用来源版本 |
| `Asset` | 已登记、受用途和权利约束的参考素材 | Source & Knowledge | 展示参考素材和权利可用性 |
| `RightsRecord` | 渠道、地区、期限和限制 | Source & Knowledge | 推导可复用状态，不复制为目录状态 |
| `KnowledgeObject` | 有证据和治理状态的知识 | Source & Knowledge | 展示批准品牌、产品和人物事实 |
| `ApprovedSnapshot` | 客户批准后的不可变内容版本 | Review & Approval | 自动沉淀人物、剧本和正式内容 |
| `Artifact` | 从批准内容生成的生产文件 | Artifact & Delivery | 提供预览和下载引用 |
| `DeliveryPackage` | 正式交付集合 | Artifact & Delivery | 展示已交付作品和清单 |
| `PerformanceObservation` | 对已批准内容的效果观察 | Performance & Learning | 首期不作为资产正文，后续只提供效果摘要 |

现有代码事实以 `internal/domain/content.go`、`internal/app/assets.go`、`internal/app/lineage.go` 和 `internal/domain/projection.go` 为准。

## 3. 统一目录投影

逻辑读模型：

```text
CreativeAssetCatalogItem
├── schema_version
├── catalog_item_id
├── tenant_id / project_scope
├── primary_category / collections[] / asset_kind
├── display
│   ├── name / summary / tags
│   └── preview_ref
├── subject_ref
│   ├── subject_type / subject_id
│   ├── version_id / digest
│   └── created_at
├── origin_refs[]
├── rights_summary
├── reuse_state / blocking_reasons[]
├── sensitivity / visibility
├── usage_summary
├── current_version_ref
└── generated_at / projection_cursor
```

约束：

- `catalog_item_id` 由 tenant、subject type、subject id 和版本策略确定性生成。
- `primary_category` 表达内容类型；`collections` 表达已批准、已交付等可重叠系统视图，不能为每个视图复制目录项。
- `subject_ref` 指向事实对象；目录不保存正文和原始媒体副本。
- `preview_ref` 必须是权限受控引用，不直接暴露 Blob ObjectKey。
- `rights_summary` 是查询摘要，权威内容仍是 `RightsRecord`。
- `reuse_state` 是派生结果，不能通过通用目录更新 API 直接修改。
- 投影可以删除和重建，重建不得调用模型、执行者或外部服务。

## 4. 资产引用契约

客户把资产加入任务时生成版本化引用：

```text
CreativeAssetRef
├── catalog_item_id          仅用于产品追溯
├── subject_type / subject_id
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

### 4.1 现有对象的版本与摘要映射

| subject type | `subject_version_id` | `subject_digest` | 说明 |
| --- | --- | --- | --- |
| `source_revision` | `SourceRevision.ID` | `SourceRevision.SHA256` | 修订本身不可变 |
| `asset` | `Asset.SourceRevisionID` | 对应 `SourceRevision.SHA256` | `Asset.ID` 仍是 subject ID；权利策略另存于 validation snapshot |
| `knowledge_object` | `KnowledgeObject.Version` | `KnowledgeObject.Digest` | 只收录满足治理策略的版本 |
| `approved_snapshot` | `ApprovedSnapshot.ID` | `ApprovedSnapshot.ContentHash` | 快照本身不可变 |
| `artifact` | `Artifact.ID` | `Artifact.SHA256` | 文件摘要固定具体内容 |
| `delivery_package` | `DeliveryPackage.ID` | 规范化 Manifest 摘要 | 当前对象未显式保存该摘要，实施前必须通过契约补齐或确定性计算 |

`subject_version_id` 对不可变对象可以等于 subject ID；契约必须允许字符串和明确的规范化表示，不能让消费者根据对象类型猜测“最新版本”。

## 5. 收录和分类规则

```text
业务事实发生变化
  -> 领域事件或兼容变更记录
  -> Catalog Projector 读取事实引用
  -> 应用收录策略、分类和可见性
  -> 生成或更新 CreativeAssetCatalogItem
  -> 客户与运营投影分别裁剪字段
```

首期使用显式、确定性规则，不用模型决定是否进入资产库：

- 来源被客户保存、选择或固定时收录。
- `Asset` 登记后收录，状态由权利和用途推导。
- `ApprovedSnapshot` 创建后收录。
- 合法 `Artifact` 和 `DeliveryPackage` 创建后收录。
- 临时模型候选、未选择搜索结果、运行日志和失败中间文件不收录。

主分类可以由对象类型和版本化业务 Schema 确定；批准和交付集合由事实关系确定。人工标签只补充检索，不改变事实类型、系统集合和权利状态。

## 6. 去重与版本

去重分为两层：

1. **对象身份去重：** 相同 `subject_ref` 只有一个目录项。
2. **内容相似提示：** 相同 SHA-256、规范 URL 或稳定业务摘要可形成重复组，供运营查看，但不自动合并不同事实对象。

自动合并会丢失权利、来源和批准历史，因此首期只提供重复提示。版本替代规则：

- 新版本创建新的事实对象或版本引用。
- 目录可以把新版本标记为 `current_version_ref`。
- 旧版本保留为 `superseded` 历史，不改写进行中的任务输入。

## 7. 可复用状态推导

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

## 8. 失效传播和历史稳定性

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

## 9. 查询与命令边界

### 客户查询

- 列表：分类、关键词、项目、状态、对象类型、时间和稳定游标。
- 详情：客户可见摘要、预览、来源、权利、版本、使用历史和允许动作。
- 选择器：仅返回与目标任务、渠道和用途兼容的资产。

### 客户命令

- 保存一个任务内来源到资产库。
- 把一个可复用资产引用加入 WorkTask 输入。
- 从资产启动一个新任务。

### 运营查询和命令

- 查询失效、待审核、重复组、敏感预览和投影延迟。
- 调用拥有域命令处理来源、权利和批准问题。
- 触发幂等的单项重投影或全量重建。

目录层不提供 `updateCatalogItemStatus`、`replaceBody` 或“强制可用”命令。

## 10. 一致性、性能和容量

- 列表和搜索读取专用投影，不在每次请求中跨域实时拼接所有对象。
- 创建任务、正式生成和交付等高风险命令必须回源校验，不能只信最终一致投影。
- 投影返回 `generated_at` 和 `projection_cursor`，延迟超过安全阈值时禁用危险动作。
- 列表使用稳定游标、有限 page size 和受控缩略图，不返回完整正文。
- 重建按 tenant/project 分区，支持检查点和重复执行。
- 首期不建立通用向量检索基础设施；关键词、标签、对象字段和稳定摘要足以验证价值。

## 11. 事件与重建

优先消费现有领域事件；事件不足时使用有所有者、游标和退场条件的兼容变更记录。建议事件语义：

- `source_revision_saved_for_reuse`
- `asset_registered`
- `rights_record_reviewed`
- `approved_snapshot_created`
- `artifact_created`
- `delivery_package_created`
- `business_subject_invalidated`

事件只携带 tenant、subject ref、version/digest、correlation 和小型变更摘要。投影器按事件 ID 幂等；乱序事件通过事实对象当前版本和事件发生时间收敛。

## 12. 测试矩阵

| 路径 | 必测情况 |
| --- | --- |
| 收录 | 保存来源、批准快照、Artifact、Delivery 自动出现；临时候选不出现 |
| 去重 | 同一 subject 重放不重复；相同摘要不同权利只提示不合并 |
| 引用 | 固定版本和摘要；页面陈旧时服务端拒绝 |
| 权利 | 渠道、地区、起止时间、拒绝、过期和仅分析模式 |
| 隔离 | 跨租户、跨项目、Restricted 预览和预签名地址 |
| 失效 | 来源撤回和权利过期阻止新使用并产生影响查询 |
| 重建 | 重放无外部副作用，结果摘要一致，重复事件幂等 |
| 投影延迟 | 显示延迟，危险动作禁用，回源校验仍正确 |
| 容量 | 分页、10k 目录项、缩略图失败和重建检查点 |

## 13. 非目标

- 不改变现有 `Asset` 的参考素材语义。
- 不把目录投影当作 Runtime 输入正文或批准事实。
- 不自动合并不同来源、权利或批准链的相似内容。
- 不建设通用 DAM、桌面同步盘、关系图数据库或低代码资产图。
- 不在首期让模型自动决定权利、可见性和正式分类。
