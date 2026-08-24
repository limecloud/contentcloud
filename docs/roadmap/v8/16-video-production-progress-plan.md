# 视频生产迁移推进计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件是 视频生产迁移的当前进度账本，负责回答“现在完成了什么、下一项是什么、还缺什么证据”。领域映射和阶段完成门槛以[迁移计划](./12-video-production-migration.md)为准；具体执行记录以[执行跟踪](./13-video-production-execution.md)和对应短周期计划为准；D2 对照范围见[D2 PostgreSQL 对照计划](./25-stage-d2-postgres-parity-plan.md)，E1d-b Blob 故障矩阵见[27](./27-e1d-b-blob-fault-injection-plan.md)，恢复执行队列见[28](./28-e1d-b-recovery-execution-plan.md)，R2 实现队列见[29](./29-e1d-b-r2-cleanup-retry-plan.md)；文档一致性由[文档同步计划](./23-documentation-sync-plan.md)跟踪。本文件不新增任务、资产、审核、交付或发布事实模型。

## 1. 目标主链

```text
Source / WorkspaceMaterial / Evidence
  -> Knowledge / Brief / ContentBatch / ContentItem
  -> SubmissionRevision -> Review -> ApprovedSnapshot
  -> WorkTask / StageRun -> MediaGenerationJob / ProviderAttempt / Effect
  -> Artifact -> MediaReview -> DeliveryPackage
  -> Jianying export -> ChannelPublication / Receipt / PerformanceObservation
```

视频生产迁移 只贡献视频生产行为、输入输出约束和验收场景。ContentCloud 继续作为唯一事实源，不引入 其 Python 服务、独立数据库、队列、Session、Asset 或发布状态模型。

## 2. 阶段进度

状态约定：`not_started`、`in_progress`、`blocked`、`complete`。阶段只有在代码、契约、正常/失败/重复/恢复测试、权限与摘要语义以及真实外部验收门槛全部满足后才能标记为 `complete`。

| 阶段 | 当前状态 | 本阶段完成项 | 下一项 / 阻塞证据 |
| --- | --- | --- | --- |
| A. 边界与现状对账 | `complete` | 视频生产迁移 能力已映射到 ContentCloud 唯一主链 | 维持无平行事实模型 |
| B. 视觉资产一致性与镜头绑定 | `complete` | 角色、场景、道具、商品参考固定摘要并纳入锁定分镜 | 阶段 G 仍需真实跨版本失效传播验收 |
| C. 批量视频准入 | `complete` | 整批预检、费用确认、预算/并发门禁、原子写入和幂等 | 维持批量入口与 Provider 策略一致 |
| D. Provider 异步恢复与费用对账 | `in_progress` | **D1 模拟状态恢复矩阵和 D2-2 PostgreSQL 集成测试实现已完成**：unknown -> running/failed/cancelled，不重复 Submit/Attempt；终态费用统一回写；集成测试覆盖 CAS、费用和 RLS | D2-3 在专用 PostgreSQL 上真实执行；随后 D3 真实 Provider 与账单回执 |
| E. 视频后期、旁白与确定性合成 | `in_progress` | E0/E1a 文档契约、E1b Composition Manifest 值对象、E1c Manifest 驱动 `CreateFinalRender`、平台 Composition Worker 执行端口、确定性独立 MP4 输出、E1d-a 矩阵、E1d-c 交付硬门禁、E1d-b Memory 原子写入测试、PostgreSQL 原子事务测试入口和应用层 Blob `Get`/`Put` 故障注入已完成；R2-1/R2-2 边界审计与状态 CAS、R2-3 清理失败自动登记、R2-4/R2-5 查询/详情/CAS 重试 BFF 已完成 | E1d-b R2-6 独立恢复、真实 PostgreSQL/Blob、同 digest 组合幂等；随后接入具备真实音视频编码能力的受控 Worker 并完成 E1e 证据 |
| F. 剪映草稿导出 | `in_progress` | F0 文档基线、F1 确定性 ZIP、F2 输入/血缘校验、F3 失败矩阵、F4 导出 lint 已完成 | F5 真实 Artifact 的人工剪映导入验收 |
| G. 真实闭环与效果回流 | `not_started` | - | 需要真实 Provider、交付包、发布/人工回执和 PerformanceObservation |

## 3. 当前执行队列

- [x] D1：补齐 Provider 状态查询异常后的 `running`、`failed` 和已请求取消 `cancelled` 恢复路径，并证明恢复不重复提交。
- [ ] D2：按[D2 PostgreSQL 对照计划](./25-stage-d2-postgres-parity-plan.md)在 Memory 与 PostgreSQL 上核对字段、版本、租户隔离和幂等语义；D2-1 盘点和 D2-2 测试实现已完成，下一项为 D2-3 专用 PostgreSQL 真实执行。
- [ ] D3：使用真实 Provider 测试环境验证异步状态、取消和账单回执。
- [x] E0：完成旁白、字幕、音频、品牌/CTA、时间轴和已选视频的输入契约审计，并登记阶段 E 的真实完成门槛。
- [x] E1a：完成版本化 composition manifest 的字段、稳定摘要、幂等和权限边界文档契约。
- [x] E1b：实现版本化 manifest 值对象、canonical JSON、`sha256:` digest、旁白/字幕显式禁用和时间轴边界校验。
- [x] E1c：将 `CreateFinalRender` 接入 manifest，重新校验批准快照、选中视频审核、时间轴 Artifact 和输出契约；同一 digest 幂等复用。
- [x] E1d-a：完成失败、恢复、重复执行和摘要漂移测试矩阵文档基线。
- [x] E1d-c：证明最终审核未批准时不能创建可交付 `DeliveryPackage`。
- [x] E1d-b-Memory：补齐 Memory 原子写入成功/失败原子性测试。
- [ ] E1d-b：在 PostgreSQL 上运行矩阵，补齐 Blob 失败恢复和失败后的可恢复断点；应用层 Blob `Get`/`Put` 失败无事实测试、数据库失败后的临时对象清理、清理失败自动登记和租户范围查询/重试已完成；跨进程恢复、同 digest PostgreSQL/Blob 幂等和真实 PostgreSQL 仍待补，见 [E1d-b Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md)、[E1d-b Blob 故障注入计划](./27-e1d-b-blob-fault-injection-plan.md)、[E1d-b 恢复执行计划](./28-e1d-b-recovery-execution-plan.md) 和 [E1d-b PostgreSQL Final Render 计划](./26-e1d-b-postgres-final-render-plan.md)。
- [x] E1d-b-R2-1：完成 Runtime 清理诊断接口、人工权限和 HTTP 边界审计；仅完成文档与证据盘点，不代表状态 CAS、自动登记或后台重试已实现，后续按[R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)执行。
- [x] E1d-b-R2-2：完成 Runtime 清理诊断状态 CAS；覆盖 Memory/PostgreSQL 版本冲突、合法状态转移、终态保护和身份不变，真实 PostgreSQL 执行仍待专用数据库。
- [x] E1d-b-R2-3：`CreateFinalRender` 在事实写入失败且 Blob 清理失败时自动登记脱敏诊断，登记失败不覆盖原始错误。
- [x] E1d-b-R2-4/R2-5：完成租户范围查询、详情、CAS 重试服务和 BFF 路由；仅允许 `tenant_admin`/`project_manager` 人工操作，Worker/Device 不得旁路。
- [x] E1d-b-R2-6c：补齐跨租户和 Device Token HTTP 负向验收；外租户列表为空、详情/重试 404 且响应脱敏，真实 Device Token 访问人工清理 BFF 的列表、详情和重试均返回 401，诊断事实保持不变。
- [x] E1d-b-R2-6b-应用层：Runtime worker 已接入持久化清理诊断恢复；过期 claim 可 CAS 回收，失败退避受尊重，跨 Application 实例测试通过。
- [ ] E1d-b-R2-6：补独立 Store/进程恢复、终态幂等、跨租户和真实 PostgreSQL 执行证据。
- [x] E2-Worker-Port：增加平台 Composition Worker 端口；默认确定性 Worker 生成独立 MP4 并把 Manifest、时间轴、旁白/字幕、品牌/CTA 和所有输入摘要写入可审计输出元数据；Worker 不拥有任务、审核或交付状态。
- [ ] E1e：在受控环境运行具备真实音视频编码能力的 Worker，证明旁白、字幕、时间轴和品牌/CTA 实际生效。
- [x] F1：实现可复现的 Jianying 草稿归档和确定性 ZIP。
- [x] F2：校验 ApprovedSnapshot、最终/选中视频审核、CompositionManifest、DeliveryPackage、Artifact 和 Blob 血缘。
- [x] F3：补正常、重复、摘要漂移、审核状态、路径穿越、交付包缺产物和 Blob 缺失测试。
- [x] F4：实现 `LintJianyingArchive`，验证 ZIP 条目、JSON 摘要、素材大小、SHA-256 和 MIME/扩展名契约。
- [x] F4b：补齐应用、HTTP BFF 和 CLI 交付编排入口及契约测试；入口复用现有事实和 Blob，只产生派生归档。
- [ ] F5：用真实已批准最终 Artifact 运行本地剪映导入验收。
- [x] F0：完成 视频生产迁移 剪映/presentation bundle 行为审计，并建立[阶段 F 剪映导出计划](./22-stage-f-jianying-export-plan.md)。

F4 完成证据：`internal/local/export/jianying.go` 提供独立 `LintJianyingArchive`；`internal/local/export/jianying_test.go` 覆盖确定性归档的 lint 通过路径。真实 Artifact 的人工剪映导入尚未执行，因此阶段 F 保持 `in_progress`。
- [ ] G1：完成一次真实 Provider 到交付、发布回执和效果观察的端到端验收。

本轮增量（2026-08-23）：补齐 Artifact/DeliveryPackage 的 Blob 补偿清理边界和 `ApprovedSnapshot -> TaskDelivery -> ChannelPublication -> PerformanceObservation -> RatingDecision` 血缘投影；Memory/模拟渠道回归测试通过，真实 PostgreSQL/Blob、Provider、渠道和指标外部证据仍待执行。

本轮平台分层增量（2026-08-23）：文章与电商工作台已从静态体验声明进入统一 `WorkTask -> SOP/JobPlan -> Runtime` 主链。文章通过 `article_collaboration` 模板别名解析，电商通过 `commerce_content` 五阶段 SOP 解析；两者的业务简报和固定工作台引用都写入 `WorkTask.RequestedOutput`，Runtime policy 统一为 `runtime-policy/customer-studio-v1`。`TestCustomerStudioArticleAndCommerceWorkbenchesUseSharedTaskAndRuntime` 已覆盖不同工作台布局、任务类型、请求输入冻结和 Runtime 节点；该证据不替代真实 PostgreSQL、Provider、Blob、Worker、外部渠道和效果指标验收。

本轮设备事实增量（2026-08-23）：Bootstrap 消费授权后，Memory 的 `ConnectedDevices` 投影改为按有效设备授权动态计算，与 PostgreSQL 的 `project_device_grants` 统计口径一致；重连、Daemon 心跳、重复授权和整机撤销测试已覆盖幂等与回收，不再要求测试或客户端额外调用一次 Attach 才能让 Studio 看到执行端。

## 4. D1 完成证据

- `internal/application/media_pipeline_state_test.go`：`TestProviderStatusUnknownRecoversByPollingWithoutResubmit`。
- `internal/application/media_pipeline_state_test.go`：`TestProviderStatusUnknownRecoversToTerminalStatesWithoutDuplicateAttempt`。
- `internal/application/media_pipeline_cancel_test.go`：取消 unknown 后 Runtime Effect 收敛测试。
- `go test ./...` 与 `git diff --check` 已通过。

## 4.1 D2-2 测试实现证据

- `internal/persistence/postgres/media_integration_test.go`：覆盖 Provider Profile/Binding、媒体任务幂等与 CAS、Attempt unknown 恢复字段、终态费用/usage 和 RLS 跨租户负向断言。
- 测试入口强制依赖 `CONTENTCLOUD_TEST_DATABASE_URL`；当前未设置，因此 `TestMediaPipelinePersistenceWithPostgres` 明确跳过。
- D2-2 只表示测试实现完成；真实 PostgreSQL 执行仍属于 D2-3，不改变阶段 D 的 `in_progress` 状态。

D1 仅代表模拟 Provider 的状态机和费用一致性已完成，不代表阶段 D、视频生产迁移或真实外部闭环完成。

E0 和 E1a 仅代表阶段 E 的输入审计与 manifest 文档契约已完成，不代表确定性合成 Worker、旁白/字幕处理或真实最终成片已完成。具体约束见[阶段 E 合成计划](./17-stage-e-composition-plan.md)和[E1 Composition Manifest 计划](./18-stage-e1-composition-manifest-plan.md)。

E1b 已完成值对象层实现：`CompositionManifest.Validate` 固定批准快照、选中视频审核、旁白/字幕状态、品牌/CTA、时间轴、renderer 和输出契约；`CanonicalJSON` 与 `Digest` 不包含时间、执行 ID 或临时路径。E1c 已将该 digest 接入 `CreateFinalRender` 的输入校验和幂等边界；Composition Worker 端口已接线，默认 Worker 生成独立 MP4 并固化完整输入血缘元数据，但尚未宣称完成真实旁白、字幕混音或画面烧录。清理失败诊断已接入自动登记、查询和 CAS 重试；独立进程恢复、真实 PostgreSQL/Blob 组合执行和同 digest 幂等仍未完成。

E1d-a 已完成验证矩阵文档基线，E1d-c 已通过 `TestMarketingVideoGoldenJourney` 证明 pending 最终审核不能创建交付包。E1d-b-Memory 已通过 `internal/persistence/memory/final_render_test.go` 证明 Artifact 与最终审核在失败时均不落库、成功时同时落库；应用层 Blob `Get`/`Put` 故障注入已通过，数据库失败后的清理诊断、自动登记、查询和 CAS 重试已覆盖；本轮新增 `RuntimeCleanupDiagnostic` 的 Memory/PostgreSQL 持久化契约、迁移和状态 CAS，但真实 PostgreSQL 执行、跨进程恢复、同 digest PostgreSQL/Blob 幂等以及真实 Worker 仍未完成。详见[阶段 E1d 失败、恢复与摘要漂移验证计划](./19-stage-e1d-failure-recovery-plan.md)和[恢复执行计划](./28-e1d-b-recovery-execution-plan.md)。

本轮补充（2026-08-23）：交付域完成 DeliveryPackage 重试幂等。内容批次和视频最终成片均以 `tenant + approved_snapshot + content_item` 作为稳定业务键；应用层先查询，持久化主键冲突后回读已有包，Memory/PostgreSQL 语义一致。该项已由 V3 全链路和视频 Golden Journey 测试覆盖，真实 PostgreSQL 连接仍未执行。

本轮主链收口（2026-08-23）：文章和电商通过旧内容版本兼容入口提交时，服务端改为创建统一 `SubmissionRevision`，并由既有审核、批准快照、Artifact、DeliveryPackage、PerformanceObservation 和 RatingDecision 接续；`TaskRevision` 只作为无统一提交事实时的历史读取回退。交付包归属改为校验 ApprovedSnapshot 的项目和任务工作区引用。Memory 端到端测试和电商契约/确定性渲染测试通过，真实 PostgreSQL、Blob、Provider、渠道和指标回流仍待独立验收。

本轮继续收口（2026-08-23）：普通 `video_script` 兼容入口也改为创建 `content_batch` 的 `SubmissionRevision`，旧 `TaskRevision` 只作为响应投影；已通过短视频流程 Gate 的 accepted 任务记录 `automated_gate` 决定并生成 `ApprovedSnapshot`，通用视频脚本渲染器统一生成 JSON、Markdown 和 XLSX 产物。新增应用层和本地渲染回归测试已通过；真实 PostgreSQL、Blob、Provider、渠道和指标回流仍待独立验收。

## 5. 更新协议

完成一项后按顺序执行：

1. 先补代码、契约和测试证据。
2. 更新本文件的队列和阶段表。
3. 同步[迁移计划](./12-video-production-migration.md)、[执行跟踪](./13-video-production-execution.md)、[能力地图](../../infra/01-capability-map.md)和[交付路线图](../../infra/03-delivery-roadmap.md)。
4. 保留未完成的外部验收门槛，不以模拟测试、Fixture 或页面存在替代真实运行证据。
