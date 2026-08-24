# 视频生产能力迁移计划

状态：`in progress`。

更新时间：2026-08-23。

当前总进度见 [视频生产迁移推进计划](./16-video-production-progress-plan.md)；逐项证据见 [视频生产迁移执行跟踪](./13-video-production-execution.md)。本计划负责范围、映射和验收门槛；执行台账负责逐项状态和证据。阶段 D 当前按 [Provider 状态查询恢复矩阵计划](./15-provider-state-matrix.md) 和 [D2 PostgreSQL 对照计划](./25-stage-d2-postgres-parity-plan.md) 补齐异常、终态恢复和持久化一致性证据。

本轮 R1：清理失败诊断已落到现有 Runtime Repository，迁移 `00053_runtime_cleanup_diagnostics.sql` 提供租户隔离和租户 + `object_key` 唯一约束。R2-1/R2-2 已完成接口、权限、HTTP 边界审计和诊断状态 CAS；R2-3 已接入 `CreateFinalRender` 清理失败自动登记，R2-4/R2-5 已提供租户范围的查询、详情、CAS 重试和 BFF 路由。R2-6 的独立 Store/进程恢复、真实 PostgreSQL/Blob 执行和终态幂等仍待验收，详见[恢复执行计划](./28-e1d-b-recovery-execution-plan.md)和[R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)。

## 1. 目标与边界

将已获许可的视频生产业务迁移为 ContentCloud 的受治理视频生产能力，覆盖：内容拆解、视觉资产一致性、分镜、批量视频准入、真实 Provider 异步任务、旁白/合成、剪映草稿交付和效果回流。

迁移的是领域行为、输入输出约束和验收场景；不把 其 Python 服务、数据库 Schema、独立队列、Session 或前端直接作为 ContentCloud 的第二套业务系统。

唯一主链保持为：

```text
Source / WorkspaceMaterial / Evidence
  -> Knowledge / Brief / ContentBatch / ContentItem
  -> SubmissionRevision -> Review -> ApprovedSnapshot
  -> WorkTask / StageRun -> MediaGenerationJob / RuntimeAttempt / Effect
  -> Artifact -> MediaReview -> DeliveryPackage
  -> ChannelPublication / Receipt / PerformanceObservation
```

## 2. 映射原则

| 视频生产迁移 领域能力 | ContentCloud 权威落点 | 迁移规则 |
| --- | --- | --- |
| 小说、剧本、商品素材 | SourceRevision、WorkspaceMaterial、Evidence、Knowledge | 保留来源、权利和摘要链，不复制 Project 输入表 |
| 内容分析、分集、脚本规划 | Brief、ContentBatch、ContentItem、Novel 本地链 | 按内容 Profile 实现，不新建平行项目状态 |
| 角色、场景、道具、商品参考 | WorkspaceMaterial、Artifact、StoryboardPackage 的受控引用 | 新增视觉身份/版本锁投影，不能泛化成新的资产事实 |
| 分镜及镜头序列 | StoryboardPackage、ApprovedSnapshot、Artifact | 复用锁定摘要，补镜头资产绑定和失效传播 |
| 图像、视频、TTS 任务 | MediaGenerationJob、ProviderAttempt、Runtime Effect、Receipt | 迁移准入、恢复和计费规则，不复制独立任务队列 |
| 批量视频准入 | WorkTask Gate / Media Job admission | 形成可解释的放行、待确认、受阻结果 |
| 成片合成 | 确定性 Worker -> Artifact | 输入/输出摘要固定，产物进入既有审核和交付链 |
| 剪映草稿 | DeliveryPackage 的 `jianying` exporter | 导出是派生交付，不等同于发布 |
| 发布与表现回流 | ChannelPublication、Receipt、PerformanceObservation | 复用现有外部回执与学习边界 |

## 3. 阶段与进度

状态约定：`not_started`、`in_progress`、`blocked`、`complete`。每个阶段完成时，必须更新本表、关联能力地图和本文件的验收证据。

| 阶段 | 状态 | 交付物 | 完成条件 | 证据 |
| --- | --- | --- | --- | --- |
| A. 迁移边界与现状对账 | `complete` | 本计划、能力地图、交付路线图 | 视频生产迁移 能力映射到唯一主链；明确无平行事实模型 | 本文件 §1-2；`docs/infra/01-capability-map.md` §9-10；`docs/infra/03-delivery-roadmap.md` §6.1 |
| B. 视觉资产一致性与镜头绑定 | `complete` | `StoryboardVisualBinding`、身份锚点引用规则、锁定摘要与本地漂移检测 | 角色/场景/道具/商品参考只能以固定 digest 进入镜头；变更可识别并阻断旧分镜复用 | `internal/work/work_model.go`；`contracts/storyboard-package-1.0.schema.json`；`internal/work/storyboard_visual_binding_test.go`；`internal/local/workspace/v5_workflow_test.go`；`internal/application/v5_submission_test.go`；定向 Go 测试通过 |
| C. 批量视频准入 | `complete` | 批次准入 DTO、Gate、费用/依赖判定、确认后统一入队、API 和批量原子写入 | 全批输出放行/待确认/受阻；任何阻断项不得导致同批部分付费提交；预算、并发和幂等边界有解释性测试 | `internal/application/media_pipeline.go`；`internal/application/media_pipeline_batch_test.go`；`internal/application/media_pipeline_budget_test.go`；`internal/persistence/memory/media.go`；`internal/persistence/postgres/media.go`；`internal/transport/http/orchestration_handlers.go`；`go test ./internal/application` |
| D. Provider 异步恢复与费用对账 | `in_progress` | Provider Adapter 恢复契约、轮询/取消/unknown 对账、账单测试 | 远端任务 ID 固定后不重复提交；取消结果不明进入对账；实际费用可追溯 | D1 模拟恢复矩阵、D2-1 接口/测试盘点和 D2-2 PostgreSQL 集成测试实现已完成；真实 PostgreSQL 执行、账单和 Provider 验收待补 |
| E. 视频后期、旁白与确定性合成 | `in_progress` | E0/E1a 文档契约、E1b Manifest 值对象、E1c Manifest 驱动入口、E1d-a 矩阵基线、E1d-c 最终审核交付硬门禁、E1d-b Memory 原子写入测试、PostgreSQL 原子事务测试入口和 Blob `Get`/`Put` 失败无事实测试已完成；R2-1/R2-2 边界审计与状态 CAS、R2-3 清理失败自动登记、R2-4/R2-5 查询/详情/CAS 重试 BFF 已完成；R2-6 独立恢复、真实 PostgreSQL/Blob、同 digest 幂等、真实 Worker、旁白/字幕处理仍待实现 | 合成结果可重复校验且只能经 MediaReview 进入最终交付 | [阶段 E 合成计划](./17-stage-e-composition-plan.md)；[E1 Manifest 计划](./18-stage-e1-composition-manifest-plan.md)；[E1d 验证计划](./19-stage-e1d-failure-recovery-plan.md)；[E1d-b Blob 清理计划](./21-e1d-b-blob-cleanup-plan.md)；[Blob 故障注入计划](./27-e1d-b-blob-fault-injection-plan.md)；[恢复执行计划](./28-e1d-b-recovery-execution-plan.md)；[R2 清理重试计划](./29-e1d-b-r2-cleanup-retry-plan.md)；`CreateFinalRender` 已拒绝缺失/漂移输入，pending 最终审核不能创建交付包，真实合成待补 |
| F. 剪映草稿导出 | `in_progress` | F0 行为审计、F1 确定性 ZIP、F2 输入/血缘校验、F3 失败矩阵、F4 导出 lint、F4b 应用/HTTP/CLI 分层入口与契约测试已完成 | F5：真实 Artifact 的人工剪映导入验收 | [阶段 F 剪映导出计划](./22-stage-f-jianying-export-plan.md)；F1-F4b 已有代码、入口与测试证据，真实导入待补 |
| G. 真实闭环验收与效果回流 | `not_started` | 一个真实 Provider、真实交付包、发布/人工回执、PerformanceObservation 样本 | 从受审核脚本到 Provider、Artifact、剪映交付和结果导入完整可追溯 | 待补 |

## 4. 第一条实施闭环

首条闭环限定为营销视频，避免同时迁移漫剧、旁白和多平台分发：

```text
Approved ContentItem
  -> locked StoryboardPackage
  -> batch admission
  -> one approved Provider profile
  -> MediaGenerationJob / ProviderAttempt / Artifact
  -> content + final MediaReview
  -> DeliveryPackage
  -> Jianying draft archive + manifest
```

首期不包含视频编辑、续写、跨供应商自动路由、自动发布或通用资产仓库。它们必须在相应阶段具备真实运行证据后再排期。

## 5. 阶段更新规则

完成任一阶段时必须同时：

1. 将 §3 对应状态改为 `complete`，添加提交、测试或运行证据。
2. 更新 `docs/infra/01-capability-map.md` 中的能力状态与真实边界。
3. 更新 `docs/infra/03-delivery-roadmap.md` 的下一阶段和外部依赖描述。
4. 若 API、Schema、CLI、Skill 或运行手册发生变化，同步更新其权威文档和测试。
5. 更新 [执行跟踪台账](./13-video-production-execution.md)，保留未完成门槛，不以代码存在替代运行验收。

不得仅因有界面、Fixture 或模拟 Provider 就把阶段标记为完成。
