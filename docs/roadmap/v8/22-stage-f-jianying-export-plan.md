# 视频生产迁移 阶段 F：剪映草稿确定性导出计划

状态：`in_progress`。

更新时间：2026-08-23。

本计划跟踪 视频生产迁移 剪映草稿交付能力迁移。导出是 ContentCloud 现有
`DeliveryPackage` 的派生操作，不创建独立的剪辑项目、素材库、发布状态或
视频生产迁移 数据库。

## 1. 目标

将已批准的最终成片导出为可复现、可校验、可人工导入剪映的 ZIP 归档：

```text
ApprovedSnapshot
  -> approved MediaReview(final)
  -> CompositionManifest
  -> DeliveryPackage + Artifact/Blob
  -> deterministic Jianying ZIP
```

同一租户、项目、批准快照、最终审核、Manifest 和 Artifact 输入必须生成完全
相同的 ZIP 字节。导出不得改变审核、交付、渠道发布或效果观察状态。

## 2. 输出契约

ZIP 至少包含以下固定条目：

| 条目 | 内容 |
| --- | --- |
| `draft_info.json` | 剪映草稿元数据、schema/version、项目和输入摘要 |
| `manifest.json` | `CompositionManifest` 的 canonical JSON、digest 和交付血缘 |
| `assets/<safe-file-name>` | 从 DeliveryPackage manifest 读取并核验摘要后的素材 Blob |

实现必须固定 ZIP 条目顺序、时间戳、权限、压缩方式和 JSON 序列化格式。素材
文件名只能使用安全 basename，拒绝绝对路径、`..`、空名称和重复目标路径。

## 3. 输入和拒绝条件

导出器只接受以下已存在事实：

- 当前租户和项目下的 `ApprovedSnapshot`；
- 状态为批准且明确选中的 `MediaReview(final)`；
- 已通过 `CompositionManifest.Validate` 的 Manifest；
- 当前项目的 `DeliveryPackage`；
- DeliveryPackage manifest 中列出的 `Artifact` 及其 Blob 内容。

必须拒绝：

- 租户、项目、快照、审核或 DeliveryPackage 不匹配；
- 最终审核不是批准或没有明确选中；
- Artifact、审核、Manifest 或 Blob 摘要漂移；
- 最终 Artifact 不在 DeliveryPackage manifest 中；
- Blob 缺失、文件名路径穿越或导出目标重复。

拒绝时不得创建新的发布事实，也不得覆盖旧归档。

## 4. 实施队列

- [x] F0：完成 视频生产迁移 `presentation_bundle`/Jianying 行为审计，确定 ZIP、manifest、素材安全路径和幂等契约。
- [x] F1：实现 `internal/local/export/jianying.go` 及其纯 Go 确定性 ZIP 生成器；固定条目顺序、时间戳、权限和 `ZIP_STORED`。
- [x] F2：接入 ApprovedSnapshot、MediaReview、CompositionManifest、DeliveryPackage、Artifact 和 Blob 的完整输入校验；导出器只读，不写业务数据库。
- [x] F3：补正常、重复、摘要漂移、审核未批准、DeliveryPackage 缺 Artifact、路径穿越和 Blob 缺失测试；测试位于 `internal/local/export/jianying_test.go`。
- [x] F4：实现 `LintJianyingArchive`，验证 ZIP 条目、JSON 摘要、素材大小、SHA-256 和 MIME/扩展名契约。
- [x] F4b：接入分层平台的交付编排入口：`DeliveryService.ExportJianying`、`POST /api/bff/projects/{projectID}/jianying-export`、CLI `artifact jianying-export`；入口只读取 ApprovedSnapshot、MediaReview、DeliveryPackage、Artifact 和 Blob，不新增任务、审核、产物或交付模型。应用、HTTP 和 CLI 契约测试已通过。
- [ ] F5：用真实已批准最终 Artifact 运行本地导入验收；在真实验收前保持阶段 F 为 `in_progress`。

## 5. 代码边界

建议新增：

```text
internal/application/jianying_export.go
internal/application/jianying_export_test.go
internal/local/export/jianying.go
internal/local/export/jianying_test.go
internal/transport/http/content_handlers.go
internal/transport/http/jianying_export_handlers_test.go
internal/transport/cli/artifacts.go
internal/transport/cli/jianying_export_test.go
```

导出器可以读取现有 `persistence/blob.Store`，但不能直接写入业务数据库；如
需保存归档，必须通过现有 Artifact/Delivery 端口，并保留输入 digest。

## 6. 完成门槛

阶段 F 只有在以下证据全部具备后才能标记为 `complete`：

1. 同一输入生成完全相同的 ZIP 字节，重复执行不会产生第二个逻辑交付结果。
2. 所有输入均来自批准快照、批准最终审核和现有 DeliveryPackage。
3. 摘要漂移、越权、路径穿越、缺失 Blob 和非法 ZIP 条目均有失败测试。
4. 导出不改变 ChannelPublication、Receipt、PerformanceObservation 或任务交付状态。
5. 至少一次真实 Artifact 归档可以被人工导入剪映并通过导出 lint。

F1-F4b 已完成本地代码、分层入口、失败矩阵、导出 lint 和契约测试证据，但本文件仍不代表阶段 F 完成：F5 真实 Artifact 的人工剪映导入验收仍未完成。
