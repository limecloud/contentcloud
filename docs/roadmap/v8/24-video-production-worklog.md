# 视频生产迁移工作台账

状态：`in_progress`。

更新时间：2026-08-23。

本文件是 视频生产迁移的短周期工作台账，用于记录当前工作项、证据和文档同步结果。它不替代[迁移计划](./12-video-production-migration.md)、[执行跟踪](./13-video-production-execution.md)或[完成计划](./20-video-production-completion-plan.md)，D2 的专项范围见[阶段 D2 PostgreSQL 对照计划](./25-stage-d2-postgres-parity-plan.md)，E1d-b PostgreSQL 范围见[Final Render 对照计划](./26-e1d-b-postgres-final-render-plan.md)，清理失败恢复范围见[ E1d-b 恢复执行计划](./28-e1d-b-recovery-execution-plan.md)，R2 接口、权限和重试入口见[R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)，独立进程和真实组合验收见[恢复验收计划](./30-e1d-b-recovery-acceptance-plan.md)，也不创建新的业务事实模型。

## 1. 当前基线

| 阶段 | 状态 | 当前证据 | 下一项 |
| --- | --- | --- | --- |
| A-C 边界、视觉一致性、批量准入 | `complete` | 领域映射、锁定摘要、批量原子准入和门禁测试 | 保持唯一主链，不新增平行模型 |
| D Provider 恢复与费用 | `in_progress` | Memory 模拟恢复矩阵覆盖 unknown -> running/failed/cancelled，终态费用一致回写 | D2：Memory/PostgreSQL 对照；随后真实 Provider 和账单回执 |
| E 后期合成与交付门禁 | `in_progress` | Manifest 校验、digest 幂等、Memory 原子写入、最终审核门禁、数据库失败后的 Blob 清理诊断、Blob `Get`/`Put` 失败无事实测试、清理失败自动登记、租户范围查询/详情/CAS 重试 BFF | 独立进程恢复、PostgreSQL 真实执行、同 digest 组合幂等和真实 Worker |
| F 剪映导出 | `in_progress` | 确定性 ZIP、应用/HTTP/CLI 交付编排入口、输入/血缘校验、失败矩阵、契约测试和 `LintJianyingArchive` | F5：真实已批准 Artifact 的人工导入验收 |
| G 发布与效果回流 | `not_started` | 现有 Channel/Performance 事实模型和模拟测试 | 真实 Provider -> 交付 -> 发布回执 -> 效果导入 |

## 2. 本轮工作项

- [x] W-1：同步 视频生产迁移 相关入口文档。已在 `docs/roadmap/v8/README.md` 和 `docs/infra/README.md` 注册 12-24 计划；同步 `docs/infra/01-capability-map.md`、`docs/infra/03-delivery-roadmap.md` 的更新时间。该项只更新导航和元数据，不改变阶段状态。
- [x] W-2a：完成 D2-1 接口与测试盘点。已核对 `DeliveryRepository`、Memory/PostgreSQL 媒体存储实现及现有 Memory 测试入口；盘点结果已用于 D2-2 集成测试实现。
- [x] W-2：完成 D2-2 Memory/PostgreSQL 对照测试实现。新增 `internal/persistence/postgres/media_integration_test.go`，覆盖 Provider Profile/Binding、`MediaGenerationJob`、`ProviderAttempt`、版本冲突、租户隔离、unknown 恢复字段、终态实际费用和 usage 汇总；未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 时测试明确跳过，真实 PostgreSQL 执行仍待 W-2b。
- [ ] W-2b：在专用 PostgreSQL 上执行 D2-2 集成测试，保存真实通过输出；若环境仍缺失，记录环境缺口并保持 D2 `in_progress`。
- [ ] W-3：补 E1d-b PostgreSQL/Blob 故障恢复证据。应用层已覆盖 Blob `Get`、`Put` 和数据库失败后的清理；仍需清理失败后台入口、跨进程恢复、专用 PostgreSQL 和组合幂等，不能用 Memory 测试替代真实环境。
- [x] W-3b：完成 Blob `Get` 失败无事实故障注入。`TestCreateFinalRenderBlobGetFailureLeavesNoFinalFacts` 证明读取失败不创建 `final_render` Artifact 或 `MediaReview(final)`；详见[27](./27-e1d-b-blob-fault-injection-plan.md)。
- [x] W-3a：新增 E1d-b PostgreSQL 原子事务测试入口。`TestPostgresFinalRenderAtomicityWithPostgres` 覆盖真实 ApprovedSnapshot 前置、Artifact + `MediaReview(final)` 同事务成功、审核插入冲突回滚和租户隔离；未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 时明确跳过，真实库执行仍并入 W-3。
- [ ] W-4：完成 F5 真实剪映导入。必须使用真实已批准 Artifact 生成归档，在本地剪映中人工确认可导入，并保留 lint 与导入结果记录。
- [ ] W-5：完成 G 真实闭环。需要真实 Provider、交付包、渠道发布或人工回执、`ChannelPublication`/`Receipt` 和 `PerformanceObservation`，并核对 ApprovedSnapshot 血缘。
- [x] W-3d：按[ E1d-b 恢复执行计划](./28-e1d-b-recovery-execution-plan.md)完成 R1 持久化清理诊断契约和唯一事实落点；新增 `RuntimeCleanupDiagnostic`、Memory/PostgreSQL 存储及 `00053_runtime_cleanup_diagnostics.sql`，后台重试和跨进程恢复仍待后续工作项。
- [x] W-3e：接入 `CreateFinalRender` 清理失败诊断写入，并按[R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)提供租户范围查询、详情和 CAS 重试入口；跨进程恢复另由[恢复验收计划](./30-e1d-b-recovery-acceptance-plan.md)跟踪。
- [x] W-3e-2：完成 R2-2 诊断状态 CAS；`Runtime.Repository`、Memory/PostgreSQL 实现和非法状态/旧版本/终态重试测试已完成。
- [x] W-3e-3：完成 R2-3/R2-5 自动登记、查询/详情/CAS 重试 BFF 的文档收口；R2-6/R3-R5 仍待独立进程和真实 PostgreSQL/Blob 验收。
- [x] W-3e-4：补齐清理诊断跨租户和 Device Token HTTP 负向验收；外租户列表为空，详情和重试返回 404 且不泄漏诊断字段；真实 Device Token 访问人工清理 BFF 的列表、详情和重试均返回 401 且不能改变诊断事实。真实 PostgreSQL/Blob 仍待 R2-6d。
- [x] W-3e-5：接入 Runtime event worker 的清理诊断维护循环；`TestProcessRuntimeEventsReconcilesCleanupDiagnostics` 验证 Blob 删除、诊断终态收敛和 `runtime_cleanup` 心跳。该项仅完成 Memory/单进程集成证据，专用 PostgreSQL 进程、真实 Blob 组合故障和同 digest 幂等仍待 R2-6d/R3-R5。

## 3. 证据规则

1. 模拟 Provider、Fixture、默认确定性 Worker 和页面存在只能作为开发证据。
2. 没有 PostgreSQL、真实 Provider、真实 Worker、真实剪映或真实渠道证据时，相关阶段保持 `in_progress` 或 `not_started`。
3. 每个工作项完成后，先更新对应专项计划，再同步 `13`、`16`、`20`、能力地图和交付路线图。
4. 每次同步运行 `git diff --check`；测试或外部环境未执行时，必须明确记录跳过原因。

## 4. 当前下一步

下一项优先处理 W-2b（D2-3）和[恢复验收计划](./30-e1d-b-recovery-acceptance-plan.md)中的 R2-6b/R2-6d；在真实 PostgreSQL 环境未提供前，不将 D2、E1d 或阶段 D/E 标记为完成。

本轮完成（2026-08-23）：W-2 完成 D2-2 测试实现；完整 `go test ./...` 与 `git diff --check` 通过，媒体专用 PostgreSQL 测试因未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 明确跳过。

本轮完成（2026-08-23）：W-3a 完成 E1d-b PostgreSQL 原子事务测试入口；完整 `go test ./...` 与 `git diff --check` 通过，真实 PostgreSQL 原子事务测试因未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 明确跳过。

本轮完成（2026-08-23）：W-3b 完成 Blob `Get` 失败故障注入；定向应用测试通过，失败后无 `final_render` Artifact 和最终媒体审核。

本轮完成（2026-08-23）：W-3c 完成 Blob `Put` 失败故障注入；定向应用测试覆盖写入前和写入后报错，失败后无最终事实，临时对象清理通过。

本轮完成（2026-08-23）：W-6 完成 E1d-b 恢复文档基线；同步 `21`、`27` 的诊断字段和未完成边界，新增[ E1d-b 恢复执行计划](./28-e1d-b-recovery-execution-plan.md)，并注册到迁移入口。持久化诊断、后台重试、跨进程恢复和真实 PostgreSQL 仍未完成。

本轮完成（2026-08-23）：W-7 完成恢复计划的完成记录同步；`20` 已明确引用 `28` 的 R0-R6 边界，避免历史记录把临时清理错误描述为后台恢复能力。该项仅更新文档，不改变阶段状态。

本轮完成（2026-08-23）：W-3d 完成 R1 持久化清理诊断契约；`RuntimeCleanupDiagnostic` 复用 Runtime Repository，Memory/PostgreSQL 实现和迁移静态检查通过，定向 `go test ./internal/persistence/memory ./internal/persistence/postgres ./internal/runtime` 及全量 `go test ./...` 均通过。`gofmt -d`、`git diff --check` 和 Markdown 相对链接检查也通过。该项只建立跨进程可读取的诊断事实，不代表清理路径自动写入或后台重试可用；下一项 W-3e 仍需接入 `CreateFinalRender` 和权限保护的后台重试入口。

本轮完成（2026-08-23）：W-3e-1 完成 R2-1 接口、权限和 HTTP 边界审计；确认状态 CAS、清理路径自动登记、查询/重试应用服务和路由均未实现，已新增[ R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)跟踪后续实现。该项只更新计划和证据边界，不代表后台重试已经可用。

本轮完成（2026-08-23）：W-3e-2 完成 R2-2 诊断状态 CAS；`internal/persistence/memory/runtime_cleanup_test.go` 和 `internal/persistence/postgres/runtime_cleanup_integration_test.go` 已建立 Memory/专用 PostgreSQL 测试入口，定向与全量 `go test ./...` 通过，真实 PostgreSQL 因未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 明确跳过。该项只完成状态持久化更新边界，不代表清理失败已自动登记或后台重试已接通。

本轮完成（2026-08-23）：W-3e-3 完成 R2-3/R2-5 文档收口；已同步 `12`、`13`、`16`、`20`、能力地图和交付路线图，新增 [恢复验收计划](./30-e1d-b-recovery-acceptance-plan.md) 跟踪 R2-6/R3-R5。应用层和独立 Store 测试入口已建立，真实 PostgreSQL/Blob 和跨进程执行仍待环境提供。

本轮完成（2026-08-23）：W-3e-4 完成 Runtime 清理诊断跨租户与 Device Token HTTP 负向测试；`TestRuntimeCleanupDiagnosticsBFFRejectsCrossTenantReadAndRetry` 证明平台底层运维事实按租户隔离，`TestRuntimeCleanupDiagnosticsBFFRejectsDeviceToken` 证明真实 Device Token 不能进入仅限用户会话的人工清理 BFF，也不能改变诊断状态。R2-6c 标记完成；R2-6、阶段 E 和 P4 仍因真实 PostgreSQL/Blob 与进程恢复保持未完成。

本轮完成（2026-08-23）：W-3e-5 接入 Runtime event worker 清理诊断维护循环；`TestProcessRuntimeEventsReconcilesCleanupDiagnostics` 证明 worker 可在维护循环中读取诊断、删除 Blob、收敛终态并写入 `runtime_cleanup` 心跳。该项使用 Memory/单进程装配，不代表专用 PostgreSQL 进程恢复、真实 Blob 组合故障或同 digest 幂等完成。

本轮完成（2026-08-23）：W-3f 统一非最终媒体 Artifact/DeliveryPackage 的 Blob 补偿清理边界；分镜素材、Seedance PromptPackage、批准快照导出和交付包在事实写入失败时不再遗留未挂接对象，批量交付失败会清理已写入对象，清理失败返回可恢复对象清单。证据：`internal/application/artifact_storage_test.go`。

本轮完成（2026-08-23）：W-5 补齐渠道到效果的血缘投影；`ProjectLineage` 增加 TaskDelivery、ChannelBinding、ChannelPublication 和 ApprovedSnapshot 基础快照继承边，渠道效果测试覆盖 Callback 幂等、PerformanceObservation、RatingDecision 与下游节点。该项使用 Memory/模拟渠道，真实 G 外部证据仍待补。

本轮完成（2026-08-23）：W-8 补齐交付包重试幂等；内容快照和视频最终成片交付统一使用稳定业务键，Memory/PostgreSQL 在重复创建或主键冲突后回读已有包，应用层测试覆盖 V3 内容链和视频 Golden Journey。该项不改变阶段 E/G 的外部验收状态；真实 PostgreSQL 并发、真实 Worker 和真实渠道仍待补。

本台账不授权 Git 提交、推送、部署、生产数据库操作或生产 API 调用。

本轮完成（2026-08-23）：W-9 补齐阶段 F 的分层交付编排入口。`DeliveryService.ExportJianying`、`POST /api/bff/projects/{projectID}/jianying-export` 和 CLI `artifact jianying-export` 复用 ApprovedSnapshot、MediaReview、Artifact、DeliveryPackage 与 Blob，不创建平行业务模型；应用、HTTP、CLI 和 exporter 测试均通过。F5 真实剪映人工导入、真实 Provider、渠道回执和效果回流仍待完成。

本轮完成（2026-08-23）：补齐文章与电商统一主链的最后一个兼容写入口。`CreateTaskRevision` 对文章、电商改为校验业务 Schema 后创建 `SubmissionRevision`，旧 DTO 由统一事实投影生成；`CreateTaskDelivery` 与阶段输出校验通过 ApprovedSnapshot 的任务工作区引用识别交付包归属。`go test ./internal/application`、契约测试和确定性渲染测试通过；真实 PostgreSQL、Blob、Provider、渠道和指标回流仍未执行。

本轮完成（2026-08-23）：普通 `video_script` 兼容入口完成同一收敛。`CreateTaskRevision` 先规范化旧脚本对象，再写入 `content_batch -> SubmissionRevision`；accepted 任务由既有 Gate 通过事实生成 `automated_gate` 决定和 `ApprovedSnapshot`，交付渲染器提供 JSON、Markdown、XLSX 三种平台产物。应用链、确定性渲染、Web、架构和文档门禁通过；真实 PostgreSQL、Blob、Provider、渠道和指标回流仍未执行。
