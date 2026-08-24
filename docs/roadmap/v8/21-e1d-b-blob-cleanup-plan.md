# 视频生产迁移 E1d-b：最终成片临时 Blob 清理计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件跟踪最终成片写入失败后的临时 Blob 处理。它是[总完成计划](./20-video-production-completion-plan.md)中 P4 的专项拆分，不改变 ContentCloud 的事实模型，也不引入 视频生产迁移 独立存储或队列。

清理失败后的持久化诊断、运维重试和跨进程恢复由[ E1d-b 恢复执行计划](./28-e1d-b-recovery-execution-plan.md)单独跟踪。本文件保留清理动作本身的事实边界。

## 1. 本轮完成项

- [x] `CreateFinalRender` 在 Artifact + `MediaReview(final)` 原子事务失败后尝试删除已写入的临时对象。
- [x] 删除返回 `ErrNotFound` 时视为对象已经清理，保留原始数据库错误。
- [x] 删除失败时返回可重试、带 `object_key`、清理错误和原始原因的结构化诊断。
- [x] 增加清理失败回归测试，证明只尝试一次清理且不会丢失数据库失败原因。

证据：`internal/application/storyboard_media.go`、`internal/application/media_pipeline_output_test.go`；定向 `go test ./internal/application ./internal/persistence/memory ./internal/delivery` 已通过。

## 2. 尚未完成

- [x] `Blob Get` 失败不产生数据库事实的故障注入测试；证据见[Blob 故障注入与事实原子性计划](./27-e1d-b-blob-fault-injection-plan.md)和 `internal/application/media_pipeline_blob_failure_test.go`。
- [x] `Blob Put` 失败不产生数据库事实的故障注入测试；同时覆盖写入前失败和写入后返回错误两种情况，并确认临时对象只尝试清理一次。
- [ ] PostgreSQL 事务回滚和租户隔离集成测试；未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 时只能明确跳过。
- [ ] 清理失败后的后台诊断/重试入口，以及跨进程恢复证据。
- [ ] 同一 Manifest digest 在 PostgreSQL/Blob 组合下的幂等重试证据。

清理失败诊断的最小字段契约已确定为：`tenant_id`、`project_id`、`task_id`、`request_id`、`manifest_digest`、`object_key`、原始数据库/事实写入原因、清理错误、诊断状态、重试次数和 `next_retry_at`。字段应保存脱敏错误摘要，不得保存令牌、客户文件内容或完整本地路径；在持久化实现和跨进程测试完成前，不能称为后台重试已可用。

本轮 R1 已将该契约落到现有 Runtime Repository：`RuntimeCleanupDiagnostic` 由迁移 `00053_runtime_cleanup_diagnostics.sql` 持久化，按租户隔离并以租户 + `object_key` 唯一。Memory 契约测试和迁移静态检查已通过；CreateFinalRender 尚未写入该记录，后台重试、跨进程恢复和真实 PostgreSQL 执行仍待 R2-R4。

PostgreSQL 数据库对照另见[E1d-b PostgreSQL Final Render 计划](./26-e1d-b-postgres-final-render-plan.md)。本轮新增的 PostgreSQL 原子事务测试入口：`internal/persistence/postgres/media_integration_test.go` 的
`TestPostgresFinalRenderAtomicityWithPostgres`。它使用真实 V3 `ApprovedSnapshot`，验证 Artifact 与
`MediaReview(final)` 同事务提交、审核插入冲突时 Artifact 回滚，以及租户隔离；仅接受
`CONTENTCLOUD_TEST_DATABASE_URL`。当前该环境变量未设置，定向测试明确 skip，因此不能视为真实 PostgreSQL
执行通过。

本轮已完成 Blob `Get` 失败故障注入：前置 fixture 可写入并生成候选 Artifact，最终渲染读取失败后不产生
`final_render` Artifact 或 `MediaReview(final)`。这只关闭读取失败项，不改变 E1d-b 或阶段 E 的整体状态。

本轮已完成 Blob `Put` 失败故障注入：`CreateFinalRender` 在对象写入前失败或对象已写入但返回错误时，均不创建
`final_render` Artifact 或 `MediaReview(final)`；已写入对象会进入统一清理路径。证据为
`TestCreateFinalRenderBlobPutFailureLeavesNoFinalFactsAndCleansObject`。

## 3. 完成门槛

E1d-b 只有在本文件 §1 与 §2 的存储故障、事务回滚、幂等和恢复证据全部存在后才能标记为 `complete`。本轮完成清理边界和 Blob `Get` 失败无事实测试，不代表阶段 E、真实合成 Worker 或 PostgreSQL 对照已经完成。
