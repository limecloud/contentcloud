# 视频生产迁移 阶段 E：确定性合成输入与血缘计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件是 视频生产迁移阶段 E 的短周期计划。它只定义受控 Worker 的输入、输出和验收顺序，不新增 Asset、Task、Review 或 Delivery 事实模型。阶段总状态仍以[迁移推进计划](./16-video-production-progress-plan.md)和[迁移计划](./12-video-production-migration.md)为准；E1 的字段级跟踪见[E1 Composition Manifest 计划](./18-stage-e1-composition-manifest-plan.md)。

## 1. 本轮目标

把 视频生产迁移 的视频后期、旁白、字幕和品牌/CTA 合成收敛为一个可重复验证的 ContentCloud Artifact 生产步骤：

```text
ApprovedSnapshot
  + selected generated Artifact
  + narration/audio Artifact
  + subtitle/caption source
  + brand/CTA settings
  + timeline/presentation manifest
  -> deterministic composition Worker
  -> final_render Artifact
  -> MediaReview(final)
  -> DeliveryPackage
```

## 2. 输入契约审计（已完成）

- [x] 确认批准内容只能从 `ApprovedSnapshot` 读取，不能以 视频生产迁移 本地文件或 Session 作为事实源。
- [x] 确认候选视频必须先通过 `MediaReview(content)`，并以选中审核的 `SubjectArtifactID` 和摘要作为输入。
- [x] 确认旁白、字幕、品牌/CTA 和时间轴需要固定为带摘要的输入清单，不能由 Worker 在执行时隐式发现。
- [x] 确认输出必须是既有 `Artifact(kind=final_render)`，并创建 `MediaReview(final)`；只有最终审核批准后才能进入 `DeliveryPackage`。
- [x] 确认重复执行必须按合成 manifest digest 幂等，输入摘要漂移时不得复用旧成片。

审计依据：`internal/application/storyboard_media.go`、`internal/application/media_pipeline.go`、`internal/delivery/work_delivery_model.go`、`internal/application/stage_outputs.go`，以及 视频生产迁移 的 `jianying_draft_service.py`、`narration_delivery_tasks.py`、`video_artifact_currency.py` 和 `presentation_bundle.py`。

这项审计只完成契约对账，不代表阶段 E 已完成真实音视频编码。当前 `CreateFinalRender` 已改为受 Manifest 驱动并执行事实校验，并通过平台 Composition Worker 端口生成独立 MP4；默认纯 Go Worker 固化完整输入血缘元数据，但不能宣称已完成旁白混音、字幕烧录或品牌画面合成。

输入消费边界补充：当前应用层不接受没有受控正文的 `inline` 字幕来源；字幕正文必须先登记为批准快照范围内的字幕 Artifact，避免 Worker 只记录声明而没有实际输入。Artifact/DeliveryPackage 的临时 Blob 清理也已统一接入，失败恢复仍需真实 PostgreSQL/Blob 组合验收。

## 3. 实施队列

- [x] E1a：完成版本化 composition manifest 的字段、摘要、幂等和权限边界文档契约。
- [x] E1b：实现 canonical manifest 值对象、校验和 digest；旁白/字幕必须显式启用或禁用，时间轴片段边界固定。
- [x] E1c：将 manifest 接入 `CreateFinalRender`，重新校验快照、审核、Artifact、时间轴和输出契约，并按 digest 幂等复用。
- [x] E1d-a：完成失败、恢复、重复执行和摘要漂移测试矩阵文档基线，见[阶段 E1d 验证计划](./19-stage-e1d-failure-recovery-plan.md)。
- [x] E1d-c：通过主链测试证明 pending 最终审核不能创建 `DeliveryPackage`。
- [x] E1d-b-Memory：运行 Memory 原子写入成功/失败测试。
- [ ] E1d-b：运行 PostgreSQL 与 Blob 失败恢复矩阵，并补齐失败后的恢复断点；本轮已完成应用层 Blob `Get`/`Put` 故障注入、数据库写入失败后的临时 Blob 清理和清理失败诊断，详见 [E1d-b Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md)。
- [ ] E1e：补齐真实 Worker 验收。
- [x] E2-port：实现 Composition Worker/Adapter 端口和默认确定性 Worker；输出为独立 `final_render` Artifact，包含 Manifest、时间轴、旁白/字幕、品牌/CTA 和输入摘要血缘。
- [ ] E2-native：接入具备真实音视频编码能力的受控 Worker，实际生成旁白、字幕和品牌/CTA 合成结果。
- [ ] E3：补齐正常、失败、重复、输入摘要漂移和恢复测试；失败不得创建可交付最终产物。
- [ ] E4：用最终 `MediaReview` 和 `DeliveryPackage` 验证合成结果可以进入既有交付链。

## 4. 完成门槛

只有同时满足以下条件，阶段 E 才能从 `not_started`/`in_progress` 更新为 `complete`：

1. manifest digest 稳定且包含所有外部输入摘要、版本和 renderer 标识。
2. 相同输入重复执行返回同一逻辑结果，不重复创建收费或交付事实。
3. 输入摘要漂移、媒体缺失或合成失败时，旧 Artifact 不会被覆盖，也不会创建可交付的最终审核。
4. 产物、最终审核和交付包的租户、项目、ApprovedSnapshot 和摘要血缘可查询。
5. 至少一次具备真实音视频编码能力的 Worker 运行证据补齐；默认确定性元数据 Worker、Fixture 或测试输出不能替代真实合成验收。

## 5. 回写规则

完成 E1-E4 任一项后，按顺序更新：

1. 本文件的实施队列和证据。
2. `13-video-production-execution.md` 的阶段 E 记录。
3. `12-video-production-migration.md`、`16-video-production-progress-plan.md` 的状态和下一步。
4. `docs/infra/01-capability-map.md`、`docs/infra/03-delivery-roadmap.md` 的真实能力边界。
