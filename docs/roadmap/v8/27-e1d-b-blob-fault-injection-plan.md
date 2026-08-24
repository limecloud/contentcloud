# 视频生产迁移 E1d-b：Blob 故障注入与事实原子性计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件跟踪 `CreateFinalRender` 在 Blob 读写异常时的事实边界。它补充[临时 Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md)和[PostgreSQL Final Render 对照计划](./26-e1d-b-postgres-final-render-plan.md)，不创建新的业务事实、队列或存储模型。

清理失败后的持久化诊断、运维重试和跨进程恢复由[ E1d-b 恢复执行计划](./28-e1d-b-recovery-execution-plan.md)跟踪。本文件只定义故障边界，不把当前进程返回的结构化错误视为已经具备后台重试能力。

## 1. 验证矩阵

| 场景 | 状态 | 证据 | 预期边界 |
| --- | --- | --- | --- |
| 候选视频 Blob `Get` 失败 | `complete` | `internal/application/media_pipeline_blob_failure_test.go`：`TestCreateFinalRenderBlobGetFailureLeavesNoFinalFacts` | 返回原始读取错误，不创建 `final_render` Artifact 或 `MediaReview(final)` |
| 最终成片 Blob `Put` 失败 | `complete` | `internal/application/media_pipeline_blob_failure_test.go`：`TestCreateFinalRenderBlobPutFailureLeavesNoFinalFactsAndCleansObject` | 写入前或写入后报错均不创建数据库事实，并尝试清理临时对象 |
| Artifact/最终审核事务失败后的清理 | `complete` | `internal/application/media_pipeline_output_test.go`；清理失败诊断测试 | 原始数据库错误保留；清理失败返回可重试结构化诊断 |
| PostgreSQL 事务回滚与租户隔离 | `implemented_pending_execution` | `TestPostgresFinalRenderAtomicityWithPostgres` | 专用数据库成功/回滚/隔离输出；未配置 URL 时只能 skip |
| 同 Manifest digest 重试 | `not_started` | - | 只复用一组 Artifact/最终审核事实 |
| 跨进程故障恢复 | `not_started` | - | 可根据诊断中的 object key 重试或清理，不依赖进程内状态 |

## 2. 本项完成证据

`getFailureBlobStore` 允许前置业务流程写入对象，但让候选成片在最终渲染读取时返回固定错误。测试通过开发视频 fixture 构造完整的批准快照、候选 Artifact 和选中审核，然后确认：

- `CreateFinalRender` 返回 Blob 读取错误；
- 任务下不存在 `kind=final_render` 的 Artifact；
- 任务下不存在 `ReviewKind=final` 的媒体审核；
- 失败不被转换成默认 Worker 成功或可交付事实。

`finalPutFailureBlobStore` 分别模拟 Blob 在写入前失败和“已写入但返回错误”两种现实故障。测试确认两种路径都返回原始写入错误、只尝试一次删除临时对象，且任务下没有 `final_render` Artifact 或 `MediaReview(final)`。

定向命令：

```text
go test ./internal/application -run 'TestCreateFinalRenderBlob(GetFailureLeavesNoFinalFacts|PutFailureLeavesNoFinalFactsAndCleansObject)$' -v
```

## 3. 下一项与完成门槛

下一项是清理失败后的持久化诊断/重试入口、跨进程恢复，以及同 Manifest digest 的 PostgreSQL/Blob 幂等重试。诊断至少需要保留租户、项目、任务、请求、Manifest digest、`object_key`、原始失败原因、清理失败原因、当前状态和下一次重试时间；具体落点必须复用 ContentCloud 现有运行时/运维事实，不能新增 视频生产迁移 平行队列。E1d-b 只有在本文件、`21`、`26` 和 `28` 的读写失败、清理、事务、幂等及恢复证据全部具备后才能标记为 `complete`。

本轮 R1 增量：已在 Runtime 事实边界中建立 `RuntimeCleanupDiagnostic` 的 Memory/PostgreSQL 存储和租户 + `object_key` 唯一约束。该记录尚未由清理路径自动写入，也尚未提供后台重试或跨进程恢复，因此本计划的跨进程与组合幂等项仍为 `not_started`。

本计划不授权 Git 提交、推送、部署、生产数据库操作或生产 API 调用。
