# 视频生产迁移 E1d-b：清理诊断与跨进程恢复执行计划

状态：`in_progress`。

更新时间：2026-08-23。

本计划跟踪最终成片临时 Blob 在数据库事实写入失败后的持久化诊断、后台重试、跨进程恢复和 PostgreSQL/Blob 组合幂等。它承接[Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md)、[Blob 故障注入计划](./27-e1d-b-blob-fault-injection-plan.md)和[PostgreSQL Final Render 计划](./26-e1d-b-postgres-final-render-plan.md)，不创建 视频生产迁移 独立队列、任务、资产或交付事实模型。R2 的接口、权限和 HTTP 边界单独跟踪于[R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)，R2-6/R3-R5 的独立进程与真实组合验收见[恢复验收计划](./30-e1d-b-recovery-acceptance-plan.md)。

## 1. 目标与边界

当最终成片对象已经写入 Blob、但 Artifact 与 `MediaReview(final)` 事务没有提交时，系统必须同时满足：

1. 请求方能收到原始事实写入错误；
2. 临时对象会被立即清理，或留下可诊断、可重试的持久化记录；
3. 进程退出后，后台仍能根据记录安全重试清理；
4. 清理重试不会创建 `final_render` Artifact、最终审核或 DeliveryPackage；
5. 同一 Manifest digest 在 PostgreSQL 与 Blob 组合重试时只产生一个逻辑最终结果。

当前已完成的应用层清理和故障注入是前置证据，不代表本计划完成。

## 2. 进度跟踪

| 编号 | 工作项 | 状态 | 完成门槛 |
| --- | --- | --- | --- |
| R0 | 文档基线：统一 `21`、`27`、`24`、`16` 和入口索引的状态、字段和缺口 | `complete` | 文档链接可达；未把当前进程错误误报为后台重试；`DOC-14` 已登记 |
| R1 | 持久化清理诊断契约与唯一事实落点 | `complete` | 已复用 Runtime Repository；字段含租户、项目、任务、请求、Manifest digest、`object_key`、原始原因、清理原因、状态、尝试次数和 `next_retry_at`，并以租户 + object key 保证唯一 |
| R2 | 后台重试/运维入口 | `in_progress` | R2-1/R2-2 审计与 CAS、R2-3 自动登记、R2-4 查询/重试服务、R2-5 HTTP 入口和现有 Runtime worker 的自动恢复入口已完成；R2-6 并发/跨进程真实执行与 R2-7 收口仍待补，详见[专项计划](./29-e1d-b-r2-cleanup-retry-plan.md) |
| R3 | 跨进程恢复测试 | `not_started` | 写入失败后终止原进程，由另一执行者读取记录并完成清理；不依赖内存 Registry 或测试进程全局变量 |
| R4 | PostgreSQL 真实事务与 Blob 故障执行 | `not_started` | 使用专用 `CONTENTCLOUD_TEST_DATABASE_URL`，覆盖提交、回滚、RLS、Blob Put 后报错和清理失败；未配置时只能明确 skip |
| R5 | 同 digest 组合幂等 | `not_started` | PostgreSQL + Blob 重试只保留一组 Artifact/最终审核事实，不重复写入或重复清理 |
| R6 | E1d-b 收口 | `not_started` | R1-R5 有可复现测试输出，并同步 `12`、`13`、`16`、`20`、能力地图和交付路线图 |

## 3. 诊断记录契约

诊断记录必须是脱敏、可跨进程读取的运维事实，至少包含：

| 字段 | 用途 |
| --- | --- |
| `tenant_id` / `project_id` / `task_id` | 权限过滤和业务定位 |
| `request_id` / `manifest_digest` | 请求幂等和输入版本定位 |
| `object_key` | 指向待清理的临时 Blob 对象 |
| `cause_code` / `cause_summary` | 保留原始事实写入失败的可诊断摘要 |
| `cleanup_error` | 最近一次删除失败摘要，可为空 |
| `status` / `attempt_count` / `next_retry_at` | 后台调度和人工重试状态 |
| `created_at` / `updated_at` | 时间审计 |

不得保存令牌、Cookie、客户文件内容、完整本地路径或未脱敏的上游响应。诊断记录不能改变 Artifact、MediaReview 或 DeliveryPackage 的业务状态。

## 4. 执行顺序

1. 完成 R1，先确定唯一持久化落点、状态机和权限边界。
2. 完成 R2，提供后台诊断查询和受权限保护的重试动作。
3. 完成 R3，使用独立进程/Store 实例证明恢复不依赖进程内状态。
4. 完成 R4，使用专用 PostgreSQL 运行真实事务、RLS 和 Blob 故障矩阵。
5. 完成 R5，验证同 digest 的重复请求和清理重试幂等。
6. 通过 R6 后，再把 E1d-b 和阶段 E 的状态更新为实际证据允许的等级。

## 5. 证据和安全规则

- Memory、Fixture、默认确定性 Worker 和“测试未设置数据库 URL 后 skip”只能作为开发证据。
- 真实 PostgreSQL 测试必须使用专用数据库，禁止连接生产库、开发共享库或删除现有数据库。
- 不新增 视频生产迁移 平行队列；后台执行应复用 ContentCloud 现有 Runtime/运维调度和审计边界。
- 每完成一个工作项，先更新本文件，再同步 `12`、`13`、`16`、`20`、`21`、`27`、能力地图和交付路线图。
- 本计划不授权 Git 提交、推送、部署、生产数据库操作或生产 API 调用。

## 6. 当前结论

R0 已完成：文档已经明确 Blob `Get`/`Put` 失败的事实边界、清理失败诊断的最小字段和跨进程恢复缺口。

R1 已完成（2026-08-23）：Runtime 新增 `RuntimeCleanupDiagnostic` 契约、状态校验、Memory/PostgreSQL 存储实现和租户 + `object_key` 唯一约束；证据为 `internal/persistence/memory/runtime_cleanup.go`、`internal/persistence/memory/runtime_cleanup_test.go`、`internal/persistence/postgres/runtime_cleanup.go` 和迁移 `00053_runtime_cleanup_diagnostics.sql`。该项只建立可跨进程读取的持久化事实，不代表后台重试已经接通。R2-R6 尚未完成，E1d-b 与阶段 E 继续保持 `in_progress`。

R1 验证记录（2026-08-23）：定向执行 `go test ./internal/persistence/memory ./internal/persistence/postgres ./internal/runtime` 和全量 `go test ./...` 均通过；同时通过 `gofmt -d`、`git diff --check` 及 Markdown 相对链接检查。PostgreSQL 真实数据库测试仍以 `CONTENTCLOUD_TEST_DATABASE_URL` 为前置，本轮环境未配置，因此未执行真实数据库连接。后续 R2 必须先把 `CreateFinalRender` 清理失败接入该记录，再增加权限保护的查询/重试入口。

R2-1 文档审计记录（2026-08-23）：确认 Runtime Repository 当前只有创建、单条读取和列表读取接口，没有诊断状态更新/CAS；`CreateFinalRender` 清理失败仍只返回结构化错误，尚未自动登记 `RuntimeCleanupDiagnostic`；现有 `requireRole` 可作为人工入口基线，但查询和重试必须显式限制 `tenant_admin`/`project_manager`，不能因 Worker/Device 类型绕过人工权限。具体拆分和完成门槛见[R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)。

R2-2 完成记录（2026-08-23）：Memory/PostgreSQL 已实现诊断状态 CAS 和合法状态机，覆盖版本冲突、身份不变、终态不可重试及租户边界；定向测试通过。

R2-3/R2-5 完成记录（2026-08-23）：`CreateFinalRender` 清理失败已自动登记脱敏诊断；应用层和 BFF 已提供租户范围查询、详情和受角色保护的 CAS 重试，删除成功、对象不存在和失败退避均有测试证据。R2-6 的 Memory 并发测试通过，独立 PostgreSQL Store 测试入口已建立但因未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 跳过；R2-6/R2-7、R3-R5 和真实 PostgreSQL/Blob 组合执行仍待完成。

R2-6b 增量（2026-08-23）：现有 `contentcloud-worker` 的 Runtime event loop 会按租户消费 `RuntimeCleanupDiagnostic`，复用 CAS 保护的删除逻辑；`retrying` claim 超过 5 分钟会先回收为可重试的 `failed`，进程退出后不依赖内存状态。新增 `runtime_cleanup` 维护心跳和 `RuntimeCleanupReconciliationResult` 计数，Memory 测试覆盖两个 Application 实例交接、过期 claim 回收和失败退避。专用 PostgreSQL 进程恢复仍需真实环境执行。
