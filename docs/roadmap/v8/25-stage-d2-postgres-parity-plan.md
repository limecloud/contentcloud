# 视频生产迁移 阶段 D2 PostgreSQL 对照计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件跟踪阶段 D2 的 Memory/PostgreSQL 对照工作。它只验证 ContentCloud 现有 Provider、媒体任务、调用尝试、Runtime Effect 和租户隔离事实，不创建 视频生产迁移 专属数据库、队列或 Provider 状态模型。

## 1. 目标与范围

D2 的目标是证明阶段 D 的恢复和费用语义在 Memory 与 PostgreSQL 存储之间保持一致。对照范围包括：

- `ProviderProfile` 与 `ProviderBinding` 的保存、读取、租户边界；
- `MediaGenerationJob` 的创建、读取、状态版本冲突和幂等键；
- `ProviderAttempt` 的外部任务 ID、`NextPollAt`、`LastPolledAt`、unknown 错误摘要和实际费用；
- Job 与 Attempt 的终态实际费用一致性；
- unknown 恢复到 `running`、`failed`、`cancelled` 时不新增 Attempt、不重复 Submit；
- `CreateFinalRender` 事务的原子写入、摘要校验和租户隔离（与阶段 E1d-b 共享 PostgreSQL 证据）。

## 2. 前置条件

- 集成测试必须通过 `CONTENTCLOUD_TEST_DATABASE_URL` 显式指定专用测试数据库；
- 未设置该变量时，测试只能输出明确的 `skip`，不能连接开发库或生产库；
- 不执行数据库删除、重命名、批量更新或生产迁移；
- 测试数据使用独立租户和项目，测试结束后由测试事务或专用数据库清理；
- Memory 测试只能作为对照基线，不能替代 PostgreSQL、RLS 或事务证据。

## 3. 对照矩阵

| 编号 | 场景 | Memory 证据 | PostgreSQL 证据 | 完成条件 |
| --- | --- | --- | --- | --- |
| D2-1 | Provider Profile/Binding 保存与读取 | 已有单元测试或补充定向测试 | 集成测试通过且字段一致 | 同租户可读，跨租户不可读 |
| D2-2 | Job 创建、读取、幂等和版本冲突 | 已有媒体管线测试 | 集成测试入口已覆盖 CAS/版本冲突；真实库执行待 D2-3 | 重试不产生重复业务事实 |
| D2-3 | Attempt 恢复字段 | unknown 状态矩阵已覆盖 | 集成测试核对外部 ID、轮询时间和错误摘要 | 恢复字段不丢失 |
| D2-4 | 终态费用 | Job/Attempt 一致性测试 | 集成测试核对事务提交后的金额 | 两处金额和币种一致，负数被拒绝 |
| D2-5 | unknown 恢复 | 不重复 Submit/Attempt 测试 | 持久化后重新读取并恢复 | `running`/`failed`/`cancelled` 均可收敛 |
| D2-6 | RLS 与租户隔离 | Memory tenant guard | PostgreSQL RLS 正向/负向测试 | 越权读取和写入均失败 |
| D2-7 | Final Render 原子性 | Memory 原子写入测试 | PostgreSQL 事务回滚测试 | Artifact 与最终审核要么同时存在，要么都不存在 |

## 3.1 D2-1 接口与测试盘点

D2-1 已完成。本次盘点只确认现有边界和可复用测试入口，不把 Memory 证据当作 PostgreSQL 证据：

| 事实 | 现有实现 | 可复用证据 | PostgreSQL 缺口 |
| --- | --- | --- | --- |
| Provider Profile | `DeliveryRepository` 提供 `CreateProviderProfile`、`ProviderProfile`；Memory/PostgreSQL 具体 Store 另有 profile 列表读取；PostgreSQL 存储在 `provider_profiles`，不带租户列；Binding 通过 `SaveProviderBinding` upsert | `internal/application/provider_management_test.go` | 需补 profile/binding 持久化读取和跨租户 binding 隔离测试 |
| Provider Binding | Memory 与 PostgreSQL 均按 `tenant_id + provider_id` 读取；PostgreSQL `SaveProviderBinding` 使用 `withTenant` | `internal/application/provider_management_test.go` | 需验证 RLS 下跨租户读写不可见，并核对敏感凭据只保存 `CredentialRef` |
| MediaGenerationJob | 创建、批量创建、按任务读取、按 ID 读取、CAS 保存均已在 `DeliveryRepository` 暴露；PostgreSQL `SaveMediaGenerationJob` 锁行并检查 `row_version` 和状态转换 | `internal/application/media_pipeline_batch_test.go`、`media_pipeline_test.go` | 需验证重启后幂等/版本冲突、外键事实和租户隔离 |
| ProviderAttempt | 创建、保存、按 Job/Runtime Job 读取；PostgreSQL 扫描并保存 `ExternalJobID`、`LastPolledAt`、`NextPollAt`、错误摘要、实际费用 | `internal/application/media_pipeline_state_test.go`、`media_pipeline_cancel_test.go` | 需验证未知提交和终态字段跨进程持久化，且不新增 Attempt |
| Final Render | `CreateFinalRender` 依赖 `AtomicFinalRenderWriter`；Memory 已有原子写入测试 | `internal/persistence/memory/final_render_test.go`；E1d-b PostgreSQL 对照计划见 [26](./26-e1d-b-postgres-final-render-plan.md) | 需在专用 PostgreSQL 执行 Artifact + MediaReview 同事务成功/回滚，并补 Blob 清理证据 |

媒体管线专用 PostgreSQL 集成测试入口已新增至 `internal/persistence/postgres/media_integration_test.go`。D2-2 复用 `CONTENTCLOUD_TEST_DATABASE_URL` skip 规则、`storepg.New`、`store.Migrate` 和真实租户/项目创建方式；未设置专用测试库时只验证测试可编译并明确跳过，不能将 skip 当作真实 PostgreSQL 证据。

## 3.2 D2-2 集成测试实现证据

D2-2 的测试实现已完成，但真实数据库执行仍未完成：

- [x] Provider Profile/Binding 保存、读取及绑定凭据引用回读；
- [x] `MediaGenerationJob` 创建、幂等键冲突、读取和 CAS 版本冲突；
- [x] `ProviderAttempt` 外部任务 ID、轮询时间、unknown 错误摘要和实际费用持久化；
- [x] unknown 恢复到 `running` 不创建新的 Attempt；
- [x] Job/Attempt 终态实际费用一致性和 Provider usage 汇总；
- [x] PostgreSQL RLS 下的跨租户读取和写入隔离负向断言。

代码证据为 `internal/persistence/postgres/media_integration_test.go`，测试入口使用专用环境变量保护，不会连接开发库或生产库。当前执行结果：`go test ./internal/persistence/postgres` 通过；`go test ./internal/persistence/postgres -run TestMediaPipelinePersistenceWithPostgres -v` 因未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 明确跳过。D2-3 仍需在专用 PostgreSQL 上真实执行并保留输出。

## 4. 执行顺序

- [x] D2-0：建立本专项计划、证据规则和入口链接。
- [x] D2-1：盘点现有 Memory/PostgreSQL 存储接口与测试辅助函数，避免重复造测试夹具；盘点结果见上节。
- [x] D2-2：新增 PostgreSQL 集成测试并覆盖媒体任务、Attempt、费用和 RLS；无测试数据库时验证明确跳过。
- [ ] D2-3：运行定向测试和 `go test ./...`，记录真实执行环境与结果。
- [ ] D2-4：若真实 PostgreSQL 未提供，保持阶段 D2 `in_progress`，同步所有主路线图的缺口说明。

## 5. 证据规则

1. `FakeProvider`、Memory Store、Fixture 或静态代码检查不能单独完成 D2。
2. PostgreSQL 集成测试必须验证 RLS、事务回滚和跨进程可读取的持久化字段。
3. 未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 时，不得把“测试已跳过”写成“测试通过”。
4. D2 完成后，先更新本计划，再同步 `12`、`13`、`16`、`20`、能力地图和交付路线图，并运行 `git diff --check`。

本专项计划不授权 Git 提交、推送、部署、生产数据库操作或生产 API 调用。
