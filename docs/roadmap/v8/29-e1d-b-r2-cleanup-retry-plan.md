# 视频生产迁移 E1d-b R2：清理重试与运维入口计划

状态：`in_progress`。

更新时间：2026-08-23。

本计划把[ E1d-b 恢复执行计划](./28-e1d-b-recovery-execution-plan.md)中的 R2 拆成可执行工作项，跟踪清理诊断的状态更新、`CreateFinalRender` 自动登记、人工查询/重试入口和跨进程恢复证据。R2-6 及 R3-R5 的独立进程、真实 PostgreSQL/Blob 和同 digest 幂等验收另见[恢复验收计划](./30-e1d-b-recovery-acceptance-plan.md)。它复用 ContentCloud Runtime、Blob、审计和 HTTP 会话边界，不创建 视频生产迁移 独立队列、任务、资产或数据库。

## 1. 当前基线

R1 已建立 `RuntimeCleanupDiagnostic` 的唯一持久化事实落点：Memory/PostgreSQL 均支持创建、单条读取和列表读取，数据库迁移 `00053_runtime_cleanup_diagnostics.sql` 提供租户隔离及 `(tenant_id, object_key)` 唯一约束。

R2-1 审计确认以下能力尚未实现：

- Repository 没有带版本/CAS 的诊断状态更新接口；
- `CreateFinalRender` 清理失败只返回结构化错误，尚未自动写入诊断记录；
- 应用层没有面向人工运维的诊断列表、详情和重试方法；
- HTTP 当前没有清理诊断查询或重试路由；
- 现有 `requireRole` 会让 `worker`/`device` 类型绕过角色判断，人工重试入口不能直接复用这一旁路。

## 2. 进度跟踪

| 编号 | 工作项 | 状态 | 完成门槛 |
| --- | --- | --- | --- |
| R2-1 | 接口、权限和 HTTP 边界审计 | `complete` | 已核对 Runtime Repository、`CreateFinalRender`、`requireRole` 和现有 `/api/v1`/`/api/studio` 路由；缺口已同步到主路线图 |
| R2-2 | 诊断状态 CAS 更新 | `complete` | Memory/PostgreSQL 新增带 `expected_version` 的更新方法；拒绝版本冲突、非法状态转移和跨租户更新 |
| R2-3 | `CreateFinalRender` 自动登记 | `complete` | 数据库事实写入失败且 Blob 删除失败时幂等创建 `pending`/`failed` 诊断；诊断写入失败不能覆盖原始事实错误 |
| R2-4 | 查询与重试应用服务 | `complete` | 仅允许租户管理员/项目经理查询和触发重试；`Delete` 成功与 `ErrNotFound` 分别收敛到 `cleaned`/`not_found`，再次失败进入 `failed` 并安排下一次重试 |
| R2-5 | HTTP 运维入口 | `complete` | 提供租户范围列表、详情和单条重试；响应脱敏，不返回 Token、Cookie、客户文件内容或完整本地路径 |
| R2-6 | 并发与跨进程测试 | `in_progress` | Memory 已覆盖不同 Application 实例的 CAS 竞争、过期 claim 回收、失败退避和跨租户 HTTP；现有 Runtime worker 已接入自动恢复；仍需独立 PostgreSQL Store/进程恢复、租户隔离和终态幂等的真实执行证据 |
| R2-7 | R2 验收与文档收口 | `not_started` | R2-2 至 R2-6 有可复现测试输出，并同步 `28`、`24`、`13`、`16`、`20`、能力地图和交付路线图 |

## 3. 状态与权限约束

清理诊断允许的状态转移为：

```text
pending -> retrying -> cleaned
pending -> retrying -> not_found
pending -> retrying -> failed -> retrying
```

`cleaned` 和 `not_found` 是终态；终态不能设置 `next_retry_at`。每次重试都必须使用当前 `version`，成功更新后递增版本，避免多个 Worker 或人工操作同时删除同一对象。

人工查询和重试的最小角色范围是 `tenant_admin`、`project_manager`，并且必须使用会话中的当前租户过滤。平台管理员沿用现有平台管理授权模式；`worker`/`device` 仅可执行受调度的后台动作，不能凭类型旁路人工运维权限。

## 4. 证据规则

1. Memory 测试只能证明状态机和并发语义，不能替代 PostgreSQL RLS、Blob 真实故障和跨进程执行。
2. 未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 时，真实 PostgreSQL 测试必须明确跳过，不能伪造通过记录。
3. 原始 `CreateFinalRender` 事实错误优先返回；诊断登记失败只记录日志或审计，不覆盖原始错误。
4. 诊断摘要必须脱敏，不保存凭据、Cookie、客户文件内容和完整本地路径。
5. R2 完成前，E1d-b、阶段 E 和 P4 均保持 `in_progress`/未完成。

## 5. R2-1 完成记录

2026-08-23：完成接口、权限和 HTTP 边界审计。证据为 `internal/runtime/repository.go`、`internal/runtime/model.go`、`internal/application/storyboard_media.go`、`internal/application/identity_project.go`、`internal/transport/http/server.go` 及 Memory/PostgreSQL 诊断存储实现。该项只建立后续实现边界和权限约束，不代表状态 CAS、自动登记或后台重试已经可用。

下一项为 R2-3：把清理失败自动登记接入 `CreateFinalRender`，并保持原始事实错误优先返回。

R2-2 完成记录（2026-08-23）：`Runtime.Repository` 新增 `UpdateRuntimeCleanupDiagnostic`；Memory/PostgreSQL 均校验 expected version、版本递增、诊断身份不变和合法状态转移。`pending -> retrying -> cleaned/not_found/failed -> retrying` 之外的转换、终态重试、旧版本写入和跨租户读取/更新均被拒绝。`internal/persistence/memory/runtime_cleanup_test.go` 和 `internal/persistence/postgres/runtime_cleanup_integration_test.go` 已覆盖对应测试入口；定向测试与全量 `go test ./...` 通过。真实 PostgreSQL 测试仍因未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 明确跳过。

R2-3 完成记录（2026-08-23）：`CreateFinalRender` 已在 Artifact/最终审核事实写入失败且临时 Blob 删除失败时自动创建 `pending` 诊断；字段只保存错误码和脱敏摘要，诊断写入失败不会覆盖原始错误。`TestCreateFinalRenderCleanupFailurePersistsSanitizedDiagnostic` 证明记录可读且不泄漏原始删除错误；正常清理和 Blob Put 故障矩阵仍保持无最终事实。

R2-4/R2-5 完成记录（2026-08-23）：应用层新增租户范围的诊断列表、详情和 CAS 重试服务，人工入口拒绝 Worker/Device 类型旁路；重试按 `pending/failed -> retrying` 抢占，并将删除结果收敛为 `cleaned`、`not_found` 或带退避时间的 `failed`。BFF 新增 `/api/bff/runtime/cleanup-diagnostics` 列表、详情和 `/retry` 路由；`TestRuntimeCleanupOperatorRetryConvergesAndProtectsRoles`、`TestRuntimeCleanupOperatorRetryHandlesNotFoundAndFailure` 和 `TestRuntimeCleanupDiagnosticsBFFIsTenantScopedAndRetryable` 通过。跨进程恢复和真实 PostgreSQL/Blob 执行仍待 R2-6/R4。

R2-6 进行中记录（2026-08-23）：`TestRuntimeCleanupOperatorCASWorksAcrossServiceInstances` 使用两个独立 Application 实例证明清理诊断只允许一个删除者；`TestRuntimeCleanupReconcilerRecoversPendingAndExpiredClaimsAcrossApplicationInstances` 证明第二个 Application 可仅依赖持久化诊断回收过期 claim 并完成删除，`TestRuntimeCleanupReconcilerHonorsFailureBackoff` 证明不会提前突破退避；`TestRuntimeCleanupDiagnosticCrossStoreWithPostgres` 验证两个独立 PostgreSQL Store 实例可以交接同一诊断记录。`TestRuntimeCleanupDiagnosticsBFFRejectsCrossTenantReadAndRetry` 进一步证明外租户列表为空、详情和重试均返回 404 且不泄漏诊断字段；应用层 `TestRuntimeCleanupOperatorRetryConvergesAndProtectsRoles` 继续证明 Worker/Device 会话不能进入人工入口。新增 `TestProcessRuntimeEventsReconcilesCleanupDiagnostics`，验证 Runtime event worker 会在同一租户维护循环中扫描诊断、删除 Blob、收敛状态并写入 `runtime_cleanup` 心跳。上述 PostgreSQL 集成测试在未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 时明确跳过，因此专用 PostgreSQL 的真实进程恢复、Blob 组合故障和终态幂等证据仍待环境提供。

本计划不授权 Git 提交、推送、部署、生产数据库操作或生产 API 调用。
