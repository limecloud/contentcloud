# V7：从业务事实到成片交付

状态：`方案草案，待产品、工程、安全、内容运营和真实 Provider 联合评审`。

更新时间：2026-08-03。

V7 的目标是修正 V6 工作台已经暴露出的产品断层：任务可以走完状态机，却没有把知识、策略、剧本、分镜和媒体作为服务端业务事实交付；空 manifest 也可以让任务进入 `delivered`。V7 不再把“流程状态完成”当成“内容生产完成”，而是交付一条可恢复、可审核、可预览、可下载的营销视频生产链。

```text
来源资料
  -> SourceRevision / Evidence
  -> KnowledgeObject / KnowledgeSnapshot
  -> AudienceStrategy / Brief
  -> ContentBatch / ContentItem
  -> StoryboardPackage / approved frames
  -> MediaGenerationJob / generated takes
  -> MediaReview / deterministic post-production
  -> final video Artifact / DeliveryPackage
  -> PublishedCreativeBinding / PerformanceObservation
```

V7 是对 V6 `zero-exec` 边界的一次受控升级：服务端仍不执行客户上传代码，不把通用创作迁入同步 HTTP 请求，也不自动批准事实、权利或内容；但受信 Worker 可以在显式租户能力、预算、Provider Profile 和人工 Gate 下调用批准的视频生成 Provider。Provider 凭据只以部署环境或 Secret Manager 引用存在，永不写入业务表、日志、前端或审计正文。

## 1. V7 要解决的问题

当前实现同时存在四个结构性问题：

1. `StageRun.output_refs` 只是字符串，不能证明输出对象真实存在、属于当前租户、通过 Schema 校验或仍可读取。
2. `TaskRevision.content` 已保存在服务端，但任务页只显示 Revision 数量，不展示正文、镜头、引用和检查结果。
3. `TaskDelivery` 允许空 manifest 直接进入 `delivered`，没有 Artifact、媒体文件、Provider Job、质检或下载事实。
4. Source、Evidence、KnowledgeObject、Submission、Artifact 和 DeliveryPackage 已经存在，但新 Task 工作流没有复用它们，形成第二套不完整的业务模型。

V7 的核心修复不是给页面补一个 JSON 预览，而是让每个 Stage 产出真实领域对象，并让任务状态由这些对象的存在、摘要和审核状态派生。

### 当前实现证据

| 位置 | 当前行为 | V7 结论 |
| --- | --- | --- |
| `internal/domain/orchestration.go` | StageRun 只保存 `OutputRefs []string` | 增加类型化 TaskStageOutput，不在 StageRun 塞正文 |
| `internal/app/task_governance.go` | Stage 上报直接保存字符串引用 | 服务端按 Stage Contract 校验规范对象和 digest |
| `internal/app/task_governance.go` | 空 manifest 也可创建 delivered TaskDelivery | 改为 final Artifact + MediaReview + DeliveryPackage hard gate |
| `web/src/workspace/workOS.tsx` | Revision 只显示数量，Delivery 只显示 digest | 按类型渲染正文、分镜、视频、质检和文件 manifest |
| `internal/localworkspace/seedance_v5.go` | 只导出提示包，明确不执行生成和下载 | 保留 external_operator；新增 managed Provider 路径 |

## 2. 产品承诺

一个 `marketing_video` 任务只有在用户能够完成以下动作时，才算完整交付：

- 在服务端看到已登记来源、解析 Evidence、候选知识和缺口。
- 在任务中阅读策略、Brief、完整剧本、引用、分镜和每个 Revision 的差异。
- 查看分镜图和生成视频，知道模型、Provider Profile、输入摘要、费用和失败原因。
- 对成片执行人工质检、选择 take、必要的确定性后期和最终批准。
- 下载真实视频文件及 manifest；从 Delivery 反查所有上游事实和审核决定。

“已完成 Stage”“已接受 Revision”“Provider 生成成功”“质检通过”“已交付”是五种不同状态，不得合并。

## 3. 最终系统模型

```text
                         Content Work OS

  Web / CLI / MCP / Agent
            |
            v
  Task Orchestrator -----------> Governance
  WorkTask / StageRun            SubmissionRevision / ApprovedSnapshot
            |                                  |
            | typed TaskStageOutput            |
            v                                  v
  Canonical Domain Objects <-------------- immutable refs
  Source / Evidence / Knowledge / Brief / Content / Storyboard
            |
            v
  Managed Media Runtime
  MediaGenerationJob -> ProviderAdapter -> external Provider
            |                    |
            | poll/callback      | approved egress only
            v                    v
  Artifact Store <---------- generated image/video/audio
            |
            v
  MediaReview -> PostProduction -> final Artifact
            |
            v
  DeliveryPackage -> download / external publish binding / results
```

PostgreSQL 保存事务、状态和摘要；对象存储保存来源文件、图片、视频、音频、审核接触图和交付包；队列由数据库租约驱动，V7 不为首版引入 Kafka、Redis Streams 或新的工作流引擎。

## 4. V7 关键决策

| ID | 决策 | 原因 |
| --- | --- | --- |
| D7-01 | Stage 输出升级为 `TaskStageOutput` 类型化引用 | 裸字符串无法校验存在性、租户、版本、摘要和状态 |
| D7-02 | 复用 Submission/ApprovedSnapshot/Artifact/DeliveryPackage，停止扩展平行的 TaskRevision/TaskDelivery 写模型 | 统一事实源，避免两套审批和交付语义继续漂移 |
| D7-03 | `delivered` 必须由最终视频 Artifact、通过的 MediaReview 和 ready DeliveryPackage 共同派生 | 阻止空 manifest 和只有剧本的伪交付 |
| D7-04 | Source Worker 继续做确定性解析；知识候选由本地 Agent 或受租约 Automation 生成，服务端不自动批准 | 保留证据和人工治理边界 |
| D7-05 | 服务端新增受信 Media Worker，仅调用显式启用的媒体 Provider | 视频生成需要异步、重试、费用和资产下载能力 |
| D7-06 | 自动化适配器使用版本化 `managed_http_video` 契约；首个真实 Vendor 在 M7-0 冻结 | 保持 Provider 可替换，生产发布仍要求至少一个真实适配器通过验收 |
| D7-07 | Seedance 导出保留为 `external_operator` 兼容模式，生成后必须上传或登记真实 MP4 才能继续 | 不丢失 V5 现有流程，也不再把导出包称为成片 |
| D7-08 | Provider Profile、预算、并发和数据披露是租户能力的一部分 | 防止未知费用、越权发送素材或过期模型配置 |
| D7-09 | 工作台直接渲染类型化对象，未知 Schema 使用安全 JSON fallback | 用户先看到业务内容，同时保持未来类型兼容 |
| D7-10 | 历史空交付保留为审计记录并标记 `legacy_incomplete`，不静默补造 Artifact | 历史真实性高于页面整洁 |

## 5. 完整交付范围

### 必须交付

- 项目资料上传、解析、Evidence 审核、知识候选、知识包和快照的服务端闭环。
- 营销视频任务的类型化 Stage 输出和统一血缘。
- 策略、Brief、剧本 Revision、分镜、媒体 Job、take、质检和 Delivery 的服务端展示。
- 托管视频生成 Job：提交、轮询或回调、超时、取消、重试、费用记录和输出下载。
- Provider Adapter、Provider Profile、SecretRef、租户开关、预算和并发门禁。
- 对象存储中的图片、视频和交付包，以及短期签名下载 URL。
- 人工 Gate：知识批准、剧本批准、分镜锁定、生成费用确认、take 选择和最终交付批准。
- 确定性后期：字幕、品牌资产、CTA 和有效 Offer 合成；动态权益失效时阻断最终渲染。
- 真实 `marketing_video` Golden Journey 和一条金陵古都香候选资料路径。

### NOT in scope

- 自动登录或自动发布到抖音、Seedance Web、微信公众号及其他外部账号；V7 只生成和交付成片。
- 服务端通用 LLM 代理或任意 Prompt 执行；知识和剧本创作继续由本地 Agent 或受租约 Automation 完成。
- 自动批准 Evidence、产品事实、素材权利、剧本、分镜或成片。
- 浏览器内完整非线性视频编辑器；首版只提供预览、take 选择和受约束的确定性后期配置。
- 同时接入多个真实付费视频 Vendor；首版完成一个真实适配器，其他 Vendor 通过同一契约后续增加。
- 静默迁移或删除 V6 历史任务、Revision 和 Delivery；只增加兼容投影和完整性标签。
- 自动从整个本地目录上传资料；金陵古都香仍按用户选择的文件和披露清单导入。

## 6. What already exists

| 现有能力 | V7 处理 |
| --- | --- |
| SourceRevision、对象存储和 Source Worker | 直接复用；补任务绑定、候选知识输出和进度投影 |
| Evidence、KnowledgeObject、KnowledgePack、KnowledgeSnapshot | 作为知识 Stage 的规范输出，不新建第二套知识表 |
| SubmissionRevision、ApprovedSnapshot、Review | 作为策略、内容和分镜正式审核事实，不让 TaskRevision 替代 |
| TaskRun、RunAttempt、租约、心跳、重试和进度事件 | 复用到本地 Agent/Automation；Media Worker 使用同样的租约原则 |
| StoryboardPackage、SeedancePromptPackage | 复用 Schema、digest、rights、continuity 和 Provider Profile 门禁 |
| Artifact、DeliveryPackage、Blob Store | 扩展支持 generated take、final video、review proxy 和媒体元数据 |
| PublishedCreativeBinding、PerformanceObservation | 继续作为外部发布和效果回流事实 |
| WorkTask、StageRun、GateEvaluation | 保留编排职责；StageRun 不再承载业务正文 |
| TaskRevision、TaskDelivery | V7 只读兼容；新任务通过规范 Submission/Artifact/DeliveryPackage 投影 |

## 7. 里程碑

| 里程碑 | 目标 | 退出条件 |
| --- | --- | --- |
| M7-0 方案冻结 | 边界、状态机、Provider 和迁移策略评审 | 产品、工程、安全、内容运营签字 |
| M7-1 事实源收敛 | 类型化 StageOutput 和规范对象投影 | 新任务不再产生裸 `output_refs` 业务结果 |
| M7-2 知识与内容可见 | Source 到剧本完整展示和审核 | 空知识库有明确补料动作，Revision 正文可读 |
| M7-3 分镜闭环 | 分镜生成、媒体存储、审核和锁定 | 每个 shot 有可预览首尾帧和 locked digest |
| M7-4 托管视频生成 | 一个真实 Provider 端到端生成并回收 MP4 | 超时、重试、取消、预算和重复回调验收通过 |
| M7-5 质检与成片 | take 选择、后期、最终 Artifact 和审批 | 用户可在 Web 播放、审核并下载成片 |
| M7-6 交付与迁移 | DeliveryPackage、历史标签和血缘完成 | 无 Artifact 不可 delivered；旧记录不丢失 |
| M7-7 生产验收 | 金陵古都香受控路径和真实租户验收 | Golden Journey、全量测试、Canary 和回滚演练通过 |

## 8. 文档导航

| 文件 | 内容 |
| --- | --- |
| [01-product-and-pipeline.md](./01-product-and-pipeline.md) | 用户目标、完整生产链、Gate 和工作台要求 |
| [02-domain-and-state-machine.md](./02-domain-and-state-machine.md) | 事实源、类型化输出、媒体 Job、状态机和 API 投影 |
| [03-server-runtime-and-providers.md](./03-server-runtime-and-providers.md) | Worker、Provider、Secret、对象存储、重试和运维 |
| [04-workspace-and-web-experience.md](./04-workspace-and-web-experience.md) | 本地 Workspace、金陵资料导入和服务端工作面 |
| [05-migration-security-and-acceptance.md](./05-migration-security-and-acceptance.md) | 迁移、安全、测试、性能、失败模式和验收 |
| [PLAN.md](./PLAN.md) | 实施工作包、依赖、并行车道和进度台账 |
| [prototype.html](./prototype.html) | 可直接打开的 V7 生产工作台原型：剧本、分镜、生成、质检、后期和交付 |

## 9. 成功指标

- 新建营销视频任务中，100% 的 completed Stage 至少引用一个可读取、摘要匹配的规范对象；控制型 Stage 除外。
- 100% 的 `delivered` 营销视频任务包含至少一个 `video/mp4` 最终 Artifact、通过的最终质检和 ready DeliveryPackage。
- 任务页可以在两次点击内打开知识、剧本正文、分镜、生成视频、失败详情或交付文件。
- Provider 重试不会产生重复扣费任务；相同 idempotency key 最多对应一个活动外部 Job。
- 线上对象存储下载不经过数据库缓冲，API 进程内存不随视频大小线性增长。
- 金陵古都香候选资料不会被自动升级为 approved；缺产品、权利或标签安全信息时停在明确的 blocked Gate。

## 10. 决策记录

| 日期 | 决策 | 原因 |
| --- | --- | --- |
| 2026-08-03 | V7 必须完整交付来源、知识、剧本、分镜、视频、质检和成片 | V6 状态机完成但业务产物为空，无法构成真实交付 |
| 2026-08-03 | 服务端允许受治理的媒体 Provider 调用 | 真实视频生成不能继续停留在人工复制提示词 |
| 2026-08-03 | 规范对象优先于 Task 平行模型 | 已有 Submission、Artifact 和 DeliveryPackage 足以承载正式事实 |
| 2026-08-03 | 不以自动批准换取流程顺滑 | 事实、权利、内容和费用仍需要明确授权边界 |
