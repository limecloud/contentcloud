# 视频生产迁移文档同步计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件只跟踪 视频生产迁移相关文档的状态同步，不创建业务事实、技术实现或部署任务。所有状态必须以代码、契约、测试和真实外部验收证据为依据；没有证据时保留 `in_progress` 或 `not_started`。

## 1. 同步范围

需要保持一致的权威文档：

- [迁移计划](./12-video-production-migration.md)：范围、唯一主链和阶段门槛；
- [执行跟踪](./13-video-production-execution.md)：逐项证据和下一步；
- [推进计划](./16-video-production-progress-plan.md)：总进度和执行队列；
- [完成计划](./20-video-production-completion-plan.md)：P0-P7 收口队列；
- [迁移工作台账](./24-video-production-worklog.md)：短周期工作项、证据和下一项执行顺序；
- [阶段 D2 PostgreSQL 对照计划](./25-stage-d2-postgres-parity-plan.md)：Memory/PostgreSQL 字段、事务、RLS 和恢复语义的对照范围；
- [E1d-b PostgreSQL Final Render 计划](./26-e1d-b-postgres-final-render-plan.md)：最终成片 Artifact 与最终审核的事务、回滚和租户隔离证据；
- [E1d-b Blob 故障注入计划](./27-e1d-b-blob-fault-injection-plan.md)：Blob 读写失败、事实原子性和跨进程恢复证据；
- [E1d-b R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)：状态 CAS、自动登记、权限保护的查询/重试入口和跨进程恢复测试；
- [E1d-b 恢复验收计划](./30-e1d-b-recovery-acceptance-plan.md)：R2-6/R3-R5 的独立进程、PostgreSQL/Blob 和同 digest 幂等验收；
- [能力地图](../../infra/01-capability-map.md) 与 [交付路线图](../../infra/03-delivery-roadmap.md)：对外能力状态和交付依赖；
- 阶段专项计划：D、E、F 的短周期实施和验收门槛。

## 2. 当前文档任务

- [x] DOC-1：对齐阶段 F 状态。`20` 已从 `not_started` 修正为 `in_progress`，并登记 F0 文档基线及 F1-F4 已完成、F5 待验收的缺口。
- [ ] DOC-2：阶段 D 完成 Memory/PostgreSQL 对照后，同步四份主路线图和能力地图。
- [ ] DOC-3：阶段 E 完成 PostgreSQL/Blob 故障恢复或真实 Worker 验收后，更新对应门槛和证据。
- [x] DOC-4：实现阶段 F exporter 和 lint 后，补充 ZIP 条目、摘要、失败矩阵和 lint 证据；真实人工导入仍由 F5 跟踪。
- [ ] DOC-5：阶段 G 完成真实 Provider、交付、发布回执和效果回流后，收口迁移状态。
- [x] DOC-12：登记 Blob `Get` 失败故障注入证据；该项已扩展登记 Blob `Put` 失败和写入后清理证据，跨进程恢复和真实 PostgreSQL 执行仍待补。
- [x] DOC-6：同步 视频生产迁移 文档入口索引，注册 12-24 计划并更新基础设施文档更新时间；工作台账见[24](./24-video-production-worklog.md)。
- [x] DOC-7：建立阶段 D2 PostgreSQL 对照计划，并将其注册到迁移入口、工作台账和主路线图；D2 测试本身仍待真实测试数据库执行。
- [x] DOC-8：记录 D2-1 接口与测试盘点，明确 Memory 已有证据、PostgreSQL 媒体专用集成测试缺口和 D2-2 的 skip 规则。
- [x] DOC-9：同步 D2-2 PostgreSQL 集成测试实现证据；明确测试覆盖范围、skip 原因和 D2-3 真实数据库执行缺口。
- [x] DOC-10：同步 E1d-b PostgreSQL 原子事务测试入口、回滚和租户隔离覆盖；明确未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 时的 skip 边界，真实 PostgreSQL/Blob 执行仍待补。
- [x] DOC-11：将 E1d-b PostgreSQL 专项计划补入基础设施文档索引和交付路线图证据链，避免 `26` 只在 V8 README 中可见。
- [x] DOC-14：建立 E1d-b 恢复执行计划，明确清理失败诊断的最小字段、跨进程重试边界和真实 PostgreSQL 验收门槛；已同步 `21`、`27`、`24` 及入口索引。
- [x] DOC-15：将 E1d-b 恢复执行计划补入完成计划 `20` 的历史记录，明确 `R0` 仅代表文档基线，避免把当前进程清理错误误报为后台恢复能力。
- [x] DOC-16：同步 R1 持久化清理诊断契约；更新 `12`、`13`、`16`、`20`、`21`、`24`、`27`、能力地图和交付路线图，明确 Runtime 唯一事实落点、迁移 `00053`、Memory/PostgreSQL 证据以及 R2-R6 未完成边界。
- [x] DOC-17：新增 R2 清理重试专项计划，完成接口、权限和 HTTP 边界审计；同步 `12`、`13`、`16`、`20`、`24`、`28` 和两个入口索引，保持状态 CAS、自动登记、后台重试和跨进程恢复为未完成项。
- [x] DOC-18：同步 R2-2 诊断状态 CAS 证据；更新 `12`、`13`、`16`、`20`、`24`、`28`、`29`、能力地图和交付路线图，明确仅状态 CAS 完成，自动登记、查询/重试和跨进程恢复仍未完成。
- [x] DOC-19：同步 R2-3/R2-5 自动登记、查询/详情/CAS 重试 BFF 证据；新增 `30` 恢复验收计划，并将 R2-6 独立恢复、真实 PostgreSQL/Blob 和同 digest 幂等保留为未完成门槛。
- [x] DOC-21：同步阶段 F 的应用服务、HTTP BFF、CLI 分层入口和契约测试证据；入口保持在交付编排层，真实剪映人工导入、真实 Provider、渠道回执和效果回流仍保持未完成。

## 3. 更新协议

完成任一代码或验收项后：

1. 先在对应专项计划中记录证据和未完成门槛。
2. 更新 `13`、`16`、`20` 的队列和阶段状态。
3. 同步 `12`、能力地图和交付路线图中的摘要描述。
4. 运行 `git diff --check`；若测试、数据库、Provider 或人工验收未执行，明确写出跳过原因。

本计划不授权 Git 提交、推送、部署、数据库删除或生产环境操作。

本轮完成记录（2026-08-23）：F4 `LintJianyingArchive` 已有代码和测试证据，已同步 `12`、`13`、`16`、`20`、`22`、能力地图及交付路线图；F5 真实 Artifact 的人工剪映导入仍未执行，阶段 F 保持 `in_progress`。

本轮完成记录（2026-08-23）：DOC-6 已完成，`roadmap/v8/README.md`、`infra/README.md`、能力地图和交付路线图的入口/更新时间已同步；短周期后续工作转由[迁移工作台账](./24-video-production-worklog.md)跟踪。

本轮完成记录（2026-08-23）：DOC-7 已完成，新增[阶段 D2 PostgreSQL 对照计划](./25-stage-d2-postgres-parity-plan.md)，并同步 `12`、`13`、`16`、`20`、`24` 及两个入口索引；D2 仍保持 `in_progress`，未宣称真实数据库测试完成。

本轮完成记录（2026-08-23）：DOC-8 已完成，D2-1 接口与测试盘点写入专项计划和工作台账；媒体专用 PostgreSQL 集成测试尚未执行，D2 仍保持 `in_progress`。

本轮完成记录（2026-08-23）：DOC-9 已完成，D2-2 测试实现已写入 `25`、`24`、`12`、`13`、`16`、`20`、能力地图和交付路线图；真实 PostgreSQL 测试仍因未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 跳过，D2 和阶段 D 保持 `in_progress`。

本轮完成记录（2026-08-23）：DOC-10 已完成，E1d-b PostgreSQL 原子事务测试入口已写入 `18`、`19`、`20`、`21`、`24` 及阶段 E 执行说明；测试实现、全量测试和 diff 校验通过，真实 PostgreSQL/Blob 执行仍因未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 跳过，E1d 和阶段 E 保持 `in_progress`。

本轮补充（2026-08-23）：新增 `26-e1d-b-postgres-final-render-plan.md` 作为 E1d-b PostgreSQL 独立跟踪文件，并注册到文档入口和同步范围；它不改变 E1d-b、阶段 E 或 D2 的完成状态。

本轮补充（2026-08-23）：DOC-11 已完成，`docs/infra/README.md` 和 `docs/infra/03-delivery-roadmap.md` 已加入 `26` 计划入口；真实 PostgreSQL/Blob 验收门槛和 `in_progress` 状态保持不变。

本轮补充（2026-08-23）：DOC-12 已完成，新增 `27` Blob 故障注入计划并登记 `Get` 失败测试；主路线图同步保留 `Put`、跨进程恢复和真实 PostgreSQL 缺口。

本轮补充（2026-08-23）：DOC-13 已完成，补充 Blob `Put` 写入前/写入后故障注入和临时对象清理证据；主路线图同步保留清理后台重试、跨进程恢复、真实 PostgreSQL 和真实 Worker 缺口。

本轮补充（2026-08-23）：DOC-14 已完成，新增 [E1d-b 恢复执行计划](./28-e1d-b-recovery-execution-plan.md)，统一持久化清理诊断字段、后台重试、跨进程恢复和真实 PostgreSQL 的未完成边界；`R0` 文档基线完成，R1-R6 仍待执行。

本轮补充（2026-08-23）：DOC-15 已完成，`20` 的完成记录已明确链接 `28` 并保留 R1-R6 未完成边界；未改变 E1d-b 或阶段 E 的状态。

本轮补充（2026-08-23）：DOC-16 已完成，R1 的 `RuntimeCleanupDiagnostic` 契约、Memory/PostgreSQL 存储和租户 + `object_key` 唯一约束已同步到主路线图及专项计划；后台重试、跨进程恢复和真实 PostgreSQL 执行仍保持未完成。

本轮补充（2026-08-23）：DOC-17 已完成，新增[ R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)，完成接口、权限和 HTTP 边界审计；R2-2 至 R2-7 仍待实现和验证，未改变 E1d-b 或阶段 E 的完成状态。

本轮补充（2026-08-23）：DOC-18 已完成，R2-2 状态 CAS 的接口、状态机、版本冲突和定向测试证据已同步；R2、E1d-b 与阶段 E 继续保持 `in_progress`。

本轮补充（2026-08-23）：DOC-19 已完成，`12`、`13`、`16`、`20`、能力地图和交付路线图已同步 R2-3/R2-5 实现事实；新增 [E1d-b 恢复验收计划](./30-e1d-b-recovery-acceptance-plan.md) 跟踪 R2-6/R3-R5，真实 PostgreSQL/Blob 和独立进程证据仍未执行。

本轮补充（2026-08-23）：DOC-20 已完成，`CreateDeliveryPackage` 与 `BuildTaskDeliveryPackage` 的交付包稳定业务键幂等、Memory/PostgreSQL 冲突回读和应用层重试测试已同步到 `13`、`16`、`20` 及本台账；真实 PostgreSQL 并发证据仍待专用测试库执行。
