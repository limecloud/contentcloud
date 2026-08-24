# 视频生产迁移 阶段 E1：Composition Manifest 计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件跟踪阶段 E1 的版本化合成清单设计与实现。它只定义合成输入的固定格式、摘要和幂等边界，不新增 Asset、Task、Review、Delivery 或发布事实模型。阶段状态仍以[视频生产迁移推进计划](./16-video-production-progress-plan.md)为准。

## 1. 目标

把一次最终成片执行所依赖的事实收敛为一个可复现的、不可变的 manifest：

```text
ApprovedSnapshot
  + selected video Artifact / MediaReview
  + narration Artifact
  + subtitle source
  + brand / CTA settings
  + timeline
  + renderer capability
  -> canonical composition manifest
  -> manifest digest
```

manifest 是合成 Worker 的输入快照，不是新的业务状态。Worker 必须通过现有应用服务读取并验证其中引用，不能把 manifest 中的显示字段当成权限或审核事实。

## 2. Manifest 契约

实现时使用稳定 JSON 表示，字段顺序由 canonical encoder 统一决定；禁止把随机 ID、当前时间、临时路径或运行日志写入摘要输入。

| 区块 | 必填内容 | 约束 |
| --- | --- | --- |
| `schema` | `name`、`version` | 版本变更必须产生新的 manifest digest；不兼容变更不得复用旧成片 |
| `approved_snapshot` | `id`、`digest` | 只能引用已批准的 `ApprovedSnapshot`；租户和项目必须由服务端重新校验 |
| `selected_video` | `artifact_id`、`digest`、`media_review_id`、`review_digest` | Artifact 必须是已通过选择的 `MediaReview(content)` 结果 |
| `narration` | Artifact 引用和 digest，语言/声音配置摘要 | 没有旁白时必须显式写 `disabled`，不得隐式发现或沿用旧输入 |
| `subtitles` | 来源引用或内嵌文本摘要、语言、样式版本 | 文本、分段和样式摘要必须固定；不能在 Worker 中重新读取可变草稿 |
| `brand_cta` | 品牌配置摘要、CTA 配置摘要 | 配置必须来源于批准输入或受控运行参数，并可审计 |
| `timeline` | 画布、帧率、时长、片段顺序、转场版本 | 时间单位统一为整数毫秒；片段顺序和边界必须确定 |
| `renderer` | renderer 名称、版本、capability digest | renderer 升级不得静默复用旧 manifest |
| `output` | `kind=final_render`、媒体格式和目标摘要 | 只描述输出契约，不提前创建 Artifact 或 DeliveryPackage |

## 3. 摘要与幂等规则

- canonical JSON 使用 UTF-8、稳定字段顺序、明确的空值策略和固定数字格式。
- `manifest_digest = SHA-256(canonical_manifest_bytes)`；digest 输入不包含创建时间、执行 ID 或临时文件路径。
- 同一 `ApprovedSnapshot`、输入 Artifact 摘要、时间轴、配置和 renderer 版本必须得到同一 digest，重复执行只能复用同一逻辑结果。
- 任一输入摘要、审核摘要、租户、项目或 renderer capability 发生变化时，必须生成新 digest；旧 `final_render` 不得覆盖或重新标记。
- manifest 本身不授予审批或交付权限；进入 `DeliveryPackage` 仍需通过既有 `MediaReview(final)` 和交付门禁。

## 4. 本轮清单

- [x] E1a：完成 manifest 字段、摘要输入、版本和边界的文档契约。
- [x] E1b：在 Go 值对象中实现 canonical 编码、校验和 digest 计算；补齐旁白/字幕显式禁用、时间轴片段边界和重复 ID 校验。
- [x] E1c：将 `CreateFinalRender` 改为消费 manifest，并拒绝摘要漂移和不完整输入。
- [x] E1d-a：完成正常、重复、输入漂移、缺输入和失败恢复测试矩阵的文档基线。
- [x] E1d-c：运行最终审核拒绝测试，pending `MediaReview(final)` 不得创建 `DeliveryPackage`。
- [x] E1d-b-Memory：实现并运行 Memory 原子写入成功/失败测试。
- [ ] E1d-b：实现并运行 PostgreSQL 与 Blob 失败恢复测试；已新增 PostgreSQL 原子事务测试入口，应用层 Blob `Get`/`Put` 故障注入、临时 Blob 清理和清理失败诊断已接入，剩余真实数据库执行、后台重试、跨进程恢复和组合幂等见 [E1d-b Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md)。
- [x] E1e-port：Composition Worker 端口已接入 `CreateFinalRender`；默认 Worker 生成独立 MP4，并把 Manifest、输入摘要和辅助 Artifact 血缘固化到输出元数据。
- [ ] E1e-native：补齐具备真实音视频编码能力的 Worker 运行证据，并证明旁白、字幕、时间轴和品牌/CTA 实际生效。

本轮 E1b 证据：`internal/delivery/composition_manifest.go`、`internal/delivery/composition_manifest_test.go`；覆盖稳定摘要、时间轴输入变化产生新摘要，以及旁白/字幕隐式输入拒绝。E1b 只完成值对象层，不代表现有 `CreateFinalRender` 已接入 manifest。

本轮 E1c 证据：`internal/application/storyboard_media.go` 已强制要求 Manifest，并重新读取批准快照、选中视频审核和时间轴 Artifact；`internal/application/media_pipeline_test.go` 覆盖缺 Manifest、快照摘要漂移和同一 digest 重复执行不重复创建 Artifact/审核；开发 Fixture 已显式构造 Manifest。`internal/integration/composition/worker.go` 提供 `contentcloud.deterministic-compositor/1.0.0` Worker 端口实现和确定性独立 MP4 输出；真实旁白、字幕混音、时间轴画面处理和品牌/CTA 烧录仍待受控 native Worker 验收。

本轮输入消费补强（2026-08-23）：应用层拒绝没有受控正文输入的 `subtitles.source_kind=inline` Manifest，避免“声明了字幕但 Worker 未收到正文”的静默成功；字幕正文必须登记为批准快照范围内的 Artifact。统一 Artifact/DeliveryPackage Blob 写入也会在事实拒绝或 Blob 写入后报错时补偿清理临时对象，相关回归证据见 `internal/application/artifact_storage_test.go`。

E1d-a/E1d-c 证据：已新增[失败、恢复与摘要漂移验证计划](./19-stage-e1d-failure-recovery-plan.md)，固定七类场景的预期结果；`TestMarketingVideoGoldenJourney` 已证明 pending 最终审核不能创建交付包。E1d-b-Memory 已由 `internal/persistence/memory/final_render_test.go` 覆盖原子成功/失败；本轮新增数据库失败后的临时 Blob 清理、清理失败诊断、Blob `Get`/`Put` 故障注入和 PostgreSQL 原子事务测试入口；真实 PostgreSQL 执行、后台重试、跨进程恢复、组合幂等或真实 Worker 证据仍待补齐。

## 5. 完成门槛

E1 只有在 E1b-E1e 全部完成后才能标记为 `complete`：

1. canonical 编码和 digest 在 Memory、PostgreSQL 及不同进程中一致。
2. 输入摘要漂移、租户/项目不匹配和缺少批准审核会被拒绝。
3. 重复执行不重复创建收费、Artifact、审核或交付事实。
4. 合成失败不会产生可交付的最终审核结果，恢复可从已有事实继续。
5. 至少一次真实 Worker 运行证明旁白、字幕、时间轴和品牌/CTA 均来自 manifest。

## 6. 回写协议

完成 E1a-E1e 任一项后，先更新本文件，再同步：

1. `13-video-production-execution.md` 的阶段 E 证据；
2. `16-video-production-progress-plan.md` 的队列和状态；
3. `12-video-production-migration.md`、`docs/infra/01-capability-map.md` 和 `docs/infra/03-delivery-roadmap.md` 的能力边界。
