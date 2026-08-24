# 视频生产迁移执行跟踪

状态：`in progress`。

更新时间：2026-08-23。

本文件是 [视频生产迁移计划](./12-video-production-migration.md) 的执行台账；总进度入口见 [视频生产迁移推进计划](./16-video-production-progress-plan.md)，文档状态同步见[文档同步计划](./23-documentation-sync-plan.md)，D2 持久化对照见[D2 PostgreSQL 对照计划](./25-stage-d2-postgres-parity-plan.md)，E1d-b PostgreSQL Final Render 对照见[专项计划](./26-e1d-b-postgres-final-render-plan.md)，清理失败恢复见[恢复执行计划](./28-e1d-b-recovery-execution-plan.md)，R2 接口、权限和重试入口见[R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)。迁移范围、领域映射和阶段完成门槛以迁移计划为准；本文件只记录当前实施项、证据和下一步，不创建新的业务事实模型。

## 1. 当前里程碑

| 项目 | 状态 | 本轮证据 | 下一步 |
| --- | --- | --- | --- |
| D0 文档基线同步 | `complete` | 能力地图、交付路线图、迁移计划、执行台账和总进度计划已互相链接并说明唯一主链 | 随阶段状态变更继续同步 |
| A. 迁移边界与现状对账 | `complete` | `12-video-production-migration.md` §1-2；`docs/infra/01-capability-map.md` §9-10 | 维持无平行任务、资产和交付模型 |
| B. 视觉资产一致性与镜头绑定 | `complete` | `StoryboardVisualBinding`、Storyboard Schema、服务端与本地漂移测试 | 在 Provider 任务中继续消费锁定摘要 |
| C. 批量视频准入 | `complete` | `CreateMediaGenerationBatch`、批量原子写入、确认后统一进入 `queued`、月度预算与并发门禁、`POST /tasks/{taskID}/media-jobs/batch`；`TestCreateMediaGenerationBatchAdmissionMatrix`、`TestCreateMediaGenerationBatchRejectsEmptyInputWithoutWrites` | 启动 D：Provider 异步恢复与费用对账 |
| D. Provider 异步恢复与费用对账 | `in_progress` | 未知提交已写入 `ProviderAttempt.NextPollAt`，并允许 `submitting -> awaiting_external_result`；补录外部 ID 后继续轮询且不重复提交；状态查询未知后可恢复到 `running`、`failed` 或已请求取消的 `cancelled`，不新增 Attempt；终态 `ActualMinor` 已统一回写任务和调用尝试；取消 unknown 后确认取消会收敛 Runtime Effect；`internal/application/media_pipeline_state_test.go`、`internal/application/media_pipeline_cancel_test.go` 定向测试通过；D2-1 已完成存储接口与测试盘点，D2-2 PostgreSQL 集成测试实现已完成 | 按 [状态查询恢复矩阵计划](./15-provider-state-matrix.md) 和 [D2 PostgreSQL 对照计划](./25-stage-d2-postgres-parity-plan.md) 执行 D2-3 真实 PostgreSQL 测试，随后补齐 Memory/PostgreSQL 一致性、真实账单回执和真实 Provider 验收 |
| E. 视频后期、旁白与确定性合成 | `in_progress` | E0/E1a 文档契约、E1b Manifest 值对象、E1c Manifest 驱动 `CreateFinalRender`、Composition Worker 端口与确定性独立 MP4 输出、E1d-a 矩阵、E1d-c 交付门禁、E1d-b Memory 原子写入测试和 PostgreSQL 原子事务测试入口已完成；入口重新校验 ApprovedSnapshot、候选 Artifact、MediaReview、时间轴引用和输出契约；R2-1/R2-2 接口/权限/HTTP 边界与状态 CAS、R2-3 清理失败自动登记、R2-4/R2-5 查询/详情/CAS 重试 BFF 已完成 | 继续完成 [E1d 验证计划](./19-stage-e1d-failure-recovery-plan.md)、[E1d-b PostgreSQL Final Render 计划](./26-e1d-b-postgres-final-render-plan.md) 与 [R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md) 的独立进程恢复、PostgreSQL/Blob 真实执行和同 digest 组合幂等；再接入具备真实音视频编码能力的 native Worker，在真实 Worker、完整失败恢复和 D3 真实 Provider 证据前不宣称阶段 E 完成 |
| F. 剪映草稿导出 | `in_progress` | F0 文档基线、F1 确定性 ZIP、F2 完整输入/血缘校验、F3 失败矩阵、F4 导出 lint、F4b 应用/HTTP/CLI 分层入口和契约测试已完成；代码见 `internal/application/jianying_export.go`、`internal/local/export/jianying.go`、`internal/transport/http/content_handlers.go`、`internal/transport/cli/artifacts.go` | 按[阶段 F 剪映导出计划](./22-stage-f-jianying-export-plan.md)完成 F5 真实导入验收 |
| G. 真实闭环验收与效果回流 | `not_started` | - | 依次验收 C-F |

本轮 E1d-b 增量：`CreateFinalRender` 在 Blob `Get`/`Put` 或数据库事实写入失败时不创建最终事实；写入后报错会清理临时 Blob，清理失败返回可重试诊断。证据见 [E1d-b Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md) 和 [Blob 故障注入计划](./27-e1d-b-blob-fault-injection-plan.md)，PostgreSQL 真实执行、后台重试和跨进程恢复仍未完成。

本轮 E1d-b R1 增量：新增 `RuntimeCleanupDiagnostic` 持久化契约、Memory/PostgreSQL 实现和 `00053_runtime_cleanup_diagnostics.sql`；以租户 + `object_key` 防止同一临时对象重复登记。该事实尚未接入清理路径自动写入，后台重试、跨进程恢复和真实 PostgreSQL 运行仍未完成。

本轮 E1d-b R2-1 增量：完成 Runtime 清理诊断接口、人工权限和 HTTP 路由边界审计；当前只有创建/读取接口，尚无状态 CAS、清理路径自动登记或查询/重试入口。R2 实现按[专项计划](./29-e1d-b-r2-cleanup-retry-plan.md)继续，阶段 E 保持 `in_progress`。

本轮 E1d-b R2-2 增量：`Runtime.Repository` 新增诊断状态 CAS 更新；Memory/PostgreSQL 均拒绝旧版本、非法状态转移、终态重试和身份篡改。`runtime_cleanup_test.go` 与专用 PostgreSQL 集成测试入口已建立，定向及全量 Go 测试通过；真实 PostgreSQL 因未设置 `CONTENTCLOUD_TEST_DATABASE_URL` 明确跳过。自动登记、人工查询/重试和跨进程恢复仍未完成。

本轮 E1d-b R2-3/R2-5 增量：`CreateFinalRender` 在数据库事实写入失败且临时 Blob 删除失败时自动登记脱敏 `RuntimeCleanupDiagnostic`；诊断登记失败不会覆盖原始事实错误。Operations Service 提供租户范围列表、详情和 `pending/failed -> retrying` 的 CAS 重试，删除成功、对象不存在和再次失败分别收敛为 `cleaned`、`not_found` 和带退避时间的 `failed`；BFF 新增 `/api/bff/runtime/cleanup-diagnostics` 列表、详情和 `/retry` 路由，并拒绝 Worker/Device 类型旁路人工权限。相关应用、HTTP、Memory 并发和 PostgreSQL 独立 Store 测试入口已建立；真实 PostgreSQL 连接、跨进程恢复和同 digest 组合幂等仍待 R2-6/R3-R5。

本轮 E1d-b R2-6c 增量：`TestRuntimeCleanupDiagnosticsBFFRejectsCrossTenantReadAndRetry` 验证外租户只能得到空列表，详情和重试均为 404，且响应不包含诊断 ID、临时对象键或 Manifest digest；`TestRuntimeCleanupDiagnosticsBFFRejectsDeviceToken` 使用真实 Device Token 验证人工清理 BFF 的列表、详情和重试均返回 401，诊断状态、版本和尝试次数不变。R2-6c 已完成；真实 PostgreSQL/Blob 和跨进程恢复仍待补齐，阶段 E 保持 `in_progress`。

本轮 E1d-b R2-6b 增量（2026-08-23）：现有 `contentcloud-worker` Runtime event loop 已按租户调用清理恢复服务；过期 `retrying` claim 会通过 CAS 回收为 `failed` 后再次处理，失败诊断尊重 `next_retry_at`，并记录 `runtime_cleanup` 维护心跳。Memory 测试证明第二个 Application 实例可接手持久化诊断并完成清理；专用 PostgreSQL 进程执行和真实 Blob 组合故障仍待补齐。

本轮 E1d-b R2-6 增量（2026-08-23）：新增 `TestProcessRuntimeEventsReconcilesCleanupDiagnostics`，覆盖 Runtime event worker 维护循环对清理诊断的读取、Blob 删除、终态收敛和 `runtime_cleanup` 心跳。该证据属于 Memory/单进程集成测试，不替代专用 PostgreSQL 进程级恢复、真实 Blob 组合故障或同 digest 幂等；阶段 E 继续保持 `in_progress`。

本轮 F0 增量：完成 视频生产迁移 `presentation_bundle`/Jianying 行为审计，固定 ZIP 条目、JSON 摘要、素材安全路径、输入血缘和导出不改变发布状态的边界。证据见[阶段 F 剪映导出计划](./22-stage-f-jianying-export-plan.md)。

本轮 F1-F4b 增量：新增 `ExportJianying` 只读导出器和 `LintJianyingArchive`，生成包含 `draft_info.json`、`manifest.json` 和安全命名 `assets/*` 的确定性 ZIP，并独立校验 ZIP 条目、JSON 摘要、Artifact 大小、SHA-256 和 MIME/扩展名契约；应用服务、HTTP BFF 和 CLI 只通过交付编排层读取并重新核对批准快照、选中视频审核、最终审核、交付包、Artifact 和 Blob 摘要，不直接访问 Runtime 或持久化实现，也不创建第二套业务事实；应用、HTTP、CLI 和 exporter 测试已通过。重复、摘要漂移、审核未批准、路径穿越、交付包缺产物、Blob 缺失和 lint 失败测试已通过。F5 真实剪映导入仍待完成。

本轮 E1d-b 增量：新增 `TestCreateFinalRenderBlobPutFailureLeavesNoFinalFactsAndCleansObject`，覆盖写入前失败和写入后返回错误两种故障；两种路径都不创建最终 Artifact/审核，且写入后对象会清理。

本轮交付幂等增量（2026-08-23）：内容快照交付与视频最终成片交付统一使用 `tenant + approved_snapshot + content_item` 的稳定业务键生成 DeliveryPackage ID；重复请求先回读已有包，Memory/PostgreSQL 在并发主键冲突后也回读已提交包。新增 V3 全链路和视频 Golden Journey 重试断言，确保不重复创建交付事实或留下重复 Blob；该项只改善内部交付一致性，不替代真实渠道回执和外部平台验收。

## 2.17 分层平台增量：文章与电商进入统一编排主链

- [x] 文章工作台通过 `article_collaboration` 模板别名绑定内置文章 SOP；电商新增 `commerce_content` 内容类型、迁移约束和五阶段 SOP（商品信息、卖点策略、内容变体、渠道预览、渠道交付）。
- [x] 两类工作台都经 Customer Studio BFF 创建同一种 `WorkTask`，将 `business_brief` 与 `workbench` 固定引用写入 `RequestedOutput`，使用同一 `runtime-policy/customer-studio-v1` 编译为 Runtime Job。
- [x] `TestCustomerStudioArticleAndCommerceWorkbenchesUseSharedTaskAndRuntime` 已证明不同 UI 布局不会产生第二套任务、流程或 Runtime 模型；文章 Runtime 图包含 6 个阶段/Gate 节点，电商包含 7 个，差异来自各自已发布 SOP。
- [x] Bootstrap 设备授权投影已统一：Memory 按有效 `project_device_grants` 动态计算 `ConnectedDevices`，与 PostgreSQL 查询一致；重连、心跳、重复授权和撤销回归测试通过。

上述完成项只证明本地应用、Memory 和迁移约束的主链一致性；真实 PostgreSQL 执行、Blob 故障组合、Provider、音视频 Worker、外部渠道回执和 Performance 导入仍由 D/E/F/G 门槛约束，不能用本轮测试替代。

## 2.18 跨业务主链收口（2026-08-23）

文章和电商兼容内容版本入口已收敛到统一 `SubmissionRevision` 写入，旧 `TaskRevision` 仅保留历史回读；交付包校验通过 ApprovedSnapshot 的任务工作区引用识别归属，支持业务对象 ID 与任务 ID 分离。新增电商完整链路和 Schema/确定性渲染测试，证明不同工作台仍共享 `WorkTask -> SOP/Runtime -> Review -> ApprovedSnapshot -> Artifact/Delivery -> Performance/Rating`。这只补齐 Memory/应用层证据，不改变 D、E、F、G 的真实基础设施状态。

## 2. 阶段 C 待办清单

阶段 C 只能在以下项目全部完成后标记为 `complete`：

- [x] 批量输入 DTO、逐项预检和统一费用报价。
- [x] 费用未确认时只返回 `confirmation_required`，不创建任务。
- [x] 任一条目阻断时整批返回 `blocked`，不创建部分任务。
- [x] 内存和 PostgreSQL 存储使用批量原子写入，并拒绝重复幂等键。
- [x] 暴露批量准入 HTTP 路由并复用现有租户/角色校验。
- [x] 确认费用后全部任务进入 `queued`，而不是继续停留在费用确认状态。
- [x] 将既有 Provider 已用费用纳入月度预算门禁，并返回可解释的阻断原因。
- [x] 将 Provider 并发占用纳入批量门禁，避免整批超过可用容量。
- [x] 增加批量专用测试：零任务创建、整批阻断、币种不一致、预算、并发和幂等冲突。

本轮完成：`CreateMediaGenerationBatch` 在聚合费用确认后统一清理任务错误字段并写入 `queued`；新增 `MediaProviderUsage`，按租户、Provider 和月份聚合实际费用、未完成任务估算费用及活动任务数；批量准入在写入前阻断超过月度预算或并发上限的 Provider。`TestCreateMediaGenerationBatchAdmissionMatrix` 覆盖费用未确认不落库、整批阻断不部分写入、币种不一致、确认后统一 `queued` 和幂等冲突原子性；`TestCreateMediaGenerationBatchRejectsEmptyInputWithoutWrites` 覆盖空批次；`TestMediaProviderUsageAndMonthlyBudgetGate` 和 `TestMediaProviderConcurrencyGate` 覆盖预算与并发边界。阶段 C 已完成，下一阶段为 Provider 异步恢复与费用对账；HTTP 路由沿用现有租户/角色校验，尚未增加独立端到端 HTTP 测试。

## 2.1 阶段 D 已完成项

- [x] 提交结果未知时保存 `PROVIDER_SUBMIT_UNKNOWN`、`NextPollAt` 和 `awaiting_external_result`，确保 Worker 可重新调度。
- [x] 状态机允许 `submitting -> awaiting_external_result`，不把未知外部副作用错误地留在提交中状态。
- [x] 补录外部任务 ID 只能更新既有 `ProviderAttempt`，恢复流程只调用 Provider `Status`，不得再次 `Submit`。
- [x] 增加未知提交、补录、恢复轮询和无重复提交回归测试。

阶段 D 仍未完成。上述证据只覆盖提交未知这一条恢复分支，不能替代真实 Provider 验收。

## 2.2 阶段 D 本轮完成项：终态费用与取消对账一致性

- [x] Provider 返回 `succeeded`、`failed` 或 `cancelled` 时，先将 `ActualMinor` 写入 `ProviderAttempt` 和 `MediaGenerationJob`，再执行下载、Artifact、审核或终态转换。
- [x] Provider 输出处理后续失败时，已发生的实际费用仍可从媒体任务和调用尝试读取。
- [x] 取消请求结果未知进入 `awaiting_external_result` 并设置 `NextPollAt`；后续状态确认取消时，任务、调用尝试和 Runtime Effect 一起进入终态。
- [x] 负数实际费用被拒绝，不写入业务事实。
- [x] 证据：`TestCancelMediaGenerationJobCallsProviderBeforeLocalTerminalState`、`TestFailedMediaGenerationPersistsActualProviderCost`、`TestSubmitUnknownIsScheduledAndReconcilesWithoutDuplicateSubmit`；完整 `go test ./...` 与 `git diff --check` 通过。

本项不等于阶段 D 完成：真实 Provider、实际账单回执的端到端对账，以及状态查询 unknown 的完整矩阵仍待补齐。

## 2.3 阶段 D 本轮完成项：状态查询异常后的恢复轮询

- [x] 状态查询返回错误时，任务写入 `PROVIDER_STATUS_UNKNOWN` 并回到 `awaiting_external_result`，保留外部任务 ID、`LastPolledAt` 和 `NextPollAt`。
- [x] 下一次处理收到 `running` 时只轮询既有外部任务，不创建新的 `ProviderAttempt`，并清理旧的错误摘要。
- [x] 证据：`TestProviderStatusUnknownRecoversByPollingWithoutResubmit`；同时修正任务错误字段与 Attempt 错误字段在未知->运行中转换时的一致性。

本项只完成状态查询异常到运行中这一条分支。失败、取消、成功后的完整矩阵、真实 Provider 和账单回执仍待补齐，阶段 D 保持 `in_progress`。

## 2.4 阶段 D 本轮完成项：未知状态恢复到失败与取消

- [x] 状态查询第一次失败后，下一次轮询到 `failed` 时任务进入 `failed`，并保留实际费用。
- [x] 状态查询第一次失败后，已请求取消的任务轮询到 `cancelled` 时任务进入 `cancelled`，并保留实际费用。
- [x] 两条恢复路径均复用原有 `ProviderAttempt`，不重复 `Submit` 或创建新的调用尝试。
- [x] 证据：`TestProviderStatusUnknownRecoversToTerminalStatesWithoutDuplicateAttempt`；测试覆盖 Memory Store，PostgreSQL 和真实 Provider 证据仍待补齐。

本项完成后，阶段 D 的模拟状态恢复矩阵已覆盖 `running`、`failed` 和已请求取消的 `cancelled`；真实 Provider、实际账单回执及 Memory/PostgreSQL 对照仍是阶段 D 的剩余门槛。

## 2.5 阶段 E 本轮完成项：输入契约审计

- [x] 对照 视频生产迁移 的 Jianying、旁白、媒体货币和 presentation bundle 服务，登记 ContentCloud 阶段 E 必须固定的输入集合。
- [x] 明确 `CreateFinalRender` 当前仅能证明选中视频摘要、幂等 manifest 和 `final_render` 审核入口，不能替代真实旁白、字幕或时间轴合成。
- [x] 新增[阶段 E 确定性合成输入与血缘计划](./17-stage-e-composition-plan.md)，并把 E1-E4 的实现和真实 Worker 门槛列入队列。

当时阶段 E 仍未进入代码实现；下一项是实现版本化 composition manifest，而不是修改阶段状态。当前阶段状态已由 E1c 回写为 `in_progress`。

## 2.6 阶段 E 本轮完成项：E1a manifest 文档契约

- [x] 明确 manifest 的批准快照、选中视频审核、旁白、字幕、品牌/CTA、时间轴、renderer 和输出契约字段。
- [x] 固定 canonical JSON、SHA-256 digest、输入摘要漂移、重复执行和权限重新校验规则。
- [x] 新增[E1 Composition Manifest 计划](./18-stage-e1-composition-manifest-plan.md)，将值对象实现、合成入口、测试和真实 Worker 运行拆为 E1b-E1e。

本项完成的是文档和契约设计，不代表 Go 值对象、确定性合成 Worker 或真实成片已经实现；随后 E1b、E1c 已分别补齐值对象和入口接线，阶段 E 当前为 `in_progress`。

## 2.7 阶段 E 本轮完成项：E1b Manifest 值对象

- [x] 新增 `internal/delivery/composition_manifest.go`，固定 schema、ApprovedSnapshot、选中视频审核、旁白、字幕、品牌/CTA、时间轴、renderer 和输出字段。
- [x] 增加 `Validate`、`CanonicalJSON` 和 `Digest`；摘要统一为 `sha256:<64 hex>`，不包含执行时间、执行 ID 或临时路径。
- [x] 旁白和字幕必须显式声明 `enabled`/`disabled`；时间轴片段校验边界、顺序、重叠和重复 ID。
- [x] 证据：`internal/delivery/composition_manifest_test.go` 覆盖摘要稳定性、输入变化和隐式输入拒绝；`go test ./internal/delivery` 通过。

本项只完成 manifest 值对象层；在该项完成时 `CreateFinalRender` 尚未消费该 manifest，真实旁白、字幕和确定性后期合成仍未完成。后续 E1c 已补齐入口接线，当前仍待 E1d-E1e。

## 2.8 阶段 E 本轮完成项：E1c Manifest 驱动最终渲染入口

- [x] `CreateFinalRenderInput` 增加必填 `manifest`，入口拒绝缺失或不符合值对象契约的清单。
- [x] 服务端重新读取并校验 `ApprovedSnapshot` 的租户、项目和摘要，校验选中视频 Artifact、`MediaReview(content)` 审核摘要、时间轴 Artifact 引用和输出契约。
- [x] 同一 Manifest digest 复用既有 `final_render` Artifact 和最终审核；摘要漂移不会覆盖旧产物。
- [x] `CreateFinalRender` 通过平台 Composition Worker 端口生成独立 MP4；默认 Worker 使用已批准的 `contentcloud.deterministic-compositor/1.0.0`，受控 native Worker 可在组合根替换并自行校验其 renderer 能力；输入 Manifest、时间轴、旁白/字幕和品牌/CTA 摘要会进入输出血缘元数据，Worker 不拥有业务事实。
- [ ] 具备真实音视频编码能力的 native Worker 仍需在受控环境验收；默认 Worker 不冒充旁白混音、字幕烧录或品牌画面合成证据。
- [x] 证据：`internal/application/storyboard_media.go`、`internal/application/media_pipeline_test.go`、`internal/application/dev_fixture_marketing_video.go`；覆盖缺 Manifest、批准快照摘要漂移和同一 Manifest 幂等复用，`go test ./internal/application ./internal/delivery` 通过。

E1c 已完成入口接线、事实校验和平台 Composition Worker 调用；默认 Worker 的独立 MP4 与完整输入血缘测试已通过，但不代表旁白、字幕、时间轴混音或品牌画面烧录已完成。E1d 的完整失败/恢复矩阵、native Worker 运行证据和真实 Provider 证据仍待补齐。

## 2.9 阶段 E 本轮完成项：E1d-a 失败与恢复验证矩阵

- [x] 完成正常、同 digest 重复、批准快照摘要漂移、输入 Artifact 摘要漂移、缺输入/租户错配、Worker 失败和失败恢复场景的预期结果定义。
- [x] 明确每个场景不得覆盖旧 Artifact；失败路径不得产生可交付的最终审核或交付包。
- [x] 新增[阶段 E1d 失败、恢复与摘要漂移验证计划](./19-stage-e1d-failure-recovery-plan.md)。

本项是文档和证据边界基线，不代表矩阵已经在 Memory/PostgreSQL 执行，也不代表真实 Worker 已运行。下一项是 E1d-b：补齐存储对照、Blob 读写失败和恢复断点测试。

## 2.10 阶段 E 本轮完成项：E1d-c 最终审核交付硬门禁

- [x] `TestMarketingVideoGoldenJourney` 验证最终 `MediaReview(final)` 仍为 `pending` 时，`BuildTaskDeliveryPackage` 必须失败。
- [x] 只有最终审核被批准且明确选中后，主链才允许创建 `DeliveryPackage`。

本项只证明交付门禁不会被失败/未批准状态绕过；最终审核写入失败时的 Artifact 清理、Blob 读写恢复、PostgreSQL 对照和真实 Worker 仍属于 E1d-b/E1e。

## 2.11 阶段 E 本轮完成项：E1d-b Memory 原子写入

- [x] `internal/persistence/memory/final_render_test.go` 覆盖 Artifact 与最终审核摘要不一致时的原子拒绝，失败后两者均不可读取。
- [x] 覆盖合法输入下 Artifact 与 `MediaReview(final)` 同时可读取，证明成功路径不会漏写其中一项。
- [ ] PostgreSQL 事务、Blob `Get`/`Put`/清理失败和跨进程恢复仍待补齐。

本项只完成 Memory Store 的原子性证据，不代表 E1d 或阶段 E 已完成。

## 2.12 阶段 D 本轮完成项：D2-2 PostgreSQL 集成测试实现

- [x] 新增 `internal/persistence/postgres/media_integration_test.go`，覆盖 Provider Profile/Binding、`MediaGenerationJob` 幂等与 CAS、`ProviderAttempt` 恢复字段、unknown 恢复不新增 Attempt、Job/Attempt 实际费用一致性、usage 汇总和 RLS 跨租户隔离。
- [x] 集成测试只接受 `CONTENTCLOUD_TEST_DATABASE_URL`；未设置时明确 `skip`，不会连接开发库或生产库。
- [x] 证据：`go test ./internal/persistence/postgres`、`go test ./...` 和 `git diff --check` 已通过；定向真实 PostgreSQL 测试因环境变量缺失跳过。

本项完成的是测试实现和安全的执行入口，不是 PostgreSQL 真实执行通过。D2-3 仍待在专用测试库运行，阶段 D 保持 `in_progress`。

## 2.13 阶段 E 本轮完成项：E1d-b PostgreSQL 原子事务测试入口

- [x] 新增 `TestPostgresFinalRenderAtomicityWithPostgres`，使用真实 V3 `ApprovedSnapshot` 前置，覆盖 Artifact 与 `MediaReview(final)` 同事务成功提交、审核插入冲突时 Artifact 回滚和租户隔离。
- [x] 测试只接受 `CONTENTCLOUD_TEST_DATABASE_URL`；未设置时明确 `skip`，不会连接开发库或生产库。
- [x] 证据：`go test ./internal/persistence/postgres`、`go test ./...` 和 `git diff --check` 已通过；定向 PostgreSQL 测试因环境变量缺失跳过。

本项完成的是 PostgreSQL 测试实现和安全执行入口，不是 PostgreSQL/Blob 真实故障恢复通过。E1d-b 仍待专用数据库、Blob Get/Put/清理失败和跨进程恢复证据，阶段 E 保持 `in_progress`。

## 2.14 阶段 E 本轮完成项：E1d-b Blob Get 失败无事实

- [x] Blob `Get` 在最终渲染读取候选成片时失败，服务返回原始读取错误。
- [x] 失败路径不创建 `final_render` Artifact，不创建 `MediaReview(final)`，也不产生可交付事实。
- [x] 证据：`internal/application/media_pipeline_blob_failure_test.go` 的 `TestCreateFinalRenderBlobGetFailureLeavesNoFinalFacts`；定向测试通过。

本项只关闭 Blob 读取失败这一项；随后已由 Blob `Put` 故障注入项补齐写入前/写入后报错及临时对象清理。跨进程恢复、同 Manifest digest 的 PostgreSQL/Blob 幂等重试和真实 PostgreSQL 执行仍待补，阶段 E 保持 `in_progress`。

## 2.15 阶段 E1d-b 恢复文档基线

- [x] 新增[ E1d-b 恢复执行计划](./28-e1d-b-recovery-execution-plan.md)，定义持久化清理诊断、后台重试、跨进程恢复和组合幂等的执行顺序。
- [x] 统一诊断最小字段：租户、项目、任务、请求、Manifest digest、`object_key`、原始原因、清理原因、状态、重试次数和 `next_retry_at`。
- [ ] 持久化实现、后台入口、跨进程测试和专用 PostgreSQL 执行仍未完成。

本项只完成文档基线，不改变 E1d-b 或阶段 E 的 `in_progress` 状态。

## 2.16 阶段 E1d-b 产物补偿与 G 血缘投影增量

- [x] 非最终媒体 Artifact 的 Blob 写入统一经 `persistArtifactObject`；分镜素材、Seedance PromptPackage 和批准快照导出在 Artifact 事实拒绝时清理临时对象。
- [x] DeliveryPackage 批量写入在任一 Blob 或交付事实失败时清理全部已写入对象，并在清理失败时返回对象级恢复详情。
- [x] `ProjectLineage` 增加 `TaskDelivery`、`ChannelBinding`、`ChannelPublication` 和批准快照基础继承边；渠道回执测试覆盖效果导入和 RatingDecision 下游节点。
- [ ] 上述实现仍需专用 PostgreSQL/Blob、真实 Provider 和真实渠道证据，不能将 Memory/模拟渠道测试标记为阶段 E/G 完成。

## 3. 阶段完成规则

每个阶段必须同时具备以下证据，才能更新为 `complete`：

1. 领域模型、API 或持久化契约已实现，并复用 ContentCloud 现有事实模型。
2. 正常、失败、重复请求和恢复路径均有测试；只有模拟 Provider 的测试不能替代真实 Provider 验收。
3. 租户隔离、角色权限、固定摘要、费用和外部副作用语义可追溯。
4. 本台账、迁移计划、能力地图和交付路线图的状态与证据一致。

## 4. 更新协议

完成一项后按以下顺序更新：

1. 先补代码、契约和测试证据。
2. 再更新本台账的状态和证据列。
3. 同步 [迁移计划](./12-video-production-migration.md)、[能力地图](../../infra/01-capability-map.md) 和 [交付路线图](../../infra/03-delivery-roadmap.md)。
4. 在阶段未满足全部门槛时，保持 `in_progress`，不得提前宣称迁移完成。
