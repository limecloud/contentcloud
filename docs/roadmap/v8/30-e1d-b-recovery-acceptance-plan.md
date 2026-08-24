# 视频生产迁移 E1d-b 恢复验收计划

状态：`in_progress`。

更新时间：2026-08-23。

本计划跟踪 E1d-b 清理诊断从应用层实现到独立进程、真实 PostgreSQL/Blob 和同 digest 幂等验收的剩余工作。它承接 [R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md) 和 [恢复执行计划](./28-e1d-b-recovery-execution-plan.md)，只复用 ContentCloud Runtime、Blob、审计和现有媒体事实，不创建 视频生产迁移 专属队列、数据库、Task、Asset 或发布模型。

## 1. 当前基线

已具备的代码和测试入口：

- `RuntimeCleanupDiagnostic` 已有 Memory/PostgreSQL 持久化契约、租户隔离、`object_key` 唯一约束和版本 CAS。
- `CreateFinalRender` 在事实写入失败且临时 Blob 删除失败时自动登记脱敏诊断；诊断写入失败不会覆盖原始事实错误。
- Operations Service 提供租户范围列表、详情和重试；只允许 `tenant_admin`、`project_manager`，平台管理员沿用平台授权，Worker/Device 不能旁路人工入口。
- BFF 提供 `/api/bff/runtime/cleanup-diagnostics` 列表、详情和 `/{diagnosticID}/retry`。
- 运营后台 `/admin/cleanup` 已接入上述 BFF：支持状态筛选、脱敏原因/对象摘要、尝试次数与退避时间展示，以及 CAS 保护的人工重试；页面明确标注该入口只处理临时对象，不创建任务、审批或产物事实。
- Memory 并发测试和 PostgreSQL 独立 Store 集成测试入口已建立；未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 时 PostgreSQL 测试必须明确跳过。

## 2. 工作项

| 编号 | 状态 | 工作内容 | 完成门槛 |
| --- | --- | --- | --- |
| R2-6a | `complete` | Memory CAS 并发竞争 | 多个重试者最多一个进入 `retrying` 并执行删除；终态不可再次重试 |
| R2-6b | `in_progress` | 独立 Store/进程交接 | Runtime worker 已能只依赖持久化诊断读取并完成清理，并能回收超过 5 分钟的 `retrying` claim；仍需专用 PostgreSQL 进程级执行证据 |
| R2-6c | `complete` | 跨租户和 HTTP 负向验收 | 外租户详情、列表、重试都不能读取或改变其他租户记录；真实 Device Token 访问人工清理 BFF 的列表、详情和重试均返回 401，响应脱敏且诊断事实保持不变 |
| R2-6d | `not_started` | 专用 PostgreSQL 执行 | 设置 `CONTENTCLOUD_TEST_DATABASE_URL` 后运行迁移、RLS、CAS、终态幂等和恢复矩阵；禁止连接生产库 |
| R3 | `not_started` | 写入失败后的跨进程恢复 | 原进程退出后另一执行者完成 `cleaned`/`not_found`/`failed` 收敛，并保留尝试次数和退避时间 |
| R4 | `not_started` | PostgreSQL + Blob 组合故障 | 覆盖 Blob Put 后数据库回滚、清理失败、重试成功和租户隔离；不产生最终 Artifact/MediaReview |
| R5 | `not_started` | 同 Manifest digest 幂等 | 重复请求或恢复只保留一组最终事实；清理诊断不会重复登记同一临时对象 |

## 3. 执行顺序

1. 先补独立进程/Store 的可执行测试，不引入全局内存状态。
2. 在专用 PostgreSQL 上运行迁移和集成矩阵；环境缺失时记录 `skip`，不得写成通过。
3. 补 Blob 故障注入与数据库事实回滚的组合场景。
4. 验证同 digest 重试、终态幂等和跨租户负向路径。
5. 将证据回写 `28`、`29`、`24`，再同步 `12`、`13`、`16`、`20`、能力地图和交付路线图。

## 4. 证据规则

- Memory、Fixture 和单进程测试只能证明状态机与应用语义，不能替代真实 PostgreSQL、Blob 或进程恢复。
- PostgreSQL 集成测试只能使用专用 `CONTENTCLOUD_TEST_DATABASE_URL`；未配置时必须显式跳过。
- 测试不得删除、重命名或批量更新现有数据库，也不得调用生产 API。
- E1d-b、阶段 E 和 P4 在 R2-6b 至 R5 全部有可复现证据前保持 `in_progress`。

本计划不授权 Git 提交、推送、部署、生产数据库操作或生产 API 调用。

2026-08-23 增量：现有 Runtime worker 已接入 `runtime_cleanup` 维护心跳和自动恢复；`TestRuntimeCleanupReconcilerRecoversPendingAndExpiredClaimsAcrossApplicationInstances` 覆盖跨 Application 实例的持久化交接，`TestRuntimeCleanupReconcilerHonorsFailureBackoff` 覆盖退避。`TestRuntimeCleanupDiagnosticsBFFRejectsCrossTenantReadAndRetry` 已覆盖外租户列表为空、详情/重试 404 和响应脱敏；`TestRuntimeCleanupDiagnosticsBFFRejectsDeviceToken` 使用真实 Device Token 覆盖列表、详情、重试全部 401，并验证诊断状态、版本和尝试次数不变；`TestRuntimeCleanupOperatorRetryConvergesAndProtectsRoles` 继续覆盖 Worker 与 Device 应用层类型拒绝。R2-6c 完成，R2-6b 仍因专用 PostgreSQL 进程执行保持 `in_progress`。

2026-08-23 增量：新增 `TestProcessRuntimeEventsReconcilesCleanupDiagnostics`，验证 Runtime event worker 在维护循环中读取持久化清理诊断、删除 Blob、将状态收敛到终态并写入 `runtime_cleanup` 心跳。该测试仍基于 Memory/单进程装配，只证明 worker 与应用服务的集成语义；专用 PostgreSQL 进程执行、真实 Blob 组合故障和同 digest 幂等仍未完成，R2-6b 与 R2-6d 至 R5 继续保持未完成状态。

2026-08-23 增量：新增运营后台清理诊断页面及路由回归（`apps/web/src/admin/views/AdminCleanupPage.tsx`、`cleanupPage.test.tsx`），补齐底层 Runtime 故障事实的人工观察入口；Web 测试、类型检查和架构门禁通过。该页面不改变 R2-6b、R2-6d 至 R5 的外部验收状态。
