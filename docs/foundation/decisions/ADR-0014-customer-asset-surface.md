# ADR-0014：客户资产入口组合工作区资料与创作结果

状态：`Accepted`

日期：2026-08-07

决策者：产品、内容运营、平台工程与设计

关联：

- [ADR-0011：统一创作资产目录采用只读投影](./ADR-0011-creative-asset-catalog-projection.md)
- [ADR-0013：客户创作结果类型与复用门禁](./ADR-0013-customer-result-asset-boundary.md)
- [客户资产产品需求](../../product/creative-asset-library/README.md)

## 背景

ADR-0013 正确分离了生成结果、任务输入和交付事实，也冻结了生成结果的确认与复用门禁。但它进一步把客户“资产”页面收窄为人物原型、剧本、分镜、图片和视频结果，导致客户上传或导入的文档、图片、视频、音频和表格没有稳定的长期工作区。

客户对“资产”的直觉不是某个底层领域对象，也不只是一组已经确认的生成结果。它是一个可以整理、查找、理解并用于创作的工作入口。客户需要同时找到自己带进平台的资料和平台为其生成的结果，但不应因此看到来源治理、权利记录、Runtime 事件或一套万能数据模型。

## 决策

### 1. “资产”是统一客户入口，不是统一写模型

客户一级导航保留一个简单的“资产”入口，页面由三个视图组成：

```text
资产
├── 我的资产
│   ├── 文件夹
│   ├── 文档
│   ├── 图片
│   ├── 视频
│   ├── 音频
│   ├── 表格
│   └── 其他文件
├── 创作结果
│   ├── 人物原型
│   ├── 剧本
│   ├── 分镜
│   ├── 图片
│   └── 视频
└── 最近使用
```

- **我的资产：** 客户主动上传、导入或登记并由当前租户拥有或管理的工作区资料。
- **创作结果：** 流水线生成的人物原型、剧本、分镜、图片和视频结果。
- **最近使用：** 对前两类只读结果的时间排序，不形成第三套资产事实。
- **交付：** 继续使用独立一级入口，负责核对和下载正式交付包。

文件类型筛选放在“我的资产”视图内，结果类型与确认状态筛选放在“创作结果”视图内。不得把所有类型平铺为一级导航。

### 2. 客户 BFF 组合两个专用投影

```text
Customer Asset Surface
├── WorkspaceMaterialProjection
│   ├── WorkspaceFolderItem
│   └── WorkspaceMaterialItem
└── CreativeResultAssetProjection
    └── CreativeAssetCatalogItem
```

`WorkspaceMaterialProjection` 和 `CreativeResultAssetProjection` 拥有不同的收录规则、状态和命令。客户 BFF 可以把它们组合到同一页面，但不得创建可写的超级 `Asset`、共享状态枚举或把一个投影复制进另一个投影。

迁移期已经存在的 `StudioAssetItem` / `CreativeAssetCatalogItem` 只继续表达生成结果行契约。不得为了快速支持“我的资产”而给它增加文件夹、上传、音频、表格和处理状态等不相干字段。新增能力先冻结窄契约：

```text
WorkspaceFolderItem
├── folder_ref / parent_ref
├── name / project_scope
├── child_count
└── created_at / updated_at

WorkspaceMaterialItem
├── material_ref / folder_ref
├── material_kind
├── origin
├── title / mime_type / size
├── preview_ref / processing_state
├── project_scope / usage
├── rights_summary
└── created_at / updated_at
```

文件夹只负责组织和导航，不拥有文件正文、权利、任务引用或结果状态。

### 3. 分类、来源、用途和状态使用独立轴

```text
material_kind: document / image / video / audio / table / other
origin: uploaded / imported / linked
usage: project_material / project_reference
processing_state: uploading / processing / ready / failed

result_type: persona / script / storyboard / image / video
result_status: draft / pending_confirmation / changes_requested /
               confirmed / delivered / superseded / blocked
```

- `result_status` 只适用于创作结果，普通文件不进入结果确认状态机。
- `processing_state` 只说明文件接收、解析或预览是否完成，不代表内容质量、权利或批准结论。
- 文件类型不表达来源，来源不表达用途，用途也不表达权利状态。
- 生成结果如果需要作为普通文件导出或下载，仍引用同一底层结果或 Artifact，不复制为另一份正文。

### 4. 收录必须由客户拥有关系或明确动作触发

以下内容可以进入“我的资产”：

- 客户从本地上传的文件。
- 客户通过受支持连接器明确导入的文件。
- 客户明确登记并有稳定访问权限的外部文件链接。
- 客户创建的文件夹与组织关系。

以下内容不会因为被搜索、抓取或执行步骤读取就自动成为客户资产：

- 搜索 API 或爬虫返回的候选。
- 来源证据、网页快照和引用片段。
- 运营知识对象、权利记录和内部治理标签。
- Runtime 上下文、执行事件、模型响应和临时文件。

“保留为项目参考”只把候选固定到当前项目的参考范围。只有客户进一步执行“导入到我的资产”，并完成权限、摘要和必要权利登记后，才创建工作区资料引用。

### 5. AI 理解是派生能力，不修改原始资产事实

客户可以对工作区资料执行预览、OCR、转写、摘要、标签建议和“加入本次创作”等动作。派生结果必须引用固定的 `material_ref + version/digest`，可以重新生成，不得覆盖原文件或被当作权利、真实性和批准结论。

需要模型或 Agent 的理解任务通过正常 WorkTask/Runtime 路径执行；确定性的元数据提取、媒体探测和格式转换优先由普通 Worker 完成。客户不需要选择 Codex、Claude Code、MCP 或具体服务商。

## 事实所有权与边界

| 能力 | 事实所有者 | 客户投影边界 |
| --- | --- | --- |
| 上传/导入文件、版本、摘要和组织位置 | Source & Knowledge / Artifact 既有拥有域，具体以实现契约冻结为准 | `WorkspaceMaterialProjection` 只读引用 |
| 文件夹和客户组织关系 | 客户工作区模块 | `WorkspaceFolderItem` |
| 生成结果、批准与交付关联 | 对应内容域、Review & Approval、Artifact & Delivery | `CreativeResultAssetProjection` |
| 搜索候选、来源证据与权利 | Source & Knowledge / Rights | 任务参考或运营投影，不自动进入资产入口 |
| 最近使用 | Experience Projection | 可重建查询，不拥有业务正文 |

Runtime 只消费固定版本引用和最小上下文，不拥有文件、文件夹、生成结果、批准或交付状态。

## 兼容与迁移

- ADR-0013 继续拥有生成结果类型、结果状态、确认门禁和 `CreativeResultAssetProjection`；其中“客户资产页只展示生成结果”和固定五类一级筛选的结论由本 ADR 修订。
- 当前代码已经实现 `WorkspaceMaterialProjection` 的首切片：文件夹创建、客户上传、受控预览、固定资料引用、加入同项目任务和最近使用；连接器导入、外链登记、移动/删除和 AI 理解仍未实现。`CreativeResultAssetProjection` 仍由任务事实动态组装，持久化 Projector 待完成。
- 先冻结 `WorkspaceFolderItem`、`WorkspaceMaterialItem` 与命令契约，再实现客户 BFF 组合；不直接扩张现有 `StudioAssetItem`。
- 旧“品牌资料”或上传资料按事实所有权迁入工作区资料投影；迁移不得复制正文或丢失版本、摘要和权利引用。
- 客户路由可以继续使用 `/assets`，通过视图参数或子路由区分 `mine / results / recent`，避免新增多个一级导航。

## 安全与运行影响

- 所有列表、预览、下载、导入和加入任务动作都重新校验 tenant、project、membership 和对象可见性。
- 外部链接必须经过允许协议、域名和重定向校验；不得由服务端任意抓取客户提供的 URL。
- Confidential 和 Restricted 文件不得进入公开索引或无权限缩略图缓存。
- AI 理解任务只接收完成当前动作所需的最小资料，输出继承原资料的数据分类。
- 普通文件处理失败不能污染生成结果状态；结果投影延迟也不能阻止客户管理已经上传的资料。

## 验证

1. 客户进入“资产”后，可以在“我的资产”和“创作结果”之间切换，无需理解底层领域对象。
2. 客户可以创建文件夹、上传或导入资料、搜索和预览，并把一个可用资料加入当前创作。
3. 生成结果继续显示独立类型、确认状态和复用门禁，只有 `confirmed` 和 `delivered` 可正式复用。
4. 搜索候选、来源证据、权利记录、Runtime 事件和交付包不会混入“我的资产”。
5. “最近使用”重建后与两个权威投影一致，不产生正文副本。
6. 同名文件、同摘要不同权利、跨租户引用、超大文件、处理失败和陈旧页面均有明确测试与恢复行为。

## 回退

“我的资产”工作包使用独立 Feature Flag。回退时先关闭上传/导入和加入任务命令，再隐藏 `mine` 视图；已登记文件、文件夹、任务引用和审计记录保留。创作结果视图与既有任务复用闭环继续可用，不需要回退 ADR-0013 的结果规则。

## 后果

正面后果：客户获得符合直觉的资产工作区，又能清楚区分自有资料和平台生成结果；未来新增文件类型或结果类型时只扩展对应投影。

负面后果：客户 BFF 需要组合两套查询和权限结果；搜索、最近使用和跨视图动作需要显式处理投影延迟与部分失败。

新增约束：任何新资产能力必须先回答它属于工作区资料、创作结果、任务参考还是交付，不得通过扩张万能 DTO 来回避领域归属。
