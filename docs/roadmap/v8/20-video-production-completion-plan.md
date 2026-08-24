# 视频生产迁移完成计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件是 视频生产迁移的执行计划和进度跟踪入口。它只引用 ContentCloud 现有的业务事实、执行事实和交付事实，不创建 视频生产迁移 专属数据库、队列、Session、Asset 或发布状态。阶段 D2 的 PostgreSQL 对照范围和执行门槛见[D2 PostgreSQL 对照计划](./25-stage-d2-postgres-parity-plan.md)，E1d-b Final Render 原子事务范围见[E1d-b PostgreSQL Final Render 计划](./26-e1d-b-postgres-final-render-plan.md)，Blob 故障矩阵见[27](./27-e1d-b-blob-fault-injection-plan.md)，清理失败恢复执行见[28](./28-e1d-b-recovery-execution-plan.md)，R2 实现拆分见[R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)。

## 1. 完成目标

验证下面这条唯一主链可以在 ContentCloud 内持续推进，并且每个阶段都有代码、契约、正常/失败/重复/恢复测试及必要的真实外部证据：

```text
Source / WorkspaceMaterial / Evidence
  -> Knowledge / Brief / ContentBatch / ContentItem
  -> SubmissionRevision -> Review -> ApprovedSnapshot
  -> WorkTask / StageRun -> MediaGenerationJob / Artifact
  -> MediaReview -> DeliveryPackage
  -> Jianying export -> ChannelPublication / Receipt / PerformanceObservation
```

“完成”不等于页面存在、Fixture 通过、模拟 Provider 成功或默认确定性 Worker 只固化了输入血缘。未完成的真实外部依赖必须继续保留为未完成状态。

## 2. 当前基线

| 区域 | 状态 | 已有证据 | 主要缺口 |
| --- | --- | --- | --- |
| A-C 基础内容、审批和批量准入 | `complete` | 现有领域代码、提交/审核/幂等测试 | 真实跨版本失效传播仍需阶段 G 验收 |
| D Provider 异步恢复与费用 | `in_progress` | D1 模拟状态恢复矩阵；D2-1 接口与测试盘点；D2-2 媒体专用 PostgreSQL 对照测试实现 | D2-3 专用 PostgreSQL 真实执行、真实 Provider 和账单回执 |
| E 后期合成与审核门禁 | `in_progress` | Manifest 校验、原子写入端口、交付审核门禁、PostgreSQL 原子事务测试入口 | PostgreSQL/Blob 真实执行、失败恢复、真实 Worker |
| F 剪映草稿导出 | `in_progress` | F0 行为审计、F1 确定性 ZIP、F2 完整血缘校验、F3 失败矩阵、F4 导出 lint、F4b 分层交付编排入口及契约测试已完成；专项计划见[阶段 F 剪映导出计划](./22-stage-f-jianying-export-plan.md) | F5：真实人工导入验收 |
| G 发布与效果回流 | `not_started` | 现有 Channel/Performance 领域基础 | 真实发布回执、效果导入和 ApprovedSnapshot 血缘闭环 |

## 3. 执行队列

- [x] P0：建立迁移文档基线、状态口径、更新协议和证据要求。
- [x] P1：实现 `CreateFinalRender` 的 Artifact + 最终审核原子写入端口。
- [x] P2：补齐 Memory 原子写入成功/失败原子性测试。
- [ ] P3：补齐并运行 PostgreSQL 原子事务集成测试；测试入口已新增，范围和证据规则见[D2 PostgreSQL 对照计划](./25-stage-d2-postgres-parity-plan.md)和[E1d-b PostgreSQL Final Render 计划](./26-e1d-b-postgres-final-render-plan.md)；未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 时只能明确跳过。
- [ ] P4：补齐 Blob `Get`/`Put`/清理失败测试，并验证失败后不能创建交付事实；Blob `Get`/`Put` 无事实测试、数据库失败后的清理诊断、R2-1/R2-2 边界审计与状态 CAS、R2-3 自动登记、R2-4/R2-5 查询/重试入口已完成；独立进程恢复、真实 PostgreSQL/Blob 幂等仍待补，详见[27](./27-e1d-b-blob-fault-injection-plan.md)、[28](./28-e1d-b-recovery-execution-plan.md)和[29](./29-e1d-b-r2-cleanup-retry-plan.md)。
- [ ] P5：接入真实后期 Worker，旁白、字幕、时间轴、品牌和 CTA 必须来自 Manifest。
- [ ] P6：完成确定性 Jianying 草稿归档和导出 lint；F1-F4b exporter、分层入口、输入校验、失败矩阵、契约测试和 lint 已完成，F5 真实导入仍待补，具体拆分见[阶段 F 剪映导出计划](./22-stage-f-jianying-export-plan.md)。
- [ ] P7：完成真实 Provider -> 交付 -> 发布回执 -> PerformanceObservation 端到端验收。

## 4. 阶段门槛

### E1d

E1d 只有在以下证据全部存在后才能标记完成：

1. Memory 与 PostgreSQL 对原子写入、租户隔离和幂等语义有对照测试。
2. Blob 读取、写入和清理失败都不会留下不可诊断的 Artifact、审核或交付事实。
3. 同一 Manifest digest 重试只产生一个逻辑结果。
4. 最终审核为 `pending`、`changes_requested` 或 `rejected` 时，不能创建 `DeliveryPackage`。

### F

F 只有在导出输入全部来自已批准快照、已批准最终审核和现有 `DeliveryPackage`，且同一输入生成同字节 ZIP 后才能进入真实渠道验收。

### G

G 只有在真实 Provider、真实发布/人工回执和效果导入都写入现有 ContentCloud 事实模型后才能完成。任何模拟结果都只能作为开发证据。

## 5. 更新协议

每完成一个 P 项：

1. 先提交代码、契约和测试证据到工作区。
2. 更新本文件的队列和证据。
3. 同步 `16-video-production-progress-plan.md`、`13-video-production-execution.md`、阶段专项计划、能力地图和交付路线图。
4. 运行相关测试、`go test ./...` 和 `git diff --check`；若真实数据库或 Provider 未运行，明确记录跳过原因。

本计划不授权 Git 提交、推送、部署或生产环境操作。

本轮新增专项跟踪：[E1d-b Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md)。当前仅完成数据库事实写入失败后的临时对象清理与清理失败诊断；P4 仍保持未完成。

本轮补充专项跟踪：[E1d-b 恢复执行计划](./28-e1d-b-recovery-execution-plan.md)。该计划将持久化清理诊断、后台重试、跨进程恢复和 PostgreSQL/Blob 组合幂等拆分为 R1-R6；当前已完成 R0 文档基线和 R1 持久化诊断事实落点，不能将当前进程返回的清理错误误报为后台恢复能力。

本轮新增专项跟踪：[阶段 F 剪映导出计划](./22-stage-f-jianying-export-plan.md)。已完成 F0 文档基线及 F1-F4 exporter、输入校验、失败矩阵和 lint；P6 仍等待 F5 真实导入验收。

本轮 P6 增量：F1-F4b 已完成 exporter、完整输入/血缘校验、应用/HTTP/CLI 分层入口、契约测试、失败矩阵和独立导出 lint 测试；P6 仍保持未完成，直到真实剪映导入验收完成。

本轮文档对齐：修正本文件阶段 F 的状态和证据，使其与迁移计划、执行台账、能力地图及阶段 F 专项计划一致；同步规则和后续文档任务见[文档同步计划](./23-documentation-sync-plan.md)。

本轮 P4 增量：完成 Blob `Get`/`Put` 失败故障注入，证明读取或写入失败不创建最终 Artifact 或最终媒体审核，并统一清理写入后报错的临时对象；P4 仍保持未完成。

本轮 E1d-b R1 增量：完成 `RuntimeCleanupDiagnostic` 持久化契约、Memory/PostgreSQL 存储实现和 `00053_runtime_cleanup_diagnostics.sql`；诊断记录按租户隔离并以 `object_key` 唯一。该项只完成持久化事实落点，CreateFinalRender 自动登记、后台重试、跨进程恢复和真实 PostgreSQL/Blob 组合执行仍待 `28` 的 R2-R5，P4 和阶段 E 保持未完成。

本轮 E1d-b R2-1 增量：完成接口、人工权限和 HTTP 边界审计，新增[R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)跟踪状态 CAS、自动登记、查询/重试服务、路由和恢复测试；P4 与阶段 E 继续保持未完成。

本轮 E1d-b R2-2 增量：完成 `RuntimeCleanupDiagnostic` 状态 CAS；Memory/PostgreSQL 实现及旧版本、非法状态、终态重试和身份不变测试通过。该项不代表 `CreateFinalRender` 自动登记、人工查询/重试、跨进程恢复或真实 PostgreSQL 已完成。

本轮 E1d-b R2-3/R2-5 增量：`CreateFinalRender` 已在事实写入失败且临时 Blob 删除失败时自动登记脱敏诊断；Operations Service 与 BFF 已提供租户范围查询、详情和受角色保护的 CAS 重试。删除成功、对象不存在和再次失败的状态收敛及退避测试已通过；R2-6 独立进程恢复、真实 PostgreSQL/Blob 和同 digest 幂等仍待补，P4 与阶段 E 保持未完成。

本轮 E1d-b R2-6c 增量（2026-08-23）：补充清理诊断跨租户和 Device Token HTTP 负向测试。外租户列表为空、详情/重试 404 且响应不泄漏诊断字段；真实 Device Token 访问人工清理 BFF 的列表、详情和重试均返回 401，不能改变诊断状态或尝试次数。独立进程恢复、真实 PostgreSQL/Blob 和同 digest 幂等仍待补，P4 与阶段 E 保持未完成。

本轮 E1d-b R2-6b 增量（2026-08-23）：`contentcloud-worker` 已复用 Runtime event loop 自动消费清理诊断；过期 `retrying` claim 以 CAS 回收，失败重试尊重 `next_retry_at`，并新增 `runtime_cleanup` 健康心跳。Memory/跨 Application 实例测试通过；专用 PostgreSQL 进程执行、Blob 组合故障和同 digest 幂等仍待补，P4 与阶段 E 保持未完成。

本轮 E1d-b/P4 增量（2026-08-23）：统一 `persistArtifactObject` 与 `cleanupSpeculativeObjects` 边界，分镜素材、Seedance PromptPackage、ApprovedSnapshot 导出和 DeliveryPackage 在事实写入失败时都会补偿删除已写入 Blob；交付包批量失败会清理全部已写入对象，清理失败返回包含对象列表的可恢复结构化错误。新增 `internal/application/artifact_storage_test.go` 覆盖原始事实错误、批量清理和清理失败三类路径。该项不替代真实 PostgreSQL/Blob 组合故障验收。

本轮 G 血缘投影增量（2026-08-23）：`ProjectLineage` 现在投影 `TaskDelivery`、`ChannelBinding`、`ChannelPublication`，并通过任务交付清单回溯 `ApprovedSnapshot -> TaskDelivery -> ChannelPublication -> PerformanceObservation -> RatingDecision`；批准快照的 `BaseSnapshotIDs` 也会形成显式 `derived_from` 边。`TestRemoteChannelCallbackIsDeduplicatedAndOwnsPublishedTransition` 已覆盖回执、效果导入、学习决策和下游血缘节点。该项仍是 Memory/模拟渠道证据，G 真实 Provider、渠道和指标外部验收保持未完成。

本轮闭环补充（2026-08-23）：`CreateDeliveryPackage` 与 `BuildTaskDeliveryPackage` 已完成稳定业务键幂等，重复请求不会生成新的 DeliveryPackage；PostgreSQL 主键冲突路径会回读已提交交付包，临时 Blob 仍按既有补偿清理规则处理。该证据来自 Memory/应用层测试，真实数据库并发执行和外部渠道验收仍保持未完成。

本轮分层平台补充（2026-08-23）：文章与电商工作台已完成统一平台主链的本地验收。文章由 `article_collaboration` 别名进入文章 SOP，电商由 `commerce_content` 五阶段 SOP 进入 Runtime；两者都冻结 `WorkTask.RequestedOutput`、SOP、ExecutionBinding 和 InputDigest，不创建工作台专属任务、审批、Artifact、Delivery 或 Performance 模型。Bootstrap 设备授权投影也已修复为 Memory/PostgreSQL 一致口径。该项不改变 D/E/F/G 的真实基础设施和外部闭环状态。

本轮补充（2026-08-23）：统一内容版本兼容入口已完成事实收敛，文章和电商新写入不再创建旧 `TaskRevision`；交付包通过 ApprovedSnapshot 回溯任务工作区，不要求业务内容项 ID 等于任务 ID。新增的电商全链和契约测试覆盖 WorkTask、Runtime、Submission、Review、ApprovedSnapshot、三格式 Artifact、DeliveryPackage、Performance 和 Rating。真实 PostgreSQL/Blob/Provider/渠道验收仍保持未完成。
