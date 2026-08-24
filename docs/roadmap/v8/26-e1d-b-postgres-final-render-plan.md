# 视频生产迁移 E1d-b：PostgreSQL Final Render 对照计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件跟踪 `CreateFinalRender` 在 PostgreSQL 上的原子写入、回滚和租户隔离证据。它是[E1d-b Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md)的数据库对照拆分；Blob 故障注入矩阵见[27](./27-e1d-b-blob-fault-injection-plan.md)，不创建新的业务事实、队列或数据库。

## 1. 范围

| 项目 | 目标 | 当前状态 |
| --- | --- | --- |
| 成功事务 | `Artifact(kind=final_render)` 与 `MediaReview(final,pending)` 同一事务提交 | 测试已实现，真实库待执行 |
| 回滚事务 | 第二个事实插入失败时不留下孤立 Artifact | 测试已实现，真实库待执行 |
| 作用域校验 | Artifact 与审核租户、项目、主体摘要一致 | 代码已有，集成测试覆盖成功前置 |
| 租户隔离 | 其他租户不可读取最终成片 Artifact | 测试已实现，真实库待执行 |
| Blob 故障 | `Get`、`Put`、清理失败和跨进程恢复 | 应用层 `Get`/`Put` 失败和清理诊断已完成；后台重试、跨进程恢复及 PostgreSQL/Blob 组合幂等仍待补 |

## 2. 测试入口与证据

- [x] 新增 `internal/persistence/postgres/media_integration_test.go` 中的 `TestPostgresFinalRenderAtomicityWithPostgres`。
- [x] 测试使用真实 V3 `ApprovedSnapshot` 前置，不把 Memory、Fixture 或伪造快照当作 PostgreSQL 证据。
- [x] 测试只接受 `CONTENTCLOUD_TEST_DATABASE_URL`，并执行 `store.Migrate`；未设置时明确 `skip`，不会连接开发库或生产库。
- [x] 覆盖成功提交、审核主键冲突导致的事务回滚和跨租户读取隔离。
- [x] `go test ./internal/persistence/postgres`、`go test ./...`、`git diff --check` 已通过。
- [ ] 在专用 PostgreSQL 上运行定向测试并保留完整输出；当前因 `CONTENTCLOUD_TEST_DATABASE_URL` 未设置而跳过。

## 3. 后续门槛

E1d-b 只有在本文件的 PostgreSQL 真实执行与[Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md)中的 `Get`、`Put`、清理失败、幂等和跨进程恢复证据全部存在后，才能标记为 `complete`。测试入口存在或 skip 通过不等于真实数据库验收完成。

下一项：提供专用测试数据库后运行 `go test ./internal/persistence/postgres -run TestPostgresFinalRenderAtomicityWithPostgres -v`，再补 Blob 故障注入和恢复断点。

本计划不授权 Git 提交、推送、部署、生产数据库操作或生产 API 调用。
