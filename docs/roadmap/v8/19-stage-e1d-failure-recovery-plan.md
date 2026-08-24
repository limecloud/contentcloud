# 视频生产迁移 阶段 E1d：失败、恢复与摘要漂移验证计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件跟踪 `CompositionManifest` 接入后的失败、恢复、重复执行和输入漂移验证。它只定义测试证据与真实 Worker 的验收顺序，不新增 Asset、Task、Review、Delivery 或发布事实模型。阶段 E1 的总状态以[迁移推进计划](./16-video-production-progress-plan.md)为准。

## 1. 本轮目标

证明 `CreateFinalRender` 在真实合成 Worker 接入前，已经具备可执行的拒绝和恢复边界：

```text
Manifest
  -> 输入/权限/摘要校验
  -> 确定性执行或明确失败
  -> 不覆盖旧 Artifact
  -> 仅由最终 MediaReview 决定是否可交付
```

本轮不把默认 `contentcloud.deterministic-compositor` 元数据 Worker 当作真实后期合成，也不以 Fixture 代替旁白、字幕、时间轴和品牌/CTA 的真实运行证据。

## 2. 验证矩阵

| 场景 | 预期结果 | 需要固定的事实 |
| --- | --- | --- |
| 正常执行 | 创建一个带 manifest digest 的 `final_render` Artifact 和最终审核 | ApprovedSnapshot、输入 Artifact、renderer、输出摘要 |
| 同一 digest 重复执行 | 复用既有逻辑结果，不创建第二个 Artifact、审核或收费事实 | manifest digest、租户、项目 |
| ApprovedSnapshot 摘要漂移 | 拒绝执行，不覆盖旧结果 | snapshot ID 与 digest |
| 视频/时间轴 Artifact 摘要漂移 | 拒绝执行，不复用旧成片 | Artifact ID 与 digest |
| 输入缺失或租户错配 | 拒绝执行，不创建最终审核 | 引用完整性、tenant/project |
| Worker 失败 | 保留可诊断失败，不产生可交付最终审核 | 错误摘要、已有输入血缘 |
| 失败后恢复 | 使用同一 manifest 继续，成功时只产生一个最终逻辑结果 | 幂等键、manifest digest |

## 3. 任务清单

- [x] E1d-a：完成失败、恢复、重复执行和摘要漂移测试矩阵，并登记每个场景的预期事实边界。
- [x] E1d-b-Memory：在 Memory 上验证 Artifact 与最终审核的原子成功/失败。
- [ ] E1d-b：在 PostgreSQL 上实现并运行矩阵测试，补齐 Blob 读写失败和恢复断点；已新增受 `CONTENTCLOUD_TEST_DATABASE_URL` 保护的 `TestPostgresFinalRenderAtomicityWithPostgres`，真实执行仍待专用数据库；Memory/应用层已完成 Blob `Get`/`Put` 故障注入、数据库失败后的临时 Blob 清理与清理失败诊断，详见 [E1d-b Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md) 和 [Blob 故障注入计划](./27-e1d-b-blob-fault-injection-plan.md)。
- [x] E1d-c：验证最终审核仍为 `pending` 时不会创建 `DeliveryPackage`；失败路径仍不能越过最终审核门禁。
- [ ] E1e：使用真实 Worker 证明旁白、字幕、时间轴和品牌/CTA 均来自 manifest。

## 4. 完成门槛

E1d 只有在 E1d-b 和 E1d-c 均有可重复测试记录后才能标记为 `complete`。当前 E1d-c 已有 Memory 主链测试记录，E1d-b-Memory 已有 `internal/persistence/memory/final_render_test.go` 原子成功/失败记录，PostgreSQL 原子事务测试入口已存在但因未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 明确 skip；PostgreSQL 真实执行、Blob 读写失败恢复仍未完成。E1e 仍需额外的 native Worker 运行记录；模拟 Provider、Fixture 和默认元数据 Worker 结果不能替代该门槛。

## 4.1 E1d-b Blob 清理证据

- `internal/application/storyboard_media.go`：最终成片原子落库失败后使用 `DeleteStore` 清理对象；清理错误会转为可重试结构化诊断，`ErrNotFound` 视为已清理。
- `internal/application/media_pipeline_output_test.go`：`TestCleanupFinalRenderBlobSurfacesCleanupFailure` 验证清理失败不会吞掉数据库失败原因。
- 本证据不覆盖 PostgreSQL 事务真实执行、清理失败后的后台重试或跨进程恢复；Blob `Get`/`Put` 的应用层故障注入另见 [27](./27-e1d-b-blob-fault-injection-plan.md)。

## 4.2 E1d-c 证据

- `internal/application/media_pipeline_test.go`：`TestMarketingVideoGoldenJourney` 在最终 `MediaReview(final)` 仍为 `pending` 时调用 `BuildTaskDeliveryPackage`，必须失败；只有审核批准并选中后才允许创建交付包。
- 该断言验证的是既有交付硬门禁，不代表最终审核写入失败时的 Artifact 清理或 PostgreSQL 事务恢复已经完成。

## 5. 回写规则

完成 E1d-a、E1d-b、E1d-c 或 E1e 任一项后，按顺序更新：

1. 本文件的任务清单和证据。
2. `13-video-production-execution.md` 的阶段 E 记录。
3. `17-stage-e-composition-plan.md`、`18-stage-e1-composition-manifest-plan.md` 和 `16-video-production-progress-plan.md`。
4. `12-video-production-migration.md`、`docs/infra/01-capability-map.md`、`docs/infra/03-delivery-roadmap.md` 和 `docs/roadmap/v8/README.md`。
